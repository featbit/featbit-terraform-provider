package normalization

import (
	"reflect"
	"testing"
)

func TestVariationValueCanonicalization(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		variationType string
		input         string
		want          string
	}{
		"boolean": {variationType: "boolean", input: "TRUE", want: "true"},
		"number":  {variationType: "number", input: "1.000", want: "1"},
		"large precise number": {
			variationType: "number",
			input:         "9007199254740993.000",
			want:          "9007199254740993",
		},
		"string": {variationType: "string", input: "  unchanged  ", want: "  unchanged  "},
		"json": {
			variationType: "json",
			input:         `{ "z": 2, "a": [true, 1.0, 9007199254740993] }`,
			want:          `{"a":[true,1,9007199254740993],"z":2}`,
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := CanonicalVariationValue(test.variationType, test.input)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("CanonicalVariationValue() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRolloutWeightRangeRoundTrip(t *testing.T) {
	t.Parallel()

	weights := []int{25000, 0, 75000}
	rollouts, err := WeightsToRollouts([]string{"a", "b", "c"}, weights)
	if err != nil {
		t.Fatal(err)
	}
	if rollouts[0].Start != 0 || rollouts[0].End != 0.25 ||
		rollouts[1].Start != 0.25 || rollouts[1].End != 0.25 ||
		rollouts[2].Start != 0.25 || rollouts[2].End != 1 {
		t.Fatalf("unexpected ranges: %+v", rollouts)
	}
	got, err := RolloutsToWeights(rollouts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, weights) {
		t.Fatalf("round-trip weights = %v, want %v", got, weights)
	}
}

func TestConditionValueRoundTrip(t *testing.T) {
	t.Parallel()

	values := []string{"one", `two"quoted`, ""}
	encoded, err := EncodeConditionValues(values)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeConditionValues(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, values) {
		t.Fatalf("condition values = %#v, want %#v", decoded, values)
	}
}

func TestCanonicalComplexFlagProducesEmptyLogicalDiff(t *testing.T) {
	t.Parallel()

	variations := []Variation{
		{ID: "variation-a", Name: "A", Value: `{"b":2,"a":1}`},
		{ID: "variation-b", Name: "B", Value: `{ "a": 3, "b": 4 }`},
	}
	rollouts, err := WeightsToRollouts([]string{"variation-a", "variation-b"}, []int{30000, 70000})
	if err != nil {
		t.Fatal(err)
	}
	desired := Flag{
		VariationType:       "json",
		Variations:          variations,
		DisabledVariationID: "variation-b",
		Fallthrough:         rollouts,
		Targets: []Target{
			{VariationID: "variation-b", Keys: []string{"user-b", "user-a"}},
			{VariationID: "variation-a", Keys: []string{"user-c"}},
		},
		Rules: []Rule{{
			ID:   "rule-1",
			Name: "first",
			Conditions: []Condition{{
				ID:       "condition-1",
				Property: "country",
				Operator: "IsOneOf",
				Values:   []string{"US", "CA"},
			}},
			Rollouts: rollouts,
		}},
		Tags: []string{"team-b", "team-a"},
	}
	observed := desired
	observed.Variations = append([]Variation(nil), desired.Variations...)
	observed.Variations[0].Value = `{ "a": 1.0, "b": 2 }`
	observed.Targets = []Target{
		{VariationID: "variation-a", Keys: []string{"user-c"}},
		{VariationID: "variation-b", Keys: []string{"user-a", "user-b", "user-a"}},
	}
	observed.Tags = []string{"team-a", "team-b", "team-a"}

	canonicalDesired, err := CanonicalFlag(desired)
	if err != nil {
		t.Fatal(err)
	}
	canonicalObserved, err := CanonicalFlag(observed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(canonicalDesired, canonicalObserved) {
		t.Fatalf("canonical flag differs:\ndesired=%+v\nobserved=%+v", canonicalDesired, canonicalObserved)
	}
	if canonicalDesired.Rules[0].ID != "rule-1" ||
		canonicalDesired.Rules[0].Conditions[0].ID != "condition-1" {
		t.Fatal("meaningful rule/condition order or identity was not preserved")
	}
}

func TestCanonicalComplexSegmentUsesSetsAndPreservesRuleOrder(t *testing.T) {
	t.Parallel()

	input := Segment{
		Type:     "shared",
		Scopes:   []string{"environment-b", "environment-a"},
		Included: []string{"user-b", "user-a", "user-b"},
		Excluded: []string{"user-d", "user-c"},
		Rules: []Rule{
			{ID: "rule-2", Name: "second"},
			{ID: "rule-1", Name: "first"},
		},
		Tags: []string{"tag-b", "tag-a"},
	}
	got := CanonicalSegment(input)
	if !reflect.DeepEqual(got.Scopes, []string{"environment-a", "environment-b"}) ||
		!reflect.DeepEqual(got.Included, []string{"user-a", "user-b"}) ||
		!reflect.DeepEqual(got.Tags, []string{"tag-a", "tag-b"}) {
		t.Fatalf("segment sets were not canonicalized: %+v", got)
	}
	if got.Rules[0].ID != "rule-2" || got.Rules[1].ID != "rule-1" {
		t.Fatalf("rule order changed: %+v", got.Rules)
	}
}

func TestVariationIdentityMappingRejectsAmbiguity(t *testing.T) {
	t.Parallel()

	variations := []Variation{{ID: "a"}, {ID: "b"}}
	if index, err := VariationIndex(variations, "b"); err != nil || index != 1 {
		t.Fatalf("VariationIndex() = %d, %v", index, err)
	}
	if id, err := VariationID(variations, 0); err != nil || id != "a" {
		t.Fatalf("VariationID() = %q, %v", id, err)
	}
	if _, err := VariationIndex([]Variation{{ID: "a"}, {ID: "a"}}, "a"); err == nil {
		t.Fatal("duplicate variation ID was accepted")
	}
	if _, err := VariationID(variations, 2); err == nil {
		t.Fatal("out-of-range variation index was accepted")
	}
}
