// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	testingterraform "github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	environmentProtocolProjectResourceName = "featbit_project.environment_parent"
	environmentProtocolResourceName        = "featbit_environment.test"
	environmentProtocolDataSourceName      = "data.featbit_environment.exact"
)

func TestEnvironmentProtocolLifecycle(t *testing.T) {
	fixture := newProjectProtocolFixture(t)
	t.Cleanup(func() {
		fixture.setDirectReadFailure(false)
		fixture.setDirectEnvironmentReadFailure(false)
		if count := fixture.environmentCount(); count != 0 {
			t.Errorf("Environment protocol fixture teardown count = %d, want 0", count)
		}
		if count := fixture.projectCount(); count != 0 {
			t.Errorf("Environment protocol parent teardown count = %d, want 0", count)
		}
		fixture.close()
	})

	var initialProjectID string
	var initialEnvironmentID string
	var keyReplacementEnvironmentID string
	var parentReplacementProjectID string
	var parentReplacementEnvironmentID string
	var recreatedEnvironmentID string
	var cascadeProjectID string
	var cascadeEnvironmentID string
	var fallbackBaseline int
	var settingsBaseline int

	initialConfig := environmentProtocolConfig(
		fixture.apiOrigin(),
		"Protocol Parent",
		"protocol-parent",
		"Protocol Staging",
		"protocol-staging",
		"Protocol description",
	)
	keyReplacementConfig := environmentProtocolConfig(
		fixture.apiOrigin(),
		"Protocol Parent",
		"protocol-parent",
		"Protocol Staging",
		"protocol-staging-replacement",
		"Protocol description",
	)
	parentReplacementConfig := environmentProtocolConfig(
		fixture.apiOrigin(),
		"Protocol Parent",
		"protocol-parent-replacement",
		"Protocol Staging",
		"protocol-staging-replacement",
		"Protocol description",
	)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"featbit": providerserver.NewProtocol6WithError(New("protocol-test")()),
		},
		Steps: []resource.TestStep{
			{
				Config: initialConfig,
				Check: resource.ComposeTestCheckFunc(
					environmentProtocolStateChecks(
						"Protocol Staging",
						"protocol-staging",
						"Protocol description",
					),
					captureEnvironmentProtocolIDs(
						&initialProjectID,
						&initialEnvironmentID,
						"",
						"",
					),
					checkEnvironmentProtocolFixture(
						fixture,
						&initialProjectID,
						&initialEnvironmentID,
						"Protocol Staging",
						"Protocol description",
					),
				),
			},
			{
				Config:   initialConfig,
				PlanOnly: true,
			},
			{
				ResourceName:      environmentProtocolResourceName,
				ImportState:       true,
				ImportStateIdFunc: environmentProtocolImportID(&initialProjectID, &initialEnvironmentID),
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					settingsBaseline = fixture.settingsPreservedUpdateCount()
					if err := fixture.renameEnvironment(
						initialProjectID,
						initialEnvironmentID,
						"External Environment Name",
						"External description",
					); err != nil {
						t.Errorf("prepare external Environment drift: %v", err)
					}
				},
				Config: initialConfig,
				Check: resource.ComposeTestCheckFunc(
					environmentProtocolStateChecks(
						"Protocol Staging",
						"protocol-staging",
						"Protocol description",
					),
					checkEnvironmentProtocolFixture(
						fixture,
						&initialProjectID,
						&initialEnvironmentID,
						"Protocol Staging",
						"Protocol description",
					),
					resource.TestCheckResourceAttrWith(
						environmentProtocolResourceName,
						"id",
						func(string) error {
							if fixture.settingsPreservedUpdateCount() <= settingsBaseline {
								return fmt.Errorf("Protocol v6 Update did not preserve the existing settings")
							}
							return nil
						},
					),
				),
			},
			{
				PreConfig: func() {
					fallbackBaseline = fixture.environmentDirectFallbackCount()
					fixture.setDirectEnvironmentReadFailure(true)
				},
				Config: initialConfig,
				Check: resource.ComposeTestCheckFunc(
					environmentProtocolStateChecks(
						"Protocol Staging",
						"protocol-staging",
						"Protocol description",
					),
					resource.TestCheckResourceAttrWith(
						environmentProtocolResourceName,
						"id",
						func(string) error {
							fixture.setDirectEnvironmentReadFailure(false)
							if fixture.environmentDirectFallbackCount() <= fallbackBaseline {
								return fmt.Errorf("Protocol v6 read did not exercise exact parent fallback")
							}
							return nil
						},
					),
				),
			},
			{
				Config: keyReplacementConfig,
				Check: resource.ComposeTestCheckFunc(
					environmentProtocolStateChecks(
						"Protocol Staging",
						"protocol-staging-replacement",
						"Protocol description",
					),
					captureEnvironmentProtocolIDs(
						&initialProjectID,
						&keyReplacementEnvironmentID,
						"",
						initialEnvironmentID,
					),
					resource.TestCheckResourceAttrWith(
						environmentProtocolResourceName,
						"id",
						func(string) error {
							if fixture.hasEnvironment(initialProjectID, initialEnvironmentID) {
								return fmt.Errorf("key replacement left the prior Environment present")
							}
							return nil
						},
					),
				),
			},
			{
				Config: parentReplacementConfig,
				Check: resource.ComposeTestCheckFunc(
					environmentProtocolStateChecks(
						"Protocol Staging",
						"protocol-staging-replacement",
						"Protocol description",
					),
					captureEnvironmentProtocolIDs(
						&parentReplacementProjectID,
						&parentReplacementEnvironmentID,
						initialProjectID,
						keyReplacementEnvironmentID,
					),
					resource.TestCheckResourceAttrWith(
						environmentProtocolResourceName,
						"id",
						func(string) error {
							if fixture.hasProject(initialProjectID) {
								return fmt.Errorf("parent replacement left the prior Project present")
							}
							return nil
						},
					),
				),
			},
			{
				PreConfig: func() {
					if err := fixture.removeEnvironment(
						parentReplacementProjectID,
						parentReplacementEnvironmentID,
					); err != nil {
						t.Errorf("prepare out-of-band Environment deletion: %v", err)
					}
				},
				Config: parentReplacementConfig,
				Check: resource.ComposeTestCheckFunc(
					environmentProtocolStateChecks(
						"Protocol Staging",
						"protocol-staging-replacement",
						"Protocol description",
					),
					captureEnvironmentProtocolIDs(
						&parentReplacementProjectID,
						&recreatedEnvironmentID,
						"",
						parentReplacementEnvironmentID,
					),
					checkEnvironmentProtocolFixture(
						fixture,
						&parentReplacementProjectID,
						&recreatedEnvironmentID,
						"Protocol Staging",
						"Protocol description",
					),
				),
			},
			{
				PreConfig: func() {
					if err := fixture.removeProject(parentReplacementProjectID); err != nil {
						t.Errorf("prepare Project cascade deletion: %v", err)
					}
				},
				Config: parentReplacementConfig,
				Check: resource.ComposeTestCheckFunc(
					environmentProtocolStateChecks(
						"Protocol Staging",
						"protocol-staging-replacement",
						"Protocol description",
					),
					captureEnvironmentProtocolIDs(
						&cascadeProjectID,
						&cascadeEnvironmentID,
						parentReplacementProjectID,
						recreatedEnvironmentID,
					),
					checkEnvironmentProtocolFixture(
						fixture,
						&cascadeProjectID,
						&cascadeEnvironmentID,
						"Protocol Staging",
						"Protocol description",
					),
				),
			},
		},
	})

	if count := fixture.environmentCount(); count != 0 {
		t.Fatalf("Environment count after Protocol v6 destroy = %d, want 0", count)
	}
	if count := fixture.projectCount(); count != 0 {
		t.Fatalf("Project count after Environment Protocol v6 destroy = %d, want 0", count)
	}
	if violations := fixture.violationSnapshot(); len(violations) != 0 {
		t.Fatalf("Environment protocol fixture request violations = %v", violations)
	}
	if updates := fixture.settingsPreservedUpdateCount(); updates != 1 {
		t.Fatalf("settings-preserving Environment updates = %d, want 1", updates)
	}
	assertEnvironmentProtocolMutationOrder(t, fixture.requestSnapshot())
}

func environmentProtocolConfig(
	apiOrigin string,
	projectName string,
	projectKey string,
	environmentName string,
	environmentKey string,
	description string,
) string {
	return fmt.Sprintf(`
provider "featbit" {
  api_url              = %q
  access_token         = %q
  http_timeout_seconds = 5
  max_concurrency      = 4
  max_retries          = 0
}

resource "featbit_project" "environment_parent" {
  name = %q
  key  = %q
}

resource "featbit_environment" "test" {
  project_id = featbit_project.environment_parent.id
  name        = %q
  key         = %q
  description = %q
}

data "featbit_environment" "exact" {
  project_id = featbit_environment.test.project_id
  id         = featbit_environment.test.id
}
`, apiOrigin, syntheticProviderAccessToken, projectName, projectKey,
		environmentName, environmentKey, description)
}

func environmentProtocolStateChecks(name string, key string, description string) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttr(environmentProtocolResourceName, "name", name),
		resource.TestCheckResourceAttr(environmentProtocolResourceName, "key", key),
		resource.TestCheckResourceAttr(environmentProtocolResourceName, "description", description),
		resource.TestCheckResourceAttr(environmentProtocolDataSourceName, "name", name),
		resource.TestCheckResourceAttr(environmentProtocolDataSourceName, "key", key),
		resource.TestCheckResourceAttr(environmentProtocolDataSourceName, "description", description),
		resource.TestCheckResourceAttrPair(
			environmentProtocolResourceName,
			"project_id",
			environmentProtocolProjectResourceName,
			"id",
		),
		resource.TestCheckResourceAttrPair(
			environmentProtocolResourceName,
			"project_id",
			environmentProtocolDataSourceName,
			"project_id",
		),
		resource.TestCheckResourceAttrPair(
			environmentProtocolResourceName,
			"id",
			environmentProtocolDataSourceName,
			"id",
		),
	)
}

func captureEnvironmentProtocolIDs(
	projectDestination *string,
	environmentDestination *string,
	projectMustDifferFrom string,
	environmentMustDifferFrom string,
) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttrWith(
			environmentProtocolProjectResourceName,
			"id",
			func(value string) error {
				if !validUUID(value) {
					return fmt.Errorf("Project state ID is not a valid UUID")
				}
				if projectMustDifferFrom != "" && value == projectMustDifferFrom {
					return fmt.Errorf("Project replacement retained the prior UUID")
				}
				*projectDestination = value
				return nil
			},
		),
		resource.TestCheckResourceAttrWith(
			environmentProtocolResourceName,
			"id",
			func(value string) error {
				if !validUUID(value) {
					return fmt.Errorf("Environment state ID is not a valid UUID")
				}
				if environmentMustDifferFrom != "" && value == environmentMustDifferFrom {
					return fmt.Errorf("Environment replacement or recreation retained the prior UUID")
				}
				*environmentDestination = value
				return nil
			},
		),
	)
}

func checkEnvironmentProtocolFixture(
	fixture *projectProtocolFixture,
	projectID *string,
	environmentID *string,
	wantName string,
	wantDescription string,
) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(
		environmentProtocolResourceName,
		"id",
		func(value string) error {
			if *projectID == "" || *environmentID == "" || value != *environmentID {
				return fmt.Errorf("Environment state and captured fixture identity differ")
			}
			name, description, found := fixture.environmentValues(*projectID, value)
			if !found {
				return fmt.Errorf("Environment state identity is absent from the exact parent")
			}
			if name != wantName || description != wantDescription {
				return fmt.Errorf("fixture Environment fields did not converge")
			}
			if fixture.projectCount() != 1 || fixture.environmentCount() != 3 {
				return fmt.Errorf("fixture Project/Environment counts did not converge to 1/3")
			}
			return nil
		},
	)
}

func environmentProtocolImportID(
	projectID *string,
	environmentID *string,
) resource.ImportStateIdFunc {
	return func(*testingterraform.State) (string, error) {
		if *projectID == "" || *environmentID == "" {
			return "", fmt.Errorf("Environment import identities were not captured")
		}
		return *projectID + "/" + *environmentID, nil
	}
}

func assertEnvironmentProtocolMutationOrder(t *testing.T, requests []projectFixtureRequest) {
	t.Helper()

	mutations := make([]projectFixtureRequest, 0)
	projectPosts := 0
	projectDeletes := 0
	environmentPosts := 0
	environmentPuts := 0
	environmentDeletes := 0
	for index, request := range requests {
		if request.Method == http.MethodGet {
			continue
		}
		mutations = append(mutations, request)
		isEnvironmentPath := strings.Contains(request.Path, "/envs")
		switch {
		case request.Method == http.MethodPost && isEnvironmentPath:
			environmentPosts++
			if index == 0 || requests[index-1] != (projectFixtureRequest{
				Method: http.MethodGet,
				Path:   strings.TrimSuffix(request.Path, "/envs"),
			}) {
				t.Fatalf("Environment POST at request %d lacked exact-parent preflight", index)
			}
		case request.Method == http.MethodPut && isEnvironmentPath:
			environmentPuts++
			if index == 0 || requests[index-1].Method != http.MethodGet ||
				requests[index-1].Path != environmentParentPath(request.Path) {
				t.Fatalf("Environment PUT at request %d lacked immediate settings read", index)
			}
			if index+1 >= len(requests) || requests[index+1] != (projectFixtureRequest{
				Method: http.MethodGet,
				Path:   request.Path,
			}) {
				t.Fatalf("Environment PUT at request %d lacked canonical Read", index)
			}
		case request.Method == http.MethodDelete && isEnvironmentPath:
			environmentDeletes++
			if index+1 >= len(requests) || requests[index+1] != (projectFixtureRequest{
				Method: http.MethodGet,
				Path:   request.Path,
			}) {
				t.Fatalf("Environment DELETE at request %d lacked exact absence Read", index)
			}
		case request.Method == http.MethodPost:
			projectPosts++
		case request.Method == http.MethodDelete:
			projectDeletes++
		default:
			t.Fatalf("unexpected Protocol v6 mutation = %v", request)
		}
	}

	if projectPosts != 3 || projectDeletes != 2 || environmentPosts != 5 ||
		environmentPuts != 1 || environmentDeletes != 3 {
		t.Fatalf(
			"Protocol v6 mutation counts = project POST/DELETE %d/%d, Environment POST/PUT/DELETE %d/%d/%d",
			projectPosts,
			projectDeletes,
			environmentPosts,
			environmentPuts,
			environmentDeletes,
		)
	}
	if len(mutations) < 2 ||
		mutations[len(mutations)-2].Method != http.MethodDelete ||
		!strings.Contains(mutations[len(mutations)-2].Path, "/envs/") ||
		mutations[len(mutations)-1].Method != http.MethodDelete ||
		strings.Contains(mutations[len(mutations)-1].Path, "/envs") {
		t.Fatalf("final Protocol v6 cleanup was not child-before-parent: %v", mutations)
	}
}

func environmentParentPath(environmentPath string) string {
	marker := "/envs/"
	index := strings.Index(environmentPath, marker)
	if index < 0 {
		return ""
	}
	return environmentPath[:index]
}
