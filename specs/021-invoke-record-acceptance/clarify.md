# Clarification Record: Invoke-to-Record Acceptance

## Decisions

1. The acceptance environment is a real Docker Compose deployment. The E2E
   command fails when Docker or PostgreSQL is unavailable; it does not silently
   skip the gate.
2. Compose uses the active runtime contracts (Northbound v4, Router Internal
   v3, Agent Router v1, Runtime Error v4, Event v0.3, Stream Event v2).
3. Cards are registered through the public Gateway before every clean run. The
   Router resolves endpoint and exact installed version through Control Plane;
   no Card or endpoint is hard-coded inside Router or the harness.
4. Runtime B is the direct A2A sample and supports deterministic JSON/SSE
   fixtures. Runtime A is the isolated trpc-agent-go sample and performs one
   non-streaming SDK nested call to Runtime B.
5. Failure cases are asserted through safe Platform Error codes and status;
   raw service body text is never used as an acceptance oracle.
6. The 100-call concurrency gate uses independent input values and checks both
   returned content and Ledger lineage; it is not a benchmark or a retry loop.

## Unresolved Policy

None. Runtime task retention, graceful shutdown, and post-side-effect Ledger
failure policies remain explicitly deferred under T020/T021 of Spec 010 and are
not invented by this acceptance feature.

## Fallback Audit

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
