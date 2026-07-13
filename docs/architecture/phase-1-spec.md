# Phase 1 Architecture Specification

## Objective

Phase 1 proves one complete platform loop:

```text
Register -> Discover -> Install -> Invoke -> Record
```

The deliverable is an Agent Operating Platform slice, not a marketplace catalog page. A user must be able to publish a versioned Agent Card, discover it by capability, install it into a workspace, invoke it through the A2A Router, and inspect the complete invocation lineage.

## Deployment Units

```text
Console -> Control Plane -> A2A Router -> Agents
                     \          |
                      \------ Ledger
```

- `apps/console` is the only user interface.
- `apps/control-plane` is one Phase 1 process containing Gateway, Catalog, Workspace, and Invocation Dispatch modules.
- `apps/a2a-router` is an independent data-plane process.
- Sample agents are independent A2A servers.
- PostgreSQL is the target persistent store. Logical module ownership applies even when modules share one database instance.

## Ownership

| Module | Owns | Must not own |
| --- | --- | --- |
| Gateway | Northbound HTTP boundary, caller/workspace context, response shape | Agent Card persistence, A2A transport |
| Registry | Agent Card versions and publication state | Running agents, invocation execution |
| Discovery | Query projection derived from published cards | A second source of truth |
| Workspace | Installations and accepted permissions | Agent deployment |
| Invocation Dispatch | Invocation identity and pre-dispatch authorization | A2A protocol execution |
| A2A Router | Resolution, transport, context propagation, timeout/cancel, event emission | Permanent Agent Card ownership |
| Ledger | Append-only invocation events and query projection | Routing or authorization decisions |

## Contract Packages

The `@nekiro/contracts` package exposes versioned subpaths:

- `@nekiro/contracts/agent-card`
- `@nekiro/contracts/platform-api`
- `@nekiro/contracts/internal-api`
- `@nekiro/contracts/a2a-profile`
- `@nekiro/contracts/events`
- `@nekiro/contracts/identifiers`
- `@nekiro/contracts/errors`
- `@nekiro/contracts/common`

Services must not exchange internal implementation types across a process boundary.

## Northbound API v1

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/v1/agents` | Register a draft Agent Card version |
| `POST` | `/v1/agents/:agentId/versions/:version/publish` | Publish an immutable version |
| `POST` | `/v1/agents/:agentId/versions/:version/disable` | Disable a version for new resolutions |
| `GET` | `/v1/agents` | Discover published agents by query/capability/owner |
| `GET` | `/v1/agents/:agentId/versions/:version` | Read an exact Agent Card version |
| `POST` | `/v1/workspaces/:workspaceId/installations` | Install and accept declared permissions |
| `PATCH` | `/v1/workspaces/:workspaceId/installations/:installationId` | Enable or disable an installation |
| `DELETE` | `/v1/workspaces/:workspaceId/installations/:installationId` | Uninstall an agent |
| `POST` | `/v1/workspaces/:workspaceId/invocations` | Authorize and dispatch an invocation |
| `GET` | `/v1/invocations/:invocationId` | Read one invocation and its events |
| `GET` | `/v1/traces/:traceId` | Read a complete parent/child invocation trace |

The Gateway returns the shared `PlatformError` shape for known failures. Dependency failure must never be represented as not found, an empty list, or success.

## Internal API v1

| Method | Path | Owner | Purpose |
| --- | --- | --- | --- |
| `POST` | `/internal/v1/resolve-agent` | Control Plane | Resolve an installed exact Agent Card and capability |
| `POST` | `/internal/v1/invocations` | Router | Accept an authorized invocation for A2A execution |
| `GET` | `/internal/v1/invocations/:id` | Router | Read Router-owned invocation facts |
| `GET` | `/internal/v1/traces/:traceId` | Router | Read invocation lineage |

The Router resolves cards through the internal Control Plane API. It must not query Registry tables directly.

## Invocation Lifecycle

```text
pending -> routing -> running -> succeeded
                            \-> failed
                            \-> canceled
                            \-> timed_out
```

Every invocation carries `invocation_id`, `root_task_id`, `trace_id`, and an optional `parent_invocation_id`. Agent-to-Agent calls create child invocations through the Router and preserve all lineage identifiers.

Ledger writes are append-only events. A mutable read projection may be derived from events, but it cannot replace the event history.

## A2A Profile

Phase 1 targets A2A `0.3.0` over JSON-RPC using the official `@a2a-js/sdk`. Agents expose `/.well-known/agent-card.json` and support:

- `message/send`
- `message/stream`
- `tasks/get`
- `tasks/cancel`

Platform trace and workspace context uses the headers declared in `contracts/a2a-profile`. Authentication credentials are resolved at the Router boundary and must never be persisted in Agent Card or Ledger payloads.

## Acceptance

The final E2E suite must prove:

1. A valid card can be registered and published; invalid cards and cards containing undeclared fields are rejected.
2. Discovery only returns eligible published versions and can filter by capability.
3. Installation requires explicit permission acceptance and resolves an exact version.
4. An installed agent can be invoked through the Router with streaming events.
5. An uninstalled, disabled, or unauthorized agent is rejected before dispatch.
6. Agent A can call Agent B through the Router and produce a complete parent-child trace.
7. Timeout, cancellation, route failure, A2A failure, and agent failure remain distinguishable in Ledger.
