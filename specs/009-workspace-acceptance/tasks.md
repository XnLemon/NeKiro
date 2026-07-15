# Tasks: Workspace Acceptance

**Input**: Design documents from `/specs/009-workspace-acceptance/`.

**Scope**: Issue #9 only. Existing Catalog, Workspace, Gateway, and active
contracts are reused. No Router, Ledger, frontend, Agent runtime, or new
product behavior is included.

## Phase 1: Observe and Design Gate

- [X] T001 Read AGENTS.md, latest `main@99e52aa`, Issue #9, active contracts,
  Specs #3/#5/#6/#7/#8, handoff, and existing integration/test seams.
- [X] T002 Freeze the evidence-only scope, boundary ownership, dedicated
  `_test` database prerequisite, error semantics, no-Agent policy, and zero
  added fallback policy in `spec.md` and `research.md`.
- [X] T003 Produce `plan.md`, `data-model.md`, the acceptance contract guide,
  `quickstart.md`, and the requirements checklist.
- [X] T004 Run the Spec quality checklist and self-review; confirm no
  unresolved clarification marker, placeholder, or contract ambiguity remains.
- [X] T005 Run cross-artifact analysis after Tasks generation and stop before
  implementation if a constitution or requirement coverage conflict remains.

## Phase 2: Acceptance Implementation

- [X] T006 [P] Strengthen
  `TestConcurrentLifecycleAndReinstallRequestsPreserveOneCurrentRow` in
  `apps/control-plane/internal/workspace/integration/workspace_test.go` to
  retain every successful response, validate legal transitions, immutable
  fields, terminal timestamps, reinstall identity, conflict counts, and final
  current-row uniqueness.
- [X] T007 [P] Create
  `apps/control-plane/internal/workspace/integration/acceptance_http_test.go`
  with a real PostgreSQL-backed Catalog/Workspace/Gateway harness and the
  `TestAcceptanceWorkspaceControlPlaneHTTPWorkflow` public/internal flow.
- [X] T008 [P] Add `TestAcceptanceHTTPFailureBoundaries` to the acceptance
  harness for auth, owner, input, identity, disabled, unknown capability,
  Catalog-disabled, dependency, Trace, and secret-exclusion outcomes.
- [X] T009 [P] Add acceptance assertions that fixture Agent endpoint URLs are
  never contacted and that internal resolution uses a separate authenticator
  from public routes.

## Phase 3: Evidence and Documentation

- [X] T010 Record focused test names, route versions, race counts, database
  prerequisite behavior, and fallback delta in this Tasks file and
  `quickstart.md`.
- [X] T011 Update `docs/handoffs/CURRENT.md` with Issue #9 acceptance closure,
  verification limitations, and Invocation Dispatch/A2A Router as next scope.

## Phase 4: Verification, Review, and Converge

- [X] T012 Run `gofmt`, focused Go tests, contract tests, and acceptance tests;
  record exact results.
- [X] T013 Run the real PostgreSQL matrix serially against a disposable
  `_test` database, including Catalog migration, Workspace migration,
  inspection, and Workspace integration packages.
- [X] T014 Run broad Go tests, race, vet, build, `go mod tidy -diff`, Compose
  configuration, and `git diff --check`.
- [X] T015 Perform an independent review against Spec, Plan, Tasks, contracts,
  AGENTS.md, and the implementation; append any finding as a traceable task.
- [X] T016 Run Converge and resolve every remaining approved-scope task; do not
  invent fallback or expand into Router/Frontend/Agent runtime behavior.
- [X] T017 Verify repository-local Git identity, commit the focused Issue #9
  change, and report branch, commit, tests, SDD status, and fallback delta.
- [X] T018 [Review-R1] Record and remediate the independent review findings:
  complete the HTTP failure matrix and Trace assertions, strengthen race
  result assertions, remove fixture conflict swallowing, and align the
  no-Agent wording with the sentinel-server evidence.
- [X] T019 [Review-R2] Add Trace assertions to all successful failure-fixture
  setup responses, align the Plan with the actual public read/restart evidence,
  and correct the Research statement about the sentinel server's local port.
- [X] T020 [Review-R4 Spec] Close the four Spec-axis evidence gaps: concurrent
  Workspace creation, public cursor traversal, durable state immutability after
  rejected requests, and real schema/transaction failure mapping.
- [X] T021 [Review-R5] Add real PostgreSQL-backed internal HTTP evidence for
  `AGENT_NOT_INSTALLED` after uninstall and `CAPABILITY_NOT_ALLOWED` when the
  accepted permission snapshot omits a capability requirement.
- [X] T022 [Review-R5] Close the remaining acceptance evidence gaps by proving
  highest matching version selection through the composed HTTP flow and
  enabled/disabled/uninstalled facts after store reconstruction.
- [X] T023 [Review-R6] Preserve valid request correlation when internal
  resolution rejects a non-correlation field such as Workspace, Agent, or
  capability identity.
- [X] T024 [Review-R6] Make Workspace schema readiness reject incomplete
  Installation columns and constraints, with real PostgreSQL regression
  evidence.

## Requirement Coverage Map

| Requirement | Tasks | Evidence |
| --- | --- | --- |
| FR-001 | T007, T012, T013 | Public capability discovery returns the published fixture. |
| FR-002 | T007, T012, T013 | Public create/install returns exact pin and permission snapshot. |
| FR-003 | T007, T012, T013, T020 | HTTP list/detail/cursor traversal plus existing keyset/restart integration. |
| FR-004 | T006, T007, T012, T013 | Legal/illegal lifecycle and committed timestamps. |
| FR-005 | T007, T008, T012, T013, T021, T023 | Separate internal auth and exact resolution correlation, including typed resolution failures and validation precedence. |
| FR-006 | T006, T007, T013, T022 | Restart, enabled/disabled/terminal history, and new reinstall identity. |
| FR-007 | T006, T013, T020 | 100-request create/install/lifecycle races and per-result legal outcome validation. |
| FR-008 | T007, T008, T012 | Active v3/public and v2/internal HTTP routes only. |
| FR-009 | T008, T012, T020, T021 | Distinct auth, owner, identity, conflict, disabled, capability, and dependency results with state immutability checks. |
| FR-010 | T008, T013, T020, T024 | Canceled/dependency/schema/transaction failures remain explicit, including incomplete Installation schema readiness. |
| FR-011 | T007, T008, T009 | No Agent endpoint call and no future runtime dependency. |
| FR-012 | T002, T003, T010, T011, T015, T016, T017 | SDD artifacts, quickstart, review, converge, and handoff. |

## Dependency and Write Scope

T001-T005 are the completed observe/design gate. T006, T007, T008, and T009
have disjoint test write scopes and may run in parallel after T005. T010/T011
follow implementation results. T012-T017 are sequential verification and
delivery gates. No task changes production ownership or active contract
versions.

## Implementation and Verification Evidence

- T006 strengthened
  `TestConcurrentLifecycleAndReinstallRequestsPreserveOneCurrentRow` to
  retain successful results, assert one disable success/99 conflicts, validate
  terminal timestamps and immutable facts, count uninstall/reinstall outcomes,
  and reject duplicate or unknown final history rows.
- T020 added
  `TestConcurrentWorkspaceCreateLeavesOneCommittedRow` with 100 concurrent
  create requests, one committed result, 99 conflicts, and a durable read-back.
- T007/T008 added
  `TestAcceptanceWorkspaceControlPlaneHTTPWorkflow` and
  `TestAcceptanceHTTPFailureBoundaries` in a real PostgreSQL-backed composed
  Catalog/Workspace/Gateway harness.
- T020 extended the composed HTTP workflow with three Installation rows and
  public `limit=1` cursor traversal, and extended failure coverage with a
  durable Installation read after rejected requests.
- T020 also exercises a real Workspace schema outage and a PostgreSQL trigger
  that forces a transaction failure; both must map to `503 DEPENDENCY_ERROR`
  without exposing dependency details.
- T020 integration-tag compilation and all static checks passed locally. The
  PostgreSQL-backed T020 execution remains pending because this environment has
  neither `NEKIRO_TEST_DATABASE_URL` nor a running Docker PostgreSQL service.
- T009 uses a local fixture Agent HTTP server that records every request; both
  acceptance tests completed with zero Agent endpoint calls. Public and
  internal authenticators use different tokens.
- T012 passed `gofmt`, integration-tag compilation, `go test -count=1 ./...`,
  `go test -race -count=1 ./...`, `go vet ./...`, `go build ./...`,
  `go mod tidy -diff`, and `git diff --check`.
- T013 passed serially against a disposable PostgreSQL 17 database named
  `nekiro_test`:
  `go test -tags=integration -count=1 ./apps/control-plane/internal/catalog/postgres`,
  `./apps/control-plane/internal/workspace/postgres`,
  `./apps/control-plane/internal/workspace/integration`, and
  `./tests/integration/catalog`.
- T014 broad static and Compose verification passed. The disposable database
  container was removed after the run.

## Review-R1 Remediation Evidence

Independent Review-R1 identified the following gaps: the HTTP failure matrix
was missing malformed-auth, mutation-owner, wrong-pair, conflict, and HTTP
dependency cases; several successful responses lacked Trace assertions; race
reinstall/conflict results were under-asserted; fixture setup swallowed
conflicts; and Quickstart wording incorrectly said the sentinel Agent server
was not started. T018 fixes each finding. Review-R2 found no P0/P1 issue and
identified three P2 evidence/documentation gaps; T019 fixes all three. T015
and T016 are now complete after Review-R3 PASS and a clean converge assessment.

## Review-R3 and Converge Evidence

Review-R3 re-read the current acceptance tests and SDD artifacts after T019.
It found no P0, P1, or P2 issue. Converge checked all FR/SC mappings, plan
decisions, active contract reuse, constitution boundaries, and task status and
found no remaining unbuilt work. T015, T016, and T017 are complete.

## Review-R5 Remediation Evidence

The independent review identified a remaining acceptance-evidence gap for
`AGENT_NOT_INSTALLED` after uninstall and `CAPABILITY_NOT_ALLOWED` when the
accepted permission snapshot is incomplete, plus highest-version selection and
disabled-state restart durability. T021 adds the typed HTTP failure cases and
T022 adds the real PostgreSQL-backed version and reconstruction evidence. The
post-remediation OCR review reported no comments.

## Review-R6 Remediation Evidence

Review-R6 found that internal resolution classified invalid Workspace, Agent,
and capability identifiers as pre-correlation failures, and that Workspace
readiness checked only the Installation table and indexes. T023 limits the
pre-correlation gate to invocation, root-task, and Trace identifiers and adds
HTTP regression cases. T024 validates all Installation columns and named
constraints and adds PostgreSQL readiness regression cases. No public contract
version or fallback policy changes.

## Spec Review Remediation

T020 addresses the four Spec-axis evidence gaps without changing production
ownership, contracts, routes, migrations, or fallback policy. The new tests
cover concurrent Workspace creation, public cursor traversal, durable state
immutability after rejected requests, and explicit schema/transaction failure
mapping through the composed Gateway.

## Cross-Artifact Analysis Evidence

- Constitution alignment: PASS. The feature is evidence-only, preserves
  Catalog/Workspace ownership, uses active v3/v2 contracts, and does not claim
  Router, Ledger, Frontend, or Agent runtime behavior.
- Requirement coverage: PASS. FR-001 through FR-012 each map to at least one
  implementation or verification task; SC-001 through SC-008 are covered by
  the workflow, race, restart, error, and final verification tasks.
- Contract compatibility: PASS. No schema, OpenAPI, DTO, route, event, or
  version change is planned.
- Dependency order: PASS. The design gate precedes disjoint test work, which
  precedes evidence updates and sequential verification/review/converge.
- Fallback policy: PASS. No new fallback is planned; the only retained policy
  is the active contract's legitimate empty Installation list.

**Analysis result**: PASS. Implementation may proceed within the listed test
and documentation write scopes.

## Fallback Report

```text
Fallback delta: removed 0, retained 1, added 0, net 0
Added fallback evidence: none
```

## Phase 5: Convergence

- [X] T025 [Review-R6] Correct
  `apps/control-plane/internal/workspace/postgres/migrations.go` so the exact
  migrated Workspace schema passes readiness while every incomplete
  Installation column, constraint, and index case remains rejected per SC-007
  and FR-010 (contradicts).
- [X] T026 [Review-R6] Rerun the real PostgreSQL suites and PR #18 project closure
  checks, then obtain a fresh independent review of the T025 correction before
  marking the Workspace parent ready to close per FR-012 (partial).
- [X] T027 Update `specs/009-workspace-acceptance/spec.md`, this evidence
  record, and `docs/handoffs/CURRENT.md` only after T026 is green; do not claim
  completed verification from compile-only evidence per SC-007 (partial).

### Latest CI Evidence

PR #18 commit `33eb1ae` corrected the PostgreSQL default varchar index opclass.
Run `29442651978` on 2026-07-16 passed `workspace-integration`, `go-quality`,
`frontend`, and `compose-config`, and PR #18 merged as upstream commit `5f94565`.
T025 is complete. T026 passed a fresh independent closure review over
`8998916..5f94565` with no P0-P2 finding; OCR also produced zero comments.
T027 refreshed Spec, tasks, handoff, and GitHub parent evidence, removed the
obsolete blocked label, and closed Workspace parent Issue #2. The next runtime
base is unblocked.
