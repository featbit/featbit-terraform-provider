// Copyright IBM Corp. 2021, 2025
// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/url"
	"os"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ provider.Provider = (*FeatBitProvider)(nil)

// FeatBitProvider is the Terraform provider implementation.
type FeatBitProvider struct {
	version   string
	lookupEnv func(string) (string, bool)
	newClient clientFactory
}

type clientFactory func(*url.URL, string, client.Options) (*client.Client, error)

// Metadata returns the provider type name and build version.
func (p *FeatBitProvider) Metadata(
	_ context.Context,
	_ provider.MetadataRequest,
	resp *provider.MetadataResponse,
) {
	resp.TypeName = "featbit"
	resp.Version = p.version
}

// Schema returns the provider configuration schema.
func (p *FeatBitProvider) Schema(
	_ context.Context,
	_ provider.SchemaRequest,
	resp *provider.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Terraform provider for managing FeatBit through its documented public REST API.",
		Attributes: map[string]schema.Attribute{
			"api_url": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "FeatBit API base URL. May also be set with `FEATBIT_API_URL`. " +
					"Defaults to `https://app-api.featbit.co`. The path may be empty or `/api/v1`.",
				Validators: []validator.String{
					apiURLValidator{},
				},
			},
			"access_token": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				MarkdownDescription: "FeatBit personal or service API access token. May also be set with " +
					"`FEATBIT_ACCESS_TOKEN`; service tokens are recommended for CI/CD.",
				Validators: []validator.String{
					accessTokenValidator{},
				},
			},
			"http_timeout_seconds": schema.Int64Attribute{
				Optional: true,
				MarkdownDescription: "Timeout in seconds for one FeatBit HTTP request. May also be set with " +
					"`FEATBIT_HTTP_TIMEOUT_SECONDS`. Defaults to 30; valid range is 1 through 300.",
				Validators: []validator.Int64{
					boundedInt64Validator{setting: httpTimeoutSecondsSetting},
				},
			},
			"max_concurrency": schema.Int64Attribute{
				Optional: true,
				MarkdownDescription: "Maximum number of concurrent FeatBit API requests. May also be set with " +
					"`FEATBIT_MAX_CONCURRENCY`. Defaults to 4; valid range is 1 through 32.",
				Validators: []validator.Int64{
					boundedInt64Validator{setting: maxConcurrencySetting},
				},
			},
			"max_retries": schema.Int64Attribute{
				Optional: true,
				MarkdownDescription: "Maximum retry count for safely retryable reads. Mutations are never retried " +
					"automatically. May also be set with `FEATBIT_MAX_RETRIES`. Defaults to 3; valid range is 0 through 10.",
				Validators: []validator.Int64{
					boundedInt64Validator{setting: maxRetriesSetting},
				},
			},
		},
	}
}

// Configure resolves and validates provider settings and constructs the
// handwritten API client shared by resources and data sources.
func (p *FeatBitProvider) Configure(
	ctx context.Context,
	req provider.ConfigureRequest,
	resp *provider.ConfigureResponse,
) {
	tflog.Debug(ctx, "Configuring FeatBit provider")

	var model providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	lookupEnv := p.lookupEnv
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}

	config, diagnostics := newProviderConfig(model, lookupEnv)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}

	newClient := p.newClient
	if newClient == nil {
		newClient = client.New
	}

	apiClient, err := newClient(
		config.apiURL,
		config.accessToken,
		client.Options{
			HTTPTimeout:     config.httpTimeout,
			MaxConcurrency:  config.maxConcurrency,
			MaxRetries:      config.maxRetries,
			ProviderVersion: p.version,
		},
	)
	if err != nil {
		// Do not include the client error: a future transport implementation
		// could wrap request details. Configuration diagnostics stay credential
		// safe even when a dependency returns an unsafe error string.
		resp.Diagnostics.AddError(
			"Unable to Configure FeatBit API Client",
			"The provider could not create the FeatBit API client. Verify the provider settings and try again.",
		)
		return
	}

	resp.DataSourceData = apiClient
	resp.ResourceData = apiClient
	tflog.Debug(ctx, "Configured FeatBit provider")
}

// Resources registers the managed Project, Environment, and Feature Flag
// resources.
func (p *FeatBitProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newProjectResource,
		newEnvironmentResource,
		newFeatureFlagResource,
	}
}

// DataSources registers the exact single-object Project, Environment, and
// Feature Flag data sources.
func (p *FeatBitProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newProjectDataSource,
		newEnvironmentDataSource,
		newFeatureFlagDataSource,
	}
}

// New creates a provider factory with the supplied build version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &FeatBitProvider{
			version:   version,
			lookupEnv: os.LookupEnv,
			newClient: client.New,
		}
	}
}
