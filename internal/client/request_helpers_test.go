// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"io"
	"net/http"
	"testing"
)

func TestUUIDHelpers(t *testing.T) {
	t.Parallel()

	const lower = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	const upper = "AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE"
	if !ValidUUID(lower) || !ValidUUID(upper) {
		t.Fatal("ValidUUID() rejected valid hexadecimal UUID syntax")
	}
	if ValidUUID("not-a-uuid") || ValidUUID(lower+"/extra") {
		t.Fatal("ValidUUID() accepted an invalid UUID")
	}
	if !EqualUUID(lower, upper) || EqualUUID(lower, "ffffffff-ffff-4fff-8fff-ffffffffffff") {
		t.Fatal("EqualUUID() returned an incorrect identity comparison")
	}
}

func TestNewJSONRequestEscapesEachPathSegment(t *testing.T) {
	t.Parallel()

	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			t.Fatal("request helper unexpectedly executed the transport")
			return nil, nil
		},
	))

	request, err := clientUnderTest.newJSONRequest(
		context.Background(),
		http.MethodPost,
		[]string{"projects", "key/with space"},
		struct {
			Name string `json:"name"`
		}{Name: "Project"},
	)
	if err != nil {
		t.Fatalf("newJSONRequest() error = %v", err)
	}
	if got := request.URL.EscapedPath(); got != "/api/v1/projects/key%2Fwith%20space" {
		t.Fatalf("escaped path = %q", got)
	}
	if request.URL.RawQuery != "" {
		t.Fatalf("query = %q", request.URL.RawQuery)
	}
	if got := request.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("read JSON request body: %v", err)
	}
	if got := string(body); got != `{"name":"Project"}` {
		t.Fatalf("JSON request body = %s", got)
	}
}
