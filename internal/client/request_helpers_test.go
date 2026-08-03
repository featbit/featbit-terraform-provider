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

	const canonical = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	tests := map[string]struct {
		value         string
		wantCanonical string
		valid         bool
	}{
		"canonical lowercase": {
			value:         canonical,
			wantCanonical: canonical,
			valid:         true,
		},
		"canonical uppercase": {
			value:         "AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE",
			wantCanonical: canonical,
			valid:         true,
		},
		"nil UUID": {
			value:         "00000000-0000-0000-0000-000000000000",
			wantCanonical: "00000000-0000-0000-0000-000000000000",
			valid:         true,
		},
		"non hexadecimal": {value: "not-a-uuid"},
		"invalid canonical hex": {
			value: "gaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
		},
		"path suffix": {value: canonical + "/extra"},
		"raw hex rejected": {
			value: "aaaaaaaabbbb4ccc8dddeeeeeeeeeeee",
		},
		"URN rejected": {
			value: "urn:uuid:" + canonical,
		},
		"Microsoft braces rejected": {
			value: "{" + canonical + "}",
		},
		"surrounding whitespace": {value: " " + canonical + " "},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, valid := CanonicalUUID(test.value)
			if valid != test.valid || got != test.wantCanonical {
				t.Fatalf("CanonicalUUID() = %q, %t; want %q, %t", got, valid, test.wantCanonical, test.valid)
			}
			if ValidUUID(test.value) != test.valid {
				t.Fatalf("ValidUUID() validity differed from CanonicalUUID()")
			}
		})
	}

	equalityTests := map[string]struct {
		left  string
		right string
		equal bool
	}{
		"case insensitive canonical identity": {
			left:  canonical,
			right: "AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE",
			equal: true,
		},
		"different canonical identity": {
			left:  canonical,
			right: "ffffffff-ffff-4fff-8fff-ffffffffffff",
		},
		"identical invalid values are not identities": {
			left:  "not-a-uuid",
			right: "not-a-uuid",
		},
		"alternate encoding is not an identity": {
			left:  canonical,
			right: "urn:uuid:" + canonical,
		},
	}
	for name, test := range equalityTests {
		name := name
		test := test
		t.Run("equal/"+name, func(t *testing.T) {
			t.Parallel()
			got := EqualUUID(test.left, test.right)
			if got != test.equal {
				t.Fatalf("EqualUUID() = %t, want %t", got, test.equal)
			}
		})
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
