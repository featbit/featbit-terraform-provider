// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const providerFeatureFlagSecondID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"

func TestFeatureFlagResourceMetadataSchemaAndConfigure(t *testing.T) {
	t.Parallel()

	resourceUnderTest := &featureFlagResource{}
	var metadataResponse frameworkresource.MetadataResponse
	resourceUnderTest.Metadata(
		context.Background(),
		frameworkresource.MetadataRequest{ProviderTypeName: "featbit"},
		&metadataResponse,
	)
	if metadataResponse.TypeName != "featbit_feature_flag" {
		t.Fatalf("type name = %q", metadataResponse.TypeName)
	}

	var schemaResponse frameworkresource.SchemaResponse
	resourceUnderTest.Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		&schemaResponse,
	)
	if schemaResponse.Diagnostics.HasError() || len(schemaResponse.Schema.Attributes) != 7 {
		t.Fatalf("Schema() = %d attributes / %v", len(schemaResponse.Schema.Attributes), schemaResponse.Diagnostics)
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

func TestFeatureFlagResourceCreateAllTypesUsesCompletePreflightAndCanonicalRead(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		variationType string
		values        []string
		wantValues    []string
	}{
		"boolean": {
			variationType: featureFlagVariationTypeBoolean,
			values:        []string{"TRUE", "False"},
			wantValues:    []string{"true", "false"},
		},
		"string": {
			variationType: featureFlagVariationTypeString,
			values:        []string{" exact string ", "second"},
			wantValues:    []string{" exact string ", "second"},
		},
		"number": {
			variationType: featureFlagVariationTypeNumber,
			values:        []string{"1.00e2", "90071992547409931234567890.00"},
			wantValues:    []string{"100", "90071992547409931234567890"},
		},
		"json": {
			variationType: featureFlagVariationTypeJSON,
			values:        []string{`{"b":2,"a":1}`, `[3, 2, 1]`},
			wantValues:    []string{`{"a":1,"b":2}`, `[3,2,1]`},
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			key := "create-" + name
			planModel := featureFlagResourcePlanModel(
				providerEnvironmentA,
				key,
				"Created "+name,
				"Definition",
				test.variationType,
				test.values,
			)
			planned, seed, err := canonicalizeFeatureFlagPlanModel(planModel)
			if err != nil {
				t.Fatalf("canonicalize test plan: %v", err)
			}
			wantIDs := canonicalFeatureFlagVariationIDs(planned)

			var calls atomic.Int32
			var posts atomic.Int32
			apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
				func(response http.ResponseWriter, request *http.Request) {
					call := calls.Add(1)
					assertFeatureFlagResourceRequestBoundary(t, request)
					switch {
					case request.Method == http.MethodGet && request.URL.RawQuery != "":
						archived := request.URL.Query().Get("IsArchived") == "true"
						if call == 1 && archived || call == 2 && !archived {
							t.Fatal("create preflight did not read active then archived views")
						}
						items := []string{}
						if !archived {
							items = []string{featureFlagResourceListItemJSON(
								providerFeatureFlagSecondID,
								strings.ToUpper(key),
							)}
						}
						writeProjectResourceEnvelope(
							t,
							response,
							http.StatusOK,
							featureFlagResourcePageJSON(int64(len(items)), items),
						)
					case request.Method == http.MethodPost:
						posts.Add(1)
						if call != 3 || request.URL.EscapedPath() != "/api/v1/envs/"+
							providerEnvironmentA+"/feature-flags" {
							t.Fatalf("unexpected create call %d at %s", call, request.URL.EscapedPath())
						}
						var payload client.CreateFeatureFlagRequest
						if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
							t.Fatalf("decode create payload: %v", err)
						}
						if payload.Name != planned.Name || payload.Key != key || payload.IsEnabled ||
							payload.Description != planned.Description ||
							payload.VariationType != test.variationType || payload.Tags == nil ||
							len(payload.Tags) != 0 || len(payload.Variations) != len(wantIDs) ||
							payload.EnabledVariationID != wantIDs[0] ||
							payload.DisabledVariationID != wantIDs[0] {
							t.Fatal("Create sent a non-minimal or non-deterministic payload")
						}
						for index, variation := range payload.Variations {
							if variation.ID != wantIDs[index] || variation.Value != test.wantValues[index] {
								t.Fatal("Create did not send canonical values with stable UUIDs")
							}
						}
						writeProjectResourceEnvelope(
							t,
							response,
							http.StatusOK,
							featureFlagResourceDefinitionJSON(
								t,
								planned,
								providerFeatureFlagID,
								false,
								true,
							),
						)
					case request.Method == http.MethodGet && request.URL.RawQuery == "":
						if call != 4 || request.URL.EscapedPath() != "/api/v1/envs/"+
							providerEnvironmentA+"/feature-flags/"+key {
							t.Fatalf("unexpected canonical read %d at %s", call, request.URL.EscapedPath())
						}
						writeProjectResourceEnvelope(
							t,
							response,
							http.StatusOK,
							featureFlagResourceDefinitionJSON(
								t,
								planned,
								providerFeatureFlagID,
								false,
								true,
							),
						)
					default:
						t.Fatalf("unexpected Feature Flag request %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
					}
				},
			))
			defer closeServer()

			featureFlagSchema := featureFlagResourceSchema()
			response := frameworkresource.CreateResponse{
				State: emptyFeatureFlagResourceState(t, featureFlagSchema),
			}
			(&featureFlagResource{client: apiClient}).Create(
				context.Background(),
				frameworkresource.CreateRequest{
					Plan: featureFlagResourceTestPlan(t, featureFlagSchema, planModel),
				},
				&response,
			)
			if response.Diagnostics.HasError() {
				t.Fatalf("Create() diagnostics = %v", response.Diagnostics)
			}
			state := featureFlagResourceStateModel(t, response.State)
			if state.ID.ValueString() != providerFeatureFlagID ||
				state.EnvironmentID.ValueString() != providerEnvironmentA ||
				state.Key.ValueString() != key || state.VariationType.ValueString() != test.variationType ||
				len(state.Variations) != len(wantIDs) {
				t.Fatal("Create did not persist the canonical server definition")
			}
			for index, variation := range state.Variations {
				if variation.ID.ValueString() != wantIDs[index] ||
					variation.Value.ValueString() != test.values[index] {
					t.Fatal("Create state did not correlate reordered variations by UUID")
				}
			}
			if calls.Load() != 4 || posts.Load() != 1 ||
				seed.EnabledVariationID != wantIDs[0] {
				t.Fatal("Create did not use the expected preflight/mutation/read sequence")
			}
		})
	}
}

func TestFeatureFlagResourceCreateRejectsInvalidPlanBeforeTransport(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*featureFlagModel){
		"invalid environment": func(model *featureFlagModel) {
			model.EnvironmentID = types.StringValue("not-a-uuid")
		},
		"invalid key": func(model *featureFlagModel) {
			model.Key = types.StringValue("invalid/key")
		},
		"invalid type": func(model *featureFlagModel) {
			model.VariationType = types.StringValue("Boolean")
		},
		"invalid value": func(model *featureFlagModel) {
			model.Variations[0].Value = types.StringValue("not-boolean")
		},
		"unknown value": func(model *featureFlagModel) {
			model.Variations[0].Value = types.StringUnknown()
		},
	}
	for name, mutate := range tests {
		name := name
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
				func(http.ResponseWriter, *http.Request) { calls.Add(1) },
			))
			defer closeServer()
			model := featureFlagResourcePlanModel(
				providerEnvironmentA,
				"invalid-plan",
				"Invalid Plan",
				"",
				featureFlagVariationTypeBoolean,
				[]string{"true"},
			)
			mutate(&model)
			featureFlagSchema := featureFlagResourceSchema()
			response := frameworkresource.CreateResponse{
				State: emptyFeatureFlagResourceState(t, featureFlagSchema),
			}
			(&featureFlagResource{client: apiClient}).Create(
				context.Background(),
				frameworkresource.CreateRequest{
					Plan: featureFlagResourceTestPlan(t, featureFlagSchema, model),
				},
				&response,
			)
			if !response.Diagnostics.HasError() || calls.Load() != 0 {
				t.Fatalf("invalid Create diagnostics/calls = %v/%d", response.Diagnostics, calls.Load())
			}
		})
	}
}

func TestFeatureFlagResourceCreatePreflightFailsClosedForCollisionsAndDuplicates(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		active   []string
		archived []string
	}{
		"active collision": {
			active: []string{featureFlagResourceListItemJSON(providerFeatureFlagID, "collision")},
		},
		"archived collision": {
			archived: []string{featureFlagResourceListItemJSON(providerFeatureFlagID, "collision")},
		},
		"duplicate active": {
			active: []string{
				featureFlagResourceListItemJSON(providerFeatureFlagID, "collision"),
				featureFlagResourceListItemJSON(providerFeatureFlagSecondID, "collision"),
			},
		},
		"same key in both views": {
			active:   []string{featureFlagResourceListItemJSON(providerFeatureFlagID, "collision")},
			archived: []string{featureFlagResourceListItemJSON(providerFeatureFlagSecondID, "collision")},
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var posts atomic.Int32
			apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
				func(response http.ResponseWriter, request *http.Request) {
					if request.Method == http.MethodPost {
						posts.Add(1)
						t.Fatal("preflight collision reached Create")
					}
					archived := request.URL.Query().Get("IsArchived") == "true"
					items := test.active
					if archived {
						items = test.archived
					}
					writeProjectResourceEnvelope(
						t,
						response,
						http.StatusOK,
						featureFlagResourcePageJSON(int64(len(items)), items),
					)
				},
			))
			defer closeServer()
			featureFlagSchema := featureFlagResourceSchema()
			response := frameworkresource.CreateResponse{
				State: emptyFeatureFlagResourceState(t, featureFlagSchema),
			}
			(&featureFlagResource{client: apiClient}).Create(
				context.Background(),
				frameworkresource.CreateRequest{Plan: featureFlagResourceTestPlan(
					t,
					featureFlagSchema,
					featureFlagResourcePlanModel(
						providerEnvironmentA,
						"collision",
						"Collision",
						"",
						featureFlagVariationTypeString,
						[]string{"value"},
					),
				)},
				&response,
			)
			if !response.Diagnostics.HasError() || posts.Load() != 0 {
				t.Fatal("preflight collision did not fail closed before mutation")
			}
		})
	}
}

func TestFeatureFlagResourceAmbiguousCreateReconcilesWithoutRetryOrAdoption(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		recoveryActive   []string
		recoveryArchived []string
		incomplete       bool
	}{
		"exact zero": {},
		"one active": {
			recoveryActive: []string{featureFlagResourceListItemJSON(providerFeatureFlagID, "ambiguous-create")},
		},
		"one archived": {
			recoveryArchived: []string{featureFlagResourceListItemJSON(providerFeatureFlagID, "ambiguous-create")},
		},
		"duplicates": {
			recoveryActive: []string{
				featureFlagResourceListItemJSON(providerFeatureFlagID, "ambiguous-create"),
				featureFlagResourceListItemJSON(providerFeatureFlagSecondID, "ambiguous-create"),
			},
		},
		"incomplete recovery": {incomplete: true},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var collectionCalls atomic.Int32
			var posts atomic.Int32
			apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
				func(response http.ResponseWriter, request *http.Request) {
					if request.Method == http.MethodPost {
						posts.Add(1)
						writeProjectResourceEnvelope(t, response, http.StatusServiceUnavailable, "null")
						return
					}
					collectionCall := collectionCalls.Add(1)
					archived := request.URL.Query().Get("IsArchived") == "true"
					if collectionCall <= 2 {
						writeProjectResourceEnvelope(
							t,
							response,
							http.StatusOK,
							featureFlagResourcePageJSON(0, []string{}),
						)
						return
					}
					if test.incomplete && !archived {
						writeProjectResourceEnvelope(
							t,
							response,
							http.StatusOK,
							featureFlagResourcePageJSON(1, []string{}),
						)
						return
					}
					items := test.recoveryActive
					if archived {
						items = test.recoveryArchived
					}
					writeProjectResourceEnvelope(
						t,
						response,
						http.StatusOK,
						featureFlagResourcePageJSON(int64(len(items)), items),
					)
				},
			))
			defer closeServer()
			featureFlagSchema := featureFlagResourceSchema()
			response := frameworkresource.CreateResponse{
				State: emptyFeatureFlagResourceState(t, featureFlagSchema),
			}
			(&featureFlagResource{client: apiClient}).Create(
				context.Background(),
				frameworkresource.CreateRequest{Plan: featureFlagResourceTestPlan(
					t,
					featureFlagSchema,
					featureFlagResourcePlanModel(
						providerEnvironmentA,
						"ambiguous-create",
						"Ambiguous",
						"",
						featureFlagVariationTypeString,
						[]string{"value"},
					),
				)},
				&response,
			)
			if !response.Diagnostics.HasError() || posts.Load() != 1 {
				t.Fatalf("ambiguous create diagnostics/posts = %v/%d", response.Diagnostics, posts.Load())
			}
			state := featureFlagResourceStateModel(t, response.State)
			if !state.ID.IsNull() || !state.Name.IsNull() || len(state.Variations) != 0 {
				t.Fatal("ambiguous Create silently adopted or persisted an unconfirmed object")
			}
		})
	}
}

func TestFeatureFlagResourceCreateIdentityMismatchPreservesMutationIdentity(t *testing.T) {
	t.Parallel()

	model := featureFlagResourcePlanModel(
		providerEnvironmentA,
		"identity-mismatch",
		"Identity Mismatch",
		"",
		featureFlagVariationTypeString,
		[]string{"value"},
	)
	planned, _, err := canonicalizeFeatureFlagPlanModel(model)
	if err != nil {
		t.Fatalf("canonicalize test plan: %v", err)
	}
	var calls atomic.Int32
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			call := calls.Add(1)
			switch call {
			case 1, 2:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					featureFlagResourcePageJSON(0, []string{}),
				)
			case 3:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					featureFlagResourceDefinitionJSON(t, planned, providerFeatureFlagID, false, false),
				)
			case 4:
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					featureFlagResourceDefinitionJSON(t, planned, providerFeatureFlagSecondID, false, false),
				)
			default:
				t.Fatal("identity mismatch triggered an unexpected request")
			}
		},
	))
	defer closeServer()
	featureFlagSchema := featureFlagResourceSchema()
	response := frameworkresource.CreateResponse{
		State: emptyFeatureFlagResourceState(t, featureFlagSchema),
	}
	(&featureFlagResource{client: apiClient}).Create(
		context.Background(),
		frameworkresource.CreateRequest{
			Plan: featureFlagResourceTestPlan(t, featureFlagSchema, model),
		},
		&response,
	)
	if !response.Diagnostics.HasError() {
		t.Fatal("create/read identity mismatch produced no diagnostic")
	}
	state := featureFlagResourceStateModel(t, response.State)
	if state.ID.ValueString() != providerFeatureFlagID {
		t.Fatal("identity mismatch did not preserve the mutation-confirmed identity")
	}
}

func TestFeatureFlagResourceImportAndReadRetainServerVariationIDs(t *testing.T) {
	t.Parallel()

	deterministicKey := ""
	for index := 0; index < 100; index++ {
		candidate := fmt.Sprintf("imported-deterministic-%d", index)
		if deterministicFeatureFlagVariationID(providerEnvironmentA, candidate, 0) >
			deterministicFeatureFlagVariationID(providerEnvironmentA, candidate, 1) {
			deterministicKey = candidate
			break
		}
	}
	if deterministicKey == "" {
		t.Fatal("could not construct a deterministic non-lexical variation order")
	}
	deterministicFirstID := deterministicFeatureFlagVariationID(
		providerEnvironmentA,
		deterministicKey,
		0,
	)
	deterministicSecondID := deterministicFeatureFlagVariationID(
		providerEnvironmentA,
		deterministicKey,
		1,
	)

	tests := []struct {
		name       string
		key        string
		variations []canonicalFeatureFlagVariation
		wantIDs    []string
		wantNames  []string
	}{
		{
			name: "external IDs use canonical UUID order",
			key:  "imported-key",
			variations: []canonicalFeatureFlagVariation{
				{ID: providerFeatureVariationTwo, Name: "Two", Value: "2"},
				{ID: providerFeatureVariationOne, Name: "One", Value: "1"},
			},
			wantIDs:   []string{providerFeatureVariationOne, providerFeatureVariationTwo},
			wantNames: []string{"One", "Two"},
		},
		{
			name: "provider IDs recover configured index order",
			key:  deterministicKey,
			variations: []canonicalFeatureFlagVariation{
				{ID: deterministicSecondID, Name: "Second", Value: "2"},
				{ID: deterministicFirstID, Name: "First", Value: "1"},
			},
			wantIDs:   []string{deterministicFirstID, deterministicSecondID},
			wantNames: []string{"First", "Second"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			featureFlagSchema := featureFlagResourceSchema()
			importState := emptyFeatureFlagResourceState(t, featureFlagSchema)
			importResponse := frameworkresource.ImportStateResponse{State: importState}
			(&featureFlagResource{}).ImportState(
				context.Background(),
				frameworkresource.ImportStateRequest{ID: providerEnvironmentA + "/" + test.key},
				&importResponse,
			)
			if importResponse.Diagnostics.HasError() {
				t.Fatalf("ImportState() diagnostics = %v", importResponse.Diagnostics)
			}

			remote := canonicalFeatureFlag{
				EnvironmentID: providerEnvironmentA,
				Name:          "Imported",
				Description:   "",
				Key:           test.key,
				VariationType: featureFlagVariationTypeNumber,
				Variations:    test.variations,
			}
			apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
				func(response http.ResponseWriter, request *http.Request) {
					if request.Method != http.MethodGet || request.URL.RawQuery != "" {
						t.Fatal("import refresh did not use the exact active read")
					}
					writeProjectResourceEnvelope(
						t,
						response,
						http.StatusOK,
						featureFlagResourceDefinitionJSON(
							t,
							remote,
							providerFeatureFlagID,
							false,
							false,
						),
					)
				},
			))
			defer closeServer()
			readResponse := frameworkresource.ReadResponse{State: importResponse.State}
			(&featureFlagResource{client: apiClient}).Read(
				context.Background(),
				frameworkresource.ReadRequest{State: importResponse.State},
				&readResponse,
			)
			if readResponse.Diagnostics.HasError() {
				t.Fatalf("import Read() diagnostics = %v", readResponse.Diagnostics)
			}
			state := featureFlagResourceStateModel(t, readResponse.State)
			if state.ID.ValueString() != providerFeatureFlagID ||
				len(state.Variations) != len(test.wantIDs) {
				t.Fatal("import refresh did not retain the server Feature Flag definition")
			}
			for index := range test.wantIDs {
				if state.Variations[index].ID.ValueString() != test.wantIDs[index] ||
					state.Variations[index].Name.ValueString() != test.wantNames[index] {
					t.Fatal("import refresh did not use the expected stable variation order")
				}
			}
		})
	}
}

func TestFeatureFlagResourceReadRemovesOnlyConfirmedAbsenceAndPreservesUnconfirmedState(t *testing.T) {
	t.Parallel()

	featureFlagSchema := featureFlagResourceSchema()
	priorState, priorCanonical := featureFlagManagedResourceState(
		t,
		featureFlagSchema,
		"read-state",
	)

	t.Run("archived", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
			func(response http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					featureFlagResourceDefinitionJSON(
						t,
						priorCanonical,
						providerFeatureFlagID,
						true,
						false,
					),
				)
			},
		))
		defer closeServer()
		response := frameworkresource.ReadResponse{State: priorState}
		(&featureFlagResource{client: apiClient}).Read(
			context.Background(),
			frameworkresource.ReadRequest{State: priorState},
			&response,
		)
		if !response.Diagnostics.HasError() || calls.Load() != 1 ||
			!response.State.Raw.Equal(priorState.Raw) {
			t.Fatal("archived Read did not preserve managed state")
		}
	})

	t.Run("incomplete fallback", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
			func(response http.ResponseWriter, _ *http.Request) {
				switch calls.Add(1) {
				case 1:
					writeProjectResourceEnvelope(t, response, http.StatusNotFound, "null")
				case 2:
					writeProjectResourceEnvelope(
						t,
						response,
						http.StatusOK,
						featureFlagResourcePageJSON(1, []string{}),
					)
				default:
					t.Fatal("incomplete Read made an unexpected request")
				}
			},
		))
		defer closeServer()
		response := frameworkresource.ReadResponse{State: priorState}
		(&featureFlagResource{client: apiClient}).Read(
			context.Background(),
			frameworkresource.ReadRequest{State: priorState},
			&response,
		)
		if !response.Diagnostics.HasError() || calls.Load() != 2 ||
			!response.State.Raw.Equal(priorState.Raw) {
			t.Fatal("incomplete Read changed managed state")
		}
	})

	t.Run("canceled", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
			func(http.ResponseWriter, *http.Request) { calls.Add(1) },
		))
		defer closeServer()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		response := frameworkresource.ReadResponse{State: priorState}
		(&featureFlagResource{client: apiClient}).Read(
			ctx,
			frameworkresource.ReadRequest{State: priorState},
			&response,
		)
		if !response.Diagnostics.HasError() || calls.Load() != 0 ||
			!response.State.Raw.Equal(priorState.Raw) {
			t.Fatal("canceled Read changed managed state or reached transport")
		}
	})

	t.Run("confirmed absence", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
			func(response http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 {
					writeProjectResourceEnvelope(t, response, http.StatusNotFound, "null")
					return
				}
				writeProjectResourceEnvelope(
					t,
					response,
					http.StatusOK,
					featureFlagResourcePageJSON(0, []string{}),
				)
			},
		))
		defer closeServer()
		response := frameworkresource.ReadResponse{State: priorState}
		(&featureFlagResource{client: apiClient}).Read(
			context.Background(),
			frameworkresource.ReadRequest{State: priorState},
			&response,
		)
		if response.Diagnostics.HasError() || calls.Load() != 3 ||
			!response.State.Raw.IsNull() {
			t.Fatal("complete active/archived exact zero did not remove state")
		}
	})
}

func TestFeatureFlagResourceImportValidation(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		importID string
		wantErr  bool
	}{
		"valid":               {importID: providerEnvironmentA + "/exact-key"},
		"empty":               {importID: "", wantErr: true},
		"one component":       {importID: providerEnvironmentA, wantErr: true},
		"three components":    {importID: providerEnvironmentA + "/key/extra", wantErr: true},
		"invalid environment": {importID: "not-a-uuid/exact-key", wantErr: true},
		"invalid key":         {importID: providerEnvironmentA + "/invalid key", wantErr: true},
		"empty key":           {importID: providerEnvironmentA + "/", wantErr: true},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			featureFlagSchema := featureFlagResourceSchema()
			response := frameworkresource.ImportStateResponse{
				State: emptyFeatureFlagResourceState(t, featureFlagSchema),
			}
			(&featureFlagResource{}).ImportState(
				context.Background(),
				frameworkresource.ImportStateRequest{ID: test.importID},
				&response,
			)
			if response.Diagnostics.HasError() != test.wantErr {
				t.Fatalf("ImportState() diagnostics = %v", response.Diagnostics)
			}
			if test.wantErr {
				if test.importID != "" && diagnosticsContain(response.Diagnostics, test.importID) {
					t.Fatal("ImportState() diagnostic echoed a rejected runtime identifier")
				}
				return
			}
			var environmentID types.String
			var key types.String
			if diagnostics := response.State.GetAttribute(
				context.Background(),
				path.Root("environment_id"),
				&environmentID,
			); diagnostics.HasError() {
				t.Fatalf("read imported Environment ID: %v", diagnostics)
			}
			if diagnostics := response.State.GetAttribute(
				context.Background(),
				path.Root("key"),
				&key,
			); diagnostics.HasError() {
				t.Fatalf("read imported key: %v", diagnostics)
			}
			if environmentID.ValueString() != providerEnvironmentA || key.ValueString() != "exact-key" {
				t.Fatal("ImportState() did not set exactly the two public identity components")
			}
		})
	}
}

func featureFlagResourcePlanModel(
	environmentID string,
	key string,
	name string,
	description string,
	variationType string,
	values []string,
) featureFlagModel {
	model := featureFlagModel{
		EnvironmentID: types.StringValue(environmentID),
		ID:            types.StringUnknown(),
		Name:          types.StringValue(name),
		Description:   types.StringValue(description),
		Key:           types.StringValue(key),
		VariationType: types.StringValue(variationType),
		Variations:    make([]featureFlagVariationModel, 0, len(values)),
	}
	for index, value := range values {
		model.Variations = append(model.Variations, featureFlagVariationModel{
			ID:    types.StringUnknown(),
			Name:  types.StringValue(fmt.Sprintf("Variation %d", index+1)),
			Value: types.StringValue(value),
		})
	}
	return model
}

func emptyFeatureFlagResourceState(
	t *testing.T,
	featureFlagSchema resourceschema.Schema,
) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: featureFlagSchema}
	empty := featureFlagModel{
		EnvironmentID: types.StringNull(),
		ID:            types.StringNull(),
		Name:          types.StringNull(),
		Description:   types.StringNull(),
		Key:           types.StringNull(),
		VariationType: types.StringNull(),
		Variations:    nil,
	}
	if diagnostics := state.Set(context.Background(), &empty); diagnostics.HasError() {
		t.Fatalf("initialize empty Feature Flag resource state: %v", diagnostics)
	}
	return state
}

func featureFlagResourceStateModel(t *testing.T, state tfsdk.State) featureFlagModel {
	t.Helper()
	var model featureFlagModel
	if diagnostics := state.Get(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("read Feature Flag resource state: %v", diagnostics)
	}
	return model
}

func featureFlagManagedResourceState(
	t *testing.T,
	featureFlagSchema resourceschema.Schema,
	key string,
) (tfsdk.State, canonicalFeatureFlag) {
	t.Helper()
	planned, _, err := canonicalizePlannedFeatureFlag(
		providerEnvironmentA,
		key,
		"Managed",
		"",
		featureFlagVariationTypeString,
		[]featureFlagVariationInput{{Name: "One", Value: "value"}},
	)
	if err != nil {
		t.Fatalf("canonicalize managed Feature Flag state: %v", err)
	}
	planned.ID = providerFeatureFlagID
	state := tfsdk.State{Schema: featureFlagSchema}
	model := flattenCanonicalFeatureFlag(planned)
	if diagnostics := state.Set(context.Background(), &model); diagnostics.HasError() {
		t.Fatalf("initialize managed Feature Flag state: %v", diagnostics)
	}
	return state, planned
}

func featureFlagResourceListItemJSON(id string, key string) string {
	return `{"id":"` + id + `","name":"Flag","key":"` + key + `"}`
}

func featureFlagResourcePageJSON(total int64, items []string) string {
	return fmt.Sprintf(`{"totalCount":%d,"items":[%s]}`, total, strings.Join(items, ","))
}

func featureFlagResourceDefinitionJSON(
	t *testing.T,
	flag canonicalFeatureFlag,
	id string,
	archived bool,
	reverse bool,
) string {
	t.Helper()
	variations := make([]map[string]any, 0, len(flag.Variations))
	for _, variation := range flag.Variations {
		variations = append(variations, map[string]any{
			"id": variation.ID, "name": variation.Name, "value": variation.Value,
		})
	}
	if reverse {
		for left, right := 0, len(variations)-1; left < right; left, right = left+1, right-1 {
			variations[left], variations[right] = variations[right], variations[left]
		}
	}
	payload := map[string]any{
		"id": id, "envId": flag.EnvironmentID, "name": flag.Name,
		"description": flag.Description, "key": flag.Key,
		"variationType": flag.VariationType, "variations": variations,
		"isArchived": archived, "isEnabled": true,
		"tags":        []string{"synthetic-ui-owned-tag"},
		"targetUsers": []map[string]any{{"keyIds": []string{"synthetic-ui-owned-target"}}},
		"rules":       []map[string]any{{"name": "synthetic-ui-owned-rule"}},
		"fallthrough": map[string]any{"dispatchKey": "synthetic-ui-owned-dispatch"},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode Feature Flag definition fixture: %v", err)
	}
	return string(encoded)
}

func assertFeatureFlagResourceRequestBoundary(t *testing.T, request *http.Request) {
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
			t.Fatalf("Feature Flag request sent unsupported context header %q", header)
		}
	}
}
