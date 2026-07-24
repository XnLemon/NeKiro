# Research: Router-to-Agent Authentication

## Existing repository facts

- Router transport already owns all outbound A2A JSON/SSE/cancellation HTTP
  calls and receives the exact trusted release ID/Card digest established by
  Spec 023.
- Current A2A context headers carry Workspace, invocation, root task, optional
  parent, and trace identifiers, but they are not authenticated.
- The existing opaque `NEKIRO_ROUTER_AGENT_PRINCIPALS_JSON` boundary authenticates
  Runtime A when it calls the Router for a nested invocation. It is the opposite
  direction and must remain separate.
- Both sample Agents expose their A2A JSON-RPC handler without an execution
  authentication middleware. The ownership challenge and readiness routes
  have separate purposes and must remain reachable.
- Agent Card v0.2 already declares `http_bearer`; no Card schema expansion is
  needed to state that the managed execution endpoint requires Bearer auth.

## Decision 1: Ed25519-signed compact JWT

**Decision**: Use compact JWT/JWS with `alg=EdDSA` backed by Ed25519. Router
holds the 64-byte private key; Agents hold only the 32-byte public key.

**Rationale**: Asymmetric trust prevents a compromised Agent from minting
Router credentials for another Agent. Ed25519 is standardized for JOSE by RFC
8037, has small deterministic signatures, and is available in Go's standard
library. `github.com/golang-jwt/jwt/v5` v5.3.1 supplies maintained JWT/JWS
parsing and EdDSA verification while strict profile validation remains owned
by NeKiro.

**Alternatives considered**:

- HMAC: rejected because every verifying Agent would receive a signing secret.
- RSA: secure but larger keys/tokens and no benefit for this first profile.
- Custom non-JWT signature envelope: rejected because Issue #50 explicitly
  requires short-lived JWT credentials.

## Decision 2: Exact protected header and claims object

**Decision**: Require only `alg`, `typ`, `kid` in the protected header and the
claims enumerated by Spec FR-002. Reject duplicate and unknown members before
library validation; require unpadded strict Base64url.

**Rationale**: JWT libraries correctly verify signatures but generic JWT
extensibility can create algorithm, duplicate-member, or unsupported-claim
ambiguity at a trust boundary. A closed v1 profile is language-neutral and
testable. The parser allowlist pins `EdDSA`, exact `kid`, exact issuer, required
expiry, and issued-at validation.

**Alternatives considered**:

- Accepting arbitrary JWT headers/claims: rejected because this creates
  unaudited compatibility behavior.
- Accepting padded or non-strict Base64: rejected; RFC 7515 compact JWS uses
  unpadded Base64url and no legacy producer exists.

## Decision 3: Canonical endpoint origin is the audience

**Decision**: `aud` contains exactly one canonical trusted endpoint origin.
The Agent adapter is explicitly configured with that same audience. Exact
logical Agent, version, release, digest, and capability remain separate claims
and headers.

**Rationale**: Audience must identify the recipient that can validate the
credential. Origin works for a deployment endpoint and permits the existing
Runtime B acceptance fixture to host multiple logical Agent Cards on the same
verified origin without weakening their exact per-invocation bindings.

**Alternatives considered**:

- Agent ID as audience: rejected because Runtime B intentionally hosts
  multiple acceptance Agent IDs at one endpoint.
- Full endpoint path: rejected because the same recipient's A2A and cancel
  routes may differ, while trusted release/path remains bound separately.
- Global platform audience: rejected because a token could be replayed to a
  different Agent endpoint that trusts the same platform key.

## Decision 4: Short lifetime, no leeway, Agent-local one-time `jti`

**Decision**: Router TTL is a required integer from 1 through 300 seconds.
Agent requires `exp > iat`, `exp - iat <= 300 seconds`, and `now < exp`, with no
leeway. A synchronized in-memory replay map atomically accepts a verified
`jti` once and retains it only until expiry.

**Rationale**: This proves replay rejection without introducing a new storage
owner or network dependency. A hard maximum bounds memory and restart risk.
No repository policy authorizes durable coordination, clock-skew forgiveness,
or retries, so those are deferred explicitly.

**Alternatives considered**:

- PostgreSQL/Redis replay storage: rejected as premature persistence and
  availability coupling without a replica topology policy.
- Clock leeway: rejected because no clock-skew allowance is specified.
- Reusing one token for stream and cancel: rejected because `jti` is single-use
  per HTTP request.

## Decision 5: Strict 401/403 split with generic bodies

**Decision**: Missing/malformed/forged/expired/wrong-issuer/unknown-key/replayed
credentials return 401 `UNAUTHENTICATED`. A valid signature for the wrong
audience or mismatched context returns 403 `FORBIDDEN`. Bodies include only a
stable code and generic message; 401 adds `WWW-Authenticate: Bearer`.

**Rationale**: The distinction is required by Issue #50 while generic bodies
avoid key, token, claim, and validation-oracle leakage. Rejection precedes the
runtime handler.

**Alternatives considered**:

- One 401 for all failures: rejected because wrong recipient/context is an
  authenticated authorization failure.
- Detailed crypto error bodies: rejected by secret and internal-detail safety.

## Decision 6: Official Agent SDK adapter, not Runtime logic

**Decision**: Implement validation/replay as `sdks/agent-sdk/routerauth` and
wire it at the HTTP execution route of both sample Agents. Router issuance
stays in `apps/a2a-router/internal/credential`.

**Rationale**: Agent-side verification is protocol adaptation and context
propagation, which belongs in the lightweight Agent SDK. It remains independent
of models, tools, workflows, memory, and either Runtime framework. The Router
does not import SDK internals and both directions depend only on contracts.

**Alternatives considered**:

- Put verification inside Runtime handlers: rejected because it duplicates
  security logic and promotes platform behavior into a Runtime.
- Put signing and verification in one shared service package: rejected because
  it would blur Router/Agent dependency direction and key ownership.

## Fallback classification

- **Remove**: anonymous `authentication.type=none` from newly trusted sample
  execution Cards/transport.
- **Keep**: Go's documented nil-Transport behavior in cloned HTTP clients.
- **Keep**: ADR 0006's one bounded, non-promoting remote cancel attempt after a
  local timeout/cancel outcome.
- **Needs policy**: none in this feature. Rotation, remote key discovery,
  durable replay, and clock leeway are excluded instead of implemented as
  guessed fallback.

Fallback target: removed 1, retained 2, added 0, net -1.
Added fallback evidence: none.
