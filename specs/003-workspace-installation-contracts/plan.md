# Implementation Plan: Minimal Workspace and Installation

**Branch**: `codex/003-workspace-installation-contracts` | **Date**: 2026-07-14 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from
`/specs/003-workspace-installation-contracts/spec.md`

**Note**: This contract-gate branch freezes the approved behavior and active
language-neutral contracts before any Workspace runtime implementation begins.
The tasks describe that later implementation and its post-implementation tests.

## Summary

Close the Phase 1 `Discover -> Install` gap with a minimal Control Plane
Workspace authorization root and durable Agent Installation. A trusted creator
owns one caller-selected Workspace ID. The owner installs the highest currently
published Agent version satisfying an explicit SemVer constraint, accepts an
exact permission subset, and receives an immutable exact-version pin. Workspace
owns lifecycle and history in PostgreSQL, consumes Catalog facts through a
narrow controlled port, and serves an internal exact-resolution boundary for
the future Router. This gate completes Workspace/Installation schemas,
 Northbound v3, Control Plane Internal v2, Platform Error v3 for
Workspace/Installation, Go contract mappings, and conformance tests without
adding runtime handlers or persistence.

## Technical Context

**Language/Version**: Go 1.26 for the later Control Plane implementation and
contract mappings; OpenAPI 3.1 and JSON Schema 2020-12 remain contract facts

**Primary Dependencies**: Go standard library `net/http`; existing
`github.com/Masterminds/semver/v3 v3.5.0`; existing
`github.com/jackc/pgx/v5 v5.10.0`; existing `github.com/jackc/tern/v2 v2.4.1`;
existing contract validator packages

**Storage**: PostgreSQL 17, Workspace-owned schema with Workspace and
Installation tables; exact accepted permissions stored as a validated ordered
snapshot; no Catalog table access from Workspace code

**Testing**: Go `testing` and `httptest`; contract mapping tests; unit tests
after implementation; real PostgreSQL integration and HTTP acceptance under an
explicit `integration` build tag

**Target Platform**: Linux container for deployment and CI; Windows/Linux host
execution for development

**Project Type**: Control Plane backend slice in the Phase 1 monorepo; no
Frontend, Router, Agent Runtime, or deployment implementation

**Performance Goals**: Deterministic version selection over the first Catalog
scale target of 10,000 published versions; exact resolution and owner reads use
indexed point queries; a 100-request concurrent same-Agent install race commits
one current Installation

**Constraints**: Gateway-only Northbound access; owner-only policy behind a
narrow replacement boundary; active contracts only; exact-version pins; no
automatic upgrades; required dependencies fail explicitly; no cache, queue,
retry, alternate source, Kubernetes binding, Membership/RBAC, Policy Hook, or
compatibility fallback

**Scale/Scope**: Seven Northbound Workspace/Installation operations, one
internal exact-resolution operation, two Workspace-owned tables, one Control
Plane process, five independently testable user stories, and three stable
post-foundation implementation slices

## Constitution Check

*GATE: Passed before Phase 0 research; re-checked and passed after Phase 1
design.*

- **Phase 1 loop - PASS**: The feature closes `Discover -> Install` and creates
  the authorization fact required before `Invoke`.
- **Ownership - PASS**: Workspace alone owns Workspace and Installation rows.
  Gateway adapts Northbound HTTP. Catalog remains the Agent Card fact owner and
  is reached only through a controlled in-process port. Router remains a future
  internal API consumer and never reads Control Plane tables.
- **Runtime independence - PASS**: Installation refers only to language-neutral
  Agent Card identity, versions, permissions, and capabilities. It does not
  start, probe, deploy, or import any Agent Runtime.
- **Contracts - PASS**: Workspace v1, Installation v2, Northbound v3, Control
  Plane Internal v2, and Platform Error v3 for Workspace/Installation are
  completed before runtime code. Existing historical contracts remain
  unchanged; Catalog and Invocation continue to consume Platform Error v2.
  The pre-runtime declaration and migration impact are recorded in
  compatibility documentation.
- **Invocation lineage - PASS**: Workspace creates no Invocation, Task, result,
  or Ledger record. Internal resolution only validates and preserves
  correlation already created by a later dispatch flow.
- **Failure safety - PASS**: Invalid, unauthenticated, forbidden, not-found,
  conflict, disabled, capability-denied, and dependency outcomes remain
  distinct. No owner, constraint, Card, permission, persistence, or empty-list
  fallback exists; contract fields contain no secrets.
- **SDD traceability - PASS**: FR-001 through FR-030 map to design artifacts and
  tasks. Implementation tasks precede their mapped unit, contract, integration,
  race, and restart tests. Independent Review and Converge remain explicit.
- **Cross-runtime proof - PASS**: Both future sample Agents use the same
  Installation and resolution contracts. Live invocation remains assigned to a
  later Router/Dispatch Spec.

## Design Decisions

### Workspace Root and Authorization

- A Workspace is a logical authorization and audit root, not an infrastructure
  namespace. Its complete public fact is `workspaceId`, immutable `ownerId`,
  `createdAt`, and `updatedAt`.
- `POST /v3/workspaces` accepts only `workspaceId`. Gateway authentication
  supplies the trusted owner; owner identity is never accepted from a header or
  request body field.
- Workspace operations call a narrow `Authorizer` policy port. Phase 1 uses an
  exact owner-ID equality implementation. Membership and RBAC may replace that
  policy later without altering Workspace or Installation rows.
- Duplicate creation and repeated lifecycle mutation are conflicts. No
  undocumented idempotency is introduced.

### Version Selection and Catalog Port

- Workspace depends on a `CatalogReader` port that exposes published-version
  selection and exact-version reads as Catalog-owned behavior. It never imports
  the PostgreSQL adapter or queries `catalog` tables.
- Strict Agent Card versions are evaluated against the submitted constraint
  with the pinned SemVer library. Pre-releases are excluded unless the matching
  constraint branch explicitly includes a pre-release comparator.
- Candidate ordering is SemVer precedence descending. Equal precedence caused
  by build metadata uses the bytewise-greatest original exact version string as
  the deterministic final key. The exact Card version is persisted unchanged.
- The successful Catalog selection response is the version-choice
  linearization point. The Workspace transaction then validates the permission
  subset and persists the pin. A Catalog disable that commits later does not
  roll back Installation; exact resolution observes it as `AGENT_DISABLED`.
- A Catalog dependency failure is explicit. No stale Card, historical Card,
  wildcard constraint, exact-version substitution, or in-memory candidate list
  is used.

### Permission Snapshot and Capability Authorization

- `acceptedPermissions` is a set over the exact selected Card's declared
  permission IDs. Input IDs remain case-sensitive and are never trimmed.
- The empty set is valid. Unknown or duplicate IDs reject the install before
  persistence. Stored and returned IDs use ascending bytewise order for one
  canonical representation.
- A capability resolves only when it exists on the exact Card and every one of
  its `requiredPermissions` is present in the accepted snapshot. Missing
  capability and insufficient accepted permissions share the intentional
  `CAPABILITY_NOT_ALLOWED` public policy and reveal no Card details.

### Installation Lifecycle and History

- New Installations begin `enabled`. The only transitions are
  `enabled -> disabled`, `disabled -> enabled`, and
  `disabled -> uninstalled`.
- Same-state updates, direct enabled-to-uninstalled requests, and every mutation
  of uninstalled history return conflict. Uninstall is not an idempotent delete.
- An uninstalled record remains readable with `uninstalledAt`; reinstall creates
  a new ID and row. A partial unique index on `(workspace_id, agent_id)` for
  non-uninstalled rows enforces one current Installation under concurrency.
- Lifecycle mutation locks one row. Transition validation, timestamp changes,
  and uniqueness-slot release commit atomically.
- List returns bounded pages of current and historical rows ordered by
  `installed_at` then `installation_id`, both ascending. Every request supplies
  an explicit page size from 1-100, and an opaque cursor binds Workspace, page
  size, and the last ordering tuple. A real no-record result is an explicit
  empty items array after Workspace existence and owner authorization succeed.

### Exact Internal Resolution

- Control Plane Internal v2 remains the Control Plane-owned destination. The
  operation requires a trusted internal Bearer principal and the existing
  invocation, root Task, Trace, Workspace, Agent, exact version, and capability.
- The process receives a separately injected internal `Authenticator`. Local
  development requires explicit internal auth mode and principal-digest
  configuration; Northbound principals are not implicitly internal principals,
  and no token or principal default exists.
- Resolution first finds the current exact Installation, verifies requested and
  pinned versions match, then checks Installation state, current exact Catalog
  state, capability existence, and accepted permissions in that order.
- Success returns the exact active Agent Card 0.2 plus the minimal enabled
  Installation authorization facts already required by the Router.
- Errors after strict correlation validation preserve request correlation and distinguish
  `AGENT_NOT_INSTALLED`, `INSTALLATION_DISABLED`, `AGENT_DISABLED`,
  `CAPABILITY_NOT_ALLOWED`, and `DEPENDENCY_ERROR`. No failure returns a Card.
  Malformed/missing correlation and pre-authentication failures use the
  pre-correlation shape with only fixed `code`/`message` and a generated safe
  `traceId`.

### Contracts and Compatibility

- Add Workspace v1 Schema and Installation v2 with terminal timestamp and
  canonical-permission semantic rules. Preserve Northbound v2 unchanged and
  complete all active routes and DTOs in Northbound v3. Complete internal authentication, explicit
  pre-correlation errors, and exact error sets in Control Plane Internal v2.
- Add `INSTALLATION_DISABLED` and its fixed message to Platform Error v3 for
  Workspace/Installation and internal resolution. Preserve Platform Error v2
  for existing Catalog/Invocation consumers. No deployed Workspace or Router
  runtime consumes the earlier underspecified documents, so the gate records a
  pre-runtime declaration rather than a dual-version window.
- Go DTOs and validators consume the language-neutral sources and receive
  mapping tests. They do not become an alternate contract fact.

### Persistence and Transaction Boundaries

- PostgreSQL schema `workspace` is the only durable Workspace data owner.
  Catalog and Workspace may share one database instance but not tables or write
  transactions.
- Workspace creation is one insert transaction. Installation first performs
  authentication, request validation, Workspace existence, owner authorization,
  and current-Installation conflict preflight. Only then does Catalog selection
  occur. A separate Workspace-owned transaction subsequently locks and
  rechecks Workspace ownership/current uniqueness, validates the selected Card
  permission subset, and inserts the Installation. Catalog calls never occur
  while a Workspace transaction is held open.
- Lifecycle operations lock the target Installation row and update one legal
  transition. Reads use indexed statements and never synthesize missing rows.
- Expected uniqueness and transition races map to domain conflict. All other
  database errors remain dependency failures.
- Migrations remain explicit, embedded, ordered, and forward-only. Serving
  verifies schema readiness and never migrates automatically.

### Test Strategy

- Contract-gate tests validate every new Schema/OpenAPI/Go mapping, exact route
  security and response set, bounded Installation cursor pagination,
  conditional uninstall/timestamp semantics, pre-correlation outcomes, and
  the versioned fixed Platform Error policies.
- After implementation, unit tests cover policy, SemVer/pre-release/tie-break,
  permission sets, transition tables, adapters, and error precedence without
  PostgreSQL.
- Integration-tagged tests use a dedicated database ending in `_test` and cover
  migrations, persistence restart, controlled Catalog port behavior, HTTP,
  internal resolution, injected dependency failures, and 100-way races.
- Tests follow implementation, map to Spec scenarios, and do not invent new
  fallback policy. A fresh independent Review precedes Converge.

### Fallback Inventory

| Candidate | Classification | Policy |
| --- | --- | --- |
| Empty Installation list after authorized existing Workspace lookup | Keep | Genuine product result required by FR-018 |
| Catalog Discovery omitted-limit default 25 carried into Northbound v3 | Keep | Existing Spec 002 product policy; Workspace work does not change it |
| Owner inferred from request/header/default principal | Remove | Owner comes only from trusted authentication context |
| Invalid/omitted SemVer constraint replaced by wildcard/exact version | Remove | Invalid input is `VALIDATION_ERROR` |
| Catalog failure replaced by stale/historical/in-memory Card | Remove | Dependency failure is `DEPENDENCY_ERROR` |
| Same-state lifecycle mutation treated as success | Remove | FR-014 requires `CONFLICT` |
| Catalog disable rewrites or auto-upgrades Installation | Remove | FR-022 preserves pin and returns `AGENT_DISABLED` |

Fallback delta for this gate: removed `0`, retained `2`, added `0`, net `0`.
Added fallback evidence: none. The retained empty list is approved product
semantics; the Discovery default is an unchanged Spec 002 policy.

## Project Structure

### Documentation (this feature)

```text
specs/003-workspace-installation-contracts/
|-- spec.md
|-- plan.md
|-- research.md
|-- data-model.md
|-- quickstart.md
|-- contracts/
|   `-- workspace-installation-api.md
|-- checklists/
|   `-- requirements.md
`-- tasks.md
```

### Source Code (repository root)

```text
apps/control-plane/
|-- internal/catalog/
|   |-- resolution.go
|   `-- postgres/resolution.go
|-- internal/gateway/
|   |-- workspace_root_handler.go
|   |-- workspace_install_handler.go
|   |-- workspace_inspection_handler.go
|   |-- workspace_lifecycle_handler.go
|   |-- internal_resolution_handler.go
|   `-- *_test.go
|-- internal/workspace/
|   |-- model.go
|   |-- errors.go
|   |-- policy.go
|   |-- catalog.go
|   |-- store.go
|   |-- root.go
|   |-- install.go
|   |-- inspection.go
|   |-- lifecycle.go
|   |-- resolution.go
|   `-- *_test.go
|-- internal/workspace/postgres/
|   |-- store.go
|   |-- root.go
|   |-- install.go
|   |-- inspection.go
|   |-- lifecycle.go
|   |-- resolution.go
|   `-- *_integration_test.go
|-- internal/config/
|   `-- config.go
`-- migrations/
    `-- 003_workspace.sql

contracts/
|-- schemas/workspace.v1.schema.json
|-- schemas/installation.v2.schema.json
|-- schemas/platform-error.v3.schema.json
|-- openapi/control-plane.v2.yaml
|-- openapi/control-plane.v3.yaml
|-- openapi/control-plane-internal.v2.yaml
|-- contracts.go
|-- validate.go
`-- *_test.go

tests/integration/workspace/
`-- workspace_test.go

docs/decisions/0005-minimal-workspace-installation-boundary.md
docs/contracts/compatibility.md
docs/runbooks/local-development.md
deploy/compose.yaml
.github/workflows/ci.yml
AGENTS.md
```

**Structure Decision**: Extend the existing single Control Plane process with a
`workspace` domain package and a nested PostgreSQL adapter. Catalog implements
the controlled published/exact resolution port without exposing its store.
Gateway depends on narrow Workspace operation interfaces; Workspace depends on
policy, store, and Catalog read ports; adapters point inward. Root/install are
the serial foundation, while inspection/lifecycle/resolution use disjoint files
for stable three-way parallelism. No generic repository, shared table access,
new process, or infrastructure abstraction is introduced.

## Implementation Order

1. Merge this contract gate so every shared route, DTO, error, and state rule is
   frozen before runtime agents begin.
2. Add the Workspace migration, domain values, owner policy port, Catalog read
   port, and store interface.
3. Implement Workspace create/read, then install/version pin/permission
   snapshot as the blocking trust foundation.
4. After install/pin is stable, run three disjoint slices in parallel:
   inspection, lifecycle, and internal exact resolution.
5. Add Gateway adapters and composition without changing the frozen contracts.
6. Add mapped unit, contract, PostgreSQL integration, HTTP, restart, race, and
   failure-path tests after each approved implementation slice.
7. Run quickstart, fallback audit, static/race verification, fresh independent
   Review, remediation, fresh Review, and Converge.

## Complexity Tracking

No constitution violations require justification. The owner policy and Catalog
reader interfaces are narrow ownership ports required to keep future identity
policy and existing Catalog storage out of Workspace domain logic; neither is a
generic framework.
