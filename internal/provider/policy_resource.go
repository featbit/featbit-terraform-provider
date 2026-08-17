// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"sync"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

var (
	_ resource.Resource                = (*policyResource)(nil)
	_ resource.ResourceWithConfigure   = (*policyResource)(nil)
	_ resource.ResourceWithImportState = (*policyResource)(nil)
)

var (
	errManagedPolicyBuiltIn  = errors.New("Policy is built-in")
	errManagedPolicyIdentity = errors.New("Policy identity is inconsistent")
)

type policyResource struct {
	client   *client.Client
	lockOnce sync.Once
	locks    *keyedLockManager
}

func newPolicyResource() resource.Resource {
	return &policyResource{}
}

func (r *policyResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_policy"
}

func (r *policyResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = policyResourceSchema()
}

func (r *policyResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	r.client = clientFromProviderData(req.ProviderData, "Resource", &resp.Diagnostics)
}

func (r *policyResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	if !requireAPIClient(r.client, "managing a Policy", &resp.Diagnostics) {
		return
	}
	var planModel policyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planModel)...)
	if resp.Diagnostics.HasError() {
		return
	}
	planned, err := canonicalizePolicyPlanModel(ctx, planModel)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Policy Plan",
			"The complete custom Policy settings and statement set could not be canonicalized safely. No mutation was sent.",
		)
		return
	}

	release, err := r.policyLocks().acquire(ctx, policyWriteLockKey(planned.Key))
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Acquire FeatBit Policy Create Lock",
			"Creation was canceled while waiting to serialize the exact Policy lifecycle. No mutation was sent.",
		)
		return
	}
	defer release()

	policies, err := r.client.ListPolicies(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Check Existing FeatBit Policies",
			"The provider could not complete the exact-key Policy create preflight. "+err.Error()+".",
		)
		return
	}
	_, found, resolveErr := r.client.ResolvePolicyByKey(policies, planned.Key)
	if found || resolveErr != nil {
		detail := "A Policy with the configured exact key already exists. Terraform will not adopt it automatically; import an intended custom Policy by UUID or choose another key."
		if resolveErr != nil {
			detail = "Multiple Policies have the configured exact key, so creation is ambiguous. Resolve the duplicates before retrying."
		}
		resp.Diagnostics.AddError("FeatBit Policy Create Preflight Failed", detail)
		return
	}

	created, err := r.client.CreatePolicy(ctx, client.CreatePolicyRequest{
		Name:        planned.Name,
		Key:         planned.Key,
		Description: planned.Description,
	})
	if err != nil {
		if mutationOutcomeAmbiguous(err) {
			r.reconcileAmbiguousPolicyCreate(ctx, err, planned.Key, &resp.Diagnostics)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to Create FeatBit Policy",
			"The Policy settings create request failed without a confirmed remote identity. Terraform did not retry the mutation. "+err.Error()+".",
		)
		return
	}
	createdCanonical, err := canonicalizeRemoteManagedPolicy(created)
	if err != nil || createdCanonical.Key != planned.Key {
		resp.Diagnostics.AddError(
			"Created FeatBit Policy Is Invalid",
			"The create response did not contain one safe custom Policy with the configured exact key. Terraform did not adopt an unconfirmed object.",
		)
		return
	}
	if !r.setPolicyState(ctx, &resp.State, &resp.Diagnostics, createdCanonical, &planModel) {
		return
	}

	confirmed, found, err := r.readManagedPolicy(
		ctx,
		createdCanonical.ID,
		planned.Key,
	)
	if err != nil || !found {
		detail := "The settings object was created, but its exact canonical state could not be confirmed. The confirmed create identity remains in Terraform state for safe recovery."
		if err != nil {
			detail += " " + err.Error() + "."
		}
		resp.Diagnostics.AddError("Unable to Confirm Created FeatBit Policy", detail)
		return
	}
	if !r.setPolicyState(ctx, &resp.State, &resp.Diagnostics, confirmed, &planModel) {
		return
	}
	if !samePolicySettings(confirmed, planned) {
		resp.Diagnostics.AddError(
			"Created FeatBit Policy Settings Are Unconfirmed",
			"The exact canonical Policy does not contain the planned name and description. Its confirmed server form remains in state and no statement mutation was sent.",
		)
		return
	}

	statementResponse, statementErr := r.client.ReplacePolicyStatements(
		ctx,
		confirmed.ID,
		expandPolicyStatements(planned),
	)
	if statementErr == nil {
		if provisional, provisionalErr := r.canonicalExpectedPolicy(
			statementResponse,
			confirmed.ID,
			planned.Key,
		); provisionalErr == nil {
			if !r.setPolicyState(
				ctx,
				&resp.State,
				&resp.Diagnostics,
				provisional,
				&planModel,
			) {
				return
			}
		}
	}

	final, found, readErr := r.readManagedPolicy(ctx, confirmed.ID, planned.Key)
	if readErr != nil || !found {
		detail := "The complete statement replacement outcome could not be confirmed through an exact canonical read. Terraform did not retry the mutation and preserved the last confirmed Policy state."
		if statementErr != nil {
			detail += " " + statementErr.Error() + "."
		} else if readErr != nil {
			detail += " " + readErr.Error() + "."
		}
		resp.Diagnostics.AddError("Unable to Confirm FeatBit Policy Statements", detail)
		return
	}
	if !r.setPolicyState(ctx, &resp.State, &resp.Diagnostics, final, &planModel) {
		return
	}
	if statementErr != nil && !mutationNeedsReconciliation(statementErr) {
		resp.Diagnostics.AddError(
			"Unable to Set FeatBit Policy Statements",
			"The complete statement replacement failed without an ambiguous outcome. Terraform preserved the exact reread state and did not retry the mutation. "+statementErr.Error()+".",
		)
		return
	}
	if !samePolicyStatements(final.Statements, planned.Statements) {
		resp.Diagnostics.AddError(
			"FeatBit Policy Statement Replacement Is Unconfirmed",
			"The exact canonical Policy does not contain the complete planned statement set. Terraform did not retry the mutation; the confirmed server form remains in state.",
		)
		return
	}
	if !samePolicyDefinition(final, planned) {
		resp.Diagnostics.AddError(
			"Created FeatBit Policy Definition Is Unconfirmed",
			"The final exact canonical Policy does not match the complete planned definition. Its confirmed server form remains in state.",
		)
	}
}

func (r *policyResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a Policy", &resp.Diagnostics) {
		return
	}
	var prior policyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() || !knownString(prior.ID) ||
		!client.ValidUUID(prior.ID.ValueString()) {
		if !resp.Diagnostics.HasError() {
			resp.Diagnostics.AddError(
				"Invalid FeatBit Policy State",
				"The managed Policy state does not contain a valid exact UUID. Terraform state was preserved.",
			)
		}
		return
	}
	expectedKey := ""
	if knownString(prior.Key) {
		expectedKey = prior.Key.ValueString()
	}
	policy, found, err := r.readManagedPolicy(ctx, prior.ID.ValueString(), expectedKey)
	if err != nil {
		title := "Unable to Read FeatBit Policy"
		detail := "The provider could not confirm the exact custom Policy through the documented public API. Terraform state has been preserved. " + err.Error() + "."
		if errors.Is(err, errManagedPolicyBuiltIn) {
			title = "Built-in FeatBit Policy Cannot Be Managed"
			detail = "The exact UUID belongs to a built-in SysManaged Policy. Built-in Policies are read-only; remove this resource from state and use the featbit_policy data source instead. No mutation was sent."
		}
		resp.Diagnostics.AddError(title, detail)
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	r.setPolicyState(ctx, &resp.State, &resp.Diagnostics, policy, &prior)
}

func (r *policyResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a Policy", &resp.Diagnostics) {
		return
	}
	var priorModel policyModel
	var planModel policyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &priorModel)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planModel)...)
	if resp.Diagnostics.HasError() {
		return
	}
	prior, priorErr := canonicalizePolicyStateModel(ctx, priorModel)
	planned, planErr := canonicalizePolicyPlanModel(ctx, planModel)
	if priorErr != nil || planErr != nil || prior.Key != planned.Key {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Policy Update",
			"The complete prior state and plan could not be correlated as one safe custom Policy with an unchanged key. Terraform state was preserved and no mutation was sent.",
		)
		return
	}

	release, err := r.policyLocks().acquire(ctx, policyWriteLockKey(prior.Key))
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Acquire FeatBit Policy Update Lock",
			"The update was canceled while waiting to serialize writes to the exact Policy. Terraform state was preserved and no mutation was sent.",
		)
		return
	}
	defer release()

	current, found, err := r.readManagedPolicy(ctx, prior.ID, prior.Key)
	if err != nil || !found {
		detail := "The exact custom Policy could not be confirmed before Update. Terraform state was preserved and no mutation was sent."
		if err != nil {
			detail += " " + err.Error() + "."
		}
		resp.Diagnostics.AddError("Unable to Confirm FeatBit Policy Before Update", detail)
		return
	}
	if !r.setPolicyState(ctx, &resp.State, &resp.Diagnostics, current, &priorModel) {
		return
	}

	settingsChanged := !samePolicySettings(prior, planned)
	statementsChanged := !samePolicyStatements(prior.Statements, planned.Statements)

	if settingsChanged {
		mutationResponse, mutationErr := r.client.UpdatePolicySettings(
			ctx,
			prior.ID,
			client.UpdatePolicySettingsRequest{
				Name:        planned.Name,
				Description: planned.Description,
			},
		)
		if mutationErr == nil {
			if provisional, provisionalErr := r.canonicalExpectedPolicy(
				mutationResponse,
				prior.ID,
				prior.Key,
			); provisionalErr == nil {
				if !r.setPolicyState(
					ctx,
					&resp.State,
					&resp.Diagnostics,
					provisional,
					&planModel,
				) {
					return
				}
			}
		}
		confirmed, found, readErr := r.readManagedPolicy(ctx, prior.ID, prior.Key)
		if readErr != nil || !found {
			detail := "The Policy settings outcome could not be confirmed through an exact canonical read. Terraform did not retry the mutation and preserved the last confirmed state."
			if mutationErr != nil {
				detail += " " + mutationErr.Error() + "."
			} else if readErr != nil {
				detail += " " + readErr.Error() + "."
			}
			resp.Diagnostics.AddError("Unable to Confirm Updated FeatBit Policy Settings", detail)
			return
		}
		if !r.setPolicyState(ctx, &resp.State, &resp.Diagnostics, confirmed, &planModel) {
			return
		}
		if mutationErr != nil && !mutationNeedsReconciliation(mutationErr) {
			resp.Diagnostics.AddError(
				"Unable to Update FeatBit Policy Settings",
				"The settings mutation failed without an ambiguous outcome. Terraform preserved the exact reread state and did not retry the request. "+mutationErr.Error()+".",
			)
			return
		}
		if !samePolicySettings(confirmed, planned) {
			resp.Diagnostics.AddError(
				"FeatBit Policy Settings Update Is Unconfirmed",
				"The exact canonical Policy does not contain the planned name and description. Terraform did not retry the mutation; the confirmed server form remains in state.",
			)
			return
		}
	}

	if statementsChanged {
		mutationResponse, mutationErr := r.client.ReplacePolicyStatements(
			ctx,
			prior.ID,
			expandPolicyStatements(planned),
		)
		if mutationErr == nil {
			if provisional, provisionalErr := r.canonicalExpectedPolicy(
				mutationResponse,
				prior.ID,
				prior.Key,
			); provisionalErr == nil {
				if !r.setPolicyState(
					ctx,
					&resp.State,
					&resp.Diagnostics,
					provisional,
					&planModel,
				) {
					return
				}
			}
		}
		confirmed, found, readErr := r.readManagedPolicy(ctx, prior.ID, prior.Key)
		if readErr != nil || !found {
			detail := "The complete statement replacement outcome could not be confirmed through an exact canonical read. Terraform did not retry the mutation and preserved the last confirmed state."
			if mutationErr != nil {
				detail += " " + mutationErr.Error() + "."
			} else if readErr != nil {
				detail += " " + readErr.Error() + "."
			}
			resp.Diagnostics.AddError("Unable to Confirm Updated FeatBit Policy Statements", detail)
			return
		}
		if !r.setPolicyState(ctx, &resp.State, &resp.Diagnostics, confirmed, &planModel) {
			return
		}
		if mutationErr != nil && !mutationNeedsReconciliation(mutationErr) {
			resp.Diagnostics.AddError(
				"Unable to Replace FeatBit Policy Statements",
				"The complete statement mutation failed without an ambiguous outcome. Terraform preserved the exact reread state and did not retry the request. "+mutationErr.Error()+".",
			)
			return
		}
		if !samePolicyStatements(confirmed.Statements, planned.Statements) {
			resp.Diagnostics.AddError(
				"FeatBit Policy Statement Replacement Is Unconfirmed",
				"The exact canonical Policy does not contain the complete planned statement set. Terraform did not retry the mutation; the confirmed server form remains in state.",
			)
			return
		}
	}

	final, found, err := r.readManagedPolicy(ctx, prior.ID, prior.Key)
	if err != nil || !found {
		detail := "The deterministic Policy updates completed, but the final exact canonical Policy could not be read. Terraform preserved the last mutation-confirmed state."
		if err != nil {
			detail += " " + err.Error() + "."
		}
		resp.Diagnostics.AddError("Unable to Confirm Updated FeatBit Policy", detail)
		return
	}
	if !r.setPolicyState(ctx, &resp.State, &resp.Diagnostics, final, &planModel) {
		return
	}
	if !samePolicyDefinition(final, planned) {
		resp.Diagnostics.AddError(
			"Updated FeatBit Policy Definition Is Unconfirmed",
			"The final exact canonical Policy does not match the complete planned definition. The confirmed server form remains in state.",
		)
	}
}

func (r *policyResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a Policy", &resp.Diagnostics) {
		return
	}
	var priorModel policyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &priorModel)...)
	if resp.Diagnostics.HasError() {
		return
	}
	prior, err := canonicalizePolicyStateModel(ctx, priorModel)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Policy Delete State",
			"The stored custom Policy definition is incomplete or unsafe. Terraform state was preserved and no mutation was sent.",
		)
		return
	}

	release, err := r.policyLocks().acquire(ctx, policyWriteLockKey(prior.Key))
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Acquire FeatBit Policy Delete Lock",
			"Deletion was canceled while waiting to serialize writes to the exact Policy. Terraform state was preserved and no mutation was sent.",
		)
		return
	}
	defer release()

	current, found, err := r.readManagedPolicy(ctx, prior.ID, prior.Key)
	if err != nil {
		title := "Unable to Confirm FeatBit Policy Before Delete"
		detail := "The exact custom Policy could not be confirmed before Delete. Terraform state was preserved and no mutation was sent. " + err.Error() + "."
		if errors.Is(err, errManagedPolicyBuiltIn) {
			title = "Built-in FeatBit Policy Cannot Be Deleted"
			detail = "The exact UUID belongs to a built-in SysManaged Policy. Terraform structurally forbids every built-in mutation; state was preserved and no association or delete request was sent."
		}
		resp.Diagnostics.AddError(title, detail)
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	if !r.setPolicyState(ctx, &resp.State, &resp.Diagnostics, current, &priorModel) {
		return
	}

	groupCount, err := r.client.CountPolicyGroups(ctx, prior.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Check FeatBit Policy Group Associations",
			"The provider could not read the complete exact Group association collection. Terraform state was preserved and no delete mutation was sent. "+err.Error()+".",
		)
		return
	}
	memberCount, err := r.client.CountPolicyMembers(ctx, prior.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Check FeatBit Policy Member Associations",
			"The provider could not read the complete direct-Member association collection. Terraform state was preserved and no delete mutation was sent. "+err.Error()+".",
		)
		return
	}
	if groupCount != 0 || memberCount != 0 {
		resp.Diagnostics.AddError(
			"FeatBit Policy Still Has Live Associations",
			"Destroy refuses to cascade a Policy that is still assigned to one or more Groups or direct Members. Remove those exact bindings first; Terraform state was preserved and no delete mutation was sent.",
		)
		return
	}

	deleteErr := r.client.DeletePolicy(ctx, prior.ID)
	policies, listErr := r.client.ListPolicies(ctx)
	if listErr != nil {
		detail := "The delete attempt completed, but the complete Policy collection could not prove exact absence. Terraform state was preserved. " + listErr.Error() + "."
		if deleteErr != nil {
			detail = "The delete request failed and the complete Policy collection could not prove exact absence. Terraform state was preserved. " + deleteErr.Error() + "."
		}
		resp.Diagnostics.AddError("Unable to Confirm FeatBit Policy Deletion", detail)
		return
	}
	remaining, found, resolveErr := r.client.ResolvePolicyByID(policies, prior.ID)
	if resolveErr != nil {
		resp.Diagnostics.AddError(
			"Unable to Confirm FeatBit Policy Deletion",
			"The complete Policy collection returned an ambiguous exact identity after Delete. Terraform state was preserved.",
		)
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	if canonical, canonicalErr := r.canonicalExpectedPolicy(
		remaining,
		prior.ID,
		prior.Key,
	); canonicalErr == nil {
		r.setPolicyState(ctx, &resp.State, &resp.Diagnostics, canonical, &priorModel)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	detail := "The exact custom Policy still exists after the delete request. Terraform state was preserved."
	if deleteErr != nil {
		detail = "The delete request failed and the exact custom Policy still exists. Terraform state was preserved. " + deleteErr.Error() + "."
	}
	resp.Diagnostics.AddError("FeatBit Policy Was Not Deleted", detail)
}

func (r *policyResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	policyID, valid := client.CanonicalUUID(req.ID)
	if !valid {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Policy Import Identifier",
			"Import a custom Policy with exactly one UUID in 8-4-4-4-12 hexadecimal form.",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), policyID)...)
}

func (r *policyResource) readManagedPolicy(
	ctx context.Context,
	policyID string,
	expectedKey string,
) (canonicalPolicy, bool, error) {
	policy, found, err := r.client.GetPolicy(ctx, policyID)
	if err != nil || !found {
		return canonicalPolicy{}, found, err
	}
	if policy.Type == client.PolicyTypeSysManaged {
		return canonicalPolicy{}, false, errManagedPolicyBuiltIn
	}
	canonical, err := r.canonicalExpectedPolicy(policy, policyID, expectedKey)
	if err != nil {
		return canonicalPolicy{}, false, err
	}
	return canonical, true, nil
}

func (r *policyResource) canonicalExpectedPolicy(
	policy client.Policy,
	policyID string,
	expectedKey string,
) (canonicalPolicy, error) {
	canonical, err := canonicalizeRemoteManagedPolicy(policy)
	if err != nil || !client.EqualUUID(canonical.ID, policyID) ||
		(expectedKey != "" && canonical.Key != expectedKey) {
		return canonicalPolicy{}, errManagedPolicyIdentity
	}
	return canonical, nil
}

func (r *policyResource) setPolicyState(
	ctx context.Context,
	state *tfsdk.State,
	diagnostics *diag.Diagnostics,
	policy canonicalPolicy,
	preferred *policyModel,
) bool {
	model := flattenManagedPolicy(ctx, policy, preferred)
	diagnostics.Append(state.Set(ctx, &model)...)
	return !diagnostics.HasError()
}

func (r *policyResource) reconcileAmbiguousPolicyCreate(
	ctx context.Context,
	createErr error,
	key string,
	diagnostics *diag.Diagnostics,
) {
	policies, listErr := r.client.ListPolicies(ctx)
	if listErr != nil {
		diagnostics.AddError(
			"FeatBit Policy Create Outcome Is Unconfirmed",
			"The create result was ambiguous and the complete Policy collection could not be read. Terraform did not retry or adopt any object. Verify the remote system before retrying, then import an intended custom Policy by UUID if it exists. "+createErr.Error()+".",
		)
		return
	}
	_, found, resolveErr := r.client.ResolvePolicyByKey(policies, key)
	switch {
	case resolveErr != nil:
		diagnostics.AddError(
			"FeatBit Policy Create Outcome Is Ambiguous",
			"The create result was ambiguous and multiple Policies now have the configured exact key. Terraform did not retry or adopt any object. Resolve the duplicates before continuing.",
		)
	case found:
		diagnostics.AddError(
			"FeatBit Policy Create Outcome Requires Recovery",
			"The create result was ambiguous and exactly one Policy now has the configured key. Terraform did not retry or adopt it. Verify that object, then import it deliberately by UUID if it is custom or remove it before retrying.",
		)
	default:
		diagnostics.AddError(
			"Unable to Create FeatBit Policy",
			"The create result was ambiguous, but the complete Policy collection contains no exact-key match. Terraform did not retry the mutation. "+createErr.Error()+".",
		)
	}
}

func (r *policyResource) policyLocks() *keyedLockManager {
	r.lockOnce.Do(func() {
		if r.locks == nil {
			r.locks = newKeyedLockManager()
		}
	})
	return r.locks
}

func policyWriteLockKey(key string) string {
	return "policy\x00" + key
}
