# FeatBit Terraform Provider Plan

- Status: **Active**
- Current release: `v0.2.0`
- Module: `github.com/featbit/terraform-provider-featbit`
- Registry address: `registry.terraform.io/featbit/featbit`
- Next phase: **Not selected**
- Next task: **Not selected**

This file contains only the current Provider position, product boundary, and
prioritized future work with its blockers. It is not a completed-phase or
release-history archive.

## 1. Current position

Stable `v0.2.0` exposes five Provider attributes, ten resources, and seven data
sources across the implemented core and IAM surfaces.

The Provider manages Projects, Environments, Feature Flag definitions,
environment-specific Segments, custom Policies, Groups, explicit IAM bindings,
and exact lookup of existing core and IAM objects. Feature Flag operational
state remains UI-owned; shared Segments remain read-only; Segment End
User/property prerequisites, Member account lifecycle, and Service Access
Token lifecycle remain outside the implemented surface.

FeatBit Cloud is the verified runtime target. A configurable API origin keeps
self-hosted deployments as an intended target, but no exact self-hosted release
is currently certified. No next phase or implementation task is selected.

## 2. Product boundary

The Provider manages FeatBit configuration through documented public REST
APIs. Terraform owns only fields and relationships explicitly declared by each
resource.

Terraform does not implicitly own UI-managed operational Flag state, shared
End Users or custom-property metadata, inherited IAM permissions, Member or
workspace accounts, credentials, or secret values. Pair and complete-set IAM
ownership must not overlap for the same relationship collection.

It does not call UI-only APIs, access the FeatBit database directly, or expose
an arbitrary HTTP resource. Runtime flag evaluation, FeatBit deployment, and
feature-evaluation event pipelines remain outside this Provider.

## 3. Prioritized future work

This order is based on customer GitOps value and the 2026-08-24 comparison of
the current Provider with FeatBit `v5.4.7`. Readiness does not change business
priority: a blocked item keeps its position, but it must not force use of a
private API or stop independent ready work.

| Priority | Outcome | Current readiness |
|---|---|---|
| 1 | Complete environment-specific Feature Flag operational GitOps | **Ready.** Required Flag and Environment operations are public. |
| 2 | Close Segment and Flag End User/custom-property prerequisites | **API-blocked.** Public End User and End User Property operations are missing. |
| 3 | Manage least-privilege Service Access Tokens | **API-blocked.** Public Access Token lifecycle operations are missing. |
| 4 | Improve Feature Flag definition and retirement lifecycle | **Provider-ready after priority 1.** Public description, variation, tag, archive, and restore operations exist. |
| 5 | Add safely bounded Organization Member lifecycle | **Design-gated.** Basic public Member add/read/remove operations exist. |
| 6 | Manage shared Segments | **API/ownership-gated.** Safe shared-scope mutation ownership is not proven. |
| 7 | Add remaining governance and integrations | **Mixed.** Some Workspace/OIDC/audit operations are public; Webhook, scheduling, approval mutation, and secret lifecycle gaps remain. |

### Priority 1 — Manage Feature Flag targeting and enabled state

Goal: let Terraform opt into ownership of one Feature Flag's
environment-specific behavior instead of managing only its definition.

Readiness: the current public OpenAPI exposes Environment update plus Feature
Flag read, targeting, toggle, off variation, variations, tags, archive,
restore, and pending-change operations. The base work has no known external API
blocker, but the selected implementation phase must freeze the exact request,
authentication, ownership, and conflict contract before writing runtime code.

Implement in this order:

1. Freeze the current public request/read contract for Environment
   `requireChangeComment`, mutation audit comments, enabled state, off
   variation, targeting, ordered rules, and fallthrough or percentage
   distribution.
2. Add optional mutation-comment plumbing and narrowly owned
   `require_change_comment` Environment governance. CI must be able to supply a
   useful PR/commit comment without making it ordinary object state or causing
   perpetual diffs.
3. Add a separate opt-in operational resource, working name
   `featbit_feature_flag_targeting`. It references one exact existing
   Environment and Feature Flag key and owns only the frozen operational
   surface. The existing `featbit_feature_flag` remains the definition owner.
4. Define explicit coexistence behavior for UI edits, pending changes,
   experiments, approvals, archived Flags, concurrent changes, Import, drift,
   and removal of Terraform ownership.
5. Prove canonical readback, one-shot mutation reconciliation, stable Import,
   non-destructive ownership release, and an empty second plan.

Direct user targets and custom-property rules retain the existing prerequisite
limitation until priority 2 is unblocked: the referenced Environment user and
property metadata must already exist. Priority 1 must not call Portal-private
prerequisite endpoints.

Flag tags and safe in-place definition variation changes belong to priority 4,
not to the operational resource, unless contract analysis proves a field is
inseparable from canonical operational state.

### Priority 2 — Targeting prerequisite closure

Goal: make fresh direct targets and custom-property rules fully declarative for
both Segments and Feature Flags.

External condition: this priority is blocked until the documented public API
and Access Token authentication provide:

- exact Environment-scoped End User lookup by exact key, across complete
  pagination when applicable;
- idempotent create-missing-only End User registration with explicit
  non-overwrite and conflict behavior;
- exact Environment-scoped End User Property lookup by exact property name;
- idempotent create-missing-only custom-property registration; and
- documented case, duplicate, authorization, retry, pagination, and
  redaction-safe failure semantics.

A destructive upsert, fuzzy search, Portal-private route, or direct database
operation does not satisfy this dependency.

After the required public API ships, the Provider should:

- collect and deterministically deduplicate exact user keys and non-built-in
  custom property names;
- look up each prerequisite in the exact Environment;
- create only values proven missing;
- never overwrite an existing user's name, custom properties, or metadata;
- complete prerequisite registration before targeting mutation;
- preserve truthful state after partial failure or an ambiguous outcome; and
- never delete shared End Users or property metadata when targeting changes or
  a Segment/Flag resource is removed.

The implementation should be shared by Segment and Feature Flag targeting only
where their semantics and safety boundaries genuinely match.

### Priority 3 — Service Access Tokens

Goal: bootstrap with an externally supplied Personal Token, then let Terraform
create least-privilege Service Tokens for downstream CI/CD workloads.

External condition: this priority is blocked until the documented public API
provides:

- exact list/read metadata and a stable token identifier;
- server-side Service Token creation with a clearly defined one-time secret;
- inline Policy-statement permissions matching the current product model;
- name/permission update, enable/disable, rotation or replacement, and
  revoke/delete behavior;
- Import/read behavior that never requires returning an existing plaintext
  secret; and
- redaction-safe authorization, conflict, and failure responses.

The future resource should manage one server-created Service Token, its name,
inline Policy statements, active/inactive status, rotation or replacement, and
revocation. Its creation secret must be computed and Sensitive, with an
explicit warning that Terraform state contains the plaintext value.

FeatBit Service Tokens currently use inline permission statements. No
Token-to-Group relationship should be invented unless the FeatBit product and
public API add that relationship explicitly.

### Priorities 4–7 — Follow-up completeness

After priority 1, recheck the external conditions for priorities 2 and 3. If
they remain blocked, continue with the highest independent ready item:

1. Feature Flag tags, safe in-place description/variation changes, and explicit
   archive/restore or archive-on-destroy semantics.
2. Organization Member lifecycle limited to organization membership; never
   leak generated passwords or delete a shared workspace account implicitly.
3. Shared Segment mutation only after shared-scope identity, ownership,
   reference, archive, and destroy behavior are public and safe.
4. Webhooks, schedules, approvals/change requests, Workspace OIDC, audit
   observation, and Environment secret rotation, each as a separate capability
   with its own public-API and sensitive-state contract.

These priorities are not assigned to phase numbers. Create a phase README/TODO
only after the user explicitly selects the next scope.
