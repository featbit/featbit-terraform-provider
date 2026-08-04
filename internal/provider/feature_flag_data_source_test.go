// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestFeatureFlagDataSourceMetadataSchemaAndConfigure(t *testing.T) {
	t.Parallel()

	dataSource := &featureFlagDataSource{}
	var metadataResponse datasource.MetadataResponse
	dataSource.Metadata(
		context.Background(),
		datasource.MetadataRequest{ProviderTypeName: "featbit"},
		&metadataResponse,
	)
	if metadataResponse.TypeName != "featbit_feature_flag" {
		t.Fatalf("type name = %q", metadataResponse.TypeName)
	}

	featureFlagSchema := featureFlagDataSourceTestSchema(t)
	if got := len(featureFlagSchema.Attributes); got != 7 {
		t.Fatalf("attribute count = %d, want 7", got)
	}
	for _, name := range []string{"environment_id", "key"} {
		attribute, ok := featureFlagSchema.Attributes[name].(datasourceschema.StringAttribute)
		if !ok || !attribute.Required || attribute.Optional || attribute.Computed ||
			len(attribute.Validators) != 1 {
			t.Fatalf("%s attribute = %#v", name, featureFlagSchema.Attributes[name])
		}
	}
	for _, name := range []string{"id", "name", "description", "variation_type"} {
		attribute, ok := featureFlagSchema.Attributes[name].(datasourceschema.StringAttribute)
		if !ok || !attribute.Computed || attribute.Required || attribute.Optional {
			t.Fatalf("%s attribute = %#v", name, featureFlagSchema.Attributes[name])
		}
	}
	variations, ok := featureFlagSchema.Attributes["variations"].(datasourceschema.ListNestedAttribute)
	if !ok || !variations.Computed || variations.Required || variations.Optional ||
		len(variations.NestedObject.Attributes) != 3 {
		t.Fatalf("variations attribute = %#v", featureFlagSchema.Attributes["variations"])
	}
	assertNoFeatureFlagOperationalSchemaFields(t, featureFlagSchema.Attributes)

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

func TestFeatureFlagDataSourceReadCanonicalizesSafeDefinition(t *testing.T) {
	t.Parallel()

	const (
		keyMarker        = "feature-flag-data-source-key"
		uiTagMarker      = "feature-flag-ui-tag-marker"
		uiTargetMarker   = "feature-flag-ui-target-marker"
		uiDispatchMarker = "feature-flag-ui-dispatch-marker"
	)
	var requestCount int
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			requestCount++
			if request.Method != http.MethodGet || request.URL.RawQuery != "" ||
				request.URL.EscapedPath() != "/api/v1/envs/"+providerEnvironmentA+
					"/feature-flags/"+keyMarker {
				t.Fatalf("unexpected exact request %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
			}
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
			writeProjectResourceEnvelope(
				t,
				response,
				http.StatusOK,
				`{"id":"`+providerFeatureFlagID+`","envId":"`+providerEnvironmentA+
					`","name":"Number Flag","key":"`+keyMarker+`",`+
					`"variationType":"number","variations":[`+
					`{"id":"`+providerFeatureVariationTwo+`","name":"Two","value":"2.00"},`+
					`{"id":"`+providerFeatureVariationOne+`","name":"One","value":"1e0"}],`+
					`"isArchived":false,"isEnabled":true,"tags":["`+uiTagMarker+`"],`+
					`"targetUsers":[{"keyIds":["`+uiTargetMarker+`"]}],`+
					`"rules":[{"name":"`+uiTargetMarker+`"}],`+
					`"fallthrough":{"dispatchKey":"`+uiDispatchMarker+`"}}`,
			)
		},
	))
	defer closeServer()

	featureFlagSchema := featureFlagDataSourceTestSchema(t)
	request := featureFlagDataSourceReadRequest(t, featureFlagSchema, providerEnvironmentA, keyMarker)
	response := datasource.ReadResponse{State: tfsdk.State{Schema: featureFlagSchema}}
	(&featureFlagDataSource{client: apiClient}).Read(context.Background(), request, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("Read() diagnostics = %v", response.Diagnostics)
	}
	var state featureFlagModel
	if diagnostics := response.State.Get(context.Background(), &state); diagnostics.HasError() {
		t.Fatalf("read Feature Flag data source state: %v", diagnostics)
	}
	if state.EnvironmentID.ValueString() != providerEnvironmentA ||
		state.ID.ValueString() != providerFeatureFlagID || state.Name.ValueString() != "Number Flag" ||
		state.Description.ValueString() != "" || state.Key.ValueString() != keyMarker ||
		state.VariationType.ValueString() != featureFlagVariationTypeNumber ||
		len(state.Variations) != 2 ||
		state.Variations[0].ID.ValueString() != providerFeatureVariationOne ||
		state.Variations[0].Value.ValueString() != "1" ||
		state.Variations[1].ID.ValueString() != providerFeatureVariationTwo ||
		state.Variations[1].Value.ValueString() != "2" {
		t.Fatal("Read() did not persist the UUID-sorted canonical definition")
	}
	formattedSchema := fmt.Sprintf("%v", featureFlagSchema)
	for _, unsafe := range []string{uiTagMarker, uiTargetMarker, uiDispatchMarker} {
		if strings.Contains(formattedSchema, unsafe) {
			t.Fatal("Feature Flag data source schema retained a UI-owned field value")
		}
	}
	if requestCount != 1 {
		t.Fatalf("Read() request count = %d, want 1", requestCount)
	}
}

func TestFeatureFlagDataSourceReadFailsClosedForArchivedAbsentAmbiguousAndInvalid(t *testing.T) {
	t.Parallel()

	const (
		keyMarker   = "feature-flag-read-outcome-key"
		valueMarker = "feature-flag-read-outcome-value"
	)
	tests := map[string]struct {
		handler func(*testing.T, http.ResponseWriter, *http.Request, int)
	}{
		"archived exact match": {
			handler: func(t *testing.T, response http.ResponseWriter, _ *http.Request, call int) {
				if call != 1 {
					t.Fatal("archived direct result unexpectedly used collection fallback")
				}
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerFeatureFlagExactJSON(keyMarker, true, providerFeatureVariationsJSON(valueMarker)),
				)
			},
		},
		"exact zero": {
			handler: func(t *testing.T, response http.ResponseWriter, _ *http.Request, call int) {
				switch call {
				case 1:
					writeProjectResourceEnvelope(t, response, http.StatusNotFound, "null")
				case 2, 3:
					writeProjectResourceEnvelope(t, response, http.StatusOK, `{"totalCount":0,"items":[]}`)
				default:
					t.Fatal("exact-zero fallback made an unexpected request")
				}
			},
		},
		"duplicate active exact keys": {
			handler: func(t *testing.T, response http.ResponseWriter, _ *http.Request, call int) {
				switch call {
				case 1:
					writeProjectResourceEnvelope(t, response, http.StatusNotFound, "null")
				case 2:
					first := providerFeatureFlagListItemJSON(
						providerFeatureFlagID,
						keyMarker,
						providerFeatureVariationsJSON(valueMarker),
					)
					second := providerFeatureFlagListItemJSON(
						"dddddddd-dddd-4ddd-8ddd-dddddddddddd",
						keyMarker,
						providerFeatureVariationsJSON(valueMarker),
					)
					writeProjectResourceEnvelope(
						t,
						response,
						http.StatusOK,
						`{"totalCount":2,"items":[`+first+`,`+second+`]}`,
					)
				case 3:
					writeProjectResourceEnvelope(t, response, http.StatusOK, `{"totalCount":0,"items":[]}`)
				default:
					t.Fatal("duplicate fallback made an unexpected request")
				}
			},
		},
		"missing variation ID": {
			handler: func(t *testing.T, response http.ResponseWriter, _ *http.Request, call int) {
				if call != 1 {
					t.Fatal("invalid direct definition unexpectedly used collection fallback")
				}
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerFeatureFlagExactJSON(
						keyMarker,
						false,
						`[{"id":"","name":"Missing","value":"`+valueMarker+`"}]`,
					),
				)
			},
		},
		"duplicate variation IDs": {
			handler: func(t *testing.T, response http.ResponseWriter, _ *http.Request, call int) {
				if call != 1 {
					t.Fatal("invalid direct definition unexpectedly used collection fallback")
				}
				variations := `[{"id":"` + providerFeatureVariationOne +
					`","name":"First","value":"` + valueMarker + `"},` +
					`{"id":"` + strings.ToUpper(providerFeatureVariationOne) +
					`","name":"Second","value":"` + valueMarker + `"}]`
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					providerFeatureFlagExactJSON(keyMarker, false, variations),
				)
			},
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
					test.handler(t, response, request, requestCount)
				},
			))
			defer closeServer()

			featureFlagSchema := featureFlagDataSourceTestSchema(t)
			request := featureFlagDataSourceReadRequest(
				t,
				featureFlagSchema,
				providerEnvironmentA,
				keyMarker,
			)
			prior := featureFlagPriorDataSourceState(t, featureFlagSchema, keyMarker)
			response := datasource.ReadResponse{State: prior}
			priorRaw := prior.Raw.Copy()
			(&featureFlagDataSource{client: apiClient}).Read(
				context.Background(),
				request,
				&response,
			)
			if !response.Diagnostics.HasError() {
				t.Fatal("unconfirmed Feature Flag data source read produced no error diagnostic")
			}
			if !response.State.Raw.Equal(priorRaw) {
				t.Fatal("unconfirmed Feature Flag data source read changed prior state")
			}
			formatted := fmt.Sprintf("%v", response.Diagnostics)
			for _, unsafe := range []string{
				providerEnvironmentA,
				providerFeatureFlagID,
				providerFeatureVariationOne,
				keyMarker,
				valueMarker,
			} {
				if strings.Contains(formatted, unsafe) {
					t.Fatal("Feature Flag read diagnostic exposed a runtime identity or value")
				}
			}
		})
	}
}

func TestFeatureFlagResourceSchemaFreezesOwnershipReplacementAndStableID(t *testing.T) {
	t.Parallel()

	featureFlagSchema := featureFlagResourceSchema()
	if got := len(featureFlagSchema.Attributes); got != 7 {
		t.Fatalf("attribute count = %d, want 7", got)
	}
	assertNoFeatureFlagOperationalSchemaFields(t, featureFlagSchema.Attributes)

	idAttribute, ok := featureFlagSchema.Attributes["id"].(resourceschema.StringAttribute)
	if !ok || !idAttribute.Computed || idAttribute.Required || idAttribute.Optional ||
		len(idAttribute.PlanModifiers) != 1 {
		t.Fatalf("id attribute = %#v", featureFlagSchema.Attributes["id"])
	}
	nameAttribute, ok := featureFlagSchema.Attributes["name"].(resourceschema.StringAttribute)
	if !ok || !nameAttribute.Required || nameAttribute.Optional || nameAttribute.Computed ||
		len(nameAttribute.PlanModifiers) != 0 || len(nameAttribute.Validators) != 1 {
		t.Fatalf("name attribute = %#v", featureFlagSchema.Attributes["name"])
	}
	for _, name := range []string{"environment_id", "key", "variation_type"} {
		attribute, ok := featureFlagSchema.Attributes[name].(resourceschema.StringAttribute)
		if !ok || !attribute.Required || attribute.Optional || attribute.Computed ||
			len(attribute.PlanModifiers) != 1 || len(attribute.Validators) != 1 {
			t.Fatalf("%s attribute = %#v", name, featureFlagSchema.Attributes[name])
		}
	}
	description, ok := featureFlagSchema.Attributes["description"].(resourceschema.StringAttribute)
	if !ok || !description.Optional || !description.Computed || description.Required ||
		description.Default == nil || len(description.PlanModifiers) != 1 {
		t.Fatalf("description attribute = %#v", featureFlagSchema.Attributes["description"])
	}
	variations, ok := featureFlagSchema.Attributes["variations"].(resourceschema.ListNestedAttribute)
	if !ok || !variations.Required || variations.Optional || variations.Computed ||
		len(variations.PlanModifiers) != 1 || len(variations.Validators) != 1 ||
		len(variations.NestedObject.Attributes) != 3 {
		t.Fatalf("variations attribute = %#v", featureFlagSchema.Attributes["variations"])
	}
	variationID, ok := variations.NestedObject.Attributes["id"].(resourceschema.StringAttribute)
	if !ok || !variationID.Computed || variationID.Required || variationID.Optional {
		t.Fatalf("variation id attribute = %#v", variations.NestedObject.Attributes["id"])
	}
	for _, name := range []string{"name", "value"} {
		attribute, ok := variations.NestedObject.Attributes[name].(resourceschema.StringAttribute)
		if !ok || !attribute.Required || attribute.Optional || attribute.Computed ||
			len(attribute.Validators) != 1 {
			t.Fatalf("variation %s attribute = %#v", name, variations.NestedObject.Attributes[name])
		}
	}

	stateModel := providerFeatureFlagResourceModel("Original", "stable-key", "", "one")
	stateModel.ID = types.StringValue(providerFeatureFlagID)
	planModel := providerFeatureFlagResourceModel("Renamed", "stable-key", "", "one")
	planModel.ID = types.StringUnknown()
	state := featureFlagResourceTestState(t, featureFlagSchema, stateModel)
	plan := featureFlagResourceTestPlan(t, featureFlagSchema, planModel)
	var stableResponse planmodifier.StringResponse
	stableResponse.PlanValue = types.StringUnknown()
	idAttribute.PlanModifiers[0].PlanModifyString(
		context.Background(),
		planmodifier.StringRequest{
			PlanValue:  types.StringUnknown(),
			StateValue: types.StringValue(providerFeatureFlagID),
			Plan:       plan,
			State:      state,
		},
		&stableResponse,
	)
	if stableResponse.Diagnostics.HasError() ||
		!stableResponse.PlanValue.Equal(types.StringValue(providerFeatureFlagID)) {
		t.Fatalf("name-only stable ID response = %#v", stableResponse)
	}

	replacements := map[string]func(*featureFlagModel){
		"environment_id": func(model *featureFlagModel) {
			model.EnvironmentID = types.StringValue(providerEnvironmentB)
		},
		"key": func(model *featureFlagModel) {
			model.Key = types.StringValue("replacement-key")
		},
		"variation_type": func(model *featureFlagModel) {
			model.VariationType = types.StringValue(featureFlagVariationTypeJSON)
		},
		"description": func(model *featureFlagModel) {
			model.Description = types.StringValue("replacement")
		},
		"variations": func(model *featureFlagModel) {
			model.Variations[0].Value = types.StringValue("replacement")
		},
	}
	for name, change := range replacements {
		name := name
		change := change
		t.Run("stable ID rejects "+name+" replacement", func(t *testing.T) {
			t.Parallel()

			replacement := providerFeatureFlagResourceModel("Original", "stable-key", "", "one")
			replacement.ID = types.StringUnknown()
			change(&replacement)
			var response planmodifier.StringResponse
			response.PlanValue = types.StringUnknown()
			idAttribute.PlanModifiers[0].PlanModifyString(
				context.Background(),
				planmodifier.StringRequest{
					PlanValue:  types.StringUnknown(),
					StateValue: types.StringValue(providerFeatureFlagID),
					Plan: featureFlagResourceTestPlan(
						t,
						featureFlagSchema,
						replacement,
					),
					State: state,
				},
				&response,
			)
			if response.Diagnostics.HasError() || !response.PlanValue.IsUnknown() {
				t.Fatalf("replacement stable ID response = %#v", response)
			}
		})
	}

	for _, name := range []string{"environment_id", "key", "variation_type", "description"} {
		attribute := featureFlagSchema.Attributes[name].(resourceschema.StringAttribute)
		var response planmodifier.StringResponse
		attribute.PlanModifiers[0].PlanModifyString(
			context.Background(),
			planmodifier.StringRequest{
				ConfigValue: types.StringValue("new"),
				PlanValue:   types.StringValue("new"),
				StateValue:  types.StringValue("old"),
				Plan:        plan,
				State:       state,
			},
			&response,
		)
		if !response.RequiresReplace {
			t.Fatalf("%s plan modifier did not require replacement", name)
		}
	}
	var plannedVariations types.List
	var priorVariations types.List
	changedVariationModel := providerFeatureFlagResourceModel("Original", "stable-key", "", "changed")
	changedVariationModel.ID = types.StringUnknown()
	changedPlan := featureFlagResourceTestPlan(t, featureFlagSchema, changedVariationModel)
	if diagnostics := changedPlan.GetAttribute(context.Background(), path.Root("variations"), &plannedVariations); diagnostics.HasError() {
		t.Fatalf("read planned variations: %v", diagnostics)
	}
	if diagnostics := state.GetAttribute(context.Background(), path.Root("variations"), &priorVariations); diagnostics.HasError() {
		t.Fatalf("read prior variations: %v", diagnostics)
	}
	var listResponse planmodifier.ListResponse
	variations.PlanModifiers[0].PlanModifyList(
		context.Background(),
		planmodifier.ListRequest{
			ConfigValue: plannedVariations,
			PlanValue:   plannedVariations,
			StateValue:  priorVariations,
			Plan:        changedPlan,
			State:       state,
		},
		&listResponse,
	)
	if !listResponse.RequiresReplace {
		t.Fatal("variations plan modifier did not require replacement")
	}
}

func TestFeatureFlagProtocolSchemasAndValidation(t *testing.T) {
	t.Parallel()

	productionServer := providerserver.NewProtocol6(New("test")())()
	productionSchema, err := productionServer.GetProviderSchema(
		context.Background(),
		&tfprotov6.GetProviderSchemaRequest{},
	)
	if err != nil || protocolHasError(productionSchema.Diagnostics) {
		t.Fatalf("production GetProviderSchema() failed: %v / %v", err, productionSchema.Diagnostics)
	}
	if len(productionSchema.ResourceSchemas) != 3 {
		t.Fatalf("production resource schema count = %d, want 3", len(productionSchema.ResourceSchemas))
	}
	if len(productionSchema.DataSourceSchemas) != 4 {
		t.Fatalf("production data source schema count = %d, want 4", len(productionSchema.DataSourceSchemas))
	}
	dataSourceSchema := productionSchema.DataSourceSchemas["featbit_feature_flag"]
	if dataSourceSchema == nil {
		t.Fatal("Protocol schema omitted featbit_feature_flag data source")
	}
	assertProtocolFeatureFlagAttributes(t, dataSourceSchema.Block.Attributes, true)

	schemaServer := productionServer
	resourceSchema := productionSchema.ResourceSchemas["featbit_feature_flag"]
	if resourceSchema == nil {
		t.Fatal("production Protocol provider omitted featbit_feature_flag resource")
	}
	assertProtocolFeatureFlagAttributes(t, resourceSchema.Block.Attributes, false)

	validConfigurations := map[string]struct {
		variationType string
		value         string
	}{
		"boolean": {variationType: featureFlagVariationTypeBoolean, value: "TRUE"},
		"string":  {variationType: featureFlagVariationTypeString, value: " exact "},
		"number":  {variationType: featureFlagVariationTypeNumber, value: "90071992547409931234567890"},
		"json":    {variationType: featureFlagVariationTypeJSON, value: `{"b":2,"a":1}`},
	}
	for name, configuration := range validConfigurations {
		name := name
		configuration := configuration
		t.Run("valid "+name, func(t *testing.T) {
			t.Parallel()

			config := protocolFeatureFlagResourceConfig(
				t,
				resourceSchema,
				configuration.variationType,
				[]string{configuration.value},
			)
			response, err := schemaServer.ValidateResourceConfig(
				context.Background(),
				&tfprotov6.ValidateResourceConfigRequest{
					TypeName: "featbit_feature_flag",
					Config:   &config,
				},
			)
			if err != nil || protocolHasError(response.Diagnostics) {
				t.Fatalf("valid Protocol resource config failed: %v / %v", err, response.Diagnostics)
			}
		})
	}

	invalidConfigurations := map[string]struct {
		variationType string
		values        []string
	}{
		"non-canonical type":  {variationType: "Boolean", values: []string{"true"}},
		"invalid typed value": {variationType: featureFlagVariationTypeNumber, values: []string{"NaN"}},
		"empty variations":    {variationType: featureFlagVariationTypeString, values: []string{}},
	}
	for name, configuration := range invalidConfigurations {
		name := name
		configuration := configuration
		t.Run("invalid "+name, func(t *testing.T) {
			t.Parallel()

			config := protocolFeatureFlagResourceConfig(
				t,
				resourceSchema,
				configuration.variationType,
				configuration.values,
			)
			response, err := schemaServer.ValidateResourceConfig(
				context.Background(),
				&tfprotov6.ValidateResourceConfigRequest{
					TypeName: "featbit_feature_flag",
					Config:   &config,
				},
			)
			if err != nil {
				t.Fatalf("ValidateResourceConfig() error = %v", err)
			}
			if !protocolHasError(response.Diagnostics) {
				t.Fatal("invalid Protocol resource config produced no error diagnostic")
			}
		})
	}
}

func featureFlagDataSourceTestSchema(t *testing.T) datasourceschema.Schema {
	t.Helper()
	var response datasource.SchemaResponse
	(&featureFlagDataSource{}).Schema(
		context.Background(),
		datasource.SchemaRequest{},
		&response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Feature Flag data source schema diagnostics = %v", response.Diagnostics)
	}
	return response.Schema
}

func featureFlagDataSourceReadRequest(
	t *testing.T,
	featureFlagSchema datasourceschema.Schema,
	environmentID string,
	key string,
) datasource.ReadRequest {
	t.Helper()
	configured := featureFlagModel{
		EnvironmentID: types.StringValue(environmentID),
		ID:            types.StringNull(),
		Name:          types.StringNull(),
		Description:   types.StringNull(),
		Key:           types.StringValue(key),
		VariationType: types.StringNull(),
		Variations:    nil,
	}
	configuredState := tfsdk.State{Schema: featureFlagSchema}
	if diagnostics := configuredState.Set(context.Background(), &configured); diagnostics.HasError() {
		t.Fatalf("initialize Feature Flag data source config: %v", diagnostics)
	}
	return datasource.ReadRequest{
		Config: tfsdk.Config{Schema: featureFlagSchema, Raw: configuredState.Raw},
	}
}

func featureFlagPriorDataSourceState(
	t *testing.T,
	featureFlagSchema datasourceschema.Schema,
	key string,
) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: featureFlagSchema}
	model := featureFlagModel{
		EnvironmentID: types.StringValue(providerEnvironmentA),
		ID:            types.StringValue(providerFeatureFlagID),
		Name:          types.StringValue("Prior"),
		Description:   types.StringValue(""),
		Key:           types.StringValue(key),
		VariationType: types.StringValue(featureFlagVariationTypeString),
		Variations: []featureFlagVariationModel{
			{
				ID:    types.StringValue(providerFeatureVariationOne),
				Name:  types.StringValue("Prior"),
				Value: types.StringValue("prior"),
			},
		},
	}
	if diagnostics := state.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize Feature Flag data source prior state: %v", diagnostics)
	}
	return state
}

func providerFeatureFlagExactJSON(key string, archived bool, variations string) string {
	return `{"id":"` + providerFeatureFlagID + `","envId":"` + providerEnvironmentA +
		`","name":"Feature Flag","description":"","key":"` + key +
		`","variationType":"string","variations":` + variations +
		`,"isArchived":` + fmt.Sprintf("%t", archived) + `}`
}

func providerFeatureFlagListItemJSON(id, key, variations string) string {
	return `{"id":"` + id + `","name":"Feature Flag","description":"","key":"` + key +
		`","variationType":"string","variations":` + variations + `}`
}

func providerFeatureVariationsJSON(value string) string {
	return `[{"id":"` + providerFeatureVariationOne +
		`","name":"First","value":"` + value + `"}]`
}

func assertNoFeatureFlagOperationalSchemaFields(t *testing.T, attributes any) {
	t.Helper()
	formatted := fmt.Sprintf("%v", attributes)
	for _, name := range []string{
		"is_enabled",
		"enabled_variation_id",
		"disabled_variation_id",
		"target_users",
		"targeting",
		"rules",
		"rollouts",
		"fallthrough",
		"tags",
		"is_archived",
	} {
		if strings.Contains(formatted, name) {
			t.Fatalf("schema exposed UI-owned operational field %q", name)
		}
	}
}

func providerFeatureFlagResourceModel(name, key, description, value string) featureFlagModel {
	return featureFlagModel{
		EnvironmentID: types.StringValue(providerEnvironmentA),
		ID:            types.StringUnknown(),
		Name:          types.StringValue(name),
		Description:   types.StringValue(description),
		Key:           types.StringValue(key),
		VariationType: types.StringValue(featureFlagVariationTypeString),
		Variations: []featureFlagVariationModel{
			{
				ID:    types.StringValue(providerFeatureVariationOne),
				Name:  types.StringValue("One"),
				Value: types.StringValue(value),
			},
		},
	}
}

func featureFlagResourceTestState(
	t *testing.T,
	featureFlagSchema resourceschema.Schema,
	model featureFlagModel,
) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: featureFlagSchema}
	if diagnostics := state.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize Feature Flag resource state: %v", diagnostics)
	}
	return state
}

func featureFlagResourceTestPlan(
	t *testing.T,
	featureFlagSchema resourceschema.Schema,
	model featureFlagModel,
) tfsdk.Plan {
	t.Helper()
	state := featureFlagResourceTestState(t, featureFlagSchema, model)
	return tfsdk.Plan{Schema: featureFlagSchema, Raw: state.Raw}
}

func assertProtocolFeatureFlagAttributes(
	t *testing.T,
	attributes []*tfprotov6.SchemaAttribute,
	dataSource bool,
) {
	t.Helper()
	byName := make(map[string]*tfprotov6.SchemaAttribute, len(attributes))
	for _, attribute := range attributes {
		if attribute != nil {
			byName[attribute.Name] = attribute
		}
	}
	if len(byName) != 7 {
		t.Fatalf("Protocol Feature Flag attribute count = %d, want 7", len(byName))
	}
	for _, name := range []string{"environment_id", "key"} {
		attribute := byName[name]
		if attribute == nil || !attribute.Required || attribute.Optional || attribute.Computed {
			t.Fatalf("Protocol %s ownership is incorrect", name)
		}
	}
	if dataSource {
		for _, name := range []string{"id", "name", "description", "variation_type", "variations"} {
			attribute := byName[name]
			if attribute == nil || !attribute.Computed || attribute.Required || attribute.Optional {
				t.Fatalf("Protocol data source %s ownership is incorrect", name)
			}
		}
	} else {
		if attribute := byName["id"]; attribute == nil || !attribute.Computed ||
			attribute.Required || attribute.Optional {
			t.Fatal("Protocol resource id ownership is incorrect")
		}
		for _, name := range []string{"name", "variation_type", "variations"} {
			attribute := byName[name]
			if attribute == nil || !attribute.Required || attribute.Optional || attribute.Computed {
				t.Fatalf("Protocol resource %s ownership is incorrect", name)
			}
		}
		if attribute := byName["description"]; attribute == nil || !attribute.Optional ||
			!attribute.Computed || attribute.Required {
			t.Fatal("Protocol resource description ownership is incorrect")
		}
	}
	variationAttribute := byName["variations"]
	if variationAttribute == nil || variationAttribute.NestedType == nil ||
		len(variationAttribute.NestedType.Attributes) != 3 {
		t.Fatal("Protocol variations nested schema is incomplete")
	}
	nested := make(map[string]*tfprotov6.SchemaAttribute, 3)
	for _, attribute := range variationAttribute.NestedType.Attributes {
		if attribute != nil {
			nested[attribute.Name] = attribute
		}
	}
	if dataSource {
		for _, name := range []string{"id", "name", "value"} {
			attribute := nested[name]
			if attribute == nil || !attribute.Computed || attribute.Required || attribute.Optional {
				t.Fatalf("Protocol data source variation %s ownership is incorrect", name)
			}
		}
	} else {
		if attribute := nested["id"]; attribute == nil || !attribute.Computed ||
			attribute.Required || attribute.Optional {
			t.Fatal("Protocol resource variation id ownership is incorrect")
		}
		for _, name := range []string{"name", "value"} {
			attribute := nested[name]
			if attribute == nil || !attribute.Required || attribute.Optional || attribute.Computed {
				t.Fatalf("Protocol resource variation %s ownership is incorrect", name)
			}
		}
	}
	assertNoFeatureFlagOperationalSchemaFields(t, byName)
}

func protocolFeatureFlagResourceConfig(
	t *testing.T,
	resourceSchema *tfprotov6.Schema,
	variationType string,
	variationValues []string,
) tfprotov6.DynamicValue {
	t.Helper()
	valueType, ok := resourceSchema.ValueType().(tftypes.Object)
	if !ok {
		t.Fatalf("Feature Flag resource value type = %T, want object", resourceSchema.ValueType())
	}
	values := make(map[string]tftypes.Value, len(valueType.AttributeTypes))
	for name, attributeType := range valueType.AttributeTypes {
		switch name {
		case "environment_id":
			values[name] = tftypes.NewValue(attributeType, providerEnvironmentA)
		case "id":
			values[name] = tftypes.NewValue(attributeType, nil)
		case "name":
			values[name] = tftypes.NewValue(attributeType, "Feature Flag")
		case "description":
			values[name] = tftypes.NewValue(attributeType, nil)
		case "key":
			values[name] = tftypes.NewValue(attributeType, "protocol-key")
		case "variation_type":
			values[name] = tftypes.NewValue(attributeType, variationType)
		case "variations":
			listType, ok := attributeType.(tftypes.List)
			if !ok {
				t.Fatalf("variations type = %T, want list", attributeType)
			}
			elementType, ok := listType.ElementType.(tftypes.Object)
			if !ok {
				t.Fatalf("variation element type = %T, want object", listType.ElementType)
			}
			elements := make([]tftypes.Value, 0, len(variationValues))
			for index, value := range variationValues {
				elementValues := make(map[string]tftypes.Value, len(elementType.AttributeTypes))
				for elementName, elementAttributeType := range elementType.AttributeTypes {
					switch elementName {
					case "id":
						elementValues[elementName] = tftypes.NewValue(elementAttributeType, nil)
					case "name":
						elementValues[elementName] = tftypes.NewValue(
							elementAttributeType,
							fmt.Sprintf("Variation %d", index+1),
						)
					case "value":
						elementValues[elementName] = tftypes.NewValue(elementAttributeType, value)
					}
				}
				elements = append(elements, tftypes.NewValue(elementType, elementValues))
			}
			values[name] = tftypes.NewValue(listType, elements)
		}
	}
	terraformValue := tftypes.NewValue(valueType, values)
	dynamicValue, err := tfprotov6.NewDynamicValue(valueType, terraformValue)
	if err != nil {
		t.Fatalf("tfprotov6.NewDynamicValue() error = %v", err)
	}
	return dynamicValue
}
