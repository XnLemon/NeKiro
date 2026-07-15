# Quickstart: Validate Issue #6 Exact Resolution

## Prerequisites

- Go version from `go.mod`.
- Repository dependencies downloaded.
- For integration tests, `NEKIRO_TEST_DATABASE_URL` must point to a dedicated
  PostgreSQL database whose name ends in `_test`.

## Focused Validation

```sh
go test -count=1 ./apps/control-plane/internal/workspace ./apps/control-plane/internal/gateway ./contracts
go test -race -count=1 ./apps/control-plane/internal/workspace ./apps/control-plane/internal/gateway ./contracts
go vet ./apps/control-plane/internal/workspace ./apps/control-plane/internal/gateway ./contracts
```

These commands verify the Workspace precedence correction, capability
containment, internal auth boundary, exact error mapping, response safety, and
contract source mapping.

## PostgreSQL Validation

Run only with the dedicated database configured:

```sh
go test -tags=integration -count=1 ./apps/control-plane/internal/workspace/postgres ./apps/control-plane/internal/workspace/integration
```

Expected evidence includes missing Workspace vs missing Installation mapping,
process/store reconstruction, Catalog disablement without Installation
mutation, exact permission authorization, and dependency failures.

## Full Repository Validation

```sh
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./apps/control-plane/cmd/control-plane
go mod tidy -diff
git diff --check
```

No command adds retries, endpoint probes, alternate sources, defaults, caches,
or compatibility routes.
