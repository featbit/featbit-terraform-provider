package openapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
)

const maxSnapshotBytes = 10 << 20

var (
	httpMethods   = []string{"get", "post", "put", "patch", "delete"}
	targetPattern = regexp.MustCompile(`^\$\.paths\['([^']+)'\]\.(get|post|put|patch|delete)$`)
)

type Lock struct {
	SourceURL string `json:"source_url"`
	SHA256    string `json:"sha256"`
	Bytes     int    `json:"bytes"`
}

type Overlay struct {
	Version string          `json:"overlay"`
	Info    OverlayInfo     `json:"info"`
	Extends string          `json:"extends,omitempty"`
	Actions []OverlayAction `json:"actions"`
}

type OverlayInfo struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

type OverlayAction struct {
	Target      string                 `json:"target"`
	Description string                 `json:"description,omitempty"`
	Update      map[string]interface{} `json:"update"`
}

type Inventory struct {
	OpenAPIVersion                   string         `json:"openapi_version"`
	APIVersion                       string         `json:"api_version"`
	PathCount                        int            `json:"path_count"`
	OperationCount                   int            `json:"operation_count"`
	SchemaCount                      int            `json:"schema_count"`
	SchemaPropertyCount              int            `json:"schema_property_count"`
	SchemasWithRequiredCount         int            `json:"schemas_with_required_count"`
	PropertiesWithEnumCount          int            `json:"properties_with_enum_count"`
	SecuritySchemes                  []string       `json:"security_schemes"`
	OperationsByTag                  map[string]int `json:"operations_by_tag"`
	ResponseStatusCounts             map[string]int `json:"response_status_counts"`
	OperationsWithOrganizationHeader int            `json:"operations_with_organization_header"`
	OperationsWithWorkspaceHeader    int            `json:"operations_with_workspace_header"`
	RequiredContextHeaderCount       int            `json:"required_context_header_count"`
	PaginatedOperationCount          int            `json:"paginated_operation_count"`
	MissingOperationIDCount          int            `json:"missing_operation_id_count"`
}

func LoadLock(path string) (Lock, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Lock{}, fmt.Errorf("read OpenAPI lock: %w", err)
	}
	var lock Lock
	if err := json.Unmarshal(content, &lock); err != nil {
		return Lock{}, fmt.Errorf("decode OpenAPI lock: %w", err)
	}
	if lock.SourceURL == "" || len(lock.SHA256) != 64 || lock.Bytes <= 0 {
		return Lock{}, errors.New("OpenAPI lock is incomplete")
	}
	return lock, nil
}

func Pin(ctx context.Context, client *http.Client, lock Lock, snapshotPath string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, lock.SourceURL, nil)
	if err != nil {
		return errors.New("construct OpenAPI snapshot request")
	}
	request.Header.Set("User-Agent", "terraform-provider-featbit-phase0-openapi-pin/1.0")
	response, err := client.Do(request)
	if err != nil {
		return errors.New("download OpenAPI snapshot")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download OpenAPI snapshot: unexpected HTTP status %d", response.StatusCode)
	}

	content, err := io.ReadAll(io.LimitReader(response.Body, maxSnapshotBytes+1))
	if err != nil {
		return errors.New("read OpenAPI snapshot")
	}
	if len(content) > maxSnapshotBytes {
		return errors.New("OpenAPI snapshot exceeds size limit")
	}
	if err := VerifySnapshot(content, lock); err != nil {
		return err
	}
	if err := os.WriteFile(snapshotPath, content, 0o644); err != nil {
		return fmt.Errorf("write OpenAPI snapshot: %w", err)
	}
	return nil
}

func VerifySnapshot(content []byte, lock Lock) error {
	if len(content) != lock.Bytes {
		return fmt.Errorf("OpenAPI snapshot length %d does not match lock length %d", len(content), lock.Bytes)
	}
	got := SHA256(content)
	if !strings.EqualFold(got, lock.SHA256) {
		return fmt.Errorf("OpenAPI snapshot SHA-256 %s does not match lock", got)
	}
	return nil
}

func Generate(snapshot, overlayContent []byte) ([]byte, []byte, Inventory, error) {
	document, err := decodeObject(snapshot)
	if err != nil {
		return nil, nil, Inventory{}, fmt.Errorf("decode OpenAPI snapshot: %w", err)
	}
	var overlay Overlay
	if err := json.Unmarshal(overlayContent, &overlay); err != nil {
		return nil, nil, Inventory{}, fmt.Errorf("decode OpenAPI overlay: %w", err)
	}
	if err := applyOverlay(document, overlay); err != nil {
		return nil, nil, Inventory{}, err
	}
	if err := validateOperationIDs(document); err != nil {
		return nil, nil, Inventory{}, err
	}

	applied, err := marshalCanonical(document)
	if err != nil {
		return nil, nil, Inventory{}, fmt.Errorf("encode overlaid OpenAPI: %w", err)
	}
	inventory, err := BuildInventory(document)
	if err != nil {
		return nil, nil, Inventory{}, err
	}
	inventoryContent, err := marshalCanonical(inventory)
	if err != nil {
		return nil, nil, Inventory{}, fmt.Errorf("encode OpenAPI inventory: %w", err)
	}
	return applied, inventoryContent, inventory, nil
}

func BuildInventory(document map[string]interface{}) (Inventory, error) {
	paths, ok := objectField(document, "paths")
	if !ok {
		return Inventory{}, errors.New("OpenAPI paths object is missing")
	}
	components, _ := objectField(document, "components")
	schemas, _ := objectField(components, "schemas")
	security, _ := objectField(components, "securitySchemes")

	inventory := Inventory{
		OpenAPIVersion:       stringField(document, "openapi"),
		PathCount:            len(paths),
		SchemaCount:          len(schemas),
		OperationsByTag:      map[string]int{},
		ResponseStatusCounts: map[string]int{},
	}
	if info, ok := objectField(document, "info"); ok {
		inventory.APIVersion = stringField(info, "version")
	}

	for _, pathValue := range paths {
		pathItem, ok := pathValue.(map[string]interface{})
		if !ok {
			continue
		}
		for _, method := range httpMethods {
			operation, ok := pathItem[method].(map[string]interface{})
			if !ok {
				continue
			}
			inventory.OperationCount++
			if stringField(operation, "operationId") == "" {
				inventory.MissingOperationIDCount++
			}
			if responses, ok := objectField(operation, "responses"); ok {
				for status := range responses {
					inventory.ResponseStatusCounts[status]++
				}
			}
			hasPageIndex := false
			hasPageSize := false
			if parameters, ok := operation["parameters"].([]interface{}); ok {
				for _, parameterValue := range parameters {
					parameter, ok := parameterValue.(map[string]interface{})
					if !ok {
						continue
					}
					name := stringField(parameter, "name")
					location := stringField(parameter, "in")
					if location == "header" && name == "Organization" {
						inventory.OperationsWithOrganizationHeader++
						if required, _ := parameter["required"].(bool); required {
							inventory.RequiredContextHeaderCount++
						}
					}
					if location == "header" && name == "Workspace" {
						inventory.OperationsWithWorkspaceHeader++
						if required, _ := parameter["required"].(bool); required {
							inventory.RequiredContextHeaderCount++
						}
					}
					if location == "query" && strings.EqualFold(name, "PageIndex") {
						hasPageIndex = true
					}
					if location == "query" && strings.EqualFold(name, "PageSize") {
						hasPageSize = true
					}
				}
			}
			if hasPageIndex && hasPageSize {
				inventory.PaginatedOperationCount++
			}
			tags, _ := operation["tags"].([]interface{})
			if len(tags) == 0 {
				inventory.OperationsByTag["<untagged>"]++
				continue
			}
			for _, rawTag := range tags {
				tag, ok := rawTag.(string)
				if ok {
					inventory.OperationsByTag[tag]++
				}
			}
		}
	}

	for _, schemaValue := range schemas {
		schema, ok := schemaValue.(map[string]interface{})
		if !ok {
			continue
		}
		properties, _ := objectField(schema, "properties")
		inventory.SchemaPropertyCount += len(properties)
		if required, ok := schema["required"].([]interface{}); ok && len(required) > 0 {
			inventory.SchemasWithRequiredCount++
		}
		for _, propertyValue := range properties {
			property, ok := propertyValue.(map[string]interface{})
			if !ok {
				continue
			}
			if enum, ok := property["enum"].([]interface{}); ok && len(enum) > 0 {
				inventory.PropertiesWithEnumCount++
			}
		}
	}
	for name := range security {
		inventory.SecuritySchemes = append(inventory.SecuritySchemes, name)
	}
	sort.Strings(inventory.SecuritySchemes)
	return inventory, nil
}

func SHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func applyOverlay(document map[string]interface{}, overlay Overlay) error {
	if overlay.Version != "1.1.0" {
		return fmt.Errorf("unsupported Overlay Specification version %q", overlay.Version)
	}
	if overlay.Info.Title == "" || overlay.Info.Version == "" || len(overlay.Actions) == 0 {
		return errors.New("OpenAPI overlay is missing required metadata or actions")
	}
	paths, ok := objectField(document, "paths")
	if !ok {
		return errors.New("OpenAPI paths object is missing")
	}

	seenTargets := make(map[string]struct{}, len(overlay.Actions))
	for _, action := range overlay.Actions {
		if _, duplicate := seenTargets[action.Target]; duplicate {
			return fmt.Errorf("duplicate overlay target %q", action.Target)
		}
		seenTargets[action.Target] = struct{}{}

		match := targetPattern.FindStringSubmatch(action.Target)
		if match == nil {
			return fmt.Errorf("unsupported overlay target %q", action.Target)
		}
		pathItem, ok := paths[match[1]].(map[string]interface{})
		if !ok {
			return fmt.Errorf("overlay path target does not exist: %s", match[1])
		}
		operation, ok := pathItem[match[2]].(map[string]interface{})
		if !ok {
			return fmt.Errorf("overlay operation target does not exist: %s %s", strings.ToUpper(match[2]), match[1])
		}
		if len(action.Update) != 1 || stringField(action.Update, "operationId") == "" {
			return fmt.Errorf("overlay target %q must update exactly one operationId", action.Target)
		}
		mergeObject(operation, action.Update)
	}
	return nil
}

func validateOperationIDs(document map[string]interface{}) error {
	paths, ok := objectField(document, "paths")
	if !ok {
		return errors.New("OpenAPI paths object is missing")
	}
	seen := map[string]string{}
	missing := []string{}
	for path, pathValue := range paths {
		pathItem, ok := pathValue.(map[string]interface{})
		if !ok {
			continue
		}
		for _, method := range httpMethods {
			operation, ok := pathItem[method].(map[string]interface{})
			if !ok {
				continue
			}
			id := stringField(operation, "operationId")
			identity := strings.ToUpper(method) + " " + path
			if id == "" {
				missing = append(missing, identity)
				continue
			}
			if previous, duplicate := seen[id]; duplicate {
				return fmt.Errorf("duplicate operationId %q on %s and %s", id, previous, identity)
			}
			seen[id] = identity
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("%d operations are missing operationId; first is %s", len(missing), missing[0])
	}
	return nil
}

func mergeObject(target, update map[string]interface{}) {
	for key, value := range update {
		updateObject, updateIsObject := value.(map[string]interface{})
		targetObject, targetIsObject := target[key].(map[string]interface{})
		if updateIsObject && targetIsObject {
			mergeObject(targetObject, updateObject)
			continue
		}
		target[key] = value
	}
}

func decodeObject(content []byte) (map[string]interface{}, error) {
	var document map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	return document, nil
}

func marshalCanonical(value interface{}) ([]byte, error) {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(content, '\n'), nil
}

func objectField(object map[string]interface{}, key string) (map[string]interface{}, bool) {
	if object == nil {
		return nil, false
	}
	value, ok := object[key].(map[string]interface{})
	return value, ok
}

func stringField(object map[string]interface{}, key string) string {
	if object == nil {
		return ""
	}
	value, _ := object[key].(string)
	return value
}
