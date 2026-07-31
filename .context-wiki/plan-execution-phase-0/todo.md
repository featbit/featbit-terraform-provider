# Phase 0 TODO List

Status: **In progress**
Plan: [plan.md](plan.md)
Protocol: [context-protocol.md](context-protocol.md)

## Completion rules

- `[ ]` means incomplete.
- `[x]` means verified and linked to evidence or an accepted ADR.
- A blocked item remains unchecked and ends with `BLOCKED: <reason>`.
- A conditional item may be marked `N/A` only with a finding explaining why it is outside the supported surface.
- Do not mark a task complete based only on an undocumented manual observation.

## A. Session baseline and safety

- [x] **P0-001** Read `AGENTS.md`, the master plan, this execution package, current status, and handoff.
  Evidence: first Phase 0 entry in `session-log.md` lists the documents and selected task IDs.

- [x] **P0-002** Record the initial Git branch, commit, dirty-worktree files, date, and operator/session scope.
  Evidence: sanitized baseline entry in `session-log.md`.

- [x] **P0-003** Define the approved Cloud and self-hosted test targets without recording private tenant identifiers.
  Evidence: target rows in `compatibility-matrix.md`.

- [x] **P0-004** Define a unique test-resource prefix and reject an empty or unsafe prefix.
  Evidence: probe configuration test and finding.

- [x] **P0-005** Define environment-variable-only credential inputs and add a value-free example/configuration description.
  Evidence: tracked configuration contains variable names but no values.

- [x] **P0-006** Implement base-URL safety checks or an explicit allowlist for mutating probes.
  Evidence: unit test showing an unapproved target is rejected.

- [x] **P0-007** Create a cleanup inventory that records every test-created remote identity.
  Evidence: evidence README or probe output format plus a dry-run example.
  Dated supplement: exact identities stay only in the ignored runtime
  inventory and CLI output uses fixed redaction markers; see
  [cleanup exact-fallback evidence](evidence/20260731-offline-contracts.md).

- [x] **P0-008** Establish a redaction policy for tokens, secret values, response headers, emails, and tenant identifiers.
  Evidence: redaction test and [evidence/README.md](evidence/README.md).
  Dated supplement: cleanup path observations use templates and runtime
  UUIDs/keys are never printed; see
  [cleanup exact-fallback evidence](evidence/20260731-offline-contracts.md).

## B. OpenAPI baseline and reusable probe

- [x] **P0-010** Download the OpenAPI document reproducibly and store the pinned snapshot at the ADR-003-approved path.
  Evidence: committed snapshot and source URL.

- [x] **P0-011** Calculate and record the snapshot SHA-256.
  Evidence: finding and reproducible command.

- [x] **P0-012** Reconfirm path, operation, schema, property, and security-scheme counts.
  Evidence: sanitized OpenAPI inventory report.

- [x] **P0-013** Create the local overlay with stable, unique operation IDs for core Phase 0 operations.
  Evidence: overlay plus validation showing no duplicate operation IDs.

- [x] **P0-014** Pin the overlay/generation tool and prove the command is deterministic.
  Evidence: two runs produce no diff.

- [x] **P0-015** Implement a minimal Go API probe with timeout, cancellation, token auth, envelope parsing, and redaction.
  Evidence: unit/mock tests plus the sanitized Cloud read in [20260731-cloud-auth.md](evidence/20260731-cloud-auth.md).

- [x] **P0-016** Add paginated exact-match helpers for ID, key, and normalized email.
  Evidence: unit tests including multiple pages, no match, one match, and duplicate match.

- [x] **P0-017** Add created-resource tracking and cleanup execution to the probe.
  Evidence: repeated mock failure/recovery tests plus the cleaned live lifecycle
  in [20260731-cloud-project-environment.md](evidence/20260731-cloud-project-environment.md).
  Dated supplement: cleanup now marks project/environment/flag/segment entries
  clean only after exact absence or owned-parent cascade proof; see
  [cleanup exact-fallback evidence](evidence/20260731-offline-contracts.md).

- [x] **P0-018** Ensure probe logs never emit `Authorization` or environment secret values.
  Evidence: automated log assertion and repository secret scan.

## C. Authentication and context

- [x] **P0-020** Verify a service access token sent directly in `Authorization`.
  Evidence: sanitized successful request observation in [20260731-cloud-auth.md](evidence/20260731-cloud-auth.md).

- [x] **P0-021** Verify a personal access token sent directly in `Authorization`, if available.
  Evidence: `N/A`; the product owner confirmed that “personal” and “service”
  are not distinct provider authentication contracts. They use the same direct
  access-token transport and permission model; see `FND-0022`.

- [x] **P0-022** Observe missing, malformed, inactive, and insufficient-scope token behavior where safely testable.
  Evidence: status/envelope table without token values.
  CONSTRAINED: missing and synthetic-malformed cases are target-verified.
  Controlled inactive and insufficient-scope tokens are unavailable, so those
  rows remain explicitly `Not tested`. Provider v1 does not depend on their
  error body/code: every 401 is authentication failure and every 403 is
  authorization failure; both preserve state and never retry a mutation. See
  [Cloud negative-auth evidence](evidence/20260731-cloud-auth.md)
  and [FND-0011](findings.md#fnd-0011--missing-and-malformed-cloud-authorization-is-structured-401).

- [x] **P0-023** Confirm that the supported public workflow requires no username/password login, JWT refresh, MFA, or SSO handling.
  Evidence: official documentation link plus probe findings.

- [x] **P0-024** Verify whether any organization/workspace context headers are required.
  Evidence: the successful request used only the documented direct `Authorization` header; see [20260731-cloud-auth.md](evidence/20260731-cloud-auth.md).

- [x] **P0-025** Record the Phase 1 provider authentication schema and multi-context alias approach.
  Evidence: accepted auth finding referenced by `handoff.md`.

## D. Error, absence, and retry behavior

- [x] **P0-030** Record HTTP status and FeatBit envelope behavior for validation errors.
  Evidence: project/environment empty-name Creates returned sanitized HTTP 400,
  `success=false`, code `name_is_required`; see
  [Cloud compatibility evidence](evidence/20260731-cloud-project-environment.md).

- [x] **P0-031** Record duplicate identity behavior for each applicable core family.
  Evidence: project/environment/flag/segment duplicate matrix.
  CONSTRAINED: project and environment duplicate keys returned HTTP 422,
  `success=false`, code `KeyHasBeenUsed`, and the exact identity set remained
  one. Flag and environment-specific segment duplicate/all-page exact-set
  guards pass offline. The one-flag and one-segment Cloud approvals
  deliberately excluded a second Create, so their target-specific status/code
  rows remain `Not tested`. The v1 workflow is behavior-independent: require
  all-page exact zero before one Create; on any ambiguous failure, re-read all
  pages and never adopt or retry blindly. Zero is a failed Create, one produces
  an exact Import diagnostic, and multiple matches fail safely. See
  [offline flag guardrails](evidence/20260731-offline-contracts.md)
  [offline segment guardrails](evidence/20260731-offline-contracts.md),
  and
  [Cloud compatibility evidence](evidence/20260731-cloud-project-environment.md).

- [x] **P0-032** Record post-delete read behavior for each core family.
  Evidence: status/envelope observations.
  Evidence: Cloud environment direct Read returned 403/`Forbidden` and project
  direct Read returned 500/`InternalServerError` after successful Delete; exact
  collection fallbacks proved both absent. Cloud flag and segment direct Reads returned
  404/`ResourceNotFound` after successful archive-plus-Delete, followed by
  exact-zero active and archived collection scans. See
  [Cloud type-matrix evidence](evidence/20260731-cloud-feature-flags.md)
  and [Cloud segment evidence](evidence/20260731-cloud-segments.md).

- [x] **P0-033** Prove exact project absence through a fully paginated exact-ID fallback.
  Evidence: present and absent cases.
  Evidence: the pinned public operation is a complete non-paginated collection;
  it contained the created ID exactly once before Delete and zero times after
  Delete; see
  [Cloud compatibility evidence](evidence/20260731-cloud-project-environment.md).

- [x] **P0-034** Prove exact environment absence through its parent project's environment collection.
  Evidence: present and absent cases.
  Evidence: the new environment appeared exactly once in the new parent
  project before Delete and zero times afterward; see
  [Cloud compatibility evidence](evidence/20260731-cloud-project-environment.md).

- [x] **P0-035** Prove exact feature-flag absence through a fully paginated exact-key fallback.
  Evidence: present and absent cases.
  Evidence: the Cloud flag appeared exactly once after Create, remained once
  after rejected direct Delete, and was zero across complete
  `IsArchived=false` plus `IsArchived=true` views after archive-plus-Delete;
  fuzzy-first rejection remains covered offline. See
  [Cloud flag lifecycle](evidence/20260731-cloud-feature-flags.md)
  and [offline flag guardrails](evidence/20260731-offline-contracts.md).

- [x] **P0-036** Prove exact segment absence through an exact ID or fully paginated exact-ID fallback.
  Evidence: present and absent cases.
  Evidence: the owned Cloud segment appeared exactly once by UUID and key
  before Delete. After archive-plus-hard-Delete, direct Read returned
  404/`ResourceNotFound` and complete `IsArchived=false` plus
  `IsArchived=true` scans returned zero exact UUID/key matches. See
  [Cloud segment lifecycle](evidence/20260731-cloud-segments.md)
  and [offline segment guardrails](evidence/20260731-offline-contracts.md).

- [x] **P0-037** Observe `401` and `403` and verify they never remove state.
  Evidence: error-classifier tests.
  Evidence: missing/malformed access tokens returned 401; an environment
  post-delete direct Read returned 403, which remained unconfirmed until an
  exact parent lookup returned zero. Classifier tests preserve state on both;
  see [negative-auth evidence](evidence/20260731-cloud-auth.md)
  and [Cloud compatibility evidence](evidence/20260731-cloud-project-environment.md).

- [x] **P0-038** Probe or mock `429`, transient `5xx`, timeout, cancellation, and network interruption handling.
  Evidence: retry classification tests; live tests are optional when unsafe.

- [x] **P0-039** Define the centralized error-classification decision table for Phase 1.
  Evidence: finding linked by `handoff.md`.

## E. Project and environment lifecycle

- [x] **P0-040** Probe project create/read/update/delete and canonical second Read.
  Evidence: sanitized lifecycle and cleanup record in [20260731-cloud-project-environment.md](evidence/20260731-cloud-project-environment.md).

- [x] **P0-041** Record project key mutability and duplicate-key behavior.
  Evidence: finding supporting `RequiresReplace` or safe update.
  Evidence: the public Update schema omits key, live name Update preserved key,
  and duplicate Create returned HTTP 422/`KeyHasBeenUsed` while exactly one
  original ID remained. Project key is `RequiresReplace`; see
  [Cloud compatibility evidence](evidence/20260731-cloud-project-environment.md).

- [x] **P0-042** Record automatically created environment count, names, keys, settings, and ordering.
  Evidence: two ordered automatic environments and redacted settings/secret
  metadata in [20260731-cloud-project-environment.md](evidence/20260731-cloud-project-environment.md).

- [x] **P0-043** Probe environment create/read/update/delete and canonical second Read.
  Evidence: sanitized lifecycle and cleanup record in [20260731-cloud-project-environment.md](evidence/20260731-cloud-project-environment.md).

- [x] **P0-044** Record environment key and parent mutability.
  Evidence: the public Update schema omits both, and the live name/description
  Update preserved exact parent/key; see `FND-0024`.

- [x] **P0-045** Observe project/environment delete cascade, blocking, or asynchronous behavior.
  Evidence: explicit environment Delete and project Delete with two automatic
  environments still present, followed by exact absence and `pending=0`; see
  [20260731-cloud-project-environment.md](evidence/20260731-cloud-project-environment.md).

- [x] **P0-046** Inventory environment secret metadata and whether values are returned consistently, without recording values.
  Evidence: field-presence/type counts across three environments and repeated
  Reads in [20260731-cloud-project-environment.md](evidence/20260731-cloud-project-environment.md).

- [x] **P0-047** Decide metadata-only versus opt-in Sensitive secrets data source behavior.
  Evidence: accepted finding or ADR consequence addressing Terraform state security.

## F. Feature flag lifecycle and normalization

- [x] **P0-050** Probe boolean flag create/read/update/delete.
  Evidence: sanitized lifecycle record.
  Evidence: one owned Cloud boolean flag completed Create, exact Read,
  specialized name Update, archive-prerequisite hard Delete, exact active and
  archived absence, and cleanup; see
  [Cloud flag lifecycle](evidence/20260731-cloud-feature-flags.md).

- [x] **P0-051** Repeat the round trip for string, number, and JSON variation types.
  Evidence: canonicalization table for all four types.
  Evidence: the approved one-parent/three-child Cloud probe completed String,
  Number, and JSON Create, exact Read, name-only Update, repeated canonical
  Read, archive-plus-Delete, direct 404, and two-view exact absence, ending at
  `pending=0`. See
  [Cloud type-matrix evidence](evidence/20260731-cloud-feature-flags.md)
  and [FND-0037](findings.md#fnd-0037--v1-supports-all-four-public-flag-types-with-immutable-type-and-variations).

- [x] **P0-052** Map `enabledVariationId`, `fallthrough`, disabled variation, and server-generated variation IDs.
  Evidence: bidirectional mapping finding.
  Evidence: the live Boolean response preserved both client-supplied variation
  IDs, referenced the enabled ID through fallthrough, and preserved the
  disabled ID. v1 always supplies IDs on Create and retains exact returned IDs
  on Import; it does not depend on the omitted/server-generated path. See
  [FND-0036](findings.md#fnd-0036--reduced-v1-boolean-mapping-avoids-unverified-id-and-revision-writes).

- [x] **P0-053** Probe variations, metadata, enabled state, targets, rules, rollouts, and tags through specialized endpoints.
  Evidence: operation sequence and canonical final Read.
  REDUCED: specialized name and archive operations are live-verified and the
  final Read preserved unrelated fields. The product owner confirmed that
  enabled state, targeting, rules, and rollouts remain UI-managed; tags are
  Computed-only or omitted. Type/description/variations are Create-owned and
  `RequiresReplace`, so v1 invokes no unverified specialized writer. See
  [ADR-001](adrs/ADR-001-terraform-ui-ownership.md).

- [x] **P0-054** Record ordering semantics for variations, rules, conditions, tags, and targets.
  Evidence: List/Set decision table.

- [x] **P0-055** Record JSON and condition-value normalization.
  Evidence: repeated Read produces byte-equivalent canonical state.
  Evidence: exact-precision JSON/number and condition-list canonicalization
  repeatedly produces byte-equivalent state offline; the Cloud segment
  condition JSON-string representation also survived repeated complex Reads.
  JSON variation flags are in the v1 type set through the documented
  Create/Read plus immutable-field workaround; all four types are now
  target-verified on `cloud-current`. See
  [normalization evidence](evidence/20260731-offline-contracts.md)
  [Cloud type-matrix evidence](evidence/20260731-cloud-feature-flags.md),
  and [Cloud segment lifecycle](evidence/20260731-cloud-segments.md).

- [x] **P0-056** Observe revision use and a stale-revision conflict where safely reproducible.
  Evidence: status/envelope and retry/no-retry decision.
  Evidence: Cloud returned a revision and advanced it after the narrow name
  update. The safe same-value contract classifies 409/`RevisionConflict`; v1
  treats revision as Computed, sends no revision-bearing operational write,
  refreshes, preserves state, and never retries blindly. See
  [FND-0036](findings.md#fnd-0036--reduced-v1-boolean-mapping-avoids-unverified-id-and-revision-writes).

- [x] **P0-057** Verify that a narrow update does not rewrite an unrelated targeting field.
  Evidence: before/after comparison.
  Evidence: before/after Cloud canonical comparison around the specialized
  name Update preserved every unrelated field, including empty targets/rules,
  variations, tags, enabled state, fallthrough, and description; see
  [Cloud flag lifecycle](evidence/20260731-cloud-feature-flags.md).

- [x] **P0-058** Inject a controlled failure after base creation and verify state/import recovery.
  Evidence: orphan handling, recovered identity, cleanup, and exact Import command.
  Evidence: live direct Delete failure preserved three exact owned identities;
  archive-before-Delete recovery removed flag/environment/project in dependency
  order and ended at zero. The exact Import command/parser is linked from the
  combined recovery evidence; see
  [Cloud flag lifecycle](evidence/20260731-cloud-feature-flags.md)
  and
  [normalization/recovery](evidence/20260731-offline-contracts.md).

- [x] **P0-059** Demonstrate an empty logical diff after creating and reading one complex flag.
  Evidence: normalized desired-versus-observed comparison.
  Evidence: the Cloud JSON flag's normalized desired-versus-observed
  comparison and repeated canonical Reads were equal. This is the complex
  Create-owned JSON variation contract; UI-owned targeting, rules, rollouts,
  enabled state, and tags were deliberately not written. See
  [Cloud type-matrix evidence](evidence/20260731-cloud-feature-flags.md).

## G. Segment lifecycle and normalization

- [x] **P0-060** Probe environment-specific segment create/read/update/delete.
  Evidence: sanitized lifecycle record.
  Evidence: one approved Cloud segment completed Create, exact Read, all
  specialized updates, archive/restore, archive-before-hard-Delete, direct
  post-delete 404, exact two-view absence, and parent cleanup. See
  [Cloud segment lifecycle](evidence/20260731-cloud-segments.md).

- [x] **P0-061** Probe shared segment creation and scopes.
  Evidence: sanitized lifecycle record or documented unsupported result.
  REDUCED: provider v1 classifies shared segments as **Read/bind only**.
  Exact public Reads of the three rows visible from a fresh Cloud environment
  confirmed resource-name scope strings can supply an in-memory organization
  prefix, but no existing row was mutated and no cross-scope Create/Update/
  Delete was attempted. The OpenAPI still omits the scope grammar and the
  blast radius exceeds the approved disposable parents. See
  [Cloud segment lifecycle](evidence/20260731-cloud-segments.md)
  and [FND-0034](findings.md#fnd-0034--cloud-environment-specific-segments-need-resource-name-scopes-and-converge).

- [x] **P0-062** Probe rules, included/excluded users, tags, and archive/restore.
  Evidence: operation sequence and canonical final Read.
  Evidence: Cloud persisted two synthetic included keys, one excluded key, one
  rule/condition, and two tags. Archive and restore each preserved unrelated
  canonical fields. See
  [Cloud segment lifecycle](evidence/20260731-cloud-segments.md).

- [x] **P0-063** Observe safe update behavior for key, type, and scopes.
  Evidence: matrix; v1 remains `RequiresReplace` unless every supported target is deterministic.
  Evidence: no documented specialized update exposes key, type, or scopes.
  The Cloud update sequence never resent them and every exact Read preserved
  all three. They remain `RequiresReplace`; see
  [Cloud segment lifecycle](evidence/20260731-cloud-segments.md)
  and [FND-0018](findings.md#fnd-0018--segment-key-type-and-scopes-remain-replace-only).

- [x] **P0-064** Observe deletion when a feature flag references the segment.
  Evidence: conflict/error behavior and cleanup steps.
  CONSTRAINED/WORKAROUND: the live documented `flag-references` preflight returned
  an empty array before successful archive-plus-Delete. A non-empty mock
  response skips Delete and preserves state so the user can remove exact
  references first. Provider v1 therefore never depends on a live server-side
  conflict shape: non-empty references prevent DELETE, while a race or any
  ambiguous DELETE response preserves state and is not retried. The live
  non-empty status/envelope remains explicitly `Not tested`; see
  [Cloud segment lifecycle](evidence/20260731-cloud-segments.md)
  and [offline segment guardrails](evidence/20260731-offline-contracts.md).

- [x] **P0-065** Demonstrate an empty logical diff after creating and reading one complex segment.
  Evidence: normalized desired-versus-observed comparison.
  Evidence: each live specialized update matched its canonical desired value,
  preserved unrelated fields, and repeated final complex Reads were equal.
  See
  [Cloud segment lifecycle](evidence/20260731-cloud-segments.md).

## H. Member and IAM feasibility

- [x] **P0-070** Verify paginated member lookup and normalized exact-email filtering.
  Evidence: zero, one, and duplicate exact-match tests.
  DEFERRED/READ-BIND ONLY: zero, one, duplicate, and fuzzy-first all-page exact
  email tests pass offline. The authenticated member endpoint is unavailable,
  so no member data source ships in core v1; target verification is an entry
  gate for the later IAM stage. See
  [member/IAM evidence](evidence/20260731-offline-contracts.md)
  and [FND-0019](findings.md#fnd-0019--member-creation-is-an-external-prerequisite-for-v1).

- [x] **P0-071** Preflight absence, call `POST /members/add`, and poll for one exact member ID in a disposable scope.
  Evidence: timing and result without recording real email or password values.
  EXTERNAL/OMITTED: no approved disposable member email/target/credential is
  available, and the public Add response contains no member ID. Core v1 omits
  member creation instead of assuming synchronous reconciliation; members are
  provisioned externally. See
  [member/IAM evidence](evidence/20260731-offline-contracts.md).

- [x] **P0-072** Determine whether invitations or human acceptance prevent synchronous reconciliation.
  Evidence: verified workflow finding.
  EXTERNAL/OMITTED: invitation timing and human acceptance remain `Not tested`
  because no approved disposable workflow exists. This ambiguity is removed
  from core v1 by omitting managed member creation; a future IAM-stage probe
  must supersede the decision before such a resource can ship. See
  [FND-0019](findings.md#fnd-0019--member-creation-is-an-external-prerequisite-for-v1).

- [x] **P0-073** Classify managed member creation as supported or external.
  Evidence: support-level decision; do not leave this ambiguous.

- [x] **P0-074** Verify that group/policy/member relationships have stable IDs suitable for independent binding resources.
  Evidence: endpoint and composite-identity finding.

## I. ADRs and capability classification

- [x] **P0-080** Write and accept ADR-001: Terraform versus UI ownership.
  Evidence: accepted ADR linked to flag findings.
  Evidence: [ADR-001](adrs/ADR-001-terraform-ui-ownership.md) accepts verified
  Boolean/name and environment-segment ownership plus the public/offline
  four-type Create/Read contract. Unverified flag enabled/target/rule/rollout/
  tag behavior is Computed-only or omitted; type/description/variations replace
  rather than use unverified writes.

- [x] **P0-081** Write and accept ADR-002: Delete versus archive.
  Evidence: accepted ADR linked to lifecycle findings.
  Evidence: [ADR-002](adrs/ADR-002-delete-vs-archive.md) requires reference
  preflight, archive, hard Delete, and exact active-plus-archived absence.
  Restore/key-reuse guarantees are omitted and non-empty references fail
  closed.

- [x] **P0-082** Write and accept ADR-003: Generated OpenAPI client and local overlay.
  Evidence: accepted ADR plus deterministic generation proof.

- [x] **P0-083** Write and accept ADR-004: Public Import ID formats.
  Evidence: accepted ADR covering every planned resource/binding.

- [x] **P0-084** Write and accept ADR-005: Supported Cloud/self-hosted/Go/Terraform compatibility matrix.
  Evidence: accepted ADR and completed matrix.
  Evidence: [ADR-005](adrs/ADR-005-supported-compatibility-matrix.md) accepts
  the toolchain and deployment-neutral public-API contract after completing
  every available Cloud core row or explicitly constraining, externalizing, or
  omitting unverified behavior. Self-hosted remains unavailable/`Not tested`;
  no exact release is falsely certified.

- [x] **P0-085** Classify every candidate capability into one of the five support levels.
  Evidence: capability table in `findings.md`.

- [x] **P0-086** Map every Phase 1 client responsibility to a finding, ADR, or explicit test requirement.
  Evidence: implementation input section in `handoff.md`.

## J. Closure and Phase 1 readiness

- [x] **P0-090** Rerun required probes on every available supported target.
  Evidence: completed compatibility matrix with dates.
  Evidence: `cloud-current`, the only available target, completed direct auth,
  parent lifecycle, all four constrained flag types, one complex
  environment-specific segment, exact absence, normalization, and cleanup on
  2026-07-30/31. Remaining live gaps have explicit constrained/external/
  omitted decisions. The unavailable self-hosted row remains `Not tested` and
  is not counted as an available target; see
  [compatibility matrix](compatibility-matrix.md).

- [x] **P0-091** Delete all test-created remote objects or list an owner and exact manual cleanup action.
  Evidence: empty or explicitly owned cleanup inventory.

- [x] **P0-092** Run repository credential/secret checks over tracked and untracked Phase 0 artifacts.
  Evidence: sanitized command and successful result.

- [x] **P0-093** Review every completed TODO for linked evidence.
  Evidence: no checked task lacks evidence.

- [x] **P0-094** Update `status.md` with final decisions, supported scope, risks, and exit-gate result.
  Evidence: status is current and internally linked.

- [x] **P0-095** Complete `handoff.md` with exact Phase 1 inputs and no unresolved implementation assumption.
  Evidence: handoff links all five accepted ADRs and compatibility findings.
  Evidence: [handoff.md](handoff.md) links all five Accepted ADRs, pinned
  OpenAPI/toolchains, exact client/resource contracts, reductions, target
  boundaries, verification, cleanup, and the Phase 1 entry action.

- [x] **P0-096** Append the final Phase 0 session-log entry and declare `Ready for Phase 1`.
  Evidence: final chronological record.
  Evidence: [status.md](status.md) says `Complete — Ready for Phase 1`, and the
  final chronological record below is linked from [handoff.md](handoff.md).
