# Feature Specification: Non-Streaming A2A Dispatch

**Feature Branch**: `codex/016-nonstream-a2a-dispatch`

**Created**: 2026-07-16

**Status**: Ready for implementation

**Input**: Spec 010 T006: implement non-streaming exact A2A dispatch and
transient result delivery with mapped Router unit, HTTP, PostgreSQL, and A2A
tests.

## Context

The local Spec 016 baseline combines the closed Invocation Runtime contract
gate, the A2A Router Foundation, the Router-owned Ledger checkpoint, and the
Runtime B direct A2A sample. The Router currently validates an internal
dispatch request, authenticates Control Plane service calls, performs exact
Control Plane resolution, and returns a correlated `ROUTE_NOT_FOUND`
placeholder. It does not yet call an Agent.

This feature replaces that placeholder for `stream=false` requests with an
exact A2A `message/send` call to the resolved Agent endpoint and returns the
live result in the same request. It remains Router-owned Data Plane work: no
Control Plane internals, no SDK, no streaming, no cancellation semantics, and
no result persistence are introduced here.

## User Scenarios & Testing

### User Story 1 - Dispatch a Non-Streaming Invocation (Priority: P1)

As the Control Plane Invocation Dispatch service, I need the Router to resolve
an exact installed Agent version and call that Agent through A2A `message/send`
so the caller receives one correlated JSON result without direct Agent access.

**Independent Test**: Start a Runtime B `httptest` A2A endpoint, configure a
Router dispatch handler with a resolver that returns that endpoint, submit one
valid `stream=false` dispatch request, and verify the response status, trace
headers, exact JSON result, and observed platform context headers.

**Acceptance Scenarios**:

1. **Given** a valid non-streaming Router dispatch request and an exact
   resolved Agent endpoint, **when** the Agent returns an A2A message result,
   **then** the Router returns one JSON invocation result correlated with the
   request identifiers.
2. **Given** the resolved Agent endpoint is unreachable, times out, or returns
   invalid A2A protocol data, **when** dispatch runs, **then** the Router
   returns a correlated non-success platform error and does not fabricate a
   normal result.
3. **Given** a request with `stream=true`, **when** this feature is active,
   **then** the existing stream-mode validation/placeholder behavior remains
   owned by later Spec 017 and is not implemented here.

---

### User Story 2 - Record Metadata-Only Non-Streaming Lifecycle Facts (Priority: P1)

As a platform operator, I need accepted non-streaming invocations to produce
metadata-only Ledger lifecycle facts so successful and failed Agent calls are
auditable without storing Agent input or result content.

**Independent Test**: Use the Router Ledger store or a strict test recorder to
verify accepted non-streaming dispatch emits ordered metadata-only lifecycle
events for routing, Agent attempt, and terminal success/failure, while the
returned result body is not stored.

**Acceptance Scenarios**:

1. **Given** a resolved Agent call succeeds, **when** the Router returns the
   result, **then** the terminal success fact is committed before the caller
   receives success.
2. **Given** the Agent call fails after the accepted invocation boundary,
   **when** the Router returns an error, **then** a terminal failure fact is
   committed with a safe classification and without dependency detail text.
3. **Given** any Ledger append fails before a successful Agent result can be
   exposed, **when** dispatch handles the failure, **then** the caller receives
   explicit non-success and no successful result is emitted.

---

### User Story 3 - Preserve Boundaries and Failure Semantics (Priority: P1)

As a platform maintainer, I need non-streaming dispatch to preserve existing
Router validation, resolution, auth, trace, and fallback policies while adding
only the Agent transport behavior this feature owns.

**Independent Test**: Re-run existing Router dispatch validation/resolution
tests, add failure matrix cases for Agent protocol/dependency errors, and scan
the diff for forbidden Control Plane imports, result persistence, retries,
fallback endpoints, compatibility branches, caches, or direct database access
outside Ledger ownership.

**Acceptance Scenarios**:

1. **Given** invalid media, invalid body, auth failure, resolution failure, or
   dependency failure before Agent transport, **when** dispatch is invoked,
   **then** the existing explicit error semantics remain unchanged.
2. **Given** a resolved Agent Card has an unsupported profile, auth mode,
   endpoint, or capability mapping, **when** dispatch evaluates it, **then**
   the Router fails closed with a correlated platform error.
3. **Given** result content, input content, credentials, or raw dependency
   messages, **when** Ledger rows and logs are inspected, **then** none are
   stored as invocation facts.

## Functional Requirements

- **FR-001**: The Router MUST implement non-streaming `stream=false` dispatch
  by calling the resolved Agent through A2A `message/send`.
- **FR-002**: The Router MUST reuse the existing internal dispatch request
  validation, service authentication, exact Control Plane resolution, and
  correlation identifiers.
- **FR-003**: The Router MUST propagate platform context to the Agent using the
  active contract headers only; it MUST NOT trust Agent-supplied Workspace,
  permission, or Ledger facts.
- **FR-004**: The Router MUST return exactly one live JSON result for a
  successful non-streaming Agent call and MUST NOT persist, replay, or poll
  the result.
- **FR-005**: The Router MUST distinguish resolution failures, unsupported
  profile/endpoint/auth, Agent endpoint dependency failure, A2A protocol
  failure, Agent business failure, and Ledger persistence failure.
- **FR-006**: The Router MUST record only metadata-only invocation lifecycle
  facts required by the active Invocation Event contract for accepted
  non-streaming dispatch.
- **FR-007**: The Router MUST commit the terminal success Ledger fact before
  returning a successful result to the caller.
- **FR-008**: The Router MUST return explicit non-success when required Ledger
  persistence fails and MUST NOT convert that state into a successful result.
- **FR-009**: The Router MUST not add retries, caches, fallback endpoints,
  default credentials, default Agent URLs, compatibility branches, or degraded
  success behavior.
- **FR-010**: The implementation MUST remain inside Router-owned packages and
  shared contracts; it MUST NOT import Control Plane internals or expand the
  Agent SDK or streaming/cancellation scope.

## Edge Cases

- Runtime B returns a valid A2A message with nested JSON data.
- Agent endpoint returns non-JSON, malformed JSON-RPC, unsupported result type,
  HTTP failure, timeout, or connection refusal.
- Resolved endpoint uses an unsupported URL scheme or empty endpoint.
- Resolved Agent Card declares an unsupported auth mode for this feature.
- Ledger append fails before or after Agent transport has started.
- `stream=true` requests continue to be rejected or deferred to Spec 017.

## Success Criteria

- **SC-001**: Focused Router non-streaming tests prove one valid Runtime B call
  returns the exact deterministic result and context propagation.
- **SC-002**: Failure tests cover resolution, endpoint dependency, protocol,
  Agent business, and Ledger persistence errors without fallback success.
- **SC-003**: `go test ./apps/a2a-router/... ./agents/runtime-b/...`,
  `go test ./...`, `go vet ./...`, and `git diff --check` pass.
- **SC-004**: Fallback delta reports added `0`; no result content is stored in
  Ledger rows or documented storage structures.
