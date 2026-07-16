# Research: Non-Streaming A2A Dispatch

## Decision 1: Add a narrow Router transport package

**Decision**: Implement non-streaming Agent calls under
`apps/a2a-router/internal/transport/a2a/` and keep dispatch HTTP validation in
`internal/api`.

**Rationale**: The Router owns Agent transport and must not grow Control Plane
dependencies. A package seam makes A2A protocol mapping independently testable
and leaves streaming/cancellation for Spec 017.

**Alternatives considered**:

- Put A2A calls directly in `dispatch_handler.go`: rejected because it mixes
  HTTP boundary validation, resolution mapping, Ledger ordering, and protocol
  transport in one file.
- Put transport in a shared SDK: rejected because Agent SDK work is Spec 019.

## Decision 2: Return live result only

**Decision**: Return the successful A2A `message/send` result in the same
Router response and do not store or replay result content.

**Rationale**: Spec 010 explicitly says invocation results are live-request
only and Ledger is metadata-only.

## Decision 3: Use strict unsupported-state failures

**Decision**: Unsupported endpoint schemes, unsupported Agent auth modes,
unsupported profiles, malformed A2A responses, and unexpected A2A result types
fail closed with correlated platform errors.

**Rationale**: The fallback policy forbids guessed compatibility behavior.

## Decision 4: Test Ledger ordering without requiring PostgreSQL for every unit test

**Decision**: Use a strict Ledger recorder interface in focused Router tests and
reuse the real Ledger store in integration tests when `NEKIRO_TEST_DATABASE_URL`
is available.

**Rationale**: Spec 014 real PostgreSQL verification remains environment
dependent; Spec 016 should still prove transport/Ledger ordering locally.
