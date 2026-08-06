# Support

## Choose the right route

| Need | Route |
|---|---|
| Suspected provider vulnerability or credential disclosure caused by the provider | Follow [SECURITY.md](SECURITY.md). Do not include details in a public issue. |
| Provider bug, documentation problem, feature request, or usage question | Open a [provider issue](https://github.com/featbit/featbit-terraform-provider/issues/new?template=bug_report.yml) with a synthetic, credential-free reproduction. |
| FeatBit account, token issuance, billing, hosted-service availability, or server administration | Use the verified support route associated with your FeatBit account or deployment. Do not post account data here. |
| Behavior reproducible without this provider in Terraform CLI or another upstream component | Report it to that upstream project after removing sensitive data. |

The configurable `api_url` does not certify an unspecified self-hosted FeatBit
release. When a provider issue occurs against self-hosted FeatBit, name the
exact FeatBit version and deployment type without exposing its hostname or
tenant details.

## Before opening a provider issue

Check the [README](README.md), [Registry documentation](docs/index.md), existing
issues, and the [upgrade policy](UPGRADING.md). Then prepare:

- Terraform CLI and FeatBit provider versions;
- operating system and architecture;
- FeatBit Cloud or the exact named self-hosted version;
- minimal HCL using synthetic names and `example.com`;
- deterministic reproduction steps; and
- expected behavior plus a concise, manually sanitized error description.

Do not attach or paste access tokens, authorization headers, state or plan
content, backend snapshots, debug or crash logs, raw API responses, internal
URLs or paths, organization/workspace data, real object IDs or keys, or user
identifiers. Do not enable debug logging merely to file an issue. If the report
might describe a security weakness, stop and follow [SECURITY.md](SECURITY.md).

Provider issues are handled as maintainer capacity permits; no response or
resolution SLA is promised.
