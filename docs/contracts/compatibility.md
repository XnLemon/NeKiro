# Contract Compatibility Policy

## Independent Versions

Agent Card Schema version, Agent version, HTTP API version, internal API
version, event version, result version, A2A Profile Schema version, and A2A
protocol version are independent values. They must not be inferred from one
another.

The versioned JSON Schema, OpenAPI, semantic-rule, conformance, and A2A Profile
files under `contracts/` are contract facts. Go and TypeScript mappings are
consumers and must not redefine their semantics.

## Phase 1 Contract Set

| Contract | Historical | Active target | Compatibility impact |
| --- | --- | --- | --- |
| Agent Card Schema | `0.1` | `0.2` | Breaking: portable semantic rejection rules narrow accepted Cards |
| Northbound API | `v1` | `v2` | Breaking: invocation changes from `202` acceptance to direct result delivery |
| Control Plane Internal API | none | `v1` | New directional owner contract |
| Router Internal API | `v1` | `v2` | Breaking: dispatch returns JSON/SSE results and no longer owns resolution |
| Invocation Event Schema | `0.1` | `0.2` | Breaking: terminal status and error-code combinations are stricter |
| Platform Error | `v1` | `v2` | Breaking: trace correlation is required and `NOT_ACCEPTABLE` is added |
| Invocation Result | none | `v1` | New transient JSON and SSE result contracts |
| A2A Profile Schema | `0.1` | `0.2` | Breaking profile metadata and conformance requirements |
| A2A protocol | `0.3.0` | `0.3.0` | Unchanged wire protocol |

Historical files remain unchanged as migration evidence. The first backend
runtime implements only active targets. No deployed runtime consumer exists,
so there is no dual-read, dual-write, or dual-dispatch compatibility window.
All consumers must adopt the active target before that runtime is introduced.

## Catalog v2 Completion

Spec 002 additively completes the existing Northbound v2 Catalog operations
before their first runtime implementation. The success representations and
operation paths are unchanged. The active document now makes previously
unspecified behavior explicit:

- all five Catalog operations require Gateway Bearer authentication;
- every Catalog response carries the Gateway-assigned `x-nek-trace-id`;
- registration and lifecycle mutation enforce immutable owner identity;
- published exact versions are authenticated-visible, while draft and disabled
  exact versions are owner-visible only;
- omitted discovery limit is the product policy `25`, explicit limits are
  `1-100`, and opaque cursors are bound to filters and traversal boundary;
- validation, unauthenticated, forbidden, not found, conflict, and dependency
  failures use their exact Platform Error v2 status/code mappings.
- the registration transport cap is 16,777,216 bytes and uses the existing
  validation failure, while active unbounded JSON integer fields keep exact
  `json.Number` semantics instead of a machine `int64` range.

No existing deployed Catalog runtime or generated client consumes the earlier
underspecified form, so a new API version or compatibility window is not
required. Northbound v1 and Agent Card 0.1 remain byte-unchanged historical
evidence and receive no runtime route, decoder, auto-upgrade, or fallback.

## Compatible Changes

- Adding an optional field is additive when omission preserves existing
  semantics.
- Adding a new endpoint or event type is additive only when existing consumers
  remain valid.
- Adding an enum member requires consumer impact review because exhaustive
  consumers may treat it as breaking.

## Breaking Changes

- Removing or renaming a field
- Changing a field type or requiredness
- Changing an existing field's semantics
- Changing response status or media type for an existing operation
- Tightening accepted values or semantic validation rules
- Moving an operation to a different service owner or destination
- Reusing an error code for a different state
- Changing the fixed public message associated with an error code
- Reinterpreting historical Ledger events

Breaking changes require a new contract version, migration guidance, and an
explicit compatibility window or a documented pre-runtime declaration that no
compatibility runtime is justified.

## Invocation v2 Migration

- Replace Northbound `POST /v1/workspaces/{workspaceId}/invocations` and Router
  `POST /internal/v1/invocations` acceptance handling with the corresponding v2
  same-request result operations.
- Send `Accept: application/json` or a compatible wildcard with `stream=false`.
- Send `Accept: text/event-stream` with `stream=true` and consume ordered SSE
  data values until exactly one terminal event.
- Treat `406 NOT_ACCEPTABLE` as request negotiation failure.
- Treat EOF without a terminal event as interrupted delivery. Do not treat
  received chunks as a successful result.
- Do not poll Ledger APIs for result content. Results are not persisted,
  replayed, or recoverable after disconnect; obtaining output requires a new
  Invocation.
- Route exact Agent resolution to Control Plane Internal v1. Route dispatch and
  Ledger/trace reads to Router Internal v2.

## Failure And Data Semantics

Missing input, invalid input, not found, forbidden, disabled, dependency
failure, timeout, cancellation, and protocol failure are distinct states.
Contracts must not collapse them into `null`, an empty collection, a boolean,
or a normal success response.

Platform Error v2 contains only fixed public messages and safe correlation.
Agent input, result data, endpoint details, credentials, raw dependency errors,
and stack data are forbidden. Invocation Event v0.2 and Ledger query contracts
contain metadata only; no result or chunk field is compatible with that model.
