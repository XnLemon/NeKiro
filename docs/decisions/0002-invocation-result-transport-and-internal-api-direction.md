# ADR 0002: Invocation Result Transport and Internal API Direction

- Status: Accepted
- Date: 2026-07-13
- Decision owners: Gateway, Invocation Dispatch, A2A Router, and Ledger
- Feature: Spec 001 Complete Invocation Contracts

## Context

Northbound API v1 and Router Internal API v1 acknowledge dispatch with `202`
but do not deliver Agent output through the trusted Gateway boundary. The mixed
Router v1 document also describes both Control Plane-owned resolution and
Router-owned dispatch under one Router destination. Those shapes cannot close
the Phase 1 `Invoke -> Record` path or generate clients with an unambiguous
service destination.

Invocation Results and Ledger facts have different security and retention
semantics. Result content is caller data transported on the live invocation
response. Ledger events are append-only operational facts used for audit and
diagnosis. Treating one as the other would persist Agent output and expose it
through metadata query interfaces.

## Decision

### Same-request result delivery

The platform adopts Option A on both Gateway and Router dispatch boundaries:

- `stream=false` with `Accept: application/json` or a compatible wildcard
  returns `200 application/json` and one Invocation Result v1.
- `stream=true` with `Accept: text/event-stream` returns `200` and ordered
  Invocation Result Stream Event v1 SSE data values.
- A request-mode and `Accept` mismatch returns `406` with Platform Error v2
  code `NOT_ACCEPTABLE`.
- Before response commitment, cancellation uses `409`, Agent or A2A failures
  use `502`, unavailable routes, Agents, or dependencies use `503`, and timeout
  uses `504`.
- After SSE commitment, failure, cancellation, and timeout are fixed,
  correlated terminal events on the `200` stream.

Every clean stream begins with `accepted`, preserves zero-based event and chunk
order, and ends with exactly one of `completed`, `failed`, `canceled`, or
`timed_out`. The first valid terminal event is immutable. Later events are
invalid and discarded. EOF before a terminal event means interrupted delivery;
preceding chunks are incomplete output, never a successful result.

Result and chunk values may be any JSON value allowed by the resolved Agent
Skill output schema. The result contracts do not assume strings or concatenate
chunks.

### No result persistence or replay

Phase 1 has no result table, result GET endpoint, replay token, reconnect
cursor, or polling lifecycle. A disconnected caller may inspect Ledger facts
for diagnosis but must create a new Invocation to receive output. Agent input,
result, and chunk content are forbidden from Invocation Event v0.2 and all
Ledger query responses.

### Directional internal APIs

Internal operations are split by service owner and destination:

| Contract | Served by | Called by | Operations |
| --- | --- | --- | --- |
| `control-plane-internal.v2.yaml` | Control Plane | A2A Router | Exact authorized Agent resolution with pre/post-correlation errors |
| `control-plane-internal.v3.yaml` | Control Plane | A2A Router | Deterministic enabled-Installation version resolution for nested calls |
| `router-internal.v4.yaml` | A2A Router | Control Plane | Active managed-auth dispatch and result delivery |
| `router-metadata.v3.yaml` | A2A Router | Control Plane | Active Workspace-scoped Invocation/Trace reads |

The Router resolves through the Control Plane contract and never reads Registry
or Workspace storage directly. The Control Plane dispatches only through the
Router contract. Production destinations are configured explicitly; neither
contract defines a localhost fallback.

### Version and failure semantics

Northbound v3/v4, Router Internal Dispatch v4, Router Metadata v3, Control Plane Internal v2/v3, Invocation Event v0.3, Platform Error v2/v3,
and Invocation Result v1 are active contracts. Historical Northbound v1/v2,
Router Internal v1/v2/v3, and Invocation Event v0.1 files remain unchanged as
migration evidence.

Platform Error v2 fixes one public message per code, requires trace
correlation, and permits Invocation and root Task correlation only as a pair.
It has no detail, endpoint, credential, payload, Agent output, dependency error,
or stack fields. Invocation Event v0.2 makes terminal type, status, and error
classification coherent: `TIMEOUT` belongs only to `timed_out`, `CANCELED`
belongs only to `canceled`, and `failed` excludes both.

### Amendment: Spec 019 nested invocation

Spec 019 adds Control Plane Internal v3 as an additive, active contract for
deterministic installed-version selection. It does not change v2 exact Card
resolution and does not introduce a runtime fallback between the versions.
Agent Router credentials are bound to one `(Workspace, Agent)` pair so a parent
from another Workspace cannot be borrowed with the same Agent ID.

## Consequences

- Authorized callers receive Agent output without direct Router or Agent
  access.
- Gateway and Control Plane can forward streams without buffering a complete
  result.
- Result delivery interruption is visible and cannot be mistaken for success.
- Ledger remains append-only, metadata-only, and suitable for audit.
- Generated internal clients have one owner and one destination per document.
- API v1 consumers must migrate before the first backend runtime; the runtime
  does not implement speculative dual-version reads or dispatch behavior.
- A caller that loses its result connection cannot recover output from the
  platform and must invoke again.

## Rejected Alternatives

- `202` plus polling or a separate result endpoint: requires result persistence
  and introduces a second lifecycle.
- Result content in Ledger events: violates data ownership, retention, and
  secret-safety boundaries.
- Full-stream buffering at the Gateway: defeats streaming and adds unrequired
  memory pressure.
- One mixed internal OpenAPI document: leaves operation ownership and generated
  client destinations ambiguous.
- Runtime support for both historical and active versions: no runtime consumer
  exists to justify the compatibility branch.
