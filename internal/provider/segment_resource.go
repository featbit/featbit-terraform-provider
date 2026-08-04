// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var (
	_ resource.Resource                = (*segmentResource)(nil)
	_ resource.ResourceWithConfigure   = (*segmentResource)(nil)
	_ resource.ResourceWithImportState = (*segmentResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*segmentResource)(nil)
)

var (
	errManagedSegmentArchived = errors.New("managed Segment is archived")
	errManagedSegmentIdentity = errors.New("managed Segment identity is inconsistent")
	errManagedSegmentShared   = errors.New("managed Segment is not environment-specific")
)

type segmentResource struct {
	client   *client.Client
	lockOnce sync.Once
	locks    *keyedLockManager
}

func newSegmentResource() resource.Resource {
	return &segmentResource{}
}

func (r *segmentResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_segment"
}

func (r *segmentResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = segmentResourceSchema()
}

func (r *segmentResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	r.client = clientFromProviderData(req.ProviderData, "Resource", &resp.Diagnostics)
}

func (r *segmentResource) ModifyPlan(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan segmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !segmentPlanDefinitionKnown(plan) {
		return
	}
	planned, err := canonicalizeSegmentPlanModel(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Segment Plan",
			"The complete environment-specific Segment plan could not be canonicalized safely.",
		)
		return
	}

	var prior segmentModel
	if !req.State.Raw.IsNull() {
		resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
		if resp.Diagnostics.HasError() {
			return
		}
		canonicalPrior, priorErr := canonicalizeSegmentStateModel(ctx, prior)
		if priorErr == nil && sameSegmentReplacementInputs(planned, canonicalPrior) {
			planned.ID = canonicalPrior.ID
			preservePlannedSegmentTargetingIDs(&planned, plan, canonicalPrior)
			if !validCanonicalSegmentTargetingIDs(planned) {
				resp.Diagnostics.AddError(
					"Invalid FeatBit Segment Plan",
					"The planned and prior Segment targeting identities cannot be correlated without a duplicate UUID.",
				)
				return
			}
		}
	}

	canonicalModel := flattenCanonicalSegment(planned)
	canonicalModel.EnvironmentID = plan.EnvironmentID
	if planned.ID == "" {
		canonicalModel.ID = plan.ID
	} else if !prior.ID.IsNull() && !prior.ID.IsUnknown() &&
		sameKnownSegmentIdentity(planned, prior) {
		canonicalModel.ID = prior.ID
	}
	preserveConfiguredSegmentConditionValues(&canonicalModel, plan)
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &canonicalModel)...)
}

func (r *segmentResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	if !requireAPIClient(r.client, "managing a Segment", &resp.Diagnostics) {
		return
	}

	var plan segmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	planned, err := canonicalizeSegmentPlanModel(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Segment Plan",
			"The Segment definition contains an unknown, missing, contradictory, or invalid value. Correct the configuration before retrying.",
		)
		return
	}

	release, err := r.segmentLocks().acquire(
		ctx,
		segmentCreateLockKey(planned.EnvironmentID, planned.Key),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Acquire FeatBit Segment Create Lock",
			"Segment creation was canceled while waiting to serialize the exact Environment and key. No create or initialization request was sent.",
		)
		return
	}
	defer release()

	_, status, err := r.client.ResolveSegment(
		ctx,
		planned.EnvironmentID,
		client.SegmentIdentity{Key: planned.Key},
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Check Existing FeatBit Segments",
			"The provider could not complete both active and archived exact-key collection views for the Segment create preflight. "+err.Error()+".",
		)
		return
	}
	switch status {
	case client.SegmentStatusAbsent:
		// Complete active and archived views proved exact zero.
	case client.SegmentStatusActive:
		resp.Diagnostics.AddError(
			"FeatBit Segment Create Preflight Failed",
			"An active Segment with the configured exact key already exists in the Environment. Terraform will not adopt it automatically; import the intended environment-specific Segment as <environment_uuid>/<segment_uuid> or choose another key.",
		)
		return
	case client.SegmentStatusArchived:
		resp.Diagnostics.AddError(
			"FeatBit Segment Archived-Key Collision",
			"An archived Segment with the configured exact key already exists in the Environment. Permanently remove that archived Segment outside Terraform or choose another key before retrying.",
		)
		return
	default:
		resp.Diagnostics.AddError(
			"FeatBit Segment Create Preflight Is Unconfirmed",
			"The provider did not receive an authoritative exact-key status from the complete active and archived views. No create request was sent.",
		)
		return
	}

	created, createErr := r.client.CreateSegment(
		ctx,
		planned.EnvironmentID,
		expandSegmentCreateRequest(planned),
	)
	createdID, idValid := client.CanonicalUUID(created.ID)
	if !idValid {
		if createErr != nil && mutationOutcomeAmbiguous(createErr) {
			r.reconcileAmbiguousSegmentCreate(
				ctx,
				planned.EnvironmentID,
				planned.Key,
				createErr,
				&resp.Diagnostics,
			)
			return
		}
		detail := "The Segment create response did not establish a valid remote identity. Terraform did not retry or adopt an object; verify the remote Environment before retrying."
		if createErr != nil {
			detail = "The Segment create request failed without a mutation-confirmed remote identity. " + createErr.Error() + "."
		}
		resp.Diagnostics.AddError("Unable to Create FeatBit Segment", detail)
		return
	}

	// Preserve the mutation-confirmed UUID before any exact read or follow-up
	// initialization. The remaining provisional definition is the validated
	// plan and is replaced by each confirmed server boundary as it becomes
	// available.
	planned.ID = createdID
	provisional := flattenCanonicalSegment(planned)
	provisional.EnvironmentID = plan.EnvironmentID
	preserveConfiguredSegmentConditionValues(&provisional, plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &provisional)...)
	if resp.Diagnostics.HasError() {
		return
	}

	current, err := r.readExactManagedSegment(
		ctx,
		planned.EnvironmentID,
		createdID,
		planned.Key,
	)
	if err != nil {
		title := "Unable to Confirm Created FeatBit Segment"
		detail := "The Segment was created, but its exact canonical definition could not be confirmed. The mutation-confirmed UUID remains in Terraform state for safe recovery."
		if errors.Is(err, errManagedSegmentShared) {
			title = "Created FeatBit Segment Has Unsafe Type or Scope"
			detail = "The mutation-confirmed object is not a safe environment-specific Segment. Terraform preserved provisional state and sent no targeting or tag mutation. Resolve the remote object before continuing."
		} else if errors.Is(err, errManagedSegmentArchived) ||
			errors.Is(err, errManagedSegmentIdentity) {
			title = "Created FeatBit Segment Identity Is Unconfirmed"
			detail = "The mutation-confirmed UUID did not resolve to the expected active environment-specific Segment. Terraform preserved provisional state and sent no targeting or tag mutation."
		}
		if createErr != nil && !errors.Is(err, errManagedSegmentShared) {
			detail += " The create response was also ambiguous; Terraform did not replay it."
		}
		resp.Diagnostics.AddError(title, detail)
		return
	}

	if !sameSegmentCreateFields(planned, current) {
		state := segmentStateFromCanonical(current, plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
		resp.Diagnostics.AddError(
			"Created FeatBit Segment Definition Mismatch",
			"The exact active Segment does not match the immutable metadata sent by Terraform. The confirmed server form remains in state; no targeting or tag mutation was sent.",
		)
		return
	}
	state := segmentStateFromCanonical(current, plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !sameSegmentTargeting(planned, current) {
		mutationErr := r.client.UpdateSegmentTargeting(
			ctx,
			planned.EnvironmentID,
			createdID,
			expandSegmentTargetingRequest(planned),
		)
		if mutationErr != nil && !segmentMutationNeedsReconciliation(mutationErr) {
			resp.Diagnostics.AddError(
				"Unable to Initialize FeatBit Segment Targeting",
				"The specialized targeting initialization did not complete. Terraform preserved the last confirmed server state and did not retry the mutation. "+mutationErr.Error()+".",
			)
			return
		}
		confirmed, readErr := r.readExactManagedSegment(
			ctx,
			planned.EnvironmentID,
			createdID,
			planned.Key,
		)
		if readErr != nil {
			detail := "The targeting initialization response succeeded, but exact canonical state could not be confirmed. Terraform preserved the last confirmed server state."
			if mutationErr != nil {
				detail = "The targeting initialization result was ambiguous and exact canonical state could not confirm its outcome. Terraform did not retry the mutation and preserved the last confirmed server state."
			}
			resp.Diagnostics.AddError("Unable to Confirm FeatBit Segment Targeting", detail)
			return
		}
		state = segmentStateFromCanonical(confirmed, plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if !sameSegmentCreateFields(planned, confirmed) ||
			!sameSegmentTargeting(planned, confirmed) {
			detail := "The specialized targeting initialization returned success, but the exact canonical Segment does not contain the planned targeting. The confirmed server form remains in state."
			if mutationErr != nil {
				detail = "The targeting initialization result was ambiguous and the exact canonical Segment does not contain the planned targeting. Terraform did not retry the mutation; the confirmed server form remains in state."
			}
			resp.Diagnostics.AddError("FeatBit Segment Targeting Is Unconfirmed", detail)
			return
		}
		current = confirmed
	}

	if !sameSegmentTags(planned, current) {
		mutationErr := r.client.UpdateSegmentTags(
			ctx,
			planned.EnvironmentID,
			createdID,
			client.UpdateSegmentTagsRequest{Tags: append([]string{}, planned.Tags...)},
		)
		if mutationErr != nil && !segmentMutationNeedsReconciliation(mutationErr) {
			resp.Diagnostics.AddError(
				"Unable to Initialize FeatBit Segment Tags",
				"The specialized tag initialization did not complete. Terraform preserved the last confirmed server state and did not retry the mutation. "+mutationErr.Error()+".",
			)
			return
		}
		confirmed, readErr := r.readExactManagedSegment(
			ctx,
			planned.EnvironmentID,
			createdID,
			planned.Key,
		)
		if readErr != nil {
			detail := "The tag initialization response succeeded, but exact canonical state could not be confirmed. Terraform preserved the last confirmed server state."
			if mutationErr != nil {
				detail = "The tag initialization result was ambiguous and exact canonical state could not confirm its outcome. Terraform did not retry the mutation and preserved the last confirmed server state."
			}
			resp.Diagnostics.AddError("Unable to Confirm FeatBit Segment Tags", detail)
			return
		}
		state = segmentStateFromCanonical(confirmed, plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if !sameSegmentCreateFields(planned, confirmed) ||
			!sameSegmentTargeting(planned, confirmed) ||
			!sameSegmentTags(planned, confirmed) {
			detail := "The specialized tag initialization returned success, but the exact canonical Segment does not contain the complete planned definition. The confirmed server form remains in state."
			if mutationErr != nil {
				detail = "The tag initialization result was ambiguous and the exact canonical Segment does not contain the complete planned definition. Terraform did not retry the mutation; the confirmed server form remains in state."
			}
			resp.Diagnostics.AddError("FeatBit Segment Tag Initialization Is Unconfirmed", detail)
			return
		}
		current = confirmed
	}

	if !sameSegmentDefinition(planned, current) {
		resp.Diagnostics.AddError(
			"Created FeatBit Segment Definition Mismatch",
			"The canonical active Segment does not match the complete deterministic definition sent by Terraform. The confirmed server form remains in state; review the remote object before retrying.",
		)
	}
}

func (r *segmentResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a Segment", &resp.Diagnostics) {
		return
	}

	var prior segmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !knownString(prior.EnvironmentID) || !knownString(prior.ID) ||
		!client.ValidUUID(prior.EnvironmentID.ValueString()) ||
		!client.ValidUUID(prior.ID.ValueString()) ||
		(knownString(prior.Key) && !validSegmentKey(prior.Key.ValueString())) {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Segment State",
			"The Segment state does not contain valid exact Environment and Segment UUIDs, or contains an invalid exact key. State has been preserved; import the intended object again if necessary.",
		)
		return
	}

	environmentID := prior.EnvironmentID.ValueString()
	segmentID := prior.ID.ValueString()
	remote, directErr := r.client.GetSegment(ctx, environmentID, segmentID)
	if directErr != nil {
		identity := client.SegmentIdentity{ID: segmentID}
		if knownString(prior.Key) {
			identity.Key = prior.Key.ValueString()
		}
		_, status, resolveErr := r.client.ResolveSegment(ctx, environmentID, identity)
		if resolveErr != nil {
			resp.Diagnostics.AddError(
				"Unable to Read FeatBit Segment",
				"The exact read failed and complete active and archived views could not resolve the managed Segment. Terraform state has been preserved. "+resolveErr.Error()+".",
			)
			return
		}
		switch status {
		case client.SegmentStatusAbsent:
			resp.State.RemoveResource(ctx)
		case client.SegmentStatusArchived:
			resp.Diagnostics.AddError(
				"Managed FeatBit Segment Was Archived Outside Terraform",
				"The exact Segment exists only in the archived view. Terraform preserved state and will not restore it or create a colliding replacement. Restore it outside Terraform to continue managing it, or remove it from configuration and destroy it through the archived cleanup path.",
			)
		case client.SegmentStatusActive:
			resp.Diagnostics.AddError(
				"FeatBit Segment Definition Is Unconfirmed",
				"Complete collection views still contain the exact active Segment, but the complete exact definition could not be read. Terraform state has been preserved.",
			)
		default:
			resp.Diagnostics.AddError(
				"Unable to Read FeatBit Segment",
				"The provider received an unconfirmed Segment status. Terraform state has been preserved.",
			)
		}
		return
	}

	key := ""
	if knownString(prior.Key) {
		key = prior.Key.ValueString()
	}
	canonical, err := canonicalManagedSegment(remote, environmentID, segmentID, key)
	if err != nil {
		switch {
		case errors.Is(err, errManagedSegmentArchived):
			resp.Diagnostics.AddError(
				"Managed FeatBit Segment Was Archived Outside Terraform",
				"The exact Segment is archived. Terraform preserved state and will not restore it or create a colliding replacement.",
			)
		case errors.Is(err, errManagedSegmentShared):
			resp.Diagnostics.AddError(
				"Managed FeatBit Segment Has Unsafe Type or Scope",
				"The exact UUID now resolves to a shared or contradictory Segment definition. Terraform preserved state and sent no mutation; shared Segments are data-source-only.",
			)
		default:
			resp.Diagnostics.AddError(
				"Invalid FeatBit Segment Definition",
				"The exact active Segment definition could not be canonicalized or correlated safely. Correct the remote definition before retrying; Terraform state has been preserved.",
			)
		}
		return
	}
	state := segmentStateFromCanonical(canonical, prior)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *segmentResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a Segment", &resp.Diagnostics) {
		return
	}

	var priorModel segmentModel
	var planModel segmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &priorModel)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planModel)...)
	if resp.Diagnostics.HasError() {
		return
	}
	prior, priorErr := canonicalizeSegmentStateModel(ctx, priorModel)
	planned, planErr := canonicalizeSegmentPlanModel(ctx, planModel)
	if priorErr != nil || planErr != nil {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Segment Update",
			"The complete prior state and plan could not be canonicalized as safe environment-specific Segment definitions. Terraform state was preserved and no mutation was sent.",
		)
		return
	}
	if !sameSegmentImmutableFields(prior, planned) {
		resp.Diagnostics.AddError(
			"Unsafe FeatBit Segment Update",
			"The prior state and plan disagree on immutable Segment identity, type, or scope. Terraform state was preserved and no shared or replacement mutation was sent.",
		)
		return
	}

	// Freeze the canonical diff before the first request. A reconciliation read
	// may observe concurrent remote drift, but it never expands, suppresses, or
	// replays this deterministic one-shot mutation set.
	steps := []segmentUpdateStep{
		{
			name:    "Name",
			changed: prior.Name != planned.Name,
			mutate: func() error {
				return r.client.UpdateSegmentName(
					ctx,
					planned.EnvironmentID,
					planned.ID,
					client.UpdateSegmentNameRequest{Name: planned.Name},
				)
			},
			matches: func(actual canonicalSegment) bool {
				return actual.Name == planned.Name
			},
			apply: func(current *canonicalSegment) {
				current.Name = planned.Name
			},
		},
		{
			name:    "Description",
			changed: prior.Description != planned.Description,
			mutate: func() error {
				return r.client.UpdateSegmentDescription(
					ctx,
					planned.EnvironmentID,
					planned.ID,
					client.UpdateSegmentDescriptionRequest{Description: planned.Description},
				)
			},
			matches: func(actual canonicalSegment) bool {
				return actual.Description == planned.Description
			},
			apply: func(current *canonicalSegment) {
				current.Description = planned.Description
			},
		},
		{
			name:    "Targeting",
			changed: !sameSegmentTargeting(prior, planned),
			mutate: func() error {
				return r.client.UpdateSegmentTargeting(
					ctx,
					planned.EnvironmentID,
					planned.ID,
					expandSegmentTargetingRequest(planned),
				)
			},
			matches: func(actual canonicalSegment) bool {
				return sameSegmentTargeting(actual, planned)
			},
			apply: func(current *canonicalSegment) {
				current.IncludedUsers = append([]string(nil), planned.IncludedUsers...)
				current.ExcludedUsers = append([]string(nil), planned.ExcludedUsers...)
				current.Rules = cloneCanonicalSegmentRules(planned.Rules)
			},
		},
		{
			name:    "Tags",
			changed: !sameSegmentTags(prior, planned),
			mutate: func() error {
				return r.client.UpdateSegmentTags(
					ctx,
					planned.EnvironmentID,
					planned.ID,
					client.UpdateSegmentTagsRequest{Tags: append([]string(nil), planned.Tags...)},
				)
			},
			matches: func(actual canonicalSegment) bool {
				return sameSegmentTags(actual, planned)
			},
			apply: func(current *canonicalSegment) {
				current.Tags = append([]string(nil), planned.Tags...)
			},
		},
	}

	release, err := r.segmentLocks().acquire(
		ctx,
		segmentWriteLockKey(planned.EnvironmentID, planned.ID),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Acquire FeatBit Segment Update Lock",
			"The update was canceled while waiting to serialize writes to the exact Environment and Segment. Terraform state was preserved and no mutation was sent.",
		)
		return
	}
	defer release()

	current := prior
	setState := func(segment canonicalSegment) bool {
		state := segmentStateFromCanonical(segment, planModel)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return !resp.Diagnostics.HasError()
	}

	for _, step := range steps {
		if !step.changed {
			continue
		}
		mutationErr := step.mutate()
		if mutationErr == nil {
			step.apply(&current)
			if !setState(current) {
				return
			}
			continue
		}
		if !segmentMutationNeedsReconciliation(mutationErr) {
			resp.Diagnostics.AddError(
				"Unable to Update FeatBit Segment "+step.name,
				"The specialized "+strings.ToLower(step.name)+" update did not complete. Terraform preserved the last mutation-confirmed state and did not retry the request. "+mutationErr.Error()+".",
			)
			return
		}

		confirmed, readErr := r.readExactManagedSegment(
			ctx,
			planned.EnvironmentID,
			planned.ID,
			planned.Key,
		)
		if readErr != nil {
			resp.Diagnostics.AddError(
				"Unable to Reconcile FeatBit Segment "+step.name,
				"The specialized "+strings.ToLower(step.name)+" update result was ambiguous and the exact canonical Segment could not confirm its outcome. Terraform did not retry the mutation and preserved the last mutation-confirmed state.",
			)
			return
		}
		if !setState(confirmed) {
			return
		}
		if !sameSegmentImmutableFields(planned, confirmed) {
			resp.Diagnostics.AddError(
				"Updated FeatBit Segment Identity or Scope Mismatch",
				"The exact canonical read no longer matches the managed Segment identity, type, or scope. The confirmed server form remains in state and no further mutation was sent.",
			)
			return
		}
		if !step.matches(confirmed) {
			resp.Diagnostics.AddError(
				"FeatBit Segment "+step.name+" Update Is Unconfirmed",
				"The specialized "+strings.ToLower(step.name)+" update result was ambiguous and the exact canonical Segment did not contain the planned component. Terraform did not retry the mutation; the confirmed server form remains in state.",
			)
			return
		}
		current = confirmed
	}

	confirmed, err := r.readExactManagedSegment(
		ctx,
		planned.EnvironmentID,
		planned.ID,
		planned.Key,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Confirm Updated FeatBit Segment",
			"The deterministic specialized updates completed, but the final exact canonical Segment could not be read safely. Terraform preserved the last mutation-confirmed state.",
		)
		return
	}
	if !setState(confirmed) {
		return
	}
	if !sameSegmentImmutableFields(planned, confirmed) {
		resp.Diagnostics.AddError(
			"Updated FeatBit Segment Identity or Scope Mismatch",
			"The final exact canonical read no longer matches the managed Segment identity, type, or scope. The confirmed server form remains in state.",
		)
		return
	}
	if !sameSegmentDefinition(planned, confirmed) {
		resp.Diagnostics.AddError(
			"Updated FeatBit Segment Definition Is Unconfirmed",
			"The final exact canonical Segment does not match the complete planned definition. The confirmed server form remains in state; review the remote object before retrying.",
		)
	}
}

// P4-014 replaces this fail-closed boundary with reference-aware archive and
// permanent deletion. No destructive request is reachable before that exact
// lifecycle is implemented.
func (r *segmentResource) Delete(
	_ context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	resp.State = req.State
	resp.Diagnostics.AddError(
		"FeatBit Segment Deletion Is Not Available",
		"This provider build cannot yet prove references and permanently delete a Segment. Terraform state was preserved and no destructive mutation was sent.",
	)
}

func (r *segmentResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	components := strings.Split(req.ID, "/")
	if len(components) != 2 || !validUUID(components[0]) || !validUUID(components[1]) {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Segment Import Identifier",
			"Import a Segment as <environment_uuid>/<segment_uuid>, with both values in 8-4-4-4-12 hexadecimal UUID form.",
		)
		return
	}
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("environment_id"), components[0])...,
	)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), components[1])...)
}

type segmentUpdateStep struct {
	name    string
	changed bool
	mutate  func() error
	matches func(canonicalSegment) bool
	apply   func(*canonicalSegment)
}

func (r *segmentResource) readExactManagedSegment(
	ctx context.Context,
	environmentID string,
	segmentID string,
	key string,
) (canonicalSegment, error) {
	remote, err := r.client.GetSegment(ctx, environmentID, segmentID)
	if err != nil {
		return canonicalSegment{}, err
	}
	return canonicalManagedSegment(remote, environmentID, segmentID, key)
}

func canonicalManagedSegment(
	remote client.Segment,
	environmentID string,
	segmentID string,
	key string,
) (canonicalSegment, error) {
	if remote.IsArchived {
		return canonicalSegment{}, errManagedSegmentArchived
	}
	canonical, err := canonicalizeRemoteSegment(remote)
	if err != nil {
		return canonicalSegment{}, errInvalidSegmentDefinition
	}
	if !client.EqualUUID(canonical.EnvironmentID, environmentID) ||
		!client.EqualUUID(canonical.ID, segmentID) ||
		(key != "" && canonical.Key != key) {
		return canonicalSegment{}, errManagedSegmentIdentity
	}
	if canonical.Type != client.SegmentTypeEnvironmentSpecific ||
		!validEnvironmentSpecificScopes(canonical.Scopes) {
		return canonicalSegment{}, errManagedSegmentShared
	}
	return canonical, nil
}

func (r *segmentResource) reconcileAmbiguousSegmentCreate(
	ctx context.Context,
	environmentID string,
	key string,
	createErr error,
	diagnostics *diag.Diagnostics,
) {
	_, status, resolveErr := r.client.ResolveSegment(
		ctx,
		environmentID,
		client.SegmentIdentity{Key: key},
	)
	if resolveErr != nil {
		diagnostics.AddError(
			"FeatBit Segment Create Outcome Is Unconfirmed",
			"The create result was ambiguous and complete active and archived views could not resolve the exact key. Terraform did not retry or adopt any object. Verify the remote Environment before retrying, then import the intended environment-specific Segment as <environment_uuid>/<segment_uuid> if it exists. "+createErr.Error()+".",
		)
		return
	}
	switch status {
	case client.SegmentStatusAbsent:
		diagnostics.AddError(
			"Unable to Create FeatBit Segment",
			"The create result was ambiguous, but complete active and archived views contain no exact-key match. Terraform did not retry the mutation. "+createErr.Error()+".",
		)
	case client.SegmentStatusActive:
		diagnostics.AddError(
			"FeatBit Segment Create Outcome Requires Recovery",
			"The create result was ambiguous and exactly one active Segment now has the configured key. Terraform did not retry or adopt it. Verify that it is environment-specific, then import it as <environment_uuid>/<segment_uuid> or remove it before retrying.",
		)
	case client.SegmentStatusArchived:
		diagnostics.AddError(
			"FeatBit Segment Create Outcome Requires Archived Recovery",
			"The create result was ambiguous and exactly one archived Segment now has the configured key. Terraform did not retry or adopt it. Verify and permanently remove that object, or restore and import it deliberately before continuing.",
		)
	default:
		diagnostics.AddError(
			"FeatBit Segment Create Outcome Is Unconfirmed",
			"The create result was ambiguous and no authoritative exact-key status was returned. Terraform did not retry or adopt any object.",
		)
	}
}

func expandSegmentCreateRequest(segment canonicalSegment) client.CreateSegmentRequest {
	return client.CreateSegmentRequest{
		Type:        segment.Type,
		Name:        segment.Name,
		Key:         segment.Key,
		Description: segment.Description,
		Scopes:      append([]string(nil), segment.Scopes...),
	}
}

func expandSegmentTargetingRequest(segment canonicalSegment) client.UpdateSegmentTargetingRequest {
	rules := make([]client.SegmentRule, 0, len(segment.Rules))
	for _, rule := range segment.Rules {
		conditions := make([]client.SegmentCondition, 0, len(rule.Conditions))
		for _, condition := range rule.Conditions {
			conditions = append(conditions, client.SegmentCondition{
				ID:       condition.ID,
				Property: condition.Property,
				Operator: condition.Operator,
				Value:    condition.Value,
			})
		}
		rules = append(rules, client.SegmentRule{
			ID:         rule.ID,
			Name:       rule.Name,
			Conditions: conditions,
		})
	}
	return client.UpdateSegmentTargetingRequest{
		Included: append([]string{}, segment.IncludedUsers...),
		Excluded: append([]string{}, segment.ExcludedUsers...),
		Rules:    rules,
	}
}

func segmentPlanDefinitionKnown(plan segmentModel) bool {
	if !knownString(plan.EnvironmentID) || !knownString(plan.Name) ||
		!knownString(plan.Key) || !knownString(plan.Description) ||
		plan.Scopes.IsNull() || plan.Scopes.IsUnknown() ||
		plan.IncludedUsers.IsNull() || plan.IncludedUsers.IsUnknown() ||
		plan.ExcludedUsers.IsNull() || plan.ExcludedUsers.IsUnknown() ||
		plan.Tags.IsNull() || plan.Tags.IsUnknown() {
		return false
	}
	for _, rule := range plan.Rules {
		if !knownString(rule.Name) || len(rule.Conditions) == 0 {
			return false
		}
		for _, condition := range rule.Conditions {
			if !knownString(condition.Property) || !knownString(condition.Operator) ||
				!knownString(condition.Value) {
				return false
			}
		}
	}
	return true
}

func preservePlannedSegmentTargetingIDs(
	planned *canonicalSegment,
	plan segmentModel,
	prior canonicalSegment,
) {
	if planned == nil {
		return
	}
	for ruleIndex := 0; ruleIndex < len(planned.Rules) && ruleIndex < len(prior.Rules); ruleIndex++ {
		if ruleIndex < len(plan.Rules) &&
			(plan.Rules[ruleIndex].ID.IsNull() || plan.Rules[ruleIndex].ID.IsUnknown()) {
			planned.Rules[ruleIndex].ID = prior.Rules[ruleIndex].ID
		}
		for conditionIndex := 0; conditionIndex < len(planned.Rules[ruleIndex].Conditions) &&
			conditionIndex < len(prior.Rules[ruleIndex].Conditions); conditionIndex++ {
			if conditionIndex < len(plan.Rules[ruleIndex].Conditions) &&
				(plan.Rules[ruleIndex].Conditions[conditionIndex].ID.IsNull() ||
					plan.Rules[ruleIndex].Conditions[conditionIndex].ID.IsUnknown()) {
				planned.Rules[ruleIndex].Conditions[conditionIndex].ID =
					prior.Rules[ruleIndex].Conditions[conditionIndex].ID
			}
		}
	}
}

func validCanonicalSegmentTargetingIDs(segment canonicalSegment) bool {
	seenRuleIDs := make(map[string]struct{}, len(segment.Rules))
	seenConditionIDs := make(map[string]struct{})
	for _, rule := range segment.Rules {
		ruleID, valid := client.CanonicalUUID(rule.ID)
		if !valid {
			return false
		}
		if _, duplicate := seenRuleIDs[ruleID]; duplicate {
			return false
		}
		seenRuleIDs[ruleID] = struct{}{}
		for _, condition := range rule.Conditions {
			conditionID, valid := client.CanonicalUUID(condition.ID)
			if !valid {
				return false
			}
			if _, duplicate := seenConditionIDs[conditionID]; duplicate {
				return false
			}
			seenConditionIDs[conditionID] = struct{}{}
		}
	}
	return true
}

func preserveConfiguredSegmentConditionValues(
	canonicalState *segmentModel,
	configured segmentModel,
) {
	if canonicalState == nil || len(canonicalState.Rules) != len(configured.Rules) {
		return
	}
	for ruleIndex := range canonicalState.Rules {
		canonicalRule := &canonicalState.Rules[ruleIndex]
		configuredRule := configured.Rules[ruleIndex]
		if len(canonicalRule.Conditions) != len(configuredRule.Conditions) {
			continue
		}
		for conditionIndex := range canonicalRule.Conditions {
			canonicalCondition := &canonicalRule.Conditions[conditionIndex]
			configuredCondition := configuredRule.Conditions[conditionIndex]
			if !knownString(canonicalCondition.Property) ||
				!knownString(canonicalCondition.Operator) ||
				!knownString(canonicalCondition.Value) ||
				!knownString(configuredCondition.Property) ||
				!knownString(configuredCondition.Operator) ||
				!knownString(configuredCondition.Value) ||
				canonicalCondition.Property.ValueString() != configuredCondition.Property.ValueString() ||
				canonicalCondition.Operator.ValueString() != configuredCondition.Operator.ValueString() {
				continue
			}
			canonicalValue, canonicalErr := canonicalizeSegmentConditionValue(
				canonicalCondition.Operator.ValueString(),
				canonicalCondition.Value.ValueString(),
			)
			configuredValue, configuredErr := canonicalizeSegmentConditionValue(
				configuredCondition.Operator.ValueString(),
				configuredCondition.Value.ValueString(),
			)
			if canonicalErr == nil && configuredErr == nil && canonicalValue == configuredValue {
				canonicalCondition.Value = configuredCondition.Value
			}
		}
	}
}

func segmentStateFromCanonical(segment canonicalSegment, prior segmentModel) segmentModel {
	state := flattenCanonicalSegment(segment)
	if knownString(prior.EnvironmentID) &&
		client.EqualUUID(prior.EnvironmentID.ValueString(), segment.EnvironmentID) {
		state.EnvironmentID = prior.EnvironmentID
	}
	preserveConfiguredSegmentConditionValues(&state, prior)
	return state
}

func sameKnownSegmentIdentity(planned canonicalSegment, prior segmentModel) bool {
	if !knownString(prior.EnvironmentID) || !knownString(prior.Key) ||
		prior.Scopes.IsNull() || prior.Scopes.IsUnknown() {
		return false
	}
	priorScopes, err := terraformStringSet(context.Background(), prior.Scopes)
	return err == nil && client.EqualUUID(planned.EnvironmentID, prior.EnvironmentID.ValueString()) &&
		planned.Key == prior.Key.ValueString() && slices.Equal(planned.Scopes, priorScopes)
}

func sameSegmentReplacementInputs(left canonicalSegment, right canonicalSegment) bool {
	return client.EqualUUID(left.EnvironmentID, right.EnvironmentID) &&
		left.Key == right.Key && slices.Equal(left.Scopes, right.Scopes)
}

func sameSegmentImmutableFields(left canonicalSegment, right canonicalSegment) bool {
	return client.EqualUUID(left.EnvironmentID, right.EnvironmentID) &&
		client.EqualUUID(left.ID, right.ID) && left.Key == right.Key &&
		left.Type == right.Type && slices.Equal(left.Scopes, right.Scopes)
}

func sameSegmentCreateFields(left canonicalSegment, right canonicalSegment) bool {
	return client.EqualUUID(left.EnvironmentID, right.EnvironmentID) &&
		client.EqualUUID(left.ID, right.ID) && left.Name == right.Name &&
		left.Key == right.Key && left.Description == right.Description &&
		left.Type == right.Type && slices.Equal(left.Scopes, right.Scopes)
}

func sameSegmentTargeting(left canonicalSegment, right canonicalSegment) bool {
	if !slices.Equal(left.IncludedUsers, right.IncludedUsers) ||
		!slices.Equal(left.ExcludedUsers, right.ExcludedUsers) ||
		len(left.Rules) != len(right.Rules) {
		return false
	}
	for ruleIndex := range left.Rules {
		leftRule := left.Rules[ruleIndex]
		rightRule := right.Rules[ruleIndex]
		if !client.EqualUUID(leftRule.ID, rightRule.ID) ||
			leftRule.Name != rightRule.Name ||
			len(leftRule.Conditions) != len(rightRule.Conditions) {
			return false
		}
		for conditionIndex := range leftRule.Conditions {
			leftCondition := leftRule.Conditions[conditionIndex]
			rightCondition := rightRule.Conditions[conditionIndex]
			if !client.EqualUUID(leftCondition.ID, rightCondition.ID) ||
				leftCondition.Property != rightCondition.Property ||
				leftCondition.Operator != rightCondition.Operator ||
				leftCondition.Value != rightCondition.Value {
				return false
			}
		}
	}
	return true
}

func sameSegmentTags(left canonicalSegment, right canonicalSegment) bool {
	return slices.Equal(left.Tags, right.Tags)
}

func sameSegmentDefinition(left canonicalSegment, right canonicalSegment) bool {
	return sameSegmentCreateFields(left, right) &&
		sameSegmentTargeting(left, right) && sameSegmentTags(left, right)
}

func cloneCanonicalSegmentRules(rules []canonicalSegmentRule) []canonicalSegmentRule {
	cloned := make([]canonicalSegmentRule, len(rules))
	for index, rule := range rules {
		cloned[index] = canonicalSegmentRule{
			ID:         rule.ID,
			Name:       rule.Name,
			Conditions: append([]canonicalSegmentCondition(nil), rule.Conditions...),
		}
	}
	return cloned
}

func segmentMutationNeedsReconciliation(err error) bool {
	if mutationOutcomeAmbiguous(err) {
		return true
	}
	switch client.Classify(0, nil, err) {
	case client.ClassificationConflict, client.ClassificationNotFoundUnconfirmed:
		return true
	default:
		return false
	}
}

func (r *segmentResource) segmentLocks() *keyedLockManager {
	r.lockOnce.Do(func() {
		if r.locks == nil {
			r.locks = newKeyedLockManager()
		}
	})
	return r.locks
}

func segmentCreateLockKey(environmentID string, key string) string {
	canonicalEnvironmentID, valid := client.CanonicalUUID(environmentID)
	if !valid {
		canonicalEnvironmentID = environmentID
	}
	return canonicalEnvironmentID + "\x00" + key
}

func segmentWriteLockKey(environmentID string, segmentID string) string {
	canonicalEnvironmentID, valid := client.CanonicalUUID(environmentID)
	if !valid {
		canonicalEnvironmentID = environmentID
	}
	canonicalSegmentID, valid := client.CanonicalUUID(segmentID)
	if !valid {
		canonicalSegmentID = segmentID
	}
	return "segment\x00" + canonicalEnvironmentID + "\x00" + canonicalSegmentID
}
