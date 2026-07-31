# Cloud environment-specific segment evidence

- Target: `cloud-current`
- Observed: 2026-07-31
- Build: current Cloud deployment; exact build unavailable
- Scope: one segment created only inside probe-owned disposable parents

This record replaces `20260731-cloud-current-environment-segment-lifecycle.md`.

## Scope discovery

The first Create with an empty `scopes` array returned 400 with code
`scopes_is_invalid`; this was a request-shape correction, not a missing
capability. Exact public Reads of visible segments produced one unique
organization resource-name prefix in memory. The probe never emitted or
persisted that tenant value, then created one environment-specific segment
with the exact environment resource-name scope.

A fresh environment exposed three pre-existing segment rows. They were read
only for scope discovery and never adopted or mutated. Shared segments remain
read/bind only because cross-scope ownership is not contained.

## Verified lifecycle

The owned environment-specific segment completed:

- exact-zero key/UUID preflight and Create;
- direct exact Read and all-page exact identity verification;
- specialized name, description, targeting, and tags Updates;
- repeated complex canonical Reads;
- archive and restore, with unrelated fields preserved;
- documented flag-reference preflight returning an empty array;
- archive-before-hard-Delete; and
- direct 404 plus exact active/archived UUID and key counts of zero.

Included and excluded users and tags use set semantics. Rules and conditions
preserve list order; condition `list(string)` values round-trip through the
API's JSON-string encoding. Key, type, and scopes were never resent by an
Update and remain replace-only.

Direct Delete while active returned 422 with code
`CannotDeleteUnArchivedSegment`. Destroy therefore preflights references,
archives, hard-deletes, then proves direct and two-view exact absence.

## Referenced-delete boundary

A non-empty live reference conflict was deliberately not created. Provider v1
does not depend on its server error shape: a non-empty documented
`flag-references` result stops before DELETE and preserves state. The user must
remove exact references through the UI. A racing or ambiguous Delete also
preserves state and is never retried blindly.

## Cleanup and redaction

The segment, explicit environment, project, and automatic environments were
deleted. Controlled failure paths also converged; cleanup ended at
`pending=0`. No token, secret, runtime identity, scope prefix, tenant value,
response body, or member email is retained.

## Reproduce

```text
cd tools/api-probe
go run ./cmd/featbit-api-probe project-env-lifecycle --execute --segment-crud-checks
go run ./cmd/featbit-api-probe cleanup --dry-run
```
