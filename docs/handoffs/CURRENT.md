# Current Handoff: Trusted Publication Acceptance Complete

**Updated**: 2026-07-24 (Asia/Shanghai)

**State**: Spec 026 / Issue #52 is complete and merged through PR #57. All
seven checks passed in CI run `30074754169`; Issue #52 and parent Issue #47 are
closed.

## Repository State

- Upstream: `https://github.com/NeKiro-project/NeKiro.git`
- Fork: `https://github.com/XnLemon/NeKiro.git`
- Base: `main` at `785f9cf2deec57f4bd9f6dd1d1ead46c800da351`
- Delivery: PR #57, squash merge `785f9cf2deec57f4bd9f6dd1d1ead46c800da351`
- Active artifacts: `specs/026-trusted-publication-acceptance/`
- Parent artifacts: `specs/023-trusted-agent-publication/`
- Required Git identity: `Nene7ko_ <1604009816@qq.com>`

## Delivered Scope

- One authoritative clean Compose/PostgreSQL acceptance for trusted Register ->
  Verify -> Publish -> Discover -> Install -> Invoke -> Record.
- Exact Binding, Challenge, Release, Installation, Invocation/Event, and Trace
  fact retention in the E2E harness, with Release ID/Card digest linkage back to
  the Catalog-owned `http_well_known` trust method.
- Cross-runtime Runtime A -> Router -> Runtime B lineage with one Trace/root
  Task and exact per-Agent immutable Release provenance.
- Wrong proof plus fresh-challenge recovery, expired/reused challenge,
  disallowed destination, unavailable verification endpoint, unpublished/
  suspended/revoked Release, and disabled Installation cases.
- Compose-internal forged, expired, wrong-audience, and missing Router
  credential rejection without exposing an Agent host port.
- Accepted unavailable endpoint failure with correlated terminal Ledger record
  and exact Release provenance.
- Extended post-issuance response, Card/Binding/Challenge/Release/Installation,
  Ledger, and process-log secrecy scans.
- Router cancellation race correction: terminal commits use one bounded context
  independent of caller cancellation; interrupted stream chunks are not retried
  or sequence-advanced, and cancellation no longer becomes dependency failure
  or permanent `running` state.
- Provider/Workspace-owner/operator trusted-publication operations runbook with
  inspection, responsibility, recovery, completion, and `Needs policy` tables.

## Verification Completed

- Fresh `nekiro-acceptance-spec026` Compose volumes and
  `go test -tags=e2e -count=1 ./tests/e2e/invoke-record`: PASS in 28.351s,
  including five real cancellation probes and exact no-Ledger rejection reads.
- `go test ./...`, `go vet ./...`, `golangci-lint run`: PASS.
- WSL/Linux `go test -race ./...`: PASS.
- Runtime A test/vet and WSL/Linux race: PASS.
- `docker compose ... config --quiet`, GitHub Actions `actionlint`, and
  `git diff --check`: PASS.
- `speckit-analyze`: 18/18 FR, 8/8 SC, and 23/23 tasks mapped; zero
  Critical/High/Medium findings.
- Independent Spec and standards re-review: zero High/Medium findings.
- `speckit-converge`: 18/18 FR, 8/8 SC, 12/12 acceptance scenarios, 7/7 plan
  decisions, and 8/8 constitution principles satisfied; no task appended.
- PR #57 CI run `30074754169`: all seven checks PASS, including
  `backend-acceptance` in 2m32s.
- Issue #52 and parent Issue #47: CLOSED after criterion-by-criterion audit.

## Delivery Closure

No Spec 026 delivery gates remain. Any follow-on retention, key-rotation,
replay, reconciliation, or production-governance work requires a separate
approved Spec/ADR.

Do not add retry, alternate endpoint, automatic Release recovery, key rotation,
cross-replica replay storage, old-protocol fallback, SQL state mutation, or
direct Agent access. Those behaviors require a separate approved Spec/ADR.
