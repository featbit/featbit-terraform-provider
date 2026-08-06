// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package toolenv

import (
	"strings"
	"testing"
)

func TestSanitized(t *testing.T) {
	t.Setenv("FEATBIT_ACCESS_TOKEN", "must-not-escape")
	t.Setenv("TF_ACC", "1")
	t.Setenv("FEATBIT_DOCS_SAFE_TEST", "preserved")
	t.Setenv("PATH", "ambient-path")

	environment := Sanitized(map[string]string{
		"PATH":             "pinned-tool-path",
		"TF_IN_AUTOMATION": "1",
	})

	values := make(map[string]string, len(environment))
	for _, item := range environment {
		name, value, found := strings.Cut(item, "=")
		if found {
			values[strings.ToUpper(name)] = value
		}
	}

	tests := []struct {
		name    string
		key     string
		want    string
		present bool
	}{
		{name: "FeatBit credential removed", key: "FEATBIT_ACCESS_TOKEN"},
		{name: "acceptance opt-in removed", key: "TF_ACC"},
		{name: "unrelated setting preserved", key: "FEATBIT_DOCS_SAFE_TEST", want: "preserved", present: true},
		{name: "ambient setting replaced", key: "PATH", want: "pinned-tool-path", present: true},
		{name: "new setting added", key: "TF_IN_AUTOMATION", want: "1", present: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, present := values[test.key]
			if present != test.present {
				t.Fatalf("presence of %s = %t, want %t", test.key, present, test.present)
			}
			if got != test.want {
				t.Fatalf("value of %s = %q, want %q", test.key, got, test.want)
			}
		})
	}
}
