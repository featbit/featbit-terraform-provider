// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-log/tfsdklog"
)

const (
	redactionAPITokenMarker  = "api-test-only-token-marker-12345678"
	redactionSecretMarker    = "test-only-secret-marker-041-043"
	redactionEmailMarker     = "test-only-member@example.invalid"
	redactionUUIDMarker      = "00000000-0000-4000-8000-000000000043"
	redactionKeyMarker       = "test-only-feature-key-marker"
	redactionQueryMarker     = "test-only-query-value-marker"
	redactionHeaderMarker    = "test-only-header-value-marker"
	redactionEnvelopeMarker  = "test-only-envelope-message-marker"
	redactionNetworkMarker   = "test-only-network-error-marker"
	redactionTenantMarker    = "featbit:test-only-tenant/member-marker"
	redactionOriginMarker    = "test-only-tenant.example.invalid"
	redactionContractLogLine = "client-redaction-contract-log"
)

var redactionPathMarker = "/api/v1/projects/" + redactionUUIDMarker +
	"/feature-flags/" + redactionKeyMarker + "?filter=" + redactionQueryMarker

func TestClientEnforcesConfiguredMaximumConcurrency(t *testing.T) {
	t.Parallel()

	const (
		maximum = 3
		total   = 9
	)
	var active atomic.Int32
	var maximumSeen atomic.Int32
	started := make(chan struct{}, total)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseAll()

	options := defaultTestOptions()
	options.MaxConcurrency = maximum
	options.MaxRetries = 0
	clientUnderTest := newTestClientWithTransport(t, options, roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			current := active.Add(1)
			defer active.Add(-1)
			updateMaximum(&maximumSeen, current)
			started <- struct{}{}
			select {
			case <-release:
			case <-request.Context().Done():
				return nil, request.Context().Err()
			}
			return syntheticResponse(
				request,
				http.StatusOK,
				io.NopCloser(strings.NewReader("success")),
			), nil
		},
	))

	requests := make([]*http.Request, total)
	for index := range requests {
		requests[index] = mustNewRequest(
			t,
			http.MethodGet,
			"https://featbit.example.test/api/v1/projects",
			nil,
		)
	}
	results := make(chan doResult, total)
	for _, request := range requests {
		request := request
		go func() {
			response, err := clientUnderTest.Do(request)
			results <- doResult{response: response, err: err}
		}()
	}

	for index := 0; index < maximum; index++ {
		waitForSignal(t, started, "configured concurrent requests did not start")
	}
	if active.Load() != maximum {
		t.Fatal("request limiter did not saturate at the configured maximum")
	}
	select {
	case <-started:
		t.Fatal("request limiter admitted more than the configured maximum")
	case <-time.After(75 * time.Millisecond):
	}

	releaseAll()
	for index := 0; index < total; index++ {
		result := waitForDoResult(t, results, "concurrent request did not finish")
		if result.err != nil || result.response == nil {
			t.Fatal("concurrent request returned an unexpected failure")
		}
		mustCloseResponse(t, result.response)
	}
	if maximumSeen.Load() != maximum || active.Load() != 0 {
		t.Fatal("request limiter exceeded its maximum or left a request active")
	}
}

func TestClientCancelsRequestWhileQueuedForPermit(t *testing.T) {
	t.Parallel()

	var transportCalls atomic.Int32
	transportStarted := make(chan struct{}, 3)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseAll()

	options := defaultTestOptions()
	options.MaxConcurrency = 1
	options.MaxRetries = 0
	clientUnderTest := newTestClientWithTransport(t, options, roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			transportCalls.Add(1)
			transportStarted <- struct{}{}
			select {
			case <-release:
			case <-request.Context().Done():
				return nil, request.Context().Err()
			}
			return syntheticResponse(
				request,
				http.StatusOK,
				io.NopCloser(strings.NewReader("success")),
			), nil
		},
	))
	firstRequest := mustNewRequest(
		t,
		http.MethodGet,
		"https://featbit.example.test/api/v1/first",
		nil,
	)
	firstResult := make(chan doResult, 1)
	go func() {
		response, doError := clientUnderTest.Do(firstRequest)
		firstResult <- doResult{response: response, err: doError}
	}()
	waitForSignal(t, transportStarted, "first request did not occupy the only permit")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	queuedRequest := mustNewRequestWithContext(
		t,
		ctx,
		http.MethodGet,
		"https://featbit.example.test/api/v1/queued",
		nil,
	)
	queuedCallStarted := make(chan struct{}, 1)
	queuedResult := make(chan doResult, 1)
	go func() {
		queuedCallStarted <- struct{}{}
		response, doError := clientUnderTest.Do(queuedRequest)
		queuedResult <- doResult{response: response, err: doError}
	}()
	waitForSignal(t, queuedCallStarted, "queued request did not call Client.Do")
	select {
	case <-queuedResult:
		t.Fatal("queued request returned before cancellation or permit release")
	case <-time.After(75 * time.Millisecond):
	}
	cancel()

	queued := waitForDoResult(t, queuedResult, "queued request did not return after cancellation")
	if queued.response != nil {
		t.Fatal("queued cancellation returned an HTTP response")
	}
	requireAPIErrorClassification(t, queued.err, ClassificationCanceled)
	if transportCalls.Load() != 1 {
		t.Fatal("queued canceled request reached the transport")
	}

	releaseAll()
	first := waitForDoResult(t, firstResult, "first request did not finish after permit release")
	if first.err != nil || first.response == nil {
		t.Fatal("first request failed after permit release")
	}
	mustCloseResponse(t, first.response)

	progressRequest := mustNewRequest(
		t,
		http.MethodGet,
		"https://featbit.example.test/api/v1/progress",
		nil,
	)
	progressResponse, err := clientUnderTest.Do(progressRequest)
	if err != nil || progressResponse == nil {
		t.Fatal("limiter did not progress after queued cancellation")
	}
	mustCloseResponse(t, progressResponse)
	if transportCalls.Load() != 2 {
		t.Fatal("queued cancellation changed transport count")
	}
}

func TestClientReleasesPermitBeforeRetryWait(t *testing.T) {
	t.Parallel()

	var retryAttempts atomic.Int32
	var progressAttempts atomic.Int32
	retryWaitStarted := make(chan struct{}, 1)
	releaseRetryWait := make(chan struct{})
	var releaseOnce sync.Once
	releaseWait := func() { releaseOnce.Do(func() { close(releaseRetryWait) }) }
	defer releaseWait()

	options := defaultTestOptions()
	options.MaxConcurrency = 1
	options.MaxRetries = 1
	clientUnderTest := newTestClientWithTransport(t, options, roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			switch request.URL.Path {
			case "/api/v1/retry":
				if retryAttempts.Add(1) == 1 {
					return syntheticResponse(
						request,
						http.StatusTooManyRequests,
						io.NopCloser(strings.NewReader("retry")),
					), nil
				}
			case "/api/v1/progress":
				progressAttempts.Add(1)
			}
			return syntheticResponse(
				request,
				http.StatusOK,
				io.NopCloser(strings.NewReader("success")),
			), nil
		},
	))
	clientUnderTest.retries.wait = func(ctx context.Context, _ time.Duration) error {
		retryWaitStarted <- struct{}{}
		select {
		case <-releaseRetryWait:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	retryRequest := mustNewRequest(
		t,
		http.MethodGet,
		"https://featbit.example.test/api/v1/retry",
		nil,
	)
	retryResult := make(chan doResult, 1)
	go func() {
		response, doError := clientUnderTest.Do(retryRequest)
		retryResult <- doResult{response: response, err: doError}
	}()
	waitForSignal(t, retryWaitStarted, "retrying request did not enter its wait")
	progressRequest := mustNewRequest(
		t,
		http.MethodGet,
		"https://featbit.example.test/api/v1/progress",
		nil,
	)
	progressResult := make(chan doResult, 1)
	go func() {
		response, doError := clientUnderTest.Do(progressRequest)
		progressResult <- doResult{response: response, err: doError}
	}()
	progress := waitForDoResult(t, progressResult, "another request could not progress during retry wait")
	if progress.err != nil || progress.response == nil {
		t.Fatal("another request failed during retry wait")
	}
	mustCloseResponse(t, progress.response)

	releaseWait()
	retried := waitForDoResult(t, retryResult, "retrying request did not finish")
	if retried.err != nil || retried.response == nil {
		t.Fatal("retrying request did not recover")
	}
	mustCloseResponse(t, retried.response)
	if retryAttempts.Load() != 2 || progressAttempts.Load() != 1 {
		t.Fatal("retry wait blocked progress or changed attempt counts")
	}
}

func TestClientLimiterProgressesAfterFailures(t *testing.T) {
	t.Parallel()

	failures := map[string]func(*http.Request) (*http.Response, error){
		"network": func(*http.Request) (*http.Response, error) {
			return nil, errors.New(redactionNetworkMarker)
		},
		"body read": func(request *http.Request) (*http.Response, error) {
			return syntheticResponse(
				request,
				http.StatusOK,
				&failingReadCloser{err: errors.New(unsafeReadErrorMarker)},
			), nil
		},
		"oversized body": func(request *http.Request) (*http.Response, error) {
			return syntheticResponse(
				request,
				http.StatusOK,
				&trackingReadCloser{reader: io.LimitReader(repeatingReader{}, MaxResponseBytes+1)},
			), nil
		},
	}

	for name, failure := range failures {
		name := name
		failure := failure
		t.Run(name, func(t *testing.T) {
			var attempts atomic.Int32
			options := defaultTestOptions()
			options.MaxConcurrency = 1
			options.MaxRetries = 0
			clientUnderTest := newTestClientWithTransport(t, options, roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					if attempts.Add(1) == 1 {
						return failure(request)
					}
					return syntheticResponse(
						request,
						http.StatusOK,
						io.NopCloser(strings.NewReader("success")),
					), nil
				},
			))
			request := mustNewRequest(
				t,
				http.MethodGet,
				"https://featbit.example.test/api/v1/projects",
				nil,
			)

			response, err := clientUnderTest.Do(request)
			if response != nil || err == nil {
				t.Fatal("synthetic failure did not fail its first request")
			}
			response, err = clientUnderTest.Do(request)
			if err != nil || response == nil {
				t.Fatal("limiter did not progress after a failure")
			}
			mustCloseResponse(t, response)
			if attempts.Load() != 2 {
				t.Fatal("failure path changed attempt count")
			}
		})
	}
}

func TestRedactorTextAndHeadersRemoveRuntimeMarkers(t *testing.T) {
	t.Parallel()

	redactor := contractRedactor()
	textMarkers := []string{
		redactionAPITokenMarker,
		redactionSecretMarker,
		redactionEmailMarker,
		redactionUUIDMarker,
		redactionKeyMarker,
		redactionQueryMarker,
		redactionHeaderMarker,
		redactionEnvelopeMarker,
		redactionTenantMarker,
		redactionPathMarker,
	}
	redactedText := redactor.Text(strings.Join(textMarkers, " | "))
	assertNoMarkers(t, redactedText, textMarkers)

	shapeInputMarkers := []string{
		redactionAPITokenMarker,
		redactionEmailMarker,
		redactionUUIDMarker,
		redactionTenantMarker,
		redactionPathMarker,
	}
	shapeMarkers := append([]string(nil), shapeInputMarkers...)
	shapeMarkers = append(shapeMarkers,
		redactionKeyMarker,
		redactionQueryMarker,
	)
	assertNoMarkers(t, RedactText(strings.Join(shapeInputMarkers, " | ")), shapeMarkers)

	headers := contractHeaders()
	redactedHeaders := redactor.Headers(headers)
	assertNoMarkers(t, fmt.Sprintf("%v|%+v|%#v", redactedHeaders, redactedHeaders, redactedHeaders), textMarkers)
	if headers.Get("Authorization") != redactionSecretMarker {
		t.Fatal("Redactor.Headers() mutated the source headers")
	}
}

func TestRedactorRequestRemovesAllUnsafeMetadata(t *testing.T) {
	t.Parallel()

	redactor := contractRedactor()
	request := hostileRequest(t)
	redacted := redactor.Request(request)
	if redacted == nil || redacted.URL == nil {
		t.Fatal("Redactor.Request() returned no request metadata")
	}
	if redacted.URL.Host != "redacted.invalid" || redacted.URL.RawQuery != "" ||
		redacted.URL.Path != "/api/v1/"+redactedPathIdentity ||
		redacted.Host != "redacted.invalid" {
		t.Fatal("Redactor.Request() retained unsafe URL metadata")
	}
	if redacted.Body != nil || redacted.GetBody != nil || redacted.ContentLength != 0 ||
		redacted.Form != nil || redacted.PostForm != nil || redacted.MultipartForm != nil {
		t.Fatal("Redactor.Request() retained request content")
	}
	if redacted.RemoteAddr != "" || len(redacted.TransferEncoding) != 0 ||
		redacted.TLS != nil || redacted.Response != nil {
		t.Fatal("Redactor.Request() retained transport or response metadata")
	}

	markers := allRedactionMarkers()
	output := fmt.Sprintf(
		"%v|%+v|%#v|%s|%v|%v|%v",
		redacted,
		redacted,
		redacted,
		redacted.URL.String(),
		redacted.Header,
		redacted.Trailer,
		redacted.Context(),
	)
	assertNoMarkers(t, output, markers)
	if request.URL.Host != redactionOriginMarker || request.Body == nil {
		t.Fatal("Redactor.Request() mutated the source request")
	}
}

func TestClientErrorsAndResponseMetadataAreRedacted(t *testing.T) {
	t.Parallel()

	baseURL := mustParseURL(t, "https://"+redactionOriginMarker+"/api/v1")
	options := defaultTestOptions()
	options.MaxRetries = 0
	clientUnderTest, err := newClient(baseURL, redactionSecretMarker, options, roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			return syntheticResponse(
				request,
				http.StatusOK,
				io.NopCloser(strings.NewReader("success")),
			), nil
		},
	))
	if err != nil {
		t.Fatal("newClient() could not construct the redaction test client")
	}
	request := hostileRequest(t)
	request.RequestURI = ""
	response, err := clientUnderTest.Do(request)
	if err != nil || response == nil || response.Request == nil {
		t.Fatal("Client.Do() did not return diagnostic request metadata")
	}
	mustCloseResponse(t, response)
	metadataOutput := fmt.Sprintf(
		"%v|%+v|%#v|%s|%v|%v",
		response.Request,
		response.Request,
		response.Request,
		response.Request.URL.String(),
		response.Request.Header,
		response.Request.Trailer,
	)
	assertNoMarkers(t, metadataOutput, allRedactionMarkers())

	detail := strings.Join(allRedactionMarkers(), " | ")
	envelope, marshalError := json.Marshal(map[string]any{
		"success": false,
		"data":    nil,
		"errors":  []string{detail},
	})
	if marshalError != nil {
		t.Fatal("json.Marshal() could not construct the synthetic envelope")
	}
	err = clientUnderTest.DecodeResponse(
		"read_project",
		&http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(string(envelope))),
		},
		nil,
		redactionSecretMarker,
		redactionKeyMarker,
		redactionQueryMarker,
		redactionHeaderMarker,
		redactionEnvelopeMarker,
		redactionNetworkMarker,
		redactionOriginMarker,
	)
	apiError := requireAPIErrorClassification(t, err, ClassificationValidation)
	diagnosticOutput := fmt.Sprintf(
		"%v|%+v|%#v|%s|%v",
		apiError,
		apiError,
		apiError,
		apiError,
		apiError.Details(),
	)
	assertNoMarkers(t, diagnosticOutput, allRedactionMarkers())

	networkClient, err := newClient(baseURL, redactionSecretMarker, options, roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			return nil, errors.New(redactionNetworkMarker + " " + detail)
		},
	))
	if err != nil {
		t.Fatal("newClient() could not construct the network redaction client")
	}
	networkResponse, networkError := networkClient.Do(request)
	if networkResponse != nil {
		t.Fatal("network failure returned an HTTP response")
	}
	networkAPIError := requireAPIErrorClassification(t, networkError, ClassificationNetwork)
	if networkAPIError.Unwrap() != nil {
		t.Fatal("network APIError retained an unsafe raw cause")
	}
	assertNoMarkers(
		t,
		fmt.Sprintf("%v|%+v|%#v|%s", networkAPIError, networkAPIError, networkAPIError, networkAPIError),
		allRedactionMarkers(),
	)

	formatterOutput := fmt.Sprintf(
		"%v|%+v|%#v|%s|%v|%+v|%#v|%v|%+v|%#v",
		clientUnderTest,
		clientUnderTest,
		clientUnderTest,
		clientUnderTest,
		clientUnderTest.redactor,
		clientUnderTest.redactor,
		clientUnderTest.redactor,
		clientUnderTest.httpClient.Transport,
		clientUnderTest.httpClient.Transport,
		clientUnderTest.httpClient,
	)
	assertNoMarkers(t, formatterOutput, allRedactionMarkers())
}

func TestClientRedactedValuesRemainSafeInCapturedLogs(t *testing.T) {
	t.Setenv("TF_LOG", "TRACE")
	t.Setenv("TF_LOG_PATH", "")
	t.Setenv("TF_ACC_LOG_PATH", "")
	t.Setenv("TF_LOG_PATH_MASK", "")

	redactor := contractRedactor()
	request := redactor.Request(hostileRequest(t))
	apiError := newAPIError(
		ClassificationValidation,
		http.StatusBadRequest,
		"read_project",
		[]string{strings.Join(allRedactionMarkers(), " | ")},
		redactor.With(allRedactionMarkers()...),
	)
	networkError := newTransportError(errors.New(
		redactionNetworkMarker + " " + strings.Join(allRedactionMarkers(), " | "),
	))
	formatted := fmt.Sprintf(
		"%v|%+v|%#v|%v|%+v|%#v|%v|%v|%v",
		redactor,
		redactor,
		redactor,
		request,
		request,
		request,
		apiError,
		apiError.Details(),
		networkError,
	)
	assertNoMarkers(t, formatted, allRedactionMarkers())

	logReader, logWriter, err := os.Pipe()
	if err != nil {
		t.Fatal("os.Pipe() could not create the log capture")
	}
	originalStderr := os.Stderr
	os.Stderr = logWriter
	defer func() {
		os.Stderr = originalStderr
		_ = logWriter.Close()
		_ = logReader.Close()
	}()
	ctx := tfsdklog.ContextWithTestLogging(context.Background(), t.Name())
	ctx = tfsdklog.NewRootSDKLogger(ctx)
	ctx = tfsdklog.NewRootProviderLogger(ctx)
	os.Stderr = originalStderr

	type logReadResult struct {
		content []byte
		err     error
	}
	logResult := make(chan logReadResult, 1)
	go func() {
		content, readError := io.ReadAll(logReader)
		logResult <- logReadResult{content: content, err: readError}
	}()
	logRedactor := redactor.With(allRedactionMarkers()...)
	tflog.Trace(ctx, redactionContractLogLine, map[string]interface{}{
		"formatted": formatted,
		"text":      logRedactor.Text(strings.Join(allRedactionMarkers(), " | ")),
		"headers":   fmt.Sprintf("%v", logRedactor.Headers(contractHeaders())),
	})
	if err := logWriter.Close(); err != nil {
		t.Fatal("captured log writer could not be closed")
	}
	var captured logReadResult
	select {
	case captured = <-logResult:
	case <-time.After(2 * time.Second):
		t.Fatal("captured log reader did not finish")
	}
	if captured.err != nil {
		t.Fatal("captured log output could not be read")
	}
	logOutput := string(captured.content)
	if !strings.Contains(logOutput, redactionContractLogLine) {
		t.Fatal("redaction test did not capture the expected log line")
	}
	assertNoMarkers(t, logOutput, allRedactionMarkers())
}

func updateMaximum(maximum *atomic.Int32, current int32) {
	for {
		previous := maximum.Load()
		if current <= previous || maximum.CompareAndSwap(previous, current) {
			return
		}
	}
}

func contractRedactor() *Redactor {
	return NewRedactor(
		redactionSecretMarker,
		redactionHeaderMarker,
		redactionQueryMarker,
		redactionEnvelopeMarker,
	).With(redactionKeyMarker)
}

func contractHeaders() http.Header {
	return http.Header{
		"Authorization":       []string{redactionSecretMarker},
		"Cookie":              []string{redactionSecretMarker},
		"Set-Cookie":          []string{redactionSecretMarker},
		"Proxy-Authorization": []string{redactionSecretMarker},
		"X-Api-Key":           []string{redactionHeaderMarker},
		"X-Member":            []string{redactionEmailMarker},
		"X-Resource":          []string{redactionUUIDMarker},
		"X-Feature-Key":       []string{redactionKeyMarker},
		"X-Query":             []string{redactionQueryMarker},
		"X-Envelope":          []string{redactionEnvelopeMarker},
		"X-Tenant":            []string{redactionTenantMarker},
		"X-Path":              []string{redactionPathMarker},
		"X-Origin":            []string{redactionOriginMarker},
	}
}

func hostileRequest(t *testing.T) *http.Request {
	t.Helper()

	requestURL, err := url.Parse("https://" + redactionOriginMarker + redactionPathMarker)
	if err != nil {
		t.Fatal("url.Parse() could not construct hostile request metadata")
	}
	request := &http.Request{
		Method:           http.MethodGet,
		URL:              requestURL,
		Header:           contractHeaders(),
		Trailer:          contractHeaders(),
		Body:             io.NopCloser(strings.NewReader(redactionSecretMarker)),
		GetBody:          func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(redactionSecretMarker)), nil },
		ContentLength:    int64(len(redactionSecretMarker)),
		TransferEncoding: []string{redactionSecretMarker},
		Host:             redactionOriginMarker,
		Form:             url.Values{"filter": []string{redactionQueryMarker}},
		PostForm:         url.Values{"feature_key": []string{redactionKeyMarker}},
		RemoteAddr:       redactionEmailMarker,
		RequestURI:       redactionPathMarker,
		TLS:              &tls.ConnectionState{ServerName: redactionOriginMarker},
		Pattern:          redactionPathMarker,
		Response:         &http.Response{Status: redactionSecretMarker, Header: contractHeaders()},
	}
	ctx := context.WithValue(context.Background(), struct{}{}, redactionSecretMarker)
	return request.WithContext(ctx)
}

func allRedactionMarkers() []string {
	return []string{
		redactionAPITokenMarker,
		redactionSecretMarker,
		redactionEmailMarker,
		redactionUUIDMarker,
		redactionKeyMarker,
		redactionQueryMarker,
		redactionHeaderMarker,
		redactionEnvelopeMarker,
		redactionNetworkMarker,
		redactionTenantMarker,
		redactionOriginMarker,
		redactionPathMarker,
	}
}

func assertNoMarkers(t *testing.T, output string, markers []string) {
	t.Helper()

	for _, marker := range markers {
		if strings.Contains(output, marker) {
			t.Fatal("redacted output retained an unsafe marker")
		}
	}
}
