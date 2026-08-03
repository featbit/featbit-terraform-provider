// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestNewClientDoesNotPerformLoginOrNetworkRequest(t *testing.T) {
	t.Parallel()

	requestCount := 0
	clientUnderTest, err := newClient(
		mustParseURL(t, "https://featbit.example.test/api/v1"),
		syntheticAccessToken,
		defaultTestOptions(),
		roundTripFunc(func(*http.Request) (*http.Response, error) {
			requestCount++
			return nil, errors.New("unexpected request")
		}),
	)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}
	if clientUnderTest == nil {
		t.Fatal("newClient() returned a nil client")
	}
	if requestCount != 0 {
		t.Fatalf("newClient() performed %d requests, want 0", requestCount)
	}
	if clientUnderTest.httpClient.Timeout != DefaultHTTPTimeout ||
		clientUnderTest.limiter == nil ||
		clientUnderTest.maxRetries != DefaultMaxRetries {
		t.Fatal("newClient() did not retain the resolved bounded settings")
	}
}

func TestClientSendsTokenDirectlyInAuthorization(t *testing.T) {
	t.Parallel()

	requestCount := 0
	clientUnderTest, err := newClient(
		mustParseURL(t, "https://featbit.example.test/api/v1"),
		syntheticAccessToken,
		defaultTestOptions(),
		roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requestCount++
			if got := request.Header.Get("Authorization"); got != syntheticAccessToken {
				t.Fatal("Authorization did not contain the direct access-token value")
			}
			if strings.HasPrefix(request.Header.Get("Authorization"), "Bearer ") {
				t.Fatal("Authorization unexpectedly used a Bearer prefix")
			}
			if got := request.Header.Get("User-Agent"); got != "terraform-provider-featbit/test" {
				t.Fatalf("User-Agent = %q, want provider product and version", got)
			}
			for _, header := range []string{
				"Organization",
				"Workspace",
				"X-Organization",
				"X-Organization-Id",
				"X-Workspace",
				"X-Workspace-Id",
			} {
				if request.Header.Get(header) != "" {
					t.Fatalf("client unexpectedly added context header %q", header)
				}
			}
			if request.URL.Path != "/api/v1/projects" {
				t.Fatalf("request path = %q, want documented API path", request.URL.Path)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("{}")),
				Request:    request,
			}, nil
		}),
	)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}

	request := mustNewRequest(
		t,
		http.MethodGet,
		"https://featbit.example.test/api/v1/projects",
		nil,
	)
	request.Header.Set("User-Agent", "caller-value")
	for _, header := range contextHeaders {
		request.Header.Set(header, "synthetic-tenant-value")
	}
	response, err := clientUnderTest.Do(request)
	if err != nil {
		t.Fatalf("Client.Do() error = %v", err)
	}
	mustCloseResponse(t, response)

	if requestCount != 1 {
		t.Fatalf("Client.Do() performed %d requests, want exactly 1", requestCount)
	}
	if request.Header.Get("Authorization") != "" {
		t.Fatal("Client.Do() mutated the caller's request with a credential")
	}
}

func TestClientRefusesToSendTokenOffOrigin(t *testing.T) {
	t.Parallel()

	requestCount := 0
	clientUnderTest, err := newClient(
		mustParseURL(t, "https://featbit.example.test/api/v1"),
		syntheticAccessToken,
		defaultTestOptions(),
		roundTripFunc(func(*http.Request) (*http.Response, error) {
			requestCount++
			return nil, errors.New("unexpected request")
		}),
	)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}

	request := mustNewRequest(
		t,
		http.MethodGet,
		"https://other.example.test/api/v1/projects",
		nil,
	)
	_, err = clientUnderTest.Do(request)
	if err == nil {
		t.Fatal("Client.Do() sent a request to an unconfigured origin")
	}
	if strings.Contains(err.Error(), syntheticAccessToken) {
		t.Fatal("off-origin error disclosed the access token")
	}
	if requestCount != 0 {
		t.Fatalf("off-origin request reached the transport %d times, want 0", requestCount)
	}
}

func TestNewClientValidatesOptionsWithoutDisclosingToken(t *testing.T) {
	t.Parallel()

	baseURL := mustParseURL(t, "https://featbit.example.test/api/v1")
	tests := map[string]struct {
		baseURL *url.URL
		token   string
		options Options
	}{
		"missing URL": {
			token:   syntheticAccessToken,
			options: defaultTestOptions(),
		},
		"invalid token": {
			baseURL: baseURL,
			token:   " " + syntheticAccessToken + " ",
			options: defaultTestOptions(),
		},
		"timeout below minimum": {
			baseURL: baseURL,
			token:   syntheticAccessToken,
			options: Options{HTTPTimeout: MinHTTPTimeout - 1, MaxConcurrency: DefaultMaxConcurrency, MaxRetries: DefaultMaxRetries},
		},
		"concurrency above maximum": {
			baseURL: baseURL,
			token:   syntheticAccessToken,
			options: Options{HTTPTimeout: DefaultHTTPTimeout, MaxConcurrency: MaxConcurrency + 1, MaxRetries: DefaultMaxRetries},
		},
		"retries above maximum": {
			baseURL: baseURL,
			token:   syntheticAccessToken,
			options: Options{HTTPTimeout: DefaultHTTPTimeout, MaxConcurrency: DefaultMaxConcurrency, MaxRetries: MaxRetries + 1},
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := newClient(test.baseURL, test.token, test.options, http.DefaultTransport)
			if err == nil {
				t.Fatal("newClient() accepted invalid configuration")
			}
			if strings.Contains(err.Error(), syntheticAccessToken) {
				t.Fatal("newClient() error disclosed the access token")
			}
		})
	}
}

func TestClientFormattingDoesNotDiscloseToken(t *testing.T) {
	t.Parallel()

	clientUnderTest, err := New(
		mustParseURL(t, "https://featbit.example.test/api/v1"),
		syntheticAccessToken,
		defaultTestOptions(),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	formatted := fmt.Sprintf(
		"%v|%+v|%#v|%s|%+v|%+v|%#v",
		clientUnderTest,
		clientUnderTest,
		clientUnderTest,
		clientUnderTest,
		clientUnderTest.httpClient.Transport,
		clientUnderTest.httpClient,
		clientUnderTest.httpClient,
	)
	if strings.Contains(formatted, syntheticAccessToken) {
		t.Fatal("formatted client or transport disclosed the access token")
	}
}
