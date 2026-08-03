// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	featureFlagVariationTypeBoolean = "boolean"
	featureFlagVariationTypeString  = "string"
	featureFlagVariationTypeNumber  = "number"
	featureFlagVariationTypeJSON    = "json"

	featureFlagMaximumNameLength = 128
	featureFlagMaximumKeyLength  = 128
	maximumPlainNumberExpansion  = int64(1024)
)

var (
	featureFlagKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	jsonNumberPattern     = regexp.MustCompile(
		`^(-?)(0|[1-9][0-9]*)(?:\.([0-9]+))?(?:[eE]([+-]?[0-9]+))?$`,
	)
	errInvalidFeatureFlagDefinition = errors.New("Feature Flag definition is invalid")
	errInvalidFeatureFlagValue      = errors.New("Feature Flag variation value is invalid")
)

type featureFlagModel struct {
	EnvironmentID types.String                `tfsdk:"environment_id"`
	ID            types.String                `tfsdk:"id"`
	Name          types.String                `tfsdk:"name"`
	Description   types.String                `tfsdk:"description"`
	Key           types.String                `tfsdk:"key"`
	VariationType types.String                `tfsdk:"variation_type"`
	Variations    []featureFlagVariationModel `tfsdk:"variations"`
}

func (featureFlagModel) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.featureFlagModel{redacted}")
}

type featureFlagVariationModel struct {
	ID    types.String `tfsdk:"id"`
	Name  types.String `tfsdk:"name"`
	Value types.String `tfsdk:"value"`
}

func (featureFlagVariationModel) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.featureFlagVariationModel{redacted}")
}

type featureFlagVariationInput struct {
	Name  string
	Value string
}

type canonicalFeatureFlag struct {
	EnvironmentID string
	ID            string
	Name          string
	Description   string
	Key           string
	VariationType string
	Variations    []canonicalFeatureFlagVariation
}

func (canonicalFeatureFlag) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.canonicalFeatureFlag{redacted}")
}

type canonicalFeatureFlagVariation struct {
	ID    string
	Name  string
	Value string
}

func (canonicalFeatureFlagVariation) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.canonicalFeatureFlagVariation{redacted}")
}

// featureFlagCreateSeed contains only the API-required operational fields
// initialized during a later Create. Both selections point at the first
// deterministic variation while the flag is disabled, so initial on/off
// serving behavior is identical until a user deliberately changes UI-owned
// settings.
type featureFlagCreateSeed struct {
	IsEnabled           bool
	EnabledVariationID  string
	DisabledVariationID string
	Tags                []string
}

func (featureFlagCreateSeed) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.featureFlagCreateSeed{redacted}")
}

func canonicalizePlannedFeatureFlag(
	environmentID string,
	key string,
	name string,
	description string,
	variationType string,
	variations []featureFlagVariationInput,
) (canonicalFeatureFlag, featureFlagCreateSeed, error) {
	canonicalType, err := validateFeatureFlagDefinitionFields(
		environmentID,
		key,
		name,
		variationType,
	)
	if err != nil || len(variations) == 0 {
		return canonicalFeatureFlag{}, featureFlagCreateSeed{}, errInvalidFeatureFlagDefinition
	}

	canonical := canonicalFeatureFlag{
		EnvironmentID: environmentID,
		Name:          name,
		Description:   description,
		Key:           key,
		VariationType: canonicalType,
		Variations:    make([]canonicalFeatureFlagVariation, 0, len(variations)),
	}
	for index, variation := range variations {
		if !validFeatureFlagVariationName(variation.Name) || variation.Value == "" {
			return canonicalFeatureFlag{}, featureFlagCreateSeed{}, errInvalidFeatureFlagDefinition
		}
		value, err := canonicalizeFeatureFlagValue(canonicalType, variation.Value)
		if err != nil {
			return canonicalFeatureFlag{}, featureFlagCreateSeed{}, err
		}
		canonical.Variations = append(canonical.Variations, canonicalFeatureFlagVariation{
			ID:    deterministicFeatureFlagVariationID(environmentID, key, index),
			Name:  variation.Name,
			Value: value,
		})
	}

	seed, err := initialFeatureFlagCreateSeed(canonical.Variations)
	if err != nil {
		return canonicalFeatureFlag{}, featureFlagCreateSeed{}, err
	}
	return canonical, seed, nil
}

func canonicalizeRemoteFeatureFlag(
	flag client.FeatureFlag,
	variationOrder []string,
) (canonicalFeatureFlag, error) {
	flagID, valid := client.CanonicalUUID(flag.ID)
	if !valid {
		return canonicalFeatureFlag{}, errInvalidFeatureFlagDefinition
	}
	canonicalType, err := validateFeatureFlagDefinitionFields(
		flag.EnvironmentID,
		flag.Key,
		flag.Name,
		flag.VariationType,
	)
	if err != nil || len(flag.Variations) == 0 {
		return canonicalFeatureFlag{}, errInvalidFeatureFlagDefinition
	}

	canonical := canonicalFeatureFlag{
		EnvironmentID: flag.EnvironmentID,
		ID:            flagID,
		Name:          flag.Name,
		Description:   flag.Description,
		Key:           flag.Key,
		VariationType: canonicalType,
		Variations:    make([]canonicalFeatureFlagVariation, 0, len(flag.Variations)),
	}
	byID := make(map[string]canonicalFeatureFlagVariation, len(flag.Variations))
	for _, variation := range flag.Variations {
		id, valid := client.CanonicalUUID(variation.ID)
		if !valid || !validFeatureFlagVariationName(variation.Name) || variation.Value == "" {
			return canonicalFeatureFlag{}, errInvalidFeatureFlagDefinition
		}
		if _, duplicate := byID[id]; duplicate {
			return canonicalFeatureFlag{}, errInvalidFeatureFlagDefinition
		}
		value, err := canonicalizeFeatureFlagValue(canonicalType, variation.Value)
		if err != nil {
			return canonicalFeatureFlag{}, err
		}
		byID[id] = canonicalFeatureFlagVariation{
			ID:    id,
			Name:  variation.Name,
			Value: value,
		}
	}

	if variationOrder == nil {
		for _, variation := range byID {
			canonical.Variations = append(canonical.Variations, variation)
		}
		sort.SliceStable(canonical.Variations, func(left, right int) bool {
			return canonical.Variations[left].ID < canonical.Variations[right].ID
		})
		return canonical, nil
	}

	if len(variationOrder) != len(byID) {
		return canonicalFeatureFlag{}, errInvalidFeatureFlagDefinition
	}
	seenOrder := make(map[string]struct{}, len(variationOrder))
	for _, requestedID := range variationOrder {
		normalizedID, valid := client.CanonicalUUID(requestedID)
		if !valid {
			return canonicalFeatureFlag{}, errInvalidFeatureFlagDefinition
		}
		if _, duplicate := seenOrder[normalizedID]; duplicate {
			return canonicalFeatureFlag{}, errInvalidFeatureFlagDefinition
		}
		seenOrder[normalizedID] = struct{}{}
		variation, exists := byID[normalizedID]
		if !exists {
			return canonicalFeatureFlag{}, errInvalidFeatureFlagDefinition
		}
		canonical.Variations = append(canonical.Variations, variation)
	}
	return canonical, nil
}

func flattenFeatureFlag(
	flag client.FeatureFlag,
	variationOrder []string,
) (featureFlagModel, error) {
	canonical, err := canonicalizeRemoteFeatureFlag(flag, variationOrder)
	if err != nil {
		return featureFlagModel{}, err
	}
	return flattenCanonicalFeatureFlag(canonical), nil
}

func flattenCanonicalFeatureFlag(flag canonicalFeatureFlag) featureFlagModel {
	model := featureFlagModel{
		EnvironmentID: types.StringValue(flag.EnvironmentID),
		ID:            types.StringValue(flag.ID),
		Name:          types.StringValue(flag.Name),
		Description:   types.StringValue(flag.Description),
		Key:           types.StringValue(flag.Key),
		VariationType: types.StringValue(flag.VariationType),
		Variations:    make([]featureFlagVariationModel, 0, len(flag.Variations)),
	}
	for _, variation := range flag.Variations {
		model.Variations = append(model.Variations, featureFlagVariationModel{
			ID:    types.StringValue(variation.ID),
			Name:  types.StringValue(variation.Name),
			Value: types.StringValue(variation.Value),
		})
	}
	return model
}

func validateFeatureFlagDefinitionFields(
	environmentID string,
	key string,
	name string,
	variationType string,
) (string, error) {
	if !client.ValidUUID(environmentID) || !validFeatureFlagKey(key) ||
		!validFeatureFlagName(name) {
		return "", errInvalidFeatureFlagDefinition
	}
	return canonicalizeFeatureFlagVariationType(variationType)
}

func canonicalizeFeatureFlagVariationType(value string) (string, error) {
	switch value {
	case featureFlagVariationTypeBoolean,
		featureFlagVariationTypeString,
		featureFlagVariationTypeNumber,
		featureFlagVariationTypeJSON:
		return value, nil
	default:
		return "", errInvalidFeatureFlagDefinition
	}
}

func canonicalizeFeatureFlagValue(variationType string, value string) (string, error) {
	switch variationType {
	case featureFlagVariationTypeBoolean:
		if strings.EqualFold(value, "true") {
			return "true", nil
		}
		if strings.EqualFold(value, "false") {
			return "false", nil
		}
		return "", errInvalidFeatureFlagValue
	case featureFlagVariationTypeString:
		return value, nil
	case featureFlagVariationTypeNumber:
		return canonicalizeFeatureFlagNumber(value)
	case featureFlagVariationTypeJSON:
		return canonicalizeFeatureFlagJSON(value)
	default:
		return "", errInvalidFeatureFlagDefinition
	}
}

func canonicalizeFeatureFlagNumber(value string) (string, error) {
	matches := jsonNumberPattern.FindStringSubmatch(value)
	if matches == nil {
		return "", errInvalidFeatureFlagValue
	}

	digits := matches[2] + matches[3]
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		return "0", nil
	}

	var power big.Int
	exponent := matches[4]
	if exponent != "" {
		exponent = strings.TrimPrefix(exponent, "+")
		if _, ok := power.SetString(exponent, 10); !ok {
			return "", errInvalidFeatureFlagValue
		}
	}
	power.Sub(&power, big.NewInt(int64(len(matches[3]))))

	trimmedDigits := strings.TrimRight(digits, "0")
	trailingZeroes := len(digits) - len(trimmedDigits)
	digits = trimmedDigits
	power.Add(&power, big.NewInt(int64(trailingZeroes)))

	decimalPosition := new(big.Int).Add(&power, big.NewInt(int64(len(digits))))
	plain, ok := plainCanonicalNumber(digits, decimalPosition)
	if ok {
		if matches[1] == "-" {
			return "-" + plain, nil
		}
		return plain, nil
	}

	mantissa := digits[:1]
	if len(digits) > 1 {
		mantissa += "." + digits[1:]
	}
	scientificExponent := new(big.Int).Sub(decimalPosition, big.NewInt(1))
	if scientificExponent.Sign() != 0 {
		mantissa += "e" + scientificExponent.String()
	}
	if matches[1] == "-" {
		mantissa = "-" + mantissa
	}
	return mantissa, nil
}

func plainCanonicalNumber(digits string, decimalPosition *big.Int) (string, bool) {
	if !decimalPosition.IsInt64() {
		return "", false
	}
	position := decimalPosition.Int64()
	digitCount := int64(len(digits))
	switch {
	case position > 0 && position < digitCount:
		index := int(position)
		return digits[:index] + "." + digits[index:], true
	case position == digitCount:
		return digits, true
	case position > digitCount && position-digitCount <= maximumPlainNumberExpansion:
		return digits + strings.Repeat("0", int(position-digitCount)), true
	case position <= 0 && position >= -maximumPlainNumberExpansion:
		return "0." + strings.Repeat("0", int(-position)) + digits, true
	default:
		return "", false
	}
}

func canonicalizeFeatureFlagJSON(value string) (string, error) {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	decoded, err := decodeFeatureFlagJSONValue(decoder)
	if err != nil {
		return "", errInvalidFeatureFlagValue
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return "", errInvalidFeatureFlagValue
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return "", errInvalidFeatureFlagValue
	}
	return string(canonical), nil
}

func decodeFeatureFlagJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch typed := token.(type) {
	case json.Delim:
		switch typed {
		case '{':
			object := make(map[string]any)
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, errInvalidFeatureFlagValue
				}
				if _, duplicate := object[key]; duplicate {
					return nil, errInvalidFeatureFlagValue
				}
				item, err := decodeFeatureFlagJSONValue(decoder)
				if err != nil {
					return nil, err
				}
				object[key] = item
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim('}') {
				return nil, errInvalidFeatureFlagValue
			}
			return object, nil
		case '[':
			array := make([]any, 0)
			for decoder.More() {
				item, err := decodeFeatureFlagJSONValue(decoder)
				if err != nil {
					return nil, err
				}
				array = append(array, item)
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim(']') {
				return nil, errInvalidFeatureFlagValue
			}
			return array, nil
		default:
			return nil, errInvalidFeatureFlagValue
		}
	case json.Number:
		canonical, err := canonicalizeFeatureFlagNumber(typed.String())
		if err != nil {
			return nil, err
		}
		return json.Number(canonical), nil
	case string, bool, nil:
		return typed, nil
	default:
		return nil, errInvalidFeatureFlagValue
	}
}

func deterministicFeatureFlagVariationID(environmentID, key string, index int) string {
	canonicalEnvironmentID, valid := client.CanonicalUUID(environmentID)
	if !valid {
		return ""
	}
	name := strings.Join([]string{
		"terraform-provider-featbit/feature-flag-variation/v1",
		canonicalEnvironmentID,
		key,
		strconv.Itoa(index),
	}, "\x00")
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(name)).String()
}

func initialFeatureFlagCreateSeed(
	variations []canonicalFeatureFlagVariation,
) (featureFlagCreateSeed, error) {
	if len(variations) == 0 {
		return featureFlagCreateSeed{}, errInvalidFeatureFlagDefinition
	}
	if _, valid := client.CanonicalUUID(variations[0].ID); !valid {
		return featureFlagCreateSeed{}, errInvalidFeatureFlagDefinition
	}
	return featureFlagCreateSeed{
		IsEnabled:           false,
		EnabledVariationID:  variations[0].ID,
		DisabledVariationID: variations[0].ID,
		Tags:                make([]string, 0),
	}, nil
}

func validFeatureFlagKey(value string) bool {
	return value != "" && len(value) <= featureFlagMaximumKeyLength &&
		featureFlagKeyPattern.MatchString(value)
}

func validFeatureFlagName(value string) bool {
	return strings.TrimSpace(value) != "" && utf16Length(value) <= featureFlagMaximumNameLength
}

func validFeatureFlagVariationName(value string) bool {
	return strings.TrimSpace(value) != ""
}

func utf16Length(value string) int {
	length := 0
	for _, r := range value {
		length += utf16.RuneLen(r)
	}
	return length
}
