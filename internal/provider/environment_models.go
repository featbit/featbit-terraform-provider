// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type environmentModel struct {
	ProjectID   types.String `tfsdk:"project_id"`
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Key         types.String `tfsdk:"key"`
	Description types.String `tfsdk:"description"`
}

func flattenEnvironment(projectID string, environment client.Environment) environmentModel {
	return environmentModel{
		ProjectID:   types.StringValue(projectID),
		ID:          types.StringValue(environment.ID),
		Name:        types.StringValue(environment.Name),
		Key:         types.StringValue(environment.Key),
		Description: types.StringValue(environment.Description),
	}
}
