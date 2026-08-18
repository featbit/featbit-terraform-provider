// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"

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
	_ resource.Resource                = (*memberDirectPoliciesResource)(nil)
	_ resource.ResourceWithConfigure   = (*memberDirectPoliciesResource)(nil)
	_ resource.ResourceWithImportState = (*memberDirectPoliciesResource)(nil)
)

var errInvalidMemberDirectPolicies = errors.New("member direct-Policy definition is invalid")

type memberDirectPoliciesModel struct {
	ID        types.String `tfsdk:"id"`
	MemberID  types.String `tfsdk:"member_id"`
	PolicyIDs types.Set    `tfsdk:"policy_ids"`
}

func (memberDirectPoliciesModel) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.memberDirectPoliciesModel{redacted}")
}

type canonicalMemberDirectPolicies struct {
	MemberID  string
	PolicyIDs []string
}

func (canonicalMemberDirectPolicies) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.canonicalMemberDirectPolicies{redacted}")
}

type memberDirectPoliciesResource struct {
	client   *client.Client
	lockOnce sync.Once
	locks    *keyedLockManager
}

func newMemberDirectPoliciesResource() resource.Resource {
	return &memberDirectPoliciesResource{}
}

func (r *memberDirectPoliciesResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_member_direct_policies"
}

func (r *memberDirectPoliciesResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Authoritatively manages one existing FeatBit Member's complete direct Policy set through the documented public API. It never manages inherited Group Policies, Group bindings, or the Member lifecycle.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Canonical Member UUID, equal to `member_id`.",
				PlanModifiers: []planmodifier.String{
					useStateForUnknownIfUnchanged(path.Root("member_id")),
				},
			},
			"member_id": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				MarkdownDescription: "Exact existing Member UUID. Changing it replaces this authoritative direct-Policy set.",
				Validators: []validator.String{
					uuidValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"policy_ids": schema.SetAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Complete unordered set of direct custom or built-in Policy UUIDs. An empty set removes every direct Policy without changing inherited Policies.",
				Validators: []validator.Set{
					memberDirectPolicyIDsValidator{},
				},
			},
		},
	}
}

func (r *memberDirectPoliciesResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	r.client = clientFromProviderData(req.ProviderData, "Resource", &resp.Diagnostics)
}

func (r *memberDirectPoliciesResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	if !requireAPIClient(r.client, "managing Member direct Policies", &resp.Diagnostics) {
		return
	}
	var planModel memberDirectPoliciesModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planModel)...)
	if resp.Diagnostics.HasError() {
		return
	}
	planned, err := canonicalizeMemberDirectPoliciesPlan(ctx, planModel)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Member Direct Policies Plan",
			"The Member UUID and complete direct Policy UUID set could not be canonicalized safely. No mutation was sent.",
		)
		return
	}

	release, err := r.memberDirectPolicyLocks().acquire(
		ctx,
		memberDirectPoliciesWriteLockKey(planned.MemberID),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Acquire FeatBit Member Direct Policies Create Lock",
			"Creation was canceled while waiting to serialize writes for the exact Member. No mutation was sent.",
		)
		return
	}
	defer release()

	if !r.validateMemberAndDesiredPolicies(
		ctx,
		planned,
		"Create",
		"No mutation was sent.",
		&resp.Diagnostics,
	) {
		return
	}
	current, err := r.client.ListMemberDirectPolicyIDs(ctx, planned.MemberID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read Existing FeatBit Member Direct Policies",
			"The provider could not read the complete direct Policy collection before Create. No mutation was sent. "+err.Error()+".",
		)
		return
	}
	if !r.setDirectPoliciesState(
		ctx,
		&resp.State,
		&resp.Diagnostics,
		planned.MemberID,
		current,
	) {
		return
	}
	r.reconcileDirectPolicies(
		ctx,
		&resp.State,
		&resp.Diagnostics,
		planned.MemberID,
		current,
		planned.PolicyIDs,
	)
}

func (r *memberDirectPoliciesResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing Member direct Policies", &resp.Diagnostics) {
		return
	}
	identity, err := memberDirectPoliciesStateIdentity(ctx, req.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Member Direct Policies State",
			"The managed state does not contain one consistent canonical Member UUID. Terraform state was preserved.",
		)
		return
	}

	_, found, err := r.client.GetMember(ctx, identity)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Member Direct Policies",
			"The provider could not confirm the exact Member through the complete token-scoped collection. Terraform state was preserved. "+err.Error()+".",
		)
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	current, err := r.client.ListMemberDirectPolicyIDs(ctx, identity)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Member Direct Policies",
			"The provider could not read the exact Member's complete direct Policy collection. Terraform state was preserved. "+err.Error()+".",
		)
		return
	}
	r.setDirectPoliciesState(ctx, &resp.State, &resp.Diagnostics, identity, current)
}

func (r *memberDirectPoliciesResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing Member direct Policies", &resp.Diagnostics) {
		return
	}
	var priorModel memberDirectPoliciesModel
	var planModel memberDirectPoliciesModel
	resp.Diagnostics.Append(req.State.Get(ctx, &priorModel)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planModel)...)
	if resp.Diagnostics.HasError() {
		return
	}
	prior, priorErr := canonicalizeMemberDirectPoliciesState(ctx, priorModel)
	planned, planErr := canonicalizeMemberDirectPoliciesPlan(ctx, planModel)
	if priorErr != nil || planErr != nil || prior.MemberID != planned.MemberID {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Member Direct Policies Update",
			"The complete prior state and plan could not be correlated to one unchanged existing Member. Terraform state was preserved and no mutation was sent.",
		)
		return
	}

	release, err := r.memberDirectPolicyLocks().acquire(
		ctx,
		memberDirectPoliciesWriteLockKey(prior.MemberID),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Acquire FeatBit Member Direct Policies Update Lock",
			"Update was canceled while waiting to serialize writes for the exact Member. Terraform state was preserved and no mutation was sent.",
		)
		return
	}
	defer release()

	_, memberFound, err := r.client.GetMember(ctx, prior.MemberID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Validate FeatBit Member Before Direct Policy Update",
			"The provider could not resolve the exact Member through the complete token-scoped collection. Terraform state was preserved and no mutation was sent. "+err.Error()+".",
		)
		return
	}
	if !memberFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if !r.validateDesiredPolicies(
		ctx,
		planned.PolicyIDs,
		"Update",
		"Terraform state was preserved and no mutation was sent.",
		&resp.Diagnostics,
	) {
		return
	}
	current, err := r.client.ListMemberDirectPolicyIDs(ctx, prior.MemberID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Member Direct Policies Before Update",
			"The provider could not read the complete direct Policy collection. Terraform state was preserved and no mutation was sent. "+err.Error()+".",
		)
		return
	}
	if !r.setDirectPoliciesState(
		ctx,
		&resp.State,
		&resp.Diagnostics,
		prior.MemberID,
		current,
	) {
		return
	}
	r.reconcileDirectPolicies(
		ctx,
		&resp.State,
		&resp.Diagnostics,
		prior.MemberID,
		current,
		planned.PolicyIDs,
	)
}

func (r *memberDirectPoliciesResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing Member direct Policies", &resp.Diagnostics) {
		return
	}
	memberID, err := memberDirectPoliciesStateIdentity(ctx, req.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Member Direct Policies Delete State",
			"The stored direct-Policy owner does not contain one consistent canonical Member UUID. Terraform state was preserved and no mutation was sent.",
		)
		return
	}

	release, err := r.memberDirectPolicyLocks().acquire(
		ctx,
		memberDirectPoliciesWriteLockKey(memberID),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Acquire FeatBit Member Direct Policies Delete Lock",
			"Deletion was canceled while waiting to serialize writes for the exact Member. Terraform state was preserved and no mutation was sent.",
		)
		return
	}
	defer release()

	_, memberFound, err := r.client.GetMember(ctx, memberID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Validate FeatBit Member Before Direct Policy Delete",
			"The provider could not resolve the exact Member through the complete token-scoped collection. Terraform state was preserved and no mutation was sent. "+err.Error()+".",
		)
		return
	}
	if !memberFound {
		resp.State.RemoveResource(ctx)
		return
	}
	current, err := r.client.ListMemberDirectPolicyIDs(ctx, memberID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Member Direct Policies Before Delete",
			"The provider could not read the complete direct Policy collection. Terraform state was preserved and no mutation was sent. "+err.Error()+".",
		)
		return
	}
	if !r.setDirectPoliciesState(
		ctx,
		&resp.State,
		&resp.Diagnostics,
		memberID,
		current,
	) {
		return
	}
	if r.reconcileDirectPolicies(
		ctx,
		&resp.State,
		&resp.Diagnostics,
		memberID,
		current,
		nil,
	) {
		resp.State.RemoveResource(ctx)
	}
}

func (r *memberDirectPoliciesResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	memberID, valid := client.CanonicalUUID(req.ID)
	if !valid {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Member Direct Policies Import Identifier",
			"Import an authoritative Member direct-Policy set with exactly one Member UUID in 8-4-4-4-12 hexadecimal form.",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), memberID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("member_id"), memberID)...)
}

func (r *memberDirectPoliciesResource) validateMemberAndDesiredPolicies(
	ctx context.Context,
	desired canonicalMemberDirectPolicies,
	operation string,
	failureSuffix string,
	diagnostics *diag.Diagnostics,
) bool {
	_, found, err := r.client.GetMember(ctx, desired.MemberID)
	if err != nil {
		diagnostics.AddError(
			"Unable to Validate FeatBit Member Before Direct Policy "+operation,
			"The provider could not resolve the exact Member through the complete token-scoped collection. "+failureSuffix+" "+err.Error()+".",
		)
		return false
	}
	if !found {
		diagnostics.AddError(
			"FeatBit Member Does Not Exist",
			"The exact existing Member is absent from the complete token-scoped Member collection. "+failureSuffix,
		)
		return false
	}
	return r.validateDesiredPolicies(
		ctx,
		desired.PolicyIDs,
		operation,
		failureSuffix,
		diagnostics,
	)
}

func (r *memberDirectPoliciesResource) validateDesiredPolicies(
	ctx context.Context,
	desiredPolicyIDs []string,
	operation string,
	failureSuffix string,
	diagnostics *diag.Diagnostics,
) bool {
	if len(desiredPolicyIDs) == 0 {
		return true
	}
	policies, err := r.client.ListPolicies(ctx)
	if err != nil {
		diagnostics.AddError(
			"Unable to Validate FeatBit Policies Before Direct Policy "+operation,
			"The provider could not read the complete token-scoped Policy collection. "+failureSuffix+" "+err.Error()+".",
		)
		return false
	}
	for _, policyID := range desiredPolicyIDs {
		_, found, err := r.client.ResolvePolicyByID(policies, policyID)
		if err != nil {
			diagnostics.AddError(
				"Unable to Validate FeatBit Policies Before Direct Policy "+operation,
				"The complete Policy collection contains an ambiguous exact identity. "+failureSuffix,
			)
			return false
		}
		if !found {
			diagnostics.AddError(
				"FeatBit Direct Policy Target Does Not Exist",
				"At least one configured direct Policy UUID is absent from the complete token-scoped Policy collection. "+failureSuffix,
			)
			return false
		}
	}
	return true
}

func (r *memberDirectPoliciesResource) reconcileDirectPolicies(
	ctx context.Context,
	state *tfsdk.State,
	diagnostics *diag.Diagnostics,
	memberID string,
	current []string,
	desired []string,
) bool {
	missing := memberDirectPolicyDifference(desired, current)
	for _, policyID := range missing {
		mutationErr := r.client.AddMemberDirectPolicy(ctx, memberID, policyID)
		confirmed, readErr := r.client.ListMemberDirectPolicyIDs(ctx, memberID)
		if readErr != nil {
			detail := "The add request completed, but the complete direct Policy collection could not confirm its outcome. Terraform did not retry the mutation and preserved the last confirmed set. " + readErr.Error() + "."
			if mutationErr != nil {
				detail = "The add request failed and its outcome could not be confirmed through the complete direct Policy collection. Terraform did not retry the mutation and preserved the last confirmed set. " + mutationErr.Error() + "."
			}
			diagnostics.AddError(
				"Unable to Confirm FeatBit Member Direct Policy Addition",
				detail,
			)
			return false
		}
		if !r.setDirectPoliciesState(ctx, state, diagnostics, memberID, confirmed) {
			return false
		}
		current = confirmed
		if !slices.Contains(current, policyID) {
			detail := "The exact Policy remains absent after the add request. Terraform did not retry the mutation; the confirmed direct Policy set remains in state."
			if mutationErr != nil {
				detail = "The add request failed and the exact Policy remains absent. Terraform did not retry the mutation; the confirmed direct Policy set remains in state. " + mutationErr.Error() + "."
			}
			diagnostics.AddError(
				"FeatBit Member Direct Policy Was Not Added",
				detail,
			)
			return false
		}
	}

	if len(memberDirectPolicyDifference(desired, current)) != 0 {
		diagnostics.AddError(
			"FeatBit Member Direct Policy Additions Are Unconfirmed",
			"The latest complete direct Policy collection no longer contains every desired Policy. Terraform preserved the confirmed set and sent no remove mutations.",
		)
		return false
	}

	extra := memberDirectPolicyDifference(current, desired)
	for _, policyID := range extra {
		mutationErr := r.client.RemoveMemberDirectPolicy(ctx, memberID, policyID)
		confirmed, readErr := r.client.ListMemberDirectPolicyIDs(ctx, memberID)
		if readErr != nil {
			detail := "The remove request completed, but the complete direct Policy collection could not confirm its outcome. Terraform did not retry the mutation and preserved the last confirmed set. " + readErr.Error() + "."
			if mutationErr != nil {
				detail = "The remove request failed and its outcome could not be confirmed through the complete direct Policy collection. Terraform did not retry the mutation and preserved the last confirmed set. " + mutationErr.Error() + "."
			}
			diagnostics.AddError(
				"Unable to Confirm FeatBit Member Direct Policy Removal",
				detail,
			)
			return false
		}
		if !r.setDirectPoliciesState(ctx, state, diagnostics, memberID, confirmed) {
			return false
		}
		current = confirmed
		if slices.Contains(current, policyID) {
			detail := "The exact Policy still exists after the remove request. Terraform did not retry the mutation; the confirmed direct Policy set remains in state."
			if mutationErr != nil {
				detail = "The remove request failed and the exact Policy still exists. Terraform did not retry the mutation; the confirmed direct Policy set remains in state. " + mutationErr.Error() + "."
			}
			diagnostics.AddError(
				"FeatBit Member Direct Policy Was Not Removed",
				detail,
			)
			return false
		}
		if len(memberDirectPolicyDifference(desired, current)) != 0 {
			diagnostics.AddError(
				"FeatBit Member Direct Policy Set Changed Concurrently",
				"The latest complete direct Policy collection no longer contains every desired Policy. Terraform stopped removing Policies and preserved the confirmed set.",
			)
			return false
		}
	}

	final, err := r.client.ListMemberDirectPolicyIDs(ctx, memberID)
	if err != nil {
		diagnostics.AddError(
			"Unable to Confirm FeatBit Member Direct Policy Set",
			"The deterministic direct Policy mutations completed, but the final complete set could not be reread. Terraform preserved the last mutation-confirmed set. "+err.Error()+".",
		)
		return false
	}
	if !r.setDirectPoliciesState(ctx, state, diagnostics, memberID, final) {
		return false
	}
	if !slices.Equal(final, desired) {
		diagnostics.AddError(
			"FeatBit Member Direct Policy Set Is Unconfirmed",
			"The final complete direct Policy set does not match the configured authoritative set. Terraform preserved the exact server form and did not retry any mutation.",
		)
		return false
	}
	return true
}

func (r *memberDirectPoliciesResource) setDirectPoliciesState(
	ctx context.Context,
	state *tfsdk.State,
	diagnostics *diag.Diagnostics,
	memberID string,
	policyIDs []string,
) bool {
	model := memberDirectPoliciesModel{
		ID:        types.StringValue(memberID),
		MemberID:  types.StringValue(memberID),
		PolicyIDs: terraformStringSetValue(policyIDs),
	}
	diagnostics.Append(state.Set(ctx, &model)...)
	return !diagnostics.HasError()
}

func (r *memberDirectPoliciesResource) memberDirectPolicyLocks() *keyedLockManager {
	r.lockOnce.Do(func() {
		if r.locks == nil {
			r.locks = newKeyedLockManager()
		}
	})
	return r.locks
}

func memberDirectPoliciesWriteLockKey(memberID string) string {
	canonicalMemberID, valid := client.CanonicalUUID(memberID)
	if !valid {
		canonicalMemberID = memberID
	}
	return "member-direct-policies\x00" + canonicalMemberID
}

func canonicalizeMemberDirectPoliciesPlan(
	ctx context.Context,
	model memberDirectPoliciesModel,
) (canonicalMemberDirectPolicies, error) {
	if !knownString(model.MemberID) || model.PolicyIDs.IsNull() || model.PolicyIDs.IsUnknown() {
		return canonicalMemberDirectPolicies{}, errInvalidMemberDirectPolicies
	}
	memberID, valid := client.CanonicalUUID(model.MemberID.ValueString())
	if !valid {
		return canonicalMemberDirectPolicies{}, errInvalidMemberDirectPolicies
	}
	policyIDs, err := memberDirectPolicyIDsFromTerraform(ctx, model.PolicyIDs)
	if err != nil {
		return canonicalMemberDirectPolicies{}, errInvalidMemberDirectPolicies
	}
	return canonicalMemberDirectPolicies{MemberID: memberID, PolicyIDs: policyIDs}, nil
}

func canonicalizeMemberDirectPoliciesState(
	ctx context.Context,
	model memberDirectPoliciesModel,
) (canonicalMemberDirectPolicies, error) {
	if !knownString(model.ID) {
		return canonicalMemberDirectPolicies{}, errInvalidMemberDirectPolicies
	}
	canonical, err := canonicalizeMemberDirectPoliciesPlan(ctx, model)
	if err != nil {
		return canonicalMemberDirectPolicies{}, errInvalidMemberDirectPolicies
	}
	stateID, valid := client.CanonicalUUID(model.ID.ValueString())
	if !valid || stateID != canonical.MemberID {
		return canonicalMemberDirectPolicies{}, errInvalidMemberDirectPolicies
	}
	return canonical, nil
}

func memberDirectPoliciesStateIdentity(
	ctx context.Context,
	state tfsdk.State,
	diagnostics *diag.Diagnostics,
) (string, error) {
	var id types.String
	var memberID types.String
	diagnostics.Append(state.GetAttribute(ctx, path.Root("id"), &id)...)
	diagnostics.Append(state.GetAttribute(ctx, path.Root("member_id"), &memberID)...)
	if diagnostics.HasError() || !knownString(id) || !knownString(memberID) {
		return "", errInvalidMemberDirectPolicies
	}
	canonicalID, idValid := client.CanonicalUUID(id.ValueString())
	canonicalMemberID, memberValid := client.CanonicalUUID(memberID.ValueString())
	if !idValid || !memberValid || canonicalID != canonicalMemberID {
		return "", errInvalidMemberDirectPolicies
	}
	return canonicalMemberID, nil
}

func memberDirectPolicyIDsFromTerraform(
	ctx context.Context,
	value types.Set,
) ([]string, error) {
	var values []string
	if diagnostics := value.ElementsAs(ctx, &values, false); diagnostics.HasError() {
		return nil, errInvalidMemberDirectPolicies
	}
	return canonicalizeMemberPolicyUUIDs(values)
}

func canonicalizeMemberPolicyUUIDs(values []string) ([]string, error) {
	canonical := make([]string, 0, len(values))
	for _, value := range values {
		policyID, valid := client.CanonicalUUID(value)
		if !valid {
			return nil, errInvalidMemberDirectPolicies
		}
		canonical = append(canonical, policyID)
	}
	slices.Sort(canonical)
	return slices.Compact(canonical), nil
}

func memberDirectPolicyDifference(left []string, right []string) []string {
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range right {
		rightSet[value] = struct{}{}
	}
	difference := make([]string, 0)
	for _, value := range left {
		if _, exists := rightSet[value]; !exists {
			difference = append(difference, value)
		}
	}
	return difference
}

type memberDirectPolicyIDsValidator struct{}

var _ validator.Set = memberDirectPolicyIDsValidator{}

func (memberDirectPolicyIDsValidator) Description(context.Context) string {
	return "must contain only UUIDs in 8-4-4-4-12 hexadecimal form"
}

func (v memberDirectPolicyIDsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (memberDirectPolicyIDsValidator) ValidateSet(
	ctx context.Context,
	req validator.SetRequest,
	resp *validator.SetResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	var values []string
	if diagnostics := req.ConfigValue.ElementsAs(ctx, &values, false); diagnostics.HasError() {
		return
	}
	for _, value := range values {
		if !client.ValidUUID(value) {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Invalid FeatBit Direct Policy UUID Set",
				"Every policy_ids value must be a UUID in 8-4-4-4-12 hexadecimal form.",
			)
			return
		}
	}
}
