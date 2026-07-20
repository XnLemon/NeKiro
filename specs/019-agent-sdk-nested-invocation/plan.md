# Implementation Plan: Agent SDK Nested Invocation

**Branch**: `codex/016-nonstream-a2a-dispatch` | **Date**: 2026-07-16 | **Spec**: [spec.md](spec.md)

**Status**: T000 resolved; implementation active.

## Summary

Add a thin Go SDK for the accepted Agent Router v1 contract and a Router-owned
nested adapter. The adapter authenticates an Agent binding, reads one committed
parent projection, derives trusted child context, and delegates child execution
to the existing DispatchHandler/transport/Ledger path. No Agent Runtime code or
new persistence is introduced.

The current exact resolver requires a Card version, while Agent Router v1 does
not carry one. No implementation may choose a version or add a compatibility
fallback until T000 is decided.

## Technical Context

**Language/Version**: Go 1.26.

**Dependencies**: standard `net/http`, `encoding/json`, existing versioned
contracts and Router packages. The SDK does not import Control Plane,
PostgreSQL, or an Agent framework.

**Boundaries**:

```text
Managed Agent transport headers
  -> SDK context validation
  -> POST /agent/v1/invocations (agent bearer binding)
  -> Router parent Ledger read
  -> child DispatchHandler trusted adapter
  -> existing exact resolution / A2A transport / Ledger
```

The Agent-facing handler owns only authentication, strict request decoding,
parent checks, child ID generation, and response adaptation. DispatchHandler
continues to own result modes, route resolution, transport, and lifecycle facts.

## Trust and Failure Decisions

- Agent credentials are an explicit `[]auth.Principal` binding supplied by the
  process owner; the SDK receives one raw token only through its constructor and
  never serializes it into a request body or result.
- Parent `InvocationDetailResponseV4` is read through the Router Ledger port.
  A missing parent is `404`, a target mismatch is `403`, and a non-running parent
  is `409`; all happen before child `created` acceptance.
- Child identity and all inherited lineage are generated from the parent. The
  nested request cannot provide them.
- The adapter delegates to a trusted DispatchHandler entry point so child calls
  use the same exact resolver, transport, deadline, Ledger, and JSON/SSE rules.
- The SDK HTTP client disables redirects with `http.ErrUseLastResponse`; it
  makes one request and returns dependency errors without alternate routes.

## Project Structure

```text
sdks/agent-sdk/
  client.go
  client_test.go
apps/a2a-router/internal/nested/
  context.go
  context_test.go
  binding.go
  binding_test.go
apps/a2a-router/internal/api/
  agent_invocation_handler.go
  agent_invocation_handler_test.go
```

Process and Compose registration remain outside this child and are owned by
the parent acceptance task.

## Verification

- SDK unit tests cover context grammar, strict request shape, exact headers,
  one request, and redirect rejection.
- Nested handler tests cover auth-first rejection, parent status/identity,
  derived child correlation, trusted-field injection, mode negotiation, and
  zero Ledger/Agent calls before acceptance.
- Contract tests validate router-agent.v1 and active runtime error/result
  shapes; `go test ./...`, `go vet ./...`, race, and diff checks remain required.

## Fallback Report

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
