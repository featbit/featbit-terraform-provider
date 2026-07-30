# Phase 0 Work Plan

Phase: **Empirical API compatibility and ADRs**  
Status: **Ready to start**  
Timebox: **3–5 person-days for one engineer**  
Backend assumption: **The current public REST API cannot be changed**

## 1. Objective

Produce a verified, versioned compatibility contract for implementing the FeatBit Terraform Provider against the current API. Phase 0 must replace undocumented assumptions with evidence and choose a safe Terraform support level for every candidate capability.

The primary output is not production resource code. It is the evidence, normalization rules, failure semantics, ownership boundaries, and ADR set that make Phase 1 implementation deterministic.

## 2. Required outcomes

By the end of Phase 0:

- The OpenAPI input and its hash are pinned.
- Personal and service access-token behavior is verified without implementing login.
- Core CRUD, absence, duplicate, authorization, pagination, and multi-step behavior is observed.
- Exact-identity fallbacks are proven for projects, environments, flags, and segments.
- Feature flag and segment round trips have canonical normalization rules.
- Member creation is classified as managed or external based on the exact-email prototype.
- Environment secret exposure has a state-security decision.
- Every candidate capability has a support level.
- Five ADRs are accepted.
- Phase 1 has an unambiguous implementation handoff.

## 3. Non-goals

- Modifying the FeatBit backend or OpenAPI published by the service.
- Implementing complete Terraform resources or publishing a provider.
- Matching LaunchDarkly resource-for-resource.
- Using private Portal endpoints, browser automation, or database access.
- Testing against production customer resources.
- Persisting real credentials or secret values in repository files.

## 4. Inputs and prerequisites

### Repository inputs

- [Master implementation plan](../plan.md)
- [Root workspace instructions](../../AGENTS.md)
- FeatBit OpenAPI: <https://app-api.featbit.co/swagger/OpenApi/swagger.json>
- FeatBit REST API guide: <https://docs.featbit.co/api-docs/using-featbit-rest-api>
- FeatBit access-token guide: <https://docs.featbit.co/integrations/api-access-tokens>

### Runtime prerequisites

- A dedicated FeatBit Cloud test tenant or workspace.
- A service access token with the minimum permissions needed for tested operations.
- A personal token for authentication-comparison tests, if available.
- A pinned self-hosted FeatBit instance representing the minimum supported release.
- Permission to create and delete test projects, environments, flags, segments, and optional IAM objects.

Use environment variables only:

```text
FEATBIT_TEST_API_URL
FEATBIT_TEST_SERVICE_TOKEN
FEATBIT_TEST_PERSONAL_TOKEN
FEATBIT_TEST_TARGET
FEATBIT_TEST_RESOURCE_PREFIX
```

Do not add token values to `.env.example`, commands, evidence, logs, or Terraform configuration.

## 5. Deliverables

| Deliverable | Target location |
|---|---|
| Pinned OpenAPI snapshot and overlay | Repository location selected by ADR-003 |
| Reusable sanitized API probe | `tools/api-probe/` or another ADR-003-approved path |
| Observed findings | [findings.md](findings.md) |
| Deployment/version matrix | [compatibility-matrix.md](compatibility-matrix.md) |
| Capability support matrix | Section in `findings.md`, linked by `status.md` |
| Sanitized observations | [evidence/](evidence/) |
| ADR-001 through ADR-005 | [adrs/](adrs/) |
| Completed checklist | [todo.md](todo.md) |
| Phase completion summary | [status.md](status.md) |
| Phase 1 handoff | [handoff.md](handoff.md) |

## 6. Execution strategy

### Workstream A — Baseline, safety, and reproducibility

1. Capture the repository state and active plan version.
2. Establish test-resource naming, target isolation, permissions, and cleanup inventory.
3. Download the OpenAPI document reproducibly and verify its SHA-256.
4. Create a minimal Go probe capable of:
   - Sending access-token-authenticated requests
   - Recording HTTP status and the FeatBit response envelope
   - Redacting headers and sensitive response fields
   - Paginating list operations
   - Tracking created test identities for cleanup
5. Ensure a probe run cannot target an unapproved production base URL accidentally.

### Workstream B — Authentication and account context

Verify:

- Service and personal access tokens are passed directly in `Authorization`.
- Missing, malformed, inactive, and insufficient-scope tokens are distinguishable.
- No username/password login, JWT refresh, organization header, or workspace header is required for the public API workflow.
- A token selects a stable account context.
- Multiple account contexts can be represented with provider aliases and different tokens.

The output feeds the Phase 1 provider schema and auth transport.

### Workstream C — Errors, identity, and absence

For each core resource family:

1. Observe create, exact read, update, delete, and read-after-delete.
2. Observe duplicate key/name behavior where uniqueness applies.
3. Observe a forbidden operation with an intentionally restricted token if safe.
4. Record HTTP status and `{success,data,errors}` together.
5. Prove the exact parent-scoped fallback:
   - Project: paginated project list filtered by exact ID
   - Environment: parent project response filtered by exact environment ID
   - Feature flag: paginated environment flag list filtered by exact key
   - Segment: exact ID read/list-by-ID or paginated list filtered by exact ID
6. Confirm that ambiguity preserves state and produces a diagnostic rather than removing state.

Do not base provider behavior on matching English error-message fragments.

### Workstream D — Lifecycle and round-trip probes

#### Projects and environments

- Record server defaults and auto-created environments.
- Determine key immutability and update normalization.
- Observe delete cascade, blocking, and asynchronous behavior.
- Record environment settings and returned secret metadata.

#### Feature flags

- Create all supported variation types: boolean, string, number, and JSON.
- Map create-time `enabledVariationId` to read-time `fallthrough`.
- Preserve server-generated variation IDs while exposing Terraform-friendly indices.
- Apply targeting, tags, toggle state, variations, and metadata through their specialized endpoints.
- Record revision behavior and stale-revision outcomes where possible.
- Verify read-after-write convergence and canonical ordering/JSON.
- Inject a controlled partial failure after base creation and prove recovery or an exact Import instruction.
- Verify that updating one field does not rewrite unrelated targeting.

#### Segments

- Probe environment-specific and shared segment creation.
- Verify rules, included/excluded users, tags, archive/restore, and references.
- Attempt safe update probes for `key`, `type`, and `scopes`; regardless of undocumented success, retain `RequiresReplace` unless behavior is deterministic across all supported targets.
- Record delete behavior when a flag references the segment.

#### Members and IAM

- List all pages and filter members using normalized exact email.
- Confirm whether `POST /members/add` becomes visible synchronously and yields exactly one resolvable member ID.
- Observe invitation or human-acceptance behavior.
- If deterministic, document the reconciliation algorithm and Import identity.
- If not deterministic, classify member creation as an external prerequisite while retaining lookup and binding support.

### Workstream E — Architecture decisions

Create and accept:

1. `ADR-001-terraform-ui-ownership.md`
2. `ADR-002-delete-vs-archive.md`
3. `ADR-003-openapi-client-and-overlay.md`
4. `ADR-004-import-identities.md`
5. `ADR-005-supported-compatibility-matrix.md`

Each ADR must cite Phase 0 evidence, identify rejected alternatives, and state implications for public Terraform behavior.

### Workstream F — Classification and Phase 1 handoff

Classify every candidate surface:

- Fully managed
- Constrained managed
- Read/bind only
- External prerequisite
- Omitted

Update the compatibility matrix, close or explicitly defer every TODO, complete cleanup, and write a handoff that maps findings and ADRs to Phase 1 packages and tests.

## 7. Suggested 3–5 day sequence

| Day | Focus | Expected checkpoint |
|---|---|---|
| 1 | Safety, OpenAPI snapshot, Go probe, authentication | Reproducible probe and verified token transport |
| 2 | Error/absence contract, projects, environments | Exact-identity fallbacks and normalized core observations |
| 3 | Complex flags and segments | Round-trip mappings and constrained update decisions |
| 4 | Members, secrets, failure injection, ADR drafts | All uncertain capabilities classified |
| 5 | Cross-target reruns, ADR acceptance, cleanup, handoff | Exit gate passed or precise reduced-scope decisions |

The sequence is a guide, not permission to skip evidence. If only one deployment target is available, mark the other matrix row unverified and do not claim full compatibility.

## 8. Risk controls

| Risk | Control |
|---|---|
| Accidental production mutation | Approved base-URL allowlist, dedicated tenant, randomized prefix, cleanup inventory |
| Credential leakage | Environment variables, header/body redaction, no request dumps, repository secret scan |
| False Not Found | Exact scoped lookup fallback; ambiguous errors preserve state |
| Partial create orphan | Track identity immediately, read after failure, rollback only when safe, provide exact Import recovery |
| UI/Terraform overwrite | Narrow ownership, specialized update endpoints, read-before/write-after verification |
| Incorrect cross-version claim | Per-target evidence and explicit `Not tested` matrix cells |
| Scope expansion toward parity | Capability tier must cite a FeatBit customer workflow |

## 9. Exit gate

Phase 0 passes when all of the following are true:

- Required TODOs are completed with linked evidence.
- OpenAPI generation inputs are pinned and reproducible.
- API access-token behavior is verified.
- Core resource absence can be classified without fuzzy message matching.
- Project, environment, complex flag, and segment create/read/update/delete probes converge deterministically or have an explicit constrained scope.
- Member and environment-secret decisions are recorded.
- ADR-001 through ADR-005 are `Accepted`.
- Compatibility and capability matrices are complete for every available target.
- Cleanup inventory is empty, or remaining objects and owners are explicitly listed.
- No credential or secret value exists in tracked files.
- `status.md` and `handoff.md` declare readiness for Phase 1.

If a behavior remains ambiguous, Phase 0 can still pass only after the affected capability is made replace-only, read-only, external, or omitted. Ambiguity must not be passed downstream as an undocumented Phase 1 assumption.
