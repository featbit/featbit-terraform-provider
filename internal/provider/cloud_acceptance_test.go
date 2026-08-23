// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	testingterraform "github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	cloudAcceptanceProjectResourceName = "featbit_project.cloud_parent"
	cloudAcceptanceEnvironmentName     = "featbit_environment.cloud_child"
	cloudAcceptanceProjectDataName     = "data.featbit_project.cloud_parent"
	cloudAcceptanceEnvironmentDataName = "data.featbit_environment.cloud_child"
	cloudAcceptanceTimeout             = 2 * time.Minute
)

func TestAccProjectEnvironmentCloudLifecycle(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("set TF_ACC=1 to run trusted FeatBit Cloud acceptance")
	}
	accessToken, ok := os.LookupEnv(envAccessToken)
	if !ok || accessToken == "" {
		t.Skip("trusted FeatBit Cloud acceptance requires FEATBIT_ACCESS_TOKEN")
	}
	apiURL := cloudAcceptanceAPIURL(t)
	apiClient, err := client.New(apiURL, accessToken, client.Options{
		HTTPTimeout:     client.DefaultHTTPTimeout,
		MaxConcurrency:  client.DefaultMaxConcurrency,
		MaxRetries:      client.DefaultMaxRetries,
		ProviderVersion: "cloud-acceptance",
	})
	if err != nil {
		t.Fatal("could not construct the trusted Cloud acceptance client")
	}
	cloudProxy := newCloudAcceptanceProxy(apiURL)
	t.Cleanup(cloudProxy.close)

	prefix := cloudAcceptancePrefix(t)
	projectKeyA := prefix + "-project-a"
	projectKeyB := prefix + "-project-b"
	environmentKeyA := prefix + "-env-a"
	environmentKeyB := prefix + "-env-b"
	inventory := newCloudAcceptanceInventory(
		apiClient,
		[]string{projectKeyA, projectKeyB},
		[]string{environmentKeyA, environmentKeyB},
	)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
		defer cancel()
		if err := inventory.cleanupAndVerify(ctx); err != nil {
			t.Error("trusted Cloud acceptance cleanup did not reach exact zero")
		}
	})

	var projectID string
	var environmentID string
	var replacementEnvironmentID string
	var replacementProjectID string
	var replacementProjectEnvironmentID string
	var recreatedEnvironmentID string
	var cascadeProjectID string
	var cascadeEnvironmentID string
	var lifecycleCompleted bool

	initialConfig := cloudAcceptanceConfig(
		cloudProxy.apiOrigin(),
		"Terraform Acceptance Project",
		projectKeyA,
		"Terraform Acceptance Environment",
		environmentKeyA,
		"Initial Cloud acceptance description",
	)
	updatedConfig := cloudAcceptanceConfig(
		cloudProxy.apiOrigin(),
		"Terraform Acceptance Project Updated",
		projectKeyA,
		"Terraform Acceptance Environment Updated",
		environmentKeyA,
		"Updated Cloud acceptance description",
	)
	environmentReplacementConfig := cloudAcceptanceConfig(
		cloudProxy.apiOrigin(),
		"Terraform Acceptance Project Updated",
		projectKeyA,
		"Terraform Acceptance Environment Updated",
		environmentKeyB,
		"Updated Cloud acceptance description",
	)
	parentReplacementConfig := cloudAcceptanceConfig(
		cloudProxy.apiOrigin(),
		"Terraform Acceptance Project Updated",
		projectKeyB,
		"Terraform Acceptance Environment Updated",
		environmentKeyB,
		"Updated Cloud acceptance description",
	)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"featbit": providerserver.NewProtocol6WithError(New("cloud-acceptance")()),
		},
		PreCheck: func() {
			cloudAcceptancePreCheck(t)
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
			return cloudProxy.verifySettingsPreservation(2)
		},
		Steps: []resource.TestStep{
			{
				Config: initialConfig,
				Check: resource.ComposeTestCheckFunc(
					cloudAcceptanceStateChecks(
						"Terraform Acceptance Project",
						projectKeyA,
						"Terraform Acceptance Environment",
						environmentKeyA,
						"Initial Cloud acceptance description",
					),
					cloudAcceptanceCaptureIDs(
						inventory,
						&projectID,
						&environmentID,
						projectKeyA,
						environmentKeyA,
						"",
						"",
					),
				),
			},
			{
				Config:   initialConfig,
				PlanOnly: true,
			},
			{
				// Plan-only refresh does not persist the parent's newly observed
				// standalone Environment into the state used by a later Import
				// comparison. A normal no-op step persists that Computed-only
				// observation without issuing a mutation.
				Config: initialConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						cloudAcceptanceProjectResourceName,
						"environments.#",
						"3",
					),
					resource.TestCheckResourceAttrPair(
						cloudAcceptanceProjectResourceName,
						"environments.2.id",
						cloudAcceptanceEnvironmentName,
						"id",
					),
				),
			},
			{
				ResourceName:      cloudAcceptanceProjectResourceName,
				ImportState:       true,
				ImportStateIdFunc: cloudAcceptanceProjectImportID(&projectID),
				ImportStateVerify: true,
			},
			{
				ResourceName:      cloudAcceptanceEnvironmentName,
				ImportState:       true,
				ImportStateIdFunc: cloudAcceptanceEnvironmentImportID(&projectID, &environmentID),
				ImportStateVerify: true,
			},
			{
				Config:   initialConfig,
				PlanOnly: true,
			},
			{
				Config: updatedConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(
							cloudAcceptanceProjectResourceName,
							plancheck.ResourceActionUpdate,
						),
						plancheck.ExpectResourceAction(
							cloudAcceptanceEnvironmentName,
							plancheck.ResourceActionUpdate,
						),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					cloudAcceptanceStateChecks(
						"Terraform Acceptance Project Updated",
						projectKeyA,
						"Terraform Acceptance Environment Updated",
						environmentKeyA,
						"Updated Cloud acceptance description",
					),
					cloudAcceptanceIDsUnchanged(&projectID, &environmentID),
					func(*testingterraform.State) error {
						return cloudProxy.verifySettingsPreservation(1)
					},
				),
			},
			{
				PreConfig: func() {
					cloudAcceptanceDriftProjectAndEnvironment(
						t,
						apiClient,
						projectID,
						environmentID,
					)
				},
				Config: updatedConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(
							cloudAcceptanceProjectResourceName,
							plancheck.ResourceActionUpdate,
						),
						plancheck.ExpectResourceAction(
							cloudAcceptanceEnvironmentName,
							plancheck.ResourceActionUpdate,
						),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					cloudAcceptanceStateChecks(
						"Terraform Acceptance Project Updated",
						projectKeyA,
						"Terraform Acceptance Environment Updated",
						environmentKeyA,
						"Updated Cloud acceptance description",
					),
					func(*testingterraform.State) error {
						return cloudProxy.verifySettingsPreservation(2)
					},
				),
			},
			{
				Config: environmentReplacementConfig,
				Check: resource.ComposeTestCheckFunc(
					cloudAcceptanceStateChecks(
						"Terraform Acceptance Project Updated",
						projectKeyA,
						"Terraform Acceptance Environment Updated",
						environmentKeyB,
						"Updated Cloud acceptance description",
					),
					cloudAcceptanceCaptureIDs(
						inventory,
						&projectID,
						&replacementEnvironmentID,
						projectKeyA,
						environmentKeyB,
						"",
						environmentID,
					),
				),
			},
			{
				Config: parentReplacementConfig,
				Check: resource.ComposeTestCheckFunc(
					cloudAcceptanceStateChecks(
						"Terraform Acceptance Project Updated",
						projectKeyB,
						"Terraform Acceptance Environment Updated",
						environmentKeyB,
						"Updated Cloud acceptance description",
					),
					cloudAcceptanceCaptureIDs(
						inventory,
						&replacementProjectID,
						&replacementProjectEnvironmentID,
						projectKeyB,
						environmentKeyB,
						projectID,
						replacementEnvironmentID,
					),
				),
			},
			{
				PreConfig: func() {
					cloudAcceptanceDeleteEnvironment(
						t,
						inventory,
						replacementProjectID,
						replacementProjectEnvironmentID,
					)
				},
				Config: parentReplacementConfig,
				Check: resource.ComposeTestCheckFunc(
					cloudAcceptanceStateChecks(
						"Terraform Acceptance Project Updated",
						projectKeyB,
						"Terraform Acceptance Environment Updated",
						environmentKeyB,
						"Updated Cloud acceptance description",
					),
					cloudAcceptanceCaptureIDs(
						inventory,
						&replacementProjectID,
						&recreatedEnvironmentID,
						projectKeyB,
						environmentKeyB,
						"",
						replacementProjectEnvironmentID,
					),
				),
			},
			{
				PreConfig: func() {
					cloudAcceptanceDeleteProject(t, inventory, replacementProjectID)
				},
				Config: parentReplacementConfig,
				Check: resource.ComposeTestCheckFunc(
					cloudAcceptanceStateChecks(
						"Terraform Acceptance Project Updated",
						projectKeyB,
						"Terraform Acceptance Environment Updated",
						environmentKeyB,
						"Updated Cloud acceptance description",
					),
					cloudAcceptanceCaptureIDs(
						inventory,
						&cascadeProjectID,
						&cascadeEnvironmentID,
						projectKeyB,
						environmentKeyB,
						replacementProjectID,
						recreatedEnvironmentID,
					),
					func(*testingterraform.State) error {
						lifecycleCompleted = true
						return nil
					},
				),
			},
		},
	})
}

func TestCloudAcceptanceInventoryCleansRegisteredAndUnreturnedCreates(t *testing.T) {
	fixture := newProjectProtocolFixture(t)
	defer fixture.close()
	apiURL, err := parseAPIURL(fixture.apiOrigin())
	if err != nil {
		t.Fatal("could not configure the cleanup inventory fixture URL")
	}
	apiClient, err := client.New(apiURL, syntheticProviderAccessToken, client.Options{
		HTTPTimeout:     client.DefaultHTTPTimeout,
		MaxConcurrency:  client.DefaultMaxConcurrency,
		MaxRetries:      0,
		ProviderVersion: "protocol-test",
	})
	if err != nil {
		t.Fatal("could not construct the cleanup inventory fixture client")
	}

	const (
		registeredProjectKey = "cloud-cleanup-registered"
		unreturnedProjectKey = "cloud-cleanup-unreturned"
		environmentKey       = "cloud-cleanup-environment"
	)
	inventory := newCloudAcceptanceInventory(
		apiClient,
		[]string{registeredProjectKey, unreturnedProjectKey},
		[]string{environmentKey},
	)
	ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
	defer cancel()

	registered, err := apiClient.CreateProject(ctx, client.CreateProjectRequest{
		Name: "Registered cleanup Project",
		Key:  registeredProjectKey,
	})
	if err != nil {
		t.Fatal("could not create the registered cleanup fixture Project")
	}
	inventory.registerProject(registered.ID, registeredProjectKey)
	environment, err := apiClient.CreateEnvironment(
		ctx,
		registered.ID,
		client.CreateEnvironmentRequest{
			Name: "Cleanup Environment",
			Key:  environmentKey,
		},
	)
	if err != nil {
		t.Fatal("could not create the cleanup fixture Environment")
	}
	inventory.registerEnvironment(registered.ID, environment.ID, environmentKey)

	// Deliberately omit registration to model a successful Create whose
	// response never reached Terraform state. Exact-key discovery owns cleanup.
	if _, err := apiClient.CreateProject(ctx, client.CreateProjectRequest{
		Name: "Unreturned cleanup Project",
		Key:  unreturnedProjectKey,
	}); err != nil {
		t.Fatal("could not create the unreturned cleanup fixture Project")
	}

	if err := inventory.cleanupAndVerify(ctx); err != nil {
		t.Fatal("cleanup inventory did not remove registered and unreturned objects")
	}
	if fixture.projectCount() != 0 || fixture.environmentCount() != 0 {
		t.Fatal("cleanup inventory fixture did not reach exact zero")
	}
	if len(fixture.violationSnapshot()) != 0 {
		t.Fatal("cleanup inventory fixture observed a request contract violation")
	}
}

func TestCloudAcceptanceProxyProvesSettingsPreservation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			if request.Header.Get("Authorization") != syntheticProviderAccessToken {
				t.Fatal("Cloud acceptance proxy did not forward direct authorization")
			}
			switch request.Method {
			case http.MethodGet:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerEnvironmentExactJSON(
						providerEnvironmentA,
						"Before",
						"staging",
						"Before",
					),
				)
			case http.MethodPut:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					`{"id":"`+providerEnvironmentA+`"}`,
				)
			default:
				t.Fatal("Cloud acceptance proxy fixture received an unexpected method")
			}
		},
	))
	defer upstream.Close()
	target, err := parseAPIURL(upstream.URL)
	if err != nil {
		t.Fatal("could not configure the Cloud proxy fixture target")
	}
	proxy := newCloudAcceptanceProxy(target)
	defer proxy.close()
	proxyURL, err := parseAPIURL(proxy.apiOrigin())
	if err != nil {
		t.Fatal("could not configure the Cloud proxy fixture origin")
	}
	apiClient, err := client.New(proxyURL, syntheticProviderAccessToken, client.Options{
		HTTPTimeout:     client.DefaultHTTPTimeout,
		MaxConcurrency:  client.DefaultMaxConcurrency,
		MaxRetries:      0,
		ProviderVersion: "proxy-test",
	})
	if err != nil {
		t.Fatal("could not construct the Cloud proxy fixture client")
	}
	current, found, err := apiClient.GetEnvironment(
		context.Background(),
		providerProjectID,
		providerEnvironmentA,
	)
	if err != nil || !found {
		t.Fatal("Cloud proxy fixture could not read the exact Environment")
	}
	if err := apiClient.UpdateEnvironment(
		context.Background(),
		providerProjectID,
		providerEnvironmentA,
		current,
		client.UpdateEnvironmentRequest{Name: "After", Description: "After"},
	); err != nil {
		t.Fatal("Cloud proxy fixture could not update the exact Environment")
	}
	if err := proxy.verifySettingsPreservation(1); err != nil {
		t.Fatal("Cloud proxy fixture did not prove settings preservation")
	}
}

func cloudAcceptancePreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("set TF_ACC=1 to run trusted FeatBit Cloud acceptance")
	}
	if value, ok := os.LookupEnv(envAccessToken); !ok || value == "" {
		t.Skip("trusted FeatBit Cloud acceptance requires FEATBIT_ACCESS_TOKEN")
	}
	_ = cloudAcceptanceAPIURL(t)
}

func cloudAcceptanceAPIURL(t *testing.T) *url.URL {
	t.Helper()
	rawURL := defaultCloudAPIURL
	if configured, ok := os.LookupEnv(envAPIURL); ok && configured != "" {
		rawURL = configured
	}
	parsed, err := parseAPIURL(rawURL)
	if err != nil || parsed.Scheme != "https" ||
		!strings.EqualFold(parsed.Hostname(), "app-api.featbit.co") || parsed.Port() != "" {
		t.Skip("trusted Cloud acceptance requires the current app-api.featbit.co endpoint")
	}
	return parsed
}

func cloudAcceptancePrefix(t *testing.T) string {
	t.Helper()
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		t.Fatal("could not generate a unique Cloud acceptance prefix")
	}
	return "tfacc-pe-" + hex.EncodeToString(random)
}

func cloudAcceptanceConfig(
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
  http_timeout_seconds = 30
  max_concurrency      = 2
  max_retries          = 3
}

resource "featbit_project" "cloud_parent" {
  name = %q
  key  = %q
}

resource "featbit_environment" "cloud_child" {
  project_id = featbit_project.cloud_parent.id
  name        = %q
  key         = %q
  description = %q
}

data "featbit_project" "cloud_parent" {
  id         = featbit_project.cloud_parent.id
  depends_on = [featbit_environment.cloud_child]
}

data "featbit_environment" "cloud_child" {
  project_id = featbit_environment.cloud_child.project_id
  id         = featbit_environment.cloud_child.id
}
`, apiOrigin, projectName, projectKey, environmentName, environmentKey, description)
}

func cloudAcceptanceStateChecks(
	projectName string,
	projectKey string,
	environmentName string,
	environmentKey string,
	description string,
) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttr(cloudAcceptanceProjectResourceName, "name", projectName),
		resource.TestCheckResourceAttr(cloudAcceptanceProjectResourceName, "key", projectKey),
		resource.TestCheckResourceAttr(cloudAcceptanceEnvironmentName, "name", environmentName),
		resource.TestCheckResourceAttr(cloudAcceptanceEnvironmentName, "key", environmentKey),
		resource.TestCheckResourceAttr(cloudAcceptanceEnvironmentName, "description", description),
		resource.TestCheckResourceAttr(cloudAcceptanceProjectDataName, "name", projectName),
		resource.TestCheckResourceAttr(cloudAcceptanceProjectDataName, "key", projectKey),
		resource.TestCheckResourceAttr(cloudAcceptanceProjectDataName, "environments.#", "3"),
		resource.TestCheckResourceAttr(cloudAcceptanceEnvironmentDataName, "name", environmentName),
		resource.TestCheckResourceAttr(cloudAcceptanceEnvironmentDataName, "key", environmentKey),
		resource.TestCheckResourceAttr(
			cloudAcceptanceEnvironmentDataName,
			"description",
			description,
		),
		resource.TestCheckResourceAttrPair(
			cloudAcceptanceProjectResourceName,
			"id",
			cloudAcceptanceEnvironmentName,
			"project_id",
		),
		resource.TestCheckResourceAttrPair(
			cloudAcceptanceProjectResourceName,
			"id",
			cloudAcceptanceProjectDataName,
			"id",
		),
		resource.TestCheckResourceAttrPair(
			cloudAcceptanceEnvironmentName,
			"id",
			cloudAcceptanceEnvironmentDataName,
			"id",
		),
	)
}

func cloudAcceptanceCaptureIDs(
	inventory *cloudAcceptanceInventory,
	projectDestination *string,
	environmentDestination *string,
	projectKey string,
	environmentKey string,
	projectMustDifferFrom string,
	environmentMustDifferFrom string,
) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttrWith(
			cloudAcceptanceProjectResourceName,
			"id",
			func(value string) error {
				if !validUUID(value) ||
					(projectMustDifferFrom != "" && value == projectMustDifferFrom) {
					return fmt.Errorf("Cloud Project identity did not satisfy the lifecycle contract")
				}
				*projectDestination = value
				inventory.registerProject(value, projectKey)
				return nil
			},
		),
		resource.TestCheckResourceAttrWith(
			cloudAcceptanceEnvironmentName,
			"id",
			func(value string) error {
				if !validUUID(value) ||
					(environmentMustDifferFrom != "" && value == environmentMustDifferFrom) {
					return fmt.Errorf("Cloud Environment identity did not satisfy the lifecycle contract")
				}
				*environmentDestination = value
				inventory.registerEnvironment(*projectDestination, value, environmentKey)
				return nil
			},
		),
	)
}

func cloudAcceptanceIDsUnchanged(
	projectID *string,
	environmentID *string,
) resource.TestCheckFunc {
	return resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttrWith(
			cloudAcceptanceProjectResourceName,
			"id",
			func(value string) error {
				if value != *projectID {
					return fmt.Errorf("Cloud Project name Update changed its identity")
				}
				return nil
			},
		),
		resource.TestCheckResourceAttrWith(
			cloudAcceptanceEnvironmentName,
			"id",
			func(value string) error {
				if value != *environmentID {
					return fmt.Errorf("Cloud Environment metadata Update changed its identity")
				}
				return nil
			},
		),
	)
}

func cloudAcceptanceProjectImportID(projectID *string) resource.ImportStateIdFunc {
	return func(*testingterraform.State) (string, error) {
		if !validUUID(*projectID) {
			return "", fmt.Errorf("Cloud Project Import identity is unavailable")
		}
		return *projectID, nil
	}
}

func cloudAcceptanceEnvironmentImportID(
	projectID *string,
	environmentID *string,
) resource.ImportStateIdFunc {
	return func(*testingterraform.State) (string, error) {
		if !validUUID(*projectID) || !validUUID(*environmentID) {
			return "", fmt.Errorf("Cloud Environment Import identities are unavailable")
		}
		return *projectID + "/" + *environmentID, nil
	}
}

func cloudAcceptanceDriftProjectAndEnvironment(
	t *testing.T,
	apiClient *client.Client,
	projectID string,
	environmentID string,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
	defer cancel()
	if err := apiClient.UpdateProject(
		ctx,
		projectID,
		client.UpdateProjectRequest{Name: "External Cloud Project Drift"},
	); err != nil {
		t.Fatal("could not prepare external Cloud Project drift")
	}
	current, found, outcome := cloudAcceptanceWaitEnvironment(
		ctx,
		apiClient,
		projectID,
		environmentID,
	)
	if !found {
		t.Fatalf("could not read the Cloud Environment before external drift (%s)", outcome)
	}
	if err := apiClient.UpdateEnvironment(
		ctx,
		projectID,
		environmentID,
		current,
		client.UpdateEnvironmentRequest{
			Name:        "External Cloud Environment Drift",
			Description: "External Cloud description drift",
		},
	); err != nil {
		t.Fatal("could not prepare external Cloud Environment drift")
	}
}

func cloudAcceptanceWaitEnvironment(
	ctx context.Context,
	apiClient *client.Client,
	projectID string,
	environmentID string,
) (client.Environment, bool, string) {
	outcome := "unconfirmed"
	for attempt := 0; attempt < 30; attempt++ {
		environment, found, err := apiClient.GetEnvironment(ctx, projectID, environmentID)
		if err == nil && found {
			return environment, true, "confirmed"
		}
		if err != nil {
			outcome = string(client.Classify(0, nil, err))
		} else {
			outcome = "exact_zero"
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return client.Environment{}, false, outcome
		case <-timer.C:
		}
	}
	return client.Environment{}, false, outcome
}

func cloudAcceptanceDeleteEnvironment(
	t *testing.T,
	inventory *cloudAcceptanceInventory,
	projectID string,
	environmentID string,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
	defer cancel()
	if err := inventory.deleteEnvironment(ctx, projectID, environmentID); err != nil {
		t.Fatal("could not prepare out-of-band Cloud Environment deletion")
	}
}

func cloudAcceptanceDeleteProject(
	t *testing.T,
	inventory *cloudAcceptanceInventory,
	projectID string,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
	defer cancel()
	if err := inventory.deleteProject(ctx, projectID); err != nil {
		t.Fatal("could not prepare out-of-band Cloud Project cascade deletion")
	}
}

type cloudAcceptanceProjectRecord struct {
	key string
}

type cloudAcceptanceEnvironmentRecord struct {
	projectID string
	key       string
}

type cloudAcceptanceInventory struct {
	api *client.Client

	mu              sync.Mutex
	projectKeys     map[string]struct{}
	environmentKeys map[string]struct{}
	projects        map[string]cloudAcceptanceProjectRecord
	environments    map[string]cloudAcceptanceEnvironmentRecord
}

func newCloudAcceptanceInventory(
	apiClient *client.Client,
	projectKeys []string,
	environmentKeys []string,
) *cloudAcceptanceInventory {
	inventory := &cloudAcceptanceInventory{
		api:             apiClient,
		projectKeys:     make(map[string]struct{}, len(projectKeys)),
		environmentKeys: make(map[string]struct{}, len(environmentKeys)),
		projects:        make(map[string]cloudAcceptanceProjectRecord),
		environments:    make(map[string]cloudAcceptanceEnvironmentRecord),
	}
	for _, key := range projectKeys {
		inventory.projectKeys[key] = struct{}{}
	}
	for _, key := range environmentKeys {
		inventory.environmentKeys[key] = struct{}{}
	}
	return inventory
}

func (i *cloudAcceptanceInventory) registerProject(projectID string, key string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.projects[projectID] = cloudAcceptanceProjectRecord{key: key}
}

func (i *cloudAcceptanceInventory) registerEnvironment(
	projectID string,
	environmentID string,
	key string,
) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.environments[environmentID] = cloudAcceptanceEnvironmentRecord{
		projectID: projectID,
		key:       key,
	}
}

func (i *cloudAcceptanceInventory) deleteEnvironment(
	ctx context.Context,
	projectID string,
	environmentID string,
) error {
	deleteErr := i.api.DeleteEnvironment(ctx, projectID, environmentID)
	_, found, readErr := i.api.GetEnvironment(ctx, projectID, environmentID)
	if readErr != nil || found {
		if deleteErr != nil {
			return fmt.Errorf("Cloud Environment delete and absence proof failed")
		}
		return fmt.Errorf("Cloud Environment absence proof failed")
	}
	i.mu.Lock()
	delete(i.environments, environmentID)
	i.mu.Unlock()
	return nil
}

func (i *cloudAcceptanceInventory) deleteProject(
	ctx context.Context,
	projectID string,
) error {
	deleteErr := i.api.DeleteProject(ctx, projectID)
	_, found, readErr := i.api.GetProject(ctx, projectID)
	if readErr != nil || found {
		if deleteErr != nil {
			return fmt.Errorf("Cloud Project delete and absence proof failed")
		}
		return fmt.Errorf("Cloud Project absence proof failed")
	}
	i.mu.Lock()
	delete(i.projects, projectID)
	for environmentID, environment := range i.environments {
		if environment.projectID == projectID {
			delete(i.environments, environmentID)
		}
	}
	i.mu.Unlock()
	return nil
}

func (i *cloudAcceptanceInventory) cleanupAndVerify(ctx context.Context) error {
	i.mu.Lock()
	environments := make(map[string]cloudAcceptanceEnvironmentRecord, len(i.environments))
	for environmentID, environment := range i.environments {
		environments[environmentID] = environment
	}
	projects := make([]string, 0, len(i.projects))
	for projectID := range i.projects {
		projects = append(projects, projectID)
	}
	i.mu.Unlock()

	cleanupFailures := 0
	for environmentID, environment := range environments {
		if err := i.deleteEnvironment(ctx, environment.projectID, environmentID); err != nil {
			cleanupFailures++
		}
	}
	for _, projectID := range projects {
		if err := i.deleteProject(ctx, projectID); err != nil {
			cleanupFailures++
		}
	}

	// A create can succeed before Terraform returns state. Discover and clean
	// any such object only by the unique exact test keys registered up front.
	collection, err := i.api.ListProjects(ctx)
	if err != nil {
		return fmt.Errorf("Cloud cleanup collection verification failed")
	}
	for _, project := range collection {
		if _, tracked := i.projectKeys[project.Key]; !tracked {
			continue
		}
		i.registerProject(project.ID, project.Key)
		if err := i.deleteProject(ctx, project.ID); err != nil {
			cleanupFailures++
		}
	}

	remaining, err := i.api.ListProjects(ctx)
	if err != nil {
		return fmt.Errorf("Cloud final collection verification failed")
	}
	for _, project := range remaining {
		if _, tracked := i.projectKeys[project.Key]; !tracked {
			continue
		}
		cleanupFailures++
		for _, environment := range project.Environments {
			if _, tracked := i.environmentKeys[environment.Key]; tracked {
				cleanupFailures++
			}
		}
	}

	i.mu.Lock()
	pending := len(i.projects) + len(i.environments)
	i.mu.Unlock()
	if cleanupFailures != 0 || pending != 0 {
		return fmt.Errorf("Cloud cleanup retained an exact test object or pending owner")
	}
	return nil
}

func TestCloudAcceptanceInventoryScopesEnvironmentKeysToOwnedProjects(t *testing.T) {
	fixture := newProjectProtocolFixture(t)
	defer fixture.close()
	apiURL, err := parseAPIURL(fixture.apiOrigin())
	if err != nil {
		t.Fatal("could not configure the scoped cleanup fixture URL")
	}
	apiClient, err := client.New(apiURL, syntheticProviderAccessToken, client.Options{
		HTTPTimeout:     client.DefaultHTTPTimeout,
		MaxConcurrency:  client.DefaultMaxConcurrency,
		MaxRetries:      0,
		ProviderVersion: "protocol-test",
	})
	if err != nil {
		t.Fatal("could not construct the scoped cleanup fixture client")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cloudAcceptanceTimeout)
	defer cancel()
	unrelated, err := apiClient.CreateProject(ctx, client.CreateProjectRequest{
		Name: "Unrelated Project",
		Key:  "unrelated-project",
	})
	if err != nil {
		t.Fatal("could not create the unrelated cleanup fixture Project")
	}
	t.Cleanup(func() {
		_ = apiClient.DeleteProject(context.Background(), unrelated.ID)
	})

	inventory := newCloudAcceptanceInventory(
		apiClient,
		[]string{"owned-project-that-was-never-created"},
		[]string{"dev", "prod"},
	)
	if err := inventory.cleanupAndVerify(ctx); err != nil {
		t.Fatal("cleanup inventory treated a common Environment key outside its Project as owned")
	}
	if _, found, err := apiClient.GetProject(ctx, unrelated.ID); err != nil || !found {
		t.Fatal("cleanup inventory changed an unrelated Project")
	}
}

type cloudSettingsLeaf struct {
	kind   string
	digest [sha256.Size]byte
}

type cloudSettingsFingerprint map[string]cloudSettingsLeaf

type cloudAcceptanceProxy struct {
	target     url.URL
	server     *httptest.Server
	httpClient http.Client

	mu                        sync.Mutex
	settingsByEnvironmentPath map[string]cloudSettingsFingerprint
	preservedUpdates          int
	violations                int
	putRequests               int
	environmentPutRequests    int
	exactEnvironmentPuts      int
	postRequests              int
	environmentPostRequests   int
	deleteRequests            int
	environmentDeleteRequests int
}

func newCloudAcceptanceProxy(target *url.URL) *cloudAcceptanceProxy {
	proxy := &cloudAcceptanceProxy{
		target: *target,
		httpClient: http.Client{
			Timeout: client.DefaultHTTPTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		settingsByEnvironmentPath: make(map[string]cloudSettingsFingerprint),
	}
	proxy.server = httptest.NewServer(http.HandlerFunc(proxy.handle))
	return proxy
}

func (p *cloudAcceptanceProxy) apiOrigin() string {
	return p.server.URL
}

func (p *cloudAcceptanceProxy) close() {
	p.server.Close()
}

func (p *cloudAcceptanceProxy) handle(response http.ResponseWriter, request *http.Request) {
	if request == nil || request.URL == nil ||
		!strings.HasPrefix(request.URL.EscapedPath(), "/api/v1/") {
		p.recordViolation()
		writeCloudAcceptanceProxyFailure(response)
		return
	}
	requestBody, err := readCloudAcceptanceProxyBody(request.Body)
	if err != nil {
		p.recordViolation()
		writeCloudAcceptanceProxyFailure(response)
		return
	}
	path := request.URL.EscapedPath()
	if request.Method == http.MethodPost || request.Method == http.MethodDelete {
		p.mu.Lock()
		if request.Method == http.MethodPost {
			p.postRequests++
			if strings.HasSuffix(path, "/envs") {
				p.environmentPostRequests++
			}
		} else {
			p.deleteRequests++
			if strings.Contains(path, "/envs/") {
				p.environmentDeleteRequests++
			}
		}
		p.mu.Unlock()
	}
	if request.Method == http.MethodPut {
		p.mu.Lock()
		p.putRequests++
		if strings.Contains(path, "/envs/") {
			p.environmentPutRequests++
		}
		if cloudAcceptanceEnvironmentExactPath(path) {
			p.exactEnvironmentPuts++
		}
		p.mu.Unlock()
	}
	if request.Method == http.MethodPut && cloudAcceptanceEnvironmentExactPath(path) {
		p.observeEnvironmentUpdate(path, requestBody)
	}

	target := p.target
	target.Path = request.URL.Path
	target.RawPath = request.URL.RawPath
	target.RawQuery = request.URL.RawQuery
	outbound, err := http.NewRequestWithContext(
		request.Context(),
		request.Method,
		target.String(),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		p.recordViolation()
		writeCloudAcceptanceProxyFailure(response)
		return
	}
	outbound.Header = request.Header.Clone()
	outbound.Host = target.Host
	upstream, err := p.httpClient.Do(outbound)
	if err != nil {
		writeCloudAcceptanceProxyFailure(response)
		return
	}
	upstreamBody, readErr := readCloudAcceptanceProxyBody(upstream.Body)
	_ = upstream.Body.Close()
	if readErr != nil {
		p.recordViolation()
		writeCloudAcceptanceProxyFailure(response)
		return
	}
	if request.Method == http.MethodGet && cloudAcceptanceEnvironmentExactPath(path) &&
		upstream.StatusCode >= http.StatusOK && upstream.StatusCode < http.StatusMultipleChoices {
		p.observeEnvironmentRead(path, upstreamBody)
	}
	for name, values := range upstream.Header {
		for _, value := range values {
			response.Header().Add(name, value)
		}
	}
	response.WriteHeader(upstream.StatusCode)
	_, _ = response.Write(upstreamBody)
}

func readCloudAcceptanceProxyBody(body io.ReadCloser) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	content, err := io.ReadAll(io.LimitReader(body, client.MaxResponseBytes+1))
	if err != nil || int64(len(content)) > client.MaxResponseBytes {
		return nil, fmt.Errorf("Cloud acceptance proxy body boundary failed")
	}
	return content, nil
}

func cloudAcceptanceEnvironmentExactPath(path string) bool {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	return len(segments) == 6 && segments[0] == "api" && segments[1] == "v1" &&
		segments[2] == "projects" && validUUID(segments[3]) && segments[4] == "envs" &&
		validUUID(segments[5])
}

func (p *cloudAcceptanceProxy) observeEnvironmentRead(path string, body []byte) {
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Settings json.RawMessage `json:"settings"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &envelope) != nil || !envelope.Success {
		p.recordViolation()
		return
	}
	fingerprint, err := fingerprintCloudSettings(envelope.Data.Settings)
	if err != nil {
		// A normal refresh may succeed without returning settings. It is
		// relevant only if a subsequent PUT tries to use it; in that case the
		// missing baseline is rejected by observeEnvironmentUpdate.
		return
	}
	p.mu.Lock()
	p.settingsByEnvironmentPath[path] = fingerprint
	p.mu.Unlock()
}

func (p *cloudAcceptanceProxy) observeEnvironmentUpdate(path string, body []byte) {
	var payload struct {
		Settings json.RawMessage `json:"settings"`
	}
	if json.Unmarshal(body, &payload) != nil {
		p.recordViolation()
		return
	}
	fingerprint, err := fingerprintCloudSettings(payload.Settings)
	if err != nil {
		p.recordViolation()
		return
	}
	p.mu.Lock()
	baseline, found := p.settingsByEnvironmentPath[path]
	if !found || cloudSettingsFingerprintDifference(baseline, fingerprint) != "" {
		p.violations++
	} else {
		p.preservedUpdates++
	}
	p.mu.Unlock()
}

func (p *cloudAcceptanceProxy) recordViolation() {
	p.mu.Lock()
	p.violations++
	p.mu.Unlock()
}

func (p *cloudAcceptanceProxy) verifySettingsPreservation(want int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.violations != 0 || p.preservedUpdates != want {
		return fmt.Errorf(
			"Cloud acceptance settings proof count mismatch (preserved=%d, violations=%d, puts=%d, environment_puts=%d, exact_environment_puts=%d, posts=%d, environment_posts=%d, deletes=%d, environment_deletes=%d)",
			p.preservedUpdates,
			p.violations,
			p.putRequests,
			p.environmentPutRequests,
			p.exactEnvironmentPuts,
			p.postRequests,
			p.environmentPostRequests,
			p.deleteRequests,
			p.environmentDeleteRequests,
		)
	}
	return nil
}

func writeCloudAcceptanceProxyFailure(response http.ResponseWriter) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusBadGateway)
	_, _ = response.Write([]byte(`{"success":false,"data":null,"errors":[]}`))
}

func fingerprintCloudSettings(settings json.RawMessage) (cloudSettingsFingerprint, error) {
	settings = bytes.TrimSpace(settings)
	if len(settings) < 2 || settings[0] != '{' || settings[len(settings)-1] != '}' {
		return nil, fmt.Errorf("Cloud settings snapshot was not an object")
	}
	var normalized any
	if json.Unmarshal(settings, &normalized) != nil {
		return nil, fmt.Errorf("Cloud settings snapshot was invalid")
	}
	fingerprint := make(cloudSettingsFingerprint)
	if err := addCloudSettingsFingerprint(fingerprint, "$", normalized); err != nil {
		return nil, fmt.Errorf("Cloud settings snapshot could not be fingerprinted")
	}
	return fingerprint, nil
}

func addCloudSettingsFingerprint(
	fingerprint cloudSettingsFingerprint,
	path string,
	value any,
) error {
	leaf := cloudSettingsLeaf{kind: fmt.Sprintf("%T", value)}
	switch typed := value.(type) {
	case map[string]any:
		fingerprint[path] = leaf
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			escaped := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
			if err := addCloudSettingsFingerprint(fingerprint, path+"/"+escaped, typed[key]); err != nil {
				return err
			}
		}
		return nil
	case []any:
		fingerprint[path] = leaf
		for index, item := range typed {
			if err := addCloudSettingsFingerprint(
				fingerprint,
				fmt.Sprintf("%s/%d", path, index),
				item,
			); err != nil {
				return err
			}
		}
		return nil
	default:
		canonical, err := json.Marshal(value)
		if err != nil {
			return err
		}
		leaf.digest = sha256.Sum256(canonical)
		fingerprint[path] = leaf
		return nil
	}
}

func cloudSettingsFingerprintDifference(
	baseline cloudSettingsFingerprint,
	current cloudSettingsFingerprint,
) string {
	paths := make([]string, 0)
	for path, before := range baseline {
		after, found := current[path]
		if !found {
			paths = append(paths, "removed "+path)
			continue
		}
		if before != after {
			paths = append(paths, "changed "+path)
		}
	}
	for path := range current {
		if _, found := baseline[path]; !found {
			paths = append(paths, "added "+path)
		}
	}
	sort.Strings(paths)
	if len(paths) > 8 {
		paths = append(paths[:8], "additional paths omitted")
	}
	return strings.Join(paths, ", ")
}
