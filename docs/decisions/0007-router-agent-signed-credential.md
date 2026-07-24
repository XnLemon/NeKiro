# ADR 0007: Router-to-Agent Signed Invocation Credential

- Status: Accepted
- Date: 2026-07-22
- Decision owners: A2A Router, Agent SDK, sample Agent adapters
- Feature: Spec 024 Router-to-Agent Authentication / GitHub #50
- Supersedes: ADR 0006 section "Phase 1 Agent transport authentication"

## Context

ADR 0006 intentionally allowed only anonymous Agent Card
`authentication.type=none` until key ownership, credential binding, expiry,
failure, and replay policy were specified. Issues #48 and #49 now establish a
provider-controlled endpoint and an immutable published release. The remaining
managed-hop gap is that a caller can send an A2A body directly to that endpoint
and can forge the platform context headers that sample Agents currently read.

The solution must authenticate the exact Workspace, Agent release,
capability, Invocation, Task, and Trace without placing a credential in Agent
Card, Registry, Workspace, Ledger, result DTOs, or Agent Runtime internals. It
must work for JSON, streaming, cancellation, and nested calls across different
Runtime implementations.

## Decision

### One Ed25519 JWT per A2A HTTP request

The A2A Router signs a compact JWT/JWS immediately before every outbound A2A
HTTP request. `alg` is exactly `EdDSA` using Ed25519, `typ` is exactly
`nekiro-router+jwt`, and `kid` is one explicitly configured key identifier.
The Router holds the 64-byte private key. Agent adapters hold only the 32-byte
public key, so a compromised Agent cannot mint Router credentials.

The registered claims are an exact issuer, exactly one audience, integer
issuance/expiration times, and one unique token ID. Custom claims bind the
Workspace, Agent ID/Card version, immutable Release ID/Card digest, capability,
Invocation ID, root Task ID, optional parent Invocation ID, and Trace ID. The
audience is the canonical trusted endpoint origin. A full endpoint path is not
an audience because protocol operations at one recipient may use multiple
paths; a global platform audience would allow recipient confusion.

Every protocol HTTP request receives a fresh token ID. In particular, a
streaming request and a later `tasks/cancel` request never reuse one credential.

### Strict, closed profile

Router Invocation Credential v1 is a companion contract to A2A Profile 0.2;
it does not mutate the existing profile version. The protected header and
claims objects reject unknown and duplicate members. Compact segments use
strict unpadded Base64url. NumericDate claims are required integers. Only
`EdDSA` is allowed.

Credential TTL is a required Router setting from 1 through 300 seconds. Agent
verification requires `exp > iat`, `exp - iat <= 300`, `iat <= now`, and
`now < exp`. No clock-skew leeway is implicit. Deployment is responsible for
synchronized clocks.

The Agent validates signature, issuer, exact key ID, required claims, maximum
lifetime, and exact configured audience. It then requires every claim to equal
one corresponding platform context header before Runtime execution. The
optional parent claim/header must be present or absent together. Neither body,
Host, Card, nor another header can supply a missing fact.

### Agent-local replay boundary

After all other validation succeeds, the Agent adapter atomically records
`jti -> exp` in memory. A live duplicate is rejected before Runtime execution;
expired entries are removed. The credential's hard maximum lifetime bounds the
window and map retention. Durable, cross-replica, and cross-restart replay
coordination is deferred until a deployment topology and availability policy
exists. It is not approximated with Registry, Workspace, or Ledger storage.

### Failure and secrecy

Missing, malformed, forged, expired, wrong-issuer, unknown-key, and replayed
credentials return HTTP 401 with stable `UNAUTHENTICATED`; 401 includes only
`WWW-Authenticate: Bearer`. A validly signed wrong-audience or exact-context
mismatch returns HTTP 403 with stable `FORBIDDEN`. Bodies have fixed generic
messages and `Cache-Control: no-store`. Rejection occurs before Agent Runtime
logic.

Credentials, signatures, key material, and token IDs are prohibited from Card,
public errors beyond the fixed code/message, logs, events, Ledger projections,
and persistent tables.

### Configuration ownership

Router serving requires exact issuer, key ID, raw Ed25519 private key encoded
as unpadded Base64url, and TTL. Every Agent adapter requires exact issuer,
canonical audience, key ID, and raw public key encoded the same way. Key strings
are not trimmed or repaired. Missing, blank, padded, wrong-length, malformed,
or unsafe values fail the owning process at startup.

The first profile has one active key. Multi-key rotation, remote JWKS discovery,
provider-managed keys, OAuth exchange, API-key forwarding, and mTLS require a
later Spec/ADR. There is no anonymous, alternate-key, retry, or old-token
fallback.

### Platform and Runtime boundary

Signing stays inside Router Data Plane code. Verification/replay is a thin Go
Agent SDK protocol adapter wired around only the A2A execution routes of both
sample Agents. Readiness and endpoint ownership proof remain separate. Agent
model, tool, workflow, session, and business errors remain Runtime-owned.

Agent→Router nested authentication remains the existing opaque credential
bound to one Workspace/Agent pair. It is the opposite direction and is not
replaced by the Router→Agent JWT.

## Compatibility

- Agent Card Schema remains 0.2 and uses the existing `http_bearer` enum.
- Newly trusted sample releases change from `none` to `http_bearer`; managed
  transport no longer executes a trusted sample Card declaring `none`.
- A2A Profile Schema 0.2 and protocol 0.3.0 remain unchanged. Router Invocation
  Credential v1 owns the additional signed-header contract.
- Router Internal dispatch moves from v3 to v4 because supported managed Card
  authentication changes from `none` to `http_bearer`; the v3 dispatch route
  is retired without dual-read while metadata reads remain on v3. Northbound
  Invocation v4, Control Plane resolution, Invocation Event v0.3, Result v1,
  Stream Event v2, and Ledger schemas remain unchanged.
- No previous Router-to-Agent credential is accepted, dual-read, retried, or
  translated.

## Consequences

- A public Agent endpoint can distinguish one exact managed call from a direct
  caller without accessing platform storage.
- Each outbound HTTP request performs one local signature and Agent-side
  verification, with no new service or database round trip.
- Restarting or horizontally scaling an Agent does not share replay state; the
  bounded residual risk is explicit until the deferred topology policy exists.
- Operators must distribute one private/public key pair and synchronize clocks.
- Sample and future adapters can use the contract without importing Router or
  Runtime framework internals.

## Rejected Alternatives

- HMAC shared secrets, which would let every Agent mint Router credentials.
- RSA for the first profile, which adds larger keys/tokens without a required
  interoperability benefit.
- A global audience or Agent ID-only audience.
- Reusing one token for stream cancellation.
- Accepting unknown claims, padded Base64, multiple algorithms, clock leeway,
  or detailed crypto errors for compatibility.
- PostgreSQL/Redis replay storage without an approved replica/availability
  policy.
- Putting verification inside each Runtime handler or storing auth facts in
  Registry/Ledger.

## Fallback Report

```text
Fallback delta: removed 1, retained 2, added 0, net -1
Added fallback evidence: none
```

The retained behaviors are Go's documented nil-Transport policy and ADR
0006's one bounded remote cancellation propagation attempt. Neither substitutes
identity, key, endpoint, result, or persistence data.
