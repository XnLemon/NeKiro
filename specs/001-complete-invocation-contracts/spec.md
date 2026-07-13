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
- Q: How do strict JSON DTO boundaries handle legal JSON numbers outside a
  native bounded numeric range? → A: Duplicate-member and syntax validation
  preserves the JSON number token without applying an implementation numeric
  range. A later field-specific contract may impose a range, but unconstrained
  Invocation `result` and `chunk` values preserve legal values such as `1e400`.

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
4. **Given** a successful result or result chunk containing a legal JSON number
   such as `1e400`, **When** a strict public DTO boundary checks it for
   duplicate members and decodes it, **Then** the value is accepted and its
   numeric token is preserved without bounded-number coercion.

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
- Agent resolution is unavailable after an invocation has been accepted and
  its failure must remain correlated to the existing invocation, root task, and
  trace.
- A Router Ledger or trace read encounters dependency failure, which must not
  be reported as Agent or route unavailability.
- A nested terminal Platform Error carries identifiers that differ from its
  enclosing Invocation Event or result-stream event.
- A post-dispatch HTTP failure omits invocation or root-task correlation even
  though the Invocation already exists.
- A non-streaming success carries valid identifiers that belong to a different
  Invocation than the request being completed.
- A public result, event, error, envelope, or resolution DTO repeats a JSON
  member and different parsers select different values.
- A result or chunk contains a syntactically legal JSON number such as `1e400`,
  including at a nested depth, that exceeds a native floating-point range but
  is not restricted by the capability output schema.
- A nominally valid JSON-RPC response contains both `result` and `error`, or an
  ID type that the pinned SDK/server cannot accept.
- An invalid A2A fixture fails, but for a different protocol reason than the
  `protocolError` declared by its manifest case.
- An A2A Profile operation declares result, event, or error fields belonging to
  a different operation while still satisfying its required fields.

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
- **FR-018**: Agent resolution performed after invocation creation MUST receive
  the existing `invocation_id`, `root_task_id`, and `trace_id`. Every resolution
  failure MUST preserve those exact identifiers in its fixed public error and
  related Ledger facts; neither side may generate replacement correlation.
- **FR-019**: Each active API operation MUST declare only error codes that can
  arise from that operation. Router Ledger and trace reads MUST distinguish
  dependency failure from dispatch-only route or Agent availability failures.
- **FR-020**: The language-neutral Invocation contracts MUST normatively require
  a nested terminal Platform Error's invocation, root-task, and trace identifiers
  to equal the enclosing Invocation Event or result-stream event identifiers.
  Portable positive and negative conformance cases MUST produce the same
  decision in every supported implementation.
- **FR-021**: Every HTTP error produced after Invocation creation MUST require
  `invocation_id`, `root_task_id`, and `trace_id` and preserve the request's
  exact values. The reusable base error shape MAY keep invocation and root-task
  identifiers conditional only for operations that fail before creation.
- **FR-022**: A non-streaming Invocation Result MUST be validated against the
  expected invocation, root-task, and trace identifiers of the request that is
  being completed; an independently valid result from another Invocation is
  invalid for that request.
- **FR-023**: Public JSON decoders for Invocation results, stream events, Ledger
  events, Platform Errors, Router envelopes, and resolution requests MUST reject
  duplicate object member names before typed decoding. First-member-wins and
  last-member-wins behavior are both nonconforming.
- **FR-024**: Every A2A JSON-RPC response case MUST enforce version `2.0`, a
  pinned-SDK-compatible ID type, and exactly one of `result` or `error` as
  baseline envelope conformance. A valid fixture cannot omit this baseline by
  leaving a rule out of its manifest.
- **FR-025**: An invalid A2A conformance case MUST prove the exact stable failure
  classification declared by `protocolError`. Failure of a prerequisite or a
  different assertion MUST NOT count as evidence for the declared error.
- **FR-026**: Each A2A Profile operation MUST be a closed per-method variant.
  Fields for accepted results, accepted stream events, and expected errors MUST
  be required or forbidden according to that exact operation; incompatible
  operation metadata is structurally invalid.
- **FR-027**: Strict duplicate-member validation at every public JSON DTO
  boundary MUST preserve syntactically legal JSON number tokens without first
  coercing them through a bounded implementation numeric type. Field-specific
  schemas and typed DTO fields MAY apply their declared numeric constraints,
  but unconstrained Invocation `result` and `chunk` values such as `1e400` MUST
  reach validation and delivery without precision loss or range rejection.

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
- **Invocation Correlation Rule**: A language-neutral semantic invariant that
  binds duplicated correlation identifiers across an Invocation envelope and
  its nested Platform Error.
- **Strict JSON DTO Boundary**: The ordered boundary that rejects malformed or
  duplicate-member JSON without adding an implementation-specific numeric
  range before field-specific validation.

### Runtime/Platform Boundary

- **Platform-owned behavior**: NeKiro owns the language-neutral result,
  streaming, error, event, correlation, directional API, and A2A Profile
  contracts plus their portable conformance decisions.
- **Runtime-owned behavior**: Agent Runtimes produce result and chunk content
  and apply capability-specific output semantics; model, tool, workflow,
  memory, and session execution remain outside this feature.
- **Cross-runtime proof**: Raw JSON and A2A conformance corpora must produce the
  same decisions without framework-specific Control Plane or Router behavior.
  The later Phase 1 E2E feature proves this boundary with independently
  implemented sample Agents.

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
- **SC-012**: 100% of accepted invocations that fail during Agent resolution
  return and record the same invocation, root-task, and trace identifiers that
  the Gateway created.
- **SC-013**: 100% of active API operations pass exact status-to-error mapping
  checks, including Router Ledger and trace reads that expose dependency failure
  without dispatch-only error codes.
- **SC-014**: Every published Invocation correlation conformance case produces
  the same valid or invalid decision in each supported language implementation.
- **SC-015**: 100% of post-creation `502`, `503`, and `504` responses in active
  invocation APIs require and preserve invocation, root-task, and trace
  correlation, while pre-creation errors retain their declared semantics.
- **SC-016**: 100% of non-streaming result cases with any mismatched invocation,
  root-task, or trace identifier are rejected before delivery.
- **SC-017**: 100% of Module A public DTO fixtures containing duplicate JSON
  members are rejected before business validation.
- **SC-018**: 100% of A2A response fixtures with both/neither result and error or
  with a non-supported JSON-RPC ID type are rejected by baseline conformance.
- **SC-019**: 100% of invalid A2A cases whose actual failure classification does
  not equal their declared `protocolError` are rejected as malformed cases.
- **SC-020**: 100% of A2A Profile operations containing fields from an
  incompatible operation variant fail structural Schema validation.
- **SC-021**: 100% of fixed non-streaming result and streaming chunk cases that
  contain legal large JSON numbers, including nested `1e400`, decode
  successfully and preserve the exact numeric token while duplicate-member
  cases remain rejected.

## Assumptions

- The repository constitution remains the source for Control Plane/Data Plane
  ownership and the Frontend-to-Gateway-only rule.
- Phase 1 supports both non-streaming and streaming invocation modes as already
  stated by the accepted architecture.
- Agent Card Schema and Agent version identifiers remain separate and versioned.
- Invocation Results are transient response data and are not retained for
  polling, replay, or reconnect.
- JSON number validity follows the language-neutral JSON grammar. Numeric range
  restrictions exist only where a field or resolved capability schema declares
  them; the platform does not invent a range for arbitrary result content.

## Non-Goals

- Persisting Agent input or output in the Invocation Ledger.
- Persisting or replaying Invocation Results through a separate result store.
- Adding a Scheduler, Planner, Agent runtime deployment, billing, rating, or
  enterprise governance workflow.
- Allowing the Frontend to access the Router or Agent endpoint directly.
- Replacing the pinned A2A protocol or changing the platform technology stack.
- Implementing Catalog, Workspace, Router transport, or sample Agents beyond
  changes required to make their shared contracts implementable.
- Normalizing arbitrary Agent result numbers into one platform floating-point
  representation.
