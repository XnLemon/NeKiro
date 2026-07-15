# Quickstart: Workspace Acceptance Evidence

## Preconditions

Use a dedicated PostgreSQL database whose name ends in `_test`:

```sh
export NEKIRO_TEST_DATABASE_URL='postgres://user:password@127.0.0.1:5432/nekiro_test?sslmode=disable'
```

The integration helpers drop and recreate the Catalog and Workspace schemas.
Run schema-resetting packages serially.

## Acceptance Workflow

```sh
go test -tags=integration -count=1 ./apps/control-plane/internal/workspace/integration -run 'TestAcceptance'
```

The acceptance tests must demonstrate:

```text
Catalog publish -> public discovery
-> public Workspace create/install/inspect/lifecycle
-> internal exact resolution
-> terminal uninstall and new-identity reinstall
```

They use the real Catalog and Workspace stores/services and composed Gateway
handlers. A local sentinel server is used only to detect accidental Agent
requests; no Agent request is expected or made.

## Existing Integration Matrix

Run these packages serially against the same disposable `_test` database:

```sh
go test -tags=integration -count=1 ./apps/control-plane/internal/catalog/postgres
go test -tags=integration -count=1 ./apps/control-plane/internal/workspace/postgres
go test -tags=integration -count=1 ./apps/control-plane/internal/workspace/integration
go test -tags=integration -count=1 ./tests/integration/catalog
```

If `NEKIRO_TEST_DATABASE_URL` is missing, invalid, or not `_test`, report the
integration gate as not run. Do not convert that prerequisite failure into a
passing result.

## Static Verification

```sh
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./...
go mod tidy -diff
git diff --check
docker compose --file deploy/compose.yaml config --quiet
```

## Fallback Report

```text
Fallback delta: removed 0, retained 1, added 0, net 0
Added fallback evidence: none
```

The retained behavior is the active contract's legitimate empty Installation
list for an existing authorized Workspace. Issue #9 adds no fallback.
