# Phase 6 TODO — IAM

Work one item at a time. Before adding any helper, wire model, client method,
schema, lifecycle branch, or test fixture, search the existing implementation
for a compatible contract and trace the runtime call relationship. The current
item is requirements work only; do not begin IAM production implementation
until it is complete.

## Requirements alignment

- [ ] **P6-010 — Re-align IAM scope from customer feedback.**

  Begin with the customer IAM feedback that will be supplied after this roadmap
  reorder. Translate each requested workflow into a concrete actor, action,
  scope, target object or relationship, expected lifecycle, and observable
  success outcome. Separate required workflows from optional ideas and record
  explicit non-goals.

  Compare the feedback with the provisional IAM outline in the phase README.
  For every retained workflow, establish the required tenant or organization
  context, exact identities and lookup scope, managed/read-only/external
  ownership, relationship direction, Import expectations, drift and deletion
  behavior, and protected values. Do not infer a Terraform resource shape from
  endpoint names alone.

  Then identify the documented public API operations and authentication
  behavior that must be proven before implementation. Record uncertainty as an
  API or product question; never fill it with a Portal-private endpoint, fuzzy
  lookup, destructive whole-set replacement, or initial-password handling.

  Completion checks:

  - customer workflows, priorities, and exclusions are explicit;
  - the managed/read-only/external boundary is unambiguous for every IAM object
    and relationship;
  - exact tenant scope, identity, ownership, lifecycle, Import, and redaction
    questions are answered or named as blocking evidence gaps;
  - the phase README contains a measurable exit gate for the aligned scope;
  - this TODO is replaced with concrete, ordered follow-up items, with one next
    item active;
  - the ordered work ends with IAM release qualification and separately
    maintainer-authorized publication, without starting Segment implementation;
    and
  - `.context-wiki/plan.md` changes only where the alignment alters a
    still-current cross-phase architecture, product-contract, or roadmap fact.

## Follow-up task shaping

Do not create implementation item numbers before P6-010 completes. The aligned
requirements must determine the smallest sequence for public API verification,
ownership and schema design, endpoint adapters, Provider lifecycle, focused and
Protocol tests, trusted current-Cloud acceptance, documentation, and the phase
gate. The sequence must finish with release qualification and an explicitly
authorized IAM release. Phase 7 Segment work remains outside this branch.
