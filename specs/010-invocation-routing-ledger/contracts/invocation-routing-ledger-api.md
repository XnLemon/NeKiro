# Contract Plan: Invocation Routing and Ledger

## Active Starting Surface

| Boundary | Active source | Direction |
| --- | --- | --- |
| Public invoke and metadata reads | `contracts/openapi/control-plane.v3.yaml` | Caller -> Gateway |
| Exact authorized resolution | `contracts/openapi/control-plane-internal.v2.yaml` | Router -> Control Plane |
| Dispatch/result and Ledger reads | `contracts/openapi/router-internal.v2.yaml` | Control Plane -> Router |
| Result | `contracts/schemas/invocation-result.v1.schema.json` | Router -> Gateway -> caller |
| Result stream | `contracts/schemas/invocation-result-stream-event.v1.schema.json` | Router -> Gateway -> caller |
| Ledger fact | `contracts/schemas/invocation-event.v0.2.schema.json` | Router -> Ledger/read clients |
| Errors | Platform Error v2/v3 | Owning boundary -> caller |
| A2A subset | A2A Profile Schema 0.2 / protocol 0.3.0 | Router <-> Agent |

Historical v1/v2 artifacts are migration evidence only. No runtime dual read or
route is planned.

## Existing Semantics to Preserve

- `stream=false` plus JSON-compatible `Accept` returns one result on the same
  POST; `stream=true` plus SSE `Accept` returns one ordered result stream.
- Mode mismatch is `NOT_ACCEPTABLE` without dispatch.
- Results/chunks are transient and absent from Ledger/read responses.
- Every correlated result, event, and post-context error preserves Invocation,
  root Task, and Trace values exactly.
- Router resolution reaches only the Control Plane internal destination.
- Control Plane dispatch and metadata reads reach only the Router destination.
- Timeout, cancel, protocol, Agent, route, authorization, and dependency errors
  remain distinct.

## Blocking Contract Gaps for T001

### 1. Agent SDK Invocation Direction

The current Router Internal v2 description authorizes Control Plane callers.
It does not define how a running Agent/SDK authenticates to the Router or how
the Router derives trusted `caller.type=agent`, `caller.id`, Workspace, parent,
root Task, and Trace context. T001 must either define a separate versioned
Agent-facing API or explicitly version the current API for both caller classes.

Required decision evidence:

- authentication and credential source;
- trusted versus request-supplied fields;
- parent Invocation existence/Workspace checks;
- request/result modes supported by the first Go SDK;
- exact pre/post-correlation errors;
- protection against direct target endpoint use.

### 2. Agent Credential Binding

Agent Card declares `none`, `api_key`, `http_bearer`,
`oauth2_client_credentials`, or `mutual_tls`, but intentionally contains no
secret or secret locator. T001 must freeze which types Phase 1 runtime supports
and how Router obtains a version/Workspace-scoped binding without storing a
credential in Card/Ledger/logs.

No missing binding may become empty auth, anonymous access, a default token,
direct URL use, or successful degraded invocation.

### 3. Ledger Append Failure

Contracts distinguish dependency failure but do not state the public/metadata
representation when Ledger storage fails after an Agent side effect. T001 must
define:

- the point before which no Agent call is allowed;
- when result/terminal response may be emitted;
- post-commit SSE dependency failure behavior;
- how a durable non-terminal history is exposed;
- whether any recovery mechanism is approved. Default is none.

The successful Router `created` commit defines the accepted-Invocation boundary.
Failures before it create no Ledger fact; this absence is the required evidence,
not a synthetic terminal event.

The contract gate MUST NOT require a fabricated terminal Ledger event when the
Ledger is the failed dependency. It must distinguish normal failures whose
terminal append commits from post-side-effect persistence failure, which is
non-success with an observable non-terminal audit history.

### 4. Deadline and Cancellation

Northbound request has no timeout field and no cancellation endpoint. T001 must
freeze required deployment deadline configuration, HTTP disconnect behavior,
A2A `tasks/cancel` usage, task-not-cancelable mapping, and first-terminal-wins
semantics. No arbitrary duration or retry may be added as an implementation
default.

### 5. Size and Framing Limits

T001 must verify or add explicit contract/profile limits for public and
internal request size, A2A response/event size, SSE line/event framing, and
safe error handling before large input or output reaches the Router/Gateway.

## Compatibility Rule

T001 performs semantic compatibility analysis. If one of the decisions changes
an accepted request, response, auth requirement, or failure meaning, it must
create a new contract version and migration note. It must not mutate a
published version in place or implement both versions as a fallback without an
actual consumer policy.

## Contract Test Matrix

| Area | Required evidence |
| --- | --- |
| Direction/auth | Wrong service/Agent credential rejected before owned behavior |
| Correlation | Exact values across root, child, results, errors, events, and reads |
| Mode/media | All JSON/SSE compatible and mismatched combinations |
| A2A | All active operations, result/event kinds, task states, JSON-RPC and SSE invalid cases |
| Lifecycle | Created/routing/started/stream/terminal ordering and first terminal |
| Content exclusion | Input/result/chunk/credential/dependency detail absent from all metadata contracts |
| Compatibility | Historical artifacts unchanged; active version decision documented |

## Fallback Report

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
