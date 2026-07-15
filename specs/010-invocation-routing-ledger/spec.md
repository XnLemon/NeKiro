# Feature Specification: Deliver Invocation Routing and Ledger

**Feature Branch**: `codex/010-invocation-routing-ledger`

**Created**: 2026-07-16

**Status**: Ready for T001 contract and policy gate

**Input**: Define the next backend platform stage after Workspace acceptance,
update the repository planning facts, and split the work into a GitHub parent
task with dependency-aware sub-issues.

## Context

Catalog registration/discovery and Workspace installation/authorization are
implemented. The next Phase 1 value gap is the managed execution boundary:
an authorized Workspace caller still cannot invoke an installed Agent through
the platform or inspect the resulting invocation lineage.

This feature plans the backend `Invoke -> Record` stage as one parent delivery
with independently reviewed child slices. It includes Invocation Dispatch,
the separately deployed A2A Router, append-only Ledger facts, transient JSON
and streaming results, a thin Agent SDK for nested calls, two independently
implemented sample Agents, and backend acceptance. It does not start Console
implementation.

Workspace readiness hardening PR #18 is merged, its project closure CI jobs are
green, and Workspace parent Issue #2 is closed after fresh independent review.
T001 is the only gate before parallel runtime child work begins.

## Clarifications

### Session 2026-07-16

- Existing active Invocation, result, error, internal API, and A2A Profile
  contracts are the starting policy. The first child is a contract/design gate
  that may correct proven contradictions but must not invent compatibility
  fallbacks.
- Invocation results use the live request: one JSON result or one ordered
  result stream. Results are not stored, replayed, polled, or recovered from
  the Ledger.
- Ledger data is append-only, metadata-only, and durable. Agent input, result,
  chunks, credentials, and internal dependency details are excluded.
- Platform-managed user and nested Agent calls always traverse the Router.
  Sample Agents use different Runtime implementations and share only platform
  contracts and A2A behavior.
- Console, Agent deployment, model/tool/workflow runtime features, billing,
  quota, general policy engines, and production identity federation remain
  outside this parent delivery.

## User Scenarios & Testing

### User Story 1 - Invoke an Installed Agent (Priority: P1)

As a Workspace caller, I need to invoke an installed, enabled, authorized Agent
through the trusted platform boundary and receive its exact JSON or streaming
result without contacting the Agent directly.

**Why this priority**: This closes the user-visible `Invoke` step and proves
that Workspace authorization controls real Agent execution.

**Independent Test**: Register, publish, install, and invoke one sample Agent
through the public boundary in both result modes, then verify the Agent saw the
platform correlation context and the caller received the exact result.

**Acceptance Scenarios**:

1. **Given** an enabled Installation whose accepted permissions authorize the
   requested capability, **when** the owner requests a non-streaming
   invocation, **then** the platform returns one correlated successful result.
2. **Given** the same authorized Installation, **when** the owner requests a
   stream, **then** the stream starts with acceptance, contains ordered chunks,
   and ends with exactly one successful terminal event.
3. **Given** an uninstalled, disabled, Catalog-disabled, or permission-incomplete
   Installation, **when** invocation is requested, **then** execution is
   rejected with the exact owning-boundary outcome before any Agent call.
4. **Given** a caller that bypasses the Gateway and addresses a managed Agent
   directly, **when** platform acceptance is evaluated, **then** that call is
   outside the managed path and creates no false platform success or Ledger fact.

---

### User Story 2 - Inspect Durable Invocation Facts (Priority: P1)

As a Workspace owner or operator, I need durable metadata-only invocation and
trace history so I can audit success and diagnose routing, protocol, Agent,
timeout, and cancellation outcomes without exposing Agent content.

**Why this priority**: `Record` is part of the Phase 1 loop and is required to
make Router behavior trustworthy rather than opaque.

**Independent Test**: Complete successful and failed invocations, reconstruct
the Router/Ledger process, and query one invocation and one trace. Verify exact
ordered events, terminal immutability, lineage, Workspace authorization, and
the absence of input/output content.

**Acceptance Scenarios**:

1. **Given** a completed invocation, **when** its metadata is queried after a
   process restart, **then** all append-only lifecycle events and the derived
   terminal projection remain available in deterministic order.
2. **Given** a trace containing parent and child invocations, **when** the trace
   is queried, **then** every invocation appears once with exact parent/root
   relationships and no unrelated Workspace facts.
3. **Given** Agent input, streaming chunks, final output, credentials, or
   dependency error text, **when** stored rows, returned events, and logs are
   inspected, **then** none of that content appears.
4. **Given** a missing invocation, foreign Workspace, or unavailable Ledger,
   **when** a read is requested, **then** not-found, forbidden, and dependency
   failure remain distinguishable and never become an empty successful history.

---

### User Story 3 - Perform a Cross-Runtime Nested Call (Priority: P1)

As an Agent developer, I need my Agent to call another installed Agent through
the thin platform SDK so nested work preserves authorization and lineage even
when the two Agents use different Runtime implementations.

**Why this priority**: This is the required proof that NeKiro is a cross-Runtime
operating platform rather than a wrapper around one Agent framework.

**Independent Test**: Invoke Sample Agent A through the Gateway, have A use the
SDK to invoke Sample Agent B through the Router, and verify the returned result
plus one durable parent-child trace across the two Runtime implementations.

**Acceptance Scenarios**:

1. **Given** Agent A receives trusted platform context, **when** it requests an
   authorized nested capability from Agent B, **then** the Router creates a new
   child invocation with the same root task and trace and the exact parent ID.
2. **Given** Agent A and Agent B use different Runtime implementations, **when**
   they complete the nested call, **then** no Runtime-internal type, memory,
   storage, or workflow object crosses the platform boundary.
3. **Given** missing or malformed platform context, **when** the SDK attempts a
   nested call, **then** it fails explicitly and does not synthesize identity,
   route directly to Agent B, or create an orphan invocation.

---

### User Story 4 - Preserve Failure, Timeout, and Cancellation Semantics (Priority: P1)

As a platform maintainer, I need every pre-dispatch and in-flight failure to
remain explicit and correlated so callers and operators can distinguish policy,
routing, protocol, Agent, dependency, timeout, cancellation, and interrupted
delivery outcomes.

**Why this priority**: A successful happy path without trustworthy failure
facts cannot satisfy the managed invocation boundary.

**Independent Test**: Exercise each documented failure before and after stream
commitment. Verify pre-acceptance failures return an explicit safe error and no
Ledger fact; accepted failures have a committed terminal fact; and only when
Ledger persistence itself fails after an external side effect, the caller gets
explicit non-success with durable non-terminal audit history. Verify
classification, correlation, secrecy, and terminal immutability.

**Acceptance Scenarios**:

1. **Given** request mode and accepted response mode disagree, **when** the
   invocation is validated, **then** the platform rejects it without dispatch.
2. **Given** no route, an unreachable Agent, invalid protocol data, Agent
   failure, or a required dependency failure, **when** invocation runs, **then**
   each outcome is classified distinctly and correlated.
3. **Given** caller cancellation or deadline expiry, **when** work is in flight,
   **then** cancellation is propagated, the first successfully committed
   terminal fact wins, and a later result cannot overwrite it.
4. **Given** a committed stream ends without a terminal event, **when** delivery
   is assessed, **then** preceding chunks remain incomplete and are never
   reported or recorded as successful output.

### Edge Cases

- Authentication or request validation fails before an Invocation context is
  trusted; no fabricated Invocation or root-task identity is returned.
- The Installation changes state after Dispatch authorizes but before Router
  resolution; the Router's controlled exact re-resolution is authoritative for
  whether the Agent call proceeds.
- The Agent Card endpoint contains user info, an unsupported protocol version,
  or an endpoint that cannot be used under the active routing policy.
- A non-streaming A2A response returns a Task rather than a terminal Message,
  requiring the supported task lifecycle without result polling at the public
  API.
- Streaming events repeat, skip order, change correlation, emit content after a
  terminal event, or end before a terminal event.
- Cancellation races with completion or timeout; only the first successfully
  committed terminal outcome becomes durable and visible.
- Ledger append fails before or after an Agent interaction; the invocation must
  not be reported as a clean success with missing required audit facts.
- Duplicate event delivery or a process restart must not create duplicate event
  identity, sequence, or terminal projections.
- Concurrent parent and child invocations must not leak correlation or results
  across Workspaces.
- A nested call targets an Agent that is absent, disabled, or lacks accepted
  permissions in the same Workspace.

## Requirements

### Functional Requirements

- **FR-001**: The platform MUST accept managed invocation requests only through
  the authenticated Gateway and MUST derive the caller identity from trusted
  authentication context.
- **FR-002**: The platform MUST create exact Invocation, root Task, and Trace
  correlation for every accepted root invocation and MUST preserve those values
  across Dispatch, Router, Agent transport, results, errors, and Ledger facts.
- **FR-003**: Invocation Dispatch MUST authorize the requested Workspace,
  Installation, exact Agent version, capability, and accepted permissions
  before sending work to the Router.
- **FR-004**: Control Plane managed execution MUST call only the Router boundary;
  it MUST NOT call an Agent endpoint directly.
- **FR-005**: The Router MUST re-resolve the exact authorized Agent through the
  controlled Control Plane boundary and MUST NOT read Catalog or Workspace
  storage directly or retain an independent permanent Agent Card copy.
- **FR-006**: The Router MUST invoke only the protocol endpoint and operations
  allowed by the active A2A Profile and MUST validate response kind, task state,
  correlation, and terminal behavior.
- **FR-007**: A non-streaming request MUST return exactly one correlated
  successful result or one explicit pre-commit error; it MUST NOT expose a
  polling or replay lifecycle.
- **FR-008**: A streaming request MUST return ordered correlated events,
  beginning with acceptance and ending with exactly one completed, failed,
  canceled, or timed-out terminal event.
- **FR-009**: Cancellation and deadlines MUST propagate across Gateway,
  Dispatch, Router, and Agent transport, and the first valid terminal outcome
  MUST be immutable.
- **FR-010**: The Router MUST commit the initial append-only `created` fact
  before any Agent side effect; that successful commit defines the accepted
  Invocation boundary. Authentication, validation, media-mode, Dispatch, Router
  connectivity, or initial-Ledger failures before that boundary MUST return an
  explicit safe error and MUST NOT fabricate a Ledger fact. The Ledger MUST persist every successfully
  committed lifecycle fact with caller, Workspace, target, capability, exact
  Card version, status, timing, lineage, and classified error. A clean terminal
  result MUST NOT be reported until its terminal fact commits; if persistence
  itself fails after an Agent side effect, the invocation MUST remain explicit
  non-success and MAY retain a non-terminal audit history rather than a
  fabricated terminal fact.
- **FR-011**: Ledger events, projections, logs, and query responses MUST NOT
  contain Agent input, result values, chunk values, credentials, tokens, raw
  dependency errors, or internal stack details.
- **FR-012**: Authorized readers MUST be able to inspect one Invocation and one
  complete Trace after process restart without receiving facts from another
  Workspace.
- **FR-013**: The Agent SDK MUST remain limited to contract conformance,
  platform context validation/propagation, and nested calls through the Router.
- **FR-014**: A managed nested call MUST create a child Invocation with the same
  root Task and Trace, the calling Invocation as parent, and a newly assigned
  Invocation identity.
- **FR-015**: Two sample Agents implemented with different Runtime approaches
  MUST pass the same A2A conformance and complete one Router-mediated nested call
  without shared Runtime-internal types or storage.
- **FR-016**: Validation, authentication, authorization, not-found, disabled,
  mode mismatch, route, Agent availability, protocol, Agent execution,
  dependency, timeout, cancellation, and interrupted delivery outcomes MUST
  remain distinguishable at their owning boundary.
- **FR-017**: Required Router destinations, listener addresses, database
  configuration, service credentials, deadlines, and security material MUST be
  explicit and validated; missing or invalid values MUST NOT fall back to
  localhost, mock identities, weak secrets, or inferred defaults.
- **FR-018**: Invocation results and stream chunks MUST remain transient and
  MUST NOT be persisted, replayed, exposed by a result query endpoint, or
  reconstructed from Ledger metadata.
- **FR-019**: Concurrent root and child invocations MUST preserve per-call
  correlation, event order, terminal uniqueness, and Workspace isolation.
- **FR-020**: Each child delivery MUST have its own Spec, clarification, plan,
  tasks, mapped post-implementation tests, independent review, and convergence
  evidence before the parent can close.
- **FR-021**: The feature MUST add no retry, cache, alternate route, stale Card,
  degraded success, compatibility branch, or other fallback unless a separate
  approved policy source explicitly requires it.

### Key Entities

- **Invocation Context**: Trusted identity and lineage for one managed root or
  child call, including caller, Workspace, target, capability, exact version,
  Invocation ID, root Task ID, optional parent ID, and Trace ID.
- **Invocation Event**: One append-only metadata fact in an Invocation lifecycle
  with stable identity, sequence, time, status, optional safe error, and no
  Agent content.
- **Invocation Projection**: A read model derived from ordered events for one
  Invocation; it never replaces the event history.
- **Trace Lineage**: The Workspace-authorized set of root and child Invocations
  sharing one Trace and root Task.
- **Invocation Result**: The transient successful JSON value returned on the
  live non-streaming request.
- **Result Stream Event**: One transient ordered acceptance, chunk, or terminal
  value returned on the live streaming request.
- **Resolved Route**: The exact authorized Agent Card version and endpoint
  obtained by the Router from the Control Plane for this invocation.
- **Platform Context**: The trusted correlation and Workspace values propagated
  to an Agent and validated by the SDK for a nested call.

### Runtime/Platform Boundary

- **Platform-owned behavior**: Gateway authentication, Workspace authorization,
  Invocation identity, routing, A2A protocol adaptation, cancellation/timeout,
  transient result forwarding, append-only lineage, and metadata reads.
- **Runtime-owned behavior**: Model calls, prompts, tools, workflows, memory,
  RAG, sessions, task internals, and the business meaning of Agent results.
- **Cross-runtime proof**: Two independently implemented sample Agents pass the
  same profile; Agent A invokes Agent B only through the SDK and Router, and one
  metadata-only Ledger trace proves the parent-child relationship.

## Success Criteria

### Measurable Outcomes

- **SC-001**: One clean-environment acceptance run completes
  `Register -> Discover -> Install -> Invoke -> Record` without the caller or
  Control Plane directly addressing an Agent endpoint.
- **SC-002**: Non-streaming acceptance returns one exact result, while every
  clean streaming run begins at sequence 0, has gap-free event/chunk order, and
  ends with exactly one terminal event.
- **SC-003**: A 100-request concurrent invocation run produces 100 isolated
  Invocation identities, no cross-call correlation, and exactly one durable
  terminal outcome per accepted Invocation.
- **SC-004**: One cross-Runtime nested acceptance run produces exactly two
  correlated Invocations with one root Task and Trace and the exact parent-child
  relationship.
- **SC-005**: Every documented policy, route, protocol, Agent, dependency,
  timeout, cancellation, and interruption failure case yields its expected
  caller-visible class without a false success. Pre-acceptance failures have no
  Ledger fact; accepted cases whose terminal append commits have a matching
  Ledger terminal fact; an induced post-side-effect Ledger persistence failure
  instead leaves an explicitly verified non-terminal audit history and no
  successful result.
- **SC-006**: After Router/Ledger process reconstruction, 100% of committed
  Invocation events and trace relationships remain queryable in deterministic
  order, while no transient result becomes recoverable.
- **SC-007**: Both sample Agents pass 100% of the active A2A Profile conformance
  corpus used by the platform.
- **SC-008**: Automated storage, event, log, and API inspections find zero Agent
  input values, result values, chunk values, credentials, or raw dependency
  details in persistent or metadata-only surfaces.
- **SC-009**: Every FR-001 through FR-021 item maps to at least one child issue,
  implementation task, deterministic verification, and final acceptance result.

## Assumptions

- Agent Card `0.2`, Workspace `1`, Installation `2`, Northbound API `v3`,
  Control Plane Internal API `v2`, Router Internal API `v2`, Invocation Event
  `0.2`, Invocation Result/Stream Event `v1`, Platform Error `v2`/`v3`, and the
  A2A Profile for protocol `0.3.0` remain the active contract starting point.
- Workspace exact resolution remains the Control Plane fact boundary used by
  the Router; the new parent does not move ownership into the data plane.
- PostgreSQL remains the Phase 1 durable store, with a Router/Ledger-owned
  schema distinct from Catalog and Workspace ownership.
- The existing protocol-focused A2A library may be used by Router and sample
  adapters; no full Agent Runtime framework becomes a core dependency.
- Workspace parent Issue #2 is closed; runtime children use the merged
  Workspace implementation only through the approved ports and contracts.

## Non-Goals

- React Console invocation or trace screens.
- Result persistence, polling, replay, reconnect cursor, resume, or result query.
- Agent deployment, health orchestration, Kubernetes, autoscaling, scheduling,
  secrets distribution, or runtime lifecycle management.
- Model providers, prompts, tools, planners, workflows, memory, RAG, sessions,
  evaluation, or other Agent Runtime features in platform core or SDK.
- General RBAC, Workspace membership, quota, approval, billing, rating,
  marketplace, federation, certification, or recommendation behavior.
- Message queues, search clusters, distributed caches, alternate routing data
  sources, speculative retries, old-contract runtime support, or degraded success.
- Persisting or exposing Agent input, output, chunks, credentials, dependency
  error details, or Runtime-internal telemetry in Ledger.

## Fallback Policy

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```

This planning feature adds no fallback. Any child that discovers a required
fallback policy must return to specification and cite its pre-existing product,
contract, ADR, Runbook, SLO, or caller-policy source before implementation.
