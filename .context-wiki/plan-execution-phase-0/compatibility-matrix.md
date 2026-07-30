# FeatBit API Compatibility Matrix

Status: **Not tested**  
Last updated: **2026-07-30**

Do not mark a cell supported without linked evidence. Use `Not tested`, `Supported`, `Constrained`, `Unsupported`, or `Not applicable`.

## Deployment targets

| Target ID | Deployment | FeatBit version/build | API URL class | Test date | Status | Evidence |
|---|---|---|---|---|---|---|
| cloud-current | FeatBit Cloud test tenant | Current at test time | Cloud | Pending | Not tested | Pending |
| selfhosted-min | Self-hosted minimum supported release | To be selected by ADR-005 | Local/disposable | Pending | Not tested | Pending |

Never record private tenant names, tokens, or full private URLs.

## High-level behavior

| Capability | cloud-current | selfhosted-min | Notes/evidence |
|---|---|---|---|
| Service access token | Not tested | Not tested | |
| Personal access token | Not tested | Not tested | Conditional on availability |
| Token-selected context | Not tested | Not tested | |
| Project lifecycle | Not tested | Not tested | |
| Environment lifecycle | Not tested | Not tested | |
| Feature flag lifecycle | Not tested | Not tested | |
| Segment lifecycle | Not tested | Not tested | |
| Member exact-email reconciliation | Not tested | Not tested | |
| Environment secret metadata | Not tested | Not tested | Values must be redacted |

## Error and existence behavior

| Scenario | cloud-current HTTP/envelope | selfhosted-min HTTP/envelope | Provider classification | Evidence |
|---|---|---|---|---|
| Missing token | Not tested | Not tested | Authentication error | |
| Invalid token | Not tested | Not tested | Authentication error | |
| Insufficient permission | Not tested | Not tested | Authorization error | |
| Validation failure | Not tested | Not tested | User diagnostic | |
| Duplicate identity | Not tested | Not tested | Conflict/user diagnostic | |
| Missing project | Not tested | Not tested | Exact fallback required | |
| Missing environment | Not tested | Not tested | Exact fallback required | |
| Missing flag | Not tested | Not tested | Exact fallback required | |
| Missing segment | Not tested | Not tested | Exact fallback required | |
| Stale revision | Not tested | Not tested | Refresh/no unsafe retry | |
| Rate limit | Not tested | Not tested | Safe-read retry only | Live probe optional |
| Transient server failure | Not tested | Not tested | Safe-read retry only | Mock acceptable |

## Normalization behavior

| Behavior | cloud-current | selfhosted-min | Canonical provider rule | Evidence |
|---|---|---|---|---|
| Project defaults | Not tested | Not tested | Pending | |
| Auto-created environments | Not tested | Not tested | Pending | |
| Environment defaults | Not tested | Not tested | Pending | |
| Variation ID stability | Not tested | Not tested | Pending | |
| Enabled/fallthrough mapping | Not tested | Not tested | Pending | |
| Rule/condition ordering | Not tested | Not tested | Pending | |
| JSON normalization | Not tested | Not tested | Pending | |
| Segment type/scopes | Not tested | Not tested | `RequiresReplace` candidate | |
| Secret metadata/types | Not tested | Not tested | Pending | |

## Compatibility conclusion

Not evaluated. ADR-005 must define the supported matrix and how an untested or incompatible version is reported to users.
