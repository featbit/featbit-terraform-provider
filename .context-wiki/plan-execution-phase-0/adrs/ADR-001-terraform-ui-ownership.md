# ADR-001: Terraform and UI ownership of flags and segments

- Status: Accepted
- Date: 2026-07-30
- Owners: FeatBit Terraform Provider maintainers
- Related TODOs: `P0-050` through `P0-059`, `P0-060` through `P0-065`, `P0-080`
- Supersedes: None
- Superseded by: None

## Context

FeatBit feature flags are environment-scoped objects containing metadata,
variations, enabled state, targeting, and tags. Splitting that one remote
object into multiple Terraform resources would create competing writers.
Conversely, making every field authoritative would let a later apply overwrite
an operational UI rollout that the user did not intend Terraform to own.

The public API exposes specialized update operations and revision fields.
Cloud Boolean name-update isolation/revision advancement and Cloud
environment-specific segment specialized-update isolation are verified.
Unverified flag operational fields are reduced out of v1 ownership.

## Decision drivers

- Deterministic plan/apply/read/import behavior
- No silent overwrite of unrelated UI-managed configuration
- One writer per remote object/attribute
- Safe behavior when the user omits operational configuration
- Current public API only; no backend dependency

## Evidence

- [Offline lifecycle, normalization, and recovery contracts](../evidence/20260731-offline-contracts.md)
- [Cloud four-type feature-flag lifecycle](../evidence/20260731-cloud-feature-flags.md)
- [Cloud environment-specific segment lifecycle](../evidence/20260731-cloud-segments.md)
- [Findings index](../findings.md)
- The Cloud probe verifies specialized name-update isolation and revision
  advancement for one Boolean flag; other specialized ownership and
  stale-revision rows remain blocked
- The live segment probe verifies specialized-update isolation, complex
  canonical convergence, and replace-only identity preservation; shared
  segments are reduced to read/bind only

## Options considered

### Option A — Separate flag and targeting resources

Rejected. Both resources would mutate the same remote flag and would not have
independent read/delete identities.

### Option B — One resource with reduced v1 operational ownership

Accepted. The resource owns only fields with verified v1 behavior.
Unverified flag operational fields are Computed-only or omitted rather than
using ambiguous Optional+Computed ownership. Specialized endpoints are called
only for changed owned fields.

### Option C — One fully authoritative resource

Rejected as the default. It is simple but can silently revert UI rollouts.
Users may still choose full authority by configuring every field.

## Decision

- Publish one `featbit_feature_flag` resource per environment/key; never publish
  a second resource that mutates its targeting.
- For v1 flags, Terraform owns the exact key/type/description/variation
  contract at Create for Boolean, String, Number, and JSON. Key, type,
  description, and variation changes are
  `RequiresReplace`; only name has a verified in-place update.
- Flag enabled state, targeting, rules, rollouts, and tags are Computed-only or
  omitted in v1. Terraform observes but never corrects those UI-operational
  fields. No `ignore_changes` convention substitutes for provider ownership.
- Flag revision is Computed. v1 does not submit unverified stale-revision or
  operational writes.
- Read after every logical write and compare a canonical representation.
- Environment-specific segment metadata/targeting uses one resource.
  Key/type/scopes are replace-only; included/excluded users and tags are sets,
  while rules preserve order.
- The environment-specific segment scope resolver may use only all-page exact
  public Reads, must require one unique organization resource-name prefix, and
  must never store or emit that tenant value.
- Shared segments are read/bind only in v1 because exact scope encoding and
  cross-scope mutation containment are unverified. No Terraform resource may
  create, update, or delete them without superseding evidence.

## Consequences

### Positive

- No competing Terraform resources write one remote flag/segment.
- Omitted operational configuration cannot be silently overwritten.
- Specialized updates narrow concurrent-write impact.

### Negative

- Initial flag functionality is intentionally narrow; operational fields ship
  read-only or omitted.
- Changes to description or variations replace the flag rather than using
  unverified specialized writes.
- Concurrent edits to the same owned field remain last-writer-wins.

### Follow-up

- Re-run the accepted ownership rows on each newly supported target.
- Expand flag operational ownership only through a superseding ADR with live
  narrow-update, drift, revision, and plan evidence.
- Keep shared segments read/bind only unless a superseding ADR verifies
  cross-scope ownership and containment.

## 2026-07-31 evidence audit

The flag and environment-specific segment probes now prove fail-closed
ownership and narrow-update comparison against mocks. Shared segments are
definitively reduced to read/bind only. This is useful design evidence, but it
does not prove how a deployment preserves flag/segment fields after the
specialized writes or how concurrent UI drift and revisions behave.

The core flag and environment-specific segment capabilities have not been
reduced to Computed-only or omitted, so the alternative acceptance path is
also unmet. ADR-001 therefore remains `Proposed`; accepting it would pass live
ownership assumptions into Phase 1.

## 2026-07-31 Cloud flag supplement

One Cloud Boolean flag verified that a specialized name update advances the
revision while preserving every unrelated canonical field, including empty
targeting/rule collections, variations, enabled state, fallthrough, tags, and
description. This completes the narrow-update evidence for that endpoint.

It does not establish ownership behavior for variations, enabled state,
targets, rules, rollouts, or tags, and it does not simulate concurrent UI drift
or a stale-revision conflict. ADR-001 therefore remains `Proposed`.

## 2026-07-31 Cloud segment supplement and acceptance

One Cloud environment-specific segment completed name, description,
targeting, tags, archive, and restore specialized updates. Exact Reads after
each operation proved unrelated canonical fields were preserved; a final
complex Read repeated without logical diff. Key, type, and the exact
environment scope remained unchanged.

The unresolved flag operational rows do not need to pass into Phase 1 as
assumptions. v1 definitively reduces enabled state, targeting, rules, rollouts,
and tags to Computed-only or omitted; description and variation changes
replace the basic Boolean flag; stale-revision writes are not sent. Shared
segments remain read/bind only.

This narrower ownership contract has one writer per managed field, never
overwrites omitted UI-operational flag configuration, and uses only verified
in-place updates. ADR-001 is therefore `Accepted`.

## 2026-07-31 four-type flag correction

The product owner requires Boolean, String, Number, and JSON flags in v1 while
leaving enabled state, targeting, rules, rollouts, and tags UI-owned. This
supersedes the Boolean-only scope in the accepted supplement; it does not
expand operational ownership.

All four types use the same documented Create/Read variation model, with values
transported as strings. The provider validates and canonicalizes each type:
Boolean lowercase, String byte-exact, Number without `float64`, and JSON with
stable object ordering and exact decimal handling. Type, description, and
variations remain `RequiresReplace`; no variation-update endpoint is added to
the owned path.

Only Boolean has target-specific Cloud evidence. String, Number, and JSON use
the explicitly labelled public-specification plus offline-canonicalization
workaround until the contained three-child live probe is separately approved.
This distinction appears in the compatibility matrix and does not upgrade
those target rows to verified.

Because the correction adds only Create/Read validation and replacement—not an
unverified in-place writer—the one-writer and UI-coexistence decision remains
`Accepted`.

## 2026-07-31 Cloud four-type confirmation

The preceding target-unverified paragraph is superseded. A separately approved
Cloud run completed String, Number, and JSON exact Create/Read, name-only
Update, canonical empty-diff comparison, archive-plus-hard-Delete, and exact
absence. Together with the earlier Boolean lifecycle, all four v1 types are
target-verified on `cloud-current`.

No operational ownership changed: targeting, rules, rollouts, enabled state,
and tags were not written. Type, description, and variations remain
`RequiresReplace`, so ADR-001 remains `Accepted` without adding an unverified
writer.

## Acceptance criteria

- [x] Live Boolean round-trip/name-update evidence and public/offline four-type
  Create/Read canonicalization evidence exist; every unverified type/variation
  change replaces, and unverified operational behavior is removed from v1
  ownership.
- [x] Phase 1/3 implementation responsibilities are explicit.
- [x] Rejected alternatives and tradeoffs are recorded.
- [x] Documentation and test implications are identified.
- [x] ADR status is `Accepted`.
