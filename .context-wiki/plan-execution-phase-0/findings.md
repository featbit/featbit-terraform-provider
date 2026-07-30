# Phase 0 Findings

This file contains verified observations, explicit hypotheses, and capability classification. Every verified finding must link sanitized evidence.

## Status vocabulary

- **Verified**: reproduced on the named deployment/version with evidence.
- **Documented**: stated by an official source but not yet reproduced.
- **Hypothesis**: requires a probe or decision.
- **Superseded**: retained for history and linked to the correction.

## Established project constraints

### FND-0001 — Current API is fixed for provider v1

Status: **Decision**  
Source: User direction recorded on 2026-07-30.

Provider GA must not depend on FeatBit backend or public OpenAPI changes. Gaps are handled through the provider compatibility layer, constrained lifecycle semantics, external prerequisites, or omission.

### FND-0002 — LaunchDarkly is a reference, not a parity requirement

Status: **Decision**  
Source: User direction recorded on 2026-07-30.

LaunchDarkly may inform mature Terraform engineering patterns. FeatBit customer workflows and FeatBit's resource model determine scope and schema.

### FND-0003 — API access tokens are the provider authentication contract

Status: **Documented; live verification pending**

Official FeatBit documentation states that the REST API accepts personal and service access tokens in the `Authorization` header. Service tokens are intended for long-term integrations. Provider v1 does not implement login or JWT lifecycle management.

Sources:

- <https://docs.featbit.co/api-docs/overview>
- <https://docs.featbit.co/integrations/api-access-tokens>
- <https://docs.featbit.co/api-docs/using-featbit-rest-api>

Required evidence: `P0-020` through `P0-025`.

## OpenAPI findings

No Phase 0 evidence recorded yet. Reproduce the planning inventory under `P0-010` through `P0-014`.

## Authentication findings

No live observations recorded yet.

## Error and absence findings

No live observations recorded yet.

## Project and environment findings

No live observations recorded yet.

## Feature flag findings

No live observations recorded yet.

## Segment findings

No live observations recorded yet.

## Member and IAM findings

No live observations recorded yet.

## Environment secret findings

No live observations recorded yet. Never record secret values.

## Capability support matrix

Fill this table only after the relevant probe tasks complete.

| Capability | Customer workflow | Support level | Identity | Lifecycle constraints | Evidence/ADR |
|---|---|---|---|---|---|
| Project | Bootstrap and manage projects | Pending | Pending | Pending | Pending |
| Environment | Manage deployment environments | Pending | Pending | Pending | Pending |
| Feature flag | Manage flag configuration and targeting | Pending | Pending | Pending | Pending |
| Segment | Manage reusable audiences | Pending | Pending | Pending | Pending |
| Group | Manage IAM groups | Pending | Pending | Pending | Pending |
| Policy | Manage IAM policies | Pending | Pending | Pending | Pending |
| Member | Lookup/bind or create members | Pending | Pending | Pending | Pending |
| Environment secrets | Consume SDK credentials | Pending | Pending | State-security decision pending | Pending |
| Workspace | Read account context | Pending | Pending | Pending | Pending |
| Audit/analytics | Operational observation | Pending | N/A | Likely omitted/read-only | Pending |

Allowed support levels:

- Fully managed
- Constrained managed
- Read/bind only
- External prerequisite
- Omitted

## Superseded findings

None.
