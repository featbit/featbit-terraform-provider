# Phase 1 TODO — Repository and Provider Scaffold

Status: **In progress — provider configuration complete through P1-020**

Check an item only when its stated result exists. Record concise verification
under the item instead of creating a new context file by default.

## A. Baseline and decisions

- [x] **P1-001** Read the Phase 0 handoff and all five Accepted ADRs; confirm the exit gate still passes.
  Verification (2026-08-01): reviewed the handoff and ADR-001 through
  ADR-005 with no contradiction or unresolved Phase 1 assumption. Re-ran the
  Phase 0 local gate: `gofmt`, `go test ./...`, `go vet ./...`, the 20-run
  normalization/probe suite, deterministic OpenAPI check, context/evidence
  check, repository secret scan, and `git diff --check` all passed. The
  context check reported 76/76 TODOs, five Accepted ADRs, and zero findings;
  the secret scan reported 70 files and zero findings. No live API request or
  credential read was used. The Phase 0 exit gate remains passed.
- [x] **P1-002** Inspect `git status` and preserve every pre-existing change.
  Verification (2026-08-01): baseline HEAD was
  `f685852fc50bbae94cf563acbbd1ed98d7dc6070` on `main`, aligned with
  `origin/main`; tracked changes, staged changes, and untracked files were all
  empty. No stash, reset, clean, checkout, or unrelated edit was performed.
- [x] **P1-003** Record installed Go/Terraform/tool versions and gaps against ADR-005.
  Verification (2026-08-01, Windows/amd64): Go is `1.19.4` with CGO enabled,
  below the `1.25.8` module baseline and both CI lines. Terraform is `1.5.6`,
  which meets the `1.0.0` minimum but is below the `1.15.8` current test line.
  Git is `2.36.0.windows.1`. GNU Make, GCC, Clang, `golangci-lint`,
  `govulncheck`, `oapi-codegen`, `tfplugindocs`, GoReleaser, and GPG are not
  installed. Framework `v1.19.0`, Plugin Go `v0.31.0`, Plugin Testing
  `v1.16.0`, Plugin Log `v0.10.0`, Plugin Docs `v0.25.0`, and
  `oapi-codegen v2.8.0` remain module/tool pins rather than global installs.
  Upgrade Go before P1-010, provide a C compiler for the race gate, and use
  the future pinned tools module/CI for missing executables.
  Update (2026-08-01): the official Windows/amd64 Go `1.26.5` archive was
  installed at user scope after SHA-256 verification. All P1-010 through
  P1-014 commands used that binary with `GOTOOLCHAIN=local`. The machine-wide
  `go` command remains `1.19.4` because the non-elevated MSI upgrade could not
  complete; the missing C compiler/race-suite gap remains assigned to P1-045
  and CI.
- [x] **P1-004** Resolve MPL-2.0 versus MIT before adding scaffold-derived files.
  Decision (2026-08-01): license this provider under MPL-2.0. The
  [pinned HashiCorp scaffold](https://github.com/hashicorp/terraform-provider-scaffolding-framework/blob/f781750309d9d63c50f9d6992a788fa7245ec7fc/LICENSE)
  is MPL-2.0. The separate [FeatBit server repository](https://github.com/featbit/featbit/blob/main/LICENSE)
  is MIT, but no accepted requirement mandates the same license here. Using
  MPL-2.0 avoids a mixed-license scaffold baseline. P1-011 will add the full
  license and appropriate notices; copied or modified upstream files retain
  their applicable copyright notices. A future MIT mandate requires a
  separate provenance/license review before relicensing.
- [x] **P1-005** Confirm the final Phase 1 package/module layout before generating code.
  Verification (2026-08-01): the root runtime module, separate `tools/`
  module, retained Phase 0 probe module, provider package, handwritten client,
  generated client, and pinned OpenAPI input boundaries are fixed in
  [plan.md](plan.md#confirmed-phase-1-module-and-package-layout). No generated
  code or scaffold source was added during this decision task.

## B. Repository and provider scaffold

- [x] **P1-010** Normalize the root Go module and pin direct dependencies.
  Verification (2026-08-01): root module
  `github.com/featbit/terraform-provider-featbit` uses `go 1.25.8` and pins
  Framework `v1.19.0`, Plugin Go `v0.31.0`, Plugin Log `v0.10.0`, and Plugin
  Testing `v1.16.0`. Go `1.26.5` produced `go.sum`; `go mod tidy` and
  `go mod verify` passed.
- [x] **P1-011** Add license, editor settings, and provider-safe `.gitignore` rules.
  Verification (2026-08-01): added the full MPL-2.0 `LICENSE`, source SPDX and
  retained scaffold copyright notices, `.editorconfig`, and ignore rules for
  credentials, Go artifacts, Terraform state/plans, local CLI configuration,
  editor files, and the existing Phase 0 cleanup inventory.
- [x] **P1-012** Add version injection and a Protocol v6 provider server entry point.
  Verification (2026-08-01): `main.go` serves
  `registry.terraform.io/featbit/featbit`, accepts the Framework debug flag,
  and exposes linker-injected `main.version`. A temporary build with
  `-X main.version=p1-014` contained the injected value; the Registry manifest
  declares protocol `6.0`, and the provider factory test creates a Protocol v6
  server.
- [x] **P1-013** Add the minimal provider implementation and schema registration.
  Verification (2026-08-01): the provider reports type `featbit` and its build
  version, exposes an intentionally empty Phase 1 configuration schema, logs
  only a constant Configure message, and registers no resources or data
  sources. Metadata, schema, registration, and Protocol v6 tests pass.
- [x] **P1-014** Prove the provider binary builds without importing the Phase 0 probe module.
  Verification (2026-08-01): `go test ./...`, `go vet ./...`, and
  `go build ./...` passed under Go `1.26.5`. `go list -deps ./...` contained no
  `github.com/featbit/terraform-provider-featbit/tools/api-probe` dependency;
  the retained probe remains an independent nested module.

## C. Provider configuration

- [x] **P1-020** Implement `api_url` plus `FEATBIT_API_URL` fallback and validation.
  Verification (2026-08-01): added the optional `api_url` provider schema
  attribute, explicit-value/`FEATBIT_API_URL`/Cloud-default precedence, and a
  shared validator/runtime parser. HTTP and HTTPS origins or `/api/v1` roots
  are accepted and normalized to `/api/v1`; credentials, query parameters,
  fragments, invalid ports, whitespace, relative URLs, and other paths are
  rejected without echoing the configured URL. Focused precedence and URL
  tests pass. Under the verified user-scoped Go `1.26.5`, the formatting check
  returned no files and `go test ./...`, `go vet ./...`, and `go build ./...`
  passed. Protocol-level aggregate configuration coverage remains assigned to
  P1-024.
- [ ] **P1-021** Implement Sensitive `access_token` plus `FEATBIT_ACCESS_TOKEN` fallback.
- [ ] **P1-022** Implement bounded timeout, concurrency, and retry configuration.
- [ ] **P1-023** Configure the client with direct `Authorization` and no login/context-header flow.
- [ ] **P1-024** Test explicit values, environment fallbacks, Null/Unknown, defaults, and invalid configuration.
- [ ] **P1-025** Prove configuration diagnostics and logs cannot reveal credentials.

## D. OpenAPI and client foundation

- [ ] **P1-030** Wire the pinned snapshot, overlay, generator lock, and deterministic generate command.
- [ ] **P1-031** Generate isolated transport/model code without manual edits.
- [ ] **P1-032** Implement base URL, `/api/v1`, User-Agent, timeout, and cancellation behavior.
- [ ] **P1-033** Implement FeatBit envelope decoding and centralized error classification.
- [ ] **P1-034** Implement all-page pagination and exact-identity result types.
- [ ] **P1-035** Implement safe-read-only retry, backoff/jitter, and `Retry-After` handling.
- [ ] **P1-036** Implement bounded concurrency and per-object write serialization interfaces.
- [ ] **P1-037** Implement token, secret, tenant, email, path-identity, and response redaction.

## E. Tests and reproducibility

- [ ] **P1-040** Test headers, base paths, envelopes, 401/403, validation, and ambiguous errors.
- [ ] **P1-041** Test exact zero/one/duplicate pagination, fuzzy-first rejection, and cancellation.
- [ ] **P1-042** Test read retries and prove unsafe writes are never retried.
- [ ] **P1-043** Test redaction against tokens, environment secrets, member emails, and runtime identities.
- [ ] **P1-044** Run generation twice and prove the second run has no diff.
- [ ] **P1-045** Run `gofmt`, `go vet`, unit tests, and `go test -race ./...`.

## F. Developer workflow and exit gate

- [ ] **P1-050** Add `GNUmakefile` targets: `fmt`, `lint`, `generate`, `test`, `testacc`, and `build`.
- [ ] **P1-051** Add pinned lint/vulnerability tooling and fork-safe CI.
- [ ] **P1-052** Add a local provider override guide.
- [ ] **P1-053** Load the provider through a local override.
- [ ] **P1-054** Run `terraform providers schema -json` successfully.
- [ ] **P1-055** Run the repository secret/redaction scan with zero findings.
- [ ] **P1-056** Update this README/TODO with final verification and the exact Phase 2 action.
- [ ] **P1-057** Declare Phase 1 complete only after every exit-gate condition passes.
