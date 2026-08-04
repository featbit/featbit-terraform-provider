// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// useStateForUnknownIfUnchanged keeps an immutable computed identifier known
// during in-place updates, while leaving it unknown when any identity-defining
// input changes and the resource must be replaced.
func useStateForUnknownIfUnchanged(
	identityInputs ...path.Path,
) planmodifier.String {
	inputs := make([]stableIDIdentityInput, 0, len(identityInputs))
	for _, inputPath := range identityInputs {
		inputs = append(inputs, stableIDStringIdentity(inputPath))
	}
	return useStateForUnknownIfAttributeValuesUnchanged(inputs...)
}

type stableIDIdentityKind uint8

const (
	stableIDIdentityString stableIDIdentityKind = iota
	stableIDIdentityList
	stableIDIdentitySet
)

type stableIDIdentityInput struct {
	path path.Path
	kind stableIDIdentityKind
}

func stableIDStringIdentity(inputPath path.Path) stableIDIdentityInput {
	return stableIDIdentityInput{path: inputPath, kind: stableIDIdentityString}
}

func stableIDListIdentity(inputPath path.Path) stableIDIdentityInput {
	return stableIDIdentityInput{path: inputPath, kind: stableIDIdentityList}
}

func stableIDSetIdentity(inputPath path.Path) stableIDIdentityInput {
	return stableIDIdentityInput{path: inputPath, kind: stableIDIdentitySet}
}

func useStateForUnknownIfAttributeValuesUnchanged(
	identityInputs ...stableIDIdentityInput,
) planmodifier.String {
	return stableIDPlanModifier{identityInputs: identityInputs}
}

type stableIDPlanModifier struct {
	identityInputs []stableIDIdentityInput
}

func (m stableIDPlanModifier) Description(context.Context) string {
	return "Keeps the prior identifier known only for in-place updates."
}

func (m stableIDPlanModifier) MarkdownDescription(context.Context) string {
	return "Keeps the prior identifier known only for in-place updates."
}

func (m stableIDPlanModifier) PlanModifyString(
	ctx context.Context,
	req planmodifier.StringRequest,
	resp *planmodifier.StringResponse,
) {
	if req.State.Raw.IsNull() || !req.PlanValue.IsUnknown() ||
		req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}

	for _, input := range m.identityInputs {
		switch input.kind {
		case stableIDIdentityString:
			var planned types.String
			var prior types.String
			resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, input.path, &planned)...)
			resp.Diagnostics.Append(req.State.GetAttribute(ctx, input.path, &prior)...)
			if resp.Diagnostics.HasError() || planned.IsNull() || planned.IsUnknown() ||
				prior.IsNull() || prior.IsUnknown() || !planned.Equal(prior) {
				return
			}
		case stableIDIdentityList:
			var planned types.List
			var prior types.List
			resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, input.path, &planned)...)
			resp.Diagnostics.Append(req.State.GetAttribute(ctx, input.path, &prior)...)
			if resp.Diagnostics.HasError() || planned.IsNull() || planned.IsUnknown() ||
				prior.IsNull() || prior.IsUnknown() || !planned.Equal(prior) {
				return
			}
		case stableIDIdentitySet:
			var planned types.Set
			var prior types.Set
			resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, input.path, &planned)...)
			resp.Diagnostics.Append(req.State.GetAttribute(ctx, input.path, &prior)...)
			if resp.Diagnostics.HasError() || planned.IsNull() || planned.IsUnknown() ||
				prior.IsNull() || prior.IsUnknown() || !planned.Equal(prior) {
				return
			}
		default:
			return
		}
	}

	resp.PlanValue = req.StateValue
}
