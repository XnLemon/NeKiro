# Clarification Record: Trusted Publication Acceptance and Operations

**Date**: 2026-07-24

## Result

No critical ambiguities detected worth formal clarification. Zero questions
were required because the active contracts, Spec 023, Spec 024, ADR 0004, ADR
0006, and Issue #52 already determine the state, error, ownership, and Ledger
semantics.

## Resolved terminology

- Issue wording that refers to a disabled Workspace is normalized to the
  existing `disabled` Workspace Installation state. Phase 1 defines no
  Workspace disable lifecycle.
- `trust method is queryable` means an Invocation exposes its exact Release ID
  and Card digest, and the authorized Release read exposes the corresponding
  verification method. It does not add the verification method to Ledger.
- Policy, authentication, and direct-Agent failures before the Router-owned
  `created` event are not accepted Invocations and do not create Ledger facts.
  Accepted route/transport failures do terminalize in Ledger.

## Deferred policy

- Verification-evidence retention, suspension approval, signing-key rotation,
  cross-replica replay protection, and automated reconciliation remain outside
  this feature. The runbook labels them as `Needs policy` and adds no fallback.
