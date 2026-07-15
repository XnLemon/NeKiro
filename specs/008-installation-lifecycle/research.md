# Research: Installation Lifecycle Baseline and Gap Record

## Observed Baseline

- `apps/control-plane/internal/workspace/service.go` already exposes owner
  authorization before `UpdateInstallation` and `Uninstall` store calls.
- `apps/control-plane/internal/workspace/postgres/store.go` already locks the
  targeted Installation with `FOR UPDATE`, evaluates transitions, performs
  atomic status/timestamp updates, and returns `RETURNING` values.
- `apps/control-plane/migrations/003_workspace.sql` already retains terminal
  rows, enforces state/timestamp coherence, and releases current uniqueness
  only for `uninstalled` rows.
- `apps/control-plane/internal/gateway/workspace_handler.go` already exposes
  active v3 PATCH/DELETE routes with strict JSON decoding, authentication, and
  fixed error mapping.
- Existing tests prove one happy-path lifecycle workflow, but do not independently
  prove the complete transition table, same-state and repeated-delete conflicts,
  wrong Workspace/Installation behavior, dependency propagation, committed
  timestamp equality after restart, or concurrent lifecycle/install histories.

## Gap Classification

| Area | Classification | Evidence | Required Issue #8 work |
| --- | --- | --- | --- |
| Service transition policy | Partial evidence | One combined workflow in `service_test.go` | Add table-driven legal/illegal and owner tests |
| PostgreSQL transition transaction | Partial evidence | Store has row locks and `RETURNING`; no focused lifecycle integration suite | Add committed timestamp, constraints, restart, and concurrency tests |
| HTTP adapters | Missing focused evidence | Routes exist; lifecycle methods are test stubs in the gateway fake | Add PATCH/DELETE success, malformed, auth, owner, conflict, not-found, dependency, and secret tests |
| Catalog boundary | Partial evidence | Lifecycle methods have no Catalog calls | Add a spy/dependency assertion that lifecycle does not consult Catalog |
| Contract mapping | Partial evidence | Active v3 OpenAPI already declares the routes and errors | Add contract assertions for transition target and response/error set |

## Decisions

1. Preserve the active Northbound v3 contract and existing module ownership.
2. Keep all state transitions in the Workspace store transaction and use the
   committed `RETURNING` row as the response fact.
3. Treat every self-transition and terminal repetition as `CONFLICT`.
4. Preserve the only approved empty-result behavior: a real empty Installation
   list for an existing authorized Workspace. No new fallback is allowed.

## Fallback Inventory

| Candidate | Classification | Evidence |
| --- | --- | --- |
| Empty Installation list for an existing empty Workspace | Keep | Existing Workspace v3 contract and Spec 003 |
| Same-state success | Remove | Explicit lifecycle transition graph and conflict policy |
| Hard-delete or row reuse on uninstall | Remove | Immutable audit evidence and reinstall-new-ID policy |
| Catalog reconciliation or endpoint probe | Remove | Workspace/Catalog ownership boundary |
| Retry, cache, alternate source, degraded success | Remove | No approved product or operational policy |

Fallback delta for Issue #8: removed `0`, retained `1`, added `0`, net `0`.
Added fallback evidence: none.

## Prerequisite and Baseline Verification

The repository PowerShell prerequisite script is present, but no `pwsh` or
`powershell` executable is available in this macOS worktree. Equivalent file
and artifact checks are performed manually. The initial focused run exposed
two test-only issues in the new evidence: the lifecycle service fixture lacked
a published Card, and the OpenAPI assertion used an unavailable helper method.
Those issues are corrected in the Issue #8 changes. The focused verification
commands are:

```text
go test -count=1 ./contracts ./apps/control-plane/internal/workspace ./apps/control-plane/internal/gateway ./apps/control-plane/internal/workspace/postgres
go test -race -count=1 ./apps/control-plane/internal/workspace ./apps/control-plane/internal/gateway
git diff --check
```
