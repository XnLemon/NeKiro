# Trusted Publication Contract v1 (Slice A)

This is the contract design input for the implementation. JSON Schema and
OpenAPI files are added with the implementation PR and become the runtime
source of truth.

## Create provider binding

`POST /v4/providers/{providerId}/agents/{agentId}/endpoint-bindings`

Request:

```json
{
  "endpoint": "https://agent.example/a2a",
  "method": "http_well_known",
  "version": "1.0.0"
}
```

Response returns `bindingId`, `agentCardVersion`, canonical endpoint,
`verificationStatus`, and `verificationMethod`; it does not return a secret.

## Create challenge

`POST /v4/providers/{providerId}/endpoint-bindings/{bindingId}/challenges`

Response returns a one-time `challengeId`, `challengeUrl`, `expiresAt`, and
the proof exactly once. The proof is not stored or returned by subsequent
reads.

## Complete challenge

`POST /v4/providers/{providerId}/endpoint-bindings/{bindingId}/challenges/{challengeId}/complete`

The Registry performs the exact declared-origin request and returns the
binding state. Typed public failures include `INVALID_ENDPOINT`,
`DISALLOWED_NETWORK`, `ENDPOINT_UNAVAILABLE`, `WRONG_PROOF`,
`CHALLENGE_EXPIRED`, `CHALLENGE_REUSED`, `REDIRECT_NOT_ALLOWED`, and
`DEPENDENCY_ERROR`. The response also includes the exact `agentCardVersion`.

## Read binding

`GET /v4/providers/{providerId}/endpoint-bindings/{bindingId}`

Returns provider, Agent identity, canonical endpoint, method, state, and
timestamps only. Challenge proof and dependency details are omitted.

## Agent Release lifecycle

`POST /v4/providers/{providerId}/agents/{agentId}/releases` creates a Registry
release for an exact Card version and endpoint binding. The binding may be
pending (the release starts `pending_verification`) or verified (the release
starts `verified`). The response includes `releaseId`, exact `agentCardVersion`,
`cardDigest`, provider/binding identity, canonical endpoint origin/path,
verification evidence digest when available, state, and timestamps.

`POST /v4/releases/{releaseId}/verify` re-checks the referenced binding and
transitions `pending_verification` to `verified`.

`POST /v4/releases/{releaseId}/publish` transitions only `verified` to
`published`. `POST /v4/releases/{releaseId}/suspend` transitions a verified or
published release to `suspended`; `POST /v4/releases/{releaseId}/revoke`
transitions a verified, published, or suspended release to terminal `revoked`.
All transitions lock the release and reject illegal or repeated changes with
the typed `CONFLICT` error. The release's Card, endpoint, provider, binding,
and digest fields are never updated in place.

`GET /v4/releases/{releaseId}` returns the exact immutable release record.
Public responses never include challenge proof, endpoint credentials, or raw
dependency errors.
