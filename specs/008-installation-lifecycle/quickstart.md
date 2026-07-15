# Quickstart: Installation Lifecycle Evidence

## Focused Checks

```sh
go test -count=1 ./contracts ./apps/control-plane/internal/workspace ./apps/control-plane/internal/gateway ./apps/control-plane/internal/workspace/postgres
go test -race -count=1 ./apps/control-plane/internal/workspace ./apps/control-plane/internal/gateway
```

## PostgreSQL Checks

Set `NEKIRO_TEST_DATABASE_URL` to an explicit PostgreSQL URL whose database
name ends in `_test`. Run schema-resetting packages serially:

```sh
go test -tags=integration -count=1 ./apps/control-plane/internal/workspace/postgres
go test -tags=integration -count=1 ./apps/control-plane/internal/workspace/integration
```

Without that dedicated database, integration tests must be reported as not run.

## Broad Checks

```sh
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./...
go mod tidy -diff
git diff --check
docker compose --file deploy/compose.yaml config --quiet
```

## Acceptance Evidence

Verify the complete transition table, owner/auth/error matrix, immutable pin
and permission preservation, terminal row restart equality, reinstall-new-ID,
and concurrent lifecycle/install invariants. Verify lifecycle does not invoke
Catalog or add retry, cache, probe, reconciliation, or degraded success.

Fallback delta must remain:

```text
Fallback delta: removed 0, retained 1, added 0, net 0
Added fallback evidence: none
```
