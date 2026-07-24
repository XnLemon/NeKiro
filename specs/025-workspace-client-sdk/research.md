# Research: Workspace Client SDK

## Existing repository facts

- Gateway already serves `POST /v4/workspaces/{workspaceId}/invocations` and is
  the only authorized application-to-Agent path.
- Gateway authenticates one opaque Bearer as a caller principal, applies the
  existing Owner-only Workspace policy, creates root correlation, and sends
  the resolved exact Release through Router Internal dispatch v4.
- Northbound Invocation v4 requires exact JSON/SSE media negotiation and uses
  Invocation Result v1, Result Stream Event v2, and Platform Error v4.
- The existing Agent SDK targets `/agent/v1/invocations` with trusted parent
  context and an Agent-bound Router credential. Its direction, request fields,
  authentication, and lineage responsibilities are not valid for application
  calls.
- Runtime contract validators already enforce Platform Error v4 messages and
  shapes, Result Stream Event v2 correlation/sequence, and terminal completion.
  The Client SDK can consume them without importing any service internals.
- Gateway currently writes `x-nek-trace-id` for every invocation response and
  can emit `500 INTERNAL_ERROR`; the v4 OpenAPI omitted those declarations.
- Gateway currently replaces its generated northbound Trace when Router sends
  any non-empty Trace header, while the Router client validates only media.
  This permits an internal missing/duplicate/mismatched Trace to escape the
  boundary instead of failing correlation.
- Router's error status switch omits an explicit `INTERNAL_ERROR` case and
  therefore maps it through a default 503 unavailable branch even though the
  Platform Error v4 semantic is internal failure.

## Decision 1: Consume and correct Northbound Invocation v4

**Decision**: Keep the existing v4 routes and wire schemas. Correct Northbound
and Router Internal OpenAPI responses to require the emitted Trace header and
declare 500 `INTERNAL_ERROR`. Require the Control Plane Router client to accept
exactly one response Trace equal to its dispatch Trace, retain the
Gateway-created northbound value, and map Router `INTERNAL_ERROR` explicitly to
500 before implementing the SDK.

**Rationale**: SDK result/error correlation requires a normative Trace header,
and the dispatch request already carries the Gateway-created Trace through
Router. Platform Error v4 already defines `INTERNAL_ERROR` as distinct from
dependency failure. The correction changes no accepted request shape and does
not justify a second runtime route or dual-version policy.

**Alternatives considered**:

- Add v5 only for the header declaration: rejected because no wire behavior
  changes and it would create unnecessary runtime duplication.
- Accept a missing header in the SDK: rejected because it weakens Trace
  correlation and invents a fallback absent from the platform contract.
- Trust result/error body Trace without the response header: rejected because
  all other platform HTTP adapters validate both sources.
- Forward any non-empty Router Trace or let `INTERNAL_ERROR` use default 503:
  rejected because both behaviors compress an internal contract violation into
  a different authoritative fact or failure category.

## Decision 2: Bind one Client to one Workspace and Owner credential

**Decision**: `Config` carries one Gateway origin, Workspace ID, opaque Bearer,
explicit HTTP client, and byte limits. The per-call model has exactly Agent ID,
capability, and raw JSON input. Phase 1 uses an out-of-band credential mapped by
Gateway to the existing Workspace Owner.

**Rationale**: This matches Issue #51, the prior Owner-only decision, and the
Workspace authorization boundary. It prevents business code from choosing a
different Workspace with the same client or supplying trusted routing facts.

**Alternatives considered**:

- Put Workspace and credential on every request: rejected because they are
  security context, not Agent business input, and would allow accidental
  cross-Workspace use.
- Add application-credential tables and issuance APIs: rejected as a separate
  lifecycle/RBAC feature not approved by #51.
- Derive Workspace or credential from environment inside the SDK: rejected
  because required security configuration must be explicit to the application.

## Decision 3: Use a dedicated Client SDK package

**Decision**: Create `sdks/client-sdk` with package name `clientsdk`. It imports
root `contracts` only and does not depend on `sdks/agent-sdk`.

**Rationale**: Both SDKs parse related result contracts, but their caller trust,
credentials, paths, request fields, and correlation sources differ. A direct
dependency would blur the constitution's application-to-Gateway and
Agent-to-Router directions. A shared abstraction is not yet stable enough to
justify moving code.

**Alternatives considered**:

- Extend Agent SDK with a mode flag: rejected because caller classes and
  credentials must not be interchangeable.
- Refactor both SDKs onto a new generic transport engine now: rejected because
  it expands change risk before a second stable API proves the abstraction.
- Generate a broad OpenAPI client: rejected because Issue #51 needs only
  invocation and strict lifecycle validation, not the full Control Plane API.

## Decision 4: Preserve input bytes with `json.RawMessage`

**Decision**: The public `InvokeRequest.Input` is `json.RawMessage`. Validation
requires one duplicate-free JSON object; wire encoding adds only the three
business fields plus the method-selected `stream` boolean.

**Rationale**: `map[string]any` can normalize number spelling and adds an
unnecessary application model. Raw JSON preserves contract-valid input while
the SDK still rejects missing, scalar, array, null, malformed, and duplicate
objects before network I/O.

**Alternatives considered**:

- Expose `map[string]any`: rejected because it loses exact JSON numeric/text
  representation and forces callers into one generic data model.
- Accept arbitrary JSON values: rejected because the active Gateway contract
  requires an object.

## Decision 5: Method-selected result mode

**Decision**: `Invoke` always sends `stream=false` and `Accept:
application/json`; `InvokeStream` always sends `stream=true` and `Accept:
text/event-stream`. The public request has no stream flag.

**Rationale**: A method/result-type mismatch becomes impossible, and the exact
active media matrix is visible in the API.

**Alternatives considered**:

- One method returning a union based on `Stream`: rejected because it permits
  invalid method/mode combinations and complicates resource ownership.
- Accept wildcard JSON media from Gateway responses: rejected; wildcards are
  request negotiation values, not valid response Content-Types.

## Decision 6: Strict bounded response validation

**Decision**: Require explicit full-request, JSON/error-response, and per-SSE
frame limits. Validate duplicate/unknown/trailing members, exact media, exact
one-value Trace header, language-neutral schemas, fixed error messages,
status/code/phase mapping, result/header correlation, and stream sequence. A
stream is clean only after a terminal event is followed by actual EOF; Close
before that point records and returns interruption, including Close immediately
after terminal.

**Rationale**: The SDK is a trust adapter at the application boundary. Returning
partially decoded or unbounded data would make application behavior depend on
malformed Gateway/Router output and would bypass contract guarantees.

**Alternatives considered**:

- Choose SDK size defaults: rejected because deployment policy is not specified
  and the repository explicitly forbids invented defaults.
- Return raw error bytes for debugging: rejected because raw bodies may contain
  secrets or dependency detail and force every application to reimplement the
  contract.
- Treat EOF after chunks as success: rejected because the active stream
  contract requires a terminal event.
- Treat a read terminal followed by immediate Close as clean: rejected because
  it prevents detection of a forbidden post-terminal frame.

## Decision 7: One request, caller-owned cancellation, no redirect

**Decision**: Clone the required caller-supplied `*http.Client`, replace its
redirect hook with rejection, bind every request to the supplied context, and
perform no retry, alternate route, or Ledger recovery.

**Rationale**: Invocations can have side effects. Retrying or following a
redirect could repeat work or leak the application credential to another
origin. Context and HTTP client configuration already provide the application
policy boundary.

**Alternatives considered**:

- Use `http.DefaultClient` when none is supplied: rejected as an undocumented
  transport/timeout fallback.
- Retry idempotent-looking transport failures: rejected because Agent
  invocation idempotency is not defined.
- Follow same-origin redirects: rejected because the Gateway contract has one
  exact route and no redirect policy.

## Error matrix

| HTTP | Allowed codes | Required phase |
| --- | --- | --- |
| 400 | `VALIDATION_ERROR` | pre-correlation |
| 401 | `UNAUTHENTICATED` | pre-correlation |
| 403 | `FORBIDDEN`, `CAPABILITY_NOT_ALLOWED` | pre-correlation |
| 404 | `NOT_FOUND`, `AGENT_NOT_INSTALLED` | pre-correlation |
| 406 | `NOT_ACCEPTABLE` | pre-correlation |
| 409 | `CONFLICT`, `INSTALLATION_DISABLED`, `AGENT_DISABLED`, `AGENT_RELEASE_UNPUBLISHED`, `AGENT_RELEASE_SUSPENDED`, `AGENT_RELEASE_REVOKED`, `CANCELED` | pre or correlated |
| 413 | `PAYLOAD_TOO_LARGE` | pre-correlation |
| 500 | `INTERNAL_ERROR` | pre or correlated |
| 502 | `AGENT_AUTH_UNSUPPORTED`, `AGENT_RESPONSE_TOO_LARGE`, `AGENT_EXECUTION_FAILED`, `A2A_PROTOCOL_ERROR` | correlated |
| 503 | `ROUTE_NOT_FOUND`, `AGENT_UNAVAILABLE`, `DEPENDENCY_ERROR` | pre or correlated |
| 504 | `TIMEOUT` | pre or correlated |

## Fallback audit

Fallback delta: removed 2, retained 1, added 0, net -2.

Retained policy: a non-nil caller-supplied `http.Client` may use Go's documented
nil-`Transport` behavior. The SDK itself supplies no client, timeout, origin,
Workspace, credential, limit, retry, redirect, response, or version fallback.

Removed policies: Gateway no longer treats a missing/different Router Trace as
an optional source-selection branch, and Router no longer maps
`INTERNAL_ERROR` through its default 503 unavailable branch.

Added fallback evidence: none.
