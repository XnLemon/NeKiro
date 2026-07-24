# Implementation Plan: Workspace Client SDK

**Branch**: `codex/workspace-client-sdk` | **Date**: 2026-07-24 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/025-workspace-client-sdk/spec.md`

## Summary

Add a lightweight Go application Client SDK under `sdks/client-sdk`. One
immutable client binds an explicit HTTP client, canonical Gateway origin,
Workspace ID, opaque Owner credential, and request/response/SSE byte limits.
Business calls supply only Agent ID, capability, and raw JSON-object input.
The SDK sends exactly one Northbound Invocation v4 request, validates JSON or
SSE results and Platform Error v4 responses against existing contract
validators, exposes safe platform correlation, and never accepts routing,
Release, version, Router, or Agent-secret inputs.

The active Northbound and Router Internal v4 OpenAPI documents are corrected
before SDK implementation to declare the `x-nek-trace-id` response header and
the existing `500 INTERNAL_ERROR` outcome. The Control Plane Router adapter
also enforces that the internal response Trace equals the Gateway-created Trace,
and Router maps `INTERNAL_ERROR` explicitly instead of falling through to 503.
No service, persistence, credential lifecycle, wire route, or API version is
added.

## Technical Context

**Language/Version**: Go 1.26.0

**Primary Dependencies**: Go standard `net/http`, `net/url`, `encoding/json`,
`bufio`, and `context`; existing NeKiro result/runtime contract validators

**Storage**: N/A. The credential and configuration remain process-local in the
application-owned Client instance; the SDK performs no persistence.

**Testing**: Go unit and contract tests with `httptest`, compiled Go examples,
race detector, vet, repository lint, and existing root regression suites

**Target Platform**: Go server applications on the existing supported
Linux/Windows development and deployment environments

**Project Type**: Public Go library consuming the Gateway HTTP API

**Performance Goals**: Exactly one Gateway HTTP request per invocation; no new
network round trip or background worker; JSON is bounded in memory and SSE is
processed one bounded frame at a time

**Constraints**: One Gateway origin and Workspace per Client; explicit HTTP
client and byte limits; caller-owned context cancellation/deadline; no retry,
redirect, endpoint switching, Ledger polling, raw error exposure, secret
logging, or compatibility fallback

**Scale/Scope**: One concurrency-safe immutable Client may serve concurrent
application calls through its caller-supplied HTTP client; Phase 1 delivers one
Go SDK, JSON/SSE invocation, typed errors, documentation, and one compiled
application example

## Constitution Check

### Pre-design gate

- **Phase 1 loop**: PASS. The SDK makes the installed Agent directly usable by
  application code and therefore completes the consumer side of `Install ->
  Invoke`.
- **Ownership**: PASS. The SDK owns only local transport adaptation. Gateway
  remains the northbound authentication/authorization boundary, Workspace owns
  Installation policy, Router owns managed execution, and Ledger ownership is
  unchanged.
- **Runtime independence**: PASS. The request uses Agent ID, capability, and
  JSON only and works unchanged for both sample Runtimes.
- **Contracts**: PASS. Northbound Invocation v4, Platform Error v4, Invocation
  Result v1, and Result Stream Event v2 remain language-neutral facts. The v4
  OpenAPI correction precedes the Go public API and implementation.
- **Invocation lineage**: PASS. Gateway continues to create root correlation.
  The SDK validates and returns invocation/root Task/Trace values but cannot
  supply or rewrite them.
- **Failure safety**: PASS. The exact Gateway error matrix remains typed; raw
  bodies and credentials are not exposed. All limits are explicit and no retry,
  redirect, empty-result, or alternate-destination fallback is added.
- **SDD traceability**: PASS. Public types and tests map to FR-001 through
  FR-021 and SC-001 through SC-007; tests follow the approved implementation.
- **Cross-runtime proof**: PASS. One request type and one application example
  target either existing sample Agent through Gateway without Runtime imports.

### Post-design gate

PASS with no exception or complexity waiver. Research resolves the application
credential boundary, URL/transport policy, public API, strict JSON/SSE parsing,
error phase/status mapping, Trace validation, compatibility impact, and
fallback inventory. No unknown policy remains in implementation scope.

## Design

### Public Client boundary

`clientsdk.NewClient(Config)` receives an explicit `*http.Client`, canonical
HTTP(S) Gateway origin, safe Workspace ID, opaque printable Bearer credential,
and request/response/event byte limits. The Client clones the supplied HTTP
client and installs a reject-redirect policy without mutating caller state. Its
fields are immutable after construction and safe for concurrent use.

The per-call `InvokeRequest` contains exactly:

- `AgentID string`
- `Capability string`
- `Input json.RawMessage`

`Invoke` fixes `stream=false`; `InvokeStream` fixes `stream=true`. The SDK does
not expose a stream switch or any Workspace, endpoint, version, Release,
digest, Router, or credential field in the request model.

### Request mapping and transport

The SDK validates platform identifier grammar, requires a duplicate-free JSON
object input, encodes the exact v4 wire object, and applies the configured limit
to the full encoded request before network I/O. It sends one POST to:

```text
{gateway-origin}/v4/workspaces/{configured-workspace-id}/invocations
```

The only generated headers are one opaque Bearer `Authorization`, exact
`Content-Type: application/json`, and exact mode-specific `Accept`. The caller
context owns cancellation/deadline. Redirect responses remain responses and
are rejected as invalid; the SDK never follows them or retries.

### Non-streaming result

For HTTP 200 JSON, the SDK requires exactly one valid `x-nek-trace-id`, exact
`application/json`, bounded duplicate-free JSON with no unknown/trailing
members, Invocation Result v1 validity, and body/header Trace equality. It
returns a small SDK `Result` carrying result content plus invocation/root
Task/Trace fields. It validates the wire-only succeeded status but does not
duplicate that constant in the public result. It never polls Ledger or persists
result content.

### Streaming result

For HTTP 200 SSE, the SDK validates the Trace header before exposing a stream.
`Stream.Recv` reads exactly one bounded single-`data:` compact-JSON frame,
strictly decodes Result Stream Event v2, initializes correlation from the first
required accepted event, and enforces body/header Trace equality, contiguous
sequence/chunk indices, and first-terminal semantics with the existing runtime
sequence validator. Clean completion requires reading EOF after a valid
terminal event so a post-terminal event cannot be hidden. Close at any point
before that clean EOF, including immediately after terminal, reports
interruption. Repeat Close is idempotent and returns the same recorded outcome;
Recv after Close is an explicit closed-stream error.

### Typed errors

Every non-200 response must be exact `application/json`, stay within the
configured response limit, contain exactly one Trace header, and validate as
the pre- or correlated Platform Error v4 shape permitted by the v4 response
phase/status/code matrix. The public `PlatformError` retains only status, code,
Trace, and the optional all-or-none invocation/root Task pair. It does not
retain fixed message text or raw bytes.

Local configuration, transport, decoding, media, correlation, and interruption
failures remain wrapped local errors. They are never recast as a Gateway
Platform Error or successful empty result.

### Credential and ownership boundary

Phase 1 reuses the existing development-static Gateway Bearer authentication
and Owner-only Workspace policy. The SDK accepts the raw credential only at
construction and holds it solely for Authorization header generation. It does
not mint, hash, persist, rotate, list, revoke, serialize, log, or expose the
credential. A wrong but well-formed credential is a Gateway 401, not a local
identity guess.

### Contract correction and compatibility

`contracts/openapi/control-plane-invocation.v4.yaml` will declare one required
`x-nek-trace-id` on success and every structured error response and add the
existing 500 `INTERNAL_ERROR` phase response.

`contracts/openapi/router-internal.v4.yaml` will make the same Trace and 500
facts explicit. Router maps `INTERNAL_ERROR` directly to 500. The Control Plane
Router client requires exactly one response Trace equal to the dispatch request
Trace, closes drifted responses, and returns an internal dependency failure.
Gateway retains its initially generated Trace and no longer conditionally
overwrites it from Router response headers.

These changes align the language-neutral contracts with the intended existing
v4 correlation and Platform Error v4 semantics. Request/result/event shapes and
routes do not change, so a new API version or compatibility path is not
justified.

The Go SDK contract is documented in
`specs/025-workspace-client-sdk/contracts/client-sdk-api.md`. It consumes active
contracts and does not become a competing wire fact.

## Fallback Inventory

| Existing or proposed behavior | Classification | Decision | Evidence |
| --- | --- | --- | --- |
| A caller-supplied `http.Client` with nil `Transport` uses Go's documented default transport | Standard-library policy | Keep | Go `http.Client` contract and Spec 024 retained-policy precedent |
| Gateway conditionally keeps its Trace when Router omits one or replaces it with any non-empty Router Trace | Correlation fallback that hides an internal contract failure | Remove | FR-020 and the single Gateway-created Trace model |
| Router `INTERNAL_ERROR` falls through the default 503 dependency/unavailable status | Semantic-confusion fallback | Remove | FR-021 and Platform Error v4 fixed code/status meaning |
| Retry, redirect, alternate Gateway/Router/Agent destination, Ledger polling, or v3 invocation compatibility | Unsupported degraded/compatibility paths | Do not add | FR-006, FR-011, constitution VII, active v4-only runtime |
| Missing URL, Workspace, credential, HTTP client, limits, malformed result, or interrupted stream returns an empty result | Error-swallowing fallback | Do not add | FR-002, FR-012 through FR-015 |

Fallback target: removed 2, retained 1, added 0, net -2.
Added fallback evidence: none.

### Retained fallback evidence

- **Evidence**: Go `http.Client` documents that nil `Transport` uses
  `DefaultTransport`; Spec 024 already recognizes the same standard-library
  policy.
- **Trigger**: The application explicitly supplies a non-nil `*http.Client`
  whose `Transport` field is nil.
- **Semantics**: Requests still use Go's documented HTTP transport; no identity,
  Workspace, URL, credential, limit, timeout, retry, or result value is
  substituted by the SDK.
- **Boundary**: Go's standard library owns transport selection. The SDK only
  clones the supplied Client and disables redirects.
- **Visibility**: This is ordinary documented transport selection, not degraded
  service behavior; the application retains its original explicit Client
  configuration.
- **Tests**: T013 verifies that a non-nil Client with nil Transport is accepted,
  remains unmodified, and can use the documented transport contract while nil
  Client configuration fails.

## Project Structure

### Documentation (this feature)

```text
specs/025-workspace-client-sdk/
|-- spec.md
|-- clarify.md
|-- plan.md
|-- research.md
|-- data-model.md
|-- quickstart.md
|-- contracts/
|   `-- client-sdk-api.md
|-- checklists/
|   `-- requirements.md
`-- tasks.md
```

### Source Code (repository root)

```text
contracts/
|-- openapi/control-plane-invocation.v4.yaml
|-- openapi/router-internal.v4.yaml
`-- result_api_contracts_test.go

apps/control-plane/internal/
|-- invocation/router_client.go
|-- invocation/router_client_test.go
|-- gateway/invocation_handler.go
`-- gateway/invocation_handler_test.go

apps/a2a-router/internal/api/
|-- dispatch_handler.go
`-- dispatch_handler_test.go

sdks/client-sdk/
|-- client.go
|-- config.go
|-- errors.go
|-- stream.go
|-- json.go
|-- client_test.go
|-- config_test.go
|-- stream_test.go
|-- contract_test.go
|-- example_test.go
`-- README.md

README.md
docs/contracts/compatibility.md
docs/handoffs/CURRENT.md
specs/023-trusted-agent-publication/tasks.md
AGENTS.md
```

**Structure Decision**: Add one standalone application SDK beside, not inside,
`sdks/agent-sdk`. Reuse only root `contracts` DTOs and validators. Do not import
Control Plane, Router, Workspace, Catalog, Agent SDK, or Agent Runtime internal
packages, and do not refactor the Agent SDK in this slice.

## Complexity Tracking

No constitution violation requires justification.

## Verification Commands

```text
gofmt -w sdks/client-sdk
go test ./contracts ./sdks/client-sdk/...
go test -race ./sdks/client-sdk/...
go vet ./...
go test ./...
golangci-lint run
git diff --check
```

Issue #52 owns the clean Compose/PostgreSQL trusted-publication E2E extension;
this slice supplies contract/transport tests and a compiled application example
without duplicating that later acceptance owner.
