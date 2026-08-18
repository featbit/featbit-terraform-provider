// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

const (
	groupsPath        = "groups"
	groupPageSize     = 100
	maxGroupPageIndex = int64(1<<31 - 1)
)

// Group contains only the documented Group settings owned by Terraform.
// Relationship collections and audit timestamps are deliberately absent.
type Group struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Format prevents Group identities and settings from entering diagnostics or
// logs if a response is formatted accidentally.
func (Group) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.Group{redacted}")
}

// CreateGroupRequest is the complete documented Group create payload. The
// organization context is derived from the access token rather than sent in
// the body.
type CreateGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (CreateGroupRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.CreateGroupRequest{redacted}")
}

// UpdateGroupRequest contains every mutable Group setting. Relationship
// membership is owned by separate Terraform resources.
type UpdateGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (UpdateGroupRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.UpdateGroupRequest{redacted}")
}

type groupPageWire struct {
	TotalCount *int64  `json:"totalCount"`
	Items      []Group `json:"items"`
}

type groupAssociationWire struct {
	ID            string `json:"id"`
	IsGroupMember *bool  `json:"isGroupMember"`
	IsGroupPolicy *bool  `json:"isGroupPolicy"`
}

type groupAssociationPageWire struct {
	TotalCount *int64                 `json:"totalCount"`
	Items      []groupAssociationWire `json:"items"`
}

type groupAssociationKind struct {
	pathSegment string
	queryName   string
	operation   string
	isPolicy    bool
}

var (
	groupMemberAssociation = groupAssociationKind{
		pathSegment: "members",
		queryName:   "GetAllMembers",
		operation:   "list_group_members",
	}
	groupPolicyAssociation = groupAssociationKind{
		pathSegment: "policies",
		queryName:   "GetAllPolicies",
		operation:   "list_group_policies",
		isPolicy:    true,
	}
)

// ListGroups consumes every explicit zero-based page and refuses partial,
// duplicate-ID, or structurally incomplete collections.
func (c *Client) ListGroups(ctx context.Context) ([]Group, error) {
	groups := make([]Group, 0)
	pages := newCompletePageTracker(
		"list_groups",
		groupPageSize,
		maxGroupPageIndex,
		c.redactor,
	)

	for pageIndex := int64(0); ; pageIndex++ {
		page, statusCode, err := c.listGroupPage(ctx, pageIndex)
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

		for _, group := range page.Items {
			if !validGroup(group) {
				return nil, newAPIError(
					ClassificationAmbiguous,
					statusCode,
					"list_groups",
					nil,
					c.redactor.With(groupSensitiveValues(group)...),
				)
			}
			canonicalID, _ := CanonicalUUID(group.ID)
			if err := pages.recordExactID(
				canonicalID,
				statusCode,
				c.redactor.With(group.ID),
			); err != nil {
				return nil, err
			}
			groups = append(groups, group)
		}

		complete, err := pages.pageComplete(pageIndex, statusCode)
		if err != nil {
			return nil, err
		}
		if complete {
			return groups, nil
		}
	}
}

// GetGroup first proves exact token-scoped membership through the complete
// Group collection, then reads the documented exact-ID endpoint. A direct 404
// never establishes absence on its own.
func (c *Client) GetGroup(ctx context.Context, groupID string) (Group, bool, error) {
	if !ValidUUID(groupID) {
		return Group{}, false, newAPIError(
			ClassificationValidation,
			0,
			"get_group",
			nil,
			c.redactor,
		)
	}

	groups, err := c.ListGroups(ctx)
	if err != nil {
		return Group{}, false, err
	}
	listed, found, err := c.ResolveGroupByID(groups, groupID)
	if err != nil || !found {
		return listed, found, err
	}

	direct, err := c.getGroupDirect(ctx, listed.ID)
	if err != nil {
		return Group{}, false, err
	}
	if !validGroup(direct) || !EqualUUID(direct.ID, listed.ID) {
		return Group{}, false, newAPIError(
			ClassificationAmbiguous,
			0,
			"get_group",
			nil,
			c.redactor.With(groupSensitiveValues(direct)...),
		)
	}
	return direct, true, nil
}

// GetGroupByName resolves one organization-scoped case-sensitive exact name
// across the complete token-scoped collection, then confirms it through the
// exact-ID endpoint. A concurrent rename is ambiguous rather than a match.
func (c *Client) GetGroupByName(ctx context.Context, name string) (Group, bool, error) {
	if name == "" {
		return Group{}, false, newAPIError(
			ClassificationValidation,
			0,
			"get_group_by_name",
			nil,
			c.redactor,
		)
	}
	groups, err := c.ListGroups(ctx)
	if err != nil {
		return Group{}, false, err
	}
	listed, found, err := c.ResolveGroupByName(groups, name)
	if err != nil || !found {
		return listed, found, err
	}
	direct, err := c.getGroupDirect(ctx, listed.ID)
	if err != nil {
		return Group{}, false, err
	}
	if !validGroup(direct) || !EqualUUID(direct.ID, listed.ID) || direct.Name != name {
		return Group{}, false, newAPIError(
			ClassificationAmbiguous,
			0,
			"get_group_by_name",
			nil,
			c.redactor.With(append(groupSensitiveValues(direct), name)...),
		)
	}
	return direct, true, nil
}

// ResolveGroupByID applies the shared exact zero/one/duplicate identity
// contract to an already complete Group collection.
func (c *Client) ResolveGroupByID(groups []Group, groupID string) (Group, bool, error) {
	return resolveExactOne(
		groups,
		func(group Group) bool { return EqualUUID(group.ID, groupID) },
		"resolve_group",
		c.redactor.With(groupID),
	)
}

// ResolveGroupByName applies case-sensitive exact matching and rejects
// duplicate exact names rather than selecting the first fuzzy result.
func (c *Client) ResolveGroupByName(groups []Group, name string) (Group, bool, error) {
	return resolveExactOne(
		groups,
		func(group Group) bool { return group.Name == name },
		"resolve_group_by_name",
		c.redactor.With(name),
	)
}

// CreateGroup executes the documented mutation exactly once. Exact-name
// preflight and ambiguous-outcome reconciliation belong to the Terraform
// lifecycle caller.
func (c *Client) CreateGroup(ctx context.Context, input CreateGroupRequest) (Group, error) {
	if input.Name == "" {
		return Group{}, newAPIError(
			ClassificationValidation,
			0,
			"create_group",
			nil,
			c.redactor,
		)
	}
	request, err := c.newGroupJSONRequest(ctx, http.MethodPost, nil, input)
	if err != nil {
		return Group{}, newAPIError(
			ClassificationAmbiguous,
			0,
			"create_group",
			nil,
			c.redactor.With(input.Name, input.Description),
		)
	}
	response, err := c.Do(request)
	if err != nil {
		return Group{}, err
	}
	var group Group
	if err := c.DecodeResponse(
		"create_group",
		response,
		&group,
		input.Name,
		input.Description,
	); err != nil {
		return Group{}, err
	}
	if !validGroup(group) || group.Name != input.Name {
		return Group{}, newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"create_group",
			nil,
			c.redactor.With(append(
				groupSensitiveValues(group),
				input.Name,
				input.Description,
			)...),
		)
	}
	return group, nil
}

// UpdateGroup executes the documented settings mutation exactly once and
// returns its full Group response. The lifecycle caller performs the exact
// canonical reread and reconciliation.
func (c *Client) UpdateGroup(
	ctx context.Context,
	groupID string,
	input UpdateGroupRequest,
) (Group, error) {
	if !ValidUUID(groupID) || input.Name == "" {
		return Group{}, newAPIError(
			ClassificationValidation,
			0,
			"update_group",
			nil,
			c.redactor,
		)
	}
	request, err := c.newGroupJSONRequest(
		ctx,
		http.MethodPut,
		[]string{groupID},
		input,
	)
	if err != nil {
		return Group{}, newAPIError(
			ClassificationAmbiguous,
			0,
			"update_group",
			nil,
			c.redactor.With(groupID, input.Name, input.Description),
		)
	}
	response, err := c.Do(request)
	if err != nil {
		return Group{}, err
	}
	var group Group
	if err := c.DecodeResponse(
		"update_group",
		response,
		&group,
		groupID,
		input.Name,
		input.Description,
	); err != nil {
		return Group{}, err
	}
	if !validGroup(group) || !EqualUUID(group.ID, groupID) {
		return Group{}, newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"update_group",
			nil,
			c.redactor.With(append(groupSensitiveValues(group), groupID)...),
		)
	}
	return group, nil
}

// DeleteGroup executes one permanent delete and requires the documented true
// Boolean response. The lifecycle caller separately proves exact absence.
func (c *Client) DeleteGroup(ctx context.Context, groupID string) error {
	if !ValidUUID(groupID) {
		return newAPIError(
			ClassificationValidation,
			0,
			"delete_group",
			nil,
			c.redactor,
		)
	}
	request, err := c.newGroupRequest(ctx, http.MethodDelete, []string{groupID})
	if err != nil {
		return newTransportError(err)
	}
	response, err := c.Do(request)
	if err != nil {
		return err
	}
	var deleted bool
	if err := c.DecodeResponse("delete_group", response, &deleted, groupID); err != nil {
		return err
	}
	if !deleted {
		return newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"delete_group",
			nil,
			c.redactor.With(groupID),
		)
	}
	return nil
}

// CountGroupMembers consumes the complete exact Group member collection while
// decoding only UUID and membership status.
func (c *Client) CountGroupMembers(ctx context.Context, groupID string) (int, error) {
	associationIDs, err := c.listGroupAssociationIDs(ctx, groupID, groupMemberAssociation)
	return len(associationIDs), err
}

// ListGroupPolicyIDs consumes the complete exact Group Policy collection and
// returns only canonical Policy UUIDs. Policy display fields and ownership
// types are deliberately not decoded by this relationship adapter.
func (c *Client) ListGroupPolicyIDs(ctx context.Context, groupID string) ([]string, error) {
	return c.listGroupAssociationIDs(ctx, groupID, groupPolicyAssociation)
}

// CountGroupPolicies derives the association count from the same complete ID
// collection used by exact-pair binding resources.
func (c *Client) CountGroupPolicies(ctx context.Context, groupID string) (int, error) {
	associationIDs, err := c.ListGroupPolicyIDs(ctx, groupID)
	return len(associationIDs), err
}

// AddGroupPolicy executes the documented exact-pair mutation once. Endpoint
// existence validation and authoritative relationship rereads belong to the
// Terraform lifecycle caller because the API accepts missing IDs as no-ops.
func (c *Client) AddGroupPolicy(ctx context.Context, groupID string, policyID string) error {
	return c.mutateGroupPolicy(ctx, groupID, policyID, "add-policy", "add_group_policy")
}

// RemoveGroupPolicy executes the documented exact-pair removal once.
func (c *Client) RemoveGroupPolicy(ctx context.Context, groupID string, policyID string) error {
	return c.mutateGroupPolicy(ctx, groupID, policyID, "remove-policy", "remove_group_policy")
}

func (c *Client) listGroupAssociationIDs(
	ctx context.Context,
	groupID string,
	kind groupAssociationKind,
) ([]string, error) {
	return listCompleteAssociationIDs(
		ctx,
		kind.operation,
		groupID,
		groupPageSize,
		maxGroupPageIndex,
		c.redactor,
		func(ctx context.Context, pageIndex int64) (exactAssociationPage, int, error) {
			return c.listGroupAssociationPage(ctx, groupID, kind, pageIndex)
		},
	)
}

func (c *Client) mutateGroupPolicy(
	ctx context.Context,
	groupID string,
	policyID string,
	pathSegment string,
	operation string,
) error {
	if !ValidUUID(groupID) || !ValidUUID(policyID) {
		return newAPIError(
			ClassificationValidation,
			0,
			operation,
			nil,
			c.redactor,
		)
	}
	request, err := c.newGroupRequest(
		ctx,
		http.MethodPut,
		[]string{groupID, pathSegment, policyID},
	)
	if err != nil {
		return newTransportError(err)
	}
	response, err := c.Do(request)
	if err != nil {
		return err
	}
	var changed bool
	if err := c.DecodeResponse(
		operation,
		response,
		&changed,
		groupID,
		policyID,
	); err != nil {
		return err
	}
	if !changed {
		return newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			operation,
			nil,
			c.redactor.With(groupID, policyID),
		)
	}
	return nil
}

func (c *Client) getGroupDirect(ctx context.Context, groupID string) (Group, error) {
	request, err := c.newGroupRequest(ctx, http.MethodGet, []string{groupID})
	if err != nil {
		return Group{}, newTransportError(err)
	}
	response, err := c.Do(request)
	if err != nil {
		return Group{}, err
	}
	var group Group
	if err := c.DecodeResponse("get_group", response, &group, groupID); err != nil {
		return Group{}, readErrorWithoutDetails(
			"get_group",
			err,
			c.redactor.With(groupID),
		)
	}
	return group, nil
}

func (c *Client) listGroupPage(
	ctx context.Context,
	pageIndex int64,
) (groupPageWire, int, error) {
	request, err := c.newGroupRequest(ctx, http.MethodGet, nil)
	if err != nil {
		return groupPageWire{}, 0, newTransportError(err)
	}
	query := request.URL.Query()
	query.Set("PageIndex", strconv.FormatInt(pageIndex, 10))
	query.Set("PageSize", strconv.Itoa(groupPageSize))
	request.URL.RawQuery = query.Encode()

	response, err := c.Do(request)
	if err != nil {
		return groupPageWire{}, 0, err
	}
	var page groupPageWire
	if err := c.DecodeResponse("list_groups", response, &page); err != nil {
		return groupPageWire{}, response.StatusCode, readErrorWithoutDetails(
			"list_groups",
			err,
			c.redactor,
		)
	}
	return page, response.StatusCode, nil
}

func (c *Client) listGroupAssociationPage(
	ctx context.Context,
	groupID string,
	kind groupAssociationKind,
	pageIndex int64,
) (exactAssociationPage, int, error) {
	request, err := c.newGroupRequest(
		ctx,
		http.MethodGet,
		[]string{groupID, kind.pathSegment},
	)
	if err != nil {
		return exactAssociationPage{}, 0, newTransportError(err)
	}
	query := request.URL.Query()
	query.Set(kind.queryName, "false")
	query.Set("PageIndex", strconv.FormatInt(pageIndex, 10))
	query.Set("PageSize", strconv.Itoa(groupPageSize))
	request.URL.RawQuery = query.Encode()

	response, err := c.Do(request)
	if err != nil {
		return exactAssociationPage{}, 0, err
	}
	var page groupAssociationPageWire
	if err := c.DecodeResponse(kind.operation, response, &page, groupID); err != nil {
		return exactAssociationPage{}, response.StatusCode, readErrorWithoutDetails(
			kind.operation,
			err,
			c.redactor.With(groupID),
		)
	}
	var items []exactAssociation
	if page.Items != nil {
		items = make([]exactAssociation, 0, len(page.Items))
		for _, association := range page.Items {
			membership := association.IsGroupMember
			if kind.isPolicy {
				membership = association.IsGroupPolicy
			}
			items = append(items, exactAssociation{
				ID:      association.ID,
				Present: membership,
			})
		}
	}
	return exactAssociationPage{
		TotalCount: page.TotalCount,
		Items:      items,
	}, response.StatusCode, nil
}

func (c *Client) newGroupRequest(
	ctx context.Context,
	method string,
	segments []string,
) (*http.Request, error) {
	return c.newRequest(ctx, method, groupPath(segments), nil)
}

func (c *Client) newGroupJSONRequest(
	ctx context.Context,
	method string,
	segments []string,
	payload any,
) (*http.Request, error) {
	return c.newJSONRequest(ctx, method, groupPath(segments), payload)
}

func groupPath(segments []string) []string {
	return append([]string{groupsPath}, segments...)
}

func validGroup(group Group) bool {
	return ValidUUID(group.ID) && group.Name != ""
}

func groupSensitiveValues(group Group) []string {
	return []string{group.ID, group.Name, group.Description}
}
