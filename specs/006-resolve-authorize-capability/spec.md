# Feature Specification: Resolve and Authorize an Installed Agent Capability

**Feature Branch**: `codex/issue-6-resolution`
**Created**: 2026-07-15
**Status**: Approved
**Input**: GitHub issue #6, dependent on the active Workspace, Installation, Catalog, and Control Plane Internal v2 contracts.

## Clarifications

### Session 2026-07-15

- The resolver first proves that the requested Workspace exists, then reads the
  current Workspace-owned Installation, then re-reads the exact Catalog version,
  and finally evaluates the requested capability and permissions. A missing
  Workspace is `NOT_FOUND`; a missing current Installation or exact pin mismatch
  is `AGENT_NOT_INSTALLED`.
- The trusted internal service identity is authorized at the internal HTTP
  boundary. It is not treated as a Workspace owner and does not receive a
  caller-supplied owner override.
- A Catalog exact-version read that fails because the required Card cannot be
  read is `DEPENDENCY_ERROR`; a readable non-published exact version is
  `AGENT_DISABLED`. The Installation row is never changed by resolution.

## User Scenarios & Testing

### User Story 1 - Resolve an Authorized Capability (Priority: P1)

An A2A Router asks the trusted Control Plane internal operation for an exact
installed Agent capability. The platform returns the exact currently published
Card and the enabled Installation authorization facts only when all requested
identity, installation, publication, capability, and permission checks pass.

**Why this priority**: This is the pre-dispatch trust decision in the
`Install -> Invoke` path and keeps the Router outside Control Plane storage.

**Independent Test**: Seed one Workspace, enabled exact Installation, published
Card, and authorized capability; call the internal operation and verify the
exact Card and minimal Installation response.

**Acceptance Scenarios**:

1. **Given** a trusted internal caller and an enabled exact Installation whose
   accepted permissions contain every permission required by an existing Card
   capability, **when** the exact request is resolved, **then** the response
   contains the exact Card and enabled Installation facts.
2. **Given** an Installation whose pinned version differs from the request,
   **when** resolution is requested, **then** the response is
   `404 AGENT_NOT_INSTALLED` and Catalog is not consulted.
3. **Given** a disabled Installation, **when** resolution is requested, **then**
   the response is `403 INSTALLATION_DISABLED` and Catalog is not consulted.
4. **Given** an enabled Installation and a Catalog exact version that is not
   published, **when** resolution is requested, **then** the response is
   `403 AGENT_DISABLED` and the Installation remains unchanged.
5. **Given** a missing capability or a capability requiring an unaccepted
   permission, **when** resolution is requested, **then** the response is
   `403 CAPABILITY_NOT_ALLOWED` and no Card is returned.

### User Story 2 - Preserve Correlation and Failure Semantics (Priority: P1)

The Router receives a safe, contract-defined failure for every resolution
outcome and can associate each post-validation failure with the original
Invocation, root Task, and Trace.

**Why this priority**: Correlation is required to build a trustworthy parent /
child invocation lineage and must not be repaired with synthesized IDs.

**Independent Test**: Exercise validation, authentication, not-found,
installation, disabled, capability, and dependency failures over HTTP and
compare each status, fixed message, response shape, and correlation value.

**Acceptance Scenarios**:

1. **Given** valid invocation, root Task, and Trace identifiers, **when** any
   post-correlation resolution failure occurs, **then** the error repeats the
   exact three request values and the trace response header repeats the request
   Trace.
2. **Given** malformed or missing correlation, **when** the request is rejected,
   **then** the pre-correlation error contains only fixed `code`, `message`, and
   a generated safe `traceId`, without echoing malformed IDs.
3. **Given** a northbound credential or absent internal credential, **when** the
   internal route is called, **then** it returns `401 UNAUTHENTICATED` and does
   not invoke the resolver.
4. **Given** a missing Workspace, **when** resolution is requested, **then** it
   returns correlated `404 NOT_FOUND`; a missing current Installation remains
   correlated `404 AGENT_NOT_INSTALLED`.
5. **Given** a Catalog or Workspace store dependency failure, **when** resolution
   is requested, **then** it returns correlated `503 DEPENDENCY_ERROR` and no
   stale, empty, or synthetic success.

### User Story 3 - Retain a Router-Safe Response Boundary (Priority: P2)

The Router receives only the exact Card contract and enabled Installation
authorization facts needed for routing. Resolution does not expose credentials,
Agent input or output, health, deployment, or internal storage details.

**Why this priority**: The response crosses the Control Plane/Data Plane
boundary and must remain runtime-neutral and secret-safe.

**Independent Test**: Validate the response against the active Control Plane
Internal v2 contract, inspect its field set, and reconstruct the service from
PostgreSQL before resolving the same exact pin.

**Acceptance Scenarios**:

1. **Given** a successful resolution, **when** the response is encoded, **then**
   it contains only `card` and the approved resolved Installation fields and
   contains no credentials, result/input payloads, health data, or storage
   error details.
2. **Given** a committed enabled Installation and a process/store
   reconstruction, **when** the same request is resolved, **then** the exact
   pin and permission snapshot are unchanged.
3. **Given** Catalog disables the pinned version after installation, **when**
   the exact request is resolved, **then** it returns `AGENT_DISABLED` without
   rewriting the Installation.

## Edge Cases

- Authentication is checked before request validation. Missing or untrusted
  internal identity therefore receives generated-trace `UNAUTHENTICATED`.
- Strict correlation is validated before other request fields. Invalid
  correlation receives the pre-correlation shape; valid correlation with an
  invalid version, capability, or identifier receives a correlated validation
  error.
- Exact IDs are case-sensitive and are not trimmed or replaced.
- A current Installation is the enabled or disabled Workspace fact; an
  uninstalled historical record cannot resolve.
- A Catalog read failure is never converted to `AGENT_DISABLED`, not found, an
  empty Card, or success. Only an explicitly readable non-published Card is
  disabled.
- Capability authorization requires existence and complete set containment;
  an empty accepted snapshot only authorizes a capability requiring no
  permissions.

## Requirements

### Functional Requirements

- **FR-001**: The Control Plane MUST expose the active `POST /internal/v2/resolve-agent` operation as the only Router-facing exact-resolution path.
- **FR-002**: The operation MUST require a separately configured trusted internal Bearer identity; northbound identities MUST NOT authorize it implicitly.
- **FR-003**: The operation MUST validate authentication, request shape, and strict correlation in the contract-defined precedence.
- **FR-004**: A post-correlation error MUST preserve the exact request `invocationId`, `rootTaskId`, and `traceId`; no replacement or synthesized correlated ID is allowed.
- **FR-005**: A pre-correlation error MUST contain only fixed `code`, fixed `message`, and a generated safe `traceId`; malformed or missing request IDs MUST NOT be echoed.
- **FR-006**: Resolution MUST first establish that the exact Workspace exists, then read only the current non-uninstalled Installation for the requested Agent.
- **FR-007**: A missing Workspace MUST map to `NOT_FOUND`, while no current Installation or an exact pin mismatch MUST map to `AGENT_NOT_INSTALLED`.
- **FR-008**: Resolution MUST reject a disabled Installation as `INSTALLATION_DISABLED` before reading Catalog state.
- **FR-009**: Resolution MUST re-read the exact requested Card through the Catalog-owned boundary and MUST accept only its current published state.
- **FR-010**: A readable non-published exact Card MUST map to `AGENT_DISABLED`; a Catalog read/dependency failure MUST map to `DEPENDENCY_ERROR`.
- **FR-011**: Resolution MUST authorize only an existing requested capability for which every required permission is present in the accepted Installation snapshot.
- **FR-012**: Missing capability and incomplete accepted permissions MUST map to `CAPABILITY_NOT_ALLOWED` without returning a Card.
- **FR-013**: Success MUST return the exact Card and only enabled resolved Installation facts: installation ID, Workspace ID, Agent ID, installed version, accepted permissions, and enabled status.
- **FR-014**: Resolution MUST use Workspace Store and CatalogReader ports only; it MUST NOT access Catalog or Workspace tables directly, invoke an Agent, probe an endpoint, retry, cache, or use an alternate source.
- **FR-015**: Resolution MUST not rewrite an Installation when Catalog later disables its pinned version, and exact pins/permission snapshots MUST survive process reconstruction.
- **FR-016**: Contract, unit, PostgreSQL, internal HTTP, auth, correlation, authorization, restart, disablement, and dependency tests MUST map to these requirements and active contract messages.

## Key Entities

- **Workspace**: The existing logical authorization root whose existence is
  established before exact resolution.
- **Installation**: The Workspace-owned current pin and accepted permission
  snapshot; only enabled, non-uninstalled facts can resolve.
- **Agent Card**: The Catalog-owned exact versioned metadata returned only when
  currently published.
- **Exact Resolution**: A transient authorization result combining one exact
  current Card and one enabled resolved Installation.

## Runtime/Platform Boundary

- **Platform-owned behavior**: Control Plane exact resolution, Workspace
  authorization facts, Catalog publication-state reads, error/correlation
  mapping, and the internal HTTP boundary.
- **Runtime-owned behavior**: Agent execution, endpoint health, credentials,
  model/tool/workflow behavior, and result payloads.
- **Cross-runtime proof**: The same internal v2 resolution result is consumed by
  any supported A2A Router and does not depend on a Runtime framework. This
  issue proves the boundary; sample Agents and Router execution are later work.

## Success Criteria

### Measurable Outcomes

- **SC-001**: Every acceptance scenario has a focused unit, contract, HTTP, or PostgreSQL test with the exact contract status/code/message asserted.
- **SC-002**: 100% of valid post-correlation failures repeat the three exact request correlation values, and 100% of pre-correlation failures omit malformed or missing IDs.
- **SC-003**: 100% of missing Workspace, missing Installation, pin mismatch, Installation-disabled, Catalog-disabled, capability-denied, and dependency cases remain distinct.
- **SC-004**: A successful response validates against the active Control Plane Internal v2 response schema and contains no fields outside the approved Card and resolved Installation contracts.
- **SC-005**: A PostgreSQL reconstruction returns the same exact Installation pin and permission snapshot, and Catalog disablement changes no Installation fields.
- **SC-006**: The fallback delta is zero additions: no retry, cache, alternate source, default ID, endpoint probe, stale Card, empty-success substitution, or compatibility route is introduced.

## Assumptions

- Issue #3/4/5 implementations and active CatalogReader, Workspace Store,
  Agent Card, Installation v2, Platform Error v3, and Control Plane Internal v2
  contracts are the source of truth.
- The internal Bearer authenticator is configured explicitly by the process;
  no token, principal, Workspace, version, or capability default exists.
- PostgreSQL integration tests are run only when `NEKIRO_TEST_DATABASE_URL`
  names a dedicated database ending in `_test`.

## Non-Goals

- Router implementation, A2A transport, invocation dispatch, Ledger writes,
  sample Agents, endpoint probing, health checks, deployment, credentials, or
  Agent execution.
- Workspace membership/RBAC, automatic upgrades, lifecycle changes, caching,
  retries, queues, historical compatibility routes, or alternate Catalog data.
