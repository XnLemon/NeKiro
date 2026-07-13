# Research: Catalog Registration and Discovery

## Decision 1: Deliver One Control Plane Catalog Slice

**Decision**: Implement the first runnable Control Plane slice around the five
active Catalog operations: register an immutable Card version, publish it,
disable it, read an exact version, and discover published versions. Gateway
HTTP adaptation, Registry, Card Store, and Discovery remain logical modules in
one deployable process.

**Rationale**: This closes `Register -> Discover` without introducing a service
split before scale or ownership requires it. It creates user-visible platform
value while preserving the long-term Control Plane boundary.

**Alternatives considered**:

- Implement all Control Plane domains together: rejected because Workspace,
  Dispatch, and Router integration have separate data and failure semantics.
- Implement only repository code: rejected because an internal storage layer
  alone does not demonstrate the user workflow.
- Start Frontend in parallel: rejected because Frontend remains explicitly
  paused and cannot precede a working Gateway boundary.

## Decision 2: Use PostgreSQL Through pgx With Explicit Migrations

**Decision**: Use PostgreSQL 17 as the durable Catalog store,
`github.com/jackc/pgx/v5 v5.10.0` for database access, and
`github.com/jackc/tern/v2 v2.4.1` for ordered SQL migrations. The server never
auto-creates or silently upgrades schema at request time. A separate migration
command applies embedded, versioned SQL before startup.

**Rationale**: PostgreSQL and `pgx/v5` are project constraints. `tern` is a
small pgx-native migration tool maintained in the same ecosystem and avoids
building an ad hoc migration history. Explicit migration keeps schema failure
visible at deployment boundaries.

**Alternatives considered**:

- In-memory or file persistence: rejected because restart durability and
  concurrent state transitions require a transactional source of truth.
- An ORM: rejected because the domain has a small number of explicit queries
  and an ORM would obscure ownership, locking, and cursor predicates.
- Runtime auto-migration: rejected because request-serving startup must not
  acquire schema ownership or hide failed deployment steps.
- Hand-written one-shot schema initialization: rejected because later features
  need a durable, ordered migration history.

## Decision 3: Store One Immutable Card Fact Plus Derived Index Rows

**Decision**: Store each validated Agent Card as PostgreSQL `jsonb` in the
Registry-owned version row, accompanied by its active Card Schema version and a
SHA-256 digest of the canonical mapped Card. Keep Agent ownership in a stable
Agent identity row. Store capability IDs in a transactionally maintained child
table for exact filtering. Do not duplicate the full Card into Discovery.

**Rationale**: `jsonb` preserves language-neutral Card data and supports exact
version reads without normalizing every embedded input/output schema into
business tables. Owner and capability rows enforce the cross-version and query
rules efficiently while remaining derived from the validated Card in the same
Registry transaction.

**Alternatives considered**:

- Normalize every Agent Card field: rejected because arbitrary JSON Schemas
  would become a second, lossy contract model.
- Store only raw text: rejected because queryable metadata and structural
  integrity would depend entirely on application scans.
- Put historical Cards in object storage now: rejected because the current
  scale has no cold-storage requirement and would add another dependency.
- Copy full Cards into a separate Discovery database: rejected because that
  creates a second mutable Card fact and a consistency problem without scale
  evidence.

## Decision 4: Keep Publication and Discovery Strongly Consistent

**Decision**: Registration writes Agent identity, immutable version, and
capability index rows in one transaction. Publication and disablement lock the
exact version row and commit one legal state transition. Discovery queries a
Registry-owned published-version read model over those same tables. A write is
not acknowledged until all owned facts commit.

**Rationale**: The first deployment does not need an asynchronous projection.
Transactional consistency proves immediate discoverability and disablement,
eliminates an unsupported stale-data fallback, and keeps failure semantics
simple.

**Alternatives considered**:

- Publish an event and update Discovery asynchronously: rejected because no
  queue, retry policy, lag SLO, or reconciliation ownership is yet justified.
- Return success and repair a failed index later: rejected because it violates
  the Spec's explicit no-false-success requirement.
- Read from a cache during database failure: rejected because no stale-read
  policy exists and authorization-sensitive eligibility must fail closed.

## Decision 5: Use Row Locks and Database Constraints for Races

**Decision**: Enforce `(agent_id, version)` uniqueness and ownership with
database constraints. Lifecycle commands use a transaction and lock the exact
version row before evaluating state. Registering the first Agent identity and a
version occurs atomically; concurrent owner claims resolve to one committed
owner and explicit conflict/forbidden outcomes.

**Rationale**: Process-local mutexes do not protect multiple instances and
cannot make state durable. Database serialization keeps behavior correct when
the Control Plane is later replicated.

**Alternatives considered**:

- Last-write-wins updates: rejected because Cards are immutable.
- Read-then-write without locking: rejected because publish/disable races can
  produce impossible or lost states.
- Global advisory lock: rejected because row and uniqueness constraints provide
  a narrower ownership boundary.

## Decision 6: Use a Versioned Stateless Discovery Cursor

**Decision**: Encode a strict versioned cursor as base64url JSON containing a
hash of normalized filters and page size, the first-page publication boundary,
and the last `(published_at, agent_id, version)` ordering tuple. Search uses
keyset predicates ordered by publication time descending, Agent ID ascending,
and exact version string ascending.

**Rationale**: Keyset pagination remains stable at the expected scale and does
not degrade with deep offsets. A filter hash prevents accidental cursor reuse
with a different search. The payload contains no secret or Card content.

The cursor is opaque but not an authorization credential. It is not signed in
Phase 1 because every query reapplies authentication, filters, publication
state, and database authorization; forging a boundary cannot reveal data the
caller could not query directly. Malformed or inconsistent values fail
validation.

**Alternatives considered**:

- Numeric offsets: rejected because concurrent publication can duplicate or
  skip rows and deep pages become progressively slower.
- Server-side cursor sessions: rejected because they add retention, cleanup,
  and affinity not required by the contract.
- HMAC signing: deferred because the cursor grants no authority and introducing
  a required signing secret has no current threat-model evidence.
- Silent cursor reset: rejected because it hides invalid client state.

## Decision 7: Keep Authentication Replaceable and Explicit

**Decision**: Gateway handlers consume an `Authenticator` interface that maps
an HTTP Bearer credential to one trusted caller ID. Integration tests inject a
deterministic authenticator. The runnable local binary may enable only an
explicit `development-static` adapter configured with a non-empty principal
list; absent, empty, malformed, or unsupported authentication configuration
fails startup. Raw caller-ID headers are never trusted.

**Rationale**: The Catalog must prove owner authorization now, while enterprise
OIDC and RBAC remain out of scope. An interface prevents the temporary local
adapter from becoming the platform identity model. Explicit mode selection and
required credentials avoid anonymous or default-user fallback.

**Alternatives considered**:

- Trust `X-Caller-ID` from the public request: rejected because any caller could
  claim an Agent owner.
- Skip authorization until OIDC exists: rejected because registration would
  create untrustworthy ownership facts.
- Implement OIDC in this feature: rejected because it expands scope beyond the
  Catalog slice and requires its own issuer, audience, rotation, and operations
  policies.
- Use a built-in default development token: rejected because secrets and caller
  identity must never have defaults.

## Decision 8: Amend Northbound v2 Additively Before Runtime Code

**Decision**: Keep Northbound API `v2` and Agent Card `0.2`. Before writing the
server, update the active Catalog operations to declare Bearer authentication,
the explicit `25` page default, cursor/filter behavior, and exact `400`, `401`,
`403`, `404`, `409`, and `503` outcomes that can occur. Existing success payload
shapes remain unchanged. Historical v1 remains untouched.

**Rationale**: These changes complete previously unspecified behavior without
changing a deployed consumer. The contract remains the language-neutral fact
and the Go handler maps to it rather than inventing runtime-only semantics.

**Alternatives considered**:

- Implement undocumented responses: rejected because generated clients and
  cross-language callers would not share the failure contract.
- Create Northbound v3: rejected because success shapes and operation meanings
  do not change and no v2 runtime consumer exists.
- Mutate v1: rejected because historical artifacts are migration evidence.

## Decision 9: Separate Unit, Contract, Integration, and Acceptance Tests

**Decision**:

- Unit tests cover state transitions, owner checks, filter normalization, cursor
  decoding, configuration, and fixed error mapping after implementation.
- Contract tests cover the amended OpenAPI-to-Go mappings and exact declared
  status/error sets.
- PostgreSQL integration tests use the real migration and repository behind an
  explicit `integration` build tag and required test database URL.
- HTTP acceptance tests start the real handler against PostgreSQL and exercise
  Register -> Publish -> Discover, disablement, restart durability, owner
  rejection, pagination, dependency failure, and two Runtime-neutral fixture
  Cards.
- CI provisions a PostgreSQL service and runs both default and integration test
  commands. It does not change pnpm `minimumReleaseAge` or lockfile policy.

**Rationale**: Fast default tests remain usable without Docker, while the
database and HTTP boundaries receive real acceptance evidence in CI and the
feature quickstart.

**Alternatives considered**:

- Mock all database behavior: rejected because SQL constraints, transactions,
  and cursor ordering are core acceptance risks.
- Require Docker for every `go test ./...`: rejected because it makes unrelated
  contract and unit work depend on local infrastructure.
- Testcontainers: rejected for the first slice because the repository already
  has a pinned Compose PostgreSQL service and CI can provide an explicit
  database dependency with less library surface.

## Decision 10: Defer Deployment Hot/Cold Layers

**Decision**: This feature stores online Catalog facts only. It does not deploy
Agents, create a serving registry, archive Cards to cold storage, or compose
dynamic deployment endpoints. Future deployment work must use exact immutable
Card versions and a separate Deployment Binding contract rather than rewriting
Registry history.

**Rationale**: The user explicitly deferred the hot/cold deployment direction
until after the second feature workflow is operational. Keeping it out prevents
Registry from becoming an Agent Runtime.

**Alternatives considered**:

- Deploy on publish: rejected because publication and runtime readiness are
  separate state machines and ownership domains.
- Store dynamic endpoints in historical Cards: rejected because Card versions
  are immutable contract facts.

## Fallback Inventory

| Candidate | Classification | Evidence and behavior |
|---|---|---|
| Default discovery page size 25 | Keep | FR-017 establishes explicit omitted-limit product policy; invalid explicit values fail |
| Empty discovery items | Keep | US3/AC6 and FR-019 define genuine no-match success |
| Idempotent disable | Keep | US2/AC4 and FR-009 define repeated-disable semantics |
| Default caller or anonymous mutation | Remove | FR-003 requires trusted identity; missing identity is unauthenticated |
| Stale Discovery on database failure | Remove | FR-010 and FR-019 require explicit dependency failure |
| Historical Card dual-read | Remove | FR-023 forbids runtime compatibility fallback |
| Default database/listen/auth configuration | Remove | Required runtime configuration fails startup when missing or invalid |
| Automatic schema creation on server startup | Remove | Migration is an explicit deployment command |

Fallback delta planned: removed `0`, retained `3`, added `0`, net `0`.
The retained behaviors are already explicit Spec policies, not newly invented
failure recovery paths. Added fallback evidence: none.
