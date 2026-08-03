// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	providerFeatureFlagFuzzyIDOne = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	providerFeatureFlagFuzzyIDTwo = "ffffffff-ffff-4fff-8fff-ffffffffffff"
)

type featureFlagHTTPExpectation struct {
	method    string
	path      string
	query     string
	status    int
	data      string
	checkBody func(*testing.T, *http.Request)
}

type featureFlagHTTPScript struct {
	t            *testing.T
	mu           sync.Mutex
	expectations []featureFlagHTTPExpectation
	next         int
}

func (s *featureFlagHTTPScript) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	s.mu.Lock()
	if s.next >= len(s.expectations) {
		s.mu.Unlock()
		s.t.Errorf("unexpected Feature Flag request %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
		writeProjectResourceEnvelope(s.t, response, http.StatusInternalServerError, "null")
		return
	}
	expectation := s.expectations[s.next]
	s.next++
	s.mu.Unlock()

	if request.Method != expectation.method || request.URL.EscapedPath() != expectation.path ||
		request.URL.RawQuery != expectation.query {
		s.t.Errorf(
			"Feature Flag request = %s %s?%s, want %s %s?%s",
			request.Method,
			request.URL.EscapedPath(),
			request.URL.RawQuery,
			expectation.method,
			expectation.path,
			expectation.query,
		)
	}
	assertFeatureFlagResourceRequestBoundary(s.t, request)
	if expectation.checkBody != nil {
		expectation.checkBody(s.t, request)
	}
	writeProjectResourceEnvelope(s.t, response, expectation.status, expectation.data)
}

func (s *featureFlagHTTPScript) consumed() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.next
}

func (s *featureFlagHTTPScript) assertComplete(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.next != len(s.expectations) {
		t.Fatalf("Feature Flag script consumed %d/%d requests", s.next, len(s.expectations))
	}
}

func TestFeatureFlagResourceUpdateSendsOnlyNameAndReadsCanonicalState(t *testing.T) {
	t.Parallel()

	const key = "update-name"
	featureFlagSchema := featureFlagResourceSchema()
	priorState, plan, remote := featureFlagUpdateStateAndPlan(
		t,
		featureFlagSchema,
		key,
		"Before",
		"After",
	)
	pathPrefix := featureFlagProviderPath(key)
	script := &featureFlagHTTPScript{
		t: t,
		expectations: []featureFlagHTTPExpectation{
			{
				method: http.MethodPut,
				path:   pathPrefix + "/name",
				status: http.StatusOK,
				data:   `"` + providerFeatureFlagID + `"`,
				checkBody: func(t *testing.T, request *http.Request) {
					var body map[string]json.RawMessage
					if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
						t.Fatalf("decode name-only Update body: %v", err)
					}
					var name string
					if len(body) != 1 || json.Unmarshal(body["name"], &name) != nil ||
						name != "After" {
						t.Fatal("Update body rewrote a field other than name")
					}
				},
			},
			{
				method: http.MethodGet,
				path:   pathPrefix,
				status: http.StatusOK,
				data: featureFlagResourceDefinitionJSON(
					t,
					remote,
					providerFeatureFlagID,
					false,
					true,
				),
			},
		},
	}
	apiClient, closeServer := newProjectResourceTestClient(t, script)
	defer closeServer()

	response := frameworkresource.UpdateResponse{State: priorState}
	(&featureFlagResource{client: apiClient}).Update(
		context.Background(),
		frameworkresource.UpdateRequest{State: priorState, Plan: plan},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Update() diagnostics = %v", response.Diagnostics)
	}
	state := featureFlagResourceStateModel(t, response.State)
	if state.ID.ValueString() != providerFeatureFlagID || state.Name.ValueString() != "After" ||
		len(state.Variations) != 2 ||
		state.Variations[0].ID.ValueString() != remote.Variations[0].ID ||
		state.Variations[1].ID.ValueString() != remote.Variations[1].ID {
		t.Fatal("Update did not persist UUID-correlated canonical state")
	}
	script.assertComplete(t)
}

func TestFeatureFlagResourceAmbiguousUpdateReconcilesWithoutRetry(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		observedName string
		duplicate    bool
		wantErr      bool
	}{
		"planned name observed": {observedName: "After"},
		"prior name remains":    {observedName: "Before", wantErr: true},
		"duplicate exact key":   {observedName: "After", duplicate: true, wantErr: true},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			const key = "ambiguous-update"
			featureFlagSchema := featureFlagResourceSchema()
			priorState, plan, remote := featureFlagUpdateStateAndPlan(
				t,
				featureFlagSchema,
				key,
				"Before",
				"After",
			)
			remote.Name = test.observedName
			items := []string{featureFlagResourceDefinitionJSON(
				t,
				remote,
				providerFeatureFlagID,
				false,
				true,
			)}
			if test.duplicate {
				items = append(items, featureFlagResourceDefinitionJSON(
					t,
					remote,
					providerFeatureFlagSecondID,
					false,
					false,
				))
			}
			script := &featureFlagHTTPScript{
				t: t,
				expectations: []featureFlagHTTPExpectation{
					{
						method: http.MethodPut,
						path:   featureFlagProviderPath(key) + "/name",
						status: http.StatusServiceUnavailable,
						data:   "null",
					},
					featureFlagCollectionExpectation(false, 0, int64(len(items)), items),
					featureFlagCollectionExpectation(true, 0, 0, []string{}),
				},
			}
			apiClient, closeServer := newProjectResourceTestClient(t, script)
			defer closeServer()
			response := frameworkresource.UpdateResponse{State: priorState}
			(&featureFlagResource{client: apiClient}).Update(
				context.Background(),
				frameworkresource.UpdateRequest{State: priorState, Plan: plan},
				&response,
			)
			if response.Diagnostics.HasError() != test.wantErr {
				t.Fatalf("ambiguous Update diagnostics = %v", response.Diagnostics)
			}
			if test.wantErr {
				if !response.State.Raw.Equal(priorState.Raw) {
					t.Fatal("unconfirmed Update changed Terraform state")
				}
			} else if state := featureFlagResourceStateModel(t, response.State); state.Name.ValueString() != "After" {
				t.Fatal("confirmed ambiguous Update did not persist the observed planned name")
			}
			script.assertComplete(t)
		})
	}
}

func TestFeatureFlagResourceUpdateFailurePreservesState(t *testing.T) {
	t.Parallel()

	const key = "update-failure"
	featureFlagSchema := featureFlagResourceSchema()
	priorState, plan, _ := featureFlagUpdateStateAndPlan(
		t,
		featureFlagSchema,
		key,
		"Before",
		"After",
	)
	script := &featureFlagHTTPScript{
		t: t,
		expectations: []featureFlagHTTPExpectation{
			{
				method: http.MethodPut,
				path:   featureFlagProviderPath(key) + "/name",
				status: http.StatusBadRequest,
				data:   "null",
			},
		},
	}
	apiClient, closeServer := newProjectResourceTestClient(t, script)
	defer closeServer()
	response := frameworkresource.UpdateResponse{State: priorState}
	(&featureFlagResource{client: apiClient}).Update(
		context.Background(),
		frameworkresource.UpdateRequest{State: priorState, Plan: plan},
		&response,
	)
	if !response.Diagnostics.HasError() || !response.State.Raw.Equal(priorState.Raw) {
		t.Fatal("non-reconcilable Update failure changed state")
	}
	script.assertComplete(t)
}

func TestFeatureFlagResourceUpdateConfirmationFailuresPreserveState(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*testing.T, string, canonicalFeatureFlag) []featureFlagHTTPExpectation{
		"canonical read unavailable": func(*testing.T, string, canonicalFeatureFlag) []featureFlagHTTPExpectation {
			return []featureFlagHTTPExpectation{
				{
					method: http.MethodPut,
					path:   featureFlagProviderPath("update-confirmation") + "/name",
					status: http.StatusOK,
					data:   `"` + providerFeatureFlagID + `"`,
				},
				{
					method: http.MethodGet,
					path:   featureFlagProviderPath("update-confirmation"),
					status: http.StatusInternalServerError,
					data:   "null",
				},
				{
					method: http.MethodGet,
					path:   "/api/v1/envs/" + providerEnvironmentA + "/feature-flags",
					query:  "IsArchived=false&PageIndex=0&PageSize=100",
					status: http.StatusInternalServerError,
					data:   "null",
				},
			}
		},
		"became archived": func(t *testing.T, _ string, canonical canonicalFeatureFlag) []featureFlagHTTPExpectation {
			return []featureFlagHTTPExpectation{
				{
					method: http.MethodPut,
					path:   featureFlagProviderPath("update-confirmation") + "/name",
					status: http.StatusOK,
					data:   `"` + providerFeatureFlagID + `"`,
				},
				{
					method: http.MethodGet,
					path:   featureFlagProviderPath("update-confirmation"),
					status: http.StatusOK,
					data: featureFlagResourceDefinitionJSON(
						t,
						canonical,
						providerFeatureFlagID,
						true,
						false,
					),
				},
			}
		},
	}

	for name, expectations := range tests {
		name := name
		expectations := expectations
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			const key = "update-confirmation"
			featureFlagSchema := featureFlagResourceSchema()
			priorState, plan, remote := featureFlagUpdateStateAndPlan(
				t,
				featureFlagSchema,
				key,
				"Before",
				"After",
			)
			script := &featureFlagHTTPScript{
				t:            t,
				expectations: expectations(t, key, remote),
			}
			apiClient, closeServer := newProjectResourceTestClient(t, script)
			defer closeServer()
			response := frameworkresource.UpdateResponse{State: priorState}
			(&featureFlagResource{client: apiClient}).Update(
				context.Background(),
				frameworkresource.UpdateRequest{State: priorState, Plan: plan},
				&response,
			)
			if !response.Diagnostics.HasError() || !response.State.Raw.Equal(priorState.Raw) {
				t.Fatalf("%s did not preserve state: %v", name, response.Diagnostics)
			}
			script.assertComplete(t)
		})
	}
}

func TestFeatureFlagResourceDeleteActiveArchivesDeletesAndProvesLaterPageZero(t *testing.T) {
	t.Parallel()

	const key = "delete-active"
	featureFlagSchema := featureFlagResourceSchema()
	priorState, canonical := featureFlagManagedResourceState(t, featureFlagSchema, key)
	exact := featureFlagResourceDefinitionJSON(t, canonical, providerFeatureFlagID, false, false)
	script := &featureFlagHTTPScript{
		t: t,
		expectations: []featureFlagHTTPExpectation{
			featureFlagCollectionExpectation(false, 0, 1, []string{exact}),
			featureFlagCollectionExpectation(true, 0, 0, []string{}),
			featureFlagBodylessMutationExpectation(http.MethodPut, featureFlagProviderPath(key)+"/archive", http.StatusOK, "true"),
			featureFlagBodylessMutationExpectation(http.MethodDelete, featureFlagProviderPath(key), http.StatusOK, "true"),
			featureFlagCollectionExpectation(false, 0, 2, []string{
				featureFlagResourceListItemJSON(providerFeatureFlagFuzzyIDOne, key+"-fuzzy-one"),
			}),
			featureFlagCollectionExpectation(false, 1, 2, []string{
				featureFlagResourceListItemJSON(providerFeatureFlagFuzzyIDTwo, key+"-fuzzy-two"),
			}),
			featureFlagCollectionExpectation(true, 0, 2, []string{
				featureFlagResourceListItemJSON(providerFeatureFlagFuzzyIDOne, key+"-archived-one"),
			}),
			featureFlagCollectionExpectation(true, 1, 2, []string{
				featureFlagResourceListItemJSON(providerFeatureFlagFuzzyIDTwo, key+"-archived-two"),
			}),
		},
	}
	apiClient, closeServer := newProjectResourceTestClient(t, script)
	defer closeServer()
	response := frameworkresource.DeleteResponse{State: priorState}
	(&featureFlagResource{client: apiClient}).Delete(
		context.Background(),
		frameworkresource.DeleteRequest{State: priorState},
		&response,
	)
	if response.Diagnostics.HasError() || !response.State.Raw.IsNull() {
		t.Fatalf("active Delete diagnostics/state = %v/%v", response.Diagnostics, response.State.Raw)
	}
	script.assertComplete(t)
}

func TestFeatureFlagResourceDeleteAlreadyArchivedAndAbsent(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		key             string
		initialActive   []string
		initialArchived []string
		expectDelete    bool
	}{
		"already archived": {key: "delete-archived", expectDelete: true},
		"already absent":   {key: "delete-absent"},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			key := test.key
			featureFlagSchema := featureFlagResourceSchema()
			priorState, canonical := featureFlagManagedResourceState(t, featureFlagSchema, key)
			if test.expectDelete {
				test.initialArchived = []string{featureFlagResourceDefinitionJSON(
					t,
					canonical,
					providerFeatureFlagID,
					true,
					false,
				)}
			}
			expectations := []featureFlagHTTPExpectation{
				featureFlagCollectionExpectation(false, 0, int64(len(test.initialActive)), test.initialActive),
				featureFlagCollectionExpectation(true, 0, int64(len(test.initialArchived)), test.initialArchived),
			}
			if test.expectDelete {
				expectations = append(expectations,
					featureFlagBodylessMutationExpectation(http.MethodDelete, featureFlagProviderPath(key), http.StatusOK, "true"),
					featureFlagCollectionExpectation(false, 0, 0, []string{}),
					featureFlagCollectionExpectation(true, 0, 0, []string{}),
				)
			}
			script := &featureFlagHTTPScript{t: t, expectations: expectations}
			apiClient, closeServer := newProjectResourceTestClient(t, script)
			defer closeServer()
			response := frameworkresource.DeleteResponse{State: priorState}
			(&featureFlagResource{client: apiClient}).Delete(
				context.Background(),
				frameworkresource.DeleteRequest{State: priorState},
				&response,
			)
			if response.Diagnostics.HasError() || !response.State.Raw.IsNull() {
				t.Fatalf("%s Delete diagnostics = %v", name, response.Diagnostics)
			}
			script.assertComplete(t)
		})
	}
}

func TestFeatureFlagResourceDeleteReconcilesAmbiguousArchiveAndDelete(t *testing.T) {
	t.Parallel()

	t.Run("archive conflict became archived", func(t *testing.T) {
		t.Parallel()
		const key = "archive-conflict"
		featureFlagSchema := featureFlagResourceSchema()
		priorState, canonical := featureFlagManagedResourceState(t, featureFlagSchema, key)
		active := featureFlagResourceDefinitionJSON(t, canonical, providerFeatureFlagID, false, false)
		archived := featureFlagResourceDefinitionJSON(t, canonical, providerFeatureFlagID, true, false)
		script := &featureFlagHTTPScript{
			t: t,
			expectations: []featureFlagHTTPExpectation{
				featureFlagCollectionExpectation(false, 0, 1, []string{active}),
				featureFlagCollectionExpectation(true, 0, 0, []string{}),
				featureFlagBodylessMutationExpectation(http.MethodPut, featureFlagProviderPath(key)+"/archive", http.StatusConflict, "null"),
				featureFlagCollectionExpectation(false, 0, 0, []string{}),
				featureFlagCollectionExpectation(true, 0, 1, []string{archived}),
				featureFlagBodylessMutationExpectation(http.MethodDelete, featureFlagProviderPath(key), http.StatusOK, "true"),
				featureFlagCollectionExpectation(false, 0, 0, []string{}),
				featureFlagCollectionExpectation(true, 0, 0, []string{}),
			},
		}
		apiClient, closeServer := newProjectResourceTestClient(t, script)
		defer closeServer()
		response := frameworkresource.DeleteResponse{State: priorState}
		(&featureFlagResource{client: apiClient}).Delete(
			context.Background(),
			frameworkresource.DeleteRequest{State: priorState},
			&response,
		)
		if response.Diagnostics.HasError() || !response.State.Raw.IsNull() {
			t.Fatalf("reconciled archive conflict diagnostics = %v", response.Diagnostics)
		}
		script.assertComplete(t)
	})

	t.Run("ambiguous delete proved absent", func(t *testing.T) {
		t.Parallel()
		const key = "ambiguous-delete"
		featureFlagSchema := featureFlagResourceSchema()
		priorState, canonical := featureFlagManagedResourceState(t, featureFlagSchema, key)
		archived := featureFlagResourceDefinitionJSON(t, canonical, providerFeatureFlagID, true, false)
		script := &featureFlagHTTPScript{
			t: t,
			expectations: []featureFlagHTTPExpectation{
				featureFlagCollectionExpectation(false, 0, 0, []string{}),
				featureFlagCollectionExpectation(true, 0, 1, []string{archived}),
				featureFlagBodylessMutationExpectation(http.MethodDelete, featureFlagProviderPath(key), http.StatusServiceUnavailable, "null"),
				featureFlagCollectionExpectation(false, 0, 0, []string{}),
				featureFlagCollectionExpectation(true, 0, 0, []string{}),
			},
		}
		apiClient, closeServer := newProjectResourceTestClient(t, script)
		defer closeServer()
		response := frameworkresource.DeleteResponse{State: priorState}
		(&featureFlagResource{client: apiClient}).Delete(
			context.Background(),
			frameworkresource.DeleteRequest{State: priorState},
			&response,
		)
		if response.Diagnostics.HasError() || !response.State.Raw.IsNull() {
			t.Fatalf("reconciled ambiguous delete diagnostics = %v", response.Diagnostics)
		}
		script.assertComplete(t)
	})
}

func TestFeatureFlagResourceDeleteFailuresPreserveState(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*testing.T, string, canonicalFeatureFlag) []featureFlagHTTPExpectation{
		"initial duplicate": func(t *testing.T, key string, canonical canonicalFeatureFlag) []featureFlagHTTPExpectation {
			exact := featureFlagResourceDefinitionJSON(t, canonical, providerFeatureFlagID, false, false)
			duplicate := featureFlagResourceDefinitionJSON(t, canonical, providerFeatureFlagSecondID, false, false)
			return []featureFlagHTTPExpectation{
				featureFlagCollectionExpectation(false, 0, 2, []string{exact, duplicate}),
				featureFlagCollectionExpectation(true, 0, 0, []string{}),
			}
		},
		"initial incomplete": func(*testing.T, string, canonicalFeatureFlag) []featureFlagHTTPExpectation {
			return []featureFlagHTTPExpectation{
				featureFlagCollectionExpectation(false, 0, 1, []string{}),
			}
		},
		"archive validation failure": func(t *testing.T, key string, canonical canonicalFeatureFlag) []featureFlagHTTPExpectation {
			exact := featureFlagResourceDefinitionJSON(t, canonical, providerFeatureFlagID, false, false)
			return []featureFlagHTTPExpectation{
				featureFlagCollectionExpectation(false, 0, 1, []string{exact}),
				featureFlagCollectionExpectation(true, 0, 0, []string{}),
				featureFlagBodylessMutationExpectation(http.MethodPut, featureFlagProviderPath(key)+"/archive", http.StatusBadRequest, "null"),
			}
		},
		"ambiguous archive remains active": func(t *testing.T, key string, canonical canonicalFeatureFlag) []featureFlagHTTPExpectation {
			exact := featureFlagResourceDefinitionJSON(t, canonical, providerFeatureFlagID, false, false)
			return []featureFlagHTTPExpectation{
				featureFlagCollectionExpectation(false, 0, 1, []string{exact}),
				featureFlagCollectionExpectation(true, 0, 0, []string{}),
				featureFlagBodylessMutationExpectation(http.MethodPut, featureFlagProviderPath(key)+"/archive", http.StatusServiceUnavailable, "null"),
				featureFlagCollectionExpectation(false, 0, 1, []string{exact}),
				featureFlagCollectionExpectation(true, 0, 0, []string{}),
			}
		},
		"delete validation failure": func(t *testing.T, key string, canonical canonicalFeatureFlag) []featureFlagHTTPExpectation {
			archived := featureFlagResourceDefinitionJSON(t, canonical, providerFeatureFlagID, true, false)
			return []featureFlagHTTPExpectation{
				featureFlagCollectionExpectation(false, 0, 0, []string{}),
				featureFlagCollectionExpectation(true, 0, 1, []string{archived}),
				featureFlagBodylessMutationExpectation(http.MethodDelete, featureFlagProviderPath(key), http.StatusBadRequest, "null"),
			}
		},
		"ambiguous delete remains archived": func(t *testing.T, key string, canonical canonicalFeatureFlag) []featureFlagHTTPExpectation {
			archived := featureFlagResourceDefinitionJSON(t, canonical, providerFeatureFlagID, true, false)
			return []featureFlagHTTPExpectation{
				featureFlagCollectionExpectation(false, 0, 0, []string{}),
				featureFlagCollectionExpectation(true, 0, 1, []string{archived}),
				featureFlagBodylessMutationExpectation(http.MethodDelete, featureFlagProviderPath(key), http.StatusServiceUnavailable, "null"),
				featureFlagCollectionExpectation(false, 0, 0, []string{}),
				featureFlagCollectionExpectation(true, 0, 1, []string{archived}),
			}
		},
		"successful delete incomplete proof": func(t *testing.T, key string, canonical canonicalFeatureFlag) []featureFlagHTTPExpectation {
			archived := featureFlagResourceDefinitionJSON(t, canonical, providerFeatureFlagID, true, false)
			return []featureFlagHTTPExpectation{
				featureFlagCollectionExpectation(false, 0, 0, []string{}),
				featureFlagCollectionExpectation(true, 0, 1, []string{archived}),
				featureFlagBodylessMutationExpectation(http.MethodDelete, featureFlagProviderPath(key), http.StatusOK, "true"),
				featureFlagCollectionExpectation(false, 0, 1, []string{}),
			}
		},
	}

	for name, expectations := range tests {
		name := name
		expectations := expectations
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			key := "delete-failure"
			featureFlagSchema := featureFlagResourceSchema()
			priorState, canonical := featureFlagManagedResourceState(t, featureFlagSchema, key)
			script := &featureFlagHTTPScript{
				t:            t,
				expectations: expectations(t, key, canonical),
			}
			apiClient, closeServer := newProjectResourceTestClient(t, script)
			defer closeServer()
			response := frameworkresource.DeleteResponse{State: priorState}
			(&featureFlagResource{client: apiClient}).Delete(
				context.Background(),
				frameworkresource.DeleteRequest{State: priorState},
				&response,
			)
			if !response.Diagnostics.HasError() || !response.State.Raw.Equal(priorState.Raw) {
				t.Fatalf("%s did not preserve state: %v", name, response.Diagnostics)
			}
			script.assertComplete(t)
		})
	}
}

func TestFeatureFlagResourceCancellationWhileWaitingForWriteLock(t *testing.T) {
	t.Parallel()

	const key = "lock-wait"
	featureFlagSchema := featureFlagResourceSchema()
	priorState, plan, remote := featureFlagUpdateStateAndPlan(
		t,
		featureFlagSchema,
		key,
		"Before",
		"After",
	)
	script := &featureFlagHTTPScript{
		t: t,
		expectations: []featureFlagHTTPExpectation{
			{
				method: http.MethodPut,
				path:   featureFlagProviderPath(key) + "/name",
				status: http.StatusOK,
				data:   `"` + providerFeatureFlagID + `"`,
			},
			{
				method: http.MethodGet,
				path:   featureFlagProviderPath(key),
				status: http.StatusOK,
				data: featureFlagResourceDefinitionJSON(
					t,
					remote,
					providerFeatureFlagID,
					false,
					false,
				),
			},
		},
	}
	apiClient, closeServer := newProjectResourceTestClient(t, script)
	defer closeServer()
	resourceUnderTest := &featureFlagResource{client: apiClient}
	lockKey := featureFlagWriteLockKey(providerEnvironmentA, key)
	release, err := resourceUnderTest.featureFlagLocks().acquire(context.Background(), lockKey)
	if err != nil {
		t.Fatalf("occupy Feature Flag write lock: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan frameworkresource.UpdateResponse, 1)
	go func() {
		response := frameworkresource.UpdateResponse{State: priorState}
		resourceUnderTest.Update(
			ctx,
			frameworkresource.UpdateRequest{State: priorState, Plan: plan},
			&response,
		)
		result <- response
	}()
	waitForKeyedLockUsers(t, resourceUnderTest.featureFlagLocks(), lockKey, 2)
	cancel()

	var canceled frameworkresource.UpdateResponse
	select {
	case canceled = <-result:
	case <-time.After(2 * time.Second):
		t.Fatal("Feature Flag Update did not return after lock-wait cancellation")
	}
	if !canceled.Diagnostics.HasError() || !canceled.State.Raw.Equal(priorState.Raw) ||
		script.consumed() != 0 {
		t.Fatal("canceled Feature Flag Update changed state or reached transport")
	}
	release()

	progress := frameworkresource.UpdateResponse{State: priorState}
	resourceUnderTest.Update(
		context.Background(),
		frameworkresource.UpdateRequest{State: priorState, Plan: plan},
		&progress,
	)
	if progress.Diagnostics.HasError() {
		t.Fatalf("Feature Flag Update did not progress after cancellation: %v", progress.Diagnostics)
	}
	script.assertComplete(t)
	manager := resourceUnderTest.featureFlagLocks()
	manager.mu.Lock()
	remaining := len(manager.locks)
	manager.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("Feature Flag write lock manager retained %d unused locks", remaining)
	}
}

func TestFeatureFlagWriteLockKeyKeepsExactKeyCaseSensitive(t *testing.T) {
	t.Parallel()

	manager := newKeyedLockManager()
	releaseUpper, err := manager.acquire(
		context.Background(),
		featureFlagWriteLockKey(providerEnvironmentA, "Exact"),
	)
	if err != nil {
		t.Fatalf("acquire uppercase-key lock: %v", err)
	}
	releaseLower, err := manager.acquire(
		context.Background(),
		featureFlagWriteLockKey(providerEnvironmentA, "exact"),
	)
	if err != nil {
		t.Fatalf("case-distinct key was blocked: %v", err)
	}
	releaseLower()
	releaseUpper()
	manager.mu.Lock()
	remaining := len(manager.locks)
	manager.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("case-sensitive lock test retained %d entries", remaining)
	}
}

func featureFlagUpdateStateAndPlan(
	t *testing.T,
	featureFlagSchema resourceschema.Schema,
	key string,
	priorName string,
	plannedName string,
) (tfsdk.State, tfsdk.Plan, canonicalFeatureFlag) {
	t.Helper()
	canonical, _, err := canonicalizePlannedFeatureFlag(
		providerEnvironmentA,
		key,
		priorName,
		"Definition",
		featureFlagVariationTypeString,
		[]featureFlagVariationInput{
			{Name: "One", Value: "one"},
			{Name: "Two", Value: "two"},
		},
	)
	if err != nil {
		t.Fatalf("canonicalize Feature Flag Update fixture: %v", err)
	}
	canonical.ID = providerFeatureFlagID
	priorModel := flattenCanonicalFeatureFlag(canonical)
	priorState := tfsdk.State{Schema: featureFlagSchema}
	if diagnostics := priorState.Set(context.Background(), &priorModel); diagnostics.HasError() {
		t.Fatalf("initialize Feature Flag Update state: %v", diagnostics)
	}
	plannedModel := priorModel
	plannedModel.Name = types.StringValue(plannedName)
	plannedModel.ID = types.StringValue(providerFeatureFlagID)
	plan := featureFlagResourceTestPlan(t, featureFlagSchema, plannedModel)
	canonical.Name = plannedName
	return priorState, plan, canonical
}

func featureFlagProviderPath(key string) string {
	return "/api/v1/envs/" + providerEnvironmentA + "/feature-flags/" + key
}

func featureFlagCollectionExpectation(
	archived bool,
	pageIndex int,
	total int64,
	items []string,
) featureFlagHTTPExpectation {
	return featureFlagHTTPExpectation{
		method: http.MethodGet,
		path:   "/api/v1/envs/" + providerEnvironmentA + "/feature-flags",
		query: fmt.Sprintf(
			"IsArchived=%t&PageIndex=%d&PageSize=100",
			archived,
			pageIndex,
		),
		status: http.StatusOK,
		data:   featureFlagResourcePageJSON(total, items),
	}
}

func featureFlagBodylessMutationExpectation(
	method string,
	path string,
	status int,
	data string,
) featureFlagHTTPExpectation {
	return featureFlagHTTPExpectation{
		method: method,
		path:   path,
		status: status,
		data:   data,
		checkBody: func(t *testing.T, request *http.Request) {
			if request.Body != nil && request.Body != http.NoBody ||
				request.Header.Get("Content-Type") != "" {
				t.Fatal("archive/delete mutation sent an optional comment or another body")
			}
		},
	}
}

func waitForKeyedLockUsers(
	t *testing.T,
	manager *keyedLockManager,
	key string,
	want int,
) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		manager.mu.Lock()
		entry := manager.locks[key]
		users := 0
		if entry != nil {
			users = entry.users
		}
		manager.mu.Unlock()
		if users == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("keyed lock did not reach %d users", want)
}
