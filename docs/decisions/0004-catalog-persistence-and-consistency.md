# ADR 0004: Catalog Persistence And Strong Discovery Consistency

- Status: Accepted
- Date: 2026-07-14

## Context

Spec 002 introduces the first runnable Control Plane slice: immutable Agent Card
registration, exact reads, publication, disablement, and published-only
Discovery. Registry must remain the sole Card fact, successful writes must
survive process restart, and dependency failures must never become stale or
empty success. The Gateway also needs owner authentication before enterprise
identity-provider selection is in scope.

## Decision

- Catalog owns one PostgreSQL schema accessed through pinned `pgx/v5`.
- Ordered SQL migrations use pinned `tern/v2`, are embedded in the Control Plane
  binary, and run only through an explicit migration command. Serving verifies
  the expected schema version and never migrates automatically.
- Agent identity, immutable Card version, lifecycle metadata, and capability
  index rows commit transactionally. Discovery reads those Registry-owned facts
  directly and stores no second Card copy.
- Publication and disablement use row locking and database constraints.
  Publication additionally increments one Catalog-owned transactional clock
  row whose lock is held until commit, making sequence order equal successful
  publication commit order. Publication is discoverable in the same commit;
  disablement is excluded from subsequent reads without an asynchronous
  projection, queue, cache, retry, or repair path.
- Discovery uses a stateless versioned keyset cursor bound to normalized
  filters, page size, and a commit-ordered first-page publication boundary read
  with eligible rows in one repeatable-read snapshot.
- Gateway authentication is replaceable. The initial runnable adapter is
  enabled only by explicit `development-static` mode and accepts strict caller
  IDs paired with SHA-256 token digests. It stores no raw token and compares
  incoming token digests in constant time. Public caller headers are never
  trusted.
- Required database, listen, authentication mode, and principal configuration
  have no application defaults. Missing, blank, malformed, unsupported, or
  unreachable values fail at their owning boundary.

## Compatibility

Northbound API v2 is additively completed before runtime implementation with
Bearer security, Catalog trace headers, pagination policy, visibility rules,
and exact fixed errors. Agent Card 0.2 and existing success payloads are
unchanged. Historical Northbound v1 and Agent Card 0.1 remain migration evidence
only; no runtime decoder, route, upgrade, or dual-read window is introduced.

## Consequences

- Catalog writes and Discovery eligibility have one durable transactional truth.
- PostgreSQL/schema failure is visible as startup, readiness, or dependency
  failure rather than degraded success.
- Deep pagination avoids offsets and excludes publications after traversal
  begins, including transactions that began earlier but committed later, while
  still rechecking current published state on every page.
- Production identity-provider selection, asynchronous projections, search
  infrastructure, hot/cold Card storage, Agent deployment, and Runtime health
  remain deferred and require separate decisions when evidence exists.
