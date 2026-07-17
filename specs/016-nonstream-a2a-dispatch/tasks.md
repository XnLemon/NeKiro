# Tasks: Non-Streaming A2A Dispatch

**Input**: Design documents from `specs/016-nonstream-a2a-dispatch/`

**Scope**: Implement Router-owned non-streaming exact A2A dispatch and
transient result delivery. Tests are added after the corresponding
implementation work, per project policy.

## Phase 1: Setup

- [X] T001 Create the Router non-streaming transport package skeleton in `apps/a2a-router/internal/transport/a2a/` with explicit dependencies and no fallback endpoint/default credential behavior.

---

## Phase 2: Foundational Mapping

- [X] T002 Define focused Router-side interfaces for non-streaming Agent transport and metadata-only Ledger append ordering in `apps/a2a-router/internal/api/dispatch_handler.go` without importing Control Plane internals.
- [X] T003 Map resolved Agent endpoint/profile/auth facts into a strict non-streaming transport target in `apps/a2a-router/internal/transport/a2a/`, rejecting unsupported states explicitly.

**Checkpoint**: Router can represent an exact resolved non-streaming target but still has no Agent side effect.

Checkpoint evidence: `apps/a2a-router/internal/transport/a2a` now validates
resolved target endpoint/profile/auth/capability without fallback endpoints or
default credentials. `dispatch_handler.go` now declares narrow non-streaming
transport and Ledger append interfaces for later wiring, without Control Plane
internal imports or Agent side effects.

---

## Phase 3: User Story 1 - Dispatch a Non-Streaming Invocation

**Goal**: Replace the `ROUTE_NOT_FOUND` placeholder for `stream=false` with one A2A `message/send` call and live JSON result.

**Independent Test**: Runtime B `httptest` endpoint receives one call with platform context and the Router returns the deterministic result.

- [X] T004 [US1] Implement A2A `message/send` client behavior in `apps/a2a-router/internal/transport/a2a/`.
- [X] T005 [US1] Wire successful non-streaming transport into `apps/a2a-router/internal/api/dispatch_handler.go` while preserving existing validation/resolution failures.
- [X] T006 [US1] Add mapped Runtime B success and context propagation tests in `apps/a2a-router/internal/api/dispatch_handler_test.go` and/or `apps/a2a-router/internal/transport/a2a/` tests.

Checkpoint evidence: `NewDispatchHandlerWithTransport` now injects the
Router-owned non-streaming A2A transport while the existing constructor keeps
the previous placeholder path for pre-existing validation and resolution tests.
`apps/a2a-router/internal/transport/a2a/nonstreaming.go` maps a validated
dispatch request and exact resolved Card into one A2A `message/send` call and
returns a transient Invocation Result v1 payload without result persistence.
Focused verification passed:
`go test -count=1 ./apps/a2a-router/internal/api ./apps/a2a-router/internal/transport/a2a ./agents/runtime-b`.
Interim static verification also passed after the T005-T006 patch:
`go test -count=1 ./apps/a2a-router/... ./agents/runtime-b/...`,
`go test ./...`, `go vet ./...`, `git diff --check`, and a focused
Router dispatch/transport fallback scan with no hits.

---

## Phase 4: User Story 2 - Record Metadata-Only Lifecycle Facts

**Goal**: Commit required safe Ledger facts around accepted non-streaming dispatch without storing Agent content.

**Independent Test**: Strict recorder verifies ordering and metadata-only terminal facts for success and accepted failure.

- [X] T007 [US2] Add Router Ledger append orchestration for accepted non-streaming dispatch in `apps/a2a-router/internal/api/dispatch_handler.go`.
- [X] T008 [US2] Add mapped success, accepted failure, and Ledger failure tests proving terminal success is not emitted before required Ledger commit.

Checkpoint evidence: `NewDispatchHandlerWithTransportAndLedger` now appends
metadata-only Invocation Event v0.3 facts for accepted non-streaming dispatch:
`created -> routing -> started -> succeeded/failed/timed_out`. The success
path appends the terminal `succeeded` fact before returning the live
Invocation Result v1 payload; Ledger append failures return correlated
`DEPENDENCY_ERROR` and do not expose a successful result. Recorder tests cover
success ordering, transport failure terminalization, terminal Ledger failure,
and pre-transport Ledger failure skipping the Agent call. Focused verification
passed: `go test -count=1 ./apps/a2a-router/internal/api` and
`go test -count=1 ./apps/a2a-router/... ./agents/runtime-b/...`.

---

## Phase 5: User Story 3 - Preserve Boundaries and Failure Semantics

**Goal**: Keep pre-existing validation/resolution behavior and add explicit Agent transport failure mapping only.

**Independent Test**: Failure matrix covers unsupported target/profile/auth, endpoint dependency failure, protocol failure, Agent business failure, and no forbidden dependencies/fallbacks.

- [X] T009 [US3] Implement explicit transport failure classification without retries, caches, compatibility branches, or fallback endpoints.
- [X] T010 [US3] Add failure matrix and fallback/write-scope scan evidence to this tasks file.

Checkpoint evidence: the Router transport now exposes only a generic
`PlatformErrorCode()` capability across the API seam. Target/profile contract
violations map to `A2A_PROTOCOL_ERROR`; unsupported Card authentication maps to
`AGENT_AUTH_UNSUPPORTED`; HTTP status and network failures map to
`AGENT_UNAVAILABLE`; malformed A2A envelopes/results map to
`A2A_PROTOCOL_ERROR`; A2A JSON-RPC `*a2a.Error` maps to
`AGENT_EXECUTION_FAILED`; deadline errors map to `TIMEOUT`; and canceled Agent
tasks map to `CANCELED`/HTTP 409. Non-streaming message/task results and raw
JSON-RPC envelopes are validated against the active A2A Profile before they
are returned, including media type, version, response ID, result/error XOR,
unknown members, and duplicate members. API failure-matrix tests cover these
classifications and their 409/502/503/504 statuses. Transport tests cover
target rejection, JSON-RPC Agent failure, malformed/invalid results, HTTP
failure, response overflow, and deadline mapping.

Fallback/write-scope evidence: implementation-only scan over
`apps/a2a-router/internal/api/dispatch_handler.go`,
`apps/a2a-router/internal/transport/a2a/`, strict Router config/assembly, and
the Runtime B media-type adapter found no Control Plane imports,
database writes, result persistence, retry, cache, alternate endpoint,
compatibility branch, default credential, or fallback endpoint. The broader
test scan's only `retry` match is the pre-existing test name
`TestDispatchMapsResolutionDependencyWithoutRetry`; it does not add retry
behavior. `git diff --check` passed. Fallback delta for T009-T010:
removed 0, retained 0, added 0, net 0; added fallback evidence: none.

---

## Phase 6: Verification, Review, and Converge

- [X] T011 Run formatting, focused Router/Runtime B tests, WSL race where practical, full repository tests, vet, diff check, and record verification evidence.
- [X] T012 Obtain fresh independent Review against Spec, Plan, Tasks, active contracts, and constitution; return findings to Spec/Tasks before fixes.
- [X] T013 Run Converge after Review and resolve the in-scope findings in the Router transport/API seam.
- [X] T014 [US2] Wire the production Router assembly to a required DB-backed Ledger appender and strict Ledger schema readiness in `apps/a2a-router/cmd/a2a-router/` and `apps/a2a-router/internal/config/`.
- [X] T015 [US3] Add required Agent response/A2A event byte-limit configuration and enforce the non-stream effective input/output bounds as the minimum of configured and exact Card limits.
- [X] T016 [P1] Add a deployment-owned Ledger migration command/service before Router startup; the current Router intentionally fails readiness when the schema is absent and never auto-migrates.
- [X] T017 [P1] Implement A2A event/SSE byte-limit enforcement with streaming in Spec 017; T015 only establishes required configuration and non-stream Agent response enforcement.
- [X] T018 [P2] Execute the complete active A2A negative corpus matrix (missing result/error, invalid scalar IDs, and trailing data) as explicit Router transport tests.

## Dependencies & Execution Order

```text
T001 -> T002 -> T003 -> T004 -> T005 -> T006 -> T007 -> T008 -> T009 -> T010 -> T011 -> T012 -> T013 -> T014 -> T015 -> T016 -> T018
```

## Requirement Coverage

| Requirement | Tasks |
| --- | --- |
| FR-001, FR-004 | T004-T006 |
| FR-002, FR-010 | T002, T005, T010 |
| FR-003 | T003, T006 |
| FR-005 | T003, T009-T010, T015 |
| FR-006, FR-007, FR-008 | T007-T008, T014 |
| FR-009 | T001, T009-T011, T015 |
| SC-001-SC-004 | T006, T008, T010-T015, T018 |

## Completion State

- Implementation and mapped tests: T005-T018 complete across the non-stream slice and its approved Spec 017 streaming follow-up
- Independent Review: complete; no P0 findings
- Converge: complete for in-scope findings
- Fallback delta: removed 0, retained 0, added 0, net 0; added fallback evidence: none

T011 verification evidence: focused Router/Runtime B tests, full repository
tests, vet, and diff check all passed. The practical WSL race gate also passed:
`wsl.exe -d Ubuntu-26.04 -- bash -lc 'cd /mnt/e/NeKiro && go test -race -count=1 ./apps/a2a-router/... ./agents/runtime-b'`.
The same full, vet, diff, and WSL race gates were rerun after T014-T015 and
the final transport/config/preflight changes; all passed.

Review and Converge evidence: an independent Review Agent found no P0 issue.
The in-scope findings were resolved by raw A2A JSON-RPC envelope/media
validation, `CANCELED` HTTP 409 mapping, production Ledger constructor wiring,
strict database/limit configuration, bounded Agent response reads, and
configured/Card minimum input/output bounds. Spec 017 tasks T004, T006, and
T011-T013 complete T017 with separate upstream A2A event and full SSE frame
limits plus boundary tests; the active negative corpus is complete in T018.

T016 deployment evidence: the Router binary exposes `migrate up` and `serve`
commands; `a2a-router-migrate` runs the embedded Ledger migration and the
Compose Router service depends on its successful completion and the healthy
Control Plane. `docker compose --file deploy/compose.yaml config --quiet`
passed with all required non-empty environment values supplied explicitly.

Final post-review verification after the target-validation lifecycle fix and
local Router runbook update also passed:

```text
go test -count=1 ./...
go vet ./...
git diff --check
wsl.exe -d Ubuntu-26.04 -- bash -lc 'cd /mnt/e/NeKiro && go test -race -count=1 ./apps/a2a-router/... ./agents/runtime-b'
docker compose --file deploy/compose.yaml config --quiet
```

The Ledger target-validation failure case now records exactly
`created -> routing -> failed`, preserves `AGENT_AUTH_UNSUPPORTED`, and never
appends `started` or calls the Agent. `ValidateNonStreamingTarget` passes the
dispatch capability to exact target construction, so malformed Cards cannot
panic or silently select a different skill.

Review follow-up: target validation now runs before non-streaming input
preflight. An unsupported endpoint/profile/auth/capability therefore wins over
an oversized input and receives the correlated target failure plus Ledger
terminal facts; a valid target with oversized input still returns the existing
pre-correlation `PAYLOAD_TOO_LARGE` without accepting Ledger facts. Fresh
standards/spec review after this change found no remaining blocking issue.

T018 evidence: `TestClientRejectsActiveA2ANegativeCorpus` covers missing
`result`/`error`, boolean/object/array response IDs, and trailing JSON data;
each case maps to `A2A_PROTOCOL_ERROR`; response ID type validation runs before
request/response ID equality, so invalid IDs cannot pass only by mismatch. The
test runs against the active JSON-RPC transport path and complements the
existing duplicate-member,
unknown-field, version, media-type, ID-mismatch, and result/error-XOR cases.
T017 is completed by the separately owned streaming implementation and tests in
`specs/017-streaming-a2a-events/`; Spec 016 still does not own streaming
transport or event sequencing.
