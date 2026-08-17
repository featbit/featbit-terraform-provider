# Phase 6 TODO — IAM v1 and release

Work one item at a time. Search the existing implementation before adding a
helper, wire model, client method, schema, lifecycle branch, or fixture. Record
only the concise result under the active item. The current item is **P6-030**.

## Scope and API contract

- [x] **P6-010 — Freeze IAM v1 from customer feedback.**

  IAM v1 manages custom Policies with statements, Groups, Group-Policy and
  Group-Member bindings, and an existing Member's direct Policy set. It adds
  exact Policy, Member, Project-key, and Environment-key lookup. Policy
  statements cover Project, Environment, Feature Flag, and Segment control
  levels. Member CRUD, Service Tokens, built-in Policy mutation, and Phase 7
  Segment targeting prerequisites are excluded.

- [x] **P6-020 — Prove the documented public IAM API.**

  Verify organization scope, access-token authentication, pagination, exact
  lookup, mutation results, idempotence, and error behavior for every Group,
  Policy, Member, and binding operation in scope. Confirm built-in Policies are
  read-only and Member responses can be consumed without retaining
  `initialPassword`. Prove `project`, `env`, `flag`, and `segment` statement
  types, their actions, Allow/Deny effects, and documented wildcard,
  exact-key, and tag selector forms.

  Done when the endpoint matrix supports the full IAM v1 workflow or records a
  blocking public-API gap. Do not add production code before this passes.

  Result: **passed on current Cloud on 2026-08-17; no blocking public-API gap.**
  The tested OpenAPI (`/swagger/OpenApi/swagger.json`, SHA-256
  `8de202f939f6721748d66449c3dfe4eee2e2bf369a57f121df808907a44d11c4`)
  supplies the complete IAM v1 matrix:

  | Area | Documented operations |
  |---|---|
  | Group | paginated list, exact ID read, create/update/delete, paginated member and Policy collections, and add/remove pair mutations |
  | Policy | paginated list, exact ID read, create, settings update, complete statement replacement, delete, and paginated Group/Member collections |
  | Member | paginated list, organization-scoped exact ID read, paginated direct/inherited Policy and Group collections, and direct-Policy add/remove mutations |

  A unique custom Policy and Group passed create, exact list/read, update,
  delete, and repeated delete. Complete statement replacement, identical
  replacement, and clearing all returned 200 and round-tripped lower-case
  `allow`/`deny`; `project`, `env`, `flag`, and `segment`; every published
  action; hierarchical exact selectors
  `project/<key>[:env/<key>[:flag|segment/<key>]]`; `*` wildcards; and
  `;<tag>[,<tag>...]` Flag/Segment suffixes. The action catalog is:

  | Type | Actions |
  |---|---|
  | `project` | `CanAccessProject`, `CreateProject`, `DeleteProject`, `UpdateProjectSettings`, `CreateEnv` |
  | `env` | `CanAccessEnv`, `DeleteEnv`, `UpdateEnvSettings`, `DeleteEnvSecret`, `CreateEnvSecret`, `UpdateEnvSecret` |
  | `flag` | `*`, `CreateFlag`, `ArchiveFlag`, `RestoreFlag`, `DeleteFlag`, `CloneFlag`, `CopyFlagTo`, `ToggleFlag`, `UpdateFlagName`, `UpdateFlagDescription`, `UpdateFlagOffVariation`, `UpdateFlagVariations`, `UpdateFlagTags`, `UpdateFlagDefaultRule`, `UpdateFlagIndividualTargeting`, `UpdateFlagTargetingRules` |
  | `segment` | `*`, `CreateSegment`, `ArchiveSegment`, `RestoreSegment`, `DeleteSegment`, `UpdateSegmentName`, `UpdateSegmentDescription`, `UpdateSegmentTags`, `UpdateSegmentTargetingUsers`, `UpdateSegmentRules` |

  Group-Member, Group-Policy, and direct Member-Policy add/add/remove/remove
  sequences each returned 200 and were state-idempotent. They were isolated by
  using a Policy with no statements and a Group with no Policies or Members.
  The pre-authorized existing Member's direct, inherited, and Group sets matched
  their exact baselines after cleanup. The unique Policy and Group were absent
  in an independent final inventory.

  The access token works directly in `Authorization`; an invalid token returns
  401. Complete pagination returned 12 Policies, 11 Members, and 4 Groups, with
  exact ID/key/email matches and 404 for missing object reads. Omitted context
  headers select the token organization/workspace, while caller-supplied context
  headers can select a different collection; retain the transport's complete
  organization/workspace-header stripping.

  Mandatory Provider boundaries from the proof are: decode an explicit safe
  Member allowlist because `MemberVm` contains `initialPassword`; validate all
  statement types, effects, actions, and selectors because Cloud accepted an
  invalid catalog with 200; and resolve every ID through the token-scoped
  complete collection before mutation, then read the exact relationship back,
  because missing-ID binding calls also return successful 200 no-ops. Missing
  Group update and Policy settings/statements return 404. All three built-in
  `SysManaged` Policies remain read-only to Terraform: current Cloud rejected a
  same-value settings update with 422, while source inspection found no
  equivalent built-in guard on delete, so the Provider must never invoke any
  built-in mutation.

- [ ] **P6-030 — Freeze Terraform schemas and lifecycle contracts.**

  Fix resource and data-source names, attributes, canonical ordering, exact
  selectors, replacement rules, Import IDs, drift behavior, and destroy
  behavior. Freeze the statement schema for `resource_type`, `effect`,
  `actions`, and `resources`, including action/type validation and selector
  canonicalization. Define the authoritative per-Member direct-Policy resource
  so `policy_ids = []` means no direct Policies and never affects inherited
  ones.

  Done when each schema maps to proven API calls and has an unambiguous owner.

## Provider implementation

- [ ] **P6-040 — Add exact-key Project and Environment lookup.**

  Extend the existing data sources to select a Project by exact organization
  key and an Environment by exact key within its Project, while preserving UUID
  configurations. Reuse the complete Project read and reject duplicate matches.
  Confirm the existing Feature Flag and Segment outputs provide the exact keys
  needed by specific-resource selectors.

  Runtime: `project_data_source.go` and `environment_data_source.go` → existing
  Project/Environment client adapters. Done when UUID and key selectors both
  pass focused and Protocol tests.

- [ ] **P6-050 — Implement Policy management and lookup.**

  Add a custom Policy resource that owns settings and all statements as one
  Terraform object, even though Create uses separate Policy and statement API
  calls. Support Project, Environment, Feature Flag, and Segment statements,
  and add exact-key Policy lookup for custom and built-in Policies.

  Runtime: Policy resource/data source → `internal/client/policies.go` → public
  Policy endpoints. Done when CRUD, Import, drift, canonical statements,
  partial failure, built-in read-only behavior, and all four control levels
  pass.

- [ ] **P6-060 — Implement Group management.**

  Add Group CRUD and Import for name and description only. Do not place member
  IDs or Policy IDs in the Group resource.

  Runtime: Group resource → `internal/client/groups.go` → public Group
  endpoints. Done when lifecycle, exact absence, drift, and ambiguous mutation
  handling pass.

- [ ] **P6-070 — Implement Group-Policy bindings.**

  Add a resource for one exact Group/Policy pair. Read through the complete
  Group Policy collection; Create and Destroy add or remove only that pair.

  Runtime: binding resource → Group client adapter → public Group Policy
  endpoints. Done when Import, repeated apply, pre-existing binding, drift, and
  out-of-band removal pass.

- [ ] **P6-080 — Implement Member lookup and Group-Member bindings.**

  Add exact existing-Member lookup by ID or email and a resource for one exact
  Group/Member pair. Decode only safe Member fields; never create, invite,
  update, or delete the Member.

  Runtime: Member data source and binding resource → Member/Group client
  adapters → public Member and Group-member endpoints. Done when exact lookup,
  Import, repeated apply, drift, and identity redaction pass.

- [ ] **P6-090 — Implement authoritative direct Member Policies.**

  Add one resource that reconciles only the complete direct Policy set of one
  existing Member. An empty set removes direct Policies; Group-inherited
  Policies are never changed.

  Runtime: direct-Policy resource → Member client adapter → direct-Policy list,
  add, and remove endpoints. Done when empty-set enforcement, Import, drift,
  partial failure, reread, and destroy follow P6-030.

## Verification and release

- [ ] **P6-100 — Prove the complete IAM workflow locally.**

  Add Protocol and integration coverage for the customer-shaped resource
  graph. Look up Project/Environment and the built-in Owner Policy by exact
  key. Create a base-access custom Policy for Project/Environment visibility
  and a scoped custom Policy for dev Feature Flag/Segment operations plus one
  exact prod Feature Flag exception. Attach Owner to an admin Group, attach
  both custom Policies to a developer Group, assign an existing Member only to
  the developer Group, and reconcile that Member's direct Policy set to empty.

  Across the workflow and focused variants, cover all four statement levels,
  wildcard and exact-resource selectors, Allow/Deny, and action subsets. Prove
  Import, drift, redaction, cleanup, and an empty second plan. Removing one
  binding must remove only its exact pair and preserve the other Policy and
  Member bindings.

- [ ] **P6-110 — Pass trusted current-Cloud acceptance.**

  Run the same customer-shaped graph with a unique test-owned Project, its
  default dev/prod Environments, three Feature Flags, a Segment, two custom
  Policies, admin/developer Groups, and exact bindings. Resolve and attach the
  built-in Owner Policy without mutating it. Confirm all four statement levels
  round-trip and the second Terraform plan is empty.

  Use a pre-authorized existing Member fixture with no other effective Group
  Policies and a matching credential supplied outside Terraform to prove
  effective access: Project/dev/prod reads succeed; reversible metadata
  operations on the dev Feature Flag and Segment succeed; the permitted
  operation on the selected prod Feature Flag succeeds; and the same operation
  on another prod Feature Flag returns 403. The Member must not belong to the
  admin Group, and its direct Policy set must be empty during these checks so
  access comes only from the developer Group. Do not create, delete, or change
  the Member profile or organization membership, and do not create or manage a
  Service Token. Never retain the Member credential. Restore the Member's
  original direct Policies and remove every test-owned object and edge,
  including after a failed assertion.

- [ ] **P6-120 — Publish the supported IAM surface in documentation.**

  Add resource/data-source pages and runnable examples, regenerate Registry
  docs, and update README and schema assertions. Show the required parent
  Project/Environment access alongside Feature Flag and Segment permissions.
  State clearly that Member CRUD, Service Token management, and Phase 7 Segment
  prerequisites are unsupported.

- [ ] **P6-130 — Qualify the IAM release.**

  Run formatting, unit/race tests, vet, build, module checks, Protocol/schema
  checks, generated-doc checks, redaction scans, and the GoReleaser snapshot.
  Confirm the release contains exactly the approved IAM surface.

- [ ] **P6-140 — Publish and close the IAM release.**

  Begin only with explicit maintainer authorization. Create and inspect the
  release through the existing protected workflow, publish it, merge only
  still-current facts into the master plan, and remove the completed Phase 6
  package. Do not start Phase 7 implementation on this branch.
