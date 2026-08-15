# Phase 6 — IAM and release

## Purpose

Make IAM the next Provider phase, align it to the customer's actual workflows,
implement only that approved surface, and carry it through the next Provider
release. The existing roadmap names candidate IAM objects and relationships,
but no Provider schema, lifecycle, Import identity, or public API dependency is
frozen yet.

The current branch is limited to IAM plus its release. The unfinished Segment
targeting prerequisite work is Phase 7 on a separate future branch. It is
deferred because the required documented public API is planned for a later
FeatBit version; it is neither a blocker nor an implementation target for this
phase.

## Current entry point

Start with [P6-010](todo.md) after the customer feedback is supplied. Convert
that feedback into explicit supported workflows, managed/read-only/external
boundaries, exact identities, relationship ownership, and measurable
acceptance outcomes. Reconcile the result with the current roadmap instead of
treating the existing IAM outline as already approved.

Do not add IAM endpoint adapters, Terraform schemas, state models, examples, or
tests before this alignment is complete. P6-010 must first determine what the
Provider should own; the documented public API gate follows from that scope.

## Provisional context to revalidate

The current roadmap carries these candidates forward only as input to
requirements alignment:

- exact member lookup for relationship endpoints, without exposing
  initial-password data;
- managed groups and custom policies, with built-in policies read-only;
- independent group-member, group-policy, and direct member-policy edges that
  own one exact pair rather than an entire shared relationship set;
- verified access-token tenant scope and optional context-header behavior; and
- member invitation, creation, profile mutation, and team removal outside
  Terraform ownership.

Customer feedback may narrow, reorder, or replace this candidate surface. Any
change must still fit the documented public API and the Provider's exact-match,
state-preservation, one-shot mutation, cancellation, and redaction contracts.

## Required alignment outputs

P6-010 must leave enough current context to plan implementation without
guessing:

- the customer actors, workflows, desired outcomes, and explicit exclusions;
- the IAM objects and relationships that are managed, observed, or external;
- tenant and organization scope, authentication, and context selection;
- exact lookup keys, stable IDs, relationship direction, and Import identity;
- ownership, drift, replacement, deletion, and out-of-band change behavior;
- secret and identity redaction boundaries;
- the documented public operations needed for every read and mutation; and
- local, Protocol, and trusted current-Cloud acceptance outcomes.

After those outputs are agreed, replace the placeholder-only TODO with the
smallest ordered set of API verification, design, implementation, verification,
documentation, release qualification, and separately maintainer-authorized
publication items needed by the aligned scope. The final item on this branch
must close the IAM release; it must not start Phase 7 implementation.

## Guardrails

- Use documented public APIs only; never depend on Portal-private controllers
  or direct database access.
- Use exact IDs or scoped exact keys across complete result sets; never select
  the first fuzzy match.
- Never store or expose tokens, passwords, tenant/member identities, request
  paths, or unsafe response bodies in code, state, fixtures, logs, or
  diagnostics.
- Preserve existing user changes and shared relationship data. A resource may
  claim only the object or edge explicitly represented by its state.
- Reuse existing transport, escaping, pagination, exact resolution, error
  classification, cancellation, concurrency, and redaction behavior when the
  ownership boundary matches.
- Keep Phase 7 Segment prerequisite implementation out of this phase.
- Do not create a tag, sign or finalize release assets, or publish a release
  without explicit maintainer authorization.

## Exit gate

The detailed exit gate is intentionally not frozen before customer-feedback
alignment. P6-010 must rewrite it as measurable outcomes for the approved IAM
surface. At minimum, no IAM contract may be published until every consumed
operation is documented and exact, tenant scope and Import identities are
stable, shared relationships cannot be accidentally claimed or removed, a
second plan is empty, trusted current-Cloud verification is redaction-safe,
Registry documentation and release artifacts match the implemented surface,
and the approved IAM release is published. Passing this gate ends the current
branch; Phase 7 continues separately.
