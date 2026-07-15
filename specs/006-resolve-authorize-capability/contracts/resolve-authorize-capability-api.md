# Resolve and Authorize Capability Contract Guide

The language-neutral source of truth is
`contracts/openapi/control-plane-internal.v2.yaml`, with Agent Card v0.2,
Installation v2, and Platform Error v3 schemas.

## Operation

`POST /internal/v2/resolve-agent`

Required request fields:

```json
{
  "invocationId": "invocation-alpha",
  "rootTaskId": "task-alpha",
  "traceId": "trace-alpha",
  "workspaceId": "workspace-alpha",
  "agentId": "agent-alpha",
  "version": "1.0.0",
  "capability": "document.read"
}
```

The operation requires the separately configured internal Bearer principal.
Northbound credentials are not implicitly trusted.

## Success

`200 application/json` contains only the active exact Card and:

```json
{
  "installationId": "installation-alpha",
  "workspaceId": "workspace-alpha",
  "agentId": "agent-alpha",
  "installedVersion": "1.0.0",
  "acceptedPermissions": ["document.read"],
  "status": "enabled"
}
```

## Failure Mapping

| Condition | Status/code |
| --- | --- |
| Valid correlation, missing Workspace | `404 NOT_FOUND` |
| Valid correlation, no current Installation or exact pin mismatch | `404 AGENT_NOT_INSTALLED` |
| Disabled Installation | `403 INSTALLATION_DISABLED` |
| Readable exact Card not published | `403 AGENT_DISABLED` |
| Missing capability or incomplete permission containment | `403 CAPABILITY_NOT_ALLOWED` |
| Catalog/Workspace dependency failure | `503 DEPENDENCY_ERROR` |
| Valid internal auth absent | `401 UNAUTHENTICATED` |
| Invalid request/correlation | `400 VALIDATION_ERROR` |

After strict correlation validation, the error body and `x-nek-trace-id`
preserve exact request `invocationId`, `rootTaskId`, and `traceId`. Before that
point, the body contains only fixed `code`, `message`, and generated safe
`traceId`; malformed IDs are not echoed. The active v2 404 response advertises
both `NOT_FOUND` and `AGENT_NOT_INSTALLED` so Workspace absence remains distinct
from missing or mismatched Installation state.
