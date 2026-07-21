# Implementation Plan: Invoke-to-Record Backend Acceptance

**Branch**: `codex/021-invoke-record-acceptance`  
**Spec**: [spec.md](spec.md)

## Summary

Wire the already implemented Control Plane, Router/Ledger, Runtime B, and
Runtime A into one reproducible Compose deployment. Add a Go E2E harness that
drives the northbound API, validates live JSON/SSE and metadata contracts, and
records the failure/concurrency/secrecy evidence required by Issue #30.

## Architecture

```text
PostgreSQL <- migrations <- Control Plane (Gateway/Catalog/Workspace/Dispatch)
                    ^ v3 exact installed-version resolution
                    |
                 A2A Router <- v3 dispatch + v1 Agent nested endpoint
                   |   |
              Runtime B  Runtime A -> Agent SDK -> Router -> Runtime B
```

Compose owns only process startup, network boundaries, health dependencies, and
explicit configuration. Registry and Workspace data remain Control Plane-owned;
Ledger remains Router-owned. Runtime images contain no database client.

## Files and Ownership

- `apps/a2a-router/Dockerfile`, `agents/runtime-b/Dockerfile`,
  `agents/runtime-a/Dockerfile`: process image owners.
- `deploy/compose.yaml`: final process/network/migration composition owner.
- `tests/e2e/invoke-record/`: acceptance harness owner; it may read PostgreSQL
  only for metadata/secrecy assertions and never writes platform tables.
- `.github/workflows/ci.yml`: acceptance job owner.
- `specs/021-invoke-record-acceptance/`: SDD and evidence owner.

No contracts, SDK, Control Plane, Router, or Runtime business behavior changes
are planned.

## Configuration

Every service receives explicit listener, database, service credential, URL,
limit, and deadline values. Secrets are supplied by CI/environment variables;
no default credential or localhost fallback is introduced. Compose healthchecks
are readiness checks only and do not auto-migrate serving processes.

## Verification Strategy

1. Validate all images and Compose configuration.
2. Run the Go harness against a clean project and dedicated PostgreSQL volume.
3. Register/publish Cards, discover, create Workspace, install both Agents.
4. Exercise direct JSON/SSE, nested JSON, reads/restart, failure matrix, and
   100 concurrent requests.
5. Scan logs, API payloads, and Ledger tables for forbidden literals and verify
   no result persistence endpoint exists.
6. Run static root tests, runtime-a nested-module tests, and CI Compose/E2E.

## Constitution Check

- Phase 1 loop: PASS; this is the final backend proof of Invoke -> Record.
- Runtime boundary: PASS; framework code remains in Runtime A only.
- Ownership/contracts: PASS; wiring consumes active contracts without changes.
- Router mediation/lineage: PASS; all managed root and nested calls traverse
  Router and retain root task, parent, and trace IDs.
- Explicit failures/secrets: PASS; missing dependencies fail the job and only
  safe error codes are asserted.
- SDD/review: PASS; this plan is followed by tasks, analysis, implementation,
  mapped tests, independent two-axis review, and convergence.

## Fallback Audit

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
