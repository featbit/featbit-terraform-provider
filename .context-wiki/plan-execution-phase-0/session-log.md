# Phase 0 Session Log

This file is append-only. Add one entry per working session.

## 2026-07-30 — Execution package created

Scope:

- Converted the Phase 0 roadmap into a standalone execution package.
- Defined cross-session context and evidence protocols.
- Created the detailed TODO list, status/findings/matrix templates, ADR/evidence structure, handoff, and Phase 0/Phase 1 prompts.
- Added root `AGENTS.md` to describe the workspace and make `.context-wiki/` the traceable project context.

Verification:

- Documentation structure and internal links still require final validation in this planning session.
- No live API probe was executed.
- No credential was accessed.
- No remote FeatBit resource was created.

Phase 0 task progress:

- `0 / 76` execution tasks complete.

Next action:

- Start a new session using `phase-0-execution-prompt.md`.
- Begin with `P0-001` through `P0-008`.

## 2026-07-30 — Execution-package validation completed

Scope:

- Validated the newly created Phase 0 context package and root workspace instructions.

Verification:

- All local Markdown links resolve.
- Every Markdown file has balanced fenced code blocks.
- `todo.md` contains 76 unique task IDs and 76 unchecked execution tasks.
- `git diff --check` reports no whitespace errors.
- A repository/context scan found no value matching the checked access-token/JWT patterns.
- No Phase 0 live API task was executed.
- No remote resource was created.

Next action:

- Open a new session with `phase-0-execution-prompt.md`.
- Confirm disposable targets and credentials through environment-variable presence only.
- Execute `P0-001` through `P0-008` before any mutating probe.

## 2026-07-30 — Root instructions simplified

- Reduced `AGENTS.md` to the workspace purpose, context read/write contract, and essential guardrails.
- Detailed execution rules remain in the active phase's `context-protocol.md`.
- No Phase 0 API task was executed.

## 2026-07-30 — Phase 0 execution started

Started: `2026-07-30T07:24:21Z`

Session scope:

- Execute Phase 0 in TODO dependency order without starting production provider resources.
- Initial task set: `P0-001` through `P0-008`, followed by `P0-010` through `P0-018`.
- Continue safe offline/specification/probe/unit-test work when live prerequisites are unavailable.

Read baseline:

- `AGENTS.md`
- `.context-wiki/plan.md`
- Phase 0 `README.md`, `plan.md`, `context-protocol.md`, `status.md`, `todo.md`, and `handoff.md`
- Linked `findings.md`, `compatibility-matrix.md`, ADR guidance/template, `evidence/README.md`, and prior `session-log.md`

Repository baseline:

- Branch: `main`
- Commit: `4f04f4eec2b26f9df1358a34638d4189c861361d`
- Initial worktree: clean
- Operator/session: automated Codex Phase 0 execution in the approved workspace

Runtime prerequisite presence:

- `FEATBIT_TEST_API_URL`: absent
- `FEATBIT_TEST_SERVICE_TOKEN`: absent
- `FEATBIT_TEST_PERSONAL_TOKEN`: absent
- `FEATBIT_TEST_TARGET`: absent
- `FEATBIT_TEST_RESOURCE_PREFIX`: absent

Safety decision:

- No credential value from chat will be copied into a command, file, log, fixture, or evidence record.
- Live read/write tasks remain blocked until the named environment variables are present.
- No remote object existed in the cleanup inventory at session start.

Ended: `2026-07-30T07:58:25Z`

Completed task IDs:

- `P0-001` through `P0-008`
- `P0-010` through `P0-014`
- `P0-016`, `P0-018`
- `P0-023`, `P0-025`
- `P0-038`, `P0-039`
- `P0-047`, `P0-054`
- `P0-073`, `P0-074`
- `P0-082`, `P0-083`, `P0-085`, `P0-086`
- `P0-091` through `P0-094`

Material work:

- Pinned the exact public OpenAPI snapshot, hash, inventory, Overlay 1.1.0
  operation IDs, overlaid output, and `oapi-codegen v2.8.0` inputs.
- Built the reusable Go probe with fail-closed mutation configuration, direct
  access-token transport, bounded HTTP/envelope handling, redaction, exact
  all-page lookup, cleanup inventory/execution, error/existence classification,
  safe-read retry policy, and value-free diagnostics.
- Added precise flag/segment JSON/number/condition/rollout normalization,
  partial-create recovery checkpoints, and strict Import identity parsers.
- Recorded sanitized Cloud missing/malformed-auth observations.
- Completed conservative capability classifications and Phase 1 responsibility
  mapping.
- Created ADR-001 through ADR-005. ADR-003 and ADR-004 are Accepted; ADR-001,
  ADR-002, and ADR-005 remain Proposed with exact live blockers.

Verification:

- `go test ./...`: passed.
- `go vet ./...`: passed.
- `go test -count=10 ./internal/...`: passed.
- OpenAPI deterministic/stale check: passed with the locked hashes and zero
  missing/duplicate operation IDs.
- Context link/fence/TODO-evidence validation: passed.
- Repository secret scan: passed without printing candidate values.
- `git diff --check`: passed.
- `go test -race ./...`: blocked because `gcc` is unavailable on this Windows
  host; ordinary tests pass.

Deployment observations:

- `cloud-current`: only missing and synthetic-malformed authentication were
  tested; both returned structured 401.
- No successful authenticated Cloud request was executed.
- `selfhosted-min`: not available and no version selected.

Cleanup:

- No remote object was created.
- Cleanup dry run: zero pending entries.
- No manual owner/action is outstanding.

Exit gate:

- **Not passed.** Phase state is blocked on live prerequisites.
- Phase 1 was not started.

Exact next action:

- Supply the required `FEATBIT_TEST_*` values in the process environment.
- Run `featbit-api-probe config`, then the sanitized service-token
  `projects-list` read.
- Record that successful read before any mutation, then continue at `P0-020`
  and project/environment lifecycle tasks in dependency order.
- Select/provision the exact `selfhosted-min` release in parallel.

## 2026-07-31 — Authenticated Cloud read verified

Scope:

- Loaded the service credential from the Windows user environment into one
  probe process without printing or persisting its value.
- Configured the exact documented Cloud API host, `cloud-current` target, and
  synthetic `tfp0-20260731-codex01` resource prefix.
- Ran only the read-only project-list probe. No lifecycle mutation ran.

Verification:

- Probe configuration: `read_ready=true`, `mutation_ready=true`.
- `GET /api/v1/projects`: HTTP 200, `success=true`,
  `data_shape=array(items=11)`, no structured errors.
- The request sent the service token directly in `Authorization` and no
  organization/workspace context headers.
- Output contained no raw body, resource identities, tenant context, or
  environment secret values.
- Personal-token verification is `N/A`; no personal token was supplied.

Completed task IDs:

- `P0-015`
- `P0-020`
- `P0-021` (`N/A` by its stated acceptance condition)
- `P0-024`

Cleanup:

- No remote object was created, modified, or deleted.
- Cleanup inventory remains empty.

Exit gate:

- **Not passed.** Project/environment lifecycle and the remaining live matrix
  are still pending.

Exact next action:

- Implement and unit-test the fail-closed project/environment lifecycle command.
- Report the planned request sequence before running it.
- Then create, update, and delete only the new uniquely prefixed project and
  its new environment, recording IDs immediately and verifying cleanup.

## 2026-07-31 — Access-token terminology corrected

Clarification:

- The product owner confirmed that personal/access-token labels do not
  represent separate provider authentication contracts.
- Both use the same direct `Authorization` transport and permission model.
- Effective capabilities are determined by token permissions.

Decision:

- Provider v1 exposes one Sensitive `access_token` attribute.
- Provider v1 exposes no token-kind selector.
- `P0-021` is `N/A` because a second credential would repeat the same
  transport contract, not because a credential happened to be unavailable.
- Permission behavior will be observed per operation during lifecycle probes.

Cleanup:

- No remote operation was performed for this correction.

## 2026-07-31 — Project/environment lifecycle command implemented offline

Scope:

- Implemented `project-env-lifecycle --execute` against only documented public
  project/environment endpoints.
- Added a separate concrete request path versus sanitized observation-template
  path so reports never contain resource UUIDs.
- Added exact inventory completion and duplicate-pending protection.
- Added custom environment-secret decoding that retains field/type presence
  and discards values.
- Added normal lifecycle, ambiguous-create recovery, pre-existing-key refusal,
  failure recovery, path redaction, and exact inventory tests.

Safety contract:

- The command accepts no project/environment ID.
- Project key preflight must return zero exact matches.
- Every update/delete ID originates from this invocation's create response or
  its explicitly labelled exact-key reconciliation workaround.
- Auto-created environments are never mutated.
- A failure leaves an exact cleanup inventory; a second lifecycle is refused
  while any entry is pending.

Verification:

- `go test ./...`: passed.
- `go vet ./...`: passed.
- `go test -count=10 ./internal/probe`: passed.
- Focused lifecycle/ownership/redaction tests repeated 20 times: passed.
- OpenAPI deterministic check: passed.
- Explicit `--execute` command guard: passed.

Cleanup:

- No live API mutation ran.
- No remote object was created.
- All successful mock runs ended with zero pending cleanup entries.

Exit gate:

- **Not passed.** The live project/environment lifecycle evidence is pending.

Exact next action:

- Present the normal 17-request sequence to the operator.
- After confirmation, run exactly one live prefixed lifecycle.
- Require exact environment/project absence and zero pending cleanup entries.

## 2026-07-31 — Cloud project/environment lifecycle completed

Scope:

- Loaded the access token from the Windows user environment into one probe
  process without printing or persisting it.
- Used the synthetic `tfp0-20260731-codex02` prefix.
- Executed exactly one reviewed project/environment lifecycle against
  `cloud-current`.

Observed:

- All 17 documented public API requests returned HTTP 200 with
  `success=true`.
- Project Create/Read/name Update/repeated Read/Delete converged.
- Environment Create/Read/name-description Update/repeated Read/Delete
  converged.
- Project key and environment parent/key were preserved.
- Project creation returned `Dev/dev`, then `Prod/prod`.
- All three environments returned two secret objects with complete metadata
  and string values; values and identities were discarded.
- Project Delete succeeded with both automatic environments still present and
  final complete-list exact lookup returned zero matches.
- No exact-key reconciliation workaround was needed.

Completed task IDs:

- `P0-017`
- `P0-040`
- `P0-042` through `P0-046`

Remaining project task:

- `P0-041`: duplicate project-key behavior was not tested. Key remains
  conservatively replace-only from the public schema and observed preservation.

Cleanup:

- Explicit environment exact absence: verified.
- Project and automatic-environment cascade exact absence: verified.
- Preflight and final project collection count: 11.
- Cleanup inventory dry run: `pending=0`.
- Manual owner/action: none.

Exit gate:

- **Not passed.** Flag, segment, error/absence, IAM/member, self-hosted, and
  remaining ADR evidence is still pending.

Exact next action:

- Resolve `P0-041` conservatively or through one separately reviewed
  duplicate-key observation.
- Continue error/absence TODOs in dependency order.

## 2026-07-31 — Cloud project/environment compatibility completed

Scope:

- Extended the reusable Go lifecycle probe with opt-in validation,
  duplicate-key, direct post-delete, and exact collection checks.
- Added immediate exact inventory tracking and a fail-stop path if a duplicate
  request unexpectedly returns or lists a second identity.
- Added explicit workaround labels for project complete-collection and
  environment parent-collection post-delete fallbacks.
- Added mock coverage for normal compatibility behavior, unexpected duplicate
  creation and recovery, ID/path redaction, and zero cleanup.
- Loaded the access token from the Windows user environment into one probe
  process without printing or persisting it.
- Used the synthetic `tfp0-20260731-codex03` prefix and the existing approval
  limited to one newly created project and its newly created environment.

Observed:

- Project/environment empty-name Creates returned HTTP 400,
  `success=false`, code `name_is_required`, with zero exact matches.
- Project/environment duplicate-key Creates returned HTTP 422,
  `success=false`, code `KeyHasBeenUsed`, with exactly the original identity
  remaining.
- Both valid lifecycles converged through repeated canonical Reads.
- After successful environment Delete, direct Read returned HTTP 403,
  `success=false`, code `Forbidden`; exact parent lookup returned zero.
- After successful project Delete, direct Read returned HTTP 500,
  `success=false`, code `InternalServerError`; the complete project collection
  returned zero exact matches.
- The complete project collection was 11 before creation, 12 while present,
  and 11 after cleanup.
- No existing object, automatic environment, undocumented endpoint, private
  API, login flow, or backend was mutated.

Completed task IDs:

- `P0-030`
- `P0-033`
- `P0-034`
- `P0-037`
- `P0-041`

Partial task IDs:

- `P0-031`: project/environment duplicate rows verified; flag/segment rows
  remain blocked by mutation scope.
- `P0-032`: project/environment post-delete rows verified; flag/segment rows
  remain blocked by mutation scope.

Verification:

- `go test ./...`: passed.
- `go vet ./...`: passed.
- `go test -count=10 ./internal/...`: passed.
- Focused compatibility, unexpected-duplicate cleanup, and path-redaction
  tests repeated 25 times: passed.
- OpenAPI deterministic check: passed; 60 paths, 76 operations, 112 schemas,
  454 properties, zero missing operation IDs.
- Repository secret scan: passed without printing candidate values.
- Context link/evidence/TODO check: passed.
- Whitespace check: passed except expected Git LF-to-CRLF notices.

Cleanup:

- The new explicit environment and project were deleted.
- Project Delete removed its two automatic environments.
- Exact environment/project absence was verified through documented
  collection fallbacks.
- Final project count matched preflight at 11.
- Cleanup inventory dry run: `pending=0`.
- Manual owner/action: none.

Exit gate:

- **Not passed.** Feature-flag/segment rows, member invitation, self-hosted
  compatibility, and ADR-001/ADR-002/ADR-005 acceptance remain incomplete.
- Phase 1 was not started.

Exact next action:

- Audit the blocked flag/segment TODOs against documented public operations and
  record safe workaround or reduced-support decisions without production
  mutation.
- Do not run flag/segment/IAM/member writes unless a disposable target or new
  explicit reviewed scope is provided.
- Select/provision the exact `selfhosted-min` target needed by ADR-005.

## 2026-07-31 — Offline feature-flag lifecycle guardrails completed

Scope:

- Added a reusable opt-in feature-flag lifecycle to the existing Go
  project/environment probe.
- Restricted the flag to a project and environment created by the same
  invocation; no caller-supplied remote identity is accepted.
- Added all-page exact-key preflight/reconciliation, immediate cleanup
  inventory, specialized name update, same-value stale-revision probing,
  direct-read plus exact-list deletion confirmation, and failure recovery.
- Kept the generic PATCH operation excluded because the pinned public OpenAPI
  supplies no request schema.
- Made feature-flag and project/environment compatibility modes mutually
  exclusive.

Observed:

- Mocks prove fuzzy-first results are ignored and an exact key on a later page
  is selected.
- Ambiguous Create is not retried and is adopted only after exactly one
  all-page exact-key match.
- A narrow name update preserves all unrelated canonical fields.
- The stale-revision mock returns synthetic HTTP 409/`RevisionConflict`; this
  is classifier coverage, not a Cloud observation.
- After Delete, synthetic direct HTTP 404 does not remove state until the
  all-page exact-key count is zero.
- A controlled post-create failure leaves flag, environment, and project
  inventory entries that cleanup removes in dependency order.
- No live API request or production object was created in this session.

Completed task IDs:

- None. Deployment evidence required by the affected TODOs is still absent.

Partial task IDs:

- `P0-031`, `P0-032`, `P0-035`
- `P0-050`, `P0-052`, `P0-053`
- `P0-056`, `P0-057`, `P0-058`, `P0-059`

Verification:

- `go test ./...`: passed.
- `go vet ./...`: passed.
- `go test -count=10 ./internal/...`: passed.
- Focused feature-flag lifecycle, pagination, ambiguity, redaction, and
  recovery tests repeated 25 times: passed.
- Combined compatibility/feature-flag mode guard: passed.
- Secret scan: passed without printing candidate values.
- Cleanup inventory dry run: `pending=0`.

Cleanup:

- Live objects created: none.
- Runtime cleanup inventory: `pending=0`.
- Manual owner/action: none.

Exit gate:

- **Not passed.** No deployment target has run the feature-flag probe, segment
  work remains blocked, no self-hosted target is selected, and ADR-001,
  ADR-002, and ADR-005 remain Proposed.
- Phase 1 was not started.

Exact next action:

- Build the equivalent offline segment lifecycle guardrail without a
  production API request.
- Before any flag/segment live execution, review the exact sequence and obtain
  an approved disposable scope.

## 2026-07-31 — Offline environment-specific segment guardrails completed

Scope:

- Added a reusable opt-in environment-specific segment lifecycle to the
  existing Go project/environment probe.
- Restricted it to a project and environment created by the same invocation;
  no caller-supplied remote identity is accepted.
- Added all-page exact-key and exact-UUID reconciliation, immediate cleanup
  inventory, duplicate tracking, specialized updates, archive/restore,
  flag-reference preflight, exact deletion confirmation, and failure recovery.
- Excluded the schema-less generic PATCH.
- Excluded shared segment mutation because its scope can extend outside the
  owned environment and the public schema does not define exact scope-string
  encoding.

Observed:

- Mocks prove fuzzy key/UUID values on an earlier page are ignored.
- Ambiguous Create is not retried and is adopted only after exactly one
  all-page exact-key match.
- An unexpected accepted duplicate inventories both returned UUIDs and stops
  before parent cleanup.
- Name, description, targeting, tags, archive, and restore preserve unrelated
  canonical fields; key/type/scopes remain unchanged.
- A non-empty synthetic flag-reference collection skips Delete and preserves
  state.
- After Delete, synthetic direct HTTP 404 does not remove state until exact
  UUID and exact key counts are both zero.
- No live API request or production object was created in this session.

Completed task IDs:

- None. Deployment evidence required by the affected TODOs is still absent.

Partial task IDs:

- `P0-031`, `P0-032`, `P0-036`
- `P0-060`, `P0-061`, `P0-062`, `P0-063`, `P0-064`, `P0-065`

Support reduction:

- Environment-specific segment: constrained managed candidate, live evidence
  pending.
- Shared segment: read/bind only unless exact scope encoding and a disposable
  workspace are later verified.
- Referenced Delete: use the documented flag-reference preflight; do not rely
  on an unknown conflict response.

Verification:

- `go test ./...`: passed.
- `go vet ./...`: passed.
- Focused segment lifecycle, pagination, ambiguity, unexpected-duplicate,
  reference-preflight, redaction, and recovery tests repeated 25 times:
  passed.
- Combined lifecycle-mode guard: passed.
- Cleanup inventory dry run: `pending=0`.

Cleanup:

- Live objects created: none.
- Runtime cleanup inventory: `pending=0`.
- Manual owner/action: none.

Exit gate:

- **Not passed.** No deployment target has run the flag/segment probes, no
  self-hosted target is selected, and ADR-001, ADR-002, and ADR-005 remain
  Proposed.
- Phase 1 was not started.

Exact next action:

- Audit ADR-001, ADR-002, and ADR-005 against the explicit reductions; do not
  accept an ADR whose evidence criteria remain unmet.
- Obtain separately reviewed environment-specific flag/segment scope before
  any live execution; never include shared segment mutation implicitly.

## 2026-07-31 — Cleanup exact-absence recovery strengthened

Scope:

- Changed runtime cleanup from DELETE-status-only handling to a single DELETE
  followed by documented exact absence verification.
- Added project/environment complete-collection fallbacks and flag/segment
  all-page exact lookups.
- Added an owned project/environment cascade fallback for child collections
  that become unavailable after parent deletion.
- Added project identity to newly inventoried flag/segment entries.
- Replaced cleanup CLI identities with fixed redaction markers.

Observed:

- A synthetic DELETE 500 plus exact-zero segment list converges.
- A synthetic DELETE 200 plus an exact remaining segment UUID stays pending.
- Failed child list plus exact owned-parent absence converges.
- A still-present parent cannot turn a failed child lookup into absence.
- Runtime cleanup output helpers never return the exact synthetic UUID or key.
- No live API request or object was created.

Completed task IDs:

- None; this is a dated strengthening of completed `P0-007`, `P0-008`, and
  `P0-017`.

Verification:

- Full `go test ./...`: passed.
- Full `go vet ./...`: passed.
- Focused cleanup, cascade, recovery, duplicate, and output-redaction tests
  repeated 25 times: passed.
- Cleanup inventory dry run: `pending=0`.

Cleanup:

- Live objects created: none.
- Runtime cleanup inventory: `pending=0`.
- Manual owner/action: none.

Exit gate:

- **Not passed.** Flag/segment deployment rows, a self-hosted minimum, and
  ADR-001/ADR-002/ADR-005 acceptance remain blocked.
- Phase 1 was not started.

Exact next action:

- Add a read-only flag/segment collection check inside a newly owned
  project/environment and mock-test that it cannot make child mutations.
- Review its exact request sequence before a Cloud run under the existing
  project/environment-only mutation approval.

## 2026-07-31 — Cloud child missing-read compatibility completed

Scope:

- Added and mock-tested a `--child-read-checks` extension that creates only its
  own project/environment parents.
- Reviewed the exact request sequence before live execution.
- Loaded the access token from the Windows user environment into one process
  without printing or persisting it.
- Used the synthetic `tfp0-20260731-codex04` prefix.
- Sent only GET requests to flag/segment endpoints; made no child mutation.

Observed:

- Missing flag direct Read returned HTTP 404, `success=false`,
  `data=null`, code `ResourceNotFound`.
- The complete flag collection contained zero items and zero exact synthetic
  key matches.
- Missing segment direct Read returned HTTP 404, `success=false`,
  `data=null`, code `ResourceNotFound`.
- Two independent complete segment scans each returned a page containing three
  pre-existing rows, but zero exact synthetic UUID/key matches.
- No segment row identity, name, key, type, scope, or other field was retained;
  the first row was never selected.
- Project/environment lifecycle converged; the complete project collection was
  11 before and 11 after cleanup.

Completed task IDs:

- None. `P0-032`, `P0-035`, and `P0-036` still require child post-delete and
  exact present cases; `P0-061` still lacks a contained shared mutation.

Partial task IDs:

- `P0-032`: live missing baseline, not child post-delete.
- `P0-035`: live flag absent exact-key case.
- `P0-036`: live segment absent exact-UUID/key case.
- `P0-061`: cross-scope visibility risk strengthened without inspecting rows.

Verification:

- Full `go test ./...`: passed before the run.
- Full `go vet ./...`: passed before the run.
- Read-only child lifecycle repeated 25 times against mocks: passed.
- Tests rejected every non-GET flag/segment request.
- Preflight cleanup dry run: `pending=0`.
- Final cleanup dry run: `pending=0`.

Cleanup:

- The explicit environment and project were deleted.
- Project Delete removed its two automatic environments.
- Exact environment/project absence was verified through documented parent
  collections.
- Final project count matched preflight at 11.
- Manual owner/action: none.

Exit gate:

- **Not passed.** Flag/segment present, mutation, post-delete, complex round
  trips, self-hosted compatibility, and ADR-001/ADR-002/ADR-005 acceptance
  remain incomplete.
- Phase 1 was not started.

Exact next action:

- Obtain a newly reviewed environment-specific flag mutation scope before
  running the documented `--feature-flag-checks` sequence.
- Require exact cleanup zero, then review `--segment-checks` separately.
- Never include shared segment mutation; separately select/provision
  `selfhosted-min`.

## 2026-07-31 — Final validation after Cloud child-read run

Scope:

- Re-ran every repository/context guard after recording the third Cloud
  lifecycle.
- Made no API request and created no object.

Verification:

- `gofmt -l cmd internal`: empty.
- `go test ./...`: passed.
- `go vet ./...`: passed.
- `go test -count=10 ./internal/...`: passed.
- OpenAPI deterministic check: 60 paths, 76 operations, 112 schemas, 454
  properties, zero missing operation IDs; hashes unchanged.
- Context check: 35 Markdown files, 16 evidence files, 76 unique TODO IDs, 47
  completed TODOs, 2 Accepted ADRs, zero findings.
- Secret scan: 79 files, zero findings.
- Cleanup dry run: `pending=0`.
- Whitespace check: passed; only Git LF-to-CRLF notices were emitted.

Cleanup:

- Every Cloud object created by all three Phase 0 lifecycles has exact
  confirmed absence.
- Manual owner/action: none.

Exit gate:

- **Not passed.** The phase remains at 47/76 and 2/5 Accepted ADRs.
- Phase 1 was not started.

Exact next action:

- Obtain explicit reviewed approval for environment-specific
  `--feature-flag-checks`, run it alone, and require exact cleanup zero.
- Then separately review `--segment-checks`; shared segment mutation stays
  excluded.
- Select and provision the exact `selfhosted-min` release required by ADR-005.

## 2026-07-31 — Cloud Boolean flag CRUD and archive-before-Delete recovery

Session record opened: `2026-07-31T08:30:02Z`
Ended: `2026-07-31T08:38:19Z`

Baseline:

- Branch: `main`
- Commit: `4f04f4eec2b26f9df1358a34638d4189c861361d`
- The pre-existing dirty Phase 0 context, `internal/`, and `tools/` worktree
  was preserved; no reset, checkout, or unrelated rewrite was performed.
- `FEATBIT_TEST_SERVICE_TOKEN` was present. Personal-token input was absent and
  unnecessary. API URL, target, and a fresh safe prefix were supplied only to
  the probe process; no value was recorded.
- Cleanup dry run before mutation: `pending=0`.

Approved scope:

- The product owner approved exactly one environment-specific feature flag
  inside the next project/environment created by the probe.
- Duplicate Create, stale-revision writes, other flag types, segments, IAM,
  members, and all pre-existing resources were excluded.
- The existing full `--feature-flag-checks` mode exceeded that scope because it
  includes duplicate and stale-revision compatibility writes. A narrower
  `--feature-flag-crud-checks` mode was added in the same Go probe and proved to
  issue one flag Create, one normal name Update, and no variation update.

Live observation:

- Created and inventoried one disposable project, one explicit environment,
  and one Boolean flag. The project also created its two normal automatic
  environments; they were never individually mutated.
- Flag Create and exact Read returned HTTP 200. The two requested variation IDs
  were preserved, enabled fallthrough and disabled variation references were
  stable, and a revision was present.
- Specialized name Update returned HTTP 200, advanced the revision, and
  preserved every unrelated canonical field. Repeated canonical Reads were
  equal.
- Direct hard Delete of the unarchived flag returned HTTP 422,
  `success=false`, code `CannotDeleteUnarchivedFeatureFlag`. Direct Read
  remained HTTP 200 and the exact active key count remained one.
- The probe stopped safely with `pending=3`; it did not discard state or touch
  another object.

Workaround and safety correction:

- Recovery used the documented public archive endpoint as a transient
  prerequisite, required success, retried hard Delete once, and verified exact
  zero in both `IsArchived=false` and `IsArchived=true` paginated views.
- This two-view check corrects the previous fallback: the default collection
  alone cannot prove absence because archive hides a still-existing flag.
- Cleanup then removed the explicit environment and project in dependency
  order. Final dry run: `pending=0`; manual owner/action: none.
- No second flag was created. No segment or existing resource was read for
  selection, updated, archived, or deleted.

Completed task IDs:

- `P0-035`
- `P0-050`
- `P0-057`
- `P0-058`

Partial/open task effects:

- `P0-031`: flag duplicate deliberately not attempted; segment duplicate
  remains untested.
- `P0-032`: failed-Delete direct Read is verified, but no direct Read was
  retained after successful archive-plus-Delete; segment remains untested.
- `P0-052`, `P0-053`, `P0-056`: Boolean mapping, name isolation, archive, and
  revision advancement gained live evidence; generated IDs, operational
  specialized updates, and stale conflict remain open.
- `P0-051`, `P0-055`, `P0-059`, and `P0-060` through `P0-065` remain open.

Decision impact:

- Added `FND-0033`: Cloud flag destroy requires archive, then hard Delete, then
  active-plus-archived exact-zero confirmation.
- ADR-001 remains Proposed: one Boolean name update is insufficient to decide
  operational ownership and UI drift.
- ADR-002 remains Proposed: the Cloud flag path is verified, while restore/key
  reuse and all segment rows remain blocked.
- ADR-005 remains Proposed; `selfhosted-min` is unavailable.
- Progress is `51 / 76`; Accepted ADRs remain `2 / 5`.

Verification:

- `gofmt -l cmd internal`: empty.
- `go test ./...`: passed.
- `go vet ./...`: passed.
- `go test -count=10 ./internal/...`: passed.
- Focused one-create CRUD, archive-before-Delete lifecycle, and cleanup tests
  repeated 25 times: passed.
- OpenAPI deterministic check: 60 paths, 76 operations, 112 schemas, 454
  properties, zero missing operation IDs; hashes unchanged.
- Context check: 36 Markdown files, 17 evidence files, 76 unique TODO IDs, 51
  completed TODOs, 2 Accepted ADRs, zero findings.
- Secret scan: 80 files, zero findings.
- Cleanup dry run: `pending=0`.
- Whitespace check: passed; only Git LF-to-CRLF notices were emitted.

Exit gate:

- **Not passed.** Complex flag ownership, remaining flag compatibility rows,
  environment-specific segment lifecycle, self-hosted compatibility, and
  ADR-001/ADR-002/ADR-005 acceptance remain incomplete.
- Phase 1 was not started.

Exact next action:

- Obtain separate explicit approval for one newly owned environment-specific
  `--segment-checks` run, never including shared segments, and require cleanup
  zero before and after.
- Do not run more flag writes without another separately reviewed scope.
- Independently select/provision the exact `selfhosted-min` release.

## 2026-07-31 — Cloud environment-specific segment lifecycle completed

Approved scope:

- The product owner approved exactly one environment-specific segment inside
  the next project/environment created by the probe.
- Shared segments, duplicate Create, flags, IAM, members, and all pre-existing
  mutations were excluded.
- Existing visible segments could be read only by exact UUID to resolve an
  otherwise unavailable organization scope prefix; no returned identity,
  scope, or tenant value could be retained.

Probe correction:

- The existing segment request sent `scopes=[]`. Cloud returned HTTP 400,
  `success=false`, code `scopes_is_invalid`; no segment was created.
- The two exact parents remained inventoried and were deleted. Cleanup ended
  at `pending=0`.
- Public OpenAPI describes only a nullable string array. Official control-plane
  behavior uses a non-empty resource name. The Go probe now scans all active
  and archived pages, direct-Reads every exact returned UUID, requires one
  unique organization prefix, constructs the new environment scope in memory,
  and never serializes the prefix.
- A new `--segment-crud-checks` mode issues exactly one segment Create and
  skips the duplicate-Create compatibility write.

Live lifecycle:

- A valid-scope attempt created exactly one environment-specific segment and
  verified exact UUID/key presence.
- Name, description, targeting, and tags updates returned HTTP 200 and
  preserved unrelated canonical fields.
- Two synthetic included keys, one excluded key, one rule/condition, and two
  tags round-tripped. Archive and restore both converged; repeated complex
  canonical Reads were equal.
- `flag-references` returned an empty array.
- Direct hard Delete after restore returned HTTP 422,
  `CannotDeleteUnArchivedSegment`, and left the exact segment present.

Delete workaround and recovery:

- Lifecycle and cleanup now match that exact error code, archive the exact
  owned UUID, require success, retry hard Delete, and scan every page of both
  `IsArchived=false` and `IsArchived=true`.
- The first valid-scope attempt was recovered in dependency order: segment,
  environment, project. Cleanup ended at `pending=0`.
- A final fresh-prefix run completed the full workaround in one invocation.
  Retry Delete returned HTTP 200, direct Read returned
  404/`ResourceNotFound`, exact UUID/key counts were zero in both views, parent
  absence was verified, and cleanup ended at `pending=0`.

Completed task IDs:

- `P0-036`
- `P0-052`
- `P0-055`
- `P0-056`
- `P0-060`
- `P0-061` (shared management reduced to read/bind only)
- `P0-062`
- `P0-063`
- `P0-065`
- `P0-080`
- `P0-081`

Open/partial effects:

- `P0-031`: flag and segment duplicate Creates were deliberately excluded.
- `P0-032`: segment direct post-delete 404 plus exact zero is complete; the
  flag direct post-successful-delete row remains.
- `P0-064`: empty live reference preflight is verified; non-empty live behavior
  remains unavailable and fails closed offline.
- Multi-type/complex flag rows, IAM/member writes, and `selfhosted-min` remain
  unavailable.

Decision impact:

- Added `FND-0034`: Cloud environment-specific segments require a constrained
  resource-name scope resolver and their complex specialized updates converge.
- Added `FND-0035`: active Cloud segment Delete requires archive before hard
  Delete and two-view exact absence.
- ADR-001 is Accepted with a reduced flag contract: basic Boolean/name
  ownership only; description/variations replace; unverified operational
  fields are Computed-only or omitted. Environment-segment verified fields are
  owned; shared segments are read/bind only.
- ADR-002 is Accepted: reference preflight, archive, hard Delete, and exact
  active-plus-archived absence; restore/key-reuse guarantees are omitted.
- ADR-005 remains Proposed because `selfhosted-min` is unavailable and the
  Cloud matrix still has open rows.
- Progress is `62 / 76`; Accepted ADRs are `4 / 5`.

Verification:

- `gofmt -l cmd internal`: empty.
- `go test ./...`: passed.
- `go vet ./...`: passed.
- `go test -count=10 ./internal/...`: passed.
- Focused single-segment Create, ambiguous-scope fail-closed, lifecycle
  archive-before-Delete, and cleanup archive-before-Delete tests repeated 25
  times: passed.
- OpenAPI deterministic check: 60 paths, 76 operations, 112 schemas, 454
  properties, zero missing operation IDs; pinned hashes unchanged.
- Context check: 37 Markdown files, 18 evidence files, 76 TODO IDs, 62 checked
  tasks, 4 Accepted ADRs, zero findings.
- Secret scan: 81 files, zero findings.
- `git diff --check`: passed; only LF-to-CRLF notices were emitted.
- Runtime cleanup dry run: `pending=0`.
- Manual cleanup owner/action: none.

Exit gate:

- **Not passed.** ADR-005, exact self-hosted compatibility, remaining Cloud
  flag/IAM/member rows, and final closure tasks remain incomplete.
- Phase 1 was not started.

Exact next action:

- Select and provision the exact disposable `selfhosted-min` release and
  provide its required values only through the named environment variables.
- Do not run another Cloud mutation without a new separately reviewed scope.

## 2026-07-31 — Self-hosted prerequisite correction

- The product owner clarified that only the online Cloud environment exists;
  there is no `selfhosted-min` target and no ability to provision one for
  Phase 0.
- The previous handoff instruction to inject self-hosted URL/token/target/
  prefix variables was incorrect and is superseded. No such configuration is
  requested.
- ADR-005 now requires a product-scope decision: explicitly support only Cloud
  in v1, or leave self-hosted support unclaimed and keep the Phase 0 exit gate
  blocked.
- No API request or mutation was made in this clarification session. Cleanup
  remains `pending=0` from the preceding verified run.

## 2026-07-31 — Product deployment-scope clarification

- The product owner clarified that FeatBit customers use both SaaS and
  self-hosted deployments; provider v1 is intended to serve both.
- The preceding Cloud-only versus unclaimed-self-hosted choice is superseded.
  Cloud-only is not an acceptable scope reduction.
- The evidence status is unchanged: Cloud has target-specific observations;
  self-hosted remains `Not tested` because no approved disposable target
  exists. ADR-005 and the Phase 0 exit gate therefore remain blocked.
- No API request or mutation was made. Cleanup remains `pending=0`.

## 2026-07-31 — Available-target exit-gate correction

- Re-reading Phase 0 plan lines 196 and 221 showed that an unavailable target
  must remain unverified and only available targets must have a complete
  compatibility matrix.
- The preceding claim that missing self-hosted evidence alone blocks Phase 0
  was too strict and is superseded. Self-hosted has no failed observation; it
  has no observation and remains `Not tested`.
- Provider v1 remains intended for both deployment modes under the pinned
  public REST API contract. No exact self-hosted release is certified.
- Phase 0 still has unresolved available-Cloud, member, and closure rows. No
  API request or mutation was made; cleanup remains `pending=0`.

## 2026-07-31 — Provider-v1 executive scope clarification

- The Phase 0 probes are compatibility research, not a production Terraform
  provider implementation. No Phase 1 resource implementation has started.
- The verified core v1 candidate is a configurable Cloud/self-hosted provider
  with direct access-token auth plus project, environment, feature-flag, and
  environment-specific segment resources, their single-object data sources,
  and Import.
- The remaining material product choice is feature-flag breadth. The currently
  accepted conservative contract manages a basic Boolean flag and leaves
  enabled state, targeting, rules, rollouts, and tags UI-owned. Broader v1 flag
  ownership requires additional contained Cloud evidence.
- IAM, member creation, secret rotation, shared-segment mutation, analytics,
  and FeatBit deployment are outside the core first release.
- No API request or mutation was made; cleanup remains `pending=0`.

## 2026-07-31 — Four-type feature-flag v1 decision and offline probe

- The product owner accepted UI ownership of targeting and rollout but required
  Boolean, String, Number, and JSON feature flags in provider v1.
- The public Feature Flag contract confirms all four `variationType` values and
  transports every variation value as a string. The provider contract uses
  Boolean lowercase, byte-exact String, exact-precision Number, and canonical
  JSON validation.
- ADR-001 and `FND-0037` now make type, description, and variations
  `RequiresReplace`; enabled state, targeting, rules, rollouts, and tags remain
  Computed-only or omitted. No unverified operational writer was added.
- The reusable Go probe gained
  `--feature-flag-type-matrix-checks`. Under one owned project/environment it
  sequentially exercises String, Number, and JSON and proves each key absent
  before continuing. Mock coverage observed three Creates, three name-only
  Updates, no variation/targeting writes, archive-before-Delete, exact two-view
  absence, and `pending=0`.
- `go test ./...` passed. No API request or remote mutation was made; remote
  cleanup remains `pending=0`.

Exact next action:

- Obtain explicit approval before running the three-child Cloud type matrix.
- If approved, run only the documented mode above, sanitize the observation,
  update `P0-051`/`P0-059`, and verify cleanup is again `pending=0`.

## 2026-07-31 — Cloud String/Number/JSON type-matrix verification

- The product owner explicitly approved three sequential feature flags inside
  one newly created project/environment, with no targeting, rollout, enabled,
  tag, or post-Create variation update.
- The contained `cloud-current` run completed String, Number, and JSON Create,
  exact Read, name-only Update, repeated canonical Read,
  archive-plus-hard-Delete, direct post-delete 404, and exact-zero active and
  archived scans.
- Requested variation identities, enabled/disabled references, exact Number
  precision, and canonical JSON were preserved. The JSON
  desired-versus-observed comparison was logically empty.
- The three flags ran sequentially and each was proven absent before the next
  Create. The explicit environment and project were then removed.
- The command exited successfully. Its final cleanup report and a separate
  `cleanup --dry-run` both reported `pending=0`.
- The sanitized observation is recorded in
  [the Cloud type-matrix evidence](evidence/20260731-cloud-feature-flags.md).

Exact next action:

- Audit Phase 0-created files by reference and reproducibility need. Remove
  only verified transient or redundant files, then run context, redaction,
  determinism, and secret checks before resolving the remaining conservative
  capability classifications and ADR-005.

## 2026-07-31 — Post-verification file cleanup

- Deleted the empty ignored runtime cleanup inventory after two independent
  checks reported `pending=0`. Its sanitized final state remains in evidence;
  runtime identities were intentionally not retained.
- Consolidated the now-superseded offline String/Number/JSON guardrail record
  into the Cloud type-matrix evidence, which contains the same mutation
  boundary, canonicalization table, operation sequence, test command, and
  cleanup result. The redundant offline evidence file was removed and every
  reference was redirected to the target-specific evidence.
- Retained `tools/api-probe` because reusable Go probe logic is a Phase 0
  deliverable. Retained `internal/client/openapi` because ADR-003 requires its
  pinned and deterministic Phase 1 generation inputs. All other evidence/ADR
  files remain linked to a TODO, finding, decision, or compatibility row.

## 2026-07-31 — Phase 0 final closure

- Closed the remaining unavailable live rows with explicit safe behavior:
  generic 401/403 handling, behavior-independent child duplicate
  reconciliation, fail-closed referenced-segment preflight, external member
  creation, and IAM/member lookup deferral from core v1.
- Accepted ADR-005 after completing the matrix for `cloud-current`, the only
  available target. Self-hosted remains `Not tested`; no exact release is
  certified and no incompatibility is claimed.
- Replaced stale interim status/handoff prose with concise final documents.
  All 76 TODOs now have linked evidence or an explicit constrained,
  read/bind-only, external, omitted, or unavailable-target disposition.
- `gofmt`, full Go tests, `go vet`, 20 repeated normalization/probe runs,
  deterministic OpenAPI verification, repository secret scanning, deleted-file
  reference checks, and cleanup checks passed. `go test -race` remains a local
  environment limitation because no C compiler is installed; Phase 1 CI owns
  that check.
- All Cloud-created objects are exactly absent; cleanup is `pending=0` and no
  manual owner/action remains.
- Phase state is `Complete — Ready for Phase 1`. Phase 1 was not started.

Exact next action:

- Execute the [Phase 1 package](../plan-execution-phase-1/README.md), beginning
  with the Protocol v6 provider/client scaffold and Phase 1 context package.
  Preserve the current worktree and do not implement production resources in
  Phase 1.

## 2026-08-01 — Compact Phase 1 package and evidence consolidation

- Created the formal compact Phase 1 package with only
  [README](../plan-execution-phase-1/README.md),
  [plan](../plan-execution-phase-1/plan.md), and
  [TODO](../plan-execution-phase-1/todo.md). Implementation did not start.
- Removed the transitional Phase 1 execution prompt from the completed Phase 0
  directory and redirected the master plan, Phase 0 README, protocol, status,
  handoff, and historical link to the formal Phase 1 package.
- Updated `AGENTS.md` so future phases use the compact documents declared by
  their README instead of automatically creating unread status/log/handoff
  files.
- Consolidated 19 Phase 0 evidence records into five reader-oriented records:
  Cloud authentication, project/environment, feature flags, segments, and
  offline contracts. Updated every TODO, finding, ADR, matrix, status, handoff,
  and session-log link before deleting the superseded records.
- No API request, remote mutation, credential read, or provider implementation
  occurred. Remote cleanup remains `pending=0`.
- Validation passed: Phase 0 context has 23 Markdown files, five evidence
  records, 76/76 TODOs, five Accepted ADRs, and zero link/evidence findings;
  Phase 1 has three Markdown files and 38 unique TODO IDs; the repository
  secret scan covered 70 files with zero findings; whitespace check passed.

Exact next action:

- Review and simplify the remaining completed Phase 0 top-level documents and
  `tools/` as a separate task. Do not begin Phase 1 implementation as part of
  that cleanup.
