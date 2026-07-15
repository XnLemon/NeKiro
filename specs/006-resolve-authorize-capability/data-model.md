# Data Model: Exact Resolution

## Existing Facts

### Workspace

Owned by Workspace persistence. Resolution reads its existence and does not
return or mutate it.

| Field | Meaning |
| --- | --- |
| `workspaceId` | Exact logical authorization root identifier |
| `ownerId` | Trusted immutable creator identity; not accepted from resolution request |
| `createdAt`, `updatedAt` | Server timestamps |

### Installation

Owned by Workspace persistence. A current Installation is enabled or disabled;
uninstalled records are historical and cannot resolve.

| Field | Resolution use |
| --- | --- |
| `installationId` | Returned exact authorization fact |
| `workspaceId`, `agentId` | Must equal request values |
| `installedVersion` | Must equal requested exact version |
| `acceptedPermissions` | Complete containment source for capability authorization |
| `status` | Must be `enabled` on success; `disabled` is an explicit failure |
| version constraint/timestamps | Persisted facts, not changed by resolution and not returned in `ResolvedInstallation` |

### Agent Card

Owned by Catalog. The exact requested version is re-read on every resolution;
only a currently published Card is eligible. The Card contract is the complete
approved response shape and contains no credential, health, input, or output
payload data.

## Transient Resolution Result

`ResolveAgentResponse` contains exactly:

- the exact active `card`;
- `installation` with `installationId`, `workspaceId`, `agentId`,
  `installedVersion`, `acceptedPermissions`, and `status: enabled`.

The result has no persistence identity beyond the source facts and no Agent
invocation/result content.

## State/Decision Flow

```text
request valid
  -> Workspace exists
  -> current Installation exists and exact pin matches
  -> Installation enabled
  -> exact Catalog Card readable and published
  -> capability exists
  -> required permissions subset of accepted snapshot
  -> return exact result
```

Any failed step returns its active typed error and no partial response.
