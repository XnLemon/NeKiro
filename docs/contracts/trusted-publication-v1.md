# Trusted Publication v1

Trusted Publication v1 adds a Registry-owned proof step before an endpoint can
participate in a verified Agent Release. It does not deploy the Agent and it
does not permit Console or client applications to call the Agent directly.

## Provider flow

1. Register Agent Card v0.2 through the existing Catalog API.
2. Create an endpoint binding with the authenticated provider ID, Agent ID,
   exact registered Card version, endpoint, and method `http_well_known`.
3. Create a single-use challenge. The response returns the proof exactly once.
4. Serve that exact proof at the returned
   `/.well-known/nekiro/challenges/{challengeId}` URL.
5. Complete the challenge. A successful response exposes the binding status
   and SHA-256 evidence digest, never the proof.

Creating a verified endpoint binding does not publish an Agent Release. The
release lifecycle endpoints create an immutable release, verify a pending
binding, publish the exact verified release, and suspend/revoke without
changing any bound fact.

Workspace installation copies the exact published Release ID. The Catalog
migration marks every pre-v4 published row, including the Phase 1 samples, as
`legacy_unverified`; newly registered or old-API-published versions without a
Release cannot use the legacy installation path. For a trusted installation,
version resolution evaluates the highest SemVer match for the requested
constraint and does not silently downgrade when that release fails the trust
gate.
Gateway Dispatch, Router lifecycle events, and the append-only Ledger persist
the Release ID and canonical Card SHA-256 digest as metadata. These fields are
optional only for the explicitly pre-v4 legacy compatibility path and are
always omitted as a pair there. The absent pair is the explicit
legacy/unverified encoding for existing Invocation Event 0.3 payloads; Catalog
still requires its internal `legacy_unverified` marker before creating one.
Control Plane exact resolution returns the Catalog-owned pair, and Router
rejects an omitted or mismatched dispatch pair before Agent transport or Ledger
writes. Router does not recompute the digest from normalized Card response JSON.

Workspace and invocation boundaries expose Release gate failures without raw
storage detail: `AGENT_RELEASE_UNPUBLISHED`, `AGENT_RELEASE_SUSPENDED`, and
`AGENT_RELEASE_REVOKED` remain distinct from `INSTALLATION_DISABLED` and
`AGENT_DISABLED`.

## Network policy

Verification accepts only `http` or `https` endpoints without credentials,
query strings, or fragments. Redirects are rejected. DNS is resolved once,
every returned address is checked, and the request is pinned to an approved
address to prevent a second resolution from bypassing the policy.

Loopback, private, link-local, multicast, and unspecified addresses are denied
unless their hostname appears in the explicit
`NEKIRO_ENDPOINT_ALLOWED_PRIVATE_HOSTS_JSON` configuration. The allowlist has
no implicit localhost or development value.

The following settings are required:

- `NEKIRO_ENDPOINT_CHALLENGE_TTL_SECONDS`
- `NEKIRO_ENDPOINT_VERIFICATION_TIMEOUT_MS`
- `NEKIRO_ENDPOINT_ALLOWED_PRIVATE_HOSTS_JSON` (use `[]` to deny all private
  hosts)

The verification timeout must be shorter than the challenge TTL.

The Compose Sample Agents expose the same well-known route from an explicitly
configured `NEKIRO_AGENT_CHALLENGE_DIRECTORY`. The operator writes the
exact one-time proof to a file named exactly after `challengeId`, without
adding a trailing newline, and removes it after challenge completion. This
Agent-side directory has no default; a missing, blank, relative, or malformed
path fails Sample Agent startup instead of silently selecting storage.

## Failure semantics

Failure responses use the trusted-publication error contract. Invalid endpoint,
wrong proof, and redirect are distinct typed failures; disallowed network,
unknown resources, challenge expiry/reuse, endpoint unavailability, and
dependency failure each retain their own public code. The exact codes are
`INVALID_ENDPOINT`, `DISALLOWED_NETWORK`, `ENDPOINT_UNAVAILABLE`,
`WRONG_PROOF`, `CHALLENGE_EXPIRED`, `CHALLENGE_REUSED`,
`REDIRECT_NOT_ALLOWED`, and `DEPENDENCY_ERROR`.

Failure responses include the platform trace ID but never proof values,
dependency details, tokens, or endpoint credentials.

## Source contracts

- `contracts/schemas/trusted-publication.v1.schema.json`
- `contracts/openapi/trusted-publication.v1.yaml`
