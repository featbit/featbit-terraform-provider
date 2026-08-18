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
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	providerMemberID    = "88888888-8888-4888-8888-888888888888"
	providerMemberIDTwo = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
)

func TestMemberDataSourceMetadataConfigureSchemaAndSelectors(t *testing.T) {
	t.Parallel()

	dataSourceUnderTest := &memberDataSource{}
	var metadata datasource.MetadataResponse
	dataSourceUnderTest.Metadata(
		context.Background(),
		datasource.MetadataRequest{ProviderTypeName: "featbit"},
		&metadata,
	)
	if metadata.TypeName != "featbit_member" {
		t.Fatalf("metadata type = %q", metadata.TypeName)
	}

	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {
			t.Fatal("Configure() reached transport")
		},
	))
	defer closeServer()
	var configure datasource.ConfigureResponse
	dataSourceUnderTest.Configure(
		context.Background(),
		datasource.ConfigureRequest{ProviderData: apiClient},
		&configure,
	)
	if configure.Diagnostics.HasError() || dataSourceUnderTest.client != apiClient {
		t.Fatalf("Configure() diagnostics/client = %v/%p", configure.Diagnostics, dataSourceUnderTest.client)
	}

	memberSchema := memberDataSourceTestSchema(t)
	if len(memberSchema.Attributes) != 3 {
		t.Fatalf("Member data source attributes = %v", memberSchema.Attributes)
	}
	id, ok := memberSchema.Attributes["id"].(datasourceschema.StringAttribute)
	if !ok || !id.Optional || !id.Computed || id.Required || !id.Sensitive ||
		len(id.Validators) != 1 {
		t.Fatalf("id schema = %#v", id)
	}
	email, ok := memberSchema.Attributes["email"].(datasourceschema.StringAttribute)
	if !ok || !email.Optional || !email.Computed || email.Required || !email.Sensitive ||
		len(email.Validators) != 1 {
		t.Fatalf("email schema = %#v", email)
	}
	name, ok := memberSchema.Attributes["name"].(datasourceschema.StringAttribute)
	if !ok || !name.Computed || name.Optional || name.Required || !name.Sensitive {
		t.Fatalf("name schema = %#v", name)
	}
	for _, forbidden := range []string{
		"initial_password", "initialPassword", "invitor_id", "groups", "policies",
	} {
		if _, exists := memberSchema.Attributes[forbidden]; exists {
			t.Fatalf("Member data source unexpectedly exposes %q", forbidden)
		}
	}

	tests := map[string]struct {
		id        types.String
		email     types.String
		wantError bool
	}{
		"exact UUID": {
			id:    types.StringValue(providerMemberID),
			email: types.StringNull(),
		},
		"exact email": {
			id:    types.StringNull(),
			email: types.StringValue("member@example.test"),
		},
		"unknown reference": {
			id:    types.StringUnknown(),
			email: types.StringNull(),
		},
		"missing": {
			id:        types.StringNull(),
			email:     types.StringNull(),
			wantError: true,
		},
		"both": {
			id:        types.StringValue(providerMemberID),
			email:     types.StringValue("member@example.test"),
			wantError: true,
		},
	}
	for testName, test := range tests {
		testName := testName
		test := test
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			var response datasource.ValidateConfigResponse
			dataSourceUnderTest.ValidateConfig(
				context.Background(),
				datasource.ValidateConfigRequest{Config: memberDataSourceTestConfig(
					t, memberSchema, test.id, test.email,
				)},
				&response,
			)
			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf("ValidateConfig() error = %t, want %t: %v", got, test.wantError, response.Diagnostics)
			}
		})
	}
}

func TestMemberDataSourceReadsExistingMemberByExactSelector(t *testing.T) {
	t.Parallel()

	member := client.Member{
		ID: providerMemberID, Email: "Canonical.Member@example.test", Name: "Canonical Member",
	}
	tests := map[string]struct {
		id    types.String
		email types.String
	}{
		"UUID": {
			id:    types.StringValue(strings.ToUpper(providerMemberID)),
			email: types.StringNull(),
		},
		"case-insensitive full email": {
			id:    types.StringNull(),
			email: types.StringValue(strings.ToLower(member.Email)),
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newMemberDataSourceFixture(t, member)
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			memberSchema := memberDataSourceTestSchema(t)
			response := datasource.ReadResponse{State: tfsdk.State{Schema: memberSchema}}
			(&memberDataSource{client: apiClient}).Read(
				context.Background(),
				datasource.ReadRequest{Config: memberDataSourceTestConfig(
					t, memberSchema, test.id, test.email,
				)},
				&response,
			)
			if response.Diagnostics.HasError() {
				t.Fatalf("Read() diagnostics = %v", response.Diagnostics)
			}
			state := memberDataSourceStateModel(t, response.State)
			if state.ID.ValueString() != providerMemberID ||
				state.Email.ValueString() != member.Email || state.Name.ValueString() != member.Name {
				t.Fatalf("Read() state = %#v", state)
			}
			if fixture.nonReadRequests() != 0 {
				t.Fatalf("Member data source sent %d non-read requests", fixture.nonReadRequests())
			}
		})
	}
}

func TestMemberDataSourceRejectsMissingFuzzyAndDuplicateSelectorsWithoutDisclosure(t *testing.T) {
	t.Parallel()

	const runtimeEmail = "runtime.member@example.invalid"
	const runtimeName = "runtime-member-name-marker"
	tests := map[string]struct {
		id      types.String
		email   types.String
		members []client.Member
	}{
		"missing UUID": {
			id:    types.StringValue(providerMemberID),
			email: types.StringNull(),
		},
		"fuzzy email": {
			id:    types.StringNull(),
			email: types.StringValue("runtime.member"),
			members: []client.Member{{
				ID: providerMemberID, Email: runtimeEmail, Name: runtimeName,
			}},
		},
		"duplicate exact email ignoring case": {
			id:    types.StringNull(),
			email: types.StringValue(runtimeEmail),
			members: []client.Member{
				{ID: providerMemberID, Email: runtimeEmail, Name: runtimeName},
				{ID: providerMemberIDTwo, Email: strings.ToUpper(runtimeEmail), Name: runtimeName + " two"},
			},
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newMemberDataSourceFixture(t, test.members...)
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			memberSchema := memberDataSourceTestSchema(t)
			response := datasource.ReadResponse{State: tfsdk.State{Schema: memberSchema}}
			(&memberDataSource{client: apiClient}).Read(
				context.Background(),
				datasource.ReadRequest{Config: memberDataSourceTestConfig(
					t, memberSchema, test.id, test.email,
				)},
				&response,
			)
			if !response.Diagnostics.HasError() {
				t.Fatal("Read() accepted a missing, fuzzy, or non-unique Member selector")
			}
			formatted := fmt.Sprint(response.Diagnostics)
			for _, unsafe := range []string{
				runtimeEmail,
				runtimeName,
				providerMemberID,
				providerMemberIDTwo,
				"/api/v1/members",
			} {
				if strings.Contains(formatted, unsafe) {
					t.Fatalf("diagnostic exposed runtime value %q: %s", unsafe, formatted)
				}
			}
		})
	}
}

func TestMemberModelsCanonicalizeSafeFieldsAndRedactFormatting(t *testing.T) {
	t.Parallel()

	member := client.Member{
		ID: strings.ToUpper(providerMemberID), Email: "member@example.invalid", Name: "Member Name",
	}
	canonical, err := canonicalizeRemoteMember(member)
	if err != nil || canonical.ID != providerMemberID {
		t.Fatalf("canonicalizeRemoteMember() = %#v/%v", canonical, err)
	}
	model := flattenMember(canonical)
	formatted := fmt.Sprintf("%v|%+v|%#v", model, model, model)
	for _, unsafe := range []string{providerMemberID, member.Email, member.Name} {
		if strings.Contains(formatted, unsafe) {
			t.Fatalf("Member model formatting exposed runtime value %q", unsafe)
		}
	}
	if _, err := canonicalizeRemoteMember(client.Member{ID: providerMemberID}); err == nil {
		t.Fatal("canonicalizeRemoteMember() accepted a Member without email")
	}
}

type memberDataSourceFixture struct {
	t *testing.T

	mu               sync.Mutex
	members          []client.Member
	nonReadCallCount int
}

func newMemberDataSourceFixture(
	t *testing.T,
	members ...client.Member,
) *memberDataSourceFixture {
	t.Helper()
	return &memberDataSourceFixture{t: t, members: append([]client.Member(nil), members...)}
}

func (f *memberDataSourceFixture) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if request.Method != http.MethodGet {
		f.nonReadCallCount++
		f.t.Fatalf("Member data source sent mutation %s %s", request.Method, request.URL.EscapedPath())
	}
	if request.URL.EscapedPath() == "/api/v1/members" {
		items := make([]map[string]any, 0, len(f.members))
		for _, member := range f.members {
			items = append(items, map[string]any{
				"id": member.ID, "email": member.Email, "name": member.Name,
				"invitorId": "ignored-invitor", "initialPassword": "must-not-be-decoded",
				"groups": []map[string]any{{"id": "ignored-group"}},
			})
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(items), "items": items,
		})
		return
	}
	const prefix = "/api/v1/members/"
	if strings.HasPrefix(request.URL.EscapedPath(), prefix) {
		memberID := strings.TrimPrefix(request.URL.EscapedPath(), prefix)
		for _, member := range f.members {
			if client.EqualUUID(member.ID, memberID) {
				writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
					"id": member.ID, "email": member.Email, "name": member.Name,
					"initialPassword": "must-not-be-decoded",
				})
				return
			}
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
		return
	}
	f.t.Fatalf("unexpected Member data source request %s %s", request.Method, request.URL.EscapedPath())
}

func (f *memberDataSourceFixture) nonReadRequests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.nonReadCallCount
}

func memberDataSourceTestSchema(t *testing.T) datasourceschema.Schema {
	t.Helper()
	var response datasource.SchemaResponse
	(&memberDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("Member data source schema diagnostics = %v", response.Diagnostics)
	}
	return response.Schema
}

func memberDataSourceTestConfig(
	t *testing.T,
	memberSchema datasourceschema.Schema,
	id types.String,
	email types.String,
) tfsdk.Config {
	t.Helper()
	configured := memberModel{
		ID: id, Email: email, Name: types.StringNull(),
	}
	state := tfsdk.State{Schema: memberSchema}
	if diagnostics := state.Set(context.Background(), &configured); diagnostics.HasError() {
		t.Fatalf("initialize Member data source config: %v", diagnostics)
	}
	return tfsdk.Config{Schema: memberSchema, Raw: state.Raw}
}

func memberDataSourceStateModel(t *testing.T, state tfsdk.State) memberModel {
	t.Helper()
	var model memberModel
	if diagnostics := state.Get(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("read Member data source state: %v", diagnostics)
	}
	return model
}
