# Implementation Plan: Control Plane Invocation Dispatch

**Branch**: `codex/012-invocation-dispatch` | **Date**: 2026-07-16 | **Spec**: [spec.md](spec.md)

## Summary

Add a Control Plane-owned Invocation application service and Router Internal v3 HTTP adapter, a v4 Gateway invoke handler, the narrow Workspace authorization operation required to return the exact current pin, and strict runtime configuration/wiring. Forward Router JSON as a stream and SSE one bounded flushed event at a time. Do not implement Router, Ledger, reads, Agent transport, retries, or integration deployment.

## Technical Context

**Language**: Go 1.26  
**Dependencies**: Go standard HTTP/JSON packages and existing contract DTO/validators  
**Storage**: Existing Workspace/Catalog stores through Workspace service only; no Invocation persistence  
**Testing**: Unit and HTTP tests with fakes/`httptest`, then full `go test` and `go vet`  
**Constraints**: Frozen v4/v3 contracts; live proxy; required no-default config; zero fallback

## Ownership And Flow

```text
Gateway v4
  -> strict auth/media/body validation
  -> Workspace.AuthorizeInvocation(owner, workspace, agent, capability)
  -> Invocation Dispatch creates root IDs
  -> Router Internal v3 HTTP client (one attempt)
  -> live JSON or bounded/flushed SSE proxy
```

- `gateway/` owns the public HTTP boundary and pre-correlation errors.
- `workspace/` owns owner/installation/pin/capability authorization.
- `invocation/` owns trusted root context construction and the Router-only port.
- Router remains the only owner of acceptance, execution, and Ledger facts.

## Constitution Check

- Control Plane/Data Plane split: PASS; only Router is downstream.
- Data ownership: PASS; Dispatch does not read Workspace tables.
- Versioned boundary: PASS; consumes frozen Northbound v4 and Router Internal v3 DTOs.
- Runtime agnosticism: PASS; no Agent framework or endpoint enters Control Plane.
- Fallback: PASS; one explicit destination, one attempt, required config.

## Write Scope

- `specs/012-control-plane-invocation-dispatch/`
- `apps/control-plane/internal/invocation/`
- `apps/control-plane/internal/gateway/invocation_handler*.go`
- narrow `apps/control-plane/internal/workspace/` authorization method/tests
- `apps/control-plane/internal/config/` and `apps/control-plane/cmd/control-plane/` wiring/tests

Shared contracts, Router, Ledger, deployment, CI, and Console are excluded.

## Post-Design Analysis

PASS. Spec 012 specializes Spec 010 T002 and consumes Spec 011 without changing shared schemas. The only cross-owner addition is a narrow Workspace authorization operation, necessary because existing internal resolution requires an already-known exact version and public Dispatch must not access Workspace storage. No unresolved high-impact ambiguity remains.

## Fallback Report

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
