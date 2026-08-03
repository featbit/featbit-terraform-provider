// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const providerEnvironmentSecretMarker = "test-only-resource-environment-secret-marker"

func TestEnvironmentResourceMetadataSchemaAndConfigure(t *testing.T) {
	t.Parallel()

	resourceUnderTest := &environmentResource{}
	var metadataResponse frameworkresource.MetadataResponse
	resourceUnderTest.Metadata(
		context.Background(),
		frameworkresource.MetadataRequest{ProviderTypeName: "featbit"},
		&metadataResponse,
	)
	if metadataResponse.TypeName != "featbit_environment" {
		t.Fatalf("type name = %q", metadataResponse.TypeName)
	}

	environmentSchema := environmentResourceTestSchema(t)
	if got := len(environmentSchema.Attributes); got != 5 {
		t.Fatalf("attribute count = %d, want 5", got)
	}
	idAttribute, ok := environmentSchema.Attributes["id"].(resourceschema.StringAttribute)
	if !ok || !idAttribute.Computed || idAttribute.Required || idAttribute.Optional ||
		len(idAttribute.PlanModifiers) != 1 {
		t.Fatalf("id attribute = %#v", environmentSchema.Attributes["id"])
	}
	var stableIDResponse planmodifier.StringResponse
	stableIDResponse.PlanValue = types.StringUnknown()
	idAttribute.PlanModifiers[0].PlanModifyString(
		context.Background(),
		planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			PlanValue:   types.StringUnknown(),
			StateValue:  types.StringValue(providerEnvironmentA),
			Plan: environmentResourceTestPlan(
				t,
				environmentSchema,
				providerProjectID,
				"Staging Updated",
				"staging",
				"Updated",
			),
			State: environmentResourceTestState(
				t,
				environmentSchema,
				providerProjectID,
				providerEnvironmentA,
				"Staging",
				"staging",
				"",
			),
		},
		&stableIDResponse,
	)
	if stableIDResponse.Diagnostics.HasError() ||
		!stableIDResponse.PlanValue.Equal(types.StringValue(providerEnvironmentA)) {
		t.Fatalf("id in-place plan modifier response = %#v", stableIDResponse)
	}
	var replacementIDResponse planmodifier.StringResponse
	replacementIDResponse.PlanValue = types.StringUnknown()
	idAttribute.PlanModifiers[0].PlanModifyString(
		context.Background(),
		planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			PlanValue:   types.StringUnknown(),
			StateValue:  types.StringValue(providerEnvironmentA),
			Plan: environmentResourceTestPlan(
				t,
				environmentSchema,
				providerProjectID,
				"Staging",
				"replacement-key",
				"",
			),
			State: environmentResourceTestState(
				t,
				environmentSchema,
				providerProjectID,
				providerEnvironmentA,
				"Staging",
				"staging",
				"",
			),
		},
		&replacementIDResponse,
	)
	if replacementIDResponse.Diagnostics.HasError() || !replacementIDResponse.PlanValue.IsUnknown() {
		t.Fatalf("id replacement plan modifier response = %#v", replacementIDResponse)
	}
	nameAttribute, ok := environmentSchema.Attributes["name"].(resourceschema.StringAttribute)
	if !ok || !nameAttribute.Required || nameAttribute.Computed || nameAttribute.Optional {
		t.Fatalf("name attribute = %#v", environmentSchema.Attributes["name"])
	}
	for _, name := range []string{"project_id", "key"} {
		attribute, ok := environmentSchema.Attributes[name].(resourceschema.StringAttribute)
		if !ok || !attribute.Required || attribute.Computed || attribute.Optional ||
			len(attribute.PlanModifiers) != 1 {
			t.Fatalf("%s attribute = %#v", name, environmentSchema.Attributes[name])
		}
		if name == "project_id" && len(attribute.Validators) != 1 {
			t.Fatalf("project_id validator count = %d, want 1", len(attribute.Validators))
		}
		var modifierResponse planmodifier.StringResponse
		attribute.PlanModifiers[0].PlanModifyString(
			context.Background(),
			planmodifier.StringRequest{
				ConfigValue: types.StringValue("new"),
				PlanValue:   types.StringValue("new"),
				StateValue:  types.StringValue("old"),
				Plan: environmentResourceTestPlan(
					t,
					environmentSchema,
					providerProjectID,
					"Staging",
					"staging",
					"",
				),
				State: environmentResourceTestState(
					t,
					environmentSchema,
					providerProjectID,
					providerEnvironmentA,
					"Staging",
					"staging",
					"",
				),
			},
			&modifierResponse,
		)
		if !modifierResponse.RequiresReplace {
			t.Fatalf("%s plan modifier %T did not require replacement", name, attribute.PlanModifiers[0])
		}
	}
	descriptionAttribute, ok := environmentSchema.Attributes["description"].(resourceschema.StringAttribute)
	if !ok || !descriptionAttribute.Optional || !descriptionAttribute.Computed ||
		descriptionAttribute.Required || descriptionAttribute.Default == nil {
		t.Fatalf("description attribute = %#v", environmentSchema.Attributes["description"])
	}

	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {
			t.Fatal("Configure() executed an HTTP request")
		},
	))
	defer closeServer()
	var configureResponse frameworkresource.ConfigureResponse
	resourceUnderTest.Configure(
		context.Background(),
		frameworkresource.ConfigureRequest{ProviderData: apiClient},
		&configureResponse,
	)
	if configureResponse.Diagnostics.HasError() || resourceUnderTest.client != apiClient {
		t.Fatalf("Configure() diagnostics = %v", configureResponse.Diagnostics)
	}

	var wrongTypeResponse frameworkresource.ConfigureResponse
	resourceUnderTest.Configure(
		context.Background(),
		frameworkresource.ConfigureRequest{ProviderData: struct{}{}},
		&wrongTypeResponse,
	)
	if !wrongTypeResponse.Diagnostics.HasError() {
		t.Fatal("Configure() accepted an unexpected provider data type")
	}
}

func TestEnvironmentResourceCreatePreflightAndCanonicalRead(t *testing.T) {
	t.Parallel()

	const environmentKey = "staging"
	var requests []string
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			requests = append(requests, request.Method+" "+request.URL.EscapedPath())
			switch len(requests) {
			case 1:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerEnvironmentParentJSON(
						providerProjectID,
						`[{"id":"`+providerEnvironmentB+`","name":"Fuzzy",`+
							`"key":"staging-extra","description":""}]`,
					),
				)
			case 2:
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatalf("read create body: %v", err)
				}
				if got := string(body); got != `{"name":"Staging","key":"staging","description":"QA"}` {
					t.Fatalf("create body = %s", got)
				}
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerEnvironmentVMJSON(providerEnvironmentA, "Staging", "QA"),
				)
			case 3:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerEnvironmentExactJSON(
						providerEnvironmentA,
						"Staging",
						environmentKey,
						"QA",
					),
				)
			default:
				t.Fatalf("unexpected request %s %s", request.Method, request.URL.EscapedPath())
			}
		},
	))
	defer closeServer()

	environmentSchema := environmentResourceTestSchema(t)
	response := frameworkresource.CreateResponse{State: tfsdk.State{Schema: environmentSchema}}
	(&environmentResource{client: apiClient}).Create(
		context.Background(),
		frameworkresource.CreateRequest{
			Plan: environmentResourceTestPlan(
				t,
				environmentSchema,
				providerProjectID,
				"Staging",
				environmentKey,
				"QA",
			),
		},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Create() diagnostics = %v", response.Diagnostics)
	}
	state := environmentResourceStateModel(t, response.State)
	if state.ProjectID.ValueString() != providerProjectID ||
		state.ID.ValueString() != providerEnvironmentA ||
		state.Key.ValueString() != environmentKey || state.Description.ValueString() != "QA" {
		t.Fatalf("Create() state = %#v", state)
	}
	wantRequests := []string{
		"GET /api/v1/projects/" + providerProjectID,
		"POST /api/v1/projects/" + providerProjectID + "/envs",
		"GET /api/v1/projects/" + providerProjectID + "/envs/" + providerEnvironmentA,
	}
	if fmt.Sprint(requests) != fmt.Sprint(wantRequests) {
		t.Fatalf("requests = %v, want %v", requests, wantRequests)
	}
}

func TestEnvironmentResourceCreateRejectsMissingParentAndExactDuplicate(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		handler http.Handler
		wantReq int
	}{
		"missing parent": {
			wantReq: 2,
			handler: http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodGet {
					t.Fatal("missing parent preflight reached a mutation")
				}
				if request.URL.EscapedPath() == "/api/v1/projects" {
					writeProjectResourceEnvelope(t, response, http.StatusOK, `[]`)
					return
				}
				writeProjectResourceEnvelope(t, response, http.StatusNotFound, `null`)
			}),
		},
		"duplicate exact keys": {
			wantReq: 1,
			handler: http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodGet {
					t.Fatal("duplicate preflight reached a mutation")
				}
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerEnvironmentParentJSON(
						providerProjectID,
						`[{"id":"`+providerEnvironmentA+`","name":"One","key":"staging"},`+
							`{"id":"`+providerEnvironmentB+`","name":"Two","key":"staging"}]`,
					),
				)
			}),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			requestCount := 0
			apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
				func(response http.ResponseWriter, request *http.Request) {
					requestCount++
					test.handler.ServeHTTP(response, request)
				},
			))
			defer closeServer()

			environmentSchema := environmentResourceTestSchema(t)
			response := frameworkresource.CreateResponse{State: tfsdk.State{Schema: environmentSchema}}
			(&environmentResource{client: apiClient}).Create(
				context.Background(),
				frameworkresource.CreateRequest{
					Plan: environmentResourceTestPlan(
						t,
						environmentSchema,
						providerProjectID,
						"Staging",
						"staging",
						"",
					),
				},
				&response,
			)
			if !response.Diagnostics.HasError() || requestCount != test.wantReq {
				t.Fatalf("Create() diagnostics = %v, requests = %d", response.Diagnostics, requestCount)
			}
		})
	}
}

func TestEnvironmentResourceAmbiguousCreateReconcilesWithoutRetryOrAdoption(t *testing.T) {
	t.Parallel()

	const environmentKey = "recovery-environment-key"
	requestCount := 0
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, _ *http.Request) {
			requestCount++
			switch requestCount {
			case 1:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerEnvironmentParentJSON(providerProjectID, `[]`),
				)
			case 2:
				writeProjectResourceEnvelope(t, response, http.StatusInternalServerError, `null`)
			case 3:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerEnvironmentParentJSON(
						providerProjectID,
						`[{"id":"`+providerEnvironmentA+`","name":"Recovered",`+
							`"key":"`+environmentKey+`","description":""}]`,
					),
				)
			default:
				t.Fatal("ambiguous Create retried or made an unexpected request")
			}
		},
	))
	defer closeServer()

	environmentSchema := environmentResourceTestSchema(t)
	response := frameworkresource.CreateResponse{State: tfsdk.State{Schema: environmentSchema}}
	(&environmentResource{client: apiClient}).Create(
		context.Background(),
		frameworkresource.CreateRequest{
			Plan: environmentResourceTestPlan(
				t,
				environmentSchema,
				providerProjectID,
				"Staging",
				environmentKey,
				"",
			),
		},
		&response,
	)
	if !response.Diagnostics.HasError() || requestCount != 3 {
		t.Fatalf("Create() diagnostics = %v, requests = %d", response.Diagnostics, requestCount)
	}
	if !diagnosticsContain(response.Diagnostics, "import") {
		t.Fatal("ambiguous Create diagnostic omitted stable Import recovery guidance")
	}
	if diagnosticsContain(response.Diagnostics, environmentKey) ||
		diagnosticsContain(response.Diagnostics, providerEnvironmentA) ||
		diagnosticsContain(response.Diagnostics, providerProjectID) {
		t.Fatal("ambiguous Create diagnostic disclosed a runtime key or UUID")
	}
}

func TestEnvironmentResourceUpdatePreservesSettingsAndReadsCanonicalState(t *testing.T) {
	t.Parallel()

	requestCount := 0
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			requestCount++
			switch requestCount {
			case 1:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerEnvironmentExactJSON(providerEnvironmentA, "Original", "staging", "Before"),
				)
			case 2:
				if request.Method != http.MethodPut {
					t.Fatalf("update method = %s", request.Method)
				}
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatalf("read update body: %v", err)
				}
				want := `{"name":"Renamed","description":"After",` +
					`"settings":{"requireChangeComment":true,"future":{"mode":"keep"}}}`
				if got := string(body); got != want {
					t.Fatalf("update body = %s, want %s", got, want)
				}
				if strings.Contains(string(body), providerEnvironmentSecretMarker) ||
					strings.Contains(string(body), "secrets") {
					t.Fatal("Update() round-tripped a generated Environment secret")
				}
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerEnvironmentVMJSON(providerEnvironmentA, "Renamed", "After"),
				)
			case 3:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerEnvironmentExactJSON(providerEnvironmentA, "Renamed", "staging", "After"),
				)
			default:
				t.Fatal("Update() made an unexpected request")
			}
		},
	))
	defer closeServer()

	environmentSchema := environmentResourceTestSchema(t)
	priorState := environmentResourceTestState(
		t,
		environmentSchema,
		providerProjectID,
		providerEnvironmentA,
		"Original",
		"staging",
		"Before",
	)
	plan := environmentResourceTestPlan(
		t,
		environmentSchema,
		providerProjectID,
		"Renamed",
		"staging",
		"After",
	)
	response := frameworkresource.UpdateResponse{State: priorState}
	(&environmentResource{client: apiClient}).Update(
		context.Background(),
		frameworkresource.UpdateRequest{Plan: plan, State: priorState},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Update() diagnostics = %v", response.Diagnostics)
	}
	state := environmentResourceStateModel(t, response.State)
	if state.Name.ValueString() != "Renamed" || state.Description.ValueString() != "After" ||
		state.Key.ValueString() != "staging" {
		t.Fatalf("Update() state = %#v", state)
	}
	if requestCount != 3 {
		t.Fatalf("Update() requests = %d, want 3", requestCount)
	}
}

func TestEnvironmentResourceUpdateFailurePreservesStateAndLaterProgresses(t *testing.T) {
	t.Parallel()

	requestCount := 0
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, _ *http.Request) {
			requestCount++
			switch requestCount {
			case 1, 3:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerEnvironmentExactJSON(providerEnvironmentA, "Original", "staging", "Before"),
				)
			case 2:
				writeProjectResourceEnvelope(t, response, http.StatusInternalServerError, `null`)
			case 4:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerEnvironmentVMJSON(providerEnvironmentA, "Renamed", "After"),
				)
			case 5:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerEnvironmentExactJSON(providerEnvironmentA, "Renamed", "staging", "After"),
				)
			default:
				t.Fatal("Update() made an unexpected request")
			}
		},
	))
	defer closeServer()

	environmentSchema := environmentResourceTestSchema(t)
	priorState := environmentResourceTestState(
		t,
		environmentSchema,
		providerProjectID,
		providerEnvironmentA,
		"Original",
		"staging",
		"Before",
	)
	plan := environmentResourceTestPlan(
		t,
		environmentSchema,
		providerProjectID,
		"Renamed",
		"staging",
		"After",
	)
	resourceUnderTest := &environmentResource{client: apiClient}
	failed := frameworkresource.UpdateResponse{State: priorState}
	resourceUnderTest.Update(
		context.Background(),
		frameworkresource.UpdateRequest{Plan: plan, State: priorState},
		&failed,
	)
	if !failed.Diagnostics.HasError() || !failed.State.Raw.Equal(priorState.Raw) {
		t.Fatalf("failed Update() diagnostics = %v", failed.Diagnostics)
	}

	progress := frameworkresource.UpdateResponse{State: priorState}
	resourceUnderTest.Update(
		context.Background(),
		frameworkresource.UpdateRequest{Plan: plan, State: priorState},
		&progress,
	)
	if progress.Diagnostics.HasError() || requestCount != 5 {
		t.Fatalf("progress Update() diagnostics = %v, requests = %d", progress.Diagnostics, requestCount)
	}
}

func TestEnvironmentResourceCancellationWhileWaitingForLockPreservesState(t *testing.T) {
	t.Parallel()

	requestCount := 0
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, _ *http.Request) {
			requestCount++
			switch requestCount {
			case 1:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerEnvironmentExactJSON(providerEnvironmentA, "Original", "staging", "Before"),
				)
			case 2:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerEnvironmentVMJSON(providerEnvironmentA, "Renamed", "After"),
				)
			case 3:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerEnvironmentExactJSON(providerEnvironmentA, "Renamed", "staging", "After"),
				)
			default:
				t.Fatal("Update() made an unexpected request")
			}
		},
	))
	defer closeServer()

	environmentSchema := environmentResourceTestSchema(t)
	priorState := environmentResourceTestState(
		t,
		environmentSchema,
		providerProjectID,
		providerEnvironmentA,
		"Original",
		"staging",
		"Before",
	)
	plan := environmentResourceTestPlan(
		t,
		environmentSchema,
		providerProjectID,
		"Renamed",
		"staging",
		"After",
	)
	resourceUnderTest := &environmentResource{client: apiClient}
	manager := resourceUnderTest.environmentLocks()
	release, err := manager.acquire(context.Background(), providerProjectID, providerEnvironmentA)
	if err != nil {
		t.Fatalf("occupy Environment lock: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan frameworkresource.UpdateResponse, 1)
	go func() {
		response := frameworkresource.UpdateResponse{State: priorState}
		resourceUnderTest.Update(
			ctx,
			frameworkresource.UpdateRequest{Plan: plan, State: priorState},
			&response,
		)
		result <- response
	}()
	waitForEnvironmentLockUsers(t, manager, providerProjectID, providerEnvironmentA, 2)
	cancel()

	var canceled frameworkresource.UpdateResponse
	select {
	case canceled = <-result:
	case <-time.After(2 * time.Second):
		t.Fatal("Update() did not return after lock-wait cancellation")
	}
	if !canceled.Diagnostics.HasError() || !canceled.State.Raw.Equal(priorState.Raw) || requestCount != 0 {
		t.Fatalf("canceled Update() diagnostics = %v, requests = %d", canceled.Diagnostics, requestCount)
	}
	release()

	progress := frameworkresource.UpdateResponse{State: priorState}
	resourceUnderTest.Update(
		context.Background(),
		frameworkresource.UpdateRequest{Plan: plan, State: priorState},
		&progress,
	)
	if progress.Diagnostics.HasError() || requestCount != 3 {
		t.Fatalf("progress Update() diagnostics = %v, requests = %d", progress.Diagnostics, requestCount)
	}
}

func TestEnvironmentLockManagerScopesByExactEnvironmentAndCleansUp(t *testing.T) {
	t.Parallel()

	manager := newEnvironmentLockManager()
	releaseA, err := manager.acquire(context.Background(), providerProjectID, providerEnvironmentA)
	if err != nil {
		t.Fatalf("acquire first Environment lock: %v", err)
	}
	releaseB, err := manager.acquire(context.Background(), providerProjectID, providerEnvironmentB)
	if err != nil {
		t.Fatalf("a different Environment was blocked by the first lock: %v", err)
	}
	releaseB()
	releaseA()

	manager.mu.Lock()
	remaining := len(manager.locks)
	manager.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("Environment lock manager retained %d unused locks", remaining)
	}
}

func TestEnvironmentResourceDeleteUsesExactParentFallback(t *testing.T) {
	t.Parallel()

	tests := map[string]int{
		"direct forbidden after successful delete": http.StatusOK,
		"already absent": http.StatusNotFound,
	}
	for name, deleteStatus := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var requests []string
			apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
				func(response http.ResponseWriter, request *http.Request) {
					requests = append(requests, request.Method+" "+request.URL.EscapedPath())
					switch len(requests) {
					case 1:
						data := `true`
						if deleteStatus != http.StatusOK {
							data = `null`
						}
						writeProjectResourceEnvelope(t, response, deleteStatus, data)
					case 2:
						writeProjectResourceEnvelope(t, response, http.StatusForbidden, `null`)
					case 3:
						writeProjectResourceEnvelope(
							t,
							response,
							http.StatusOK,
							providerEnvironmentParentJSON(providerProjectID, `[]`),
						)
					default:
						t.Fatal("Delete() made an unexpected request")
					}
				},
			))
			defer closeServer()

			environmentSchema := environmentResourceTestSchema(t)
			priorState := environmentResourceTestState(
				t,
				environmentSchema,
				providerProjectID,
				providerEnvironmentA,
				"Staging",
				"staging",
				"",
			)
			response := frameworkresource.DeleteResponse{State: priorState}
			(&environmentResource{client: apiClient}).Delete(
				context.Background(),
				frameworkresource.DeleteRequest{State: priorState},
				&response,
			)
			if response.Diagnostics.HasError() {
				t.Fatalf("Delete() diagnostics = %v", response.Diagnostics)
			}
			if !response.State.Raw.IsNull() {
				t.Fatal("Delete() did not remove authoritatively absent Environment state")
			}
			wantRequests := []string{
				"DELETE /api/v1/projects/" + providerProjectID + "/envs/" + providerEnvironmentA,
				"GET /api/v1/projects/" + providerProjectID + "/envs/" + providerEnvironmentA,
				"GET /api/v1/projects/" + providerProjectID,
			}
			if fmt.Sprint(requests) != fmt.Sprint(wantRequests) {
				t.Fatalf("requests = %v, want %v", requests, wantRequests)
			}
		})
	}
}

func TestEnvironmentResourceDeleteAmbiguityPreservesState(t *testing.T) {
	t.Parallel()

	requestCount := 0
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, _ *http.Request) {
			requestCount++
			switch requestCount {
			case 1, 2:
				writeProjectResourceEnvelope(t, response, http.StatusNotFound, `null`)
			case 3, 4:
				writeProjectResourceEnvelope(t, response, http.StatusInternalServerError, `null`)
			default:
				t.Fatal("Delete() made an unexpected request")
			}
		},
	))
	defer closeServer()

	environmentSchema := environmentResourceTestSchema(t)
	priorState := environmentResourceTestState(
		t,
		environmentSchema,
		providerProjectID,
		providerEnvironmentA,
		"Staging",
		"staging",
		"",
	)
	response := frameworkresource.DeleteResponse{State: priorState}
	(&environmentResource{client: apiClient}).Delete(
		context.Background(),
		frameworkresource.DeleteRequest{State: priorState},
		&response,
	)
	if !response.Diagnostics.HasError() || requestCount != 4 {
		t.Fatalf("Delete() diagnostics = %v, requests = %d", response.Diagnostics, requestCount)
	}
	if !response.State.Raw.Equal(priorState.Raw) {
		t.Fatal("ambiguous Delete changed Terraform state")
	}
}

func TestEnvironmentResourceImportValidation(t *testing.T) {
	t.Parallel()

	environmentSchema := environmentResourceTestSchema(t)
	validID := providerProjectID + "/" + providerEnvironmentA
	tests := map[string]struct {
		importID string
		wantErr  bool
	}{
		"valid composite":        {importID: validID},
		"empty":                  {importID: "", wantErr: true},
		"one component":          {importID: providerProjectID, wantErr: true},
		"three components":       {importID: validID + "/extra", wantErr: true},
		"malformed project UUID": {importID: "not-a-uuid/" + providerEnvironmentA, wantErr: true},
		"malformed environment UUID": {
			importID: providerProjectID + "/not-a-uuid",
			wantErr:  true,
		},
		"surrounding space": {importID: " " + validID, wantErr: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			state := tfsdk.State{Schema: environmentSchema}
			initial := environmentModel{
				ProjectID:   types.StringNull(),
				ID:          types.StringNull(),
				Name:        types.StringNull(),
				Key:         types.StringNull(),
				Description: types.StringNull(),
			}
			if diagnostics := state.Set(context.Background(), &initial); diagnostics.HasError() {
				t.Fatalf("initialize import state: %v", diagnostics)
			}
			response := frameworkresource.ImportStateResponse{State: state}
			(&environmentResource{}).ImportState(
				context.Background(),
				frameworkresource.ImportStateRequest{ID: test.importID},
				&response,
			)
			if response.Diagnostics.HasError() != test.wantErr {
				t.Fatalf("ImportState() diagnostics = %v", response.Diagnostics)
			}
			if test.wantErr {
				if diagnosticsContain(response.Diagnostics, test.importID) && test.importID != "" {
					t.Fatal("ImportState() diagnostic echoed the rejected identifier")
				}
				return
			}
			var projectID types.String
			var environmentID types.String
			projectDiagnostics := response.State.GetAttribute(
				context.Background(),
				path.Root("project_id"),
				&projectID,
			)
			environmentDiagnostics := response.State.GetAttribute(
				context.Background(),
				path.Root("id"),
				&environmentID,
			)
			if projectDiagnostics.HasError() || environmentDiagnostics.HasError() ||
				projectID.ValueString() != providerProjectID ||
				environmentID.ValueString() != providerEnvironmentA {
				t.Fatalf(
					"imported IDs = %q/%q, diagnostics = %v/%v",
					projectID.ValueString(),
					environmentID.ValueString(),
					projectDiagnostics,
					environmentDiagnostics,
				)
			}
		})
	}
}

func environmentResourceTestSchema(t *testing.T) resourceschema.Schema {
	t.Helper()
	var response frameworkresource.SchemaResponse
	(&environmentResource{}).Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Environment resource schema diagnostics = %v", response.Diagnostics)
	}
	return response.Schema
}

func environmentResourceTestPlan(
	t *testing.T,
	environmentSchema resourceschema.Schema,
	projectID string,
	name string,
	key string,
	description string,
) tfsdk.Plan {
	t.Helper()
	plan := tfsdk.Plan{Schema: environmentSchema}
	model := environmentModel{
		ProjectID:   types.StringValue(projectID),
		ID:          types.StringUnknown(),
		Name:        types.StringValue(name),
		Key:         types.StringValue(key),
		Description: types.StringValue(description),
	}
	if diagnostics := plan.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize Environment plan: %v", diagnostics)
	}
	return plan
}

func environmentResourceTestState(
	t *testing.T,
	environmentSchema resourceschema.Schema,
	projectID string,
	environmentID string,
	name string,
	key string,
	description string,
) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: environmentSchema}
	model := flattenEnvironment(projectID, client.Environment{
		ID:          environmentID,
		Name:        name,
		Key:         key,
		Description: description,
	})
	if diagnostics := state.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize Environment state: %v", diagnostics)
	}
	return state
}

func environmentResourceStateModel(t *testing.T, state tfsdk.State) environmentModel {
	t.Helper()
	var model environmentModel
	if diagnostics := state.Get(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("read Environment state: %v", diagnostics)
	}
	return model
}

func providerEnvironmentParentJSON(projectID string, environments string) string {
	return `{"id":"` + projectID + `","name":"Parent","key":"parent","environments":` +
		environments + `}`
}

func providerEnvironmentVMJSON(environmentID string, name string, description string) string {
	return `{"id":"` + environmentID + `","name":"` + name + `","description":"` + description +
		`","secrets":[{"value":"` + providerEnvironmentSecretMarker + `"}],` +
		`"settings":{"requireChangeComment":true,"future":{"mode":"keep"}}}`
}

func providerEnvironmentExactJSON(
	environmentID string,
	name string,
	key string,
	description string,
) string {
	return `{"id":"` + environmentID + `","name":"` + name + `","key":"` + key +
		`","description":"` + description + `","secrets":[{"value":"` +
		providerEnvironmentSecretMarker + `"}],` +
		`"settings":{"requireChangeComment":true,"future":{"mode":"keep"}}}`
}

func waitForEnvironmentLockUsers(
	t *testing.T,
	manager *environmentLockManager,
	projectID string,
	environmentID string,
	want int,
) {
	t.Helper()
	key := strings.ToLower(projectID + "/" + environmentID)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		manager.mu.Lock()
		entry := manager.locks[key]
		users := 0
		if entry != nil {
			users = entry.users
		}
		manager.mu.Unlock()
		if users == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("Environment lock users did not reach %d", want)
}
