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
	featureFlagsPath        = "feature-flags"
	featureFlagNamePath     = "name"
	featureFlagArchivePath  = "archive"
	featureFlagPageSize     = 100
	maxFeatureFlagPageIndex = int64(1<<31 - 1)
)

// FeatureFlag contains only the public definition fields consumed by the
// Terraform Feature Flag lifecycle. Enabled state, selections, targeting,
// rules, rollouts, tags, audit data, and other UI-owned fields are
// deliberately absent.
type FeatureFlag struct {
	ID            string                 `json:"id"`
	EnvironmentID string                 `json:"envId"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	Key           string                 `json:"key"`
	VariationType string                 `json:"variationType"`
	Variations    []FeatureFlagVariation `json:"variations"`
	IsArchived    bool                   `json:"isArchived"`
}

// Format prevents runtime identities and variation values from being exposed
// if a Feature Flag response is accidentally included in diagnostics, logs,
// or test assertion output.
func (FeatureFlag) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.FeatureFlag{redacted}")
}

// FeatureFlagVariation is the safe definition shape shared by exact and
// collection Feature Flag reads.
type FeatureFlagVariation struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Format applies the Feature Flag redaction boundary when a variation is
// formatted independently of its parent.
func (FeatureFlagVariation) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.FeatureFlagVariation{redacted}")
}

// CreateFeatureFlagRequest is the complete documented Feature Flag create
// payload. The operational fields are required only to establish one
// deterministic disabled-safe initial state; they are deliberately absent
// from the read model so Terraform does not retain ownership of them.
type CreateFeatureFlagRequest struct {
	Name                string                 `json:"name"`
	Key                 string                 `json:"key"`
	IsEnabled           bool                   `json:"isEnabled"`
	Description         string                 `json:"description"`
	VariationType       string                 `json:"variationType"`
	Variations          []FeatureFlagVariation `json:"variations"`
	EnabledVariationID  string                 `json:"enabledVariationId"`
	DisabledVariationID string                 `json:"disabledVariationId"`
	Tags                []string               `json:"tags"`
}

// Format prevents a create payload from exposing runtime definition values.
func (CreateFeatureFlagRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.CreateFeatureFlagRequest{redacted}")
}

// UpdateFeatureFlagNameRequest contains the only Feature Flag field Terraform
// updates in place. Path identity and optional audit comments are omitted.
type UpdateFeatureFlagNameRequest struct {
	Name string `json:"name"`
}

// Format prevents a Feature Flag name from entering formatted diagnostics.
func (UpdateFeatureFlagNameRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.UpdateFeatureFlagNameRequest{redacted}")
}

// FeatureFlagStatus is the authoritative exact-key state composed from a
// direct exact read or complete active and archived collection views.
type FeatureFlagStatus string

const (
	FeatureFlagStatusUnknown  FeatureFlagStatus = ""
	FeatureFlagStatusAbsent   FeatureFlagStatus = "absent"
	FeatureFlagStatusActive   FeatureFlagStatus = "active"
	FeatureFlagStatusArchived FeatureFlagStatus = "archived"
)

// featureFlagWire represents the safe union of the documented FeatureFlag
// exact response and FeatureFlagVm collection item. Collection items omit
// envId and isArchived, so their values are supplied and checked from the
// request context after decoding.
type featureFlagWire struct {
	ID            string                 `json:"id"`
	EnvironmentID string                 `json:"envId"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	Key           string                 `json:"key"`
	VariationType string                 `json:"variationType"`
	Variations    []FeatureFlagVariation `json:"variations"`
	IsArchived    *bool                  `json:"isArchived"`
}

type featureFlagPageWire struct {
	TotalCount *int64            `json:"totalCount"`
	Items      []featureFlagWire `json:"items"`
}

// GetFeatureFlag first uses the documented exact environment/key endpoint.
// A structurally valid exact response is canonical. Any failed or incomplete
// direct read is resolved through complete active and archived collection
// views; only those complete views can prove exact absence.
func (c *Client) GetFeatureFlag(
	ctx context.Context,
	environmentID string,
	key string,
) (FeatureFlag, FeatureFlagStatus, error) {
	if !ValidUUID(environmentID) || key == "" {
		return FeatureFlag{}, FeatureFlagStatusUnknown, newAPIError(
			ClassificationValidation,
			0,
			"get_feature_flag",
			nil,
			c.redactor,
		)
	}

	flag, status, directErr := c.getFeatureFlagDirect(ctx, environmentID, key)
	if directErr == nil {
		return flag, status, nil
	}

	return c.ResolveFeatureFlag(ctx, environmentID, key)
}

// ResolveFeatureFlag always consumes the complete active and archived
// collection views before resolving one case-sensitive exact key. Lifecycle
// preflights and mutation reconciliation use this method when a direct exact
// response alone cannot prove zero or detect cross-view inconsistency.
func (c *Client) ResolveFeatureFlag(
	ctx context.Context,
	environmentID string,
	key string,
) (FeatureFlag, FeatureFlagStatus, error) {
	if !ValidUUID(environmentID) || key == "" {
		return FeatureFlag{}, FeatureFlagStatusUnknown, newAPIError(
			ClassificationValidation,
			0,
			"resolve_feature_flag",
			nil,
			c.redactor,
		)
	}

	active, err := c.listFeatureFlags(ctx, environmentID, false, key)
	if err != nil {
		return FeatureFlag{}, FeatureFlagStatusUnknown, err
	}
	archived, err := c.listFeatureFlags(ctx, environmentID, true, key)
	if err != nil {
		return FeatureFlag{}, FeatureFlagStatusUnknown, err
	}

	return resolveFeatureFlagByKey(
		active,
		archived,
		key,
		c.redactor.With(environmentID, key),
	)
}

// CreateFeatureFlag executes the documented mutation exactly once. Exact-zero
// preflight, canonical read-after-write, and ambiguous-outcome reconciliation
// belong to the Terraform lifecycle caller.
func (c *Client) CreateFeatureFlag(
	ctx context.Context,
	environmentID string,
	input CreateFeatureFlagRequest,
) (FeatureFlag, error) {
	if !ValidUUID(environmentID) || input.Key == "" {
		return FeatureFlag{}, newAPIError(
			ClassificationValidation,
			0,
			"create_feature_flag",
			nil,
			c.redactor,
		)
	}

	sensitiveValues := featureFlagCreateSensitiveValues(environmentID, input)
	request, err := c.newFeatureFlagJSONRequest(
		ctx,
		http.MethodPost,
		environmentID,
		nil,
		input,
	)
	if err != nil {
		return FeatureFlag{}, newAPIError(
			ClassificationAmbiguous,
			0,
			"create_feature_flag",
			nil,
			c.redactor.With(sensitiveValues...),
		)
	}

	response, err := c.Do(request)
	if err != nil {
		return FeatureFlag{}, err
	}
	var wire featureFlagWire
	if err := c.DecodeResponse(
		"create_feature_flag",
		response,
		&wire,
		sensitiveValues...,
	); err != nil {
		return FeatureFlag{}, readErrorWithoutDetails(
			"create_feature_flag",
			err,
			c.redactor.With(sensitiveValues...),
		)
	}

	flag, valid := featureFlagFromWire(wire, environmentID, false, true)
	if !valid || flag.Key != input.Key || flag.IsArchived {
		return FeatureFlag{}, newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"create_feature_flag",
			nil,
			c.redactor.With(sensitiveValues...),
		)
	}
	return flag, nil
}

// UpdateFeatureFlagName executes the documented specialized name mutation
// exactly once and confirms that its GuidApiResponse identifies the object
// already tracked by Terraform.
func (c *Client) UpdateFeatureFlagName(
	ctx context.Context,
	environmentID string,
	key string,
	featureFlagID string,
	input UpdateFeatureFlagNameRequest,
) error {
	if !ValidUUID(environmentID) || key == "" || !ValidUUID(featureFlagID) || input.Name == "" {
		return newAPIError(
			ClassificationValidation,
			0,
			"update_feature_flag_name",
			nil,
			c.redactor,
		)
	}
	sensitiveValues := []string{environmentID, key, featureFlagID, input.Name}
	request, err := c.newFeatureFlagJSONRequest(
		ctx,
		http.MethodPut,
		environmentID,
		[]string{key, featureFlagNamePath},
		input,
	)
	if err != nil {
		return newAPIError(
			ClassificationAmbiguous,
			0,
			"update_feature_flag_name",
			nil,
			c.redactor.With(sensitiveValues...),
		)
	}

	response, err := c.Do(request)
	if err != nil {
		return err
	}
	var updatedID string
	if err := c.DecodeResponse(
		"update_feature_flag_name",
		response,
		&updatedID,
		sensitiveValues...,
	); err != nil {
		return readErrorWithoutDetails(
			"update_feature_flag_name",
			err,
			c.redactor.With(sensitiveValues...),
		)
	}
	if !EqualUUID(updatedID, featureFlagID) {
		return newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"update_feature_flag_name",
			nil,
			c.redactor.With(sensitiveValues...).With(updatedID),
		)
	}
	return nil
}

// ArchiveFeatureFlag executes the documented archive prerequisite exactly
// once. The optional ResourceChangeRequest comment body is deliberately
// omitted. The lifecycle caller reconciles status before continuing.
func (c *Client) ArchiveFeatureFlag(
	ctx context.Context,
	environmentID string,
	key string,
) error {
	return c.mutateFeatureFlagBoolean(
		ctx,
		http.MethodPut,
		environmentID,
		key,
		[]string{key, featureFlagArchivePath},
		"archive_feature_flag",
	)
}

// DeleteFeatureFlag executes the documented permanent deletion exactly once.
// Exact absence is still proven through both complete collection views by the
// lifecycle caller.
func (c *Client) DeleteFeatureFlag(
	ctx context.Context,
	environmentID string,
	key string,
) error {
	return c.mutateFeatureFlagBoolean(
		ctx,
		http.MethodDelete,
		environmentID,
		key,
		[]string{key},
		"delete_feature_flag",
	)
}

func (c *Client) mutateFeatureFlagBoolean(
	ctx context.Context,
	method string,
	environmentID string,
	key string,
	segments []string,
	operation string,
) error {
	if !ValidUUID(environmentID) || key == "" {
		return newAPIError(ClassificationValidation, 0, operation, nil, c.redactor)
	}
	request, err := c.newFeatureFlagRequest(ctx, method, environmentID, segments)
	if err != nil {
		return newAPIError(
			ClassificationAmbiguous,
			0,
			operation,
			nil,
			c.redactor.With(environmentID, key),
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
		environmentID,
		key,
	); err != nil {
		return readErrorWithoutDetails(
			operation,
			err,
			c.redactor.With(environmentID, key),
		)
	}
	if !completed {
		return newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			operation,
			nil,
			c.redactor.With(environmentID, key),
		)
	}
	return nil
}

// ListFeatureFlags returns one complete, structurally reconciled active or
// archived collection. It requests explicit zero-based pages and does not
// return partial results when totalCount or page contents are inconsistent.
func (c *Client) ListFeatureFlags(
	ctx context.Context,
	environmentID string,
	archived bool,
) ([]FeatureFlag, error) {
	return c.listFeatureFlags(ctx, environmentID, archived)
}

func (c *Client) listFeatureFlags(
	ctx context.Context,
	environmentID string,
	archived bool,
	additionalSensitiveValues ...string,
) ([]FeatureFlag, error) {
	if !ValidUUID(environmentID) {
		return nil, newAPIError(
			ClassificationValidation,
			0,
			"list_feature_flags",
			nil,
			c.redactor,
		)
	}

	flags := make([]FeatureFlag, 0)
	redactor := c.redactor.With(environmentID)
	redactor = redactor.With(additionalSensitiveValues...)
	pages := newCompletePageTracker(
		"list_feature_flags",
		featureFlagPageSize,
		maxFeatureFlagPageIndex,
		redactor,
	)

	for pageIndex := int64(0); ; pageIndex++ {
		page, statusCode, err := c.listFeatureFlagPage(
			ctx,
			environmentID,
			archived,
			pageIndex,
			additionalSensitiveValues...,
		)
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

		for _, wire := range page.Items {
			flag, valid := featureFlagFromWire(wire, environmentID, archived, false)
			if !valid {
				return nil, newAPIError(
					ClassificationAmbiguous,
					statusCode,
					"list_feature_flags",
					nil,
					redactor,
				)
			}
			normalizedID, _ := CanonicalUUID(flag.ID)
			if err := pages.recordExactID(
				normalizedID,
				statusCode,
				redactor.With(flag.ID),
			); err != nil {
				return nil, err
			}
			flags = append(flags, flag)
		}

		complete, err := pages.pageComplete(pageIndex, statusCode)
		if err != nil {
			return nil, err
		}
		if complete {
			return flags, nil
		}
	}
}

func (c *Client) getFeatureFlagDirect(
	ctx context.Context,
	environmentID string,
	key string,
) (FeatureFlag, FeatureFlagStatus, error) {
	request, err := c.newFeatureFlagRequest(
		ctx,
		http.MethodGet,
		environmentID,
		[]string{key},
	)
	if err != nil {
		return FeatureFlag{}, FeatureFlagStatusUnknown, newTransportError(err)
	}

	response, err := c.Do(request)
	if err != nil {
		return FeatureFlag{}, FeatureFlagStatusUnknown, err
	}
	var wire featureFlagWire
	if err := c.DecodeResponse(
		"get_feature_flag",
		response,
		&wire,
		environmentID,
		key,
	); err != nil {
		return FeatureFlag{}, FeatureFlagStatusUnknown, readErrorWithoutDetails(
			"get_feature_flag",
			err,
			c.redactor.With(environmentID, key),
		)
	}

	flag, valid := featureFlagFromWire(wire, environmentID, false, true)
	if !valid || flag.Key != key {
		return FeatureFlag{}, FeatureFlagStatusUnknown, newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"get_feature_flag",
			nil,
			c.redactor.With(environmentID, key, wire.ID, wire.Key),
		)
	}
	if flag.IsArchived {
		return flag, FeatureFlagStatusArchived, nil
	}
	return flag, FeatureFlagStatusActive, nil
}

func (c *Client) listFeatureFlagPage(
	ctx context.Context,
	environmentID string,
	archived bool,
	pageIndex int64,
	additionalSensitiveValues ...string,
) (featureFlagPageWire, int, error) {
	request, err := c.newFeatureFlagRequest(
		ctx,
		http.MethodGet,
		environmentID,
		nil,
	)
	if err != nil {
		return featureFlagPageWire{}, 0, newTransportError(err)
	}
	query := request.URL.Query()
	query.Set("IsArchived", strconv.FormatBool(archived))
	query.Set("PageIndex", strconv.FormatInt(pageIndex, 10))
	query.Set("PageSize", strconv.Itoa(featureFlagPageSize))
	request.URL.RawQuery = query.Encode()

	response, err := c.Do(request)
	if err != nil {
		return featureFlagPageWire{}, 0, err
	}
	var page featureFlagPageWire
	sensitiveValues := make([]string, 0, 1+len(additionalSensitiveValues))
	sensitiveValues = append(sensitiveValues, environmentID)
	sensitiveValues = append(sensitiveValues, additionalSensitiveValues...)
	if err := c.DecodeResponse(
		"list_feature_flags",
		response,
		&page,
		sensitiveValues...,
	); err != nil {
		return featureFlagPageWire{}, response.StatusCode, readErrorWithoutDetails(
			"list_feature_flags",
			err,
			c.redactor.With(sensitiveValues...),
		)
	}
	return page, response.StatusCode, nil
}

func (c *Client) newFeatureFlagRequest(
	ctx context.Context,
	method string,
	environmentID string,
	segments []string,
) (*http.Request, error) {
	return c.newRequest(ctx, method, featureFlagPath(environmentID, segments), nil)
}

func (c *Client) newFeatureFlagJSONRequest(
	ctx context.Context,
	method string,
	environmentID string,
	segments []string,
	payload any,
) (*http.Request, error) {
	return c.newJSONRequest(ctx, method, featureFlagPath(environmentID, segments), payload)
}

func featureFlagPath(environmentID string, segments []string) []string {
	path := []string{environmentsPath, environmentID, featureFlagsPath}
	return append(path, segments...)
}

func featureFlagCreateSensitiveValues(
	environmentID string,
	input CreateFeatureFlagRequest,
) []string {
	values := []string{
		environmentID,
		input.Name,
		input.Key,
		input.Description,
		input.VariationType,
		input.EnabledVariationID,
		input.DisabledVariationID,
	}
	for _, variation := range input.Variations {
		values = append(values, variation.ID, variation.Name, variation.Value)
	}
	values = append(values, input.Tags...)
	return values
}

func featureFlagFromWire(
	wire featureFlagWire,
	environmentID string,
	archived bool,
	requireExactContext bool,
) (FeatureFlag, bool) {
	if !ValidUUID(wire.ID) || wire.Key == "" {
		return FeatureFlag{}, false
	}

	if requireExactContext {
		if !ValidUUID(wire.EnvironmentID) ||
			!EqualUUID(wire.EnvironmentID, environmentID) ||
			wire.IsArchived == nil {
			return FeatureFlag{}, false
		}
		archived = *wire.IsArchived
	} else {
		if wire.EnvironmentID != "" &&
			(!ValidUUID(wire.EnvironmentID) || !EqualUUID(wire.EnvironmentID, environmentID)) {
			return FeatureFlag{}, false
		}
		if wire.IsArchived != nil && *wire.IsArchived != archived {
			return FeatureFlag{}, false
		}
	}

	variations := wire.Variations
	if variations != nil {
		variations = append([]FeatureFlagVariation(nil), variations...)
	}
	return FeatureFlag{
		ID:            wire.ID,
		EnvironmentID: environmentID,
		Name:          wire.Name,
		Description:   wire.Description,
		Key:           wire.Key,
		VariationType: wire.VariationType,
		Variations:    variations,
		IsArchived:    archived,
	}, true
}

func resolveFeatureFlagByKey(
	active []FeatureFlag,
	archived []FeatureFlag,
	key string,
	redactor *Redactor,
) (FeatureFlag, FeatureFlagStatus, error) {
	var match FeatureFlag
	status := FeatureFlagStatusUnknown
	matchCount := 0

	for _, candidate := range active {
		if candidate.Key != key {
			continue
		}
		if candidate.IsArchived {
			return FeatureFlag{}, FeatureFlagStatusUnknown, newAPIError(
				ClassificationAmbiguous,
				0,
				"resolve_feature_flag",
				nil,
				redactor,
			)
		}
		match = candidate
		status = FeatureFlagStatusActive
		matchCount++
	}
	for _, candidate := range archived {
		if candidate.Key != key {
			continue
		}
		if !candidate.IsArchived {
			return FeatureFlag{}, FeatureFlagStatusUnknown, newAPIError(
				ClassificationAmbiguous,
				0,
				"resolve_feature_flag",
				nil,
				redactor,
			)
		}
		match = candidate
		status = FeatureFlagStatusArchived
		matchCount++
	}

	switch matchCount {
	case 0:
		return FeatureFlag{}, FeatureFlagStatusAbsent, nil
	case 1:
		return match, status, nil
	default:
		return FeatureFlag{}, FeatureFlagStatusUnknown, newAPIError(
			ClassificationAmbiguous,
			0,
			"resolve_feature_flag",
			nil,
			redactor,
		)
	}
}
