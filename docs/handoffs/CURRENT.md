# Current Handoff: Trusted Publication Acceptance and Operations

**Updated**: 2026-07-24 (Asia/Shanghai)

**State**: Spec 026 / Issue #52 is implemented, locally verified,
independently reviewed, and converged on
`codex/trusted-publication-acceptance`. Commit/PR/CI/merge and issue closure
remain.

## Repository State

- Upstream: `https://github.com/NeKiro-project/NeKiro.git`
- Fork: `https://github.com/XnLemon/NeKiro.git`
- Base: `main` at `2566defd510b5acbd36ffdce255b67500e8783c0`
- Current branch: `codex/trusted-publication-acceptance`
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

## Remaining Delivery Gates

1. Commit with the repository-local identity, push, and open a ready PR that
   references #52 and #47.
2. Require green CI, merge, close #52, and close #47 only after checking every
   parent acceptance criterion against the merged evidence.

Do not add retry, alternate endpoint, automatic Release recovery, key rotation,
cross-replica replay storage, old-protocol fallback, SQL state mutation, or
direct Agent access. Those behaviors require a separate approved Spec/ADR.
