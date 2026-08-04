// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/google/uuid"
)

const (
	segmentProtocolMutationCreate      = "create"
	segmentProtocolMutationName        = "name"
	segmentProtocolMutationDescription = "description"
	segmentProtocolMutationTargeting   = "targeting"
	segmentProtocolMutationTags        = "tags"
	segmentProtocolMutationArchive     = "archive"
	segmentProtocolMutationDelete      = "delete"
)

type segmentProtocolFixture struct {
	t      *testing.T
	server *httptest.Server

	mu                    sync.Mutex
	active                map[string]*segmentProtocolObject
	archived              map[string]*segmentProtocolObject
	shared                *segmentProtocolObject
	references            map[string][]client.SegmentFlagReference
	requests              []segmentProtocolRequest
	mutations             []string
	violations            []string
	nextID                int
	reverseSets           bool
	reverseCollections    bool
	directFailureID       string
	directFailure         bool
	directFailureCount    int
	ambiguousNextMutation string
	lastDeletedID         string
	lastDeletedEnv        string
	teardownActiveProof   bool
	teardownArchiveProof  bool
}

type segmentProtocolObject struct {
	ID            string
	EnvironmentID string
	Name          string
	Key           string
	Description   string
	Type          client.SegmentType
	Scopes        []string
	Included      []string
	Excluded      []string
	Rules         []client.SegmentRule
	Tags          []string
	Archived      bool
	Mutable       bool
}

type segmentProtocolRequest struct {
	Method string
	Path   string
	Query  string
}

func newSegmentProtocolFixture(t *testing.T) *segmentProtocolFixture {
	t.Helper()
	sharedID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("segment-protocol/shared")).String()
	sharedRuleID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("segment-protocol/shared-rule")).String()
	sharedConditionID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("segment-protocol/shared-condition")).String()
	fixture := &segmentProtocolFixture{
		t:          t,
		active:     make(map[string]*segmentProtocolObject),
		archived:   make(map[string]*segmentProtocolObject),
		references: make(map[string][]client.SegmentFlagReference),
		shared: &segmentProtocolObject{
			ID:            sharedID,
			EnvironmentID: providerEnvironmentA,
			Name:          "Synthetic Shared Segment",
			Key:           "protocol-shared-segment",
			Description:   "Synthetic shared observation",
			Type:          client.SegmentTypeShared,
			Scopes: []string{
				providerSegmentProjectScope,
				providerSegmentOrganizationScope,
			},
			Included: []string{"shared-user-z", "shared-user-a"},
			Excluded: []string{"shared-excluded-z", "shared-excluded-a"},
			Rules: []client.SegmentRule{
				{
					ID:   sharedRuleID,
					Name: "Shared Rule",
					Conditions: []client.SegmentCondition{
						{
							ID:       sharedConditionID,
							Property: "shared-property",
							Operator: segmentOperatorEqual,
							Value:    "shared-value",
						},
					},
				},
			},
			Tags: []string{"shared-tag-z", "shared-tag-a"},
		},
	}
	fixture.server = httptest.NewServer(fixture)
	return fixture
}

func (f *segmentProtocolFixture) ServeHTTP(
	response http.ResponseWriter,
	request *http.Request,
) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.requests = append(f.requests, segmentProtocolRequest{
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
	if len(segments) < 3 || segments[0] != "envs" || segments[2] != "segments" ||
		!client.ValidUUID(segments[1]) {
		f.recordViolationLocked("request left the documented Segment boundary")
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
		f.handleExactGetLocked(response, request, environmentID, segments[3])
	case len(segments) == 4 && request.Method == http.MethodDelete:
		f.handleDeleteLocked(response, request, environmentID, segments[3])
	case len(segments) == 5 && request.Method == http.MethodGet &&
		segments[4] == "flag-references":
		f.handleReferencesLocked(response, request, environmentID, segments[3])
	case len(segments) == 5 && request.Method == http.MethodPut &&
		segments[4] == "archive":
		f.handleArchiveLocked(response, request, environmentID, segments[3])
	case len(segments) == 5 && request.Method == http.MethodPut &&
		segments[4] == "name":
		f.handleNameLocked(response, request, environmentID, segments[3])
	case len(segments) == 5 && request.Method == http.MethodPut &&
		segments[4] == "description":
		f.handleDescriptionLocked(response, request, environmentID, segments[3])
	case len(segments) == 5 && request.Method == http.MethodPut &&
		segments[4] == "targeting":
		f.handleTargetingLocked(response, request, environmentID, segments[3])
	case len(segments) == 5 && request.Method == http.MethodPut &&
		segments[4] == "tags":
		f.handleTagsLocked(response, request, environmentID, segments[3])
	default:
		f.recordViolationLocked("request used a forbidden or unsupported Segment operation")
		f.writeEnvelopeLocked(response, http.StatusNotFound, nil)
	}
}

func (f *segmentProtocolFixture) handleListLocked(
	response http.ResponseWriter,
	request *http.Request,
	environmentID string,
) {
	query := request.URL.Query()
	archived, archivedErr := strconv.ParseBool(query.Get("IsArchived"))
	pageIndex, pageErr := strconv.Atoi(query.Get("PageIndex"))
	pageSize, sizeErr := strconv.Atoi(query.Get("PageSize"))
	_, hasName := query["Name"]
	if archivedErr != nil || pageErr != nil || sizeErr != nil || pageIndex < 0 ||
		pageSize != 100 || !hasName || query.Get("Name") != "" || len(query) != 4 {
		f.recordViolationLocked("collection request omitted the documented exact pagination query")
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return
	}

	objects := f.collectionObjectsLocked(environmentID, archived)
	items := make([]any, 0, 1)
	if pageIndex < len(objects) {
		items = append(items, f.collectionDataLocked(objects[pageIndex]))
	}
	if len(objects) > 0 && pageIndex == len(objects)-1 &&
		f.lastDeletedID != "" && client.EqualUUID(environmentID, f.lastDeletedEnv) &&
		!f.collectionContainsIDLocked(objects, f.lastDeletedID) {
		if archived {
			f.teardownArchiveProof = true
		} else {
			f.teardownActiveProof = true
		}
	}
	f.writeEnvelopeLocked(response, http.StatusOK, map[string]any{
		"totalCount": len(objects),
		"items":      items,
	})
}

func (f *segmentProtocolFixture) handleCreateLocked(
	response http.ResponseWriter,
	request *http.Request,
	environmentID string,
) {
	var input client.CreateSegmentRequest
	if !f.decodeExactJSONLocked(response, request, []string{
		"type", "name", "key", "description", "scopes",
	}, &input) {
		return
	}
	if input.Type != client.SegmentTypeEnvironmentSpecific ||
		!validSegmentName(input.Name) || !validSegmentKey(input.Key) ||
		!validEnvironmentSpecificScopes(input.Scopes) {
		f.recordViolationLocked("create request contained an unsafe Segment definition")
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return
	}
	if f.findByKeyLocked(environmentID, input.Key, false) != nil ||
		f.findByKeyLocked(environmentID, input.Key, true) != nil {
		f.writeEnvelopeLocked(response, http.StatusConflict, nil)
		return
	}

	f.nextID++
	object := &segmentProtocolObject{
		ID: uuid.NewSHA1(
			uuid.NameSpaceURL,
			[]byte(fmt.Sprintf("segment-protocol/mutable/%d", f.nextID)),
		).String(),
		EnvironmentID: environmentID,
		Name:          input.Name,
		Key:           input.Key,
		Description:   input.Description,
		Type:          input.Type,
		Scopes:        append([]string{}, input.Scopes...),
		Included:      []string{},
		Excluded:      []string{},
		Rules:         []client.SegmentRule{},
		Tags:          []string{},
		Mutable:       true,
	}
	f.active[segmentProtocolObjectIdentity(environmentID, object.ID)] = object
	f.teardownActiveProof = false
	f.teardownArchiveProof = false
	f.mutations = append(f.mutations, segmentProtocolMutationCreate)
	f.writeEnvelopeLocked(response, http.StatusOK, f.exactDataLocked(object))
}

func (f *segmentProtocolFixture) handleExactGetLocked(
	response http.ResponseWriter,
	request *http.Request,
	environmentID string,
	segmentID string,
) {
	if request.URL.RawQuery != "" || !client.ValidUUID(segmentID) {
		f.recordViolationLocked("exact read used an invalid UUID or query")
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return
	}
	if f.directFailure && client.EqualUUID(segmentID, f.directFailureID) {
		f.directFailureCount++
		f.writeEnvelopeLocked(response, http.StatusServiceUnavailable, nil)
		return
	}
	object := f.findByIDLocked(environmentID, segmentID)
	if object == nil {
		f.writeEnvelopeLocked(response, http.StatusNotFound, nil)
		return
	}
	f.writeEnvelopeLocked(response, http.StatusOK, f.exactDataLocked(object))
}

func (f *segmentProtocolFixture) handleNameLocked(
	response http.ResponseWriter,
	request *http.Request,
	environmentID string,
	segmentID string,
) {
	var input client.UpdateSegmentNameRequest
	if !f.decodeExactJSONLocked(response, request, []string{"name"}, &input) {
		return
	}
	object := f.mutableActiveLocked(environmentID, segmentID)
	if object == nil || !validSegmentName(input.Name) {
		f.writeEnvelopeLocked(response, http.StatusConflict, nil)
		return
	}
	object.Name = input.Name
	f.finishBooleanMutationLocked(response, segmentProtocolMutationName)
}

func (f *segmentProtocolFixture) handleDescriptionLocked(
	response http.ResponseWriter,
	request *http.Request,
	environmentID string,
	segmentID string,
) {
	var input client.UpdateSegmentDescriptionRequest
	if !f.decodeExactJSONLocked(response, request, []string{"description"}, &input) {
		return
	}
	object := f.mutableActiveLocked(environmentID, segmentID)
	if object == nil {
		f.writeEnvelopeLocked(response, http.StatusConflict, nil)
		return
	}
	object.Description = input.Description
	f.finishBooleanMutationLocked(response, segmentProtocolMutationDescription)
}

func (f *segmentProtocolFixture) handleTargetingLocked(
	response http.ResponseWriter,
	request *http.Request,
	environmentID string,
	segmentID string,
) {
	var input client.UpdateSegmentTargetingRequest
	if !f.decodeExactJSONLocked(response, request, []string{
		"included", "excluded", "rules",
	}, &input) {
		return
	}
	object := f.mutableActiveLocked(environmentID, segmentID)
	if object == nil || input.Included == nil || input.Excluded == nil || input.Rules == nil {
		f.writeEnvelopeLocked(response, http.StatusConflict, nil)
		return
	}
	candidate := cloneSegmentProtocolObject(object)
	candidate.Included = append([]string{}, input.Included...)
	candidate.Excluded = append([]string{}, input.Excluded...)
	candidate.Rules = cloneSegmentProtocolRules(input.Rules)
	if _, err := canonicalizeRemoteSegment(candidate.clientSegment()); err != nil {
		f.recordViolationLocked("targeting update contained an invalid canonical definition")
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return
	}
	object.Included = candidate.Included
	object.Excluded = candidate.Excluded
	object.Rules = candidate.Rules
	f.finishBooleanMutationLocked(response, segmentProtocolMutationTargeting)
}

func (f *segmentProtocolFixture) handleTagsLocked(
	response http.ResponseWriter,
	request *http.Request,
	environmentID string,
	segmentID string,
) {
	var input client.UpdateSegmentTagsRequest
	if !f.decodeExactJSONLocked(response, request, []string{"tags"}, &input) {
		return
	}
	object := f.mutableActiveLocked(environmentID, segmentID)
	if object == nil || input.Tags == nil {
		f.writeEnvelopeLocked(response, http.StatusConflict, nil)
		return
	}
	object.Tags = append([]string{}, input.Tags...)
	f.finishBooleanMutationLocked(response, segmentProtocolMutationTags)
}

func (f *segmentProtocolFixture) handleReferencesLocked(
	response http.ResponseWriter,
	request *http.Request,
	environmentID string,
	segmentID string,
) {
	if !f.validBodylessLocked(request) || f.findByIDLocked(environmentID, segmentID) == nil {
		f.writeEnvelopeLocked(response, http.StatusNotFound, nil)
		return
	}
	references := append([]client.SegmentFlagReference{}, f.references[segmentID]...)
	f.writeEnvelopeLocked(response, http.StatusOK, references)
}

func (f *segmentProtocolFixture) handleArchiveLocked(
	response http.ResponseWriter,
	request *http.Request,
	environmentID string,
	segmentID string,
) {
	if !f.validBodylessLocked(request) {
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return
	}
	identity := segmentProtocolObjectIdentity(environmentID, segmentID)
	object := f.active[identity]
	if object == nil || !object.Mutable || object.Type != client.SegmentTypeEnvironmentSpecific ||
		len(f.references[object.ID]) != 0 {
		f.writeEnvelopeLocked(response, http.StatusConflict, nil)
		return
	}
	delete(f.active, identity)
	object.Archived = true
	f.archived[identity] = object
	f.finishBooleanMutationLocked(response, segmentProtocolMutationArchive)
}

func (f *segmentProtocolFixture) handleDeleteLocked(
	response http.ResponseWriter,
	request *http.Request,
	environmentID string,
	segmentID string,
) {
	if !f.validBodylessLocked(request) {
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return
	}
	identity := segmentProtocolObjectIdentity(environmentID, segmentID)
	object := f.archived[identity]
	if object == nil || !object.Mutable || object.Type != client.SegmentTypeEnvironmentSpecific {
		f.writeEnvelopeLocked(response, http.StatusConflict, nil)
		return
	}
	delete(f.archived, identity)
	delete(f.references, object.ID)
	f.lastDeletedID = object.ID
	f.lastDeletedEnv = object.EnvironmentID
	f.teardownActiveProof = false
	f.teardownArchiveProof = false
	f.finishBooleanMutationLocked(response, segmentProtocolMutationDelete)
}

func (f *segmentProtocolFixture) decodeExactJSONLocked(
	response http.ResponseWriter,
	request *http.Request,
	fields []string,
	destination any,
) bool {
	if request.URL.RawQuery != "" || request.Header.Get("Content-Type") != "application/json" {
		f.recordViolationLocked("JSON mutation used an unexpected query or content type")
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return false
	}
	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(request.Body)
	if err := decoder.Decode(&raw); err != nil {
		f.recordViolationLocked("JSON mutation body was malformed")
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		f.recordViolationLocked("JSON mutation body contained trailing data")
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return false
	}
	allowed := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		allowed[field] = struct{}{}
	}
	if len(raw) != len(allowed) {
		f.recordViolationLocked("JSON mutation did not contain exactly the owned fields")
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return false
	}
	for field := range raw {
		if _, ok := allowed[field]; !ok {
			f.recordViolationLocked("JSON mutation contained an unsupported field")
			f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
			return false
		}
	}
	encoded, _ := json.Marshal(raw)
	if err := json.Unmarshal(encoded, destination); err != nil {
		f.recordViolationLocked("JSON mutation could not be decoded")
		f.writeEnvelopeLocked(response, http.StatusBadRequest, nil)
		return false
	}
	return true
}

func (f *segmentProtocolFixture) validBodylessLocked(request *http.Request) bool {
	valid := request.URL.RawQuery == "" && request.Header.Get("Content-Type") == "" &&
		(request.Body == nil || request.Body == http.NoBody)
	if !valid {
		f.recordViolationLocked("bodyless Segment operation sent a query, content type, or body")
	}
	return valid
}

func (f *segmentProtocolFixture) finishBooleanMutationLocked(
	response http.ResponseWriter,
	operation string,
) {
	f.mutations = append(f.mutations, operation)
	if f.ambiguousNextMutation == operation {
		f.ambiguousNextMutation = ""
		f.writeEnvelopeLocked(response, http.StatusServiceUnavailable, nil)
		return
	}
	f.writeEnvelopeLocked(response, http.StatusOK, true)
}

func (f *segmentProtocolFixture) collectionObjectsLocked(
	environmentID string,
	archived bool,
) []*segmentProtocolObject {
	objects := []*segmentProtocolObject{
		f.fuzzyObjectLocked(environmentID, archived, 0),
		f.fuzzyObjectLocked(environmentID, archived, 1),
	}
	source := f.active
	if archived {
		source = f.archived
	}
	mutable := make([]*segmentProtocolObject, 0, len(source))
	for _, object := range source {
		if client.EqualUUID(object.EnvironmentID, environmentID) {
			mutable = append(mutable, object)
		}
	}
	sort.Slice(mutable, func(left, right int) bool {
		if mutable[left].Key == mutable[right].Key {
			return mutable[left].ID < mutable[right].ID
		}
		return mutable[left].Key < mutable[right].Key
	})
	objects = append(objects, mutable...)
	if !archived && client.EqualUUID(environmentID, f.shared.EnvironmentID) {
		objects = append(objects, f.shared)
	}
	if f.reverseCollections {
		for left, right := 0, len(objects)-1; left < right; left, right = left+1, right-1 {
			objects[left], objects[right] = objects[right], objects[left]
		}
	}
	return objects
}

func (f *segmentProtocolFixture) fuzzyObjectLocked(
	environmentID string,
	archived bool,
	index int,
) *segmentProtocolObject {
	view := "active"
	if archived {
		view = "archived"
	}
	return &segmentProtocolObject{
		ID: uuid.NewSHA1(
			uuid.NameSpaceURL,
			[]byte(fmt.Sprintf("segment-protocol/fuzzy/%s/%s/%d", environmentID, view, index)),
		).String(),
		EnvironmentID: environmentID,
		Name:          "Synthetic fuzzy Segment",
		Key:           fmt.Sprintf("protocol-fuzzy-%s-%d", view, index),
		Description:   "",
		Type:          client.SegmentTypeEnvironmentSpecific,
		Scopes: []string{
			"organization/fixture:project/fixture:env/" + environmentID,
		},
		Included: []string{},
		Excluded: []string{},
		Rules:    []client.SegmentRule{},
		Tags:     []string{},
		Archived: archived,
	}
}

func (f *segmentProtocolFixture) collectionDataLocked(
	object *segmentProtocolObject,
) map[string]any {
	return map[string]any{
		"id":                    object.ID,
		"envId":                 object.EnvironmentID,
		"key":                   object.Key,
		"type":                  string(object.Type),
		"scopes":                f.stringSetDataLocked(object.Scopes),
		"isArchived":            object.Archived,
		"isEnvironmentSpecific": object.Type == client.SegmentTypeEnvironmentSpecific,
	}
}

func (f *segmentProtocolFixture) exactDataLocked(
	object *segmentProtocolObject,
) map[string]any {
	rules := make([]map[string]any, 0, len(object.Rules))
	for _, rule := range object.Rules {
		conditions := make([]map[string]any, 0, len(rule.Conditions))
		for _, condition := range rule.Conditions {
			conditions = append(conditions, map[string]any{
				"id":       condition.ID,
				"property": condition.Property,
				"op":       condition.Operator,
				"value":    condition.Value,
			})
		}
		rules = append(rules, map[string]any{
			"id": rule.ID, "name": rule.Name, "conditions": conditions,
		})
	}
	return map[string]any{
		"id": object.ID, "envId": object.EnvironmentID,
		"name": object.Name, "key": object.Key,
		"description": object.Description, "type": string(object.Type),
		"scopes":                f.stringSetDataLocked(object.Scopes),
		"included":              f.stringSetDataLocked(object.Included),
		"excluded":              f.stringSetDataLocked(object.Excluded),
		"rules":                 rules,
		"tags":                  f.stringSetDataLocked(object.Tags),
		"isArchived":            object.Archived,
		"isEnvironmentSpecific": object.Type == client.SegmentTypeEnvironmentSpecific,
	}
}

func (f *segmentProtocolFixture) stringSetDataLocked(values []string) []string {
	result := append([]string{}, values...)
	if f.reverseSets {
		for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
			result[left], result[right] = result[right], result[left]
		}
	}
	return result
}

func (f *segmentProtocolFixture) findByIDLocked(
	environmentID string,
	segmentID string,
) *segmentProtocolObject {
	identity := segmentProtocolObjectIdentity(environmentID, segmentID)
	if object := f.active[identity]; object != nil {
		return object
	}
	if object := f.archived[identity]; object != nil {
		return object
	}
	if client.EqualUUID(environmentID, f.shared.EnvironmentID) &&
		client.EqualUUID(segmentID, f.shared.ID) {
		return f.shared
	}
	return nil
}

func (f *segmentProtocolFixture) findByKeyLocked(
	environmentID string,
	key string,
	archived bool,
) *segmentProtocolObject {
	source := f.active
	if archived {
		source = f.archived
	}
	for _, object := range source {
		if client.EqualUUID(object.EnvironmentID, environmentID) && object.Key == key {
			return object
		}
	}
	if !archived && client.EqualUUID(environmentID, f.shared.EnvironmentID) &&
		f.shared.Key == key {
		return f.shared
	}
	return nil
}

func (f *segmentProtocolFixture) mutableActiveLocked(
	environmentID string,
	segmentID string,
) *segmentProtocolObject {
	object := f.active[segmentProtocolObjectIdentity(environmentID, segmentID)]
	if object == nil || !object.Mutable || object.Archived ||
		object.Type != client.SegmentTypeEnvironmentSpecific {
		return nil
	}
	return object
}

func (f *segmentProtocolFixture) collectionContainsIDLocked(
	objects []*segmentProtocolObject,
	segmentID string,
) bool {
	for _, object := range objects {
		if client.EqualUUID(object.ID, segmentID) {
			return true
		}
	}
	return false
}

func (f *segmentProtocolFixture) writeEnvelopeLocked(
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
		f.t.Errorf("write Segment protocol fixture response: %v", err)
	}
}

func (f *segmentProtocolFixture) recordViolationLocked(message string) {
	f.violations = append(f.violations, message)
}

func (f *segmentProtocolFixture) apiOrigin() string {
	return f.server.URL
}

func (f *segmentProtocolFixture) close() {
	f.server.Close()
}

func (f *segmentProtocolFixture) sharedID() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.shared.ID
}

func (f *segmentProtocolFixture) sharedKey() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.shared.Key
}

func (f *segmentProtocolFixture) setReverseSets(enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reverseSets = enabled
}

func (f *segmentProtocolFixture) setReverseCollections(enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reverseCollections = enabled
}

func (f *segmentProtocolFixture) setAmbiguousNextMutation(operation string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ambiguousNextMutation = operation
}

func (f *segmentProtocolFixture) setDirectReadFailure(segmentID string, enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.directFailureID = segmentID
	f.directFailure = enabled
}

func (f *segmentProtocolFixture) directFailures() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.directFailureCount
}

func (f *segmentProtocolFixture) currentObject(
	environmentID string,
	key string,
) (*segmentProtocolObject, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	object := f.findByKeyLocked(environmentID, key, false)
	if object == nil || !object.Mutable {
		return nil, false
	}
	return cloneSegmentProtocolObject(object), true
}

func (f *segmentProtocolFixture) managedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.active) + len(f.archived)
}

func (f *segmentProtocolFixture) driftOwnedFields(
	environmentID string,
	key string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	object := f.findByKeyLocked(environmentID, key, false)
	if object == nil || !object.Mutable || len(object.Rules) == 0 ||
		len(object.Rules[0].Conditions) == 0 {
		return fmt.Errorf("active mutable Segment targeting is absent")
	}
	object.Name = "Synthetic external name drift"
	object.Description = "Synthetic external description drift"
	object.Included = []string{"drift-included"}
	object.Excluded = []string{"drift-excluded"}
	object.Rules[0].Name = "Synthetic external rule drift"
	object.Rules[0].Conditions[0].Value = "synthetic-external-value"
	object.Tags = []string{"drift-tag"}
	return nil
}

func (f *segmentProtocolFixture) removeActive(
	environmentID string,
	key string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	object := f.findByKeyLocked(environmentID, key, false)
	if object == nil || !object.Mutable {
		return fmt.Errorf("active mutable Segment is absent")
	}
	delete(f.active, segmentProtocolObjectIdentity(environmentID, object.ID))
	delete(f.references, object.ID)
	return nil
}

func (f *segmentProtocolFixture) archiveExternal(
	environmentID string,
	key string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	object := f.findByKeyLocked(environmentID, key, false)
	if object == nil || !object.Mutable {
		return fmt.Errorf("active mutable Segment is absent")
	}
	identity := segmentProtocolObjectIdentity(environmentID, object.ID)
	delete(f.active, identity)
	object.Archived = true
	f.archived[identity] = object
	return nil
}

func (f *segmentProtocolFixture) restoreExternal(
	environmentID string,
	key string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	object := f.findByKeyLocked(environmentID, key, true)
	if object == nil || !object.Mutable {
		return fmt.Errorf("archived mutable Segment is absent")
	}
	identity := segmentProtocolObjectIdentity(environmentID, object.ID)
	delete(f.archived, identity)
	object.Archived = false
	f.active[identity] = object
	return nil
}

func (f *segmentProtocolFixture) setManagedTaxonomy(
	environmentID string,
	key string,
	segmentType client.SegmentType,
	scopes []string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	object := f.findByKeyLocked(environmentID, key, false)
	if object == nil || !object.Mutable {
		return fmt.Errorf("active mutable Segment is absent")
	}
	object.Type = segmentType
	object.Scopes = append([]string{}, scopes...)
	return nil
}

func (f *segmentProtocolFixture) setReference(
	environmentID string,
	key string,
	present bool,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	object := f.findByKeyLocked(environmentID, key, false)
	if object == nil || !object.Mutable {
		return fmt.Errorf("active mutable Segment is absent")
	}
	if !present {
		f.references[object.ID] = []client.SegmentFlagReference{}
		return nil
	}
	f.references[object.ID] = []client.SegmentFlagReference{
		{
			EnvironmentID: environmentID,
			ID: uuid.NewSHA1(
				uuid.NameSpaceURL,
				[]byte("segment-protocol/reference"),
			).String(),
			Name: "Synthetic reference",
			Key:  "synthetic-reference",
		},
	}
	return nil
}

func (f *segmentProtocolFixture) mutationSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.mutations...)
}

func (f *segmentProtocolFixture) requestSnapshot() []segmentProtocolRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]segmentProtocolRequest(nil), f.requests...)
}

func (f *segmentProtocolFixture) violationSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.violations...)
}

func (f *segmentProtocolFixture) teardownProof() (bool, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.teardownActiveProof, f.teardownArchiveProof
}

func (o *segmentProtocolObject) clientSegment() client.Segment {
	return client.Segment{
		ID:            o.ID,
		EnvironmentID: o.EnvironmentID,
		Name:          o.Name,
		Key:           o.Key,
		Type:          o.Type,
		Scopes:        append([]string(nil), o.Scopes...),
		Description:   o.Description,
		Included:      append([]string(nil), o.Included...),
		Excluded:      append([]string(nil), o.Excluded...),
		Rules:         cloneSegmentProtocolRules(o.Rules),
		Tags:          append([]string(nil), o.Tags...),
		IsArchived:    o.Archived,
	}
}

func cloneSegmentProtocolObject(object *segmentProtocolObject) *segmentProtocolObject {
	if object == nil {
		return nil
	}
	cloned := *object
	cloned.Scopes = append([]string(nil), object.Scopes...)
	cloned.Included = append([]string(nil), object.Included...)
	cloned.Excluded = append([]string(nil), object.Excluded...)
	cloned.Rules = cloneSegmentProtocolRules(object.Rules)
	cloned.Tags = append([]string(nil), object.Tags...)
	return &cloned
}

func cloneSegmentProtocolRules(rules []client.SegmentRule) []client.SegmentRule {
	cloned := make([]client.SegmentRule, len(rules))
	for index, rule := range rules {
		cloned[index] = client.SegmentRule{
			ID:         rule.ID,
			Name:       rule.Name,
			Conditions: append([]client.SegmentCondition(nil), rule.Conditions...),
		}
	}
	return cloned
}

func segmentProtocolObjectIdentity(environmentID string, segmentID string) string {
	canonicalEnvironmentID, valid := client.CanonicalUUID(environmentID)
	if !valid {
		canonicalEnvironmentID = environmentID
	}
	canonicalSegmentID, valid := client.CanonicalUUID(segmentID)
	if !valid {
		canonicalSegmentID = segmentID
	}
	return canonicalEnvironmentID + "\x00" + canonicalSegmentID
}
