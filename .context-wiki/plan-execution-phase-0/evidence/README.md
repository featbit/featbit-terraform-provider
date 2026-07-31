# Phase 0 Evidence Index

Phase 0 evidence is intentionally consolidated. Read an evidence file only
when an ADR, finding, or compatibility row needs its underlying observation.

| Evidence | What it proves |
|---|---|
| [Cloud authentication](20260731-cloud-auth.md) | direct access-token transport; missing/malformed 401; no required context headers |
| [Cloud project/environment](20260731-cloud-project-environment.md) | lifecycle, defaults, validation, duplicates, secrets metadata, child missing reads, and exact absence |
| [Cloud feature flags](20260731-cloud-feature-flags.md) | Boolean/String/Number/JSON lifecycle, canonicalization, ownership boundary, archive/hard-Delete, and cleanup |
| [Cloud segments](20260731-cloud-segments.md) | environment-specific segment scope, complex updates, reference preflight, archive/hard-Delete, and shared-scope reduction |
| [Offline contracts](20260731-offline-contracts.md) | pinned OpenAPI/toolchains, probe safety, normalization, errors/retry, exact identity, recovery, Import, and IAM boundary |

The four Cloud files contain sanitized target observations. The offline file is
not deployment evidence; executable Go tests and lock files remain the
reproducible source for those contracts.

## Consolidation record

On 2026-08-01, 19 narrowly scoped Phase 0 records were merged into the five
records above at the product owner's request. No conclusion was removed:

- two authentication records became **Cloud authentication**;
- three parent/absence records became **Cloud project/environment**;
- two flag records became **Cloud feature flags**;
- one live segment record became **Cloud segments**; and
- eleven specification/mock/toolchain records became **Offline contracts**.

All ADR, finding, TODO, status, handoff, compatibility, and session-log links
were redirected. The old files were then deleted rather than kept as unread
duplicates.

## Redaction and cleanup

Evidence must never contain tokens, authorization values, passwords,
environment secret values, private tenant identifiers, runtime IDs/keys, or
real member emails. Only public path templates and sanitized shapes/counts are
retained.

Every Phase 0-created remote object is exactly absent. Final cleanup state:
`pending=0`; no manual owner/action.
