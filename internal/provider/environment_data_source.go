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
	_ datasource.DataSource              = (*environmentDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*environmentDataSource)(nil)
)

type environmentDataSource struct {
	client *client.Client
}

func newEnvironmentDataSource() datasource.DataSource {
	return &environmentDataSource{}
}

func (d *environmentDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

func (d *environmentDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads one FeatBit Environment by its exact parent Project UUID and Environment UUID.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Exact parent Project UUID.",
				Validators: []validator.String{
					uuidValidator{},
				},
			},
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Exact Environment UUID.",
				Validators: []validator.String{
					uuidValidator{},
				},
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Environment display name.",
			},
			"key": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Environment key.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Environment description, canonicalized to an empty string when absent.",
			},
		},
	}
}

func (d *environmentDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	d.client = clientFromProviderData(req.ProviderData, "Data Source", &resp.Diagnostics)
}

func (d *environmentDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var projectID types.String
	var environmentID types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("project_id"), &projectID)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &environmentID)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireAPIClient(d.client, "reading an Environment", &resp.Diagnostics) {
		return
	}

	environment, found, err := d.client.GetEnvironment(
		ctx,
		projectID.ValueString(),
		environmentID.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Environment",
			"The provider could not confirm the requested Environment through the documented public API. "+
				err.Error()+".",
		)
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"FeatBit Environment Not Found",
			"No Environment with the configured exact UUID exists in the exact parent Project.",
		)
		return
	}

	state := flattenEnvironment(projectID.ValueString(), environment)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
