# Tasks: Invocation Runtime Contracts

**Input**: Design artifacts in `specs/011-invocation-runtime-contracts/`
**Tests**: Approved contract implementation precedes mapped tests.

## Phase 1: SDD Gate

- [x] T001 Read AGENTS.md, constitution, parent Spec 010, active contracts, ADRs, and GitHub #20/#19.
- [x] T002 Write and clarify `spec.md`; complete `checklists/requirements.md`.
- [x] T003 Write `plan.md`, `research.md`, `data-model.md`, `contracts/runtime-contract.md`, and `quickstart.md` with pre/post Constitution checks.
- [x] T004 Derive this dependency-ordered task list and fallback inventory.
- [x] T005 Run read-only Spec/Plan/Tasks analysis and resolve every blocking inconsistency before implementation.

## Phase 2: Language-neutral implementation

- [x] T006 [US2] Add Platform Error v4 with distinct unsupported-auth and payload-size outcomes in `contracts/schemas/platform-error.v4.schema.json`.
- [x] T007 [P] [US3] Add Invocation Event v0.3 referencing Platform Error v4 in `contracts/schemas/invocation-event.v0.3.schema.json`.
- [x] T008 [P] [US3] Add Result Stream Event v2 referencing Platform Error v4 in `contracts/schemas/invocation-result-stream-event.v2.schema.json`.
- [x] T009 [US1] Add authenticated parent-derived nested surface in `contracts/openapi/router-agent.v1.yaml`.
- [x] T010 [P] [US3] Add accepted/failure/limit service surface in `contracts/openapi/router-internal.v3.yaml`.
- [x] T011 [P] [US3] Add Northbound invoke/read target and size semantics in `contracts/openapi/control-plane-invocation.v4.yaml`.
- [x] T012 [US4] Record trust, authentication, acceptance, interruption, deadline, cancel, size, SSE, and first-terminal policy in `docs/decisions/0006-invocation-runtime-trust-and-failure-policy.md`.
- [x] T013 [US4] Update active target/migration/no-fallback guidance in `docs/contracts/compatibility.md`.
- [x] T014 [US1] Add Go target constants and DTO consumers in `contracts/runtime_contracts.go` without runtime behavior.

## Phase 3: Mapped tests after implementation

- [x] T015 [US1] Add OpenAPI direction/auth/trusted-field and strict-shape tests in `contracts/runtime_contracts_test.go`.
- [x] T016 [US2] Add error/auth-support and secret/content exclusion tests in `contracts/runtime_contracts_test.go`.
- [x] T017 [US3] Add acceptance/lifecycle/post-side-effect failure contract tests in `contracts/runtime_contracts_test.go`.
- [x] T018 [US4] Add required limit/no-default, cancellation, and SSE framing declaration tests in `contracts/runtime_contracts_test.go`.
- [x] T019 [US4] Add compatibility/historical immutability assertions in `contracts/runtime_contracts_test.go` and active embedding/loading lists where required.

## Phase 4: Verification and delivery

- [x] T020 Run focused contract tests.
- [x] T021 Run full contract, repository, vet, formatting, and diff checks.
- [x] T022 Confirm repo-local identity, commit logical changes, push branch, and open a draft PR referencing #20 and #19 with evidence.

## Phase 4A: Independent Review Remediation

- [x] R001 Return Review findings to Spec/Plan/Tasks/ADR/compatibility before contract changes.
- [x] R002 Restore Workspace-scoped Invocation detail and Trace lineage response contracts and migration guidance.
- [x] R003 Split Platform Error v4 pre-correlation/correlated shapes and apply them by acceptance phase.
- [x] R004 Freeze Agent response/A2A event no-default limits, post-acceptance oversize mapping, and shared strict media negotiation.
- [x] R005 Add exported runtime schema/semantic validators, rules, and positive/negative conformance corpus.
- [x] R006 Add mapped DTO/OpenAPI/schema/corpus tests after remediation implementation.
- [x] R007 Run focused/full/static verification, commit, push PR #32, and report finding locations.
- [x] R008 Add outer/nested error correlation plus Result Stream v2 sequence/chunk/first-terminal validator and negative corpus after follow-up Review.
- [x] R009 Add Workspace detail/Trace projection cross-field validators and versioned negative corpus, including non-empty/parent-complete lineage.
- [x] R010 Re-run full verification and push the follow-up Review remediation.
- [x] R011 Add Result Stream EOF/Finish terminal validation and missing-terminal negative corpus.
- [x] R012 Enforce Trace parent-before-child, no self-parent/cycle, and root-Task stability with negative corpus.
- [x] R013 Re-run full verification and push the third Review remediation.

- [ ] T023 Independent Review by a non-implementing agent. **Owner: root; intentionally not completed here.**
- [ ] T024 Converge findings into tasks and resolve them. **Blocked by T023; intentionally not completed here.**

## Dependencies

`T001-T004 -> T005 -> T006 -> T007/T008 -> T009/T010/T011 -> T012-T014 -> T015-T019 -> T020-T022 -> T023 -> T024`.

T007/T008 are parallel after the shared error schema. T009/T010/T011 are parallel by file after shared schemas. Tests are sequentially added after all approved contract implementation to avoid using tests as policy evidence.

## Requirement Coverage

| Requirements | Tasks |
| --- | --- |
| FR-001..FR-005 | T009, T014, T015 |
| FR-006 | T006, T007, T009-T011, T016 |
| FR-007..FR-012 | T007-T012, T017 |
| FR-013..FR-017 | T009-T012, T018 |
| FR-018 | T006-T014, T016 |
| FR-019 | T012-T013, T019 |
| FR-020 | R002, R006 |
| FR-021 | R003, R005-R006 |
| FR-022 | R005-R006 |
| FR-023..FR-024 | R004-R006 |

## Fallback Report

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
