// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"io"
	"math/big"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tfsdklog"
)

type capturedClientConfiguration struct {
	calls       int
	baseURL     string
	accessToken string
	options     client.Options
}

type protocolUnknownValue struct{}

func TestProtocol6ProviderConfigurationExplicitValues(t *testing.T) {
	t.Parallel()

	var captured capturedClientConfiguration
	providerUnderTest := &FeatBitProvider{
		version: "test",
		lookupEnv: func(string) (string, bool) {
			t.Fatal("explicit provider values unexpectedly used an environment fallback")
			return "", false
		},
		newClient: capturingClientFactory(t, &captured),
	}
	server := providerserver.NewProtocol6(providerUnderTest)()

	config := protocolProviderConfig(t, server, map[string]any{
		"api_url":              "http://explicit.example.test:8080/",
		"access_token":         syntheticProviderAccessToken,
		"http_timeout_seconds": int64(45),
		"max_concurrency":      int64(7),
		"max_retries":          int64(0),
	})
	assertValidProtocolConfiguration(t, server, config)

	if captured.calls != 1 {
		t.Fatalf("client factory calls = %d, want 1", captured.calls)
	}
	if captured.baseURL != "http://explicit.example.test:8080/api/v1" {
		t.Fatalf("configured API URL = %q, want canonical explicit URL", captured.baseURL)
	}
	if captured.accessToken != syntheticProviderAccessToken {
		t.Fatal("configured client received the wrong access token")
	}
	if captured.options.HTTPTimeout != 45*time.Second ||
		captured.options.MaxConcurrency != 7 ||
		captured.options.MaxRetries != 0 {
		t.Fatalf("configured client options = %#v, want explicit values", captured.options)
	}
}

func TestProtocol6ProviderConfigurationEnvironmentFallbacks(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		envAPIURL:             "https://environment.example.test/api/v1/",
		envAccessToken:        "test-only-environment-token",
		envHTTPTimeoutSeconds: "75",
		envMaxConcurrency:     "6",
		envMaxRetries:         "2",
	}
	var captured capturedClientConfiguration
	providerUnderTest := &FeatBitProvider{
		version: "test",
		lookupEnv: func(key string) (string, bool) {
			value, ok := values[key]
			return value, ok
		},
		newClient: capturingClientFactory(t, &captured),
	}
	server := providerserver.NewProtocol6(providerUnderTest)()

	config := protocolProviderConfig(t, server, nil)
	assertValidProtocolConfiguration(t, server, config)

	if captured.calls != 1 {
		t.Fatalf("client factory calls = %d, want 1", captured.calls)
	}
	if captured.baseURL != "https://environment.example.test/api/v1" {
		t.Fatalf("configured API URL = %q, want canonical environment URL", captured.baseURL)
	}
	if captured.accessToken != values[envAccessToken] {
		t.Fatal("configured client did not receive the environment access token")
	}
	if captured.options.HTTPTimeout != 75*time.Second ||
		captured.options.MaxConcurrency != 6 ||
		captured.options.MaxRetries != 2 {
		t.Fatalf("configured client options = %#v, want environment values", captured.options)
	}
}

func TestProtocol6ProviderConfigurationDefaults(t *testing.T) {
	t.Parallel()

	var captured capturedClientConfiguration
	providerUnderTest := &FeatBitProvider{
		version: "test",
		lookupEnv: func(key string) (string, bool) {
			if key == envAccessToken {
				return syntheticProviderAccessToken, true
			}
			return "", false
		},
		newClient: capturingClientFactory(t, &captured),
	}
	server := providerserver.NewProtocol6(providerUnderTest)()

	config := protocolProviderConfig(t, server, nil)
	assertValidProtocolConfiguration(t, server, config)

	if captured.calls != 1 {
		t.Fatalf("client factory calls = %d, want 1", captured.calls)
	}
	if captured.baseURL != "https://app-api.featbit.co/api/v1" {
		t.Fatalf("configured API URL = %q, want Cloud default", captured.baseURL)
	}
	if captured.options.HTTPTimeout != client.DefaultHTTPTimeout ||
		captured.options.MaxConcurrency != client.DefaultMaxConcurrency ||
		captured.options.MaxRetries != client.DefaultMaxRetries {
		t.Fatalf("configured client options = %#v, want defaults", captured.options)
	}
}

func TestProtocol6ProviderConfigurationRejectsUnknownAndInvalidValues(t *testing.T) {
	t.Parallel()

	tests := map[string]map[string]any{
		"missing access token": {},
		"unknown access token": {
			"access_token": protocolUnknownValue{},
		},
		"invalid access token": {
			"access_token": " " + syntheticProviderAccessToken + " ",
		},
		"unknown timeout": {
			"access_token":         syntheticProviderAccessToken,
			"http_timeout_seconds": protocolUnknownValue{},
		},
		"invalid timeout": {
			"access_token":         syntheticProviderAccessToken,
			"http_timeout_seconds": int64(301),
		},
		"invalid concurrency": {
			"access_token":    syntheticProviderAccessToken,
			"max_concurrency": int64(0),
		},
		"invalid retries": {
			"access_token": syntheticProviderAccessToken,
			"max_retries":  int64(-1),
		},
		"invalid URL": {
			"api_url":      "https://tenant.example.test/private",
			"access_token": syntheticProviderAccessToken,
		},
	}

	for name, values := range tests {
		name := name
		values := values
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			factoryCalls := 0
			providerUnderTest := &FeatBitProvider{
				version:   "test",
				lookupEnv: func(string) (string, bool) { return "", false },
				newClient: func(*url.URL, string, client.Options) (*client.Client, error) {
					factoryCalls++
					return nil, errors.New("unexpected client construction")
				},
			}
			server := providerserver.NewProtocol6(providerUnderTest)()
			config := protocolProviderConfig(t, server, values)

			validationResponse, err := server.ValidateProviderConfig(
				context.Background(),
				&tfprotov6.ValidateProviderConfigRequest{Config: &config},
			)
			if err != nil {
				t.Fatalf("ValidateProviderConfig() error = %v", err)
			}

			configurationResponse, err := server.ConfigureProvider(
				context.Background(),
				&tfprotov6.ConfigureProviderRequest{Config: &config, TerraformVersion: "test"},
			)
			if err != nil {
				t.Fatalf("ConfigureProvider() error = %v", err)
			}
			if !protocolHasError(validationResponse.Diagnostics) &&
				!protocolHasError(configurationResponse.Diagnostics) {
				t.Fatal("invalid provider configuration produced no error diagnostics")
			}
			if factoryCalls != 0 {
				t.Fatalf("invalid configuration called the client factory %d times, want 0", factoryCalls)
			}
			if protocolDiagnosticsContain(validationResponse.Diagnostics, syntheticProviderAccessToken) ||
				protocolDiagnosticsContain(configurationResponse.Diagnostics, syntheticProviderAccessToken) {
				t.Fatal("invalid configuration diagnostic disclosed the access token")
			}
		})
	}
}

func TestProtocol6ProviderSchemaMarksAccessTokenSensitive(t *testing.T) {
	t.Parallel()

	server := providerserver.NewProtocol6(New("test")())()
	response, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("GetProviderSchema() error = %v", err)
	}
	if protocolHasError(response.Diagnostics) {
		t.Fatalf("GetProviderSchema() diagnostics = %v", response.Diagnostics)
	}

	for _, attribute := range response.Provider.Block.Attributes {
		if attribute.Name == "access_token" {
			if !attribute.Optional || !attribute.Sensitive {
				t.Fatal("Protocol v6 access_token is not optional and Sensitive")
			}
			return
		}
	}
	t.Fatal("Protocol v6 provider schema has no access_token attribute")
}

func TestProviderClientFailureDiagnosticDoesNotDiscloseCredential(t *testing.T) {
	t.Parallel()

	const credentialMarker = "credential-marker-that-must-not-appear"
	providerUnderTest := &FeatBitProvider{
		version:   "test",
		lookupEnv: func(string) (string, bool) { return "", false },
		newClient: func(_ *url.URL, accessToken string, _ client.Options) (*client.Client, error) {
			return nil, errors.New("unsafe dependency error containing " + accessToken)
		},
	}
	server := providerserver.NewProtocol6(providerUnderTest)()
	config := protocolProviderConfig(t, server, map[string]any{
		"access_token": credentialMarker,
	})

	response, err := server.ConfigureProvider(
		context.Background(),
		&tfprotov6.ConfigureProviderRequest{Config: &config, TerraformVersion: "test"},
	)
	if err != nil {
		t.Fatalf("ConfigureProvider() error = %v", err)
	}
	if !protocolHasError(response.Diagnostics) {
		t.Fatal("client factory failure produced no diagnostic")
	}
	if protocolDiagnosticsContain(response.Diagnostics, credentialMarker) {
		t.Fatal("client factory failure diagnostic disclosed the access token")
	}
}

func TestProviderConfigurationLogsDoNotDiscloseCredential(t *testing.T) {
	const credentialMarker = "credential-marker-that-must-not-appear"
	t.Setenv("TF_LOG", "TRACE")
	t.Setenv("TF_LOG_PATH", "")
	t.Setenv("TF_ACC_LOG_PATH", "")
	t.Setenv("TF_LOG_PATH_MASK", "")

	logReader, logWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	originalStderr := os.Stderr
	os.Stderr = logWriter
	defer func() {
		os.Stderr = originalStderr
		_ = logWriter.Close()
		_ = logReader.Close()
	}()

	providerUnderTest := &FeatBitProvider{
		version:   "test",
		lookupEnv: func(string) (string, bool) { return "", false },
		newClient: client.New,
	}
	server := providerserver.NewProtocol6(providerUnderTest)()
	config := protocolProviderConfig(t, server, map[string]any{
		"access_token": credentialMarker,
	})

	ctx := tfsdklog.ContextWithTestLogging(context.Background(), t.Name())
	ctx = tfsdklog.NewRootSDKLogger(ctx)
	ctx = tfsdklog.NewRootProviderLogger(ctx)
	os.Stderr = originalStderr

	response, err := server.ConfigureProvider(
		ctx,
		&tfprotov6.ConfigureProviderRequest{Config: &config, TerraformVersion: "test"},
	)
	if err != nil {
		t.Fatalf("ConfigureProvider() error = %v", err)
	}
	if protocolHasError(response.Diagnostics) {
		t.Fatalf("ConfigureProvider() diagnostics = %v", response.Diagnostics)
	}

	if err := logWriter.Close(); err != nil {
		t.Fatalf("log writer close error = %v", err)
	}
	logBytes, err := io.ReadAll(logReader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	logOutput := string(logBytes)
	if !strings.Contains(logOutput, "Configuring FeatBit provider") {
		t.Fatal("test did not capture the provider configuration log")
	}
	if strings.Contains(logOutput, credentialMarker) {
		t.Fatal("provider configuration log disclosed the access token")
	}
}

func capturingClientFactory(
	t *testing.T,
	captured *capturedClientConfiguration,
) clientFactory {
	t.Helper()

	return func(baseURL *url.URL, accessToken string, options client.Options) (*client.Client, error) {
		captured.calls++
		captured.baseURL = baseURL.String()
		captured.accessToken = accessToken
		captured.options = options
		return client.New(baseURL, accessToken, options)
	}
}

func assertValidProtocolConfiguration(
	t *testing.T,
	server tfprotov6.ProviderServer,
	config tfprotov6.DynamicValue,
) {
	t.Helper()

	validationResponse, err := server.ValidateProviderConfig(
		context.Background(),
		&tfprotov6.ValidateProviderConfigRequest{Config: &config},
	)
	if err != nil {
		t.Fatalf("ValidateProviderConfig() error = %v", err)
	}
	if protocolHasError(validationResponse.Diagnostics) {
		t.Fatalf("ValidateProviderConfig() diagnostics = %v", validationResponse.Diagnostics)
	}

	configurationResponse, err := server.ConfigureProvider(
		context.Background(),
		&tfprotov6.ConfigureProviderRequest{Config: &config, TerraformVersion: "test"},
	)
	if err != nil {
		t.Fatalf("ConfigureProvider() error = %v", err)
	}
	if protocolHasError(configurationResponse.Diagnostics) {
		t.Fatalf("ConfigureProvider() diagnostics = %v", configurationResponse.Diagnostics)
	}
}

func protocolProviderConfig(
	t *testing.T,
	server tfprotov6.ProviderServer,
	configured map[string]any,
) tfprotov6.DynamicValue {
	t.Helper()

	schemaResponse, err := server.GetProviderSchema(
		context.Background(),
		&tfprotov6.GetProviderSchemaRequest{},
	)
	if err != nil {
		t.Fatalf("GetProviderSchema() error = %v", err)
	}
	if protocolHasError(schemaResponse.Diagnostics) {
		t.Fatalf("GetProviderSchema() diagnostics = %v", schemaResponse.Diagnostics)
	}

	valueType, ok := schemaResponse.Provider.ValueType().(tftypes.Object)
	if !ok {
		t.Fatalf("provider schema value type = %T, want tftypes.Object", schemaResponse.Provider.ValueType())
	}
	values := make(map[string]tftypes.Value, len(valueType.AttributeTypes))
	for name, attributeType := range valueType.AttributeTypes {
		configuredValue, exists := configured[name]
		if !exists || configuredValue == nil {
			values[name] = tftypes.NewValue(attributeType, nil)
			continue
		}

		switch value := configuredValue.(type) {
		case protocolUnknownValue:
			values[name] = tftypes.NewValue(attributeType, tftypes.UnknownValue)
		case string:
			values[name] = tftypes.NewValue(attributeType, value)
		case int64:
			values[name] = tftypes.NewValue(attributeType, new(big.Float).SetInt64(value))
		default:
			t.Fatalf("unsupported protocol test value type %T for %s", configuredValue, name)
		}
	}

	terraformValue := tftypes.NewValue(valueType, values)
	dynamicValue, err := tfprotov6.NewDynamicValue(valueType, terraformValue)
	if err != nil {
		t.Fatalf("tfprotov6.NewDynamicValue() error = %v", err)
	}
	return dynamicValue
}

func protocolHasError(diagnostics []*tfprotov6.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic != nil && diagnostic.Severity == tfprotov6.DiagnosticSeverityError {
			return true
		}
	}
	return false
}

func protocolDiagnosticsContain(diagnostics []*tfprotov6.Diagnostic, value string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic != nil &&
			(strings.Contains(diagnostic.Summary, value) || strings.Contains(diagnostic.Detail, value)) {
			return true
		}
	}
	return false
}

var _ frameworkprovider.Provider = (*FeatBitProvider)(nil)
