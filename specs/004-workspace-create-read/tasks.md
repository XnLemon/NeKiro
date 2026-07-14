# Tasks: Workspace Create and Read

**Input**: Design documents from
`/specs/004-workspace-create-read/`

**Prerequisites**: `spec.md`, `plan.md`, `research.md`, `data-model.md`,
`contracts/workspace-create-read-api.md`, and `quickstart.md`.

**Scope**: Issue #4 only. Installation and exact resolution are inherited from
the dependent #3 branch and are not expanded by these tasks.

## Format

Tasks use `[ID] [P?] [Story]` and name the owning paths. Tests are scheduled
after the corresponding implementation verification, per the repository
constitution.

## Phase 1: Observe and Design Gate

- [X] T001 Read `AGENTS.md`, `.specify/memory/constitution.md`, issue #4,
  active Workspace v1/Northbound v3/Platform Error v3 contracts, and the
  dependent #3 implementation; record the ownership and compatibility
  boundary in `specs/004-workspace-create-read/research.md`.
- [X] T002 Approve the #4 user scenarios, functional requirements, non-goals,
  and zero-fallback policy in `specs/004-workspace-create-read/spec.md`.
- [X] T003 Produce the architecture, persistence, failure, contract, and test
  plan in `specs/004-workspace-create-read/plan.md`, plus the data model,
  contract guide, quickstart, and checklist.
- [X] T004 Run cross-artifact analysis over the #4 Spec, Plan, Tasks, active
  contracts, and constitution; record any high-impact conflict before code
  changes in `specs/004-workspace-create-read/tasks.md`.

## Phase 2: Workspace Runtime Verification and Implementation

**Goal**: A trusted caller can create and read one durable owner-controlled
Workspace through the Gateway.

- [X] T005 Verify or correct `workspace.Service.CreateWorkspace` and
  `GetWorkspace` in `apps/control-plane/internal/workspace/service.go` so
  owner assignment, identifier validation, policy ordering, duplicate
  conflict, and dependency errors match FR-001 through FR-007.
- [X] T006 Verify or correct the Workspace-owned PostgreSQL create/read path in
  `apps/control-plane/internal/workspace/postgres/store.go` and explicit
  schema/readiness behavior in
  `apps/control-plane/internal/workspace/postgres/migrations.go` and
  `apps/control-plane/cmd/control-plane/main.go` (FR-008 through FR-011).
- [X] T007 Verify or correct strict create/read HTTP adaptation in
  `apps/control-plane/internal/gateway/workspace_handler.go`, including
  authentication-before-body processing, active v3 status/error mapping,
  trace equality, and secret-safe failure bodies (FR-001, FR-006, FR-007,
  FR-012).

## Phase 3: Post-Implementation Tests

- [X] T008 [P] [US1] Add unit tests for trusted owner assignment, exact four
  fields, invalid identity/identifier, duplicate preservation, owner policy,
  unknown read, non-owner read, and injected store failures in
  `apps/control-plane/internal/workspace/service_test.go` (US1 scenarios 1-5;
  FR-001 through FR-007).
- [X] T009 [P] [US1] Add HTTP tests for create/read success, missing and
  rejected auth, duplicate/error mappings, unknown and non-owner paths,
  unknown body fields, duplicate JSON members, and trace behavior in
  `apps/control-plane/internal/gateway/workspace_handler_test.go` (US1
  scenarios 1-6; FR-001, FR-004, FR-006, FR-007, FR-012).
- [X] T010 [P] [US1] Add PostgreSQL integration evidence for durable create/read,
  duplicate conflict preservation, non-owner/unknown behavior, service
  reconstruction after closing and reopening the store pool, and injected
  query/commit failure in
  `apps/control-plane/internal/workspace/integration/workspace_test.go` (US1
  scenarios 1-8; FR-005, FR-008, FR-011).
- [X] T011 [P] [US1] Extend Workspace migration/readiness integration coverage
  for missing, stale, incomplete, and unavailable schema in
  `apps/control-plane/internal/workspace/postgres/migrations_integration_test.go`
  (US1 scenario 8; FR-009).

## Phase 4: Verification, Review, and Converge

- [X] T012 Run contract, unit, integration, race, vet, build, module-tidy, and
  diff checks from `quickstart.md`; record exact results and fallback delta in
  this file (SC-001 through SC-005).
- [X] T013 Run a fresh independent Review against the #4 Spec, Plan, Tasks,
  active contracts, implementation, tests, and constitution. Record all
  findings and do not treat passing tests as a substitute for Review.
- [X] T014 Resolve any High/Medium Review findings by updating the appropriate
  Spec/Tasks first, then code/tests; rerun the full verification and a fresh
  independent Review.
- [X] T015 Run Converge; append any remaining traceable work to this file and
  resolve it before marking issue #4 complete.
- [ ] T016 Update `docs/handoffs/CURRENT.md`, check every completed issue #4
  acceptance checkbox, commit with `Nene7ko_ <1604009816@qq.com>`, push the
  branch to `origin`, and create a ready PR targeting `main`.

## Dependency and Write Scope

All Phase 2 tasks share existing runtime files and therefore run serially.
Phase 3 test tasks are parallel only after Phase 2 is complete; each owns its
test file, though all integration tests use the explicitly serialized CI
database job. No task may modify Catalog tables, contracts, Installation
semantics, Router code, or Frontend code for #4.

## Cross-Artifact Analysis Evidence

The read-only analysis completed after task generation found no unresolved
constitution conflict, contradictory requirement, placeholder, or uncovered
acceptance story. The active contract is reused rather than duplicated, and
all runtime changes remain behind the existing Gateway, Workspace policy, and
Workspace store boundaries.

| Requirement set | Mapped tasks | Evidence target |
| --- | --- | --- |
| FR-001, FR-002, FR-003, FR-004 | T005, T007, T008, T009 | Trusted auth, exact create DTO, owner/timestamp assignment, identifier validation |
| FR-005, FR-006, FR-007 | T005, T008, T009, T010 | Conflict preservation, owner/not-found distinction, auth-before-service ordering |
| FR-008, FR-009, FR-010, FR-011 | T006, T010, T011 | Workspace schema ownership, readiness, policy boundary, durable reconstruction |
| FR-012, FR-013 | T007, T009, T012 | Active v3 mapping, trace/error safety, zero-fallback audit |
| SC-001 | T008, T009, T012 | Complete authenticated create/read workflow |
| SC-002 | T008, T009, T010, T012 | Exact invalid/auth/conflict/forbidden/not-found outcomes |
| SC-003 | T010, T012 | Restart-style durable field equality |
| SC-004 | T006, T011, T012 | Missing/stale/incomplete/unavailable readiness failure |
| SC-005 | T005-T012 | Four-field boundary and fallback delta |

**Analysis result**: PASS. Implementation may proceed to the existing runtime
verification tasks; no Spec, Plan, or Tasks rewrite is required before them.

## Implementation and Verification Evidence

T005-T011 are complete. The Workspace PostgreSQL create path returns the
database `RETURNING` values so the first response uses the same microsecond
timestamp fact that a later read returns. Readiness now rejects a version-1
schema whose Workspace table has missing or unexpected columns.

T012 verification passed:

```text
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./...
go mod tidy -diff
git diff --check
docker compose --file deploy/compose.yaml config --quiet
go test -tags=integration -count=1 ./apps/control-plane/internal/workspace/postgres
go test -tags=integration -count=1 ./apps/control-plane/internal/workspace/integration
```

The two schema-resetting PostgreSQL packages were run serially against the
dedicated `_test` database. Fallback delta remains `removed 0, retained 0,
added 0, net 0`; added fallback evidence is none.

## Independent Review Evidence

Three independent `ocr` passes were run against the current Workspace change.
The first two passes found only readiness gaps that were fixed: Workspace
column metadata/constraints, identifier collation, and timestamp precision.
The final pass reviewed 3 files and reported 0 comments. T013 and T014 are
complete; no High or Medium finding remains open.

## Converge Evidence

The post-review Converge audit found no remaining implementation gap against
the #4 Spec, Plan, Tasks, active contracts, or constitution. There are no
clarification markers, missing runtime paths, or untracked acceptance
requirements. No Convergence task was appended.
