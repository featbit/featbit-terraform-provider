// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*groupBindingResource)(nil)
	_ resource.ResourceWithConfigure   = (*groupBindingResource)(nil)
	_ resource.ResourceWithImportState = (*groupBindingResource)(nil)
)

var errInvalidGroupBinding = errors.New("Group binding identity is invalid")

type groupBindingIdentity struct {
	GroupID  string
	TargetID string
}

func (groupBindingIdentity) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.groupBindingIdentity{redacted}")
}

func (identity groupBindingIdentity) syntheticID() string {
	return identity.GroupID + "/" + identity.TargetID
}

type groupBindingKind struct {
	typeNameSuffix    string
	pairName          string
	sourceName        string
	sourceAttribute   string
	sourceImportName  string
	sourceDescription string
	sourceSensitive   bool
	sourcePresent     func(*client.Client, context.Context, string) (bool, error)
	targetName        string
	targetAttribute   string
	targetImportName  string
	targetDescription string
	collectionName    string
	collectionOwner   string
	targetSensitive   bool
	targetPresent     func(*client.Client, context.Context, string) (bool, error)
	listIDs           func(*client.Client, context.Context, string) ([]string, error)
	add               func(*client.Client, context.Context, string, string) error
	remove            func(*client.Client, context.Context, string, string) error
}

var (
	groupPolicyBindingKind = groupBindingKind{
		typeNameSuffix:    "group_policy_binding",
		pairName:          "Group-Policy",
		sourceName:        "Group",
		sourceAttribute:   "group_id",
		sourceImportName:  "group_uuid",
		sourceDescription: "Exact Group UUID. Changing it replaces the binding.",
		sourcePresent:     groupBindingTargetPresent((*client.Client).GetGroup),
		targetName:        "Policy",
		targetAttribute:   "policy_id",
		targetImportName:  "policy_uuid",
		targetDescription: "Exact custom or built-in Policy UUID. Changing it replaces the binding.",
		collectionName:    "Group Policy",
		collectionOwner:   "the Group's complete Policy",
		targetPresent:     groupBindingTargetPresent((*client.Client).GetPolicy),
		listIDs:           (*client.Client).ListGroupPolicyIDs,
		add:               (*client.Client).AddGroupPolicy,
		remove:            (*client.Client).RemoveGroupPolicy,
	}
	groupMemberBindingKind = groupBindingKind{
		typeNameSuffix:    "group_member_binding",
		pairName:          "Group-Member",
		sourceName:        "Group",
		sourceAttribute:   "group_id",
		sourceImportName:  "group_uuid",
		sourceDescription: "Exact Group UUID. Changing it replaces the binding.",
		sourcePresent:     groupBindingTargetPresent((*client.Client).GetGroup),
		targetName:        "Member",
		targetAttribute:   "member_id",
		targetImportName:  "member_uuid",
		targetDescription: "Exact existing Member UUID. Changing it replaces the binding.",
		collectionName:    "Group Member",
		collectionOwner:   "the Group's complete Member",
		targetSensitive:   true,
		targetPresent:     groupBindingTargetPresent((*client.Client).GetMember),
		listIDs:           (*client.Client).ListGroupMemberIDs,
		add:               (*client.Client).AddGroupMember,
		remove:            (*client.Client).RemoveGroupMember,
	}
	memberPolicyBindingKind = groupBindingKind{
		typeNameSuffix:    "member_policy_binding",
		pairName:          "Member-Policy",
		sourceName:        "Member",
		sourceAttribute:   "member_id",
		sourceImportName:  "member_uuid",
		sourceDescription: "Exact existing Member UUID. Changing it replaces the binding.",
		sourceSensitive:   true,
		sourcePresent:     groupBindingTargetPresent((*client.Client).GetMember),
		targetName:        "Policy",
		targetAttribute:   "policy_id",
		targetImportName:  "policy_uuid",
		targetDescription: "Exact custom or built-in Policy UUID. Changing it replaces the binding.",
		collectionName:    "Member direct Policy",
		collectionOwner:   "the Member's complete direct Policy",
		targetPresent:     groupBindingTargetPresent((*client.Client).GetPolicy),
		listIDs:           (*client.Client).ListMemberDirectPolicyIDs,
		add:               (*client.Client).AddMemberDirectPolicy,
		remove:            (*client.Client).RemoveMemberDirectPolicy,
	}
)

type groupBindingResource struct {
	client *client.Client
	kind   groupBindingKind
}

func newGroupPolicyBindingResource() resource.Resource {
	return &groupBindingResource{kind: groupPolicyBindingKind}
}

func newGroupMemberBindingResource() resource.Resource {
	return &groupBindingResource{kind: groupMemberBindingKind}
}

func newMemberPolicyBindingResource() resource.Resource {
	return &groupBindingResource{kind: memberPolicyBindingKind}
}

func (r *groupBindingResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_" + r.kind.typeNameSuffix
}

func (r *groupBindingResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages one exact FeatBit " + r.kind.sourceName + "-to-" + r.kind.targetName + " binding through the documented public API. It owns only the configured pair, not either endpoint or " + r.kind.collectionOwner + " collection.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Sensitive:           r.kind.sourceSensitive || r.kind.targetSensitive,
				MarkdownDescription: "Canonical synthetic binding ID in `<" + r.kind.sourceImportName + ">/<" + r.kind.targetImportName + ">` form.",
				PlanModifiers: []planmodifier.String{
					useStateForUnknownIfUnchanged(
						path.Root(r.kind.sourceAttribute),
						path.Root(r.kind.targetAttribute),
					),
				},
			},
			r.kind.sourceAttribute: schema.StringAttribute{
				Required:            true,
				Sensitive:           r.kind.sourceSensitive,
				MarkdownDescription: r.kind.sourceDescription,
				Validators: []validator.String{
					uuidValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			r.kind.targetAttribute: schema.StringAttribute{
				Required:            true,
				Sensitive:           r.kind.targetSensitive,
				MarkdownDescription: r.kind.targetDescription,
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

func (r *groupBindingResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	r.client = clientFromProviderData(req.ProviderData, "Resource", &resp.Diagnostics)
}

func (r *groupBindingResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	if !requireAPIClient(r.client, "managing a "+r.kind.pairName+" binding", &resp.Diagnostics) {
		return
	}
	identity, err := r.planIdentity(ctx, req.Plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit "+r.kind.pairName+" Binding Plan",
			"The "+r.kind.sourceName+" and "+r.kind.targetName+" identifiers could not be canonicalized safely. No mutation was sent.",
		)
		return
	}

	endpointsPresent, err := r.bindingEndpointsPresent(ctx, identity)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Validate FeatBit "+r.kind.pairName+" Binding Endpoints",
			"The provider could not resolve both exact endpoint objects through their complete token-scoped collections. No mutation was sent. "+err.Error()+".",
		)
		return
	}
	if !endpointsPresent {
		resp.Diagnostics.AddError(
			"FeatBit "+r.kind.pairName+" Binding Endpoint Does Not Exist",
			"The exact "+r.kind.sourceName+" or "+r.kind.targetName+" does not exist in the token-scoped collections. No mutation was sent.",
		)
		return
	}

	present, err := r.readBinding(ctx, identity)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Check Existing FeatBit "+r.kind.pairName+" Binding",
			"The provider could not read the complete exact "+r.kind.collectionName+" collection. No mutation was sent. "+err.Error()+".",
		)
		return
	}
	if present {
		r.setBindingState(ctx, &resp.State, &resp.Diagnostics, identity)
		return
	}

	mutationErr := r.kind.add(r.client, ctx, identity.GroupID, identity.TargetID)
	present, readErr := r.readBinding(ctx, identity)
	if readErr != nil {
		detail := "The add request completed, but the complete exact " + r.kind.collectionName + " collection could not confirm the binding. Terraform did not retry or adopt an unconfirmed pair. " + readErr.Error() + "."
		if mutationErr != nil {
			detail = "The add request failed and the exact binding outcome could not be confirmed. Terraform did not retry or adopt an unconfirmed pair. " + mutationErr.Error() + "."
		}
		resp.Diagnostics.AddError("Unable to Confirm FeatBit "+r.kind.pairName+" Binding", detail)
		return
	}
	if !present {
		detail := "The exact " + r.kind.pairName + " pair is absent after the add request. Terraform did not retry or record the binding."
		if mutationErr != nil {
			detail = "The add request failed and the exact " + r.kind.pairName + " pair remains absent. Terraform did not retry or record the binding. " + mutationErr.Error() + "."
		}
		resp.Diagnostics.AddError("FeatBit "+r.kind.pairName+" Binding Was Not Created", detail)
		return
	}
	r.setBindingState(ctx, &resp.State, &resp.Diagnostics, identity)
}

func (r *groupBindingResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a "+r.kind.pairName+" binding", &resp.Diagnostics) {
		return
	}
	identity, err := r.stateIdentity(ctx, req.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit "+r.kind.pairName+" Binding State",
			"The managed binding state does not contain one consistent canonical "+r.kind.sourceName+" and "+r.kind.targetName+" pair. Terraform state was preserved.",
		)
		return
	}
	present, err := r.readBinding(ctx, identity)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit "+r.kind.pairName+" Binding",
			"The provider could not confirm the exact pair through a complete "+r.kind.collectionName+" collection. Terraform state was preserved. "+err.Error()+".",
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
func (r *groupBindingResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a "+r.kind.pairName+" binding", &resp.Diagnostics) {
		return
	}
	prior, priorErr := r.stateIdentity(ctx, req.State, &resp.Diagnostics)
	planned, planErr := r.planIdentity(ctx, req.Plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if priorErr != nil || planErr != nil || prior != planned {
		resp.Diagnostics.AddError(
			"Invalid FeatBit "+r.kind.pairName+" Binding Update",
			"A "+r.kind.pairName+" binding identity cannot be updated in place. Terraform state was preserved and no mutation was sent.",
		)
		return
	}
	present, err := r.readBinding(ctx, prior)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Confirm FeatBit "+r.kind.pairName+" Binding",
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

func (r *groupBindingResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a "+r.kind.pairName+" binding", &resp.Diagnostics) {
		return
	}
	identity, err := r.stateIdentity(ctx, req.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit "+r.kind.pairName+" Binding Delete State",
			"The stored binding identity is incomplete or inconsistent. Terraform state was preserved and no mutation was sent.",
		)
		return
	}

	present, err := r.readBinding(ctx, identity)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Confirm FeatBit "+r.kind.pairName+" Binding Before Delete",
			"The provider could not read the complete exact "+r.kind.collectionName+" collection. Terraform state was preserved and no mutation was sent. "+err.Error()+".",
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
			"Unable to Validate FeatBit "+r.kind.pairName+" Binding Endpoints Before Delete",
			"The provider could not resolve both exact endpoint objects through their complete token-scoped collections. Terraform state was preserved and no mutation was sent. "+err.Error()+".",
		)
		return
	}
	if !endpointsPresent {
		resp.State.RemoveResource(ctx)
		return
	}

	mutationErr := r.kind.remove(r.client, ctx, identity.GroupID, identity.TargetID)
	present, readErr := r.readBinding(ctx, identity)
	if readErr != nil {
		detail := "The remove request completed, but the complete exact " + r.kind.collectionName + " collection could not prove binding absence. Terraform state was preserved. " + readErr.Error() + "."
		if mutationErr != nil {
			detail = "The remove request failed and exact binding absence could not be confirmed. Terraform state was preserved. " + mutationErr.Error() + "."
		}
		resp.Diagnostics.AddError("Unable to Confirm FeatBit "+r.kind.pairName+" Binding Removal", detail)
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
	detail := "The exact " + r.kind.pairName + " pair still exists after the remove request. Terraform state was preserved."
	if mutationErr != nil {
		detail = "The remove request failed and the exact " + r.kind.pairName + " pair still exists. Terraform state was preserved. " + mutationErr.Error() + "."
	}
	resp.Diagnostics.AddError("FeatBit "+r.kind.pairName+" Binding Was Not Removed", detail)
}

func (r *groupBindingResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	identity, valid := canonicalizeGroupBindingImportID(req.ID)
	if !valid {
		resp.Diagnostics.AddError(
			"Invalid FeatBit "+r.kind.pairName+" Binding Import Identifier",
			"Import a "+r.kind.pairName+" binding as <"+r.kind.sourceImportName+">/<"+r.kind.targetImportName+">, with both values in 8-4-4-4-12 hexadecimal UUID form.",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), identity.syntheticID())...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(r.kind.sourceAttribute), identity.GroupID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(r.kind.targetAttribute), identity.TargetID)...)
}

func (r *groupBindingResource) planIdentity(
	ctx context.Context,
	plan tfsdk.Plan,
	diagnostics *diag.Diagnostics,
) (groupBindingIdentity, error) {
	var sourceID types.String
	var targetID types.String
	diagnostics.Append(plan.GetAttribute(ctx, path.Root(r.kind.sourceAttribute), &sourceID)...)
	diagnostics.Append(plan.GetAttribute(ctx, path.Root(r.kind.targetAttribute), &targetID)...)
	if diagnostics.HasError() {
		return groupBindingIdentity{}, errInvalidGroupBinding
	}
	return canonicalizeGroupBindingPlanValues(sourceID, targetID)
}

func (r *groupBindingResource) stateIdentity(
	ctx context.Context,
	state tfsdk.State,
	diagnostics *diag.Diagnostics,
) (groupBindingIdentity, error) {
	var id types.String
	var sourceID types.String
	var targetID types.String
	diagnostics.Append(state.GetAttribute(ctx, path.Root("id"), &id)...)
	diagnostics.Append(state.GetAttribute(ctx, path.Root(r.kind.sourceAttribute), &sourceID)...)
	diagnostics.Append(state.GetAttribute(ctx, path.Root(r.kind.targetAttribute), &targetID)...)
	if diagnostics.HasError() {
		return groupBindingIdentity{}, errInvalidGroupBinding
	}
	return canonicalizeGroupBindingStateValues(id, sourceID, targetID)
}

func (r *groupBindingResource) bindingEndpointsPresent(
	ctx context.Context,
	identity groupBindingIdentity,
) (bool, error) {
	sourceFound, err := r.kind.sourcePresent(r.client, ctx, identity.GroupID)
	if err != nil || !sourceFound {
		return false, err
	}
	return r.kind.targetPresent(r.client, ctx, identity.TargetID)
}

func (r *groupBindingResource) readBinding(
	ctx context.Context,
	identity groupBindingIdentity,
) (bool, error) {
	targetIDs, listErr := r.kind.listIDs(r.client, ctx, identity.GroupID)
	if listErr == nil {
		return slices.Contains(targetIDs, identity.TargetID), nil
	}

	endpointsPresent, endpointErr := r.bindingEndpointsPresent(ctx, identity)
	if endpointErr == nil && !endpointsPresent {
		return false, nil
	}
	return false, listErr
}

func (r *groupBindingResource) setBindingState(
	ctx context.Context,
	state *tfsdk.State,
	diagnostics *diag.Diagnostics,
	identity groupBindingIdentity,
) bool {
	diagnostics.Append(state.SetAttribute(ctx, path.Root("id"), identity.syntheticID())...)
	diagnostics.Append(state.SetAttribute(ctx, path.Root(r.kind.sourceAttribute), identity.GroupID)...)
	diagnostics.Append(state.SetAttribute(ctx, path.Root(r.kind.targetAttribute), identity.TargetID)...)
	return !diagnostics.HasError()
}

func canonicalizeGroupBindingPlanValues(
	groupValue types.String,
	targetValue types.String,
) (groupBindingIdentity, error) {
	if !knownString(groupValue) || !knownString(targetValue) {
		return groupBindingIdentity{}, errInvalidGroupBinding
	}
	groupID, groupValid := client.CanonicalUUID(groupValue.ValueString())
	targetID, targetValid := client.CanonicalUUID(targetValue.ValueString())
	if !groupValid || !targetValid {
		return groupBindingIdentity{}, errInvalidGroupBinding
	}
	return groupBindingIdentity{GroupID: groupID, TargetID: targetID}, nil
}

func canonicalizeGroupBindingStateValues(
	idValue types.String,
	groupValue types.String,
	targetValue types.String,
) (groupBindingIdentity, error) {
	identity, err := canonicalizeGroupBindingPlanValues(groupValue, targetValue)
	if err != nil || !knownString(idValue) {
		return groupBindingIdentity{}, errInvalidGroupBinding
	}
	stateIdentity, valid := canonicalizeGroupBindingImportID(idValue.ValueString())
	if !valid || stateIdentity != identity {
		return groupBindingIdentity{}, errInvalidGroupBinding
	}
	return identity, nil
}

func canonicalizeGroupBindingImportID(value string) (groupBindingIdentity, bool) {
	components := strings.Split(value, "/")
	if len(components) != 2 {
		return groupBindingIdentity{}, false
	}
	groupID, groupValid := client.CanonicalUUID(components[0])
	targetID, targetValid := client.CanonicalUUID(components[1])
	if !groupValid || !targetValid {
		return groupBindingIdentity{}, false
	}
	return groupBindingIdentity{GroupID: groupID, TargetID: targetID}, true
}

func groupBindingTargetPresent[T any](
	get func(*client.Client, context.Context, string) (T, bool, error),
) func(*client.Client, context.Context, string) (bool, error) {
	return func(apiClient *client.Client, ctx context.Context, targetID string) (bool, error) {
		_, found, err := get(apiClient, ctx, targetID)
		return found, err
	}
}
