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
)

const (
	featureFlagsPath        = "feature-flags"
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
	seenIDs := make(map[string]struct{})
	var expectedTotal int64 = -1

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

		redactor := c.redactor.With(environmentID)
		redactor = redactor.With(additionalSensitiveValues...)
		if page.TotalCount == nil || *page.TotalCount < 0 || page.Items == nil ||
			len(page.Items) > featureFlagPageSize {
			return nil, newAPIError(
				ClassificationAmbiguous,
				statusCode,
				"list_feature_flags",
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
				"list_feature_flags",
				nil,
				redactor,
			)
		}

		if len(page.Items) == 0 && int64(len(flags)) < expectedTotal {
			return nil, newAPIError(
				ClassificationAmbiguous,
				statusCode,
				"list_feature_flags",
				nil,
				redactor,
			)
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
			if _, duplicate := seenIDs[normalizedID]; duplicate {
				return nil, newAPIError(
					ClassificationAmbiguous,
					statusCode,
					"list_feature_flags",
					nil,
					redactor.With(flag.ID),
				)
			}
			seenIDs[normalizedID] = struct{}{}
			flags = append(flags, flag)
		}

		collected := int64(len(flags))
		if collected > expectedTotal {
			return nil, newAPIError(
				ClassificationAmbiguous,
				statusCode,
				"list_feature_flags",
				nil,
				redactor,
			)
		}
		if collected == expectedTotal {
			return flags, nil
		}
		if pageIndex == maxFeatureFlagPageIndex {
			return nil, newAPIError(
				ClassificationAmbiguous,
				statusCode,
				"list_feature_flags",
				nil,
				redactor,
			)
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
		return FeatureFlag{}, FeatureFlagStatusUnknown, featureFlagReadError(
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
		return featureFlagPageWire{}, response.StatusCode, featureFlagReadError(
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

func featureFlagPath(environmentID string, segments []string) []string {
	path := []string{environmentsPath, environmentID, featureFlagsPath}
	return append(path, segments...)
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

// featureFlagReadError retains the shared classification, status, and safe
// cancellation sentinel while discarding server detail strings. A Feature
// Flag read can mention arbitrary variation values or UI-owned targeting data
// that the caller cannot enumerate for exact redaction.
func featureFlagReadError(operation string, err error, redactor *Redactor) error {
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
