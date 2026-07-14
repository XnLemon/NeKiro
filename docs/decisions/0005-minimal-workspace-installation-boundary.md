# ADR 0005: Minimal Workspace and Installation Boundary

- Status: Accepted
- Date: 2026-07-14

## Context

After Catalog registration and discovery, Phase 1 needs a durable trust fact
between a caller and an exact Agent version before Invocation Dispatch can
exist. The repository already contains partial Installation v1 and API
foundations, but they do not decide Workspace identity, version selection,
permission subsets, lifecycle, ownership, concurrency, internal authorization,
or complete failures.

Without one frozen boundary, concurrent runtime slices could couple Workspace
to Catalog tables, reinterpret a Workspace as deployment infrastructure,
silently upgrade versions, overwrite permission acceptance, or collapse
disabled and dependency states.

## Decision

- A Phase 1 Workspace is a logical authorization and audit root containing only
  caller-selected `workspaceId`, immutable trusted creator `ownerId`, and server
  timestamps. It is not a Kubernetes Namespace or deployment object.
- Owner authorization is accessed through a narrow policy interface. The first
  implementation uses exact caller/owner equality. Membership and RBAC are
  future policy owners and do not add columns to the minimal Workspace fact.
- Workspace owns PostgreSQL Workspace and Installation tables. It does not read
  or write Catalog tables. Catalog publishes a controlled read interface for
  current published-version selection and exact-version state.
- Installation selects the highest currently published version satisfying the
  submitted SemVer constraint. Pre-releases participate only when explicitly
  included by their constraint branch. Equal SemVer precedence uses the
  bytewise-greatest exact version string as the deterministic final key.
- The successful Catalog selection result is the version-choice linearization
  point. Workspace persists that exact version permanently and does not
  auto-upgrade or background-reconcile it.
- Accepted permissions are an exact case-sensitive subset of the selected
  Card's declarations. The canonical stored snapshot is sorted, may be empty,
  and authorizes a capability only when every required permission is present.
- One Workspace may have at most one enabled or disabled Installation for an
  Agent. Lifecycle is `enabled <-> disabled -> uninstalled`. Uninstall preserves
  the row and releases a partial unique slot; reinstall creates a new row and
  identifier.
- Internal exact resolution is served by Control Plane Internal v2 to a
  separately authenticated trusted service principal. It returns only an
  enabled exact Installation and currently published exact Card whose capability
  is authorized. The Router never reads Control Plane storage.
- Installation v2 freezes ascending bytewise permission order,
  `installedAt <= updatedAt`, and `uninstalledAt == updatedAt` for terminal
  records. Inspection uses bounded opaque cursor pagination.
- Installation disabled and Catalog Agent-version disabled are different
  failures. Platform Error v3 adds `INSTALLATION_DISABLED`; existing Platform
  Error v2 remains the Catalog/Invocation contract and `AGENT_DISABLED` retains
  its Catalog meaning.
- Required identity, Workspace, Catalog, and PostgreSQL failures are explicit.
  No retry, stale Card, alternate source, default owner/range, same-state
  success, or automatic upgrade fallback is introduced. A successful empty
  Installation list for an existing authorized Workspace is genuine product
  behavior.

## Compatibility

Issue #3 completes Workspace v1, Installation v2, Northbound API v3, Control
Plane Internal API v2, and Platform Error v3 for Workspace/Installation before
their first runtime consumers. Installation semantic invariants, internal
pre-correlation errors, and bounded inspection pagination are explicit contract
changes. Existing Catalog/Invocation consumers remain on Platform Error v2.

No deployed Workspace or Router resolution runtime requires a compatibility
window. Historical Northbound v2, Installation v1, and Internal API v1 remain unchanged, and
no dual route, decoder, field fallback, retry, or background migration behavior
is added.

## Consequences

- Workspace Installation becomes a durable, runtime-neutral authorization fact
  that remains useful across Agent languages and frameworks.
- Catalog remains the sole Agent Card source of truth, while Workspace owns the
  exact pin and accepted permission snapshot.
- A Catalog disable can make a newly committed Installation immediately
  non-resolvable if it follows the selection linearization point; this is
  visible as `AGENT_DISABLED` and does not rewrite history.
- PostgreSQL row locks and a partial unique index provide replica-safe lifecycle
  and one-current-Installation guarantees.
- Northbound owners and internal Router callers have distinct authentication
  boundaries without introducing general RBAC.
- Explicit upgrade, Membership/RBAC, K8s bindings, Policy Hooks, quotas,
  approvals, retries, caching, and Agent deployment remain separate future
  decisions driven by evidence.
