# Research: Invocation Runtime Contracts

## Decision 1: Separate Agent-facing Router API

**Decision**: Add `router-agent.v1.yaml`; do not reuse the service-authenticated Router Internal API.

**Rationale**: Control Plane and Agents are different caller classes. The Agent credential establishes only Agent identity; committed parent metadata establishes lineage and Workspace.

**Rejected**: SDK calling Control Plane; shared service credential; request-supplied caller/Workspace/correlation; direct Agent endpoints.

## Decision 2: Parent-derived nested trust

**Decision**: The request contains only `parentInvocationId`, target, capability, input, and `stream`. Router authenticates the Agent, loads the parent, requires parent target Agent equality and `running` state, then generates the child ID and copies Workspace/root Task/Trace.

**Rationale**: Every trusted value has one owner and forged values are structurally impossible.

**Rejected**: Signed context headers as the source of truth in Phase 1; caller-provided Card version; accepting terminal or cross-Workspace parents.

## Decision 3: `none`-only callee authentication

**Decision**: Phase 1 Agent transport supports only `authentication.type=none`. Other Card types stay valid Registry metadata but invocation fails from routing with `AGENT_AUTH_UNSUPPORTED` and no Agent request.

**Rationale**: Cards intentionally hold neither secrets nor locators, and no approved credential store/binding lifecycle exists.

**Rejected**: Empty Authorization, environment-variable naming convention, default token, anonymous fallback, overloading unavailable/route errors.

## Decision 4: Created commit is acceptance

**Decision**: Router may call an Agent only after `created` commits. Before that, every failure has zero Ledger facts. Normal route/auth/resolution failures append an exact terminal from routing.

**Rationale**: This is the earliest durable audit point and makes acceptance testable.

## Decision 5: Post-side-effect storage loss stays non-terminal

**Decision**: A JSON request receives correlated dependency failure if response is uncommitted. A committed SSE receives a correlated `failed` delivery event when writable. Ledger history remains at the last committed non-terminal status. No terminal Ledger event, retry, alternate store, or reconciler is invented.

**Rationale**: A failed Ledger cannot truthfully persist its own failure. The response describes delivery failure; it does not rewrite durable history.

## Decision 6: Required operator limits without invented values

**Decision**: Deadline and public/internal request, A2A response/event, and SSE event limits are required positive decimal configuration. Contracts set validation ranges, not defaults or deployment values. Effective Agent deadline/input/output is the mathematical minimum of the present configured bounds and resolved Card bounds.

**Rationale**: Operators must make capacity policy explicit; Card limits remain exact Agent declarations.

## Decision 7: Cancel once, first terminal wins

**Decision**: Disconnect/deadline closes local work and, if an A2A task ID is known, sends at most one `tasks/cancel`. This is protocol propagation, not a retry. Task-not-cancelable/not-found cannot replace the local outcome. The first committed terminal fact wins.

## Decision 8: Strict one-line SSE

**Decision**: Each event is `data:` plus compact UTF-8 JSON on one line, then one blank line and flush. CR/LF are escaped by JSON encoding. Multiple data lines, non-data fields, oversized events, malformed JSON, and EOF without delimiter/terminal are failures.

## Decision 9: Compatibility

Version only surfaces with changed accepted behavior. New v4/v3/v2 documents coexist as historical files, not runtime alternatives. The first runtime implements targets only because no deployed consumer exists.

## Fallback Report

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
