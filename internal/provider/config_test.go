// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestParseAPIURL(t *testing.T) {
	t.Parallel()

	valid := map[string]string{
		"cloud origin":            "https://app-api.featbit.co/api/v1",
		"origin trailing slash":   "https://self-hosted.example.test/api/v1",
		"documented API root":     "http://localhost:8080/api/v1",
		"API root trailing slash": "http://[::1]:8080/api/v1",
	}
	inputs := map[string]string{
		"cloud origin":            "https://app-api.featbit.co",
		"origin trailing slash":   "https://self-hosted.example.test/",
		"documented API root":     "http://localhost:8080/api/v1",
		"API root trailing slash": "http://[::1]:8080/api/v1/",
	}

	for name, input := range inputs {
		name := name
		input := input
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			parsed, err := parseAPIURL(input)
			if err != nil {
				t.Fatalf("parseAPIURL() error = %v", err)
			}
			if got := parsed.String(); got != valid[name] {
				t.Fatalf("parseAPIURL() = %q, want %q", got, valid[name])
			}
		})
	}

	invalid := map[string]string{
		"empty":              "",
		"surrounding space":  " https://app-api.featbit.co",
		"relative":           "/api/v1",
		"unsupported scheme": "ftp://app-api.featbit.co",
		"user information":   "https://user:password@app-api.featbit.co",
		"query":              "https://app-api.featbit.co?workspace=example",
		"empty query":        "https://app-api.featbit.co?",
		"fragment":           "https://app-api.featbit.co#example",
		"empty fragment":     "https://app-api.featbit.co#",
		"unexpected path":    "https://app-api.featbit.co/custom/api/v1",
		"encoded path":       "https://app-api.featbit.co/%61pi/v1",
		"zero port":          "https://app-api.featbit.co:0",
		"port out of range":  "https://app-api.featbit.co:65536",
	}

	for name, input := range invalid {
		name := name
		input := input
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := parseAPIURL(input); err == nil {
				t.Fatalf("parseAPIURL(%q) accepted an invalid URL", input)
			}
		})
	}
}

func TestNewProviderConfigAPIURLPrecedence(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		configured       types.String
		environmentValue string
		environmentSet   bool
		wantURL          string
		wantError        bool
	}{
		"explicit value wins": {
			configured:       types.StringValue("http://explicit.example.test:8080"),
			environmentValue: "not a URL",
			environmentSet:   true,
			wantURL:          "http://explicit.example.test:8080/api/v1",
		},
		"environment fallback": {
			configured:       types.StringNull(),
			environmentValue: "https://environment.example.test/api/v1/",
			environmentSet:   true,
			wantURL:          "https://environment.example.test/api/v1",
		},
		"cloud default": {
			configured: types.StringNull(),
			wantURL:    "https://app-api.featbit.co/api/v1",
		},
		"unknown value": {
			configured: types.StringUnknown(),
			wantError:  true,
		},
		"empty environment value": {
			configured:     types.StringNull(),
			environmentSet: true,
			wantError:      true,
		},
		"invalid explicit value": {
			configured: types.StringValue("https://explicit.example.test/not-api"),
			wantError:  true,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			lookup := func(key string) (string, bool) {
				if key != envAPIURL {
					t.Fatalf("unexpected environment lookup for %q", key)
				}
				return test.environmentValue, test.environmentSet
			}
			config, diagnostics := newProviderConfig(
				providerModel{APIURL: test.configured},
				lookup,
			)

			if diagnostics.HasError() != test.wantError {
				t.Fatalf("newProviderConfig() diagnostics = %v, wantError %t", diagnostics, test.wantError)
			}
			if test.wantError {
				if config != nil {
					t.Fatal("newProviderConfig() returned configuration with error diagnostics")
				}
				return
			}
			if config == nil || config.APIURL == nil {
				t.Fatal("newProviderConfig() returned no API URL")
			}
			if got := config.APIURL.String(); got != test.wantURL {
				t.Fatalf("newProviderConfig() API URL = %q, want %q", got, test.wantURL)
			}
		})
	}
}

func TestInvalidAPIURLDiagnosticDoesNotEchoValue(t *testing.T) {
	t.Parallel()

	const invalidURL = "https://tenant-name.example.test/private/path"
	_, diagnostics := newProviderConfig(
		providerModel{APIURL: types.StringValue(invalidURL)},
		func(string) (string, bool) { return "", false },
	)

	if !diagnostics.HasError() {
		t.Fatal("newProviderConfig() accepted an invalid API URL")
	}
	if strings.Contains(diagnostics.Errors()[0].Detail(), invalidURL) {
		t.Fatal("invalid API URL diagnostic echoed the configured value")
	}
}
