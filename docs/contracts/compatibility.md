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
| Trusted Publication | none | `v1` | New Registry-owned provider, endpoint-binding, challenge, and typed verification-error contract |
| Workspace Schema | none | `v1` | New minimal authorization-root fact |
| Installation Schema | `v1` | `v2` | Breaking: canonical semantic invariants are frozen |
| Northbound API | `v1` / `v2` | `v3` | Breaking: v3 completes authenticated Workspace/Installation semantics and body-bearing uninstall |
| Control Plane Internal API | `v1` | `v2` exact Card resolution / `v3` installed-version resolution | v2 is breaking from v1; v3 additively owns deterministic nested version selection |
| Router Internal metadata API | `v1` / `v2` | `v3` | Breaking: Workspace-scoped metadata reads use the runtime contract |
| Invocation Event Schema | `0.1` | `0.2` | Breaking: terminal status and error-code combinations are stricter |
| Platform Error | `v1` | `v2` / `v3` | v2 remains active for Catalog/Invocation; v3 adds Workspace `INSTALLATION_DISABLED` |
| Invocation Result | none | `v1` | New transient JSON and SSE result contracts |
| A2A Profile Schema | `0.1` | `0.2` | Breaking profile metadata and conformance requirements |
| Router Invocation Credential | none | `v1` | New companion contract: exact Ed25519 Router-to-Agent request authentication |
| A2A protocol | `0.3.0` | `0.3.0` | Unchanged wire protocol |

Spec 011 adds invocation-runtime targets without replacing the active Catalog
and Workspace surfaces:

| Contract | Historical | Runtime target | Compatibility impact |
| --- | --- | --- | --- |
| Northbound Invocation API | invocation routes in Control Plane `v3` | invocation-only `v4` | Breaking acceptance, size, error, and persistence-interruption semantics; Catalog/Workspace/Installation remain on v3 |
| Router Internal dispatch API | `v1` / `v2` / `v3` | `v4` | Breaking service-auth, managed `http_bearer` acceptance, size, and post-side-effect failure semantics; v3 dispatch is historical evidence |
| Agent Router API | none | `v1` | New authenticated Agent-SDK direction and parent-derived trust model |
| Control Plane Internal API | `v1` | `v2` exact Card resolution / `v3` installed-version resolution | v3 adds a phase-aware nested version-selection operation without dual-read fallback |
| Platform Error | `v1` / `v2` / `v3` | `v4` for invocation runtime | Breaking closed pre/correlated shapes and exact unsupported-auth/request-size/Agent-response-size outcomes |
| Invocation Event | `0.1` / `0.2` | `0.3` | Breaking embedded Platform Error v4 revision |
| Result Stream Event | `v1` | `v2` | Breaking embedded Platform Error v4 revision |

Historical files remain unchanged as migration evidence. The first backend
runtime implements only active targets. No deployed runtime consumer exists,
so there is no dual-read, dual-write, or dual-dispatch compatibility window.
All consumers must adopt the active target before that runtime is introduced.

Router Invocation Credential `v1` is a separately versioned companion contract
for the managed Router-to-Agent HTTP hop. It owns the complete signed claim and
context-header binding, strict 401/403 response shape, and portable conformance
corpus under `contracts/router-agent-credential/v1/`. It does not modify A2A
Profile Schema `0.2`, Agent Card Schema `0.2`, Router Internal dispatch API `v4`,
Router Internal metadata API `v3` (`router-metadata.v3.yaml`), result contracts,
or Invocation Ledger facts. The complete `router-internal.v3.yaml` is
historical evidence and is not an active dependency.
Agent-to-Router nested credentials remain
the existing opaque Workspace/Agent binding in the opposite direction.

## Catalog v2 Completion

Spec 002 additively completes the existing Northbound v2 Catalog operations
before their first runtime implementation. The success representations and
operation paths are unchanged. The active document now makes previously
unspecified behavior explicit:

- all five Catalog operations require Gateway Bearer authentication;
- every Catalog response carries the Gateway-assigned `x-nek-trace-id`;
- registration and lifecycle mutation enforce immutable owner identity;
- Trusted Publication v1 release records copy exact Card, endpoint binding,
  provider, and digest facts; `installedReleaseId` is an additive optional
  Installation field for trusted pins. Catalog migration marks every pre-v4
  published row as `legacy_unverified`; a missing Release on
  a new version is not a compatibility signal. Trusted invocation metadata
  carries the exact Release ID and Card digest into Router/Ledger records;
  the absence of both fields is the explicit legacy/unverified wire encoding
  retained for historical events. Control Plane Internal v2 additively returns
  the exact Catalog-owned Card digest beside `installedReleaseId`, and Router
  rejects dispatch provenance that omits or differs from that pair instead of
  recomputing historical Card bytes;
- Platform Error v3/v4 add stable Release-state codes for unpublished,
  suspended, and revoked Releases; no previously valid error payload changes;
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

## Workspace And Installation Contract Gate

Spec 003 completes the previously partial Workspace/Installation foundations
before their first runtime implementation:

- Workspace v1 adds the exact four-field logical authorization root:
  `workspaceId`, immutable trusted `ownerId`, `createdAt`, and `updatedAt`.
- Installation v2 keeps the submitted constraint, exact installed version,
  accepted permission snapshot, state, and timestamps; it additionally freezes
  canonical permission order, constraint-compatible exact pins, and timestamp
  relationships. `uninstalledAt` is
  required only for terminal uninstalled history.
- Northbound v2 remains byte-unchanged migration evidence. Northbound v3
  completes Workspace create/read and Installation create/read/list/lifecycle
  with Bearer security, Trace headers, Installation v2 responses, and
  operation-specific fixed errors.
- Northbound v3 uninstall returns `200` with the preserved terminal
  Installation v2 fact. Historical v2 retains its original `204` behavior.
- Installation list inspection in v3 requires an explicit bounded `limit`
  (range 1-100), stable keyset order, and an opaque continuation cursor.
- Control Plane Internal v2 requires a separately trusted service Bearer
  identity, distinguishes missing Installation, Installation disabled, Catalog
  version disabled, capability denial, and dependency failure, and defines a
  pre-correlation error shape for malformed/missing IDs.
- Control Plane Internal v3 uses the same service boundary and validates
  phase-specific status/code/correlation combinations for installed-version
  resolution; it never falls back to v2 for that operation.
- Platform Error v3 adds `INSTALLATION_DISABLED` with fixed message
  `The Agent installation is disabled.` `AGENT_DISABLED` retains its Catalog
  Agent-version meaning; existing Platform Error v2 remains unchanged.
- The previous `common.v1` `semverRange` length tightening is removed; SemVer
  parser validation remains the sole active range constraint.

Northbound v2 and Installation v1 remain byte-unchanged historical evidence. Installation v1's structural
shape did not freeze the v2 semantic invariants, so first Workspace consumers
must adopt Installation v2. Control Plane Internal v1 remains historical and
must not be dual-read; first Router consumers use v2/v3 according to operation. Platform Error v2 remains
the active Catalog/Invocation contract in Northbound v3, while first Workspace and
internal-resolution consumers use v3. No deployed Workspace or Router
resolution runtime exists, so these version increments need no compatibility
runtime window. First runtime consumers implement v3 only; migration impact is
explicit in the active contract guide.

Historical Northbound v1/v2, Agent Card 0.1, Router Internal v1, and all other
historical artifacts remain unchanged migration evidence.

## Northbound v3 Migration

- Replace `/v2` Northbound paths with their `/v3` equivalents.
- Supply an explicit Installation list `limit` from 1 through 100; omission is
  a validation error and has no default.
- Consume uninstall as `200 application/json` with an Installation v2 body.
- Do not run v2 and v3 as a fallback pair. v2 remains contract history only.

## Invocation Runtime Target Migration

- Keep Catalog, Workspace, and Installation clients on
  `control-plane.v3.yaml`. Use `control-plane-invocation.v4.yaml` at the same
  Gateway destination only for `/v4/workspaces/{workspaceId}/invocations...`
  and `/v4/workspaces/{workspaceId}/traces/...`. The invocation-only document is not a second fact for
  the v3-owned domains.
- Legacy Invocation paths embedded in `control-plane.v3.yaml` are migration
  evidence only; no runtime may serve them or pair them with the v4 routes.
- Control Plane Dispatch uses Router Internal dispatch v4. Workspace-scoped
  Invocation/Trace reads use Router Internal metadata v3. Agent SDKs use Agent Router
  v1 with an Agent-bound credential; the caller classes and credentials are not
  interchangeable.
- Adopt Platform Error v4, Invocation Event 0.3, and Result Stream Event v2
  together. Treat pre-acceptance HTTP 413 as `PAYLOAD_TOO_LARGE`; treat HTTP
  502/in-band failed as `AGENT_AUTH_UNSUPPORTED` or
  `AGENT_RESPONSE_TOO_LARGE` only when that exact code is present. After
  acceptance the correlated error shape is mandatory.
- Replace unscoped v3 `/v3/invocations/{invocationId}` and
  `/v3/traces/{traceId}` reads with Workspace-scoped
  `/v4/workspaces/{workspaceId}/invocations/{invocationId}` and
  `/v4/workspaces/{workspaceId}/traces/{traceId}`. Consume the Invocation
  detail projection/events and Trace lineage projection responses; v4 does not
  expose raw event arrays.
- Use the exact shared Accept matrix: non-stream JSON accepts
  `application/json`, `application/*`, or `*/*`; stream accepts only
  `text/event-stream`. Do not normalize or fall back from unsupported values.
- Configure every deadline/size value explicitly. Omission or invalid text is a
  startup/readiness failure and has no migration default.
- Treat successful `created` commit as acceptance. A post-side-effect
  `DEPENDENCY_ERROR` may coexist with a last committed non-terminal Ledger
  history; do not infer or synthesize a terminal outcome.
- Do not run v3/v4 Northbound Invocation or v3/v4 Router dispatch as fallback
  pairs. No deployed runtime consumer justifies a compatibility window; the v3
  dispatch route is retired while v3 metadata reads remain active.

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
- Route nested installed-version selection to Control Plane Internal v3, then
  exact Card resolution to Control Plane Internal v2. Route dispatch to Router
  Internal v4 and Ledger/trace reads to Router Internal metadata v3.

## Failure And Data Semantics

Missing input, invalid input, not found, forbidden, disabled, dependency
failure, timeout, cancellation, and protocol failure are distinct states.
Contracts must not collapse them into `null`, an empty collection, a boolean,
or a normal success response.

Catalog Platform Error v2, Workspace/Installation Platform Error v3, and runtime
Platform Error v4 contain only fixed public messages and safe correlation on
their respective surfaces. Agent input, result data, endpoint details,
credentials, raw dependency errors, and stack data are forbidden. Runtime
Invocation Event v0.3 and Ledger query contracts contain metadata only; the
historical Invocation Event v0.2 remains migration evidence and no result or
chunk field is compatible with the active metadata model.
