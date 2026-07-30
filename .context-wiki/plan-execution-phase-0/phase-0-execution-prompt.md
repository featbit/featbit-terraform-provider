# New-Session Prompt: Execute Phase 0

Copy the following prompt into a new session opened at the repository root.

```text
Execute Phase 0 of the FeatBit Terraform Provider project: “Empirical API compatibility and ADRs.”

Workspace:
C:\Code\featbit\featbit-terraform-provider

Start by reading, in order:
1. AGENTS.md
2. .context-wiki/plan.md
3. .context-wiki/plan-execution-phase-0/README.md
4. .context-wiki/plan-execution-phase-0/plan.md
5. .context-wiki/plan-execution-phase-0/context-protocol.md
6. .context-wiki/plan-execution-phase-0/status.md
7. .context-wiki/plan-execution-phase-0/todo.md
8. .context-wiki/plan-execution-phase-0/handoff.md
9. Relevant findings, compatibility, ADR, and evidence files linked from them.

Then execute the Phase 0 plan, not merely review or rewrite it.

Hard constraints:
- Treat the current public FeatBit REST API as fixed. Do not require backend changes.
- Use only documented public API endpoints.
- LaunchDarkly is an engineering reference, not a parity target.
- Use API access tokens directly in the Authorization header. Do not implement login, username/password auth, JWT refresh, MFA, or SSO.
- Never print, store, or commit tokens, passwords, environment secret values, private tenant identifiers, or real member emails.
- Use only approved disposable Cloud/self-hosted targets for mutations.
- Do not start Phase 1 or implement production Terraform resources.
- Preserve all pre-existing user changes in the worktree.

Execution requirements:
- Inspect git status before editing.
- Begin with TODOs P0-001 through P0-008 and establish mutation, redaction, and cleanup guardrails before any live write.
- Build reusable probe logic in Go.
- Work through todo.md in dependency order.
- Mark a TODO complete only when its stated evidence and acceptance condition exist.
- Record live observations as sanitized evidence, then connect them to findings and ADRs.
- Test exact identity across all pages; never select the first fuzzy search result.
- On ambiguous behavior, preserve safety and classify the capability as constrained, read/bind only, external, or omitted.
- Keep the compatibility matrix target-specific. Do not claim a target was tested when it was not available.
- Clean up every created object or record an exact owner/action.
- Run relevant formatting, unit, determinism, redaction, and secret checks.

Context maintenance is part of the work:
- Append every working session to session-log.md.
- Keep status.md, todo.md, findings.md, compatibility-matrix.md, evidence/, ADRs, and handoff.md current.
- Do not silently rewrite historical findings or evidence.
- Before ending, update the exact next action in handoff.md.

Credentials and test targets, if available, will be supplied through:
- FEATBIT_TEST_API_URL
- FEATBIT_TEST_SERVICE_TOKEN
- FEATBIT_TEST_PERSONAL_TOKEN
- FEATBIT_TEST_TARGET
- FEATBIT_TEST_RESOURCE_PREFIX

If a required credential or target is unavailable:
- Continue all safe offline/specification/probe/unit-test work.
- Mark only the affected live tasks BLOCKED.
- Do not claim the Phase 0 exit gate passed.
- Report the precise missing prerequisite without requesting or exposing a token value in chat.

Completion condition:
Phase 0 is complete only when the exit gate in plan.md passes, ADR-001 through ADR-005 are accepted, cleanup is complete, status.md says “Complete — Ready for Phase 1,” and handoff.md contains the verified inputs for Phase 1.

In your final response, lead with whether the Phase 0 exit gate passed. Summarize completed task IDs, verified deployment targets, accepted ADRs, reduced/omitted capabilities, cleanup status, tests run, and the exact next action. Link the relevant local context files.
```
