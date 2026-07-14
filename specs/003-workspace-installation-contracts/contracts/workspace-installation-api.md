# Contract: Minimal Workspace and Installation

## Active Sources

| Contract | Active artifact | Gate change |
| --- | --- | --- |
| Workspace | `contracts/schemas/workspace.v1.schema.json` | New exact public Workspace fact |
| Installation | `contracts/schemas/installation.v2.schema.json` | Freeze semantic ordering and timestamp invariants |
| Northbound | `contracts/openapi/control-plane.v3.yaml` | Complete seven Workspace/Installation operations without changing v2 |
| Internal resolution | `contracts/openapi/control-plane-internal.v2.yaml` | Complete service authentication, pre/post-correlation, and exact failures |
| Platform Error | `contracts/schemas/platform-error.v3.schema.json` | Add `INSTALLATION_DISABLED` without changing v2 consumers |

Historical Northbound v1, Agent Card 0.1, Router Internal v1, and every other
historical artifact remain unchanged. No runtime route accepts a historical
Workspace or Installation contract.

## Shared Transport Rules

- Every Northbound operation requires Gateway Bearer authentication.
- Every Northbound success and error includes `x-nek-trace-id`; Workspace
  errors use Platform Error v3 with the same exact `traceId`.
- The internal operation requires a trusted service Bearer identity. After
  strict correlation validation, every success and error includes
  `x-nek-trace-id` equal to the request `traceId`.
- JSON objects reject unknown and duplicate members, trailing values, null for
  required fields, and values outside active referenced schemas.
- Public messages remain fixed Platform Error messages (v2 for
  Catalog/Invocation and v3 for Workspace/internal resolution). They contain no
  bearer token, Card body, endpoint, permission details, database error, stack,
  or Agent input/output.

## Workspace DTO

```json
{
  "workspaceId": "workspace-alpha",
  "ownerId": "owner-alpha",
  "createdAt": "2026-07-14T10:00:00Z",
  "updatedAt": "2026-07-14T10:00:00Z"
}
```

All four fields are required. No owner or timestamp appears in create input.

## Installation DTO

Current Installation:

```json
{
  "installationId": "installation-alpha",
  "workspaceId": "workspace-alpha",
  "agentId": "runtime-a",
  "versionConstraint": "^1.2.0",
  "installedVersion": "1.4.3",
  "acceptedPermissions": ["document.read"],
  "status": "enabled",
  "installedAt": "2026-07-14T10:01:00Z",
  "updatedAt": "2026-07-14T10:01:00Z"
}
```

Uninstalled Installation additionally requires:

```json
{
  "status": "uninstalled",
  "uninstalledAt": "2026-07-14T10:05:00Z"
}
```

`uninstalledAt` is forbidden while status is enabled or disabled. It is
required for uninstalled. Installation v2 also requires sorted unique
`acceptedPermissions`, `installedAt <= updatedAt`, and
`uninstalledAt == updatedAt` for terminal records. Contract validation does not
sort, trim, coerce, or fill values.

## Northbound Operations

### Create Workspace

`POST /v3/workspaces`

Request:

```json
{"workspaceId":"workspace-alpha"}
```

| Status | Error codes | Meaning |
| --- | --- | --- |
| `201` | - | Returns Workspace; authenticated caller is immutable owner |
| `400` | `VALIDATION_ERROR` | Invalid JSON or Workspace ID |
| `401` | `UNAUTHENTICATED` | No valid Gateway identity |
| `409` | `CONFLICT` | Workspace ID already exists |
| `503` | `DEPENDENCY_ERROR` | Workspace persistence failed |

### Read Workspace

`GET /v3/workspaces/{workspaceId}`

| Status | Error codes | Meaning |
| --- | --- | --- |
| `200` | - | Returns exact Workspace |
| `400` | `VALIDATION_ERROR` | Invalid path identifier |
| `401` | `UNAUTHENTICATED` | No valid Gateway identity |
| `403` | `FORBIDDEN` | Authenticated caller is not owner |
| `404` | `NOT_FOUND` | Workspace does not exist |
| `503` | `DEPENDENCY_ERROR` | Workspace persistence failed |

### Install Agent

`POST /v3/workspaces/{workspaceId}/installations`

Request:

```json
{
  "agentId": "runtime-a",
  "versionConstraint": "^1.2.0",
  "acceptedPermissions": ["document.read"]
}
```

| Status | Error codes | Meaning |
| --- | --- | --- |
| `201` | - | Returns new enabled exact-pinned Installation |
| `400` | `VALIDATION_ERROR` | Invalid ID/range, duplicate or unknown permission |
| `401` | `UNAUTHENTICATED` | No valid Gateway identity |
| `403` | `FORBIDDEN` | Authenticated caller is not owner |
| `404` | `NOT_FOUND` | Workspace or matching published Agent version absent |
| `409` | `CONFLICT` | Current Installation already exists |
| `503` | `DEPENDENCY_ERROR` | Workspace or Catalog dependency failed |

Error precedence is authentication, structural/semantic request validation,
Workspace existence, owner authorization, current Installation conflict,
Catalog selection, permission subset validation, and insert. The concurrent
partial-unique race maps to the same conflict.

### List Installations

`GET /v3/workspaces/{workspaceId}/installations`

Success:

```json
{"items":[]}
```

The response always has `items`; it includes enabled, disabled, and uninstalled
facts ordered by `installedAt`, then `installationId`, ascending. `limit` is
required with bounds `1-100`. `cursor` is opaque and bound to
the Workspace, page size, and last ordering tuple. `nextCursor` is omitted on
the final page and on a genuine empty result.

| Status | Error codes | Meaning |
| --- | --- | --- |
| `200` | - | Returns one bounded page; empty is a genuine result only after successful owner authorization and query |
| `400` | `VALIDATION_ERROR` | Invalid path identifier |
| `401` | `UNAUTHENTICATED` | No valid Gateway identity |
| `403` | `FORBIDDEN` | Authenticated caller is not owner |
| `404` | `NOT_FOUND` | Workspace does not exist |
| `503` | `DEPENDENCY_ERROR` | Workspace persistence failed |

### Read Installation

`GET /v3/workspaces/{workspaceId}/installations/{installationId}`

| Status | Error codes | Meaning |
| --- | --- | --- |
| `200` | - | Returns complete current or historical Installation |
| `400` | `VALIDATION_ERROR` | Invalid path identifier |
| `401` | `UNAUTHENTICATED` | No valid Gateway identity |
| `403` | `FORBIDDEN` | Authenticated caller is not owner |
| `404` | `NOT_FOUND` | Workspace or Installation under it does not exist |
| `503` | `DEPENDENCY_ERROR` | Workspace persistence failed |

### Enable or Disable Installation

`PATCH /v3/workspaces/{workspaceId}/installations/{installationId}`

Request:

```json
{"status":"disabled"}
```

Only `enabled` and `disabled` are request values.

| Status | Error codes | Meaning |
| --- | --- | --- |
| `200` | - | Returns Installation after one legal transition |
| `400` | `VALIDATION_ERROR` | Invalid path, JSON, or status |
| `401` | `UNAUTHENTICATED` | No valid Gateway identity |
| `403` | `FORBIDDEN` | Authenticated caller is not owner |
| `404` | `NOT_FOUND` | Workspace or Installation under it does not exist |
| `409` | `CONFLICT` | Same-state or transition from uninstalled |
| `503` | `DEPENDENCY_ERROR` | Workspace persistence failed |

### Uninstall Agent

`DELETE /v3/workspaces/{workspaceId}/installations/{installationId}`

| Status | Error codes | Meaning |
| --- | --- | --- |
| `200` | - | Returns the preserved terminal Installation after uninstall |
| `400` | `VALIDATION_ERROR` | Invalid path identifier |
| `401` | `UNAUTHENTICATED` | No valid Gateway identity |
| `403` | `FORBIDDEN` | Authenticated caller is not owner |
| `404` | `NOT_FOUND` | Workspace or Installation under it does not exist |
| `409` | `CONFLICT` | Installation is enabled or already uninstalled |
| `503` | `DEPENDENCY_ERROR` | Workspace persistence failed |

Uninstall is not idempotent. The owner must disable an enabled Installation
first.

## Internal Exact Resolution

`POST /internal/v2/resolve-agent`

Request remains:

```json
{
  "invocationId": "invocation-alpha",
  "rootTaskId": "task-alpha",
  "traceId": "trace-alpha",
  "workspaceId": "workspace-alpha",
  "agentId": "runtime-a",
  "version": "1.4.3",
  "capability": "runtime.echo"
}
```

Success returns exact Agent Card 0.2 plus:

```json
{
  "installationId": "installation-alpha",
  "workspaceId": "workspace-alpha",
  "agentId": "runtime-a",
  "installedVersion": "1.4.3",
  "acceptedPermissions": ["document.read"],
  "status": "enabled"
}
```

| Status | Error codes | Meaning |
| --- | --- | --- |
| `200` | - | Exact currently published Card and enabled authorization facts |
| `400` | `VALIDATION_ERROR` | Invalid request or correlation; before strict correlation the body contains only fixed fields and a generated trace |
| `401` | `UNAUTHENTICATED` | Untrusted internal principal; pre-correlation body only |
| `403` | `INSTALLATION_DISABLED` | Current exact Installation is disabled |
| `403` | `AGENT_DISABLED` | Pinned exact Catalog version is disabled |
| `403` | `CAPABILITY_NOT_ALLOWED` | Missing capability or insufficient accepted permissions |
| `404` | `AGENT_NOT_INSTALLED` | No current Installation or exact version mismatch |
| `503` | `DEPENDENCY_ERROR` | Workspace or Catalog dependency failed/invariant missing |

After strict correlation validation, every Platform Error repeats exact request
`invocationId`, `rootTaskId`, and `traceId`, and the response header repeats the
request `traceId`. A pre-correlation error contains only `code`, fixed
`message`, and a generated safe `traceId`; it never echoes malformed or missing
request identifiers. Failure returns no Card or Installation fragment.

## Platform Error v3 Addition

```json
{
  "code": "INSTALLATION_DISABLED",
  "message": "The Agent installation is disabled.",
  "traceId": "trace-alpha",
  "invocationId": "invocation-alpha",
  "rootTaskId": "task-alpha"
}
```

The code is owned by Workspace Installation state. `AGENT_DISABLED` remains
owned by Catalog Agent-version state. Neither code is reused for the other
condition.

## Compatibility

These are pre-runtime completions of active contract foundations. Installation
semantic rules, Northbound behavior, and internal error/correlation behavior
are versioned explicitly. Northbound v2 remains byte-unchanged historical
evidence; first runtime consumers implement Northbound v3 only. Platform Error
v2 remains active for Catalog/Invocation operations in v3; active v3
Workspace and internal resolution use Platform Error v3. No compatibility
decoder, dual route, retry, or fallback is introduced.
