# Tasks: Minimal Workspace and Installation

**Input**: Design documents from
`/specs/003-workspace-installation-contracts/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`,
`contracts/workspace-installation-api.md`, `quickstart.md`

**Tests**: Tests are required and are scheduled after the corresponding
approved implementation. Every test task references Spec acceptance scenarios,
failure semantics, compatibility requirements, or measurable outcomes.

**Organization**: Tasks are grouped by user story. Workspace root and
install/pin are the serial trust foundation. Inspection, lifecycle, and internal
resolution then use disjoint files and may run as three parallel slices.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel because write paths are disjoint and prerequisites
  are complete.
- **[Story]**: Maps the task to one user story in `spec.md`.
- Every task names exact repository paths.

## Phase 1: Contract Gate Intake

**Purpose**: Treat issue #3 artifacts as the accepted behavior source before
runtime implementation.

- [ ] T001 Verify `go test -count=1 ./contracts`, `go vet ./contracts`, and
  `git diff --check` pass against the frozen Workspace v1, Installation v2,
  Northbound v3, Control Plane Internal v2, and versioned Platform Error sources; record
  the exact baseline commit and results in
  `specs/003-workspace-installation-contracts/tasks.md` (FR-030)
- [ ] T002 Re-read `AGENTS.md`, `.specify/memory/constitution.md`,
  `specs/003-workspace-installation-contracts/spec.md`,
  `specs/003-workspace-installation-contracts/plan.md`, and
  `docs/decisions/0005-minimal-workspace-installation-boundary.md`; record any
  contradiction in `specs/003-workspace-installation-contracts/tasks.md` and
  stop implementation until the higher-order artifact is corrected (FR-026,
  FR-029)
- [ ] T003 Confirm repository-local Git identity is
  `Nene7ko_ <1604009816@qq.com>` and record the clean implementation starting
  point in `specs/003-workspace-installation-contracts/tasks.md`

**Checkpoint**: Active contracts and architecture are accepted; runtime work may
start without changing public behavior concurrently.

---

## Phase 2: Shared Workspace Foundation

**Purpose**: Add data ownership, ports, migration, and shared values required by
all user stories.

**CRITICAL**: No user-story implementation starts until this phase completes.

- [ ] T004 Add forward-only Workspace-owned tables, state/timestamp checks,
  non-cascading parent relation, partial one-current-Installation uniqueness,
  and list/resolution indexes in
  `apps/control-plane/migrations/003_workspace.sql` (FR-002, FR-012, FR-015,
  FR-026)
- [ ] T005 [P] Add Workspace, Installation, status, authenticated caller, exact
  Catalog result, and operation error domain values without infrastructure or
  secret fields in `apps/control-plane/internal/workspace/model.go` and
  `apps/control-plane/internal/workspace/errors.go` (FR-002, FR-013, FR-029)
- [ ] T006 [P] Define the complete Workspace store, Catalog read, owner policy,
  identifier/time generator, and internal resolution ports before parallel
  slices in `apps/control-plane/internal/workspace/store.go`,
  `apps/control-plane/internal/workspace/catalog.go`, and
  `apps/control-plane/internal/workspace/policy.go` (FR-003, FR-019, FR-026)
- [ ] T007 Implement exact owner-only authorization with no inferred owner,
  default principal, or Membership/RBAC behavior in
  `apps/control-plane/internal/workspace/policy.go` (FR-001, FR-003, FR-028)
- [ ] T008 Add the Workspace PostgreSQL adapter constructor, transaction helper,
  expected-conflict classifier, and schema readiness checks without Catalog SQL
  or in-memory fallback in
  `apps/control-plane/internal/workspace/postgres/store.go` and
  `apps/control-plane/internal/workspace/postgres/migrations.go` (FR-024,
  FR-026, FR-028)
- [ ] T009 Wire the command layer to invoke the Catalog-owned and
  Workspace-owned migrators separately and compose both readiness checks in
  `apps/control-plane/cmd/control-plane/main.go` while keeping serving
  migration-free, each module's schema ownership intact, and public migration
  forward-only (FR-024, FR-026, FR-028)

**Checkpoint**: Workspace owns durable schema and stable ports; story files can
implement behavior without changing shared interfaces.

---

## Phase 3: User Story 1 - Establish a Workspace Boundary (Priority: P1)

**Goal**: A trusted creator creates and reads one durable owner-controlled
Workspace.

**Independent Test**: Create/read as owner, reject a non-owner, reject duplicate
creation, restart the service, and read the identical four-field Workspace.

### Implementation for User Story 1

- [ ] T010 [US1] Implement strict create/read ordering, immutable trusted owner,
  duplicate conflict, and owner policy calls in
  `apps/control-plane/internal/workspace/root.go` (FR-001 through FR-004,
  FR-024, FR-025)
- [ ] T011 [US1] Implement transactional Workspace insert and exact read with
  expected primary-key conflict mapping in
  `apps/control-plane/internal/workspace/postgres/root.go` (FR-001, FR-002,
  FR-004, FR-026)
- [ ] T012 [US1] Implement strict JSON/path decoding, Bearer authentication,
  Trace equality, fixed errors, and Workspace create/read responses in
  `apps/control-plane/internal/gateway/workspace_root_handler.go` (FR-001,
  FR-024, FR-025)

### Tests After User Story 1 Implementation

- [ ] T013 [P] [US1] Add post-implementation owner policy, exact field,
  duplicate create, non-owner, invalid identifier, error precedence, and
  dependency tests in `apps/control-plane/internal/workspace/root_test.go` and
  `apps/control-plane/internal/workspace/policy_test.go` (US1 scenarios 1-4;
  FR-001 through FR-004, FR-024)
- [ ] T014 [P] [US1] Add real PostgreSQL migration, insert/read, duplicate race,
  owner immutability, restart, and no-Catalog-table-access tests in
  `apps/control-plane/internal/workspace/postgres/root_integration_test.go`
  (US1 scenarios 1-4; SC-005)
- [ ] T015 [US1] Add authenticated HTTP create/read, Trace header/body equality,
  unknown/duplicate JSON, forbidden, not-found, conflict, dependency, and secret
  exclusion tests in
  `apps/control-plane/internal/gateway/workspace_root_handler_test.go` (US1
  scenarios 1-4; FR-024)

**Checkpoint**: Workspace is a durable authorization root independently usable
before any Agent is installed.

---

## Phase 4: User Story 2 - Install and Pin an Agent Version (Priority: P1)

**Goal**: An owner installs one deterministic currently published exact version
and immutable accepted-permission snapshot.

**Independent Test**: Resolve stable/pre-release/build-metadata candidates,
persist one exact pin and permission subset, reject invalid inputs and duplicate
current installs, then publish a newer version and prove the pin is unchanged.

### Implementation for User Story 2

- [ ] T016 [US2] Extend Catalog's controlled domain/store interface for
  published candidate selection and exact state reads in
  `apps/control-plane/internal/catalog/model.go` and
  `apps/control-plane/internal/catalog/store.go` without exposing PostgreSQL
  types (FR-006 through FR-009, FR-026)
- [ ] T017 [US2] Implement strict SemVer constraint validation, branch-specific
  pre-release eligibility, precedence ordering, and bytewise exact-version
  tie-break in `apps/control-plane/internal/catalog/resolution.go` (FR-005,
  FR-006, FR-007, FR-008, FR-009, FR-027)
- [ ] T018 [US2] Implement published-candidate and exact-version queries with
  exact Card preservation and explicit dependency/not-found state in
  `apps/control-plane/internal/catalog/postgres/resolution.go` (FR-006,
  FR-022, FR-026 through FR-028)
- [ ] T019 [US2] Implement install operation ordering, owner authorization,
  current conflict, Catalog selection, exact permission-subset validation,
  canonical sorting, immutable pin creation, and no retry/upgrade behavior in
  `apps/control-plane/internal/workspace/install.go` (FR-005 through FR-013,
  FR-025 through FR-028)
- [ ] T020 [US2] Implement Workspace-locked Installation insert and expected
  partial-unique race mapping in
  `apps/control-plane/internal/workspace/postgres/install.go` (FR-012, FR-013,
  FR-025 through FR-027)
- [ ] T021 [US2] Implement strict install request decoding, owner Bearer/Trace,
  `201` response, and exact validation/forbidden/not-found/conflict/dependency
  mappings in `apps/control-plane/internal/gateway/workspace_install_handler.go`
  (FR-005, FR-010, FR-024, FR-025)

### Tests After User Story 2 Implementation

- [ ] T022 [P] [US2] Add post-implementation stable, wildcard, hyphen, OR,
  invalid, whitespace, parser-boundary, pre-release branch, SemVer precedence,
  build-metadata tie, and dependency tests in
  `apps/control-plane/internal/catalog/resolution_test.go` (US2 scenarios 1-4;
  FR-005 through FR-009; SC-002)
- [ ] T023 [P] [US2] Add permission subset, empty set, unknown/duplicate ID,
  canonical order, operation precedence, immutable pin, Catalog-disable race,
  no retry, and no-upgrade tests in
  `apps/control-plane/internal/workspace/install_test.go` (US2 scenarios 5-8;
  FR-010, FR-011, FR-025, FR-027, FR-028; SC-003)
- [ ] T024 [US2] Add real PostgreSQL 100-way same-Agent install race, one-current
  uniqueness, rollback, restart, and exact snapshot tests in
  `apps/control-plane/internal/workspace/postgres/install_integration_test.go`
  (US2 scenario 7; FR-012, FR-013; SC-004, SC-005)
- [ ] T025 [US2] Add authenticated HTTP install success plus exact invalid,
  unauthenticated, non-owner, no-match, duplicate, Catalog failure, persistence
  failure, and Trace/secret tests in
  `apps/control-plane/internal/gateway/workspace_install_handler_test.go` (US2
  scenarios 1-8; FR-024, FR-025; SC-006)

**Checkpoint**: `Discover -> Install` is runnable and produces one durable exact
authorization pin. Inspection, lifecycle, and resolution may now run in
parallel.

---

## Phase 5A: User Story 3 - Inspect Installation Facts (Priority: P2)

**Goal**: An owner reads one Installation and lists all current and historical
facts in deterministic order.

**Independent Test**: Read/list enabled, disabled, and uninstalled records;
prove genuine empty list and distinct missing/forbidden/dependency failures.

**Exclusive parallel write range**:
`workspace/inspection*`, `workspace/postgres/inspection*`,
`gateway/workspace_inspection_handler*`.

### Implementation for User Story 3

- [ ] T026 [P] [US3] Implement owner-authorized exact read and stable bounded
  list behavior with required explicit limit, opaque cursor binding, and a
  genuine empty array only after successful lookup,
  in `apps/control-plane/internal/workspace/inspection.go` (FR-017, FR-018,
  FR-024, FR-025, FR-028)
- [ ] T027 [US3] Implement current/historical exact read and bounded keyset
  pagination using `installed_at ASC, installation_id ASC` in
  `apps/control-plane/internal/workspace/postgres/inspection.go` (FR-017,
  FR-018, FR-026)
- [ ] T028 [US3] Implement strict owner HTTP read/list adapters with bounded
  `limit`/opaque `cursor`, Trace, and exact validation/auth/forbidden/not-found/
  dependency mappings in
  `apps/control-plane/internal/gateway/workspace_inspection_handler.go`
  (FR-017, FR-018, FR-024)

### Tests After User Story 3 Implementation

- [ ] T029 [P] [US3] Add post-implementation complete fact, bounded page,
  cursor continuation/filter mismatch, stable order, empty, non-owner, missing,
  cross-Workspace Installation, and dependency tests
  in `apps/control-plane/internal/workspace/inspection_test.go` (US3 scenarios
  1-4; FR-017, FR-018, FR-024, FR-028)
- [ ] T030 [US3] Add real PostgreSQL current/history ordering, bounded keyset
  cursor, empty result, restart, and injected query-failure tests in
  `apps/control-plane/internal/workspace/postgres/inspection_integration_test.go`
  (US3 scenarios 1-4; SC-005, SC-006)
- [ ] T031 [US3] Add authenticated HTTP read/list response, pagination,
  empty array, Trace equality, exact failure, and secret exclusion tests in
  `apps/control-plane/internal/gateway/workspace_inspection_handler_test.go`
  (US3 scenarios 1-4; FR-024)

**Checkpoint**: Owners can inspect exact current and historical authorization
facts without lifecycle or Router behavior.

---

## Phase 5B: User Story 4 - Manage Installation Lifecycle (Priority: P2)

**Goal**: An owner applies only legal enable/disable/uninstall transitions and
reinstalls after preserved terminal history.

**Independent Test**: Exercise every transition and race, verify timestamps and
immutable fields, then reinstall with a new ID.

**Exclusive parallel write range**:
`workspace/lifecycle*`, `workspace/postgres/lifecycle*`,
`gateway/workspace_lifecycle_handler*`.

### Implementation for User Story 4

- [ ] T032 [P] [US4] Implement the exact transition table, immutable pin checks,
  same-state/direct-uninstall conflict, terminal history, and fresh reinstall
  semantics in `apps/control-plane/internal/workspace/lifecycle.go` (FR-014
  through FR-016, FR-024, FR-025)
- [ ] T033 [US4] Implement row-locked enable/disable/uninstall transitions with
  atomic timestamp updates and partial-unique slot release in
  `apps/control-plane/internal/workspace/postgres/lifecycle.go` (FR-012,
  FR-014 through FR-016, FR-026)
- [ ] T034 [US4] Implement strict PATCH/DELETE owner adapters returning the
  updated/preserved Installation and exact conflict/dependency mappings in
  `apps/control-plane/internal/gateway/workspace_lifecycle_handler.go` (FR-014,
  FR-015, FR-024)

### Tests After User Story 4 Implementation

- [ ] T035 [P] [US4] Add post-implementation complete transition table,
  immutable fields, timestamp, same-state, enabled-uninstall, terminal mutation,
  and reinstall-new-ID tests in
  `apps/control-plane/internal/workspace/lifecycle_test.go` (US4 scenarios 1-6;
  FR-014 through FR-016)
- [ ] T036 [US4] Add real PostgreSQL competing lifecycle/install races,
  impossible-state constraint, atomic uniqueness-slot release, history, and
  restart tests in
  `apps/control-plane/internal/workspace/postgres/lifecycle_integration_test.go`
  (US4 scenario 7; FR-012, FR-014 through FR-016; SC-004, SC-005)
- [ ] T037 [US4] Add authenticated PATCH/DELETE success, body/path validation,
  owner, not-found, every conflict, dependency, Trace, and secret exclusion
  tests in
  `apps/control-plane/internal/gateway/workspace_lifecycle_handler_test.go`
  (US4 scenarios 1-7; FR-024, FR-025; SC-006)

**Checkpoint**: Installation lifecycle is durable and explicit; no operation
silently upgrades, deletes, or resurrects a pin.

---

## Phase 5C: User Story 5 - Resolve Authorized Exact Agent Facts (Priority: P2)

**Goal**: A trusted internal caller receives a Card only for one matching,
enabled, currently published, capability-authorized Installation.

**Independent Test**: Resolve success, then vary exact version, state, Catalog
state, capability, permissions, identity, correlation, and dependencies.

**Exclusive parallel write range**:
`workspace/resolution*`, `workspace/postgres/resolution*`,
`gateway/internal_resolution_handler*`, and internal-auth config fields/tests.

### Implementation for User Story 5

- [ ] T038 [P] [US5] Implement exact Installation lookup, version/state
  precedence, controlled Catalog exact read, capability existence, permission
  containment, and no-Card failure responses in
  `apps/control-plane/internal/workspace/resolution.go` (FR-019 through FR-023,
  FR-025 through FR-028)
- [ ] T039 [US5] Implement indexed current exact Installation lookup without
  historical fallback in
  `apps/control-plane/internal/workspace/postgres/resolution.go` (FR-019,
  FR-020, FR-026, FR-028)
- [ ] T040 [US5] Add separately required internal auth mode/principal digest
  configuration with no Northbound-principal inheritance or default in
  `apps/control-plane/internal/config/config.go` (FR-019, FR-024, FR-028)
- [ ] T041 [US5] Implement bounded strict correlation decode, dedicated internal
  authentication, explicit pre-correlation error DTO, exact post-correlation
  request/header correlation, success DTO, and exact 400/401/403/404/503 errors in
  `apps/control-plane/internal/gateway/internal_resolution_handler.go` (FR-019,
  FR-020, FR-021, FR-022, FR-023, FR-024)

### Tests After User Story 5 Implementation

- [ ] T042 [P] [US5] Add post-implementation exact success, version mismatch,
  uninstalled, Installation disabled, Catalog disabled, missing capability,
  missing permission, dependency precedence, and no-Card failure tests in
  `apps/control-plane/internal/workspace/resolution_test.go` (US5 scenarios 1-6;
  FR-019 through FR-023; SC-003, SC-006)
- [ ] T043 [P] [US5] Add missing/blank/malformed/duplicate internal principal,
  Northbound/internal credential separation, constant-time digest, and secret
  redaction tests in `apps/control-plane/internal/config/config_test.go` and
  `apps/control-plane/internal/gateway/internal_resolution_handler_test.go`
  (US5 scenario 6; FR-019, FR-024, FR-028)
- [ ] T044 [US5] Add real PostgreSQL exact lookup plus internal HTTP correlation,
  status/capability matrix, Catalog-disable-after-install, injected dependency,
  and restart tests in
  `apps/control-plane/internal/workspace/postgres/resolution_integration_test.go`
  and `tests/integration/workspace/resolution_test.go` (US5 scenarios 1-6;
  FR-019 through FR-023, FR-027; SC-005, SC-006)

**Checkpoint**: The pre-dispatch trust boundary is complete without invoking or
deploying an Agent.

---

## Phase 6: Integration, Operations, and Cross-Story Acceptance

**Purpose**: Compose disjoint slices, validate the full owner workflow, and keep
runtime scope/failure policy aligned with the frozen gate.

- [ ] T045 Wire Workspace root/install/inspection/lifecycle routes and the
  separately authenticated internal resolution route into
  `apps/control-plane/internal/gateway/catalog_handler.go` and
  `apps/control-plane/cmd/control-plane/main.go` only after Phases 5A-5C merge;
  do not change frozen public contracts (FR-024, FR-030)
- [ ] T046 [P] Document explicit internal auth configuration, migration,
  readiness, owner workflow, persistence, and failure recovery in
  `.env.example`, `docs/runbooks/local-development.md`, and `README.md` without
  raw/default credentials or localhost fallback (FR-024, FR-028, FR-029)
- [ ] T047 [P] Add PostgreSQL Workspace services and integration-tagged suites
  to `.github/workflows/ci.yml` while preserving existing Catalog, frontend,
  pnpm lockfile, and `minimumReleaseAge` policy (SC-004, SC-005)
- [ ] T048 Add complete HTTP `Create -> Read -> Install -> Inspect -> Disable ->
  Enable -> Disable -> Uninstall -> Reinstall` owner/non-owner acceptance and
  both Runtime-neutral Catalog fixtures in
  `tests/integration/workspace/workspace_test.go` (US1-US4; FR-001 through
  FR-018; SC-001, SC-005, SC-006)
- [ ] T049 Add cross-story 100-request install/lifecycle race, permission matrix,
  deterministic selector matrix, dependency injection, restart, exact internal
  resolution, and no-output/secret contract acceptance in
  `tests/integration/workspace/workspace_test.go` (US2-US5; FR-005 through
  FR-029; SC-002 through SC-006)
- [ ] T050 Run every current command in
  `specs/003-workspace-installation-contracts/quickstart.md`, including contract,
  default, integration, race, vet, build, module-tidy, Compose, and diff checks;
  record exact evidence in
  `specs/003-workspace-installation-contracts/tasks.md`
- [ ] T051 Audit fallback and scope with `rg` across
  `apps/control-plane/internal/workspace`,
  `apps/control-plane/internal/catalog`,
  `apps/control-plane/internal/gateway`, `contracts`, and `deploy`; record in
  `specs/003-workspace-installation-contracts/tasks.md` the required delta
  removed `0`, retained `1`, added `0`, net `0`, added evidence `none`, and prove
  no stale source, retry, default owner/range, auto-upgrade, K8s, Membership/RBAC,
  Policy Hook, Invocation, Ledger, Agent Runtime, or Frontend behavior was added
  (FR-028, FR-029; SC-007)

---

## Phase 7: Independent Review and Converge

**Purpose**: Require an implementation-independent assessment and close every
remaining accepted gap before delivery.

- [ ] T052 Create a fresh Review Agent that did not implement this feature to
  review the complete diff against `AGENTS.md`,
  `specs/003-workspace-installation-contracts/spec.md`,
  `specs/003-workspace-installation-contracts/plan.md`,
  `specs/003-workspace-installation-contracts/tasks.md`, active contracts, and
  `docs/decisions/0005-minimal-workspace-installation-boundary.md`; record
  High/Medium/Low findings and explicit PASS/FAIL in
  `specs/003-workspace-installation-contracts/tasks.md`
- [ ] T053 For every valid Review or acceptance finding, update
  `specs/003-workspace-installation-contracts/spec.md` or
  `specs/003-workspace-installation-contracts/tasks.md` before behavioral fixes,
  apply fixes only in the owning module paths, rerun the full quickstart, and
  obtain a new independent Reviewer until High `0`, Medium `0`, and explicit
  PASS are recorded in `specs/003-workspace-installation-contracts/tasks.md`
- [ ] T054 Run Spec Kit Converge against the implementation and append every
  remaining accepted gap to
  `specs/003-workspace-installation-contracts/tasks.md`; implement any appended
  tasks, repeat independent Review, and finish only when Converge appends no
  blocking work
- [ ] T055 Update `docs/handoffs/CURRENT.md` with final commit/base, completed
  behavior, verification, Review identity, fallback delta/evidence, remaining
  non-goals, and clean-worktree recovery commands without machine-specific
  assumptions

---

## Dependencies & Execution Order

### Phase Dependencies

- **Contract Gate Intake (Phase 1)**: No dependencies; validates this issue's
  merged artifacts.
- **Shared Foundation (Phase 2)**: Depends on Phase 1 and blocks every story.
- **Workspace Root (Phase 3 / US1)**: Depends on Phase 2.
- **Install/Pin (Phase 4 / US2)**: Depends on US1 and blocks all three Phase 5
  slices.
- **Inspection, Lifecycle, Resolution (Phases 5A-5C)**: Depend on US2 and may run
  concurrently because write ranges are disjoint.
- **Integration (Phase 6)**: Depends on desired Phase 5 slices; full acceptance
  requires all three.
- **Review/Converge (Phase 7)**: Depends on Phase 6 completion and clean
  verification evidence.

### User Story Dependencies

```text
US1 Workspace root
  -> US2 install and pin
       -> US3 inspection
       -> US4 lifecycle
       -> US5 exact resolution
            all -> integration -> independent Review -> Converge
```

### Parallel Ownership After US2

The implementation scheduler has a global maximum of **3 active tasks**. The
three active slots correspond to the US3 inspection, US4 lifecycle, and US5
resolution implementation chains. `[P]` marks work that may run concurrently
only within its owning slice after that slice's implementation dependency is
complete; it does not create additional cross-story agents or slots. Test
fan-outs from multiple stories MUST be queued when they would exceed the
three-task limit.

| Slice | Exclusive implementation files | Must not modify while parallel |
| --- | --- | --- |
| US3 Inspection | `workspace/inspection*`, `workspace/postgres/inspection*`, `gateway/workspace_inspection_handler*` | shared ports, migration, root/install, lifecycle, resolution, composition |
| US4 Lifecycle | `workspace/lifecycle*`, `workspace/postgres/lifecycle*`, `gateway/workspace_lifecycle_handler*` | shared ports, migration, root/install, inspection, resolution, composition |
| US5 Resolution | `workspace/resolution*`, `workspace/postgres/resolution*`, `gateway/internal_resolution_handler*`, internal-auth config | shared ports, migration, root/install, inspection, lifecycle, composition |

T045 owns final shared composition after all three slices finish. If a parallel
slice discovers that a shared port or public contract must change, it stops and
returns to Spec/Plan/Tasks analysis rather than editing the shared file.

### Within Each User Story

- Domain policy before persistence adapter behavior.
- Persistence behavior before transport adapter behavior.
- Complete approved implementation before mapped unit/integration tests.
- Story tests pass before the story checkpoint.
- No test creates a new policy that is absent from the Spec.

## Parallel Examples

After T025 and the US2 checkpoint, launch these three implementation chains:

```text
US3: T026 -> T027 -> T028 -> {T029, T030} -> T031
US4: T032 -> T033 -> T034 -> {T035, T036} -> T037
US5: T038 -> T039 -> T040 -> T041 -> {T042, T043} -> T044
```

Within a completed implementation slice, `[P]` unit tests may run concurrently
with its isolated PostgreSQL test setup when they do not share a database.

## Implementation Strategy

### Contract-First MVP

1. Complete Phases 1-2.
2. Deliver US1 as the independently testable Workspace authorization root.
3. Deliver US2 to close the actual `Discover -> Install` MVP.
4. Stop and validate exact pin, permissions, concurrency, and no-fallback
   semantics before parallel expansion.

### Incremental Delivery

1. Add US3 for owner visibility.
2. Add US4 for explicit authorization lifecycle.
3. Add US5 for the future Router trust boundary.
4. Compose only after each isolated story passes its post-implementation tests.
5. Run full integration, independent Review, remediation, fresh Review, and
   Converge.

### Completion Signal

The feature is not complete merely because unit or integration tests pass. It
requires all selected story checkpoints, full quickstart evidence, fallback
delta/evidence, independent Review PASS with no High/Medium findings, Converge
with no remaining blocking work, and an accurate handoff.

## Phase 8: Review Remediation Convergence

- [X] T056 CRITICAL preserve Northbound v2 byte-for-byte and publish the
  approved Workspace/Installation behavior as Northbound v3, including the
  `200` terminal Installation uninstall response, per Constitution V, FR-014
  through FR-018, and SC-001 (contradicts)
- [X] T057 remove the unsupported omitted Installation page-size default and
  require an explicit `limit` in the 1-100 range across Spec, contracts, Go
  constants, and tests per FR-017 and the fallback policy (contradicts)
- [X] T058 remove the stale 1-256 SemVer constraint wording so the strict
  parser remains the sole range boundary across every Spec 003 artifact per
  FR-005 and the research decision (contradicts)
- [X] T059 align the Northbound v3 API guide and quickstart with explicit
  pagination/cursor continuation and the body-bearing uninstall response per
  FR-017, FR-018, and SC-001 (partial)
- [X] T060 update active contract mappings, architecture documentation, ADR,
  handoff, and compatibility guidance for Northbound v3 while retaining
  Northbound v2 as historical migration evidence per Constitution V (partial)
- [X] T061 rewrite the PR commit sequence so approved Spec/Plan/Tasks and ADR
  precede contract implementation, then rerun independent Review and all
  quality gates per Constitution VIII (contradicts)
