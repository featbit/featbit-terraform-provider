// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	providerGroupID       = "66666666-6666-4666-8666-666666666666"
	providerGroupIDTwo    = "77777777-7777-4777-8777-777777777777"
	providerGroupMemberID = "88888888-8888-4888-8888-888888888888"
	providerGroupPolicyID = "99999999-9999-4999-8999-999999999999"
)

func TestGroupResourceMetadataConfigureSchemaAndImport(t *testing.T) {
	t.Parallel()

	resourceUnderTest := &groupResource{}
	var metadata frameworkresource.MetadataResponse
	resourceUnderTest.Metadata(
		context.Background(),
		frameworkresource.MetadataRequest{ProviderTypeName: "featbit"},
		&metadata,
	)
	if metadata.TypeName != "featbit_group" {
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

	groupSchema := groupResourceTestSchema(t)
	if len(groupSchema.Attributes) != 3 {
		t.Fatalf("Group schema attributes = %v", groupSchema.Attributes)
	}
	id, ok := groupSchema.Attributes["id"].(resourceschema.StringAttribute)
	if !ok || !id.Computed || id.Required || id.Optional || len(id.PlanModifiers) != 1 {
		t.Fatalf("id schema = %#v", id)
	}
	name, ok := groupSchema.Attributes["name"].(resourceschema.StringAttribute)
	if !ok || !name.Required || name.Optional || name.Computed || len(name.Validators) != 1 {
		t.Fatalf("name schema = %#v", name)
	}
	description, ok := groupSchema.Attributes["description"].(resourceschema.StringAttribute)
	if !ok || !description.Optional || !description.Computed || description.Required {
		t.Fatalf("description schema = %#v", description)
	}
	for _, forbidden := range []string{"member_ids", "policy_ids", "members", "policies"} {
		if _, exists := groupSchema.Attributes[forbidden]; exists {
			t.Fatalf("Group schema unexpectedly owns %q", forbidden)
		}
	}

	tests := map[string]struct {
		id        string
		wantError bool
	}{
		"canonical":               {id: providerGroupID},
		"uppercase canonicalized": {id: strings.ToUpper(providerGroupID)},
		"missing":                 {id: "", wantError: true},
		"invalid":                 {id: "not-a-uuid", wantError: true},
		"composite":               {id: providerGroupID + "/extra", wantError: true},
	}
	for testName, test := range tests {
		testName := testName
		test := test
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			response := frameworkresource.ImportStateResponse{
				State: emptyGroupResourceState(t, groupSchema),
			}
			resourceUnderTest.ImportState(
				context.Background(),
				frameworkresource.ImportStateRequest{ID: test.id},
				&response,
			)
			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf("ImportState() error = %t, want %t: %v", got, test.wantError, response.Diagnostics)
			}
			if !test.wantError {
				state := groupStateModel(t, response.State)
				if state.ID.ValueString() != providerGroupID || !state.Name.IsNull() ||
					!state.Description.IsNull() {
					t.Fatalf("ImportState() state = %#v", state)
				}
			} else if strings.Contains(fmt.Sprint(response.Diagnostics), test.id) && test.id != "" {
				t.Fatal("ImportState() diagnostic echoed the rejected identifier")
			}
		})
	}
}

func TestGroupResourceCreateLifecycle(t *testing.T) {
	t.Parallel()

	fixture := newGroupResourceFixture(t)
	apiClient, closeServer := newProjectResourceTestClient(t, fixture)
	defer closeServer()
	groupSchema := groupResourceTestSchema(t)
	response := frameworkresource.CreateResponse{State: emptyGroupResourceState(t, groupSchema)}
	(&groupResource{client: apiClient}).Create(
		context.Background(),
		frameworkresource.CreateRequest{
			Plan: groupResourceTestPlan(t, groupSchema, "Managed", "Description"),
		},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Create() diagnostics = %v", response.Diagnostics)
	}
	state := groupStateModel(t, response.State)
	if state.ID.ValueString() != providerGroupID || state.Name.ValueString() != "Managed" ||
		state.Description.ValueString() != "Description" {
		t.Fatalf("Create() state = %#v", state)
	}
	if got := fixture.mutations(); fmt.Sprint(got) != fmt.Sprint([]string{"create"}) {
		t.Fatalf("mutations = %v", got)
	}
	remote, found := fixture.currentGroup(providerGroupID)
	if !found || remote.Name != "Managed" || remote.Description != "Description" {
		t.Fatalf("remote Group = %#v/%t", remote, found)
	}
}

func TestGroupResourceCreatePreflightAndAmbiguousOutcomeNeverAdopt(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		groups               []client.Group
		failCreateAfterApply bool
		wantMutation         bool
	}{
		"exact duplicate": {
			groups: []client.Group{{ID: providerGroupID, Name: "Managed"}},
		},
		"duplicate exact names": {
			groups: []client.Group{
				{ID: providerGroupID, Name: "Managed"},
				{ID: providerGroupIDTwo, Name: "Managed"},
			},
		},
		"ambiguous create applied": {
			failCreateAfterApply: true,
			wantMutation:         true,
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newGroupResourceFixture(t, test.groups...)
			fixture.failCreateAfterApply = test.failCreateAfterApply
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			groupSchema := groupResourceTestSchema(t)
			response := frameworkresource.CreateResponse{State: emptyGroupResourceState(t, groupSchema)}
			(&groupResource{client: apiClient}).Create(
				context.Background(),
				frameworkresource.CreateRequest{
					Plan: groupResourceTestPlan(t, groupSchema, "Managed", "Description"),
				},
				&response,
			)
			if !response.Diagnostics.HasError() {
				t.Fatal("Create() unexpectedly succeeded")
			}
			state := groupStateModel(t, response.State)
			if !state.ID.IsNull() {
				t.Fatalf("Create() adopted an unconfirmed Group: %#v", state)
			}
			mutations := fixture.mutations()
			if (len(mutations) != 0) != test.wantMutation ||
				(test.wantMutation && fmt.Sprint(mutations) != fmt.Sprint([]string{"create"})) {
				t.Fatalf("mutations = %v, want mutation %t", mutations, test.wantMutation)
			}
			for _, unsafe := range []string{"Managed", "Description", providerGroupID} {
				if strings.Contains(fmt.Sprint(response.Diagnostics), unsafe) {
					t.Fatalf("diagnostic exposed Group value %q: %v", unsafe, response.Diagnostics)
				}
			}
		})
	}
}

func TestGroupResourceReadRefreshesDriftAndRemovesAuthoritativeAbsence(t *testing.T) {
	t.Parallel()

	fixture := newGroupResourceFixture(t, client.Group{
		ID: providerGroupID, Name: "Remote", Description: "Remote description",
	})
	apiClient, closeServer := newProjectResourceTestClient(t, fixture)
	defer closeServer()
	groupSchema := groupResourceTestSchema(t)
	initial := groupResourceTestState(t, groupSchema, "Prior", "Prior description")
	var driftResponse frameworkresource.ReadResponse
	(&groupResource{client: apiClient}).Read(
		context.Background(),
		frameworkresource.ReadRequest{State: initial},
		&driftResponse,
	)
	if driftResponse.Diagnostics.HasError() {
		t.Fatalf("Read() drift diagnostics = %v", driftResponse.Diagnostics)
	}
	drifted := groupStateModel(t, driftResponse.State)
	if drifted.Name.ValueString() != "Remote" ||
		drifted.Description.ValueString() != "Remote description" {
		t.Fatalf("Read() drift state = %#v", drifted)
	}

	fixture.removeGroup(providerGroupID)
	var absentResponse frameworkresource.ReadResponse
	(&groupResource{client: apiClient}).Read(
		context.Background(),
		frameworkresource.ReadRequest{State: driftResponse.State},
		&absentResponse,
	)
	if absentResponse.Diagnostics.HasError() || !absentResponse.State.Raw.IsNull() {
		t.Fatalf("Read() absence diagnostics/state = %v/%s", absentResponse.Diagnostics, absentResponse.State.Raw)
	}
}

func TestGroupResourceUpdateReconcilesSingleMutation(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		failBeforeApply bool
		failAfterApply  bool
		wantError       bool
		wantName        string
	}{
		"success": {
			wantName: "Planned",
		},
		"ambiguous response after apply": {
			failAfterApply: true,
			wantName:       "Planned",
		},
		"ambiguous response before apply": {
			failBeforeApply: true,
			wantError:       true,
			wantName:        "Prior",
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newGroupResourceFixture(t, client.Group{
				ID: providerGroupID, Name: "Prior", Description: "Prior description",
			})
			fixture.failUpdateBeforeApply = test.failBeforeApply
			fixture.failUpdateAfterApply = test.failAfterApply
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			groupSchema := groupResourceTestSchema(t)
			state := groupResourceTestState(t, groupSchema, "Prior", "Prior description")
			plan := groupResourceTestPlan(t, groupSchema, "Planned", "Planned description")
			var response frameworkresource.UpdateResponse
			(&groupResource{client: apiClient}).Update(
				context.Background(),
				frameworkresource.UpdateRequest{State: state, Plan: plan},
				&response,
			)
			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf("Update() error = %t, want %t: %v", got, test.wantError, response.Diagnostics)
			}
			current := groupStateModel(t, response.State)
			if current.Name.ValueString() != test.wantName {
				t.Fatalf("Update() state = %#v", current)
			}
			if got := fixture.mutations(); fmt.Sprint(got) != fmt.Sprint([]string{"update"}) {
				t.Fatalf("mutations = %v", got)
			}
		})
	}
}

func TestGroupResourceDeleteRefusesAssociations(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		memberIDs []string
		policyIDs []string
	}{
		"member": {memberIDs: []string{providerGroupMemberID}},
		"policy": {policyIDs: []string{providerGroupPolicyID}},
		"both": {
			memberIDs: []string{providerGroupMemberID},
			policyIDs: []string{providerGroupPolicyID},
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newGroupResourceFixture(t, client.Group{
				ID: providerGroupID, Name: "Managed", Description: "Description",
			})
			fixture.memberIDs = append([]string(nil), test.memberIDs...)
			fixture.policyIDs = append([]string(nil), test.policyIDs...)
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			groupSchema := groupResourceTestSchema(t)
			state := groupResourceTestState(t, groupSchema, "Managed", "Description")
			var response frameworkresource.DeleteResponse
			(&groupResource{client: apiClient}).Delete(
				context.Background(),
				frameworkresource.DeleteRequest{State: state},
				&response,
			)
			if !response.Diagnostics.HasError() || response.State.Raw.IsNull() ||
				len(fixture.mutations()) != 0 {
				t.Fatalf("Delete() diagnostics/state/mutations = %v/%s/%v", response.Diagnostics, response.State.Raw, fixture.mutations())
			}
		})
	}
}

func TestGroupResourceDeleteRequiresExactAbsence(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		startPresent          bool
		failDeleteBeforeApply bool
		failDeleteAfterApply  bool
		wantError             bool
		wantMutation          bool
		wantAbsent            bool
	}{
		"success": {
			startPresent: true,
			wantMutation: true,
			wantAbsent:   true,
		},
		"already absent": {
			wantAbsent: true,
		},
		"ambiguous response after apply": {
			startPresent:         true,
			failDeleteAfterApply: true,
			wantMutation:         true,
			wantAbsent:           true,
		},
		"ambiguous response before apply": {
			startPresent:          true,
			failDeleteBeforeApply: true,
			wantMutation:          true,
			wantError:             true,
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newGroupResourceFixture(t)
			if test.startPresent {
				fixture.groups = append(fixture.groups, client.Group{
					ID: providerGroupID, Name: "Managed", Description: "Description",
				})
			}
			fixture.failDeleteBeforeApply = test.failDeleteBeforeApply
			fixture.failDeleteAfterApply = test.failDeleteAfterApply
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			groupSchema := groupResourceTestSchema(t)
			state := groupResourceTestState(t, groupSchema, "Managed", "Description")
			var response frameworkresource.DeleteResponse
			(&groupResource{client: apiClient}).Delete(
				context.Background(),
				frameworkresource.DeleteRequest{State: state},
				&response,
			)
			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf("Delete() error = %t, want %t: %v", got, test.wantError, response.Diagnostics)
			}
			if got := response.State.Raw.IsNull(); got != test.wantAbsent {
				t.Fatalf("Delete() absent = %t, want %t; state = %s", got, test.wantAbsent, response.State.Raw)
			}
			mutations := fixture.mutations()
			if (len(mutations) != 0) != test.wantMutation ||
				(test.wantMutation && fmt.Sprint(mutations) != fmt.Sprint([]string{"delete"})) {
				t.Fatalf("mutations = %v, want mutation %t", mutations, test.wantMutation)
			}
		})
	}
}

type groupResourceFixture struct {
	t *testing.T

	mu                    sync.Mutex
	groups                []client.Group
	mutationNames         []string
	failCreateAfterApply  bool
	failUpdateBeforeApply bool
	failUpdateAfterApply  bool
	failDeleteBeforeApply bool
	failDeleteAfterApply  bool
	memberIDs             []string
	policyIDs             []string
}

func newGroupResourceFixture(t *testing.T, groups ...client.Group) *groupResourceFixture {
	t.Helper()
	return &groupResourceFixture{
		t:      t,
		groups: append([]client.Group(nil), groups...),
	}
}

func (f *groupResourceFixture) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	basePath := "/api/v1/groups"
	switch {
	case request.Method == http.MethodGet && request.URL.EscapedPath() == basePath:
		items := make([]client.Group, len(f.groups))
		copy(items, f.groups)
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(f.groups),
			"items":      items,
		})
	case request.Method == http.MethodPost && request.URL.EscapedPath() == basePath:
		f.mutationNames = append(f.mutationNames, "create")
		var input client.CreateGroupRequest
		f.decodeBody(request.Body, &input)
		created := client.Group{
			ID: providerGroupID, Name: input.Name, Description: input.Description,
		}
		f.groups = append(f.groups, created)
		if f.failCreateAfterApply {
			writePolicyProviderEnvelope(f.t, response, http.StatusInternalServerError, nil)
			return
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, created)
	case request.Method == http.MethodGet && strings.HasSuffix(request.URL.EscapedPath(), "/members"):
		items := make([]map[string]any, 0, len(f.memberIDs))
		for _, id := range f.memberIDs {
			items = append(items, map[string]any{"id": id, "isGroupMember": true})
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(items), "items": items,
		})
	case request.Method == http.MethodGet && strings.HasSuffix(request.URL.EscapedPath(), "/policies"):
		items := make([]map[string]any, 0, len(f.policyIDs))
		for _, id := range f.policyIDs {
			items = append(items, map[string]any{"id": id, "isGroupPolicy": true})
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(items), "items": items,
		})
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.EscapedPath(), basePath+"/"):
		group, found := f.findGroup(strings.TrimPrefix(request.URL.EscapedPath(), basePath+"/"))
		if !found {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, group)
	case request.Method == http.MethodPut && strings.HasPrefix(request.URL.EscapedPath(), basePath+"/"):
		f.mutationNames = append(f.mutationNames, "update")
		var input client.UpdateGroupRequest
		f.decodeBody(request.Body, &input)
		if f.failUpdateBeforeApply {
			writePolicyProviderEnvelope(f.t, response, http.StatusInternalServerError, nil)
			return
		}
		index := f.findGroupIndex(strings.TrimPrefix(request.URL.EscapedPath(), basePath+"/"))
		if index < 0 {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		f.groups[index].Name = input.Name
		f.groups[index].Description = input.Description
		if f.failUpdateAfterApply {
			writePolicyProviderEnvelope(f.t, response, http.StatusInternalServerError, nil)
			return
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, f.groups[index])
	case request.Method == http.MethodDelete && strings.HasPrefix(request.URL.EscapedPath(), basePath+"/"):
		f.mutationNames = append(f.mutationNames, "delete")
		if f.failDeleteBeforeApply {
			writePolicyProviderEnvelope(f.t, response, http.StatusInternalServerError, nil)
			return
		}
		index := f.findGroupIndex(strings.TrimPrefix(request.URL.EscapedPath(), basePath+"/"))
		if index >= 0 {
			f.groups = append(f.groups[:index], f.groups[index+1:]...)
		}
		if f.failDeleteAfterApply {
			writePolicyProviderEnvelope(f.t, response, http.StatusInternalServerError, nil)
			return
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, true)
	default:
		f.t.Fatalf("unexpected Group fixture request %s %s", request.Method, request.URL.EscapedPath())
	}
}

func (f *groupResourceFixture) decodeBody(body io.ReadCloser, target any) {
	f.t.Helper()
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		f.t.Fatalf("decode Group request: %v", err)
	}
}

func (f *groupResourceFixture) findGroup(id string) (client.Group, bool) {
	index := f.findGroupIndex(id)
	if index < 0 {
		return client.Group{}, false
	}
	return f.groups[index], true
}

func (f *groupResourceFixture) findGroupIndex(id string) int {
	for index, group := range f.groups {
		if client.EqualUUID(group.ID, id) {
			return index
		}
	}
	return -1
}

func (f *groupResourceFixture) mutations() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.mutationNames...)
}

func (f *groupResourceFixture) currentGroup(id string) (client.Group, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.findGroup(id)
}

func (f *groupResourceFixture) removeGroup(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	index := f.findGroupIndex(id)
	if index >= 0 {
		f.groups = append(f.groups[:index], f.groups[index+1:]...)
	}
}

func groupResourceTestSchema(t *testing.T) resourceschema.Schema {
	t.Helper()
	var response frameworkresource.SchemaResponse
	(&groupResource{}).Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Group resource schema diagnostics = %v", response.Diagnostics)
	}
	return response.Schema
}

func groupResourceTestPlan(
	t *testing.T,
	groupSchema resourceschema.Schema,
	name string,
	description string,
) tfsdk.Plan {
	t.Helper()
	plan := tfsdk.Plan{Schema: groupSchema}
	model := groupModel{
		ID:          types.StringUnknown(),
		Name:        types.StringValue(name),
		Description: types.StringValue(description),
	}
	if diagnostics := plan.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize Group plan: %v", diagnostics)
	}
	return plan
}

func groupResourceTestState(
	t *testing.T,
	groupSchema resourceschema.Schema,
	name string,
	description string,
) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: groupSchema}
	model := flattenGroup(client.Group{
		ID: providerGroupID, Name: name, Description: description,
	})
	if diagnostics := state.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize Group state: %v", diagnostics)
	}
	return state
}

func emptyGroupResourceState(t *testing.T, groupSchema resourceschema.Schema) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: groupSchema}
	model := groupModel{
		ID:          types.StringNull(),
		Name:        types.StringNull(),
		Description: types.StringNull(),
	}
	if diagnostics := state.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize empty Group state: %v", diagnostics)
	}
	return state
}

func groupStateModel(t *testing.T, state tfsdk.State) groupModel {
	t.Helper()
	var model groupModel
	if diagnostics := state.Get(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("read Group state: %v", diagnostics)
	}
	return model
}
