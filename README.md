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
  <a href="#quick-start">Quick Start</a> &middot;
  <a href="docs/index.md">Registry Documentation</a> &middot;
  <a href="examples">Examples</a> &middot;
  <a href="SUPPORT.md">Support</a> &middot;
  <a href="SECURITY.md">Security</a>
</div>

## Why this provider?

If your team already uses FeatBit and wants its core configuration reviewed,
repeated, and dependency-ordered with the rest of its infrastructure, this
provider gives Terraform exact ownership of that configuration. It uses only
the documented public FeatBit API and resolves objects by exact identifiers,
never by the first fuzzy search result.

## Highlights

- Models Projects, Environments, Feature Flags, and environment-specific
  Segments as one dependency-aware Terraform graph.
- Imports existing objects with stable, documented identifiers and reads each
  data source by an exact UUID or scoped exact key.
- Repairs managed drift while preserving canonical server identities and
  refusing ambiguous mutation outcomes.
- Leaves Feature Flag targeting, rules, rollouts, enabled state, and tags under
  FeatBit UI ownership.
- Keeps access tokens out of configuration examples and accepts them safely
  from the process environment.

## Quick start

Copy [`examples/provider/provider.tf`](examples/provider/provider.tf), set
`FEATBIT_ACCESS_TOKEN` in the process environment through your normal secret
manager, then run:

```shell
terraform init
terraform plan
```

The first Registry release, `v0.1.0`, is not published yet. Registry
installation will become available after that release.

## Install

Declare the provider from the Terraform Registry:

```hcl
terraform {
  required_version = ">= 1.0.0"

  required_providers {
    featbit = {
      source  = "featbit/featbit"
      version = "~> 0.1.0"
    }
  }
}

provider "featbit" {}
```

Terraform 1.0 or later is required.

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

## Usage

References between managed objects give Terraform their creation and destroy
order:

```hcl
resource "featbit_project" "application" {
  name = "Application"
  key  = "application"
}

resource "featbit_environment" "staging" {
  project_id = featbit_project.application.id
  name       = "Staging"
  key        = "staging"
}
```

Continue with the complete [Feature Flag example](examples/resources/featbit_feature_flag)
or [Segment example](examples/resources/featbit_segment).

## Supported surface

| Managed resource | Exact data source | Terraform ownership |
|---|---|---|
| `featbit_project` | `data.featbit_project` | Project name and immutable key; child Environments are non-owning observations. |
| `featbit_environment` | `data.featbit_environment` | Name and description; parent Project and key define replacement identity. |
| `featbit_feature_flag` | `data.featbit_feature_flag` | Flag definition and variations; only the name updates in place. |
| `featbit_segment` | `data.featbit_segment` | Environment-specific Segment metadata, targeting, and tags; shared Segments are read-only. |

IAM, flag evaluation, deployments, analytics, audit streams, Portal-private
APIs, and a generic raw REST resource are outside the initial release.

## Ownership and deletion

Changing an identity-defining field produces replacement rather than silently
adopting another object. Feature Flag and Segment destroy operations archive,
permanently delete, and prove exact absence; Segment destroy first refuses to
continue while exact Feature Flag references remain. Project and Environment
deletion also require authoritative absence before state is removed.

Feature Flag targeting and operational state remain UI-owned. Terraform does
not toggle flags or rewrite targeting, rules, rollouts, or tags while managing
the definition. Shared Segments can be observed with
`data.featbit_segment`, but the resource cannot mutate them.

## Documentation and examples

The [Registry provider guide](docs/index.md) documents authentication and
provider configuration. Each [resource](docs/resources) and
[data source](docs/data-sources) has a complete reference page and example;
every resource page includes its exact Import form.

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
