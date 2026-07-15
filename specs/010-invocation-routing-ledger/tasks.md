# Tasks: Invocation Routing and Ledger Parent Delivery

**Input**: Design documents from `specs/010-invocation-routing-ledger/`

**Scope**: Parent-level GitHub delivery graph. Each task becomes one native
GitHub Sub-issue and MUST create its own listed Spec directory before runtime
implementation. Child implementation tasks and mapped tests are generated in
that child Spec; this file does not replace them.

**Tests**: Every child issue implements approved behavior first, then adds the
contract/unit/integration/E2E evidence named in its acceptance profile. T011 is
the integrated parent acceptance and independent closure review.

## Phase 1: Contract and Policy Gate

**Purpose**: Freeze shared contracts and failure semantics before parallel
runtime work. Workspace Issue #2 is closed; T001 is ready.

- [ ] T001 Freeze the Invocation/Router/Ledger/SDK contract, credential, deadline, cancellation, and Ledger-failure policy in `specs/011-invocation-runtime-contracts/`, `contracts/`, and `docs/decisions/`

**Checkpoint**: Active version decisions, SDK-facing Router direction, Agent
credential binding, size/deadline rules, and failure semantics are approved.

## Phase 2: Foundational Parallel Slices

**Purpose**: Establish disjoint process, authorization, persistence, and sample
boundaries after T001. These four tasks are the maximum parallel batch.

- [ ] T002 [P] Implement Control Plane Invocation Dispatch and Gateway live-result proxy with post-implementation tests in `specs/012-control-plane-invocation-dispatch/` and `apps/control-plane/internal/invocation/`
- [ ] T003 [P] Build the strict A2A Router process, internal authentication, configuration, readiness, and Control Plane resolution client with post-implementation tests in `specs/013-a2a-router-foundation/`, `apps/a2a-router/cmd/a2a-router/`, `apps/a2a-router/internal/config/`, `apps/a2a-router/internal/auth/`, `apps/a2a-router/internal/resolution/`, and `apps/a2a-router/Dockerfile`
- [ ] T004 [P] Implement the Router-owned append-only Ledger, transactional projection, migrations, and internal read API with post-implementation PostgreSQL tests in `specs/014-invocation-ledger/`, `apps/a2a-router/internal/ledger/`, and `apps/a2a-router/internal/api/ledger_handler.go`
- [ ] T005 [P] Build the deterministic direct A2A callee sample and active-profile conformance evidence in `specs/015-direct-a2a-sample/` and `agents/runtime-b/`

**Checkpoint**: Dispatch can target the Router contract, Router can resolve
through Control Plane, Ledger can append/read durable facts, and one conforming
callee exists. No integrated Agent result is claimed yet.

## Phase 3: User Story 1 - Invoke an Installed Agent (Priority: P1)

**Goal**: Deliver root invocation through Gateway -> Dispatch -> Router -> Agent
with transient JSON first, then ordered streaming/cancellation behavior.

**Independent Test**: Invoke T005 through the public Gateway in JSON and SSE
modes, verify exact correlation/result order, and prove the Control Plane never
contacts the Agent directly.

- [ ] T006 [US1] Implement non-streaming exact A2A dispatch and transient result delivery with post-implementation unit, HTTP, PostgreSQL, and A2A tests in `specs/016-nonstream-a2a-dispatch/` and `apps/a2a-router/internal/transport/a2a/`
- [ ] T007 [P] [US1] Implement streaming, explicit deadline, HTTP disconnect cancellation, A2A task cancellation, and first-terminal-wins behavior with post-implementation SSE/race tests in `specs/017-stream-cancel-timeout/` and `apps/a2a-router/internal/taskcontext/`

**Checkpoint**: Root JSON and SSE calls are independently usable and produce
durable metadata-only lifecycle facts.

## Phase 4: User Story 2 - Inspect Invocation Facts (Priority: P1)

**Goal**: Expose authorized metadata-only Invocation and Trace reads through
Gateway while preserving Workspace isolation and Router ownership.

**Independent Test**: Query success/failure facts and a trace after Router store
reconstruction, including not-found, foreign Workspace, and dependency failure.

- [ ] T008 [P] [US2] Implement authorized Northbound Invocation/Trace metadata reads and Router read proxies with post-implementation HTTP/restart/isolation tests in `specs/018-invocation-trace-reads/` and `apps/control-plane/internal/gateway/`

**Checkpoint**: A caller can inspect durable metadata but cannot retrieve input,
output, chunks, credentials, or unrelated Workspace facts.

## Phase 5: User Story 3 - Cross-Runtime Nested Invocation (Priority: P1)

**Goal**: Deliver the thin Go Agent SDK and a second Runtime caller that invokes
the direct callee only through the Router.

**Independent Test**: Invoke Agent A through Gateway; Agent A uses the SDK to
invoke Agent B; verify exact parent/root/trace lineage and no shared Runtime
internal type or storage.

- [ ] T009 [P] [US3] Implement the thin Go Agent SDK, trusted context validation/propagation, authenticated nested Router handler/adapter, and post-implementation contract tests in `specs/019-agent-sdk-nested-invocation/`, `sdks/agent-sdk/`, `apps/a2a-router/internal/api/agent_invocation_handler.go`, and `apps/a2a-router/internal/nested/`
- [ ] T010 [US3] Build and pin the isolated second-Runtime caller sample, integrate the SDK nested call, and add Runtime-independence evidence in `specs/020-cross-runtime-caller/` and `agents/runtime-a/`

**Checkpoint**: Two different Runtime implementations complete one managed
parent-child call through the same Router and Ledger semantics.

## Phase 6: User Story 4 - Failure and Parent Acceptance (Priority: P1)

**Goal**: Prove the complete backend loop and every required failure/operational
boundary, then independently review and converge the parent.

**Independent Test**: Run real Control Plane, Router, PostgreSQL, and both
Agents through the full clean, failure, restart, and 100-concurrent matrix.

- [ ] T011 [US4] Complete cross-process backend E2E, failure/concurrency/content-exclusion evidence, Compose/CI wiring, independent Review, Converge, and handoff in `specs/021-invoke-record-acceptance/`, `tests/e2e/invoke-record/`, `deploy/compose.yaml`, and `.github/workflows/ci.yml`

**Checkpoint**: `Register -> Discover -> Install -> Invoke -> Record` passes at
the backend boundary and the parent has no unresolved blocking review finding.

## Dependency Graph

```text
Workspace Issue #2 closed
  -> T001
  -> [T002 || T003 || T004 || T005]
  -> [T006 || T008]
  -> [T007 || T009]
  -> T010
  -> T011
```

### Exact Blocked-By Relations

| Task | Blocked by |
| --- | --- |
| T001 | None; Workspace Issue #2 is closed |
| T002 | T001 |
| T003 | T001 |
| T004 | T001 |
| T005 | T001 |
| T006 | T002, T003, T004, T005 |
| T007 | T006 |
| T008 | T002, T004 |
| T009 | T001, T003, T006 |
| T010 | T005, T009 |
| T011 | T007, T008, T009, T010 |

## Parallel Execution

| Batch | Tasks | Maximum | Why writes do not conflict |
| --- | --- | --- | --- |
| A | T002, T003, T004, T005 | 4 | Control Plane, Router cmd/config/auth/resolution, Ledger/its handler, and sample directories are disjoint; T001 alone owns shared contracts |
| B | T006, T008 | 2 | Router transport and Control Plane read proxy use separate owners after their prerequisites |
| C | T007, T008 if unfinished, T009 | 3 | Task context/streaming, read proxy, and SDK use separate directories and frozen APIs |
| Integration | T010 then T011 | 1 | Caller sample integrates SDK/callee; final Compose/CI/handoff is single-owner |

Do not run T002-T010 concurrently with T001 contract writes. Do not let multiple
children edit `deploy/compose.yaml` or `.github/workflows/ci.yml`; T011 owns final
integration, while earlier children keep local/focused fixtures in their own
scope.

## Requirement Coverage

| Requirements | Parent tasks |
| --- | --- |
| FR-001 through FR-004 | T001, T002, T006, T011 |
| FR-005 through FR-009 | T001, T003, T006, T007, T011 |
| FR-010 through FR-012 | T001, T004, T008, T011 |
| FR-013 through FR-015 | T001, T005, T009, T010, T011 |
| FR-016 through FR-019 | T001, T002, T003, T004, T006, T007, T011 |
| FR-020 | T001 through T011; each child has independent SDD/Review/Converge |
| FR-021 | T001 through T011 fallback inventory; T011 final audit |
| SC-001 through SC-009 | T011 integrates evidence produced by T002-T010 |

## Sub-Issue Acceptance Profiles

### T001 Contract Gate

- Freeze SDK-facing Router direction/auth, trusted context, and exact errors.
- Freeze Agent credential binding/support without storing or defaulting secrets.
- Freeze Ledger post-side-effect failure and non-terminal visibility.
- Freeze terminal transitions from `routing` and cancellation/timeout from
  `pending`, `routing`, or `running`; success remains `running`-only.
- Freeze explicit deadline/cancel/size/SSE behavior and compatibility versions.
- Pass contract/conformance tests, independent Review, and Converge with added
  fallback count zero.

### T002 Control Plane Dispatch

- Authenticate/validate before trusted Invocation creation.
- Generate exact root IDs, authorize through Workspace, and obtain exact pin.
- Dispatch only to Router; forward JSON/SSE without full buffering.
- Preserve all typed policy/dependency errors and add no direct Agent path.

### T003 Router Foundation

- Start a separate process with strict required config and explicit readiness.
- Require distinct service authentication and destination configuration.
- Resolve only through Control Plane Internal v2; import no Control Plane internals.
- Validate active contracts/Profile before transport work is accepted.
- Do not edit `internal/ledger/` or Ledger-specific API handlers owned by T004.

### T004 Ledger

- Append immutable events and transactionally update the projection.
- Enforce exact sequence/context/state/terminal invariants under concurrency.
- Preserve restart durability and deterministic Invocation/Trace reads.
- Prove zero input/result/chunk/credential/raw dependency content.
- Do not edit Router cmd/config/auth/resolution paths owned by T003; final
  process/Compose integration remains T011-owned.

### T005 Direct Callee Sample

- Run as an independent Agent process with no platform database access.
- Pass every active A2A Profile conformance case used by the platform.
- Provide deterministic JSON and stream behavior for later acceptance.
- Keep Runtime internals outside shared contracts/core packages.

### T006 Non-Streaming Dispatch

- Complete exact re-resolution and `message/send` through the pinned library.
- Map Message/Task outcomes and explicit route/protocol/Agent/dependency errors.
- Commit terminal Ledger fact before returning a clean result.
- Return exact arbitrary JSON result with no persistence or replay.

### T007 Streaming, Cancel, and Timeout

- Emit accepted sequence 0, gap-free chunks, and one terminal event.
- Propagate disconnect/deadline and supported A2A cancellation.
- Enforce first-terminal-wins and discard post-terminal output.
- Distinguish pre-commit HTTP and post-commit in-band failures.

### T008 Invocation and Trace Reads

- Authorize reads to the owning Workspace before Router access.
- Return deterministic metadata-only events/projection/lineage after restart.
- Distinguish not found, forbidden, and dependency failure.
- Never expose result content or another Workspace's facts.

### T009 Agent SDK

- Validate all required platform context; synthesize no identity/correlation.
- Authenticate only to the frozen SDK-facing Router boundary.
- Implement the Router agent-facing handler and nested adapter in the T009-owned
  paths; leave final process/Compose registration to T011.
- Preserve root/trace and set exact parent while Router creates a new child ID.
- Contain no model/tool/workflow/memory/retry framework behavior.

### T010 Cross-Runtime Caller

- Pin a verified Runtime dependency only under the sample directory.
- Receive a root call and use the SDK for one nested callee invocation.
- Share no Runtime-internal types/storage with the direct sample.
- Pass the same platform/A2A conformance and secret-safety rules.

### T011 Parent Acceptance

- Exercise JSON, SSE, nested, failure, restart, and 100-concurrent scenarios.
- Verify exact durable parent-child lineage and one clean terminal per accepted
  successful/failure lifecycle.
- Inspect storage/log/API surfaces for forbidden content and secrets.
- Run all static, contract, PostgreSQL, A2A, Compose, and E2E gates.
- Complete fresh independent Review and append/fix every Converge finding.

## Implementation Strategy

### Backend MVP

1. Close Workspace blocker.
2. Complete T001.
3. Run T002-T005 in parallel.
4. Complete T006 and validate one non-streaming root invocation plus Ledger.
5. Stop and review before adding streaming or nested behavior.

### Incremental Completion

1. Add T008 reads while non-stream transport stabilizes.
2. Run T007 and T009 in parallel after T006.
3. Add T010 cross-Runtime caller.
4. Run T011 as the only full integration/closure owner.

## Format Validation

- Total tasks: 11.
- Child Sub-issues: 11 plus one parent GitHub task.
- Every task line has a checkbox, unique sequential ID, optional valid `[P]`,
  required story label only in user-story phases, and exact repository paths.
- Stable maximum parallelism: 4.
- Suggested MVP: T001-T006 (one authorized non-streaming root invocation with a
  durable terminal Ledger fact).

## GitHub Delivery Map

**Parent**: [#19 Deliver Invocation Dispatch, A2A Routing, and Ledger](https://github.com/NeKiro-project/NeKiro/issues/19)

| Task | GitHub Sub-issue |
| --- | --- |
| T001 | [#20](https://github.com/NeKiro-project/NeKiro/issues/20) |
| T002 | [#21](https://github.com/NeKiro-project/NeKiro/issues/21) |
| T003 | [#22](https://github.com/NeKiro-project/NeKiro/issues/22) |
| T004 | [#23](https://github.com/NeKiro-project/NeKiro/issues/23) |
| T005 | [#24](https://github.com/NeKiro-project/NeKiro/issues/24) |
| T006 | [#25](https://github.com/NeKiro-project/NeKiro/issues/25) |
| T007 | [#26](https://github.com/NeKiro-project/NeKiro/issues/26) |
| T008 | [#27](https://github.com/NeKiro-project/NeKiro/issues/27) |
| T009 | [#28](https://github.com/NeKiro-project/NeKiro/issues/28) |
| T010 | [#29](https://github.com/NeKiro-project/NeKiro/issues/29) |
| T011 | [#30](https://github.com/NeKiro-project/NeKiro/issues/30) |

GitHub native Sub-issue count is 11. Native dependency relations match the
Exact Blocked-By table above; the parent #19 and T001/#20 dependency on closed
Workspace parent #2 is satisfied, so T001 is ready.

## Independent Planning Review

The independent reviewer first identified conflicting Ledger evidence for
post-side-effect persistence failure and missing terminal transitions from
`routing`; the Spec, Plan, data model, contract guide, and T001 acceptance now
distinguish pre-acceptance/no-Ledger failures, accepted terminal facts, and the
explicit non-terminal audit exception.

Follow-up review found and resolved two ownership gaps: T003/T004 now have
disjoint Router foundation versus Ledger paths, and T009 explicitly owns both
the Agent SDK and Router agent-facing nested adapter. The final review result is
**PASS with no remaining P0-P2 planning issue**.

## Fallback Report

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
