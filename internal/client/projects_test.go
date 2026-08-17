// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	projectIDOne   = "11111111-1111-4111-8111-111111111111"
	projectIDTwo   = "22222222-2222-4222-8222-222222222222"
	environmentOne = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	environmentTwo = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

func TestGetProjectDirectContractAndSafeWireShape(t *testing.T) {
	t.Parallel()

	const secretMarker = "secret-value-that-must-not-be-decoded"
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			if request.Method != http.MethodGet {
				t.Fatalf("method = %q, want GET", request.Method)
			}
			if got := request.URL.EscapedPath(); got != "/api/v1/projects/"+projectIDOne {
				t.Fatalf("escaped path = %q", got)
			}
			if request.URL.RawQuery != "" {
				t.Fatalf("unexpected query = %q", request.URL.RawQuery)
			}
			if got := request.Header.Get("Authorization"); got != syntheticAccessToken {
				t.Fatal("request did not use direct access-token authorization")
			}
			if got := request.Header.Get("User-Agent"); got != "terraform-provider-featbit/test" {
				t.Fatalf("User-Agent = %q", got)
			}
			if request.Header.Get("Organization") != "" || request.Header.Get("Workspace") != "" {
				t.Fatal("request sent an unexpected context header")
			}

			body := `{"success":true,"data":{` +
				`"id":"` + projectIDOne + `","name":"Project","key":"project",` +
				`"environments":[` +
				`{"id":"` + environmentTwo + `","name":"Prod","key":"prod",` +
				`"description":"Production","secrets":[{"value":"` + secretMarker + `"}],` +
				`"settings":{"flagListFilter":{"filter":"unsafe"}}},` +
				`{"id":"` + environmentOne + `","name":"Dev","key":"dev"}` +
				`]},"errors":[]}`
			return syntheticResponse(
				request,
				http.StatusOK,
				io.NopCloser(strings.NewReader(body)),
			), nil
		},
	))

	project, found, err := clientUnderTest.GetProject(context.Background(), projectIDOne)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if !found || project.ID != projectIDOne || len(project.Environments) != 2 {
		t.Fatalf("GetProject() = %#v, found %t", project, found)
	}
	if project.Environments[1].Description != "" {
		t.Fatal("missing environment description did not decode as the canonical empty string")
	}
	encoded, err := json.Marshal(project)
	if err != nil {
		t.Fatalf("json.Marshal(Project) error = %v", err)
	}
	if strings.Contains(string(encoded), secretMarker) || strings.Contains(string(encoded), "settings") {
		t.Fatal("safe Project wire model retained a secret or settings field")
	}
	formatted := fmt.Sprintf(
		"%v|%+v|%#v|%v|%+v|%#v",
		project,
		project,
		project,
		project.Environments[0],
		project.Environments[0],
		project.Environments[0],
	)
	for _, unsafe := range []string{projectIDOne, environmentOne, environmentTwo, "project", "prod"} {
		if strings.Contains(formatted, unsafe) {
			t.Fatal("formatted Project wire model exposed a runtime identity")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("request count = %d, want 1", calls.Load())
	}
}

func TestGetProjectFallsBackToCompleteCollection(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			switch calls.Add(1) {
			case 1:
				if got := request.URL.EscapedPath(); got != "/api/v1/projects/"+projectIDOne {
					t.Fatalf("direct path = %q", got)
				}
				return projectTestResponse(request, http.StatusForbidden, `null`), nil
			case 2:
				if request.Method != http.MethodGet || request.URL.EscapedPath() != "/api/v1/projects" {
					t.Fatalf("fallback request = %s %s", request.Method, request.URL.EscapedPath())
				}
				if request.URL.RawQuery != "" {
					t.Fatalf("fallback query = %q", request.URL.RawQuery)
				}
				return projectTestResponse(request, http.StatusOK, `[`+
					projectTestJSON(projectIDTwo, "Other", "other")+`,`+
					projectTestJSON(projectIDOne, "Exact", "exact")+`]`), nil
			default:
				t.Fatal("GetProject() made an unexpected request")
				return nil, errors.New("unexpected request")
			}
		},
	))

	project, found, err := clientUnderTest.GetProject(context.Background(), projectIDOne)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if !found || project.Name != "Exact" {
		t.Fatalf("GetProject() = %#v, found %t", project, found)
	}
}

func TestGetProjectExactFallbackOutcomes(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		collection         string
		wantFound          bool
		wantClassification Classification
	}{
		"exact zero ignores fuzzy values": {
			collection: `[` + projectTestJSON(projectIDTwo, "Similar", "exact-extra") + `]`,
		},
		"exact one": {
			collection: `[` + projectTestJSON(projectIDOne, "Exact", "exact") + `]`,
			wantFound:  true,
		},
		"duplicate exact IDs": {
			collection: `[` + projectTestJSON(projectIDOne, "First", "first") + `,` +
				projectTestJSON(projectIDOne, "Second", "second") + `]`,
			wantClassification: ClassificationAmbiguous,
		},
		"null collection": {
			collection:         `null`,
			wantClassification: ClassificationAmbiguous,
		},
		"incomplete collection": {
			collection:         `[{"id":"","name":"Incomplete","key":"incomplete","environments":[]}]`,
			wantClassification: ClassificationAmbiguous,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					if calls.Add(1) == 1 {
						return projectTestResponse(request, http.StatusNotFound, `null`), nil
					}
					return projectTestResponse(request, http.StatusOK, test.collection), nil
				},
			))

			_, found, err := clientUnderTest.GetProject(context.Background(), projectIDOne)
			if found != test.wantFound {
				t.Fatalf("found = %t, want %t", found, test.wantFound)
			}
			if test.wantClassification == "" {
				if err != nil {
					t.Fatalf("GetProject() error = %v", err)
				}
				return
			}
			requireAPIErrorClassification(t, err, test.wantClassification)
		})
	}
}

func TestGetProjectByKeyExactOutcomes(t *testing.T) {
	t.Parallel()

	const exactKey = "exact-project-key"
	tests := map[string]struct {
		collection         string
		wantFound          bool
		wantName           string
		wantClassification Classification
	}{
		"case-sensitive exact zero ignores fuzzy values": {
			collection: `[` + projectTestJSON(projectIDOne, "Case", "Exact-Project-Key") + `,` +
				projectTestJSON(projectIDTwo, "Fuzzy", exactKey+"-extra") + `]`,
		},
		"exact one": {
			collection: `[` + projectTestJSON(projectIDTwo, "Other", "other") + `,` +
				projectTestJSON(projectIDOne, "Exact", exactKey) + `]`,
			wantFound: true,
			wantName:  "Exact",
		},
		"duplicate exact keys": {
			collection: `[` + projectTestJSON(projectIDOne, "First", exactKey) + `,` +
				projectTestJSON(projectIDTwo, "Second", exactKey) + `]`,
			wantClassification: ClassificationAmbiguous,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					calls.Add(1)
					if request.Method != http.MethodGet ||
						request.URL.EscapedPath() != "/api/v1/projects" ||
						request.URL.RawQuery != "" {
						t.Fatalf("request = %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
					}
					return projectTestResponse(request, http.StatusOK, test.collection), nil
				},
			))

			project, found, err := clientUnderTest.GetProjectByKey(
				context.Background(),
				exactKey,
			)
			if found != test.wantFound {
				t.Fatalf("found = %t, want %t", found, test.wantFound)
			}
			if test.wantClassification == "" {
				if err != nil {
					t.Fatalf("GetProjectByKey() error = %v", err)
				}
				if found && (project.Name != test.wantName || project.Key != exactKey) {
					t.Fatalf("GetProjectByKey() = %#v", project)
				}
			} else {
				requireAPIErrorClassification(t, err, test.wantClassification)
				if strings.Contains(fmt.Sprint(err), exactKey) {
					t.Fatal("exact-key ambiguity exposed the configured Project key")
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("request count = %d, want 1", calls.Load())
			}
		})
	}
}

func TestGetProjectRejectsInvalidUUIDBeforeRequest(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, errors.New("request must not execute")
		},
	))

	_, _, err := clientUnderTest.GetProject(context.Background(), "not-a/project")
	requireAPIErrorClassification(t, err, ClassificationValidation)
	if calls.Load() != 0 {
		t.Fatalf("invalid UUID executed %d requests", calls.Load())
	}
}

func TestGetProjectCancellationDoesNotReachTransport(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, errors.New("request must not execute")
		},
	))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := clientUnderTest.GetProject(ctx, projectIDOne)
	apiError := requireAPIErrorClassification(t, err, ClassificationCanceled)
	if !errors.Is(apiError, context.Canceled) {
		t.Fatal("cancellation sentinel was not preserved")
	}
	if calls.Load() != 0 {
		t.Fatalf("canceled read executed %d transport calls", calls.Load())
	}
}

func TestListProjectsEnvelopeFailure(t *testing.T) {
	t.Parallel()

	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			return syntheticResponse(
				request,
				http.StatusOK,
				io.NopCloser(strings.NewReader(`{"success":false,"data":[],"errors":["failure"]}`)),
			), nil
		},
	))

	_, err := clientUnderTest.ListProjects(context.Background())
	requireAPIErrorClassification(t, err, ClassificationApplicationFailure)
}

func TestCreateProjectContract(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			if request.Method != http.MethodPost || request.URL.EscapedPath() != "/api/v1/projects" {
				t.Fatalf("request = %s %s", request.Method, request.URL.EscapedPath())
			}
			if request.URL.RawQuery != "" {
				t.Fatalf("query = %q", request.URL.RawQuery)
			}
			if got := request.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q", got)
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			if got := string(body); got != `{"name":"Project","key":"project"}` {
				t.Fatalf("body = %s", got)
			}
			return projectTestResponse(
				request,
				http.StatusOK,
				projectTestJSON(projectIDOne, "Project", "project"),
			), nil
		},
	))

	project, err := clientUnderTest.CreateProject(context.Background(), CreateProjectRequest{
		Name: "Project",
		Key:  "project",
	})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if project.ID != projectIDOne || calls.Load() != 1 {
		t.Fatalf("CreateProject() = %#v, calls = %d", project, calls.Load())
	}
}

func TestUpdateProjectContractUsesNameOnlyBody(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			if request.Method != http.MethodPut ||
				request.URL.EscapedPath() != "/api/v1/projects/"+projectIDOne {
				t.Fatalf("request = %s %s", request.Method, request.URL.EscapedPath())
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			if got := string(body); got != `{"name":"Renamed"}` {
				t.Fatalf("body = %s", got)
			}
			return projectTestResponse(
				request,
				http.StatusOK,
				`{"id":"`+projectIDOne+`","name":"Renamed"}`,
			), nil
		},
	))

	err := clientUnderTest.UpdateProject(
		context.Background(),
		projectIDOne,
		UpdateProjectRequest{Name: "Renamed"},
	)
	if err != nil {
		t.Fatalf("UpdateProject() error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("request count = %d, want 1", calls.Load())
	}
}

func TestDeleteProjectContract(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			if request.Method != http.MethodDelete ||
				request.URL.EscapedPath() != "/api/v1/projects/"+projectIDOne {
				t.Fatalf("request = %s %s", request.Method, request.URL.EscapedPath())
			}
			if request.Body != nil && request.Body != http.NoBody {
				t.Fatal("DELETE request unexpectedly contained a body")
			}
			return projectTestResponse(request, http.StatusOK, `true`), nil
		},
	))

	if err := clientUnderTest.DeleteProject(context.Background(), projectIDOne); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("request count = %d, want 1", calls.Load())
	}
}

func TestProjectMutationsExecuteOnce(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*Client) error{
		"create": func(apiClient *Client) error {
			_, err := apiClient.CreateProject(
				context.Background(),
				CreateProjectRequest{Name: "Project", Key: "project"},
			)
			return err
		},
		"update": func(apiClient *Client) error {
			return apiClient.UpdateProject(
				context.Background(),
				projectIDOne,
				UpdateProjectRequest{Name: "Renamed"},
			)
		},
		"delete": func(apiClient *Client) error {
			return apiClient.DeleteProject(context.Background(), projectIDOne)
		},
	}

	for name, invoke := range tests {
		name := name
		invoke := invoke
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					calls.Add(1)
					return projectTestResponse(request, http.StatusInternalServerError, `null`), nil
				},
			))
			err := invoke(clientUnderTest)
			requireAPIErrorClassification(t, err, ClassificationTransientServer)
			if calls.Load() != 1 {
				t.Fatalf("mutation request count = %d, want 1", calls.Load())
			}
		})
	}
}

func TestProjectMutationValidationAndFalseDelete(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			return projectTestResponse(request, http.StatusOK, `false`), nil
		},
	))

	requireAPIErrorClassification(
		t,
		clientUnderTest.UpdateProject(context.Background(), "invalid", UpdateProjectRequest{}),
		ClassificationValidation,
	)
	requireAPIErrorClassification(
		t,
		clientUnderTest.DeleteProject(context.Background(), "invalid"),
		ClassificationValidation,
	)
	requireAPIErrorClassification(
		t,
		clientUnderTest.DeleteProject(context.Background(), projectIDOne),
		ClassificationAmbiguous,
	)
	if calls.Load() != 1 {
		t.Fatalf("request count = %d, want only the valid delete request", calls.Load())
	}
}

func TestProjectMutationCancellationDoesNotReachTransport(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, errors.New("request must not execute")
		},
	))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := clientUnderTest.CreateProject(ctx, CreateProjectRequest{
		Name: "Project",
		Key:  "project",
	})
	apiError := requireAPIErrorClassification(t, err, ClassificationCanceled)
	if !errors.Is(apiError, context.Canceled) {
		t.Fatal("mutation cancellation sentinel was not preserved")
	}
	if calls.Load() != 0 {
		t.Fatalf("canceled mutation executed %d transport calls", calls.Load())
	}
}

func projectTestResponse(request *http.Request, status int, data string) *http.Response {
	success := status >= http.StatusOK && status < http.StatusMultipleChoices
	body := `{"success":` + strconv.FormatBool(success) + `,"data":` + data + `,"errors":[]}`
	return syntheticResponse(request, status, io.NopCloser(strings.NewReader(body)))
}

func projectTestJSON(id, name, key string) string {
	return `{"id":"` + id + `","name":"` + name + `","key":"` + key +
		`","environments":[{"id":"` + environmentOne + `","name":"Dev","key":"dev","description":""}]}`
}
