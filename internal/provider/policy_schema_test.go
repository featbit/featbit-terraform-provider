// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func TestPolicyResourceSchemaFreezesOwnershipAndReplacement(t *testing.T) {
	t.Parallel()

	var response resource.SchemaResponse
	(&policyResource{}).Schema(context.Background(), resource.SchemaRequest{}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("Policy resource schema diagnostics = %v", response.Diagnostics)
	}
	policySchema := response.Schema
	if got := len(policySchema.Attributes); got != 6 {
		t.Fatalf("attribute count = %d, want 6", got)
	}
	id, ok := policySchema.Attributes["id"].(resourceschema.StringAttribute)
	if !ok || !id.Computed || id.Required || id.Optional || len(id.PlanModifiers) != 1 {
		t.Fatalf("id attribute = %#v", policySchema.Attributes["id"])
	}
	name, ok := policySchema.Attributes["name"].(resourceschema.StringAttribute)
	if !ok || !name.Required || name.Optional || name.Computed || len(name.Validators) != 1 {
		t.Fatalf("name attribute = %#v", policySchema.Attributes["name"])
	}
	key, ok := policySchema.Attributes["key"].(resourceschema.StringAttribute)
	if !ok || !key.Required || key.Optional || key.Computed ||
		len(key.Validators) != 1 || len(key.PlanModifiers) != 1 {
		t.Fatalf("key attribute = %#v", policySchema.Attributes["key"])
	}
	description, ok := policySchema.Attributes["description"].(resourceschema.StringAttribute)
	if !ok || !description.Optional || !description.Computed || description.Required ||
		description.Default == nil {
		t.Fatalf("description attribute = %#v", policySchema.Attributes["description"])
	}
	policyType, ok := policySchema.Attributes["type"].(resourceschema.StringAttribute)
	if !ok || !policyType.Computed || policyType.Required || policyType.Optional {
		t.Fatalf("type attribute = %#v", policySchema.Attributes["type"])
	}
	statements, ok := policySchema.Attributes["statements"].(resourceschema.SetNestedAttribute)
	if !ok || !statements.Required || statements.Optional || statements.Computed ||
		len(statements.Validators) != 1 || len(statements.NestedObject.Attributes) != 4 {
		t.Fatalf("statements attribute = %#v", policySchema.Attributes["statements"])
	}
	for _, field := range []string{"resource_type", "effect"} {
		attribute, ok := statements.NestedObject.Attributes[field].(resourceschema.StringAttribute)
		if !ok || !attribute.Required || len(attribute.Validators) != 1 {
			t.Fatalf("statement %s attribute = %#v", field, statements.NestedObject.Attributes[field])
		}
	}
	for _, field := range []string{"actions", "resources"} {
		attribute, ok := statements.NestedObject.Attributes[field].(resourceschema.SetAttribute)
		if !ok || !attribute.Required || len(attribute.Validators) != 1 {
			t.Fatalf("statement %s attribute = %#v", field, statements.NestedObject.Attributes[field])
		}
	}
}

func TestPolicyDataSourceSchemaKeepsBuiltInObservationReadOnly(t *testing.T) {
	t.Parallel()

	var response datasource.SchemaResponse
	(&policyDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("Policy data source schema diagnostics = %v", response.Diagnostics)
	}
	policySchema := response.Schema
	if got := len(policySchema.Attributes); got != 6 {
		t.Fatalf("attribute count = %d, want 6", got)
	}
	for _, field := range []string{"id", "key"} {
		attribute, ok := policySchema.Attributes[field].(datasourceschema.StringAttribute)
		if !ok || !attribute.Optional || !attribute.Computed || attribute.Required {
			t.Fatalf("selector %s attribute = %#v", field, policySchema.Attributes[field])
		}
	}
	for _, field := range []string{"name", "description", "type"} {
		attribute, ok := policySchema.Attributes[field].(datasourceschema.StringAttribute)
		if !ok || !attribute.Computed || attribute.Optional || attribute.Required {
			t.Fatalf("computed %s attribute = %#v", field, policySchema.Attributes[field])
		}
	}
	statements, ok := policySchema.Attributes["statements"].(datasourceschema.SetNestedAttribute)
	if !ok || !statements.Computed || statements.Required || statements.Optional ||
		len(statements.NestedObject.Attributes) != 4 {
		t.Fatalf("statements attribute = %#v", policySchema.Attributes["statements"])
	}
	for name, attribute := range statements.NestedObject.Attributes {
		switch typed := attribute.(type) {
		case datasourceschema.StringAttribute:
			if !typed.Computed || typed.Required || typed.Optional {
				t.Fatalf("statement %s string attribute = %#v", name, typed)
			}
		case datasourceschema.SetAttribute:
			if !typed.Computed || typed.Required || typed.Optional {
				t.Fatalf("statement %s set attribute = %#v", name, typed)
			}
		default:
			t.Fatalf("statement %s attribute type = %T", name, attribute)
		}
	}
}
