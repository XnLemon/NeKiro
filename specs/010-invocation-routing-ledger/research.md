# Research: Invocation Routing and Ledger

## Decision 1: Treat Workspace Closure as a Hard Runtime Blocker

**Decision**: Parent Issue #2 is closed after PR #18 merged with green real
PostgreSQL evidence and a fresh independent closure review. T001 may proceed.

**Rationale**: The readiness defect is corrected in merged commit `5f94565`,
and the repository's Workspace completion gate is now formally satisfied.

**Alternatives considered**:

- Treat the merged PR as automatic parent closure: rejected; the team instead
  completed the independent review and explicit Issue #2 evidence update.
- Copy Workspace behavior into Dispatch: rejected because Workspace owns the
  facts and policy.

## Decision 2: Use a Parent Roadmap with Child SDD Slices

**Decision**: Spec 010 is the macro delivery source. Each child issue creates a
new independent Spec rather than sharing one mutable implementation task file.

**Rationale**: Dispatch, Router, Ledger, transport, SDK, samples, and E2E have
different owners and review surfaces. The parent graph preserves dependency and
parallelism without allowing simultaneous edits to shared contracts.

**Alternatives considered**:

- One implementation issue: rejected because it removes independent review and
  creates broad shared-write conflicts.
- One microservice per module: rejected because only Router is a required
  separate process; Dispatch remains inside Control Plane.

## Decision 3: Freeze Missing Policies Before Runtime

**Decision**: T001 owns four blocking design gaps: SDK-facing Router API,
credential binding, post-side-effect Ledger failure visibility, and explicit
deadline/cancellation behavior. It also decides whether active contract
versions can express the result or require version increments.

**Rationale**: Existing contracts describe Control-Plane-to-Router dispatch and
Agent Card auth metadata, but not an Agent caller entry point or secret locator.
Guessing would create an undocumented trust or fallback path.

**Alternatives considered**:

- Let SDK call the Control Plane internal API: rejected because that API is
  Router-to-Control-Plane exact resolution, not Agent dispatch.
- Reuse Router Internal v2 without changing its caller/auth model: rejected
  until the contract gate proves it is valid.
- Treat a missing credential as anonymous: rejected as a secret/identity
  fallback.

## Decision 4: Authorize Twice at Different Trust Boundaries

**Decision**: Dispatch performs owner/pre-dispatch authorization through a
narrow Workspace port and obtains the exact pin. Router then re-resolves that
exact pin through Control Plane Internal v2 immediately before transport.

**Rationale**: Dispatch rejects user-visible policy failures before transport;
Router protects the data-plane boundary against state changes and never trusts
caller-provided Workspace/Card facts.

**Alternatives considered**:

- Dispatch only: rejected because Router must not trust a stale or forged Card.
- Router only: rejected because Gateway/Dispatch owns northbound authorization
  and should not send known-invalid work into the data plane.
- Router reads Workspace tables: rejected by ownership rules.

## Decision 5: Keep Results Live and Ledger Metadata-Only

**Decision**: Reuse same-request JSON/SSE delivery. Persist event metadata and a
derived projection only. No result replay, polling, reconnect cursor, or output
field is added.

**Rationale**: ADR 0002 already separates result sensitivity/retention from
audit facts and prevents the Ledger from becoming a result store.

**Alternatives considered**:

- Persist output for retries/replay: rejected by the active contract and secret
  boundary.
- Buffer SSE in Gateway: rejected because it defeats streaming.

## Decision 6: Use an Event Store plus Transactional Projection

**Decision**: Router/Ledger owns immutable `invocation_events` and a mutable
`invocations` projection. Each append validates the next legal sequence and
updates the projection in the same PostgreSQL transaction.

**Rationale**: Event history remains the fact source while query endpoints can
serve bounded metadata without reconstructing every projection in memory.

**Alternatives considered**:

- Projection-only row: rejected because it cannot prove lifecycle history.
- Event-only reads for every query: feasible but unnecessarily complicates the
  Phase 1 read path and trace queries.
- Queue/event broker: rejected as premature infrastructure.

## Decision 7: Fail Closed Around Ledger Writes

**Decision**: Append `created` before Agent interaction and commit a terminal
fact before reporting clean terminal success. An append failure is explicit;
there is no retry, alternate store, or fabricated successful projection.
T001 must freeze how a post-side-effect append failure is exposed.

**Rationale**: This minimizes unrecorded side effects and honors the zero
fallback budget without pretending a distributed write/Agent call is atomic.

**Alternatives considered**:

- Retry writes automatically: rejected without an approved retry policy.
- Return success when the terminal append fails: rejected as false success.
- Add a message broker/reconciler now: rejected as out of Phase 1 scope.

## Decision 8: Use the Pinned A2A Profile as the Transport Engine

**Decision**: Router uses `a2a-go v0.3.15` for JSON-RPC/A2A operations and
executes the existing conformance corpus. It maps supported Message/Task/event
kinds and rejects unsupported task states explicitly.

**Rationale**: A proven protocol library and executable profile avoid a second
handwritten A2A interpretation.

**Alternatives considered**:

- Raw JSON-RPC implementation: rejected because the established engine already
  exists and is pinned.
- Full Agent framework in Router: rejected by runtime independence.

## Decision 9: Keep the Agent SDK Thin

**Decision**: SDK handles Card/profile conformance helpers, trusted context
parsing, and nested Router invocation. It contains no model, tool, workflow,
memory, planner, session, or retry behavior.

**Rationale**: This is the minimum cross-Runtime integration value required by
the charter and keeps execution inside external Runtimes.

**Alternatives considered**:

- General Agent framework: rejected by ADR 0003.
- Direct nested Agent URL calls: rejected because they bypass authorization and
  lineage.

## Decision 10: Prove Two Runtime Implementations, Not Two Copies

**Decision**: One sample is a deterministic direct A2A implementation using the
pinned protocol library. The second is an isolated Runtime adapter selected and
pinned in its child Spec. They share contracts/context only and complete one
nested call.

**Rationale**: The proof must survive different Runtime internals, while the
exact second framework dependency should not be added until its child research
can validate a current version and API.

**Alternatives considered**:

- Two handlers using the same internal runtime package: rejected because it
  does not prove runtime independence.
- Put a full Runtime dependency in Router/SDK: rejected by ownership rules.

## Decision 11: Fallback Inventory

| Candidate | Classification | Evidence / action |
| --- | --- | --- |
| Default Router/Control Plane URL | Remove | Required destinations fail config validation |
| Anonymous credential when binding missing | Remove | Identity/secret fallback prohibited |
| Direct Agent route when Router fails | Remove | Every managed call traverses Router |
| Stale Card/cache when resolution fails | Remove | Registry/Workspace facts remain authoritative |
| Automatic Ledger retry/alternate store | Needs policy | No current retry/SLO policy; not planned |
| Result persistence/replay after disconnect | Remove | ADR 0002 rejects it |
| Historical API dual runtime | Remove | Active-only runtime policy |

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
