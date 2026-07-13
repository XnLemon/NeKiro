# Feature Specification: Catalog Registration and Discovery

**Feature Branch**: `codex/002-catalog-registry-discovery`

**Created**: 2026-07-14

**Status**: Ready for Implementation

**Input**: Specify the next NeKiro backend capability after the contract
foundation: make Agent registration, version publication, exact version reads,
disablement, and capability discovery operational without introducing Agent
Runtime behavior.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Register an Immutable Agent Version (Priority: P1)

An Agent developer submits a conforming Agent Card for a new Agent version and
receives a durable draft Catalog entry. The developer can subsequently read the
same exact Card and publication state without the platform changing any Card
field.

**Why this priority**: Registration creates the Registry fact on which every
later publication, installation, resolution, and invocation depends.

**Independent Test**: Register one valid Agent Card, read the exact version,
restart the Catalog boundary, and verify that the same immutable draft remains
available while invalid and duplicate submissions create no Registry fact.

**Acceptance Scenarios**:

1. **Given** an authenticated developer whose identity matches the Card owner,
   **When** the developer submits a structurally and semantically valid active
   Agent Card version, **Then** the platform stores one immutable draft version
   and returns its server-assigned registration time.
2. **Given** a Card that violates the active structural or semantic contract,
   **When** registration is attempted, **Then** the request is rejected with a
   fixed validation failure and no Agent or version is created.
3. **Given** an existing Agent ID and version, **When** any caller submits that
   identity again with identical or different Card content, **Then** the request
   conflicts and the original Card remains unchanged. Exact-version conflict
   takes precedence over owner mismatch; the response reveals no Card, owner,
   or publication metadata.
4. **Given** an existing Agent owned by one developer, **When** that owner
   registers a different valid version, **Then** the new immutable draft is
   accepted; a different owner cannot register a version under that Agent ID.

---

### User Story 2 - Publish and Disable a Version (Priority: P1)

The owning developer publishes a reviewed draft so Workspace users can discover
it, and can later disable the exact version so it is no longer eligible for new
discovery or resolution. Historical identity and Card content remain readable.

**Why this priority**: Discovery must be driven by an explicit publication
decision rather than exposing drafts or treating every registered Card as
usable.

**Independent Test**: Seed a valid draft, publish it, verify immediate discovery
eligibility, disable it, and verify immediate exclusion while its exact
historical Catalog entry remains readable.

**Acceptance Scenarios**:

1. **Given** an owned draft version, **When** its owner publishes it, **Then**
   the state becomes published exactly once, the first publication time is
   recorded, and the immutable Card is unchanged.
2. **Given** a successful publication response, **When** discovery is queried
   immediately, **Then** the published exact version is eligible without an
   unreported projection delay.
3. **Given** a published or disabled version, **When** publication is requested,
   **Then** the request conflicts and neither state nor timestamps are rewritten.
4. **Given** an owned draft or published version, **When** its owner disables
   it, **Then** the state becomes disabled and it is excluded from new discovery
   and resolution; disabling an already disabled version is idempotent.
5. **Given** a disabled version, **When** its exact Catalog entry is read by an
   authorized caller, **Then** the historical Card and disabled state remain
   available and the version cannot be republished.

---

### User Story 3 - Discover Published Agent Versions (Priority: P1)

A Workspace user searches the Catalog for published Agent versions by free
text, capability, or owner and can inspect an exact published Card before a
later feature installs it.

**Why this priority**: Registration has platform value only when users can find
eligible Agents through the common Card language rather than knowing endpoints
or Runtime implementations in advance.

**Independent Test**: Seed draft, published, and disabled versions across
multiple owners and capabilities; verify filtering, ordering, pagination,
visibility, and exact reads using only Catalog operations.

**Acceptance Scenarios**:

1. **Given** mixed publication states, **When** a user performs an unfiltered
   discovery query, **Then** every returned item is one exact published version
   and no draft or disabled version is present.
2. **Given** published versions with different names, descriptions,
   capabilities, and owners, **When** filters are supplied, **Then** free text
   matches name or description case-insensitively, capability and owner use
   exact identifier equality, and all supplied filters are combined with AND.
3. **Given** more matching versions than one page, **When** the user follows the
   opaque continuation cursor with the same filters, **Then** ordering is stable
   and an unchanged Catalog yields every match exactly once.
4. **Given** a continuation traversal, **When** a new version is published after
   the first page, **Then** it is not inserted into that traversal; a version
   disabled before a later page is returned is excluded without duplicating
   another result.
5. **Given** an exact published Agent ID and version, **When** a user reads it,
   **Then** the corresponding Card and publication metadata are returned;
   unknown identities return not found rather than an empty object.
6. **Given** no matching published version, **When** discovery completes,
   **Then** it returns an explicit empty item list; a malformed, filter-mismatched,
   or otherwise invalid cursor returns a validation failure rather than silently
   restarting from the first page.

---

### User Story 4 - Preserve Catalog Trust During Failure (Priority: P2)

A platform operator can distinguish invalid input, missing identity, forbidden
ownership, state conflicts, not-found versions, and dependency failures. A
failure never becomes a false registration, publication, or empty discovery
success.

**Why this priority**: The Registry is the source of truth. Ambiguous success or
fallback data would make later installation and routing decisions unsafe.

**Independent Test**: Inject each owned-boundary failure before and during
registration, lifecycle transition, exact read, and discovery; verify fixed
public outcomes, durable state, and secret-safe diagnostics.

**Acceptance Scenarios**:

1. **Given** no trusted caller identity, **When** a protected Catalog operation
   is attempted, **Then** it fails as unauthenticated without using a default
   developer or owner.
2. **Given** a caller who does not own the Agent, **When** registration under an
   existing identity, publication, disablement, or a non-public exact read is
   attempted, **Then** it fails as forbidden without revealing mutable internals.
3. **Given** the Registry or its discovery projection cannot complete an
   operation, **When** the request is handled, **Then** the caller receives an
   explicit dependency failure and no empty, stale, or successful fallback.
4. **Given** a process restart after a successful state transition, **When** the
   Catalog is queried again, **Then** all immutable versions, owners, states,
   and original timestamps retain their committed values.

### Edge Cases

- Two callers concurrently register the same Agent ID and version with equal or
  different payloads.
- A caller attempts to claim an existing Agent ID by changing the Card owner in
  a later version.
- Publication and disablement race for the same draft; only a legal serialized
  state may commit, and a committed disablement cannot remain discoverable.
- The Card is valid JSON but has duplicate members, unknown fields, duplicate
  skill or permission IDs, undeclared permissions, credential-bearing endpoint
  userinfo, or another active conformance failure.
- A registration body exceeds 16,777,216 bytes or is not fully received within
  30 seconds measured from the start of registration body processing; the
  Gateway must stop reading it and must not persist a partial Card. Header read
  time is governed by a separate server deadline and does not consume the body
  window.
- A valid Card uses a positive JSON integer beyond machine `int64` range for
  `maxInputBytes` or `maxOutputBytes`; the Go mapping must preserve the exact
  number rather than reject or round it. Values beyond PostgreSQL `numeric` /
  `jsonb` range, such as `1e131072`, remain valid and must survive registration,
  persistence, restart, exact reads, and Discovery without database numeric
  coercion. Exponents beyond a validation library's materialization range, such
  as `1e1000001`, remain valid and must not be rejected by implementation limits.
- A publication state is committed but the derived discovery update fails
  before the operation can report success.
- Search text is blank or over its declared length, a page size is outside its
  bounds, or a cursor is malformed or reused with different filters.
- Several published versions share the same publication time or display name;
  deterministic tie-breakers must keep traversal stable.
- A matching version is disabled between discovery pages, or a new matching
  version is published after traversal begins.
- A dependency becomes unavailable during an exact read or empty-result search;
  the failure must not be represented as not found or an empty list.
- An operator requests an unsupported reverse migration against a populated
  Catalog; the command must fail without dropping or rewriting Catalog data.
- Logs or fixed public errors accidentally include the Card body, endpoint
  credentials, input/output schemas, or internal dependency details.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The platform MUST accept registrations only for the active Agent
  Card contract and MUST apply both its structural and semantic validation.
  Active unbounded JSON integer fields MUST retain their exact number values
  even when they exceed a machine integer or PostgreSQL `numeric` / `jsonb`
  range. The durable Card fact MUST use a representation that does not parse or
  coerce those number tokens at the database boundary. Validation MUST determine
  JSON number syntax, mathematical integrality, and minimum `1` without
  materializing an unbounded value or inheriting a library exponent limit.
- **FR-002**: A successful registration MUST create one durable immutable draft
  identified by the exact `(agent_id, version)` pair and MUST assign its
  registration time at the platform boundary.
- **FR-003**: Registration, publication, and disablement MUST require a trusted
  authenticated caller whose identity exactly matches the Agent owner; missing
  identity and ownership mismatch MUST remain distinct failures.
- **FR-004**: The first successfully registered version of an Agent ID MUST
  establish its immutable owner, and every later version under that Agent ID
  MUST declare the same owner.
- **FR-005**: Re-registering an existing `(agent_id, version)` MUST conflict and
  MUST NOT overwrite, merge, or compare-and-replace the stored Card, even when
  the submitted content is byte-for-byte identical. This exact-version conflict
  MUST take precedence over stable-owner mismatch; a different version under an
  existing Agent ID still uses the distinct forbidden ownership failure.
- **FR-006**: Agent Card content MUST remain immutable after registration.
  Publication metadata MUST be stored separately and MUST NOT alter the Card.
- **FR-007**: Publication state MUST follow `draft -> published -> disabled`,
  with the additional terminal transition `draft -> disabled`; no disabled
  version may return to draft or published.
- **FR-008**: Publishing MUST succeed only for a draft, record the first
  publication time exactly once, and report a conflict for published or disabled
  versions.
- **FR-009**: Disablement MUST be idempotent, MUST preserve the Card and original
  timestamps, and MUST make a draft or published version disabled for every new
  discovery and resolution decision.
- **FR-010**: A publication or disablement operation MUST report success only
  after the Registry fact and its discovery eligibility are mutually
  consistent; a projection failure MUST be explicit and retryable without a
  false success response.
- **FR-011**: Discovery MUST derive from Registry-owned version and publication
  facts and MUST NOT become a second mutable source of Agent Card truth.
- **FR-012**: Discovery MUST return one item per exact published version and
  MUST exclude every draft and disabled version.
- **FR-013**: Free-text discovery MUST perform a case-insensitive substring
  match over Card name and description. Capability and owner filters MUST use
  exact identifier equality, and multiple supplied filters MUST use AND.
- **FR-014**: Discovery results MUST order publication time descending, then
  Agent ID ascending, then the exact version string ascending. These exact keys
  MUST also define cursor continuation when values are tied; this ordering MUST
  NOT be presented as version recommendation or compatibility ranking.
- **FR-015**: Discovery MUST support an opaque continuation cursor bound to the
  original filters and traversal boundary. It MUST reject malformed or
  filter-mismatched cursors and MUST NOT silently restart a traversal.
- **FR-016**: A traversal MUST exclude versions published after its first page,
  exclude versions disabled before a later page is returned, and avoid duplicate
  results. With no concurrent Catalog changes, it MUST return every match exactly
  once.
- **FR-017**: A discovery request that omits its page size MUST return at most 25
  entries. An explicit page size MUST remain between 1 and 100 inclusive; an
  invalid value MUST fail validation rather than being clamped or defaulted.
- **FR-018**: An exact version read MUST return the immutable Card, publication
  state, registration time, and first publication time when present. Published
  versions are readable by authenticated platform users; draft and disabled
  versions require the owning developer.
- **FR-019**: A genuine no-match discovery result MUST return an explicit empty
  item list. Not found, invalid input, unauthenticated, forbidden, conflict, and
  dependency failure MUST remain distinct and MUST NOT collapse into empty or
  successful responses.
- **FR-020**: All public failures MUST use fixed, versioned, secret-safe error
  semantics and MUST NOT contain Card bodies, endpoint credentials, schema
  content, stack traces, or dependency details. Registration bodies MUST be
  limited to 16,777,216 bytes and 30 seconds of request-body read time measured
  from the start of registration body processing, independently of the header
  deadline; an oversized fully handled request uses the existing
  `400 VALIDATION_ERROR` response and no partial Card is persisted.
- **FR-021**: Successful Catalog writes MUST survive process restart without
  changing Card content, owner, state, or previously assigned timestamps. The
  public migration command MUST apply forward migrations only; unsupported
  directions, including `down`, MUST fail before changing or deleting Catalog
  data.
- **FR-022**: Concurrent registration and lifecycle operations MUST be atomic:
  at most one conflicting write succeeds and no intermediate or impossible
  publication state becomes observable.
- **FR-023**: Historical Agent Card and Northbound contract versions MUST remain
  migration evidence only. This feature MUST NOT add runtime dual-read,
  auto-upgrade, or compatibility fallback for them.
- **FR-024**: Catalog behavior MUST remain independent of Agent endpoint health,
  implementation language, model, or Runtime framework. Registration or
  publication MUST NOT invoke the Agent or inspect Runtime internals.
- **FR-025**: Every requirement and acceptance scenario in this feature MUST map
  to a later contract, implementation, post-implementation test, and independent
  Review task before the feature can be marked complete.

### Key Entities

- **Agent Identity**: The stable Agent ID and immutable owning developer identity
  shared by all versions of one Agent.
- **Agent Version**: One immutable active-version Agent Card identified by Agent
  ID and semantic version, with a server-assigned registration time.
- **Publication State**: Draft, published, or disabled metadata owned by the
  Registry, including the first publication time when publication occurred.
- **Discovery Projection**: A query-oriented view derived from published
  Registry facts; it does not own or rewrite Agent Card content.
- **Discovery Cursor**: An opaque continuation token bound to one filter set,
  deterministic ordering position, and publication boundary.
- **Caller Identity**: Trusted Gateway-provided identity used to distinguish
  authenticated platform users and Agent owners without defaulting missing
  identity.

### Runtime/Platform Boundary

- **Platform-owned behavior**: Agent identity ownership, Card validation,
  immutable version registration, publication state, exact version reads,
  capability discovery, authorization decisions, persistence, and fixed public
  failures belong to NeKiro's Control Plane.
- **Runtime-owned behavior**: Endpoint serving, model calls, prompts, tools,
  workflows, memory, sessions, capability execution, health, and runtime
  telemetry remain entirely inside external Agent Runtimes.
- **Cross-runtime proof**: Catalog acceptance uses conforming Cards from at least
  two independently implemented fixture Agents and produces identical
  registration and discovery decisions without starting or importing either
  Runtime. Live cross-Runtime invocation remains a later feature.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An authenticated developer can register, publish, and find a valid
  Agent version through the platform Catalog in under two minutes without using
  the Agent endpoint directly.
- **SC-002**: 100% of published valid fixture versions become discoverable when
  publication reports success, and 0% of draft or disabled fixture versions
  appear in discovery.
- **SC-003**: 100% of invalid Cards, duplicate exact versions, cross-owner Agent
  versions, and illegal lifecycle transitions are rejected without changing an
  existing Registry fact.
- **SC-004**: 100% of protected mutation cases reject missing identity and
  ownership mismatch with distinct outcomes and no inferred caller fallback.
- **SC-005**: For a fixed Catalog of at least 1,000 matching versions, following
  every continuation cursor returns every match exactly once in deterministic
  order with no duplicates.
- **SC-006**: 100% of free-text, capability, owner, and combined-filter
  acceptance cases return exactly the published versions that satisfy all
  declared filters.
- **SC-007**: After a process restart, 100% of committed fixture versions retain
  identical Card content, owner, publication state, and original timestamps.
- **SC-008**: 100% of injected Registry or discovery dependency failures produce
  an explicit dependency failure rather than success, not found, stale data, or
  an empty result.
- **SC-009**: No acceptance log, fixed public error, or discovery response
  contains endpoint credentials, complete Card request bodies, internal
  dependency details, or Agent input/output data.
- **SC-010**: In an acceptance Catalog containing 10,000 published versions,
  at least 95% of first-page discovery requests complete within one second.
- **SC-011**: The same Catalog workflow accepts conforming Cards from two
  different Runtime implementations without loading, starting, or depending on
  either Runtime framework.

## Assumptions

- The active Agent Card `0.2`, Platform Error `v2`, and Northbound API `v2`
  remain the language-neutral contract baseline. Previously unspecified Catalog
  error and pagination behavior may be clarified before implementation without
  adding support for historical runtime contracts.
- The Gateway supplies a verified caller identity to Catalog operations. The
  identity provider and credential exchange mechanism are separate concerns;
  missing or unverified identity is never accepted or defaulted.
- Agent Cards contain metadata and schemas, not credential values. The platform
  does not log complete registration payloads as an operational fallback.
- Publication is a metadata eligibility decision, not evidence that an Agent
  endpoint is deployed, reachable, healthy, or semantically correct.
- No deployed Catalog data or external consumer currently requires a migration
  or dual-version compatibility path.
- The first implementation may keep Registry and Discovery in one Control Plane
  deployment while preserving their ownership boundary.
- The default discovery page size is 25 entries. This is an explicit product
  policy for omitted limits, not an error fallback; invalid explicit limits are
  rejected.

## Non-Goals

- Workspace Installation, permission acceptance, version-constraint resolution,
  Invocation Dispatch, A2A routing, result transport, Ledger, or trace queries.
- Frontend Console pages or direct Frontend access to Catalog storage.
- Agent deployment, endpoint health checks, certification, benchmarking,
  recommendation ranking, ratings, billing, or Marketplace moderation.
- Full enterprise OIDC, organization membership, RBAC, approval workflows, or
  anonymous public Catalog access; this feature consumes only a trusted caller
  identity at the Gateway boundary.
- Mutable drafts, Card replacement, version deletion, republishing disabled
  versions, or editing historical publication timestamps.
- Agent model, prompt, tool, workflow, memory, RAG, session, evaluation, or
  Runtime telemetry behavior.
- A dedicated search cluster, message queue, event-streaming platform, or
  premature service split for Registry and Discovery.
- Runtime support for Agent Card `0.1` or Northbound `v1`.
