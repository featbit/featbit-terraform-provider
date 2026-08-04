// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// segmentResourceSchema defines the environment-specific-only resource
// contract. Shared Segment mutation is structurally impossible because type
// is a computed observation rather than a configurable input.
func segmentResourceSchema() resourceschema.Schema {
	return resourceschema.Schema{
		MarkdownDescription: "Manages an environment-specific FeatBit Segment through the documented public API.",
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
				MarkdownDescription: "Segment UUID.",
				PlanModifiers: []planmodifier.String{
					useStateForUnknownIfAttributeValuesUnchanged(
						stableIDStringIdentity(path.Root("environment_id")),
						stableIDStringIdentity(path.Root("key")),
						stableIDSetIdentity(path.Root("scopes")),
					),
				},
			},
			"name": resourceschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Segment display name, updated in place.",
				Validators: []validator.String{
					segmentNameValidator{},
				},
			},
			"key": resourceschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Immutable exact case-sensitive Segment key within the Environment.",
				Validators: []validator.String{
					segmentKeyValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": resourceschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Segment description, canonicalized to an empty string when omitted and updated in place.",
				Default:             stringdefault.StaticString(""),
			},
			"type": resourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Fixed Segment type. Managed resources are always `environment-specific`.",
			},
			"scopes": resourceschema.SetAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Exactly one immutable, fully qualified Environment resource RN.",
				Validators: []validator.Set{
					segmentEnvironmentScopesValidator{},
				},
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
				},
			},
			"included_users": resourceschema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Exact user keys explicitly included in the Segment. Ordering is not significant.",
				Default:             setdefault.StaticValue(emptySegmentStringSet()),
				Validators: []validator.Set{
					segmentIncludedUsersValidator{},
				},
			},
			"excluded_users": resourceschema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Exact user keys explicitly excluded from the Segment. Ordering is not significant.",
				Default:             setdefault.StaticValue(emptySegmentStringSet()),
			},
			"rules": segmentResourceRulesAttribute(),
			"tags": resourceschema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Segment tags. Ordering is not significant.",
				Default:             setdefault.StaticValue(emptySegmentStringSet()),
			},
		},
	}
}

func segmentResourceRulesAttribute() resourceschema.ListNestedAttribute {
	return resourceschema.ListNestedAttribute{
		Optional:            true,
		Computed:            true,
		MarkdownDescription: "Ordered Segment rules. Provider-created rule and condition UUIDs are deterministic.",
		Default:             listdefault.StaticValue(emptySegmentRulesList()),
		Validators: []validator.List{
			segmentRulesValidator{},
		},
		NestedObject: resourceschema.NestedAttributeObject{
			Attributes: map[string]resourceschema.Attribute{
				"id": resourceschema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Stable rule UUID.",
				},
				"name": resourceschema.StringAttribute{
					Required:            true,
					MarkdownDescription: "Rule display name.",
				},
				"conditions": resourceschema.ListNestedAttribute{
					Required:            true,
					MarkdownDescription: "Ordered conditions combined with logical AND.",
					NestedObject: resourceschema.NestedAttributeObject{
						Attributes: map[string]resourceschema.Attribute{
							"id": resourceschema.StringAttribute{
								Computed:            true,
								MarkdownDescription: "Stable condition UUID.",
							},
							"property": resourceschema.StringAttribute{
								Required:            true,
								MarkdownDescription: "Exact user property name.",
								Validators: []validator.String{
									segmentConditionPropertyValidator{},
								},
							},
							"operator": resourceschema.StringAttribute{
								Required:            true,
								MarkdownDescription: "Exact documented Segment condition operator.",
								Validators: []validator.String{
									segmentOperatorValidator{},
								},
							},
							"value": resourceschema.StringAttribute{
								Required: true,
								MarkdownDescription: "Operator value. Multi-value operators use a JSON string array; " +
									"IsTrue and IsFalse require an empty string.",
							},
						},
					},
				},
			},
		},
	}
}

func segmentConditionObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"id":       types.StringType,
		"property": types.StringType,
		"operator": types.StringType,
		"value":    types.StringType,
	}}
}

func segmentRuleObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"id":         types.StringType,
		"name":       types.StringType,
		"conditions": types.ListType{ElemType: segmentConditionObjectType()},
	}}
}

func emptySegmentStringSet() types.Set {
	return types.SetValueMust(types.StringType, nil)
}

func emptySegmentRulesList() types.List {
	return types.ListValueMust(segmentRuleObjectType(), nil)
}

type segmentNameValidator struct{}

var _ validator.String = segmentNameValidator{}

func (segmentNameValidator) Description(context.Context) string {
	return "must be non-blank and at most 128 characters"
}

func (v segmentNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (segmentNameValidator) ValidateString(
	_ context.Context,
	req validator.StringRequest,
	resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !validSegmentName(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Segment Name",
			"The name must be non-blank and at most 128 characters.",
		)
	}
}

type segmentKeyValidator struct{}

var _ validator.String = segmentKeyValidator{}

func (segmentKeyValidator) Description(context.Context) string {
	return "must be 1 through 128 ASCII letters, digits, periods, underscores, or hyphens"
}

func (v segmentKeyValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (segmentKeyValidator) ValidateString(
	_ context.Context,
	req validator.StringRequest,
	resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !validSegmentKey(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Segment Key",
			"The key must be non-empty, at most 128 characters, and contain only ASCII letters, digits, periods, underscores, or hyphens.",
		)
	}
}

type segmentEnvironmentScopesValidator struct{}

var _ validator.Set = segmentEnvironmentScopesValidator{}

func (segmentEnvironmentScopesValidator) Description(context.Context) string {
	return "must contain exactly one fully qualified Environment resource RN"
}

func (v segmentEnvironmentScopesValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (segmentEnvironmentScopesValidator) ValidateSet(
	ctx context.Context,
	req validator.SetRequest,
	resp *validator.SetResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	values, err := terraformStringSet(ctx, req.ConfigValue)
	if err != nil || !validEnvironmentSpecificScopes(values) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Segment Scopes",
			"Managed Segments require exactly one fully qualified Environment resource RN without wildcards.",
		)
	}
}

type segmentConditionPropertyValidator struct{}

var _ validator.String = segmentConditionPropertyValidator{}

func (segmentConditionPropertyValidator) Description(context.Context) string {
	return "must be a non-empty user property and cannot reference another Segment"
}

func (v segmentConditionPropertyValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (segmentConditionPropertyValidator) ValidateString(
	_ context.Context,
	req validator.StringRequest,
	resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !validSegmentConditionProperty(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Segment Condition Property",
			"The condition property must be non-empty. Segment-to-Segment conditions are not supported by this resource.",
		)
	}
}

type segmentOperatorValidator struct{}

var _ validator.String = segmentOperatorValidator{}

func (segmentOperatorValidator) Description(context.Context) string {
	return "must be an exact documented Segment condition operator"
}

func (v segmentOperatorValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (segmentOperatorValidator) ValidateString(
	_ context.Context,
	req validator.StringRequest,
	resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !validSegmentOperator(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Segment Condition Operator",
			"The operator must use one of the exact documented Segment operator spellings.",
		)
	}
}

type segmentRulesValidator struct{}

var _ validator.List = segmentRulesValidator{}

func (segmentRulesValidator) Description(context.Context) string {
	return "must contain ordered rules with at least one valid ordered condition each"
}

func (v segmentRulesValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (segmentRulesValidator) ValidateList(
	ctx context.Context,
	req validator.ListRequest,
	resp *validator.ListResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	var rules []segmentRuleModel
	resp.Diagnostics.Append(req.ConfigValue.ElementsAs(ctx, &rules, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, rule := range rules {
		if rule.Name.IsNull() || rule.Name.IsUnknown() || len(rule.Conditions) == 0 {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Invalid Segment Rules",
				"Every configured rule must have a known name and at least one ordered condition.",
			)
			return
		}
		for _, condition := range rule.Conditions {
			if !knownString(condition.Property) || !knownString(condition.Operator) ||
				!knownString(condition.Value) {
				return
			}
			if !validSegmentConditionProperty(condition.Property.ValueString()) ||
				!validSegmentOperator(condition.Operator.ValueString()) {
				return
			}
			if _, err := canonicalizeSegmentConditionValue(
				condition.Operator.ValueString(),
				condition.Value.ValueString(),
			); err != nil {
				resp.Diagnostics.AddAttributeError(
					req.Path,
					"Invalid Segment Condition Value",
					"A condition value does not use the required encoding for its operator.",
				)
				return
			}
		}
	}
}

type segmentIncludedUsersValidator struct{}

var _ validator.Set = segmentIncludedUsersValidator{}

func (segmentIncludedUsersValidator) Description(context.Context) string {
	return "must not overlap excluded_users"
}

func (v segmentIncludedUsersValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (segmentIncludedUsersValidator) ValidateSet(
	ctx context.Context,
	req validator.SetRequest,
	resp *validator.SetResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	var excluded types.Set
	resp.Diagnostics.Append(
		req.Config.GetAttribute(ctx, path.Root("excluded_users"), &excluded)...,
	)
	if resp.Diagnostics.HasError() || excluded.IsNull() || excluded.IsUnknown() {
		return
	}
	includedValues, includedErr := terraformStringSet(ctx, req.ConfigValue)
	excludedValues, excludedErr := terraformStringSet(ctx, excluded)
	if includedErr != nil || excludedErr != nil {
		return
	}
	if stringSetsIntersect(includedValues, excludedValues) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Conflicting Segment User Sets",
			"The same exact user key cannot be both included and excluded.",
		)
	}
}
