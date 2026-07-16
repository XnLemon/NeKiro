# Tasks: Deterministic Direct A2A Sample

**Input**: Design documents from `specs/015-direct-a2a-sample/`

**Prerequisites**: `spec.md`, `research.md`, `data-model.md`, `plan.md`,
`quickstart.md`

**Tests**: Required after corresponding approved implementation.

**Organization**: Tasks are grouped by independently testable user story.

## Phase 1: Setup

- [X] T001 Create the Runtime B package and required-listener executable skeleton in `agents/runtime-b/server.go` and `agents/runtime-b/cmd/runtime-b/main.go`

---

## Phase 2: Foundational

- [X] T002 Implement strict structured fixture parsing and domain-separated deterministic identities in `agents/runtime-b/fixture.go` and `agents/runtime-b/identity.go`
- [X] T003 Implement mutex-protected process-local task snapshots, explicit transitions, clones, and history bounds in `agents/runtime-b/handler.go`

**Checkpoint**: Strict request and state foundations exist before operation behavior.

---

## Phase 3: User Story 1 - Invoke the Direct Callee (Priority: P1)

**Goal**: Deterministic JSON success and explicit failure through `message/send`.

**Independent Test**: Invoke success, failure, and invalid inputs through the official client/server path.

- [X] T004 [US1] Implement `message/send`, exact structured result, and distinct invalid/failure semantics in `agents/runtime-b/handler.go`
- [X] T005 [US1] Add mapped JSON success, failure, invalid-input, required-config, and concurrent identity tests in `agents/runtime-b/handler_test.go` and `agents/runtime-b/server_test.go`

---

## Phase 4: User Story 2 - Consume an Ordered Stream (Priority: P1)

**Goal**: Exact successful stream and explicit held-task cancellation.

**Independent Test**: Verify the five-event completed stream and same-task canceled stream.

- [X] T006 [US2] Implement `message/stream` success/hold fixtures and immutable completed/canceled terminal behavior in `agents/runtime-b/handler.go`
- [X] T007 [US2] Add mapped event-order, terminal uniqueness, cancellation, request-context termination, and race tests in `agents/runtime-b/handler_test.go` and `agents/runtime-b/server_test.go`

---

## Phase 5: User Story 3 - Inspect Active A2A Tasks (Priority: P1)

**Goal**: Conforming get/cancel operations and complete active-profile evidence.

**Independent Test**: Query/cancel created tasks, exercise task errors, and verify all operations and context headers.

- [X] T008 [US3] Implement strict `tasks/get`, `tasks/cancel`, unsupported-operation behavior, and HTTP handler assembly in `agents/runtime-b/handler.go` and `agents/runtime-b/server.go`
- [X] T009 [US3] Add mapped get/cancel/history/error, all-four-operation, SSE framing, and five-context-header conformance tests in `agents/runtime-b/handler_test.go` and `agents/runtime-b/server_test.go`

---

## Phase 6: Verification and Handoff

- [X] T010 Run formatting, vet, package tests, race tests, and repository tests; record zero-fallback and write-scope evidence in `specs/015-direct-a2a-sample/tasks.md`
- [X] T011 Obtain fresh independent Review against Spec, Plan, Tasks, active contracts, and constitution; return findings to Spec/Tasks before fixes
- [X] T012 Run Converge after Review and append/resolve any remaining implementation tasks

## Dependencies & Execution Order

```text
T001 -> T002 -> T003 -> T004 -> T005 -> T006 -> T007 -> T008 -> T009 -> T010 -> T011 -> T012
```

Tests follow approved implementation. This narrow slice has one implementation
owner and overlapping `handler.go` behavior, so no within-branch task is marked
parallel. Independent Review and Converge remain root-owned gates.

## Requirement Coverage

| Requirement | Tasks |
| --- | --- |
| FR-001, FR-002, FR-009, FR-010 | T001, T008, T009 |
| FR-003, FR-004 | T002, T004, T005 |
| FR-005, FR-006 | T003, T006, T007 |
| FR-007, FR-008 | T003, T008, T009 |
| FR-011 | T005, T007, T009, T010 |
| FR-012 | T002-T010 fallback inventory |
| SC-001-SC-005 | T005, T007, T009, T010 |

## Completion State

- Implementation and mapped tests: T001-T009 complete in the WIP branch
- Independent Review: intentionally pending for a non-implementing agent
- Converge: intentionally pending until Review completes
- Fallback delta checkpoint: removed `0`, retained `0`, added `0`, net `0`
- Final completion: Spec 015 implementation, verification, Review, and Converge complete in this branch

## Verification Checkpoint

2026-07-16 checkpoint:

- `.specify/feature.json` now points to `specs/015-direct-a2a-sample`.
- Non-race verification passed:
  - `gofmt -l agents/runtime-b` returned no files requiring formatting
  - `go test ./agents/runtime-b ./agents/runtime-b/cmd/runtime-b`
  - `go test ./...`
  - `go vet ./...`
  - `git diff --check`
- Fallback/write-scope scan found no platform database access, Control Plane,
  Router, Ledger, SDK, retry, cache, alternate route, compatibility fallback,
  or platform-core dependency in runtime code. The only non-test keyword hits
  were explicit `switch default` invalid-fixture branches and the
  `JSON-compatible` validation message.
- Windows race verification remained unavailable:
  - with default CGO settings: `go: -race requires cgo`
  - with `CGO_ENABLED=1`: `cgo: C compiler "gcc" not found`
  - `where gcc`, `where clang`, and `where cl` found no C compiler on PATH
  - observed Windows toolchain: `go version go1.26.3 windows/amd64`,
    `CGO_ENABLED=0`, `CC=gcc`
- WSL race verification passed without changing system configuration:
  - WSL toolchain: `go version go1.26.0 linux/amd64`, `CGO_ENABLED=1`,
    `CC=x86_64-linux-gnu-gcc`, `/usr/bin/gcc`
  - `go test -race ./agents/runtime-b` passed under Ubuntu-26.04

T011 independent Review and T012 Converge are complete. Review R1 returned two
P2 findings; T013 and T014 record and close those findings. Review R2 returned
PASS with no remaining P0-P2 findings.

## Review and Converge Checkpoint

2026-07-16 session note:

- T011 independent Review R1 returned FAIL with two P2 findings:
  - Runtime B task/message snapshots were shallow-cloned enough that returned
    task history or original request data could mutate stored task history.
  - The documented `historyLength` greater-than-available-history invalid
    params edge case lacked a regression test.
- T012 Converge appended and resolved T013/T014 below.
- T011 independent Review R2 returned PASS with no remaining P0-P2 findings.
- Fallback delta for the convergence fix: removed `0`, retained `0`, added
  `0`, net `0`. Added fallback evidence: none.

## Phase 7: Convergence

- [X] T013 [Review] Deep-clone Runtime B task/message snapshots in `agents/runtime-b/handler.go` and add a regression test proving mutations to an original request or returned `tasks/get` snapshot cannot alter stored task history.
- [X] T014 [Review] Add a Runtime B history-bounds regression test for `historyLength` greater than available history, asserting the documented invalid-params behavior.
