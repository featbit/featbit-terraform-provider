# Phase 0 Evidence

Store only sanitized, reproducible observations here. Evidence supports findings and ADRs; it is not a dump of raw HTTP traffic.

## Filename convention

```text
YYYYMMDD-<target-id>-<topic>.md
YYYYMMDD-<target-id>-<topic>.json
```

Examples:

```text
20260731-cloud-current-project-not-found.md
20260731-selfhosted-min-flag-roundtrip.json
```

## Required metadata

Every evidence record must include:

- Related TODO ID
- UTC timestamp
- Target ID from `compatibility-matrix.md`
- FeatBit version/build when known
- Probe commit or working-tree identifier
- Request method and path template
- Preconditions
- Expected behavior
- Observed HTTP status
- Observed `success`, data shape, and redacted errors
- Exact-match or normalization result
- Created-resource identities in sanitized form
- Cleanup result
- Redactions performed
- Reproduction command using environment-variable names only

## Evidence template

```markdown
# <Topic>

- TODO: `P0-000`
- Timestamp (UTC): `YYYY-MM-DDTHH:MM:SSZ`
- Target: `cloud-current | selfhosted-min`
- FeatBit version/build: `<value or unknown>`
- Probe revision: `<commit or dirty identifier>`

## Preconditions

<Describe target state and permissions without private identifiers.>

## Request

- Method: `GET`
- Path template: `/api/v1/...`
- Authentication: `Authorization: <REDACTED>`

## Expected

<Expected result.>

## Observed

- HTTP status: `<code>`
- Envelope success: `<true|false|missing>`
- Data shape: `<sanitized description>`
- Errors: `<sanitized description>`

## Interpretation

<Exact provider behavior supported by this observation.>

## Cleanup

<Deleted, not applicable, or exact remaining cleanup action and owner.>

## Reproduce

`<command containing environment-variable names only>`
```

## Redaction rules

Never store:

- `Authorization` header values
- Access tokens, JWTs, passwords, or cookies
- Environment secret values or SDK keys
- Full private tenant, organization, or workspace identifiers
- Real member email addresses
- Unredacted request/response dumps

Use deterministic placeholders such as `<TOKEN>`, `<TENANT>`, `<MEMBER_EMAIL>`, and `<SECRET_VALUE>`. Retain only data shapes and non-sensitive randomly generated test IDs needed to establish identity behavior.

## Immutability

After an evidence record is referenced by a finding or ADR, do not silently change its observation. Create a new dated file and mark the old finding superseded.

## Cleanup inventory

Maintain this table during execution. The phase cannot close with an unowned entry.

| Created at | Target | Resource type | Sanitized identity | Creating TODO | Cleanup status | Owner/action |
|---|---|---|---|---|---|---|
| — | — | — | — | — | Empty | — |
