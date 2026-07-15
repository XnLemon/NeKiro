# Feature Specification: Prove Workspace Trust, Durability, and Concurrency

**Feature Branch**: `codex/issue-9-acceptance`

**Created**: 2026-07-15

**Status**: Implemented and verified

**Input**: GitHub Issue #9, "[Acceptance] Prove Workspace trust, durability, and concurrency".

## Context

Issues #6, #7, and #8 deliver the Workspace and Installation runtime slices.
This feature closes their parent acceptance gate with independent evidence that
the slices work together across the real Control Plane boundaries and durable
storage. It does not introduce a new product capability.

The acceptance evidence must prove the complete Workspace flow:

```text
Create Workspace -> Discover -> Install -> Inspect -> Disable/Enable
-> Resolve -> Uninstall
```

The evidence must preserve the project boundary: Catalog owns Agent Card facts,
Workspace owns Workspace and Installation facts, Gateway is the public entry
point, and internal resolution remains a separately authenticated Control
Plane boundary. No Agent endpoint is invoked.

## Clarifications

### Session 2026-07-15

- This feature is evidence-only. Existing active contracts and runtime behavior
  are reused; a failing acceptance test may expose a defect, but the test suite
  must not invent a new fallback or silently weaken an existing assertion.
- The main acceptance workflow uses the real Catalog and Workspace PostgreSQL
  stores, services, and Gateway handlers. Test doubles remain allowed only for
  isolated failure-injection tests where a real dependency cannot be failed
  deterministically.
- A dedicated PostgreSQL database whose name ends in `_test` is required for
  integration execution. Missing or unsafe database configuration is reported
  as not-run evidence, never as a passing result.
- The acceptance suite does not start an Agent process or make an outbound
  Agent call. Resolution is verified up to the exact resolved Installation and
  published Agent Card facts.
- The next unfinished platform scope after this gate is Invocation Dispatch
  and the A2A Router; this feature does not claim those components exist.

## User Scenarios & Testing

### User Story 1 - Verify the Complete Workspace Flow (Priority: P1)

As a platform maintainer, I need one reproducible acceptance workflow that
proves a published Agent can be discovered, installed into an owned Workspace,
inspected, lifecycle-managed, and exactly resolved.

**Why this priority**: This is the approved Minimal Workspace and Installation
closure and is the prerequisite for later invocation work.

**Independent Test**: Run the acceptance workflow against a fresh dedicated
database and verify each public or internal response, durable fact, status, and
correlation identifier.

**Acceptance Scenarios**:

1. **Given** a published Agent Card with a capability, **when** the platform
   searches by that capability, **then** the published Agent version is
   discoverable through the public Catalog boundary.
2. **Given** an authenticated Workspace owner, **when** the owner creates a
   Workspace and installs the matching Agent, **then** the Installation pins
   the selected exact version and canonical accepted permissions.
3. **Given** an enabled Installation, **when** the owner lists and reads its
   history, **then** the response contains the committed durable Installation
   fact and its current status.
4. **Given** a current Installation, **when** the owner disables, enables, and
   then disables it before uninstalling, **then** each legal transition is
   visible and the immutable pin and permission snapshot remain unchanged.
5. **Given** an enabled, published, capability-authorized Installation,
   **when** the internal resolver receives valid correlation identifiers,
   **then** it returns the exact Card and Installation without calling an Agent.
6. **Given** a disabled Installation, **when** the owner uninstalls it,
   **then** the terminal row remains inspectable and a new install uses a new
   Installation identity.

### User Story 2 - Prove Durable Facts Across Restart (Priority: P1)

As an operator, I need Workspace and Installation facts to survive a process
or store reconstruction so that authorization and audit history do not depend
on in-memory state.

**Independent Test**: Complete create, install, lifecycle, and uninstall
operations, reconstruct the data access and service boundary, then compare all
returned immutable and lifecycle fields before and after reconstruction.

**Acceptance Scenarios**:

1. **Given** current and terminal Installation rows, **when** a fresh service
   reads the Workspace and paginates its history, **then** every row appears
   exactly once in the documented order.
2. **Given** a terminal Installation, **when** the store is reconstructed,
   **then** its identity, pin, permissions, terminal status, and timestamps are
   unchanged.
3. **Given** a prior uninstalled row, **when** the owner reinstalls the Agent,
   **then** the old row remains terminal and the new row has a distinct
   identity.

### User Story 3 - Prove Concurrency and State Invariants (Priority: P1)

As a platform maintainer, I need concurrent requests to produce only legal
committed histories and one current Installation so later dispatch can trust
Workspace facts.

**Independent Test**: Race duplicate installs and lifecycle/reinstall requests,
collect every result, inspect all committed rows, and validate each successful
transition and terminal timestamp against the approved state machine.

**Acceptance Scenarios**:

1. **Given** 100 concurrent installs for the same Workspace and Agent, **when**
   they complete, **then** exactly one succeeds, all other outcomes are the
   documented conflict, and exactly one current row exists.
2. **Given** concurrent lifecycle requests for one Installation, **when** they
   complete, **then** every successful transition is legal, conflicts do not
   alter timestamps or immutable fields, and no current-row uniqueness rule is
   violated.
3. **Given** concurrent uninstall and reinstall requests, **when** they
   complete, **then** the old row is terminal, any new current row has a new
   identity, and no result claims an uncommitted write succeeded.

### User Story 4 - Preserve Failure and Trust Boundaries (Priority: P1)

As a platform maintainer, I need failures and authorization decisions to stay
explicit so an outage or malformed request cannot look like an empty, stale, or
successful result.

**Independent Test**: Exercise the public and internal HTTP boundaries with
invalid, unauthenticated, unauthorized, missing, conflicting, and dependency
failure inputs and compare status, platform error code, Trace header, and
response fields.

**Acceptance Scenarios**:

1. **Given** missing or malformed authentication, **when** a public or internal
   request is sent, **then** it returns the documented unauthenticated result
   with a safe Trace header.
2. **Given** a non-owner Workspace caller, **when** it accesses or mutates an
   Installation, **then** it returns forbidden and does not change durable
   state.
3. **Given** an unknown Workspace, wrong Installation/Workspace pair, missing
   permission, unknown capability, disabled Installation, or Catalog-disabled
   Agent, **when** the relevant operation is requested, **then** its exact
   documented not-found, authorization, conflict, or disabled outcome is
   preserved.
4. **Given** malformed input, a canceled dependency, schema failure, or
   transaction failure, **when** the operation is requested, **then** the
   response is an explicit validation or dependency error and never an empty,
   stale, or successful fallback.
5. **Given** any error response, **when** its JSON body and headers are
   inspected, **then** it contains no API key, token, password, or dependency
   connection detail.

## Functional Requirements

- **FR-001**: The acceptance evidence MUST demonstrate discovery of a
  published Agent by capability through the public Catalog boundary.
- **FR-002**: The acceptance evidence MUST demonstrate Workspace creation and
  owner-authorized Installation of the highest matching published version with
  an exact permission snapshot.
- **FR-003**: The acceptance evidence MUST cover Installation list and detail
  inspection for current and terminal rows, including opaque-cursor traversal
  without duplicates or skipped rows.
- **FR-004**: The acceptance evidence MUST cover every legal and illegal
  lifecycle transition in the approved state machine and verify immutable facts
  and committed timestamps.
- **FR-005**: The acceptance evidence MUST demonstrate exact internal
  resolution only for an enabled, currently published, capability-authorized
  Installation and preserve invocation, root-task, and Trace identifiers.
- **FR-006**: The acceptance evidence MUST prove restart reconstruction,
  terminal-history durability, and reinstall-with-new-identity behavior.
- **FR-007**: The acceptance evidence MUST prove concurrent install and
  lifecycle operations produce no duplicate current Installation, illegal
  history, impossible timestamp relation, or false success.
- **FR-008**: Public and internal HTTP acceptance tests MUST use the active
  routes and error mappings without adding historical routes, dual reads, or
  compatibility fallbacks.
- **FR-009**: Authentication, owner policy, missing identity, not-found,
  conflict, disabled, validation, and dependency outcomes MUST remain
  distinguishable at their owning boundary.
- **FR-010**: Database, schema, transaction, malformed-input, and canceled
  dependency failures MUST remain visible and MUST NOT be converted to empty,
  stale, or successful results.
- **FR-011**: Acceptance tests MUST not invoke, probe, deploy, or depend on an
  Agent endpoint, Router, Ledger, frontend, or runtime framework.
- **FR-012**: The feature MUST provide a reproducible quickstart, complete
  Spec/Plan/Tasks evidence, an independent review record, and a converge result
  identifying Invocation Dispatch/A2A Router as the next scope.

## Success Criteria

- **SC-001**: One clean-database acceptance run completes the full
  Create -> Discover -> Install -> Inspect -> Lifecycle -> Resolve -> Uninstall
  workflow with all expected results and no Agent endpoint call.
- **SC-002**: The acceptance suite covers 100% of FR-001 through FR-011 with
  at least one mapped test or deterministic verification command per
  requirement.
- **SC-003**: A 100-request duplicate-install race produces exactly one current
  Installation and 99 documented conflicts.
- **SC-004**: A concurrent lifecycle/reinstall run leaves only legal history,
  at most one current row, distinct reinstall identity, and no false-success
  result.
- **SC-005**: Every restart comparison preserves all Installation immutable
  fields and lifecycle timestamps, and every history page contains each row
  exactly once.
- **SC-006**: Every exercised public/internal error class has the documented
  status and platform code, a safe Trace header, and no secret or dependency
  detail.
- **SC-007**: Contract, unit, integration, race, vet, build, migration,
  Compose, and diff checks pass; when the dedicated database is unavailable,
  integration execution is explicitly reported as not run rather than passed.
- **SC-008**: The completed feature adds zero fallback behaviors and records
  the retained legitimate empty-list policy with its existing contract source.

## Key Entities

- **Published Agent Card**: The Catalog-owned versioned description selected by
  capability and version constraint.
- **Workspace**: The immutable owner boundary for Installation authorization.
- **Installation**: The Workspace-owned durable pin, accepted permission
  snapshot, lifecycle state, and historical identity.
- **Resolution Request**: The internally authenticated request carrying
  invocation, root-task, Trace, Workspace, Agent, version, and capability
  identifiers.
- **Acceptance Evidence**: A reproducible test, command, or document mapping a
  requirement to an observed result without changing production ownership.

## Assumptions

- Active Agent Card, Workspace, Installation, Northbound API v3, Control Plane
  Internal API v2, and Platform Error contracts remain unchanged.
- Issues #6, #7, and #8 are merged into the base branch and their existing
  tests are reused rather than copied into a second business implementation.
- A dedicated PostgreSQL database ending in `_test` is available for full
  integration execution in environments that claim the integration gate.
- Test fixtures may seed a published Agent through the Catalog service where
  the scenario under test is Workspace or HTTP behavior; the public discovery
  path itself is exercised through Gateway.

## Out of Scope

- Invocation Dispatch, A2A Router, Task/Session transport, Ledger, SDK, Sample
  Agents, frontend Console, deployment, Kubernetes, or Agent endpoint calls.
- New public or internal API versions, historical route fallback, retry,
  caching, alternate data sources, reconciliation, or degraded success.
- New production data models, migrations, runtime services, or authorization
  policy.
- Performance/load benchmarking beyond the deterministic 100-request
  concurrency acceptance cases.

## Fallback Policy

```text
Fallback delta: removed 0, retained 1, added 0, net 0
Added fallback evidence: none
```

The one retained fallback-classified behavior is the legitimate empty
Installation list for an existing authorized Workspace, already defined by the
active Workspace contract. Issue #9 adds no fallback.
