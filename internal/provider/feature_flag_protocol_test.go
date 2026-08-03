// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const (
	featureFlagProtocolResourceName   = "featbit_feature_flag.test"
	featureFlagProtocolDataSourceName = "data.featbit_feature_flag.exact"
)

type featureFlagProtocolDefinition struct {
	EnvironmentID string
	Name          string
	Description   string
	Key           string
	VariationType string
	Variations    []featureFlagVariationInput
}

func TestFeatureFlagProtocolFourTypeLifecycle(t *testing.T) {
	tests := []struct {
		name       string
		definition featureFlagProtocolDefinition
	}{
		{
			name: "boolean",
			definition: featureFlagProtocolDefinition{
				EnvironmentID: providerEnvironmentA,
				Name:          "Protocol Boolean Flag",
				Description:   "Boolean definition",
				Key:           "protocol-boolean-4",
				VariationType: featureFlagVariationTypeBoolean,
				Variations: []featureFlagVariationInput{
					{Name: "Enabled", Value: "true"},
					{Name: "Disabled", Value: "false"},
				},
			},
		},
		{
			name: "string",
			definition: featureFlagProtocolDefinition{
				EnvironmentID: providerEnvironmentA,
				Name:          "Protocol String Flag",
				Description:   "String definition",
				Key:           "protocol-string-0",
				VariationType: featureFlagVariationTypeString,
				Variations: []featureFlagVariationInput{
					{Name: "First", Value: " exact string "},
					{Name: "Second", Value: "second"},
				},
			},
		},
		{
			name: "number",
			definition: featureFlagProtocolDefinition{
				EnvironmentID: providerEnvironmentA,
				Name:          "Protocol Number Flag",
				Description:   "Number definition",
				Key:           "protocol-number-1",
				VariationType: featureFlagVariationTypeNumber,
				Variations: []featureFlagVariationInput{
					{Name: "Precise", Value: "100"},
					{Name: "Large", Value: "90071992547409931234567890"},
				},
			},
		},
		{
			name: "json",
			definition: featureFlagProtocolDefinition{
				EnvironmentID: providerEnvironmentA,
				Name:          "Protocol JSON Flag",
				Description:   "JSON definition",
				Key:           "protocol-json-0",
				VariationType: featureFlagVariationTypeJSON,
				Variations: []featureFlagVariationInput{
					{Name: "Object", Value: `{"a":1,"b":2}`},
					{Name: "Array", Value: `[3,2,1]`},
				},
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			runFeatureFlagProtocolLifecycle(t, test.definition)
		})
	}
}

func runFeatureFlagProtocolLifecycle(
	t *testing.T,
	initial featureFlagProtocolDefinition,
) {
	t.Helper()
	fixture := newFeatureFlagProtocolFixture(t)
	t.Cleanup(func() {
		fixture.setDirectReadFailure(false)
		if count := fixture.objectCount(); count != 0 {
			t.Errorf("Feature Flag protocol fixture teardown count = %d, want 0", count)
		}
		fixture.close()
	})

	descriptionReplacement := cloneFeatureFlagProtocolDefinition(initial)
	descriptionReplacement.Description += " replacement"
	variationReplacement := cloneFeatureFlagProtocolDefinition(descriptionReplacement)
	variationReplacement.Variations = featureFlagProtocolReplacementVariations(
		variationReplacement.VariationType,
	)
	typeReplacement := cloneFeatureFlagProtocolDefinition(variationReplacement)
	typeReplacement.VariationType = featureFlagProtocolNextType(initial.VariationType)
	typeReplacement.Variations = featureFlagProtocolValuesForType(typeReplacement.VariationType)
	keyReplacement := cloneFeatureFlagProtocolDefinition(typeReplacement)
	keyReplacement.Key += "-key-replacement"
	environmentReplacement := cloneFeatureFlagProtocolDefinition(keyReplacement)
	environmentReplacement.EnvironmentID = providerEnvironmentB

	var initialID string
	var descriptionID string
	var variationID string
	var typeID string
	var keyID string
	var environmentID string
	var recreatedID string
	var protectedID string
	var fallbackBaseline int

	initialConfig := featureFlagProtocolConfig(fixture.apiOrigin(), initial)
	descriptionConfig := featureFlagProtocolConfig(fixture.apiOrigin(), descriptionReplacement)
	variationConfig := featureFlagProtocolConfig(fixture.apiOrigin(), variationReplacement)
	typeConfig := featureFlagProtocolConfig(fixture.apiOrigin(), typeReplacement)
	keyConfig := featureFlagProtocolConfig(fixture.apiOrigin(), keyReplacement)
	environmentConfig := featureFlagProtocolConfig(fixture.apiOrigin(), environmentReplacement)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"featbit": providerserver.NewProtocol6WithError(New("protocol-test")()),
		},
		Steps: []resource.TestStep{
			{
				Config: initialConfig,
				Check: resource.ComposeTestCheckFunc(
					featureFlagProtocolStateChecks(initial),
					captureFeatureFlagProtocolID(&initialID, ""),
					checkFeatureFlagProtocolFixture(fixture, initial),
				),
			},
			{
				Config:   initialConfig,
				PlanOnly: true,
			},
			{
				PreConfig: func() {
					var err error
					protectedID, err = fixture.protectCustomUI(initial.EnvironmentID, initial.Key)
					if err != nil {
						t.Errorf("prepare protected UI-owned state: %v", err)
					}
					fixture.setReverseVariations(true)
				},
				ResourceName:      featureFlagProtocolResourceName,
				ImportState:       true,
				ImportStateId:     initial.EnvironmentID + "/" + initial.Key,
				ImportStateVerify: true,
			},
			{
				Config:   initialConfig,
				PlanOnly: true,
			},
			{
				PreConfig: func() {
					if err := fixture.rename(initial.EnvironmentID, initial.Key, "External Name Drift"); err != nil {
						t.Errorf("prepare external Feature Flag name drift: %v", err)
					}
				},
				Config: initialConfig,
				Check: resource.ComposeTestCheckFunc(
					featureFlagProtocolStateChecks(initial),
					checkFeatureFlagProtocolFixture(fixture, initial),
					resource.TestCheckResourceAttrWith(
						featureFlagProtocolResourceName,
						"id",
						func(value string) error {
							if value != initialID {
								return fmt.Errorf("name drift repair replaced the Feature Flag")
							}
							if !fixture.uiPreserved(protectedID) {
								return fmt.Errorf("name drift repair changed UI-owned state")
							}
							return nil
						},
					),
				),
			},
			{
				PreConfig: func() {
					fallbackBaseline = fixture.directFallbacks()
					fixture.setDirectReadFailure(true)
					fixture.setReverseVariations(true)
				},
				Config: initialConfig,
				Check: resource.ComposeTestCheckFunc(
					featureFlagProtocolStateChecks(initial),
					resource.TestCheckResourceAttrWith(
						featureFlagProtocolResourceName,
						"id",
						func(string) error {
							fixture.setDirectReadFailure(false)
							if fixture.directFallbacks() <= fallbackBaseline {
								return fmt.Errorf("Protocol lifecycle did not exercise complete collection fallback")
							}
							if !fixture.uiPreserved(protectedID) {
								return fmt.Errorf("fallback/reordering changed UI-owned state")
							}
							return nil
						},
					),
				),
			},
			{
				Config: descriptionConfig,
				Check: resource.ComposeTestCheckFunc(
					featureFlagProtocolStateChecks(descriptionReplacement),
					captureFeatureFlagProtocolID(&descriptionID, initialID),
					checkFeatureFlagProtocolFixture(fixture, descriptionReplacement),
					resource.TestCheckResourceAttrWith(
						featureFlagProtocolResourceName,
						"id",
						func(string) error {
							if !fixture.uiPreserved(protectedID) {
								return fmt.Errorf("replacement rewrote protected UI-owned state before deletion")
							}
							return nil
						},
					),
				),
			},
			{
				Config: variationConfig,
				Check: resource.ComposeTestCheckFunc(
					featureFlagProtocolStateChecks(variationReplacement),
					captureFeatureFlagProtocolID(&variationID, descriptionID),
					checkFeatureFlagProtocolFixture(fixture, variationReplacement),
				),
			},
			{
				Config: typeConfig,
				Check: resource.ComposeTestCheckFunc(
					featureFlagProtocolStateChecks(typeReplacement),
					captureFeatureFlagProtocolID(&typeID, variationID),
					checkFeatureFlagProtocolFixture(fixture, typeReplacement),
				),
			},
			{
				Config: keyConfig,
				Check: resource.ComposeTestCheckFunc(
					featureFlagProtocolStateChecks(keyReplacement),
					captureFeatureFlagProtocolID(&keyID, typeID),
					checkFeatureFlagProtocolFixture(fixture, keyReplacement),
				),
			},
			{
				Config: environmentConfig,
				Check: resource.ComposeTestCheckFunc(
					featureFlagProtocolStateChecks(environmentReplacement),
					captureFeatureFlagProtocolID(&environmentID, keyID),
					checkFeatureFlagProtocolFixture(fixture, environmentReplacement),
				),
			},
			{
				PreConfig: func() {
					if err := fixture.removeActive(
						environmentReplacement.EnvironmentID,
						environmentReplacement.Key,
					); err != nil {
						t.Errorf("prepare out-of-band Feature Flag deletion: %v", err)
					}
				},
				Config: environmentConfig,
				Check: resource.ComposeTestCheckFunc(
					featureFlagProtocolStateChecks(environmentReplacement),
					captureFeatureFlagProtocolID(&recreatedID, environmentID),
					checkFeatureFlagProtocolFixture(fixture, environmentReplacement),
				),
			},
			{
				PreConfig: func() {
					if err := fixture.archiveExternal(
						environmentReplacement.EnvironmentID,
						environmentReplacement.Key,
					); err != nil {
						t.Errorf("prepare external Feature Flag archive: %v", err)
					}
				},
				Config:      environmentConfig,
				ExpectError: regexp.MustCompile(`(?i)feature flag.*archived|archived.*feature flag`),
			},
			{
				PreConfig: func() {
					if err := fixture.restoreExternal(
						environmentReplacement.EnvironmentID,
						environmentReplacement.Key,
					); err != nil {
						t.Errorf("restore externally archived Feature Flag for teardown: %v", err)
					}
				},
				Config: environmentConfig,
				Check: resource.ComposeTestCheckFunc(
					featureFlagProtocolStateChecks(environmentReplacement),
					resource.TestCheckResourceAttrWith(
						featureFlagProtocolResourceName,
						"id",
						func(value string) error {
							if recreatedID == "" || value != recreatedID {
								return fmt.Errorf("external archive recovery changed the Feature Flag identity")
							}
							return nil
						},
					),
				),
			},
		},
	})

	if count := fixture.objectCount(); count != 0 {
		t.Fatalf("Feature Flag count after Protocol v6 destroy = %d, want 0", count)
	}
	if violations := fixture.violationSnapshot(); len(violations) != 0 {
		t.Fatalf("Feature Flag protocol fixture request violations = %v", violations)
	}
	creates, nameUpdates, archives, deletes := fixture.mutationCounts()
	if creates != 7 || nameUpdates != 1 || archives != 6 || deletes != 6 {
		t.Fatalf(
			"Feature Flag mutation counts = create/name/archive/delete %d/%d/%d/%d, want 7/1/6/6",
			creates,
			nameUpdates,
			archives,
			deletes,
		)
	}
	assertFeatureFlagProtocolTeardownProof(t, fixture.requestSnapshot())
}

func featureFlagProtocolConfig(
	apiOrigin string,
	definition featureFlagProtocolDefinition,
) string {
	variations := make([]string, 0, len(definition.Variations))
	for _, variation := range definition.Variations {
		variations = append(variations, fmt.Sprintf(`
    {
      name  = %q
      value = %q
    }`, variation.Name, variation.Value))
	}
	return fmt.Sprintf(`
provider "featbit" {
  api_url              = %q
  access_token         = %q
  http_timeout_seconds = 5
  max_concurrency      = 4
  max_retries          = 0
}

resource "featbit_feature_flag" "test" {
  environment_id = %q
  name           = %q
  description    = %q
  key            = %q
  variation_type = %q
  variations     = [%s
  ]
}

data "featbit_feature_flag" "exact" {
  environment_id = featbit_feature_flag.test.environment_id
  key            = featbit_feature_flag.test.key
}
`, apiOrigin, syntheticProviderAccessToken, definition.EnvironmentID,
		definition.Name, definition.Description, definition.Key,
		definition.VariationType, joinFeatureFlagProtocolVariations(variations))
}

func joinFeatureFlagProtocolVariations(variations []string) string {
	result := ""
	for index, variation := range variations {
		if index > 0 {
			result += ","
		}
		result += variation
	}
	return result
}

func featureFlagProtocolStateChecks(
	definition featureFlagProtocolDefinition,
) resource.TestCheckFunc {
	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(featureFlagProtocolResourceName, "environment_id", definition.EnvironmentID),
		resource.TestCheckResourceAttr(featureFlagProtocolResourceName, "name", definition.Name),
		resource.TestCheckResourceAttr(featureFlagProtocolResourceName, "description", definition.Description),
		resource.TestCheckResourceAttr(featureFlagProtocolResourceName, "key", definition.Key),
		resource.TestCheckResourceAttr(featureFlagProtocolResourceName, "variation_type", definition.VariationType),
		resource.TestCheckResourceAttr(featureFlagProtocolResourceName, "variations.#", strconv.Itoa(len(definition.Variations))),
		resource.TestCheckResourceAttr(featureFlagProtocolDataSourceName, "environment_id", definition.EnvironmentID),
		resource.TestCheckResourceAttr(featureFlagProtocolDataSourceName, "name", definition.Name),
		resource.TestCheckResourceAttr(featureFlagProtocolDataSourceName, "description", definition.Description),
		resource.TestCheckResourceAttr(featureFlagProtocolDataSourceName, "key", definition.Key),
		resource.TestCheckResourceAttr(featureFlagProtocolDataSourceName, "variation_type", definition.VariationType),
		resource.TestCheckResourceAttr(featureFlagProtocolDataSourceName, "variations.#", strconv.Itoa(len(definition.Variations))),
		resource.TestCheckResourceAttrPair(featureFlagProtocolResourceName, "id", featureFlagProtocolDataSourceName, "id"),
	}
	for index, variation := range definition.Variations {
		checks = append(checks,
			resource.TestCheckResourceAttr(
				featureFlagProtocolResourceName,
				fmt.Sprintf("variations.%d.name", index),
				variation.Name,
			),
			resource.TestCheckResourceAttr(
				featureFlagProtocolResourceName,
				fmt.Sprintf("variations.%d.value", index),
				variation.Value,
			),
			resource.TestCheckResourceAttrWith(
				featureFlagProtocolResourceName,
				fmt.Sprintf("variations.%d.id", index),
				func(value string) error {
					if !validUUID(value) {
						return fmt.Errorf("resource variation ID is not a valid UUID")
					}
					return nil
				},
			),
		)
	}
	return resource.ComposeTestCheckFunc(checks...)
}

func captureFeatureFlagProtocolID(
	destination *string,
	mustDifferFrom string,
) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(
		featureFlagProtocolResourceName,
		"id",
		func(value string) error {
			if !validUUID(value) {
				return fmt.Errorf("Feature Flag state ID is not a valid UUID")
			}
			if mustDifferFrom != "" && value == mustDifferFrom {
				return fmt.Errorf("Feature Flag replacement or recreation retained the prior UUID")
			}
			*destination = value
			return nil
		},
	)
}

func checkFeatureFlagProtocolFixture(
	fixture *featureFlagProtocolFixture,
	definition featureFlagProtocolDefinition,
) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(
		featureFlagProtocolResourceName,
		"id",
		func(value string) error {
			fixtureID, found := fixture.currentID(definition.EnvironmentID, definition.Key)
			if !found || fixtureID != value {
				return fmt.Errorf("Feature Flag state identity is absent from the exact fixture scope")
			}
			name, description, variationType, variationCount, found := fixture.currentDefinition(
				definition.EnvironmentID,
				definition.Key,
			)
			if !found || name != definition.Name || description != definition.Description ||
				variationType != definition.VariationType || variationCount != len(definition.Variations) {
				return fmt.Errorf("Feature Flag fixture definition did not converge")
			}
			if fixture.objectCount() != 1 {
				return fmt.Errorf("Feature Flag fixture count did not converge to one")
			}
			return nil
		},
	)
}

func cloneFeatureFlagProtocolDefinition(
	definition featureFlagProtocolDefinition,
) featureFlagProtocolDefinition {
	definition.Variations = append([]featureFlagVariationInput(nil), definition.Variations...)
	return definition
}

func featureFlagProtocolReplacementVariations(
	variationType string,
) []featureFlagVariationInput {
	switch variationType {
	case featureFlagVariationTypeBoolean:
		return []featureFlagVariationInput{
			{Name: "Enabled", Value: "true"},
			{Name: "Also Enabled", Value: "TRUE"},
		}
	case featureFlagVariationTypeString:
		return []featureFlagVariationInput{
			{Name: "First", Value: " exact string "},
			{Name: "Replacement", Value: "replacement"},
		}
	case featureFlagVariationTypeNumber:
		return []featureFlagVariationInput{
			{Name: "Precise", Value: "1e2"},
			{Name: "Replacement", Value: "4.200e1"},
		}
	default:
		return []featureFlagVariationInput{
			{Name: "Object", Value: `{"a":1,"b":2}`},
			{Name: "Replacement", Value: `{"replacement": true}`},
		}
	}
}

func featureFlagProtocolNextType(variationType string) string {
	switch variationType {
	case featureFlagVariationTypeBoolean:
		return featureFlagVariationTypeString
	case featureFlagVariationTypeString:
		return featureFlagVariationTypeNumber
	case featureFlagVariationTypeNumber:
		return featureFlagVariationTypeJSON
	default:
		return featureFlagVariationTypeBoolean
	}
}

func featureFlagProtocolValuesForType(variationType string) []featureFlagVariationInput {
	switch variationType {
	case featureFlagVariationTypeBoolean:
		return []featureFlagVariationInput{
			{Name: "Enabled", Value: "TRUE"},
			{Name: "Disabled", Value: "false"},
		}
	case featureFlagVariationTypeString:
		return []featureFlagVariationInput{
			{Name: "First", Value: "first"},
			{Name: "Second", Value: " second "},
		}
	case featureFlagVariationTypeNumber:
		return []featureFlagVariationInput{
			{Name: "First", Value: "42.00"},
			{Name: "Second", Value: "1e-3"},
		}
	default:
		return []featureFlagVariationInput{
			{Name: "Object", Value: `{"z":0,"a":1}`},
			{Name: "Array", Value: `[true, false]`},
		}
	}
}

func assertFeatureFlagProtocolTeardownProof(
	t *testing.T,
	requests []featureFlagFixtureRequest,
) {
	t.Helper()
	if len(requests) < 4 {
		t.Fatal("Feature Flag Protocol recorder did not observe final teardown proof")
	}
	last := requests[len(requests)-1]
	if last.Method != http.MethodGet ||
		last.Path != "/api/v1/envs/"+providerEnvironmentB+"/feature-flags" ||
		last.Query != "IsArchived=true&PageIndex=1&PageSize=100" {
		t.Fatalf("final Feature Flag request did not complete the archived zero-proof view: %v", last)
	}
	for _, request := range requests {
		if request.Method == http.MethodPatch ||
			strings.Contains(request.Path, "/restore") ||
			strings.Contains(request.Path, "/toggle") ||
			strings.Contains(request.Path, "/variations") ||
			strings.Contains(request.Path, "/targeting") ||
			strings.Contains(request.Path, "/tags") ||
			strings.Contains(request.Path, "/clone") {
			t.Fatalf("Protocol recorder observed a forbidden Feature Flag operation")
		}
	}
}
