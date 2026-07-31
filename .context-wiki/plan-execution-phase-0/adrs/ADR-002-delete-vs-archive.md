# ADR-002: Terraform destroy versus archive

- Status: Accepted
- Date: 2026-07-30
- Owners: FeatBit Terraform Provider maintainers
- Related TODOs: `P0-032`, `P0-045`, `P0-050`, `P0-060`, `P0-064`, `P0-081`
- Supersedes: None
- Superseded by: None

## Context

The public API exposes both DELETE and archive/restore operations for feature
flags and segments. Terraform destroy conventionally means deletion, while an
archive-on-destroy option can reserve keys and make later recreation/import
surprising. Project/environment cascade, flag/segment archive prerequisites,
and zero-reference segment deletion are now verified on current Cloud.

## Decision drivers

- Terraform destroy semantics should be unsurprising
- No hidden key reservation or restore requirement
- Missing/already-deleted objects should converge safely
- Reference conflicts must not damage state
- Current public API only

## Evidence

- [Offline operation, error, reference, and exact-absence contracts](../evidence/20260731-offline-contracts.md)
- [Cloud project/environment Delete and exact-absence behavior](../evidence/20260731-cloud-project-environment.md)
- [Cloud boolean flag archive-before-Delete and exact two-view absence](../evidence/20260731-cloud-feature-flags.md)
- [Cloud environment-specific segment archive/restore, archive-before-Delete, and exact two-view absence](../evidence/20260731-cloud-segments.md)
- Live flag restore/key-reuse, non-empty segment-reference conflict, and
  segment key-reuse evidence is unavailable and deliberately excluded from the
  accepted v1 behavior

## Options considered

### Option A — Hard delete on destroy, with required archive prerequisite

Accepted. Resource Delete converges on actual absence. For Cloud flags and
environment-specific segments, the documented archive operation is a required
transient prerequisite before the documented DELETE operation. Archive remains
neither the final destroy state nor a substitute for hard Delete.

### Option B — Archive on destroy by default

Rejected. State removal would hide a still-existing object and reserved key.

### Option C — Provider-level archive-on-destroy switch

Rejected for v1. Provider-wide behavior is easy to misapply and complicates
Import/recreation. It can be reconsidered only with demonstrated customer need.

## Decision

- Project/environment destroy calls the documented DELETE endpoint.
- Feature-flag destroy first calls the documented archive endpoint, requires
  success, then calls documented DELETE. It removes state only after all-page
  exact-key scans of both active and archived views return zero.
- Managed environment-specific segment destroy first reads the documented
  flag-reference collection, archives the exact UUID, requires success, then
  calls documented DELETE. It removes state only after direct absence and
  all-page exact UUID/key zero in both active and archived views.
- No `archive_*_on_destroy` provider option in v1.
- Flag/segment archive state and restore controls are omitted from v1. Archive
  is an internal transient destroy prerequisite only.
- A direct 404 never removes state by itself. Exact scoped absence confirmation
  is required; deleting an exactly confirmed already-absent object succeeds.
- Segment Delete first reads the documented flag-reference collection. A
  non-empty result skips Delete, preserves state, and identifies the required
  external reference-removal step without serializing flag identities.
- A reference conflict or failed reference preflight preserves state and
  returns a diagnostic.
- Shared segments have no v1 managed destroy because they are read/bind only.
- No code assumes that archived or deleted keys can be reused. v1 makes no
  key-reuse guarantee and never retries a rejected recreation blindly.

## Consequences

### Positive

- Destroy matches Terraform expectations.
- Archive cannot masquerade as deletion.
- Recovery/import behavior stays explicit.

### Negative

- Retention/archive-only workflows and automatic restore are not available in
  v1.

### Follow-up

- Re-run the accepted sequence on every newly supported target.
- Add restore, key-reuse, or archive-only behavior only through a superseding
  ADR with live evidence.
- Keep a non-empty segment-reference contract test; a future disposable live
  reference probe may refine diagnostics without broadening v1 destroy.

## 2026-07-31 evidence audit

Cloud project/environment hard Delete and exact absence are verified. The
offline flag and environment-specific segment probes prove safe confirmation
and cleanup composition, and shared segments no longer have managed destroy.
The segment flag-reference preflight is an explicit safe workaround.

Flag/segment deployment behavior for hard Delete, archive/restore, key reuse,
and reference conflicts is still absent. Those managed capabilities have not
been omitted. ADR-002 therefore remains `Proposed`; the offline mock responses
cannot establish remote destroy semantics.

## 2026-07-31 Cloud flag correction

The one-flag Cloud run falsified the earlier direct-hard-Delete assumption.
Deleting an unarchived flag returned HTTP 422 and code
`CannotDeleteUnarchivedFeatureFlag`; the exact flag remained readable and
present.

The documented archive endpoint followed by hard Delete converged to zero
exact keys in both active and archived collection views. Archive alone cannot
count as Terraform destroy because it hides the object from the default list
while retaining it and its key. This correction revises the flag portion of
the proposed decision without changing the rejection of archive-on-destroy as
the final state.

Flag restore/key reuse and every live segment destroy/archive/reference row
remain unverified, so ADR-002 remains `Proposed`.

## 2026-07-31 Cloud segment correction and acceptance

One Cloud environment-specific segment completed archive and restore. After it
was restored, hard Delete returned HTTP 422 and
`CannotDeleteUnArchivedSegment`; the exact segment remained present. The
documented archive operation followed by hard Delete produced direct
404/`ResourceNotFound` and zero exact UUID/key matches in both active and
archived views. The same workaround also completed recovery cleanup.

The live flag-reference preflight returned an empty array. v1 does not attempt
Delete when the preflight is non-empty, so an unobserved server-side conflict
cannot damage state. Restore and key reuse are not required to decide destroy:
v1 omits archive/restore controls and makes no key-reuse guarantee.

With those reductions, all accepted destroy paths use documented endpoints,
converge only on verified exact absence, and preserve state on ambiguity.
ADR-002 is therefore `Accepted`.

## Acceptance criteria

- [x] Required live destroy evidence exists for managed Cloud resources;
  restore/key-reuse/reference-conflict uncertainty is removed from v1 through
  explicit omission, no-guarantee, and fail-closed preflight decisions.
- [x] Implementation responsibilities are explicit.
- [x] Rejected alternatives and tradeoffs are recorded.
- [x] Documentation/test implications are identified.
- [x] ADR status is `Accepted`.
