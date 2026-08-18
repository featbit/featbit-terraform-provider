// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

const clientMemberIDTwo = "55555555-5555-4555-8555-555555555555"

func TestListMembersConsumesEveryPageUsingSafeAllowlist(t *testing.T) {
	t.Parallel()

	members := make([]Member, 0, memberPageSize+1)
	for index := 0; index < memberPageSize+1; index++ {
		members = append(members, Member{
			ID:    fmt.Sprintf("00000000-0000-4000-8000-%012x", index+1),
			Email: fmt.Sprintf("member-%d@example.test", index),
			Name:  fmt.Sprintf("Member %d", index),
		})
	}

	const initialPasswordMarker = "must-never-enter-the-member-model"
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			if request.Method != http.MethodGet || request.URL.EscapedPath() != "/api/v1/members" ||
				request.URL.Query().Get("PageSize") != "100" ||
				request.URL.Query().Get("SearchText") != "" {
				t.Fatalf("request = %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
			}
			var page []Member
			switch request.URL.Query().Get("PageIndex") {
			case "0":
				page = members[:memberPageSize]
			case "1":
				page = members[memberPageSize:]
			default:
				t.Fatalf("unexpected PageIndex = %q", request.URL.Query().Get("PageIndex"))
			}
			items := make([]map[string]any, 0, len(page))
			for _, member := range page {
				items = append(items, map[string]any{
					"id":              strings.ToUpper(member.ID),
					"email":           member.Email,
					"name":            member.Name,
					"invitorId":       "ignored-invitor",
					"initialPassword": initialPasswordMarker,
					"groups":          []map[string]any{{"id": "ignored-group"}},
				})
			}
			return iamTestResponse(request, http.StatusOK, map[string]any{
				"totalCount": len(members),
				"items":      items,
			}), nil
		},
	))

	got, err := clientUnderTest.ListMembers(context.Background())
	if err != nil {
		t.Fatalf("ListMembers() error = %v", err)
	}
	if len(got) != len(members) || calls.Load() != 2 {
		t.Fatalf("ListMembers() count/calls = %d/%d, want %d/2", len(got), calls.Load(), len(members))
	}
	if !EqualUUID(got[0].ID, members[0].ID) || got[0].Email != members[0].Email ||
		got[0].Name != members[0].Name {
		t.Fatalf("first Member = %#v", got[0])
	}
	if strings.Contains(fmt.Sprintf("%#v", got), initialPasswordMarker) {
		t.Fatal("safe Member model retained initialPassword")
	}
}

func TestListMembersRejectsIncompleteDuplicateAndInvalidCollections(t *testing.T) {
	t.Parallel()

	tests := map[string]map[string]any{
		"missing items": {
			"totalCount": 0,
		},
		"incomplete total": {
			"totalCount": 1,
			"items":      []Member{},
		},
		"duplicate UUID": {
			"totalCount": 2,
			"items": []Member{
				{ID: clientMemberID, Email: "one@example.test"},
				{ID: strings.ToUpper(clientMemberID), Email: "two@example.test"},
			},
		},
		"invalid UUID": {
			"totalCount": 1,
			"items": []Member{{
				ID: "not-a-uuid", Email: "member@example.test",
			}},
		},
		"missing email": {
			"totalCount": 1,
			"items":      []Member{{ID: clientMemberID}},
		},
	}

	for name, page := range tests {
		name := name
		page := page
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					return iamTestResponse(request, http.StatusOK, page), nil
				},
			))
			_, err := clientUnderTest.ListMembers(context.Background())
			requireAPIErrorClassification(t, err, ClassificationAmbiguous)
		})
	}
}

func TestGetMemberUsesCompleteMembershipAndExactRead(t *testing.T) {
	t.Parallel()

	canonical := Member{
		ID: clientMemberID, Email: "Canonical.Member@example.test", Name: "Canonical Member",
	}
	tests := map[string]struct {
		byEmail       bool
		selector      string
		listed        []Member
		direct        Member
		directStatus  int
		wantFound     bool
		wantError     bool
		wantDirectGet bool
	}{
		"UUID": {
			selector:      strings.ToUpper(clientMemberID),
			listed:        []Member{canonical},
			direct:        canonical,
			wantFound:     true,
			wantDirectGet: true,
		},
		"case-insensitive full email": {
			byEmail:       true,
			selector:      strings.ToLower(canonical.Email),
			listed:        []Member{canonical},
			direct:        canonical,
			wantFound:     true,
			wantDirectGet: true,
		},
		"missing UUID": {
			selector: clientMemberID,
		},
		"fuzzy email is not accepted": {
			byEmail:  true,
			selector: "canonical.member",
			listed:   []Member{canonical},
		},
		"duplicate email ignoring case": {
			byEmail:   true,
			selector:  canonical.Email,
			wantError: true,
			listed: []Member{
				canonical,
				{ID: clientMemberIDTwo, Email: strings.ToUpper(canonical.Email), Name: "Duplicate"},
			},
		},
		"concurrent email change is ambiguous": {
			byEmail:       true,
			selector:      canonical.Email,
			listed:        []Member{canonical},
			direct:        Member{ID: clientMemberID, Email: "changed@example.test", Name: canonical.Name},
			wantError:     true,
			wantDirectGet: true,
		},
		"mismatched direct identity is ambiguous": {
			selector:      clientMemberID,
			listed:        []Member{canonical},
			direct:        Member{ID: clientMemberIDTwo, Email: canonical.Email, Name: canonical.Name},
			wantError:     true,
			wantDirectGet: true,
		},
		"direct 404 does not override complete collection membership": {
			selector:      clientMemberID,
			listed:        []Member{canonical},
			directStatus:  http.StatusNotFound,
			wantError:     true,
			wantDirectGet: true,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var directCalls atomic.Int32
			directStatus := test.directStatus
			if directStatus == 0 {
				directStatus = http.StatusOK
			}
			listed := test.listed
			if listed == nil {
				listed = []Member{}
			}
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					switch request.URL.EscapedPath() {
					case "/api/v1/members":
						return iamTestResponse(request, http.StatusOK, map[string]any{
							"totalCount": len(listed), "items": listed,
						}), nil
					case "/api/v1/members/" + clientMemberID:
						directCalls.Add(1)
						return iamTestResponse(request, directStatus, test.direct), nil
					default:
						t.Fatalf("unexpected request %s", request.URL.EscapedPath())
						return nil, nil
					}
				},
			))

			var got Member
			var found bool
			var err error
			if test.byEmail {
				got, found, err = clientUnderTest.GetMemberByEmail(context.Background(), test.selector)
			} else {
				got, found, err = clientUnderTest.GetMember(context.Background(), test.selector)
			}
			if (err != nil) != test.wantError || found != test.wantFound {
				t.Fatalf("Member lookup = %#v/%t/%v, want found/error %t/%t", got, found, err, test.wantFound, test.wantError)
			}
			wantDirectCalls := int32(0)
			if test.wantDirectGet {
				wantDirectCalls = 1
			}
			if directCalls.Load() != wantDirectCalls {
				t.Fatalf("direct calls = %d, want %d", directCalls.Load(), wantDirectCalls)
			}
			if test.wantFound && (got.Email != canonical.Email || got.Name != canonical.Name) {
				t.Fatalf("canonical Member = %#v", got)
			}
		})
	}
}

func TestMemberValidationAndErrorsRedactEveryRuntimeIdentity(t *testing.T) {
	t.Parallel()

	const runtimeEmail = "runtime-member-email-marker@example.invalid"
	const runtimeName = "runtime-member-name-marker"
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			return iamTestResponse(request, http.StatusOK, map[string]any{
				"totalCount": 1,
				"items": []Member{{
					ID: "not-a-uuid", Email: runtimeEmail, Name: runtimeName,
				}},
			}), nil
		},
	))

	if _, _, err := clientUnderTest.GetMember(context.Background(), "not-a-uuid"); err == nil {
		t.Fatal("GetMember() accepted an invalid UUID")
	}
	if _, _, err := clientUnderTest.GetMemberByEmail(context.Background(), ""); err == nil {
		t.Fatal("GetMemberByEmail() accepted an empty email")
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid selectors reached transport %d times", calls.Load())
	}

	_, err := clientUnderTest.ListMembers(context.Background())
	if err == nil {
		t.Fatal("ListMembers() accepted an invalid runtime Member")
	}
	formatted := fmt.Sprint(err)
	for _, unsafe := range []string{
		"not-a-uuid", runtimeEmail, runtimeName, "/api/v1/members",
	} {
		if strings.Contains(formatted, unsafe) {
			t.Fatalf("Member error exposed runtime value %q: %s", unsafe, formatted)
		}
	}

	formatted = fmt.Sprintf("%v|%+v|%#v", Member{
		ID: clientMemberID, Email: runtimeEmail, Name: runtimeName,
	}, Member{
		ID: clientMemberID, Email: runtimeEmail, Name: runtimeName,
	}, Member{
		ID: clientMemberID, Email: runtimeEmail, Name: runtimeName,
	})
	for _, unsafe := range []string{clientMemberID, runtimeEmail, runtimeName} {
		if strings.Contains(formatted, unsafe) {
			t.Fatalf("Member formatting exposed runtime value %q", unsafe)
		}
	}
}
