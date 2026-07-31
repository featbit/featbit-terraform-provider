# Phase 1 — Repository and Provider Scaffold

State: **Ready to start; implementation has not begun**
Prepared: **2026-08-01**

Phase 1 turns the accepted Phase 0 API contract into a loadable Terraform
Plugin Framework provider and a tested API client foundation. It does not
implement Project, Environment, Feature Flag, Segment, or IAM resources.

## Read first

1. [../../AGENTS.md](../../AGENTS.md)
2. [../plan.md](../plan.md)
3. [Phase 0 status](../plan-execution-phase-0/status.md)
4. [Phase 0 handoff](../plan-execution-phase-0/handoff.md)
5. [Phase 0 accepted ADRs](../plan-execution-phase-0/adrs/README.md)
6. [plan.md](plan.md)
7. [todo.md](todo.md)

Phase 0 findings and evidence are consulted only through links from the
handoff or an ADR; they are not part of the normal Phase 1 reading path.

## Files

| File | Purpose |
|---|---|
| [README.md](README.md) | Current phase state, scope, and reading order |
| [plan.md](plan.md) | Accepted inputs, implementation workstreams, constraints, and exit gate |
| [todo.md](todo.md) | Executable checklist and evidence required for completion |

This compact three-file package intentionally replaces the earlier proposal
for separate status, findings, session-log, evidence, and handoff files. Update
the state above and the relevant TODO notes as work progresses; add another
file only when it has a clear reader and cannot fit here.

## Definition of done

- A local developer override loads the Protocol v6 provider.
- `terraform providers schema -json` succeeds.
- Provider configuration and mock API client tests pass.
- OpenAPI generation is deterministic.
- Tokens and secret values cannot appear in logs or diagnostics.
- [todo.md](todo.md) is complete and identifies the exact Phase 2 entry action.

## Next action

After the separately requested Phase 0/`tools` cleanup, inspect the dirty
worktree and preserve the retained Phase 0 decisions. Then execute `P1-001`
onward from [todo.md](todo.md). Do not implement a production Terraform
resource during this phase.
