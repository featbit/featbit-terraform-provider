// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	policyResourceTypeProject = "project"
	policyResourceTypeEnv     = "env"
	policyResourceTypeFlag    = "flag"
	policyResourceTypeSegment = "segment"

	policyEffectAllow = "allow"
	policyEffectDeny  = "deny"
)

var (
	errInvalidPolicyDefinition = errors.New("Policy definition is invalid")

	policyActions = map[string]map[string]struct{}{
		policyResourceTypeProject: stringCatalog(
			"CanAccessProject",
			"CreateProject",
			"DeleteProject",
			"UpdateProjectSettings",
			"CreateEnv",
		),
		policyResourceTypeEnv: stringCatalog(
			"CanAccessEnv",
			"DeleteEnv",
			"UpdateEnvSettings",
			"DeleteEnvSecret",
			"CreateEnvSecret",
			"UpdateEnvSecret",
		),
		policyResourceTypeFlag: stringCatalog(
			"*",
			"CreateFlag",
			"ArchiveFlag",
			"RestoreFlag",
			"DeleteFlag",
			"CloneFlag",
			"CopyFlagTo",
			"ToggleFlag",
			"UpdateFlagName",
			"UpdateFlagDescription",
			"UpdateFlagOffVariation",
			"UpdateFlagVariations",
			"UpdateFlagTags",
			"UpdateFlagDefaultRule",
			"UpdateFlagIndividualTargeting",
			"UpdateFlagTargetingRules",
		),
		policyResourceTypeSegment: stringCatalog(
			"*",
			"CreateSegment",
			"ArchiveSegment",
			"RestoreSegment",
			"DeleteSegment",
			"UpdateSegmentName",
			"UpdateSegmentDescription",
			"UpdateSegmentTags",
			"UpdateSegmentTargetingUsers",
			"UpdateSegmentRules",
		),
	}
)

type policyModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Key         types.String `tfsdk:"key"`
	Description types.String `tfsdk:"description"`
	Type        types.String `tfsdk:"type"`
	Statements  types.Set    `tfsdk:"statements"`
}

func (policyModel) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.policyModel{redacted}")
}

type policyStatementModel struct {
	ResourceType types.String `tfsdk:"resource_type"`
	Effect       types.String `tfsdk:"effect"`
	Actions      types.Set    `tfsdk:"actions"`
	Resources    types.Set    `tfsdk:"resources"`
}

func (policyStatementModel) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.policyStatementModel{redacted}")
}

type canonicalPolicy struct {
	ID          string
	Name        string
	Key         string
	Description string
	Type        string
	Statements  []canonicalPolicyStatement
}

func (canonicalPolicy) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.canonicalPolicy{redacted}")
}

type canonicalPolicyStatement struct {
	ResourceType string
	Effect       string
	Actions      []string
	Resources    []string
}

func (canonicalPolicyStatement) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.canonicalPolicyStatement{redacted}")
}

func policyStatementAttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"resource_type": types.StringType,
		"effect":        types.StringType,
		"actions":       types.SetType{ElemType: types.StringType},
		"resources":     types.SetType{ElemType: types.StringType},
	}
}

func policyStatementObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: policyStatementAttributeTypes()}
}

func canonicalizePolicyPlanModel(
	ctx context.Context,
	model policyModel,
) (canonicalPolicy, error) {
	if !knownString(model.Name) || !knownString(model.Key) ||
		!knownString(model.Description) || model.Name.ValueString() == "" ||
		model.Key.ValueString() == "" {
		return canonicalPolicy{}, errInvalidPolicyDefinition
	}
	statements, err := canonicalizeTerraformPolicyStatements(ctx, model.Statements)
	if err != nil {
		return canonicalPolicy{}, errInvalidPolicyDefinition
	}
	return canonicalPolicy{
		Name:        model.Name.ValueString(),
		Key:         model.Key.ValueString(),
		Description: model.Description.ValueString(),
		Type:        client.PolicyTypeCustomerManaged,
		Statements:  statements,
	}, nil
}

func canonicalizePolicyStateModel(
	ctx context.Context,
	model policyModel,
) (canonicalPolicy, error) {
	if !knownString(model.ID) || !knownString(model.Type) {
		return canonicalPolicy{}, errInvalidPolicyDefinition
	}
	policy, err := canonicalizePolicyPlanModel(ctx, model)
	if err != nil || model.Type.ValueString() != client.PolicyTypeCustomerManaged {
		return canonicalPolicy{}, errInvalidPolicyDefinition
	}
	policyID, valid := client.CanonicalUUID(model.ID.ValueString())
	if !valid {
		return canonicalPolicy{}, errInvalidPolicyDefinition
	}
	policy.ID = policyID
	return policy, nil
}

func canonicalizeRemoteManagedPolicy(policy client.Policy) (canonicalPolicy, error) {
	policyID, valid := client.CanonicalUUID(policy.ID)
	if !valid || policy.Name == "" || policy.Key == "" ||
		policy.Type != client.PolicyTypeCustomerManaged || policy.Statements == nil {
		return canonicalPolicy{}, errInvalidPolicyDefinition
	}
	statements, err := canonicalizeManagedPolicyStatements(policy.Statements)
	if err != nil {
		return canonicalPolicy{}, err
	}
	return canonicalPolicy{
		ID:          policyID,
		Name:        policy.Name,
		Key:         policy.Key,
		Description: policy.Description,
		Type:        policy.Type,
		Statements:  statements,
	}, nil
}

func canonicalizeRemoteObservedPolicy(policy client.Policy) (canonicalPolicy, error) {
	policyID, valid := client.CanonicalUUID(policy.ID)
	if !valid || policy.Name == "" || policy.Key == "" ||
		(policy.Type != client.PolicyTypeCustomerManaged &&
			policy.Type != client.PolicyTypeSysManaged) || policy.Statements == nil {
		return canonicalPolicy{}, errInvalidPolicyDefinition
	}
	statements, err := canonicalizeObservedPolicyStatements(policy.Statements)
	if err != nil {
		return canonicalPolicy{}, err
	}
	return canonicalPolicy{
		ID:          policyID,
		Name:        policy.Name,
		Key:         policy.Key,
		Description: policy.Description,
		Type:        policy.Type,
		Statements:  statements,
	}, nil
}

func canonicalizeTerraformPolicyStatements(
	ctx context.Context,
	value types.Set,
) ([]canonicalPolicyStatement, error) {
	models, err := terraformPolicyStatementModels(ctx, value)
	if err != nil {
		return nil, err
	}
	statements := make([]client.PolicyStatement, 0, len(models))
	for _, model := range models {
		if !knownPolicyStatementModel(model) {
			return nil, errInvalidPolicyDefinition
		}
		actions, err := terraformStringSet(ctx, model.Actions)
		if err != nil {
			return nil, errInvalidPolicyDefinition
		}
		resources, err := terraformStringSet(ctx, model.Resources)
		if err != nil {
			return nil, errInvalidPolicyDefinition
		}
		statements = append(statements, client.PolicyStatement{
			ResourceType: model.ResourceType.ValueString(),
			Effect:       model.Effect.ValueString(),
			Actions:      actions,
			Resources:    resources,
		})
	}
	return canonicalizeManagedPolicyStatements(statements)
}

func canonicalizeManagedPolicyStatements(
	statements []client.PolicyStatement,
) ([]canonicalPolicyStatement, error) {
	canonical := make([]canonicalPolicyStatement, 0, len(statements))
	seen := make(map[string]struct{}, len(statements))
	for _, statement := range statements {
		catalog, exists := policyActions[statement.ResourceType]
		if !exists || (statement.Effect != policyEffectAllow &&
			statement.Effect != policyEffectDeny) || len(statement.Actions) == 0 ||
			len(statement.Resources) == 0 {
			return nil, errInvalidPolicyDefinition
		}

		actions := canonicalStringSet(statement.Actions)
		for _, action := range actions {
			if _, supported := catalog[action]; !supported {
				return nil, errInvalidPolicyDefinition
			}
		}

		resourceSet := make(map[string]struct{}, len(statement.Resources))
		resources := make([]string, 0, len(statement.Resources))
		for _, resource := range statement.Resources {
			canonicalResource, err := canonicalizePolicyResourceSelector(
				statement.ResourceType,
				resource,
			)
			if err != nil {
				return nil, errInvalidPolicyDefinition
			}
			if _, duplicate := resourceSet[canonicalResource]; duplicate {
				continue
			}
			resourceSet[canonicalResource] = struct{}{}
			resources = append(resources, canonicalResource)
		}
		sort.Strings(resources)

		item := canonicalPolicyStatement{
			ResourceType: statement.ResourceType,
			Effect:       statement.Effect,
			Actions:      actions,
			Resources:    resources,
		}
		signature := policyStatementSignature(item)
		if _, duplicate := seen[signature]; duplicate {
			continue
		}
		seen[signature] = struct{}{}
		canonical = append(canonical, item)
	}
	sortCanonicalPolicyStatements(canonical)
	return canonical, nil
}

func canonicalizeObservedPolicyStatements(
	statements []client.PolicyStatement,
) ([]canonicalPolicyStatement, error) {
	canonical := make([]canonicalPolicyStatement, 0, len(statements))
	seen := make(map[string]struct{}, len(statements))
	for _, statement := range statements {
		if statement.ResourceType == "" || statement.Effect == "" ||
			len(statement.Actions) == 0 || len(statement.Resources) == 0 {
			return nil, errInvalidPolicyDefinition
		}
		item := canonicalPolicyStatement{
			ResourceType: statement.ResourceType,
			Effect:       statement.Effect,
			Actions:      canonicalStringSet(statement.Actions),
			Resources:    canonicalStringSet(statement.Resources),
		}
		for _, action := range item.Actions {
			if action == "" {
				return nil, errInvalidPolicyDefinition
			}
		}
		for _, resource := range item.Resources {
			if resource == "" {
				return nil, errInvalidPolicyDefinition
			}
		}
		signature := policyStatementSignature(item)
		if _, duplicate := seen[signature]; duplicate {
			continue
		}
		seen[signature] = struct{}{}
		canonical = append(canonical, item)
	}
	sortCanonicalPolicyStatements(canonical)
	return canonical, nil
}

func flattenManagedPolicy(
	ctx context.Context,
	policy canonicalPolicy,
	preferred *policyModel,
) policyModel {
	model := flattenCanonicalPolicy(policy)
	if preferred == nil || preferred.Statements.IsNull() || preferred.Statements.IsUnknown() {
		return model
	}
	preferredStatements, err := canonicalizeTerraformPolicyStatements(
		ctx,
		preferred.Statements,
	)
	if err == nil && samePolicyStatements(preferredStatements, policy.Statements) {
		// Keep the known Terraform spelling when it represents the same
		// canonical statement set. This preserves configurations that differ
		// only by tag order or semantically duplicate set elements while the
		// API payload and drift comparison remain fully canonical.
		model.Statements = preferred.Statements
	}
	return model
}

func flattenObservedPolicy(policy canonicalPolicy) policyModel {
	return flattenCanonicalPolicy(policy)
}

func flattenCanonicalPolicy(policy canonicalPolicy) policyModel {
	statementValues := make([]attr.Value, 0, len(policy.Statements))
	attributeTypes := policyStatementAttributeTypes()
	for _, statement := range policy.Statements {
		statementValues = append(statementValues, types.ObjectValueMust(
			attributeTypes,
			map[string]attr.Value{
				"resource_type": types.StringValue(statement.ResourceType),
				"effect":        types.StringValue(statement.Effect),
				"actions":       terraformStringSetValue(statement.Actions),
				"resources":     terraformStringSetValue(statement.Resources),
			},
		))
	}
	return policyModel{
		ID:          types.StringValue(policy.ID),
		Name:        types.StringValue(policy.Name),
		Key:         types.StringValue(policy.Key),
		Description: types.StringValue(policy.Description),
		Type:        types.StringValue(policy.Type),
		Statements:  types.SetValueMust(policyStatementObjectType(), statementValues),
	}
}

func terraformPolicyStatementModels(
	ctx context.Context,
	value types.Set,
) ([]policyStatementModel, error) {
	if value.IsNull() || value.IsUnknown() {
		return nil, errInvalidPolicyDefinition
	}
	var models []policyStatementModel
	if diagnostics := value.ElementsAs(ctx, &models, false); diagnostics.HasError() {
		return nil, errInvalidPolicyDefinition
	}
	return models, nil
}

func knownPolicyStatementModel(model policyStatementModel) bool {
	return knownString(model.ResourceType) && knownString(model.Effect) &&
		!model.Actions.IsNull() && !model.Actions.IsUnknown() &&
		!model.Resources.IsNull() && !model.Resources.IsUnknown()
}

func expandPolicyStatements(policy canonicalPolicy) []client.PolicyStatement {
	statements := make([]client.PolicyStatement, 0, len(policy.Statements))
	for _, statement := range policy.Statements {
		statements = append(statements, client.PolicyStatement{
			ResourceType: statement.ResourceType,
			Effect:       statement.Effect,
			Actions:      append([]string(nil), statement.Actions...),
			Resources:    append([]string(nil), statement.Resources...),
		})
	}
	return statements
}

func samePolicySettings(left canonicalPolicy, right canonicalPolicy) bool {
	return left.Name == right.Name && left.Description == right.Description
}

func samePolicyStatements(left []canonicalPolicyStatement, right []canonicalPolicyStatement) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ResourceType != right[index].ResourceType ||
			left[index].Effect != right[index].Effect ||
			!sameStrings(left[index].Actions, right[index].Actions) ||
			!sameStrings(left[index].Resources, right[index].Resources) {
			return false
		}
	}
	return true
}

func samePolicyDefinition(left canonicalPolicy, right canonicalPolicy) bool {
	return left.Key == right.Key && left.Type == right.Type &&
		samePolicySettings(left, right) &&
		samePolicyStatements(left.Statements, right.Statements)
}

func sameStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sortCanonicalPolicyStatements(statements []canonicalPolicyStatement) {
	sort.SliceStable(statements, func(left int, right int) bool {
		leftStatement := statements[left]
		rightStatement := statements[right]
		if leftStatement.ResourceType != rightStatement.ResourceType {
			return leftStatement.ResourceType < rightStatement.ResourceType
		}
		if leftStatement.Effect != rightStatement.Effect {
			return leftStatement.Effect < rightStatement.Effect
		}
		if comparison := compareStringSlices(
			leftStatement.Actions,
			rightStatement.Actions,
		); comparison != 0 {
			return comparison < 0
		}
		return compareStringSlices(leftStatement.Resources, rightStatement.Resources) < 0
	})
}

func compareStringSlices(left []string, right []string) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	switch {
	case len(left) < len(right):
		return -1
	case len(left) > len(right):
		return 1
	default:
		return 0
	}
}

func policyStatementSignature(statement canonicalPolicyStatement) string {
	encoded, _ := json.Marshal(struct {
		ResourceType string
		Effect       string
		Actions      []string
		Resources    []string
	}{
		ResourceType: statement.ResourceType,
		Effect:       statement.Effect,
		Actions:      statement.Actions,
		Resources:    statement.Resources,
	})
	return string(encoded)
}

func canonicalizePolicyResourceSelector(resourceType string, value string) (string, error) {
	if value == "" || value == "*" || strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return "", errInvalidPolicyDefinition
	}

	base := value
	var tags []string
	if before, after, found := strings.Cut(value, ";"); found {
		if resourceType != policyResourceTypeFlag && resourceType != policyResourceTypeSegment {
			return "", errInvalidPolicyDefinition
		}
		base = before
		if after == "" || strings.Contains(after, ";") {
			return "", errInvalidPolicyDefinition
		}
		for _, tag := range strings.Split(after, ",") {
			if !validPolicySelectorToken(tag, false) {
				return "", errInvalidPolicyDefinition
			}
			tags = append(tags, tag)
		}
		tags = canonicalStringSet(tags)
	}

	segments := strings.Split(base, ":")
	wantSegments := 0
	switch resourceType {
	case policyResourceTypeProject:
		wantSegments = 1
	case policyResourceTypeEnv:
		wantSegments = 2
	case policyResourceTypeFlag, policyResourceTypeSegment:
		wantSegments = 3
	default:
		return "", errInvalidPolicyDefinition
	}
	if len(segments) != wantSegments {
		return "", errInvalidPolicyDefinition
	}
	wantKinds := []string{"project", "env", resourceType}
	canonicalSegments := make([]string, 0, len(segments))
	for index, segment := range segments {
		parts := strings.Split(segment, "/")
		if len(parts) != 2 || parts[0] != wantKinds[index] ||
			!validPolicySelectorToken(parts[1], true) {
			return "", errInvalidPolicyDefinition
		}
		canonicalSegments = append(canonicalSegments, parts[0]+"/"+parts[1])
	}
	canonical := strings.Join(canonicalSegments, ":")
	if len(tags) != 0 {
		canonical += ";" + strings.Join(tags, ",")
	}
	return canonical, nil
}

func validPolicySelectorToken(value string, allowWildcard bool) bool {
	if value == "" {
		return false
	}
	if allowWildcard && value == "*" {
		return true
	}
	for _, character := range value {
		if unicode.IsSpace(character) || unicode.IsControl(character) ||
			strings.ContainsRune("*/:;,", character) {
			return false
		}
	}
	return true
}

func stringCatalog(values ...string) map[string]struct{} {
	catalog := make(map[string]struct{}, len(values))
	for _, value := range values {
		catalog[value] = struct{}{}
	}
	return catalog
}
