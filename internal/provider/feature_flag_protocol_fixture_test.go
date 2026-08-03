// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/google/uuid"
)

type featureFlagProtocolFixture struct {
	t      *testing.T
	server *httptest.Server

	mu                   sync.Mutex
	active               map[string]*featureFlagFixtureObject
	archived             map[string]*featureFlagFixtureObject
	requests             []featureFlagFixtureRequest
	violations           []string
	nextID               int
	directReadFailure    bool
	directFallbackCount  int
	reverseVariations    bool
	reverseCollections   bool
	protectedUI          map[string]featureFlagFixtureUI
	deletedProtectedUI   map[string]featureFlagFixtureUI
	createCount          int
	nameUpdateCount      int
	archiveMutationCount int
	deleteMutationCount  int
}

type featureFlagFixtureObject struct {
	ID            string
	EnvironmentID string
	Name          string
	Description   string
	Key           string
	VariationType string
	Variations    []featureFlagFixtureVariation
	Archived      bool
	UI            featureFlagFixtureUI
}

type featureFlagFixtureVariation struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type featureFlagFixtureUI struct {
	IsEnabled           bool
	EnabledVariationID  string
	DisabledVariationID string
	Tags                []string
	TargetMarker        string
	RuleMarker          string
	DispatchMarker      string
}

type featureFlagFixtureRequest struct {
	Method string
	Path   string
	Query  string
}

type featureFlagFixtureCreateInput struct {
	Name                string                        `json:"name"`
	Key                 string                        `json:"key"`
	IsEnabled           bool                          `json:"isEnabled"`
	Description         string                        `json:"description"`
	VariationType       string                        `json:"variationType"`
	Variations          []featureFlagFixtureVariation `json:"variations"`
	EnabledVariationID  string                        `json:"enabledVariationId"`
	DisabledVariationID string                        `json:"disabledVariationId"`
	Tags                []string                      `json:"tags"`
}

func newFeatureFlagProtocolFixture(t *testing.T) *featureFlagProtocolFixture {
	t.Helper()
	fixture := &featureFlagProtocolFixture{
		t:                  t,
		active:             make(map[string]*featureFlagFixtureObject),
		archived:           make(map[string]*featureFlagFixtureObject),
		protectedUI:        make(map[string]featureFlagFixtureUI),
		deletedProtectedUI: make(map[string]featureFlagFixtureUI),
	}
	fixture.server = httptest.NewServer(fixture)
	return fixture
}

func (f *featureFlagProtocolFixture) ServeHTTP(
	response http.ResponseWriter,
	request *http.Request,
) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.requests = append(f.requests, featureFlagFixtureRequest{
		Method: request.Method,
		Path:   request.URL.EscapedPath(),
		Query:  request.URL.RawQuery,
	})
	if request.Header.Get("Authorization") != syntheticProviderAccessToken {
		f.recordViolationLocked("request omitted direct access-token authorization")
	}
	for _, header := range []string{
		"Organization", "Workspace", "X-Organization", "X-Organization-Id",
		"X-Workspace", "X-Workspace-Id",
	} {
		if request.Header.Get(header) != "" {
			f.recordViolationLocked("request sent an unsupported context header")
		}
	}
	if request.Method == http.MethodPatch {
		f.recordViolationLocked("request used the forbidden generic PATCH endpoint")
		f.writeEnvelopeLocked(response, http.StatusMethodNotAllowed, nil)
		return
	}

	segments := strings.Split(strings.TrimPrefix(request.URL.Path, "/api/v1/"), "/")
	if len(segments) < 3 || segments[0] != "envs" ||
		segments[2] != "feature-flags" || !client.ValidUUID(segments[1]) {
		f.recordViolationLocked("request left the documented Feature Flag boundary")
		f.writeEnvelopeLocked(response, http.StatusNotFound, nil)
		return
	}
	environmentID := segments[1]

	switch {
	case len(segments) == 3 && request.Method == http.MethodGet:
		f.handleListLocked(response, request, environmentID)
	case len(segments) == 3 && request.Method == http.MethodPost:
		f.handleCreateLocked(response, request, environmentID)
	case len(segments) == 4 && request.Method == http.MethodGet:
		f.handleExactGetLocked(response, environmentID, segments[3])
	case len(segments) == 5 && request.Method == http.MethodPut && segments[4] == "name":
		f.handleNameUpdateLocked(response, request, environmentID, segments[3])
	case len(segments) == 5 && request.Method == http.MethodPut && segments[4] == "archive":
		f.handleArchiveLocked(response, request, environmentID, segments[3])
	case len(segments) == 4 && request.Method == http.MethodDelete:
		f.handleDeleteLocked(response, request, environmentID, segments[3])
	default:
		f.recordViolationLocked("request used a forbidden or unsupported Feature Flag operation")
		f.writeEnvelopeLocked(response, http.StatusNotFound, nil)
	}
}

func (f *featureFlagProtocolFixture) handleListLocked(
	response http.ResponseWriter,
	request *http.Request,
	environmentID string,
) {
	query := request.URL.Query()
	archived, archivedErr := strconv.ParseBool(query.Get("IsArchived"))
	pageIndex, pageErr := strconv.Atoi(query.Get("PageIndex"))
	pageSize, sizeErr := strconv.Atoi(query.Get("PageSize"))
	if archivedErr != nil || pageErr != nil || sizeErr != nil || pageIndex < 0 ||
		pageSize != 100 || len(query) != 3 {
		f.recordViolationLocked("collection request omitted the documented pagination query")
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return
	}

	objects := f.collectionObjectsLocked(environmentID, archived)
	items := make([]any, 0, 1)
	if pageIndex < len(objects) {
		items = append(items, f.collectionDataLocked(objects[pageIndex]))
	}
	f.writeEnvelopeLocked(response, http.StatusOK, map[string]any{
		"totalCount": len(objects),
		"items":      items,
	})
}

func (f *featureFlagProtocolFixture) handleCreateLocked(
	response http.ResponseWriter,
	request *http.Request,
	environmentID string,
) {
	if request.URL.RawQuery != "" || request.Header.Get("Content-Type") != "application/json" {
		f.recordViolationLocked("create request had an unexpected query or content type")
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(request.Body).Decode(&raw); err != nil {
		f.recordViolationLocked("create request body was not valid JSON")
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return
	}
	allowed := map[string]struct{}{
		"name": {}, "key": {}, "isEnabled": {}, "description": {},
		"variationType": {}, "variations": {}, "enabledVariationId": {},
		"disabledVariationId": {}, "tags": {},
	}
	if len(raw) != len(allowed) {
		f.recordViolationLocked("create request did not contain exactly the deterministic seed fields")
	}
	for field := range raw {
		if _, ok := allowed[field]; !ok {
			f.recordViolationLocked("create request contained a UI-owned operation field")
		}
	}
	encoded, _ := json.Marshal(raw)
	var input featureFlagFixtureCreateInput
	if json.Unmarshal(encoded, &input) != nil || !validFeatureFlagKey(input.Key) ||
		!validFeatureFlagName(input.Name) || len(input.Variations) == 0 ||
		input.Tags == nil || len(input.Tags) != 0 || input.IsEnabled {
		f.recordViolationLocked("create request contained an invalid deterministic definition or seed")
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return
	}
	canonicalType, err := canonicalizeFeatureFlagVariationType(input.VariationType)
	if err != nil || input.EnabledVariationID != input.Variations[0].ID ||
		input.DisabledVariationID != input.Variations[0].ID {
		f.recordViolationLocked("create request did not use the disabled-safe first variation seed")
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return
	}
	seenVariationIDs := make(map[string]struct{}, len(input.Variations))
	for index, variation := range input.Variations {
		canonicalID, valid := client.CanonicalUUID(variation.ID)
		canonicalValue, valueErr := canonicalizeFeatureFlagValue(canonicalType, variation.Value)
		if !valid || canonicalValue != variation.Value || valueErr != nil ||
			variation.ID != deterministicFeatureFlagVariationID(environmentID, input.Key, index) {
			f.recordViolationLocked("create request did not send canonical values with deterministic UUIDs")
			f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
			return
		}
		if _, duplicate := seenVariationIDs[canonicalID]; duplicate {
			f.recordViolationLocked("create request repeated a variation UUID")
			f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
			return
		}
		seenVariationIDs[canonicalID] = struct{}{}
	}

	identity := featureFlagFixtureIdentity(environmentID, input.Key)
	if f.active[identity] != nil || f.archived[identity] != nil {
		f.writeEnvelopeLocked(response, http.StatusConflict, nil)
		return
	}
	f.nextID++
	object := &featureFlagFixtureObject{
		ID:            uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("feature-flag-protocol/%d", f.nextID))).String(),
		EnvironmentID: environmentID,
		Name:          input.Name,
		Description:   input.Description,
		Key:           input.Key,
		VariationType: canonicalType,
		Variations:    append([]featureFlagFixtureVariation(nil), input.Variations...),
		UI: featureFlagFixtureUI{
			IsEnabled:           input.IsEnabled,
			EnabledVariationID:  input.EnabledVariationID,
			DisabledVariationID: input.DisabledVariationID,
			Tags:                append([]string(nil), input.Tags...),
		},
	}
	f.active[identity] = object
	f.createCount++
	f.writeEnvelopeLocked(response, http.StatusOK, f.exactDataLocked(object))
}

func (f *featureFlagProtocolFixture) handleExactGetLocked(
	response http.ResponseWriter,
	environmentID string,
	key string,
) {
	if f.directReadFailure {
		f.directFallbackCount++
		f.writeEnvelopeLocked(response, http.StatusServiceUnavailable, nil)
		return
	}
	identity := featureFlagFixtureIdentity(environmentID, key)
	object := f.active[identity]
	if object == nil {
		object = f.archived[identity]
	}
	if object == nil {
		f.writeEnvelopeLocked(response, http.StatusNotFound, nil)
		return
	}
	f.writeEnvelopeLocked(response, http.StatusOK, f.exactDataLocked(object))
}

func (f *featureFlagProtocolFixture) handleNameUpdateLocked(
	response http.ResponseWriter,
	request *http.Request,
	environmentID string,
	key string,
) {
	var raw map[string]json.RawMessage
	if request.URL.RawQuery != "" || request.Header.Get("Content-Type") != "application/json" ||
		json.NewDecoder(request.Body).Decode(&raw) != nil || len(raw) != 1 {
		f.recordViolationLocked("name update did not contain exactly one JSON field")
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return
	}
	var name string
	if json.Unmarshal(raw["name"], &name) != nil || !validFeatureFlagName(name) {
		f.recordViolationLocked("name update omitted the valid owned name")
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return
	}
	object := f.active[featureFlagFixtureIdentity(environmentID, key)]
	if object == nil {
		f.writeEnvelopeLocked(response, http.StatusNotFound, nil)
		return
	}
	before := cloneFeatureFlagFixtureUI(object.UI)
	object.Name = name
	if !reflect.DeepEqual(before, object.UI) {
		f.recordViolationLocked("name update changed UI-owned operational state")
	}
	f.nameUpdateCount++
	f.writeEnvelopeLocked(response, http.StatusOK, object.ID)
}

func (f *featureFlagProtocolFixture) handleArchiveLocked(
	response http.ResponseWriter,
	request *http.Request,
	environmentID string,
	key string,
) {
	if request.URL.RawQuery != "" || request.Body != nil && request.Body != http.NoBody ||
		request.Header.Get("Content-Type") != "" {
		f.recordViolationLocked("archive request sent an optional comment or another body")
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return
	}
	identity := featureFlagFixtureIdentity(environmentID, key)
	object := f.active[identity]
	if object == nil {
		f.writeEnvelopeLocked(response, http.StatusConflict, nil)
		return
	}
	f.assertProtectedUILocked(object)
	delete(f.active, identity)
	object.Archived = true
	f.archived[identity] = object
	f.archiveMutationCount++
	f.writeEnvelopeLocked(response, http.StatusOK, true)
}

func (f *featureFlagProtocolFixture) handleDeleteLocked(
	response http.ResponseWriter,
	request *http.Request,
	environmentID string,
	key string,
) {
	if request.URL.RawQuery != "" || request.Body != nil && request.Body != http.NoBody ||
		request.Header.Get("Content-Type") != "" {
		f.recordViolationLocked("permanent delete sent an optional comment or another body")
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return
	}
	identity := featureFlagFixtureIdentity(environmentID, key)
	object := f.archived[identity]
	if object == nil {
		f.recordViolationLocked("permanent delete ran before the archive prerequisite")
		f.writeEnvelopeLocked(response, http.StatusConflict, nil)
		return
	}
	f.assertProtectedUILocked(object)
	if protected, exists := f.protectedUI[object.ID]; exists {
		f.deletedProtectedUI[object.ID] = cloneFeatureFlagFixtureUI(protected)
	}
	delete(f.archived, identity)
	f.deleteMutationCount++
	f.writeEnvelopeLocked(response, http.StatusOK, true)
}

func (f *featureFlagProtocolFixture) collectionObjectsLocked(
	environmentID string,
	archived bool,
) []*featureFlagFixtureObject {
	objects := []*featureFlagFixtureObject{
		f.fuzzyObjectLocked(environmentID, archived, 0),
		f.fuzzyObjectLocked(environmentID, archived, 1),
	}
	source := f.active
	if archived {
		source = f.archived
	}
	keys := make([]string, 0)
	for _, object := range source {
		if client.EqualUUID(object.EnvironmentID, environmentID) {
			keys = append(keys, object.Key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		objects = append(objects, source[featureFlagFixtureIdentity(environmentID, key)])
	}
	if f.reverseCollections {
		for left, right := 0, len(objects)-1; left < right; left, right = left+1, right-1 {
			objects[left], objects[right] = objects[right], objects[left]
		}
	}
	return objects
}

func (f *featureFlagProtocolFixture) fuzzyObjectLocked(
	environmentID string,
	archived bool,
	index int,
) *featureFlagFixtureObject {
	view := "active"
	if archived {
		view = "archived"
	}
	key := fmt.Sprintf("fixture-%s-fuzzy-%d", view, index)
	return &featureFlagFixtureObject{
		ID: uuid.NewSHA1(
			uuid.NameSpaceURL,
			[]byte("feature-flag-protocol-fuzzy/"+environmentID+"/"+view+"/"+strconv.Itoa(index)),
		).String(),
		EnvironmentID: environmentID,
		Name:          "Fixture fuzzy",
		Description:   "",
		Key:           key,
		VariationType: featureFlagVariationTypeString,
		Variations: []featureFlagFixtureVariation{
			{
				ID:    uuid.NewSHA1(uuid.NameSpaceURL, []byte("feature-flag-protocol-fuzzy-variation/"+key)).String(),
				Name:  "Fuzzy",
				Value: "fuzzy",
			},
		},
		Archived: archived,
	}
}

func (f *featureFlagProtocolFixture) collectionDataLocked(
	object *featureFlagFixtureObject,
) map[string]any {
	data := f.definitionDataLocked(object)
	delete(data, "envId")
	delete(data, "isArchived")
	return data
}

func (f *featureFlagProtocolFixture) exactDataLocked(
	object *featureFlagFixtureObject,
) map[string]any {
	return f.definitionDataLocked(object)
}

func (f *featureFlagProtocolFixture) definitionDataLocked(
	object *featureFlagFixtureObject,
) map[string]any {
	variations := append([]featureFlagFixtureVariation(nil), object.Variations...)
	if f.reverseVariations {
		for left, right := 0, len(variations)-1; left < right; left, right = left+1, right-1 {
			variations[left], variations[right] = variations[right], variations[left]
		}
	}
	return map[string]any{
		"id": object.ID, "envId": object.EnvironmentID, "name": object.Name,
		"description": object.Description, "key": object.Key,
		"variationType": object.VariationType, "variations": variations,
		"isArchived": object.Archived, "isEnabled": object.UI.IsEnabled,
		"enabledVariationId":  object.UI.EnabledVariationID,
		"disabledVariationId": object.UI.DisabledVariationID,
		"tags":                append([]string(nil), object.UI.Tags...),
		"targetUsers": []map[string]any{
			{"variationId": object.UI.EnabledVariationID, "keyIds": []string{object.UI.TargetMarker}},
		},
		"rules": []map[string]any{{"name": object.UI.RuleMarker}},
		"fallthrough": map[string]any{
			"dispatchKey": object.UI.DispatchMarker,
			"variations":  []map[string]any{{"id": object.UI.EnabledVariationID, "rollout": []float64{0, 1}}},
		},
	}
}

func (f *featureFlagProtocolFixture) writeEnvelopeLocked(
	response http.ResponseWriter,
	status int,
	data any,
) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	success := status >= http.StatusOK && status < http.StatusMultipleChoices
	if err := json.NewEncoder(response).Encode(map[string]any{
		"success": success,
		"data":    data,
		"errors":  []string{},
	}); err != nil {
		f.t.Errorf("write Feature Flag fixture response: %v", err)
	}
}

func (f *featureFlagProtocolFixture) recordViolationLocked(message string) {
	f.violations = append(f.violations, message)
}

func (f *featureFlagProtocolFixture) assertProtectedUILocked(object *featureFlagFixtureObject) {
	protected, exists := f.protectedUI[object.ID]
	if exists && !reflect.DeepEqual(protected, object.UI) {
		f.recordViolationLocked("provider mutation rewrote protected UI-owned state")
	}
}

func (f *featureFlagProtocolFixture) apiOrigin() string {
	return f.server.URL
}

func (f *featureFlagProtocolFixture) close() {
	f.server.Close()
}

func (f *featureFlagProtocolFixture) objectCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.active) + len(f.archived)
}

func (f *featureFlagProtocolFixture) currentID(environmentID string, key string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	identity := featureFlagFixtureIdentity(environmentID, key)
	object := f.active[identity]
	if object == nil {
		object = f.archived[identity]
	}
	if object == nil {
		return "", false
	}
	return object.ID, true
}

func (f *featureFlagProtocolFixture) currentDefinition(
	environmentID string,
	key string,
) (string, string, string, int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	object := f.active[featureFlagFixtureIdentity(environmentID, key)]
	if object == nil {
		return "", "", "", 0, false
	}
	return object.Name, object.Description, object.VariationType, len(object.Variations), true
}

func (f *featureFlagProtocolFixture) rename(environmentID string, key string, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	object := f.active[featureFlagFixtureIdentity(environmentID, key)]
	if object == nil {
		return fmt.Errorf("active Feature Flag is absent")
	}
	object.Name = name
	return nil
}

func (f *featureFlagProtocolFixture) protectCustomUI(environmentID string, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	object := f.active[featureFlagFixtureIdentity(environmentID, key)]
	if object == nil {
		return "", fmt.Errorf("active Feature Flag is absent")
	}
	object.UI = featureFlagFixtureUI{
		IsEnabled:           true,
		EnabledVariationID:  object.Variations[len(object.Variations)-1].ID,
		DisabledVariationID: object.Variations[0].ID,
		Tags:                []string{"synthetic-ui-tag", "synthetic-ui-owner"},
		TargetMarker:        "synthetic-ui-target",
		RuleMarker:          "synthetic-ui-rule",
		DispatchMarker:      "synthetic-ui-dispatch",
	}
	f.protectedUI[object.ID] = cloneFeatureFlagFixtureUI(object.UI)
	return object.ID, nil
}

func (f *featureFlagProtocolFixture) uiPreserved(featureFlagID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	want, protected := f.protectedUI[featureFlagID]
	if !protected {
		return false
	}
	for _, object := range f.active {
		if object.ID == featureFlagID {
			return reflect.DeepEqual(want, object.UI)
		}
	}
	for _, object := range f.archived {
		if object.ID == featureFlagID {
			return reflect.DeepEqual(want, object.UI)
		}
	}
	deleted, exists := f.deletedProtectedUI[featureFlagID]
	return exists && reflect.DeepEqual(want, deleted)
}

func (f *featureFlagProtocolFixture) setDirectReadFailure(enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.directReadFailure = enabled
}

func (f *featureFlagProtocolFixture) directFallbacks() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.directFallbackCount
}

func (f *featureFlagProtocolFixture) setReverseVariations(enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reverseVariations = enabled
}

func (f *featureFlagProtocolFixture) setReverseCollections(enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reverseCollections = enabled
}

func (f *featureFlagProtocolFixture) removeActive(environmentID string, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	identity := featureFlagFixtureIdentity(environmentID, key)
	if f.active[identity] == nil {
		return fmt.Errorf("active Feature Flag is absent")
	}
	delete(f.active, identity)
	return nil
}

func (f *featureFlagProtocolFixture) archiveExternal(environmentID string, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	identity := featureFlagFixtureIdentity(environmentID, key)
	object := f.active[identity]
	if object == nil {
		return fmt.Errorf("active Feature Flag is absent")
	}
	delete(f.active, identity)
	object.Archived = true
	f.archived[identity] = object
	return nil
}

func (f *featureFlagProtocolFixture) restoreExternal(environmentID string, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	identity := featureFlagFixtureIdentity(environmentID, key)
	object := f.archived[identity]
	if object == nil {
		return fmt.Errorf("archived Feature Flag is absent")
	}
	delete(f.archived, identity)
	object.Archived = false
	f.active[identity] = object
	return nil
}

func (f *featureFlagProtocolFixture) mutationCounts() (int, int, int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.createCount, f.nameUpdateCount, f.archiveMutationCount, f.deleteMutationCount
}

func (f *featureFlagProtocolFixture) violationSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.violations...)
}

func (f *featureFlagProtocolFixture) requestSnapshot() []featureFlagFixtureRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]featureFlagFixtureRequest(nil), f.requests...)
}

func featureFlagFixtureIdentity(environmentID string, key string) string {
	canonicalEnvironmentID, valid := client.CanonicalUUID(environmentID)
	if !valid {
		canonicalEnvironmentID = environmentID
	}
	return canonicalEnvironmentID + "\x00" + key
}

func cloneFeatureFlagFixtureUI(value featureFlagFixtureUI) featureFlagFixtureUI {
	value.Tags = append([]string(nil), value.Tags...)
	return value
}
