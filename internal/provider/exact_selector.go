// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// validateExactlyOneStringSelector enforces the shared data-source contract
// that one of two Optional+Computed string attributes is configured. Unknown
// values count as configured so references can be resolved by Terraform before
// Read without requiring both selectors to be known during validation.
func validateExactlyOneStringSelector(
	ctx context.Context,
	config tfsdk.Config,
	firstPath path.Path,
	secondPath path.Path,
	title string,
	detail string,
) diag.Diagnostics {
	var diagnostics diag.Diagnostics
	var first types.String
	var second types.String
	diagnostics.Append(config.GetAttribute(ctx, firstPath, &first)...)
	diagnostics.Append(config.GetAttribute(ctx, secondPath, &second)...)
	if diagnostics.HasError() {
		return diagnostics
	}

	configured := 0
	if !first.IsNull() {
		configured++
	}
	if !second.IsNull() {
		configured++
	}
	if configured != 1 {
		diagnostics.AddError(title, detail)
	}
	return diagnostics
}
