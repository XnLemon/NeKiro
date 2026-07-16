# Feature Specification: Deterministic Direct A2A Sample

**Feature Branch**: `codex/015-runtime-b-agent`

**Created**: 2026-07-16

**Status**: Implemented, locally verified, independently reviewed, and converged in WIP branch

**Input**: GitHub Issue #24 / Spec 010 T005: build an independent deterministic
callee Agent that supplies active-profile conformance evidence and stable
fixtures for later invocation acceptance.

## Context

The platform contracts now define the A2A protocol boundary, but the repository
does not contain a runnable Agent that later Router work can invoke. This
feature supplies the direct callee half of the cross-Runtime proof. It is a
sample Agent, not a Control Plane or Router component, and it owns no platform
data.

## Clarifications

### Session 2026-07-16

- The sample uses the A2A Profile-pinned Go protocol library directly, without
  a full Agent Runtime framework. This keeps Runtime B independently
  implemented from the later SDK-backed caller Agent.
- The four active operations are `message/send`, `message/stream`, `tasks/get`,
  and `tasks/cancel`; operations outside that set are explicitly unsupported.
- Fixture selection is explicit in a structured request part. Missing, empty,
  or unknown fixture instructions fail validation and never select a default.
- Stable fixture behavior includes one JSON success, one ordered successful
  stream, one explicit Agent failure, and one cancelable stream task.
- Task state is process-local sample Runtime state. The Agent does not access
  Catalog, Workspace, Router, Ledger, or any platform database.

## User Scenarios & Testing

### User Story 1 - Invoke the Direct Callee (Priority: P1)

As a platform developer, I need a deterministic direct A2A callee so Router
transport can be integrated against stable success and failure behavior.

**Why this priority**: T006 cannot prove exact A2A result delivery without a
real conforming target Agent.

**Independent Test**: Start the sample in-process, call `message/send` through
the pinned A2A client with explicit success and failure fixtures, and verify the
exact result and protocol error class.

**Acceptance Scenarios**:

1. **Given** a valid success fixture, **when** `message/send` is called, **then**
   one Agent-role structured Message with deterministic identity and data is
   returned.
2. **Given** a valid failure fixture, **when** `message/send` is called, **then**
   the call fails explicitly and no success Message or Task is returned.
3. **Given** a missing, empty, malformed, or unknown fixture instruction,
   **when** either message operation is called, **then** invalid parameters are
   reported without choosing a fixture implicitly.

---

### User Story 2 - Consume an Ordered Stream (Priority: P1)

As a platform developer, I need stable streaming and cancellation fixtures so
later Router work can verify ordering, terminal behavior, and cancellation.

**Why this priority**: Streaming and cancellation are mandatory Phase 1 failure
and result semantics.

**Independent Test**: Call `message/stream` with explicit success and hold
fixtures, inspect ordered events, cancel the held task once, and verify the
terminal state.

**Acceptance Scenarios**:

1. **Given** a stream-success fixture, **when** `message/stream` is consumed,
   **then** the Agent emits one working Task, one Agent Message, ordered base and
   final artifact chunks, and exactly one final completed status.
2. **Given** a hold fixture, **when** the stream exposes its working Task and
   `tasks/cancel` is called, **then** the same Task becomes canceled and the
   stream ends with exactly one final canceled status.
3. **Given** a completed, failed, or canceled Task, **when** cancellation is
   requested, **then** the operation returns task-not-cancelable rather than
   reporting success.

---

### User Story 3 - Inspect Active A2A Tasks (Priority: P1)

As a protocol integrator, I need `tasks/get` and `tasks/cancel` to obey the same
task identity and state rules as the message operations.

**Why this priority**: All four active A2A operations must conform before the
sample can gate Router integration.

**Independent Test**: Create deterministic stream tasks, query them with an
explicit history length, cancel a working task, and exercise missing and
terminal task errors through the official A2A client.

**Acceptance Scenarios**:

1. **Given** an existing Task and explicit history length, **when** `tasks/get`
   is called, **then** the same task/context identity, current supported state,
   and exactly the requested most-recent history are returned.
2. **Given** an unknown Task ID, **when** get or cancel is called, **then**
   task-not-found is returned.
3. **Given** every active operation, **when** conformance tests execute through
   the official JSON-RPC/SSE server and client, **then** responses satisfy the
   active A2A Profile and preserve required platform context headers.

### Edge Cases

- A message has no parts, multiple parts, a non-data part, or a structured part
  with fields of the wrong type.
- Two requests carry different message IDs but the same fixture value; their
  protocol identities must remain distinct while their result data stays
  stable.
- A client stops consuming a held stream before cancellation; request context
  termination releases the stream without inventing a completed result.
- Get requests ask for a negative or greater-than-history history length.
- Get races with cancellation; each returned snapshot is internally
  consistent, and cancellation can commit only from a working state.

## Requirements

### Functional Requirements

- **FR-001**: The sample MUST run as an independent Agent process and MUST NOT
  import platform service internals or access a platform database.
- **FR-002**: The sample MUST expose the active Profile operations through the
  A2A Profile-pinned protocol library and JSON-RPC/SSE transport.
- **FR-003**: The sample MUST require an explicit structured fixture instruction
  for message operations and MUST distinguish invalid parameters, deterministic
  Agent failure, missing Task, and non-cancelable Task.
- **FR-004**: Success output MUST be deterministic for a given request and MUST
  use request-derived, collision-resistant message, Task, context, and artifact
  identities rather than random identifiers.
- **FR-005**: A successful stream MUST emit gap-free events in the exact order
  Task working, Agent Message, base artifact chunk, final artifact chunk, and
  final completed status.
- **FR-006**: A cancelable stream MUST publish a working Task, wait for explicit
  cancellation or request-context termination, and emit no completed terminal
  outcome after cancellation.
- **FR-007**: `tasks/get` MUST return only an existing task, preserve supported
  state, and apply an explicitly supplied non-negative history length.
- **FR-008**: `tasks/cancel` MUST change only an existing working Task to
  canceled; terminal Tasks MUST return task-not-cancelable.
- **FR-009**: The sample MUST preserve all five active platform context headers
  at its HTTP boundary so conformance evidence can prove Router propagation.
- **FR-010**: Runtime state and helpers MUST remain inside `agents/runtime-b/`;
  no Runtime-internal type may be added to shared contracts or platform core.
- **FR-011**: Post-implementation tests MUST exercise all four operations,
  exact successful JSON, ordered successful streaming, failure, cancellation,
  task errors, context headers, and concurrent identity isolation.
- **FR-012**: The implementation MUST add no retry, cache, alternate source,
  inferred configuration, compatibility branch, degraded success, or other
  undocumented fallback.

### Key Entities

- **Fixture Request**: Explicit structured input selecting `success`,
  `stream-success`, `failure`, or `hold`, plus a JSON-compatible value.
- **Runtime Task**: Process-local task snapshot containing deterministic Task
  and context IDs, supported state, bounded history, and cancellation signal.
- **Fixture Result**: Stable Agent-role structured Message or stream artifact
  content derived from the request.

### Runtime/Platform Boundary

- **Platform-owned behavior**: Contracts, Router authorization/routing,
  platform context generation, transient result forwarding, and Ledger facts.
- **Runtime-owned behavior**: Fixture parsing, deterministic sample result
  generation, process-local task state, and A2A server handling.
- **Cross-runtime proof**: This direct-library callee is Runtime B. T009/T010
  will supply a separately implemented SDK-backed Runtime A caller; only A2A
  and platform contracts cross between them.

## Success Criteria

### Measurable Outcomes

- **SC-001**: One clean test run invokes all four active operations through the
  official client/server path with 100% passing conformance assertions.
- **SC-002**: Every successful stream contains exactly five ordered events and
  exactly one final terminal status.
- **SC-003**: A cancellation run returns the same Task identity from stream,
  get, and cancel and produces exactly one canceled terminal state.
- **SC-004**: A 100-request concurrent run produces 100 distinct deterministic
  protocol identities with no task or result crossover.
- **SC-005**: Inspection of imports and execution evidence finds zero platform
  database access, zero platform-core dependency, and zero added fallback.

## Assumptions

- Agent authentication type `none` is the only Phase 1 transport mode that the
  Router may invoke, per Spec 011 and ADR 0006.
- The repository's active A2A Profile Schema 0.2 / protocol 0.3.0 and pinned
  `github.com/a2aproject/a2a-go v0.3.15` are frozen by T001.
- Process-local sample task state is sufficient because durable invocation
  audit belongs to Router Ledger work, not an Agent Runtime.

## Non-Goals

- No Router, Dispatch, Ledger, Workspace, Catalog, SDK, nested call, Console,
  deployment orchestration, or platform persistence implementation.
- No model, prompt, tool, planner, workflow, memory, RAG, session, or general
  Agent Runtime framework.
- No result replay, task resubscription, push notifications, task listing,
  retry, cache, alternate route, or compatibility fallback.
