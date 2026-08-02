# FeatBit Terraform Provider

## Purpose

This workspace builds the open-source FeatBit Terraform Provider in Go, using Terraform Plugin Framework and Protocol v6.

FeatBit customer workflows and the current public FeatBit API define the provider scope.

## Project context

`.context-wiki/` contains only the current architecture, roadmap, and active
execution context. It is not a development-history archive.

Before working, read:

1. `.context-wiki/plan.md`
2. The active phase's `README.md` and `todo.md`
3. The relevant existing production code and tests. Search the repository for
   compatible or reusable behavior across:
   - API clients, transports, request construction, and serialization
   - schemas, wire/state models, conversions, flattening, and canonicalization
   - identifiers, exact-match lookup, validation, import, and lifecycle/state
     behavior
   - configuration, constants/defaults, errors, diagnostics, redaction, retry,
     cancellation, and concurrency
   - interfaces, test fixtures/helpers, and protocol or contract test patterns

Work one active TODO item at a time. Keep its implementation scope, important
files, runtime call relationship, and completion checks directly under that
item. After material work, record the concise result only under that item.

The active phase README and TODO own execution status, completed-item results,
and the next task. Do not update `.context-wiki/plan.md` merely because a TODO
item completed, a verification passed, or the next-task pointer advanced.
Update the master plan only when work changes a cross-phase, still-current
architecture, product contract, or roadmap fact. Edit or replace the relevant
fact; never append task-completion history.

When a phase passes its exit gate, merge only still-current architecture and
roadmap facts into `.context-wiki/plan.md`, delete the completed phase package,
and create the next phase's `README.md` and `todo.md`. Do not retain ADRs,
evidence files, findings, handoffs, prompts, or session logs unless the user
explicitly asks for a durable historical record.

## Reuse before adding

Every session must inspect the existing implementation before adding or
duplicating a file, helper, schema, wire/state model, conversion, validator,
request path, lifecycle branch, constant/default, error/diagnostic, fixture, or
test helper. Trace the relevant runtime call relationship and search the
repository for behavior that already implements all or part of the required
contract.

Prefer extending or reusing existing code when its semantics and safety
boundary match. In particular, check for reusable request construction and
URL/path/query escaping, serialization and wire mappings, UUID/identity
validation, exact-match resolution, provider-client configuration, schema and
state modeling, canonicalization and flattening, import and lifecycle behavior,
diagnostics/error classification/redaction, retry/cancellation/concurrency,
state preservation, and focused protocol or contract test infrastructure. Do
not create a second implementation merely because the work begins in a new
Codex session or TODO item.

When a new shared helper is justified, place it at the narrowest layer that
owns the common behavior, keep endpoint-specific wire types and lifecycle
policy with their production caller, and add focused coverage for the shared
contract. Reuse must not weaken exact matching, redaction, cancellation,
one-shot mutation, state preservation, or public-API boundaries.

Some duplication is intentional when framework types or ownership contracts
differ, such as resource and data-source schemas. Do not introduce speculative
generic frameworks just to remove superficial repetition; keep intentional
duplication small, explicit, and independently testable.

## Guardrails

- Assume the current public API cannot change for provider v1.
- Use documented public endpoints only; never use Portal-private APIs or direct database access.
- Never store credentials or secret values in code, logs, fixtures, or `.context-wiki/`.
- Use exact IDs or scoped exact keys, never the first fuzzy search result.
- Preserve existing user changes and pass the active phase exit gate before starting the next phase.
