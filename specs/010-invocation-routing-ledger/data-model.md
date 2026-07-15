# Data Model: Invocation Routing and Ledger

## Ownership

Router/Ledger owns all new persistent data. Control Plane, Catalog, and
Workspace do not write these tables. Existing Agent Card and Installation
facts are resolved through their owning interfaces and copied only as immutable
identity metadata on Invocation events.

## Invocation Context

| Field | Rule |
| --- | --- |
| `invocation_id` | Unique safe identifier for this root or child call |
| `root_task_id` | Shared by every Invocation in one managed task tree |
| `parent_invocation_id` | Null for root; exact calling Invocation for child |
| `trace_id` | Shared trace correlation; exact case-sensitive propagation |
| `caller_type`, `caller_id` | Trusted user/service/agent identity from the owning boundary |
| `workspace_id` | Workspace authorization and query-isolation boundary |
| `target_agent_id` | Exact Agent identity |
| `agent_card_version` | Exact installed/resolved version, never a range |
| `capability` | Requested exact capability identifier |

`input` and `stream` are dispatch/result-transport fields, not Ledger entity
fields. Input must never enter either persistent table.

## Proposed Router-Owned Tables

### `ledger.invocation_events`

Append-only source of Invocation lifecycle truth.

| Column | Constraint |
| --- | --- |
| `event_id` | Primary key; server-generated safe ID |
| `invocation_id` | Required; groups one lifecycle |
| `sequence` | Required non-negative; unique with `invocation_id` |
| `occurred_at` | Required UTC timestamp with microsecond precision |
| `type` | `created`, `routing`, `started`, `stream`, `succeeded`, `failed`, `canceled`, or `timed_out` |
| `status` | Contract-compatible status for `type` |
| context fields | Required exact values from Invocation Context on every row |
| `chunk_index`, `chunk_bytes` | Present only for `stream`; no chunk value |
| `latency_ms` | Present only on terminal rows; non-negative |
| `error_code`, `error_message` | Fixed safe Platform Error facts only on failed/canceled/timed-out rows |

Rules:

- No UPDATE or DELETE path exists in the application store.
- `(invocation_id, sequence)` and `event_id` are unique.
- Sequence begins at 0 and advances by exactly one in the append transaction.
- Stable context fields must equal sequence 0 for every later event.
- The first terminal event forbids later events.
- Result-stream event sequence and Ledger event sequence are separate ordered
  domains; neither is inferred from the other.

### `ledger.invocations`

Derived mutable read projection maintained in the same transaction as each
event append.

| Column | Constraint |
| --- | --- |
| Invocation Context fields | Primary identity and immutable routing/lineage metadata |
| `status` | Current status derived from the last committed event |
| `latency_ms` | Null before terminal; terminal value thereafter |
| `error_code` | Null except failed/canceled/timed-out |
| `created_at` | Time of sequence 0 |
| `updated_at` | Time of latest committed event; never decreases |

Indexes:

- Primary key on `invocation_id`.
- Trace ordering on `(workspace_id, trace_id, created_at, invocation_id)`.
- Root-task ordering on `(workspace_id, root_task_id, created_at, invocation_id)`.
- Parent lookup on `(workspace_id, parent_invocation_id, created_at, invocation_id)`.

The exact physical DDL is frozen by T001/T004. No JSON payload column is
planned because it would allow content persistence outside the contract.

## State Machine

```text
pending -> routing -> running -> succeeded
   |          |          |-----> failed
   |          |          |-----> canceled
   |          |          `-----> timed_out
   |          |----------------> failed / canceled / timed_out
   `---------------------------> canceled / timed_out
```

- `created` produces `pending` at sequence 0.
- `routing` and `started` occur at most once and in order.
- Route, credential, or exact-resolution failure may terminalize from
  `routing`; cancellation or timeout may terminalize from `pending`, `routing`,
  or `running`. Success is allowed only from `running`.
- Zero or more `stream` facts may occur only while running; chunk index begins
  at 0 and advances without gaps for observed chunks.
- Exactly one terminal fact may commit on a clean completed lifecycle, and the
  first committed terminal forbids later events.
- A dependency interruption may leave a non-terminal history; it must not be
  projected or returned as success. T001 freezes its external visibility.

## Trace Invariants

- A root Invocation has no parent and `invocation_id` differs from
  `root_task_id` unless the contract explicitly permits equality; no equality
  is synthesized.
- A child has a new `invocation_id`, the same `root_task_id` and `trace_id`, and
  an existing parent Invocation in the same Workspace.
- Caller identity for a nested call is the trusted calling Agent identity, not
  a user value copied from Agent input.
- Trace reads are Workspace-authorized and deterministically ordered.

## Result Entities (Transient Only)

`InvocationResult` and `InvocationResultStreamEvent` exist in memory/on the
live HTTP response. They may contain arbitrary contract-valid JSON result or
chunk values. They are never serialized into Ledger tables, metadata events,
logs, or a recovery store.

## Validation and Failure Rules

- Identifiers are exact and case-sensitive; no trimming/normalization.
- Card version is strict SemVer and equals the current authorized Installation
  pin at both Dispatch and Router resolution points.
- Error codes, terminal type, and terminal status obey active schema semantics.
- Database configuration/schema failure is dependency failure, not not-found or
  empty success.
- Schema migration/readiness is explicit; serving never auto-migrates.

## Fallback Delta

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
