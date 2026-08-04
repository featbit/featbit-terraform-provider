# Phase 5 — IAM

- Status: **In progress**
- Updated: **2026-08-04**
- Next task: `P5-010`

Read [AGENTS.md](../../AGENTS.md), the
[current project plan](../plan.md), then [todo.md](todo.md). No completed phase
package is required.

## Starting point

The Phase 4 exit gate passed. The repository has one locally loadable Protocol
v6 provider with five configuration attributes, a shared handwritten HTTP
client, and four registered core resources plus their exact single-object data
sources. Project, Environment, Feature Flag, and environment-specific Segment
lifecycles already provide:

- escaped path construction, strict UUID validation, bounded responses,
  cancellation, bodyless-GET retry, one-shot mutations, central envelope/error
  handling, and runtime-value redaction;
- complete pagination and exact zero/one/duplicate resolution that never treats
  a direct `404`, fuzzy filter result, or partial collection as authoritative;
- canonical read-after-write state, strict Import parsing, replacement-aware
  stable IDs, ambiguous-mutation reconciliation, and cancellation-safe keyed
  serialization where a concrete lifecycle needs it; and
- Protocol v6, cross-resource ownership, trusted current-Cloud cleanup, local
  override, schema JSON, race, dependency, license, vulnerability, and
  repository-secret verification.

Reuse those contracts only when IAM tenant scope, identity, collection, and
relationship ownership match. Do not weaken the transport boundary or expose
member, policy-resource, or tenant values merely to share an existing helper.

## Objective

Deliver exact read-only member lookup, managed custom groups and policies, and
three independent relationship resources:

- `featbit_group_member`;
- `featbit_group_policy`; and
- `featbit_member_policy` for direct member policies only.

Also provide exact single-object data sources for members, groups, and
policies. Every relationship resource owns one pair and must coexist with
relationships created by the UI, other Terraform states, or other automation.
Member invitation, creation, profile changes, password handling, and removal
from the team remain external.

## Public IAM boundary to freeze

The current official [IAM documentation](https://docs.featbit.co/iam/overview)
describes members, groups, cumulative policies, and direct or inherited
assignments. The current public
[OpenAPI document](https://app-api.featbit.co/swagger/OpenApi/swagger.json)
advertises the candidate operations below. P5-010 and P5-011 must freeze their
exact safe request/response shapes before a Terraform schema or mutation caller
depends on them.

| Purpose | Candidate documented operation |
|---|---|
| Member exact/list read | `GET /api/v1/members/{memberId}` and `GET /api/v1/members` with `SearchText`, `PageIndex`, and `PageSize` |
| Member group/direct-policy reads | `GET /api/v1/members/{memberId}/groups` and `GET /api/v1/members/{memberId}/direct-policies` |
| Group exact/list read | `GET /api/v1/groups/{id}` and `GET /api/v1/groups` with `Name`, `PageIndex`, and `PageSize` |
| Group lifecycle | `POST /api/v1/groups`, `PUT /api/v1/groups/{id}`, and `DELETE /api/v1/groups/{id}` |
| Group member edge | `GET /api/v1/groups/{groupId}/members` plus `PUT .../add-member/{memberId}` and `PUT .../remove-member/{memberId}` |
| Group policy edge | `GET /api/v1/groups/{groupId}/policies` plus `PUT .../add-policy/{policyId}` and `PUT .../remove-policy/{policyId}` |
| Policy exact/list read | `GET /api/v1/policies/{id}` and `GET /api/v1/policies` with `Name`, `PageIndex`, and `PageSize` |
| Custom policy lifecycle | `POST /api/v1/policies`, `PUT /api/v1/policies/{policyId}/settings`, `PUT /api/v1/policies/{policyId}/statements`, and `DELETE /api/v1/policies/{policyId}` |
| Direct member policy edge | `GET /api/v1/members/{memberId}/direct-policies` plus `PUT .../add-policy/{policyId}` and `PUT .../remove-policy/{policyId}` |

The list and relationship endpoints also expose exact-cased `GetAllMembers`,
`GetAllGroups`, or `GetAllPolicies` switches where applicable. Do not infer
their containment semantics; focused contracts must freeze which value proves
one edge and which value merely supplies candidates.

Do not call member-add, invitation, organization/workspace removal, policy
clone, built-in-policy mutation, Portal-private, or direct database operations.
Do not expose a generic IAM/raw-REST resource.

## Tenant scope and authentication boundary

The general public REST guide requires `Authorization` and `Content-Type` for
body requests. IAM OpenAPI operations additionally advertise optional
`Organization` and `Workspace` header parameters, while group/policy create
bodies include an `organizationId` field. The existing transport deliberately
strips caller-supplied organization/workspace headers.

P5-010 must determine, using official sources and narrowly scoped read-only
current-Cloud evidence, whether an access token alone selects the exact IAM
tenant or whether IAM requires an explicit immutable tenant selector. Until
that task completes:

- preserve the existing five-attribute provider schema;
- do not enable arbitrary header injection or forward caller headers;
- do not hardcode any Cloud organization/workspace identity;
- do not assume an organization key and organization UUID are interchangeable;
  and
- do not publish IAM Import IDs as stable contracts.

If a tenant selector is proven necessary, add only the narrow validated
provider/client contract required by public IAM callers, make it impossible to
send credentials or tenant headers to another origin/path, and redact the
selector from all diagnostics and logs.

## Terraform contracts

### Exact member data source

`data.featbit_member` is read-only. P5-010 freezes whether it accepts an exact
UUID only or also an exact email selector. A search result is discovery input,
not identity: consume every page, compare the frozen field exactly, and reject
zero, duplicate, contradictory, or incomplete results. Return only safe fields
needed by binding callers, such as exact ID, name, and email.

The public member response can contain `initialPassword`. Endpoint wire types,
formatters, fixtures, diagnostics, logs, and Terraform state must omit it
completely. The provider never invites, creates, updates, or removes members.

### Group resource and data source

`featbit_group` manages only the frozen custom-group name and description
contract. Membership and attached policies are not set-valued fields owned by
the group resource. `data.featbit_group` reads one exact group without granting
mutation ownership.

P5-011 freezes tenant identity, name uniqueness/filter behavior, nullable
description canonicalization, server UUID/RN observations, replacement
semantics, and Import form before P5-012 registers CRUD.

### Policy resource and data source

`featbit_policy` manages custom policy settings and the statement fields proven
safe by the public contract. `data.featbit_policy` may observe built-in or
custom policies, but built-in policy type must be structurally unreachable from
resource mutation.

Policy statement order is semantically irrelevant according to the public IAM
documentation. P5-011 must still freeze exact effect/resource-type/action/RN
spellings, element identity, server IDs, set canonicalization, validation, and
unknown-value behavior before exposing statements. Member and group
assignments remain separate resources, never fields owned by the policy.

### Independent relationship resources

Each binding resource requires two exact immutable identities and computes one
canonical composite identity after P5-010/P5-011 freeze its public form:

| Resource | Owned edge | Authoritative read |
|---|---|---|
| `featbit_group_member` | one group UUID + member UUID | complete exact group-member or member-group relation |
| `featbit_group_policy` | one group UUID + custom/built-in policy UUID | complete exact group-policy or policy-group relation |
| `featbit_member_policy` | one member UUID + direct policy UUID | direct-policy view only; inherited policy is never an owned edge |

Create adds the one missing edge once. Read preserves state unless complete
evidence proves the exact pair absent. Delete removes the one pair once and
then proves exact absence. Ambiguous add/remove responses reconcile through the
authoritative relation read without replay. Import accepts only the frozen
ordered pair and never claims sibling edges.

## Identity, pagination, and canonicalization invariants

- Treat IAM object IDs as strict UUIDs once the public response proves that
  contract. Never select the first name/email filter result.
- Preserve email/name case and whitespace exactly until P5-010/P5-011 freeze
  documented comparison and validation semantics.
- Consume every page and reconcile `totalCount`. Empty-before-total, repeated,
  malformed, inconsistent, or non-advancing pages are ambiguous.
- A direct `404` or filtered zero is not authoritative absence when a complete
  collection or relationship view is required by the lifecycle.
- Keep endpoint wire models narrow. Never retain `initialPassword`, invitation
  data, tenant details, audit fields, or unrelated member/group/policy lists.
- Canonicalize policy statements independently of API order without changing
  deny/allow meaning, exact resource RNs, or exact action strings.
- Correlate server-owned statement identities only when the public API makes
  them stable. Reject duplicates or missing required identities instead of
  matching by response index.

## Lifecycle and relationship invariants

- Before creating an object, use only the collision contract frozen for that
  endpoint. Do not invent uniqueness or adopt an existing fuzzy match.
- Execute every add, remove, Create, Update, and Delete mutation once. Reconcile
  ambiguous results by exact read; never retry a mutation automatically.
- Read and persist canonical state after each logical write boundary. Preserve
  prior state whenever identity, tenant scope, relationship existence, or
  mutation outcome is ambiguous.
- Reject built-in/system policy mutation before transport. A custom policy must
  not become mutable merely because its response shape resembles a built-in
  policy.
- Parent object resources never rewrite complete member/policy collections.
  Binding resources never remove or reorder sibling relationships.
- Direct member-policy ownership excludes policies inherited through groups.
- Serialize only concrete colliding write boundaries and keep lock waits
  cancellation-safe. Do not add a generic global IAM lock.
- Trusted cleanup removes member-policy, group-policy, and group-member edges
  before custom policies and groups, while proving all pre-existing edges are
  unchanged.

## Security and redaction invariants

- Credentials remain out of Terraform state, files, fixtures, diagnostics, and
  logs. Prefer least-privilege service tokens for long-lived automation.
- Treat member IDs, names, emails, tenant selectors, group/policy IDs and keys,
  statement resources/actions, and relation pairs as runtime values in errors
  and captured logs.
- Never format or decode `initialPassword`. Tests inject markers for every
  unsafe field and prove they are absent from diagnostics, logs, assertions,
  and repository content.
- Do not persist current-Cloud member inventories, policy documents, tenant
  identity, response bodies, or cleanup journals. Test ownership and cleanup
  inventory remain in memory.

## Verification strategy

- Table-driven endpoint contracts freeze method, escaped path, exact query
  casing, optional tenant context, JSON body, envelope, pagination,
  cancellation, retry, one-shot mutation, and redaction behavior.
- Focused provider tests freeze schemas, canonicalization, plan modifiers,
  Import, ambiguity, state preservation, and single-edge ownership.
- Protocol v6 tests exercise member/group/policy data sources, both managed
  objects, all three bindings, Import, drift, second plans, out-of-band edge
  changes, and child-first destroy through one narrow stateful fixture.
- Cross-resource tests prove core resources remain unchanged while IAM policy
  RNs refer to them only as opaque verified values.
- Trusted current-Cloud tests use only uniquely named test-owned groups/custom
  policies and an explicitly supplied member identity. They never create or
  remove a member, mutate built-in policies, inspect unrelated projects, or
  alter unrelated relationships.
- The complete local gate retains formatting, vet, unit/race, repeated
  contracts, Protocol tests, build, module/dependency/license/vulnerability,
  diff, local override, schema JSON, and repository redaction scans.

## Execution order

1. Freeze tenant scope/context-header behavior and complete exact member reads.
2. Freeze Group/Policy wire taxonomy, canonicalization, schemas, and exact data
   sources.
3. Add custom Group CRUD, Import, and recovery.
4. Add custom Policy settings/statements CRUD, Import, and recovery.
5. Add one-edge group-member, group-policy, and direct member-policy resources.
6. Prove the combined Protocol lifecycle, ownership/redaction boundaries,
   trusted scoped current-Cloud behavior, and the complete Phase 5 gate.

## Out of scope

- Member invitation, creation, initial password, profile changes, activation,
  removal from an organization/workspace/team, or Terraform ownership of the
  default team.
- Nested groups, bulk relationship-set ownership, inherited-policy mutation,
  built-in policy mutation/clone, access-token management, SSO, licenses,
  Relay Proxy permissions, or IAM audit streams.
- Arbitrary context headers, Portal APIs, direct database access, generated
  clients, a raw REST resource, or speculative generic graph/policy engines.
- Permanent CI/release wiring, Registry documentation, packaging, and
  publication; those remain Phase 6.

## Exit gate

- All items in [todo.md](todo.md) are complete.
- IAM tenant scope and authentication are proven without arbitrary header
  forwarding, credential leakage, or regression of the four core resources.
- The provider schema preserves every core provider/resource/data-source
  contract and exposes exactly the IAM objects and bindings frozen in this
  phase.
- Exact member lookup rejects fuzzy, duplicate, incomplete, or contradictory
  results and never observes or stores `initialPassword`.
- Custom Group and Policy Create, exact Read, Update, Import, second-plan
  idempotence, drift, replacement where applicable, out-of-band deletion, and
  exact cleanup pass.
- All three binding resources converge, reconcile ambiguous one-shot
  mutations, preserve sibling/direct/inherited relationships, and own only one
  exact pair.
- Built-in policies and member lifecycle remain structurally unreachable from
  mutation paths.
- Formatting, vet, unit/race, repeated endpoint contracts, Protocol v6 tests,
  build, module/dependency verification, diff checks, local override, schema
  assertions, repository redaction scans, and trusted scoped current-Cloud
  acceptance pass.
- The current plan identifies Phase 6's first concrete release-readiness task.

After the gate passes, fold only still-current architecture and roadmap facts
into [the master plan](../plan.md), delete this Phase 5 directory, and create
only the Phase 6 README/TODO.
