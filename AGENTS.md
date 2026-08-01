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

Work one active TODO item at a time. Keep its implementation scope, important
files, runtime call relationship, and completion checks directly under that
item. After material work, record the concise result there and synchronize any
changed current-architecture fact in the master plan.

When a phase passes its exit gate, merge only still-current architecture and
roadmap facts into `.context-wiki/plan.md`, delete the completed phase package,
and create the next phase's `README.md` and `todo.md`. Do not retain ADRs,
evidence files, findings, handoffs, prompts, or session logs unless the user
explicitly asks for a durable historical record.

## Guardrails

- Assume the current public API cannot change for provider v1.
- Use documented public endpoints only; never use Portal-private APIs or direct database access.
- Never store credentials or secret values in code, logs, fixtures, or `.context-wiki/`.
- Use exact IDs or scoped exact keys, never the first fuzzy search result.
- Preserve existing user changes and pass the active phase exit gate before starting the next phase.
