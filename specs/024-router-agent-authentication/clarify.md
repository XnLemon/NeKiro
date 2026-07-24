# Clarification Record: Router-to-Agent Authentication

**Date**: 2026-07-22

No critical ambiguity requires additional user input. Issues #47 and #50,
Spec 023, the repository constitution, and the existing Router/Agent contracts
already determine the product boundary.

## Resolved decisions

- Authentication is mandatory for every managed call to a trusted sample
  Agent; there is no anonymous or provider-key fallback.
- A credential binds the complete platform context used by Agent execution,
  including root and optional parent lineage in addition to Issue #50's
  required release and invocation fields.
- Missing, malformed, forged, expired, wrong-issuer, and replayed credentials
  are unauthenticated. A valid signature for a different audience or context
  is forbidden.
- Replay protection is Agent-local for the bounded credential lifetime. A
  durable cross-replica policy is deferred rather than guessed.
- Key rotation and remote key discovery are explicitly out of scope. The
  first profile has one required key ID and one required asymmetric key pair.
- The prior Router Internal dispatch route accepted anonymous `none` Cards.
  That semantic change is breaking, so dispatch moves to Router Internal v4;
  the existing metadata reads remain on Router Metadata v3 and there is no
  runtime compatibility or dual-read path.

## Coverage status

| Category | Status | Evidence |
| --- | --- | --- |
| Functional scope | Clear | Spec user stories and FR-001 through FR-013 |
| Domain and lifecycle | Clear | Credential, signing identity, replay entry, authenticated context |
| Security and privacy | Clear | 401/403 split, claim binding, replay, secrecy requirements |
| Integration | Clear | JSON, SSE, cancellation, nested runtime scenarios |
| Failure handling | Clear | Negative matrix and no fallback policy |
| Compatibility | Clear | Existing result/Ledger semantics are preserved; credential and Router dispatch contracts are versioned |
| Deferred policy | Clear | Rotation, remote discovery, and durable replay are non-goals |
