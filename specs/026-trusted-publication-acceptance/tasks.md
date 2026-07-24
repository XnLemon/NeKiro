# Tasks: Trusted Publication Acceptance and Operations

**Input**: Design documents from `specs/026-trusted-publication-acceptance/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`,
`contracts/acceptance-matrix.md`, `quickstart.md`

**Tests**: The feature is an acceptance slice. Reusable fixture operations are
implemented before their mapped scenarios; every scenario maps to Spec 026.

## Phase 1: Setup and shared acceptance operations

**Purpose**: Make existing public operations reusable without changing service
behavior or creating a second harness.

- [x] T001 Refactor trusted-publication helpers to return exact Binding, Challenge, Release, and Installation facts in `tests/e2e/invoke-record/invoke_record_test.go`
- [x] T002 Add safe Release-read and Invocation/Trace Release-provenance validators in `tests/e2e/invoke-record/invoke_record_test.go`
- [x] T003 Add strict test-only Ed25519 credential fixture generation and Compose-internal Agent request execution in `tests/e2e/invoke-record/invoke_record_test.go`

**Checkpoint**: All scenario code can use public APIs and internal Agent network
access without SQL state writes, port exposure, retry, or secret output.

---

## Phase 2: User Story 1 - Prove a trusted Agent is usable (Priority: P1)

**Goal**: Complete and inspect one trusted cross-runtime lifecycle from an
empty environment.

**Independent Test**: Run the positive portion of the clean acceptance and
prove Runtime A and Runtime B results, lineage, per-Agent Release provenance,
and Release trust method.

### Implementation for User Story 1

- [x] T004 [US1] Return and retain immutable published Release and installed Installation facts during positive setup in `tests/e2e/invoke-record/invoke_record_test.go`

### Tests for User Story 1

- [x] T005 [US1] Assert root/child Invocation, Task, Trace, Workspace, Card version, Release ID, Card digest, and `http_well_known` Release linkage for FR-001 through FR-005 in `tests/e2e/invoke-record/invoke_record_test.go`
- [x] T006 [US1] Extend every post-issuance response, metadata-only persistence, and log scan to include Release provenance, verification evidence metadata, challenge material, and signing material for FR-012 in `tests/e2e/invoke-record/invoke_record_test.go`

**Checkpoint**: User Story 1 independently proves the trusted positive loop and
cross-runtime provenance.

---

## Phase 3: User Story 2 - Prove trust boundaries reject unsafe calls (Priority: P1)

**Goal**: Execute every publication, lifecycle, credential, direct-access, and
unavailable-endpoint negative case with exact error and acceptance semantics.

**Independent Test**: Run the acceptance matrix from
`contracts/acceptance-matrix.md` and verify every expected status/code and
Ledger/no-Ledger boundary.

### Implementation for User Story 2

- [x] T007 [US2] Add reusable wrong-proof recovery, real-expiry, disallowed-destination, and unavailable-verification fixture operations in `tests/e2e/invoke-record/invoke_record_test.go`
- [x] T008 [US2] Add reusable unpublished/suspended/revoked Release and disabled/enabled Installation transition operations in `tests/e2e/invoke-record/invoke_record_test.go`

### Tests for User Story 2

- [x] T009 [US2] Add Compose E2E cases for wrong, expired, reused, disallowed, and unavailable endpoint verification outcomes and fresh-challenge recovery for FR-006 in `tests/e2e/invoke-record/invoke_record_test.go`
- [x] T010 [US2] Add Compose E2E cases for unpublished installation plus disabled Installation and suspended/revoked Release invocation errors for FR-007 and FR-011 in `tests/e2e/invoke-record/invoke_record_test.go`
- [x] T011 [US2] Add Compose-internal forged, expired, wrong-audience, and unauthenticated Agent request cases for FR-008 and FR-009 in `tests/e2e/invoke-record/invoke_record_test.go`
- [x] T012 [US2] Require the accepted unavailable-Agent failure to return correlation and persist exact failed Release provenance for FR-010 in `tests/e2e/invoke-record/invoke_record_test.go`
- [x] T013 [US2] Configure an explicit isolated Compose project, run the real challenge-expiry acceptance in every clean CI run, and retain unconditional logs/volume cleanup for FR-013 in `.github/workflows/ci.yml`

**Checkpoint**: User Story 2 independently proves the complete negative matrix
without Agent execution on rejected direct or pre-acceptance paths.

---

## Phase 4: User Story 3 - Diagnose and recover publication safely (Priority: P2)

**Goal**: Give providers, Workspace owners, and platform operators an exact
state/error-to-action guide over existing contracts.

**Independent Test**: Follow each row from observed error/state through owner,
manual action, and completion check without SQL, retry, fallback, or immutable
Release mutation.

### Implementation for User Story 3

- [x] T014 [P] [US3] Write verification, publication, suspension, revocation, inspection, responsibility, and recovery runbook tables for FR-014 through FR-016 in `docs/runbooks/trusted-publication-operations.md`
- [x] T015 [P] [US3] Link the trusted-publication runbook and clean acceptance from `docs/runbooks/local-development.md` and `README.md`

### Tests for User Story 3

- [x] T016 [US3] Cross-check every acceptance error category and deferred `Needs policy` item against the runbook and Spec matrix in `specs/026-trusted-publication-acceptance/contracts/acceptance-matrix.md`

**Checkpoint**: User Story 3 independently provides truthful recovery guidance
using only existing public/deployment boundaries.

---

## Phase 5: Polish, governance, and delivery

- [x] T017 Correct the existing Router cancellation race so a chunk or terminal append that loses to caller cancellation records one bounded `canceled`/`timed_out` terminal without retry or dependency remapping in `apps/a2a-router/internal/api/dispatch_handler.go`
- [x] T018 Add focused regressions for cancellation during stream-chunk and terminal commit, and keep timeout/cancellation on independent acceptance fixtures in `apps/a2a-router/internal/api/dispatch_handler_test.go` and `tests/e2e/invoke-record/invoke_record_test.go`
- [x] T019 Update completion evidence and active project status in `specs/023-trusted-agent-publication/tasks.md`, `AGENTS.md`, and `docs/handoffs/CURRENT.md`
- [x] T020 Run focused E2E, full Go/race/vet/lint, Compose config, and diff checks recorded in `specs/026-trusted-publication-acceptance/quickstart.md`
- [x] T021 Perform an independent Spec/standards Review and fix all High/Medium findings in the files named by the Review
- [x] T022 Run `speckit-converge` against `specs/026-trusted-publication-acceptance/` and complete any appended tasks
- [x] T023 Commit, push, open a PR referencing #52 and #47, require green CI, merge, close #52, and close #47 only when its parent acceptance criteria are satisfied
  - Delivery evidence: PR #57 merged as `785f9cf`; all seven checks passed in
    CI run `30074754169`; Issues #52 and #47 are closed.

---

## Dependencies & Execution Order

```text
T001 -> T002 -> T004 -> T005 -> T006
T001 -> T007 -> T009
T001 -> T008 -> T010
T003 -> T011
T002 -> T012
T009 -> T013
T014 + T015 -> T016
T006 + T010 + T011 + T012 + T013 + T016 -> T017 -> T018 -> T019 -> T020 -> T021 -> T022 -> T023
```

### User Story Dependencies

- **US1** starts after T001/T002 and can be validated before the negative matrix.
- **US2** starts after the relevant shared helper (T001/T002/T003); its
  publication, lifecycle, credential, and accepted-failure cases are disjoint
  by fixture identity but share one test file and therefore are edited
  sequentially.
- **US3** has no code dependency on US1/US2; the runbook and links may be
  drafted in parallel, then T016 compares them to the final matrix.

### Parallel Opportunities

- T014 and T015 touch different documentation files and are parallelizable.
- Review agents may inspect Spec compliance and code/standards independently
  after T018; neither may edit the implementation during its review.
- Tasks in `tests/e2e/invoke-record/invoke_record_test.go` remain sequential to
  avoid overlapping writes despite independent fixture behavior.

## Implementation Strategy

1. Preserve one authoritative clean acceptance run and make its operations
   return the exact facts needed by assertions.
2. Complete positive provenance linkage first so every later accepted failure
   can reuse the validator.
3. Add negative cases by boundary: publication, lifecycle, Agent credential,
   accepted transport failure.
4. Add the runbook after the exact matrix is executable, then cross-check each
   category and owner.
5. Correct any existing contract/ADR violation exposed by the acceptance, add
   its focused regression, and return the change to the Spec before delivery.
6. Run full gates, independent Review, convergence, PR CI, merge, and issue
   closure.

## Format Validation

All 23 tasks use the required checkbox, sequential ID, optional `[P]`, required
user-story label inside story phases, and explicit repository path format.
