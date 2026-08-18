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
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestSegmentDataSourceMetadataSchemaAndConfigure(t *testing.T) {
	t.Parallel()

	dataSource := &segmentDataSource{}
	var metadataResponse datasource.MetadataResponse
	dataSource.Metadata(
		context.Background(),
		datasource.MetadataRequest{ProviderTypeName: "featbit"},
		&metadataResponse,
	)
	if metadataResponse.TypeName != "featbit_segment" {
		t.Fatalf("type name = %q", metadataResponse.TypeName)
	}

	segmentSchema := segmentDataSourceTestSchema(t)
	if got := len(segmentSchema.Attributes); got != 11 {
		t.Fatalf("attribute count = %d, want 11", got)
	}
	for _, name := range []string{"environment_id", "id"} {
		attribute, ok := segmentSchema.Attributes[name].(datasourceschema.StringAttribute)
		if !ok || !attribute.Required || attribute.Optional || attribute.Computed ||
			len(attribute.Validators) != 1 {
			t.Fatalf("%s attribute = %#v", name, segmentSchema.Attributes[name])
		}
	}
	for _, name := range []string{"name", "key", "description", "type"} {
		attribute, ok := segmentSchema.Attributes[name].(datasourceschema.StringAttribute)
		if !ok || !attribute.Computed || attribute.Required || attribute.Optional {
			t.Fatalf("%s attribute = %#v", name, segmentSchema.Attributes[name])
		}
	}
	for _, name := range []string{"scopes", "included_users", "excluded_users", "tags"} {
		attribute, ok := segmentSchema.Attributes[name].(datasourceschema.SetAttribute)
		if !ok || !attribute.Computed || attribute.Required || attribute.Optional ||
			attribute.ElementType != types.StringType {
			t.Fatalf("%s attribute = %#v", name, segmentSchema.Attributes[name])
		}
	}
	rules, ok := segmentSchema.Attributes["rules"].(datasourceschema.ListNestedAttribute)
	if !ok || !rules.Computed || rules.Required || rules.Optional ||
		len(rules.NestedObject.Attributes) != 3 {
		t.Fatalf("rules attribute = %#v", segmentSchema.Attributes["rules"])
	}
	conditions, ok := rules.NestedObject.Attributes["conditions"].(datasourceschema.ListNestedAttribute)
	if !ok || !conditions.Computed || len(conditions.NestedObject.Attributes) != 4 {
		t.Fatalf("conditions attribute = %#v", rules.NestedObject.Attributes["conditions"])
	}

	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {
			t.Fatal("Configure() executed an HTTP request")
		},
	))
	defer closeServer()
	var configureResponse datasource.ConfigureResponse
	dataSource.Configure(
		context.Background(),
		datasource.ConfigureRequest{ProviderData: apiClient},
		&configureResponse,
	)
	if configureResponse.Diagnostics.HasError() || dataSource.client != apiClient {
		t.Fatalf("Configure() diagnostics = %v", configureResponse.Diagnostics)
	}

	var wrongTypeResponse datasource.ConfigureResponse
	dataSource.Configure(
		context.Background(),
		datasource.ConfigureRequest{ProviderData: struct{}{}},
		&wrongTypeResponse,
	)
	if !wrongTypeResponse.Diagnostics.HasError() {
		t.Fatal("Configure() accepted an unexpected provider data type")
	}
}

func TestSegmentDataSourceReadsEnvironmentSpecificAndSharedDefinitions(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		segmentType client.SegmentType
		scopes      []string
		wireEnvID   string
	}{
		"environment-specific": {
			segmentType: client.SegmentTypeEnvironmentSpecific,
			scopes:      []string{providerSegmentEnvironmentScope},
			wireEnvID:   providerEnvironmentA,
		},
		"shared": {
			segmentType: client.SegmentTypeShared,
			scopes: []string{
				providerSegmentProjectScope,
				providerSegmentOrganizationScope,
			},
			// Shared Segments are visible from the requested Environment even
			// when their stored environment context is different.
			wireEnvID: providerEnvironmentB,
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
					if request.Method != http.MethodGet || request.URL.RawQuery != "" ||
						request.URL.EscapedPath() != "/api/v1/envs/"+providerEnvironmentA+
							"/segments/"+providerSegmentID {
						t.Fatalf("unexpected exact request %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
					}
					assertProviderSegmentRequestHasNoContextHeaders(t, request)
					data := providerSegmentExactData(test.segmentType, test.scopes, false)
					data["envId"] = test.wireEnvID
					writeProjectResourceEnvelope(
						t,
						response,
						http.StatusOK,
						mustProviderSegmentJSON(t, data),
					)
				},
			))
			defer closeServer()

			segmentSchema := segmentDataSourceTestSchema(t)
			request := segmentDataSourceReadRequest(
				t,
				segmentSchema,
				providerEnvironmentA,
				providerSegmentID,
			)
			response := datasource.ReadResponse{State: tfsdk.State{Schema: segmentSchema}}
			(&segmentDataSource{client: apiClient}).Read(context.Background(), request, &response)
			if response.Diagnostics.HasError() {
				t.Fatalf("Read() diagnostics = %v", response.Diagnostics)
			}
			var state segmentModel
			if diagnostics := response.State.Get(context.Background(), &state); diagnostics.HasError() {
				t.Fatalf("read Segment data source state: %v", diagnostics)
			}
			included, _ := terraformStringSet(context.Background(), state.IncludedUsers)
			excluded, _ := terraformStringSet(context.Background(), state.ExcludedUsers)
			tags, _ := terraformStringSet(context.Background(), state.Tags)
			scopes, _ := terraformStringSet(context.Background(), state.Scopes)
			if state.EnvironmentID.ValueString() != providerEnvironmentA ||
				state.ID.ValueString() != providerSegmentID ||
				state.Name.ValueString() != "Synthetic Segment" ||
				state.Key.ValueString() != "synthetic-segment" ||
				state.Description.ValueString() != "Synthetic description" ||
				state.Type.ValueString() != string(test.segmentType) ||
				strings.Join(included, ",") != "user-a,user-z" ||
				strings.Join(excluded, ",") != "user-b,user-y" ||
				strings.Join(tags, ",") != "tag-a,tag-z" ||
				len(scopes) != len(canonicalStringSet(test.scopes)) ||
				len(state.Rules) != 2 || state.Rules[0].Name.ValueString() != "First" ||
				state.Rules[1].Name.ValueString() != "Second" ||
				state.Rules[0].Conditions[0].Value.ValueString() != `["a","b"]` ||
				state.Rules[0].Conditions[1].Value.ValueString() != "1" {
				t.Fatal("Read() did not persist the complete canonical Segment definition")
			}
			if requestCount != 1 {
				t.Fatalf("Read() request count = %d, want 1", requestCount)
			}
		})
	}
}

func TestSegmentDataSourceReadFailsClosedAndPreservesState(t *testing.T) {
	t.Parallel()

	const runtimeMarker = "segment-data-source-runtime-marker"
	tests := map[string]struct {
		status int
		mutate func(map[string]any)
	}{
		"archived": {
			status: http.StatusOK,
			mutate: func(data map[string]any) { data["isArchived"] = true },
		},
		"direct not found remains unconfirmed": {
			status: http.StatusNotFound,
		},
		"missing rule identity": {
			status: http.StatusOK,
			mutate: func(data map[string]any) {
				providerSegmentRuleData(data, 0)["id"] = ""
			},
		},
		"duplicate rule identity": {
			status: http.StatusOK,
			mutate: func(data map[string]any) {
				providerSegmentRuleData(data, 1)["id"] = strings.ToUpper(providerSegmentRuleOne)
			},
		},
		"duplicate condition identity": {
			status: http.StatusOK,
			mutate: func(data map[string]any) {
				providerSegmentConditionData(data, 1, 0)["id"] = strings.ToUpper(providerSegmentConditionOne)
			},
		},
		"unknown operator": {
			status: http.StatusOK,
			mutate: func(data map[string]any) {
				providerSegmentConditionData(data, 0, 0)["op"] = runtimeMarker
			},
		},
		"contradictory user sets": {
			status: http.StatusOK,
			mutate: func(data map[string]any) {
				data["included"] = []string{runtimeMarker}
				data["excluded"] = []string{runtimeMarker}
			},
		},
		"unsafe scope": {
			status: http.StatusOK,
			mutate: func(data map[string]any) { data["scopes"] = []string{"project/" + runtimeMarker} },
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			requestCount := 0
			apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
				func(response http.ResponseWriter, _ *http.Request) {
					requestCount++
					if test.status == http.StatusNotFound {
						writeProjectResourceEnvelope(t, response, test.status, "null")
						return
					}
					data := providerSegmentExactData(
						client.SegmentTypeEnvironmentSpecific,
						[]string{providerSegmentEnvironmentScope},
						false,
					)
					if test.mutate != nil {
						test.mutate(data)
					}
					writeProjectResourceEnvelope(
						t,
						response,
						test.status,
						mustProviderSegmentJSON(t, data),
					)
				},
			))
			defer closeServer()

			segmentSchema := segmentDataSourceTestSchema(t)
			request := segmentDataSourceReadRequest(
				t,
				segmentSchema,
				providerEnvironmentA,
				providerSegmentID,
			)
			prior := providerSegmentPriorDataSourceState(t, segmentSchema)
			priorRaw := prior.Raw.Copy()
			response := datasource.ReadResponse{State: prior}
			(&segmentDataSource{client: apiClient}).Read(context.Background(), request, &response)
			if !response.Diagnostics.HasError() {
				t.Fatal("unconfirmed Segment read produced no error diagnostic")
			}
			if !response.State.Raw.Equal(priorRaw) {
				t.Fatal("unconfirmed Segment read changed prior state")
			}
			formatted := fmt.Sprintf("%v", response.Diagnostics)
			for _, unsafe := range []string{
				providerEnvironmentA,
				providerSegmentID,
				providerSegmentRuleOne,
				providerSegmentConditionOne,
				runtimeMarker,
			} {
				if strings.Contains(formatted, unsafe) {
					t.Fatal("Segment data source diagnostic exposed a runtime identity or value")
				}
			}
			if requestCount != 1 {
				t.Fatalf("failed exact read request count = %d, want 1", requestCount)
			}
		})
	}
}

func TestSegmentDataSourceRequiresConfiguredClient(t *testing.T) {
	t.Parallel()

	segmentSchema := segmentDataSourceTestSchema(t)
	request := segmentDataSourceReadRequest(
		t,
		segmentSchema,
		providerEnvironmentA,
		providerSegmentID,
	)
	response := datasource.ReadResponse{State: tfsdk.State{Schema: segmentSchema}}
	(&segmentDataSource{}).Read(context.Background(), request, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("Read() accepted an unconfigured API client")
	}
}

func TestSegmentProtocolDataSourceSchemaAndUUIDValidation(t *testing.T) {
	t.Parallel()

	server := providerserver.NewProtocol6(New("test")())()
	productionSchema, err := server.GetProviderSchema(
		context.Background(),
		&tfprotov6.GetProviderSchemaRequest{},
	)
	if err != nil || protocolHasError(productionSchema.Diagnostics) {
		t.Fatalf("production GetProviderSchema() failed: %v / %v", err, productionSchema.Diagnostics)
	}
	if len(productionSchema.ResourceSchemas) != 7 {
		t.Fatalf("production resource schema count = %d, want 7", len(productionSchema.ResourceSchemas))
	}
	if len(productionSchema.DataSourceSchemas) != 6 {
		t.Fatalf("production data source schema count = %d, want 6", len(productionSchema.DataSourceSchemas))
	}
	if productionSchema.ResourceSchemas["featbit_segment"] == nil {
		t.Fatal("Protocol schema omitted registered featbit_segment resource")
	}
	segmentSchema := productionSchema.DataSourceSchemas["featbit_segment"]
	if segmentSchema == nil {
		t.Fatal("Protocol schema omitted featbit_segment data source")
	}
	assertProtocolSegmentDataSourceAttributes(t, segmentSchema.Block.Attributes)

	for name, configuration := range map[string]struct {
		environmentID string
		segmentID     string
		valid         bool
	}{
		"valid exact UUIDs": {
			environmentID: providerEnvironmentA,
			segmentID:     providerSegmentID,
			valid:         true,
		},
		"invalid environment UUID": {
			environmentID: "invalid",
			segmentID:     providerSegmentID,
		},
		"invalid Segment UUID": {
			environmentID: providerEnvironmentA,
			segmentID:     "invalid",
		},
	} {
		name := name
		configuration := configuration
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			config := protocolSegmentDataSourceConfig(
				t,
				segmentSchema,
				configuration.environmentID,
				configuration.segmentID,
			)
			response, err := server.ValidateDataResourceConfig(
				context.Background(),
				&tfprotov6.ValidateDataResourceConfigRequest{
					TypeName: "featbit_segment",
					Config:   &config,
				},
			)
			if err != nil {
				t.Fatalf("ValidateDataResourceConfig() error = %v", err)
			}
			if protocolHasError(response.Diagnostics) == configuration.valid {
				t.Fatalf("Protocol validation diagnostics = %v", response.Diagnostics)
			}
		})
	}
}

func segmentDataSourceTestSchema(t *testing.T) datasourceschema.Schema {
	t.Helper()
	var response datasource.SchemaResponse
	(&segmentDataSource{}).Schema(
		context.Background(),
		datasource.SchemaRequest{},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Segment data source schema diagnostics = %v", response.Diagnostics)
	}
	return response.Schema
}

func segmentDataSourceReadRequest(
	t *testing.T,
	segmentSchema datasourceschema.Schema,
	environmentID string,
	segmentID string,
) datasource.ReadRequest {
	t.Helper()
	configured := segmentModel{
		EnvironmentID: types.StringValue(environmentID),
		ID:            types.StringValue(segmentID),
		Name:          types.StringNull(),
		Key:           types.StringNull(),
		Description:   types.StringNull(),
		Type:          types.StringNull(),
		Scopes:        types.SetNull(types.StringType),
		IncludedUsers: types.SetNull(types.StringType),
		ExcludedUsers: types.SetNull(types.StringType),
		Rules:         nil,
		Tags:          types.SetNull(types.StringType),
	}
	configuredState := tfsdk.State{Schema: segmentSchema}
	if diagnostics := configuredState.Set(context.Background(), &configured); diagnostics.HasError() {
		t.Fatalf("initialize Segment data source config: %v", diagnostics)
	}
	return datasource.ReadRequest{
		Config: tfsdk.Config{Schema: segmentSchema, Raw: configuredState.Raw},
	}
}

func providerSegmentPriorDataSourceState(
	t *testing.T,
	segmentSchema datasourceschema.Schema,
) tfsdk.State {
	t.Helper()
	model, err := flattenSegment(providerRemoteSegment(
		client.SegmentTypeEnvironmentSpecific,
		[]string{providerSegmentEnvironmentScope},
	))
	if err != nil {
		t.Fatalf("build prior Segment data source model: %v", err)
	}
	state := tfsdk.State{Schema: segmentSchema}
	if diagnostics := state.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize Segment data source prior state: %v", diagnostics)
	}
	return state
}

func providerSegmentExactData(
	segmentType client.SegmentType,
	scopes []string,
	archived bool,
) map[string]any {
	return map[string]any{
		"id":                    providerSegmentID,
		"envId":                 providerEnvironmentA,
		"name":                  "Synthetic Segment",
		"key":                   "synthetic-segment",
		"description":           "Synthetic description",
		"type":                  string(segmentType),
		"scopes":                append([]string(nil), scopes...),
		"included":              []string{"user-z", "user-a", "user-z"},
		"excluded":              []string{"user-y", "user-b", "user-y"},
		"tags":                  []string{"tag-z", "tag-a", "tag-z"},
		"isArchived":            archived,
		"isEnvironmentSpecific": segmentType == client.SegmentTypeEnvironmentSpecific,
		"rules": []any{
			map[string]any{
				"id":   strings.ToUpper(providerSegmentRuleOne),
				"name": "First",
				"conditions": []any{
					map[string]any{
						"id":       strings.ToUpper(providerSegmentConditionOne),
						"property": "region",
						"op":       segmentOperatorIsOneOf,
						"value":    `["b","a","b"]`,
					},
					map[string]any{
						"id":       providerSegmentConditionTwo,
						"property": "score",
						"op":       segmentOperatorLessThan,
						"value":    "1.00",
					},
				},
			},
			map[string]any{
				"id":   providerSegmentRuleTwo,
				"name": "Second",
				"conditions": []any{
					map[string]any{
						"id":       providerSegmentConditionTri,
						"property": "enabled",
						"op":       segmentOperatorIsFalse,
						"value":    "",
					},
				},
			},
		},
	}
}

func providerSegmentRuleData(data map[string]any, index int) map[string]any {
	return data["rules"].([]any)[index].(map[string]any)
}

func providerSegmentConditionData(data map[string]any, ruleIndex, conditionIndex int) map[string]any {
	return providerSegmentRuleData(data, ruleIndex)["conditions"].([]any)[conditionIndex].(map[string]any)
}

func mustProviderSegmentJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal("encode Segment test response")
	}
	return string(encoded)
}

func assertProviderSegmentRequestHasNoContextHeaders(t *testing.T, request *http.Request) {
	t.Helper()
	for _, header := range []string{
		"Organization",
		"Workspace",
		"X-Organization",
		"X-Organization-Id",
		"X-Workspace",
		"X-Workspace-Id",
	} {
		if request.Header.Get(header) != "" {
			t.Fatalf("request sent unsupported context header %q", header)
		}
	}
}

func assertProtocolSegmentDataSourceAttributes(
	t *testing.T,
	attributes []*tfprotov6.SchemaAttribute,
) {
	t.Helper()
	byName := make(map[string]*tfprotov6.SchemaAttribute, len(attributes))
	for _, attribute := range attributes {
		if attribute != nil {
			byName[attribute.Name] = attribute
		}
	}
	if len(byName) != 11 {
		t.Fatalf("Protocol Segment attribute count = %d, want 11", len(byName))
	}
	for _, name := range []string{"environment_id", "id"} {
		attribute := byName[name]
		if attribute == nil || !attribute.Required || attribute.Optional || attribute.Computed {
			t.Fatalf("Protocol data source %s ownership is incorrect", name)
		}
	}
	for _, name := range []string{
		"name", "key", "description", "type", "scopes", "included_users",
		"excluded_users", "rules", "tags",
	} {
		attribute := byName[name]
		if attribute == nil || !attribute.Computed || attribute.Required || attribute.Optional {
			t.Fatalf("Protocol data source %s ownership is incorrect", name)
		}
	}
	rules := byName["rules"]
	if rules.NestedType == nil || len(rules.NestedType.Attributes) != 3 {
		t.Fatal("Protocol Segment rules schema is incomplete")
	}
	nestedRules := make(map[string]*tfprotov6.SchemaAttribute, 3)
	for _, attribute := range rules.NestedType.Attributes {
		if attribute != nil {
			nestedRules[attribute.Name] = attribute
		}
	}
	conditions := nestedRules["conditions"]
	if conditions == nil || conditions.NestedType == nil ||
		len(conditions.NestedType.Attributes) != 4 {
		t.Fatal("Protocol Segment conditions schema is incomplete")
	}
}

func protocolSegmentDataSourceConfig(
	t *testing.T,
	segmentSchema *tfprotov6.Schema,
	environmentID string,
	segmentID string,
) tfprotov6.DynamicValue {
	t.Helper()
	valueType, ok := segmentSchema.ValueType().(tftypes.Object)
	if !ok {
		t.Fatalf("Segment data source value type = %T, want object", segmentSchema.ValueType())
	}
	values := make(map[string]tftypes.Value, len(valueType.AttributeTypes))
	for name, attributeType := range valueType.AttributeTypes {
		switch name {
		case "environment_id":
			values[name] = tftypes.NewValue(attributeType, environmentID)
		case "id":
			values[name] = tftypes.NewValue(attributeType, segmentID)
		default:
			values[name] = tftypes.NewValue(attributeType, nil)
		}
	}
	terraformValue := tftypes.NewValue(valueType, values)
	dynamicValue, err := tfprotov6.NewDynamicValue(valueType, terraformValue)
	if err != nil {
		t.Fatalf("tfprotov6.NewDynamicValue() error = %v", err)
	}
	return dynamicValue
}
