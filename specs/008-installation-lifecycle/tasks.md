# Tasks: Installation Lifecycle

**Input**: Design documents from `/specs/008-installation-lifecycle/`.

**Scope**: Issue #8 only. Active contracts and Catalog behavior are reused.

## Phase 1: Observe and Design Gate

- [X] T001 Read governing architecture, active v3 contracts, and Spec 003/004/005; record the existing lifecycle/store/HTTP baseline and missing evidence in `research.md`.
- [X] T002 Freeze owner policy, transition graph, terminal/reinstall policy, error distinctions, no-Catalog boundary, concurrency semantics, and zero-added-fallback policy in `spec.md`.
- [X] T003 Produce `plan.md`, `data-model.md`, the lifecycle contract guide, `quickstart.md`, and requirements checklist.
- [X] T004 Analyze Spec, Plan, Tasks, active contracts, and constitution before code edits; record the result below.

## Phase 2: Runtime Audit and Corrections

- [X] T005 Audit `workspace.Service.UpdateInstallation` and `Uninstall` for owner ordering, strict targets, exact error propagation, immutable facts, and no Catalog call; correct only evidence-backed gaps.
- [X] T006 Audit `workspace/postgres.Store` and migrations for row locks, atomic committed timestamps, terminal preservation, partial uniqueness, cross-Workspace not-found, and dependency mapping; correct only evidence-backed gaps.
- [X] T007 Audit active v3 PATCH/DELETE routes and contract declarations for strict input, owner/auth/error/Trace mapping, terminal response, and secret exclusion; correct only evidence-backed gaps.

## Phase 3: Post-Implementation Tests

- [X] T008 [P] [US1] Add mapped service tests for the complete transition table, same-state/terminal conflicts, timestamps, immutable fields, reinstall-new-ID, owner, identity, and dependency outcomes.
- [X] T009 [P] [US2] Add mapped HTTP tests for PATCH/DELETE success, malformed/unknown body and target, unauthenticated, forbidden, not-found/cross-Workspace, conflict, dependency, Trace, and secret exclusion outcomes.
- [X] T010 [P] [US1] Add contract tests for active Northbound v3 lifecycle paths, target enum, terminal response, and exact mapped error sets.
- [X] T011 [US3] Add PostgreSQL lifecycle integration coverage for constraints, committed timestamp facts, restart reconstruction, terminal history, reinstall identity, and dependency failures.
- [X] T012 [US3] Add PostgreSQL concurrent lifecycle/install race coverage proving row-lock serialization, legal histories, one current row, and no false success.

## Phase 4: Verification, Review, and Converge

- [X] T013 Run focused and broad static/unit/race checks, dedicated PostgreSQL integration when configured, Compose config validation, and `git diff --check`; record exact evidence here.
- [X] T014 Perform an independent review against Spec, Plan, Tasks, contracts, implementation, tests, and constitution; resolve any High/Medium findings.
- [X] T015 Run Converge; append and resolve only traceable remaining work.
- [X] T016 Confirm tasks are marked complete, verify repository Git identity, commit the complete branch with a focused message, and report hash, files, tests, SDD status, fallback delta, and integration limitations.
- [X] T017 [Review-R1] Fix the lock-contention timestamp regression identified
  by independent review: compute a strictly monotonic committed lifecycle time
  inside the locked Workspace store transaction, add unit and PostgreSQL
  coverage, and update the Spec/Plan evidence before implementation.

## Dependency and Write Scope

Phase 2 is serial because service, store, migration, and HTTP behavior share
one lifecycle contract. Phase 3 tests follow implementation and may be split
by file, but PostgreSQL schema-resetting packages run serially. No task changes
Catalog storage, Router, Ledger, frontend, or active contract versions.

## Cross-Artifact Analysis Evidence

The analysis completed after task generation found no unresolved constitution
conflict, requirement without a mapped task, or contract ambiguity. Existing
active Northbound v3 lifecycle operations are reused. Runtime code already
contains the core transition logic; the Issue #8 implementation gap is focused
proof for every acceptance path plus any defect exposed by that evidence.

| Requirement set | Mapped tasks | Evidence |
| --- | --- | --- |
| FR-001 through FR-006 | T005, T006, T008, T011 | owner policy, transition graph, terminal preservation, reinstall |
| FR-007 | T007, T009, T010 | distinct HTTP/service outcomes and Trace |
| FR-008, FR-010, FR-011 | T006, T011, T012, T017 | locks, constraints, one-current race, restart, monotonic committed timestamps |
| FR-009 | T005, T007, T009 | no Catalog call, no fallback, no secret leakage |
| SC-001 through SC-005 | T008-T013 | mapped test and verification evidence |

**Analysis result**: PASS. Implementation may proceed without changing active
contract versions or expanding scope.

## Implementation and Verification Evidence

### Formatting and Focused Verification

- `gofmt` on the lifecycle Gateway, service/store tests, integration tests, and PostgreSQL store completed successfully.
- `go test -count=1 ./contracts` passed.
- `go test -count=1 ./apps/control-plane/internal/workspace` passed.
- `go test -count=1 ./apps/control-plane/internal/gateway` passed.
- `go test -count=1 ./apps/control-plane/internal/workspace/postgres` passed.
- `go test -race -count=1 ./apps/control-plane/internal/workspace` passed.
- `go test -race -count=1 ./apps/control-plane/internal/gateway` passed.
- `git diff --check` passed.

### Integration and Review Limits

- PostgreSQL: `go test -tags=integration -count=1 ./apps/control-plane/internal/workspace/postgres ./apps/control-plane/internal/workspace/integration` passed serially against a disposable dedicated `_test` database, including stale-candidate timestamp regression coverage.
- Compose validation passed with explicit local values; PowerShell prerequisite execution remains Windows-only and was not run on this macOS host.
- Independent review by Galileo found the stale timestamp issue and an undersized #7 pagination corpus; T017 and the 101-row inspection test resolved both findings.
- Converge found no remaining Issue #8 behavior or artifact gap after remediation.

### Fallback Report

```text
Fallback delta: removed 0, retained 1, added 0, net 0
Added fallback evidence: none
```
