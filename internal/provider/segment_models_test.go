// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	providerSegmentID           = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	providerSegmentRuleOne      = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	providerSegmentRuleTwo      = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	providerSegmentConditionOne = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	providerSegmentConditionTwo = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	providerSegmentConditionTri = "ffffffff-ffff-4fff-8fff-ffffffffffff"

	providerSegmentOrganizationScope = "organization/synthetic-org"
	providerSegmentProjectScope      = providerSegmentOrganizationScope + ":project/synthetic-project"
	providerSegmentEnvironmentScope  = providerSegmentProjectScope + ":env/synthetic-env"
)

func TestCanonicalizeRemoteSegmentPreservesOrderedTargetingAndCanonicalizesSets(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		segmentType client.SegmentType
		scopes      []string
		wantScopes  []string
	}{
		"environment-specific": {
			segmentType: client.SegmentTypeEnvironmentSpecific,
			scopes:      []string{providerSegmentEnvironmentScope},
			wantScopes:  []string{providerSegmentEnvironmentScope},
		},
		"shared": {
			segmentType: client.SegmentTypeShared,
			scopes: []string{
				providerSegmentEnvironmentScope,
				providerSegmentOrganizationScope,
				providerSegmentProjectScope,
				providerSegmentOrganizationScope,
			},
			wantScopes: []string{
				providerSegmentOrganizationScope,
				providerSegmentProjectScope,
				providerSegmentEnvironmentScope,
			},
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			segment := providerRemoteSegment(test.segmentType, test.scopes)
			segment.ID = strings.ToUpper(providerSegmentID)
			segment.Included = []string{"user-z", "user-a", "user-z"}
			segment.Excluded = []string{"user-y", "user-b", "user-y"}
			segment.Tags = []string{"tag-z", "tag-a", "tag-z"}
			canonical, err := canonicalizeRemoteSegment(segment)
			if err != nil {
				t.Fatalf("canonicalizeRemoteSegment() error = %v", err)
			}
			if canonical.ID != providerSegmentID || canonical.Type != test.segmentType ||
				!reflect.DeepEqual(canonical.Scopes, test.wantScopes) ||
				!reflect.DeepEqual(canonical.IncludedUsers, []string{"user-a", "user-z"}) ||
				!reflect.DeepEqual(canonical.ExcludedUsers, []string{"user-b", "user-y"}) ||
				!reflect.DeepEqual(canonical.Tags, []string{"tag-a", "tag-z"}) {
				t.Fatal("remote Segment sets or taxonomy were not canonicalized")
			}
			if len(canonical.Rules) != 2 || canonical.Rules[0].Name != "First" ||
				canonical.Rules[1].Name != "Second" ||
				len(canonical.Rules[0].Conditions) != 2 ||
				canonical.Rules[0].Conditions[0].Property != "region" ||
				canonical.Rules[0].Conditions[1].Property != "score" ||
				canonical.Rules[0].Conditions[0].Value != `["a","b"]` ||
				canonical.Rules[0].Conditions[1].Value != "1" ||
				canonical.Rules[1].Conditions[0].Value != "" {
				t.Fatal("remote Segment rule or condition evaluation order/value encoding changed")
			}
			if canonical.Rules[0].ID != providerSegmentRuleOne ||
				canonical.Rules[0].Conditions[0].ID != providerSegmentConditionOne {
				t.Fatal("remote Segment targeting UUIDs were not canonicalized")
			}

			model := flattenCanonicalSegment(canonical)
			included, err := terraformStringSet(context.Background(), model.IncludedUsers)
			if err != nil || !reflect.DeepEqual(included, canonical.IncludedUsers) {
				t.Fatal("flattened Segment did not retain canonical set semantics")
			}
			if len(model.Rules) != 2 || model.Rules[0].Name.ValueString() != "First" ||
				model.Rules[1].Name.ValueString() != "Second" {
				t.Fatal("flattened Segment changed rule order")
			}
		})
	}
}

func TestSegmentConditionOperatorsAndValueEncoding(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		operator string
		value    string
		want     string
		valid    bool
	}{
		"less than":          {operator: segmentOperatorLessThan, value: "1.00", want: "1", valid: true},
		"bigger than":        {operator: segmentOperatorBiggerThan, value: "-0", want: "0", valid: true},
		"less equal":         {operator: segmentOperatorLessEqualThan, value: "1e2", want: "100", valid: true},
		"bigger equal":       {operator: segmentOperatorBiggerEqualThan, value: "0.25", want: "0.25", valid: true},
		"equal":              {operator: segmentOperatorEqual, value: " exact ", want: " exact ", valid: true},
		"not equal":          {operator: segmentOperatorNotEqual, value: "", want: "", valid: true},
		"contains":           {operator: segmentOperatorContains, value: "x", want: "x", valid: true},
		"not contain":        {operator: segmentOperatorNotContain, value: "x", want: "x", valid: true},
		"starts with":        {operator: segmentOperatorStartsWith, value: "x", want: "x", valid: true},
		"ends with":          {operator: segmentOperatorEndsWith, value: "x", want: "x", valid: true},
		"regex":              {operator: segmentOperatorMatchRegex, value: `^x+$`, want: `^x+$`, valid: true},
		"not regex":          {operator: segmentOperatorNotMatchRegex, value: `^x+$`, want: `^x+$`, valid: true},
		"one of":             {operator: segmentOperatorIsOneOf, value: `["z","a","z"]`, want: `["a","z"]`, valid: true},
		"not one of":         {operator: segmentOperatorNotOneOf, value: `[]`, want: `[]`, valid: true},
		"is true":            {operator: segmentOperatorIsTrue, value: "", want: "", valid: true},
		"is false":           {operator: segmentOperatorIsFalse, value: "", want: "", valid: true},
		"unknown":            {operator: "Unknown", value: "x"},
		"invalid number":     {operator: segmentOperatorLessThan, value: "NaN"},
		"invalid multi null": {operator: segmentOperatorIsOneOf, value: `null`},
		"invalid multi item": {operator: segmentOperatorIsOneOf, value: `[1]`},
		"trailing JSON":      {operator: segmentOperatorIsOneOf, value: `["x"] trailing`},
		"boolean payload":    {operator: segmentOperatorIsTrue, value: "ignored"},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			canonical, err := canonicalizeSegmentConditionValue(test.operator, test.value)
			if test.valid {
				if err != nil || canonical != test.want || !validSegmentOperator(test.operator) {
					t.Fatalf("condition canonicalization = %q / %v", canonical, err)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid condition operator/value was accepted")
			}
		})
	}
}

func TestCanonicalizeRemoteSegmentFailsClosedForContradictions(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*client.Segment){
		"blank name":   func(segment *client.Segment) { segment.Name = " " },
		"invalid key":  func(segment *client.Segment) { segment.Key = "invalid key" },
		"unknown type": func(segment *client.Segment) { segment.Type = client.SegmentType("unknown") },
		"environment type with project scope": func(segment *client.Segment) {
			segment.Scopes = []string{providerSegmentProjectScope}
		},
		"included and excluded overlap": func(segment *client.Segment) {
			segment.Included = []string{"same"}
			segment.Excluded = []string{"same"}
		},
		"missing rule id": func(segment *client.Segment) { segment.Rules[0].ID = "" },
		"duplicate rule id": func(segment *client.Segment) {
			segment.Rules[1].ID = strings.ToUpper(segment.Rules[0].ID)
		},
		"empty rule":           func(segment *client.Segment) { segment.Rules[0].Conditions = []client.SegmentCondition{} },
		"missing condition id": func(segment *client.Segment) { segment.Rules[0].Conditions[0].ID = "" },
		"duplicate condition id": func(segment *client.Segment) {
			segment.Rules[1].Conditions[0].ID = strings.ToUpper(segment.Rules[0].Conditions[0].ID)
		},
		"empty condition property": func(segment *client.Segment) { segment.Rules[0].Conditions[0].Property = "" },
		"segment condition property": func(segment *client.Segment) {
			segment.Rules[0].Conditions[0].Property = "User is in segment"
		},
		"unknown operator":       func(segment *client.Segment) { segment.Rules[0].Conditions[0].Operator = "unknown" },
		"invalid value encoding": func(segment *client.Segment) { segment.Rules[0].Conditions[0].Value = "not-json" },
	}

	for name, mutate := range tests {
		name := name
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			segment := providerRemoteSegment(
				client.SegmentTypeEnvironmentSpecific,
				[]string{providerSegmentEnvironmentScope},
			)
			mutate(&segment)
			if _, err := canonicalizeRemoteSegment(segment); err == nil {
				t.Fatal("contradictory remote Segment was accepted")
			}
		})
	}
}

func TestCanonicalizeSegmentPlanUsesStableProviderOwnedTargetingIDs(t *testing.T) {
	t.Parallel()

	plan := providerSegmentPlanModel()
	first, err := canonicalizeSegmentPlanModel(context.Background(), plan)
	if err != nil {
		t.Fatalf("canonicalizeSegmentPlanModel() error = %v", err)
	}
	second, err := canonicalizeSegmentPlanModel(context.Background(), plan)
	if err != nil {
		t.Fatalf("second canonicalizeSegmentPlanModel() error = %v", err)
	}
	if first.Type != client.SegmentTypeEnvironmentSpecific || first.ID != "" ||
		len(first.Rules) != 2 || len(first.Rules[0].Conditions) != 2 ||
		first.Rules[0].ID != second.Rules[0].ID ||
		first.Rules[0].Conditions[0].ID != second.Rules[0].Conditions[0].ID ||
		first.Rules[0].ID == first.Rules[1].ID ||
		first.Rules[0].Conditions[0].ID == first.Rules[0].Conditions[1].ID ||
		!client.ValidUUID(first.Rules[0].ID) ||
		!client.ValidUUID(first.Rules[0].Conditions[0].ID) {
		t.Fatal("planned Segment did not receive stable distinct deterministic targeting UUIDs")
	}

	changed := providerSegmentPlanModel()
	changed.Key = types.StringValue("other-key")
	other, err := canonicalizeSegmentPlanModel(context.Background(), changed)
	if err != nil || other.Rules[0].ID == first.Rules[0].ID ||
		other.Rules[0].Conditions[0].ID == first.Rules[0].Conditions[0].ID {
		t.Fatal("identity-defining Segment input did not reseed targeting UUIDs")
	}

	imported := providerSegmentPlanModel()
	imported.ID = types.StringValue(strings.ToUpper(providerSegmentID))
	imported.Type = types.StringValue(string(client.SegmentTypeEnvironmentSpecific))
	imported.Rules[0].ID = types.StringValue(strings.ToUpper(providerSegmentRuleOne))
	imported.Rules[0].Conditions[0].ID = types.StringValue(strings.ToUpper(providerSegmentConditionOne))
	canonicalImported, err := canonicalizeSegmentStateModel(context.Background(), imported)
	if err != nil || canonicalImported.ID != providerSegmentID ||
		canonicalImported.Rules[0].ID != providerSegmentRuleOne ||
		canonicalImported.Rules[0].Conditions[0].ID != providerSegmentConditionOne {
		t.Fatal("imported Segment targeting identities were not preserved canonically")
	}
}

func TestCanonicalizeSegmentPlanRejectsUnknownUnsafeOrSharedState(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*segmentModel){
		"unknown environment": func(model *segmentModel) { model.EnvironmentID = types.StringUnknown() },
		"null description":    func(model *segmentModel) { model.Description = types.StringNull() },
		"unknown scopes":      func(model *segmentModel) { model.Scopes = types.SetUnknown(types.StringType) },
		"project scope": func(model *segmentModel) {
			model.Scopes = terraformStringSetValue([]string{providerSegmentProjectScope})
		},
		"overlapping users": func(model *segmentModel) {
			model.IncludedUsers = terraformStringSetValue([]string{"same"})
			model.ExcludedUsers = terraformStringSetValue([]string{"same"})
		},
		"shared resource type": func(model *segmentModel) {
			model.Type = types.StringValue(string(client.SegmentTypeShared))
		},
		"empty rule conditions": func(model *segmentModel) { model.Rules[0].Conditions = nil },
		"unknown condition": func(model *segmentModel) {
			model.Rules[0].Conditions[0].Value = types.StringUnknown()
		},
	}

	for name, mutate := range tests {
		name := name
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			model := providerSegmentPlanModel()
			mutate(&model)
			if _, err := canonicalizeSegmentPlanModel(context.Background(), model); err == nil {
				t.Fatal("unknown, unsafe, or shared Segment resource plan was accepted")
			}
		})
	}
}

func TestSegmentModelsRedactAccidentalFormatting(t *testing.T) {
	t.Parallel()

	marker := "segment-model-redaction-marker"
	model := providerSegmentPlanModel()
	model.Key = types.StringValue(marker)
	canonical, err := canonicalizeSegmentPlanModel(context.Background(), model)
	if err != nil {
		t.Fatal("could not build canonical Segment for redaction test")
	}
	formatted := fmt.Sprintf(
		"%v|%+v|%#v|%v|%v|%v",
		model,
		model.Rules[0],
		model.Rules[0].Conditions[0],
		canonical,
		canonical.Rules[0],
		canonical.Rules[0].Conditions[0],
	)
	if strings.Contains(formatted, marker) {
		t.Fatal("Segment model formatting exposed a runtime value")
	}
}

func providerRemoteSegment(segmentType client.SegmentType, scopes []string) client.Segment {
	return client.Segment{
		EnvironmentID: providerEnvironmentA,
		ID:            providerSegmentID,
		Name:          "Synthetic Segment",
		Key:           "synthetic-segment",
		Description:   "Synthetic description",
		Type:          segmentType,
		Scopes:        append([]string(nil), scopes...),
		Included:      []string{},
		Excluded:      []string{},
		Tags:          []string{},
		Rules: []client.SegmentRule{
			{
				ID:   strings.ToUpper(providerSegmentRuleOne),
				Name: "First",
				Conditions: []client.SegmentCondition{
					{
						ID:       strings.ToUpper(providerSegmentConditionOne),
						Property: "region",
						Operator: segmentOperatorIsOneOf,
						Value:    `["b","a","b"]`,
					},
					{
						ID:       providerSegmentConditionTwo,
						Property: "score",
						Operator: segmentOperatorLessThan,
						Value:    "1.00",
					},
				},
			},
			{
				ID:   providerSegmentRuleTwo,
				Name: "Second",
				Conditions: []client.SegmentCondition{
					{
						ID:       providerSegmentConditionTri,
						Property: "enabled",
						Operator: segmentOperatorIsTrue,
						Value:    "",
					},
				},
			},
		},
	}
}

func providerSegmentPlanModel() segmentModel {
	return segmentModel{
		EnvironmentID: types.StringValue(providerEnvironmentA),
		ID:            types.StringUnknown(),
		Name:          types.StringValue("Synthetic Segment"),
		Key:           types.StringValue("synthetic-segment"),
		Description:   types.StringValue(""),
		Type:          types.StringUnknown(),
		Scopes:        terraformStringSetValue([]string{providerSegmentEnvironmentScope}),
		IncludedUsers: terraformStringSetValue([]string{"user-z", "user-a"}),
		ExcludedUsers: terraformStringSetValue([]string{"user-y"}),
		Tags:          terraformStringSetValue([]string{"tag-z", "tag-a"}),
		Rules: []segmentRuleModel{
			{
				ID:   types.StringUnknown(),
				Name: types.StringValue("First"),
				Conditions: []segmentConditionModel{
					{
						ID:       types.StringUnknown(),
						Property: types.StringValue("region"),
						Operator: types.StringValue(segmentOperatorIsOneOf),
						Value:    types.StringValue(`["b","a"]`),
					},
					{
						ID:       types.StringUnknown(),
						Property: types.StringValue("score"),
						Operator: types.StringValue(segmentOperatorBiggerEqualThan),
						Value:    types.StringValue("1.00"),
					},
				},
			},
			{
				ID:   types.StringUnknown(),
				Name: types.StringValue("Second"),
				Conditions: []segmentConditionModel{
					{
						ID:       types.StringUnknown(),
						Property: types.StringValue("enabled"),
						Operator: types.StringValue(segmentOperatorIsFalse),
						Value:    types.StringValue(""),
					},
				},
			},
		},
	}
}
