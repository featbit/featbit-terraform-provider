// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
)

type projectProtocolFixture struct {
	server *httptest.Server

	mu                         sync.Mutex
	projects                   map[string]projectFixtureObject
	nextProject                int
	nextEnvironment            int
	requests                   []projectFixtureRequest
	violations                 []string
	failDirectProjectRead      bool
	projectDirectFallbacks     int
	failDirectEnvironmentRead  bool
	environmentDirectFallbacks int
	settingsPreservedUpdates   int
}

type projectFixtureObject struct {
	ID           string                      `json:"id"`
	Name         string                      `json:"name"`
	Key          string                      `json:"key"`
	Environments []projectFixtureEnvironment `json:"environments"`
}

type projectFixtureEnvironment struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Key         string                 `json:"key"`
	Description string                 `json:"description"`
	Secrets     []projectFixtureSecret `json:"secrets"`
	Settings    json.RawMessage        `json:"settings"`
}

type projectFixtureSecret struct {
	Value string `json:"value"`
}

type projectFixtureRequest struct {
	Method string
	Path   string
}

type projectFixtureCreateInput struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type projectFixtureUpdateInput struct {
	Name string `json:"name"`
}

type projectFixtureCreateEnvironmentInput struct {
	Name        string `json:"name"`
	Key         string `json:"key"`
	Description string `json:"description"`
}

type projectFixtureUpdateEnvironmentInput struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Settings    json.RawMessage `json:"settings"`
}

type projectFixtureEnvironmentVM struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Secrets     []projectFixtureSecret `json:"secrets"`
	Settings    json.RawMessage        `json:"settings"`
}

func newProjectProtocolFixture(t *testing.T) *projectProtocolFixture {
	t.Helper()
	fixture := &projectProtocolFixture{
		projects: make(map[string]projectFixtureObject),
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.handle))
	return fixture
}

func (f *projectProtocolFixture) close() {
	f.server.Close()
}

func (f *projectProtocolFixture) apiOrigin() string {
	return f.server.URL
}

func (f *projectProtocolFixture) handle(response http.ResponseWriter, request *http.Request) {
	if !f.validateRequest(request) {
		writeProjectFixtureEnvelope(response, http.StatusBadRequest, nil)
		return
	}

	f.mu.Lock()
	f.requests = append(f.requests, projectFixtureRequest{
		Method: request.Method,
		Path:   request.URL.EscapedPath(),
	})
	f.mu.Unlock()

	path := request.URL.EscapedPath()
	if path == "/api/v1/projects" {
		f.handleCollection(response, request)
		return
	}
	const prefix = "/api/v1/projects/"
	if !strings.HasPrefix(path, prefix) {
		f.recordViolation("unexpected Project API path")
		writeProjectFixtureEnvelope(response, http.StatusNotFound, nil)
		return
	}
	segments := strings.Split(strings.TrimPrefix(path, prefix), "/")
	switch {
	case len(segments) == 1 && segments[0] != "":
		f.handleExact(response, request, segments[0])
	case len(segments) == 2 && segments[0] != "" && segments[1] == "envs":
		f.handleEnvironmentCollection(response, request, segments[0])
	case len(segments) == 3 && segments[0] != "" && segments[1] == "envs" && segments[2] != "":
		f.handleEnvironmentExact(response, request, segments[0], segments[2])
	default:
		f.recordViolation("unexpected Project or Environment API path")
		writeProjectFixtureEnvelope(response, http.StatusNotFound, nil)
	}
}

func (f *projectProtocolFixture) validateRequest(request *http.Request) bool {
	valid := true
	if request.URL.RawQuery != "" {
		f.recordViolation("request contained a query")
		valid = false
	}
	if request.Header.Get("Organization") != "" || request.Header.Get("Workspace") != "" {
		f.recordViolation("request contained an organization or workspace header")
		valid = false
	}
	if request.Header.Get("Authorization") != syntheticProviderAccessToken {
		f.recordViolation("request did not use the configured direct access token")
		valid = false
	}
	if request.Header.Get("User-Agent") != "terraform-provider-featbit/protocol-test" {
		f.recordViolation("request used an unexpected User-Agent")
		valid = false
	}
	return valid
}

func (f *projectProtocolFixture) handleCollection(
	response http.ResponseWriter,
	request *http.Request,
) {
	switch request.Method {
	case http.MethodGet:
		f.mu.Lock()
		projects := make([]projectFixtureObject, 0, len(f.projects))
		for _, project := range f.projects {
			projects = append(projects, cloneFixtureProject(project))
		}
		f.mu.Unlock()
		sort.Slice(projects, func(left, right int) bool {
			return projects[left].ID > projects[right].ID
		})
		writeProjectFixtureEnvelope(response, http.StatusOK, projects)
	case http.MethodPost:
		if request.Header.Get("Content-Type") != "application/json" {
			f.recordViolation("Project create omitted application/json Content-Type")
			writeProjectFixtureEnvelope(response, http.StatusBadRequest, nil)
			return
		}
		var input projectFixtureCreateInput
		if err := decodeProjectFixtureBody(request.Body, &input); err != nil ||
			input.Name == "" || input.Key == "" {
			f.recordViolation("Project create body did not match the public contract")
			writeProjectFixtureEnvelope(response, http.StatusBadRequest, nil)
			return
		}

		f.mu.Lock()
		for _, project := range f.projects {
			if project.Key == input.Key {
				f.mu.Unlock()
				writeProjectFixtureEnvelope(response, http.StatusConflict, nil)
				return
			}
		}
		f.nextProject++
		sequence := f.nextProject
		project := projectFixtureObject{
			ID:   projectFixtureUUID(sequence, 0),
			Name: input.Name,
			Key:  input.Key,
			// Deliberately return reverse canonical order. Provider state must
			// always sort by key and ID.
			Environments: []projectFixtureEnvironment{
				newProjectFixtureEnvironment(
					projectFixtureUUID(sequence, 2),
					"Prod",
					"prod",
					"Production",
				),
				newProjectFixtureEnvironment(
					projectFixtureUUID(sequence, 1),
					"Dev",
					"dev",
					"Development",
				),
			},
		}
		f.projects[project.ID] = project
		f.mu.Unlock()
		writeProjectFixtureEnvelope(response, http.StatusOK, project)
	default:
		f.recordViolation("unexpected method on Project collection")
		writeProjectFixtureEnvelope(response, http.StatusNotFound, nil)
	}
}

func (f *projectProtocolFixture) handleExact(
	response http.ResponseWriter,
	request *http.Request,
	projectID string,
) {
	switch request.Method {
	case http.MethodGet:
		f.mu.Lock()
		if f.failDirectProjectRead {
			f.projectDirectFallbacks++
			f.mu.Unlock()
			writeProjectFixtureEnvelope(response, http.StatusForbidden, nil)
			return
		}
		project, found := f.projects[projectID]
		f.mu.Unlock()
		if !found {
			writeProjectFixtureEnvelope(response, http.StatusNotFound, nil)
			return
		}
		writeProjectFixtureEnvelope(response, http.StatusOK, cloneFixtureProject(project))
	case http.MethodPut:
		if request.Header.Get("Content-Type") != "application/json" {
			f.recordViolation("Project update omitted application/json Content-Type")
			writeProjectFixtureEnvelope(response, http.StatusBadRequest, nil)
			return
		}
		var input projectFixtureUpdateInput
		if err := decodeProjectFixtureBody(request.Body, &input); err != nil || input.Name == "" {
			f.recordViolation("Project update body was not name-only")
			writeProjectFixtureEnvelope(response, http.StatusBadRequest, nil)
			return
		}
		f.mu.Lock()
		project, found := f.projects[projectID]
		if found {
			project.Name = input.Name
			f.projects[projectID] = project
		}
		f.mu.Unlock()
		if !found {
			writeProjectFixtureEnvelope(response, http.StatusNotFound, nil)
			return
		}
		writeProjectFixtureEnvelope(response, http.StatusOK, map[string]string{
			"id":   project.ID,
			"name": project.Name,
		})
	case http.MethodDelete:
		if request.Body != nil && request.Body != http.NoBody {
			body, err := io.ReadAll(request.Body)
			if err != nil || len(body) != 0 {
				f.recordViolation("Project delete contained a request body")
				writeProjectFixtureEnvelope(response, http.StatusBadRequest, nil)
				return
			}
		}
		f.mu.Lock()
		_, found := f.projects[projectID]
		if found {
			delete(f.projects, projectID)
		}
		f.mu.Unlock()
		if !found {
			writeProjectFixtureEnvelope(response, http.StatusNotFound, nil)
			return
		}
		writeProjectFixtureEnvelope(response, http.StatusOK, true)
	default:
		f.recordViolation("unexpected method on exact Project endpoint")
		writeProjectFixtureEnvelope(response, http.StatusNotFound, nil)
	}
}

func (f *projectProtocolFixture) handleEnvironmentCollection(
	response http.ResponseWriter,
	request *http.Request,
	projectID string,
) {
	if request.Method != http.MethodPost {
		f.recordViolation("unexpected method on Environment collection")
		writeProjectFixtureEnvelope(response, http.StatusNotFound, nil)
		return
	}
	if request.Header.Get("Content-Type") != "application/json" {
		f.recordViolation("Environment create omitted application/json Content-Type")
		writeProjectFixtureEnvelope(response, http.StatusBadRequest, nil)
		return
	}
	var input projectFixtureCreateEnvironmentInput
	if err := decodeProjectFixtureBody(request.Body, &input); err != nil ||
		input.Name == "" || input.Key == "" {
		f.recordViolation("Environment create body did not match the public contract")
		writeProjectFixtureEnvelope(response, http.StatusBadRequest, nil)
		return
	}

	f.mu.Lock()
	project, found := f.projects[projectID]
	if found {
		for _, environment := range project.Environments {
			if environment.Key == input.Key {
				f.mu.Unlock()
				writeProjectFixtureEnvelope(response, http.StatusConflict, nil)
				return
			}
		}
		f.nextEnvironment++
		environment := newProjectFixtureEnvironment(
			projectFixtureUUID(1000+f.nextEnvironment, f.nextEnvironment),
			input.Name,
			input.Key,
			input.Description,
		)
		project.Environments = append(project.Environments, environment)
		f.projects[projectID] = project
		f.mu.Unlock()
		writeProjectFixtureEnvelope(response, http.StatusOK, environmentFixtureVM(environment))
		return
	}
	f.mu.Unlock()
	writeProjectFixtureEnvelope(response, http.StatusNotFound, nil)
}

func (f *projectProtocolFixture) handleEnvironmentExact(
	response http.ResponseWriter,
	request *http.Request,
	projectID string,
	environmentID string,
) {
	switch request.Method {
	case http.MethodGet:
		f.mu.Lock()
		if f.failDirectEnvironmentRead {
			f.environmentDirectFallbacks++
			f.mu.Unlock()
			writeProjectFixtureEnvelope(response, http.StatusForbidden, nil)
			return
		}
		project, projectFound := f.projects[projectID]
		index := fixtureEnvironmentIndex(project.Environments, environmentID)
		f.mu.Unlock()
		if !projectFound || index < 0 {
			writeProjectFixtureEnvelope(response, http.StatusNotFound, nil)
			return
		}
		writeProjectFixtureEnvelope(
			response,
			http.StatusOK,
			environmentFixtureVM(project.Environments[index]),
		)
	case http.MethodPut:
		if request.Header.Get("Content-Type") != "application/json" {
			f.recordViolation("Environment update omitted application/json Content-Type")
			writeProjectFixtureEnvelope(response, http.StatusBadRequest, nil)
			return
		}
		var input projectFixtureUpdateEnvironmentInput
		if err := decodeProjectFixtureBody(request.Body, &input); err != nil ||
			input.Name == "" || !validFixtureSettings(input.Settings) {
			f.recordViolation("Environment update body did not match the public contract")
			writeProjectFixtureEnvelope(response, http.StatusBadRequest, nil)
			return
		}

		f.mu.Lock()
		project, projectFound := f.projects[projectID]
		index := fixtureEnvironmentIndex(project.Environments, environmentID)
		if projectFound && index >= 0 &&
			!equalFixtureJSON(project.Environments[index].Settings, input.Settings) {
			f.violations = append(f.violations, "Environment update changed UI-owned settings")
			f.mu.Unlock()
			writeProjectFixtureEnvelope(response, http.StatusBadRequest, nil)
			return
		}
		if projectFound && index >= 0 {
			environment := project.Environments[index]
			environment.Name = input.Name
			environment.Description = input.Description
			environment.Settings = append(json.RawMessage(nil), input.Settings...)
			project.Environments[index] = environment
			f.projects[projectID] = project
			f.settingsPreservedUpdates++
			f.mu.Unlock()
			writeProjectFixtureEnvelope(response, http.StatusOK, environmentFixtureVM(environment))
			return
		}
		f.mu.Unlock()
		writeProjectFixtureEnvelope(response, http.StatusNotFound, nil)
	case http.MethodDelete:
		if request.Body != nil && request.Body != http.NoBody {
			body, err := io.ReadAll(request.Body)
			if err != nil || len(body) != 0 {
				f.recordViolation("Environment delete contained a request body")
				writeProjectFixtureEnvelope(response, http.StatusBadRequest, nil)
				return
			}
		}
		f.mu.Lock()
		project, projectFound := f.projects[projectID]
		index := fixtureEnvironmentIndex(project.Environments, environmentID)
		if projectFound && index >= 0 {
			project.Environments = append(
				project.Environments[:index],
				project.Environments[index+1:]...,
			)
			f.projects[projectID] = project
		}
		f.mu.Unlock()
		if !projectFound || index < 0 {
			writeProjectFixtureEnvelope(response, http.StatusNotFound, nil)
			return
		}
		writeProjectFixtureEnvelope(response, http.StatusOK, true)
	default:
		f.recordViolation("unexpected method on exact Environment endpoint")
		writeProjectFixtureEnvelope(response, http.StatusNotFound, nil)
	}
}

func (f *projectProtocolFixture) renameProject(projectID, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	project, found := f.projects[projectID]
	if !found {
		return fmt.Errorf("fixture Project does not exist")
	}
	project.Name = name
	f.projects[projectID] = project
	return nil
}

func (f *projectProtocolFixture) removeProject(projectID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, found := f.projects[projectID]; !found {
		return fmt.Errorf("fixture Project does not exist")
	}
	delete(f.projects, projectID)
	return nil
}

func (f *projectProtocolFixture) hasProject(projectID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, found := f.projects[projectID]
	return found
}

func (f *projectProtocolFixture) projectName(projectID string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	project, found := f.projects[projectID]
	return project.Name, found
}

func (f *projectProtocolFixture) projectCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.projects)
}

func (f *projectProtocolFixture) renameEnvironment(
	projectID string,
	environmentID string,
	name string,
	description string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	project, found := f.projects[projectID]
	if !found {
		return fmt.Errorf("fixture parent Project does not exist")
	}
	index := fixtureEnvironmentIndex(project.Environments, environmentID)
	if index < 0 {
		return fmt.Errorf("fixture Environment does not exist")
	}
	project.Environments[index].Name = name
	project.Environments[index].Description = description
	f.projects[projectID] = project
	return nil
}

func (f *projectProtocolFixture) removeEnvironment(projectID string, environmentID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	project, found := f.projects[projectID]
	if !found {
		return fmt.Errorf("fixture parent Project does not exist")
	}
	index := fixtureEnvironmentIndex(project.Environments, environmentID)
	if index < 0 {
		return fmt.Errorf("fixture Environment does not exist")
	}
	project.Environments = append(
		project.Environments[:index],
		project.Environments[index+1:]...,
	)
	f.projects[projectID] = project
	return nil
}

func (f *projectProtocolFixture) hasEnvironment(projectID string, environmentID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	project, found := f.projects[projectID]
	return found && fixtureEnvironmentIndex(project.Environments, environmentID) >= 0
}

func (f *projectProtocolFixture) environmentValues(
	projectID string,
	environmentID string,
) (string, string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	project, found := f.projects[projectID]
	if !found {
		return "", "", false
	}
	index := fixtureEnvironmentIndex(project.Environments, environmentID)
	if index < 0 {
		return "", "", false
	}
	environment := project.Environments[index]
	return environment.Name, environment.Description, true
}

func (f *projectProtocolFixture) environmentCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, project := range f.projects {
		count += len(project.Environments)
	}
	return count
}

func (f *projectProtocolFixture) setDirectReadFailure(enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failDirectProjectRead = enabled
}

func (f *projectProtocolFixture) directFallbackCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.projectDirectFallbacks
}

func (f *projectProtocolFixture) setDirectEnvironmentReadFailure(enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failDirectEnvironmentRead = enabled
}

func (f *projectProtocolFixture) environmentDirectFallbackCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.environmentDirectFallbacks
}

func (f *projectProtocolFixture) settingsPreservedUpdateCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.settingsPreservedUpdates
}

func (f *projectProtocolFixture) requestSnapshot() []projectFixtureRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]projectFixtureRequest(nil), f.requests...)
}

func (f *projectProtocolFixture) violationSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.violations...)
}

func (f *projectProtocolFixture) recordViolation(message string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.violations = append(f.violations, message)
}

func cloneFixtureProject(project projectFixtureObject) projectFixtureObject {
	project.Environments = append([]projectFixtureEnvironment(nil), project.Environments...)
	for index := range project.Environments {
		project.Environments[index].Secrets = append(
			[]projectFixtureSecret(nil),
			project.Environments[index].Secrets...,
		)
		project.Environments[index].Settings = append(
			json.RawMessage(nil),
			project.Environments[index].Settings...,
		)
	}
	return project
}

func newProjectFixtureEnvironment(
	id string,
	name string,
	key string,
	description string,
) projectFixtureEnvironment {
	return projectFixtureEnvironment{
		ID:          id,
		Name:        name,
		Key:         key,
		Description: description,
		Secrets: []projectFixtureSecret{
			{Value: "test-only-protocol-environment-secret-marker"},
		},
		Settings: json.RawMessage(
			`{"requireChangeComment":true,"future":{"mode":"keep"}}`,
		),
	}
}

func environmentFixtureVM(environment projectFixtureEnvironment) projectFixtureEnvironmentVM {
	return projectFixtureEnvironmentVM{
		ID:          environment.ID,
		Name:        environment.Name,
		Description: environment.Description,
		Secrets:     append([]projectFixtureSecret(nil), environment.Secrets...),
		Settings:    append(json.RawMessage(nil), environment.Settings...),
	}
}

func fixtureEnvironmentIndex(environments []projectFixtureEnvironment, environmentID string) int {
	for index, environment := range environments {
		if environment.ID == environmentID {
			return index
		}
	}
	return -1
}

func validFixtureSettings(settings json.RawMessage) bool {
	settings = bytes.TrimSpace(settings)
	return len(settings) >= 2 && settings[0] == '{' && settings[len(settings)-1] == '}' &&
		json.Valid(settings)
}

func equalFixtureJSON(left json.RawMessage, right json.RawMessage) bool {
	var compactLeft bytes.Buffer
	var compactRight bytes.Buffer
	if json.Compact(&compactLeft, left) != nil || json.Compact(&compactRight, right) != nil {
		return false
	}
	return bytes.Equal(compactLeft.Bytes(), compactRight.Bytes())
}

func projectFixtureUUID(projectSequence, childSequence int) string {
	return fmt.Sprintf(
		"%08x-%04x-4%03x-8%03x-%012x",
		projectSequence,
		childSequence,
		projectSequence,
		childSequence,
		projectSequence*16+childSequence,
	)
}

func decodeProjectFixtureBody(body io.ReadCloser, destination any) error {
	if body == nil {
		return fmt.Errorf("request body is missing")
	}
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("request body contains trailing JSON")
	}
	return nil
}

func writeProjectFixtureEnvelope(response http.ResponseWriter, status int, data any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	success := status >= http.StatusOK && status < http.StatusMultipleChoices
	_ = json.NewEncoder(response).Encode(map[string]any{
		"success": success,
		"data":    data,
		"errors":  []string{},
	})
}
