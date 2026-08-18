// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"slices"
	"sort"
	"testing"

	featbitprovider "github.com/featbit/terraform-provider-featbit/internal/provider"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const (
	initialReleaseVersion       = "0.1.0"
	releaseSchemaPath           = "internal/provider/testdata/release-schema.json"
	releaseProjectID            = "11111111-1111-4111-8111-111111111111"
	releaseEnvironmentID        = "22222222-2222-4222-8222-222222222222"
	releaseSegmentID            = "33333333-3333-4333-8333-333333333333"
	releasePolicyID             = "44444444-4444-4444-8444-444444444444"
	releaseGroupID              = "55555555-5555-4555-8555-555555555555"
	releaseMemberID             = "66666666-6666-4666-8666-666666666666"
	releaseGroupPolicyBindingID = releaseGroupID + "/" + releasePolicyID
	releaseGroupMemberBindingID = releaseGroupID + "/" + releaseMemberID
)

var releaseImportForms = map[string]string{
	"featbit_project":              "<project_uuid>",
	"featbit_environment":          "<project_uuid>/<environment_uuid>",
	"featbit_feature_flag":         "<environment_uuid>/<exact_key>",
	"featbit_group":                "<group_uuid>",
	"featbit_group_member_binding": "<group_uuid>/<member_uuid>",
	"featbit_group_policy_binding": "<group_uuid>/<policy_uuid>",
	"featbit_policy":               "<policy_uuid>",
	"featbit_segment":              "<environment_uuid>/<segment_uuid>",
}

type releaseContractSnapshot struct {
	FormatVersion      int                              `json:"format_version"`
	ProviderAddress    string                           `json:"provider_address"`
	ProtocolVersions   []string                         `json:"protocol_versions"`
	Provider           *releaseSchemaSnapshot           `json:"provider"`
	ProviderMeta       *releaseSchemaSnapshot           `json:"provider_meta"`
	Resources          map[string]releaseSchemaSnapshot `json:"resources"`
	DataSources        map[string]releaseSchemaSnapshot `json:"data_sources"`
	Functions          []string                         `json:"functions"`
	EphemeralResources []string                         `json:"ephemeral_resources"`
	ListResources      []string                         `json:"list_resources"`
	Actions            []string                         `json:"actions"`
	StateStores        []string                         `json:"state_stores"`
	ImportForms        map[string]string                `json:"import_forms"`
}

type releaseSchemaSnapshot struct {
	Version int64                `json:"version"`
	Block   releaseBlockSnapshot `json:"block"`
}

type releaseBlockSnapshot struct {
	Version            int64                        `json:"version"`
	Description        string                       `json:"description,omitempty"`
	DescriptionKind    string                       `json:"description_kind"`
	Deprecated         bool                         `json:"deprecated"`
	DeprecationMessage string                       `json:"deprecation_message,omitempty"`
	Attributes         []releaseAttributeSnapshot   `json:"attributes"`
	BlockTypes         []releaseNestedBlockSnapshot `json:"block_types"`
}

type releaseAttributeSnapshot struct {
	Name               string                       `json:"name"`
	Type               json.RawMessage              `json:"type,omitempty"`
	NestedType         *releaseNestedObjectSnapshot `json:"nested_type,omitempty"`
	Description        string                       `json:"description,omitempty"`
	DescriptionKind    string                       `json:"description_kind"`
	Required           bool                         `json:"required"`
	Optional           bool                         `json:"optional"`
	Computed           bool                         `json:"computed"`
	Sensitive          bool                         `json:"sensitive"`
	WriteOnly          bool                         `json:"write_only"`
	Deprecated         bool                         `json:"deprecated"`
	DeprecationMessage string                       `json:"deprecation_message,omitempty"`
}

type releaseNestedObjectSnapshot struct {
	Nesting    string                     `json:"nesting"`
	Attributes []releaseAttributeSnapshot `json:"attributes"`
}

type releaseNestedBlockSnapshot struct {
	TypeName string               `json:"type_name"`
	Nesting  string               `json:"nesting"`
	MinItems int64                `json:"min_items"`
	MaxItems int64                `json:"max_items"`
	Block    releaseBlockSnapshot `json:"block"`
}

func TestInitialReleaseProtocolSchemaSnapshot(t *testing.T) {
	t.Parallel()

	server := providerserver.NewProtocol6(featbitprovider.New(initialReleaseVersion)())()
	response, err := server.GetProviderSchema(
		context.Background(),
		&tfprotov6.GetProviderSchemaRequest{},
	)
	if err != nil {
		t.Fatalf("GetProviderSchema() error = %v", err)
	}
	if releaseDiagnosticsHaveError(response.Diagnostics) {
		t.Fatalf("GetProviderSchema() diagnostics = %v", response.Diagnostics)
	}
	assertReleaseSurfaceNames(t, response)

	actual := releaseSnapshotJSON(t, response)
	expectedBytes, err := os.ReadFile(releaseSchemaPath)
	if err != nil {
		t.Fatalf("read release schema snapshot: %v", err)
	}
	var expected releaseContractSnapshot
	decoder := json.NewDecoder(bytes.NewReader(expectedBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&expected); err != nil {
		t.Fatalf("decode release schema snapshot: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatalf("release schema snapshot has trailing content: %v", err)
	}
	expectedCanonical, err := json.MarshalIndent(expected, "", "  ")
	if err != nil {
		t.Fatalf("encode release schema snapshot: %v", err)
	}
	expectedCanonical = append(expectedCanonical, '\n')
	if bytes.Equal(actual, expectedCanonical) {
		return
	}
	t.Fatal("Protocol v6 release schema differs from internal/provider/testdata/release-schema.json")
}

func TestInitialReleaseImportForms(t *testing.T) {
	t.Parallel()

	server := providerserver.NewProtocol6(featbitprovider.New(initialReleaseVersion)())()
	schemaResponse, err := server.GetProviderSchema(
		context.Background(),
		&tfprotov6.GetProviderSchemaRequest{},
	)
	if err != nil || releaseDiagnosticsHaveError(schemaResponse.Diagnostics) {
		t.Fatalf("GetProviderSchema() = %v / %v", err, schemaResponse.Diagnostics)
	}

	tests := map[string]struct {
		validID       string
		identity      map[string]string
		rejectedForms []string
	}{
		"featbit_project": {
			validID:  releaseProjectID,
			identity: map[string]string{"id": releaseProjectID},
			rejectedForms: []string{
				"",
				releaseProjectID + "/" + releaseEnvironmentID,
				"not-a-uuid",
			},
		},
		"featbit_environment": {
			validID: releaseProjectID + "/" + releaseEnvironmentID,
			identity: map[string]string{
				"project_id": releaseProjectID,
				"id":         releaseEnvironmentID,
			},
			rejectedForms: []string{
				"",
				releaseEnvironmentID,
				releaseProjectID + "/" + releaseEnvironmentID + "/extra",
			},
		},
		"featbit_feature_flag": {
			validID: releaseEnvironmentID + "/exact-key",
			identity: map[string]string{
				"environment_id": releaseEnvironmentID,
				"key":            "exact-key",
			},
			rejectedForms: []string{
				"",
				releaseEnvironmentID,
				releaseEnvironmentID + "/invalid key",
				releaseEnvironmentID + "/exact-key/extra",
			},
		},
		"featbit_group": {
			validID:  releaseGroupID,
			identity: map[string]string{"id": releaseGroupID},
			rejectedForms: []string{
				"",
				"not-a-uuid",
				releaseGroupID + "/extra",
			},
		},
		"featbit_group_policy_binding": {
			validID: releaseGroupPolicyBindingID,
			identity: map[string]string{
				"id":        releaseGroupPolicyBindingID,
				"group_id":  releaseGroupID,
				"policy_id": releasePolicyID,
			},
			rejectedForms: []string{
				"",
				releaseGroupID,
				"not-a-uuid/" + releasePolicyID,
				releaseGroupID + "/not-a-uuid",
				releaseGroupPolicyBindingID + "/extra",
			},
		},
		"featbit_group_member_binding": {
			validID: releaseGroupMemberBindingID,
			identity: map[string]string{
				"id":        releaseGroupMemberBindingID,
				"group_id":  releaseGroupID,
				"member_id": releaseMemberID,
			},
			rejectedForms: []string{
				"",
				releaseGroupID,
				"not-a-uuid/" + releaseMemberID,
				releaseGroupID + "/not-a-uuid",
				releaseGroupMemberBindingID + "/extra",
			},
		},
		"featbit_policy": {
			validID:  releasePolicyID,
			identity: map[string]string{"id": releasePolicyID},
			rejectedForms: []string{
				"",
				"not-a-uuid",
				releasePolicyID + "/extra",
			},
		},
		"featbit_segment": {
			validID: releaseEnvironmentID + "/" + releaseSegmentID,
			identity: map[string]string{
				"environment_id": releaseEnvironmentID,
				"id":             releaseSegmentID,
			},
			rejectedForms: []string{
				"",
				releaseSegmentID,
				releaseEnvironmentID + "/not-a-uuid",
				releaseEnvironmentID + "/" + releaseSegmentID + "/extra",
			},
		},
	}

	if len(tests) != len(releaseImportForms) {
		t.Fatalf("Import contract cases = %d, frozen forms = %d", len(tests), len(releaseImportForms))
	}
	for typeName, test := range tests {
		typeName := typeName
		test := test
		t.Run(typeName, func(t *testing.T) {
			t.Parallel()

			resourceSchema, ok := schemaResponse.ResourceSchemas[typeName]
			if !ok {
				t.Fatalf("release schema has no %s resource", typeName)
			}
			response, err := server.ImportResourceState(
				context.Background(),
				&tfprotov6.ImportResourceStateRequest{TypeName: typeName, ID: test.validID},
			)
			if err != nil || releaseDiagnosticsHaveError(response.Diagnostics) {
				t.Fatalf("valid ImportResourceState() = %v / %v", err, response.Diagnostics)
			}
			assertReleaseImportState(t, typeName, resourceSchema, response, test.identity)

			for _, rejected := range test.rejectedForms {
				response, err := server.ImportResourceState(
					context.Background(),
					&tfprotov6.ImportResourceStateRequest{TypeName: typeName, ID: rejected},
				)
				if err != nil {
					t.Fatalf("rejected ImportResourceState(%q) transport error = %v", rejected, err)
				}
				if !releaseDiagnosticsHaveError(response.Diagnostics) {
					t.Fatalf("ImportResourceState(%q) unexpectedly accepted an unfrozen form", rejected)
				}
			}
		})
	}
}

func TestInitialReleaseMetadataAndManifest(t *testing.T) {
	t.Parallel()

	providerUnderTest := featbitprovider.New(initialReleaseVersion)()
	var metadata frameworkprovider.MetadataResponse
	providerUnderTest.Metadata(
		context.Background(),
		frameworkprovider.MetadataRequest{},
		&metadata,
	)
	if metadata.TypeName != "featbit" || metadata.Version != initialReleaseVersion {
		t.Fatalf("provider metadata = %q/%q", metadata.TypeName, metadata.Version)
	}
	if providerAddress != "registry.terraform.io/featbit/featbit" {
		t.Fatalf("provider address = %q", providerAddress)
	}

	type registryManifest struct {
		Version  int `json:"version"`
		Metadata struct {
			ProtocolVersions []string `json:"protocol_versions"`
		} `json:"metadata"`
	}
	manifestBytes, err := os.ReadFile("terraform-registry-manifest.json")
	if err != nil {
		t.Fatalf("read Registry manifest: %v", err)
	}
	var manifest registryManifest
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatalf("decode Registry manifest: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatalf("Registry manifest has trailing content: %v", err)
	}
	if manifest.Version != 1 || len(manifest.Metadata.ProtocolVersions) != 1 ||
		manifest.Metadata.ProtocolVersions[0] != "6.0" {
		t.Fatalf("Registry manifest = %#v, want format 1 and Protocol 6.0", manifest)
	}
}

func releaseSnapshotJSON(t *testing.T, response *tfprotov6.GetProviderSchemaResponse) []byte {
	t.Helper()

	snapshot := releaseContractSnapshot{
		FormatVersion:      1,
		ProviderAddress:    providerAddress,
		ProtocolVersions:   []string{"6.0"},
		Provider:           releaseSchema(t, response.Provider),
		ProviderMeta:       releaseSchema(t, response.ProviderMeta),
		Resources:          releaseSchemaMap(t, response.ResourceSchemas),
		DataSources:        releaseSchemaMap(t, response.DataSourceSchemas),
		Functions:          sortedReleaseKeys(response.Functions),
		EphemeralResources: sortedReleaseKeys(response.EphemeralResourceSchemas),
		ListResources:      sortedReleaseKeys(response.ListResourceSchemas),
		Actions:            sortedReleaseKeys(response.ActionSchemas),
		StateStores:        sortedReleaseKeys(response.StateStoreSchemas),
		ImportForms:        releaseImportForms,
	}
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatalf("encode current Protocol schema: %v", err)
	}
	return append(encoded, '\n')
}

func releaseSchemaMap(
	t *testing.T,
	schemas map[string]*tfprotov6.Schema,
) map[string]releaseSchemaSnapshot {
	t.Helper()

	snapshots := make(map[string]releaseSchemaSnapshot, len(schemas))
	for name, schema := range schemas {
		snapshot := releaseSchema(t, schema)
		if snapshot == nil {
			t.Fatalf("schema %q is nil", name)
		}
		snapshots[name] = *snapshot
	}
	return snapshots
}

func releaseSchema(t *testing.T, schema *tfprotov6.Schema) *releaseSchemaSnapshot {
	t.Helper()
	if schema == nil {
		return nil
	}
	if schema.Block == nil {
		t.Fatal("Protocol schema has a nil root block")
	}
	return &releaseSchemaSnapshot{
		Version: schema.Version,
		Block:   releaseBlock(t, schema.Block),
	}
}

func releaseBlock(t *testing.T, block *tfprotov6.SchemaBlock) releaseBlockSnapshot {
	t.Helper()

	attributes := make([]releaseAttributeSnapshot, 0, len(block.Attributes))
	for _, attribute := range block.Attributes {
		attributes = append(attributes, releaseAttribute(t, attribute))
	}
	sort.Slice(attributes, func(left, right int) bool {
		return attributes[left].Name < attributes[right].Name
	})
	blockTypes := make([]releaseNestedBlockSnapshot, 0, len(block.BlockTypes))
	for _, nested := range block.BlockTypes {
		if nested == nil || nested.Block == nil {
			t.Fatal("Protocol schema contains a nil nested block")
		}
		blockTypes = append(blockTypes, releaseNestedBlockSnapshot{
			TypeName: nested.TypeName,
			Nesting:  nested.Nesting.String(),
			MinItems: nested.MinItems,
			MaxItems: nested.MaxItems,
			Block:    releaseBlock(t, nested.Block),
		})
	}
	sort.Slice(blockTypes, func(left, right int) bool {
		return blockTypes[left].TypeName < blockTypes[right].TypeName
	})
	return releaseBlockSnapshot{
		Version:            block.Version,
		Description:        block.Description,
		DescriptionKind:    block.DescriptionKind.String(),
		Deprecated:         block.Deprecated,
		DeprecationMessage: block.DeprecationMessage,
		Attributes:         attributes,
		BlockTypes:         blockTypes,
	}
}

func releaseAttribute(
	t *testing.T,
	attribute *tfprotov6.SchemaAttribute,
) releaseAttributeSnapshot {
	t.Helper()
	if attribute == nil {
		t.Fatal("Protocol schema contains a nil attribute")
	}
	snapshot := releaseAttributeSnapshot{
		Name:               attribute.Name,
		Description:        attribute.Description,
		DescriptionKind:    attribute.DescriptionKind.String(),
		Required:           attribute.Required,
		Optional:           attribute.Optional,
		Computed:           attribute.Computed,
		Sensitive:          attribute.Sensitive,
		WriteOnly:          attribute.WriteOnly,
		Deprecated:         attribute.Deprecated,
		DeprecationMessage: attribute.DeprecationMessage,
	}
	if attribute.Type != nil {
		encoded, err := attribute.Type.MarshalJSON()
		if err != nil {
			t.Fatalf("encode type for attribute %q: %v", attribute.Name, err)
		}
		snapshot.Type = encoded
	}
	if attribute.NestedType != nil {
		attributes := make([]releaseAttributeSnapshot, 0, len(attribute.NestedType.Attributes))
		for _, nested := range attribute.NestedType.Attributes {
			attributes = append(attributes, releaseAttribute(t, nested))
		}
		sort.Slice(attributes, func(left, right int) bool {
			return attributes[left].Name < attributes[right].Name
		})
		snapshot.NestedType = &releaseNestedObjectSnapshot{
			Nesting:    attribute.NestedType.Nesting.String(),
			Attributes: attributes,
		}
	}
	return snapshot
}

func sortedReleaseKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func assertReleaseSurfaceNames(t *testing.T, response *tfprotov6.GetProviderSchemaResponse) {
	t.Helper()
	if response.Provider == nil || response.Provider.Block == nil {
		t.Fatal("initial release provider schema is nil")
	}

	providerAttributes := make([]string, 0, len(response.Provider.Block.Attributes))
	for _, attribute := range response.Provider.Block.Attributes {
		providerAttributes = append(providerAttributes, attribute.Name)
	}
	sort.Strings(providerAttributes)
	wantProviderAttributes := []string{
		"access_token",
		"api_url",
		"http_timeout_seconds",
		"max_concurrency",
		"max_retries",
	}
	wantResources := []string{
		"featbit_environment",
		"featbit_feature_flag",
		"featbit_group",
		"featbit_group_member_binding",
		"featbit_group_policy_binding",
		"featbit_policy",
		"featbit_project",
		"featbit_segment",
	}
	wantDataSources := []string{
		"featbit_environment",
		"featbit_feature_flag",
		"featbit_group",
		"featbit_member",
		"featbit_policy",
		"featbit_project",
		"featbit_segment",
	}
	checks := map[string]struct {
		got  []string
		want []string
	}{
		"provider attributes": {got: providerAttributes, want: wantProviderAttributes},
		"resources":           {got: sortedReleaseKeys(response.ResourceSchemas), want: wantResources},
		"data sources":        {got: sortedReleaseKeys(response.DataSourceSchemas), want: wantDataSources},
		"functions":           {got: sortedReleaseKeys(response.Functions), want: []string{}},
		"ephemeral resources": {got: sortedReleaseKeys(response.EphemeralResourceSchemas), want: []string{}},
		"list resources":      {got: sortedReleaseKeys(response.ListResourceSchemas), want: []string{}},
		"actions":             {got: sortedReleaseKeys(response.ActionSchemas), want: []string{}},
		"state stores":        {got: sortedReleaseKeys(response.StateStoreSchemas), want: []string{}},
	}
	for name, check := range checks {
		if !slices.Equal(check.got, check.want) {
			t.Fatalf("initial release %s = %v, want %v", name, check.got, check.want)
		}
	}
}

func assertReleaseImportState(
	t *testing.T,
	typeName string,
	schema *tfprotov6.Schema,
	response *tfprotov6.ImportResourceStateResponse,
	want map[string]string,
) {
	t.Helper()
	remaining := make(map[string]string, len(want))
	for name, value := range want {
		remaining[name] = value
	}
	if len(response.ImportedResources) != 1 {
		t.Fatalf("imported resources = %d, want 1", len(response.ImportedResources))
	}
	imported := response.ImportedResources[0]
	if imported.TypeName != typeName || imported.State == nil {
		t.Fatalf("imported resource = %#v, want type %s with state", imported, typeName)
	}
	state, err := imported.State.Unmarshal(schema.ValueType())
	if err != nil {
		t.Fatalf("decode imported state: %v", err)
	}
	var values map[string]tftypes.Value
	if err := state.As(&values); err != nil {
		t.Fatalf("convert imported state: %v", err)
	}
	for name, value := range values {
		expected, isIdentity := remaining[name]
		if !isIdentity {
			if !value.IsNull() {
				t.Fatalf("ImportState set non-identity attribute %q", name)
			}
			continue
		}
		var actual string
		if err := value.As(&actual); err != nil || actual != expected {
			t.Fatalf("imported identity %q = %q / %v, want %q", name, actual, err, expected)
		}
		delete(remaining, name)
	}
	if len(remaining) != 0 {
		t.Fatalf("ImportState omitted identity attributes %v", sortedReleaseKeys(remaining))
	}
}

func releaseDiagnosticsHaveError(diagnostics []*tfprotov6.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic != nil && diagnostic.Severity == tfprotov6.DiagnosticSeverityError {
			return true
		}
	}
	return false
}
