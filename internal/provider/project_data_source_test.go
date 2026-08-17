// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	providerProjectID    = "11111111-1111-4111-8111-111111111111"
	providerEnvironmentA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	providerEnvironmentB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

func TestProjectDataSourceMetadataAndSchema(t *testing.T) {
	t.Parallel()

	dataSource := &projectDataSource{}
	var metadataResponse datasource.MetadataResponse
	dataSource.Metadata(
		context.Background(),
		datasource.MetadataRequest{ProviderTypeName: "featbit"},
		&metadataResponse,
	)
	if metadataResponse.TypeName != "featbit_project" {
		t.Fatalf("type name = %q", metadataResponse.TypeName)
	}

	var schemaResponse datasource.SchemaResponse
	dataSource.Schema(context.Background(), datasource.SchemaRequest{}, &schemaResponse)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics = %v", schemaResponse.Diagnostics)
	}
	if got := len(schemaResponse.Schema.Attributes); got != 4 {
		t.Fatalf("attribute count = %d, want 4", got)
	}
	idAttribute, ok := schemaResponse.Schema.Attributes["id"].(datasourceschema.StringAttribute)
	if !ok || idAttribute.Required || !idAttribute.Optional || !idAttribute.Computed {
		t.Fatalf("id attribute = %#v", schemaResponse.Schema.Attributes["id"])
	}
	if len(idAttribute.Validators) != 1 {
		t.Fatalf("id validator count = %d, want 1", len(idAttribute.Validators))
	}
	nameAttribute, ok := schemaResponse.Schema.Attributes["name"].(datasourceschema.StringAttribute)
	if !ok || !nameAttribute.Computed || nameAttribute.Required || nameAttribute.Optional {
		t.Fatalf("name attribute = %#v", schemaResponse.Schema.Attributes["name"])
	}
	keyAttribute, ok := schemaResponse.Schema.Attributes["key"].(datasourceschema.StringAttribute)
	if !ok || keyAttribute.Required || !keyAttribute.Optional || !keyAttribute.Computed {
		t.Fatalf("key attribute = %#v", schemaResponse.Schema.Attributes["key"])
	}
	environments, ok := schemaResponse.Schema.Attributes["environments"].(datasourceschema.ListNestedAttribute)
	if !ok || !environments.Computed || environments.Required || environments.Optional {
		t.Fatalf("environments attribute = %#v", schemaResponse.Schema.Attributes["environments"])
	}
	if got := len(environments.NestedObject.Attributes); got != 4 {
		t.Fatalf("environment nested attribute count = %d, want 4", got)
	}
}

func TestProjectDataSourceSelectorValidation(t *testing.T) {
	t.Parallel()

	projectSchema := projectDataSourceTestSchema(t)
	tests := map[string]struct {
		id        types.String
		key       types.String
		wantError bool
	}{
		"exact UUID": {
			id:  types.StringValue(providerProjectID),
			key: types.StringNull(),
		},
		"exact key": {
			id:  types.StringNull(),
			key: types.StringValue("exact-project-key"),
		},
		"unknown UUID reference": {
			id:  types.StringUnknown(),
			key: types.StringNull(),
		},
		"missing selector": {
			id:        types.StringNull(),
			key:       types.StringNull(),
			wantError: true,
		},
		"both selectors": {
			id:        types.StringValue(providerProjectID),
			key:       types.StringValue("exact-project-key"),
			wantError: true,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var response datasource.ValidateConfigResponse
			(&projectDataSource{}).ValidateConfig(
				context.Background(),
				datasource.ValidateConfigRequest{
					Config: projectDataSourceTestConfig(t, projectSchema, test.id, test.key),
				},
				&response,
			)
			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf("ValidateConfig() error = %t, want %t: %v", got, test.wantError, response.Diagnostics)
			}
		})
	}
}

func TestProjectDataSourceReadByExactSelector(t *testing.T) {
	t.Parallel()

	const exactKey = "exact-project-key"
	tests := map[string]struct {
		id       types.String
		key      types.String
		wantPath string
		response string
	}{
		"UUID": {
			id:       types.StringValue(providerProjectID),
			key:      types.StringNull(),
			wantPath: "/api/v1/projects/" + providerProjectID,
			response: providerProjectJSON(providerProjectID, "Project", exactKey),
		},
		"key": {
			id:       types.StringNull(),
			key:      types.StringValue(exactKey),
			wantPath: "/api/v1/projects",
			response: "[" + providerProjectJSON(providerProjectID, "Project", exactKey) + "]",
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			requestCount := 0
			apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
				func(response http.ResponseWriter, request *http.Request) {
					requestCount++
					if request.Method != http.MethodGet || request.URL.EscapedPath() != test.wantPath ||
						request.URL.RawQuery != "" {
						t.Fatalf("request = %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
					}
					writeProjectResourceEnvelope(t, response, http.StatusOK, test.response)
				},
			))
			defer closeServer()

			projectSchema := projectDataSourceTestSchema(t)
			response := datasource.ReadResponse{State: tfsdk.State{Schema: projectSchema}}
			(&projectDataSource{client: apiClient}).Read(
				context.Background(),
				datasource.ReadRequest{
					Config: projectDataSourceTestConfig(t, projectSchema, test.id, test.key),
				},
				&response,
			)
			if response.Diagnostics.HasError() {
				t.Fatalf("Read() diagnostics = %v", response.Diagnostics)
			}
			var state projectModel
			if diagnostics := response.State.Get(context.Background(), &state); diagnostics.HasError() {
				t.Fatalf("read Project data source state: %v", diagnostics)
			}
			if state.ID.ValueString() != providerProjectID || state.Name.ValueString() != "Project" ||
				state.Key.ValueString() != exactKey || len(state.Environments) != 2 {
				t.Fatalf("Read() state = %#v", state)
			}
			if requestCount != 1 {
				t.Fatalf("request count = %d, want 1", requestCount)
			}
		})
	}
}

func TestProjectDataSourceKeyReadRejectsZeroAndDuplicateMatches(t *testing.T) {
	t.Parallel()

	const exactKey = "runtime-project-key"
	tests := map[string]string{
		"exact zero": `[]`,
		"duplicate exact key": `[` +
			providerProjectJSON(providerProjectID, "First", exactKey) + `,` +
			providerProjectJSON("22222222-2222-4222-8222-222222222222", "Second", exactKey) + `]`,
	}

	for name, body := range tests {
		name := name
		body := body
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
				func(response http.ResponseWriter, request *http.Request) {
					writeProjectResourceEnvelope(t, response, http.StatusOK, body)
				},
			))
			defer closeServer()

			projectSchema := projectDataSourceTestSchema(t)
			response := datasource.ReadResponse{State: tfsdk.State{Schema: projectSchema}}
			(&projectDataSource{client: apiClient}).Read(
				context.Background(),
				datasource.ReadRequest{Config: projectDataSourceTestConfig(
					t,
					projectSchema,
					types.StringNull(),
					types.StringValue(exactKey),
				)},
				&response,
			)
			if !response.Diagnostics.HasError() {
				t.Fatal("Read() accepted a non-unique exact Project key")
			}
			formatted := fmt.Sprint(response.Diagnostics)
			for _, unsafe := range []string{exactKey, providerProjectID, "/api/v1/projects"} {
				if strings.Contains(formatted, unsafe) {
					t.Fatal("exact-key diagnostic exposed a runtime Project identity")
				}
			}
		})
	}
}

func TestProjectDataSourceConfigureTypeChecksWithoutRequest(t *testing.T) {
	t.Parallel()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requestCount++
	}))
	defer server.Close()
	apiURL, err := url.Parse(server.URL + "/api/v1")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	apiClient, err := client.New(apiURL, syntheticProviderAccessToken, client.Options{
		HTTPTimeout:     client.DefaultHTTPTimeout,
		MaxConcurrency:  client.DefaultMaxConcurrency,
		MaxRetries:      client.DefaultMaxRetries,
		ProviderVersion: "test",
	})
	if err != nil {
		t.Fatalf("client.New() error = %v", err)
	}

	dataSource := &projectDataSource{}
	var response datasource.ConfigureResponse
	dataSource.Configure(
		context.Background(),
		datasource.ConfigureRequest{ProviderData: apiClient},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Configure() diagnostics = %v", response.Diagnostics)
	}
	if dataSource.client != apiClient {
		t.Fatal("Configure() did not retain the typed API client")
	}
	if requestCount != 0 {
		t.Fatalf("Configure() executed %d HTTP requests", requestCount)
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

func TestFlattenProjectCanonicalizesEnvironments(t *testing.T) {
	t.Parallel()

	project := client.Project{
		ID:   providerProjectID,
		Name: "Project",
		Key:  "project",
		Environments: []client.ProjectEnvironment{
			{ID: providerEnvironmentB, Name: "Prod", Key: "prod", Description: "Production"},
			{ID: providerEnvironmentA, Name: "Dev", Key: "dev"},
		},
	}
	model := flattenProject(project)
	if model.ID.ValueString() != providerProjectID || model.Name.ValueString() != "Project" ||
		model.Key.ValueString() != "project" {
		t.Fatalf("flattenProject() project fields = %#v", model)
	}
	if len(model.Environments) != 2 || model.Environments[0].Key.ValueString() != "dev" ||
		model.Environments[1].Key.ValueString() != "prod" {
		t.Fatalf("flattenProject() environments = %#v", model.Environments)
	}
	if model.Environments[0].Description.ValueString() != "" {
		t.Fatal("missing description did not canonicalize to the empty string")
	}
}

func projectDataSourceTestSchema(t *testing.T) datasourceschema.Schema {
	t.Helper()
	var response datasource.SchemaResponse
	(&projectDataSource{}).Schema(
		context.Background(),
		datasource.SchemaRequest{},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Project data source schema diagnostics = %v", response.Diagnostics)
	}
	return response.Schema
}

func projectDataSourceTestConfig(
	t *testing.T,
	projectSchema datasourceschema.Schema,
	id types.String,
	key types.String,
) tfsdk.Config {
	t.Helper()
	configured := projectModel{
		ID:           id,
		Name:         types.StringNull(),
		Key:          key,
		Environments: nil,
	}
	state := tfsdk.State{Schema: projectSchema}
	if diagnostics := state.Set(context.Background(), &configured); diagnostics.HasError() {
		t.Fatalf("initialize Project data source config: %v", diagnostics)
	}
	return tfsdk.Config{Schema: projectSchema, Raw: state.Raw}
}
