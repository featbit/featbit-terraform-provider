# Phase 0 Status

Last updated: **2026-08-01**
Phase state: **Complete — Ready for Phase 1**
Exit gate: **Passed**
TODO progress: **76 / 76 completed**

## Outcome

Phase 0 empirically characterized the current documented FeatBit REST API on
every available target and converted all remaining ambiguity into an explicit
constrained, read/bind-only, external, omitted, or unavailable-target decision.
No backend change or undocumented endpoint is required for provider v1.

Phase 1 may start from [handoff.md](handoff.md). No production Terraform
resource was implemented during Phase 0.

## Target results

| Target | Result | Scope |
|---|---|---|
| `cloud-current` | Verified on 2026-07-30/31 | direct access-token auth; project/environment lifecycle; Boolean, String, Number, and JSON flags; one complex environment-specific segment; exact absence; normalization; cleanup |
| self-hosted | `Not tested` — no disposable target or exact release was available | no failure observed and no exact-release certification claimed; provider contract remains deployment-neutral through configurable API URL and the pinned public API |

The complete target-specific record is
[compatibility-matrix.md](compatibility-matrix.md).

## Verified core contract

- Direct permission-scoped access token in `Authorization`; no login,
  username/password, JWT refresh, MFA, SSO, or context headers.
- Pinned OpenAPI snapshot, operation-ID-only overlay, deterministic inventory,
  and exact generator/toolchain versions.
- Exact all-page identity lookup; zero/one/duplicate classifications; no fuzzy
  or first-result adoption.
- Central envelope/error handling, exact absence fallbacks, no mutation retry,
  safe-read-only retry, redaction, and partial-create recovery.
- Project and environment CRUD, canonical reads, narrow updates, duplicate-key
  behavior, delete/cascade behavior, and exact absence.
- Boolean, String, Number, and JSON flag Create/Read/name Update/canonical
  empty-diff behavior with stable requested variation IDs.
- Environment-specific segment Create/Read/specialized Update, complex
  canonical empty diff, archive/restore observation, reference preflight, and
  exact absence.
- Environment secret metadata shape is readable; values are discarded before
  reporting/state.

## Required workarounds and reductions

- Feature flags: key, type, description, and variations replace; only name
  updates in place. Targeting, rules, rollouts, enabled state, and tags remain
  UI-owned and are Computed-only or omitted.
- Flag destroy: archive, hard Delete, direct 404, then exact-zero active and
  archived scans. Archive alone is not destroy completion.
- Segment destroy: preflight documented flag references; non-empty stops before
  DELETE. Otherwise archive, hard Delete, direct 404, and exact-zero active and
  archived scans.
- Shared segments are read/bind only. Restore/key-reuse guarantees are omitted.
- Child duplicate response codes are not required: preflight exact zero, write
  once, then exact reconciliation/Import diagnostic on ambiguity.
- Inactive/restricted token bodies are not required: all 401/403 responses
  preserve state and mutations are not retried.
- Member creation is external. Member lookup and IAM are deferred from core v1
  until later target-specific IAM evidence exists.
- Secret rotation, audit/analytics, experiments/metrics, webhooks, approvals,
  triggers, scheduled changes, and generic raw REST are external or omitted.

The complete support-level table is in
[findings.md](findings.md#capability-support-matrix).

## Accepted ADRs

- [ADR-001 — Terraform/UI ownership](adrs/ADR-001-terraform-ui-ownership.md)
- [ADR-002 — Delete versus archive](adrs/ADR-002-delete-vs-archive.md)
- [ADR-003 — Pinned OpenAPI client and overlay](adrs/ADR-003-openapi-client-and-overlay.md)
- [ADR-004 — Public Import identities](adrs/ADR-004-import-identities.md)
- [ADR-005 — Supported compatibility matrix](adrs/ADR-005-supported-compatibility-matrix.md)

## Cleanup

- Every Cloud-created flag, segment, explicit environment, project, and
  automatically created child environment was deleted and exactly absent.
- Flag/segment absence was checked in both active and archived views.
- Final run and independent cleanup check: `pending=0`.
- No manual cleanup owner/action remains.
- The empty ignored runtime inventory was deleted. Nineteen narrow evidence
  records were consolidated into five indexed records; reusable probe code and
  all decision links remain.

## Verification

- `gofmt -l cmd internal`: empty.
- `go test ./...`: pass.
- `go vet ./...`: pass.
- `go test -count=20 ./internal/normalization ./internal/probe`: pass.
- Live Cloud String/Number/JSON type-matrix command: pass; cleanup `pending=0`.
- Independent `cleanup --dry-run`: `pending=0`.
- `openapi-tool check`: pass; 60 paths, 76 operations, 112 schemas, 454
  properties, zero missing operation IDs; pinned hashes unchanged.
- Repository secret scan after consolidation: 70 files, zero findings.
- Deleted-reference and runtime-inventory absence checks: pass.
- Context/link/evidence check and final whitespace check: pass.
- `go test -race ./...`: not run because this workstation has no C compiler;
  Phase 1 CI must run the race suite in its pinned toolchain environment.

## Exit-gate audit

- Required TODOs: complete with linked evidence or explicit safe reductions.
- OpenAPI generation: pinned and reproducible.
- Access-token behavior: verified.
- Core absence and exact identity: deterministic.
- Project/environment/complex flag/segment lifecycles: verified or explicitly
  constrained.
- Member and secret decisions: recorded.
- ADR-001 through ADR-005: Accepted.
- Every available target: complete; unavailable self-hosted row remains honest.
- Cleanup inventory: empty.
- Credential/secret scan: clean.
- Status and handoff: Ready for Phase 1.

## Exact next action

Review and simplify the remaining completed Phase 0 top-level documents and
`tools/` as a separate cleanup task. Preserve the Accepted ADRs, compatibility
conclusions, five consolidated evidence records, and `internal/client/openapi`
inputs. Then start [Phase 1](../plan-execution-phase-1/README.md) at `P1-001`.
