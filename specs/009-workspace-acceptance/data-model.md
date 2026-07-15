# Data Model: Workspace Acceptance

Issue #9 introduces no persisted entity and no schema migration. It verifies
the existing facts and their ownership.

## Existing Facts Under Test

| Fact | Owner | Acceptance assertions |
| --- | --- | --- |
| Published Agent Card/version | Catalog | Capability discovery, exact publication selection, Catalog disable outcome |
| Workspace | Workspace | Immutable owner, create/read durability, owner authorization |
| Installation | Workspace | Exact pin, permission snapshot, lifecycle status, timestamps, history, current-row uniqueness |
| Resolution correlation | Gateway/Workspace boundary | Invocation ID, root Task ID, Trace ID preserved on success and typed failures |
| Platform Error | Gateway contract | Status/code distinction, safe Trace header, no secret/dependency detail |

## Acceptance Evidence Record

Tests and documents use the following conceptual evidence tuple; it is not a
new runtime type or database table:

```text
requirement_id
scenario
boundary
setup
observed_result
verification_command
```

`boundary` identifies Catalog, Workspace, public Gateway, internal Gateway, or
PostgreSQL. `observed_result` must distinguish success, conflict, not found,
forbidden, unauthenticated, validation, disabled, and dependency failure.

## Invariants

- Workspace owner identity is immutable and comes from the authenticated
  caller, never from request data.
- A current Installation is enabled or disabled and is unique per Workspace /
  Agent; an uninstalled row remains historical and releases the current slot.
- Installation immutable fields do not change across lifecycle transitions or
  Catalog publication changes.
- A successful lifecycle timestamp is strictly later than the locked prior
  `updatedAt` after PostgreSQL precision normalization.
- A successful resolution identifies the exact enabled Installation and current
  published Card, and carries all required correlation identifiers.
- An error never becomes an empty result, stale result, or successful response.
