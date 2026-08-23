// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"
	"sync"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var (
	_ resource.Resource                = (*featureFlagResource)(nil)
	_ resource.ResourceWithConfigure   = (*featureFlagResource)(nil)
	_ resource.ResourceWithImportState = (*featureFlagResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*featureFlagResource)(nil)
)

type featureFlagResource struct {
	client   *client.Client
	lockOnce sync.Once
	locks    *keyedLockManager
}

func newFeatureFlagResource() resource.Resource {
	return &featureFlagResource{}
}

func (r *featureFlagResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_feature_flag"
}

func (r *featureFlagResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = featureFlagResourceSchema()
}

func (r *featureFlagResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	r.client = clientFromProviderData(req.ProviderData, "Resource", &resp.Diagnostics)
}

func (r *featureFlagResource) ModifyPlan(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan featureFlagModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !featureFlagPlanDefinitionKnown(plan) {
		return
	}
	planned, _, err := canonicalizeFeatureFlagPlanModel(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Feature Flag Plan",
			"The complete Feature Flag plan could not be canonicalized safely.",
		)
		return
	}

	canonicalModel := flattenCanonicalFeatureFlag(planned)
	canonicalModel.EnvironmentID = plan.EnvironmentID
	canonicalModel.ID = plan.ID
	for index := range canonicalModel.Variations {
		// Required configuration values must remain byte-for-byte consistent in
		// the plan. The custom value type establishes type-aware semantic
		// equality when canonical API state is returned after Apply or Read.
		canonicalModel.Variations[index].Value = plan.Variations[index].Value
	}

	if !req.State.Raw.IsNull() {
		var prior featureFlagModel
		resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
		if resp.Diagnostics.HasError() {
			return
		}
		canonicalPrior, priorErr := canonicalizeFeatureFlagStateModel(prior)
		if priorErr == nil && sameFeatureFlagReplacementInputs(planned, canonicalPrior) {
			canonicalModel.ID = prior.ID
			for index := range canonicalModel.Variations {
				canonicalModel.Variations[index].ID = prior.Variations[index].ID
			}
		}
	}

	resp.Diagnostics.Append(resp.Plan.Set(ctx, &canonicalModel)...)
}

func (r *featureFlagResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	if !requireAPIClient(r.client, "managing a Feature Flag", &resp.Diagnostics) {
		return
	}

	var plan featureFlagModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	planned, seed, err := canonicalizeFeatureFlagPlanModel(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Feature Flag Plan",
			"The Feature Flag definition contains an unknown, missing, or invalid value. Correct the configuration before retrying.",
		)
		return
	}
	release, err := r.featureFlagLocks().acquire(
		ctx,
		featureFlagWriteLockKey(planned.EnvironmentID, planned.Key),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Acquire FeatBit Feature Flag Create Lock",
			"Feature Flag creation was canceled while waiting to serialize the exact Environment and key. No create request was sent.",
		)
		return
	}
	defer release()

	_, status, err := r.client.ResolveFeatureFlag(ctx, planned.EnvironmentID, planned.Key)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Check Existing FeatBit Feature Flags",
			"The provider could not complete both active and archived exact-key collection views for the Feature Flag create preflight. "+err.Error()+".",
		)
		return
	}
	switch status {
	case client.FeatureFlagStatusAbsent:
		// Complete active and archived views proved exact zero.
	case client.FeatureFlagStatusActive:
		resp.Diagnostics.AddError(
			"FeatBit Feature Flag Create Preflight Failed",
			"An active Feature Flag with the configured exact key already exists in the Environment. Terraform will not adopt it automatically; import the intended flag as <environment_uuid>/<exact_key> or choose another key.",
		)
		return
	case client.FeatureFlagStatusArchived:
		resp.Diagnostics.AddError(
			"FeatBit Feature Flag Archived-Key Collision",
			"An archived Feature Flag with the configured exact key already exists in the Environment. Permanently remove that archived flag outside Terraform or choose another key before retrying.",
		)
		return
	default:
		resp.Diagnostics.AddError(
			"FeatBit Feature Flag Create Preflight Is Unconfirmed",
			"The provider did not receive an authoritative exact-key status from the complete active and archived views. No create request was sent.",
		)
		return
	}

	created, err := r.client.CreateFeatureFlag(
		ctx,
		planned.EnvironmentID,
		expandFeatureFlagCreateRequest(planned, seed),
	)
	if err != nil {
		if mutationOutcomeAmbiguous(err) {
			r.reconcileAmbiguousCreate(
				ctx,
				planned.EnvironmentID,
				planned.Key,
				err,
				&resp.Diagnostics,
			)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to Create FeatBit Feature Flag",
			"The Feature Flag create request failed without a confirmed remote object. "+err.Error()+".",
		)
		return
	}

	createdID, valid := client.CanonicalUUID(created.ID)
	if !valid {
		resp.Diagnostics.AddError(
			"Created FeatBit Feature Flag Is Unconfirmed",
			"The Feature Flag create response did not contain a valid identity. Terraform did not retry or adopt an object; verify the remote Environment before retrying.",
		)
		return
	}

	// Preserve the mutation-confirmed identity before the required canonical
	// read. The definition comes from the validated request and is replaced by
	// the server form only after exact UUID-correlated confirmation.
	provisional := planned
	provisional.ID = createdID
	provisionalState := flattenCanonicalFeatureFlag(provisional)
	preserveEquivalentFeatureFlagVariationValues(&provisionalState, plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &provisionalState)...)
	if resp.Diagnostics.HasError() {
		return
	}

	remote, remoteStatus, err := r.client.GetFeatureFlag(
		ctx,
		planned.EnvironmentID,
		planned.Key,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Confirm Created FeatBit Feature Flag",
			"The Feature Flag was created, but its canonical active definition could not be confirmed. The mutation-confirmed identity remains in Terraform state for safe recovery. "+err.Error()+".",
		)
		return
	}
	if remoteStatus != client.FeatureFlagStatusActive {
		resp.Diagnostics.AddError(
			"Created FeatBit Feature Flag Is Unconfirmed",
			"The create response supplied an identity, but the exact canonical read did not return one active Feature Flag. The mutation-confirmed identity remains in Terraform state for safe recovery.",
		)
		return
	}
	if !client.EqualUUID(remote.ID, createdID) {
		resp.Diagnostics.AddError(
			"Created FeatBit Feature Flag Identity Mismatch",
			"The create response and canonical exact read returned different Feature Flag identities. Terraform preserved the mutation-confirmed identity and did not adopt the other object. Resolve the remote inconsistency before continuing.",
		)
		return
	}

	variationOrder := canonicalFeatureFlagVariationIDs(planned)
	canonicalRemote, err := canonicalizeRemoteFeatureFlag(remote, variationOrder)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid Created FeatBit Feature Flag Definition",
			"The active Feature Flag returned after Create could not be canonicalized or correlated by exact variation UUID. The mutation-confirmed identity remains in Terraform state for safe recovery.",
		)
		return
	}
	canonicalState := flattenCanonicalFeatureFlag(canonicalRemote)
	canonicalState.EnvironmentID = plan.EnvironmentID
	preserveEquivalentFeatureFlagVariationValues(&canonicalState, plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &canonicalState)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !sameFeatureFlagDefinition(planned, canonicalRemote) {
		resp.Diagnostics.AddError(
			"Created FeatBit Feature Flag Definition Mismatch",
			"The canonical active Feature Flag does not match the deterministic definition sent by Terraform. The confirmed server form remains in state; review the remote object before retrying.",
		)
	}
}

func (r *featureFlagResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a Feature Flag", &resp.Diagnostics) {
		return
	}

	var prior featureFlagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if prior.EnvironmentID.IsNull() || prior.EnvironmentID.IsUnknown() ||
		prior.Key.IsNull() || prior.Key.IsUnknown() ||
		!client.ValidUUID(prior.EnvironmentID.ValueString()) ||
		!validFeatureFlagKey(prior.Key.ValueString()) {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Feature Flag State",
			"The Feature Flag state does not contain a valid exact Environment UUID and key. State has been preserved; import the intended object again if necessary.",
		)
		return
	}

	flag, status, err := r.client.GetFeatureFlag(
		ctx,
		prior.EnvironmentID.ValueString(),
		prior.Key.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Feature Flag",
			"The provider could not confirm the Feature Flag through the documented public API. Terraform state has been preserved. "+err.Error()+".",
		)
		return
	}
	switch status {
	case client.FeatureFlagStatusAbsent:
		resp.State.RemoveResource(ctx)
		return
	case client.FeatureFlagStatusArchived:
		resp.Diagnostics.AddError(
			"Managed FeatBit Feature Flag Was Archived Outside Terraform",
			"The exact Feature Flag exists only in the archived view. Terraform preserved state and will not restore it or create a colliding replacement. Restore it outside Terraform to continue managing it, or remove it from configuration and destroy it through the archived cleanup path.",
		)
		return
	case client.FeatureFlagStatusActive:
		// Continue with canonical active state.
	default:
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Feature Flag",
			"The provider received an unconfirmed Feature Flag status. Terraform state has been preserved.",
		)
		return
	}

	variationOrder, err := featureFlagVariationOrder(prior)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Feature Flag State",
			"The stored variation identities are incomplete or invalid. Terraform state has been preserved for safe recovery.",
		)
		return
	}
	if variationOrder == nil {
		variationOrder = deterministicRemoteFeatureFlagVariationOrder(flag)
	}
	state, err := flattenFeatureFlag(flag, variationOrder)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Feature Flag Definition",
			"The active Feature Flag definition could not be canonicalized or correlated safely. Correct the remote definition before retrying; Terraform state has been preserved.",
		)
		return
	}
	state.EnvironmentID = prior.EnvironmentID
	preserveEquivalentFeatureFlagVariationValues(&state, prior)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *featureFlagResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a Feature Flag", &resp.Diagnostics) {
		return
	}

	var prior featureFlagModel
	var plan featureFlagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if prior.EnvironmentID.IsNull() || prior.EnvironmentID.IsUnknown() ||
		prior.ID.IsNull() || prior.ID.IsUnknown() ||
		prior.Key.IsNull() || prior.Key.IsUnknown() ||
		plan.Name.IsNull() || plan.Name.IsUnknown() ||
		!client.ValidUUID(prior.EnvironmentID.ValueString()) ||
		!client.ValidUUID(prior.ID.ValueString()) ||
		!validFeatureFlagKey(prior.Key.ValueString()) ||
		!validFeatureFlagName(plan.Name.ValueString()) {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Feature Flag Update State",
			"The Feature Flag update requires a valid exact Environment, key, current UUID, and planned name. Terraform state has been preserved.",
		)
		return
	}
	variationOrder, err := featureFlagVariationOrder(prior)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Feature Flag Update State",
			"The stored variation identities are incomplete or invalid. Terraform state has been preserved for safe recovery.",
		)
		return
	}

	environmentID := prior.EnvironmentID.ValueString()
	key := prior.Key.ValueString()
	release, err := r.featureFlagLocks().acquire(
		ctx,
		featureFlagWriteLockKey(environmentID, key),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Acquire FeatBit Feature Flag Update Lock",
			"The name update was canceled while waiting for another write to the exact Feature Flag. Terraform state has been preserved.",
		)
		return
	}
	defer release()

	updateErr := r.client.UpdateFeatureFlagName(
		ctx,
		environmentID,
		key,
		prior.ID.ValueString(),
		client.UpdateFeatureFlagNameRequest{Name: plan.Name.ValueString()},
	)
	if updateErr != nil && !mutationNeedsReconciliation(updateErr) {
		resp.Diagnostics.AddError(
			"Unable to Update FeatBit Feature Flag Name",
			"The specialized name update did not complete. Terraform state has been preserved. "+updateErr.Error()+".",
		)
		return
	}

	var flag client.FeatureFlag
	var status client.FeatureFlagStatus
	if updateErr == nil {
		flag, status, err = r.client.GetFeatureFlag(ctx, environmentID, key)
	} else {
		flag, status, err = r.client.ResolveFeatureFlag(ctx, environmentID, key)
	}
	if err != nil {
		detail := "The name update response succeeded, but canonical state could not be confirmed. Terraform state has been preserved. " + err.Error() + "."
		if updateErr != nil {
			detail = "The name update result was ambiguous and complete active and archived views could not confirm its outcome. Terraform did not retry the mutation and preserved state. " + updateErr.Error() + "."
		}
		resp.Diagnostics.AddError("Unable to Confirm Updated FeatBit Feature Flag", detail)
		return
	}
	if status != client.FeatureFlagStatusActive {
		detail := "The name update response succeeded, but the exact canonical read did not return one active Feature Flag. Terraform state has been preserved for recovery."
		if updateErr != nil {
			detail = "The name update result was ambiguous and complete views did not confirm one active Feature Flag with the planned name. Terraform did not retry the mutation and preserved state."
		}
		resp.Diagnostics.AddError("Updated FeatBit Feature Flag Is Unconfirmed", detail)
		return
	}
	if !client.EqualUUID(flag.ID, prior.ID.ValueString()) {
		resp.Diagnostics.AddError(
			"Updated FeatBit Feature Flag Identity Mismatch",
			"The canonical read returned a different Feature Flag UUID for the exact key. Terraform state has been preserved and the other object was not adopted.",
		)
		return
	}
	canonical, err := canonicalizeRemoteFeatureFlag(flag, variationOrder)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid Updated FeatBit Feature Flag Definition",
			"The active Feature Flag returned after Update could not be canonicalized or correlated by exact variation UUID. Terraform state has been preserved.",
		)
		return
	}
	if updateErr != nil && canonical.Name != plan.Name.ValueString() {
		resp.Diagnostics.AddError(
			"FeatBit Feature Flag Name Update Is Unconfirmed",
			"The name update result was ambiguous and the complete exact-key resolver did not observe the planned name. Terraform did not retry the mutation and preserved state.",
		)
		return
	}
	state := flattenCanonicalFeatureFlag(canonical)
	state.EnvironmentID = prior.EnvironmentID
	preserveEquivalentFeatureFlagVariationValues(&state, prior)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *featureFlagResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a Feature Flag", &resp.Diagnostics) {
		return
	}

	var prior featureFlagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if prior.EnvironmentID.IsNull() || prior.EnvironmentID.IsUnknown() ||
		prior.Key.IsNull() || prior.Key.IsUnknown() ||
		!client.ValidUUID(prior.EnvironmentID.ValueString()) ||
		!validFeatureFlagKey(prior.Key.ValueString()) {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Feature Flag Delete State",
			"The Feature Flag delete requires a valid exact Environment UUID and key. Terraform state has been preserved.",
		)
		return
	}

	environmentID := prior.EnvironmentID.ValueString()
	key := prior.Key.ValueString()
	release, err := r.featureFlagLocks().acquire(
		ctx,
		featureFlagWriteLockKey(environmentID, key),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Acquire FeatBit Feature Flag Delete Lock",
			"Feature Flag deletion was canceled while waiting for another write to the exact Environment and key. Terraform state has been preserved.",
		)
		return
	}
	defer release()

	_, status, err := r.client.ResolveFeatureFlag(ctx, environmentID, key)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Resolve FeatBit Feature Flag Before Deletion",
			"The provider could not complete both active and archived views before deletion. No mutation was sent and Terraform state has been preserved. "+err.Error()+".",
		)
		return
	}
	if status == client.FeatureFlagStatusAbsent {
		resp.State.RemoveResource(ctx)
		return
	}
	if status == client.FeatureFlagStatusActive {
		archiveErr := r.client.ArchiveFeatureFlag(ctx, environmentID, key)
		if archiveErr != nil {
			if !mutationNeedsReconciliation(archiveErr) {
				resp.Diagnostics.AddError(
					"Unable to Archive FeatBit Feature Flag",
					"The required archive request failed. Permanent deletion was not attempted and Terraform state has been preserved. "+archiveErr.Error()+".",
				)
				return
			}
			_, status, err = r.client.ResolveFeatureFlag(ctx, environmentID, key)
			if err != nil {
				resp.Diagnostics.AddError(
					"FeatBit Feature Flag Archive Outcome Is Unconfirmed",
					"The archive result was ambiguous and complete active and archived views could not confirm its outcome. Permanent deletion was not attempted; Terraform state has been preserved. "+archiveErr.Error()+".",
				)
				return
			}
			switch status {
			case client.FeatureFlagStatusAbsent:
				resp.State.RemoveResource(ctx)
				return
			case client.FeatureFlagStatusArchived:
				// The exact archived state confirms the prerequisite.
			case client.FeatureFlagStatusActive:
				resp.Diagnostics.AddError(
					"FeatBit Feature Flag Was Not Archived",
					"The archive result was ambiguous and the complete views still contain the exact active Feature Flag. Permanent deletion was not attempted; Terraform state has been preserved.",
				)
				return
			default:
				resp.Diagnostics.AddError(
					"FeatBit Feature Flag Archive Outcome Is Unconfirmed",
					"The archive result was ambiguous and no authoritative exact status was returned. Permanent deletion was not attempted; Terraform state has been preserved.",
				)
				return
			}
		}
	} else if status != client.FeatureFlagStatusArchived {
		resp.Diagnostics.AddError(
			"FeatBit Feature Flag Delete Status Is Unconfirmed",
			"The complete active and archived views did not return an authoritative delete starting state. No mutation was sent and Terraform state has been preserved.",
		)
		return
	}

	deleteErr := r.client.DeleteFeatureFlag(ctx, environmentID, key)
	if deleteErr != nil {
		if !mutationNeedsReconciliation(deleteErr) {
			resp.Diagnostics.AddError(
				"Unable to Permanently Delete FeatBit Feature Flag",
				"The permanent delete request failed. Terraform state has been preserved. "+deleteErr.Error()+".",
			)
			return
		}
		_, status, err = r.client.ResolveFeatureFlag(ctx, environmentID, key)
		if err != nil {
			resp.Diagnostics.AddError(
				"FeatBit Feature Flag Delete Outcome Is Unconfirmed",
				"The permanent delete result was ambiguous and complete active and archived views could not confirm exact absence. Terraform did not retry the mutation and preserved state. "+deleteErr.Error()+".",
			)
			return
		}
		if status == client.FeatureFlagStatusAbsent {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"FeatBit Feature Flag Was Not Permanently Deleted",
			"The permanent delete result was ambiguous and the exact Feature Flag remains in a complete active or archived view. Terraform did not retry the mutation and preserved state.",
		)
		return
	}

	_, status, err = r.client.ResolveFeatureFlag(ctx, environmentID, key)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Confirm FeatBit Feature Flag Deletion",
			"The permanent delete response succeeded, but complete active and archived views could not prove exact absence. Terraform state has been preserved. "+err.Error()+".",
		)
		return
	}
	if status != client.FeatureFlagStatusAbsent {
		resp.Diagnostics.AddError(
			"FeatBit Feature Flag Was Not Permanently Deleted",
			"The exact Feature Flag remains in a complete active or archived view after permanent deletion. Terraform state has been preserved.",
		)
		return
	}
	resp.State.RemoveResource(ctx)
}

func (r *featureFlagResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	components := strings.Split(req.ID, "/")
	if len(components) != 2 || !validUUID(components[0]) ||
		!validFeatureFlagKey(components[1]) {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Feature Flag Import Identifier",
			"Import a Feature Flag as <environment_uuid>/<exact_key>. The Environment must use 8-4-4-4-12 hexadecimal UUID form, and the key must contain 1 through 128 ASCII letters, digits, periods, underscores, or hyphens.",
		)
		return
	}
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("environment_id"), components[0])...,
	)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), components[1])...)
}

func (r *featureFlagResource) reconcileAmbiguousCreate(
	ctx context.Context,
	environmentID string,
	key string,
	createErr error,
	diagnostics *diag.Diagnostics,
) {
	_, status, resolveErr := r.client.ResolveFeatureFlag(ctx, environmentID, key)
	if resolveErr != nil {
		diagnostics.AddError(
			"FeatBit Feature Flag Create Outcome Is Unconfirmed",
			"The create result was ambiguous and complete active and archived views could not resolve the exact key. Terraform did not retry or adopt any object. Verify the remote Environment before retrying, then import the intended flag as <environment_uuid>/<exact_key> if it exists. "+createErr.Error()+".",
		)
		return
	}

	switch status {
	case client.FeatureFlagStatusAbsent:
		diagnostics.AddError(
			"Unable to Create FeatBit Feature Flag",
			"The create result was ambiguous, but complete active and archived views contain no exact-key match. Terraform did not retry the mutation. "+createErr.Error()+".",
		)
	case client.FeatureFlagStatusActive:
		diagnostics.AddError(
			"FeatBit Feature Flag Create Outcome Requires Recovery",
			"The create result was ambiguous and exactly one active Feature Flag now has the configured key. Terraform did not retry or adopt it. Verify that object, then import it as <environment_uuid>/<exact_key> or remove it before retrying.",
		)
	case client.FeatureFlagStatusArchived:
		diagnostics.AddError(
			"FeatBit Feature Flag Create Outcome Requires Archived Recovery",
			"The create result was ambiguous and exactly one archived Feature Flag now has the configured key. Terraform did not retry or adopt it. Verify and permanently remove that object, or restore and import it deliberately before continuing.",
		)
	default:
		diagnostics.AddError(
			"FeatBit Feature Flag Create Outcome Is Unconfirmed",
			"The create result was ambiguous and no authoritative exact-key status was returned. Terraform did not retry or adopt any object.",
		)
	}
}

func canonicalizeFeatureFlagPlanModel(
	plan featureFlagModel,
) (canonicalFeatureFlag, featureFlagCreateSeed, error) {
	if plan.EnvironmentID.IsNull() || plan.EnvironmentID.IsUnknown() ||
		plan.Key.IsNull() || plan.Key.IsUnknown() ||
		plan.Name.IsNull() || plan.Name.IsUnknown() ||
		plan.Description.IsNull() || plan.Description.IsUnknown() ||
		plan.VariationType.IsNull() || plan.VariationType.IsUnknown() ||
		len(plan.Variations) == 0 {
		return canonicalFeatureFlag{}, featureFlagCreateSeed{}, errInvalidFeatureFlagDefinition
	}

	variations := make([]featureFlagVariationInput, 0, len(plan.Variations))
	for _, variation := range plan.Variations {
		if variation.Name.IsNull() || variation.Name.IsUnknown() ||
			variation.Value.IsNull() || variation.Value.IsUnknown() {
			return canonicalFeatureFlag{}, featureFlagCreateSeed{}, errInvalidFeatureFlagDefinition
		}
		variations = append(variations, featureFlagVariationInput{
			Name:  variation.Name.ValueString(),
			Value: variation.Value.ValueString(),
		})
	}
	return canonicalizePlannedFeatureFlag(
		plan.EnvironmentID.ValueString(),
		plan.Key.ValueString(),
		plan.Name.ValueString(),
		plan.Description.ValueString(),
		plan.VariationType.ValueString(),
		variations,
	)
}

func featureFlagPlanDefinitionKnown(plan featureFlagModel) bool {
	if plan.EnvironmentID.IsNull() || plan.EnvironmentID.IsUnknown() ||
		plan.Key.IsNull() || plan.Key.IsUnknown() ||
		plan.Name.IsNull() || plan.Name.IsUnknown() ||
		plan.Description.IsNull() || plan.Description.IsUnknown() ||
		plan.VariationType.IsNull() || plan.VariationType.IsUnknown() ||
		len(plan.Variations) == 0 {
		return false
	}
	for _, variation := range plan.Variations {
		if variation.Name.IsNull() || variation.Name.IsUnknown() ||
			variation.Value.IsNull() || variation.Value.IsUnknown() {
			return false
		}
	}
	return true
}

func canonicalizeFeatureFlagStateModel(state featureFlagModel) (canonicalFeatureFlag, error) {
	if state.EnvironmentID.IsNull() || state.EnvironmentID.IsUnknown() ||
		state.ID.IsNull() || state.ID.IsUnknown() ||
		state.Key.IsNull() || state.Key.IsUnknown() ||
		state.Name.IsNull() || state.Name.IsUnknown() ||
		state.Description.IsNull() || state.Description.IsUnknown() ||
		state.VariationType.IsNull() || state.VariationType.IsUnknown() ||
		len(state.Variations) == 0 {
		return canonicalFeatureFlag{}, errInvalidFeatureFlagDefinition
	}
	order, err := featureFlagVariationOrder(state)
	if err != nil {
		return canonicalFeatureFlag{}, err
	}
	flag := client.FeatureFlag{
		EnvironmentID: state.EnvironmentID.ValueString(),
		ID:            state.ID.ValueString(),
		Name:          state.Name.ValueString(),
		Description:   state.Description.ValueString(),
		Key:           state.Key.ValueString(),
		VariationType: state.VariationType.ValueString(),
		Variations:    make([]client.FeatureFlagVariation, 0, len(state.Variations)),
	}
	for _, variation := range state.Variations {
		if variation.Name.IsNull() || variation.Name.IsUnknown() ||
			variation.Value.IsNull() || variation.Value.IsUnknown() {
			return canonicalFeatureFlag{}, errInvalidFeatureFlagDefinition
		}
		flag.Variations = append(flag.Variations, client.FeatureFlagVariation{
			ID:    variation.ID.ValueString(),
			Name:  variation.Name.ValueString(),
			Value: variation.Value.ValueString(),
		})
	}
	return canonicalizeRemoteFeatureFlag(flag, order)
}

func sameFeatureFlagReplacementInputs(left canonicalFeatureFlag, right canonicalFeatureFlag) bool {
	if !client.EqualUUID(left.EnvironmentID, right.EnvironmentID) ||
		left.Description != right.Description || left.Key != right.Key ||
		left.VariationType != right.VariationType ||
		len(left.Variations) != len(right.Variations) {
		return false
	}
	for index := range left.Variations {
		if left.Variations[index].Name != right.Variations[index].Name ||
			left.Variations[index].Value != right.Variations[index].Value {
			return false
		}
	}
	return true
}

func expandFeatureFlagCreateRequest(
	flag canonicalFeatureFlag,
	seed featureFlagCreateSeed,
) client.CreateFeatureFlagRequest {
	request := client.CreateFeatureFlagRequest{
		Name:                flag.Name,
		Key:                 flag.Key,
		IsEnabled:           seed.IsEnabled,
		Description:         flag.Description,
		VariationType:       flag.VariationType,
		Variations:          make([]client.FeatureFlagVariation, 0, len(flag.Variations)),
		EnabledVariationID:  seed.EnabledVariationID,
		DisabledVariationID: seed.DisabledVariationID,
		Tags:                make([]string, len(seed.Tags)),
	}
	copy(request.Tags, seed.Tags)
	for _, variation := range flag.Variations {
		request.Variations = append(request.Variations, client.FeatureFlagVariation{
			ID:    variation.ID,
			Name:  variation.Name,
			Value: variation.Value,
		})
	}
	return request
}

func featureFlagVariationOrder(state featureFlagModel) ([]string, error) {
	if len(state.Variations) == 0 {
		return nil, nil
	}
	order := make([]string, 0, len(state.Variations))
	seen := make(map[string]struct{}, len(state.Variations))
	for _, variation := range state.Variations {
		if variation.ID.IsNull() || variation.ID.IsUnknown() {
			return nil, errInvalidFeatureFlagDefinition
		}
		id, valid := client.CanonicalUUID(variation.ID.ValueString())
		if !valid {
			return nil, errInvalidFeatureFlagDefinition
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, errInvalidFeatureFlagDefinition
		}
		seen[id] = struct{}{}
		order = append(order, id)
	}
	return order, nil
}

func canonicalFeatureFlagVariationIDs(flag canonicalFeatureFlag) []string {
	ids := make([]string, 0, len(flag.Variations))
	for _, variation := range flag.Variations {
		ids = append(ids, variation.ID)
	}
	return ids
}

// deterministicRemoteFeatureFlagVariationOrder recovers the configured order
// on the first Read after importing a Feature Flag created by this provider.
// Other imported flags retain the stable server-UUID ordering used when no
// prior Terraform state is available.
func deterministicRemoteFeatureFlagVariationOrder(flag client.FeatureFlag) []string {
	if len(flag.Variations) == 0 || !validFeatureFlagKey(flag.Key) {
		return nil
	}
	remoteIDs := make(map[string]struct{}, len(flag.Variations))
	for _, variation := range flag.Variations {
		id, valid := client.CanonicalUUID(variation.ID)
		if !valid {
			return nil
		}
		if _, duplicate := remoteIDs[id]; duplicate {
			return nil
		}
		remoteIDs[id] = struct{}{}
	}

	order := make([]string, 0, len(flag.Variations))
	for index := range flag.Variations {
		expectedID := deterministicFeatureFlagVariationID(flag.EnvironmentID, flag.Key, index)
		if expectedID == "" {
			return nil
		}
		if _, exists := remoteIDs[expectedID]; !exists {
			return nil
		}
		order = append(order, expectedID)
	}
	return order
}

func sameFeatureFlagDefinition(left canonicalFeatureFlag, right canonicalFeatureFlag) bool {
	if !client.EqualUUID(left.EnvironmentID, right.EnvironmentID) ||
		left.Name != right.Name || left.Description != right.Description ||
		left.Key != right.Key || left.VariationType != right.VariationType ||
		len(left.Variations) != len(right.Variations) {
		return false
	}
	for index := range left.Variations {
		leftVariation := left.Variations[index]
		rightVariation := right.Variations[index]
		if !client.EqualUUID(leftVariation.ID, rightVariation.ID) ||
			leftVariation.Name != rightVariation.Name ||
			leftVariation.Value != rightVariation.Value {
			return false
		}
	}
	return true
}

func preserveEquivalentFeatureFlagVariationValues(
	canonicalState *featureFlagModel,
	prior featureFlagModel,
) {
	if canonicalState == nil || prior.VariationType.IsNull() ||
		prior.VariationType.IsUnknown() || canonicalState.VariationType.IsNull() ||
		canonicalState.VariationType.IsUnknown() ||
		prior.VariationType.ValueString() != canonicalState.VariationType.ValueString() ||
		len(prior.Variations) != len(canonicalState.Variations) {
		return
	}
	variationType := canonicalState.VariationType.ValueString()
	for index := range canonicalState.Variations {
		priorVariation := prior.Variations[index]
		canonicalVariation := canonicalState.Variations[index]
		if priorVariation.Name.IsNull() || priorVariation.Name.IsUnknown() ||
			priorVariation.Value.IsNull() || priorVariation.Value.IsUnknown() ||
			canonicalVariation.Name.IsNull() || canonicalVariation.Name.IsUnknown() ||
			canonicalVariation.Value.IsNull() || canonicalVariation.Value.IsUnknown() ||
			priorVariation.Name.ValueString() != canonicalVariation.Name.ValueString() {
			continue
		}
		priorValue, priorErr := canonicalizeFeatureFlagValue(
			variationType,
			priorVariation.Value.ValueString(),
		)
		canonicalValue, canonicalErr := canonicalizeFeatureFlagValue(
			variationType,
			canonicalVariation.Value.ValueString(),
		)
		if priorErr == nil && canonicalErr == nil && priorValue == canonicalValue {
			canonicalState.Variations[index].Value = priorVariation.Value
		}
	}
}

func (r *featureFlagResource) featureFlagLocks() *keyedLockManager {
	r.lockOnce.Do(func() {
		if r.locks == nil {
			r.locks = newKeyedLockManager()
		}
	})
	return r.locks
}

func featureFlagWriteLockKey(environmentID string, key string) string {
	canonicalEnvironmentID, valid := client.CanonicalUUID(environmentID)
	if !valid {
		canonicalEnvironmentID = environmentID
	}
	return canonicalEnvironmentID + "\x00" + key
}
