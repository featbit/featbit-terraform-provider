# Phase 0 Handoff

Handoff state: **Ready to begin Phase 0**  
Prepared: **2026-07-30**

## Start here

Use [phase-0-execution-prompt.md](phase-0-execution-prompt.md) in the new session.

Read:

1. [../../AGENTS.md](../../AGENTS.md)
2. [../plan.md](../plan.md)
3. [README.md](README.md)
4. [plan.md](plan.md)
5. [context-protocol.md](context-protocol.md)
6. [status.md](status.md)
7. [todo.md](todo.md)

## Exact next task

Execute:

1. `P0-001` — establish the session read baseline.
2. `P0-002` — record repository state.
3. `P0-003` through `P0-008` — establish safe test targets, resource prefix, credential handling, cleanup, and redaction.
4. Continue to the OpenAPI/probe tasks only after the mutation guardrails exist.

## Prerequisites to confirm

- Availability of a disposable FeatBit Cloud test tenant.
- Availability and exact version of a disposable self-hosted FeatBit instance.
- `FEATBIT_TEST_SERVICE_TOKEN` present in the session environment.
- Optional `FEATBIT_TEST_PERSONAL_TOKEN`.
- Permission to create/delete prefixed test resources.

Do not record credential values or private tenant identifiers in context files.

## Current verified state

- Only the planning package has been created.
- No Phase 0 API behavior has been empirically verified yet.
- No remote resource needs cleanup.
- Phase 1 must not begin yet.

## Phase 1 readiness

Not ready. When Phase 0 passes, replace this section with:

- Exit-gate result
- Accepted ADR links
- Supported capability and version matrix
- Pinned OpenAPI/overlay paths and hashes
- Phase 1 client behavior inputs
- Known reduced-scope decisions
- Link to [phase-1-execution-prompt.md](phase-1-execution-prompt.md)
