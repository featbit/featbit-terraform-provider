// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

const (
	syntheticAccessToken  = "test-only-not-a-credential"
	unsafeReadErrorMarker = "unsafe-body-read-error-marker"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type doResult struct {
	response *http.Response
	err      error
}

type trackingReadCloser struct {
	reader     io.Reader
	closeCalls atomic.Int32
}

func (body *trackingReadCloser) Read(buffer []byte) (int, error) {
	return body.reader.Read(buffer)
}

func (body *trackingReadCloser) Close() error {
	body.closeCalls.Add(1)
	return nil
}

type failingReadCloser struct {
	err        error
	closeCalls atomic.Int32
}

func (body *failingReadCloser) Read([]byte) (int, error) {
	return 0, body.err
}

func (body *failingReadCloser) Close() error {
	body.closeCalls.Add(1)
	return nil
}

type repeatingReader struct{}

func (repeatingReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = 'x'
	}
	return len(buffer), nil
}

func defaultTestOptions() Options {
	return Options{
		HTTPTimeout:     DefaultHTTPTimeout,
		MaxConcurrency:  DefaultMaxConcurrency,
		MaxRetries:      DefaultMaxRetries,
		ProviderVersion: "test",
	}
}

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal("url.Parse() could not parse the test URL")
	}
	return parsed
}

func mustNewRequest(
	t *testing.T,
	method string,
	rawURL string,
	body io.Reader,
) *http.Request {
	t.Helper()
	return mustNewRequestWithContext(t, context.Background(), method, rawURL, body)
}

func mustNewRequestWithContext(
	t *testing.T,
	ctx context.Context,
	method string,
	rawURL string,
	body io.Reader,
) *http.Request {
	t.Helper()

	request, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		t.Fatal("http.NewRequestWithContext() could not construct the test request")
	}
	return request
}

func newTestClientWithTransport(
	t *testing.T,
	options Options,
	transport http.RoundTripper,
) *Client {
	t.Helper()

	clientUnderTest, err := newClient(
		mustParseURL(t, "https://featbit.example.test/api/v1"),
		syntheticAccessToken,
		options,
		transport,
	)
	if err != nil {
		t.Fatal("newClient() could not construct the test client")
	}
	return clientUnderTest
}

func syntheticResponse(
	request *http.Request,
	status int,
	body io.ReadCloser,
) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       body,
		Request:    request,
	}
}

func mustCloseResponse(t *testing.T, response *http.Response) {
	t.Helper()

	if response == nil || response.Body == nil {
		t.Fatal("test response has no body to close")
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal("test response body could not be closed")
	}
}

func requireAPIErrorClassification(
	t *testing.T,
	err error,
	want Classification,
) *APIError {
	t.Helper()

	if err == nil {
		t.Fatal("operation returned no APIError")
	}
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatal("operation did not return an APIError")
	}
	if apiError.Classification() != want {
		t.Fatalf("APIError classification = %q, want %q", apiError.Classification(), want)
	}
	return apiError
}

func waitForSignal(t *testing.T, signal <-chan struct{}, failureMessage string) {
	t.Helper()

	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal(failureMessage)
	}
}

func waitForDoResult(t *testing.T, result <-chan doResult, failureMessage string) doResult {
	t.Helper()

	select {
	case received := <-result:
		return received
	case <-time.After(2 * time.Second):
		t.Fatal(failureMessage)
		return doResult{}
	}
}
