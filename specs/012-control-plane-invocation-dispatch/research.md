# Research: Control Plane Invocation Dispatch

## Exact Pin Authorization

**Decision**: Add one narrow Workspace-owned authorization operation for the public Dispatch path.  
**Rationale**: Existing `Resolve` is the Router's re-resolution boundary and requires an already-known exact version; Dispatch must obtain that pin without reading Workspace storage.  
**Alternatives considered**: Calling `Resolve` with a guessed version or reading installation tables from Dispatch; both violate ownership and zero-fallback policy.

## Result Media

**Decision**: Reuse `contracts.NegotiateInvocationResultMode` and frozen Router Internal v3 unchanged.  
**Rationale**: Spec 011 already provides the executable exact Accept matrix and trusted downstream DTO.  
**Alternatives considered**: Lenient media parsing or historical v2 decode; both are prohibited compatibility fallbacks.

## Live Proxy

**Decision**: Stream JSON reads directly and buffer at most one bounded SSE event before an immediate flush.  
**Rationale**: This proves live delivery while enforcing the Gateway event limit and exact framing.  
**Alternatives considered**: Full response buffering/replay or unbounded line reads; both violate scope or size policy.

## Destination And Credentials

**Decision**: Require one absolute exact Router operation URL and one explicit visible-ASCII Bearer token.  
**Rationale**: A single validated destination and credential make failure explicit and prevent accidental secret-in-URL or alternate-route behavior.  
**Alternatives considered**: Default localhost, URL userinfo, destination lists, anonymous access, or retries; all are rejected.
