# FeatBit Phase 0 API probe

This Go tool records sanitized behavior of documented FeatBit REST API
endpoints. It is a Phase 0 compatibility probe, not production provider code.

## Configuration

All credentials and target configuration are read from environment variables.
The probe never accepts a token as a command-line argument.

| Variable | Required for reads | Required for writes | Meaning |
|---|---:|---:|---|
| `FEATBIT_TEST_API_URL` | Yes | Yes | Approved target base URL |
| `FEATBIT_TEST_SERVICE_TOKEN` | Yes | Yes | Permission-scoped access token; the variable name is retained for the Phase 0 harness |
| `FEATBIT_TEST_PERSONAL_TOKEN` | No | No | Legacy comparison input; not required because token labels share one provider authentication contract |
| `FEATBIT_TEST_TARGET` | Yes | Yes | `cloud-current` or `selfhosted-min` |
| `FEATBIT_TEST_RESOURCE_PREFIX` | No | Yes | Unique prefix matching `tfp0-<lowercase-safe-suffix>` |

Token values are sent directly in the `Authorization` header. The probe does
not implement login, password authentication, JWT refresh, MFA, SSO, or
additional organization/workspace context headers.

## Mutation interlock

A mutating command is rejected unless all of these conditions hold:

1. the target ID is recognized;
2. the resource prefix is non-empty, starts with `tfp0-`, and passes the
   conservative character/length checks;
3. `cloud-current` uses exactly the documented FeatBit Cloud API host over
   HTTPS, or `selfhosted-min` resolves to a loopback/private address;
4. an API access token is present in an environment variable; and
5. the caller explicitly selects a mutating command.

These checks make an accidental write fail closed. A public self-hosted test
host requires a reviewed safety-rule change before it can be mutated.

## Cleanup inventory

The probe persists every created identity immediately to
`.featbit-api-probe-cleanup.json`, which is ignored by Git. Cleanup is planned
in dependency order (flags and segments, then environments, then projects).
If Cloud rejects deletion of an unarchived flag, cleanup uses the documented
archive endpoint as a prerequisite, retries hard Delete once, and verifies
zero exact keys in both active and archived list views.
Use the cleanup dry run before execution:

```text
go run ./cmd/featbit-api-probe cleanup --dry-run
```

The inventory contains test resource IDs/keys only. It must never contain
tokens, environment secret values, tenant IDs, passwords, or member emails.

## Safe verification

```text
go test ./...
go test -race ./...
go run ./cmd/featbit-api-probe config
go run ./cmd/featbit-api-probe projects-list --token-kind service
go run ./cmd/featbit-api-probe auth-negative --case missing
go run ./cmd/featbit-api-probe auth-negative --case malformed
```

The `config` command reports only variable presence, target class, and whether
the mutation interlock passed. It never prints configuration values.

`projects-list` is the minimal read-only authentication/envelope probe. It
prints only method/path template, HTTP status, envelope success, data shape,
structured error codes, and a redacted `Retry-After` value. It never prints the
raw response or headers.

`auth-negative` is a read-only exception used to observe missing and
synthetic-malformed authentication against the exact documented Cloud host. It
cannot accept a URL or token, so it cannot disclose a credential or be
redirected to another target.

## Project/environment lifecycle

`project-env-lifecycle` performs one isolated lifecycle and requires an
explicit `--execute`. It accepts no project or environment ID arguments.

Before creating anything it confirms that the exact project key derived from
`FEATBIT_TEST_RESOURCE_PREFIX` has zero matches. It then:

1. creates one prefixed project and immediately records the returned UUID;
2. reads and updates only that UUID;
3. creates one additional environment under that project and immediately
   records its returned UUID;
4. confirms that UUID belongs to the new project before updating it;
5. deletes the additional environment and verifies exact absence;
6. deletes the new project and verifies exact absence; and
7. requires the cleanup inventory to finish with zero pending entries.

The command never mutates an auto-created environment or a pre-existing
resource. A create response that is ambiguous is not retried. Because the key
was proven absent before creation, the probe may instead reconcile exactly one
matching key through the documented read endpoint; the report labels that path
as a workaround.

```text
go run ./cmd/featbit-api-probe project-env-lifecycle --execute
```

The opt-in `--compatibility-checks` mode additionally sends an empty-name
create before each valid create, submits each newly created key a second time,
and directly reads each deleted ID. Every write still uses only the run's new
project/environment key and IDs. If an invalid or duplicate request
unexpectedly creates an object, the probe adopts its exact returned/listed ID
into the cleanup inventory; a second distinct identity stops the lifecycle so
cleanup can run. A non-successful direct post-delete Read is explicitly
labelled with the documented exact parent/complete-collection fallback used to
confirm absence.

```text
go run ./cmd/featbit-api-probe project-env-lifecycle --execute --compatibility-checks
```

The opt-in `--child-read-checks` mode keeps the same project/environment
lifecycle but makes no flag or segment mutation. Inside the newly created
environment it:

1. directly reads one fresh synthetic missing feature-flag key;
2. traverses every flag page and requires zero exact key matches;
3. directly reads one deterministic synthetic missing segment UUID;
4. traverses every segment page and requires zero exact UUID and key matches;
   and
5. deletes only the new environment/project parents.

The direct-read classifications and exact-zero fallbacks are serialized
without the concrete key, UUID, parent identity, response body, or headers.
Tests reject any non-GET request whose path contains `/feature-flags` or
`/segments`.

```text
go run ./cmd/featbit-api-probe project-env-lifecycle --execute --child-read-checks
```

This is the only child-resource mode that can fit an approval limited to
project/environment mutations. Review the exact sequence for the target before
running it.

The narrower `--feature-flag-crud-checks` mode creates exactly one Boolean
flag, reads it, changes only its name, reads it again, and destroys it. It does
not submit a duplicate Create or stale-revision write. Cloud requires the
documented archive operation before hard Delete; exact absence scans every
page in both `IsArchived=false` and `IsArchived=true` views.

```text
go run ./cmd/featbit-api-probe project-env-lifecycle --execute --feature-flag-crud-checks
```

No current Cloud approval remains for this mode. Every new run requires a
separately reviewed scope.

The `--feature-flag-type-matrix-checks` mode reuses one newly created project
and environment, then sequentially creates, reads, changes only the name of,
and destroys exactly three flags: String, Number, and JSON. Each flag uses a
different exact prefixed key and must be absent from both active and archived
views before the next type starts. Values are compared canonically without
`float64` conversion: strings remain exact, numbers retain decimal precision,
and JSON normalizes whitespace, object-key order, and number spelling. The
mode never writes targeting, rules, rollouts, tags, enabled state, or
variations after Create.

```text
go run ./cmd/featbit-api-probe project-env-lifecycle --execute --feature-flag-type-matrix-checks
```

This mode has offline/mock approval only. It creates three child objects and
must not run on Cloud without a separately reviewed explicit scope.

The narrower `--segment-crud-checks` mode creates exactly one
environment-specific segment under the newly created environment. It exercises
the documented name, description, targeting, tags, archive, restore, reference
preflight, and Delete operations on that same exact ID. It does not submit the
duplicate Create used by `--segment-checks`. Exact preflight and absence checks
scan every page in both `IsArchived=false` and `IsArchived=true` views. Create
requires one environment resource-name scope. Because the public API has no
organization-key read, the probe direct-reads every visible exact segment UUID,
requires one unique organization prefix, keeps it only in memory, and fails
closed otherwise. Cloud destroy archives the segment after an empty reference
preflight, then hard-deletes it and proves exact absence in both views.

```text
go run ./cmd/featbit-api-probe project-env-lifecycle --execute --segment-crud-checks
```

No current Cloud approval remains for this mode. Every new run requires a
separately reviewed scope. Shared segments and pre-existing objects are
excluded from mutation.

The separately opt-in `--feature-flag-checks` mode is an offline-reviewed
future live probe. It creates the same isolated project/environment parents,
then uses only that new environment for one boolean flag. It:

1. scans every advertised list page and requires zero exact key matches;
2. creates the flag and inventories its scoped `environment/key` identity;
3. probes a duplicate key and requires exactly the original identity;
4. changes only the name through the documented specialized endpoint and
   verifies that unrelated canonical fields did not change;
5. if the name change advances the revision, submits the same variation values
   with the stale revision so even an unexpected acceptance is logically
   harmless;
6. archives the flag, hard-deletes it, and requires all-page exact-key counts
   of zero in both active and archived views; and
7. deletes its new environment/project parents through the normal cleanup path.

Ambiguous flag creation is never retried. It may be adopted only after the
zero-match preflight and exactly one all-page exact-key result. Failure leaves
the flag and both owned parents in dependency-ordered cleanup inventory.

```text
go run ./cmd/featbit-api-probe project-env-lifecycle --execute --feature-flag-checks
```

This mode must not be run on Cloud under the current approval; feature-flag
production mutations require a separately reviewed explicit scope or a
disposable non-production target.

The separately opt-in `--segment-checks` mode is also an offline-reviewed
future live probe. It creates the same isolated project/environment parents
and manages one environment-specific segment only inside that new
environment. It:

1. traverses every advertised active and archived list page and requires zero
   exact key matches;
2. creates and immediately inventories the exact `environment/segment-ID`;
3. probes a duplicate key and requires the original canonical identity;
4. uses only the documented name, description, targeting, tags, archive, and
   restore operations, checking unrelated fields after each write;
5. checks the documented flag-reference collection and refuses Delete when it
   is non-empty;
6. after the reference preflight is empty, archives then hard-deletes the exact
   segment and requires both exact ID and exact key counts to be zero across
   every active and archived page; and
7. deletes its new environment/project parents through the normal cleanup
   path.

Ambiguous creation is never retried. The generic PATCH is excluded because the
pinned OpenAPI has no request schema. Shared segments are also excluded from
this command: their scopes may cross environments, projects, or organizations,
and the public schema does not define the scope-string encoding needed to
contain a shared mutation safely. Environment-specific Create uses the
fail-closed exact-read scope resolver described above.

```text
go run ./cmd/featbit-api-probe project-env-lifecycle --execute --segment-checks
```

This mode must not be run on Cloud under the current approval; segment
production mutations require a separately reviewed explicit scope or a
disposable non-production target.

Do not run this command until its request sequence has been reviewed for the
approved target.
