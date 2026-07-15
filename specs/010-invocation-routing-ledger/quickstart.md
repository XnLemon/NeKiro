# Quickstart: Validate Invocation Routing and Ledger Planning

This guide validates the current repository baseline and the Spec 010 planning
artifacts. It does not claim that the Router, Ledger, SDK, or sample Agents are
implemented.

## 1. Confirm Repository and Workspace Blocker

```powershell
git status --short --branch
git log -3 --oneline
gh issue view 2 --repo NeKiro-project/NeKiro
gh pr checks 18 --repo NeKiro-project/NeKiro
```

Expected current planning result:

- parent Issue #2 is closed;
- PR #18 is merged and its Workspace PostgreSQL closure job is green;
- T001 is the active gate before runtime implementation.

## 2. Validate Existing Contract Baseline

```powershell
go test -count=1 ./contracts
go test -count=1 ./...
go vet ./...
go build ./...
git diff --check
```

These commands validate current code/contracts only. They do not prove an
unimplemented Router or Ledger.

## 3. Validate Spec 010 Artifacts

```powershell
rg -n "NEEDS CLARIFICATION|TODO|TKTK|\\?\\?\\?" specs/010-invocation-routing-ledger
rg -n "Fallback delta|Added fallback evidence" specs/010-invocation-routing-ledger
rg -n "^\\- \\[ \\] T[0-9]{3}" specs/010-invocation-routing-ledger/tasks.md
```

Expected after task generation:

- no unresolved placeholder appears;
- every artifact reports zero added fallback;
- every task uses one unique sequential ID and exact path scope.

## 4. Review Dependency Graph

Verify `plan.md` and `tasks.md` agree on:

```text
Workspace Issue #2 closed
  -> T001 contract gate
  -> {T002 Dispatch, T003 Router foundation, T004 Ledger, T005 callee sample}
  -> {T006 non-stream transport, T008 metadata reads}
  -> {T007 stream/cancel, T009 SDK}
  -> T010 second-Runtime caller
  -> T011 E2E/review/converge
```

The stable maximum parallel batch is four. Shared contract files are owned only
by T001, and final Compose/CI/handoff integration is owned only by T011.

## 5. Planned Backend Acceptance (Not Yet Runnable)

T011 must eventually provide commands that start PostgreSQL, Control Plane,
Router, and two sample Agents and prove:

1. Register, publish, discover, and install both Agents.
2. Invoke the direct sample in JSON and streaming modes through Gateway.
3. Invoke the second Runtime sample, which calls the first through Agent SDK
   and Router.
4. Query the root/child trace after Router reconstruction.
5. Exercise disabled/uninstalled/permission, route, protocol, Agent,
   dependency, timeout, cancel, and interrupted-stream cases.
6. Run 100 concurrent invocations with isolated correlation and one terminal
   outcome per clean accepted invocation.
7. Inspect PostgreSQL, API results, and logs for zero persisted input/output,
   chunk, credential, or raw dependency content.

Do not add placeholder commands that return success before these processes and
tests exist. A missing database, service, credential, or fixture is a not-run or
failed gate, never a pass.

## Fallback Report

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
