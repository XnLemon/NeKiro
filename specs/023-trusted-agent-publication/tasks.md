# Tasks: Trusted Agent Publication

## Slice A — Provider identity and endpoint ownership (#48)

- [x] T001 Add versioned trusted-publication contract models and JSON Schema.
- [x] T002 Add Registry Provider, Endpoint Binding, and Challenge data models
      with explicit states and typed errors.
- [x] T003 Add catalog migration and readiness checks for provider/binding/
      challenge ownership.
- [x] T004 Add strict endpoint parser and network policy validator; reject
      credentials, unsupported schemes, redirects, and disallowed addresses.
- [x] T005 Add challenge creation/completion service with single-use expiry,
      exact proof comparison, and no secret persistence.
- [x] T006 Add authenticated Gateway routes and OpenAPI contract mappings.
- [x] T007 Add unit, contract, and integration tests for success and all
      specified negative paths.
- [x] T008 Update provider/operator documentation and link #48/#47.

## Slice B — Immutable release lifecycle (#49)

- [x] T009 [US3] Add `AgentRelease` contracts, Registry release service/ports,
      immutable bound facts, and explicit state transitions in
      `contracts/trusted_publication.go`, `apps/control-plane/internal/catalog/release.go`,
      and `apps/control-plane/internal/catalog/release_test.go`.
- [x] T010 [US4] Gate trusted Workspace installation and internal resolution on
      an exact verified/published release, persist `installedReleaseId`, and
      persist explicit legacy markers so newly registered unverified versions
      cannot use the compatibility path; preserve the documented pre-v4 legacy compatibility in
      `apps/control-plane/internal/workspace/service.go`,
      `apps/control-plane/internal/workspace/model.go`,
      `apps/control-plane/internal/workspace/postgres/store.go`, and
      `contracts/contracts.go`.
- [x] T011 [US3] [US4] Add catalog/workspace migration 004/002, readiness
      checks, OpenAPI/JSON Schema mappings, exact Release ID/Card digest
      propagation into Router/Ledger metadata, and lifecycle integration/
      contract tests in `apps/control-plane/migrations/004_agent_release.sql`,
      `apps/control-plane/migrations/004_workspace_installation_release.sql`,
      `apps/control-plane/internal/catalog/postgres/`,
      `apps/control-plane/internal/workspace/postgres/`, and `contracts/`.

## Slice C — Router-to-Agent trust (#50)

- [x] T012 Define Router credential contract and explicit key configuration.
- [x] T013 Sign and validate short-lived Router-to-Agent credentials.
- [x] T014 Add forged, expired, audience, direct, JSON/SSE, and nested tests.

## Slice D — Client SDK (#51)

- [x] T015 Add lightweight Go Client SDK through Gateway.
- [x] T016 Add SDK contract tests and application example.

## Slice E — Acceptance (#52)

- [ ] T017 Add clean Compose E2E for Register -> Verify -> Publish -> Install
      -> Invoke -> Record and all negative paths.
- [ ] T018 Add operator/provider failure-category/next-action presentation,
      recovery runbook, and convergence evidence.

## Dependency order

```text
T001 -> T002 -> T003 -> T004 -> T005 -> T006 -> T007 -> T008
T008 -> T009 -> T010 -> T011
T011 -> T012 -> T013 -> T014
T014 -> T015 -> T016
T016 -> T017 -> T018
```

## Ownership / parallelism

- T001/T004 can be prepared in parallel after the Spec review; they touch
  contracts and a new validation package respectively.
- T002/T003/T005 are Catalog-owned and must not modify Workspace or Router
  tables.
- T006 owns Gateway routes only; it consumes Catalog ports.
- T007/T008 may run after the service contract stabilizes.

## Implementation and verification record

- T009-T011 are implemented on `codex/trusted-agent-release` with immutable
  Release state transitions, exact Card/Release provenance, explicit legacy
  markers, stable Workspace gate errors, and active contract/schema mappings.
- Verification completed: `go test ./...`, `go vet ./...`,
  `golangci-lint run ./...`, `gofmt`, and `git diff --check`.
- PostgreSQL integration coverage includes migration readiness, immutable
  Release transitions, concurrent publication, verified-only suspend/revoke,
  and explicit pre-v4 fixture seeding. Full database-backed integration runs
  require `NEKIRO_TEST_DATABASE_URL` and are not executed when it is absent.
- The existing clean Invoke-to-Record acceptance now provisions one-time proof
  files in the Sample Agent containers and exercises the trusted
  Register -> Verify -> Publish -> Install path. T017 remains open for the
  complete trusted-publication negative-path acceptance matrix.
- T012-T014 completed in Spec 024 and merged through PR #55 after CI run
  `30060752722` passed authenticated JSON/SSE/nested/cancel acceptance, race,
  vet, lint, and credential conformance gates.
- T015-T016 are implemented under Spec 025 with a standalone Gateway-only Go
  Client SDK, strict JSON/SSE and Platform Error v4 validation, compiled
  application example, and focused contract/Router/Control Plane/SDK tests.
  Full Issue #51 gates and independent Review are recorded in Spec 025.
