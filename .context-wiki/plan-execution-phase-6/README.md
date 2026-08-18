# Phase 6 — IAM and release

## Purpose

Deliver the customer-requested IAM workflows through the documented public
API, then publish the next Provider release. This branch contains IAM and its
release only; deferred Segment prerequisite work remains in Phase 7.

## Current entry point

The public IAM API gate passed, the Terraform schema and lifecycle contract is
frozen, and exact-key Project/Environment lookup plus Policy and Group
management and lookup are implemented. Start with [P6-070](todo.md): add one
exact Group-Policy binding resource through the proven public IAM endpoints.

## IAM v1 scope

Managed resources:

- a custom Policy whose settings and statements are one Terraform resource;
  statements cover Project, Environment, Feature Flag, and Segment control
  levels;
- a Group whose name and description are managed independently from bindings;
- one exact Group-to-Policy binding;
- one exact Group-to-Member binding; and
- one existing Member's complete direct-Policy set, including an empty set for
  group-only effective access.

Read-only data sources:

- a Policy resolved by exact key, including built-in policies such as Owner;
- an existing organization-wide Group resolved by exact ID or case-sensitive
  exact name;
- an existing Member resolved by exact ID or email; and
- Project and Environment lookup by exact key, added to the existing data
  sources without breaking UUID lookup.

Explicit exclusions:

- Member invitation, creation, profile changes, organization/workspace
  removal, and deletion;
- Service access-token creation or Group assignment;
- mutation of built-in Policies;
- authoritative ownership of a Group's complete member or Policy collection;
  and
- all Phase 7 Segment targeting prerequisite work. Phase 6 may authorize
  Segment operations, but it does not create End Users or property metadata.

## Ownership rules

- The custom Policy resource hides the API's create-then-set-statements
  sequence and always reads back the complete canonical Policy.
- Policy statements preserve `resource_type`, `effect`, `actions`, and resource
  selectors. Supported selector levels are `project`, `env`, `flag`, and
  `segment`, including documented wildcard and exact-key forms.
- Group-to-Policy and Group-to-Member resources own one exact pair. Destroy
  removes only that pair.
- The Group data source observes an existing Group without adopting its
  lifecycle or relationships; its ID can feed either binding resource.
- The Member direct-Policy resource is intentionally authoritative for one
  Member's direct Policy set. It never owns inherited Group Policies or the
  Member lifecycle.
- Built-in Policies, Groups selected through the data source, and existing
  Members are observed, not adopted as managed objects.
- Exact lookup scans complete paginated results and rejects zero or duplicate
  matches; fuzzy search results are never accepted.

## Guardrails

- Use documented public APIs only; never depend on Portal-private controllers
  or direct database access.
- Never store or expose tokens, initial passwords, tenant/member identities,
  request paths, or unsafe response bodies in state, fixtures, logs, or
  diagnostics.
- Reuse existing transport, escaping, pagination, exact resolution, error
  classification, cancellation, concurrency, and redaction contracts.
- Mutations execute once. Ambiguous results require exact reconciliation and
  truthful state preservation.
- Do not create a tag, sign or finalize release assets, or publish a release
  without explicit maintainer authorization.

## Exit gate

The phase passes only when:

- every consumed IAM operation is documented, tenant-scoped, exact, and usable
  with Provider access-token authentication;
- Policy statements round-trip Project, Environment, Feature Flag, and Segment
  control levels with canonical effects, actions, and wildcard or exact-key
  selectors;
- Policy-with-statements, Group, both exact bindings, and authoritative direct
  Member Policies pass lifecycle, Import, drift, and empty-second-plan tests;
- built-in Policy, Group-name, Member, Project-key, and Environment-key lookup
  reject zero and duplicate exact matches;
- a trusted current-Cloud run proves the customer workflow without creating or
  deleting a Member and restores every test-owned relationship;
- diagnostics, logs, fixtures, and state contain no protected values;
- Registry documentation and release artifacts expose exactly the approved IAM
  surface; and
- the maintainer-authorized IAM release is published.

Passing this gate ends this branch. Phase 7 continues separately.
