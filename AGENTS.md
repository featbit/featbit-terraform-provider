# FeatBit Terraform Provider

## Purpose

This workspace builds the open-source FeatBit Terraform Provider in Go, using Terraform Plugin Framework and Protocol v6.

FeatBit customer workflows and the current public FeatBit API define the provider scope.

## Project context

`.context-wiki/` is the traceable development history and source of project context.

Before working, read:

1. `.context-wiki/plan.md`
2. The active phase's `README.md`, `status.md`, `todo.md`, and `handoff.md`
3. Any linked findings, ADRs, and evidence

After material work, update the active phase's TODO, status, session log, and handoff. Do not delete historical findings; supersede them with a dated correction. Follow the active phase's `context-protocol.md`.

## Guardrails

- Assume the current public API cannot change for provider v1.
- Use documented public endpoints only; never use Portal-private APIs or direct database access.
- Never store credentials or secret values in code, logs, fixtures, or `.context-wiki/`.
- Use exact IDs or scoped exact keys, never the first fuzzy search result.
- Preserve existing user changes and pass the active phase exit gate before starting the next phase.
