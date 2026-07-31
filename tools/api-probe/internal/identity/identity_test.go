package identity

import "testing"

const (
	firstUUID  = "11111111-1111-1111-1111-111111111111"
	secondUUID = "22222222-2222-2222-2222-222222222222"
)

func TestParseStableImportIdentities(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		kind  Kind
		value string
	}{
		"project":       {kind: Project, value: firstUUID},
		"environment":   {kind: Environment, value: firstUUID + "/" + secondUUID},
		"feature flag":  {kind: FeatureFlag, value: firstUUID + "/exact.flag-key_1"},
		"segment":       {kind: Segment, value: firstUUID + "/" + secondUUID},
		"group":         {kind: Group, value: firstUUID},
		"policy":        {kind: Policy, value: firstUUID},
		"member":        {kind: Member, value: firstUUID},
		"group member":  {kind: GroupMember, value: firstUUID + "/" + secondUUID},
		"group policy":  {kind: GroupPolicy, value: firstUUID + "/" + secondUUID},
		"member policy": {kind: MemberPolicy, value: firstUUID + "/" + secondUUID},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse(test.kind, test.value); err != nil {
				t.Fatalf("Parse(%s, %q): %v", test.kind, test.value, err)
			}
		})
	}
}

func TestParseRejectsAmbiguousOrFuzzyImportIdentities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind  Kind
		value string
	}{
		{kind: Project, value: "project-name"},
		{kind: Environment, value: firstUUID},
		{kind: Environment, value: firstUUID + "/" + secondUUID + "/extra"},
		{kind: FeatureFlag, value: firstUUID + "/key/extra"},
		{kind: FeatureFlag, value: firstUUID + "/fuzzy key"},
		{kind: Segment, value: firstUUID + "/segment-key"},
		{kind: GroupMember, value: " " + firstUUID + "/" + secondUUID},
	}
	for _, test := range tests {
		if _, err := Parse(test.kind, test.value); err == nil {
			t.Fatalf("Parse(%s, %q) accepted an ambiguous identity", test.kind, test.value)
		}
	}
}
