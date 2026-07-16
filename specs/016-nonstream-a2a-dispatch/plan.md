# Implementation Plan: Non-Streaming A2A Dispatch

**Branch**: `codex/016-nonstream-a2a-dispatch` | **Date**: 2026-07-16 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/016-nonstream-a2a-dispatch/spec.md`

## Summary

Replace the Router Foundation dispatch placeholder for `stream=false` requests
with a real A2A `message/send` transport. The Router will validate and resolve
the request as it does today, call the resolved Runtime B-compatible Agent
endpoint with trusted platform context headers, commit required metadata-only
Ledger lifecycle facts, and return one transient JSON result. Streaming,
deadline/cancel propagation, SDK nested calls, and acceptance E2E remain later
Spec 017+ work.

## Technical Context

**Language/Version**: Go 1.26.0

**Primary Dependencies**: Go `net/http`; `github.com/a2aproject/a2a-go v0.3.15`
client/server packages; existing contracts validators; existing Router auth,
config, resolution, API, and Ledger packages.

**Storage**: Router-owned Ledger PostgreSQL schema for metadata-only lifecycle
facts. Tests may use a strict in-memory recorder for transport mapping and the
existing Ledger store where a PostgreSQL database is available.

**Testing**: Go `testing`, `httptest`, Runtime B sample endpoint, focused
Router unit/HTTP tests, optional integration tests under the existing
integration tag, active A2A negative corpus tests, race where practical, full
`go test ./...`, vet, and diff checks.

**Target Platform**: Windows developer shell and Linux/WSL/container runtime.

**Project Type**: Router Data Plane feature slice.

**Performance Goals**: Preserve correct correlation and result isolation for
100 concurrent non-streaming dispatches. No new latency SLO is introduced.

**Constraints**: Router-only Agent transport; no Control Plane internal imports;
no result storage/replay; metadata-only Ledger; no streaming/cancellation scope;
fallback-addition budget zero.

## Constitution Check

- **Phase 1 loop**: PASS. This feature advances the `Invoke` step and prepares
  durable metadata for `Record`.
- **Ownership**: PASS. Router owns A2A transport and Ledger writes; Control
  Plane facts are consumed only through versioned internal resolution.
- **Runtime independence**: PASS. Runtime B is an external sample endpoint;
  Router depends on A2A protocol behavior, not Runtime internals.
- **Contracts**: PASS. Uses active Router Internal v3, Router Agent v1,
  Invocation Event v0.3, Result Stream Event v2, Platform Error v4, and A2A
  Profile constraints.
- **Invocation lineage**: PASS. Existing invocation/root task/trace IDs remain
  authoritative and are propagated, not regenerated.
- **Failure safety**: PASS. No fallback endpoint, retry, cache, default secret,
  default Agent URL, or degraded success is planned.
- **SDD traceability**: PASS. Tasks map implementation and tests to FRs and
  success criteria.

## Project Structure

### Documentation

```text
specs/016-nonstream-a2a-dispatch/
|-- spec.md
|-- checklists/requirements.md
|-- research.md
|-- data-model.md
|-- plan.md
|-- quickstart.md
`-- tasks.md
```

### Source Code

```text
apps/a2a-router/internal/api/dispatch_handler.go
apps/a2a-router/internal/api/dispatch_handler_test.go
apps/a2a-router/internal/transport/a2a/
apps/a2a-router/internal/ledger/
agents/runtime-b/
apps/a2a-router/cmd/a2a-router/
apps/a2a-router/internal/config/
```

**Structure Decision**: Add a narrow Router-owned `transport/a2a` package for
non-streaming Agent calls. Keep HTTP boundary orchestration in
`internal/api`. Use interfaces for Ledger appends and transport clients so
unit tests can assert ordering without a PostgreSQL dependency.

## Failure Mapping

| Condition | Router behavior | Ledger behavior |
| --- | --- | --- |
| Invalid request/auth/media | Existing pre-resolution platform error | No accepted fact |
| Control Plane resolution failure | Preserve typed resolution error | No Agent call |
| Unsupported endpoint/profile/auth | Correlated platform error | Accepted failure if after accepted boundary |
| Agent endpoint dependency failure | Correlated dependency/protocol error | Terminal failure if accepted |
| Agent business failure | Correlated Agent failure | Terminal failure |
| Ledger append failure before success | Explicit non-success | No fabricated success |

## Write Scope

- Owned: `specs/016-nonstream-a2a-dispatch/**`,
  `apps/a2a-router/internal/api/dispatch_handler.go`,
  `apps/a2a-router/internal/api/dispatch_handler_test.go`,
  `apps/a2a-router/internal/transport/a2a/**`,
  `apps/a2a-router/cmd/a2a-router/**`,
  `apps/a2a-router/internal/config/**`, `deploy/compose.yaml`, the narrow
  Runtime B HTTP media-type adapter, and focused test fixtures.
- Referenced but not re-owned: `apps/a2a-router/internal/ledger/**` and
  `contracts/**`.
- Not owned: Control Plane internals, SDK, Runtime A, streaming/cancellation,
  Console, or contract version changes.

Converge additions require `NEKIRO_DATABASE_URL`,
`NEKIRO_ROUTER_AGENT_RESPONSE_LIMIT_BYTES`, and
`NEKIRO_ROUTER_A2A_EVENT_LIMIT_BYTES` at Router startup. The Router checks the
Ledger schema and fails closed; it does not auto-migrate. The current Compose
file now orders `a2a-router-migrate` before the Router `serve` service. Standalone
deployments must provide equivalent ordering.

## Complexity Tracking

No constitution violations require justification.
