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

func TestGroupAssociationCountsUseMinimalExactCollections(t *testing.T) {
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
	policies, err := clientUnderTest.CountGroupPolicies(context.Background(), clientGroupID)
	if err != nil || policies != 1 {
		t.Fatalf("CountGroupPolicies() = %d/%v", policies, err)
	}
}

func TestGroupAssociationCountsRejectUnconfirmedMembership(t *testing.T) {
	t.Parallel()

	tests := map[string]map[string]any{
		"missing membership": {
			"id": clientMemberID,
		},
		"false membership": {
			"id":            clientMemberID,
			"isGroupMember": false,
		},
		"invalid UUID": {
			"id":            "not-a-uuid",
			"isGroupMember": true,
		},
	}
	for name, item := range tests {
		name := name
		item := item
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					return iamTestResponse(request, http.StatusOK, map[string]any{
						"totalCount": 1,
						"items":      []map[string]any{item},
					}), nil
				},
			))
			_, err := clientUnderTest.CountGroupMembers(context.Background(), clientGroupID)
			requireAPIErrorClassification(t, err, ClassificationAmbiguous)
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
