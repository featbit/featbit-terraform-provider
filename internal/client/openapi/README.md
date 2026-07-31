# Pinned FeatBit OpenAPI input

This directory contains Phase 0 generation inputs. It does not contain
production Terraform resource code.

| File | Purpose |
|---|---|
| `openapi.lock.json` | Immutable source URL, byte length, and SHA-256 |
| `featbit.openapi.json` | Exact upstream bytes |
| `overlay.json` | OpenAPI Overlay 1.1.0 actions adding provider-owned operation IDs only |
| `featbit.overlayed.openapi.json` | Deterministic generated input for the future typed client |
| `inventory.json` | Deterministic counts and security-scheme inventory |
| `generator.lock.json` | Exact future Go client generator module/version |
| `oapi-codegen.yaml` | Phase 1 typed client configuration |

The upstream snapshot is never edited manually. The overlay must not invent an
endpoint, parameter, schema constraint, or server behavior.

From `tools/api-probe/`, reproduce and verify:

```text
go run ./cmd/openapi-tool pin
go run ./cmd/openapi-tool generate
go run ./cmd/openapi-tool check
```

`pin` refuses content whose length or SHA-256 differs from the lock.
`generate` validates that every overlay target exists and every operation ID is
non-empty and unique. `check` performs the same generation in memory and fails
if either generated artifact differs.

The Overlay Specification input is pinned to `1.1.0`. The applying tool is
repository-owned Go code in `tools/api-probe/internal/openapi`; its exact
version is therefore the repository commit rather than a floating executable.

Phase 1 will add the locked `oapi-codegen v2.8.0` command to its Go tools
module and generate from `featbit.overlayed.openapi.json`. Phase 0 deliberately
does not commit a production generated client or start provider scaffolding.
