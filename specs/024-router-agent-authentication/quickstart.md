# Quickstart: Validate Router-to-Agent Authentication

## Prerequisites

- Go 1.26
- Docker Engine and Docker Compose for the real acceptance flow
- An Ed25519 key pair encoded as unpadded Base64url
- Every existing required local deployment variable from `.env.example`

Set the new values explicitly. The example names below are placeholders, not
defaults or production credentials:

```text
NEKIRO_ROUTER_AGENT_CREDENTIAL_ISSUER=https://a2a-router.nekiro.dev
NEKIRO_ROUTER_AGENT_CREDENTIAL_KEY_ID=<safe-key-id>
NEKIRO_ROUTER_AGENT_CREDENTIAL_PRIVATE_KEY_BASE64URL=<64-byte-private-key>
NEKIRO_ROUTER_AGENT_CREDENTIAL_TTL_SECONDS=30
NEKIRO_AGENT_ROUTER_ISSUER=https://a2a-router.nekiro.dev
NEKIRO_AGENT_ROUTER_KEY_ID=<same-safe-key-id>
NEKIRO_AGENT_ROUTER_PUBLIC_KEY_BASE64URL=<32-byte-public-key>
```

Compose configures each sample Agent's exact audience from its canonical
container endpoint origin. Missing or malformed values must fail config/startup
checks rather than use a test key or anonymous mode.

## Contract and unit validation

```powershell
go test ./contracts/...
go test ./apps/a2a-router/internal/credential ./apps/a2a-router/internal/transport/a2a
go test ./sdks/agent-sdk/routerauth
go test -race ./apps/a2a-router/internal/credential ./sdks/agent-sdk/routerauth
```

Expected outcomes:

- the contract corpus accepts only the exact v1 header/claims shape;
- signing produces distinct `jti` values and a maximum 300-second lifetime;
- direct, forged, expired, wrong-issuer, wrong-audience, replayed, and context-
  mismatched requests are rejected before the test runtime handler runs;
- concurrent replay executes the guarded handler at most once.

## Sample Runtime validation

```powershell
go test ./agents/runtime-b/...
Push-Location agents/runtime-a
go test -count=1 ./...
go test -race ./...
Pop-Location
```

Expected outcomes:

- readiness remains available without an execution credential;
- ownership proof remains handled by the existing challenge adapter;
- A2A execution is protected in both Runtimes;
- Runtime A still uses the Agent SDK to call the Router for a nested child.

## Compose configuration validation

```powershell
docker compose --env-file .env --file deploy/compose.yaml config --quiet
```

The command succeeds only when Router signing and both Agent verification
settings are present and exact.

## Real Invoke-to-Record acceptance

```powershell
docker compose --env-file .env --file deploy/compose.yaml up --build --detach --wait
go test -tags=e2e -count=1 ./tests/e2e/invoke-record
docker compose --env-file .env --file deploy/compose.yaml down --volumes --remove-orphans
```

The suite must demonstrate:

1. `Register -> Verify -> Publish -> Install -> Invoke -> Record` succeeds.
2. Direct unauthenticated Agent execution returns 401.
3. JSON and SSE calls reach Runtime B with distinct credentials.
4. Runtime A parent and Runtime B child hops are both authenticated and retain
   one root Task/Trace lineage.
5. Failure and cancellation Ledger semantics remain unchanged.
6. Logs and database secrecy scans contain no credential, key, signature, or
   `jti` material.

## No fallback assertion

Delete or corrupt each required Router/Agent auth variable in an isolated
config test. The owning process must fail. It must not switch to `none`, a
built-in key, another audience, localhost, a second key source, or a degraded
execution mode.
