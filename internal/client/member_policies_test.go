// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

func TestListMemberDirectPolicyIDsConsumesEveryPageAndCanonicalizes(t *testing.T) {
	t.Parallel()

	policyIDs := make([]string, 0, memberPageSize+1)
	for index := 0; index < memberPageSize+1; index++ {
		policyIDs = append(
			policyIDs,
			fmt.Sprintf("00000000-0000-4000-8000-%012x", index+1),
		)
	}
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodGet ||
				request.URL.EscapedPath() != "/api/v1/members/"+clientMemberID+"/direct-policies" ||
				request.URL.Query().Get("GetAllPolicies") != "false" ||
				request.URL.Query().Get("PageSize") != "100" ||
				request.URL.Query().Get("Name") != "" {
				t.Fatalf(
					"request = %s %s?%s",
					request.Method,
					request.URL.EscapedPath(),
					request.URL.RawQuery,
				)
			}
			var pageIDs []string
			switch pageIndex := request.URL.Query().Get("PageIndex"); pageIndex {
			case "0":
				pageIDs = slices.Clone(policyIDs[:memberPageSize])
			case "1":
				pageIDs = slices.Clone(policyIDs[memberPageSize:])
			default:
				t.Fatalf("unexpected PageIndex = %q", pageIndex)
			}
			slices.Reverse(pageIDs)
			calls.Add(1)
			items := make([]map[string]any, 0, len(pageIDs))
			for _, policyID := range pageIDs {
				items = append(items, map[string]any{
					"id":             strings.ToUpper(policyID),
					"isMemberPolicy": true,
					"name":           "must-not-be-decoded",
					"description":    "must-not-be-decoded",
					"statements":     []map[string]any{{"resources": []string{"must-not-be-decoded"}}},
				})
			}
			return iamTestResponse(request, http.StatusOK, map[string]any{
				"totalCount": len(policyIDs),
				"items":      items,
			}), nil
		},
	))

	got, err := clientUnderTest.ListMemberDirectPolicyIDs(
		context.Background(),
		clientMemberID,
	)
	if err != nil {
		t.Fatalf("ListMemberDirectPolicyIDs() error = %v", err)
	}
	if !slices.Equal(got, policyIDs) {
		t.Fatalf("ListMemberDirectPolicyIDs() = %v, want canonical IDs", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("page calls = %d, want 2", calls.Load())
	}
}

func TestListMemberDirectPolicyIDsRejectsUnsafeCollections(t *testing.T) {
	t.Parallel()

	tests := map[string]map[string]any{
		"missing items": {
			"totalCount": 0,
		},
		"incomplete total": {
			"totalCount": 1,
			"items":      []map[string]any{},
		},
		"missing membership": {
			"totalCount": 1,
			"items":      []map[string]any{{"id": clientPolicyIDOne}},
		},
		"false membership": {
			"totalCount": 1,
			"items": []map[string]any{{
				"id": clientPolicyIDOne, "isMemberPolicy": false,
			}},
		},
		"invalid UUID": {
			"totalCount": 1,
			"items": []map[string]any{{
				"id": "not-a-uuid", "isMemberPolicy": true,
			}},
		},
		"duplicate UUID": {
			"totalCount": 2,
			"items": []map[string]any{
				{"id": clientPolicyIDOne, "isMemberPolicy": true},
				{"id": strings.ToUpper(clientPolicyIDOne), "isMemberPolicy": true},
			},
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
			_, err := clientUnderTest.ListMemberDirectPolicyIDs(
				context.Background(),
				clientMemberID,
			)
			requireAPIErrorClassification(t, err, ClassificationAmbiguous)
			formatted := fmt.Sprint(err)
			for _, unsafe := range []string{
				clientMemberID,
				clientPolicyIDOne,
				"not-a-uuid",
				"/api/v1/members",
			} {
				if strings.Contains(formatted, unsafe) {
					t.Fatalf("collection error exposed runtime value %q: %s", unsafe, formatted)
				}
			}
		})
	}
}

func TestMemberDirectPolicyMutationContractsAndOneShotFailure(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		path   string
		invoke func(*Client) error
	}{
		"add": {
			path: "/api/v1/members/" + clientMemberID + "/add-policy/" + clientPolicyIDOne,
			invoke: func(apiClient *Client) error {
				return apiClient.AddMemberDirectPolicy(
					context.Background(), clientMemberID, clientPolicyIDOne,
				)
			},
		},
		"remove": {
			path: "/api/v1/members/" + clientMemberID + "/remove-policy/" + clientPolicyIDOne,
			invoke: func(apiClient *Client) error {
				return apiClient.RemoveMemberDirectPolicy(
					context.Background(), clientMemberID, clientPolicyIDOne,
				)
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
					if request.Method != http.MethodPut ||
						request.URL.EscapedPath() != test.path ||
						request.URL.RawQuery != "" || body != "" {
						t.Fatalf(
							"request = %s %s?%s %q",
							request.Method,
							request.URL.EscapedPath(),
							request.URL.RawQuery,
							body,
						)
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

func TestMemberDirectPolicyOperationsRejectInvalidOrUnconfirmedPairs(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			return iamTestResponse(request, http.StatusOK, false), nil
		},
	))

	if _, err := clientUnderTest.ListMemberDirectPolicyIDs(
		context.Background(),
		"not-a-uuid",
	); err == nil {
		t.Fatal("ListMemberDirectPolicyIDs() accepted an invalid Member UUID")
	}
	invalidMutations := map[string]error{
		"invalid Member": clientUnderTest.AddMemberDirectPolicy(
			context.Background(), "not-a-uuid", clientPolicyIDOne,
		),
		"invalid Policy": clientUnderTest.RemoveMemberDirectPolicy(
			context.Background(), clientMemberID, "not-a-uuid",
		),
	}
	for name, err := range invalidMutations {
		requireAPIErrorClassification(t, err, ClassificationValidation)
		if strings.Contains(fmt.Sprint(err), "not-a-uuid") {
			t.Fatalf("%s error exposed rejected identity", name)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid operations reached transport %d times", calls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	apiError := requireAPIErrorClassification(
		t,
		clientUnderTest.AddMemberDirectPolicy(ctx, clientMemberID, clientPolicyIDOne),
		ClassificationCanceled,
	)
	if !errors.Is(apiError, context.Canceled) || calls.Load() != 0 {
		t.Fatalf("canceled mutation cause/requests = %v/%d", apiError, calls.Load())
	}

	err := clientUnderTest.AddMemberDirectPolicy(
		context.Background(), clientMemberID, clientPolicyIDOne,
	)
	requireAPIErrorClassification(t, err, ClassificationAmbiguous)
	formatted := fmt.Sprint(err)
	for _, unsafe := range []string{
		clientMemberID,
		clientPolicyIDOne,
		"/api/v1/members",
	} {
		if strings.Contains(formatted, unsafe) {
			t.Fatalf("unconfirmed mutation error exposed runtime value %q: %s", unsafe, formatted)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("unconfirmed mutation calls = %d, want 1", calls.Load())
	}
}
