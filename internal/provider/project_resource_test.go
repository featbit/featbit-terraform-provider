// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestProjectResourceMetadataSchemaAndConfigure(t *testing.T) {
	t.Parallel()

	resourceUnderTest := &projectResource{}
	var metadataResponse frameworkresource.MetadataResponse
	resourceUnderTest.Metadata(
		context.Background(),
		frameworkresource.MetadataRequest{ProviderTypeName: "featbit"},
		&metadataResponse,
	)
	if metadataResponse.TypeName != "featbit_project" {
		t.Fatalf("type name = %q", metadataResponse.TypeName)
	}

	projectSchema := projectResourceTestSchema(t)
	if got := len(projectSchema.Attributes); got != 4 {
		t.Fatalf("attribute count = %d, want 4", got)
	}
	idAttribute, ok := projectSchema.Attributes["id"].(resourceschema.StringAttribute)
	if !ok || !idAttribute.Computed || idAttribute.Required || idAttribute.Optional ||
		len(idAttribute.PlanModifiers) != 1 {
		t.Fatalf("id attribute = %#v", projectSchema.Attributes["id"])
	}
	var stableIDResponse planmodifier.StringResponse
	stableIDResponse.PlanValue = types.StringUnknown()
	idAttribute.PlanModifiers[0].PlanModifyString(
		context.Background(),
		planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			PlanValue:   types.StringUnknown(),
			StateValue:  types.StringValue(providerProjectID),
			Plan:        projectResourceTestPlan(t, projectSchema, "Project Updated", "stable-key"),
			State:       projectResourceTestState(t, projectSchema, "Project", "stable-key"),
		},
		&stableIDResponse,
	)
	if stableIDResponse.Diagnostics.HasError() ||
		!stableIDResponse.PlanValue.Equal(types.StringValue(providerProjectID)) {
		t.Fatalf("id in-place plan modifier response = %#v", stableIDResponse)
	}
	var replacementIDResponse planmodifier.StringResponse
	replacementIDResponse.PlanValue = types.StringUnknown()
	idAttribute.PlanModifiers[0].PlanModifyString(
		context.Background(),
		planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			PlanValue:   types.StringUnknown(),
			StateValue:  types.StringValue(providerProjectID),
			Plan:        projectResourceTestPlan(t, projectSchema, "Project", "new-key"),
			State:       projectResourceTestState(t, projectSchema, "Project", "old-key"),
		},
		&replacementIDResponse,
	)
	if replacementIDResponse.Diagnostics.HasError() || !replacementIDResponse.PlanValue.IsUnknown() {
		t.Fatalf("id replacement plan modifier response = %#v", replacementIDResponse)
	}
	nameAttribute, ok := projectSchema.Attributes["name"].(resourceschema.StringAttribute)
	if !ok || !nameAttribute.Required || nameAttribute.Computed || nameAttribute.Optional {
		t.Fatalf("name attribute = %#v", projectSchema.Attributes["name"])
	}
	keyAttribute, ok := projectSchema.Attributes["key"].(resourceschema.StringAttribute)
	if !ok || !keyAttribute.Required || len(keyAttribute.PlanModifiers) != 1 {
		t.Fatalf("key attribute = %#v", projectSchema.Attributes["key"])
	}
	var modifierResponse planmodifier.StringResponse
	keyAttribute.PlanModifiers[0].PlanModifyString(
		context.Background(),
		planmodifier.StringRequest{
			ConfigValue: types.StringValue("new"),
			PlanValue:   types.StringValue("new"),
			StateValue:  types.StringValue("old"),
			Plan:        projectResourceTestPlan(t, projectSchema, "Project", "new"),
			State:       projectResourceTestState(t, projectSchema, "Project", "old"),
		},
		&modifierResponse,
	)
	if !modifierResponse.RequiresReplace {
		t.Fatalf("key plan modifier %T did not require replacement", keyAttribute.PlanModifiers[0])
	}
	environments, ok := projectSchema.Attributes["environments"].(resourceschema.ListNestedAttribute)
	if !ok || !environments.Computed || len(environments.NestedObject.Attributes) != 4 {
		t.Fatalf("environments attribute = %#v", projectSchema.Attributes["environments"])
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

func TestProjectResourceCreatePreflightAndCanonicalRead(t *testing.T) {
	t.Parallel()

	const projectKey = "exact-key"
	var mu sync.Mutex
	var requests []string
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			mu.Lock()
			requests = append(requests, request.Method+" "+request.URL.EscapedPath())
			call := len(requests)
			mu.Unlock()

			switch call {
			case 1:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					`[`+providerProjectJSON(
						"22222222-2222-4222-8222-222222222222",
						"Fuzzy",
						projectKey+"-extra",
					)+`]`,
				)
			case 2:
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatalf("read create body: %v", err)
				}
				if got := string(body); got != `{"name":"Project","key":"`+projectKey+`"}` {
					t.Fatalf("create body = %s", got)
				}
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerProjectJSON(providerProjectID, "Project", projectKey),
				)
			case 3:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerProjectJSON(providerProjectID, "Project", projectKey),
				)
			default:
				t.Fatalf("unexpected request %s %s", request.Method, request.URL.EscapedPath())
			}
		},
	))
	defer closeServer()

	resourceUnderTest := &projectResource{client: apiClient}
	projectSchema := projectResourceTestSchema(t)
	plan := projectResourceTestPlan(t, projectSchema, "Project", projectKey)
	response := frameworkresource.CreateResponse{State: tfsdk.State{Schema: projectSchema}}
	resourceUnderTest.Create(
		context.Background(),
		frameworkresource.CreateRequest{Plan: plan},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Create() diagnostics = %v", response.Diagnostics)
	}
	state := projectResourceStateModel(t, response.State)
	if state.ID.ValueString() != providerProjectID || state.Key.ValueString() != projectKey {
		t.Fatalf("Create() state = %#v", state)
	}
	if len(state.Environments) != 2 || state.Environments[0].Key.ValueString() != "dev" ||
		state.Environments[1].Key.ValueString() != "prod" {
		t.Fatalf("Create() environments = %#v", state.Environments)
	}
	mu.Lock()
	defer mu.Unlock()
	wantRequests := []string{
		"GET /api/v1/projects",
		"POST /api/v1/projects",
		"GET /api/v1/projects/" + providerProjectID,
	}
	if fmt.Sprint(requests) != fmt.Sprint(wantRequests) {
		t.Fatalf("requests = %v, want %v", requests, wantRequests)
	}
}

func TestProjectResourceCreateRejectsExactDuplicatePreflight(t *testing.T) {
	t.Parallel()

	const projectKey = "duplicate-key"
	requestCount := 0
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			requestCount++
			if request.Method != http.MethodGet {
				t.Fatal("duplicate preflight reached a mutation")
			}
			writeProjectResourceEnvelope(
				t,
				response,
				http.StatusOK,
				`[`+providerProjectJSON(providerProjectID, "One", projectKey)+`,`+
					providerProjectJSON(
						"22222222-2222-4222-8222-222222222222",
						"Two",
						projectKey,
					)+`]`,
			)
		},
	))
	defer closeServer()

	projectSchema := projectResourceTestSchema(t)
	response := frameworkresource.CreateResponse{State: tfsdk.State{Schema: projectSchema}}
	(&projectResource{client: apiClient}).Create(
		context.Background(),
		frameworkresource.CreateRequest{
			Plan: projectResourceTestPlan(t, projectSchema, "Project", projectKey),
		},
		&response,
	)
	if !response.Diagnostics.HasError() || requestCount != 1 {
		t.Fatalf("Create() diagnostics = %v, request count = %d", response.Diagnostics, requestCount)
	}
}

func TestProjectResourceAmbiguousCreateReconcilesWithoutRetryOrAdoption(t *testing.T) {
	t.Parallel()

	const projectKey = "recovery-key-marker"
	requestCount := 0
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			requestCount++
			switch requestCount {
			case 1:
				writeProjectResourceEnvelope(t, response, http.StatusOK, `[]`)
			case 2:
				writeProjectResourceEnvelope(t, response, http.StatusInternalServerError, `null`)
			case 3:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					`[`+providerProjectJSON(providerProjectID, "Recovered", projectKey)+`]`,
				)
			default:
				t.Fatal("ambiguous Create retried or made an unexpected request")
			}
		},
	))
	defer closeServer()

	projectSchema := projectResourceTestSchema(t)
	response := frameworkresource.CreateResponse{State: tfsdk.State{Schema: projectSchema}}
	(&projectResource{client: apiClient}).Create(
		context.Background(),
		frameworkresource.CreateRequest{
			Plan: projectResourceTestPlan(t, projectSchema, "Project", projectKey),
		},
		&response,
	)
	if !response.Diagnostics.HasError() || requestCount != 3 {
		t.Fatalf("Create() diagnostics = %v, request count = %d", response.Diagnostics, requestCount)
	}
	if !diagnosticsContain(response.Diagnostics, "import") {
		t.Fatal("ambiguous Create diagnostic omitted stable Import recovery guidance")
	}
	if diagnosticsContain(response.Diagnostics, projectKey) ||
		diagnosticsContain(response.Diagnostics, providerProjectID) {
		t.Fatal("ambiguous Create diagnostic disclosed a runtime key or UUID")
	}
}

func TestProjectResourceUpdateReadsCanonicalState(t *testing.T) {
	t.Parallel()

	const projectKey = "project-key"
	requestCount := 0
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			requestCount++
			switch requestCount {
			case 1:
				if request.Method != http.MethodPut {
					t.Fatalf("update method = %s", request.Method)
				}
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatalf("read update body: %v", err)
				}
				if got := string(body); got != `{"name":"Renamed"}` {
					t.Fatalf("update body = %s", got)
				}
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					`{"id":"`+providerProjectID+`","name":"Renamed"}`,
				)
			case 2:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerProjectJSON(providerProjectID, "Renamed", projectKey),
				)
			default:
				t.Fatal("Update() made an unexpected request")
			}
		},
	))
	defer closeServer()

	projectSchema := projectResourceTestSchema(t)
	priorState := projectResourceTestState(t, projectSchema, "Original", projectKey)
	plan := projectResourceTestPlan(t, projectSchema, "Renamed", projectKey)
	response := frameworkresource.UpdateResponse{State: priorState}
	(&projectResource{client: apiClient}).Update(
		context.Background(),
		frameworkresource.UpdateRequest{Plan: plan, State: priorState},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Update() diagnostics = %v", response.Diagnostics)
	}
	state := projectResourceStateModel(t, response.State)
	if state.Name.ValueString() != "Renamed" || state.Key.ValueString() != projectKey {
		t.Fatalf("Update() state = %#v", state)
	}
}

func TestProjectResourceUpdateFailurePreservesState(t *testing.T) {
	t.Parallel()

	requestCount := 0
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, _ *http.Request) {
			requestCount++
			writeProjectResourceEnvelope(t, response, http.StatusInternalServerError, `null`)
		},
	))
	defer closeServer()

	projectSchema := projectResourceTestSchema(t)
	priorState := projectResourceTestState(t, projectSchema, "Original", "project-key")
	response := frameworkresource.UpdateResponse{State: priorState}
	(&projectResource{client: apiClient}).Update(
		context.Background(),
		frameworkresource.UpdateRequest{
			Plan:  projectResourceTestPlan(t, projectSchema, "Renamed", "project-key"),
			State: priorState,
		},
		&response,
	)
	if !response.Diagnostics.HasError() || requestCount != 1 {
		t.Fatalf("Update() diagnostics = %v, request count = %d", response.Diagnostics, requestCount)
	}
	if !response.State.Raw.Equal(priorState.Raw) {
		t.Fatal("Update() failure changed Terraform state")
	}
}

func TestProjectResourceReadAmbiguityPreservesState(t *testing.T) {
	t.Parallel()

	requestCount := 0
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, _ *http.Request) {
			requestCount++
			switch requestCount {
			case 1:
				writeProjectResourceEnvelope(t, response, http.StatusNotFound, `null`)
			case 2:
				writeProjectResourceEnvelope(t, response, http.StatusInternalServerError, `null`)
			default:
				t.Fatal("Read() made an unexpected request")
			}
		},
	))
	defer closeServer()

	projectSchema := projectResourceTestSchema(t)
	priorState := projectResourceTestState(t, projectSchema, "Project", "project-key")
	response := frameworkresource.ReadResponse{State: priorState}
	(&projectResource{client: apiClient}).Read(
		context.Background(),
		frameworkresource.ReadRequest{State: priorState},
		&response,
	)
	if !response.Diagnostics.HasError() || requestCount != 2 {
		t.Fatalf("Read() diagnostics = %v, request count = %d", response.Diagnostics, requestCount)
	}
	if !response.State.Raw.Equal(priorState.Raw) {
		t.Fatal("ambiguous Read changed Terraform state")
	}
}

func TestProjectResourceCanceledUpdatePreservesStateAndLaterProgresses(t *testing.T) {
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
					`{"id":"`+providerProjectID+`","name":"Renamed"}`,
				)
			case 2:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerProjectJSON(providerProjectID, "Renamed", "project-key"),
				)
			default:
				t.Fatalf("unexpected request %s %s", request.Method, request.URL.EscapedPath())
			}
		},
	))
	defer closeServer()

	projectSchema := projectResourceTestSchema(t)
	priorState := projectResourceTestState(t, projectSchema, "Project", "project-key")
	plan := projectResourceTestPlan(t, projectSchema, "Renamed", "project-key")
	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	canceledResponse := frameworkresource.UpdateResponse{State: priorState}
	resourceUnderTest := &projectResource{client: apiClient}
	resourceUnderTest.Update(
		canceledContext,
		frameworkresource.UpdateRequest{Plan: plan, State: priorState},
		&canceledResponse,
	)
	if !canceledResponse.Diagnostics.HasError() || requestCount != 0 {
		t.Fatalf("canceled Update() diagnostics = %v, requests = %d", canceledResponse.Diagnostics, requestCount)
	}
	if !canceledResponse.State.Raw.Equal(priorState.Raw) {
		t.Fatal("canceled Update() changed Terraform state")
	}

	progressResponse := frameworkresource.UpdateResponse{State: priorState}
	resourceUnderTest.Update(
		context.Background(),
		frameworkresource.UpdateRequest{Plan: plan, State: priorState},
		&progressResponse,
	)
	if progressResponse.Diagnostics.HasError() || requestCount != 2 {
		t.Fatalf("progress Update() diagnostics = %v, requests = %d", progressResponse.Diagnostics, requestCount)
	}
}

func TestProjectResourceDeleteUsesExactFallbackForAlreadyAbsentObject(t *testing.T) {
	t.Parallel()

	var requests []string
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			requests = append(requests, request.Method+" "+request.URL.EscapedPath())
			switch len(requests) {
			case 1, 2:
				writeProjectResourceEnvelope(t, response, http.StatusNotFound, `null`)
			case 3:
				writeProjectResourceEnvelope(t, response, http.StatusOK, `[]`)
			default:
				t.Fatal("Delete() made an unexpected request")
			}
		},
	))
	defer closeServer()

	projectSchema := projectResourceTestSchema(t)
	priorState := projectResourceTestState(t, projectSchema, "Project", "project-key")
	response := frameworkresource.DeleteResponse{State: priorState}
	(&projectResource{client: apiClient}).Delete(
		context.Background(),
		frameworkresource.DeleteRequest{State: priorState},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Delete() diagnostics = %v", response.Diagnostics)
	}
	if !response.State.Raw.IsNull() {
		t.Fatal("Delete() did not remove authoritatively absent Project state")
	}
	wantRequests := []string{
		"DELETE /api/v1/projects/" + providerProjectID,
		"GET /api/v1/projects/" + providerProjectID,
		"GET /api/v1/projects",
	}
	if fmt.Sprint(requests) != fmt.Sprint(wantRequests) {
		t.Fatalf("requests = %v, want %v", requests, wantRequests)
	}
}

func TestProjectResourceDeleteAmbiguityPreservesState(t *testing.T) {
	t.Parallel()

	requestCount := 0
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, _ *http.Request) {
			requestCount++
			switch requestCount {
			case 1, 2:
				writeProjectResourceEnvelope(t, response, http.StatusNotFound, `null`)
			case 3:
				writeProjectResourceEnvelope(t, response, http.StatusInternalServerError, `null`)
			default:
				t.Fatal("Delete() made an unexpected request")
			}
		},
	))
	defer closeServer()

	projectSchema := projectResourceTestSchema(t)
	priorState := projectResourceTestState(t, projectSchema, "Project", "project-key")
	response := frameworkresource.DeleteResponse{State: priorState}
	(&projectResource{client: apiClient}).Delete(
		context.Background(),
		frameworkresource.DeleteRequest{State: priorState},
		&response,
	)
	if !response.Diagnostics.HasError() || requestCount != 3 {
		t.Fatalf("Delete() diagnostics = %v, request count = %d", response.Diagnostics, requestCount)
	}
	if !response.State.Raw.Equal(priorState.Raw) {
		t.Fatal("ambiguous Delete changed Terraform state")
	}
}

func TestProjectResourceImportValidation(t *testing.T) {
	t.Parallel()

	projectSchema := projectResourceTestSchema(t)
	tests := map[string]struct {
		importID string
		wantErr  bool
	}{
		"valid UUID":        {importID: providerProjectID},
		"empty":             {importID: "", wantErr: true},
		"two components":    {importID: providerProjectID + "/" + providerEnvironmentA, wantErr: true},
		"malformed UUID":    {importID: "not-a-uuid", wantErr: true},
		"surrounding space": {importID: " " + providerProjectID, wantErr: true},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			state := tfsdk.State{Schema: projectSchema}
			initial := projectModel{
				ID:           types.StringNull(),
				Name:         types.StringNull(),
				Key:          types.StringNull(),
				Environments: nil,
			}
			if diagnostics := state.Set(context.Background(), &initial); diagnostics.HasError() {
				t.Fatalf("initialize import state: %v", diagnostics)
			}
			response := frameworkresource.ImportStateResponse{State: state}
			(&projectResource{}).ImportState(
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
			var importedID types.String
			diagnostics := response.State.GetAttribute(
				context.Background(),
				path.Root("id"),
				&importedID,
			)
			if diagnostics.HasError() || importedID.ValueString() != providerProjectID {
				t.Fatalf("imported ID = %q, diagnostics = %v", importedID.ValueString(), diagnostics)
			}
		})
	}
}

func projectResourceTestSchema(t *testing.T) resourceschema.Schema {
	t.Helper()
	var response frameworkresource.SchemaResponse
	(&projectResource{}).Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Project resource schema diagnostics = %v", response.Diagnostics)
	}
	return response.Schema
}

func projectResourceTestPlan(
	t *testing.T,
	projectSchema resourceschema.Schema,
	name string,
	key string,
) tfsdk.Plan {
	t.Helper()
	plan := tfsdk.Plan{Schema: projectSchema}
	model := projectModel{
		ID:           types.StringUnknown(),
		Name:         types.StringValue(name),
		Key:          types.StringValue(key),
		Environments: nil,
	}
	if diagnostics := plan.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize Project plan: %v", diagnostics)
	}
	return plan
}

func projectResourceTestState(
	t *testing.T,
	projectSchema resourceschema.Schema,
	name string,
	key string,
) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: projectSchema}
	model := flattenProject(client.Project{
		ID:   providerProjectID,
		Name: name,
		Key:  key,
		Environments: []client.ProjectEnvironment{
			{ID: providerEnvironmentB, Name: "Prod", Key: "prod"},
			{ID: providerEnvironmentA, Name: "Dev", Key: "dev"},
		},
	})
	if diagnostics := state.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize Project state: %v", diagnostics)
	}
	return state
}

func projectResourceStateModel(t *testing.T, state tfsdk.State) projectModel {
	t.Helper()
	var model projectModel
	if diagnostics := state.Get(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("read Project state: %v", diagnostics)
	}
	return model
}

func newProjectResourceTestClient(
	t *testing.T,
	handler http.Handler,
) (*client.Client, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	apiURL, err := url.Parse(server.URL + "/api/v1")
	if err != nil {
		server.Close()
		t.Fatalf("url.Parse() error = %v", err)
	}
	apiClient, err := client.New(apiURL, syntheticProviderAccessToken, client.Options{
		HTTPTimeout:     client.DefaultHTTPTimeout,
		MaxConcurrency:  client.DefaultMaxConcurrency,
		MaxRetries:      0,
		ProviderVersion: "test",
	})
	if err != nil {
		server.Close()
		t.Fatalf("client.New() error = %v", err)
	}
	return apiClient, server.Close
}

func writeProjectResourceEnvelope(
	t *testing.T,
	response http.ResponseWriter,
	status int,
	data string,
) {
	t.Helper()
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	success := status >= http.StatusOK && status < http.StatusMultipleChoices
	if _, err := fmt.Fprintf(
		response,
		`{"success":%t,"data":%s,"errors":[]}`,
		success,
		data,
	); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func providerProjectJSON(id, name, key string) string {
	return `{"id":"` + id + `","name":"` + name + `","key":"` + key +
		`","environments":[` +
		`{"id":"` + providerEnvironmentB + `","name":"Prod","key":"prod","description":""},` +
		`{"id":"` + providerEnvironmentA + `","name":"Dev","key":"dev","description":""}` +
		`]}`
}
