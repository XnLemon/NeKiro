# Contract Design: Catalog Registration and Discovery

## Baseline and Compatibility

This feature implements and additively completes the Catalog subset of
Northbound API `v2`:

- register one immutable Agent Card `0.2` version;
- publish an exact version;
- disable an exact version;
- read an exact version;
- discover published versions.

Success payloads remain the existing `CatalogEntry` and discovery response
shapes. Historical Northbound `v1` and Agent Card `0.1` remain unchanged
migration evidence and receive no runtime route or compatibility decoder.

Before runtime implementation, `contracts/openapi/control-plane.v2.yaml` must
declare the authentication, pagination, visibility, and failure behavior below.
Go DTOs remain consumers of that document and the versioned JSON Schemas.

## Authentication Boundary

Every Catalog operation requires an HTTP Bearer credential through the
Northbound security scheme. The Gateway authenticator maps it to one trusted
caller ID before Catalog code runs.

- Missing, empty, malformed, or rejected credentials return `401` with Platform
  Error v2 code `UNAUTHENTICATED`.
- Registration, publication, and disablement require caller ID equal to the
  immutable Agent owner; mismatch returns `403 FORBIDDEN`.
- Published exact versions are readable by authenticated platform callers.
- Draft and disabled exact versions are readable only by their owner; other
  authenticated callers receive `403 FORBIDDEN`.
- The public request never supplies a trusted owner through a caller-ID header.
- Authentication credentials never appear in Card, response, log, cursor, or
  Platform Error fields.

The initial local `development-static` authenticator is an adapter, not a new
public identity contract. Production identity-provider selection is deferred.

## Correlation

Gateway assigns one trace ID to each HTTP request before producing any
Platform Error. Successful and failed responses expose the same value in the
`x-nek-trace-id` response header; Platform Error v2 repeats it in `traceId`.
Catalog operations create no Invocation, root Task, or Ledger fact.

The server initializes its trace-ID generator before accepting traffic. Missing
or invalid request correlation is never replaced with caller-controlled data.

## Strict Request Handling

- JSON request bodies use `application/json`.
- Registration rejects malformed JSON, duplicate members at any depth,
  unknown envelope/Card fields, trailing JSON values, and every active Agent
  Card structural or semantic violation before persistence.
- Registration accepts at most 16,777,216 request-body bytes and must receive
  the complete body within the server's 30-second body-read window. An
  oversized body returns `400 VALIDATION_ERROR`; a partial or timed-out body is
  never passed to Registry persistence.
- Go mappings preserve every active legal JSON integer exactly. In particular,
  unbounded positive `maxInputBytes` and `maxOutputBytes` values are not decoded
  through `int64` or `float64`.
- Path and query identifiers use the active common contract primitives.
- Blank free text, explicit limits outside 1-100, malformed cursors, and cursors
  bound to different filters are validation failures.
- Omitted `limit` is the explicit product policy `25`; an invalid explicit value
  is never clamped to it.
- Fixed public errors do not include the request body or internal validation,
  database, or authentication details.

## Operations

### Register Agent Version

**Success**: `201 application/json` with an immutable draft `CatalogEntry`.

The authenticated caller must equal `card.owner.id`. The exact Card identity is
globally unique. A repeated `(agentId, version)` returns conflict regardless of
whether the submitted Card is equal.

| Status | Platform Error code | Meaning |
|---:|---|---|
| `400` | `VALIDATION_ERROR` | Malformed, oversized, or nonconforming active Card request |
| `401` | `UNAUTHENTICATED` | No accepted caller identity |
| `403` | `FORBIDDEN` | Caller does not match Card owner or existing Agent owner |
| `409` | `CONFLICT` | Exact version already exists or immutable owner conflicts |
| `503` | `DEPENDENCY_ERROR` | Registry transaction could not complete |

### Publish Agent Version

**Success**: `200 application/json` with the now-published `CatalogEntry`.

Only an owned draft may publish. Success means the version is immediately
eligible for Discovery. Published or disabled state returns conflict and never
rewrites `publishedAt`.

| Status | Platform Error code | Meaning |
|---:|---|---|
| `400` | `VALIDATION_ERROR` | Invalid path identity |
| `401` | `UNAUTHENTICATED` | No accepted caller identity |
| `403` | `FORBIDDEN` | Caller does not own the Agent |
| `404` | `NOT_FOUND` | Exact version does not exist |
| `409` | `CONFLICT` | Current state is not draft |
| `503` | `DEPENDENCY_ERROR` | Registry transition could not complete |

### Disable Agent Version

**Success**: `200 application/json` with the disabled `CatalogEntry`.

An owned draft or published version transitions to disabled. Repeating disable
returns the unchanged disabled entry as the specified idempotent success. A
disabled version is no longer discoverable and cannot publish.

| Status | Platform Error code | Meaning |
|---:|---|---|
| `400` | `VALIDATION_ERROR` | Invalid path identity |
| `401` | `UNAUTHENTICATED` | No accepted caller identity |
| `403` | `FORBIDDEN` | Caller does not own the Agent |
| `404` | `NOT_FOUND` | Exact version does not exist |
| `503` | `DEPENDENCY_ERROR` | Registry transition could not complete |

### Read Exact Agent Version

**Success**: `200 application/json` with the exact immutable `CatalogEntry`.

Published versions are visible to any authenticated caller. Draft or disabled
versions require the immutable owner.

| Status | Platform Error code | Meaning |
|---:|---|---|
| `400` | `VALIDATION_ERROR` | Invalid path identity |
| `401` | `UNAUTHENTICATED` | No accepted caller identity |
| `403` | `FORBIDDEN` | Non-owner requested a non-public version |
| `404` | `NOT_FOUND` | Exact version does not exist |
| `503` | `DEPENDENCY_ERROR` | Registry read could not complete |

### Discover Published Agent Versions

**Success**: `200 application/json` with required `items` and optional
`nextCursor`.

- Every item is one exact published version.
- No matches is `items: []` with no cursor.
- `query` is a literal case-insensitive substring over Card name or description.
- `capability` and `ownerId` use exact identifier equality.
- Supplied filters combine with AND.
- `limit` defaults to 25 and accepts explicit 1-100.
- Ordering is publication time descending, Agent ID ascending, and exact version
  string ascending.
- Continuation is opaque and bound to the original filters, page size, and
  first-page publication boundary.

| Status | Platform Error code | Meaning |
|---:|---|---|
| `400` | `VALIDATION_ERROR` | Invalid filter, limit, or cursor |
| `401` | `UNAUTHENTICATED` | No accepted caller identity |
| `503` | `DEPENDENCY_ERROR` | Discovery read could not complete |

Dependency failure is never represented as empty discovery or not found.

## State and Visibility Matrix

| State | Owner exact read | Other authenticated exact read | Discovery | Publish | Disable |
|---|---|---|---|---|---|
| `draft` | allowed | forbidden | excluded | allowed once | allowed |
| `published` | allowed | allowed | included | conflict | allowed |
| `disabled` | allowed | forbidden | excluded | conflict | idempotent success |

## Local Authentication Adapter Contract

The runnable local adapter is selected only by explicit mode
`development-static`. Its required configuration is a strict JSON array of
objects containing exactly `id` and `tokenSha256` strings. `tokenSha256` is the
64-character lowercase hexadecimal SHA-256 digest of the local bearer token;
the raw token is never stored in process configuration.

- Missing, blank, duplicate, or unknown fields, malformed digests, and duplicate
  IDs/digests fail startup.
- Incoming bearer tokens are hashed and compared in constant time without
  logging token or digest values.
- The configuration has no built-in principal, token, file, or mode default.
- The adapter is not enabled under another mode and is not described as a
  production authentication mechanism.

Tests may inject an in-memory authenticator directly; runtime handlers never
fall back to trusting a public caller header.

## Secret and Data Exclusions

The following are forbidden from Catalog responses, cursors, and logs:

- Bearer credentials or static-auth configuration;
- complete registration request/Card bodies;
- endpoint credential material;
- input/output schema content in operational logs;
- raw database errors, SQL, DSNs, or dependency host details;
- Agent input, output, Runtime state, health, or invocation data.

## Cross-Runtime Evidence

Acceptance fixtures include two conforming Cards that describe Agents backed by
different Runtime implementations. Catalog tests validate, register, publish,
and discover them without importing, starting, probing, or depending on either
Runtime. This proves metadata portability only; live A2A invocation remains a
later feature.
