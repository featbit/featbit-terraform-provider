// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const (
	membersPath        = "members"
	memberPageSize     = 100
	maxMemberPageIndex = int64(1<<31 - 1)
)

// Member is an explicit safe allowlist for existing Member reads. The public
// response also contains invitation and initial-password fields, which must
// never enter a Provider model.
type Member struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// Format prevents Member identities from entering diagnostics or logs if a
// response is formatted accidentally.
func (Member) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.Member{redacted}")
}

type memberPageWire struct {
	TotalCount *int64   `json:"totalCount"`
	Items      []Member `json:"items"`
}

type memberPolicyAssociationWire struct {
	ID             string `json:"id"`
	IsMemberPolicy *bool  `json:"isMemberPolicy"`
}

type memberPolicyAssociationPageWire struct {
	TotalCount *int64                        `json:"totalCount"`
	Items      []memberPolicyAssociationWire `json:"items"`
}

// ListMembers consumes every explicit zero-based page and refuses partial,
// duplicate-ID, or structurally incomplete collections. No server-side fuzzy
// search is used for exact Member selection.
func (c *Client) ListMembers(ctx context.Context) ([]Member, error) {
	members := make([]Member, 0)
	pages := newCompletePageTracker(
		"list_members",
		memberPageSize,
		maxMemberPageIndex,
		c.redactor,
	)

	for pageIndex := int64(0); ; pageIndex++ {
		page, statusCode, err := c.listMemberPage(ctx, pageIndex)
		if err != nil {
			return nil, err
		}
		if err := pages.validatePage(
			page.TotalCount,
			page.Items != nil,
			len(page.Items),
			statusCode,
		); err != nil {
			return nil, err
		}

		for _, member := range page.Items {
			if !validMember(member) {
				return nil, newAPIError(
					ClassificationAmbiguous,
					statusCode,
					"list_members",
					nil,
					c.redactor.With(memberSensitiveValues(member)...),
				)
			}
			canonicalID, _ := CanonicalUUID(member.ID)
			if err := pages.recordExactID(
				canonicalID,
				statusCode,
				c.redactor.With(memberSensitiveValues(member)...),
			); err != nil {
				return nil, err
			}
			members = append(members, member)
		}

		complete, err := pages.pageComplete(pageIndex, statusCode)
		if err != nil {
			return nil, err
		}
		if complete {
			return members, nil
		}
	}
}

// GetMember first proves exact token-scoped membership through the complete
// Member collection, then reads the documented exact-ID endpoint. A direct
// 404 never establishes absence on its own.
func (c *Client) GetMember(ctx context.Context, memberID string) (Member, bool, error) {
	if !ValidUUID(memberID) {
		return Member{}, false, newAPIError(
			ClassificationValidation,
			0,
			"get_member",
			nil,
			c.redactor,
		)
	}

	members, err := c.ListMembers(ctx)
	if err != nil {
		return Member{}, false, err
	}
	listed, found, err := c.ResolveMemberByID(members, memberID)
	if err != nil || !found {
		return listed, found, err
	}

	direct, err := c.getMemberDirect(ctx, listed.ID)
	if err != nil {
		return Member{}, false, err
	}
	if !validMember(direct) || !EqualUUID(direct.ID, listed.ID) {
		return Member{}, false, newAPIError(
			ClassificationAmbiguous,
			0,
			"get_member",
			nil,
			c.redactor.With(memberSensitiveValues(direct)...),
		)
	}
	return direct, true, nil
}

// GetMemberByEmail resolves one organization-scoped case-insensitive exact
// full email across the complete token-scoped collection, then confirms it
// through the exact-ID endpoint. State retains the server's canonical email
// spelling.
func (c *Client) GetMemberByEmail(
	ctx context.Context,
	email string,
) (Member, bool, error) {
	if email == "" {
		return Member{}, false, newAPIError(
			ClassificationValidation,
			0,
			"get_member_by_email",
			nil,
			c.redactor,
		)
	}
	members, err := c.ListMembers(ctx)
	if err != nil {
		return Member{}, false, err
	}
	listed, found, err := c.ResolveMemberByEmail(members, email)
	if err != nil || !found {
		return listed, found, err
	}
	direct, err := c.getMemberDirect(ctx, listed.ID)
	if err != nil {
		return Member{}, false, err
	}
	if !validMember(direct) || !EqualUUID(direct.ID, listed.ID) ||
		!strings.EqualFold(direct.Email, email) {
		return Member{}, false, newAPIError(
			ClassificationAmbiguous,
			0,
			"get_member_by_email",
			nil,
			c.redactor.With(append(memberSensitiveValues(direct), email)...),
		)
	}
	return direct, true, nil
}

// ResolveMemberByID applies the shared exact zero/one/duplicate identity
// contract to an already complete Member collection.
func (c *Client) ResolveMemberByID(
	members []Member,
	memberID string,
) (Member, bool, error) {
	return resolveExactOne(
		members,
		func(member Member) bool { return EqualUUID(member.ID, memberID) },
		"resolve_member",
		c.redactor.With(memberID),
	)
}

// ResolveMemberByEmail applies case-insensitive full-string equality and
// rejects duplicate exact emails rather than accepting a fuzzy search result.
func (c *Client) ResolveMemberByEmail(
	members []Member,
	email string,
) (Member, bool, error) {
	return resolveExactOne(
		members,
		func(member Member) bool { return strings.EqualFold(member.Email, email) },
		"resolve_member_by_email",
		c.redactor.With(email),
	)
}

// ListMemberDirectPolicyIDs consumes the complete direct-Policy collection
// for one Member and returns canonical Policy UUIDs only. It deliberately
// never reads the combined or inherited Policy endpoints as owned state.
func (c *Client) ListMemberDirectPolicyIDs(
	ctx context.Context,
	memberID string,
) ([]string, error) {
	return listCompleteAssociationIDs(
		ctx,
		"list_member_direct_policies",
		memberID,
		memberPageSize,
		maxMemberPageIndex,
		c.redactor,
		func(ctx context.Context, pageIndex int64) (exactAssociationPage, int, error) {
			return c.listMemberDirectPolicyPage(ctx, memberID, pageIndex)
		},
	)
}

// AddMemberDirectPolicy executes one documented direct-Policy mutation. The
// lifecycle caller validates endpoint membership and performs the exact
// authoritative reread because the API accepts missing IDs as no-ops.
func (c *Client) AddMemberDirectPolicy(
	ctx context.Context,
	memberID string,
	policyID string,
) error {
	return c.mutateMemberDirectPolicy(
		ctx,
		memberID,
		policyID,
		"add-policy",
		"add_member_direct_policy",
	)
}

// RemoveMemberDirectPolicy executes one documented direct-Policy removal.
func (c *Client) RemoveMemberDirectPolicy(
	ctx context.Context,
	memberID string,
	policyID string,
) error {
	return c.mutateMemberDirectPolicy(
		ctx,
		memberID,
		policyID,
		"remove-policy",
		"remove_member_direct_policy",
	)
}

func (c *Client) getMemberDirect(ctx context.Context, memberID string) (Member, error) {
	request, err := c.newMemberRequest(ctx, http.MethodGet, []string{memberID})
	if err != nil {
		return Member{}, newTransportError(err)
	}
	response, err := c.Do(request)
	if err != nil {
		return Member{}, err
	}
	var member Member
	if err := c.DecodeResponse("get_member", response, &member, memberID); err != nil {
		return Member{}, readErrorWithoutDetails(
			"get_member",
			err,
			c.redactor.With(memberID),
		)
	}
	return member, nil
}

func (c *Client) listMemberPage(
	ctx context.Context,
	pageIndex int64,
) (memberPageWire, int, error) {
	request, err := c.newMemberRequest(ctx, http.MethodGet, nil)
	if err != nil {
		return memberPageWire{}, 0, newTransportError(err)
	}
	query := request.URL.Query()
	query.Set("PageIndex", strconv.FormatInt(pageIndex, 10))
	query.Set("PageSize", strconv.Itoa(memberPageSize))
	request.URL.RawQuery = query.Encode()

	response, err := c.Do(request)
	if err != nil {
		return memberPageWire{}, 0, err
	}
	var page memberPageWire
	if err := c.DecodeResponse("list_members", response, &page); err != nil {
		return memberPageWire{}, response.StatusCode, readErrorWithoutDetails(
			"list_members",
			err,
			c.redactor,
		)
	}
	return page, response.StatusCode, nil
}

func (c *Client) listMemberDirectPolicyPage(
	ctx context.Context,
	memberID string,
	pageIndex int64,
) (exactAssociationPage, int, error) {
	request, err := c.newMemberRequest(
		ctx,
		http.MethodGet,
		[]string{memberID, "direct-policies"},
	)
	if err != nil {
		return exactAssociationPage{}, 0, newTransportError(err)
	}
	query := request.URL.Query()
	query.Set("GetAllPolicies", "false")
	query.Set("PageIndex", strconv.FormatInt(pageIndex, 10))
	query.Set("PageSize", strconv.Itoa(memberPageSize))
	request.URL.RawQuery = query.Encode()

	response, err := c.Do(request)
	if err != nil {
		return exactAssociationPage{}, 0, err
	}
	var page memberPolicyAssociationPageWire
	if err := c.DecodeResponse(
		"list_member_direct_policies",
		response,
		&page,
		memberID,
	); err != nil {
		return exactAssociationPage{}, response.StatusCode, readErrorWithoutDetails(
			"list_member_direct_policies",
			err,
			c.redactor.With(memberID),
		)
	}
	var items []exactAssociation
	if page.Items != nil {
		items = make([]exactAssociation, 0, len(page.Items))
		for _, association := range page.Items {
			items = append(items, exactAssociation{
				ID:      association.ID,
				Present: association.IsMemberPolicy,
			})
		}
	}
	return exactAssociationPage{
		TotalCount: page.TotalCount,
		Items:      items,
	}, response.StatusCode, nil
}

func (c *Client) mutateMemberDirectPolicy(
	ctx context.Context,
	memberID string,
	policyID string,
	pathSegment string,
	operation string,
) error {
	return c.mutateExactAssociation(
		ctx,
		memberID,
		policyID,
		operation,
		func(ctx context.Context) (*http.Request, error) {
			return c.newMemberRequest(
				ctx,
				http.MethodPut,
				[]string{memberID, pathSegment, policyID},
			)
		},
	)
}

func (c *Client) newMemberRequest(
	ctx context.Context,
	method string,
	segments []string,
) (*http.Request, error) {
	return c.newRequest(ctx, method, memberPath(segments), nil)
}

func memberPath(segments []string) []string {
	return append([]string{membersPath}, segments...)
}

func validMember(member Member) bool {
	return ValidUUID(member.ID) && member.Email != ""
}

func memberSensitiveValues(member Member) []string {
	return []string{member.ID, member.Email, member.Name}
}
