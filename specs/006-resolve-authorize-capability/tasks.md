# Tasks: Resolve and Authorize an Installed Agent Capability

**Input**: Design documents from `specs/006-resolve-authorize-capability/`

**Tests**: Tests follow the approved implementation per the repository
constitution and map to Issue #6 acceptance scenarios and active contracts.

## Phase 1: Setup

- [X] T001 Record the baseline audit, active contract sources, and no-fallback classification in `specs/006-resolve-authorize-capability/research.md`.
- [X] T002 Create the Issue #6 specification quality checklist in `specs/006-resolve-authorize-capability/checklists/requirements.md`.

## Phase 2: Foundational Design

- [X] T003 [P] Define exact resolution entities and decision flow in `specs/006-resolve-authorize-capability/data-model.md`.
- [X] T004 [P] Document the active Internal v2 operation and failure mapping in `specs/006-resolve-authorize-capability/contracts/resolve-authorize-capability-api.md`.
- [X] T005 Generate the implementation plan and validation quickstart in `specs/006-resolve-authorize-capability/plan.md` and `specs/006-resolve-authorize-capability/quickstart.md`.
- [X] T006 Analyze Spec, Plan, active contracts, and Tasks for coverage, precedence, ownership, and fallback conflicts before implementation.

## Phase 3: User Story 1 - Resolve an Authorized Capability (P1)

- [X] T007 [US1] Establish Workspace existence before current Installation lookup in `apps/control-plane/internal/workspace/service.go`, preserving typed dependency and not-found errors.
- [X] T008 [US1] Add focused service coverage for Workspace/not-installed precedence, exact pin matching, disabled Installation short-circuiting, Catalog publication state, capability existence, permission containment, response-safe success, and dependency failures in `apps/control-plane/internal/workspace/service_test.go`.
- [X] T009 [US1] Reuse the existing PostgreSQL lifecycle/reconstruction evidence in `apps/control-plane/internal/workspace/integration/workspace_test.go`; no new integration migration or persistence behavior is needed for this minimal resolution correction.

## Phase 4: User Story 2 - Preserve Correlation and Failure Semantics (P1)

- [X] T010 [US2] Add internal HTTP tests for separate trusted internal authentication, fixed status/code/message mappings, pre-correlation omission, and post-correlation exact Invocation/root Task/Trace preservation in `apps/control-plane/internal/gateway/workspace_handler_test.go`.
- [X] T011 [US2] Add contract assertions for the active Internal v2 operation, security destination, approved response fields, and correlated/pre-correlation error shapes in `contracts/result_api_contracts_test.go` and `contracts/workspace_api_contracts_test.go`.
- [X] T012 [US2] Add dependency and forbidden/error precedence tests proving no stale, empty, or synthetic resolution success in `apps/control-plane/internal/workspace/service_test.go` and `apps/control-plane/internal/gateway/workspace_handler_test.go`.

## Phase 5: User Story 3 - Retain a Router-Safe Response Boundary (P2)

- [X] T013 [US3] Verify response serialization contains only active Card and resolved Installation contract fields and no secret, input/output payload, health, or dependency detail in `apps/control-plane/internal/gateway/workspace_handler_test.go`.
- [X] T014 [US3] Run focused `gofmt`, `go test ./contracts ./apps/control-plane/internal/workspace ./apps/control-plane/internal/gateway`, and `git diff --check`; the PostgreSQL-gated suite was not run because no dedicated test database was configured.

## Phase 6: Review and Convergence

- [X] T015 Review the final diff against Spec, Plan, Tasks, AGENTS.md, active contracts, ADR 0005, and fallback policy; no unrelated files or fallback additions remain.
- [X] T016 Run convergence against the completed artifacts, ensure every task is marked complete, and commit the complete Issue #6 branch with repository-local Git identity.

## Dependencies and Execution Order

Design tasks T001-T006 precede business-code changes. T007 precedes the
resolution tests. T010-T013 depend on the existing route and service behavior
plus T007. T014-T016 are final gates.

## Implementation Strategy

The MVP is the one-line precedence correction plus mapped evidence for the
active resolver. No public contract or storage migration changes are required.
The final branch is complete only when focused and broad verification is fresh,
integration limitations are stated, the fallback delta is reported, and the
repository-local commit identity is used.

## Evidence

- Focused tests passed: `go test ./contracts ./apps/control-plane/internal/workspace ./apps/control-plane/internal/gateway`.
- Formatting passed: `gofmt` on all modified Go files.
- Diff hygiene passed: `git diff --check`.
- PostgreSQL integration: `go test -tags=integration -count=1 ./apps/control-plane/internal/workspace/integration` passed against a disposable dedicated `_test` database; the resolver path was exercised through persisted Workspace/Installation reconstruction and Catalog disablement.
- Fallback delta: removed 1, retained 2, added 0, net -1. Added fallback evidence: none.
