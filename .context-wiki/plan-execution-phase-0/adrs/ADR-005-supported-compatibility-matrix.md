# ADR-005: Supported FeatBit, Go, Framework, and Terraform matrix

- Status: Accepted
- Date: 2026-07-30
- Owners: FeatBit Terraform Provider maintainers
- Related TODOs: `P0-003`, `P0-084`, `P0-090`
- Supersedes: None
- Superseded by: None

## Context

Toolchain versions can be verified from primary release sources, but provider
support also requires successful target-specific behavior on FeatBit Cloud and
a selected minimum self-hosted release. Neither authenticated target is
currently configured.

## Decision drivers

- Current secure Go patch lines
- Terraform Plugin Protocol v6 and current Framework
- Reproducible exact dependencies
- Honest target-specific compatibility claims
- A clear minimum self-hosted version

## Evidence

- [Compatibility matrix](../compatibility-matrix.md)
- [Cloud authentication](../evidence/20260731-cloud-auth.md)
- [Cloud project/environment behavior](../evidence/20260731-cloud-project-environment.md)
- [Cloud four-type feature flags](../evidence/20260731-cloud-feature-flags.md)
- [Cloud environment-specific segments](../evidence/20260731-cloud-segments.md)
- [Offline toolchain and compatibility contracts](../evidence/20260731-offline-contracts.md)

## Options considered

### Option A — Claim Cloud/current self-hosted from specification alone

Rejected. A specification is not lifecycle compatibility evidence.

### Option B — Pin toolchains now; accept deployment rows only after probes

Accepted. This preserves implementation reproducibility without making a false
server claim.

### Option C — Support every historical self-hosted version

Rejected. Unbounded versions cannot be tested or supported safely.

## Decision

| Surface | Proposed support |
|---|---|
| Go module baseline | `1.25.8` |
| Go CI | `1.25.12`, `1.26.5` |
| Release Go | `1.26.5` |
| Plugin Framework | `v1.19.0` |
| Plugin Testing | `v1.16.0` |
| Plugin Go | `v0.31.0` |
| Plugin Log | `v0.10.0` |
| Plugin Docs | `v0.25.0` |
| Generator | `oapi-codegen v2.8.0` |
| Plugin protocol | `6`; Registry manifest `["6.0"]` |
| Terraform CLI | minimum `1.0.0`; test current `1.15.8` |
| FeatBit Cloud | Project/environment, constrained Boolean/String/Number/JSON flag rows, and one constrained environment-specific segment row target-verified; provider support not claimed until the complete Phase 0 gate passes |
| FeatBit self-hosted | Intended v1 deployment under the same pinned public-API contract; no exact release is target-certified in Phase 0, and the unavailable row remains `Not tested` |

The local Phase 0 machine's Go `1.19.4` and Terraform `1.5.6` are diagnostic
inputs only, not the proposed build matrix.

## Consequences

### Positive

- Phase 1 has exact dependency inputs.
- Untested deployment versions are not mislabeled supported.

### Negative

- A release-specific self-hosted compatibility claim still requires an exact
  version and target-specific testing.
- The initial matrix cannot publish a certified minimum self-hosted release.

### Follow-up

- Complete the remaining approved Cloud probes.
- Retain both Cloud and self-hosted in the intended v1 scope under one pinned
  public-API contract; add release-specific self-hosted certification when an
  approved disposable target becomes available.
- Run Terraform minimum/current and Go CI matrices in an environment with the
  required toolchains and race dependencies.

## 2026-07-31 evidence audit

The earlier statement that no authenticated Cloud target was configured is
superseded for the project/environment rows: direct access-token transport and
three clean Cloud lifecycles now exist. Missing flag/segment reads and
exact-zero fallbacks are also verified. Feature-flag and segment lifecycle
rows remain `Not tested`; their offline probes do not upgrade the deployment
matrix.

No exact minimum self-hosted release or disposable target has been selected.
ADR-005 therefore remains `Proposed`. Claiming either complete Cloud support or
a self-hosted minimum from the public specification would violate the
target-specific evidence rule.

## 2026-07-31 Boolean flag supplement

One Cloud Boolean flag CRUD row is now target-verified, including the
archive-before-hard-Delete constraint and active-plus-archived exact absence.
Duplicate, stale-revision, other variation types, and complex flag operational
fields remain untested. The Cloud matrix and ADR therefore remain incomplete.

## 2026-07-31 environment-segment supplement

One Cloud environment-specific segment verified resource-name scope
resolution, complex specialized updates, archive/restore,
archive-before-hard-Delete, direct post-delete 404, active-plus-archived exact
absence, and cleanup. Shared segments remain read/bind only and make no
managed-lifecycle compatibility claim.

This upgrades only the `cloud-current` environment-specific segment cell. No
exact self-hosted release or disposable target is available, so ADR-005
remains `Proposed`.

## 2026-07-31 self-hosted availability correction

Only the online Cloud environment is available. The product owner cannot
provision `selfhosted-min` for Phase 0, so no URL, token, target, or prefix
environment variables are requested. ADR-005 must explicitly choose one
honest v1 claim:

1. Cloud-only support, with self-hosted omitted from the supported matrix; or
2. Cloud verified while self-hosted remains unclaimed, which leaves the Phase
   0 exit gate blocked under the current acceptance criteria.

No self-hosted compatibility may be inferred from Cloud or OpenAPI evidence.

## 2026-07-31 product-scope correction

The product owner confirmed that FeatBit customers use both SaaS and
self-hosted deployments, so provider v1 is intended to serve both. The
Cloud-only alternative above is rejected and the earlier binary choice is
superseded.

This product requirement does not turn untested behavior into evidence. The
self-hosted row remains `Not tested` and no exact self-hosted release is
certified.

## 2026-07-31 unavailable-target exit-gate correction

The Phase 0 plan explicitly says that when only one deployment target is
available, the other row is marked unverified, and the exit gate requires a
complete matrix for every *available* target. Therefore, absence of a
self-hosted target is not an observed incompatibility and does not by itself
block Phase 0. The earlier stricter blocker statement is superseded.

ADR-005 remains `Proposed` because the available Cloud matrix still has
unresolved rows, not because self-hosted failed. Its support policy is
deployment-neutral: v1 targets FeatBit's pinned public REST API on both Cloud
and self-hosted deployments, while the evidence table separately identifies
which exact targets/releases were empirically verified.

## 2026-07-31 four-type flag scope

The product owner requires Boolean, String, Number, and JSON flags in v1 while
keeping targeting, rules, rollouts, enabled state, and tags outside Terraform
ownership. All four use one pinned public Create/Read contract and deterministic
canonicalization; type, description, and variations replace.

The Boolean Cloud row remains the only target-certified type. A reusable
String/Number/JSON probe passes offline with one owned parent scope and exact
cleanup, but it has not received a new three-child Cloud mutation approval.
The matrix therefore records the v1 contract without falsely upgrading those
three Cloud rows.

## 2026-07-31 Cloud four-type flag supplement

The preceding Boolean-only target-certification statement is superseded. A
separately approved contained run completed String, Number, and JSON
Create/Read/name-Update/canonicalization and archive-plus-hard-Delete under one
new disposable project/environment. Direct post-delete Reads returned 404,
active and archived exact counts were zero, and cleanup ended at `pending=0`.

All four flag types are therefore target-verified on `cloud-current` under the
accepted constrained ownership contract. No targeting, rules, rollouts,
enabled state, tags, or post-Create variations were written.

## 2026-07-31 available-target completion and acceptance

The current Cloud deployment is the only available Phase 0 target. Its core-v1
matrix is complete: direct access-token transport, project, environment,
Boolean/String/Number/JSON feature flags, one complex environment-specific
segment, exact absence, canonicalization, environment secret metadata, and
cleanup are target-verified. Every remaining uncertain behavior has an explicit
boundary rather than an implementation assumption:

- child duplicate status codes are not relied upon because exact-zero
  preflight and post-failure exact reconciliation fail closed;
- stale-revision operational writes, flag targeting/rollout/enabled updates,
  restore/key-reuse guarantees, and shared-segment mutation are omitted or
  read/bind only;
- referenced-segment destroy stops before DELETE when the documented reference
  preflight is non-empty;
- member creation is external, while member lookup and IAM are deferred from
  core v1 until later target-specific evidence exists; and
- inactive/restricted token body details are not relied upon; generic 401/403
  classes preserve state and do not retry mutations.

No disposable self-hosted target or exact release exists for Phase 0. That row
remains `Not tested`, as required by the plan, and no release-specific
certification is claimed. Provider v1 is deployment-neutral: it exposes a
configurable API URL and targets the pinned documented public REST contract for
both Cloud and self-hosted installations. Future release certification must
rerun the same matrix on an approved disposable self-hosted target.

This completes the matrix for every available target without inferring an
unavailable target. ADR-005 is `Accepted`.

## Acceptance criteria

- [x] Toolchain versions are supported by primary release evidence.
- [x] Cloud authenticated lifecycle matrix is complete, including explicit
  constrained/external/omitted classifications for untested behavior.
- [x] Unavailable self-hosted target is explicitly `Not tested`; no minimum
  release or target-specific compatibility is claimed.
- [x] Rejected alternatives and tradeoffs are recorded.
- [x] ADR status is `Accepted`.
