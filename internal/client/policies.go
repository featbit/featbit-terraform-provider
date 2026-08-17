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
	policiesPath       = "policies"
	policyPageSize     = 100
	maxPolicyPageIndex = int64(1<<31 - 1)

	PolicyTypeCustomerManaged = "CustomerManaged"
	PolicyTypeSysManaged      = "SysManaged"
)

// Policy contains only the documented Policy fields consumed by Terraform.
// Audit timestamps and relationship display fields are deliberately absent.
type Policy struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Key         string            `json:"key"`
	Type        string            `json:"type"`
	Description string            `json:"description"`
	Statements  []PolicyStatement `json:"statements"`
}

// Format prevents Policy identities and resource selectors from entering
// diagnostics or logs if a response is formatted accidentally.
func (Policy) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.Policy{redacted}")
}

// PolicyStatement is the safe public statement shape. Server statement IDs
// are intentionally not decoded because Terraform owns statements by value.
type PolicyStatement struct {
	ResourceType string   `json:"resourceType"`
	Effect       string   `json:"effect"`
	Actions      []string `json:"actions"`
	Resources    []string `json:"resources"`
}

func (PolicyStatement) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.PolicyStatement{redacted}")
}

// CreatePolicyRequest is the complete documented Policy create payload.
// Statements are replaced through their specialized endpoint afterwards.
type CreatePolicyRequest struct {
	Name        string `json:"name"`
	Key         string `json:"key"`
	Description string `json:"description"`
}

func (CreatePolicyRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.CreatePolicyRequest{redacted}")
}

// UpdatePolicySettingsRequest contains every mutable Policy setting and no
// immutable key or statement data.
type UpdatePolicySettingsRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (UpdatePolicySettingsRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.UpdatePolicySettingsRequest{redacted}")
}

type policyPageWire struct {
	TotalCount *int64   `json:"totalCount"`
	Items      []Policy `json:"items"`
}

type policyAssociationWire struct {
	ID             string `json:"id"`
	IsPolicyGroup  *bool  `json:"isPolicyGroup"`
	IsPolicyMember *bool  `json:"isPolicyMember"`
}

type policyAssociationPageWire struct {
	TotalCount *int64                  `json:"totalCount"`
	Items      []policyAssociationWire `json:"items"`
}

type policyAssociationKind struct {
	pathSegment string
	queryName   string
	operation   string
	isMember    bool
}

var (
	policyGroupAssociation = policyAssociationKind{
		pathSegment: "groups",
		queryName:   "GetAllGroups",
		operation:   "list_policy_groups",
	}
	policyMemberAssociation = policyAssociationKind{
		pathSegment: "members",
		queryName:   "GetAllMembers",
		operation:   "list_policy_members",
		isMember:    true,
	}
)

// ListPolicies consumes every explicit zero-based page and refuses partial,
// duplicate-ID, or structurally incomplete collections.
func (c *Client) ListPolicies(ctx context.Context) ([]Policy, error) {
	policies := make([]Policy, 0)
	pages := newCompletePageTracker(
		"list_policies",
		policyPageSize,
		maxPolicyPageIndex,
		c.redactor,
	)

	for pageIndex := int64(0); ; pageIndex++ {
		page, statusCode, err := c.listPolicyPage(ctx, pageIndex)
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

		for _, policy := range page.Items {
			if !validPolicy(policy) {
				return nil, newAPIError(
					ClassificationAmbiguous,
					statusCode,
					"list_policies",
					nil,
					c.redactor.With(policySensitiveValues(policy)...),
				)
			}
			canonicalID, _ := CanonicalUUID(policy.ID)
			if err := pages.recordExactID(
				canonicalID,
				statusCode,
				c.redactor.With(policy.ID),
			); err != nil {
				return nil, err
			}
			policies = append(policies, policy)
		}

		complete, err := pages.pageComplete(pageIndex, statusCode)
		if err != nil {
			return nil, err
		}
		if complete {
			return policies, nil
		}
	}
}

// GetPolicy first proves exact token-scoped membership through the complete
// Policy collection, then reads the documented exact-ID endpoint. A direct
// 404 never establishes absence on its own.
func (c *Client) GetPolicy(ctx context.Context, policyID string) (Policy, bool, error) {
	if !ValidUUID(policyID) {
		return Policy{}, false, newAPIError(
			ClassificationValidation,
			0,
			"get_policy",
			nil,
			c.redactor,
		)
	}

	policies, err := c.ListPolicies(ctx)
	if err != nil {
		return Policy{}, false, err
	}
	listed, found, err := c.ResolvePolicyByID(policies, policyID)
	if err != nil || !found {
		return listed, found, err
	}

	direct, err := c.getPolicyDirect(ctx, policyID)
	if err != nil {
		return Policy{}, false, err
	}
	if !validPolicy(direct) || !EqualUUID(direct.ID, policyID) ||
		direct.Key != listed.Key || direct.Type != listed.Type {
		return Policy{}, false, newAPIError(
			ClassificationAmbiguous,
			0,
			"get_policy",
			nil,
			c.redactor.With(policySensitiveValues(direct)...),
		)
	}
	return direct, true, nil
}

// GetPolicyByKey resolves one case-sensitive exact key across the complete
// token-scoped collection and confirms it through the exact-ID endpoint.
func (c *Client) GetPolicyByKey(ctx context.Context, key string) (Policy, bool, error) {
	if key == "" {
		return Policy{}, false, newAPIError(
			ClassificationValidation,
			0,
			"get_policy_by_key",
			nil,
			c.redactor,
		)
	}
	policies, err := c.ListPolicies(ctx)
	if err != nil {
		return Policy{}, false, err
	}
	listed, found, err := c.ResolvePolicyByKey(policies, key)
	if err != nil || !found {
		return listed, found, err
	}
	direct, err := c.getPolicyDirect(ctx, listed.ID)
	if err != nil {
		return Policy{}, false, err
	}
	if !validPolicy(direct) || !EqualUUID(direct.ID, listed.ID) ||
		direct.Key != key || direct.Type != listed.Type {
		return Policy{}, false, newAPIError(
			ClassificationAmbiguous,
			0,
			"get_policy_by_key",
			nil,
			c.redactor.With(append(policySensitiveValues(direct), key)...),
		)
	}
	return direct, true, nil
}

// ResolvePolicyByID applies the shared exact zero/one/duplicate identity
// contract to an already complete Policy collection.
func (c *Client) ResolvePolicyByID(
	policies []Policy,
	policyID string,
) (Policy, bool, error) {
	var match Policy
	count := 0
	for _, policy := range policies {
		if EqualUUID(policy.ID, policyID) {
			match = policy
			count++
		}
	}
	switch count {
	case 0:
		return Policy{}, false, nil
	case 1:
		return match, true, nil
	default:
		return Policy{}, false, newAPIError(
			ClassificationAmbiguous,
			0,
			"resolve_policy",
			nil,
			c.redactor.With(policyID),
		)
	}
}

// ResolvePolicyByKey applies case-sensitive exact matching and rejects
// duplicate exact keys rather than selecting the first fuzzy result.
func (c *Client) ResolvePolicyByKey(
	policies []Policy,
	key string,
) (Policy, bool, error) {
	var match Policy
	count := 0
	for _, policy := range policies {
		if policy.Key == key {
			match = policy
			count++
		}
	}
	switch count {
	case 0:
		return Policy{}, false, nil
	case 1:
		return match, true, nil
	default:
		return Policy{}, false, newAPIError(
			ClassificationAmbiguous,
			0,
			"resolve_policy_by_key",
			nil,
			c.redactor.With(key),
		)
	}
}

// CreatePolicy executes the settings mutation exactly once. Statement
// replacement and exact-key preflight belong to the Terraform lifecycle.
func (c *Client) CreatePolicy(ctx context.Context, input CreatePolicyRequest) (Policy, error) {
	if input.Name == "" || input.Key == "" {
		return Policy{}, newAPIError(
			ClassificationValidation,
			0,
			"create_policy",
			nil,
			c.redactor,
		)
	}
	request, err := c.newPolicyJSONRequest(ctx, http.MethodPost, nil, input)
	if err != nil {
		return Policy{}, newAPIError(
			ClassificationAmbiguous,
			0,
			"create_policy",
			nil,
			c.redactor.With(input.Name, input.Key, input.Description),
		)
	}
	response, err := c.Do(request)
	if err != nil {
		return Policy{}, err
	}
	var policy Policy
	if err := c.DecodeResponse(
		"create_policy",
		response,
		&policy,
		input.Name,
		input.Key,
		input.Description,
	); err != nil {
		return Policy{}, err
	}
	if !validPolicy(policy) || policy.Type != PolicyTypeCustomerManaged ||
		policy.Key != input.Key {
		return Policy{}, newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"create_policy",
			nil,
			c.redactor.With(append(
				policySensitiveValues(policy),
				input.Name,
				input.Key,
				input.Description,
			)...),
		)
	}
	return policy, nil
}

// UpdatePolicySettings executes the specialized settings mutation exactly
// once and returns its documented full Policy response.
func (c *Client) UpdatePolicySettings(
	ctx context.Context,
	policyID string,
	input UpdatePolicySettingsRequest,
) (Policy, error) {
	if !ValidUUID(policyID) || input.Name == "" {
		return Policy{}, newAPIError(
			ClassificationValidation,
			0,
			"update_policy_settings",
			nil,
			c.redactor,
		)
	}
	request, err := c.newPolicyJSONRequest(
		ctx,
		http.MethodPut,
		[]string{policyID, "settings"},
		input,
	)
	if err != nil {
		return Policy{}, newAPIError(
			ClassificationAmbiguous,
			0,
			"update_policy_settings",
			nil,
			c.redactor.With(policyID, input.Name, input.Description),
		)
	}
	response, err := c.Do(request)
	if err != nil {
		return Policy{}, err
	}
	var policy Policy
	if err := c.DecodeResponse(
		"update_policy_settings",
		response,
		&policy,
		policyID,
		input.Name,
		input.Description,
	); err != nil {
		return Policy{}, err
	}
	if !validPolicy(policy) || !EqualUUID(policy.ID, policyID) ||
		policy.Type != PolicyTypeCustomerManaged {
		return Policy{}, newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"update_policy_settings",
			nil,
			c.redactor.With(append(policySensitiveValues(policy), policyID)...),
		)
	}
	return policy, nil
}

// ReplacePolicyStatements sends the complete statement collection exactly
// once, including an explicit empty array when Terraform owns no statements.
func (c *Client) ReplacePolicyStatements(
	ctx context.Context,
	policyID string,
	statements []PolicyStatement,
) (Policy, error) {
	if !ValidUUID(policyID) || statements == nil || !validPolicyStatements(statements) {
		return Policy{}, newAPIError(
			ClassificationValidation,
			0,
			"replace_policy_statements",
			nil,
			c.redactor,
		)
	}
	request, err := c.newPolicyJSONRequest(
		ctx,
		http.MethodPut,
		[]string{policyID, "statements"},
		statements,
	)
	if err != nil {
		return Policy{}, newAPIError(
			ClassificationAmbiguous,
			0,
			"replace_policy_statements",
			nil,
			c.redactor.With(append(
				policyStatementSensitiveValues(statements),
				policyID,
			)...),
		)
	}
	response, err := c.Do(request)
	if err != nil {
		return Policy{}, err
	}
	var policy Policy
	sensitive := append(policyStatementSensitiveValues(statements), policyID)
	if err := c.DecodeResponse(
		"replace_policy_statements",
		response,
		&policy,
		sensitive...,
	); err != nil {
		return Policy{}, err
	}
	if !validPolicy(policy) || !EqualUUID(policy.ID, policyID) ||
		policy.Type != PolicyTypeCustomerManaged {
		return Policy{}, newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"replace_policy_statements",
			nil,
			c.redactor.With(append(policySensitiveValues(policy), sensitive...)...),
		)
	}
	return policy, nil
}

// DeletePolicy executes one permanent delete and requires the documented true
// Boolean response. The lifecycle caller separately proves exact absence.
func (c *Client) DeletePolicy(ctx context.Context, policyID string) error {
	if !ValidUUID(policyID) {
		return newAPIError(
			ClassificationValidation,
			0,
			"delete_policy",
			nil,
			c.redactor,
		)
	}
	request, err := c.newPolicyRequest(ctx, http.MethodDelete, []string{policyID})
	if err != nil {
		return newTransportError(err)
	}
	response, err := c.Do(request)
	if err != nil {
		return err
	}
	var deleted bool
	if err := c.DecodeResponse("delete_policy", response, &deleted, policyID); err != nil {
		return err
	}
	if !deleted {
		return newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"delete_policy",
			nil,
			c.redactor.With(policyID),
		)
	}
	return nil
}

// CountPolicyGroups consumes the complete collection of exact Group
// associations without retaining Group names or descriptions.
func (c *Client) CountPolicyGroups(ctx context.Context, policyID string) (int, error) {
	return c.countPolicyAssociations(ctx, policyID, policyGroupAssociation)
}

// CountPolicyMembers consumes the complete collection of direct Member
// associations while decoding only UUID and membership status. Member names
// and emails are never represented in this adapter.
func (c *Client) CountPolicyMembers(ctx context.Context, policyID string) (int, error) {
	return c.countPolicyAssociations(ctx, policyID, policyMemberAssociation)
}

func (c *Client) countPolicyAssociations(
	ctx context.Context,
	policyID string,
	kind policyAssociationKind,
) (int, error) {
	if !ValidUUID(policyID) {
		return 0, newAPIError(
			ClassificationValidation,
			0,
			kind.operation,
			nil,
			c.redactor,
		)
	}
	pages := newCompletePageTracker(
		kind.operation,
		policyPageSize,
		maxPolicyPageIndex,
		c.redactor.With(policyID),
	)
	count := 0
	for pageIndex := int64(0); ; pageIndex++ {
		page, statusCode, err := c.listPolicyAssociationPage(
			ctx,
			policyID,
			kind,
			pageIndex,
		)
		if err != nil {
			return 0, err
		}
		if err := pages.validatePage(
			page.TotalCount,
			page.Items != nil,
			len(page.Items),
			statusCode,
		); err != nil {
			return 0, err
		}
		for _, association := range page.Items {
			membership := association.IsPolicyGroup
			if kind.isMember {
				membership = association.IsPolicyMember
			}
			canonicalID, valid := CanonicalUUID(association.ID)
			if !valid || membership == nil || !*membership {
				return 0, newAPIError(
					ClassificationAmbiguous,
					statusCode,
					kind.operation,
					nil,
					c.redactor.With(policyID, association.ID),
				)
			}
			if err := pages.recordExactID(
				canonicalID,
				statusCode,
				c.redactor.With(policyID, association.ID),
			); err != nil {
				return 0, err
			}
			count++
		}
		complete, err := pages.pageComplete(pageIndex, statusCode)
		if err != nil {
			return 0, err
		}
		if complete {
			return count, nil
		}
	}
}

func (c *Client) getPolicyDirect(ctx context.Context, policyID string) (Policy, error) {
	request, err := c.newPolicyRequest(ctx, http.MethodGet, []string{policyID})
	if err != nil {
		return Policy{}, newTransportError(err)
	}
	response, err := c.Do(request)
	if err != nil {
		return Policy{}, err
	}
	var policy Policy
	if err := c.DecodeResponse("get_policy", response, &policy, policyID); err != nil {
		return Policy{}, readErrorWithoutDetails(
			"get_policy",
			err,
			c.redactor.With(policyID),
		)
	}
	return policy, nil
}

func (c *Client) listPolicyPage(
	ctx context.Context,
	pageIndex int64,
) (policyPageWire, int, error) {
	request, err := c.newPolicyRequest(ctx, http.MethodGet, nil)
	if err != nil {
		return policyPageWire{}, 0, newTransportError(err)
	}
	query := request.URL.Query()
	query.Set("PageIndex", strconv.FormatInt(pageIndex, 10))
	query.Set("PageSize", strconv.Itoa(policyPageSize))
	request.URL.RawQuery = query.Encode()

	response, err := c.Do(request)
	if err != nil {
		return policyPageWire{}, 0, err
	}
	var page policyPageWire
	if err := c.DecodeResponse("list_policies", response, &page); err != nil {
		return policyPageWire{}, response.StatusCode, readErrorWithoutDetails(
			"list_policies",
			err,
			c.redactor,
		)
	}
	return page, response.StatusCode, nil
}

func (c *Client) listPolicyAssociationPage(
	ctx context.Context,
	policyID string,
	kind policyAssociationKind,
	pageIndex int64,
) (policyAssociationPageWire, int, error) {
	request, err := c.newPolicyRequest(
		ctx,
		http.MethodGet,
		[]string{policyID, kind.pathSegment},
	)
	if err != nil {
		return policyAssociationPageWire{}, 0, newTransportError(err)
	}
	query := request.URL.Query()
	query.Set(kind.queryName, "false")
	query.Set("PageIndex", strconv.FormatInt(pageIndex, 10))
	query.Set("PageSize", strconv.Itoa(policyPageSize))
	request.URL.RawQuery = query.Encode()

	response, err := c.Do(request)
	if err != nil {
		return policyAssociationPageWire{}, 0, err
	}
	var page policyAssociationPageWire
	if err := c.DecodeResponse(kind.operation, response, &page, policyID); err != nil {
		return policyAssociationPageWire{}, response.StatusCode, readErrorWithoutDetails(
			kind.operation,
			err,
			c.redactor.With(policyID),
		)
	}
	return page, response.StatusCode, nil
}

func (c *Client) newPolicyRequest(
	ctx context.Context,
	method string,
	segments []string,
) (*http.Request, error) {
	return c.newRequest(ctx, method, policyPath(segments), nil)
}

func (c *Client) newPolicyJSONRequest(
	ctx context.Context,
	method string,
	segments []string,
	payload any,
) (*http.Request, error) {
	return c.newJSONRequest(ctx, method, policyPath(segments), payload)
}

func policyPath(segments []string) []string {
	return append([]string{policiesPath}, segments...)
}

func validPolicy(policy Policy) bool {
	if !ValidUUID(policy.ID) || policy.Name == "" || policy.Key == "" ||
		(policy.Type != PolicyTypeCustomerManaged && policy.Type != PolicyTypeSysManaged) ||
		policy.Statements == nil {
		return false
	}
	return validPolicyStatements(policy.Statements)
}

func validPolicyStatements(statements []PolicyStatement) bool {
	for _, statement := range statements {
		if statement.ResourceType == "" || statement.Effect == "" ||
			len(statement.Actions) == 0 || len(statement.Resources) == 0 {
			return false
		}
		for _, action := range statement.Actions {
			if action == "" {
				return false
			}
		}
		for _, resource := range statement.Resources {
			if resource == "" {
				return false
			}
		}
	}
	return true
}

func policySensitiveValues(policy Policy) []string {
	values := []string{
		policy.ID,
		policy.Name,
		policy.Key,
		policy.Description,
	}
	return append(values, policyStatementSensitiveValues(policy.Statements)...)
}

func policyStatementSensitiveValues(statements []PolicyStatement) []string {
	values := make([]string, 0)
	for _, statement := range statements {
		values = append(values, statement.ResourceType, statement.Effect)
		values = append(values, statement.Actions...)
		values = append(values, statement.Resources...)
	}
	return values
}
