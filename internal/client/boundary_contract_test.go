// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientCancellationBeforeConcurrencyAdmission(t *testing.T) {
	t.Parallel()

	var transportCalls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			transportCalls.Add(1)
			return nil, errors.New("unexpected transport call")
		},
	))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := mustNewRequestWithContext(
		t,
		ctx,
		http.MethodGet,
		"https://featbit.example.test/api/v1/projects",
		nil,
	)

	response, err := clientUnderTest.Do(request)
	if response != nil {
		t.Fatal("Client.Do() returned a response for a pre-canceled request")
	}
	requireAPIErrorClassification(t, err, ClassificationCanceled)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("pre-admission cancellation did not preserve context.Canceled")
	}
	if transportCalls.Load() != 0 {
		t.Fatal("pre-admission cancellation reached transport")
	}
}

func TestClientCancellationDuringHTTPExecution(t *testing.T) {
	t.Parallel()

	transportStarted := make(chan struct{}, 1)
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			transportStarted <- struct{}{}
			<-request.Context().Done()
			return nil, request.Context().Err()
		},
	))
	clientUnderTest.maxRetries = 0
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
	waitForSignal(t, transportStarted, "transport did not start")
	cancel()

	doResult := waitForDoResult(t, result, "Client.Do() did not return after cancellation")
	if doResult.response != nil {
		t.Fatal("Client.Do() returned a response after HTTP cancellation")
	}
	requireAPIErrorClassification(t, doResult.err, ClassificationCanceled)
	if !errors.Is(doResult.err, context.Canceled) {
		t.Fatal("HTTP cancellation did not preserve context.Canceled")
	}
}

func TestClientCancellationDuringRetryWait(t *testing.T) {
	t.Parallel()

	retryWaitStarted := make(chan struct{}, 1)
	responseBody := &trackingReadCloser{reader: strings.NewReader("retry")}
	options := defaultTestOptions()
	options.MaxRetries = 1
	clientUnderTest := newTestClientWithTransport(t, options, roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			return syntheticResponse(request, http.StatusTooManyRequests, responseBody), nil
		},
	))
	clientUnderTest.retries.wait = func(ctx context.Context, _ time.Duration) error {
		retryWaitStarted <- struct{}{}
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
	waitForSignal(t, retryWaitStarted, "Client.Do() did not enter the retry wait")
	if responseBody.closeCalls.Load() != 1 {
		t.Fatal("retry wait began before closing the response")
	}
	cancel()

	doResult := waitForDoResult(t, result, "Client.Do() did not return after retry-wait cancellation")
	if doResult.response != nil {
		t.Fatal("Client.Do() returned a response after retry-wait cancellation")
	}
	requireAPIErrorClassification(t, doResult.err, ClassificationCanceled)
	if !errors.Is(doResult.err, context.Canceled) {
		t.Fatal("retry-wait cancellation did not preserve context.Canceled")
	}
}

func TestClientTimeoutDuringHTTPExecution(t *testing.T) {
	t.Parallel()

	var transportCalls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			transportCalls.Add(1)
			<-request.Context().Done()
			return nil, request.Context().Err()
		},
	))
	clientUnderTest.httpClient.Timeout = 25 * time.Millisecond
	clientUnderTest.maxRetries = 0
	request := mustNewRequest(
		t,
		http.MethodGet,
		"https://featbit.example.test/api/v1/projects",
		nil,
	)

	response, err := clientUnderTest.Do(request)
	if response != nil {
		t.Fatal("Client.Do() returned a response after the client timeout")
	}
	requireAPIErrorClassification(t, err, ClassificationTimeout)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("client timeout did not preserve context.DeadlineExceeded")
	}
	if transportCalls.Load() != 1 {
		t.Fatal("client timeout unexpectedly retried")
	}
}

func TestClientNilRequestAndResponseContracts(t *testing.T) {
	t.Parallel()

	clientUnderTest := newTestClientWithTransport(
		t,
		defaultTestOptions(),
		roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, nil }),
	)
	clientUnderTest.maxRetries = 0

	t.Run("nil request", func(t *testing.T) {
		response, err := clientUnderTest.Do(nil)
		if response != nil {
			t.Fatal("Client.Do(nil) returned a response")
		}
		requireAPIErrorClassification(t, err, ClassificationAmbiguous)
	})

	t.Run("nil request URL", func(t *testing.T) {
		response, err := clientUnderTest.Do(&http.Request{})
		if response != nil {
			t.Fatal("Client.Do() returned a response for a nil request URL")
		}
		requireAPIErrorClassification(t, err, ClassificationAmbiguous)
	})

	t.Run("nil transport response", func(t *testing.T) {
		request := mustNewRequest(
			t,
			http.MethodGet,
			"https://featbit.example.test/api/v1/projects",
			nil,
		)
		response, err := clientUnderTest.Do(request)
		if response != nil {
			t.Fatal("Client.Do() returned a nil transport response as valid")
		}
		requireAPIErrorClassification(t, err, ClassificationNetwork)
	})

	t.Run("nil decode response", func(t *testing.T) {
		err := clientUnderTest.DecodeResponse("read_project", nil, nil)
		apiError := requireAPIErrorClassification(t, err, ClassificationAmbiguous)
		if apiError.StatusCode() != 0 {
			t.Fatal("nil DecodeResponse input returned a nonzero status")
		}
	})

	t.Run("nil response body", func(t *testing.T) {
		response := &http.Response{StatusCode: http.StatusOK}
		err := clientUnderTest.DecodeResponse("read_project", response, nil)
		requireAPIErrorClassification(t, err, ClassificationAmbiguous)
	})

	t.Run("nil bounded body", func(t *testing.T) {
		body, err := readBoundedResponse(nil)
		if err != nil || body != nil {
			t.Fatal("readBoundedResponse(nil) did not return an empty result")
		}
	})
}

func TestClientResponseSizeBoundaryAndClosure(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		size      int64
		wantError bool
	}{
		"exactly maximum": {size: MaxResponseBytes},
		"one byte oversized": {
			size:      MaxResponseBytes + 1,
			wantError: true,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			body := &trackingReadCloser{reader: io.LimitReader(repeatingReader{}, test.size)}
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					return syntheticResponse(request, http.StatusOK, body), nil
				},
			))
			clientUnderTest.maxRetries = 0
			request := mustNewRequest(
				t,
				http.MethodGet,
				"https://featbit.example.test/api/v1/projects",
				nil,
			)

			response, err := clientUnderTest.Do(request)
			if test.wantError {
				if response != nil {
					t.Fatal("oversized response returned a usable response")
				}
				requireAPIErrorClassification(t, err, ClassificationAmbiguous)
			} else {
				if err != nil || response == nil {
					t.Fatal("exactly bounded response was rejected")
				}
				if response.ContentLength != MaxResponseBytes {
					t.Fatal("exactly bounded response reported the wrong content length")
				}
				mustCloseResponse(t, response)
			}
			if body.closeCalls.Load() != 1 {
				t.Fatal("response boundary path did not close the source body")
			}
		})
	}
}

func TestClientBodyReadFailureIsClosedSafeAndRecoverable(t *testing.T) {
	t.Parallel()

	failingBody := &failingReadCloser{err: errors.New(unsafeReadErrorMarker)}
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			if calls.Add(1) == 1 {
				return syntheticResponse(request, http.StatusOK, failingBody), nil
			}
			return syntheticResponse(
				request,
				http.StatusOK,
				io.NopCloser(strings.NewReader("success")),
			), nil
		},
	))
	clientUnderTest.maxRetries = 0
	request := mustNewRequest(
		t,
		http.MethodGet,
		"https://featbit.example.test/api/v1/projects",
		nil,
	)

	response, err := clientUnderTest.Do(request)
	if response != nil {
		t.Fatal("body read failure returned a usable response")
	}
	requireAPIErrorClassification(t, err, ClassificationNetwork)
	if strings.Contains(fmt.Sprintf("%v|%+v|%#v", err, err, err), unsafeReadErrorMarker) {
		t.Fatal("body read failure disclosed the unsafe transport detail")
	}
	if failingBody.closeCalls.Load() != 1 {
		t.Fatal("body read failure did not close its body")
	}

	response, err = clientUnderTest.Do(request)
	if err != nil || response == nil {
		t.Fatal("request limiter did not recover after a body read failure")
	}
	mustCloseResponse(t, response)
}

func TestDecodeResponseClosesBodyOnEveryPath(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		status    int
		reader    io.Reader
		wantError bool
	}{
		"success": {
			status: http.StatusOK,
			reader: strings.NewReader(`{"success":true,"data":{},"errors":[]}`),
		},
		"classified error": {
			status:    http.StatusConflict,
			reader:    strings.NewReader(`{"success":false,"data":null,"errors":[]}`),
			wantError: true,
		},
		"malformed envelope": {
			status:    http.StatusOK,
			reader:    strings.NewReader(`{"success":`),
			wantError: true,
		},
	}

	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), http.DefaultTransport)
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			body := &trackingReadCloser{reader: test.reader}
			err := clientUnderTest.DecodeResponse(
				"read_project",
				&http.Response{StatusCode: test.status, Body: body},
				nil,
			)
			if (err != nil) != test.wantError {
				t.Fatal("DecodeResponse() returned an unexpected success/error result")
			}
			if body.closeCalls.Load() != 1 {
				t.Fatal("DecodeResponse() did not close the response body")
			}
		})
	}

	t.Run("read failure", func(t *testing.T) {
		body := &failingReadCloser{err: errors.New(unsafeReadErrorMarker)}
		err := clientUnderTest.DecodeResponse(
			"read_project",
			&http.Response{StatusCode: http.StatusOK, Body: body},
			nil,
		)
		requireAPIErrorClassification(t, err, ClassificationNetwork)
		if body.closeCalls.Load() != 1 {
			t.Fatal("DecodeResponse() did not close a failing response body")
		}
		if strings.Contains(err.Error(), unsafeReadErrorMarker) {
			t.Fatal("DecodeResponse() disclosed a body read failure")
		}
	})
}
