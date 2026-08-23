// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

var (
	_ resource.Resource                = (*groupResource)(nil)
	_ resource.ResourceWithConfigure   = (*groupResource)(nil)
	_ resource.ResourceWithImportState = (*groupResource)(nil)
)

type groupResource struct {
	client *client.Client
}

func newGroupResource() resource.Resource {
	return &groupResource{}
}

func (r *groupResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (r *groupResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one FeatBit Group's settings through the documented public API. Group members and Policies are managed by separate binding resources.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Group UUID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Group display name.",
				Validators: []validator.String{
					nonEmptyStringValidator{object: "Group", field: "name"},
				},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Group description. An omitted value canonicalizes to an empty string.",
				Default:             stringdefault.StaticString(""),
			},
		},
	}
}

func (r *groupResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	r.client = clientFromProviderData(req.ProviderData, "Resource", &resp.Diagnostics)
}

func (r *groupResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	if !requireAPIClient(r.client, "managing a Group", &resp.Diagnostics) {
		return
	}
	var planModel groupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planModel)...)
	if resp.Diagnostics.HasError() {
		return
	}
	planned, err := canonicalizeGroupPlanModel(planModel)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Group Plan",
			"The Group name and description could not be canonicalized safely. No mutation was sent.",
		)
		return
	}

	groups, err := r.client.ListGroups(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Check Existing FeatBit Groups",
			"The provider could not complete the exact-name Group create preflight. "+err.Error()+".",
		)
		return
	}
	_, found, resolveErr := r.client.ResolveGroupByName(groups, planned.Name)
	if found || resolveErr != nil {
		detail := "A Group with the configured exact name already exists. Terraform will not adopt it automatically; import the intended Group by UUID or choose another name."
		if resolveErr != nil {
			detail = "Multiple Groups have the configured exact name, so creation is ambiguous. Resolve the duplicates before retrying."
		}
		resp.Diagnostics.AddError("FeatBit Group Create Preflight Failed", detail)
		return
	}

	created, err := r.client.CreateGroup(ctx, client.CreateGroupRequest{
		Name:        planned.Name,
		Description: planned.Description,
	})
	if err != nil {
		if mutationOutcomeAmbiguous(err) {
			r.reconcileAmbiguousGroupCreate(ctx, err, planned.Name, &resp.Diagnostics)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to Create FeatBit Group",
			"The Group create request failed without a confirmed remote identity. Terraform did not retry the mutation. "+err.Error()+".",
		)
		return
	}
	createdCanonical, err := canonicalizeRemoteGroup(created)
	if err != nil || createdCanonical.Name != planned.Name {
		resp.Diagnostics.AddError(
			"Created FeatBit Group Is Invalid",
			"The create response did not contain one safe Group with the configured exact name. Terraform did not adopt an unconfirmed object.",
		)
		return
	}
	if !r.setGroupState(ctx, &resp.State, &resp.Diagnostics, createdCanonical) {
		return
	}

	canonical, found, err := r.readGroup(ctx, createdCanonical.ID)
	if err != nil || !found {
		detail := "The Group was created, but its exact canonical state could not be confirmed. The confirmed create identity remains in Terraform state for safe recovery."
		if err != nil {
			detail += " " + err.Error() + "."
		}
		resp.Diagnostics.AddError("Unable to Confirm Created FeatBit Group", detail)
		return
	}
	r.setGroupState(ctx, &resp.State, &resp.Diagnostics, canonical)
}

func (r *groupResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a Group", &resp.Diagnostics) {
		return
	}
	var prior groupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() || !knownString(prior.ID) ||
		!client.ValidUUID(prior.ID.ValueString()) {
		if !resp.Diagnostics.HasError() {
			resp.Diagnostics.AddError(
				"Invalid FeatBit Group State",
				"The managed Group state does not contain a valid exact UUID. Terraform state was preserved.",
			)
		}
		return
	}
	group, found, err := r.readGroup(ctx, prior.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Group",
			"The provider could not confirm the exact Group through the documented public API. Terraform state has been preserved. "+err.Error()+".",
		)
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	r.setGroupState(ctx, &resp.State, &resp.Diagnostics, group)
}

func (r *groupResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a Group", &resp.Diagnostics) {
		return
	}
	var priorModel groupModel
	var planModel groupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &priorModel)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planModel)...)
	if resp.Diagnostics.HasError() {
		return
	}
	prior, priorErr := canonicalizeGroupStateModel(priorModel)
	planned, planErr := canonicalizeGroupPlanModel(planModel)
	if priorErr != nil || planErr != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Group Update",
			"The prior state and planned settings could not be correlated as one safe Group. Terraform state was preserved and no mutation was sent.",
		)
		return
	}

	mutationResponse, mutationErr := r.client.UpdateGroup(
		ctx,
		prior.ID,
		client.UpdateGroupRequest{
			Name:        planned.Name,
			Description: planned.Description,
		},
	)
	if mutationErr == nil {
		if provisional, provisionalErr := canonicalizeRemoteGroup(mutationResponse); provisionalErr == nil &&
			provisional.ID == prior.ID {
			if !r.setGroupState(ctx, &resp.State, &resp.Diagnostics, provisional) {
				return
			}
		}
	}

	canonical, found, readErr := r.readGroup(ctx, prior.ID)
	if readErr != nil || !found {
		detail := "The Group settings outcome could not be confirmed through an exact canonical read. Terraform did not retry the mutation and preserved the last confirmed state."
		if mutationErr != nil {
			detail += " " + mutationErr.Error() + "."
		} else if readErr != nil {
			detail += " " + readErr.Error() + "."
		}
		resp.Diagnostics.AddError("Unable to Confirm Updated FeatBit Group", detail)
		return
	}
	if !r.setGroupState(ctx, &resp.State, &resp.Diagnostics, canonical) {
		return
	}
	if mutationErr != nil && !mutationNeedsReconciliation(mutationErr) {
		resp.Diagnostics.AddError(
			"Unable to Update FeatBit Group",
			"The settings mutation failed without an ambiguous outcome. Terraform preserved the exact reread state and did not retry the request. "+mutationErr.Error()+".",
		)
		return
	}
	if !sameGroupSettings(canonical, planned) {
		resp.Diagnostics.AddError(
			"FeatBit Group Settings Update Is Unconfirmed",
			"The exact canonical Group does not contain the planned name and description. Terraform did not retry the mutation; the confirmed server form remains in state.",
		)
	}
}

func (r *groupResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a Group", &resp.Diagnostics) {
		return
	}
	var priorModel groupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &priorModel)...)
	if resp.Diagnostics.HasError() {
		return
	}
	prior, err := canonicalizeGroupStateModel(priorModel)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Group Delete State",
			"The stored Group definition is incomplete or unsafe. Terraform state was preserved and no mutation was sent.",
		)
		return
	}

	current, found, err := r.readGroup(ctx, prior.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Confirm FeatBit Group Before Delete",
			"The exact Group could not be confirmed before Delete. Terraform state was preserved and no association or delete mutation was sent. "+err.Error()+".",
		)
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	if !r.setGroupState(ctx, &resp.State, &resp.Diagnostics, current) {
		return
	}

	memberCount, err := r.client.CountGroupMembers(ctx, prior.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Check FeatBit Group Member Associations",
			"The provider could not read the complete exact Member association collection. Terraform state was preserved and no delete mutation was sent. "+err.Error()+".",
		)
		return
	}
	policyCount, err := r.client.CountGroupPolicies(ctx, prior.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Check FeatBit Group Policy Associations",
			"The provider could not read the complete exact Policy association collection. Terraform state was preserved and no delete mutation was sent. "+err.Error()+".",
		)
		return
	}
	if memberCount != 0 || policyCount != 0 {
		resp.Diagnostics.AddError(
			"FeatBit Group Still Has Live Associations",
			"Destroy refuses to cascade a Group that still contains one or more Members or Policies. Remove those exact bindings first; Terraform state was preserved and no delete mutation was sent.",
		)
		return
	}

	deleteErr := r.client.DeleteGroup(ctx, prior.ID)
	groups, listErr := r.client.ListGroups(ctx)
	if listErr != nil {
		detail := "The delete attempt completed, but the complete Group collection could not prove exact absence. Terraform state was preserved. " + listErr.Error() + "."
		if deleteErr != nil {
			detail = "The delete request failed and the complete Group collection could not prove exact absence. Terraform state was preserved. " + deleteErr.Error() + "."
		}
		resp.Diagnostics.AddError("Unable to Confirm FeatBit Group Deletion", detail)
		return
	}
	remaining, found, resolveErr := r.client.ResolveGroupByID(groups, prior.ID)
	if resolveErr != nil {
		resp.Diagnostics.AddError(
			"Unable to Confirm FeatBit Group Deletion",
			"The complete Group collection returned an ambiguous exact identity after Delete. Terraform state was preserved.",
		)
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	if canonical, canonicalErr := canonicalizeRemoteGroup(remaining); canonicalErr == nil {
		r.setGroupState(ctx, &resp.State, &resp.Diagnostics, canonical)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	detail := "The exact Group still exists after the delete request. Terraform state was preserved."
	if deleteErr != nil {
		detail = "The delete request failed and the exact Group still exists. Terraform state was preserved. " + deleteErr.Error() + "."
	}
	resp.Diagnostics.AddError("FeatBit Group Was Not Deleted", detail)
}

func (r *groupResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	groupID, valid := client.CanonicalUUID(req.ID)
	if !valid {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Group Import Identifier",
			"Import a Group with exactly one UUID in 8-4-4-4-12 hexadecimal form.",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), groupID)...)
}

func (r *groupResource) readGroup(
	ctx context.Context,
	groupID string,
) (client.Group, bool, error) {
	group, found, err := r.client.GetGroup(ctx, groupID)
	if err != nil || !found {
		return client.Group{}, found, err
	}
	canonical, err := canonicalizeRemoteGroup(group)
	if err != nil || !client.EqualUUID(canonical.ID, groupID) {
		return client.Group{}, false, errInvalidGroupDefinition
	}
	return canonical, true, nil
}

func (r *groupResource) setGroupState(
	ctx context.Context,
	state *tfsdk.State,
	diagnostics *diag.Diagnostics,
	group client.Group,
) bool {
	model := flattenGroup(group)
	diagnostics.Append(state.Set(ctx, &model)...)
	return !diagnostics.HasError()
}

func (r *groupResource) reconcileAmbiguousGroupCreate(
	ctx context.Context,
	createErr error,
	name string,
	diagnostics *diag.Diagnostics,
) {
	groups, listErr := r.client.ListGroups(ctx)
	if listErr != nil {
		diagnostics.AddError(
			"FeatBit Group Create Outcome Is Unconfirmed",
			"The create result was ambiguous and the complete Group collection could not be read. Terraform did not retry or adopt any object. Verify the remote system before retrying, then import the intended Group by UUID if it exists. "+createErr.Error()+".",
		)
		return
	}
	_, found, resolveErr := r.client.ResolveGroupByName(groups, name)
	switch {
	case resolveErr != nil:
		diagnostics.AddError(
			"FeatBit Group Create Outcome Is Ambiguous",
			"The create result was ambiguous and multiple Groups now have the configured exact name. Terraform did not retry or adopt any object. Resolve the duplicates before continuing.",
		)
	case found:
		diagnostics.AddError(
			"FeatBit Group Create Outcome Requires Recovery",
			"The create result was ambiguous and exactly one Group now has the configured name. Terraform did not retry or adopt it. Verify that object, then import it deliberately by UUID or remove it before retrying.",
		)
	default:
		diagnostics.AddError(
			"Unable to Create FeatBit Group",
			"The create result was ambiguous, but the complete Group collection contains no exact-name match. Terraform did not retry the mutation. "+createErr.Error()+".",
		)
	}
}
