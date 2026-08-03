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
	return stableIDPlanModifier{identityInputs: identityInputs}
}

type stableIDPlanModifier struct {
	identityInputs []path.Path
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

	for _, inputPath := range m.identityInputs {
		var planned types.String
		var prior types.String
		resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, inputPath, &planned)...)
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, inputPath, &prior)...)
		if resp.Diagnostics.HasError() || planned.IsNull() || planned.IsUnknown() ||
			prior.IsNull() || prior.IsUnknown() || !planned.Equal(prior) {
			return
		}
	}

	resp.PlanValue = req.StateValue
}
