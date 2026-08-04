// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSegmentResourceUpdateSendsOnlyFrozenCanonicalDiffInStableOrder(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mutatePlan func(*segmentModel)
		components []string
	}{
		"unchanged definition reads without mutation": {},
		"name only": {
			mutatePlan: func(plan *segmentModel) {
				plan.Name = types.StringValue("Synthetic renamed Segment")
			},
			components: []string{"name"},
		},
		"description only": {
			mutatePlan: func(plan *segmentModel) {
				plan.Description = types.StringValue("Synthetic updated description")
			},
			components: []string{"description"},
		},
		"targeting only": {
			mutatePlan: func(plan *segmentModel) {
				plan.IncludedUsers = terraformStringSetValue([]string{"user-a", "user-new"})
			},
			components: []string{"targeting"},
		},
		"tags only": {
			mutatePlan: func(plan *segmentModel) {
				plan.Tags = terraformStringSetValue([]string{"tag-a", "tag-new"})
			},
			components: []string{"tags"},
		},
		"combined update uses frozen order": {
			mutatePlan: func(plan *segmentModel) {
				plan.Name = types.StringValue("Synthetic combined name")
				plan.Description = types.StringValue("Synthetic combined description")
				plan.ExcludedUsers = terraformStringSetValue([]string{"user-new-excluded"})
				plan.Tags = terraformStringSetValue([]string{"tag-combined"})
			},
			components: []string{"name", "description", "targeting", "tags"},
		},
		"set reordering is canonical no-op": {
			mutatePlan: func(plan *segmentModel) {
				plan.IncludedUsers = terraformStringSetValue([]string{"user-a", "user-z"})
				plan.ExcludedUsers = terraformStringSetValue([]string{"user-y"})
				plan.Tags = terraformStringSetValue([]string{"tag-a", "tag-z"})
			},
		},
		"rule reordering preserves evaluation semantics": {
			mutatePlan: func(plan *segmentModel) {
				plan.Rules[0], plan.Rules[1] = plan.Rules[1], plan.Rules[0]
			},
			components: []string{"targeting"},
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			priorModel := providerSegmentResourceStateModel()
			planModel := providerSegmentResourceStateModel()
			if test.mutatePlan != nil {
				test.mutatePlan(&planModel)
			}
			planned := canonicalSegmentUpdateModel(t, planModel)
			expectations := segmentUpdateMutationExpectations(t, planned, test.components...)
			expectations = append(expectations, segmentExactExpectation(t, planned))
			script := &segmentHTTPScript{t: t, expectations: expectations}
			apiClient, closeServer := newProjectResourceTestClient(t, script)
			defer closeServer()

			segmentSchema := segmentResourceSchema()
			priorState := segmentResourceTestState(t, segmentSchema, priorModel)
			response := frameworkresource.UpdateResponse{State: priorState}
			(&segmentResource{client: apiClient}).Update(
				context.Background(),
				frameworkresource.UpdateRequest{
					State: priorState,
					Plan:  segmentResourceTestPlan(t, segmentSchema, planModel),
				},
				&response,
			)
			if response.Diagnostics.HasError() {
				t.Fatalf("Update() diagnostics = %v", response.Diagnostics)
			}
			assertSegmentUpdateState(t, response.State, planned)
			state := segmentResourceStateModel(t, response.State)
			if len(state.Rules) != len(planModel.Rules) ||
				state.Rules[0].Conditions[0].Value.ValueString() !=
					planModel.Rules[0].Conditions[0].Value.ValueString() {
				t.Fatal("Update() did not preserve semantically equivalent configured condition values")
			}
			script.assertComplete(t)
		})
	}
}

func TestSegmentResourceUpdateReconcilesAmbiguityAndPreservesPartialSuccess(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mutatePlan   func(*segmentModel)
		expectations func(*testing.T, canonicalSegment, canonicalSegment) []segmentHTTPExpectation
		wantState    func(canonicalSegment, canonicalSegment) canonicalSegment
		wantError    bool
	}{
		"ambiguous name observed continues without replay": {
			mutatePlan: func(plan *segmentModel) {
				plan.Name = types.StringValue("Synthetic reconciled name")
				plan.Description = types.StringValue("Synthetic reconciled description")
			},
			expectations: func(t *testing.T, prior canonicalSegment, planned canonicalSegment) []segmentHTTPExpectation {
				nameConfirmed := prior
				nameConfirmed.Name = planned.Name
				return []segmentHTTPExpectation{
					segmentUpdateMutationExpectation(t, planned, "name", http.StatusServiceUnavailable, "null"),
					segmentExactExpectation(t, nameConfirmed),
					segmentUpdateMutationExpectation(t, planned, "description", http.StatusOK, "true"),
					segmentExactExpectation(t, planned),
				}
			},
			wantState: func(_ canonicalSegment, planned canonicalSegment) canonicalSegment { return planned },
		},
		"ambiguous targeting not observed stops without replay": {
			mutatePlan: func(plan *segmentModel) {
				plan.IncludedUsers = terraformStringSetValue([]string{"user-unconfirmed"})
			},
			expectations: func(t *testing.T, prior canonicalSegment, planned canonicalSegment) []segmentHTTPExpectation {
				return []segmentHTTPExpectation{
					segmentUpdateMutationExpectation(t, planned, "targeting", http.StatusServiceUnavailable, "null"),
					segmentExactExpectation(t, prior),
				}
			},
			wantState: func(prior canonicalSegment, _ canonicalSegment) canonicalSegment { return prior },
			wantError: true,
		},
		"confirmed name survives later deterministic failure": {
			mutatePlan: func(plan *segmentModel) {
				plan.Name = types.StringValue("Synthetic partial name")
				plan.Description = types.StringValue("Synthetic rejected description")
				plan.Tags = terraformStringSetValue([]string{"tag-not-reached"})
			},
			expectations: func(t *testing.T, _ canonicalSegment, planned canonicalSegment) []segmentHTTPExpectation {
				return []segmentHTTPExpectation{
					segmentUpdateMutationExpectation(t, planned, "name", http.StatusOK, "true"),
					segmentUpdateMutationExpectation(t, planned, "description", http.StatusBadRequest, "null"),
				}
			},
			wantState: func(prior canonicalSegment, planned canonicalSegment) canonicalSegment {
				prior.Name = planned.Name
				return prior
			},
			wantError: true,
		},
		"final read failure retains every mutation-confirmed component": {
			mutatePlan: func(plan *segmentModel) {
				plan.Name = types.StringValue("Synthetic confirmed name")
				plan.Tags = terraformStringSetValue([]string{"tag-confirmed"})
			},
			expectations: func(t *testing.T, _ canonicalSegment, planned canonicalSegment) []segmentHTTPExpectation {
				return []segmentHTTPExpectation{
					segmentUpdateMutationExpectation(t, planned, "name", http.StatusOK, "true"),
					segmentUpdateMutationExpectation(t, planned, "tags", http.StatusOK, "true"),
					{
						method: http.MethodGet,
						path:   segmentResourceExactPath(planned.ID),
						status: http.StatusInternalServerError,
						data:   "null",
					},
				}
			},
			wantState: func(_ canonicalSegment, planned canonicalSegment) canonicalSegment { return planned },
			wantError: true,
		},
		"final read mismatch replaces provisional state with server form": {
			mutatePlan: func(plan *segmentModel) {
				plan.Name = types.StringValue("Synthetic final mismatch")
			},
			expectations: func(t *testing.T, prior canonicalSegment, planned canonicalSegment) []segmentHTTPExpectation {
				return []segmentHTTPExpectation{
					segmentUpdateMutationExpectation(t, planned, "name", http.StatusOK, "true"),
					segmentExactExpectation(t, prior),
				}
			},
			wantState: func(prior canonicalSegment, _ canonicalSegment) canonicalSegment { return prior },
			wantError: true,
		},
		"ambiguous tag observed still receives final canonical read": {
			mutatePlan: func(plan *segmentModel) {
				plan.Tags = terraformStringSetValue([]string{"tag-reconciled"})
			},
			expectations: func(t *testing.T, _ canonicalSegment, planned canonicalSegment) []segmentHTTPExpectation {
				return []segmentHTTPExpectation{
					segmentUpdateMutationExpectation(t, planned, "tags", http.StatusServiceUnavailable, "null"),
					segmentExactExpectation(t, planned),
					segmentExactExpectation(t, planned),
				}
			},
			wantState: func(_ canonicalSegment, planned canonicalSegment) canonicalSegment { return planned },
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			priorModel := providerSegmentResourceStateModel()
			planModel := providerSegmentResourceStateModel()
			test.mutatePlan(&planModel)
			prior := canonicalSegmentUpdateModel(t, priorModel)
			planned := canonicalSegmentUpdateModel(t, planModel)
			script := &segmentHTTPScript{
				t:            t,
				expectations: test.expectations(t, prior, planned),
			}
			apiClient, closeServer := newProjectResourceTestClient(t, script)
			defer closeServer()
			segmentSchema := segmentResourceSchema()
			priorState := segmentResourceTestState(t, segmentSchema, priorModel)
			response := frameworkresource.UpdateResponse{State: priorState}
			(&segmentResource{client: apiClient}).Update(
				context.Background(),
				frameworkresource.UpdateRequest{
					State: priorState,
					Plan:  segmentResourceTestPlan(t, segmentSchema, planModel),
				},
				&response,
			)
			if response.Diagnostics.HasError() != test.wantError {
				t.Fatalf("Update() diagnostics = %v", response.Diagnostics)
			}
			assertSegmentUpdateState(t, response.State, test.wantState(prior, planned))
			script.assertComplete(t)
		})
	}
}

func TestSegmentResourceUpdateRejectsUnsafeStateAndPlanBeforeTransport(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*segmentModel, *segmentModel){
		"shared prior state": func(prior *segmentModel, _ *segmentModel) {
			prior.Type = types.StringValue(string(client.SegmentTypeShared))
			prior.Scopes = terraformStringSetValue([]string{"organization/synthetic-organization"})
		},
		"shared plan": func(_ *segmentModel, plan *segmentModel) {
			plan.Type = types.StringValue(string(client.SegmentTypeShared))
			plan.Scopes = terraformStringSetValue([]string{"organization/synthetic-organization"})
		},
		"scope mismatch": func(_ *segmentModel, plan *segmentModel) {
			plan.Scopes = terraformStringSetValue([]string{
				"organization/synthetic-organization:project/synthetic-project:env/synthetic-other-environment",
			})
		},
		"segment identity mismatch": func(_ *segmentModel, plan *segmentModel) {
			plan.ID = types.StringValue(providerSegmentCreatedID)
		},
	}

	for name, mutate := range tests {
		name := name
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var requests atomic.Int32
			apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
				func(http.ResponseWriter, *http.Request) { requests.Add(1) },
			))
			defer closeServer()
			priorModel := providerSegmentResourceStateModel()
			planModel := providerSegmentResourceStateModel()
			mutate(&priorModel, &planModel)
			segmentSchema := segmentResourceSchema()
			priorState := segmentResourceTestState(t, segmentSchema, priorModel)
			response := frameworkresource.UpdateResponse{State: priorState}
			(&segmentResource{client: apiClient}).Update(
				context.Background(),
				frameworkresource.UpdateRequest{
					State: priorState,
					Plan:  segmentResourceTestPlan(t, segmentSchema, planModel),
				},
				&response,
			)
			if !response.Diagnostics.HasError() || !response.State.Raw.Equal(priorState.Raw) ||
				requests.Load() != 0 {
				t.Fatalf("unsafe Update diagnostics/state/requests = %v/%t/%d", response.Diagnostics, response.State.Raw.Equal(priorState.Raw), requests.Load())
			}
		})
	}
}

func TestSegmentResourceUpdateRejectsRemoteSharedOrScopeDriftDuringReconciliation(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		remote    func(canonicalSegment) canonicalSegment
		wantPrior bool
	}{
		"shared segment": {
			remote: func(segment canonicalSegment) canonicalSegment {
				segment.Type = client.SegmentTypeShared
				segment.Scopes = []string{"organization/synthetic-organization"}
				return segment
			},
			wantPrior: true,
		},
		"environment scope drift": {
			remote: func(segment canonicalSegment) canonicalSegment {
				segment.Scopes = []string{
					"organization/synthetic-organization:project/synthetic-project:env/synthetic-other-environment",
				}
				return segment
			},
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			priorModel := providerSegmentResourceStateModel()
			planModel := providerSegmentResourceStateModel()
			planModel.Name = types.StringValue("Synthetic unsafe reconciliation")
			prior := canonicalSegmentUpdateModel(t, priorModel)
			planned := canonicalSegmentUpdateModel(t, planModel)
			remote := test.remote(prior)
			script := &segmentHTTPScript{t: t, expectations: []segmentHTTPExpectation{
				segmentUpdateMutationExpectation(t, planned, "name", http.StatusServiceUnavailable, "null"),
				segmentExactExpectation(t, remote),
			}}
			apiClient, closeServer := newProjectResourceTestClient(t, script)
			defer closeServer()
			segmentSchema := segmentResourceSchema()
			priorState := segmentResourceTestState(t, segmentSchema, priorModel)
			response := frameworkresource.UpdateResponse{State: priorState}
			(&segmentResource{client: apiClient}).Update(
				context.Background(),
				frameworkresource.UpdateRequest{
					State: priorState,
					Plan:  segmentResourceTestPlan(t, segmentSchema, planModel),
				},
				&response,
			)
			if !response.Diagnostics.HasError() {
				t.Fatal("unsafe reconciliation produced no diagnostic")
			}
			if test.wantPrior {
				if !response.State.Raw.Equal(priorState.Raw) {
					t.Fatal("shared reconciliation changed prior managed state")
				}
			} else {
				assertSegmentUpdateState(t, response.State, remote)
			}
			script.assertComplete(t)
		})
	}
}

func TestSegmentResourceUpdateCancellationWhileWaitingForWriteLockSendsNoRequest(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) { requests.Add(1) },
	))
	defer closeServer()
	priorModel := providerSegmentResourceStateModel()
	planModel := providerSegmentResourceStateModel()
	planModel.Name = types.StringValue("Synthetic canceled update")
	planned := canonicalSegmentUpdateModel(t, planModel)
	manager := newKeyedLockManager()
	lockKey := segmentWriteLockKey(planned.EnvironmentID, planned.ID)
	release, err := manager.acquire(context.Background(), lockKey)
	if err != nil {
		t.Fatalf("acquire Segment update lock: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	segmentSchema := segmentResourceSchema()
	priorState := segmentResourceTestState(t, segmentSchema, priorModel)
	response := frameworkresource.UpdateResponse{State: priorState}
	(&segmentResource{client: apiClient, locks: manager}).Update(
		ctx,
		frameworkresource.UpdateRequest{
			State: priorState,
			Plan:  segmentResourceTestPlan(t, segmentSchema, planModel),
		},
		&response,
	)
	release()
	if !response.Diagnostics.HasError() || !response.State.Raw.Equal(priorState.Raw) ||
		requests.Load() != 0 {
		t.Fatalf("canceled Update diagnostics/state/requests = %v/%t/%d", response.Diagnostics, response.State.Raw.Equal(priorState.Raw), requests.Load())
	}
	manager.mu.Lock()
	remaining := len(manager.locks)
	manager.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("canceled Segment update retained %d lock entries", remaining)
	}
	if segmentWriteLockKey(strings.ToUpper(planned.EnvironmentID), strings.ToUpper(planned.ID)) != lockKey {
		t.Fatal("Segment write lock did not canonicalize exact UUID identity")
	}
}

func TestSegmentResourceUpdateDiagnosticsRedactRuntimeDefinition(t *testing.T) {
	t.Parallel()

	const runtimeMarker = "segment-update-runtime-marker"
	var requests atomic.Int32
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			requests.Add(1)
			if request.Method != http.MethodPut {
				t.Error("redaction failure test received a non-mutation request")
			}
			response.Header().Set("Content-Type", "application/json")
			response.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(
				response,
				`{"success":false,"data":null,"errors":[%q]}`,
				strings.Join([]string{runtimeMarker, providerEnvironmentA, providerSegmentID}, " | "),
			)
		},
	))
	defer closeServer()
	priorModel := providerSegmentResourceStateModel()
	planModel := providerSegmentResourceStateModel()
	planModel.IncludedUsers = terraformStringSetValue([]string{runtimeMarker})
	segmentSchema := segmentResourceSchema()
	priorState := segmentResourceTestState(t, segmentSchema, priorModel)
	response := frameworkresource.UpdateResponse{State: priorState}
	(&segmentResource{client: apiClient}).Update(
		context.Background(),
		frameworkresource.UpdateRequest{
			State: priorState,
			Plan:  segmentResourceTestPlan(t, segmentSchema, planModel),
		},
		&response,
	)
	if !response.Diagnostics.HasError() || !response.State.Raw.Equal(priorState.Raw) ||
		requests.Load() != 1 {
		t.Fatalf("redaction Update diagnostics/state/requests = %v/%t/%d", response.Diagnostics, response.State.Raw.Equal(priorState.Raw), requests.Load())
	}
	formatted := fmt.Sprintf("%v", response.Diagnostics)
	for _, unsafe := range []string{runtimeMarker, providerEnvironmentA, providerSegmentID} {
		if strings.Contains(formatted, unsafe) {
			t.Fatal("Segment Update diagnostic exposed a runtime identity or targeting value")
		}
	}
}

func segmentUpdateMutationExpectations(
	t *testing.T,
	planned canonicalSegment,
	components ...string,
) []segmentHTTPExpectation {
	t.Helper()
	expectations := make([]segmentHTTPExpectation, 0, len(components))
	for _, component := range components {
		expectations = append(
			expectations,
			segmentUpdateMutationExpectation(t, planned, component, http.StatusOK, "true"),
		)
	}
	return expectations
}

func segmentUpdateMutationExpectation(
	t *testing.T,
	planned canonicalSegment,
	component string,
	status int,
	data string,
) segmentHTTPExpectation {
	t.Helper()
	var body any
	switch component {
	case "name":
		body = client.UpdateSegmentNameRequest{Name: planned.Name}
	case "description":
		body = client.UpdateSegmentDescriptionRequest{Description: planned.Description}
	case "targeting":
		body = expandSegmentTargetingRequest(planned)
	case "tags":
		body = client.UpdateSegmentTagsRequest{Tags: append([]string(nil), planned.Tags...)}
	default:
		t.Fatalf("unknown Segment update component %q", component)
	}
	return segmentHTTPExpectation{
		method: http.MethodPut,
		path:   segmentResourceExactPath(planned.ID) + "/" + component,
		status: status,
		data:   data,
		checkBody: func(t *testing.T, request *http.Request) {
			assertProviderSegmentJSONBody(t, request, body)
		},
	}
}

func canonicalSegmentUpdateModel(t *testing.T, model segmentModel) canonicalSegment {
	t.Helper()
	canonical, err := canonicalizeSegmentStateModel(context.Background(), model)
	if err != nil {
		t.Fatalf("canonicalize Segment update model: %v", err)
	}
	return canonical
}

func assertSegmentUpdateState(
	t *testing.T,
	state tfsdk.State,
	want canonicalSegment,
) {
	t.Helper()
	model := segmentResourceStateModel(t, state)
	got, err := canonicalizeSegmentStateModel(context.Background(), model)
	if err != nil || !sameSegmentDefinition(got, want) {
		t.Fatal("Segment Update state did not match the expected recoverable definition")
	}
}
