# Data Model: Installation Lifecycle

## Installation State

| Status | `uninstalledAt` | Legal next operation |
| --- | --- | --- |
| `enabled` | null | disable |
| `disabled` | null | enable or uninstall |
| `uninstalled` | terminal timestamp equal to `updatedAt` | none |

`installationId`, `workspaceId`, `agentId`, `versionConstraint`,
`installedVersion`, and `acceptedPermissions` are immutable after creation.
`installedAt <= updatedAt` always holds. A terminal row remains queryable and
is never hard-deleted or reused.

## Constraints

- Workspace foreign key remains valid.
- Status is one of `enabled`, `disabled`, or `uninstalled`.
- Enabled and disabled rows have no `uninstalledAt`.
- Uninstalled rows have `uninstalledAt == updatedAt`.
- Partial unique `(workspace_id, agent_id)` applies only to non-uninstalled
  rows, releasing the current slot atomically with uninstall.
- Ordered list index remains `(workspace_id, installed_at, installation_id)`.

## Transition Fact

A successful transition is one committed row version. The response must be the
row returned by the same update transaction, and restart reconstruction must
produce equal values. Failed transitions return no row and do not advance
`updatedAt`.

## Ownership

Workspace owns Installation data and the immutable owner policy. Catalog owns
Agent publication state. Lifecycle does not read or write Catalog tables.
