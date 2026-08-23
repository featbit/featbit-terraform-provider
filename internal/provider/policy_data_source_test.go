// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestPolicyDataSourceSelectorValidation(t *testing.T) {
	t.Parallel()

	policySchema := policyDataSourceTestSchema(t)
	tests := map[string]struct {
		id        types.String
		key       types.String
		wantError bool
	}{
		"exact UUID": {
			id:  types.StringValue(providerPolicyID),
			key: types.StringNull(),
		},
		"exact key": {
			id:  types.StringNull(),
			key: types.StringValue("Owner"),
		},
		"unknown reference": {
			id:  types.StringUnknown(),
			key: types.StringNull(),
		},
		"missing": {
			id:        types.StringNull(),
			key:       types.StringNull(),
			wantError: true,
		},
		"both": {
			id:        types.StringValue(providerPolicyID),
			key:       types.StringValue("Owner"),
			wantError: true,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var response datasource.ValidateConfigResponse
			(&policyDataSource{}).ValidateConfig(
				context.Background(),
				datasource.ValidateConfigRequest{Config: policyDataSourceTestConfig(
					t,
					policySchema,
					test.id,
					test.key,
				)},
				&response,
			)
			if got := response.Diagnostics.HasError(); got != test.wantError {
				t.Fatalf("ValidateConfig() error = %t, want %t: %v", got, test.wantError, response.Diagnostics)
			}
		})
	}
}

func TestPolicyDataSourceReadsCustomAndBuiltInByExactSelector(t *testing.T) {
	t.Parallel()

	builtIn := client.Policy{
		ID:          providerPolicyID,
		Name:        "Owner",
		Key:         "Owner",
		Type:        client.PolicyTypeSysManaged,
		Description: "",
		Statements: []client.PolicyStatement{{
			ResourceType: "*",
			Effect:       "allow",
			Actions:      []string{"*"},
			Resources:    []string{"*"},
		}},
	}
	custom := builtIn
	custom.Name = "Custom"
	custom.Key = "custom"
	custom.Type = client.PolicyTypeCustomerManaged
	custom.Statements = []client.PolicyStatement{{
		ResourceType: "project",
		Effect:       "allow",
		Actions:      []string{"CreateProject", "CanAccessProject"},
		Resources:    []string{"project/*"},
	}}

	tests := map[string]struct {
		id     types.String
		key    types.String
		policy client.Policy
	}{
		"built-in by UUID": {
			id:     types.StringValue(providerPolicyID),
			key:    types.StringNull(),
			policy: builtIn,
		},
		"custom by key": {
			id:     types.StringNull(),
			key:    types.StringValue("custom"),
			policy: custom,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			requestCount := 0
			apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
				func(response http.ResponseWriter, request *http.Request) {
					requestCount++
					switch requestCount {
					case 1:
						if request.Method != http.MethodGet || request.URL.EscapedPath() != "/api/v1/policies" ||
							request.URL.Query().Get("PageIndex") != "0" ||
							request.URL.Query().Get("PageSize") != "100" {
							t.Fatalf("list request = %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
						}
						writePolicyProviderEnvelope(t, response, http.StatusOK, map[string]any{
							"totalCount": 1,
							"items":      []client.Policy{test.policy},
						})
					case 2:
						if request.Method != http.MethodGet ||
							request.URL.EscapedPath() != "/api/v1/policies/"+providerPolicyID ||
							request.URL.RawQuery != "" {
							t.Fatalf("exact request = %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
						}
						writePolicyProviderEnvelope(t, response, http.StatusOK, test.policy)
					default:
						t.Fatal("Policy data source made an unexpected request")
					}
				},
			))
			defer closeServer()

			policySchema := policyDataSourceTestSchema(t)
			response := datasource.ReadResponse{State: tfsdk.State{Schema: policySchema}}
			(&policyDataSource{client: apiClient}).Read(
				context.Background(),
				datasource.ReadRequest{Config: policyDataSourceTestConfig(
					t,
					policySchema,
					test.id,
					test.key,
				)},
				&response,
			)
			if response.Diagnostics.HasError() {
				t.Fatalf("Read() diagnostics = %v", response.Diagnostics)
			}
			state := policyStateModel(t, response.State)
			if state.ID.ValueString() != providerPolicyID ||
				state.Key.ValueString() != test.policy.Key ||
				state.Type.ValueString() != test.policy.Type {
				t.Fatalf("Read() state = %#v", state)
			}
			statements, err := terraformPolicyStatementModels(context.Background(), state.Statements)
			if err != nil || len(statements) != 1 ||
				statements[0].ResourceType.ValueString() != test.policy.Statements[0].ResourceType {
				t.Fatalf("Read() statements = %#v/%v", statements, err)
			}
			if requestCount != 2 {
				t.Fatalf("request count = %d, want 2", requestCount)
			}
		})
	}
}

func TestPolicyDataSourceExactKeyRejectsZeroAndDuplicatesWithoutDisclosure(t *testing.T) {
	t.Parallel()

	const runtimeKey = "runtime-policy-key-marker"
	tests := map[string][]client.Policy{
		"zero": {},
		"duplicate": {
			{
				ID: providerPolicyID, Name: "One", Key: runtimeKey,
				Type: client.PolicyTypeCustomerManaged, Statements: []client.PolicyStatement{},
			},
			{
				ID: "66666666-6666-4666-8666-666666666666", Name: "Two", Key: runtimeKey,
				Type: client.PolicyTypeCustomerManaged, Statements: []client.PolicyStatement{},
			},
		},
	}

	for name, policies := range tests {
		name := name
		policies := policies
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
				func(response http.ResponseWriter, request *http.Request) {
					writePolicyProviderEnvelope(t, response, http.StatusOK, map[string]any{
						"totalCount": len(policies),
						"items":      policies,
					})
				},
			))
			defer closeServer()

			policySchema := policyDataSourceTestSchema(t)
			response := datasource.ReadResponse{State: tfsdk.State{Schema: policySchema}}
			(&policyDataSource{client: apiClient}).Read(
				context.Background(),
				datasource.ReadRequest{Config: policyDataSourceTestConfig(
					t,
					policySchema,
					types.StringNull(),
					types.StringValue(runtimeKey),
				)},
				&response,
			)
			if !response.Diagnostics.HasError() {
				t.Fatal("Read() accepted a non-unique exact Policy key")
			}
			formatted := fmt.Sprint(response.Diagnostics)
			for _, unsafe := range []string{runtimeKey, providerPolicyID, "/api/v1/policies"} {
				if strings.Contains(formatted, unsafe) {
					t.Fatalf("diagnostic exposed runtime value %q", unsafe)
				}
			}
		})
	}
}

func policyDataSourceTestSchema(t *testing.T) datasourceschema.Schema {
	t.Helper()
	var response datasource.SchemaResponse
	(&policyDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("Policy data source schema diagnostics = %v", response.Diagnostics)
	}
	return response.Schema
}

func policyDataSourceTestConfig(
	t *testing.T,
	policySchema datasourceschema.Schema,
	id types.String,
	key types.String,
) tfsdk.Config {
	t.Helper()
	configured := policyModel{
		ID:          id,
		Name:        types.StringNull(),
		Key:         key,
		Description: types.StringNull(),
		Type:        types.StringNull(),
		Statements:  types.SetNull(policyStatementObjectType()),
	}
	state := tfsdk.State{Schema: policySchema}
	if diagnostics := state.Set(context.Background(), &configured); diagnostics.HasError() {
		t.Fatalf("initialize Policy data source config: %v", diagnostics)
	}
	return tfsdk.Config{Schema: policySchema, Raw: state.Raw}
}

func policyStateModel(t *testing.T, state tfsdk.State) policyModel {
	t.Helper()
	var model policyModel
	if diagnostics := state.Get(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("read Policy state: %v", diagnostics)
	}
	return model
}

func writePolicyProviderEnvelope(
	t *testing.T,
	response http.ResponseWriter,
	status int,
	data any,
) {
	t.Helper()
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	payload := map[string]any{
		"success": status >= http.StatusOK && status < http.StatusMultipleChoices,
		"data":    data,
		"errors":  []string{},
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		payload["errors"] = []string{"synthetic Policy failure"}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal Policy response: %v", err)
	}
	if _, err := response.Write(encoded); err != nil {
		t.Errorf("write Policy response: %v", err)
	}
}
