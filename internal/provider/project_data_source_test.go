// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
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
	if !ok || !idAttribute.Required || idAttribute.Optional || idAttribute.Computed {
		t.Fatalf("id attribute = %#v", schemaResponse.Schema.Attributes["id"])
	}
	if len(idAttribute.Validators) != 1 {
		t.Fatalf("id validator count = %d, want 1", len(idAttribute.Validators))
	}
	for _, name := range []string{"name", "key"} {
		attribute, ok := schemaResponse.Schema.Attributes[name].(datasourceschema.StringAttribute)
		if !ok || !attribute.Computed || attribute.Required || attribute.Optional {
			t.Fatalf("%s attribute = %#v", name, schemaResponse.Schema.Attributes[name])
		}
	}
	environments, ok := schemaResponse.Schema.Attributes["environments"].(datasourceschema.ListNestedAttribute)
	if !ok || !environments.Computed || environments.Required || environments.Optional {
		t.Fatalf("environments attribute = %#v", schemaResponse.Schema.Attributes["environments"])
	}
	if got := len(environments.NestedObject.Attributes); got != 4 {
		t.Fatalf("environment nested attribute count = %d, want 4", got)
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
