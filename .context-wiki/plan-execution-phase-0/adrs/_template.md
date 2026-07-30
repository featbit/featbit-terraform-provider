# ADR-NNN: Title

- Status: Proposed
- Date: YYYY-MM-DD
- Owners: FeatBit Terraform Provider maintainers
- Related TODOs: `P0-...`
- Supersedes: None
- Superseded by: None

## Context

Describe the customer workflow, current FeatBit API constraints, Terraform lifecycle requirement, and why a durable decision is needed.

## Decision drivers

- Deterministic plan/apply/read/import behavior
- No silent overwrite of unrelated UI-managed configuration
- Compatibility with FeatBit Cloud and supported self-hosted releases
- Stable public schema and Import contract
- Secure treatment of credentials and state
- Minimal complexity consistent with the customer outcome

## Evidence

- Evidence record: `../evidence/YYYYMMDD-<target>-<topic>.md`
- [Findings index](../findings.md)
- [Compatibility result](../compatibility-matrix.md)

Do not accept the ADR while required evidence is missing. Explicitly distinguish documented claims from live verification.

## Options considered

### Option A — Name

Description, benefits, limitations, and failure behavior.

### Option B — Name

Description, benefits, limitations, and failure behavior.

### Option C — Name

Description, benefits, limitations, and failure behavior.

## Decision

State the selected behavior precisely enough to implement and test. Include:

- Public Terraform shape
- Ownership and lifecycle
- Identity and Import implications
- Error and recovery behavior
- Compatibility constraints

## Consequences

### Positive

- ...

### Negative

- ...

### Follow-up

- ...

## Acceptance criteria

- [ ] The decision is supported by linked evidence.
- [ ] Phase 1 implementation responsibilities are explicit.
- [ ] Rejected alternatives and tradeoffs are recorded.
- [ ] Documentation and test implications are identified.
- [ ] The ADR status is updated to `Accepted`, `Rejected`, or remains `Proposed` with a blocker.
