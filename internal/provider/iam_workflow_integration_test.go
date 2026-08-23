// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	testingterraform "github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	iamWorkflowOwnerPolicyID  = "10000000-0000-4000-8000-000000000001"
	iamWorkflowBasePolicyID   = "20000000-0000-4000-8000-000000000001"
	iamWorkflowScopedPolicyID = "20000000-0000-4000-8000-000000000002"
	iamWorkflowAdminGroupID   = "30000000-0000-4000-8000-000000000001"
	iamWorkflowDeveloperID    = "30000000-0000-4000-8000-000000000002"
	iamWorkflowMemberID       = "40000000-0000-4000-8000-000000000001"

	iamWorkflowInitialPasswordMarker = "p6-100-initial-password-must-not-enter-state"
	iamWorkflowProjectKey            = "iam-workflow"
	iamWorkflowMemberEmail           = "p6-100.member@example.invalid"

	iamWorkflowProjectResourceName       = "featbit_project.iam"
	iamWorkflowProjectDataName           = "data.featbit_project.iam"
	iamWorkflowDevEnvironmentDataName    = "data.featbit_environment.dev"
	iamWorkflowProdEnvironmentDataName   = "data.featbit_environment.prod"
	iamWorkflowOwnerPolicyDataName       = "data.featbit_policy.owner"
	iamWorkflowBasePolicyResourceName    = "featbit_policy.base_access"
	iamWorkflowScopedPolicyResourceName  = "featbit_policy.scoped_access"
	iamWorkflowAdminGroupResourceName    = "featbit_group.admin"
	iamWorkflowDeveloperResourceName     = "featbit_group.developer"
	iamWorkflowAdminGroupDataName        = "data.featbit_group.admin_by_id"
	iamWorkflowDeveloperGroupDataName    = "data.featbit_group.developer_by_name"
	iamWorkflowMemberDataName            = "data.featbit_member.developer"
	iamWorkflowOwnerBindingResourceName  = "featbit_group_policy_binding.admin_owner"
	iamWorkflowBaseBindingResourceName   = "featbit_group_policy_binding.developer_base"
	iamWorkflowScopedBindingResourceName = "featbit_group_policy_binding.developer_scoped"
	iamWorkflowMemberBindingResourceName = "featbit_group_member_binding.developer"
	iamWorkflowMemberPolicyResourceName  = "featbit_member_policy_binding.developer"
)

func TestIAMCustomerWorkflowProtocolIntegrationExactPairAndCleanup(t *testing.T) {
	fixture := newIAMWorkflowFixture(t)
	t.Cleanup(func() {
		if err := fixture.cleanupError(); err != nil {
			t.Error(err)
		}
		fixture.close()
	})

	fullConfig := iamWorkflowConfig(fixture.apiOrigin(), true)
	withoutBaseBinding := iamWorkflowConfig(fixture.apiOrigin(), false)
	var removalMutationBaseline int

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"featbit": providerserver.NewProtocol6WithError(New("protocol-test")()),
		},
		Steps: []resource.TestStep{
			{
				Config: fullConfig,
				Check: resource.ComposeTestCheckFunc(
					iamWorkflowStateChecks(true),
					iamWorkflowStatementStateCheck,
					iamWorkflowStateSafetyCheck,
					fixture.graphCheck(true),
					fixture.initialMutationCheck(),
				),
			},
			{
				Config:   fullConfig,
				PlanOnly: true,
			},
			{
				PreConfig: func() {
					removalMutationBaseline = fixture.mutationCount()
				},
				Config: withoutBaseBinding,
				Check: resource.ComposeTestCheckFunc(
					iamWorkflowStateChecks(false),
					iamWorkflowStatementStateCheck,
					iamWorkflowStateSafetyCheck,
					fixture.graphCheck(false),
					fixture.exactRemovalMutationCheck(&removalMutationBaseline),
				),
			},
		},
	})

	if err := fixture.cleanupError(); err != nil {
		t.Fatal(err)
	}
}

func iamWorkflowConfig(apiOrigin string, includeBaseBinding bool) string {
	baseBinding := ""
	if includeBaseBinding {
		baseBinding = `
resource "featbit_group_policy_binding" "developer_base" {
  group_id  = data.featbit_group.developer_by_name.id
  policy_id = featbit_policy.base_access.id
}
`
	}

	return fmt.Sprintf(`
provider "featbit" {
  api_url              = %q
  access_token         = %q
  http_timeout_seconds = 5
  max_concurrency      = 4
  max_retries          = 0
}

resource "featbit_project" "iam" {
  name = "IAM Workflow"
  key  = %q
}

data "featbit_project" "iam" {
  key        = featbit_project.iam.key
  depends_on = [featbit_project.iam]
}

data "featbit_environment" "dev" {
  project_id = data.featbit_project.iam.id
  key        = "dev"
}

data "featbit_environment" "prod" {
  project_id = data.featbit_project.iam.id
  key        = "prod"
}

data "featbit_policy" "owner" {
  key = "Owner"
}

resource "featbit_policy" "base_access" {
  name        = "IAM Base Access"
  key         = "iam-base-access"
  description = "Project and Environment visibility"

  statements = [
    {
      resource_type = "project"
      effect        = "allow"
      actions       = ["CanAccessProject"]
      resources     = ["project/${data.featbit_project.iam.key}"]
    },
    {
      resource_type = "env"
      effect        = "allow"
      actions       = ["CanAccessEnv"]
      resources     = ["project/${data.featbit_project.iam.key}:env/*"]
    },
  ]
}

resource "featbit_policy" "scoped_access" {
  name        = "IAM Scoped Access"
  key         = "iam-scoped-access"
  description = "Dev operations with one prod exception"

  statements = [
    {
      resource_type = "flag"
      effect        = "allow"
      actions       = ["UpdateFlagName", "ToggleFlag"]
      resources     = ["project/${data.featbit_project.iam.key}:env/${data.featbit_environment.dev.key}:flag/*"]
    },
    {
      resource_type = "segment"
      effect        = "allow"
      actions       = ["UpdateSegmentRules", "UpdateSegmentDescription"]
      resources     = ["project/${data.featbit_project.iam.key}:env/${data.featbit_environment.dev.key}:segment/*"]
    },
    {
      resource_type = "flag"
      effect        = "allow"
      actions       = ["ToggleFlag"]
      resources     = ["project/${data.featbit_project.iam.key}:env/${data.featbit_environment.prod.key}:flag/prod-exception"]
    },
    {
      resource_type = "flag"
      effect        = "deny"
      actions       = ["DeleteFlag"]
      resources     = ["project/${data.featbit_project.iam.key}:env/${data.featbit_environment.dev.key}:flag/protected-flag"]
    },
  ]
}

resource "featbit_group" "admin" {
  name        = "IAM Administrators"
  description = "Owner access"
}

resource "featbit_group" "developer" {
  name        = "IAM Developers"
  description = "Scoped developer access"
}

data "featbit_group" "admin_by_id" {
  id = featbit_group.admin.id
}

data "featbit_group" "developer_by_name" {
  name       = featbit_group.developer.name
  depends_on = [featbit_group.developer]
}

data "featbit_member" "developer" {
  email = %q
}

resource "featbit_group_policy_binding" "admin_owner" {
  group_id  = data.featbit_group.admin_by_id.id
  policy_id = data.featbit_policy.owner.id
}

%s
resource "featbit_group_policy_binding" "developer_scoped" {
  group_id  = data.featbit_group.developer_by_name.id
  policy_id = featbit_policy.scoped_access.id
}

resource "featbit_group_member_binding" "developer" {
  group_id  = data.featbit_group.developer_by_name.id
  member_id = data.featbit_member.developer.id
}

resource "featbit_member_policy_binding" "developer" {
  member_id = data.featbit_member.developer.id
  policy_id = featbit_policy.base_access.id
}
`, apiOrigin, syntheticProviderAccessToken, iamWorkflowProjectKey,
		iamWorkflowMemberEmail, baseBinding)
}

func iamWorkflowStateChecks(includeBaseBinding bool) resource.TestCheckFunc {
	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(iamWorkflowProjectResourceName, "key", iamWorkflowProjectKey),
		resource.TestCheckResourceAttr(iamWorkflowProjectDataName, "key", iamWorkflowProjectKey),
		resource.TestCheckResourceAttr(iamWorkflowDevEnvironmentDataName, "key", "dev"),
		resource.TestCheckResourceAttr(iamWorkflowProdEnvironmentDataName, "key", "prod"),
		resource.TestCheckResourceAttr(iamWorkflowOwnerPolicyDataName, "key", "Owner"),
		resource.TestCheckResourceAttr(iamWorkflowOwnerPolicyDataName, "type", client.PolicyTypeSysManaged),
		resource.TestCheckResourceAttr(iamWorkflowBasePolicyResourceName, "type", client.PolicyTypeCustomerManaged),
		resource.TestCheckResourceAttr(iamWorkflowBasePolicyResourceName, "statements.#", "2"),
		resource.TestCheckResourceAttr(iamWorkflowScopedPolicyResourceName, "type", client.PolicyTypeCustomerManaged),
		resource.TestCheckResourceAttr(iamWorkflowScopedPolicyResourceName, "statements.#", "4"),
		resource.TestCheckResourceAttr(iamWorkflowMemberDataName, "email", iamWorkflowMemberEmail),
		resource.TestCheckResourceAttrPair(
			iamWorkflowProjectResourceName, "id", iamWorkflowProjectDataName, "id",
		),
		resource.TestCheckResourceAttrPair(
			iamWorkflowProjectDataName, "id", iamWorkflowDevEnvironmentDataName, "project_id",
		),
		resource.TestCheckResourceAttrPair(
			iamWorkflowProjectDataName, "id", iamWorkflowProdEnvironmentDataName, "project_id",
		),
		resource.TestCheckResourceAttrPair(
			iamWorkflowAdminGroupResourceName, "id", iamWorkflowAdminGroupDataName, "id",
		),
		resource.TestCheckResourceAttrPair(
			iamWorkflowDeveloperResourceName, "id", iamWorkflowDeveloperGroupDataName, "id",
		),
		resource.TestCheckResourceAttrPair(
			iamWorkflowAdminGroupDataName, "id", iamWorkflowOwnerBindingResourceName, "group_id",
		),
		resource.TestCheckResourceAttrPair(
			iamWorkflowOwnerPolicyDataName, "id", iamWorkflowOwnerBindingResourceName, "policy_id",
		),
		resource.TestCheckResourceAttrPair(
			iamWorkflowDeveloperGroupDataName, "id", iamWorkflowScopedBindingResourceName, "group_id",
		),
		resource.TestCheckResourceAttrPair(
			iamWorkflowScopedPolicyResourceName, "id", iamWorkflowScopedBindingResourceName, "policy_id",
		),
		resource.TestCheckResourceAttrPair(
			iamWorkflowDeveloperGroupDataName, "id", iamWorkflowMemberBindingResourceName, "group_id",
		),
		resource.TestCheckResourceAttrPair(
			iamWorkflowMemberDataName, "id", iamWorkflowMemberBindingResourceName, "member_id",
		),
		resource.TestCheckResourceAttrPair(
			iamWorkflowMemberDataName, "id", iamWorkflowMemberPolicyResourceName, "member_id",
		),
		resource.TestCheckResourceAttrPair(
			iamWorkflowBasePolicyResourceName, "id", iamWorkflowMemberPolicyResourceName, "policy_id",
		),
	}
	if includeBaseBinding {
		checks = append(checks,
			resource.TestCheckResourceAttrPair(
				iamWorkflowDeveloperGroupDataName, "id", iamWorkflowBaseBindingResourceName, "group_id",
			),
			resource.TestCheckResourceAttrPair(
				iamWorkflowBasePolicyResourceName, "id", iamWorkflowBaseBindingResourceName, "policy_id",
			),
		)
	} else {
		checks = append(checks, func(state *testingterraform.State) error {
			if _, exists := state.RootModule().Resources[iamWorkflowBaseBindingResourceName]; exists {
				return fmt.Errorf("removed base Group-Policy binding remained in Terraform state")
			}
			return nil
		})
	}
	return resource.ComposeTestCheckFunc(checks...)
}

func iamWorkflowStatementStateCheck(state *testingterraform.State) error {
	wantValues := map[string][]string{
		iamWorkflowBasePolicyResourceName: {
			"project", "env", "allow", "CanAccessProject", "CanAccessEnv",
			"project/iam-workflow", "project/iam-workflow:env/*",
		},
		iamWorkflowScopedPolicyResourceName: {
			"flag", "segment", "allow", "deny", "ToggleFlag", "UpdateFlagName",
			"UpdateSegmentDescription", "UpdateSegmentRules", "DeleteFlag",
			"project/iam-workflow:env/dev:flag/*",
			"project/iam-workflow:env/dev:segment/*",
			"project/iam-workflow:env/prod:flag/prod-exception",
			"project/iam-workflow:env/dev:flag/protected-flag",
		},
	}
	for address, expected := range wantValues {
		resourceState, exists := state.RootModule().Resources[address]
		if !exists || resourceState.Primary == nil {
			return fmt.Errorf("IAM Policy state %s is missing", address)
		}
		values := make(map[string]struct{}, len(resourceState.Primary.Attributes))
		for _, value := range resourceState.Primary.Attributes {
			values[value] = struct{}{}
		}
		for _, value := range expected {
			if _, exists := values[value]; !exists {
				return fmt.Errorf("IAM Policy state %s did not round-trip %q", address, value)
			}
		}
	}
	return nil
}

func iamWorkflowStateSafetyCheck(state *testingterraform.State) error {
	formatted := fmt.Sprintf("%#v", state)
	for _, unsafe := range []string{
		syntheticProviderAccessToken,
		iamWorkflowInitialPasswordMarker,
		"test-only-protocol-environment-secret-marker",
	} {
		if strings.Contains(formatted, unsafe) {
			return fmt.Errorf("IAM workflow state retained an endpoint-only protected value")
		}
	}
	return nil
}

func iamWorkflowBasePolicy() client.Policy {
	return client.Policy{
		ID:          iamWorkflowBasePolicyID,
		Name:        "IAM Base Access",
		Key:         "iam-base-access",
		Description: "Project and Environment visibility",
		Type:        client.PolicyTypeCustomerManaged,
		Statements: []client.PolicyStatement{
			{
				ResourceType: "project", Effect: "allow",
				Actions: []string{"CanAccessProject"}, Resources: []string{"project/iam-workflow"},
			},
			{
				ResourceType: "env", Effect: "allow",
				Actions: []string{"CanAccessEnv"}, Resources: []string{"project/iam-workflow:env/*"},
			},
		},
	}
}

func iamWorkflowScopedPolicy() client.Policy {
	return client.Policy{
		ID:          iamWorkflowScopedPolicyID,
		Name:        "IAM Scoped Access",
		Key:         "iam-scoped-access",
		Description: "Dev operations with one prod exception",
		Type:        client.PolicyTypeCustomerManaged,
		Statements: []client.PolicyStatement{
			{
				ResourceType: "flag", Effect: "allow",
				Actions:   []string{"UpdateFlagName", "ToggleFlag"},
				Resources: []string{"project/iam-workflow:env/dev:flag/*"},
			},
			{
				ResourceType: "segment", Effect: "allow",
				Actions:   []string{"UpdateSegmentRules", "UpdateSegmentDescription"},
				Resources: []string{"project/iam-workflow:env/dev:segment/*"},
			},
			{
				ResourceType: "flag", Effect: "allow",
				Actions:   []string{"ToggleFlag"},
				Resources: []string{"project/iam-workflow:env/prod:flag/prod-exception"},
			},
			{
				ResourceType: "flag", Effect: "deny",
				Actions:   []string{"DeleteFlag"},
				Resources: []string{"project/iam-workflow:env/dev:flag/protected-flag"},
			},
		},
	}
}

type iamWorkflowFixture struct {
	t       *testing.T
	project *projectProtocolFixture
	server  *httptest.Server

	mu              sync.Mutex
	policies        map[string]client.Policy
	groups          map[string]client.Group
	groupPolicies   map[string]map[string]struct{}
	groupMembers    map[string]map[string]struct{}
	directPolicyIDs map[string]struct{}
	member          client.Member
	ownerBaseline   client.Policy
	mutations       []string
	violations      []string
}

func newIAMWorkflowFixture(t *testing.T) *iamWorkflowFixture {
	t.Helper()
	owner := client.Policy{
		ID:          iamWorkflowOwnerPolicyID,
		Name:        "Owner",
		Key:         "Owner",
		Description: "Built-in owner policy",
		Type:        client.PolicyTypeSysManaged,
		Statements: []client.PolicyStatement{{
			ResourceType: "*",
			Effect:       "allow",
			Actions:      []string{"*"},
			Resources:    []string{"*"},
		}},
	}
	fixture := &iamWorkflowFixture{
		t:       t,
		project: newProjectProtocolFixture(t),
		policies: map[string]client.Policy{
			iamWorkflowOwnerPolicyID: cloneProviderPolicy(owner),
		},
		groups:          make(map[string]client.Group),
		groupPolicies:   make(map[string]map[string]struct{}),
		groupMembers:    make(map[string]map[string]struct{}),
		directPolicyIDs: map[string]struct{}{iamWorkflowOwnerPolicyID: {}},
		member: client.Member{
			ID: iamWorkflowMemberID, Email: iamWorkflowMemberEmail, Name: "P6-100 Existing Member",
		},
		ownerBaseline: cloneProviderPolicy(owner),
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.handle))
	return fixture
}

func (f *iamWorkflowFixture) apiOrigin() string {
	return f.server.URL
}

func (f *iamWorkflowFixture) close() {
	f.server.Close()
	f.project.close()
}

func (f *iamWorkflowFixture) handle(response http.ResponseWriter, request *http.Request) {
	path := request.URL.EscapedPath()
	if path == "/api/v1/projects" || strings.HasPrefix(path, "/api/v1/projects/") {
		f.project.handle(response, request)
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.validateBaseRequestLocked(request) {
		writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
		return
	}
	if !strings.HasPrefix(path, "/api/v1/") {
		f.violateLocked("request escaped the documented API root")
		writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
		return
	}
	segments := strings.Split(strings.TrimPrefix(path, "/api/v1/"), "/")
	switch segments[0] {
	case "policies":
		f.handlePoliciesLocked(response, request, segments)
	case "groups":
		f.handleGroupsLocked(response, request, segments)
	case "members":
		f.handleMembersLocked(response, request, segments)
	default:
		f.violateLocked("request used an unexpected IAM path")
		writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
	}
}

func (f *iamWorkflowFixture) handlePoliciesLocked(
	response http.ResponseWriter,
	request *http.Request,
	segments []string,
) {
	switch {
	case len(segments) == 1 && request.Method == http.MethodGet:
		if !f.validatePageQueryLocked(request, "") {
			writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
			return
		}
		items := make([]client.Policy, 0, len(f.policies))
		for _, policy := range f.policies {
			items = append(items, cloneProviderPolicy(policy))
		}
		sort.Slice(items, func(left, right int) bool { return items[left].ID > items[right].ID })
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(items), "items": items,
		})
	case len(segments) == 1 && request.Method == http.MethodPost:
		if !f.validateJSONMutationLocked(request) {
			writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
			return
		}
		var input client.CreatePolicyRequest
		if err := decodeIAMWorkflowBody(request.Body, &input); err != nil ||
			input.Name == "" || input.Key == "" {
			f.violateLocked("Policy create body did not match the documented contract")
			writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
			return
		}
		for _, policy := range f.policies {
			if policy.Key == input.Key {
				writePolicyProviderEnvelope(f.t, response, http.StatusConflict, nil)
				return
			}
		}
		policyID := iamWorkflowPolicyIDForKey(input.Key)
		if policyID == "" {
			f.violateLocked("Policy create used an unexpected exact key")
			writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
			return
		}
		created := client.Policy{
			ID: policyID, Name: input.Name, Key: input.Key, Description: input.Description,
			Type: client.PolicyTypeCustomerManaged, Statements: []client.PolicyStatement{},
		}
		f.policies[policyID] = created
		f.mutations = append(f.mutations, "create-policy:"+input.Key)
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, cloneProviderPolicy(created))
	case len(segments) == 2 && request.Method == http.MethodGet:
		if !f.validateNoQueryLocked(request) {
			writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
			return
		}
		policy, found := f.policies[segments[1]]
		if !found {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, cloneProviderPolicy(policy))
	case len(segments) == 3 && request.Method == http.MethodGet && segments[2] == "groups":
		if !f.validatePageQueryLocked(request, "GetAllGroups") {
			writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
			return
		}
		if _, found := f.policies[segments[1]]; !found {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		groupIDs := make([]string, 0)
		for groupID, policyIDs := range f.groupPolicies {
			if _, present := policyIDs[segments[1]]; present {
				groupIDs = append(groupIDs, groupID)
			}
		}
		sort.Sort(sort.Reverse(sort.StringSlice(groupIDs)))
		items := make([]map[string]any, 0, len(groupIDs))
		for _, groupID := range groupIDs {
			items = append(items, map[string]any{"id": groupID, "isPolicyGroup": true})
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(items), "items": items,
		})
	case len(segments) == 3 && request.Method == http.MethodGet && segments[2] == "members":
		if !f.validatePageQueryLocked(request, "GetAllMembers") {
			writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
			return
		}
		if _, found := f.policies[segments[1]]; !found {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		items := []map[string]any{}
		if _, present := f.directPolicyIDs[segments[1]]; present {
			items = append(items, map[string]any{
				"id": f.member.ID, "isPolicyMember": true,
			})
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(items), "items": items,
		})
	case len(segments) == 3 && request.Method == http.MethodPut && segments[2] == "settings":
		f.updatePolicySettingsLocked(response, request, segments[1])
	case len(segments) == 3 && request.Method == http.MethodPut && segments[2] == "statements":
		f.replacePolicyStatementsLocked(response, request, segments[1])
	case len(segments) == 2 && request.Method == http.MethodDelete:
		f.deletePolicyLocked(response, request, segments[1])
	default:
		f.violateLocked("request used an unexpected Policy operation")
		writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
	}
}

func (f *iamWorkflowFixture) updatePolicySettingsLocked(
	response http.ResponseWriter,
	request *http.Request,
	policyID string,
) {
	if policyID == iamWorkflowOwnerPolicyID {
		f.violateLocked("provider attempted to mutate the built-in Owner Policy settings")
		writePolicyProviderEnvelope(f.t, response, http.StatusUnprocessableEntity, nil)
		return
	}
	if !f.validateJSONMutationLocked(request) {
		writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
		return
	}
	var input client.UpdatePolicySettingsRequest
	if err := decodeIAMWorkflowBody(request.Body, &input); err != nil || input.Name == "" {
		f.violateLocked("Policy settings body did not match the documented contract")
		writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
		return
	}
	policy, found := f.policies[policyID]
	if !found {
		writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
		return
	}
	policy.Name = input.Name
	policy.Description = input.Description
	f.policies[policyID] = policy
	f.mutations = append(f.mutations, "update-policy-settings:"+policyID)
	writePolicyProviderEnvelope(f.t, response, http.StatusOK, cloneProviderPolicy(policy))
}

func (f *iamWorkflowFixture) replacePolicyStatementsLocked(
	response http.ResponseWriter,
	request *http.Request,
	policyID string,
) {
	if policyID == iamWorkflowOwnerPolicyID {
		f.violateLocked("provider attempted to mutate the built-in Owner Policy statements")
		writePolicyProviderEnvelope(f.t, response, http.StatusUnprocessableEntity, nil)
		return
	}
	if !f.validateJSONMutationLocked(request) {
		writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
		return
	}
	var statements []client.PolicyStatement
	if err := decodeIAMWorkflowBody(request.Body, &statements); err != nil || statements == nil {
		f.violateLocked("Policy statement body did not match the documented contract")
		writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
		return
	}
	policy, found := f.policies[policyID]
	if !found {
		writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
		return
	}
	policy.Statements = reverseIAMWorkflowStatementOrder(statements)
	f.policies[policyID] = policy
	f.mutations = append(f.mutations, "replace-policy-statements:"+policyID)
	writePolicyProviderEnvelope(f.t, response, http.StatusOK, cloneProviderPolicy(policy))
}

func (f *iamWorkflowFixture) deletePolicyLocked(
	response http.ResponseWriter,
	request *http.Request,
	policyID string,
) {
	if policyID == iamWorkflowOwnerPolicyID {
		f.violateLocked("provider attempted to delete the built-in Owner Policy")
		writePolicyProviderEnvelope(f.t, response, http.StatusForbidden, nil)
		return
	}
	if !f.validateEmptyMutationLocked(request) {
		writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
		return
	}
	if _, found := f.policies[policyID]; !found {
		writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
		return
	}
	if f.policyHasAssociationsLocked(policyID) {
		f.violateLocked("Policy delete attempted to cascade a live relationship")
		writePolicyProviderEnvelope(f.t, response, http.StatusConflict, nil)
		return
	}
	delete(f.policies, policyID)
	f.mutations = append(f.mutations, "delete-policy:"+policyID)
	writePolicyProviderEnvelope(f.t, response, http.StatusOK, true)
}

func (f *iamWorkflowFixture) handleGroupsLocked(
	response http.ResponseWriter,
	request *http.Request,
	segments []string,
) {
	switch {
	case len(segments) == 1 && request.Method == http.MethodGet:
		if !f.validatePageQueryLocked(request, "") {
			writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
			return
		}
		items := make([]client.Group, 0, len(f.groups))
		for _, group := range f.groups {
			items = append(items, group)
		}
		sort.Slice(items, func(left, right int) bool { return items[left].ID > items[right].ID })
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(items), "items": items,
		})
	case len(segments) == 1 && request.Method == http.MethodPost:
		f.createGroupLocked(response, request)
	case len(segments) == 2 && request.Method == http.MethodGet:
		if !f.validateNoQueryLocked(request) {
			writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
			return
		}
		group, found := f.groups[segments[1]]
		if !found {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, group)
	case len(segments) == 2 && request.Method == http.MethodPut:
		f.updateGroupLocked(response, request, segments[1])
	case len(segments) == 2 && request.Method == http.MethodDelete:
		f.deleteGroupLocked(response, request, segments[1])
	case len(segments) == 3 && request.Method == http.MethodGet && segments[2] == "policies":
		f.listGroupAssociationsLocked(response, request, segments[1], true)
	case len(segments) == 3 && request.Method == http.MethodGet && segments[2] == "members":
		f.listGroupAssociationsLocked(response, request, segments[1], false)
	case len(segments) == 4 && request.Method == http.MethodPut:
		f.mutateGroupAssociationLocked(response, request, segments)
	default:
		f.violateLocked("request used an unexpected Group operation")
		writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
	}
}

func (f *iamWorkflowFixture) createGroupLocked(
	response http.ResponseWriter,
	request *http.Request,
) {
	if !f.validateJSONMutationLocked(request) {
		writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
		return
	}
	var input client.CreateGroupRequest
	if err := decodeIAMWorkflowBody(request.Body, &input); err != nil || input.Name == "" {
		f.violateLocked("Group create body did not match the documented contract")
		writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
		return
	}
	for _, group := range f.groups {
		if group.Name == input.Name {
			writePolicyProviderEnvelope(f.t, response, http.StatusConflict, nil)
			return
		}
	}
	groupID := iamWorkflowGroupIDForName(input.Name)
	if groupID == "" {
		f.violateLocked("Group create used an unexpected exact name")
		writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
		return
	}
	group := client.Group{ID: groupID, Name: input.Name, Description: input.Description}
	f.groups[groupID] = group
	f.groupPolicies[groupID] = make(map[string]struct{})
	f.groupMembers[groupID] = make(map[string]struct{})
	f.mutations = append(f.mutations, "create-group:"+input.Name)
	writePolicyProviderEnvelope(f.t, response, http.StatusOK, group)
}

func (f *iamWorkflowFixture) updateGroupLocked(
	response http.ResponseWriter,
	request *http.Request,
	groupID string,
) {
	if !f.validateJSONMutationLocked(request) {
		writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
		return
	}
	var input client.UpdateGroupRequest
	if err := decodeIAMWorkflowBody(request.Body, &input); err != nil || input.Name == "" {
		f.violateLocked("Group settings body did not match the documented contract")
		writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
		return
	}
	group, found := f.groups[groupID]
	if !found {
		writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
		return
	}
	group.Name = input.Name
	group.Description = input.Description
	f.groups[groupID] = group
	f.mutations = append(f.mutations, "update-group:"+groupID)
	writePolicyProviderEnvelope(f.t, response, http.StatusOK, group)
}

func (f *iamWorkflowFixture) deleteGroupLocked(
	response http.ResponseWriter,
	request *http.Request,
	groupID string,
) {
	if !f.validateEmptyMutationLocked(request) {
		writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
		return
	}
	if _, found := f.groups[groupID]; !found {
		writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
		return
	}
	if len(f.groupPolicies[groupID]) != 0 || len(f.groupMembers[groupID]) != 0 {
		f.violateLocked("Group delete attempted to cascade a live relationship")
		writePolicyProviderEnvelope(f.t, response, http.StatusConflict, nil)
		return
	}
	delete(f.groups, groupID)
	delete(f.groupPolicies, groupID)
	delete(f.groupMembers, groupID)
	f.mutations = append(f.mutations, "delete-group:"+groupID)
	writePolicyProviderEnvelope(f.t, response, http.StatusOK, true)
}

func (f *iamWorkflowFixture) listGroupAssociationsLocked(
	response http.ResponseWriter,
	request *http.Request,
	groupID string,
	policies bool,
) {
	queryName := "GetAllMembers"
	if policies {
		queryName = "GetAllPolicies"
	}
	if !f.validatePageQueryLocked(request, queryName) {
		writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
		return
	}
	if _, found := f.groups[groupID]; !found {
		writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
		return
	}
	ids := f.groupMembers[groupID]
	membershipName := "isGroupMember"
	if policies {
		ids = f.groupPolicies[groupID]
		membershipName = "isGroupPolicy"
	}
	sorted := sortedIAMWorkflowIDs(ids)
	items := make([]map[string]any, 0, len(sorted))
	for _, id := range sorted {
		items = append(items, map[string]any{"id": id, membershipName: true})
	}
	writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
		"totalCount": len(items), "items": items,
	})
}

func (f *iamWorkflowFixture) mutateGroupAssociationLocked(
	response http.ResponseWriter,
	request *http.Request,
	segments []string,
) {
	if !f.validateEmptyMutationLocked(request) {
		writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
		return
	}
	groupID, operation, targetID := segments[1], segments[2], segments[3]
	if _, found := f.groups[groupID]; !found {
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, true)
		return
	}
	var targetSet map[string]struct{}
	var targetExists bool
	switch operation {
	case "add-policy", "remove-policy":
		targetSet = f.groupPolicies[groupID]
		_, targetExists = f.policies[targetID]
	case "add-member", "remove-member":
		targetSet = f.groupMembers[groupID]
		targetExists = client.EqualUUID(targetID, f.member.ID)
	default:
		f.violateLocked("Group relationship used an unexpected operation")
		writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
		return
	}
	if !targetExists {
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, true)
		return
	}
	prefix := strings.TrimSuffix(operation, "-policy")
	prefix = strings.TrimSuffix(prefix, "-member")
	kind := "group-policy"
	if strings.HasSuffix(operation, "member") {
		kind = "group-member"
	}
	if strings.HasPrefix(operation, "add-") {
		targetSet[targetID] = struct{}{}
	} else {
		delete(targetSet, targetID)
	}
	f.mutations = append(f.mutations, prefix+"-"+kind+":"+groupID+":"+targetID)
	writePolicyProviderEnvelope(f.t, response, http.StatusOK, true)
}

func (f *iamWorkflowFixture) handleMembersLocked(
	response http.ResponseWriter,
	request *http.Request,
	segments []string,
) {
	switch {
	case len(segments) == 1 && request.Method == http.MethodGet:
		if !f.validatePageQueryLocked(request, "") {
			writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
			return
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": 1,
			"items":      []map[string]any{f.memberResponseLocked()},
		})
	case len(segments) == 2 && request.Method == http.MethodGet:
		if !f.validateNoQueryLocked(request) {
			writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
			return
		}
		if !client.EqualUUID(segments[1], f.member.ID) {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, f.memberResponseLocked())
	case len(segments) == 3 && request.Method == http.MethodGet && segments[2] == "direct-policies":
		if !f.validatePageQueryLocked(request, "GetAllPolicies") {
			writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
			return
		}
		if !client.EqualUUID(segments[1], f.member.ID) {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		ids := sortedIAMWorkflowIDs(f.directPolicyIDs)
		items := make([]map[string]any, 0, len(ids))
		for _, policyID := range ids {
			items = append(items, map[string]any{
				"id": policyID, "isMemberPolicy": true,
				"name": "endpoint-only relationship display value",
			})
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(items), "items": items,
		})
	case len(segments) == 4 && request.Method == http.MethodPut &&
		(segments[2] == "add-policy" || segments[2] == "remove-policy"):
		f.mutateMemberPolicyLocked(response, request, segments)
	default:
		f.violateLocked("provider attempted an unsupported Member operation")
		writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
	}
}

func (f *iamWorkflowFixture) mutateMemberPolicyLocked(
	response http.ResponseWriter,
	request *http.Request,
	segments []string,
) {
	if !f.validateEmptyMutationLocked(request) {
		writePolicyProviderEnvelope(f.t, response, http.StatusBadRequest, nil)
		return
	}
	memberID, operation, policyID := segments[1], segments[2], segments[3]
	if !client.EqualUUID(memberID, f.member.ID) {
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, true)
		return
	}
	if _, found := f.policies[policyID]; !found {
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, true)
		return
	}
	verb := "add"
	if operation == "remove-policy" {
		verb = "remove"
		delete(f.directPolicyIDs, policyID)
	} else {
		f.directPolicyIDs[policyID] = struct{}{}
	}
	f.mutations = append(f.mutations, verb+"-member-policy:"+memberID+":"+policyID)
	writePolicyProviderEnvelope(f.t, response, http.StatusOK, true)
}

func (f *iamWorkflowFixture) memberResponseLocked() map[string]any {
	return map[string]any{
		"id":               f.member.ID,
		"email":            f.member.Email,
		"name":             f.member.Name,
		"initialPassword":  iamWorkflowInitialPasswordMarker,
		"invitationStatus": "endpoint-only",
	}
}

func (f *iamWorkflowFixture) validateBaseRequestLocked(request *http.Request) bool {
	valid := true
	if request.Header.Get("Authorization") != syntheticProviderAccessToken {
		f.violateLocked("IAM request did not use the configured direct access token")
		valid = false
	}
	if request.Header.Get("User-Agent") != "terraform-provider-featbit/protocol-test" {
		f.violateLocked("IAM request used an unexpected User-Agent")
		valid = false
	}
	if request.Header.Get("Organization") != "" || request.Header.Get("Workspace") != "" {
		f.violateLocked("IAM request contained a caller-supplied context header")
		valid = false
	}
	return valid
}

func (f *iamWorkflowFixture) validatePageQueryLocked(
	request *http.Request,
	membershipQuery string,
) bool {
	query := request.URL.Query()
	wantValues := 2
	valid := query.Get("PageIndex") == "0" && query.Get("PageSize") == "100"
	if membershipQuery != "" {
		wantValues++
		valid = valid && query.Get(membershipQuery) == "false"
	}
	if !valid || len(query) != wantValues {
		f.violateLocked("IAM collection request did not use the complete pagination contract")
		return false
	}
	return true
}

func (f *iamWorkflowFixture) validateNoQueryLocked(request *http.Request) bool {
	if request.URL.RawQuery != "" {
		f.violateLocked("exact IAM request contained a query")
		return false
	}
	return true
}

func (f *iamWorkflowFixture) validateJSONMutationLocked(request *http.Request) bool {
	valid := f.validateNoQueryLocked(request)
	if !strings.HasPrefix(request.Header.Get("Content-Type"), "application/json") {
		f.violateLocked("IAM JSON mutation omitted application/json Content-Type")
		valid = false
	}
	return valid
}

func (f *iamWorkflowFixture) validateEmptyMutationLocked(request *http.Request) bool {
	valid := f.validateNoQueryLocked(request)
	if request.Body != nil && request.Body != http.NoBody {
		body, err := io.ReadAll(request.Body)
		if err != nil || len(body) != 0 {
			f.violateLocked("IAM relationship or delete mutation contained a body")
			valid = false
		}
	}
	return valid
}

func (f *iamWorkflowFixture) violateLocked(detail string) {
	f.violations = append(f.violations, detail)
}

func (f *iamWorkflowFixture) policyHasAssociationsLocked(policyID string) bool {
	if _, present := f.directPolicyIDs[policyID]; present {
		return true
	}
	for _, policies := range f.groupPolicies {
		if _, present := policies[policyID]; present {
			return true
		}
	}
	return false
}

func (f *iamWorkflowFixture) graphCheck(includeBaseBinding bool) resource.TestCheckFunc {
	return func(*testingterraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.policies) != 3 {
			return fmt.Errorf("IAM workflow Policy count = %d, want built-in plus two custom", len(f.policies))
		}
		for _, expected := range []client.Policy{iamWorkflowBasePolicy(), iamWorkflowScopedPolicy()} {
			remote, found := f.policies[expected.ID]
			if !found {
				return fmt.Errorf("IAM workflow custom Policy %s is absent", expected.Key)
			}
			canonicalRemote, remoteErr := canonicalizeRemoteManagedPolicy(remote)
			canonicalExpected, expectedErr := canonicalizeRemoteManagedPolicy(expected)
			if remoteErr != nil || expectedErr != nil ||
				!samePolicyDefinition(canonicalRemote, canonicalExpected) {
				return fmt.Errorf("IAM workflow custom Policy %s did not converge", expected.Key)
			}
		}
		if owner := f.policies[iamWorkflowOwnerPolicyID]; !reflect.DeepEqual(owner, f.ownerBaseline) {
			return fmt.Errorf("IAM workflow mutated the built-in Owner Policy")
		}
		if len(f.groups) != 2 || f.groups[iamWorkflowAdminGroupID] != (client.Group{
			ID: iamWorkflowAdminGroupID, Name: "IAM Administrators", Description: "Owner access",
		}) || f.groups[iamWorkflowDeveloperID] != (client.Group{
			ID: iamWorkflowDeveloperID, Name: "IAM Developers", Description: "Scoped developer access",
		}) {
			return fmt.Errorf("IAM workflow Groups did not converge to their owned settings")
		}
		if !sameIAMWorkflowIDSet(
			f.groupPolicies[iamWorkflowAdminGroupID],
			[]string{iamWorkflowOwnerPolicyID},
		) {
			return fmt.Errorf("admin Group does not own exactly the Owner binding")
		}
		developerPolicies := []string{iamWorkflowScopedPolicyID}
		if includeBaseBinding {
			developerPolicies = append(developerPolicies, iamWorkflowBasePolicyID)
		}
		if !sameIAMWorkflowIDSet(f.groupPolicies[iamWorkflowDeveloperID], developerPolicies) {
			return fmt.Errorf("developer Group Policy bindings did not preserve exact-pair ownership")
		}
		if len(f.groupMembers[iamWorkflowAdminGroupID]) != 0 ||
			!sameIAMWorkflowIDSet(
				f.groupMembers[iamWorkflowDeveloperID],
				[]string{iamWorkflowMemberID},
			) {
			return fmt.Errorf("existing Member was not assigned only to the developer Group")
		}
		if !sameIAMWorkflowIDSet(
			f.directPolicyIDs,
			[]string{iamWorkflowOwnerPolicyID, iamWorkflowBasePolicyID},
		) {
			return fmt.Errorf("additive Member-Policy binding did not preserve the existing direct Policy")
		}
		if f.member != (client.Member{
			ID: iamWorkflowMemberID, Email: iamWorkflowMemberEmail, Name: "P6-100 Existing Member",
		}) {
			return fmt.Errorf("IAM workflow changed the existing Member profile")
		}
		return nil
	}
}

func (f *iamWorkflowFixture) initialMutationCheck() resource.TestCheckFunc {
	return func(*testingterraform.State) error {
		expected := []string{
			"create-policy:iam-base-access",
			"replace-policy-statements:" + iamWorkflowBasePolicyID,
			"create-policy:iam-scoped-access",
			"replace-policy-statements:" + iamWorkflowScopedPolicyID,
			"create-group:IAM Administrators",
			"create-group:IAM Developers",
			"add-group-policy:" + iamWorkflowAdminGroupID + ":" + iamWorkflowOwnerPolicyID,
			"add-group-policy:" + iamWorkflowDeveloperID + ":" + iamWorkflowBasePolicyID,
			"add-group-policy:" + iamWorkflowDeveloperID + ":" + iamWorkflowScopedPolicyID,
			"add-group-member:" + iamWorkflowDeveloperID + ":" + iamWorkflowMemberID,
			"add-member-policy:" + iamWorkflowMemberID + ":" + iamWorkflowBasePolicyID,
		}
		actual := f.mutationSnapshot()
		if !sameIAMWorkflowMutationSet(actual, expected) {
			return fmt.Errorf("initial IAM workflow mutations = %v, want %v", actual, expected)
		}
		return nil
	}
}

func (f *iamWorkflowFixture) exactRemovalMutationCheck(baseline *int) resource.TestCheckFunc {
	return f.mutationDeltaCheck(baseline, []string{
		"remove-group-policy:" + iamWorkflowDeveloperID + ":" + iamWorkflowBasePolicyID,
	})
}

func (f *iamWorkflowFixture) mutationDeltaCheck(
	baseline *int,
	expected []string,
) resource.TestCheckFunc {
	return func(*testingterraform.State) error {
		mutations := f.mutationSnapshot()
		if *baseline < 0 || *baseline > len(mutations) {
			return fmt.Errorf("IAM mutation baseline is invalid")
		}
		actual := mutations[*baseline:]
		if !sameIAMWorkflowMutationSet(actual, expected) {
			return fmt.Errorf("IAM mutation delta = %v, want %v", actual, expected)
		}
		return nil
	}
}

func (f *iamWorkflowFixture) mutationSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.mutations...)
}

func (f *iamWorkflowFixture) mutationCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.mutations)
}

func (f *iamWorkflowFixture) cleanupError() error {
	f.mu.Lock()
	policyCount := len(f.policies)
	owner, ownerPresent := f.policies[iamWorkflowOwnerPolicyID]
	groupCount := len(f.groups)
	directPolicyIDs := sortedIAMWorkflowIDs(f.directPolicyIDs)
	relationshipCount := 0
	for _, ids := range f.groupPolicies {
		relationshipCount += len(ids)
	}
	for _, ids := range f.groupMembers {
		relationshipCount += len(ids)
	}
	violations := append([]string(nil), f.violations...)
	mutations := append([]string(nil), f.mutations...)
	member := f.member
	f.mu.Unlock()

	if policyCount != 1 || !ownerPresent || !reflect.DeepEqual(owner, f.ownerBaseline) {
		return fmt.Errorf("IAM workflow cleanup did not preserve exactly the immutable Owner Policy")
	}
	if groupCount != 0 || relationshipCount != 0 {
		return fmt.Errorf("IAM workflow cleanup retained %d Groups and %d relationships", groupCount, relationshipCount)
	}
	if !slices.Equal(directPolicyIDs, []string{iamWorkflowOwnerPolicyID}) {
		return fmt.Errorf("IAM workflow cleanup did not restore the Member direct Policy baseline")
	}
	if member != (client.Member{
		ID: iamWorkflowMemberID, Email: iamWorkflowMemberEmail, Name: "P6-100 Existing Member",
	}) {
		return fmt.Errorf("IAM workflow cleanup changed the existing Member")
	}
	if f.project.projectCount() != 0 || f.project.environmentCount() != 0 {
		return fmt.Errorf("IAM workflow cleanup retained its test-owned Project or Environments")
	}
	if len(violations) != 0 {
		return fmt.Errorf("IAM workflow fixture request violations = %v", violations)
	}
	if projectViolations := f.project.violationSnapshot(); len(projectViolations) != 0 {
		return fmt.Errorf("IAM workflow Project fixture request violations = %v", projectViolations)
	}
	for _, mutation := range mutations {
		if mutation == "update-policy-settings:"+iamWorkflowOwnerPolicyID ||
			mutation == "replace-policy-statements:"+iamWorkflowOwnerPolicyID ||
			mutation == "delete-policy:"+iamWorkflowOwnerPolicyID {
			return fmt.Errorf("IAM workflow sent a built-in Owner Policy mutation")
		}
	}
	projectMutations := make([]projectFixtureRequest, 0, 2)
	for _, request := range f.project.requestSnapshot() {
		if request.Method != http.MethodGet {
			projectMutations = append(projectMutations, request)
		}
	}
	if len(projectMutations) != 2 || projectMutations[0].Method != http.MethodPost ||
		projectMutations[0].Path != "/api/v1/projects" ||
		projectMutations[1].Method != http.MethodDelete ||
		strings.Contains(projectMutations[1].Path, "/envs") {
		return fmt.Errorf("IAM workflow Project ownership mutations = %v", projectMutations)
	}
	return nil
}

func iamWorkflowPolicyIDForKey(key string) string {
	switch key {
	case "iam-base-access":
		return iamWorkflowBasePolicyID
	case "iam-scoped-access":
		return iamWorkflowScopedPolicyID
	default:
		return ""
	}
}

func iamWorkflowGroupIDForName(name string) string {
	switch name {
	case "IAM Administrators":
		return iamWorkflowAdminGroupID
	case "IAM Developers":
		return iamWorkflowDeveloperID
	default:
		return ""
	}
}

func reverseIAMWorkflowStatementOrder(
	statements []client.PolicyStatement,
) []client.PolicyStatement {
	reversed := make([]client.PolicyStatement, 0, len(statements))
	for _, statement := range statements {
		cloned := client.PolicyStatement{
			ResourceType: statement.ResourceType,
			Effect:       statement.Effect,
			Actions:      append([]string(nil), statement.Actions...),
			Resources:    append([]string(nil), statement.Resources...),
		}
		slices.Reverse(cloned.Actions)
		slices.Reverse(cloned.Resources)
		reversed = append(reversed, cloned)
	}
	slices.Reverse(reversed)
	return reversed
}

func sortedIAMWorkflowIDs(ids map[string]struct{}) []string {
	result := make([]string, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(result)))
	return result
}

func sameIAMWorkflowIDSet(actual map[string]struct{}, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for _, id := range expected {
		if _, present := actual[id]; !present {
			return false
		}
	}
	return true
}

func sameIAMWorkflowMutationSet(actual []string, expected []string) bool {
	actual = append([]string(nil), actual...)
	expected = append([]string(nil), expected...)
	slices.Sort(actual)
	slices.Sort(expected)
	return slices.Equal(actual, expected)
}

func decodeIAMWorkflowBody(body io.ReadCloser, destination any) error {
	if body == nil {
		return fmt.Errorf("request body is missing")
	}
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("request body contains trailing JSON")
	}
	return nil
}
