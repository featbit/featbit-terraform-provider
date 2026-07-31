# FeatBit Terraform Provider Implementation Plan

> Status: Draft (English version)  
> Research baseline: July 30, 2026  
> Target repository: `github.com/featbit/terraform-provider-featbit`  
> Intended Registry address: `registry.terraform.io/featbit/featbit`
>
> Active execution package: [Phase 1 — Repository and provider scaffold](./plan-execution-phase-1/README.md)

## 1. Conclusions and implementation principles

1. Use Go and HashiCorp's currently recommended **Terraform Plugin Framework**. Do not start a new provider on the legacy `terraform-plugin-sdk/v2` architecture.
2. Use **Terraform Plugin Protocol v6** and set the Registry manifest protocol to `["6.0"]`.
3. Use the LaunchDarkly provider as an engineering reference for mature Terraform patterns—CRUD behavior, imports, data sources, tests, documentation, and releases—but let FeatBit customer workflows and the current FeatBit API determine the public schema. Resource-for-resource parity is not a goal.
4. A FeatBit feature flag is already scoped to an environment. LaunchDarkly's two-level `feature_flag` and `feature_flag_environment` model cannot be copied directly.
5. The first release should prioritize the resources customers need most:
   - `featbit_project`
   - `featbit_environment`
   - `featbit_feature_flag`
   - `featbit_segment`
   - Corresponding single-object data sources and import support
6. The second stage should add IAM:
   - Groups
   - Policies
   - Group/member/policy bindings
   - Member lookup and, only if exact-email reconciliation proves safe, member creation
7. Assume the current public API cannot change for v1. Deliver the provider through a compatibility layer, constrained schemas, replacement semantics, exact lookups, post-write verification, and documented external prerequisites. Backend improvements are optional future optimizations, not GA dependencies.

## 2. July 2026 technical baseline

### 2.1 Verified versions

| Component | Planned version | Rationale |
|---|---:|---|
| Go module language baseline | `go 1.25.8` | Matches the official Framework scaffold from July 27, 2026 and LaunchDarkly provider v3.1.1 from July 28, 2026 |
| Minimum Go CI patch | `1.25.12` | Current security patch line for Go 1.25 |
| Go CI/release toolchain | `1.26.5` | Latest stable Go on July 30, 2026; use for releases after cross-platform verification |
| Terraform Plugin Framework | `v1.19.0` | Latest stable release from March 10, 2026 |
| Terraform Plugin Go | `v0.31.0` | Version used by the current official scaffold |
| Terraform Plugin Testing | `v1.16.0` | Latest stable release from April 23, 2026 |
| Terraform Plugin Log | `v0.10.0` | Version used by the current official scaffold |
| Terraform Plugin Docs | `v0.25.0` | Latest stable release from April 20, 2026 |
| Terraform Plugin Protocol | `v6` | Current recommended protocol; compatible with Terraform CLI 1.0+ |
| Current stable Terraform CLI | `1.15.8` | Released July 8, 2026 |
| GoReleaser | v2 configuration | Follow the official scaffold's v2 format and pin the executable version in CI |

At implementation kickoff, verify these versions again, pin exact versions, and commit `go.mod`, `go.sum`, and the tools module. CI must not depend on floating `latest` versions.

### 2.2 Reference implementations

- HashiCorp official scaffold:
  [`terraform-provider-scaffolding-framework@f781750`](https://github.com/hashicorp/terraform-provider-scaffolding-framework/tree/f781750309d9d63c50f9d6992a788fa7245ec7fc)
- Current LaunchDarkly implementation:
  [`terraform-provider-launchdarkly@4bd1eec`, v3.1.1](https://github.com/launchdarkly/terraform-provider-launchdarkly/tree/4bd1eecb87d61c4b8c11031ce7cb1fa95add2309)
- The current LaunchDarkly provider also uses Plugin Framework `v1.19.0` and Protocol v6. Its provider configuration, lifecycle patterns, tests, documentation, and release setup are useful references; its resource inventory is not a FeatBit roadmap.

## 3. Product goals and non-goals

### 3.1 Core provider goals

- Let users declare the desired state of FeatBit projects, environments, flags, and segments in HCL.
- Support `plan`, `apply`, `refresh`, `destroy`, and `import`.
- Support FeatBit Cloud and self-hosted FeatBit through a configurable API URL.
- Mark API tokens, environment secrets, and other credentials as Sensitive and prevent them from entering logs.
- Detect drift caused by UI or direct API changes.
- Clearly separate Terraform-owned attributes from UI-owned operational attributes so a later `apply` does not unexpectedly overwrite a production rollout.
- Provide ready-to-use CI/CD, preview-environment, and multi-environment examples.
- Optimize for the customer journeys that matter: bootstrap projects and environments, manage or import flags and segments, run changes from CI/CD with a service token, detect drift, and safely coexist with changes made in the FeatBit UI.

### 3.2 Out of scope for core v1.0

- Runtime feature flag evaluation, which belongs to FeatBit SDKs and OpenFeature providers.
- Deploying FeatBit itself, which should use Helm, Kubernetes, Azure, AWS, or other infrastructure providers.
- Dynamic data such as experiment analytics, event data, or continuous audit log streams.
- Matching LaunchDarkly's resource count or exposing LaunchDarkly-specific concepts that do not solve a FeatBit customer need.
- Terraform actions, functions, ephemeral resources, and list resources. The core use cases do not currently need these newer abstractions.

## 4. High-level architecture

```text
*.tf
  │
  ▼
Terraform Core
  │ Terraform Plugin Protocol v6 / gRPC
  ▼
terraform-provider-featbit
  ├─ provider schema and configuration
  ├─ resources and data sources
  ├─ expand/flatten and state migration
  └─ FeatBit API client wrapper
       │ HTTPS + Access Token
       ▼
FeatBit Open API /api/v1
```

### 4.1 Proposed repository structure

```text
.
├─ main.go
├─ go.mod
├─ go.sum
├─ terraform-registry-manifest.json
├─ internal/
│  ├─ provider/
│  │  ├─ provider.go
│  │  ├─ provider_test.go
│  │  ├─ project_resource.go
│  │  ├─ project_data_source.go
│  │  ├─ environment_resource.go
│  │  ├─ feature_flag_resource.go
│  │  ├─ segment_resource.go
│  │  └─ *_test.go
│  ├─ client/
│  │  ├─ client.go
│  │  ├─ auth.go
│  │  ├─ errors.go
│  │  ├─ retry.go
│  │  ├─ pagination.go
│  │  ├─ generated/
│  │  └─ openapi/
│  │     ├─ featbit.openapi.json
│  │     └─ overlay.yaml
│  ├─ models/
│  └─ validators/
├─ examples/
├─ docs/
├─ templates/
├─ tools/
├─ .github/workflows/
└─ .goreleaser.yml
```

### 4.2 API client strategy

Generate a typed API transport inside the provider repository and wrap it with a handwritten client:

1. Commit a fixed upstream OpenAPI snapshot so builds are reproducible.
2. Use a pinned Go OpenAPI generator for transport and API types. Never edit generated code manually.
3. Maintain a provider-owned, reviewable overlay for stable `operationId` values and generator fixes. The provider does not depend on upstream OpenAPI changes, and the overlay must not invent server behavior.
4. The handwritten wrapper is responsible for:
   - Access Token authentication
   - Base URL handling
   - Response envelopes
   - Terraform-oriented error classification
   - Retry and backoff
   - Pagination
   - Concurrency limits
   - User-Agent
   - Exact-identity lookup fallbacks
   - Read-after-write verification and normalization
5. Handwrite Terraform schemas and expand/flatten logic. Do not generate them directly from OpenAPI, because the HCL UX and Terraform Null, Unknown, and state semantics require deliberate design.
6. If FeatBit later publishes a separately versioned official Go Management API client, replace the internal generated client with that module.

## 5. FeatBit OpenAPI audit

Audited specification:

- URL: <https://app-api.featbit.co/swagger/OpenApi/swagger.json>
- OpenAPI version: `3.0.4`
- API version: `1.0`
- Snapshot SHA-256: `8DE202F939F6721748D66449C3DFE4EEE2E2BF369A57F121DF808907A44D11C4`
- Paths: 60
- Operations: 76
- Schemas: 112
- Schema properties: 454
- Authentication advertised by OpenAPI: JWT Bearer and Access Token
- Public provider authentication: personal or service API access token only

### 5.1 Exposed capabilities

| API tag | Operations | Terraform suitability |
|---|---:|---|
| Project | 5 | Complete list/create/read/update/delete; suitable for a resource and data source |
| Environment | 4 | Complete create/read/update/delete; no standalone list, but project responses include environments |
| FeatureFlag | 17 | CRUD plus archive/restore, toggle, variations, targeting, and tags; sufficient for the core resource |
| Segment | 14 | CRUD plus archive/restore, targeting, and tags; sufficient for the core resource |
| Group | 11 | CRUD plus member and policy bindings; suitable for phase 2 |
| Policy | 9 | Create/read/list/delete plus settings and statements updates; suitable for phase 2 |
| Member | 11 | Invite, read, delete, and bindings exist; managed creation is possible only if exact-email reconciliation is deterministic |
| Workspace | 4 | Singleton get/update, license, and OIDC; start with a data source |
| AuditLog | 1 | Read-only list; not a stable managed Terraform resource |

### 5.2 Core CRUD/API mapping

| Terraform object | Create | Read | Update | Delete | List/lookup | Assessment |
|---|---|---|---|---|---|---|
| Project | `POST /projects` | `GET /projects/{id}` | `PUT /projects/{id}` | `DELETE /projects/{id}` | `GET /projects` | Sufficient |
| Environment | `POST /projects/{projectId}/envs` | `GET /projects/{projectId}/envs/{id}` | `PUT .../{id}` | `DELETE .../{id}` | Environments in the project response | Sufficient |
| Feature Flag | `POST /envs/{envId}/feature-flags` | `GET .../{key}` | Specialized PUT endpoints plus PATCH | `DELETE .../{key}` | GET list | Sufficient, but create/read models are asymmetric |
| Segment | `POST /envs/{envId}/segments` | `GET .../{segmentId}` | Specialized PUT endpoints plus PATCH | `DELETE .../{segmentId}` | GET list/by-ids | Sufficient, but shared-scope update semantics are incomplete |
| Group | `POST /groups` | `GET /groups/{id}` | `PUT /groups/{id}` | `DELETE /groups/{id}` | GET list | Sufficient |
| Policy | `POST /policies` | `GET /policies/{id}` | PUT settings/statements | DELETE | GET list | Sufficient |
| Member | `POST /members/add` | `GET /members/{id}` | Bindings only | Delete from org/workspace | GET list | Use an exact-email reconciliation prototype; otherwise expose lookup/bindings and provision the member outside Terraform |

### 5.3 LaunchDarkly reference mapping, not a parity target

This table translates familiar concepts for design review only. It does not require identical schemas, lifecycle behavior, or release scope.

| LaunchDarkly | FeatBit design |
|---|---|
| `launchdarkly_project` | `featbit_project` |
| `launchdarkly_environment` | `featbit_environment` |
| Project-scoped `launchdarkly_feature_flag` | Environment-scoped `featbit_feature_flag` |
| Separate `launchdarkly_feature_flag_environment` | Do not copy directly; the FeatBit flag already includes environment state and targeting |
| `launchdarkly_segment` | `featbit_segment` |
| `custom_role` | `featbit_policy` |
| `team` / `team_member` | `featbit_group` / `featbit_member` |
| Team/member role mapping | Dedicated binding resources |
| SDK keys | Secrets returned by the environment API; always Sensitive |

Do not create two Terraform resources that both modify the same FeatBit flag object. That would create conflicting ownership and permanent diffs.

## 6. Working within the current API constraints

Provider v1 assumes that the current public REST API is fixed. No release milestone depends on a backend change. The provider must use documented public endpoints only; undocumented Portal endpoints, browser automation, direct database access, and a generic “raw REST request” Terraform resource are not acceptable workarounds.

Every capability must be assigned one of these support levels:

| Support level | Use when | Terraform behavior |
|---|---|---|
| Fully managed | CRUD and identity are deterministic | Resource, data source, Import, drift detection |
| Constrained managed | CRUD works but some updates are unsafe or ambiguous | Immutable fields use `RequiresReplace`; updates use only safe endpoints |
| Read/bind only | Lookup is deterministic but creation is not | Data source and relationship resources; object is created outside Terraform |
| External prerequisite | The API cannot represent the lifecycle safely | Document the UI/API bootstrap step and consume its stable ID or token |
| Omitted | No current customer need or no public API | Do not expose a placeholder merely for parity |

### 6.1 Provider-side error and OpenAPI compatibility layer

The OpenAPI document lists only `200`, `401`, and `403` responses and lacks `operationId`, `required`, enum, and validation metadata. Work around this inside the provider:

1. Pin an OpenAPI snapshot and apply a small local overlay for generator-only metadata such as stable operation IDs.
2. Handwrite the Terraform schema, validators, and canonicalization rules from verified server behavior and official product documentation.
3. Maintain a centralized error classifier using all available signals:
   - HTTP status
   - The FeatBit `{success,data,errors}` envelope
   - The operation and resource identity
4. For `Read`, remove an object from state only when:
   - The server returns an authoritative Not Found result; or
   - An exact parent-scoped list/lookup confirms that the ID or key is absent.
5. Never infer deletion from a fuzzy error string. If existence is uncertain, preserve state and return a diagnostic.
6. Treat `401` and `403` as authentication/authorization errors, never as absence.
7. Retry network failures, `429`, and transient `5xx` responses only for safe reads. Limit concurrency and use exponential backoff with jitter; honor `Retry-After` when present.
8. Test and record behavior separately for each supported FeatBit Cloud and self-hosted version. A version with incompatible behavior can be excluded from the compatibility matrix without blocking the whole provider.

Exact absence fallbacks are available for the core resources: project list by ID, environments from their parent project by ID, feature flag list by exact key, and segment list by exact ID. The handwritten client wrapper hides these fallbacks from resource implementations.

### 6.2 Concurrency, multi-call writes, and round-trip normalization

The lack of a uniform ETag/revision contract does not prevent safe support if ownership is narrow:

- Read immediately before an update and use `revision` wherever the endpoint accepts it.
- Call specialized endpoints only for attributes whose Terraform plan changed; avoid whole-object writes that overwrite unrelated UI changes.
- Serialize writes to the same remote object within one provider process.
- Make configured attributes Terraform-owned and allow operational attributes to be omitted or explicitly ignored.
- Read after every logical write and poll for bounded eventual consistency. Return success only when the intended values are observed.
- Do not automatically retry an ambiguous mutating request. First read the object to determine whether the requested change was already applied.
- Document unavoidable last-writer-wins behavior when Terraform and a human concurrently edit the same owned attribute.

Create operations that span multiple endpoints use a reconciliation workflow:

1. Perform an exact lookup to ensure the configured identity is absent.
2. Create the base object.
3. Capture or recover its stable identity through an exact lookup and set preliminary Terraform state before later sub-operations.
4. Apply variations, targeting, tags, statements, or other sub-resources in a deterministic order.
5. Read the complete object and write only the canonical server representation to Terraform state.
6. On partial failure, read the remote object and either resume safely on the next apply or perform a best-effort rollback when deletion is known to be safe. Acceptance-test whether Terraform persists the preliminary state after a Create diagnostic; otherwise include an exact `terraform import` recovery command and identity in the diagnostic.

Feature flag `enabledVariationId` versus `fallthrough`, server-generated variation IDs, default values, ordering, and JSON normalization belong in tested expand/flatten code. A backend transaction or combined create endpoint would be helpful but is not required.

### 6.3 Resource-specific fallback decisions

| Capability | v1 decision with the current API | Workaround |
|---|---|---|
| Projects | Fully managed | Exact ID lookup; auto-created environments are Computed |
| Environments | Fully managed | Parent project supplies an exact-list fallback; key and parent are `RequiresReplace` |
| Feature flags | Fully managed through reconciliation | Orchestrate specialized endpoints, preserve variation IDs, use revisions where available, then verify by Read |
| Segments | Constrained managed | `key`, `type`, and `scopes` are `RequiresReplace`; other changes use specialized endpoints and post-write verification |
| Groups and policies | Fully managed in phase 2 | Keep memberships and policy assignments in independent binding resources |
| Members | Read/bind first; managed creation is conditional | Resolve all pages and filter by normalized exact email. After `POST /members/add`, poll until exactly one matching member yields an ID. If this is not deterministic in every supported environment, keep member creation external and still support the member data source and bindings |
| Environment secrets | Metadata by default; values are opt-in | Expose the raw server-returned collection without guessing `server_key`/`client_key`. If values are exposed, use a separate Sensitive data source and warn that Terraform state is not a secret store |
| Workspace | Read-only by default | Add singleton updates only for a demonstrated customer workflow with deterministic round trips |
| Audit log and analytics | Not managed state | Keep outside core; consider narrowly scoped read-only data sources only when customers need them |

Member reconciliation must never adopt the first fuzzy search result. Before creation, fail with an Import instruction if exactly one matching member already exists; fail safely on zero-after-timeout or multiple matches. If invitations require a human acceptance step before an ID exists, a managed `featbit_member` resource is omitted and member provisioning remains an explicit prerequisite.

Sensitive only redacts Terraform output; it does not encrypt state. Environment secret values therefore require an explicit opt-in and documentation for encrypted remote state and restricted state access.

### 6.4 Deliberate scope boundaries instead of LaunchDarkly parity

The following are not v1 blockers:

| Capability absent from the current public API | Customer path |
|---|---|
| Access token/service-token management | Create the token once in FeatBit's Integrations page; pass it through `FEATBIT_ACCESS_TOKEN`. A provider cannot safely bootstrap its own credential |
| Environment secret rotation | Rotate outside Terraform and refresh dependent secret systems; do not model a fake lifecycle |
| Organization lifecycle | Treat the organization/workspace as account bootstrap and manage resources inside it |
| Webhooks, approval workflows, flag triggers, and scheduled changes | Continue to use the FeatBit UI or their existing supported integration until a stable public management API and customer demand exist |
| Experiments, metrics, metric groups, and audit streams | These are operational/analytical data rather than durable desired state; do not create managed resources |
| Relay proxy configuration, IP allowlists, destinations, context schemas, AI/model config, and views | Add only when FeatBit has a corresponding public capability and a validated customer workflow |

Avoid recommending `local-exec` plus `curl` as part of the provider design: Terraform cannot reliably Read, Import, detect drift, redact errors, or recover partial failures for such commands.

### 6.5 Optional future backend improvements

The following improvements would simplify the provider but are backlog items, not dependencies:

- Stable structured error codes and documented Not Found/conflict/rate-limit responses
- Stable `operationId`, required fields, enums, and constraints in OpenAPI
- Consistent revision or ETag handling
- `Idempotency-Key` for create operations
- Member creation returning the member ID
- Documented environment secret types and a rotation endpoint
- Complete create payloads for flags, segments, and policies
- A published API compatibility and deprecation policy

## 7. Public provider interface

### 7.1 Provider configuration

Proposed fields:

| Purpose | HCL field | Environment variable | Behavior |
|---|---|---|---|
| API URL | `api_url` | `FEATBIT_API_URL` | Required or defaulted to the FeatBit Cloud API; self-hosted users can override it |
| API Access Token | `access_token` | `FEATBIT_ACCESS_TOKEN` | Sensitive; accept personal or service tokens, with a service token recommended for CI/CD |
| HTTP timeout | `http_timeout_seconds` | `FEATBIT_HTTP_TIMEOUT_SECONDS` | Optional positive integer |
| Maximum concurrency | `max_concurrency` | `FEATBIT_MAX_CONCURRENCY` | Optional with a conservative default |
| Maximum retries | `max_retries` | `FEATBIT_MAX_RETRIES` | Used only for safely retryable requests |

The provider must:

- Validate the URL scheme and path.
- Reject an Unknown token during configuration.
- Never expose tokens in logs, diagnostic details, or User-Agent values.
- Send a `terraform-provider-featbit/<version>` User-Agent.
- Return a Terraform diagnostic when `success=false`, even if the HTTP status is 200.
- Retry only GET requests and explicitly idempotent operations.
- Respect context cancellation and timeouts.

Provider v1 supports API access tokens only and sends the token directly as the `Authorization` header value. It must not implement username/password login, JWT refresh, MFA, or SSO flows. The token determines the account context under the current public API contract; use provider aliases with different tokens when multiple contexts are required.

### 7.2 Core resources

| Terraform type | Identity/import format | Initial ownership boundary |
|---|---|---|
| `featbit_project` | `project_id` | name and key; key uses `RequiresReplace`; auto-created environments are Computed rather than an authoritative nested set |
| `featbit_environment` | `project_id/environment_id` | project_id, name, key, description, and require_change_comment; project_id/key use `RequiresReplace` |
| `featbit_feature_flag` | `environment_id/flag_key` | metadata, variation type, variations, enabled/off/fallthrough, targets, rules, tags, and archived state |
| `featbit_segment` | `environment_id/segment_id` | name, key, type, scopes, description, included, excluded, rules, tags, and archived state |

### 7.3 Core data sources

- `data.featbit_project`
- `data.featbit_environment`
- `data.featbit_feature_flag`
- `data.featbit_segment`
- `data.featbit_workspace`
- `data.featbit_environment_secrets` (explicit opt-in; all returned values are Sensitive and stored in Terraform state)

Single-object data sources must use unambiguous identity:

- Prefer ID.
- If key lookup is supported, require an exact match.
- Never use the first fuzzy search result.
- Validate `id` and `key` with `ExactlyOneOf`.

Defer list data sources until there is a clear HCL consumption use case. Avoid writing paginated, frequently changing collections into Terraform state unnecessarily.

### 7.4 Feature flag HCL model

Hide internal API UUIDs and rollout ranges from users:

- Users refer to variations by index.
- The provider converts indices to API variation IDs.
- Variation IDs generated by the provider or server remain stable in state.
- HCL rollouts use integer weights, such as a total of `100000`; the provider converts them to API `[start,end]` ranges.
- HCL conditions expose `values` as `list(string)`; the provider converts them to and from the API's current JSON-string representation.
- Rules and variations use List because order is meaningful.
- Tags, target user keys, and other unordered collections use Set.

Required validators:

- Variation type: `boolean|string|number|json`
- Variation value matches its type
- Minimum variation count, based on the actual backend constraint
- Enabled/off variation indices are in range
- Rollout weights are non-negative, match the variation count, and total correctly
- Rule names, conditions, and operators
- Flag key format

### 7.5 Terraform and UI attribute ownership

LaunchDarkly separates project-level flags from environment-level targeting. FeatBit flags are already environment-scoped, so that split cannot safely be copied.

Complete an ADR and prototype before implementation. Recommended sequence:

1. Use one `featbit_feature_flag` resource.
2. Make core metadata and variations explicitly Terraform-authoritative.
3. Treat `is_enabled` and `targeting` as optional operational configuration:
   - If configured, Terraform is authoritative.
   - If omitted, read the current value without correcting UI changes.
4. Verify that Plugin Framework Optional+Computed and plan modifier behavior does not create permanent diffs.
5. If "omitted means unmanaged" cannot be expressed reliably, use standard Terraform behavior and document:

```hcl
lifecycle {
  ignore_changes = [
    is_enabled,
    targeting,
  ]
}
```

Do not publish a `featbit_feature_flag_targeting` resource that modifies the same remote object as `featbit_feature_flag`, unless the backend first provides an independent identity and independent read/delete semantics.

### 7.6 Destroy, archive, and import

- By default, `Delete` should call the actual DELETE endpoint to match Terraform destroy semantics.
- An optional `archive_flags_on_destroy` / `archive_segments_on_destroy` provider setting can follow LaunchDarkly's approach, but documentation must prominently warn that:
  - Archived keys remain reserved.
  - A later apply may require Import or Restore.
  - Archive does not mean the object was deleted.
- When Read returns Not Found, remove the object from state.
- Deleting an already missing object is successful.
- The first plan after Import must be empty.
- Import ID formats become compatibility contracts once published.

### 7.7 Phase 2 IAM resources

Proposed types:

- `featbit_group`
- `featbit_policy`
- `featbit_group_member`
- `featbit_group_policy`
- `featbit_member_policy`
- `data.featbit_group`
- `data.featbit_policy`
- `data.featbit_member`

Model each relationship as one binding resource with a composite ID. This prevents groups, members, and policies from all claiming authoritative ownership over the same complete relationship set.

Start with the member data source and binding resources. Add a managed `featbit_member` only if the exact-email reconciliation workflow in section 6.3 passes Cloud and self-hosted acceptance tests. Email is immutable/`RequiresReplace`, and any initial password is Sensitive.

## 8. Phased work packages

### Phase 0: Empirical API compatibility and ADRs (3–5 person-days)

- [ ] Pin the OpenAPI snapshot and SHA-256.
- [ ] Define provider-owned stable `operationId` values for core operations in the local overlay.
- [ ] Build an observed-behavior matrix for 400/404/409/429/5xx and `success=false` responses.
- [ ] Prototype exact-absence fallbacks for projects, environments, flags, and segments.
- [ ] Verify personal and service access tokens and confirm that no login flow or extra context headers are required.
- [ ] Confirm the number and keys of environments created automatically with a project.
- [ ] Confirm whether project/environment deletion cascades, is blocked, or is asynchronous.
- [ ] Confirm environment secret types and sensitive-field behavior.
- [ ] Prototype a multi-call complex flag create, canonical Read, and partial-failure recovery.
- [ ] Classify each candidate capability as fully managed, constrained, read/bind only, external prerequisite, or omitted.
- [ ] ADR-001: Flag ownership between Terraform and the UI.
- [ ] ADR-002: Delete versus archive.
- [ ] ADR-003: Generated OpenAPI client and overlay.
- [ ] ADR-004: Import ID formats.
- [ ] ADR-005: Supported Go, Framework, and Terraform CLI matrix.

Gate: Core behavior can be encoded in deterministic tests. Any ambiguous operation is narrowed, made replace-only, or removed from v1 rather than becoming a backend dependency.

### Phase 1: Repository and provider scaffold (4–6 person-days)

- [ ] Initialize from the HashiCorp Framework scaffold.
- [ ] Set the Go module, provider address, Protocol v6, and version injection.
- [ ] Add MPL-2.0. If FeatBit requires MIT, confirm the licensing approach before copying scaffold code.
- [ ] Implement provider schema, environment variables, and Configure.
- [ ] Implement HTTP client authentication, errors, retry, pagination, and concurrency.
- [ ] Generate the OpenAPI client and create a deterministic `make generate`.
- [ ] Add `terraform-plugin-log` and verify that sensitive values never enter logs.
- [ ] Add provider schema and configuration unit tests.
- [ ] Add GNUmakefile targets: `fmt`, `lint`, `generate`, `test`, `testacc`, and `build`.

Gate: A local developer override works, `terraform providers schema -json` succeeds, and mock API configuration tests pass.

### Phase 2: Project and environment (5–8 person-days)

- [ ] Implement `featbit_project` resource CRUD and Import.
- [ ] Implement `featbit_project` data source.
- [ ] Implement `featbit_environment` resource CRUD and Import.
- [ ] Implement `featbit_environment` data source.
- [ ] Apply `RequiresReplace` to key and parent identity fields.
- [ ] Expose project auto-created environments as Computed without conflicting with standalone environment resources.
- [ ] Normalize environment setting defaults.
- [ ] Keep environment secret metadata Computed; add an opt-in Sensitive secrets data source only after documenting state-security implications.
- [ ] Add acceptance tests for create, update, drift, import, out-of-band deletion, and destroy.

Gate: A second apply produces an empty plan, Import produces an empty plan, and refresh removes externally deleted objects from state.

### Phase 3: Feature flags (10–15 person-days)

- [ ] Define schemas for variations, fallthrough, targets, conditions, and rules.
- [ ] Implement variation index to API UUID mapping.
- [ ] Implement rollout weight to API range conversion with round-trip tests.
- [ ] After Create, call variations, targeting, and tags endpoints as needed.
- [ ] During Update, call only the specialized endpoints for changed attributes so unrelated UI-owned targeting is not overwritten.
- [ ] Use revisions where accepted, serialize same-object writes, and verify every logical write with a canonical Read.
- [ ] Support enabled/off variations and archive/restore.
- [ ] Support boolean, string, number, and JSON variations.
- [ ] Implement optional operational ownership or document the `ignore_changes` fallback.
- [ ] Import with `environment_id/flag_key`.
- [ ] Cover every variation type, weighted rollout, targets, rules, tags, archive, drift, and revision conflict in acceptance tests.

Gate: Creating and reading a complex flag produces no diff; changing one field does not rewrite unrelated targeting; UI-owned mode is not overwritten by apply.

### Phase 4: Segments (6–10 person-days)

- [ ] Implement `featbit_segment` resource CRUD and Import.
- [ ] Implement `featbit_segment` data source.
- [ ] Use Set for included and excluded users.
- [ ] Preserve rule order and round-trip conditions.
- [ ] Support tags and archive/restore.
- [ ] Apply `RequiresReplace` to key, type, and scopes in v1.
- [ ] Validate environment-specific/shared values and scopes.
- [ ] Return a useful diagnostic when deleting a segment referenced by a flag.
- [ ] Cover included/excluded users, rules, shared/environment-specific types, flag references, and out-of-band drift in acceptance tests.

Gate: The complete segment lifecycle and Import have no permanent diff, and reference conflicts do not leave damaged state.

### Phase 5: IAM (10–15 person-days; can follow core work)

- [ ] Implement `featbit_group` resource and data source.
- [ ] Implement `featbit_policy` resource and data source.
- [ ] Define policy statement schemas and effect/action/resource validation.
- [ ] Implement group-member, group-policy, and member-policy binding resources.
- [ ] Implement composite Import IDs.
- [ ] Prototype exact-email member reconciliation across all pages and both supported deployment types.
- [ ] Publish `featbit_member` only if that prototype is deterministic; otherwise document external member provisioning and ship the member data source/bindings.
- [ ] Convert 403 responses into diagnostics that identify the operation and resource without leaking credentials.

Gate: Binding applies are idempotent, and pagination, ordering, or relationships added by other administrators do not cause unintended deletion.

### Phase 6: Documentation, compatibility, and release (5–8 person-days)

- [ ] Generate Registry documentation with `tfplugindocs v0.25.0`.
- [ ] Provide an example and import example for every resource and data source.
- [ ] Write guides for:
  - Authentication and self-hosted FeatBit
  - CI/CD
  - Preview environments
  - Terraform versus UI ownership
  - Importing existing resources
  - External prerequisites and intentionally unsupported capabilities
  - Sensitive environment values and Terraform state security
  - Upgrades and migrations
- [ ] Add README, CONTRIBUTING, SECURITY, and CHANGELOG.
- [ ] Configure GoReleaser with CGO disabled, trimpath, multiple OS/architectures, SHA256SUMS, and GPG signatures.
- [ ] Commit `terraform-registry-manifest.json` with Protocol `6.0`.
- [ ] Publish a prerelease and run smoke tests on Windows, Linux, and macOS.
- [ ] Publish to the Terraform Registry and verify `terraform init`.

Gate: A clean directory can use only the Registry address to init, plan, apply, and import; release assets and signatures meet Registry requirements.

## 9. Test plan

### 9.1 Unit and contract tests

- Provider configuration: HCL, environment variables, Unknown/Null, and invalid URLs.
- API client: headers, base path, timeout, cancellation, response envelope, and pagination.
- Error mapping: HTTP 200 with `success=false`, plus observed 401, 403, 404, 409, 429, and 5xx behavior.
- Existence classification: authoritative Not Found, exact-list absence fallback, ambiguous failure that preserves state, and pagination.
- Retry: only safe requests, honoring `Retry-After`, with exponential backoff and jitter.
- Expand/flatten: bidirectional round trips for every model.
- Validators: keys, UUIDs, variation types/values, operators, and rollouts.
- State: Set/List ordering, computed IDs, and sensitive values.
- OpenAPI snapshot: generation must produce a clean diff; breaking changes fail CI.

### 9.2 Acceptance tests

Use `terraform-plugin-testing v1.16.0` with `TF_ACC=1`:

- Basic create/read/update/delete.
- ImportState verification.
- Empty plan after Import.
- Empty plan after a second apply.
- Expected drift after an out-of-band update.
- State removal after out-of-band deletion.
- Successful deletion of an already missing object.
- Replacement plan for immutable fields.
- API and revision conflicts.
- Paginated lookup.
- Rate-limit behavior during concurrent refresh.
- Sensitive values absent from CLI output and logs.

Use randomized prefixes for all test resources and register sweepers. Tests must clean up even after failure.

### 9.3 Test environments

Priority:

1. Pull requests: unit plus mock/contract tests.
2. Trusted pull requests or branches: start a pinned self-hosted FeatBit stack and run core acceptance tests.
3. Nightly: full acceptance suite plus Terraform/Go matrices.
4. Release candidate: FeatBit Cloud test tenant plus the current stable self-hosted release.

Never expose a long-lived privileged token to fork pull requests. GitHub OIDC or secret environments should be limited to trusted workflows, and test tokens should be restricted to a workspace/organization and resource-name prefix.

### 9.4 Compatibility matrix

- Terraform CLI: minimum supported version for the Protocol v6 baseline plus current stable `1.15.x`.
- Go CI: `1.25.12` and `1.26.5`.
- Build/smoke OS: Linux, Windows, and macOS.
- Architectures: amd64 and arm64, plus other Registry-recommended scaffold outputs.
- FeatBit: current Cloud API and current stable self-hosted release.

## 10. CI/CD and supply chain

Every pull request must pass:

- `go fmt` / `gofmt`
- `go vet`
- `golangci-lint` with a pinned version
- `go test -race ./...`
- Coverage threshold
- `govulncheck`
- Clean generated documentation/client diff
- OpenAPI breaking-change check
- Terraform example `fmt` and `validate`
- Trusted acceptance tests

Release requirements:

- SemVer tag such as `v0.1.0` or `v1.0.0`.
- GoReleaser v2.
- `CGO_ENABLED=0`.
- Reproducible `-trimpath` builds.
- One zip per OS/architecture.
- SHA-256 checksums.
- GPG detached signature.
- SBOM and provenance, provided they do not break Registry asset naming.
- Never replace a published tag or release asset; publish fixes as a new version.

## 11. Milestones and estimated effort

| Milestone | Scope | Estimate |
|---|---|---:|
| M0 | Compatibility probes, ADRs, scaffold, and client | 1.5–2 weeks |
| M1 / `v0.1.0-alpha` | Project and Environment | 1–1.5 weeks |
| M2 / `v0.2.0-beta` | Feature Flag | 2–3 weeks |
| M3 / `v0.3.0-beta` | Segment and core documentation | 1.5–2 weeks |
| M4 / `v1.0.0` | Hardening, compatibility, and Registry GA | 1.5–2 weeks |
| M5 | Group, Policy, bindings, member lookup, and conditional member creation | 2–3 weeks |

For one engineer experienced with Go and Terraform:

- Core beta: approximately 6–8 person-weeks.
- Core GA: approximately 8–10 person-weeks.
- IAM functionality suitable for Terraform in the current OpenAPI surface: an additional 2–3 person-weeks.

Backend improvements can proceed independently, but provider GA does not wait for them. If a capability cannot meet deterministic lifecycle tests with the current API, reduce that capability's ownership boundary or omit it while keeping the customer workflow available through lookup, Import, bindings, or a documented external prerequisite.

## 12. Release criteria

### Core beta

- Four core resources and corresponding data sources are available.
- CRUD, Import, drift, and out-of-band deletion have acceptance coverage.
- A second plan for complex feature flags and segments is empty.
- Cloud and self-hosted authentication documentation is complete.
- The observed API behavior matrix is versioned, and Not Found detection uses an authoritative response or exact-identity fallback rather than fuzzy string matching.

### v1.0 GA

- Every public schema, Import ID, and default has passed compatibility review.
- There are no known permanent diffs.
- The provider cannot silently overwrite production targeting that Terraform does not own.
- Sensitive values never appear in provider logs.
- OpenAPI breaking changes are detected automatically.
- Release assets, signatures, and Registry documentation are complete.
- At least one prerelease has been validated by a real CI/CD user.
- Upgrade, deprecation, and state migration policies exist.

## 13. Proposed first GitHub issues

1. `provider: capture observed API error and existence behavior`
2. `provider: add OpenAPI overlay and reproducible client generation`
3. `provider: implement exact-identity lookup and error classification`
4. `provider: prototype multi-call flag reconciliation and recovery`
5. `provider: define the supported capability tiers and compatibility matrix`
6. `provider: scaffold Plugin Framework v1.19 Protocol 6`
7. `provider: implement API access-token authentication`
8. `provider: implement generated OpenAPI client wrapper`
9. `provider: implement project resource/data source/import`
10. `provider: implement environment resource/data source/import`
11. `provider: ADR and prototype for flag UI/Terraform ownership`
12. `provider: implement feature flag resource/data source/import`
13. `provider: implement segment resource/data source/import`
14. `provider: acceptance test environment and sweepers`
15. `provider: Registry docs and signed release pipeline`

## 14. Primary references

- [Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework)
- [Provider servers and Protocol v6](https://developer.hashicorp.com/terraform/plugin/framework/provider-servers)
- [HashiCorp-supported provider languages and libraries](https://developer.hashicorp.com/terraform/plugin/best-practices/provider-code)
- [Terraform Plugin Framework v1.19.0](https://github.com/hashicorp/terraform-plugin-framework/releases/tag/v1.19.0)
- [Terraform Plugin Testing v1.16.0](https://github.com/hashicorp/terraform-plugin-testing/releases/tag/v1.16.0)
- [HashiCorp provider scaffold](https://github.com/hashicorp/terraform-provider-scaffolding-framework)
- [Terraform Registry provider publishing requirements](https://developer.hashicorp.com/terraform/registry/providers/publishing)
- [Terraform Registry provider documentation requirements](https://developer.hashicorp.com/terraform/registry/providers/docs)
- [LaunchDarkly Terraform guide](https://launchdarkly.com/docs/guides/infrastructure/terraform)
- [LaunchDarkly Terraform provider](https://github.com/launchdarkly/terraform-provider-launchdarkly)
- [FeatBit API access tokens](https://docs.featbit.co/integrations/api-access-tokens)
- [Using the FeatBit REST API](https://docs.featbit.co/api-docs/using-featbit-rest-api)
- [FeatBit OpenAPI](https://app-api.featbit.co/swagger/OpenApi/swagger.json)
