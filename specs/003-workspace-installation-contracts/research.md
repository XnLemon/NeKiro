# Research: Minimal Workspace and Installation

## Decision 1: Workspace Is an Infrastructure-Independent Authorization Root

**Decision**: Model the Phase 1 Workspace as exactly one caller-selected safe
identifier, one immutable trusted creator-owner, and server timestamps. Keep
owner authorization behind a narrow `Authorizer` port whose initial policy is
exact caller/owner equality.

**Rationale**: This is the minimum durable boundary needed for permission
acceptance and later audit. The policy port permits Membership/RBAC to replace
the initial decision without changing Installation facts or importing a generic
policy engine now.

**Alternatives considered**:

- Kubernetes Namespace identity: rejected because deployment infrastructure is
  outside Workspace ownership and Phase 1 scope.
- Membership or role tables: rejected because one creator-owner is approved and
  no multi-user policy is required yet.
- Hard-code owner equality inside every handler: rejected because it couples
  transport to authorization and makes the approved replacement boundary fake.

## Decision 2: Use the Existing Strict SemVer Contract and Pinned Evaluator

**Decision**: Validate exact Agent versions with the active strict SemVer
primitive and parse constraints with `Masterminds/semver/v3 v3.5.0`, already
pinned in the repository. Do not set `IncludePrerelease`; a constraint branch
must explicitly contain a pre-release comparator for pre-release candidates to
participate. Do not trim or coerce submitted constraints; the active SemVer
parser is the sole range-boundary authority.

**Rationale**: The package already backs the repository's `semver-range` format,
has explicit pre-release branch behavior, caps constraint group complexity, and
keeps implementation and contract validation aligned. Strict Card versions
avoid the package's loose-version input forms.

**Alternatives considered**:

- Accept every non-whitespace string and parse during selection: rejected
  because malformed constraints would cross the Gateway boundary.
- Force all pre-releases into every range: rejected because it violates the
  approved explicit pre-release policy and common package semantics.
- Hand-write a reduced range parser: rejected because a proven pinned parser
  exists and custom grammar would create a second contract.

## Decision 3: Resolve Equal SemVer Precedence Deterministically

**Decision**: Sort matching published versions by SemVer precedence descending,
then by original exact version string descending in bytewise order. The second
key only distinguishes versions equal in precedence because SemVer ignores build
metadata.

**Rationale**: Catalog permits exact versions that differ only by build
metadata. SemVer intentionally gives them equal precedence, so the install
operation needs a deterministic final key to produce one exact pin under all
database query plans and process runs. Exact byte order requires no mutable
timestamp or inferred preference.

**Alternatives considered**:

- Prefer latest publication timestamp: rejected because publication time is not
  version precedence and would make republishing order a product selector.
- Reject equal-precedence candidate sets: rejected because the owner submitted a
  valid constraint and all candidates are valid published versions.
- Ignore the tie and accept database order: rejected as nondeterministic.

## Decision 4: Catalog Selection Is a Controlled Read, Not a Shared Transaction

**Decision**: Workspace calls a Catalog-owned read port for published candidate
selection and exact Card reads. The successful selection response is the
version-choice linearization point. Workspace then commits its own Installation
transaction. A later Catalog disable remains visible during exact resolution and
does not rewrite the pin.

**Rationale**: A cross-domain SQL transaction or direct Catalog table query
would violate ownership. The approved product rule already defines historical
pins as immutable when Catalog state changes. A clear linearization point makes
the unavoidable concurrent-disable case testable without pretending the two
modules share ownership.

**Alternatives considered**:

- Join Catalog and Workspace tables in one transaction: rejected because
  Workspace would depend on Catalog storage and migration internals.
- Copy all published Cards into Workspace: rejected because Registry is the sole
  Card source of truth.
- Retry selection if Catalog changes: rejected because no retry policy exists
  and repeated reads could still race while hiding the original operation.

## Decision 5: Persist Permission Acceptance as a Canonical Snapshot

**Decision**: Validate accepted IDs against the exact selected Card, allow the
empty set, reject duplicates and unknown IDs, and store the set in ascending
bytewise order. Capability authorization is set containment over the exact
Card's required IDs.

**Rationale**: A snapshot proves exactly what the owner accepted and remains
stable when later Card versions change. Canonical ordering makes equality,
responses, tests, and auditing deterministic without changing identifier
semantics.

**Alternatives considered**:

- Require every declared permission: rejected because the approved design
  explicitly permits a subset.
- Re-read accepted permissions from the newest Card: rejected because it would
  silently expand or alter authorization.
- Preserve duplicate or input order: rejected because the value is a set and
  duplicates are invalid contract input.

## Decision 6: Preserve Uninstalled Rows with Partial Uniqueness

**Decision**: Store one Installation row per install event. Use states
`enabled`, `disabled`, and `uninstalled`, a terminal `uninstalled_at`, and a
partial unique constraint over `(workspace_id, agent_id)` where state is not
`uninstalled`.

**Rationale**: This enforces one current Installation under concurrency while
preserving history and permitting a later fresh install. Row locks serialize
lifecycle transitions; the partial uniqueness slot is released in the same
uninstall commit.

**Alternatives considered**:

- Hard-delete uninstall: rejected because accepted permissions and exact pins
  are audit facts.
- Reuse one row on reinstall: rejected because it rewrites history and erases
  the prior acceptance event.
- Process-local uniqueness checks: rejected because they fail across replicas
  and restarts.

## Decision 7: Separate Installation Disabled from Agent Version Disabled

**Decision**: Add `INSTALLATION_DISABLED` with fixed message
`The Agent installation is disabled.` to Platform Error v3. Preserve Platform
Error v2 for existing Catalog/Invocation consumers and preserve
`AGENT_DISABLED` exclusively for the pinned Catalog version's disabled state.

**Rationale**: Installation lifecycle and Registry publication lifecycle are
different owners and recovery actions. Reusing one code would violate explicit
failure semantics and misdirect the operator.

**Alternatives considered**:

- Use generic `FORBIDDEN` for disabled Installation: rejected because it
  collapses owner policy denial and a recoverable Installation state.
- Reuse `AGENT_DISABLED`: rejected because that message and owner refer to the
  Agent version, not the Workspace authorization fact.
- Return success with a disabled marker: rejected because exact resolution must
  not expose a Card when dispatch is unauthorized.

## Decision 8: Complete Existing Active Contracts Before First Runtime

**Decision**: Add Workspace v1 and version the underspecified active
Installation behavior as v2, the Control Plane Internal resolution contract as
v2, the Workspace/Installation Platform Error behavior as v3, and the complete
Northbound contract as v3. Northbound v2 remains byte-unchanged because its
existing install, lifecycle, authentication, error, and uninstall semantics
cannot be tightened in place. Record every migration impact in compatibility
documentation. Do not alter historical artifacts or add dual-version runtime
behavior.

**Rationale**: Constitution V requires a new contract version for breaking
field or semantic changes even before a runtime consumer exists. Northbound v3
is the sole active implementation target; v2 is migration evidence, not a
runtime compatibility fallback.

**Alternatives considered**:

- Complete Northbound v2 in place: rejected because adding mandatory
  authentication, replacing Installation v1 responses, changing exact errors,
  and changing uninstall semantics are breaking.
- Leave active OpenAPI vague and decide in handlers: rejected because concurrent
  implementations would create conflicting facts.
- Change historical contracts too: rejected because they are immutable
  migration evidence.

## Decision 9: Keep the Fallback Budget at Zero

**Decision**: Retain only the genuine empty list for an existing authorized
Workspace with no Installation records. Remove or reject every proposed owner,
constraint, Card source, lifecycle, retry, or degraded-result fallback.

**Rationale**: Dependency and policy states have exact public meanings. Empty,
not found, or success cannot preserve those meanings when a dependency failed.

**Alternatives considered**:

- Return cached Installations or Cards on PostgreSQL/Catalog failure: rejected
  because no stale-read product policy or SLO exists.
- Retry dependency calls: rejected because retry ownership, budgets, and
  idempotency policy are not approved.
- Treat same-state lifecycle requests as success: rejected because the approved
  transition graph excludes self transitions.

## Resolved Unknowns

All Technical Context unknowns are resolved. No `NEEDS CLARIFICATION` marker
remains. Membership/RBAC policy shape, explicit upgrade operations, K8s
bindings, Policy Hooks, retries, and cache behavior remain out of scope rather
than unresolved implementation choices.
