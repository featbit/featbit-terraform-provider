// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"slices"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const providerPolicyID = "55555555-5555-4555-8555-555555555555"

func TestCanonicalizeManagedPolicyStatementsCoversAllControlLevels(t *testing.T) {
	t.Parallel()

	remote := client.Policy{
		ID:          providerPolicyID,
		Name:        "Managed",
		Key:         "managed",
		Type:        client.PolicyTypeCustomerManaged,
		Description: "Description",
		Statements: []client.PolicyStatement{
			{
				ResourceType: policyResourceTypeSegment,
				Effect:       policyEffectDeny,
				Actions:      []string{"UpdateSegmentRules", "*", "UpdateSegmentRules"},
				Resources: []string{
					"project/Project:env/Prod:segment/Checkout;zeta,Alpha,zeta",
					"project/*:env/*:segment/*",
				},
			},
			{
				ResourceType: policyResourceTypeProject,
				Effect:       policyEffectAllow,
				Actions:      []string{"CreateEnv", "CanAccessProject"},
				Resources:    []string{"project/Exact", "project/*"},
			},
			{
				ResourceType: policyResourceTypeFlag,
				Effect:       policyEffectAllow,
				Actions:      []string{"ToggleFlag", "*"},
				Resources: []string{
					"project/Project:env/Dev:flag/checkout;beta,Alpha,beta",
				},
			},
			{
				ResourceType: policyResourceTypeEnv,
				Effect:       policyEffectDeny,
				Actions:      []string{"UpdateEnvSettings", "CanAccessEnv"},
				Resources:    []string{"project/Project:env/*"},
			},
		},
	}

	canonical, err := canonicalizeRemoteManagedPolicy(remote)
	if err != nil {
		t.Fatalf("canonicalizeRemoteManagedPolicy() error = %v", err)
	}
	if canonical.ID != providerPolicyID || canonical.Type != client.PolicyTypeCustomerManaged ||
		len(canonical.Statements) != 4 {
		t.Fatalf("canonical Policy = %#v", canonical)
	}
	wantTypes := []string{"env", "flag", "project", "segment"}
	for index, want := range wantTypes {
		if canonical.Statements[index].ResourceType != want {
			t.Fatalf("statement %d type = %q, want %q", index, canonical.Statements[index].ResourceType, want)
		}
	}
	flag := canonical.Statements[1]
	if !slices.Equal(flag.Actions, []string{"*", "ToggleFlag"}) ||
		!slices.Equal(flag.Resources, []string{
			"project/Project:env/Dev:flag/checkout;Alpha,beta",
		}) {
		t.Fatalf("canonical Flag statement = %#v", flag)
	}
	segment := canonical.Statements[3]
	if !slices.Equal(segment.Actions, []string{"*", "UpdateSegmentRules"}) ||
		!slices.Equal(segment.Resources, []string{
			"project/*:env/*:segment/*",
			"project/Project:env/Prod:segment/Checkout;Alpha,zeta",
		}) {
		t.Fatalf("canonical Segment statement = %#v", segment)
	}
}

func TestManagedPolicyStatementValidationRejectsUnsafeCatalogAndSelectors(t *testing.T) {
	t.Parallel()

	tests := map[string]client.PolicyStatement{
		"unknown resource type": {
			ResourceType: "*", Effect: "allow", Actions: []string{"*"}, Resources: []string{"*"},
		},
		"uppercase effect": {
			ResourceType: "project", Effect: "Allow", Actions: []string{"CreateProject"}, Resources: []string{"project/*"},
		},
		"wrong action catalog": {
			ResourceType: "env", Effect: "allow", Actions: []string{"CreateProject"}, Resources: []string{"project/*:env/*"},
		},
		"project star action": {
			ResourceType: "project", Effect: "allow", Actions: []string{"*"}, Resources: []string{"project/*"},
		},
		"global selector": {
			ResourceType: "flag", Effect: "allow", Actions: []string{"*"}, Resources: []string{"*"},
		},
		"partial glob": {
			ResourceType: "flag", Effect: "allow", Actions: []string{"*"}, Resources: []string{"project/proj*:env/*:flag/*"},
		},
		"wrong hierarchy": {
			ResourceType: "segment", Effect: "allow", Actions: []string{"*"}, Resources: []string{"project/p:segment/s"},
		},
		"tag on environment": {
			ResourceType: "env", Effect: "allow", Actions: []string{"CanAccessEnv"}, Resources: []string{"project/p:env/e;tag"},
		},
		"empty tag": {
			ResourceType: "flag", Effect: "allow", Actions: []string{"*"}, Resources: []string{"project/p:env/e:flag/f;tag,"},
		},
		"whitespace": {
			ResourceType: "project", Effect: "allow", Actions: []string{"CreateProject"}, Resources: []string{"project/project key"},
		},
		"reserved delimiter": {
			ResourceType: "project", Effect: "allow", Actions: []string{"CreateProject"}, Resources: []string{"project/key/extra"},
		},
	}

	for name, statement := range tests {
		name := name
		statement := statement
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := canonicalizeManagedPolicyStatements([]client.PolicyStatement{statement}); err == nil {
				t.Fatalf("canonicalizeManagedPolicyStatements() accepted %#v", statement)
			}
		})
	}
}

func TestObservedBuiltInPolicyDoesNotWidenManagedCatalog(t *testing.T) {
	t.Parallel()

	builtIn := client.Policy{
		ID:          providerPolicyID,
		Name:        "Owner",
		Key:         "Owner",
		Type:        client.PolicyTypeSysManaged,
		Description: "",
		Statements: []client.PolicyStatement{{
			ResourceType: "*",
			Effect:       "allow",
			Actions:      []string{"*"},
			Resources:    []string{"*"},
		}},
	}
	observed, err := canonicalizeRemoteObservedPolicy(builtIn)
	if err != nil || len(observed.Statements) != 1 || observed.Statements[0].ResourceType != "*" {
		t.Fatalf("canonicalizeRemoteObservedPolicy() = %#v/%v", observed, err)
	}
	if _, err := canonicalizeRemoteManagedPolicy(builtIn); err == nil {
		t.Fatal("canonicalizeRemoteManagedPolicy() accepted a built-in Policy")
	}
	if _, err := canonicalizeManagedPolicyStatements(builtIn.Statements); err == nil {
		t.Fatal("managed statement catalog accepted built-in wildcard shape")
	}
}

func TestFlattenManagedPolicyPreservesEquivalentConfiguredSelectorSpelling(t *testing.T) {
	t.Parallel()

	preferred := policyTestModel(
		t,
		providerPolicyID,
		"Managed",
		"managed",
		"Description",
		[]client.PolicyStatement{{
			ResourceType: "flag",
			Effect:       "allow",
			Actions:      []string{"ToggleFlag"},
			Resources: []string{
				"project/P:env/E:flag/F;zeta,Alpha,zeta",
				"project/P:env/E:flag/F;Alpha,zeta",
			},
		}},
	)
	planned, err := canonicalizePolicyPlanModel(context.Background(), preferred)
	if err != nil {
		t.Fatalf("canonicalizePolicyPlanModel() error = %v", err)
	}
	planned.ID = providerPolicyID
	flattened := flattenManagedPolicy(context.Background(), planned, &preferred)
	models, err := terraformPolicyStatementModels(context.Background(), flattened.Statements)
	if err != nil || len(models) != 1 {
		t.Fatalf("flattened statements = %#v/%v", models, err)
	}
	resources, err := terraformStringSet(context.Background(), models[0].Resources)
	if err != nil || !slices.Equal(resources, []string{
		"project/P:env/E:flag/F;Alpha,zeta",
		"project/P:env/E:flag/F;zeta,Alpha,zeta",
	}) {
		t.Fatalf("flattened resources = %v/%v", resources, err)
	}
	if !slices.Equal(planned.Statements[0].Resources, []string{
		"project/P:env/E:flag/F;Alpha,zeta",
	}) {
		t.Fatalf("canonical resources = %v", planned.Statements[0].Resources)
	}
}

func policyTestModel(
	t *testing.T,
	id string,
	name string,
	key string,
	description string,
	statements []client.PolicyStatement,
) policyModel {
	t.Helper()
	values := make([]attr.Value, 0, len(statements))
	attributeTypes := policyStatementAttributeTypes()
	for _, statement := range statements {
		values = append(values, types.ObjectValueMust(attributeTypes, map[string]attr.Value{
			"resource_type": types.StringValue(statement.ResourceType),
			"effect":        types.StringValue(statement.Effect),
			"actions":       terraformStringSetValue(statement.Actions),
			"resources":     terraformStringSetValue(statement.Resources),
		}))
	}
	idValue := types.StringValue(id)
	if id == "" {
		idValue = types.StringUnknown()
	}
	return policyModel{
		ID:          idValue,
		Name:        types.StringValue(name),
		Key:         types.StringValue(key),
		Description: types.StringValue(description),
		Type:        types.StringValue(client.PolicyTypeCustomerManaged),
		Statements:  types.SetValueMust(policyStatementObjectType(), values),
	}
}
