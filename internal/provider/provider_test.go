// Copyright IBM Corp. 2021, 2025
// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestProviderMetadata(t *testing.T) {
	t.Parallel()

	providerUnderTest := New("test")()
	var response frameworkprovider.MetadataResponse

	providerUnderTest.Metadata(context.Background(), frameworkprovider.MetadataRequest{}, &response)

	if response.TypeName != "featbit" {
		t.Fatalf("expected provider type name featbit, got %q", response.TypeName)
	}
	if response.Version != "test" {
		t.Fatalf("expected provider version test, got %q", response.Version)
	}
}

func TestProviderSchema(t *testing.T) {
	t.Parallel()

	providerUnderTest := New("test")()
	var response frameworkprovider.SchemaResponse

	providerUnderTest.Schema(context.Background(), frameworkprovider.SchemaRequest{}, &response)

	if response.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", response.Diagnostics)
	}
	if got := len(response.Schema.Attributes); got != 5 {
		t.Fatalf("expected five provider configuration attributes, got %d", got)
	}

	apiURLAttribute, ok := response.Schema.Attributes["api_url"].(providerschema.StringAttribute)
	if !ok {
		t.Fatalf("expected api_url to be a string attribute, got %T", response.Schema.Attributes["api_url"])
	}
	if !apiURLAttribute.Optional || apiURLAttribute.Required {
		t.Fatal("expected api_url to be optional and not required")
	}
	if got := len(apiURLAttribute.Validators); got != 1 {
		t.Fatalf("expected api_url to have one validator, got %d", got)
	}

	accessTokenAttribute, ok := response.Schema.Attributes["access_token"].(providerschema.StringAttribute)
	if !ok {
		t.Fatalf("expected access_token to be a string attribute, got %T", response.Schema.Attributes["access_token"])
	}
	if !accessTokenAttribute.Optional || accessTokenAttribute.Required || !accessTokenAttribute.Sensitive {
		t.Fatal("expected access_token to be optional, not required, and Sensitive")
	}
	if got := len(accessTokenAttribute.Validators); got != 1 {
		t.Fatalf("expected access_token to have one validator, got %d", got)
	}

	for _, name := range []string{"http_timeout_seconds", "max_concurrency", "max_retries"} {
		attribute, ok := response.Schema.Attributes[name].(providerschema.Int64Attribute)
		if !ok {
			t.Fatalf("expected %s to be an int64 attribute, got %T", name, response.Schema.Attributes[name])
		}
		if !attribute.Optional || attribute.Required {
			t.Fatalf("expected %s to be optional and not required", name)
		}
		if got := len(attribute.Validators); got != 1 {
			t.Fatalf("expected %s to have one validator, got %d", name, got)
		}
	}
}

func TestProviderRegistrations(t *testing.T) {
	t.Parallel()

	providerUnderTest := New("test")()

	if got := len(providerUnderTest.Resources(context.Background())); got != 5 {
		t.Fatalf("expected four core resources and Policy, got %d", got)
	}
	if got := len(providerUnderTest.DataSources(context.Background())); got != 5 {
		t.Fatalf("expected four core data sources and Policy, got %d", got)
	}
}

func TestProtocol6ProviderFactory(t *testing.T) {
	t.Parallel()

	factories := map[string]func() (tfprotov6.ProviderServer, error){
		"featbit": providerserver.NewProtocol6WithError(New("test")()),
	}
	testCase := resource.TestCase{ProtoV6ProviderFactories: factories}

	server, err := testCase.ProtoV6ProviderFactories["featbit"]()
	if err != nil {
		t.Fatalf("create Protocol v6 provider server: %v", err)
	}
	if server == nil {
		t.Fatal("expected a Protocol v6 provider server")
	}
}
