# Tasks: Router-to-Agent Authentication

**Input**: Design documents from `specs/024-router-agent-authentication/`

**Prerequisites**: `spec.md`, `clarify.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

**Tests**: Required after the corresponding implementation and mapped to Spec acceptance/failure semantics.

**Organization**: Tasks are grouped by user story. Router issuance, Agent SDK verification, sample Runtime wiring, and acceptance files have explicit owners and write scopes.

## Phase 1: Setup

**Purpose**: Pin the reviewed JWT dependency and architecture decision before behavior changes.

- [x] T001 Add `github.com/golang-jwt/jwt/v5` v5.3.1 to `go.mod` and `go.sum` without changing either sample Runtime framework dependency set.
- [x] T002 Record Ed25519 key ownership, exact JWT profile, audience, lifetime, replay, 401/403, compatibility, and deferred rotation decisions in `docs/decisions/0007-router-agent-signed-credential.md`.

---

## Phase 2: Foundational Contracts and Configuration

**Purpose**: Establish the language-neutral credential fact and strict process boundaries used by all stories.

**Critical**: Story implementation starts only after this phase is complete.

- [x] T003 Add Router Invocation Credential v1 Schema, semantic rules, portable claim/header conformance corpus, Go DTO/error mappings, and version constants in `contracts/schemas/router-agent-credential.v1.schema.json`, `contracts/router-agent-credential/v1/`, and `contracts/router_agent_credential.go` per FR-002/FR-006/FR-011.
- [x] T004 Document the authenticated Router-to-Agent companion contract and unchanged A2A Profile 0.2 boundary in `docs/contracts/compatibility.md` and contract discoverability checks without mutating existing Profile 0.2 or adding credential fields to Ledger/result contracts.
- [x] T005 [P] Add required Router issuer/key ID/private-key/TTL parsing and typed signer configuration to `apps/a2a-router/internal/config/config.go` and `apps/a2a-router/internal/credential/config.go` per FR-003.
- [x] T006 [P] Add required Agent issuer/audience/key ID/public-key parsing to `sdks/agent-sdk/routerauth/config.go` per FR-003.

**Checkpoint**: Credential contract and both owning config boundaries fail closed with no key, identity, TTL, or audience defaults.

---

## Phase 3: User Story 1 - Accept only an exact managed invocation (Priority: P1) MVP

**Goal**: Router-signed requests reach a sample Agent; direct or context-mismatched requests do not reach Runtime logic.

**Independent Test**: Send the same valid A2A body through Router and directly. The managed call succeeds, direct call is 401, and a signed context mismatch is 403 before the guarded handler executes.

### Implementation for User Story 1

- [x] T007 [US1] Implement Ed25519 JWT issuance with injected clock/random ID source and exact v1 header/claims in `apps/a2a-router/internal/credential/issuer.go` per FR-001/FR-002/FR-008.
- [x] T008 [US1] Require `http_bearer`, derive canonical endpoint-origin audience, add all signed context headers, and inject a freshly issued Bearer credential in `apps/a2a-router/internal/transport/a2a/target.go`, `client.go`, and `nonstreaming.go` per FR-001/FR-008/FR-013.
- [x] T009 [US1] Implement strict Agent-side signature/issuer/time/claim/header validation and trusted request-context propagation in `sdks/agent-sdk/routerauth/verifier.go`, `headers.go`, and `middleware.go` per FR-004/FR-007.
- [x] T010 [P] [US1] Wire the required execution middleware without protecting readiness or ownership proof routes in `agents/runtime-a/config.go`, `agents/runtime-a/cmd/runtime-a/main.go`, and `agents/runtime-a/handler.go` per FR-012.
- [x] T011 [P] [US1] Wire the same execution middleware around Runtime B JSON-RPC and unavailable fixture routes in `agents/runtime-b/server.go`, `agents/runtime-b/cmd/runtime-b/main.go`, and related config code per FR-012.

### Tests for User Story 1

- [x] T012 [P] [US1] Add Schema/semantic/header/error compatibility tests for managed success and missing required fields in `contracts/router_agent_credential_contracts_test.go` and `contracts/active_contracts_integration_test.go` per US1 acceptance.
- [x] T013 [P] [US1] Add Router config and issuer tests for exact Ed25519 key parsing, required TTL, distinct `jti`, and complete claims in `apps/a2a-router/internal/config/config_test.go` and `apps/a2a-router/internal/credential/issuer_test.go` per SC-001/SC-005.
- [x] T014 [P] [US1] Add non-streaming target and transport tests proving `http_bearer`, canonical audience, one Authorization header, exact release context, and rejection of `none` in `apps/a2a-router/internal/transport/a2a/client_test.go` and `target_test.go`.
- [x] T015 [P] [US1] Add Agent middleware tests for valid managed execution, direct 401, every signed context mismatch 403, and no downstream execution in `sdks/agent-sdk/routerauth/middleware_test.go` per US1 acceptance.
- [x] T016 [US1] Update Runtime A/Runtime B HTTP and configuration tests to prove readiness/challenge compatibility and protected execution in `agents/runtime-a/config_test.go`, `agents/runtime-a/e2e_test.go`, `agents/runtime-b/server_test.go`, and `agents/runtime-b/handler_test.go`.

**Checkpoint**: One exact non-streaming managed request succeeds at both sample adapter boundaries; direct and mismatched calls are rejected.

---

## Phase 4: User Story 2 - Reject invalid and replayed credentials (Priority: P1)

**Goal**: Close forgery, ambiguity, expiry, recipient-confusion, and single-process replay paths with stable 401/403 semantics.

**Independent Test**: Present the negative credential matrix and a concurrent duplicate to the verifier; no invalid request executes and at most one duplicate reaches the handler.

### Implementation for User Story 2

- [x] T017 [US2] Enforce exactly three unpadded compact segments, strict Base64url, duplicate/unknown protected-header and claims rejection, exact `alg`/`typ`/`kid`, and required integer NumericDates in `sdks/agent-sdk/routerauth/verifier.go` per FR-004/FR-006.
- [x] T018 [US2] Implement atomic Agent-local `jti` acceptance and expired-only cleanup in `sdks/agent-sdk/routerauth/replay.go` per FR-005.
- [x] T019 [US2] Implement generic no-store 401/403 Agent authentication responses with `WWW-Authenticate: Bearer` only on 401 in `sdks/agent-sdk/routerauth/errors.go` per FR-006/FR-010.

### Tests for User Story 2

- [x] T020 [P] [US2] Add forged, unsupported algorithm, malformed/padded segment, duplicate/unknown member, missing claim, wrong issuer, expired, future-issued, excessive lifetime, and wrong-audience tests in `sdks/agent-sdk/routerauth/verifier_test.go` per US2 acceptance.
- [x] T021 [P] [US2] Add sequential/concurrent replay and expired-entry cleanup tests with an injected clock in `sdks/agent-sdk/routerauth/replay_test.go` per FR-005/SC-001.
- [x] T022 [P] [US2] Add missing, blank, whitespace, padded, wrong-length, wrong-key, invalid URI/audience, unsafe key-ID, and TTL range tests in `apps/a2a-router/internal/credential/config_test.go` and `sdks/agent-sdk/routerauth/config_test.go` per SC-005.

**Checkpoint**: The complete negative matrix is deterministic, does not expose validation details, and executes no Agent runtime logic.

---

## Phase 5: User Story 3 - Preserve JSON, streaming, and nested lineage (Priority: P2)

**Goal**: Apply the credential to every protocol HTTP request and retain current result, failure, cancellation, and Ledger lineage semantics.

**Independent Test**: Run clean Compose acceptance with direct JSON, SSE, Runtime A→Router→Runtime B nesting, failure, timeout/cancel, direct 401, and secrecy assertions.

### Implementation for User Story 3

- [x] T023 [US3] Replace static streaming/cancel metadata injection with per-call credential issuance and exact signed context in `apps/a2a-router/internal/transport/a2a/streaming.go` so stream and cancel use distinct `jti` values per FR-001/FR-009.
- [x] T024 [US3] Configure required Router signing and per-Agent verification values, update sample Cards to `http_bearer`, and keep test credentials explicitly CI/local-only in `deploy/compose.yaml`, `.github/workflows/ci.yml`, `.env.example`, `tests/fixtures/catalog/runtime-a-card.json`, `tests/fixtures/catalog/runtime-b-card.json`, and `tests/e2e/invoke-record/invoke_record_test.go` per FR-013.
- [x] T025 [US3] Extend E2E setup with a direct unauthenticated Agent request and credential/key/`jti` secrecy sentinels in `tests/e2e/invoke-record/invoke_record_test.go` per SC-002/SC-004.

### Tests for User Story 3

- [x] T026 [P] [US3] Add streaming and cancellation transport tests proving fresh credentials and unchanged stream/cancel classification in `apps/a2a-router/internal/transport/a2a/streaming_test.go` per US3 acceptance.
- [x] T027 [US3] Run and, only where mapped behavior requires, update JSON/SSE/nested/failure/Ledger acceptance assertions in `tests/e2e/invoke-record/invoke_record_test.go` per SC-003/SC-004.

**Checkpoint**: Both Router-to-Agent hops in the cross-runtime call are authenticated and the full Ledger lineage remains metadata-only.

---

## Phase 6: Documentation, Verification, and Delivery Readiness

**Purpose**: Align operator guidance and prove the complete approved scope.

- [x] T028 [P] Update required key generation/configuration, startup failure, direct-request rejection, and credential secrecy guidance in `README.md`, `docs/runbooks/local-development.md`, `docs/architecture/phase-1-spec.md`, `docs/handoffs/CURRENT.md`, and the active-contract/status text in `AGENTS.md`.
- [x] T029 Execute the commands in `specs/024-router-agent-authentication/quickstart.md`, run `git diff --check`, verify no credential fields entered Card/Ledger/result schemas, record the fallback delta, and mark all completed tasks in `specs/024-router-agent-authentication/tasks.md`.

---

## Dependencies & Execution Order

### Phase Dependencies

- Phase 1 has no dependencies.
- Phase 2 depends on Phase 1 and blocks all user stories.
- User Story 1 depends on Phase 2 and supplies the managed success path.
- User Story 2 depends on the US1 verifier seam but is independently testable at that boundary.
- User Story 3 depends on US1/US2 and applies the finished profile to streaming, nested, Compose, and E2E paths.
- Phase 6 depends on all selected stories.

### Module Ownership and Write Coordination

- `contracts/`: T003/T004 before T012; no parallel contract writers.
- Router config/credential: T005/T007 before T013/T022.
- Router transport: T008 before T014 and T023 before T026.
- Agent SDK auth: T006/T009/T017/T018/T019 are sequential; their disjoint tests T015/T020/T021/T022 may run in parallel after implementation.
- Runtime A and Runtime B wiring T010/T011 can run in parallel because their files are disjoint.
- Compose/CI/E2E T024/T025/T027 are sequential because they share deployment and acceptance assumptions.

### Parallel Opportunities

- T005 and T006 are independent config owners.
- T010 and T011 are independent sample Runtime adapters.
- T012, T013, T014, and T015 write disjoint test suites after US1 implementation.
- T020, T021, and the Agent half of T022 write disjoint verifier/replay/config tests after US2 implementation.
- T026 and T028 are disjoint after the runtime implementation stabilizes.

## Parallel Example: User Story 1

```text
Task: T010 wire Runtime A execution middleware
Task: T011 wire Runtime B execution middleware

After implementation:
Task: T012 contract tests
Task: T013 issuer/config tests
Task: T014 transport tests
Task: T015 middleware tests
```

## Implementation Strategy

### MVP First

1. Complete exact contracts and required configuration.
2. Complete US1 signer, non-streaming transport, verifier, and both sample adapters.
3. Validate managed success/direct rejection before adding replay hardening.

### Incremental Delivery

1. US1 establishes one authenticated exact request.
2. US2 closes malformed, forged, temporal, audience, and replay paths.
3. US3 applies the same profile to SSE, cancellation, nested calls, and real acceptance.
4. Documentation/full verification completes delivery; no partial anonymous deployment is considered done.

## Notes

- Implementation precedes its mapped tests per repository policy.
- Credentials and `jti` are transient protocol data, never persistence fields.
- `NEKIRO_ROUTER_AGENT_PRINCIPALS_JSON` remains the separate Agent→Router nested authentication boundary.
- No task authorizes key defaults, clock leeway, multi-key compatibility, retry, alternate endpoint/key source, durable replay, or anonymous fallback.

## Phase 7: Review Remediation

**Purpose**: Close independent-review findings before delivery without
silently changing the approved trust boundary.

- [x] T030 [P] Version the breaking Router Internal dispatch semantic as v4, retain metadata reads on v3, and retire the v3 dispatch route per FR-014 and Constitution V.
- [x] T031 [P] Align the language-neutral credential semantic rules and tests with the canonical HTTPS issuer requirement per Constitution V.
- [x] T032 [P] Reject a present-but-empty `parentInvocationId` claim and add the root/child presence matrix per the credential contract and FR-004.
- [x] T033 [P] Make replay acceptance reject `now >= exp` inside the atomic guard and cover the expiration-boundary clock transition per FR-005.
- [x] T034 [P] Expand Agent authentication tests for multiple/non-Bearer/empty Authorization values, wrong `typ`, and duplicate signed context headers per SC-002 and T020.
- [x] T035 Update compatibility/status/handoff documentation so Issue #50 Compose acceptance remains explicitly pending until CI runs T027/T029.

## Phase 8: Second Review Remediation

**Purpose**: Remove the remaining active/historical contract ambiguity and
ensure the Agent error adapter consumes the credential contract directly.

- [x] T036 [P] Keep `router-internal.v3.yaml` historical, make dispatch v4 self-contained, and add self-contained `router-metadata.v3.yaml` as the active metadata contract per FR-014 and Constitution V.
- [x] T037 [P] Remove the `DispatchInvocationRequestV3` compatibility alias and use `DispatchInvocationRequestV4` throughout all active Go boundaries per FR-014 and the zero-fallback policy.
- [x] T038 [P] Serialize Agent 401/403 bodies from `RouterAgentAuthenticationErrorV1`, remove unreachable nil-cause defaults, and correct the v4 environment guidance per FR-006/FR-011.

## Phase 9: Secrecy Acceptance Remediation

**Purpose**: Make the credential secrecy acceptance detect encoded dynamic
JWT material rather than only plaintext markers.

- [x] T039 Detect compact Router credentials and standalone Ed25519 signature encodings across response, Ledger, and process-log scans, with a focused detector test, per FR-010/SC-004.

## Phase 10: Final Review Remediation

**Purpose**: Close the final independent-review findings before delivery and
keep implementation, SDD artifacts, and verification evidence aligned.

- [x] T040 Correct the issuance-boundary plan to name `RouterInvocationCredentialContextV1` and active `DispatchInvocationRequestV4` per FR-014 and Constitution V/VIII.
- [x] T041 Clarify that README CI run `29810057739` proves the Spec 021 baseline while Spec 024 authenticated Compose acceptance remains pending per T027/T029.
- [x] T042 Apply encoded Router credential/signature detection to successful JSON and every SSE event, published Card responses, and Catalog/Workspace persistent rows per FR-010/SC-004.

## Phase 11: Acceptance Sentinel Remediation

**Purpose**: Preserve legitimate Agent result content while keeping all
credential material forbidden on every public response surface.

- [x] T043 Split credential/key/JTI sentinels from content-only storage/error sentinels so successful JSON/SSE output remains valid while FR-010/SC-004 secrecy checks stay complete.

## Phase 12: CI Compose Remediation

**Purpose**: Make the Runtime B container consume the same thin Agent SDK
adapter already used by its source build.

- [x] T044 Copy `sdks/agent-sdk` into the Runtime B Docker build context before compiling, per FR-012/SC-003 and the failed backend-acceptance build evidence from CI run `30059903275`.

## Phase 13: CI Lint Remediation

**Purpose**: Align new Router credential validation errors with the repository's
Go lint gate without changing failure semantics.

- [x] T045 Lowercase Router credential `Config.Validate` error strings per the ST1005 findings in CI run `30060127995` and re-run the credential tests.

## Phase 14: Complete CI Lint Remediation

**Purpose**: Remove the remaining capitalized internal error strings that the
lint job reports in batches of three.

- [x] T046 Lowercase every Spec 024 internal `errors.New`/`fmt.Errorf` prefix identified by the full inventory after CI run `30060327191`, without changing public error payloads or validation branches.
