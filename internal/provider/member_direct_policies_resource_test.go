// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	frameworkvalidator "github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	providerDirectPolicyIDOne   = "11111111-1111-4111-8111-111111111111"
	providerDirectPolicyIDTwo   = "22222222-2222-4222-8222-222222222222"
	providerDirectPolicyIDThree = "33333333-3333-4333-8333-333333333333"
)

func TestMemberDirectPoliciesResourceMetadataConfigureSchemaAndImport(t *testing.T) {
	t.Parallel()

	resourceUnderTest := newMemberDirectPoliciesTestResource(nil)
	var metadata frameworkresource.MetadataResponse
	resourceUnderTest.Metadata(
		context.Background(),
		frameworkresource.MetadataRequest{ProviderTypeName: "featbit"},
		&metadata,
	)
	if metadata.TypeName != "featbit_member_direct_policies" {
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
		t.Fatalf(
			"Configure() diagnostics/client = %v/%p",
			configure.Diagnostics,
			resourceUnderTest.client,
		)
	}

	directSchema := memberDirectPoliciesResourceTestSchema(t)
	if len(directSchema.Attributes) != 3 {
		t.Fatalf("resource schema attributes = %v", directSchema.Attributes)
	}
	idAttribute, ok := directSchema.Attributes["id"].(resourceschema.StringAttribute)
	if !ok || !idAttribute.Computed || idAttribute.Required || idAttribute.Optional ||
		!idAttribute.Sensitive || len(idAttribute.PlanModifiers) != 1 {
		t.Fatalf("id schema = %#v", directSchema.Attributes["id"])
	}
	memberIDAttribute, ok := directSchema.Attributes["member_id"].(resourceschema.StringAttribute)
	if !ok || !memberIDAttribute.Required || memberIDAttribute.Optional ||
		memberIDAttribute.Computed || !memberIDAttribute.Sensitive ||
		len(memberIDAttribute.Validators) != 1 || len(memberIDAttribute.PlanModifiers) != 1 {
		t.Fatalf("member_id schema = %#v", directSchema.Attributes["member_id"])
	}
	policyIDsAttribute, ok := directSchema.Attributes["policy_ids"].(resourceschema.SetAttribute)
	if !ok || !policyIDsAttribute.Required || policyIDsAttribute.Optional ||
		policyIDsAttribute.Computed || policyIDsAttribute.Sensitive ||
		policyIDsAttribute.ElementType != types.StringType ||
		len(policyIDsAttribute.Validators) != 1 {
		t.Fatalf("policy_ids schema = %#v", directSchema.Attributes["policy_ids"])
	}
	var invalidSetResponse frameworkvalidator.SetResponse
	policyIDsAttribute.Validators[0].ValidateSet(
		context.Background(),
		frameworkvalidator.SetRequest{
			ConfigValue: terraformStringSetValue([]string{"not-a-uuid"}),
		},
		&invalidSetResponse,
	)
	if !invalidSetResponse.Diagnostics.HasError() {
		t.Fatal("policy_ids validator accepted an invalid UUID")
	}
	var emptySetResponse frameworkvalidator.SetResponse
	policyIDsAttribute.Validators[0].ValidateSet(
		context.Background(),
		frameworkvalidator.SetRequest{
			ConfigValue: terraformStringSetValue(nil),
		},
		&emptySetResponse,
	)
	if emptySetResponse.Diagnostics.HasError() {
		t.Fatalf("policy_ids validator rejected an empty set: %v", emptySetResponse.Diagnostics)
	}

	priorState := memberDirectPoliciesResourceTestState(
		t,
		directSchema,
		providerMemberID,
		[]string{providerDirectPolicyIDOne},
	)
	var stableIDResponse planmodifier.StringResponse
	stableIDResponse.PlanValue = types.StringUnknown()
	idAttribute.PlanModifiers[0].PlanModifyString(
		context.Background(),
		planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			PlanValue:   types.StringUnknown(),
			StateValue:  types.StringValue(providerMemberID),
			Plan: memberDirectPoliciesResourceTestPlan(
				t,
				directSchema,
				providerMemberID,
				[]string{providerDirectPolicyIDTwo},
			),
			State: priorState,
		},
		&stableIDResponse,
	)
	if stableIDResponse.Diagnostics.HasError() ||
		stableIDResponse.PlanValue.ValueString() != providerMemberID {
		t.Fatalf("stable id modifier response = %#v", stableIDResponse)
	}
	var replacementIDResponse planmodifier.StringResponse
	replacementIDResponse.PlanValue = types.StringUnknown()
	idAttribute.PlanModifiers[0].PlanModifyString(
		context.Background(),
		planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			PlanValue:   types.StringUnknown(),
			StateValue:  types.StringValue(providerMemberID),
			Plan: memberDirectPoliciesResourceTestPlan(
				t,
				directSchema,
				providerMemberIDTwo,
				[]string{providerDirectPolicyIDOne},
			),
			State: priorState,
		},
		&replacementIDResponse,
	)
	if replacementIDResponse.Diagnostics.HasError() ||
		!replacementIDResponse.PlanValue.IsUnknown() {
		t.Fatalf("replacement id modifier response = %#v", replacementIDResponse)
	}

	tests := map[string]struct {
		id        string
		wantError bool
	}{
		"canonical": {id: providerMemberID},
		"missing":   {id: "", wantError: true},
		"invalid":   {id: "not-a-uuid", wantError: true},
		"composite": {id: providerMemberID + "/extra", wantError: true},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			response := frameworkresource.ImportStateResponse{
				State: emptyMemberDirectPoliciesResourceState(t, directSchema),
			}
			resourceUnderTest.ImportState(
				context.Background(),
				frameworkresource.ImportStateRequest{ID: test.id},
				&response,
			)
			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf(
					"ImportState() error = %t, want %t: %v",
					got,
					test.wantError,
					response.Diagnostics,
				)
			}
			if test.wantError {
				if test.id != "" && strings.Contains(fmt.Sprint(response.Diagnostics), test.id) {
					t.Fatal("ImportState() diagnostic echoed the rejected identifier")
				}
				return
			}
			state := memberDirectPoliciesStateModel(t, response.State)
			if state.ID.ValueString() != providerMemberID ||
				state.MemberID.ValueString() != providerMemberID ||
				!state.PolicyIDs.IsNull() {
				t.Fatalf("ImportState() state = %#v", state)
			}
		})
	}
}

func TestMemberDirectPoliciesModelsCanonicalizeAndRedact(t *testing.T) {
	t.Parallel()

	model := memberDirectPoliciesModel{
		ID:       types.StringValue(strings.ToUpper(providerMemberID)),
		MemberID: types.StringValue(strings.ToUpper(providerMemberID)),
		PolicyIDs: terraformStringSetValue([]string{
			strings.ToUpper(providerDirectPolicyIDTwo),
			providerDirectPolicyIDOne,
			providerDirectPolicyIDTwo,
		}),
	}
	canonical, err := canonicalizeMemberDirectPoliciesState(context.Background(), model)
	if err != nil || canonical.MemberID != providerMemberID ||
		!slices.Equal(canonical.PolicyIDs, []string{
			providerDirectPolicyIDOne,
			providerDirectPolicyIDTwo,
		}) {
		t.Fatalf("canonical state = %#v/%v", canonical, err)
	}
	model.ID = types.StringValue(providerMemberIDTwo)
	if _, err := canonicalizeMemberDirectPoliciesState(context.Background(), model); err == nil {
		t.Fatal("inconsistent Member direct-Policy state was accepted")
	}

	formatted := fmt.Sprintf("%v|%+v|%#v|%v", model, model, model, canonical)
	for _, unsafe := range []string{
		providerMemberID,
		providerMemberIDTwo,
		providerDirectPolicyIDOne,
		providerDirectPolicyIDTwo,
	} {
		if strings.Contains(formatted, unsafe) {
			t.Fatalf("direct-Policy model formatting exposed runtime identity %q", unsafe)
		}
	}
}

func TestMemberDirectPoliciesResourceCreateReconcilesAuthoritativeSet(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		current       []string
		desired       []string
		wantMutations []string
	}{
		"adds custom and built-in before removing extras in canonical order": {
			current: []string{providerDirectPolicyIDThree},
			desired: []string{
				providerDirectPolicyIDTwo,
				providerDirectPolicyIDOne,
			},
			wantMutations: []string{
				"add:" + providerDirectPolicyIDOne,
				"add:" + providerDirectPolicyIDTwo,
				"remove:" + providerDirectPolicyIDThree,
			},
		},
		"empty set removes every direct Policy": {
			current: []string{
				providerDirectPolicyIDTwo,
				providerDirectPolicyIDOne,
			},
			desired: nil,
			wantMutations: []string{
				"remove:" + providerDirectPolicyIDOne,
				"remove:" + providerDirectPolicyIDTwo,
			},
		},
		"already exact is idempotent": {
			current: []string{
				providerDirectPolicyIDTwo,
				providerDirectPolicyIDOne,
			},
			desired: []string{
				providerDirectPolicyIDOne,
				providerDirectPolicyIDTwo,
			},
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newMemberDirectPoliciesFixture(t)
			fixture.directPolicyIDs = append([]string(nil), test.current...)
			inheritedBaseline := fixture.inheritedPolicySnapshot()
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			directSchema := memberDirectPoliciesResourceTestSchema(t)
			response := frameworkresource.CreateResponse{
				State: emptyMemberDirectPoliciesResourceState(t, directSchema),
			}
			newMemberDirectPoliciesTestResource(apiClient).Create(
				context.Background(),
				frameworkresource.CreateRequest{Plan: memberDirectPoliciesResourceTestPlan(
					t,
					directSchema,
					providerMemberID,
					test.desired,
				)},
				&response,
			)
			if response.Diagnostics.HasError() {
				t.Fatalf("Create() diagnostics = %v", response.Diagnostics)
			}
			assertMemberDirectPoliciesState(
				t,
				response.State,
				providerMemberID,
				test.desired,
			)
			if got := fixture.mutationSnapshot(); !slices.Equal(got, test.wantMutations) {
				t.Fatalf("mutations = %v, want %v", got, test.wantMutations)
			}
			assertCanonicalPolicyIDs(t, fixture.directPolicySnapshot(), test.desired)
			if got := fixture.directReadCount(); got != len(test.wantMutations)+2 {
				t.Fatalf("direct rereads = %d, want %d", got, len(test.wantMutations)+2)
			}
			wantPolicyReads := 0
			if len(test.desired) != 0 {
				wantPolicyReads = 1
			}
			if got := fixture.policyReadCount(); got != wantPolicyReads {
				t.Fatalf("Policy collection reads = %d, want %d", got, wantPolicyReads)
			}
			if !slices.Equal(fixture.inheritedPolicySnapshot(), inheritedBaseline) {
				t.Fatal("Create() changed inherited Policies")
			}
		})
	}
}

func TestMemberDirectPoliciesResourcePreservesConfirmedPartialState(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		current       []string
		desired       []string
		failBefore    string
		failAfter     string
		skipApply     string
		failReadAt    int
		wantError     bool
		wantState     []string
		wantRemote    []string
		wantMutations []string
	}{
		"second add fails before apply": {
			desired: []string{
				providerDirectPolicyIDOne,
				providerDirectPolicyIDTwo,
			},
			failBefore: "add:" + providerDirectPolicyIDTwo,
			wantError:  true,
			wantState:  []string{providerDirectPolicyIDOne},
			wantRemote: []string{providerDirectPolicyIDOne},
			wantMutations: []string{
				"add:" + providerDirectPolicyIDOne,
				"add:" + providerDirectPolicyIDTwo,
			},
		},
		"second add ambiguous after apply reconciles": {
			desired: []string{
				providerDirectPolicyIDOne,
				providerDirectPolicyIDTwo,
			},
			failAfter: "add:" + providerDirectPolicyIDTwo,
			wantState: []string{
				providerDirectPolicyIDOne,
				providerDirectPolicyIDTwo,
			},
			wantRemote: []string{
				providerDirectPolicyIDOne,
				providerDirectPolicyIDTwo,
			},
			wantMutations: []string{
				"add:" + providerDirectPolicyIDOne,
				"add:" + providerDirectPolicyIDTwo,
			},
		},
		"second remove fails before apply": {
			current: []string{
				providerDirectPolicyIDTwo,
				providerDirectPolicyIDOne,
			},
			failBefore: "remove:" + providerDirectPolicyIDTwo,
			wantError:  true,
			wantState:  []string{providerDirectPolicyIDTwo},
			wantRemote: []string{providerDirectPolicyIDTwo},
			wantMutations: []string{
				"remove:" + providerDirectPolicyIDOne,
				"remove:" + providerDirectPolicyIDTwo,
			},
		},
		"second remove ambiguous after apply reconciles": {
			current: []string{
				providerDirectPolicyIDTwo,
				providerDirectPolicyIDOne,
			},
			failAfter: "remove:" + providerDirectPolicyIDTwo,
			wantMutations: []string{
				"remove:" + providerDirectPolicyIDOne,
				"remove:" + providerDirectPolicyIDTwo,
			},
		},
		"successful add no-op is rejected": {
			current:       []string{providerDirectPolicyIDThree},
			desired:       []string{providerDirectPolicyIDOne},
			skipApply:     "add:" + providerDirectPolicyIDOne,
			wantError:     true,
			wantState:     []string{providerDirectPolicyIDThree},
			wantRemote:    []string{providerDirectPolicyIDThree},
			wantMutations: []string{"add:" + providerDirectPolicyIDOne},
		},
		"successful remove no-op is rejected": {
			current:       []string{providerDirectPolicyIDOne},
			skipApply:     "remove:" + providerDirectPolicyIDOne,
			wantError:     true,
			wantState:     []string{providerDirectPolicyIDOne},
			wantRemote:    []string{providerDirectPolicyIDOne},
			wantMutations: []string{"remove:" + providerDirectPolicyIDOne},
		},
		"mutation reread failure keeps prior confirmation": {
			desired:       []string{providerDirectPolicyIDOne},
			failReadAt:    2,
			wantError:     true,
			wantState:     nil,
			wantRemote:    []string{providerDirectPolicyIDOne},
			wantMutations: []string{"add:" + providerDirectPolicyIDOne},
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newMemberDirectPoliciesFixture(t)
			fixture.directPolicyIDs = append([]string(nil), test.current...)
			fixture.failBeforeMutation = test.failBefore
			fixture.failAfterMutation = test.failAfter
			fixture.skipMutationApply = test.skipApply
			fixture.failDirectReadAt = test.failReadAt
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			directSchema := memberDirectPoliciesResourceTestSchema(t)
			response := frameworkresource.CreateResponse{
				State: emptyMemberDirectPoliciesResourceState(t, directSchema),
			}
			newMemberDirectPoliciesTestResource(apiClient).Create(
				context.Background(),
				frameworkresource.CreateRequest{Plan: memberDirectPoliciesResourceTestPlan(
					t,
					directSchema,
					providerMemberID,
					test.desired,
				)},
				&response,
			)
			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf(
					"Create() error = %t, want %t: %v",
					got,
					test.wantError,
					response.Diagnostics,
				)
			}
			assertMemberDirectPoliciesState(
				t,
				response.State,
				providerMemberID,
				test.wantState,
			)
			assertCanonicalPolicyIDs(t, fixture.directPolicySnapshot(), test.wantRemote)
			if got := fixture.mutationSnapshot(); !slices.Equal(got, test.wantMutations) {
				t.Fatalf("mutations = %v, want %v", got, test.wantMutations)
			}
		})
	}
}

func TestMemberDirectPoliciesResourceCreateRequiresExactEndpoints(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*memberDirectPoliciesFixture){
		"missing Member": func(fixture *memberDirectPoliciesFixture) {
			fixture.memberPresent = false
		},
		"missing desired Policy": func(fixture *memberDirectPoliciesFixture) {
			delete(fixture.policies, providerDirectPolicyIDOne)
		},
		"unreadable direct collection": func(fixture *memberDirectPoliciesFixture) {
			fixture.failDirectReadAt = 1
		},
	}
	for name, arrange := range tests {
		name := name
		arrange := arrange
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newMemberDirectPoliciesFixture(t)
			arrange(fixture)
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			directSchema := memberDirectPoliciesResourceTestSchema(t)
			response := frameworkresource.CreateResponse{
				State: emptyMemberDirectPoliciesResourceState(t, directSchema),
			}
			newMemberDirectPoliciesTestResource(apiClient).Create(
				context.Background(),
				frameworkresource.CreateRequest{Plan: memberDirectPoliciesResourceTestPlan(
					t,
					directSchema,
					providerMemberID,
					[]string{providerDirectPolicyIDOne},
				)},
				&response,
			)
			state := memberDirectPoliciesStateModel(t, response.State)
			if !response.Diagnostics.HasError() || knownString(state.ID) ||
				len(fixture.mutationSnapshot()) != 0 {
				t.Fatalf(
					"Create() diagnostics/state/mutations = %v/%#v/%v",
					response.Diagnostics,
					state,
					fixture.mutationSnapshot(),
				)
			}
			formatted := fmt.Sprint(response.Diagnostics)
			for _, unsafe := range []string{
				providerMemberID,
				providerDirectPolicyIDOne,
				fixture.member.Email,
				fixture.member.Name,
				"/api/v1/members",
				"/api/v1/policies",
			} {
				if strings.Contains(formatted, unsafe) {
					t.Fatalf("Create() diagnostic exposed runtime identity %q", unsafe)
				}
			}
		})
	}
}

func TestMemberDirectPoliciesResourceReadRefreshesImportTracksDriftAndMemberAbsence(t *testing.T) {
	t.Parallel()

	fixture := newMemberDirectPoliciesFixture(t)
	fixture.directPolicyIDs = []string{
		providerDirectPolicyIDThree,
		providerDirectPolicyIDTwo,
	}
	apiClient, closeServer := newProjectResourceTestClient(t, fixture)
	defer closeServer()
	directSchema := memberDirectPoliciesResourceTestSchema(t)
	resourceUnderTest := newMemberDirectPoliciesTestResource(apiClient)
	imported := frameworkresource.ImportStateResponse{
		State: emptyMemberDirectPoliciesResourceState(t, directSchema),
	}
	resourceUnderTest.ImportState(
		context.Background(),
		frameworkresource.ImportStateRequest{ID: strings.ToUpper(providerMemberID)},
		&imported,
	)
	if imported.Diagnostics.HasError() {
		t.Fatalf("ImportState() diagnostics = %v", imported.Diagnostics)
	}

	var driftResponse frameworkresource.ReadResponse
	resourceUnderTest.Read(
		context.Background(),
		frameworkresource.ReadRequest{State: imported.State},
		&driftResponse,
	)
	if driftResponse.Diagnostics.HasError() || driftResponse.State.Raw.IsNull() {
		t.Fatalf(
			"Read() drift diagnostics/state = %v/%s",
			driftResponse.Diagnostics,
			driftResponse.State.Raw,
		)
	}
	assertMemberDirectPoliciesState(
		t,
		driftResponse.State,
		providerMemberID,
		[]string{providerDirectPolicyIDTwo, providerDirectPolicyIDThree},
	)
	if len(fixture.mutationSnapshot()) != 0 {
		t.Fatalf("Read() sent mutations: %v", fixture.mutationSnapshot())
	}

	fixture.setDirectPolicyIDs(nil)
	var emptyResponse frameworkresource.ReadResponse
	resourceUnderTest.Read(
		context.Background(),
		frameworkresource.ReadRequest{State: driftResponse.State},
		&emptyResponse,
	)
	if emptyResponse.Diagnostics.HasError() || emptyResponse.State.Raw.IsNull() {
		t.Fatalf(
			"Read() empty diagnostics/state = %v/%s",
			emptyResponse.Diagnostics,
			emptyResponse.State.Raw,
		)
	}
	assertMemberDirectPoliciesState(t, emptyResponse.State, providerMemberID, nil)

	fixture.setMemberPresent(false)
	var absentResponse frameworkresource.ReadResponse
	resourceUnderTest.Read(
		context.Background(),
		frameworkresource.ReadRequest{State: emptyResponse.State},
		&absentResponse,
	)
	if absentResponse.Diagnostics.HasError() || !absentResponse.State.Raw.IsNull() {
		t.Fatalf(
			"Read() absent diagnostics/state = %v/%s",
			absentResponse.Diagnostics,
			absentResponse.State.Raw,
		)
	}
}

func TestMemberDirectPoliciesResourceReadPreservesStateOnAmbiguousCollection(t *testing.T) {
	t.Parallel()

	tests := map[string]int{
		"direct 404 is not authoritative absence": http.StatusNotFound,
		"server failure is ambiguous":             http.StatusInternalServerError,
	}
	for name, status := range tests {
		name := name
		status := status
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newMemberDirectPoliciesFixture(t)
			fixture.directPolicyIDs = []string{providerDirectPolicyIDTwo}
			fixture.failDirectReadAt = 1
			fixture.failDirectReadStatus = status
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			directSchema := memberDirectPoliciesResourceTestSchema(t)
			state := memberDirectPoliciesResourceTestState(
				t,
				directSchema,
				providerMemberID,
				[]string{providerDirectPolicyIDOne},
			)
			var response frameworkresource.ReadResponse
			newMemberDirectPoliciesTestResource(apiClient).Read(
				context.Background(),
				frameworkresource.ReadRequest{State: state},
				&response,
			)
			if !response.Diagnostics.HasError() || response.State.Raw.IsNull() {
				t.Fatalf(
					"Read() diagnostics/state = %v/%s",
					response.Diagnostics,
					response.State.Raw,
				)
			}
			assertMemberDirectPoliciesState(
				t,
				response.State,
				providerMemberID,
				[]string{providerDirectPolicyIDOne},
			)
		})
	}
}

func TestMemberDirectPoliciesResourceUpdateReconcilesRemoteDrift(t *testing.T) {
	t.Parallel()

	fixture := newMemberDirectPoliciesFixture(t)
	fixture.directPolicyIDs = []string{providerDirectPolicyIDThree}
	apiClient, closeServer := newProjectResourceTestClient(t, fixture)
	defer closeServer()
	directSchema := memberDirectPoliciesResourceTestSchema(t)
	state := memberDirectPoliciesResourceTestState(
		t,
		directSchema,
		providerMemberID,
		[]string{providerDirectPolicyIDOne},
	)
	plan := memberDirectPoliciesResourceTestPlan(
		t,
		directSchema,
		providerMemberID,
		[]string{providerDirectPolicyIDTwo, providerDirectPolicyIDOne},
	)
	var response frameworkresource.UpdateResponse
	newMemberDirectPoliciesTestResource(apiClient).Update(
		context.Background(),
		frameworkresource.UpdateRequest{State: state, Plan: plan},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Update() diagnostics = %v", response.Diagnostics)
	}
	want := []string{providerDirectPolicyIDOne, providerDirectPolicyIDTwo}
	assertMemberDirectPoliciesState(t, response.State, providerMemberID, want)
	assertCanonicalPolicyIDs(t, fixture.directPolicySnapshot(), want)
	wantMutations := []string{
		"add:" + providerDirectPolicyIDOne,
		"add:" + providerDirectPolicyIDTwo,
		"remove:" + providerDirectPolicyIDThree,
	}
	if got := fixture.mutationSnapshot(); !slices.Equal(got, wantMutations) {
		t.Fatalf("mutations = %v, want %v", got, wantMutations)
	}
}

func TestMemberDirectPoliciesResourceUpdateRejectsIdentityChange(t *testing.T) {
	t.Parallel()

	fixture := newMemberDirectPoliciesFixture(t)
	apiClient, closeServer := newProjectResourceTestClient(t, fixture)
	defer closeServer()
	directSchema := memberDirectPoliciesResourceTestSchema(t)
	state := memberDirectPoliciesResourceTestState(
		t,
		directSchema,
		providerMemberID,
		[]string{providerDirectPolicyIDOne},
	)
	plan := memberDirectPoliciesResourceTestPlan(
		t,
		directSchema,
		providerMemberIDTwo,
		[]string{providerDirectPolicyIDTwo},
	)
	var response frameworkresource.UpdateResponse
	newMemberDirectPoliciesTestResource(apiClient).Update(
		context.Background(),
		frameworkresource.UpdateRequest{State: state, Plan: plan},
		&response,
	)
	if !response.Diagnostics.HasError() || len(fixture.mutationSnapshot()) != 0 {
		t.Fatalf(
			"Update() diagnostics/mutations = %v/%v",
			response.Diagnostics,
			fixture.mutationSnapshot(),
		)
	}
	assertMemberDirectPoliciesState(
		t,
		response.State,
		providerMemberID,
		[]string{providerDirectPolicyIDOne},
	)
}

func TestMemberDirectPoliciesResourceUpdateHonorsCanonicalWriteLockCancellation(t *testing.T) {
	t.Parallel()

	fixture := newMemberDirectPoliciesFixture(t)
	apiClient, closeServer := newProjectResourceTestClient(t, fixture)
	defer closeServer()
	directSchema := memberDirectPoliciesResourceTestSchema(t)
	state := memberDirectPoliciesResourceTestState(
		t,
		directSchema,
		providerMemberID,
		[]string{providerDirectPolicyIDOne},
	)
	plan := memberDirectPoliciesResourceTestPlan(
		t,
		directSchema,
		providerMemberID,
		[]string{providerDirectPolicyIDTwo},
	)
	resourceUnderTest := newMemberDirectPoliciesTestResource(apiClient)
	manager := resourceUnderTest.memberDirectPolicyLocks()
	release, err := manager.acquire(
		context.Background(),
		memberDirectPoliciesWriteLockKey(strings.ToUpper(providerMemberID)),
	)
	if err != nil {
		t.Fatalf("occupy Member direct-Policies write lock: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	response := frameworkresource.UpdateResponse{State: state}
	resourceUnderTest.Update(
		ctx,
		frameworkresource.UpdateRequest{State: state, Plan: plan},
		&response,
	)
	release()

	if !response.Diagnostics.HasError() || !response.State.Raw.Equal(state.Raw) ||
		fixture.requestCount() != 0 {
		t.Fatalf(
			"canceled Update diagnostics/state/requests = %v/%t/%d",
			response.Diagnostics,
			response.State.Raw.Equal(state.Raw),
			fixture.requestCount(),
		)
	}
	manager.mu.Lock()
	remaining := len(manager.locks)
	manager.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("Member direct-Policies lock manager retained %d entries", remaining)
	}
}

func TestMemberDirectPoliciesResourceDeleteClearsOnlyDirectPolicies(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		memberPresent bool
		failBefore    string
		wantError     bool
		wantState     []string
		wantRemote    []string
		wantMutations []string
	}{
		"success": {
			memberPresent: true,
			wantMutations: []string{
				"remove:" + providerDirectPolicyIDOne,
				"remove:" + providerDirectPolicyIDThree,
			},
		},
		"partial failure": {
			memberPresent: true,
			failBefore:    "remove:" + providerDirectPolicyIDThree,
			wantError:     true,
			wantState:     []string{providerDirectPolicyIDThree},
			wantRemote:    []string{providerDirectPolicyIDThree},
			wantMutations: []string{
				"remove:" + providerDirectPolicyIDOne,
				"remove:" + providerDirectPolicyIDThree,
			},
		},
		"confirmed missing Member": {
			memberPresent: false,
			wantRemote: []string{
				providerDirectPolicyIDOne,
				providerDirectPolicyIDThree,
			},
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newMemberDirectPoliciesFixture(t)
			fixture.memberPresent = test.memberPresent
			fixture.directPolicyIDs = []string{
				providerDirectPolicyIDThree,
				providerDirectPolicyIDOne,
			}
			fixture.failBeforeMutation = test.failBefore
			inheritedBaseline := fixture.inheritedPolicySnapshot()
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			directSchema := memberDirectPoliciesResourceTestSchema(t)
			state := memberDirectPoliciesResourceTestState(
				t,
				directSchema,
				providerMemberID,
				[]string{providerDirectPolicyIDOne},
			)
			var response frameworkresource.DeleteResponse
			newMemberDirectPoliciesTestResource(apiClient).Delete(
				context.Background(),
				frameworkresource.DeleteRequest{State: state},
				&response,
			)
			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf(
					"Delete() error = %t, want %t: %v",
					got,
					test.wantError,
					response.Diagnostics,
				)
			}
			if test.wantError {
				if response.State.Raw.IsNull() {
					t.Fatal("Delete() discarded state after partial failure")
				}
				assertMemberDirectPoliciesState(
					t,
					response.State,
					providerMemberID,
					test.wantState,
				)
			} else if !response.State.Raw.IsNull() {
				t.Fatalf("Delete() preserved state = %s", response.State.Raw)
			}
			assertCanonicalPolicyIDs(t, fixture.directPolicySnapshot(), test.wantRemote)
			if got := fixture.mutationSnapshot(); !slices.Equal(got, test.wantMutations) {
				t.Fatalf("mutations = %v, want %v", got, test.wantMutations)
			}
			if !slices.Equal(fixture.inheritedPolicySnapshot(), inheritedBaseline) {
				t.Fatal("Delete() changed inherited Policies")
			}
		})
	}
}

type memberDirectPoliciesFixture struct {
	t  *testing.T
	mu sync.Mutex

	member               client.Member
	memberPresent        bool
	policies             map[string]client.Policy
	directPolicyIDs      []string
	inheritedPolicyIDs   []string
	mutationNames        []string
	requests             int
	directReads          int
	policyReads          int
	failBeforeMutation   string
	failAfterMutation    string
	skipMutationApply    string
	failDirectReadAt     int
	failDirectReadStatus int
}

func newMemberDirectPoliciesFixture(t *testing.T) *memberDirectPoliciesFixture {
	t.Helper()
	policies := make(map[string]client.Policy)
	for index, policyID := range []string{
		providerDirectPolicyIDOne,
		providerDirectPolicyIDTwo,
		providerDirectPolicyIDThree,
	} {
		policies[policyID] = client.Policy{
			ID:          policyID,
			Name:        fmt.Sprintf("Policy %d", index+1),
			Key:         fmt.Sprintf("policy-%d", index+1),
			Type:        client.PolicyTypeCustomerManaged,
			Description: "",
			Statements:  []client.PolicyStatement{},
		}
	}
	policy := policies[providerDirectPolicyIDTwo]
	policy.Type = client.PolicyTypeSysManaged
	policies[providerDirectPolicyIDTwo] = policy
	return &memberDirectPoliciesFixture{
		t:             t,
		memberPresent: true,
		member: client.Member{
			ID: providerMemberID, Email: "member@example.invalid", Name: "Managed Member",
		},
		policies: policies,
		inheritedPolicyIDs: []string{
			providerDirectPolicyIDThree,
		},
	}
}

func (f *memberDirectPoliciesFixture) ServeHTTP(
	response http.ResponseWriter,
	request *http.Request,
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++

	memberBasePath := "/api/v1/members"
	memberPath := memberBasePath + "/" + providerMemberID
	directPath := memberPath + "/direct-policies"
	policyBasePath := "/api/v1/policies"

	switch {
	case request.Method == http.MethodGet && request.URL.EscapedPath() == memberBasePath:
		members := []map[string]any{}
		if f.memberPresent {
			members = append(members, map[string]any{
				"id":              f.member.ID,
				"email":           f.member.Email,
				"name":            f.member.Name,
				"initialPassword": "must-not-be-decoded",
			})
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(members),
			"items":      members,
		})
	case request.Method == http.MethodGet && request.URL.EscapedPath() == memberPath:
		if !f.memberPresent {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"id":              f.member.ID,
			"email":           f.member.Email,
			"name":            f.member.Name,
			"initialPassword": "must-not-be-decoded",
		})
	case request.Method == http.MethodGet && request.URL.EscapedPath() == policyBasePath:
		f.policyReads++
		policies := make([]client.Policy, 0, len(f.policies))
		for _, policy := range f.policies {
			policies = append(policies, policy)
		}
		sort.Slice(policies, func(left int, right int) bool {
			return policies[left].ID < policies[right].ID
		})
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(policies),
			"items":      policies,
		})
	case request.Method == http.MethodGet && strings.HasPrefix(
		request.URL.EscapedPath(),
		policyBasePath+"/",
	):
		policyID := strings.TrimPrefix(request.URL.EscapedPath(), policyBasePath+"/")
		policy, exists := f.policies[policyID]
		if !exists {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, policy)
	case request.Method == http.MethodGet && request.URL.EscapedPath() == directPath:
		f.directReads++
		if f.failDirectReadAt != 0 && f.directReads == f.failDirectReadAt {
			status := f.failDirectReadStatus
			if status == 0 {
				status = http.StatusInternalServerError
			}
			writePolicyProviderEnvelope(f.t, response, status, nil)
			return
		}
		if !f.memberPresent {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		items := make([]map[string]any, 0, len(f.directPolicyIDs))
		for _, policyID := range f.directPolicyIDs {
			items = append(items, map[string]any{
				"id": policyID, "isMemberPolicy": true,
				"name": "must-not-be-decoded",
			})
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(items),
			"items":      items,
		})
	case request.Method == http.MethodPut &&
		strings.HasPrefix(request.URL.EscapedPath(), memberPath+"/add-policy/"):
		policyID := strings.TrimPrefix(request.URL.EscapedPath(), memberPath+"/add-policy/")
		f.handleMutation(response, request, "add", policyID)
	case request.Method == http.MethodPut &&
		strings.HasPrefix(request.URL.EscapedPath(), memberPath+"/remove-policy/"):
		policyID := strings.TrimPrefix(request.URL.EscapedPath(), memberPath+"/remove-policy/")
		f.handleMutation(response, request, "remove", policyID)
	default:
		f.t.Fatalf(
			"unexpected Member direct-Policy fixture request %s %s?%s",
			request.Method,
			request.URL.EscapedPath(),
			request.URL.RawQuery,
		)
	}
}

func (f *memberDirectPoliciesFixture) handleMutation(
	response http.ResponseWriter,
	request *http.Request,
	operation string,
	policyID string,
) {
	if request.URL.RawQuery != "" {
		f.t.Fatalf("direct-Policy mutation contained query %q", request.URL.RawQuery)
	}
	if request.Body != nil && request.Body != http.NoBody {
		body, err := io.ReadAll(request.Body)
		if err != nil || len(body) != 0 {
			f.t.Fatalf("direct-Policy mutation body = %q/%v", body, err)
		}
	}
	mutationName := operation + ":" + policyID
	f.mutationNames = append(f.mutationNames, mutationName)
	if mutationName == f.failBeforeMutation {
		writePolicyProviderEnvelope(f.t, response, http.StatusInternalServerError, nil)
		return
	}
	if mutationName != f.skipMutationApply && f.memberPresent {
		if _, policyExists := f.policies[policyID]; policyExists {
			switch operation {
			case "add":
				if !slices.Contains(f.directPolicyIDs, policyID) {
					f.directPolicyIDs = append(f.directPolicyIDs, policyID)
				}
			case "remove":
				f.removeDirectPolicyLocked(policyID)
			}
		}
	}
	if mutationName == f.failAfterMutation {
		writePolicyProviderEnvelope(f.t, response, http.StatusInternalServerError, nil)
		return
	}
	writePolicyProviderEnvelope(f.t, response, http.StatusOK, true)
}

func (f *memberDirectPoliciesFixture) removeDirectPolicyLocked(policyID string) {
	for index, candidate := range f.directPolicyIDs {
		if client.EqualUUID(candidate, policyID) {
			f.directPolicyIDs = append(
				f.directPolicyIDs[:index],
				f.directPolicyIDs[index+1:]...,
			)
			return
		}
	}
}

func (f *memberDirectPoliciesFixture) setDirectPolicyIDs(policyIDs []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.directPolicyIDs = append([]string(nil), policyIDs...)
}

func (f *memberDirectPoliciesFixture) setMemberPresent(present bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.memberPresent = present
}

func (f *memberDirectPoliciesFixture) directPolicySnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.directPolicyIDs...)
}

func (f *memberDirectPoliciesFixture) inheritedPolicySnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.inheritedPolicyIDs...)
}

func (f *memberDirectPoliciesFixture) mutationSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.mutationNames...)
}

func (f *memberDirectPoliciesFixture) directReadCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.directReads
}

func (f *memberDirectPoliciesFixture) policyReadCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.policyReads
}

func (f *memberDirectPoliciesFixture) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}

func newMemberDirectPoliciesTestResource(
	apiClient *client.Client,
) *memberDirectPoliciesResource {
	return &memberDirectPoliciesResource{client: apiClient}
}

func memberDirectPoliciesResourceTestSchema(t *testing.T) resourceschema.Schema {
	t.Helper()
	var response frameworkresource.SchemaResponse
	newMemberDirectPoliciesTestResource(nil).Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Member direct-Policies schema diagnostics = %v", response.Diagnostics)
	}
	return response.Schema
}

func memberDirectPoliciesResourceTestPlan(
	t *testing.T,
	directSchema resourceschema.Schema,
	memberID string,
	policyIDs []string,
) tfsdk.Plan {
	t.Helper()
	plan := tfsdk.Plan{Schema: directSchema}
	model := memberDirectPoliciesModel{
		ID:        types.StringUnknown(),
		MemberID:  types.StringValue(memberID),
		PolicyIDs: terraformStringSetValue(policyIDs),
	}
	if diagnostics := plan.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize Member direct-Policies plan: %v", diagnostics)
	}
	return plan
}

func memberDirectPoliciesResourceTestState(
	t *testing.T,
	directSchema resourceschema.Schema,
	memberID string,
	policyIDs []string,
) tfsdk.State {
	t.Helper()
	canonicalMemberID, valid := client.CanonicalUUID(memberID)
	if !valid {
		t.Fatalf("invalid Member test UUID %q", memberID)
	}
	canonicalPolicyIDs, err := canonicalizeMemberPolicyUUIDs(policyIDs)
	if err != nil {
		t.Fatalf("canonicalize direct Policy test UUIDs: %v", err)
	}
	state := tfsdk.State{Schema: directSchema}
	model := memberDirectPoliciesModel{
		ID:        types.StringValue(canonicalMemberID),
		MemberID:  types.StringValue(canonicalMemberID),
		PolicyIDs: terraformStringSetValue(canonicalPolicyIDs),
	}
	if diagnostics := state.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize Member direct-Policies state: %v", diagnostics)
	}
	return state
}

func emptyMemberDirectPoliciesResourceState(
	t *testing.T,
	directSchema resourceschema.Schema,
) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: directSchema}
	model := memberDirectPoliciesModel{
		ID:        types.StringNull(),
		MemberID:  types.StringNull(),
		PolicyIDs: types.SetNull(types.StringType),
	}
	if diagnostics := state.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize empty Member direct-Policies state: %v", diagnostics)
	}
	return state
}

func memberDirectPoliciesStateModel(
	t *testing.T,
	state tfsdk.State,
) memberDirectPoliciesModel {
	t.Helper()
	var model memberDirectPoliciesModel
	if diagnostics := state.Get(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("read Member direct-Policies state: %v", diagnostics)
	}
	return model
}

func assertMemberDirectPoliciesState(
	t *testing.T,
	state tfsdk.State,
	memberID string,
	policyIDs []string,
) {
	t.Helper()
	model := memberDirectPoliciesStateModel(t, state)
	if model.ID.ValueString() != memberID || model.MemberID.ValueString() != memberID {
		t.Fatalf("Member direct-Policies identity state = %#v", model)
	}
	got, err := memberDirectPolicyIDsFromTerraform(context.Background(), model.PolicyIDs)
	if err != nil {
		t.Fatalf("read canonical direct Policy state: %v", err)
	}
	want, err := canonicalizeMemberPolicyUUIDs(policyIDs)
	if err != nil {
		t.Fatalf("canonicalize expected direct Policy state: %v", err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Member direct Policy IDs = %v, want %v", got, want)
	}
}

func assertCanonicalPolicyIDs(t *testing.T, got []string, want []string) {
	t.Helper()
	canonicalGot, err := canonicalizeMemberPolicyUUIDs(got)
	if err != nil {
		t.Fatalf("canonicalize actual direct Policy IDs: %v", err)
	}
	canonicalWant, err := canonicalizeMemberPolicyUUIDs(want)
	if err != nil {
		t.Fatalf("canonicalize expected direct Policy IDs: %v", err)
	}
	if !slices.Equal(canonicalGot, canonicalWant) {
		t.Fatalf("direct Policy IDs = %v, want %v", canonicalGot, canonicalWant)
	}
}
