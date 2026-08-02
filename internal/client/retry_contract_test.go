// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type syntheticTimeoutError struct{}

func (syntheticTimeoutError) Error() string {
	return "unsafe-timeout-error-marker"
}

func (syntheticTimeoutError) Timeout() bool {
	return true
}

type retryFailure struct {
	status int
	err    error
}

func TestClientRetriesBodylessGETForRetryableFailures(t *testing.T) {
	t.Parallel()

	tests := map[string]retryFailure{
		"429":     {status: http.StatusTooManyRequests},
		"5xx":     {status: http.StatusServiceUnavailable},
		"timeout": {err: syntheticTimeoutError{}},
		"network": {err: errors.New("unsafe-network-error-marker")},
	}

	for name, failure := range tests {
		name := name
		failure := failure
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var attempts atomic.Int32
			var waits atomic.Int32
			var firstBody *trackingReadCloser
			options := defaultTestOptions()
			options.MaxRetries = 1
			clientUnderTest := newTestClientWithTransport(t, options, roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					if attempts.Add(1) == 1 {
						if failure.err != nil {
							return nil, failure.err
						}
						firstBody = &trackingReadCloser{reader: strings.NewReader("retry")}
						return syntheticResponse(request, failure.status, firstBody), nil
					}
					return syntheticResponse(
						request,
						http.StatusOK,
						io.NopCloser(strings.NewReader("success")),
					), nil
				},
			))
			clientUnderTest.retries.wait = func(context.Context, time.Duration) error {
				waits.Add(1)
				return nil
			}
			request := mustNewRequest(
				t,
				http.MethodGet,
				"https://featbit.example.test/api/v1/projects",
				nil,
			)

			response, err := clientUnderTest.Do(request)
			if err != nil || response == nil || response.StatusCode != http.StatusOK {
				t.Fatal("bodyless GET did not recover from a retryable failure")
			}
			mustCloseResponse(t, response)
			if attempts.Load() != 2 || waits.Load() != 1 {
				t.Fatal("bodyless GET used the wrong retry or wait count")
			}
			if firstBody != nil && firstBody.closeCalls.Load() != 1 {
				t.Fatal("retryable HTTP response body was not closed")
			}
		})
	}
}

func TestClientNeverRetriesMutationsOrGETWithBody(t *testing.T) {
	t.Parallel()

	requests := map[string]struct {
		method string
		body   string
	}{
		"POST":          {method: http.MethodPost},
		"PUT":           {method: http.MethodPut},
		"PATCH":         {method: http.MethodPatch},
		"DELETE":        {method: http.MethodDelete},
		"GET with body": {method: http.MethodGet, body: "synthetic request body"},
	}
	failures := map[string]retryFailure{
		"429":     {status: http.StatusTooManyRequests},
		"5xx":     {status: http.StatusInternalServerError},
		"timeout": {err: syntheticTimeoutError{}},
		"network": {err: errors.New("unsafe-network-error-marker")},
	}

	for requestName, requestContract := range requests {
		requestName := requestName
		requestContract := requestContract
		for failureName, failure := range failures {
			failureName := failureName
			failure := failure
			t.Run(requestName+"/"+failureName, func(t *testing.T) {
				t.Parallel()

				var attempts atomic.Int32
				var waits atomic.Int32
				options := defaultTestOptions()
				options.MaxRetries = MaxRetries
				clientUnderTest := newTestClientWithTransport(t, options, roundTripFunc(
					func(request *http.Request) (*http.Response, error) {
						attempts.Add(1)
						if failure.err != nil {
							return nil, failure.err
						}
						return syntheticResponse(
							request,
							failure.status,
							io.NopCloser(strings.NewReader("failure")),
						), nil
					},
				))
				clientUnderTest.retries.wait = func(context.Context, time.Duration) error {
					waits.Add(1)
					return nil
				}

				var body io.Reader
				if requestContract.body != "" {
					body = strings.NewReader(requestContract.body)
				}
				request := mustNewRequest(
					t,
					requestContract.method,
					"https://featbit.example.test/api/v1/projects",
					body,
				)

				response, err := clientUnderTest.Do(request)
				if response != nil {
					mustCloseResponse(t, response)
				} else if err == nil {
					t.Fatal("single-attempt transport failure returned neither response nor error")
				}
				if attempts.Load() != 1 || waits.Load() != 0 {
					t.Fatal("unsafe request was retried or entered a retry wait")
				}
			})
		}
	}
}

func TestShouldRetryRejectsEveryClassificationForMutations(t *testing.T) {
	t.Parallel()

	classifications := []Classification{
		ClassificationSuccess,
		ClassificationValidation,
		ClassificationAuthentication,
		ClassificationAuthorization,
		ClassificationNotFoundUnconfirmed,
		ClassificationConflict,
		ClassificationRateLimited,
		ClassificationTransientServer,
		ClassificationApplicationFailure,
		ClassificationTimeout,
		ClassificationCanceled,
		ClassificationNetwork,
		ClassificationAmbiguous,
	}
	for _, method := range []string{
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	} {
		for _, classification := range classifications {
			if ShouldRetry(method, classification) {
				t.Fatalf("ShouldRetry(%q, %q) allowed an unsafe mutation", method, classification)
			}
		}
	}
}

func TestClientHonorsConfiguredRetryCount(t *testing.T) {
	t.Parallel()

	const retryCount = 3
	var attempts atomic.Int32
	var waits atomic.Int32
	bodies := make([]*trackingReadCloser, 0, retryCount+1)
	options := defaultTestOptions()
	options.MaxRetries = retryCount
	clientUnderTest := newTestClientWithTransport(t, options, roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			attempts.Add(1)
			body := &trackingReadCloser{reader: strings.NewReader("retry")}
			bodies = append(bodies, body)
			return syntheticResponse(request, http.StatusTooManyRequests, body), nil
		},
	))
	clientUnderTest.retries.wait = func(context.Context, time.Duration) error {
		waits.Add(1)
		return nil
	}
	request := mustNewRequest(
		t,
		http.MethodGet,
		"https://featbit.example.test/api/v1/projects",
		nil,
	)

	response, err := clientUnderTest.Do(request)
	if err != nil || response == nil || response.StatusCode != http.StatusTooManyRequests {
		t.Fatal("exhausted retries did not return the final HTTP response")
	}
	mustCloseResponse(t, response)
	if attempts.Load() != retryCount+1 || waits.Load() != retryCount {
		t.Fatal("configured retry count produced the wrong attempt or wait count")
	}
	for _, body := range bodies {
		if body.closeCalls.Load() != 1 {
			t.Fatal("a retry attempt did not close its source response body")
		}
	}
}

func TestRetryControllerExponentialBackoffAndJitterBounds(t *testing.T) {
	t.Parallel()

	expectedBases := []time.Duration{
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1600 * time.Millisecond,
		3200 * time.Millisecond,
		maximumRetryDelay,
		maximumRetryDelay,
	}
	for retryIndex, expectedBase := range expectedBases {
		var jitterInput time.Duration
		controller := retryController{
			now: time.Now,
			jitter: func(delay time.Duration) time.Duration {
				jitterInput = delay
				return delay
			},
		}
		if delay := controller.delay(nil, retryIndex); delay != expectedBase || jitterInput != expectedBase {
			t.Fatalf("retry index %d did not use the expected exponential base", retryIndex)
		}
	}

	controller := newRetryController()
	for retryIndex, base := range expectedBases {
		for sample := 0; sample < 100; sample++ {
			delay := controller.delay(nil, retryIndex)
			if delay < base/2 || delay > maximumRetryDelay {
				t.Fatalf("retry index %d produced jitter outside the supported bounds", retryIndex)
			}
			if base < maximumRetryDelay && delay >= base+base/2 {
				t.Fatalf("retry index %d produced jitter outside the [0.5, 1.5) range", retryIndex)
			}
		}
	}

	negative := retryController{jitter: func(time.Duration) time.Duration { return -time.Second }}
	if delay := negative.delay(nil, 0); delay != 0 {
		t.Fatal("negative jitter was not clamped to zero")
	}
	oversized := retryController{jitter: func(time.Duration) time.Duration { return time.Hour }}
	if delay := oversized.delay(nil, 0); delay != maximumRetryDelay {
		t.Fatal("oversized jitter was not capped")
	}
}

func TestParseRetryAfterContract(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		value     string
		wantDelay time.Duration
		wantValid bool
	}{
		"zero seconds":           {value: "0", wantValid: true},
		"delta seconds":          {value: "3", wantDelay: 3 * time.Second, wantValid: true},
		"surrounding space":      {value: " 2 ", wantDelay: 2 * time.Second, wantValid: true},
		"future HTTP date":       {value: now.Add(7 * time.Second).Format(http.TimeFormat), wantDelay: 7 * time.Second, wantValid: true},
		"current HTTP date":      {value: now.Format(http.TimeFormat), wantValid: true},
		"empty":                  {},
		"negative":               {value: "-1"},
		"past HTTP date":         {value: now.Add(-time.Second).Format(http.TimeFormat)},
		"malformed":              {value: "not-a-retry-after"},
		"duration overflow":      {value: "9223372037"},
		"integer parse overflow": {value: "999999999999999999999999999999"},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			delay, valid := ParseRetryAfter(test.value, now)
			if valid != test.wantValid || delay != test.wantDelay {
				t.Fatal("ParseRetryAfter() returned an unexpected validity or delay")
			}
		})
	}
}

func TestRetryControllerUsesRetryAfterOrBackoff(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		header          string
		wantDelay       time.Duration
		wantJitterCalls int32
	}{
		"delta seconds": {
			header:    "4",
			wantDelay: 4 * time.Second,
		},
		"HTTP date": {
			header:    now.Add(3 * time.Second).Format(http.TimeFormat),
			wantDelay: 3 * time.Second,
		},
		"invalid fallback": {
			header:          "invalid",
			wantDelay:       125 * time.Millisecond,
			wantJitterCalls: 1,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			var jitterCalls atomic.Int32
			controller := retryController{
				now: func() time.Time { return now },
				jitter: func(time.Duration) time.Duration {
					jitterCalls.Add(1)
					return 125 * time.Millisecond
				},
			}
			response := &http.Response{Header: http.Header{"Retry-After": []string{test.header}}}
			if delay := controller.delay(response, 0); delay != test.wantDelay {
				t.Fatal("retryController.delay() returned an unexpected delay")
			}
			if jitterCalls.Load() != test.wantJitterCalls {
				t.Fatal("retryController.delay() used jitter unexpectedly")
			}
		})
	}
}

func TestClientCancellationStopsRetryAttempts(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	var waits atomic.Int32
	waitStarted := make(chan struct{}, 1)
	options := defaultTestOptions()
	options.MaxRetries = MaxRetries
	clientUnderTest := newTestClientWithTransport(t, options, roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			attempts.Add(1)
			return syntheticResponse(
				request,
				http.StatusTooManyRequests,
				io.NopCloser(strings.NewReader("retry")),
			), nil
		},
	))
	clientUnderTest.retries.wait = func(ctx context.Context, _ time.Duration) error {
		waits.Add(1)
		waitStarted <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	request := mustNewRequestWithContext(
		t,
		ctx,
		http.MethodGet,
		"https://featbit.example.test/api/v1/projects",
		nil,
	)

	result := make(chan doResult, 1)
	go func() {
		response, doError := clientUnderTest.Do(request)
		result <- doResult{response: response, err: doError}
	}()
	waitForSignal(t, waitStarted, "Client.Do() did not enter the retry wait")
	cancel()
	doResult := waitForDoResult(t, result, "Client.Do() did not stop after retry cancellation")
	if doResult.response != nil {
		t.Fatal("retry cancellation returned an HTTP response")
	}
	requireAPIErrorClassification(t, doResult.err, ClassificationCanceled)
	if attempts.Load() != 1 || waits.Load() != 1 {
		t.Fatal("retry cancellation allowed another attempt or wait")
	}
}
