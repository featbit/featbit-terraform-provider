# Phase 6 TODO — Segment targeting prerequisites

Work one item at a time. Before adding a helper, wire model, client method,
lifecycle branch, or test fixture, search the existing implementation for a
compatible contract. Record only concise current results under the active item.
Do not begin IAM, publish a release, modify the FeatBit backend, or use a
Portal-private endpoint without a separate explicit user authorization.

## Public API gate

- [ ] **P6-010 — Verify the documented public prerequisite API.**

  Read the official Swagger/OpenAPI document and its authentication contract.
  Trace the corresponding FeatBit UI, controller, application service, and
  authorization code only to understand semantics and determine whether each
  operation is intentionally public. Do not equate an endpoint used by the UI
  with a stable Provider API.

  Prove or disprove exact Environment-scoped lookup and idempotent
  create-missing-only registration for both End Users and End User Property
  metadata. Record method, documented path, request/response shape, status and
  conflict behavior, access-token support, pagination or exact-match behavior,
  retry safety, and whether errors can be diagnosed without echoing protected
  identifiers.

  Important sources: official FeatBit Swagger/OpenAPI, `README.md` supported
  surface, existing provider client/transport/redaction tests, and read-only
  FeatBit front-end/back-end source references for semantic comparison.

  Stop condition: if any required operation is undocumented, Portal-private,
  unable to authenticate with the Provider token, fuzzy-only, or destructive
  upsert-only, do not add a client call. Record the smallest upstream public
  contract needed: exact Environment-scoped lookup plus idempotent
  create-missing-only registration for users and property metadata, with
  explicit non-overwrite, conflict, authorization, and redaction semantics.

## Ownership and lifecycle design

- [ ] **P6-020 — Freeze the Terraform ownership and drift contract.**

  Begin only if P6-010 proves a sufficient public API. Compare implicit
  prerequisite ensure in `featbit_segment` with first-class resources. Prefer
  the smallest design that can truthfully handle Import, refresh, out-of-band
  prerequisite deletion, partial failure, retries, cancellation, and concurrent
  applies without claiming destroy ownership of shared records.

  Freeze deterministic handling for existing Environment users, Global-only
  users, duplicate keys, a key present in both included and excluded sets,
  repeated rule properties, built-in property recognition, and case semantics.
  Specify call ordering, one-shot mutation rules, state preservation, and the
  exact point at which targeting may be sent. Document any schema/state or
  migration effect before implementation.

## Provider implementation

- [ ] **P6-030 — Add exact, redaction-safe prerequisite client operations.**

  Reuse the existing transport, escaping, retry, cancellation, error
  classification, and diagnostic-redaction behavior. Keep endpoint wire models
  with their caller. Add focused table-driven tests for exact lookup,
  create-missing-only requests, already-existing/conflict outcomes, retry
  safety, cancellation, malformed responses, and protected-value redaction.

- [ ] **P6-040 — Integrate the frozen prerequisite lifecycle with Segment.**

  Canonicalize and deduplicate included/excluded keys and non-built-in rule
  properties. Query exact records and create only missing prerequisites before
  `UpdateSegmentTargeting`. Never mutate existing user data. Preserve
  truthful Terraform state when a prerequisite or targeting call fails, and
  never delete prerequisites during update or destroy.

  Keep Create, Update, Read, Import, refresh, and drift behavior aligned with
  P6-020. Reuse existing Segment request expansion and reconciliation instead
  of introducing a second targeting implementation.

## Verification and documentation

- [ ] **P6-050 — Freeze lifecycle, ordering, idempotence, and redaction tests.**

  At minimum cover: creation registers missing included/excluded users;
  existing users are not modified; update registers only newly missing users;
  removal deletes no user; custom property registration is deduplicated and
  reused; built-ins trigger no registration; repeated apply is idempotent;
  user/property failure cannot report complete success; HTTP order and request
  counts are exact; concurrent/conflicting inputs follow P6-020; logs and
  diagnostics remain redacted; and all existing Segment
  Import/read/update/delete tests continue to pass.

- [ ] **P6-060 — Pass a trusted current-Cloud acceptance gate.**

  Use unique test-owned Project, Environment, Segment, user keys, and custom
  property. Prove exact Environment association, metadata existence, existing
  record preservation, empty second plan, and non-deletion after targeting
  removal and Segment destroy. Independently clean up only the enclosing
  test-owned Project tree after evidence is captured. Never print protected
  IDs, keys, properties, targeting values, or credentials.

- [ ] **P6-070 — Update supported-surface documentation and examples.**

  If implementation proceeds, update the canonical resource example,
  templates, generated Registry docs, README, compatibility notes, and focused
  documentation assertions to describe the exact ownership boundary. If
  P6-010 stops implementation, document the unsupported public-API prerequisite
  precisely without recommending Portal-private calls or SDK pre-registration
  as the only Terraform solution.

- [ ] **P6-080 — Run the complete phase gate.**

  Run formatting, `go test ./...`, vet, build, module verification, generated
  docs and examples checks, Protocol/schema contracts, GoReleaser snapshot,
  secret/redaction checks, and repository diff checks. A runtime release remains
  separately maintainer-authorized.

## Phase exit

- [ ] **P6-090 — Close Phase 6 before creating the IAM execution package.**

  Confirm the README exit gate. Merge only still-current architecture and
  roadmap facts into the master plan, delete this completed package, and create
  only the Phase 7 IAM README/TODO. Do not preserve phase history files or begin
  IAM while any Phase 6 gate is unresolved.
