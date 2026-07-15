# Acceptance Evidence Contract Guide

Issue #9 consumes active contracts and adds no public or internal contract
version.

## Active Operations Reused

- Public Catalog discovery: `GET /v3/agents?capability=...`
- Public Workspace creation: `POST /v3/workspaces`
- Public Installation: `POST /v3/workspaces/{workspaceId}/installations`
- Public inspection: `GET /v3/workspaces/{workspaceId}/installations` and
  `GET /v3/workspaces/{workspaceId}/installations/{installationId}`
- Public lifecycle: `PATCH` and `DELETE` on the active v3 Installation routes
- Internal resolution: `POST /internal/v2/resolve-agent`

## Evidence Rules

1. Use the active route and schema versions only. Historical v2 public routes
   are not accepted as compatibility evidence.
2. Public operations authenticate with the northbound development principal;
   internal resolution authenticates with the separate internal principal.
3. Successful responses must contain committed facts and the expected
   `x-nek-trace-id` header. Internal resolution must preserve the request Trace
   ID in its response/error mapping after validation.
4. Error responses must use the documented Platform Error status/code mapping
   and must not expose credentials, tokens, passwords, connection strings, or
   dependency internals.
5. Test setup may seed Catalog facts through the Catalog service, but discovery
   and the Workspace/resolution acceptance path must cross the actual Gateway
   handler boundary.

## Compatibility Decision

No schema, OpenAPI, DTO, route, event, or version change is required. A future
test failure that would require changing one of these is a contract decision
outside Issue #9 and must be recorded before implementation.
