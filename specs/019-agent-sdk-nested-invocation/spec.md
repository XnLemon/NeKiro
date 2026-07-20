# Feature Specification: Agent SDK Nested Invocation

**Feature Branch**: `019-agent-sdk-nested-invocation`

**Created**: 2026-07-16

**Status**: Active — T000 resolved

**Input**: Parent Spec 010 T009, accepted ADR 0006, and the active Agent
Router API v1 contract.

## Clarification Resolved (T000)

**Decision**: Add a new Control Plane Internal v3 endpoint
`/internal/v3/resolve-installed-version` that resolves the deterministic
installed Agent Card version given workspace, agent, and capability.

**Rationale**:

1. Control Plane owns Workspace/Installation data and is the authority for
   installed versions.
2. Router does not make version selection decisions; it queries the Control
   Plane for the exact pinned `installedVersion` from the enabled Installation.
3. The new endpoint is additive and does not modify the existing v2
   `resolve-agent` contract.
4. The resolution is deterministic: one enabled Installation per
   (workspace, agent) pair has exactly one pinned `installedVersion`.
5. After obtaining the version, the Router calls the existing v2
   `resolve-agent` endpoint for full Card and Installation authorization.

**Contract**: `contracts/openapi/control-plane-internal.v3.yaml` defines the
new `ResolveInstalledVersion` operation. The Router resolution client gains a
`ResolveInstalledVersion` method.

## Context

An Agent that is already running under a managed Invocation needs to request
work from another Agent without receiving authority to choose an endpoint,
forge Workspace identity, or create an untracked child call. This feature adds
the smallest platform SDK and Router boundary needed for that nested hop. It
does not add an Agent Runtime, model/tool orchestration, or a second Ledger.

## User Scenarios & Testing

### User Story 1 - Make a trusted nested call (Priority: P1)

As a managed Agent, I need to ask another installed Agent to perform a
capability so the platform records a child Invocation linked to my current
call.

**Independent Test**: Send an authenticated Agent Router v1 request with a
valid parent Invocation and inspect the child result and Ledger lineage; the
child has a new identity but the parent's Workspace, root Task, Trace, and
parent relationship.

**Acceptance Scenarios**:

1. **Given** an authenticated Agent matches a running parent target, **when**
   it submits valid target work, **then** Router creates and dispatches one
   child Invocation through the existing A2A pipeline.
2. **Given** the child succeeds or fails, **when** the request completes,
   **then** the result/error preserves the child correlation and the Ledger
   contains no input, output, token, or endpoint.

### User Story 2 - Reject forged or unusable nested calls (Priority: P1)

As a platform operator, I need nested calls to fail before child acceptance
when identity, parent state, request shape, or result mode is invalid.

**Independent Test**: Exercise missing/unknown credentials, mismatched Agent,
non-running or missing parent, trusted-field injection, invalid identifiers,
and JSON/SSE mode mismatch; verify no child `created` fact or target request.

**Acceptance Scenarios**:

1. **Given** a request that supplies caller, Workspace, root Task, Trace,
   endpoint, credential, or child identity fields, **when** it is decoded,
   **then** Router rejects it as validation failure without accepting a child.
2. **Given** an unknown or forbidden Agent credential, **when** the request is
   authenticated, **then** Router returns the exact pre-correlation auth error
   and does not query the parent or target.
3. **Given** a valid credential but missing, foreign, or non-running parent,
   **when** the request is checked, **then** Router returns not-found/forbidden
   or conflict as defined by the active contract and creates no child fact.

### User Story 3 - Keep the SDK thin and runtime-neutral (Priority: P1)

As an Agent Runtime integrator, I need a small SDK that validates inherited
platform context and calls only the Agent Router API, so changing Runtime,
language, or framework does not change platform semantics.

**Independent Test**: Use the SDK with a valid context and an HTTP test Router,
then attempt missing/invalid context and confirm no request is sent; inspect
the package for model, tool, workflow, memory, retry, cache, endpoint, or
credential-inference behavior.

**Acceptance Scenarios**:

1. **Given** a valid inherited context and explicit Router credential/URL,
   **when** the SDK invokes a nested target, **then** it sends only the active
   request fields and propagates the parent Invocation ID and result mode.
2. **Given** malformed or incomplete context, **when** the SDK is called,
   **then** it fails locally without synthesizing identity or correlation.

## Edge Cases

- Duplicate or trailing JSON members are rejected.
- An authenticated Agent whose parent target differs receives `FORBIDDEN`.
- A parent that is terminal, absent, or from another Workspace cannot create a
  child, and no empty-success response is returned.
- Redirects, alternate Router destinations, retries, and direct Agent endpoint
  calls are not followed.
- A child response is transient; result content is never written to Ledger or
  logged by the platform boundary.

## Requirements

### Functional Requirements

- **FR-001**: Router MUST expose only the versioned Agent-facing `/agent/v1/invocations` boundary for SDK nested calls.
- **FR-002**: Agent authentication MUST bind one explicit opaque credential to one exact Agent ID; missing, duplicate, unknown, and mismatched bindings MUST fail without defaults.
- **FR-003**: The nested request MUST contain only `parentInvocationId`, `targetAgentId`, `capability`, `input`, and `stream`; trusted identity, lineage, endpoint, credential, and child ID fields MUST be rejected.
- **FR-004**: Router MUST load the committed parent before child acceptance, require its status to be `running`, and require its target Agent to equal the authenticated Agent.
- **FR-005**: Router MUST derive child caller, Workspace, root Task, Trace, and parent facts from the committed parent and MUST generate a new child Invocation ID.
- **FR-006**: Child execution MUST reuse the existing exact resolution, A2A transport, Ledger lifecycle, media negotiation, deadline, and result validation semantics.
- **FR-007**: The SDK MUST validate inherited platform context and explicit configuration locally, propagate only the parent reference and untrusted work, and perform exactly one Router request without retry or redirect.
- **FR-008**: Authentication, shape, parent, and mode failures before child `created` commit MUST use pre-correlation errors and create no child Ledger fact or Agent request.
- **FR-009**: After child acceptance, results and errors MUST preserve child Invocation/root Task/Trace correlation; successful result content remains transient.
- **FR-010**: No SDK, Router nested handler, error, log, or Ledger fact MAY contain credentials, tokens, endpoints, Agent input, result values, or raw dependency detail.
- **FR-011**: The feature MUST remain runtime-neutral: no model, tool, planner, workflow, memory, retry, cache, or general Agent execution abstraction is allowed in the SDK or Router core.
- **FR-012**: Active `router-agent.v1.yaml`, Platform Error v4, Invocation Event 0.3, Result v1, and Result Stream Event v2 are the sole runtime contracts; no historical dual-read or compatibility fallback is permitted.

## Key Entities

- **Platform Context**: Trusted inherited Invocation, root Task, Trace, Workspace, and Agent identity presented by the managed transport and validated before SDK use.
- **Nested Invocation Request**: Untrusted target capability/input and the existing parent Invocation reference.
- **Agent Binding**: Explicit deployment-owned mapping from one opaque bearer credential to one Agent ID; never a Card or Ledger field.
- **Child Invocation**: A newly identified Ledger lifecycle whose parent, Workspace, root Task, Trace, and caller are derived from the committed parent.

## Runtime/Platform Boundary

- **Platform-owned behavior**: Agent binding, parent validation, child identity/lineage derivation, Router dispatch, authorization, transport, and Ledger facts.
- **Runtime-owned behavior**: Agent model/tool/workflow execution and any interpretation of input or result content.
- **Cross-runtime proof**: A later Spec 020 sample uses this SDK from a different Runtime implementation to call the existing direct callee.

## Success Criteria

- **SC-001**: Every accepted nested call creates exactly one child Invocation with a new ID and exact parent/root/Trace correlation.
- **SC-002**: 100 invalid-auth, forged-context, parent-state, and mode cases create zero child Ledger facts and zero target requests.
- **SC-003**: A valid SDK call performs exactly one request to the configured Agent Router origin and never follows a redirect or retries.
- **SC-004**: Static inspection and contract tests find no secret, endpoint, input, output, or runtime-framework behavior in SDK/Router metadata surfaces.
- **SC-005**: The SDK package remains usable without importing Control Plane, Catalog, Workspace storage, Ledger storage, or any full Agent Runtime.

## Assumptions

- The Router Ledger already exposes committed Invocation detail reads and
  append semantics from Specs 014/018.
- Only Agent Card authentication type `none` is invocable in Phase 1; this
  feature does not introduce secret binding for target Agents.
- Router process wiring and deployment variables for Agent bindings are owned by
  the later parent acceptance task unless explicitly added by that task.
- JSON and SSE nested result handling reuse the active shared media and stream
  validators; no new result contract is introduced.

## Non-Goals

- Agent Runtime implementation, model/tool/workflow/memory APIs, or a generic
  framework abstraction.
- A second storage system, result replay, polling, caching, retries, or direct
  Agent endpoint access.
- The second Runtime sample and full cross-process E2E; those belong to Spec
  020 and Spec 021.

## Fallback Report

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
