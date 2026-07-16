# Feature Specification: Control Plane Invocation Dispatch

**Feature Branch**: `codex/012-invocation-dispatch`  
**Created**: 2026-07-16  
**Status**: Ready for implementation  
**Input**: GitHub #21 / Spec 010 T002.

## Clarifications

### Session 2026-07-16

- Q: When are root Invocation and Task identifiers created? A: Only after public authentication, strict body/media validation, and Workspace authorization return an exact installed version. The boundary Trace exists earlier for pre-correlation errors.
- Q: Which downstream is permitted? A: Router Internal v3 at one required configured HTTPS/HTTP destination with one explicit service Bearer token; Dispatch never resolves or calls an Agent endpoint.
- Q: How are results proxied? A: JSON response bodies are streamed as received and SSE is forwarded one complete bounded event at a time with immediate flush. The complete response is never buffered or replayed.
- Q: Which limits are owned here? A: Gateway public body, Gateway SSE event, and Gateway invocation deadline are required strict configuration with no defaults. Router owns its internal-body and Agent/A2A limits.
- Q: What happens when Router connectivity fails? A: An explicit correlated `DEPENDENCY_ERROR` is returned because root correlation already exists, with no retry, alternate Router, cached result, or direct Agent path.

## User Scenarios & Testing

### User Story 1 - Invoke an authorized installed Agent (Priority: P1)

A Workspace owner submits one exact v4 request and receives the Router's live JSON or SSE outcome only after the Control Plane verifies the current enabled installation, exact pin, published Card, capability, and accepted permissions.

**Independent Test**: Invoke through an in-memory Gateway with fake Workspace and Router ports; verify exact root context, exact pinned version, and one Router request while unauthorized cases produce no correlation IDs and no Router call.

**Acceptance Scenarios**:

1. **Given** valid auth, body, media, and an enabled authorized installation, **When** JSON invocation is requested, **Then** Dispatch creates distinct root Invocation/Task IDs, sends one Router Internal v3 request, and streams its JSON response.
2. **Given** the same authorization for stream mode, **When** Router emits flushed SSE events, **Then** Gateway forwards bounded complete events in order and flushes each event.
3. **Given** a missing/disabled/unpublished installation or disallowed capability, **When** invoke is requested, **Then** the exact Workspace policy error is returned before root correlation creation and Router is not contacted.

### User Story 2 - Reject malformed public requests before dispatch (Priority: P1)

A caller receives deterministic v4 errors for authentication, media, body-size, shape, and identifier failures without creating an Invocation.

**Independent Test**: Exercise missing auth, wrong Content-Type/Accept, duplicate/unknown fields, invalid JSON/object input, trailing data, and body overflow; assert a three-field v4 error and zero Workspace/Router calls where applicable.

### User Story 3 - Preserve typed Router failures (Priority: P1)

A caller receives the Router's exact status, v4 error body, correlation headers, and result media rather than a remapped generic success or error.

**Independent Test**: Return representative pre/post-acceptance 4xx/5xx JSON and SSE outcomes from a fake Router and compare the public response byte stream and status.

## Requirements

- **FR-001**: Gateway MUST authenticate and strictly validate Content-Type, Accept, body size, duplicate/unknown fields, required fields, identifiers, object input, and trailing content before Workspace authorization.
- **FR-002**: Gateway MUST return Platform Error v4 `PAYLOAD_TOO_LARGE` with HTTP 413 for public body overflow and `NOT_ACCEPTABLE` with HTTP 406 for media mismatch.
- **FR-003**: Dispatch MUST authorize through the Workspace owner and current Installation policy and obtain the exact installed Card version without accessing Workspace storage directly.
- **FR-004**: Root Invocation ID and root Task ID MUST be generated only after FR-001 and FR-003 succeed; they MUST be distinct valid identifiers and use the Gateway Trace.
- **FR-005**: Dispatch MUST send exactly one `DispatchInvocationRequestV3` to Router Internal v3 with `caller.type=user`, authenticated caller ID, exact Workspace/target/version/capability/input/mode, and trusted correlation.
- **FR-006**: Control Plane MUST NOT call, resolve, cache, or accept an Agent endpoint.
- **FR-007**: Router URL, Router service Bearer token, public body limit, Gateway SSE event limit, and invocation deadline MUST be required strict configuration with no default.
- **FR-008**: JSON and SSE response bodies MUST be forwarded live without full-response buffering; each SSE event MUST remain bounded and be flushed immediately.
- **FR-009**: Router HTTP status, result media, exact v4 error/result bytes, and correlation MUST be preserved; a pre-response Router transport failure MUST produce correlated HTTP 503 `DEPENDENCY_ERROR`.
- **FR-010**: Dispatch MUST NOT retry, replay, switch Router destinations, use cached results, or fall through to direct Agent access.

## Non-Goals

Router execution, Ledger persistence/reads, Agent transport, Agent SDK, cancellation protocol, result replay, Console UI, historical contract serving, deployment/Compose integration, or Agent Runtime behavior.

## Success Criteria

- **SC-001**: All invalid auth/media/body/Workspace cases make zero Router calls and all pre-authorization failures contain no Invocation/root Task ID.
- **SC-002**: Valid JSON and SSE cases deliver one exact Router request with exact pin and distinct generated root identifiers.
- **SC-003**: Streaming tests prove first bytes/events reach the caller before Router completion and every SSE event is immediately flushed.
- **SC-004**: Config tests reject missing, blank, malformed, whitespace-padded, zero, negative, signed, fractional, exponent, overflow, out-of-range, credential-bearing URL, and malformed destination/limit values.

## Fallback Report

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
