# FeatBit Terraform Provider Plan

- Status: **Active**
- Module: `github.com/featbit/terraform-provider-featbit`
- Registry address: `registry.terraform.io/featbit/featbit`
- Active work: [Phase 2 — project and environment](plan-execution-phase-2/README.md)

This file contains the current architecture, product contract, and phase
roadmap.

## 1. Current position

Phase 2 is active. The repository exposes a locally loadable Protocol v6
provider with its configuration schema, shared handwritten HTTP client
runtime, and registered `featbit_project` and `featbit_environment` resources
plus their exact single-object data sources. Their lifecycle-owned endpoint
adapters implement exact reads, CRUD, Import, canonical state, absence
confirmation, and settings-preserving Environment Update. The next action is
Phase 2 `P2-030`: prove their shared ownership, state-safety, and redaction
boundaries.

## 2. Product boundary

Core v1 manages these environment-scoped customer workflows:

- `featbit_project`
- `featbit_environment`
- `featbit_feature_flag`
- `featbit_segment`
- one exact single-object data source and Import support for each resource

IAM follows after the core resources: groups, policies, exact member lookup,
and independent group/member/policy binding resources. Member invitation or
creation remains external.

Core v1 does not evaluate flags, deploy FeatBit, manage analytics/audit streams,
copy LaunchDarkly's resource model, or expose a generic raw-REST resource. It
uses documented public FeatBit endpoints only and does not depend on backend
or public API changes.

FeatBit Cloud behavior for the core scope was verified on 2026-07-31.
Self-hosted is an intended target through a configurable API origin, but no
exact self-hosted release is currently certified.

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
exchange, token-kind selector, or organization/workspace context header. The
client never sends credentials outside the configured origin and `/api/v1`
path.

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

## 4. Current resource contracts

| Object | Terraform ownership and lifecycle |
|---|---|
| Project | UUID identity. Manage verified safe fields; key replaces. Server-created `Dev/dev` and `Prod/prod` environments are Computed. Confirm absence through the complete project collection when direct Read is ambiguous. |
| Environment | Project-scoped UUID identity. Name/description update; project and key replace. Discard secret values from ordinary state. Confirm absence through the parent project's environment collection. |
| Feature flag | Environment plus exact key identity. Support Boolean, String, Number, and JSON. Only name updates in place; key, type, description, and variations replace. Targeting, rules, rollouts, enabled state, and tags remain UI-owned. Destroy archives, hard-deletes, then proves exact zero in active and archived views. |
| Environment-specific segment | Environment plus UUID identity. Verified metadata, targeting, and tags update through specialized endpoints; key, type, and scopes replace. Destroy first checks flag references, then archives, hard-deletes, and proves exact active/archived absence. |
| Shared segment | Read/bind only; Terraform does not create, update, or delete it. |

Common lifecycle rules:

- Match exact IDs or parent-scoped exact keys across every page; never select
  the first fuzzy search result.
- Before Create, require exact zero. After an ambiguous mutation, reconcile by
  exact identity instead of retrying blindly or adopting an unrelated object.
- Read after each logical write and persist the canonical server form.
- Serialize writes only when a real multi-call lifecycle requires it.
- Archive is an internal deletion prerequisite, never final Terraform destroy
  state or a user-facing destroy option.
- Preserve state whenever absence or mutation outcome is ambiguous.

Core Import IDs are stable public contracts:

| Object | Import ID |
|---|---|
| Project | `<project_uuid>` |
| Environment | `<project_uuid>/<environment_uuid>` |
| Feature flag | `<environment_uuid>/<exact_key>` |
| Segment | `<environment_uuid>/<segment_uuid>` |
| Future IAM object | `<uuid>` |
| Future IAM binding | `<left_uuid>/<right_uuid>` |

## 5. Roadmap

### Phase 1 — Provider foundation (complete)

Establish the shared client, local developer workflow, local override, and
provider schema verification. Do not add resource endpoint models in this
phase.

Gate: local override loads the provider; `terraform providers schema -json`
succeeds; format, vet, unit/race, build, redaction, and dependency checks pass.

### Phase 2 — Project and environment (active)

Add only the Project/Environment endpoint adapters needed by their resources
and data sources. Implement exact lookup, CRUD, Import, replacement semantics,
canonical state, drift, and out-of-band deletion tests.

Gate: Create/Read/Update/Delete and Import converge to an empty plan.

### Phase 3 — Feature flags

Implement the constrained four-type feature-flag resource/data source with
stable variation identity, precise normalization, UI-field preservation,
replacement, Import, and archive-plus-hard-delete behavior.

Gate: every supported type converges without rewriting UI-owned operations.

### Phase 4 — Segments

Implement environment-specific segment resource/data source behavior,
ordered rules, set-valued users/tags, scope resolution, reference preflight,
Import, drift, and exact destroy. Keep shared segments read/bind only.

Gate: lifecycle and Import converge; reference conflicts preserve valid state.

### Phase 5 — IAM

After target-specific member lookup verification, add groups, policies, and
independent binding resources with composite IDs. Keep member creation external.

Gate: bindings are idempotent and never claim an entire shared relationship set.

### Phase 6 — Release

Add fork-safe credential-free pull-request CI with pinned quality tools, while
keeping live acceptance jobs separately trusted and scoped. Add Registry
documentation/examples, security and contribution guides, upgrade policy,
cross-platform release packaging, checksums/signatures, Cloud and self-hosted
compatibility runs, prerelease smoke tests, and Registry publication.

Gate: a clean directory can initialize, plan, apply, destroy, and import using
the Registry provider; release assets satisfy Registry requirements.

## 6. Global verification

Current runtime pins are authoritative in `go.mod`: Go `1.25.8`, Plugin
Framework `v1.19.0`, Plugin Go `v0.31.0`, Plugin Testing `v1.16.0`, and Plugin
Log `v0.10.0`. CI/release uses Go `1.26.5`; Protocol is `6.0`.

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
