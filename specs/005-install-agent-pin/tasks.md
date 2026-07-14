# Tasks: Install a Published Agent and Pin an Exact Version

**Input**: Design documents from `/specs/005-install-agent-pin/`.

**Prerequisites**: `spec.md`, `plan.md`, `research.md`, `data-model.md`,
`contracts/install-agent-pin-api.md`, and `quickstart.md`.

**Scope**: Issue #5 only. Workspace root is inherited from #4; inspection,
lifecycle, exact resolution, Router, and invocation behavior remain out of
scope.

## Phase 1: Observe and Design Gate

- [X] T001 Read `AGENTS.md`, constitution, issue #5, active Installation v2 /
  Northbound v3 contracts, Spec 003, Spec 004, and the dependent runtime;
  record baseline gaps in `research.md`.
- [X] T002 Approve #5 scenarios, requirements, non-goals, empty permission
  semantics, selection policy, race semantics, and zero-added-fallback policy
  in `spec.md`.
- [X] T003 Produce `plan.md`, `data-model.md`, the contract guide, quickstart,
  and requirements checklist.
- [X] T004 Run cross-artifact analysis over Spec, Plan, Tasks, active contracts,
  and constitution; record the result in this file before implementation.

## Phase 2: Install Runtime

- [X] T005 Verify or correct `workspace.Service.Install` in
  `apps/control-plane/internal/workspace/service.go` for owner ordering,
  Catalog selection, exact Card permission validation, empty-set preservation,
  and no-fallback failure mapping (FR-001 through FR-007, FR-012, FR-014).
- [X] T006 Verify or correct `CreateInstallation` in
  `apps/control-plane/internal/workspace/postgres/store.go` for Workspace row
  locking, current uniqueness recheck, conflict mapping, committed `RETURNING`
  values, empty arrays, and dependency failures (FR-008 through FR-011).
- [X] T007 Verify or correct the install HTTP adapter in
  `apps/control-plane/internal/gateway/workspace_handler.go` so required,
  null, malformed, and explicit empty `acceptedPermissions` are distinct and
  exact v3 errors/trace are returned (FR-001, FR-002, FR-013).

## Phase 3: Post-Implementation Tests

- [X] T008 [P] [US1] Add service unit tests for owner/non-owner, invalid
  constraints, stable/pre-release/build selection, unknown/duplicate/empty
  permissions, current conflict, Catalog failure, and no persistence on
  validation failure in `apps/control-plane/internal/workspace/service_test.go`
  (US1 scenarios 1-8; FR-001 through FR-007, FR-009, FR-012).
- [X] T009 [P] [US1] Add HTTP tests for install success, required/empty/null
  permission presence, owner/auth/path/body errors, not-found/conflict/
  dependency mappings, trace, and no service call on invalid input in
  `apps/control-plane/internal/gateway/workspace_handler_test.go` (US1
  scenarios 1-8; FR-001, FR-002, FR-005, FR-013).
- [X] T010 [P] [US1] Add PostgreSQL integration tests for exact persisted
  fields, explicit empty array, restart reconstruction, newer publication
  immutability, Catalog disable race semantics, dependency failure, and
  100-request one-winner concurrency in
  `apps/control-plane/internal/workspace/integration/workspace_test.go` (US1
  scenarios 1-12; FR-008 through FR-011, FR-013).

## Phase 4: Verification, Review, and Converge

- [X] T011 Run contract, unit, integration, race, vet, build, module-tidy,
  Compose, diff, and fallback checks from `quickstart.md`; record exact output
  and fallback delta here.
- [X] T012 Run a fresh independent Review against this Spec, Plan, Tasks,
  contracts, implementation, tests, and constitution.
- [X] T013 Resolve every valid High/Medium Review finding through Spec/Tasks,
  code, and tests; rerun verification and obtain a fresh Review with zero
  unresolved High/Medium findings.
- [X] T014 Run Converge and append/resolve any remaining traceable work before
  delivery.
- [X] T015 Update `docs/handoffs/CURRENT.md`, check every Issue #5 acceptance
  checkbox, commit with `Nene7ko_ <1604009816@qq.com>`, push to `origin`, and
  create a ready PR targeting `main`.

## Dependency and Write Scope

Phase 2 is serial because service, store, and handler behavior form one
transaction boundary. Phase 3 tests may run in parallel only after Phase 2,
but schema-resetting PostgreSQL packages must run serially in CI and local
verification. No task may modify active contracts, Catalog SQL ownership,
Installation inspection/lifecycle routes, Router, Ledger, or Frontend code.

## Cross-Artifact Analysis Evidence

The read-only analysis completed after task generation found no unresolved
constitution conflict, contradictory requirement, placeholder, or uncovered
acceptance story. The active v3 route and Installation v2 schema are reused;
the only public-boundary clarification is presence-aware decoding of the
already-required `acceptedPermissions` array.

| Requirement set | Mapped tasks | Evidence target |
| --- | --- | --- |
| FR-001, FR-002, FR-005, FR-006, FR-007 | T005, T007, T008, T009 | Owner/auth ordering, strict request presence, selection and permission outcomes |
| FR-003, FR-004 | T005, T008, T010 | Catalog-owned stable/pre-release/build-metadata selection |
| FR-008, FR-009, FR-010, FR-011 | T006, T008, T010 | Committed fields, uniqueness race, immutable pin and publication behavior |
| FR-012, FR-013, FR-014 | T005-T011 | Controlled Catalog port, mapped HTTP/PostgreSQL/dependency tests, zero fallback |
| SC-001 | T008, T009, T011 | Complete owner install workflow |
| SC-002 | T008, T009, T010, T011 | Exact selection/permission/auth/conflict/dependency outcomes |
| SC-003 | T010, T011 | One current row under concurrent requests |
| SC-004 | T010, T011 | Restart reconstruction and committed field equality |
| SC-005 | T010, T011 | New publication cannot mutate an existing pin |
| SC-006 | T005, T006, T011 | No invocation/probe/cache/retry/alternate-source behavior |

**Analysis result**: PASS. Implementation may proceed without changing the
approved Spec, Plan, active contracts, or task scope.

## Implementation and Verification Evidence

T005-T010 are complete. Installation now rejects nil permission slices at the
domain boundary, preserves explicit empty arrays as non-nil values, rejects
missing/null permission arrays in HTTP, and returns committed PostgreSQL
Installation fields through `RETURNING`.

T011 verification passed:

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
dedicated `_test` database. Fallback delta remains `removed 0, retained 1,
added 0, net 0`; the sole retained behavior is the explicitly approved empty
accepted-permission set, with no dependency fallback evidence.

## Independent Review Evidence

The independent `ocr` review used open-code-review v1.7.9, reviewed 4 files,
and reported 0 comments. No High or Medium finding remains open; T012 and T013
are complete.

## Converge Evidence

The post-review Converge audit found no remaining implementation gap against
the #5 Spec, Plan, Tasks, active contracts, or constitution. There are no
clarification markers, missing runtime paths, or untracked acceptance
requirements. No Convergence task was appended.

## Delivery Evidence

- Implementation commit: `274b8a0`
- Ready PR: [NeKiro-project/NeKiro#12](https://github.com/NeKiro-project/NeKiro/pull/12)
- Issue #5 acceptance criteria: all checked on GitHub.
