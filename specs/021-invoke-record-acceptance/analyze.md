# Analyze: Invoke-to-Record Backend Acceptance

## Consistency Checks

- Constitution and `AGENTS.md` require a final `Register -> Discover -> Install
  -> Invoke -> Record` proof; the plan keeps all process calls behind Gateway
  and Router.
- Active contracts are consumed as implemented in `contracts/runtime_contracts.go`:
  Northbound v4, Router Internal v3, Agent Router v1, Error v4, Event 0.3, and
  Stream Event 2. No stale v2 endpoint is introduced by Compose or tests.
- Control Plane owns Catalog/Workspace and migrations; Router owns Ledger and
  its migration. The harness never mutates those schemas.
- Runtime A's nested module remains the only location that imports
  `trpc-agent-go`; Runtime B remains an independent direct A2A implementation.
- Tasks map one owner to each shared file: Compose/CI are T011-owned and no
  child runtime task edits them.
- Failure semantics remain explicit. The E2E suite asserts safe code/status,
  not raw dependency text, and treats absent Docker/DB as failure.
- No new fallback, retry, cache, alternate source, or result persistence is
  present in the approved scope.

## Gate Result

PASS for implementation. The only remaining work is the unchecked task list;
implementation must not modify active contracts or platform business logic.
