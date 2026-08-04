// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSegmentResourceSchemaFreezesOwnershipReplacementAndDefaults(t *testing.T) {
	t.Parallel()

	segmentSchema := segmentResourceSchema()
	if diagnostics := segmentSchema.ValidateImplementation(context.Background()); diagnostics.HasError() {
		t.Fatalf("Segment resource schema implementation diagnostics = %v", diagnostics)
	}
	if got := len(segmentSchema.Attributes); got != 11 {
		t.Fatalf("attribute count = %d, want 11", got)
	}

	id, ok := segmentSchema.Attributes["id"].(resourceschema.StringAttribute)
	if !ok || !id.Computed || id.Required || id.Optional || len(id.PlanModifiers) != 1 {
		t.Fatalf("id attribute = %#v", segmentSchema.Attributes["id"])
	}
	segmentType, ok := segmentSchema.Attributes["type"].(resourceschema.StringAttribute)
	if !ok || !segmentType.Computed || segmentType.Required || segmentType.Optional ||
		len(segmentType.PlanModifiers) != 0 {
		t.Fatalf("type attribute = %#v", segmentSchema.Attributes["type"])
	}
	name, ok := segmentSchema.Attributes["name"].(resourceschema.StringAttribute)
	if !ok || !name.Required || name.Optional || name.Computed ||
		len(name.Validators) != 1 || len(name.PlanModifiers) != 0 {
		t.Fatalf("name attribute = %#v", segmentSchema.Attributes["name"])
	}
	for _, attributeName := range []string{"environment_id", "key"} {
		attribute, ok := segmentSchema.Attributes[attributeName].(resourceschema.StringAttribute)
		if !ok || !attribute.Required || attribute.Optional || attribute.Computed ||
			len(attribute.Validators) != 1 || len(attribute.PlanModifiers) != 1 {
			t.Fatalf("%s attribute = %#v", attributeName, segmentSchema.Attributes[attributeName])
		}
	}
	description, ok := segmentSchema.Attributes["description"].(resourceschema.StringAttribute)
	if !ok || !description.Optional || !description.Computed || description.Required ||
		description.Default == nil || len(description.PlanModifiers) != 0 {
		t.Fatalf("description attribute = %#v", segmentSchema.Attributes["description"])
	}
	scopes, ok := segmentSchema.Attributes["scopes"].(resourceschema.SetAttribute)
	if !ok || !scopes.Required || scopes.Optional || scopes.Computed ||
		len(scopes.Validators) != 1 || len(scopes.PlanModifiers) != 1 {
		t.Fatalf("scopes attribute = %#v", segmentSchema.Attributes["scopes"])
	}
	for _, attributeName := range []string{"included_users", "excluded_users", "tags"} {
		attribute, ok := segmentSchema.Attributes[attributeName].(resourceschema.SetAttribute)
		if !ok || !attribute.Optional || !attribute.Computed || attribute.Required ||
			attribute.Default == nil || len(attribute.PlanModifiers) != 0 {
			t.Fatalf("%s attribute = %#v", attributeName, segmentSchema.Attributes[attributeName])
		}
	}
	rules, ok := segmentSchema.Attributes["rules"].(resourceschema.ListNestedAttribute)
	if !ok || !rules.Optional || !rules.Computed || rules.Required ||
		rules.Default == nil || len(rules.Validators) != 1 ||
		len(rules.PlanModifiers) != 0 || len(rules.NestedObject.Attributes) != 3 {
		t.Fatalf("rules attribute = %#v", segmentSchema.Attributes["rules"])
	}
	ruleID, ok := rules.NestedObject.Attributes["id"].(resourceschema.StringAttribute)
	if !ok || !ruleID.Computed || ruleID.Required || ruleID.Optional {
		t.Fatalf("rule id attribute = %#v", rules.NestedObject.Attributes["id"])
	}
	ruleName, ok := rules.NestedObject.Attributes["name"].(resourceschema.StringAttribute)
	if !ok || !ruleName.Required || ruleName.Optional || ruleName.Computed {
		t.Fatalf("rule name attribute = %#v", rules.NestedObject.Attributes["name"])
	}
	conditions, ok := rules.NestedObject.Attributes["conditions"].(resourceschema.ListNestedAttribute)
	if !ok || !conditions.Required || conditions.Optional || conditions.Computed ||
		len(conditions.NestedObject.Attributes) != 4 {
		t.Fatalf("conditions attribute = %#v", rules.NestedObject.Attributes["conditions"])
	}
	conditionID, ok := conditions.NestedObject.Attributes["id"].(resourceschema.StringAttribute)
	if !ok || !conditionID.Computed || conditionID.Required || conditionID.Optional {
		t.Fatalf("condition id attribute = %#v", conditions.NestedObject.Attributes["id"])
	}
	for _, attributeName := range []string{"property", "operator", "value"} {
		attribute, ok := conditions.NestedObject.Attributes[attributeName].(resourceschema.StringAttribute)
		if !ok || !attribute.Required || attribute.Optional || attribute.Computed {
			t.Fatalf("condition %s attribute = %#v", attributeName, conditions.NestedObject.Attributes[attributeName])
		}
	}
	if operator := conditions.NestedObject.Attributes["operator"].(resourceschema.StringAttribute); len(operator.Validators) != 1 {
		t.Fatal("condition operator schema omitted its exact-spelling validator")
	}
	if property := conditions.NestedObject.Attributes["property"].(resourceschema.StringAttribute); len(property.Validators) != 1 {
		t.Fatal("condition property schema omitted its Segment-reference guard")
	}
}

func TestSegmentResourceSchemaKeepsIDOnlyForInPlaceChanges(t *testing.T) {
	t.Parallel()

	segmentSchema := segmentResourceSchema()
	id := segmentSchema.Attributes["id"].(resourceschema.StringAttribute)
	stateModel := providerSegmentResourceStateModel()
	state := segmentResourceTestState(t, segmentSchema, stateModel)

	inPlace := providerSegmentPlanModel()
	inPlace.Name = types.StringValue("Renamed")
	inPlace.Description = types.StringValue("Changed")
	inPlace.IncludedUsers = terraformStringSetValue([]string{"new-user"})
	inPlace.Tags = terraformStringSetValue([]string{"new-tag"})
	inPlace.Rules[0].Conditions[0].Value = types.StringValue(`["changed"]`)
	assertSegmentStableIDResult(t, id, segmentSchema, state, inPlace, true)

	reorderedScopeSet := providerSegmentPlanModel()
	reorderedScopeSet.Scopes = terraformStringSetValue([]string{providerSegmentEnvironmentScope})
	assertSegmentStableIDResult(t, id, segmentSchema, state, reorderedScopeSet, true)

	replacements := map[string]func(*segmentModel){
		"environment_id": func(model *segmentModel) {
			model.EnvironmentID = types.StringValue(providerEnvironmentB)
		},
		"key": func(model *segmentModel) {
			model.Key = types.StringValue("replacement-key")
		},
		"scopes": func(model *segmentModel) {
			model.Scopes = terraformStringSetValue([]string{
				providerSegmentProjectScope + ":env/replacement",
			})
		},
	}
	for name, change := range replacements {
		name := name
		change := change
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			plan := providerSegmentPlanModel()
			change(&plan)
			assertSegmentStableIDResult(t, id, segmentSchema, state, plan, false)
		})
	}
}

func TestSegmentResourceSchemaReplacementModifiers(t *testing.T) {
	t.Parallel()

	segmentSchema := segmentResourceSchema()
	stateModel := providerSegmentResourceStateModel()
	state := segmentResourceTestState(t, segmentSchema, stateModel)
	planModel := providerSegmentPlanModel()
	plan := segmentResourceTestPlan(t, segmentSchema, planModel)

	for _, attributeName := range []string{"environment_id", "key"} {
		attribute := segmentSchema.Attributes[attributeName].(resourceschema.StringAttribute)
		var response planmodifier.StringResponse
		attribute.PlanModifiers[0].PlanModifyString(
			context.Background(),
			planmodifier.StringRequest{
				ConfigValue: types.StringValue("new"),
				PlanValue:   types.StringValue("new"),
				StateValue:  types.StringValue("old"),
				Plan:        plan,
				State:       state,
			},
			&response,
		)
		if !response.RequiresReplace {
			t.Fatalf("%s plan modifier did not require replacement", attributeName)
		}
	}

	scopes := segmentSchema.Attributes["scopes"].(resourceschema.SetAttribute)
	var setResponse planmodifier.SetResponse
	scopes.PlanModifiers[0].PlanModifySet(
		context.Background(),
		planmodifier.SetRequest{
			ConfigValue: terraformStringSetValue([]string{providerSegmentEnvironmentScope}),
			PlanValue:   terraformStringSetValue([]string{providerSegmentEnvironmentScope}),
			StateValue: terraformStringSetValue([]string{
				providerSegmentProjectScope + ":env/old",
			}),
			Plan:  plan,
			State: state,
		},
		&setResponse,
	)
	if !setResponse.RequiresReplace {
		t.Fatal("scopes plan modifier did not require replacement")
	}

	// Every remaining configurable field is deliberately updated in place.
	for _, attributeName := range []string{
		"name", "description", "included_users", "excluded_users", "rules", "tags",
	} {
		switch attribute := segmentSchema.Attributes[attributeName].(type) {
		case resourceschema.StringAttribute:
			if len(attribute.PlanModifiers) != 0 {
				t.Fatalf("%s unexpectedly requires replacement", attributeName)
			}
		case resourceschema.SetAttribute:
			if len(attribute.PlanModifiers) != 0 {
				t.Fatalf("%s unexpectedly requires replacement", attributeName)
			}
		case resourceschema.ListNestedAttribute:
			if len(attribute.PlanModifiers) != 0 {
				t.Fatalf("%s unexpectedly requires replacement", attributeName)
			}
		default:
			t.Fatalf("unexpected %s schema type %T", attributeName, attribute)
		}
	}
}

func TestSegmentResourceSchemaRejectsUnsafeScopesUsersRulesAndSharedType(t *testing.T) {
	t.Parallel()

	// A computed-only type makes shared Segment mutation structurally
	// unreachable before any lifecycle code exists.
	segmentSchema := segmentResourceSchema()
	segmentType := segmentSchema.Attributes["type"].(resourceschema.StringAttribute)
	if !segmentType.Computed || segmentType.Optional || segmentType.Required {
		t.Fatal("shared Segment type can be configured on the resource")
	}

	for name, scopes := range map[string]struct {
		values []string
		valid  bool
	}{
		"full environment RN": {
			values: []string{providerSegmentEnvironmentScope},
			valid:  true,
		},
		"project RN": {values: []string{providerSegmentProjectScope}},
		"wildcard RN": {
			values: []string{providerSegmentEnvironmentScope + "*"},
		},
		"multiple RNs": {
			values: []string{
				providerSegmentEnvironmentScope,
				providerSegmentProjectScope + ":env/other",
			},
		},
	} {
		name := name
		scopes := scopes
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var response validatorSetResponse
			validateSegmentScopesForTest(scopes.values, &response)
			if response.hasError == scopes.valid {
				t.Fatalf("scope validation error = %t", response.hasError)
			}
		})
	}

	var unknownResponse validator.SetResponse
	segmentEnvironmentScopesValidator{}.ValidateSet(
		context.Background(),
		validator.SetRequest{
			ConfigValue: types.SetValueMust(
				types.StringType,
				[]attr.Value{types.StringUnknown()},
			),
			Path: path.Root("scopes"),
		},
		&unknownResponse,
	)
	if unknownResponse.Diagnostics.HasError() {
		t.Fatal("scope validation rejected a parent-dependent unknown RN")
	}

	model := providerSegmentPlanModel()
	model.Type = types.StringValue(string(client.SegmentTypeShared))
	if _, err := canonicalizeSegmentPlanModel(context.Background(), model); err == nil {
		t.Fatal("resource canonicalizer accepted shared Segment mutation")
	}
}

// validatorSetResponse keeps the table above focused without retaining
// runtime scope values in test failure formatting.
type validatorSetResponse struct {
	hasError bool
}

func validateSegmentScopesForTest(values []string, result *validatorSetResponse) {
	var response validator.SetResponse
	segmentEnvironmentScopesValidator{}.ValidateSet(
		context.Background(),
		validator.SetRequest{
			ConfigValue: terraformStringSetValue(values),
			Path:        path.Root("scopes"),
		},
		&response,
	)
	result.hasError = response.Diagnostics.HasError()
}

func providerSegmentResourceStateModel() segmentModel {
	model := providerSegmentPlanModel()
	model.ID = types.StringValue(providerSegmentID)
	model.Type = types.StringValue(string(client.SegmentTypeEnvironmentSpecific))
	model.Rules[0].ID = types.StringValue(providerSegmentRuleOne)
	model.Rules[0].Conditions[0].ID = types.StringValue(providerSegmentConditionOne)
	model.Rules[0].Conditions[1].ID = types.StringValue(providerSegmentConditionTwo)
	model.Rules[1].ID = types.StringValue(providerSegmentRuleTwo)
	model.Rules[1].Conditions[0].ID = types.StringValue(providerSegmentConditionTri)
	return model
}

func segmentResourceTestState(
	t *testing.T,
	segmentSchema resourceschema.Schema,
	model segmentModel,
) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: segmentSchema}
	if diagnostics := state.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize Segment resource state: %v", diagnostics)
	}
	return state
}

func segmentResourceTestPlan(
	t *testing.T,
	segmentSchema resourceschema.Schema,
	model segmentModel,
) tfsdk.Plan {
	t.Helper()
	state := segmentResourceTestState(t, segmentSchema, model)
	return tfsdk.Plan{Schema: segmentSchema, Raw: state.Raw}
}

func assertSegmentStableIDResult(
	t *testing.T,
	id resourceschema.StringAttribute,
	segmentSchema resourceschema.Schema,
	state tfsdk.State,
	model segmentModel,
	wantStable bool,
) {
	t.Helper()
	var response planmodifier.StringResponse
	response.PlanValue = types.StringUnknown()
	id.PlanModifiers[0].PlanModifyString(
		context.Background(),
		planmodifier.StringRequest{
			PlanValue:  types.StringUnknown(),
			StateValue: types.StringValue(providerSegmentID),
			Plan:       segmentResourceTestPlan(t, segmentSchema, model),
			State:      state,
		},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("stable ID diagnostics = %v", response.Diagnostics)
	}
	if wantStable && !response.PlanValue.Equal(types.StringValue(providerSegmentID)) {
		t.Fatal("in-place Segment plan did not retain the stable ID")
	}
	if !wantStable && !response.PlanValue.IsUnknown() {
		t.Fatal("replacement Segment plan retained the prior ID")
	}
}
