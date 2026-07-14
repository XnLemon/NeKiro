# Current Handoff: Issue 004 Workspace Create and Read

**Updated**: 2026-07-15 (Asia/Hong_Kong)

**State**: Issue #4 Workspace create/read runtime and its independent delivery
gate are complete on the feature branch. Issue #3 remains the dependent base
PR; Issue #5 installation delivery is the next slice. Invocation Dispatch,
Router, Ledger, SDK, Sample Agents, and the complete E2E loop remain future
scope.

## Repository State

- Upstream repository: `https://github.com/NeKiro-project/NeKiro.git`
- Fork remote: `https://github.com/XnLemon/NeKiro.git`
- Branch: `codex/004-workspace-create-read`
- Base: `upstream/main` at `f4caf4d`
- Required local Git identity: `Nene7ko_ <1604009816@qq.com>`
- Frontend remains paused.

The handoff commit hash is intentionally not recorded because this file is part
of that commit. Resolve the repository root on the current machine; do not
assume a previous Windows path exists.

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

Independent closure Review used `open-code-review` v1.7.9. Three review passes
on the #4 change found and fixed Workspace schema readiness gaps for column
metadata/constraints, identifier collation, and timestamp precision. The final
review produced zero comments.

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
OpenAPI paths. Continue future implementation by creating the Issue #5 Spec
and a fresh worktree after Issue #4 is accepted.

## Recovery

```powershell
git clone https://github.com/XnLemon/NeKiro.git
Set-Location NeKiro
git remote add upstream https://github.com/NeKiro-project/NeKiro.git
git fetch origin --prune
git fetch upstream --prune
git switch --track origin/codex/004-workspace-create-read
git config --local user.name Nene7ko_
git config --local user.email 1604009816@qq.com
git status --short --branch
git log -8 --oneline
```

Before modifying runtime code, read in full:

- `AGENTS.md`
- `.specify/memory/constitution.md`
- this file
- every artifact under `specs/004-workspace-create-read/`
- every artifact under `specs/003-workspace-installation-contracts/`
- `docs/decisions/0005-minimal-workspace-installation-boundary.md`
- `docs/contracts/compatibility.md`

Do not modify `pnpm-lock.yaml`, relax `minimumReleaseAge`, add a fallback data
source, or begin Frontend/Router/Runtime work in the Workspace implementation
slice.
