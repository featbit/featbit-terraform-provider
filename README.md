<div align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset=".github/logo-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset=".github/logo-light.svg">
    <img alt="FeatBit" src=".github/logo-light.svg" width="88">
  </picture>

  <h1>Terraform Provider for FeatBit</h1>
  <p>Manage FeatBit projects, environments, feature flags, and environment-specific segments as reviewable Terraform code.</p>
</div>

<div align="center">

[![License: MPL 2.0][license-shield]][license-url]
[![Terraform: >= 1.0.0][terraform-shield]][terraform-url]

</div>

<div align="center">
  <a href="GitOpsGettingStarted.md">Getting Started</a> &middot;
  <a href="docs/index.md">Registry Documentation</a> &middot;
  <a href="examples">Examples</a> &middot;
  <a href="SUPPORT.md">Support</a> &middot;
  <a href="SECURITY.md">Security</a>
</div>

## Why this provider?

If your team already uses FeatBit and wants its core configuration reviewed,
repeated, and dependency-ordered with the rest of its infrastructure, this
provider models Projects, Environments, Feature Flags, and environment-specific
Segments as one dependency-aware Terraform graph. Existing objects can be
brought under Terraform through stable Import identifiers and exact object
lookup. Managed drift is reconciled without silently adopting ambiguous
objects or overwriting UI-owned Feature Flag targeting and runtime settings.
The provider uses only the documented public FeatBit API and never selects the
first fuzzy search result.

## Getting started

Start with the
**[FeatBit Terraform GitOps Tutorial](GitOpsGettingStarted.md)**. It walks
through provider installation and authentication, then evolves one Terraform
root across Dev, Stage, and Prod with Projects, Environments, Feature Flags,
and environment-specific Segments. The workflow verifies empty second plans,
demonstrates promotion and rollback, and finishes with reviewed cleanup.

The provider supports Terraform 1.0 or later. The tutorial uses Terraform
`>= 1.5.0, < 2.0.0`.

## Authentication

Set `FEATBIT_ACCESS_TOKEN` outside Terraform configuration. A FeatBit service
token is recommended for CI/CD. Do not put token values in `.tf` files,
variable defaults, committed environment files, plans, state, or logs.
See the [security policy](SECURITY.md) for safe handling and incident steps.

FeatBit Cloud is the default API origin. To select another documented public
API root, set it directly in the provider block:

```hcl
provider "featbit" {
  api_url = "https://featbit.example.com/api/v1"
}
```

Alternatively, leave `api_url` unset and export the same value through
`FEATBIT_API_URL`. The value must be an absolute HTTP or HTTPS URL with a host,
an optional port, and either no path or `/api/v1`; the provider normalizes both
forms to `/api/v1`. User information, query strings, fragments, and other paths
are rejected.

## Documentation and examples

The [Registry provider guide](docs/index.md) documents authentication and
provider configuration. Each [resource](docs/resources) and
[data source](docs/data-sources) has a complete reference page and example;
every resource page includes its exact Import form.

For focused HCL examples, use the complete
[Feature Flag example](examples/resources/featbit_feature_flag) or
[Segment example](examples/resources/featbit_segment).

## Security and support

Use [SUPPORT.md](SUPPORT.md) to choose the correct provider, upstream, or
FeatBit service route and to prepare a synthetic bug report. Never include
credentials, state or plan content, logs, raw responses, runtime identifiers,
or vulnerability details in a public issue.

For a suspected vulnerability, follow [SECURITY.md](SECURITY.md) without
disclosing vulnerability details publicly.

## Upgrading

Pin the initial line with `~> 0.1.0` so Terraform cannot automatically select
a potentially breaking pre-1.0 minor release. [UPGRADING.md](UPGRADING.md)
defines the SemVer, schema, state, and Import compatibility contract plus a
safe plan-first upgrade and rollback workflow.

## License

This provider is licensed under the [Mozilla Public License 2.0](LICENSE).

[license-shield]: https://img.shields.io/badge/License-MPL--2.0-blue.svg
[license-url]: LICENSE
[terraform-shield]: https://img.shields.io/badge/Terraform-%3E%3D_1.0.0-844FBA?logo=terraform&logoColor=white
[terraform-url]: https://developer.hashicorp.com/terraform/plugin/terraform-plugin-protocol#protocol-version-6
