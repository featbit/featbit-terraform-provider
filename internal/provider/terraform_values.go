// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var errInvalidTerraformStringSet = errors.New("Terraform string set is null, unknown, or invalid")

func canonicalStringSet(values []string) []string {
	canonical := slices.Clone(values)
	slices.Sort(canonical)
	return slices.Compact(canonical)
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
		return nil, errInvalidTerraformStringSet
	}
	var values []string
	if diagnostics := value.ElementsAs(ctx, &values, false); diagnostics.HasError() {
		return nil, errInvalidTerraformStringSet
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
