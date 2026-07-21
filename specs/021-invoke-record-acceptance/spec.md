# Feature Specification: Invoke-to-Record Backend Acceptance

**Feature Branch**: `codex/021-invoke-record-acceptance`
**Parent Task**: Issue #30 / T011 of Spec 010
**Status**: Draft for implementation

## Context

All platform slices required by Spec 010 are now present independently. The
remaining risk is integration: a clean run must use the real Control Plane,
Router-owned PostgreSQL Ledger, Runtime B, and Runtime A processes together.
This feature adds only the final process wiring and acceptance evidence. It
does not change the active runtime contracts or move ownership between modules.

## User Scenarios and Acceptance

### US1 - Complete the managed loop (P1)

As a Workspace owner, I can register and publish both sample Cards, discover a
capability, install the exact version, invoke through Gateway, and query the
resulting metadata.

**Acceptance scenarios**

1. A clean environment completes `Register -> Discover -> Install -> Invoke ->
   Record` without a Gateway or Control Plane request addressing an Agent URL.
2. JSON returns one exact correlated result. SSE starts with `accepted`, keeps
   contiguous sequence/chunk indexes, and ends with one terminal event.
3. A Runtime A root call creates exactly one Runtime B child through the SDK and
   Router; both records share root task and trace and the child names the root
   as its parent.

### US2 - Prove durability and isolation (P1)

As an operator, I can restart Router and read committed Invocation and Trace
metadata from PostgreSQL without recovering Agent content.

**Acceptance scenarios**

1. Router reconstruction preserves ordered events and the terminal projection.
2. A second Workspace cannot read the first Workspace's Invocation or Trace.
3. Database rows, API metadata, and process logs contain no input, result,
   chunk, credential, or raw dependency error.

### US3 - Prove failure semantics and concurrency (P1)

As a maintainer, I can distinguish policy, route, protocol, Agent,
dependency, timeout, cancellation, and interrupted-delivery outcomes.

**Acceptance scenarios**

1. The acceptance suite covers each failure class at its owning boundary and
   verifies no false success or secret leakage.
2. One hundred concurrent accepted calls have unique correlation, no cross-call
   result leakage, and exactly one durable terminal outcome per invocation.

## Functional Requirements

- **FR-001**: Compose MUST start PostgreSQL migrations, Control Plane, Router,
  Runtime B, and Runtime A with explicit non-secret deployment configuration.
- **FR-002**: Runtime Cards used by acceptance MUST point at the actual service
  endpoints and remain versioned Registry facts.
- **FR-003**: The E2E harness MUST exercise registration, publication,
  capability discovery, Workspace installation, JSON invoke, SSE invoke, nested
  invoke, metadata reads, and a Router restart.
- **FR-004**: The harness MUST assert exact Invocation/root-task/Trace
  correlation, append-only event order, terminal uniqueness, and Workspace
  isolation.
- **FR-005**: The harness MUST exercise policy, route, protocol, Agent,
  dependency, timeout, cancellation, and interrupted delivery classes and
  assert their active Platform Error codes.
- **FR-006**: A 100-request run MUST assert one unique Invocation identity and
  one terminal Ledger projection per accepted request.
- **FR-007**: Persistent, API, and log inspection MUST reject Agent input,
  output, chunks, credentials, and raw dependency details.
- **FR-008**: CI MUST validate Compose configuration and run the acceptance
  harness with Docker and PostgreSQL; missing Docker/DB is a failed gate, not a
  successful skip.
- **FR-009**: No active contract, retry, cache, alternate route, stale Card,
  result persistence, or fallback policy may be added by this feature.

## Success Criteria

- **SC-001**: One clean Compose run passes Register -> Discover -> Install ->
  Invoke -> Record for both JSON and SSE.
- **SC-002**: The nested run produces exactly two Invocation records with one
  root Task, one Trace, and the exact parent-child relationship.
- **SC-003**: Router restart leaves all committed events and trace reads intact.
- **SC-004**: The failure matrix reports the expected distinct codes and no
  forbidden content appears in PostgreSQL, API responses, or logs.
- **SC-005**: One hundred concurrent requests produce 100 unique accepted
  Invocations and 100 terminal projections.
- **SC-006**: Active static, contract, integration, Compose, and E2E gates pass;
  independent Standards/Spec review and Converge have no blocker.

## Scope and Non-goals

In scope: process images, Compose wiring, deterministic Cards/fixture setup,
the backend E2E harness, CI execution, and acceptance documentation.

Out of scope: Console UI, Agent deployment/orchestration, result replay or
polling, new runtime behavior, contract changes, queues/caches/retries,
production identity federation, and general RBAC/quota/billing.

## Fallback Audit

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
