# Local development

This runbook covers the repository's current local infrastructure baseline:
the Go backend workspace, frontend tooling, and one PostgreSQL container. It
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

Choose non-empty values locally. Do not commit `.env`, reuse these credentials
for production, or place production credentials in this Compose deployment.
Missing and empty values fail during Compose interpolation; there are no
credential, database, host, or port defaults.

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
docker compose --env-file .env --file deploy/compose.yaml ps postgres
docker compose --env-file .env --file deploy/compose.yaml logs postgres
```
