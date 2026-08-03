// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func featureFlagResourceSchema() resourceschema.Schema {
	return resourceschema.Schema{
		MarkdownDescription: "Manages a FeatBit Feature Flag definition through the documented public API.",
		Attributes: map[string]resourceschema.Attribute{
			"environment_id": resourceschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Immutable parent Environment UUID.",
				Validators: []validator.String{
					uuidValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"id": resourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Feature Flag UUID.",
				PlanModifiers: []planmodifier.String{
					useStateForUnknownIfAttributeValuesUnchanged(
						stableIDStringIdentity(path.Root("environment_id")),
						stableIDStringIdentity(path.Root("key")),
						stableIDStringIdentity(path.Root("variation_type")),
						stableIDStringIdentity(path.Root("description")),
						stableIDListIdentity(path.Root("variations")),
					),
				},
			},
			"name": resourceschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Feature Flag display name. This is the only field updated in place.",
				Validators: []validator.String{
					featureFlagNameValidator{},
				},
			},
			"key": resourceschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Immutable exact Feature Flag key within the parent Environment.",
				Validators: []validator.String{
					featureFlagKeyValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": resourceschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Immutable Feature Flag description. An omitted value canonicalizes to an empty string.",
				Default:             stringdefault.StaticString(""),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"variation_type": resourceschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Immutable variation type: boolean, string, number, or json.",
				Validators: []validator.String{
					featureFlagVariationTypeValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"variations": featureFlagResourceVariationsAttribute(),
		},
	}
}

func featureFlagResourceVariationsAttribute() resourceschema.ListNestedAttribute {
	return resourceschema.ListNestedAttribute{
		Required:            true,
		MarkdownDescription: "Immutable ordered Feature Flag variations. IDs are computed deterministically for provider-created flags.",
		Validators: []validator.List{
			featureFlagVariationsValidator{},
		},
		PlanModifiers: []planmodifier.List{
			featureFlagVariationsRequiresReplace{},
		},
		NestedObject: resourceschema.NestedAttributeObject{
			Attributes: map[string]resourceschema.Attribute{
				"id": resourceschema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Stable variation UUID.",
				},
				"name": resourceschema.StringAttribute{
					Required:            true,
					MarkdownDescription: "Variation display name.",
					Validators: []validator.String{
						featureFlagVariationNameValidator{},
					},
				},
				"value": resourceschema.StringAttribute{
					Required:            true,
					MarkdownDescription: "Variation value, canonicalized according to variation_type.",
					Validators: []validator.String{
						featureFlagVariationValueValidator{},
					},
				},
			},
		},
	}
}

// featureFlagVariationsRequiresReplace compares the user-owned semantic
// variation definition. Computed UUIDs and equivalent type-aware spellings do
// not cause replacement; ModifyPlan canonicalizes them afterwards.
type featureFlagVariationsRequiresReplace struct{}

var _ planmodifier.List = featureFlagVariationsRequiresReplace{}

func (featureFlagVariationsRequiresReplace) Description(context.Context) string {
	return "Requires replacement when the canonical variation names or values change."
}

func (m featureFlagVariationsRequiresReplace) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (featureFlagVariationsRequiresReplace) PlanModifyList(
	ctx context.Context,
	req planmodifier.ListRequest,
	resp *planmodifier.ListResponse,
) {
	if req.State.Raw.IsNull() || req.PlanValue.IsNull() || req.PlanValue.IsUnknown() ||
		req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}

	var plannedType types.String
	var priorType types.String
	resp.Diagnostics.Append(
		req.Plan.GetAttribute(ctx, path.Root("variation_type"), &plannedType)...,
	)
	resp.Diagnostics.Append(
		req.State.GetAttribute(ctx, path.Root("variation_type"), &priorType)...,
	)
	if resp.Diagnostics.HasError() || plannedType.IsNull() || plannedType.IsUnknown() ||
		priorType.IsNull() || priorType.IsUnknown() {
		return
	}
	canonicalPlannedType, err := canonicalizeFeatureFlagVariationType(plannedType.ValueString())
	if err != nil {
		return
	}
	canonicalPriorType, err := canonicalizeFeatureFlagVariationType(priorType.ValueString())
	if err != nil || canonicalPlannedType != canonicalPriorType {
		// variation_type owns replacement when the type changes.
		return
	}

	var planned []featureFlagVariationModel
	var prior []featureFlagVariationModel
	resp.Diagnostics.Append(req.PlanValue.ElementsAs(ctx, &planned, false)...)
	resp.Diagnostics.Append(req.StateValue.ElementsAs(ctx, &prior, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(planned) != len(prior) {
		resp.RequiresReplace = true
		return
	}
	for index := range planned {
		if planned[index].Name.IsNull() || planned[index].Name.IsUnknown() ||
			planned[index].Value.IsNull() || planned[index].Value.IsUnknown() ||
			prior[index].Name.IsNull() || prior[index].Name.IsUnknown() ||
			prior[index].Value.IsNull() || prior[index].Value.IsUnknown() {
			return
		}
		plannedValue, plannedErr := canonicalizeFeatureFlagValue(
			canonicalPlannedType,
			planned[index].Value.ValueString(),
		)
		priorValue, priorErr := canonicalizeFeatureFlagValue(
			canonicalPriorType,
			prior[index].Value.ValueString(),
		)
		if plannedErr != nil || priorErr != nil {
			return
		}
		if planned[index].Name.ValueString() != prior[index].Name.ValueString() ||
			plannedValue != priorValue {
			resp.RequiresReplace = true
			return
		}
	}
}

type featureFlagKeyValidator struct{}

var _ validator.String = featureFlagKeyValidator{}

func (featureFlagKeyValidator) Description(context.Context) string {
	return "must be 1 through 128 ASCII letters, digits, periods, underscores, or hyphens"
}

func (v featureFlagKeyValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (featureFlagKeyValidator) ValidateString(
	_ context.Context,
	req validator.StringRequest,
	resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !validFeatureFlagKey(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Feature Flag Key",
			"The key must be non-empty, at most 128 characters, and contain only ASCII letters, digits, periods, underscores, or hyphens.",
		)
	}
}

type featureFlagNameValidator struct{}

var _ validator.String = featureFlagNameValidator{}

func (featureFlagNameValidator) Description(context.Context) string {
	return "must be non-blank and at most 128 characters"
}

func (v featureFlagNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (featureFlagNameValidator) ValidateString(
	_ context.Context,
	req validator.StringRequest,
	resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !validFeatureFlagName(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Feature Flag Name",
			"The name must be non-blank and at most 128 characters.",
		)
	}
}

type featureFlagVariationTypeValidator struct{}

var _ validator.String = featureFlagVariationTypeValidator{}

func (featureFlagVariationTypeValidator) Description(context.Context) string {
	return "must be exactly boolean, string, number, or json"
}

func (v featureFlagVariationTypeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (featureFlagVariationTypeValidator) ValidateString(
	_ context.Context,
	req validator.StringRequest,
	resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := canonicalizeFeatureFlagVariationType(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Feature Flag Variation Type",
			"The variation type must be exactly one of: boolean, string, number, or json.",
		)
	}
}

type featureFlagVariationNameValidator struct{}

var _ validator.String = featureFlagVariationNameValidator{}

func (featureFlagVariationNameValidator) Description(context.Context) string {
	return "must be non-blank"
}

func (v featureFlagVariationNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (featureFlagVariationNameValidator) ValidateString(
	_ context.Context,
	req validator.StringRequest,
	resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !validFeatureFlagVariationName(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Feature Flag Variation Name",
			"Each variation name must be non-blank.",
		)
	}
}

type featureFlagVariationValueValidator struct{}

var _ validator.String = featureFlagVariationValueValidator{}

func (featureFlagVariationValueValidator) Description(context.Context) string {
	return "must be non-empty"
}

func (v featureFlagVariationValueValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (featureFlagVariationValueValidator) ValidateString(
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
			"Invalid Feature Flag Variation Value",
			"Each variation value must be non-empty.",
		)
	}
}

type featureFlagVariationsValidator struct{}

var _ validator.List = featureFlagVariationsValidator{}

func (featureFlagVariationsValidator) Description(context.Context) string {
	return "must contain at least one variation whose value is valid for variation_type"
}

func (v featureFlagVariationsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (featureFlagVariationsValidator) ValidateList(
	ctx context.Context,
	req validator.ListRequest,
	resp *validator.ListResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if len(req.ConfigValue.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Feature Flag Variations",
			"At least one variation is required.",
		)
		return
	}

	var variationType types.String
	resp.Diagnostics.Append(
		req.Config.GetAttribute(ctx, path.Root("variation_type"), &variationType)...,
	)
	if resp.Diagnostics.HasError() || variationType.IsNull() || variationType.IsUnknown() {
		return
	}
	canonicalType, err := canonicalizeFeatureFlagVariationType(variationType.ValueString())
	if err != nil {
		return
	}

	var variations []featureFlagVariationModel
	resp.Diagnostics.Append(req.ConfigValue.ElementsAs(ctx, &variations, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, variation := range variations {
		if variation.Name.IsNull() || variation.Name.IsUnknown() ||
			variation.Value.IsNull() || variation.Value.IsUnknown() {
			return
		}
		if !validFeatureFlagVariationName(variation.Name.ValueString()) ||
			variation.Value.ValueString() == "" {
			return
		}
		if _, err := canonicalizeFeatureFlagValue(
			canonicalType,
			variation.Value.ValueString(),
		); err != nil {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Invalid Feature Flag Variation Value",
				"A variation value is invalid for the configured variation type.",
			)
			return
		}
	}
}
