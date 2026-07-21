# Implementation Plan: Cross-Runtime Caller Sample

**Branch**: `codex/020-cross-runtime-caller` | **Date**: 2026-07-21 | **Spec**: [spec.md](spec.md)

## Summary

Implement Runtime A as an isolated Go sample using `trpc-agent-go` v1.10.0 for Agent/Runner/Event execution. A thin A2A adapter at the sample boundary accepts the active JSON-RPC Profile, derives trusted platform context from Router-injected headers, invokes Runtime B exactly once through `sdks/agent-sdk`, and returns a deterministic data-part message. The framework and all Runtime A state remain inside a nested module; the root module, Control Plane, Router, contracts, and SDK remain unchanged.

## Technical Context

**Language/Version**: Go 1.26 for the repository; Runtime A module targets Go 1.26.

**Primary Dependencies**: `trpc.group/trpc-go/trpc-agent-go v1.10.0` (Runtime A only), `github.com/a2aproject/a2a-go v0.3.15` (active wire adapter), and the local root module for `contracts` and `sdks/agent-sdk`.

**Storage**: No durable storage. The framework may use an ephemeral process-local Session for one Runner execution; Runtime A owns no database or Runtime B task store.

**Testing**: `go test ./...`, `go test -race ./...`, and `go vet ./...` from `agents/runtime-a`; root `go test ./...` remains the platform regression gate.

**Target Platform**: Linux process/container; explicit HTTP listener and Router configuration are required at startup.

**Project Type**: Isolated Agent sample and protocol adapter.

**Performance Goals**: Focused tests sustain 100 concurrent root calls without cross-call context or result contamination; no new latency or retry policy is introduced.

**Constraints**: Required settings use strict parsing and no defaults. One root call produces at most one SDK nested call. Runtime framework types do not cross the sample boundary.

**Scale/Scope**: One stateless sample process, one deterministic callee call per request, JSON root mode only for this child feature; final SSE integration is owned by Spec 021.

## Constitution Check

*GATE: Pass before and after design.*

- **Phase 1 loop**: PASS. This completes the second Runtime proof for Invoke and Record without expanding the platform core.
- **Ownership**: PASS. Runtime A owns only its sample execution and adapter; Router owns routing/lineage/Ledger and Runtime B owns its task state.
- **Runtime independence**: PASS. Framework dependency is nested under `agents/runtime-a`; core imports remain framework-free.
- **Contracts**: PASS. Active Agent Router v1, A2A Profile 0.2/0.3.0, and SDK contracts are consumed unchanged; no new public contract is required.
- **Invocation lineage**: PASS. Headers are validated at the adapter boundary; SDK sends the root Invocation as `parentInvocationId`, and child results are checked against root Task/Trace.
- **Failure safety**: PASS. Configuration, input, protocol, Router, and correlation failures remain visible. No retry/cache/alternate path/default is added.
- **SDD traceability**: PASS. Tasks and focused tests map one-to-one to the feature requirements and scenarios.
- **Cross-runtime proof**: PASS. Runtime A uses trpc-agent-go Agent/Runner/Event execution; Runtime B remains the direct a2a-go implementation.

## Research Summary

See [research.md](research.md). The selected framework is pinned only in the nested Runtime A module, while the external A2A adapter uses the repository's active `a2a-go` profile library to preserve wire compatibility.

## Architecture and Data Flow

```text
Router -> A2A JSON-RPC + x-nek-* context headers -> Runtime A adapter
       -> trpc Agent/Runner/Event execution
       -> Agent SDK (one call, configured bearer) -> Router Agent v1
       -> exact Runtime B resolution/invocation -> child result
       -> Runtime A deterministic data message -> Router -> Gateway
```

The adapter does not accept a target endpoint from the request. Its callee Agent ID and capability are deployment configuration and are sent only as SDK request fields. The SDK owns the only Router destination construction.

## Project Structure

### Documentation

```text
specs/020-cross-runtime-caller/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── checklists/requirements.md
└── tasks.md
```

### Source Code

```text
agents/runtime-a/
├── go.mod                  # nested module; pins framework locally
├── go.sum
├── README.md
├── config.go               # strict required environment parsing
├── handler.go              # active a2a-go JSON-RPC adapter
├── runtime.go              # trpc Agent/Runner/Event composition
├── cmd/runtime-a/main.go   # process entrypoint
├── config_test.go
├── nested_test.go
├── e2e_test.go
└── isolation_test.go
```

No files under `apps/`, `contracts/`, `sdks/agent-sdk/`, `agents/runtime-b/`, `deploy/`, or `.github/workflows/` are modified by this child.

## Implementation Phases

1. Create the nested module and strict configuration boundary.
2. Implement Runtime A's framework Agent/Runner/Event execution and deterministic composition.
3. Implement the A2A adapter, platform-header validation, and process entrypoint.
4. Add focused contract, failure, concurrency, no-direct-URL, correlation, and content-exclusion tests after implementation.
5. Run the root regression suite and isolated race/vet suite, perform independent Standards/Spec review, append any Converge tasks, and resolve all findings before PR/CI.

## Complexity Tracking

No constitution violations. The nested module is the minimum isolation seam needed to pin a second Runtime without making the platform depend on it.

## Fallback Report

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
