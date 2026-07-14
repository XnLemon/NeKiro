# Research: Workspace Create and Read

## Observed Baseline

- Issue #3 freezes the active Workspace v1 DTO, Northbound v3 routes, Platform
  Error v3 mappings, and the owner-policy boundary.
- The dependent branch already contains a Workspace service, PostgreSQL store,
  Gateway adapter, explicit migration, and combined readiness check.
- The existing service assigns owner and timestamps at create time, and the
  store reads the row by exact identifier.
- Existing tests cover parts of the broader Installation feature but do not
  provide a complete, separately traceable #4 create/read acceptance path.

## Decisions

### Reuse the active contract

Issue #4 does not change the public Workspace shape or route semantics. A new
contract version would create compatibility work without a product need, so
the implementation consumes the active v3 contract from #3.

### Preserve the existing ports

The Workspace service and store ports already keep Gateway, policy, and
PostgreSQL concerns separate. Adding a generic repository or a second Workspace
model would weaken ownership and create no user value.

### Treat duplicate creation as a database conflict

The service does not pre-read an existing row before insertion. The primary key
is the single conflict authority, which prevents a race from changing owner or
turning a retry into a false success.

### Authenticate before parsing or persistence

The Gateway authenticates before request decoding. Invalid credentials cannot
reach the Workspace service, and request payloads cannot influence owner
identity.

### Fail readiness explicitly

Serving verifies the schema after connecting to PostgreSQL. Missing, stale, or
incomplete schema is an operational failure, not an empty Workspace result or
an automatic migration.

## Fallback Inventory

| Candidate | Classification | Evidence |
| --- | --- | --- |
| Empty result for an unknown Workspace | Remove | Unknown Workspace is `NOT_FOUND` by issue #4 |
| Anonymous/default owner | Remove | Owner must come from trusted authentication |
| In-memory Workspace read/write | Remove | Durable PostgreSQL fact is required |
| Automatic migration during serve | Remove | Existing runbook and #3 plan require explicit migration |
| Retry or alternate persistence source | Remove | No approved policy exists |

Fallback delta for this feature: removed `0`, retained `0`, added `0`, net `0`.
Added fallback evidence: none.
