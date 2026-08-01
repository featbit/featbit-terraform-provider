// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

// Package client contains the handwritten FeatBit API client boundary.
package client

import (
	"bytes"
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

	// DefaultMaxRetries applies only to safe GET requests.
	DefaultMaxRetries = 3
	// MinRetries and MaxRetries bound safe-read retry configuration.
	MinRetries = 0
	MaxRetries = 10

	// MaxResponseBytes bounds every buffered FeatBit JSON response. Buffering
	// lets the client retry safe reads whose response body fails mid-stream.
	MaxResponseBytes int64 = 16 << 20

	userAgentProduct = "terraform-provider-featbit"
)

// Options contains non-secret client settings resolved by the provider.
type Options struct {
	HTTPTimeout     time.Duration
	MaxConcurrency  int
	MaxRetries      int
	ProviderVersion string
}

// Client is the handwritten API client boundary used by provider resources.
// Credentials and transport details are deliberately unexported.
type Client struct {
	baseURL    url.URL
	httpClient http.Client
	maxRetries int
	limiter    *requestLimiter
	retries    retryController
	redactor   *Redactor
}

// Format prevents accidental structured formatting of the client from
// traversing into credential-bearing or runtime-identity fields.
func (Client) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.Client{redacted}")
}

// New constructs a client without performing a login or any network request.
// The access token is sent directly in Authorization for requests to the
// configured FeatBit API origin and /api/v1 path.
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
	redactor := NewRedactor(accessToken)
	client := &Client{
		baseURL: baseURLCopy,
		httpClient: http.Client{
			Transport: &authorizationTransport{
				base:      transport,
				apiScheme: baseURLCopy.Scheme,
				apiHost:   baseURLCopy.Host,
				apiPath:   baseURLCopy.EscapedPath(),
				token:     accessToken,
				userAgent: makeUserAgent(options.ProviderVersion),
			},
			Timeout: options.HTTPTimeout,
		},
		maxRetries: options.MaxRetries,
		limiter:    newRequestLimiter(options.MaxConcurrency),
		retries:    newRetryController(),
		redactor:   redactor,
	}
	return client, nil
}

// Do executes one request created by a handwritten endpoint adapter. It
// buffers a bounded response, limits concurrent in-flight requests, and
// retries only bodyless GET requests classified as rate limited, transient
// server, timeout, or network failures. Mutations always have one attempt.
func (c *Client) Do(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, newAPIError(ClassificationAmbiguous, 0, "request", nil, nil)
	}
	if err := request.Context().Err(); err != nil {
		return nil, newTransportError(err)
	}

	retryableRead := request.Method == http.MethodGet &&
		(request.Body == nil || request.Body == http.NoBody)
	maxAttempts := 1
	if retryableRead {
		maxAttempts += c.maxRetries
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		response, err := c.doAttempt(request)
		classification := Classify(statusCode(response), nil, err)
		if err == nil && !ShouldRetry(request.Method, classification) {
			return response, nil
		}
		if !retryableRead || !ShouldRetry(request.Method, classification) || attempt+1 >= maxAttempts {
			if err != nil {
				return nil, newTransportError(err)
			}
			return response, nil
		}

		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		delay := c.retries.delay(response, attempt)
		if err := c.retries.wait(request.Context(), delay); err != nil {
			return nil, newTransportError(err)
		}
	}

	return nil, newAPIError(ClassificationAmbiguous, 0, "request", nil, nil)
}

func (c *Client) doAttempt(request *http.Request) (*http.Response, error) {
	if err := c.limiter.acquire(request.Context()); err != nil {
		return nil, err
	}
	defer c.limiter.release()

	requestCopy := request.Clone(request.Context())
	response, err := c.httpClient.Do(requestCopy)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, err
	}
	if response == nil {
		return nil, errors.New("FeatBit API transport returned no response")
	}

	body, err := readBoundedResponse(response.Body)
	if err != nil {
		return nil, err
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	response.Request = c.redactor.Request(response.Request)
	return response, nil
}

func readBoundedResponse(body io.ReadCloser) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	defer body.Close()

	content, err := io.ReadAll(io.LimitReader(body, MaxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > MaxResponseBytes {
		return nil, errResponseTooLarge
	}
	return content, nil
}

func statusCode(response *http.Response) int {
	if response == nil {
		return 0
	}
	return response.StatusCode
}

func validBaseURL(baseURL *url.URL) bool {
	if baseURL == nil || baseURL.Opaque != "" || baseURL.Hostname() == "" {
		return false
	}
	if !strings.EqualFold(baseURL.Scheme, "http") && !strings.EqualFold(baseURL.Scheme, "https") {
		return false
	}
	return baseURL.User == nil && baseURL.Path == "/api/v1" && baseURL.RawPath == "" &&
		baseURL.RawQuery == "" && baseURL.Fragment == ""
}

func validAccessToken(accessToken string) bool {
	if accessToken == "" || strings.TrimSpace(accessToken) != accessToken {
		return false
	}

	return strings.IndexFunc(accessToken, func(r rune) bool {
		return unicode.IsControl(r)
	}) == -1
}
