// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"sort"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type projectModel struct {
	ID           types.String              `tfsdk:"id"`
	Name         types.String              `tfsdk:"name"`
	Key          types.String              `tfsdk:"key"`
	Environments []projectEnvironmentModel `tfsdk:"environments"`
}

type projectEnvironmentModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Key         types.String `tfsdk:"key"`
	Description types.String `tfsdk:"description"`
}

func flattenProject(project client.Project) projectModel {
	environments := append([]client.ProjectEnvironment(nil), project.Environments...)
	sort.SliceStable(environments, func(left, right int) bool {
		leftEnvironment := environments[left]
		rightEnvironment := environments[right]
		if leftEnvironment.Key != rightEnvironment.Key {
			return leftEnvironment.Key < rightEnvironment.Key
		}
		if leftEnvironment.ID != rightEnvironment.ID {
			return leftEnvironment.ID < rightEnvironment.ID
		}
		if leftEnvironment.Name != rightEnvironment.Name {
			return leftEnvironment.Name < rightEnvironment.Name
		}
		return leftEnvironment.Description < rightEnvironment.Description
	})

	model := projectModel{
		ID:           types.StringValue(project.ID),
		Name:         types.StringValue(project.Name),
		Key:          types.StringValue(project.Key),
		Environments: make([]projectEnvironmentModel, 0, len(environments)),
	}
	for _, environment := range environments {
		model.Environments = append(model.Environments, projectEnvironmentModel{
			ID:          types.StringValue(environment.ID),
			Name:        types.StringValue(environment.Name),
			Key:         types.StringValue(environment.Key),
			Description: types.StringValue(environment.Description),
		})
	}
	return model
}
