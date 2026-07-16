# Implementation Plan: Deterministic Direct A2A Sample

**Branch**: `codex/015-runtime-b-agent` | **Date**: 2026-07-16 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/015-direct-a2a-sample/spec.md`

## Summary

Add a runnable direct-library A2A callee under `agents/runtime-b/`. A strict
fixture parser drives deterministic JSON, ordered stream, explicit failure,
and cancelable-task behavior. A process-local task store supports the active
get/cancel operations. Conformance tests invoke the actual official
JSON-RPC/SSE server with the official client.

## Technical Context

**Language/Version**: Go 1.26.0

**Primary Dependencies**: `github.com/a2aproject/a2a-go v0.3.15` (`a2a`,
`a2asrv`, and test-only `a2aclient`); Go standard library HTTP and crypto

**Storage**: Process-local in-memory Runtime Task map; no platform database

**Testing**: Go `testing`, `httptest`, official A2A client, race detector

**Target Platform**: Independent Linux/Windows server process

**Project Type**: Sample Agent web service

**Performance Goals**: 100 concurrent deterministic calls with isolated
identities and no result crossover

**Constraints**: Four active operations only; exact five-event success stream;
explicit configuration; no retry/default/cache/compatibility fallback

**Scale/Scope**: One sample callee, four fixtures, process-lifetime task state

## Constitution Check

- **Phase 1 loop**: PASS. The sample is the real Agent target required for the
  `Invoke` stage and later cross-Runtime acceptance.
- **Ownership**: PASS. Runtime state exists only under `agents/runtime-b/`; no
  platform-owned data or database is accessed.
- **Runtime independence**: PASS. Direct use of the protocol library is isolated
  from Control Plane/Router core and intentionally differs from Runtime A.
- **Contracts**: PASS. Active Agent Card 0.2 and A2A Profile Schema 0.2 /
  protocol 0.3.0 remain unchanged; the sample only consumes them.
- **Invocation lineage**: PASS. The sample receives correlation headers but
  does not synthesize platform lineage or Ledger facts.
- **Failure safety**: PASS. Invalid, failed, missing, non-cancelable, canceled,
  and context-terminated states remain explicit; no secret/config fallback.
- **SDD traceability**: PASS. Tasks map implementation before tests to FRs and
  acceptance scenarios.
- **Cross-runtime proof**: PASS. This is the direct-library Runtime B callee;
  T009/T010 own the distinct Runtime A caller and nested integration.

Post-design re-check: PASS. No contract, ownership, fallback, or framework
boundary changed during Phase 0/1 design.

## Project Structure

### Documentation (this feature)

```text
specs/015-direct-a2a-sample/
|-- spec.md
|-- checklists/requirements.md
|-- research.md
|-- data-model.md
|-- plan.md
|-- quickstart.md
`-- tasks.md
```

No child `contracts/` artifact is created because this feature consumes the
already frozen active A2A Profile and introduces no public contract.

### Source Code (repository root)

```text
agents/runtime-b/
|-- cmd/runtime-b/main.go
|-- handler.go
|-- fixture.go
|-- identity.go
|-- server.go
|-- handler_test.go
`-- server_test.go
```

**Structure Decision**: Runtime internals and executable remain wholly inside
the sample-owned `agents/runtime-b/` tree. No shared package is introduced.

## Failure Mapping

| Condition | Runtime behavior |
| --- | --- |
| Missing/invalid fixture input | `a2a.ErrInvalidParams` |
| Explicit `failure` fixture | Stable non-success error |
| Unknown Task | `a2a.ErrTaskNotFound` |
| Terminal Task cancellation | `a2a.ErrTaskNotCancelable` |
| Held request context ends | Stream terminates without fabricated terminal success |
| Non-profile handler method | `a2a.ErrUnsupportedOperation` |

## Write Scope

- Owned: `specs/015-direct-a2a-sample/**`, `agents/runtime-b/**`
- Not owned: shared `contracts/**`, `apps/**`, `sdks/**`, `deploy/**`, CI,
  root Agent context, or other Specs

## Complexity Tracking

No constitution violations require justification.
