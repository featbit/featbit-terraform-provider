# Cloud authentication and authorization evidence

- Target: `cloud-current`
- Observed: 2026-07-30 through 2026-07-31
- Build: current Cloud deployment; exact build unavailable
- Scope: read-only authentication behavior; no mutation

This record consolidates the former `20260730-cloud-current-auth-negative.md`
and `20260731-cloud-current-authenticated-project-list.md` records.

## Sanitized observations

| Case | Public request | HTTP/envelope | Decision |
|---|---|---|---|
| Valid permission-scoped access token | `GET /api/v1/projects` with the token value directly in `Authorization` | 200, `success=true`, array data | Supported; no organization/workspace header required |
| Missing header | same GET without `Authorization` | 401, `success=false`, `data=null`, code `Unauthorized` | Authentication error; preserve state; no retry |
| Synthetic malformed value | same GET with a non-secret synthetic value | 401, `success=false`, `data=null`, code `Unauthorized` | Authentication error; preserve state; no retry |

Personal/service labels do not create separate provider authentication
contracts. The provider exposes one Sensitive access token and effective
permissions come from that token. It implements no login, username/password,
JWT refresh, MFA, SSO, token-kind selector, or account-context header.

Inactive and deliberately restricted tokens were unavailable. Provider code
must therefore avoid depending on their response body or error code: every 401
is authentication failure and every 403 is authorization failure; both
preserve state and no mutation is retried.

## Redaction and cleanup

No header value, token, response body, resource identity, tenant context,
environment secret, or member email was retained. These were read-only probes;
cleanup was not applicable.

## Reproduce

Use the reusable probe with credentials supplied out of band. Never print the
environment variable value:

```text
cd tools/api-probe
go run ./cmd/featbit-api-probe auth-negative --case missing
go run ./cmd/featbit-api-probe auth-negative --case malformed
go run ./cmd/featbit-api-probe projects-list --token-kind service
```
