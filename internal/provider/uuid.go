// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

const uuidDescription = "must be a UUID in 8-4-4-4-12 hexadecimal form"

type uuidValidator struct{}

var _ validator.String = uuidValidator{}

func (uuidValidator) Description(context.Context) string {
	return uuidDescription
}

func (v uuidValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (uuidValidator) ValidateString(
	_ context.Context,
	req validator.StringRequest,
	resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !validUUID(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid UUID",
			"The value must be a UUID in 8-4-4-4-12 hexadecimal form.",
		)
	}
}

func validUUID(value string) bool {
	return client.ValidUUID(value)
}
