# Phase 3 TODO — Feature flags

Status: **In progress**
Next: **P3-090**

Work one item at a time. Before checking an item, add a concise `Result` under
it containing:

- the important files changed;
- the runtime call relationship introduced or verified; and
- the exact verification that passed.

Do not create a separate ADR, evidence file, findings log, session log, or
handoff.

## Feature Flag read contract

- [x] **P3-010 — Add complete Feature Flag reads and exact status resolution.**

  Scope: add only the safe Feature Flag/variation wire fields consumed by this
  phase; implement the documented exact Get and paginated collection Get;
  consume every page for active and archived views and compose them into one
  exact-key result that distinguishes active, archived, exact zero, and
  ambiguous duplicates/inconsistency. Reuse the shared request construction,
  UUID validation, envelope/error classification, cancellation, retry, and
  redaction contracts.

  Important files: `internal/client/feature_flags.go` and focused tests;
  existing `request.go`, `identifiers.go`, `errors.go`, and client test helpers
  only where a genuinely shared contract needs extension. Do not add Terraform
  schemas or mutations yet.

  Runtime relationship: a future Feature Flag lifecycle calls
  `Client.GetFeatureFlag`; its exact direct GET supplies the canonical object
  when valid, while absence-sensitive or failed reads call
  `Client.ListFeatureFlags(active) + Client.ListFeatureFlags(archived) ->
  complete pagination -> exact environment/key resolver`.

  Done when focused contracts prove exact methods, escaped environment/key
  paths, documented query names, no organization/workspace headers, page
  advancement and termination, `totalCount` reconciliation, stable active /
  archived / zero / duplicate outcomes, malformed or incomplete fail-closed
  behavior, bodyless-GET retry boundaries, cancellation, and redaction of
  runtime keys, IDs, variation values, tenant data, paths, and raw bodies.

  Result: Added the safe Feature Flag/variation read shapes, exact GET,
  explicit active/archived paginated GET, complete `totalCount` reconciliation,
  and exact case-sensitive status resolver in `internal/client/feature_flags.go`,
  with focused endpoint, pagination, retry, cancellation, ambiguity, and
  redaction contracts in `feature_flags_test.go`. The verified runtime path is
  `Client.GetFeatureFlag -> exact GET`, falling back to complete
  `ListFeatureFlags(active) + ListFeatureFlags(archived) -> exact-key
  resolution`. `gofmt -l internal/client` was empty; `go vet
  ./internal/client`, `go test ./internal/client -count=1`, and the focused
  Feature Flag suite repeated with `-count=10` passed.
  A follow-up library audit replaced the shared channel-based request limiter
  and keyed Environment permit with `golang.org/x/sync/semaphore`; observable
  concurrency, queued-cancellation, retry-progress, failure-recovery, and lock
  cleanup contracts remained green under repeated tests.

- [x] **P3-011 — Freeze canonical values, stable variation identity, schemas, and the exact data source.**

  Scope: implement one type-aware canonicalization layer for Boolean, String,
  Number, and JSON values without `float64`; validate Feature Flag keys, names,
  variation counts/names/values, types, and UUID uniqueness; define deterministic
  provider-created variation IDs and the disabled-safe API-required Create
  seed. Add and register `data.featbit_feature_flag` with required exact
  `environment_id`/`key` and only safe Computed definition fields. Freeze the
  resource schema and replacement/ownership contract in focused schema tests,
  but do not register a resource lifecycle yet.

  Important files: narrow Feature Flag Terraform models and canonicalization
  under `internal/provider`; `feature_flag_data_source.go` and focused tests;
  provider data-source registration and schema/configuration tests. Reuse the
  existing UUID validator, ProviderData checks, and stable computed-ID plan
  modifier where their ownership semantics match.

  Runtime relationship: `featureFlagDataSource.Read ->
  Client.GetFeatureFlag -> exact active result -> type-aware value
  canonicalization + UUID-keyed variation flattening -> Terraform state`.
  Resource planning uses the same canonicalizer and derives deterministic UUIDs
  before a later Create call.

  Done when tests cover exactly four lowercase types; precise boolean, exact
  string, arbitrary-precision number, and one-value JSON normalization;
  invalid/trailing/non-finite input; stable object-key ordering; deterministic
  unique UUIDs including duplicate-looking variations; response reordering;
  missing/duplicate IDs; archived/absent/ambiguous reads; Configure type
  checks; and schema ownership. Protocol schema tests must prove only `name`
  is mutable while environment/key/type/description/variations replace and no
  UI-owned operational field is configurable or exposed.

  Result: Added the provider-owned four-type canonicalization and validation
  layer, deterministic UUID v5 variation identity, disabled-safe Create seed,
  UUID-correlated flattening, and frozen resource schema in
  `feature_flag_models.go` / `feature_flag_schema.go`; added and registered the
  exact active-only `featbit_feature_flag` data source in
  `feature_flag_data_source.go`, and extended the shared stable-ID modifier to
  compare nested lists without changing existing scalar behavior. The runtime
  path is `featureFlagDataSource.Read -> Client.GetFeatureFlag -> active status
  -> type-aware canonicalization + UUID ordering -> Terraform state`; future
  resource planning uses the same planned-definition canonicalizer and fixed
  seed. Focused model/data-source/schema/Protocol tests repeated with
  `-count=10`; `go test ./...`, `go vet ./...`, `go build ./...`, `go mod tidy
  -diff`, `go mod verify`, `gofmt -l .`, and `git diff --check` passed.
  A follow-up library audit replaced regular-expression UUID validation and
  manual UUID case normalization with a strict canonical adapter over
  `github.com/google/uuid`, while retaining the required 8-4-4-4-12 contract;
  UUID tests now reject the library's permissive alternate encodings and
  independently verify frozen RFC UUID v5 variation IDs. UTF-16 name length
  counting now uses `unicode/utf16`.

## Feature Flag resource

- [x] **P3-012 — Add Feature Flag Create, replacement, Import, and recovery.**

  Scope: add and register `featbit_feature_flag`; add the documented one-shot
  Create endpoint; require exact zero across complete active and archived
  collections; construct the minimal deterministic disabled-safe payload with
  stable variation UUIDs; read canonical state after success; add strict
  `<environment_uuid>/<exact_key>` Import; and reconcile ambiguous Create
  without mutation retry or silent adoption. Apply `RequiresReplace` to
  environment, key, type, description, and variations while keeping computed
  IDs known only for in-place name updates.

  Important files: `internal/client/feature_flags.go` and endpoint tests;
  `internal/provider/feature_flag_resource.go`, the models/canonicalizer from
  P3-011, composite Import parsing, provider registration, and focused
  lifecycle/schema tests. Extend a shared helper only if its existing exact
  contract remains unchanged.

  Runtime relationship: `featureFlagResource.Create -> exact active+archived
  zero preflight -> one Client.CreateFeatureFlag -> Client.GetFeatureFlag ->
  canonical UUID-correlated state`; ambiguous Create goes through the same
  complete exact resolver and returns Import/recovery guidance without
  adopting or retrying.

  Done when tests cover invalid environment/key/type/values before transport;
  exact-zero, archived collision, fuzzy non-match, active/archived duplicates,
  one-shot payload and no context headers; all four deterministic Create
  payloads; response/read mismatch; ambiguous recovery zero/one/duplicate;
  read-after-write; strict two-component Import; imported server variation
  IDs; name-only stable ID planning; every replacement input; cancellation;
  and state preservation for each unconfirmed result.

  Result: Added the one-shot documented Feature Flag POST and complete
  active/archived resolver in `internal/client/feature_flags.go`; registered
  `featbit_feature_flag` and implemented exact-zero Create preflight,
  deterministic disabled-safe expansion, provisional/canonical state,
  exact-key Read, strict composite Import, and non-adopting ambiguous recovery
  in `internal/provider/feature_flag_resource.go`. The runtime path is
  `Create -> ResolveFeatureFlag(active+archived complete views) -> one
  CreateFeatureFlag -> GetFeatureFlag -> UUID-correlated canonical state`,
  with ambiguous Create returning recovery guidance after the same complete
  resolver. Focused client/resource/schema contracts repeated with
  `-count=10`; `go test ./...`, `go vet ./...`, `go build ./...`, `gofmt -l
  .`, and `git diff --check` passed.

- [x] **P3-013 — Add name-only Update and archive-plus-hard-delete.**

  Scope: add only the specialized name Update, archive, and permanent-delete
  endpoint methods. Update executes one narrow name mutation followed by exact
  canonical Read. Destroy resolves current active/archived status, archives
  only when needed, hard-deletes, then proves exact zero across every active
  and archived page. Reconcile ambiguous steps without replaying a mutation;
  preserve state unless exact zero is authoritative. Add only the narrow keyed
  serialization demonstrated necessary by this multi-call lifecycle.

  Important files: `internal/client/feature_flags.go` and focused mutation
  contracts; `internal/provider/feature_flag_resource.go`, any narrow lock,
  and lifecycle tests. Do not call generic PATCH, restore, toggle,
  description, variations, targeting, tags, or clone endpoints.

  Runtime relationship: `featureFlagResource.Update -> one name PUT -> exact
  Read -> canonical state`; `Delete -> exact status -> optional one archive
  PUT -> one permanent DELETE -> complete active+archived exact-zero proof`.

  Done when tests prove name-only bodies and no operational-field rewrite;
  read-after-update; externally changed UI-owned fields survive; active,
  already-archived, and already-absent destroy; archive conflict/failure;
  permanent-delete failure; ambiguous mutation reconciliation; exact-zero
  proof across later pages; duplicate/incomplete fail-closed behavior;
  cancellation including any lock wait; one-shot call counts; and redaction-
  safe state preservation/recovery guidance.

  Result: Added the specialized name PUT, bodyless archive PUT, and permanent
  DELETE adapters with UUID/Boolean response checks and one-shot/redaction
  contracts in `internal/client/feature_flags.go`; completed name-only Update,
  ambiguous status reconciliation, archive prerequisite, hard delete, and
  complete final active/archived zero proof in
  `internal/provider/feature_flag_resource.go`. Create/Update/Delete now share
  a cancellation-safe keyed writer at exact Environment UUID plus
  case-sensitive key, while Environment retains its prior serialization
  semantics through the generalized helper. The runtime paths are `Update ->
  one name PUT -> exact canonical Read` and `Delete -> complete status ->
  optional one archive PUT -> one DELETE -> complete exact-zero proof`.
  Focused client/provider/Environment-lock contracts repeated with
  `-count=10`; `go test ./...`, `go vet ./...`, `go build ./...`, `gofmt -l
  .`, and `git diff --check` passed.

- [x] **P3-014 — Prove the four-type Terraform Protocol v6 lifecycle.**

  Scope: exercise the registered Feature Flag resource/data source through
  Protocol v6 against one narrow stateful public-API fixture. Cover Boolean,
  String, Number, and JSON without turning the fixture into a general FeatBit
  emulator. Model active/archived pagination, generic-operation UI state, and
  permanent deletion only as needed by production calls.

  Important files: Feature Flag protocol tests and a focused fixture under
  `internal/provider`. Reuse existing Terraform CLI/provider-server harness
  patterns and test helpers where their semantics match.

  Runtime relationship: `terraform plan/apply/refresh/import/destroy ->
  providerserver Protocol v6 -> Feature Flag resource/data source -> Feature
  Flag adapter -> isolated public-API fixture`.

  Done when every type passes Create, exact data-source Read, Import then empty
  plan, second empty plan, external name drift repair, each replacement input,
  direct-read fallback, response reordering, out-of-band deletion, externally
  archived fail-closed behavior, and archive-plus-delete. The recorder proves
  exact call order, no mutation retry, no forbidden endpoint, UI-owned fields
  unchanged, and exact-zero active/archived teardown.

  Result: Added a narrow stateful public-API fixture and four-type 14-step
  Protocol v6 lifecycle in `feature_flag_protocol_fixture_test.go` and
  `feature_flag_protocol_test.go`. Each Boolean, String, Number, and JSON flag
  now proves Create, exact data-source Read, Import plus empty plan, second
  empty plan, external name repair, description/variations/type/key/environment
  replacement, complete direct-read fallback, response reordering,
  out-of-band deletion, archived fail-closed behavior, and final
  archive-plus-delete. Resource `ModifyPlan` now supplies deterministic IDs on
  Create/replacement, preserves imported server IDs for unchanged definitions,
  and compares variation replacements by type-aware canonical values; resource
  state preserves an equivalent configured spelling where Terraform's Required
  value consistency demands it, while wire comparison and data-source state
  remain canonical. The verified runtime is `Terraform plan/apply/refresh/
  import/destroy -> Protocol v6 -> Feature Flag lifecycle -> public fixture`;
  each recorder observed exactly 7/1/6/6 Create/name/archive/delete mutations,
  no forbidden endpoint or UI-field rewrite, and a final archived last-page
  zero proof. The four-type Protocol suite and `go test ./...` passed; focused
  client/provider contracts repeated with `-count=10`; `go vet ./...`, `go
  build ./...`, `go mod tidy -diff`, `go mod verify`, `gofmt -l .`, and `git
  diff --check` passed.

## Integration and verification

- [x] **P3-030 — Prove cross-resource ownership, state safety, and redaction.**

  Scope: test Feature Flags with their Project/Environment parents and at
  shared safety boundaries: replacement identity planning, canonical variation
  correlation, Null/Unknown/default behavior, ambiguous state preservation,
  Import diagnostics, and secret/runtime-value redaction. Confirm an
  Environment can be replaced or deleted without the Feature Flag lifecycle
  claiming Project/Environment fields or UI-owned Feature Flag operations.

  Important files: focused integration tests in `internal/provider`, endpoint
  redaction tests in `internal/client`, and production fixes only where a
  failing contract demonstrates a concrete issue.

  Runtime relationship: `Project -> Environment -> Feature Flag protocol
  dependency graph -> lifecycle-owned adapters -> canonical state`, while
  failures flow through `Client.DecodeResponse -> redacted APIError ->
  lifecycle diagnostics/tflog`.

  Done when parent/child dependency order and replacement converge; arbitrary
  page/variation ordering and Null/Unknown values do not cause permanent diffs;
  active/archived duplicates preserve state; and injected tokens, keys, UUIDs,
  variation values, tags, targeting content, tenant/server details, paths, and
  raw bodies appear in neither state, diagnostics, logs, fixtures, nor
  assertion output.

  Result: Added the combined Project/Environment/Feature Flag Protocol v6
  ownership fixture in `project_environment_feature_flag_integration_test.go`,
  arbitrary collection ordering in `feature_flag_protocol_fixture_test.go`,
  and expanded Feature Flag endpoint redaction contracts in
  `internal/client/feature_flags_test.go`. The verified runtime is `Project ->
  Environment -> Feature Flag -> lifecycle-owned adapters`, with dependent
  Unknown IDs resolving on Create, exact child-before-parent archive/delete on
  Environment replacement and removal, canonical UUID-correlated variations,
  default empty descriptions, UI-owned operation preservation, and
  active/archived duplicate state preservation; failure details flow through
  `DecodeResponse -> featureFlagReadError -> redacted diagnostics`. Both full
  `internal/client` and `internal/provider` packages passed once, and the new
  focused client/provider contracts passed with `-count=10`; `gofmt` and `git
  diff --check` passed.

- [x] **P3-031 — Run trusted current-Cloud acceptance and exact cleanup.**

  Scope: with credentials supplied only out of band, run uniquely prefixed
  Feature Flags of all four types against the documented current Cloud API.
  Create only under test-owned Project/Environment parents, register each
  identity before mutation, and clean flags before environments/projects even
  after failure. Snapshot and restore any test-owned UI changes needed to prove
  preservation; never inspect or modify unrelated objects.

  Important files: credential-gated acceptance tests and their in-memory
  cleanup inventory under `internal/provider`. Reuse the existing Cloud
  Project/Environment harness only where it preserves its cleanup/redaction
  guarantees. Permanent CI wiring remains Phase 6 work.

  Runtime relationship: `terraform-plugin-testing -> local Protocol v6
  provider -> shared client -> documented FeatBit Cloud endpoints -> exact
  active+archived cleanup verification`.

  Done when all four types pass Create, exact data source, Import followed by
  empty plan, second-plan idempotence, name drift repair, replacement,
  out-of-band deletion, UI-field preservation, and destroy. Independent final
  complete active/archived Feature Flag queries plus Project collection
  verification return exact zero for every unique test key, with no pending
  cleanup owner/action and no persisted runtime identity or value.

  Result: Added the credential-gated four-type Cloud lifecycle and its
  child-first exact cleanup inventory in
  `internal/provider/feature_flag_cloud_acceptance_test.go` and
  `internal/provider/feature_flag_cloud_harness_test.go`. The verified runtime
  is `terraform-plugin-testing -> local Protocol v6 provider -> shared client
  -> documented Cloud API`, covering Create, exact data sources, four Imports
  plus empty plans, second-plan idempotence, external name drift repair,
  name-only UI-owned-field fingerprint preservation, description replacement,
  out-of-band deletion/recreation, and destroy. Current Cloud response
  reordering exposed and fixed first-Import correlation for provider-generated
  deterministic variation UUIDs while externally created flags retain stable
  UUID ordering; ambiguous successful name mutations remained one-shot and
  converged through complete active/archived reconciliation. The focused
  Import, cleanup, and UI-fingerprint contracts passed with `-count=10`; the
  complete Cloud acceptance passed, and independent active/archived Flag plus
  Project/Environment collection checks proved exact zero after child-first
  cleanup without persisting runtime identities or values.

- [x] **P3-032 — Run the complete Phase 3 local gate.**

  Scope: run formatting, vet, all unit/mock/protocol tests, repeated endpoint
  and normalization contracts, Windows race tests with the verified compiler
  toolchain, build, module tidy/verify, dependency inspection, diff checks,
  local provider override, schema JSON assertions, and repository
  secret/redaction scans.

  Important files: production/test fixes required by the gate, this active
  TODO, and the Phase 3 README next-task pointer. Do not add Phase 6 CI,
  release/Registry work, generated clients, or generator dependencies.

  Runtime relationship: verify the complete `Terraform Core -> Protocol v6
  provider -> Project/Environment/Feature Flag lifecycle -> shared client ->
  public API contract` and confirm no other local runtime path exists.

  Done when `gofmt -l .` is empty; `go vet ./...`, `go test ./...`, repeated
  focused tests, `go test -race ./...`, `go build ./...`, `go mod tidy -diff`,
  `go mod verify`, dependency scans, and `git diff --check` pass. The local
  override loads; schema JSON asserts five provider attributes plus exactly
  three resources/three data sources and every visible ownership/Sensitive
  flag; focused Go tests assert all replacement/normalization contracts; and
  repository scans find no real secret, runtime Cloud identity, variation
  value, forbidden endpoint, or risky state artifact.

  Result: Verified the complete `Terraform Core -> Protocol v6 provider ->
  Project/Environment/Feature Flag lifecycle -> shared client -> documented
  public API` path. `gofmt -l .`, `git diff --check`, `go vet ./...`, `go test
  ./...`, endpoint and normalization/replacement contracts with `-count=10`,
  Windows `go test -race ./...` using a checksum-verified temporary MinGW-w64
  toolchain, `go build ./...`, `go mod tidy -diff`, and `go mod verify` all
  passed. The dependency scan exposed reachable grpc, x/net, and x/text
  advisories; `go.mod`/`go.sum` now select their compatible fixed dependency
  graph, after which `govulncheck ./...` reported no vulnerabilities, all
  selected modules verified, no selected version was retracted, and all seven
  direct modules had an inspectable license or notice. A credential-free local
  development override loaded the built provider and schema JSON proved five
  provider attributes, exactly three resources, exactly three data sources,
  and all 51 visible ownership/Sensitive flags. Final scans found no real
  secret, runtime Cloud identity/value, forbidden production endpoint/context
  write/database path, Terraform state, crash log, binary, or pending temporary
  toolchain/config artifact.

## Phase exit

- [ ] **P3-090 — Close Phase 3 and prepare Phase 4.**

  Confirm every item above and the README exit gate. Update the master plan
  only with still-current runtime/roadmap facts and make its next action the
  first concrete Segment task. Then delete this completed Phase 3 package and
  create only a Phase 4 `README.md` and detailed `todo.md`.

- [ ] **P3-091 — Declare Phase 3 complete only after the exit gate passes.**

  This final consistency check must find no unchecked item, unresolved Feature
  Flag runtime assumption, UI-ownership violation, canonicalization gap,
  secret/value finding, failed lifecycle or Import convergence, pending active
  or archived Cloud object, missing schema verification, or absent Phase 4
  entry point.
