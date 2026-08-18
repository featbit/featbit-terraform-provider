// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestListGroupMemberIDsConsumesEveryPageAndCanonicalizes(t *testing.T) {
	t.Parallel()

	memberIDs := make([]string, 0, groupPageSize+1)
	for index := 0; index < groupPageSize+1; index++ {
		memberIDs = append(memberIDs, fmt.Sprintf("00000000-0000-4000-8000-%012x", index+1))
	}
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodGet ||
				request.URL.EscapedPath() != "/api/v1/groups/"+clientGroupID+"/members" ||
				request.URL.Query().Get("GetAllMembers") != "false" ||
				request.URL.Query().Get("PageSize") != "100" {
				t.Fatalf("request = %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
			}
			var pageIDs []string
			switch pageIndex := request.URL.Query().Get("PageIndex"); pageIndex {
			case "0":
				pageIDs = memberIDs[:groupPageSize]
			case "1":
				pageIDs = memberIDs[groupPageSize:]
			default:
				t.Fatalf("unexpected PageIndex = %q", pageIndex)
			}
			calls.Add(1)
			items := make([]map[string]any, 0, len(pageIDs))
			for _, memberID := range pageIDs {
				items = append(items, map[string]any{
					"id":              strings.ToUpper(memberID),
					"email":           "ignored@example.test",
					"initialPassword": "must-not-be-decoded",
					"isGroupMember":   true,
				})
			}
			return iamTestResponse(request, http.StatusOK, map[string]any{
				"totalCount": len(memberIDs),
				"items":      items,
			}), nil
		},
	))

	got, err := clientUnderTest.ListGroupMemberIDs(context.Background(), clientGroupID)
	if err != nil {
		t.Fatalf("ListGroupMemberIDs() error = %v", err)
	}
	if fmt.Sprint(got) != fmt.Sprint(memberIDs) {
		t.Fatalf("ListGroupMemberIDs() = %v, want canonical IDs", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("page calls = %d, want 2", calls.Load())
	}

	countClient := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			return iamTestResponse(request, http.StatusOK, map[string]any{
				"totalCount": 1,
				"items": []map[string]any{{
					"id": clientMemberID, "isGroupMember": true,
				}},
			}), nil
		},
	))
	count, err := countClient.CountGroupMembers(context.Background(), clientGroupID)
	if err != nil || count != 1 {
		t.Fatalf("CountGroupMembers() = %d/%v", count, err)
	}
}

func TestGroupMemberMutationContractsAndOneShotFailure(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		path   string
		invoke func(*Client) error
	}{
		"add": {
			path: "/api/v1/groups/" + clientGroupID + "/add-member/" + clientMemberID,
			invoke: func(apiClient *Client) error {
				return apiClient.AddGroupMember(context.Background(), clientGroupID, clientMemberID)
			},
		},
		"remove": {
			path: "/api/v1/groups/" + clientGroupID + "/remove-member/" + clientMemberID,
			invoke: func(apiClient *Client) error {
				return apiClient.RemoveGroupMember(context.Background(), clientGroupID, clientMemberID)
			},
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					calls.Add(1)
					body := ""
					if request.Body != nil && request.Body != http.NoBody {
						contents, err := io.ReadAll(request.Body)
						if err != nil {
							t.Fatalf("read mutation body: %v", err)
						}
						body = string(contents)
					}
					if request.Method != http.MethodPut || request.URL.EscapedPath() != test.path || body != "" {
						t.Fatalf("request = %s %s %q", request.Method, request.URL.EscapedPath(), body)
					}
					return iamTestResponse(request, http.StatusOK, true), nil
				},
			))
			if err := test.invoke(clientUnderTest); err != nil {
				t.Fatalf("mutation error = %v", err)
			}
			if calls.Load() != 1 {
				t.Fatalf("mutation calls = %d, want 1", calls.Load())
			}

			var failedCalls atomic.Int32
			failedClient := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					failedCalls.Add(1)
					return iamTestResponse(request, http.StatusInternalServerError, nil), nil
				},
			))
			err := test.invoke(failedClient)
			requireAPIErrorClassification(t, err, ClassificationTransientServer)
			if failedCalls.Load() != 1 {
				t.Fatalf("failed mutation calls = %d, want 1", failedCalls.Load())
			}
		})
	}
}

func TestGroupMemberMutationsRejectInvalidOrUnconfirmedPairs(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			return iamTestResponse(request, http.StatusOK, false), nil
		},
	))

	tests := map[string]error{
		"invalid Group": clientUnderTest.AddGroupMember(
			context.Background(), "not-a-uuid", clientMemberID,
		),
		"invalid Member": clientUnderTest.RemoveGroupMember(
			context.Background(), clientGroupID, "not-a-uuid",
		),
	}
	for name, err := range tests {
		requireAPIErrorClassification(t, err, ClassificationValidation)
		if strings.Contains(fmt.Sprint(err), "not-a-uuid") {
			t.Fatalf("%s error exposed rejected identity", name)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid mutations reached transport %d times", calls.Load())
	}

	err := clientUnderTest.AddGroupMember(
		context.Background(), clientGroupID, clientMemberID,
	)
	requireAPIErrorClassification(t, err, ClassificationAmbiguous)
	formatted := fmt.Sprint(err)
	for _, unsafe := range []string{clientGroupID, clientMemberID, "/api/v1/groups"} {
		if strings.Contains(formatted, unsafe) {
			t.Fatalf("unconfirmed mutation error exposed runtime value %q: %s", unsafe, formatted)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("unconfirmed mutation calls = %d, want 1", calls.Load())
	}
}
