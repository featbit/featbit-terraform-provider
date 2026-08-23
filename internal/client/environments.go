// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const environmentsPath = "envs"

// Environment contains only the safe public Environment fields consumed by
// Terraform. Secret values and UI-owned settings are deliberately absent from
// this read model so they cannot enter ordinary state, diagnostics, or logs.
type Environment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Key         string `json:"key"`
	Description string `json:"description"`
	settings    json.RawMessage
}

// Format prevents the private settings snapshot from being exposed if an
// Environment value is accidentally formatted by diagnostics or logging.
func (Environment) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.Environment{redacted}")
}

// UnmarshalJSON captures the UI-owned settings for a subsequent safe Update
// while continuing to ignore generated Environment secrets. The settings are
// private and therefore cannot be serialized into Terraform-facing models.
func (e *Environment) UnmarshalJSON(data []byte) error {
	type environmentWire struct {
		ID          string          `json:"id"`
		Name        string          `json:"name"`
		Key         string          `json:"key"`
		Description string          `json:"description"`
		Settings    json.RawMessage `json:"settings"`
	}
	var wire environmentWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	e.ID = wire.ID
	e.Name = wire.Name
	e.Key = wire.Key
	e.Description = wire.Description
	e.settings = append(e.settings[:0], wire.Settings...)
	return nil
}

// CreateEnvironmentRequest is the complete Terraform-owned Environment
// create payload. Server-generated secrets and UI-owned settings are omitted.
type CreateEnvironmentRequest struct {
	Name        string `json:"name"`
	Key         string `json:"key"`
	Description string `json:"description"`
}

// UpdateEnvironmentRequest contains only the mutable fields owned by
// Terraform. UpdateEnvironment adds the current private settings snapshot.
type UpdateEnvironmentRequest struct {
	Name        string
	Description string
}

type updateEnvironmentPayload struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Settings    json.RawMessage `json:"settings"`
}

type environmentUpdateResponse struct {
	ID string `json:"id"`
}

// GetEnvironment first uses the documented parent-scoped single-object
// endpoint. A failed direct read is resolved through the exact parent Project
// and exact Environment UUID. Current EnvironmentVm responses can omit key;
// in that case the same exact parent membership supplies it without fuzzy
// lookup. The boolean is true only when one exact object is confirmed.
func (c *Client) GetEnvironment(
	ctx context.Context,
	projectID string,
	environmentID string,
) (Environment, bool, error) {
	if !ValidUUID(projectID) || !ValidUUID(environmentID) {
		return Environment{}, false, newAPIError(
			ClassificationValidation,
			0,
			"get_environment",
			nil,
			c.redactor,
		)
	}

	direct, directErr := c.getEnvironmentDirect(ctx, projectID, environmentID)
	directConfirmed := directErr == nil && validEnvironment(direct) &&
		EqualUUID(direct.ID, environmentID)
	if directConfirmed && direct.Key != "" {
		return direct, true, nil
	}

	project, projectFound, err := c.GetProject(ctx, projectID)
	if err != nil {
		return Environment{}, false, err
	}
	if !projectFound {
		if directConfirmed {
			return Environment{}, false, newAPIError(
				ClassificationAmbiguous,
				0,
				"resolve_environment",
				nil,
				c.redactor.With(projectID, environmentID),
			)
		}
		return Environment{}, false, nil
	}

	parentEnvironment, found, err := resolveEnvironmentByID(
		project.Environments,
		environmentID,
		c.redactor.With(projectID),
	)
	if err != nil {
		return Environment{}, false, err
	}
	if !found {
		if directConfirmed {
			return Environment{}, false, newAPIError(
				ClassificationAmbiguous,
				0,
				"resolve_environment",
				nil,
				c.redactor.With(projectID, environmentID),
			)
		}
		return Environment{}, false, nil
	}

	if directConfirmed {
		direct.Key = parentEnvironment.Key
		return direct, true, nil
	}

	return parentEnvironment, true, nil
}

// GetEnvironmentByKey reads the exact parent Project and resolves one
// case-sensitive Environment key from its complete nested Environment list.
// The boolean is true only when the parent exists and exactly one match is
// confirmed; duplicate exact keys are ambiguous.
func (c *Client) GetEnvironmentByKey(
	ctx context.Context,
	projectID string,
	key string,
) (Environment, bool, error) {
	if !ValidUUID(projectID) {
		return Environment{}, false, newAPIError(
			ClassificationValidation,
			0,
			"get_environment_by_key",
			nil,
			c.redactor,
		)
	}

	project, found, err := c.GetProject(ctx, projectID)
	if err != nil || !found {
		return Environment{}, false, err
	}
	return c.ResolveEnvironmentByKey(
		project.Environments,
		key,
	)
}

// CreateEnvironment executes the documented mutation exactly once. Exact-key
// preflight and ambiguous-outcome reconciliation belong to the Terraform
// lifecycle caller.
func (c *Client) CreateEnvironment(
	ctx context.Context,
	projectID string,
	input CreateEnvironmentRequest,
) (Environment, error) {
	if !ValidUUID(projectID) {
		return Environment{}, newAPIError(
			ClassificationValidation,
			0,
			"create_environment",
			nil,
			c.redactor,
		)
	}
	request, err := c.newEnvironmentJSONRequest(
		ctx,
		http.MethodPost,
		projectID,
		nil,
		input,
	)
	if err != nil {
		return Environment{}, newAPIError(
			ClassificationAmbiguous,
			0,
			"create_environment",
			nil,
			c.redactor.With(projectID, input.Key),
		)
	}

	response, err := c.Do(request)
	if err != nil {
		return Environment{}, err
	}
	var environment Environment
	if err := c.DecodeResponse(
		"create_environment",
		response,
		&environment,
		projectID,
		input.Key,
	); err != nil {
		return Environment{}, err
	}
	if !validEnvironment(environment) {
		return Environment{}, newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"create_environment",
			nil,
			c.redactor.With(projectID, input.Key),
		)
	}
	if environment.Key != "" && environment.Key != input.Key {
		return Environment{}, newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"create_environment",
			nil,
			c.redactor.With(projectID, input.Key, environment.Key),
		)
	}
	// Current EnvironmentVm responses omit key. The create request is the
	// server-confirmed immutable key until the canonical parent read follows.
	environment.Key = input.Key
	return environment, nil
}

// UpdateEnvironment changes only name and description while passing through
// the settings snapshot obtained by the caller's immediately preceding exact
// GetEnvironment. It executes the mutation exactly once.
func (c *Client) UpdateEnvironment(
	ctx context.Context,
	projectID string,
	environmentID string,
	current Environment,
	input UpdateEnvironmentRequest,
) error {
	if !ValidUUID(projectID) || !ValidUUID(environmentID) ||
		!ValidUUID(current.ID) || !EqualUUID(current.ID, environmentID) {
		return newAPIError(
			ClassificationValidation,
			0,
			"update_environment",
			nil,
			c.redactor,
		)
	}
	if !validEnvironmentSettings(current.settings) {
		return newAPIError(
			ClassificationAmbiguous,
			0,
			"update_environment",
			nil,
			c.redactor.With(projectID, environmentID),
		)
	}

	payload := updateEnvironmentPayload{
		Name:        input.Name,
		Description: input.Description,
		Settings:    append(json.RawMessage(nil), current.settings...),
	}
	request, err := c.newEnvironmentJSONRequest(
		ctx,
		http.MethodPut,
		projectID,
		[]string{environmentID},
		payload,
	)
	if err != nil {
		return newAPIError(
			ClassificationAmbiguous,
			0,
			"update_environment",
			nil,
			c.redactor.With(projectID, environmentID),
		)
	}

	response, err := c.Do(request)
	if err != nil {
		return err
	}
	var updated environmentUpdateResponse
	if err := c.DecodeResponse(
		"update_environment",
		response,
		&updated,
		projectID,
		environmentID,
		input.Name,
	); err != nil {
		return err
	}
	if !ValidUUID(updated.ID) || !EqualUUID(updated.ID, environmentID) {
		return newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"update_environment",
			nil,
			c.redactor.With(projectID, environmentID, input.Name),
		)
	}
	return nil
}

// DeleteEnvironment executes the documented mutation exactly once and
// requires a true BooleanApiResponse. Exact parent-scoped absence is still
// proven by the lifecycle caller.
func (c *Client) DeleteEnvironment(
	ctx context.Context,
	projectID string,
	environmentID string,
) error {
	if !ValidUUID(projectID) || !ValidUUID(environmentID) {
		return newAPIError(
			ClassificationValidation,
			0,
			"delete_environment",
			nil,
			c.redactor,
		)
	}
	request, err := c.newEnvironmentRequest(
		ctx,
		http.MethodDelete,
		projectID,
		[]string{environmentID},
	)
	if err != nil {
		return newTransportError(err)
	}
	response, err := c.Do(request)
	if err != nil {
		return err
	}
	var deleted bool
	if err := c.DecodeResponse(
		"delete_environment",
		response,
		&deleted,
		projectID,
		environmentID,
	); err != nil {
		return err
	}
	if !deleted {
		return newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"delete_environment",
			nil,
			c.redactor.With(projectID, environmentID),
		)
	}
	return nil
}

func (c *Client) getEnvironmentDirect(
	ctx context.Context,
	projectID string,
	environmentID string,
) (Environment, error) {
	request, err := c.newEnvironmentRequest(
		ctx,
		http.MethodGet,
		projectID,
		[]string{environmentID},
	)
	if err != nil {
		return Environment{}, newTransportError(err)
	}

	response, err := c.Do(request)
	if err != nil {
		return Environment{}, err
	}
	var environment Environment
	if err := c.DecodeResponse(
		"get_environment",
		response,
		&environment,
		projectID,
		environmentID,
	); err != nil {
		return Environment{}, err
	}
	return environment, nil
}

func (c *Client) newEnvironmentRequest(
	ctx context.Context,
	method string,
	projectID string,
	segments []string,
) (*http.Request, error) {
	return c.newRequest(ctx, method, environmentPath(projectID, segments), nil)
}

func (c *Client) newEnvironmentJSONRequest(
	ctx context.Context,
	method string,
	projectID string,
	segments []string,
	payload any,
) (*http.Request, error) {
	return c.newJSONRequest(ctx, method, environmentPath(projectID, segments), payload)
}

func environmentPath(projectID string, segments []string) []string {
	path := []string{projectsPath, projectID, environmentsPath}
	return append(path, segments...)
}

func resolveEnvironmentByID(
	environments []ProjectEnvironment,
	environmentID string,
	redactor *Redactor,
) (Environment, bool, error) {
	match, found, err := resolveExactOne(
		environments,
		func(environment ProjectEnvironment) bool {
			return EqualUUID(environment.ID, environmentID)
		},
		"resolve_environment",
		redactor.With(environmentID),
	)
	if err != nil || !found {
		return Environment{}, found, err
	}
	return environmentFromProject(match), true, nil
}

// ResolveEnvironmentByKey applies the shared case-sensitive exact zero/one/
// duplicate contract to a complete, already-validated nested Environment
// collection. It is reused by lookup and create preflight/reconciliation
// callers.
func (c *Client) ResolveEnvironmentByKey(
	environments []ProjectEnvironment,
	key string,
) (Environment, bool, error) {
	match, found, err := resolveExactOne(
		environments,
		func(environment ProjectEnvironment) bool { return environment.Key == key },
		"resolve_environment_by_key",
		c.redactor.With(key),
	)
	if err != nil || !found {
		return Environment{}, found, err
	}
	return environmentFromProject(match), true, nil
}

func environmentFromProject(environment ProjectEnvironment) Environment {
	return Environment{
		ID:          environment.ID,
		Name:        environment.Name,
		Key:         environment.Key,
		Description: environment.Description,
	}
}

func validEnvironment(environment Environment) bool {
	return ValidUUID(environment.ID)
}

func validEnvironmentSettings(settings json.RawMessage) bool {
	settings = bytes.TrimSpace(settings)
	return len(settings) >= 2 && settings[0] == '{' && settings[len(settings)-1] == '}' &&
		json.Valid(settings)
}
