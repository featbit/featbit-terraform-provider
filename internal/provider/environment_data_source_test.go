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

func TestEnvironmentDataSourceMetadataSchemaAndConfigure(t *testing.T) {
	t.Parallel()

	dataSource := &environmentDataSource{}
	var metadataResponse datasource.MetadataResponse
	dataSource.Metadata(
		context.Background(),
		datasource.MetadataRequest{ProviderTypeName: "featbit"},
		&metadataResponse,
	)
	if metadataResponse.TypeName != "featbit_environment" {
		t.Fatalf("type name = %q", metadataResponse.TypeName)
	}

	environmentSchema := environmentDataSourceTestSchema(t)
	if got := len(environmentSchema.Attributes); got != 5 {
		t.Fatalf("attribute count = %d, want 5", got)
	}
	projectIDAttribute, ok := environmentSchema.Attributes["project_id"].(datasourceschema.StringAttribute)
	if !ok || !projectIDAttribute.Required || projectIDAttribute.Optional || projectIDAttribute.Computed {
		t.Fatalf("project_id attribute = %#v", environmentSchema.Attributes["project_id"])
	}
	if len(projectIDAttribute.Validators) != 1 {
		t.Fatalf("project_id validator count = %d, want 1", len(projectIDAttribute.Validators))
	}
	idAttribute, ok := environmentSchema.Attributes["id"].(datasourceschema.StringAttribute)
	if !ok || idAttribute.Required || !idAttribute.Optional || !idAttribute.Computed {
		t.Fatalf("id attribute = %#v", environmentSchema.Attributes["id"])
	}
	if len(idAttribute.Validators) != 1 {
		t.Fatalf("id validator count = %d, want 1", len(idAttribute.Validators))
	}
	for _, name := range []string{"name", "description"} {
		attribute, ok := environmentSchema.Attributes[name].(datasourceschema.StringAttribute)
		if !ok || !attribute.Computed || attribute.Required || attribute.Optional {
			t.Fatalf("%s attribute = %#v", name, environmentSchema.Attributes[name])
		}
	}
	keyAttribute, ok := environmentSchema.Attributes["key"].(datasourceschema.StringAttribute)
	if !ok || keyAttribute.Required || !keyAttribute.Optional || !keyAttribute.Computed {
		t.Fatalf("key attribute = %#v", environmentSchema.Attributes["key"])
	}

	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {
			t.Fatal("Configure() executed an HTTP request")
		},
	))
	defer closeServer()
	var configureResponse datasource.ConfigureResponse
	dataSource.Configure(
		context.Background(),
		datasource.ConfigureRequest{ProviderData: apiClient},
		&configureResponse,
	)
	if configureResponse.Diagnostics.HasError() || dataSource.client != apiClient {
		t.Fatalf("Configure() diagnostics = %v", configureResponse.Diagnostics)
	}

	var wrongTypeResponse datasource.ConfigureResponse
	dataSource.Configure(
		context.Background(),
		datasource.ConfigureRequest{ProviderData: struct{}{}},
		&wrongTypeResponse,
	)
	if !wrongTypeResponse.Diagnostics.HasError() {
		t.Fatal("Configure() accepted an unexpected provider data type")
	}
}

func TestEnvironmentDataSourceSelectorValidation(t *testing.T) {
	t.Parallel()

	environmentSchema := environmentDataSourceTestSchema(t)
	tests := map[string]struct {
		id        types.String
		key       types.String
		wantError bool
	}{
		"exact UUID": {
			id:  types.StringValue(providerEnvironmentA),
			key: types.StringNull(),
		},
		"exact key": {
			id:  types.StringNull(),
			key: types.StringValue("staging"),
		},
		"unknown key reference": {
			id:  types.StringNull(),
			key: types.StringUnknown(),
		},
		"missing selector": {
			id:        types.StringNull(),
			key:       types.StringNull(),
			wantError: true,
		},
		"both selectors": {
			id:        types.StringValue(providerEnvironmentA),
			key:       types.StringValue("staging"),
			wantError: true,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var response datasource.ValidateConfigResponse
			(&environmentDataSource{}).ValidateConfig(
				context.Background(),
				datasource.ValidateConfigRequest{Config: environmentDataSourceTestConfig(
					t,
					environmentSchema,
					types.StringValue(providerProjectID),
					test.id,
					test.key,
				)},
				&response,
			)
			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf("ValidateConfig() error = %t, want %t: %v", got, test.wantError, response.Diagnostics)
			}
		})
	}
}

func TestEnvironmentDataSourceReadCanonicalizesAndOmitsSecrets(t *testing.T) {
	t.Parallel()

	const secretMarker = "test-only-data-source-secret-marker"
	requestCount := 0
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			requestCount++
			switch requestCount {
			case 1:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					`{"id":"`+providerEnvironmentA+`","name":"Staging",`+
						`"secrets":[{"value":"`+secretMarker+`"}],`+
						`"settings":{"requireChangeComment":true}}`,
				)
			case 2:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					`{"id":"`+providerProjectID+`","name":"Parent","key":"parent",`+
						`"environments":[{"id":"`+providerEnvironmentA+`",`+
						`"name":"Parent Name","key":"staging","description":"",`+
						`"secrets":[{"value":"`+secretMarker+`"}]}]}`,
				)
			default:
				t.Fatalf("unexpected request %s %s", request.Method, request.URL.EscapedPath())
			}
		},
	))
	defer closeServer()

	environmentSchema := environmentDataSourceTestSchema(t)
	configured := environmentModel{
		ProjectID:   types.StringValue(providerProjectID),
		ID:          types.StringValue(providerEnvironmentA),
		Name:        types.StringNull(),
		Key:         types.StringNull(),
		Description: types.StringNull(),
	}
	configuredState := tfsdk.State{Schema: environmentSchema}
	if diagnostics := configuredState.Set(context.Background(), &configured); diagnostics.HasError() {
		t.Fatalf("initialize Environment data source config: %v", diagnostics)
	}
	config := tfsdk.Config{Schema: environmentSchema, Raw: configuredState.Raw}
	response := datasource.ReadResponse{State: tfsdk.State{Schema: environmentSchema}}
	(&environmentDataSource{client: apiClient}).Read(
		context.Background(),
		datasource.ReadRequest{Config: config},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Read() diagnostics = %v", response.Diagnostics)
	}
	var state environmentModel
	if diagnostics := response.State.Get(context.Background(), &state); diagnostics.HasError() {
		t.Fatalf("read Environment data source state: %v", diagnostics)
	}
	if state.ProjectID.ValueString() != providerProjectID ||
		state.ID.ValueString() != providerEnvironmentA ||
		state.Name.ValueString() != "Staging" ||
		state.Key.ValueString() != "staging" ||
		state.Description.ValueString() != "" {
		t.Fatalf("Read() state = %#v", state)
	}
	formatted := fmt.Sprintf(
		"%v|%+v|%#v|%v|%v",
		state,
		state,
		state,
		response.State.Raw,
		response.Diagnostics,
	)
	if strings.Contains(formatted, secretMarker) || strings.Contains(formatted, "requireChangeComment") {
		t.Fatal("Environment data source state or diagnostics retained secrets or settings")
	}
	if requestCount != 2 {
		t.Fatalf("Read() request count = %d, want 2", requestCount)
	}
}

func TestEnvironmentDataSourceReadByExactKeyUsesParentProject(t *testing.T) {
	t.Parallel()

	const (
		exactKey     = "staging"
		secretMarker = "test-only-key-lookup-secret-marker"
	)
	requestCount := 0
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			requestCount++
			if request.Method != http.MethodGet ||
				request.URL.EscapedPath() != "/api/v1/projects/"+providerProjectID ||
				request.URL.RawQuery != "" {
				t.Fatalf("request = %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
			}
			writeProjectResourceEnvelope(
				t,
				response,
				http.StatusOK,
				providerEnvironmentParentJSON(
					providerProjectID,
					`[{"id":"`+providerEnvironmentB+`","name":"Other","key":"other"},`+
						`{"id":"`+providerEnvironmentA+`","name":"Staging","key":"`+exactKey+`",`+
						`"description":"Exact description","secrets":[{"value":"`+secretMarker+`"}],`+
						`"settings":{"requireChangeComment":true}}]`,
				),
			)
		},
	))
	defer closeServer()

	environmentSchema := environmentDataSourceTestSchema(t)
	response := datasource.ReadResponse{State: tfsdk.State{Schema: environmentSchema}}
	(&environmentDataSource{client: apiClient}).Read(
		context.Background(),
		datasource.ReadRequest{Config: environmentDataSourceTestConfig(
			t,
			environmentSchema,
			types.StringValue(providerProjectID),
			types.StringNull(),
			types.StringValue(exactKey),
		)},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Read() diagnostics = %v", response.Diagnostics)
	}
	var state environmentModel
	if diagnostics := response.State.Get(context.Background(), &state); diagnostics.HasError() {
		t.Fatalf("read Environment data source state: %v", diagnostics)
	}
	if state.ProjectID.ValueString() != providerProjectID ||
		state.ID.ValueString() != providerEnvironmentA ||
		state.Name.ValueString() != "Staging" ||
		state.Key.ValueString() != exactKey ||
		state.Description.ValueString() != "Exact description" {
		t.Fatalf("Read() state = %#v", state)
	}
	formatted := fmt.Sprint(state, response.State.Raw, response.Diagnostics)
	if strings.Contains(formatted, secretMarker) || strings.Contains(formatted, "requireChangeComment") {
		t.Fatal("Environment exact-key state retained secrets or settings")
	}
	if requestCount != 1 {
		t.Fatalf("Read() request count = %d, want 1", requestCount)
	}
}

func TestFlattenEnvironmentCanonicalizesMissingDescription(t *testing.T) {
	t.Parallel()

	model := flattenEnvironment(providerProjectID, client.Environment{
		ID:   providerEnvironmentA,
		Name: "Staging",
		Key:  "staging",
	})
	if model.ProjectID.ValueString() != providerProjectID ||
		model.ID.ValueString() != providerEnvironmentA ||
		model.Description.ValueString() != "" {
		t.Fatalf("flattenEnvironment() = %#v", model)
	}
}

func environmentDataSourceTestSchema(t *testing.T) datasourceschema.Schema {
	t.Helper()
	var response datasource.SchemaResponse
	(&environmentDataSource{}).Schema(
		context.Background(),
		datasource.SchemaRequest{},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Environment data source schema diagnostics = %v", response.Diagnostics)
	}
	return response.Schema
}

func environmentDataSourceTestConfig(
	t *testing.T,
	environmentSchema datasourceschema.Schema,
	projectID types.String,
	id types.String,
	key types.String,
) tfsdk.Config {
	t.Helper()
	configured := environmentModel{
		ProjectID:   projectID,
		ID:          id,
		Name:        types.StringNull(),
		Key:         key,
		Description: types.StringNull(),
	}
	state := tfsdk.State{Schema: environmentSchema}
	if diagnostics := state.Set(context.Background(), &configured); diagnostics.HasError() {
		t.Fatalf("initialize Environment data source config: %v", diagnostics)
	}
	return tfsdk.Config{Schema: environmentSchema, Raw: state.Raw}
}
