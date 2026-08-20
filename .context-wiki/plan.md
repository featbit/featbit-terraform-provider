# FeatBit Terraform Provider Plan

- Status: **Active**
- Module: `github.com/featbit/terraform-provider-featbit`
- Registry address: `registry.terraform.io/featbit/featbit`
- Next phase: [Phase 6 — IAM and release](plan-execution-phase-6/README.md)

This file contains the current architecture, product contract, and phase
roadmap.

## 1. Current position

The Phase 6 public IAM API, frozen Terraform contracts, runtime implementation,
local Protocol workflow, trusted current-Cloud workflow, Registry
documentation, `v0.2.0-beta.1` release qualification, maintainer-authorized
publication, exact Registry installation, published-provider customer
scenario, and final candidate requalification have passed without a
release-blocking finding. The next step is maintainer-authorized publication
and Registry verification of stable `v0.2.0`. The current branch owns only
that IAM surface and its release; it must not
implement the deferred Phase 7 Segment prerequisite work. Stable releases
`v0.1.0` and documentation-only `v0.1.1` remain core-only, while the published
`v0.2.0-beta.1` prerelease exposes the additive IAM surface for validation.
That `0.2.x` line exposes a Protocol v6 provider with five
configuration attributes, a shared handwritten HTTP client, nine managed
resources, and seven exact single-object data sources. In addition to Project,
Environment, Feature Flag,
and Segment, it implements custom Policies with complete statements, Groups,
exact Group-Policy and Group-Member bindings, one existing Member's
authoritative direct-Policy set, and exact Policy, Group, and Member lookup.
The lifecycle-owned adapters implement exact reads, CRUD or narrow relationship
mutation as applicable, Import, canonical state, authoritative absence
composition, replacement-aware stable ID planning, one-shot mutation
reconciliation, and redaction-safe diagnostics.
The Segment resource manages only environment-specific metadata, targeting,
and tags through specialized public endpoints; exact shared Segment reads
remain data-source-only, and reference-aware destroy proves complete
active/archived absence. Segment targeting writes do not create Environment
End Users or register custom End User Property metadata because those
prerequisites are not exposed through the documented public API. The required
public operations are planned for a later FeatBit version, so that unfinished
Segment work is deferred until Phase 7, after IAM, and must not be implemented
through Portal-private endpoints in the meantime. All four core resource
phases passed their local, Protocol, and trusted current-Cloud gates with exact
cleanup. The `v0.2.0-beta.1` GitHub prerelease and exact Registry version
contain the complete approved IAM surface and pass customer-state upgrade,
IAM create/update/import, empty-plan, and exact-cleanup scenarios, while
`v0.1.1` remains GitHub's latest stable release. The checked-in Protocol v6
release snapshot and focused
registration, Import, manifest, and generated-document checks freeze that
surface. GoReleaser owns tag-derived version injection. Terraform `1.0.11`,
`1.5.7`, and `1.15.8`, the Linux race gate, and the frozen five-platform
GoReleaser archive matrix passed qualification; a clean direct Registry install
also loaded the exact beta with its signed published hashes and frozen schema.

## 2. Product boundary

Core v1 manages these environment-scoped customer workflows:

- `featbit_project`
- `featbit_environment`
- `featbit_feature_flag`
- `featbit_segment`
- one exact single-object data source and Import support for each resource

IAM is the first post-initial-release phase. IAM v1 manages custom Policies and
their statements as one Terraform resource, including Project, Environment,
Feature Flag, and Segment control levels; Groups; exact Group-Policy and
Group-Member bindings; and one existing Member's authoritative direct-Policy
set. It observes built-in Policies, existing organization-wide Groups, and
existing Members, and extends the existing Project and Environment data
sources with exact-key selection. Member
invitation, creation, profile mutation, organization/workspace removal, and
deletion remain external. Service Token management, including Service
Token-to-Group assignment, is excluded because the required public resource and
relationship contracts do not exist. The initial release exposes no IAM
surface.

Segment targeting prerequisite closure follows IAM as Phase 7. It must wait
for a later FeatBit version to expose documented public operations for exact
Environment End User and End User Property lookup plus create-missing-only
registration before the Provider depends on those operations. Phase 6 Segment
Policy statements authorize Segment operations only and do not depend on or
implement those targeting prerequisites.

Core v1 does not evaluate flags, deploy FeatBit, manage analytics/audit streams,
copy LaunchDarkly's resource model, or expose a generic raw-REST resource. It
uses documented public FeatBit endpoints only and does not depend on backend
or public API changes.

FeatBit Cloud behavior for the planned core scope was verified on 2026-07-31.
The implemented Project/Environment contracts and exact-zero cleanup were
reverified against the current Cloud API on 2026-08-03, and the four-type
Feature Flag lifecycle passed its current-Cloud gate with exact active and
archived cleanup on the same date. The environment-specific Segment lifecycle
passed on 2026-08-04 with exact Segment and parent cleanup. No safely owned
shared Segment fixture was available, so shared reads remain verified through
public-contract and Protocol tests without inspecting or mutating unrelated
objects. Each post-release resource must pass its own current-Cloud gate.
Self-hosted is an intended target through a configurable API origin, but no
exact self-hosted release is currently certified and the initial release must
not claim otherwise.

## 3. Current architecture

```text
Terraform Core
  -> Protocol v6 provider
     -> resource or data-source lifecycle
        -> endpoint adapter owned by that lifecycle
           -> shared handwritten HTTP client
              -> documented FeatBit /api/v1 endpoint
```

| Area | Responsibility |
|---|---|
| `main.go` | Starts the Protocol v6 provider and supplies the build version. |
| `internal/provider` | Provider schema, configuration, resources, data sources, Terraform models, expand/flatten, and diagnostics. |
| `internal/client` | Shared transport, authentication, request execution, envelope/errors, retries, concurrency, and redaction. |
| Resource-phase endpoint files | Only the request/response types and methods used by their production Terraform caller. |

Architecture rules:

- The root provider is the only Go module.
- API client code is handwritten. There is no OpenAPI snapshot, generator,
  generated package, probe module, or speculative tools module.
- Add an endpoint adapter only with its first production resource or data
  source. Its focused tests define method, escaped path, query, JSON body,
  envelope, error, and cancellation behavior.
- Add pagination, exact-existence composition, normalization, reconciliation,
  and per-object write serialization with the concrete lifecycle that needs
  them.
- Reuse the existing escaped request construction, UUID validation, exact
  zero/one/duplicate resolution, ProviderData checks, error classification,
  cancellation, retry, and redaction contracts whenever their ownership and
  safety boundaries match.
- Computed resource IDs remain known during in-place plans only when every
  identity-defining input is unchanged; replacement plans leave them unknown.
- Terraform schemas remain handwritten because ownership, Null/Unknown,
  Sensitive, ordering, replacement, Import, and state behavior are product
  decisions rather than transport shapes.

### Provider configuration

| Attribute | Environment fallback | Default and bounds |
|---|---|---|
| `api_url` | `FEATBIT_API_URL` | `https://app-api.featbit.co`, normalized to `/api/v1` |
| `access_token` | `FEATBIT_ACCESS_TOKEN` | Required and Sensitive |
| `http_timeout_seconds` | `FEATBIT_HTTP_TIMEOUT_SECONDS` | `30`, range `1..300` |
| `max_concurrency` | `FEATBIT_MAX_CONCURRENCY` | `4`, range `1..32` |
| `max_retries` | `FEATBIT_MAX_RETRIES` | `3`, range `0..10` |

The token is sent directly in `Authorization`, without a Bearer prefix, login
exchange, or token-kind selector. The implemented core transport strips
organization/workspace context headers and never sends credentials outside the
configured origin and `/api/v1` path. The initial release preserves that
five-attribute schema and transport boundary. Any future IAM tenant/context
contract must be proven in its post-release phase before provider
configuration or transport changes.

### Shared HTTP contract

- Send `terraform-provider-featbit/<version>` as User-Agent.
- Propagate context cancellation through admission, HTTP execution, and retry
  waits.
- Limit each buffered response to 16 MiB and hold one concurrency permit until
  its body is completely read.
- Decode FeatBit's `{success,data,errors}` envelope, including HTTP `2xx` with
  `success=false`.
- Classify validation, authentication, authorization, unconfirmed absence,
  conflict, rate limit, transient server, application, timeout, cancellation,
  network, and ambiguous failures centrally.
- Retry only bodyless `GET` requests on `429`, transient `5xx`, timeout, or
  network failure. Mutations execute once.
- Treat `401` and `403` as authentication/authorization failures that preserve
  state. A direct `404` is not automatically authoritative absence.
- Never expose tokens, secret values, tenant/member identities, request paths,
  raw response bodies, or unsafe network errors in logs or diagnostics.

## 4. Current and planned resource contracts

| Object | Status | Terraform ownership and lifecycle |
|---|---|---|
| Project | Implemented | UUID identity. Manage verified safe fields; key replaces. Server-created `Dev/dev` and `Prod/prod` environments are Computed. Confirm absence through the complete project collection when direct Read is ambiguous. |
| Environment | Implemented | Project-scoped UUID identity. Name/description update; project and key replace. Discard secret values from ordinary state. Confirm absence through the parent project's environment collection and preserve UI-owned settings across Update. |
| Feature flag | Implemented | Environment plus exact key identity. Support Boolean, String, Number, and JSON. Only name updates in place; environment, key, type, description, and variations replace. Targeting, rules, rollouts, enabled state, and tags remain UI-owned. Destroy archives, hard-deletes, then proves exact zero in complete active and archived views. |
| Environment-specific segment | Implemented with a targeting prerequisite gap | Environment plus UUID identity. Manage name, description, included/excluded targeting keys, ordered rules/conditions, and tags through specialized endpoints; key and scopes are immutable. The current public API does not let the Provider create missing Environment users or custom-property metadata. Phase 7 will close that gap after the required public API ships, without overwriting or deleting shared prerequisite data. Destroy refuses exact Feature Flag references, then archives, hard-deletes, and proves exact active/archived absence without deleting users or property metadata. |
| Shared segment | Implemented, read-only | Exact data-source observation only; Terraform cannot create, update, archive, restore, or delete it. |
| IAM member | Published in `v0.2.0-beta.1` for beta validation | `featbit_member` reads one existing Member by exact ID or case-insensitive full email and exposes only sensitive ID/email/name. `featbit_member_direct_policies` owns that Member's complete direct Policy set; an empty set and Destroy remove direct Policies only. Invitation, profile, organization/workspace membership, deletion, and inherited Policies remain external, and `initialPassword` is never decoded into the Provider model. |
| IAM group and custom policy | Published in `v0.2.0-beta.1` for beta validation | The `featbit_group` resource owns Group existence and name/description but no relationships; its data source observes an existing Group by exact ID or organization-scoped, case-sensitive exact name without adopting it. `featbit_policy` owns one custom Policy's settings and complete unordered statement set; its exact-key data source can also observe built-in Policies, but every built-in mutation is structurally forbidden. Statements cover only Project, Environment, Feature Flag, and Segment with exact lower-case types/effects, the frozen action catalogs, and canonical wildcard/exact-key/tag selectors. |
| IAM relationship edge | Published in `v0.2.0-beta.1` for beta validation | `featbit_group_policy_binding` and `featbit_group_member_binding` each own one exact pair and never a complete Group collection. Group and Policy destroy refuse to cascade live relationships. The authoritative direct-Policy resource never reads inherited Policies as owned state or changes Group edges. |

Common lifecycle rules:

- Match exact IDs, parent-scoped exact keys, or organization-scoped exact names
  across every page; never select the first fuzzy search result.
- Before Create, require exact zero. After an ambiguous mutation, reconcile by
  exact identity instead of retrying blindly or adopting an unrelated object.
- Read after each logical write and persist the canonical server form.
- Keep a computed ID known only for an in-place plan; an identity-changing
  replacement must plan a new unknown ID.
- Serialize writes only when a real multi-call lifecycle requires it.
- Archive is an internal deletion prerequisite, never final Terraform destroy
  state or a user-facing destroy option.
- Preserve state whenever absence or mutation outcome is ambiguous.

Implemented core Import IDs are stable public contracts:

| Object | Import ID |
|---|---|
| Project | `<project_uuid>` |
| Environment | `<project_uuid>/<environment_uuid>` |
| Feature flag | `<environment_uuid>/<exact_key>` |
| Segment | `<environment_uuid>/<segment_uuid>` |

IAM remains absent from the stable `0.1.x` releases. Its frozen Phase 6 Import
forms are published in the `v0.2.0-beta.1` prerelease:

| Object | Import ID |
|---|---|
| Custom Policy | `<policy_uuid>` |
| Group | `<group_uuid>` |
| Group-Policy binding | `<group_uuid>/<policy_uuid>` |
| Group-Member binding | `<group_uuid>/<member_uuid>` |
| Member direct Policies | `<member_uuid>` |

## 5. Roadmap

### Phase 1 — Provider foundation (complete)

Establish the shared client, local developer workflow, local override, and
provider schema verification. Do not add resource endpoint models in this
phase.

Gate: local override loads the provider; `terraform providers schema -json`
succeeds; format, vet, unit/race, build, redaction, and dependency checks pass.

### Phase 2 — Project and environment (complete)

Add only the Project/Environment endpoint adapters needed by their resources
and data sources. Implement exact lookup, CRUD, Import, replacement semantics,
canonical state, drift, and out-of-band deletion tests.

Gate: Create/Read/Update/Delete and Import converge to an empty plan.

### Phase 3 — Feature flags (complete)

Implement the constrained four-type feature-flag resource/data source with
stable variation identity, precise normalization, UI-field preservation,
replacement, Import, and archive-plus-hard-delete behavior.

Gate: every supported type converges without rewriting UI-owned operations.

### Phase 4 — Segments (complete)

Implement environment-specific segment resource/data source behavior,
ordered rules, set-valued users/tags, scope resolution, reference preflight,
Import, drift, and exact destroy. Keep shared segments read/bind only, and keep
Environment-user and custom-property metadata registration outside the
resource while the documented public API does not expose those prerequisites.

Gate: lifecycle and Import converge; reference conflicts preserve valid state.

### Phase 5 — Initial release (complete)

Add fork-safe credential-free pull-request CI with pinned quality tools, while
keeping live acceptance jobs separately trusted and scoped. Add Registry
documentation/examples, security and support guidance, upgrade policy,
cross-platform release packaging, checksums/signatures, Cloud and self-hosted
compatibility claims backed by evidence, prerelease smoke tests, and Registry
publication. The release schema contains exactly the four implemented core
resources and four data sources; it contains no IAM surface.

Gate: a clean directory can initialize, plan, apply, destroy, and import using
the Registry provider; release assets satisfy Registry requirements.

### Phase 6 — IAM and release

Verify access-token tenant scope, optional context-header behavior, complete
exact lookup, and every required IAM mutation before freezing schemas. The
aligned surface consists of custom Policies with statements, Groups, exact
Group-Member and Group-Policy bindings, an authoritative direct-Policy set for
one existing Member, exact built-in Policy, existing Group, and Member lookup,
and exact-key Project/Environment lookup. Policy statements cover Project, Environment,
Feature Flag, and Segment control levels, including documented wildcard and
exact-key resource selectors. Member lifecycle and Service Tokens are external.

Gate: every consumed operation is documented and exact; lifecycle and Import
identities are frozen before publication; exact-pair bindings and the explicit
per-Member direct-Policy set preserve their ownership boundaries; all four
Policy control levels round-trip with canonical effects, actions, and resource
selectors; current-Cloud verification is redaction-safe; Registry documentation
and release artifacts describe exactly the implemented IAM surface; the beta
is published and exercised in real scenarios; every resulting release blocker
is resolved and requalified; and stable `v0.2.0` is published only afterward.
Tag creation, signing, draft finalization, and publication remain separately
maintainer-authorized.

### Phase 7 — Segment targeting prerequisites (deferred)

Begin only on a separate future branch after the IAM release and after a later
FeatBit version exposes the required documented public operations. Verify that
the official Swagger and OpenAPI authentication expose stable operations for
exact Environment End User and End User Property lookup plus
create-missing-only registration. If that contract is still absent, record the
minimum upstream API requirement and stop: the Provider must not call
Portal-private endpoints, modify the FeatBit backend, or infer authority from
UI behavior.

Once the public contract exists, choose and freeze the Terraform ownership
model before implementation. The preferred Segment lifecycle behavior is to
deduplicate included/excluded keys and custom rule properties, query the exact
Environment prerequisites, register only missing values, never overwrite an
existing user's name or custom properties, ignore built-in properties such as
`keyId` and `name`, and never delete users or property metadata when targeting
changes or a Segment is destroyed. Registration must complete before the
targeting mutation; partial failure must preserve truthful state and produce
redacted diagnostics. Concurrency, duplicate/conflicting keys, Import,
refresh, drift detection, cancellation, and second-plan idempotence require
focused contracts. If a first-class End User or End User Property resource is
safer than implicit ensure behavior, the phase must document a non-destructive
migration and ownership model before adding it.

Gate: a fresh Environment-specific Segment with new included/excluded user
keys and a repeated custom property registers only missing prerequisites,
preserves existing records, converges to an empty second plan, reports any
registration failure instead of false success, and leaves every prerequisite
intact after targeting removal and Segment destroy. Trusted current-Cloud
acceptance must query exact Environment users and properties and expose no
token, Environment ID, user key, property name, or targeting value.

## 6. Global verification

Current runtime pins are authoritative in `go.mod`: Go `1.26.6`, Plugin
Framework `v1.19.0`, Plugin Go `v0.31.0`, Plugin Testing `v1.16.0`, and Plugin
Log `v0.10.0`. Protocol is `6.0`, so the minimum Terraform CLI is `1.0.0`.
Release qualification passes on credential-free Linux/AMD64 with Terraform
`1.0.11`, `1.5.7`, and `1.15.8` through the existing Protocol contract.
Initial archives are limited to `darwin_amd64`, `darwin_arm64`, `linux_amd64`,
`linux_arm64`, and `windows_amd64`, cross-built without CGO and checked by the
GoReleaser snapshot. The credential-free Go 1.26.6 snapshot produces
exactly those five archives. This archive matrix is a distribution contract,
not a claim that every target has a separate native-runner qualification. The
Registry serves stable non-prerelease releases `v0.1.0` and `v0.1.1` plus the
exact IAM prerelease `v0.2.0-beta.1`; `v0.1.1` changes documentation, examples,
and roadmap context only and does not change runtime behavior, schema, state,
or compatibility.
The repository contains fork-safe, read-only, credential-free CI with pinned
actions and quality/supply-chain tools; deterministic five-platform GoReleaser
packaging; a protected SemVer-tag workflow that creates a signed draft and
keeps prereleases from replacing the latest stable release; and the existing
frozen Protocol schema contract. The current release design
intentionally follows the scaffold without a custom artifact verifier or
clean-install harness. Release signing identity, protected environment, tag
creation, draft inspection/finalization, Registry connection, and publication
remain maintainer-owned gates and do not expand the frozen core schema.

Every applicable phase gate includes:

- `gofmt`, `go vet`, `go test`, `go test -race`, `go build`, and module
  verification;
- focused mock contracts for every consumed endpoint;
- acceptance coverage for CRUD, Import, second-plan idempotence, drift,
  replacement, out-of-band deletion, exact lookup, and cleanup;
- zero credential/secret leakage in diagnostics, logs, fixtures, or repository
  content; and
- preservation of existing user changes and cleanup of every test-created
  remote object.
