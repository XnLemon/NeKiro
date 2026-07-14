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

Choose non-empty values locally. Do not commit `.env`, reuse these credentials
for production, or place production credentials in this Compose deployment.
Missing and empty values fail during Compose interpolation; there are no
credential, database, host, principal, token, or port defaults. URL-encode
credential characters in `NEKIRO_COMPOSE_DATABASE_URL`. Generate each local
bearer token outside `.env`, place only its SHA-256 digest in the principal
JSON, and retain the raw token only in the invoking shell.

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
