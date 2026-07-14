# NeKiro Agent Operating Platform

NeKiro is an Agent Operating Platform with a React Console and Go Control
Plane / A2A Router. Phase 1 proves this loop:

```text
Register -> Discover -> Install -> Invoke -> Record
```

## Product boundary

NeKiro operates independently built Agents from the outside. It owns versioned
registration, capability discovery, Workspace installation and permission
acceptance, managed routing, and cross-Agent invocation lineage. Agent Runtime
frameworks own model calls, prompts, tools, workflows, memory, RAG, sessions,
and runtime-internal telemetry.

Frameworks such as `trpc-agent-go` are complementary Runtime integrations, not
the implementation foundation of the Control Plane or A2A Router. The NeKiro
Agent SDK stays thin and covers Agent Card conformance, platform context
propagation, and nested calls through the Router.

Phase 1 must prove this boundary with at least two sample Agents backed by
different Runtime implementations. See
[Platform direction](docs/architecture/platform-direction.md) and
[ADR 0003](docs/decisions/0003-runtime-agnostic-platform-boundary.md).

## Current status

The repository has an active language-neutral contract set and its tested Go
mappings: Agent Card `0.2`, Workspace `v1`, Installation `v2`, Northbound API
`v3`, Control Plane Internal API `v2`, Router Internal API `v2`, Invocation
Event `0.2`, Platform Error `v2` / `v3`, Invocation Result and Result Stream Event
`v1`, and A2A Profile Schema `0.2` for protocol `0.3.0`. Historical `v1` and
`0.1` artifacts remain readable migration evidence; the runtime does not add
speculative dual-read behavior for them.

The first runnable Control Plane Catalog slice implements durable,
authenticated `Register -> Publish -> Discover -> Disable` behavior with
PostgreSQL, immutable Agent Card versions, exact reads, stable cursor
pagination, readiness, fixed errors, container wiring, and real
HTTP/PostgreSQL acceptance. Spec 003 now adds the durable owner-controlled
Workspace and Installation runtime: exact published SemVer selection,
permission snapshots, inspection pagination, lifecycle history, internal exact
resolution, separate internal authentication, migrations, and unit/HTTP
coverage. The cross-Runtime fixtures still prove metadata portability only; no
Agent endpoint is invoked.

Frontend work remains paused. Invocation Dispatch, the A2A Router, Ledger,
SDKs, live sample Agents, and the complete end-to-end loop remain
unimplemented.

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

- Go 1.26 or newer
- Node.js 24 for frontend tooling (CI uses 24.16.0)
- Corepack and pnpm 11.3.0
- Docker Engine with Docker Compose 2.20 or newer

## Local setup

From the repository root:

```powershell
corepack enable
pnpm install --frozen-lockfile
Copy-Item .env.example .env
```

Set every required value in `.env`: PostgreSQL bootstrap values, the explicit
Compose database URL, a development principal digest array, and the Control
Plane host port. Raw bearer tokens do not belong in `.env`; no credential,
identity, database, address, or port fallback is provided.

Validate the rendered Compose model without printing its environment, then
start PostgreSQL and wait for its health check. The second command intentionally
starts only the database; see the runbook for migration and Control Plane
commands:

```powershell
docker compose --env-file .env --file deploy/compose.yaml config --quiet
docker compose --env-file .env --file deploy/compose.yaml up --detach --wait postgres
```

Run the monorepo checks:

```powershell
go test ./...
go vet ./...
pnpm typecheck
pnpm test
pnpm build
```

Stop the local dependency without deleting its persistent volume:

```powershell
docker compose --env-file .env --file deploy/compose.yaml down
```

See [Local development](docs/runbooks/local-development.md) for health,
persistence, migration, dedicated integration-database safeguards, and reset
procedures.
