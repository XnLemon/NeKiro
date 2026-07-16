# Data Model: Control Plane Invocation Dispatch

## Authorized Target

Ephemeral Workspace-owned decision: `workspace_id`, `agent_id`, exact `agent_card_version`, and capability authorization. It is not persisted by Dispatch.

## Root Context

Ephemeral Gateway-owned values created after authorization: `invocation_id`, `root_task_id`, `trace_id`, and authenticated user caller. Root Invocation and root Task IDs are distinct.

## Router Exchange

One `DispatchInvocationRequestV3` plus one live HTTP response body. Input and result bytes are never stored, cached, replayed, or logged by this feature.
