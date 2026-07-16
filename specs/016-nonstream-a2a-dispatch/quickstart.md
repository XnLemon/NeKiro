# Quickstart: Non-Streaming A2A Dispatch

## Local verification

```powershell
go test -count=1 ./apps/a2a-router/internal/api ./apps/a2a-router/internal/transport/a2a ./agents/runtime-b
go test -count=1 ./...
go vet ./...
git diff --check
```

## Router startup prerequisites

The production Router assembly requires non-empty, strictly parsed
`NEKIRO_DATABASE_URL`, `NEKIRO_ROUTER_AGENT_RESPONSE_LIMIT_BYTES`, and
`NEKIRO_ROUTER_A2A_EVENT_LIMIT_BYTES`. The Ledger schema must already be
migrated by the deployment-owned migration step; Router startup checks schema
readiness and fails closed rather than auto-migrating or using a no-op appender.
The Compose file now includes `a2a-router-migrate` followed by the Router
`serve` service; standalone deployments must provide the equivalent migration
ordering before serving traffic.

## Optional race check with WSL

```powershell
wsl.exe -d Ubuntu-26.04 -- bash -lc 'cd /mnt/e/NeKiro && go test -race -count=1 ./apps/a2a-router/... ./agents/runtime-b'
```

## Optional PostgreSQL integration

Set `NEKIRO_TEST_DATABASE_URL` to a disposable PostgreSQL 17 database before
running integration-tagged Ledger/Router tests. Missing database configuration
is not success evidence.

```powershell
$env:NEKIRO_TEST_DATABASE_URL = "postgres://..."
go test -tags=integration -count=1 ./apps/a2a-router/internal/ledger
```

## Expected non-streaming behavior

1. Dispatch handler receives one valid `stream=false` internal request.
2. Router authenticates service caller and resolves the exact Agent through
   Control Plane Internal API.
3. Router calls Runtime B through A2A `message/send` with platform context.
4. Router commits required metadata-only Ledger facts.
5. Router returns one JSON result and never stores Agent result content.
