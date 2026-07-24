# Local development

This runbook covers the runnable Catalog and Workspace/Installation slices in
the Go Control Plane, the frontend tooling baseline, and local PostgreSQL. It
is not a production deployment configuration.

## Requirements

- Go 1.26 or newer
- Node.js 24; CI is pinned to 24.16.0
- Corepack with pnpm 11.3.0
- Docker Engine and Docker Compose 2.20 or newer

Run all commands from the repository root.

## Configure PostgreSQL

Create the ignored local environment file:

```powershell
Copy-Item .env.example .env
```

Every current variable is required:

| Variable | Purpose |
| --- | --- |
| `POSTGRES_USER` | PostgreSQL bootstrap role for this local environment |
| `POSTGRES_PASSWORD` | Explicit credential for the bootstrap role |
| `POSTGRES_DB` | Database created during first initialization |
| `POSTGRES_PORT` | Available host port bound to container port 5432 |
| `NEKIRO_COMPOSE_DATABASE_URL` | Explicit PostgreSQL URL used inside Compose; its host is normally `postgres` |
| `NEKIRO_DEV_AUTH_PRINCIPALS_JSON` | Strict local principal array containing `id` and lowercase SHA-256 `tokenSha256` only |
| `NEKIRO_INTERNAL_AUTH_MODE` | Explicit internal service authentication mode; currently `development-static` |
| `NEKIRO_INTERNAL_DEV_AUTH_PRINCIPALS_JSON` | Separate strict principal array for Router/internal callers |
| `CONTROL_PLANE_PORT` | Available host loopback port for the Control Plane |
| `NEKIRO_ROUTER_INTERNAL_URL` | Fixed Router v4 dispatch URL used by the Control Plane; Compose sets the service origin |
| `NEKIRO_ROUTER_INTERNAL_BEARER_TOKEN` | Raw local token the Control Plane presents to the Router; keep it out of logs and commits |
| `NEKIRO_GATEWAY_INVOCATION_REQUEST_MAX_BYTES` | Maximum northbound invocation request body size |
| `NEKIRO_GATEWAY_SSE_EVENT_MAX_BYTES` | Maximum northbound SSE event/frame size |
| `NEKIRO_GATEWAY_METADATA_RESPONSE_MAX_BYTES` | Maximum Router metadata response body size buffered for validation |
| `NEKIRO_GATEWAY_INVOCATION_DEADLINE_MS` | Gateway invocation and metadata read deadline in milliseconds |
| `NEKIRO_ROUTER_LISTEN_ADDRESS` | Router bind address and port for host-process serving |
| `NEKIRO_ROUTER_SERVICE_PRINCIPALS_JSON` | Strict principal array trusted by the Router's internal endpoint |
| `NEKIRO_CONTROL_PLANE_RESOLVE_URL` | Control Plane exact-resolution URL used by the Router |
| `NEKIRO_CONTROL_PLANE_SERVICE_TOKEN` | Raw service token the Router presents to the Control Plane; keep it out of logs and commits |
| `NEKIRO_ROUTER_INTERNAL_REQUEST_LIMIT_BYTES` | Maximum Router dispatch request body size |
| `NEKIRO_ROUTER_CONTROL_PLANE_RESPONSE_LIMIT_BYTES` | Maximum Control Plane resolution response size |
| `NEKIRO_ROUTER_AGENT_RESPONSE_LIMIT_BYTES` | Maximum non-streaming Agent response size |
| `NEKIRO_ROUTER_A2A_EVENT_LIMIT_BYTES` | Maximum A2A event size reserved for the streaming profile |
| `NEKIRO_ROUTER_SSE_EVENT_LIMIT_BYTES` | Maximum serialized SSE event/frame size; independent from the A2A event limit |
| `NEKIRO_ROUTER_RESOLUTION_DEADLINE_MS` | Resolution deadline in milliseconds |
| `NEKIRO_ROUTER_AGENT_CREDENTIAL_ISSUER` | Canonical HTTPS Router signing issuer |
| `NEKIRO_ROUTER_AGENT_CREDENTIAL_KEY_ID` | Exact safe identifier placed in JWT `kid` |
| `NEKIRO_ROUTER_AGENT_CREDENTIAL_PRIVATE_KEY_BASE64URL` | 64 raw Ed25519 private-key bytes encoded as unpadded Base64url |
| `NEKIRO_ROUTER_AGENT_CREDENTIAL_TTL_SECONDS` | Required integer lifetime from 1 through 300 seconds |
| `NEKIRO_AGENT_ROUTER_ISSUER` | Exact Router issuer trusted by each Agent adapter |
| `NEKIRO_AGENT_ROUTER_KEY_ID` | Exact Router key ID trusted by each Agent adapter |
| `NEKIRO_AGENT_ROUTER_PUBLIC_KEY_BASE64URL` | 32 raw Ed25519 public-key bytes encoded as unpadded Base64url |

Choose non-empty values locally. Do not commit `.env`, reuse these credentials
for production, or place production credentials in this Compose deployment.
Missing and empty values fail during Compose interpolation; there are no
credential, database, host, principal, token, or port defaults. URL-encode
credential characters in `NEKIRO_COMPOSE_DATABASE_URL`. Generate each local
bearer token outside `.env`, place only its SHA-256 digest in the principal
JSON, and retain the raw token only in the invoking shell.

Generate one Ed25519 key pair with an audited key-management tool. Export the
canonical 64-byte private value and 32-byte public value as unpadded Base64url;
PEM text, padded Base64, whitespace repair, default keys, and multi-key lookup
are not accepted. Keep the private value only on the Router. Compose sets each
sample Agent's audience to its exact service origin (`http://runtime-a:8091`
or `http://runtime-b:8092`).

Validate the configuration without rendering secrets to the terminal:

```powershell
docker compose --env-file .env --file deploy/compose.yaml config --quiet
```

## Start and verify

Start PostgreSQL and wait until Docker reports it healthy:

```powershell
docker compose --env-file .env --file deploy/compose.yaml up --detach --wait postgres
```

Inspect container state and run PostgreSQL's readiness probe inside the
container:

```powershell
docker compose --env-file .env --file deploy/compose.yaml ps postgres
docker compose --env-file .env --file deploy/compose.yaml exec postgres sh -ec 'pg_isready --username "$POSTGRES_USER" --dbname "$POSTGRES_DB"'
```

PostgreSQL is bound only to `127.0.0.1` on `POSTGRES_PORT`. The container also
joins the isolated `platform-internal` network for future platform processes.
The project-scoped `local-access` bridge exists only to make the loopback port
binding reachable from the host.

## Migrate and run the Catalog

For a host process, set every required application variable explicitly:

```powershell
$env:NEKIRO_DATABASE_URL = 'postgresql://<user>:<url-encoded-password>@127.0.0.1:<port>/<database>?sslmode=disable'
$env:NEKIRO_LISTEN_ADDRESS = '127.0.0.1:18080'
$env:NEKIRO_AUTH_MODE = 'development-static'
$env:NEKIRO_DEV_AUTH_PRINCIPALS_JSON = '<strict principal JSON from .env>'
$env:NEKIRO_INTERNAL_AUTH_MODE = 'development-static'
$env:NEKIRO_INTERNAL_DEV_AUTH_PRINCIPALS_JSON = '<separate strict internal principal JSON from .env>'
$env:NEKIRO_ROUTER_INTERNAL_URL = 'http://127.0.0.1:18081/internal/v4/invocations'
$env:NEKIRO_ROUTER_INTERNAL_BEARER_TOKEN = '<raw token matching the configured Router principal digest>'
$env:NEKIRO_GATEWAY_INVOCATION_REQUEST_MAX_BYTES = '1048576'
$env:NEKIRO_GATEWAY_SSE_EVENT_MAX_BYTES = '1048576'
$env:NEKIRO_GATEWAY_METADATA_RESPONSE_MAX_BYTES = '1048576'
$env:NEKIRO_GATEWAY_INVOCATION_DEADLINE_MS = '30000'
go run ./apps/control-plane/cmd/control-plane migrate up
go run ./apps/control-plane/cmd/control-plane serve
```

`serve` verifies schema version and dependency readiness but never creates or
upgrades schema. `migrate up` is the sole migration command; `migrate down` is
rejected before schema or data changes. The process exposes
`/livez` and `/readyz`; the authenticated Catalog operations are under
`/v3/agents`, Workspace/Installation operations are under `/v3/workspaces`,
and exact internal resolution is under `/internal/v2/resolve-agent`. The
internal route accepts only the separately configured internal principal.

To run the containerized local deployment, Compose executes the distinct
`control-plane-migrate` one-shot service before `control-plane`:

```powershell
docker compose --env-file .env --file deploy/compose.yaml up --detach --wait
docker compose --env-file .env --file deploy/compose.yaml ps
```

Committed Catalog rows remain in the PostgreSQL volume across process and
container restarts. Logs and fixed errors omit bearer credentials, principal
digests, Card bodies, schemas, SQL, DSNs, and raw dependency details.

## Migrate and run the A2A Router

The Router owns its Ledger schema. Apply its migration before starting the
server, and keep the same PostgreSQL URL as the Control Plane:

```powershell
$env:NEKIRO_DATABASE_URL = 'postgresql://<user>:<url-encoded-password>@127.0.0.1:<port>/<database>?sslmode=disable'
go run ./apps/a2a-router/cmd/a2a-router migrate up
```

For a host process, set the Router's remaining required values explicitly:

```powershell
$env:NEKIRO_ROUTER_LISTEN_ADDRESS = '127.0.0.1:18081'
$env:NEKIRO_ROUTER_SERVICE_PRINCIPALS_JSON = '<strict Router principal JSON from .env>'
$env:NEKIRO_CONTROL_PLANE_RESOLVE_URL = 'http://127.0.0.1:18080/internal/v2/resolve-agent'
$env:NEKIRO_CONTROL_PLANE_SERVICE_TOKEN = '<raw token matching the configured internal principal digest>'
$env:NEKIRO_ROUTER_INTERNAL_REQUEST_LIMIT_BYTES = '1048576'
$env:NEKIRO_ROUTER_CONTROL_PLANE_RESPONSE_LIMIT_BYTES = '1048576'
$env:NEKIRO_ROUTER_AGENT_RESPONSE_LIMIT_BYTES = '1048576'
$env:NEKIRO_ROUTER_A2A_EVENT_LIMIT_BYTES = '1048576'
$env:NEKIRO_ROUTER_SSE_EVENT_LIMIT_BYTES = '1048576'
$env:NEKIRO_ROUTER_RESOLUTION_DEADLINE_MS = '5000'
$env:NEKIRO_ROUTER_AGENT_CREDENTIAL_ISSUER = 'https://a2a-router.nekiro.example'
$env:NEKIRO_ROUTER_AGENT_CREDENTIAL_KEY_ID = '<safe-key-id>'
$env:NEKIRO_ROUTER_AGENT_CREDENTIAL_PRIVATE_KEY_BASE64URL = '<64-byte-private-key>'
$env:NEKIRO_ROUTER_AGENT_CREDENTIAL_TTL_SECONDS = '30'
go run ./apps/a2a-router/cmd/a2a-router serve
```

`serve` checks Ledger schema readiness and dependency configuration but never
creates or upgrades schema. The Router exposes `/readyz` and accepts internal
dispatches at `/internal/v4/invocations`. Its non-streaming input and output
limits are the minimum of the configured limit and the exact Agent Card limit.
Missing or invalid values fail startup; there is no no-op Ledger, fallback
endpoint, or default credential.

Each Agent process separately requires `NEKIRO_AGENT_ROUTER_ISSUER`,
`NEKIRO_AGENT_ROUTER_AUDIENCE`, `NEKIRO_AGENT_ROUTER_KEY_ID`, and
`NEKIRO_AGENT_ROUTER_PUBLIC_KEY_BASE64URL`. Readiness and ownership challenge
proof remain public process routes. A2A execution, streaming, cancellation,
and the Runtime B unavailable fixture reject a missing/invalid credential with
the fixed no-store 401 response; a valid credential with the wrong audience or
bound context receives the fixed no-store 403 response. Credentials, keys, and
`jti` values must never be logged or persisted in Ledger.

## Integration acceptance

Use only a dedicated database whose name ends in `_test`. The suite drops the
owned schemas, applies migrations, and owns all data in that database:

```powershell
$env:NEKIRO_TEST_DATABASE_URL = 'postgresql://<user>:<url-encoded-password>@127.0.0.1:<port>/<database>_test?sslmode=disable'
go test -tags=integration -count=1 ./tests/integration/catalog
go test -tags=integration -count=1 ./apps/control-plane/internal/workspace/postgres
```

The suffix guard is mandatory and prevents accidental execution against a
shared, staging, or production database.

## Clean trusted-publication acceptance

The authoritative backend acceptance starts from an isolated Compose project
with empty volumes and proves Register -> Verify -> Publish -> Discover ->
Install -> Invoke -> Record, cross-runtime nested lineage, the full negative
trust matrix, and metadata/log secrecy. Use the explicit environment from the
`backend-acceptance` CI job and follow
[the Spec 026 quickstart](../../specs/026-trusted-publication-acceptance/quickstart.md).

Do not reuse the ordinary local Compose project or database. Set a unique
`NEKIRO_E2E_COMPOSE_PROJECT`, keep the challenge verification timeout shorter
than its explicit TTL, and always run teardown with `--volumes` even after a
failed test. Provider, Workspace-owner, and operator diagnosis and recovery are
documented in the
[trusted-publication operations runbook](trusted-publication-operations.md).

## Install and verify the workspace

```powershell
corepack enable
pnpm install --frozen-lockfile
go test ./...
go vet ./...
pnpm typecheck
pnpm test
pnpm build
```

The frozen install fails when `pnpm-lock.yaml` and workspace manifests disagree.

## Stop or reset

Stop containers while retaining database data:

```powershell
docker compose --env-file .env --file deploy/compose.yaml down
```

The named `postgres-data` volume persists across `down` and container
recreation. PostgreSQL bootstrap variables apply only when that volume is
empty. Changing credentials or the bootstrap database in `.env` does not
rewrite an initialized database.

To intentionally delete all local database data and initialize from the
current `.env` on the next start:

```powershell
docker compose --env-file .env --file deploy/compose.yaml down --volumes
```

This reset is destructive. For startup diagnosis, inspect the service state
and logs directly:

```powershell
docker compose --env-file .env --file deploy/compose.yaml ps
docker compose --env-file .env --file deploy/compose.yaml logs postgres control-plane-migrate control-plane
```
