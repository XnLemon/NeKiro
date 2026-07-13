# Tasks: Complete Invocation Contracts

**Input**: Design documents from
`/specs/001-complete-invocation-contracts/`

**Prerequisites**: `spec.md`, `plan.md`, `research.md`, `data-model.md`,
`contracts/`, and `quickstart.md`

**Execution rule**: Implementation tasks precede their mapped test tasks. Every
module implementation Agent reads `C:\Users\16040\.codex\skills\implement\SKILL.md`,
commits its work, and is followed by a new independent Review Agent reading
`C:\Users\16040\.codex\skills\code-review\SKILL.md`.

## Format

- `[P]` means the task can run in parallel because its write scope is disjoint.
- `[USn]` maps the task to a user story in `spec.md`.
- Exact file ownership below is exclusive while parallel Agents are active.

## Phase 1: Workspace Preparation

**Purpose**: Remove obsolete local artifacts and verify the SDD gate before any
contract implementation.

- [x] T001 Verify ignored/untracked status and delete only the resolved
  `E:\NeKiro\node_modules`, `E:\NeKiro\contracts\node_modules`,
  `E:\NeKiro\contracts\dist`, `E:\NeKiro\contracts\platform-contracts`, and
  confirmed-empty `E:\NeKiro\contracts\agent-card`,
  `E:\NeKiro\contracts\common`, `E:\NeKiro\contracts\errors`,
  `E:\NeKiro\contracts\events`, `E:\NeKiro\contracts\identifiers`,
  `E:\NeKiro\contracts\internal-api`, and
  `E:\NeKiro\contracts\platform-api`; retain root package manifests and
  lockfiles
- [x] T002 Run placeholder, clarification, path-ownership, and constitution
  checks against `specs/001-complete-invocation-contracts/` and record the
  analyze result before implementation

**Checkpoint**: Working tree contains only tracked project assets and the
approved Spec/Plan/Tasks define all implementation behavior.

**Analyze result (2026-07-13)**: 13/13 Functional Requirements covered by 34
tasks; no Critical/High findings, unresolved placeholders, or constitution
conflicts. The T006 → T012 artifact dependency is explicit and does not share a
write scope.

---

## Phase 2: Module A - Result and Directional API Contracts (US1, US2, US4)

**Ownership**: This Agent exclusively owns the files listed in T003-T009. It
MUST NOT edit `contracts/contracts.go`, `contracts/validate.go`,
`contracts/contracts_test.go`, Agent Card semantic artifacts, or A2A Profile
artifacts.

### Implementation

- [ ] T003 [P] [US1] Add transient result schemas at
  `contracts/schemas/invocation-result.v1.schema.json` and
  `contracts/schemas/invocation-result-stream-event.v1.schema.json`
- [ ] T004 [P] [US2] Add coherent metadata-only event schema at
  `contracts/schemas/invocation-event.v0.2.schema.json` and safe correlated
  error schema at `contracts/schemas/platform-error.v2.schema.json`
- [ ] T005 [US1] Add Go result/error DTO mappings and stream sequence validation
  in `contracts/result_contracts.go`
- [ ] T006 [US1] [US4] Add active API documents
  `contracts/openapi/control-plane.v2.yaml`,
  `contracts/openapi/control-plane-internal.v1.yaml`, and
  `contracts/openapi/router-internal.v2.yaml` without changing historical v1
  documents
- [ ] T007 [US1] [US4] Record migration and ownership decisions in
  `docs/decisions/0002-invocation-result-transport-and-internal-api-direction.md`,
  `docs/contracts/compatibility.md`, and
  `docs/architecture/phase-1-spec.md`

### Tests After Implementation

- [ ] T008 [P] [US1] Add non-streaming/streaming result, first-terminal-wins,
  interrupted-stream, no-result-in-Ledger, and Platform Error v2 tests in
  `contracts/result_contracts_test.go`
- [ ] T009 [P] [US2] [US4] Add terminal coherence and directional OpenAPI/media
  negotiation mapping tests in `contracts/result_api_contracts_test.go`
- [ ] T010 [US1] Run Module A tests, `go vet ./...`, and `git diff --check`,
  report Module A fallback delta/evidence, then commit all Module A-owned files

### Independent Review Gate

- [ ] T011 Create a new Review Agent for the Module A commit; fix every High or
  Medium finding and use a fresh Review Agent until it explicitly passes

---

## Phase 3: Module B - Agent Card Semantic Conformance (US3)

**Ownership**: This Agent exclusively owns the files listed in T012-T016. It
MUST NOT edit shared Go mapping files or Result/A2A artifacts.

### Implementation

- [ ] T012 [P] [US3] Add active structural schema
  `contracts/schemas/agent-card.v0.2.schema.json` and normative RFC 2119 rules
  `contracts/agent-card/v0.2/semantic-rules.md`
- [ ] T013 [P] [US3] Add the raw Card corpus and manifest under
  `contracts/agent-card/v0.2/conformance/` for valid baseline, shared
  permission, duplicate skill, duplicate permission, undeclared permission,
  and cross-version permission cases
- [ ] T014 [US3] Add stable rule IDs, fixture manifest DTOs, and reusable
  schema-independent semantic evaluation in
  `contracts/agent_card_semantics.go`

### Tests After Implementation

- [ ] T015 [US3] Add corpus-driven structural/semantic decision tests and rule
  ID assertions in `contracts/agent_card_conformance_test.go`
- [ ] T016 [US3] Run Module B tests, `go vet ./...`, and `git diff --check`,
  report Module B fallback delta/evidence, then commit all Module B-owned files

### Independent Review Gate

- [ ] T017 Create a new Review Agent for the Module B commit; fix every High or
  Medium finding and use a fresh Review Agent until it explicitly passes

---

## Phase 4: Module C - A2A Profile Conformance (US3)

**Ownership**: This Agent exclusively owns the files listed in T018-T023. It
MUST NOT edit shared Go mapping files or Result/Agent Card artifacts.

### Implementation

- [ ] T018 [P] [US3] Add Profile Schema
  `contracts/schemas/a2a-profile.v0.2.schema.json` and active profile
  `contracts/a2a-profile/v0.3.0/profile.v0.2.json` while preserving the legacy
  profile file
- [ ] T019 [P] [US3] Add hand-authored JSON-RPC/SSE request, result, error,
  lifecycle, artifact, and propagation fixtures plus manifest under
  `contracts/a2a-profile/v0.3.0/conformance/`
- [ ] T020 [US3] Add Profile v0.2 DTOs, loader, state mapping, and conformance
  manifest types in `contracts/a2a_profile_v02.go`

### Tests After Implementation

- [ ] T021 [US3] Add compile-time SDK signature/type assertions and fixed
  fixture validation through real `a2aclient`/`a2asrv` paths in
  `contracts/a2a_profile_conformance_test.go`
- [ ] T022 [US3] Cover invalid envelopes, zero Tasks, unsupported states,
  mismatched IDs, event-after-terminal, EOF-without-terminal, task errors,
  artifact ordering, and all context headers in the same conformance test file
- [ ] T023 [US3] Run Module C tests, `go vet ./...`, and `git diff --check`,
  report Module C fallback delta/evidence, then commit all Module C-owned files

### Independent Review Gate

- [ ] T024 Create a new Review Agent for the Module C commit; fix every High or
  Medium finding and use a fresh Review Agent until it explicitly passes

---

## Phase 5: Shared Go Mapping Integration

**Depends on**: T011, T017, and T024 all passed.

**Ownership**: The integration Agent may edit only the shared files and new
integration test file listed below. It works with, and does not revert, all
reviewed module commits.

### Implementation

- [ ] T025 Update active version constants and shared DTO aliases in
  `contracts/contracts.go` while keeping historical contract artifacts readable
- [ ] T026 Update embedded resources, active compiled schemas, Agent Card
  semantic-rule integration, Result validators, and Profile v0.2 validation in
  `contracts/validate.go`
- [ ] T027 Update legacy mapping expectations for active versions without
  deleting historical parse checks in `contracts/contracts_test.go`
- [ ] T028 Update active Spec Kit/contract references and current repository
  status in `AGENTS.md` and `README.md` without altering the project charter

### Tests After Implementation

- [ ] T029 Add cross-artifact version synchronization, OpenAPI-to-Go mapping,
  corpus discovery, and secret/result exclusion tests in
  `contracts/active_contracts_integration_test.go`
- [ ] T030 Execute every command in
  `specs/001-complete-invocation-contracts/quickstart.md`, including
  `go test -count=1 ./...`, `go vet ./...`, and `git diff --check`, report the
  Integration fallback delta/evidence, then commit the shared integration files

### Independent Review Gate

- [ ] T031 Create a new Review Agent for the integrated Spec 001 diff against
  the pre-feature commit; fix every High or Medium finding and create a fresh
  Review Agent after each fix until it explicitly passes

---

## Phase 6: Convergence

- [ ] T032 Map every Spec requirement and acceptance scenario to implemented
  artifacts and passing tests in `specs/001-complete-invocation-contracts/tasks.md`
- [ ] T033 Confirm fallback delta is reported for every implementation module
  and total added fallback count is zero
- [ ] T034 Update the Spec status to complete only after all module and
  integration Review gates pass, then commit the finalized SDD artifacts

---

## Dependencies and Parallel Execution

### Phase Dependencies

- Phase 1 blocks all implementation.
- Modules A, B, and C may begin in parallel after Phase 1.
- Module A's Northbound v2 OpenAPI mapping check requires the Agent Card `0.2`
  schema from T012; Module A may implement its other files while T012 completes.
- Shared Integration starts only after all three module Review gates pass.
- Convergence starts only after integrated Review passes.

### Parallel Write Sets

```text
Module A: result/event/error schemas, v2/directional OpenAPI, result Go files,
          ADR and compatibility/architecture docs
Module B: Agent Card v0.2 schema, semantic rules/corpus, card semantic Go files
Module C: A2A Profile v0.2 schema/profile/corpus, profile Go files
Integration: contracts.go, validate.go, contracts_test.go, integration test
```

No two active implementation Agents may edit the same file. Reviews are
read-only. If a required change falls outside an Agent's scope, it is recorded
for the integration phase instead of crossing ownership.

## Implementation Strategy

1. Clean the workspace and pass SDD analysis.
2. Start Module B first, then Modules A and C immediately; only T006 waits for
   T012 if the new Schema reference is not yet present.
3. Review and close each module independently.
4. Integrate shared Go mappings only from reviewed module commits.
5. Run full contract validation and independent integrated Review.
6. Converge the Spec and then proceed to the next backend feature Spec.
