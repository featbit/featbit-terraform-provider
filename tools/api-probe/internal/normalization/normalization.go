package normalization

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

const RolloutScale = 100000

type Variation struct {
	ID    string
	Name  string
	Value string
}

type Rollout struct {
	VariationID string
	Start       float64
	End         float64
}

type Condition struct {
	ID       string
	Property string
	Operator string
	Values   []string
}

type Rule struct {
	ID         string
	Name       string
	Conditions []Condition
	Rollouts   []Rollout
}

type Target struct {
	VariationID string
	Keys        []string
}

type Flag struct {
	VariationType       string
	Variations          []Variation
	DisabledVariationID string
	Fallthrough         []Rollout
	Targets             []Target
	Rules               []Rule
	Tags                []string
}

type Segment struct {
	Type       string
	Scopes     []string
	Included   []string
	Excluded   []string
	Rules      []Rule
	Tags       []string
	IsArchived bool
}

func CanonicalJSON(input string) (string, error) {
	var value interface{}
	decoder := json.NewDecoder(bytes.NewBufferString(input))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", errors.New("variation value is not valid JSON")
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", errors.New("variation value contains multiple JSON values")
	}
	value, err := normalizeJSONValue(value)
	if err != nil {
		return "", err
	}
	output, err := json.Marshal(value)
	if err != nil {
		return "", errors.New("canonicalize JSON variation")
	}
	return string(output), nil
}

func CanonicalVariationValue(variationType, value string) (string, error) {
	switch variationType {
	case "boolean":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return "", errors.New("boolean variation must be true or false")
		}
		return strconv.FormatBool(parsed), nil
	case "number":
		var parsed json.Number
		decoder := json.NewDecoder(bytes.NewBufferString(value))
		decoder.UseNumber()
		if err := decoder.Decode(&parsed); err != nil {
			return "", errors.New("number variation must be a finite JSON number")
		}
		var extra interface{}
		if err := decoder.Decode(&extra); err != io.EOF {
			return "", errors.New("number variation must contain exactly one JSON number")
		}
		return normalizeJSONNumber(parsed.String())
	case "json":
		return CanonicalJSON(value)
	case "string":
		return value, nil
	default:
		return "", fmt.Errorf("unsupported variation type %q", variationType)
	}
}

func WeightsToRollouts(variationIDs []string, weights []int) ([]Rollout, error) {
	if len(variationIDs) == 0 || len(variationIDs) != len(weights) {
		return nil, errors.New("rollout variation IDs and weights must have the same non-zero length")
	}
	total := 0
	for _, weight := range weights {
		if weight < 0 {
			return nil, errors.New("rollout weights must be non-negative")
		}
		total += weight
	}
	if total != RolloutScale {
		return nil, fmt.Errorf("rollout weights total %d, want %d", total, RolloutScale)
	}

	result := make([]Rollout, len(weights))
	cursor := 0
	for index, weight := range weights {
		next := cursor + weight
		result[index] = Rollout{
			VariationID: variationIDs[index],
			Start:       float64(cursor) / RolloutScale,
			End:         float64(next) / RolloutScale,
		}
		cursor = next
	}
	return result, nil
}

func RolloutsToWeights(rollouts []Rollout) ([]int, error) {
	if len(rollouts) == 0 {
		return nil, errors.New("at least one rollout is required")
	}
	const tolerance = 0.5 / RolloutScale
	weights := make([]int, len(rollouts))
	cursor := 0.0
	total := 0
	for index, rollout := range rollouts {
		if math.Abs(rollout.Start-cursor) > tolerance || rollout.End+float64(tolerance) < rollout.Start ||
			rollout.End < 0 || rollout.End > 1 {
			return nil, errors.New("rollout ranges must be contiguous, ordered, and within [0,1]")
		}
		weight := int(math.Round((rollout.End - rollout.Start) * RolloutScale))
		if weight < 0 {
			return nil, errors.New("rollout range produced a negative weight")
		}
		weights[index] = weight
		total += weight
		cursor = rollout.End
	}
	if math.Abs(cursor-1) > tolerance || total != RolloutScale {
		return nil, errors.New("rollout ranges must cover exactly [0,1]")
	}
	return weights, nil
}

func EncodeConditionValues(values []string) (string, error) {
	content, err := json.Marshal(values)
	if err != nil {
		return "", errors.New("encode condition values")
	}
	return string(content), nil
}

func DecodeConditionValues(value string) ([]string, error) {
	var values []string
	if err := json.Unmarshal([]byte(value), &values); err != nil {
		return nil, errors.New("condition value is not a JSON string array")
	}
	if values == nil {
		return []string{}, nil
	}
	return values, nil
}

func VariationIndex(variations []Variation, id string) (int, error) {
	found := -1
	for index, variation := range variations {
		if variation.ID == id {
			if found != -1 {
				return -1, fmt.Errorf("variation ID %q is duplicated", id)
			}
			found = index
		}
	}
	if found == -1 {
		return -1, fmt.Errorf("variation ID %q is not present", id)
	}
	return found, nil
}

func VariationID(variations []Variation, index int) (string, error) {
	if index < 0 || index >= len(variations) {
		return "", fmt.Errorf("variation index %d is out of range", index)
	}
	if variations[index].ID == "" {
		return "", fmt.Errorf("variation index %d has no stable ID", index)
	}
	return variations[index].ID, nil
}

func CanonicalFlag(input Flag) (Flag, error) {
	result := input
	result.Variations = append([]Variation(nil), input.Variations...)
	for index := range result.Variations {
		value, err := CanonicalVariationValue(result.VariationType, result.Variations[index].Value)
		if err != nil {
			return Flag{}, fmt.Errorf("variation %d: %w", index, err)
		}
		result.Variations[index].Value = value
	}
	if _, err := VariationIndex(result.Variations, result.DisabledVariationID); err != nil {
		return Flag{}, fmt.Errorf("disabled variation: %w", err)
	}
	if err := validateRolloutIDs(result.Variations, result.Fallthrough); err != nil {
		return Flag{}, fmt.Errorf("fallthrough: %w", err)
	}
	if _, err := RolloutsToWeights(result.Fallthrough); err != nil {
		return Flag{}, fmt.Errorf("fallthrough: %w", err)
	}

	result.Tags = canonicalSet(input.Tags)
	result.Targets = append([]Target(nil), input.Targets...)
	variationOrder := make(map[string]int, len(result.Variations))
	for index, variation := range result.Variations {
		variationOrder[variation.ID] = index
	}
	for index := range result.Targets {
		if _, ok := variationOrder[result.Targets[index].VariationID]; !ok {
			return Flag{}, fmt.Errorf("target %d references an unknown variation", index)
		}
		result.Targets[index].Keys = canonicalSet(result.Targets[index].Keys)
	}
	sort.SliceStable(result.Targets, func(left, right int) bool {
		return variationOrder[result.Targets[left].VariationID] < variationOrder[result.Targets[right].VariationID]
	})
	result.Rules = cloneRules(input.Rules)
	for ruleIndex := range result.Rules {
		if err := validateRolloutIDs(result.Variations, result.Rules[ruleIndex].Rollouts); err != nil {
			return Flag{}, fmt.Errorf("rule %d: %w", ruleIndex, err)
		}
		if _, err := RolloutsToWeights(result.Rules[ruleIndex].Rollouts); err != nil {
			return Flag{}, fmt.Errorf("rule %d: %w", ruleIndex, err)
		}
	}
	return result, nil
}

func CanonicalSegment(input Segment) Segment {
	result := input
	result.Scopes = canonicalSet(input.Scopes)
	result.Included = canonicalSet(input.Included)
	result.Excluded = canonicalSet(input.Excluded)
	result.Tags = canonicalSet(input.Tags)
	result.Rules = cloneRules(input.Rules)
	return result
}

func canonicalSet(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func cloneRules(input []Rule) []Rule {
	result := make([]Rule, len(input))
	for index, rule := range input {
		result[index] = rule
		result[index].Conditions = append([]Condition(nil), rule.Conditions...)
		for conditionIndex := range result[index].Conditions {
			result[index].Conditions[conditionIndex].Values =
				append([]string(nil), rule.Conditions[conditionIndex].Values...)
		}
		result[index].Rollouts = append([]Rollout(nil), rule.Rollouts...)
	}
	return result
}

func validateRolloutIDs(variations []Variation, rollouts []Rollout) error {
	known := make(map[string]struct{}, len(variations))
	for _, variation := range variations {
		if variation.ID == "" {
			return errors.New("variation has no stable ID")
		}
		if _, duplicate := known[variation.ID]; duplicate {
			return fmt.Errorf("variation ID %q is duplicated", variation.ID)
		}
		known[variation.ID] = struct{}{}
	}
	for _, rollout := range rollouts {
		if _, ok := known[rollout.VariationID]; !ok {
			return fmt.Errorf("rollout references unknown variation ID %q", rollout.VariationID)
		}
	}
	return nil
}

func normalizeJSONValue(value interface{}) (interface{}, error) {
	switch typed := value.(type) {
	case json.Number:
		normalized, err := normalizeJSONNumber(typed.String())
		if err != nil {
			return nil, err
		}
		return json.Number(normalized), nil
	case []interface{}:
		result := make([]interface{}, len(typed))
		for index, item := range typed {
			normalized, err := normalizeJSONValue(item)
			if err != nil {
				return nil, err
			}
			result[index] = normalized
		}
		return result, nil
	case map[string]interface{}:
		result := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			normalized, err := normalizeJSONValue(item)
			if err != nil {
				return nil, err
			}
			result[key] = normalized
		}
		return result, nil
	default:
		return value, nil
	}
}

// normalizeJSONNumber produces an exact decimal representation without
// converting through float64, so large integer and decimal values retain
// precision.
func normalizeJSONNumber(value string) (string, error) {
	sign := ""
	if len(value) > 0 && value[0] == '-' {
		sign = "-"
		value = value[1:]
	}
	exponent := 0
	if index := strings.IndexAny(value, "eE"); index >= 0 {
		parsedExponent, err := strconv.Atoi(value[index+1:])
		if err != nil || parsedExponent < -10000 || parsedExponent > 10000 {
			return "", errors.New("JSON number exponent is outside the supported canonical range")
		}
		exponent = parsedExponent
		value = value[:index]
	}
	fractionDigits := 0
	if index := strings.IndexByte(value, '.'); index >= 0 {
		fractionDigits = len(value) - index - 1
		value = value[:index] + value[index+1:]
	}
	digits := strings.TrimLeft(value, "0")
	if digits == "" {
		return "0", nil
	}
	scale := fractionDigits - exponent
	for scale > 0 && strings.HasSuffix(digits, "0") {
		digits = strings.TrimSuffix(digits, "0")
		scale--
	}
	var normalized string
	switch {
	case scale <= 0:
		normalized = digits + strings.Repeat("0", -scale)
	case len(digits) > scale:
		split := len(digits) - scale
		normalized = digits[:split] + "." + digits[split:]
	default:
		normalized = "0." + strings.Repeat("0", scale-len(digits)) + digits
	}
	return sign + normalized, nil
}
