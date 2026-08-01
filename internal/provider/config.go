// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	defaultCloudAPIURL    = "https://app-api.featbit.co"
	envAPIURL             = "FEATBIT_API_URL"
	envAccessToken        = "FEATBIT_ACCESS_TOKEN"
	envHTTPTimeoutSeconds = "FEATBIT_HTTP_TIMEOUT_SECONDS"
	envMaxConcurrency     = "FEATBIT_MAX_CONCURRENCY"
	envMaxRetries         = "FEATBIT_MAX_RETRIES"
)

var (
	httpTimeoutSecondsSetting = integerSetting{
		attributeName: "http_timeout_seconds",
		environment:   envHTTPTimeoutSeconds,
		title:         "HTTP Timeout",
		defaultValue:  int64(client.DefaultHTTPTimeout / time.Second),
		minimum:       int64(client.MinHTTPTimeout / time.Second),
		maximum:       int64(client.MaxHTTPTimeout / time.Second),
	}
	maxConcurrencySetting = integerSetting{
		attributeName: "max_concurrency",
		environment:   envMaxConcurrency,
		title:         "Maximum Concurrency",
		defaultValue:  int64(client.DefaultMaxConcurrency),
		minimum:       int64(client.MinConcurrency),
		maximum:       int64(client.MaxConcurrency),
	}
	maxRetriesSetting = integerSetting{
		attributeName: "max_retries",
		environment:   envMaxRetries,
		title:         "Maximum Retries",
		defaultValue:  int64(client.DefaultMaxRetries),
		minimum:       int64(client.MinRetries),
		maximum:       int64(client.MaxRetries),
	}
)

// providerModel is the Terraform representation of the provider block.
type providerModel struct {
	APIURL             types.String `tfsdk:"api_url"`
	AccessToken        types.String `tfsdk:"access_token"`
	HTTPTimeoutSeconds types.Int64  `tfsdk:"http_timeout_seconds"`
	MaxConcurrency     types.Int64  `tfsdk:"max_concurrency"`
	MaxRetries         types.Int64  `tfsdk:"max_retries"`
}

// providerConfig is the validated runtime representation. Secret fields stay
// unexported so ordinary formatting outside this package cannot reveal them.
type providerConfig struct {
	apiURL         *url.URL
	accessToken    string
	httpTimeout    time.Duration
	maxConcurrency int
	maxRetries     int
}

// Format prevents accidental structured formatting from traversing into the
// resolved access token.
func (providerConfig) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.providerConfig{redacted}")
}

type apiURLValidator struct{}

var _ validator.String = apiURLValidator{}

func (apiURLValidator) Description(context.Context) string {
	return "must be an absolute HTTP or HTTPS URL without credentials, query parameters, or a fragment, and with an empty or /api/v1 path"
}

func (v apiURLValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (apiURLValidator) ValidateString(
	_ context.Context,
	req validator.StringRequest,
	resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if _, err := parseAPIURL(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid FeatBit API URL",
			fmt.Sprintf("The api_url value is invalid: %s.", err),
		)
	}
}

type accessTokenValidator struct{}

var _ validator.String = accessTokenValidator{}

func (accessTokenValidator) Description(context.Context) string {
	return "must be non-empty, contain no control characters, and have no surrounding whitespace"
}

func (v accessTokenValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (accessTokenValidator) ValidateString(
	_ context.Context,
	req validator.StringRequest,
	resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if err := validateAccessToken(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid FeatBit API Access Token",
			fmt.Sprintf("The access_token value is invalid: %s.", err),
		)
	}
}

type integerSetting struct {
	attributeName string
	environment   string
	title         string
	defaultValue  int64
	minimum       int64
	maximum       int64
}

type boundedInt64Validator struct {
	setting integerSetting
}

var _ validator.Int64 = boundedInt64Validator{}

func (v boundedInt64Validator) Description(context.Context) string {
	return fmt.Sprintf("must be an integer between %d and %d", v.setting.minimum, v.setting.maximum)
}

func (v boundedInt64Validator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v boundedInt64Validator) ValidateInt64(
	_ context.Context,
	req validator.Int64Request,
	resp *validator.Int64Response,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if !v.setting.valid(req.ConfigValue.ValueInt64()) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid FeatBit "+v.setting.title,
			fmt.Sprintf(
				"The %s value must be an integer between %d and %d.",
				v.setting.attributeName,
				v.setting.minimum,
				v.setting.maximum,
			),
		)
	}
}

func (s integerSetting) valid(value int64) bool {
	return value >= s.minimum && value <= s.maximum
}

func newProviderConfig(
	model providerModel,
	lookupEnv func(string) (string, bool),
) (*providerConfig, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	apiURL, apiURLDiagnostics := resolveAPIURL(model.APIURL, lookupEnv)
	diagnostics.Append(apiURLDiagnostics...)

	accessToken, accessTokenDiagnostics := resolveAccessToken(model.AccessToken, lookupEnv)
	diagnostics.Append(accessTokenDiagnostics...)

	httpTimeoutSeconds, httpTimeoutDiagnostics := resolveIntegerSetting(
		model.HTTPTimeoutSeconds,
		httpTimeoutSecondsSetting,
		lookupEnv,
	)
	diagnostics.Append(httpTimeoutDiagnostics...)

	maxConcurrency, maxConcurrencyDiagnostics := resolveIntegerSetting(
		model.MaxConcurrency,
		maxConcurrencySetting,
		lookupEnv,
	)
	diagnostics.Append(maxConcurrencyDiagnostics...)

	maxRetries, maxRetriesDiagnostics := resolveIntegerSetting(
		model.MaxRetries,
		maxRetriesSetting,
		lookupEnv,
	)
	diagnostics.Append(maxRetriesDiagnostics...)

	if diagnostics.HasError() {
		return nil, diagnostics
	}

	return &providerConfig{
		apiURL:         apiURL,
		accessToken:    accessToken,
		httpTimeout:    time.Duration(httpTimeoutSeconds) * time.Second,
		maxConcurrency: int(maxConcurrency),
		maxRetries:     int(maxRetries),
	}, diagnostics
}

func resolveAPIURL(
	configured types.String,
	lookupEnv func(string) (string, bool),
) (*url.URL, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	if configured.IsUnknown() {
		diagnostics.AddAttributeError(
			path.Root("api_url"),
			"Unknown FeatBit API URL",
			"The api_url value must be known when the provider is configured.",
		)
		return nil, diagnostics
	}

	rawURL := configured.ValueString()
	source := "The api_url value"
	if configured.IsNull() {
		rawURL = defaultCloudAPIURL
		source = "The FeatBit Cloud default API URL"
		if environmentURL, ok := lookupEnv(envAPIURL); ok {
			rawURL = environmentURL
			source = "The FEATBIT_API_URL environment variable"
		}
	}

	apiURL, err := parseAPIURL(rawURL)
	if err != nil {
		diagnostics.AddAttributeError(
			path.Root("api_url"),
			"Invalid FeatBit API URL",
			fmt.Sprintf("%s is invalid: %s.", source, err),
		)
		return nil, diagnostics
	}

	return apiURL, diagnostics
}

func resolveAccessToken(
	configured types.String,
	lookupEnv func(string) (string, bool),
) (string, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	if configured.IsUnknown() {
		diagnostics.AddAttributeError(
			path.Root("access_token"),
			"Unknown FeatBit API Access Token",
			"The access_token value must be known when the provider is configured.",
		)
		return "", diagnostics
	}

	accessToken := configured.ValueString()
	source := "The access_token value"
	if configured.IsNull() {
		var ok bool
		accessToken, ok = lookupEnv(envAccessToken)
		if !ok || accessToken == "" {
			diagnostics.AddAttributeError(
				path.Root("access_token"),
				"Missing FeatBit API Access Token",
				"Set the access_token provider attribute or the FEATBIT_ACCESS_TOKEN environment variable.",
			)
			return "", diagnostics
		}
		source = "The FEATBIT_ACCESS_TOKEN environment variable"
	}

	if err := validateAccessToken(accessToken); err != nil {
		diagnostics.AddAttributeError(
			path.Root("access_token"),
			"Invalid FeatBit API Access Token",
			fmt.Sprintf("%s is invalid: %s.", source, err),
		)
		return "", diagnostics
	}

	return accessToken, diagnostics
}

func resolveIntegerSetting(
	configured types.Int64,
	setting integerSetting,
	lookupEnv func(string) (string, bool),
) (int64, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	if configured.IsUnknown() {
		diagnostics.AddAttributeError(
			path.Root(setting.attributeName),
			"Unknown FeatBit "+setting.title,
			fmt.Sprintf(
				"The %s value must be known when the provider is configured.",
				setting.attributeName,
			),
		)
		return 0, diagnostics
	}

	value := configured.ValueInt64()
	source := "The " + setting.attributeName + " value"
	if configured.IsNull() {
		value = setting.defaultValue
		source = "The default " + setting.attributeName + " value"
		if environmentValue, ok := lookupEnv(setting.environment); ok {
			parsed, err := parseEnvironmentInteger(environmentValue)
			if err != nil {
				diagnostics.AddAttributeError(
					path.Root(setting.attributeName),
					"Invalid FeatBit "+setting.title,
					fmt.Sprintf(
						"The %s environment variable must be an integer between %d and %d.",
						setting.environment,
						setting.minimum,
						setting.maximum,
					),
				)
				return 0, diagnostics
			}
			value = parsed
			source = "The " + setting.environment + " environment variable"
		}
	}

	if !setting.valid(value) {
		diagnostics.AddAttributeError(
			path.Root(setting.attributeName),
			"Invalid FeatBit "+setting.title,
			fmt.Sprintf(
				"%s must be an integer between %d and %d.",
				source,
				setting.minimum,
				setting.maximum,
			),
		)
		return 0, diagnostics
	}

	return value, diagnostics
}

func parseEnvironmentInteger(value string) (int64, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return 0, errors.New("environment integer is empty or has surrounding whitespace")
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, errors.New("environment integer is invalid")
	}
	return parsed, nil
}

// validateAccessToken deliberately validates only header safety and obvious
// configuration mistakes. FeatBit token formats and permissions are server
// concerns, and rejected values are never included in returned errors.
func validateAccessToken(accessToken string) error {
	if accessToken == "" {
		return errors.New("it must be non-empty")
	}
	if strings.TrimSpace(accessToken) != accessToken {
		return errors.New("it must not have surrounding whitespace")
	}
	if strings.IndexFunc(accessToken, unicode.IsControl) != -1 {
		return errors.New("it must not contain control characters")
	}
	return nil
}

// parseAPIURL accepts either an origin or the documented API root and returns
// one canonical URL ending in /api/v1. It never includes the rejected input
// in an error so configuration diagnostics cannot disclose URL contents.
func parseAPIURL(rawURL string) (*url.URL, error) {
	if rawURL == "" || strings.TrimSpace(rawURL) != rawURL {
		return nil, errors.New("it must be non-empty and have no surrounding whitespace")
	}

	apiURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, errors.New("it must be a valid absolute URL")
	}

	if apiURL.Opaque != "" || !apiURL.IsAbs() || apiURL.Host == "" || apiURL.Hostname() == "" {
		return nil, errors.New("it must be an absolute URL with a host")
	}
	if !strings.EqualFold(apiURL.Scheme, "http") && !strings.EqualFold(apiURL.Scheme, "https") {
		return nil, errors.New("its scheme must be HTTP or HTTPS")
	}
	if apiURL.User != nil {
		return nil, errors.New("it must not contain user information")
	}
	if apiURL.RawQuery != "" || apiURL.ForceQuery {
		return nil, errors.New("it must not contain query parameters")
	}
	if apiURL.Fragment != "" || strings.Contains(rawURL, "#") {
		return nil, errors.New("it must not contain a fragment")
	}

	if port := apiURL.Port(); port != "" {
		portNumber, conversionErr := strconv.ParseUint(port, 10, 16)
		if conversionErr != nil || portNumber == 0 {
			return nil, errors.New("its port must be between 1 and 65535")
		}
	}

	switch strings.TrimSuffix(apiURL.EscapedPath(), "/") {
	case "", "/api/v1":
		apiURL.Path = "/api/v1"
		apiURL.RawPath = ""
	default:
		return nil, errors.New("its path must be empty or /api/v1")
	}

	apiURL.Scheme = strings.ToLower(apiURL.Scheme)
	return apiURL, nil
}
