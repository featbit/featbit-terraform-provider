// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestPolicyResourceMetadataConfigureAndImport(t *testing.T) {
	t.Parallel()

	resourceUnderTest := &policyResource{}
	var metadata frameworkresource.MetadataResponse
	resourceUnderTest.Metadata(
		context.Background(),
		frameworkresource.MetadataRequest{ProviderTypeName: "featbit"},
		&metadata,
	)
	if metadata.TypeName != "featbit_policy" {
		t.Fatalf("metadata type = %q", metadata.TypeName)
	}

	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {
			t.Fatal("Configure() reached transport")
		},
	))
	defer closeServer()
	var configure frameworkresource.ConfigureResponse
	resourceUnderTest.Configure(
		context.Background(),
		frameworkresource.ConfigureRequest{ProviderData: apiClient},
		&configure,
	)
	if configure.Diagnostics.HasError() || resourceUnderTest.client != apiClient {
		t.Fatalf("Configure() diagnostics/client = %v/%p", configure.Diagnostics, resourceUnderTest.client)
	}

	policySchema := policyResourceTestSchema(t)
	tests := map[string]struct {
		id        string
		wantError bool
	}{
		"canonical":               {id: providerPolicyID},
		"uppercase canonicalized": {id: strings.ToUpper(providerPolicyID)},
		"missing":                 {id: "", wantError: true},
		"invalid":                 {id: "not-a-uuid", wantError: true},
		"composite":               {id: providerPolicyID + "/extra", wantError: true},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			response := frameworkresource.ImportStateResponse{State: emptyPolicyResourceState(t, policySchema)}
			resourceUnderTest.ImportState(
				context.Background(),
				frameworkresource.ImportStateRequest{ID: test.id},
				&response,
			)
			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf("ImportState() error = %t, want %t: %v", got, test.wantError, response.Diagnostics)
			}
			if !test.wantError {
				state := policyStateModel(t, response.State)
				if state.ID.ValueString() != providerPolicyID || !state.Name.IsNull() ||
					!state.Key.IsNull() || !state.Statements.IsNull() {
					t.Fatalf("ImportState() state = %#v", state)
				}
			}
		})
	}
}

func TestPolicyResourceCreateOwnsSettingsAndCompleteStatements(t *testing.T) {
	t.Parallel()

	tests := map[string][]client.PolicyStatement{
		"empty": {},
		"all control levels": {
			{
				ResourceType: "segment", Effect: "deny",
				Actions:   []string{"UpdateSegmentRules", "*"},
				Resources: []string{"project/P:env/E:segment/S;zeta,Alpha,zeta"},
			},
			{
				ResourceType: "project", Effect: "allow",
				Actions:   []string{"CreateEnv", "CanAccessProject"},
				Resources: []string{"project/*"},
			},
			{
				ResourceType: "flag", Effect: "allow",
				Actions:   []string{"ToggleFlag"},
				Resources: []string{"project/P:env/E:flag/F"},
			},
			{
				ResourceType: "env", Effect: "allow",
				Actions:   []string{"CanAccessEnv"},
				Resources: []string{"project/P:env/*"},
			},
		},
	}

	for name, statements := range tests {
		name := name
		statements := statements
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newPolicyResourceFixture(t, nil)
			apiClient, closeServer := newProjectResourceTestClient(t, fixture)
			defer closeServer()

			model := policyTestModel(t, "", "Managed", "managed", "Description", statements)
			policySchema := policyResourceTestSchema(t)
			response := frameworkresource.CreateResponse{State: tfsdk.State{Schema: policySchema}}
			(&policyResource{client: apiClient}).Create(
				context.Background(),
				frameworkresource.CreateRequest{Plan: policyResourceTestPlan(t, policySchema, model)},
				&response,
			)
			if response.Diagnostics.HasError() {
				t.Fatalf("Create() diagnostics = %v", response.Diagnostics)
			}
			state := policyStateModel(t, response.State)
			canonicalState, err := canonicalizePolicyStateModel(context.Background(), state)
			if err != nil || canonicalState.ID != providerPolicyID ||
				canonicalState.Key != "managed" || len(canonicalState.Statements) != len(statements) {
				t.Fatalf("Create() state = %#v/%v", canonicalState, err)
			}
			if got := fixture.mutations(); fmt.Sprint(got) != fmt.Sprint([]string{"create", "statements"}) {
				t.Fatalf("mutations = %v", got)
			}
			remote := fixture.currentPolicy()
			canonicalRemote, err := canonicalizeRemoteManagedPolicy(remote)
			if err != nil || !samePolicyDefinition(canonicalRemote, canonicalState) {
				t.Fatalf("remote/state = %#v/%#v/%v", canonicalRemote, canonicalState, err)
			}
			if name == "all control levels" {
				if canonicalRemote.Statements[0].ResourceType != "env" ||
					canonicalRemote.Statements[3].Resources[0] !=
						"project/P:env/E:segment/S;Alpha,zeta" {
					t.Fatalf("canonical mutation payload = %#v", canonicalRemote.Statements)
				}
			}
		})
	}
}

func TestPolicyResourceCreatePreflightAndAmbiguousOutcomeNeverAdopt(t *testing.T) {
	t.Parallel()

	t.Run("exact duplicate", func(t *testing.T) {
		t.Parallel()
		fixture := newPolicyResourceFixture(t, &client.Policy{
			ID: providerPolicyID, Name: "Existing", Key: "managed",
			Type: client.PolicyTypeCustomerManaged, Statements: []client.PolicyStatement{},
		})
		apiClient, closeServer := newProjectResourceTestClient(t, fixture)
		defer closeServer()
		policySchema := policyResourceTestSchema(t)
		response := frameworkresource.CreateResponse{State: tfsdk.State{Schema: policySchema}}
		(&policyResource{client: apiClient}).Create(
			context.Background(),
			frameworkresource.CreateRequest{Plan: policyResourceTestPlan(
				t,
				policySchema,
				policyTestModel(t, "", "Managed", "managed", "", nil),
			)},
			&response,
		)
		if !response.Diagnostics.HasError() || len(fixture.mutations()) != 0 {
			t.Fatalf("Create() diagnostics/mutations = %v/%v", response.Diagnostics, fixture.mutations())
		}
	})

	t.Run("ambiguous create", func(t *testing.T) {
		t.Parallel()
		fixture := newPolicyResourceFixture(t, nil)
		fixture.failCreateAfterApply = true
		apiClient, closeServer := newProjectResourceTestClient(t, fixture)
		defer closeServer()
		policySchema := policyResourceTestSchema(t)
		response := frameworkresource.CreateResponse{State: tfsdk.State{Schema: policySchema}}
		(&policyResource{client: apiClient}).Create(
			context.Background(),
			frameworkresource.CreateRequest{Plan: policyResourceTestPlan(
				t,
				policySchema,
				policyTestModel(t, "", "Managed", "managed", "", nil),
			)},
			&response,
		)
		if !response.Diagnostics.HasError() ||
			fmt.Sprint(fixture.mutations()) != fmt.Sprint([]string{"create"}) {
			t.Fatalf("Create() diagnostics/mutations = %v/%v", response.Diagnostics, fixture.mutations())
		}
		if !response.State.Raw.IsNull() {
			t.Fatal("ambiguous Create adopted the exact-key recovery object")
		}
		formatted := fmt.Sprint(response.Diagnostics)
		for _, unsafe := range []string{"managed", providerPolicyID, "/api/v1/policies"} {
			if strings.Contains(formatted, unsafe) {
				t.Fatalf("ambiguous Create diagnostic exposed %q", unsafe)
			}
		}
	})
}

func TestPolicyResourceCreateStatementFailurePreservesCreatedSettings(t *testing.T) {
	t.Parallel()

	fixture := newPolicyResourceFixture(t, nil)
	fixture.failStatementsBeforeApply = true
	apiClient, closeServer := newProjectResourceTestClient(t, fixture)
	defer closeServer()
	statements := []client.PolicyStatement{{
		ResourceType: "project", Effect: "allow",
		Actions: []string{"CreateProject"}, Resources: []string{"project/*"},
	}}
	policySchema := policyResourceTestSchema(t)
	response := frameworkresource.CreateResponse{State: tfsdk.State{Schema: policySchema}}
	(&policyResource{client: apiClient}).Create(
		context.Background(),
		frameworkresource.CreateRequest{Plan: policyResourceTestPlan(
			t,
			policySchema,
			policyTestModel(t, "", "Managed", "managed", "Description", statements),
		)},
		&response,
	)
	if !response.Diagnostics.HasError() ||
		fmt.Sprint(fixture.mutations()) != fmt.Sprint([]string{"create", "statements"}) {
		t.Fatalf("Create() diagnostics/mutations = %v/%v", response.Diagnostics, fixture.mutations())
	}
	state := policyStateModel(t, response.State)
	canonical, err := canonicalizePolicyStateModel(context.Background(), state)
	if err != nil || canonical.ID != providerPolicyID || canonical.Name != "Managed" ||
		len(canonical.Statements) != 0 {
		t.Fatalf("partial Create state = %#v/%v", canonical, err)
	}
}

func TestPolicyResourceUpdatePreservesConfirmedSettingsWhenStatementsFail(t *testing.T) {
	t.Parallel()

	priorStatements := []client.PolicyStatement{{
		ResourceType: "project", Effect: "allow",
		Actions: []string{"CanAccessProject"}, Resources: []string{"project/*"},
	}}
	plannedStatements := []client.PolicyStatement{{
		ResourceType: "flag", Effect: "allow",
		Actions: []string{"ToggleFlag"}, Resources: []string{"project/P:env/E:flag/F"},
	}}
	fixture := newPolicyResourceFixture(t, &client.Policy{
		ID: providerPolicyID, Name: "Original", Key: "managed", Description: "Before",
		Type: client.PolicyTypeCustomerManaged, Statements: priorStatements,
	})
	fixture.failStatementsBeforeApply = true
	apiClient, closeServer := newProjectResourceTestClient(t, fixture)
	defer closeServer()

	policySchema := policyResourceTestSchema(t)
	priorModel := policyTestModel(
		t, providerPolicyID, "Original", "managed", "Before", priorStatements,
	)
	planModel := policyTestModel(
		t, providerPolicyID, "Renamed", "managed", "After", plannedStatements,
	)
	priorState := policyResourceTestState(t, policySchema, priorModel)
	response := frameworkresource.UpdateResponse{State: priorState}
	(&policyResource{client: apiClient}).Update(
		context.Background(),
		frameworkresource.UpdateRequest{
			State: priorState,
			Plan:  policyResourceTestPlan(t, policySchema, planModel),
		},
		&response,
	)
	if !response.Diagnostics.HasError() ||
		fmt.Sprint(fixture.mutations()) != fmt.Sprint([]string{"settings", "statements"}) {
		t.Fatalf("Update() diagnostics/mutations = %v/%v", response.Diagnostics, fixture.mutations())
	}
	state := policyStateModel(t, response.State)
	canonical, err := canonicalizePolicyStateModel(context.Background(), state)
	if err != nil || canonical.Name != "Renamed" || canonical.Description != "After" ||
		!samePolicyStatements(canonical.Statements, mustCanonicalPolicyStatements(t, priorStatements)) {
		t.Fatalf("partial Update state = %#v/%v", canonical, err)
	}
}

func TestPolicyResourceReadHandlesDriftAbsenceAndBuiltInImport(t *testing.T) {
	t.Parallel()

	t.Run("drift", func(t *testing.T) {
		t.Parallel()
		remoteStatements := []client.PolicyStatement{{
			ResourceType: "segment", Effect: "deny",
			Actions: []string{"UpdateSegmentRules"}, Resources: []string{"project/P:env/E:segment/S"},
		}}
		fixture := newPolicyResourceFixture(t, &client.Policy{
			ID: providerPolicyID, Name: "Drifted", Key: "managed", Description: "Remote",
			Type: client.PolicyTypeCustomerManaged, Statements: remoteStatements,
		})
		apiClient, closeServer := newProjectResourceTestClient(t, fixture)
		defer closeServer()
		policySchema := policyResourceTestSchema(t)
		prior := policyResourceTestState(t, policySchema, policyTestModel(
			t, providerPolicyID, "Original", "managed", "Before", nil,
		))
		response := frameworkresource.ReadResponse{State: prior}
		(&policyResource{client: apiClient}).Read(
			context.Background(),
			frameworkresource.ReadRequest{State: prior},
			&response,
		)
		if response.Diagnostics.HasError() {
			t.Fatalf("Read() diagnostics = %v", response.Diagnostics)
		}
		state, err := canonicalizePolicyStateModel(
			context.Background(),
			policyStateModel(t, response.State),
		)
		if err != nil || state.Name != "Drifted" || state.Description != "Remote" ||
			len(state.Statements) != 1 {
			t.Fatalf("drift state = %#v/%v", state, err)
		}
	})

	t.Run("authoritative absence", func(t *testing.T) {
		t.Parallel()
		fixture := newPolicyResourceFixture(t, nil)
		apiClient, closeServer := newProjectResourceTestClient(t, fixture)
		defer closeServer()
		policySchema := policyResourceTestSchema(t)
		prior := policyResourceTestState(t, policySchema, policyTestModel(
			t, providerPolicyID, "Original", "managed", "", nil,
		))
		response := frameworkresource.ReadResponse{State: prior}
		(&policyResource{client: apiClient}).Read(
			context.Background(),
			frameworkresource.ReadRequest{State: prior},
			&response,
		)
		if response.Diagnostics.HasError() || !response.State.Raw.IsNull() {
			t.Fatalf("absence Read() diagnostics/state = %v/%s", response.Diagnostics, response.State.Raw)
		}
	})

	t.Run("built-in import", func(t *testing.T) {
		t.Parallel()
		fixture := newPolicyResourceFixture(t, &client.Policy{
			ID: providerPolicyID, Name: "Owner", Key: "Owner",
			Type: client.PolicyTypeSysManaged,
			Statements: []client.PolicyStatement{{
				ResourceType: "*", Effect: "allow", Actions: []string{"*"}, Resources: []string{"*"},
			}},
		})
		apiClient, closeServer := newProjectResourceTestClient(t, fixture)
		defer closeServer()
		policySchema := policyResourceTestSchema(t)
		imported := emptyPolicyResourceState(t, policySchema)
		var importResponse frameworkresource.ImportStateResponse
		importResponse.State = imported
		(&policyResource{}).ImportState(
			context.Background(),
			frameworkresource.ImportStateRequest{ID: providerPolicyID},
			&importResponse,
		)
		if importResponse.Diagnostics.HasError() {
			t.Fatalf("ImportState() diagnostics = %v", importResponse.Diagnostics)
		}
		response := frameworkresource.ReadResponse{State: importResponse.State}
		(&policyResource{client: apiClient}).Read(
			context.Background(),
			frameworkresource.ReadRequest{State: importResponse.State},
			&response,
		)
		if !response.Diagnostics.HasError() || len(fixture.mutations()) != 0 {
			t.Fatalf("built-in Read() diagnostics/mutations = %v/%v", response.Diagnostics, fixture.mutations())
		}
		if !diagnosticsContain(response.Diagnostics, "data source") {
			t.Fatal("built-in resource diagnostic omitted read-only data-source guidance")
		}
	})

	t.Run("invalid custom statement preserves state", func(t *testing.T) {
		t.Parallel()
		fixture := newPolicyResourceFixture(t, &client.Policy{
			ID: providerPolicyID, Name: "Managed", Key: "managed",
			Type: client.PolicyTypeCustomerManaged,
			Statements: []client.PolicyStatement{{
				ResourceType: "*", Effect: "allow", Actions: []string{"*"}, Resources: []string{"*"},
			}},
		})
		apiClient, closeServer := newProjectResourceTestClient(t, fixture)
		defer closeServer()
		policySchema := policyResourceTestSchema(t)
		prior := policyResourceTestState(t, policySchema, policyTestModel(
			t, providerPolicyID, "Managed", "managed", "", nil,
		))
		response := frameworkresource.ReadResponse{State: prior}
		(&policyResource{client: apiClient}).Read(
			context.Background(),
			frameworkresource.ReadRequest{State: prior},
			&response,
		)
		if !response.Diagnostics.HasError() || !response.State.Raw.Equal(prior.Raw) ||
			len(fixture.mutations()) != 0 {
			t.Fatalf("invalid remote Read() diagnostics/state/mutations = %v/%t/%v",
				response.Diagnostics, response.State.Raw.Equal(prior.Raw), fixture.mutations())
		}
	})
}

func TestPolicyResourceBuiltInNeverMutates(t *testing.T) {
	t.Parallel()

	fixture := newPolicyResourceFixture(t, &client.Policy{
		ID: providerPolicyID, Name: "Owner", Key: "Owner",
		Type: client.PolicyTypeSysManaged,
		Statements: []client.PolicyStatement{{
			ResourceType: "*", Effect: "allow", Actions: []string{"*"}, Resources: []string{"*"},
		}},
	})
	apiClient, closeServer := newProjectResourceTestClient(t, fixture)
	defer closeServer()
	policySchema := policyResourceTestSchema(t)
	priorModel := policyTestModel(t, providerPolicyID, "Owner", "Owner", "", nil)
	prior := policyResourceTestState(t, policySchema, priorModel)

	updateResponse := frameworkresource.UpdateResponse{State: prior}
	(&policyResource{client: apiClient}).Update(
		context.Background(),
		frameworkresource.UpdateRequest{
			State: prior,
			Plan: policyResourceTestPlan(t, policySchema, policyTestModel(
				t, providerPolicyID, "Renamed", "Owner", "", nil,
			)),
		},
		&updateResponse,
	)
	if !updateResponse.Diagnostics.HasError() || !updateResponse.State.Raw.Equal(prior.Raw) {
		t.Fatalf("built-in Update() diagnostics/state = %v/%t",
			updateResponse.Diagnostics, updateResponse.State.Raw.Equal(prior.Raw))
	}

	deleteResponse := frameworkresource.DeleteResponse{State: prior}
	(&policyResource{client: apiClient}).Delete(
		context.Background(),
		frameworkresource.DeleteRequest{State: prior},
		&deleteResponse,
	)
	if !deleteResponse.Diagnostics.HasError() || !deleteResponse.State.Raw.Equal(prior.Raw) ||
		len(fixture.mutations()) != 0 {
		t.Fatalf("built-in Delete() diagnostics/state/mutations = %v/%t/%v",
			deleteResponse.Diagnostics, deleteResponse.State.Raw.Equal(prior.Raw), fixture.mutations())
	}
}

func TestPolicyResourceDeleteRefusesAssociationsAndProvesAbsence(t *testing.T) {
	t.Parallel()

	base := &client.Policy{
		ID: providerPolicyID, Name: "Managed", Key: "managed",
		Type: client.PolicyTypeCustomerManaged, Statements: []client.PolicyStatement{},
	}
	t.Run("live association", func(t *testing.T) {
		t.Parallel()
		fixture := newPolicyResourceFixture(t, base)
		fixture.groupIDs = []string{"77777777-7777-4777-8777-777777777777"}
		fixture.memberIDs = []string{"88888888-8888-4888-8888-888888888888"}
		apiClient, closeServer := newProjectResourceTestClient(t, fixture)
		defer closeServer()
		policySchema := policyResourceTestSchema(t)
		prior := policyResourceTestState(t, policySchema, policyTestModel(
			t, providerPolicyID, "Managed", "managed", "", nil,
		))
		response := frameworkresource.DeleteResponse{State: prior}
		(&policyResource{client: apiClient}).Delete(
			context.Background(),
			frameworkresource.DeleteRequest{State: prior},
			&response,
		)
		if !response.Diagnostics.HasError() || len(fixture.mutations()) != 0 ||
			response.State.Raw.IsNull() {
			t.Fatalf("Delete() diagnostics/mutations/state = %v/%v/%s", response.Diagnostics, fixture.mutations(), response.State.Raw)
		}
	})

	t.Run("exact absence", func(t *testing.T) {
		t.Parallel()
		fixture := newPolicyResourceFixture(t, base)
		apiClient, closeServer := newProjectResourceTestClient(t, fixture)
		defer closeServer()
		policySchema := policyResourceTestSchema(t)
		prior := policyResourceTestState(t, policySchema, policyTestModel(
			t, providerPolicyID, "Managed", "managed", "", nil,
		))
		response := frameworkresource.DeleteResponse{State: prior}
		(&policyResource{client: apiClient}).Delete(
			context.Background(),
			frameworkresource.DeleteRequest{State: prior},
			&response,
		)
		if response.Diagnostics.HasError() || !response.State.Raw.IsNull() ||
			fmt.Sprint(fixture.mutations()) != fmt.Sprint([]string{"delete"}) {
			t.Fatalf("Delete() diagnostics/mutations/state = %v/%v/%s", response.Diagnostics, fixture.mutations(), response.State.Raw)
		}
	})
}

type policyResourceFixture struct {
	t *testing.T

	mu                        sync.Mutex
	policy                    *client.Policy
	requests                  []string
	mutationNames             []string
	failCreateAfterApply      bool
	failStatementsBeforeApply bool
	groupIDs                  []string
	memberIDs                 []string
}

func newPolicyResourceFixture(t *testing.T, policy *client.Policy) *policyResourceFixture {
	t.Helper()
	fixture := &policyResourceFixture{t: t}
	if policy != nil {
		copied := cloneProviderPolicy(*policy)
		fixture.policy = &copied
	}
	return fixture
}

func (f *policyResourceFixture) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, request.Method+" "+request.URL.EscapedPath())

	basePath := "/api/v1/policies"
	switch {
	case request.Method == http.MethodGet && request.URL.EscapedPath() == basePath:
		items := []client.Policy{}
		if f.policy != nil {
			items = append(items, cloneProviderPolicy(*f.policy))
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(items),
			"items":      items,
		})
	case request.Method == http.MethodPost && request.URL.EscapedPath() == basePath:
		f.mutationNames = append(f.mutationNames, "create")
		var input client.CreatePolicyRequest
		f.decodeBody(request.Body, &input)
		created := client.Policy{
			ID: providerPolicyID, Name: input.Name, Key: input.Key,
			Description: input.Description, Type: client.PolicyTypeCustomerManaged,
			Statements: []client.PolicyStatement{},
		}
		f.policy = &created
		if f.failCreateAfterApply {
			writePolicyProviderEnvelope(f.t, response, http.StatusInternalServerError, nil)
			return
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, created)
	case request.Method == http.MethodGet && strings.HasSuffix(request.URL.EscapedPath(), "/groups"):
		items := make([]map[string]any, 0, len(f.groupIDs))
		for _, id := range f.groupIDs {
			items = append(items, map[string]any{"id": id, "isPolicyGroup": true})
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(items), "items": items,
		})
	case request.Method == http.MethodGet && strings.HasSuffix(request.URL.EscapedPath(), "/members"):
		items := make([]map[string]any, 0, len(f.memberIDs))
		for _, id := range f.memberIDs {
			items = append(items, map[string]any{"id": id, "isPolicyMember": true})
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, map[string]any{
			"totalCount": len(items), "items": items,
		})
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.EscapedPath(), basePath+"/"):
		if f.policy == nil {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, cloneProviderPolicy(*f.policy))
	case request.Method == http.MethodPut && strings.HasSuffix(request.URL.EscapedPath(), "/settings"):
		f.mutationNames = append(f.mutationNames, "settings")
		var input client.UpdatePolicySettingsRequest
		f.decodeBody(request.Body, &input)
		if f.policy == nil {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		f.policy.Name = input.Name
		f.policy.Description = input.Description
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, cloneProviderPolicy(*f.policy))
	case request.Method == http.MethodPut && strings.HasSuffix(request.URL.EscapedPath(), "/statements"):
		f.mutationNames = append(f.mutationNames, "statements")
		var statements []client.PolicyStatement
		f.decodeBody(request.Body, &statements)
		if f.failStatementsBeforeApply {
			writePolicyProviderEnvelope(f.t, response, http.StatusInternalServerError, nil)
			return
		}
		if f.policy == nil {
			writePolicyProviderEnvelope(f.t, response, http.StatusNotFound, nil)
			return
		}
		f.policy.Statements = statements
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, cloneProviderPolicy(*f.policy))
	case request.Method == http.MethodDelete && strings.HasPrefix(request.URL.EscapedPath(), basePath+"/"):
		f.mutationNames = append(f.mutationNames, "delete")
		f.policy = nil
		writePolicyProviderEnvelope(f.t, response, http.StatusOK, true)
	default:
		f.t.Fatalf("unexpected Policy fixture request %s %s", request.Method, request.URL.EscapedPath())
	}
}

func (f *policyResourceFixture) decodeBody(body io.ReadCloser, target any) {
	f.t.Helper()
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		f.t.Fatalf("decode Policy request: %v", err)
	}
}

func (f *policyResourceFixture) mutations() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.mutationNames...)
}

func (f *policyResourceFixture) currentPolicy() client.Policy {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.policy == nil {
		return client.Policy{}
	}
	return cloneProviderPolicy(*f.policy)
}

func cloneProviderPolicy(policy client.Policy) client.Policy {
	cloned := policy
	cloned.Statements = make([]client.PolicyStatement, 0, len(policy.Statements))
	for _, statement := range policy.Statements {
		cloned.Statements = append(cloned.Statements, client.PolicyStatement{
			ResourceType: statement.ResourceType,
			Effect:       statement.Effect,
			Actions:      append([]string(nil), statement.Actions...),
			Resources:    append([]string(nil), statement.Resources...),
		})
	}
	return cloned
}

func mustCanonicalPolicyStatements(
	t *testing.T,
	statements []client.PolicyStatement,
) []canonicalPolicyStatement {
	t.Helper()
	canonical, err := canonicalizeManagedPolicyStatements(statements)
	if err != nil {
		t.Fatalf("canonicalize test statements: %v", err)
	}
	return canonical
}

func policyResourceTestSchema(t *testing.T) resourceschema.Schema {
	t.Helper()
	var response frameworkresource.SchemaResponse
	(&policyResource{}).Schema(context.Background(), frameworkresource.SchemaRequest{}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("Policy resource schema diagnostics = %v", response.Diagnostics)
	}
	return response.Schema
}

func policyResourceTestPlan(
	t *testing.T,
	policySchema resourceschema.Schema,
	model policyModel,
) tfsdk.Plan {
	t.Helper()
	model.ID = types.StringUnknown()
	model.Type = types.StringUnknown()
	state := tfsdk.State{Schema: policySchema}
	if diagnostics := state.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize Policy plan: %v", diagnostics)
	}
	return tfsdk.Plan{Schema: policySchema, Raw: state.Raw}
}

func policyResourceTestState(
	t *testing.T,
	policySchema resourceschema.Schema,
	model policyModel,
) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: policySchema}
	if diagnostics := state.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize Policy state: %v", diagnostics)
	}
	return state
}

func emptyPolicyResourceState(t *testing.T, policySchema resourceschema.Schema) tfsdk.State {
	t.Helper()
	return policyResourceTestState(t, policySchema, policyModel{
		ID:          types.StringNull(),
		Name:        types.StringNull(),
		Key:         types.StringNull(),
		Description: types.StringNull(),
		Type:        types.StringNull(),
		Statements:  types.SetNull(policyStatementObjectType()),
	})
}
