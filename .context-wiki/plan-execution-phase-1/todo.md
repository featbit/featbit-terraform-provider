# Phase 1 TODO — Provider foundation

Status: **In progress**
Next: **P1-044**

Work one item at a time. Before checking an item, add a concise `Result` under
it containing:

- the important files changed;
- the runtime call relationship introduced or verified; and
- the exact verification that passed.

Do not create a separate ADR, evidence file, session log, or handoff.

## Shared client tests

- [x] **P1-040 — Complete request and error-contract tests.**

  Exercise the shared client through an `httptest` server. Cover the normalized
  `/api/v1` path, direct `Authorization`, User-Agent, removal of unsupported
  context headers, success envelopes, HTTP `2xx` with `success=false`,
  validation `400/422`, `401`, `403`, unconfirmed `404`, `409`, `429`, `5xx`,
  and malformed envelopes. Assertions must not print credentials or bodies.

  Done when the tests prove both runtime relationships: `provider.Configure ->
  client.New` constructs the shared client without a request; a synthetic
  endpoint request then calls `Client.Do -> authorizationTransport`, followed
  by `Client.DecodeResponse` for envelope/error classification.

  Result (2026-08-02): Added `internal/client/request_contract_test.go` and
  extended `internal/provider/provider_configuration_test.go`. The provider
  test proves `FeatBitProvider.Configure -> client.New` normalizes the API root,
  constructs one client shared by resources and data sources, and performs no
  request. The `httptest` contracts prove `Client.Do -> authorizationTransport
  -> HTTP server -> Client.DecodeResponse`, including the required headers,
  `/api/v1` boundary, success data, every listed HTTP classification, and
  malformed envelopes without assertion output containing credentials or raw
  bodies. Passed `gofmt -l .`, `go vet ./...`, `git diff --check`, `go test
  ./...`, `go test ./internal/client -run
  'TestClient(RequestAndSuccessEnvelopeContract|ErrorEnvelopeContract)$'
  -count=20`, and `go test ./internal/provider -run
  '^TestProviderConfigureConstructsSharedClientWithoutRequest$' -count=20`.

- [x] **P1-041 — Complete cancellation and response-boundary tests.**

  Cover cancellation before concurrency admission, during HTTP execution, and
  during retry wait; client timeout; nil requests/responses; body read failure;
  exactly-16-MiB response acceptance; oversized response rejection; and body
  closure on every success/failure path.

  Done when no path can hang, leak a permit/body, or return unsafe transport
  details.

  Result (2026-08-02): Added `internal/client/boundary_contract_test.go`.
  The contracts verify `Client.Do -> requestLimiter.acquire -> http.Client.Do
  -> readBoundedResponse -> requestLimiter.release`, cancellation before
  admission, during HTTP, and during retry wait, plus
  `Client.DecodeResponse -> readBoundedResponse` closure on every result.
  Nil inputs are safe, timeout sentinels survive, exactly 16 MiB is accepted,
  one extra byte is rejected, and failures neither leak bodies/permits nor
  expose transport details. Passed `go test ./internal/client -run
  '^(TestClientCancellationBeforeConcurrencyAdmission|TestClientCancellationDuringHTTPExecution|TestClientCancellationDuringRetryWait|TestClientTimeoutDuringHTTPExecution|TestClientNilRequestAndResponseContracts|TestClientResponseSizeBoundaryAndClosure|TestClientBodyReadFailureIsClosedSafeAndRecoverable|TestDecodeResponseClosesBodyOnEveryPath)$'
  -count=5`.

- [x] **P1-042 — Complete retry-policy tests.**

  Prove that only bodyless `GET` requests retry `429`, transient `5xx`, timeout,
  and network failure. Verify retry count, exponential backoff/jitter bounds,
  valid/invalid `Retry-After`, cancellation, and that POST/PUT/PATCH/DELETE and
  GET-with-body execute once.

  Done when an unsafe mutation cannot be retried by any classifier result.

  Result (2026-08-02): Added `internal/client/retry_contract_test.go`.
  Integration contracts verify `Client.Do -> Classify -> ShouldRetry ->
  retryController` retries only bodyless GET requests for `429`, transient
  `5xx`, timeout, and network failures. Every mutation method and GET with a
  body executes once for each retryable classifier; configured attempt counts,
  cancellation, response closure, exponential bases, jitter limits, and valid
  delta/date plus invalid `Retry-After` handling are proven. Passed `go test
  ./internal/client -run
  '^(TestClientRetriesBodylessGETForRetryableFailures|TestClientNeverRetriesMutationsOrGETWithBody|TestShouldRetryRejectsEveryClassificationForMutations|TestClientHonorsConfiguredRetryCount|TestRetryControllerExponentialBackoffAndJitterBounds|TestParseRetryAfterContract|TestRetryControllerUsesRetryAfterOrBackoff|TestClientCancellationStopsRetryAttempts)$'
  -count=10`.

- [x] **P1-043 — Complete concurrency and redaction tests.**

  Saturate the request limiter, verify the configured maximum, cancellation
  while queued, permit release before retry wait, and progress after failures.
  Inject markers shaped like tokens, secrets, emails, UUIDs, keys, paths,
  query values, headers, envelope messages, and network errors through every
  exported formatter/diagnostic path.

  Done when concurrency remains bounded and no marker appears in returned
  errors, formatted values, request metadata, or captured logs.

  Result (2026-08-02): Added
  `internal/client/concurrency_redaction_contract_test.go` and tightened
  `internal/client/redaction.go`. Contracts verify `Client.Do ->
  requestLimiter` never exceeds its configured maximum, queued cancellation
  never reaches transport, permits are released before retry waits, and later
  requests progress after every tested failure. `Client.Do ->
  Redactor.Request`, `Redactor.Text/Headers/Request`, `DecodeResponse ->
  APIError`, every formatter verb, returned request metadata, unsafe network
  errors, and captured provider logs were exercised with synthetic markers.
  Diagnostic request copies now redact every header/trailer value and clear
  transfer, remote-address, TLS, response, body, form, URL identity, query, and
  context metadata. Passed `go test ./internal/client -run
  '^(TestClientEnforcesConfiguredMaximumConcurrency|TestClientCancelsRequestWhileQueuedForPermit|TestClientReleasesPermitBeforeRetryWait|TestClientLimiterProgressesAfterFailures|TestRedactorTextAndHeadersRemoveRuntimeMarkers|TestRedactorRequestRemovesAllUnsafeMetadata|TestClientErrorsAndResponseMetadataAreRedacted|TestClientRedactedValuesRemainSafeInCapturedLogs)$'
  -count=10 -timeout=60s`. Final combined verification also passed `gofmt -l
  .`, `go vet ./...`, `git diff --check`, `go test -count=1 ./...`, and `go
  test ./internal/client -count=10 -timeout=120s`. A follow-up test-only
  refactor centralized the client, request, response, tracked-body, APIError,
  and bounded-wait fixtures used across P1-040 through P1-043 in
  `internal/client/test_helpers_test.go`; the same combined verification passed
  again with unchanged coverage and runtime behavior.

## Reproducibility

- [ ] **P1-044 — Prove the runtime dependency boundary.**

  Run `go list -deps ./...` and inspect imports. The root module must contain no
  generated API package, OpenAPI runtime, generator command, nested tool
  module, or endpoint-specific model without a production caller.

- [ ] **P1-045 — Run the complete local quality gate.**

  Run `gofmt -l .`, `go vet ./...`, `go test ./...`, repeated focused client
  tests, `go test -race ./...`, `go build ./...`, `go mod tidy` clean-diff,
  `go mod verify`, and `git diff --check`. If the Windows race run still lacks
  a C compiler, install/provide one or make the same gate mandatory in CI; do
  not mark this item complete with neither result.

## Developer workflow

- [ ] **P1-050 — Add standard developer commands.**

  Add `GNUmakefile` targets `fmt`, `lint`, `test`, `testacc`, and `build` that
  call repository-pinned behavior and work from the repository root. Do not add
  an API-client generation target.

- [ ] **P1-051 — Add fork-safe CI and pinned quality tools.**

  Add pull-request jobs for formatting, vet, race tests, build, lint, and
  vulnerability checks. Pin tool versions in the consuming workflow. Fork jobs
  must use no FeatBit credentials; live acceptance jobs must be separately
  trusted and scoped.

- [ ] **P1-052 — Document a local provider override.**

  Add a short developer guide outside `.context-wiki` showing build,
  `dev_overrides`, provider address, safe environment-variable setup, and the
  commands needed to validate a minimal provider configuration. Do not include
  real tokens or state.

- [ ] **P1-053 — Load the provider through the local override.**

  Build the binary with a visible development version, configure an isolated
  CLI override, and prove Terraform starts the local Protocol v6 provider. Use
  synthetic configuration where no API call is needed.

- [ ] **P1-054 — Verify the Terraform-visible schema.**

  Run `terraform providers schema -json` through the local override. Confirm
  the five provider attribute types/optionality, the Sensitive access token,
  and the intentional absence of resources and data sources. Defaults and
  custom validation remain verified by Go tests because Terraform's schema
  JSON does not expose their complete runtime behavior.

- [ ] **P1-055 — Run the repository secret and redaction gate.**

  Scan tracked and untracked repository content with the selected pinned CI
  scanner plus focused marker tests. The result must contain zero real
  credentials, secret values, tenant identifiers, or unsafe fixtures.

## Phase exit

- [ ] **P1-056 — Close Phase 1 and prepare Phase 2.**

  Confirm every item above and the README exit gate. Update the master plan
  with only the final current runtime state and make its next action the first
  concrete Project/Environment task. Then delete this completed Phase 1
  package and create only a Phase 2 `README.md` and detailed `todo.md`.

- [ ] **P1-057 — Declare Phase 1 complete only after the exit gate passes.**

  This is the final consistency check: no unchecked item, unresolved runtime
  assumption, secret finding, failed local-provider load, or missing schema
  verification may remain.
