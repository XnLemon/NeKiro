# Quickstart: Invoke-to-Record Acceptance

## Local prerequisites

Docker Engine/Compose and Go 1.26 are required. The acceptance command is
intentionally fail-fast when Docker or PostgreSQL is unavailable.

```powershell
$env:POSTGRES_USER = "nekiro_acceptance"
$env:POSTGRES_PASSWORD = "acceptance-only-password"
$env:POSTGRES_DB = "nekiro_acceptance"
$env:POSTGRES_PORT = "55432"
$env:CONTROL_PLANE_PORT = "18080"
$env:A2A_ROUTER_PORT = "18081"
$env:RUNTIME_B_PORT = "18082"
$env:RUNTIME_A_PORT = "18083"
$env:NEKIRO_COMPOSE_DATABASE_URL = "postgresql://nekiro_acceptance:acceptance-only-password@postgres:5432/nekiro_acceptance?sslmode=disable"
docker compose --file deploy/compose.yaml up --build --detach
go test -tags=e2e -count=1 ./tests/e2e/invoke-record
docker compose --file deploy/compose.yaml down --volumes
```

## Static gates

```powershell
go test -count=1 ./...
go vet ./...
Push-Location agents/runtime-a; go test -count=1 ./...; go vet ./...; Pop-Location
docker compose --file deploy/compose.yaml config --quiet
```

The E2E report must include JSON, SSE, nested lineage, restart, isolation,
failure, secrecy, and 100-concurrent outcomes. A skipped Compose run is not a
passing acceptance result.

## Fallback audit

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
