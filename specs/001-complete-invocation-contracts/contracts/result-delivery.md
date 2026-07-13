# Contract Design: Invocation Result Delivery

## Northbound Operation

`POST /v2/workspaces/{workspaceId}/invocations`

The JSON request retains `agentId`, `capability`, `input`, and `stream`.

| Request mode | Required `Accept` | Success response |
|---|---|---|
| `stream=false` | `application/json` or compatible wildcard | `200 application/json` with Invocation Result v1 |
| `stream=true` | `text/event-stream` | `200 text/event-stream` with Invocation Result Stream Event v1 values |

A mode/header mismatch returns `406` with Platform Error v2 code
`NOT_ACCEPTABLE`. The operation description is normative because OpenAPI 3.1
cannot condition a response media type on a request-body boolean.

## Router Dispatch Operation

`POST /internal/v2/invocations` uses the same request mode and response model.
The Control Plane relays the Router result through the Gateway while preserving
correlation IDs and fixed errors. Neither process creates a result store.
Non-streaming delivery compares the returned `invocationId`, `rootTaskId`, and
`traceId` with this dispatch context before the result is accepted.

## Response Schemas

- `contracts/schemas/invocation-result.v1.schema.json`
- `contracts/schemas/invocation-result-stream-event.v1.schema.json`
- `contracts/schemas/platform-error.v2.schema.json`

The result schema permits any JSON value in `result`/`chunk`; the resolved Agent
Skill output schema supplies capability-specific validation.

### JSON Number Preservation

Strict public DTO decoding checks JSON syntax and duplicate object members
before typed decoding. That pre-scan MUST preserve number tokens without
coercing them into a bounded implementation numeric type. It therefore accepts
legal unconstrained result or chunk values such as `1e400`, including nested
values, and leaves capability-specific numeric limits to the resolved output
schema.

Typed envelope fields retain their declared constraints. Number preservation
does not make an out-of-range `sequence`, `chunkIndex`, or other constrained
field valid. It prevents the duplicate-member scanner from adding a second,
implementation-specific range to arbitrary Agent output.

## Correlation Semantics

`contracts/invocation/v1/semantic-rules.md` defines `INV-CORR-001`: a Platform
Error v2 nested in Invocation Event `0.2` or Invocation Result Stream Event `1`
MUST repeat the enclosing `invocationId`, `rootTaskId`, and `traceId` exactly.
Standard JSON Schema validates shape and presence but cannot compare instance
locations, so raw JSON cases and their manifest live under
`contracts/invocation/v1/conformance/`. All language implementations MUST make
the same decision for that corpus.

All public JSON DTO decoders covered by this contract reject duplicate member
names before typed decoding, including nested objects. Duplicate-member payloads
do not receive first-member-wins or last-member-wins interpretation.

## Failure Matrix

Before an SSE response is committed:

| Outcome | HTTP status | Public error |
|---|---:|---|
| Invalid input or policy request | existing 4xx | existing fixed code |
| Result mode not acceptable | `406` | `NOT_ACCEPTABLE` |
| Conflict or observable cancellation | `409` | `CONFLICT` or `CANCELED` |
| Agent or A2A protocol failure | `502` | `AGENT_EXECUTION_FAILED` or `A2A_PROTOCOL_ERROR` |
| Route, Agent, or dependency unavailable | `503` | matching fixed unavailable/dependency code |
| Deadline exceeded | `504` | `TIMEOUT` |

The `502`, `503`, and `504` rows occur after Invocation creation. Their active
Northbound and Router error responses require `traceId`, `invocationId`, and
`rootTaskId`; each value is the existing request context. The reusable base
Platform Error v2 shape remains suitable for pre-creation failures where
invocation and root-task identifiers do not yet exist.

After SSE commitment, failed, canceled, and timed-out outcomes are terminal
stream events. HTTP remains `200`; clients MUST inspect the terminal event.

## Non-Persistence

- Result and chunk data are forbidden in Invocation Event schemas.
- No replay token, result GET endpoint, polling endpoint, or reconnect cursor is
  defined.
- EOF without a terminal event is interrupted delivery. Ledger facts can be
  queried, but obtaining output requires a new Invocation.

## Migration

Northbound v1 and Router Internal v1 remain historical. Their `202` acceptance
responses do not satisfy this feature and are not implemented by the initial
backend runtime. API v2 is a deliberate breaking version.
