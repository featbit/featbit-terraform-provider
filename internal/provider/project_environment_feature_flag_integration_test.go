// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

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
	crossResourceProjectName         = "featbit_project.cross_parent"
	crossResourceEnvironmentName     = "featbit_environment.cross_child"
	crossResourceFeatureFlagName     = "featbit_feature_flag.cross_child"
	crossResourceProjectDataName     = "data.featbit_project.cross_parent"
	crossResourceEnvironmentDataName = "data.featbit_environment.cross_child"
	crossResourceFeatureFlagDataName = "data.featbit_feature_flag.cross_child"
	crossResourceFeatureFlagKey      = "cross-owned-flag"
)

type crossResourceFixtureRequest struct {
	Method string
	Path   string
}

type crossResourceProtocolFixture struct {
	project *projectProtocolFixture
	flags   *featureFlagProtocolFixture
	server  *httptest.Server

	mu         sync.Mutex
	requests   []crossResourceFixtureRequest
	violations int
}

func newCrossResourceProtocolFixture(t *testing.T) *crossResourceProtocolFixture {
	t.Helper()
	fixture := &crossResourceProtocolFixture{
		project: newProjectProtocolFixture(t),
		flags:   newFeatureFlagProtocolFixture(t),
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.handle))
	return fixture
}

func (f *crossResourceProtocolFixture) close() {
	f.server.Close()
	f.flags.close()
	f.project.close()
}

func (f *crossResourceProtocolFixture) apiOrigin() string {
	return f.server.URL
}

func (f *crossResourceProtocolFixture) handle(
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

	if environmentID, featureFlagPath := crossResourceFeatureFlagEnvironment(path); featureFlagPath {
		if request.Method == http.MethodPost &&
			!crossResourceProjectContainsEnvironment(f.project, environmentID) {
			f.recordViolation()
			writeProjectFixtureEnvelope(response, http.StatusConflict, nil)
			return
		}
		f.flags.ServeHTTP(response, request)
		return
	}

	if request.Method == http.MethodDelete {
		if environmentID, exactEnvironment := crossResourceExactEnvironment(path); exactEnvironment {
			if crossResourceFeatureFlagCount(f.flags, environmentID) != 0 {
				f.recordViolation()
				writeProjectFixtureEnvelope(response, http.StatusConflict, nil)
				return
			}
		} else if _, exactProject := crossResourceExactProject(path); exactProject {
			if f.flags.objectCount() != 0 || crossResourceProjectHasManagedEnvironment(f.project) {
				f.recordViolation()
				writeProjectFixtureEnvelope(response, http.StatusConflict, nil)
				return
			}
		}
	}

	f.project.handle(response, request)
}

func (f *crossResourceProtocolFixture) recordViolation() {
	f.mu.Lock()
	f.violations++
	f.mu.Unlock()
}

func (f *crossResourceProtocolFixture) requestSnapshot() []crossResourceFixtureRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]crossResourceFixtureRequest(nil), f.requests...)
}

func (f *crossResourceProtocolFixture) violationCount() int {
	f.mu.Lock()
	violations := f.violations
	f.mu.Unlock()
	return violations + len(f.project.violationSnapshot()) + len(f.flags.violationSnapshot())
}

func TestProjectEnvironmentFeatureFlagProtocolOwnershipStateSafetyAndCanonicalization(
	t *testing.T,
) {
	fixture := newCrossResourceProtocolFixture(t)
	fixture.flags.setReverseVariations(true)
	fixture.flags.setReverseCollections(true)
	t.Cleanup(func() {
		fixture.flags.setDirectReadFailure(false)
		if fixture.flags.objectCount() != 0 || fixture.project.projectCount() != 0 ||
			fixture.project.environmentCount() != 0 {
			t.Error("cross-resource Protocol fixture retained a test object")
		}
		fixture.close()
	})

	const (
		initialEnvironmentKey     = "cross-stage-a"
		replacementEnvironmentKey = "cross-stage-b"
	)
	var projectID string
	var initialEnvironmentID string
	var initialFeatureFlagID string
	var replacementEnvironmentID string
	var replacementFeatureFlagID string
	var initialProtectedUI string
	var replacementProtectedUI string
	var fallbackBaseline int

	initialConfig := crossResourceIntegrationConfig(
		fixture.apiOrigin(),
		initialEnvironmentKey,
	)
	replacementConfig := crossResourceIntegrationConfig(
		fixture.apiOrigin(),
		replacementEnvironmentKey,
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
							crossResourceFeatureFlagName,
							tfjsonpath.New("environment_id"),
						),
						plancheck.ExpectUnknownValue(
							crossResourceFeatureFlagName,
							tfjsonpath.New("id"),
						),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					crossResourceStateChecks(initialEnvironmentKey),
					crossResourceCaptureIDs(
						&projectID,
						&initialEnvironmentID,
						&initialFeatureFlagID,
						"",
						"",
					),
					crossResourceVariationCorrelationCheck(&initialEnvironmentID),
					crossResourceProtectUICheck(
						fixture.flags,
						&initialEnvironmentID,
						&initialProtectedUI,
					),
					crossResourceStateSafetyCheck,
					crossResourceMutationCountCheck(fixture, 3),
				),
			},
			{
				PreConfig: func() {
					fallbackBaseline = fixture.flags.directFallbacks()
					fixture.flags.setDirectReadFailure(true)
				},
				Config: initialConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(crossResourceProjectName, "environments.#", "3"),
					crossResourceStateChecks(initialEnvironmentKey),
					crossResourceVariationCorrelationCheck(&initialEnvironmentID),
					crossResourceStateSafetyCheck,
					crossResourceMutationCountCheck(fixture, 3),
					func(*testingterraform.State) error {
						fixture.flags.setDirectReadFailure(false)
						if fixture.flags.directFallbacks() <= fallbackBaseline {
							return fmt.Errorf("cross-resource refresh did not use complete paginated fallback")
						}
						return nil
					},
				),
			},
			{
				Config:   initialConfig,
				PlanOnly: true,
			},
			{
				PreConfig: func() {
					fixture.flags.setDirectReadFailure(false)
				},
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
							crossResourceFeatureFlagName,
							plancheck.ResourceActionReplace,
						),
						plancheck.ExpectUnknownValue(
							crossResourceFeatureFlagName,
							tfjsonpath.New("environment_id"),
						),
						plancheck.ExpectUnknownValue(
							crossResourceFeatureFlagName,
							tfjsonpath.New("id"),
						),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					crossResourceStateChecks(replacementEnvironmentKey),
					crossResourceCaptureIDs(
						&projectID,
						&replacementEnvironmentID,
						&replacementFeatureFlagID,
						initialEnvironmentID,
						initialFeatureFlagID,
					),
					crossResourceVariationCorrelationCheck(&replacementEnvironmentID),
					func(*testingterraform.State) error {
						if !fixture.flags.uiPreserved(initialProtectedUI) {
							return fmt.Errorf("Environment replacement changed UI-owned Feature Flag state")
						}
						return nil
					},
					crossResourceProtectUICheck(
						fixture.flags,
						&replacementEnvironmentID,
						&replacementProtectedUI,
					),
					crossResourceStateSafetyCheck,
					crossResourceMutationCountCheck(fixture, 8),
				),
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
							crossResourceEnvironmentName,
							plancheck.ResourceActionDestroy,
						),
						plancheck.ExpectResourceAction(
							crossResourceFeatureFlagName,
							plancheck.ResourceActionDestroy,
						),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrWith(
						crossResourceProjectName,
						"id",
						func(value string) error {
							if value != projectID {
								return fmt.Errorf("Environment deletion changed the parent Project identity")
							}
							return nil
						},
					),
					crossResourceStateSafetyCheck,
					crossResourceMutationCountCheck(fixture, 11),
					func(*testingterraform.State) error {
						if fixture.flags.objectCount() != 0 ||
							!fixture.flags.uiPreserved(replacementProtectedUI) {
							return fmt.Errorf("Environment deletion did not preserve and clean its Feature Flag")
						}
						return nil
					},
				),
			},
			{
				Config: projectOnlyConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(crossResourceProjectName, "environments.#", "2"),
					crossResourceStateSafetyCheck,
					crossResourceMutationCountCheck(fixture, 11),
				),
			},
			{
				Config:   projectOnlyConfig,
				PlanOnly: true,
			},
		},
	})

	if fixture.flags.objectCount() != 0 || fixture.project.projectCount() != 0 ||
		fixture.project.environmentCount() != 0 {
		t.Fatal("cross-resource Protocol destroy did not reach exact zero")
	}
	if fixture.violationCount() != 0 {
		t.Fatal("cross-resource Protocol fixture observed an ownership violation")
	}
	assertCrossResourceMutationOwnership(
		t,
		fixture.requestSnapshot(),
		projectID,
		initialEnvironmentID,
		replacementEnvironmentID,
	)
}

func TestFeatureFlagCrossViewDuplicatesPreserveStateAndImportDiagnosticsRedact(
	t *testing.T,
) {
	t.Parallel()

	const key = "cross-view-duplicate"
	featureFlagSchema := featureFlagResourceSchema()
	priorState, _ := featureFlagManagedResourceState(t, featureFlagSchema, key)
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
					featureFlagResourcePageJSON(
						1,
						[]string{featureFlagResourceListItemJSON(providerFeatureFlagID, key)},
					),
				)
			case request.URL.Query().Get("IsArchived") == "true":
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					featureFlagResourcePageJSON(
						1,
						[]string{featureFlagResourceListItemJSON(providerFeatureFlagSecondID, key)},
					),
				)
			default:
				t.Fatal("cross-view duplicate test received an unexpected request")
			}
		},
	))
	defer closeServer()

	readResponse := frameworkresource.ReadResponse{State: priorState}
	(&featureFlagResource{client: apiClient}).Read(
		context.Background(),
		frameworkresource.ReadRequest{State: priorState},
		&readResponse,
	)
	if !readResponse.Diagnostics.HasError() || !readResponse.State.Raw.Equal(priorState.Raw) {
		t.Fatal("active/archived duplicate resolution did not preserve Feature Flag state")
	}

	const rejected = "api-cross-import-token-marker/11111111-1111-4111-8111-111111111111/" +
		"cross-runtime-key/cross-variation-value/cross-targeting-content/cross-raw-body"
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
		State: emptyFeatureFlagResourceState(t, featureFlagSchema),
	}
	(&featureFlagResource{}).ImportState(
		context.Background(),
		frameworkresource.ImportStateRequest{ID: rejected},
		&featureFlagResponse,
	)
	if !projectResponse.Diagnostics.HasError() || !environmentResponse.Diagnostics.HasError() ||
		!featureFlagResponse.Diagnostics.HasError() {
		t.Fatal("a cross-resource Import accepted an unsafe identifier")
	}

	diagnosticOutput := fmt.Sprint(
		readResponse.Diagnostics,
		projectResponse.Diagnostics,
		environmentResponse.Diagnostics,
		featureFlagResponse.Diagnostics,
	)
	for _, unsafe := range []string{
		syntheticProviderAccessToken,
		providerEnvironmentA,
		providerFeatureFlagID,
		providerFeatureFlagSecondID,
		key,
		"api-cross-import-token-marker",
		"cross-runtime-key",
		"cross-variation-value",
		"cross-targeting-content",
		"cross-raw-body",
		"/api/v1/envs/",
	} {
		if strings.Contains(diagnosticOutput, unsafe) {
			t.Fatal("cross-resource diagnostics exposed a runtime identity or value")
		}
	}
}

func crossResourceIntegrationConfig(apiOrigin string, environmentKey string) string {
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

resource "featbit_feature_flag" "cross_child" {
  environment_id = featbit_environment.cross_child.id
  name           = "Cross-resource Feature Flag"
  key            = %q
  variation_type = "number"
  variations = [
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

data "featbit_feature_flag" "cross_child" {
  environment_id = featbit_feature_flag.cross_child.environment_id
  key            = featbit_feature_flag.cross_child.key
}
`, apiOrigin, syntheticProviderAccessToken, environmentKey, crossResourceFeatureFlagKey)
}

func crossResourceProjectOnlyConfig(apiOrigin string) string {
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
`, apiOrigin, syntheticProviderAccessToken)
}

func crossResourceStateChecks(environmentKey string) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttr(crossResourceProjectName, "name", "Cross-resource Parent"),
		resource.TestCheckResourceAttr(crossResourceEnvironmentName, "key", environmentKey),
		resource.TestCheckResourceAttr(crossResourceEnvironmentName, "description", ""),
		resource.TestCheckResourceAttr(crossResourceFeatureFlagName, "key", crossResourceFeatureFlagKey),
		resource.TestCheckResourceAttr(crossResourceFeatureFlagName, "description", ""),
		resource.TestCheckResourceAttr(crossResourceFeatureFlagName, "variation_type", "number"),
		resource.TestCheckResourceAttr(crossResourceFeatureFlagName, "variations.0.value", "1e2"),
		resource.TestCheckResourceAttr(crossResourceFeatureFlagName, "variations.1.value", "2.00"),
		resource.TestCheckResourceAttr(crossResourceProjectDataName, "environments.#", "3"),
		resource.TestCheckResourceAttr(crossResourceEnvironmentDataName, "description", ""),
		resource.TestCheckResourceAttr(crossResourceFeatureFlagDataName, "description", ""),
		resource.TestCheckResourceAttr(crossResourceFeatureFlagDataName, "variation_type", "number"),
		resource.TestCheckResourceAttrPair(
			crossResourceProjectName,
			"id",
			crossResourceEnvironmentName,
			"project_id",
		),
		resource.TestCheckResourceAttrPair(
			crossResourceEnvironmentName,
			"id",
			crossResourceFeatureFlagName,
			"environment_id",
		),
		resource.TestCheckResourceAttrPair(
			crossResourceEnvironmentName,
			"id",
			crossResourceEnvironmentDataName,
			"id",
		),
		resource.TestCheckResourceAttrPair(
			crossResourceFeatureFlagName,
			"id",
			crossResourceFeatureFlagDataName,
			"id",
		),
		crossResourceDataSourceCanonicalValuesCheck,
	)
}

func crossResourceCaptureIDs(
	projectID *string,
	environmentID *string,
	featureFlagID *string,
	environmentMustDifferFrom string,
	featureFlagMustDifferFrom string,
) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttrWith(crossResourceProjectName, "id", func(value string) error {
			if !validUUID(value) {
				return fmt.Errorf("cross-resource Project identity is invalid")
			}
			if *projectID != "" && value != *projectID {
				return fmt.Errorf("Environment replacement changed the parent Project identity")
			}
			*projectID = value
			return nil
		}),
		resource.TestCheckResourceAttrWith(crossResourceEnvironmentName, "id", func(value string) error {
			if !validUUID(value) || environmentMustDifferFrom != "" && value == environmentMustDifferFrom {
				return fmt.Errorf("cross-resource Environment identity did not replace safely")
			}
			*environmentID = value
			return nil
		}),
		resource.TestCheckResourceAttrWith(crossResourceFeatureFlagName, "id", func(value string) error {
			if !validUUID(value) || featureFlagMustDifferFrom != "" && value == featureFlagMustDifferFrom {
				return fmt.Errorf("cross-resource Feature Flag identity did not replace safely")
			}
			*featureFlagID = value
			return nil
		}),
	)
}

func crossResourceVariationCorrelationCheck(environmentID *string) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttrWith(
			crossResourceFeatureFlagName,
			"variations.0.id",
			func(value string) error {
				if value != deterministicFeatureFlagVariationID(
					*environmentID,
					crossResourceFeatureFlagKey,
					0,
				) {
					return fmt.Errorf("first variation lost deterministic UUID correlation")
				}
				return nil
			},
		),
		resource.TestCheckResourceAttrWith(
			crossResourceFeatureFlagName,
			"variations.1.id",
			func(value string) error {
				if value != deterministicFeatureFlagVariationID(
					*environmentID,
					crossResourceFeatureFlagKey,
					1,
				) {
					return fmt.Errorf("second variation lost deterministic UUID correlation")
				}
				return nil
			},
		),
	)
}

func crossResourceProtectUICheck(
	fixture *featureFlagProtocolFixture,
	environmentID *string,
	destination *string,
) resource.TestCheckFunc {
	return func(*testingterraform.State) error {
		protectedID, err := fixture.protectCustomUI(*environmentID, crossResourceFeatureFlagKey)
		if err != nil {
			return fmt.Errorf("could not protect UI-owned Feature Flag state")
		}
		*destination = protectedID
		return nil
	}
}

func crossResourceDataSourceCanonicalValuesCheck(state *testingterraform.State) error {
	resourceState, found := state.RootModule().Resources[crossResourceFeatureFlagDataName]
	if !found || resourceState.Primary == nil {
		return fmt.Errorf("Feature Flag data source state is missing")
	}
	values := map[string]bool{
		resourceState.Primary.Attributes["variations.0.value"]: true,
		resourceState.Primary.Attributes["variations.1.value"]: true,
	}
	if !values["100"] || !values["2"] || len(values) != 2 {
		return fmt.Errorf("Feature Flag data source values were not canonical and order-independent")
	}
	return nil
}

func crossResourceStateSafetyCheck(state *testingterraform.State) error {
	formatted := fmt.Sprintf("%#v", state)
	for _, unsafe := range []string{
		"test-only-protocol-environment-secret-marker",
		"requireChangeComment",
		"synthetic-ui-tag",
		"synthetic-ui-owner",
		"synthetic-ui-target",
		"synthetic-ui-rule",
		"synthetic-ui-dispatch",
		syntheticProviderAccessToken,
	} {
		if strings.Contains(formatted, unsafe) {
			return fmt.Errorf("cross-resource Terraform state retained an endpoint-only value")
		}
	}
	return nil
}

func crossResourceMutationCountCheck(
	fixture *crossResourceProtocolFixture,
	want int,
) resource.TestCheckFunc {
	return func(*testingterraform.State) error {
		got := 0
		for _, request := range fixture.requestSnapshot() {
			if request.Method != http.MethodGet {
				got++
			}
		}
		if got != want {
			return fmt.Errorf("cross-resource ownership produced %d mutations, want %d", got, want)
		}
		return nil
	}
}

func assertCrossResourceMutationOwnership(
	t *testing.T,
	requests []crossResourceFixtureRequest,
	projectID string,
	initialEnvironmentID string,
	replacementEnvironmentID string,
) {
	t.Helper()
	projectPath := "/api/v1/projects/" + projectID
	initialEnvironmentPath := projectPath + "/envs/" + initialEnvironmentID
	replacementEnvironmentPath := projectPath + "/envs/" + replacementEnvironmentID
	initialFlagPath := "/api/v1/envs/" + initialEnvironmentID + "/feature-flags/" +
		crossResourceFeatureFlagKey
	replacementFlagPath := "/api/v1/envs/" + replacementEnvironmentID + "/feature-flags/" +
		crossResourceFeatureFlagKey

	expected := []crossResourceFixtureRequest{
		{Method: http.MethodPost, Path: "/api/v1/projects"},
		{Method: http.MethodPost, Path: projectPath + "/envs"},
		{Method: http.MethodPost, Path: "/api/v1/envs/" + initialEnvironmentID + "/feature-flags"},
		{Method: http.MethodPut, Path: initialFlagPath + "/archive"},
		{Method: http.MethodDelete, Path: initialFlagPath},
		{Method: http.MethodDelete, Path: initialEnvironmentPath},
		{Method: http.MethodPost, Path: projectPath + "/envs"},
		{Method: http.MethodPost, Path: "/api/v1/envs/" + replacementEnvironmentID + "/feature-flags"},
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
		if mutationIndex >= len(expected) || request != expected[mutationIndex] {
			t.Fatal("cross-resource mutation order or ownership was incorrect")
		}
		mutationIndex++
	}
	if mutationIndex != len(expected) {
		t.Fatal("cross-resource mutation recorder was incomplete")
	}
}

func crossResourceFeatureFlagEnvironment(path string) (string, bool) {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) < 5 || segments[0] != "api" || segments[1] != "v1" ||
		segments[2] != "envs" || !validUUID(segments[3]) || segments[4] != "feature-flags" {
		return "", false
	}
	return segments[3], true
}

func crossResourceExactEnvironment(path string) (string, bool) {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) != 6 || segments[0] != "api" || segments[1] != "v1" ||
		segments[2] != "projects" || !validUUID(segments[3]) || segments[4] != "envs" ||
		!validUUID(segments[5]) {
		return "", false
	}
	return segments[5], true
}

func crossResourceExactProject(path string) (string, bool) {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) != 4 || segments[0] != "api" || segments[1] != "v1" ||
		segments[2] != "projects" || !validUUID(segments[3]) {
		return "", false
	}
	return segments[3], true
}

func crossResourceProjectContainsEnvironment(
	fixture *projectProtocolFixture,
	environmentID string,
) bool {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	for _, project := range fixture.projects {
		if fixtureEnvironmentIndex(project.Environments, environmentID) >= 0 {
			return true
		}
	}
	return false
}

func crossResourceProjectHasManagedEnvironment(fixture *projectProtocolFixture) bool {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	for _, project := range fixture.projects {
		for _, environment := range project.Environments {
			if environment.Key != "dev" && environment.Key != "prod" {
				return true
			}
		}
	}
	return false
}

func crossResourceFeatureFlagCount(
	fixture *featureFlagProtocolFixture,
	environmentID string,
) int {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	count := 0
	for _, source := range []map[string]*featureFlagFixtureObject{fixture.active, fixture.archived} {
		for _, flag := range source {
			if flag.EnvironmentID == environmentID {
				count++
			}
		}
	}
	return count
}
