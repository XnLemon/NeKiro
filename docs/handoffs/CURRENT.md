# Current Handoff: Invocation Ledger Checkpoint

**Updated**: 2026-07-16 (Asia/Hong_Kong)

**State**: Workspace closure is complete. Invocation runtime contracts,
Control Plane Dispatch, and Router Foundation have local implementation
branches. Invocation Ledger is implemented on `codex/014-invocation-ledger`
and non-integration verified, but its real PostgreSQL integration run,
independent Review, and Converge remain open. Runtime B Direct A2A Sample is
implemented and locally verified; its independent Review and Converge remain
open.

## Spec 012 Control Plane Invocation Dispatch

Spec 012 is now integrated into this branch from its independently reviewed
implementation. Gateway exposes `POST /v4/workspaces/{workspaceId}/invocations`,
validates the public request, and authorizes the exact enabled installation
through the Workspace boundary before creating root correlation and sending
one authenticated Router Internal v3 request. JSON and bounded SSE responses
are forwarded live, including the approved Router trace header. Router
URL/token, public body/SSE limits, and deadline configuration are required with
no defaults.

Spec 012 verification and convergence evidence passed on its source branch and
was rerun against the current Router/Streaming branch: full tests, vet, and
WSL race checks pass. Deployment/Compose wiring for the new Gateway runtime
variables remains intentionally outside this child Spec and belongs to the
parent acceptance task. Fallback delta for this slice remains `removed 0,
retained 0, added 0, net 0`.

## Repository State

- Upstream repository: `https://github.com/NeKiro-project/NeKiro.git`
- Fork remote: `https://github.com/XnLemon/NeKiro.git`
- Branch: `codex/014-invocation-ledger`
- Base: `origin/main` at `99e52aa`
- Required local Git identity: `Nene7ko_ <1604009816@qq.com>`
- Frontend remains paused.

Resolve the repository root on the current machine; do not assume a previous
Windows path exists.

## Spec 014 Invocation Ledger Progress

Spec 014 owns the Router-side metadata-only `Record` boundary:

- Explicit Ledger migration/readiness, append-only events, transactional
  projection, nested parent lineage checks, restart-safe Workspace-scoped
  Invocation/Trace reads, and Router Internal v3 read adapter are implemented
  in `apps/a2a-router/internal/ledger` and `apps/a2a-router/internal/api`.
- PostgreSQL integration tests exist for lifecycle atomicity, strict readiness,
  nested lineage, restart/order/isolation, prohibited content columns,
  dependency failure, and concurrency.
- Non-integration verification passed: `go test ./apps/a2a-router/internal/ledger ./apps/a2a-router/internal/api`,
  `go test ./...`, `go vet ./...`, and `git diff --check`.
- Real integration verification is still pending: `NEKIRO_TEST_DATABASE_URL`
  is unset and Docker daemon access failed while attempting to start a
  disposable PostgreSQL 17 container.

Open Spec 014 gates:

- T012: run formatting/unit/integration/race/vet/full/fallback checks once a
  real PostgreSQL test database is available.
- T013: independent Review by a non-implementing agent.
- T014: Converge review findings and repeat Review.

## Spec 013 A2A Router Foundation Progress

Spec 013 now adds the first standalone Data Plane Router foundation on branch
`codex/013-router-foundation`:

- Created `specs/013-a2a-router-foundation/` with Spec, Plan, research,
  data model, contract guide, quickstart, checklist, and executable tasks.
- Added `apps/a2a-router` process assembly, strict no-default config,
  Router service bearer auth, readiness handler, Router Internal v3 dispatch
  validation, Control Plane Internal v2 resolution client, and correlated
  `ROUTE_NOT_FOUND` post-resolution placeholder.
- Preserves boundaries: no Control Plane internal imports, no Agent endpoint
  call, no Ledger path/write, no retry/cache/alternate source, and no shared
  contract mutation.
- Local verification passed: focused Router tests, `go test ./...`,
  `go vet ./...`, and `git diff --check`.

Independent Review-R1 found and converged three blockers: duplicate
`Authorization` header acceptance, reconstructed Control Plane resolution
errors, and a fabricated entropy-failure trace fallback. The fixes require
exactly one Authorization value, preserve exact Control Plane failure
status/body/trace through the Router, and fail closed when pre-correlation
trace entropy is unavailable. Focused Router tests, `go test ./...`,
`go vet ./...`, and `git diff --check` passed after convergence. Follow-up
Review returned PASS with no remaining P0-P2 blocker.

## Spec 015 Runtime B Direct A2A Sample Progress

Spec 015 owns the deterministic direct-library Runtime B callee for later
Router transport acceptance:

- `agents/runtime-b` implements strict fixture parsing, domain-separated
  deterministic IDs, process-local task snapshots, `message/send`,
  `message/stream`, `tasks/get`, `tasks/cancel`, and JSON-RPC/SSE server
  assembly.
- Tests cover JSON success/failure/invalid input, ordered streaming,
  cancellation, terminal task errors, history bounds, concurrent identity
  isolation, and platform context headers.
- Non-race verification passed: `gofmt -l agents/runtime-b` returned no files,
  `go test ./agents/runtime-b ./agents/runtime-b/cmd/runtime-b`,
  `go test ./...`, `go vet ./...`, and `git diff --check`.
- Windows race verification remains unavailable because Windows lacks a C
  compiler for cgo: `go test -race` requires cgo, and `CGO_ENABLED=1` fails
  with `gcc not found`; `where gcc`, `where clang`, and `where cl` also find
  no compiler on PATH.
- WSL race verification passed under Ubuntu-26.04 with `go version go1.26.0
  linux/amd64`, `CGO_ENABLED=1`, `CC=x86_64-linux-gnu-gcc`, and `/usr/bin/gcc`.
- Runtime code has no platform database, Control Plane, Router, Ledger, SDK,
  retry, cache, alternate route, compatibility fallback, or platform-core
  dependency.

Open Spec 015 gates:

- T010: rerun verification in a race-capable Go environment.
- T011: independent Review by a non-implementing agent.
- T012: Converge review findings and repeat Review.

Review attempt note: T011 was attempted in the 2026-07-16 session after local
verification passed, but the independent review agents did not return usable
PASS/FAIL results before interruption. Treat T011 as still open and rerun an
independent read-only Review from the branch head.

Review attempt note: T011 was attempted in the 2026-07-16 session after local
verification passed, but the independent review agents did not return usable
PASS/FAIL results before interruption. Treat T011 as still open and rerun an
independent read-only Review from branch head `cd65b7c` or later.

## Workspace Closure and Next Plan

- PR #18 run `29442651978` passes `workspace-integration`, `go-quality`,
  `frontend`, and `compose-config`; commit `33eb1ae` fixes the PostgreSQL
  default varchar index opclass and the PR is merged.
- Spec 009 T025-T027 are complete. The fresh independent closure review found
  no P0-P2 issue; OCR produced zero comments and all local static/race checks
  passed against the merged code.
- `specs/010-invocation-routing-ledger/` is the next planning source for the
  backend Invocation Dispatch, A2A Router, Ledger, thin SDK, cross-Runtime
  sample Agents, and acceptance parent.
- GitHub parent [#19](https://github.com/NeKiro-project/NeKiro/issues/19) owns
  eleven native Sub-issues [#20](https://github.com/NeKiro-project/NeKiro/issues/20)
  through [#30](https://github.com/NeKiro-project/NeKiro/issues/30), with native
  dependency relations matching Spec 010 `tasks.md`.
- Workspace parent Issue #2 and Spec 010 T001 are complete. Spec 012 delivers
  T002 Control Plane Invocation Dispatch; the Router foundation, Ledger,
  Runtime B sample, and T006/T007 transport slices are also present on this
  branch. T008 Invocation/Trace reads, T009 SDK/nested Router calls, T010 the
  second Runtime caller, and T011 integrated acceptance remain open.
- Frontend remains paused and is not included in Spec 010.

## Issue #9 Acceptance Closure

Issue #9 adds evidence, not a new runtime feature. The acceptance harness uses
the real Catalog and Workspace PostgreSQL stores/services and composes both
public and internal Gateway handlers under `httptest`.

- Public capability discovery, Workspace creation, exact Installation pin,
  inspection, lifecycle, terminal uninstall, and new-identity reinstall pass
  in one workflow; Review-R1/R2 findings were remediated and Review-R3 passed
  with no P0-P2 findings.
- The composed acceptance flow now publishes `1.0.0` and `1.1.0` and proves a
  `^1.0.0` installation pins the highest matching `1.1.0` version. Store
  reconstruction compares durable enabled, disabled, and uninstalled rows.
- The internal HTTP failure matrix proves `AGENT_NOT_INSTALLED` after
  uninstall and `CAPABILITY_NOT_ALLOWED` when the accepted permission snapshot
  omits the capability requirement, with request correlation preserved.
- Internal resolution now preserves valid correlation for non-correlation
  validation failures, and Workspace readiness rejects incomplete Installation
  columns or constraints before serving traffic.
- Internal resolution uses a separate authenticated principal and preserves
  the request correlation identifiers. The fixture Agent endpoint records zero
  requests.
- Restart, history, Catalog disable, dependency, duplicate-install, lifecycle,
  and uninstall/reinstall concurrency evidence remains covered by the existing
  Workspace integration package; the lifecycle race now validates every
  successful response and final legal history.
- Active Northbound v3 and Control Plane Internal v2 contracts are reused.
  No route, migration, fallback, retry, cache, alternate source, Router, or
  Ledger behavior is added.

The Issue #9 verification and later PR #18 readiness correction passed against
a disposable PostgreSQL 17 `_test` database:

```sh
go test -tags=integration -count=1 ./apps/control-plane/internal/catalog/postgres
go test -tags=integration -count=1 ./apps/control-plane/internal/workspace/postgres
go test -tags=integration -count=1 ./apps/control-plane/internal/workspace/integration
go test -tags=integration -count=1 ./tests/integration/catalog
```

Static verification also passed: `go test ./...`, race, `go vet`, `go build`,
`go mod tidy -diff`, and `git diff --check`. PR #18 then passed its project
closure PostgreSQL and static CI jobs before merge. Missing or unsafe database
configuration is not a pass. The non-required Codecov patch check remained red
and is not presented as test success.

Fallback delta: removed `0`, retained `1`, added `0`, net `0`. Added fallback
evidence: none. The retained behavior is the active contract's legitimate
empty Installation list.

## Issue #5 Delivery

The active #5 Spec/Plan/Tasks define and verify the owner installation slice:

- `POST /v3/workspaces/{workspaceId}/installations` requires trusted owner
  authentication, a valid SemVer constraint, and a required permission array;
  missing/null is invalid while explicit `[]` is preserved.
- Catalog remains the sole published-version selector. Installation stores the
  exact selected version, canonical permission subset, submitted constraint,
  enabled status, server ID, and committed database timestamps through the
  controlled Catalog reader and Workspace-owned transaction.
- Workspace row locking plus the partial unique index leaves one current
  Installation under concurrent requests. New matching publications do not
  mutate an existing pin.
- Integration evidence covers stable/pre-release/build selection, empty
  permissions, restart reconstruction, publication immutability, dependency
  failure, and the 100-request race. No Agent endpoint is invoked or probed.
- Fallback delta: removed `0`, retained `1`, added `0`, net `0`; the retained
  behavior is the explicitly approved empty permission set.

## Issue #4 Delivery

The active #4 Spec/Plan/Tasks define and verify the first end-to-end Workspace
root without changing Installation semantics:

- `POST /v3/workspaces` accepts only an explicit safe `workspaceId`; the
  trusted Gateway caller becomes the immutable owner and the database-returned
  timestamps are returned to keep create/read facts identical at PostgreSQL
  microsecond precision.
- `GET /v3/workspaces/{workspaceId}` returns the exact durable four-field fact
  only to its owner. Unknown, non-owner, invalid, unauthenticated, conflict,
  and persistence failure outcomes remain distinct.
- Workspace readiness verifies the schema version, exact Workspace columns,
  types, lengths, non-nullability, `COLLATE "C"`, timestamp precision, primary
  key, identifier checks, timestamp check, Installation tables, and required
  indexes. Serving never auto-migrates.
- PostgreSQL integration evidence covers restart-style store reconstruction,
  duplicate preservation, missing/stale/incomplete/nullable/non-Collation/
  reduced-precision schema, and canceled dependency behavior.
- No fallback was added: removed `0`, retained `0`, added `0`, net `0`.

## Delivered Contract Gate

Spec 003 freezes and implements the Minimal Workspace and Agent Installation
boundary:

- `spec.md` defines Workspace create/read, Installation
  create/read/list/lifecycle, and internal exact-resolution user scenarios,
  requirements, failures, measurable outcomes, and non-goals.
- Clarifications fix immutable creator ownership, SemVer/pre-release selection,
  build-metadata tie-break, accepted permission subsets, one-current uniqueness,
  uninstall history, and Catalog-disable behavior.
- `plan.md`, `research.md`, `data-model.md`, the API contract guide, and
  `quickstart.md` define module ownership, controlled Catalog access,
  PostgreSQL transactions, extension boundaries, validation, and operations.
- `tasks.md` records the dependency-ordered implementation and the completed
  contract-gate remediation tasks. Workspace root and install/pin, inspection,
  lifecycle, and exact resolution now share one Workspace-owned persistence
  boundary. Invocation runtime remains outside this feature.
- ADR 0005 records the infrastructure-independent Workspace and Installation
  boundary.

## Active Contract Changes

- New Workspace v1 Schema contains exactly `workspaceId`, `ownerId`,
  `createdAt`, and `updatedAt`.
- Installation v2 preserves exact pin, accepted permission snapshot, lifecycle,
  and terminal `uninstalledAt` coherence, with canonical permission and
  timestamp and constraint-compatible pin semantic validation.
- Northbound v3 now declares seven Workspace/Installation operations with
  Bearer security, Trace headers, success DTOs, exact per-operation errors, and
  bounded opaque-cursor Installation inspection.
- Control Plane Internal v2 now requires a separately trusted service Bearer
  identity, explicit pre-correlation failures, and distinguishes exact
  resolution states.
- Platform Error v3 adds `INSTALLATION_DISABLED` for Workspace/internal
  resolution with fixed public message while Platform Error v2 remains the
  Catalog/Invocation contract.
- Go DTOs, validators, and mapping tests consume these language-neutral facts.
- Northbound v2 and the other historical contract artifacts remain unchanged;
  no compatibility fallback or dual route is introduced.
- Northbound invocation POST and Ledger reads explicitly require Bearer
  security; Ledger reads expose authentication failures in the active mapping.
- Control Plane configuration requires a separate internal auth mode and
  principal digest set; Northbound credentials are not implicitly trusted for
  internal resolution.

## Key Policies

- Workspace is a logical authorization/audit root, not a Kubernetes Namespace.
- The authenticated creator is the immutable owner. Northbound owner identity
  never comes from request data or a public header.
- Installation pins the highest currently published matching SemVer. A
  constraint branch must explicitly include a pre-release comparator for
  pre-releases to participate. Equal precedence uses the bytewise-greatest
  exact version string.
- Accepted permissions are an exact case-sensitive subset, may be empty, and
  are stored in canonical bytewise order.
- Lifecycle is `enabled <-> disabled -> uninstalled`. Same-state and terminal
  repetitions are conflicts; uninstall preserves history and is not idempotent.
- Catalog disable never rewrites an Installation. Exact resolution returns
  `AGENT_DISABLED`.
- The only retained fallback-classified behavior is the genuine empty
  Installation list for an existing authorized Workspace.

## Verification

The contract, unit, race, static, and real PostgreSQL gates passed:

```powershell
go test -count=1 ./contracts
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go mod tidy -diff
git diff --check
go test -tags=integration -count=1 ./apps/control-plane/internal/catalog/postgres
go test -tags=integration -count=1 ./apps/control-plane/internal/workspace/postgres
go test -tags=integration -count=1 ./apps/control-plane/internal/workspace/integration
docker compose --file deploy/compose.yaml config --quiet
```

Workspace domain, Gateway, migration, and contract tests are present. The real
PostgreSQL run passed migrations, exact resolution, lifecycle history,
reinstall, and the 100-request current-install uniqueness race against a
dedicated `_test` database.

Spec Kit analysis result for the completed gate:

- Requirements: 30 functional plus 7 measurable outcomes
- Tasks: 67 total; all implementation and remediation tasks are recorded complete
- Requirement coverage: 100%
- Ambiguity, duplication, constitution conflict, and Critical findings: 0

Fallback delta: removed `0`, retained `2`, added `0`, net `0`.
Added fallback evidence: none. The retained behaviors are the approved genuine
empty Installation list and the unchanged Spec 002 Discovery page-size policy,
not degraded dependency handling.

Independent closure Review used `open-code-review` v1.7.9. The #4 change had
three remediation passes for Workspace readiness; the #5 change was reviewed
independently across 4 files and produced zero comments.

## Runtime Boundary

Implemented Control Plane runtime currently covers Catalog, Workspace, and the
public Invocation Dispatch boundary:

```text
Register -> Publish -> Discover -> Disable
```

Workspace adds `Create/Read -> Install -> Inspect -> Disable/Enable ->
Uninstall -> Reinstall` and internal exact resolution over the controlled
Catalog port. The v4 Gateway invoke route authorizes an exact installation and
forwards live JSON/SSE results through the Router.

Workspace persistence, owner policy, Installation selection/lifecycle, and
internal exact resolution are implemented. Invocation Dispatch, A2A Router,
Ledger, SDK/runtime behavior, live sample Agents, Frontend, and the complete
E2E loop remain unimplemented.

Do not infer metadata reads, SDK/nested calls, or the complete acceptance
demonstration from active schemas or OpenAPI paths; those remain future work
under fresh child Specs.

## Recovery

```powershell
git clone https://github.com/XnLemon/NeKiro.git
Set-Location NeKiro
git remote add upstream https://github.com/NeKiro-project/NeKiro.git
git fetch origin --prune
git fetch upstream --prune
git switch --track origin/codex/010-invocation-routing-ledger
git config --local user.name Nene7ko_
git config --local user.email 1604009816@qq.com
git status --short --branch
git log -8 --oneline
```

Before modifying runtime code, read in full:

- `AGENTS.md`
- `.specify/memory/constitution.md`
- this file
- every artifact under `specs/009-workspace-acceptance/`
- every artifact under `specs/010-invocation-routing-ledger/`
- the active contracts under `contracts/openapi/`, `contracts/schemas/`,
  `contracts/invocation/`, and `contracts/a2a-profile/`
- `docs/decisions/0005-minimal-workspace-installation-boundary.md`
- `docs/contracts/compatibility.md`

Do not modify `pnpm-lock.yaml`, relax `minimumReleaseAge`, add a fallback data
source, or begin Spec 010 runtime implementation before T001/#20 freezes and
merges the shared contract and failure policy.
