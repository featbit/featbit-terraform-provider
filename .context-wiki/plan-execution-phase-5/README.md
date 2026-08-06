# Phase 5 — Initial release

- Status: **In progress**
- Updated: **2026-08-06**
- Next task: `P5-012`

This package is the active execution context for the first public release of
the FeatBit Terraform Provider. The release contains the four completed core
resource families only. IAM begins after this release and must not add schema,
configuration, transport headers, API calls, documentation, or compatibility
claims during Phase 5.

## Starting point

Phases 1 through 4 passed their local, Protocol v6, and scoped current-Cloud
gates. The repository currently contains:

- a Protocol v6 provider at `registry.terraform.io/featbit/featbit`;
- five provider configuration attributes;
- Project, Environment, Feature Flag, and Segment resources;
- one exact single-object data source for each resource family;
- stable Import contracts for all four managed resources;
- a handwritten public REST client with exact lookup, cancellation, bounded
  concurrency, read-only retry, one-shot mutation, reconciliation, and
  redaction contracts;
- `main.go` build-time version injection and a Protocol 6
  `terraform-registry-manifest.json`; and
- trusted current-Cloud acceptance coverage with exact cleanup for every
  managed core resource.

The repository now contains a public README, schema-derived Registry
documentation, credential-free examples, and non-writing documentation drift
and example validation. It does not yet contain GitHub Actions, GoReleaser
configuration, a release signing workflow, complete security/contribution
guidance, or a clean-directory Registry-install smoke test. Phase 5 adds those
remaining release surfaces without changing the implemented product scope.

## Objective

Publish a signed, reproducible, documented initial provider release whose
public contract consists of exactly:

- `featbit_project`;
- `featbit_environment`;
- `featbit_feature_flag`;
- `featbit_segment`; and
- `data.featbit_project`, `data.featbit_environment`,
  `data.featbit_feature_flag`, and `data.featbit_segment`.

The Segment resource manages environment-specific segments only. Its data
source may also observe an exact shared segment through the already-proven
read-only contract.

The release must be installable from a clean Terraform directory, report its
real build version, expose only the frozen Protocol v6 schema, and complete
plan/apply/refresh/import/destroy lifecycles without permanent diffs or leaked
credentials. Publication is not complete until the Registry-served artifact,
rather than a development override, passes the final smoke test.

## Frozen product boundary

The initial release preserves these provider attributes:

| Attribute | Environment fallback | Contract |
|---|---|---|
| `api_url` | `FEATBIT_API_URL` | Optional; defaults to current FeatBit Cloud and supports an empty path or `/api/v1`. |
| `access_token` | `FEATBIT_ACCESS_TOKEN` | Optional in configuration, required after environment resolution, and Sensitive. |
| `http_timeout_seconds` | `FEATBIT_HTTP_TIMEOUT_SECONDS` | Optional; default `30`, range `1..300`. |
| `max_concurrency` | `FEATBIT_MAX_CONCURRENCY` | Optional; default `4`, range `1..32`. |
| `max_retries` | `FEATBIT_MAX_RETRIES` | Optional; default `3`, range `0..10`; mutations are never retried. |

Phase 5 must not:

- register an IAM resource or data source;
- add an organization, workspace, member, group, policy, team, or context-header
  provider attribute;
- forward caller-supplied organization/workspace headers;
- call IAM, Portal-private, deployment, analytics, or audit endpoints;
- change an existing resource identity, ownership boundary, Import ID, default,
  replacement rule, or canonical state shape merely for release convenience;
  or
- claim self-hosted compatibility that has not been exercised against an exact
  named version and configuration.

Release-only fixes are allowed when a focused failing contract proves that the
documented core behavior, packaging, generated documentation, or installation
path is incorrect. Any public schema change requires an explicit compatibility
decision under the active TODO item.

## Frozen release baseline

The `P5-010` baseline was updated on 2026-08-06. Its executable authority is
[`release_contract_test.go`](../../release_contract_test.go) together with the
reviewed Protocol snapshot at
[`internal/provider/testdata/release-schema.json`](../../internal/provider/testdata/release-schema.json).
The test obtains the schema through the production Protocol v6 server, not a
parallel hand-written resource model.

### Version and runtime contract

| Area | Frozen contract |
|---|---|
| First Registry artifact | Stable, non-prerelease SemVer release `v0.1.0`. Terraform does not select prereleases without an exact constraint, so a prerelease cannot satisfy the normal clean-install gate. Freezing this version does not authorize tag creation or publication. |
| Binary version | `0.1.0` without the tag's leading `v`, injected into `main.version`; `dev`, empty, or a value different from the release version fails the release-contract check. |
| Terraform CLI | Minimum supported language/protocol boundary is `>= 1.0.0`, because Protocol 6 requires Terraform 1.0 or later. Release qualification is pinned to `1.0.11` (last 1.0 patch), `1.5.7` (representative intermediate), and `1.15.8` (current stable on 2026-08-05). These become public tested claims only after P5-030 passes with packaged binaries. |
| Go | CI and release builds use exactly Go `1.25.8`, matching `go.mod` and the inspected current HashiCorp Plugin Framework scaffold. A newer local Go toolchain is not release evidence. |
| Plugin protocol | Protocol `6.0` only, served with `providerserver.NewProtocol6` and declared by manifest format version `1`. |

These values were reconciled against HashiCorp's current
[Protocol 6 compatibility contract](https://developer.hashicorp.com/terraform/plugin/terraform-plugin-protocol#protocol-version-6),
[official Terraform release index](https://releases.hashicorp.com/terraform/),
[provider publishing requirements](https://developer.hashicorp.com/terraform/registry/providers/publishing),
and [Plugin Framework scaffold](https://github.com/hashicorp/terraform-provider-scaffolding-framework),
rather than copied from its template unchanged.

Patch releases within a published minor line are backward-compatible bug and
security fixes. Additive resources, data sources, and optional capabilities use
a minor release. Removing or renaming an existing attribute/object, changing a
type, identity, default, ownership/replacement rule, canonical state meaning,
or rejecting a previously valid configuration or Import ID is breaking. Because
the initial release is `v0.1.0`, patches remain compatible within `0.1.x`; a
breaking change may occur only at a new minor such as `v0.2.0` and requires an
explicit migration decision and upgrade guidance. It must be called out rather
than hidden in a patch.

No published tag or asset may be replaced. A correction is always a new
version.

### Archive and native-verification matrix

The initial release produces exactly these five `CGO_ENABLED=0` archives:

| OS | Architectures |
|---|---|
| `darwin` | `amd64`, `arm64` |
| `linux` | `amd64`, `arm64` |
| `windows` | `amd64` |

Go 1.25.8 supports all five targets, this repository cross-builds for all five
without CGO, Terraform 1.0.11 published a matching CLI for each, and current
standard GitHub-hosted native runner families cover them. P5-030 must still
start and test each packaged target natively before the matrix becomes a public
compatibility claim. Windows ARM64, 32-bit, ARM32, FreeBSD, and other Go targets
are not initial-release archives: cross-compilation alone would not satisfy the
native release gate, and Terraform 1.0.11 did not publish Windows ARM64.

### Public schema, state, and Import compatibility

The snapshot freezes exactly five provider attributes, these four managed
resources, and the same four exact single-object data sources:

- `featbit_project`;
- `featbit_environment`;
- `featbit_feature_flag`; and
- `featbit_segment`.

It also freezes every Protocol-visible schema version, attribute/nested type,
Required/Optional/Computed/Sensitive/WriteOnly flag, description, and the
absence of provider-meta, functions, ephemeral/list resources, actions, and
state stores. The focused name assertions remain independent of snapshot
regeneration so adding an IAM or other registration requires a deliberate
contract change.

The accepted Import forms remain:

| Resource | Import ID |
|---|---|
| Project | `<project_uuid>` |
| Environment | `<project_uuid>/<environment_uuid>` |
| Feature Flag | `<environment_uuid>/<exact_key>` |
| Segment | `<environment_uuid>/<segment_uuid>` |

Compatible releases preserve existing configuration, refresh existing state,
and continue accepting all four forms. New state fields must be additive and
safe for old state, or ship with an explicit schema-version migration before
use. An additional Import spelling may be additive, but none of the frozen
forms may be removed or reinterpreted. New resources/data sources may be added
without changing the existing four lifecycles; IAM remains entirely outside
the initial release.

### Compatibility claims

The implemented core behavior was exercised against current FeatBit Cloud on
the dated gates recorded in the master plan. P5-031 must repeat that evidence
with the exact release candidate before publication. `api_url` makes another
documented `/api/v1` origin configurable; configurability is not a
compatibility certification. No named self-hosted FeatBit version or deployment
configuration has been tested, so the initial release makes no self-hosted
support claim.

### Repository baseline and external prerequisites

Read-only inspection on 2026-08-05 proved that
`featbit/featbit-terraform-provider` is a public, lowercase, correctly named
GitHub repository with default branch `main`, and that the checked-out module,
provider address, and repository namespace agree. It had no tag or GitHub
release. The tree had no public README, Registry docs/examples, GitHub Actions,
GoReleaser configuration, signing workflow, public security/contribution
guidance, or isolated Registry-install smoke test. Existing local gates are
`gofmt`, vet, unit/mock/Protocol tests, race, build, module tidiness/verification,
and separately opted-in trusted Cloud acceptance.

The following facts and actions cannot be inferred from source and remain
maintainer-owned prerequisites, not P5-010 implementation work:

- confirm that the intended Terraform Registry organization namespace is
  `featbit` and that the publishing GitHub account has organization-admin and
  Registry application access;
- select a Registry-compatible RSA release-signing identity (the current
  Registry documentation accepts RSA/DSA but not the default ECC key type),
  register only its public key for that namespace, and retain the private
  key/passphrase only in protected release secrets;
- create and approve the protected GitHub release environment and required
  secrets; the repository exposed no configured Actions environment or secret
  name during the read-only baseline;
- decide in P5-015 whether the workflow creates a draft for inspection before
  finalization; and
- explicitly authorize tag creation, GitHub release finalization, Registry
  connection/resynchronization, and publication in P5-033.

P5-010 used no FeatBit endpoint or credential, signing material, tag, GitHub
release mutation, repository-setting mutation, or Terraform Registry mutation.

## Registry documentation

The public repository must contain:

- a concise `README.md` covering scope, installation, authentication,
  supported resource families, development, testing, and security reporting;
- Registry `docs/index.md`;
- one generated-and-reviewed page for every resource and data source;
- credential-free examples for provider configuration, all four resource
  families, exact data-source lookup, and every Import form;
- an explicit statement that targeting owned by the UI remains outside the
  Feature Flag resource and that shared Segments are read-only; and
- no example token, tenant value, live object identifier, state, plan, cleanup
  journal, or copied Cloud response.

Use the official `terraform-plugin-docs` generator when its schema-derived
output matches the provider contract. Pin the tool and retain the narrow
templates or examples needed for human guidance. CI must regenerate into an
isolated or clean tree and fail on drift; a release must never silently publish
stale docs.

## Registry artifact contract

The current official
[provider publishing requirements](https://developer.hashicorp.com/terraform/registry/providers/publishing)
are the authority. At minimum, a final release must provide:

- the valid initial SemVer tag `v0.1.0` with no same-named branch;
- one zip per supported OS/architecture containing the correctly named
  `terraform-provider-featbit_v0.1.0` binary;
- `terraform-provider-featbit_0.1.0_manifest.json` declaring Protocol
  `6.0`;
- `terraform-provider-featbit_0.1.0_SHA256SUMS` covering every archive and
  the manifest; and
- a valid detached GPG signature
  `terraform-provider-featbit_0.1.0_SHA256SUMS.sig` made by the key registered
  for the Registry namespace.

Use a pinned GoReleaser v2 configuration derived from the current official
Terraform Plugin Framework scaffold, then narrow it to the platform matrix
frozen in `P5-010`. Builds are `CGO_ENABLED=0`, trimmed, version-injected
through `main.version`, and free of local paths. Snapshot builds may exercise
the complete packaging path but must not create a tag, GitHub release, Registry
version, or reusable signing artifact.

Never replace assets for an already published version. A required correction
is a new version.

## CI and privilege separation

Credential-free pull-request and ordinary branch CI:

- uses read-only repository permissions;
- works for untrusted forks without secrets;
- never uses `pull_request_target` to execute or package contributor code;
- pins every action to a verified full commit SHA;
- runs format, vet, unit/Protocol tests, race where supported, build,
  module-tidiness/verification, documentation drift, dependency/license/
  vulnerability checks, secret scanning, and snapshot packaging as applicable;
  and
- leaves trusted FeatBit Cloud tests skipped because no token is present.

Trusted current-Cloud acceptance is a separate, explicitly invoked or protected
workflow. It receives only the required token through a protected environment,
uses a unique test-owned prefix, operates only on its own core objects, and
proves exact child-first cleanup. It never enumerates or modifies unrelated
projects.

The tag release workflow is separate again. It has only the permissions needed
to create GitHub release assets and receives the GPG key/passphrase only after
the protected release boundary. It must not process an untrusted checkout,
reuse artifacts from an untrusted workflow, print secret-derived data, or
publish when validation fails.

## Compatibility and installation verification

Before publication, snapshot assets must be verified independently of the
GoReleaser job:

- archive names and contents match the frozen matrix;
- every checksum verifies and the detached signature verifies with the public
  key;
- the embedded provider version matches the proposed release;
- the manifest parses and advertises Protocol `6.0`;
- representative native runners can start the packaged provider and retrieve
  its exact schema; and
- a clean Terraform directory installs the candidate through an isolated
  filesystem/network-mirror path without a development override.

The release candidate then runs the frozen Terraform CLI compatibility matrix.
The trusted current-Cloud smoke test covers Project, Environment, all four
Feature Flag types, an environment-specific Segment, Import, second-plan
idempotence, drift repair, dependency-ordered destroy, and independent exact
cleanup. Shared Segment behavior remains public-contract/Protocol verified
unless an explicitly owned shared fixture exists; lack of one never authorizes
reading unrelated objects.

After publication, a new clean directory must install the exact Registry
version and repeat schema, plan, apply, Import, empty second plan, and destroy
smoke tests without local overrides.

## Security, support, and supply chain

- Keep credentials, GPG private material, passphrases, Cloud runtime values,
  Terraform state, plans, and generated logs out of the repository and release
  assets.
- Publish a security policy with a private reporting path, supported-version
  policy, and response expectations; do not ask reporters to disclose a
  vulnerability publicly.
- Publish contribution, development, test, documentation-generation, and
  release-maintainer guidance without embedding workstation-specific paths.
- Record an upgrade and compatibility policy before the first stable public
  contract.
- Pin quality/release tools and GitHub Actions. Review licenses, vulnerabilities,
  provenance, permissions, and maintenance before adding them.
- Produce and inspect a software bill of materials when the chosen release
  tooling supports it without weakening the required Registry artifact names
  or signature contract.
- Scan the current tree and final archives for secrets and unexpected files.

## Maintainer-owned external actions

The following actions require explicit maintainer authorization and cannot be
inferred from implementation work:

- creating/importing the release GPG key and storing protected secrets;
- registering the public signing key with the Terraform Registry namespace;
- changing GitHub organization/repository Actions or environment settings;
- creating or pushing a release tag;
- publishing/finalizing a GitHub release;
- connecting or resynchronizing the provider in the Terraform Registry; and
- announcing the release.

Tasks may prepare and locally verify everything before these boundaries. Stop
and request authorization before performing an external action that has not
already been explicitly requested.

## Execution order

1. Freeze the core-only release, compatibility, versioning, and platform
   contract.
2. Add Registry documentation, examples, and a reproducible drift check.
3. Add public security, contribution, support, and upgrade guidance.
4. Add fork-safe credential-free CI with pinned tools/actions.
5. Add reproducible GoReleaser packaging, checksums, signing configuration,
   and snapshot verification.
6. Add the isolated protected release workflow.
7. Prove packaged-provider installation and the Terraform/platform
   compatibility matrix.
8. Run the trusted core-only current-Cloud release-candidate gate and the full
   local/supply-chain gate.
9. With explicit maintainer authorization, publish and verify the initial
   GitHub/Registry release.

## Out of scope

- Every IAM member, group, policy, team, relationship, tenant-context, or
  context-header capability. These begin in Phase 6.
- New core resource fields, generic raw-REST resources, Portal-private APIs,
  backend/database access, flag evaluation, deployments, Relay Proxy,
  analytics, or audit streams.
- Modifying unrelated FeatBit projects, shared objects, account settings,
  tokens, organization membership, or repository settings outside a separately
  authorized release prerequisite.
- Certifying an unspecified self-hosted version.
- Publishing an unsigned, mutable, unverified, or locally overridden provider
  as the initial release.

## Exit gate

- All items in [todo.md](todo.md) are complete.
- The published provider exposes exactly five provider attributes, four core
  resources, four data sources, Protocol `6.0`, and no IAM surface.
- README, Registry docs, examples, security/contribution/support guidance, and
  upgrade policy match the frozen schema and contain no credentials or runtime
  tenant/object values.
- Fork pull requests pass credential-free read-only CI; no untrusted workflow
  can access release or Cloud secrets.
- Snapshot and final artifacts match the frozen OS/architecture matrix,
  contain the correct versioned binary and manifest, pass checksum/signature/
  provenance inspection, and contain no unexpected file.
- The Terraform CLI/platform compatibility matrix and clean-directory
  prepublication install tests pass.
- Trusted current-Cloud release-candidate tests pass only against uniquely
  test-owned core objects and independently prove exact cleanup without reading
  or mutating unrelated projects.
- Formatting, vet, unit/race/Protocol tests, build, module/dependency/license/
  vulnerability checks, docs drift, secret scans, and repository diff checks
  pass.
- With explicit maintainer authorization, the signed GitHub release is final,
  the Terraform Registry serves the exact version, and a new clean directory
  completes init/schema/plan/apply/Import/empty-plan/destroy without a local
  override.
- The current plan identifies Phase 6 IAM as post-initial-release work.

After the gate passes, fold only still-current architecture, compatibility, and
roadmap facts into [the master plan](../plan.md), delete this Phase 5 package,
and create only the Phase 6 IAM README/TODO.
