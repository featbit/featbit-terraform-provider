# New-Session Prompt: Execute Phase 1

Use this prompt only after Phase 0 is complete and its handoff says `Ready for Phase 1`.

```text
Execute Phase 1 of the FeatBit Terraform Provider project: “Repository and provider scaffold.”

Workspace:
C:\Code\featbit\featbit-terraform-provider

This is an implementation session. Complete the Phase 1 scaffold and client foundation, verify it, and maintain the repository-backed context history.

Read first, in order:
1. AGENTS.md
2. .context-wiki/plan.md
3. .context-wiki/plan-execution-phase-0/README.md
4. .context-wiki/plan-execution-phase-0/status.md
5. .context-wiki/plan-execution-phase-0/handoff.md
6. All accepted Phase 0 ADRs
7. .context-wiki/plan-execution-phase-0/findings.md
8. .context-wiki/plan-execution-phase-0/compatibility-matrix.md
9. Phase 0 evidence linked by the handoff

Precondition:
- Verify that the Phase 0 exit gate passed.
- Verify that ADR-001 through ADR-005 are Accepted.
- Verify that the handoff identifies the pinned OpenAPI snapshot/overlay, error classification, auth contract, Import identities, compatibility scope, and constrained/omitted capabilities.
- If any precondition is not met, do not invent the missing contract or start material Phase 1 implementation. Report the exact blocker and update the handoff.

Before implementation:
1. Inspect git status and preserve all pre-existing user changes.
2. Create .context-wiki/plan-execution-phase-1/ with:
   - README.md
   - plan.md
   - todo.md
   - status.md
   - session-log.md
   - findings.md
   - evidence/README.md
   - handoff.md
3. Follow the same cross-session protocol defined in Phase 0.
4. Link the new Phase 1 package from .context-wiki/plan.md.

Implement only Phase 1 scope:

A. Repository and toolchain
- Initialize the Go module for github.com/featbit/terraform-provider-featbit.
- Use the Go, Terraform Plugin Framework, Plugin Go, Plugin Testing, Plugin Log, and Protocol v6 versions accepted by ADR-005/master plan; re-verify versions only if the baseline is stale.
- Start from HashiCorp’s current Plugin Framework scaffold patterns.
- Do not copy LaunchDarkly source code. Use it only to understand mature provider behavior.
- Add the agreed license, .gitignore, editor configuration, and deterministic tool pinning.
- Add version injection and a Protocol v6 provider server entry point.

B. Provider configuration
- Implement api_url with FEATBIT_API_URL fallback.
- Implement Sensitive access_token with FEATBIT_ACCESS_TOKEN fallback.
- Implement http_timeout_seconds, max_concurrency, and max_retries according to the accepted plan.
- Support personal and service API access tokens; recommend service tokens for CI/CD.
- Send the token directly as the Authorization header value.
- Do not implement login, username/password, Bearer JWT refresh, MFA, SSO, organization headers, or workspace headers unless an accepted Phase 0 ADR explicitly requires them.
- Validate configuration and ensure no credential can appear in diagnostics or logs.

C. OpenAPI and client foundation
- Place the pinned Phase 0 OpenAPI snapshot and overlay at the ADR-003 paths.
- Pin the generator and implement a deterministic generate command.
- Keep generated transport/types isolated from the handwritten client wrapper.
- Implement:
  - Base URL and /api/v1 path handling
  - Access-token authentication
  - FeatBit {success,data,errors} envelope handling
  - The Phase 0 error-classification decision table
  - Interfaces/helpers for exact-identity absence checks
  - Pagination
  - Timeout and context cancellation
  - Safe-read-only retry with exponential backoff, jitter, and Retry-After support
  - Conservative concurrency control
  - terraform-provider-featbit/<version> User-Agent
  - Complete token/secret redaction
- Do not add behavior unsupported by Phase 0 evidence.

D. Tests
- Add provider schema/configuration unit tests covering HCL values, environment fallbacks, Null/Unknown values, URL validation, and missing credentials.
- Add client tests for headers, base path, envelopes, 401/403, authoritative Not Found, ambiguous errors, exact-fallback interfaces, pagination, cancellation, rate-limit/transient retry, no retry for unsafe writes, and redaction.
- Use mock HTTP servers and sanitized fixtures; unit tests must not require live credentials.
- Prove that two generation runs produce a clean diff.
- Prove logs and diagnostics contain no token or environment secret values.

E. Developer workflow
- Add GNUmakefile targets for fmt, lint, generate, test, testacc, and build.
- Add gofmt/go vet/test/lint/govulncheck configuration with pinned tool versions where appropriate.
- Add a local provider-development/override guide.
- Add the initial CI workflow needed to run formatting, generation-diff, unit, race, and security checks safely.
- Do not expose long-lived tokens to fork pull requests.

Explicitly out of Phase 1:
- Do not implement featbit_project, featbit_environment, featbit_feature_flag, featbit_segment, or IAM resources/data sources. Those belong to later phases.
- Do not publish to the Terraform Registry.
- Do not expand scope to match LaunchDarkly.
- Do not change the FeatBit backend or use undocumented Portal endpoints.

Required validation:
- gofmt on all Go files
- go vet ./...
- go test -race ./...
- deterministic generation check
- terraform providers schema -json through a local developer override
- repository secret/redaction check
- any additional checks required by AGENTS.md or accepted ADRs

Phase 1 exit gate:
- A local developer override can load the provider.
- terraform providers schema -json succeeds.
- Provider configuration and mock API client tests pass.
- Generated code/document inputs are reproducible.
- Sensitive values are absent from logs and diagnostics.
- The Phase 1 context package is complete and its handoff identifies the exact entry point for Phase 2.

Work autonomously within this scope. When a reversible implementation detail is not public API, choose the simplest tested design and record it. Stop and ask only if a choice would change an accepted ADR, public provider schema, license, supported compatibility range, or requires new external authority.

In your final response, lead with whether the Phase 1 exit gate passed. Summarize implemented files, checks run, deviations from the accepted Phase 0 decisions, remaining blockers, and the next Phase 2 action. Link the Phase 1 status and handoff files.
```
