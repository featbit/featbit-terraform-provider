// Copyright IBM Corp. 2021, 2025
// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ provider.Provider = (*FeatBitProvider)(nil)

// FeatBitProvider is the Terraform provider implementation.
type FeatBitProvider struct {
	version string
}

// Metadata returns the provider type name and build version.
func (p *FeatBitProvider) Metadata(
	_ context.Context,
	_ provider.MetadataRequest,
	resp *provider.MetadataResponse,
) {
	resp.TypeName = "featbit"
	resp.Version = p.version
}

// Schema returns the provider configuration schema. Configuration attributes
// are added in P1-020 through P1-022.
func (p *FeatBitProvider) Schema(
	_ context.Context,
	_ provider.SchemaRequest,
	resp *provider.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Terraform provider for managing FeatBit through its documented public REST API.",
	}
}

// Configure is intentionally minimal until the API client is wired in P1-023.
func (p *FeatBitProvider) Configure(
	ctx context.Context,
	_ provider.ConfigureRequest,
	_ *provider.ConfigureResponse,
) {
	tflog.Debug(ctx, "Configuring FeatBit provider")
}

// Resources returns no managed resources during Phase 1.
func (p *FeatBitProvider) Resources(context.Context) []func() resource.Resource {
	return nil
}

// DataSources returns no data sources during Phase 1.
func (p *FeatBitProvider) DataSources(context.Context) []func() datasource.DataSource {
	return nil
}

// New creates a provider factory with the supplied build version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &FeatBitProvider{version: version}
	}
}
