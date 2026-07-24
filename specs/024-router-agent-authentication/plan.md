# Implementation Plan: Router-to-Agent Authentication

**Branch**: `codex/router-agent-auth` | **Date**: 2026-07-22 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/024-router-agent-authentication/spec.md`

## Summary

Add a versioned Router Invocation Credential profile for every managed
Router-to-Agent A2A HTTP request. The Router signs one Ed25519 JWT per HTTP
request from already validated dispatch/release context. A thin Agent SDK
adapter strictly validates the compact token, exact issuer/audience/key,
lifetime, single-use `jti`, and equality with delivered platform context
headers before either sample Runtime executes the A2A request. The current
JSON, SSE, cancellation, nested invocation, and metadata-only Ledger paths
remain unchanged after authentication succeeds.

## Technical Context

**Language/Version**: Go 1.26.0 for Router, contracts, Agent SDK, and both sample adapters

**Primary Dependencies**: Go standard `crypto/ed25519`; `github.com/golang-jwt/jwt/v5` v5.3.1; existing `github.com/a2aproject/a2a-go` v0.3.15

**Storage**: No platform persistence. Agent-local synchronized in-memory `jti -> expiresAt` replay window only.

**Testing**: Go unit/contract/race/vet suites, Runtime A nested-module suite, real Docker Compose/PostgreSQL E2E

**Target Platform**: Linux server/container processes; local Windows/Linux Go tests

**Project Type**: Multi-process platform with a Go Data Plane and independently implemented sample Agent Runtimes

**Performance Goals**: One local Ed25519 signature and verification per outbound A2A HTTP request, with no new network round trip or database write

**Constraints**: Maximum credential lifetime 300 seconds; exact zero-leeway time validation; one active key ID; no anonymous, provider-key, key-discovery, retry, or compatibility fallback

**Scale/Scope**: One Router signer and one verifier/replay window per sample Agent process; JSON, SSE, cancellation, and nested paths in the existing Phase 1 acceptance stack

## Constitution Check

### Pre-design gate

- **Phase 1 loop**: PASS. Authentication makes the existing Invoke step and
  trusted Release-to-Record chain enforceable rather than adding unrelated
  platform surface.
- **Ownership**: PASS. Router owns issuance and transport; Agent adapter owns
  verification/replay; Registry continues to own release facts; Ledger remains
  metadata-only and never receives a token.
- **Runtime independence**: PASS. The credential is an A2A HTTP adapter
  contract used by Runtime A and Runtime B without importing either Runtime
  framework into Router core.
- **Contracts**: PASS. `router-agent-credential.v1` JSON Schema, semantic
  profile, header/claim conformance cases, and Go mapping precede
  implementation. The existing Agent Card v0.2 `http_bearer` enum is used;
  Card schema version does not change.
- **Invocation lineage**: PASS. Credential claims bind existing invocation,
  root task, optional parent, and trace identifiers; Ledger lifecycle DTOs are
  unchanged.
- **Failure safety**: PASS. 401 authentication and 403 authorization failures
  remain distinct; configuration is required and exact; secrets do not enter
  response, logs, Card, event, or storage models.
- **SDD traceability**: PASS. Tasks map to FR/SC and tests follow the approved
  implementation.
- **Cross-runtime proof**: PASS. Both sample adapters enforce the profile and
  Runtime A's nested call causes a separately authenticated Runtime B hop.

### Post-design gate

PASS with no exception or complexity waiver. Research resolves algorithm,
claim shape, audience, lifetime, replay, configuration, and error semantics.
The design introduces no new service or persistence owner.

## Design

### Credential contract

- Compact JWS with protected header exactly `alg`, `typ`, and `kid`.
- `alg` is exactly `EdDSA`; `typ` is exactly `nekiro-router+jwt`; `kid` is the
  explicit configured signing identity.
- Claims are the registered `iss`, one-element `aud`, `exp`, `iat`, `jti` plus
  `workspaceId`, `agentId`, `agentVersion`, `releaseId`, `cardDigest`,
  `capability`, `invocationId`, `rootTaskId`, optional `parentInvocationId`,
  and `traceId`.
- Header and claims objects reject duplicate and unknown members. Compact
  segments use unpadded Base64url with strict trailing-bit validation.
- `exp` is later than `iat`, `exp - iat` is at most 300 seconds, and the token
  is expired when `now >= exp`. No leeway is configured.

### Issuance boundary

`apps/a2a-router/internal/credential` receives only typed config and a trusted
`contracts.RouterInvocationCredentialContextV1` context. The transport derives
that context from `DispatchInvocationRequestV4`, the validated resolved Card,
and canonical endpoint origin. A dynamic A2A call interceptor mints a fresh
token immediately before each protocol HTTP request, so streaming cancellation
never reuses the streaming request's `jti`.

The transport adds `Authorization: Bearer <JWT>` and versioned context headers
for target Agent/version, Release/digest, capability, root/parent lineage, and
existing Workspace/invocation/trace identifiers. Required headers have exactly
one value. The parent header is omitted for a root invocation rather than
sent as an empty value.

### Agent validation boundary

`sdks/agent-sdk/routerauth` is a small protocol adapter, not a Runtime. It:

1. loads one exact Ed25519 public key, issuer, audience, and key ID;
2. strictly parses one Bearer credential;
3. validates signature, registered claims, maximum lifetime, identifiers, and
   digest format;
4. compares every bound claim with exactly one corresponding context header;
5. atomically registers `jti` in a process-local replay set until `exp`;
6. stores verified claims only in request context and calls the Runtime handler.

401/403 bodies use the language-neutral v1 Agent authentication error schema
with only stable code/message fields. They do not echo token, claim, header, or
cryptographic failure details. Readiness and ownership-challenge routes remain
outside this execution middleware.

### Configuration contract

Router serving requires:

- `NEKIRO_ROUTER_AGENT_CREDENTIAL_ISSUER`
- `NEKIRO_ROUTER_AGENT_CREDENTIAL_KEY_ID`
- `NEKIRO_ROUTER_AGENT_CREDENTIAL_PRIVATE_KEY_BASE64URL`
- `NEKIRO_ROUTER_AGENT_CREDENTIAL_TTL_SECONDS`

Each Agent adapter requires:

- `NEKIRO_AGENT_ROUTER_ISSUER`
- `NEKIRO_AGENT_ROUTER_AUDIENCE`
- `NEKIRO_AGENT_ROUTER_KEY_ID`
- `NEKIRO_AGENT_ROUTER_PUBLIC_KEY_BASE64URL`

The private key is exactly 64 raw Ed25519 bytes and the public key is exactly
32 raw bytes, both encoded as unpadded Base64url. Secret/key strings are never
trimmed or repaired. Missing, blank, padded, wrong-length, or otherwise
malformed values fail startup. Audience is the canonical trusted endpoint
origin, so multiple logical fixtures on one verified origin share one
recipient while their exact Agent/release claims remain separately bound.

### Replay and concurrency

Only a credential that has passed signature, registered-claim, audience, and
context equality validation enters the replay set. Under one mutex, expired
entries are removed and a live duplicate `jti` is rejected before insertion.
Thus concurrent presentations produce at most one runtime execution. No
durable, cross-process, or replicated replay behavior is invented.

### Compatibility

- Agent Card stays at v0.2 and uses its existing `http_bearer` value. Trusted
  sample Cards change from `none` to `http_bearer`; `none` is no longer an
  executable managed transport path for the sample acceptance release.
- Router Internal dispatch moves to v4 because the prior `none` execution
  semantics are retired; the existing metadata reads remain on Router
  Internal v3. Northbound Invocation v4, Invocation Event v0.3, Result/Stream,
  and Ledger schemas remain unchanged.
- A2A Profile Schema remains 0.2 and protocol remains 0.3.0. The platform
  credential is a new v1 companion contract whose own conformance corpus
  versions the complete signed context header set. Existing Profile 0.2
  `contextHeaders` are not mutated in place.
- Agent→Router nested bearer bindings remain opaque Workspace/Agent
  credentials and are not replaced by Router→Agent JWTs in this feature.

## Fallback Inventory

| Existing behavior | Classification | Decision | Evidence |
| --- | --- | --- | --- |
| `http.Client.Transport == nil` uses Go's documented default transport | Valid platform-library policy | Keep unchanged | Go `http.Client` contract and existing transport tests |
| One bounded streaming cancel attempt does not replace the already committed local timeout/cancel result | Valid failure policy | Keep unchanged | ADR 0006 and existing streaming tests |
| Agent Card `none` reaches a trusted sample execution endpoint | Anonymous execution path, incompatible with FR-013 | Remove from executable trusted sample path | Issue #50 and Spec 024 |

No default issuer, audience, key, TTL, identity, endpoint, anonymous mode,
retry, legacy token, alternate key source, or degraded result is introduced.

## Project Structure

### Documentation (this feature)

```text
specs/024-router-agent-authentication/
|-- spec.md
|-- clarify.md
|-- plan.md
|-- research.md
|-- data-model.md
|-- quickstart.md
|-- contracts/
|   `-- router-agent-credential.v1.md
|-- checklists/
|   `-- requirements.md
`-- tasks.md
```

### Source Code (repository root)

```text
contracts/
|-- openapi/router-internal.v4.yaml
|-- openapi/router-metadata.v3.yaml
|-- schemas/router-agent-credential.v1.schema.json
|-- router-agent-credential/v1/semantic-rules.md
|-- router-agent-credential/v1/conformance/
|-- router_agent_credential.go
`-- router_agent_credential_contracts_test.go

apps/a2a-router/
|-- internal/config/
|-- internal/credential/
|-- internal/transport/a2a/
`-- cmd/a2a-router/

sdks/agent-sdk/
`-- routerauth/

agents/
|-- runtime-a/
`-- runtime-b/

tests/e2e/invoke-record/
deploy/compose.yaml
.github/workflows/ci.yml
.env.example
docs/decisions/0007-router-agent-signed-credential.md
docs/runbooks/local-development.md
```

**Structure Decision**: Keep issuance inside Router's Data Plane, reusable
verification inside the lightweight Go Agent SDK adapter, and runtime-specific
HTTP wiring inside each sample Agent. No Control Plane import, shared database,
or Agent framework dependency is added.

## Complexity Tracking

No constitution violation requires justification.

## Verification Commands

```text
go test ./contracts/... ./apps/a2a-router/... ./sdks/agent-sdk/...
go test -race ./apps/a2a-router/... ./sdks/agent-sdk/...
go vet ./...
go test -count=1 ./agents/runtime-a/...
go test -race ./agents/runtime-a/...
docker compose --file deploy/compose.yaml config --quiet
go test -tags=e2e -count=1 ./tests/e2e/invoke-record
```

Fallback target: removed 1, retained 2, added 0, net -1.
Added fallback evidence: none.
