# Data Model: Catalog Registration and Discovery

## Ownership

The Catalog domain owns every entity in this document. Registry owns Agent
identity, immutable Card versions, publication state, and capability index
rows. Discovery reads a projection over those facts and never owns a second
Card copy. No Workspace, Router, Ledger, Runtime, or Frontend module writes this
data.

The initial deployment uses one PostgreSQL schema named `catalog` inside the
shared Phase 1 database. Schema co-location does not grant another module write
access.

## Entity: Agent Identity

One row establishes the stable identity and owner shared by all versions of an
Agent.

| Field | Logical type | Constraints |
|---|---|---|
| `agent_id` | safe identifier | Primary key; immutable |
| `owner_id` | safe identifier | Required; immutable after first registration |
| `created_at` | timestamp | Required; server assigned once |

### Rules

- The first committed version registration creates the identity and owner.
- A later Card under the same `agent_id` must declare exactly the same
  `owner_id`.
- A caller may not claim an existing Agent identity by submitting a new owner.
- The identity is not deleted when a version is disabled.

## Entity: Agent Version

One row stores the immutable active-version Agent Card and mutable publication
metadata.

| Field | Logical type | Constraints |
|---|---|---|
| `agent_id` | safe identifier | Foreign key to Agent Identity; part of primary key |
| `version` | semantic-version string | Part of primary key; immutable |
| `schema_version` | string | Required; active value is `0.2` |
| `card` | JSON document | Required validated Agent Card; immutable |
| `card_digest` | 32-byte digest | Required SHA-256 of canonical mapped Card; immutable |
| `publication_status` | enum | `draft`, `published`, or `disabled` |
| `registered_at` | timestamp | Required; server assigned once |
| `published_at` | optional timestamp | Assigned on first publication; never rewritten |
| `disabled_at` | optional timestamp | Assigned on first disablement; never rewritten |

Primary key: `(agent_id, version)`.

### Card Rules

- Card `agentId` and `version` must equal the primary-key values.
- Card `owner.id` must equal Agent Identity `owner_id`.
- Structural and semantic Agent Card `0.2` validation completes before the
  transaction writes any row.
- Re-registration never replaces or merges `card`, even when the new document
  has the same digest.
- JSON storage may normalize insignificant whitespace and object member order;
  logical JSON values, number values, and all contract fields remain unchanged.
- Publication metadata is not inserted into the Card document.

### State/Timestamp Constraints

| State | `published_at` | `disabled_at` |
|---|---|---|
| `draft` | null | null |
| `published` | non-null | null |
| `disabled` after draft | null | non-null |
| `disabled` after publication | non-null | non-null |

Database checks reject every other combination.

## Entity: Agent Version Capability

This child row is a transactionally derived exact-match index over
`card.skills[*].id`.

| Field | Logical type | Constraints |
|---|---|---|
| `agent_id` | safe identifier | Part of parent foreign key and primary key |
| `version` | semantic-version string | Part of parent foreign key and primary key |
| `capability_id` | safe identifier | Part of primary key; copied from one unique Card skill ID |

Primary key: `(agent_id, version, capability_id)`.

### Rules

- Capability rows and their parent Agent Version commit in one registration
  transaction.
- Rows are derived only from the already validated Card.
- No capability row can outlive or refer to a different Card version.
- Discovery may use this table for exact filtering but must return the parent
  Card as the result fact.

## Read Model: Published Agent Version

Discovery reads a Registry-owned relational projection joining:

- Agent Identity for immutable owner;
- Agent Version for Card, status, and timestamps;
- Agent Version Capability when capability filtering is requested.

It includes only `publication_status = published`. It is a query model, not a
separately writable table. Free text compares the supplied literal substring
case-insensitively against Card `name` and `description`; `%`, `_`, and escape
characters in user text are literals rather than query wildcards.

### Ordering

The exact order is:

1. `published_at` descending;
2. `agent_id` ascending using deterministic code-point/database-C ordering;
3. exact `version` string ascending using the same deterministic ordering.

This is discovery traversal order, not version recommendation or SemVer
compatibility precedence.

### Indexes

- Partial published-order index on
  `(published_at DESC, agent_id ASC, version ASC)`.
- Capability lookup index on `(capability_id, agent_id, version)`.
- Owner lookup index on `(owner_id, agent_id)`.

The first scale target is 10,000 published versions. Free-text search may use a
bounded sequential/filter scan at that scale; no search extension or cluster is
introduced until measured pressure justifies it.

## Value Object: Discovery Filter

| Field | Logical type | Rules |
|---|---|---|
| `query` | optional string | 1-256 characters, contains non-whitespace; literal case-insensitive substring |
| `capability` | optional capability ID | Exact, case-sensitive identifier equality |
| `owner_id` | optional owner ID | Exact, case-sensitive identifier equality |
| `limit` | integer | Explicit 1-100; omitted becomes product default 25 |

All supplied filters use AND. Whitespace-only query and out-of-range explicit
limit are validation failures. Values are not trimmed into a different query;
the caller must send the intended value.

## Value Object: Discovery Cursor v1

The external cursor is opaque base64url. Its decoded strict JSON payload is an
internal versioned value:

| Field | Logical type | Purpose |
|---|---|---|
| `v` | integer | Cursor format version; exactly `1` |
| `filter_hash` | lowercase hex digest | SHA-256 of canonical query/capability/owner/limit values |
| `snapshot_published_at` | timestamp | Excludes versions published after the first page began |
| `last_published_at` | timestamp | First key of the last returned item |
| `last_agent_id` | safe identifier | Second key of the last returned item |
| `last_version` | semantic-version string | Third key of the last returned item |

### Cursor Rules

- Cursor decoding rejects unknown or duplicate JSON members, invalid base64url,
  unsupported version, invalid identifiers/timestamps, trailing content, and a
  filter hash that differs from the current request.
- A cursor grants no authorization and contains no Card, owner secret, bearer
  credential, database offset, or internal error.
- The first page records a snapshot publication boundary. Later pages retain it
  and use the last ordering tuple as a keyset continuation.
- New publications after the snapshot are excluded. A version disabled before a
  later page is read is excluded, so a page may contain fewer than `limit`
  items.
- The response includes `nextCursor` only when another eligible row exists.

## Value Object: Authenticated Caller

| Field | Logical type | Rules |
|---|---|---|
| `caller_id` | safe identifier | Required trusted identity from the Gateway authenticator |
| `authentication_kind` | enum | Identifies the configured adapter; never accepted from a raw caller header |

Bearer credentials are transport secrets and are never part of this value after
authentication. Caller identity is not persisted with the Card in this feature;
Agent owner identity is the durable authorization fact.

## State Transitions

```text
register -> draft

draft -> published -> disabled
   \----------------> disabled
```

### Publish

1. Authenticate caller and load the exact version for update.
2. Return not found when no exact version exists.
3. Require caller ID to equal immutable owner.
4. Require current state `draft`; otherwise return conflict.
5. Set state `published` and assign `published_at` in one commit.

### Disable

1. Authenticate caller and load the exact version for update.
2. Return not found when no exact version exists.
3. Require caller ID to equal immutable owner.
4. For `draft` or `published`, set `disabled` and assign `disabled_at` once.
5. For `disabled`, return the existing entry unchanged as the specified
   idempotent success.

### Concurrent Publish/Disable

The row lock serializes state evaluation. If publish commits first, disable may
then commit and the final state is disabled with both timestamps. If disable
commits first, publish observes disabled and conflicts. No successful response
may describe a state that did not commit.

## Transaction Boundaries

### Register Version

One transaction:

1. insert or lock Agent Identity;
2. verify immutable owner;
3. insert immutable Agent Version;
4. insert all capability index rows;
5. commit.

Any conflict or dependency failure rolls back every step. A database uniqueness
violation is mapped to the domain conflict only when it identifies the expected
Agent/version race; unrelated database errors remain dependency failures.

### Read and Discover

Exact reads and each discovery page use one read transaction or equivalent
consistent statement boundary. Dependency errors return explicitly; they do
not fall back to stale process memory or an empty list.

## Retention and Future Evolution

- Agent versions have no deletion transition in this feature.
- Disabled Cards remain online historical Registry facts.
- Hot deployment registries, dynamic endpoints, object-storage archives, and
  cold retention are deferred. A future design references the exact immutable
  `(agent_id, version)` rather than moving source-of-truth ownership.
