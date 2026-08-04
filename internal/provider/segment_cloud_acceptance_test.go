// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	testingterraform "github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	cloudSegmentOrganizationKeyEnv = "FEATBIT_TEST_ORGANIZATION_KEY"
	cloudSegmentResourceName       = "featbit_segment.cloud_segment"
	cloudSegmentDataName           = "data.featbit_segment.cloud_segment"
)

func TestAccSegmentCloudLifecycleAndExactCleanup(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("set TF_ACC=1 to run trusted FeatBit Cloud Segment acceptance")
	}
	accessToken, ok := os.LookupEnv(envAccessToken)
	if !ok || accessToken == "" {
		t.Skip("trusted FeatBit Cloud Segment acceptance requires FEATBIT_ACCESS_TOKEN")
	}
	organizationKey := cloudSegmentAcceptanceOrganizationKey(t)
	apiURL := cloudAcceptanceAPIURL(t)
	apiClient, err := client.New(apiURL, accessToken, client.Options{
		HTTPTimeout:     client.DefaultHTTPTimeout,
		MaxConcurrency:  client.DefaultMaxConcurrency,
		MaxRetries:      client.DefaultMaxRetries,
		ProviderVersion: "cloud-segment-acceptance",
	})
	if err != nil {
		t.Fatal("could not construct the trusted Cloud Segment acceptance client")
	}
	targetingClient := newCloudFeatureFlagTargetingClient(apiURL, accessToken)

	prefix := strings.Replace(cloudAcceptancePrefix(t), "tfacc-pe-", "tfacc-seg-", 1)
	projectKey := prefix + "-project"
	environmentKey := prefix + "-env"
	segmentKeyA := prefix + "-segment-a"
	segmentKeyB := prefix + "-segment-b"
	referenceFlag := cloudFeatureFlagDefinition{
		terraformName: "cloud_segment_reference",
		name:          "Terraform Cloud Segment Reference Flag",
		key:           prefix + "-reference-flag",
		variationType: featureFlagVariationTypeBoolean,
		variations: []featureFlagVariationInput{
			{Name: "Enabled", Value: "true"},
			{Name: "Disabled", Value: "false"},
		},
	}
	scope := fmt.Sprintf(
		"organization/%s:project/%s:env/%s",
		organizationKey,
		projectKey,
		environmentKey,
	)
	if kind, valid := client.ClassifySegmentScope(scope); !valid || kind != client.SegmentScopeEnvironment {
		t.Skip("trusted Cloud Segment acceptance requires a valid out-of-band organization key")
	}

	initial := segmentProtocolDefinition{
		Name:        "Terraform Cloud Segment",
		Key:         segmentKeyA,
		Description: "Initial Cloud Segment definition",
		Scopes:      []string{scope},
		Included:    []string{"cloud-segment-user-z", "cloud-segment-user-a"},
		Excluded:    []string{"cloud-segment-excluded-z", "cloud-segment-excluded-a"},
		Rules: []segmentProtocolRuleDefinition{{
			Name: "Cloud Segment Rule",
			Conditions: []segmentProtocolConditionDefinition{{
				Property: "country",
				Operator: segmentOperatorIsOneOf,
				Value:    `["CA","US"]`,
			}},
		}},
		Tags: []string{"cloud-segment-tag-z", "cloud-segment-tag-a"},
	}
	replacement := cloneSegmentProtocolDefinition(initial)
	replacement.Key = segmentKeyB
	replacement.Description = "Replacement Cloud Segment definition"

	inventory := newCloudSegmentInventory(
		apiClient,
		[]string{projectKey},
		[]string{environmentKey},
		[]string{referenceFlag.key},
		[]string{segmentKeyA, segmentKeyB},
	)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
		defer cancel()
		if err := inventory.cleanupAndVerify(ctx); err != nil {
			t.Error("trusted Cloud Segment acceptance cleanup did not reach exact zero")
		}
	})

	var projectID string
	var environmentID string
	var initialSegmentID string
	var replacementSegmentID string
	var recreatedSegmentID string
	flagIDs := make(map[string]string, 1)
	var referenceSnapshot cloudFeatureFlagReferenceSnapshot
	var segmentZeroVerified bool
	var featureFlagZeroVerified bool

	initialConfig := cloudSegmentAcceptanceConfig(
		apiURL.String(),
		projectKey,
		environmentKey,
		referenceFlag,
		initial,
		true,
	)
	replacementConfig := cloudSegmentAcceptanceConfig(
		apiURL.String(),
		projectKey,
		environmentKey,
		referenceFlag,
		replacement,
		true,
	)
	parentAndFlagConfig := cloudSegmentAcceptanceConfig(
		apiURL.String(),
		projectKey,
		environmentKey,
		referenceFlag,
		replacement,
		false,
	)
	parentOnlyConfig := cloudAcceptanceConfig(
		apiURL.String(),
		"Terraform Segment Acceptance Project",
		projectKey,
		"Terraform Segment Acceptance Environment",
		environmentKey,
		"Segment Cloud acceptance parent",
	)
	providerOnlyConfig := cloudAcceptanceProviderOnlyConfig(apiURL.String())

	steps := []resource.TestStep{
		{
			Config: initialConfig,
			Check: resource.ComposeTestCheckFunc(
				cloudSegmentParentAndFlagStateChecks(projectKey, environmentKey, referenceFlag),
				cloudSegmentStateChecks(initial, true),
				cloudFeatureFlagCaptureIDs(
					inventory.flags,
					[]cloudFeatureFlagDefinition{referenceFlag},
					&projectID,
					&environmentID,
					flagIDs,
					nil,
					projectKey,
					environmentKey,
				),
				cloudSegmentCaptureID(
					inventory,
					apiClient,
					&environmentID,
					&initialSegmentID,
					nil,
					initial,
				),
			),
		},
		{
			Config:   initialConfig,
			PlanOnly: true,
		},
		{
			ResourceName:      cloudSegmentResourceName,
			ImportState:       true,
			ImportStateIdFunc: cloudSegmentImportID(&environmentID, &initialSegmentID),
			ImportStateVerify: true,
		},
		{
			Config:   initialConfig,
			PlanOnly: true,
		},
		{
			PreConfig: func() {
				cloudSegmentDriftOwnedFields(
					t,
					apiClient,
					environmentID,
					initialSegmentID,
				)
			},
			Config: initialConfig,
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(
						cloudSegmentResourceName,
						plancheck.ResourceActionUpdate,
					),
				},
			},
			Check: resource.ComposeTestCheckFunc(
				cloudSegmentParentAndFlagStateChecks(projectKey, environmentKey, referenceFlag),
				cloudSegmentStateChecks(initial, true),
				cloudSegmentIDUnchanged(&initialSegmentID),
				cloudFeatureFlagIDsUnchanged(
					[]cloudFeatureFlagDefinition{referenceFlag},
					flagIDs,
				),
				cloudSegmentRemoteDefinitionCheck(
					apiClient,
					&environmentID,
					&initialSegmentID,
					initial,
				),
			),
		},
		{
			Config: replacementConfig,
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(
						cloudSegmentResourceName,
						plancheck.ResourceActionReplace,
					),
				},
			},
			Check: resource.ComposeTestCheckFunc(
				cloudSegmentParentAndFlagStateChecks(projectKey, environmentKey, referenceFlag),
				cloudSegmentStateChecks(replacement, true),
				cloudFeatureFlagIDsUnchanged(
					[]cloudFeatureFlagDefinition{referenceFlag},
					flagIDs,
				),
				cloudSegmentCaptureID(
					inventory,
					apiClient,
					&environmentID,
					&replacementSegmentID,
					&initialSegmentID,
					replacement,
				),
			),
		},
		{
			PreConfig: func() {
				cloudSegmentDeleteOutOfBand(
					t,
					inventory,
					environmentID,
					replacementSegmentID,
					replacement.Key,
				)
			},
			Config: replacementConfig,
			Check: resource.ComposeTestCheckFunc(
				cloudSegmentParentAndFlagStateChecks(projectKey, environmentKey, referenceFlag),
				cloudSegmentStateChecks(replacement, true),
				cloudFeatureFlagIDsUnchanged(
					[]cloudFeatureFlagDefinition{referenceFlag},
					flagIDs,
				),
				cloudSegmentCaptureID(
					inventory,
					apiClient,
					&environmentID,
					&recreatedSegmentID,
					&replacementSegmentID,
					replacement,
				),
			),
		},
		{
			PreConfig: func() {
				referenceSnapshot = cloudSegmentAddFeatureFlagReference(
					t,
					targetingClient,
					apiClient,
					environmentID,
					referenceFlag.key,
					flagIDs[referenceFlag.terraformName],
					recreatedSegmentID,
				)
			},
			Config:      parentAndFlagConfig,
			ExpectError: regexp.MustCompile(`(?i)segment is referenced by feature flags`),
		},
		{
			PreConfig: func() {
				cloudSegmentRemoveFeatureFlagReference(
					t,
					targetingClient,
					apiClient,
					referenceSnapshot,
				)
			},
			Config: parentAndFlagConfig,
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(
						cloudSegmentResourceName,
						plancheck.ResourceActionDestroy,
					),
				},
			},
			Check: resource.ComposeTestCheckFunc(
				cloudSegmentParentAndFlagStateChecks(projectKey, environmentKey, referenceFlag),
				cloudFeatureFlagIDsUnchanged(
					[]cloudFeatureFlagDefinition{referenceFlag},
					flagIDs,
				),
				cloudSegmentResourceAbsentCheck(),
				cloudSegmentExactZeroCheck(
					inventory,
					&environmentID,
					[]string{segmentKeyA, segmentKeyB},
				),
				func(*testingterraform.State) error {
					ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
					defer cancel()
					if err := targetingClient.verifyBaseline(ctx, referenceSnapshot); err != nil {
						return err
					}
					segmentZeroVerified = true
					return nil
				},
			),
		},
		{
			Config: parentOnlyConfig,
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(
						cloudFeatureFlagResourceAddress(referenceFlag),
						plancheck.ResourceActionDestroy,
					),
				},
			},
			Check: resource.ComposeTestCheckFunc(
				cloudSegmentExactZeroCheck(
					inventory,
					&environmentID,
					[]string{segmentKeyA, segmentKeyB},
				),
				cloudFeatureFlagExactZeroCheck(
					inventory.flags,
					&environmentID,
					[]string{referenceFlag.key},
				),
				func(*testingterraform.State) error {
					featureFlagZeroVerified = true
					return nil
				},
			),
		},
		{
			Config: providerOnlyConfig,
			Check: cloudSegmentFinalParentZeroCheck(
				inventory,
				&segmentZeroVerified,
				&featureFlagZeroVerified,
			),
		},
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"featbit": providerserver.NewProtocol6WithError(New("cloud-segment-acceptance")()),
		},
		PreCheck: func() {
			cloudSegmentAcceptancePreCheck(t)
		},
		CheckDestroy: func(*testingterraform.State) error {
			ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
			defer cancel()
			return inventory.cleanupAndVerify(ctx)
		},
		Steps: steps,
	})
}

func TestCloudSegmentInventoryCleansUnreturnedCreatesBeforeParents(t *testing.T) {
	fixture := newCrossSegmentProtocolFixture(t)
	defer fixture.close()
	apiURL, err := parseAPIURL(fixture.apiOrigin())
	if err != nil {
		t.Fatal("could not configure the Segment cleanup inventory fixture URL")
	}
	apiClient, err := client.New(apiURL, syntheticProviderAccessToken, client.Options{
		HTTPTimeout:     client.DefaultHTTPTimeout,
		MaxConcurrency:  client.DefaultMaxConcurrency,
		MaxRetries:      0,
		ProviderVersion: "protocol-test",
	})
	if err != nil {
		t.Fatal("could not construct the Segment cleanup inventory fixture client")
	}

	const (
		projectKey     = "cloud-segment-cleanup-project"
		environmentKey = "cloud-segment-cleanup-environment"
		segmentKey     = "cloud-segment-cleanup-segment"
		flagKey        = "cloud-segment-cleanup-flag"
	)
	inventory := newCloudSegmentInventory(
		apiClient,
		[]string{projectKey},
		[]string{environmentKey},
		[]string{flagKey},
		[]string{segmentKey},
	)
	ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
	defer cancel()

	project, err := apiClient.CreateProject(ctx, client.CreateProjectRequest{
		Name: "Cloud Segment cleanup Project",
		Key:  projectKey,
	})
	if err != nil {
		t.Fatal("could not create the Segment cleanup Project")
	}
	environment, err := apiClient.CreateEnvironment(
		ctx,
		project.ID,
		client.CreateEnvironmentRequest{
			Name: "Cloud Segment cleanup Environment",
			Key:  environmentKey,
		},
	)
	if err != nil {
		t.Fatal("could not create the Segment cleanup Environment")
	}
	segment, err := apiClient.CreateSegment(ctx, environment.ID, client.CreateSegmentRequest{
		Type:        client.SegmentTypeEnvironmentSpecific,
		Name:        "Cloud Segment cleanup Segment",
		Key:         segmentKey,
		Description: "cleanup",
		Scopes: []string{
			"organization/test:project/cleanup:env/cleanup",
		},
	})
	if err != nil {
		t.Fatal("could not create the unreturned Segment cleanup object")
	}
	variationOn := deterministicFeatureFlagVariationID(environment.ID, flagKey, 0)
	variationOff := deterministicFeatureFlagVariationID(environment.ID, flagKey, 1)
	flag, err := apiClient.CreateFeatureFlag(ctx, environment.ID, client.CreateFeatureFlagRequest{
		Name:          "Cloud Segment cleanup Feature Flag",
		Key:           flagKey,
		IsEnabled:     false,
		Description:   "cleanup",
		VariationType: featureFlagVariationTypeBoolean,
		Variations: []client.FeatureFlagVariation{
			{ID: variationOn, Name: "Enabled", Value: "true"},
			{ID: variationOff, Name: "Disabled", Value: "false"},
		},
		EnabledVariationID:  variationOn,
		DisabledVariationID: variationOn,
		Tags:                []string{},
	})
	if err != nil {
		t.Fatal("could not create the unreturned Segment reference cleanup object")
	}

	// Register no returned identity. Exact test keys must discover both children
	// and remove the Feature Flag before the Segment and both parents.
	if err := inventory.cleanupAndVerify(ctx); err != nil {
		t.Fatal("Segment cleanup inventory did not remove unreturned objects")
	}
	if fixture.base.flags.objectCount() != 0 || fixture.segments.managedCount() != 0 ||
		fixture.base.project.projectCount() != 0 || fixture.base.project.environmentCount() != 0 {
		t.Fatal("Segment cleanup inventory fixture did not reach exact zero")
	}
	if fixture.violationCount() != 0 {
		t.Fatal("Segment cleanup inventory fixture observed a request contract violation")
	}
	assertCloudSegmentCleanupOrder(
		t,
		fixture.requestSnapshot(),
		environment.ID,
		flag.Key,
		segment.ID,
		project.ID,
	)
}

func TestCloudFeatureFlagTargetingClientAddsAndRemovesOnlyExactSegmentReference(
	t *testing.T,
) {
	const flagKey = "cloud-segment-reference-contract"
	variationID := uuid.NewString()
	revision := uuid.NewString()
	targeting := cloudFeatureFlagTargeting{
		DisabledVariationID:   variationID,
		TargetUsers:           json.RawMessage(`[]`),
		Rules:                 json.RawMessage(`[]`),
		Fallthrough:           json.RawMessage(`{"dispatchKey":"keyId","includedInExpt":false,"variations":[]}`),
		ExptIncludeAllTargets: false,
	}
	putCount := 0
	violations := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Header.Get("Authorization") != syntheticProviderAccessToken ||
			request.Header.Get("Organization") != "" || request.Header.Get("Workspace") != "" {
			violations++
			writeProjectFixtureEnvelope(response, http.StatusBadRequest, nil)
			return
		}
		exactPath := "/api/v1/envs/" + providerEnvironmentA + "/feature-flags/" + flagKey
		switch {
		case request.Method == http.MethodGet && request.URL.EscapedPath() == exactPath:
			writeProjectFixtureEnvelope(response, http.StatusOK, map[string]any{
				"id":                    providerFeatureFlagID,
				"envId":                 providerEnvironmentA,
				"key":                   flagKey,
				"revision":              revision,
				"variations":            []map[string]any{{"id": variationID}},
				"disabledVariationId":   targeting.DisabledVariationID,
				"targetUsers":           targeting.TargetUsers,
				"rules":                 targeting.Rules,
				"fallthrough":           targeting.Fallthrough,
				"exptIncludeAllTargets": targeting.ExptIncludeAllTargets,
			})
		case request.Method == http.MethodPut &&
			request.URL.EscapedPath() == exactPath+"/targeting":
			var payload struct {
				Revision  string                    `json:"revision"`
				Targeting cloudFeatureFlagTargeting `json:"targeting"`
			}
			if request.Header.Get("Content-Type") != "application/json" ||
				json.NewDecoder(request.Body).Decode(&payload) != nil || payload.Revision != revision {
				violations++
				writeProjectFixtureEnvelope(response, http.StatusBadRequest, nil)
				return
			}
			targeting = payload.Targeting
			revision = uuid.NewString()
			putCount++
			writeProjectFixtureEnvelope(response, http.StatusOK, revision)
		default:
			violations++
			writeProjectFixtureEnvelope(response, http.StatusNotFound, nil)
		}
	}))
	defer server.Close()
	apiURL, err := parseAPIURL(server.URL)
	if err != nil {
		t.Fatal("could not configure the Cloud targeting contract URL")
	}
	targetingClient := newCloudFeatureFlagTargetingClient(apiURL, syntheticProviderAccessToken)
	ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
	defer cancel()
	snapshot, err := targetingClient.addSegmentReference(
		ctx,
		providerEnvironmentA,
		flagKey,
		providerSegmentID,
	)
	if err != nil {
		t.Fatal("Cloud targeting contract did not add the exact Segment reference")
	}
	flag, err := targetingClient.get(ctx, providerEnvironmentA, flagKey)
	if err != nil || !cloudFeatureFlagHasExactReference(flag, snapshot) {
		t.Fatal("Cloud targeting contract did not retain the exact Segment reference")
	}
	if err := targetingClient.removeSegmentReference(ctx, snapshot); err != nil {
		t.Fatal("Cloud targeting contract did not remove the exact Segment reference")
	}
	if err := targetingClient.verifyBaseline(ctx, snapshot); err != nil {
		t.Fatal("Cloud targeting contract did not restore the original targeting baseline")
	}
	if putCount != 2 || violations != 0 {
		t.Fatal("Cloud targeting contract did not use two exact public mutations")
	}
}

func TestCloudWaitSegmentDirectAbsenceRequiresCompleteViews(t *testing.T) {
	script := &segmentHTTPScript{
		t: t,
		expectations: []segmentHTTPExpectation{
			{
				method: http.MethodGet,
				path:   segmentResourceExactPath(providerSegmentID),
				status: http.StatusNotFound,
				data:   "null",
			},
			segmentCollectionExpectation(false, 0, []string{}),
			segmentCollectionExpectation(true, 0, []string{}),
		},
	}
	apiClient, closeServer := newProjectResourceTestClient(t, script)
	defer closeServer()
	ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
	defer cancel()
	if !cloudWaitSegmentDirectAbsence(
		ctx,
		apiClient,
		providerEnvironmentA,
		providerSegmentID,
		"synthetic-segment",
	) {
		t.Fatal("Cloud Segment direct and collection absence proof did not converge")
	}
	script.assertComplete(t)
}

func cloudSegmentAcceptancePreCheck(t *testing.T) {
	t.Helper()
	cloudAcceptancePreCheck(t)
	_ = cloudSegmentAcceptanceOrganizationKey(t)
}

func cloudSegmentAcceptanceOrganizationKey(t *testing.T) string {
	t.Helper()
	value, ok := os.LookupEnv(cloudSegmentOrganizationKeyEnv)
	if !ok || value == "" {
		t.Skip("trusted Cloud Segment acceptance requires FEATBIT_TEST_ORGANIZATION_KEY")
	}
	if strings.TrimSpace(value) != value || strings.ContainsAny(value, ":/*") {
		t.Skip("trusted Cloud Segment acceptance requires a valid FEATBIT_TEST_ORGANIZATION_KEY")
	}
	return value
}

func cloudSegmentAcceptanceConfig(
	apiOrigin string,
	projectKey string,
	environmentKey string,
	referenceFlag cloudFeatureFlagDefinition,
	definition segmentProtocolDefinition,
	includeSegment bool,
) string {
	config := cloudFeatureFlagConfig(
		apiOrigin,
		projectKey,
		environmentKey,
		[]cloudFeatureFlagDefinition{referenceFlag},
		false,
	)
	if !includeSegment {
		return config
	}
	return config + fmt.Sprintf(`

resource "featbit_segment" "cloud_segment" {
  environment_id = featbit_environment.cloud_child.id
  name           = %q
  key            = %q
  description    = %q
  scopes         = %s
  included_users = %s
  excluded_users = %s
  rules          = %s
  tags           = %s
}

data "featbit_segment" "cloud_segment" {
  environment_id = featbit_segment.cloud_segment.environment_id
  id             = featbit_segment.cloud_segment.id
}
`, definition.Name, definition.Key, definition.Description,
		segmentProtocolStringList(definition.Scopes),
		segmentProtocolStringList(definition.Included),
		segmentProtocolStringList(definition.Excluded),
		segmentProtocolRules(definition.Rules),
		segmentProtocolStringList(definition.Tags))
}

func cloudAcceptanceProviderOnlyConfig(apiOrigin string) string {
	return fmt.Sprintf(`
provider "featbit" {
  api_url              = %q
  http_timeout_seconds = 30
  max_concurrency      = 2
  max_retries          = 3
}
`, apiOrigin)
}

func cloudSegmentParentAndFlagStateChecks(
	projectKey string,
	environmentKey string,
	referenceFlag cloudFeatureFlagDefinition,
) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		cloudAcceptanceStateChecks(
			"Terraform Feature Flag Acceptance Project",
			projectKey,
			"Terraform Feature Flag Acceptance Environment",
			environmentKey,
			"Feature Flag Cloud acceptance parent",
		),
		cloudFeatureFlagStateChecks([]cloudFeatureFlagDefinition{referenceFlag}, false),
	)
}

func cloudSegmentStateChecks(
	definition segmentProtocolDefinition,
	includeExactData bool,
) resource.TestCheckFunc {
	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(cloudSegmentResourceName, "name", definition.Name),
		resource.TestCheckResourceAttr(cloudSegmentResourceName, "key", definition.Key),
		resource.TestCheckResourceAttr(cloudSegmentResourceName, "description", definition.Description),
		resource.TestCheckResourceAttr(
			cloudSegmentResourceName,
			"type",
			string(client.SegmentTypeEnvironmentSpecific),
		),
		resource.TestCheckResourceAttrPair(
			cloudAcceptanceEnvironmentName,
			"id",
			cloudSegmentResourceName,
			"environment_id",
		),
		segmentProtocolSetChecks(cloudSegmentResourceName, "scopes", definition.Scopes),
		segmentProtocolSetChecks(cloudSegmentResourceName, "included_users", definition.Included),
		segmentProtocolSetChecks(cloudSegmentResourceName, "excluded_users", definition.Excluded),
		segmentProtocolSetChecks(cloudSegmentResourceName, "tags", definition.Tags),
		segmentProtocolRuleChecks(cloudSegmentResourceName, definition.Rules),
	}
	if includeExactData {
		checks = append(checks,
			resource.TestCheckResourceAttrPair(
				cloudSegmentResourceName,
				"id",
				cloudSegmentDataName,
				"id",
			),
			resource.TestCheckResourceAttrPair(
				cloudSegmentResourceName,
				"environment_id",
				cloudSegmentDataName,
				"environment_id",
			),
			resource.TestCheckResourceAttr(cloudSegmentDataName, "name", definition.Name),
			resource.TestCheckResourceAttr(cloudSegmentDataName, "key", definition.Key),
			resource.TestCheckResourceAttr(cloudSegmentDataName, "description", definition.Description),
			resource.TestCheckResourceAttr(
				cloudSegmentDataName,
				"type",
				string(client.SegmentTypeEnvironmentSpecific),
			),
			segmentProtocolSetChecks(cloudSegmentDataName, "scopes", definition.Scopes),
			segmentProtocolSetChecks(cloudSegmentDataName, "included_users", definition.Included),
			segmentProtocolSetChecks(cloudSegmentDataName, "excluded_users", definition.Excluded),
			segmentProtocolSetChecks(cloudSegmentDataName, "tags", definition.Tags),
			segmentProtocolRuleChecks(cloudSegmentDataName, definition.Rules),
		)
	}
	return resource.ComposeTestCheckFunc(checks...)
}

func cloudSegmentCaptureID(
	inventory *cloudSegmentInventory,
	apiClient *client.Client,
	environmentID *string,
	destination *string,
	mustDifferFrom *string,
	definition segmentProtocolDefinition,
) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttrWith(
			cloudSegmentResourceName,
			"id",
			func(value string) error {
				if !validUUID(value) || !validUUID(*environmentID) ||
					mustDifferFrom != nil && value == *mustDifferFrom {
					return errors.New("Cloud Segment identity did not satisfy replacement or recreation")
				}
				*destination = value
				inventory.registerSegment(*environmentID, value, definition.Key)
				return nil
			},
		),
		cloudSegmentRemoteDefinitionCheck(apiClient, environmentID, destination, definition),
	)
}

func cloudSegmentIDUnchanged(segmentID *string) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(
		cloudSegmentResourceName,
		"id",
		func(value string) error {
			if value != *segmentID {
				return errors.New("Cloud Segment specialized Update changed its identity")
			}
			return nil
		},
	)
}

func cloudSegmentRemoteDefinitionCheck(
	apiClient *client.Client,
	environmentID *string,
	segmentID *string,
	definition segmentProtocolDefinition,
) resource.TestCheckFunc {
	return func(*testingterraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
		defer cancel()
		segment, err := apiClient.GetSegment(ctx, *environmentID, *segmentID)
		if err != nil || !cloudSegmentMatchesDefinition(segment, definition) {
			return errors.New("Cloud Segment exact definition did not converge")
		}
		return nil
	}
}

func cloudSegmentMatchesDefinition(
	segment client.Segment,
	definition segmentProtocolDefinition,
) bool {
	if segment.IsArchived || segment.Type != client.SegmentTypeEnvironmentSpecific ||
		segment.Name != definition.Name || segment.Key != definition.Key ||
		segment.Description != definition.Description ||
		!slices.Equal(canonicalStringSet(segment.Scopes), canonicalStringSet(definition.Scopes)) ||
		!slices.Equal(canonicalStringSet(segment.Included), canonicalStringSet(definition.Included)) ||
		!slices.Equal(canonicalStringSet(segment.Excluded), canonicalStringSet(definition.Excluded)) ||
		!slices.Equal(canonicalStringSet(segment.Tags), canonicalStringSet(definition.Tags)) ||
		len(segment.Rules) != len(definition.Rules) {
		return false
	}
	for ruleIndex, rule := range definition.Rules {
		actual := segment.Rules[ruleIndex]
		if !validUUID(actual.ID) || actual.Name != rule.Name ||
			len(actual.Conditions) != len(rule.Conditions) {
			return false
		}
		for conditionIndex, condition := range rule.Conditions {
			actualCondition := actual.Conditions[conditionIndex]
			if !validUUID(actualCondition.ID) ||
				actualCondition.Property != condition.Property ||
				actualCondition.Operator != condition.Operator ||
				actualCondition.Value != condition.Value {
				return false
			}
		}
	}
	return true
}

func cloudSegmentImportID(
	environmentID *string,
	segmentID *string,
) resource.ImportStateIdFunc {
	return func(*testingterraform.State) (string, error) {
		if !validUUID(*environmentID) || !validUUID(*segmentID) {
			return "", errors.New("Cloud Segment Import identities are unavailable")
		}
		return *environmentID + "/" + *segmentID, nil
	}
}

func cloudSegmentDriftOwnedFields(
	t *testing.T,
	apiClient *client.Client,
	environmentID string,
	segmentID string,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
	defer cancel()
	mutations := []struct {
		apply func() error
		match func(client.Segment) bool
	}{
		{
			apply: func() error {
				return apiClient.UpdateSegmentName(
					ctx,
					environmentID,
					segmentID,
					client.UpdateSegmentNameRequest{Name: "External Cloud Segment Drift"},
				)
			},
			match: func(segment client.Segment) bool {
				return segment.Name == "External Cloud Segment Drift"
			},
		},
		{
			apply: func() error {
				return apiClient.UpdateSegmentDescription(
					ctx,
					environmentID,
					segmentID,
					client.UpdateSegmentDescriptionRequest{Description: "External drift"},
				)
			},
			match: func(segment client.Segment) bool {
				return segment.Description == "External drift"
			},
		},
		{
			apply: func() error {
				return apiClient.UpdateSegmentTargeting(
					ctx,
					environmentID,
					segmentID,
					client.UpdateSegmentTargetingRequest{
						Included: []string{"external-cloud-user"},
						Excluded: []string{},
						Rules: []client.SegmentRule{{
							ID:   uuid.NewString(),
							Name: "External Cloud Rule",
							Conditions: []client.SegmentCondition{{
								ID:       uuid.NewString(),
								Property: "country",
								Operator: segmentOperatorEqual,
								Value:    "external",
							}},
						}},
					},
				)
			},
			match: func(segment client.Segment) bool {
				return slices.Equal(segment.Included, []string{"external-cloud-user"}) &&
					len(segment.Excluded) == 0 && len(segment.Rules) == 1 &&
					segment.Rules[0].Name == "External Cloud Rule"
			},
		},
		{
			apply: func() error {
				return apiClient.UpdateSegmentTags(
					ctx,
					environmentID,
					segmentID,
					client.UpdateSegmentTagsRequest{Tags: []string{"external-cloud-tag"}},
				)
			},
			match: func(segment client.Segment) bool {
				return slices.Equal(segment.Tags, []string{"external-cloud-tag"})
			},
		},
	}
	for _, mutation := range mutations {
		mutationErr := mutation.apply()
		if mutationErr != nil && !segmentMutationNeedsReconciliation(mutationErr) {
			t.Fatal("could not prepare external Cloud Segment drift")
		}
		if !cloudWaitExactSegment(ctx, apiClient, environmentID, segmentID, mutation.match) {
			t.Fatal("external Cloud Segment drift was not confirmed exactly")
		}
	}
}

func cloudWaitExactSegment(
	ctx context.Context,
	apiClient *client.Client,
	environmentID string,
	segmentID string,
	match func(client.Segment) bool,
) bool {
	for attempt := 0; attempt < 30; attempt++ {
		segment, err := apiClient.GetSegment(ctx, environmentID, segmentID)
		if err == nil && match(segment) {
			return true
		}
		if !cloudObservationDelay(ctx) {
			break
		}
	}
	return false
}

func cloudSegmentDeleteOutOfBand(
	t *testing.T,
	inventory *cloudSegmentInventory,
	environmentID string,
	segmentID string,
	key string,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
	defer cancel()
	if err := inventory.deleteSegment(ctx, environmentID, segmentID, key); err != nil {
		t.Fatal("could not prepare out-of-band Cloud Segment deletion")
	}
	if !cloudWaitSegmentDirectAbsence(ctx, inventory.api, environmentID, segmentID, key) {
		t.Fatal("out-of-band Cloud Segment deletion did not converge across exact and collection reads")
	}
}

func cloudWaitSegmentDirectAbsence(
	ctx context.Context,
	apiClient *client.Client,
	environmentID string,
	segmentID string,
	key string,
) bool {
	identity := client.SegmentIdentity{ID: segmentID, Key: key}
	for attempt := 0; attempt < 60; attempt++ {
		_, directErr := apiClient.GetSegment(ctx, environmentID, segmentID)
		_, status, resolveErr := apiClient.ResolveSegment(ctx, environmentID, identity)
		if directErr != nil &&
			client.Classify(0, nil, directErr) == client.ClassificationNotFoundUnconfirmed &&
			resolveErr == nil && status == client.SegmentStatusAbsent {
			return true
		}
		if !cloudObservationDelay(ctx) {
			break
		}
	}
	return false
}

func cloudSegmentAddFeatureFlagReference(
	t *testing.T,
	targetingClient *cloudFeatureFlagTargetingClient,
	apiClient *client.Client,
	environmentID string,
	flagKey string,
	flagID string,
	segmentID string,
) cloudFeatureFlagReferenceSnapshot {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
	defer cancel()
	snapshot, err := targetingClient.addSegmentReference(ctx, environmentID, flagKey, segmentID)
	if err != nil || !client.EqualUUID(snapshot.flagID, flagID) {
		t.Fatal("could not prepare an exact Cloud Segment Feature Flag reference")
	}
	if !cloudWaitExactSegmentReferences(
		ctx,
		apiClient,
		environmentID,
		segmentID,
		flagID,
		flagKey,
		true,
	) {
		t.Fatal("Cloud Segment Feature Flag reference was not observed exactly")
	}
	return snapshot
}

func cloudSegmentRemoveFeatureFlagReference(
	t *testing.T,
	targetingClient *cloudFeatureFlagTargetingClient,
	apiClient *client.Client,
	snapshot cloudFeatureFlagReferenceSnapshot,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
	defer cancel()
	if !cloudWaitExactSegmentReferences(
		ctx,
		apiClient,
		snapshot.environmentID,
		snapshot.segmentID,
		snapshot.flagID,
		snapshot.flagKey,
		true,
	) {
		t.Fatal("reference-conflicted Cloud Segment or Feature Flag changed unexpectedly")
	}
	if err := targetingClient.removeSegmentReference(ctx, snapshot); err != nil {
		t.Fatal("could not remove the exact Cloud Segment Feature Flag reference")
	}
	if !cloudWaitExactSegmentReferences(
		ctx,
		apiClient,
		snapshot.environmentID,
		snapshot.segmentID,
		snapshot.flagID,
		snapshot.flagKey,
		false,
	) {
		t.Fatal("Cloud Segment Feature Flag reference removal was not observed exactly")
	}
}

func cloudWaitExactSegmentReferences(
	ctx context.Context,
	apiClient *client.Client,
	environmentID string,
	segmentID string,
	flagID string,
	flagKey string,
	wantReference bool,
) bool {
	for attempt := 0; attempt < 30; attempt++ {
		references, err := apiClient.GetSegmentFlagReferences(ctx, environmentID, segmentID)
		if err == nil {
			if !wantReference && len(references) == 0 {
				return true
			}
			if wantReference && len(references) == 1 &&
				client.EqualUUID(references[0].EnvironmentID, environmentID) &&
				client.EqualUUID(references[0].ID, flagID) && references[0].Key == flagKey {
				return true
			}
		}
		if !cloudObservationDelay(ctx) {
			break
		}
	}
	return false
}

func cloudSegmentExactZeroCheck(
	inventory *cloudSegmentInventory,
	environmentID *string,
	keys []string,
) resource.TestCheckFunc {
	return func(*testingterraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
		defer cancel()
		return inventory.verifySegmentsAbsent(ctx, *environmentID, keys)
	}
}

func cloudSegmentResourceAbsentCheck() resource.TestCheckFunc {
	return func(state *testingterraform.State) error {
		if _, found := state.RootModule().Resources[cloudSegmentResourceName]; found {
			return errors.New("Cloud Segment remained in Terraform state after exact destroy")
		}
		return nil
	}
}

func cloudSegmentFinalParentZeroCheck(
	inventory *cloudSegmentInventory,
	segmentZeroVerified *bool,
	featureFlagZeroVerified *bool,
) resource.TestCheckFunc {
	return func(*testingterraform.State) error {
		if !*segmentZeroVerified || !*featureFlagZeroVerified {
			return errors.New("Cloud Segment child exact-zero proof did not complete")
		}
		ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
		defer cancel()
		projects, err := inventory.api.ListProjects(ctx)
		if err != nil {
			return errors.New("Cloud Segment final parent collection could not be verified")
		}
		for _, project := range projects {
			if _, tracked := inventory.parents.projectKeys[project.Key]; tracked {
				return errors.New("Cloud Segment final Project collection retained an exact parent")
			}
			for _, environment := range project.Environments {
				if _, tracked := inventory.parents.environmentKeys[environment.Key]; tracked {
					return errors.New("Cloud Segment final Project collection retained an exact Environment")
				}
			}
		}
		inventory.parents.mu.Lock()
		clear(inventory.parents.projects)
		clear(inventory.parents.environments)
		inventory.parents.mu.Unlock()
		inventory.flags.mu.Lock()
		clear(inventory.flags.flags)
		inventory.flags.mu.Unlock()
		inventory.mu.Lock()
		clear(inventory.segments)
		inventory.mu.Unlock()
		return nil
	}
}

func assertCloudSegmentCleanupOrder(
	t *testing.T,
	requests []crossResourceFixtureRequest,
	environmentID string,
	flagKey string,
	segmentID string,
	projectID string,
) {
	t.Helper()
	paths := []struct {
		method string
		path   string
	}{
		{http.MethodDelete, "/api/v1/envs/" + environmentID + "/feature-flags/" + flagKey},
		{http.MethodPut, "/api/v1/envs/" + environmentID + "/segments/" + segmentID + "/archive"},
		{http.MethodDelete, "/api/v1/envs/" + environmentID + "/segments/" + segmentID},
		{http.MethodDelete, "/api/v1/projects/" + projectID + "/envs/" + environmentID},
		{http.MethodDelete, "/api/v1/projects/" + projectID},
	}
	previous := -1
	for _, want := range paths {
		found := -1
		for index, request := range requests {
			if request.Method == want.method && request.Path == want.path {
				found = index
				break
			}
		}
		if found <= previous {
			t.Fatal("Cloud Segment cleanup was not Feature Flag/Segment/Environment/Project ordered")
		}
		previous = found
	}
}
