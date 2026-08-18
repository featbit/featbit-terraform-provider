// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// nonEmptyStringValidator is shared only by resource fields whose public
// contract permits every non-empty string. More restrictive keys and values
// retain their endpoint-specific validators.
type nonEmptyStringValidator struct {
	object string
	field  string
}

var _ validator.String = nonEmptyStringValidator{}

func (nonEmptyStringValidator) Description(context.Context) string {
	return "must be non-empty"
}

func (v nonEmptyStringValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v nonEmptyStringValidator) ValidateString(
	_ context.Context,
	req validator.StringRequest,
	resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid FeatBit "+v.object+" "+v.field,
			"The "+v.object+" "+v.field+" must be non-empty.",
		)
	}
}
