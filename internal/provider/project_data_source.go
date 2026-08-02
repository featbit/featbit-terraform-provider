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
	_ datasource.DataSource              = (*projectDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*projectDataSource)(nil)
)

type projectDataSource struct {
	client *client.Client
}

func newProjectDataSource() datasource.DataSource {
	return &projectDataSource{}
}

func (d *projectDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (d *projectDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads one FeatBit Project by its exact UUID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Exact Project UUID.",
				Validators: []validator.String{
					uuidValidator{},
				},
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Project display name.",
			},
			"key": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Project key.",
			},
			"environments": projectDataSourceEnvironmentsAttribute(),
		},
	}
}

func projectDataSourceEnvironmentsAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Computed: true,
		MarkdownDescription: "Canonical, non-owning observations of the Project environments. " +
			"Secret values and settings are not exposed.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"id": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Environment UUID.",
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
					MarkdownDescription: "Environment description.",
				},
			},
		},
	}
}

func (d *projectDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	d.client = clientFromProviderData(req.ProviderData, "Data Source", &resp.Diagnostics)
}

func (d *projectDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var projectID types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &projectID)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireAPIClient(d.client, "reading a Project", &resp.Diagnostics) {
		return
	}

	project, found, err := d.client.GetProject(ctx, projectID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Project",
			"The provider could not confirm the requested Project through the documented public API. "+
				err.Error()+".",
		)
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"FeatBit Project Not Found",
			"No Project with the configured exact UUID exists in the complete Project collection.",
		)
		return
	}

	state := flattenProject(project)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
