// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"net/http"
)

const projectsPath = "projects"

// Project contains only the public Project fields consumed by Terraform.
// In particular, nested environment secrets and settings are deliberately
// absent so they cannot enter provider state, diagnostics, or logs.
type Project struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Key          string               `json:"key"`
	Environments []ProjectEnvironment `json:"environments"`
}

// ProjectEnvironment is the safe, non-owning environment observation exposed
// with a Project.
type ProjectEnvironment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Key         string `json:"key"`
	Description string `json:"description"`
}

// CreateProjectRequest is the complete documented Project create payload.
type CreateProjectRequest struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// UpdateProjectRequest contains the only mutable Project field owned by
// Terraform. The project key is immutable and intentionally omitted.
type UpdateProjectRequest struct {
	Name string `json:"name"`
}

type projectUpdateResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetProject first uses the documented single-object endpoint. Because a
// direct failure does not prove absence, it falls back to the complete Project
// collection and resolves the exact UUID. The boolean is true only when one
// exact object is confirmed.
func (c *Client) GetProject(ctx context.Context, projectID string) (Project, bool, error) {
	if !ValidUUID(projectID) {
		return Project{}, false, newAPIError(
			ClassificationValidation,
			0,
			"get_project",
			nil,
			c.redactor,
		)
	}

	project, err := c.getProjectDirect(ctx, projectID)
	if err == nil && validProject(project) && EqualUUID(project.ID, projectID) {
		return project, true, nil
	}

	projects, listErr := c.ListProjects(ctx)
	if listErr != nil {
		return Project{}, false, listErr
	}
	return resolveProjectByID(projects, projectID, c.redactor)
}

// ListProjects returns the documented complete, unpaginated Project
// collection. A null or structurally incomplete collection is ambiguous, not
// authoritative absence.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	request, err := c.newProjectRequest(ctx, http.MethodGet, nil)
	if err != nil {
		return nil, newTransportError(err)
	}

	response, err := c.Do(request)
	if err != nil {
		return nil, err
	}

	var projects []Project
	if err := c.DecodeResponse("list_projects", response, &projects); err != nil {
		return nil, err
	}
	if projects == nil {
		return nil, newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"list_projects",
			nil,
			c.redactor,
		)
	}
	for _, project := range projects {
		if !validProject(project) {
			return nil, newAPIError(
				ClassificationAmbiguous,
				response.StatusCode,
				"list_projects",
				nil,
				c.redactor,
			)
		}
	}
	return projects, nil
}

// CreateProject executes the documented mutation exactly once. Preflight and
// ambiguous-outcome reconciliation belong to the Terraform lifecycle caller.
func (c *Client) CreateProject(ctx context.Context, input CreateProjectRequest) (Project, error) {
	request, err := c.newProjectJSONRequest(ctx, http.MethodPost, nil, input)
	if err != nil {
		return Project{}, newAPIError(ClassificationAmbiguous, 0, "create_project", nil, c.redactor)
	}

	response, err := c.Do(request)
	if err != nil {
		return Project{}, err
	}
	var project Project
	if err := c.DecodeResponse("create_project", response, &project, input.Key); err != nil {
		return Project{}, err
	}
	if !validProject(project) || project.Key != input.Key {
		return Project{}, newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"create_project",
			nil,
			c.redactor.With(input.Key),
		)
	}
	return project, nil
}

// UpdateProject changes only the Terraform-owned Project name and decodes the
// documented ProjectVm response. The lifecycle caller reads the full Project
// immediately afterwards.
func (c *Client) UpdateProject(
	ctx context.Context,
	projectID string,
	input UpdateProjectRequest,
) error {
	if !ValidUUID(projectID) {
		return newAPIError(ClassificationValidation, 0, "update_project", nil, c.redactor)
	}
	request, err := c.newProjectJSONRequest(
		ctx,
		http.MethodPut,
		[]string{projectID},
		input,
	)
	if err != nil {
		return newAPIError(ClassificationAmbiguous, 0, "update_project", nil, c.redactor)
	}

	response, err := c.Do(request)
	if err != nil {
		return err
	}
	var updated projectUpdateResponse
	if err := c.DecodeResponse(
		"update_project",
		response,
		&updated,
		projectID,
		input.Name,
	); err != nil {
		return err
	}
	if !ValidUUID(updated.ID) || !EqualUUID(updated.ID, projectID) {
		return newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"update_project",
			nil,
			c.redactor.With(projectID, input.Name),
		)
	}
	return nil
}

// DeleteProject executes the documented mutation exactly once and requires a
// true BooleanApiResponse. Exact absence is still proven by the lifecycle
// caller after this method returns.
func (c *Client) DeleteProject(ctx context.Context, projectID string) error {
	if !ValidUUID(projectID) {
		return newAPIError(ClassificationValidation, 0, "delete_project", nil, c.redactor)
	}
	request, err := c.newProjectRequest(ctx, http.MethodDelete, []string{projectID})
	if err != nil {
		return newTransportError(err)
	}
	response, err := c.Do(request)
	if err != nil {
		return err
	}
	var deleted bool
	if err := c.DecodeResponse("delete_project", response, &deleted, projectID); err != nil {
		return err
	}
	if !deleted {
		return newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			"delete_project",
			nil,
			c.redactor.With(projectID),
		)
	}
	return nil
}

func (c *Client) getProjectDirect(ctx context.Context, projectID string) (Project, error) {
	request, err := c.newProjectRequest(ctx, http.MethodGet, []string{projectID})
	if err != nil {
		return Project{}, newTransportError(err)
	}

	response, err := c.Do(request)
	if err != nil {
		return Project{}, err
	}
	var project Project
	if err := c.DecodeResponse("get_project", response, &project, projectID); err != nil {
		return Project{}, err
	}
	return project, nil
}

func (c *Client) newProjectRequest(
	ctx context.Context,
	method string,
	segments []string,
) (*http.Request, error) {
	return c.newRequest(ctx, method, projectPath(segments), nil)
}

func (c *Client) newProjectJSONRequest(
	ctx context.Context,
	method string,
	segments []string,
	payload any,
) (*http.Request, error) {
	return c.newJSONRequest(ctx, method, projectPath(segments), payload)
}

func projectPath(segments []string) []string {
	return append([]string{projectsPath}, segments...)
}

func resolveProjectByID(
	projects []Project,
	projectID string,
	redactor *Redactor,
) (Project, bool, error) {
	var match Project
	matchCount := 0
	for _, project := range projects {
		if EqualUUID(project.ID, projectID) {
			match = project
			matchCount++
		}
	}

	switch matchCount {
	case 0:
		return Project{}, false, nil
	case 1:
		return match, true, nil
	default:
		return Project{}, false, newAPIError(
			ClassificationAmbiguous,
			0,
			"resolve_project",
			nil,
			redactor.With(projectID),
		)
	}
}

func validProject(project Project) bool {
	if !ValidUUID(project.ID) || project.Environments == nil {
		return false
	}
	for _, environment := range project.Environments {
		if !ValidUUID(environment.ID) {
			return false
		}
	}
	return true
}
