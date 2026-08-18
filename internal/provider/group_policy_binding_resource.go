// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"slices"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

var (
	_ resource.Resource                = (*groupPolicyBindingResource)(nil)
	_ resource.ResourceWithConfigure   = (*groupPolicyBindingResource)(nil)
	_ resource.ResourceWithImportState = (*groupPolicyBindingResource)(nil)
)

type groupPolicyBindingResource struct {
	client *client.Client
}

func newGroupPolicyBindingResource() resource.Resource {
	return &groupPolicyBindingResource{}
}

func (r *groupPolicyBindingResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_group_policy_binding"
}

func (r *groupPolicyBindingResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one exact FeatBit Group-to-Policy binding through the documented public API. It owns only the configured pair, not either endpoint or the Group's complete Policy collection.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Canonical synthetic binding ID in `<group_uuid>/<policy_uuid>` form.",
				PlanModifiers: []planmodifier.String{
					useStateForUnknownIfUnchanged(
						path.Root("group_id"),
						path.Root("policy_id"),
					),
				},
			},
			"group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Exact Group UUID. Changing it replaces the binding.",
				Validators: []validator.String{
					uuidValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"policy_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Exact custom or built-in Policy UUID. Changing it replaces the binding.",
				Validators: []validator.String{
					uuidValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *groupPolicyBindingResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	r.client = clientFromProviderData(req.ProviderData, "Resource", &resp.Diagnostics)
}

func (r *groupPolicyBindingResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	if !requireAPIClient(r.client, "managing a Group-Policy binding", &resp.Diagnostics) {
		return
	}
	var planModel groupPolicyBindingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planModel)...)
	if resp.Diagnostics.HasError() {
		return
	}
	identity, err := canonicalizeGroupPolicyBindingPlanModel(planModel)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Group-Policy Binding Plan",
			"The Group and Policy identifiers could not be canonicalized safely. No mutation was sent.",
		)
		return
	}

	endpointsPresent, err := r.bindingEndpointsPresent(ctx, identity)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Validate FeatBit Group-Policy Binding Endpoints",
			"The provider could not resolve both exact endpoint objects through their complete token-scoped collections. No mutation was sent. "+err.Error()+".",
		)
		return
	}
	if !endpointsPresent {
		resp.Diagnostics.AddError(
			"FeatBit Group-Policy Binding Endpoint Does Not Exist",
			"The exact Group or Policy does not exist in the token-scoped collections. No mutation was sent.",
		)
		return
	}

	present, err := r.readBinding(ctx, identity)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Check Existing FeatBit Group-Policy Binding",
			"The provider could not read the complete exact Group Policy collection. No mutation was sent. "+err.Error()+".",
		)
		return
	}
	if present {
		r.setBindingState(ctx, &resp.State, &resp.Diagnostics, identity)
		return
	}

	mutationErr := r.client.AddGroupPolicy(ctx, identity.GroupID, identity.PolicyID)
	present, readErr := r.readBinding(ctx, identity)
	if readErr != nil {
		detail := "The add request completed, but the complete exact Group Policy collection could not confirm the binding. Terraform did not retry or adopt an unconfirmed pair. " + readErr.Error() + "."
		if mutationErr != nil {
			detail = "The add request failed and the exact binding outcome could not be confirmed. Terraform did not retry or adopt an unconfirmed pair. " + mutationErr.Error() + "."
		}
		resp.Diagnostics.AddError("Unable to Confirm FeatBit Group-Policy Binding", detail)
		return
	}
	if !present {
		detail := "The exact Group-Policy pair is absent after the add request. Terraform did not retry or record the binding."
		if mutationErr != nil {
			detail = "The add request failed and the exact Group-Policy pair remains absent. Terraform did not retry or record the binding. " + mutationErr.Error() + "."
		}
		resp.Diagnostics.AddError("FeatBit Group-Policy Binding Was Not Created", detail)
		return
	}
	r.setBindingState(ctx, &resp.State, &resp.Diagnostics, identity)
}

func (r *groupPolicyBindingResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a Group-Policy binding", &resp.Diagnostics) {
		return
	}
	var priorModel groupPolicyBindingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &priorModel)...)
	if resp.Diagnostics.HasError() {
		return
	}
	identity, err := canonicalizeGroupPolicyBindingStateModel(priorModel)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Group-Policy Binding State",
			"The managed binding state does not contain one consistent canonical Group and Policy pair. Terraform state was preserved.",
		)
		return
	}
	present, err := r.readBinding(ctx, identity)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Group-Policy Binding",
			"The provider could not confirm the exact pair through a complete Group Policy collection. Terraform state was preserved. "+err.Error()+".",
		)
		return
	}
	if !present {
		resp.State.RemoveResource(ctx)
		return
	}
	r.setBindingState(ctx, &resp.State, &resp.Diagnostics, identity)
}

// Update is a read-only safety path. Both configurable attributes require
// replacement, so Terraform must never use Update to change an exact pair.
func (r *groupPolicyBindingResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a Group-Policy binding", &resp.Diagnostics) {
		return
	}
	var priorModel groupPolicyBindingModel
	var planModel groupPolicyBindingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &priorModel)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planModel)...)
	if resp.Diagnostics.HasError() {
		return
	}
	prior, priorErr := canonicalizeGroupPolicyBindingStateModel(priorModel)
	planned, planErr := canonicalizeGroupPolicyBindingPlanModel(planModel)
	if priorErr != nil || planErr != nil || prior != planned {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Group-Policy Binding Update",
			"A Group-Policy binding identity cannot be updated in place. Terraform state was preserved and no mutation was sent.",
		)
		return
	}
	present, err := r.readBinding(ctx, prior)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Confirm FeatBit Group-Policy Binding",
			"The provider could not confirm the unchanged exact pair. Terraform state was preserved and no mutation was sent. "+err.Error()+".",
		)
		return
	}
	if !present {
		resp.State.RemoveResource(ctx)
		return
	}
	r.setBindingState(ctx, &resp.State, &resp.Diagnostics, prior)
}

func (r *groupPolicyBindingResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a Group-Policy binding", &resp.Diagnostics) {
		return
	}
	var priorModel groupPolicyBindingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &priorModel)...)
	if resp.Diagnostics.HasError() {
		return
	}
	identity, err := canonicalizeGroupPolicyBindingStateModel(priorModel)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Group-Policy Binding Delete State",
			"The stored binding identity is incomplete or inconsistent. Terraform state was preserved and no mutation was sent.",
		)
		return
	}

	present, err := r.readBinding(ctx, identity)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Confirm FeatBit Group-Policy Binding Before Delete",
			"The provider could not read the complete exact Group Policy collection. Terraform state was preserved and no mutation was sent. "+err.Error()+".",
		)
		return
	}
	if !present {
		resp.State.RemoveResource(ctx)
		return
	}

	endpointsPresent, err := r.bindingEndpointsPresent(ctx, identity)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Validate FeatBit Group-Policy Binding Endpoints Before Delete",
			"The provider could not resolve both exact endpoint objects through their complete token-scoped collections. Terraform state was preserved and no mutation was sent. "+err.Error()+".",
		)
		return
	}
	if !endpointsPresent {
		resp.State.RemoveResource(ctx)
		return
	}

	mutationErr := r.client.RemoveGroupPolicy(ctx, identity.GroupID, identity.PolicyID)
	present, readErr := r.readBinding(ctx, identity)
	if readErr != nil {
		detail := "The remove request completed, but the complete exact Group Policy collection could not prove binding absence. Terraform state was preserved. " + readErr.Error() + "."
		if mutationErr != nil {
			detail = "The remove request failed and exact binding absence could not be confirmed. Terraform state was preserved. " + mutationErr.Error() + "."
		}
		resp.Diagnostics.AddError("Unable to Confirm FeatBit Group-Policy Binding Removal", detail)
		return
	}
	if !present {
		resp.State.RemoveResource(ctx)
		return
	}
	r.setBindingState(ctx, &resp.State, &resp.Diagnostics, identity)
	if resp.Diagnostics.HasError() {
		return
	}
	detail := "The exact Group-Policy pair still exists after the remove request. Terraform state was preserved."
	if mutationErr != nil {
		detail = "The remove request failed and the exact Group-Policy pair still exists. Terraform state was preserved. " + mutationErr.Error() + "."
	}
	resp.Diagnostics.AddError("FeatBit Group-Policy Binding Was Not Removed", detail)
}

func (r *groupPolicyBindingResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	identity, valid := canonicalizeGroupPolicyBindingImportID(req.ID)
	if !valid {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Group-Policy Binding Import Identifier",
			"Import a Group-Policy binding as <group_uuid>/<policy_uuid>, with both values in 8-4-4-4-12 hexadecimal UUID form.",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), identity.syntheticID())...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), identity.GroupID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("policy_id"), identity.PolicyID)...)
}

func (r *groupPolicyBindingResource) bindingEndpointsPresent(
	ctx context.Context,
	identity groupPolicyBindingIdentity,
) (bool, error) {
	_, groupFound, err := r.client.GetGroup(ctx, identity.GroupID)
	if err != nil || !groupFound {
		return false, err
	}
	_, policyFound, err := r.client.GetPolicy(ctx, identity.PolicyID)
	if err != nil || !policyFound {
		return false, err
	}
	return true, nil
}

func (r *groupPolicyBindingResource) readBinding(
	ctx context.Context,
	identity groupPolicyBindingIdentity,
) (bool, error) {
	policyIDs, listErr := r.client.ListGroupPolicyIDs(ctx, identity.GroupID)
	if listErr == nil {
		return slices.Contains(policyIDs, identity.PolicyID), nil
	}

	endpointsPresent, endpointErr := r.bindingEndpointsPresent(ctx, identity)
	if endpointErr == nil && !endpointsPresent {
		return false, nil
	}
	return false, listErr
}

func (r *groupPolicyBindingResource) setBindingState(
	ctx context.Context,
	state *tfsdk.State,
	diagnostics *diag.Diagnostics,
	identity groupPolicyBindingIdentity,
) bool {
	model := flattenGroupPolicyBinding(identity)
	diagnostics.Append(state.Set(ctx, &model)...)
	return !diagnostics.HasError()
}
