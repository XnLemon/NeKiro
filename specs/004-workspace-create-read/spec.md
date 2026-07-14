# Feature Specification: Create and Read an Owner-Controlled Workspace

**Feature Branch**: `codex/004-workspace-create-read`
**Created**: 2026-07-15
**Status**: Approved
**Input**: GitHub issue #4, dependent on issue #3

## Clarifications

- A Workspace is a logical authorization and audit boundary, not an
  infrastructure namespace.
- The authenticated creator is the immutable owner. Owner identity is taken
  only from trusted Gateway authentication context; it is never accepted in
  the request body, path, or a caller-controlled header.
- The caller chooses the Workspace identifier. It must be non-empty, exact,
  case-sensitive, and use the active safe-identifier rules.
- Reusing an identifier is a conflict for every caller. A retry is not an
  implicit idempotent success and must not alter the original owner or times.
- Only the owner may read the Workspace. An unknown identifier is not found;
  an existing Workspace requested by a non-owner is forbidden.
- The public Workspace fact contains exactly `workspaceId`, `ownerId`,
  `createdAt`, and `updatedAt`. Membership, role, deployment, Kubernetes,
  endpoint, and secret fields are outside this feature.
- Owner authorization is called through the existing narrow policy boundary so
  a future Membership/RBAC policy can replace the equality decision without
  changing the Workspace fact or API.
- Persistence, migration, and readiness failures are explicit dependency
  failures. No anonymous owner, localhost database, in-memory store, stale
  read, retry, or alternate persistence source is allowed.

## User Scenarios & Testing

### User Story 1 - Establish a Workspace Boundary (Priority: P1)

An authenticated platform caller creates a durable Workspace with an explicit
safe identifier. The platform records the caller as the immutable owner and
the owner can read the exact resulting Workspace after a process restart.

**Why this priority**: Every later Installation needs a trusted Workspace
authorization root.

**Independent Test**: Create a Workspace as owner A, read it as owner A,
restart or reconstruct the process, read it again, reject owner B, reject an
unknown identifier, and verify that duplicate creation never changes the
stored owner or timestamps.

#### Acceptance Scenarios

1. **Given** an authenticated caller and an unused safe identifier, **when**
   the caller creates the Workspace, **then** the response contains the exact
   identifier, the trusted caller as owner, and server-assigned timestamps.
2. **Given** an existing Workspace, **when** its owner reads it, **then** the
   response contains the exact four-field durable fact.
3. **Given** an existing Workspace, **when** a different authenticated caller
   reads it, **then** the operation returns the contract-defined forbidden
   error and does not reveal the Workspace body.
4. **Given** an unknown identifier, **when** an authenticated caller reads it,
   **then** the operation returns the contract-defined not-found error and
   does not return an empty or synthetic Workspace.
5. **Given** an existing identifier, **when** any caller creates it again,
   **then** the operation returns conflict and the original owner and
   timestamps remain unchanged.
6. **Given** missing, malformed, or rejected authentication, **when** a
   caller creates or reads a Workspace, **then** the operation returns
   unauthenticated and the Workspace service is not invoked.
7. **Given** a committed Workspace and a process restart, **when** its owner
   reads it, **then** all four fields are identical to the committed values.
8. **Given** an unavailable or unready Workspace persistence dependency,
   **when** the process starts or a request is served, **then** readiness or
   the request fails explicitly and never reports an empty or successful
   Workspace result.

## Edge Cases

- Empty, whitespace-only, overlong, punctuation-invalid, or alternate-case
  identifiers follow the active identifier validator; input is never trimmed
  or normalized.
- Duplicate JSON members, unknown JSON members, trailing JSON values, and
  malformed JSON are validation failures.
- `ownerId`, `createdAt`, and `updatedAt` in a create request are rejected as
  unknown fields. The caller cannot override server values.
- A valid authentication token that maps to an invalid principal identifier is
  rejected at the Workspace boundary; no default principal is selected.
- Database conflict, missing schema, stale schema, query failure, and commit
  failure remain dependency or conflict outcomes according to the active
  contract and never become not-found or empty success.
- The owner policy must not inspect or create Membership, RBAC, deployment,
  endpoint, or secret state.

## Requirements

### Functional Requirements

- **FR-001**: The platform MUST accept Workspace creation only after trusted
  authentication succeeds.
- **FR-002**: Creation MUST accept exactly one caller-supplied `workspaceId`
  and MUST derive `ownerId` from the trusted authentication context.
- **FR-003**: A Workspace MUST contain exactly `workspaceId`, `ownerId`,
  `createdAt`, and `updatedAt`; creation MUST assign both timestamps at the
  platform boundary.
- **FR-004**: A Workspace identifier MUST be safe, exact, case-sensitive, and
  rejected when missing, malformed, blank, or overlong.
- **FR-005**: Creating an existing identifier MUST return `CONFLICT` and MUST
  preserve the original row byte-for-byte at the domain-field level.
- **FR-006**: Reading a Workspace MUST require the owner policy boundary;
  owner success, non-owner `FORBIDDEN`, and unknown `NOT_FOUND` are distinct.
- **FR-007**: Authentication failure MUST return `UNAUTHENTICATED` before
  request-body or persistence processing and MUST never infer an owner.
- **FR-008**: Workspace persistence MUST be owned by the Workspace module;
  Catalog code MUST NOT read or write Workspace tables.
- **FR-009**: Workspace migration and readiness MUST verify the required
  Workspace schema explicitly and MUST fail for missing, stale, incomplete,
  or unavailable dependencies.
- **FR-010**: The owner policy MUST be replaceable through its narrow boundary
  without adding Membership, role, deployment, Kubernetes, endpoint, or
  secret fields to the Workspace entity.
- **FR-011**: Committed Workspace facts MUST survive process reconstruction
  with unchanged owner and timestamps.
- **FR-012**: Public success and error responses MUST use the active versioned
  contract, fixed messages, trace correlation, and secret-safe payloads.
- **FR-013**: This feature MUST add zero fallback behavior.

## Success Criteria

- **SC-001**: An authenticated caller can create and read an owner-controlled
  Workspace using the active public API in one complete workflow.
- **SC-002**: 100% of duplicate, non-owner, unknown, invalid-input, and
  unauthenticated cases return their exact contract-defined outcome without
  changing stored data.
- **SC-003**: 100% of committed Workspace facts retain all four fields after
  process reconstruction.
- **SC-004**: Missing, stale, incomplete, or unavailable Workspace persistence
  never reports readiness or a request as successful.
- **SC-005**: The implementation adds no fallback and no fields outside the
  approved Workspace fact.

## Assumptions

- Issue #3 has frozen Workspace v1, Northbound API v3, Platform Error v3, and
  the owner-policy boundary used here.
- The current Control Plane process and Gateway are reused; this slice does
  not create a new service or frontend route.
- Authentication credentials and principal management remain outside this
  feature. The existing explicit development authenticator is only a local
  adapter and has no anonymous mode.

## Non-Goals

- Agent Installation, SemVer selection, accepted permissions, lifecycle,
  exact resolution, Invocation Dispatch, A2A Router, or Ledger behavior.
- Membership, RBAC, organization management, OIDC, approval workflows, or
  multi-tenant governance.
- Workspace deletion, owner transfer, rename, update, or idempotency keys.
- Agent deployment, endpoint health, Kubernetes binding, cache, queue, retry,
  search, or alternate persistence.
- Frontend changes.
