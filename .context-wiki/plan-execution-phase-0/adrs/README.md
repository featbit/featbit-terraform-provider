# Phase 0 Architecture Decision Records

Create ADRs from [_template.md](_template.md). ADRs are accepted only after their empirical assumptions link to Phase 0 evidence.

## Required ADRs

| ADR | Question | Required evidence | Status |
|---|---|---|---|
| ADR-001 | Which feature flag and segment attributes are owned by Terraform versus the UI? | Round-trip, narrow-update, drift, and revision probes | Accepted — all four flag types and environment-specific segment ownership verified; flag operational fields reduced to Computed-only/omitted |
| ADR-002 | Does Terraform destroy delete or archive flags/segments, and is any provider option exposed? | Delete/archive/restore and key-reuse behavior | Accepted — flags and environment segments archive then hard Delete; restore/key-reuse guarantees omitted; references fail closed |
| ADR-003 | How are the pinned OpenAPI snapshot, overlay, generated client, and handwritten wrapper structured? | Deterministic snapshot/overlay generation and API asymmetry inventory | Accepted |
| ADR-004 | What are the permanent Import ID formats for resources and bindings? | Stable remote identities and exact lookup behavior | Accepted |
| ADR-005 | Which FeatBit Cloud/self-hosted, Terraform, and Go versions are supported? | Completed compatibility matrix and current toolchain baseline | Accepted — available Cloud core matrix complete; self-hosted unavailable/`Not tested`, with no exact-release claim |

## Naming

```text
ADR-001-terraform-ui-ownership.md
ADR-002-delete-vs-archive.md
ADR-003-openapi-client-and-overlay.md
ADR-004-import-identities.md
ADR-005-supported-compatibility-matrix.md
```

## Status transitions

```text
Proposed -> Accepted
Proposed -> Rejected
Accepted -> Superseded by ADR-NNN
```

Do not edit an accepted decision merely because later work chooses differently. Add a superseding ADR or a clearly dated amendment when the decision remains compatible.
