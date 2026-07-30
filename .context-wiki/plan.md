# FeatBit Terraform Provider Implementation Plan

> Status: Draft (English version)  
> Research baseline: July 30, 2026  
> Target repository: `github.com/featbit/terraform-provider-featbit`  
> Intended Registry address: `registry.terraform.io/featbit/featbit`

## 1. Conclusions and implementation principles

1. Use Go and HashiCorp's currently recommended **Terraform Plugin Framework**. Do not start a new provider on the legacy `terraform-plugin-sdk/v2` architecture.
2. Use **Terraform Plugin Protocol v6** and set the Registry manifest protocol to `["6.0"]`.
3. Follow GitOps and infrastructure-as-code principles: declarative desired state, version-controlled and reviewable changes, automated application, drift detection, and repeatable reconciliation. Follow HashiCorp's official conventions for provider schemas, CRUD behavior, imports, data sources, tests, documentation, and releases.
4. Model Terraform resources around FeatBit's own API identities and lifecycle. A FeatBit feature flag is already scoped to an environment, so it should not be split into multiple Terraform resources that compete to manage the same remote object.
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
   - The member resource requires an API improvement first
7. The current OpenAPI surface is sufficient to begin the core provider. Stable error semantics, stronger OpenAPI constraints, and concurrency contracts must be addressed before GA.
8. The provider enables GitOps-aligned workflows but is not itself a GitOps controller. Repository policy, pull-request review, automated `plan`/`apply`, and periodic reconciliation are supplied by the customer's CI/CD or GitOps platform.
9. Use a clean-room implementation process: derive behavior and schemas from FeatBit's own product model, OpenAPI contract, and observed API behavior, while using only HashiCorp's official provider guidance and scaffold for Terraform mechanics. Do not copy source code, schema wording, documentation, examples, or tests from third-party providers.

## 2. July 2026 technical baseline

### 2.1 Verified versions

| Component | Planned version | Rationale |
|---|---:|---|
| Go module language baseline | `go 1.25.8` | Matches the official Framework scaffold from July 27, 2026 |
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

### 2.2 Standards and official implementation references

- HashiCorp official scaffold:
  [`terraform-provider-scaffolding-framework@f781750`](https://github.com/hashicorp/terraform-provider-scaffolding-framework/tree/f781750309d9d63c50f9d6992a788fa7245ec7fc)
- OpenGitOps principles:
  [declarative, versioned and immutable, pulled automatically, and continuously reconciled](https://opengitops.dev/)
- HashiCorp's official Plugin Framework, testing, documentation, and Registry guidance are the implementation authorities for provider behavior.

## 3. Product goals and non-goals

### 3.1 Core provider goals

- Let users declare the desired state of FeatBit projects, environments, flags, and segments in HCL.
- Support `plan`, `apply`, `refresh`, `destroy`, and `import`.
- Support FeatBit Cloud and self-hosted FeatBit through a configurable API URL.
- Mark API tokens, environment secrets, and other credentials as Sensitive and prevent them from entering logs.
- Detect drift caused by UI or direct API changes.
- Clearly separate Terraform-owned attributes from UI-owned operational attributes so a later `apply` does not unexpectedly overwrite a production rollout.
- Provide ready-to-use CI/CD, preview-environment, and multi-environment examples.

### 3.2 Out of scope for core v1.0

- Runtime feature flag evaluation, which belongs to FeatBit SDKs and OpenFeature providers.
- Deploying FeatBit itself, which should use Helm, Kubernetes, Azure, AWS, or other infrastructure providers.
- Dynamic data such as experiment analytics, event data, or continuous audit log streams.
- Resources for which FeatBit exposes no corresponding product capability or stable management API.
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
3. Until the upstream specification is corrected, use a reviewable overlay to add stable `operationId` values. Do not use the overlay to invent server behavior.
4. The handwritten wrapper is responsible for:
   - Access Token authentication
   - Base URL handling
   - Response envelopes
   - Terraform-oriented error classification
   - Retry and backoff
   - Pagination
   - Concurrency limits
   - User-Agent
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
- Authentication: JWT Bearer and Access Token

### 5.1 Exposed capabilities

| API tag | Operations | Terraform suitability |
|---|---:|---|
| Project | 5 | Complete list/create/read/update/delete; suitable for a resource and data source |
| Environment | 4 | Complete create/read/update/delete; no standalone list, but project responses include environments |
| FeatureFlag | 17 | CRUD plus archive/restore, toggle, variations, targeting, and tags; sufficient for the core resource |
| Segment | 14 | CRUD plus archive/restore, targeting, and tags; sufficient for the core resource |
| Group | 11 | CRUD plus member and policy bindings; suitable for phase 2 |
| Policy | 9 | Create/read/list/delete plus settings and statements updates; suitable for phase 2 |
| Member | 11 | Invite, read, delete, and bindings exist, but creation returns only a Boolean; the resource is currently blocked |
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
| Member | `POST /members/add` | `GET /members/{id}` | Bindings only | Delete from org/workspace | GET list | Cannot reliably obtain the remote ID after creation |

### 5.3 FeatBit resource model and GitOps ownership boundaries

| FeatBit API object | Terraform type | Ownership rule |
|---|---|---|
| Project | `featbit_project` | Terraform owns project metadata; automatically created environments remain computed unless imported separately |
| Environment | `featbit_environment` | One resource owns one environment and its stable settings |
| Environment-scoped feature flag | `featbit_feature_flag` | One resource owns one remote flag; operational fields can be explicitly managed or intentionally left to the UI |
| Segment | `featbit_segment` | One resource owns metadata and, when configured, targeting rules |
| Policy | `featbit_policy` | Terraform owns the policy document and stable metadata |
| Group/member/policy relationship | Dedicated binding resource | One binding resource owns one relationship instead of multiple authoritative collections |
| Environment SDK secrets | Computed Sensitive attributes | Values are observed and exported carefully, never treated as ordinary configuration |

The Git repository is the desired-state source of truth for every attribute declared in HCL. Do not create two Terraform resources that modify the same FeatBit object, because competing reconcilers create ambiguous ownership and permanent diffs.

## 6. API gaps and required contract improvements

### 6.1 Core beta can begin, but these items must be confirmed or fixed before GA

#### API-01: No stable error and Not Found contract

All 76 operations currently document only:

- `200`
- `401`
- `403`

The specification does not describe:

- `400` validation errors
- `404` resource not found
- `409` duplicate key or revision conflict
- `429` rate limiting
- `5xx` transient/server errors

A Terraform resource `Read` operation must reliably distinguish:

- The object was deleted externally: remove it from state.
- A transient error occurred: preserve state and return an error.
- Permission was denied: return a clear diagnostic.
- A revision conflict occurred: refresh and require a retry.

Acceptance requirement: every resource family must return consistent HTTP statuses or stable machine-readable error codes, and those responses must be included in OpenAPI.

#### API-02: OpenAPI lacks metadata required for stable generation and validation

Audit results:

- 76 of 76 operations have no `operationId`.
- 112 of 112 schemas have no `required` list.
- None of the 454 properties define an `enum`, `pattern`, length, numeric range, or item-count constraint.

At minimum, add:

- Stable, unique `operationId` values
- Required fields for create and update payloads
- Key pattern, length, and uniqueness rules
- `variationType` values
- Segment `type` values
- Condition operators
- Policy effect, resource type, and action values
- OIDC client authentication methods
- Rollout ranges and sum constraints

The provider can temporarily use an overlay, but the upstream OpenAPI document should become the long-term source of truth.

#### API-03: Concurrency semantics are not consistently defined

Feature flag responses contain a `revision`, and some update payloads require it. The name, description, tags, and toggle endpoints do not use revisions consistently. Segments also lack a consistent revision or ETag model.

Clarify:

- Which updates use optimistic concurrency
- Which status or error code represents an old revision
- Whether the provider should refresh and retry automatically
- Whether an approval or pending change can cause `apply` to return success before the state has actually changed

Until this is defined, the provider must not silently overwrite a targeting change made moments earlier in the UI.

#### API-04: Create and Read models must round-trip cleanly

Feature flags:

- Create accepts `enabledVariationId`.
- Read returns `fallthrough` and has no same-named `enabledVariationId`.
- Full targeting requires an additional API call after creation.

Segments:

- Create does not accept included users, excluded users, rules, or tags.
- A complete object requires targeting and tag calls after creation.

Policies:

- Statements must be set after creation.

Requirements:

- Document field mappings and server defaults.
- A create followed immediately by a read must produce normalized state with no Terraform diff.
- Prefer complete create payloads or provide transaction and idempotency guarantees.

### 6.2 Gaps that block specific later resources

#### API-05: Member creation does not return the member ID

`POST /api/v1/members/add` returns `BooleanApiResponse`. Terraform creation requires a stable remote ID.

Recommended changes:

- Return `MemberVmApiResponse` or at least the member ID.
- Add an exact-email lookup endpoint.
- Document invitation and asynchronous states.

Until this API is fixed:

- A `featbit_member` data source is possible.
- Do not publish a `featbit_member` managed resource.
- Do not guess a newly created member's ID from the first fuzzy `SearchText` result.

#### API-06: Environment secret contract is incomplete

Environment responses contain `secrets[]` with `id`, `name`, `type`, and `value`, but OpenAPI does not define:

- Stable secret type/name values
- Which secret is the server or client key
- Whether every read returns the value
- A rotation API
- Delete/recreate behavior

Until clarified, the MVP should either:

- Expose the entire `secrets` collection as Computed and Sensitive; or
- Expose only secret metadata and omit values.

Stable convenience attributes such as `server_key` and `client_key` require a stable API contract first.

#### API-07: Shared segment update semantics are incomplete

Create supports:

- `type = environment-specific | shared`
- `scopes`

The specialized update APIs do not explain whether type or scopes can change. The generic PATCH documentation does not include examples for these fields.

Initially mark `key`, `type`, and `scopes` as `RequiresReplace`, unless the API team confirms and documents stable update behavior.

### 6.3 APIs needed for broader declarative GitOps coverage

To manage more of the FeatBit control plane declaratively, the OpenAPI document would need management endpoints for:

- Access tokens or service accounts
- Webhooks
- Audit log subscriptions
- Experiments, metrics, or metric groups
- Approval request workflows
- Flag triggers or scheduled changes
- Relay proxy configuration
- Environment secret rotation
- IP allowlists
- Integration destinations/delivery configuration
- Organization lifecycle
- Context kinds or schemas

Do not add resources merely to expand the catalog. Add a Terraform resource only when FeatBit has a corresponding product concept, a stable lifecycle, and a management API suitable for reconciliation.

### 6.4 Recommended API stability improvements

- Support `Idempotency-Key` on create endpoints.
- Document rate-limit headers and `Retry-After`.
- Define an API compatibility and deprecation policy.
- Return structured errors with `code`, `message`, and `field`, rather than only `string[]`.
- Add `servers` or clearly document base URL composition.
- Clarify whether an Access Token fixes the organization/workspace context and whether header overrides are permitted.

## 7. Public provider interface

### 7.1 Provider configuration

Proposed fields:

| Purpose | HCL field | Environment variable | Behavior |
|---|---|---|---|
| API URL | `api_url` | `FEATBIT_API_URL` | Required or defaulted to the FeatBit Cloud API; self-hosted users can override it |
| Access Token | `access_token` | `FEATBIT_ACCESS_TOKEN` | Sensitive; the recommended machine identity for v1 |
| Organization | `organization_id` | `FEATBIT_ORGANIZATION_ID` | Optional; send only when the token does not determine context |
| Workspace | `workspace_id` | `FEATBIT_WORKSPACE_ID` | Optional; same rule as organization |
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

JWT can be added later as an explicit authentication mode, but CI/CD should not depend on short-lived user JWTs.

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

GitOps requires a clear source of truth. When both Terraform and the FeatBit UI modify the same attribute, they become competing writers. Because a FeatBit flag is already environment-scoped, splitting one remote flag across multiple Terraform resources would make that ownership less clear rather than more reliable.

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
- The provider may offer optional `archive_flags_on_destroy` / `archive_segments_on_destroy` settings, but documentation must prominently warn that:
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

Implement the `featbit_member` managed resource only after API-05 is fixed. Email should be immutable/`RequiresReplace`, and any initial password must be Sensitive.

## 8. Phased work packages

### Phase 0: API contracts and ADRs (3–5 person-days)

- [ ] Pin the OpenAPI snapshot and SHA-256.
- [ ] Define stable `operationId` values for core operations.
- [ ] Create backend issues for API-01 through API-07.
- [ ] Test actual 400/404/409/429/5xx behavior.
- [ ] Confirm Access Token, Organization, and Workspace header behavior.
- [ ] Confirm the number and keys of environments created automatically with a project.
- [ ] Confirm whether project/environment deletion cascades, is blocked, or is asynchronous.
- [ ] Confirm environment secret types and sensitive-field behavior.
- [ ] ADR-001: Flag ownership between Terraform and the UI.
- [ ] ADR-002: Delete versus archive.
- [ ] ADR-003: Generated OpenAPI client and overlay.
- [ ] ADR-004: Import ID formats.
- [ ] ADR-005: Supported Go, Framework, and Terraform CLI matrix.
- [ ] Record the clean-room design basis and license/attribution inventory; verify that public schema text, examples, and tests are original to FeatBit.

Gate: Core API create/read/update/delete and Not Found behavior can be encoded in deterministic acceptance tests.

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
- [ ] Mark all secrets Sensitive; do not publish unstable convenience fields before the API contract is defined.
- [ ] Add acceptance tests for create, update, drift, import, out-of-band deletion, and destroy.

Gate: A second apply produces an empty plan, Import produces an empty plan, and refresh removes externally deleted objects from state.

### Phase 3: Feature flags (10–15 person-days)

- [ ] Define schemas for variations, fallthrough, targets, conditions, and rules.
- [ ] Implement variation index to API UUID mapping.
- [ ] Implement rollout weight to API range conversion with round-trip tests.
- [ ] After Create, call variations, targeting, and tags endpoints as needed.
- [ ] During Update, call only the specialized endpoints for changed attributes so unrelated UI-owned targeting is not overwritten.
- [ ] Use revisions for optimistic concurrency.
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
- [ ] Until API-07 is resolved, apply `RequiresReplace` to key, type, and scopes.
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
- [ ] Implement `featbit_member` after API-05 is fixed.
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
- Error mapping: HTTP 200 with `success=false`, plus 401, 403, 404, 409, 429, and 5xx.
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
| M0 | API contracts, ADRs, scaffold, and client | 1.5–2 weeks |
| M1 / `v0.1.0-alpha` | Project and Environment | 1–1.5 weeks |
| M2 / `v0.2.0-beta` | Feature Flag | 2–3 weeks |
| M3 / `v0.3.0-beta` | Segment and core documentation | 1.5–2 weeks |
| M4 / `v1.0.0` | Hardening, compatibility, and Registry GA | 1.5–2 weeks |
| M5 | Group, Policy, bindings, and Member | 2–3 weeks; depends on API-05 |

For one engineer experienced with Go and Terraform:

- Core beta: approximately 6–8 person-weeks.
- Core GA: approximately 8–10 person-weeks.
- IAM functionality suitable for Terraform in the current OpenAPI surface: an additional 2–3 person-weeks.

The API team should address API-01 through API-07 in parallel with provider implementation. Otherwise, feature flag/member round-trip and lifecycle stability will delay GA.

## 12. Release criteria

### Core beta

- Four core resources and corresponding data sources are available.
- CRUD, Import, drift, and out-of-band deletion have acceptance coverage.
- A second plan for complex feature flags and segments is empty.
- Cloud and self-hosted authentication documentation is complete.
- The API contract is fixed and Not Found detection does not depend on fuzzy string matching.

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

1. `api: define error/status contract for Terraform lifecycle`
2. `api: add operationId, required fields, enums and validation constraints`
3. `api: document revision and optimistic concurrency semantics`
4. `api: make feature flag create/read models round-trip`
5. `api: return member ID from member creation`
6. `api: document environment secret types and rotation`
7. `provider: scaffold Plugin Framework v1.19 Protocol 6`
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
- [OpenGitOps principles](https://opengitops.dev/)
- [FeatBit OpenAPI](https://app-api.featbit.co/swagger/OpenApi/swagger.json)
