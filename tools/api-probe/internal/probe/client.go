package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultProbeTimeout = 30 * time.Second
	maxResponseBytes    = 5 << 20
	probeUserAgent      = "terraform-provider-featbit-phase0-probe/1.0"
)

type TokenKind string

const (
	TokenService  TokenKind = "service"
	TokenPersonal TokenKind = "personal"
)

type NegativeAuthCase string

const (
	NegativeAuthMissing   NegativeAuthCase = "missing"
	NegativeAuthMalformed NegativeAuthCase = "malformed"
)

const syntheticMalformedToken = "api-synthetic-invalid-phase0-token"

type Client struct {
	baseURL            *url.URL
	token              string
	httpClient         *http.Client
	cleanupMu          sync.Mutex
	cleanupWorkarounds []string
}

type Envelope struct {
	Success *bool           `json:"success"`
	Data    json.RawMessage `json:"data"`
	Errors  json.RawMessage `json:"errors"`
}

// Observation is the only response representation intended for logs/evidence.
// It deliberately excludes request/response headers and raw data.
type Observation struct {
	Method          string   `json:"method"`
	PathTemplate    string   `json:"path_template"`
	HTTPStatus      int      `json:"http_status"`
	EnvelopeSuccess *bool    `json:"envelope_success"`
	DataShape       string   `json:"data_shape"`
	ErrorCodes      []string `json:"error_codes"`
	RetryAfter      string   `json:"retry_after,omitempty"`
}

type Result struct {
	Envelope    Envelope
	Observation Observation
}

type safeTransportError struct {
	cause error
}

func (e safeTransportError) Error() string {
	return "probe HTTP transport failed"
}

func (e safeTransportError) Unwrap() error {
	return e.cause
}

func NewClient(cfg Config, kind TokenKind, timeout time.Duration, httpClient *http.Client) (*Client, error) {
	if err := cfg.ValidateReadOnly(); err != nil {
		return nil, err
	}
	token := cfg.ServiceToken
	if kind == TokenPersonal {
		token = cfg.PersonalToken
	}
	if token == "" {
		return nil, fmt.Errorf("%s access token is not configured", kind)
	}
	if timeout <= 0 {
		timeout = defaultProbeTimeout
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	} else if httpClient.Timeout == 0 {
		httpClient.Timeout = timeout
	}
	baseURL := *cfg.APIURL
	return &Client{baseURL: &baseURL, token: token, httpClient: httpClient}, nil
}

// NewCloudNegativeAuthClient is intentionally restricted to the approved
// public Cloud host and supports only missing/synthetic-malformed auth probes.
func NewCloudNegativeAuthClient(testCase NegativeAuthCase, timeout time.Duration, httpClient *http.Client) (*Client, error) {
	baseURL, err := parseAPIURL("https://" + cloudAPIHost)
	if err != nil {
		return nil, err
	}
	token := ""
	switch testCase {
	case NegativeAuthMissing:
	case NegativeAuthMalformed:
		token = syntheticMalformedToken
	default:
		return nil, errors.New("negative auth case must be missing or malformed")
	}
	if timeout <= 0 {
		timeout = defaultProbeTimeout
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	} else if httpClient.Timeout == 0 {
		httpClient.Timeout = timeout
	}
	return &Client{baseURL: baseURL, token: token, httpClient: httpClient}, nil
}

func (c *Client) DoJSON(ctx context.Context, method, pathTemplate string, body interface{}) (Result, error) {
	return c.doJSON(ctx, method, pathTemplate, "", body)
}

// DoJSONAt separates the concrete request path from the path template retained
// in sanitized observations. Callers use it for resource paths containing IDs;
// the concrete path is never serialized into a report.
func (c *Client) DoJSONAt(
	ctx context.Context,
	method string,
	requestPath string,
	observationPathTemplate string,
	body interface{},
) (Result, error) {
	if err := validateObservationPathTemplate(observationPathTemplate); err != nil {
		return Result{}, err
	}
	return c.doJSON(ctx, method, requestPath, observationPathTemplate, body)
}

func (c *Client) doJSON(
	ctx context.Context,
	method string,
	requestPath string,
	observationPathTemplate string,
	body interface{},
) (Result, error) {
	endpoint, err := c.endpoint(requestPath)
	if err != nil {
		return Result{}, err
	}
	var requestBody io.Reader
	if body != nil {
		content, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return Result{}, errors.New("encode probe request body")
		}
		requestBody = bytes.NewReader(content)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), requestBody)
	if err != nil {
		return Result{}, errors.New("construct probe request")
	}
	if c.token != "" {
		request.Header.Set("Authorization", c.token)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", probeUserAgent)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return Result{}, safeTransportError{cause: err}
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return Result{}, errors.New("read probe response")
	}
	if len(content) > maxResponseBytes {
		return Result{}, errors.New("probe response exceeds size limit")
	}

	if observationPathTemplate == "" {
		observationPathTemplate = endpoint.EscapedPath()
	}
	result := Result{
		Observation: Observation{
			Method:       method,
			PathTemplate: observationPathTemplate,
			HTTPStatus:   response.StatusCode,
			RetryAfter:   RedactText(response.Header.Get("Retry-After")),
		},
	}
	if err := json.Unmarshal(content, &result.Envelope); err != nil {
		result.Observation.DataShape = "unparseable"
		return result, errors.New("decode FeatBit response envelope")
	}
	result.Observation.EnvelopeSuccess = result.Envelope.Success
	result.Observation.DataShape = describeDataShape(result.Envelope.Data)
	result.Observation.ErrorCodes = extractErrorCodes(result.Envelope.Errors)
	return result, nil
}

func validateObservationPathTemplate(pathTemplate string) error {
	reference, err := url.Parse(pathTemplate)
	if err != nil ||
		reference.IsAbs() ||
		reference.Host != "" ||
		reference.User != nil ||
		reference.RawQuery != "" ||
		reference.Fragment != "" {
		return errors.New("observation path template must be a relative documented API path without a query")
	}
	if !strings.HasPrefix(reference.Path, "/api/v1/") && reference.Path != "/api/v1" {
		return errors.New("observation path template must begin with /api/v1")
	}
	if len(pathTemplate) > 256 {
		return errors.New("observation path template is too long")
	}
	return nil
}

func (c *Client) DeleteInventoryEntry(ctx context.Context, entry InventoryEntry) error {
	path, err := cleanupPath(entry)
	if err != nil {
		return err
	}
	pathTemplate, err := cleanupPathTemplate(entry)
	if err != nil {
		return err
	}
	result, requestErr := c.DoJSONAt(
		ctx,
		http.MethodDelete,
		path,
		pathTemplate,
		nil,
	)
	classification := Classify(result.Observation, requestErr)
	deleteSucceeded := requestErr == nil &&
		classification == ClassificationSuccess
	archiveBeforeDelete := ""

	if entry.Type == ResourceFlag &&
		observationHasErrorCode(
			result.Observation,
			"CannotDeleteUnarchivedFeatureFlag",
		) {
		archiveResult, archiveErr := c.DoJSONAt(
			ctx,
			http.MethodPut,
			featureFlagArchivePath(
				entry.Identity.EnvironmentID,
				entry.Identity.Key,
			),
			featureFlagArchiveTemplate,
			resourceChangeRequest{
				Comment: "Terraform provider Phase 0 cleanup prerequisite",
			},
		)
		if archiveErr != nil ||
			Classify(archiveResult.Observation, archiveErr) !=
				ClassificationSuccess {
			return errors.New(
				"feature-flag cleanup archive prerequisite failed",
			)
		}
		archiveBeforeDelete = "feature_flag_archive_before_delete"
		result, requestErr = c.DoJSONAt(
			ctx,
			http.MethodDelete,
			path,
			pathTemplate,
			nil,
		)
		classification = Classify(result.Observation, requestErr)
		deleteSucceeded = requestErr == nil &&
			classification == ClassificationSuccess
	}
	if entry.Type == ResourceSegment &&
		observationHasErrorCode(
			result.Observation,
			"CannotDeleteUnArchivedSegment",
		) {
		archiveResult, archiveErr := c.DoJSONAt(
			ctx,
			http.MethodPut,
			segmentArchivePath(
				entry.Identity.EnvironmentID,
				entry.Identity.ID,
			),
			segmentArchiveTemplate,
			segmentResourceChangeRequest{
				Comment: "Terraform provider Phase 0 cleanup prerequisite",
			},
		)
		if archiveErr != nil ||
			Classify(archiveResult.Observation, archiveErr) !=
				ClassificationSuccess {
			return errors.New(
				"segment cleanup archive prerequisite failed",
			)
		}
		archiveBeforeDelete = "segment_archive_before_delete"
		result, requestErr = c.DoJSONAt(
			ctx,
			http.MethodDelete,
			path,
			pathTemplate,
			nil,
		)
		classification = Classify(result.Observation, requestErr)
		deleteSucceeded = requestErr == nil &&
			classification == ClassificationSuccess
	}

	if exactCleanupVerificationSupported(entry.Type) {
		absent, verifyErr := c.verifyInventoryEntryAbsent(ctx, entry)
		switch {
		case verifyErr == nil && absent:
			if archiveBeforeDelete != "" {
				c.recordCleanupWorkaround(archiveBeforeDelete)
			}
			return nil
		case verifyErr != nil && deleteSucceeded:
			return fmt.Errorf(
				"cleanup %s Delete succeeded but exact absence could not be verified",
				entry.Type,
			)
		case verifyErr != nil:
			return fmt.Errorf(
				"cleanup %s Delete classification %s and exact absence could not be verified",
				entry.Type,
				classification,
			)
		case deleteSucceeded:
			return fmt.Errorf(
				"cleanup %s Delete succeeded but the exact identity remains",
				entry.Type,
			)
		default:
			return fmt.Errorf(
				"cleanup %s Delete classification %s and the exact identity remains",
				entry.Type,
				classification,
			)
		}
	}

	if !deleteSucceeded {
		return fmt.Errorf(
			"cleanup %s failed with classification %s",
			entry.Type,
			classification,
		)
	}
	return nil
}

func (c *Client) recordCleanupWorkaround(value string) {
	c.cleanupMu.Lock()
	defer c.cleanupMu.Unlock()
	c.cleanupWorkarounds = append(c.cleanupWorkarounds, value)
}

func (c *Client) CleanupWorkarounds() []string {
	c.cleanupMu.Lock()
	defer c.cleanupMu.Unlock()
	return append([]string{}, c.cleanupWorkarounds...)
}

func exactCleanupVerificationSupported(resourceType ResourceType) bool {
	switch resourceType {
	case ResourceProject, ResourceEnvironment, ResourceFlag, ResourceSegment:
		return true
	default:
		return false
	}
}

func (c *Client) verifyInventoryEntryAbsent(
	ctx context.Context,
	entry InventoryEntry,
) (bool, error) {
	report := ProjectEnvironmentLifecycleReport{}
	switch entry.Type {
	case ResourceProject:
		projects, err := lifecycleListProjects(ctx, c, &report)
		if err != nil {
			return false, errors.New(
				"project cleanup complete-collection fallback failed",
			)
		}
		count := countProjectsByID(projects, entry.Identity.ID)
		if count > 1 {
			return false, errors.New(
				"project cleanup complete collection returned duplicate exact IDs",
			)
		}
		return count == 0, nil
	case ResourceEnvironment:
		return c.verifyCleanupEnvironmentAbsent(ctx, entry, &report)
	case ResourceFlag:
		match, err := lifecycleFindFeatureFlagByKeyAnyState(
			ctx,
			c,
			&report,
			entry.Identity.EnvironmentID,
			entry.Identity.Key,
		)
		if err == nil {
			if match.Count > 1 {
				return false, errors.New(
					"feature-flag cleanup collection returned duplicate exact keys",
				)
			}
			return match.Count == 0, nil
		}
		return c.verifyCleanupChildByParentAbsence(ctx, entry, &report)
	case ResourceSegment:
		match, err := lifecycleFindSegmentByIDAnyState(
			ctx,
			c,
			&report,
			entry.Identity.EnvironmentID,
			entry.Identity.ID,
		)
		if err == nil {
			if match.Count > 1 {
				return false, errors.New(
					"segment cleanup collection returned duplicate exact IDs",
				)
			}
			return match.Count == 0, nil
		}
		return c.verifyCleanupChildByParentAbsence(ctx, entry, &report)
	default:
		return false, errors.New(
			"resource type has no exact cleanup verification",
		)
	}
}

func (c *Client) verifyCleanupEnvironmentAbsent(
	ctx context.Context,
	entry InventoryEntry,
	report *ProjectEnvironmentLifecycleReport,
) (bool, error) {
	project, projectCount, err := c.cleanupProjectByID(
		ctx,
		entry.Identity.ProjectID,
		report,
	)
	if err != nil {
		return false, err
	}
	if projectCount == 0 {
		return true, nil
	}
	_, environmentCount := environmentByID(
		project.Environments,
		entry.Identity.ID,
	)
	if environmentCount > 1 {
		return false, errors.New(
			"environment cleanup parent returned duplicate exact IDs",
		)
	}
	return environmentCount == 0, nil
}

func (c *Client) verifyCleanupChildByParentAbsence(
	ctx context.Context,
	entry InventoryEntry,
	report *ProjectEnvironmentLifecycleReport,
) (bool, error) {
	if entry.Identity.ProjectID == "" {
		return false, errors.New(
			"child cleanup identity lacks its project for parent fallback",
		)
	}
	project, projectCount, err := c.cleanupProjectByID(
		ctx,
		entry.Identity.ProjectID,
		report,
	)
	if err != nil {
		return false, err
	}
	if projectCount == 0 {
		return true, nil
	}
	_, environmentCount := environmentByID(
		project.Environments,
		entry.Identity.EnvironmentID,
	)
	if environmentCount == 0 {
		return true, nil
	}
	if environmentCount > 1 {
		return false, errors.New(
			"child cleanup parent returned duplicate exact environment IDs",
		)
	}
	return false, errors.New(
		"child exact collection failed while its parent environment remains",
	)
}

func (c *Client) cleanupProjectByID(
	ctx context.Context,
	projectID string,
	report *ProjectEnvironmentLifecycleReport,
) (projectWire, int, error) {
	if projectID == "" {
		return projectWire{}, 0, errors.New(
			"cleanup identity lacks its project",
		)
	}
	projects, err := lifecycleListProjects(ctx, c, report)
	if err != nil {
		return projectWire{}, 0, errors.New(
			"cleanup complete project collection fallback failed",
		)
	}
	var match projectWire
	count := 0
	for _, project := range projects {
		if project.ID == projectID {
			match = project
			count++
		}
	}
	if count > 1 {
		return projectWire{}, count, errors.New(
			"cleanup complete project collection returned duplicate exact IDs",
		)
	}
	return match, count, nil
}

func MarshalObservation(observation Observation) ([]byte, error) {
	content, err := json.MarshalIndent(observation, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(RedactJSON(content), '\n'), nil
}

func (c *Client) endpoint(pathTemplate string) (*url.URL, error) {
	reference, err := url.Parse(pathTemplate)
	if err != nil || reference.IsAbs() || reference.Host != "" || reference.User != nil || reference.Fragment != "" {
		return nil, errors.New("probe path must be a relative documented API path")
	}
	if !strings.HasPrefix(reference.Path, "/api/v1/") && reference.Path != "/api/v1" {
		return nil, errors.New("probe path must begin with /api/v1")
	}
	endpoint := *c.baseURL
	endpoint.Path = reference.Path
	endpoint.RawPath = reference.RawPath
	endpoint.RawQuery = reference.RawQuery
	return &endpoint, nil
}

func describeDataShape(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "missing"
	}
	var value interface{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "unparseable"
	}
	switch typed := value.(type) {
	case nil:
		return "null"
	case []interface{}:
		return fmt.Sprintf("array(items=%d)", len(typed))
	case map[string]interface{}:
		if items, ok := typed["items"].([]interface{}); ok {
			_, totalPresent := typed["totalCount"]
			return fmt.Sprintf("page(items=%d,total_count_present=%t)", len(items), totalPresent)
		}
		return fmt.Sprintf("object(fields=%d)", len(typed))
	default:
		return fmt.Sprintf("%T", value)
	}
}

func extractErrorCodes(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var values []interface{}
	if err := json.Unmarshal(raw, &values); err != nil {
		return []string{"<UNPARSEABLE_ERRORS>"}
	}
	codes := make([]string, 0, len(values))
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			codes = append(codes, safeErrorCode(typed))
		case map[string]interface{}:
			code, _ := typed["code"].(string)
			codes = append(codes, safeErrorCode(code))
		default:
			codes = append(codes, "<REDACTED_ERROR>")
		}
	}
	return codes
}

func safeErrorCode(code string) string {
	if code == "" || len(code) > 64 {
		return "<REDACTED_ERROR>"
	}
	for _, character := range code {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == ':' || character == '_' || character == '-' {
			continue
		}
		return "<REDACTED_ERROR>"
	}
	return RedactText(code)
}

func cleanupPath(entry InventoryEntry) (string, error) {
	escape := url.PathEscape
	switch entry.Type {
	case ResourceProject:
		if entry.Identity.ID == "" {
			break
		}
		return "/api/v1/projects/" + escape(entry.Identity.ID), nil
	case ResourceEnvironment:
		if entry.Identity.ProjectID == "" || entry.Identity.ID == "" {
			break
		}
		return "/api/v1/projects/" + escape(entry.Identity.ProjectID) + "/envs/" + escape(entry.Identity.ID), nil
	case ResourceFlag:
		if entry.Identity.EnvironmentID == "" || entry.Identity.Key == "" {
			break
		}
		return "/api/v1/envs/" + escape(entry.Identity.EnvironmentID) + "/feature-flags/" + escape(entry.Identity.Key), nil
	case ResourceSegment:
		if entry.Identity.EnvironmentID == "" || entry.Identity.ID == "" {
			break
		}
		return "/api/v1/envs/" + escape(entry.Identity.EnvironmentID) + "/segments/" + escape(entry.Identity.ID), nil
	case ResourceGroup:
		if entry.Identity.ID == "" {
			break
		}
		return "/api/v1/groups/" + escape(entry.Identity.ID), nil
	case ResourcePolicy:
		if entry.Identity.ID == "" {
			break
		}
		return "/api/v1/policies/" + escape(entry.Identity.ID), nil
	}
	return "", fmt.Errorf("cleanup identity for %s is incomplete or unsupported", entry.Type)
}

func cleanupPathTemplate(entry InventoryEntry) (string, error) {
	switch entry.Type {
	case ResourceProject:
		return projectItemTemplate, nil
	case ResourceEnvironment:
		return environmentItem, nil
	case ResourceFlag:
		return featureFlagItemTemplate, nil
	case ResourceSegment:
		return segmentItemTemplate, nil
	case ResourceGroup:
		return "/api/v1/groups/{id}", nil
	case ResourcePolicy:
		return "/api/v1/policies/{id}", nil
	default:
		return "", fmt.Errorf(
			"cleanup path template for %s is unsupported",
			entry.Type,
		)
	}
}
