# Tasks: Control Plane Invocation Dispatch

**Input**: Design documents from `specs/012-control-plane-invocation-dispatch/`

**Tests**: Mapped tests follow approved implementation, per project policy.

## Phase 1: SDD Gate

- [x] T001 Observe AGENTS, Specs 010/011, frozen contracts, current Control Plane code, and Issue #21 in `specs/012-control-plane-invocation-dispatch/research.md`
- [x] T002 Specify and clarify public validation, trusted root correlation, Workspace exact-pin authorization, live proxy, failures, and non-goals in `specs/012-control-plane-invocation-dispatch/spec.md`
- [x] T003 Plan ownership, data flow, strict configuration, tests, and zero fallback in `specs/012-control-plane-invocation-dispatch/plan.md` and supporting design artifacts
- [x] T004 Analyze constitution, Spec, Plan, Tasks, and frozen contracts for consistency before implementation in `specs/012-control-plane-invocation-dispatch/plan.md`

## Phase 2: Foundational Runtime Boundary

- [x] T005 Add required strict Router URL/token, public body, SSE event, and deadline configuration in `apps/control-plane/internal/config/config.go`
- [x] T006 Add root ID construction and one-attempt Router Internal v3 HTTP adapter in `apps/control-plane/internal/invocation/service.go` and `apps/control-plane/internal/invocation/router_client.go`
- [x] T007 Wire Invocation Dispatch and the v4 Gateway route into `apps/control-plane/cmd/control-plane/main.go`

## Phase 3: User Story 1 - Invoke an Authorized Installed Agent (Priority: P1)

**Goal**: Authorize one exact current pin and dispatch one trusted root call only through Router.

**Independent Test**: A fake Workspace and Router receive one exact request; rejected Workspace policy produces no Router call.

- [x] T008 [US1] Add Workspace-owned owner/installation/pin/capability authorization in `apps/control-plane/internal/workspace/service.go` and `apps/control-plane/internal/workspace/model.go`
- [x] T009 [US1] Create exact root context and invoke Router only after authorization in `apps/control-plane/internal/invocation/service.go`
- [x] T010 [US1] Expose the v4 public invoke route and live JSON response proxy in `apps/control-plane/internal/gateway/invocation_handler.go`
- [x] T011 [P] [US1] Add exact-pin and policy tests after implementation in `apps/control-plane/internal/workspace/service_test.go`
- [x] T012 [P] [US1] Add Dispatch and Router direction tests after implementation in `apps/control-plane/internal/invocation/service_test.go` and `apps/control-plane/internal/invocation/router_client_test.go`

## Phase 4: User Story 2 - Reject Invalid Requests Before Dispatch (Priority: P1)

**Goal**: Return exact pre-correlation v4 failures and create no Invocation for invalid public input.

**Independent Test**: Auth, media, JSON shape, identifier, and overflow cases produce zero Dispatch calls and no root identifiers.

- [x] T013 [US2] Implement strict auth-first Content-Type, bounded JSON, duplicate/unknown/required-field, identifier, and Accept validation in `apps/control-plane/internal/gateway/invocation_handler.go`
- [x] T014 [US2] Implement exact Platform Error v4 pre-correlation status/code mapping in `apps/control-plane/internal/gateway/invocation_handler.go`
- [x] T015 [US2] Add invalid request, media, 413, and zero-dispatch tests after implementation in `apps/control-plane/internal/gateway/invocation_handler_test.go`

## Phase 5: User Story 3 - Preserve Typed Router Failures And Live SSE (Priority: P1)

**Goal**: Preserve Router result/error bytes and forward each bounded SSE event immediately.

**Independent Test**: Correlated Router dependency failure remains correlated, JSON bytes remain exact, and two SSE events cause two flushes without full-response buffering.

- [x] T016 [US3] Preserve Router status/media/body and map only local transport/deadline failures with exact correlation in `apps/control-plane/internal/invocation/` and `apps/control-plane/internal/gateway/invocation_handler.go`
- [x] T017 [US3] Implement bounded one-data-line SSE event forwarding and immediate flush in `apps/control-plane/internal/gateway/invocation_handler.go`
- [x] T018 [P] [US3] Add typed failure, exact JSON, SSE framing/limit/flush tests after implementation in `apps/control-plane/internal/gateway/invocation_handler_test.go`
- [x] T019 [P] [US3] Add strict no-default invocation runtime configuration tests after implementation in `apps/control-plane/internal/config/config_test.go`

## Phase 6: Verification And Independent Gates

- [ ] T020 Run formatting, focused/full tests, vet, fallback audit, and compatibility/write-scope review across the repository
- [ ] T021 Complete independent Review by a non-implementing agent (root-owned)
- [ ] T022 Converge findings and repeat independent Review (root-owned)

## Dependencies And Parallelism

- T001-T004 precede runtime implementation.
- T005-T007 establish the shared boundary before user-story completion.
- US1 establishes Dispatch; US2 and US3 refine its public invalid/failure/result surfaces.
- T011/T012, then T018/T019 are parallel test tasks because their files and owners are disjoint.
- T021/T022 remain blocked on T020 and are owned by root supervision.

## Implementation Strategy

The MVP is US1: one authorized non-streaming root request reaches Router with an exact pin. US2 then closes all pre-dispatch rejection paths; US3 adds live SSE and typed downstream failure preservation. No child task creates Router execution, Ledger facts, or deployment integration.
