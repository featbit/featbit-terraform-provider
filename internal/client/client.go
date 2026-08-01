// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

// Package client contains the handwritten FeatBit API client boundary.
package client

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

const (
	// DefaultHTTPTimeout is the provider's default timeout for one HTTP request.
	DefaultHTTPTimeout = 30 * time.Second
	// MinHTTPTimeout and MaxHTTPTimeout bound configured request timeouts.
	MinHTTPTimeout = time.Second
	MaxHTTPTimeout = 5 * time.Minute

	// DefaultMaxConcurrency is the conservative process-local request limit.
	DefaultMaxConcurrency = 4
	// MinConcurrency and MaxConcurrency bound the configured request limit.
	MinConcurrency = 1
	MaxConcurrency = 32

	// DefaultMaxRetries applies only to safe reads once retry behavior is wired.
	DefaultMaxRetries = 3
	// MinRetries and MaxRetries bound safe-read retry configuration.
	MinRetries = 0
	MaxRetries = 10
)

// Options contains non-secret client settings resolved by the provider.
type Options struct {
	HTTPTimeout    time.Duration
	MaxConcurrency int
	MaxRetries     int
}

// Client is the handwritten API client boundary used by provider resources.
// The access token is deliberately kept in an unexported transport field.
type Client struct {
	baseURL        url.URL
	httpClient     http.Client
	maxConcurrency int
	maxRetries     int
}

// Format prevents accidental structured formatting of the client from
// traversing into the credential-bearing transport.
func (Client) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.Client{redacted}")
}

// New constructs a client without performing a login or any network request.
// The access token is sent directly in Authorization for requests to the
// configured FeatBit API origin.
func New(baseURL *url.URL, accessToken string, options Options) (*Client, error) {
	return newClient(baseURL, accessToken, options, http.DefaultTransport)
}

func newClient(
	baseURL *url.URL,
	accessToken string,
	options Options,
	transport http.RoundTripper,
) (*Client, error) {
	if !validBaseURL(baseURL) {
		return nil, errors.New("FeatBit API base URL is invalid")
	}
	if !validAccessToken(accessToken) {
		return nil, errors.New("FeatBit API access token is invalid")
	}
	if options.HTTPTimeout < MinHTTPTimeout || options.HTTPTimeout > MaxHTTPTimeout {
		return nil, errors.New("FeatBit HTTP timeout is outside the supported range")
	}
	if options.MaxConcurrency < MinConcurrency || options.MaxConcurrency > MaxConcurrency {
		return nil, errors.New("FeatBit maximum concurrency is outside the supported range")
	}
	if options.MaxRetries < MinRetries || options.MaxRetries > MaxRetries {
		return nil, errors.New("FeatBit maximum retries is outside the supported range")
	}
	if transport == nil {
		transport = http.DefaultTransport
	}

	baseURLCopy := *baseURL

	return &Client{
		baseURL: baseURLCopy,
		httpClient: http.Client{
			Transport: &authorizationTransport{
				base:        transport,
				apiScheme:   baseURLCopy.Scheme,
				apiHost:     baseURLCopy.Host,
				accessToken: accessToken,
			},
			Timeout: options.HTTPTimeout,
		},
		maxConcurrency: options.MaxConcurrency,
		maxRetries:     options.MaxRetries,
	}, nil
}

// Do sends one request through the credential-injecting HTTP client. Retry,
// concurrency, and generated-transport orchestration are added by later Phase
// 1 tasks; this method currently performs exactly one HTTP client operation.
func (c *Client) Do(request *http.Request) (*http.Response, error) {
	return c.httpClient.Do(request)
}

type authorizationTransport struct {
	base        http.RoundTripper
	apiScheme   string
	apiHost     string
	accessToken string
}

// Format prevents accidental formatting of the transport from exposing the
// Authorization credential. HTTP request/response redaction is added later.
func (authorizationTransport) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.authorizationTransport{redacted}")
}

func (t *authorizationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, errors.New("FeatBit API request URL is missing")
	}
	if !strings.EqualFold(request.URL.Scheme, t.apiScheme) ||
		!strings.EqualFold(request.URL.Host, t.apiHost) {
		return nil, errors.New("request URL does not match the configured FeatBit API origin")
	}

	requestCopy := request.Clone(request.Context())
	requestCopy.Header = request.Header.Clone()
	requestCopy.Header.Set("Authorization", t.accessToken)

	return t.base.RoundTrip(requestCopy)
}

func validBaseURL(baseURL *url.URL) bool {
	if baseURL == nil || baseURL.Opaque != "" || baseURL.Hostname() == "" {
		return false
	}
	if !strings.EqualFold(baseURL.Scheme, "http") && !strings.EqualFold(baseURL.Scheme, "https") {
		return false
	}
	return baseURL.User == nil && baseURL.Path == "/api/v1" && baseURL.RawQuery == "" && baseURL.Fragment == ""
}

func validAccessToken(accessToken string) bool {
	if accessToken == "" || strings.TrimSpace(accessToken) != accessToken {
		return false
	}

	return strings.IndexFunc(accessToken, func(r rune) bool {
		return unicode.IsControl(r)
	}) == -1
}
