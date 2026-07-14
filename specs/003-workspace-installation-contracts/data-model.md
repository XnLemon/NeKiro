# Data Model: Minimal Workspace and Installation

## Ownership

The Workspace domain owns every durable entity in this document. Catalog owns
Agent Card versions and publication state; Workspace consumes those facts only
through a controlled Catalog read port. Gateway owns transport adaptation and
trusted caller extraction. Router, Dispatch, Ledger, Agent Runtime, Frontend,
Kubernetes, and Membership/RBAC own no field here.

The first deployment uses one PostgreSQL schema named `workspace` in the shared
Phase 1 database. Sharing the database instance does not permit Workspace SQL to
read or write the `catalog` schema, or Catalog SQL to read or write the
`workspace` schema.

## Entity: Workspace

One row defines a logical authorization and audit root.

| Field | Logical type | Constraints |
| --- | --- | --- |
| `workspace_id` | safe identifier | Primary key; caller selected; immutable; exact case-sensitive identity |
| `owner_id` | safe identifier | Required; derived from trusted creator; immutable |
| `created_at` | timestamp | Required; server assigned once |
| `updated_at` | timestamp | Required; server assigned; equals `created_at` in Phase 1 |

### Rules

- Create input contains only `workspaceId`; the Gateway-authenticated caller ID
  becomes `owner_id`.
- Duplicate `workspace_id` is a conflict even when the same owner repeats the
  request. Create is not idempotent.
- The owner cannot be changed in Phase 1.
- No deletion transition exists. A Workspace remains the parent of its current
  and historical Installation records.
- The initial owner-only policy compares exact caller and owner identifiers
  behind a replaceable authorization port. The policy result is not stored as a
  second field.
- `workspace_id` and `owner_id` use deterministic bytewise comparison for
  identity and ordering; neither value is trimmed or case-folded.

### Excluded Fields

Workspace has no name, description, organization, members, roles, Kubernetes
namespace, deployment target, quota, approval, policy payload, credential, or
runtime status in this feature.

## Entity: Installation

One row records one completed Agent install event and its current or terminal
lifecycle state.

| Field | Logical type | Constraints |
| --- | --- | --- |
| `installation_id` | safe identifier | Primary key; platform assigned; immutable; never reused |
| `workspace_id` | safe identifier | Required parent Workspace; immutable |
| `agent_id` | safe identifier | Required Agent identity; immutable |
| `version_constraint` | SemVer range string | Required; accepted only by the strict SemVer range parser; exact submitted value; immutable |
| `installed_version` | strict SemVer string | Required exact selected Card version; immutable |
| `accepted_permissions` | ordered array of permission IDs | Required; may be empty; unique and ascending bytewise; immutable |
| `status` | enum | Required: `enabled`, `disabled`, or `uninstalled` |
| `installed_at` | timestamp | Required; server assigned once |
| `updated_at` | timestamp | Required; server assigned on insert and each committed transition |
| `uninstalled_at` | optional timestamp | Required only for `uninstalled`; assigned once at terminal transition |

Foreign key: `workspace_id` references `Workspace.workspace_id` with no cascade
delete.

Partial unique key: `(workspace_id, agent_id)` where
`status <> 'uninstalled'`.

### Immutable Pin Rules

- `version_constraint` records what the owner requested. It is never rewritten
  to the exact version or a normalized range.
- `installed_version` equals the exact version from the Card selected by the
  successful Catalog operation. It never changes on enable, disable, Catalog
  disable, or publication of newer versions.
- `accepted_permissions` is validated against the selected exact Card before
  insert. The stored array is sorted after exact case-sensitive validation; it
  is never expanded from later Card versions.
- The empty accepted set is valid. It authorizes only capabilities whose exact
  Card `requiredPermissions` set is empty.
- The Installation contains no full or partial Agent Card copy, endpoint,
  authentication material, deployment data, health, or invocation content.

### State/Timestamp Constraints

| Status | `uninstalled_at` | Allowed next status |
| --- | --- | --- |
| `enabled` | null | `disabled` |
| `disabled` | null | `enabled`, `uninstalled` |
| `uninstalled` | non-null and equal to terminal `updated_at` | none |

Additional constraints:

- `installed_at <= updated_at`.
- A non-uninstalled row must have `uninstalled_at = null`.
- An uninstalled row must have `uninstalled_at = updated_at`.
- No successful state mutation changes immutable pin or permission fields.
- Repeating the current state, uninstalling enabled, or mutating uninstalled is
  conflict. There is no same-state or terminal idempotency policy.

## Value Object: Create Workspace Request

| Field | Rules |
| --- | --- |
| `workspaceId` | Required safe identifier; no owner or timestamp accepted |

Unknown, duplicate, null, empty, or unsafe fields fail validation. Trusted owner
identity comes from authentication and is not merged from request data.

## Value Object: Install Agent Request

| Field | Rules |
| --- | --- |
| `agentId` | Required safe identifier; exact case-sensitive match |
| `versionConstraint` | Required valid SemVer range accepted by the strict parser; no trimming or coercion |
| `acceptedPermissions` | Required unique array; each item is a safe permission identifier; empty array allowed |

### Version Candidate Ordering

Catalog considers only currently published versions of the requested Agent that
satisfy the exact constraint. Candidate order is:

1. SemVer precedence descending;
2. exact original version string descending by bytewise order.

The first candidate is selected. Build metadata is ignored by the first key as
required by SemVer and used only through the second deterministic key.
Pre-release candidates participate only in a matching constraint branch that
explicitly contains a pre-release comparator.

## Value Object: Installation State Request

| Field | Rules |
| --- | --- |
| `status` | Required; exactly `enabled` or `disabled` |

Uninstall is a separate terminal operation and is not represented by this
request. Unknown fields and unsupported status values fail validation before
state lookup.

## Value Object: Installation List

| Field | Rules |
| --- | --- |
| `items` | Required array of complete Installation facts; may be empty |
| `nextCursor` | Optional opaque continuation token; absent when no later page exists |

Items are ordered by `installed_at ASC, installation_id ASC` using deterministic
bytewise identifier ordering. Each page is bounded by a required explicit
`limit` in the range 1-100. The opaque cursor encodes the active
contract version, Workspace/filter binding, page size, and the last ordering
tuple. A malformed or filter/page-size-mismatched cursor is validation failure;
the service never silently restarts from page one. The empty array is valid only
after the Workspace exists, authentication succeeds, owner policy allows the
caller, and the query successfully proves there are no rows.

## Installation v2 Semantic Conformance

The active structural schema is `contracts/schemas/installation.v2.schema.json`.
Its conformance validator additionally enforces the rules in
`contracts/installation/v2/semantic-rules.md`: accepted permissions are unique
and ascending by bytewise identifier, `installedAt <= updatedAt`, and an
uninstalled record has `uninstalledAt == updatedAt`. Validation rejects these
violations; it never sorts, trims, coerces, or fills received values.

## Controlled Read Model: Published Version Selection

This is a transient Catalog-owned result, not a Workspace table.

| Field | Source | Rules |
| --- | --- | --- |
| `card` | Catalog exact Agent Card 0.2 fact | Required; exact selected published version |
| `publication_status` | Catalog lifecycle | Must be `published` at the selection linearization point |

Workspace calls the controlled Catalog interface with exact `agentId` and
`versionConstraint`. Catalog validates the constraint, evaluates current
published candidates, applies the documented order, and returns one exact Card
or an explicit not-found/dependency failure.

No selected Card is persisted by Workspace. A Catalog disable after this result
may commit before or after the Workspace transaction; it does not invalidate the
historical selection result or rewrite the new Installation.

## Controlled Read Model: Exact Catalog Version

This is a transient Catalog-owned result used by internal exact resolution.

| Field | Rules |
| --- | --- |
| `card` | Exact immutable Agent Card 0.2 for requested `(agentId, version)` |
| `publication_status` | Exact current Catalog state: `draft`, `published`, or `disabled` |

An installed exact version is expected to remain present because Registry
retains history. A missing exact Catalog fact after a valid Installation is a
dependency/integrity failure, not an alternate not-found source and not a reason
to consult another Card.

## Value Object: Exact Resolution Request

| Field | Rules |
| --- | --- |
| `invocationId` | Required existing safe Invocation identifier |
| `rootTaskId` | Required existing safe root Task identifier |
| `traceId` | Required existing safe Trace identifier |
| `workspaceId` | Required exact Workspace identifier |
| `agentId` | Required exact Agent identifier |
| `version` | Required strict exact SemVer; must equal the current Installation pin |
| `capability` | Required exact capability identifier |

The request is authenticated as a trusted internal service. It does not carry a
Workspace owner, accepted permissions, Card, endpoint, or caller-supplied policy
decision.

## Value Object: Resolved Installation

The internal success response contains only the Installation authorization
facts needed by the Router:

| Field | Rules |
| --- | --- |
| `installationId` | Exact current Installation ID |
| `workspaceId` | Exact requested Workspace ID |
| `agentId` | Exact requested Agent ID |
| `installedVersion` | Exact requested and pinned Card version |
| `acceptedPermissions` | Immutable sorted snapshot |
| `status` | Constant `enabled` |

The response pairs this value with the exact currently published Agent Card
0.2. No success response exists for disabled, uninstalled, capability-denied,
Catalog-disabled, or dependency-failed state.

## Capability Authorization

For one exact Card and accepted snapshot:

1. Find the skill whose `id` exactly equals the requested capability.
2. If no skill exists, return `CAPABILITY_NOT_ALLOWED`.
3. For every exact `requiredPermissions` ID, require membership in the accepted
   snapshot.
4. If any ID is absent, return `CAPABILITY_NOT_ALLOWED`.
5. Only then return the Card and Resolved Installation.

No partial capability result, permission suggestion, newest-Card lookup, or
case-insensitive match is returned.

## Transaction Boundaries

### Create Workspace

One Workspace-owned transaction:

1. validate authenticated creator and input;
2. insert Workspace with owner and one server timestamp;
3. commit.

Expected primary-key races map to conflict. Other storage errors remain
dependency failures.

### Install Agent

Two ownership-preserving boundaries:

1. authenticate the Northbound caller and validate request/path semantics;
2. read the Workspace and establish existence;
3. authorize the immutable owner;
4. check for a current Installation conflict;
5. call the controlled Catalog operation to validate the constraint and return
   the exact selected currently published Card. Its successful result is the
   version-choice linearization point;
6. validate the permission subset against that exact Card;
7. start one Workspace-owned transaction, lock the Workspace, recheck
   existence/ownership/current uniqueness, insert the enabled Installation, and
   commit.

Any Workspace failure rolls back only Workspace writes. The transaction does
not write or lock Catalog rows. A partial unique violation from a concurrent
same-Agent install maps to conflict only when it matches the expected index.
The Catalog call occurs before the insert transaction begins, so a Workspace
transaction is never held open across a Catalog dependency call.

### Lifecycle Mutation

One Workspace-owned transaction:

1. lock the Workspace and authorize the owner;
2. lock the Installation under that Workspace;
3. evaluate the exact current state and requested transition;
4. update state and timestamps together;
5. commit.

The row lock prevents skipped or competing transitions. Uninstall releases the
partial unique slot in the same commit.

### Read and List

Workspace existence, owner authorization, and row query occur through the
Workspace service and store. A storage failure is returned explicitly. The
service never returns process-memory rows or an empty list after a failed query.

### Exact Internal Resolution

Resolution is read-only across two controlled owners:

1. authenticate the internal principal and parse the request. Before strict
   correlation validation, return only the pre-correlation error shape with a
   generated safe trace ID; never echo malformed or missing IDs;
2. validate request/correlation. Once correlation is valid, every later error
   repeats the exact request IDs;
3. read the current non-uninstalled Installation matching Workspace and Agent;
4. require its exact version and enabled state;
5. read the exact Catalog version through the controlled port;
6. require current `published` state and authorize the capability;
7. return the exact Card and minimal Installation fact.

Errors stop the sequence and expose no Card. No retry or alternate source is
used.

## Indexes

- Workspace primary key on `workspace_id`.
- Installation primary key on `installation_id`.
- Installation partial unique index on `(workspace_id, agent_id)` for
  `status <> 'uninstalled'`.
- Installation list index on
  `(workspace_id, installed_at ASC, installation_id ASC)`.
- Installation current resolution index on
  `(workspace_id, agent_id, installed_version)` for
  `status <> 'uninstalled'`.

## Retention and Future Evolution

- Phase 1 has no Workspace delete or Installation purge operation.
- Membership/RBAC may add separate policy-owned data and replace the
  authorization adapter; it must not reinterpret historical `owner_id` or
  permission snapshots.
- An explicit upgrade feature may create a new version-selection policy in a
  later Spec. It must not silently mutate existing pins.
- Deployment bindings, Kubernetes resources, Policy Hooks, quotas, approvals,
  and credentials require separate owning modules and are not nullable columns
  reserved in these tables.
