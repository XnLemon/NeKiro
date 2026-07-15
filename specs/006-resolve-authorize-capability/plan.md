# Implementation Plan: Resolve and Authorize an Installed Agent Capability

**Branch**: `codex/issue-6-resolution` | **Date**: 2026-07-15 | **Spec**: [spec.md](spec.md)

## Summary

Complete the existing Control Plane Internal v2 exact-resolution operation for
Router use. Correct its failure precedence by proving Workspace existence before
current Installation lookup, complete the existing v2 404 error metadata for
generic `NOT_FOUND`, then add focused unit, contract, internal HTTP,
PostgreSQL, restart, authorization, correlation, and dependency evidence. Keep
the route, wire fields, contract version, and module ports unchanged.

## Technical Context

**Language/Version**: Go version declared by `go.mod`

**Primary Dependencies**: Go standard HTTP library, `pgx/v5`, existing Catalog and Workspace ports, active contract validator

**Storage**: PostgreSQL, with Workspace owning Workspace/Installation facts and Catalog owning Agent Card publication state

**Testing**: Go unit tests, contract tests, `-race`, and integration-tagged PostgreSQL tests using a dedicated `_test` database only

**Target Platform**: Control Plane Go process on the server; Router-facing HTTP Internal v2 boundary

**Project Type**: Go Control Plane backend

**Performance Goals**: Preserve the existing synchronous exact-resolution operation; no new latency policy, cache, retry, or endpoint probe

**Constraints**: Active Internal API v2 only; strict correlation; exact failure precedence; no direct table access; no secret/result/health fields; no fallback additions

**Scale/Scope**: One exact Workspace/Agent/version/capability request per operation; tests cover concurrent-independent state transitions rather than new scale behavior

## Constitution Check

*GATE: Passed before design and re-checked after tests/review.*

- **Phase 1 loop**: PASS. This closes the pre-dispatch `Install -> Invoke` authorization boundary.
- **Ownership**: PASS. Workspace Store and CatalogReader remain the only dependencies; no cross-module table access.
- **Runtime independence**: PASS. No Agent Runtime or A2A execution dependency is introduced.
- **Contracts**: PASS. Active Control Plane Internal v2, Agent Card v0.2, Installation v2, and Platform Error v3 are reused without version changes; only the existing v2 404 response metadata is completed for `NOT_FOUND`.
- **Invocation lineage**: PASS. HTTP errors preserve existing request Invocation/root Task/Trace identifiers after strict validation.
- **Failure safety**: PASS. Missing, disabled, unauthorized, not-found, and dependency states remain distinct; no secrets or dependency details cross the boundary.
- **SDD traceability**: PASS. Each behavior and test is mapped to this Spec and Tasks; tests follow implementation per project policy.
- **Cross-runtime proof**: PASS. The result remains a runtime-neutral Card/Installation authorization fact for a later Router.

## Architecture and Ownership

```text
Router
  -> POST /internal/v2/resolve-agent
  -> internal Bearer authentication
  -> strict body and correlation validation
  -> workspace.Service.Resolve
      -> Workspace Store.GetWorkspace (existence)
      -> Workspace Store.GetCurrentInstallation (exact current pin)
      -> CatalogReader.GetVersion (exact current Card)
      -> capability/permission containment
  -> exact Card + enabled ResolvedInstallation
```

The Gateway owns authentication, strict decoding, correlation shape, and public
HTTP mapping. Workspace owns resolution ordering and authorization policy.
Catalog remains the sole source of Card state. The Router-facing operation does
not read either module's storage directly.

## Required Correction

`Service.Resolve` must call `Store.GetWorkspace` after request validation and
before `GetCurrentInstallation`. Map `ErrNotFound` from this preflight to
`workspace.ErrNotFound`; preserve dependency and other typed failures. Existing
current-installation, Catalog, capability, response validation, and HTTP error
paths remain unchanged unless a mapped test demonstrates a contract violation.

## Test Matrix

| Area | Evidence |
| --- | --- |
| Service unit | Workspace precedence; install missing/pin mismatch; disabled ordering; published/disabled/missing Catalog; capability and permission containment; response safety |
| Contract | Active Internal v2 path/security/fields/error set and exact response mapping |
| Internal HTTP | Separate internal auth, strict validation, pre/post correlation, all typed outcomes and fixed messages |
| PostgreSQL | Missing Workspace vs missing Installation, exact disabled state, dependency mapping, reconstructed service resolution, no Installation mutation |
| Static/race | `go vet`, `go test ./...`, and `go test -race ./...` |

## Files To Change

- Modify: `apps/control-plane/internal/workspace/service.go`
- Modify: `apps/control-plane/internal/workspace/service_test.go`
- Modify: `apps/control-plane/internal/gateway/workspace_handler_test.go`
- Modify: `apps/control-plane/internal/workspace/integration/workspace_test.go`
- Modify: `contracts/workspace_api_contracts_test.go` or
  `contracts/result_api_contracts_test.go` only if existing contract coverage
  cannot express the required mapping.
- Create: `specs/006-resolve-authorize-capability/*`

No migrations, runtime dependencies, route changes, wire-field changes, or
unrelated refactors are planned.

## Verification Gates

1. Focused Workspace/Gateway/contract unit tests pass.
2. Focused and broad race tests pass.
3. `go vet ./...`, `go build ./apps/control-plane/cmd/control-plane`,
   `go mod tidy -diff`, and `git diff --check` pass.
4. Integration-tagged PostgreSQL tests run only when
   `NEKIRO_TEST_DATABASE_URL` names a dedicated `_test` database; otherwise
   report that limitation explicitly.
5. Independent diff review finds no High/Medium issue; convergence reports no
   remaining work.
