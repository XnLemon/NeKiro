# Implementation Plan: Installation Lifecycle

## Summary

Complete Issue #8 against the existing Workspace Installation runtime. The
active Northbound v3 contract is reused. The implementation owner remains the
Workspace module; Catalog is not consulted by lifecycle operations.

## Architecture and Boundaries

- Gateway authenticates, validates request shape, generates Trace, and maps
  fixed errors.
- Workspace service validates identifiers, applies owner policy, and delegates
  lifecycle mutation to the Workspace store.
- PostgreSQL Workspace store owns Installation rows, row locks, state checks,
  timestamp updates, terminal preservation, and current uniqueness.
- Contracts remain the source of truth for Installation v2 and Northbound v3.
- No Router, Catalog mutation, deployment, reconciliation, retry, cache, or
  alternate source is added.

## Existing Runtime Touch Points

- `apps/control-plane/internal/workspace/service.go`
- `apps/control-plane/internal/workspace/store.go`
- `apps/control-plane/internal/workspace/postgres/store.go`
- `apps/control-plane/internal/workspace/postgres/migrations.go`
- `apps/control-plane/migrations/003_workspace.sql`
- `apps/control-plane/internal/gateway/workspace_handler.go`
- `contracts/openapi/control-plane.v3.yaml`

The initial implementation pass will first verify these paths with focused
tests. A code change is made only where a test demonstrates a gap in the
approved semantics.

## Data and Transaction Design

1. Authenticate and validate path/body before persistence.
2. Read Workspace and authorize immutable owner.
3. Begin one Workspace-owned lifecycle transaction.
4. Lock the exact `(workspaceId, installationId)` row with `FOR UPDATE`.
5. Return `NOT_FOUND` for an unknown or cross-Workspace row.
6. Apply only the exact transition graph; return `CONFLICT` for all illegal
   transitions without an update.
7. Under the row lock, normalize candidate and previous times to PostgreSQL
   microsecond precision, then choose a committed timestamp strictly later than
   the previous `updatedAt`; advance a stale or equal candidate by one
   PostgreSQL microsecond when contention makes the caller-provided time
   non-monotonic. Update status atomically, and for uninstall set both terminal
   timestamps to that same committed value.
8. Return `RETURNING` values after commit, preserving immutable facts.

The existing install transaction locks the Workspace row and rechecks partial
uniqueness. This is retained so concurrent install/lifecycle operations cannot
create two current rows.

## Error Semantics

| Condition | Service error | HTTP |
| --- | --- | --- |
| Invalid path/body/target | `ErrInvalid` | 400 `VALIDATION_ERROR` |
| Missing trusted bearer | gateway auth failure | 401 `UNAUTHENTICATED` |
| Non-owner | `ErrForbidden` | 403 `FORBIDDEN` |
| Unknown Workspace/Installation or cross-root pair | `ErrNotFound` | 404 `NOT_FOUND` |
| Illegal or repeated transition | `ErrConflict` | 409 `CONFLICT` |
| Store/transaction dependency failure | `ErrDependency` | 503 `DEPENDENCY_ERROR` |

Error responses expose only the fixed contract payload and safe Trace ID.

## Verification Strategy

- Contract: inspect active OpenAPI paths, PATCH enum, DELETE response, and all
  lifecycle error response references.
- Unit: table-drive legal transitions, illegal transitions, immutable fields,
  timestamps, owner/identity/error ordering, and no Catalog call.
- HTTP: exercise PATCH/DELETE success and every mapped failure, strict body,
  authentication, Trace, and secret exclusion.
- PostgreSQL: run dedicated lifecycle transition, constraint, restart, and
  concurrent lifecycle/install tests when `NEKIRO_TEST_DATABASE_URL` names a
  database ending `_test`.
- Broad: run `go test`, race, vet, build, module tidy, diff check, and Compose
  config validation where tools/configuration are available.

## Risks and Controls

- Timestamp drift is controlled by asserting response values equal a fresh
  database read and by retaining SQL `RETURNING` facts.
- Row race outcomes are controlled by `FOR UPDATE`, the partial unique index,
  and post-race invariant queries.
- Cross-module mutation is controlled by a test spy and static review showing
  no Catalog call in lifecycle paths.
