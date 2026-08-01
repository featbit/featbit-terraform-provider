# Phase 1 — Provider foundation

- Status: **In progress**
- Updated: **2026-08-02**
- Next task: `P1-041`

Read [AGENTS.md](../../AGENTS.md), the
[current project plan](../plan.md), then [todo.md](todo.md). No completed phase
context is required.

## Current implementation

The provider already has:

- one Go module, MPL-2.0, Protocol v6 entry point, version injection, and
  Registry manifest;
- provider attributes for API URL, Sensitive access token, timeout,
  concurrency, and safe-read retry count, including environment fallbacks;
- `Configure` construction of one shared `*client.Client` for resources and
  data sources;
- direct `Authorization` transport restricted to the configured `/api/v1`
  origin, plus provider User-Agent;
- bounded responses, timeout/cancellation, error/envelope classification,
  bodyless-GET retry, request concurrency, and redaction; and
- provider configuration tests plus complete shared-client request and error
  contracts through a synthetic HTTP server.

There are intentionally no resources, data sources, endpoint-specific API
models, pagination helpers, existence resolvers, or per-object write locks yet.
Those belong to the first production lifecycle that consumes them.

## Remaining scope

Phase 1 now finishes only:

1. the remaining shared-client boundary, retry, concurrency, and redaction
   tests;
2. dependency and race verification;
3. developer commands and fork-safe CI;
4. local provider override and schema loading; and
5. the Phase 1 exit gate and Phase 2 handoff through the master plan.

## Exit gate

- All items in [todo.md](todo.md) are complete.
- `gofmt`, vet, unit/race tests, build, and module verification pass.
- The dependency graph has no generated API or generator dependency.
- Tokens, secrets, and runtime identities do not appear in diagnostics, logs,
  fixtures, or repository scans.
- A local override loads the provider and
  `terraform providers schema -json` succeeds.
- The current plan identifies Phase 2's exact first task.

After the gate passes, fold the final current state into
[the master plan](../plan.md), delete this Phase 1 directory, and create only
the Phase 2 README/TODO.
