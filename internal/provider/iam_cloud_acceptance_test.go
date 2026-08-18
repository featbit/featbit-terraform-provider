// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	testingterraform "github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	cloudIAMAdminTokenEnv      = "FEATBIT_TEST_SERVICE_TOKEN"
	cloudIAMMemberTokenEnv     = "FEATBIT_TEST_MEMBER_TOKEN"
	cloudIAMMemberIDEnv        = "FEATBIT_TEST_MEMBER_ID"
	cloudIAMTerraformMemberVar = "TF_VAR_featbit_test_member_id"

	cloudIAMProjectResource       = "featbit_project.iam"
	cloudIAMProjectData           = "data.featbit_project.iam"
	cloudIAMDevEnvironmentData    = "data.featbit_environment.dev"
	cloudIAMProdEnvironmentData   = "data.featbit_environment.prod"
	cloudIAMDevFlagResource       = "featbit_feature_flag.dev"
	cloudIAMProdAllowedResource   = "featbit_feature_flag.prod_allowed"
	cloudIAMProdDeniedResource    = "featbit_feature_flag.prod_denied"
	cloudIAMSegmentResource       = "featbit_segment.dev"
	cloudIAMOwnerPolicyData       = "data.featbit_policy.owner"
	cloudIAMBasePolicyResource    = "featbit_policy.base"
	cloudIAMScopedPolicyResource  = "featbit_policy.scoped"
	cloudIAMAdminGroupResource    = "featbit_group.admin"
	cloudIAMDeveloperResource     = "featbit_group.developer"
	cloudIAMAdminGroupData        = "data.featbit_group.admin"
	cloudIAMDeveloperGroupData    = "data.featbit_group.developer"
	cloudIAMMemberData            = "data.featbit_member.developer"
	cloudIAMOwnerBindingResource  = "featbit_group_policy_binding.admin_owner"
	cloudIAMBaseBindingResource   = "featbit_group_policy_binding.developer_base"
	cloudIAMScopedBindingResource = "featbit_group_policy_binding.developer_scoped"
	cloudIAMMemberBindingResource = "featbit_group_member_binding.developer"
	cloudIAMDirectResource        = "featbit_member_direct_policies.developer"
)

type cloudIAMDefinition struct {
	projectKey      string
	ownerPolicyKey  string
	devFlagKey      string
	prodAllowedKey  string
	prodDeniedKey   string
	segmentKey      string
	basePolicyKey   string
	scopedPolicyKey string
	adminGroupName  string
	developerName   string
}

func TestAccIAMCloudCustomerWorkflowEffectiveAccessAndCleanup(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("set TF_ACC=1 to run trusted FeatBit Cloud IAM acceptance")
	}
	adminToken, ok := os.LookupEnv(cloudIAMAdminTokenEnv)
	if !ok || adminToken == "" {
		t.Skip("trusted FeatBit Cloud IAM acceptance requires FEATBIT_TEST_SERVICE_TOKEN")
	}
	memberToken, ok := os.LookupEnv(cloudIAMMemberTokenEnv)
	if !ok || memberToken == "" {
		t.Skip("trusted FeatBit Cloud IAM acceptance requires FEATBIT_TEST_MEMBER_TOKEN")
	}
	if adminToken == memberToken {
		t.Skip("trusted FeatBit Cloud IAM acceptance requires distinct admin and Member credentials")
	}
	memberID, ok := os.LookupEnv(cloudIAMMemberIDEnv)
	canonicalMemberID, validMemberID := client.CanonicalUUID(memberID)
	if !ok || !validMemberID {
		t.Skip("trusted FeatBit Cloud IAM acceptance requires FEATBIT_TEST_MEMBER_ID")
	}
	organizationKey := cloudSegmentAcceptanceOrganizationKey(t)
	apiURL := cloudAcceptanceAPIURL(t)
	t.Setenv(envAccessToken, adminToken)
	admin := newCloudIAMClient(t, apiURL.String(), adminToken, "cloud-iam-acceptance")
	restricted := newCloudIAMClient(t, apiURL.String(), memberToken, "cloud-iam-effective-access")

	ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
	member, memberFound, err := admin.GetMember(ctx, canonicalMemberID)
	if err != nil || !memberFound || !client.EqualUUID(member.ID, canonicalMemberID) {
		cancel()
		t.Skip("trusted Cloud IAM acceptance requires an exact pre-authorized Member")
	}
	baselineDirect, err := admin.ListMemberDirectPolicyIDs(ctx, canonicalMemberID)
	if err != nil {
		cancel()
		t.Skip("trusted Cloud IAM acceptance requires a readable direct-Policy baseline")
	}
	owner, ownerFound, err := admin.GetPolicyByKey(ctx, "owner")
	if err != nil || !ownerFound || owner.Type != client.PolicyTypeSysManaged {
		cancel()
		t.Skip("trusted Cloud IAM acceptance requires the exact built-in Owner Policy")
	}
	if err := cloudIAMVerifyNoEffectiveGroupPolicies(ctx, admin, canonicalMemberID, ""); err != nil {
		cancel()
		t.Skip("trusted Cloud IAM acceptance Member has another effective Group Policy")
	}
	cancel()

	prefix := strings.Replace(cloudAcceptancePrefix(t), "tfacc-pe-", "tfacc-iam-", 1)
	definition := cloudIAMDefinition{
		projectKey:      prefix + "-project",
		ownerPolicyKey:  owner.Key,
		devFlagKey:      prefix + "-dev-flag",
		prodAllowedKey:  prefix + "-prod-allowed",
		prodDeniedKey:   prefix + "-prod-denied",
		segmentKey:      prefix + "-segment",
		basePolicyKey:   prefix + "-base",
		scopedPolicyKey: prefix + "-scoped",
		adminGroupName:  prefix + " administrators",
		developerName:   prefix + " developers",
	}
	children := newCloudSegmentInventory(
		admin,
		[]string{definition.projectKey},
		[]string{"dev", "prod"},
		[]string{definition.devFlagKey, definition.prodAllowedKey, definition.prodDeniedKey},
		[]string{definition.segmentKey},
	)
	inventory, err := newCloudIAMInventory(
		admin,
		children,
		canonicalMemberID,
		baselineDirect,
		owner,
		definition.ownerPolicyKey,
		definition.basePolicyKey,
		definition.scopedPolicyKey,
		definition.adminGroupName,
		definition.developerName,
	)
	if err != nil {
		t.Fatal("could not construct the trusted Cloud IAM cleanup inventory")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
		defer cancel()
		if err := inventory.cleanupAndVerify(ctx); err != nil {
			t.Error("trusted Cloud IAM acceptance cleanup did not restore every owned boundary")
		}
	})
	t.Setenv(cloudIAMTerraformMemberVar, canonicalMemberID)

	fullConfig := cloudIAMConfig(apiURL.String(), organizationKey, definition, true)
	detachedConfig := cloudIAMConfig(apiURL.String(), organizationKey, definition, false)
	runtime := &cloudIAMRuntime{}
	effectiveDefinition := cloudIAMEffectiveDefinitions{
		projectKey:         definition.projectKey,
		devFlagKey:         definition.devFlagKey,
		devFlagName:        "Terraform IAM Dev Flag",
		prodAllowedKey:     definition.prodAllowedKey,
		prodAllowedName:    "Terraform IAM Prod Allowed Flag",
		prodDeniedKey:      definition.prodDeniedKey,
		prodDeniedName:     "Terraform IAM Prod Denied Flag",
		segmentDescription: "Terraform IAM dev Segment",
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"featbit": providerserver.NewProtocol6WithError(New("cloud-iam-acceptance")()),
		},
		PreCheck: func() {
			cloudIAMAcceptancePreCheck(t)
		},
		CheckDestroy: func(*testingterraform.State) error {
			ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
			defer cancel()
			return inventory.cleanupAndVerify(ctx)
		},
		Steps: []resource.TestStep{
			{
				Config: fullConfig,
				Check: resource.ComposeTestCheckFunc(
					cloudIAMStateChecks(definition, true),
					cloudIAMCaptureRuntime(children, runtime, definition),
					cloudIAMRemoteGraphCheck(admin, inventory, runtime, definition, true),
					cloudIAMEffectiveCheck(admin, restricted, runtime, effectiveDefinition, true),
				),
			},
			{
				Config:   fullConfig,
				PlanOnly: true,
			},
			{
				Config: detachedConfig,
				Check: resource.ComposeTestCheckFunc(
					cloudIAMStateChecks(definition, false),
					cloudIAMCaptureRuntime(children, runtime, definition),
					cloudIAMRemoteGraphCheck(admin, inventory, runtime, definition, false),
					cloudIAMEffectiveCheck(admin, restricted, runtime, effectiveDefinition, false),
				),
			},
			{
				Config: fullConfig,
				Check: resource.ComposeTestCheckFunc(
					cloudIAMStateChecks(definition, true),
					cloudIAMCaptureRuntime(children, runtime, definition),
					cloudIAMRemoteGraphCheck(admin, inventory, runtime, definition, true),
					cloudIAMEffectiveCheck(admin, restricted, runtime, effectiveDefinition, true),
				),
			},
			{
				Config:   fullConfig,
				PlanOnly: true,
			},
		},
	})
}

func newCloudIAMClient(t *testing.T, apiOrigin string, token string, version string) *client.Client {
	t.Helper()
	apiURL, err := parseAPIURL(apiOrigin)
	if err != nil {
		t.Fatal("could not parse the trusted Cloud IAM API origin")
	}
	apiClient, err := client.New(apiURL, token, client.Options{
		HTTPTimeout:     client.DefaultHTTPTimeout,
		MaxConcurrency:  client.DefaultMaxConcurrency,
		MaxRetries:      client.DefaultMaxRetries,
		ProviderVersion: version,
	})
	if err != nil {
		t.Fatal("could not construct a trusted Cloud IAM client")
	}
	return apiClient
}

func cloudIAMAcceptancePreCheck(t *testing.T) {
	t.Helper()
	cloudSegmentAcceptancePreCheck(t)
	for _, name := range []string{
		cloudIAMAdminTokenEnv,
		cloudIAMMemberTokenEnv,
		cloudIAMMemberIDEnv,
	} {
		if value, ok := os.LookupEnv(name); !ok || value == "" {
			t.Skip("trusted Cloud IAM acceptance requires its out-of-band test fixture")
		}
	}
	if _, valid := client.CanonicalUUID(os.Getenv(cloudIAMMemberIDEnv)); !valid {
		t.Skip("trusted Cloud IAM acceptance requires a valid exact Member UUID")
	}
}

func cloudIAMConfig(
	apiOrigin string,
	organizationKey string,
	definition cloudIAMDefinition,
	includeMemberBinding bool,
) string {
	memberBinding := ""
	directDependency := ""
	if includeMemberBinding {
		memberBinding = `
resource "featbit_group_member_binding" "developer" {
  group_id  = data.featbit_group.developer.id
  member_id = data.featbit_member.developer.id
}
`
		directDependency = "\n  depends_on = [featbit_group_member_binding.developer]"
	}
	return fmt.Sprintf(`
variable "featbit_test_member_id" {
  type      = string
  sensitive = true
}

provider "featbit" {
  api_url              = %q
  http_timeout_seconds = 30
  max_concurrency      = 2
  max_retries          = 3
}

resource "featbit_project" "iam" {
  name = "Terraform IAM Cloud Acceptance"
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

resource "featbit_feature_flag" "dev" {
  environment_id = data.featbit_environment.dev.id
  name           = "Terraform IAM Dev Flag"
  description    = "Terraform IAM effective-access dev flag"
  key            = %q
  variation_type = "boolean"
  variations = [
    { name = "Enabled", value = "true" },
    { name = "Disabled", value = "false" },
  ]
}

resource "featbit_feature_flag" "prod_allowed" {
  environment_id = data.featbit_environment.prod.id
  name           = "Terraform IAM Prod Allowed Flag"
  description    = "Terraform IAM exact prod exception"
  key            = %q
  variation_type = "boolean"
  variations = [
    { name = "Enabled", value = "true" },
    { name = "Disabled", value = "false" },
  ]
}

resource "featbit_feature_flag" "prod_denied" {
  environment_id = data.featbit_environment.prod.id
  name           = "Terraform IAM Prod Denied Flag"
  description    = "Terraform IAM forbidden prod peer"
  key            = %q
  variation_type = "boolean"
  variations = [
    { name = "Enabled", value = "true" },
    { name = "Disabled", value = "false" },
  ]
}

resource "featbit_segment" "dev" {
  environment_id = data.featbit_environment.dev.id
  name           = "Terraform IAM Dev Segment"
  key            = %q
  description    = "Terraform IAM dev Segment"
  scopes = [%q]
  included_users = []
  excluded_users = []
  rules          = []
  tags           = []
}

data "featbit_policy" "owner" {
  key = %q
}

resource "featbit_policy" "base" {
  name        = "Terraform IAM Base Access"
  key         = %q
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

resource "featbit_policy" "scoped" {
  name        = "Terraform IAM Scoped Access"
  key         = %q
  description = "Dev metadata with one exact prod exception"
  statements = [
    {
      resource_type = "flag"
      effect        = "allow"
      actions       = ["UpdateFlagName"]
      resources     = ["project/${data.featbit_project.iam.key}:env/${data.featbit_environment.dev.key}:flag/*"]
    },
    {
      resource_type = "segment"
      effect        = "allow"
      actions       = ["UpdateSegmentDescription"]
      resources     = ["project/${data.featbit_project.iam.key}:env/${data.featbit_environment.dev.key}:segment/*"]
    },
    {
      resource_type = "flag"
      effect        = "allow"
      actions       = ["UpdateFlagName"]
      resources     = ["project/${data.featbit_project.iam.key}:env/${data.featbit_environment.prod.key}:flag/${featbit_feature_flag.prod_allowed.key}"]
    },
    {
      resource_type = "flag"
      effect        = "deny"
      actions       = ["UpdateFlagName"]
      resources     = ["project/${data.featbit_project.iam.key}:env/${data.featbit_environment.prod.key}:flag/${featbit_feature_flag.prod_denied.key}"]
    },
  ]
}

resource "featbit_group" "admin" {
  name        = %q
  description = "Immutable Owner binding proof"
}

resource "featbit_group" "developer" {
  name        = %q
  description = "Scoped developer access proof"
}

data "featbit_group" "admin" {
  id = featbit_group.admin.id
}

data "featbit_group" "developer" {
  name       = featbit_group.developer.name
  depends_on = [featbit_group.developer]
}

data "featbit_member" "developer" {
  id = var.featbit_test_member_id
}

resource "featbit_group_policy_binding" "admin_owner" {
  group_id  = data.featbit_group.admin.id
  policy_id = data.featbit_policy.owner.id
}

resource "featbit_group_policy_binding" "developer_base" {
  group_id  = data.featbit_group.developer.id
  policy_id = featbit_policy.base.id
}

resource "featbit_group_policy_binding" "developer_scoped" {
  group_id  = data.featbit_group.developer.id
  policy_id = featbit_policy.scoped.id
}

%s
resource "featbit_member_direct_policies" "developer" {
  member_id  = data.featbit_member.developer.id
  policy_ids = []%s
}
`, apiOrigin,
		definition.projectKey,
		definition.devFlagKey,
		definition.prodAllowedKey,
		definition.prodDeniedKey,
		definition.segmentKey,
		fmt.Sprintf(
			"organization/%s:project/${data.featbit_project.iam.key}:env/${data.featbit_environment.dev.key}",
			organizationKey,
		),
		definition.ownerPolicyKey,
		definition.basePolicyKey,
		definition.scopedPolicyKey,
		definition.adminGroupName,
		definition.developerName,
		memberBinding,
		directDependency,
	)
}

func cloudIAMStateChecks(
	definition cloudIAMDefinition,
	includeMemberBinding bool,
) resource.TestCheckFunc {
	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(cloudIAMProjectResource, "key", definition.projectKey),
		resource.TestCheckResourceAttr(cloudIAMProjectData, "key", definition.projectKey),
		resource.TestCheckResourceAttr(cloudIAMDevEnvironmentData, "key", "dev"),
		resource.TestCheckResourceAttr(cloudIAMProdEnvironmentData, "key", "prod"),
		resource.TestCheckResourceAttr(cloudIAMDevFlagResource, "key", definition.devFlagKey),
		resource.TestCheckResourceAttr(cloudIAMProdAllowedResource, "key", definition.prodAllowedKey),
		resource.TestCheckResourceAttr(cloudIAMProdDeniedResource, "key", definition.prodDeniedKey),
		resource.TestCheckResourceAttr(cloudIAMSegmentResource, "key", definition.segmentKey),
		resource.TestCheckResourceAttr(cloudIAMOwnerPolicyData, "key", definition.ownerPolicyKey),
		resource.TestCheckResourceAttr(cloudIAMOwnerPolicyData, "type", client.PolicyTypeSysManaged),
		resource.TestCheckResourceAttr(cloudIAMBasePolicyResource, "key", definition.basePolicyKey),
		resource.TestCheckResourceAttr(cloudIAMBasePolicyResource, "statements.#", "2"),
		resource.TestCheckResourceAttr(cloudIAMScopedPolicyResource, "key", definition.scopedPolicyKey),
		resource.TestCheckResourceAttr(cloudIAMScopedPolicyResource, "statements.#", "4"),
		resource.TestCheckResourceAttr(cloudIAMAdminGroupResource, "name", definition.adminGroupName),
		resource.TestCheckResourceAttr(cloudIAMDeveloperResource, "name", definition.developerName),
		resource.TestCheckResourceAttr(cloudIAMDirectResource, "policy_ids.#", "0"),
		resource.TestCheckResourceAttrPair(cloudIAMProjectResource, "id", cloudIAMProjectData, "id"),
		resource.TestCheckResourceAttrPair(cloudIAMAdminGroupResource, "id", cloudIAMAdminGroupData, "id"),
		resource.TestCheckResourceAttrPair(cloudIAMDeveloperResource, "id", cloudIAMDeveloperGroupData, "id"),
		resource.TestCheckResourceAttrPair(cloudIAMMemberData, "id", cloudIAMDirectResource, "member_id"),
		cloudIAMStatementStateCheck(definition),
		cloudIAMStateSafetyCheck,
	}
	if includeMemberBinding {
		checks = append(checks, resource.TestCheckResourceAttrPair(
			cloudIAMMemberData,
			"id",
			cloudIAMMemberBindingResource,
			"member_id",
		))
	} else {
		checks = append(checks, func(state *testingterraform.State) error {
			if _, found := state.RootModule().Resources[cloudIAMMemberBindingResource]; found {
				return errors.New("detached Cloud IAM Member binding remained in state")
			}
			return nil
		})
	}
	return resource.ComposeTestCheckFunc(checks...)
}

func cloudIAMStatementStateCheck(definition cloudIAMDefinition) resource.TestCheckFunc {
	return func(state *testingterraform.State) error {
		want := map[string][]string{
			cloudIAMBasePolicyResource: {
				"project", "env", "allow", "CanAccessProject", "CanAccessEnv",
				"project/" + definition.projectKey,
				"project/" + definition.projectKey + ":env/*",
			},
			cloudIAMScopedPolicyResource: {
				"flag", "segment", "allow", "deny", "UpdateFlagName",
				"UpdateSegmentDescription",
				"project/" + definition.projectKey + ":env/dev:flag/*",
				"project/" + definition.projectKey + ":env/dev:segment/*",
				"project/" + definition.projectKey + ":env/prod:flag/" + definition.prodAllowedKey,
				"project/" + definition.projectKey + ":env/prod:flag/" + definition.prodDeniedKey,
			},
		}
		for address, expected := range want {
			resourceState, found := state.RootModule().Resources[address]
			if !found || resourceState.Primary == nil {
				return errors.New("Cloud IAM Policy state was missing")
			}
			values := make(map[string]struct{}, len(resourceState.Primary.Attributes))
			for _, value := range resourceState.Primary.Attributes {
				values[value] = struct{}{}
			}
			for _, value := range expected {
				if _, found := values[value]; !found {
					return errors.New("Cloud IAM statement did not round-trip canonically")
				}
			}
		}
		return nil
	}
}

func cloudIAMStateSafetyCheck(state *testingterraform.State) error {
	formatted := fmt.Sprintf("%#v", state)
	for _, name := range []string{
		envAccessToken,
		cloudIAMAdminTokenEnv,
		cloudIAMMemberTokenEnv,
	} {
		if value := os.Getenv(name); value != "" && strings.Contains(formatted, value) {
			return errors.New("Cloud IAM state retained an access credential")
		}
	}
	return nil
}

func cloudIAMCaptureRuntime(
	children *cloudSegmentInventory,
	runtime *cloudIAMRuntime,
	definition cloudIAMDefinition,
) resource.TestCheckFunc {
	return func(state *testingterraform.State) error {
		captures := []struct {
			address     string
			destination *string
		}{
			{cloudIAMProjectResource, &runtime.projectID},
			{cloudIAMDevEnvironmentData, &runtime.devEnvironment},
			{cloudIAMProdEnvironmentData, &runtime.prodEnvironment},
			{cloudIAMDevFlagResource, &runtime.devFlagID},
			{cloudIAMProdAllowedResource, &runtime.prodAllowedID},
			{cloudIAMProdDeniedResource, &runtime.prodDeniedID},
			{cloudIAMSegmentResource, &runtime.segmentID},
			{cloudIAMBasePolicyResource, &runtime.basePolicyID},
			{cloudIAMScopedPolicyResource, &runtime.scopedPolicyID},
			{cloudIAMAdminGroupResource, &runtime.adminGroupID},
			{cloudIAMDeveloperResource, &runtime.developerID},
		}
		for _, capture := range captures {
			value, found := cloudIAMStateValue(state, capture.address, "id")
			if !found || !client.ValidUUID(value) ||
				*capture.destination != "" && *capture.destination != value {
				return errors.New("Cloud IAM runtime identity was missing or changed")
			}
			*capture.destination = value
		}
		children.flags.registerProject(runtime.projectID, definition.projectKey)
		children.flags.registerEnvironment(runtime.projectID, runtime.devEnvironment, "dev")
		children.flags.registerEnvironment(runtime.projectID, runtime.prodEnvironment, "prod")
		children.flags.registerFeatureFlag(runtime.devEnvironment, runtime.devFlagID, definition.devFlagKey)
		children.flags.registerFeatureFlag(
			runtime.prodEnvironment,
			runtime.prodAllowedID,
			definition.prodAllowedKey,
		)
		children.flags.registerFeatureFlag(
			runtime.prodEnvironment,
			runtime.prodDeniedID,
			definition.prodDeniedKey,
		)
		children.registerSegment(runtime.devEnvironment, runtime.segmentID, definition.segmentKey)
		return nil
	}
}

func cloudIAMRemoteGraphCheck(
	admin *client.Client,
	inventory *cloudIAMInventory,
	runtime *cloudIAMRuntime,
	definition cloudIAMDefinition,
	memberBound bool,
) resource.TestCheckFunc {
	return func(*testingterraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
		defer cancel()
		if !runtime.complete() {
			return errors.New("Cloud IAM runtime graph was incomplete")
		}
		owner, found, err := admin.GetPolicyByKey(ctx, definition.ownerPolicyKey)
		if err != nil || !found {
			return errors.New("Cloud IAM built-in Owner Policy was not readable")
		}
		canonicalOwner, err := canonicalizeRemoteObservedPolicy(owner)
		if err != nil || !samePolicyDefinition(canonicalOwner, inventory.ownerBaseline) {
			return errors.New("Cloud IAM workflow changed the built-in Owner Policy")
		}
		if err := cloudIAMVerifyManagedPolicies(ctx, admin, runtime, definition); err != nil {
			return err
		}
		adminPolicies, err := admin.ListGroupPolicyIDs(ctx, runtime.adminGroupID)
		if err != nil || !slices.Equal(adminPolicies, []string{owner.ID}) {
			return errors.New("Cloud IAM admin Group did not contain exactly Owner")
		}
		developerPolicies, err := admin.ListGroupPolicyIDs(ctx, runtime.developerID)
		wantDeveloperPolicies := []string{runtime.basePolicyID, runtime.scopedPolicyID}
		slices.Sort(wantDeveloperPolicies)
		if err != nil || !slices.Equal(developerPolicies, wantDeveloperPolicies) {
			return errors.New("Cloud IAM developer Group Policy set was not exact")
		}
		adminMembers, err := admin.ListGroupMemberIDs(ctx, runtime.adminGroupID)
		if err != nil || slices.Contains(adminMembers, inventory.memberID) {
			return errors.New("Cloud IAM Member entered the admin Group")
		}
		developerMembers, err := admin.ListGroupMemberIDs(ctx, runtime.developerID)
		if err != nil || slices.Contains(developerMembers, inventory.memberID) != memberBound {
			return errors.New("Cloud IAM developer Member binding was not exact")
		}
		direct, err := admin.ListMemberDirectPolicyIDs(ctx, inventory.memberID)
		if err != nil || len(direct) != 0 {
			return errors.New("Cloud IAM Member direct Policy set was not empty")
		}
		allowedDeveloperID := ""
		if memberBound {
			allowedDeveloperID = runtime.developerID
		}
		if err := cloudIAMVerifyNoEffectiveGroupPolicies(
			ctx,
			admin,
			inventory.memberID,
			allowedDeveloperID,
		); err != nil {
			return err
		}
		return nil
	}
}

func cloudIAMVerifyManagedPolicies(
	ctx context.Context,
	admin *client.Client,
	runtime *cloudIAMRuntime,
	definition cloudIAMDefinition,
) error {
	want := []client.Policy{
		{
			ID: runtime.basePolicyID, Name: "Terraform IAM Base Access",
			Key: definition.basePolicyKey, Description: "Project and Environment visibility",
			Type: client.PolicyTypeCustomerManaged,
			Statements: []client.PolicyStatement{
				{ResourceType: "project", Effect: "allow", Actions: []string{"CanAccessProject"}, Resources: []string{"project/" + definition.projectKey}},
				{ResourceType: "env", Effect: "allow", Actions: []string{"CanAccessEnv"}, Resources: []string{"project/" + definition.projectKey + ":env/*"}},
			},
		},
		{
			ID: runtime.scopedPolicyID, Name: "Terraform IAM Scoped Access",
			Key: definition.scopedPolicyKey, Description: "Dev metadata with one exact prod exception",
			Type: client.PolicyTypeCustomerManaged,
			Statements: []client.PolicyStatement{
				{ResourceType: "flag", Effect: "allow", Actions: []string{"UpdateFlagName"}, Resources: []string{"project/" + definition.projectKey + ":env/dev:flag/*"}},
				{ResourceType: "segment", Effect: "allow", Actions: []string{"UpdateSegmentDescription"}, Resources: []string{"project/" + definition.projectKey + ":env/dev:segment/*"}},
				{ResourceType: "flag", Effect: "allow", Actions: []string{"UpdateFlagName"}, Resources: []string{"project/" + definition.projectKey + ":env/prod:flag/" + definition.prodAllowedKey}},
				{ResourceType: "flag", Effect: "deny", Actions: []string{"UpdateFlagName"}, Resources: []string{"project/" + definition.projectKey + ":env/prod:flag/" + definition.prodDeniedKey}},
			},
		},
	}
	for _, expected := range want {
		actual, found, err := admin.GetPolicy(ctx, expected.ID)
		if err != nil || !found {
			return errors.New("Cloud IAM custom Policy exact read failed")
		}
		canonicalActual, actualErr := canonicalizeRemoteManagedPolicy(actual)
		canonicalExpected, expectedErr := canonicalizeRemoteManagedPolicy(expected)
		if actualErr != nil || expectedErr != nil ||
			!samePolicyDefinition(canonicalActual, canonicalExpected) {
			return errors.New("Cloud IAM custom Policy did not round-trip canonically")
		}
	}
	return nil
}

func cloudIAMVerifyNoEffectiveGroupPolicies(
	ctx context.Context,
	admin *client.Client,
	memberID string,
	allowedGroupID string,
) error {
	groups, err := admin.ListGroups(ctx)
	if err != nil {
		return errors.New("Cloud IAM Member Group baseline was unreadable")
	}
	for _, group := range groups {
		members, err := admin.ListGroupMemberIDs(ctx, group.ID)
		if err != nil {
			return errors.New("Cloud IAM Group Member collection was unreadable")
		}
		if !slices.Contains(members, memberID) {
			continue
		}
		policies, err := admin.ListGroupPolicyIDs(ctx, group.ID)
		if err != nil {
			return errors.New("Cloud IAM Group Policy collection was unreadable")
		}
		if client.EqualUUID(group.ID, allowedGroupID) {
			if len(policies) != 2 {
				return errors.New("Cloud IAM allowed developer Group was incomplete")
			}
			continue
		}
		if len(policies) != 0 {
			return errors.New("Cloud IAM Member retained another effective Group Policy")
		}
	}
	return nil
}

func cloudIAMEffectiveCheck(
	admin *client.Client,
	restricted *client.Client,
	runtime *cloudIAMRuntime,
	definition cloudIAMEffectiveDefinitions,
	memberBound bool,
) resource.TestCheckFunc {
	return func(*testingterraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
		defer cancel()
		if memberBound {
			return cloudIAMVerifyEffectiveAccess(ctx, admin, restricted, runtime, definition)
		}
		return cloudIAMVerifyDetachedAccess(ctx, admin, restricted, runtime, definition)
	}
}
