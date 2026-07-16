# Current Handoff: Spec 016 Non-Streaming A2A Dispatch Baseline

**Updated**: 2026-07-16 (Asia/Hong_Kong)

**State**: Workspace closure is complete. Invocation runtime contracts,
A2A Router Foundation, Invocation Ledger, and Runtime B Direct A2A Sample have
been merged into codex/016-nonstream-a2a-dispatch as the local baseline for
the next Invoke slice. Control Plane Dispatch remains a separate local branch
and is not required for the Router-owned non-streaming transport implementation
unless Spec 016 discovers a contract dependency.

## Repository State

- Upstream repository: https://github.com/NeKiro-project/NeKiro.git
- Fork remote: https://github.com/XnLemon/NeKiro.git
- Branch: codex/016-nonstream-a2a-dispatch
- Base lineage: codex/013-router-foundation plus merged
  codex/011-invocation-runtime-contracts,
  codex/014-invocation-ledger, and codex/015-runtime-b-agent
- Required local Git identity: Nene7ko_ <1604009816@qq.com>
- Frontend remains paused.
- Active feature: specs/016-nonstream-a2a-dispatch

Resolve the repository root on the current machine; do not assume a previous
Windows path exists.

## Spec 016 Baseline Progress

Spec 016 is now the active local feature target for Router-owned non-streaming
exact A2A dispatch and transient result delivery:

- Available baseline code: apps/a2a-router foundation, strict Router config
  and auth, Control Plane resolution client, Router dispatch placeholder,
  Router-owned Ledger package/API, and agents/runtime-b direct A2A callee.
- Available contracts: Invocation runtime semantic rules, Router Internal v3,
  Router Agent v1, result stream event v2, invocation event v0.3, and platform
  error v4.
- Current Spec 016 progress: T001-T016 are complete for the non-stream slice.
  The Router now wires an
  explicit non-streaming A2A transport into dispatch, maps a validated
  `stream=false` request to one A2A `message/send`, appends metadata-only
  Ledger lifecycle facts for accepted non-streaming dispatch, and returns the
  transient Invocation Result v1 payload only after the terminal success fact
  is committed.
- Transport failure mapping is now explicit: target/profile contract errors
  are `A2A_PROTOCOL_ERROR`, unsupported auth is `AGENT_AUTH_UNSUPPORTED`, HTTP
  and network failure is `AGENT_UNAVAILABLE`, malformed A2A result is
  `A2A_PROTOCOL_ERROR`, JSON-RPC `*a2a.Error` is `AGENT_EXECUTION_FAILED`, and
  deadline is `TIMEOUT`; canceled Agent tasks map to `CANCELED`/HTTP 409. The
  API consumes only the generic
  `PlatformErrorCode()` capability and maps these to the active 502/503/504
  response statuses (and 409 for canceled).
- Production assembly now requires `NEKIRO_DATABASE_URL`, opens the
  Router-owned Ledger store, checks schema readiness, and injects the Ledger
  appender. It fails closed if the schema is absent; it does not no-op or
  auto-migrate. Required Router limits are
  `NEKIRO_ROUTER_AGENT_RESPONSE_LIMIT_BYTES` and
  `NEKIRO_ROUTER_A2A_EVENT_LIMIT_BYTES`; non-stream input/output uses the
  configured/Card minimum and response overflow maps to
  `AGENT_RESPONSE_TOO_LARGE`.
- Pending follow-ups: T017 streaming A2A/SSE event limits and T018 exhaustive
  active A2A negative corpus tests. T016 now provides the deployment-owned
  `migrate up` command and Compose ordering before Router `serve`.
- Open risk: Spec 014 real PostgreSQL integration remains environment-pending;
  use non-integration Ledger evidence unless a PostgreSQL test database becomes
  available.

Final post-review verification after the target-validation lifecycle fix and
local Router runbook update passed:

```text
go test -count=1 ./...
go vet ./...
git diff --check
wsl.exe -d Ubuntu-26.04 -- bash -lc 'cd /mnt/e/NeKiro && go test -race -count=1 ./apps/a2a-router/... ./agents/runtime-b'
docker compose --file deploy/compose.yaml config --quiet
```

The Ledger target-validation failure case records exactly
`created -> routing -> failed`, preserves `AGENT_AUTH_UNSUPPORTED`, and does
not append `started` or call the Agent. Exact target construction now receives
the dispatch capability instead of selecting a Card skill by position.

## Recently Closed Gates

- Spec 011 Invocation Runtime Contracts: closed and merged into this branch.
- Spec 013 A2A Router Foundation: closed and used as this branch base.
- Spec 014 Invocation Ledger: merged as a checkpoint; real PostgreSQL
  integration/Review/Converge remain documented as pending in its own tasks.
- Spec 015 Runtime B Direct A2A Sample: implemented, WSL race verified,
  independently reviewed, converged, and merged into this branch.
- Fallback delta for the Spec 016 baseline merge: removed 0, retained 0,
  added 0, net 0. Added fallback evidence: none.

Spec 016 T009-T010 evidence:

- Focused failure matrix: `go test -count=1 ./apps/a2a-router/internal/api`
  and `go test -count=1 ./apps/a2a-router/internal/transport/a2a` pass.
- Transport tests cover unsupported profile/auth, malformed message/result and
  JSON-RPC envelope/media, A2A JSON-RPC Agent failure, HTTP failure, response
  overflow, and deadline mapping.
- Implementation-only fallback/write-scope scan found no Control Plane import,
  direct database access, result persistence, retry, cache, alternate endpoint,
  compatibility branch, default credential, or fallback endpoint. The only
  broader scan match is the pre-existing test name
  `TestDispatchMapsResolutionDependencyWithoutRetry`.
- Fallback delta: removed 0, retained 0, added 0, net 0; added fallback
  evidence: none.

Independent Review and Converge completed: no P0 findings. In-scope findings
(raw envelope validation, CANCELED 409, production Ledger injection, strict
DB/limit configuration, bounded response, and Card/config input/output bounds)
were resolved. T016-T018 remain explicit rather than silently claimed.

Spec 016 T011 verification passed:

```text
go test -count=1 ./apps/a2a-router/... ./agents/runtime-b/...
go test -count=1 ./...
go vet ./...
git diff --check
wsl.exe -d Ubuntu-26.04 -- bash -lc 'cd /mnt/e/NeKiro && go test -race -count=1 ./apps/a2a-router/... ./agents/runtime-b'
```

The focused Router/Runtime B tests, full repository tests, vet, diff check,
and WSL race run all passed. The WSL race command was rerun after the final
Ledger wiring, strict config, Card-bound preflight, response-limit, and A2A
envelope changes.
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
- Workspace parent Issue #2 is closed. Spec 010 T001 contract review may start;
  T002-T010 remain blocked by their recorded dependency graph.
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

Implemented Control Plane runtime currently covers the Catalog and Workspace
boundaries:

```text
Register -> Publish -> Discover -> Disable
```

Workspace adds `Create/Read -> Install -> Inspect -> Disable/Enable ->
Uninstall -> Reinstall` and internal exact resolution over the controlled
Catalog port.

Workspace persistence, owner policy, Installation selection/lifecycle, and
internal exact resolution are implemented. Invocation Dispatch, A2A Router,
Ledger, SDK/runtime behavior, live sample Agents, Frontend, and the complete
E2E loop remain unimplemented.

Do not infer Invocation/Router runtime availability from active schemas or
OpenAPI paths. Continue future implementation with a fresh Spec/worktree for
Invocation Dispatch and the A2A Router after Issue #9 is accepted.

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

