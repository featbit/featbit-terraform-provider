// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	defaultCloudAPIURL = "https://app-api.featbit.co"
	envAPIURL          = "FEATBIT_API_URL"
)

// providerModel is the Terraform representation of the provider block.
// P1-021 and P1-022 add the remaining provider settings to this model.
type providerModel struct {
	APIURL types.String `tfsdk:"api_url"`
}

// providerConfig is the validated runtime representation. P1-023 uses this
// input to construct the handwritten FeatBit API client.
type providerConfig struct {
	APIURL *url.URL
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

func newProviderConfig(
	model providerModel,
	lookupEnv func(string) (string, bool),
) (*providerConfig, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	if model.APIURL.IsUnknown() {
		diagnostics.AddAttributeError(
			path.Root("api_url"),
			"Unknown FeatBit API URL",
			"The api_url value must be known when the provider is configured.",
		)
		return nil, diagnostics
	}

	rawURL := model.APIURL.ValueString()
	source := "The api_url value"
	if model.APIURL.IsNull() {
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

	return &providerConfig{APIURL: apiURL}, diagnostics
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
