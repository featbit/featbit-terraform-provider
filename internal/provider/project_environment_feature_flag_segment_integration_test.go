// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	testingterraform "github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

const (
	crossSegmentResourceName    = "featbit_segment.cross_child"
	crossSegmentDataName        = "data.featbit_segment.cross_child"
	crossSegmentKey             = "cross-owned-segment"
	crossSegmentReferenceMarker = "test-only-cross-segment-reference-marker"
)

type crossSegmentProtocolFixture struct {
	base     *crossResourceProtocolFixture
	segments *segmentProtocolFixture
	server   *httptest.Server

	mu         sync.Mutex
	requests   []crossResourceFixtureRequest
	violations int
}

type crossSegmentReferenceSnapshot struct {
	segment *segmentProtocolObject
	flagID  string
	flagUI  featureFlagFixtureUI
}

func newCrossSegmentProtocolFixture(t *testing.T) *crossSegmentProtocolFixture {
	t.Helper()
	fixture := &crossSegmentProtocolFixture{
		base:     newCrossResourceProtocolFixture(t),
		segments: newSegmentProtocolFixture(t),
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.handle))
	return fixture
}

func (f *crossSegmentProtocolFixture) handle(
	response http.ResponseWriter,
	request *http.Request,
) {
	path := request.URL.EscapedPath()
	f.mu.Lock()
	f.requests = append(f.requests, crossResourceFixtureRequest{
		Method: request.Method,
		Path:   path,
	})
	f.mu.Unlock()

	if environmentID, segmentPath := crossResourceSegmentEnvironment(path); segmentPath {
		if request.Method == http.MethodPost &&
			!crossResourceProjectContainsEnvironment(f.base.project, environmentID) {
			f.recordViolation()
			writeProjectFixtureEnvelope(response, http.StatusConflict, nil)
			return
		}
		f.segments.ServeHTTP(response, request)
		return
	}

	if request.Method == http.MethodDelete {
		if environmentID, exactEnvironment := crossResourceExactEnvironment(path); exactEnvironment {
			if crossResourceSegmentCount(f.segments, environmentID) != 0 {
				f.recordViolation()
				writeProjectFixtureEnvelope(response, http.StatusConflict, nil)
				return
			}
		} else if _, exactProject := crossResourceExactProject(path); exactProject {
			if f.segments.managedCount() != 0 {
				f.recordViolation()
				writeProjectFixtureEnvelope(response, http.StatusConflict, nil)
				return
			}
		}
	}

	f.base.handle(response, request)
}

func (f *crossSegmentProtocolFixture) recordViolation() {
	f.mu.Lock()
	f.violations++
	f.mu.Unlock()
}

func (f *crossSegmentProtocolFixture) apiOrigin() string {
	return f.server.URL
}

func (f *crossSegmentProtocolFixture) close() {
	f.server.Close()
	f.segments.close()
	f.base.close()
}

func (f *crossSegmentProtocolFixture) requestSnapshot() []crossResourceFixtureRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]crossResourceFixtureRequest(nil), f.requests...)
}

func (f *crossSegmentProtocolFixture) violationCount() int {
	f.mu.Lock()
	violations := f.violations
	f.mu.Unlock()
	return violations + f.base.violationCount() + len(f.segments.violationSnapshot())
}

func TestProjectEnvironmentSegmentFeatureFlagProtocolOwnershipStateSafetyAndRedaction(
	t *testing.T,
) {
	fixture := newCrossSegmentProtocolFixture(t)
	fixture.base.flags.setReverseVariations(true)
	fixture.base.flags.setReverseCollections(true)
	fixture.segments.setReverseSets(true)
	fixture.segments.setReverseCollections(true)
	t.Cleanup(func() {
		if fixture.base.flags.objectCount() != 0 || fixture.segments.managedCount() != 0 ||
			fixture.base.project.projectCount() != 0 ||
			fixture.base.project.environmentCount() != 0 {
			t.Error("Segment cross-resource Protocol fixture retained a test object")
		}
		fixture.close()
	})

	const (
		initialEnvironmentKey     = "segment-cross-stage-a"
		replacementEnvironmentKey = "segment-cross-stage-b"
	)
	var projectID string
	var initialEnvironmentID string
	var initialSegmentID string
	var initialFeatureFlagID string
	var replacementEnvironmentID string
	var replacementSegmentID string
	var replacementFeatureFlagID string
	var referenceSnapshot crossSegmentReferenceSnapshot
	var referenceMutationBaseline int

	initialConfig := crossSegmentIntegrationConfig(
		fixture.apiOrigin(),
		initialEnvironmentKey,
		true,
	)
	replacementConfig := crossSegmentIntegrationConfig(
		fixture.apiOrigin(),
		replacementEnvironmentKey,
		true,
	)
	withoutSegmentConfig := crossSegmentIntegrationConfig(
		fixture.apiOrigin(),
		replacementEnvironmentKey,
		false,
	)
	projectOnlyConfig := crossResourceProjectOnlyConfig(fixture.apiOrigin())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"featbit": providerserver.NewProtocol6WithError(New("protocol-test")()),
		},
		Steps: []resource.TestStep{
			{
				Config: initialConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectUnknownValue(
							crossResourceEnvironmentName,
							tfjsonpath.New("id"),
						),
						plancheck.ExpectUnknownValue(
							crossSegmentResourceName,
							tfjsonpath.New("environment_id"),
						),
						plancheck.ExpectUnknownValue(
							crossSegmentResourceName,
							tfjsonpath.New("id"),
						),
						plancheck.ExpectUnknownValue(
							crossResourceFeatureFlagName,
							tfjsonpath.New("environment_id"),
						),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					crossSegmentCaptureIDs(
						&projectID,
						&initialEnvironmentID,
						&initialSegmentID,
						&initialFeatureFlagID,
						nil,
						nil,
						nil,
					),
					crossSegmentStateChecks(&initialEnvironmentID, initialEnvironmentKey, true),
					crossSegmentTargetingCorrelationCheck(&initialEnvironmentID),
					crossSegmentStateSafetyCheck,
					crossSegmentMutationCountCheck(fixture, 6),
				),
			},
			{
				Config:   initialConfig,
				PlanOnly: true,
			},
			{
				Config: replacementConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(
							crossResourceProjectName,
							plancheck.ResourceActionNoop,
						),
						plancheck.ExpectResourceAction(
							crossResourceEnvironmentName,
							plancheck.ResourceActionReplace,
						),
						plancheck.ExpectResourceAction(
							crossSegmentResourceName,
							plancheck.ResourceActionReplace,
						),
						plancheck.ExpectResourceAction(
							crossResourceFeatureFlagName,
							plancheck.ResourceActionReplace,
						),
						plancheck.ExpectUnknownValue(
							crossSegmentResourceName,
							tfjsonpath.New("environment_id"),
						),
						plancheck.ExpectUnknownValue(
							crossSegmentResourceName,
							tfjsonpath.New("id"),
						),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					crossSegmentCaptureIDs(
						&projectID,
						&replacementEnvironmentID,
						&replacementSegmentID,
						&replacementFeatureFlagID,
						&initialEnvironmentID,
						&initialSegmentID,
						&initialFeatureFlagID,
					),
					crossSegmentStateChecks(
						&replacementEnvironmentID,
						replacementEnvironmentKey,
						true,
					),
					crossSegmentTargetingCorrelationCheck(&replacementEnvironmentID),
					crossSegmentStateSafetyCheck,
					crossSegmentMutationCountCheck(fixture, 16),
				),
			},
			{
				Config:   replacementConfig,
				PlanOnly: true,
			},
			{
				PreConfig: func() {
					var err error
					referenceSnapshot, err = crossSegmentEstablishReference(
						fixture,
						replacementEnvironmentID,
						crossSegmentKey,
						crossResourceFeatureFlagKey,
					)
					if err != nil {
						t.Error("could not prepare an exact Segment reference")
					}
					referenceMutationBaseline = crossSegmentMutationCount(fixture)
				},
				Config: withoutSegmentConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(
							crossResourceProjectName,
							plancheck.ResourceActionNoop,
						),
						plancheck.ExpectResourceAction(
							crossResourceEnvironmentName,
							plancheck.ResourceActionNoop,
						),
						plancheck.ExpectResourceAction(
							crossResourceFeatureFlagName,
							plancheck.ResourceActionNoop,
						),
						plancheck.ExpectResourceAction(
							crossSegmentResourceName,
							plancheck.ResourceActionDestroy,
						),
					},
				},
				ExpectError: regexp.MustCompile(`(?i)segment is referenced by feature flags`),
			},
			{
				PreConfig: func() {
					if crossSegmentMutationCount(fixture) != referenceMutationBaseline ||
						!crossSegmentReferenceUnchanged(fixture, referenceSnapshot) {
						t.Error("reference-conflicted destroy changed the Segment or Feature Flag")
					}
					if err := crossSegmentRemoveReference(fixture, referenceSnapshot); err != nil {
						t.Error("could not remove the exact Segment reference externally")
					}
				},
				Config: withoutSegmentConfig,
				Check: resource.ComposeTestCheckFunc(
					crossSegmentStateChecks(
						&replacementEnvironmentID,
						replacementEnvironmentKey,
						false,
					),
					crossSegmentStableIDCheck(
						crossResourceFeatureFlagName,
						&replacementFeatureFlagID,
					),
					crossSegmentReferenceRemovalCheck(fixture, &referenceSnapshot),
					crossSegmentStateSafetyCheck,
					crossSegmentMutationCountCheck(fixture, 18),
				),
			},
			{
				Config:   withoutSegmentConfig,
				PlanOnly: true,
			},
			{
				Config: projectOnlyConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(
							crossResourceProjectName,
							plancheck.ResourceActionNoop,
						),
						plancheck.ExpectResourceAction(
							crossResourceFeatureFlagName,
							plancheck.ResourceActionDestroy,
						),
						plancheck.ExpectResourceAction(
							crossResourceEnvironmentName,
							plancheck.ResourceActionDestroy,
						),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					crossSegmentStableIDCheck(crossResourceProjectName, &projectID),
					crossSegmentStateSafetyCheck,
					crossSegmentMutationCountCheck(fixture, 21),
				),
			},
			{
				Config: projectOnlyConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(crossResourceProjectName, "environments.#", "2"),
					crossSegmentStateSafetyCheck,
					crossSegmentMutationCountCheck(fixture, 21),
				),
			},
			{
				Config:   projectOnlyConfig,
				PlanOnly: true,
			},
		},
	})

	if fixture.base.flags.objectCount() != 0 || fixture.segments.managedCount() != 0 ||
		fixture.base.project.projectCount() != 0 ||
		fixture.base.project.environmentCount() != 0 {
		t.Fatal("Segment cross-resource Protocol destroy did not reach exact zero")
	}
	if fixture.violationCount() != 0 {
		t.Fatal("Segment cross-resource Protocol fixture observed an ownership violation")
	}
	assertCrossSegmentMutationOwnership(
		t,
		fixture.requestSnapshot(),
		projectID,
		initialEnvironmentID,
		initialSegmentID,
		replacementEnvironmentID,
		replacementSegmentID,
	)
}

func TestSegmentCrossViewAmbiguityPreservesStateAndCrossResourceImportDiagnosticsRedact(
	t *testing.T,
) {
	t.Parallel()

	remote := providerRemoteSegment(
		client.SegmentTypeEnvironmentSpecific,
		[]string{providerSegmentEnvironmentScope},
	)
	canonical, err := canonicalizeRemoteSegment(remote)
	if err != nil {
		t.Fatal("could not prepare canonical Segment ambiguity state")
	}
	segmentSchema := segmentResourceSchema()
	priorState := tfsdk.State{Schema: segmentSchema}
	if diagnostics := priorState.Set(
		context.Background(),
		flattenCanonicalSegment(canonical),
	); diagnostics.HasError() {
		t.Fatal("could not prepare Segment ambiguity state")
	}

	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			switch {
			case request.URL.RawQuery == "":
				writeProjectResourceEnvelope(t, response, http.StatusNotFound, "null")
			case request.URL.Query().Get("IsArchived") == "false":
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					segmentDeleteJSON(t, map[string]any{
						"totalCount": 1,
						"items": []any{map[string]any{
							"id": remote.ID, "envId": remote.EnvironmentID,
							"key": remote.Key, "type": string(remote.Type),
							"scopes": remote.Scopes, "isArchived": false,
							"isEnvironmentSpecific": true,
						}},
					}),
				)
			case request.URL.Query().Get("IsArchived") == "true":
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					segmentDeleteJSON(t, map[string]any{
						"totalCount": 1,
						"items": []any{map[string]any{
							"id": remote.ID, "envId": remote.EnvironmentID,
							"key": remote.Key, "type": string(remote.Type),
							"scopes": remote.Scopes, "isArchived": true,
							"isEnvironmentSpecific": true,
						}},
					}),
				)
			default:
				t.Fatal("Segment ambiguity test received an unexpected request")
			}
		},
	))
	defer closeServer()

	readResponse := frameworkresource.ReadResponse{State: priorState}
	(&segmentResource{client: apiClient}).Read(
		context.Background(),
		frameworkresource.ReadRequest{State: priorState},
		&readResponse,
	)
	if !readResponse.Diagnostics.HasError() || !readResponse.State.Raw.Equal(priorState.Raw) {
		t.Fatal("active/archived Segment duplicate did not preserve managed state")
	}

	const rejected = "api-segment-cross-import-marker/segment-user-cross-marker/" +
		"segment-flag-cross-marker/segment-key-cross-marker/segment-condition-cross-marker/" +
		"segment-tag-cross-marker/segment-scope-cross-marker/segment-tenant-cross-marker/" +
		"segment-server-cross-marker/segment-path-cross-marker/segment-body-cross-marker"
	projectResponse := frameworkresource.ImportStateResponse{
		State: tfsdk.State{Schema: projectResourceTestSchema(t)},
	}
	(&projectResource{}).ImportState(
		context.Background(),
		frameworkresource.ImportStateRequest{ID: rejected},
		&projectResponse,
	)
	environmentResponse := frameworkresource.ImportStateResponse{
		State: tfsdk.State{Schema: environmentResourceTestSchema(t)},
	}
	(&environmentResource{}).ImportState(
		context.Background(),
		frameworkresource.ImportStateRequest{ID: rejected},
		&environmentResponse,
	)
	featureFlagResponse := frameworkresource.ImportStateResponse{
		State: emptyFeatureFlagResourceState(t, featureFlagResourceSchema()),
	}
	(&featureFlagResource{}).ImportState(
		context.Background(),
		frameworkresource.ImportStateRequest{ID: rejected},
		&featureFlagResponse,
	)
	segmentResponse := frameworkresource.ImportStateResponse{
		State: emptySegmentResourceState(t, segmentSchema),
	}
	(&segmentResource{}).ImportState(
		context.Background(),
		frameworkresource.ImportStateRequest{ID: rejected},
		&segmentResponse,
	)
	if !projectResponse.Diagnostics.HasError() || !environmentResponse.Diagnostics.HasError() ||
		!featureFlagResponse.Diagnostics.HasError() ||
		!segmentResponse.Diagnostics.HasError() {
		t.Fatal("a cross-resource Import accepted an unsafe identifier")
	}

	diagnosticOutput := fmt.Sprint(
		readResponse.Diagnostics,
		projectResponse.Diagnostics,
		environmentResponse.Diagnostics,
		featureFlagResponse.Diagnostics,
		segmentResponse.Diagnostics,
	)
	for _, unsafe := range []string{
		syntheticProviderAccessToken,
		providerEnvironmentA,
		providerSegmentID,
		remote.Key,
		"api-segment-cross-import-marker",
		"segment-user-cross-marker",
		"segment-flag-cross-marker",
		"segment-key-cross-marker",
		"segment-condition-cross-marker",
		"segment-tag-cross-marker",
		"segment-scope-cross-marker",
		"segment-tenant-cross-marker",
		"segment-server-cross-marker",
		"segment-path-cross-marker",
		"segment-body-cross-marker",
		"/api/v1/envs/",
	} {
		if strings.Contains(diagnosticOutput, unsafe) {
			t.Fatal("Segment cross-resource diagnostics exposed a runtime value")
		}
	}
}

func crossSegmentIntegrationConfig(
	apiOrigin string,
	environmentKey string,
	includeSegment bool,
) string {
	segmentBlock := ""
	segmentDependency := ""
	segmentData := ""
	if includeSegment {
		segmentBlock = fmt.Sprintf(`
resource "featbit_segment" "cross_child" {
  environment_id = featbit_environment.cross_child.id
  name           = "Cross-resource Segment"
  key            = %q
  description    = null
  scopes = [
    "organization/cross-resource:project/cross-resource:env/${featbit_environment.cross_child.id}"
  ]
  included_users = ["cross-user-z", "cross-user-a"]
  excluded_users = ["cross-excluded-z", "cross-excluded-a"]
  rules = [
    {
      name = "Cross Rule"
      conditions = [
        {
          property = "cross-property"
          operator = "IsOneOf"
          value    = "[\"zeta\",\"alpha\"]"
        }
      ]
    }
  ]
  tags = ["cross-tag-z", "cross-tag-a"]
}
`, crossSegmentKey)
		segmentDependency = "  depends_on     = [featbit_segment.cross_child]\n"
		segmentData = `
data "featbit_segment" "cross_child" {
  environment_id = featbit_segment.cross_child.environment_id
  id             = featbit_segment.cross_child.id
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

resource "featbit_project" "cross_parent" {
  name = "Cross-resource Parent"
  key  = "cross-resource-parent"
}

resource "featbit_environment" "cross_child" {
  project_id = featbit_project.cross_parent.id
  name       = "Cross-resource Environment"
  key        = %q
}
%s
resource "featbit_feature_flag" "cross_child" {
  environment_id = featbit_environment.cross_child.id
  name           = "Cross-resource Feature Flag"
  key            = %q
  variation_type = "number"
%s  variations = [
    {
      name  = "Hundred"
      value = "1e2"
    },
    {
      name  = "Two"
      value = "2.00"
    }
  ]
}

data "featbit_project" "cross_parent" {
  id         = featbit_project.cross_parent.id
  depends_on = [featbit_feature_flag.cross_child]
}

data "featbit_environment" "cross_child" {
  project_id = featbit_environment.cross_child.project_id
  id         = featbit_environment.cross_child.id
}
%s
data "featbit_feature_flag" "cross_child" {
  environment_id = featbit_feature_flag.cross_child.environment_id
  key            = featbit_feature_flag.cross_child.key
}
`, apiOrigin, syntheticProviderAccessToken, environmentKey, segmentBlock,
		crossResourceFeatureFlagKey, segmentDependency, segmentData)
}

func crossSegmentStateChecks(
	environmentID *string,
	environmentKey string,
	includeSegment bool,
) resource.TestCheckFunc {
	checks := []resource.TestCheckFunc{
		crossResourceStateChecks(environmentKey),
	}
	if includeSegment {
		checks = append(checks,
			resource.TestCheckResourceAttr(crossSegmentResourceName, "name", "Cross-resource Segment"),
			resource.TestCheckResourceAttr(crossSegmentResourceName, "key", crossSegmentKey),
			resource.TestCheckResourceAttr(crossSegmentResourceName, "description", ""),
			resource.TestCheckResourceAttr(crossSegmentResourceName, "type", string(client.SegmentTypeEnvironmentSpecific)),
			resource.TestCheckResourceAttr(crossSegmentResourceName, "included_users.#", "2"),
			resource.TestCheckResourceAttr(crossSegmentResourceName, "excluded_users.#", "2"),
			resource.TestCheckResourceAttr(crossSegmentResourceName, "tags.#", "2"),
			resource.TestCheckResourceAttr(crossSegmentResourceName, "rules.#", "1"),
			resource.TestCheckResourceAttr(crossSegmentResourceName, "rules.0.conditions.0.value", `["zeta","alpha"]`),
			resource.TestCheckResourceAttr(crossSegmentDataName, "rules.0.conditions.0.value", `["alpha","zeta"]`),
			resource.TestCheckResourceAttrPair(
				crossResourceEnvironmentName,
				"id",
				crossSegmentResourceName,
				"environment_id",
			),
			resource.TestCheckResourceAttrPair(
				crossSegmentResourceName,
				"id",
				crossSegmentDataName,
				"id",
			),
			crossSegmentScopeCheck(environmentID),
		)
	}
	return resource.ComposeTestCheckFunc(checks...)
}

func crossSegmentScopeCheck(environmentID *string) resource.TestCheckFunc {
	return func(state *testingterraform.State) error {
		if environmentID == nil || *environmentID == "" {
			return fmt.Errorf("cross-resource Environment identity is unavailable")
		}
		want := "organization/cross-resource:project/cross-resource:env/" + *environmentID
		for _, address := range []string{crossSegmentResourceName, crossSegmentDataName} {
			resourceState, found := state.RootModule().Resources[address]
			if !found || resourceState.Primary == nil ||
				resourceState.Primary.Attributes["scopes.#"] != "1" {
				return fmt.Errorf("cross-resource Segment scope state is missing")
			}
			foundScope := false
			for key, value := range resourceState.Primary.Attributes {
				if strings.HasPrefix(key, "scopes.") && key != "scopes.#" && value == want {
					foundScope = true
				}
			}
			if !foundScope {
				return fmt.Errorf("cross-resource Segment scope lost parent correlation")
			}
		}
		return nil
	}
}

func crossSegmentCaptureIDs(
	projectID *string,
	environmentID *string,
	segmentID *string,
	featureFlagID *string,
	environmentMustDifferFrom *string,
	segmentMustDifferFrom *string,
	featureFlagMustDifferFrom *string,
) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttrWith(crossResourceProjectName, "id", func(value string) error {
			if !validUUID(value) || *projectID != "" && value != *projectID {
				return fmt.Errorf("Segment lifecycle changed the parent Project identity")
			}
			*projectID = value
			return nil
		}),
		resource.TestCheckResourceAttrWith(crossResourceEnvironmentName, "id", func(value string) error {
			if !validUUID(value) || environmentMustDifferFrom != nil &&
				*environmentMustDifferFrom != "" && value == *environmentMustDifferFrom {
				return fmt.Errorf("cross-resource Environment identity did not replace")
			}
			*environmentID = value
			return nil
		}),
		resource.TestCheckResourceAttrWith(crossSegmentResourceName, "id", func(value string) error {
			if !validUUID(value) || segmentMustDifferFrom != nil &&
				*segmentMustDifferFrom != "" && value == *segmentMustDifferFrom {
				return fmt.Errorf("cross-resource Segment identity did not replace")
			}
			*segmentID = value
			return nil
		}),
		resource.TestCheckResourceAttrWith(crossResourceFeatureFlagName, "id", func(value string) error {
			if !validUUID(value) || featureFlagMustDifferFrom != nil &&
				*featureFlagMustDifferFrom != "" && value == *featureFlagMustDifferFrom {
				return fmt.Errorf("cross-resource Feature Flag identity did not replace")
			}
			*featureFlagID = value
			return nil
		}),
	)
}

func crossSegmentTargetingCorrelationCheck(environmentID *string) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttrWith(
			crossSegmentResourceName,
			"rules.0.id",
			func(value string) error {
				if environmentID == nil || value != deterministicSegmentRuleID(
					*environmentID,
					crossSegmentKey,
					0,
				) {
					return fmt.Errorf("Segment rule lost deterministic parent correlation")
				}
				return nil
			},
		),
		resource.TestCheckResourceAttrWith(
			crossSegmentResourceName,
			"rules.0.conditions.0.id",
			func(value string) error {
				if environmentID == nil || value != deterministicSegmentConditionID(
					*environmentID,
					crossSegmentKey,
					0,
					0,
				) {
					return fmt.Errorf("Segment condition lost deterministic parent correlation")
				}
				return nil
			},
		),
	)
}

func crossSegmentStableIDCheck(resourceName string, want *string) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(resourceName, "id", func(value string) error {
		if want == nil || *want == "" || value != *want {
			return fmt.Errorf("cross-resource stable identity changed unexpectedly")
		}
		return nil
	})
}

func crossSegmentStateSafetyCheck(state *testingterraform.State) error {
	if err := crossResourceStateSafetyCheck(state); err != nil {
		return err
	}
	formatted := fmt.Sprintf("%#v", state)
	for _, unsafe := range []string{
		crossSegmentReferenceMarker,
		"segment-reference-server-only-marker",
	} {
		if strings.Contains(formatted, unsafe) {
			return fmt.Errorf("cross-resource Terraform state retained a Segment endpoint-only value")
		}
	}
	return nil
}

func crossSegmentMutationCount(fixture *crossSegmentProtocolFixture) int {
	count := 0
	for _, request := range fixture.requestSnapshot() {
		if request.Method != http.MethodGet {
			count++
		}
	}
	return count
}

func crossSegmentMutationCountCheck(
	fixture *crossSegmentProtocolFixture,
	want int,
) resource.TestCheckFunc {
	return func(*testingterraform.State) error {
		if got := crossSegmentMutationCount(fixture); got != want {
			return fmt.Errorf("Segment cross-resource ownership produced an unexpected mutation count")
		}
		return nil
	}
}

func crossSegmentEstablishReference(
	fixture *crossSegmentProtocolFixture,
	environmentID string,
	segmentKey string,
	featureFlagKey string,
) (crossSegmentReferenceSnapshot, error) {
	segment, found := fixture.segments.currentObject(environmentID, segmentKey)
	if !found {
		return crossSegmentReferenceSnapshot{}, fmt.Errorf("managed Segment is absent")
	}

	fixture.base.flags.mu.Lock()
	flag := fixture.base.flags.active[featureFlagFixtureIdentity(environmentID, featureFlagKey)]
	if flag == nil {
		fixture.base.flags.mu.Unlock()
		return crossSegmentReferenceSnapshot{}, fmt.Errorf("Feature Flag is absent")
	}
	flag.UI.TargetMarker = crossSegmentReferenceMarker
	flagID := flag.ID
	flagUI := cloneFeatureFlagFixtureUI(flag.UI)
	fixture.base.flags.mu.Unlock()

	fixture.segments.mu.Lock()
	fixture.segments.references[segment.ID] = []client.SegmentFlagReference{
		{
			EnvironmentID: environmentID,
			ID:            flagID,
			Name:          "Synthetic cross-resource reference",
			Key:           featureFlagKey,
		},
	}
	fixture.segments.mu.Unlock()

	return crossSegmentReferenceSnapshot{
		segment: segment,
		flagID:  flagID,
		flagUI:  flagUI,
	}, nil
}

func crossSegmentReferenceUnchanged(
	fixture *crossSegmentProtocolFixture,
	snapshot crossSegmentReferenceSnapshot,
) bool {
	if snapshot.segment == nil {
		return false
	}
	segment, found := fixture.segments.currentObject(
		snapshot.segment.EnvironmentID,
		snapshot.segment.Key,
	)
	if !found || !reflect.DeepEqual(segment, snapshot.segment) {
		return false
	}

	fixture.base.flags.mu.Lock()
	defer fixture.base.flags.mu.Unlock()
	flag := fixture.base.flags.active[featureFlagFixtureIdentity(
		snapshot.segment.EnvironmentID,
		crossResourceFeatureFlagKey,
	)]
	return flag != nil && flag.ID == snapshot.flagID && reflect.DeepEqual(flag.UI, snapshot.flagUI)
}

func crossSegmentRemoveReference(
	fixture *crossSegmentProtocolFixture,
	snapshot crossSegmentReferenceSnapshot,
) error {
	if snapshot.segment == nil {
		return fmt.Errorf("reference snapshot is incomplete")
	}
	fixture.base.flags.mu.Lock()
	flag := fixture.base.flags.active[featureFlagFixtureIdentity(
		snapshot.segment.EnvironmentID,
		crossResourceFeatureFlagKey,
	)]
	if flag == nil || flag.ID != snapshot.flagID {
		fixture.base.flags.mu.Unlock()
		return fmt.Errorf("Feature Flag changed during reference removal")
	}
	flag.UI.TargetMarker = ""
	fixture.base.flags.mu.Unlock()

	fixture.segments.mu.Lock()
	fixture.segments.references[snapshot.segment.ID] = []client.SegmentFlagReference{}
	fixture.segments.mu.Unlock()
	return nil
}

func crossSegmentReferenceRemovalCheck(
	fixture *crossSegmentProtocolFixture,
	snapshot *crossSegmentReferenceSnapshot,
) resource.TestCheckFunc {
	return func(*testingterraform.State) error {
		if snapshot == nil || snapshot.segment == nil {
			return fmt.Errorf("Segment reference snapshot was unavailable")
		}
		if fixture.segments.managedCount() != 0 {
			return fmt.Errorf("Segment remained after its exact reference was removed")
		}
		fixture.base.flags.mu.Lock()
		defer fixture.base.flags.mu.Unlock()
		flag := fixture.base.flags.active[featureFlagFixtureIdentity(
			snapshot.segment.EnvironmentID,
			crossResourceFeatureFlagKey,
		)]
		if flag == nil || flag.ID != snapshot.flagID || flag.UI.TargetMarker != "" {
			return fmt.Errorf("Segment deletion changed the surviving Feature Flag")
		}
		wantUI := snapshot.flagUI
		wantUI.TargetMarker = ""
		if !reflect.DeepEqual(flag.UI, wantUI) {
			return fmt.Errorf("Segment deletion rewrote Feature Flag UI-owned state")
		}
		return nil
	}
}

func assertCrossSegmentMutationOwnership(
	t *testing.T,
	requests []crossResourceFixtureRequest,
	projectID string,
	initialEnvironmentID string,
	initialSegmentID string,
	replacementEnvironmentID string,
	replacementSegmentID string,
) {
	t.Helper()
	projectPath := "/api/v1/projects/" + projectID
	initialEnvironmentPath := projectPath + "/envs/" + initialEnvironmentID
	replacementEnvironmentPath := projectPath + "/envs/" + replacementEnvironmentID
	initialSegmentPath := "/api/v1/envs/" + initialEnvironmentID + "/segments/" + initialSegmentID
	replacementSegmentPath := "/api/v1/envs/" + replacementEnvironmentID + "/segments/" + replacementSegmentID
	initialFlagPath := "/api/v1/envs/" + initialEnvironmentID + "/feature-flags/" +
		crossResourceFeatureFlagKey
	replacementFlagPath := "/api/v1/envs/" + replacementEnvironmentID + "/feature-flags/" +
		crossResourceFeatureFlagKey

	want := []crossResourceFixtureRequest{
		{Method: http.MethodPost, Path: "/api/v1/projects"},
		{Method: http.MethodPost, Path: projectPath + "/envs"},
		{Method: http.MethodPost, Path: "/api/v1/envs/" + initialEnvironmentID + "/segments"},
		{Method: http.MethodPut, Path: initialSegmentPath + "/targeting"},
		{Method: http.MethodPut, Path: initialSegmentPath + "/tags"},
		{Method: http.MethodPost, Path: "/api/v1/envs/" + initialEnvironmentID + "/feature-flags"},
		{Method: http.MethodPut, Path: initialFlagPath + "/archive"},
		{Method: http.MethodDelete, Path: initialFlagPath},
		{Method: http.MethodPut, Path: initialSegmentPath + "/archive"},
		{Method: http.MethodDelete, Path: initialSegmentPath},
		{Method: http.MethodDelete, Path: initialEnvironmentPath},
		{Method: http.MethodPost, Path: projectPath + "/envs"},
		{Method: http.MethodPost, Path: "/api/v1/envs/" + replacementEnvironmentID + "/segments"},
		{Method: http.MethodPut, Path: replacementSegmentPath + "/targeting"},
		{Method: http.MethodPut, Path: replacementSegmentPath + "/tags"},
		{Method: http.MethodPost, Path: "/api/v1/envs/" + replacementEnvironmentID + "/feature-flags"},
		{Method: http.MethodPut, Path: replacementSegmentPath + "/archive"},
		{Method: http.MethodDelete, Path: replacementSegmentPath},
		{Method: http.MethodPut, Path: replacementFlagPath + "/archive"},
		{Method: http.MethodDelete, Path: replacementFlagPath},
		{Method: http.MethodDelete, Path: replacementEnvironmentPath},
		{Method: http.MethodDelete, Path: projectPath},
	}
	mutationIndex := 0
	for _, request := range requests {
		if request.Method == http.MethodGet {
			continue
		}
		if mutationIndex >= len(want) || request != want[mutationIndex] {
			t.Fatal("Segment cross-resource mutation order or ownership was incorrect")
		}
		mutationIndex++
	}
	if mutationIndex != len(want) {
		t.Fatal("Segment cross-resource mutation recorder was incomplete")
	}
}

func crossResourceSegmentEnvironment(path string) (string, bool) {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) < 5 || segments[0] != "api" || segments[1] != "v1" ||
		segments[2] != "envs" || !validUUID(segments[3]) || segments[4] != "segments" {
		return "", false
	}
	return segments[3], true
}

func crossResourceSegmentCount(
	fixture *segmentProtocolFixture,
	environmentID string,
) int {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	count := 0
	for _, source := range []map[string]*segmentProtocolObject{fixture.active, fixture.archived} {
		for _, segment := range source {
			if client.EqualUUID(segment.EnvironmentID, environmentID) {
				count++
			}
		}
	}
	return count
}
