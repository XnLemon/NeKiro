# Tasks: Invoke-to-Record Backend Acceptance

## Phase 1: Specification and Integration Gate

- [X] T001 Create Spec, clarification, plan, tasks, and analyze artifacts for Issue #30; confirm active contracts and ownership boundaries.
- [ ] T002 Add standalone Dockerfiles for Runtime B and the nested Runtime A module, with explicit non-root execution and no platform database access (`agents/runtime-b/Dockerfile`, `agents/runtime-a/Dockerfile`).
- [ ] T003 Compose PostgreSQL, both migrations, Control Plane, Router, Runtime B, and Runtime A with explicit env validation and readiness dependencies (`deploy/compose.yaml`).

## Phase 2: Acceptance Harness

- [ ] T004 Add a real-process Go harness that registers/publishes Cards, discovers capability, creates a Workspace, installs both Agents, and invokes through Northbound v4 (`tests/e2e/invoke-record/`).
- [ ] T005 Verify JSON/SSE result contracts, exact correlation, nested parent-child lineage, metadata reads, restart durability, and Workspace isolation (`tests/e2e/invoke-record/`).
- [ ] T006 Add failure, content-exclusion, and 100-concurrent acceptance checks with no raw dependency/error assertions (`tests/e2e/invoke-record/`).

## Phase 3: Delivery Gates and Review

- [ ] T007 Wire Compose config and real E2E execution into CI; make missing Docker/PostgreSQL a failed gate (`.github/workflows/ci.yml`).
- [ ] T008 Run root, nested Runtime A, static, contract, PostgreSQL, Compose, and E2E verification; capture command/evidence in `specs/021-invoke-record-acceptance/quickstart.md`.
- [ ] T009 Independent Standards review against AGENTS.md/constitution and smell baseline; independent Spec review against this Spec/Plan/Tasks; return findings as append-only convergence tasks.
- [ ] T010 Resolve every review finding, rerun all mapped gates, and perform Converge with zero blockers; update Spec 010 parent task and Issue #19/#30 handoff facts.

## Dependency Graph

```text
T001 -> [T002 || T004 design]
T002 -> T003 -> [T004 || T005]
T004 -> T005 -> T006 -> T007 -> T008 -> T009 -> T010
```

## Fallback Audit

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
