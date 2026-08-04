// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	testingterraform "github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	segmentProtocolResourceName       = "featbit_segment.test"
	segmentProtocolExactDataName      = "data.featbit_segment.exact"
	segmentProtocolSharedDataName     = "data.featbit_segment.shared"
	segmentProtocolSharedResourceName = "featbit_segment.shared_collision"
)

type segmentProtocolDefinition struct {
	EnvironmentID string
	Name          string
	Key           string
	Description   string
	Scopes        []string
	Included      []string
	Excluded      []string
	Rules         []segmentProtocolRuleDefinition
	Tags          []string
}

type segmentProtocolRuleDefinition struct {
	Name       string
	Conditions []segmentProtocolConditionDefinition
}

type segmentProtocolConditionDefinition struct {
	Property string
	Operator string
	Value    string
}

func TestSegmentProtocolLifecycle(t *testing.T) {
	fixture := newSegmentProtocolFixture(t)
	t.Cleanup(func() {
		fixture.setDirectReadFailure("", false)
		if count := fixture.managedCount(); count != 0 {
			t.Errorf("Segment protocol fixture teardown count = %d, want 0", count)
		}
		fixture.close()
	})

	initial := segmentProtocolDefinition{
		EnvironmentID: providerEnvironmentA,
		Name:          "Protocol Segment",
		Key:           "protocol-segment",
		Description:   "Protocol definition",
		Scopes:        []string{providerSegmentEnvironmentScope},
		Included:      []string{"protocol-included-z", "protocol-included-a"},
		Excluded:      []string{"protocol-excluded-z", "protocol-excluded-a"},
		Rules: []segmentProtocolRuleDefinition{
			{
				Name: "First Rule",
				Conditions: []segmentProtocolConditionDefinition{
					{
						Property: "country",
						Operator: segmentOperatorEqual,
						Value:    "synthetic-country",
					},
				},
			},
			{
				Name: "Second Rule",
				Conditions: []segmentProtocolConditionDefinition{
					{
						Property: "tier",
						Operator: segmentOperatorNotEqual,
						Value:    "synthetic-tier",
					},
				},
			},
		},
		Tags: []string{"protocol-tag-z", "protocol-tag-a"},
	}
	ordered := cloneSegmentProtocolDefinition(initial)
	ordered.Rules[0], ordered.Rules[1] = ordered.Rules[1], ordered.Rules[0]
	scopeReplacement := cloneSegmentProtocolDefinition(ordered)
	scopeReplacement.Scopes = []string{
		providerSegmentProjectScope + ":env/protocol-scope-replacement",
	}
	keyReplacement := cloneSegmentProtocolDefinition(scopeReplacement)
	keyReplacement.Key = "protocol-segment-replacement"
	environmentReplacement := cloneSegmentProtocolDefinition(keyReplacement)
	environmentReplacement.EnvironmentID = providerEnvironmentB
	guardUpdate := cloneSegmentProtocolDefinition(environmentReplacement)
	guardUpdate.Name = "Protocol Segment unsafe shared update"

	initialConfig := segmentProtocolConfig(fixture, initial, true, "")
	orderedConfig := segmentProtocolConfig(fixture, ordered, true, "")
	scopeConfig := segmentProtocolConfig(fixture, scopeReplacement, true, "")
	keyConfig := segmentProtocolConfig(fixture, keyReplacement, true, "")
	environmentConfig := segmentProtocolConfig(fixture, environmentReplacement, true, "")
	environmentConfigWithoutExact := segmentProtocolConfig(
		fixture,
		environmentReplacement,
		false,
		"",
	)
	sharedCollisionConfig := segmentProtocolConfig(
		fixture,
		environmentReplacement,
		true,
		segmentProtocolSharedCollisionBlock(fixture),
	)
	guardUpdateConfig := segmentProtocolConfig(fixture, guardUpdate, false, "")

	var initialID string
	var scopeID string
	var keyID string
	var environmentID string
	var recreatedID string
	var directFailureBaseline int
	var collisionMutationBaseline int
	var sharedGuardMutationBaseline int
	var archiveMutationBaseline int
	var referenceMutationBaseline int

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"featbit": providerserver.NewProtocol6WithError(New("protocol-test")()),
		},
		Steps: []resource.TestStep{
			{
				Config: initialConfig,
				Check: resource.ComposeTestCheckFunc(
					segmentProtocolStateChecks(fixture, initial, true),
					captureSegmentProtocolID(&initialID, nil),
					checkSegmentProtocolFixture(fixture, initial),
				),
			},
			{
				Config:   initialConfig,
				PlanOnly: true,
			},
			{
				PreConfig: func() {
					fixture.setReverseSets(true)
					fixture.setReverseCollections(true)
				},
				ResourceName:      segmentProtocolResourceName,
				ImportState:       true,
				ImportStateIdFunc: segmentProtocolImportID(&initialID, initial.EnvironmentID),
				ImportStateVerify: true,
			},
			{
				Config:   initialConfig,
				PlanOnly: true,
			},
			{
				PreConfig: func() {
					if err := fixture.driftOwnedFields(initial.EnvironmentID, initial.Key); err != nil {
						t.Errorf("prepare external Segment owned-field drift: %v", err)
					}
				},
				Config: initialConfig,
				Check: resource.ComposeTestCheckFunc(
					segmentProtocolStateChecks(fixture, initial, true),
					checkSegmentProtocolID(&initialID),
					checkSegmentProtocolFixture(fixture, initial),
				),
			},
			{
				PreConfig: func() {
					fixture.setAmbiguousNextMutation(segmentProtocolMutationTargeting)
				},
				Config: orderedConfig,
				Check: resource.ComposeTestCheckFunc(
					segmentProtocolStateChecks(fixture, ordered, true),
					checkSegmentProtocolID(&initialID),
					checkSegmentProtocolFixture(fixture, ordered),
				),
			},
			{
				Config: scopeConfig,
				Check: resource.ComposeTestCheckFunc(
					segmentProtocolStateChecks(fixture, scopeReplacement, true),
					captureSegmentProtocolID(&scopeID, &initialID),
					checkSegmentProtocolFixture(fixture, scopeReplacement),
				),
			},
			{
				Config: keyConfig,
				Check: resource.ComposeTestCheckFunc(
					segmentProtocolStateChecks(fixture, keyReplacement, true),
					captureSegmentProtocolID(&keyID, &scopeID),
					checkSegmentProtocolFixture(fixture, keyReplacement),
				),
			},
			{
				Config: environmentConfig,
				Check: resource.ComposeTestCheckFunc(
					segmentProtocolStateChecks(fixture, environmentReplacement, true),
					captureSegmentProtocolID(&environmentID, &keyID),
					checkSegmentProtocolFixture(fixture, environmentReplacement),
				),
			},
			{
				PreConfig: func() {
					if err := fixture.removeActive(
						environmentReplacement.EnvironmentID,
						environmentReplacement.Key,
					); err != nil {
						t.Errorf("prepare out-of-band Segment deletion: %v", err)
					}
				},
				Config: environmentConfig,
				Check: resource.ComposeTestCheckFunc(
					segmentProtocolStateChecks(fixture, environmentReplacement, true),
					captureSegmentProtocolID(&recreatedID, &environmentID),
					checkSegmentProtocolFixture(fixture, environmentReplacement),
				),
			},
			{
				PreConfig: func() {
					directFailureBaseline = fixture.directFailures()
					fixture.setDirectReadFailure(recreatedID, true)
				},
				Config: environmentConfigWithoutExact,
				ExpectError: regexp.MustCompile(
					`(?i)segment definition is unconfirmed|complete collection views.*active`,
				),
			},
			{
				PreConfig: func() {
					fixture.setDirectReadFailure(recreatedID, false)
					if fixture.directFailures() <= directFailureBaseline {
						t.Error("Protocol lifecycle did not exercise exact-read collection fallback")
					}
				},
				Config: environmentConfig,
				Check: resource.ComposeTestCheckFunc(
					segmentProtocolStateChecks(fixture, environmentReplacement, true),
					checkSegmentProtocolID(&recreatedID),
				),
			},
			{
				PreConfig: func() {
					collisionMutationBaseline = len(fixture.mutationSnapshot())
				},
				Config: sharedCollisionConfig,
				ExpectError: regexp.MustCompile(
					`(?i)segment create preflight failed|active segment.*exact key`,
				),
			},
			{
				PreConfig: func() {
					if len(fixture.mutationSnapshot()) != collisionMutationBaseline {
						t.Error("shared Segment create collision sent a mutation")
					}
					sharedGuardMutationBaseline = len(fixture.mutationSnapshot())
					if err := fixture.setManagedTaxonomy(
						environmentReplacement.EnvironmentID,
						environmentReplacement.Key,
						client.SegmentTypeShared,
						[]string{providerSegmentProjectScope},
					); err != nil {
						t.Errorf("prepare shared Segment mutation guard: %v", err)
					}
				},
				Config: guardUpdateConfig,
				ExpectError: regexp.MustCompile(
					`(?i)unsafe type or scope|shared.*data-source-only`,
				),
			},
			{
				Config:      environmentConfigWithoutExact,
				Destroy:     true,
				ExpectError: regexp.MustCompile(`(?i)unsafe type or scope|shared`),
			},
			{
				PreConfig: func() {
					if len(fixture.mutationSnapshot()) != sharedGuardMutationBaseline {
						t.Error("shared Segment update or destroy guard sent a mutation")
					}
					if err := fixture.setManagedTaxonomy(
						environmentReplacement.EnvironmentID,
						environmentReplacement.Key,
						client.SegmentTypeEnvironmentSpecific,
						environmentReplacement.Scopes,
					); err != nil {
						t.Errorf("restore environment-specific Segment taxonomy: %v", err)
					}
				},
				Config: environmentConfig,
				Check: resource.ComposeTestCheckFunc(
					segmentProtocolStateChecks(fixture, environmentReplacement, true),
					checkSegmentProtocolID(&recreatedID),
				),
			},
			{
				PreConfig: func() {
					archiveMutationBaseline = len(fixture.mutationSnapshot())
					if err := fixture.archiveExternal(
						environmentReplacement.EnvironmentID,
						environmentReplacement.Key,
					); err != nil {
						t.Errorf("prepare external Segment archive: %v", err)
					}
				},
				Config: environmentConfigWithoutExact,
				ExpectError: regexp.MustCompile(
					`(?i)segment.*archived outside terraform|managed.*archived`,
				),
			},
			{
				PreConfig: func() {
					if len(fixture.mutationSnapshot()) != archiveMutationBaseline {
						t.Error("external Segment archive recovery sent a mutation")
					}
					if err := fixture.restoreExternal(
						environmentReplacement.EnvironmentID,
						environmentReplacement.Key,
					); err != nil {
						t.Errorf("restore externally archived Segment: %v", err)
					}
				},
				Config: environmentConfig,
				Check: resource.ComposeTestCheckFunc(
					segmentProtocolStateChecks(fixture, environmentReplacement, true),
					checkSegmentProtocolID(&recreatedID),
				),
			},
			{
				PreConfig: func() {
					referenceMutationBaseline = len(fixture.mutationSnapshot())
					if err := fixture.setReference(
						environmentReplacement.EnvironmentID,
						environmentReplacement.Key,
						true,
					); err != nil {
						t.Errorf("prepare Segment Feature Flag reference: %v", err)
					}
				},
				Config:      environmentConfig,
				Destroy:     true,
				ExpectError: regexp.MustCompile(`(?i)segment is referenced by feature flags`),
			},
			{
				PreConfig: func() {
					if len(fixture.mutationSnapshot()) != referenceMutationBaseline {
						t.Error("reference-conflicted Segment destroy sent a mutation")
					}
					if err := fixture.setReference(
						environmentReplacement.EnvironmentID,
						environmentReplacement.Key,
						false,
					); err != nil {
						t.Errorf("remove Segment Feature Flag reference: %v", err)
					}
				},
				Config:  environmentConfig,
				Destroy: true,
			},
		},
	})

	if count := fixture.managedCount(); count != 0 {
		t.Fatalf("Segment count after Protocol v6 destroy = %d, want 0", count)
	}
	if violations := fixture.violationSnapshot(); len(violations) != 0 {
		t.Fatalf("Segment protocol fixture request violations = %v", violations)
	}
	assertSegmentProtocolMutationOrder(t, fixture.mutationSnapshot())
	assertSegmentProtocolRecorder(t, fixture)
}

func segmentProtocolConfig(
	fixture *segmentProtocolFixture,
	definition segmentProtocolDefinition,
	includeExactData bool,
	extra string,
) string {
	exactData := ""
	if includeExactData {
		exactData = `
data "featbit_segment" "exact" {
  environment_id = featbit_segment.test.environment_id
  id             = featbit_segment.test.id
}
`
	}
	return fmt.Sprintf(`
provider "featbit" {
  api_url              = %q
  access_token         = %q
  http_timeout_seconds = 5
  max_concurrency      = 4
  max_retries          = 1
}

resource "featbit_segment" "test" {
  environment_id = %q
  name           = %q
  key            = %q
  description    = %q
  scopes         = %s
  included_users = %s
  excluded_users = %s
  rules          = %s
  tags           = %s
}
%s
data "featbit_segment" "shared" {
  environment_id = %q
  id             = %q
}
%s
`, fixture.apiOrigin(), syntheticProviderAccessToken,
		definition.EnvironmentID, definition.Name, definition.Key,
		definition.Description, segmentProtocolStringList(definition.Scopes),
		segmentProtocolStringList(definition.Included),
		segmentProtocolStringList(definition.Excluded),
		segmentProtocolRules(definition.Rules),
		segmentProtocolStringList(definition.Tags), exactData,
		providerEnvironmentA, fixture.sharedID(), extra)
}

func segmentProtocolSharedCollisionBlock(fixture *segmentProtocolFixture) string {
	return fmt.Sprintf(`
resource "featbit_segment" "shared_collision" {
  environment_id = %q
  name           = "Synthetic shared collision"
  key            = %q
  scopes         = [%q]
}
`, providerEnvironmentA, fixture.sharedKey(), providerSegmentEnvironmentScope)
}

func segmentProtocolStringList(values []string) string {
	encoded := make([]string, 0, len(values))
	for _, value := range values {
		encoded = append(encoded, strconv.Quote(value))
	}
	return "[" + strings.Join(encoded, ", ") + "]"
}

func segmentProtocolRules(rules []segmentProtocolRuleDefinition) string {
	encodedRules := make([]string, 0, len(rules))
	for _, rule := range rules {
		conditions := make([]string, 0, len(rule.Conditions))
		for _, condition := range rule.Conditions {
			conditions = append(conditions, fmt.Sprintf(`{
        property = %q
        operator = %q
        value    = %q
      }`, condition.Property, condition.Operator, condition.Value))
		}
		encodedRules = append(encodedRules, fmt.Sprintf(`{
    name       = %q
    conditions = [%s]
  }`, rule.Name, strings.Join(conditions, ",")))
	}
	return "[" + strings.Join(encodedRules, ",") + "]"
}

func segmentProtocolStateChecks(
	fixture *segmentProtocolFixture,
	definition segmentProtocolDefinition,
	includeExactData bool,
) resource.TestCheckFunc {
	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(segmentProtocolResourceName, "environment_id", definition.EnvironmentID),
		resource.TestCheckResourceAttr(segmentProtocolResourceName, "name", definition.Name),
		resource.TestCheckResourceAttr(segmentProtocolResourceName, "key", definition.Key),
		resource.TestCheckResourceAttr(segmentProtocolResourceName, "description", definition.Description),
		resource.TestCheckResourceAttr(segmentProtocolResourceName, "type", string(client.SegmentTypeEnvironmentSpecific)),
		segmentProtocolSetChecks(segmentProtocolResourceName, "scopes", definition.Scopes),
		segmentProtocolSetChecks(segmentProtocolResourceName, "included_users", definition.Included),
		segmentProtocolSetChecks(segmentProtocolResourceName, "excluded_users", definition.Excluded),
		segmentProtocolSetChecks(segmentProtocolResourceName, "tags", definition.Tags),
		segmentProtocolRuleChecks(segmentProtocolResourceName, definition.Rules),
		resource.TestCheckResourceAttr(segmentProtocolSharedDataName, "id", fixture.sharedID()),
		resource.TestCheckResourceAttr(segmentProtocolSharedDataName, "environment_id", providerEnvironmentA),
		resource.TestCheckResourceAttr(segmentProtocolSharedDataName, "key", fixture.sharedKey()),
		resource.TestCheckResourceAttr(segmentProtocolSharedDataName, "type", string(client.SegmentTypeShared)),
		resource.TestCheckResourceAttr(segmentProtocolSharedDataName, "scopes.#", "2"),
		resource.TestCheckTypeSetElemAttr(segmentProtocolSharedDataName, "scopes.*", providerSegmentOrganizationScope),
		resource.TestCheckTypeSetElemAttr(segmentProtocolSharedDataName, "scopes.*", providerSegmentProjectScope),
		resource.TestCheckResourceAttr(segmentProtocolSharedDataName, "included_users.#", "2"),
		resource.TestCheckResourceAttr(segmentProtocolSharedDataName, "excluded_users.#", "2"),
		resource.TestCheckResourceAttr(segmentProtocolSharedDataName, "tags.#", "2"),
		resource.TestCheckResourceAttr(segmentProtocolSharedDataName, "rules.#", "1"),
		resource.TestCheckResourceAttr(segmentProtocolSharedDataName, "rules.0.name", "Shared Rule"),
	}
	if includeExactData {
		checks = append(checks,
			resource.TestCheckResourceAttrPair(
				segmentProtocolResourceName,
				"id",
				segmentProtocolExactDataName,
				"id",
			),
			resource.TestCheckResourceAttr(segmentProtocolExactDataName, "environment_id", definition.EnvironmentID),
			resource.TestCheckResourceAttr(segmentProtocolExactDataName, "name", definition.Name),
			resource.TestCheckResourceAttr(segmentProtocolExactDataName, "key", definition.Key),
			resource.TestCheckResourceAttr(segmentProtocolExactDataName, "description", definition.Description),
			resource.TestCheckResourceAttr(segmentProtocolExactDataName, "type", string(client.SegmentTypeEnvironmentSpecific)),
			segmentProtocolSetChecks(segmentProtocolExactDataName, "scopes", definition.Scopes),
			segmentProtocolSetChecks(segmentProtocolExactDataName, "included_users", definition.Included),
			segmentProtocolSetChecks(segmentProtocolExactDataName, "excluded_users", definition.Excluded),
			segmentProtocolSetChecks(segmentProtocolExactDataName, "tags", definition.Tags),
			segmentProtocolRuleChecks(segmentProtocolExactDataName, definition.Rules),
		)
	}
	return resource.ComposeTestCheckFunc(checks...)
}

func segmentProtocolSetChecks(
	resourceName string,
	attribute string,
	values []string,
) resource.TestCheckFunc {
	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(resourceName, attribute+".#", strconv.Itoa(len(values))),
	}
	for _, value := range values {
		checks = append(checks, resource.TestCheckTypeSetElemAttr(
			resourceName,
			attribute+".*",
			value,
		))
	}
	return resource.ComposeTestCheckFunc(checks...)
}

func segmentProtocolRuleChecks(
	resourceName string,
	rules []segmentProtocolRuleDefinition,
) resource.TestCheckFunc {
	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(resourceName, "rules.#", strconv.Itoa(len(rules))),
	}
	for ruleIndex, rule := range rules {
		checks = append(checks,
			resource.TestCheckResourceAttr(
				resourceName,
				fmt.Sprintf("rules.%d.name", ruleIndex),
				rule.Name,
			),
			resource.TestCheckResourceAttr(
				resourceName,
				fmt.Sprintf("rules.%d.conditions.#", ruleIndex),
				strconv.Itoa(len(rule.Conditions)),
			),
			segmentProtocolUUIDCheck(resourceName, fmt.Sprintf("rules.%d.id", ruleIndex)),
		)
		for conditionIndex, condition := range rule.Conditions {
			prefix := fmt.Sprintf("rules.%d.conditions.%d", ruleIndex, conditionIndex)
			checks = append(checks,
				segmentProtocolUUIDCheck(resourceName, prefix+".id"),
				resource.TestCheckResourceAttr(resourceName, prefix+".property", condition.Property),
				resource.TestCheckResourceAttr(resourceName, prefix+".operator", condition.Operator),
				resource.TestCheckResourceAttr(resourceName, prefix+".value", condition.Value),
			)
		}
	}
	return resource.ComposeTestCheckFunc(checks...)
}

func segmentProtocolUUIDCheck(resourceName string, attribute string) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(resourceName, attribute, func(value string) error {
		if !validUUID(value) {
			return fmt.Errorf("Segment targeting state identity is not a valid UUID")
		}
		return nil
	})
}

func captureSegmentProtocolID(
	destination *string,
	mustDifferFrom *string,
) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(
		segmentProtocolResourceName,
		"id",
		func(value string) error {
			if !validUUID(value) {
				return fmt.Errorf("Segment state ID is not a valid UUID")
			}
			if mustDifferFrom != nil && *mustDifferFrom != "" && value == *mustDifferFrom {
				return fmt.Errorf("Segment replacement or recreation retained the prior UUID")
			}
			*destination = value
			return nil
		},
	)
}

func checkSegmentProtocolID(want *string) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(
		segmentProtocolResourceName,
		"id",
		func(value string) error {
			if want == nil || *want == "" || value != *want {
				return fmt.Errorf("in-place Segment lifecycle changed its UUID")
			}
			return nil
		},
	)
}

func checkSegmentProtocolFixture(
	fixture *segmentProtocolFixture,
	definition segmentProtocolDefinition,
) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(
		segmentProtocolResourceName,
		"id",
		func(value string) error {
			object, found := fixture.currentObject(definition.EnvironmentID, definition.Key)
			if !found || !client.EqualUUID(object.ID, value) {
				return fmt.Errorf("Segment state identity is absent from the exact fixture scope")
			}
			if !segmentProtocolObjectMatchesDefinition(object, definition) {
				return fmt.Errorf("Segment fixture definition did not converge")
			}
			if fixture.managedCount() != 1 {
				return fmt.Errorf("Segment fixture count did not converge to one")
			}
			return nil
		},
	)
}

func segmentProtocolObjectMatchesDefinition(
	object *segmentProtocolObject,
	definition segmentProtocolDefinition,
) bool {
	if object == nil || object.Archived || !object.Mutable ||
		object.Type != client.SegmentTypeEnvironmentSpecific ||
		!client.EqualUUID(object.EnvironmentID, definition.EnvironmentID) ||
		object.Name != definition.Name || object.Key != definition.Key ||
		object.Description != definition.Description ||
		!slices.Equal(canonicalStringSet(object.Scopes), canonicalStringSet(definition.Scopes)) ||
		!slices.Equal(canonicalStringSet(object.Included), canonicalStringSet(definition.Included)) ||
		!slices.Equal(canonicalStringSet(object.Excluded), canonicalStringSet(definition.Excluded)) ||
		!slices.Equal(canonicalStringSet(object.Tags), canonicalStringSet(definition.Tags)) ||
		len(object.Rules) != len(definition.Rules) {
		return false
	}
	for ruleIndex, rule := range definition.Rules {
		actual := object.Rules[ruleIndex]
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

func segmentProtocolImportID(
	segmentID *string,
	environmentID string,
) resource.ImportStateIdFunc {
	return func(*testingterraform.State) (string, error) {
		if *segmentID == "" {
			return "", fmt.Errorf("Segment import identity was not captured")
		}
		return environmentID + "/" + *segmentID, nil
	}
}

func cloneSegmentProtocolDefinition(
	definition segmentProtocolDefinition,
) segmentProtocolDefinition {
	definition.Scopes = append([]string(nil), definition.Scopes...)
	definition.Included = append([]string(nil), definition.Included...)
	definition.Excluded = append([]string(nil), definition.Excluded...)
	definition.Tags = append([]string(nil), definition.Tags...)
	clonedRules := make([]segmentProtocolRuleDefinition, len(definition.Rules))
	for index, rule := range definition.Rules {
		clonedRules[index] = segmentProtocolRuleDefinition{
			Name:       rule.Name,
			Conditions: append([]segmentProtocolConditionDefinition(nil), rule.Conditions...),
		}
	}
	definition.Rules = clonedRules
	return definition
}

func assertSegmentProtocolMutationOrder(t *testing.T, got []string) {
	t.Helper()
	want := []string{
		segmentProtocolMutationCreate,
		segmentProtocolMutationTargeting,
		segmentProtocolMutationTags,
		segmentProtocolMutationName,
		segmentProtocolMutationDescription,
		segmentProtocolMutationTargeting,
		segmentProtocolMutationTags,
		segmentProtocolMutationTargeting,
		segmentProtocolMutationArchive,
		segmentProtocolMutationDelete,
		segmentProtocolMutationCreate,
		segmentProtocolMutationTargeting,
		segmentProtocolMutationTags,
		segmentProtocolMutationArchive,
		segmentProtocolMutationDelete,
		segmentProtocolMutationCreate,
		segmentProtocolMutationTargeting,
		segmentProtocolMutationTags,
		segmentProtocolMutationArchive,
		segmentProtocolMutationDelete,
		segmentProtocolMutationCreate,
		segmentProtocolMutationTargeting,
		segmentProtocolMutationTags,
		segmentProtocolMutationCreate,
		segmentProtocolMutationTargeting,
		segmentProtocolMutationTags,
		segmentProtocolMutationArchive,
		segmentProtocolMutationDelete,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Segment Protocol mutation order = %v, want %v", got, want)
	}
}

func assertSegmentProtocolRecorder(t *testing.T, fixture *segmentProtocolFixture) {
	t.Helper()
	activeProof, archivedProof := fixture.teardownProof()
	if !activeProof || !archivedProof {
		t.Fatalf(
			"Segment Protocol teardown proof active/archived = %t/%t, want true/true",
			activeProof,
			archivedProof,
		)
	}
	requests := fixture.requestSnapshot()
	if len(requests) == 0 {
		t.Fatal("Segment Protocol recorder observed no requests")
	}
	for _, request := range requests {
		if request.Method == http.MethodPatch ||
			strings.Contains(request.Path, "/restore") ||
			strings.Contains(request.Path, "/by-ids") ||
			strings.Contains(request.Path, "/all-tags") {
			t.Fatalf("Protocol recorder observed a forbidden Segment operation")
		}
	}
}
