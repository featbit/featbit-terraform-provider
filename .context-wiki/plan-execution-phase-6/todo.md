# Phase 6 TODO — IAM v1 and release

Work one item at a time. Search the existing implementation before adding a
helper, wire model, client method, schema, lifecycle branch, or fixture. Record
only the concise result under the active item. The current item is **P6-140**.

## Scope and API contract

- [x] **P6-010 — Freeze IAM v1 from customer feedback.**

  IAM v1 manages custom Policies with statements, Groups, Group-Policy and
  Group-Member bindings, and an existing Member's direct Policy set. It adds
  exact Policy, existing Group, Member, Project-key, and Environment-key
  lookup. Policy statements cover Project, Environment, Feature Flag, and
  Segment control levels. Member CRUD, Service Tokens, built-in Policy
  mutation, and Phase 7 Segment targeting prerequisites are excluded.

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

- [x] **P6-030 — Freeze Terraform schemas and lifecycle contracts.**

  Fix resource and data-source names, attributes, canonical ordering, exact
  selectors, replacement rules, Import IDs, drift behavior, and destroy
  behavior. Freeze the statement schema for `resource_type`, `effect`,
  `actions`, and `resources`, including action/type validation and selector
  canonicalization. Define the authoritative per-Member direct-Policy resource
  so `policy_ids = []` means no direct Policies and never affects inherited
  ones.

  Done when each schema maps to proven API calls and has an unambiguous owner.

  Result: the IAM v1 Terraform contract is frozen as follows. `R`, `O`, `C`,
  and `S` mean Required, Optional, Computed, and Sensitive. Every omitted
  description canonicalizes to `""`; timestamps, API statement IDs, Member
  invitation fields, `initialPassword`, and relationship display fields are
  intentionally absent from state.

  | Terraform type | Frozen attributes | Replacement and Import |
  |---|---|---|
  | resource `featbit_policy` | `id` C UUID; `name` R; `key` R; `description` O+C; `type` C and always `CustomerManaged`; `statements` R set | only `key` replaces; `<policy_uuid>` |
  | resource `featbit_group` | `id` C UUID; `name` R; `description` O+C | all settings update in place; `<group_uuid>` |
  | resource `featbit_group_policy_binding` | `id` C synthetic pair; `group_id` R UUID; `policy_id` R UUID | either input replaces; `<group_uuid>/<policy_uuid>` |
  | resource `featbit_group_member_binding` | `id` C+S synthetic pair; `group_id` R UUID; `member_id` R+S UUID | either input replaces; `<group_uuid>/<member_uuid>` |
  | resource `featbit_member_direct_policies` | `id` C+S and equal to the canonical Member UUID; `member_id` R+S UUID; `policy_ids` R set of UUIDs, including `[]` | `member_id` replaces; `<member_uuid>` |
  | data source `featbit_policy` | exactly one of `id` O+C UUID or exact case-sensitive `key` O+C; `name`, `description`, `type`, and statement set C | no ownership or Import |
  | data source `featbit_group` | exactly one of `id` O+C UUID or organization-scoped exact case-sensitive `name` O+C; `description` C | no ownership or Import |
  | data source `featbit_member` | exactly one of `id` O+C+S UUID or `email` O+C+S; `name` C+S | no ownership or Import |
  | existing data source `featbit_project` | `id` and `key` become O+C selectors with exactly one configured; existing computed outputs stay unchanged | exact UUID or organization-scoped case-sensitive exact key |
  | existing data source `featbit_environment` | `project_id` remains R; `id` and `key` become O+C selectors with exactly one configured; existing computed outputs stay unchanged | exact UUID or case-sensitive exact key inside the exact Project |

  Group name and Member email selection compare the complete token-scoped
  collection by full-string equality, never substring search, and reject
  duplicate exact matches. Group names are case-sensitive; Member emails are
  case-insensitive. State retains the server's canonical spelling.
  Member adapters decode only `id`, `email`, and `name`; relationship adapters
  decode only the identifiers, types, and membership booleans needed by their
  caller. Member values are never interpolated into diagnostics.

  `featbit_policy.statements` is an unordered required set and may be empty.
  Each element has exactly these required fields:

  | Field | Frozen form |
  |---|---|
  | `resource_type` | exact lower-case `project`, `env`, `flag`, or `segment` |
  | `effect` | exact lower-case `allow` or `deny` |
  | `actions` | non-empty set of exact case-sensitive actions from the P6-020 catalog for that `resource_type`; `*` is valid only for `flag` and `segment` |
  | `resources` | non-empty set of selectors whose full shape matches `resource_type` |

  Selector shapes are `project/<project>`,
  `project/<project>:env/<env>`,
  `project/<project>:env/<env>:flag/<flag>[;<tag>[,<tag>...]]`, and
  `project/<project>:env/<env>:segment/<segment>[;<tag>[,<tag>...]]`.
  Each key position is either one exact case-sensitive key or the whole-token
  wildcard `*`. IAM v1 rejects a global `*`, partial globs, wrong or missing
  segments, tags outside the Flag/Segment leaf, empty tokens, whitespace, and
  reserved RN delimiters inside keys or tags. Tag suffixes use OR semantics.

  Canonicalization preserves exact key and tag case, sorts and deduplicates
  tags, actions, resources, and UUID sets bytewise, and treats statement order
  as insignificant. API payloads sort statements by `resource_type`, `effect`,
  canonical actions, then canonical resources; reads ignore API array order
  and statement IDs. A custom Policy whose remote statements cannot satisfy
  this catalog produces a state-preserving diagnostic. The computed Policy
  data source may still observe system-managed statement shapes such as
  `resource_type = "*"`; that read-only allowance never widens the managed
  resource schema.

  Ownership and API call relationships are frozen as follows:

  - `featbit_policy` owns one custom Policy's name, immutable key,
    description, and complete statement set. Create scans the complete Policy
    collection for exact-key zero, calls create once, calls complete statement
    replacement once even for an empty set, then reads the exact canonical
    Policy. Update calls settings and/or complete-statement replacement once in
    that order, persisting each confirmed intermediate state. Read first proves
    token-scoped exact-ID membership and `CustomerManaged` type. Destroy
    rechecks that type, refuses while any exact Group or direct-Member
    association remains, calls delete once, and requires complete-list exact
    absence. Import of a `SysManaged` Policy fails before any mutation; no
    built-in mutation path exists.
  - The `featbit_group` resource owns only Group existence, name, and
    description. It never contains member or Policy IDs. Create uses
    complete-list exact-name zero, then one create and an exact read; Update is
    one settings call. Because the public delete cascades relationships,
    Destroy first requires complete Group Member and Policy collections to be
    empty, then deletes once and proves complete-list exact absence. The
    `featbit_group` data source selects an existing Group by exact ID or exact
    name and adopts neither its lifecycle nor its relationships.
  - Each binding resource owns only its configured exact pair. Create resolves
    both IDs through complete token-scoped collections, reads the complete
    Group relationship collection, deliberately takes ownership without a
    mutation when that exact pair already exists, or sends one add and verifies
    the pair. Read removes state only on authoritative exact-pair absence.
    Destroy sends at most one remove and requires authoritative absence; a
    confirmed missing endpoint object already proves the pair absent. No other
    pair is read into state, added, or removed. Built-in Policies are valid
    Group-Policy binding targets.
  - `featbit_member_direct_policies` is the sole owner of one existing Member's
    entire direct Policy set. Create, Update, and drift reconciliation resolve
    the Member and every desired Policy through complete token-scoped
    collections, read only `direct-policies`, add missing IDs in canonical order
    and verify them before removing extra IDs in canonical order, then persist
    the exact reread set. Each pair mutation executes once and every confirmed
    intermediate set is recoverable after partial failure. `policy_ids = []`
    removes every direct Policy. Destroy likewise removes the complete current
    direct set and then drops state. It never reads as authority from the
    combined or inherited Policy collections, never removes a Group edge or
    inherited Policy, and never updates or deletes the Member.

  All IDs and Import components use canonical lower-case 8-4-4-4-12 UUID
  spelling. Computed IDs stay known only while every replacement input is
  unchanged; synthetic pair IDs use the corresponding canonical Import form.
  Every list is consumed through all pages and resolved as exact
  zero/one/duplicate. Direct object 404 alone is not authoritative absence.
  Every mutation executes once, is followed by an exact reread, and preserves
  the last confirmed state when completion or absence is ambiguous. Ambiguous
  object Create is reconciled by exact identity for recovery diagnostics but
  is never retried or automatically adopted without a confirmed returned ID.
  Only the multi-call Policy and per-Member authoritative-set lifecycles require
  narrow write serialization.

## Provider implementation

- [x] **P6-040 — Add exact-key Project and Environment lookup.**

  Extend the existing data sources to select a Project by exact organization
  key and an Environment by exact key within its Project, while preserving UUID
  configurations. Reuse the complete Project read and reject duplicate matches.
  Confirm the existing Feature Flag and Segment outputs provide the exact keys
  needed by specific-resource selectors.

  Runtime: `project_data_source.go` and `environment_data_source.go` → existing
  Project/Environment client adapters. Done when UUID and key selectors both
  pass focused and Protocol tests.

  Result: both data sources now require exactly one Optional+Computed `id` or
  `key` selector. Project keys resolve case-sensitively across the complete
  organization Project collection; Environment keys resolve case-sensitively
  across the exact parent Project's complete nested Environment list. Exact
  zero returns not found, duplicate exact keys are ambiguous, UUID lookup is
  preserved, and diagnostics retain the existing identity-redaction boundary.
  Focused and Protocol v6 tests cover both selector forms and the Protocol
  schema snapshot is aligned. Existing Feature Flag and Segment data-source
  state already exposes their exact keys and retains Protocol assertions for
  those outputs. Repository tests, vet, build, formatting, and module tidy
  checks pass. A follow-up reuse pass consolidated selector extraction and
  validation, complete pagination accounting, safe read-error sanitization,
  and mutation-reconciliation classification without changing schemas or
  lifecycle behavior.

- [x] **P6-050 — Implement Policy management and lookup.**

  Add a custom Policy resource that owns settings and all statements as one
  Terraform object, even though Create uses separate Policy and statement API
  calls. Support Project, Environment, Feature Flag, and Segment statements,
  and add exact-key Policy lookup for custom and built-in Policies.

  Runtime: Policy resource/data source → `internal/client/policies.go` → public
  Policy endpoints. Done when CRUD, Import, drift, canonical statements,
  partial failure, built-in read-only behavior, and all four control levels
  pass.

  Result: registered `featbit_policy` resource and data source now use the
  documented paginated Policy, exact read, settings, complete-statements,
  association, and delete endpoints. The resource owns one custom Policy in a
  serialized two-step lifecycle with exact-key create preflight, canonical
  four-level statements, confirmed intermediate-state preservation, Import,
  drift, association-safe destroy, and complete-list absence proof. The data
  source resolves an exact UUID or case-sensitive exact key and can observe
  built-in statement shapes, while every resource path rejects `SysManaged`
  Policies before mutation. Focused client/resource/data-source tests cover
  pagination, duplicates, one-shot mutations, empty statements, all control
  levels, partial failure, invalid remote state preservation, redaction, and
  built-in no-mutation behavior. The Protocol v6 schema/import snapshot is
  aligned; repository tests, vet, build, touched-file formatting, module tidy,
  and module verification pass.

- [x] **P6-060 — Implement Group management.**

  Add Group CRUD and Import for name and description only, plus exact read-only
  lookup of an existing Group by ID or organization-scoped name. Do not place
  member IDs or Policy IDs in the Group resource or data source.

  Runtime: Group resource/data source → `internal/client/groups.go` → public
  Group endpoints. Done when lifecycle, exact lookup and absence, drift, and
  ambiguous mutation handling pass.

  Result: the registered `featbit_group` resource owns only canonical UUID,
  name, and description, while its data source observes an existing Group by
  exact UUID or organization-scoped, case-sensitive exact name without taking
  ownership. Both use the documented complete paginated list and exact read;
  the resource additionally uses create, update, delete, and minimal Group
  Member/Policy collection reads. The lifecycle enforces exact-name create
  zero, one-shot mutation
  reconciliation, canonical drift and out-of-band absence, empty-association
  destroy preflight, complete-list deletion proof, and canonical UUID Import.
  Focused client/resource/data-source tests cover pagination, duplicate and
  missing selectors, redaction, association refusal, and ambiguous mutations
  before or after remote apply. Protocol schema/Import, repository tests, vet,
  build, formatting, module
  tidy, and module verification pass. A follow-up production reuse/dead-code
  pass removed the duplicate
  canonical Group type, centralized exact zero/one/duplicate resolution across
  Project, Environment, Policy, and Group, and centralized complete IAM
  association-ID pagination for Policy and Group. Go `x/tools/deadcode`
  confirms every business method is reachable; only deliberate
  `fmt.Formatter` redaction hooks remain reported as statically unreachable.

- [x] **P6-070 — Implement Group-Policy bindings.**

  Add a resource for one exact Group/Policy pair. Read through the complete
  Group Policy collection; Create and Destroy add or remove only that pair.

  Reuse the existing complete association-ID pagination in `groups.go`:
  refactor the current count-only path into one narrow ID-returning helper,
  expose the Group Policy IDs needed by the binding, and derive the existing
  Group association counts with `len`. Do not add a second pagination,
  membership-validation, UUID-canonicalization, or redaction path.

  Runtime: binding resource → Group client adapter → public Group Policy
  endpoints. Done when Import, repeated apply, pre-existing binding, drift,
  out-of-band removal, and reuse of the single complete association-ID path
  pass.

  Result: registered `featbit_group_policy_binding` now owns only one canonical
  Group/Policy pair, uses replacement-aware synthetic ID and Import form
  `<group_uuid>/<policy_uuid>`, accepts custom or built-in Policies, and adopts
  an already-existing exact pair without mutation. Create and Destroy resolve
  both endpoint IDs through their complete token-scoped collections, execute
  at most one documented add/remove call, and require an exact complete
  relationship reread; Read removes state only on authoritative pair absence,
  including a confirmed missing endpoint, while ambiguous reads preserve
  state. The Group client exposes canonical Policy IDs through the existing
  single complete association paginator, and both prior Group association
  counts now derive from that same ID-returning path. Focused client/resource
  tests cover pagination, Member/Policy membership validation, duplicate IDs,
  one-shot mutations, pre-existing and built-in bindings, repeated refresh,
  drift, exact out-of-band removal, unrelated-pair preservation, ambiguous
  preflight and mutation reconciliation, the read-only Update safety path,
  replacement planning, inconsistent synthetic state, redaction, and canonical
  Import. A whole-repository reuse/reachability/test audit retained the shared
  association paginator and narrow Group mutation helpers without introducing
  a cross-resource binding abstraction; no unused production business method
  remains. It removed one unused test method and one overwritten test
  assignment and simplified one redundant nil/length check. Go
  `x/tools/deadcode` now reports only deliberate dynamic `fmt.Formatter`
  redaction hooks, and targeted Staticcheck is clean. The Protocol v6
  schema/Import snapshot is aligned; repository tests, vet, build, formatting,
  module tidy, and module verification pass.

- [x] **P6-080 — Implement Member lookup and Group-Member bindings.**

  Add exact existing-Member lookup by ID or email and a resource for one exact
  Group/Member pair. Decode only safe Member fields; never create, invite,
  update, or delete the Member.

  Runtime: Member data source and binding resource → Member/Group client
  adapters → public Member and Group-member endpoints. Done when exact lookup,
  Import, repeated apply, drift, and identity redaction pass.

  Result: registered `featbit_member` now reads one existing Member by exact
  UUID or organization-scoped case-insensitive full email across every page,
  rejects duplicate exact matches, retains canonical server spelling, and
  exposes only Sensitive `id`, `email`, and `name`. Its client adapter is an
  explicit three-field allowlist, so invitation, Group, and
  `initialPassword` response fields never enter Provider models. Registered
  `featbit_group_member_binding` owns only one canonical Group/Member pair,
  marks its synthetic ID and Member ID Sensitive, supports
  `<group_uuid>/<member_uuid>` Import, adopts an
  already-present edge, tracks exact drift and authoritative endpoint absence,
  and reconciles each one-shot add/remove without changing unrelated edges.
  Group Member IDs and counts reuse the existing complete association
  paginator, while Member and Policy pair mutations share one narrow Group
  mutation helper. A whole-repository follow-up audit consolidated the
  Group-Policy and Group-Member production resources into one narrow
  kind-parameterized lifecycle engine while preserving distinct schemas,
  sensitivity, endpoints, diagnostics, and Import forms; direct Client method
  expressions also replaced forwarding-only adapters. Existing
  `google/uuid`, Terraform Framework, and standard-library primitives already
  own the reusable generic behavior, so no additional dependency was
  justified for endpoint-specific pagination, exact matching, reconciliation,
  or redaction. The complete Group-Policy suite now owns the shared lifecycle
  matrix, while the smaller Member suite verifies only its sensitive schema
  and endpoint wiring. Focused coverage additionally proves target-ID
  replacement, non-authoritative direct 404, and confirmed-missing-Member
  deletion; redundant duplicate lifecycle and global registration-count tests
  were removed. Production dead-code analysis reports only deliberate dynamic
  `fmt.Formatter` redaction hooks, and test-inclusive analysis is clean after
  removing one unreachable legacy test formatter. Protocol v6 schema/Import,
  repository tests, client/provider coverage, vet, build, formatting, module
  tidy, and module verification pass.

- [x] **P6-090 — Implement authoritative direct Member Policies.**

  Add one resource that reconciles only the complete direct Policy set of one
  existing Member. An empty set removes direct Policies; Group-inherited
  Policies are never changed.

  Runtime: direct-Policy resource → Member client adapter → direct-Policy list,
  add, and remove endpoints. Done when empty-set enforcement, Import, drift,
  partial failure, reread, and destroy follow P6-030.

  Result: registered `featbit_member_direct_policies` now owns one existing
  Member's complete direct Policy UUID set, uses the canonical sensitive
  Member UUID as its ID and Import form, and treats an empty set or Destroy as
  removal of direct Policies only. The Member adapter consumes every page of
  `direct-policies`, decodes only canonical relationship IDs and the direct
  membership flag, and shares the established one-shot Boolean association
  mutation contract. Create and Update resolve the exact Member and every
  desired custom or built-in Policy through complete token-scoped collections,
  add missing IDs before removing extras in canonical order, reread after every
  mutation, and persist each confirmed intermediate set. Per-Member write
  serialization, ambiguous-outcome reconciliation, final exact rereads, drift,
  external Member absence, and partial add/remove failure preserve truthful
  state without reading or changing inherited Policies or Group edges. Focused
  client/resource tests cover pagination, invalid collections, empty-set
  enforcement, idempotence, Import, replacement planning, redaction, drift,
  partial failure, and Destroy isolation. A whole-repository reuse and test
  audit made the shared IAM association paginator return deterministically
  sorted canonical IDs, moved generic Terraform string-set conversion out of
  Segment ownership, and replaced handwritten equality, containment, sorting,
  and deduplication with Go `slices` operations. It removed two forwarding-only
  production methods, two redundant production collection helpers, and one
  cross-file test helper; existing dependencies already cover the reusable
  behavior, so no new third-party dependency is justified. Focused coverage
  additionally proves unsorted remote pagination, built-in Policy targets,
  Import refresh, non-authoritative direct 404, successful mutation no-ops,
  ambiguous remove reconciliation, add-before-remove safety, cancellation, and
  canonical per-Member locking. One duplicate Import subcase was folded into
  the Import-refresh path; every remaining test owns a distinct contract. The
  Protocol v6 schema/Import snapshot is aligned; repository tests,
  client/provider coverage (87.3%/77.8%), vet, build, touched-file formatting,
  module tidy/verification, and production/test-inclusive reachability checks
  pass. Production reachability reports only deliberate dynamic
  `fmt.Formatter` redaction hooks, while test-inclusive reachability is clean.

## Verification and release

- [x] **P6-100 — Prove the complete IAM workflow locally.**

  Add Protocol and integration coverage for the customer-shaped resource
  graph. Look up Project/Environment and the built-in Owner Policy by exact
  key. Create a base-access custom Policy for Project/Environment visibility
  and a scoped custom Policy for dev Feature Flag/Segment operations plus one
  exact prod Feature Flag exception. Attach Owner to an admin Group, attach
  both custom Policies to a developer Group, assign an existing Member only to
  the developer Group, and reconcile that Member's direct Policy set to empty.

  Across the workflow and focused variants, cover all four statement levels,
  wildcard and exact-resource selectors, Allow/Deny, and action subsets. Prove
  existing Group lookup by exact UUID and name, Import, drift, redaction,
  cleanup, and an empty second plan. A binding must accept the looked-up Group
  ID. Removing one binding must remove only its exact pair and preserve the
  other Policy and Member bindings.

  Result: `internal/provider/iam_workflow_integration_test.go` now drives the
  registered Protocol v6 provider and existing client adapters against one
  stateful documented-endpoint fixture. The graph creates one test-owned
  Project, resolves it plus its `dev`/`prod` Environments and the immutable
  built-in Owner Policy by exact key, creates the base/scoped custom Policies
  and admin/developer Groups, observes both Groups by exact UUID/name, and feeds
  those observed IDs into the three Group-Policy and one Group-Member bindings.
  One protected existing Member is assigned only to the developer Group and its
  authoritative direct Policy set converges from Owner to empty without profile
  mutation. The graph round-trips `project`, `env`, `flag`, and `segment`
  statements through reversed server order with wildcard/exact selectors,
  Allow/Deny, and action subsets, then produces an empty second plan. Removing
  only the developer base-Policy binding sends one exact remove, while the
  admin Owner, developer scoped-Policy, and developer Member bindings remain.
  Automatic destroy reaches exact zero for every test-owned Project, Policy,
  Group, and relationship while preserving the built-in Owner and existing
  Member; endpoint-only passwords, Environment secrets, and the access token do
  not enter state. The established focused suites continue to prove every IAM
  Import form, per-resource drift, exact zero/duplicate behavior, and diagnostic
  redaction. The graph exposed and fixed deferred validation of computed Policy
  selector set elements: full catalog validation now waits until values are
  known, while Create still fails closed before mutation. Targeted and complete
  repository tests pass (client/provider coverage 87.3%/79.0%), as do vet,
  build, formatting of touched Go files, module tidy/verification, actionlint,
  and the 180-second current-Terraform Protocol selector. Ordinary and
  compatibility per-package ceilings are now 180 seconds for the expanded
  serial Protocol suite.

- [x] **P6-110 — Pass trusted current-Cloud acceptance.**

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

  Result: **passed on current Cloud on 2026-08-18.** The full five-step run
  round-tripped the four statement levels, produced empty plans, proved exact
  Project/Environment reads, reversible allowed Flag and Segment writes, the
  sibling prod Flag 403, and loss then restoration of access across the exact
  developer Member edge. Mutations remained one-shot; successful revision-ID
  responses were accepted only after exact object reconciliation. Independent
  final inventory found zero test Projects, Policies, and Groups, restored the
  original one-Policy direct baseline with zero Group/inherited Policies, and
  confirmed the exact built-in `owner` remained system-managed. Credentials
  and the runtime-only Member identity were not retained.

- [x] **P6-120 — Publish the supported IAM surface in documentation.**

  Add resource/data-source pages and runnable examples, regenerate Registry
  docs, and update README and schema assertions. Show the required parent
  Project/Environment access alongside Feature Flag and Segment permissions.
  State clearly that Member CRUD, Service Token management, and Phase 7 Segment
  prerequisites are unsupported.

  Result: Registry templates, generated pages, and independently validatable
  examples now cover all five IAM resources and three IAM data sources, while
  the existing Project and Environment data-source pages demonstrate exact-key
  lookup. The Policy reference publishes the frozen four-level action catalog,
  selector grammar, and required parent Project/Environment access alongside
  Flag and Segment permissions. Group pair ownership, authoritative direct
  Member Policies, Sensitive Member state, built-in Policy immutability,
  Member lifecycle exclusion, external Service Token management, and deferred
  Segment End User/property prerequisites are explicit. Public guides and the
  release assertions now target the additive `0.2.x` IAM line. The generated
  9-resource/7-data-source document surface is byte-current; all 17
  credential-free example sets validate, and repository tests, vet, build,
  module tidy/verification, formatting, and diff checks pass.

- [x] **P6-130 — Qualify the IAM release.**

  Run formatting, unit/race tests, vet, build, module checks, Protocol/schema
  checks, generated-doc checks, redaction scans, and the GoReleaser snapshot.
  Confirm the release contains exactly the approved IAM surface.

  Result: the first IAM candidate is frozen as `v0.2.0-beta.1`; public examples
  pin that exact prerelease, and GoReleaser marks prereleases without replacing
  the latest stable release. The toolchain is frozen at Go 1.26.6 after the
  patch release removed six reachable standard-library vulnerabilities.
  Formatting, unit and Linux race tests, vet, build, module graph/tidy/verify,
  generated docs and 17 examples, workflow syntax, licenses, vulnerability and
  secret scans, release configuration, and Terraform `1.0.11`, `1.5.7`, and
  `1.15.8` Protocol contracts pass. The Protocol v6 release snapshot contains
  exactly five provider attributes, nine resources, seven data sources, and no
  other framework surfaces. The standard snapshot and exact beta candidate
  each produce only the five frozen single-executable archives; all six
  checksums, including the Protocol 6.0 manifest, verify. No credential, tag,
  signature, draft, or publication was created.

- [ ] **P6-140 — Publish and verify the IAM beta.**

  Begin only with explicit maintainer authorization. Create and inspect the
  signed draft for exact tag `v0.2.0-beta.1` through the existing protected
  workflow. Confirm GitHub classifies it as a prerelease without changing the
  latest stable release, publish it, and verify a clean Terraform directory can
  install and use that exact Registry version. Do not close Phase 6 yet.

- [ ] **P6-150 — Exercise real scenarios and remediate the beta.**

  Use the published beta in the intended real customer scenarios. Record only
  current actionable findings, fix every release-blocking defect, and rerun the
  complete P6-130 qualification on the resulting candidate. Publish another
  beta only when needed and only with explicit maintainer authorization.

- [ ] **P6-160 — Publish and close stable `v0.2.0`.**

  Begin only after beta scenario findings are resolved and the final candidate
  passes the complete release gate, and only with explicit maintainer
  authorization. Create, inspect, publish, and Registry-verify stable
  `v0.2.0`; then merge only still-current facts into the master plan and remove
  the completed Phase 6 package. Do not start Phase 7 implementation on this
  branch.
