# Phase 2 — Project and environment

- Status: **In progress**
- Updated: **2026-08-03**
- Next task: `P2-090`

Read [AGENTS.md](../../AGENTS.md), the
[current project plan](../plan.md), then [todo.md](todo.md). No completed phase
package is required.

## Starting point

The Phase 1 exit gate passed. The repository has one Protocol v6 provider, five
configuration attributes, and one shared handwritten `*client.Client` with:

- direct access-token authorization constrained to the configured `/api/v1`
  origin;
- bounded responses, request concurrency, context cancellation, and safe
  bodyless-GET retry;
- central `{success,data,errors}` envelope and error classification; and
- redaction contracts covering credentials, paths, runtime identities, server
  details, diagnostics, and logs.

There are no resources, data sources, endpoint-specific wire models,
exact-existence resolvers, pagination helpers, or per-object write locks. Add
each endpoint adapter only with the first production lifecycle that calls it.

## Objective

Deliver `featbit_project` and `featbit_environment` resources, their exact
single-object data sources, and their public Import contracts. CRUD, refresh,
drift, replacement, Import, out-of-band deletion, and destroy must converge
using only the documented public API.

## Public API boundary

| Object | Documented operations used in this phase |
|---|---|
| Project | `GET/POST /api/v1/projects` and `GET/PUT/DELETE /api/v1/projects/{id}` |
| Environment | `POST /api/v1/projects/{projectId}/envs` and `GET/PUT/DELETE /api/v1/projects/{projectId}/envs/{id}` |
| Environment absence fallback | `GET /api/v1/projects/{projectId}` and exact-ID matching in its environment collection |

Endpoint files contain only the request/response fields used by these
Terraform callers. Do not restore the deleted generated/OpenAPI/probe
architecture, add Portal-private operations, or send organization/workspace
context headers.

The production call relationship is:

```text
Terraform Core
  -> featbit_project or featbit_environment lifecycle
     -> lifecycle-owned Project/Environment client method
        -> Client.Do
           -> authorizationTransport
        -> Client.DecodeResponse
     -> canonical Terraform state or a redaction-safe diagnostic
```

## Terraform contracts

### Project

The resource owns `name` and `key`. `id` is Computed, and `key` uses
`RequiresReplace`. Import accepts exactly `<project_uuid>`.

`environments` is a fully Computed observation of the environments returned
with the project. Canonicalize it by stable key/ID ordering and expose only
safe fields needed to identify the automatic environments: `id`, `name`,
`key`, and `description`. It does not grant nested ownership and must not
conflict with standalone `featbit_environment` resources.

The data source requires one exact project UUID and computes the same canonical
project fields. It does not support name or fuzzy lookup.

### Environment

The resource owns `project_id`, `name`, `key`, and `description`. `id` is
Computed; `project_id` and `key` use `RequiresReplace`. A missing description
canonicalizes to the empty string. Import accepts exactly
`<project_uuid>/<environment_uuid>`.

The public Update payload also carries environment settings. Terraform does
not own those settings in Phase 2: read them immediately before Update, pass
their current value through unchanged, and serialize that read/write/read
sequence per environment. Do not copy secret values into Terraform models,
state, diagnostics, logs, or fixtures. No environment-secrets data source is
part of this phase.

The data source requires one project UUID and one exact environment UUID and
computes `name`, `key`, and `description`. It does not support name, key, or
fuzzy lookup.

## Lifecycle invariants

- Project Create requires exact zero for its key across the complete project
  collection. Environment Create requires exact zero for its key within the
  exact parent project.
- Direct Read failure never proves absence. Project uses the complete project
  collection by exact ID; Environment uses the exact parent project's
  environment collection by exact ID.
- Exact zero means absent, exact one means present, and duplicates or an
  incomplete collection are ambiguous. Ambiguity preserves resource state.
- A mutation executes once. After an ambiguous Create, reconcile by the scoped
  exact key, never retry or silently adopt; return stable Import/recovery
  guidance if exactly one object exists.
- Read after every successful logical write and persist the canonical server
  form. An ambiguous Delete may remove state only after exact zero is proven.
- Project operations need no per-object lock. Add only the narrow environment
  lock required to preserve UI-owned settings across its multi-call Update.
- Validate Import component count and UUID syntax before any request. Escape
  every path component and never select the first fuzzy result.
- Cancellation, authentication, authorization, transient failure, and
  malformed responses preserve state and remain redaction-safe.

## Execution order

1. Project read adapter and exact data source.
2. Project resource, Import, and protocol lifecycle.
3. Environment read adapter and exact data source.
4. Environment resource, settings-preserving Update, Import, and protocol
   lifecycle.
5. Cross-resource ownership/redaction checks, trusted Cloud acceptance, and
   the complete Phase 2 gate.

## Out of scope

- Feature flags, segments, IAM, workspace settings, environment-secret values,
  and collection data sources.
- A generic pagination/existence framework without a Phase 2 caller.
- Archive/restore behavior, raw REST resources, backend changes, Portal APIs,
  direct database access, and Registry release work.

## Exit gate

- All items in [todo.md](todo.md) are complete.
- `terraform providers schema -json` exposes exactly the two Phase 2 resources
  and two exact data sources while preserving the five provider attributes.
- Project and Environment CRUD, Read-after-write, Import, second-plan
  idempotence, drift, replacement, out-of-band deletion, exact fallback, and
  cleanup pass their focused mock/protocol and trusted Cloud checks.
- Project automatic environments remain Computed observations; a standalone
  environment does not create a competing nested owner.
- Environment Update preserves UI-owned settings, and no environment secret
  value reaches ordinary state or any diagnostic/log/fixture.
- `gofmt`, vet, unit/race tests, repeated endpoint contracts, acceptance,
  build, module/dependency verification, diff checks, and the repository
  secret/redaction gate pass.
- The current plan identifies Phase 3's exact first Feature Flag task.

After the gate passes, fold only still-current architecture and roadmap facts
into [the master plan](../plan.md), delete this Phase 2 directory, and create
only the Phase 3 README/TODO.
