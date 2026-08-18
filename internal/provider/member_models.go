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

var errInvalidMemberDefinition = errors.New("Member definition is invalid")

// memberModel is the complete safe Terraform shape for an existing Member.
// Every value is Sensitive in the schema; invitation and credential fields do
// not exist in either this model or client.Member.
type memberModel struct {
	ID    types.String `tfsdk:"id"`
	Email types.String `tfsdk:"email"`
	Name  types.String `tfsdk:"name"`
}

func (memberModel) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.memberModel{redacted}")
}

func canonicalizeRemoteMember(member client.Member) (client.Member, error) {
	memberID, valid := client.CanonicalUUID(member.ID)
	if !valid || member.Email == "" {
		return client.Member{}, errInvalidMemberDefinition
	}
	member.ID = memberID
	return member, nil
}

func flattenMember(member client.Member) memberModel {
	return memberModel{
		ID:    types.StringValue(member.ID),
		Email: types.StringValue(member.Email),
		Name:  types.StringValue(member.Name),
	}
}
