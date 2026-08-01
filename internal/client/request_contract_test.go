// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestClientRequestAndSuccessEnvelopeContract(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestCount.Add(1)
		if request.URL.EscapedPath() != "/api/v1/projects" {
			t.Error("request did not use the normalized /api/v1 path")
		}
		if request.Header.Get("Authorization") != syntheticAccessToken {
			t.Error("Authorization did not contain the direct access-token value")
		}
		if request.Header.Get("User-Agent") != "terraform-provider-featbit/test" {
			t.Error("User-Agent did not contain the provider product and version")
		}
		for _, header := range contextHeaders {
			if request.Header.Get(header) != "" {
				t.Errorf("request retained unsupported context header %q", header)
			}
		}

		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusOK)
		if _, err := io.WriteString(response, `{"success":true,"data":{"id":"synthetic-project-id"},"errors":[]}`); err != nil {
			t.Error("test server could not write the success envelope")
		}
	}))
	defer server.Close()

	options := defaultTestOptions()
	options.MaxRetries = 0
	clientUnderTest, err := New(
		mustParseURL(t, server.URL+"/api/v1"),
		syntheticAccessToken,
		options,
	)
	if err != nil {
		t.Fatal("New() could not construct the test client")
	}

	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/projects", nil)
	if err != nil {
		t.Fatal("http.NewRequest() could not construct the synthetic endpoint request")
	}
	request.Header.Set("Authorization", "caller-value-that-must-be-replaced")
	request.Header.Set("User-Agent", "caller-value-that-must-be-replaced")
	for _, header := range contextHeaders {
		request.Header.Set(header, "synthetic-context-value")
	}

	response, err := clientUnderTest.Do(request)
	if err != nil {
		t.Fatal("Client.Do() failed for the success envelope")
	}
	var decoded struct {
		ID string `json:"id"`
	}
	if err := clientUnderTest.DecodeResponse("read_project", response, &decoded); err != nil {
		t.Fatal("Client.DecodeResponse() rejected the success envelope")
	}

	if decoded.ID != "synthetic-project-id" {
		t.Fatal("Client.DecodeResponse() did not decode the envelope data")
	}
	if requestCount.Load() != 1 {
		t.Fatal("Client.Do() did not execute exactly one synthetic endpoint request")
	}
	if request.Header.Get("Authorization") != "caller-value-that-must-be-replaced" {
		t.Fatal("Client.Do() mutated the caller's Authorization header")
	}
	for _, header := range contextHeaders {
		if request.Header.Get(header) != "synthetic-context-value" {
			t.Fatalf("Client.Do() mutated caller context header %q", header)
		}
	}
}

func TestClientErrorEnvelopeContract(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		status             int
		envelope           string
		wantClassification Classification
		wantDetail         bool
	}{
		"2xx application failure": {
			status:             http.StatusOK,
			envelope:           `{"success":false,"data":null,"errors":["synthetic application failure"]}`,
			wantClassification: ClassificationApplicationFailure,
			wantDetail:         true,
		},
		"400 validation": {
			status:             http.StatusBadRequest,
			envelope:           `{"success":false,"data":null,"errors":["synthetic validation failure"]}`,
			wantClassification: ClassificationValidation,
			wantDetail:         true,
		},
		"422 validation": {
			status:             http.StatusUnprocessableEntity,
			envelope:           `{"success":false,"data":null,"errors":["synthetic validation failure"]}`,
			wantClassification: ClassificationValidation,
			wantDetail:         true,
		},
		"401 authentication": {
			status:             http.StatusUnauthorized,
			envelope:           `{"success":false,"data":null,"errors":["synthetic authentication failure"]}`,
			wantClassification: ClassificationAuthentication,
			wantDetail:         true,
		},
		"403 authorization": {
			status:             http.StatusForbidden,
			envelope:           `{"success":false,"data":null,"errors":["synthetic authorization failure"]}`,
			wantClassification: ClassificationAuthorization,
			wantDetail:         true,
		},
		"404 unconfirmed absence": {
			status:             http.StatusNotFound,
			envelope:           `{"success":false,"data":null,"errors":["synthetic not-found failure"]}`,
			wantClassification: ClassificationNotFoundUnconfirmed,
			wantDetail:         true,
		},
		"409 conflict": {
			status:             http.StatusConflict,
			envelope:           `{"success":false,"data":null,"errors":["synthetic conflict failure"]}`,
			wantClassification: ClassificationConflict,
			wantDetail:         true,
		},
		"429 rate limit": {
			status:             http.StatusTooManyRequests,
			envelope:           `{"success":false,"data":null,"errors":["synthetic rate-limit failure"]}`,
			wantClassification: ClassificationRateLimited,
			wantDetail:         true,
		},
		"500 transient server failure": {
			status:             http.StatusInternalServerError,
			envelope:           `{"success":false,"data":null,"errors":["synthetic server failure"]}`,
			wantClassification: ClassificationTransientServer,
			wantDetail:         true,
		},
		"599 transient server boundary": {
			status:             599,
			envelope:           `{"success":false,"data":null,"errors":["synthetic server failure"]}`,
			wantClassification: ClassificationTransientServer,
			wantDetail:         true,
		},
		"malformed JSON envelope": {
			status:             http.StatusOK,
			envelope:           `{"success":`,
			wantClassification: ClassificationAmbiguous,
		},
		"missing success member": {
			status:             http.StatusOK,
			envelope:           `{"data":{"id":"synthetic-project-id"},"errors":[]}`,
			wantClassification: ClassificationAmbiguous,
		},
		"missing data member": {
			status:             http.StatusOK,
			envelope:           `{"success":true,"errors":[]}`,
			wantClassification: ClassificationAmbiguous,
		},
		"invalid data member": {
			status:             http.StatusOK,
			envelope:           `{"success":true,"data":"not-an-object","errors":[]}`,
			wantClassification: ClassificationAmbiguous,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var requestCount atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				requestCount.Add(1)
				response.Header().Set("Content-Type", "application/json")
				response.WriteHeader(test.status)
				if _, err := io.WriteString(response, test.envelope); err != nil {
					t.Error("test server could not write the response envelope")
				}
			}))
			defer server.Close()

			options := defaultTestOptions()
			options.MaxRetries = 0
			clientUnderTest, err := New(
				mustParseURL(t, server.URL+"/api/v1"),
				syntheticAccessToken,
				options,
			)
			if err != nil {
				t.Fatal("New() could not construct the test client")
			}
			request, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/synthetic", nil)
			if err != nil {
				t.Fatal("http.NewRequest() could not construct the synthetic endpoint request")
			}

			response, err := clientUnderTest.Do(request)
			if err != nil {
				t.Fatal("Client.Do() rejected an HTTP response before envelope decoding")
			}
			var destination struct {
				ID string `json:"id"`
			}
			err = clientUnderTest.DecodeResponse("read_project", response, &destination)
			if err == nil {
				t.Fatal("Client.DecodeResponse() accepted an error or malformed envelope")
			}

			var apiError *APIError
			if !errors.As(err, &apiError) {
				t.Fatal("Client.DecodeResponse() did not return an APIError")
			}
			if apiError.Classification() != test.wantClassification {
				t.Fatalf(
					"APIError classification = %q, want %q",
					apiError.Classification(),
					test.wantClassification,
				)
			}
			if apiError.StatusCode() != test.status {
				t.Fatalf("APIError status = %d, want %d", apiError.StatusCode(), test.status)
			}
			if got := len(apiError.Details()); (got == 1) != test.wantDetail {
				t.Fatal("APIError details did not match the envelope contract")
			}
			if requestCount.Load() != 1 {
				t.Fatal("Client.Do() did not execute exactly one synthetic endpoint request")
			}
		})
	}
}
