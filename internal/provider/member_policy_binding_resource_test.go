// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestMemberPolicyBindingResourceMetadataSchemaAndImport(t *testing.T) {
	t.Parallel()

	resourceUnderTest := newMemberPolicyBindingTestResource(nil)
	var metadata frameworkresource.MetadataResponse
	resourceUnderTest.Metadata(
		context.Background(),
		frameworkresource.MetadataRequest{ProviderTypeName: "featbit"},
		&metadata,
	)
	if metadata.TypeName != "featbit_member_policy_binding" {
		t.Fatalf("metadata type = %q", metadata.TypeName)
	}

	bindingSchema := memberPolicyBindingResourceTestSchema(t)
	if len(bindingSchema.Attributes) != 3 {
		t.Fatalf("binding schema attributes = %v", bindingSchema.Attributes)
	}
	idAttribute, idOK := bindingSchema.Attributes["id"].(resourceschema.StringAttribute)
	memberAttribute, memberOK := bindingSchema.Attributes["member_id"].(resourceschema.StringAttribute)
	policyAttribute, policyOK := bindingSchema.Attributes["policy_id"].(resourceschema.StringAttribute)
	if !idOK || !idAttribute.Computed || !idAttribute.Sensitive ||
		!memberOK || !memberAttribute.Required || !memberAttribute.Sensitive ||
		!policyOK || !policyAttribute.Required || policyAttribute.Sensitive {
		t.Fatalf("binding schema = %#v", bindingSchema.Attributes)
	}
	for name, attribute := range map[string]resourceschema.StringAttribute{
		"member_id": memberAttribute,
		"policy_id": policyAttribute,
	} {
		if len(attribute.Validators) != 1 || len(attribute.PlanModifiers) != 1 {
			t.Fatalf("%s schema = %#v", name, attribute)
		}
	}

	canonicalImportID := providerMemberID + "/" + providerDirectPolicyIDOne
	tests := map[string]struct {
		id        string
		wantError bool
	}{
		"canonical":               {id: canonicalImportID},
		"uppercase canonicalized": {id: strings.ToUpper(canonicalImportID)},
		"missing":                 {id: "", wantError: true},
		"Member only":             {id: providerMemberID, wantError: true},
		"invalid Member":          {id: "not-a-uuid/" + providerDirectPolicyIDOne, wantError: true},
		"invalid Policy":          {id: providerMemberID + "/not-a-uuid", wantError: true},
		"extra component":         {id: canonicalImportID + "/extra", wantError: true},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			response := frameworkresource.ImportStateResponse{
				State: emptyMemberPolicyBindingResourceState(t, bindingSchema),
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
			assertMemberPolicyBindingState(
				t,
				response.State,
				providerMemberID,
				providerDirectPolicyIDOne,
			)
		})
	}
}

func TestMemberPolicyBindingResourceOwnsOnlyExactDirectPair(t *testing.T) {
	t.Parallel()

	fixture := newMemberDirectPoliciesFixture(t)
	baseline := []string{providerDirectPolicyIDTwo, providerDirectPolicyIDThree}
	fixture.setDirectPolicyIDs(baseline)
	apiClient, closeServer := newProjectResourceTestClient(t, fixture)
	defer closeServer()

	bindingSchema := memberPolicyBindingResourceTestSchema(t)
	plan := memberPolicyBindingResourceTestPlan(
		t,
		bindingSchema,
		providerMemberID,
		providerDirectPolicyIDOne,
	)
	resourceUnderTest := newMemberPolicyBindingTestResource(apiClient)
	createResponse := frameworkresource.CreateResponse{
		State: emptyMemberPolicyBindingResourceState(t, bindingSchema),
	}
	resourceUnderTest.Create(
		context.Background(),
		frameworkresource.CreateRequest{Plan: plan},
		&createResponse,
	)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("Create() diagnostics = %v", createResponse.Diagnostics)
	}
	assertMemberPolicyBindingState(
		t,
		createResponse.State,
		providerMemberID,
		providerDirectPolicyIDOne,
	)
	assertCanonicalPolicyIDs(
		t,
		fixture.directPolicySnapshot(),
		append(append([]string(nil), baseline...), providerDirectPolicyIDOne),
	)
	if mutations := fixture.mutationSnapshot(); !slices.Equal(
		mutations,
		[]string{"add:" + providerDirectPolicyIDOne},
	) {
		t.Fatalf("Create() mutations = %v", mutations)
	}

	adoptResponse := frameworkresource.CreateResponse{
		State: emptyMemberPolicyBindingResourceState(t, bindingSchema),
	}
	resourceUnderTest.Create(
		context.Background(),
		frameworkresource.CreateRequest{Plan: plan},
		&adoptResponse,
	)
	if adoptResponse.Diagnostics.HasError() || len(fixture.mutationSnapshot()) != 1 {
		t.Fatalf("adopting Create() diagnostics/mutations = %v/%v", adoptResponse.Diagnostics, fixture.mutationSnapshot())
	}

	deleteResponse := frameworkresource.DeleteResponse{State: adoptResponse.State}
	resourceUnderTest.Delete(
		context.Background(),
		frameworkresource.DeleteRequest{State: adoptResponse.State},
		&deleteResponse,
	)
	if deleteResponse.Diagnostics.HasError() || !deleteResponse.State.Raw.IsNull() {
		t.Fatalf("Delete() diagnostics/state = %v/%s", deleteResponse.Diagnostics, deleteResponse.State.Raw)
	}
	assertCanonicalPolicyIDs(t, fixture.directPolicySnapshot(), baseline)
	if mutations := fixture.mutationSnapshot(); !slices.Equal(
		mutations,
		[]string{
			"add:" + providerDirectPolicyIDOne,
			"remove:" + providerDirectPolicyIDOne,
		},
	) {
		t.Fatalf("binding lifecycle mutations = %v", mutations)
	}
}

type memberPolicyBindingTestModel struct {
	ID       types.String `tfsdk:"id"`
	MemberID types.String `tfsdk:"member_id"`
	PolicyID types.String `tfsdk:"policy_id"`
}

func newMemberPolicyBindingTestResource(apiClient *client.Client) *groupBindingResource {
	return &groupBindingResource{client: apiClient, kind: memberPolicyBindingKind}
}

func memberPolicyBindingResourceTestSchema(t *testing.T) resourceschema.Schema {
	t.Helper()
	var response frameworkresource.SchemaResponse
	newMemberPolicyBindingTestResource(nil).Schema(
		context.Background(), frameworkresource.SchemaRequest{}, &response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Member-Policy binding schema diagnostics = %v", response.Diagnostics)
	}
	return response.Schema
}

func memberPolicyBindingResourceTestPlan(
	t *testing.T,
	bindingSchema resourceschema.Schema,
	memberID string,
	policyID string,
) tfsdk.Plan {
	t.Helper()
	plan := tfsdk.Plan{Schema: bindingSchema}
	model := memberPolicyBindingTestModel{
		ID:       types.StringUnknown(),
		MemberID: types.StringValue(memberID),
		PolicyID: types.StringValue(policyID),
	}
	if diagnostics := plan.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize Member-Policy binding plan: %v", diagnostics)
	}
	return plan
}

func emptyMemberPolicyBindingResourceState(
	t *testing.T,
	bindingSchema resourceschema.Schema,
) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: bindingSchema}
	model := memberPolicyBindingTestModel{
		ID: types.StringNull(), MemberID: types.StringNull(), PolicyID: types.StringNull(),
	}
	if diagnostics := state.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize empty Member-Policy binding state: %v", diagnostics)
	}
	return state
}

func assertMemberPolicyBindingState(
	t *testing.T,
	state tfsdk.State,
	memberID string,
	policyID string,
) {
	t.Helper()
	var model memberPolicyBindingTestModel
	if diagnostics := state.Get(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("read Member-Policy binding state: %v", diagnostics)
	}
	if model.ID.ValueString() != memberID+"/"+policyID ||
		model.MemberID.ValueString() != memberID ||
		model.PolicyID.ValueString() != policyID {
		t.Fatalf("Member-Policy binding state = %#v", model)
	}
}
