# Cloud Boolean, String, Number, and JSON feature-flag evidence

- Target: `cloud-current`
- Observed: 2026-07-31
- Build: current Cloud deployment; exact build unavailable
- Scope: flags created only inside probe-owned disposable parents

This record consolidates the former Boolean CRUD and String/Number/JSON
type-matrix records:

- `20260731-cloud-current-feature-flag-crud-lifecycle.md`
- `20260731-cloud-current-feature-flag-type-matrix.md`

## Verified lifecycle

Boolean, String, Number, and JSON each completed:

1. active and archived all-page exact-zero preflight;
2. `POST /api/v1/envs/{envId}/feature-flags`;
3. exact-key direct Read;
4. name-only specialized Update;
5. repeated canonical Reads with unrelated fields preserved;
6. archive followed by hard Delete;
7. direct post-delete 404/`ResourceNotFound`; and
8. exact active and archived key counts of zero.

The initial hard Delete of every unarchived type returned 422 with code
`CannotDeleteUnarchivedFeatureFlag`. The safe destroy workflow is therefore
archive, hard Delete, direct Read, then complete active/archived exact-zero
verification. Archive alone is not Terraform destroy completion.

## Type and identity observations

| Type | Canonical rule | Cloud result |
|---|---|---|
| Boolean | lowercase `true`/`false` | exact values and repeated Reads matched |
| String | preserve bytes | exact values and repeated Reads matched |
| Number | parse/canonicalize without `float64` | large integer and decimal precision matched |
| JSON | validate one JSON value; canonicalize key order, whitespace, and decimal spelling | desired/observed and repeated Reads were logically equal |

Every Create supplied two deterministic variation UUIDs and the server
preserved them. Fallthrough referenced the requested enabled variation and the
disabled variation ID was preserved. A revision was present and advanced after
the name-only update. Variations are mapped by stable UUID, never list index.

## Ownership boundary

Provider v1 owns key, type, description, and variations at Create; changes to
those fields replace the flag. Only name updates in place. Targeting, rules,
rollouts, enabled state, and tags remain UI-owned and are Computed-only or
omitted. The live type matrix did not write any of those operational fields or
post-Create variations.

Child duplicate error codes, stale-revision writes, restore, and key reuse were
not tested and are not implementation dependencies. Exact-zero preflight,
single-write behavior, exact reconciliation, replace-only fields, and omitted
restore/key-reuse guarantees fail closed.

## Recovery and cleanup

The first Boolean unarchived-Delete failure kept the exact flag, environment,
and project in the ignored cleanup inventory. Recovery archived/deleted the
flag, deleted its parents in dependency order, and ended at `pending=0`.
String, Number, and JSON ran sequentially; each was exactly absent before the
next Create. All parents were removed and no manual cleanup remains.

No token, secret, runtime identity, variation value, tenant identifier, raw
body, or header value is retained.

## Reproduce

```text
cd tools/api-probe
go run ./cmd/featbit-api-probe project-env-lifecycle --execute --feature-flag-crud-checks
go run ./cmd/featbit-api-probe project-env-lifecycle --execute --feature-flag-type-matrix-checks --timeout 30s
go run ./cmd/featbit-api-probe cleanup --dry-run
```
