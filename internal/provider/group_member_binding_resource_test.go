// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
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

func TestGroupMemberBindingResourceMetadataConfigureSchemaAndImport(t *testing.T) {
	t.Parallel()

	resourceUnderTest := newGroupMemberBindingTestResource(nil)
	var metadata frameworkresource.MetadataResponse
	resourceUnderTest.Metadata(
		context.Background(),
		frameworkresource.MetadataRequest{ProviderTypeName: "featbit"},
		&metadata,
	)
	if metadata.TypeName != "featbit_group_member_binding" {
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

	bindingSchema := groupMemberBindingResourceTestSchema(t)
	if len(bindingSchema.Attributes) != 3 {
		t.Fatalf("binding schema attributes = %v", bindingSchema.Attributes)
	}
	idAttribute, ok := bindingSchema.Attributes["id"].(resourceschema.StringAttribute)
	if !ok || !idAttribute.Computed || idAttribute.Required || idAttribute.Optional ||
		!idAttribute.Sensitive || len(idAttribute.PlanModifiers) != 1 {
		t.Fatalf("id schema = %#v", bindingSchema.Attributes["id"])
	}
	groupIDAttribute, ok := bindingSchema.Attributes["group_id"].(resourceschema.StringAttribute)
	if !ok || !groupIDAttribute.Required || groupIDAttribute.Optional || groupIDAttribute.Computed ||
		groupIDAttribute.Sensitive || len(groupIDAttribute.Validators) != 1 ||
		len(groupIDAttribute.PlanModifiers) != 1 {
		t.Fatalf("group_id schema = %#v", bindingSchema.Attributes["group_id"])
	}
	memberIDAttribute, ok := bindingSchema.Attributes["member_id"].(resourceschema.StringAttribute)
	if !ok || !memberIDAttribute.Required || memberIDAttribute.Optional || memberIDAttribute.Computed ||
		!memberIDAttribute.Sensitive || len(memberIDAttribute.Validators) != 1 ||
		len(memberIDAttribute.PlanModifiers) != 1 {
		t.Fatalf("member_id schema = %#v", bindingSchema.Attributes["member_id"])
	}

	priorState := groupMemberBindingResourceTestState(
		t, bindingSchema, providerGroupID, providerMemberID,
	)
	var stableIDResponse planmodifier.StringResponse
	stableIDResponse.PlanValue = types.StringUnknown()
	idAttribute.PlanModifiers[0].PlanModifyString(
		context.Background(),
		planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			PlanValue:   types.StringUnknown(),
			StateValue:  types.StringValue(providerGroupID + "/" + providerMemberID),
			Plan: groupMemberBindingResourceTestPlan(
				t, bindingSchema, providerGroupID, providerMemberID,
			),
			State: priorState,
		},
		&stableIDResponse,
	)
	if stableIDResponse.Diagnostics.HasError() ||
		stableIDResponse.PlanValue.ValueString() != providerGroupID+"/"+providerMemberID {
		t.Fatalf("stable id modifier response = %#v", stableIDResponse)
	}
	var replacementIDResponse planmodifier.StringResponse
	replacementIDResponse.PlanValue = types.StringUnknown()
	idAttribute.PlanModifiers[0].PlanModifyString(
		context.Background(),
		planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			PlanValue:   types.StringUnknown(),
			StateValue:  types.StringValue(providerGroupID + "/" + providerMemberID),
			Plan: groupMemberBindingResourceTestPlan(
				t, bindingSchema, providerGroupIDTwo, providerMemberID,
			),
			State: priorState,
		},
		&replacementIDResponse,
	)
	if replacementIDResponse.Diagnostics.HasError() || !replacementIDResponse.PlanValue.IsUnknown() {
		t.Fatalf("replacement id modifier response = %#v", replacementIDResponse)
	}
	var targetReplacementIDResponse planmodifier.StringResponse
	targetReplacementIDResponse.PlanValue = types.StringUnknown()
	idAttribute.PlanModifiers[0].PlanModifyString(
		context.Background(),
		planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			PlanValue:   types.StringUnknown(),
			StateValue:  types.StringValue(providerGroupID + "/" + providerMemberID),
			Plan: groupMemberBindingResourceTestPlan(
				t, bindingSchema, providerGroupID, providerMemberIDTwo,
			),
			State: priorState,
		},
		&targetReplacementIDResponse,
	)
	if targetReplacementIDResponse.Diagnostics.HasError() ||
		!targetReplacementIDResponse.PlanValue.IsUnknown() {
		t.Fatalf("target replacement id modifier response = %#v", targetReplacementIDResponse)
	}

	canonicalImportID := providerGroupID + "/" + providerMemberID
	tests := map[string]struct {
		id        string
		wantError bool
	}{
		"canonical":               {id: canonicalImportID},
		"uppercase canonicalized": {id: strings.ToUpper(canonicalImportID)},
		"missing":                 {id: "", wantError: true},
		"Group only":              {id: providerGroupID, wantError: true},
		"invalid Group":           {id: "not-a-uuid/" + providerMemberID, wantError: true},
		"invalid Member":          {id: providerGroupID + "/not-a-uuid", wantError: true},
		"extra component":         {id: canonicalImportID + "/extra", wantError: true},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			response := frameworkresource.ImportStateResponse{
				State: emptyGroupMemberBindingResourceState(t, bindingSchema),
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
			state := groupMemberBindingStateModel(t, response.State)
			if state.ID.ValueString() != canonicalImportID ||
				state.GroupID.ValueString() != providerGroupID ||
				state.MemberID.ValueString() != providerMemberID {
				t.Fatalf("ImportState() state = %#v", state)
			}
		})
	}
}

func TestGroupMemberBindingResourceCreateAdoptsOrReconcilesExactPair(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		preExisting       bool
		wantMutationCount int
	}{
		"create": {
			wantMutationCount: 1,
		},
		"pre-existing binding": {
			preExisting: true,
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newGroupMemberBindingFixture(t)
			if test.preExisting {
				fixture.memberIDs = []string{providerMemberID}
			}
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			bindingSchema := groupMemberBindingResourceTestSchema(t)
			response := frameworkresource.CreateResponse{
				State: emptyGroupMemberBindingResourceState(t, bindingSchema),
			}
			newGroupMemberBindingTestResource(apiClient).Create(
				context.Background(),
				frameworkresource.CreateRequest{Plan: groupMemberBindingResourceTestPlan(
					t, bindingSchema, providerGroupID, providerMemberID,
				)},
				&response,
			)
			if response.Diagnostics.HasError() {
				t.Fatalf("Create() diagnostics = %v", response.Diagnostics)
			}
			stateModel := groupMemberBindingStateModel(t, response.State)
			if !knownString(stateModel.ID) {
				t.Fatalf("Create() did not record state: %#v", stateModel)
			}
			if got := len(fixture.mutations()); got != test.wantMutationCount {
				t.Fatalf("mutation count = %d, want %d: %v", got, test.wantMutationCount, fixture.mutations())
			}
			assertGroupMemberBindingState(t, response.State, providerGroupID, providerMemberID)
			var readResponse frameworkresource.ReadResponse
			newGroupMemberBindingTestResource(apiClient).Read(
				context.Background(),
				frameworkresource.ReadRequest{State: response.State},
				&readResponse,
			)
			if readResponse.Diagnostics.HasError() || readResponse.State.Raw.IsNull() {
				t.Fatalf("Read() = %v/%s", readResponse.Diagnostics, readResponse.State.Raw)
			}
			if got := len(fixture.mutations()); got != test.wantMutationCount {
				t.Fatalf("Read() changed mutation count to %d", got)
			}
		})
	}
}

func TestGroupMemberBindingResourceCreateRequiresExactEndpointsAndReadableCollection(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*groupMemberBindingFixture){
		"missing Member": func(fixture *groupMemberBindingFixture) {
			fixture.memberPresent = false
		},
		"ambiguous relationship collection": func(fixture *groupMemberBindingFixture) {
			fixture.failRelationshipReads = true
		},
	}
	for name, arrange := range tests {
		name := name
		arrange := arrange
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newGroupMemberBindingFixture(t)
			arrange(fixture)
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			bindingSchema := groupMemberBindingResourceTestSchema(t)
			response := frameworkresource.CreateResponse{
				State: emptyGroupMemberBindingResourceState(t, bindingSchema),
			}
			newGroupMemberBindingTestResource(apiClient).Create(
				context.Background(),
				frameworkresource.CreateRequest{Plan: groupMemberBindingResourceTestPlan(
					t, bindingSchema, providerGroupID, providerMemberID,
				)},
				&response,
			)
			stateModel := groupMemberBindingStateModel(t, response.State)
			if !response.Diagnostics.HasError() || knownString(stateModel.ID) || len(fixture.mutations()) != 0 {
				t.Fatalf("Create() diagnostics/state/mutations = %v/%#v/%v", response.Diagnostics, stateModel, fixture.mutations())
			}
			formatted := fmt.Sprint(response.Diagnostics)
			for _, unsafe := range []string{
				providerGroupID, providerMemberID, fixture.member.Email, fixture.member.Name,
				"/api/v1/groups", "/api/v1/members",
			} {
				if strings.Contains(formatted, unsafe) {
					t.Fatalf("Create() diagnostic exposed runtime identity %q", unsafe)
				}
			}
		})
	}
}

func TestGroupMemberBindingResourceReadTracksExactPairDriftAndAbsence(t *testing.T) {
	t.Parallel()

	fixture := newGroupMemberBindingFixture(t)
	fixture.memberIDs = []string{providerMemberID, providerMemberIDTwo}
	apiClient, closeServer := newProjectResourceTestClient(t, fixture)
	defer closeServer()
	bindingSchema := groupMemberBindingResourceTestSchema(t)
	state := groupMemberBindingResourceTestState(t, bindingSchema, providerGroupID, providerMemberID)

	var presentResponse frameworkresource.ReadResponse
	newGroupMemberBindingTestResource(apiClient).Read(
		context.Background(),
		frameworkresource.ReadRequest{State: state},
		&presentResponse,
	)
	if presentResponse.Diagnostics.HasError() || presentResponse.State.Raw.IsNull() {
		t.Fatalf("Read() present diagnostics/state = %v/%s", presentResponse.Diagnostics, presentResponse.State.Raw)
	}

	fixture.removeMemberID(providerMemberID)
	var driftResponse frameworkresource.ReadResponse
	newGroupMemberBindingTestResource(apiClient).Read(
		context.Background(),
		frameworkresource.ReadRequest{State: presentResponse.State},
		&driftResponse,
	)
	if driftResponse.Diagnostics.HasError() || !driftResponse.State.Raw.IsNull() {
		t.Fatalf("Read() drift diagnostics/state = %v/%s", driftResponse.Diagnostics, driftResponse.State.Raw)
	}
	if got := fixture.currentMemberIDs(); len(got) != 1 || got[0] != providerMemberIDTwo {
		t.Fatalf("unrelated binding changed during Read: %v", got)
	}
}

func TestGroupMemberBindingResourceReadTreatsConfirmedMissingMemberAsAbsent(t *testing.T) {
	t.Parallel()

	fixture := newGroupMemberBindingFixture(t)
	fixture.memberIDs = []string{providerMemberID}
	fixture.memberPresent = false
	fixture.failRelationshipReads = true
	apiClient, closeServer := newProjectResourceTestClient(t, fixture)
	defer closeServer()
	bindingSchema := groupMemberBindingResourceTestSchema(t)
	state := groupMemberBindingResourceTestState(t, bindingSchema, providerGroupID, providerMemberID)
	var response frameworkresource.ReadResponse
	newGroupMemberBindingTestResource(apiClient).Read(
		context.Background(),
		frameworkresource.ReadRequest{State: state},
		&response,
	)
	if response.Diagnostics.HasError() || !response.State.Raw.IsNull() {
		t.Fatalf("Read() diagnostics/state = %v/%s", response.Diagnostics, response.State.Raw)
	}
}

func TestGroupMemberBindingResourceDeleteRemovesOnlyExactPairAndReconciles(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		startPresent  bool
		missingMember bool
		wantMutations []string
	}{
		"success": {
			startPresent: true, wantMutations: []string{"remove"},
		},
		"already absent": {},
		"confirmed missing Member": {
			startPresent: true, missingMember: true,
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newGroupMemberBindingFixture(t)
			fixture.memberIDs = []string{providerMemberIDTwo}
			if test.startPresent {
				fixture.memberIDs = append([]string{providerMemberID}, fixture.memberIDs...)
			}
			fixture.memberPresent = !test.missingMember
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			bindingSchema := groupMemberBindingResourceTestSchema(t)
			state := groupMemberBindingResourceTestState(t, bindingSchema, providerGroupID, providerMemberID)
			var response frameworkresource.DeleteResponse
			newGroupMemberBindingTestResource(apiClient).Delete(
				context.Background(),
				frameworkresource.DeleteRequest{State: state},
				&response,
			)
			if response.Diagnostics.HasError() {
				t.Fatalf("Delete() diagnostics = %v", response.Diagnostics)
			}
			if !response.State.Raw.IsNull() {
				t.Fatalf("Delete() preserved state = %s", response.State.Raw)
			}
			if got := fixture.mutations(); fmt.Sprint(got) != fmt.Sprint(test.wantMutations) {
				t.Fatalf("mutations = %v, want %v", got, test.wantMutations)
			}
			remaining := fixture.currentMemberIDs()
			if !containsExactString(remaining, providerMemberIDTwo) {
				t.Fatalf("Delete() removed unrelated binding: %v", remaining)
			}
		})
	}
}

type groupMemberBindingFixture struct {
	t  *testing.T
	mu sync.Mutex

	group                 client.Group
	member                client.Member
	groupPresent          bool
	memberPresent         bool
	memberIDs             []string
	mutationNames         []string
	failRelationshipReads bool
}

func newGroupMemberBindingFixture(t *testing.T) *groupMemberBindingFixture {
	t.Helper()
	return &groupMemberBindingFixture{
		t:             t,
		groupPresent:  true,
		memberPresent: true,
		group: client.Group{
			ID: providerGroupID, Name: "Managed Group", Description: "",
		},
		member: client.Member{
			ID: providerMemberID, Email: "member@example.invalid", Name: "Managed Member",
		},
	}
}

func (f *groupMemberBindingFixture) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	groupBasePath := "/api/v1/groups"
	groupPath := groupBasePath + "/" + providerGroupID
	memberBasePath := "/api/v1/members"
	memberPath := memberBasePath + "/" + providerMemberID
	relationshipPath := groupPath + "/members"

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
	case request.Method == http.MethodGet && request.URL.EscapedPath() == memberBasePath:
		members := []map[string]any{}
		if f.memberPresent {
			members = append(members, map[string]any{
				"id": f.member.ID, "email": f.member.Email, "name": f.member.Name,
				"initialPassword": "must-not-be-decoded",
			})
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(members), "items": members,
		})
	case request.Method == http.MethodGet && request.URL.EscapedPath() == memberPath:
		if !f.memberPresent {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"id": f.member.ID, "email": f.member.Email, "name": f.member.Name,
			"initialPassword": "must-not-be-decoded",
		})
	case request.Method == http.MethodGet && request.URL.EscapedPath() == relationshipPath:
		if f.failRelationshipReads {
			writePolicyProviderEnvelope(f.t, response, http.StatusInternalServerError, nil)
			return
		}
		if !f.groupPresent {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		items := make([]map[string]any, 0, len(f.memberIDs))
		for _, memberID := range f.memberIDs {
			items = append(items, map[string]any{
				"id": memberID, "isGroupMember": true,
				"email":           "must-not-be-decoded@example.invalid",
				"initialPassword": "must-not-be-decoded",
			})
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(items), "items": items,
		})
	case request.Method == http.MethodPut &&
		request.URL.EscapedPath() == groupPath+"/add-member/"+providerMemberID:
		f.mutationNames = append(f.mutationNames, "add")
		if f.groupPresent && f.memberPresent && !containsExactString(f.memberIDs, providerMemberID) {
			f.memberIDs = append(f.memberIDs, providerMemberID)
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, true)
	case request.Method == http.MethodPut &&
		request.URL.EscapedPath() == groupPath+"/remove-member/"+providerMemberID:
		f.mutationNames = append(f.mutationNames, "remove")
		f.removeMemberIDLocked(providerMemberID)
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, true)
	default:
		f.t.Fatalf("unexpected Group-Member fixture request %s %s", request.Method, request.URL.EscapedPath())
	}
}

func (f *groupMemberBindingFixture) mutations() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.mutationNames...)
}

func (f *groupMemberBindingFixture) currentMemberIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.memberIDs...)
}

func (f *groupMemberBindingFixture) removeMemberID(memberID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removeMemberIDLocked(memberID)
}

func (f *groupMemberBindingFixture) removeMemberIDLocked(memberID string) {
	for index, candidate := range f.memberIDs {
		if client.EqualUUID(candidate, memberID) {
			f.memberIDs = append(f.memberIDs[:index], f.memberIDs[index+1:]...)
			return
		}
	}
}

type groupMemberBindingTestModel struct {
	ID       types.String `tfsdk:"id"`
	GroupID  types.String `tfsdk:"group_id"`
	MemberID types.String `tfsdk:"member_id"`
}

func newGroupMemberBindingTestResource(apiClient *client.Client) *groupBindingResource {
	return &groupBindingResource{client: apiClient, kind: groupMemberBindingKind}
}

func groupMemberBindingResourceTestSchema(t *testing.T) resourceschema.Schema {
	t.Helper()
	var response frameworkresource.SchemaResponse
	newGroupMemberBindingTestResource(nil).Schema(
		context.Background(), frameworkresource.SchemaRequest{}, &response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("binding schema diagnostics = %v", response.Diagnostics)
	}
	return response.Schema
}

func groupMemberBindingResourceTestPlan(
	t *testing.T,
	bindingSchema resourceschema.Schema,
	groupID string,
	memberID string,
) tfsdk.Plan {
	t.Helper()
	plan := tfsdk.Plan{Schema: bindingSchema}
	model := groupMemberBindingTestModel{
		ID: types.StringUnknown(), GroupID: types.StringValue(groupID),
		MemberID: types.StringValue(memberID),
	}
	if diagnostics := plan.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize binding plan: %v", diagnostics)
	}
	return plan
}

func groupMemberBindingResourceTestState(
	t *testing.T,
	bindingSchema resourceschema.Schema,
	groupID string,
	memberID string,
) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: bindingSchema}
	identity, err := canonicalizeGroupBindingPlanValues(
		types.StringValue(groupID),
		types.StringValue(memberID),
	)
	if err != nil {
		t.Fatalf("canonicalize binding test identity: %v", err)
	}
	model := groupMemberBindingTestModel{
		ID:       types.StringValue(identity.syntheticID()),
		GroupID:  types.StringValue(identity.GroupID),
		MemberID: types.StringValue(identity.TargetID),
	}
	if diagnostics := state.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize binding state: %v", diagnostics)
	}
	return state
}

func emptyGroupMemberBindingResourceState(
	t *testing.T,
	bindingSchema resourceschema.Schema,
) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: bindingSchema}
	model := groupMemberBindingTestModel{
		ID: types.StringNull(), GroupID: types.StringNull(), MemberID: types.StringNull(),
	}
	if diagnostics := state.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize empty binding state: %v", diagnostics)
	}
	return state
}

func groupMemberBindingStateModel(t *testing.T, state tfsdk.State) groupMemberBindingTestModel {
	t.Helper()
	var model groupMemberBindingTestModel
	if diagnostics := state.Get(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("read binding state: %v", diagnostics)
	}
	return model
}

func assertGroupMemberBindingState(
	t *testing.T,
	state tfsdk.State,
	groupID string,
	memberID string,
) {
	t.Helper()
	model := groupMemberBindingStateModel(t, state)
	if model.ID.ValueString() != groupID+"/"+memberID ||
		model.GroupID.ValueString() != groupID || model.MemberID.ValueString() != memberID {
		t.Fatalf("binding state = %#v", model)
	}
}
