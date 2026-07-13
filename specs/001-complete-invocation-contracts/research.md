# Research: Complete Invocation Contracts

## Decision 1: Return Results on the Invocation Request

**Decision**: The `stream` request field selects one of two successful response
media types on the same versioned POST endpoint:

- `stream=false`: `200 application/json` with correlation identifiers, terminal
  succeeded status, and the complete Agent result.
- `stream=true`: `200 text/event-stream`; each `data` value is a transient,
  discriminated result-stream event.

An incompatible `Accept` header receives `406`. OpenAPI lists both response
media types and normative operation text plus contract tests enforce the
request-field/media-type relationship that OpenAPI cannot express directly.

**Rationale**: This is the user-selected Option A and closes the current gap in
which a caller receives only `202` identifiers. It avoids result persistence and
keeps the Frontend on the Gateway boundary.

**Alternatives considered**:

- `202` plus polling: rejected because it requires result storage and retention.
- `202` plus a separate stream endpoint: rejected because it recreates the
  acceptance/result gap and adds another lifecycle.
- Result data in Ledger events: rejected because result transport and auditable
  facts have different ownership and sensitivity.

## Decision 2: Separate Result Transport from Ledger Facts

**Decision**: Define `InvocationResult` and `InvocationResultStreamEvent` as
new transient contracts. Keep `InvocationEvent` content-free: stream Ledger
facts may contain chunk index and byte count, never input, result, or chunk data.

Streaming has these rules:

- Every event carries invocation, root task, and trace identifiers.
- Result chunks have monotonically increasing `chunkIndex`; each chunk is an
  ordered capability-specific JSON value and is not assumed concatenable.
- Exactly one terminal event ends a clean stream.
- Failure, timeout, and cancellation after HTTP commitment are terminal SSE
  events; HTTP remains `200` after commitment.
- EOF without a terminal event is interrupted delivery, never success.
- Chunks before a non-success terminal event are incomplete output.
- Success, timeout, and cancellation use first-terminal-wins semantics. Output
  arriving after another terminal outcome is discarded and cannot rewrite it.

**Rationale**: The user can consume output while operators retain a secret-safe,
append-only lifecycle history.

**Alternatives considered**:

- Reuse `RouterEventEnvelope`: rejected because it would turn Ledger events into
  a result store.
- Buffer the full stream at Gateway: rejected because it defeats streaming and
  adds memory pressure not required by the contract.

## Decision 3: Publish Directional Internal APIs

**Decision**:

- `control-plane-internal.v1.yaml` is served by the Control Plane and contains
  Router-to-Control-Plane exact Agent resolution.
- `router-internal.v2.yaml` is served by the Router and contains
  Control-Plane-to-Router dispatch/result transport plus Router-owned Ledger
  and trace reads.

Both documents identify caller, owner, and destination. The mixed
`router-internal.v1.yaml` remains historical and is not the active runtime
contract.

**Rationale**: One global Router server URL cannot correctly describe an
operation owned by the Control Plane. Splitting documents also prevents
generated clients from routing resolution back to the Router.

**Alternatives considered**:

- Operation-level server overrides in one document: technically possible but
  retains mixed ownership and makes client generation error-prone.
- Tags only: rejected because tags do not change destination.

## Decision 4: Version Breaking Contract Changes

**Decision**: Preserve historical artifacts and add active versions:

| Contract | Historical | Active target | Reason |
|---|---:|---:|---|
| Northbound API | `v1` | `v2` | Invocation changes from acceptance to direct result |
| Router Internal API | `v1` | `v2` | Dispatch changes from acceptance to direct result |
| Control Plane Internal API | none | `v1` | New directional owner contract |
| Agent Card Schema | `0.1` | `0.2` | Portable semantic rejection rules narrow accepted Cards |
| Invocation Event | `0.1` | `0.2` | Failed/error-code combinations become stricter |
| Platform Error | `v1` | `v2` | Result-channel context and not-acceptable semantics |
| Invocation Result | none | `v1` | New transient result contracts |
| A2A Profile Schema | `0.1` | `0.2` | Required operation/state semantics and fixtures |
| A2A Protocol | `0.3.0` | `0.3.0` | Wire protocol remains pinned |

The first backend runtime implements active targets only. No Registry, Router,
or Gateway runtime currently consumes the historical artifacts, so no
speculative dual-version branch is introduced. Historical files remain as
migration evidence.

**Rationale**: The compatibility policy classifies response and semantic
changes as breaking. Pre-runtime status removes the need for invented backward
compatibility while still preserving contract history.

**Alternatives considered**:

- Mutate existing files in place: rejected because published IDs and versioned
  filenames would lie about compatibility.
- Implement both versions: rejected because there are no runtime consumers and
  such a branch would be an unsupported fallback.

## Decision 5: Pair Agent Card Schema with Semantic Conformance

**Decision**: Agent Card `0.2` consists of:

1. JSON Schema for structural validation.
2. `contracts/agent-card/v0.2/semantic-rules.md` with RFC 2119 rules and stable
   IDs.
3. Raw JSON fixtures plus a manifest recording case ID, expected validity, and
   violated rule IDs.

Normative rules are:

- `AC-SEM-001`: `skills[*].id` values are unique within one Card.
- `AC-SEM-002`: `permissions[*].id` values are unique within one Card.
- `AC-SEM-003`: every required permission exactly and case-sensitively matches
  a permission declared in the same Card version.

Go keeps schema-then-semantic validation but maps failures to rule IDs and must
pass the same raw fixtures. Error prose and rule evaluation order are not
normative.

Agent Card `0.2` structurally forbids endpoint URI userinfo so credentials
cannot enter a Card through an otherwise valid HTTP(S) URI. The conformance
manifest uses presence-required fields, duplicate-member rejection, and
canonical corpus-confined relative paths to avoid language/parser and filesystem
differences.

**Rationale**: Draft 2020-12 `uniqueItems` compares complete values and cannot
portably enforce projected object-key uniqueness or an instance-data join.

**Alternatives considered**:

- Custom JSON Schema vocabulary: rejected because every validator would need a
  custom implementation.
- CEL, Rego, CUE, or JSON Logic: rejected as a new policy runtime for three
  stable rules.
- Go-only checks or AJV `$data`: rejected because they are not language-neutral.

## Decision 6: Make A2A Profile Conformance Executable

**Decision**: Keep `github.com/a2aproject/a2a-go v0.3.15` and A2A protocol
`0.3.0`. Publish Profile Schema `0.2`, operation/state rules, fixed JSON-RPC/SSE
fixtures, and Go tests against the real SDK transport and server handler.

Required operations map to the pinned SDK as follows:

| Wire method | Client method | Server method | Request |
|---|---|---|---|
| `message/send` | `SendMessage` | `OnSendMessage` | `*a2a.MessageSendParams` |
| `message/stream` | `SendStreamingMessage` | `OnSendMessageStream` | `*a2a.MessageSendParams` |
| `tasks/get` | `GetTask` | `OnGetTask` | `*a2a.TaskQueryParams` |
| `tasks/cancel` | `CancelTask` | `OnCancelTask` | `*a2a.TaskIDParams` |

Conformance covers both possible non-streaming results (`Message`, `Task`), all
four streaming event kinds (`Message`, `Task`, status update, artifact update),
JSON-RPC envelope integrity, SSE framing/content type, task/context identity,
artifact append/final-chunk ordering, task-not-found/not-cancelable errors, and
all five required NeKiro context headers.

Task state policy:

- Phase 1 transient: `submitted`, `working`.
- Phase 1 terminal: `completed`, `failed`, `canceled`, `rejected`.
- `rejected` maps to platform failed with `AGENT_EXECUTION_FAILED`.
- `auth-required`, `input-required`, `unknown`, and unspecified are recognized
  A2A values but unsupported by the Phase 1 invocation profile; encountering
  one terminates the platform invocation with `A2A_PROTOCOL_ERROR`.
- Timeout remains a platform Invocation outcome, not an A2A Task state.

Fixtures are hand-authored language-neutral wire values, not values marshaled
by Go and then treated as their own oracle.

The conformance manifest is executable metadata, not documentation. Its loader
rejects unknown or duplicate JSON members and validates canonical corpus-local
paths before filesystem access. Supported operation/fixture-kind/media-type
combinations are closed. Conditional fields such as `requestFile`,
`wireResultKind`, `goConcreteType`, and `protocolError` must be present or absent
as required by the case kind. Each `rules` entry names a known harness assertion
that must execute for that case; unsupported or unexecuted claims invalidate the
manifest rather than being ignored.

Successful `message/send` Message results require a non-empty Message ID, Agent
role, and at least one part. Task results continue through the task identity and
state validator. The four required operations execute through the public
`a2aclient.Client` methods and matching `a2asrv` handlers; raw JSON-RPC transport
tests remain useful only as additional wire-level evidence.

The A2A conformance manifest remains schema `0.1` because it has no accepted or
external consumer and this amendment completes the strict semantics intended by
the active, still-unreleased Profile `0.2`. No compatibility fallback or dual
decoder is introduced.

**Rationale**: SDK decode success alone is insufficient: ordinary unmarshaling
can accept zero-valued Tasks, semantically empty Messages, and arbitrary
TaskState strings. Descriptive manifest fields can also claim coverage that the
harness never performed.

**Alternatives considered**:

- Check only SDK/module versions: rejected because it proves no operation shape
  or lifecycle compatibility.
- Copy the complete upstream A2A schema: rejected because it duplicates the
  protocol instead of defining NeKiro's supported subset.
- Dispatch only by operation and fixture kind: rejected because unvalidated
  manifest type and rule metadata could falsely report conformance coverage.

## Decision 7: Failure Status Mapping

**Decision**:

- Before response commitment, policy/input failures use their existing 4xx
  meaning; incompatible media type uses `406`; protocol/Agent execution uses
  `502`; route, unavailable, or dependency failure uses `503`; timeout uses
  `504`; observable cancellation uses `409`.
- After SSE commitment, failed, timed-out, and canceled terminal events carry a
  fixed Platform Error. Agent result data and dependency details are forbidden.
- Invocation Event `0.2` permits `TIMEOUT` only with `timed_out` and `CANCELED`
  only with `canceled`; `failed` explicitly excludes both codes.

**Rationale**: HTTP status remains useful before commitment while the stream
requires in-band terminal semantics afterward. Distinct errors preserve audit
meaning.

**Alternatives considered**:

- Return every terminal outcome as normal success JSON: rejected because it
  collapses failure semantics.
- Use non-standard `499`: rejected from the public contract in favor of a
  portable standard status.
