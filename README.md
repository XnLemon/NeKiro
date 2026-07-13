# NeKiro Agent Operating Platform

NeKiro is a pnpm TypeScript monorepo for the first-stage Agent Operating
Platform loop:

```text
Register -> Discover -> Install -> Invoke -> Record
```

## Current status

The repository currently has the versioned contracts workspace and a local
PostgreSQL infrastructure baseline. The Console, Control Plane, A2A Router,
SDKs, sample Agents, and the complete end-to-end loop are not implemented in
the current tree.

The first-stage architecture keeps these boundaries:

```text
Console
  -> Northbound API
  -> Control Plane
       Gateway + Catalog + Workspace + Invocation Dispatch
  -> Internal API
  -> A2A Router
       Routing + Task Context + Transport + Policy Hooks + Ledger
  -> A2A Profile
  -> Agents
```

The Control Plane is one deployment unit with internal domain boundaries. The
A2A Router is a separate data-plane process. Cross-boundary data is defined in
`contracts/`; PostgreSQL is the local persistence dependency. This describes
the required first-stage shape, not a claim that every process already exists.

## Prerequisites

- Node.js 24 (CI uses 24.16.0)
- Corepack and pnpm 11.3.0
- Docker Engine with Docker Compose 2.20 or newer

## Local setup

From the repository root:

```powershell
corepack enable
pnpm install --frozen-lockfile
Copy-Item .env.example .env
```

Set all four required values in `.env`: `POSTGRES_USER`,
`POSTGRES_PASSWORD`, `POSTGRES_DB`, and `POSTGRES_PORT`. No credential or port
fallback is provided.

Validate the rendered Compose model without printing its environment, then
start PostgreSQL and wait for its health check:

```powershell
docker compose --env-file .env --file deploy/compose.yaml config --quiet
docker compose --env-file .env --file deploy/compose.yaml up --detach --wait postgres
```

Run the monorepo checks:

```powershell
pnpm typecheck
pnpm test
pnpm build
```

Stop the local dependency without deleting its persistent volume:

```powershell
docker compose --env-file .env --file deploy/compose.yaml down
```

See [Local development](docs/runbooks/local-development.md) for health,
persistence, and reset procedures.
