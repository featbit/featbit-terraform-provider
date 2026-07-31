# Phase 0 Findings

This file contains verified observations, explicit hypotheses, and capability classification. Every verified finding must link sanitized evidence.

## Status vocabulary

- **Verified**: reproduced on the named deployment/version with evidence.
- **Documented**: stated by an official source but not yet reproduced.
- **Hypothesis**: requires a probe or decision.
- **Superseded**: retained for history and linked to the correction.

## Established project constraints

### FND-0001 — Current API is fixed for provider v1

Status: **Decision**  
Source: User direction recorded on 2026-07-30.

Provider GA must not depend on FeatBit backend or public OpenAPI changes. Gaps are handled through the provider compatibility layer, constrained lifecycle semantics, external prerequisites, or omission.

### FND-0002 — LaunchDarkly is a reference, not a parity requirement

Status: **Decision**  
Source: User direction recorded on 2026-07-30.

LaunchDarkly may inform mature Terraform engineering patterns. FeatBit customer workflows and FeatBit's resource model determine scope and schema.

### FND-0003 — API access tokens are the provider authentication contract

Status: **Decision and documented contract; successful live verification pending**

Official FeatBit documentation states that the REST API accepts personal and service access tokens in the `Authorization` header. Service tokens are intended for long-term integrations. Provider v1 does not implement login or JWT lifecycle management.

Sources:

- <https://docs.featbit.co/api-docs/overview>
- <https://docs.featbit.co/integrations/api-access-tokens>
- <https://docs.featbit.co/api-docs/using-featbit-rest-api>

Required evidence: `P0-020` through `P0-025`.

## OpenAPI findings

### FND-0006 — The public OpenAPI baseline matches the planning inventory

Status: **Verified (public specification)**

The exact upstream document is 292420 bytes with SHA-256
`8DE202F939F6721748D66449C3DFE4EEE2E2BF369A57F121DF808907A44D11C4`.
It declares OpenAPI `3.0.4`, API version `1.0`, 60 paths, 76 operations, 112
schemas, 454 direct schema properties, and `AccessToken` plus `JwtBearer`
security schemes. All 76 upstream operations omit `operationId`.

Evidence:
[20260731-offline-contracts.md](evidence/20260731-offline-contracts.md)

### FND-0007 — A narrow operation-ID overlay is deterministic

Status: **Verified (offline contract)**

The provider-owned Overlay 1.1.0 input adds a stable unique operation ID to
each of the 76 existing operations and changes nothing else. The repository
tool validates targets and uniqueness, produces byte-stable output, and detects
stale generated artifacts.

Evidence:
[20260731-offline-contracts.md](evidence/20260731-offline-contracts.md)

## Probe safety findings

### FND-0004 — Mutating probes fail closed

Status: **Verified (offline contract)**

The Phase 0 probe accepts credentials only through the documented environment
variables. A write additionally requires a safe `tfp0-` prefix and a
target-specific URL check: the exact documented FeatBit Cloud API host over
HTTPS or a loopback/private self-hosted target. Empty prefixes, lookalike Cloud
hosts, HTTP Cloud URLs, and public self-hosted hosts are rejected.

Evidence:
[20260731-offline-contracts.md](evidence/20260731-offline-contracts.md)

### FND-0005 — Probe diagnostics and evidence are redacted by construction

Status: **Verified (offline contract)**

Sensitive headers, access-token/JWT patterns, secret response values, tenant
context fields, and member emails are replaced before diagnostic output.
Configuration reporting reveals presence only. The runtime cleanup inventory is
Git-ignored and contains only exact disposable resource identities.

Evidence:
[20260731-offline-contracts.md](evidence/20260731-offline-contracts.md)

## Authentication findings

### FND-0008 — The probe implements direct access-token transport without login

Status: **Verified (offline transport contract); live verification pending**

An HTTP contract test proves that the selected service/personal API access
token is sent directly as the `Authorization` header value. The probe contains
no login, username/password, refresh, MFA, SSO, or context-header flow. Its
serialized observation excludes headers and raw data.

This is not yet evidence that either Cloud or self-hosted accepts the header;
`P0-020` and `P0-021` remain live tasks.

Evidence:
[20260731-offline-contracts.md](evidence/20260731-offline-contracts.md)

### FND-0011 — Missing and malformed Cloud authorization is structured 401

Status: **Verified (`cloud-current`)**

Both an omitted header and a synthetic malformed header returned HTTP 401,
`success=false`, `data=null`, and structured error code `Unauthorized`.
Neither case is absence or retryable. Inactive and insufficient-scope behavior
remain untested.

#### 2026-07-31 unavailable-token reduction

No controlled inactive or insufficient-scope credential is available. Those
target rows remain `Not tested`, but they create no Phase 1 body/code
assumption: every HTTP 401 is an authentication diagnostic and every HTTP 403
is an authorization diagnostic; both preserve state and mutations are never
retried. Provider v1 uses one permission-scoped token contract and does not
branch on token kind or lifecycle state.

Evidence:
[20260731-cloud-auth.md](evidence/20260731-cloud-auth.md)

### FND-0012 — Phase 1 authentication schema is access-token only

Status: **Decision**

Phase 1 will expose a Sensitive `access_token` provider attribute with the
`FEATBIT_ACCESS_TOKEN` environment fallback and an `api_url` override with the
documented Cloud default. It will send personal or service API access tokens
directly in `Authorization`. It will not expose username, password, JWT
refresh, MFA, SSO, organization-header, or workspace-header settings.

Multiple account contexts use normal Terraform provider aliases, each with its
own token and optional API URL. Service tokens are recommended for CI/CD.
Successful direct-token and no-extra-context-header behavior must still be
confirmed before the Phase 0 exit gate.

Sources/evidence:

- <https://docs.featbit.co/api-docs/using-featbit-rest-api>
- <https://docs.featbit.co/integrations/api-access-tokens>
- [Offline transport contract](evidence/20260731-offline-contracts.md)
- [Cloud negative-auth behavior](evidence/20260731-cloud-auth.md)

## Error and absence findings

### FND-0009 — Exact identity requires all pages and an exact match count

Status: **Verified (offline contract); live endpoint verification pending**

The shared helper reads every required page and returns zero, one, or multiple
exact matches. ID/key comparison is exact. Email comparison is normalized by
trimming and lowercasing. Fuzzy results are ignored even when they appear
first.

Evidence:
[20260731-offline-contracts.md](evidence/20260731-offline-contracts.md)

### FND-0010 — A direct 404 is not sufficient to remove Terraform state

Status: **Decision supported by offline tests; live status/envelope shapes pending**

The centralized classifier maps 401 to authentication, 403 to authorization,
404 to `not_found_unconfirmed`, 409 to conflict, 429 to rate limiting, and 5xx
to transient server failure. A `2xx` response with `success=false` is an
application failure. State becomes absent only after a completed exact scoped
fallback returns zero matches. Duplicate matches and any incomplete fallback
preserve state.

Only safe reads retry rate limits, transient 5xx, timeouts, or network
failures. Mutations are not automatically retried.

Evidence:
[20260731-offline-contracts.md](evidence/20260731-offline-contracts.md)

The complete Phase 1 decision table is in
[the OpenAPI surface evidence](evidence/20260731-offline-contracts.md).

### FND-0013 — The OpenAPI document is intentionally insufficient for generated provider semantics

Status: **Verified (public specification)**

No schema declares required fields or enums, and every operation documents only
200/401/403. The provider must handwrite validation and use the centralized
status/envelope/exact-fallback classifier. Generic flag and segment PATCH
operations have no request schema and are excluded from the safe update path.

Evidence:
[20260731-offline-contracts.md](evidence/20260731-offline-contracts.md)

### FND-0014 — Project exact absence uses the complete non-paginated list

Status: **Documented by the pinned specification; live verification pending**

`GET /api/v1/projects` has no pagination inputs and returns the project
collection. The project fallback scans the complete collection for an exact ID.
The earlier planning phrase “fully paginated project list” is superseded by
this target-specific contract; pagination remains mandatory for list endpoints
that actually expose it.

Evidence:
[20260731-offline-contracts.md](evidence/20260731-offline-contracts.md)

### FND-0027 — Cloud project/environment absence requires exact collection fallback

Status: **Verified (`cloud-current`)**

After successful Delete, direct Read did not return a conventional not-found
shape: the environment endpoint returned HTTP 403, `success=false`, code
`Forbidden`, while the project endpoint returned HTTP 500, `success=false`,
code `InternalServerError`. Neither response may remove Terraform state.

The documented workarounds both converged safely:

- read the parent project and count the exact deleted environment ID; and
- read the complete, non-paginated project collection and count the exact
  deleted project ID.

Each collection contained the owned identity exactly once before Delete and
zero times afterward. The direct 403/500 classifications preserve state until
that successful exact-zero fallback completes. No fuzzy result, first search
result, undocumented endpoint, backend change, or blind write retry is needed.

This supersedes the live-pending portion of `FND-0010` for project/environment
and the live-pending portion of `FND-0014`. Flag and segment absence remain
unverified.

Evidence:
[20260731-cloud-project-environment.md](evidence/20260731-cloud-project-environment.md)

### FND-0030 — Recovery cleanup converges on exact absence, not DELETE status

Status: **Verified offline recovery contract; target shapes remain matrix-specific**

The cleanup executor sends each mutation once, then independently verifies the
desired absent state. Project and environment use the complete project
collection; flags use all-page exact key; segments use all-page exact UUID. A
successful DELETE with a remaining exact identity stays pending, while an
ambiguous DELETE with completed exact-zero evidence may be marked clean.

Flag/segment inventory entries retain their owned project parent. If a child
collection becomes unavailable only after parent deletion, exact absence of
the owned project or environment proves the child absent. A still-present
parent plus failed child lookup never proves absence.

Cleanup observations use documented path templates, and CLI output replaces
runtime UUIDs/keys with fixed redaction markers. Thus recovery can retain exact
identities internally without printing them.

Cloud project/environment fallback shapes are verified separately by
`FND-0027`. Flag and segment cleanup fallbacks remain offline contracts, not
deployment claims.

Evidence:
[Offline cleanup exact fallbacks](evidence/20260731-offline-contracts.md)

### FND-0031 — Cloud missing flag/segment reads are 404 but still require exact fallback

Status: **Verified (`cloud-current`, absent cases only)**

Inside a newly created owned environment, a synthetic missing feature-flag key
and a deterministic missing segment UUID each returned HTTP 404,
`success=false`, `data=null`, and code `ResourceNotFound`.

Neither direct result was treated as absence by itself. The complete paginated
flag collection returned zero exact key matches. Two independent complete
segment scans returned zero exact UUID and zero exact key matches. The direct
responses therefore remain `not_found_unconfirmed` until their documented
exact-zero fallback succeeds.

This verifies missing/absent read behavior only. No child resource was created
or deleted, so post-delete and present cases remain open.

Evidence:
[Cloud child-read compatibility](evidence/20260731-cloud-project-environment.md)

## Project and environment findings

### FND-0024 — Cloud project/environment CRUD converges and project Delete cascades

Status: **Verified (`cloud-current`)**

One uniquely prefixed project completed Create, canonical Read, name Update,
two equal canonical Reads, and Delete. Its key was preserved across Update.
Creation returned two automatic environments ordered `Dev/dev`, then
`Prod/prod`; both exposed settings with `requireChangeComment=false`.

One additional environment completed Create, canonical Read,
name/description Update, two equal canonical Reads, and Delete. The parent and
key were preserved. The probe confirmed the returned environment identity
under the new project before mutation.

Environment Delete was synchronously observable through an exact parent Read.
Project Delete succeeded while both automatic environments remained, and the
next complete project collection contained zero exact matches. Cloud therefore
supports constrained project/environment management with project key and
environment parent/key treated as replace-only.

Duplicate project-key behavior was not tested, so `P0-041` remains open. No
workaround path was used in this lifecycle.

This supersedes the live-pending portions of `FND-0014` and `FND-0023`;
their exact-lookup and ownership contracts remain unchanged.

Evidence:
[20260731-cloud-project-environment.md](evidence/20260731-cloud-project-environment.md)

Correction (2026-07-31): the statement that duplicate project-key behavior was
not tested is superseded by `FND-0026`. The original lifecycle evidence remains
unchanged.

### FND-0026 — Cloud validation and duplicate-key failures are structured

Status: **Verified (`cloud-current`)**

Project and environment empty-name Creates returned HTTP 400,
`success=false`, `data=null`, code `name_is_required`, with zero exact matches.
Submitting each newly created key a second time returned HTTP 422,
`success=false`, `data=null`, code `KeyHasBeenUsed`; the applicable complete or
parent collection still contained exactly the original ID.

The public project/environment Update schemas omit key, and successful Updates
preserved the original keys. Project key and environment key therefore use
`RequiresReplace`; parent project is also replace-only for an environment.
HTTP 400/422 failures become user diagnostics and never cause the provider to
adopt the first fuzzy result or discard the existing object.

This verifies only project/environment rows. Flag and segment duplicate
behavior and post-delete key reuse remain untested.

Evidence:
[20260731-cloud-project-environment.md](evidence/20260731-cloud-project-environment.md)

### FND-0038 — Child duplicate responses are not a reconciliation dependency

Status: **Constrained decision; parent behavior target-verified, child codes `Not tested`**

Cloud project and environment duplicate Creates returned structured 422
`KeyHasBeenUsed` and left the exact identity set at one. A second flag or
segment Create was outside the approved scopes, so their deployment-specific
duplicate status/code is not inferred.

Provider v1 avoids that dependency. Before one child Create it reads every
active and archived page and requires zero exact identities. Any failed or
ambiguous Create is not retried. A complete exact re-read then classifies zero
as failed creation, one as an exact Import/recovery diagnostic, and multiple as
ambiguity; it never selects a fuzzy or first result and never silently adopts
an object. This constrained workflow is covered by deterministic flag/segment
mock tests.

Evidence:

- [Cloud parent duplicate behavior](evidence/20260731-cloud-project-environment.md)
- [Offline flag guardrails](evidence/20260731-offline-contracts.md)
- [Offline segment guardrails](evidence/20260731-offline-contracts.md)

## Feature flag findings

### FND-0028 — Feature-flag live probe is fail-closed but deployment behavior remains unverified

Status: **Verified offline safety contract; live behavior pending**

The reusable feature-flag lifecycle creates its own disposable project and
environment and accepts no caller-supplied remote identity. It proves the
synthetic flag key absent across every advertised page, inventories the exact
`environment_id/flag_key` identity immediately after Create or exactly-one
ambiguous-create reconciliation, and never selects a fuzzy match.

The probe uses the documented specialized name operation and requires every
unrelated canonical field to remain unchanged. If that update advances the
revision, the stale-revision probe sends the already-current variation values
with the prior revision. This labelled same-value workaround can observe a
conflict without risking a logical variation change. The undocumented-schema
generic PATCH is excluded.

Delete is confirmed only after a direct Read plus a complete all-page exact-key
lookup returns zero. A controlled failure leaves the flag, environment, and
project in dependency-ordered cleanup; the mock cleanup converges to
`pending=0`. Mock HTTP 404 and 409 responses test classification only and are
not Cloud observations.

No live feature-flag request was made because the current production mutation
approval covers only newly created project/environment objects. This finding
does not upgrade feature-flag support, complete `P0-050` through `P0-059`, or
satisfy ADR-001/ADR-002.

Evidence:
[Offline feature-flag lifecycle guardrails](evidence/20260731-offline-contracts.md)

### FND-0033 — Cloud flag destroy requires archive then hard Delete

Status: **Verified (`cloud-current`, one owned boolean flag)**

This dated finding supersedes the live-pending portion of `FND-0028` and the
flag cleanup-pending portion of `FND-0030`; those historical findings remain
unchanged.

Inside a project and explicit environment created by the same invocation, one
boolean flag completed Create, exact Read, specialized name Update, repeated
canonical Read, and cleanup. The server preserved the requested variation IDs,
returned a revision, advanced that revision after the name update, and
preserved every unrelated canonical field, including the empty targeting and
rule collections.

Direct hard Delete of the unarchived flag returned HTTP 422,
`success=false`, `data=null`, and code
`CannotDeleteUnarchivedFeatureFlag`. Direct Read and the active collection
still contained the exact flag, so state and cleanup ownership were preserved.

The safe public-API workaround is:

1. archive the exact owned flag through the documented specialized endpoint;
2. require success before retrying hard Delete;
3. send hard Delete once; and
4. confirm zero exact keys across both `IsArchived=false` and
   `IsArchived=true` paginated views.

Archive is a Delete prerequisite, not destroy completion. The default flag
collection hides archived objects, so checking it alone can falsely report
absence while a key remains reserved. Cleanup applied this workaround, removed
the flag and both owned parents, and ended at `pending=0`.

The approved scope deliberately excluded duplicate Create, stale-revision
writes, other variation types, complex targeting/rules/rollouts, restore, and
key reuse. Those rows remain untested, as does every self-hosted row.

Evidence:
[Cloud boolean flag CRUD](evidence/20260731-cloud-feature-flags.md)

### FND-0036 — Reduced v1 Boolean mapping avoids unverified ID and revision writes

Status: **Verified (`cloud-current`) plus conservative capability reduction**

One Cloud Boolean Create supplied two client UUIDs. Read preserved both,
referenced the enabled UUID through fallthrough, retained the disabled UUID,
and advanced a non-empty revision after the specialized name update.

Provider v1 therefore always supplies variation UUIDs on Create and maps
enabled/fallthrough/disabled state by exact UUID, never by list position.
Imported flags retain the exact IDs returned by Read. It does not depend on an
omitted-ID/server-generation path. Description and variation changes are
replace-only under ADR-001.

Revision is Computed. The offline same-value stale contract verifies conflict
classification without risking a logical change, but v1 submits no
revision-bearing operational update and never blindly retries a stale write.
It refreshes, preserves state, and reports the conflict.

Exact JSON/number/condition canonicalization remains deterministic offline.
JSON variation flags are omitted from the verified v1 type set; the live
segment lifecycle additionally round-tripped the condition JSON-string
encoding. This closes the normalization decision without claiming a live JSON
flag lifecycle.

Evidence:

- [Cloud Boolean flag CRUD](evidence/20260731-cloud-feature-flags.md)
- [Offline normalization prototypes](evidence/20260731-offline-contracts.md)
- [Offline stale-revision guardrail](evidence/20260731-offline-contracts.md)
- [Cloud segment condition round trip](evidence/20260731-cloud-segments.md)

### FND-0037 — v1 supports all four public flag types with immutable type and variations

Status: **Verified (`cloud-current`) for Boolean, String, Number, and JSON**

The product owner requires `boolean`, `string`, `number`, and `json` feature
flags in provider v1. The public Create/Read contract uses one endpoint and one
variation model for all four types; variation values are transported as
strings. A reusable probe now composes String, Number, and JSON lifecycles
sequentially under one owned project/environment and has passing mock coverage.

Provider v1 exposes all four exact `variation_type` values. It canonicalizes
Boolean values to lowercase, preserves String values byte-for-byte,
canonicalizes Number values without `float64`, and canonicalizes JSON while
preserving decimal precision. Every Create supplies stable variation UUIDs,
and Read/Import maps by exact UUID rather than list position.

This is a constrained workaround, not a false target claim. Type, description,
and variations are `RequiresReplace`, so v1 adds no unverified type-specific or
variation-update write. Enabled state, targeting, rules, rollouts, and tags
remain Computed-only or omitted and UI-owned. Only Boolean has a live Cloud
round trip; the other three rows remain explicitly target-unverified until the
contained type-matrix probe receives separate approval.

This finding supersedes only the Boolean-only/JSON-omitted v1 scope statements
in `FND-0036` and the capability table. Its live Boolean identity and revision
observations remain valid.

Evidence:

- [Normalization prototypes](evidence/20260731-offline-contracts.md)
- [Cloud Boolean flag CRUD](evidence/20260731-cloud-feature-flags.md)
- [Cloud String/Number/JSON type matrix](evidence/20260731-cloud-feature-flags.md)

#### 2026-07-31 Cloud type-matrix confirmation

The earlier Boolean-only target-certification paragraph is superseded. Under
the separately approved contained scope, String, Number, and JSON each
completed exact Create/Read, name-only Update, repeated canonical Read,
archive-plus-hard-Delete, direct post-delete 404, and active-plus-archived
exact-zero verification. The JSON desired-versus-observed comparison was
logically empty. Cleanup ended at `pending=0`.

This evidence removes the temporary public-specification-only workaround for
the three additional Cloud type rows. It does not expand Terraform ownership:
targeting, rules, rollouts, enabled state, and tags remain UI-owned, and type,
description, and variations remain replace-only.

### FND-0016 — Phase 1 has deterministic offline normalization primitives

Status: **Verified (offline prototype); live canonical reads pending**

Variation IDs map safely to Terraform indices, rollout weights round-trip at
integer scale 100000, JSON/number/boolean/condition values canonicalize
deterministically, and set/list semantics are explicit. A synthetic complex
flag produces an empty logical diff after canonicalization.

This does not establish server defaults, ordering, enabled/fallthrough mapping,
or revision behavior.

Evidence:
[20260731-offline-contracts.md](evidence/20260731-offline-contracts.md)

### FND-0017 — Partial flag creation is a resumable checkpoint workflow

Status: **Decision supported by offline prototype; live injection pending**

The base identity is registered for cleanup immediately, then variations,
targeting, tags, and canonical read execute in order. Ambiguous base creation
requires exact lookup; later failure returns the exact
`environment_id/flag_key` Import identity and resumes from the first incomplete
step. No mutating call is retried blindly.

Evidence:
[20260731-offline-contracts.md](evidence/20260731-offline-contracts.md)

## Segment findings

### FND-0032 — A new Cloud environment does not imply an empty segment collection

Status: **Verified (`cloud-current`); item types intentionally unclassified**

The segment collection in a newly created environment returned a completed
page containing three items even though the probe had created no segment. The
probe discarded all item identities and fields, never selected the first
result, and retained only zero exact matches for its synthetic UUID and key.

Consequently, parent ownership is not segment ownership. Every lookup and
create preflight must traverse the advertised collection and compare exact
identity; it must never assume a fresh environment has no segments. The
observation is consistent with documented cross-scope shared segments, but the
list shape does not prove any row's type. Shared mutation therefore remains
read/bind only rather than adopting an existing row.

Evidence:
[Cloud child-read compatibility](evidence/20260731-cloud-project-environment.md)

### FND-0029 — Environment-specific segment probing is contained; shared mutation is not

Status: **Verified offline safety contract and scope reduction; live behavior pending**

The reusable segment lifecycle creates its own disposable project and
environment and accepts no caller-supplied remote identity. Within that owned
environment it uses all-page exact-key preflight, immediately inventories the
returned segment UUID, rejects fuzzy/duplicate identities, and uses only the
documented name, description, targeting, tags, archive, and restore
operations. The schema-less generic PATCH is excluded.

Before Delete it uses the documented flag-reference collection. Any reference
causes a fail-closed diagnostic and preserves state so the exact flag
configuration can be changed first. With zero references, absence is confirmed
only after direct Read plus all-page exact UUID and key counts both return
zero. Controlled failure and unexpected-duplicate mocks converge through
dependency-ordered cleanup.

Shared segment mutation is deliberately excluded. Product documentation says
its scopes may cross environments, projects, or organizations, while the
OpenAPI does not define how those scope strings encode exact identities. A
newly created environment therefore does not contain the mutation blast
radius. Provider v1 treats shared segments as **Read/bind only** unless a
disposable workspace or verified exact-scope contract supplies the missing
evidence; it must not guess a scope payload.

No live segment request was made. This finding does not complete `P0-060`
through `P0-065`, upgrade a deployment matrix cell, or satisfy ADR-001/ADR-002.

Evidence:
[Offline segment lifecycle guardrails](evidence/20260731-offline-contracts.md)

Dated correction (`2026-07-31`): `FND-0034` and `FND-0035` supersede the
live-pending sentences above for environment-specific segments. The shared
segment mutation boundary is unchanged.

### FND-0034 — Cloud environment-specific segments need resource-name scopes and converge

Status: **Verified (`cloud-current`) with constrained scope discovery**

The OpenAPI declares `scopes` as an array of strings but omits its non-empty
and resource-name contract. A contained Create with `scopes=[]` returned HTTP
400, `success=false`, code `scopes_is_invalid`; no segment was created and both
parents were cleaned.

The corrected probe scans every active and archived segment page, direct-Reads
each returned exact UUID, extracts organization prefixes from returned scope
resource names, and requires exactly one unique prefix. It constructs the new
project/environment scope in memory without logging or persisting any tenant
value. This is a constrained workaround for the missing public organization-key
read. It fails closed when the exact public Reads do not yield one prefix.

With that scope, one environment-specific segment completed Create, exact Read,
name/description/targeting/tags updates, archive/restore, and repeated canonical
Reads. Two included synthetic user keys, one excluded key, one rule/condition,
and two tags round-tripped; unrelated fields stayed equal. Key, type, and the
single environment scope survived every specialized update, but remain
`RequiresReplace` because no documented update operation owns them.

This completes the live environment-specific ownership/canonical rows without
expanding shared-segment mutation. Existing rows used for scope discovery were
read only by exact UUID and their identities/contents were discarded.

Evidence:
[Cloud environment-specific segment lifecycle](evidence/20260731-cloud-segments.md)

### FND-0035 — Cloud segment destroy requires archive before hard Delete

Status: **Verified (`cloud-current`)**

After a successful archive/restore cycle and an empty documented
`flag-references` preflight, hard Delete of the active segment returned HTTP
422, `success=false`, code `CannotDeleteUnArchivedSegment`. The exact segment
remained present.

The safe provider workflow is to archive the exact owned UUID, require archive
success, retry hard Delete once, then require direct 404/`ResourceNotFound` and
zero exact UUID/key matches across every page of both active and archived
views. The lifecycle and recovery cleanup both exercised this workaround and
ended at `pending=0`. Archive alone is not destroy completion.

A non-empty live flag-reference conflict was not created. The provider must
retain the existing fail-closed preflight and preserve state whenever
references exist or any delete result is ambiguous.

#### 2026-07-31 referenced-segment reduction

The live non-empty server conflict shape remains `Not tested`, but provider v1
does not rely on it. Destroy first calls the documented `flag-references`
operation. A non-empty result prevents DELETE and preserves state with an
actionable diagnostic; the user removes those exact references through the UI
before retrying. If the preflight is empty but a race or ambiguous DELETE
occurs, state is still preserved and the mutation is not retried blindly.

Evidence:
[Cloud environment-specific segment lifecycle](evidence/20260731-cloud-segments.md)

### FND-0018 — Segment key, type, and scopes remain replace-only

Status: **Decision supported by public specification and offline normalization**

Create exposes key/type/scopes, but no documented specialized update operation
does. The generic PATCH has no request schema. Phase 1 must therefore apply
`RequiresReplace` to all three regardless of any single-target undocumented
behavior. Included/excluded users, tags, and scopes use set semantics; rules
and conditions preserve order.

Evidence:

- [OpenAPI surface](evidence/20260731-offline-contracts.md)
- [Normalization prototypes](evidence/20260731-offline-contracts.md)

## Member and IAM findings

### FND-0019 — Member creation is an external prerequisite for v1

Status: **Accepted external-prerequisite decision; live invitation behavior unverified**

The documented add operation returns a boolean rather than a member ID, and no
live target is available to prove exactly-one normalized-email reconciliation
or invitation timing. Provider v1 therefore does not manage member creation.
Core v1 also ships no member lookup or IAM binding. The later IAM stage may
look up members by normalized exact email across all pages and use their IDs in
bindings only after live endpoint verification.

This is the deliberate workaround for the missing disposable invitation
workflow. It removes synchronous invitation timing, human acceptance, and
exact-ID polling assumptions from core v1. Member provisioning remains
external. A managed member resource remains omitted unless a future
target-specific probe supersedes this decision.

Evidence:
[20260731-offline-contracts.md](evidence/20260731-offline-contracts.md)

### FND-0020 — IAM bindings use independent composite identities

Status: **Verified (public surface and offline identity contract)**

Group-member, group-policy, and member-policy endpoints explicitly carry both
stable UUIDs and expose relationship lists for exact verification. Their
Import IDs are `<left_uuid>/<right_uuid>`. No parent resource owns an entire
relationship set.

Evidence:
[20260731-offline-contracts.md](evidence/20260731-offline-contracts.md)

## Environment secret findings

### FND-0015 — Environment secret values are excluded from initial Terraform state

Status: **Decision; live field-presence consistency pending**

The public `Secret` model includes nullable `id`, `name`, `type`, and `value`
without enums or required metadata. Managed project/environment state retains
only Computed non-value metadata and never guesses server/client roles. Secret
values are discarded before logging/state. An explicit Sensitive data source
is deferred until live consistency and state-security documentation are
verified. Secret rotation is external.

Evidence:
[20260731-offline-contracts.md](evidence/20260731-offline-contracts.md)

### FND-0025 — Cloud returns environment secret values; ordinary state must discard them

Status: **Verified (`cloud-current`)**

All three observed environments returned two secret objects. Every object had
`id`, `name`, `type`, and string `value` fields; observed type classes were
`Client` and `Server`. Values and identities were discarded in memory before
report serialization and never entered evidence.

This live result strengthens, rather than expands, the earlier boundary:
ordinary project/environment resources may expose only Computed non-value
metadata. Secret values remain outside normal Terraform state; rotation remains
external. This supersedes only the live-consistency-pending portion of
`FND-0015`.

Evidence:
[20260731-cloud-project-environment.md](evidence/20260731-cloud-project-environment.md)

## Capability support matrix

These are conservative provider-v1 classifications under the current evidence.
An unavailable live behavior is narrowed rather than assumed supported.

| Capability | Customer workflow | Support level | Identity | Lifecycle constraints | Evidence/ADR |
|---|---|---|---|---|---|
| Project | Bootstrap and manage projects | Constrained managed | Project UUID | Cloud CRUD/canonical convergence/cascade verified; key replace-only; duplicate returns structured 422; direct post-delete 500 requires complete-list exact fallback; automatic environments Computed | [FND-0024](#fnd-0024--cloud-projectenvironment-crud-converges-and-project-delete-cascades), [FND-0026](#fnd-0026--cloud-validation-and-duplicate-key-failures-are-structured), [FND-0027](#fnd-0027--cloud-projectenvironment-absence-requires-exact-collection-fallback), [ADR-004](adrs/ADR-004-import-identities.md) |
| Environment | Manage deployment environments | Constrained managed | Project UUID + environment UUID | Cloud CRUD/canonical convergence verified; parent/key replace-only; duplicate returns structured 422; direct post-delete 403 requires parent exact fallback; secret values excluded | [FND-0024](#fnd-0024--cloud-projectenvironment-crud-converges-and-project-delete-cascades), [FND-0025](#fnd-0025--cloud-returns-environment-secret-values-ordinary-state-must-discard-them), [FND-0026](#fnd-0026--cloud-validation-and-duplicate-key-failures-are-structured), [FND-0027](#fnd-0027--cloud-projectenvironment-absence-requires-exact-collection-fallback) |
| Feature flag | Manage Boolean, String, Number, and JSON values without overwriting UI operations | Constrained managed | Environment UUID + exact key | All four types have target-verified Cloud Create/Read/name-Update/canonicalization and deterministic cleanup; key/type/description/variations replace-only; enabled/targeting/rules/rollouts/tags Computed-only or omitted; destroy archives then hard-deletes and proves direct plus two-view absence | [FND-0037](#fnd-0037--v1-supports-all-four-public-flag-types-with-immutable-type-and-variations), [FND-0033](#fnd-0033--cloud-flag-destroy-requires-archive-then-hard-delete), [ADR-001](adrs/ADR-001-terraform-ui-ownership.md), [ADR-002](adrs/ADR-002-delete-vs-archive.md) |
| Environment-specific segment | Manage reusable audiences in one environment | Constrained managed | Environment UUID + segment UUID | Cloud complex CRUD/canonical convergence verified; Create needs fail-closed resource-name scope discovery; key/type/scopes replace-only; destroy preflights references, archives, hard-deletes, and proves active-plus-archived exact zero | [FND-0034](#fnd-0034--cloud-environment-specific-segments-need-resource-name-scopes-and-converge), [FND-0035](#fnd-0035--cloud-segment-destroy-requires-archive-before-hard-delete), [FND-0018](#fnd-0018--segment-key-type-and-scopes-remain-replace-only), [ADR-004](adrs/ADR-004-import-identities.md) |
| Shared segment | Reuse an audience across scopes | Read/bind only | Environment context + segment UUID | Exact public Reads exposed resource-name scopes for safe in-memory prefix discovery, but cross-scope Create/Update/Delete blast radius remains untested; do not manage in v1 without superseding evidence | [FND-0032](#fnd-0032--a-new-cloud-environment-does-not-imply-an-empty-segment-collection), [FND-0029](#fnd-0029--environment-specific-segment-probing-is-contained-shared-mutation-is-not), [FND-0034](#fnd-0034--cloud-environment-specific-segments-need-resource-name-scopes-and-converge) |
| Group | Manage IAM groups in a later stage | Constrained managed; deferred from core v1 | Group UUID | CRUD is documented; live lifecycle is a future IAM-stage entry gate; relationships remain independent | [IAM evidence](evidence/20260731-offline-contracts.md) |
| Policy | Manage IAM policies in a later stage | Constrained managed; deferred from core v1 | Policy UUID | Settings/statements use specialized updates; live lifecycle is a future IAM-stage entry gate; relationships remain independent | [IAM evidence](evidence/20260731-offline-contracts.md) |
| Member | Resolve and bind externally provisioned members in a later stage | Read/bind only; deferred from core v1 | Member UUID after all-page normalized exact-email lookup | Creation/invitation is external; target lookup verification is required before the later data source/bindings ship; duplicate matches fail safely | [FND-0019](#fnd-0019--member-creation-is-an-external-prerequisite-for-v1) |
| Environment secrets | Inspect non-value metadata | Read/bind only | Environment UUID + server-returned secret metadata | Values omitted from initial state; no guessed roles; rotation external; opt-in Sensitive data source deferred | [FND-0015](#fnd-0015--environment-secret-values-are-excluded-from-initial-terraform-state) |
| Workspace | Read account context | Read/bind only | Token-selected singleton | Data source only; updates omitted without a demonstrated deterministic workflow | [Public surface](evidence/20260731-offline-contracts.md) |
| Audit/analytics | Operational observation | Omitted | N/A | Dynamic operational data is not managed desired state | [Master scope](../plan.md) |

Additional deliberate boundaries:

| Capability | Support level | Reason/customer path |
|---|---|---|
| Access-token lifecycle | External prerequisite | A provider cannot safely bootstrap its own credential; create/rotate outside Terraform |
| Organization lifecycle | External prerequisite | Account/workspace bootstrap is outside the current deterministic provider lifecycle |
| Member creation/invitation | External prerequisite | Add returns no ID and synchronous exact reconciliation is unverified |
| Environment secret rotation | External prerequisite | No documented rotation lifecycle |
| Webhooks, approvals, triggers, scheduled changes | Omitted | No verified public management surface/customer workflow for v1 |
| Experiments, metrics, audit streams | Omitted | Dynamic analytical/operational data, not durable desired state |

Allowed support levels:

- Fully managed
- Constrained managed
- Read/bind only
- External prerequisite
- Omitted

## Superseded findings

### 2026-07-31 authenticated Cloud correction

The live-pending portions of `FND-0008` and `FND-0012` are superseded by
`FND-0021`. Their provider authentication decision is unchanged.

### FND-0021 — Cloud accepts a direct service token without context headers

Status: **Verified (`cloud-current`)**

A sanitized `GET /api/v1/projects` request returned HTTP 200,
`success=true`, and an array. The service access token was sent directly in
`Authorization`; no organization or workspace header was sent. No raw body,
resource identity, tenant context, or environment secret value was retained.

This verifies the provider authentication and context contract for the current
Cloud deployment. Personal-token comparison is `N/A` because no personal token
was supplied. Mutation permissions and lifecycle behavior remain separate
tasks.

Evidence:
[20260731-cloud-auth.md](evidence/20260731-cloud-auth.md)

### FND-0022 — Personal and service labels do not create separate provider authentication contracts

Status: **Product-owner correction recorded 2026-07-31**

The product owner confirmed that FeatBit personal/access-token labels use the
same direct `Authorization` transport and the same permission model. Effective
capabilities depend on the permissions granted to the token, not on a
provider-visible token kind.

Therefore provider v1 exposes one Sensitive `access_token` attribute and no
`token_kind`, `personal_token`, or `service_token` schema choice. The separate
comparison requested by `P0-021` is not applicable; the successful access-token
probe plus permission-specific operation observations cover the relevant
contract.

This supersedes the earlier assumption that a second credential was needed for
authentication comparison. Historical evidence remains unchanged because it
accurately records which environment variable supplied that particular run.

Evidence:
[Session-log correction](session-log.md#2026-07-31--access-token-terminology-corrected)

### FND-0023 — Project/environment lifecycle ownership is create-returned-ID only

Status: **Verified offline safety contract; live behavior pending**

The lifecycle command accepts no resource ID arguments. It proves the
synthetic project key absent before create, records each returned UUID
immediately, confirms the additional environment belongs to that new project,
and constructs every later update/delete path from those owned UUIDs. A
pre-existing exact key aborts with zero mutations. Auto-created environments
are observed but never mutated directly.

Ambiguous writes are not retried. The only recovery composition uses documented
read endpoints after a zero-match preflight and accepts exactly one exact key.
The report labels this as a workaround. Zero or duplicate matches stop safely.

This finding proves the probe guardrail, not Cloud lifecycle compatibility.

Evidence:
[Offline lifecycle guardrails](evidence/20260731-offline-contracts.md)
