// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	providerSegmentCreatedID = "12121212-1212-4121-8121-121212121212"
	providerSegmentFuzzyID   = "23232323-2323-4232-8232-232323232323"
)

type segmentHTTPExpectation struct {
	method    string
	path      string
	query     string
	status    int
	data      string
	checkBody func(*testing.T, *http.Request)
}

type segmentHTTPScript struct {
	t            *testing.T
	mu           sync.Mutex
	expectations []segmentHTTPExpectation
	next         int
}

func (s *segmentHTTPScript) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	s.mu.Lock()
	if s.next >= len(s.expectations) {
		s.mu.Unlock()
		s.t.Errorf(
			"unexpected Segment request %s %s?%s",
			request.Method,
			request.URL.EscapedPath(),
			request.URL.RawQuery,
		)
		writeProjectResourceEnvelope(s.t, response, http.StatusInternalServerError, "null")
		return
	}
	expectation := s.expectations[s.next]
	s.next++
	s.mu.Unlock()

	if request.Method != expectation.method || request.URL.EscapedPath() != expectation.path ||
		request.URL.RawQuery != expectation.query {
		s.t.Errorf(
			"Segment request = %s %s?%s, want %s %s?%s",
			request.Method,
			request.URL.EscapedPath(),
			request.URL.RawQuery,
			expectation.method,
			expectation.path,
			expectation.query,
		)
	}
	assertProviderSegmentRequestHasNoContextHeaders(s.t, request)
	if expectation.checkBody != nil {
		expectation.checkBody(s.t, request)
	}
	writeProjectResourceEnvelope(s.t, response, expectation.status, expectation.data)
}

func (s *segmentHTTPScript) consumed() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.next
}

func (s *segmentHTTPScript) assertComplete(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.next != len(s.expectations) {
		t.Fatalf("Segment script consumed %d/%d requests", s.next, len(s.expectations))
	}
}

func TestSegmentResourceMetadataSchemaConfigureAndRegistration(t *testing.T) {
	t.Parallel()

	resourceUnderTest := &segmentResource{}
	var metadata frameworkresource.MetadataResponse
	resourceUnderTest.Metadata(
		context.Background(),
		frameworkresource.MetadataRequest{ProviderTypeName: "featbit"},
		&metadata,
	)
	if metadata.TypeName != "featbit_segment" {
		t.Fatalf("Metadata type name = %q", metadata.TypeName)
	}

	var schemaResponse frameworkresource.SchemaResponse
	resourceUnderTest.Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		&schemaResponse,
	)
	if schemaResponse.Diagnostics.HasError() || len(schemaResponse.Schema.Attributes) != 11 {
		t.Fatalf("Segment resource schema diagnostics/attributes = %v/%d", schemaResponse.Diagnostics, len(schemaResponse.Schema.Attributes))
	}
	if _, ok := newSegmentResource().(*segmentResource); !ok {
		t.Fatalf("newSegmentResource() type = %T", newSegmentResource())
	}

	configuredClient, closeServer := newProjectResourceTestClient(t, http.NotFoundHandler())
	defer closeServer()
	var configureResponse frameworkresource.ConfigureResponse
	resourceUnderTest.Configure(
		context.Background(),
		frameworkresource.ConfigureRequest{ProviderData: configuredClient},
		&configureResponse,
	)
	if configureResponse.Diagnostics.HasError() || resourceUnderTest.client != configuredClient {
		t.Fatalf("Configure diagnostics/client = %v/%p", configureResponse.Diagnostics, resourceUnderTest.client)
	}

	providerResources := New("test")().Resources(context.Background())
	if len(providerResources) != 6 {
		t.Fatalf("registered resource count = %d, want 6", len(providerResources))
	}
	registered := false
	for _, factory := range providerResources {
		if _, ok := factory().(*segmentResource); ok {
			registered = true
		}
	}
	if !registered {
		t.Fatal("provider registration omitted featbit_segment")
	}
}

func TestSegmentResourceModifyPlanCanonicalizesAndPreservesStableIdentities(t *testing.T) {
	t.Parallel()

	segmentSchema := segmentResourceSchema()
	basePlan := providerSegmentPlanModel()
	tests := map[string]struct {
		state           tfsdk.State
		plan            segmentModel
		wantIDUnknown   bool
		wantSegmentID   string
		wantRuleID      string
		wantConditionID string
	}{
		"create uses deterministic identities": {
			state:           tfsdk.State{Schema: segmentSchema},
			plan:            basePlan,
			wantIDUnknown:   true,
			wantRuleID:      deterministicSegmentRuleID(providerEnvironmentA, "synthetic-segment", 0),
			wantConditionID: deterministicSegmentConditionID(providerEnvironmentA, "synthetic-segment", 0, 0),
		},
		"in-place plan preserves imported identities": {
			state:           segmentResourceTestState(t, segmentSchema, providerSegmentResourceStateModel()),
			plan:            basePlan,
			wantSegmentID:   providerSegmentID,
			wantRuleID:      providerSegmentRuleOne,
			wantConditionID: providerSegmentConditionOne,
		},
		"replacement leaves new identity unknown": {
			state: segmentResourceTestState(t, segmentSchema, providerSegmentResourceStateModel()),
			plan: func() segmentModel {
				model := providerSegmentPlanModel()
				model.Key = types.StringValue("replacement-segment")
				return model
			}(),
			wantIDUnknown:   true,
			wantRuleID:      deterministicSegmentRuleID(providerEnvironmentA, "replacement-segment", 0),
			wantConditionID: deterministicSegmentConditionID(providerEnvironmentA, "replacement-segment", 0, 0),
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			plan := segmentResourceTestPlan(t, segmentSchema, test.plan)
			response := frameworkresource.ModifyPlanResponse{
				Plan: tfsdk.Plan{Schema: segmentSchema},
			}
			(&segmentResource{}).ModifyPlan(
				context.Background(),
				frameworkresource.ModifyPlanRequest{Plan: plan, State: test.state},
				&response,
			)
			if response.Diagnostics.HasError() {
				t.Fatalf("ModifyPlan() diagnostics = %v", response.Diagnostics)
			}
			var got segmentModel
			if diagnostics := response.Plan.Get(context.Background(), &got); diagnostics.HasError() {
				t.Fatalf("read modified Segment plan: %v", diagnostics)
			}
			if got.Type.ValueString() != string(client.SegmentTypeEnvironmentSpecific) ||
				got.Rules[0].ID.ValueString() != test.wantRuleID ||
				got.Rules[0].Conditions[0].ID.ValueString() != test.wantConditionID {
				t.Fatal("ModifyPlan() did not establish the expected type and stable targeting identities")
			}
			if test.wantIDUnknown != got.ID.IsUnknown() ||
				(test.wantSegmentID != "" && got.ID.ValueString() != test.wantSegmentID) {
				t.Fatalf("modified Segment ID = %#v", got.ID)
			}
			if got.Rules[0].Conditions[0].Value.ValueString() !=
				test.plan.Rules[0].Conditions[0].Value.ValueString() ||
				got.Rules[0].Conditions[1].Value.ValueString() !=
					test.plan.Rules[0].Conditions[1].Value.ValueString() {
				t.Fatal("ModifyPlan() rewrote required condition configuration values")
			}
		})
	}
}

func TestSegmentResourceModifyPlanRejectsMixedIdentityCollision(t *testing.T) {
	t.Parallel()

	segmentSchema := segmentResourceSchema()
	prior := providerSegmentResourceStateModel()
	prior.Rules[0].ID = types.StringValue(
		deterministicSegmentRuleID(providerEnvironmentA, "synthetic-segment", 2),
	)
	plan := providerSegmentPlanModel()
	plan.Rules = append(plan.Rules, segmentRuleModel{
		ID:   types.StringUnknown(),
		Name: types.StringValue("Third"),
		Conditions: []segmentConditionModel{{
			ID:       types.StringUnknown(),
			Property: types.StringValue("tier"),
			Operator: types.StringValue(segmentOperatorEqual),
			Value:    types.StringValue("synthetic-value"),
		}},
	})
	response := frameworkresource.ModifyPlanResponse{
		Plan: tfsdk.Plan{Schema: segmentSchema},
	}
	(&segmentResource{}).ModifyPlan(
		context.Background(),
		frameworkresource.ModifyPlanRequest{
			Plan:  segmentResourceTestPlan(t, segmentSchema, plan),
			State: segmentResourceTestState(t, segmentSchema, prior),
		},
		&response,
	)
	if !response.Diagnostics.HasError() {
		t.Fatal("ModifyPlan() accepted a mixed imported/deterministic rule UUID collision")
	}
}

func TestSegmentResourceCreateInitializesOnlyPlannedTargetingAndTags(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		empty bool
	}{
		"empty targeting and tags":     {empty: true},
		"non-empty targeting and tags": {},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			model := providerSegmentPlanModel()
			if test.empty {
				model.IncludedUsers = terraformStringSetValue([]string{})
				model.ExcludedUsers = terraformStringSetValue([]string{})
				model.Rules = nil
				model.Tags = terraformStringSetValue([]string{})
			}
			planned, err := canonicalizeSegmentPlanModel(context.Background(), model)
			if err != nil {
				t.Fatalf("canonicalize Segment Create fixture: %v", err)
			}
			planned.ID = providerSegmentCreatedID

			created := planned
			created.IncludedUsers = []string{}
			created.ExcludedUsers = []string{}
			created.Rules = []canonicalSegmentRule{}
			created.Tags = []string{}
			expectations := []segmentHTTPExpectation{
				segmentCollectionExpectation(false, 1, []string{
					segmentResourceListItemJSON(providerSegmentFuzzyID, planned.Key+"-fuzzy"),
				}),
				segmentCollectionExpectation(true, 0, []string{}),
				{
					method: http.MethodPost,
					path:   segmentResourceCollectionPath(),
					status: http.StatusOK,
					data:   segmentResourceDefinitionJSON(t, created, false),
					checkBody: func(t *testing.T, request *http.Request) {
						assertProviderSegmentJSONBody(t, request, expandSegmentCreateRequest(planned))
					},
				},
				segmentExactExpectation(t, created),
			}

			current := created
			if !test.empty {
				targeted := planned
				targeted.Tags = []string{}
				expectations = append(expectations,
					segmentHTTPExpectation{
						method: http.MethodPut,
						path:   segmentResourceExactPath(providerSegmentCreatedID) + "/targeting",
						status: http.StatusOK,
						data:   "true",
						checkBody: func(t *testing.T, request *http.Request) {
							assertProviderSegmentJSONBody(t, request, expandSegmentTargetingRequest(planned))
						},
					},
					segmentExactExpectation(t, targeted),
				)
				current = targeted
				expectations = append(expectations,
					segmentHTTPExpectation{
						method: http.MethodPut,
						path:   segmentResourceExactPath(providerSegmentCreatedID) + "/tags",
						status: http.StatusOK,
						data:   "true",
						checkBody: func(t *testing.T, request *http.Request) {
							assertProviderSegmentJSONBody(
								t,
								request,
								client.UpdateSegmentTagsRequest{Tags: planned.Tags},
							)
						},
					},
					segmentExactExpectation(t, planned),
				)
				current = planned
			}

			script := &segmentHTTPScript{t: t, expectations: expectations}
			apiClient, closeServer := newProjectResourceTestClient(t, script)
			defer closeServer()
			segmentSchema := segmentResourceSchema()
			response := frameworkresource.CreateResponse{
				State: emptySegmentResourceState(t, segmentSchema),
			}
			(&segmentResource{client: apiClient}).Create(
				context.Background(),
				frameworkresource.CreateRequest{
					Plan: segmentResourceTestPlan(t, segmentSchema, model),
				},
				&response,
			)
			if response.Diagnostics.HasError() {
				t.Fatalf("Create() diagnostics = %v", response.Diagnostics)
			}
			script.assertComplete(t)

			stateModel := segmentResourceStateModel(t, response.State)
			canonicalState, err := canonicalizeSegmentStateModel(context.Background(), stateModel)
			if err != nil || !sameSegmentDefinition(current, canonicalState) {
				t.Fatal("Create() did not persist the complete canonical Segment state")
			}
			if !test.empty &&
				stateModel.Rules[0].Conditions[1].Value.ValueString() !=
					model.Rules[0].Conditions[1].Value.ValueString() {
				t.Fatal("Create() did not preserve an equivalent configured condition value")
			}
		})
	}
}

func TestSegmentResourceCreateClearsUnexpectedInitialTargetingAndTags(t *testing.T) {
	t.Parallel()

	model := providerSegmentPlanModel()
	model.IncludedUsers = terraformStringSetValue([]string{})
	model.ExcludedUsers = terraformStringSetValue([]string{})
	model.Rules = nil
	model.Tags = terraformStringSetValue([]string{})
	planned, err := canonicalizeSegmentPlanModel(context.Background(), model)
	if err != nil {
		t.Fatalf("canonicalize empty Segment fixture: %v", err)
	}
	planned.ID = providerSegmentCreatedID
	created := planned
	created.IncludedUsers = []string{"unexpected-user"}
	created.Tags = []string{"unexpected-tag"}
	targeted := planned
	targeted.Tags = append([]string{}, created.Tags...)

	script := &segmentHTTPScript{
		t: t,
		expectations: []segmentHTTPExpectation{
			segmentCollectionExpectation(false, 0, []string{}),
			segmentCollectionExpectation(true, 0, []string{}),
			{
				method: http.MethodPost,
				path:   segmentResourceCollectionPath(),
				status: http.StatusOK,
				data:   segmentResourceDefinitionJSON(t, created, false),
			},
			segmentExactExpectation(t, created),
			{
				method: http.MethodPut,
				path:   segmentResourceExactPath(providerSegmentCreatedID) + "/targeting",
				status: http.StatusOK,
				data:   "true",
				checkBody: func(t *testing.T, request *http.Request) {
					assertProviderSegmentJSONBody(t, request, client.UpdateSegmentTargetingRequest{
						Included: []string{}, Excluded: []string{}, Rules: []client.SegmentRule{},
					})
				},
			},
			segmentExactExpectation(t, targeted),
			{
				method: http.MethodPut,
				path:   segmentResourceExactPath(providerSegmentCreatedID) + "/tags",
				status: http.StatusOK,
				data:   "true",
				checkBody: func(t *testing.T, request *http.Request) {
					assertProviderSegmentJSONBody(
						t,
						request,
						client.UpdateSegmentTagsRequest{Tags: []string{}},
					)
				},
			},
			segmentExactExpectation(t, planned),
		},
	}
	apiClient, closeServer := newProjectResourceTestClient(t, script)
	defer closeServer()
	segmentSchema := segmentResourceSchema()
	response := frameworkresource.CreateResponse{
		State: emptySegmentResourceState(t, segmentSchema),
	}
	(&segmentResource{client: apiClient}).Create(
		context.Background(),
		frameworkresource.CreateRequest{
			Plan: segmentResourceTestPlan(t, segmentSchema, model),
		},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("empty canonicalization Create diagnostics = %v", response.Diagnostics)
	}
	script.assertComplete(t)
	state := segmentResourceStateModel(t, response.State)
	canonicalState, err := canonicalizeSegmentStateModel(context.Background(), state)
	if err != nil || !sameSegmentDefinition(planned, canonicalState) {
		t.Fatal("empty targeting/tag initialization did not clear unexpected server defaults")
	}
}

func TestSegmentResourceCreateRecoversPartialResponseByExactUUIDRead(t *testing.T) {
	t.Parallel()

	model := providerSegmentPlanModel()
	model.IncludedUsers = terraformStringSetValue([]string{})
	model.ExcludedUsers = terraformStringSetValue([]string{})
	model.Rules = nil
	model.Tags = terraformStringSetValue([]string{})
	planned, err := canonicalizeSegmentPlanModel(context.Background(), model)
	if err != nil {
		t.Fatalf("canonicalize partial Create response fixture: %v", err)
	}
	planned.ID = providerSegmentCreatedID
	script := &segmentHTTPScript{
		t: t,
		expectations: []segmentHTTPExpectation{
			segmentCollectionExpectation(false, 0, []string{}),
			segmentCollectionExpectation(true, 0, []string{}),
			{
				method: http.MethodPost,
				path:   segmentResourceCollectionPath(),
				status: http.StatusOK,
				data:   `{"id":"` + providerSegmentCreatedID + `"}`,
			},
			segmentExactExpectation(t, planned),
		},
	}
	apiClient, closeServer := newProjectResourceTestClient(t, script)
	defer closeServer()
	segmentSchema := segmentResourceSchema()
	response := frameworkresource.CreateResponse{
		State: emptySegmentResourceState(t, segmentSchema),
	}
	(&segmentResource{client: apiClient}).Create(
		context.Background(),
		frameworkresource.CreateRequest{
			Plan: segmentResourceTestPlan(t, segmentSchema, model),
		},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("partial Create response recovery diagnostics = %v", response.Diagnostics)
	}
	script.assertComplete(t)
	state := segmentResourceStateModel(t, response.State)
	canonicalState, err := canonicalizeSegmentStateModel(context.Background(), state)
	if err != nil || !sameSegmentDefinition(planned, canonicalState) {
		t.Fatal("partial Create response did not recover through the exact UUID read")
	}
}

func TestSegmentResourceCreateRejectsInvalidDefinitionsBeforeTransport(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*segmentModel){
		"invalid environment UUID": func(model *segmentModel) {
			model.EnvironmentID = types.StringValue("invalid-environment")
		},
		"invalid computed UUID": func(model *segmentModel) {
			model.ID = types.StringValue("invalid-segment")
		},
		"invalid key": func(model *segmentModel) {
			model.Key = types.StringValue("invalid key")
		},
		"shared type": func(model *segmentModel) {
			model.Type = types.StringValue(string(client.SegmentTypeShared))
		},
		"project scope": func(model *segmentModel) {
			model.Scopes = terraformStringSetValue([]string{providerSegmentProjectScope})
		},
		"overlapping users": func(model *segmentModel) {
			model.IncludedUsers = terraformStringSetValue([]string{"same-user"})
			model.ExcludedUsers = terraformStringSetValue([]string{"same-user"})
		},
		"unknown targeting value": func(model *segmentModel) {
			model.Rules[0].Conditions[0].Value = types.StringUnknown()
		},
		"invalid targeting operator": func(model *segmentModel) {
			model.Rules[0].Conditions[0].Operator = types.StringValue("UnknownOperator")
		},
		"duplicate rule identity": func(model *segmentModel) {
			model.Rules[0].ID = types.StringValue(providerSegmentRuleOne)
			model.Rules[1].ID = types.StringValue(providerSegmentRuleOne)
		},
	}
	for name, mutate := range tests {
		name := name
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			model := providerSegmentPlanModel()
			mutate(&model)
			var calls atomic.Int32
			apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
				func(response http.ResponseWriter, _ *http.Request) {
					calls.Add(1)
					writeProjectResourceEnvelope(t, response, http.StatusInternalServerError, "null")
				},
			))
			defer closeServer()
			segmentSchema := segmentResourceSchema()
			response := frameworkresource.CreateResponse{
				State: emptySegmentResourceState(t, segmentSchema),
			}
			(&segmentResource{client: apiClient}).Create(
				context.Background(),
				frameworkresource.CreateRequest{
					Plan: segmentResourceTestPlan(t, segmentSchema, model),
				},
				&response,
			)
			if !response.Diagnostics.HasError() || calls.Load() != 0 {
				t.Fatalf("invalid Create diagnostics/requests = %v/%d", response.Diagnostics, calls.Load())
			}
		})
	}
}

func TestSegmentResourceCreatePreflightFailsClosedWithoutMutation(t *testing.T) {
	t.Parallel()

	model := providerSegmentPlanModel()
	key := model.Key.ValueString()
	exact := segmentResourceListItemJSON(providerSegmentFuzzyID, key)
	tests := map[string][]segmentHTTPExpectation{
		"active collision": {
			segmentCollectionExpectation(false, 1, []string{exact}),
			segmentCollectionExpectation(true, 0, []string{}),
		},
		"archived collision": {
			segmentCollectionExpectation(false, 0, []string{}),
			segmentCollectionExpectation(true, 1, []string{exact}),
		},
		"cross-view duplicate": {
			segmentCollectionExpectation(false, 1, []string{exact}),
			segmentCollectionExpectation(true, 1, []string{exact}),
		},
		"incomplete active page": {
			segmentCollectionExpectation(false, 1, []string{}),
		},
	}
	for name, expectations := range tests {
		name := name
		expectations := expectations
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			script := &segmentHTTPScript{t: t, expectations: expectations}
			apiClient, closeServer := newProjectResourceTestClient(t, script)
			defer closeServer()
			segmentSchema := segmentResourceSchema()
			response := frameworkresource.CreateResponse{
				State: emptySegmentResourceState(t, segmentSchema),
			}
			(&segmentResource{client: apiClient}).Create(
				context.Background(),
				frameworkresource.CreateRequest{
					Plan: segmentResourceTestPlan(t, segmentSchema, model),
				},
				&response,
			)
			if !response.Diagnostics.HasError() {
				t.Fatal("unsafe Segment create preflight produced no diagnostic")
			}
			script.assertComplete(t)
			state := segmentResourceStateModel(t, response.State)
			if !state.ID.IsNull() || !state.Key.IsNull() {
				t.Fatal("failed Segment create preflight persisted or adopted an object")
			}
			for _, unsafe := range []string{
				providerEnvironmentA, providerSegmentFuzzyID, key,
				providerSegmentEnvironmentScope,
			} {
				if diagnosticsContain(response.Diagnostics, unsafe) {
					t.Fatal("Segment create preflight diagnostic exposed a runtime value")
				}
			}
		})
	}
}

func TestSegmentResourceCreateResponseMismatchPreservesUUIDAndSendsNoInitialization(t *testing.T) {
	t.Parallel()

	model := providerSegmentPlanModel()
	planned, err := canonicalizeSegmentPlanModel(context.Background(), model)
	if err != nil {
		t.Fatalf("canonicalize response mismatch fixture: %v", err)
	}
	planned.ID = providerSegmentCreatedID
	created := planned
	created.IncludedUsers = []string{}
	created.ExcludedUsers = []string{}
	created.Rules = []canonicalSegmentRule{}
	created.Tags = []string{}

	tests := map[string]struct {
		post  string
		exact string
	}{
		"wrong key": {
			post: mutateSegmentResourceDefinitionJSON(t, created, func(data map[string]any) {
				data["key"] = "other-key"
			}),
			exact: mutateSegmentResourceDefinitionJSON(t, created, func(data map[string]any) {
				data["key"] = "other-key"
			}),
		},
		"wrong response identity": {
			post: segmentResourceDefinitionJSON(t, created, false),
			exact: mutateSegmentResourceDefinitionJSON(t, created, func(data map[string]any) {
				data["id"] = providerSegmentFuzzyID
			}),
		},
		"shared type and scope": {
			post: mutateSegmentResourceDefinitionJSON(t, created, func(data map[string]any) {
				data["type"] = string(client.SegmentTypeShared)
				data["scopes"] = []string{providerSegmentOrganizationScope}
				data["isEnvironmentSpecific"] = false
			}),
			exact: mutateSegmentResourceDefinitionJSON(t, created, func(data map[string]any) {
				data["type"] = string(client.SegmentTypeShared)
				data["scopes"] = []string{providerSegmentOrganizationScope}
				data["isEnvironmentSpecific"] = false
			}),
		},
		"contradictory scope": {
			post: mutateSegmentResourceDefinitionJSON(t, created, func(data map[string]any) {
				data["scopes"] = []string{providerSegmentProjectScope}
			}),
			exact: mutateSegmentResourceDefinitionJSON(t, created, func(data map[string]any) {
				data["scopes"] = []string{providerSegmentProjectScope}
			}),
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			script := &segmentHTTPScript{
				t: t,
				expectations: []segmentHTTPExpectation{
					segmentCollectionExpectation(false, 0, []string{}),
					segmentCollectionExpectation(true, 0, []string{}),
					{
						method: http.MethodPost,
						path:   segmentResourceCollectionPath(),
						status: http.StatusOK,
						data:   test.post,
					},
					{
						method: http.MethodGet,
						path:   segmentResourceExactPath(providerSegmentCreatedID),
						status: http.StatusOK,
						data:   test.exact,
					},
				},
			}
			apiClient, closeServer := newProjectResourceTestClient(t, script)
			defer closeServer()
			segmentSchema := segmentResourceSchema()
			response := frameworkresource.CreateResponse{
				State: emptySegmentResourceState(t, segmentSchema),
			}
			(&segmentResource{client: apiClient}).Create(
				context.Background(),
				frameworkresource.CreateRequest{
					Plan: segmentResourceTestPlan(t, segmentSchema, model),
				},
				&response,
			)
			if !response.Diagnostics.HasError() {
				t.Fatal("unsafe Create response produced no diagnostic")
			}
			script.assertComplete(t)
			state := segmentResourceStateModel(t, response.State)
			if state.ID.ValueString() != providerSegmentCreatedID ||
				state.Type.ValueString() != string(client.SegmentTypeEnvironmentSpecific) {
				t.Fatal("Create response mismatch did not preserve safe provisional state")
			}
			for _, unsafe := range []string{
				providerEnvironmentA, providerSegmentCreatedID, model.Key.ValueString(),
				providerSegmentEnvironmentScope,
			} {
				if diagnosticsContain(response.Diagnostics, unsafe) {
					t.Fatal("Create response mismatch diagnostic exposed a runtime value")
				}
			}
		})
	}
}

func TestSegmentResourceAmbiguousCreateReconcilesWithoutRetryOrAdoption(t *testing.T) {
	t.Parallel()

	model := providerSegmentPlanModel()
	key := model.Key.ValueString()
	exact := segmentResourceListItemJSON(providerSegmentCreatedID, key)
	tests := map[string]struct {
		recovery []segmentHTTPExpectation
	}{
		"exact zero": {
			recovery: []segmentHTTPExpectation{
				segmentCollectionExpectation(false, 0, []string{}),
				segmentCollectionExpectation(true, 0, []string{}),
			},
		},
		"active recovery": {
			recovery: []segmentHTTPExpectation{
				segmentCollectionExpectation(false, 1, []string{exact}),
				segmentCollectionExpectation(true, 0, []string{}),
			},
		},
		"archived recovery": {
			recovery: []segmentHTTPExpectation{
				segmentCollectionExpectation(false, 0, []string{}),
				segmentCollectionExpectation(true, 1, []string{exact}),
			},
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			expectations := []segmentHTTPExpectation{
				segmentCollectionExpectation(false, 0, []string{}),
				segmentCollectionExpectation(true, 0, []string{}),
				{
					method: http.MethodPost,
					path:   segmentResourceCollectionPath(),
					status: http.StatusServiceUnavailable,
					data:   "null",
				},
			}
			expectations = append(expectations, test.recovery...)
			script := &segmentHTTPScript{t: t, expectations: expectations}
			apiClient, closeServer := newProjectResourceTestClient(t, script)
			defer closeServer()
			segmentSchema := segmentResourceSchema()
			response := frameworkresource.CreateResponse{
				State: emptySegmentResourceState(t, segmentSchema),
			}
			(&segmentResource{client: apiClient}).Create(
				context.Background(),
				frameworkresource.CreateRequest{
					Plan: segmentResourceTestPlan(t, segmentSchema, model),
				},
				&response,
			)
			if !response.Diagnostics.HasError() {
				t.Fatal("ambiguous Segment Create produced no recovery diagnostic")
			}
			script.assertComplete(t)
			state := segmentResourceStateModel(t, response.State)
			if !state.ID.IsNull() || !state.Key.IsNull() {
				t.Fatal("ambiguous Segment Create silently adopted a collection match")
			}
			for _, unsafe := range []string{
				providerEnvironmentA, providerSegmentCreatedID, key,
				providerSegmentEnvironmentScope,
			} {
				if diagnosticsContain(response.Diagnostics, unsafe) {
					t.Fatal("ambiguous Segment Create diagnostic exposed a runtime value")
				}
			}
		})
	}
}

func TestSegmentResourceCreateReconcilesPartialInitializationWithoutReplay(t *testing.T) {
	t.Parallel()

	model := providerSegmentPlanModel()
	planned, err := canonicalizeSegmentPlanModel(context.Background(), model)
	if err != nil {
		t.Fatalf("canonicalize partial initialization fixture: %v", err)
	}
	planned.ID = providerSegmentCreatedID
	created := planned
	created.IncludedUsers = []string{}
	created.ExcludedUsers = []string{}
	created.Rules = []canonicalSegmentRule{}
	created.Tags = []string{}
	targeted := planned
	targeted.Tags = []string{}

	targetingMutation := func(status int) segmentHTTPExpectation {
		return segmentHTTPExpectation{
			method: http.MethodPut,
			path:   segmentResourceExactPath(providerSegmentCreatedID) + "/targeting",
			status: status,
			data: func() string {
				if status == http.StatusOK {
					return "true"
				}
				return "null"
			}(),
			checkBody: func(t *testing.T, request *http.Request) {
				assertProviderSegmentJSONBody(t, request, expandSegmentTargetingRequest(planned))
			},
		}
	}
	tagMutation := func(status int) segmentHTTPExpectation {
		return segmentHTTPExpectation{
			method: http.MethodPut,
			path:   segmentResourceExactPath(providerSegmentCreatedID) + "/tags",
			status: status,
			data: func() string {
				if status == http.StatusOK {
					return "true"
				}
				return "null"
			}(),
			checkBody: func(t *testing.T, request *http.Request) {
				assertProviderSegmentJSONBody(
					t,
					request,
					client.UpdateSegmentTagsRequest{Tags: planned.Tags},
				)
			},
		}
	}
	base := func() []segmentHTTPExpectation {
		return []segmentHTTPExpectation{
			segmentCollectionExpectation(false, 0, []string{}),
			segmentCollectionExpectation(true, 0, []string{}),
			{
				method: http.MethodPost,
				path:   segmentResourceCollectionPath(),
				status: http.StatusOK,
				data:   segmentResourceDefinitionJSON(t, created, false),
			},
			segmentExactExpectation(t, created),
		}
	}
	tests := map[string]struct {
		expectations []segmentHTTPExpectation
		wantState    canonicalSegment
		wantError    bool
	}{
		"ambiguous targeting confirmed applied": {
			expectations: append(base(),
				targetingMutation(http.StatusServiceUnavailable),
				segmentExactExpectation(t, targeted),
				tagMutation(http.StatusOK),
				segmentExactExpectation(t, planned),
			),
			wantState: planned,
		},
		"ambiguous targeting confirmed not applied": {
			expectations: append(base(),
				targetingMutation(http.StatusServiceUnavailable),
				segmentExactExpectation(t, created),
			),
			wantState: created,
			wantError: true,
		},
		"tag validation failure preserves confirmed targeting": {
			expectations: append(base(),
				targetingMutation(http.StatusOK),
				segmentExactExpectation(t, targeted),
				tagMutation(http.StatusBadRequest),
			),
			wantState: targeted,
			wantError: true,
		},
		"ambiguous tags confirmed applied": {
			expectations: append(base(),
				targetingMutation(http.StatusOK),
				segmentExactExpectation(t, targeted),
				tagMutation(http.StatusServiceUnavailable),
				segmentExactExpectation(t, planned),
			),
			wantState: planned,
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			script := &segmentHTTPScript{t: t, expectations: test.expectations}
			apiClient, closeServer := newProjectResourceTestClient(t, script)
			defer closeServer()
			segmentSchema := segmentResourceSchema()
			response := frameworkresource.CreateResponse{
				State: emptySegmentResourceState(t, segmentSchema),
			}
			(&segmentResource{client: apiClient}).Create(
				context.Background(),
				frameworkresource.CreateRequest{
					Plan: segmentResourceTestPlan(t, segmentSchema, model),
				},
				&response,
			)
			if response.Diagnostics.HasError() != test.wantError {
				t.Fatalf("partial initialization diagnostics = %v, want error %t", response.Diagnostics, test.wantError)
			}
			script.assertComplete(t)
			state := segmentResourceStateModel(t, response.State)
			canonicalState, stateErr := canonicalizeSegmentStateModel(context.Background(), state)
			if stateErr != nil || !sameSegmentDefinition(test.wantState, canonicalState) {
				t.Fatal("partial initialization did not preserve the last exact canonical state")
			}
			if test.wantError {
				for _, unsafe := range []string{
					providerEnvironmentA, providerSegmentCreatedID, model.Key.ValueString(),
					providerSegmentEnvironmentScope, "user-z", "tag-z",
				} {
					if diagnosticsContain(response.Diagnostics, unsafe) {
						t.Fatal("partial initialization diagnostic exposed a runtime value")
					}
				}
			}
		})
	}
}

func TestSegmentResourceImportIsStrictAndSetsOnlyExactIdentity(t *testing.T) {
	t.Parallel()

	validID := strings.ToUpper(providerEnvironmentA) + "/" + strings.ToUpper(providerSegmentID)
	tests := map[string]struct {
		importID string
		wantErr  bool
	}{
		"valid":                  {importID: validID},
		"empty":                  {importID: "", wantErr: true},
		"one component":          {importID: providerEnvironmentA, wantErr: true},
		"three components":       {importID: providerEnvironmentA + "/" + providerSegmentID + "/extra", wantErr: true},
		"missing environment":    {importID: "/" + providerSegmentID, wantErr: true},
		"missing segment":        {importID: providerEnvironmentA + "/", wantErr: true},
		"invalid environment":    {importID: "invalid/" + providerSegmentID, wantErr: true},
		"invalid segment":        {importID: providerEnvironmentA + "/invalid", wantErr: true},
		"noncanonical UUID form": {importID: "{" + providerEnvironmentA + "}/" + providerSegmentID, wantErr: true},
	}
	segmentSchema := segmentResourceSchema()
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response := frameworkresource.ImportStateResponse{
				State: emptySegmentResourceState(t, segmentSchema),
			}
			(&segmentResource{}).ImportState(
				context.Background(),
				frameworkresource.ImportStateRequest{ID: test.importID},
				&response,
			)
			if response.Diagnostics.HasError() != test.wantErr {
				t.Fatalf("ImportState() diagnostics = %v, want error %t", response.Diagnostics, test.wantErr)
			}
			if test.wantErr {
				if test.importID != "" && diagnosticsContain(response.Diagnostics, test.importID) {
					t.Fatal("ImportState() diagnostic echoed a rejected runtime identifier")
				}
				return
			}
			state := segmentResourceStateModel(t, response.State)
			if state.EnvironmentID.ValueString() != strings.ToUpper(providerEnvironmentA) ||
				state.ID.ValueString() != strings.ToUpper(providerSegmentID) ||
				!state.Key.IsNull() || !state.Type.IsNull() || !state.Scopes.IsNull() {
				t.Fatal("ImportState() set fields beyond the two public identity components")
			}
		})
	}
}

func TestSegmentResourceImportReadRetainsServerTargetingIdentities(t *testing.T) {
	t.Parallel()

	remote := providerRemoteSegment(
		client.SegmentTypeEnvironmentSpecific,
		[]string{providerSegmentEnvironmentScope},
	)
	remote.Included = []string{"user-z", "user-a", "user-z"}
	remote.Excluded = []string{"user-y"}
	remote.Tags = []string{"tag-z", "tag-a", "tag-z"}
	canonical, err := canonicalizeRemoteSegment(remote)
	if err != nil {
		t.Fatalf("canonicalize imported Segment fixture: %v", err)
	}
	wire := mutateSegmentResourceDefinitionJSON(t, canonical, func(data map[string]any) {
		data["included"] = remote.Included
		data["excluded"] = remote.Excluded
		data["tags"] = remote.Tags
		data["rules"] = []any{
			map[string]any{
				"id": strings.ToUpper(providerSegmentRuleOne), "name": "First",
				"conditions": []any{
					map[string]any{
						"id":       strings.ToUpper(providerSegmentConditionOne),
						"property": "region", "op": segmentOperatorIsOneOf,
						"value": `["b","a","b"]`,
					},
					map[string]any{
						"id": providerSegmentConditionTwo, "property": "score",
						"op": segmentOperatorLessThan, "value": "1.00",
					},
				},
			},
			map[string]any{
				"id": providerSegmentRuleTwo, "name": "Second",
				"conditions": []any{map[string]any{
					"id": providerSegmentConditionTri, "property": "enabled",
					"op": segmentOperatorIsTrue, "value": "",
				}},
			},
		}
	})
	script := &segmentHTTPScript{
		t: t,
		expectations: []segmentHTTPExpectation{{
			method: http.MethodGet,
			path: "/api/v1/envs/" + strings.ToUpper(providerEnvironmentA) +
				"/segments/" + strings.ToUpper(providerSegmentID),
			status: http.StatusOK,
			data:   wire,
		}},
	}
	apiClient, closeServer := newProjectResourceTestClient(t, script)
	defer closeServer()
	segmentSchema := segmentResourceSchema()
	importResponse := frameworkresource.ImportStateResponse{
		State: emptySegmentResourceState(t, segmentSchema),
	}
	importID := strings.ToUpper(providerEnvironmentA) + "/" + strings.ToUpper(providerSegmentID)
	(&segmentResource{}).ImportState(
		context.Background(),
		frameworkresource.ImportStateRequest{ID: importID},
		&importResponse,
	)
	if importResponse.Diagnostics.HasError() {
		t.Fatalf("ImportState() diagnostics = %v", importResponse.Diagnostics)
	}
	readResponse := frameworkresource.ReadResponse{State: importResponse.State}
	(&segmentResource{client: apiClient}).Read(
		context.Background(),
		frameworkresource.ReadRequest{State: importResponse.State},
		&readResponse,
	)
	if readResponse.Diagnostics.HasError() {
		t.Fatalf("import Read() diagnostics = %v", readResponse.Diagnostics)
	}
	state := segmentResourceStateModel(t, readResponse.State)
	canonicalState, err := canonicalizeSegmentStateModel(context.Background(), state)
	if err != nil || !sameSegmentDefinition(canonical, canonicalState) {
		t.Fatal("import Read() did not retain the exact canonical server definition")
	}
	if state.EnvironmentID.ValueString() != strings.ToUpper(providerEnvironmentA) ||
		state.Rules[0].ID.ValueString() != providerSegmentRuleOne ||
		state.Rules[0].Conditions[0].ID.ValueString() != providerSegmentConditionOne ||
		state.Rules[0].Conditions[0].Value.ValueString() != `["a","b"]` {
		t.Fatal("import Read() did not preserve parent spelling and canonical server targeting identities")
	}
	script.assertComplete(t)
}

func TestSegmentResourceReadComposesAbsenceArchiveAndUnsafeTypeSafely(t *testing.T) {
	t.Parallel()

	segmentSchema := segmentResourceSchema()
	priorModel := providerSegmentResourceStateModel()
	priorState := segmentResourceTestState(t, segmentSchema, priorModel)
	prior, err := canonicalizeSegmentStateModel(context.Background(), priorModel)
	if err != nil {
		t.Fatalf("canonicalize managed Segment Read fixture: %v", err)
	}
	shared := prior
	shared.Type = client.SegmentTypeShared
	shared.Scopes = []string{providerSegmentOrganizationScope}
	tests := map[string]struct {
		expectations []segmentHTTPExpectation
		wantRemoved  bool
	}{
		"direct not found plus complete exact zero": {
			expectations: []segmentHTTPExpectation{
				{
					method: http.MethodGet,
					path:   segmentResourceExactPath(providerSegmentID),
					status: http.StatusNotFound,
					data:   "null",
				},
				segmentCollectionExpectation(false, 0, []string{}),
				segmentCollectionExpectation(true, 0, []string{}),
			},
			wantRemoved: true,
		},
		"direct not found but active metadata remains": {
			expectations: []segmentHTTPExpectation{
				{
					method: http.MethodGet,
					path:   segmentResourceExactPath(providerSegmentID),
					status: http.StatusNotFound,
					data:   "null",
				},
				segmentCollectionExpectation(false, 1, []string{
					segmentResourceListItemJSON(providerSegmentID, prior.Key),
				}),
				segmentCollectionExpectation(true, 0, []string{}),
			},
		},
		"archived exact definition": {
			expectations: []segmentHTTPExpectation{{
				method: http.MethodGet,
				path:   segmentResourceExactPath(providerSegmentID),
				status: http.StatusOK,
				data:   segmentResourceDefinitionJSON(t, prior, true),
			}},
		},
		"shared exact definition": {
			expectations: []segmentHTTPExpectation{{
				method: http.MethodGet,
				path:   segmentResourceExactPath(providerSegmentID),
				status: http.StatusOK,
				data:   segmentResourceDefinitionJSON(t, shared, false),
			}},
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			script := &segmentHTTPScript{t: t, expectations: test.expectations}
			apiClient, closeServer := newProjectResourceTestClient(t, script)
			defer closeServer()
			response := frameworkresource.ReadResponse{State: priorState}
			(&segmentResource{client: apiClient}).Read(
				context.Background(),
				frameworkresource.ReadRequest{State: priorState},
				&response,
			)
			script.assertComplete(t)
			if test.wantRemoved {
				if response.Diagnostics.HasError() || !response.State.Raw.IsNull() {
					t.Fatalf("authoritative exact zero diagnostics/state = %v/%v", response.Diagnostics, response.State.Raw)
				}
				return
			}
			if !response.Diagnostics.HasError() || response.State.Raw.IsNull() ||
				!response.State.Raw.Equal(priorState.Raw) {
				t.Fatal("ambiguous, archived, or shared Read did not preserve prior state")
			}
			for _, unsafe := range []string{
				providerEnvironmentA, providerSegmentID, prior.Key,
				providerSegmentEnvironmentScope, providerSegmentOrganizationScope,
			} {
				if diagnosticsContain(response.Diagnostics, unsafe) {
					t.Fatal("Segment Read diagnostic exposed a runtime value")
				}
			}
		})
	}
}

func TestSegmentResourceCancellationWhileWaitingForCreateLockSendsNoRequest(t *testing.T) {
	t.Parallel()

	model := providerSegmentPlanModel()
	planned, err := canonicalizeSegmentPlanModel(context.Background(), model)
	if err != nil {
		t.Fatalf("canonicalize Segment lock fixture: %v", err)
	}
	var calls atomic.Int32
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			writeProjectResourceEnvelope(t, response, http.StatusInternalServerError, "null")
		},
	))
	defer closeServer()
	resourceUnderTest := &segmentResource{client: apiClient}
	manager := resourceUnderTest.segmentLocks()
	lockKey := segmentCreateLockKey(planned.EnvironmentID, planned.Key)
	release, err := manager.acquire(context.Background(), lockKey)
	if err != nil {
		t.Fatalf("acquire blocking Segment lock: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	segmentSchema := segmentResourceSchema()
	response := frameworkresource.CreateResponse{
		State: emptySegmentResourceState(t, segmentSchema),
	}
	resourceUnderTest.Create(
		ctx,
		frameworkresource.CreateRequest{
			Plan: segmentResourceTestPlan(t, segmentSchema, model),
		},
		&response,
	)
	release()
	if !response.Diagnostics.HasError() || calls.Load() != 0 {
		t.Fatalf("canceled lock wait diagnostics/requests = %v/%d", response.Diagnostics, calls.Load())
	}
	state := segmentResourceStateModel(t, response.State)
	if !state.ID.IsNull() || !state.Key.IsNull() {
		t.Fatal("canceled Segment Create lock wait changed resource state")
	}
	manager.mu.Lock()
	remaining := len(manager.locks)
	manager.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("canceled Segment lock wait retained %d lock entries", remaining)
	}
}

func emptySegmentResourceState(
	t *testing.T,
	segmentSchema resourceschema.Schema,
) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: segmentSchema}
	empty := segmentModel{
		EnvironmentID: types.StringNull(),
		ID:            types.StringNull(),
		Name:          types.StringNull(),
		Key:           types.StringNull(),
		Description:   types.StringNull(),
		Type:          types.StringNull(),
		Scopes:        types.SetNull(types.StringType),
		IncludedUsers: types.SetNull(types.StringType),
		ExcludedUsers: types.SetNull(types.StringType),
		Rules:         nil,
		Tags:          types.SetNull(types.StringType),
	}
	if diagnostics := state.Set(context.Background(), &empty); diagnostics.HasError() {
		t.Fatalf("initialize empty Segment resource state: %v", diagnostics)
	}
	return state
}

func segmentResourceStateModel(t *testing.T, state tfsdk.State) segmentModel {
	t.Helper()
	var model segmentModel
	if diagnostics := state.Get(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("read Segment resource state: %v", diagnostics)
	}
	return model
}

func segmentResourceCollectionPath() string {
	return "/api/v1/envs/" + providerEnvironmentA + "/segments"
}

func segmentResourceExactPath(segmentID string) string {
	return segmentResourceCollectionPath() + "/" + segmentID
}

func segmentCollectionExpectation(
	archived bool,
	total int64,
	items []string,
) segmentHTTPExpectation {
	return segmentHTTPExpectation{
		method: http.MethodGet,
		path:   segmentResourceCollectionPath(),
		query: fmt.Sprintf(
			"IsArchived=%t&Name=&PageIndex=0&PageSize=100",
			archived,
		),
		status: http.StatusOK,
		data: fmt.Sprintf(
			`{"totalCount":%d,"items":[%s]}`,
			total,
			strings.Join(items, ","),
		),
	}
}

func segmentExactExpectation(
	t *testing.T,
	segment canonicalSegment,
) segmentHTTPExpectation {
	t.Helper()
	return segmentHTTPExpectation{
		method: http.MethodGet,
		path:   segmentResourceExactPath(segment.ID),
		status: http.StatusOK,
		data:   segmentResourceDefinitionJSON(t, segment, false),
	}
}

func segmentResourceListItemJSON(id string, key string) string {
	payload := map[string]any{
		"id":                    id,
		"name":                  "Synthetic list match",
		"key":                   key,
		"type":                  string(client.SegmentTypeEnvironmentSpecific),
		"scopes":                []string{providerSegmentEnvironmentScope},
		"isEnvironmentSpecific": true,
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}

func segmentResourceDefinitionJSON(
	t *testing.T,
	segment canonicalSegment,
	archived bool,
) string {
	t.Helper()
	rules := make([]map[string]any, 0, len(segment.Rules))
	for _, rule := range segment.Rules {
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
	payload := map[string]any{
		"id": segment.ID, "envId": segment.EnvironmentID,
		"name": segment.Name, "key": segment.Key,
		"description": segment.Description, "type": string(segment.Type),
		"scopes":   append([]string{}, segment.Scopes...),
		"included": append([]string{}, segment.IncludedUsers...),
		"excluded": append([]string{}, segment.ExcludedUsers...),
		"rules":    rules, "tags": append([]string{}, segment.Tags...),
		"isArchived":            archived,
		"isEnvironmentSpecific": segment.Type == client.SegmentTypeEnvironmentSpecific,
		"workspaceId":           "synthetic-server-owned-workspace",
		"pending":               map[string]any{"value": "synthetic-server-owned-pending"},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode Segment resource definition: %v", err)
	}
	return string(encoded)
}

func mutateSegmentResourceDefinitionJSON(
	t *testing.T,
	segment canonicalSegment,
	mutate func(map[string]any),
) string {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal([]byte(segmentResourceDefinitionJSON(t, segment, false)), &data); err != nil {
		t.Fatalf("decode Segment definition for mutation: %v", err)
	}
	mutate(data)
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("encode mutated Segment definition: %v", err)
	}
	return string(encoded)
}

func assertProviderSegmentJSONBody(t *testing.T, request *http.Request, want any) {
	t.Helper()
	var gotValue any
	if err := json.NewDecoder(request.Body).Decode(&gotValue); err != nil {
		t.Fatalf("decode Segment request body: %v", err)
	}
	wantBody, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("encode expected Segment request body: %v", err)
	}
	var wantValue any
	if err := json.Unmarshal(wantBody, &wantValue); err != nil {
		t.Fatalf("decode expected Segment request body: %v", err)
	}
	gotJSON, _ := json.Marshal(gotValue)
	wantJSON, _ := json.Marshal(wantValue)
	if string(gotJSON) != string(wantJSON) {
		t.Fatal("Segment request body did not match the exact lifecycle-owned payload")
	}
}
