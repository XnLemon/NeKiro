# Data Model: Router-to-Agent Authentication

This feature adds no persistent platform entity or database migration. The
following transient protocol values are the complete data model.

## Router Invocation Credential v1

| Field | Type | Required | Rule |
| --- | --- | --- | --- |
| `alg` | protected header string | yes | exactly `EdDSA` |
| `typ` | protected header string | yes | exactly `nekiro-router+jwt` |
| `kid` | protected header identifier | yes | exact configured key ID |
| `iss` | string | yes | exact configured Router issuer |
| `aud` | array of string | yes | exactly one canonical endpoint origin |
| `iat` | integer NumericDate | yes | not later than verification time |
| `exp` | integer NumericDate | yes | later than `iat`, no more than 300 seconds later, and later than verification time |
| `jti` | identifier | yes | unique per outbound A2A HTTP request |
| `workspaceId` | identifier | yes | exact trusted dispatch Workspace |
| `agentId` | identifier | yes | exact resolved Card Agent ID |
| `agentVersion` | semantic-version string | yes | exact resolved Card version |
| `releaseId` | identifier | yes | exact trusted installed Release ID |
| `cardDigest` | lowercase hex string | yes | exactly 64 characters |
| `capability` | identifier | yes | exact resolved/authorized capability |
| `invocationId` | identifier | yes | exact Invocation ID |
| `rootTaskId` | identifier | yes | exact root Task ID |
| `parentInvocationId` | identifier | no | present exactly for a child Invocation |
| `traceId` | identifier | yes | exact Trace ID |

The protected header and claims object have no additional members. The compact
credential is transient and MUST NOT be serialized into any domain entity.

## Authenticated Agent Context

The verified context has the same custom fields as the credential claims. It
exists only in the Agent request context after signature, registered-claim,
audience, header-equality, and replay validation all succeed.

Relationships:

- one credential authorizes exactly one A2A HTTP request;
- one credential references one Workspace, one exact Agent release, one
  capability, one Invocation, one root Task, and one Trace;
- a child credential also references one parent Invocation;
- the Agent Runtime cannot create or alter an Authenticated Agent Context.

## Credential Signing Configuration

### Router-held

| Value | Rule |
| --- | --- |
| issuer | required exact URI; not a discovered endpoint |
| key ID | required safe identifier |
| private key | required 64 raw Ed25519 bytes in unpadded Base64url |
| TTL seconds | required integer from 1 through 300 |

### Agent-held

| Value | Rule |
| --- | --- |
| issuer | required exact match with Router |
| audience | required canonical endpoint origin |
| key ID | required exact match with credential header |
| public key | required 32 raw Ed25519 bytes in unpadded Base64url |

Configuration is deployment data, not an Agent Card, Registry, Workspace, or
Ledger entity. Invalid values prevent serving execution traffic.

## Replay Entry

| Field | Type | Rule |
| --- | --- | --- |
| `jti` | identifier | map key from a fully verified credential |
| `expiresAt` | UTC instant | equal to credential `exp` |

State transitions:

```text
absent --atomic successful validation--> live
live --duplicate presentation--> rejected (entry unchanged)
live --now >= expiresAt--> removed
```

No fallback state, durable state, degraded mode, or cross-process merge exists.

## Agent Authentication Error v1

| Field | Type | Rule |
| --- | --- | --- |
| `code` | enum | `UNAUTHENTICATED` or `FORBIDDEN` |
| `message` | string | fixed generic message for the code |

It contains no trace, invocation, claim, token, key ID, signature, or internal
validation detail because no request context is trusted until authentication
succeeds.
