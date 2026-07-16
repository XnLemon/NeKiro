# Data Model: Non-Streaming A2A Dispatch

## Dispatch Request

Existing Router Internal v3 request. Important fields:

- `invocationId`
- `rootTaskId`
- `parentInvocationId` (optional)
- `traceId`
- `caller`
- `workspaceId`
- `targetAgentId`
- `agentCardVersion`
- `capability`
- `input`
- `stream=false`

## Resolved Agent Target

Derived only from Control Plane internal resolution:

- exact Agent identity and version
- endpoint URL
- A2A profile/protocol version
- supported capability
- declared auth mode and non-secret routing metadata

The Router must not persist a permanent Agent Card copy.

## Non-Streaming Transport Result

Transient in-memory result returned to the caller:

- correlation IDs copied from dispatch request
- result payload from A2A `message/send`
- safe platform error on failure

No Agent input or output is stored in Ledger.

## Ledger Facts

Existing Invocation Event v0.3 / projection model remains authoritative.
Spec 016 may append accepted/routing/Agent-attempt/terminal metadata facts as
required by active lifecycle validation. Stored facts include identifiers,
status, timing, safe error code, and metadata counts only.
