// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestProjectEnvironmentEndpointFailuresRedactRuntimeValues(t *testing.T) {
	t.Parallel()

	const (
		tokenMarker          = "api-project-environment-endpoint-token-marker"
		projectKeyMarker     = "project-environment-project-key-marker"
		environmentKeyMarker = "project-environment-environment-key-marker"
		serverDetailMarker   = "project-environment-server-detail-marker"
	)
	pathMarker := "/api/v1/projects/" + projectIDOne + "/envs/" + environmentOne
	endpointBody := func(key string) []byte {
		t.Helper()
		detail := strings.Join([]string{
			tokenMarker,
			projectIDOne,
			environmentOne,
			key,
			pathMarker,
			serverDetailMarker,
		}, " | ")
		body, err := json.Marshal(map[string]any{
			"success": false,
			"data":    nil,
			"errors":  []string{detail},
		})
		if err != nil {
			t.Fatal("could not construct endpoint redaction response")
		}
		return body
	}
	projectBody := endpointBody(projectKeyMarker)
	environmentBody := endpointBody(environmentKeyMarker)

	options := defaultTestOptions()
	options.MaxRetries = 0
	clientUnderTest, err := newClient(
		mustParseURL(t, "https://project-environment-server-detail.example.invalid/api/v1"),
		tokenMarker,
		options,
		roundTripFunc(func(request *http.Request) (*http.Response, error) {
			body := projectBody
			if strings.Contains(request.URL.EscapedPath(), "/envs") {
				body = environmentBody
			}
			return syntheticResponse(
				request,
				http.StatusBadRequest,
				io.NopCloser(strings.NewReader(string(body))),
			), nil
		}),
	)
	if err != nil {
		t.Fatal("could not construct endpoint redaction client")
	}

	_, projectErr := clientUnderTest.CreateProject(
		context.Background(),
		CreateProjectRequest{Name: "Project", Key: projectKeyMarker},
	)
	_, environmentErr := clientUnderTest.CreateEnvironment(
		context.Background(),
		projectIDOne,
		CreateEnvironmentRequest{Name: "Environment", Key: environmentKeyMarker},
	)

	for name, endpointErr := range map[string]error{
		"project":     projectErr,
		"environment": environmentErr,
	} {
		t.Run(name, func(t *testing.T) {
			apiError := requireAPIErrorClassification(
				t,
				endpointErr,
				ClassificationValidation,
			)
			formatted := fmt.Sprintf("%v|%+v|%#v", apiError, apiError, apiError)
			for _, unsafe := range []string{
				tokenMarker,
				projectIDOne,
				environmentOne,
				projectKeyMarker,
				environmentKeyMarker,
				pathMarker,
				serverDetailMarker,
				"project-environment-server-detail.example.invalid",
			} {
				if strings.Contains(formatted, unsafe) {
					t.Fatal("formatted endpoint error exposed a runtime or server value")
				}
			}

			redactedDetails := strings.Join(apiError.Details(), " | ")
			for _, unsafe := range []string{
				tokenMarker,
				projectIDOne,
				environmentOne,
				projectKeyMarker,
				environmentKeyMarker,
				pathMarker,
			} {
				if strings.Contains(redactedDetails, unsafe) {
					t.Fatal("endpoint error details exposed a credential, path, key, or UUID")
				}
			}
		})
	}
}
