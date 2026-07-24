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
mappings: Agent Card `0.2`, Workspace `v1`, Installation `v2`, Control Plane
Northbound `v3` plus the Invocation `v4` companion, Control Plane Internal API
`v2` exact Card resolution plus `v3` nested installed-version resolution,
Router Internal dispatch API `v4` (metadata reads `v3`), Agent Router API `v1`, Invocation Event `0.3`,
Platform Error `v2` / `v3` / `v4` by owning surface, Invocation Result `v1`,
Result Stream Event `v2`, and A2A Profile Schema `0.2` for protocol `0.3.0`.
Router Invocation Credential `v1` is the active companion contract for the
Router-to-Agent HTTP hop.
Historical contract generations remain migration evidence; the runtime does
not add speculative dual-read behavior for them.

The first runnable Control Plane Catalog slice implements durable,
authenticated `Register -> Publish -> Discover -> Disable` behavior with
PostgreSQL, immutable Agent Card versions, exact reads, stable cursor
pagination, readiness, fixed errors, container wiring, and real
HTTP/PostgreSQL acceptance. Spec 003 now adds the durable owner-controlled
Workspace and Installation runtime: exact published SemVer selection,
permission snapshots, inspection pagination, lifecycle history, internal exact
resolution, separate internal authentication, migrations, and unit/HTTP
coverage. Invocation Dispatch now authorizes exact installations and forwards
live JSON/SSE only through the separately deployed A2A Router. The Router
performs controlled exact resolution, invokes the deterministic Runtime B A2A
sample, and records metadata-only append-only Ledger events with
Workspace-scoped Invocation/Trace reads.

Every managed outbound Agent request now uses a fresh Router-signed Ed25519
credential bound to the exact Workspace, Agent version, release/digest,
capability, Invocation, Task, parent lineage, Trace, and endpoint origin.
Both sample Runtimes verify the credential and reject direct execution before
runtime logic; stream cancellation receives a separate one-time `jti`.

Frontend Console work remains paused and `apps/console` is not yet present. The
thin Go Agent SDK, Router-owned nested adapter, isolated Runtime A, cross-Runtime
nested invocation, and process/Compose wiring are implemented. CI run
`30060752722` passed root build/test/race/vet/lint, Runtime A test/vet/race,
PostgreSQL integration, Compose configuration, Frontend, Codecov, and the real
authenticated Invoke-to-Record acceptance. The repository therefore proves
the backend/headless Phase 1 loop, but not yet the user-facing Console or the
later production governance and deployment integration stages.

The Go Workspace Client SDK under `sdks/client-sdk` is the application-facing
entry point for invoking an installed Agent through Gateway. One immutable
Client binds an explicit HTTP client, Gateway origin, Workspace, Owner-mapped
opaque credential, and byte limits; each call supplies only Agent ID,
capability, and JSON input. JSON/SSE results and Platform Error v4 responses are
strictly validated without direct Router/Agent routing or compatibility
fallback. See [Client SDK usage](sdks/client-sdk/README.md).

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
`contracts/`; PostgreSQL is the local persistence dependency.

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
Compose database URL, public/internal development principals, service tokens,
the Router Ed25519 signing identity and Agent verification key, Control Plane
and Router host ports, and request/event/deadline limits. No
required credential, identity, database, address, limit, or port has a runtime
fallback.

Validate the rendered Compose model without printing its environment, then
start PostgreSQL and wait for its health check. The second command intentionally
starts only the database; see the runbook for Control Plane and Router migration
and serving commands:

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
