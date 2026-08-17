// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const (
	projectProtocolResourceName  = "featbit_project.test"
	projectProtocolDataByIDName  = "data.featbit_project.exact_by_id"
	projectProtocolDataByKeyName = "data.featbit_project.exact_by_key"
)

func TestProjectProtocolLifecycle(t *testing.T) {
	fixture := newProjectProtocolFixture(t)
	t.Cleanup(func() {
		fixture.setDirectReadFailure(false)
		if count := fixture.projectCount(); count != 0 {
			t.Errorf("Project protocol fixture teardown count = %d, want 0", count)
		}
		fixture.close()
	})

	var initialID string
	var replacementID string
	var recreatedID string
	var fallbackBaseline int
	initialConfig := projectProtocolConfig(fixture.apiOrigin(), "Protocol Project", "protocol-project")
	replacementConfig := projectProtocolConfig(
		fixture.apiOrigin(),
		"Protocol Project",
		"protocol-project-replacement",
	)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"featbit": providerserver.NewProtocol6WithError(New("protocol-test")()),
		},
		Steps: []resource.TestStep{
			{
				Config: initialConfig,
				Check: resource.ComposeTestCheckFunc(
					projectProtocolStateChecks("Protocol Project", "protocol-project"),
					captureProjectProtocolID(&initialID, ""),
					checkProjectProtocolFixture(fixture, &initialID, "Protocol Project"),
				),
			},
			{
				Config:   initialConfig,
				PlanOnly: true,
			},
			{
				ResourceName:      projectProtocolResourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				PreConfig: func() {
					if err := fixture.renameProject(initialID, "External Project Name"); err != nil {
						t.Errorf("prepare external Project name drift: %v", err)
					}
				},
				Config: initialConfig,
				Check: resource.ComposeTestCheckFunc(
					projectProtocolStateChecks("Protocol Project", "protocol-project"),
					checkProjectProtocolFixture(fixture, &initialID, "Protocol Project"),
				),
			},
			{
				PreConfig: func() {
					fallbackBaseline = fixture.directFallbackCount()
					fixture.setDirectReadFailure(true)
				},
				Config: initialConfig,
				Check: resource.ComposeTestCheckFunc(
					projectProtocolStateChecks("Protocol Project", "protocol-project"),
					resource.TestCheckResourceAttrWith(
						projectProtocolResourceName,
						"id",
						func(string) error {
							fixture.setDirectReadFailure(false)
							if fixture.directFallbackCount() <= fallbackBaseline {
								return fmt.Errorf("Protocol v6 read did not exercise the exact collection fallback")
							}
							return nil
						},
					),
				),
			},
			{
				Config: replacementConfig,
				Check: resource.ComposeTestCheckFunc(
					projectProtocolStateChecks(
						"Protocol Project",
						"protocol-project-replacement",
					),
					captureProjectProtocolID(&replacementID, initialID),
					resource.TestCheckResourceAttrWith(
						projectProtocolResourceName,
						"id",
						func(string) error {
							if fixture.hasProject(initialID) {
								return fmt.Errorf("key replacement left the prior Project present")
							}
							return nil
						},
					),
				),
			},
			{
				PreConfig: func() {
					if err := fixture.removeProject(replacementID); err != nil {
						t.Errorf("prepare out-of-band Project deletion: %v", err)
					}
				},
				Config: replacementConfig,
				Check: resource.ComposeTestCheckFunc(
					projectProtocolStateChecks(
						"Protocol Project",
						"protocol-project-replacement",
					),
					captureProjectProtocolID(&recreatedID, replacementID),
					checkProjectProtocolFixture(fixture, &recreatedID, "Protocol Project"),
				),
			},
		},
	})

	if count := fixture.projectCount(); count != 0 {
		t.Fatalf("Project count after Protocol v6 destroy = %d, want 0", count)
	}
	if violations := fixture.violationSnapshot(); len(violations) != 0 {
		t.Fatalf("Project protocol fixture request violations = %v", violations)
	}
	assertProjectProtocolMutationOrder(t, fixture.requestSnapshot(), initialID, recreatedID)
}

func projectProtocolConfig(apiOrigin, name, key string) string {
	return fmt.Sprintf(`
provider "featbit" {
  api_url             = %q
  access_token        = %q
  http_timeout_seconds = 5
  max_concurrency     = 4
  max_retries         = 0
}

resource "featbit_project" "test" {
  name = %q
  key  = %q
}

data "featbit_project" "exact_by_id" {
  id = featbit_project.test.id
}

data "featbit_project" "exact_by_key" {
  key        = featbit_project.test.key
  depends_on = [featbit_project.test]
}
`, apiOrigin, syntheticProviderAccessToken, name, key)
}

func projectProtocolStateChecks(name, key string) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttr(projectProtocolResourceName, "name", name),
		resource.TestCheckResourceAttr(projectProtocolResourceName, "key", key),
		resource.TestCheckResourceAttr(projectProtocolResourceName, "environments.#", "2"),
		resource.TestCheckResourceAttr(projectProtocolResourceName, "environments.0.key", "dev"),
		resource.TestCheckResourceAttr(projectProtocolResourceName, "environments.1.key", "prod"),
		resource.TestCheckResourceAttr(projectProtocolDataByIDName, "name", name),
		resource.TestCheckResourceAttr(projectProtocolDataByIDName, "key", key),
		resource.TestCheckResourceAttr(projectProtocolDataByIDName, "environments.#", "2"),
		resource.TestCheckResourceAttr(projectProtocolDataByKeyName, "name", name),
		resource.TestCheckResourceAttr(projectProtocolDataByKeyName, "key", key),
		resource.TestCheckResourceAttr(projectProtocolDataByKeyName, "environments.#", "2"),
		resource.TestCheckResourceAttrPair(
			projectProtocolResourceName,
			"id",
			projectProtocolDataByIDName,
			"id",
		),
		resource.TestCheckResourceAttrPair(
			projectProtocolResourceName,
			"id",
			projectProtocolDataByKeyName,
			"id",
		),
	)
}

func captureProjectProtocolID(destination *string, mustDifferFrom string) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(
		projectProtocolResourceName,
		"id",
		func(value string) error {
			if !validUUID(value) {
				return fmt.Errorf("Project state ID is not a valid UUID")
			}
			if mustDifferFrom != "" && value == mustDifferFrom {
				return fmt.Errorf("Project replacement or recreation retained the prior UUID")
			}
			*destination = value
			return nil
		},
	)
}

func checkProjectProtocolFixture(
	fixture *projectProtocolFixture,
	projectID *string,
	wantName string,
) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(
		projectProtocolResourceName,
		"id",
		func(value string) error {
			if *projectID == "" || value != *projectID {
				return fmt.Errorf("Project state and captured fixture identity differ")
			}
			name, found := fixture.projectName(value)
			if !found {
				return fmt.Errorf("Project state identity is absent from the fixture")
			}
			if name != wantName {
				return fmt.Errorf("fixture Project name did not converge")
			}
			if fixture.projectCount() != 1 {
				return fmt.Errorf("fixture Project count did not converge to one")
			}
			return nil
		},
	)
}

func assertProjectProtocolMutationOrder(
	t *testing.T,
	requests []projectFixtureRequest,
	initialID string,
	finalID string,
) {
	t.Helper()
	mutations := make([]projectFixtureRequest, 0)
	for _, request := range requests {
		if request.Method != http.MethodGet {
			mutations = append(mutations, request)
		}
	}
	want := []projectFixtureRequest{
		{Method: http.MethodPost, Path: "/api/v1/projects"},
		{Method: http.MethodPut, Path: "/api/v1/projects/" + initialID},
		{Method: http.MethodDelete, Path: "/api/v1/projects/" + initialID},
		{Method: http.MethodPost, Path: "/api/v1/projects"},
		{Method: http.MethodPost, Path: "/api/v1/projects"},
		{Method: http.MethodDelete, Path: "/api/v1/projects/" + finalID},
	}
	if fmt.Sprint(mutations) != fmt.Sprint(want) {
		t.Fatalf("Protocol v6 mutations = %v, want %v", mutations, want)
	}

	for index, request := range requests {
		if request.Method == http.MethodPost {
			if index == 0 || requests[index-1] != (projectFixtureRequest{
				Method: http.MethodGet,
				Path:   "/api/v1/projects",
			}) {
				t.Fatalf("Project POST at request %d was not preceded by exact-zero collection preflight", index)
			}
		}
		if request.Method == http.MethodPut {
			if index+1 >= len(requests) || requests[index+1] != (projectFixtureRequest{
				Method: http.MethodGet,
				Path:   request.Path,
			}) {
				t.Fatalf("Project PUT at request %d was not followed by canonical Read", index)
			}
		}
	}
}
