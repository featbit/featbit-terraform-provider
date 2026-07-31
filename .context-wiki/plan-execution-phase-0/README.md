# Phase 0 Execution Package

Phase: **Empirical API compatibility and ADRs**  
Estimated effort: **3–5 person-days**  
Execution status: **Complete**
Master roadmap: [../plan.md](../plan.md)

## Purpose

Phase 0 converts assumptions about the current FeatBit REST API into reproducible observations and explicit design decisions. It does not modify the FeatBit backend. Its output is the compatibility contract that Phase 1 will implement.

The phase answers five questions:

1. How do Cloud and supported self-hosted versions actually authenticate and report success, failure, absence, and conflicts?
2. Can each core Terraform lifecycle be made deterministic with the current API?
3. Which fields can be updated safely, which require replacement, and which must remain unmanaged?
4. Which provider behaviors belong in the API compatibility wrapper?
5. Is there enough verified information to scaffold Phase 1 without inventing server behavior?

## Read order for a new session

1. [../../AGENTS.md](../../AGENTS.md)
2. [../plan.md](../plan.md)
3. [plan.md](plan.md)
4. [context-protocol.md](context-protocol.md)
5. [status.md](status.md)
6. [todo.md](todo.md)
7. [handoff.md](handoff.md)
8. Relevant entries in [findings.md](findings.md), [compatibility-matrix.md](compatibility-matrix.md), [adrs/](adrs/), and [evidence/](evidence/)

## File map

| File or directory | Role |
|---|---|
| [plan.md](plan.md) | Scope, workstreams, sequence, deliverables, and exit gate |
| [todo.md](todo.md) | Task IDs, checkboxes, dependencies, and completion evidence |
| [context-protocol.md](context-protocol.md) | Cross-session read/write and user-interaction contract |
| [status.md](status.md) | Current phase summary and blockers |
| [session-log.md](session-log.md) | Append-only execution history |
| [findings.md](findings.md) | Verified behavior and superseded observations |
| [compatibility-matrix.md](compatibility-matrix.md) | Deployment/version behavior matrix |
| [adrs/](adrs/) | Architecture decision records and template |
| [evidence/](evidence/) | Sanitized, reproducible evidence |
| [handoff.md](handoff.md) | Exact next action and continuation state |
| [phase-0-execution-prompt.md](phase-0-execution-prompt.md) | Copyable prompt to start Phase 0 in a new session |

## Source-of-truth hierarchy

When files disagree, use this order:

1. Latest accepted ADR for the specific decision.
2. Current `status.md` and compatibility matrix backed by evidence.
3. This phase plan and TODO.
4. The master roadmap.
5. Older session-log or finding entries.

Do not resolve a conflict by deleting history. Add a dated correction and link the superseding decision.

## Definition of done

Phase 0 is complete only when:

- Every required TODO is completed, or the related capability has an explicit reduced support level.
- Core API observations are reproducible and sanitized.
- The five required ADRs are accepted.
- The compatibility and capability matrices are complete.
- All test-created resources are deleted or listed for manual cleanup.
- `status.md` says `Ready for Phase 1`.
- `handoff.md` contains no unresolved blocker to starting the Phase 1 scaffold.

Phase 1 planning now lives in
[../plan-execution-phase-1/](../plan-execution-phase-1/README.md). This completed
package remains the decision and evidence archive for that work.
