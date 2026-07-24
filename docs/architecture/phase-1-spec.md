# Phase 1 Architecture Specification

## Objective

Phase 1 proves one complete platform loop:

```text
Register -> Discover -> Install -> Invoke -> Record
```

The deliverable is an Agent Operating Platform slice, not a marketplace catalog page. A user must be able to publish a versioned Agent Card, discover it by capability, install it into a workspace, invoke it through the A2A Router, and inspect the complete invocation lineage.

## Product Boundary

Phase 1 operates independently implemented Agents as protocol-facing black
boxes. Agent Runtimes own model, prompt, tool, planner, workflow, memory, RAG,
session, and runtime telemetry behavior. NeKiro owns publication, discovery,
Workspace authorization, exact-version resolution, managed routing, and
platform-level invocation lineage.

Full Agent Runtime frameworks must not become Control Plane or A2A Router core
dependencies. They may be used by sample Agents or isolated adapters. The
Phase 1 proof uses at least two sample Agents backed by different Runtime
implementations so the platform cannot pass acceptance by relying on one
framework's internal types or storage.

## Deployment Units

```text
Console -> Control Plane -> A2A Router -> Agents
                     \          |
                      \------ Ledger
```

- `apps/console` is the only user interface.
- `apps/control-plane` is one Go process containing Gateway, Catalog, Workspace, and Invocation Dispatch modules.
- `apps/a2a-router` is an independent Go data-plane process.
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
| A2A Router | A2A transport, transient result forwarding, context propagation, timeout/cancel, event emission | Permanent Agent Card ownership, Registry or Workspace storage access |
| Ledger | Append-only invocation events and query projection | Routing or authorization decisions |

## Contract Sources

Cross-language contracts are owned by language-neutral artifacts:

- `contracts/schemas/` contains versioned JSON Schema documents.
- `contracts/openapi/control-plane.v3.yaml` defines the active Catalog,
  Discovery, Workspace, and Installation Northbound API; `control-plane.v2.yaml`
  remains unchanged migration evidence. Any legacy Invocation paths still
  present in the v3 document are migration evidence and are not served by the
  current Gateway.
- `contracts/openapi/control-plane-invocation.v4.yaml` defines the active
  Invocation and Trace Northbound API.
- `contracts/openapi/control-plane-internal.v2.yaml` defines Router-to-Control Plane exact Agent resolution; `control-plane-internal.v3.yaml` defines nested installed-version resolution.
- `contracts/openapi/router-internal.v4.yaml` defines active Control Plane-to-Router dispatch and result transport; `router-metadata.v3.yaml` is the active Workspace-scoped Invocation/Trace read contract, while the complete `router-internal.v3.yaml` is historical migration evidence.
- `contracts/a2a-profile/v0.3.0/profile.v0.2.json` pins the active supported A2A subset and context headers.
- `contracts/router-agent-credential/v1/` and
  `contracts/schemas/router-agent-credential.v1.schema.json` define the
  separately versioned signed Router-to-Agent request binding without changing
  A2A Profile Schema `0.2`.
- `contracts/*.go` maps these contracts into Go and verifies the mapping against the source schemas.

Go and TypeScript types are consumers of these artifacts, never competing sources of truth. Services must not exchange internal implementation types across a process boundary.

Historical v1 files remain unchanged as migration evidence. The first backend
runtime implements only the active versions and does not introduce speculative
dual-version behavior.

## Router-to-Agent authentication

The Router signs a fresh compact Ed25519 JWT for every A2A HTTP request. The
credential binds the canonical endpoint origin, exact Agent/Card release,
Workspace authorization context, capability, Invocation/Task/Trace lineage,
and optional parent Invocation. Agents accept one configured issuer, audience,
key ID, and public key, reject replayed `jti` values atomically in-process, and
execute runtime logic only after all claims match single-valued context
headers. Stream and cancel requests use different credentials. No credential,
key, signature, or `jti` enters Agent Card, result, event, or Ledger storage.

## Northbound API v3: Catalog and Workspace

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/v3/agents` | Register a draft Agent Card v0.2 version |
| `POST` | `/v3/agents/:agentId/versions/:version/publish` | Publish an immutable version |
| `POST` | `/v3/agents/:agentId/versions/:version/disable` | Disable a version for new resolutions |
| `GET` | `/v3/agents` | Discover published agents by query/capability/owner |
| `GET` | `/v3/agents/:agentId/versions/:version` | Read an exact Agent Card version |
| `POST` | `/v3/workspaces` | Create a minimal owner-controlled Workspace |
| `GET` | `/v3/workspaces/:workspaceId` | Read an owned Workspace |
| `POST` | `/v3/workspaces/:workspaceId/installations` | Install and accept declared permissions |
| `GET` | `/v3/workspaces/:workspaceId/installations` | List current and historical Installations |
| `GET` | `/v3/workspaces/:workspaceId/installations/:installationId` | Read one exact Installation |
| `PATCH` | `/v3/workspaces/:workspaceId/installations/:installationId` | Enable or disable an installation |
| `DELETE` | `/v3/workspaces/:workspaceId/installations/:installationId` | Uninstall and return preserved history |

The Gateway returns Platform Error v2 for Catalog failures and Platform Error v3
for Workspace/Installation failures. Public messages are fixed by error code and
cannot contain internal dependency errors, credentials, request payloads, or
Agent output. `INSTALLATION_DISABLED` identifies Workspace authorization state
while `AGENT_DISABLED` identifies Catalog version state.

## Northbound Invocation API v4

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/v4/workspaces/:workspaceId/invocations` | Authorize, dispatch, and return a transient JSON or SSE result |
| `GET` | `/v4/workspaces/:workspaceId/invocations/:invocationId` | Read one Workspace-scoped invocation and metadata-only Ledger events |
| `GET` | `/v4/workspaces/:workspaceId/traces/:traceId` | Read Workspace-scoped metadata-only parent/child invocation lineage |

The Invocation Gateway uses Platform Error v4 after the runtime acceptance
boundary. Trace correlation is required; Invocation and root Task correlation
are present together after Invocation creation. Dependency failure must never be
represented as not found, an empty list, or success.

## Directional Internal APIs

| Method | Path | Owner | Purpose |
| --- | --- | --- | --- |
| `POST` | `/internal/v2/resolve-agent` | Control Plane | Resolve an authorized installed exact Agent Card v0.2 and capability |
| `POST` | `/internal/v3/resolve-installed-version` | Control Plane | Resolve the exact enabled Installation pin for a nested call |
| `POST` | `/internal/v4/invocations` | Router | Execute an authorized root invocation and return a transient JSON or SSE result |
| `GET` | `/internal/v3/workspaces/:workspaceId/invocations/:invocationId` | Router | Read Workspace-scoped metadata-only Invocation detail |
| `GET` | `/internal/v3/workspaces/:workspaceId/traces/:traceId` | Router | Read Workspace-scoped metadata-only lineage |

Control Plane Internal v2/v3 are served by the Control Plane and called by the
Router. Router Internal dispatch v4 is served by the Router and called by the Control
Plane. Their server destinations are distinct and explicitly configured. The
Router resolves cards through the internal Control Plane API and must not query
Registry or Workspace tables directly.

## Invocation Result Delivery

`POST /v4/workspaces/:workspaceId/invocations` is the only Northbound result
channel. `stream=false` returns one `application/json` Invocation Result v1.
`stream=true` returns ordered `text/event-stream` Invocation Result Stream
Event v2 values on the same response. The request mode and `Accept` header must
agree; mismatch returns `406 NOT_ACCEPTABLE`.

A clean stream begins with `accepted` and ends with exactly one `completed`,
`failed`, `canceled`, or `timed_out` event. Event and chunk order is zero-based
and monotonic. The first terminal outcome is immutable. EOF before a terminal
event is interrupted delivery, and chunks before a non-success terminal event
are incomplete output.

Results and chunks are transient arbitrary JSON values constrained by the
resolved Skill output schema. Phase 1 has no result persistence, polling,
replay, reconnect cursor, or result query endpoint. A caller may inspect Ledger
facts after disconnect but must create a new Invocation to receive output.

## Invocation Lifecycle

```text
pending -> routing -> running -> succeeded
                            \-> failed
                            \-> canceled
                            \-> timed_out
```

Every invocation carries `invocation_id`, `root_task_id`, `trace_id`, and an optional `parent_invocation_id`. Agent-to-Agent calls create child invocations through the Router and preserve all lineage identifiers.

Ledger writes are append-only Invocation Event v0.3 facts. Terminal event type,
status, and error code must agree: `TIMEOUT` belongs only to `timed_out`,
`CANCELED` belongs only to `canceled`, and `failed` excludes both. Agent input,
result, and chunk content are forbidden. A mutable read projection may be
derived from events, but it cannot replace the event history.

## A2A Profile

Phase 1 targets A2A `0.3.0` over JSON-RPC using `github.com/a2aproject/a2a-go` `v0.3.15`. Agents expose `/.well-known/agent-card.json` and support:

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
4. An installed agent can be invoked through the Router and returns the exact non-streaming result or ordered streaming result events through the Gateway.
5. An uninstalled, disabled, or unauthorized agent is rejected before dispatch.
6. Two sample Agents backed by different Runtime implementations can be
   registered and installed; Agent A can call Agent B through the Router and
   produce a complete parent-child trace without shared Runtime internals.
7. Timeout, cancellation, route failure, A2A failure, and agent failure remain distinguishable in Ledger without persisting Agent input or output.
8. Router resolution reaches only the Control Plane destination, while dispatch and Ledger queries reach only the Router destination.
9. Both sample Agents pass the same A2A Profile conformance and are invoked
   without framework-specific Control Plane or Router behavior.
