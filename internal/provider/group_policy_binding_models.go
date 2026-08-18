// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var errInvalidGroupPolicyBinding = errors.New("Group-Policy binding identity is invalid")

type groupPolicyBindingModel struct {
	ID       types.String `tfsdk:"id"`
	GroupID  types.String `tfsdk:"group_id"`
	PolicyID types.String `tfsdk:"policy_id"`
}

func (groupPolicyBindingModel) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.groupPolicyBindingModel{redacted}")
}

type groupPolicyBindingIdentity struct {
	GroupID  string
	PolicyID string
}

func (groupPolicyBindingIdentity) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.groupPolicyBindingIdentity{redacted}")
}

func (identity groupPolicyBindingIdentity) syntheticID() string {
	return identity.GroupID + "/" + identity.PolicyID
}

func canonicalizeGroupPolicyBindingPlanModel(
	model groupPolicyBindingModel,
) (groupPolicyBindingIdentity, error) {
	if !knownString(model.GroupID) || !knownString(model.PolicyID) {
		return groupPolicyBindingIdentity{}, errInvalidGroupPolicyBinding
	}
	groupID, groupValid := client.CanonicalUUID(model.GroupID.ValueString())
	policyID, policyValid := client.CanonicalUUID(model.PolicyID.ValueString())
	if !groupValid || !policyValid {
		return groupPolicyBindingIdentity{}, errInvalidGroupPolicyBinding
	}
	return groupPolicyBindingIdentity{GroupID: groupID, PolicyID: policyID}, nil
}

func canonicalizeGroupPolicyBindingStateModel(
	model groupPolicyBindingModel,
) (groupPolicyBindingIdentity, error) {
	identity, err := canonicalizeGroupPolicyBindingPlanModel(model)
	if err != nil || !knownString(model.ID) {
		return groupPolicyBindingIdentity{}, errInvalidGroupPolicyBinding
	}
	stateIdentity, valid := canonicalizeGroupPolicyBindingImportID(model.ID.ValueString())
	if !valid || stateIdentity != identity {
		return groupPolicyBindingIdentity{}, errInvalidGroupPolicyBinding
	}
	return identity, nil
}

func canonicalizeGroupPolicyBindingImportID(
	value string,
) (groupPolicyBindingIdentity, bool) {
	components := strings.Split(value, "/")
	if len(components) != 2 {
		return groupPolicyBindingIdentity{}, false
	}
	groupID, groupValid := client.CanonicalUUID(components[0])
	policyID, policyValid := client.CanonicalUUID(components[1])
	if !groupValid || !policyValid {
		return groupPolicyBindingIdentity{}, false
	}
	return groupPolicyBindingIdentity{GroupID: groupID, PolicyID: policyID}, true
}

func flattenGroupPolicyBinding(
	identity groupPolicyBindingIdentity,
) groupPolicyBindingModel {
	return groupPolicyBindingModel{
		ID:       types.StringValue(identity.syntheticID()),
		GroupID:  types.StringValue(identity.GroupID),
		PolicyID: types.StringValue(identity.PolicyID),
	}
}
