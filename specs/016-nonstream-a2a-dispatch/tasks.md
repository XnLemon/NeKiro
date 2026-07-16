# Tasks: Non-Streaming A2A Dispatch

**Input**: Design documents from `specs/016-nonstream-a2a-dispatch/`

**Scope**: Implement Router-owned non-streaming exact A2A dispatch and
transient result delivery. Tests are added after the corresponding
implementation work, per project policy.

## Phase 1: Setup

- [ ] T001 Create the Router non-streaming transport package skeleton in `apps/a2a-router/internal/transport/a2a/` with explicit dependencies and no fallback endpoint/default credential behavior.

---

## Phase 2: Foundational Mapping

- [ ] T002 Define focused Router-side interfaces for non-streaming Agent transport and metadata-only Ledger append ordering in `apps/a2a-router/internal/api/dispatch_handler.go` without importing Control Plane internals.
- [ ] T003 Map resolved Agent endpoint/profile/auth facts into a strict non-streaming transport target in `apps/a2a-router/internal/transport/a2a/`, rejecting unsupported states explicitly.

**Checkpoint**: Router can represent an exact resolved non-streaming target but still has no Agent side effect.

---

## Phase 3: User Story 1 - Dispatch a Non-Streaming Invocation

**Goal**: Replace the `ROUTE_NOT_FOUND` placeholder for `stream=false` with one A2A `message/send` call and live JSON result.

**Independent Test**: Runtime B `httptest` endpoint receives one call with platform context and the Router returns the deterministic result.

- [ ] T004 [US1] Implement A2A `message/send` client behavior in `apps/a2a-router/internal/transport/a2a/`.
- [ ] T005 [US1] Wire successful non-streaming transport into `apps/a2a-router/internal/api/dispatch_handler.go` while preserving existing validation/resolution failures.
- [ ] T006 [US1] Add mapped Runtime B success and context propagation tests in `apps/a2a-router/internal/api/dispatch_handler_test.go` and/or `apps/a2a-router/internal/transport/a2a/` tests.

---

## Phase 4: User Story 2 - Record Metadata-Only Lifecycle Facts

**Goal**: Commit required safe Ledger facts around accepted non-streaming dispatch without storing Agent content.

**Independent Test**: Strict recorder verifies ordering and metadata-only terminal facts for success and accepted failure.

- [ ] T007 [US2] Add Router Ledger append orchestration for accepted non-streaming dispatch in `apps/a2a-router/internal/api/dispatch_handler.go`.
- [ ] T008 [US2] Add mapped success, accepted failure, and Ledger failure tests proving terminal success is not emitted before required Ledger commit.

---

## Phase 5: User Story 3 - Preserve Boundaries and Failure Semantics

**Goal**: Keep pre-existing validation/resolution behavior and add explicit Agent transport failure mapping only.

**Independent Test**: Failure matrix covers unsupported target/profile/auth, endpoint dependency failure, protocol failure, Agent business failure, and no forbidden dependencies/fallbacks.

- [ ] T009 [US3] Implement explicit transport failure classification without retries, caches, compatibility branches, or fallback endpoints.
- [ ] T010 [US3] Add failure matrix and fallback/write-scope scan evidence to this tasks file.

---

## Phase 6: Verification, Review, and Converge

- [ ] T011 Run formatting, focused Router/Runtime B tests, WSL race where practical, full repository tests, vet, diff check, and record verification evidence.
- [ ] T012 Obtain fresh independent Review against Spec, Plan, Tasks, active contracts, and constitution; return findings to Spec/Tasks before fixes.
- [ ] T013 Run Converge after Review and append/resolve any remaining implementation tasks.

## Dependencies & Execution Order

```text
T001 -> T002 -> T003 -> T004 -> T005 -> T006 -> T007 -> T008 -> T009 -> T010 -> T011 -> T012 -> T013
```

## Requirement Coverage

| Requirement | Tasks |
| --- | --- |
| FR-001, FR-004 | T004-T006 |
| FR-002, FR-010 | T002, T005, T010 |
| FR-003 | T003, T006 |
| FR-005 | T003, T009-T010 |
| FR-006, FR-007, FR-008 | T007-T008 |
| FR-009 | T001, T009-T011 |
| SC-001-SC-004 | T006, T008, T010-T011 |

## Completion State

- Implementation and mapped tests: pending
- Independent Review: pending
- Converge: pending
- Fallback delta: pending final implementation audit
