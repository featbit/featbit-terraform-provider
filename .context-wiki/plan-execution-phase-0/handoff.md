# Phase 0 Handoff

Handoff state: **Ready for Phase 1**
Prepared: **2026-07-31**

## Exit-gate result

Phase 0 passed. All 76 TODOs have linked evidence or an explicit constrained,
read/bind-only, external, omitted, or unavailable-target decision. All five
ADRs are Accepted, the only available target is complete, every created Cloud
object is absent, and the cleanup inventory is empty.

Start with the formal
[Phase 1 execution package](../plan-execution-phase-1/README.md). Phase 1 is
the provider/client scaffold only; it must not implement production resources.

## Accepted decisions

- [ADR-001 — Terraform/UI ownership](adrs/ADR-001-terraform-ui-ownership.md)
- [ADR-002 — Delete versus archive](adrs/ADR-002-delete-vs-archive.md)
- [ADR-003 — Pinned OpenAPI client and overlay](adrs/ADR-003-openapi-client-and-overlay.md)
- [ADR-004 — Public Import identities](adrs/ADR-004-import-identities.md)
- [ADR-005 — Supported compatibility matrix](adrs/ADR-005-supported-compatibility-matrix.md)

The target-specific result is in
[compatibility-matrix.md](compatibility-matrix.md), and capability levels are
in [findings.md](findings.md#capability-support-matrix).

## Pinned Phase 1 inputs

### Toolchain and OpenAPI

- Go module baseline `1.25.8`; CI `1.25.12` and `1.26.5`; release `1.26.5`.
- Terraform Plugin Framework `v1.19.0`, Plugin Testing `v1.16.0`, Plugin Go
  `v0.31.0`, Plugin Log `v0.10.0`, Plugin Docs `v0.25.0`.
- Protocol `6`; Registry manifest protocol `6.0`.
- Terraform CLI minimum `1.0.0`; current test line `1.15.8`.
- `oapi-codegen v2.8.0`.
- Upstream snapshot:
  [featbit.openapi.json](../../internal/client/openapi/featbit.openapi.json),
  SHA-256 `8DE202F939F6721748D66449C3DFE4EEE2E2BF369A57F121DF808907A44D11C4`.
- Applied input:
  [featbit.overlayed.openapi.json](../../internal/client/openapi/featbit.overlayed.openapi.json),
  SHA-256 `C4E7165D13A5D12FB1990BC66572FB0D330D792843F398408900623BCE92BF60`.
- Locks, overlay, inventory, and generation configuration live in
  [internal/client/openapi](../../internal/client/openapi/README.md).

### Provider authentication

- One Sensitive `access_token`, with `FEATBIT_ACCESS_TOKEN` fallback.
- Configurable `api_url`, with `FEATBIT_API_URL` fallback and the documented
  Cloud default.
- Send the token value directly in `Authorization`.
- “Personal” and “service” labels are not separate provider schema types; the
  permission set on the token controls effective capability.
- Use provider aliases for different account contexts.
- Do not implement login, username/password, Bearer/JWT refresh, MFA, SSO, or
  organization/workspace context headers.

Evidence: [authenticated Cloud read](evidence/20260731-cloud-auth.md)
and [auth findings](findings.md#authentication-findings).

### Client behavior

- Decode the FeatBit `{success,data,errors}` envelope centrally. A 2xx response
  with `success=false` is still an application error.
- Classify every 401 as authentication and every 403 as authorization; preserve
  state and do not retry writes. Do not branch on unverified inactive/scope
  body codes.
- Retry only safe reads for 429, transient 5xx, timeouts, or network failure;
  honor cancellation and `Retry-After`. Never blindly retry a mutation.
- Read every advertised page and select only an exact scoped ID/key. Zero, one,
  and duplicate matches are distinct; never select the first fuzzy result.
- A direct 404/403/500 does not alone remove state. Use the exact scoped
  fallback for the resource family and preserve state on ambiguity.
- Before Create, require exact zero. On an ambiguous Create, re-read exactly;
  never auto-adopt or repeat the write. Return an exact Import/recovery
  diagnostic when one object exists.
- Redact tokens, authorization values, secrets, runtime IDs/keys, tenant
  context, and member emails from logs and diagnostics.
- Serialize writes per object, checkpoint partial creates, read after every
  logical write, and compare canonical state.

Evidence: [offline client/cleanup contracts](evidence/20260731-offline-contracts.md)
and [Cloud compatibility](evidence/20260731-cloud-project-environment.md).

## Later resource contracts

These are implementation inputs for later phases, not authorization to start
them during Phase 1.

| Resource | Identity/Import | Managed contract |
|---|---|---|
| Project | project UUID | name updates in place; key replaces; auto-created environments are Computed; Delete may require complete project-list exact-zero fallback |
| Environment | `<project_uuid>/<environment_uuid>` | name/description update; parent/key replace; secret values never enter ordinary state; Delete may require parent environment-list exact-zero fallback |
| Feature flag | `<environment_uuid>/<exact_flag_key>` | Boolean, String, Number, JSON; name-only in-place update; key/type/description/variations replace; deterministic variation UUIDs; targeting/rules/rollouts/enabled/tags Computed-only or omitted |
| Environment-specific segment | `<environment_uuid>/<segment_uuid>` | name/description/targeting/tags update through documented specialized endpoints; key/type/scopes replace; exact scope prefix resolved in memory and never retained |

Feature-flag canonical values:

- Boolean: lowercase `true`/`false`.
- String: preserve bytes.
- Number: validate/canonicalize without `float64`, preserving large integers and
  decimal precision.
- JSON: validate one JSON value and canonicalize key ordering/decimal spelling.
- Map variations by stable UUID, never list position.

Flag Cloud evidence covers
[all four types](evidence/20260731-cloud-feature-flags.md).

Destroy behavior:

- Flag: archive exact owned key, hard Delete, require direct 404 and all-page
  active-plus-archived exact zero.
- Segment: call documented flag-reference preflight; if non-empty, skip DELETE
  and preserve state. Otherwise archive, hard Delete, require direct 404 and
  all-page active-plus-archived exact UUID/key zero.
- Archive alone never completes Terraform destroy.
- Restore and post-delete key-reuse guarantees are omitted.

Evidence: [ADR-002](adrs/ADR-002-delete-vs-archive.md) and
[segment lifecycle](evidence/20260731-cloud-segments.md).

## Deliberate reductions

- Flag targeting, rules, rollouts, enabled state, and tags remain UI-owned.
- Shared segments are read/bind only; never mutate a visible pre-existing row.
- Member creation/invitation is external. Member lookup and IAM resources are
  deferred from core v1 until a later IAM-stage target probe verifies them.
- Environment secret values and rotation are external; ordinary resources may
  expose only Computed non-value metadata.
- Audit/analytics, deployments, webhooks, approvals, triggers, scheduled
  changes, experiments, and metrics are omitted from managed state.
- No generic raw-REST Terraform resource and no undocumented endpoint.

## Compatibility boundary

- `cloud-current`: verified on 2026-07-30/31 for direct auth, project,
  environment, Boolean/String/Number/JSON flags, one complex
  environment-specific segment, exact absence, and cleanup.
- Self-hosted: no disposable target or exact version was available, so every
  row remains `Not tested`; this is not an observed failure and no minimum
  release is certified.
- Provider v1 remains deployment-neutral through configurable `api_url` and the
  pinned documented public REST contract. Certifying a self-hosted release
  requires rerunning the matrix on an approved disposable instance.

## Cleanup and local verification

- All Cloud-created flags, segment, environments, projects, and automatic
  child environments are deleted and exactly absent.
- Final and independent cleanup checks: `pending=0`; no manual owner/action.
- The empty runtime cleanup file was removed. Nineteen narrow evidence records
  were consolidated into five indexed records without changing conclusions.
- `go test ./...`, `go vet ./...`, repeated normalization/probe tests,
  deterministic OpenAPI check, context/link/evidence check, redaction/secret
  scan, and `git diff --check` pass.
- `go test -race ./...` could not run on this workstation because no C compiler
  is installed; Phase 1 CI must run it in the pinned toolchain environment.

## Exact next action

Before Phase 1 implementation, review and simplify the remaining completed
Phase 0 top-level documents and `tools/` as a separate task. Preserve the five
Accepted ADRs, compatibility conclusions, five consolidated evidence records,
and `internal/client/openapi` inputs. After that cleanup, start with
[Phase 1 TODO P1-001](../plan-execution-phase-1/todo.md).
