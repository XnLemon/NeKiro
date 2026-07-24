# ADR 0006: Invocation Runtime Trust and Failure Policy

- Status: Accepted
- Date: 2026-07-16
- Decision owners: Gateway, Invocation Dispatch, A2A Router, Ledger, Agent SDK
- Feature: Spec 011 Invocation Runtime Contracts / GitHub #20

## Context

The planned runtime needs a trust boundary for nested Agent calls, a Phase 1
policy for Card authentication types without Card secrets, an exact accepted
Invocation point, and deterministic deadline/cancel/size/SSE semantics. Router
Internal v2 describes only Control Plane callers and does not settle these
questions. Guessing them independently in runtime children would create caller
forgery, secret fallback, false Ledger success, or incompatible APIs.

## Decision

### Distinct Agent-facing direction

`router-agent.v1.yaml` is the only Agent SDK destination. An explicit opaque
Bearer credential is bound by required Router deployment configuration to one
exact `(Workspace, Agent)` pair. The same Agent installed in two Workspaces
requires two distinct credential bindings. Missing, invalid, duplicate, or
mismatched bindings fail before acceptance. The credential and any
fingerprint/locator are prohibited from
Cards, DTOs, Ledger, errors, and logs.

The nested request contains only parent Invocation ID, target Agent,
capability, input, and result mode. Router loads one committed parent, requires
it to be running and both its Workspace and target Agent to equal the
authenticated principal, then generates the child ID and derives Workspace,
root Task, Trace, parent, and caller facts. Request-supplied trusted fields are
structurally rejected. The SDK has no endpoint field and cannot bypass Router.

Control Plane Internal v3 resolves the exact installed version for nested
calls. Its errors are phase-aware: `401` is pre-correlation; `403`, `404`, and
`503` require exact request Invocation/root Task/Trace correlation; `400` may
use either complete shape. Undeclared statuses, status/code mismatches,
asymmetric identifiers, and correlation changes are invalid dependency
responses.

### Phase 1 Agent transport authentication

Only Agent Card 0.2 `authentication.type=none` is invocable. Registry continues
to accept and expose `api_key`, `http_bearer`, `oauth2_client_credentials`, and
`mutual_tls` metadata. At invocation time those four types fail from `routing`
with Platform Error v4 `AGENT_AUTH_UNSUPPORTED` and fixed message `The Agent
authentication type is not supported for invocation.` No Agent request is
made. The outcome is HTTP 502 before response commitment or a correlated
`failed` Result Stream Event v2 after SSE commitment. It is never remapped to
route-not-found, unavailable, dependency failure, anonymous access, or empty
credentials.

Platform Error v4 has two closed shapes. `preCorrelation` contains only code,
fixed message, and safe boundary Trace. `correlated` additionally requires
Invocation ID and root Task ID. Every HTTP or in-band failure after the
successful `created` commit uses `correlated`; these identifiers are never
optional after acceptance. Responses that can occur on either side of the
boundary use an explicit phase union and runtime selects by commit state.

### Acceptance and Ledger persistence

The successful Router-owned `created` append/projection transaction is the
accepted-Invocation boundary. Authentication, validation, media negotiation,
Workspace policy, Router connectivity, and initial Ledger failure before this
commit create no Ledger fact. No Agent side effect may precede it.

Normal exact-resolution, route, and authentication failure can terminalize
from routing. Cancellation/timeout can terminalize from pending, routing, or
running. Success can terminalize only from running. A clean JSON result or
clean terminal SSE event is emitted only after the corresponding terminal
Ledger transaction commits.

If Ledger persistence fails after an Agent side effect, the live operation is
an explicit correlated `DEPENDENCY_ERROR`: HTTP 503 while the response is
uncommitted, or a `failed` Result Stream Event v2 if SSE is committed and still
writable. The result-stream failure describes delivery, not a Ledger terminal.
Durable metadata remains at the last committed non-terminal fact. The Router
does not fabricate a terminal event/projection, report success, retry the
write/Agent call, switch stores, or reconcile in the background. Metadata reads
return the non-terminal history unchanged.

### Deadline and cancellation

Gateway/Router deadline configuration is required, strict base-10 decimal, and
within `1..600000` milliseconds. It has no default. The effective deadline is
the lesser configured duration and exact resolved Card `timeoutMs`.

HTTP disconnect and deadline expiry cancel local work. If an A2A task ID is
known, Router sends at most one `tasks/cancel` request. This is one protocol
propagation attempt, not a retry. It is not repeated after transport failure,
task-not-found, or task-not-cancelable. Those outcomes do not replace a local
canceled/timed-out winner. The first successfully committed terminal Ledger
transaction wins; later Agent/cancel/deadline results produce no fact/response.

### Size and SSE framing

Public request, internal request, Agent response, A2A event, and SSE event byte
limits are required deployment configuration, strict base-10 decimal integers
within `1..2147483647`, and have no defaults. Empty, whitespace-padded, signed,
fractional, exponent, overflow, and out-of-range values fail owning-process
startup/readiness. Effective Agent input/output bounds are the lesser configured
and exact Card limits.

Oversized request bodies fail before acceptance with HTTP 413 Platform Error v4
`PAYLOAD_TOO_LARGE`. Oversize detected after acceptance must commit the exact
non-success terminal `AGENT_RESPONSE_TOO_LARGE` before returning HTTP 502 or an
in-band `failed` event; a Ledger failure follows the
interruption rule above. No truncation is a success.

Router Agent-response and A2A-event byte limits are separate required
no-default settings in the same strict `1..2147483647` range. Neither is
inferred from the request/SSE limit. The resolved Card output bound also
participates in the effective Agent response limit.

Every result event is compact UTF-8 JSON on exactly one `data:` line followed
by exactly one blank line and an immediate flush. Literal CR/LF is JSON escaped.
Multiple data lines, other SSE fields, malformed JSON, oversized data values,
missing blank delimiters, and EOF without a terminal result event are invalid.
The Go Agent SDK requires explicit JSON-response and SSE-event limits, exposes
SSE incrementally, and validates framing, event schema, correlation, sequence,
chunk indexes, and terminal completion. It retains only validated safe Platform
Error v4 fields and never returns a raw Router error body.

### Shared media negotiation

All Northbound, Router Internal, and Agent Router invoke operations execute one
strict rule. `stream=false` accepts exactly one ASCII media range equal to
`application/json`, `application/*`, or `*/*`. `stream=true` accepts exactly
`text/event-stream`. Blank values, parameters, comma-separated alternatives,
case variants, whitespace variants, and every other value fail before
acceptance with `NOT_ACCEPTABLE`. The versioned conformance corpus is the
executable source for this matrix.

### Workspace-scoped metadata reads

Northbound v4 and Router Metadata v3 Invocation/Trace reads require
`workspaceId` in the path. Invocation reads return a projection plus ordered
Event 0.3 facts. Trace reads return `traceId` plus ordered Invocation
projections preserving parent-child lineage. The supplied Workspace is the
query-isolation key; global raw-event routes do not exist. This restores the
v3 projection/lineage behavior while making Workspace-first authorization
structurally required.

### Executable semantics

The Go consumer exports validators for Platform Error v4 pre/correlated
shapes, Invocation Event 0.3, Result Stream Event v2, nested child correlation,
lifecycle sequences, Result Stream event/chunk/first-terminal sequences, and
media negotiation. Every nested Event/Stream error must repeat the outer
Invocation/root Task/Trace exactly. A versioned positive/negative
corpus covers fixed code/message pairs, required post-acceptance correlation,
stable lineage/context, legal transitions, event/chunk order, first terminal,
and the media matrix. JSON Schema alone is not treated as lifecycle evidence.
Result Stream validation completes only through `Finish`; EOF before terminal
is `ErrRuntimeStreamInterrupted`, including accepted-only and accepted-plus-
chunk streams. Trace validation requires deterministic parent-before-child
order, forbids self-parent/cycles, and preserves one root Task ID throughout
the lineage.

## Compatibility

- Northbound Invocation v4 is an invocation-only companion at the existing
  Gateway destination. Catalog, Workspace, and Installation remain exclusively
  described and served by `control-plane.v3.yaml`; this is not a full Control
  Plane v4 replacement.
- Router Internal Dispatch v4 replaces the historical v3 dispatch semantic;
  Router Metadata v3 retains the Workspace-scoped read surface.
- Agent Router v1 is new.
- Platform Error v4, Invocation Event 0.3, and Result Stream Event v2 are
  required because new exact errors are embedded in those facts/frames.
- Northbound v3 unscoped Invocation/Trace reads migrate to Workspace-scoped v4
  detail/lineage paths; v4 does not return raw event arrays.
- Invocation Result v1, Control Plane Internal v2 exact Card resolution, Agent
  Card 0.2, A2A Profile Schema 0.2, and A2A protocol 0.3.0 are unchanged.
- Control Plane Internal v3 additively introduces deterministic installed-
  version resolution for nested calls; it is the active nested version-
  selection contract and is not a fallback for v2 exact Card resolution.

No deployed runtime consumer exists. First implementations consume targets
only. Historical artifacts remain byte-unchanged migration evidence and are
not served, decoded, retried, or used as fallback alternatives.

## Consequences

- Caller and lineage facts have exactly one trusted source.
- Phase 1 can route `none` Cards without inventing credential infrastructure.
- Operators can distinguish non-acceptance, committed terminal failure, and
  post-side-effect audit interruption.
- Runtime implementers must require explicit deployment limits and cannot add
  friendly defaults or cancellation/write retries.
- Supporting another Agent auth type requires a future Spec/ADR with secret
  binding ownership, lifecycle, rotation, and observability policy.

## Rejected Alternatives

- Reusing Router Internal service auth for Agents.
- Trusting SDK correlation/context or direct endpoint fields.
- Empty/default credentials or overloading an existing error code.
- Calling Agent before `created` commits.
- Returning success before terminal audit commit.
- Fabricating a Ledger failure event in a failed Ledger.
- Automatic retries, alternate stores, reconciliation, result replay, or
  historical dual-version runtime.

## Fallback Report

```text
Fallback delta: removed 1, retained 0, added 0, net -1
Added fallback evidence: none
```
