# Workspace Acceptance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use the project `speckit-implement` workflow to execute `tasks.md` task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prove the merged Workspace and Installation slices as one durable,
trusted, concurrent Control Plane workflow without adding runtime behavior.

**Architecture:** Keep Catalog and Workspace ownership unchanged. Add an
integration-tagged acceptance harness that assembles the existing PostgreSQL
stores, services, authenticators, and Gateway handlers under `httptest`; use
the existing service-level PostgreSQL tests for durable and race evidence, and
strengthen their result assertions where Issue #9 requires per-operation proof.
No production route, contract, migration, Router, Ledger, or Agent endpoint is
introduced.

**Tech Stack:** Go, `net/http/httptest`, existing Control Plane Catalog and
Workspace services, `pgx/v5`, PostgreSQL integration tags, active JSON/OpenAPI
contracts, Docker Compose validation.

## Global Constraints

- Base branch is `main@99e52aa`; work only on `codex/issue-9-acceptance`.
- Keep Catalog as Agent Card/version fact owner and Workspace as Workspace /
  Installation fact owner.
- Use active public Northbound v3 and internal resolution v2 routes only.
- Do not add public or internal contracts, migrations, fallback routes, retries,
  caches, alternate data sources, endpoint probes, or degraded success.
- Test setup may seed a published Card through Catalog service; the acceptance
  path must cross the real Gateway handlers and PostgreSQL stores.
- Integration tests require `NEKIRO_TEST_DATABASE_URL` ending in `_test`; missing
  prerequisites are reported as not run.
- PostgreSQL schema-resetting packages run serially.
- Per repository policy, implementation/test evidence is added after the
  approved design and not by inventing behavior through a new test expectation.

## File Map

| File | Responsibility |
| --- | --- |
| `apps/control-plane/internal/workspace/integration/workspace_test.go` | Strengthen the existing lifecycle/reinstall race to retain successful rows and validate legal outcomes, immutable facts, and terminal identity. |
| `apps/control-plane/internal/workspace/integration/acceptance_http_test.go` | Assemble real Catalog/Workspace PostgreSQL services and both Gateway handlers; exercise public discovery/Workspace routes and internal resolution through `httptest`. |
| `specs/009-workspace-acceptance/spec.md` | User value, acceptance scenarios, requirements, measurable outcomes, and non-goals. |
| `specs/009-workspace-acceptance/research.md` | Baseline, evidence gaps, decisions, and clarification result. |
| `specs/009-workspace-acceptance/data-model.md` | Existing fact ownership and acceptance invariants; no new entity. |
| `specs/009-workspace-acceptance/contracts/acceptance-evidence.md` | Active route/error contract reuse and compatibility decision. |
| `specs/009-workspace-acceptance/quickstart.md` | Reproducible static, PostgreSQL, and Compose verification commands. |
| `specs/009-workspace-acceptance/checklists/requirements.md` | Spec quality gate. |
| `specs/009-workspace-acceptance/tasks.md` | Dependency-ordered implementation, verification, review, and converge record. |
| `docs/handoffs/CURRENT.md` | Final handoff state and next scope after acceptance closure. |

## Task 1: Strengthen Lifecycle Race Evidence

**Files:**

- Modify: `apps/control-plane/internal/workspace/integration/workspace_test.go`

**Interfaces:**

- Consumes: existing `workspace.Service`, `contracts.Installation`,
  `workspace.ErrConflict`, and PostgreSQL integration helpers.
- Produces: stronger assertions in
  `TestConcurrentLifecycleAndReinstallRequestsPreserveOneCurrentRow`.

1. Replace error-only race channels with a local result record containing the
   operation name, returned `contracts.Installation`, and error.
2. Keep the existing 100-request phases: duplicate disable, then concurrent
   uninstall/reinstall.
3. Count outcomes explicitly. The duplicate-disable phase must have one
   success and 99 `ErrConflict` results.
4. Assert that the disable success is the original identity, has `disabled`
   status, a later `UpdatedAt`, no terminal timestamp, and unchanged pin and
   permission fields.
5. Assert that every non-conflict uninstall result is the original identity,
   terminal, has `UninstalledAt == UpdatedAt`, and preserves immutable fields.
6. Assert that every non-conflict reinstall result is `enabled`, has a new
   identity, and preserves the requested exact pin and permissions.
7. Read the committed history after the race and verify every row is one of the
   observed legal results, exactly one original terminal row exists, and at most
   one current row remains.
8. Run the package with the integration tag and the focused test before moving
   to the HTTP harness.

## Task 2: Add Real Composed HTTP Acceptance

**Files:**

- Create: `apps/control-plane/internal/workspace/integration/acceptance_http_test.go`

**Interfaces:**

- Consumes: `catalogpostgres.NewStore`, `workspacepostgres.NewStore`,
  `catalog.NewService`, `workspace.NewService`,
  `gateway.NewHandler`, `gateway.NewWorkspaceHandler`,
  `gateway.NewDevelopmentStaticAuthenticator`, and the existing migration /
  fixture helpers.
- Produces: integration-tagged tests named with the `TestAcceptance` prefix
  and helper `newAcceptanceHTTPHarness`.

1. Build a fresh dedicated database harness using the existing
   `integrationDatabaseURL` policy, drop/recreate only the owned Catalog and
   Workspace schemas, create both PostgreSQL stores and services, and register
   the existing published fixture Card.
2. Create separate development-static northbound and internal principals from
   deterministic test tokens. Do not reuse the northbound token for internal
   resolution.
3. Construct a real `http.ServeMux` by registering Catalog and Workspace
   handlers on the same mux, then serve requests with `httptest`.
4. Add `TestAcceptanceWorkspaceControlPlaneHTTPWorkflow` that sends, in order:
   - `GET /v3/agents?capability=document.read` with the public token;
   - `POST /v3/workspaces` with the public token;
   - `POST /v3/workspaces/{workspaceId}/installations` with a permission set;
   - list/detail inspection requests;
   - PATCH disable, PATCH enable, PATCH disable, and DELETE uninstall;
   - `POST /internal/v2/resolve-agent` with the separate internal token while
     the Installation is enabled.
5. Decode each success body into the active contract DTO, assert status and
   `x-nek-trace-id`, and issue subsequent public detail/list reads to verify
   committed facts. Fresh service/store reconstruction is covered by the
   existing Workspace integration tests and Task 1 rather than duplicated in
   the HTTP harness.
6. After uninstall, install the same Agent again through HTTP and assert a new
   Installation ID while the terminal row remains inspectable.
7. Add `TestAcceptanceHTTPFailureBoundaries` covering public unauthenticated,
   non-owner, malformed Workspace/Installation input, wrong Workspace/ID,
   disabled Installation, missing internal auth, unknown capability, and
   Catalog-disabled resolution. Assert exact status/code, Trace header, no
   secret or dependency detail, and no state mutation.
8. Add a canceled-context service check in the acceptance harness for a
   dependency error where a real HTTP dependency failure cannot be injected
   deterministically. Assert the error is not an empty or successful result.
9. Keep all Agent Card endpoints as fixture URLs only; no test may issue an
   HTTP request to an Agent endpoint.
10. Run focused acceptance tests and `gofmt`.

## Task 3: Record SDD Evidence and Handoff

**Files:**

- Modify: `specs/009-workspace-acceptance/tasks.md`
- Modify: `specs/009-workspace-acceptance/quickstart.md`
- Modify: `docs/handoffs/CURRENT.md`

1. Mark completed implementation and verification tasks with exact command
   output summaries; leave any unavailable PostgreSQL evidence explicitly
   marked not run.
2. Record the acceptance test names, active route versions, race counts, and
   fallback delta.
3. Update the handoff to state that Workspace trust/durability/concurrency
   acceptance is complete and that Invocation Dispatch/A2A Router remain the
   next unfinished scope.

## Task 4: Verification and Review Gates

**Files:**

- Modify: `specs/009-workspace-acceptance/tasks.md`

1. Run focused unit/contract/workspace/Gateway checks.
2. Run the schema-resetting PostgreSQL integration packages serially with a
   disposable `_test` database when available.
3. Run `go test -count=1 ./...`, `go test -race -count=1 ./...`, `go vet ./...`,
   `go build ./...`, `go mod tidy -diff`, `git diff --check`, and Compose
   configuration validation.
4. Re-read the Spec, Plan, Tasks, active contracts, and AGENTS.md. Confirm
   every FR/SC maps to a test or verification command and no task introduced
   an unapproved product behavior.
5. Obtain an independent review. Any finding must be added to `tasks.md`,
   fixed within the approved scope, and reviewed again.
6. Run a final converge check, verify local Git identity, commit with a focused
   message, and report the branch/commit and fallback delta.

## Execution Order

Task 1 and Task 2 have disjoint write scopes and may be implemented in parallel
after the SDD gate. Task 3 follows the test results. Task 4 is sequential after
all code and evidence changes; PostgreSQL packages inside Task 4 are serial.

## Expected Verification Commands

```sh
gofmt -w apps/control-plane/internal/workspace/integration/workspace_test.go \
  apps/control-plane/internal/workspace/integration/acceptance_http_test.go
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./...
go mod tidy -diff
git diff --check
docker compose --file deploy/compose.yaml config --quiet
```
