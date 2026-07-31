package probe

import (
	"strings"
	"testing"
)

const syntheticToken = "synthetic-test-token"

func TestValidateResourcePrefix(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		prefix  string
		wantErr bool
	}{
		"safe":             {prefix: "tfp0-20260730-a1"},
		"empty":            {prefix: "", wantErr: true},
		"missing marker":   {prefix: "provider-test-a1", wantErr: true},
		"uppercase":        {prefix: "tfp0-20260730-A1", wantErr: true},
		"too short":        {prefix: "tfp0-a", wantErr: true},
		"trailing hyphen":  {prefix: "tfp0-20260730-a1-", wantErr: true},
		"unsafe character": {prefix: "tfp0-20260730/a1", wantErr: true},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := ValidateResourcePrefix(test.prefix)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateResourcePrefix(%q) error = %v, wantErr %v", test.prefix, err, test.wantErr)
			}
		})
	}
}

func TestMutationTargetInterlock(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		url     string
		target  Target
		wantErr string
	}{
		"approved cloud": {
			url:    "https://app-api.featbit.co",
			target: TargetCloudCurrent,
		},
		"cloud lookalike": {
			url:     "https://app-api.featbit.co.example.test",
			target:  TargetCloudCurrent,
			wantErr: "approved FeatBit Cloud",
		},
		"cloud over HTTP": {
			url:     "http://app-api.featbit.co",
			target:  TargetCloudCurrent,
			wantErr: "approved FeatBit Cloud",
		},
		"private self-hosted": {
			url:    "http://192.168.10.20:8080/api/v1",
			target: TargetSelfHostedMin,
		},
		"loopback self-hosted": {
			url:    "http://localhost:8080",
			target: TargetSelfHostedMin,
		},
		"public self-hosted rejected": {
			url:     "https://selfhosted.example.test",
			target:  TargetSelfHostedMin,
			wantErr: "loopback or private",
		},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			parsed, err := parseAPIURL(test.url)
			if err != nil {
				t.Fatalf("parseAPIURL: %v", err)
			}
			cfg := Config{
				APIURL:         parsed,
				ServiceToken:   syntheticToken,
				Target:         test.target,
				ResourcePrefix: "tfp0-20260730-a1",
			}
			err = cfg.ValidateMutation()
			if test.wantErr == "" && err != nil {
				t.Fatalf("ValidateMutation() error = %v", err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("ValidateMutation() error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestMutationRequiresTokenAndPrefix(t *testing.T) {
	t.Parallel()

	parsed, err := parseAPIURL("https://app-api.featbit.co")
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{APIURL: parsed, Target: TargetCloudCurrent}
	if err := cfg.ValidateMutation(); err == nil {
		t.Fatal("ValidateMutation() accepted missing token and prefix")
	}

	cfg.ServiceToken = syntheticToken
	if err := cfg.ValidateMutation(); err == nil {
		t.Fatal("ValidateMutation() accepted missing prefix")
	}
}

func TestLoadConfigReportsPresenceWithoutValues(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		EnvAPIURL:       "https://app-api.featbit.co",
		EnvServiceToken: syntheticToken,
		EnvTarget:       string(TargetCloudCurrent),
	}
	cfg, presence, err := LoadConfig(func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	})
	if err != nil {
		t.Fatal(err)
	}
	if !presence.APIURL || !presence.ServiceToken || !presence.Target {
		t.Fatalf("unexpected presence: %+v", presence)
	}
	if presence.PersonalToken || presence.ResourcePrefix {
		t.Fatalf("unexpected optional presence: %+v", presence)
	}
	if cfg.ServiceToken != syntheticToken {
		t.Fatal("service token was not loaded")
	}
}
