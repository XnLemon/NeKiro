# Implementation Plan: Create and Read an Owner-Controlled Workspace

**Branch**: `codex/004-workspace-create-read`
**Date**: 2026-07-15
**Spec**: [spec.md](spec.md)

**Input**: Issue #4 and the active Workspace contracts frozen by issue #3.

## Summary

Verify and complete the first runnable Workspace root slice in the existing
Control Plane. The Gateway authenticates a caller, accepts only an explicit
safe Workspace identifier, and delegates to the Workspace service. The service
uses a narrow owner policy and a Workspace-owned PostgreSQL adapter to create
or read the exact four-field Workspace fact. The slice includes explicit
migration/readiness checks, durable restart evidence, HTTP error mapping, and
dependency-failure tests. Installation behavior already present on the
dependent #3 branch is outside this feature's implementation scope.

## Constitution Check

*GATE: Passed before implementation and must be re-checked after tests and
independent Review.*

- **Phase 1 loop - PASS**: Workspace creation is the authorization root needed
  for `Install`; it directly enables the `Discover -> Install` loop.
- **Ownership - PASS**: Gateway adapts Northbound HTTP; Workspace owns its
  rows, migrations, and persistence; Catalog has no Workspace table access.
- **Runtime independence - PASS**: No Agent Runtime, endpoint, model, or
  deployment behavior is introduced.
- **Contracts - PASS**: Active Workspace v1, Northbound v3, and Platform Error
  v3 are reused. No historical contract route or dual-read path is added.
- **Failure safety - PASS**: Authentication, validation, conflict, forbidden,
  not-found, and dependency failures remain distinct. No default owner or
  persistence fallback exists.
- **SDD and review - PASS**: This feature has its own Spec, Plan, Tasks,
  analysis record, mapped tests, independent Review, and Converge gate.

## Technical Context

**Language/Version**: Go version declared by `go.mod`; OpenAPI 3.1 and JSON
Schema remain contract facts.

**Runtime**: Existing standard-library HTTP Gateway and Control Plane process.

**Storage**: Existing Workspace-owned PostgreSQL schema and `pgx/v5` adapter.
The Workspace row is durable and contains only the approved four fields.

**Testing**: Go unit tests, active OpenAPI/schema mapping tests, HTTP tests,
and integration-tagged PostgreSQL tests. Integration tests use a dedicated
database whose name ends in `_test`.

**Constraints**: Reuse active v3 contracts and existing ports. Serving never
auto-migrates. Required configuration has no defaults. No cache, retry,
alternate source, anonymous identity, or in-memory persistence is allowed.

## Architecture and Ownership

```text
HTTP client
  -> Gateway WorkspaceHandler
  -> Authenticator
  -> workspace.Service
  -> Authorizer policy port
  -> workspace.Store port
  -> Workspace PostgreSQL schema
```

- `WorkspaceHandler` owns HTTP decoding, authentication ordering, trace
  headers, status mapping, and fixed Platform Error responses.
- `workspace.Service` owns identifier validation, trusted-owner assignment,
  owner authorization, and operation ordering.
- `workspace.Authorizer` owns the replaceable owner decision. The current
  policy is exact owner-ID equality.
- `workspace.Store` owns the persistence boundary. Its PostgreSQL adapter
  writes only `workspace.workspaces`.
- `Catalog` remains an independent module and cannot access Workspace tables.

## Persistence and Failure Semantics

- Create is one Workspace-owned insert transaction. The primary-key conflict
  maps to `CONFLICT`; all other insert/begin/commit failures map to
  `DEPENDENCY_ERROR`.
- Read is an indexed exact lookup. No row maps to `NOT_FOUND`; query failure
  maps to `DEPENDENCY_ERROR`; owner policy runs before returning the body.
- Migration is explicit and forward-only. `serve` verifies the Workspace
  schema and readiness but never migrates it.
- Readiness verifies the expected schema version, Workspace table,
  Installation table, current-install uniqueness index, and list index because
  the active Workspace schema is shared with the dependent Installation slice.
  A missing or stale dependency fails readiness rather than returning degraded
  success.
- The four returned fields are read from the durable row. No timestamp or owner
  is synthesized during reads.

## Contract Surface

The implementation consumes, without modifying, these active artifacts from
issue #3:

- `contracts/schemas/workspace.v1.schema.json`
- `contracts/openapi/control-plane.v3.yaml`
- `contracts/schemas/platform-error.v3.schema.json`
- `contracts/workspace_api_contracts_test.go`

The public operations are `POST /v3/workspaces` and
`GET /v3/workspaces/{workspaceId}`. The create request contains only
`workspaceId`; owner and timestamps are response-only fields.

## Files and Tests

Existing runtime files to verify or adjust:

- `apps/control-plane/internal/workspace/service.go`
- `apps/control-plane/internal/workspace/policy.go`
- `apps/control-plane/internal/workspace/postgres/store.go`
- `apps/control-plane/internal/workspace/postgres/migrations.go`
- `apps/control-plane/internal/gateway/workspace_handler.go`
- `apps/control-plane/cmd/control-plane/main.go`

Focused evidence added for this feature:

- Workspace service unit tests for create/read policy and error precedence.
- Gateway HTTP tests for create/read success, auth rejection, duplicate
  mapping, owner/non-owner/not-found mapping, and body field rejection.
- PostgreSQL integration tests for create/read persistence, restart-style
  service reconstruction, conflict preservation, and query failure.
- Migration/readiness integration tests for missing, stale, and incomplete
  Workspace schema.

## Implementation Order

1. Observe the dependent branch and freeze the issue #4 Spec, Plan, research,
   data model, contract guide, checklist, and Tasks.
2. Re-run contract and static analysis before business-code changes.
3. Verify or correct the existing Workspace service, policy, PostgreSQL store,
   Gateway composition, and readiness behavior only where mapped to #4.
4. Add post-implementation unit, HTTP, and PostgreSQL evidence.
5. Run the full quality matrix and fallback audit.
6. Run a fresh independent Review against this Spec, Plan, Tasks, active
   contracts, and the constitution; append and resolve any findings through
   Converge.

## Complexity Tracking

No constitution violations require justification. Reusing the existing
Workspace/Installation schema readiness checks is necessary because #4 and its
dependent #3 share one owned schema; it does not add a new abstraction or
cross-module ownership.
