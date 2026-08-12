# Phase 6 — Segment targeting prerequisites

## Purpose

Close the Environment-specific Segment prerequisite gap before any IAM work
begins. A Segment can already persist included/excluded user keys and rule
properties, but the Provider does not create missing Environment End Users or
register custom End User Property metadata. Release `v0.1.1` documents that
boundary without changing runtime behavior.

This phase starts with the public API, not with Provider code. The Provider may
depend only on stable operations documented by FeatBit's official Swagger or
OpenAPI contract and usable with the existing access-token authentication. UI
calls, Portal-private controllers, and direct database access are evidence of
product behavior only; they are not Provider contracts.

## Current entry point

Start with [P6-010](todo.md): establish whether the public API supports all of
these operations with exact Environment scope and redaction-safe failures:

- exact lookup of an End User by Environment and key;
- idempotent create-missing-only End User registration;
- exact lookup of End User Property metadata by Environment and property;
- idempotent create-missing-only custom-property registration.

If any required contract is absent, stop runtime implementation. Record exact
source evidence and the smallest upstream public API addition needed. Do not
call an undocumented endpoint and do not modify the FeatBit backend in this
workspace task.

## Intended Terraform contract

The ownership design is not frozen until P6-010 proves the public API. The
preferred behavior, if the API can support it safely, is prerequisite ensure
inside `featbit_segment` Create and Update:

```text
Terraform Segment plan
  -> canonicalize targeting users and rule properties
  -> exact Environment prerequisite lookup
  -> register only values proven missing
  -> update Segment targeting
  -> read canonical Segment state
```

The lifecycle must never overwrite an existing user's name or custom
properties. It must not treat built-ins such as `keyId` and `name` as custom
properties. Removing targeting or destroying a Segment must never delete End
Users or property metadata because those records may be shared by Flags,
Segments, SDK traffic, and applications.

If implicit ensure cannot provide truthful Import, refresh, drift, concurrency,
and partial-failure behavior, P6-020 must evaluate first-class End User and End
User Property resources. Such a design is acceptable only with explicit,
non-destructive ownership and migration semantics; it must not turn shared
records into destroy-owned Terraform children accidentally.

## Guardrails

- Work in the Provider repository. The FeatBit main repository is read-only
  reference unless a separate user request explicitly authorizes upstream work.
- Use documented public APIs only. Never depend on Portal-private APIs.
- Do not log or include in diagnostics a token, Environment ID, user key,
  property name, or targeting value.
- Use exact Environment identities and exact keys; never accept a fuzzy or
  first-result lookup.
- Register only prerequisites proven missing. Preserve all existing user names,
  custom properties, and metadata.
- Deduplicate deterministically and define included/excluded conflicts before
  sending mutations.
- Complete prerequisites before Segment targeting. A failed prerequisite must
  not be reported as a fully successful apply.
- Read, Import, and refresh behavior must not hide drift or introduce
  surprising mutations.
- Preserve the current Segment HCL and state shape unless an evidence-backed
  compatibility change is unavoidable.
- Do not begin Phase 7 IAM until this phase's exit gate passes.

## Exit gate

If the documented public API is insufficient, record the precise minimum
upstream requirement and leave this phase stopped at that dependency; IAM must
not begin while the Segment mission remains incomplete.

The phase passes only when the public contract is sufficient and the
implemented Terraform behavior proves all of the following locally, through
Protocol tests, and in a trusted current-Cloud acceptance run:

- fresh included and excluded keys become End Users in the exact Environment;
- existing users remain byte-for-byte semantically unchanged;
- custom rule properties are registered once and reused;
- built-in properties cause no property registration;
- duplicate inputs and concurrent applies remain idempotent;
- prerequisite failure prevents false-success targeting state;
- a second plan is empty;
- targeting removal and Segment destroy preserve every End User and property;
- Import, refresh, and prerequisite drift follow the frozen ownership model;
- diagnostics and logs expose none of the protected identifiers or values.

Only after this gate passes should the still-current architecture be folded
into the master plan, this Phase 6 package be removed, and a Phase 7 IAM
README/TODO be created.
