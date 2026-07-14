# Data Model: Workspace Create and Read

## Workspace

The Workspace module owns one durable row per exact Workspace identifier.

| Field | Type/constraint | Meaning |
| --- | --- | --- |
| `workspace_id` | safe identifier, primary key | Caller-selected exact identity |
| `owner_id` | safe identifier, required | Trusted authenticated creator |
| `created_at` | timestamp, required | Server-assigned creation time |
| `updated_at` | timestamp, required | Server-assigned current fact time |

The database enforces `created_at <= updated_at`. The public DTO maps these
four fields to `workspaceId`, `ownerId`, `createdAt`, and `updatedAt`.

## Ownership and Mutability

- `workspace_id`, `owner_id`, `created_at`, and `updated_at` are not modified by
  the #4 API after insertion.
- Create receives only `workspaceId`.
- Owner identity is supplied by the authenticated Gateway context.
- Read returns the durable row only after the owner policy succeeds.
- A duplicate insert has no update path and therefore cannot change the row.

## Storage Boundary

The table lives under the Workspace-owned `workspace` schema. Catalog may use
the same PostgreSQL instance but cannot read or write this table. The existing
Installation table is present because the schema migration is shared with the
dependent #3 runtime; #4 does not add or mutate Installation behavior.

## Failure Mapping

| Condition | Domain result | Public result |
| --- | --- | --- |
| Missing/rejected authentication | unauthenticated | `401 UNAUTHENTICATED` |
| Invalid body/path/principal | invalid | `400 VALIDATION_ERROR` |
| Existing identifier | conflict | `409 CONFLICT` |
| Existing row, non-owner | forbidden | `403 FORBIDDEN` |
| Unknown identifier | not found | `404 NOT_FOUND` |
| Schema/database/query/commit failure | dependency | `503 DEPENDENCY_ERROR` |

Every public result carries the Gateway trace header; errors use the active
fixed Platform Error v3 message and trace value.
