<div align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset=".github/logo-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset=".github/logo-light.svg">
    <img alt="FeatBit" src=".github/logo-light.svg" width="88">
  </picture>

  <h1>Terraform Provider for FeatBit</h1>
  <p>Manage FeatBit core configuration and supported IAM relationships as reviewable Terraform code.</p>
</div>

<div align="center">

[![License: MPL 2.0][license-shield]][license-url]
[![Terraform: >= 1.0.0][terraform-shield]][terraform-url]

</div>

<div align="center">
  <a href="GitOpsGettingStarted.md">Getting Started</a> &middot;
  <a href="GitOpsGettingStartedIAM.md">IAM Tutorial</a> &middot;
  <a href="docs/index.md">Registry Documentation</a> &middot;
  <a href="examples">Examples</a> &middot;
  <a href="SUPPORT.md">Support</a> &middot;
  <a href="SECURITY.md">Security</a>
</div>

## Why this provider?

If your team already uses FeatBit and wants configuration and access-control
changes reviewed, repeated, and dependency-ordered with the rest of its
infrastructure, this provider models the supported core and IAM surface as one
Terraform graph. Existing objects can be selected exactly, and supported
managed objects can be brought under Terraform through stable Import
identifiers. Drift is reconciled without silently adopting ambiguous objects,
overwriting UI-owned Feature Flag runtime settings, or taking ownership of an
entire Group relationship collection. The provider uses only the documented
public FeatBit API and never selects the first fuzzy search result.

## Getting started

**[FeatBit Terraform GitOps Tutorial](GitOpsGettingStarted.md)**. It walks
through provider installation and authentication, then evolves one Terraform
root across Dev, Stage, and Prod with Projects, Environments, Feature Flags,
and environment-specific Segments. The workflow verifies empty second plans,
demonstrates promotion and rollback, and finishes with reviewed cleanup.

For IAM, follow the
**[FeatBit Terraform IAM GitOps Tutorial](GitOpsGettingStartedIAM.md)**. It
creates two custom Policies and one Group, exercises direct and inherited
access with two existing Members, updates one exact Prod Feature Flag
permission, and performs dependency-ordered cleanup. The focused
[custom Policy example](examples/resources/featbit_policy) and the other IAM
examples remain useful as individual reference configurations.

The provider supports Terraform 1.0 or later. The tutorial uses Terraform
`>= 1.5.0, < 2.0.0`.

## Authentication

Set `FEATBIT_ACCESS_TOKEN` outside Terraform configuration. An existing
FeatBit service token is recommended for CI/CD, but this provider does not
create Service Tokens or assign them to Groups. Do not put token values in
`.tf` files, variable defaults, committed environment files, plans, state, or
logs. See the [security policy](SECURITY.md) for safe handling and incident
steps.

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
[Feature Flag](examples/resources/featbit_feature_flag),
[Segment](examples/resources/featbit_segment),
[IAM Policy](examples/resources/featbit_policy),
[additive Member-Policy binding](examples/resources/featbit_member_policy_binding), or
[authoritative Member direct-Policy](examples/resources/featbit_member_direct_policies)
example. Use the binding when assigning one Policy without replacing a
Member's other direct Policies. The direct-Policy page explains its distinct
complete-set ownership before you apply it; do not overlap both models for one
Member.

## Security and support

Use [SUPPORT.md](SUPPORT.md) to choose the correct provider, upstream, or
FeatBit service route and to prepare a synthetic bug report. Never include
credentials, state or plan content, logs, raw responses, runtime identifiers,
or vulnerability details in a public issue.

For a suspected vulnerability, follow [SECURITY.md](SECURITY.md) without
disclosing vulnerability details publicly.

## Upgrading

Terraform requires an explicit prerelease selection; do not assume a broad
stable constraint selects a beta. Review the target release's notes before
changing the version constraint and dependency lock file. Existing core-only
`0.1.x` configurations remain covered by the compatibility contract.
[UPGRADING.md](UPGRADING.md) defines the SemVer, schema, state, and Import
contract plus a safe plan-first upgrade and rollback workflow.

## License

This provider is licensed under the [Mozilla Public License 2.0](LICENSE).

[license-shield]: https://img.shields.io/badge/License-MPL--2.0-blue.svg
[license-url]: LICENSE
[release-shield]: https://img.shields.io/github/v/release/featbit/terraform-provider-featbit
[release-url]: https://github.com/featbit/terraform-provider-featbit/releases/latest
[terraform-shield]: https://img.shields.io/badge/Terraform-%3E%3D_1.0.0-844FBA?logo=terraform&logoColor=white
[terraform-url]: https://developer.hashicorp.com/terraform/plugin/terraform-plugin-protocol#protocol-version-6
