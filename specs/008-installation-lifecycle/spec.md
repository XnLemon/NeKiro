# Feature Specification: Enable, Disable, and Uninstall Agent Access

**Feature Branch**: `codex/issue-8-installation-lifecycle`

**Created**: 2026-07-15

**Status**: Implemented and verified

**Input**: GitHub Issue #8, "[Installation] Enable, disable, and uninstall Agent access".

## Clarifications

### Session 2026-07-15

- Lifecycle mutations are owner-only Workspace operations. The existing active
  Northbound v3 PATCH and DELETE operations are reused without a contract
  version change.
- The approved transition graph is `enabled <-> disabled -> uninstalled`.
  Same-state requests, enabled-to-uninstalled requests, and all transitions
  from `uninstalled` return `CONFLICT`; repeated uninstall follows the same
  conflict policy.
- Uninstall is terminal and preserves the row, immutable pin, accepted
  permission snapshot, and lifecycle timestamps. Reinstall uses a new row and
  identity after the old row becomes uninstalled.
- PostgreSQL is the lifecycle fact owner. A transition locks the targeted row,
  updates state and timestamps atomically, and returns the committed row. A
  partial unique constraint continues to cover only non-uninstalled rows.
- Catalog publication state is not read or changed by enable, disable, or
  uninstall. No reconciliation, deployment, probing, retry, cache, or
  alternate source is introduced.

## User Scenarios & Testing

### User Story 1 - Manage Current Access (Priority: P1)

As a Workspace owner, I can temporarily disable a current Agent Installation,
re-enable a disabled Installation, and explicitly disable it before terminal
uninstall so that access changes are deliberate and auditable.

**Why this priority**: Installation state is the Workspace authorization fact
used by later invocation dispatch.

**Independent Test**: Create an enabled Installation, exercise every legal and
illegal transition, and verify state, timestamps, immutable fields, and exact
error outcomes.

**Acceptance Scenarios**:

1. **Given** an enabled Installation, **when** its owner requests disabled,
   **then** the response and durable row are disabled and `updatedAt` advances.
2. **Given** a disabled Installation, **when** its owner requests enabled,
   **then** the response and durable row are enabled and the exact pin and
   permission snapshot are unchanged.
3. **Given** a disabled Installation, **when** its owner uninstalls it,
   **then** the response is uninstalled, `uninstalledAt == updatedAt`, and the
   complete historical row remains readable.
4. **Given** an enabled Installation, **when** its owner uninstalls it,
   **then** the operation returns `409 CONFLICT` and the row remains enabled.
5. **Given** an Installation already in the requested state or already
   uninstalled, **when** the same mutation is repeated, **then** it returns
   `409 CONFLICT` without a false success or timestamp change.

### User Story 2 - Enforce Workspace Ownership and Error Boundaries (Priority: P1)

As the platform, I must allow only the immutable Workspace owner to mutate an
Installation and preserve the distinction between invalid input, unauthenticated
requests, forbidden ownership, wrong Workspace/Installation, conflict, and
storage dependency failures.

**Independent Test**: Send PATCH and DELETE requests with each authentication,
path, body, ownership, identity, transition, and persistence failure and verify
the active v3 status/code and Trace header.

**Acceptance Scenarios**:

1. **Given** a non-owner or missing authentication, **when** a lifecycle
   mutation is requested, **then** it returns `403 FORBIDDEN` or
   `401 UNAUTHENTICATED` respectively and does not mutate the row.
2. **Given** an unknown Workspace, unknown Installation, or an Installation
   belonging to another Workspace, **when** a mutation is requested, **then**
   it returns `404 NOT_FOUND` without creating or moving a row.
3. **Given** an invalid target status, malformed body, or unsafe path identity,
   **when** PATCH is requested, **then** it returns `400 VALIDATION_ERROR`
   before persistence.
4. **Given** a Workspace store failure during authorization or transition,
   **when** a mutation is requested, **then** it returns `503 DEPENDENCY_ERROR`
   and no degraded result.

### User Story 3 - Serialize Lifecycle History (Priority: P1)

As the platform, I need concurrent lifecycle requests to serialize on one
Installation fact so that no request skips a legal transition, resurrects a
terminal row, releases the uniqueness slot early, or reports success for an
uncommitted write.

**Independent Test**: Race enable/disable/uninstall and install requests against
the same Workspace, inspect the committed rows after completion and after a
new service/store instance reads them, and verify every successful history is a
legal sequence with at most one current Installation.

**Acceptance Scenarios**:

1. **Given** concurrent state mutations, **when** they complete, **then** row
   locking produces one serialized legal outcome and conflict losers do not
   change timestamps or immutable fields.
2. **Given** concurrent uninstall and reinstall, **when** both complete,
   **then** the old row is terminal and preserved, the new row has a distinct
   identity, and at most one current row exists.
3. **Given** a Catalog version is disabled independently, **when** lifecycle
   operations run, **then** Workspace state is unchanged by Catalog and no
   retry or reconciliation is attempted.

## Edge Cases

- Status values are exact, case-sensitive identifiers; `uninstalled` is not a
  valid PATCH target and is available only through DELETE.
- A repeated DELETE is a conflict, not idempotent success.
- A wrong Workspace/Installation pair is not found within the requested
  authorization root.
- An uninstalled row has `uninstalledAt` equal to its terminal `updatedAt`, and
  enabled/disabled rows have no `uninstalledAt`.
- Lifecycle responses must contain committed timestamps and preserve the exact
  version constraint, installed version, accepted permissions, Workspace ID,
  Agent ID, and Installation ID.
- A Catalog disable does not mutate the Workspace Installation and does not
  make lifecycle requests consult another Card or data source.

## Functional Requirements

- **FR-001**: Only the authenticated Workspace owner MUST be authorized to
  enable, disable, or uninstall an Installation.
- **FR-002**: The platform MUST allow only `enabled -> disabled`,
  `disabled -> enabled`, and `disabled -> uninstalled` transitions.
- **FR-003**: Every successful non-terminal transition MUST update `updatedAt`
  and return the committed row; lifecycle mutations MUST NOT change immutable
  pin or permission fields.
- **FR-004**: Uninstall MUST be terminal, preserve the row and immutable facts,
  set `uninstalledAt`, set `updatedAt` to the same committed terminal time,
  and release the current uniqueness slot in that same transaction.
- **FR-005**: Same-state mutations, enabled-to-uninstalled requests, and all
  mutations from uninstalled MUST return `CONFLICT` without a state change.
- **FR-006**: Reinstall after uninstall MUST create a new enabled Installation
  identity and MUST leave the historical row unchanged.
- **FR-007**: Unknown Workspace/Installation, wrong Workspace/Installation,
  invalid input, unauthenticated identity, forbidden ownership, conflict, and
  dependency failure MUST remain distinct outcomes according to active
  Northbound v3 error mappings.
- **FR-008**: Concurrent lifecycle and install requests MUST serialize on
  Workspace-owned facts, produce only legal committed histories, and never
  report success for a losing or uncommitted transition.
- **FR-009**: Lifecycle operations MUST NOT mutate or reconcile Catalog state,
  probe endpoints, deploy Agents, retry dependencies, use caches, or add an
  alternate source.
- **FR-010**: The schema MUST preserve terminal rows and enforce state,
  timestamp, foreign-key, and partial-current uniqueness invariants.
- **FR-011**: A successful lifecycle transition MUST use a committed timestamp
  that is strictly later than the locked row's prior `updatedAt`; if the
  operation's candidate time is stale under lock contention, the Workspace
  store MUST normalize both values to PostgreSQL microsecond precision and
  advance it by the smallest representable timestamp unit rather than writing
  a regressed or equal timestamp.

## Success Criteria

- **SC-001**: Every legal transition and every illegal transition in the
  approved graph has a passing mapped unit and HTTP test.
- **SC-002**: PostgreSQL tests demonstrate that 100 concurrent lifecycle and
  install operations leave at most one current row and no illegal state or
  timestamp relationship.
- **SC-003**: A terminal Installation remains byte-for-byte equivalent in its
  immutable facts after a fresh store/service reconstruction, and reinstalling
  creates a distinct identity.
- **SC-004**: All acceptance error classes map to their documented status/code,
  include a safe Trace header, and contain no secret or dependency detail.
- **SC-005**: No lifecycle execution invokes Catalog or adds fallback behavior;
  the fallback delta is `removed 0, retained 1, added 0, net 0`, where the one
  retained empty result is the legitimate empty Installation list behavior
  defined by the existing Workspace contract.

## Key Entities

- **Installation**: Workspace-owned durable authorization fact with immutable
  pin and permission snapshot, mutable status, update timestamp, and optional
  terminal timestamp.
- **Workspace**: Immutable owner boundary used to authorize lifecycle changes.
- **Lifecycle Transition**: One committed state change with a legal source and
  target, committed timestamp, and returned Installation fact.

## Assumptions

- Active Northbound v3, Installation v2, and Platform Error v3 contracts are
  unchanged because they already declare PATCH/DELETE lifecycle behavior.
- Full RBAC, Deployment, Router, Ledger, Catalog reconciliation, and frontend
  work remain out of scope.
- The existing server clock and store interfaces remain the boundary for
  committed timestamps; tests must verify returned values against restart reads.

## Out of Scope

- Catalog publication lifecycle changes.
- Invocation dispatch, Router, Ledger, deployment, retry, cache, or endpoint
  health behavior.
- Workspace deletion, purge, or historical Installation compaction.
