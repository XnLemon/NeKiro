# Feature Specification: Trusted Publication Acceptance and Operations

**Feature Branch**: `codex/trusted-publication-acceptance`

**Created**: 2026-07-24

**Status**: Complete and merged through PR #57 after all required checks passed
in CI run `30074754169`

**Input**: User description: "Continue Issue #52: prove the trusted publication flow and negative paths in a fresh environment, document provider/operator recovery, review, and merge."

## User Scenarios & Testing

### User Story 1 - Prove a trusted Agent is usable (Priority: P1)

As a provider and Workspace owner, I can start from an empty platform, prove
control of two independently implemented Agent endpoints, publish immutable
releases, install them, invoke them through the managed path, and inspect one
complete root-and-child invocation lineage.

**Why this priority**: This is the smallest user-visible proof that NeKiro adds
trusted publication, Workspace authorization, managed routing, and auditable
cross-runtime execution around A2A.

**Independent Test**: Start a fresh platform environment and complete
Register -> Verify -> Publish -> Discover -> Install -> Invoke -> Record for
the two sample Agents. The test passes only when the result and linked Ledger
lineage identify the exact published releases.

**Acceptance Scenarios**:

1. **Given** an empty platform and two reachable Agent endpoints implemented by different Runtimes, **When** the provider registers, verifies, and publishes both versions and the Workspace owner installs and invokes them, **Then** the managed root call and nested call succeed only through Gateway and Router.
2. **Given** the completed nested call, **When** the Workspace owner queries its Trace and Invocation details, **Then** parent/child Invocation IDs, one root Task ID, one Trace ID, Workspace, target Agents, exact Card versions, Release IDs, and Card digests are queryable.
3. **Given** a Release ID read from an Invocation, **When** the authorized caller inspects that Release, **Then** the bound provider, Card version, endpoint binding, publication state, and trust method are queryable without any proof or credential secret.

---

### User Story 2 - Prove trust boundaries reject unsafe calls (Priority: P1)

As a security reviewer, I can run one deterministic negative matrix that proves
publication, Workspace authorization, Release lifecycle, Router-to-Agent
authentication, and endpoint availability failures remain distinguishable and
never silently execute an Agent.

**Why this priority**: A positive flow alone cannot prove that the publication
and managed-invocation boundaries are trustworthy.

**Independent Test**: Run the negative matrix in a fresh environment and verify
the exact stable error category and side-effect boundary for every case.

**Acceptance Scenarios**:

1. **Given** endpoint ownership challenges with a wrong proof, an expired or reused challenge, a disallowed destination, or an unavailable endpoint, **When** verification is completed, **Then** each case returns its distinct trusted-publication error and exposes only safe failure metadata.
2. **Given** an unpublished, suspended, or revoked Release or a disabled Installation, **When** installation or invocation is attempted, **Then** the corresponding stable Release or Installation error is returned before Agent execution.
3. **Given** a forged, expired, or wrong-audience Router credential, **When** it is presented directly to an Agent, **Then** the Agent rejects it with the contract-defined authentication or authorization outcome.
4. **Given** no Router credential, **When** a caller accesses an Agent endpoint directly, **Then** the request is rejected and the Agent operation does not execute.
5. **Given** an accepted managed Invocation whose Agent endpoint is unavailable, **When** routing fails, **Then** the correlated failure and exact Release provenance are queryable in the Ledger.
6. **Given** an accepted streaming Invocation, **When** caller cancellation races with a stream-chunk or terminal Ledger write, **Then** the Router commits exactly one `canceled` terminal fact and does not leave the Invocation `running` or report a dependency failure for caller cancellation.

---

### User Story 3 - Diagnose and recover publication safely (Priority: P2)

As a provider or platform operator, I can use one runbook to identify the
failed boundary, distinguish automatic state recording from manual action, and
perform only the recovery allowed by the current contracts.

**Why this priority**: Trust state is operationally useful only when its owner
and next action are unambiguous.

**Independent Test**: Follow the runbook from each verified failure category to
its documented inspection and recovery checkpoint without relying on hidden
database writes, retry, alternate endpoints, or secret disclosure.

**Acceptance Scenarios**:

1. **Given** a verification failure, **When** the provider inspects the Endpoint Binding, **Then** the safe failure category, timestamps, responsible actor, and permitted next action are documented.
2. **Given** a disabled Installation, suspended Release, or revoked Release, **When** the owner/operator follows the state table, **Then** the runbook distinguishes re-enabling an Installation from replacing an immutable Release with a new verified version.
3. **Given** endpoint or credential configuration failure, **When** recovery begins, **Then** the runbook identifies which component owner must correct the endpoint or signing/verifier configuration and requires a fresh explicit invocation rather than automatic retry.

### Edge Cases

- Requests rejected before the Router-owned `created` event are not accepted
  Invocations and therefore have no Ledger record; their Gateway Trace and
  typed pre-correlation error remain the evidence.
- Direct Agent requests and invalid Router credentials are outside the managed
  invocation boundary and therefore never create Ledger facts.
- A suspended or revoked Release cannot be restored in place. Recovery uses a
  new Agent Card version, Endpoint Binding, and Release.
- A failed, expired, or consumed ownership challenge is never repaired or
  reused. Recovery creates a fresh challenge after the underlying cause is
  corrected.
- Phase 1 has no disabled Workspace lifecycle. The supported control is a
  disabled Workspace Installation, and acceptance uses that canonical term.
- Historical legacy/unverified versions are not evidence for trusted
  publication and are excluded from the positive acceptance path.
- Caller cancellation can arrive while a stream event or terminal fact is
  being committed. An uncommitted stream event is not promoted to a Ledger
  fact; the Router uses its existing bounded terminal-commit policy to record
  the local cancellation winner once.

## Requirements

### Functional Requirements

- **FR-001**: The acceptance suite MUST start from a fresh platform and
  persistent store and complete Register -> Verify -> Publish -> Discover ->
  Install -> Invoke -> Record without pre-seeded trusted Release facts.
- **FR-002**: The positive flow MUST use at least two sample Agents implemented
  by different Runtimes and MUST include one Router-mediated nested call.
- **FR-003**: Each trusted Installation and accepted Invocation MUST reference
  the exact published Release ID and Card digest selected for that Agent
  version.
- **FR-004**: Trace and Invocation inspection MUST prove parent/child
  Invocation IDs, root Task ID, Trace ID, Workspace, target Agent, exact Card
  version, Release ID, and Card digest consistency.
- **FR-005**: Release inspection linked from Invocation metadata MUST expose the
  provider, binding, publication state, timestamps, and verification method.
- **FR-006**: The acceptance suite MUST distinguish wrong proof, expired
  challenge, reused challenge, disallowed destination, and verification
  endpoint unavailable outcomes.
- **FR-007**: The acceptance suite MUST distinguish unpublished, suspended,
  and revoked Release outcomes and a disabled Installation outcome.
- **FR-008**: The acceptance suite MUST distinguish forged, expired, and
  wrong-audience Router credentials and MUST verify the contract-defined
  authentication versus authorization response.
- **FR-009**: The acceptance suite MUST prove that a direct unauthenticated
  Agent request is rejected before Agent execution.
- **FR-010**: An accepted managed Invocation to an unavailable Agent endpoint
  MUST produce a correlated terminal Ledger failure with exact Release
  provenance.
- **FR-011**: Pre-acceptance policy rejection, direct Agent access, and invalid
  Agent credential cases MUST NOT be represented as successful or accepted
  Ledger Invocations.
- **FR-012**: Except for the one-time authenticated challenge-issuance response
  that is contractually required to deliver its proof to the provider, all
  later public responses, persisted Cards, Bindings, Releases, Installations,
  Ledger facts, and process logs MUST be checked for challenge proofs, bearer
  tokens, signed credentials, signatures, and signing material.
- **FR-013**: The repository CI MUST execute the fresh-environment acceptance
  suite and always capture service logs and remove persistent test state.
- **FR-014**: The provider/operator runbook MUST map every in-scope state or
  error category to its inspection surface, responsible owner, manual next
  action, and recovery verification.
- **FR-015**: The runbook MUST explicitly distinguish automatically recorded
  state from actions requiring the provider, Workspace owner, or platform
  operator.
- **FR-016**: Recovery MUST NOT introduce automatic retry, alternate endpoint,
  credential fallback, old-contract compatibility, or in-place mutation of
  immutable Release facts.
- **FR-017**: The feature MUST reuse the active Trusted Publication,
  Installation, Invocation, Router credential, Result, Error, and Ledger
  contracts without adding a new public API or persistence owner.
- **FR-018**: If caller cancellation or deadline expiry races with a streaming
  chunk or terminal Ledger append, the Router MUST make one bounded terminal
  commit independent of the canceled caller context, preserve contiguous SSE
  sequencing for any writable response, and MUST NOT retry the chunk, emit
  `DEPENDENCY_ERROR` for the local cancellation, or leave the accepted
  Invocation permanently non-terminal.

### Key Entities

- **Acceptance Run**: One isolated execution that owns its fresh environment,
  positive flow, negative matrix, secrecy checks, and cleanup result.
- **Trusted Publication Evidence**: Safe Endpoint Binding and Agent Release
  fields that prove verification method, state, immutable identity, and
  timestamps without retaining the raw proof.
- **Invocation Provenance Link**: The Release ID and Card digest carried by an
  Installation, accepted Invocation, its Ledger events, and its Release query.
- **Recovery Action**: A documented manual action owned by a provider,
  Workspace owner, or operator; it never changes an immutable fact in place.

### Runtime/Platform Boundary

- **Platform-owned behavior**: Provider/binding/release state, Workspace
  Installation authorization, Gateway/Router routing, signed invocation
  context, lineage, Ledger facts, and operator inspection.
- **Runtime-owned behavior**: Agent business execution and response content;
  the sample Runtimes only expose the agreed A2A and Router-authentication
  adapters.
- **Cross-runtime proof**: Runtime A calls Runtime B through the Router, and
  both Invocations appear in one Release-bound Ledger lineage without sharing
  Runtime-internal types or storage.

## Success Criteria

### Measurable Outcomes

- **SC-001**: One fresh acceptance run completes all six positive lifecycle
  stages and one cross-runtime nested invocation with zero direct managed calls
  to an Agent endpoint.
- **SC-002**: 100% of in-scope negative cases return the expected distinct
  error category, and zero rejected direct or invalid-credential calls execute
  Agent business behavior.
- **SC-003**: 100% of accepted trusted Invocations inspected by the suite have
  matching Release ID and Card digest across projection and every event.
- **SC-004**: One linked query from each accepted Invocation to its Release
  returns the expected trust method and published immutable identity.
- **SC-005**: Outside the one-time authenticated challenge-issuance response,
  zero challenge proofs, bearer credentials, signed Router credentials,
  private/public key material, or Agent input/output fixture secrets appear in
  the checked metadata, persistence, errors, or logs.
- **SC-006**: The runbook covers 100% of the acceptance failure categories and
  identifies exactly one primary recovery owner and an observable completion
  check for each.
- **SC-007**: Every pull request executes the clean acceptance job and tears
  down its persistent volumes even when the suite fails.
- **SC-008**: A repeated real cancellation probe and focused Router regression
  tests produce zero `running` or `DEPENDENCY_ERROR` outcomes for local caller
  cancellation.

## Assumptions

- Existing active contracts and ADRs define all required states, errors,
  acceptance boundaries, and ownership; this feature adds executable evidence
  and operating guidance, plus the smallest correction required where the
  acceptance exposed an implementation race against ADR 0006.
- Development-static Gateway identity and deterministic test-only signing
  material remain limited to the isolated acceptance environment.
- The current `http_well_known` verification method is the only Phase 1 trust
  method; additional attestation methods remain out of scope.
- Evidence retention duration, approval workflow, key rotation, and
  cross-replica replay policy remain deferred and are labelled as requiring a
  separate policy rather than being guessed in this runbook.

## Non-Goals

- Adding deployment, autoscaling, billing, certification, marketplace review,
  enterprise RBAC/OIDC, or a new Agent Runtime feature.
- Adding Workspace disablement, Release unsuspension, automatic rollback,
  automatic retry, credential rotation, or alternate verification methods.
- Persisting Agent request/result content, raw challenge proofs, signed Router
  credentials, or secret-bearing diagnostics.
- Changing active public API versions, Release state transitions, Ledger
  acceptance semantics, or data ownership boundaries.
