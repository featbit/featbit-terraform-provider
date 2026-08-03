// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = (*featureFlagDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*featureFlagDataSource)(nil)
)

type featureFlagDataSource struct {
	client *client.Client
}

func newFeatureFlagDataSource() datasource.DataSource {
	return &featureFlagDataSource{}
}

func (d *featureFlagDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_feature_flag"
}

func (d *featureFlagDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads one active FeatBit Feature Flag by exact Environment UUID and case-sensitive key.",
		Attributes: map[string]schema.Attribute{
			"environment_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Exact parent Environment UUID.",
				Validators: []validator.String{
					uuidValidator{},
				},
			},
			"key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Exact case-sensitive Feature Flag key.",
				Validators: []validator.String{
					featureFlagKeyValidator{},
				},
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Feature Flag UUID.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Feature Flag display name.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Feature Flag description, canonicalized to an empty string when absent.",
			},
			"variation_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Canonical lowercase variation type.",
			},
			"variations": featureFlagDataSourceVariationsAttribute(),
		},
	}
}

func featureFlagDataSourceVariationsAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Canonical Feature Flag variations ordered by exact UUID.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"id": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Variation UUID.",
				},
				"name": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Variation display name.",
				},
				"value": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Canonical variation value.",
				},
			},
		},
	}
}

func (d *featureFlagDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	d.client = clientFromProviderData(req.ProviderData, "Data Source", &resp.Diagnostics)
}

func (d *featureFlagDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var environmentID types.String
	var key types.String
	resp.Diagnostics.Append(
		req.Config.GetAttribute(ctx, path.Root("environment_id"), &environmentID)...,
	)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("key"), &key)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireAPIClient(d.client, "reading a Feature Flag", &resp.Diagnostics) {
		return
	}

	flag, status, err := d.client.GetFeatureFlag(
		ctx,
		environmentID.ValueString(),
		key.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Feature Flag",
			"The provider could not confirm the requested Feature Flag through the documented public API. "+
				err.Error()+".",
		)
		return
	}
	switch status {
	case client.FeatureFlagStatusAbsent:
		resp.Diagnostics.AddError(
			"FeatBit Feature Flag Not Found",
			"No Feature Flag with the configured exact key exists in the complete active or archived views of the exact Environment.",
		)
		return
	case client.FeatureFlagStatusArchived:
		resp.Diagnostics.AddError(
			"FeatBit Feature Flag Is Archived",
			"The configured exact Feature Flag exists in the archived view. Restore it outside Terraform before reading it as an active data source, or select a different active exact key.",
		)
		return
	case client.FeatureFlagStatusActive:
		// Continue with the canonical active definition.
	default:
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Feature Flag",
			"The provider received an unconfirmed Feature Flag status and preserved the prior state.",
		)
		return
	}

	if !client.EqualUUID(flag.EnvironmentID, environmentID.ValueString()) ||
		flag.Key != key.ValueString() {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Feature Flag",
			"The public API returned a Feature Flag that did not match the configured exact Environment and key. The prior state was preserved.",
		)
		return
	}
	state, err := flattenFeatureFlag(flag, nil)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Feature Flag Definition",
			"The active Feature Flag definition could not be canonicalized safely. Correct the remote definition before retrying.",
		)
		return
	}
	// Preserve the exact configured UUID spelling while all server-owned IDs
	// remain canonical lowercase values.
	state.EnvironmentID = environmentID
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
