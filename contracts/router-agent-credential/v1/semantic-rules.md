# Router Invocation Credential v1 Semantic Rules

## Protected JWS header

The compact JWS protected header MUST contain exactly `alg`, `typ`, and `kid`.
`alg` MUST equal `EdDSA`; `typ` MUST equal `nekiro-router+jwt`; and `kid` MUST
equal the verifier's explicitly configured safe identifier. Unknown or
duplicate members are invalid. Compact segments MUST use unpadded Base64url
with zero trailing padding bits.

## Claims

Claims MUST pass `router-agent-credential.v1.schema.json` and these additional
rules:

- `iss` is the verifier's exact configured Router issuer. The issuer value is
  a canonical HTTPS origin (no path, query, fragment, userinfo, or default
  port); this is part of the language-neutral profile, not a Go-only rule.
- `aud` contains exactly the verifier's one canonical HTTP(S) endpoint origin.
- `iat <= now < exp`, `exp > iat`, and `exp - iat <= 300` seconds.
- `jti` is accepted once by one Agent process during its live validity window.
- Every custom claim exactly equals the corresponding single-valued HTTP
  context header.
- `parentInvocationId` and `x-nek-parent-invocation-id` are either both absent
  for a root call or both present and equal for a child call.

No missing value is derived from the body, Host, Agent Card, another claim,
another credential, or a default.

## HTTP errors

Missing, malformed, forged, expired, wrong-issuer, unknown-key, and replayed
credentials are HTTP 401 `UNAUTHENTICATED`. A valid signature for a wrong
audience or mismatched context is HTTP 403 `FORBIDDEN`. Error bodies conform to
`#/$defs/authenticationError` and expose no validation detail.

## Secrecy

The compact credential, signature, key material, and `jti` are transient and
MUST NOT enter Agent Card, public response fields other than the fixed error,
logs, events, Ledger projection, or persistent platform storage.
