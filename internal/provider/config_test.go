// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const syntheticProviderAccessToken = "test-only-not-a-credential"

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
		"user information":   "https://user:password@localhost",
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
			model := validProviderModel()
			model.APIURL = test.configured
			config, diagnostics := newProviderConfig(model, func(key string) (string, bool) {
				if key == envAPIURL {
					return lookup(key)
				}
				return "", false
			})

			if diagnostics.HasError() != test.wantError {
				t.Fatalf("newProviderConfig() diagnostics = %v, wantError %t", diagnostics, test.wantError)
			}
			if test.wantError {
				if config != nil {
					t.Fatal("newProviderConfig() returned configuration with error diagnostics")
				}
				return
			}
			if config == nil || config.apiURL == nil {
				t.Fatal("newProviderConfig() returned no API URL")
			}
			if got := config.apiURL.String(); got != test.wantURL {
				t.Fatalf("newProviderConfig() API URL = %q, want %q", got, test.wantURL)
			}
		})
	}
}

func TestNewProviderConfigAccessTokenPrecedence(t *testing.T) {
	t.Parallel()

	const environmentToken = "test-only-environment-token"
	tests := map[string]struct {
		configured       types.String
		environmentValue string
		environmentSet   bool
		wantToken        string
		wantError        bool
	}{
		"explicit value wins": {
			configured:       types.StringValue(syntheticProviderAccessToken),
			environmentValue: " invalid environment token ",
			environmentSet:   true,
			wantToken:        syntheticProviderAccessToken,
		},
		"environment fallback": {
			configured:       types.StringNull(),
			environmentValue: environmentToken,
			environmentSet:   true,
			wantToken:        environmentToken,
		},
		"missing value": {
			configured: types.StringNull(),
			wantError:  true,
		},
		"unknown value": {
			configured: types.StringUnknown(),
			wantError:  true,
		},
		"empty explicit value": {
			configured: types.StringValue(""),
			wantError:  true,
		},
		"surrounding whitespace": {
			configured: types.StringValue(" " + syntheticProviderAccessToken + " "),
			wantError:  true,
		},
		"control character": {
			configured: types.StringValue(syntheticProviderAccessToken + "\n"),
			wantError:  true,
		},
		"empty environment value": {
			configured:       types.StringNull(),
			environmentValue: "",
			environmentSet:   true,
			wantError:        true,
		},
		"invalid environment value": {
			configured:       types.StringNull(),
			environmentValue: " " + environmentToken + " ",
			environmentSet:   true,
			wantError:        true,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			model := validProviderModel()
			model.AccessToken = test.configured
			config, diagnostics := newProviderConfig(model, func(key string) (string, bool) {
				if key == envAccessToken {
					return test.environmentValue, test.environmentSet
				}
				return "", false
			})

			if diagnostics.HasError() != test.wantError {
				t.Fatalf("newProviderConfig() diagnostics = %v, wantError %t", diagnostics, test.wantError)
			}
			if test.wantError {
				if config != nil {
					t.Fatal("newProviderConfig() returned configuration with error diagnostics")
				}
				return
			}
			if config == nil {
				t.Fatal("newProviderConfig() returned nil configuration")
			}
			if config.accessToken != test.wantToken {
				t.Fatal("newProviderConfig() selected the wrong access token source")
			}
		})
	}
}

func TestNewProviderConfigIntegerSettings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setting   integerSetting
		setModel  func(*providerModel, types.Int64)
		getConfig func(*providerConfig) int64
	}{
		{
			name:    "HTTP timeout",
			setting: httpTimeoutSecondsSetting,
			setModel: func(model *providerModel, value types.Int64) {
				model.HTTPTimeoutSeconds = value
			},
			getConfig: func(config *providerConfig) int64 {
				return int64(config.httpTimeout / time.Second)
			},
		},
		{
			name:    "maximum concurrency",
			setting: maxConcurrencySetting,
			setModel: func(model *providerModel, value types.Int64) {
				model.MaxConcurrency = value
			},
			getConfig: func(config *providerConfig) int64 {
				return int64(config.maxConcurrency)
			},
		},
		{
			name:    "maximum retries",
			setting: maxRetriesSetting,
			setModel: func(model *providerModel, value types.Int64) {
				model.MaxRetries = value
			},
			getConfig: func(config *providerConfig) int64 {
				return int64(config.maxRetries)
			},
		},
	}

	for _, settingTest := range tests {
		settingTest := settingTest
		t.Run(settingTest.name, func(t *testing.T) {
			t.Parallel()

			cases := map[string]struct {
				configured       types.Int64
				environmentValue string
				environmentSet   bool
				want             int64
				wantError        bool
			}{
				"explicit value wins": {
					configured:       types.Int64Value(settingTest.setting.minimum),
					environmentValue: "not-an-integer",
					environmentSet:   true,
					want:             settingTest.setting.minimum,
				},
				"environment fallback": {
					configured:       types.Int64Null(),
					environmentValue: strconvFormatInt(settingTest.setting.maximum),
					environmentSet:   true,
					want:             settingTest.setting.maximum,
				},
				"default": {
					configured: types.Int64Null(),
					want:       settingTest.setting.defaultValue,
				},
				"unknown": {
					configured: types.Int64Unknown(),
					wantError:  true,
				},
				"below minimum": {
					configured: types.Int64Value(settingTest.setting.minimum - 1),
					wantError:  true,
				},
				"above maximum": {
					configured: types.Int64Value(settingTest.setting.maximum + 1),
					wantError:  true,
				},
				"invalid environment integer": {
					configured:       types.Int64Null(),
					environmentValue: "not-an-integer",
					environmentSet:   true,
					wantError:        true,
				},
				"environment whitespace": {
					configured:       types.Int64Null(),
					environmentValue: " 1 ",
					environmentSet:   true,
					wantError:        true,
				},
			}

			for name, test := range cases {
				name := name
				test := test
				t.Run(name, func(t *testing.T) {
					t.Parallel()

					model := validProviderModel()
					settingTest.setModel(&model, test.configured)
					config, diagnostics := newProviderConfig(model, func(key string) (string, bool) {
						if key == settingTest.setting.environment {
							return test.environmentValue, test.environmentSet
						}
						return "", false
					})

					if diagnostics.HasError() != test.wantError {
						t.Fatalf("newProviderConfig() diagnostics = %v, wantError %t", diagnostics, test.wantError)
					}
					if test.wantError {
						if config != nil {
							t.Fatal("newProviderConfig() returned configuration with error diagnostics")
						}
						return
					}
					if config == nil {
						t.Fatal("newProviderConfig() returned nil configuration")
					}
					if got := settingTest.getConfig(config); got != test.want {
						t.Fatalf("newProviderConfig() resolved value = %d, want %d", got, test.want)
					}
				})
			}
		})
	}
}

func TestInvalidAPIURLDiagnosticDoesNotEchoValue(t *testing.T) {
	t.Parallel()

	const invalidURL = "https://tenant-name.example.test/private/path"
	model := validProviderModel()
	model.APIURL = types.StringValue(invalidURL)
	_, diagnostics := newProviderConfig(
		model,
		func(string) (string, bool) { return "", false },
	)

	if !diagnostics.HasError() {
		t.Fatal("newProviderConfig() accepted an invalid API URL")
	}
	if diagnosticsContain(diagnostics, invalidURL) {
		t.Fatal("invalid API URL diagnostic echoed the configured value")
	}
}

func TestConfigurationDiagnosticsDoNotEchoCredentials(t *testing.T) {
	t.Parallel()

	const credentialMarker = "credential-marker-that-must-not-appear"
	tests := map[string]struct {
		configured types.String
		lookupEnv  func(string) (string, bool)
	}{
		"provider attribute": {
			configured: types.StringValue(" " + credentialMarker + " "),
			lookupEnv:  func(string) (string, bool) { return "", false },
		},
		"environment variable": {
			configured: types.StringNull(),
			lookupEnv: func(key string) (string, bool) {
				if key == envAccessToken {
					return " " + credentialMarker + " ", true
				}
				return "", false
			},
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			model := validProviderModel()
			model.AccessToken = test.configured
			_, diagnostics := newProviderConfig(model, test.lookupEnv)
			if !diagnostics.HasError() {
				t.Fatal("newProviderConfig() accepted an invalid access token")
			}
			if diagnosticsContain(diagnostics, credentialMarker) {
				t.Fatal("configuration diagnostic disclosed the access token")
			}
		})
	}
}

func validProviderModel() providerModel {
	return providerModel{
		APIURL:             types.StringNull(),
		AccessToken:        types.StringValue(syntheticProviderAccessToken),
		HTTPTimeoutSeconds: types.Int64Null(),
		MaxConcurrency:     types.Int64Null(),
		MaxRetries:         types.Int64Null(),
	}
}

func diagnosticsContain(diagnostics diag.Diagnostics, value string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Summary(), value) || strings.Contains(diagnostic.Detail(), value) {
			return true
		}
	}
	return false
}

func strconvFormatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

func TestProviderConfigurationDefaultsMatchClientDefaults(t *testing.T) {
	t.Parallel()

	config, diagnostics := newProviderConfig(
		validProviderModel(),
		func(string) (string, bool) { return "", false },
	)
	if diagnostics.HasError() {
		t.Fatalf("newProviderConfig() diagnostics = %v", diagnostics)
	}
	if config.httpTimeout != client.DefaultHTTPTimeout ||
		config.maxConcurrency != client.DefaultMaxConcurrency ||
		config.maxRetries != client.DefaultMaxRetries {
		t.Fatal("provider defaults do not match client defaults")
	}
}

func TestProviderConfigurationFormattingDoesNotDiscloseCredential(t *testing.T) {
	t.Parallel()

	config, diagnostics := newProviderConfig(
		validProviderModel(),
		func(string) (string, bool) { return "", false },
	)
	if diagnostics.HasError() {
		t.Fatalf("newProviderConfig() diagnostics = %v", diagnostics)
	}

	formatted := fmt.Sprintf("%v|%+v|%#v|%s", config, config, config, config)
	if strings.Contains(formatted, syntheticProviderAccessToken) {
		t.Fatal("formatted provider configuration disclosed the access token")
	}
}
