// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

const (
	segmentIDOne       = "11111111-1111-4111-8111-111111111111"
	segmentIDTwo       = "22222222-2222-4222-8222-222222222222"
	segmentIDThree     = "33333333-3333-4333-8333-333333333333"
	segmentRuleID      = "44444444-4444-4444-8444-444444444444"
	segmentConditionID = "55555555-5555-4555-8555-555555555555"
	segmentFlagID      = "66666666-6666-4666-8666-666666666666"

	segmentOrganizationScope = "organization/synthetic-org"
	segmentProjectScope      = segmentOrganizationScope + ":project/synthetic-project"
	segmentEnvironmentScope  = segmentProjectScope + ":env/synthetic-env"
)

func TestGetSegmentUsesExactCompleteSafeContract(t *testing.T) {
	t.Parallel()

	const (
		userIncluded = "synthetic-user-included"
		userExcluded = "synthetic-user-excluded"
		ruleValue    = "synthetic-rule-value"
		tagValue     = "synthetic-tag"
		segmentKey   = "segment/key"
	)
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			if request.Method != http.MethodGet ||
				request.URL.EscapedPath() != "/api/v1/envs/"+environmentOne+"/segments/"+segmentIDOne {
				t.Fatalf("unexpected exact Segment request %s %s", request.Method, request.URL.EscapedPath())
			}
			if request.URL.RawQuery != "" || (request.Body != nil && request.Body != http.NoBody) {
				t.Fatal("exact Segment GET unexpectedly contained query or body data")
			}
			if request.Header.Get("Authorization") != syntheticAccessToken ||
				request.Header.Get("User-Agent") != "terraform-provider-featbit/test" {
				t.Fatal("exact Segment GET did not use the shared authentication contract")
			}
			assertNoContextHeaders(t, request)

			data := segmentExactTestJSON(
				segmentIDOne,
				environmentOne,
				segmentKey,
				SegmentTypeEnvironmentSpecific,
				[]string{segmentEnvironmentScope},
				false,
				true,
				map[string]any{
					"workspaceId": environmentTwo,
					"createdAt":   "2026-08-04T00:00:00Z",
					"updatedAt":   "2026-08-04T00:00:00Z",
					"pending":     map[string]any{"value": "synthetic-pending-value"},
					"included":    []string{userIncluded},
					"excluded":    []string{userExcluded},
					"rules": []any{map[string]any{
						"id":   segmentRuleID,
						"name": "Synthetic rule",
						"conditions": []any{map[string]any{
							"id": segmentConditionID, "property": "synthetic-property",
							"op": "is-one-of", "value": ruleValue,
						}},
					}},
					"tags": []string{tagValue},
				},
			)
			return segmentTestResponse(request, http.StatusOK, data), nil
		},
	))

	segment, err := clientUnderTest.GetSegment(context.Background(), environmentOne, segmentIDOne)
	if err != nil {
		t.Fatalf("GetSegment() error = %v", err)
	}
	if segment.ID != segmentIDOne || segment.EnvironmentID != environmentOne ||
		segment.Key != segmentKey || segment.Type != SegmentTypeEnvironmentSpecific ||
		segment.IsArchived || len(segment.Scopes) != 1 ||
		len(segment.Included) != 1 || segment.Included[0] != userIncluded ||
		len(segment.Excluded) != 1 || segment.Excluded[0] != userExcluded ||
		len(segment.Rules) != 1 || len(segment.Rules[0].Conditions) != 1 ||
		segment.Rules[0].Conditions[0].Value != ruleValue ||
		len(segment.Tags) != 1 || segment.Tags[0] != tagValue {
		t.Fatal("GetSegment() did not preserve the complete safe definition")
	}

	encoded, err := json.Marshal(segment)
	if err != nil {
		t.Fatal("json.Marshal(Segment) failed")
	}
	for _, unsafeField := range []string{"workspaceId", "createdAt", "updatedAt", "pending", "isEnvironmentSpecific"} {
		if strings.Contains(string(encoded), unsafeField) {
			t.Fatal("safe Segment model retained a server-owned field")
		}
	}
	formatted := fmt.Sprintf(
		"%v|%+v|%#v|%v|%v|%v|%v|%v",
		segment,
		segment,
		segment,
		segment.Rules[0],
		segment.Rules[0].Conditions[0],
		SegmentMatch{ID: segmentIDOne, Key: segmentKey, Scopes: []string{segmentEnvironmentScope}},
		SegmentIdentity{ID: segmentIDOne, Key: segmentKey},
		SegmentFlagReference{ID: segmentFlagID, Key: "flag-key"},
	)
	for _, unsafe := range []string{
		segmentIDOne, environmentOne, segmentKey, userIncluded, userExcluded,
		segmentRuleID, segmentConditionID, ruleValue, tagValue,
		segmentEnvironmentScope, segmentFlagID, "flag-key",
	} {
		if strings.Contains(formatted, unsafe) {
			t.Fatal("formatted Segment contract value exposed runtime data")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("exact Segment request count = %d, want 1", calls.Load())
	}
}

func TestCreateSegmentUsesExactOneShotContract(t *testing.T) {
	t.Parallel()

	input := CreateSegmentRequest{
		Type:        SegmentTypeEnvironmentSpecific,
		Name:        "Synthetic Segment",
		Key:         "synthetic-segment",
		Description: "Synthetic description",
		Scopes:      []string{segmentEnvironmentScope},
	}
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			if request.Method != http.MethodPost ||
				request.URL.EscapedPath() != "/api/v1/envs/"+environmentOne+"/segments" ||
				request.URL.RawQuery != "" || request.Header.Get("Content-Type") != "application/json" {
				t.Fatal("CreateSegment did not use the exact documented request boundary")
			}
			assertNoContextHeaders(t, request)
			assertSegmentJSONBody(t, request, input, []string{
				"type", "name", "key", "description", "scopes",
			})
			data := segmentExactTestJSON(
				strings.ToUpper(segmentIDOne),
				environmentOne,
				input.Key,
				input.Type,
				input.Scopes,
				false,
				true,
				nil,
			)
			return segmentTestResponse(request, http.StatusOK, data), nil
		},
	))

	segment, err := clientUnderTest.CreateSegment(context.Background(), environmentOne, input)
	if err != nil {
		t.Fatalf("CreateSegment() error = %v", err)
	}
	if !EqualUUID(segment.ID, segmentIDOne) || segment.Key != input.Key ||
		segment.Type != SegmentTypeEnvironmentSpecific || segment.IsArchived {
		t.Fatal("CreateSegment() did not return the exact created Segment")
	}
	if calls.Load() != 1 {
		t.Fatalf("CreateSegment() request count = %d, want 1", calls.Load())
	}

	formatted := fmt.Sprintf(
		"%v|%+v|%#v|%v|%v|%v|%v",
		input,
		input,
		input,
		UpdateSegmentNameRequest{Name: "synthetic-name"},
		UpdateSegmentDescriptionRequest{Description: "synthetic-description"},
		UpdateSegmentTargetingRequest{Included: []string{"synthetic-user"}},
		UpdateSegmentTagsRequest{Tags: []string{"synthetic-tag"}},
	)
	for _, unsafe := range []string{
		input.Name, input.Key, input.Description, segmentEnvironmentScope,
		"synthetic-name", "synthetic-description", "synthetic-user", "synthetic-tag",
	} {
		if strings.Contains(formatted, unsafe) {
			t.Fatal("formatted Segment mutation payload exposed a runtime value")
		}
	}
}

func TestCreateSegmentPreservesAuthoritativeIDOnResponseMismatch(t *testing.T) {
	t.Parallel()

	input := CreateSegmentRequest{
		Type:        SegmentTypeEnvironmentSpecific,
		Name:        "Synthetic Segment",
		Key:         "synthetic-segment",
		Description: "Synthetic description",
		Scopes:      []string{segmentEnvironmentScope},
	}
	tests := map[string]struct {
		mutate        func(map[string]any)
		wantPreserved bool
	}{
		"wrong environment": {
			mutate:        func(data map[string]any) { data["envId"] = environmentTwo },
			wantPreserved: true,
		},
		"wrong key": {
			mutate:        func(data map[string]any) { data["key"] = "other-key" },
			wantPreserved: true,
		},
		"shared taxonomy": {
			mutate: func(data map[string]any) {
				data["type"] = string(SegmentTypeShared)
				data["scopes"] = []string{segmentOrganizationScope}
				data["isEnvironmentSpecific"] = false
			},
			wantPreserved: true,
		},
		"unknown taxonomy": {
			mutate:        func(data map[string]any) { data["type"] = "synthetic-unknown" },
			wantPreserved: true,
		},
		"scope mismatch": {
			mutate:        func(data map[string]any) { data["scopes"] = []string{segmentProjectScope} },
			wantPreserved: true,
		},
		"archived response": {
			mutate:        func(data map[string]any) { data["isArchived"] = true },
			wantPreserved: true,
		},
		"missing id": {
			mutate: func(data map[string]any) { delete(data, "id") },
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					calls.Add(1)
					data := segmentExactTestData(
						segmentIDOne,
						environmentOne,
						input.Key,
						input.Type,
						input.Scopes,
						false,
						true,
					)
					test.mutate(data)
					return segmentTestResponse(request, http.StatusOK, mustJSON(t, data)), nil
				},
			))

			segment, err := clientUnderTest.CreateSegment(context.Background(), environmentOne, input)
			requireAPIErrorClassification(t, err, ClassificationAmbiguous)
			if got := EqualUUID(segment.ID, segmentIDOne); got != test.wantPreserved {
				t.Fatalf("authoritative response ID preserved = %t, want %t", got, test.wantPreserved)
			}
			if calls.Load() != 1 {
				t.Fatalf("CreateSegment() request count = %d, want 1", calls.Load())
			}
		})
	}
}

func TestSegmentMutationValidationStopsBeforeTransport(t *testing.T) {
	t.Parallel()

	validCreate := CreateSegmentRequest{
		Type:        SegmentTypeEnvironmentSpecific,
		Name:        "Synthetic Segment",
		Key:         "synthetic-segment",
		Description: "Synthetic description",
		Scopes:      []string{segmentEnvironmentScope},
	}
	validTargeting := func() UpdateSegmentTargetingRequest {
		return UpdateSegmentTargetingRequest{
			Included: []string{},
			Excluded: []string{},
			Rules: []SegmentRule{{
				ID: segmentRuleID, Name: "Synthetic rule",
				Conditions: []SegmentCondition{{
					ID: segmentConditionID, Property: "region", Operator: "IsOneOf", Value: `[]`,
				}},
			}},
		}
	}
	tests := map[string]func(*Client) error{
		"invalid create environment": func(apiClient *Client) error {
			_, err := apiClient.CreateSegment(context.Background(), "invalid", validCreate)
			return err
		},
		"blank create name": func(apiClient *Client) error {
			input := validCreate
			input.Name = " \t"
			_, err := apiClient.CreateSegment(context.Background(), environmentOne, input)
			return err
		},
		"missing create key": func(apiClient *Client) error {
			input := validCreate
			input.Key = ""
			_, err := apiClient.CreateSegment(context.Background(), environmentOne, input)
			return err
		},
		"shared create type": func(apiClient *Client) error {
			input := validCreate
			input.Type = SegmentTypeShared
			input.Scopes = []string{segmentOrganizationScope}
			_, err := apiClient.CreateSegment(context.Background(), environmentOne, input)
			return err
		},
		"unsafe create scope": func(apiClient *Client) error {
			input := validCreate
			input.Scopes = []string{segmentProjectScope}
			_, err := apiClient.CreateSegment(context.Background(), environmentOne, input)
			return err
		},
		"blank update name": func(apiClient *Client) error {
			return apiClient.UpdateSegmentName(
				context.Background(),
				environmentOne,
				segmentIDOne,
				UpdateSegmentNameRequest{Name: " \t"},
			)
		},
		"invalid description segment": func(apiClient *Client) error {
			return apiClient.UpdateSegmentDescription(
				context.Background(),
				environmentOne,
				"invalid",
				UpdateSegmentDescriptionRequest{Description: "Synthetic"},
			)
		},
		"invalid archive environment": func(apiClient *Client) error {
			return apiClient.ArchiveSegment(
				context.Background(), "invalid", segmentIDOne,
			)
		},
		"invalid delete segment": func(apiClient *Client) error {
			return apiClient.DeleteSegment(
				context.Background(), environmentOne, "invalid",
			)
		},
		"nil targeting users": func(apiClient *Client) error {
			input := validTargeting()
			input.Included = nil
			return apiClient.UpdateSegmentTargeting(context.Background(), environmentOne, segmentIDOne, input)
		},
		"missing targeting rule id": func(apiClient *Client) error {
			input := validTargeting()
			input.Rules[0].ID = ""
			return apiClient.UpdateSegmentTargeting(context.Background(), environmentOne, segmentIDOne, input)
		},
		"duplicate targeting condition id": func(apiClient *Client) error {
			input := validTargeting()
			input.Rules[0].Conditions = append(
				input.Rules[0].Conditions,
				input.Rules[0].Conditions[0],
			)
			return apiClient.UpdateSegmentTargeting(context.Background(), environmentOne, segmentIDOne, input)
		},
		"nil tags": func(apiClient *Client) error {
			return apiClient.UpdateSegmentTags(
				context.Background(),
				environmentOne,
				segmentIDOne,
				UpdateSegmentTagsRequest{},
			)
		},
	}
	for name, invoke := range tests {
		name := name
		invoke := invoke
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(*http.Request) (*http.Response, error) {
					t.Fatal("invalid Segment mutation reached the transport")
					return nil, nil
				},
			))
			requireAPIErrorClassification(t, invoke(clientUnderTest), ClassificationValidation)
		})
	}
}

func TestSegmentSpecializedMutationsUseExactOneShotContracts(t *testing.T) {
	t.Parallel()

	name := UpdateSegmentNameRequest{Name: "Synthetic renamed Segment"}
	description := UpdateSegmentDescriptionRequest{Description: "Synthetic updated description"}
	targeting := UpdateSegmentTargetingRequest{
		Included: []string{"synthetic-user-included"},
		Excluded: []string{"synthetic-user-excluded"},
		Rules: []SegmentRule{{
			ID: segmentRuleID, Name: "Synthetic rule",
			Conditions: []SegmentCondition{{
				ID:       segmentConditionID,
				Property: "region",
				Operator: "IsOneOf",
				Value:    `["eu"]`,
			}},
		}},
	}
	tags := UpdateSegmentTagsRequest{Tags: []string{"synthetic-tag"}}
	tests := map[string]struct {
		path        string
		payload     any
		allowedKeys []string
		invoke      func(*Client) error
	}{
		"name": {
			path:        "/api/v1/envs/" + environmentOne + "/segments/" + segmentIDOne + "/name",
			payload:     name,
			allowedKeys: []string{"name"},
			invoke: func(apiClient *Client) error {
				return apiClient.UpdateSegmentName(
					context.Background(), environmentOne, segmentIDOne, name,
				)
			},
		},
		"description": {
			path:        "/api/v1/envs/" + environmentOne + "/segments/" + segmentIDOne + "/description",
			payload:     description,
			allowedKeys: []string{"description"},
			invoke: func(apiClient *Client) error {
				return apiClient.UpdateSegmentDescription(
					context.Background(), environmentOne, segmentIDOne, description,
				)
			},
		},
		"targeting": {
			path:        "/api/v1/envs/" + environmentOne + "/segments/" + segmentIDOne + "/targeting",
			payload:     targeting,
			allowedKeys: []string{"included", "excluded", "rules"},
			invoke: func(apiClient *Client) error {
				return apiClient.UpdateSegmentTargeting(
					context.Background(), environmentOne, segmentIDOne, targeting,
				)
			},
		},
		"tags": {
			path:        "/api/v1/envs/" + environmentOne + "/segments/" + segmentIDOne + "/tags",
			payload:     tags,
			allowedKeys: []string{"tags"},
			invoke: func(apiClient *Client) error {
				return apiClient.UpdateSegmentTags(
					context.Background(), environmentOne, segmentIDOne, tags,
				)
			},
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					calls.Add(1)
					if request.Method != http.MethodPut || request.URL.EscapedPath() != test.path ||
						request.URL.RawQuery != "" || request.Header.Get("Content-Type") != "application/json" {
						t.Fatal("Segment mutation did not use its exact specialized endpoint")
					}
					assertNoContextHeaders(t, request)
					assertSegmentJSONBody(t, request, test.payload, test.allowedKeys)
					return segmentTestResponse(request, http.StatusOK, "true"), nil
				},
			))

			if err := test.invoke(clientUnderTest); err != nil {
				t.Fatalf("Segment mutation error = %v", err)
			}
			if calls.Load() != 1 {
				t.Fatalf("Segment mutation request count = %d, want 1", calls.Load())
			}
		})
	}
}

func TestSegmentDestructiveMutationsUseExactBodylessOneShotContracts(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		method string
		path   string
		invoke func(*Client) error
	}{
		"archive": {
			method: http.MethodPut,
			path:   "/api/v1/envs/" + environmentOne + "/segments/" + segmentIDOne + "/archive",
			invoke: func(apiClient *Client) error {
				return apiClient.ArchiveSegment(context.Background(), environmentOne, segmentIDOne)
			},
		},
		"permanent delete": {
			method: http.MethodDelete,
			path:   "/api/v1/envs/" + environmentOne + "/segments/" + segmentIDOne,
			invoke: func(apiClient *Client) error {
				return apiClient.DeleteSegment(context.Background(), environmentOne, segmentIDOne)
			},
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					calls.Add(1)
					if request.Method != test.method || request.URL.EscapedPath() != test.path ||
						request.URL.RawQuery != "" || request.Header.Get("Content-Type") != "" ||
						(request.Body != nil && request.Body != http.NoBody) {
						t.Fatal("destructive Segment mutation did not use its exact bodyless endpoint")
					}
					assertNoContextHeaders(t, request)
					return segmentTestResponse(request, http.StatusOK, "true"), nil
				},
			))

			if err := test.invoke(clientUnderTest); err != nil {
				t.Fatalf("destructive Segment mutation error = %v", err)
			}
			if calls.Load() != 1 {
				t.Fatalf("destructive Segment mutation request count = %d, want 1", calls.Load())
			}
		})
	}
}

func TestSegmentMutationsNeverRetryAmbiguousResponses(t *testing.T) {
	t.Parallel()

	targeting := UpdateSegmentTargetingRequest{
		Included: []string{}, Excluded: []string{}, Rules: []SegmentRule{},
	}
	tests := map[string]func(*Client) error{
		"create": func(apiClient *Client) error {
			_, err := apiClient.CreateSegment(context.Background(), environmentOne, CreateSegmentRequest{
				Type: SegmentTypeEnvironmentSpecific, Name: "Synthetic", Key: "synthetic",
				Scopes: []string{segmentEnvironmentScope},
			})
			return err
		},
		"name": func(apiClient *Client) error {
			return apiClient.UpdateSegmentName(
				context.Background(), environmentOne, segmentIDOne,
				UpdateSegmentNameRequest{Name: "Synthetic renamed Segment"},
			)
		},
		"description": func(apiClient *Client) error {
			return apiClient.UpdateSegmentDescription(
				context.Background(), environmentOne, segmentIDOne,
				UpdateSegmentDescriptionRequest{Description: "Synthetic description"},
			)
		},
		"archive": func(apiClient *Client) error {
			return apiClient.ArchiveSegment(
				context.Background(), environmentOne, segmentIDOne,
			)
		},
		"delete": func(apiClient *Client) error {
			return apiClient.DeleteSegment(
				context.Background(), environmentOne, segmentIDOne,
			)
		},
		"targeting": func(apiClient *Client) error {
			return apiClient.UpdateSegmentTargeting(
				context.Background(), environmentOne, segmentIDOne, targeting,
			)
		},
		"tags": func(apiClient *Client) error {
			return apiClient.UpdateSegmentTags(
				context.Background(), environmentOne, segmentIDOne,
				UpdateSegmentTagsRequest{Tags: []string{}},
			)
		},
	}
	for name, invoke := range tests {
		name := name
		invoke := invoke
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			options := defaultTestOptions()
			options.MaxRetries = 5
			var calls atomic.Int32
			clientUnderTest := newTestClientWithTransport(t, options, roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					calls.Add(1)
					return segmentTestResponse(request, http.StatusServiceUnavailable, "null"), nil
				},
			))

			requireAPIErrorClassification(t, invoke(clientUnderTest), ClassificationTransientServer)
			if calls.Load() != 1 {
				t.Fatalf("mutation request count = %d, want exactly 1", calls.Load())
			}
		})
	}
}

func TestSegmentSpecializedMutationsRejectFalseWithoutRetry(t *testing.T) {
	t.Parallel()

	targeting := UpdateSegmentTargetingRequest{
		Included: []string{}, Excluded: []string{}, Rules: []SegmentRule{},
	}
	tests := map[string]func(*Client) error{
		"name": func(apiClient *Client) error {
			return apiClient.UpdateSegmentName(
				context.Background(), environmentOne, segmentIDOne,
				UpdateSegmentNameRequest{Name: "Synthetic renamed Segment"},
			)
		},
		"description": func(apiClient *Client) error {
			return apiClient.UpdateSegmentDescription(
				context.Background(), environmentOne, segmentIDOne,
				UpdateSegmentDescriptionRequest{Description: "Synthetic description"},
			)
		},
		"archive": func(apiClient *Client) error {
			return apiClient.ArchiveSegment(
				context.Background(), environmentOne, segmentIDOne,
			)
		},
		"delete": func(apiClient *Client) error {
			return apiClient.DeleteSegment(
				context.Background(), environmentOne, segmentIDOne,
			)
		},
		"targeting": func(apiClient *Client) error {
			return apiClient.UpdateSegmentTargeting(
				context.Background(), environmentOne, segmentIDOne, targeting,
			)
		},
		"tags": func(apiClient *Client) error {
			return apiClient.UpdateSegmentTags(
				context.Background(), environmentOne, segmentIDOne,
				UpdateSegmentTagsRequest{Tags: []string{}},
			)
		},
	}
	for name, invoke := range tests {
		name := name
		invoke := invoke
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			options := defaultTestOptions()
			options.MaxRetries = 5
			var calls atomic.Int32
			clientUnderTest := newTestClientWithTransport(t, options, roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					calls.Add(1)
					return segmentTestResponse(request, http.StatusOK, "false"), nil
				},
			))

			requireAPIErrorClassification(t, invoke(clientUnderTest), ClassificationAmbiguous)
			if calls.Load() != 1 {
				t.Fatalf("false Segment mutation request count = %d, want exactly 1", calls.Load())
			}
		})
	}
}

func TestSegmentMutationCancellationStopsBeforeTransport(t *testing.T) {
	t.Parallel()

	tests := map[string]func(context.Context, *Client) error{
		"name": func(ctx context.Context, apiClient *Client) error {
			return apiClient.UpdateSegmentName(
				ctx,
				environmentOne,
				segmentIDOne,
				UpdateSegmentNameRequest{Name: "Synthetic canceled name"},
			)
		},
		"archive": func(ctx context.Context, apiClient *Client) error {
			return apiClient.ArchiveSegment(ctx, environmentOne, segmentIDOne)
		},
		"delete": func(ctx context.Context, apiClient *Client) error {
			return apiClient.DeleteSegment(ctx, environmentOne, segmentIDOne)
		},
	}
	for name, invoke := range tests {
		name := name
		invoke := invoke
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(*http.Request) (*http.Response, error) {
					calls.Add(1)
					return nil, errors.New("canceled Segment mutation reached transport")
				},
			))
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			apiError := requireAPIErrorClassification(t, invoke(ctx, clientUnderTest), ClassificationCanceled)
			if !errors.Is(apiError, context.Canceled) || calls.Load() != 0 {
				t.Fatalf("canceled Segment mutation cause/requests = %v/%d", apiError, calls.Load())
			}
		})
	}
}

func TestGetSegmentFailsClosedForIncompleteOrContradictoryShapes(t *testing.T) {
	t.Parallel()

	type mutation func(map[string]any)
	tests := map[string]struct {
		data      any
		mutate    mutation
		wantValid bool
	}{
		"null data":           {data: nil},
		"missing id":          {mutate: func(data map[string]any) { delete(data, "id") }},
		"wrong id":            {mutate: func(data map[string]any) { data["id"] = segmentIDTwo }},
		"missing environment": {mutate: func(data map[string]any) { delete(data, "envId") }},
		"wrong environment for environment-specific": {
			mutate: func(data map[string]any) { data["envId"] = environmentTwo },
		},
		"missing name":   {mutate: func(data map[string]any) { delete(data, "name") }},
		"empty name":     {mutate: func(data map[string]any) { data["name"] = "" }},
		"missing key":    {mutate: func(data map[string]any) { delete(data, "key") }},
		"missing type":   {mutate: func(data map[string]any) { delete(data, "type") }},
		"unknown type":   {mutate: func(data map[string]any) { data["type"] = "synthetic-unknown" }},
		"missing scopes": {mutate: func(data map[string]any) { delete(data, "scopes") }},
		"null scopes":    {mutate: func(data map[string]any) { data["scopes"] = nil }},
		"legacy scope":   {mutate: func(data map[string]any) { data["scopes"] = []string{"project/p:env/e"} }},
		"environment-specific organization scope": {
			mutate: func(data map[string]any) { data["scopes"] = []string{segmentOrganizationScope} },
		},
		"environment-specific multiple scopes": {
			mutate: func(data map[string]any) {
				data["scopes"] = []string{segmentEnvironmentScope, segmentEnvironmentScope + "-two"}
			},
		},
		"missing description": {mutate: func(data map[string]any) { delete(data, "description") }},
		"missing included":    {mutate: func(data map[string]any) { delete(data, "included") }},
		"missing excluded":    {mutate: func(data map[string]any) { delete(data, "excluded") }},
		"missing rules":       {mutate: func(data map[string]any) { delete(data, "rules") }},
		"null rule conditions": {
			mutate: func(data map[string]any) {
				data["rules"] = []any{map[string]any{"id": segmentRuleID, "name": "Rule", "conditions": nil}}
			},
		},
		"missing tags":           {mutate: func(data map[string]any) { delete(data, "tags") }},
		"missing archive status": {mutate: func(data map[string]any) { delete(data, "isArchived") }},
		"missing taxonomy marker": {
			mutate: func(data map[string]any) { delete(data, "isEnvironmentSpecific") },
		},
		"contradictory taxonomy marker": {
			mutate: func(data map[string]any) { data["isEnvironmentSpecific"] = false },
		},
		"shared exact response may originate in another environment": {
			mutate: func(data map[string]any) {
				data["envId"] = environmentTwo
				data["type"] = string(SegmentTypeShared)
				data["scopes"] = []string{segmentProjectScope}
				data["isEnvironmentSpecific"] = false
			},
			wantValid: true,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			data := test.data
			if data == nil && name != "null data" {
				data = segmentExactTestData(
					segmentIDOne,
					environmentOne,
					"exact-key",
					SegmentTypeEnvironmentSpecific,
					[]string{segmentEnvironmentScope},
					false,
					true,
				)
			}
			if test.mutate != nil {
				test.mutate(data.(map[string]any))
			}
			encoded, err := json.Marshal(data)
			if err != nil {
				t.Fatal("could not encode Segment shape test")
			}
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					return segmentTestResponse(request, http.StatusOK, string(encoded)), nil
				},
			))

			segment, err := clientUnderTest.GetSegment(context.Background(), environmentOne, segmentIDOne)
			if test.wantValid {
				if err != nil || segment.Type != SegmentTypeShared || segment.EnvironmentID != environmentOne {
					t.Fatal("valid shared Segment response was rejected or lost request context")
				}
				return
			}
			requireAPIErrorClassification(t, err, ClassificationAmbiguous)
		})
	}
}

func TestClassifySegmentScopeFreezesCurrentResourceRNLevels(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		value string
		kind  SegmentScopeKind
		valid bool
	}{
		"organization":               {value: segmentOrganizationScope, kind: SegmentScopeOrganization, valid: true},
		"project":                    {value: segmentProjectScope, kind: SegmentScopeProject, valid: true},
		"environment":                {value: segmentEnvironmentScope, kind: SegmentScopeEnvironment, valid: true},
		"empty":                      {},
		"legacy project environment": {value: "project/p:env/e"},
		"wrong order":                {value: "organization/o:env/e:project/p"},
		"unknown level":              {value: "organization/o:project/p:flag/f"},
		"wildcard":                   {value: "organization/o:project/*"},
		"missing key":                {value: "organization/"},
		"extra slash":                {value: "organization/o/project/p"},
		"extra level":                {value: "organization/o:project/p:env/e:segment/s"},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			kind, valid := ClassifySegmentScope(test.value)
			if valid != test.valid || kind != test.kind {
				t.Fatalf("ClassifySegmentScope() = %q, %t, want %q, %t", kind, valid, test.kind, test.valid)
			}
		})
	}
}

func TestListSegmentsConsumesEveryPageWithExactDocumentedQuery(t *testing.T) {
	t.Parallel()

	pages := [][]string{
		{
			segmentListItemTestJSON(segmentIDOne, "one", SegmentTypeEnvironmentSpecific, []string{segmentEnvironmentScope}, nil),
			segmentListItemTestJSON(segmentIDTwo, "two", SegmentTypeShared, []string{segmentProjectScope}, nil),
		},
		{
			segmentListItemTestJSON(segmentIDThree, "three", SegmentTypeShared, []string{segmentOrganizationScope}, map[string]any{
				"included": []string{"unsafe-list-user"},
				"rules":    []any{map[string]any{"value": "unsafe-list-rule"}},
			}),
		},
	}
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			call := int(calls.Add(1))
			if request.Method != http.MethodGet ||
				request.URL.EscapedPath() != "/api/v1/envs/"+environmentOne+"/segments" {
				t.Fatalf("unexpected Segment collection request %s %s", request.Method, request.URL.EscapedPath())
			}
			if request.Body != nil && request.Body != http.NoBody {
				t.Fatal("Segment collection GET unexpectedly contained a body")
			}
			query := request.URL.Query()
			if len(query) != 4 || query.Get("Name") != "" ||
				query.Get("IsArchived") != "true" ||
				query.Get("PageIndex") != strconv.Itoa(call-1) ||
				query.Get("PageSize") != strconv.Itoa(segmentPageSize) {
				t.Fatalf("unexpected documented Segment query = %q", request.URL.RawQuery)
			}
			wantRawQuery := "IsArchived=true&Name=&PageIndex=" + strconv.Itoa(call-1) +
				"&PageSize=" + strconv.Itoa(segmentPageSize)
			if request.URL.RawQuery != wantRawQuery {
				t.Fatalf("Segment raw query = %q, want %q", request.URL.RawQuery, wantRawQuery)
			}
			assertNoContextHeaders(t, request)
			if call > len(pages) {
				t.Fatal("Segment pagination did not terminate at totalCount")
			}
			return segmentTestResponse(
				request,
				http.StatusOK,
				segmentPageTestJSON(3, pages[call-1]),
			), nil
		},
	))

	matches, err := clientUnderTest.ListSegments(context.Background(), environmentOne, true)
	if err != nil {
		t.Fatalf("ListSegments() error = %v", err)
	}
	if len(matches) != 3 || calls.Load() != 2 {
		t.Fatalf("complete Segment collection size/calls = %d/%d, want 3/2", len(matches), calls.Load())
	}
	for _, match := range matches {
		if match.EnvironmentID != environmentOne || !match.IsArchived {
			t.Fatal("Segment collection context was not applied to a list match")
		}
	}
	encoded, err := json.Marshal(matches)
	if err != nil {
		t.Fatal("json.Marshal([]SegmentMatch) failed")
	}
	for _, unsafe := range []string{"name", "description", "tags", "included", "excluded", "rules", "unsafe-list-user", "unsafe-list-rule"} {
		if strings.Contains(string(encoded), unsafe) {
			t.Fatal("Segment collection match retained metadata or targeting state")
		}
	}
}

func TestListSegmentsFailsClosedForMalformedOrIncompletePages(t *testing.T) {
	t.Parallel()

	first := segmentListItemTestJSON(segmentIDOne, "first", SegmentTypeEnvironmentSpecific, []string{segmentEnvironmentScope}, nil)
	second := segmentListItemTestJSON(segmentIDTwo, "second", SegmentTypeShared, []string{segmentProjectScope}, nil)
	oversized := make([]string, 0, segmentPageSize+1)
	for index := 0; index <= segmentPageSize; index++ {
		id := uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("segment-page/v1/%d", index))).String()
		oversized = append(oversized, segmentListItemTestJSON(
			id,
			fmt.Sprintf("key-%d", index),
			SegmentTypeShared,
			[]string{segmentProjectScope},
			nil,
		))
	}

	tests := map[string]struct {
		archived bool
		pages    []string
		wantOK   bool
	}{
		"empty complete collection": {pages: []string{segmentPageTestJSON(0, []string{})}, wantOK: true},
		"null data":                 {pages: []string{"null"}},
		"missing total count":       {pages: []string{`{"items":[]}`}},
		"negative total count":      {pages: []string{`{"totalCount":-1,"items":[]}`}},
		"null items":                {pages: []string{`{"totalCount":0,"items":null}`}},
		"empty page before total": {
			pages: []string{segmentPageTestJSON(2, []string{first}), segmentPageTestJSON(2, []string{})},
		},
		"total count changes": {
			pages: []string{segmentPageTestJSON(2, []string{first}), segmentPageTestJSON(3, []string{second})},
		},
		"items exceed total":          {pages: []string{segmentPageTestJSON(1, []string{first, second})}},
		"page exceeds requested size": {pages: []string{segmentPageTestJSON(int64(len(oversized)), oversized)}},
		"repeated item across pages": {
			pages: []string{segmentPageTestJSON(2, []string{first}), segmentPageTestJSON(2, []string{first})},
		},
		"invalid item id": {
			pages: []string{segmentPageTestJSON(1, []string{
				segmentListItemTestJSON("not-a-uuid", "invalid", SegmentTypeShared, []string{segmentProjectScope}, nil),
			})},
		},
		"missing item key": {
			pages: []string{segmentPageTestJSON(1, []string{
				segmentListItemTestJSON(segmentIDOne, "", SegmentTypeShared, []string{segmentProjectScope}, nil),
			})},
		},
		"unknown item type": {
			pages: []string{segmentPageTestJSON(1, []string{
				segmentListItemTestJSON(segmentIDOne, "unknown", SegmentType("unknown"), []string{segmentProjectScope}, nil),
			})},
		},
		"null item scopes": {
			pages: []string{segmentPageTestJSON(1, []string{
				segmentListItemTestJSON(segmentIDOne, "null-scopes", SegmentTypeShared, nil, nil),
			})},
		},
		"legacy item scope": {
			pages: []string{segmentPageTestJSON(1, []string{
				segmentListItemTestJSON(segmentIDOne, "legacy", SegmentTypeShared, []string{"project/p"}, nil),
			})},
		},
		"environment-specific multiple scopes": {
			pages: []string{segmentPageTestJSON(1, []string{
				segmentListItemTestJSON(
					segmentIDOne,
					"multiple",
					SegmentTypeEnvironmentSpecific,
					[]string{segmentEnvironmentScope, segmentEnvironmentScope + "-two"},
					nil,
				),
			})},
		},
		"conflicting archive context": {
			archived: true,
			pages: []string{segmentPageTestJSON(1, []string{
				segmentListItemTestJSON(segmentIDOne, "archive", SegmentTypeShared, []string{segmentProjectScope}, map[string]any{"isArchived": false}),
			})},
		},
		"contradictory taxonomy marker": {
			pages: []string{segmentPageTestJSON(1, []string{
				segmentListItemTestJSON(segmentIDOne, "marker", SegmentTypeShared, []string{segmentProjectScope}, map[string]any{"isEnvironmentSpecific": true}),
			})},
		},
		"wrong environment context": {
			pages: []string{segmentPageTestJSON(1, []string{
				segmentListItemTestJSON(segmentIDOne, "context", SegmentTypeEnvironmentSpecific, []string{segmentEnvironmentScope}, map[string]any{"envId": environmentTwo}),
			})},
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					page := int(calls.Add(1)) - 1
					if page >= len(test.pages) {
						t.Fatal("incomplete Segment pagination made an unbounded request")
					}
					return segmentTestResponse(request, http.StatusOK, test.pages[page]), nil
				},
			))
			matches, err := clientUnderTest.ListSegments(context.Background(), environmentOne, test.archived)
			if test.wantOK {
				if err != nil || len(matches) != 0 {
					t.Fatal("complete empty Segment collection was rejected")
				}
				return
			}
			requireAPIErrorClassification(t, err, ClassificationAmbiguous)
			if matches != nil {
				t.Fatal("malformed Segment pagination returned a partial collection")
			}
		})
	}
}

func TestResolveSegmentDistinguishesExactUUIDKeyAndStatuses(t *testing.T) {
	t.Parallel()

	active := segmentMatchForTest(segmentIDOne, "exact", false)
	archived := segmentMatchForTest(segmentIDTwo, "archived", true)
	duplicate := segmentMatchForTest(segmentIDThree, "exact", false)

	tests := map[string]struct {
		active             []SegmentMatch
		archived           []SegmentMatch
		identity           SegmentIdentity
		wantStatus         SegmentStatus
		wantID             string
		wantClassification Classification
	}{
		"exact zero ignores fuzzy and key case": {
			active: []SegmentMatch{
				segmentMatchForTest(segmentIDOne, "exact-extra", false),
				segmentMatchForTest(segmentIDTwo, "Exact", false),
			},
			identity:   SegmentIdentity{Key: "exact"},
			wantStatus: SegmentStatusAbsent,
		},
		"exact active by id": {
			active: []SegmentMatch{active}, identity: SegmentIdentity{ID: segmentIDOne},
			wantStatus: SegmentStatusActive, wantID: segmentIDOne,
		},
		"exact archived by key": {
			archived: []SegmentMatch{archived}, identity: SegmentIdentity{Key: "archived"},
			wantStatus: SegmentStatusArchived, wantID: segmentIDTwo,
		},
		"consistent id and key": {
			active: []SegmentMatch{active}, identity: SegmentIdentity{ID: segmentIDOne, Key: "exact"},
			wantStatus: SegmentStatusActive, wantID: segmentIDOne,
		},
		"id and key identify different objects": {
			active:   []SegmentMatch{active, segmentMatchForTest(segmentIDTwo, "other", false)},
			identity: SegmentIdentity{ID: segmentIDTwo, Key: "exact"}, wantClassification: ClassificationAmbiguous,
		},
		"tracked id has a different key": {
			active: []SegmentMatch{active}, identity: SegmentIdentity{ID: segmentIDOne, Key: "changed"},
			wantClassification: ClassificationAmbiguous,
		},
		"tracked key has a different id": {
			active: []SegmentMatch{active}, identity: SegmentIdentity{ID: segmentIDTwo, Key: "exact"},
			wantClassification: ClassificationAmbiguous,
		},
		"duplicate active exact key": {
			active: []SegmentMatch{active, duplicate}, identity: SegmentIdentity{Key: "exact"},
			wantClassification: ClassificationAmbiguous,
		},
		"same exact id across views": {
			active:   []SegmentMatch{active},
			archived: []SegmentMatch{segmentMatchForTest(segmentIDOne, "exact", true)},
			identity: SegmentIdentity{ID: segmentIDOne}, wantClassification: ClassificationAmbiguous,
		},
		"active view contains archived match": {
			active:   []SegmentMatch{segmentMatchForTest(segmentIDOne, "exact", true)},
			identity: SegmentIdentity{ID: segmentIDOne}, wantClassification: ClassificationAmbiguous,
		},
		"archived view contains active match": {
			archived: []SegmentMatch{active}, identity: SegmentIdentity{ID: segmentIDOne},
			wantClassification: ClassificationAmbiguous,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			match, status, err := resolveSegment(test.active, test.archived, test.identity, NewRedactor("synthetic"))
			if test.wantClassification != "" {
				requireAPIErrorClassification(t, err, test.wantClassification)
				if status != SegmentStatusUnknown {
					t.Fatal("ambiguous Segment resolution returned a usable status")
				}
				return
			}
			if err != nil || status != test.wantStatus || match.ID != test.wantID {
				t.Fatal("exact Segment resolver returned an unexpected outcome")
			}
		})
	}
}

func TestResolveSegmentAlwaysConsumesCompleteActiveAndArchivedViews(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			call := calls.Add(1)
			wantArchived := "false"
			items := []string{segmentListItemTestJSON(
				segmentIDOne,
				"exact",
				SegmentTypeEnvironmentSpecific,
				[]string{segmentEnvironmentScope},
				nil,
			)}
			if call == 2 {
				wantArchived = "true"
				items = []string{}
			}
			if call > 2 || request.URL.Query().Get("IsArchived") != wantArchived ||
				request.URL.Query().Get("Name") != "" {
				t.Fatalf("Segment resolver view %d query = %q", call, request.URL.RawQuery)
			}
			return segmentTestResponse(request, http.StatusOK, segmentPageTestJSON(int64(len(items)), items)), nil
		},
	))

	match, status, err := clientUnderTest.ResolveSegment(
		context.Background(),
		environmentOne,
		SegmentIdentity{ID: segmentIDOne, Key: "exact"},
	)
	if err != nil || status != SegmentStatusActive || match.ID != segmentIDOne {
		t.Fatal("ResolveSegment() did not compose the complete active and archived views")
	}
	if calls.Load() != 2 {
		t.Fatalf("Segment resolver request count = %d, want 2", calls.Load())
	}
}

func TestSegmentReadValidationAndCancellationBoundaries(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, errors.New("request must not execute")
		},
	))

	_, err := clientUnderTest.GetSegment(context.Background(), "invalid", segmentIDOne)
	requireAPIErrorClassification(t, err, ClassificationValidation)
	_, err = clientUnderTest.GetSegment(context.Background(), environmentOne, "invalid")
	requireAPIErrorClassification(t, err, ClassificationValidation)
	_, err = clientUnderTest.ListSegments(context.Background(), "invalid", false)
	requireAPIErrorClassification(t, err, ClassificationValidation)
	_, _, err = clientUnderTest.ResolveSegment(context.Background(), environmentOne, SegmentIdentity{})
	requireAPIErrorClassification(t, err, ClassificationValidation)
	_, _, err = clientUnderTest.ResolveSegment(context.Background(), environmentOne, SegmentIdentity{ID: "invalid"})
	requireAPIErrorClassification(t, err, ClassificationValidation)
	_, err = clientUnderTest.GetSegmentFlagReferences(context.Background(), environmentOne, "invalid")
	requireAPIErrorClassification(t, err, ClassificationValidation)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = clientUnderTest.GetSegment(ctx, environmentOne, segmentIDOne)
	apiError := requireAPIErrorClassification(t, err, ClassificationCanceled)
	if !errors.Is(apiError, context.Canceled) {
		t.Fatal("Segment exact-read cancellation sentinel was not preserved")
	}
	_, err = clientUnderTest.GetSegmentFlagReferences(ctx, environmentOne, segmentIDOne)
	apiError = requireAPIErrorClassification(t, err, ClassificationCanceled)
	if !errors.Is(apiError, context.Canceled) {
		t.Fatal("Segment reference-preflight cancellation sentinel was not preserved")
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid or canceled Segment reads executed %d transport calls", calls.Load())
	}
}

func TestListSegmentsCancellationStopsBetweenPages(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			cancel()
			return segmentTestResponse(request, http.StatusOK, segmentPageTestJSON(2, []string{
				segmentListItemTestJSON(segmentIDOne, "first", SegmentTypeEnvironmentSpecific, []string{segmentEnvironmentScope}, nil),
			})), nil
		},
	))

	_, err := clientUnderTest.ListSegments(ctx, environmentOne, false)
	apiError := requireAPIErrorClassification(t, err, ClassificationCanceled)
	if !errors.Is(apiError, context.Canceled) {
		t.Fatal("Segment pagination cancellation sentinel was not preserved")
	}
	if calls.Load() != 1 {
		t.Fatalf("Segment pagination cancellation executed %d transport calls, want 1", calls.Load())
	}
}

func TestSegmentReadsUseBodylessGETRetryBoundary(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		call func(*Client) error
		data string
	}{
		"exact": {
			call: func(clientUnderTest *Client) error {
				_, err := clientUnderTest.GetSegment(context.Background(), environmentOne, segmentIDOne)
				return err
			},
			data: segmentExactTestJSON(
				segmentIDOne, environmentOne, "exact", SegmentTypeEnvironmentSpecific,
				[]string{segmentEnvironmentScope}, false, true, nil,
			),
		},
		"collection": {
			call: func(clientUnderTest *Client) error {
				_, err := clientUnderTest.ListSegments(context.Background(), environmentOne, false)
				return err
			},
			data: segmentPageTestJSON(0, []string{}),
		},
		"flag references": {
			call: func(clientUnderTest *Client) error {
				_, err := clientUnderTest.GetSegmentFlagReferences(context.Background(), environmentOne, segmentIDOne)
				return err
			},
			data: "[]",
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var attempts atomic.Int32
			var waits atomic.Int32
			options := defaultTestOptions()
			options.MaxRetries = 1
			clientUnderTest := newTestClientWithTransport(t, options, roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					if request.Body != nil && request.Body != http.NoBody {
						t.Fatal("Segment GET unexpectedly contained a body")
					}
					if attempts.Add(1) == 1 {
						return segmentTestResponse(request, http.StatusServiceUnavailable, "null"), nil
					}
					return segmentTestResponse(request, http.StatusOK, test.data), nil
				},
			))
			clientUnderTest.retries.wait = func(context.Context, time.Duration) error {
				waits.Add(1)
				return nil
			}

			if err := test.call(clientUnderTest); err != nil {
				t.Fatalf("retryable Segment read error = %v", err)
			}
			if attempts.Load() != 2 || waits.Load() != 1 {
				t.Fatal("Segment GET used the wrong retry or wait count")
			}
		})
	}
}

func TestGetSegmentDirectNotFoundRemainsUnconfirmed(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			return segmentTestResponse(request, http.StatusNotFound, "null"), nil
		},
	))
	_, err := clientUnderTest.GetSegment(context.Background(), environmentOne, segmentIDOne)
	requireAPIErrorClassification(t, err, ClassificationNotFoundUnconfirmed)
	if calls.Load() != 1 {
		t.Fatal("direct Segment 404 unexpectedly triggered a collection fallback")
	}
}

func TestGetSegmentFlagReferencesUsesSeparateExactReadBoundary(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			if request.Method != http.MethodGet ||
				request.URL.EscapedPath() != "/api/v1/envs/"+environmentOne+"/segments/"+segmentIDOne+"/flag-references" ||
				request.URL.RawQuery != "" || (request.Body != nil && request.Body != http.NoBody) {
				t.Fatal("Feature Flag reference preflight used an unexpected request contract")
			}
			assertNoContextHeaders(t, request)
			data := mustJSON(t, []any{
				map[string]any{"envId": environmentOne, "id": segmentFlagID, "name": "Flag", "key": "flag-one"},
				map[string]any{"envId": environmentTwo, "id": featureFlagIDTwo, "name": "Flag", "key": "flag-two"},
			})
			return segmentTestResponse(request, http.StatusOK, data), nil
		},
	))

	references, err := clientUnderTest.GetSegmentFlagReferences(context.Background(), environmentOne, segmentIDOne)
	if err != nil || len(references) != 2 || references[0].ID != segmentFlagID || references[1].EnvironmentID != environmentTwo {
		t.Fatal("GetSegmentFlagReferences() did not preserve exact reference identities")
	}
	if calls.Load() != 1 {
		t.Fatalf("Feature Flag reference request count = %d, want 1", calls.Load())
	}
}

func TestGetSegmentFlagReferencesFailsClosedForIncompleteResults(t *testing.T) {
	t.Parallel()

	valid := map[string]any{"envId": environmentOne, "id": segmentFlagID, "name": "Flag", "key": "flag-key"}
	tests := map[string]struct {
		data   any
		wantOK bool
	}{
		"empty exact result":        {data: []any{}, wantOK: true},
		"null result":               {data: nil},
		"invalid environment":       {data: []any{map[string]any{"envId": "invalid", "id": segmentFlagID, "key": "flag-key"}}},
		"invalid flag id":           {data: []any{map[string]any{"envId": environmentOne, "id": "invalid", "key": "flag-key"}}},
		"missing flag key":          {data: []any{map[string]any{"envId": environmentOne, "id": segmentFlagID}}},
		"duplicate exact reference": {data: []any{valid, valid}},
		"same flag id in different environments": {data: []any{
			valid,
			map[string]any{"envId": environmentTwo, "id": segmentFlagID, "name": "Flag", "key": "flag-key"},
		}},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					return segmentTestResponse(request, http.StatusOK, mustJSON(t, test.data)), nil
				},
			))
			references, err := clientUnderTest.GetSegmentFlagReferences(context.Background(), environmentOne, segmentIDOne)
			if test.wantOK {
				if err != nil || references == nil || len(references) != 0 {
					t.Fatal("complete empty Feature Flag reference result was rejected")
				}
				return
			}
			requireAPIErrorClassification(t, err, ClassificationAmbiguous)
			if references != nil {
				t.Fatal("incomplete Feature Flag reference result returned partial data")
			}
		})
	}
}

func TestSegmentReadErrorsRedactEveryRuntimeValueClass(t *testing.T) {
	t.Parallel()

	const (
		tokenMarker     = "api-segment-redaction-marker"
		keyMarker       = "segment-redaction-key"
		userMarker      = "segment-user-redaction-marker"
		ruleMarker      = "segment-rule-redaction-marker"
		conditionMarker = "segment-condition-redaction-marker"
		valueMarker     = "segment-value-redaction-marker"
		tagMarker       = "segment-tag-redaction-marker"
		scopeMarker     = "organization/segment-scope-redaction-marker"
		flagMarker      = "segment-flag-reference-redaction-marker"
		tenantMarker    = "featbit:segment-tenant-redaction-marker"
		pathMarker      = "/api/v1/envs/segment-path-redaction-marker"
		rawBodyMarker   = "segment-raw-body-redaction-marker"
		serverMarker    = "segment-server-redaction.example.test"
	)
	detail := strings.Join([]string{
		tokenMarker, environmentOne, segmentIDOne, keyMarker, userMarker,
		ruleMarker, conditionMarker, valueMarker, tagMarker, scopeMarker,
		flagMarker, tenantMarker, pathMarker, rawBodyMarker,
	}, " | ")
	body, err := json.Marshal(map[string]any{"success": false, "data": nil, "errors": []string{detail}})
	if err != nil {
		t.Fatal("could not construct Segment redaction response")
	}

	options := defaultTestOptions()
	options.MaxRetries = 0
	clientUnderTest, err := newClient(
		mustParseURL(t, "https://"+serverMarker+"/api/v1"),
		tokenMarker,
		options,
		roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return syntheticResponse(request, http.StatusBadRequest, io.NopCloser(strings.NewReader(string(body)))), nil
		}),
	)
	if err != nil {
		t.Fatal("could not construct Segment redaction client")
	}

	_, readErr := clientUnderTest.GetSegment(context.Background(), environmentOne, segmentIDOne)
	apiError := requireAPIErrorClassification(t, readErr, ClassificationValidation)
	formatted := fmt.Sprintf("%v|%+v|%#v|%s", apiError, apiError, apiError, apiError)
	redactedDetails := strings.Join(apiError.Details(), " | ")
	for _, unsafe := range []string{
		tokenMarker, environmentOne, segmentIDOne, keyMarker, userMarker,
		ruleMarker, conditionMarker, valueMarker, tagMarker, scopeMarker,
		flagMarker, tenantMarker, pathMarker, rawBodyMarker, serverMarker,
	} {
		if strings.Contains(formatted, unsafe) || strings.Contains(redactedDetails, unsafe) {
			t.Fatal("Segment read error exposed a runtime or server value")
		}
	}
	if len(apiError.Details()) != 0 {
		t.Fatal("Segment read error retained arbitrary server detail strings")
	}
}

func TestSegmentMutationErrorsRedactEveryRuntimeValueClass(t *testing.T) {
	t.Parallel()

	const (
		tokenMarker     = "api-segment-mutation-redaction-marker"
		keyMarker       = "segment-mutation-redaction-key"
		nameMarker      = "segment-mutation-redaction-name"
		descriptionMark = "segment-mutation-redaction-description"
		userMarker      = "segment-mutation-user-redaction-marker"
		ruleMarker      = "segment-mutation-rule-redaction-marker"
		conditionMarker = "segment-mutation-condition-redaction-marker"
		valueMarker     = "segment-mutation-value-redaction-marker"
		tagMarker       = "segment-mutation-tag-redaction-marker"
		scopeMarker     = "organization/synthetic-org:project/synthetic-project:env/segment-mutation-scope-redaction-marker"
		tenantMarker    = "featbit:segment-mutation-tenant-redaction-marker"
		pathMarker      = "/api/v1/envs/segment-mutation-path-redaction-marker"
		rawBodyMarker   = "segment-mutation-raw-body-redaction-marker"
		serverMarker    = "segment-mutation-server-redaction.example.test"
	)
	detail := strings.Join([]string{
		tokenMarker, environmentOne, segmentIDOne, keyMarker, nameMarker,
		descriptionMark, userMarker, ruleMarker, conditionMarker, valueMarker,
		tagMarker, scopeMarker, tenantMarker, pathMarker, rawBodyMarker,
	}, " | ")
	body, err := json.Marshal(map[string]any{
		"success": false, "data": nil, "errors": []string{detail},
	})
	if err != nil {
		t.Fatal("could not construct Segment mutation redaction response")
	}

	targeting := UpdateSegmentTargetingRequest{
		Included: []string{userMarker},
		Excluded: []string{},
		Rules: []SegmentRule{{
			ID: segmentRuleID, Name: ruleMarker,
			Conditions: []SegmentCondition{{
				ID: segmentConditionID, Property: conditionMarker,
				Operator: "IsOneOf", Value: valueMarker,
			}},
		}},
	}
	tests := map[string]func(*Client) error{
		"create": func(apiClient *Client) error {
			_, err := apiClient.CreateSegment(context.Background(), environmentOne, CreateSegmentRequest{
				Type: SegmentTypeEnvironmentSpecific, Name: nameMarker, Key: keyMarker,
				Description: descriptionMark, Scopes: []string{scopeMarker},
			})
			return err
		},
		"name": func(apiClient *Client) error {
			return apiClient.UpdateSegmentName(
				context.Background(), environmentOne, segmentIDOne,
				UpdateSegmentNameRequest{Name: nameMarker},
			)
		},
		"description": func(apiClient *Client) error {
			return apiClient.UpdateSegmentDescription(
				context.Background(), environmentOne, segmentIDOne,
				UpdateSegmentDescriptionRequest{Description: descriptionMark},
			)
		},
		"targeting": func(apiClient *Client) error {
			return apiClient.UpdateSegmentTargeting(
				context.Background(), environmentOne, segmentIDOne, targeting,
			)
		},
		"tags": func(apiClient *Client) error {
			return apiClient.UpdateSegmentTags(
				context.Background(), environmentOne, segmentIDOne,
				UpdateSegmentTagsRequest{Tags: []string{tagMarker}},
			)
		},
		"archive": func(apiClient *Client) error {
			return apiClient.ArchiveSegment(
				context.Background(), environmentOne, segmentIDOne,
			)
		},
		"delete": func(apiClient *Client) error {
			return apiClient.DeleteSegment(
				context.Background(), environmentOne, segmentIDOne,
			)
		},
	}
	for name, invoke := range tests {
		name := name
		invoke := invoke
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			options := defaultTestOptions()
			options.MaxRetries = 0
			clientUnderTest, err := newClient(
				mustParseURL(t, "https://"+serverMarker+"/api/v1"),
				tokenMarker,
				options,
				roundTripFunc(func(request *http.Request) (*http.Response, error) {
					return syntheticResponse(
						request,
						http.StatusBadRequest,
						io.NopCloser(strings.NewReader(string(body))),
					), nil
				}),
			)
			if err != nil {
				t.Fatal("could not construct Segment mutation redaction client")
			}

			apiError := requireAPIErrorClassification(t, invoke(clientUnderTest), ClassificationValidation)
			formatted := fmt.Sprintf("%v|%+v|%#v|%s", apiError, apiError, apiError, apiError)
			redactedDetails := strings.Join(apiError.Details(), " | ")
			for _, unsafe := range []string{
				tokenMarker, environmentOne, segmentIDOne, keyMarker, nameMarker,
				descriptionMark, userMarker, ruleMarker, conditionMarker,
				valueMarker, tagMarker, scopeMarker, tenantMarker, pathMarker,
				rawBodyMarker, serverMarker,
			} {
				if strings.Contains(formatted, unsafe) || strings.Contains(redactedDetails, unsafe) {
					t.Fatal("Segment mutation error exposed a runtime or server value")
				}
			}
			if len(apiError.Details()) != 0 {
				t.Fatal("Segment mutation error retained arbitrary server detail strings")
			}
		})
	}
}

func assertNoContextHeaders(t *testing.T, request *http.Request) {
	t.Helper()
	for _, header := range contextHeaders {
		if request.Header.Get(header) != "" {
			t.Fatalf("Segment request sent unsupported context header %q", header)
		}
	}
}

func assertSegmentJSONBody(
	t *testing.T,
	request *http.Request,
	want any,
	allowedKeys []string,
) {
	t.Helper()

	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal("could not read Segment mutation request body")
	}
	var gotValue any
	if err := json.Unmarshal(body, &gotValue); err != nil {
		t.Fatal("Segment mutation request body was not valid JSON")
	}
	wantBody, err := json.Marshal(want)
	if err != nil {
		t.Fatal("could not encode expected Segment mutation body")
	}
	var wantValue any
	if err := json.Unmarshal(wantBody, &wantValue); err != nil {
		t.Fatal("expected Segment mutation body was not valid JSON")
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatal("Segment mutation body did not match the documented payload")
	}

	object, ok := gotValue.(map[string]any)
	if !ok || len(object) != len(allowedKeys) {
		t.Fatal("Segment mutation body contained an unexpected field")
	}
	for _, key := range allowedKeys {
		if _, exists := object[key]; !exists {
			t.Fatal("Segment mutation body omitted a documented field")
		}
	}
}

func segmentTestResponse(request *http.Request, status int, data string) *http.Response {
	success := status >= http.StatusOK && status < http.StatusMultipleChoices
	body := `{"success":` + strconv.FormatBool(success) + `,"data":` + data + `,"errors":[]}`
	return syntheticResponse(request, status, io.NopCloser(strings.NewReader(body)))
}

func segmentExactTestJSON(
	id string,
	environmentID string,
	key string,
	segmentType SegmentType,
	scopes []string,
	archived bool,
	isEnvironmentSpecific bool,
	overrides map[string]any,
) string {
	data := segmentExactTestData(id, environmentID, key, segmentType, scopes, archived, isEnvironmentSpecific)
	for name, value := range overrides {
		data[name] = value
	}
	encoded, _ := json.Marshal(data)
	return string(encoded)
}

func segmentExactTestData(
	id string,
	environmentID string,
	key string,
	segmentType SegmentType,
	scopes []string,
	archived bool,
	isEnvironmentSpecific bool,
) map[string]any {
	return map[string]any{
		"id": id, "envId": environmentID, "name": "Synthetic Segment", "key": key,
		"type": string(segmentType), "scopes": scopes, "description": "Synthetic description",
		"included": []string{}, "excluded": []string{}, "rules": []any{}, "tags": []string{},
		"isArchived": archived, "isEnvironmentSpecific": isEnvironmentSpecific,
	}
}

func segmentListItemTestJSON(
	id string,
	key string,
	segmentType SegmentType,
	scopes []string,
	overrides map[string]any,
) string {
	data := map[string]any{
		"id": id, "name": "Synthetic Segment", "key": key, "type": string(segmentType),
		"scopes": scopes, "tags": []string{}, "description": "Synthetic description",
		"createdAt": "2026-08-04T00:00:00Z", "updatedAt": "2026-08-04T00:00:00Z",
	}
	for name, value := range overrides {
		data[name] = value
	}
	encoded, _ := json.Marshal(data)
	return string(encoded)
}

func segmentPageTestJSON(totalCount int64, items []string) string {
	return `{"totalCount":` + strconv.FormatInt(totalCount, 10) +
		`,"items":[` + strings.Join(items, ",") + `]}`
}

func segmentMatchForTest(id, key string, archived bool) SegmentMatch {
	return SegmentMatch{
		ID: id, EnvironmentID: environmentOne, Key: key,
		Type: SegmentTypeEnvironmentSpecific, Scopes: []string{segmentEnvironmentScope},
		IsArchived: archived,
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal("could not encode Segment test data")
	}
	return string(encoded)
}
