# Phase 5 TODO — Initial release

Status: **In progress**
Next: **P5-090**

Complete one item at a time. Keep implementation scope, important files,
runtime relationship, and completion evidence under the active item. Record
only a concise result after material work. Do not begin IAM, publish externally,
create a tag, alter repository settings, or use release/Cloud secrets unless
the corresponding item and explicit maintainer authorization permit it.

## Release contract and public documentation

- [x] **P5-010 — Freeze the core-only initial release contract and baseline.**

  Scope: inventory the actual provider schema, resource/data-source
  registrations, Import forms, build version path, manifest, repository
  visibility/name prerequisites, current test gates, and absent release
  surfaces. Freeze the initial SemVer strategy, minimum/tested Terraform CLI
  versions, CI/release Go version, supported OS/architecture archive matrix,
  Protocol version, Cloud compatibility statement, self-hosted non-claim, and
  compatibility/upgrade policy. The released schema must contain exactly the
  existing five provider attributes, four core resources, and four data
  sources. IAM and organization/workspace context contracts remain absent.
  When a version, platform, signing identity, or Registry ownership choice
  requires maintainer authority, record the exact decision needed without
  guessing or accessing secrets.

  Important files: `go.mod`, `main.go`,
  `terraform-registry-manifest.json`, `internal/provider/provider.go`,
  provider/schema/Import tests, `GNUmakefile`, `.gitignore`, the master
  plan, and this Phase 5 package. Inspect the current official HashiCorp
  provider publishing/documentation requirements and the current Terraform
  Plugin Framework scaffold; do not copy a template without reconciling it
  against this repository.

  Runtime relationship: `Terraform CLI version/platform -> Registry address
  and Protocol 6 discovery -> versioned provider binary -> frozen provider
  configuration -> one of four resource/data-source lifecycles -> documented
  public API`.

  Done when a checked-in focused release-contract/schema snapshot rejects any
  unexpected provider attribute, resource, data source, Protocol change, IAM
  registration, or unstable Import form; GoReleaser injects the tag-derived
  version; the supported version/platform matrix and compatibility policy are
  explicit; the exact public-versus-maintainer prerequisites are recorded; and
  no live endpoint, signing secret, tag, GitHub release, or Registry mutation
  was used to obtain the baseline.

  Result (updated 2026-08-06): froze the initial stable tag as `v0.1.0`,
  Terraform `>= 1.0.0` and qualification pins
  `1.0.11`/`1.5.7`/`1.15.8`, Go `1.26.5`, Protocol `6.0`, and the five
  supported 64-bit archives in the Phase README. Added
  `release_contract_test.go` and the reviewed
  `internal/provider/testdata/release-schema.json`; they independently lock
  the five provider attributes, four resources/data sources, empty non-core
  surfaces, four strict Import forms, and manifest/address metadata. The
  scaffold-style GoReleaser configuration owns release-version injection.
  Signing identity and protected secrets, Registry namespace authority, and all
  publication mutations remain explicit maintainer prerequisites. The current
  Go 1.26.5 baseline passes full tests, vet, build, tidy-diff,
  module verification, repeated focused checks, and
  CGO-free builds for every frozen target passed without a live API or secret.

- [x] **P5-011 — Add Registry documentation, examples, and drift verification.**

  Scope: add a public `README.md`, Registry provider index, one page for each
  of the four resources and four data sources, and credential-free examples
  for provider configuration, dependency ordering, exact lookup, every Import
  form, and representative lifecycles. Explain Feature Flag UI-owned targeting,
  environment-specific versus shared Segment behavior, canonical identifiers,
  replacement, deletion, and safe environment-variable authentication. Use a
  pinned `terraform-plugin-docs` workflow when schema generation is a semantic
  fit, plus narrow templates for behavior the schema cannot express. Add a
  generation/drift check that never rewrites the working tree silently in CI.

  Important files: new `README.md`, `docs/index.md`,
  `docs/resources/*.md`, `docs/data-sources/*.md`,
  `examples/provider/*.tf`, `examples/resources/**`,
  `examples/data-sources/**`, generator/templates/tool pinning, and focused
  docs/example checks. Reuse the existing schema descriptions rather than
  maintaining a contradictory manual attribute catalog.

  Runtime relationship: `provider/resource/data-source schema -> pinned docs
  generator plus reviewed templates -> Registry markdown -> credential-free
  HCL examples -> terraform fmt/validate and exact Import syntax checks`.

  Done when generated files are reproducible; every registered object has one
  correctly named Registry page; examples format and validate against the
  frozen source address/schema without a token value or local override;
  documentation states exactly what Terraform owns and leaves UI-owned fields
  alone; all four Import forms are exact; regeneration produces an empty diff;
  links resolve; and repository scans find no runtime Cloud value, state,
  plan, log, or secret.

  Result (2026-08-06): added the public README, schema-derived provider index
  and eight object pages, reviewed behavior templates, and nine credential-free
  provider/resource/data-source example sets with explicit `api_url` and
  `FEATBIT_API_URL` endpoint guidance plus all four strict Import forms.
  The root README is intentionally end-user-only: installation,
  authentication, supported capabilities, usage and ownership behavior,
  documentation, support/security, upgrades, and licensing; maintainer
  development, test, and release-qualification details remain outside it.
  The workflow pins `terraform-plugin-docs@v0.25.0`; CI installs Terraform
  1.15.8 through HashiCorp's pinned `setup-terraform` action, and local checks
  use the documented Terraform binary on `PATH`. `make docs` is the explicit
  writer, while `make docs-check` generates into a temporary directory, byte-
  compares committed output, validates Registry structure, and formats/
  initializes/validates every example against a temporary
  `registry.terraform.io/featbit/featbit` package without a token or development
  override. The non-writing drift run, all nine example
  validations, link/import/ownership/sensitive-value contract checks, full Go
  tests, vet, build, tidy-diff, and module verification passed without a live
  API or secret; scans found no state, plan, log, runtime UUID, or credential
  assignment.

- [x] **P5-012 — Add practical security, support, and upgrade guidance.**

  Scope: add only the concise user guidance needed for credential, state, plan,
  log, vulnerability, support, and upgrade handling. Use a private security-
  reporting route only when it exists and can be verified; otherwise provide a
  detail-free escalation path. Preserve the existing license and SPDX notices,
  and do not add contribution governance before a collaboration model exists.

  Important files: `SECURITY.md`, `SUPPORT.md`, `UPGRADING.md`, README links,
  and a safety-first issue template. Do not add `CONTRIBUTING.md`,
  `CODE_OF_CONDUCT.md`, speculative contacts, or unrelated process documents.

  Runtime relationship: `user -> support or security entry point -> safe
  reproduction or verified private disclosure -> compatible upgrade`.

  Done when users can choose the correct support/security path, report issues
  without exposing sensitive material, and understand compatibility and upgrade
  expectations; every referenced route exists; and the guidance makes no
  speculative SLA, legal, copyright, governance, or collaboration claim.

  Result (updated 2026-08-08): retained focused `SECURITY.md`, `SUPPORT.md`, and
  `UPGRADING.md` entry points plus a safety-first GitHub bug form and enabled
  blank issue path. They cover secret/state/plan/log handling, support
  boundaries, SemVer/schema/state/Import compatibility, and safe upgrades. The
  constraint is `~> 0.1.0`, so Terraform cannot auto-select a potentially
  breaking `0.2.0`. No standalone contribution guide is retained before a
  collaboration model is chosen. Guidance, issue-template, README, docs, test,
  build, module, and diff checks passed without credentials.

## Credential-free CI and release packaging

- [x] **P5-013 — Add fork-safe credential-free pull-request CI.**

  Scope: add read-only GitHub Actions for pull requests and ordinary branch
  pushes. Pin every action to a verified full commit SHA and every invoked tool
  to a reviewed version. Run formatting, vet, unit/mock/Protocol tests, race on
  supported runners, build, `go mod tidy -diff`, `go mod verify`, generated
  docs drift, dependency/license/vulnerability inspection, and secret scanning.
  Keep Cloud acceptance skipped and make fork execution independent of
  repository secrets. Do not use `pull_request_target` or execute artifacts
  from an untrusted workflow in a privileged context.

  Important files: `.github/workflows/test.yml` and narrowly justified
  supporting configuration; `GNUmakefile`; pinned tool installation or
  verification commands; and dependency update configuration if added. Prefer
  official/mature tools already used by the ecosystem and record license,
  maintenance, and security fit before adding one.

  Runtime relationship: `fork pull_request or branch push -> read-only
  credential-free workflow -> pinned actions/tools -> repository quality,
  Protocol, documentation, and supply-chain checks -> status result`.

  Done when an untrusted fork can run every required non-live check with
  `contents: read`, no write/id-token permission, no FeatBit/GPG secret
  reference, no `TF_ACC=1`, and no privileged follow-up consuming its
  checkout/artifacts; all actions are immutable SHA pins with auditable version
  comments; cache keys cannot cross into privileged jobs; actionlint accepts
  the workflow; and the complete credential-free workflow passes.

  Result (updated 2026-08-08): added branch-push and fork `pull_request` CI with only
  `contents: read`, credential persistence and caches disabled, no artifact or
  privileged follow-up, and official checkout/setup-go actions pinned to the
  verified full SHAs for `v7.0.1`/`v7.0.0`. Added non-writing Make targets and
  exact pins for actionlint `v1.7.12` (MIT), Go-team govulncheck `v1.6.0`
  (BSD-3-Clause), Google go-licenses `v2.0.1` (Apache-2.0), and Gitleaks
  `v8.28.0` (MIT). The vulnerability gate found 11 reachable standard-library
  vulnerabilities in the earlier toolchain baseline. Before `v0.1.0`, the
  release baseline moved to official Go 1.26.5 so CI and release packaging use
  the current stable Go line with the required fixes; every current contract
  and guidance reference was synchronized. With exact Go 1.26.5, the full
  non-live unit/mock/Protocol suite plus formatting, vet, build, module,
  docs/example, license, vulnerability, workflow, and secret gates pass; an
  isolated Linux/amd64 Provider race run also passes. Packaging remains outside
  pull-request CI and is owned by GoReleaser; no custom workflow contract test
  is retained.

- [x] **P5-014 — Add reproducible cross-platform Registry packaging.**

  Scope: add a pinned GoReleaser v2 configuration for the platform matrix
  frozen in P5-010. Build with `CGO_ENABLED=0`, `-trimpath`, deterministic
  source metadata, and `-X main.version=<SemVer>`. Produce one correctly named
  zip per supported pair, the renamed Protocol 6 manifest, SHA-256 sums over
  every archive and manifest, and the detached checksum-signature configuration
  required by the Registry. Keep `go mod tidy` as a prior verification step,
  not a release-time source mutation. Add an optional SBOM only if it is
  deterministic, reviewed, and cannot disturb required artifact names.

  Important files: `.goreleaser.yml`,
  `terraform-registry-manifest.json`, `main.go`, `GNUmakefile`, release
  workflow, and `.gitignore` for local `dist/` output. Derive naming/signing
  behavior from the current official HashiCorp scaffold and GoReleaser
  documentation, then remove unused template fields and unsupported platforms.

  Runtime relationship: `clean source commit plus snapshot or SemVer tag ->
  pinned Go toolchain and GoReleaser -> versioned CGO-free binaries -> zip
  archives plus manifest -> SHA256SUMS -> detached GPG signature`.

  Done when `goreleaser check` accepts the configuration; an unsigned local
  snapshot builds the frozen five-platform matrix without publication; release
  naming, manifest inclusion, checksums, and detached signing follow the
  official scaffold contract; signing configuration contains no checked-in
  private material; and `dist/` remains ignored and uncommitted.

  Result (updated 2026-08-08): added GoReleaser v2 configuration pinned to `v2.13.3`
  and the frozen Go 1.26.5 toolchain. It builds exactly the five reviewed
  CGO-free targets with trimmed paths, deterministic commit timestamps,
  tag-derived version injection, zip archives, the renamed Protocol 6
  manifest, SHA-256 coverage, and detached GPG
  checksum-signature configuration without private material. GoReleaser's
  built-in extra-file mapping owns manifest checksum and upload naming. The
  current scaffold-style design intentionally retains no custom artifact or
  clean-install verifier; local snapshots use GoReleaser's normal snapshot
  version and exist only for optional maintainer inspection. Configuration,
  full tests, vet, workflow lint, and diff checks passed; snapshot mode created
  no tag, signature, or release.

- [x] **P5-015 — Add an isolated, protected tag-release workflow.**

  Scope: add the scaffold-style tag workflow that checks out the tagged commit,
  installs the Go version from `go.mod`, imports the protected GPG key, and runs
  pinned GoReleaser. Grant only job-scoped `contents: write`, use an approval-
  protected environment, keep release secrets out of pull-request paths, and
  create a draft for maintainer inspection before publication.

  Important files: `.github/workflows/release.yml` and `.goreleaser.yml`. Never
  commit a key, passphrase, or repository-specific secret value.

  Runtime relationship: `maintainer-approved v* tag -> tagged commit checkout
  -> pinned Go/GPG/GoReleaser actions -> signed draft GitHub release -> manual
  inspection and publication`.

  Done when the workflow is tag-only, actions are immutable SHA pins, the GPG
  key is available only inside the protected release environment, only the
  release job receives `contents: write`, GoReleaser creates a draft, and tag
  creation/publication remain explicit maintainer actions.

  Result (2026-08-08): added the official-scaffold-shaped single release job
  with empty default permissions, job-scoped `contents: write`, a protected
  `release` environment, exact action pins, GPG secrets, pinned GoReleaser
  `v2.13.3`, and draft-only publication. Custom tag, artifact, signature, and
  clean-install verification programs and workflow contract tests are not part
  of the current minimal design, and no standalone release manual is retained.
  No tag, signature, GitHub release, repository setting, or publication was
  created or changed.

## Compatibility, acceptance, and publication

- [x] **P5-030 — Prove Terraform CLI compatibility and recheck the archive matrix.**

  Scope: add a focused Linux/AMD64 matrix to the existing credential-free test
  workflow for the minimum, representative intermediate, and current Terraform
  CLI versions frozen in P5-010. Run the existing Protocol contract against
  each version and re-run the GoReleaser snapshot for the five archive targets.
  Do not add a custom verifier, a second workflow, or five native OS/architecture
  test matrices.

  Important files: `.github/workflows/test.yml`, its pinned Terraform setup,
  the existing Protocol tests and schema snapshot, `.goreleaser.yml`, and this
  active context. Keep this credential-free; live current-Cloud coverage remains
  P5-031. Add no helper program solely for the matrix.

  Runtime relationship: `each frozen Terraform CLI on Linux/AMD64 -> existing
  Protocol contract -> compatibility result; GoReleaser snapshot -> five
  cross-built archive targets`.

  Done when all three Terraform versions pass the frozen four-resource/four-
  data-source Protocol contract on Linux/AMD64; the unsigned snapshot still
  builds exactly the five reviewed archives with Go 1.26.5; documentation keeps
  Terraform `>= 1.0.0` separate from archive availability; no native-runtime
  claim, IAM/config-header surface, credential, state, absolute workstation
  path, or non-test object value is introduced.

  Result (2026-08-08): added a focused matrix to the existing read-only,
  credential-free workflow for Terraform `1.0.11`, `1.5.7`, and `1.15.8` on
  Linux/AMD64. All three passed the existing four-resource/four-data-source
  Protocol contract with Go 1.26.5 and local test fixtures; actionlint accepted
  the workflow. A separate credential-free GoReleaser `v2.13.3` snapshot built
  exactly the frozen `darwin_amd64`, `darwin_arm64`, `linux_amd64`,
  `linux_arm64`, and `windows_amd64` archives, each containing only its provider
  executable, with checksums covering all archives and the renamed manifest.
  Public usage guidance states only the Terraform `>= 1.0.0` requirement;
  qualified CLI versions and cross-built archive evidence remain in this
  release context. No Cloud/release credential, live endpoint, native-platform
  claim, product surface, tag, or publication was used or added, and generated
  `dist/` output was not retained.

- [x] **P5-031 — Run the trusted core-only current-Cloud release-candidate gate.**

  Scope: run the exact candidate commit/binary against current FeatBit Cloud
  using credentials supplied only out of band to an explicit maintainer-run
  session.
  Create one uniquely prefixed, test-owned Project and only its child
  Environments, Feature Flags, and environment-specific Segments. Exercise all
  four Feature Flag types, exact data sources, Import followed by an empty
  plan, second-plan idempotence, drift repair, replacement, Segment-reference
  refusal/recovery, and child-first destroy. Maintain an in-memory inventory
  before each mutation and independently prove exact cleanup. Do not enumerate,
  inspect, bind, or modify any unrelated project, shared Segment, IAM object,
  organization/workspace membership, or account setting.

  Important files: existing core Cloud acceptance tests/harnesses, candidate
  binary/version wiring, and in-memory cleanup verification. Reuse the proven
  Phase 2–4 fixtures and tighten them only where the exact candidate commit
  reveals a concrete gap. Do not add a permanent Cloud workflow unless a
  recurring maintainer need is established separately.

  Runtime relationship: `explicit maintainer execution plus out-of-band token ->
  exact candidate Protocol v6 provider -> documented core public endpoints ->
  uniquely test-owned Project tree -> independent exact child-first cleanup`.

  Done when the complete core lifecycle passes with the release candidate;
  every created UUID/key is registered before the next mutation; cleanup proves
  all test-owned active/archived children and parent absent; pre-existing
  objects were never listed or changed; shared Segment behavior remains skipped
  without an explicit owned fixture; no IAM endpoint/header is sent; and token,
  runtime IDs/keys, paths, response bodies, state, logs, or cleanup inventory
  are neither persisted nor exposed.

  Result (2026-08-09): the exact checked-out provider passed the trusted
  current-Cloud Project/Environment, four-type Feature Flag, and
  environment-specific Segment gates against uniquely prefixed test-owned
  Project trees. Exact data sources, Import plus empty plans, second-plan
  idempotence, drift repair, replacement/recreation, Segment-reference
  refusal/recovery, child-first destroy, and exact active/archived cleanup all
  passed. Only registered test keys and returned identities were selected for
  mutation or cleanup; no unrelated object, shared Segment, IAM surface, or
  account setting was inspected or changed. No credential, runtime value,
  state, plan, log, or cleanup inventory was persisted or exposed, and no CI
  or release workflow was added or changed.

- [x] **P5-032 — Run the complete local and supply-chain release gate.**

  Scope: run all Phase 1–4 quality gates plus release-specific documentation,
  workflow, GoReleaser configuration/snapshot, compatibility, dependency,
  license, vulnerability, and secret checks from a clean tree. Re-run focused
  tests where ordering, concurrency, or generated output could be
  nondeterministic. Inspect the complete diff. Do not add a custom artifact
  verifier or weaken a core gate to make release automation pass.

  Important files: production/test/documentation/workflow/packaging fixes
  demonstrated by the gate, `GNUmakefile`, pinned tool metadata, this active
  TODO, and the Phase 5 README pointer. Do not add IAM code or modify the frozen
  public schema.

  Runtime relationship: `clean repository -> format/vet/test/race/build/
  module/docs/security/workflow checks -> GoReleaser config and unsigned
  snapshot -> release readiness decision`.

  Done when `gofmt -l .` is empty; `go vet ./...`, `go test ./...`,
  repeated focused/Protocol tests, `go test -race ./...`, `go build ./...`,
  `go mod tidy -diff`, and `go mod verify` pass; docs regenerate without
  diff; workflows pass actionlint; dependencies/licenses and
  `govulncheck` have no unresolved release blocker; Gitleaks and exact
  out-of-band secret scans find zero repository hit; GoReleaser configuration
  and an unsigned snapshot complete without publication; generated `dist/` is
  not retained; `git diff --check` passes; only expected release files changed;
  and the schema still contains exactly five provider attributes, four
  resources, four data sources, and no IAM surface.

  Result (2026-08-09): formatting, vet, full tests, three repeated focused
  Protocol/release-contract runs, the complete Linux/AMD64 race suite, build,
  module, documentation/example, workflow, dependency/license/vulnerability,
  Gitleaks, and exact credential gates passed. A Windows-only CRLF assumption
  in the GNUmakefile documentation assertion was normalized without changing
  generated output or product behavior. GoReleaser `v2.13.3` configuration and
  an unsigned snapshot produced exactly the frozen five archives, one provider
  executable per archive, and verified checksums for all archives plus the
  Protocol `6.0` manifest. No signature, tag, release, publication, credential,
  runtime artifact, or `dist/` directory was retained. The frozen schema still
  exposes exactly five provider attributes, four resources, four data sources,
  and no IAM surface.

- [x] **P5-033 — Publish and verify the initial GitHub/Registry release.**

  Scope: only after every prior item passes and the maintainer explicitly
  authorizes the external actions for frozen version `v0.1.0`, confirm the
  compatible GPG public key is registered for the intended Registry namespace, protected
  secrets/settings are ready, create and push the exact SemVer tag, run the
  protected release workflow, inspect every signed asset before finalization,
  connect/resynchronize the Terraform Registry provider if required, and verify
  the Registry-served version. Never overwrite an existing tag or release
  asset; abort to a new version if the draft assets differ from the frozen
  GoReleaser and Registry contract.

  Important files: no source mutation is expected after the release commit;
  use the protected tag workflow, GitHub release assets, public GPG key,
  Registry provider entry, and a new external temporary Terraform smoke
  directory. Keep all credentials and runtime test inventory out of files and
  logs.

  Runtime relationship: `explicit maintainer approval -> immutable exact
  release commit/tag -> protected signed GitHub release -> Registry webhook/
  provider version -> clean terraform init from registry -> exact core
  plan/apply/import/empty-plan/destroy -> exact Cloud cleanup`.

  Done when the final asset set byte-for-byte satisfies the frozen naming,
  checksum, signature, manifest, schema, version, and platform contracts; the
  Registry lists the exact version under
  `registry.terraform.io/featbit/featbit`; a clean directory with no override
  downloads that version and completes the core smoke lifecycle; exact cleanup
  passes; documentation renders correctly; no IAM surface appears; no secret
  or unrelated FeatBit object was accessed; and the release/tag/assets were not
  mutated after publication.

  Result (2026-08-10): the protected workflow published immutable tag
  `v0.1.0` from frozen commit `bc51bcca8335a19729a49915546c056854a6cff1`.
  Independent inspection verified the unchanged eight-asset release: five
  single-binary archives, complete checksums, the exact Protocol `6.0`
  manifest, and a signature from the Registry-registered public key. The same
  repository was renamed to the required canonical URL
  `featbit/terraform-provider-featbit`, connected under namespace `featbit`,
  and accepted by HCP without changing its tag or release assets. The public
  Registry now lists exactly `0.1.0`, Protocol `6.0`, and the frozen Darwin,
  Linux, and Windows five-platform matrix. The initially empty GitHub Release
  body was subsequently completed with English notes covering capabilities,
  installation, compatibility, integrity, scope, and support; only the Release
  description changed, while the tag and all eight assets remained unchanged.

  A clean temporary directory with a direct-only Registry installation and no
  CLI override downloaded `featbit/featbit` `0.1.0`. The Registry binary
  exposed exactly five provider attributes, four resources, four data sources,
  and no IAM surface; its core plan/apply/exact-data-source/second-plan/import/
  destroy smoke passed. A focused independent Segment Import also converged to
  an empty plan, and a trusted current-Cloud repeat proved the explicit-empty
  targeting edge with Import plus empty plan. Every temporary Project tree was
  removed with independent exact-zero verification, all local smoke artifacts
  were deleted, and no credential, runtime identity, unrelated object, tag,
  or published asset was exposed or mutated.

## Phase exit

- [ ] **P5-090 — Close Phase 5 and prepare post-release Phase 6 IAM.**

  Confirm every item above and the README exit gate. Update the master plan
  only with still-current released architecture, compatibility, support, and
  roadmap facts. Make its next action the first concrete Phase 6 IAM
  contract-verification task. Delete this completed Phase 5 package and create
  only a Phase 6 IAM `README.md` and detailed `todo.md`.

- [ ] **P5-091 — Declare Phase 5 complete only after the exit gate passes.**

  This final consistency check must find no unchecked item, unpublished or
  unverified release, mutable/missing asset, checksum/signature/manifest
  mismatch, development-version binary, stale documentation, unsupported
  compatibility claim, unpinned or over-privileged workflow, fork-accessible
  secret, unresolved dependency/license/vulnerability issue, failed core
  lifecycle/Import convergence, pending test object, changed unrelated Cloud
  object, credential/runtime-value finding, IAM schema/header/endpoint surface,
  or absent Phase 6 IAM entry point.
