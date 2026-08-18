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

func policyResourceSchema() resourceschema.Schema {
	return resourceschema.Schema{
		MarkdownDescription: "Manages one custom FeatBit Policy and its complete statement set through the documented public API.",
		Attributes: map[string]resourceschema.Attribute{
			"id": resourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Policy UUID.",
				PlanModifiers: []planmodifier.String{
					useStateForUnknownIfUnchanged(path.Root("key")),
				},
			},
			"name": resourceschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Policy display name.",
				Validators: []validator.String{
					nonEmptyStringValidator{object: "Policy", field: "name"},
				},
			},
			"key": resourceschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Immutable organization-scoped exact Policy key.",
				Validators: []validator.String{
					nonEmptyStringValidator{object: "Policy", field: "key"},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": resourceschema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Policy description. An omitted value canonicalizes to an empty string.",
				Default:             stringdefault.StaticString(""),
			},
			"type": resourceschema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Policy ownership type. Managed resources are always CustomerManaged.",
			},
			"statements": policyResourceStatementsAttribute(),
		},
	}
}

func policyResourceStatementsAttribute() resourceschema.SetNestedAttribute {
	return resourceschema.SetNestedAttribute{
		Required: true,
		MarkdownDescription: "Complete unordered Policy statement set. An empty set removes every statement. " +
			"Statement actions and selectors are validated against the managed IAM v1 catalog.",
		Validators: []validator.Set{
			policyStatementsValidator{},
		},
		NestedObject: resourceschema.NestedAttributeObject{
			Attributes: map[string]resourceschema.Attribute{
				"resource_type": resourceschema.StringAttribute{
					Required:            true,
					MarkdownDescription: "Exact lowercase control level: project, env, flag, or segment.",
					Validators: []validator.String{
						policyResourceTypeValidator{},
					},
				},
				"effect": resourceschema.StringAttribute{
					Required:            true,
					MarkdownDescription: "Exact lowercase effect: allow or deny.",
					Validators: []validator.String{
						policyEffectValidator{},
					},
				},
				"actions": resourceschema.SetAttribute{
					Required:            true,
					ElementType:         types.StringType,
					MarkdownDescription: "Non-empty set of exact case-sensitive actions valid for resource_type.",
					Validators: []validator.Set{
						policyNonEmptyStringSetValidator{field: "actions"},
					},
				},
				"resources": resourceschema.SetAttribute{
					Required:            true,
					ElementType:         types.StringType,
					MarkdownDescription: "Non-empty set of wildcard or exact-key selectors matching resource_type.",
					Validators: []validator.Set{
						policyNonEmptyStringSetValidator{field: "resources"},
					},
				},
			},
		},
	}
}

type policyResourceTypeValidator struct{}

var _ validator.String = policyResourceTypeValidator{}

func (policyResourceTypeValidator) Description(context.Context) string {
	return "must be exactly project, env, flag, or segment"
}

func (v policyResourceTypeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (policyResourceTypeValidator) ValidateString(
	_ context.Context,
	req validator.StringRequest,
	resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, exists := policyActions[req.ConfigValue.ValueString()]; !exists {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid FeatBit Policy Resource Type",
			"The resource type must be exactly one of: project, env, flag, or segment.",
		)
	}
}

type policyEffectValidator struct{}

var _ validator.String = policyEffectValidator{}

func (policyEffectValidator) Description(context.Context) string {
	return "must be exactly allow or deny"
}

func (v policyEffectValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (policyEffectValidator) ValidateString(
	_ context.Context,
	req validator.StringRequest,
	resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	value := req.ConfigValue.ValueString()
	if value != policyEffectAllow && value != policyEffectDeny {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid FeatBit Policy Effect",
			"The effect must be exactly allow or deny.",
		)
	}
}

type policyNonEmptyStringSetValidator struct {
	field string
}

var _ validator.Set = policyNonEmptyStringSetValidator{}

func (v policyNonEmptyStringSetValidator) Description(context.Context) string {
	return "must contain at least one non-empty string"
}

func (v policyNonEmptyStringSetValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v policyNonEmptyStringSetValidator) ValidateSet(
	ctx context.Context,
	req validator.SetRequest,
	resp *validator.SetResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	values, err := terraformStringSet(ctx, req.ConfigValue)
	if err != nil {
		return
	}
	if len(values) == 0 {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid FeatBit Policy "+v.field,
			"Each Policy statement must contain at least one "+v.field+" value.",
		)
		return
	}
	for _, value := range values {
		if value == "" {
			resp.Diagnostics.AddAttributeError(
				req.Path,
				"Invalid FeatBit Policy "+v.field,
				"Policy statement "+v.field+" values must be non-empty.",
			)
			return
		}
	}
}

type policyStatementsValidator struct{}

var _ validator.Set = policyStatementsValidator{}

func (policyStatementsValidator) Description(context.Context) string {
	return "must satisfy the managed FeatBit IAM statement catalog"
}

func (v policyStatementsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (policyStatementsValidator) ValidateSet(
	ctx context.Context,
	req validator.SetRequest,
	resp *validator.SetResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	models, err := terraformPolicyStatementModels(ctx, req.ConfigValue)
	if err != nil {
		return
	}
	for _, model := range models {
		if !knownPolicyStatementModel(model) {
			return
		}
	}
	if _, err := canonicalizeTerraformPolicyStatements(ctx, req.ConfigValue); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid FeatBit Policy Statements",
			"Every statement must use a supported resource type, effect, exact action catalog, and canonicalizable selector shape for that resource type.",
		)
	}
}
