// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	segmentMaximumNameLength = 128
	segmentMaximumKeyLength  = 128

	segmentOperatorLessThan        = "LessThan"
	segmentOperatorBiggerThan      = "BiggerThan"
	segmentOperatorLessEqualThan   = "LessEqualThan"
	segmentOperatorBiggerEqualThan = "BiggerEqualThan"
	segmentOperatorEqual           = "Equal"
	segmentOperatorNotEqual        = "NotEqual"
	segmentOperatorContains        = "Contains"
	segmentOperatorNotContain      = "NotContain"
	segmentOperatorStartsWith      = "StartsWith"
	segmentOperatorEndsWith        = "EndsWith"
	segmentOperatorMatchRegex      = "MatchRegex"
	segmentOperatorNotMatchRegex   = "NotMatchRegex"
	segmentOperatorIsOneOf         = "IsOneOf"
	segmentOperatorNotOneOf        = "NotOneOf"
	segmentOperatorIsTrue          = "IsTrue"
	segmentOperatorIsFalse         = "IsFalse"
)

var (
	segmentKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

	errInvalidSegmentDefinition = errors.New("Segment definition is invalid")

	segmentOperators = map[string]struct{}{
		segmentOperatorLessThan:        {},
		segmentOperatorBiggerThan:      {},
		segmentOperatorLessEqualThan:   {},
		segmentOperatorBiggerEqualThan: {},
		segmentOperatorEqual:           {},
		segmentOperatorNotEqual:        {},
		segmentOperatorContains:        {},
		segmentOperatorNotContain:      {},
		segmentOperatorStartsWith:      {},
		segmentOperatorEndsWith:        {},
		segmentOperatorMatchRegex:      {},
		segmentOperatorNotMatchRegex:   {},
		segmentOperatorIsOneOf:         {},
		segmentOperatorNotOneOf:        {},
		segmentOperatorIsTrue:          {},
		segmentOperatorIsFalse:         {},
	}
)

type segmentModel struct {
	EnvironmentID types.String       `tfsdk:"environment_id"`
	ID            types.String       `tfsdk:"id"`
	Name          types.String       `tfsdk:"name"`
	Key           types.String       `tfsdk:"key"`
	Description   types.String       `tfsdk:"description"`
	Type          types.String       `tfsdk:"type"`
	Scopes        types.Set          `tfsdk:"scopes"`
	IncludedUsers types.Set          `tfsdk:"included_users"`
	ExcludedUsers types.Set          `tfsdk:"excluded_users"`
	Rules         []segmentRuleModel `tfsdk:"rules"`
	Tags          types.Set          `tfsdk:"tags"`
}

func (segmentModel) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.segmentModel{redacted}")
}

type segmentRuleModel struct {
	ID         types.String            `tfsdk:"id"`
	Name       types.String            `tfsdk:"name"`
	Conditions []segmentConditionModel `tfsdk:"conditions"`
}

func (segmentRuleModel) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.segmentRuleModel{redacted}")
}

type segmentConditionModel struct {
	ID       types.String `tfsdk:"id"`
	Property types.String `tfsdk:"property"`
	Operator types.String `tfsdk:"operator"`
	Value    types.String `tfsdk:"value"`
}

func (segmentConditionModel) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.segmentConditionModel{redacted}")
}

type canonicalSegment struct {
	EnvironmentID string
	ID            string
	Name          string
	Key           string
	Description   string
	Type          client.SegmentType
	Scopes        []string
	IncludedUsers []string
	ExcludedUsers []string
	Rules         []canonicalSegmentRule
	Tags          []string
}

func (canonicalSegment) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.canonicalSegment{redacted}")
}

type canonicalSegmentRule struct {
	ID         string
	Name       string
	Conditions []canonicalSegmentCondition
}

func (canonicalSegmentRule) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.canonicalSegmentRule{redacted}")
}

type canonicalSegmentCondition struct {
	ID       string
	Property string
	Operator string
	Value    string
}

func (canonicalSegmentCondition) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.canonicalSegmentCondition{redacted}")
}

// canonicalizeRemoteSegment validates one complete public Segment definition.
// Set-valued fields are sorted after exact deduplication, while rule and
// condition evaluation order is preserved byte-for-byte.
func canonicalizeRemoteSegment(segment client.Segment) (canonicalSegment, error) {
	segmentID, valid := client.CanonicalUUID(segment.ID)
	if !valid || !client.ValidUUID(segment.EnvironmentID) ||
		!validSegmentName(segment.Name) || !validSegmentKey(segment.Key) ||
		!validSegmentTypeAndScopes(segment.Type, segment.Scopes) {
		return canonicalSegment{}, errInvalidSegmentDefinition
	}

	canonical := canonicalSegment{
		EnvironmentID: segment.EnvironmentID,
		ID:            segmentID,
		Name:          segment.Name,
		Key:           segment.Key,
		Description:   segment.Description,
		Type:          segment.Type,
		Scopes:        canonicalStringSet(segment.Scopes),
		IncludedUsers: canonicalStringSet(segment.Included),
		ExcludedUsers: canonicalStringSet(segment.Excluded),
		Rules:         make([]canonicalSegmentRule, 0, len(segment.Rules)),
		Tags:          canonicalStringSet(segment.Tags),
	}
	if stringSetsIntersect(canonical.IncludedUsers, canonical.ExcludedUsers) {
		return canonicalSegment{}, errInvalidSegmentDefinition
	}

	seenRuleIDs := make(map[string]struct{}, len(segment.Rules))
	seenConditionIDs := make(map[string]struct{})
	for _, rule := range segment.Rules {
		canonicalRule, err := canonicalizeRemoteSegmentRule(
			rule,
			seenRuleIDs,
			seenConditionIDs,
		)
		if err != nil {
			return canonicalSegment{}, err
		}
		canonical.Rules = append(canonical.Rules, canonicalRule)
	}
	return canonical, nil
}

func canonicalizeRemoteSegmentRule(
	rule client.SegmentRule,
	seenRuleIDs map[string]struct{},
	seenConditionIDs map[string]struct{},
) (canonicalSegmentRule, error) {
	ruleID, valid := client.CanonicalUUID(rule.ID)
	if !valid || len(rule.Conditions) == 0 {
		return canonicalSegmentRule{}, errInvalidSegmentDefinition
	}
	if _, duplicate := seenRuleIDs[ruleID]; duplicate {
		return canonicalSegmentRule{}, errInvalidSegmentDefinition
	}
	seenRuleIDs[ruleID] = struct{}{}

	canonical := canonicalSegmentRule{
		ID:         ruleID,
		Name:       rule.Name,
		Conditions: make([]canonicalSegmentCondition, 0, len(rule.Conditions)),
	}
	for _, condition := range rule.Conditions {
		conditionID, valid := client.CanonicalUUID(condition.ID)
		if !valid || !validSegmentConditionProperty(condition.Property) ||
			!validSegmentOperator(condition.Operator) {
			return canonicalSegmentRule{}, errInvalidSegmentDefinition
		}
		if _, duplicate := seenConditionIDs[conditionID]; duplicate {
			return canonicalSegmentRule{}, errInvalidSegmentDefinition
		}
		seenConditionIDs[conditionID] = struct{}{}
		value, err := canonicalizeSegmentConditionValue(condition.Operator, condition.Value)
		if err != nil {
			return canonicalSegmentRule{}, err
		}
		canonical.Conditions = append(canonical.Conditions, canonicalSegmentCondition{
			ID:       conditionID,
			Property: condition.Property,
			Operator: condition.Operator,
			Value:    value,
		})
	}
	return canonical, nil
}

// canonicalizeSegmentPlanModel applies the resource-only contract. The type
// is fixed to environment-specific, scopes are explicit immutable inputs, and
// provider-created rule/condition identities are deterministic by ordered
// position. Known imported identities are preserved exactly after UUID
// canonicalization.
func canonicalizeSegmentPlanModel(
	ctx context.Context,
	plan segmentModel,
) (canonicalSegment, error) {
	if !knownString(plan.EnvironmentID) || !knownString(plan.Name) ||
		!knownString(plan.Key) || !knownString(plan.Description) ||
		plan.Scopes.IsNull() || plan.Scopes.IsUnknown() ||
		plan.IncludedUsers.IsNull() || plan.IncludedUsers.IsUnknown() ||
		plan.ExcludedUsers.IsNull() || plan.ExcludedUsers.IsUnknown() ||
		plan.Tags.IsNull() || plan.Tags.IsUnknown() {
		return canonicalSegment{}, errInvalidSegmentDefinition
	}
	if !plan.Type.IsNull() && !plan.Type.IsUnknown() &&
		plan.Type.ValueString() != string(client.SegmentTypeEnvironmentSpecific) {
		return canonicalSegment{}, errInvalidSegmentDefinition
	}

	scopes, err := terraformStringSet(ctx, plan.Scopes)
	if err != nil || !validEnvironmentSpecificScopes(scopes) {
		return canonicalSegment{}, errInvalidSegmentDefinition
	}
	included, err := terraformStringSet(ctx, plan.IncludedUsers)
	if err != nil {
		return canonicalSegment{}, errInvalidSegmentDefinition
	}
	excluded, err := terraformStringSet(ctx, plan.ExcludedUsers)
	if err != nil || stringSetsIntersect(included, excluded) {
		return canonicalSegment{}, errInvalidSegmentDefinition
	}
	tags, err := terraformStringSet(ctx, plan.Tags)
	if err != nil {
		return canonicalSegment{}, errInvalidSegmentDefinition
	}

	canonical := canonicalSegment{
		EnvironmentID: plan.EnvironmentID.ValueString(),
		Name:          plan.Name.ValueString(),
		Key:           plan.Key.ValueString(),
		Description:   plan.Description.ValueString(),
		Type:          client.SegmentTypeEnvironmentSpecific,
		Scopes:        scopes,
		IncludedUsers: included,
		ExcludedUsers: excluded,
		Rules:         make([]canonicalSegmentRule, 0, len(plan.Rules)),
		Tags:          tags,
	}
	if !validSegmentName(canonical.Name) || !validSegmentKey(canonical.Key) ||
		!client.ValidUUID(canonical.EnvironmentID) {
		return canonicalSegment{}, errInvalidSegmentDefinition
	}
	if !plan.ID.IsNull() && !plan.ID.IsUnknown() {
		canonical.ID, _ = client.CanonicalUUID(plan.ID.ValueString())
		if canonical.ID == "" {
			return canonicalSegment{}, errInvalidSegmentDefinition
		}
	}

	seenRuleIDs := make(map[string]struct{}, len(plan.Rules))
	seenConditionIDs := make(map[string]struct{})
	for ruleIndex, rule := range plan.Rules {
		canonicalRule, err := canonicalizePlannedSegmentRule(
			canonical.EnvironmentID,
			canonical.Key,
			ruleIndex,
			rule,
			seenRuleIDs,
			seenConditionIDs,
		)
		if err != nil {
			return canonicalSegment{}, err
		}
		canonical.Rules = append(canonical.Rules, canonicalRule)
	}
	return canonical, nil
}

func canonicalizePlannedSegmentRule(
	environmentID string,
	segmentKey string,
	ruleIndex int,
	rule segmentRuleModel,
	seenRuleIDs map[string]struct{},
	seenConditionIDs map[string]struct{},
) (canonicalSegmentRule, error) {
	if !knownString(rule.Name) || len(rule.Conditions) == 0 {
		return canonicalSegmentRule{}, errInvalidSegmentDefinition
	}
	ruleID := deterministicSegmentRuleID(environmentID, segmentKey, ruleIndex)
	if !rule.ID.IsNull() && !rule.ID.IsUnknown() {
		var valid bool
		ruleID, valid = client.CanonicalUUID(rule.ID.ValueString())
		if !valid {
			return canonicalSegmentRule{}, errInvalidSegmentDefinition
		}
	}
	if _, duplicate := seenRuleIDs[ruleID]; duplicate {
		return canonicalSegmentRule{}, errInvalidSegmentDefinition
	}
	seenRuleIDs[ruleID] = struct{}{}

	canonical := canonicalSegmentRule{
		ID:         ruleID,
		Name:       rule.Name.ValueString(),
		Conditions: make([]canonicalSegmentCondition, 0, len(rule.Conditions)),
	}
	for conditionIndex, condition := range rule.Conditions {
		if !knownString(condition.Property) || !knownString(condition.Operator) ||
			!knownString(condition.Value) {
			return canonicalSegmentRule{}, errInvalidSegmentDefinition
		}
		conditionID := deterministicSegmentConditionID(
			environmentID,
			segmentKey,
			ruleIndex,
			conditionIndex,
		)
		if !condition.ID.IsNull() && !condition.ID.IsUnknown() {
			var valid bool
			conditionID, valid = client.CanonicalUUID(condition.ID.ValueString())
			if !valid {
				return canonicalSegmentRule{}, errInvalidSegmentDefinition
			}
		}
		if _, duplicate := seenConditionIDs[conditionID]; duplicate {
			return canonicalSegmentRule{}, errInvalidSegmentDefinition
		}
		seenConditionIDs[conditionID] = struct{}{}
		if !validSegmentConditionProperty(condition.Property.ValueString()) ||
			!validSegmentOperator(condition.Operator.ValueString()) {
			return canonicalSegmentRule{}, errInvalidSegmentDefinition
		}
		value, err := canonicalizeSegmentConditionValue(
			condition.Operator.ValueString(),
			condition.Value.ValueString(),
		)
		if err != nil {
			return canonicalSegmentRule{}, err
		}
		canonical.Conditions = append(canonical.Conditions, canonicalSegmentCondition{
			ID:       conditionID,
			Property: condition.Property.ValueString(),
			Operator: condition.Operator.ValueString(),
			Value:    value,
		})
	}
	return canonical, nil
}

func canonicalizeSegmentStateModel(
	ctx context.Context,
	state segmentModel,
) (canonicalSegment, error) {
	if !knownString(state.ID) || !knownString(state.Type) {
		return canonicalSegment{}, errInvalidSegmentDefinition
	}
	canonical, err := canonicalizeSegmentPlanModel(ctx, state)
	if err != nil {
		return canonicalSegment{}, err
	}
	if state.Type.ValueString() != string(client.SegmentTypeEnvironmentSpecific) {
		return canonicalSegment{}, errInvalidSegmentDefinition
	}
	canonical.ID, _ = client.CanonicalUUID(state.ID.ValueString())
	if canonical.ID == "" {
		return canonicalSegment{}, errInvalidSegmentDefinition
	}
	return canonical, nil
}

func flattenSegment(segment client.Segment) (segmentModel, error) {
	canonical, err := canonicalizeRemoteSegment(segment)
	if err != nil {
		return segmentModel{}, err
	}
	return flattenCanonicalSegment(canonical), nil
}

func flattenCanonicalSegment(segment canonicalSegment) segmentModel {
	model := segmentModel{
		EnvironmentID: types.StringValue(segment.EnvironmentID),
		ID:            types.StringValue(segment.ID),
		Name:          types.StringValue(segment.Name),
		Key:           types.StringValue(segment.Key),
		Description:   types.StringValue(segment.Description),
		Type:          types.StringValue(string(segment.Type)),
		Scopes:        terraformStringSetValue(segment.Scopes),
		IncludedUsers: terraformStringSetValue(segment.IncludedUsers),
		ExcludedUsers: terraformStringSetValue(segment.ExcludedUsers),
		Rules:         make([]segmentRuleModel, 0, len(segment.Rules)),
		Tags:          terraformStringSetValue(segment.Tags),
	}
	for _, rule := range segment.Rules {
		flattenedRule := segmentRuleModel{
			ID:         types.StringValue(rule.ID),
			Name:       types.StringValue(rule.Name),
			Conditions: make([]segmentConditionModel, 0, len(rule.Conditions)),
		}
		for _, condition := range rule.Conditions {
			flattenedRule.Conditions = append(flattenedRule.Conditions, segmentConditionModel{
				ID:       types.StringValue(condition.ID),
				Property: types.StringValue(condition.Property),
				Operator: types.StringValue(condition.Operator),
				Value:    types.StringValue(condition.Value),
			})
		}
		model.Rules = append(model.Rules, flattenedRule)
	}
	return model
}

func validSegmentName(value string) bool {
	return strings.TrimSpace(value) != "" && utf16Length(value) <= segmentMaximumNameLength
}

func validSegmentKey(value string) bool {
	return value != "" && len(value) <= segmentMaximumKeyLength &&
		segmentKeyPattern.MatchString(value)
}

func validSegmentTypeAndScopes(segmentType client.SegmentType, scopes []string) bool {
	if len(scopes) == 0 {
		return false
	}
	for _, scope := range scopes {
		if _, valid := client.ClassifySegmentScope(scope); !valid {
			return false
		}
	}
	switch segmentType {
	case client.SegmentTypeEnvironmentSpecific:
		return validEnvironmentSpecificScopes(scopes)
	case client.SegmentTypeShared:
		return true
	default:
		return false
	}
}

func validEnvironmentSpecificScopes(scopes []string) bool {
	if len(canonicalStringSet(scopes)) != 1 {
		return false
	}
	kind, valid := client.ClassifySegmentScope(scopes[0])
	return valid && kind == client.SegmentScopeEnvironment
}

func validSegmentConditionProperty(value string) bool {
	return value != "" && value != "User is in segment" &&
		value != "User is not in segment"
}

func validSegmentOperator(value string) bool {
	_, valid := segmentOperators[value]
	return valid
}

func canonicalizeSegmentConditionValue(operator string, value string) (string, error) {
	switch operator {
	case segmentOperatorIsOneOf, segmentOperatorNotOneOf:
		var values []string
		decoder := json.NewDecoder(strings.NewReader(value))
		if err := decoder.Decode(&values); err != nil || values == nil {
			return "", errInvalidSegmentDefinition
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			return "", errInvalidSegmentDefinition
		}
		canonical, err := json.Marshal(canonicalStringSet(values))
		if err != nil {
			return "", errInvalidSegmentDefinition
		}
		return string(canonical), nil
	case segmentOperatorLessThan, segmentOperatorBiggerThan,
		segmentOperatorLessEqualThan, segmentOperatorBiggerEqualThan:
		number, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return "", errInvalidSegmentDefinition
		}
		if number == 0 {
			return "0", nil
		}
		return strconv.FormatFloat(number, 'g', -1, 64), nil
	case segmentOperatorIsTrue, segmentOperatorIsFalse:
		if value != "" {
			return "", errInvalidSegmentDefinition
		}
		return "", nil
	case segmentOperatorEqual, segmentOperatorNotEqual,
		segmentOperatorContains, segmentOperatorNotContain,
		segmentOperatorStartsWith, segmentOperatorEndsWith,
		segmentOperatorMatchRegex, segmentOperatorNotMatchRegex:
		return value, nil
	default:
		return "", errInvalidSegmentDefinition
	}
}

func deterministicSegmentRuleID(environmentID, segmentKey string, index int) string {
	return deterministicSegmentTargetingID(
		"terraform-provider-featbit/segment-rule/v1",
		environmentID,
		segmentKey,
		strconv.Itoa(index),
	)
}

func deterministicSegmentConditionID(
	environmentID string,
	segmentKey string,
	ruleIndex int,
	conditionIndex int,
) string {
	return deterministicSegmentTargetingID(
		"terraform-provider-featbit/segment-condition/v1",
		environmentID,
		segmentKey,
		strconv.Itoa(ruleIndex),
		strconv.Itoa(conditionIndex),
	)
}

func deterministicSegmentTargetingID(parts ...string) string {
	if len(parts) < 3 {
		return ""
	}
	canonicalEnvironmentID, valid := client.CanonicalUUID(parts[1])
	if !valid {
		return ""
	}
	parts = append([]string(nil), parts...)
	parts[1] = canonicalEnvironmentID
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(strings.Join(parts, "\x00"))).String()
}

func canonicalStringSet(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	canonical := make([]string, 0, len(values))
	for _, value := range values {
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		canonical = append(canonical, value)
	}
	sort.Strings(canonical)
	return canonical
}

func stringSetsIntersect(left, right []string) bool {
	leftValues := make(map[string]struct{}, len(left))
	for _, value := range left {
		leftValues[value] = struct{}{}
	}
	for _, value := range right {
		if _, exists := leftValues[value]; exists {
			return true
		}
	}
	return false
}

func terraformStringSet(ctx context.Context, value types.Set) ([]string, error) {
	if value.IsNull() || value.IsUnknown() {
		return nil, errInvalidSegmentDefinition
	}
	var values []string
	if diagnostics := value.ElementsAs(ctx, &values, false); diagnostics.HasError() {
		return nil, errInvalidSegmentDefinition
	}
	return canonicalStringSet(values), nil
}

func terraformStringSetValue(values []string) types.Set {
	elements := make([]attr.Value, 0, len(values))
	for _, value := range canonicalStringSet(values) {
		elements = append(elements, types.StringValue(value))
	}
	return types.SetValueMust(types.StringType, elements)
}

func knownString(value types.String) bool {
	return !value.IsNull() && !value.IsUnknown()
}
