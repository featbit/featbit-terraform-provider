// Copyright IBM Corp. 2021, 2025
// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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

// Schema returns the provider configuration schema.
func (p *FeatBitProvider) Schema(
	_ context.Context,
	_ provider.SchemaRequest,
	resp *provider.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Terraform provider for managing FeatBit through its documented public REST API.",
		Attributes: map[string]schema.Attribute{
			"api_url": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "FeatBit API base URL. May also be set with `FEATBIT_API_URL`. " +
					"Defaults to `https://app-api.featbit.co`. The path may be empty or `/api/v1`.",
				Validators: []validator.String{
					apiURLValidator{},
				},
			},
		},
	}
}

// Configure resolves and validates provider settings. The API client is wired
// from the validated settings in P1-023.
func (p *FeatBitProvider) Configure(
	ctx context.Context,
	req provider.ConfigureRequest,
	resp *provider.ConfigureResponse,
) {
	tflog.Debug(ctx, "Configuring FeatBit provider")

	var model providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config, diagnostics := newProviderConfig(model, os.LookupEnv)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}

	// There are no Phase 1 resources or data sources yet. Keeping the resolved
	// configuration in both slots makes the P1-023 client handoff explicit.
	resp.DataSourceData = config
	resp.ResourceData = config
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
