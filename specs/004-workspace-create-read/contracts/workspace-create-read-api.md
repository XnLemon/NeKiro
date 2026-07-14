# Workspace Create and Read Contract Guide

Issue #4 consumes the active Northbound v3 contract defined by issue #3. This
guide records the subset owned by this feature; the language-neutral OpenAPI
and JSON Schema remain the source of truth.

## Create Workspace

`POST /v3/workspaces`

Request body:

```json
{"workspaceId":"workspace-alpha"}
```

The request has no owner or timestamp fields. Unknown or duplicate JSON
members and trailing JSON values are invalid.

Success: `201 Created`

```json
{
  "workspaceId": "workspace-alpha",
  "ownerId": "owner-alpha",
  "createdAt": "2026-07-15T10:00:00Z",
  "updatedAt": "2026-07-15T10:00:00Z"
}
```

Failures:

| Status | Code | Meaning |
| --- | --- | --- |
| 400 | `VALIDATION_ERROR` | Invalid body, identifier, or authenticated principal |
| 401 | `UNAUTHENTICATED` | No trusted Gateway identity |
| 409 | `CONFLICT` | Identifier already exists |
| 503 | `DEPENDENCY_ERROR` | Workspace persistence failed |

## Read Workspace

`GET /v3/workspaces/{workspaceId}`

Success: `200 OK` with the exact durable Workspace DTO.

Failures:

| Status | Code | Meaning |
| --- | --- | --- |
| 400 | `VALIDATION_ERROR` | Invalid path identifier |
| 401 | `UNAUTHENTICATED` | No trusted Gateway identity |
| 403 | `FORBIDDEN` | Caller is not the Workspace owner |
| 404 | `NOT_FOUND` | Workspace does not exist |
| 503 | `DEPENDENCY_ERROR` | Workspace persistence failed |

All responses include `x-nek-trace-id`. Platform Error v3 bodies use fixed
messages and never include database details, credentials, or Workspace data
for failed authorization.
