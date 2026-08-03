// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	testingterraform "github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	projectEnvironmentIntegrationProjectName         = "featbit_project.ownership_parent"
	projectEnvironmentIntegrationEnvironmentName     = "featbit_environment.standalone"
	projectEnvironmentIntegrationProjectDataName     = "data.featbit_project.observed"
	projectEnvironmentIntegrationEnvironmentDataName = "data.featbit_environment.exact"
)

func TestProjectEnvironmentProtocolOwnershipStateSafetyAndCanonicalization(t *testing.T) {
	fixture := newProjectProtocolFixture(t)
	t.Cleanup(func() {
		if count := fixture.environmentCount(); count != 0 {
			t.Errorf("Project/Environment integration fixture retained Environment objects")
		}
		if count := fixture.projectCount(); count != 0 {
			t.Errorf("Project/Environment integration fixture retained Project objects")
		}
		fixture.close()
	})

	var projectID string
	var environmentID string
	config := projectEnvironmentIntegrationConfig(fixture.apiOrigin(), "Ownership Parent", "Stage")
	updatedConfig := projectEnvironmentIntegrationConfig(
		fixture.apiOrigin(),
		"Ownership Parent Updated",
		"Stage Updated",
	)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"featbit": providerserver.NewProtocol6WithError(New("protocol-test")()),
		},
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						projectEnvironmentIntegrationProjectName,
						"environments.#",
						"2",
					),
					projectEnvironmentIntegrationCanonicalChecks(projectEnvironmentIntegrationProjectDataName),
					projectEnvironmentIntegrationEnvironmentChecks(),
					projectEnvironmentIntegrationCaptureIDs(&projectID, &environmentID),
					projectEnvironmentIntegrationStateSafetyCheck,
					projectEnvironmentIntegrationMutationCountCheck(fixture, 2),
				),
			},
			{
				// Refreshing the parent observes the standalone Environment in
				// its Computed list. That state-only change must not produce a
				// competing Project or Environment mutation.
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					projectEnvironmentIntegrationCanonicalChecks(projectEnvironmentIntegrationProjectName),
					projectEnvironmentIntegrationCanonicalChecks(projectEnvironmentIntegrationProjectDataName),
					projectEnvironmentIntegrationEnvironmentChecks(),
					resource.TestCheckResourceAttrPair(
						projectEnvironmentIntegrationProjectName,
						"environments.2.id",
						projectEnvironmentIntegrationEnvironmentName,
						"id",
					),
					projectEnvironmentIntegrationStateSafetyCheck,
					projectEnvironmentIntegrationMutationCountCheck(fixture, 2),
				),
			},
			{
				Config: updatedConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(
							projectEnvironmentIntegrationProjectName,
							plancheck.ResourceActionUpdate,
						),
						plancheck.ExpectResourceAction(
							projectEnvironmentIntegrationEnvironmentName,
							plancheck.ResourceActionUpdate,
						),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						projectEnvironmentIntegrationProjectName,
						"name",
						"Ownership Parent Updated",
					),
					resource.TestCheckResourceAttr(
						projectEnvironmentIntegrationEnvironmentName,
						"name",
						"Stage Updated",
					),
					resource.TestCheckResourceAttr(
						projectEnvironmentIntegrationProjectDataName,
						"name",
						"Ownership Parent Updated",
					),
					resource.TestCheckResourceAttr(
						projectEnvironmentIntegrationEnvironmentDataName,
						"name",
						"Stage Updated",
					),
					projectEnvironmentIntegrationIDsUnchanged(&projectID, &environmentID),
					projectEnvironmentIntegrationCanonicalChecks(projectEnvironmentIntegrationProjectName),
					projectEnvironmentIntegrationCanonicalChecks(projectEnvironmentIntegrationProjectDataName),
					projectEnvironmentIntegrationEnvironmentChecks(),
					projectEnvironmentIntegrationStateSafetyCheck,
					projectEnvironmentIntegrationMutationCountCheck(fixture, 4),
					func(*testingterraform.State) error {
						if fixture.settingsPreservedUpdateCount() != 1 {
							return fmt.Errorf("Environment in-place update did not preserve settings")
						}
						return nil
					},
				),
			},
			{
				Config:   updatedConfig,
				PlanOnly: true,
			},
		},
	})

	if fixture.environmentCount() != 0 || fixture.projectCount() != 0 {
		t.Fatal("Project/Environment Protocol v6 destroy did not reach exact zero")
	}
	if violations := fixture.violationSnapshot(); len(violations) != 0 {
		t.Fatal("Project/Environment integration fixture observed a request contract violation")
	}
	assertProjectEnvironmentIntegrationMutationOwnership(t, fixture.requestSnapshot())
}

func TestProjectEnvironmentAmbiguousReadsPreserveBothResourceStates(t *testing.T) {
	projectClient, closeProjectServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			switch request.URL.EscapedPath() {
			case "/api/v1/projects/" + providerProjectID:
				writeProjectResourceEnvelope(t, response, http.StatusNotFound, "null")
			case "/api/v1/projects":
				duplicate := providerProjectJSON(providerProjectID, "First", "first")
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					"["+duplicate+","+duplicate+"]",
				)
			default:
				t.Fatal("Project ambiguity test received an unexpected request")
			}
		},
	))
	defer closeProjectServer()

	projectSchema := projectResourceTestSchema(t)
	projectState := projectResourceTestState(t, projectSchema, "Project", "project-key")
	projectResponse := frameworkresource.ReadResponse{State: tfsdk.State{Schema: projectSchema}}
	(&projectResource{client: projectClient}).Read(
		context.Background(),
		frameworkresource.ReadRequest{State: projectState},
		&projectResponse,
	)
	if !projectResponse.Diagnostics.HasError() ||
		!projectResponse.State.Raw.Equal(projectState.Raw) {
		t.Fatal("ambiguous Project read did not preserve the prior state")
	}

	environmentClient, closeEnvironmentServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			switch request.URL.EscapedPath() {
			case "/api/v1/projects/" + providerProjectID + "/envs/" + providerEnvironmentA:
				writeProjectResourceEnvelope(t, response, http.StatusNotFound, "null")
			case "/api/v1/projects/" + providerProjectID:
				duplicate := `{"id":"` + providerEnvironmentA +
					`","name":"Staging","key":"staging","description":""}`
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerEnvironmentParentJSON(
						providerProjectID,
						"["+duplicate+","+duplicate+"]",
					),
				)
			default:
				t.Fatal("Environment ambiguity test received an unexpected request")
			}
		},
	))
	defer closeEnvironmentServer()

	environmentSchema := environmentResourceTestSchema(t)
	environmentState := environmentResourceTestState(
		t,
		environmentSchema,
		providerProjectID,
		providerEnvironmentA,
		"Staging",
		"staging",
		"",
	)
	environmentResponse := frameworkresource.ReadResponse{
		State: tfsdk.State{Schema: environmentSchema},
	}
	(&environmentResource{client: environmentClient}).Read(
		context.Background(),
		frameworkresource.ReadRequest{State: environmentState},
		&environmentResponse,
	)
	if !environmentResponse.Diagnostics.HasError() ||
		!environmentResponse.State.Raw.Equal(environmentState.Raw) {
		t.Fatal("ambiguous Environment read did not preserve the prior state")
	}

	diagnosticOutput := fmt.Sprint(
		projectResponse.Diagnostics,
		environmentResponse.Diagnostics,
	)
	for _, unsafe := range []string{
		syntheticProviderAccessToken,
		providerProjectID,
		providerEnvironmentA,
		"project-key",
		"staging",
		"/api/v1/projects/",
	} {
		if strings.Contains(diagnosticOutput, unsafe) {
			t.Fatal("ambiguous read diagnostic exposed a runtime identity")
		}
	}
}

func TestProjectEnvironmentImportDiagnosticsRejectUnsafeIdentifiersWithoutEcho(t *testing.T) {
	const rejected = "api-test-only-token/11111111-1111-4111-8111-111111111111/" +
		"runtime-key/server-detail"

	projectSchema := projectResourceTestSchema(t)
	projectResponse := frameworkresource.ImportStateResponse{
		State: tfsdk.State{Schema: projectSchema},
	}
	(&projectResource{}).ImportState(
		context.Background(),
		frameworkresource.ImportStateRequest{ID: rejected},
		&projectResponse,
	)

	environmentSchema := environmentResourceTestSchema(t)
	environmentResponse := frameworkresource.ImportStateResponse{
		State: tfsdk.State{Schema: environmentSchema},
	}
	(&environmentResource{}).ImportState(
		context.Background(),
		frameworkresource.ImportStateRequest{ID: rejected},
		&environmentResponse,
	)

	if !projectResponse.Diagnostics.HasError() || !environmentResponse.Diagnostics.HasError() {
		t.Fatal("Project/Environment Import accepted a malformed identifier")
	}
	if strings.Contains(
		fmt.Sprint(projectResponse.Diagnostics, environmentResponse.Diagnostics),
		rejected,
	) {
		t.Fatal("Project/Environment Import diagnostic echoed the rejected identifier")
	}
}

func projectEnvironmentIntegrationConfig(apiOrigin, projectName, environmentName string) string {
	return fmt.Sprintf(`
provider "featbit" {
  api_url              = %q
  access_token         = %q
  http_timeout_seconds = 5
  max_concurrency      = 4
  max_retries          = 0
}

resource "featbit_project" "ownership_parent" {
  name = %q
  key  = "ownership-parent"
}

resource "featbit_environment" "standalone" {
  project_id = featbit_project.ownership_parent.id
  name       = %q
  key        = "stage"
}

data "featbit_project" "observed" {
  id         = featbit_project.ownership_parent.id
  depends_on = [featbit_environment.standalone]
}

data "featbit_environment" "exact" {
  project_id = featbit_environment.standalone.project_id
  id         = featbit_environment.standalone.id
}
`, apiOrigin, syntheticProviderAccessToken, projectName, environmentName)
}

func projectEnvironmentIntegrationCaptureIDs(
	projectID *string,
	environmentID *string,
) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttrWith(
			projectEnvironmentIntegrationProjectName,
			"id",
			func(value string) error {
				if !validUUID(value) {
					return fmt.Errorf("Project identity is not a UUID")
				}
				*projectID = value
				return nil
			},
		),
		resource.TestCheckResourceAttrWith(
			projectEnvironmentIntegrationEnvironmentName,
			"id",
			func(value string) error {
				if !validUUID(value) {
					return fmt.Errorf("Environment identity is not a UUID")
				}
				*environmentID = value
				return nil
			},
		),
	)
}

func projectEnvironmentIntegrationIDsUnchanged(
	projectID *string,
	environmentID *string,
) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttrWith(
			projectEnvironmentIntegrationProjectName,
			"id",
			func(value string) error {
				if value != *projectID {
					return fmt.Errorf("Project in-place update changed identity")
				}
				return nil
			},
		),
		resource.TestCheckResourceAttrWith(
			projectEnvironmentIntegrationEnvironmentName,
			"id",
			func(value string) error {
				if value != *environmentID {
					return fmt.Errorf("Environment in-place update changed identity")
				}
				return nil
			},
		),
	)
}

func projectEnvironmentIntegrationCanonicalChecks(resourceName string) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttr(resourceName, "environments.#", "3"),
		resource.TestCheckResourceAttr(resourceName, "environments.0.key", "dev"),
		resource.TestCheckResourceAttr(resourceName, "environments.1.key", "prod"),
		resource.TestCheckResourceAttr(resourceName, "environments.2.key", "stage"),
		resource.TestCheckResourceAttr(resourceName, "environments.2.description", ""),
	)
}

func projectEnvironmentIntegrationEnvironmentChecks() resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttr(projectEnvironmentIntegrationEnvironmentName, "description", ""),
		resource.TestCheckResourceAttr(projectEnvironmentIntegrationEnvironmentDataName, "description", ""),
		resource.TestCheckResourceAttrPair(
			projectEnvironmentIntegrationEnvironmentName,
			"project_id",
			projectEnvironmentIntegrationProjectName,
			"id",
		),
		resource.TestCheckResourceAttrPair(
			projectEnvironmentIntegrationEnvironmentName,
			"id",
			projectEnvironmentIntegrationEnvironmentDataName,
			"id",
		),
		resource.TestCheckResourceAttrPair(
			projectEnvironmentIntegrationProjectDataName,
			"environments.2.id",
			projectEnvironmentIntegrationEnvironmentName,
			"id",
		),
	)
}

func projectEnvironmentIntegrationStateSafetyCheck(state *testingterraform.State) error {
	formatted := fmt.Sprintf("%#v", state)
	for _, unsafe := range []string{
		"test-only-protocol-environment-secret-marker",
		"requireChangeComment",
		`"future"`,
		syntheticProviderAccessToken,
	} {
		if strings.Contains(formatted, unsafe) {
			return fmt.Errorf("Terraform state retained an endpoint-only value")
		}
	}
	return nil
}

func projectEnvironmentIntegrationMutationCountCheck(
	fixture *projectProtocolFixture,
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
			return fmt.Errorf("shared ownership produced %d mutations, want %d", got, want)
		}
		return nil
	}
}

func assertProjectEnvironmentIntegrationMutationOwnership(
	t *testing.T,
	requests []projectFixtureRequest,
) {
	t.Helper()
	mutations := make([]projectFixtureRequest, 0, 6)
	for _, request := range requests {
		if request.Method != http.MethodGet {
			mutations = append(mutations, request)
		}
	}
	if len(mutations) != 6 {
		t.Fatalf("Project/Environment shared ownership mutation count = %d, want 6", len(mutations))
	}
	if mutations[0].Method != http.MethodPost || strings.Contains(mutations[0].Path, "/envs") ||
		mutations[1].Method != http.MethodPost || !strings.Contains(mutations[1].Path, "/envs") ||
		mutations[2].Method != http.MethodPut || strings.Contains(mutations[2].Path, "/envs") ||
		mutations[3].Method != http.MethodPut || !strings.Contains(mutations[3].Path, "/envs/") ||
		mutations[4].Method != http.MethodDelete || !strings.Contains(mutations[4].Path, "/envs/") ||
		mutations[5].Method != http.MethodDelete || strings.Contains(mutations[5].Path, "/envs") {
		t.Fatal("Project/Environment shared ownership did not mutate once or clean child before parent")
	}
}
