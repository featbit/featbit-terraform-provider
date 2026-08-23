// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	testingterraform "github.com/hashicorp/terraform-plugin-testing/terraform"
)

type cloudFeatureFlagDefinition struct {
	terraformName string
	name          string
	key           string
	variationType string
	variations    []featureFlagVariationInput
}

func TestAccFeatureFlagCloudFourTypeLifecycle(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("set TF_ACC=1 to run trusted FeatBit Cloud Feature Flag acceptance")
	}
	accessToken, ok := os.LookupEnv(envAccessToken)
	if !ok || accessToken == "" {
		t.Skip("trusted FeatBit Cloud Feature Flag acceptance requires FEATBIT_ACCESS_TOKEN")
	}
	apiURL := cloudAcceptanceAPIURL(t)
	apiClient, err := client.New(apiURL, accessToken, client.Options{
		HTTPTimeout:     client.DefaultHTTPTimeout,
		MaxConcurrency:  client.DefaultMaxConcurrency,
		MaxRetries:      client.DefaultMaxRetries,
		ProviderVersion: "cloud-feature-flag-acceptance",
	})
	if err != nil {
		t.Fatal("could not construct the trusted Cloud Feature Flag acceptance client")
	}
	cloudProxy := newCloudFeatureFlagProxy(apiURL)
	t.Cleanup(cloudProxy.close)
	proxyAPIURL, err := parseAPIURL(cloudProxy.apiOrigin())
	if err != nil {
		t.Fatal("could not configure the Cloud Feature Flag observation client URL")
	}
	proxyAPIClient, err := client.New(proxyAPIURL, accessToken, client.Options{
		HTTPTimeout:     client.DefaultHTTPTimeout,
		MaxConcurrency:  client.DefaultMaxConcurrency,
		MaxRetries:      client.DefaultMaxRetries,
		ProviderVersion: "cloud-feature-flag-observer",
	})
	if err != nil {
		t.Fatal("could not construct the Cloud Feature Flag observation client")
	}

	prefix := strings.Replace(cloudAcceptancePrefix(t), "tfacc-pe-", "tfacc-ff-", 1)
	projectKey := prefix + "-project"
	environmentKey := prefix + "-env"
	definitions := cloudFeatureFlagDefinitions(prefix)
	featureFlagKeys := cloudFeatureFlagKeys(definitions)
	inventory := newCloudFeatureFlagInventory(
		apiClient,
		[]string{projectKey},
		[]string{environmentKey},
		featureFlagKeys,
	)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
		defer cancel()
		if err := inventory.cleanupAndVerify(ctx); err != nil {
			t.Error("trusted Cloud Feature Flag acceptance cleanup did not reach exact zero")
		}
	})

	var projectID string
	var environmentID string
	initialIDs := make(map[string]string, len(definitions))
	replacementIDs := make(map[string]string, len(definitions))
	recreatedIDs := make(map[string]string, len(definitions))
	var lifecycleCompleted bool

	initialConfig := cloudFeatureFlagConfig(
		cloudProxy.apiOrigin(),
		projectKey,
		environmentKey,
		definitions,
		false,
	)
	replacementConfig := cloudFeatureFlagConfig(
		cloudProxy.apiOrigin(),
		projectKey,
		environmentKey,
		definitions,
		true,
	)
	parentOnlyConfig := cloudAcceptanceConfig(
		cloudProxy.apiOrigin(),
		"Terraform Feature Flag Acceptance Project",
		projectKey,
		"Terraform Feature Flag Acceptance Environment",
		environmentKey,
		"Feature Flag Cloud acceptance parent",
	)

	steps := []resource.TestStep{
		{
			Config: initialConfig,
			Check: resource.ComposeTestCheckFunc(
				cloudAcceptanceStateChecks(
					"Terraform Feature Flag Acceptance Project",
					projectKey,
					"Terraform Feature Flag Acceptance Environment",
					environmentKey,
					"Feature Flag Cloud acceptance parent",
				),
				cloudFeatureFlagStateChecks(definitions, false),
				cloudFeatureFlagCaptureIDs(
					inventory,
					definitions,
					&projectID,
					&environmentID,
					initialIDs,
					nil,
					projectKey,
					environmentKey,
				),
			),
		},
		{
			Config:   initialConfig,
			PlanOnly: true,
		},
	}
	for _, definition := range definitions {
		definition := definition
		steps = append(steps, resource.TestStep{
			ResourceName:      cloudFeatureFlagResourceAddress(definition),
			ImportState:       true,
			ImportStateIdFunc: cloudFeatureFlagImportID(&environmentID, definition.key),
			ImportStateVerify: true,
		})
	}
	steps = append(steps,
		resource.TestStep{
			Config:   initialConfig,
			PlanOnly: true,
		},
		resource.TestStep{
			PreConfig: func() {
				cloudFeatureFlagDriftNames(
					t,
					apiClient,
					definitions,
					environmentID,
					initialIDs,
				)
			},
			Config: initialConfig,
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: cloudFeatureFlagActionChecks(
					definitions,
					plancheck.ResourceActionUpdate,
				),
			},
			Check: resource.ComposeTestCheckFunc(
				cloudFeatureFlagStateChecks(definitions, false),
				cloudFeatureFlagIDsUnchanged(definitions, initialIDs),
				cloudFeatureFlagRefreshUIProof(
					proxyAPIClient,
					definitions,
					&environmentID,
					initialIDs,
				),
				func(*testingterraform.State) error {
					return cloudProxy.verifyUIPreservation(len(definitions))
				},
			),
		},
		resource.TestStep{
			Config: replacementConfig,
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: cloudFeatureFlagActionChecks(
					definitions,
					plancheck.ResourceActionReplace,
				),
			},
			Check: resource.ComposeTestCheckFunc(
				cloudFeatureFlagStateChecks(definitions, true),
				cloudFeatureFlagCaptureIDs(
					inventory,
					definitions,
					&projectID,
					&environmentID,
					replacementIDs,
					initialIDs,
					projectKey,
					environmentKey,
				),
			),
		},
		resource.TestStep{
			PreConfig: func() {
				cloudFeatureFlagDeleteOutOfBand(
					t,
					inventory,
					definitions,
					environmentID,
				)
			},
			Config: replacementConfig,
			Check: resource.ComposeTestCheckFunc(
				cloudFeatureFlagStateChecks(definitions, true),
				cloudFeatureFlagCaptureIDs(
					inventory,
					definitions,
					&projectID,
					&environmentID,
					recreatedIDs,
					replacementIDs,
					projectKey,
					environmentKey,
				),
			),
		},
		resource.TestStep{
			Config: parentOnlyConfig,
			ConfigPlanChecks: resource.ConfigPlanChecks{
				PreApply: cloudFeatureFlagActionChecks(
					definitions,
					plancheck.ResourceActionDestroy,
				),
			},
			Check: resource.ComposeTestCheckFunc(
				cloudFeatureFlagExactZeroCheck(
					inventory,
					&environmentID,
					featureFlagKeys,
				),
				func(*testingterraform.State) error {
					if err := cloudProxy.verifyUIPreservation(len(definitions)); err != nil {
						return err
					}
					lifecycleCompleted = true
					return nil
				},
			),
		},
	)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"featbit": providerserver.NewProtocol6WithError(New("cloud-feature-flag-acceptance")()),
		},
		PreCheck: func() {
			cloudFeatureFlagAcceptancePreCheck(t)
		},
		CheckDestroy: func(*testingterraform.State) error {
			ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
			defer cancel()
			if err := inventory.cleanupAndVerify(ctx); err != nil {
				return err
			}
			if !lifecycleCompleted {
				return nil
			}
			return cloudProxy.verifyUIPreservation(len(definitions))
		},
		Steps: steps,
	})
}

func TestCloudFeatureFlagInventoryCleansUnreturnedCreatesBeforeParents(t *testing.T) {
	fixture := newCrossResourceProtocolFixture(t)
	defer fixture.close()
	apiURL, err := parseAPIURL(fixture.apiOrigin())
	if err != nil {
		t.Fatal("could not configure the Feature Flag cleanup fixture URL")
	}
	apiClient, err := client.New(apiURL, syntheticProviderAccessToken, client.Options{
		HTTPTimeout:     client.DefaultHTTPTimeout,
		MaxConcurrency:  client.DefaultMaxConcurrency,
		MaxRetries:      0,
		ProviderVersion: "protocol-test",
	})
	if err != nil {
		t.Fatal("could not construct the Feature Flag cleanup fixture client")
	}

	const (
		projectKey     = "cloud-flag-cleanup-project"
		environmentKey = "cloud-flag-cleanup-environment"
		featureFlagKey = "cloud-flag-cleanup-key"
	)
	inventory := newCloudFeatureFlagInventory(
		apiClient,
		[]string{projectKey},
		[]string{environmentKey},
		[]string{featureFlagKey},
	)
	ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
	defer cancel()

	project, err := apiClient.CreateProject(ctx, client.CreateProjectRequest{
		Name: "Cloud cleanup Project",
		Key:  projectKey,
	})
	if err != nil {
		t.Fatal("could not create the Feature Flag cleanup Project")
	}
	environment, err := apiClient.CreateEnvironment(
		ctx,
		project.ID,
		client.CreateEnvironmentRequest{
			Name: "Cloud cleanup Environment",
			Key:  environmentKey,
		},
	)
	if err != nil {
		t.Fatal("could not create the Feature Flag cleanup Environment")
	}
	canonical, seed, err := canonicalizePlannedFeatureFlag(
		environment.ID,
		featureFlagKey,
		"Cloud cleanup Feature Flag",
		"",
		featureFlagVariationTypeString,
		[]featureFlagVariationInput{{Name: "Value", Value: "cleanup"}},
	)
	if err != nil {
		t.Fatal("could not construct the Feature Flag cleanup definition")
	}
	if _, err := apiClient.CreateFeatureFlag(
		ctx,
		environment.ID,
		expandFeatureFlagCreateRequest(canonical, seed),
	); err != nil {
		t.Fatal("could not create the unreturned Feature Flag cleanup object")
	}

	// Deliberately register nothing after Create. Cleanup must discover only
	// the unique exact keys supplied before mutation, then remove child first.
	if err := inventory.cleanupAndVerify(ctx); err != nil {
		t.Fatal("Feature Flag cleanup inventory did not remove unreturned objects")
	}
	if fixture.flags.objectCount() != 0 || fixture.project.environmentCount() != 0 ||
		fixture.project.projectCount() != 0 || fixture.violationCount() != 0 {
		t.Fatal("Feature Flag cleanup inventory did not reach ordered exact zero")
	}
}

func TestCloudFeatureFlagProxyProvesNameOnlyUIPreservation(t *testing.T) {
	t.Parallel()

	const (
		exactPath = "/api/v1/envs/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa/" +
			"feature-flags/cloud-ui-proof"
		baseline = `{"success":true,"data":{"name":"Before","isEnabled":false,` +
			`"disabledVariationId":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",` +
			`"targetUsers":[],"rules":[],"fallthrough":{"dispatchKey":"before"},` +
			`"exptIncludeAllTargets":false,"tags":["cloud-proof"],"isArchived":false}}`
	)
	tests := []struct {
		name        string
		after       string
		wantFailure bool
	}{
		{
			name: "name only",
			after: `{"success":true,"data":{"name":"After","isEnabled":false,` +
				`"disabledVariationId":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",` +
				`"targetUsers":[],"rules":[],"fallthrough":{"dispatchKey":"before"},` +
				`"exptIncludeAllTargets":false,"tags":["cloud-proof"],"isArchived":false}}`,
		},
		{
			name: "operational field changed",
			after: `{"success":true,"data":{"name":"After","isEnabled":false,` +
				`"disabledVariationId":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",` +
				`"targetUsers":[],"rules":[],"fallthrough":{"dispatchKey":"after"},` +
				`"exptIncludeAllTargets":false,"tags":["cloud-proof"],"isArchived":false}}`,
			wantFailure: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			proxy := &cloudFeatureFlagProxy{
				uiByPath:      make(map[string]cloudSettingsFingerprint),
				pendingByPath: make(map[string]cloudSettingsFingerprint),
			}
			proxy.observeExactRead(exactPath, []byte(baseline))
			proxy.observeNameUpdate(exactPath+"/name", []byte(`{"name":"After"}`))
			proxy.observeExactRead(exactPath, []byte(test.after))
			err := proxy.verifyUIPreservation(1)
			if test.wantFailure && err == nil {
				t.Fatal("UI fingerprint did not detect an operational field change")
			}
			if !test.wantFailure && err != nil {
				t.Fatal("UI fingerprint rejected a name-only update")
			}
		})
	}
}

func cloudFeatureFlagAcceptancePreCheck(t *testing.T) {
	t.Helper()
	cloudAcceptancePreCheck(t)
}

func cloudFeatureFlagDefinitions(prefix string) []cloudFeatureFlagDefinition {
	return []cloudFeatureFlagDefinition{
		{
			terraformName: "cloud_boolean",
			name:          "Terraform Cloud Boolean Flag",
			key:           prefix + "-boolean",
			variationType: featureFlagVariationTypeBoolean,
			variations: []featureFlagVariationInput{
				{Name: "Enabled", Value: "true"},
				{Name: "Disabled", Value: "false"},
			},
		},
		{
			terraformName: "cloud_string",
			name:          "Terraform Cloud String Flag",
			key:           prefix + "-string",
			variationType: featureFlagVariationTypeString,
			variations: []featureFlagVariationInput{
				{Name: "First", Value: "cloud-alpha"},
				{Name: "Second", Value: " cloud-beta "},
			},
		},
		{
			terraformName: "cloud_number",
			name:          "Terraform Cloud Number Flag",
			key:           prefix + "-number",
			variationType: featureFlagVariationTypeNumber,
			variations: []featureFlagVariationInput{
				{Name: "Precise", Value: "90071992547409931234567890"},
				{Name: "Small", Value: "0.001"},
			},
		},
		{
			terraformName: "cloud_json",
			name:          "Terraform Cloud JSON Flag",
			key:           prefix + "-json",
			variationType: featureFlagVariationTypeJSON,
			variations: []featureFlagVariationInput{
				{Name: "Object", Value: `{"a":1,"b":2}`},
				{Name: "Array", Value: `[true,false]`},
			},
		},
	}
}

func cloudFeatureFlagKeys(definitions []cloudFeatureFlagDefinition) []string {
	keys := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		keys = append(keys, definition.key)
	}
	return keys
}

func cloudFeatureFlagConfig(
	apiOrigin string,
	projectKey string,
	environmentKey string,
	definitions []cloudFeatureFlagDefinition,
	replacement bool,
) string {
	config := cloudAcceptanceConfig(
		apiOrigin,
		"Terraform Feature Flag Acceptance Project",
		projectKey,
		"Terraform Feature Flag Acceptance Environment",
		environmentKey,
		"Feature Flag Cloud acceptance parent",
	)
	for _, definition := range definitions {
		variations := make([]string, 0, len(definition.variations))
		for _, variation := range definition.variations {
			variations = append(variations, fmt.Sprintf(`
    {
      name  = %q
      value = %q
    }`, variation.Name, variation.Value))
		}
		description := "Cloud Feature Flag definition"
		if replacement {
			description = "Cloud Feature Flag replacement definition"
		}
		config += fmt.Sprintf(`

resource "featbit_feature_flag" %q {
  environment_id = featbit_environment.cloud_child.id
  name           = %q
  description    = %q
  key            = %q
  variation_type = %q
  variations     = [%s
  ]
}

data "featbit_feature_flag" %q {
  environment_id = featbit_feature_flag.%s.environment_id
  key            = featbit_feature_flag.%s.key
}
`, definition.terraformName, definition.name, description, definition.key,
			definition.variationType, joinFeatureFlagProtocolVariations(variations),
			definition.terraformName, definition.terraformName, definition.terraformName)
	}
	return config
}

func cloudFeatureFlagStateChecks(
	definitions []cloudFeatureFlagDefinition,
	replacement bool,
) resource.TestCheckFunc {
	checks := make([]resource.TestCheckFunc, 0, len(definitions)*10)
	description := "Cloud Feature Flag definition"
	if replacement {
		description = "Cloud Feature Flag replacement definition"
	}
	for _, definition := range definitions {
		resourceAddress := cloudFeatureFlagResourceAddress(definition)
		dataAddress := cloudFeatureFlagDataAddress(definition)
		checks = append(checks,
			resource.TestCheckResourceAttr(resourceAddress, "name", definition.name),
			resource.TestCheckResourceAttr(resourceAddress, "description", description),
			resource.TestCheckResourceAttr(resourceAddress, "key", definition.key),
			resource.TestCheckResourceAttr(resourceAddress, "variation_type", definition.variationType),
			resource.TestCheckResourceAttr(
				resourceAddress,
				"variations.#",
				fmt.Sprintf("%d", len(definition.variations)),
			),
			resource.TestCheckResourceAttr(dataAddress, "name", definition.name),
			resource.TestCheckResourceAttr(dataAddress, "description", description),
			resource.TestCheckResourceAttr(dataAddress, "key", definition.key),
			resource.TestCheckResourceAttr(dataAddress, "variation_type", definition.variationType),
			resource.TestCheckResourceAttrPair(resourceAddress, "id", dataAddress, "id"),
		)
		for index, variation := range definition.variations {
			checks = append(checks,
				resource.TestCheckResourceAttr(
					resourceAddress,
					fmt.Sprintf("variations.%d.name", index),
					variation.Name,
				),
				resource.TestCheckResourceAttr(
					resourceAddress,
					fmt.Sprintf("variations.%d.value", index),
					variation.Value,
				),
			)
		}
	}
	return resource.ComposeTestCheckFunc(checks...)
}

func cloudFeatureFlagCaptureIDs(
	inventory *cloudFeatureFlagInventory,
	definitions []cloudFeatureFlagDefinition,
	projectID *string,
	environmentID *string,
	destination map[string]string,
	mustDiffer map[string]string,
	projectKey string,
	environmentKey string,
) resource.TestCheckFunc {
	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttrWith(
			cloudAcceptanceProjectResourceName,
			"id",
			func(value string) error {
				if !validUUID(value) || *projectID != "" && value != *projectID {
					return fmt.Errorf("Cloud Feature Flag parent Project identity changed")
				}
				*projectID = value
				inventory.registerProject(value, projectKey)
				return nil
			},
		),
		resource.TestCheckResourceAttrWith(
			cloudAcceptanceEnvironmentName,
			"id",
			func(value string) error {
				if !validUUID(value) || *environmentID != "" && value != *environmentID {
					return fmt.Errorf("Cloud Feature Flag parent Environment identity changed")
				}
				*environmentID = value
				inventory.registerEnvironment(*projectID, value, environmentKey)
				return nil
			},
		),
	}
	for _, definition := range definitions {
		definition := definition
		checks = append(checks, resource.TestCheckResourceAttrWith(
			cloudFeatureFlagResourceAddress(definition),
			"id",
			func(value string) error {
				if !validUUID(value) || mustDiffer != nil && value == mustDiffer[definition.terraformName] {
					return fmt.Errorf("Cloud Feature Flag identity did not satisfy replacement or recreation")
				}
				destination[definition.terraformName] = value
				inventory.registerFeatureFlag(*environmentID, value, definition.key)
				return nil
			},
		))
	}
	return resource.ComposeTestCheckFunc(checks...)
}

func cloudFeatureFlagIDsUnchanged(
	definitions []cloudFeatureFlagDefinition,
	ids map[string]string,
) resource.TestCheckFunc {
	checks := make([]resource.TestCheckFunc, 0, len(definitions))
	for _, definition := range definitions {
		definition := definition
		checks = append(checks, resource.TestCheckResourceAttrWith(
			cloudFeatureFlagResourceAddress(definition),
			"id",
			func(value string) error {
				if value != ids[definition.terraformName] {
					return fmt.Errorf("Cloud Feature Flag name repair changed identity")
				}
				return nil
			},
		))
	}
	return resource.ComposeTestCheckFunc(checks...)
}

func cloudFeatureFlagImportID(
	environmentID *string,
	key string,
) resource.ImportStateIdFunc {
	return func(*testingterraform.State) (string, error) {
		if !validUUID(*environmentID) {
			return "", fmt.Errorf("Cloud Feature Flag Import Environment is unavailable")
		}
		return *environmentID + "/" + key, nil
	}
}

func cloudFeatureFlagDriftNames(
	t *testing.T,
	apiClient *client.Client,
	definitions []cloudFeatureFlagDefinition,
	environmentID string,
	ids map[string]string,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
	defer cancel()
	for _, definition := range definitions {
		driftName := "External Cloud Feature Flag Drift " + definition.terraformName
		err := apiClient.UpdateFeatureFlagName(
			ctx,
			environmentID,
			definition.key,
			ids[definition.terraformName],
			client.UpdateFeatureFlagNameRequest{Name: driftName},
		)
		if err == nil {
			continue
		}
		if !mutationNeedsReconciliation(err) {
			t.Fatalf("could not prepare external Cloud Feature Flag name drift: %v", err)
		}
		flag, status, resolveErr := apiClient.ResolveFeatureFlag(
			ctx,
			environmentID,
			definition.key,
		)
		if resolveErr != nil {
			t.Fatalf(
				"could not reconcile external Cloud Feature Flag name drift: %v",
				resolveErr,
			)
		}
		if status != client.FeatureFlagStatusActive ||
			!client.EqualUUID(flag.ID, ids[definition.terraformName]) ||
			flag.Name != driftName {
			t.Fatal("external Cloud Feature Flag name drift was not confirmed exactly")
		}
	}
}

func cloudFeatureFlagRefreshUIProof(
	apiClient *client.Client,
	definitions []cloudFeatureFlagDefinition,
	environmentID *string,
	ids map[string]string,
) resource.TestCheckFunc {
	return func(*testingterraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
		defer cancel()
		for _, definition := range definitions {
			flag, status, err := apiClient.GetFeatureFlag(
				ctx,
				*environmentID,
				definition.key,
			)
			if err != nil {
				return fmt.Errorf("Cloud Feature Flag UI proof read failed: %w", err)
			}
			if status != client.FeatureFlagStatusActive ||
				!client.EqualUUID(flag.ID, ids[definition.terraformName]) {
				return fmt.Errorf("Cloud Feature Flag UI proof read did not retain exact identity")
			}
		}
		return nil
	}
}

func cloudFeatureFlagDeleteOutOfBand(
	t *testing.T,
	inventory *cloudFeatureFlagInventory,
	definitions []cloudFeatureFlagDefinition,
	environmentID string,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
	defer cancel()
	for _, definition := range definitions {
		if err := inventory.deleteFeatureFlag(ctx, environmentID, definition.key); err != nil {
			t.Fatal("could not prepare out-of-band Cloud Feature Flag deletion")
		}
	}
}

func cloudFeatureFlagExactZeroCheck(
	inventory *cloudFeatureFlagInventory,
	environmentID *string,
	keys []string,
) resource.TestCheckFunc {
	return func(*testingterraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
		defer cancel()
		return inventory.verifyFeatureFlagsAbsent(ctx, *environmentID, keys)
	}
}

func cloudFeatureFlagActionChecks(
	definitions []cloudFeatureFlagDefinition,
	action plancheck.ResourceActionType,
) []plancheck.PlanCheck {
	checks := make([]plancheck.PlanCheck, 0, len(definitions))
	for _, definition := range definitions {
		checks = append(checks, plancheck.ExpectResourceAction(
			cloudFeatureFlagResourceAddress(definition),
			action,
		))
	}
	return checks
}

func cloudFeatureFlagResourceAddress(definition cloudFeatureFlagDefinition) string {
	return "featbit_feature_flag." + definition.terraformName
}

func cloudFeatureFlagDataAddress(definition cloudFeatureFlagDefinition) string {
	return "data.featbit_feature_flag." + definition.terraformName
}
