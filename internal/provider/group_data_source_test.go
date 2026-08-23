// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestGroupDataSourceMetadataConfigureSchemaAndSelectors(t *testing.T) {
	t.Parallel()

	dataSourceUnderTest := &groupDataSource{}
	var metadata datasource.MetadataResponse
	dataSourceUnderTest.Metadata(
		context.Background(),
		datasource.MetadataRequest{ProviderTypeName: "featbit"},
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
	var configure datasource.ConfigureResponse
	dataSourceUnderTest.Configure(
		context.Background(),
		datasource.ConfigureRequest{ProviderData: apiClient},
		&configure,
	)
	if configure.Diagnostics.HasError() || dataSourceUnderTest.client != apiClient {
		t.Fatalf("Configure() diagnostics/client = %v/%p", configure.Diagnostics, dataSourceUnderTest.client)
	}

	groupSchema := groupDataSourceTestSchema(t)
	if len(groupSchema.Attributes) != 3 {
		t.Fatalf("Group data source attributes = %v", groupSchema.Attributes)
	}
	id, ok := groupSchema.Attributes["id"].(datasourceschema.StringAttribute)
	if !ok || !id.Optional || !id.Computed || id.Required || len(id.Validators) != 1 {
		t.Fatalf("id schema = %#v", id)
	}
	name, ok := groupSchema.Attributes["name"].(datasourceschema.StringAttribute)
	if !ok || !name.Optional || !name.Computed || name.Required || len(name.Validators) != 1 {
		t.Fatalf("name schema = %#v", name)
	}
	description, ok := groupSchema.Attributes["description"].(datasourceschema.StringAttribute)
	if !ok || !description.Computed || description.Optional || description.Required {
		t.Fatalf("description schema = %#v", description)
	}
	for _, forbidden := range []string{"member_ids", "policy_ids", "members", "policies"} {
		if _, exists := groupSchema.Attributes[forbidden]; exists {
			t.Fatalf("Group data source unexpectedly exposes %q", forbidden)
		}
	}

	tests := map[string]struct {
		id        types.String
		name      types.String
		wantError bool
	}{
		"exact UUID": {
			id:   types.StringValue(providerGroupID),
			name: types.StringNull(),
		},
		"exact name": {
			id:   types.StringNull(),
			name: types.StringValue("Existing Operators"),
		},
		"unknown reference": {
			id:   types.StringUnknown(),
			name: types.StringNull(),
		},
		"missing": {
			id:        types.StringNull(),
			name:      types.StringNull(),
			wantError: true,
		},
		"both": {
			id:        types.StringValue(providerGroupID),
			name:      types.StringValue("Existing Operators"),
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
				datasource.ValidateConfigRequest{Config: groupDataSourceTestConfig(
					t,
					groupSchema,
					test.id,
					test.name,
				)},
				&response,
			)
			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf("ValidateConfig() error = %t, want %t: %v", got, test.wantError, response.Diagnostics)
			}
		})
	}
}

func TestGroupDataSourceReadsExistingGroupByExactSelector(t *testing.T) {
	t.Parallel()

	group := client.Group{
		ID: providerGroupID, Name: "Existing Operators", Description: "Shared IAM Group",
	}
	tests := map[string]struct {
		id   types.String
		name types.String
	}{
		"UUID": {
			id:   types.StringValue(providerGroupID),
			name: types.StringNull(),
		},
		"name": {
			id:   types.StringNull(),
			name: types.StringValue(group.Name),
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newGroupResourceFixture(t, group)
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			groupSchema := groupDataSourceTestSchema(t)
			response := datasource.ReadResponse{State: tfsdk.State{Schema: groupSchema}}
			(&groupDataSource{client: apiClient}).Read(
				context.Background(),
				datasource.ReadRequest{Config: groupDataSourceTestConfig(
					t,
					groupSchema,
					test.id,
					test.name,
				)},
				&response,
			)
			if response.Diagnostics.HasError() {
				t.Fatalf("Read() diagnostics = %v", response.Diagnostics)
			}
			state := groupStateModel(t, response.State)
			if state.ID.ValueString() != providerGroupID ||
				state.Name.ValueString() != group.Name ||
				state.Description.ValueString() != group.Description {
				t.Fatalf("Read() state = %#v", state)
			}
			if len(fixture.mutations()) != 0 {
				t.Fatalf("data source sent mutations = %v", fixture.mutations())
			}
		})
	}
}

func TestGroupDataSourceRejectsMissingAndDuplicateExactSelectorsWithoutDisclosure(t *testing.T) {
	t.Parallel()

	const runtimeName = "runtime-existing-group-marker"
	tests := map[string]struct {
		id     types.String
		name   types.String
		groups []client.Group
	}{
		"missing UUID": {
			id:   types.StringValue(providerGroupID),
			name: types.StringNull(),
		},
		"missing exact name": {
			id:   types.StringNull(),
			name: types.StringValue(runtimeName),
			groups: []client.Group{{
				ID: providerGroupID, Name: runtimeName + "-other",
			}},
		},
		"different case": {
			id:   types.StringNull(),
			name: types.StringValue(runtimeName),
			groups: []client.Group{{
				ID: providerGroupID, Name: strings.ToUpper(runtimeName),
			}},
		},
		"duplicate exact name": {
			id:   types.StringNull(),
			name: types.StringValue(runtimeName),
			groups: []client.Group{
				{ID: providerGroupID, Name: runtimeName},
				{ID: providerGroupIDTwo, Name: runtimeName},
			},
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newGroupResourceFixture(t, test.groups...)
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()
			groupSchema := groupDataSourceTestSchema(t)
			response := datasource.ReadResponse{State: tfsdk.State{Schema: groupSchema}}
			(&groupDataSource{client: apiClient}).Read(
				context.Background(),
				datasource.ReadRequest{Config: groupDataSourceTestConfig(
					t,
					groupSchema,
					test.id,
					test.name,
				)},
				&response,
			)
			if !response.Diagnostics.HasError() {
				t.Fatal("Read() accepted a missing or non-unique Group selector")
			}
			formatted := fmt.Sprint(response.Diagnostics)
			for _, unsafe := range []string{
				runtimeName,
				providerGroupID,
				providerGroupIDTwo,
				"/api/v1/groups",
			} {
				if strings.Contains(formatted, unsafe) {
					t.Fatalf("diagnostic exposed runtime value %q: %s", unsafe, formatted)
				}
			}
		})
	}
}

func groupDataSourceTestSchema(t *testing.T) datasourceschema.Schema {
	t.Helper()
	var response datasource.SchemaResponse
	(&groupDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("Group data source schema diagnostics = %v", response.Diagnostics)
	}
	return response.Schema
}

func groupDataSourceTestConfig(
	t *testing.T,
	groupSchema datasourceschema.Schema,
	id types.String,
	name types.String,
) tfsdk.Config {
	t.Helper()
	configured := groupModel{
		ID:          id,
		Name:        name,
		Description: types.StringNull(),
	}
	state := tfsdk.State{Schema: groupSchema}
	if diagnostics := state.Set(context.Background(), &configured); diagnostics.HasError() {
		t.Fatalf("initialize Group data source config: %v", diagnostics)
	}
	return tfsdk.Config{Schema: groupSchema, Raw: state.Raw}
}
