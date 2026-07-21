# Tasks: Cross-Runtime Caller Sample

**Input**: Design documents from `/specs/020-cross-runtime-caller/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, and `quickstart.md`

**Tests**: Tests are scheduled after the approved implementation and map to the acceptance scenarios, failure semantics, and isolation requirements in Spec 020.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish the isolated sample module without changing the root platform module.

- [x] T001 Create the nested Runtime A module and pin `trpc.group/trpc-go/trpc-agent-go v1.10.0` and `github.com/a2aproject/a2a-go v0.3.15` only in `agents/runtime-a/go.mod` and `agents/runtime-a/go.sum` (FR-001, FR-002)
- [x] T002 [P] Add the Runtime A boundary README and explicit environment contract in `agents/runtime-a/README.md` (FR-004, FR-009)

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Define strict configuration and common validation before any user story implementation.

- [x] T003 Implement strict required environment parsing, URL/listener validation, identifier grammar, and byte-limit validation in `agents/runtime-a/config.go` (FR-004, SC-004)

**Checkpoint**: The nested module and startup boundary reject missing, blank, malformed, whitespace-padded, signed, fractional, and out-of-range settings without defaults.

## Phase 3: User Story 1 - Receive and compose a managed root invocation (Priority: P1) MVP

**Goal**: Runtime A accepts a valid active A2A JSON request and returns one deterministic combined result.

**Independent Test**: Send valid and invalid `message/send` requests to the Runtime A handler and verify exact combined output or explicit validation failure.

### Implementation for User Story 1

- [x] T004 [US1] Implement the `trpc-agent-go` Agent/Runner/Event execution and deterministic Runtime A result composition in `agents/runtime-a/runtime.go` (FR-002, FR-007)
- [x] T005 [US1] Implement the active `a2a-go` JSON-RPC handler, strict request/data-part validation, unsupported-operation errors, and process HTTP handler in `agents/runtime-a/handler.go` (FR-003, FR-007, FR-009)
- [x] T006 [US1] Add the explicit-configuration process entrypoint and listener startup in `agents/runtime-a/cmd/runtime-a/main.go` (FR-004)

### Tests for User Story 1

- [x] T007 [P] [US1] Add configuration and startup rejection tests for missing, blank, whitespace-padded, malformed, and out-of-range values in `agents/runtime-a/config_test.go` (US1/AC2, SC-004)
- [x] T008 [P] [US1] Add A2A request validation and deterministic byte-equivalent result tests in `agents/runtime-a/nested_test.go` (US1/AC1, US1/AC3, SC-001)

**Checkpoint**: Runtime A independently serves deterministic JSON root calls and rejects invalid configuration/input before nested execution.

## Phase 4: User Story 2 - Perform a Router-mediated nested call (Priority: P1)

**Goal**: Runtime A invokes Runtime B exactly once through the thin SDK and preserves exact context/lineage.

**Independent Test**: Use a local Router Agent v1 test server and verify the captured SDK request, credential, child result correlation, failure propagation, and absence of direct target URL requests.

### Implementation for User Story 2

- [x] T009 [US2] Add the SDK-backed nested invocation adapter, exact platform-header extraction, root PlatformContext construction, and child-result composition in `agents/runtime-a/nested.go` (FR-005, FR-006, FR-007, FR-008)
- [x] T010 [US2] Wire the nested adapter into the Runtime A Agent/Runner path without exposing framework types outside `agents/runtime-a/` in `agents/runtime-a/runtime.go` and `agents/runtime-a/handler.go` (FR-002, FR-006)

### Tests for User Story 2

- [x] T011 [P] [US2] Add Router test-server and root-to-Router end-to-end tests for exactly one SDK request, exact Agent Router v1 JSON shape, configured bearer credential, exact root/child correlation, and no direct Runtime B URL access in `agents/runtime-a/nested_test.go` and `agents/runtime-a/e2e_test.go` (US2/AC1, US2/AC2, SC-002, SC-003)
- [x] T012 [P] [US2] Add nested Router rejection, dependency, malformed result, and correlation-mismatch tests proving explicit failure and no retry/alternate result in `agents/runtime-a/nested_test.go` (US2/AC3, FR-009)

**Checkpoint**: Runtime A performs one managed child call through Router, and every accepted result has exact Workspace/root Task/Trace lineage.

## Phase 5: User Story 3 - Demonstrate runtime isolation (Priority: P2)

**Goal**: Prove that Runtime A and Runtime B share no Runtime-internal types or storage and remain safe under concurrency.

**Independent Test**: Build/test the nested module independently, inspect imports, run 100 concurrent calls, and scan outputs for forbidden content/secrets.

### Implementation for User Story 3

- [x] T013 [US3] Add runtime-boundary documentation and static import/content-exclusion assertions in `agents/runtime-a/README.md` and `agents/runtime-a/isolation_test.go` (FR-001, FR-002, FR-010, SC-006)

### Tests for User Story 3

- [x] T014 [P] [US3] Add a race-enabled 100-concurrent-call isolation test with distinct contexts/results and no shared Runtime B state in `agents/runtime-a/isolation_test.go` (US3/AC2, SC-005)
- [x] T015 [P] [US3] Add logs/result/content scans proving no credential, raw Router error, input, output, or Runtime-internal event data is emitted outside the returned deterministic payload in `agents/runtime-a/isolation_test.go` (FR-007, FR-010, SC-006)

## Phase 6: Polish & Cross-Cutting Concerns

- [ ] T016 [P] Run `gofmt`, `go vet`, `go test`, and `go test -race` from `agents/runtime-a`; record the exact commands and results in `specs/020-cross-runtime-caller/quickstart.md` (SC-001 through SC-006)
- [x] T017 Run the root `go test ./...` and repository diff/import checks to prove no platform module, contract, SDK, Runtime B, Compose, or CI changes were introduced (FR-001, FR-002)

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies; T001 blocks all implementation and T002 is parallel documentation.
- **Foundational (Phase 2)**: T003 depends on T001 and blocks all user stories.
- **User Story 1 (Phase 3)**: T004-T006 depend on T003; T007-T008 run only after their implementation tasks.
- **User Story 2 (Phase 4)**: T009-T010 depend on T004-T006; T011-T012 run only after nested implementation.
- **User Story 3 (Phase 5)**: T013 depends on T001-T012; T014-T015 run after T013.
- **Polish (Phase 6)**: T016-T017 depend on all implementation and mapped tests.

### Parallel Opportunities

- T002 can run with T003 after T001.
- T007 and T008 can run in parallel after T004-T006.
- T011 and T012 can run in parallel after T009-T010.
- T014 and T015 can run in parallel after T013.

### Requirement Coverage

| Requirement | Tasks |
| --- | --- |
| FR-001 | T001, T013, T017 |
| FR-002 | T001, T004, T010, T013, T017 |
| FR-003 | T005, T008 |
| FR-004 | T003, T006, T007 |
| FR-005 | T009, T011 |
| FR-006 | T009, T010, T011 |
| FR-007 | T004, T005, T009, T011, T015 |
| FR-008 | T009, T011 |
| FR-009 | T003, T005, T012 |
| FR-010 | T013-T017 |
| SC-001 | T008, T016 |
| SC-002 | T011, T016 |
| SC-003 | T011, T016 |
| SC-004 | T003, T007, T016 |
| SC-005 | T014, T016 |
| SC-006 | T013, T015-T017 |

## Implementation Strategy

1. Complete T001-T003 and verify the isolated configuration boundary.
2. Deliver User Story 1 as the MVP and run T007-T008.
3. Add the Router-mediated SDK call and run T011-T012.
4. Prove isolation/concurrency/content exclusion, then run all polish gates.
5. Only after local convergence, open the child PR and wait for CI before merge.

## Fallback Report

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
