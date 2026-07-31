# ADR-004: Public Import identity formats

- Status: Accepted
- Date: 2026-07-30
- Owners: FeatBit Terraform Provider maintainers
- Related TODOs: `P0-033` through `P0-036`, `P0-058`, `P0-074`, `P0-083`
- Supersedes: None
- Superseded by: None

## Context

Import IDs become permanent user-facing compatibility contracts. The provider
must carry enough parent scope to read an object exactly and must never accept
names or fuzzy search results as identity.

## Decision drivers

- Exact deterministic identity
- Parent scope included where the public endpoint requires it
- Simple parsing and useful diagnostics
- Stable binding ownership
- No private tenant identifiers or secret material

## Evidence

- [Consolidated lifecycle, Import, recovery, and IAM identity contracts](../evidence/20260731-offline-contracts.md)

Live absence behavior remains target-specific and must be implemented through
the conservative exact-fallback classifier; it does not change the public
syntax chosen here.

Post-acceptance verification (2026-07-31): Cloud project/environment present
and absent cases confirmed that these identities map to documented exact
collection fallbacks even when direct post-delete Reads return 403 or 500. See
[the compatibility evidence](../evidence/20260731-cloud-project-environment.md).

## Options considered

### Option A — Remote UUID only for every object

Rejected for environment-scoped reads because the public endpoint also needs a
parent project/environment.

### Option B — Scoped slash-separated exact identities

Accepted. Use UUIDs and the exact flag key already required by public endpoint
paths.

### Option C — Names, email, or fuzzy keys

Rejected. They are mutable, potentially non-unique, sensitive, or search
inputs rather than stable identities.

## Decision

| Terraform type | Import ID |
|---|---|
| `featbit_project` | `<project_uuid>` |
| `featbit_environment` | `<project_uuid>/<environment_uuid>` |
| `featbit_feature_flag` | `<environment_uuid>/<exact_flag_key>` |
| `featbit_segment` | `<environment_uuid>/<segment_uuid>` |
| `featbit_group` | `<group_uuid>` |
| `featbit_policy` | `<policy_uuid>` |
| `featbit_group_member` | `<group_uuid>/<member_uuid>` |
| `featbit_group_policy` | `<group_uuid>/<policy_uuid>` |
| `featbit_member_policy` | `<member_uuid>/<policy_uuid>` |

- The parser requires exactly the documented component count, UUID syntax for
  ID components, and the exact public flag-key character set.
- It rejects whitespace, extra components, names, email, fuzzy values, and
  partial IDs.
- Member creation is external, so no managed `featbit_member` Import contract
  is published in v1. A future member resource requires a new ADR.
- Data sources and the singleton workspace have no Import contract.
- Import immediately performs the exact scoped read/fallback and writes only
  canonical server state. Ambiguity preserves state and returns a diagnostic.

## Consequences

### Positive

- Every import maps directly to documented endpoint identity.
- Parent scope is available without search.
- Binding IDs are independent and deterministic.

### Negative

- Users must supply UUIDs rather than friendly names.
- Slash-separated formats cannot be changed compatibly after release.

### Follow-up

- Phase 1 reuses the tested parser contract.
- Resource acceptance tests must verify an empty first plan after Import.

## Acceptance criteria

- [x] The decision is supported by public identities and parser evidence.
- [x] Phase 1 implementation responsibilities are explicit.
- [x] Rejected alternatives and tradeoffs are recorded.
- [x] Documentation and test implications are identified.
- [x] ADR status is `Accepted`.
