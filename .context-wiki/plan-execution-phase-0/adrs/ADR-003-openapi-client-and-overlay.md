# ADR-003: Pinned OpenAPI client and narrow local overlay

- Status: Accepted
- Date: 2026-07-30
- Owners: FeatBit Terraform Provider maintainers
- Related TODOs: `P0-010` through `P0-018`, `P0-082`
- Supersedes: None
- Superseded by: None

## Context

The public OpenAPI document has useful transport/schema shapes but lacks
operation IDs, required fields, enums, and documented error responses beyond
200/401/403. Provider builds must be reproducible without requiring a backend
specification change.

## Decision drivers

- Reproducible generation inputs and output
- Reviewable provider-owned compatibility changes
- No invented server behavior
- Typed transport plus handwritten Terraform semantics
- Secret-safe diagnostics and exact identity fallbacks

## Evidence

- [Consolidated OpenAPI, toolchain, and transport contracts](../evidence/20260731-offline-contracts.md)

## Options considered

### Option A — Handwrite every request/response type

Rejected. It duplicates a large public surface and makes upstream drift harder
to audit.

### Option B — Generate a typed transport, wrap it by hand

Accepted. Pin exact upstream bytes, add only stable operation IDs through an
overlay, generate a typed client, and put Terraform/error/pagination/retry
semantics in a handwritten wrapper.

### Option C — Generate Terraform schemas directly from OpenAPI

Rejected. The document lacks validation metadata, and Terraform null/unknown,
ownership, set/list, Sensitive, Import, and replacement semantics require
deliberate design.

## Decision

- Exact upstream bytes live at
  `internal/client/openapi/featbit.openapi.json`, locked by URL, byte length,
  and SHA-256 in `openapi.lock.json`.
- `overlay.json` follows OpenAPI Overlay 1.1.0 and adds one unique
  `operationId` to each existing operation. It cannot add paths, fields,
  constraints, or behavior.
- Repository-owned Go tooling pins/downloads, verifies, applies, inventories,
  and stale-checks the snapshot/overlay deterministically.
- Phase 1 pins
  `github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0` in its
  tools module and generates client/models from
  `featbit.overlayed.openapi.json` using `oapi-codegen.yaml`.
- Generated code is committed and never edited by hand.
- A handwritten wrapper owns base URL, direct access-token auth, user agent,
  timeout/cancellation, response envelope, redaction, centralized error
  classification, exact paginated lookup, safe-read retry/backoff, per-object
  write serialization, cleanup, and canonical post-write verification.
- Terraform schemas, validators, expand/flatten, ownership, Import parsing, and
  state migrations are handwritten.
- Any future upstream change updates the lock/overlay in a reviewed change and
  must pass deterministic and breaking-change checks.

## Consequences

### Positive

- Builds are reproducible and upstream drift is explicit.
- Stable Go method names do not depend on upstream operation IDs.
- Terraform-specific safety remains reviewable.

### Negative

- Generated and wrapper layers both require maintenance.
- The local overlay applicator intentionally supports only the narrow targeted
  operation-ID subset needed here.

### Follow-up

- Phase 1 creates the root provider/tools modules with the accepted pins.
- Add generated-output clean-diff and OpenAPI breaking-change CI checks.

## Acceptance criteria

- [x] The decision is supported by linked evidence.
- [x] Phase 1 implementation responsibilities are explicit.
- [x] Rejected alternatives and tradeoffs are recorded.
- [x] Documentation and test implications are identified.
- [x] ADR status is `Accepted`.
