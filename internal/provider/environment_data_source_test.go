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
	for _, name := range []string{"project_id", "id"} {
		attribute, ok := environmentSchema.Attributes[name].(datasourceschema.StringAttribute)
		if !ok || !attribute.Required || attribute.Optional || attribute.Computed {
			t.Fatalf("%s attribute = %#v", name, environmentSchema.Attributes[name])
		}
		if len(attribute.Validators) != 1 {
			t.Fatalf("%s validator count = %d, want 1", name, len(attribute.Validators))
		}
	}
	for _, name := range []string{"name", "key", "description"} {
		attribute, ok := environmentSchema.Attributes[name].(datasourceschema.StringAttribute)
		if !ok || !attribute.Computed || attribute.Required || attribute.Optional {
			t.Fatalf("%s attribute = %#v", name, environmentSchema.Attributes[name])
		}
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
