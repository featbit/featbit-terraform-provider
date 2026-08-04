# Phase 5 TODO — Initial release

Status: **In progress**
Next: **P5-010**

Complete one item at a time. Keep implementation scope, important files,
runtime relationship, and completion evidence under the active item. Record
only a concise result after material work. Do not begin IAM, publish externally,
create a tag, alter repository settings, or use release/Cloud secrets unless
the corresponding item and explicit maintainer authorization permit it.

## Release contract and public documentation

- [ ] **P5-010 — Freeze the core-only initial release contract and baseline.**

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
  registration, unstable Import form, or development version in a release
  build; the supported version/platform matrix and compatibility policy are
  explicit; the exact public-versus-maintainer prerequisites are recorded; and
  no live endpoint, signing secret, tag, GitHub release, or Registry mutation
  was used to obtain the baseline.

- [ ] **P5-011 — Add Registry documentation, examples, and drift verification.**

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

- [ ] **P5-012 — Add public security, contribution, support, and upgrade policy.**

  Scope: add the repository guidance required for maintainable public use:
  security reporting and supported-version expectations, contribution and
  development workflow, code of conduct if the FeatBit organization does not
  already provide an inherited one, issue/support boundaries, changelog
  policy, and compatibility/upgrade rules. Explain credential handling,
  trusted acceptance safeguards, generated-doc updates, release ownership,
  versioning, deprecation, state/schema compatibility, and the prohibition on
  editing published release assets.

  Important files: `SECURITY.md`, `CONTRIBUTING.md`,
  `CODE_OF_CONDUCT.md` only if needed, `CHANGELOG.md`, upgrade/versioning
  guidance, GitHub issue/PR templates only where they reduce unsafe reports,
  and README links. Reuse organization-level policies when they are current
  and linkable; do not duplicate or invent contact addresses.

  Runtime relationship: `user or contributor -> public support/security/
  contribution entry point -> safe reproduction or private disclosure ->
  tested change -> SemVer and state-compatible release process`.

  Done when a user can identify supported scope, report a vulnerability
  privately, request support, build/test/generate docs, and understand upgrade
  guarantees without receiving credentials or maintainer-only instructions;
  contributors are warned that acceptance tests create remote objects; release
  responsibilities and immutable-version policy are explicit; every referenced
  contact/process exists; and no speculative email, SLA, or organization policy
  is claimed.

## Credential-free CI and release packaging

- [ ] **P5-013 — Add fork-safe credential-free pull-request CI.**

  Scope: add read-only GitHub Actions for pull requests and ordinary branch
  pushes. Pin every action to a verified full commit SHA and every invoked tool
  to a reviewed version. Run formatting, vet, unit/mock/Protocol tests, race on
  supported runners, build, `go mod tidy -diff`, `go mod verify`, generated
  docs drift, dependency/license/vulnerability inspection, secret scanning,
  and snapshot-package validation as the implementation becomes available.
  Keep Cloud acceptance skipped and make fork execution independent of
  repository secrets. Do not use `pull_request_target` or execute artifacts
  from an untrusted workflow in a privileged context.

  Important files: `.github/workflows/test.yml` and narrowly justified
  supporting configuration; `GNUmakefile`; pinned tool installation or
  verification scripts; dependency update configuration if added; and tests
  that statically inspect workflow permissions, triggers, action SHAs, secret
  references, and command boundaries. Prefer official/mature tools already
  used by the ecosystem and record license/maintenance/security fit before
  adding one.

  Runtime relationship: `fork pull_request or branch push -> read-only
  credential-free workflow -> pinned actions/tools -> repository quality,
  Protocol, documentation, supply-chain, and snapshot checks -> status result`.

  Done when an untrusted fork can run every required non-live check with
  `contents: read`, no write/id-token permission, no FeatBit/GPG secret
  reference, no `TF_ACC=1`, and no privileged follow-up consuming its
  checkout/artifacts; all actions are immutable SHA pins with auditable version
  comments; cache keys cannot cross into privileged jobs; tests fail on unsafe
  triggers/permissions; and the complete credential-free workflow passes.

- [ ] **P5-014 — Add reproducible cross-platform Registry packaging.**

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
  verification tests/scripts, and `.gitignore` for local `dist/` output.
  Derive naming/signing behavior from current official HashiCorp scaffold and
  GoReleaser documentation, then remove unused template fields and unsupported
  platforms.

  Runtime relationship: `clean source commit plus SemVer snapshot/tag ->
  pinned Go toolchain and GoReleaser -> versioned CGO-free binaries -> zip
  archives plus manifest -> SHA256SUMS -> detached GPG signature`.

  Done when two clean snapshot builds have the expected file set and stable
  contents wherever the toolchain permits; each archive contains exactly one
  correctly named executable; binaries start on representative native runners
  and report the injected version; checksums cover all and only required files;
  the manifest is valid Protocol `6.0`; signature configuration uses no
  checked-in private material; snapshot mode creates no tag/release; and
  archive scans find no source tree, local path, credential, state, plan, log,
  or unexpected executable.

- [ ] **P5-015 — Add an isolated, protected tag-release workflow.**

  Scope: add the tag-triggered workflow that validates a clean `vX.Y.Z`
  SemVer tag, checks that its version matches the packaging contract, imports
  the protected GPG key, runs the already-proven release build, verifies assets,
  and creates a GitHub release. Pin actions by full SHA, grant only job-scoped
  `contents: write`, use an approval-protected environment where available,
  and keep GPG/GitHub credentials out of all pull-request paths. Decide and
  document whether the first release is created as a draft for maintainer
  inspection before finalization.

  Important files: `.github/workflows/release.yml`, workflow contract tests,
  `.goreleaser.yml`, maintainer release guidance, and protected secret/
  environment prerequisite documentation. Never commit a key, passphrase,
  fingerprint obtained from private material, or repository-specific secret
  value.

  Runtime relationship: `explicit maintainer-approved protected SemVer tag ->
  trusted exact commit checkout -> pinned Go/GPG/GoReleaser actions -> local
  verification -> signed immutable GitHub release assets -> Registry webhook`.

  Done when static tests reject non-SemVer tags, moving branch/tag ambiguity,
  unpinned actions, broad default permissions, PR-accessible secrets,
  `pull_request_target`, untrusted artifact reuse, and secret-bearing output;
  a no-publish dry run exercises validation and packaging without credentials;
  the protected workflow cannot finalize a release before all gates pass; and
  actual tag creation/publication remains pending explicit maintainer
  authorization.

- [ ] **P5-016 — Verify packaged-provider integrity and clean installation.**

  Scope: add release-asset verification independent of the packaging job.
  Check the expected matrix, archive paths/modes, binary version, manifest,
  checksums, signature with the public key, and optional provenance/SBOM. Start
  representative native binaries and obtain the exact Protocol v6 provider
  schema. Install a candidate from an isolated local filesystem/network mirror
  into a brand-new Terraform working directory with no development override,
  user plugin cache, prior lock file, or ambient provider binary.

  Important files: portable release verification scripts/tests; isolated
  Terraform smoke fixtures under `internal/provider/testdata` or a narrower
  release-test directory; `GNUmakefile`; expected schema snapshot; and
  credential-safe temporary-directory handling. Reuse existing Protocol
  fixtures rather than introducing a second fake product model.

  Runtime relationship: `signed candidate assets -> independent checksum/
  signature/content verification -> isolated provider mirror -> clean
  terraform init -> Protocol v6 schema/validate/plan -> packaged binary`.

  Done when verification fails for a missing/extra/renamed archive, wrong
  executable name/mode/version, malformed or wrong-protocol manifest, checksum
  mismatch, invalid signature, unexpected file, stale schema, dev override, or
  ambient cache hit; positive runs pass on every frozen representative
  platform; all temporary files remain outside the repository and are removed;
  and no live FeatBit credential or mutation is required.

## Compatibility, acceptance, and publication

- [ ] **P5-030 — Prove the Terraform and native-platform compatibility matrix.**

  Scope: run the packaged provider against the minimum, representative
  intermediate, and current supported Terraform CLI versions frozen in
  P5-010. Exercise provider initialization, schema, configuration validation,
  resource/data-source validation, Import parsing, and representative local
  Protocol lifecycle behavior on the native OS/architecture runners promised
  by the release. Verify documentation examples against the same source address
  and minimum syntax. Do not convert an untested cross-compiled archive into a
  stronger native-runtime support claim.

  Important files: compatibility workflow/matrix, pinned Terraform installers
  or checksums, packaged-provider smoke fixtures, schema snapshot, docs
  examples, and focused result assertions. Keep this credential-free; live
  current-Cloud coverage remains P5-031.

  Runtime relationship: `frozen Terraform CLI plus native runner -> clean
  install of exact candidate archive -> Protocol v6 provider -> schema,
  validation, Import parsing, and isolated lifecycle fixture`.

  Done when every promised Terraform/platform pair either runs natively and
  passes or is narrowed from the public support matrix; minimum-version
  examples parse; the exact four-resource/four-data-source schema is identical
  across binaries; version/User-Agent metadata is the candidate version; no
  IAM/config-header surface appears; and CI records no credential, state,
  absolute workstation path, or non-test object value.

- [ ] **P5-031 — Run the trusted core-only current-Cloud release-candidate gate.**

  Scope: run the exact candidate commit/binary against current FeatBit Cloud
  using credentials supplied only out of band through a protected environment.
  Create one uniquely prefixed, test-owned Project and only its child
  Environments, Feature Flags, and environment-specific Segments. Exercise all
  four Feature Flag types, exact data sources, Import followed by an empty
  plan, second-plan idempotence, drift repair, replacement, Segment-reference
  refusal/recovery, and child-first destroy. Maintain an in-memory inventory
  before each mutation and independently prove exact cleanup. Do not enumerate,
  inspect, bind, or modify any unrelated project, shared Segment, IAM object,
  organization/workspace membership, or account setting.

  Important files: existing core Cloud acceptance tests/harnesses, candidate
  binary/version wiring, a protected manually invoked acceptance workflow if
  justified, and in-memory cleanup verification. Reuse the proven Phase 2–4
  fixtures and tighten them only where a release-candidate artifact reveals a
  concrete gap.

  Runtime relationship: `protected manual approval plus out-of-band token ->
  exact candidate Protocol v6 provider -> documented core public endpoints ->
  uniquely test-owned Project tree -> independent exact child-first cleanup`.

  Done when the complete core lifecycle passes with the release candidate;
  every created UUID/key is registered before the next mutation; cleanup proves
  all test-owned active/archived children and parent absent; pre-existing
  objects were never listed or changed; shared Segment behavior remains skipped
  without an explicit owned fixture; no IAM endpoint/header is sent; and token,
  runtime IDs/keys, paths, response bodies, state, logs, or cleanup inventory
  are neither persisted nor exposed.

- [ ] **P5-032 — Run the complete local and supply-chain release gate.**

  Scope: run all Phase 1–4 quality gates plus release-specific documentation,
  workflow, packaging, artifact, compatibility, dependency, license,
  vulnerability, secret, and provenance checks from a clean tree. Re-run
  focused tests repeatedly where ordering, concurrency, generated output, or
  artifact enumeration could be nondeterministic. Inspect the complete diff and
  final archives. Do not weaken or skip a core gate to make release automation
  pass.

  Important files: production/test/documentation/workflow/packaging fixes
  demonstrated by the gate, `GNUmakefile`, pinned tool metadata, this active
  TODO, and the Phase 5 README pointer. Do not add IAM code or modify the frozen
  public schema.

  Runtime relationship: `clean repository -> format/vet/test/race/build/
  module/docs/security/workflow checks -> reproducible signed-candidate
  packaging -> independent artifact/install/compatibility verification ->
  release readiness decision`.

  Done when `gofmt -l .` is empty; `go vet ./...`, `go test ./...`,
  repeated focused/Protocol tests, `go test -race ./...`, `go build ./...`,
  `go mod tidy -diff`, and `go mod verify` pass; docs regenerate without
  diff; workflows pass static trust-boundary tests; dependencies/licenses and
  `govulncheck` have no unresolved release blocker; Gitleaks and exact
  out-of-band secret scans find zero repository/archive hit; snapshot artifacts
  pass integrity/install checks; `git diff --check` passes; only expected
  release files changed; and the schema still contains exactly five provider
  attributes, four resources, four data sources, and no IAM surface.

- [ ] **P5-033 — Publish and verify the initial GitHub/Registry release.**

  Scope: only after every prior item passes and the maintainer explicitly
  authorizes the exact version and external actions, confirm the compatible GPG
  public key is registered for the intended Registry namespace, protected
  secrets/settings are ready, create and push the exact SemVer tag, run the
  protected release workflow, inspect every signed asset before finalization,
  connect/resynchronize the Terraform Registry provider if required, and verify
  the Registry-served version. Never overwrite an existing tag or release
  asset; abort to a new version if any final artifact differs from the verified
  candidate.

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
