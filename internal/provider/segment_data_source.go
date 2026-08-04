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
	_ datasource.DataSource              = (*segmentDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*segmentDataSource)(nil)
)

type segmentDataSource struct {
	client *client.Client
}

func newSegmentDataSource() datasource.DataSource {
	return &segmentDataSource{}
}

func (d *segmentDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_segment"
}

func (d *segmentDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads one active FeatBit Segment by exact Environment and Segment UUIDs.",
		Attributes: map[string]schema.Attribute{
			"environment_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Exact Environment UUID from which the Segment is visible.",
				Validators: []validator.String{
					uuidValidator{},
				},
			},
			"id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Exact Segment UUID.",
				Validators: []validator.String{
					uuidValidator{},
				},
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Segment display name.",
			},
			"key": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Exact case-sensitive Segment key.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Segment description, canonicalized to an empty string when absent.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Exact Segment type: `environment-specific` or `shared`.",
			},
			"scopes": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Immutable fully qualified Segment scope RNs.",
			},
			"included_users": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Exact user keys explicitly included in the Segment.",
			},
			"excluded_users": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Exact user keys explicitly excluded from the Segment.",
			},
			"rules": segmentDataSourceRulesAttribute(),
			"tags": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Segment tags.",
			},
		},
	}
}

func segmentDataSourceRulesAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Ordered Segment rules.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"id": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Rule UUID.",
				},
				"name": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Rule display name.",
				},
				"conditions": schema.ListNestedAttribute{
					Computed:            true,
					MarkdownDescription: "Ordered conditions combined with logical AND.",
					NestedObject: schema.NestedAttributeObject{
						Attributes: map[string]schema.Attribute{
							"id": schema.StringAttribute{
								Computed:            true,
								MarkdownDescription: "Condition UUID.",
							},
							"property": schema.StringAttribute{
								Computed:            true,
								MarkdownDescription: "Exact user property name.",
							},
							"operator": schema.StringAttribute{
								Computed:            true,
								MarkdownDescription: "Exact documented condition operator.",
							},
							"value": schema.StringAttribute{
								Computed:            true,
								MarkdownDescription: "Canonical operator value encoding.",
							},
						},
					},
				},
			},
		},
	}
}

func (d *segmentDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	d.client = clientFromProviderData(req.ProviderData, "Data Source", &resp.Diagnostics)
}

func (d *segmentDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var environmentID types.String
	var segmentID types.String
	resp.Diagnostics.Append(
		req.Config.GetAttribute(ctx, path.Root("environment_id"), &environmentID)...,
	)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("id"), &segmentID)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireAPIClient(d.client, "reading a Segment", &resp.Diagnostics) {
		return
	}

	segment, err := d.client.GetSegment(
		ctx,
		environmentID.ValueString(),
		segmentID.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Segment",
			"The provider could not confirm the requested Segment through the documented exact public API. "+
				err.Error()+". The prior state was preserved.",
		)
		return
	}
	if segment.IsArchived {
		resp.Diagnostics.AddError(
			"FeatBit Segment Is Archived",
			"The configured exact Segment is archived. Restore it outside Terraform before reading it as an active data source, or select a different active exact UUID.",
		)
		return
	}
	if !client.EqualUUID(segment.EnvironmentID, environmentID.ValueString()) ||
		!client.EqualUUID(segment.ID, segmentID.ValueString()) {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Segment",
			"The public API returned a Segment that did not match the configured exact Environment and Segment UUIDs. The prior state was preserved.",
		)
		return
	}

	state, err := flattenSegment(segment)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Segment Definition",
			"The active Segment definition could not be canonicalized safely. Correct the remote definition before retrying.",
		)
		return
	}
	// Required identity inputs retain their exact configured UUID spelling;
	// server-owned rule and condition identities are canonical lowercase.
	state.EnvironmentID = environmentID
	state.ID = segmentID
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
