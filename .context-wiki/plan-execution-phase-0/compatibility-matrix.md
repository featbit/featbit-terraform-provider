# FeatBit API Compatibility Matrix

Status: **Complete for every available target — Cloud verified; self-hosted unavailable/Not tested**
Last updated: **2026-07-31**

Do not mark a cell supported without linked evidence. Use `Not tested`, `Supported`, `Constrained`, `Unsupported`, or `Not applicable`.

## Deployment targets

| Target ID | Deployment | FeatBit version/build | API URL class | Test date | Status | Evidence |
|---|---|---|---|---|---|---|
| cloud-current | Current FeatBit Cloud; disposable `tfp0-` parents only; all approved child scopes completed and closed | Current at test time; build unavailable | Exact documented Cloud host over HTTPS | 2026-07-31 | Authenticated read, parent lifecycle, child missing reads, all four constrained flag types, and one constrained environment-specific segment path supported with exact-match constraints | [Authentication](evidence/20260731-cloud-auth.md), [project/environment](evidence/20260731-cloud-project-environment.md), [feature flags](evidence/20260731-cloud-feature-flags.md), [segment](evidence/20260731-cloud-segments.md) |
| selfhosted-min | Disposable self-hosted minimum supported release | Not selected | Loopback/private target only | 2026-07-31 owner clarification | Not available — no target exists and none can be provisioned for Phase 0; no credential or configuration action is pending | [Session log](session-log.md) |

The target IDs and mutation URL classes are approved for Phase 0. This is a
safety-scope decision, not a claim that either deployment has passed a probe.
The offline probe contract is recorded separately in
[the safety evidence](evidence/20260731-offline-contracts.md).

Never record private tenant names, tokens, or full private URLs.

## High-level behavior

| Capability | cloud-current | selfhosted-min | Notes/evidence |
|---|---|---|---|
| Permission-scoped access token | Supported | Not tested | “Personal” and “service” use the same provider transport contract: direct `Authorization`; [evidence](evidence/20260731-cloud-auth.md) |
| Token-selected context | Supported without explicit organization/workspace headers | Not tested | [Authenticated read](evidence/20260731-cloud-auth.md) |
| Project lifecycle | Supported with constraints | Not tested | Create/Read/name Update/Delete converge; key replace-only; duplicate Create is structured 422; Delete cascades automatic environments; direct post-delete Read requires exact fallback |
| Environment lifecycle | Supported with constraints | Not tested | Create/Read/name-description Update/Delete converge; parent/key replace-only; duplicate Create is structured 422; direct post-delete Read requires exact fallback |
| Feature flag lifecycle | Supported with constraints for Boolean, String, Number, and JSON | Not tested | All four documented types completed Create/exact Read/narrow name Update/archive-plus-hard-Delete/direct 404/two-view exact zero; type/description/variations replace, and targeting/rollout remains UI-owned |
| Environment-specific segment lifecycle | Supported with constraints | Not tested | Resource-name scope must resolve uniquely from exact public Reads; complex specialized updates and archive/restore converge; active Delete requires archive then hard Delete; exact absence scans active and archived views |
| Shared segment lifecycle | Read/bind only | Not tested | Exact Reads exposed resource-name scopes, but cross-environment/project/organization Create/Update/Delete was excluded; never adopt or mutate a visible pre-existing row |
| Member exact-email reconciliation | Deferred from core v1; offline zero/one/duplicate/fuzzy-first contract only | Not tested | Managed creation is external; member lookup/bindings require later IAM-stage target evidence |
| Environment secret metadata | Supported with state constraints | Not tested | Two Client/Server objects observed per environment; values must be discarded from ordinary state |

## Error and existence behavior

| Scenario | cloud-current HTTP/envelope | selfhosted-min HTTP/envelope | Provider classification | Evidence |
|---|---|---|---|---|
| Missing token | `401`, `success=false`, `data=null`, code `Unauthorized` | Not tested | Authentication error; preserve state; no retry | [Cloud negative-auth evidence](evidence/20260731-cloud-auth.md) |
| Invalid token | Synthetic malformed value: `401`, `success=false`, `data=null`, code `Unauthorized` | Not tested | Authentication error; preserve state; no retry | [Cloud negative-auth evidence](evidence/20260731-cloud-auth.md) |
| Inactive token | Not tested; no controlled credential available | Not tested | Any 401 is an authentication error; preserve state; no retry; no token-state-specific body dependency | [FND-0011](findings.md#fnd-0011--missing-and-malformed-cloud-authorization-is-structured-401) |
| Insufficient permission | Not tested; no controlled restricted credential available | Not tested | Any 403 is an authorization error; preserve state; no mutation retry | [FND-0011](findings.md#fnd-0011--missing-and-malformed-cloud-authorization-is-structured-401) |
| Validation failure | Project/environment empty name: `400`, `success=false`, `data=null`, code `name_is_required` | Not tested | Validation/user diagnostic; no retry | [Cloud compatibility](evidence/20260731-cloud-project-environment.md) |
| Duplicate identity | Project/environment: `422`/`KeyHasBeenUsed`, exact set remains one; flag/segment code not tested and not relied upon | Not tested | All-page exact-zero preflight; one Create; on ambiguity exact re-read and Import diagnostic; never adopt or retry blindly | [FND-0038](findings.md#fnd-0038--child-duplicate-responses-are-not-a-reconciliation-dependency) |
| Missing project | After successful Delete: direct Read `500`, `success=false`, code `InternalServerError`; complete collection exact count 0 | Not tested | Direct result preserves state; remove only after complete exact-zero fallback | [Cloud compatibility](evidence/20260731-cloud-project-environment.md) |
| Missing environment | After successful Delete: direct Read `403`, `success=false`, code `Forbidden`; parent collection exact count 0 | Not tested | Direct result preserves state; remove only after parent exact-zero fallback | [Cloud compatibility](evidence/20260731-cloud-project-environment.md) |
| Missing flag | `404`, `success=false`, `data=null`, code `ResourceNotFound`; exact key count 0 | Not tested | Direct result preserves state; all-page exact-key zero fallback confirms absence | [Cloud child reads](evidence/20260731-cloud-project-environment.md) |
| Delete unarchived flag | `422`, `success=false`, `data=null`, code `CannotDeleteUnarchivedFeatureFlag`; direct Read remains 200 and exact active count remains one | Not tested | Archive exact owned flag, require success, then hard Delete; archive alone never removes state | [Cloud flag CRUD](evidence/20260731-cloud-feature-flags.md) |
| Flag exact absence after archive-plus-Delete | direct `404`/`ResourceNotFound`; exact active count 0 and exact archived count 0 for String, Number, and JSON | Not tested | Remove state only after direct absence and all-page exact zero in both `IsArchived=false` and `IsArchived=true` views | [Cloud type matrix](evidence/20260731-cloud-feature-flags.md) |
| Missing segment | `404`, `success=false`, `data=null`, code `ResourceNotFound`; exact UUID/key counts 0 | Not tested | Direct result preserves state; all-page exact-ID zero fallback confirms absence; exact key is an additional collision guard | [Cloud child reads](evidence/20260731-cloud-project-environment.md) |
| Empty segment scopes | `400`, `success=false`, `data=null`, code `scopes_is_invalid`; exact synthetic key remains absent | Not tested | Correct the request; scopes require a non-empty exact resource name; do not classify segment capability as missing | [Cloud segment lifecycle](evidence/20260731-cloud-segments.md) |
| Delete unarchived segment | `422`, `success=false`, `data=null`, code `CannotDeleteUnArchivedSegment`; exact segment remains present | Not tested | Preflight flag references; archive exact owned UUID, require success, then hard Delete; archive alone never removes state | [Cloud segment lifecycle](evidence/20260731-cloud-segments.md) |
| Segment exact absence after archive-plus-Delete | direct `404`/`ResourceNotFound`; exact active and archived UUID/key counts 0 | Not tested | Remove state only after direct absence and all-page exact zero in both `IsArchived=false` and `IsArchived=true` views | [Cloud segment lifecycle](evidence/20260731-cloud-segments.md) |
| Referenced segment Delete | Empty-reference preflight verified; non-empty live conflict not tested | Not tested | Call documented reference preflight; non-empty skips DELETE and preserves state; ambiguous/racing failure preserves state with no blind retry | [FND-0035](findings.md#fnd-0035--cloud-segment-destroy-requires-archive-before-hard-delete) |
| Stale revision | Not tested | Not tested | Refresh/no unsafe retry; same-value probe available | Offline contract only; [evidence](evidence/20260731-offline-contracts.md) |
| Rate limit | Not tested | Not tested | Safe-read retry only | Live probe optional |
| Transient server failure | Not tested | Not tested | Safe-read retry only | Mock acceptable |

## Normalization behavior

| Behavior | cloud-current | selfhosted-min | Canonical provider rule | Evidence |
|---|---|---|---|---|
| Project defaults | Supported for observed fields | Not tested | Ignore volatile fields; preserve exact key | [Lifecycle](evidence/20260731-cloud-project-environment.md) |
| Auto-created environments | Two: ordered `Dev/dev`, `Prod/prod`; project Delete cascades | Not tested | Computed ordered collection for the tested Cloud target | [Lifecycle](evidence/20260731-cloud-project-environment.md) |
| Environment defaults | Settings present; `requireChangeComment=false` | Not tested | Read server default; do not invent | [Lifecycle](evidence/20260731-cloud-project-environment.md) |
| Variation ID stability | Requested IDs preserved for all four flag types | Not tested | Always supply deterministic IDs on Create and map Read/Import by exact ID, not list position | [Cloud flags](evidence/20260731-cloud-feature-flags.md) |
| Enabled/fallthrough mapping | Enabled requested ID referenced in fallthrough; disabled requested ID preserved for all four types | Not tested | Map by stable variation ID, not list position | [Cloud flags](evidence/20260731-cloud-feature-flags.md) |
| Rule/condition ordering | One segment rule/condition preserved in order | Not tested | Preserve rule and condition order; included/excluded/tags use set semantics | [Cloud segment lifecycle](evidence/20260731-cloud-segments.md) |
| JSON normalization | Supported for one complex JSON flag | Not tested | Validate one JSON value; canonicalize key order and exact decimal spelling without `float64` | [Cloud type matrix](evidence/20260731-cloud-feature-flags.md) |
| Environment-specific segment type/scopes | One type and exact environment resource-name scope preserved through all specialized updates | Not tested | `RequiresReplace`; Create must resolve exactly one organization prefix without retaining it | [Cloud segment lifecycle](evidence/20260731-cloud-segments.md) |
| Shared segment scopes | Three exact rows were read only; parsable scopes yielded one unique in-memory organization prefix; contents discarded | Not tested | Do not manage; read/bind only until cross-scope mutation blast radius is independently approved and verified | [Cloud segment lifecycle](evidence/20260731-cloud-segments.md), [offline evidence](evidence/20260731-offline-contracts.md) |
| Secret metadata/types | Two per environment; all fields present; types Client/Server; values are strings and must be discarded | Not tested | Computed non-value metadata only; values excluded from ordinary state | [Lifecycle](evidence/20260731-cloud-project-environment.md) |

## Compatibility conclusion

Complete for the only available Phase 0 target. Current Cloud direct-token
transport, token-selected context, constrained project/environment lifecycle,
all four constrained flag types, one complex environment-specific segment,
canonicalization, exact absence, environment-secret metadata, and cleanup are
target-verified.

Project/environment duplicate status codes are verified. Child duplicate
codes, stale-revision writes, non-empty segment-reference conflicts, restore/
key reuse, and restricted-token shapes are not inferred; the rows use explicit
fail-closed, replace-only, read-only, external, or omitted behavior recorded in
the matrix. Shared segments remain read/bind only. Member creation is external,
and member lookup/IAM are deferred from core v1 pending later target evidence.

No self-hosted target can be provisioned for Phase 0. All self-hosted cells
therefore remain `Not tested`; this is an unavailable-target coverage limit,
not an observed incompatibility. No exact self-hosted release is certified.
[ADR-005](adrs/ADR-005-supported-compatibility-matrix.md) accepts a
deployment-neutral pinned public-API contract for Cloud and self-hosted while
keeping empirical target certification separate.
