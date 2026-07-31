# Consolidated offline contracts

- Target: `offline-contract`
- Observed: 2026-07-30 through 2026-07-31
- FeatBit specification: pinned public OpenAPI `info.version` `1.0`
- Deployment claim: none; these are specification, mock, and unit contracts

This record consolidates eleven former offline evidence files covering
OpenAPI, toolchains, safety, transport, cleanup, normalization, resource
guardrails, and IAM feasibility. Executable Go tests and pinned lock files are
the authoritative reproducible artifacts.

## Pinned OpenAPI and toolchains

- Snapshot SHA-256:
  `8DE202F939F6721748D66449C3DFE4EEE2E2BF369A57F121DF808907A44D11C4`.
- Applied operation-ID overlay SHA-256:
  `C4E7165D13A5D12FB1990BC66572FB0D330D792843F398408900623BCE92BF60`.
- Inventory: OpenAPI 3.0.4, API 1.0, 60 paths, 76 operations, 112 schemas,
  454 direct properties. All 76 upstream operations lacked `operationId`; the
  overlay adds only unique operation IDs and invents no endpoint or behavior.
- Inputs and locks live in
  [internal/client/openapi](../../../internal/client/openapi/README.md).
- Accepted pins: Go baseline `1.25.8`, CI `1.25.12`/`1.26.5`, release
  `1.26.5`; Framework `v1.19.0`; Plugin Testing `v1.16.0`; Plugin Go
  `v0.31.0`; Plugin Log `v0.10.0`; Plugin Docs `v0.25.0`;
  `oapi-codegen v2.8.0`; Protocol 6; Terraform minimum `1.0.0` and current
  test line `1.15.8`.

## Probe safety contract

- Mutations require explicit `--execute`, an approved target/URL class, a safe
  randomized prefix, and an empty cleanup inventory.
- The probe accepts no existing remote ID for mutation. It requires exact-zero
  preflight and registers each returned identity before later operations.
- Ambiguous mutations are never retried. Reconciliation reads all pages and
  accepts only exact identity; fuzzy-first, zero, and duplicate results fail
  closed.
- Reports include method/path templates, statuses, codes, counts, and Boolean
  comparisons only. Headers, tokens, secrets, tenant values, real emails,
  runtime IDs/keys, and raw bodies are excluded.

## Envelope, error, retry, and absence contract

| Signal | Classification and state behavior | Retry |
|---|---|---|
| 2xx + `success=true` | success; canonical Read | no |
| 2xx + `success=false` | application failure; preserve | no |
| 400/422 | validation; preserve and diagnose | no |
| 401 | authentication; preserve | no |
| 403 | authorization; preserve | no |
| 404 | unconfirmed absence; preserve until exact fallback | no |
| 409 | conflict; preserve and refresh | no automatic write retry |
| 429/5xx/timeout/network | preserve | bounded safe GET only |
| cancellation | preserve | no |
| complete exact fallback: zero/one/multiple | absent/present/ambiguous | no |

Cleanup sends a mutation at most once, then independently proves desired
absence. Project uses the complete project collection; Environment uses its
exact parent; Feature Flag and Segment traverse every active/archived page.
A 200 Delete with a remaining exact identity stays pending. An ambiguous
Delete with proven exact zero may converge. Failed/incomplete verification or
duplicate identity remains pending. An exactly absent owned parent may prove
child cascade; an existing parent plus failed child lookup cannot.

## Canonicalization and identity

- Boolean values canonicalize to lowercase; String is byte-exact; Number avoids
  `float64`; JSON normalizes key order/whitespace while preserving decimal
  precision; condition string lists use validated JSON-string encoding.
- Variations, rules, conditions, and rollout ranges are ordered lists. Tags,
  target keys/groups, segment included/excluded users, and scopes are
  sorted/deduplicated sets. Segment scopes also replace.
- Stable variation UUIDs map enabled/disabled/fallthrough identity; list index
  is never identity.
- Import formats: project/group/policy/member `<uuid>`; environment and segment
  `<parent_uuid>/<uuid>`; flag `<environment_uuid>/<exact_key>`; IAM bindings
  `<left_uuid>/<right_uuid>`.
- Partial creation is checkpointed immediately, exact-read after ambiguity,
  and either cleaned safely or returned with an exact Import diagnostic.

## Resource lifecycle guardrails

- Project/environment: create-returned identity only, exact parent checks,
  repeated canonical Reads, no key mutation, dependency-ordered cleanup.
- Flag: owned parents only, all-page active/archived key checks, name-only
  narrow Update, stable variation identity, UI-owned operational fields
  excluded, archive/hard-Delete exact absence.
- Segment: owned environment only, exact key/UUID, specialized field Updates,
  reference preflight, archive/hard-Delete exact absence. Shared segment
  mutation is excluded.
- Mock suites cover fuzzy-first pagination, zero/one/duplicate matches,
  ambiguous creation, controlled failure, unexpected duplicates, redaction,
  and recovery.

## Member and IAM boundary

`GET /api/v1/members` is paginated and search is not identity; offline tests
cover zero, one, duplicate, and fuzzy-first normalized exact-email matching.
The live member endpoint was unavailable. `POST /api/v1/members/add` returns a
Boolean rather than a member ID and invitation/human-acceptance timing is
unspecified. Member creation is therefore external, while lookup and IAM are
deferred from core v1 pending later target evidence.

Group-member, group-policy, and member-policy relationships expose both stable
UUIDs, add/remove operations, and parent-scoped lists. They use independent
composite identities and never claim an entire relationship collection.

## Verification

The Phase 0 probe module passed formatting, full unit tests, `go vet`, repeated
normalization/lifecycle/recovery suites, deterministic OpenAPI checks, context
checks, redaction tests, and repository secret scans. The local race suite was
blocked only by the Windows workstation's missing C compiler and is assigned
to Phase 1 CI.

Reproduce without live credentials:

```text
cd tools/api-probe
go test ./...
go vet ./...
go run ./cmd/openapi-tool check
go run ./cmd/secret-scan --root ../..
```

Cleanup was not applicable to mock/specification tests. The remote inventory
was independently verified at `pending=0`.
