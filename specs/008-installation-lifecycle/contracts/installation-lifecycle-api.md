# Installation Lifecycle Contract Guide

Issue #8 consumes the active Northbound v3 contract; this guide records the
mapped lifecycle behavior without creating a second contract source.

## Enable or Disable

`PATCH /v3/workspaces/{workspaceId}/installations/{installationId}`

Request:

```json
{"status":"disabled"}
```

The only valid targets are `enabled` and `disabled`. Success is `200` with the
complete Installation v2 response and `x-nek-trace-id`.

## Uninstall

`DELETE /v3/workspaces/{workspaceId}/installations/{installationId}`

Only a disabled Installation can be uninstalled. Success is `200` with the
preserved terminal Installation v2 response and `uninstalledAt == updatedAt`.

## Failure Mapping

| Condition | Status | Code |
| --- | ---: | --- |
| Invalid target/body/path | 400 | `VALIDATION_ERROR` |
| Missing/invalid bearer | 401 | `UNAUTHENTICATED` |
| Non-owner | 403 | `FORBIDDEN` |
| Workspace/Installation missing or cross-Workspace | 404 | `NOT_FOUND` |
| Illegal, same-state, or repeated terminal transition | 409 | `CONFLICT` |
| Workspace persistence/transaction failure | 503 | `DEPENDENCY_ERROR` |

Every response has a safe Trace header. Error payloads contain no credentials,
Catalog details, endpoint values, or dependency internals.
