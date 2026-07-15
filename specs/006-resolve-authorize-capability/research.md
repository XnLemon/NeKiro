# Research: Resolve and Authorize an Installed Agent Capability

## Baseline Audit

Observed on branch `codex/issue-6-resolution` before edits (2026-07-15):

- `apps/control-plane/internal/workspace/service.go` already performs exact
  Installation lookup, pin comparison, Installation status check, Catalog
  exact read, publication check, capability lookup, permission containment, and
  response-contract validation.
- `apps/control-plane/internal/gateway/workspace_handler.go` already registers
  only `POST /internal/v2/resolve-agent`, injects a separate internal
  authenticator, bounds/strictly decodes the body, and separates pre- and
  post-correlation errors.
- Existing unit/integration/HTTP tests cover the happy path, disabled
  Installation, Catalog disablement, some invalid correlation, and unexpected
  internal errors. They do not prove missing Workspace precedence, internal
  credential separation, complete correlated error mapping, resolver restart,
  or unchanged Installation fields after Catalog disablement.
- The current resolver reads the current Installation before proving Workspace
  existence. A missing Workspace therefore becomes `AGENT_NOT_INSTALLED` instead
  of the Workspace contract's `NOT_FOUND` result. The active Internal v2
  response metadata is completed in place to advertise both existing 404 codes;
  the operation and version remain unchanged.
- Baseline commands passed: focused `go test`, focused `go test -race`, and
  `git diff --check`. No PostgreSQL integration database was configured in the
  baseline environment.

## Contract Facts

### Decision: Reuse active Control Plane Internal v2

Rationale: `contracts/openapi/control-plane-internal.v2.yaml` is the active
Router-to-Control Plane source of truth. It defines the exact request fields,
separate internal Bearer security, pre/post-correlation error shapes, and
`NOT_FOUND`, `AGENT_NOT_INSTALLED`, `INSTALLATION_DISABLED`, `AGENT_DISABLED`,
`CAPABILITY_NOT_ALLOWED`, and `DEPENDENCY_ERROR` outcomes.

Alternatives considered: adding a new route or changing the v2 schema version.
Rejected because Issue #6 completes the existing operation and the compatibility
policy prohibits speculative contract versions. The existing 404 response
metadata is corrected without changing its version or wire fields.

### Decision: Establish Workspace existence before current Installation lookup

Rationale: the active failure precedence distinguishes a missing Workspace
(`NOT_FOUND`) from a missing current Installation (`AGENT_NOT_INSTALLED`). The
existing narrow Store port already exposes `GetWorkspace`, so no new storage
access or shared type is needed.

Alternatives considered: making `GetCurrentInstallation` return a richer result
or treating both states as `AGENT_NOT_INSTALLED`. Rejected because the former
expands the port unnecessarily and the latter violates the contract.

### Decision: Keep Catalog read failures explicit

Rationale: a readable non-published exact Card is `AGENT_DISABLED`; a missing or
failed exact Card read is a required dependency/invariant failure and remains
`DEPENDENCY_ERROR`. No stale Card, alternate source, or retry is permitted.

### Decision: Tests after existing implementation

Rationale: the repository constitution explicitly schedules tests after the
approved implementation. The code change is one precedence correction; tests
are added for every Issue #6 acceptance gap without broad refactoring.

## No-Fallback Audit

| Existing behavior | Classification | Evidence |
| --- | --- | --- |
| Missing Workspace is currently collapsed into `AGENT_NOT_INSTALLED` | Remove | Corrected by an explicit Workspace existence read |
| Empty accepted permission set authorizes permission-free capabilities | Keep | Installation and exact Card contracts explicitly permit it |
| Catalog exact read missing maps to dependency failure | Keep | Required Catalog fact/invariant failure; no stale source |
| Separate internal Bearer principal | Keep | Active Internal v2 contract and config boundary |

Fallback delta for this feature: removed 1 behavior, retained 2 approved
behaviors, added 0, net -1. Added fallback evidence: none.
