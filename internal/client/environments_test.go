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

func TestGetEnvironmentDirectContractAndSafeWireShape(t *testing.T) {
	t.Parallel()

	const secretMarker = "test-only-environment-secret-marker"
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			if request.Method != http.MethodGet {
				t.Fatalf("method = %q, want GET", request.Method)
			}
			wantPath := "/api/v1/projects/" + projectIDOne + "/envs/" + environmentOne
			if got := request.URL.EscapedPath(); got != wantPath {
				t.Fatalf("escaped path = %q, want %q", got, wantPath)
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

			return environmentTestResponse(
				request,
				http.StatusOK,
				`{"id":"`+environmentOne+`","name":"Staging","key":"staging",`+
					`"secrets":[{"value":"`+secretMarker+`"}],`+
					`"settings":{"requireChangeComment":true}}`,
			), nil
		},
	))

	environment, found, err := clientUnderTest.GetEnvironment(
		context.Background(),
		projectIDOne,
		environmentOne,
	)
	if err != nil {
		t.Fatalf("GetEnvironment() error = %v", err)
	}
	if !found || environment.ID != environmentOne || environment.Key != "staging" ||
		environment.Description != "" {
		t.Fatalf("GetEnvironment() = %#v, found %t", environment, found)
	}
	encoded, err := json.Marshal(environment)
	if err != nil {
		t.Fatalf("json.Marshal(Environment) error = %v", err)
	}
	formatted := fmt.Sprintf("%v|%+v|%#v|%s", environment, environment, environment, encoded)
	if strings.Contains(formatted, secretMarker) || strings.Contains(formatted, "settings") {
		t.Fatal("safe Environment wire model retained a secret or settings field")
	}
	if calls.Load() != 1 {
		t.Fatalf("request count = %d, want 1", calls.Load())
	}
}

func TestGetEnvironmentEnrichesCurrentKeylessResponseFromExactParent(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			switch calls.Add(1) {
			case 1:
				wantPath := "/api/v1/projects/" + projectIDOne + "/envs/" + environmentOne
				if request.Method != http.MethodGet || request.URL.EscapedPath() != wantPath {
					t.Fatalf("direct request = %s %s", request.Method, request.URL.EscapedPath())
				}
				return environmentTestResponse(
					request,
					http.StatusOK,
					`{"id":"`+environmentOne+`","name":"Direct Name",`+
						`"description":"Direct Description","settings":{"requireChangeComment":true}}`,
				), nil
			case 2:
				if request.Method != http.MethodGet ||
					request.URL.EscapedPath() != "/api/v1/projects/"+projectIDOne {
					t.Fatalf("parent request = %s %s", request.Method, request.URL.EscapedPath())
				}
				return environmentTestResponse(
					request,
					http.StatusOK,
					environmentParentJSON(
						projectIDOne,
						`[{"id":"`+environmentOne+`","name":"Parent Name",`+
							`"key":"staging","description":"Parent Description"}]`,
					),
				), nil
			default:
				t.Fatal("GetEnvironment() made an unexpected request")
				return nil, errors.New("unexpected request")
			}
		},
	))

	environment, found, err := clientUnderTest.GetEnvironment(
		context.Background(),
		projectIDOne,
		environmentOne,
	)
	if err != nil {
		t.Fatalf("GetEnvironment() error = %v", err)
	}
	if !found || environment.Key != "staging" || environment.Name != "Direct Name" ||
		environment.Description != "Direct Description" {
		t.Fatalf("GetEnvironment() = %#v, found %t", environment, found)
	}
}

func TestGetEnvironmentExactParentFallbackOutcomes(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		environments       string
		wantFound          bool
		wantClassification Classification
	}{
		"exact zero ignores another UUID": {
			environments: `[{"id":"` + environmentTwo + `","name":"Other","key":"other"}]`,
		},
		"exact one": {
			environments: `[{"id":"` + environmentOne + `","name":"Exact","key":"exact"}]`,
			wantFound:    true,
		},
		"duplicate exact IDs": {
			environments: `[{"id":"` + environmentOne + `","name":"First","key":"first"},` +
				`{"id":"` + environmentOne + `","name":"Second","key":"second"}]`,
			wantClassification: ClassificationAmbiguous,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					switch calls.Add(1) {
					case 1:
						return environmentTestResponse(request, http.StatusForbidden, `null`), nil
					case 2:
						if request.URL.EscapedPath() != "/api/v1/projects/"+projectIDOne {
							t.Fatalf("parent path = %q", request.URL.EscapedPath())
						}
						return environmentTestResponse(
							request,
							http.StatusOK,
							environmentParentJSON(projectIDOne, test.environments),
						), nil
					default:
						t.Fatal("GetEnvironment() made an unexpected request")
						return nil, errors.New("unexpected request")
					}
				},
			))

			environment, found, err := clientUnderTest.GetEnvironment(
				context.Background(),
				projectIDOne,
				environmentOne,
			)
			if found != test.wantFound {
				t.Fatalf("found = %t, want %t", found, test.wantFound)
			}
			if test.wantClassification == "" {
				if err != nil {
					t.Fatalf("GetEnvironment() error = %v", err)
				}
				if found && environment.ID != environmentOne {
					t.Fatalf("GetEnvironment() = %#v", environment)
				}
				return
			}
			requireAPIErrorClassification(t, err, test.wantClassification)
		})
	}
}

func TestGetEnvironmentWrongParentReturnsExactZero(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			switch calls.Add(1) {
			case 1:
				wantPath := "/api/v1/projects/" + projectIDTwo + "/envs/" + environmentOne
				if request.URL.EscapedPath() != wantPath {
					t.Fatalf("direct path = %q, want %q", request.URL.EscapedPath(), wantPath)
				}
				return environmentTestResponse(request, http.StatusNotFound, `null`), nil
			case 2:
				return environmentTestResponse(
					request,
					http.StatusOK,
					environmentParentJSON(projectIDTwo, `[]`),
				), nil
			default:
				t.Fatal("GetEnvironment() made an unexpected request")
				return nil, errors.New("unexpected request")
			}
		},
	))

	_, found, err := clientUnderTest.GetEnvironment(
		context.Background(),
		projectIDTwo,
		environmentOne,
	)
	if err != nil || found {
		t.Fatalf("GetEnvironment() found = %t, error = %v", found, err)
	}
}

func TestGetEnvironmentRejectsInvalidUUIDsBeforeRequest(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, errors.New("request must not execute")
		},
	))

	tests := []struct {
		projectID     string
		environmentID string
	}{
		{projectID: "not-a/project", environmentID: environmentOne},
		{projectID: projectIDOne, environmentID: "not-an/environment"},
		{projectID: "", environmentID: ""},
	}
	for _, test := range tests {
		_, _, err := clientUnderTest.GetEnvironment(
			context.Background(),
			test.projectID,
			test.environmentID,
		)
		requireAPIErrorClassification(t, err, ClassificationValidation)
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid UUIDs executed %d requests", calls.Load())
	}
}

func TestGetEnvironmentCancellationDoesNotReachTransport(t *testing.T) {
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

	_, _, err := clientUnderTest.GetEnvironment(ctx, projectIDOne, environmentOne)
	apiError := requireAPIErrorClassification(t, err, ClassificationCanceled)
	if !errors.Is(apiError, context.Canceled) {
		t.Fatal("cancellation sentinel was not preserved")
	}
	if calls.Load() != 0 {
		t.Fatalf("canceled read executed %d transport calls", calls.Load())
	}
}

func TestCreateEnvironmentContract(t *testing.T) {
	t.Parallel()

	const secretMarker = "test-only-created-environment-secret-marker"
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			if request.Method != http.MethodPost ||
				request.URL.EscapedPath() != "/api/v1/projects/"+projectIDOne+"/envs" {
				t.Fatalf("request = %s %s", request.Method, request.URL.EscapedPath())
			}
			if request.URL.RawQuery != "" || request.Header.Get("Content-Type") != "application/json" {
				t.Fatal("Environment create request had unexpected query or Content-Type")
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("read create body: %v", err)
			}
			if got := string(body); got != `{"name":"Staging","key":"staging","description":"QA"}` {
				t.Fatalf("create body = %s", got)
			}
			return environmentTestResponse(
				request,
				http.StatusOK,
				`{"id":"`+environmentOne+`","name":"Staging","description":"QA",`+
					`"secrets":[{"value":"`+secretMarker+`"}],`+
					`"settings":{"requireChangeComment":false}}`,
			), nil
		},
	))

	environment, err := clientUnderTest.CreateEnvironment(
		context.Background(),
		projectIDOne,
		CreateEnvironmentRequest{Name: "Staging", Key: "staging", Description: "QA"},
	)
	if err != nil {
		t.Fatalf("CreateEnvironment() error = %v", err)
	}
	if environment.ID != environmentOne || environment.Key != "staging" || calls.Load() != 1 {
		t.Fatalf("CreateEnvironment() = %v, calls = %d", environment, calls.Load())
	}
	if formatted := fmt.Sprintf("%v|%+v|%#v", environment, environment, environment); strings.Contains(formatted, secretMarker) {
		t.Fatal("created Environment formatting retained a generated secret")
	}
}

func TestUpdateEnvironmentContractPreservesSettingsAndOmitsSecrets(t *testing.T) {
	t.Parallel()

	const secretMarker = "test-only-update-environment-secret-marker"
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			switch calls.Add(1) {
			case 1:
				return environmentTestResponse(
					request,
					http.StatusOK,
					`{"id":"`+environmentOne+`","name":"Original","key":"staging",`+
						`"description":"Before","secrets":[{"value":"`+secretMarker+`"}],`+
						`"settings":{"requireChangeComment":true,"future":{"mode":"keep"}}}`,
				), nil
			case 2:
				wantPath := "/api/v1/projects/" + projectIDOne + "/envs/" + environmentOne
				if request.Method != http.MethodPut || request.URL.EscapedPath() != wantPath {
					t.Fatalf("update request = %s %s", request.Method, request.URL.EscapedPath())
				}
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatalf("read update body: %v", err)
				}
				want := `{"name":"Renamed","description":"After",` +
					`"settings":{"requireChangeComment":true,"future":{"mode":"keep"}}}`
				if got := string(body); got != want {
					t.Fatalf("update body = %s, want %s", got, want)
				}
				if strings.Contains(string(body), secretMarker) || strings.Contains(string(body), "secrets") {
					t.Fatal("Environment update body round-tripped a generated secret")
				}
				return environmentTestResponse(
					request,
					http.StatusOK,
					`{"id":"`+environmentOne+`","name":"Renamed","description":"After",`+
						`"settings":{"requireChangeComment":true,"future":{"mode":"keep"}}}`,
				), nil
			default:
				t.Fatal("UpdateEnvironment() made an unexpected request")
				return nil, errors.New("unexpected request")
			}
		},
	))

	current, found, err := clientUnderTest.GetEnvironment(
		context.Background(),
		projectIDOne,
		environmentOne,
	)
	if err != nil || !found {
		t.Fatalf("GetEnvironment() current = %v, found = %t, error = %v", current, found, err)
	}
	err = clientUnderTest.UpdateEnvironment(
		context.Background(),
		projectIDOne,
		environmentOne,
		current,
		UpdateEnvironmentRequest{Name: "Renamed", Description: "After"},
	)
	if err != nil {
		t.Fatalf("UpdateEnvironment() error = %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("request count = %d, want 2", calls.Load())
	}
}

func TestUpdateEnvironmentRejectsMissingSettingsWithoutMutation(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, errors.New("request must not execute")
		},
	))

	err := clientUnderTest.UpdateEnvironment(
		context.Background(),
		projectIDOne,
		environmentOne,
		Environment{ID: environmentOne, Name: "Original", Key: "staging"},
		UpdateEnvironmentRequest{Name: "Renamed"},
	)
	requireAPIErrorClassification(t, err, ClassificationAmbiguous)
	if calls.Load() != 0 {
		t.Fatalf("missing settings executed %d mutations", calls.Load())
	}
}

func TestDeleteEnvironmentContract(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			wantPath := "/api/v1/projects/" + projectIDOne + "/envs/" + environmentOne
			if request.Method != http.MethodDelete || request.URL.EscapedPath() != wantPath {
				t.Fatalf("request = %s %s", request.Method, request.URL.EscapedPath())
			}
			if request.Body != nil && request.Body != http.NoBody {
				t.Fatal("DELETE request unexpectedly contained a body")
			}
			return environmentTestResponse(request, http.StatusOK, `true`), nil
		},
	))

	if err := clientUnderTest.DeleteEnvironment(
		context.Background(),
		projectIDOne,
		environmentOne,
	); err != nil {
		t.Fatalf("DeleteEnvironment() error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("request count = %d, want 1", calls.Load())
	}
}

func TestEnvironmentMutationsExecuteOnce(t *testing.T) {
	t.Parallel()

	var current Environment
	if err := json.Unmarshal(
		[]byte(`{"id":"`+environmentOne+`","name":"Original","key":"staging",`+
			`"settings":{"requireChangeComment":true}}`),
		&current,
	); err != nil {
		t.Fatalf("prepare current Environment: %v", err)
	}
	tests := map[string]func(*Client) error{
		"create": func(apiClient *Client) error {
			_, err := apiClient.CreateEnvironment(
				context.Background(),
				projectIDOne,
				CreateEnvironmentRequest{Name: "Staging", Key: "staging"},
			)
			return err
		},
		"update": func(apiClient *Client) error {
			return apiClient.UpdateEnvironment(
				context.Background(),
				projectIDOne,
				environmentOne,
				current,
				UpdateEnvironmentRequest{Name: "Renamed"},
			)
		},
		"delete": func(apiClient *Client) error {
			return apiClient.DeleteEnvironment(context.Background(), projectIDOne, environmentOne)
		},
	}

	for name, invoke := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					calls.Add(1)
					return environmentTestResponse(request, http.StatusInternalServerError, `null`), nil
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

func TestEnvironmentMutationValidationAndFalseDelete(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			return environmentTestResponse(request, http.StatusOK, `false`), nil
		},
	))

	_, createErr := clientUnderTest.CreateEnvironment(
		context.Background(),
		"invalid",
		CreateEnvironmentRequest{},
	)
	requireAPIErrorClassification(t, createErr, ClassificationValidation)
	requireAPIErrorClassification(
		t,
		clientUnderTest.DeleteEnvironment(context.Background(), projectIDOne, "invalid"),
		ClassificationValidation,
	)
	requireAPIErrorClassification(
		t,
		clientUnderTest.DeleteEnvironment(context.Background(), projectIDOne, environmentOne),
		ClassificationAmbiguous,
	)
	if calls.Load() != 1 {
		t.Fatalf("request count = %d, want only the valid delete request", calls.Load())
	}
}

func TestEnvironmentMutationCancellationDoesNotReachTransport(t *testing.T) {
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

	_, err := clientUnderTest.CreateEnvironment(
		ctx,
		projectIDOne,
		CreateEnvironmentRequest{Name: "Staging", Key: "staging"},
	)
	apiError := requireAPIErrorClassification(t, err, ClassificationCanceled)
	if !errors.Is(apiError, context.Canceled) {
		t.Fatal("mutation cancellation sentinel was not preserved")
	}
	if calls.Load() != 0 {
		t.Fatalf("canceled mutation executed %d transport calls", calls.Load())
	}
}

func environmentTestResponse(request *http.Request, status int, data string) *http.Response {
	success := status >= http.StatusOK && status < http.StatusMultipleChoices
	body := `{"success":` + strconv.FormatBool(success) + `,"data":` + data + `,"errors":[]}`
	return syntheticResponse(request, status, io.NopCloser(strings.NewReader(body)))
}

func environmentParentJSON(projectID string, environments string) string {
	return `{"id":"` + projectID + `","name":"Parent","key":"parent","environments":` +
		environments + `}`
}
