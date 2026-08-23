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
)

var (
	_ datasource.DataSource                   = (*groupDataSource)(nil)
	_ datasource.DataSourceWithConfigure      = (*groupDataSource)(nil)
	_ datasource.DataSourceWithValidateConfig = (*groupDataSource)(nil)
)

const groupDataSourceSelectorDetail = "Configure exactly one Group selector: `id` for an exact UUID or `name` for an organization-scoped case-sensitive exact name."

type groupDataSource struct {
	client *client.Client
}

func newGroupDataSource() datasource.DataSource {
	return &groupDataSource{}
}

func (d *groupDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (d *groupDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads one existing FeatBit Group by exact UUID or organization-scoped case-sensitive exact name without adopting its lifecycle or relationships.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Exact Group UUID. Configure exactly one of `id` or `name`.",
				Validators: []validator.String{
					uuidValidator{},
				},
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Organization-scoped case-sensitive exact Group name. Configure exactly one of `id` or `name`.",
				Validators: []validator.String{
					nonEmptyStringValidator{object: "Group", field: "name"},
				},
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Group description, canonicalized to an empty string when absent.",
			},
		},
	}
}

func (d *groupDataSource) ValidateConfig(
	ctx context.Context,
	req datasource.ValidateConfigRequest,
	resp *datasource.ValidateConfigResponse,
) {
	_, _, diagnostics := readExactlyOneStringSelector(
		ctx,
		req.Config,
		path.Root("id"),
		path.Root("name"),
		"Invalid FeatBit Group Selector",
		groupDataSourceSelectorDetail,
	)
	resp.Diagnostics.Append(diagnostics...)
}

func (d *groupDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	d.client = clientFromProviderData(req.ProviderData, "Data Source", &resp.Diagnostics)
}

func (d *groupDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	groupID, name, diagnostics := readExactlyOneStringSelector(
		ctx,
		req.Config,
		path.Root("id"),
		path.Root("name"),
		"Invalid FeatBit Group Selector",
		groupDataSourceSelectorDetail,
	)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if groupID.IsUnknown() || name.IsUnknown() {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Group",
			"The configured Group selector is not known yet. Terraform must resolve it before the Group can be read.",
		)
		return
	}
	if !requireAPIClient(d.client, "reading a Group", &resp.Diagnostics) {
		return
	}

	var group client.Group
	var found bool
	var err error
	selector := "UUID"
	if !groupID.IsNull() {
		group, found, err = d.client.GetGroup(ctx, groupID.ValueString())
	} else {
		selector = "name"
		group, found, err = d.client.GetGroupByName(ctx, name.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Group",
			"The provider could not confirm the requested Group through the documented public API. "+err.Error()+".",
		)
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"FeatBit Group Not Found",
			"No Group with the configured exact "+selector+" exists in the complete token-scoped Group collection.",
		)
		return
	}
	canonical, err := canonicalizeRemoteGroup(group)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Group Definition",
			"The exact Group response could not be canonicalized safely. Correct the remote definition or select another Group.",
		)
		return
	}
	state := flattenGroup(canonical)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
