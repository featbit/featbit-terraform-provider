# Phase 3 — Feature flags

- Status: **In progress**
- Updated: **2026-08-03**
- Next task: `P3-012`

Read [AGENTS.md](../../AGENTS.md), the
[current project plan](../plan.md), then [todo.md](todo.md). No completed phase
package is required.

## Starting point

The Phase 2 exit gate passed. The repository has one locally loadable
Protocol v6 provider with five configuration attributes, a shared handwritten
HTTP client, and registered Project/Environment resources plus their exact
single-object data sources. Their reusable contracts include:

- escaped path construction, UUID validation, bounded responses, cancellation,
  safe bodyless-GET retry, one-shot mutation behavior, and central envelope,
  error, and redaction handling;
- exact zero/one/duplicate resolution that never treats a direct `404` as
  authoritative absence;
- canonical read-after-write state, strict Import parsing, and stable computed
  IDs that remain known only for in-place plans; and
- a narrow keyed lock pattern for a lifecycle that must preserve UI-owned
  values across a multi-call write.

Reuse those contracts where their semantics match. Add Feature Flag wire types,
pagination, normalization, variation correlation, and lifecycle policy only at
the narrow layer that first needs them.

## Objective

Deliver `featbit_feature_flag`, its exact single-object data source, and the
public Import contract `<environment_uuid>/<exact_key>`. Boolean, String,
Number, and JSON flags must converge through Create, Read, name-only Update,
replacement, Import, drift, out-of-band deletion, and archive-plus-hard-delete
without rewriting UI-owned operational settings.

## Public API boundary

Use only the current documented `/api/v1` operations below. Collection reads
must request every page needed for exact resolution and must examine active and
archived views separately.

| Purpose | Documented operation |
|---|---|
| Paginated exact fallback | `GET /api/v1/envs/{envId}/feature-flags` with `IsArchived`, `PageIndex`, and `PageSize` |
| Create | `POST /api/v1/envs/{envId}/feature-flags` |
| Exact read | `GET /api/v1/envs/{envId}/feature-flags/{key}` |
| Name-only update | `PUT /api/v1/envs/{envId}/feature-flags/{key}/name` |
| Archive prerequisite | `PUT /api/v1/envs/{envId}/feature-flags/{key}/archive` |
| Permanent delete | `DELETE /api/v1/envs/{envId}/feature-flags/{key}` |

Do not call the generic Feature Flag `PATCH` endpoint or the restore, toggle,
description, variations, targeting, tags, clone, or pending-change operations.
Do not add Portal-private operations or organization/workspace context headers.
Endpoint files contain only fields consumed by the production Terraform
caller.

The production call relationship is:

```text
Terraform Core
  -> featbit_feature_flag resource or data source
     -> lifecycle-owned Feature Flag client method
        -> Client.Do -> authorizationTransport
        -> Client.DecodeResponse
     -> type-aware canonicalization and UUID correlation
     -> canonical Terraform state or a redaction-safe diagnostic
```

## Terraform contracts

### Exact data source

Require `environment_id` as an exact UUID and `key` as an exact Feature Flag
key. Compute `id`, `name`, `description`, `variation_type`, and canonical
`variations`. Do not offer name, fuzzy, or collection lookup, and do not expose
enabled state, tags, targeting, rules, rollouts, or other operational fields.
An archived exact match is not an active data-source result and must produce
stable recovery guidance rather than silently restoring it.

### Resource

The resource owns:

- Required `environment_id`, `key`, `name`, `variation_type`, and
  `variations`;
- Optional `description`, canonicalized to `""`; and
- Computed Feature Flag `id` and Computed variation IDs.

`environment_id`, `key`, `variation_type`, `description`, and `variations`
require replacement. Only `name` updates in place through the specialized name
endpoint. Import accepts exactly `<environment_uuid>/<exact_key>` and validates
both components before any request.

For provider-created flags, variation IDs are deterministic valid UUIDs before
the one Create request. Imported flags retain their server IDs. Correlate and
flatten variations by exact UUID, never by response index; reject missing or
duplicate IDs and make API reordering converge. The schema exposes variation
name/value inputs but does not turn enabled/disabled selections or fallthrough
behavior into ongoing Terraform ownership.

Create must initialize only the API-required operational fields with one
documented, deterministic, disabled-safe seed. P3-011 freezes that seed and
proves it for all four types before any Create implementation. Subsequent Read
and Update omit UI-owned enabled state, disabled/fallthrough selection, tags,
targeting, rules, and rollouts so Portal changes survive Terraform plans.

## Type and value invariants

- Accept exactly the API types `boolean`, `string`, `number`, and `json` and
  store their canonical lowercase spelling.
- Boolean values canonicalize to lowercase `true` or `false`; other spellings
  are invalid.
- String values are exact user data and receive no trimming or JSON coercion.
- Number values are validated and canonicalized without `float64`, so large
  integers, decimals, and exponents do not lose precision.
- JSON values must contain exactly one valid JSON value and canonicalize
  whitespace and object-key ordering without losing numeric precision.
- Every flag has a non-empty variation collection; every variation has a
  non-empty name/value and exactly one valid, unique UUID after expansion.
- Key validation follows the public contract: non-empty, at most 128
  characters, and only ASCII letters, digits, `.`, `_`, and `-`. Name is
  non-empty and at most 128 characters.

Canonicalization belongs in one provider-owned type-aware layer reused by
resource planning, Create expansion, Read flattening, Import, and the data
source. Endpoint wire models remain serialization shapes, not Terraform state
models.

## Lifecycle invariants

- Resolve identity by exact environment UUID plus case-sensitive exact key.
  Validate and escape each path component; never select the first fuzzy match.
- A collection proof must consume all pages and reconcile `totalCount`; a
  malformed, incomplete, repeated, or inconsistent page is ambiguous, never
  exact zero.
- Distinguish active, archived, exact zero, and duplicate/inconsistent results
  across both collection views. Direct `404`, authorization failure, or a
  partial collection never proves absence.
- Create requires exact zero in active and archived views. Execute the mutation
  once, read the exact object, and persist its canonical server form. After an
  ambiguous Create, reconcile by exact key without retrying or auto-adopting.
- Update sends only the name request once, then performs an exact canonical
  Read. It never sends a generic full-object update.
- If refresh finds one archived object for managed state, preserve state and
  diagnose the out-of-band archive; do not restore it or plan a colliding
  Create as though it were absent.
- Destroy archives an active object, permanently deletes the archived object,
  then proves exact zero across every active and archived page. If the object
  is already archived, skip the redundant archive and continue safely.
- Reconcile every ambiguous archive/delete step by exact active/archived state.
  Remove Terraform state only after exact zero; otherwise preserve it.
- Add a per-flag write lock only if the concrete multi-call lifecycle needs it
  to make those invariants true. Cancellation while waiting must remain safe.
- Never place runtime keys, UUIDs, values, tags, targeting data, raw bodies,
  tokens, or tenant/server details in diagnostics, logs, fixtures, or context
  files.

## Execution order

1. Public Feature Flag read/list adapter, complete pagination, and exact
   active/archived resolver.
2. Type-aware canonicalization, stable variation identity, schemas, and the
   exact data source.
3. Resource Create, replacement planning, ambiguous recovery, and Import.
4. Specialized name Update and archive-plus-hard-delete lifecycle.
5. Four-type Protocol v6 lifecycle, ownership/redaction integration, trusted
   current-Cloud acceptance, and the complete Phase 3 gate.

## Out of scope

- Terraform ownership of enabled state, disabled/fallthrough selection, tags,
  targeting, rules, rollouts, pending changes, or approval workflows.
- Restore, clone, generic PATCH, collection data sources, raw REST resources,
  backend changes, Portal APIs, and direct database access.
- Segments, IAM, permanent CI/release wiring, Registry documentation, generated
  clients, and speculative generic pagination or value frameworks without a
  Feature Flag caller.

## Exit gate

- All items in [todo.md](todo.md) are complete.
- `terraform providers schema -json` preserves the five provider attributes
  and exposes exactly three resources plus three exact data sources.
- All four variation types pass CRUD-equivalent lifecycle, exact Read, Import,
  second-plan idempotence, drift, replacement, out-of-band deletion, and exact
  cleanup through focused mock/Protocol and trusted current-Cloud checks.
- Arbitrary-precision Number and JSON canonicalization converge, stable UUID
  correlation survives API reordering, and replacement does not leave an
  archived key collision.
- Portal changes to enabled state, fallthrough/disabled selection, tags,
  targeting, rules, and rollouts survive Read, plan, name Update, and Import.
- Destroy proves exact zero across complete active and archived collections,
  and no credential, variation value, runtime identity, or tenant detail is
  persisted or leaked.
- Formatting, vet, unit/race, repeated endpoint contracts, Protocol v6 tests,
  build, module/dependency verification, diff checks, local override, schema
  assertions, and repository redaction scans pass.
- The current plan identifies Phase 4's exact first Segment task.

After the gate passes, fold only still-current architecture and roadmap facts
into [the master plan](../plan.md), delete this Phase 3 directory, and create
only the Phase 4 README/TODO.
