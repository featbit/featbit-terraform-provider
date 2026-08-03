// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"strings"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/google/uuid"
)

const (
	providerFeatureFlagID       = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	providerFeatureVariationOne = "12345678-1234-4234-8234-1234567890ab"
	providerFeatureVariationTwo = "abcdefab-cdef-4abc-8def-abcdefabcdef"
)

func TestCanonicalizeFeatureFlagVariationTypes(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		value string
		valid bool
	}{
		"boolean":                    {value: featureFlagVariationTypeBoolean, valid: true},
		"string":                     {value: featureFlagVariationTypeString, valid: true},
		"number":                     {value: featureFlagVariationTypeNumber, valid: true},
		"json":                       {value: featureFlagVariationTypeJSON, valid: true},
		"uppercase is not canonical": {value: "Boolean"},
		"abbreviation":               {value: "bool"},
		"surrounding whitespace":     {value: " json "},
		"empty":                      {},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			canonical, err := canonicalizeFeatureFlagVariationType(test.value)
			if test.valid {
				if err != nil || canonical != test.value {
					t.Fatal("supported variation type did not retain its canonical spelling")
				}
				return
			}
			if err == nil || canonical != "" {
				t.Fatal("unsupported variation type was accepted")
			}
		})
	}
}

func TestCanonicalizeFeatureFlagValuesPrecisely(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		variationType string
		value         string
		want          string
		valid         bool
	}{
		"boolean lowercase": {
			variationType: featureFlagVariationTypeBoolean,
			value:         "false",
			want:          "false",
			valid:         true,
		},
		"boolean case canonicalization": {
			variationType: featureFlagVariationTypeBoolean,
			value:         "TRUE",
			want:          "true",
			valid:         true,
		},
		"boolean numeric spelling": {
			variationType: featureFlagVariationTypeBoolean,
			value:         "1",
		},
		"boolean surrounding whitespace": {
			variationType: featureFlagVariationTypeBoolean,
			value:         " true ",
		},
		"string exact data": {
			variationType: featureFlagVariationTypeString,
			value:         "  Exact user data\n",
			want:          "  Exact user data\n",
			valid:         true,
		},
		"large integer precision": {
			variationType: featureFlagVariationTypeNumber,
			value:         "900719925474099312345678901234567890123456789",
			want:          "900719925474099312345678901234567890123456789",
			valid:         true,
		},
		"decimal normalization": {
			variationType: featureFlagVariationTypeNumber,
			value:         "123.450000",
			want:          "123.45",
			valid:         true,
		},
		"exponent normalization": {
			variationType: featureFlagVariationTypeNumber,
			value:         "1.2300e+10",
			want:          "12300000000",
			valid:         true,
		},
		"negative zero": {
			variationType: featureFlagVariationTypeNumber,
			value:         "-0.000e999",
			want:          "0",
			valid:         true,
		},
		"large positive exponent stays bounded": {
			variationType: featureFlagVariationTypeNumber,
			value:         "1e10000",
			want:          "1e10000",
			valid:         true,
		},
		"large negative exponent stays bounded": {
			variationType: featureFlagVariationTypeNumber,
			value:         "1e-10000",
			want:          "1e-10000",
			valid:         true,
		},
		"leading zero": {
			variationType: featureFlagVariationTypeNumber,
			value:         "01",
		},
		"leading plus": {
			variationType: featureFlagVariationTypeNumber,
			value:         "+1",
		},
		"missing fractional digits": {
			variationType: featureFlagVariationTypeNumber,
			value:         "1.",
		},
		"missing integer digits": {
			variationType: featureFlagVariationTypeNumber,
			value:         ".1",
		},
		"non-finite NaN": {
			variationType: featureFlagVariationTypeNumber,
			value:         "NaN",
		},
		"non-finite infinity": {
			variationType: featureFlagVariationTypeNumber,
			value:         "Infinity",
		},
		"number trailing value": {
			variationType: featureFlagVariationTypeNumber,
			value:         "1 2",
		},
		"number whitespace": {
			variationType: featureFlagVariationTypeNumber,
			value:         " 1 ",
		},
		"JSON canonical ordering and precision": {
			variationType: featureFlagVariationTypeJSON,
			value: ` { "z": 1.2300e2, "a": [` +
				`900719925474099312345678901234567890, true], ` +
				`"nested": {"b":2,"a":1} } `,
			want: `{"a":[900719925474099312345678901234567890,true],` +
				`"nested":{"a":1,"b":2},"z":123}`,
			valid: true,
		},
		"JSON top-level string": {
			variationType: featureFlagVariationTypeJSON,
			value:         ` " exact " `,
			want:          `" exact "`,
			valid:         true,
		},
		"JSON large exponent": {
			variationType: featureFlagVariationTypeJSON,
			value:         `{"n":1e10000}`,
			want:          `{"n":1e10000}`,
			valid:         true,
		},
		"JSON trailing value": {
			variationType: featureFlagVariationTypeJSON,
			value:         `{} []`,
		},
		"JSON duplicate object key": {
			variationType: featureFlagVariationTypeJSON,
			value:         `{"same":1,"same":2}`,
		},
		"JSON non-finite number": {
			variationType: featureFlagVariationTypeJSON,
			value:         `{"n":NaN}`,
		},
		"JSON empty input": {
			variationType: featureFlagVariationTypeJSON,
		},
		"unknown variation type": {
			variationType: "unknown",
			value:         "value",
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			canonical, err := canonicalizeFeatureFlagValue(test.variationType, test.value)
			if test.valid {
				if err != nil || canonical != test.want {
					t.Fatalf("canonical value = %q, want %q", canonical, test.want)
				}
				return
			}
			if err == nil || canonical != "" {
				t.Fatal("invalid variation value was accepted")
			}
		})
	}
}

func TestCanonicalizePlannedFeatureFlagFreezesStableIDsAndCreateSeed(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		variationType string
		variations    []featureFlagVariationInput
		wantValues    []string
	}{
		"boolean": {
			variationType: featureFlagVariationTypeBoolean,
			variations: []featureFlagVariationInput{
				{Name: "Enabled", Value: "TRUE"},
				{Name: "Disabled", Value: "FALSE"},
			},
			wantValues: []string{"true", "false"},
		},
		"string duplicate-looking variations": {
			variationType: featureFlagVariationTypeString,
			variations: []featureFlagVariationInput{
				{Name: "Same", Value: "same"},
				{Name: "Same", Value: "same"},
			},
			wantValues: []string{"same", "same"},
		},
		"number": {
			variationType: featureFlagVariationTypeNumber,
			variations: []featureFlagVariationInput{
				{Name: "One", Value: "1.00"},
				{Name: "Two", Value: "2e0"},
			},
			wantValues: []string{"1", "2"},
		},
		"json": {
			variationType: featureFlagVariationTypeJSON,
			variations: []featureFlagVariationInput{
				{Name: "One", Value: `{"b":2,"a":1}`},
				{Name: "Two", Value: `false`},
			},
			wantValues: []string{`{"a":1,"b":2}`, `false`},
		},
	}

	const (
		frozenFirstID  = "994e37df-8f31-5ada-a89e-0aca2cfcb55a"
		frozenSecondID = "34a1930d-50d3-5b17-bcfb-302156df9dbb"
	)
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			canonical, seed, err := canonicalizePlannedFeatureFlag(
				providerEnvironmentA,
				"stable-key",
				"Stable Flag",
				"",
				test.variationType,
				test.variations,
			)
			if err != nil {
				t.Fatalf("canonicalizePlannedFeatureFlag() error = %v", err)
			}
			repeated, repeatedSeed, err := canonicalizePlannedFeatureFlag(
				providerEnvironmentA,
				"stable-key",
				"Stable Flag",
				"",
				test.variationType,
				test.variations,
			)
			if err != nil {
				t.Fatal("repeated canonicalization failed")
			}
			if len(canonical.Variations) != 2 || len(repeated.Variations) != 2 {
				t.Fatal("planned canonicalization returned the wrong variation count")
			}
			if canonical.Variations[0].ID != repeated.Variations[0].ID ||
				canonical.Variations[1].ID != repeated.Variations[1].ID ||
				canonical.Variations[0].ID == canonical.Variations[1].ID {
				t.Fatal("planned variation UUIDs were not deterministic, valid, and unique")
			}
			for _, variation := range canonical.Variations {
				parsed, parseErr := uuid.Parse(variation.ID)
				if parseErr != nil || parsed.String() != variation.ID ||
					parsed.Version() != uuid.Version(5) || parsed.Variant() != uuid.RFC4122 {
					t.Fatal("planned variation identity is not a canonical RFC UUID v5")
				}
			}
			if canonical.Variations[0].ID != frozenFirstID ||
				canonical.Variations[1].ID != frozenSecondID {
				t.Fatalf(
					"frozen deterministic UUIDs changed: first=%s second=%s",
					canonical.Variations[0].ID,
					canonical.Variations[1].ID,
				)
			}
			for index, variation := range canonical.Variations {
				if variation.Value != test.wantValues[index] {
					t.Fatal("planned variation value did not use the shared canonicalizer")
				}
			}
			if seed.IsEnabled || seed.EnabledVariationID != canonical.Variations[0].ID ||
				seed.DisabledVariationID != canonical.Variations[0].ID || seed.Tags == nil ||
				len(seed.Tags) != 0 || seed.IsEnabled != repeatedSeed.IsEnabled ||
				seed.EnabledVariationID != repeatedSeed.EnabledVariationID ||
				seed.DisabledVariationID != repeatedSeed.DisabledVariationID ||
				repeatedSeed.Tags == nil || len(repeatedSeed.Tags) != 0 {
				t.Fatal("disabled-safe Create seed was not deterministic and minimal")
			}
		})
	}
}

func TestCanonicalizePlannedFeatureFlagRejectsInvalidDefinitions(t *testing.T) {
	t.Parallel()

	validVariations := []featureFlagVariationInput{{Name: "One", Value: "true"}}
	tests := map[string]struct {
		environmentID string
		key           string
		name          string
		variationType string
		variations    []featureFlagVariationInput
	}{
		"invalid environment": {
			environmentID: "invalid",
			key:           "key",
			name:          "Name",
			variationType: featureFlagVariationTypeBoolean,
			variations:    validVariations,
		},
		"empty key": {
			environmentID: providerEnvironmentA,
			name:          "Name",
			variationType: featureFlagVariationTypeBoolean,
			variations:    validVariations,
		},
		"invalid key characters": {
			environmentID: providerEnvironmentA,
			key:           "invalid/key",
			name:          "Name",
			variationType: featureFlagVariationTypeBoolean,
			variations:    validVariations,
		},
		"key too long": {
			environmentID: providerEnvironmentA,
			key:           strings.Repeat("k", featureFlagMaximumKeyLength+1),
			name:          "Name",
			variationType: featureFlagVariationTypeBoolean,
			variations:    validVariations,
		},
		"blank name": {
			environmentID: providerEnvironmentA,
			key:           "key",
			name:          "  ",
			variationType: featureFlagVariationTypeBoolean,
			variations:    validVariations,
		},
		"name exceeds UTF-16 limit": {
			environmentID: providerEnvironmentA,
			key:           "key",
			name:          strings.Repeat("😀", 65),
			variationType: featureFlagVariationTypeBoolean,
			variations:    validVariations,
		},
		"invalid type": {
			environmentID: providerEnvironmentA,
			key:           "key",
			name:          "Name",
			variationType: "Boolean",
			variations:    validVariations,
		},
		"empty variations": {
			environmentID: providerEnvironmentA,
			key:           "key",
			name:          "Name",
			variationType: featureFlagVariationTypeBoolean,
			variations:    []featureFlagVariationInput{},
		},
		"blank variation name": {
			environmentID: providerEnvironmentA,
			key:           "key",
			name:          "Name",
			variationType: featureFlagVariationTypeBoolean,
			variations:    []featureFlagVariationInput{{Name: " ", Value: "true"}},
		},
		"empty variation value": {
			environmentID: providerEnvironmentA,
			key:           "key",
			name:          "Name",
			variationType: featureFlagVariationTypeString,
			variations:    []featureFlagVariationInput{{Name: "One"}},
		},
		"value invalid for type": {
			environmentID: providerEnvironmentA,
			key:           "key",
			name:          "Name",
			variationType: featureFlagVariationTypeBoolean,
			variations:    []featureFlagVariationInput{{Name: "One", Value: "yes"}},
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, _, err := canonicalizePlannedFeatureFlag(
				test.environmentID,
				test.key,
				test.name,
				"",
				test.variationType,
				test.variations,
			)
			if err == nil {
				t.Fatal("invalid planned Feature Flag definition was accepted")
			}
		})
	}

	canonical, _, err := canonicalizePlannedFeatureFlag(
		providerEnvironmentA,
		"key",
		strings.Repeat("😀", 64),
		"",
		featureFlagVariationTypeString,
		[]featureFlagVariationInput{{Name: "Whitespace", Value: " "}},
	)
	if err != nil || canonical.Variations[0].Value != " " {
		t.Fatal("valid UTF-16 name boundary or exact non-empty string value was rejected")
	}
}

func TestCanonicalizeRemoteFeatureFlagCorrelatesVariationsByExactUUID(t *testing.T) {
	t.Parallel()

	remote := providerFeatureFlagForCanonicalTest()
	remote.VariationType = featureFlagVariationTypeNumber
	remote.Variations = []client.FeatureFlagVariation{
		{ID: strings.ToUpper(providerFeatureVariationTwo), Name: "Two", Value: "2.00"},
		{ID: providerFeatureVariationOne, Name: "One", Value: "1e0"},
	}

	canonical, err := canonicalizeRemoteFeatureFlag(remote, []string{
		providerFeatureVariationOne,
		providerFeatureVariationTwo,
	})
	if err != nil {
		t.Fatalf("canonicalizeRemoteFeatureFlag() error = %v", err)
	}
	if len(canonical.Variations) != 2 ||
		canonical.Variations[0].ID != providerFeatureVariationOne ||
		canonical.Variations[0].Name != "One" || canonical.Variations[0].Value != "1" ||
		canonical.Variations[1].ID != providerFeatureVariationTwo ||
		canonical.Variations[1].Name != "Two" || canonical.Variations[1].Value != "2" {
		t.Fatal("remote variation reordering did not correlate by exact UUID")
	}

	sorted, err := canonicalizeRemoteFeatureFlag(remote, nil)
	if err != nil || sorted.Variations[0].ID != providerFeatureVariationOne ||
		sorted.Variations[1].ID != providerFeatureVariationTwo {
		t.Fatal("unowned remote variation ordering was not canonicalized by UUID")
	}
	model, err := flattenFeatureFlag(remote, []string{
		providerFeatureVariationOne,
		providerFeatureVariationTwo,
	})
	if err != nil || model.ID.ValueString() != providerFeatureFlagID ||
		model.Variations[0].ID.ValueString() != providerFeatureVariationOne ||
		model.Variations[1].Value.ValueString() != "2" {
		t.Fatal("flattenFeatureFlag() did not preserve UUID-correlated canonical state")
	}
}

func TestCanonicalizeRemoteFeatureFlagRejectsMissingDuplicateOrInvalidIdentity(t *testing.T) {
	t.Parallel()

	valid := providerFeatureFlagForCanonicalTest()
	tests := map[string]struct {
		flag  client.FeatureFlag
		order []string
	}{
		"invalid flag ID": {
			flag: withProviderFeatureFlag(valid, func(flag *client.FeatureFlag) {
				flag.ID = "invalid"
			}),
		},
		"invalid environment ID": {
			flag: withProviderFeatureFlag(valid, func(flag *client.FeatureFlag) {
				flag.EnvironmentID = "invalid"
			}),
		},
		"invalid key": {
			flag: withProviderFeatureFlag(valid, func(flag *client.FeatureFlag) {
				flag.Key = "invalid/key"
			}),
		},
		"blank name": {
			flag: withProviderFeatureFlag(valid, func(flag *client.FeatureFlag) {
				flag.Name = " "
			}),
		},
		"non-canonical type": {
			flag: withProviderFeatureFlag(valid, func(flag *client.FeatureFlag) {
				flag.VariationType = "String"
			}),
		},
		"missing variations": {
			flag: withProviderFeatureFlag(valid, func(flag *client.FeatureFlag) {
				flag.Variations = nil
			}),
		},
		"missing variation ID": {
			flag: withProviderFeatureFlag(valid, func(flag *client.FeatureFlag) {
				flag.Variations[0].ID = ""
			}),
		},
		"duplicate variation ID": {
			flag: withProviderFeatureFlag(valid, func(flag *client.FeatureFlag) {
				flag.Variations[1].ID = strings.ToUpper(flag.Variations[0].ID)
			}),
		},
		"blank variation name": {
			flag: withProviderFeatureFlag(valid, func(flag *client.FeatureFlag) {
				flag.Variations[0].Name = " "
			}),
		},
		"empty variation value": {
			flag: withProviderFeatureFlag(valid, func(flag *client.FeatureFlag) {
				flag.Variations[0].Value = ""
			}),
		},
		"invalid typed value": {
			flag: withProviderFeatureFlag(valid, func(flag *client.FeatureFlag) {
				flag.VariationType = featureFlagVariationTypeBoolean
				flag.Variations[0].Value = "yes"
			}),
		},
		"order omits variation": {
			flag:  valid,
			order: []string{providerFeatureVariationOne},
		},
		"order contains unknown ID": {
			flag: valid,
			order: []string{
				providerFeatureVariationOne,
				"ffffffff-ffff-4fff-8fff-ffffffffffff",
			},
		},
		"order contains duplicate ID": {
			flag: valid,
			order: []string{
				providerFeatureVariationOne,
				strings.ToUpper(providerFeatureVariationOne),
			},
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := canonicalizeRemoteFeatureFlag(test.flag, test.order)
			if err == nil {
				t.Fatal("invalid remote Feature Flag definition was accepted")
			}
		})
	}
}

func TestFeatureFlagCanonicalTypesRedactFormatting(t *testing.T) {
	t.Parallel()

	canonical, seed, err := canonicalizePlannedFeatureFlag(
		providerEnvironmentA,
		"runtime-key-marker",
		"Runtime Name",
		"Runtime Description",
		featureFlagVariationTypeString,
		[]featureFlagVariationInput{{Name: "Runtime Variation", Value: "runtime-value-marker"}},
	)
	if err != nil {
		t.Fatal("could not construct canonical Feature Flag for redaction test")
	}
	model := flattenCanonicalFeatureFlag(canonical)
	formatted := fmt.Sprintf(
		"%v|%+v|%#v|%v|%+v|%#v|%v|%+v|%#v|%v|%+v|%#v",
		canonical,
		canonical,
		canonical,
		canonical.Variations[0],
		canonical.Variations[0],
		canonical.Variations[0],
		seed,
		seed,
		seed,
		model,
		model,
		model,
	)
	for _, unsafe := range []string{
		providerEnvironmentA,
		"runtime-key-marker",
		"runtime-value-marker",
		canonical.Variations[0].ID,
	} {
		if strings.Contains(formatted, unsafe) {
			t.Fatal("formatted canonical Feature Flag data exposed a runtime identity or value")
		}
	}
}

func providerFeatureFlagForCanonicalTest() client.FeatureFlag {
	return client.FeatureFlag{
		ID:            providerFeatureFlagID,
		EnvironmentID: providerEnvironmentA,
		Name:          "Feature Flag",
		Description:   "",
		Key:           "feature-key",
		VariationType: featureFlagVariationTypeString,
		Variations: []client.FeatureFlagVariation{
			{ID: providerFeatureVariationOne, Name: "One", Value: "one"},
			{ID: providerFeatureVariationTwo, Name: "Two", Value: "two"},
		},
	}
}

func withProviderFeatureFlag(
	flag client.FeatureFlag,
	change func(*client.FeatureFlag),
) client.FeatureFlag {
	flag.Variations = append([]client.FeatureFlagVariation(nil), flag.Variations...)
	change(&flag)
	return flag
}
