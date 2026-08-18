// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestGroupPolicyBindingResourceMetadataConfigureSchemaAndImport(t *testing.T) {
	t.Parallel()

	resourceUnderTest := newGroupPolicyBindingTestResource(nil)
	var metadata frameworkresource.MetadataResponse
	resourceUnderTest.Metadata(
		context.Background(),
		frameworkresource.MetadataRequest{ProviderTypeName: "featbit"},
		&metadata,
	)
	if metadata.TypeName != "featbit_group_policy_binding" {
		t.Fatalf("metadata type = %q", metadata.TypeName)
	}

	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {
			t.Fatal("Configure() reached transport")
		},
	))
	defer closeServer()
	var configure frameworkresource.ConfigureResponse
	resourceUnderTest.Configure(
		context.Background(),
		frameworkresource.ConfigureRequest{ProviderData: apiClient},
		&configure,
	)
	if configure.Diagnostics.HasError() || resourceUnderTest.client != apiClient {
		t.Fatalf("Configure() diagnostics/client = %v/%p", configure.Diagnostics, resourceUnderTest.client)
	}

	bindingSchema := groupPolicyBindingResourceTestSchema(t)
	if len(bindingSchema.Attributes) != 3 {
		t.Fatalf("binding schema attributes = %v", bindingSchema.Attributes)
	}
	idAttribute, ok := bindingSchema.Attributes["id"].(resourceschema.StringAttribute)
	if !ok || !idAttribute.Computed || idAttribute.Required || idAttribute.Optional ||
		len(idAttribute.PlanModifiers) != 1 {
		t.Fatalf("id schema = %#v", bindingSchema.Attributes["id"])
	}
	for _, name := range []string{"group_id", "policy_id"} {
		attribute, ok := bindingSchema.Attributes[name].(resourceschema.StringAttribute)
		if !ok || !attribute.Required || attribute.Optional || attribute.Computed ||
			len(attribute.Validators) != 1 || len(attribute.PlanModifiers) != 1 {
			t.Fatalf("%s schema = %#v", name, bindingSchema.Attributes[name])
		}
	}

	priorState := groupPolicyBindingResourceTestState(
		t, bindingSchema, providerGroupID, providerPolicyID,
	)
	var stableIDResponse planmodifier.StringResponse
	stableIDResponse.PlanValue = types.StringUnknown()
	idAttribute.PlanModifiers[0].PlanModifyString(
		context.Background(),
		planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			PlanValue:   types.StringUnknown(),
			StateValue:  types.StringValue(providerGroupID + "/" + providerPolicyID),
			Plan: groupPolicyBindingResourceTestPlan(
				t, bindingSchema, providerGroupID, providerPolicyID,
			),
			State: priorState,
		},
		&stableIDResponse,
	)
	if stableIDResponse.Diagnostics.HasError() ||
		stableIDResponse.PlanValue.ValueString() != providerGroupID+"/"+providerPolicyID {
		t.Fatalf("stable id modifier response = %#v", stableIDResponse)
	}
	var replacementIDResponse planmodifier.StringResponse
	replacementIDResponse.PlanValue = types.StringUnknown()
	idAttribute.PlanModifiers[0].PlanModifyString(
		context.Background(),
		planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			PlanValue:   types.StringUnknown(),
			StateValue:  types.StringValue(providerGroupID + "/" + providerPolicyID),
			Plan: groupPolicyBindingResourceTestPlan(
				t, bindingSchema, providerGroupIDTwo, providerPolicyID,
			),
			State: priorState,
		},
		&replacementIDResponse,
	)
	if replacementIDResponse.Diagnostics.HasError() || !replacementIDResponse.PlanValue.IsUnknown() {
		t.Fatalf("replacement id modifier response = %#v", replacementIDResponse)
	}

	canonicalImportID := providerGroupID + "/" + providerPolicyID
	tests := map[string]struct {
		id        string
		wantError bool
	}{
		"canonical":               {id: canonicalImportID},
		"uppercase canonicalized": {id: strings.ToUpper(canonicalImportID)},
		"missing":                 {id: "", wantError: true},
		"Group only":              {id: providerGroupID, wantError: true},
		"invalid Group":           {id: "not-a-uuid/" + providerPolicyID, wantError: true},
		"invalid Policy":          {id: providerGroupID + "/not-a-uuid", wantError: true},
		"extra component":         {id: canonicalImportID + "/extra", wantError: true},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			response := frameworkresource.ImportStateResponse{
				State: emptyGroupPolicyBindingResourceState(t, bindingSchema),
			}
			resourceUnderTest.ImportState(
				context.Background(),
				frameworkresource.ImportStateRequest{ID: test.id},
				&response,
			)
			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf("ImportState() error = %t, want %t: %v", got, test.wantError, response.Diagnostics)
			}
			if test.wantError {
				if test.id != "" && strings.Contains(fmt.Sprint(response.Diagnostics), test.id) {
					t.Fatal("ImportState() diagnostic echoed the rejected identifier")
				}
				return
			}
			state := groupPolicyBindingStateModel(t, response.State)
			if state.ID.ValueString() != canonicalImportID ||
				state.GroupID.ValueString() != providerGroupID ||
				state.PolicyID.ValueString() != providerPolicyID {
				t.Fatalf("ImportState() state = %#v", state)
			}
		})
	}
}

func TestGroupPolicyBindingModelsRejectInconsistentStateAndRedactFormatting(t *testing.T) {
	t.Parallel()

	identity := groupBindingIdentity{
		GroupID:  providerGroupID,
		TargetID: providerPolicyID,
	}
	if _, err := canonicalizeGroupBindingStateValues(
		types.StringValue(providerGroupID+"/"+providerGroupPolicyID),
		types.StringValue(identity.GroupID),
		types.StringValue(identity.TargetID),
	); err == nil {
		t.Fatal("inconsistent synthetic binding ID was accepted as state")
	}

	formatted := fmt.Sprintf(
		"%v|%+v|%#v",
		identity, identity, identity,
	)
	for _, runtimeID := range []string{providerGroupID, providerPolicyID} {
		if strings.Contains(formatted, runtimeID) {
			t.Fatalf("binding model formatting exposed runtime identity %q", runtimeID)
		}
	}
}

func TestGroupPolicyBindingResourceCreateAdoptsOrReconcilesExactPair(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		preExisting       bool
		builtInPolicy     bool
		failBeforeApply   bool
		failAfterApply    bool
		wantError         bool
		wantMutationCount int
		wantState         bool
	}{
		"create": {
			wantMutationCount: 1,
			wantState:         true,
		},
		"pre-existing built-in Policy binding": {
			preExisting:   true,
			builtInPolicy: true,
			wantState:     true,
		},
		"ambiguous response after apply": {
			failAfterApply:    true,
			wantMutationCount: 1,
			wantState:         true,
		},
		"ambiguous response before apply": {
			failBeforeApply:   true,
			wantError:         true,
			wantMutationCount: 1,
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newGroupPolicyBindingFixture(t)
			if test.preExisting {
				fixture.policyIDs = []string{providerPolicyID}
			}
			if test.builtInPolicy {
				fixture.policy.Type = client.PolicyTypeSysManaged
				fixture.policy.Key = "Owner"
				fixture.policy.Name = "Owner"
			}
			fixture.failAddBeforeApply = test.failBeforeApply
			fixture.failAddAfterApply = test.failAfterApply
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			bindingSchema := groupPolicyBindingResourceTestSchema(t)
			response := frameworkresource.CreateResponse{
				State: emptyGroupPolicyBindingResourceState(t, bindingSchema),
			}
			newGroupPolicyBindingTestResource(apiClient).Create(
				context.Background(),
				frameworkresource.CreateRequest{Plan: groupPolicyBindingResourceTestPlan(
					t, bindingSchema, providerGroupID, providerPolicyID,
				)},
				&response,
			)
			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf("Create() error = %t, want %t: %v", got, test.wantError, response.Diagnostics)
			}
			stateModel := groupPolicyBindingStateModel(t, response.State)
			if got := knownString(stateModel.ID); got != test.wantState {
				t.Fatalf("Create() state recorded = %t, want %t; state = %#v", got, test.wantState, stateModel)
			}
			if got := len(fixture.mutations()); got != test.wantMutationCount {
				t.Fatalf("mutation count = %d, want %d: %v", got, test.wantMutationCount, fixture.mutations())
			}
			if test.wantState {
				assertGroupPolicyBindingState(t, response.State, providerGroupID, providerPolicyID)
				for iteration := 0; iteration < 2; iteration++ {
					var readResponse frameworkresource.ReadResponse
					newGroupPolicyBindingTestResource(apiClient).Read(
						context.Background(),
						frameworkresource.ReadRequest{State: response.State},
						&readResponse,
					)
					if readResponse.Diagnostics.HasError() || readResponse.State.Raw.IsNull() {
						t.Fatalf("repeated Read() = %v/%s", readResponse.Diagnostics, readResponse.State.Raw)
					}
					response.State = readResponse.State
				}
				if got := len(fixture.mutations()); got != test.wantMutationCount {
					t.Fatalf("repeated reads changed mutation count to %d", got)
				}
			}
		})
	}
}

func TestGroupPolicyBindingResourceCreateRequiresExactEndpointsAndReadableCollection(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*groupPolicyBindingFixture){
		"missing Group": func(fixture *groupPolicyBindingFixture) {
			fixture.groupPresent = false
		},
		"missing Policy": func(fixture *groupPolicyBindingFixture) {
			fixture.policyPresent = false
		},
		"ambiguous relationship collection": func(fixture *groupPolicyBindingFixture) {
			fixture.failRelationshipReads = true
		},
	}
	for name, arrange := range tests {
		name := name
		arrange := arrange
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newGroupPolicyBindingFixture(t)
			arrange(fixture)
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			bindingSchema := groupPolicyBindingResourceTestSchema(t)
			response := frameworkresource.CreateResponse{
				State: emptyGroupPolicyBindingResourceState(t, bindingSchema),
			}
			newGroupPolicyBindingTestResource(apiClient).Create(
				context.Background(),
				frameworkresource.CreateRequest{Plan: groupPolicyBindingResourceTestPlan(
					t, bindingSchema, providerGroupID, providerPolicyID,
				)},
				&response,
			)
			stateModel := groupPolicyBindingStateModel(t, response.State)
			if !response.Diagnostics.HasError() || knownString(stateModel.ID) || len(fixture.mutations()) != 0 {
				t.Fatalf("Create() diagnostics/state/mutations = %v/%#v/%v", response.Diagnostics, stateModel, fixture.mutations())
			}
		})
	}
}

func TestGroupPolicyBindingResourceReadTracksExactPairDriftAndAbsence(t *testing.T) {
	t.Parallel()

	fixture := newGroupPolicyBindingFixture(t)
	fixture.policyIDs = []string{providerPolicyID, providerGroupPolicyID}
	apiClient, closeServer := newProjectResourceTestClient(t, fixture)
	defer closeServer()
	bindingSchema := groupPolicyBindingResourceTestSchema(t)
	state := groupPolicyBindingResourceTestState(t, bindingSchema, providerGroupID, providerPolicyID)

	var presentResponse frameworkresource.ReadResponse
	newGroupPolicyBindingTestResource(apiClient).Read(
		context.Background(),
		frameworkresource.ReadRequest{State: state},
		&presentResponse,
	)
	if presentResponse.Diagnostics.HasError() || presentResponse.State.Raw.IsNull() {
		t.Fatalf("Read() present diagnostics/state = %v/%s", presentResponse.Diagnostics, presentResponse.State.Raw)
	}

	fixture.removePolicyID(providerPolicyID)
	var driftResponse frameworkresource.ReadResponse
	newGroupPolicyBindingTestResource(apiClient).Read(
		context.Background(),
		frameworkresource.ReadRequest{State: presentResponse.State},
		&driftResponse,
	)
	if driftResponse.Diagnostics.HasError() || !driftResponse.State.Raw.IsNull() {
		t.Fatalf("Read() drift diagnostics/state = %v/%s", driftResponse.Diagnostics, driftResponse.State.Raw)
	}
	if got := fixture.currentPolicyIDs(); len(got) != 1 || got[0] != providerGroupPolicyID {
		t.Fatalf("unrelated binding changed during Read: %v", got)
	}
}

func TestGroupPolicyBindingResourceReadPreservesAmbiguityButAcceptsMissingEndpoint(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		groupPresent bool
		wantError    bool
		wantState    bool
	}{
		"relationship read fails while endpoints exist": {
			groupPresent: true,
			wantError:    true,
			wantState:    true,
		},
		"confirmed missing Group proves pair absent": {
			wantState: false,
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newGroupPolicyBindingFixture(t)
			fixture.policyIDs = []string{providerPolicyID}
			fixture.groupPresent = test.groupPresent
			fixture.failRelationshipReads = true
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			bindingSchema := groupPolicyBindingResourceTestSchema(t)
			state := groupPolicyBindingResourceTestState(t, bindingSchema, providerGroupID, providerPolicyID)
			var response frameworkresource.ReadResponse
			newGroupPolicyBindingTestResource(apiClient).Read(
				context.Background(),
				frameworkresource.ReadRequest{State: state},
				&response,
			)
			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf("Read() error = %t, want %t: %v", got, test.wantError, response.Diagnostics)
			}
			if got := !response.State.Raw.IsNull(); got != test.wantState {
				t.Fatalf("Read() state present = %t, want %t", got, test.wantState)
			}
		})
	}
}

func TestGroupPolicyBindingResourceUpdateIsReadOnlySafetyPath(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		startPresent bool
		planGroupID  string
		wantError    bool
		wantState    bool
	}{
		"unchanged present pair": {
			startPresent: true,
			planGroupID:  providerGroupID,
			wantState:    true,
		},
		"unchanged pair removed out of band": {
			planGroupID: providerGroupID,
		},
		"identity change is rejected": {
			planGroupID: providerGroupIDTwo,
			wantError:   true,
			wantState:   true,
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newGroupPolicyBindingFixture(t)
			if test.startPresent {
				fixture.policyIDs = []string{providerPolicyID}
			}
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			bindingSchema := groupPolicyBindingResourceTestSchema(t)
			state := groupPolicyBindingResourceTestState(
				t, bindingSchema, providerGroupID, providerPolicyID,
			)
			var response frameworkresource.UpdateResponse
			newGroupPolicyBindingTestResource(apiClient).Update(
				context.Background(),
				frameworkresource.UpdateRequest{
					State: state,
					Plan: groupPolicyBindingResourceTestPlan(
						t, bindingSchema, test.planGroupID, providerPolicyID,
					),
				},
				&response,
			)
			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf("Update() error = %t, want %t: %v", got, test.wantError, response.Diagnostics)
			}
			if got := !response.State.Raw.IsNull(); got != test.wantState {
				t.Fatalf("Update() state present = %t, want %t", got, test.wantState)
			}
			if mutations := fixture.mutations(); len(mutations) != 0 {
				t.Fatalf("read-only Update() sent mutations: %v", mutations)
			}
			if test.wantState {
				assertGroupPolicyBindingState(t, response.State, providerGroupID, providerPolicyID)
			}
		})
	}
}

func TestGroupPolicyBindingResourceDeleteRemovesOnlyExactPairAndReconciles(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		startPresent      bool
		failBeforeApply   bool
		failAfterApply    bool
		skipRemoveApply   bool
		wantError         bool
		wantMutationCount int
		wantState         bool
	}{
		"success": {
			startPresent:      true,
			wantMutationCount: 1,
		},
		"already absent": {},
		"ambiguous response after apply": {
			startPresent:      true,
			failAfterApply:    true,
			wantMutationCount: 1,
		},
		"ambiguous response before apply": {
			startPresent:      true,
			failBeforeApply:   true,
			wantError:         true,
			wantMutationCount: 1,
			wantState:         true,
		},
		"successful no-op is rejected": {
			startPresent:      true,
			skipRemoveApply:   true,
			wantError:         true,
			wantMutationCount: 1,
			wantState:         true,
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newGroupPolicyBindingFixture(t)
			fixture.policyIDs = []string{providerGroupPolicyID}
			if test.startPresent {
				fixture.policyIDs = append([]string{providerPolicyID}, fixture.policyIDs...)
			}
			fixture.failRemoveBeforeApply = test.failBeforeApply
			fixture.failRemoveAfterApply = test.failAfterApply
			fixture.skipRemoveApply = test.skipRemoveApply
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			bindingSchema := groupPolicyBindingResourceTestSchema(t)
			state := groupPolicyBindingResourceTestState(t, bindingSchema, providerGroupID, providerPolicyID)
			var response frameworkresource.DeleteResponse
			newGroupPolicyBindingTestResource(apiClient).Delete(
				context.Background(),
				frameworkresource.DeleteRequest{State: state},
				&response,
			)
			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf("Delete() error = %t, want %t: %v", got, test.wantError, response.Diagnostics)
			}
			if got := !response.State.Raw.IsNull(); got != test.wantState {
				t.Fatalf("Delete() state present = %t, want %t; state = %s", got, test.wantState, response.State.Raw)
			}
			if got := len(fixture.mutations()); got != test.wantMutationCount {
				t.Fatalf("mutation count = %d, want %d: %v", got, test.wantMutationCount, fixture.mutations())
			}
			remaining := fixture.currentPolicyIDs()
			if !slices.Contains(remaining, providerGroupPolicyID) {
				t.Fatalf("Delete() removed unrelated binding: %v", remaining)
			}
		})
	}
}

type groupPolicyBindingFixture struct {
	t  *testing.T
	mu sync.Mutex

	group                 client.Group
	policy                client.Policy
	groupPresent          bool
	policyPresent         bool
	policyIDs             []string
	mutationNames         []string
	failRelationshipReads bool
	failAddBeforeApply    bool
	failAddAfterApply     bool
	failRemoveBeforeApply bool
	failRemoveAfterApply  bool
	skipRemoveApply       bool
}

func newGroupPolicyBindingFixture(t *testing.T) *groupPolicyBindingFixture {
	t.Helper()
	return &groupPolicyBindingFixture{
		t:             t,
		groupPresent:  true,
		policyPresent: true,
		group: client.Group{
			ID: providerGroupID, Name: "Managed Group", Description: "",
		},
		policy: client.Policy{
			ID: providerPolicyID, Name: "Managed Policy", Key: "managed-policy",
			Type: client.PolicyTypeCustomerManaged, Statements: []client.PolicyStatement{},
		},
	}
}

func (f *groupPolicyBindingFixture) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	groupBasePath := "/api/v1/groups"
	policyBasePath := "/api/v1/policies"
	groupPath := groupBasePath + "/" + providerGroupID
	policyPath := policyBasePath + "/" + providerPolicyID
	relationshipPath := groupPath + "/policies"
	switch {
	case request.Method == http.MethodGet && request.URL.EscapedPath() == groupBasePath:
		groups := []client.Group{}
		if f.groupPresent {
			groups = append(groups, f.group)
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(groups), "items": groups,
		})
	case request.Method == http.MethodGet && request.URL.EscapedPath() == groupPath:
		if !f.groupPresent {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, f.group)
	case request.Method == http.MethodGet && request.URL.EscapedPath() == policyBasePath:
		policies := []client.Policy{}
		if f.policyPresent {
			policies = append(policies, f.policy)
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(policies), "items": policies,
		})
	case request.Method == http.MethodGet && request.URL.EscapedPath() == policyPath:
		if !f.policyPresent {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, f.policy)
	case request.Method == http.MethodGet && request.URL.EscapedPath() == relationshipPath:
		if f.failRelationshipReads {
			writePolicyProviderEnvelope(f.t, response, http.StatusInternalServerError, nil)
			return
		}
		if !f.groupPresent {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		items := make([]map[string]any, 0, len(f.policyIDs))
		for _, policyID := range f.policyIDs {
			items = append(items, map[string]any{"id": policyID, "isGroupPolicy": true})
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(items), "items": items,
		})
	case request.Method == http.MethodPut &&
		request.URL.EscapedPath() == groupPath+"/add-policy/"+providerPolicyID:
		f.mutationNames = append(f.mutationNames, "add")
		if f.failAddBeforeApply {
			writePolicyProviderEnvelope(f.t, response, http.StatusInternalServerError, nil)
			return
		}
		if f.groupPresent && f.policyPresent && !slices.Contains(f.policyIDs, providerPolicyID) {
			f.policyIDs = append(f.policyIDs, providerPolicyID)
		}
		if f.failAddAfterApply {
			writePolicyProviderEnvelope(f.t, response, http.StatusInternalServerError, nil)
			return
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, true)
	case request.Method == http.MethodPut &&
		request.URL.EscapedPath() == groupPath+"/remove-policy/"+providerPolicyID:
		f.mutationNames = append(f.mutationNames, "remove")
		if f.failRemoveBeforeApply {
			writePolicyProviderEnvelope(f.t, response, http.StatusInternalServerError, nil)
			return
		}
		if !f.skipRemoveApply {
			f.removePolicyIDLocked(providerPolicyID)
		}
		if f.failRemoveAfterApply {
			writePolicyProviderEnvelope(f.t, response, http.StatusInternalServerError, nil)
			return
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, true)
	default:
		f.t.Fatalf("unexpected Group-Policy fixture request %s %s", request.Method, request.URL.EscapedPath())
	}
}

func (f *groupPolicyBindingFixture) mutations() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.mutationNames...)
}

func (f *groupPolicyBindingFixture) currentPolicyIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.policyIDs...)
}

func (f *groupPolicyBindingFixture) removePolicyID(policyID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removePolicyIDLocked(policyID)
}

func (f *groupPolicyBindingFixture) removePolicyIDLocked(policyID string) {
	for index, candidate := range f.policyIDs {
		if client.EqualUUID(candidate, policyID) {
			f.policyIDs = append(f.policyIDs[:index], f.policyIDs[index+1:]...)
			return
		}
	}
}

type groupPolicyBindingTestModel struct {
	ID       types.String `tfsdk:"id"`
	GroupID  types.String `tfsdk:"group_id"`
	PolicyID types.String `tfsdk:"policy_id"`
}

func newGroupPolicyBindingTestResource(apiClient *client.Client) *groupBindingResource {
	return &groupBindingResource{client: apiClient, kind: groupPolicyBindingKind}
}

func groupPolicyBindingResourceTestSchema(t *testing.T) resourceschema.Schema {
	t.Helper()
	var response frameworkresource.SchemaResponse
	newGroupPolicyBindingTestResource(nil).Schema(
		context.Background(), frameworkresource.SchemaRequest{}, &response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("binding schema diagnostics = %v", response.Diagnostics)
	}
	return response.Schema
}

func groupPolicyBindingResourceTestPlan(
	t *testing.T,
	bindingSchema resourceschema.Schema,
	groupID string,
	policyID string,
) tfsdk.Plan {
	t.Helper()
	plan := tfsdk.Plan{Schema: bindingSchema}
	model := groupPolicyBindingTestModel{
		ID: types.StringUnknown(), GroupID: types.StringValue(groupID),
		PolicyID: types.StringValue(policyID),
	}
	if diagnostics := plan.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize binding plan: %v", diagnostics)
	}
	return plan
}

func groupPolicyBindingResourceTestState(
	t *testing.T,
	bindingSchema resourceschema.Schema,
	groupID string,
	policyID string,
) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: bindingSchema}
	identity, err := canonicalizeGroupBindingPlanValues(
		types.StringValue(groupID),
		types.StringValue(policyID),
	)
	if err != nil {
		t.Fatalf("canonicalize binding test identity: %v", err)
	}
	model := groupPolicyBindingTestModel{
		ID:       types.StringValue(identity.syntheticID()),
		GroupID:  types.StringValue(identity.GroupID),
		PolicyID: types.StringValue(identity.TargetID),
	}
	if diagnostics := state.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize binding state: %v", diagnostics)
	}
	return state
}

func emptyGroupPolicyBindingResourceState(
	t *testing.T,
	bindingSchema resourceschema.Schema,
) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: bindingSchema}
	model := groupPolicyBindingTestModel{
		ID: types.StringNull(), GroupID: types.StringNull(), PolicyID: types.StringNull(),
	}
	if diagnostics := state.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize empty binding state: %v", diagnostics)
	}
	return state
}

func groupPolicyBindingStateModel(t *testing.T, state tfsdk.State) groupPolicyBindingTestModel {
	t.Helper()
	var model groupPolicyBindingTestModel
	if diagnostics := state.Get(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("read binding state: %v", diagnostics)
	}
	return model
}

func assertGroupPolicyBindingState(
	t *testing.T,
	state tfsdk.State,
	groupID string,
	policyID string,
) {
	t.Helper()
	model := groupPolicyBindingStateModel(t, state)
	if model.ID.ValueString() != groupID+"/"+policyID ||
		model.GroupID.ValueString() != groupID || model.PolicyID.ValueString() != policyID {
		t.Fatalf("binding state = %#v", model)
	}
}
