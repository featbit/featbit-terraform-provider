# Phase 0 TODO List

Status: **Not started**  
Plan: [plan.md](plan.md)  
Protocol: [context-protocol.md](context-protocol.md)

## Completion rules

- `[ ]` means incomplete.
- `[x]` means verified and linked to evidence or an accepted ADR.
- A blocked item remains unchecked and ends with `BLOCKED: <reason>`.
- A conditional item may be marked `N/A` only with a finding explaining why it is outside the supported surface.
- Do not mark a task complete based only on an undocumented manual observation.

## A. Session baseline and safety

- [ ] **P0-001** Read `AGENTS.md`, the master plan, this execution package, current status, and handoff.  
  Evidence: first Phase 0 entry in `session-log.md` lists the documents and selected task IDs.

- [ ] **P0-002** Record the initial Git branch, commit, dirty-worktree files, date, and operator/session scope.  
  Evidence: sanitized baseline entry in `session-log.md`.

- [ ] **P0-003** Define the approved Cloud and self-hosted test targets without recording private tenant identifiers.  
  Evidence: target rows in `compatibility-matrix.md`.

- [ ] **P0-004** Define a unique test-resource prefix and reject an empty or unsafe prefix.  
  Evidence: probe configuration test and finding.

- [ ] **P0-005** Define environment-variable-only credential inputs and add a value-free example/configuration description.  
  Evidence: tracked configuration contains variable names but no values.

- [ ] **P0-006** Implement base-URL safety checks or an explicit allowlist for mutating probes.  
  Evidence: unit test showing an unapproved target is rejected.

- [ ] **P0-007** Create a cleanup inventory that records every test-created remote identity.  
  Evidence: evidence README or probe output format plus a dry-run example.

- [ ] **P0-008** Establish a redaction policy for tokens, secret values, response headers, emails, and tenant identifiers.  
  Evidence: redaction test and [evidence/README.md](evidence/README.md).

## B. OpenAPI baseline and reusable probe

- [ ] **P0-010** Download the OpenAPI document reproducibly and store the pinned snapshot at the ADR-003-approved path.  
  Evidence: committed snapshot and source URL.

- [ ] **P0-011** Calculate and record the snapshot SHA-256.  
  Evidence: finding and reproducible command.

- [ ] **P0-012** Reconfirm path, operation, schema, property, and security-scheme counts.  
  Evidence: sanitized OpenAPI inventory report.

- [ ] **P0-013** Create the local overlay with stable, unique operation IDs for core Phase 0 operations.  
  Evidence: overlay plus validation showing no duplicate operation IDs.

- [ ] **P0-014** Pin the overlay/generation tool and prove the command is deterministic.  
  Evidence: two runs produce no diff.

- [ ] **P0-015** Implement a minimal Go API probe with timeout, cancellation, token auth, envelope parsing, and redaction.  
  Evidence: unit tests and a sanitized read-only run.

- [ ] **P0-016** Add paginated exact-match helpers for ID, key, and normalized email.  
  Evidence: unit tests including multiple pages, no match, one match, and duplicate match.

- [ ] **P0-017** Add created-resource tracking and cleanup execution to the probe.  
  Evidence: lifecycle test against a disposable object.

- [ ] **P0-018** Ensure probe logs never emit `Authorization` or environment secret values.  
  Evidence: automated log assertion and repository secret scan.

## C. Authentication and context

- [ ] **P0-020** Verify a service access token sent directly in `Authorization`.  
  Evidence: sanitized successful request observation.

- [ ] **P0-021** Verify a personal access token sent directly in `Authorization`, if available.  
  Evidence: sanitized observation, or `N/A` with prerequisite explanation.

- [ ] **P0-022** Observe missing, malformed, inactive, and insufficient-scope token behavior where safely testable.  
  Evidence: status/envelope table without token values.

- [ ] **P0-023** Confirm that the supported public workflow requires no username/password login, JWT refresh, MFA, or SSO handling.  
  Evidence: official documentation link plus probe findings.

- [ ] **P0-024** Verify whether any organization/workspace context headers are required.  
  Evidence: successful requests with only documented required headers.

- [ ] **P0-025** Record the Phase 1 provider authentication schema and multi-context alias approach.  
  Evidence: accepted auth finding referenced by `handoff.md`.

## D. Error, absence, and retry behavior

- [ ] **P0-030** Record HTTP status and FeatBit envelope behavior for validation errors.  
  Evidence: sanitized invalid-request observations.

- [ ] **P0-031** Record duplicate identity behavior for each applicable core family.  
  Evidence: project/environment/flag/segment duplicate matrix.

- [ ] **P0-032** Record post-delete read behavior for each core family.  
  Evidence: status/envelope observations.

- [ ] **P0-033** Prove exact project absence through a fully paginated exact-ID fallback.  
  Evidence: present and absent cases.

- [ ] **P0-034** Prove exact environment absence through its parent project's environment collection.  
  Evidence: present and absent cases.

- [ ] **P0-035** Prove exact feature-flag absence through a fully paginated exact-key fallback.  
  Evidence: present and absent cases.

- [ ] **P0-036** Prove exact segment absence through an exact ID or fully paginated exact-ID fallback.  
  Evidence: present and absent cases.

- [ ] **P0-037** Observe `401` and `403` and verify they never remove state.  
  Evidence: error-classifier tests.

- [ ] **P0-038** Probe or mock `429`, transient `5xx`, timeout, cancellation, and network interruption handling.  
  Evidence: retry classification tests; live tests are optional when unsafe.

- [ ] **P0-039** Define the centralized error-classification decision table for Phase 1.  
  Evidence: finding linked by `handoff.md`.

## E. Project and environment lifecycle

- [ ] **P0-040** Probe project create/read/update/delete and canonical second Read.  
  Evidence: sanitized lifecycle record with cleanup.

- [ ] **P0-041** Record project key mutability and duplicate-key behavior.  
  Evidence: finding supporting `RequiresReplace` or safe update.

- [ ] **P0-042** Record automatically created environment count, names, keys, settings, and ordering.  
  Evidence: normalized response with identifiers and secrets redacted.

- [ ] **P0-043** Probe environment create/read/update/delete and canonical second Read.  
  Evidence: sanitized lifecycle record with cleanup.

- [ ] **P0-044** Record environment key and parent mutability.  
  Evidence: finding supporting `RequiresReplace`.

- [ ] **P0-045** Observe project/environment delete cascade, blocking, or asynchronous behavior.  
  Evidence: explicit deletion sequence and final cleanup status.

- [ ] **P0-046** Inventory environment secret metadata and whether values are returned consistently, without recording values.  
  Evidence: field-presence/type matrix.

- [ ] **P0-047** Decide metadata-only versus opt-in Sensitive secrets data source behavior.  
  Evidence: accepted finding or ADR consequence addressing Terraform state security.

## F. Feature flag lifecycle and normalization

- [ ] **P0-050** Probe boolean flag create/read/update/delete.  
  Evidence: sanitized lifecycle record.

- [ ] **P0-051** Repeat the round trip for string, number, and JSON variation types.  
  Evidence: canonicalization table for all four types.

- [ ] **P0-052** Map `enabledVariationId`, `fallthrough`, disabled variation, and server-generated variation IDs.  
  Evidence: bidirectional mapping finding.

- [ ] **P0-053** Probe variations, metadata, enabled state, targets, rules, rollouts, and tags through specialized endpoints.  
  Evidence: operation sequence and canonical final Read.

- [ ] **P0-054** Record ordering semantics for variations, rules, conditions, tags, and targets.  
  Evidence: List/Set decision table.

- [ ] **P0-055** Record JSON and condition-value normalization.  
  Evidence: repeated Read produces byte-equivalent canonical state.

- [ ] **P0-056** Observe revision use and a stale-revision conflict where safely reproducible.  
  Evidence: status/envelope and retry/no-retry decision.

- [ ] **P0-057** Verify that a narrow update does not rewrite an unrelated targeting field.  
  Evidence: before/after comparison.

- [ ] **P0-058** Inject a controlled failure after base creation and verify state/import recovery.  
  Evidence: orphan handling, recovered identity, cleanup, and exact Import command.

- [ ] **P0-059** Demonstrate an empty logical diff after creating and reading one complex flag.  
  Evidence: normalized desired-versus-observed comparison.

## G. Segment lifecycle and normalization

- [ ] **P0-060** Probe environment-specific segment create/read/update/delete.  
  Evidence: sanitized lifecycle record.

- [ ] **P0-061** Probe shared segment creation and scopes.  
  Evidence: sanitized lifecycle record or documented unsupported result.

- [ ] **P0-062** Probe rules, included/excluded users, tags, and archive/restore.  
  Evidence: operation sequence and canonical final Read.

- [ ] **P0-063** Observe safe update behavior for key, type, and scopes.  
  Evidence: matrix; v1 remains `RequiresReplace` unless every supported target is deterministic.

- [ ] **P0-064** Observe deletion when a feature flag references the segment.  
  Evidence: conflict/error behavior and cleanup steps.

- [ ] **P0-065** Demonstrate an empty logical diff after creating and reading one complex segment.  
  Evidence: normalized desired-versus-observed comparison.

## H. Member and IAM feasibility

- [ ] **P0-070** Verify paginated member lookup and normalized exact-email filtering.  
  Evidence: zero, one, and duplicate exact-match tests.

- [ ] **P0-071** Preflight absence, call `POST /members/add`, and poll for one exact member ID in a disposable scope.  
  Evidence: timing and result without recording real email or password values.

- [ ] **P0-072** Determine whether invitations or human acceptance prevent synchronous reconciliation.  
  Evidence: verified workflow finding.

- [ ] **P0-073** Classify managed member creation as supported or external.  
  Evidence: support-level decision; do not leave this ambiguous.

- [ ] **P0-074** Verify that group/policy/member relationships have stable IDs suitable for independent binding resources.  
  Evidence: endpoint and composite-identity finding.

## I. ADRs and capability classification

- [ ] **P0-080** Write and accept ADR-001: Terraform versus UI ownership.  
  Evidence: accepted ADR linked to flag findings.

- [ ] **P0-081** Write and accept ADR-002: Delete versus archive.  
  Evidence: accepted ADR linked to lifecycle findings.

- [ ] **P0-082** Write and accept ADR-003: Generated OpenAPI client and local overlay.  
  Evidence: accepted ADR plus deterministic generation proof.

- [ ] **P0-083** Write and accept ADR-004: Public Import ID formats.  
  Evidence: accepted ADR covering every planned resource/binding.

- [ ] **P0-084** Write and accept ADR-005: Supported Cloud/self-hosted/Go/Terraform compatibility matrix.  
  Evidence: accepted ADR and completed matrix.

- [ ] **P0-085** Classify every candidate capability into one of the five support levels.  
  Evidence: capability table in `findings.md`.

- [ ] **P0-086** Map every Phase 1 client responsibility to a finding, ADR, or explicit test requirement.  
  Evidence: implementation input section in `handoff.md`.

## J. Closure and Phase 1 readiness

- [ ] **P0-090** Rerun required probes on every available supported target.  
  Evidence: completed compatibility matrix with dates.

- [ ] **P0-091** Delete all test-created remote objects or list an owner and exact manual cleanup action.  
  Evidence: empty or explicitly owned cleanup inventory.

- [ ] **P0-092** Run repository credential/secret checks over tracked and untracked Phase 0 artifacts.  
  Evidence: sanitized command and successful result.

- [ ] **P0-093** Review every completed TODO for linked evidence.  
  Evidence: no checked task lacks evidence.

- [ ] **P0-094** Update `status.md` with final decisions, supported scope, risks, and exit-gate result.  
  Evidence: status is current and internally linked.

- [ ] **P0-095** Complete `handoff.md` with exact Phase 1 inputs and no unresolved implementation assumption.  
  Evidence: handoff links all five accepted ADRs and compatibility findings.

- [ ] **P0-096** Append the final Phase 0 session-log entry and declare `Ready for Phase 1`.  
  Evidence: final chronological record.
