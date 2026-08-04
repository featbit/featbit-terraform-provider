# Phase 4 — Segments

- Status: **In progress**
- Updated: **2026-08-04**
- Next task: `P4-012`

Read [AGENTS.md](../../AGENTS.md), the
[current project plan](../plan.md), then [todo.md](todo.md). No completed phase
package is required.

## Starting point

The Phase 3 exit gate passed. The repository has one locally loadable Protocol
v6 provider with five configuration attributes, a shared handwritten HTTP
client, and registered Project, Environment, and Feature Flag resources plus
their exact single-object data sources. Their reusable contracts include:

- escaped path construction, strict canonical UUID validation, bounded
  responses, cancellation, safe bodyless-GET retry, one-shot mutations, and
  central envelope, error, and redaction handling;
- complete pagination and exact zero/one/duplicate resolution that never treats
  a direct `404` or partial collection as authoritative absence;
- canonical read-after-write state, strict composite Import parsing, stable
  computed IDs across in-place plans, and replacement-aware unknown IDs; and
- cancellation-safe keyed serialization for concrete multi-call lifecycles.

Reuse those contracts only where Segment semantics and ownership match. Add
Segment wire fields, scope/type classification, rule and condition modeling,
set normalization, reference preflight, and lifecycle policy at the narrowest
layer that owns them.

## Objective

Deliver `featbit_segment`, its exact single-object data source, and the public
Import contract `<environment_uuid>/<segment_uuid>`. The resource manages only
environment-specific segments. It must converge through Create, Read,
specialized metadata/targeting/tag Update, Import, drift, replacement,
out-of-band deletion, reference-safe archive-plus-hard-delete, and exact
cleanup. Shared segments remain exact read/bind observations only and are never
created, updated, archived, restored, or deleted by Terraform.

## Public API boundary

Use only the current documented `/api/v1` operations below. P4-010 must first
freeze the exact safe wire fields, type spellings, scope encoding, and current
Cloud behavior needed by production callers. A documented field is not
automatically Terraform-owned.

| Purpose | Documented operation |
|---|---|
| Exact read | `GET /api/v1/envs/{envId}/segments/{segmentId}` |
| Paginated active/archived proof | `GET /api/v1/envs/{envId}/segments` with `Name`, `IsArchived`, `PageIndex`, and `PageSize` |
| Create | `POST /api/v1/envs/{envId}/segments` |
| Name update | `PUT /api/v1/envs/{envId}/segments/{segmentId}/name` |
| Description update | `PUT /api/v1/envs/{envId}/segments/{segmentId}/description` |
| Targeting update | `PUT /api/v1/envs/{envId}/segments/{segmentId}/targeting` |
| Tag update | `PUT /api/v1/envs/{envId}/segments/{segmentId}/tags` |
| Reference preflight | `GET /api/v1/envs/{envId}/segments/{segmentId}/flag-references` |
| Archive prerequisite | `PUT /api/v1/envs/{envId}/segments/{segmentId}/archive` |
| Permanent delete | `DELETE /api/v1/envs/{envId}/segments/{segmentId}` |

Do not call the generic Segment `PATCH`, restore, by-IDs, or all-tags
operations. Do not add Portal-private operations, direct database access, or
organization/workspace context headers. Endpoint files contain only fields
consumed by the production Terraform caller.

The production call relationship is:

```text
Terraform Core
  -> featbit_segment resource or data source
     -> lifecycle-owned Segment client method
        -> Client.Do -> authorizationTransport
        -> Client.DecodeResponse
     -> exact type/scope and targeting canonicalization
     -> canonical Terraform state or a redaction-safe diagnostic
```

## Terraform contracts

### Exact data source

Require `environment_id` and `id` as exact UUIDs. Read one active segment only
and compute its exact key, name, description, type, scopes, included/excluded
users, ordered rules, and tags. It may observe an environment-specific segment
or a shared segment visible in that environment. An archived match is not an
active data-source result and must produce stable recovery guidance. Never use
name filtering, fuzzy matches, or the first collection result as identity.

### Resource

Manage only the `environment-specific` type. Require exact `environment_id`,
immutable `key`, and `name`; expose a Computed UUID; and own the verified
description, included users, excluded users, ordered rules/conditions, and
tags. P4-010 freezes whether the public type/scope fields are safe computed
observations or explicit immutable inputs before P4-011 freezes the schema;
that decision must not make shared Segment mutation possible.

Only the specialized name, description, targeting, and tag operations may
update state in place. Environment, key, and any identity-defining type/scope
input require replacement. Import accepts exactly
`<environment_uuid>/<segment_uuid>` and validates both components before any
request.

## Targeting, identity, and collection invariants

- Treat segment and environment UUIDs as strict canonical identities. Match a
  Create collision by case-sensitive exact key across every page; never adopt
  a pre-existing object automatically.
- Accept only documented Segment type spellings. Classify
  `environment-specific` and `shared` explicitly; an unknown or contradictory
  type/scope response is ambiguous.
- Model included users, excluded users, and tags as set-valued inputs with one
  deterministic canonical order. Preserve exact user keys and tag strings;
  never trim, case-fold, or fuzzy-match them.
- Preserve rule evaluation order. Freeze condition ordering, operator/value
  encoding, UUID ownership, and validation from the public contract before
  exposing the schema; never infer an undocumented expression language.
- Correlate server-owned rule/condition identities exactly when they are part
  of the public response. Reject missing or duplicate identities instead of
  correlating by response index.
- Treat shared scopes as immutable exact observations. Resolve their documented
  environment/project/organization meaning without granting the resource
  ownership of shared scopes.
- Canonicalization belongs in one provider-owned Segment layer reused by
  planning, Create expansion, Read flattening, Import, and the data source.
  Endpoint wire models remain serialization shapes.

## Lifecycle invariants

- A collection proof consumes all pages and reconciles `totalCount`; malformed,
  repeated, incomplete, or inconsistent pages are ambiguous, never exact zero.
- Distinguish active, archived, exact zero, cross-view duplicates, and
  inconsistent UUID/key results. A direct `404`, authorization failure, or
  metadata-only collection item cannot produce canonical managed state.
- Create requires exact zero by key across active and archived views. Execute
  each required mutation once, establish provisional state as soon as a remote
  UUID is authoritative, then read and persist the complete canonical object.
  Reconcile ambiguous outcomes without replaying a mutation or adopting an
  unrelated segment.
- Update sends only the specialized operation for each changed owned component,
  once, in a frozen deterministic order, followed by one exact canonical Read.
  Partial or ambiguous multi-call results preserve recoverable state.
- Refresh of an externally archived managed segment preserves state and
  diagnoses the archive; it never restores the segment or plans a colliding
  Create as though the key were absent.
- Destroy first proves the exact Feature Flag reference set is empty. A
  reference conflict sends no archive/delete mutation and preserves state.
  Otherwise archive when needed, permanently delete, then prove exact zero
  across complete active and archived views.
- Shared segments are never passed to a mutation path. A type/scope mismatch in
  resource state fails closed with recovery guidance.
- Serialize writes only at the exact environment/segment boundary demonstrated
  necessary by the multi-call lifecycle. Cancellation while waiting remains
  safe.
- Never place runtime segment/user identities, keys, rule values, tags, scopes,
  feature-flag references, tokens, tenant details, paths, or raw bodies in
  diagnostics, logs, fixtures, or context files.

## Execution order

1. Freeze the public Segment taxonomy and safe wire shapes; add complete
   pagination and exact active/archived UUID/key status resolution.
2. Freeze canonical targeting/type/scope models, resource ownership/schema,
   and the exact data source including shared read-only observations.
3. Add environment-specific Create, deterministic multi-call initialization,
   replacement planning, ambiguous recovery, and Import.
4. Add specialized metadata, targeting, and tag Update.
5. Add reference-aware archive-plus-hard-delete and exact cleanup.
6. Prove Protocol v6 lifecycle, cross-resource ownership/redaction, trusted
   current-Cloud acceptance, and the complete Phase 4 gate.

## Out of scope

- Creating, updating, archiving, restoring, or deleting shared segments.
- Generic Segment PATCH, restore, by-IDs/all-tags helpers, collection data
  sources, raw REST resources, backend changes, Portal APIs, or direct database
  access.
- Terraform ownership of Feature Flag targeting operations or automatic
  removal of Feature Flag references during Segment destroy.
- Global-user creation, user synchronization, Segment-to-Segment references,
  IAM, permanent CI/release wiring, Registry documentation, and generated API
  clients.
- A speculative generic pagination, targeting expression, or relationship
  framework without another production caller.

## Exit gate

- All items in [todo.md](todo.md) are complete.
- `terraform providers schema -json` preserves the five provider attributes
  and exposes exactly four resources plus four exact data sources.
- The environment-specific resource passes Create, exact Read, specialized
  Update, Import, second-plan idempotence, drift, replacement, out-of-band
  deletion, reference conflict, and archive-plus-hard-delete tests.
- The exact data source safely reads both environment-specific and shared
  segments, while every shared mutation path is structurally unreachable.
- Ordered rules/conditions, set-valued included/excluded users and tags,
  type/scope observations, and server identities converge under arbitrary API
  ordering without weakening evaluation order.
- Destroy refuses exact Feature Flag references, otherwise proves exact zero
  across complete active and archived collections; all test-owned Cloud
  objects are cleaned child-first.
- No credential, user identity, rule value, tag, scope, runtime UUID/key, flag
  reference, or tenant detail is persisted outside intended Terraform state or
  leaked through diagnostics, logs, fixtures, assertions, or repository files.
- Formatting, vet, unit/race, repeated endpoint contracts, Protocol v6 tests,
  build, module/dependency verification, diff checks, local override, schema
  assertions, repository redaction scans, and trusted current-Cloud acceptance
  pass.
- The current plan identifies Phase 5's exact first IAM task.

After the gate passes, fold only still-current architecture and roadmap facts
into [the master plan](../plan.md), delete this Phase 4 directory, and create
only the Phase 5 README/TODO.
