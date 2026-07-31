# Cloud project, environment, validation, and absence evidence

- Target: `cloud-current`
- Observed: 2026-07-31
- Build: current Cloud deployment; exact build unavailable
- Scope: disposable probe-owned project/environment parents only

This record consolidates the former project/environment lifecycle,
compatibility, and child-read records:

- `20260731-cloud-current-project-environment-lifecycle.md`
- `20260731-cloud-current-project-environment-compatibility.md`
- `20260731-cloud-current-child-read-compatibility.md`

## Lifecycle result

One uniquely prefixed project completed Create, exact Read, name Update,
repeated canonical Read, and Delete through documented public endpoints. The
server created two automatic environments (`Dev/dev` and `Prod/prod`). One
additional explicit environment completed Create, exact parent membership
verification, exact Read, name/description Update, repeated canonical Read,
and Delete.

Project key and environment parent/key are replace-only. Project automatic
environments are Computed. Project Delete cascaded its automatic environments.

Every observed environment contained two secret metadata objects with type
classes `Client` and `Server`. Secret values were discarded before report
serialization and must not enter ordinary Terraform state.

## Validation, duplicate, and absence shapes

| Scenario | Public operation | Observed | Safe provider behavior |
|---|---|---|---|
| Empty project name | `POST /api/v1/projects` | 400, code `name_is_required` | user diagnostic; no retry |
| Duplicate project key | same POST | 422, code `KeyHasBeenUsed`; exact identity remained one | preserve existing object; no adoption/retry |
| Empty environment name | `POST /api/v1/projects/{projectId}/envs` | 400, code `name_is_required` | user diagnostic; no retry |
| Duplicate environment key | same POST | 422, code `KeyHasBeenUsed`; exact identity remained one | preserve existing object; no adoption/retry |
| Environment after Delete | direct exact GET | 403, code `Forbidden` | preserve until parent environment collection has exact ID count zero |
| Project after Delete | direct exact GET | 500, code `InternalServerError` | preserve until complete project collection has exact ID count zero |
| Missing feature flag | exact GET by synthetic key | 404, code `ResourceNotFound` | preserve until every flag page has exact key count zero |
| Missing segment | exact GET by synthetic UUID | 404, code `ResourceNotFound` | preserve until every segment page has exact UUID count zero |

A newly created environment exposed three pre-existing segment rows. Parent
creation therefore does not imply an empty child collection or grant ownership
of visible segments. Every lookup must traverse all pages and match exact
identity; fuzzy-first selection is forbidden.

## Cleanup

All explicit and automatic environments and every created project were
deleted. Direct results were followed by the exact collection fallback.
Cleanup finished at `pending=0`; no manual owner/action remains.

## Redactions

No token, authorization value, environment secret value, runtime UUID/key,
tenant identifier, response body, or member email is retained.

## Reproduce

With named `FEATBIT_TEST_*` variables supplied out of band:

```text
cd tools/api-probe
go run ./cmd/featbit-api-probe project-env-lifecycle --execute
go run ./cmd/featbit-api-probe project-env-lifecycle --execute --compatibility-checks
go run ./cmd/featbit-api-probe project-env-lifecycle --execute --child-read-checks
go run ./cmd/featbit-api-probe cleanup --dry-run
```
