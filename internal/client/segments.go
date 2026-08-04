// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const (
	segmentsPath              = "segments"
	segmentFlagReferencesPath = "flag-references"
	segmentNamePath           = "name"
	segmentDescriptionPath    = "description"
	segmentTargetingPath      = "targeting"
	segmentTagsPath           = "tags"
	segmentPageSize           = 100
	maxSegmentPageIndex       = int64(1<<31 - 1)
)

// SegmentType is the documented public Segment taxonomy. Terraform may
// observe both values, but only environment-specific Segments are mutable.
type SegmentType string

const (
	SegmentTypeEnvironmentSpecific SegmentType = "environment-specific"
	SegmentTypeShared              SegmentType = "shared"
)

// SegmentScopeKind identifies the documented resource level encoded by a
// Segment scope RN.
type SegmentScopeKind string

const (
	SegmentScopeOrganization SegmentScopeKind = "organization"
	SegmentScopeProject      SegmentScopeKind = "project"
	SegmentScopeEnvironment  SegmentScopeKind = "environment"
)

// ClassifySegmentScope validates the current public resource-RN encoding and
// returns its most specific resource level. Scope values remain opaque and
// case-sensitive; this function never trims or rewrites their keys.
func ClassifySegmentScope(scope string) (SegmentScopeKind, bool) {
	parts := strings.Split(scope, ":")
	if len(parts) < 1 || len(parts) > 3 {
		return "", false
	}

	expectedTypes := [...]string{"organization", "project", "env"}
	for index, part := range parts {
		slash := strings.IndexByte(part, '/')
		if slash <= 0 || slash == len(part)-1 ||
			part[:slash] != expectedTypes[index] ||
			strings.Contains(part[slash+1:], "/") ||
			strings.Contains(part[slash+1:], "*") {
			return "", false
		}
	}

	switch len(parts) {
	case 1:
		return SegmentScopeOrganization, true
	case 2:
		return SegmentScopeProject, true
	case 3:
		return SegmentScopeEnvironment, true
	default:
		return "", false
	}
}

// Segment contains only the complete public definition fields consumed by
// the Terraform Segment lifecycle. Workspace/audit fields, pending changes,
// and other server-owned values are deliberately absent.
type Segment struct {
	ID            string        `json:"id"`
	EnvironmentID string        `json:"envId"`
	Name          string        `json:"name"`
	Key           string        `json:"key"`
	Type          SegmentType   `json:"type"`
	Scopes        []string      `json:"scopes"`
	Description   string        `json:"description"`
	Included      []string      `json:"included"`
	Excluded      []string      `json:"excluded"`
	Rules         []SegmentRule `json:"rules"`
	Tags          []string      `json:"tags"`
	IsArchived    bool          `json:"isArchived"`
}

// Format prevents Segment identities, user keys, targeting values, tags, and
// scopes from entering diagnostics or logs through accidental formatting.
func (Segment) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.Segment{redacted}")
}

// SegmentRule preserves the documented rule and condition evaluation order.
type SegmentRule struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Conditions []SegmentCondition `json:"conditions"`
}

// Format applies the Segment redaction boundary to an independently formatted
// rule.
func (SegmentRule) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.SegmentRule{redacted}")
}

// SegmentCondition preserves the public operator and string value encoding.
type SegmentCondition struct {
	ID       string `json:"id"`
	Property string `json:"property"`
	Operator string `json:"op"`
	Value    string `json:"value"`
}

// Format applies the Segment redaction boundary to an independently formatted
// condition.
func (SegmentCondition) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.SegmentCondition{redacted}")
}

// CreateSegmentRequest is the complete documented Segment create payload.
// The lifecycle-owned caller only permits the environment-specific taxonomy;
// targeting and tags are initialized through their specialized endpoints.
type CreateSegmentRequest struct {
	Type        SegmentType `json:"type"`
	Name        string      `json:"name"`
	Key         string      `json:"key"`
	Description string      `json:"description"`
	Scopes      []string    `json:"scopes"`
}

// Format prevents a Segment create payload from exposing runtime definition
// values if it is accidentally formatted in a diagnostic or log.
func (CreateSegmentRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.CreateSegmentRequest{redacted}")
}

// UpdateSegmentNameRequest is the documented specialized name payload.
// Optional audit comments and path identity are deliberately absent.
type UpdateSegmentNameRequest struct {
	Name string `json:"name"`
}

// Format applies the Segment redaction boundary to name mutations.
func (UpdateSegmentNameRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.UpdateSegmentNameRequest{redacted}")
}

// UpdateSegmentDescriptionRequest is the documented specialized description
// payload. Optional audit comments and path identity are deliberately absent.
type UpdateSegmentDescriptionRequest struct {
	Description string `json:"description"`
}

// Format applies the Segment redaction boundary to description mutations.
func (UpdateSegmentDescriptionRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.UpdateSegmentDescriptionRequest{redacted}")
}

// UpdateSegmentTargetingRequest is the documented specialized targeting
// payload. Optional audit comments and path identity are deliberately absent.
type UpdateSegmentTargetingRequest struct {
	Included []string      `json:"included"`
	Excluded []string      `json:"excluded"`
	Rules    []SegmentRule `json:"rules"`
}

// Format applies the Segment redaction boundary to targeting mutations.
func (UpdateSegmentTargetingRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.UpdateSegmentTargetingRequest{redacted}")
}

// UpdateSegmentTagsRequest is the documented specialized tag payload.
// Optional audit comments and the route-owned Segment UUID are omitted.
type UpdateSegmentTagsRequest struct {
	Tags []string `json:"tags"`
}

// Format applies the Segment redaction boundary to tag mutations.
func (UpdateSegmentTagsRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.UpdateSegmentTagsRequest{redacted}")
}

// SegmentMatch is the intentionally incomplete collection shape used only for
// authoritative identity, taxonomy, and active/archive status resolution. It
// cannot be flattened as complete Segment targeting state.
type SegmentMatch struct {
	ID            string      `json:"id"`
	EnvironmentID string      `json:"envId"`
	Key           string      `json:"key"`
	Type          SegmentType `json:"type"`
	Scopes        []string    `json:"scopes"`
	IsArchived    bool        `json:"isArchived"`
}

// Format prevents a collection match from exposing runtime identity or scope
// values.
func (SegmentMatch) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.SegmentMatch{redacted}")
}

// SegmentIdentity selects an exact UUID, an exact case-sensitive key, or both.
// When both are supplied, the collection resolver requires them to identify
// the same single object and fails closed on any partial match.
type SegmentIdentity struct {
	ID  string
	Key string
}

// Format prevents identity values from being exposed through diagnostics.
func (SegmentIdentity) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.SegmentIdentity{redacted}")
}

// SegmentStatus is the authoritative state composed from complete active and
// archived collection views.
type SegmentStatus string

const (
	SegmentStatusUnknown  SegmentStatus = ""
	SegmentStatusAbsent   SegmentStatus = "absent"
	SegmentStatusActive   SegmentStatus = "active"
	SegmentStatusArchived SegmentStatus = "archived"
)

// SegmentFlagReference is one exact Feature Flag reference returned by the
// dedicated Segment preflight endpoint.
type SegmentFlagReference struct {
	EnvironmentID string `json:"envId"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	Key           string `json:"key"`
}

// Format prevents Feature Flag reference identities from entering diagnostics
// or logs.
func (SegmentFlagReference) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.SegmentFlagReference{redacted}")
}

type segmentWire struct {
	ID                    string        `json:"id"`
	EnvironmentID         string        `json:"envId"`
	Name                  *string       `json:"name"`
	Key                   *string       `json:"key"`
	Type                  *SegmentType  `json:"type"`
	Scopes                []string      `json:"scopes"`
	Description           *string       `json:"description"`
	Included              []string      `json:"included"`
	Excluded              []string      `json:"excluded"`
	Rules                 []SegmentRule `json:"rules"`
	Tags                  []string      `json:"tags"`
	IsArchived            *bool         `json:"isArchived"`
	IsEnvironmentSpecific *bool         `json:"isEnvironmentSpecific"`
}

// segmentMatchWire is the safe union of SegmentVm list fields and optional
// exact-response context fields. Targeting fields are intentionally absent.
type segmentMatchWire struct {
	ID                    string       `json:"id"`
	EnvironmentID         string       `json:"envId"`
	Key                   *string      `json:"key"`
	Type                  *SegmentType `json:"type"`
	Scopes                []string     `json:"scopes"`
	IsArchived            *bool        `json:"isArchived"`
	IsEnvironmentSpecific *bool        `json:"isEnvironmentSpecific"`
}

type segmentPageWire struct {
	TotalCount *int64             `json:"totalCount"`
	Items      []segmentMatchWire `json:"items"`
}

// GetSegment reads one complete exact object through the documented
// environment/UUID endpoint. A direct 404 remains unconfirmed; callers that
// need authoritative absence must use ResolveSegment.
func (c *Client) GetSegment(
	ctx context.Context,
	environmentID string,
	segmentID string,
) (Segment, error) {
	if !ValidUUID(environmentID) || !ValidUUID(segmentID) {
		return Segment{}, newAPIError(
			ClassificationValidation,
			0,
			"get_segment",
			nil,
			c.redactor,
		)
	}

	request, err := c.newSegmentRequest(
		ctx,
		http.MethodGet,
		environmentID,
		[]string{segmentID},
	)
	if err != nil {
		return Segment{}, newTransportError(err)
	}

	response, err := c.Do(request)
	if err != nil {
		return Segment{}, err
	}
	var wire segmentWire
	if err := c.DecodeResponse(
		"get_segment",
		response,
		&wire,
		environmentID,
		segmentID,
	); err != nil {
		return Segment{}, segmentReadError(
			"get_segment",
			err,
			c.redactor.With(environmentID, segmentID),
		)
	}

	segment, valid := segmentFromWire(wire, environmentID)
	if !valid || !EqualUUID(segment.ID, segmentID) {
		return Segment{}, newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"get_segment",
			nil,
			c.redactor.With(environmentID, segmentID, wire.ID),
		)
	}
	return segment, nil
}

// ListSegments returns one complete, structurally reconciled active or
// archived collection. Name is explicitly empty so the documented fuzzy name
// filter cannot narrow an authoritative identity or absence proof.
func (c *Client) ListSegments(
	ctx context.Context,
	environmentID string,
	archived bool,
) ([]SegmentMatch, error) {
	return c.listSegments(ctx, environmentID, archived)
}

// ResolveSegment always consumes complete active and archived views before
// resolving an exact UUID, exact case-sensitive key, or consistent pair.
func (c *Client) ResolveSegment(
	ctx context.Context,
	environmentID string,
	identity SegmentIdentity,
) (SegmentMatch, SegmentStatus, error) {
	if !ValidUUID(environmentID) || !validSegmentIdentity(identity) {
		return SegmentMatch{}, SegmentStatusUnknown, newAPIError(
			ClassificationValidation,
			0,
			"resolve_segment",
			nil,
			c.redactor,
		)
	}

	sensitiveValues := []string{identity.ID, identity.Key}
	active, err := c.listSegments(ctx, environmentID, false, sensitiveValues...)
	if err != nil {
		return SegmentMatch{}, SegmentStatusUnknown, err
	}
	archived, err := c.listSegments(ctx, environmentID, true, sensitiveValues...)
	if err != nil {
		return SegmentMatch{}, SegmentStatusUnknown, err
	}

	return resolveSegment(
		active,
		archived,
		identity,
		c.redactor.With(environmentID, identity.ID, identity.Key),
	)
}

// CreateSegment executes the documented mutation exactly once. Exact-zero
// preflight, canonical read-after-write, specialized initialization, and
// ambiguous-outcome reconciliation belong to the Terraform lifecycle caller.
// A structurally inconsistent successful response still returns its valid UUID
// so the caller can establish recoverable provisional state without mutation
// replay or silent adoption.
func (c *Client) CreateSegment(
	ctx context.Context,
	environmentID string,
	input CreateSegmentRequest,
) (Segment, error) {
	if !ValidUUID(environmentID) || !validCreateSegmentRequest(input) {
		return Segment{}, newAPIError(
			ClassificationValidation,
			0,
			"create_segment",
			nil,
			c.redactor,
		)
	}

	sensitiveValues := segmentCreateSensitiveValues(environmentID, input)
	request, err := c.newSegmentJSONRequest(
		ctx,
		http.MethodPost,
		environmentID,
		nil,
		input,
	)
	if err != nil {
		return Segment{}, newAPIError(
			ClassificationAmbiguous,
			0,
			"create_segment",
			nil,
			c.redactor.With(sensitiveValues...),
		)
	}

	response, err := c.Do(request)
	if err != nil {
		return Segment{}, err
	}
	var wire segmentWire
	if err := c.DecodeResponse(
		"create_segment",
		response,
		&wire,
		sensitiveValues...,
	); err != nil {
		return Segment{}, segmentReadError(
			"create_segment",
			err,
			c.redactor.With(sensitiveValues...),
		)
	}

	canonicalID, idValid := CanonicalUUID(wire.ID)
	if !idValid {
		return Segment{}, newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"create_segment",
			nil,
			c.redactor.With(sensitiveValues...).With(wire.ID),
		)
	}
	segment, valid := segmentFromWire(wire, environmentID)
	if !valid {
		return Segment{ID: canonicalID, EnvironmentID: environmentID}, newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"create_segment",
			nil,
			c.redactor.With(sensitiveValues...).With(wire.ID),
		)
	}
	if segment.IsArchived || segment.Key != input.Key ||
		segment.Name != input.Name || segment.Description != input.Description ||
		segment.Type != input.Type || !sameSegmentStringSet(segment.Scopes, input.Scopes) {
		return segment, newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"create_segment",
			nil,
			c.redactor.With(sensitiveValues...).With(wire.ID),
		)
	}
	return segment, nil
}

// UpdateSegmentName executes the documented specialized name mutation exactly
// once. The lifecycle caller owns diffing, reconciliation, and canonical
// read-after-write state.
func (c *Client) UpdateSegmentName(
	ctx context.Context,
	environmentID string,
	segmentID string,
	input UpdateSegmentNameRequest,
) error {
	if !ValidUUID(environmentID) || !ValidUUID(segmentID) ||
		strings.TrimSpace(input.Name) == "" {
		return newAPIError(
			ClassificationValidation,
			0,
			"update_segment_name",
			nil,
			c.redactor,
		)
	}
	return c.mutateSegmentBoolean(
		ctx,
		http.MethodPut,
		environmentID,
		segmentID,
		[]string{segmentID, segmentNamePath},
		input,
		"update_segment_name",
		[]string{environmentID, segmentID, input.Name},
	)
}

// UpdateSegmentDescription executes the documented specialized description
// mutation exactly once. The lifecycle caller owns diffing, reconciliation,
// and canonical read-after-write state.
func (c *Client) UpdateSegmentDescription(
	ctx context.Context,
	environmentID string,
	segmentID string,
	input UpdateSegmentDescriptionRequest,
) error {
	if !ValidUUID(environmentID) || !ValidUUID(segmentID) {
		return newAPIError(
			ClassificationValidation,
			0,
			"update_segment_description",
			nil,
			c.redactor,
		)
	}
	return c.mutateSegmentBoolean(
		ctx,
		http.MethodPut,
		environmentID,
		segmentID,
		[]string{segmentID, segmentDescriptionPath},
		input,
		"update_segment_description",
		[]string{environmentID, segmentID, input.Description},
	)
}

// UpdateSegmentTargeting executes the documented specialized targeting
// mutation exactly once. The lifecycle caller owns diffing, reconciliation,
// and canonical read-after-write state.
func (c *Client) UpdateSegmentTargeting(
	ctx context.Context,
	environmentID string,
	segmentID string,
	input UpdateSegmentTargetingRequest,
) error {
	if !ValidUUID(environmentID) || !ValidUUID(segmentID) ||
		!validSegmentTargetingRequest(input) {
		return newAPIError(
			ClassificationValidation,
			0,
			"update_segment_targeting",
			nil,
			c.redactor,
		)
	}
	return c.mutateSegmentBoolean(
		ctx,
		http.MethodPut,
		environmentID,
		segmentID,
		[]string{segmentID, segmentTargetingPath},
		input,
		"update_segment_targeting",
		segmentTargetingSensitiveValues(environmentID, segmentID, input),
	)
}

// UpdateSegmentTags executes the documented specialized tag mutation exactly
// once. The lifecycle caller owns diffing, reconciliation, and canonical
// read-after-write state.
func (c *Client) UpdateSegmentTags(
	ctx context.Context,
	environmentID string,
	segmentID string,
	input UpdateSegmentTagsRequest,
) error {
	if !ValidUUID(environmentID) || !ValidUUID(segmentID) || input.Tags == nil {
		return newAPIError(
			ClassificationValidation,
			0,
			"update_segment_tags",
			nil,
			c.redactor,
		)
	}
	return c.mutateSegmentBoolean(
		ctx,
		http.MethodPut,
		environmentID,
		segmentID,
		[]string{segmentID, segmentTagsPath},
		input,
		"update_segment_tags",
		append([]string{environmentID, segmentID}, input.Tags...),
	)
}

func (c *Client) mutateSegmentBoolean(
	ctx context.Context,
	method string,
	environmentID string,
	segmentID string,
	segments []string,
	payload any,
	operation string,
	sensitiveValues []string,
) error {
	request, err := c.newSegmentJSONRequest(
		ctx,
		method,
		environmentID,
		segments,
		payload,
	)
	if err != nil {
		return newAPIError(
			ClassificationAmbiguous,
			0,
			operation,
			nil,
			c.redactor.With(sensitiveValues...),
		)
	}

	response, err := c.Do(request)
	if err != nil {
		return err
	}
	var completed bool
	if err := c.DecodeResponse(
		operation,
		response,
		&completed,
		sensitiveValues...,
	); err != nil {
		return segmentReadError(
			operation,
			err,
			c.redactor.With(sensitiveValues...),
		)
	}
	if !completed {
		return newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			operation,
			nil,
			c.redactor.With(sensitiveValues...),
		)
	}
	return nil
}

// GetSegmentFlagReferences reads the dedicated exact-ID preflight boundary.
// It is deliberately separate from ordinary Segment refresh.
func (c *Client) GetSegmentFlagReferences(
	ctx context.Context,
	environmentID string,
	segmentID string,
) ([]SegmentFlagReference, error) {
	if !ValidUUID(environmentID) || !ValidUUID(segmentID) {
		return nil, newAPIError(
			ClassificationValidation,
			0,
			"get_segment_flag_references",
			nil,
			c.redactor,
		)
	}

	request, err := c.newSegmentRequest(
		ctx,
		http.MethodGet,
		environmentID,
		[]string{segmentID, segmentFlagReferencesPath},
	)
	if err != nil {
		return nil, newTransportError(err)
	}
	response, err := c.Do(request)
	if err != nil {
		return nil, err
	}

	var references []SegmentFlagReference
	if err := c.DecodeResponse(
		"get_segment_flag_references",
		response,
		&references,
		environmentID,
		segmentID,
	); err != nil {
		return nil, segmentReadError(
			"get_segment_flag_references",
			err,
			c.redactor.With(environmentID, segmentID),
		)
	}
	if references == nil {
		return nil, newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"get_segment_flag_references",
			nil,
			c.redactor.With(environmentID, segmentID),
		)
	}

	seenIDs := make(map[string]struct{}, len(references))
	for _, reference := range references {
		_, environmentValid := CanonicalUUID(reference.EnvironmentID)
		canonicalID, idValid := CanonicalUUID(reference.ID)
		if !environmentValid || !idValid || reference.Key == "" {
			return nil, newAPIError(
				ClassificationAmbiguous,
				response.StatusCode,
				"get_segment_flag_references",
				nil,
				c.redactor.With(environmentID, segmentID, reference.EnvironmentID, reference.ID, reference.Key),
			)
		}
		if _, duplicate := seenIDs[canonicalID]; duplicate {
			return nil, newAPIError(
				ClassificationAmbiguous,
				response.StatusCode,
				"get_segment_flag_references",
				nil,
				c.redactor.With(environmentID, segmentID, reference.EnvironmentID, reference.ID, reference.Key),
			)
		}
		seenIDs[canonicalID] = struct{}{}
	}

	result := make([]SegmentFlagReference, len(references))
	copy(result, references)
	return result, nil
}

func (c *Client) listSegments(
	ctx context.Context,
	environmentID string,
	archived bool,
	additionalSensitiveValues ...string,
) ([]SegmentMatch, error) {
	if !ValidUUID(environmentID) {
		return nil, newAPIError(
			ClassificationValidation,
			0,
			"list_segments",
			nil,
			c.redactor,
		)
	}

	matches := make([]SegmentMatch, 0)
	seenIDs := make(map[string]struct{})
	var expectedTotal int64 = -1

	for pageIndex := int64(0); ; pageIndex++ {
		page, statusCode, err := c.listSegmentPage(
			ctx,
			environmentID,
			archived,
			pageIndex,
			additionalSensitiveValues...,
		)
		if err != nil {
			return nil, err
		}

		redactor := c.redactor.With(environmentID)
		redactor = redactor.With(additionalSensitiveValues...)
		if page.TotalCount == nil || *page.TotalCount < 0 || page.Items == nil ||
			len(page.Items) > segmentPageSize {
			return nil, newAPIError(
				ClassificationAmbiguous,
				statusCode,
				"list_segments",
				nil,
				redactor,
			)
		}

		if expectedTotal < 0 {
			expectedTotal = *page.TotalCount
		} else if *page.TotalCount != expectedTotal {
			return nil, newAPIError(
				ClassificationAmbiguous,
				statusCode,
				"list_segments",
				nil,
				redactor,
			)
		}

		if len(page.Items) == 0 && int64(len(matches)) < expectedTotal {
			return nil, newAPIError(
				ClassificationAmbiguous,
				statusCode,
				"list_segments",
				nil,
				redactor,
			)
		}

		for _, wire := range page.Items {
			match, valid := segmentMatchFromWire(wire, environmentID, archived)
			if !valid {
				return nil, newAPIError(
					ClassificationAmbiguous,
					statusCode,
					"list_segments",
					nil,
					redactor.With(wire.ID),
				)
			}
			canonicalID, _ := CanonicalUUID(match.ID)
			if _, duplicate := seenIDs[canonicalID]; duplicate {
				return nil, newAPIError(
					ClassificationAmbiguous,
					statusCode,
					"list_segments",
					nil,
					redactor.With(match.ID, match.Key),
				)
			}
			seenIDs[canonicalID] = struct{}{}
			matches = append(matches, match)
		}

		collected := int64(len(matches))
		if collected > expectedTotal {
			return nil, newAPIError(
				ClassificationAmbiguous,
				statusCode,
				"list_segments",
				nil,
				redactor,
			)
		}
		if collected == expectedTotal {
			return matches, nil
		}
		if pageIndex == maxSegmentPageIndex {
			return nil, newAPIError(
				ClassificationAmbiguous,
				statusCode,
				"list_segments",
				nil,
				redactor,
			)
		}
	}
}

func (c *Client) listSegmentPage(
	ctx context.Context,
	environmentID string,
	archived bool,
	pageIndex int64,
	additionalSensitiveValues ...string,
) (segmentPageWire, int, error) {
	request, err := c.newSegmentRequest(ctx, http.MethodGet, environmentID, nil)
	if err != nil {
		return segmentPageWire{}, 0, newTransportError(err)
	}
	query := request.URL.Query()
	query.Set("Name", "")
	query.Set("IsArchived", strconv.FormatBool(archived))
	query.Set("PageIndex", strconv.FormatInt(pageIndex, 10))
	query.Set("PageSize", strconv.Itoa(segmentPageSize))
	request.URL.RawQuery = query.Encode()

	response, err := c.Do(request)
	if err != nil {
		return segmentPageWire{}, 0, err
	}
	var page segmentPageWire
	sensitiveValues := make([]string, 0, 1+len(additionalSensitiveValues))
	sensitiveValues = append(sensitiveValues, environmentID)
	sensitiveValues = append(sensitiveValues, additionalSensitiveValues...)
	if err := c.DecodeResponse(
		"list_segments",
		response,
		&page,
		sensitiveValues...,
	); err != nil {
		return segmentPageWire{}, response.StatusCode, segmentReadError(
			"list_segments",
			err,
			c.redactor.With(sensitiveValues...),
		)
	}
	return page, response.StatusCode, nil
}

func (c *Client) newSegmentRequest(
	ctx context.Context,
	method string,
	environmentID string,
	segments []string,
) (*http.Request, error) {
	return c.newRequest(ctx, method, segmentPath(environmentID, segments), nil)
}

func (c *Client) newSegmentJSONRequest(
	ctx context.Context,
	method string,
	environmentID string,
	segments []string,
	payload any,
) (*http.Request, error) {
	return c.newJSONRequest(ctx, method, segmentPath(environmentID, segments), payload)
}

func segmentPath(environmentID string, segments []string) []string {
	path := []string{environmentsPath, environmentID, segmentsPath}
	return append(path, segments...)
}

func segmentFromWire(wire segmentWire, environmentID string) (Segment, bool) {
	if !ValidUUID(wire.ID) || !ValidUUID(wire.EnvironmentID) ||
		wire.Name == nil || *wire.Name == "" ||
		wire.Key == nil || *wire.Key == "" ||
		wire.Type == nil || wire.Description == nil ||
		wire.Included == nil || wire.Excluded == nil || wire.Rules == nil || wire.Tags == nil ||
		wire.IsArchived == nil || wire.IsEnvironmentSpecific == nil ||
		!validSegmentTaxonomy(*wire.Type, wire.Scopes, wire.IsEnvironmentSpecific) {
		return Segment{}, false
	}
	if *wire.Type == SegmentTypeEnvironmentSpecific &&
		!EqualUUID(wire.EnvironmentID, environmentID) {
		return Segment{}, false
	}

	rules, valid := cloneSegmentRules(wire.Rules)
	if !valid {
		return Segment{}, false
	}
	return Segment{
		ID:            wire.ID,
		EnvironmentID: environmentID,
		Name:          *wire.Name,
		Key:           *wire.Key,
		Type:          *wire.Type,
		Scopes:        append([]string(nil), wire.Scopes...),
		Description:   *wire.Description,
		Included:      append([]string(nil), wire.Included...),
		Excluded:      append([]string(nil), wire.Excluded...),
		Rules:         rules,
		Tags:          append([]string(nil), wire.Tags...),
		IsArchived:    *wire.IsArchived,
	}, true
}

func segmentMatchFromWire(
	wire segmentMatchWire,
	environmentID string,
	archived bool,
) (SegmentMatch, bool) {
	if !ValidUUID(wire.ID) || wire.Key == nil || *wire.Key == "" || wire.Type == nil ||
		!validSegmentTaxonomy(*wire.Type, wire.Scopes, wire.IsEnvironmentSpecific) {
		return SegmentMatch{}, false
	}
	if wire.EnvironmentID != "" {
		if !ValidUUID(wire.EnvironmentID) ||
			(*wire.Type == SegmentTypeEnvironmentSpecific &&
				!EqualUUID(wire.EnvironmentID, environmentID)) {
			return SegmentMatch{}, false
		}
	}
	if wire.IsArchived != nil && *wire.IsArchived != archived {
		return SegmentMatch{}, false
	}

	return SegmentMatch{
		ID:            wire.ID,
		EnvironmentID: environmentID,
		Key:           *wire.Key,
		Type:          *wire.Type,
		Scopes:        append([]string(nil), wire.Scopes...),
		IsArchived:    archived,
	}, true
}

func cloneSegmentRules(rules []SegmentRule) ([]SegmentRule, bool) {
	cloned := make([]SegmentRule, len(rules))
	for index, rule := range rules {
		if rule.Conditions == nil {
			return nil, false
		}
		cloned[index] = SegmentRule{
			ID:         rule.ID,
			Name:       rule.Name,
			Conditions: append([]SegmentCondition(nil), rule.Conditions...),
		}
	}
	return cloned, true
}

func validSegmentTaxonomy(
	segmentType SegmentType,
	scopes []string,
	isEnvironmentSpecific *bool,
) bool {
	if scopes == nil || len(scopes) == 0 {
		return false
	}
	kinds := make([]SegmentScopeKind, len(scopes))
	for index, scope := range scopes {
		kind, valid := ClassifySegmentScope(scope)
		if !valid {
			return false
		}
		kinds[index] = kind
	}

	switch segmentType {
	case SegmentTypeEnvironmentSpecific:
		return len(kinds) == 1 && kinds[0] == SegmentScopeEnvironment &&
			(isEnvironmentSpecific == nil || *isEnvironmentSpecific)
	case SegmentTypeShared:
		return isEnvironmentSpecific == nil || !*isEnvironmentSpecific
	default:
		return false
	}
}

func validSegmentIdentity(identity SegmentIdentity) bool {
	if identity.ID == "" && identity.Key == "" {
		return false
	}
	return identity.ID == "" || ValidUUID(identity.ID)
}

func validCreateSegmentRequest(input CreateSegmentRequest) bool {
	if input.Type != SegmentTypeEnvironmentSpecific ||
		strings.TrimSpace(input.Name) == "" || input.Key == "" ||
		len(input.Scopes) != 1 {
		return false
	}
	isEnvironmentSpecific := true
	return validSegmentTaxonomy(input.Type, input.Scopes, &isEnvironmentSpecific)
}

func validSegmentTargetingRequest(input UpdateSegmentTargetingRequest) bool {
	if input.Included == nil || input.Excluded == nil || input.Rules == nil {
		return false
	}
	seenRuleIDs := make(map[string]struct{}, len(input.Rules))
	seenConditionIDs := make(map[string]struct{})
	for _, rule := range input.Rules {
		ruleID, valid := CanonicalUUID(rule.ID)
		if !valid || rule.Conditions == nil || len(rule.Conditions) == 0 {
			return false
		}
		if _, duplicate := seenRuleIDs[ruleID]; duplicate {
			return false
		}
		seenRuleIDs[ruleID] = struct{}{}
		for _, condition := range rule.Conditions {
			conditionID, valid := CanonicalUUID(condition.ID)
			if !valid || condition.Property == "" || condition.Operator == "" {
				return false
			}
			if _, duplicate := seenConditionIDs[conditionID]; duplicate {
				return false
			}
			seenConditionIDs[conditionID] = struct{}{}
		}
	}
	return true
}

func sameSegmentStringSet(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	values := make(map[string]struct{}, len(left))
	for _, value := range left {
		if _, duplicate := values[value]; duplicate {
			return false
		}
		values[value] = struct{}{}
	}
	seenRight := make(map[string]struct{}, len(right))
	for _, value := range right {
		if _, exists := values[value]; !exists {
			return false
		}
		if _, duplicate := seenRight[value]; duplicate {
			return false
		}
		seenRight[value] = struct{}{}
	}
	return true
}

func segmentCreateSensitiveValues(
	environmentID string,
	input CreateSegmentRequest,
) []string {
	values := []string{
		environmentID,
		string(input.Type),
		input.Name,
		input.Key,
		input.Description,
	}
	return append(values, input.Scopes...)
}

func segmentTargetingSensitiveValues(
	environmentID string,
	segmentID string,
	input UpdateSegmentTargetingRequest,
) []string {
	values := []string{environmentID, segmentID}
	values = append(values, input.Included...)
	values = append(values, input.Excluded...)
	for _, rule := range input.Rules {
		values = append(values, rule.ID, rule.Name)
		for _, condition := range rule.Conditions {
			values = append(
				values,
				condition.ID,
				condition.Property,
				condition.Operator,
				condition.Value,
			)
		}
	}
	return values
}

func resolveSegment(
	active []SegmentMatch,
	archived []SegmentMatch,
	identity SegmentIdentity,
	redactor *Redactor,
) (SegmentMatch, SegmentStatus, error) {
	var match SegmentMatch
	status := SegmentStatusUnknown
	matchCount := 0
	partialMatch := false

	consume := func(candidates []SegmentMatch, expectedArchived bool) bool {
		for _, candidate := range candidates {
			if candidate.IsArchived != expectedArchived {
				return false
			}
			idMatches := identity.ID != "" && EqualUUID(candidate.ID, identity.ID)
			keyMatches := identity.Key != "" && candidate.Key == identity.Key
			fullMatch := (identity.ID == "" || idMatches) &&
				(identity.Key == "" || keyMatches)
			if fullMatch {
				match = candidate
				if expectedArchived {
					status = SegmentStatusArchived
				} else {
					status = SegmentStatusActive
				}
				matchCount++
				continue
			}
			if identity.ID != "" && identity.Key != "" && (idMatches || keyMatches) {
				partialMatch = true
			}
		}
		return true
	}

	if !consume(active, false) || !consume(archived, true) ||
		partialMatch || matchCount > 1 {
		return SegmentMatch{}, SegmentStatusUnknown, newAPIError(
			ClassificationAmbiguous,
			0,
			"resolve_segment",
			nil,
			redactor,
		)
	}
	if matchCount == 0 {
		return SegmentMatch{}, SegmentStatusAbsent, nil
	}
	return match, status, nil
}

// segmentReadError retains classification, status, and cancellation sentinels
// while discarding server details that may contain arbitrary Segment, user,
// condition, tag, scope, tenant, or Feature Flag reference values.
func segmentReadError(operation string, err error, redactor *Redactor) error {
	var apiError *APIError
	if !errors.As(err, &apiError) {
		return newTransportError(err)
	}
	sanitized := newAPIError(
		apiError.Classification(),
		apiError.StatusCode(),
		operation,
		nil,
		redactor,
	)
	sanitized.cause = apiError.cause
	return sanitized
}
