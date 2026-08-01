// Copyright IBM Corp. 2021, 2025
// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
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

func TestProviderSchemaIsMinimal(t *testing.T) {
	t.Parallel()

	providerUnderTest := New("test")()
	var response frameworkprovider.SchemaResponse

	providerUnderTest.Schema(context.Background(), frameworkprovider.SchemaRequest{}, &response)

	if response.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", response.Diagnostics)
	}
	if got := len(response.Schema.Attributes); got != 0 {
		t.Fatalf("expected no configuration attributes before P1-020, got %d", got)
	}
}

func TestProviderRegistrationsAreEmpty(t *testing.T) {
	t.Parallel()

	providerUnderTest := New("test")()

	if got := len(providerUnderTest.Resources(context.Background())); got != 0 {
		t.Fatalf("expected no Phase 1 resources, got %d", got)
	}
	if got := len(providerUnderTest.DataSources(context.Background())); got != 0 {
		t.Fatalf("expected no Phase 1 data sources, got %d", got)
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
