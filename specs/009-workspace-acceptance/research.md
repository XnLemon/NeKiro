# Research: Workspace Acceptance

## Observation Baseline

- Base is `main@99e52aa`, which contains the merged Issue #6, #7, and #8
  runtime slices and their SDD artifacts.
- `apps/control-plane/internal/workspace/integration/workspace_test.go`
  already proves service-to-PostgreSQL flows for installation, inspection,
  lifecycle, restart reconstruction, Catalog disable, dependency failures, and
  100-request races.
- `apps/control-plane/internal/gateway/workspace_handler_test.go` proves the
  active public and internal HTTP mappings with service doubles, but does not
  combine the Gateway handlers with real Catalog and Workspace stores.
- `apps/control-plane/internal/workspace/postgres/inspection_integration_test.go`
  and the migration integration tests require an explicit
  `NEKIRO_TEST_DATABASE_URL` whose database name ends in `_test`.
- The control-plane command composes Catalog and Workspace stores, services,
  authenticators, readiness, and both Gateway handlers in one process. The
  acceptance test reproduces that composition under `httptest`; its sentinel
  Agent server binds only a local ephemeral port to detect accidental calls,
  while no Control Plane or Agent process is started.
- There is no `specs/009-*` artifact on the base branch. The acceptance issue is
  therefore a new evidence feature, not a maintenance-only documentation edit.

## Evidence Gaps

1. No existing test sends the complete Workspace workflow through a real
   composed public/internal HTTP mux backed by the PostgreSQL stores.
2. The existing lifecycle race records errors and checks final row counts, but
   does not retain each successful response to validate every successful
   transition and terminal fact.
3. Existing tests are distributed across earlier feature specs, so there is no
   single traceable requirement-to-command map for Issue #9.
4. A missing database prerequisite must remain a visible not-run condition in
   the Quickstart and final evidence.

## Decisions

### Reuse the Existing Runtime Boundary

Add integration-tagged acceptance tests under the existing Workspace
integration package. Build the real Catalog and Workspace PostgreSQL stores,
services, development-static authenticators, Gateway handlers, and composed
`http.ServeMux` in test code. This proves the boundary without changing
production ownership or introducing a second application assembly path.

### Keep Test Setup Separate From The Behavior Under Test

Fixtures may seed a published Agent through the Catalog service so the test can
start from a known Catalog fact. Discovery, Workspace operations, and internal
resolution are exercised through their actual public/internal HTTP routes. A
test double is used only where a canceled or failing dependency cannot be
reliably produced by the real database.

### Serialize Schema-Resetting Integration Packages

The dedicated database is reset by integration helpers. The acceptance
Quickstart runs schema-resetting Go packages serially and does not claim a
parallel database run is safe.

### No Runtime or Contract Expansion

No API version, route, migration, production data model, retry, cache,
alternate source, endpoint probe, reconciliation, or degraded result is added.
Any behavioral defect exposed by the tests must be fixed only if the existing
Spec #3/#5/#6/#7/#8 contract already requires that behavior; otherwise the
remaining gap is recorded as `Needs policy` rather than guessed.

## Clarification Result

The approved Issue #9 description and the existing merged Specs resolve the
scope, trust boundary, persistence prerequisite, failure semantics, and
out-of-scope runtime. No additional clarification is required before planning.
