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

const clientGroupIDTwo = "55555555-5555-4555-8555-555555555555"

func TestListGroupsConsumesEveryPage(t *testing.T) {
	t.Parallel()

	groups := make([]Group, 0, groupPageSize+1)
	for index := 0; index < groupPageSize+1; index++ {
		groups = append(groups, Group{
			ID:          fmt.Sprintf("00000000-0000-4000-8000-%012x", index+1),
			Name:        fmt.Sprintf("Group %d", index),
			Description: "",
		})
	}

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			if request.Method != http.MethodGet || request.URL.EscapedPath() != "/api/v1/groups" {
				t.Fatalf("request = %s %s", request.Method, request.URL.EscapedPath())
			}
			if request.URL.Query().Get("PageSize") != "100" {
				t.Fatalf("PageSize = %q", request.URL.Query().Get("PageSize"))
			}
			switch request.URL.Query().Get("PageIndex") {
			case "0":
				return iamTestResponse(request, http.StatusOK, map[string]any{
					"totalCount": len(groups),
					"items":      groups[:groupPageSize],
				}), nil
			case "1":
				return iamTestResponse(request, http.StatusOK, map[string]any{
					"totalCount": len(groups),
					"items":      groups[groupPageSize:],
				}), nil
			default:
				t.Fatalf("unexpected PageIndex = %q", request.URL.Query().Get("PageIndex"))
				return nil, nil
			}
		},
	))

	got, err := clientUnderTest.ListGroups(context.Background())
	if err != nil {
		t.Fatalf("ListGroups() error = %v", err)
	}
	if len(got) != len(groups) || calls.Load() != 2 {
		t.Fatalf("ListGroups() count/calls = %d/%d, want %d/2", len(got), calls.Load(), len(groups))
	}
}

func TestListGroupsRejectsIncompleteDuplicateAndInvalidCollections(t *testing.T) {
	t.Parallel()

	tests := map[string]map[string]any{
		"missing items": {
			"totalCount": 0,
		},
		"incomplete total": {
			"totalCount": 1,
			"items":      []Group{},
		},
		"duplicate UUID": {
			"totalCount": 2,
			"items": []Group{
				clientGroup(clientGroupID, "One"),
				clientGroup(clientGroupID, "Two"),
			},
		},
		"missing name": {
			"totalCount": 1,
			"items": []Group{{
				ID: clientGroupID,
			}},
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
			_, err := clientUnderTest.ListGroups(context.Background())
			requireAPIErrorClassification(t, err, ClassificationAmbiguous)
		})
	}
}

func TestGetGroupProvesMembershipBeforeExactRead(t *testing.T) {
	t.Parallel()

	listed := clientGroup(clientGroupID, "Listed")
	direct := listed
	direct.Name = "Canonical"
	direct.Description = "Canonical description"
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			switch calls.Add(1) {
			case 1:
				if request.URL.EscapedPath() != "/api/v1/groups" {
					t.Fatalf("membership request path = %s", request.URL.EscapedPath())
				}
				return iamTestResponse(request, http.StatusOK, map[string]any{
					"totalCount": 1,
					"items":      []Group{listed},
				}), nil
			case 2:
				if request.URL.EscapedPath() != "/api/v1/groups/"+clientGroupID ||
					request.URL.RawQuery != "" {
					t.Fatalf("exact request = %s?%s", request.URL.EscapedPath(), request.URL.RawQuery)
				}
				return iamTestResponse(request, http.StatusOK, direct), nil
			default:
				t.Fatal("GetGroup() made an unexpected request")
				return nil, nil
			}
		},
	))

	got, found, err := clientUnderTest.GetGroup(context.Background(), clientGroupID)
	if err != nil || !found || got.Name != "Canonical" || calls.Load() != 2 {
		t.Fatalf("GetGroup() = %#v/%t/%v, calls = %d", got, found, err, calls.Load())
	}
}

func TestGetGroupCollectionAbsenceSkipsDirectRead(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			return iamTestResponse(request, http.StatusOK, map[string]any{
				"totalCount": 0,
				"items":      []Group{},
			}), nil
		},
	))

	_, found, err := clientUnderTest.GetGroup(context.Background(), clientGroupID)
	if err != nil || found || calls.Load() != 1 {
		t.Fatalf("GetGroup() found/error/calls = %t/%v/%d", found, err, calls.Load())
	}
}

func TestGetGroupByNameConfirmsExactNameThroughDirectRead(t *testing.T) {
	t.Parallel()

	const exactName = "Existing Operators"
	listed := clientGroup(clientGroupID, exactName)
	direct := listed
	direct.Description = "Canonical description"
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			switch calls.Add(1) {
			case 1:
				return iamTestResponse(request, http.StatusOK, map[string]any{
					"totalCount": 1,
					"items":      []Group{listed},
				}), nil
			case 2:
				if request.URL.EscapedPath() != "/api/v1/groups/"+clientGroupID {
					t.Fatalf("exact request path = %s", request.URL.EscapedPath())
				}
				return iamTestResponse(request, http.StatusOK, direct), nil
			default:
				t.Fatal("GetGroupByName() made an unexpected request")
				return nil, nil
			}
		},
	))

	got, found, err := clientUnderTest.GetGroupByName(context.Background(), exactName)
	if err != nil || !found || got.Description != direct.Description || calls.Load() != 2 {
		t.Fatalf("GetGroupByName() = %#v/%t/%v, calls = %d", got, found, err, calls.Load())
	}
}

func TestGetGroupByNameRejectsConcurrentRenameWithoutDisclosure(t *testing.T) {
	t.Parallel()

	const exactName = "runtime-group-name-marker"
	listed := clientGroup(clientGroupID, exactName)
	direct := listed
	direct.Name = "renamed-during-read"
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			if calls.Add(1) == 1 {
				return iamTestResponse(request, http.StatusOK, map[string]any{
					"totalCount": 1,
					"items":      []Group{listed},
				}), nil
			}
			return iamTestResponse(request, http.StatusOK, direct), nil
		},
	))

	_, found, err := clientUnderTest.GetGroupByName(context.Background(), exactName)
	requireAPIErrorClassification(t, err, ClassificationAmbiguous)
	if found {
		t.Fatal("GetGroupByName() accepted a concurrently renamed Group")
	}
	formatted := fmt.Sprint(err)
	for _, unsafe := range []string{exactName, direct.Name, clientGroupID, "/api/v1/groups"} {
		if strings.Contains(formatted, unsafe) {
			t.Fatalf("error exposed runtime value %q: %s", unsafe, formatted)
		}
	}
}

func TestResolveGroupByNameUsesCaseSensitiveExactMatches(t *testing.T) {
	t.Parallel()

	const exactName = "Operators"
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			t.Fatal("resolver reached transport")
			return nil, nil
		},
	))
	groups := []Group{
		clientGroup(clientGroupID, exactName),
		clientGroup(clientGroupIDTwo, "operators"),
	}

	match, found, err := clientUnderTest.ResolveGroupByName(groups, exactName)
	if err != nil || !found || !EqualUUID(match.ID, clientGroupID) {
		t.Fatalf("ResolveGroupByName(exact) = %#v/%t/%v", match, found, err)
	}
	_, found, err = clientUnderTest.ResolveGroupByName(groups, "missing")
	if err != nil || found {
		t.Fatalf("ResolveGroupByName(missing) found/error = %t/%v", found, err)
	}
	groups = append(groups, clientGroup(
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		exactName,
	))
	_, _, err = clientUnderTest.ResolveGroupByName(groups, exactName)
	requireAPIErrorClassification(t, err, ClassificationAmbiguous)
	if strings.Contains(fmt.Sprint(err), exactName) {
		t.Fatal("duplicate exact-name error exposed the Group name")
	}
}

func TestGroupMutationContracts(t *testing.T) {
	t.Parallel()

	base := clientGroup(clientGroupID, "Managed")
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			body := ""
			if request.Body != nil && request.Body != http.NoBody {
				contents, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatalf("read request body: %v", err)
				}
				body = string(contents)
			}
			switch calls.Add(1) {
			case 1:
				if request.Method != http.MethodPost || request.URL.EscapedPath() != "/api/v1/groups" ||
					body != `{"name":"Managed","description":"Description"}` {
					t.Fatalf("create request = %s %s %s", request.Method, request.URL.EscapedPath(), body)
				}
				created := base
				created.Description = "Description"
				return iamTestResponse(request, http.StatusOK, created), nil
			case 2:
				if request.Method != http.MethodPut ||
					request.URL.EscapedPath() != "/api/v1/groups/"+clientGroupID ||
					body != `{"name":"Renamed","description":"Updated"}` {
					t.Fatalf("update request = %s %s %s", request.Method, request.URL.EscapedPath(), body)
				}
				updated := base
				updated.Name = "Renamed"
				updated.Description = "Updated"
				return iamTestResponse(request, http.StatusOK, updated), nil
			case 3:
				if request.Method != http.MethodDelete ||
					request.URL.EscapedPath() != "/api/v1/groups/"+clientGroupID || body != "" {
					t.Fatalf("delete request = %s %s %s", request.Method, request.URL.EscapedPath(), body)
				}
				return iamTestResponse(request, http.StatusOK, true), nil
			default:
				t.Fatal("unexpected Group mutation request")
				return nil, nil
			}
		},
	))

	if _, err := clientUnderTest.CreateGroup(context.Background(), CreateGroupRequest{
		Name: "Managed", Description: "Description",
	}); err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}
	if _, err := clientUnderTest.UpdateGroup(
		context.Background(),
		clientGroupID,
		UpdateGroupRequest{Name: "Renamed", Description: "Updated"},
	); err != nil {
		t.Fatalf("UpdateGroup() error = %v", err)
	}
	if err := clientUnderTest.DeleteGroup(context.Background(), clientGroupID); err != nil {
		t.Fatalf("DeleteGroup() error = %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("mutation calls = %d, want 3", calls.Load())
	}
}

func TestGroupPolicyMutationContracts(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		path   string
		invoke func(*Client) error
	}{
		"add": {
			path: "/api/v1/groups/" + clientGroupID + "/add-policy/" + clientPolicyIDOne,
			invoke: func(apiClient *Client) error {
				return apiClient.AddGroupPolicy(context.Background(), clientGroupID, clientPolicyIDOne)
			},
		},
		"remove": {
			path: "/api/v1/groups/" + clientGroupID + "/remove-policy/" + clientPolicyIDOne,
			invoke: func(apiClient *Client) error {
				return apiClient.RemoveGroupPolicy(context.Background(), clientGroupID, clientPolicyIDOne)
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
		})
	}
}

func TestGroupMutationsExecuteOnce(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*Client) error{
		"create": func(apiClient *Client) error {
			_, err := apiClient.CreateGroup(
				context.Background(),
				CreateGroupRequest{Name: "Managed"},
			)
			return err
		},
		"update": func(apiClient *Client) error {
			_, err := apiClient.UpdateGroup(
				context.Background(),
				clientGroupID,
				UpdateGroupRequest{Name: "Renamed"},
			)
			return err
		},
		"delete": func(apiClient *Client) error {
			return apiClient.DeleteGroup(context.Background(), clientGroupID)
		},
		"add Policy": func(apiClient *Client) error {
			return apiClient.AddGroupPolicy(context.Background(), clientGroupID, clientPolicyIDOne)
		},
		"remove Policy": func(apiClient *Client) error {
			return apiClient.RemoveGroupPolicy(context.Background(), clientGroupID, clientPolicyIDOne)
		},
	}

	for name, invoke := range tests {
		name := name
		invoke := invoke
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					calls.Add(1)
					return iamTestResponse(request, http.StatusInternalServerError, nil), nil
				},
			))
			err := invoke(clientUnderTest)
			requireAPIErrorClassification(t, err, ClassificationTransientServer)
			if calls.Load() != 1 {
				t.Fatalf("mutation calls = %d, want 1", calls.Load())
			}
		})
	}
}

func TestGroupAssociationIDsUseMinimalExactCollections(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodGet || request.URL.Query().Get("PageIndex") != "0" ||
				request.URL.Query().Get("PageSize") != "100" {
				t.Fatalf("association request = %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
			}
			switch calls.Add(1) {
			case 1:
				if request.URL.EscapedPath() != "/api/v1/groups/"+clientGroupID+"/members" ||
					request.URL.Query().Get("GetAllMembers") != "false" {
					t.Fatalf("Member association request = %s?%s", request.URL.EscapedPath(), request.URL.RawQuery)
				}
				return iamTestResponse(request, http.StatusOK, map[string]any{
					"totalCount": 1,
					"items": []map[string]any{{
						"id":              clientMemberID,
						"email":           "ignored@example.test",
						"initialPassword": "must-not-be-decoded",
						"isGroupMember":   true,
					}},
				}), nil
			case 2:
				if request.URL.EscapedPath() != "/api/v1/groups/"+clientGroupID+"/policies" ||
					request.URL.Query().Get("GetAllPolicies") != "false" {
					t.Fatalf("Policy association request = %s?%s", request.URL.EscapedPath(), request.URL.RawQuery)
				}
				return iamTestResponse(request, http.StatusOK, map[string]any{
					"totalCount": 1,
					"items": []map[string]any{{
						"id":            clientPolicyIDOne,
						"name":          "ignored-policy-name",
						"description":   "ignored-policy-description",
						"isGroupPolicy": true,
					}},
				}), nil
			default:
				t.Fatal("unexpected association request")
				return nil, nil
			}
		},
	))

	members, err := clientUnderTest.CountGroupMembers(context.Background(), clientGroupID)
	if err != nil || members != 1 {
		t.Fatalf("CountGroupMembers() = %d/%v", members, err)
	}
	policyIDs, err := clientUnderTest.ListGroupPolicyIDs(context.Background(), clientGroupID)
	if err != nil || len(policyIDs) != 1 || policyIDs[0] != clientPolicyIDOne {
		t.Fatalf("ListGroupPolicyIDs() = %v/%v", policyIDs, err)
	}
}

func TestListGroupPolicyIDsConsumesEveryPageAndCanonicalizes(t *testing.T) {
	t.Parallel()

	policyIDs := make([]string, 0, groupPageSize+1)
	for index := 0; index < groupPageSize+1; index++ {
		policyIDs = append(policyIDs, fmt.Sprintf("00000000-0000-4000-8000-%012x", index+1))
	}
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodGet ||
				request.URL.EscapedPath() != "/api/v1/groups/"+clientGroupID+"/policies" ||
				request.URL.Query().Get("GetAllPolicies") != "false" ||
				request.URL.Query().Get("PageSize") != "100" {
				t.Fatalf("request = %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
			}
			var pageIDs []string
			switch pageIndex := request.URL.Query().Get("PageIndex"); pageIndex {
			case "0":
				pageIDs = policyIDs[:groupPageSize]
			case "1":
				pageIDs = policyIDs[groupPageSize:]
			default:
				t.Fatalf("unexpected PageIndex = %q", pageIndex)
			}
			calls.Add(1)
			items := make([]map[string]any, 0, len(pageIDs))
			for _, policyID := range pageIDs {
				items = append(items, map[string]any{
					"id":            strings.ToUpper(policyID),
					"isGroupPolicy": true,
				})
			}
			return iamTestResponse(request, http.StatusOK, map[string]any{
				"totalCount": len(policyIDs),
				"items":      items,
			}), nil
		},
	))

	got, err := clientUnderTest.ListGroupPolicyIDs(context.Background(), clientGroupID)
	if err != nil {
		t.Fatalf("ListGroupPolicyIDs() error = %v", err)
	}
	if fmt.Sprint(got) != fmt.Sprint(policyIDs) {
		t.Fatalf("ListGroupPolicyIDs() = %v, want canonical IDs", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("page calls = %d, want 2", calls.Load())
	}

	countClient := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			return iamTestResponse(request, http.StatusOK, map[string]any{
				"totalCount": 1,
				"items": []map[string]any{{
					"id": clientPolicyIDOne, "isGroupPolicy": true,
				}},
			}), nil
		},
	))
	count, err := countClient.CountGroupPolicies(context.Background(), clientGroupID)
	if err != nil || count != 1 {
		t.Fatalf("CountGroupPolicies() = %d/%v", count, err)
	}
}

func TestGroupPolicyMutationsRejectInvalidOrUnconfirmedPairs(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			return iamTestResponse(request, http.StatusOK, false), nil
		},
	))

	tests := map[string]error{
		"invalid Group": clientUnderTest.AddGroupPolicy(
			context.Background(), "not-a-uuid", clientPolicyIDOne,
		),
		"invalid Policy": clientUnderTest.RemoveGroupPolicy(
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

	err := clientUnderTest.AddGroupPolicy(
		context.Background(), clientGroupID, clientPolicyIDOne,
	)
	requireAPIErrorClassification(t, err, ClassificationAmbiguous)
	formatted := fmt.Sprint(err)
	for _, unsafe := range []string{clientGroupID, clientPolicyIDOne, "/api/v1/groups"} {
		if strings.Contains(formatted, unsafe) {
			t.Fatalf("unconfirmed mutation error exposed runtime value %q: %s", unsafe, formatted)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("unconfirmed mutation calls = %d, want 1", calls.Load())
	}
}

func TestGroupAssociationIDsRejectUnconfirmedMembership(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		items  []map[string]any
		invoke func(*Client) error
	}{
		"Member missing membership": {
			items: []map[string]any{{"id": clientMemberID}},
			invoke: func(apiClient *Client) error {
				_, err := apiClient.CountGroupMembers(context.Background(), clientGroupID)
				return err
			},
		},
		"Member false membership": {
			items: []map[string]any{{"id": clientMemberID, "isGroupMember": false}},
			invoke: func(apiClient *Client) error {
				_, err := apiClient.CountGroupMembers(context.Background(), clientGroupID)
				return err
			},
		},
		"Member invalid UUID": {
			items: []map[string]any{{"id": "not-a-uuid", "isGroupMember": true}},
			invoke: func(apiClient *Client) error {
				_, err := apiClient.CountGroupMembers(context.Background(), clientGroupID)
				return err
			},
		},
		"Policy missing membership": {
			items: []map[string]any{{"id": clientPolicyIDOne}},
			invoke: func(apiClient *Client) error {
				_, err := apiClient.ListGroupPolicyIDs(context.Background(), clientGroupID)
				return err
			},
		},
		"Policy false membership": {
			items: []map[string]any{{"id": clientPolicyIDOne, "isGroupPolicy": false}},
			invoke: func(apiClient *Client) error {
				_, err := apiClient.ListGroupPolicyIDs(context.Background(), clientGroupID)
				return err
			},
		},
		"Policy invalid UUID": {
			items: []map[string]any{{"id": "not-a-uuid", "isGroupPolicy": true}},
			invoke: func(apiClient *Client) error {
				_, err := apiClient.ListGroupPolicyIDs(context.Background(), clientGroupID)
				return err
			},
		},
		"Policy duplicate UUID": {
			items: []map[string]any{
				{"id": clientPolicyIDOne, "isGroupPolicy": true},
				{"id": strings.ToUpper(clientPolicyIDOne), "isGroupPolicy": true},
			},
			invoke: func(apiClient *Client) error {
				_, err := apiClient.ListGroupPolicyIDs(context.Background(), clientGroupID)
				return err
			},
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					return iamTestResponse(request, http.StatusOK, map[string]any{
						"totalCount": len(test.items),
						"items":      test.items,
					}), nil
				},
			))
			err := test.invoke(clientUnderTest)
			requireAPIErrorClassification(t, err, ClassificationAmbiguous)
			if strings.Contains(fmt.Sprint(err), "not-a-uuid") {
				t.Fatal("association error exposed an invalid runtime identity")
			}
		})
	}
}

func TestGroupErrorsRedactRuntimeDefinitionValues(t *testing.T) {
	t.Parallel()

	const runtimeName = "runtime-group-name-marker"
	const runtimeDescription = "runtime-group-description-marker"
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			return iamTestResponse(request, http.StatusOK, Group{
				ID:          "not-a-uuid",
				Name:        runtimeName,
				Description: runtimeDescription,
			}), nil
		},
	))
	_, err := clientUnderTest.CreateGroup(
		context.Background(),
		CreateGroupRequest{Name: runtimeName, Description: runtimeDescription},
	)
	if err == nil {
		t.Fatal("CreateGroup() unexpectedly succeeded")
	}
	formatted := fmt.Sprint(err)
	for _, unsafe := range []string{
		runtimeName,
		runtimeDescription,
		clientGroupID,
		"/api/v1/groups",
	} {
		if strings.Contains(formatted, unsafe) {
			t.Fatalf("error exposed runtime value %q: %s", unsafe, formatted)
		}
	}
}

func clientGroup(id string, name string) Group {
	return Group{ID: id, Name: name, Description: ""}
}
