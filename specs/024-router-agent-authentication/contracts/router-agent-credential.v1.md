# Router Invocation Credential v1

## Scope

This contract authenticates the Router-to-Agent HTTP hop for one managed A2A
protocol request. It does not authenticate Agent-to-Router nested requests and
does not replace Workspace authorization, exact release resolution, or Ledger
facts.

## Compact serialization

The credential MUST be a three-segment compact JWS using unpadded Base64url.
Protected header and claims JSON objects MUST reject duplicate members,
unknown members, non-integer NumericDate values, padding, non-zero trailing
bits, empty segments, and extra segments.

Protected header:

```json
{
  "alg": "EdDSA",
  "typ": "nekiro-router+jwt",
  "kid": "router-signing-key-1"
}
```

Claims example:

```json
{
  "iss": "https://a2a-router.nekiro.dev",
  "aud": ["https://agent.example"],
  "exp": 1784700060,
  "iat": 1784700030,
  "jti": "rtj_0123456789abcdef0123456789abcdef",
  "workspaceId": "workspace-a",
  "agentId": "agent-a",
  "agentVersion": "1.2.3",
  "releaseId": "release-a",
  "cardDigest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "capability": "agent.execute",
  "invocationId": "inv_0123456789abcdef0123456789abcdef",
  "rootTaskId": "task-a",
  "traceId": "trace-a"
}
```

`parentInvocationId` is omitted for a root invocation and required for a child
invocation. Empty strings and `null` are invalid for every claim.

## HTTP binding

The Router sends exactly one `Authorization: Bearer <credential>` header and
exactly one of each required context header:

| Header | Claim |
| --- | --- |
| `x-nek-workspace-id` | `workspaceId` |
| `x-nek-target-agent-id` | `agentId` |
| `x-nek-agent-card-version` | `agentVersion` |
| `x-nek-agent-release-id` | `releaseId` |
| `x-nek-agent-card-digest` | `cardDigest` |
| `x-nek-capability` | `capability` |
| `x-nek-invocation-id` | `invocationId` |
| `x-nek-root-task-id` | `rootTaskId` |
| `x-nek-parent-invocation-id` | `parentInvocationId` when present |
| `x-nek-trace-id` | `traceId` |

The Agent MUST reject duplicate headers, missing required headers, a parent
header/claim presence mismatch, or any non-exact value mismatch. It MUST NOT
derive a missing value from the request body, Host header, Card, another claim,
or local fallback.

## Validation and replay

Validation order is:

1. exact Bearer/header cardinality and compact structure;
2. exact protected header and configured key ID;
3. Ed25519 signature and configured issuer;
4. required registered/custom claims and maximum 300-second lifetime;
5. exact configured audience;
6. exact claim/context-header equality;
7. atomic single-use `jti` acceptance.

Only a request that completes step 7 may reach Agent runtime execution. A
fresh token MUST be created for every protocol HTTP request; stream cancellation
does not reuse the stream token.

## Error contract

Missing/malformed/forged/expired/wrong-issuer/unknown-key/replayed credentials:

```http
HTTP/1.1 401 Unauthorized
Content-Type: application/json
Cache-Control: no-store
WWW-Authenticate: Bearer

{"code":"UNAUTHENTICATED","message":"Authentication is required."}
```

Valid signature with wrong audience or bound-context mismatch:

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json
Cache-Control: no-store

{"code":"FORBIDDEN","message":"The managed invocation context is not allowed."}
```

No other error member or validation detail is permitted.

## Compatibility

- This is a new companion contract at version `1`; no previous Router-to-Agent
  credential format is accepted.
- Existing A2A Profile Schema 0.2 context headers remain byte-compatible. The
  credential v1 corpus, not an in-place Profile edit, owns the full signed
  header set.
- Agent Card v0.2 uses existing `authentication.type=http_bearer` for sample
  trusted releases.
- Northbound v4, Router Internal dispatch v4 (metadata reads remain on Router
  Internal v3), Invocation Event v0.3, Result v1, Stream Event v2, and Ledger
  schemas do not add credential fields. Router Internal dispatch v3 is
  historical migration evidence only.
