# Cross-Session Context Protocol

## Goal

This protocol lets a new session reconstruct the current project state without relying on chat history. `.context-wiki/` is a repository-backed context graph: tasks link to evidence, evidence supports findings, findings inform ADRs, and the handoff points to the next executable action.

## Session start

At the beginning of every Phase 0 session:

1. Read the files in the order defined by [README.md](README.md).
2. Inspect `git status` and preserve pre-existing user changes.
3. Confirm the task IDs being attempted and record them in a new `session-log.md` entry.
4. Confirm which deployment target is available:
   - FeatBit Cloud test tenant
   - Pinned self-hosted version
   - Documentation/specification only
5. Confirm that credentials are supplied only through environment variables and will not be printed.
6. Check the cleanup inventory before creating any remote object.

If a required test environment or credential is unavailable, complete safe offline work and mark the affected task `BLOCKED` with the missing prerequisite. Do not label unexecuted behavior as verified.

## During a session

Use the following traceability chain:

```text
TODO ID
  -> sanitized evidence
  -> finding
  -> ADR or support-level decision
  -> status and handoff
```

Rules:

- A TODO remains unchecked until its stated evidence and acceptance condition exist.
- Evidence filenames use `YYYYMMDD-<target>-<topic>.<md|json>`.
- Each evidence record identifies the task ID, FeatBit deployment/version, request method and path, observed HTTP/envelope behavior, cleanup result, and redactions.
- Use relative Markdown links so the context remains portable.
- Label statements as `Verified`, `Hypothesis`, `Decision`, or `Superseded`.
- Never paste raw credentials, environment secret values, passwords, private account identifiers, or unredacted response headers.
- Prefer a reusable Go probe or test over one-off manual requests. Record the exact command with environment-variable names, never values.
- Record unexpected behavior before changing the intended provider design.

## Interacting with the user

Continue with a documented assumption when it is reversible, limited to the current phase, and cannot change the public provider contract.

Stop and ask the user when:

- A choice would change a public resource name, schema, Import ID, or ownership boundary.
- Testing requires a new privileged credential, production tenant, destructive target, or additional external system.
- Existing evidence contradicts an accepted ADR.
- The only apparent workaround uses an undocumented API, direct database access, or another unsupported mechanism.

When asking, add a `Decision needed` entry to `status.md` and `handoff.md` containing:

- The exact question.
- Known evidence.
- Options and tradeoffs.
- The recommended option.
- Which task is blocked.

After the user decides, record the result in an ADR or dated finding before continuing.

## Session end

Before ending a session:

1. Run relevant verification and cleanup.
2. Update completed and blocked TODOs.
3. Add or update linked findings and evidence.
4. Update ADR status when a decision was made.
5. Update the compatibility matrix for every tested target.
6. Update `status.md`.
7. Replace `handoff.md` with the exact continuation point.
8. Finish the `session-log.md` entry with files changed, tests run, cleanup status, risks, and next action.

The final chat response should summarize the outcome and link the phase files that changed. It must not be the only place where important context is recorded.

## Corrections and historical integrity

- `session-log.md` is append-only.
- Evidence is immutable after review; create a corrected evidence record rather than silently changing the original.
- A finding may be marked `Superseded` but should not be deleted.
- ADRs use normal ADR status transitions: `Proposed`, `Accepted`, `Superseded`, or `Rejected`.
- `status.md` and `handoff.md` are current-state documents and may be rewritten.

## Phase transition

Phase 1 may start only after the Phase 0 exit gate is recorded as passed. When it passes:

1. Freeze the Phase 0 compatibility matrix and accepted ADR set.
2. Complete the cleanup inventory.
3. Set `status.md` to `Complete — Ready for Phase 1`.
4. Add a final session-log entry.
5. Update `handoff.md` to point to the formal
   [Phase 1 package](../plan-execution-phase-1/README.md).
6. Keep the Phase 1 package compact unless an additional file has a clear
   reader and cannot fit its README, plan, or TODO.
