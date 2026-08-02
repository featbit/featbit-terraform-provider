# Phase 2 TODO — Project and environment

Status: **In progress**
Next: **P2-020**

Work one item at a time. Before checking an item, add a concise `Result` under
it containing:

- the important files changed;
- the runtime call relationship introduced or verified; and
- the exact verification that passed.

Do not create a separate ADR, evidence file, findings log, session log, or
handoff.

## Project

- [x] **P2-010 — Add the Project read adapter and exact data source.**

  Scope: add only the safe Project/environment wire fields needed by this
  caller; implement documented Project Get and complete-collection List;
  compose direct Read with exact-ID zero/one/duplicate resolution; add and
  register `data.featbit_project` with required UUID `id` and Computed `name`,
  `key`, and canonical `environments`. Exact ID is the only selector.

  Important files: `internal/client/projects.go` and focused tests;
  `internal/provider/project_data_source.go`, its Terraform model/flattening,
  provider registration, and schema/configuration tests. Keep Project endpoint
  types out of unrelated shared-client files.

  Runtime relationship: `provider.DataSources -> projectDataSource.Configure
  -> *client.Client`, then `projectDataSource.Read -> Client.GetProject`. A
  non-success direct Read calls `Client.ListProjects -> exact UUID resolver`
  before returning canonical state or a safe diagnostic.

  Done when mock contracts prove method, escaped path, no unexpected
  query/context headers, envelope/error/cancellation behavior, complete
  collection handling, exact zero/one/duplicate outcomes, stable environment
  ordering, and omission of environment secrets. Provider tests prove the
  client is type-checked at Configure and no request occurs before Read.

  Result: Added the narrow Project/environment wire model and direct-Get plus
  complete-List exact-ID fallback in `internal/client/projects.go`, backed by
  reusable escaped-request and UUID helpers in `request.go` and
  `identifiers.go`; the registered exact-UUID data source, shared flattening,
  validator/schema, and reusable ProviderData checks live in
  `internal/provider/project_data_source.go`, `project_models.go`, `uuid.go`,
  and `client_configuration.go`. Runtime is `Provider.DataSources ->
  projectDataSource.Configure -> *client.Client`, then
  `projectDataSource.Read -> Client.GetProject -> Client.Do/DecodeResponse`,
  with failed direct reads resolving exact zero/one/duplicate IDs through
  `Client.ListProjects`. `gofmt -l internal/client internal/provider` was
  empty; `go test ./internal/client ./internal/provider` and
  `git diff --check` passed, including focused method/path/header,
  envelope/cancellation, complete-collection, ordering, secret-omission, and
  Configure-without-request contracts.

- [x] **P2-011 — Add the Project resource, CRUD, and Import.**

  Scope: add and register `featbit_project` with Computed `id`, Required
  `name`/`key`, `RequiresReplace` on `key`, and the same fully Computed
  non-owning `environments` shape. Add Create/Update/Delete endpoint methods,
  `<project_uuid>` Import parsing, exact-key Create preflight, ambiguous-Create
  reconciliation, canonical Read-after-write, and exact-ID destroy/absence
  proof. Do not add a Project write lock.

  Important files: `internal/client/projects.go` and tests;
  `internal/provider/project_resource.go`, Project models/expand/flatten,
  concrete UUID Import parsing, provider registration, and lifecycle tests.

  Runtime relationship: `projectResource.Create/Update/Delete -> Project
  endpoint method -> Client.Do -> Client.DecodeResponse -> projectResource.Read
  -> canonical state`. `Read/Delete -> direct Get -> complete collection exact
  fallback` decides whether state is preserved or removed.

  Done when tests cover exact-zero key preflight, fuzzy non-matches, duplicate
  keys/IDs, one-shot mutations, safe Import/recovery guidance after ambiguous
  Create, name-only Update bodies, key replacement schema, automatic
  environment canonicalization, authoritative versus ambiguous absence,
  already-absent Delete, cancellation, and state preservation for every
  unconfirmed outcome.

  Result: Extended `internal/client/projects.go` with exact POST/PUT/DELETE
  request and response contracts using the shared JSON request constructor,
  and added the registered
  `internal/provider/project_resource.go` lifecycle with exact-key preflight,
  ambiguous-Create reconciliation, strict UUID Import, canonical
  read-after-write, `key` replacement, and exact destroy proof; focused client
  and lifecycle coverage lives in `projects_test.go` and
  `project_resource_test.go`. Runtime is `projectResource.Create/Update/Delete
  -> one Project mutation -> Client.Do/DecodeResponse -> GetProject`, while
  failed direct Read/Delete confirmation falls back through the complete
  collection and preserves state unless exact zero is proven. `go test
  -count=5 ./internal/client ./internal/provider`, `go vet ./...`, `go test
  ./...`, and `git diff --check` passed, covering exact/fuzzy/duplicate
  preflight, one-shot mutations, safe ambiguous recovery, name-only update,
  replacement, canonical environments, Import validation, cancellation,
  already-absent deletion, and ambiguous-state preservation.

- [x] **P2-012 — Prove the Project Terraform protocol lifecycle.**

  Scope: exercise the registered Project resource and data source through
  Protocol v6 against an isolated stateful mock API, rather than calling
  lifecycle methods directly.

  Important files: Project acceptance/protocol tests under
  `internal/provider` and a narrow synthetic Project API fixture owned by those
  tests. Do not turn the fixture into a general API emulator.

  Runtime relationship: `terraform plan/apply/refresh/import/destroy ->
  providerserver Protocol v6 -> project resource/data source -> Project
  adapter -> mock public API`.

  Done when Create/Read/Update/Delete, exact data-source Read, Import, second
  empty plan, external name drift, key replacement, direct-Read fallback, and
  out-of-band deletion all pass; the request recorder proves expected call
  order and zero unsafe retries; teardown leaves exact project count zero.

  Result: Added `internal/provider/project_protocol_test.go` and the narrow
  stateful `project_protocol_fixture_test.go`, which drive the registered
  resource and exact data source through Terraform CLI and the in-process
  Protocol v6 server. Runtime is `terraform plan/apply/refresh/import/destroy
  -> providerserver Protocol v6 -> Project resource/data source -> Project
  adapter -> isolated public-API fixture`; its recorder validates request
  shapes, exact mutation order, read-after-write, and no mutation retry.
  `go test ./internal/provider -run '^TestProjectProtocolLifecycle$'
  -count=3`, `go vet ./...`, `go test ./...`, `go build ./...`, `go mod tidy
  -diff`, `go mod verify`, `gofmt -l .`, and `git diff --check` passed. The
  protocol suite proved CRUD, exact data-source Read, Import, an empty second
  plan, external name drift repair, key replacement, direct-Read fallback,
  out-of-band deletion recovery, and exact-zero teardown.

## Environment

- [ ] **P2-020 — Add the Environment read adapter and exact data source.**

  Scope: add only the safe Environment wire fields needed by Phase 2;
  implement documented exact Get and parent-Project exact-ID fallback; add and
  register `data.featbit_environment` with required UUID `project_id` and
  `id`, plus Computed `name`, `key`, and `description`. Exact IDs are the only
  selectors.

  Important files: `internal/client/environments.go` and focused tests;
  `internal/provider/environment_data_source.go`, its Terraform
  model/flattening, provider registration, and schema/configuration tests.

  Runtime relationship: `environmentDataSource.Read ->
  Client.GetEnvironment`; a non-success direct Read calls
  `Client.GetProject -> exact environment UUID resolver` before returning
  canonical state or a safe diagnostic.

  Done when mock contracts prove both UUIDs are validated and escaped, direct
  and parent paths are exact, zero/one/duplicate parent membership behaves
  fail-closed, missing descriptions canonicalize to `""`, and injected secret
  values never enter data-source state, errors, formatted values, or captured
  logs.

- [ ] **P2-021 — Add the Environment resource, CRUD, Import, and safe Update.**

  Scope: add and register `featbit_environment` with Computed `id`, Required
  `project_id`/`name`/`key`, Optional canonical `description`, and
  `RequiresReplace` on `project_id` and `key`. Add Create/Update/Delete
  endpoint methods, exact parent/key preflight, composite Import parsing,
  canonical Read-after-write, and parent-collection absence proof.

  Read the current Environment immediately before Update and pass its
  `settings` through unchanged while changing only `name` and `description`.
  Add the smallest per-environment serialization needed around that
  read/write/read relationship. Settings and secret values stay outside the
  Terraform model and ordinary state.

  Important files: `internal/client/environments.go` and tests;
  `internal/provider/environment_resource.go`, Environment models,
  expand/flatten, `<project_uuid>/<environment_uuid>` parsing, the narrow
  environment lock, provider registration, and lifecycle tests.

  Runtime relationship: `environmentResource.Update -> per-environment lock
  -> GetEnvironment -> UpdateEnvironment(current settings) -> Read -> unlock`.
  Create and Delete call one mutation once, then use direct Read plus exact
  parent membership to converge or preserve state.

  Done when tests cover missing/wrong parent, exact-zero scoped key preflight,
  fuzzy/duplicate keys and IDs, ambiguous Create recovery, one-shot
  mutations, name/description Update, unchanged UI-owned settings, no secret
  round-trip, parent/key replacement schema, strict two-UUID Import, direct
  `403` after Delete with exact-parent zero, already-absent Delete,
  cancellation while waiting for the lock, and safe progress after failure.

- [ ] **P2-022 — Prove the Environment Terraform protocol lifecycle.**

  Scope: exercise the registered Environment resource/data source through
  Protocol v6 against an isolated stateful mock API, including coexistence
  with its parent Project resource.

  Important files: Environment and cross-resource acceptance/protocol tests
  under `internal/provider` plus only the Project/Environment behavior needed
  by their synthetic fixture.

  Runtime relationship: `terraform plan/apply/refresh/import/destroy ->
  project dependency -> environment resource/data source -> Environment and
  Project fallback adapters -> mock public API`.

  Done when CRUD, exact data-source Read, two-component Import, second empty
  plan, external name/description drift, key and parent replacement,
  settings-preserving Update, direct-Read fallback, project-delete cascade,
  environment out-of-band deletion, and dependency-ordered cleanup pass.
  Teardown proves exact environment and project counts zero.

## Integration and verification

- [ ] **P2-030 — Prove canonical ownership, state safety, and redaction.**

  Scope: test the two resource families together at every shared boundary:
  Project's Computed environment observation versus standalone Environment
  ownership; stable ordering/default normalization; zero/one/duplicate exact
  resolution; ambiguous state preservation; Import diagnostics; and secret,
  token, path, key, UUID, and server-detail redaction.

  Important files: focused integration tests in `internal/provider` and
  endpoint redaction tests in `internal/client`. Tighten production code only
  where a failing contract demonstrates a concrete leak or convergence issue.

  Runtime relationship: `Project/Environment API responses -> endpoint wire
  models -> flatten -> Terraform state`, while failures flow through
  `Client.DecodeResponse -> lifecycle diagnostics/tflog`.

  Done when a standalone environment can coexist with the parent project's
  Computed list without a competing write or permanent diff; arbitrary API
  ordering converges; Null/Unknown/default cases are stable; and injected
  environment secret values or runtime identities appear in neither state,
  diagnostics, logs, fixtures, nor assertion output.

- [ ] **P2-031 — Run trusted current-Cloud acceptance and exact cleanup.**

  Scope: with credentials supplied only out of band, run uniquely prefixed
  Project/Environment resource and data-source tests against the documented
  current Cloud API. Register every created identity immediately and clean in
  child-before-parent order even after failure. Do not persist runtime IDs,
  keys, tenant data, tokens, secret values, or raw bodies.

  Important files: credential-gated acceptance tests and their in-memory
  cleanup inventory under `internal/provider`. Permanent CI wiring remains
  Phase 6 work.

  Runtime relationship: `terraform-plugin-testing -> local Protocol v6
  provider -> shared client -> documented FeatBit Cloud endpoints -> exact
  cleanup verification`.

  Done when resource CRUD, exact data sources, Import followed by an empty
  plan, second-plan idempotence, drift, key/parent replacement, UI-setting
  preservation, out-of-band deletion, project cascade, and destroy pass.
  Independent final Project collection verification returns exact zero for
  every test key, with no pending cleanup owner/action.

- [ ] **P2-032 — Run the complete Phase 2 local gate.**

  Scope: run formatting, vet, all unit/mock/protocol tests, repeated endpoint
  contracts, Windows race tests with the verified compiler toolchain, build,
  module tidy/verify, dependency inspection, diff checks, local provider
  override, schema JSON assertions, and repository secret/redaction scans.

  Important files: production/test fixes required by the gate, this active
  TODO, and the Phase 2 README next-task pointer. Do not add Phase 6 CI,
  release, Registry documentation, generated clients, or generator
  dependencies.

  Runtime relationship: verify the complete `Terraform Core -> Protocol v6
  provider -> Project/Environment lifecycle -> shared client -> public API
  contract` and confirm no other local runtime path exists.

  Done when `gofmt -l .` is empty; `go vet ./...`, `go test ./...`, repeated
  focused tests, `go test -race ./...`, `go build ./...`, `go mod tidy -diff`,
  `go mod verify`, dependency scans, and `git diff --check` pass. The local
  override loads, schema JSON asserts five provider attributes plus exactly
  two resources/two data sources and their visible types/optionality/Sensitive
  flags, focused Go schema tests assert `RequiresReplace`, and the repository
  scan finds no real secret or runtime identity.

## Phase exit

- [ ] **P2-090 — Close Phase 2 and prepare Phase 3.**

  Confirm every item above and the README exit gate. Update the master plan
  only with still-current runtime/roadmap facts and make its next action the
  first concrete Feature Flag task. Then delete this completed Phase 2 package
  and create only a Phase 3 `README.md` and detailed `todo.md`.

- [ ] **P2-091 — Declare Phase 2 complete only after the exit gate passes.**

  This final consistency check must find no unchecked item, unresolved
  Project/Environment runtime assumption, secret finding, failed lifecycle or
  Import convergence, pending remote object, missing schema verification, or
  absent Phase 3 entry point.
