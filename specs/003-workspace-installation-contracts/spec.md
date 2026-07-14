# Feature Specification: Minimal Workspace and Installation Contract Gate

**Feature Branch**: `codex/003-workspace-installation-contracts`

**Created**: 2026-07-14

**Status**: Draft

**Input**: GitHub issue #3, "Define the Minimal Workspace and Installation
contract gate," under the approved product design in issue #2.

## Clarifications

### Session 2026-07-14

- Q: What is a Phase 1 Workspace and who controls it? -> A: It is a logical
  authorization and audit boundary with one immutable creator-owner; it is not
  an infrastructure namespace.
- Q: How is an Agent version selected? -> A: Select the highest currently
  published version satisfying the submitted SemVer constraint, persist the
  exact version, and never upgrade it automatically.
- Q: How are pre-release and equal-precedence versions handled? -> A:
  Pre-release versions participate only when their constraint branch explicitly
  includes a pre-release comparator; equal SemVer precedence is resolved by
  the bytewise-greatest exact version string as a deterministic build-metadata
  tie-break.
- Q: Which permissions may an owner accept? -> A: Any exact, case-sensitive
  subset of permissions declared by the selected Card, including the empty set;
  unknown IDs are invalid and a capability is allowed only when every one of
  its required permissions is in the accepted snapshot.
- Q: What uniqueness and lifecycle rules apply? -> A: One Workspace may have
  at most one enabled or disabled Installation for an Agent; lifecycle is
  `enabled <-> disabled -> uninstalled`, and reinstall after uninstall creates
  a new Installation.
- Q: What happens when Catalog later disables the pinned version? -> A: Keep
  the Installation fact unchanged and reject exact resolution with
  `AGENT_DISABLED`.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Establish a Workspace Boundary (Priority: P1)

An authenticated caller creates a durable Workspace using a safe identifier.
The creator becomes its sole owner and can read the resulting authorization
boundary without supplying or overriding owner identity.

**Why this priority**: Every Installation needs a trusted Workspace owner and a
durable authorization root before permissions can be accepted.

**Independent Test**: Create a Workspace as one caller, read it as that caller,
and verify that another caller cannot read it or alter its owner.

**Acceptance Scenarios**:

1. **Given** an authenticated caller and an unused safe Workspace identifier,
   **When** the caller creates the Workspace, **Then** the Workspace is stored
   with that caller as owner and server-assigned creation and update times.
2. **Given** an existing Workspace, **When** its owner reads it, **Then** the
   exact stored identifier, owner, and timestamps are returned.
3. **Given** an existing Workspace, **When** a different authenticated caller
   attempts to read it, **Then** the operation is forbidden without changing
   the Workspace.
4. **Given** an existing Workspace identifier, **When** any caller attempts to
   create it again, **Then** the operation reports a conflict and does not
   reinterpret the retry as success.

---

### User Story 2 - Install and Pin an Agent Version (Priority: P1)

A Workspace owner installs a published Agent by submitting an Agent identifier,
a SemVer constraint, and the exact subset of declared permissions they accept.
The platform resolves and permanently records one exact version and permission
snapshot.

**Why this priority**: This story closes `Discover -> Install` and creates the
pre-dispatch trust fact used by every later managed invocation.

**Independent Test**: Publish several stable and pre-release versions, install
with a constraint and permission subset, and verify that the selected exact
version and accepted set remain unchanged after newer versions are published.

**Acceptance Scenarios**:

1. **Given** multiple published versions satisfying a valid constraint,
   **When** the owner installs the Agent, **Then** the highest eligible SemVer
   is stored as the exact installed version.
2. **Given** only pre-release matches and a constraint branch that does not
   explicitly include a pre-release comparator, **When** installation is
   requested, **Then** no version is selected and the operation reports not
   found.
3. **Given** a constraint that explicitly includes pre-releases, **When** stable
   and pre-release candidates are evaluated, **Then** all eligible candidates
   use SemVer precedence and the highest candidate is pinned.
4. **Given** multiple candidates equal in SemVer precedence because build
   metadata differs, **When** installation is requested, **Then** the
   bytewise-greatest exact version string is pinned.
5. **Given** accepted permission IDs that form a subset of the exact selected
   Card, **When** installation succeeds, **Then** that exact set is stored in
   deterministic order and is not expanded by later Card versions.
6. **Given** any accepted permission ID not declared by the selected Card,
   **When** installation is requested, **Then** validation fails and no
   Installation is stored.
7. **Given** an enabled or disabled Installation for the same Workspace and
   Agent, **When** another install is requested, **Then** the operation reports
   conflict and no duplicate current Installation is created.
8. **Given** a successful Catalog resolution followed by a concurrent Catalog
   disable, **When** the Installation commit completes, **Then** the exact pin
   remains historical truth and subsequent resolution reports
   `AGENT_DISABLED`.

---

### User Story 3 - Inspect Installation Facts (Priority: P2)

A Workspace owner reads one Installation or lists all current and historical
Installations in the Workspace, including exact version, accepted permissions,
state, and timestamps.

**Why this priority**: Owners need to review the durable authorization facts
before managing access or diagnosing a rejected invocation.

**Independent Test**: Read and list enabled, disabled, and uninstalled records,
then verify an empty Workspace returns a genuine empty list while missing,
forbidden, and dependency-failed requests do not.

**Acceptance Scenarios**:

1. **Given** an owned Workspace with current and uninstalled Installations,
   **When** the owner lists Installations with a bounded page size, **Then** at
   most that many records are returned in stable installation-time and
   identifier order, and an opaque cursor continues the traversal when more
   records remain.
2. **Given** an owned Workspace with no Installations, **When** the owner lists
   Installations, **Then** the response contains an explicit empty items array
   without a cursor.
3. **Given** an Installation identifier in the owned Workspace, **When** the
   owner reads it, **Then** its complete immutable pin, accepted permission
   snapshot, state, and lifecycle timestamps are returned.
4. **Given** a missing Workspace, a non-owner, a missing Installation, or a
   failed persistence dependency, **When** inspection is attempted, **Then**
   each condition produces its distinct failure rather than an empty list or
   synthetic record.

---

### User Story 4 - Manage Installation Lifecycle (Priority: P2)

A Workspace owner disables and re-enables current Installations, then disables
and uninstalls one when access should end. Uninstall preserves the historical
Installation and releases the uniqueness slot for a later fresh install.

**Why this priority**: Installation is an authorization lifecycle, not a
bookmark; owners need explicit, auditable access changes.

**Independent Test**: Exercise every legal and illegal transition, then
reinstall the same Agent and verify a new Installation identifier is created
without rewriting the uninstalled record.

**Acceptance Scenarios**:

1. **Given** an enabled Installation, **When** the owner disables it, **Then**
   its state becomes disabled and its update time advances.
2. **Given** a disabled Installation, **When** the owner enables it, **Then** its
   state becomes enabled without changing its exact version or permissions.
3. **Given** a disabled Installation, **When** the owner uninstalls it, **Then**
   its state becomes uninstalled, both lifecycle timestamps advance, and the
   record remains readable.
4. **Given** an enabled Installation, **When** uninstall is requested, **Then**
   the operation reports conflict and requires an explicit disable first.
5. **Given** an Installation already in the requested state or already
   uninstalled, **When** a lifecycle mutation is repeated, **Then** the
   operation reports conflict rather than an undocumented idempotent success.
6. **Given** an uninstalled historical record, **When** the owner installs the
   same Agent again, **Then** a new enabled Installation with a new identifier
   is created and the old record remains unchanged.
7. **Given** concurrent install and lifecycle requests, **When** they complete,
   **Then** at most one enabled or disabled Installation exists and every
   stored state transition is legal.

---

### User Story 5 - Resolve Authorized Exact Agent Facts (Priority: P2)

The later A2A Router asks the trusted internal Control Plane boundary to resolve
an exact Agent version and capability for one Workspace. The response includes
only the currently published exact Card and enabled, capability-authorizing
Installation facts.

**Why this priority**: This is the pre-dispatch trust boundary that keeps the
Router out of Control Plane storage and prevents direct, unauthorized Agent
calls.

**Independent Test**: Resolve one allowed capability, then independently vary
the requested version, Installation state, Catalog state, capability, accepted
permissions, internal identity, and dependencies to verify the exact outcomes.

**Acceptance Scenarios**:

1. **Given** a trusted internal caller, an enabled Installation whose exact
   version matches the request, a currently published Card, and a capability
   whose required permissions are all accepted, **When** resolution is
   requested, **Then** the exact Card and enabled Installation facts are
   returned with unchanged invocation correlation.
2. **Given** no current Installation or a requested version different from the
   pinned version, **When** resolution is requested, **Then** it reports
   `AGENT_NOT_INSTALLED`.
3. **Given** a disabled Installation, **When** resolution is requested,
   **Then** it reports `INSTALLATION_DISABLED`.
4. **Given** an enabled Installation whose pinned Catalog version is disabled,
   **When** resolution is requested, **Then** it reports `AGENT_DISABLED`
   without rewriting the Installation.
5. **Given** a missing capability or one whose required permissions are not all
   accepted, **When** resolution is requested, **Then** it reports
   `CAPABILITY_NOT_ALLOWED`.
6. **Given** invalid correlation, an untrusted internal caller, or a failed
   Control Plane dependency, **When** resolution is requested, **Then** the
   request fails respectively as validation, unauthenticated, or dependency
   error and never returns a Card.

### Edge Cases

- Workspace, Agent, capability, permission, and Installation identifiers use
  exact case-sensitive equality; whitespace is not trimmed and alternate case
  does not identify the same value.
- A valid empty accepted-permission set creates an Installation, but only
  capabilities requiring no permissions can resolve through it.
- Invalid, empty, whitespace-only, or overlong SemVer constraints fail
  validation; they are never replaced by an exact version or wildcard.
- Draft and disabled Agent versions are not installation candidates. No
  matching published version is not found, not a dependency failure.
- A Catalog dependency failure during selection or exact resolution is a
  dependency error, not not-found, an empty result, or stale success.
- A list operation returns an empty array only after authenticating the owner
  and proving that the Workspace exists and has no Installation records.
- Build metadata does not alter SemVer precedence; it participates only in the
  documented deterministic exact-string tie-break.
- Enabling an Installation does not probe or rewrite Catalog state. If the
  pinned Agent version is disabled, later exact resolution remains
  `AGENT_DISABLED`.
- An Installation identifier presented under a different Workspace is not
  found within that Workspace and does not move between authorization roots.
- Concurrent lifecycle operations lock one Installation fact and cannot skip
  required intermediate states or resurrect an uninstalled record.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The platform MUST let an authenticated caller create a Workspace
  with a caller-selected safe identifier and MUST derive the immutable owner
  from trusted authentication context.
- **FR-002**: A Workspace MUST contain exactly `workspaceId`, `ownerId`,
  `createdAt`, and `updatedAt`; it MUST NOT contain infrastructure, membership,
  deployment, or secret fields.
- **FR-003**: The platform MUST let only the Workspace owner read and manage the
  Workspace's Installation facts through an owner-only policy boundary that a
  later authorization implementation can replace without changing those facts.
- **FR-004**: Creating an existing Workspace identifier MUST return `CONFLICT`;
  the operation MUST NOT be treated as idempotent success.
- **FR-005**: Installation MUST accept an Agent identifier, a valid SemVer
  constraint, and a unique array of accepted permission IDs. The range is
  bounded by the active SemVer parser rather than an additional contract-level
  length restriction.
- **FR-006**: Installation MUST resolve only currently published Agent versions
  and MUST select the highest matching SemVer precedence.
- **FR-007**: A constraint branch without an explicit pre-release comparator
  MUST exclude pre-release candidates; a branch with one MUST evaluate eligible
  pre-releases using SemVer precedence.
- **FR-008**: When matching versions have equal SemVer precedence because only
  build metadata differs, Installation MUST select the bytewise-greatest exact
  version string.
- **FR-009**: The platform MUST persist the submitted constraint and selected
  exact version, and MUST NOT automatically upgrade or reconcile the pin.
- **FR-010**: Accepted permissions MUST be an exact, case-sensitive subset of
  the exact selected Card's declared permission IDs; the empty subset is valid,
  and any unknown ID MUST return `VALIDATION_ERROR` without persistence.
- **FR-011**: The accepted permission snapshot MUST be stored and returned in
  ascending bytewise identifier order without trimming, case folding, or later
  expansion. Installation v2 validation MUST reject unsorted or duplicate
  values rather than normalizing them.
- **FR-012**: A Workspace MUST have at most one non-uninstalled Installation
  for an Agent, including under concurrent requests; a duplicate current
  Installation MUST return `CONFLICT`.
- **FR-013**: A new Installation MUST begin enabled with a new platform-assigned
  safe identifier and server-assigned `installedAt` and `updatedAt` values.
- **FR-014**: Installation lifecycle MUST be exactly
  `enabled -> disabled`, `disabled -> enabled`, and
  `disabled -> uninstalled`; same-state, enabled-to-uninstalled, and every
  transition from uninstalled MUST return `CONFLICT`.
- **FR-015**: Uninstall MUST preserve the Installation record, set
  `uninstalledAt`, update `updatedAt`, and release that Agent's current
  uniqueness slot only in the same successful transition. Every Installation
  MUST satisfy `installedAt <= updatedAt`; an uninstalled record MUST satisfy
  `uninstalledAt == updatedAt`.
- **FR-016**: Reinstalling after uninstall MUST create a new Installation and
  MUST NOT reuse or modify the historical Installation identifier.
- **FR-017**: The owner MUST be able to read one Installation and list current
  and historical Installations through bounded pages ordered by `installedAt`
  ascending and then `installationId` ascending. List pages MUST accept an
  explicit required `limit` with bounds `1-100`, plus an opaque
  continuation cursor bound to the Workspace, page size, and last ordering
  tuple.
- **FR-018**: A successful list with no records MUST contain an explicit empty
  `items` array without a cursor; missing Workspace, forbidden caller, invalid
  or filter-mismatched cursor, and dependency failure MUST remain failures.
- **FR-019**: Exact internal resolution MUST require a trusted internal caller,
  the existing invocation, root Task, and Trace identifiers, Workspace, Agent,
  exact version, and capability.
- **FR-020**: Exact internal resolution MUST return only when a non-uninstalled
  Installation exists, its pinned version equals the requested version, it is
  enabled, the exact Catalog version is currently published, and the requested
  capability exists with every required permission accepted.
- **FR-021**: Exact resolution MUST preserve the request's invocation, root
  Task, and Trace correlation in every post-correlation error and MUST NOT
  synthesize or replace those identifiers. A failure before strict correlation
  validation MUST use the explicit pre-correlation error shape containing only
  `code`, fixed `message`, and a generated safe `traceId`; it MUST NOT echo
  malformed or missing request IDs. Once correlation validates, later errors
  MUST use the exact request values.
- **FR-022**: If Catalog disables a pinned version, the Installation MUST remain
  unchanged and exact resolution MUST return `AGENT_DISABLED`.
- **FR-023**: Exact resolution MUST distinguish `AGENT_NOT_INSTALLED`,
  `INSTALLATION_DISABLED`, `AGENT_DISABLED`, and
  `CAPABILITY_NOT_ALLOWED` from owner `FORBIDDEN`, generic `NOT_FOUND`, and
  `DEPENDENCY_ERROR`.
- **FR-024**: Every Workspace and Installation operation MUST distinguish
  invalid input, unauthenticated identity, forbidden ownership, not found,
  conflict, and dependency failure using fixed public messages and safe Trace
  correlation.
- **FR-025**: Operation precedence MUST be authentication, request validation,
  Workspace existence, owner authorization, current Installation conflict or
  target lookup, Catalog evaluation when required, permission evaluation, and
  persistence. Installation selection MUST occur only after the Workspace
  preflight, and the Workspace transaction MUST recheck Workspace ownership and
  current uniqueness before insert. Concurrent uniqueness conflicts MUST map to
  the same explicit `CONFLICT` outcome.
- **FR-026**: Workspace persistence MUST own Workspace and Installation facts;
  Catalog facts MUST be consumed through a controlled interface and Workspace
  behavior MUST NOT read or write Catalog-owned storage directly.
- **FR-027**: A successful Catalog selection is the Installation version-choice
  linearization point. A later concurrent Catalog disable MUST not roll back or
  rewrite the Installation and MUST be visible on the next exact resolution.
- **FR-028**: No operation may substitute stale data, a historical Card, an
  alternate data source, a default owner, a default constraint, or a success
  response when required Workspace, Catalog, identity, or persistence behavior
  fails.
- **FR-029**: Contracts MUST exclude credentials, deployment targets,
  Kubernetes identifiers, membership, policy-engine payloads, quotas,
  approval state, and Agent input or output.
- **FR-030**: The active versioned Workspace, Installation, Northbound, internal
  resolution, and Platform Error contracts MUST declare every approved field,
  operation, response status, correlation requirement, and exact error-code set
  before runtime implementation begins.

### Error Outcome Matrix

| Condition | Public outcome |
| --- | --- |
| Malformed or semantically invalid request | `400 VALIDATION_ERROR` |
| Missing or invalid trusted identity | `401 UNAUTHENTICATED` |
| Authenticated non-owner on a Workspace operation | `403 FORBIDDEN` |
| Missing Workspace, Installation, or matching published install candidate | `404 NOT_FOUND` |
| Duplicate Workspace/current Installation or illegal lifecycle transition | `409 CONFLICT` |
| Missing current exact Installation during internal resolution | `404 AGENT_NOT_INSTALLED` |
| Current Installation is disabled | `403 INSTALLATION_DISABLED` |
| Exact pinned Catalog version is disabled | `403 AGENT_DISABLED` |
| Capability missing or required permission not accepted | `403 CAPABILITY_NOT_ALLOWED` |
| Required Workspace, Catalog, or persistence dependency fails | `503 DEPENDENCY_ERROR` |

### Key Entities

- **Workspace**: A durable logical authorization and audit root identified by a
  caller-selected safe ID, controlled by one immutable trusted owner, and
  timestamped by the platform.
- **Installation**: A durable record that one Workspace accepted an exact
  permission subset for one exact Agent version selected from a submitted
  constraint; it has an explicit lifecycle and preserves uninstalled history.
- **Accepted Permission Snapshot**: The exact set of permission identifiers
  accepted at install time, independent of later Agent Card versions.
- **Exact Resolution**: A read-only authorization result combining current
  Workspace Installation facts with the exact current Catalog Card state for a
  requested capability.

### Runtime/Platform Boundary

- **Platform-owned behavior**: Workspace identity and owner authorization,
  Installation version selection and permission acceptance, lifecycle,
  Catalog-controlled reads, and exact pre-dispatch resolution.
- **Runtime-owned behavior**: Agent deployment, health, model calls, prompts,
  tools, workflows, memory, sessions, and capability execution remain external.
- **Cross-runtime proof**: The same Installation and exact-resolution contracts
  apply to both Phase 1 sample Agents regardless of their implementation
  language or Runtime; this feature does not start or invoke either Agent.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A trusted owner can complete create, read, install, inspect,
  disable, re-enable, and uninstall using only the declared platform boundary,
  with every operation returning the exact stored fact or documented failure.
- **SC-002**: Across a conformance matrix containing stable, pre-release,
  build-metadata, invalid-range, disabled, and no-match candidates, 100% of
  repeated installations select the same documented exact version or the same
  documented failure.
- **SC-003**: For every subset of a test Card's declared permissions, exact
  resolution allows 100% of capabilities whose required set is contained and
  denies 100% of all others; every unknown accepted permission is rejected.
- **SC-004**: In a 100-request concurrent same-Workspace/same-Agent install
  test, exactly one non-uninstalled Installation is committed and every other
  request reports conflict without partial records.
- **SC-005**: After a persistence restart, 100% of created Workspace fields,
  exact version pins, permission snapshots, states, and lifecycle timestamps
  remain identical.
- **SC-006**: Every accepted scenario and edge case maps to one testable
  success or exact error outcome, with zero dependency failures represented as
  empty collections, not found, or success.
- **SC-007**: Contract inspection finds zero Workspace or Installation fields
  for Kubernetes, deployment, membership, general RBAC, policy engines,
  invocation content, credentials, automatic upgrades, or background repair.

## Assumptions

- Gateway authentication already maps a Bearer credential to one exact trusted
  caller identifier; public caller-ID headers are not trusted.
- Catalog preserves immutable historical Agent Card versions and can expose a
  controlled selection/read interface without giving Workspace code direct
  storage access.
- Agent Card `0.2`, Northbound API `v3`, Control Plane Internal API `v2`, and
  Platform Error `v3` for Workspace/Installation are the active contract
  foundations. Platform Error `v2` remains active for Catalog/Invocation. No
  deployed Workspace runtime consumer requires a compatibility window.
- SemVer constraint grammar follows the repository's pinned range contract and
  is evaluated without trimming or loose-version coercion.
- Server-generated Installation identifiers and timestamps are authoritative;
  callers cannot submit them.
- The explicit empty Installation list is genuine product behavior, not a
  failure fallback.

## Non-Goals

- Workspace membership, invitations, organizations, role assignment, general
  RBAC, or identity-provider administration.
- Kubernetes Namespace or workload creation, deployment bindings, scheduling,
  health checks, rollback, Secret management, or any Agent Runtime behavior.
- Automatic Agent upgrades, background reconciliation, alternate Card sources,
  caches, queues, or compatibility fallback.
- Generic policy engines, Policy Hooks, quota, approval, billing, Marketplace,
  or enterprise governance.
- Invocation Dispatch, A2A transport, result delivery, Ledger behavior,
  Console work, or live sample Agent implementation.
- An explicit upgrade operation or mutation of the exact version and accepted
  permissions on an existing Installation.
