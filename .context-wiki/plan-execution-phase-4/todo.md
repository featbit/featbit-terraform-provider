# Phase 4 TODO — Segments

Status: **In progress**
Next: **P4-030**

Work one item at a time. Before checking an item, add a concise `Result` under
it containing:

- the important files changed;
- the runtime call relationship introduced or verified; and
- the exact verification that passed.

Do not create a separate ADR, evidence file, findings log, session log, or
handoff.

## Segment read contract

- [x] **P4-010 — Freeze Segment taxonomy and add complete exact reads.**

  Scope: verify the current documented public shapes for exact Segment reads,
  paginated list items, `environment-specific` versus `shared` type/scope
  semantics, archived filtering, and Feature Flag reference results. Add only
  the safe Segment fields needed by this phase. Implement exact environment/id
  Get plus complete active and archived collection reads, and compose exact
  UUID and case-sensitive key statuses for data-source/resource reads, Create
  preflight, mutation reconciliation, and absence proof. A metadata-only list
  match may prove presence/status but may not be flattened as complete
  targeting state. Reuse Feature Flag pagination only if a shared helper can
  preserve both endpoints' validation and redaction boundaries; otherwise keep
  the Segment implementation narrow.

  Important files: a new lifecycle-owned Segment endpoint file and focused
  tests under `internal/client`; existing `request.go`, `identifiers.go`,
  `errors.go`, `redaction.go`, Feature Flag pagination, and client test helpers
  only where their exact contracts match. Do not add Terraform schemas or
  mutations yet. Do not retain probe payloads, identities, or separate evidence
  files.

  Runtime relationship: future callers use `Client.GetSegment` for one complete
  exact object; absence-sensitive paths use complete
  `ListSegments(active) + ListSegments(archived) -> exact UUID/key resolver`.
  Reference preflight has a separate exact-ID read boundary and does not run as
  part of ordinary refresh.

  Done when focused contracts prove documented methods, escaped UUID paths,
  exact query names/casing, zero-based page advancement, stable `totalCount`
  reconciliation, active/archived/zero/cross-view-duplicate outcomes, exact
  case-sensitive key behavior, unknown type/scope fail-closed behavior,
  incomplete direct/list shapes, retry and cancellation boundaries, no
  organization/workspace headers, and redaction of Segment/user/flag
  identities, keys, conditions, tags, scopes, paths, bodies, and tenant data.

  Result (2026-08-04): `internal/client/segments.go` and
  `internal/client/segments_test.go` freeze the two documented Segment types,
  full organization/project/environment scope-RN classification, complete
  exact targeting reads, metadata-only list matches, stable all-page active/
  archived reconciliation, exact UUID/case-sensitive-key status resolution,
  and the separate exact flag-reference boundary. The runtime is now
  `future Segment lifecycle -> GetSegment` for one complete definition or
  `ListSegments(active + archived) -> ResolveSegment` for authoritative
  identity/status, while `GetSegmentFlagReferences` remains preflight-only.
  Current Swagger/source review plus disposable current-Cloud checks confirmed
  the complete-versus-list shape boundary, full env-scope RN requirement, and
  empty-reference encoding; every temporary child and parent was removed.
  `gofmt`, `go test ./internal/client -run Segment -count=10`, 20 repeated
  exact/list/resolver/redaction contracts, `go test ./...`, `go vet ./...`,
  `go build ./...`, and `git diff --check` passed.

- [x] **P4-011 — Freeze canonical targeting, schemas, and the exact data source.**

  Scope: define one provider-owned Segment model and normalization layer for
  metadata, type/scopes, included and excluded users, tags, ordered rules, and
  conditions. Freeze exact key/name/description limits, supported operators and
  value encoding, rule/condition ordering, UUID ownership/correlation, set
  deduplication/order, Null/Unknown/default behavior, and contradictions such as
  duplicate identities or unsafe type/scope combinations. Add and register
  `data.featbit_segment` with required exact `environment_id`/`id`; permit safe
  active reads of environment-specific and shared segments. Freeze an
  environment-specific-only resource schema and replacement contract, but do
  not register its lifecycle yet.

  Important files: narrow Segment Terraform models/canonicalization/schema
  under `internal/provider`; `segment_data_source.go` and focused tests;
  provider data-source registration and schema/configuration tests. Reuse the
  UUID validator, ProviderData checks, set/list framework types, and stable-ID
  modifier only where their semantics match. Do not build a generic targeting
  expression framework.

  Runtime relationship: `segmentDataSource.Read -> Client.GetSegment -> exact
  active full result -> type/scope + targeting canonicalization -> Terraform
  state`. Future resource planning uses the same canonicalizer but rejects
  shared or contradictory type/scope state before any mutation.

  Done when table-driven tests cover both documented types; safe shared scope
  observation; environment-specific resource-only enforcement; exact set
  behavior for included/excluded users and tags; preserved rule evaluation
  order; frozen condition order/value encoding; stable imported and, if
  required, deterministic provider-created rule/condition UUIDs; arbitrary API
  ordering; missing/duplicate identities; archived/absent/ambiguous reads;
  Configure type checks; and Protocol schema ownership. Schema tests must prove
  every in-place versus replacement field and that shared mutation cannot be
  configured.

  Result (2026-08-04): `internal/provider/segment_models.go`,
  `segment_schema.go`, and their focused tests freeze one redacting canonical
  model with exact metadata limits, all 16 public operators, canonical
  operator values, set-valued scopes/users/tags, ordered rules/conditions,
  exact imported UUID correlation, and deterministic provider-created
  targeting UUIDs. `segment_data_source.go` is registered for exact active
  environment/Segment UUID reads of both documented types; the future resource
  schema is environment-specific-only, requires an immutable Environment RN,
  preserves IDs for in-place fields, and remains deliberately unregistered.
  Runtime is `segmentDataSource.Read -> Client.GetSegment -> canonicalize ->
  Terraform state`; malformed, archived, absent, or contradictory results
  preserve prior state with redaction-safe diagnostics. `gofmt`, 10 repeated
  Segment provider/client contracts, the complete provider suite, `go test ./...`,
  `go vet ./...`, `go build ./...`, and `git diff --check` passed.

## Environment-specific Segment resource

- [x] **P4-012 — Add Segment Create, replacement, Import, and recovery.**

  Scope: add and register `featbit_segment`; add the documented one-shot Create
  endpoint and only the specialized targeting/tag initialization needed to
  materialize the full plan. Require exact zero by case-sensitive key across
  complete active and archived collections. Establish provisional state as soon
  as the server UUID is authoritative, read canonical state after each logical
  write boundary, add strict `<environment_uuid>/<segment_uuid>` Import, and
  reconcile ambiguous or partial Create without retrying mutations, adopting a
  pre-existing key, or mutating a shared segment. Apply replacement and stable
  computed-ID planning exactly as frozen in P4-011.

  Important files: `internal/client/segments.go` and focused Create/initialization
  contracts; `internal/provider/segment_resource.go`, Segment models/schema,
  composite Import parsing, provider registration, and focused lifecycle tests.
  Extend the keyed lock only when the concrete multi-call Create demonstrates
  the need.

  Runtime relationship: `segmentResource.Create -> complete active+archived
  exact-key zero preflight -> one CreateSegment -> required one-shot targeting/
  tag initialization -> GetSegment -> canonical state`. Ambiguous outcomes use
  the same complete resolver and return recovery/Import guidance without
  replay or silent adoption.

  Done when tests cover invalid environment/id/key/type/scope/targeting values
  before transport; active and archived collisions; fuzzy non-matches;
  duplicates/incomplete pages; exact documented payloads and no context
  headers; initial empty and non-empty targeting/tags; response identity/type/
  scope mismatch; ambiguous POST and partial initialization outcomes;
  read-after-write; strict two-component Import; imported server identities;
  replacement and in-place stable-ID plans; cancellation/lock waits; one-shot
  call counts; and redaction-safe recoverable state.

  Result (2026-08-04): `internal/client/segments.go` and its focused tests add
  the exact one-shot Create, targeting, and tag payloads; the new
  `internal/provider/segment_resource.go` and tests register the
  environment-specific resource with canonical ModifyPlan, exact Read,
  provisional UUID state, strict Import, keyed Create serialization, and
  fail-closed Update/Delete boundaries reserved for P4-013/P4-014. Runtime is
  now `segmentResource.Create -> complete active+archived key preflight -> one
  CreateSegment -> provisional UUID -> exact GetSegment -> conditional one
  targeting PUT/Get -> conditional one tags PUT/Get -> canonical state`;
  ambiguous POST or initialization outcomes reconcile without replay or
  adoption, and shared/type/scope contradictions reach no follow-up mutation.
  Current official OpenAPI/source shapes were rechecked. `gofmt -l .`, 10
  repeated client and provider Segment contracts, `go test ./...`, `go vet
  ./...`, `go build ./...`, `go mod tidy -diff`, `go mod verify`, and `git diff
  --check` passed. Focused race remains part of P4-032 because this Windows
  host currently has no C compiler for CGO.

- [x] **P4-013 — Add specialized metadata, targeting, and tag Update.**

  Scope: add only the documented name, description, targeting, and tag endpoint
  methods. Diff canonical prior state from plan and execute each changed owned
  component once in one frozen deterministic order, followed by exact canonical
  Read. Do not send unchanged components. Reconcile each ambiguous result
  without mutation replay and preserve recoverable state after partial
  multi-call success. Never call generic PATCH, restore, shared mutation, or a
  Feature Flag targeting operation.

  Important files: `internal/client/segments.go` and focused specialized
  mutation contracts; `internal/provider/segment_resource.go`, the Segment
  canonicalizer, keyed serialization where required, and lifecycle tests.

  Runtime relationship: `segmentResource.Update -> canonical diff -> zero or
  one name PUT -> zero or one description PUT -> zero or one targeting PUT ->
  zero or one tags PUT -> exact GetSegment -> canonical state`.

  Done when tests prove exact bodies and paths; no unchanged-field write;
  stable deterministic call order; metadata-only, targeting-only, tags-only,
  and combined updates; set reordering without mutation; rule reordering with
  the frozen semantics; read-after-update; shared/type/scope mismatch rejection;
  ambiguous and partial-success reconciliation; cancellation including lock
  wait; one-shot call counts; and diagnostics/state that reveal no runtime
  identity or targeting value.

  Result (2026-08-04): `internal/client/segments.go` and its focused tests add
  exact one-shot name and description PUT contracts while reusing the existing
  targeting/tag Boolean mutation boundary. `internal/provider/segment_resource.go`
  and the focused `segment_update_test.go` freeze one canonical prior/plan diff,
  serialize the exact Environment/Segment write boundary, send only changed
  components in name/description/targeting/tags order, advance recoverable state
  after each successful component, reconcile ambiguous results by exact read
  without replay, and finish with one exact canonical read. Runtime is now
  `segmentResource.Update -> canonical diff -> optional name PUT -> optional
  description PUT -> optional targeting PUT -> optional tags PUT -> GetSegment
  -> canonical state`; unsafe identity/type/scope state and remote reconciliation
  mismatches fail closed. Current official OpenAPI shapes were rechecked.
  `gofmt -l .`, 10 repeated specialized client and resource Update contracts,
  `go test ./...`, `go vet ./...`, `go build ./...`, `go mod tidy -diff`, `go
  mod verify`, and `git diff --check` passed. Race remains owned by P4-032:
  this Windows host has `CGO_ENABLED=0`, and enabling it confirms that `gcc` is
  not installed.

- [x] **P4-014 — Add reference-aware archive-plus-hard-delete.**

  Scope: implement exact Feature Flag reference preflight, archive, and
  permanent delete using only documented endpoints. Resolve current
  active/archived/type status, reject shared segments, and send no destructive
  mutation when any exact reference exists or the reference result is
  incomplete. Archive only when needed, hard-delete, then prove exact UUID/key
  zero across every active and archived page. Reconcile ambiguous archive and
  delete outcomes without replay and preserve state unless exact zero is
  authoritative.

  Important files: `internal/client/segments.go` plus focused reference/archive/
  delete contracts; `internal/provider/segment_resource.go`, keyed lifecycle
  coordination, and destroy tests. Do not automatically edit referring Feature
  Flags or treat a name/key list as an exact reference result.

  Runtime relationship: `segmentResource.Delete -> exact Segment status ->
  complete exact flag-reference preflight -> optional one archive PUT -> one
  permanent DELETE -> complete active+archived exact-zero proof`.

  Done when tests cover active, already archived, already absent, and shared
  statuses; zero, one, many, duplicate, malformed, and failed reference results;
  reference conflicts with zero mutations; archive conflict/failure; permanent
  delete failure; ambiguous mutation reconciliation; later-page exact-zero
  proof; UUID/key inconsistencies; cancellation; one-shot call counts; and
  redaction-safe state preservation/recovery guidance.

  Result (2026-08-04): `internal/client/segments.go` and focused contracts add
  exact bodyless one-shot archive and permanent-delete methods while retaining
  the separately validated exact Feature Flag reference read. The resource
  lifecycle in `internal/provider/segment_resource.go` and focused
  `segment_delete_test.go` now resolves complete active/archived UUID+key
  status, rejects shared or scope-drifted matches, requires an exact empty
  reference result, archives only active Segments, permanently deletes, and
  removes state only after complete exact-zero proof. Runtime is
  `segmentResource.Delete -> ResolveSegment(active + archived) ->
  GetSegmentFlagReferences -> optional ArchiveSegment -> DeleteSegment ->
  ResolveSegment(exact absent)`; conflicts and ambiguous mutations reconcile
  without replay, while any reference, malformed result, identity mismatch, or
  incomplete proof preserves state. Current official OpenAPI/source confirmed
  nullable comment bodies, server-side archive reference rechecking, and the
  archived-only delete prerequisite. `gofmt -l .`, 10 repeated full Segment
  client and resource Delete contracts, `go test ./...`, `go vet ./...`, `go
  build ./...`, `go mod tidy -diff`, `go mod verify`, and `git diff --check`
  passed. Race remains owned by P4-032 because this Windows host has no `gcc`
  available for CGO.

- [x] **P4-015 — Prove the Segment Terraform Protocol v6 lifecycle.**

  Scope: exercise the registered Segment resource/data source through Protocol
  v6 against one narrow stateful public-API fixture. Model full exact reads,
  metadata-only active/archived pagination, specialized writes, shared scope
  observations, Feature Flag references, and permanent deletion only as needed
  by production calls. Do not turn the fixture into a general FeatBit emulator.

  Important files: Segment Protocol tests and one focused fixture under
  `internal/provider`. Reuse the existing Terraform CLI/provider-server harness,
  dependency helpers, and recorder patterns where their contracts match.

  Runtime relationship: `terraform plan/apply/refresh/import/destroy ->
  providerserver Protocol v6 -> Segment resource/data source -> Segment adapter
  -> isolated public-API fixture`.

  Done when the environment-specific resource passes Create, exact data-source
  Read, Import then empty plan, second empty plan, metadata/targeting/tag drift
  repair, every replacement input, direct-read fallback/ambiguity, arbitrary
  set ordering, ordered-rule changes, out-of-band deletion, external archive
  fail-closed behavior, reference conflict, and archive-plus-delete. A shared
  segment passes exact data-source Read while every resource mutation is
  rejected. The recorder proves exact call order, no mutation retry/forbidden
  endpoint/context header, and complete active/archived teardown.

  Result (2026-08-04): `internal/provider/segment_protocol_test.go` and the
  focused `segment_protocol_fixture_test.go` exercise the registered resource
  and both environment-specific/shared exact data-source reads through the
  existing Terraform CLI/Protocol v6 harness. The narrow stateful fixture
  implements only the production Segment public calls and proves Create,
  Import plus empty plan, a second empty plan, all owned drift repair,
  arbitrary set/collection ordering, ordered-rule changes, every replacement
  input, direct-read fallback, out-of-band deletion, shared mutation rejection,
  external archive fail-closed behavior, reference conflict, and exact
  archive-plus-delete. Runtime is `terraform plan/apply/refresh/import/destroy
  -> providerserver Protocol v6 -> Segment resource/data source -> Segment
  adapter -> isolated public-API fixture`; its recorder verifies deterministic
  specialized-write order, one-shot reconciliation after an injected ambiguous
  mutation response, no forbidden endpoint or context header, and final
  complete active/archived zero proof. `gofmt -l .`, three repeated focused
  Protocol lifecycles, `go test ./...`, `go vet ./...`, `go build ./...`, `go
  mod tidy -diff`, `go mod verify`, and `git diff --check` passed.

## Integration and verification

- [ ] **P4-030 — Prove cross-resource ownership, state safety, and redaction.**

  Scope: test Segment with Project, Environment, and Feature Flag parents/
  consumers at shared safety boundaries: dependency ordering, exact reference
  conflicts, replacement identity planning, canonical targeting correlation,
  Null/Unknown/default behavior, ambiguous state preservation, Import
  diagnostics, and secret/runtime-value redaction. Confirm Segment lifecycle
  never claims Project/Environment fields or rewrites/removes a referring
  Feature Flag.

  Important files: focused integration tests in `internal/provider`, endpoint
  redaction tests in `internal/client`, existing fixtures/helpers where exact
  contracts match, and production fixes only where a failing contract
  demonstrates a concrete issue.

  Runtime relationship: `Project -> Environment -> Segment <- Feature Flag
  reference -> lifecycle-owned adapters -> canonical state`, while failures
  flow through `Client.DecodeResponse -> redacted APIError -> lifecycle
  diagnostics/tflog`.

  Done when parent/child dependency and replacement converge; Segment destroy
  refuses exact live references without changing either object; removing the
  reference externally permits exact destroy; arbitrary collection/set ordering
  and Null/Unknown values do not cause permanent diffs; cross-view duplicates
  preserve state; and injected tokens, segment/user/flag identities, keys,
  conditions, tags, scopes, tenant/server details, paths, and bodies appear in
  neither diagnostics, logs, fixtures, nor assertion output.

- [ ] **P4-031 — Run trusted current-Cloud acceptance and exact cleanup.**

  Scope: with credentials supplied only out of band, run uniquely prefixed
  environment-specific and shared-read Segment scenarios against documented
  current Cloud endpoints. Create mutable Segments only under test-owned
  Project/Environment parents, register identities before mutation, and clean
  Segments before environments/projects even after failure. Never mutate a
  shared segment or inspect unrelated objects. Permanent CI wiring remains
  Phase 6 work.

  Important files: credential-gated Segment acceptance tests and in-memory
  child-first cleanup inventory under `internal/provider`. Reuse the existing
  Cloud Project/Environment/Feature Flag harness only where it preserves exact
  ownership, cleanup, and redaction guarantees.

  Runtime relationship: `terraform-plugin-testing -> local Protocol v6
  provider -> shared client -> documented FeatBit Cloud endpoints -> exact
  active+archived Segment and parent cleanup verification`.

  Done when the environment-specific lifecycle passes Create, exact data
  source, Import followed by empty plan, second-plan idempotence, specialized
  drift repair, replacement, out-of-band deletion, reference conflict, and
  destroy. A test-visible shared segment is read exactly without mutation if a
  safely owned fixture is available; otherwise the shared Cloud limitation is
  reported in this item without inspecting unrelated state and must remain
  covered by public-contract/Protocol tests. Independent final complete active
  and archived Segment queries plus parent Project collection verification
  return exact zero for every mutable test identity, with no pending cleanup
  owner/action or persisted runtime value.

- [ ] **P4-032 — Run the complete Phase 4 local gate.**

  Scope: run formatting, vet, all unit/mock/Protocol tests, repeated Segment
  endpoint/canonicalization/reference contracts, Windows race tests with a
  verified compiler toolchain, build, module tidy/verify, dependency/license/
  vulnerability inspection, diff checks, local provider override, schema JSON
  assertions, and repository secret/redaction scans. Do not add Phase 6 CI,
  release/Registry work, generated clients, or generator dependencies.

  Important files: production/test fixes required by the gate, this active
  TODO, and the Phase 4 README next-task pointer.

  Runtime relationship: verify the complete `Terraform Core -> Protocol v6
  provider -> Project/Environment/Feature Flag/Segment lifecycle -> shared
  client -> public API contract` and confirm no other local runtime path exists.

  Done when `gofmt -l .` is empty; `go vet ./...`, `go test ./...`, repeated
  focused tests, `go test -race ./...`, `go build ./...`, `go mod tidy -diff`,
  `go mod verify`, dependency scans, and `git diff --check` pass. The local
  override loads; schema JSON asserts five provider attributes plus exactly
  four resources/four data sources and every visible ownership/Sensitive flag;
  focused tests assert replacement, set/order, reference, shared-read-only, and
  normalization contracts; repository scans find no real secret, runtime Cloud
  identity/value, forbidden endpoint, or risky state artifact.

## Phase exit

- [ ] **P4-090 — Close Phase 4 and prepare Phase 5.**

  Confirm every item above and the README exit gate. Update the master plan
  only with still-current runtime/roadmap facts and make its next action the
  first concrete IAM task. Then delete this completed Phase 4 package and create
  only a Phase 5 `README.md` and detailed `todo.md`.

- [ ] **P4-091 — Declare Phase 4 complete only after the exit gate passes.**

  This final consistency check must find no unchecked item, unresolved Segment
  type/scope/targeting assumption, shared-mutation path, reference-preflight
  gap, secret/runtime-value finding, failed lifecycle or Import convergence,
  pending active or archived Cloud object, missing schema verification, or
  absent Phase 5 entry point.
