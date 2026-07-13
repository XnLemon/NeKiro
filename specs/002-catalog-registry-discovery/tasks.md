# Tasks: Catalog Registration and Discovery

**Input**: Design documents from
`/specs/002-catalog-registry-discovery/`

**Prerequisites**: `spec.md`, `plan.md`, `research.md`, `data-model.md`,
`contracts/catalog-api.md`, and `quickstart.md`

**Tests**: Tests are required and are scheduled after their approved
implementation. Each test task maps to a Spec acceptance scenario, failure
semantic, compatibility requirement, or measurable success criterion.

**Implementation ownership**: One Catalog implementation Agent owns T001-T039
and every file explicitly listed by those tasks. The Agent MUST read and use
`E:\Progarms\contract-agent\.agent-data\codex-home\skills\implement\SKILL.md`,
must work with existing commits, and must not modify Frontend or pnpm supply
chain policy. No second implementation Agent may write these files concurrently.

**Review ownership**: Every Review is read-only and performed by a fresh Agent
that did not implement the reviewed range. It MUST use
`E:\Progarms\contract-agent\.agent-data\codex-home\skills\open-code-review\SKILL.md`.
Test success never substitutes for Review PASS.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel only when file ownership and prerequisites are
  disjoint.
- **[Story]**: Maps the task to one user story in `spec.md`.
- Every task names its exact write or verification path.

## Phase 1: Setup and Contract Gate

**Purpose**: Confirm the SDD gate, complete the active language-neutral Catalog
contract, and establish dependencies before runtime code.

- [x] T001 Verify branch/worktree, repository-local identity, Spec checklist,
  placeholder absence, fallback inventory, and Constitution compliance; record
  the pre-implementation analyze result in
  `specs/002-catalog-registry-discovery/tasks.md`
- [x] T002 Add pinned `pgx/v5 v5.10.0` and `tern/v2 v2.4.1` dependencies without
  unrelated module upgrades in `go.mod` and `go.sum`
  - Evidence: exact direct requirements and only their module checksum pairs
    were added; no existing requirement changed.
- [x] T003 Complete Northbound v2 Catalog Bearer security, trace response
  header, default limit `25`, cursor/filter descriptions, visibility rules, and
  exact operation error responses in `contracts/openapi/control-plane.v2.yaml`
  - Evidence: all five active Catalog operations now declare Bearer security,
    trace headers, visibility/pagination rules, and operation-specific Platform
    Error v2 response sets; the existing OpenAPI regression suite passes.
- [x] T004 Update only required active Catalog DTO mappings or strict public
  request decoders while preserving historical aliases in
  `contracts/contracts.go` and `contracts/validate.go`
  - Evidence: active discovery bounds are mapped once and registration gains a
    duplicate/unknown/trailing-member rejecting envelope decoder; historical
    aliases and artifacts are unchanged and contract tests pass.
- [x] T005 [P] Record Catalog persistence, strong discovery consistency,
  replaceable authentication, migration, compatibility, and deferred deployment
  decisions in `docs/decisions/0004-catalog-persistence-and-consistency.md` and
  `docs/contracts/compatibility.md`
  - Evidence: ADR 0004 records PostgreSQL/pgx/tern ownership, transactional
    Discovery, strict development auth and explicit migration; compatibility
    documents additive v2 completion and no historical runtime path.
- [x] T006 Run existing contract regression tests and `git diff --check`, then
  commit T002-T005 as `feat(contracts): complete catalog API behavior`
  - Evidence: `go test -count=1 ./contracts` and `git diff --check` pass;
    contract-gate work is committed with the required subject.

**Analyze result (2026-07-14)**: PASS. Spec Kit Analyze checked 25 Functional
Requirements, 11 buildable Success Criteria, 19 acceptance scenarios, 47 tasks,
all planned source paths, and all eight Constitution principles. Coverage is
100%; task format is valid 47/47 with continuous unique IDs; no Critical, High,
Medium, Low, ambiguity, duplication, placeholder, unmapped-path, or unmapped-task
finding remains. Implementation is authorized beginning at T002.

**Checkpoint**: Active contract behavior and compatibility are approved before
the first runtime implementation file.

---

## Phase 2: Foundational Runtime

**Purpose**: Create shared Catalog-owned persistence, domain, configuration,
identity, trace, and process foundations that block every user story.

**CRITICAL**: Complete this phase before story-specific handlers or tests.

- [x] T007 Create the `catalog` schema, Agent identity/version/capability tables,
  transactional publication clock, constraints, state/timestamp checks,
  indexes, and tern sections in
  `apps/control-plane/migrations/001_catalog.sql`
  - Evidence: migration 001 owns the Catalog schema, immutable identity/version
    facts, transactional capability index, legal state/timestamp matrix,
    commit-ordered publication clock, deterministic lookup indexes, and
    explicit tern down.
- [x] T008 Add strict embedded tern migration loading and explicit schema
  version/readiness checks in
  `apps/control-plane/internal/catalog/postgres/migrations.go`
  - Evidence: migration content is compiled into the binary, tern accepts only
    explicit up/down commands, and readiness verifies exact version 1 plus all
    four required Catalog relations without creating or upgrading schema.
- [x] T009 Define Agent identity, immutable version, publication state,
  discovery filter/result, domain errors, and the narrow Catalog Store port in
  `apps/control-plane/internal/catalog/model.go` and
  `apps/control-plane/internal/catalog/store.go`
- [x] T010 Implement required configuration parsing with no database, listen,
  auth-mode, principal, or credential defaults in
  `apps/control-plane/internal/config/config.go`
- [x] T011 Implement the replaceable Bearer `Authenticator` and strict explicit
  `development-static` principal-digest adapter with SHA-256 and constant-time
  comparison, without trusting caller-ID headers, in
  `apps/control-plane/internal/gateway/auth.go`
- [x] T012 [P] Implement startup-initialized trace generation and fixed Platform
  Error v2 HTTP mapping without secret/internal detail fields in
  `apps/control-plane/internal/gateway/trace.go` and
  `apps/control-plane/internal/gateway/errors.go`
- [x] T013 Create the standard-library route scaffold, JSON/media/path/query
  boundary helpers, liveness, and readiness handlers in
  `apps/control-plane/internal/gateway/catalog_handler.go`
- [x] T014 Add explicit `serve`, `migrate`, and `healthcheck` command wiring,
  dependency construction, graceful shutdown, and no startup auto-migration in
  `apps/control-plane/cmd/control-plane/main.go`

  - Evidence T009-T014: the domain/store port, strict URL/listen/auth config,
    digest-only Bearer adapter, startup-seeded trace generator, fixed Platform
    Error mapper, health routes, and explicit commands compile without defaults
    or serve-time migration.

**Checkpoint**: The process can be constructed only from explicit valid config,
the Catalog schema has one migration owner, and no story behavior is yet
claimed complete.

---

## Phase 3: User Story 1 - Register an Immutable Agent Version (Priority: P1)

**Goal**: An authenticated owner can register and durably read one conforming,
immutable draft version; invalid, duplicate, and cross-owner attempts fail
without changing Registry facts.

**Independent Test**: Register one valid Card, read it before and after service
reconstruction, and verify invalid/duplicate/cross-owner cases create no
replacement or partial rows.

### Implementation for User Story 1

- [x] T015 [US1] Implement transactional Agent identity claim, immutable version
  insert, capability index insert, exact read, and database error classification
  in `apps/control-plane/internal/catalog/postgres/store.go`
- [x] T016 [US1] Implement active Card validation, exact owner enforcement,
  canonical digest, immutable duplicate semantics, and exact read visibility in
  `apps/control-plane/internal/catalog/service.go`
- [x] T017 [US1] Implement strict register and exact-version Northbound handlers,
  status/error mapping, authentication, and trace correlation in
  `apps/control-plane/internal/gateway/catalog_handler.go`
- [x] T018 [US1] Wire the registration/exact-read service and routes into the
  runnable process in `apps/control-plane/cmd/control-plane/main.go`

### Tests After User Story 1 Implementation

- [x] T019 [P] [US1] Add contract mapping tests for registration, exact read,
  strict Card decoding, security, trace header, and `400/401/403/404/409/503`
  declarations plus historical v1 readability in
  `contracts/catalog_api_contracts_test.go`
- [x] T020 [P] [US1] Add post-implementation service tests for valid, invalid,
  duplicate, byte-equal duplicate, cross-owner, exact-read visibility, and
  immutable Card behavior in `apps/control-plane/internal/catalog/service_test.go`
- [x] T021 [P] [US1] Add strict configuration, authenticator, trace, fixed-error,
  registration HTTP, and exact-read HTTP tests in
  `apps/control-plane/internal/config/config_test.go` and
  `apps/control-plane/internal/gateway/catalog_handler_test.go`
- [x] T022 [US1] Add real migration/register/read/reconstruction and rollback
  acceptance cases using a guarded dedicated test database in
  `tests/integration/catalog/catalog_test.go`

  - Evidence T015-T022: contract/unit tests and the dedicated PostgreSQL HTTP
    suite prove strict active Card registration, `1e400` number preservation,
    immutable duplicates, owner visibility, rollback, committed timestamp
    equality, and restart reconstruction.

**Checkpoint**: User Story 1 independently proves a durable immutable Registry
fact through the Gateway boundary.

---

## Phase 4: User Story 2 - Publish and Disable a Version (Priority: P1)

**Goal**: The owner moves a draft to published once, disables draft/published
versions idempotently, and cannot republish or expose a committed disablement.

**Independent Test**: Seed a draft, publish and discover it, disable it, verify
immediate exclusion and historical exact read, then repeat/compete transitions.

### Implementation for User Story 2

- [x] T023 [US2] Implement row-locked publish, the commit-ordered transactional
  publication clock, idempotent disable transactions, first timestamp/sequence
  preservation, and legal race outcomes in
  `apps/control-plane/internal/catalog/postgres/store.go`
- [x] T024 [US2] Implement owner-authorized publication/disable state policy and
  exact conflict/not-found/forbidden/dependency classification in
  `apps/control-plane/internal/catalog/service.go`
- [x] T025 [US2] Implement publish and disable Northbound handlers and route
  bindings in `apps/control-plane/internal/gateway/catalog_handler.go` and
  `apps/control-plane/cmd/control-plane/main.go`

### Tests After User Story 2 Implementation

- [x] T026 [P] [US2] Add post-implementation lifecycle unit cases for first
  publication time, illegal publish, draft/published disable, repeated disable,
  disabled exact visibility, and owner rejection in
  `apps/control-plane/internal/catalog/service_test.go`
- [x] T027 [P] [US2] Add publish/disable OpenAPI contract mappings plus HTTP
  status, fixed-error, and trace cases in
  `contracts/catalog_api_contracts_test.go` and
  `apps/control-plane/internal/gateway/catalog_handler_test.go`
- [x] T028 [US2] Add real PostgreSQL publish/disable, timestamp, concurrent race,
  immediate eligibility/exclusion, and restart durability cases in
  `tests/integration/catalog/catalog_test.go`

  - Evidence T023-T028: exact-row locks and the transactional clock enforce
    legal publication order; service/HTTP/integration cases cover conflict,
    forbidden, draft/published disable, idempotency, races, immediate
    eligibility/exclusion, and durable first timestamps.

**Checkpoint**: User Stories 1 and 2 independently prove immutable version and
publication lifecycle behavior.

---

## Phase 5: User Story 3 - Discover Published Agent Versions (Priority: P1)

**Goal**: Authenticated users find only exact published versions through literal
text, capability, owner, combined filters, and stable cursor pagination.

**Independent Test**: Seed mixed states/owners/capabilities, traverse more than
one page, mutate publication state between pages, and verify exact result and
cursor semantics.

### Implementation for User Story 3

- [x] T029 [US3] Implement strict filter normalization, explicit default limit,
  cursor v1 encode/decode, duplicate-member rejection, filter hashing, monotonic
  snapshot sequence, and keyset predicates in
  `apps/control-plane/internal/catalog/cursor.go`
- [x] T030 [US3] Implement published-only literal text, exact owner/capability,
  AND filtering, repeatable-read first-page snapshot boundary, ordering,
  look-ahead cursor, and concurrent disable exclusion SQL in
  `apps/control-plane/internal/catalog/postgres/store.go`
- [x] T031 [US3] Implement discovery service validation and the authenticated
  search handler with explicit empty success and dependency failure in
  `apps/control-plane/internal/catalog/service.go` and
  `apps/control-plane/internal/gateway/catalog_handler.go`

### Tests After User Story 3 Implementation

- [x] T032 [P] [US3] Add cursor/filter unit tests for default `25`, explicit
  bounds, literal wildcard escaping, strict payload decoding, filter mismatch,
  ordering tuple, and no silent restart in
  `apps/control-plane/internal/catalog/cursor_test.go`
- [x] T033 [P] [US3] Add search OpenAPI mappings and HTTP tests for
  published-only visibility, free-text/capability/owner/AND filters, empty
  items, default/explicit limit, malformed inputs, authentication, cursor
  errors, and dependency errors in `contracts/catalog_api_contracts_test.go` and
  `apps/control-plane/internal/gateway/catalog_handler_test.go`
- [x] T034 [US3] Add real PostgreSQL 1,000-result traversal, tie ordering, new
  publication exclusion including a lower-sequence transaction delayed past the
  first-page snapshot, between-page disablement, no duplicate/missing result,
  and 10,000-version first-page latency cases in
  `tests/integration/catalog/catalog_test.go`

  - Evidence T029-T034: strict cursor/filter tests and the PostgreSQL suite
    prove literal `%`/`_`, exact AND filters, malformed/filter-mismatched cursor
    rejection, repeatable-read first pages, commit-ordered delayed publication
    exclusion, between-page disablement, 999 expected stable results after one
    disable, and the 10,000-version latency target.

**Checkpoint**: `Register -> Publish -> Discover` is runnable without Workspace,
Router, Agent endpoint, Runtime, or Frontend.

---

## Phase 6: User Story 4 - Preserve Catalog Trust During Failure (Priority: P2)

**Goal**: Failure states remain explicit, committed data survives restart, and
no secret, stale result, inferred identity, or impossible state is exposed.

**Independent Test**: Exercise missing auth, ownership rejection, invalid state,
database loss, migration mismatch, process reconstruction, and secret/log
exclusions through the actual HTTP and PostgreSQL boundaries.

### Implementation for User Story 4

- [x] T035 [US4] Complete dependency failure, readiness/schema mismatch,
  secret-safe structured logging, response header/body trace equality, and
  request-body exclusion behavior in
  `apps/control-plane/internal/gateway/catalog_handler.go`,
  `apps/control-plane/internal/gateway/errors.go`, and
  `apps/control-plane/cmd/control-plane/main.go`
- [x] T036 [US4] Add the runnable Control Plane container and explicit local
  configuration with no application defaults in
  `apps/control-plane/Dockerfile`, `deploy/compose.yaml`, and `.env.example`
- [x] T037 [US4] Document migration, readiness, generated development auth,
  process startup/shutdown, data persistence, and destructive test-database
  guard in `docs/runbooks/local-development.md` and `README.md`

### Tests After User Story 4 Implementation

- [x] T038 [P] [US4] Add post-implementation missing/blank/invalid config,
  unsupported auth mode, malformed/duplicate principal digests, constant-time
  token authentication, token/digest redaction,
  dependency failure, readiness, and trace equality tests in
  `apps/control-plane/internal/config/config_test.go` and
  `apps/control-plane/internal/gateway/catalog_handler_test.go`
- [x] T039 [US4] Add Runtime-A/Runtime-B Card fixtures plus complete HTTP
  Register -> Publish -> Discover -> Disable, migration idempotency, restart,
  under-two-minute primary workflow, failure injection, no-fallback, secret
  exclusion, and dedicated-database guard acceptance in
  `tests/fixtures/catalog/runtime-a-card.json`,
  `tests/fixtures/catalog/runtime-b-card.json`, and
  `tests/integration/catalog/catalog_test.go`

  - Evidence T035-T039: schema rename/version mismatch and dependency tests stay
    explicit, response/header traces match, captured logs exclude credentials
    and Card content, strict config rejects libpq defaults, migration down/up is
    verified, two Runtime-independent fixtures complete the Catalog workflow,
    and the `_test` database suffix guard protects destructive acceptance.

**Checkpoint**: All four user stories and explicit failure semantics are
implemented and independently testable.

---

## Phase 7: Integration, Verification, and Review

**Purpose**: Integrate repository automation, execute every acceptance command,
and require a fresh independent Review before convergence.

- [x] T040 Update CI to provision a dedicated PostgreSQL service, run migrations
  and integration-tagged Catalog tests, and leave pnpm lockfile and
  `minimumReleaseAge` policy unchanged in `.github/workflows/ci.yml`
- [x] T041 Run `gofmt` on all changed Go files and execute every command in
  `specs/002-catalog-registry-discovery/quickstart.md`, including default,
  integration, race, vet, build, Compose config, and diff checks; record the
  implementation fallback delta/evidence and verify dependency/scope scans show
  no Agent Runtime, Workspace, Router, Ledger, Frontend, historical runtime, or
  deployment implementation in
  `specs/002-catalog-registry-discovery/tasks.md`
  - Evidence: contract, Control Plane, integration, full repository, split race,
    vet, binary build, Compose config, pinned Docker image build, module tidy,
    and `git diff --check` passed on 2026-07-14. Scope scans found no Runtime
    framework, Workspace, Router, Ledger, Frontend, historical runtime,
    in-memory store, cache, or retry implementation; pnpm policy files and
    historical v1/0.1 artifacts are unchanged.
  - Fallback delta: removed `0`, retained `3`, added `0`, net `0`.
    Added fallback evidence: none. Retained policies remain omitted limit `25`,
    genuine empty discovery, and idempotent disablement.
- [x] T042 Commit runtime and post-implementation test work with repository-local
  identity using logical Catalog commits, leaving the worktree clean and without
  push; record commit subjects in
  `specs/002-catalog-registry-discovery/tasks.md`
  - Evidence: repository-local identity created `docs(spec): serialize catalog
    publication order`, `feat(control-plane): implement catalog registration
    and discovery`, and `test(catalog): verify postgres and http workflows`;
    the final status-document commit completes the clean no-push checkpoint.
- [x] T043 Fetch `origin`, resolve its current HEAD, rebase the clean feature
  branch without force onto that remote base, rerun the full quickstart, and
  record the exact base in `specs/002-catalog-registry-discovery/tasks.md`
  - Evidence: fetched and pruned `origin`, resolved `origin/HEAD` to
    `origin/main` at `3bcb844`, and rebased without force; the branch was already
    current with that base. Post-rebase verification is recorded with T041.
- [ ] T044 Create a fresh independent Review Agent for the complete rebased
  Spec 002 diff; fix every High or Medium finding through the original
  implementation Agent, update Spec/Tasks before behavioral fixes, and use a new
  Reviewer after every fix until explicit PASS is recorded in
  `specs/002-catalog-registry-discovery/tasks.md`
  - Review round 1: Reviewer `019f5ce2-28ae-7352-a31f-206561292e97` used
    `open-code-review`; `ocr` reviewed 23 files and returned 8 comments. Four
    validated Medium findings make the round FAIL: unbounded body reads,
    machine-range Agent limits, incomplete Publication Clock readiness, and
    insufficiently synchronized concurrency coverage.
  - Review round 2: fresh boundary-focused Reviewer
    `019f5d04-ca84-7372-993b-d055ff874608` did not use OCR and returned
    `High 0`, `Medium 1`, `Low 0`: PostgreSQL `jsonb` rejects an otherwise
    valid unbounded number such as `1e131072`, so FR-001 fails at persistence.
  - Review round 3: fresh boundary-focused Reviewer
    `019f5d19-b583-7083-97b6-d0c33a1b84de` did not use OCR and validated the
    unbounded-number remediation, but returned `High 0`, `Medium 2`, `Low 0`:
    public `migrate down` deletes an ordinary populated Catalog, and the
    connection-level 30-second `ReadTimeout` includes header time rather than
    providing the specified registration-body window.
  - Review round 4: fresh boundary-focused Reviewer
    `019f5d2f-2e36-7923-b258-22ac42610c33` did not use OCR and validated the
    migration/deadline remediations, but returned `High 0`, `Medium 1`, `Low 0`:
    exact duplicate registration by a different owner returns forbidden before
    reaching the FR-005 conflict rule, while the contract described both codes.

### Review Round 1 Remediation

- [x] T048 [Review-R1] Enforce the Spec-defined 16,777,216-byte registration
  cap and 30-second request-body read window with fixed validation/no-partial
  persistence semantics in `contracts/openapi/control-plane.v2.yaml`,
  `apps/control-plane/internal/gateway/catalog_handler.go`, and
  `apps/control-plane/cmd/control-plane/main.go`
- [x] T049 [Review-R1] Preserve active unbounded Agent Card limit integers as
  exact `json.Number` values and add beyond-`int64` contract/Catalog round-trip
  coverage in `contracts/contracts.go`, contract tests, and Catalog acceptance
- [x] T050 [Review-R1] Require exactly one valid Publication Clock singleton in
  readiness and test missing-row failure in
  `apps/control-plane/internal/catalog/postgres/migrations.go` and acceptance
- [x] T051 [Review-R1] Synchronize lifecycle race starts and add concurrent
  duplicate registration atomicity acceptance in
  `tests/integration/catalog/catalog_test.go`
  - Evidence T048-T051: Gateway caps registration at 16 MiB with a 30-second
    server read window; active limit integers use `json.Number` and `1e400`
    survives validation/PostgreSQL round-trip; readiness rejects a missing
    clock singleton; lifecycle competitors share a start barrier and 12
    concurrent duplicate registrations produce one create plus 11 conflicts.
    Default, integration, split race, vet, build, and diff checks pass.
  - Review remediation fallback delta: removed `0`, retained `3`, added `0`,
    net `0`. Added fallback evidence: none.

### Review Round 2 Remediation

- [x] T052 [Review-R2] Add an ordered Catalog schema-v2 migration that converts
  the immutable Card fact from PostgreSQL `jsonb` to JSON `text`, backfills
  transactionally derived `card_name` and `card_description`, and makes
  readiness require schema v2 in `apps/control-plane/migrations/`,
  `apps/control-plane/internal/catalog/postgres/migrations.go`, and migration
  tests
- [x] T053 [Review-R2] Persist canonical Card text and derived free-text query
  columns without PostgreSQL numeric coercion, while preserving immutable
  digest, owner/capability transactionality, exact reads, and Discovery in
  `apps/control-plane/internal/catalog/postgres/store.go`
- [x] T054 [Review-R2] Prove a conforming Card containing `1e131072` registers,
  survives PostgreSQL storage and process reconstruction, and round-trips
  through exact read and Discovery; retain the existing `1e400` contract
  coverage in `tests/integration/catalog/catalog_test.go` and relevant migration
  tests
  - Evidence T052-T054: schema migration v2 preserves existing v1 Cards while
    replacing `jsonb` with text plus required derived name/description columns;
    readiness requires v2. A fresh PostgreSQL 17 container passed migration
    up/up/down/up and the real HTTP acceptance, including `1e131072` storage,
    publication, Discovery, process reconstruction, and exact read. Default and
    full race tests, vet, binary and pinned Docker builds, Compose rendering,
    tidy-diff, formatting, and diff checks pass.
  - Review round 2 remediation fallback delta: removed `0`, retained `3`, added
    `0`, net `0`. Added fallback evidence: none.
- [ ] T055 [Review-R2] Run default, real PostgreSQL integration, split race,
  vet, build, Compose, migration, tidy-diff, and diff verification; report
  fallback delta and create a fresh non-OCR independent Reviewer

### Review Round 3 Remediation

- [x] T056 [Review-R3] Restrict the public migration command to `up` and reject
  `down` or any other direction before invoking tern or changing Catalog data in
  `apps/control-plane/cmd/control-plane/main.go`,
  `apps/control-plane/internal/catalog/postgres/migrations.go`, and tests
- [x] T057 [Review-R3] Replace the connection-wide registration `ReadTimeout`
  with a separate five-second header deadline and a 30-second read deadline set
  when registration body processing begins; fail before persistence when the
  deadline cannot be enforced in `apps/control-plane/cmd/control-plane/main.go`,
  `apps/control-plane/internal/gateway/catalog_handler.go`, and tests
- [x] T058 [Review-R3] Add real PostgreSQL acceptance proving unsupported
  `migrate down` leaves an ordinary populated Catalog and schema v2 unchanged,
  while isolated tern v1/v2 migration verification remains explicit in
  `tests/integration/catalog/catalog_test.go`
- [x] T059 [Review-R3] Verify the body deadline is independent of header time,
  clears after a complete body read, and never passes partial/timed-out content
  to Catalog using production-compatible ResponseController and socket coverage
  - Evidence T056-T059: the CLI and migration adapter reject every non-`up`
    direction before database mutation; a populated real PostgreSQL schema v2
    remains byte-for-byte equivalent across the rejected actual-binary command.
    Registration now has a separate five-second header deadline and starts its
    30-second body deadline inside the handler. Real socket coverage proves a
    delayed header does not consume the body window, a completed read clears
    the deadline for connection reuse, and a partial body times out without a
    Catalog call. Deadline-control failures also stop before persistence.
    Default tests, five repeated socket rounds, real integration, full race,
    vet, binary and pinned Docker builds, Compose, tidy-diff, formatting, and
    diff checks pass.
  - Review round 3 remediation fallback delta: removed `0`, retained `3`, added
    `0`, net `0`. Added fallback evidence: none.
- [ ] T060 [Review-R3] Run the complete verification matrix, report fallback
  delta, and create another fresh non-OCR independent Reviewer

### Review Round 4 Remediation

- [x] T061 [Review-R4] After locking the stable Agent identity, detect an
  existing exact `(agent_id, version)` before evaluating owner mismatch and
  return fixed conflict without exposing stored metadata in
  `apps/control-plane/internal/catalog/postgres/store.go`
- [x] T062 [Review-R4] Add real PostgreSQL/HTTP acceptance proving cross-owner
  exact duplicates return `409 CONFLICT` with the original Card unchanged,
  while a cross-owner different version remains `403 FORBIDDEN`, in
  `tests/integration/catalog/catalog_test.go`
  - Evidence T061-T062: registration checks exact version existence while the
    stable Agent identity row is locked and before owner mismatch, retaining the
    primary key as the final concurrent conflict guard. Fresh PostgreSQL/HTTP
    acceptance proves a cross-owner exact duplicate returns fixed 409 without
    metadata disclosure or fact mutation, while a different cross-owner version
    returns fixed 403. Default, integration, full race, vet, build, Compose,
    tidy-diff, formatting, and diff checks pass.
  - Review round 4 remediation fallback delta: removed `0`, retained `3`, added
    `0`, net `0`. Added fallback evidence: none.
- [ ] T063 [Review-R4] Run the complete verification matrix, report fallback
  delta, and create another fresh non-OCR independent Reviewer

---

## Phase 8: Convergence and Handoff

- [ ] T045 Run `speckit-converge` against the implemented repository and append
  only genuine remaining work to
  `specs/002-catalog-registry-discovery/tasks.md`
- [ ] T046 Map FR-001 through FR-025 and every US1-US4 acceptance scenario to
  implemented artifacts and passing tests, confirm total added fallback is zero,
  and mark Spec complete only after Review PASS in
  `specs/002-catalog-registry-discovery/spec.md` and
  `specs/002-catalog-registry-discovery/tasks.md`
- [ ] T047 Update current repository status, remaining non-goals, and the next
  Phase 1 feature entry point in `docs/handoffs/CURRENT.md`, `AGENTS.md`, and
  `README.md`, then commit finalized SDD/handoff artifacts without push

---

## Dependencies and Execution Order

### Phase Dependencies

- Phase 1 contract gate blocks all runtime implementation.
- Phase 2 foundational runtime blocks every user story.
- US1 establishes immutable version registration and exact reads.
- US2 depends on US1's exact version/store behavior.
- US3 depends on US1 and US2 because only published versions are discoverable.
- US4 evaluates all prior behavior under failure and process reconstruction.
- Integration/Review begins only after US1-US4 implementation and mapped tests.
- Convergence begins only after the final independent Review explicitly passes.

### User Story Dependencies

- **US1 (P1)**: Starts after Phase 2; independently demonstrates durable draft
  registration and read.
- **US2 (P1)**: Reuses US1 version rows and adds publication state transitions.
- **US3 (P1)**: Reuses published US2 rows and adds Discovery/cursor behavior.
- **US4 (P2)**: Cross-cuts all stories only after their successful paths exist.

### Parallel Opportunities

- T005 may proceed in parallel with T002-T004 because its documentation files
  are disjoint.
- T011 and T012 are disjoint after T009 defines shared domain values.
- Post-implementation test tasks marked `[P]` write disjoint test files and may
  run concurrently only after their story implementation is complete.
- No implementation Agent parallelism is authorized for shared domain, Gateway,
  or pgx files in this feature.

## Parallel Example: User Story 1 Tests

```text
After T015-T018 are complete:
Task T019: contracts/catalog_api_contracts_test.go
Task T020: apps/control-plane/internal/catalog/service_test.go
Task T021: config/gateway test files
```

## Implementation Strategy

### MVP First

1. Complete contract and foundation phases.
2. Implement US1 registration and exact reads.
3. Add US1 tests after implementation and validate independently.
4. Continue immediately through publication and Discovery to close the requested
   second-feature workflow; do not claim Phase completion at draft-only MVP.

### Incremental Delivery

1. Immutable draft Registry fact.
2. Owner-controlled publication and disablement.
3. Published-only capability Discovery and cursor traversal.
4. Failure/durability hardening and runnable operations.
5. CI, full quickstart, fresh Review, and convergence.

## Notes

- Implementation precedes its mapped tests; TDD is not the required sequence.
- The fallback addition budget is zero. Helpers, retry loops, stale caches,
  anonymous identity, default DSNs, in-memory stores, auto-migration, and
  historical-version branches still count as fallback behavior.
- A genuine no-match empty list, default page size 25, and idempotent disable
  are retained Spec policies and must not be generalized into failure fallback.
- Frontend, Workspace, Invocation, Router, Ledger, deployment, health polling,
  cold storage, Marketplace, and Agent Runtime features remain out of scope.
- Do not push unless the user explicitly requests it.
