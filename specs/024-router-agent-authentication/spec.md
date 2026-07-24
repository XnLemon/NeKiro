# Feature Specification: Router-to-Agent Authentication

**Feature Branch**: `codex/router-agent-auth`

**Created**: 2026-07-22

**Status**: Draft

**Input**: GitHub Issue #50, "Router-to-Agent authenticated invocation"

## User Scenarios & Testing

### User Story 1 - Accept only an exact managed invocation (Priority: P1)

As an Agent provider, I want my published Agent to accept work only when the
platform proves that the A2A Router authorized the exact Workspace, release,
capability, invocation, and trace context, so a caller cannot bypass NeKiro by
posting directly to the endpoint.

**Why this priority**: This is the trust boundary that gives Router-mediated
invocation value beyond public endpoint discovery.

**Independent Test**: Publish and install one Agent release, invoke it through
the Router, and show that the managed request succeeds while the same A2A body
without a platform credential is rejected before Agent runtime execution.

**Acceptance Scenarios**:

1. **Given** an exact published release installed in a Workspace, **When** the
   Router invokes its endpoint with a valid short-lived platform credential
   bound to the resolved context, **Then** the Agent accepts the request and
   receives the same trusted context.
2. **Given** a reachable Agent endpoint, **When** a caller sends an otherwise
   valid A2A request without a platform credential, **Then** the Agent returns
   typed unauthenticated semantics and does not run Agent logic.
3. **Given** a platform-signed credential, **When** any bound Workspace,
   Agent/version, release/digest, capability, invocation, root task, parent,
   or trace value differs from the delivered context, **Then** the Agent
   returns typed forbidden semantics and does not run Agent logic.

---

### User Story 2 - Reject invalid and replayed credentials (Priority: P1)

As a Workspace owner, I want captured, forged, expired, or misdirected
credentials to be unusable, so an accepted call represents one current
Router authorization rather than a reusable endpoint key.

**Why this priority**: Signature validation without recipient, lifetime, and
single-use enforcement would still permit impersonation or replay.

**Independent Test**: Exercise the Agent boundary with forged, expired,
wrong-issuer, wrong-audience, duplicate, malformed, and replayed credentials
and verify that every request is rejected before Agent runtime execution.

**Acceptance Scenarios**:

1. **Given** an altered or unknown-key credential, **When** it reaches the
   Agent, **Then** the Agent returns typed unauthenticated semantics.
2. **Given** an expired credential or a credential whose lifetime exceeds the
   profile maximum, **When** it reaches the Agent, **Then** the Agent returns
   typed unauthenticated semantics.
3. **Given** a validly signed credential intended for another Agent endpoint,
   **When** it reaches this Agent, **Then** the Agent returns typed forbidden
   semantics.
4. **Given** a credential already accepted once, **When** the same credential
   is presented again before or after its expiry, **Then** the Agent rejects
   the replay and does not execute the invocation a second time.

---

### User Story 3 - Preserve JSON, streaming, and nested lineage (Priority: P2)

As a platform operator, I want Router authentication to cover non-streaming,
streaming, cancellation, and Agent-to-Agent calls without changing result or
Ledger semantics, so the existing Invoke-to-Record loop remains observable.

**Why this priority**: The security boundary is incomplete if only one
transport mode works or if nested calls bypass it.

**Independent Test**: Run direct JSON, SSE, and Runtime A to Runtime B nested
calls through the Router and verify successful results, cancellation/failure
classification, and one unchanged parent-child Ledger lineage.

**Acceptance Scenarios**:

1. **Given** a trusted Runtime B release, **When** the Router performs JSON and
   SSE invocations, **Then** both requests carry distinct valid credentials
   and retain their existing results and Ledger lifecycle.
2. **Given** Runtime A is processing a managed parent invocation, **When** it
   uses the Agent SDK to invoke Runtime B through the Router, **Then** both
   Router-to-Agent hops are independently authenticated and the child remains
   linked to the same root task and trace.
3. **Given** a streaming invocation needs a protocol cancellation request,
   **When** the Router issues that second HTTP request, **Then** it uses a new
   single-use credential bound to the same invocation context.
4. **Given** Agent authentication rejects a Router request, **When** the
   Router records the managed invocation outcome, **Then** existing failure
   correlation is retained and no credential or key material is recorded.

### Edge Cases

- Multiple `Authorization` headers, a non-Bearer scheme, an empty Bearer
  value, malformed token segments, duplicate token fields, and unsupported
  signing algorithms are unauthenticated failures.
- A valid signature with a missing required claim is unauthenticated; a valid
  signature whose audience or bound context differs is forbidden.
- A credential is not accepted at its expiration instant. No clock-skew
  leeway, retry, or alternate credential path is implicit.
- Two requests presenting the same valid credential concurrently result in
  at most one runtime execution.
- An Agent process restart does not make an already expired credential valid.
  Cross-restart replay persistence is outside this slice; the short maximum
  lifetime bounds the in-memory replay window.
- Readiness and endpoint ownership challenge routes remain callable for their
  existing purposes; only the A2A execution route requires the credential.

## Requirements

### Functional Requirements

- **FR-001**: The Router MUST issue a new platform-signed, short-lived JWT for
  every outbound A2A HTTP request, including a separate cancellation request.
- **FR-002**: The credential profile MUST require an exact issuer, exactly one
  audience, expiration, issuance time, unique token ID, Workspace, target
  Agent ID and Card version, release ID and Card digest, capability,
  invocation ID, root task ID, optional parent invocation ID, and trace ID.
- **FR-003**: Credentials MUST use one explicitly configured asymmetric
  signing identity. Missing, blank, malformed, or whitespace-repaired private
  or public key material MUST fail at the owning process boundary.
- **FR-004**: The Agent adapter MUST validate the signing algorithm, key ID,
  signature, issuer, audience, issuance time, expiry, maximum lifetime, token
  ID, required claims, and exact equality of every credential-bound platform
  context value before forwarding the request to Agent runtime logic.
- **FR-005**: The Agent adapter MUST atomically accept a token ID at most once
  during its validity window and MUST remove only expired replay entries.
- **FR-006**: Missing, malformed, forged, expired, wrong-issuer, unknown-key,
  and replayed credentials MUST return HTTP 401 with a stable
  `UNAUTHENTICATED` code. A validly signed wrong-audience or context-mismatched
  credential MUST return HTTP 403 with a stable `FORBIDDEN` code.
- **FR-007**: Authentication rejection MUST happen before the A2A runtime
  handler reads or executes the task and MUST NOT create an Agent-originated
  nested invocation.
- **FR-008**: The Router MUST derive credential claims only from the exact
  trusted dispatch and Catalog resolution already validated for that
  invocation. Request body, Agent response, or caller-supplied headers MUST
  NOT override credential claims.
- **FR-009**: JSON, SSE, cancellation, Agent failure, timeout, and nested
  invocation paths MUST preserve the current result, error, Trace, parent/
  child, and Ledger semantics.
- **FR-010**: Raw credentials, private/public key material, signatures, and
  token IDs MUST NOT appear in Agent Cards, public errors, application logs,
  Invocation events, Ledger projections, or persisted platform tables.
- **FR-011**: The credential contract MUST be language-neutral and versioned;
  Router signing and Agent validation implementations consume that contract
  without importing one another's internal types.
- **FR-012**: Both independently implemented sample Agents MUST enforce the
  same credential profile at their A2A adapter boundary.
- **FR-013**: Published sample Agent Cards MUST explicitly declare managed
  HTTP Bearer authentication; a newly trusted managed release MUST NOT use an
  anonymous or provider-key fallback.
- **FR-014**: Because managed Router dispatch no longer accepts the previous
  anonymous `none` execution semantics, the Router Internal dispatch contract
  MUST use a new major version with an explicit migration boundary. The
  unchanged Router metadata-read surface may remain on its existing version;
  no runtime may serve the retired dispatch route or silently dual-read it.

### Key Entities

- **Router Invocation Credential**: A signed, transient authorization for one
  outbound A2A HTTP request and one exact resolved invocation context.
- **Credential Signing Identity**: The Router-held private key, Agent-held
  public key, exact issuer, exact key ID, and credential profile version.
- **Replay Entry**: An Agent-local association of one accepted token ID with
  its expiration instant; it is neither a platform record nor Ledger data.
- **Authenticated Agent Context**: The verified Workspace, target release,
  capability, invocation, task, and trace values made available to the Agent
  adapter after successful verification.

### Runtime/Platform Boundary

- **Platform-owned behavior**: Router credential issuance, claim derivation,
  context propagation, failure classification, and Ledger secrecy are Data
  Plane responsibilities. The credential schema is a platform contract.
- **Runtime-owned behavior**: Agent model, tool, workflow, task execution, and
  business errors remain inside each Agent Runtime. Runtime logic receives an
  already authenticated adapter context and never validates platform storage.
- **Cross-runtime proof**: Runtime A and Runtime B use different runtime
  implementations but enforce the same adapter-level credential contract;
  their nested call still traverses the Router twice and yields one lineage.

## Success Criteria

### Measurable Outcomes

- **SC-001**: 100% of managed sample-Agent A2A execution requests, across
  JSON, SSE, nested, and cancellation paths, carry a unique short-lived
  credential and are accepted only once.
- **SC-002**: Every required negative case—direct unauthenticated, forged,
  expired, wrong issuer, wrong audience, replayed, and each bound-context
  mismatch—is rejected before Agent runtime execution with the specified
  401/403 distinction.
- **SC-003**: The clean acceptance flow still completes `Register -> Verify ->
  Publish -> Install -> Invoke -> Record`, including a two-runtime nested call
  whose parent and child share one root task and trace.
- **SC-004**: Automated secrecy inspection finds zero credential, signing key,
  signature, or token-ID values in logs, Card payloads, public errors, Ledger
  events, Ledger projections, and persisted platform rows.
- **SC-005**: Missing or invalid signing/verifying configuration prevents the
  owning Router or Agent process from serving execution traffic in 100% of
  configuration test cases; no default credential path is used.

## Assumptions

- Issue #48 endpoint ownership and Issue #49 immutable published releases are
  already active prerequisites; the Router receives an exact validated
  release ID/Card digest pair before transport.
- The first profile uses one active asymmetric signing key and exact key ID.
  Multi-key rotation and remote key discovery require a later policy/spec.
- Deployment clocks are synchronized. This slice deliberately defines no
  implicit clock-skew allowance because none exists in the current policy.
- Agent-local replay state covers the credential's bounded lifetime. Durable
  replay coordination across Agent replicas or restarts is deferred until a
  deployment topology and availability policy exists.

## Non-Goals

- Provider-managed bearer tokens, API-key forwarding, OAuth exchange, mTLS,
  remote JWKS discovery, multi-key rotation, or end-user identity federation.
- Durable or cross-replica replay storage, network retries, alternate Agent
  endpoints, anonymous execution, or compatibility dual-read behavior.
- Agent deployment, autoscaling, model/tool/workflow/session behavior,
  billing, quota, certification, or Marketplace review.
- Persisting a credential, token ID, public key, private key, signature, or
  replay entry in Registry, Workspace, or Ledger storage.

## Clarifications

### Session 2026-07-22

- Q: Is Router-to-Agent authentication optional for trusted managed calls? -> A: No; every published sample-Agent execution route requires it.
- Q: Which context must the credential bind? -> A: Exact Workspace, Agent/version, release/digest, capability, invocation, root/parent task, and trace.
- Q: How are invalid credentials classified? -> A: Authentication failures are 401; valid-recipient/context authorization mismatches are 403.
- Q: Is credential replay state persisted? -> A: No; it is Agent-local and bounded by the short credential lifetime in this slice.
