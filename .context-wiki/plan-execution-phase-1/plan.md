# Phase 1 Plan — Repository and Provider Scaffold

## Objective

Build a loadable Terraform Plugin Framework/Protocol v6 provider scaffold and
the generated-plus-handwritten FeatBit API client foundation. All behavior must
implement the accepted Phase 0 contract without backend changes or
undocumented endpoints.

## Accepted inputs

- [Phase 0 handoff](../plan-execution-phase-0/handoff.md) is the concise
  implementation contract.
- [ADR-001 through ADR-005](../plan-execution-phase-0/adrs/README.md) are
  Accepted and take precedence over older planning text.
- The pinned OpenAPI inputs remain in
  [internal/client/openapi](../../internal/client/openapi/README.md).
- Authentication is one Sensitive access token sent directly in
  `Authorization`; personal/service labels do not create separate provider
  schema types.
- Cloud is empirically verified. Self-hosted remains configurable through
  `api_url` but no exact self-hosted release is certified.

## Workstreams

### 1. Repository and toolchain

- Normalize the root Go module as
  `github.com/featbit/terraform-provider-featbit`.
- Pin the Go, Terraform Plugin Framework, Plugin Go, Plugin Testing, Plugin
  Log, Plugin Docs, `oapi-codegen`, Terraform CLI, and Protocol v6 versions
  accepted by ADR-005.
- Add the provider server entry point and version injection.
- Use MPL-2.0 for this provider. Preserve notices on any scaffold-derived
  source, use HashiCorp patterns as reference, and do not copy LaunchDarkly
  source. P1-011 adds the license text and source notices.

### 2. Provider configuration

- `api_url`, with `FEATBIT_API_URL` fallback and documented Cloud default.
- Sensitive `access_token`, with `FEATBIT_ACCESS_TOKEN` fallback.
- `http_timeout_seconds`, `max_concurrency`, and `max_retries` using bounded,
  tested defaults.
- No login, username/password, Bearer/JWT refresh, MFA, SSO, organization
  header, workspace header, or token-kind selector.

P1-022 fixes the configuration defaults and accepted ranges:

| Setting | Default | Accepted range |
|---|---:|---:|
| `http_timeout_seconds` | `30` | `1` through `300` |
| `max_concurrency` | `4` | `1` through `32` |
| `max_retries` | `3` | `0` through `10` |

The retry value is configuration for the later safe-read-only retry executor;
it never authorizes automatic mutation retries. The concurrency value is
configuration for the later process-local request limiter.

### 3. OpenAPI transport and handwritten client

- Generate typed transport/models from the pinned operation-ID overlay.
- Keep generated code isolated and never edit it manually.
- Implement base URL and `/api/v1` handling, direct token auth, User-Agent,
  envelope decoding, error classification, pagination, exact-identity helper
  interfaces, timeout/cancellation, bounded concurrency, safe-read-only retry,
  and complete redaction.
- Never retry a mutation automatically or treat a direct error as confirmed
  absence without the Phase 0 exact fallback.

### 4. Tests

- Provider configuration: HCL values, environment fallbacks, Null/Unknown,
  URL validation, defaults, and missing credentials.
- Client: headers, base paths, envelopes, 401/403, ambiguous absence,
  pagination, cancellation, retry policy, unsafe-write no-retry, concurrency,
  User-Agent, and redaction.
- Use mock HTTP servers and synthetic fixtures only; no live credential is
  required for Phase 1 tests.
- Run generation twice and prove a clean diff.

### 5. Developer workflow

- Add `GNUmakefile` targets for formatting, linting, generation, unit tests,
  acceptance tests, and build.
- Add pinned lint/security tooling, CI, a local provider override guide, and a
  fork-safe secret policy.
- Run the race suite in an environment with a C compiler; the Phase 0 Windows
  workstation could not run it.

## Confirmed Phase 1 module and package layout

P1-005 fixes the package boundaries before generation:

```text
.
├─ main.go
├─ go.mod                         # github.com/featbit/terraform-provider-featbit
├─ go.sum
├─ terraform-registry-manifest.json
├─ internal/
│  ├─ provider/                   # Framework provider/configuration and tests
│  └─ client/
│     ├─ generated/
│     │  └─ client.gen.go         # generated only; never edited manually
│     ├─ openapi/                 # retained pinned snapshot, overlay, and locks
│     └─ *.go                     # handwritten transport wrapper and tests
├─ tools/
│  ├─ go.mod                      # module tools; pinned build/generation tools
│  ├─ go.sum
│  ├─ tools.go
│  └─ api-probe/                  # retained independent Phase 0 module
├─ GNUmakefile
└─ .github/workflows/
```

- The module identities are
  `github.com/featbit/terraform-provider-featbit`, `tools`, and the retained
  `github.com/featbit/terraform-provider-featbit/tools/api-probe`. They are
  independent; do not add a committed `go.work`.
- `internal/provider` depends only on the handwritten `internal/client`
  contract. Only `internal/client` may import `internal/client/generated`.
- Generation runs from `internal/client/openapi` and writes the one committed
  output already declared by `oapi-codegen.yaml` at
  `internal/client/generated/client.gen.go`.
- The tooling module pins generators and documentation tools without adding
  them to the provider runtime dependency graph. The Phase 0 probe is never
  imported by either Phase 1 module.
- Add resource-specific model or validator packages only when a later phase
  has real consumers; Phase 1 does not create empty placeholder packages or
  scaffold example resources, data sources, actions, functions, or ephemeral
  resources.

## Execution order

1. Preserve the current worktree and record the Phase 1 baseline.
2. Resolve license and toolchain inputs.
3. Build the minimal Protocol v6 provider and configuration schema.
4. Generate the OpenAPI client and implement the handwritten wrapper.
5. Add tests, generation checks, developer commands, and CI.
6. Verify the exit gate and update README/TODO with the Phase 2 entry point.

## Explicitly out of scope

- Project, Environment, Feature Flag, Segment, IAM resources or data sources.
- Terraform Registry publication.
- FeatBit backend changes, Portal-private endpoints, database access, or a
  generic raw-REST resource.
- LaunchDarkly schema/resource parity.

## Exit gate

Phase 1 passes only when:

- the local override loads the provider and `terraform providers schema -json`
  succeeds;
- configuration and mock client tests pass, including race and redaction;
- generated inputs and outputs are reproducible;
- secrets are absent from logs, diagnostics, fixtures, and repository scans;
- every completed TODO has verifiable output; and
- README/TODO identify the exact Phase 2 starting point without an unresolved
  public schema or client-contract assumption.
