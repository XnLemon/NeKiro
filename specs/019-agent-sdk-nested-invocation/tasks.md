# Tasks: Agent SDK Nested Invocation

## Phase 1: SDD Gate

- [X] T000 Resolve how a nested `targetAgentId`/`capability` request obtains a
  deterministic installed Agent Card version and record the decision in the
  active versioned Control Plane contract before implementation.
- [X] T001 Observe AGENTS, ADR 0006, Agent Router v1, existing transport, Ledger read port, and DispatchHandler ownership in `specs/019-agent-sdk-nested-invocation/`.
- [X] T002 Specify the trusted context, agent binding, parent checks, child derivation, and no-fallback policy in `spec.md`.
- [X] T003 Plan SDK/Router ownership and test boundaries in `plan.md`, `research.md`, and `data-model.md`.
- [X] T004 Analyze the feature artifacts and active contracts before implementation.

## Phase 2: Foundational Boundary

- [ ] T005 [P] Add SDK platform-context validation and strict nested request/client types in `sdks/agent-sdk/`.
- [ ] T006 [P] Add Router agent binding authenticator and nested child derivation helpers in `apps/a2a-router/internal/nested/`.
- [ ] T007 Expose a trusted child-dispatch entry point from `apps/a2a-router/internal/api/dispatch_handler.go` that reuses existing resolution, transport, and Ledger semantics.

## Phase 3: User Story 1 - Trusted Nested Call

- [ ] T008 [US1] Implement authenticated Agent Router v1 handler, parent read, child derivation, and result/error adaptation in `apps/a2a-router/internal/api/agent_invocation_handler.go`.
- [ ] T009 [US1] Add handler/adapter tests for valid child JSON/SSE paths, parent-derived lineage, exact binding, and no content persistence in `apps/a2a-router/internal/api/agent_invocation_handler_test.go` and `apps/a2a-router/internal/nested/`.

## Phase 4: User Story 2 - Rejection and Isolation

- [ ] T010 [US2] Add strict request, auth-first, parent-state, mode, redirect, and zero-side-effect tests in `apps/a2a-router/internal/api/agent_invocation_handler_test.go` and `sdks/agent-sdk/client_test.go`.
- [ ] T011 [US2] Update active contract/config tests for Agent Router v1 binding and forbidden-field/content exclusion in `contracts/`.

## Phase 5: User Story 3 - Thin Runtime-Neutral SDK

- [ ] T012 [US3] Add SDK documentation and package-level runtime-neutral constraints in `sdks/agent-sdk/README.md`, then run package and repository verification.

## Phase 6: Review and Converge

- [ ] T013 Obtain independent Spec/Standards Review against AGENTS, ADR 0006, active contracts, and write scope.
- [ ] T014 Converge findings, update artifacts, rerun all tests/vet/race/diff checks, and record handoff evidence.

## Dependencies

```text
T000 -> T001-T004 -> [T005 || T006] -> T007 -> T008 -> [T009 || T010 || T011] -> T012 -> T013 -> T014
```

## Fallback Report

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
