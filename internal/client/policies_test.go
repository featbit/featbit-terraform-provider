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

const (
	clientPolicyIDOne = "11111111-1111-4111-8111-111111111111"
	clientPolicyIDTwo = "22222222-2222-4222-8222-222222222222"
	clientGroupID     = "33333333-3333-4333-8333-333333333333"
	clientMemberID    = "44444444-4444-4444-8444-444444444444"
)

func TestListPoliciesConsumesEveryPage(t *testing.T) {
	t.Parallel()

	policies := make([]Policy, 0, policyPageSize+1)
	for index := 0; index < policyPageSize+1; index++ {
		policies = append(policies, Policy{
			ID:          fmt.Sprintf("00000000-0000-4000-8000-%012x", index+1),
			Name:        fmt.Sprintf("Policy %d", index),
			Key:         fmt.Sprintf("policy-%d", index),
			Type:        PolicyTypeCustomerManaged,
			Description: "",
			Statements:  []PolicyStatement{},
		})
	}

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			if request.Method != http.MethodGet || request.URL.EscapedPath() != "/api/v1/policies" {
				t.Fatalf("request = %s %s", request.Method, request.URL.EscapedPath())
			}
			page := request.URL.Query().Get("PageIndex")
			if request.URL.Query().Get("PageSize") != "100" {
				t.Fatalf("PageSize = %q", request.URL.Query().Get("PageSize"))
			}
			switch page {
			case "0":
				return iamTestResponse(request, http.StatusOK, map[string]any{
					"totalCount": len(policies),
					"items":      policies[:policyPageSize],
				}), nil
			case "1":
				return iamTestResponse(request, http.StatusOK, map[string]any{
					"totalCount": len(policies),
					"items":      policies[policyPageSize:],
				}), nil
			default:
				t.Fatalf("unexpected PageIndex = %q", page)
				return nil, nil
			}
		},
	))

	got, err := clientUnderTest.ListPolicies(context.Background())
	if err != nil {
		t.Fatalf("ListPolicies() error = %v", err)
	}
	if len(got) != len(policies) || calls.Load() != 2 {
		t.Fatalf("ListPolicies() count/calls = %d/%d, want %d/2", len(got), calls.Load(), len(policies))
	}
}

func TestListPoliciesRejectsIncompleteAndDuplicateCollections(t *testing.T) {
	t.Parallel()

	tests := map[string]map[string]any{
		"missing items": {
			"totalCount": 0,
		},
		"inconsistent total": {
			"totalCount": 2,
			"items":      []Policy{},
		},
		"duplicate UUID": {
			"totalCount": 2,
			"items": []Policy{
				clientPolicy(clientPolicyIDOne, "one", PolicyTypeCustomerManaged, nil),
				clientPolicy(clientPolicyIDOne, "two", PolicyTypeCustomerManaged, nil),
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
			_, err := clientUnderTest.ListPolicies(context.Background())
			requireAPIErrorClassification(t, err, ClassificationAmbiguous)
		})
	}
}

func TestGetPolicyProvesMembershipBeforeExactRead(t *testing.T) {
	t.Parallel()

	listed := clientPolicy(clientPolicyIDOne, "exact", PolicyTypeCustomerManaged, []PolicyStatement{})
	direct := listed
	direct.Name = "Canonical"
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			switch calls.Add(1) {
			case 1:
				if request.URL.EscapedPath() != "/api/v1/policies" {
					t.Fatalf("membership request path = %s", request.URL.EscapedPath())
				}
				return iamTestResponse(request, http.StatusOK, map[string]any{
					"totalCount": 1,
					"items":      []Policy{listed},
				}), nil
			case 2:
				if request.URL.EscapedPath() != "/api/v1/policies/"+clientPolicyIDOne ||
					request.URL.RawQuery != "" {
					t.Fatalf("exact request = %s?%s", request.URL.EscapedPath(), request.URL.RawQuery)
				}
				return iamTestResponse(request, http.StatusOK, direct), nil
			default:
				t.Fatal("GetPolicy() made an unexpected request")
				return nil, nil
			}
		},
	))

	got, found, err := clientUnderTest.GetPolicy(context.Background(), clientPolicyIDOne)
	if err != nil || !found || got.Name != "Canonical" || calls.Load() != 2 {
		t.Fatalf("GetPolicy() = %#v/%t/%v, calls = %d", got, found, err, calls.Load())
	}
}

func TestGetPolicyCollectionAbsenceSkipsDirectRead(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			return iamTestResponse(request, http.StatusOK, map[string]any{
				"totalCount": 0,
				"items":      []Policy{},
			}), nil
		},
	))

	_, found, err := clientUnderTest.GetPolicy(context.Background(), clientPolicyIDOne)
	if err != nil || found || calls.Load() != 1 {
		t.Fatalf("GetPolicy() found/error/calls = %t/%v/%d", found, err, calls.Load())
	}
}

func TestResolvePolicyExactKeyIncludesBuiltInsAndRejectsDuplicates(t *testing.T) {
	t.Parallel()

	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			t.Fatal("resolver reached transport")
			return nil, nil
		},
	))
	policies := []Policy{
		clientPolicy(clientPolicyIDOne, "Owner", PolicyTypeSysManaged, []PolicyStatement{}),
		clientPolicy(clientPolicyIDTwo, "owner", PolicyTypeCustomerManaged, []PolicyStatement{}),
	}

	match, found, err := clientUnderTest.ResolvePolicyByKey(policies, "Owner")
	if err != nil || !found || match.Type != PolicyTypeSysManaged {
		t.Fatalf("ResolvePolicyByKey(Owner) = %#v/%t/%v", match, found, err)
	}
	_, found, err = clientUnderTest.ResolvePolicyByKey(policies, "missing")
	if err != nil || found {
		t.Fatalf("ResolvePolicyByKey(missing) found/error = %t/%v", found, err)
	}
	policies = append(policies, clientPolicy(
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"Owner",
		PolicyTypeSysManaged,
		[]PolicyStatement{},
	))
	_, _, err = clientUnderTest.ResolvePolicyByKey(policies, "Owner")
	requireAPIErrorClassification(t, err, ClassificationAmbiguous)
}

func TestPolicyMutationContracts(t *testing.T) {
	t.Parallel()

	statement := PolicyStatement{
		ResourceType: "flag",
		Effect:       "allow",
		Actions:      []string{"ToggleFlag"},
		Resources:    []string{"project/p:env/e:flag/f"},
	}
	base := clientPolicy(clientPolicyIDOne, "managed", PolicyTypeCustomerManaged, []PolicyStatement{})
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
				if request.Method != http.MethodPost || request.URL.EscapedPath() != "/api/v1/policies" ||
					body != `{"name":"Managed","key":"managed","description":"Description"}` {
					t.Fatalf("create request = %s %s %s", request.Method, request.URL.EscapedPath(), body)
				}
				created := base
				created.Name = "Managed"
				created.Description = "Description"
				return iamTestResponse(request, http.StatusOK, created), nil
			case 2:
				if request.Method != http.MethodPut ||
					request.URL.EscapedPath() != "/api/v1/policies/"+clientPolicyIDOne+"/settings" ||
					body != `{"name":"Renamed","description":"Updated"}` {
					t.Fatalf("settings request = %s %s %s", request.Method, request.URL.EscapedPath(), body)
				}
				updated := base
				updated.Name = "Renamed"
				updated.Description = "Updated"
				return iamTestResponse(request, http.StatusOK, updated), nil
			case 3:
				if request.Method != http.MethodPut ||
					request.URL.EscapedPath() != "/api/v1/policies/"+clientPolicyIDOne+"/statements" ||
					body != `[{"resourceType":"flag","effect":"allow","actions":["ToggleFlag"],"resources":["project/p:env/e:flag/f"]}]` {
					t.Fatalf("statements request = %s %s %s", request.Method, request.URL.EscapedPath(), body)
				}
				updated := base
				updated.Statements = []PolicyStatement{statement}
				return iamTestResponse(request, http.StatusOK, updated), nil
			case 4:
				if request.Method != http.MethodDelete ||
					request.URL.EscapedPath() != "/api/v1/policies/"+clientPolicyIDOne || body != "" {
					t.Fatalf("delete request = %s %s %s", request.Method, request.URL.EscapedPath(), body)
				}
				return iamTestResponse(request, http.StatusOK, true), nil
			default:
				t.Fatal("unexpected Policy mutation request")
				return nil, nil
			}
		},
	))

	if _, err := clientUnderTest.CreatePolicy(context.Background(), CreatePolicyRequest{
		Name:        "Managed",
		Key:         "managed",
		Description: "Description",
	}); err != nil {
		t.Fatalf("CreatePolicy() error = %v", err)
	}
	if _, err := clientUnderTest.UpdatePolicySettings(
		context.Background(),
		clientPolicyIDOne,
		UpdatePolicySettingsRequest{Name: "Renamed", Description: "Updated"},
	); err != nil {
		t.Fatalf("UpdatePolicySettings() error = %v", err)
	}
	if _, err := clientUnderTest.ReplacePolicyStatements(
		context.Background(),
		clientPolicyIDOne,
		[]PolicyStatement{statement},
	); err != nil {
		t.Fatalf("ReplacePolicyStatements() error = %v", err)
	}
	if err := clientUnderTest.DeletePolicy(context.Background(), clientPolicyIDOne); err != nil {
		t.Fatalf("DeletePolicy() error = %v", err)
	}
	if calls.Load() != 4 {
		t.Fatalf("mutation calls = %d, want 4", calls.Load())
	}
}

func TestPolicyMutationsExecuteOnce(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*Client) error{
		"create": func(apiClient *Client) error {
			_, err := apiClient.CreatePolicy(context.Background(), CreatePolicyRequest{
				Name: "Managed", Key: "managed",
			})
			return err
		},
		"settings": func(apiClient *Client) error {
			_, err := apiClient.UpdatePolicySettings(
				context.Background(),
				clientPolicyIDOne,
				UpdatePolicySettingsRequest{Name: "Renamed"},
			)
			return err
		},
		"statements": func(apiClient *Client) error {
			_, err := apiClient.ReplacePolicyStatements(
				context.Background(),
				clientPolicyIDOne,
				[]PolicyStatement{},
			)
			return err
		},
		"delete": func(apiClient *Client) error {
			return apiClient.DeletePolicy(context.Background(), clientPolicyIDOne)
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

func TestPolicyAssociationCountsUseMinimalExactCollections(t *testing.T) {
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
				if request.URL.EscapedPath() != "/api/v1/policies/"+clientPolicyIDOne+"/groups" ||
					request.URL.Query().Get("GetAllGroups") != "false" {
					t.Fatalf("Group association request = %s?%s", request.URL.EscapedPath(), request.URL.RawQuery)
				}
				return iamTestResponse(request, http.StatusOK, map[string]any{
					"totalCount": 1,
					"items": []map[string]any{{
						"id":            clientGroupID,
						"name":          "ignored-group-name",
						"isPolicyGroup": true,
					}},
				}), nil
			case 2:
				if request.URL.EscapedPath() != "/api/v1/policies/"+clientPolicyIDOne+"/members" ||
					request.URL.Query().Get("GetAllMembers") != "false" {
					t.Fatalf("Member association request = %s?%s", request.URL.EscapedPath(), request.URL.RawQuery)
				}
				return iamTestResponse(request, http.StatusOK, map[string]any{
					"totalCount": 1,
					"items": []map[string]any{{
						"id":              clientMemberID,
						"email":           "ignored@example.test",
						"initialPassword": "must-not-be-decoded",
						"isPolicyMember":  true,
					}},
				}), nil
			default:
				t.Fatal("unexpected association request")
				return nil, nil
			}
		},
	))

	groups, err := clientUnderTest.CountPolicyGroups(context.Background(), clientPolicyIDOne)
	if err != nil || groups != 1 {
		t.Fatalf("CountPolicyGroups() = %d/%v", groups, err)
	}
	members, err := clientUnderTest.CountPolicyMembers(context.Background(), clientPolicyIDOne)
	if err != nil || members != 1 {
		t.Fatalf("CountPolicyMembers() = %d/%v", members, err)
	}
}

func TestPolicyErrorsRedactRuntimeDefinitionValues(t *testing.T) {
	t.Parallel()

	const runtimeKey = "runtime-policy-key-marker"
	const runtimeSelector = "project/runtime-project:env/runtime-env:flag/runtime-flag;runtime-tag"
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			return iamTestResponse(request, http.StatusUnprocessableEntity, map[string]any{
				"message": runtimeKey + runtimeSelector,
			}), nil
		},
	))
	_, err := clientUnderTest.ReplacePolicyStatements(
		context.Background(),
		clientPolicyIDOne,
		[]PolicyStatement{{
			ResourceType: "flag",
			Effect:       "allow",
			Actions:      []string{"ToggleFlag"},
			Resources:    []string{runtimeSelector},
		}},
	)
	if err == nil {
		t.Fatal("ReplacePolicyStatements() unexpectedly succeeded")
	}
	formatted := fmt.Sprint(err)
	for _, unsafe := range []string{runtimeKey, runtimeSelector, clientPolicyIDOne, "/api/v1/policies"} {
		if strings.Contains(formatted, unsafe) {
			t.Fatalf("error exposed runtime value %q: %s", unsafe, formatted)
		}
	}
}

func clientPolicy(
	id string,
	key string,
	policyType string,
	statements []PolicyStatement,
) Policy {
	if statements == nil {
		statements = []PolicyStatement{}
	}
	return Policy{
		ID:          id,
		Name:        "Policy",
		Key:         key,
		Type:        policyType,
		Description: "",
		Statements:  statements,
	}
}
