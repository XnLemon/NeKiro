# Contract Design: Invocation Runtime Gate

## Direction Matrix

| Contract | Authenticated caller | Owner | Trusted context |
| --- | --- | --- | --- |
| Northbound Invocation v4 | Gateway user Bearer | Gateway/Dispatch | Gateway identity, path Workspace, generated root correlation |
| Router Internal v3 | Control Plane service Bearer | Router | Entire service DTO after service authentication; Router still re-resolves exact Card |
| Agent Router v1 | Agent Bearer binding | Router | Agent ID from binding; lineage/Workspace from parent Ledger |
| Control Plane Internal v2 | Router service Bearer | Control Plane | Existing exact resolution request |

## Pre/Post Acceptance Errors

- Before context generation, errors contain safe Trace only.
- After root context exists, failures repeat exact Invocation/root Task/Trace.
- Agent-facing failures before child creation contain request correlation only when it is trusted: parent ID may be referenced, but no child IDs are synthesized for an unauthenticated/invalid parent.
- A successful child `created` commit enables exact child correlation on subsequent errors/results/events.

## HTTP and SSE

- `413 PAYLOAD_TOO_LARGE` is distinct from malformed `400 VALIDATION_ERROR`.
- JSON success waits for terminal Ledger commit.
- SSE `accepted` is emitted only after `created` commits.
- After SSE commitment, dependency failure uses a correlated `failed` Result Stream Event v2; it does not imply a failed Ledger terminal.
- EOF without a result terminal remains interrupted delivery.

## Compatibility

Northbound Invocation v4, Router Internal v3, Platform Error v4, Invocation Event 0.3, and Result Stream Event v2 are breaking target revisions. Agent Router v1 is new. Invocation Result v1, Control Plane Internal v2, Agent Card 0.2, and A2A Profile 0.2/0.3.0 remain unchanged.

Historical files are immutable evidence only. Runtime children must not serve or decode historical alternatives.
