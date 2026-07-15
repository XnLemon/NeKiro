# Implementation Plan: Invocation Runtime Contracts

**Branch**: `codex/011-invocation-runtime-contracts` | **Date**: 2026-07-16 | **Spec**: [spec.md](spec.md)

## Summary

Freeze the shared runtime boundary before parallel implementation. Add a distinct Agent-facing Router API, version the breaking Control-Plane-to-Router and Northbound Invocation surfaces, add exact unsupported-auth and payload-size errors, version runtime Ledger/result event schemas where those errors are embedded, document acceptance/interruption/cancel/framing policy in ADR 0006, expose thin Go DTO mappings, and verify everything with contract/static tests. No runtime service is implemented.

## Technical Context

**Language/Version**: Language-neutral OpenAPI 3.1 / JSON Schema 2020-12; Go 1.26 consumer mappings
**Primary Dependencies**: Existing contract validator stack and `kin-openapi`; no new runtime dependency
**Storage**: N/A; only persistence semantics and ownership contracts are frozen
**Testing**: Go contract tests, schema/OpenAPI loading, `go test`, `go vet`
**Target Platform**: Cross-process HTTP/SSE contracts for Linux containers and local development
**Project Type**: Monorepo contract/design gate
**Performance Goals**: No throughput SLO; deterministic byte/deadline bounds and one-terminal semantics
**Constraints**: Zero fallback; historical artifacts immutable; no Router/Dispatch/Ledger runtime; secrets absent from contract facts
**Scale/Scope**: Three HTTP directions, two new embedded error/event schema revisions, one ADR, one Go mapping file

## Constitution Check

- **Phase 1 loop - PASS**: Unblocks `Invoke -> Record`.
- **Ownership - PASS**: Agent-facing and Control-Plane-facing Router directions are separate; Router derives trust and owns acceptance/Ledger semantics.
- **Runtime independence - PASS**: The contract uses Agent identity and A2A facts only.
- **Contracts - PASS**: Each breaking surface has a new version and migration note; historical files remain unchanged.
- **Invocation lineage - PASS**: Root and child trust sources are explicit.
- **Failure safety - PASS**: Unsupported auth, payload size, dependency interruption, cancel, and timeout remain distinct; no secret field or fallback exists.
- **SDD traceability - PASS**: Implementation precedes mapped tests and Review/Converge remain open.
- **Cross-runtime proof - PASS**: Later SDK/sample tasks consume the same Agent-facing API.

## Version Decisions

| Contract | Target | Reason |
| --- | --- | --- |
| Northbound Invocation API | `v4` | Adds required request cap source, 413 outcome, accepted boundary, and post-commit Ledger-failure delivery meaning to an already published invoke operation. |
| Router Internal API | `v3` | Tightens service authentication, body/response bounds, accepted boundary, and dependency interruption behavior. |
| Agent Router API | `v1` | New direction, caller class, trust derivation, and authentication boundary. |
| Platform Error | `v4` | Adds distinct `AGENT_AUTH_UNSUPPORTED` and `PAYLOAD_TOO_LARGE` codes/messages. |
| Invocation Event | `v0.3` | Routing terminal facts must carry Platform Error v4 and its new unsupported-auth code. |
| Result Stream Event | `v2` | Committed SSE must carry Platform Error v4 for unsupported-auth/size/dependency delivery. |
| Invocation Result | `v1` | Success shape is unchanged. |
| Control Plane Internal | `v2` | Exact resolution request/response semantics are unchanged. |
| Agent Card | `0.2` | Registry acceptance remains unchanged; runtime support is an invocation policy. |
| A2A Profile | Schema `0.2`, protocol `0.3.0` | Existing operations remain sufficient; cancellation/limit execution policy belongs in ADR/API extensions. |

No runtime consumer exists, so first implementations consume targets only. No dual route, decoder, error remapping, or historical fallback is approved.

## Project Structure

```text
specs/011-invocation-runtime-contracts/
|-- spec.md
|-- plan.md
|-- research.md
|-- data-model.md
|-- quickstart.md
|-- contracts/runtime-contract.md
|-- checklists/requirements.md
`-- tasks.md

contracts/
|-- openapi/control-plane-invocation.v4.yaml
|-- openapi/router-internal.v3.yaml
|-- openapi/router-agent.v1.yaml
|-- schemas/platform-error.v4.schema.json
|-- schemas/invocation-event.v0.3.schema.json
|-- schemas/invocation-result-stream-event.v2.schema.json
|-- runtime_contracts.go
`-- runtime_contracts_test.go

docs/decisions/0006-invocation-runtime-trust-and-failure-policy.md
docs/contracts/compatibility.md
```

**Structure Decision**: Keep language-neutral facts under `contracts/`, policy in one ADR, and Go DTOs as consumers. Do not create runtime package directories.

## Delivery Phases

1. Write target schemas and three directional OpenAPI documents.
2. Record ADR and compatibility migration policy.
3. Add Go DTO/constants without changing historical mapping aliases.
4. Add focused contract tests and run full static/contract validation.
5. Commit/push/open draft PR; leave independent Review and Converge unchecked.

## Post-Design Constitution Check

PASS. The design introduces no service implementation, no cross-owner data access, no Runtime framework dependency, no secret-bearing field, and no compatibility fallback.

## Fallback Inventory

| Candidate | Classification | Result |
| --- | --- | --- |
| Missing Agent caller binding | Remove | Unauthenticated rejection before acceptance |
| SDK-supplied lineage | Remove | Router derives from committed parent |
| Non-`none` Agent auth | Remove | Distinct unsupported-auth terminal; no empty credential |
| Missing deadline/size config | Remove | Startup/readiness failure |
| Ledger write retry/alternate store | Remove | Explicit non-success, non-terminal history |
| Cancel retry | Remove | At most one protocol propagation request |
| Historical contract runtime | Remove | Target-only first implementation |

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
