# Feature Specification: Workspace Client SDK

**Feature Branch**: `codex/workspace-client-sdk`

**Created**: 2026-07-24

**Status**: Draft

**Input**: GitHub Issue #51, "Workspace Client SDK and application credentials", under parent Issue #47

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Invoke an installed Agent from application code (Priority: P1)

A Workspace owner configures one application client with the platform Gateway,
Workspace identity, and an opaque application credential. Business code can
then invoke an installed Agent by supplying only the Agent identity,
capability, and JSON input. It does not need to know the installed version,
verified Release, provider endpoint, Router address, or Agent credential.

**Why this priority**: This is the application-facing value of installing an
Agent into a Workspace. Without it, consumers still have to reproduce platform
routing and trust details themselves.

**Independent Test**: Configure a client for an existing Workspace, invoke one
installed capability, and receive a correlated successful result while the
request model contains no routing or provider-controlled destination fields.

**Acceptance Scenarios**:

1. **Given** a published verified Release is installed and enabled in a
   Workspace owned by the authenticated application principal, **When**
   business code invokes its allowed capability with a JSON object, **Then**
   the call traverses Gateway and returns the Agent result with the platform
   invocation, root Task, and Trace identifiers.
2. **Given** the same application client, **When** business code invokes
   another installed Agent, **Then** it changes only Agent identity,
   capability, and input; no endpoint, version, Release, Router, or Agent
   secret is supplied.
3. **Given** an application credential is missing, blank, malformed, or not
   recognized by Gateway, **When** the client is configured or invoked,
   **Then** execution fails explicitly and no Agent endpoint is contacted.

---

### User Story 2 - Consume a live streaming result (Priority: P2)

An application can request the streaming form of the same installed Agent
invocation and process accepted, chunk, and terminal events incrementally. The
application can cancel through its normal request context and can distinguish
a valid terminal completion from an interrupted stream.

**Why this priority**: Long-running Agents need incremental delivery, but a
stream must preserve the same Workspace authorization and correlation guarantees
as a non-streaming call.

**Independent Test**: Invoke an installed streaming capability, consume its
events through the terminal event, and verify order, correlation, chunks, and
completion without buffering the complete result first.

**Acceptance Scenarios**:

1. **Given** an enabled Installation and a streaming Agent, **When** the
   application consumes the stream, **Then** it receives exactly one accepted
   event, zero or more ordered chunks, and exactly one correlated terminal
   event.
2. **Given** an active streaming call, **When** the application cancels its
   request context, **Then** the SDK stops delivery, reports cancellation or
   interruption, and does not retry or report a successful terminal result.
3. **Given** a stream ends before a valid terminal event or changes its
   correlation identifiers, **When** the application reads or closes it,
   **Then** the SDK reports a protocol/interruption error rather than treating
   partial output as success.

---

### User Story 3 - Handle platform failures with safe typed context (Priority: P3)

Application code can branch on stable platform error codes and inspect safe
Trace and, when available, invocation correlation fields. It never needs to
parse raw response text or infer authorization state from an empty result.

**Why this priority**: Installed Agent use is operational only when applications
can distinguish authorization, lifecycle, dependency, timeout, cancellation,
and protocol failures without leaking credentials or provider details.

**Independent Test**: Exercise every active Gateway failure category and prove
the returned typed error preserves the exact safe code/status/correlation while
discarding raw and unrecognized response content.

**Acceptance Scenarios**:

1. **Given** an Agent is not installed, its Installation is disabled, its
   capability is not accepted, or its Release is suspended or revoked, **When**
   the application invokes it, **Then** the SDK returns the corresponding
   distinct typed platform error.
2. **Given** Gateway rejects a request before invocation correlation exists,
   **When** the SDK returns the error, **Then** it exposes the Gateway Trace and
   no fabricated invocation or root Task identifier.
3. **Given** failure occurs after invocation correlation exists, **When** the
   SDK returns the error, **Then** it exposes the exact invocation, root Task,
   and Trace identifiers supplied by the platform.
4. **Given** an error response has invalid media, shape, status/code pairing,
   correlation, or Trace header, **When** the SDK receives it, **Then** it
   returns a local response-validation error and does not expose the raw body.
5. **Given** the internal Router response omits, duplicates, or changes the
   Gateway-created Trace, **When** Control Plane receives it, **Then** the
   internal response is rejected and the different downstream Trace is never
   exposed as authoritative to the application.
6. **Given** an internal platform failure occurs before or after invocation
   acceptance, **When** it reaches the corresponding HTTP boundary, **Then**
   `INTERNAL_ERROR` is represented by HTTP 500 with the correct pre- or
   correlated shape rather than being collapsed into dependency unavailable.

### Edge Cases

- The Gateway origin is missing, blank, non-canonical, contains credentials, or
  names a path/query/fragment not allowed by the client contract, or the
  Workspace ID is missing or invalid.
- The Agent identity or capability violates the platform identifier grammar,
  the input is absent, or the input is not a JSON object.
- A request or response exceeds an explicitly configured byte limit.
- Gateway redirects the invocation request, returns an unexpected media type,
  or returns trailing/duplicate/unknown JSON members.
- A successful result, stream event, error body, and `x-nek-trace-id` header do
  not agree on correlation.
- The application credential contains whitespace or control characters, or a
  response attempts to echo credential-like material.
- The caller context is already canceled or expires while connecting, reading
  JSON, or consuming a stream.
- A stream is closed before acceptance, after acceptance but before terminal,
  after terminal but before end-of-stream, or more than once.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The platform MUST provide a lightweight application-facing Client
  SDK that sends all managed application invocations through Gateway.
- **FR-002**: A Client instance MUST be bound to exactly one explicit Gateway
  origin, one Workspace identity, and one opaque application credential.
- **FR-003**: The Phase 1 application credential MUST authenticate an existing
  Workspace Owner principal. Gateway MUST remain the authority that decides
  whether that principal may invoke in the bound Workspace.
- **FR-004**: Credential issuance, persistence, listing, revocation, rotation,
  and role delegation MUST NOT be introduced by this feature. The credential
  is supplied to the application out of band and is never minted by the SDK.
- **FR-005**: An invocation request exposed to business code MUST contain only
  Agent identity, capability, and a required JSON-object input.
- **FR-006**: The public invocation request MUST NOT accept an Agent version,
  Release identity, Card digest, provider endpoint, Router address, provider
  credential, Agent credential, Workspace identity, or application credential.
- **FR-007**: The SDK MUST propagate the opaque application credential only as
  the single Gateway authorization value for the configured request and MUST
  NOT include it in a request body, result, error, log-oriented representation,
  or public accessor.
- **FR-008**: The SDK MUST support both non-streaming result delivery and live
  streaming delivery through the active Gateway invocation contract.
- **FR-009**: Non-streaming success MUST expose the Agent result and exact
  platform-assigned invocation, root Task, and Trace identifiers.
- **FR-010**: Streaming delivery MUST expose validated events incrementally,
  require one accepted event before other events, enforce ordered chunks and
  correlation, and require exactly one terminal event followed by actual
  end-of-stream before clean completion. Closing after terminal but before
  end-of-stream MUST remain an interrupted stream.
- **FR-011**: The SDK MUST propagate caller cancellation and deadlines to the
  managed request. It MUST NOT retry, redirect, switch destinations, poll the
  Ledger for result content, or convert interruption into success.
- **FR-012**: The SDK MUST require explicit request, response, and streaming
  event size policies and fail configuration or processing when a limit is
  missing, invalid, or exceeded. It MUST NOT invent size defaults.
- **FR-013**: The SDK MUST validate the active success and error representations,
  media type, HTTP status, Trace header, and correlation before returning
  platform data to application code.
- **FR-014**: Gateway Platform Error v4 failures MUST be returned as a typed SDK
  error containing only the HTTP status, stable code, Trace ID, and optional
  correlated invocation/root Task pair. Raw response bodies and unknown fields
  MUST NOT be exposed.
- **FR-015**: Not-installed, Installation-disabled, capability-denied,
  Release-unpublished, Release-suspended, and Release-revoked states MUST remain
  distinguishable; missing, forbidden, dependency, timeout, cancellation, and
  protocol failures MUST NOT be collapsed into an empty or successful result.
- **FR-016**: The SDK MUST NOT import Control Plane, Router, Registry, Workspace,
  or Agent Runtime internal implementations. It may depend only on public
  language-neutral contract mappings and transport interfaces.
- **FR-017**: The Client SDK MUST remain distinct from the Agent SDK. It MUST NOT
  accept Router Agent bindings or trusted nested-invocation platform context.
- **FR-018**: Documentation MUST show the boundary between installing an Agent
  through Gateway and invoking the resulting Workspace Installation from
  application code, including credential handling and typed failure examples.
- **FR-019**: Contract evidence MUST cover configuration, non-streaming,
  streaming, cancellation, correlation, all active Gateway error mappings,
  credential secrecy, and rejection of every forbidden routing input.
- **FR-020**: The Gateway-created Trace MUST remain the single northbound Trace
  fact. The Control Plane Router adapter MUST require exactly one matching
  internal Trace header and Gateway MUST NOT replace its Trace with an absent,
  duplicate, or different downstream value.
- **FR-021**: Router Internal v4 and Northbound Invocation v4 MUST explicitly
  declare and return HTTP 500 for `INTERNAL_ERROR` using the phase-appropriate
  Platform Error v4 shape. `INTERNAL_ERROR` MUST NOT fall through to the
  dependency-unavailable status.

### Key Entities

- **Client Configuration**: One Gateway origin, Workspace identity, opaque
  application credential, transport, and explicit byte limits. It is
  application-local configuration, not a persisted platform record.
- **Application Credential**: An opaque secret presented to Gateway and mapped
  to an existing Owner principal in Phase 1. Its lifecycle is outside this
  feature.
- **Application Invocation Request**: Agent identity, capability, and JSON
  object input supplied by business code.
- **Application Invocation Result**: Agent result plus platform-assigned
  invocation, root Task, and Trace identifiers.
- **Application Result Stream**: Incremental correlated accepted/chunk/terminal
  events with explicit interruption and close semantics.
- **Client Platform Error**: Safe typed status, platform code, Trace, and
  optional invocation/root Task correlation reconstructed only from a valid
  Gateway error response.

### Runtime/Platform Boundary *(mandatory when feature touches Agent execution or integration)*

- **Platform-owned behavior**: Gateway authentication, Workspace Owner
  authorization, Installation/Release/capability resolution, Router-mediated
  invocation, platform correlation, and typed error/result contracts.
- **Runtime-owned behavior**: Agent model, prompt, tools, workflow, memory,
  session, output content, and internal execution remain outside the Client SDK.
- **Cross-runtime proof**: The same SDK request model invokes either existing
  sample Runtime through Gateway without importing a Runtime type or changing
  routing inputs.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An application can invoke either sample Runtime after installation
  while supplying exactly three per-call business fields: Agent identity,
  capability, and input.
- **SC-002**: All non-streaming and streaming acceptance cases preserve one
  platform-assigned invocation/root Task/Trace correlation with zero endpoint,
  version, Release, Router, or Agent-secret inputs from business code.
- **SC-003**: Every supported platform rejection in the acceptance matrix is
  returned under its exact stable code; zero cases become an empty result,
  generic success, or different lifecycle state.
- **SC-004**: Every valid stream is processed incrementally through exactly one
  terminal event, and 100% of truncated, reordered, post-terminal, or
  correlation-changing streams are rejected.
- **SC-005**: Credential scans across public request/result/error values, test
  output, and documented examples find zero raw application credentials,
  provider credentials, Router credentials, or Agent credentials.
- **SC-006**: Contract, unit, race, vet, lint, and application-example checks
  complete with no blocking finding, and an independent Review reports zero
  High or Medium issues.
- **SC-007**: Contract and adapter tests reject 100% of missing, duplicate, or
  mismatched internal Trace headers and map 100% of `INTERNAL_ERROR` cases to
  HTTP 500 without changing the Gateway-created Trace.

## Assumptions

- Issues #49 and #50 provide immutable trusted Release resolution and
  authenticated Router-to-Agent delivery before this SDK is used.
- The active Gateway invocation contract remains Northbound Invocation v4 with
  Platform Error v4, Invocation Result v1, and Result Stream Event v2.
- The existing Owner-only Workspace policy is the Phase 1 authorization model;
  an application uses an out-of-band credential mapped to that Owner principal.
- Gateway, not the application, assigns invocation, root Task, and Trace
  identifiers. The SDK propagates these identifiers back to application code
  and does not accept caller-generated platform correlation.
- Installation is an explicit operation completed before invocation. This SDK
  does not silently install, enable, upgrade, or replace an Agent.
- One Client instance intentionally targets one Workspace. Applications using
  multiple Workspaces construct separate clients and credentials.

## Non-Goals

- Application credential issuance, storage, management APIs, rotation,
  revocation lists, OAuth/OIDC, delegated roles, service accounts, or complete
  RBAC.
- Catalog discovery, Agent registration/publication, Workspace creation, or
  Installation lifecycle methods in the Client SDK.
- Direct Registry, Router, Ledger-store, database, or Agent endpoint access.
- Agent deployment, provider billing, quota, retry, cache, offline result
  recovery, or result persistence.
- TypeScript or other language SDKs in this slice.
- Model, prompt, tool, workflow, memory, RAG, session, or other Agent Runtime
  behavior.
