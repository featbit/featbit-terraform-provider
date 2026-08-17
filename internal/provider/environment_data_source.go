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
	_ datasource.DataSource                   = (*environmentDataSource)(nil)
	_ datasource.DataSourceWithConfigure      = (*environmentDataSource)(nil)
	_ datasource.DataSourceWithValidateConfig = (*environmentDataSource)(nil)
)

const environmentDataSourceSelectorDetail = "Configure exactly one Environment selector: `id` for an exact UUID or `key` for a case-sensitive exact key within `project_id`."

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
		MarkdownDescription: "Reads one FeatBit Environment within an exact parent Project by exact UUID or case-sensitive exact key.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Exact parent Project UUID.",
				Validators: []validator.String{
					uuidValidator{},
				},
			},
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Exact Environment UUID. Configure exactly one of `id` or `key`.",
				Validators: []validator.String{
					uuidValidator{},
				},
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Environment display name.",
			},
			"key": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Case-sensitive exact Environment key within `project_id`. Configure exactly one of `id` or `key`.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Environment description, canonicalized to an empty string when absent.",
			},
		},
	}
}

func (d *environmentDataSource) ValidateConfig(
	ctx context.Context,
	req datasource.ValidateConfigRequest,
	resp *datasource.ValidateConfigResponse,
) {
	resp.Diagnostics.Append(validateExactlyOneStringSelector(
		ctx,
		req.Config,
		path.Root("id"),
		path.Root("key"),
		"Invalid FeatBit Environment Selector",
		environmentDataSourceSelectorDetail,
	)...)
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
	var key types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("project_id"), &projectID)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &environmentID)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("key"), &key)...)
	resp.Diagnostics.Append(validateExactlyOneStringSelector(
		ctx,
		req.Config,
		path.Root("id"),
		path.Root("key"),
		"Invalid FeatBit Environment Selector",
		environmentDataSourceSelectorDetail,
	)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if projectID.IsUnknown() || environmentID.IsUnknown() || key.IsUnknown() {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Environment",
			"The configured Environment selector or parent Project UUID is not known yet. Terraform must resolve it before the Environment can be read.",
		)
		return
	}
	if !requireAPIClient(d.client, "reading an Environment", &resp.Diagnostics) {
		return
	}

	var environment client.Environment
	var found bool
	var err error
	selector := "UUID"
	if !environmentID.IsNull() {
		environment, found, err = d.client.GetEnvironment(
			ctx,
			projectID.ValueString(),
			environmentID.ValueString(),
		)
	} else {
		selector = "key"
		environment, found, err = d.client.GetEnvironmentByKey(
			ctx,
			projectID.ValueString(),
			key.ValueString(),
		)
	}
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
			"No Environment with the configured exact "+selector+" exists in the exact parent Project.",
		)
		return
	}

	state := flattenEnvironment(projectID.ValueString(), environment)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
