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
	_ datasource.DataSource                   = (*policyDataSource)(nil)
	_ datasource.DataSourceWithConfigure      = (*policyDataSource)(nil)
	_ datasource.DataSourceWithValidateConfig = (*policyDataSource)(nil)
)

const policyDataSourceSelectorDetail = "Configure exactly one Policy selector: `id` for an exact UUID or `key` for an organization-scoped case-sensitive exact key."

type policyDataSource struct {
	client *client.Client
}

func newPolicyDataSource() datasource.DataSource {
	return &policyDataSource{}
}

func (d *policyDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_policy"
}

func (d *policyDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads one custom or built-in FeatBit Policy by exact UUID or organization-scoped case-sensitive exact key.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Exact Policy UUID. Configure exactly one of `id` or `key`.",
				Validators: []validator.String{
					uuidValidator{},
				},
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Policy display name.",
			},
			"key": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Organization-scoped case-sensitive exact Policy key. Configure exactly one of `id` or `key`.",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Policy description, canonicalized to an empty string when absent.",
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Policy type: CustomerManaged or SysManaged.",
			},
			"statements": policyDataSourceStatementsAttribute(),
		},
	}
}

func policyDataSourceStatementsAttribute() schema.SetNestedAttribute {
	return schema.SetNestedAttribute{
		Computed:            true,
		MarkdownDescription: "Canonical unordered Policy statements. Built-in statement shapes are observed read-only and do not widen the managed Policy schema.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"resource_type": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Server-reported statement resource type.",
				},
				"effect": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Server-reported statement effect.",
				},
				"actions": schema.SetAttribute{
					Computed:            true,
					ElementType:         types.StringType,
					MarkdownDescription: "Canonical statement action set.",
				},
				"resources": schema.SetAttribute{
					Computed:            true,
					ElementType:         types.StringType,
					MarkdownDescription: "Canonical statement resource selector set.",
				},
			},
		},
	}
}

func (d *policyDataSource) ValidateConfig(
	ctx context.Context,
	req datasource.ValidateConfigRequest,
	resp *datasource.ValidateConfigResponse,
) {
	_, _, diagnostics := readExactlyOneStringSelector(
		ctx,
		req.Config,
		path.Root("id"),
		path.Root("key"),
		"Invalid FeatBit Policy Selector",
		policyDataSourceSelectorDetail,
	)
	resp.Diagnostics.Append(diagnostics...)
}

func (d *policyDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	d.client = clientFromProviderData(req.ProviderData, "Data Source", &resp.Diagnostics)
}

func (d *policyDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	policyID, key, diagnostics := readExactlyOneStringSelector(
		ctx,
		req.Config,
		path.Root("id"),
		path.Root("key"),
		"Invalid FeatBit Policy Selector",
		policyDataSourceSelectorDetail,
	)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if policyID.IsUnknown() || key.IsUnknown() {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Policy",
			"The configured Policy selector is not known yet. Terraform must resolve it before the Policy can be read.",
		)
		return
	}
	if !requireAPIClient(d.client, "reading a Policy", &resp.Diagnostics) {
		return
	}

	var policy client.Policy
	var found bool
	var err error
	selector := "UUID"
	if !policyID.IsNull() {
		policy, found, err = d.client.GetPolicy(ctx, policyID.ValueString())
	} else {
		selector = "key"
		policy, found, err = d.client.GetPolicyByKey(ctx, key.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Policy",
			"The provider could not confirm the requested Policy through the documented public API. "+err.Error()+".",
		)
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"FeatBit Policy Not Found",
			"No Policy with the configured exact "+selector+" exists in the complete token-scoped Policy collection.",
		)
		return
	}
	canonical, err := canonicalizeRemoteObservedPolicy(policy)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Policy Definition",
			"The exact Policy response could not be canonicalized safely. Correct the remote definition or select another Policy.",
		)
		return
	}
	state := flattenObservedPolicy(canonical)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
