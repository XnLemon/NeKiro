# Feature Specification: Complete Invocation Contracts

**Feature Branch**: `main`

**Created**: 2026-07-13

**Status**: Ready for Implementation

**Input**: Complete the Phase 1 contract foundation so platform users receive
Agent results through the Gateway, platform modules communicate through
unambiguous directional contracts, and all language implementations agree on
Agent Card, invocation lifecycle, and supported A2A behavior.

## Clarifications

### Session 2026-07-13

- Q: How does the Gateway return non-streaming and streaming Agent results? → A: The invocation request itself returns the result; non-streaming returns one complete result and streaming emits ordered result chunks on the same response. Results are not persisted or replayed.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Receive the Agent Result (Priority: P1)

A Workspace user invokes an installed Agent through the platform and receives
the Agent's actual result through the same trusted northbound platform boundary.
The user does not need direct access to the Router or Agent endpoint.

**Why this priority**: An invocation that only returns identifiers and byte
counts does not complete the Phase 1 `Invoke` step.

**Independent Test**: Invoke a successful installed Agent in non-streaming and
streaming modes and verify that the caller receives the exact result while the
Ledger remains queryable without storing the result content.

**Acceptance Scenarios**:

1. **Given** an enabled installation and an allowed capability, **When** the
   user invokes it without streaming, **Then** the user receives the completed
   Agent result correlated to the invocation and trace.
2. **Given** an enabled installation whose capability supports streaming,
   **When** the user requests streaming, **Then** ordered result chunks and a
   terminal outcome are delivered through the Gateway and carry correlation
   identifiers.
3. **Given** any successful invocation, **When** an operator queries its Ledger
   history, **Then** the lifecycle is complete but Agent input and output
   content are absent from Ledger facts.

---

### User Story 2 - Diagnose an Unambiguous Terminal Outcome (Priority: P2)

An operator inspecting an invocation can distinguish failure, timeout, and
cancellation without interpreting contradictory event fields.

**Why this priority**: Audit, troubleshooting, and later billing depend on a
single reliable meaning for each terminal fact.

**Independent Test**: Exercise each terminal outcome and verify that its event
type, status, and error classification agree and that invalid combinations are
rejected before they become Ledger facts.

**Acceptance Scenarios**:

1. **Given** an Agent business or protocol failure, **When** its terminal fact
   is recorded, **Then** it is classified as failed and does not use timeout or
   cancellation error classifications.
2. **Given** an invocation deadline expires, **When** its terminal fact is
   recorded, **Then** it is classified only as timed out.
3. **Given** an invocation is canceled, **When** its terminal fact is recorded,
   **Then** it is classified only as canceled.

---

### User Story 3 - Integrate Against One Portable Contract (Priority: P3)

An Agent or platform integrator can validate a Card and the supported A2A
interaction profile without depending on Go-specific validation behavior.

**Why this priority**: Agent Card and A2A Profile are ecosystem contracts and
must produce the same acceptance decision across languages.

**Independent Test**: Run equivalent valid and invalid Card examples through
independent conforming validators, then verify every declared A2A interaction
against the pinned profile.

**Acceptance Scenarios**:

1. **Given** a Card with duplicate skill or permission identifiers, **When** it
   is validated by any conforming implementation, **Then** it is rejected.
2. **Given** a skill references an undeclared permission, **When** the Card is
   validated by any conforming implementation, **Then** it is rejected.
3. **Given** an implementation claims conformance to the platform A2A Profile,
   **When** each declared interaction and lifecycle state is checked, **Then**
   unsupported or structurally incompatible behavior is identified.

---

### User Story 4 - Resolve an Agent Across the Correct Boundary (Priority: P3)

The Router resolves an authorized exact Agent version from the Control Plane
through a contract whose service owner and destination are unambiguous.

**Why this priority**: A contract that routes a resolution request back to the
Router cannot support the platform's Control Plane/Data Plane boundary.

**Independent Test**: Configure the Control Plane and Router as separate
processes and verify that Agent resolution targets the Control Plane while
dispatch and invocation queries target the Router.

**Acceptance Scenarios**:

1. **Given** separate Control Plane and Router addresses, **When** the Router
   resolves an invocation target, **Then** the resolution request reaches the
   Control Plane only.
2. **Given** the Control Plane dispatches an authorized invocation, **When** it
   sends the request, **Then** the request reaches the Router only.

### Edge Cases

- A stream ends after one or more result chunks but before a valid terminal
  outcome.
- A timeout or cancellation races with an Agent result.
- The Agent returns an A2A response that cannot be interpreted by the pinned
  profile.
- A result chunk exceeds declared Agent output limits.
- An Agent Card contains distinct objects that reuse the same identifier.
- A Card references a permission that exists only in a different Agent version.
- An Agent Card endpoint embeds URI userinfo such as a username or password.
- A conformance manifest repeats a JSON member name or references an absolute,
  parent-traversing, or platform-specific fixture path.
- An A2A conformance manifest declares metadata that is inconsistent with the
  referenced fixture, operation, media type, result type, or asserted rules.
- `message/send` returns a syntactically decodable but semantically empty Agent
  Message.
- Agent resolution is unavailable after an invocation has been accepted.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The platform MUST deliver the actual Agent result to an authorized
  northbound caller; invocation identifiers and Ledger metadata alone are not a
  successful result.
- **FR-002**: Streaming result chunks MUST preserve order and correlate to one
  invocation and trace through the Gateway.
- **FR-003**: The invocation request itself MUST return the Agent result.
  Non-streaming requests return one complete result, while streaming requests
  emit ordered result chunks and a terminal outcome on the same response.
- **FR-004**: Result content MUST remain separate from append-only Ledger facts
  and MUST NOT be exposed through Router-only interfaces to the Frontend.
- **FR-005**: The platform MUST define the observable outcome when result
  delivery is interrupted, timed out, canceled, or fails after partial output.
- **FR-006**: Invocation results MUST NOT be persisted or replayed in Phase 1.
  After a result connection is lost, the caller can inspect Ledger facts for
  the original invocation but MUST start a new invocation to receive a result.
- **FR-007**: Control Plane-owned resolution and Router-owned dispatch/query
  operations MUST identify different service ownership and destinations.
- **FR-008**: Every terminal invocation fact MUST have a coherent event type,
  status, and error classification; contradictory combinations MUST be invalid.
- **FR-009**: The language-neutral Agent Card contract MUST normatively define
  identifier uniqueness and declared-permission reference rules.
- **FR-010**: All conforming Agent Card validators MUST make the same decision
  for the published valid and invalid examples in this feature.
- **FR-011**: The A2A Profile MUST define verifiable compatibility expectations
  for message sending, message streaming, task lookup, task cancellation, and
  the task lifecycle states used by Phase 1.
- **FR-012**: Contract changes MUST state version and compatibility impact and
  MUST preserve fixed public error messages without exposing Agent output,
  secrets, or dependency details.
- **FR-013**: Every requirement in this feature MUST map to a later contract,
  implementation, and post-implementation test task before the feature can be
  marked complete.
- **FR-014**: An Agent Card protocol endpoint MUST be an absolute HTTP(S) URI
  without URI userinfo; usernames, passwords, tokens, and equivalent credential
  material are invalid even when the URI is otherwise well formed.
- **FR-015**: Agent Card conformance manifests MUST preserve required-field
  presence, reject duplicate JSON member names, and use canonical portable
  relative fixture paths confined to the conformance corpus.
- **FR-016**: A2A conformance manifests MUST reject unknown or duplicate JSON
  members and unsafe fixture paths. Their operation, fixture kind, media type,
  request reference, expected result type, protocol error, and rule list MUST
  be authoritative claims that are validated before and during fixture
  execution rather than descriptive labels.
- **FR-017**: A successful `message/send` Message result MUST identify a
  concrete Agent-authored Message with at least one part. Each required A2A
  operation MUST be exercised through its pinned SDK client method and server
  handler path; raw transport checks MAY only supplement those paths.

### Key Entities

- **Invocation Result**: Agent-produced output delivered to the authorized
  caller and correlated to an Invocation without becoming a Ledger fact.
- **Result Chunk**: An ordered portion of a streaming Invocation Result.
- **Invocation Terminal Fact**: The final append-only lifecycle fact describing
  success, failure, timeout, or cancellation.
- **Directional Internal Contract**: A cross-plane operation grouped by its
  owning service and destination.
- **Agent Card Semantic Rule**: A language-neutral invariant that cannot be
  fully expressed by structural field typing alone.
- **A2A Conformance Case**: A portable example whose manifest metadata is an
  executable assertion of one declared profile interaction or lifecycle
  expectation.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of successful acceptance invocations deliver Agent-produced
  output to the authorized caller in the requested result mode.
- **SC-002**: 100% of result chunks in conformance scenarios are ordered and
  correlated to exactly one invocation and trace.
- **SC-003**: All invalid terminal type/status/error combinations defined by the
  feature are rejected before Ledger persistence.
- **SC-004**: All published Agent Card conformance examples produce identical
  decisions in every supported language implementation.
- **SC-005**: Every A2A interaction declared by the Phase 1 Profile has at least
  one verifiable success case and one incompatible-response case where relevant.
- **SC-006**: A two-process demonstration routes 100% of resolution calls to the
  Control Plane and 100% of dispatch/query calls to the Router.
- **SC-007**: No Agent input, Agent output, credential, or internal dependency
  detail appears in Ledger facts or fixed public errors during acceptance tests.
- **SC-008**: 100% of Agent Cards containing endpoint URI userinfo are rejected
  by the language-neutral structural contract.
- **SC-009**: 100% of malformed conformance manifests with omitted/null required
  fields, duplicate members, or unsafe paths are rejected consistently.
- **SC-010**: 100% of A2A conformance cases either execute every rule and type
  claim declared by their manifest metadata or fail manifest validation before
  the fixture is treated as covered.
- **SC-011**: All four required A2A operations pass through the pinned SDK
  client method and server handler in conformance tests, and all semantically
  empty `message/send` Message results are rejected.

## Assumptions

- The repository constitution remains the source for Control Plane/Data Plane
  ownership and the Frontend-to-Gateway-only rule.
- Phase 1 supports both non-streaming and streaming invocation modes as already
  stated by the accepted architecture.
- Agent Card Schema and Agent version identifiers remain separate and versioned.
- Invocation Results are transient response data and are not retained for
  polling, replay, or reconnect.

## Non-Goals

- Persisting Agent input or output in the Invocation Ledger.
- Persisting or replaying Invocation Results through a separate result store.
- Adding a Scheduler, Planner, Agent runtime deployment, billing, rating, or
  enterprise governance workflow.
- Allowing the Frontend to access the Router or Agent endpoint directly.
- Replacing the pinned A2A protocol or changing the platform technology stack.
- Implementing Catalog, Workspace, Router transport, or sample Agents beyond
  changes required to make their shared contracts implementable.
