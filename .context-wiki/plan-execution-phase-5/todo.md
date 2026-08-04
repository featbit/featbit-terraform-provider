# Phase 5 TODO — IAM

Status: **In progress**
Next: **P5-010**

Work one item at a time. Before checking an item, add a concise `Result` under
it containing:

- the important files changed;
- the runtime call relationship introduced or verified; and
- the exact verification that passed.

Do not create a separate ADR, evidence file, findings log, session log, or
handoff.

## IAM tenant and member read contract

- [ ] **P5-010 — Freeze IAM tenant scope and complete exact Member reads.**

  Scope: reconcile the current official REST/IAM documentation, public OpenAPI,
  existing authorization transport, and narrowly scoped current-Cloud read
  behavior before any IAM Terraform schema or mutation. Freeze whether an
  access token alone selects the exact tenant, whether the OpenAPI's optional
  Organization/Workspace headers are ever required, and how `organizationId`
  in future create bodies is obtained without hardcoding or leaking tenant
  identity. Add only the safe `MemberVm` fields needed for exact member lookup;
  implement direct exact-ID read plus complete pagination and an exact resolver
  for any verified email selector. Omit `initialPassword` and all invitation or
  member-lifecycle fields structurally.

  Important files: a narrow IAM Member endpoint file and focused tests under
  `internal/client`; existing `transport.go`, request construction, pagination,
  UUID, error, retry, and redaction helpers only where their contracts match.
  Provider schema/configuration and the authorization transport may change only
  if public evidence proves a tenant selector is necessary. Do not add a Member
  resource, Group/Policy schema, relationship mutation, generated client, raw
  OpenAPI snapshot, or persisted probe output.

  Runtime relationship: future `data.featbit_member` and binding callers use
  `Client.GetMember` for one exact complete object or
  `ListMembers(all pages) -> ResolveMember(exact ID/email)` when collection
  composition is required. Requests remain
  `lifecycle caller -> shared request builder -> authorizationTransport ->
  documented /api/v1 member endpoint`.

  Done when focused contracts freeze direct/list response completeness, exact
  query names and casing (`SearchText`, `PageIndex`, `PageSize`), zero-based
  advancement, stable `totalCount` reconciliation, UUID and any exact-email
  comparison semantics, zero/one/duplicate/contradictory results, fuzzy search
  rejection, direct `404` behavior, malformed/incomplete/repeated pages,
  cancellation, bodyless-GET retry, and redaction. Transport tests prove
  whether Authorization-only scoping remains correct or a narrowly validated
  tenant selector is required; caller-injected context headers remain
  impossible. Formatting, errors, logs, fixtures, and assertions contain no
  token, tenant identity, member ID/name/email, response body, path, or
  `initialPassword`. Any current-Cloud read uses only an explicitly supplied
  tenant/member target, performs no mutation, inspects no unrelated project,
  and retains no runtime value.

## Group and Policy read/schema contract

- [ ] **P5-011 — Freeze Group/Policy taxonomy, canonicalization, schemas, and exact data sources.**

  Scope: add complete exact/list reads for Groups and Policies using only the
  tenant contract frozen in P5-010. Freeze Group name/description/UUID/RN
  shapes, custom versus built-in Policy classification, Policy key/type,
  settings, statements, effects, resource types, actions, resource RNs,
  server-owned statement IDs, nullable fields, length/character limits, and
  filter behavior. Define one canonical Policy model in which statement order
  is irrelevant without weakening exact deny/allow, resource, or action
  semantics. Add and register exact `data.featbit_member`,
  `data.featbit_group`, and `data.featbit_policy`; freeze Group and custom
  Policy resource schemas and Import/replacement contracts without registering
  mutation lifecycles yet.

  Important files: narrow Group/Policy endpoint files and focused tests under
  `internal/client`; IAM provider models, canonicalization, validators, schemas,
  exact data sources, and provider registration tests under
  `internal/provider`. Reuse the UUID validator, ProviderData checks,
  set/list/object framework types, stable-ID modifier, and complete-pagination
  helpers only when exact ownership matches. Keep Group members and Policy
  assignments out of parent resource ownership.

  Runtime relationship: `data.featbit_member/group/policy.Read -> exact direct
  read or complete filtered collection -> exact resolver -> IAM canonicalizer
  -> Terraform state`. Future resources reuse the same canonical Group/Policy
  state layer, while endpoint wire models remain serialization-only.

  Done when tests freeze exact methods, escaped paths, `Name`/page query casing,
  complete pagination, direct-versus-list shape boundaries, exact UUID and
  verified key/name identity behavior, custom/built-in Policy classification,
  built-in mutation rejection, statement set equality, nested action/resource
  set ordering, server-ID correlation, Null/Unknown/default behavior, duplicate
  or contradictory statements, invalid effects/resource types/RNs/actions,
  and redaction. Schema tests freeze every Required/Optional/Computed/Sensitive
  flag and replacement modifier, member read-only behavior, no
  `initialPassword` field, no relationship-set ownership, strict provisional
  Import grammar, and exact provider data-source registrations.

## Managed IAM objects

- [ ] **P5-012 — Add custom Group CRUD, Import, and recovery.**

  Scope: register `featbit_group` and add only the documented one-shot Group
  Create, settings Update, and Delete operations. Use the uniqueness/collision
  contract frozen in P5-011 rather than assuming name uniqueness. Establish
  provisional UUID state as soon as a create response is authoritative, read
  canonical state after each logical mutation, and reconcile ambiguous results
  without replay, fuzzy adoption, or claiming member/policy collections.
  Import accepts only the tenant/object identity form frozen by P5-010/P5-011.

  Important files: Group client endpoint methods and focused contracts;
  `group_resource.go`, frozen Group models/schema, provider registration,
  Import parsing, keyed serialization only if a concrete collision boundary
  requires it, and focused lifecycle tests. Do not add binding or Policy
  mutations.

  Runtime relationship: `groupResource.Create/Read/Update/Delete -> frozen
  exact Group resolver -> one documented mutation when needed -> exact
  canonical read/absence proof -> Terraform state`.

  Done when tests cover Create, exact Read, name/description Update, no-op
  Update, Import, in-place ID stability, every replacement input, second-plan
  idempotence, arbitrary list ordering, drift, out-of-band deletion,
  pre-existing collisions, duplicate/fuzzy matches, partial/direct-read
  failures, ambiguous one-shot mutation reconciliation, cancellation including
  lock wait, tenant mismatch, and exact absence. Delete never removes or
  rewrites sibling Groups, members, policies, or any default team object; all
  diagnostics and logs redact runtime Group/tenant values.

- [ ] **P5-013 — Add custom Policy CRUD, statements, Import, and recovery.**

  Scope: register `featbit_policy` and add only documented custom Policy Create,
  settings Update, statement-list Update, and Delete. Reject built-in/system
  policy mutation before transport. Diff canonical prior state from plan and
  execute changed settings/statements once in a fixed order followed by one
  exact canonical read. Preserve statement meaning while canonicalizing
  order-insensitive collections and server-owned identities. Reconcile partial
  or ambiguous multi-call results without replay or adoption.

  Important files: Policy endpoint methods and focused contracts; Policy
  models/canonicalizer/schema, `policy_resource.go`, provider registration,
  strict Import parsing, concrete keyed coordination if required, and focused
  lifecycle tests. Relationship endpoints remain reserved for P5-014/P5-015.

  Runtime relationship: `policyResource.Create -> one CreatePolicy ->
  provisional UUID -> optional one statements PUT -> exact GetPolicy ->
  canonical state`; Update is `canonical diff -> optional settings PUT ->
  optional statements PUT -> exact GetPolicy`; Delete is one exact custom-policy
  delete followed by authoritative absence proof.

  Done when tests cover custom Create, exact Read, settings-only,
  statements-only, and combined Update; no unchanged write; deterministic
  order; Import; second-plan idempotence; API reordering; statement/action/
  resource set changes; server-ID correlation; replacement inputs; drift;
  out-of-band deletion; built-in policy read-only data-source behavior and
  zero mutation from every resource path; ambiguous/partial reconciliation;
  cancellation; one-shot counts; relation-set preservation; and exact absence.
  Diagnostics/state/logs reveal no tenant, Policy key/ID, statement action,
  resource RN, token, path, or body.

## Independent relationship resources

- [ ] **P5-014 — Add the one-edge Group/Member binding resource.**

  Scope: add complete authoritative Group-member relation reads plus documented
  one-shot add/remove operations. Register `featbit_group_member` with two exact
  immutable identities and the composite Import contract frozen by P5-010/
  P5-011. Create proves the exact pair absent before adding it once; Read proves
  exact presence without flattening unrelated members; Delete removes only that
  pair once and then proves exact absence. Ambiguous add/remove results
  reconcile through a complete relation read without replay.

  Important files: Group-member client methods and focused pagination/
  relationship contracts; a dedicated binding model, schema, resource,
  registration, Import parser, and lifecycle tests. Do not add a members set to
  `featbit_group`, mutate the Member, or manage the default team.

  Runtime relationship: `groupMemberResource -> complete exact group/member
  relation read -> optional one add-member or remove-member PUT -> complete
  exact relation read -> one-pair Terraform state`.

  Done when tests cover exact absent/present, case/canonical UUID handling,
  duplicate/inconsistent relation items, fuzzy email/name results, later-page
  matches, malformed/incomplete pagination, already-present Create,
  out-of-band add/remove, Import then empty plan, one-shot ambiguous add/remove
  reconciliation, cancellation, parent absence/tenant mismatch, composite-ID
  ordering, and sibling-edge preservation. Destroy never removes the Member or
  Group and never changes any other group membership or policy edge.

- [ ] **P5-015 — Add one-edge Group/Policy and direct Member/Policy bindings.**

  Scope: add complete authoritative Group-policy and direct Member-policy reads
  plus documented one-shot add/remove operations. Register
  `featbit_group_policy` and `featbit_member_policy` as separate pair resources.
  Direct member binding must distinguish explicitly assigned policies from
  policies inherited through Groups; an inherited match alone is not an owned
  edge and must not be removed. Permit a built-in Policy as a binding target
  only if the public contract proves the edge is safe, while its Policy object
  remains immutable.

  Important files: narrow Group/Policy and Member/direct-Policy relation
  methods and tests; two dedicated resource schemas/models, registration,
  composite Import parsing, exact lifecycle tests, and shared pair helpers only
  if relationship direction, error, and redaction semantics are identical.

  Runtime relationship: each binding uses `complete exact relation read ->
  optional one add-policy/remove-policy PUT -> complete exact relation read ->
  one-pair Terraform state`; the Member variant reads direct policies only for
  ownership and may observe inherited policies only as non-owned context.

  Done when both resources cover exact absent/present, later-page matches,
  duplicate/inconsistent items, already-present Create, out-of-band add/remove,
  Import and empty second plan, ambiguous one-shot reconciliation,
  cancellation, composite-ID order, parent absence, tenant mismatch,
  custom/built-in target rules, and sibling-edge preservation. Member-policy
  tests prove an inherited-only policy is never adopted or removed; Group
  tests prove no member edge is changed; Policy tests prove no statement or
  settings mutation is sent.

## Protocol and cross-resource verification

- [ ] **P5-016 — Prove the IAM Terraform Protocol v6 lifecycle.**

  Scope: exercise the registered Member/Group/Policy data sources, custom Group
  and Policy resources, and all three bindings through Protocol v6 against one
  narrow stateful public-API fixture. Model only exact IAM reads, complete
  pagination, custom-object mutations, direct/inherited relation views, and
  add/remove operations needed by production callers. Do not turn the fixture
  into a general FeatBit IAM server.

  Important files: IAM Protocol tests and one focused fixture under
  `internal/provider`. Reuse the existing Terraform CLI/provider-server
  harness, dependency helpers, recorders, and canonical response helpers only
  where contracts match.

  Runtime relationship: `terraform plan/apply/refresh/import/destroy ->
  providerserver Protocol v6 -> IAM object/data-source/binding lifecycle ->
  handwritten IAM adapter -> isolated public-API fixture`.

  Done when Protocol tests cover exact member read, custom Group/Policy Create,
  exact data-source Read, Import plus empty plan, second empty plan, settings/
  statements drift repair, every frozen replacement, arbitrary collection
  ordering, out-of-band object and edge deletion, direct versus inherited
  Policy ownership, ambiguous response recovery, and child-first destroy. The
  recorder proves exact call order, no mutation retry, no forbidden member/
  built-in/bulk endpoint, no arbitrary context header, no sibling-edge rewrite,
  and final exact object/edge cleanup.

- [ ] **P5-030 — Prove cross-resource ownership, state safety, and redaction.**

  Scope: compose Member, Group, Policy, all three bindings, and representative
  core resource RNs at shared safety boundaries. Test dependency ordering,
  immutable pair replacement, exact direct/inherited relationship distinction,
  policy statement canonicalization, Null/Unknown/default behavior, ambiguous
  state preservation, Import diagnostics, tenant isolation, and
  secret/runtime-value redaction. Confirm IAM lifecycles never claim or mutate
  Project/Environment/Feature Flag/Segment objects referenced by opaque Policy
  RNs.

  Important files: focused integration tests under `internal/provider`,
  endpoint redaction/log-capture tests under `internal/client`, and existing
  fixtures/helpers only where exact semantics match. Production fixes only
  where a failing contract demonstrates a concrete issue.

  Runtime relationship: `core resource RN <- custom Policy <- direct Member or
  Group binding <- Group Member -> lifecycle-owned adapters -> canonical
  state`, while failures flow through `Client.DecodeResponse -> redacted
  APIError -> lifecycle diagnostics/tflog`.

  Done when object and edge dependency/replacement plans converge; direct and
  inherited policies remain distinct; removing one edge leaves all siblings
  and core objects unchanged; arbitrary set/API ordering and Null/Unknown
  values do not cause permanent diffs; duplicates preserve state; and injected
  tokens, tenant/member/email, Group/Policy IDs/keys, actions, resource RNs,
  relation pairs, server details, paths, bodies, and initial-password markers
  appear in neither diagnostics, logs, fixtures, nor assertion output.

## Integration and verification

- [ ] **P5-031 — Run trusted scoped current-Cloud IAM acceptance and exact cleanup.**

  Scope: with credentials and tenant/member identity supplied only out of band,
  run uniquely prefixed custom Group/Policy and one-edge binding scenarios
  against documented current-Cloud endpoints. Create only test-owned Groups
  and custom Policies in the explicitly authorized tenant. Bind only the
  explicitly supplied existing Member, use a no-access or otherwise harmless
  test Policy contract frozen earlier, record every created object/edge before
  mutation, and restore the member's exact baseline. Never create/remove a
  Member, mutate a built-in Policy, inspect unrelated projects, or alter an
  unrelated relationship.

  Important files: credential-gated IAM acceptance tests and in-memory
  child-first cleanup inventory under `internal/provider`. Reuse the Cloud
  transport/harness only where it preserves explicit tenant scope, exact edge
  ownership, cleanup, and redaction. Permanent CI wiring remains Phase 6.

  Runtime relationship: `terraform-plugin-testing -> local Protocol v6
  provider -> shared client -> documented FeatBit Cloud IAM endpoints -> exact
  relation and custom object cleanup verification`.

  Done when custom Group/Policy lifecycle, exact data sources, Import followed
  by empty plan, second-plan idempotence, drift repair, every binding,
  direct/inherited distinction, out-of-band edge deletion, and destroy pass.
  Cleanup removes direct member-policy, group-policy, and group-member edges
  before Policy and Group, then independent complete reads prove every test
  object/edge absent and every pre-existing Member edge unchanged. No pending
  cleanup owner/action, persisted tenant/member/policy value, or unrelated
  object read remains. If safe permissions or an explicit Member fixture are
  unavailable, record the exact limitation under this item and retain
  public-contract/Protocol coverage without broadening access.

- [ ] **P5-032 — Run the complete Phase 5 local gate.**

  Scope: run formatting, vet, all unit/mock/Protocol tests, repeated IAM
  endpoint/pagination/canonicalization/relation/redaction contracts, Windows
  race with a verified compiler, build, module tidy/verify, dependency/license/
  vulnerability inspection, diff checks, local provider override, schema JSON
  assertions, and repository secret/runtime-value scans. Do not add Phase 6
  CI, Registry/release work, generated clients, or generator dependencies.

  Important files: production/test fixes required by the gate, this active
  TODO, and the Phase 5 README next-task pointer.

  Runtime relationship: verify `Terraform Core -> Protocol v6 provider -> core
  plus IAM object/binding lifecycles -> shared handwritten client -> documented
  public API` and confirm no other local runtime path exists.

  Done when `gofmt -l .` is empty; `go vet ./...`, `go test ./...`, repeated
  focused tests, `go test -race ./...`, `go build ./...`, `go mod tidy -diff`,
  `go mod verify`, dependency scans, and `git diff --check` pass. The local
  override loads; schema JSON asserts the frozen provider attributes, every
  core registration, the exact IAM resource/data-source count, and every
  visible ownership/Sensitive flag. Focused tests assert tenant isolation,
  member read-only behavior, custom/built-in Policy separation, statement
  normalization, replacement, direct/inherited distinction, one-edge
  ownership, and sibling preservation. Repository scans find no real secret,
  member/email/initial-password marker, runtime Cloud tenant/object/relation
  value, forbidden endpoint/header, or risky state artifact.

## Phase exit

- [ ] **P5-090 — Close Phase 5 and prepare Phase 6.**

  Confirm every item above and the README exit gate. Update the master plan
  only with still-current runtime/roadmap facts and make its next action the
  first concrete release-readiness task. Then delete this completed Phase 5
  package and create only a Phase 6 `README.md` and detailed `todo.md`.

- [ ] **P5-091 — Declare Phase 5 complete only after the exit gate passes.**

  This final consistency check must find no unchecked item, unresolved IAM
  tenant/header/identity assumption, Member lifecycle path, built-in Policy
  mutation path, statement canonicalization gap, whole-set relationship
  ownership, direct/inherited confusion, secret/runtime-value finding, failed
  lifecycle or Import convergence, changed pre-existing Cloud edge, pending
  test object/edge, missing schema verification, or absent Phase 6 entry point.
