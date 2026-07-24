# Data Model: Trusted Publication Acceptance and Operations

This feature adds no persistent entity or schema. It validates relationships
among existing owned facts and introduces only test/runbook concepts.

## Existing fact relationships under acceptance

```text
Provider
  -> Endpoint Binding (verificationMethod, status, evidence digest)
  -> Agent Release (immutable Card version/digest + Binding + state)
  -> Workspace Installation (exact installedReleaseId)
  -> Invocation / Event (exact Release ID + Card digest)
  -> Release query (verificationMethod and immutable trust context)
```

### Acceptance Run

- Identity: one test process execution against one newly created Compose
  project and empty volume set.
- Contains: positive lifecycle, negative cases, secrecy scan, cleanup result.
- Persistence: none; CI logs are workflow evidence, not a platform fact.

### Publication Fixture

- Exact Agent ID and Card version.
- Endpoint and capability.
- Endpoint Binding ID and verification status/failure category.
- Release ID, Card digest, verification method, and lifecycle state.
- Installation ID and status when installed.
- Validation: values are always obtained from public responses; no guessed IDs
  or direct state writes.

### Invocation Provenance Link

- Invocation ID, root Task ID, optional parent Invocation ID, Trace ID.
- Workspace ID, Agent ID, Card version, Release ID, Card digest.
- Relationship: projection and every Event repeat one exact provenance pair;
  the Release read repeats the Release ID, Agent/Card identity, and Card digest.
- Trust method: read from the Catalog-owned Release, not persisted in Ledger.

### Recovery Action

- Trigger: existing stable state or error category.
- Owner: provider, Workspace owner, or platform operator.
- Automatic evidence: public state/error/Trace/Ledger fact already recorded.
- Manual action: existing API or deployment correction.
- Completion: explicit state/readiness/new invocation check.
- Persistence: documentation only.

## State transitions exercised

```text
Endpoint Binding: pending -> failed -> verified (using a fresh challenge)
Agent Release: pending_verification -> verified -> published
Agent Release: published -> suspended -> revoked
Installation: enabled -> disabled -> enabled
Invocation: created -> routing -> running -> succeeded
Invocation: created -> routing -> failed (endpoint unavailable)
Invocation: created -> routing -> running -> canceled (caller disconnect race)
```

`revoked` is terminal. A suspended or revoked Release is not updated back to
published; recovery creates a new Card version/Binding/Release chain.
