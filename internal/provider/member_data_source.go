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
	_ datasource.DataSource                   = (*memberDataSource)(nil)
	_ datasource.DataSourceWithConfigure      = (*memberDataSource)(nil)
	_ datasource.DataSourceWithValidateConfig = (*memberDataSource)(nil)
)

const memberDataSourceSelectorDetail = "Configure exactly one Member selector: `id` for an exact UUID or `email` for an organization-scoped case-insensitive full email."

type memberDataSource struct {
	client *client.Client
}

func newMemberDataSource() datasource.DataSource {
	return &memberDataSource{}
}

func (d *memberDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_member"
}

func (d *memberDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads one existing FeatBit Member by exact UUID or organization-scoped case-insensitive full email. It never invites, creates, updates, removes, or deletes the Member.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Exact Member UUID. Configure exactly one of `id` or `email`.",
				Validators: []validator.String{
					uuidValidator{},
				},
			},
			"email": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Organization-scoped case-insensitive full Member email. Configure exactly one of `id` or `email`; state retains the server's canonical spelling.",
				Validators: []validator.String{
					nonEmptyStringValidator{object: "Member", field: "email"},
				},
			},
			"name": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Server-reported Member display name.",
			},
		},
	}
}

func (d *memberDataSource) ValidateConfig(
	ctx context.Context,
	req datasource.ValidateConfigRequest,
	resp *datasource.ValidateConfigResponse,
) {
	_, _, diagnostics := readExactlyOneStringSelector(
		ctx,
		req.Config,
		path.Root("id"),
		path.Root("email"),
		"Invalid FeatBit Member Selector",
		memberDataSourceSelectorDetail,
	)
	resp.Diagnostics.Append(diagnostics...)
}

func (d *memberDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	d.client = clientFromProviderData(req.ProviderData, "Data Source", &resp.Diagnostics)
}

func (d *memberDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	memberID, email, diagnostics := readExactlyOneStringSelector(
		ctx,
		req.Config,
		path.Root("id"),
		path.Root("email"),
		"Invalid FeatBit Member Selector",
		memberDataSourceSelectorDetail,
	)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if memberID.IsUnknown() || email.IsUnknown() {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Member",
			"The configured Member selector is not known yet. Terraform must resolve it before the Member can be read.",
		)
		return
	}
	if !requireAPIClient(d.client, "reading a Member", &resp.Diagnostics) {
		return
	}

	var member client.Member
	var found bool
	var err error
	selector := "UUID"
	if !memberID.IsNull() {
		member, found, err = d.client.GetMember(ctx, memberID.ValueString())
	} else {
		selector = "email"
		member, found, err = d.client.GetMemberByEmail(ctx, email.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Member",
			"The provider could not confirm the requested Member through the documented public API. "+err.Error()+".",
		)
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"FeatBit Member Not Found",
			"No Member with the configured exact "+selector+" exists in the complete token-scoped Member collection.",
		)
		return
	}
	canonical, err := canonicalizeRemoteMember(member)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Member Definition",
			"The exact Member response could not be canonicalized safely. Correct the remote definition or select another Member.",
		)
		return
	}
	state := flattenMember(canonical)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
