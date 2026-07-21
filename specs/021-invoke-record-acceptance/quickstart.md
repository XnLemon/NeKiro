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
$env:NEKIRO_COMPOSE_DATABASE_URL = "postgresql://nekiro_acceptance:acceptance-only-password@postgres:5432/nekiro_acceptance?sslmode=disable"
$env:NEKIRO_DEV_AUTH_PRINCIPALS_JSON = '[{"id":"acceptance-owner","tokenSha256":"465aedffb32a2cb642cbca8fc75b806bcd33f703d70c49dcfb05e9db88df32d2"},{"id":"acceptance-user","tokenSha256":"2af4f9af4fa535905378ccee817aa532244dcf102f3d3ebeaf9a2a92abdeb42d"},{"id":"acceptance-other","tokenSha256":"7f85fe19123d4f88c475cb754dd30f422877ba3b3e2d5eed8ff8c2f9453ebeaf"}]'
$env:NEKIRO_INTERNAL_DEV_AUTH_PRINCIPALS_JSON = '[{"id":"router-internal","tokenSha256":"f9232718425b5ebee721187a79703448bce513ecf0600eb161f9256ddac27c4d"}]'
$env:NEKIRO_ROUTER_SERVICE_PRINCIPALS_JSON = '[{"id":"control-plane","tokenSha256":"5abfd00de27c6b2f57d45fdc90999134e4e088414ba1f39bf67ee0d1c9cec554"}]'
$env:NEKIRO_ROUTER_AGENT_PRINCIPALS_JSON = '[{"workspaceId":"workspace-acceptance","agentId":"runtime-a","tokenSha256":"e304d0370532633d535824a897d5c03445b636e8d1649064aa35a8fb50fef200"}]'
$env:NEKIRO_ROUTER_INTERNAL_BEARER_TOKEN = "router-internal-token"
$env:NEKIRO_CONTROL_PLANE_SERVICE_TOKEN = "control-plane-internal-token"
$env:RUNTIME_A_ROUTER_TOKEN = "runtime-a-router-token"
$env:NEKIRO_CONTROL_PLANE_INTERNAL_REQUEST_MAX_BYTES = "1048576"
$env:NEKIRO_GATEWAY_INVOCATION_REQUEST_MAX_BYTES = "1048576"
$env:NEKIRO_GATEWAY_SSE_EVENT_MAX_BYTES = "65536"
$env:NEKIRO_GATEWAY_METADATA_RESPONSE_MAX_BYTES = "1048576"
$env:NEKIRO_GATEWAY_INVOCATION_DEADLINE_MS = "30000"
$env:NEKIRO_ROUTER_INTERNAL_REQUEST_LIMIT_BYTES = "1048576"
$env:NEKIRO_ROUTER_AGENT_REQUEST_LIMIT_BYTES = "1048576"
$env:NEKIRO_ROUTER_CONTROL_PLANE_RESPONSE_LIMIT_BYTES = "1048576"
$env:NEKIRO_ROUTER_AGENT_RESPONSE_LIMIT_BYTES = "1048576"
$env:NEKIRO_ROUTER_A2A_EVENT_LIMIT_BYTES = "1048576"
$env:NEKIRO_ROUTER_SSE_EVENT_LIMIT_BYTES = "65536"
$env:NEKIRO_ROUTER_RESOLUTION_DEADLINE_MS = "30000"
$env:NEKIRO_ROUTER_AGENT_DEADLINE_MS = "30000"
$env:RUNTIME_A_RESPONSE_LIMIT_BYTES = "1048576"
$env:RUNTIME_A_EVENT_LIMIT_BYTES = "65536"
docker compose --file deploy/compose.yaml up --build --detach
$env:NEKIRO_E2E_CONTROL_PLANE_URL = "http://127.0.0.1:18080"
$env:NEKIRO_E2E_ROUTER_URL = "http://127.0.0.1:18081"
$env:NEKIRO_E2E_ROUTER_TOKEN = "router-internal-token"
$env:NEKIRO_E2E_OWNER_TOKEN = "acceptance-owner-token"
$env:NEKIRO_E2E_USER_TOKEN = "acceptance-user-token"
$env:NEKIRO_E2E_OTHER_TOKEN = "acceptance-other-token"
$env:NEKIRO_E2E_DATABASE_URL = "postgresql://nekiro_acceptance:acceptance-only-password@127.0.0.1:55432/nekiro_acceptance?sslmode=disable"
$env:NEKIRO_E2E_COMPOSE_FILE = (Resolve-Path deploy/compose.yaml).Path
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

## Verification evidence

Local static evidence on 2026-07-21:

```text
go test -count=1 ./...                                      PASS
go vet ./...                                                PASS
agents/runtime-a: go test -count=1 ./...                    PASS
agents/runtime-a: go vet ./...                              PASS
go test -tags=e2e -run '^$' ./tests/e2e/invoke-record          PASS (compile-only)
docker compose --file deploy/compose.yaml config --quiet    PASS
```

The local Docker client is installed, but its `desktop-linux` daemon is not
available (`failed to connect ... dockerDesktopLinuxEngine`). Therefore a real
Compose/PostgreSQL E2E result is not claimed locally; the required
`backend-acceptance` CI job is the authoritative clean-stack gate, and its
passing evidence is recorded below.

CI evidence: GitHub Actions run `29810057739` passed all checks on 2026-07-21,
including `go-quality`, `runtime-samples-quality` (nested Runtime A tests,
vet, and race), `workspace-integration`, `compose-config`, `frontend`, and
`backend-acceptance` (clean Compose/PostgreSQL build and the full E2E harness).
The run used an absolute Compose path and completed teardown with volumes.

## Fallback audit

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
