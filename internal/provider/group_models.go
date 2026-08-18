// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"errors"
	"fmt"
	"io"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var errInvalidGroupDefinition = errors.New("Group definition is invalid")

type groupModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

func (groupModel) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.groupModel{redacted}")
}

func canonicalizeGroupPlanModel(model groupModel) (client.Group, error) {
	if !knownString(model.Name) || !knownString(model.Description) ||
		model.Name.ValueString() == "" {
		return client.Group{}, errInvalidGroupDefinition
	}
	return client.Group{
		Name:        model.Name.ValueString(),
		Description: model.Description.ValueString(),
	}, nil
}

func canonicalizeGroupStateModel(model groupModel) (client.Group, error) {
	group, err := canonicalizeGroupPlanModel(model)
	if err != nil || !knownString(model.ID) {
		return client.Group{}, errInvalidGroupDefinition
	}
	groupID, valid := client.CanonicalUUID(model.ID.ValueString())
	if !valid {
		return client.Group{}, errInvalidGroupDefinition
	}
	group.ID = groupID
	return group, nil
}

func canonicalizeRemoteGroup(group client.Group) (client.Group, error) {
	groupID, valid := client.CanonicalUUID(group.ID)
	if !valid || group.Name == "" {
		return client.Group{}, errInvalidGroupDefinition
	}
	group.ID = groupID
	return group, nil
}

func flattenGroup(group client.Group) groupModel {
	return groupModel{
		ID:          types.StringValue(group.ID),
		Name:        types.StringValue(group.Name),
		Description: types.StringValue(group.Description),
	}
}

func sameGroupSettings(left client.Group, right client.Group) bool {
	return left.Name == right.Name && left.Description == right.Description
}
