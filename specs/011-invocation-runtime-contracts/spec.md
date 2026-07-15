# Feature Specification: Invocation Runtime Contracts

**Feature Branch**: `codex/011-invocation-runtime-contracts`
**Created**: 2026-07-16
**Status**: Ready for implementation
**Input**: GitHub #20 / parent #19 contract and policy gate.

## Clarifications

### Session 2026-07-16

- Q: Which nested fields may an Agent SDK assert? A: Only the parent Invocation ID, target Agent, capability, input, and result mode. Router derives caller Agent identity from authentication and derives Workspace, root Task, Trace, and parent context from the committed parent record.
- Q: Which Card authentication types can Phase 1 invoke? A: `none` only. Registry continues accepting every Agent Card 0.2 authentication declaration; runtime rejects every other type with the distinct `AGENT_AUTH_UNSUPPORTED` outcome.
- Q: What is an accepted Invocation? A: The successful Router-owned `created` event commit. Before it, no Ledger fact and no Agent side effect exist.
- Q: How is a post-Agent Ledger failure represented? A: The live response is explicit non-success; committed audit history stays at its last non-terminal fact. Router does not fabricate a terminal Ledger fact and does not retry.
- Q: Where do deadline and size values come from? A: Required, strictly parsed deployment configuration with documented ranges and no defaults. The effective Agent deadline and content limits are the stricter configured/Card limits.

## User Scenarios & Testing

### User Story 1 - Trustworthy nested invocation (Priority: P1)

An Agent developer can use the thin SDK to invoke another installed Agent without being able to forge caller identity or lineage.

**Independent Test**: Authenticate as one Agent, submit a parent and target request, and prove the child derives exact parent Workspace/root/Trace values while rejecting a credential/parent mismatch before acceptance.

**Acceptance Scenarios**:

1. **Given** a running accepted parent whose target Agent matches the authenticated SDK identity, **When** the SDK requests a nested call, **Then** Router generates the child Invocation identity and derives its trusted lineage from the parent.
2. **Given** a missing, invalid, wrong-Agent, cross-Workspace, non-running, or terminal parent, **When** a nested request arrives, **Then** Router rejects it before `created` and creates no Ledger fact.
3. **Given** an Agent-facing request, **When** it supplies caller, Workspace, root Task, Trace, child Invocation, Card version, or endpoint fields, **Then** strict validation rejects the request.

### User Story 2 - Explicit runtime authentication support (Priority: P1)

An operator can predict whether a published Card is invocable without secrets being placed in a Card, event, error, or log.

**Independent Test**: Resolve Cards for all Agent Card 0.2 authentication types and verify `none` proceeds while each other type commits an `AGENT_AUTH_UNSUPPORTED` routing failure without an Agent request.

**Acceptance Scenarios**:

1. **Given** a Card with `authentication.type=none`, **When** routing succeeds, **Then** Router sends no authorization credential to the Agent.
2. **Given** any other declared type, **When** Router reaches authentication selection, **Then** it commits a failed routing terminal with the distinct unsupported-auth error and sends no Agent request.
3. **Given** a non-`none` Card, **When** it is registered or discovered, **Then** Catalog acceptance remains unchanged; only Phase 1 invocation support is restricted.

### User Story 3 - Auditable acceptance and interruption (Priority: P1)

A Workspace user and operator can distinguish a request that never became an Invocation, a normally terminalized Invocation, and an accepted Invocation whose Ledger persistence failed after an Agent side effect.

**Independent Test**: Validate the contract state matrix for failure before `created`, failure during routing, and Ledger loss after Agent output; verify it requires zero facts, a committed terminal fact, and explicit live dependency failure with only non-terminal durable history respectively.

**Acceptance Scenarios**:

1. **Given** initial Ledger append failure, **When** dispatch occurs, **Then** no Agent call occurs and no Ledger fact exists.
2. **Given** a route, exact-resolution, or unsupported-auth failure after acceptance, **When** the terminal append commits, **Then** history terminalizes from `routing`.
3. **Given** Ledger failure after an Agent side effect, **When** Router cannot commit the next or terminal fact, **Then** JSON returns dependency non-success or committed SSE ends with a correlated failed delivery event; the Ledger retains only its last non-terminal facts.
4. **Given** a post-side-effect persistence failure, **When** recovery is considered, **Then** no fabricated event, retry, alternate store, or reconciliation is performed.

### User Story 4 - Bounded cancellation and framing (Priority: P2)

An operator can run invocation transport with explicit deadlines and byte bounds, and callers receive deterministic cancellation and SSE behavior.

**Independent Test**: Validate that the contracts declare required configuration and exact outcomes for deadline, disconnect, oversized bodies/events, malformed SSE, and competing terminal results.

**Acceptance Scenarios**:

1. **Given** missing, blank, malformed, zero, negative, or out-of-range required runtime configuration, **When** a process starts, **Then** its owning boundary fails readiness/startup without a default.
2. **Given** deadline expiry or caller disconnect, **When** an A2A task ID is known, **Then** Router sends at most one `tasks/cancel` propagation request and never retries it.
3. **Given** `task-not-cancelable`, **When** local cancellation or timeout has won, **Then** the local outcome remains canceled or timed out and the protocol error does not replace it.
4. **Given** competing Agent, disconnect, and deadline outcomes, **When** one valid terminal Ledger commit wins, **Then** later outcomes create no event or response.
5. **Given** an oversized request, response, A2A event, or SSE data event, **When** its configured/Card limit is crossed, **Then** processing stops with an explicit size failure and no truncated success.

### Edge Cases

- The parent disappears, is in another Workspace, or changes from running before child acceptance.
- Authentication succeeds but binds to an Agent different from the parent target.
- A deadline expires in `pending`, `routing`, or `running`.
- Disconnect occurs before acceptance versus after SSE commitment.
- `tasks/cancel` returns task-not-found, task-not-cancelable, malformed data, or cannot be delivered.
- Terminal Ledger commit succeeds while a losing cancellation signal arrives concurrently.
- SSE contains CR/LF inside encoded JSON, multiple `data:` fields, an empty event, or EOF without a blank-line delimiter.

## Requirements

### Functional Requirements

- **FR-001**: The platform MUST expose Agent SDK nested invocation through a separately versioned, authenticated Agent-facing Router boundary.
- **FR-002**: Router MUST derive `caller.type=agent` and `caller.id` from the authenticated credential and MUST NOT accept them from request data.
- **FR-003**: Router MUST derive Workspace, root Task, Trace, and parent identity from one committed parent Invocation whose target Agent matches the authenticated Agent and whose state permits nesting.
- **FR-004**: Router MUST generate the child Invocation ID and MUST reject request-supplied trusted correlation, Card version, or endpoint fields.
- **FR-005**: The first Go SDK MUST support both JSON and SSE result modes without direct target endpoint access, retry, or result replay.
- **FR-006**: Phase 1 transport MUST support only Card authentication type `none`; other Card 0.2 types MUST remain registrable but fail invocation with `AGENT_AUTH_UNSUPPORTED` before Agent transport.
- **FR-007**: A successful `created` event commit MUST be the accepted-Invocation boundary, and no Agent side effect may precede it.
- **FR-008**: Pre-acceptance authentication, validation, media, authorization, connectivity, and initial Ledger failures MUST create no Ledger fact.
- **FR-009**: Routing failures MAY terminalize only from `routing`; cancellation and timeout MAY terminalize from `pending`, `routing`, or `running`; success MUST terminalize only from `running`.
- **FR-010**: A clean result or clean terminal stream event MUST NOT be emitted before the corresponding terminal Ledger fact commits.
- **FR-011**: Post-side-effect Ledger persistence failure MUST be an explicit correlated dependency non-success while durable metadata remains non-terminal; no terminal fact may be fabricated.
- **FR-012**: Runtime MUST NOT retry Ledger writes, Agent requests, or cancellation propagation, and MUST NOT introduce an alternate store or reconciliation loop.
- **FR-013**: Deadline, public/internal body, Agent response, A2A event, and SSE event limits MUST be required strictly parsed positive configuration within contract ranges and have no defaults.
- **FR-014**: The effective deadline/input/output limit MUST be the stricter applicable configured limit and exact resolved Card limit, without mutating the Card fact.
- **FR-015**: HTTP disconnect and deadline MUST propagate cancellation; when a task ID exists Router sends at most one `tasks/cancel` request, with no retry.
- **FR-016**: First successfully committed terminal outcome MUST win and all later events/results MUST be discarded.
- **FR-017**: Each SSE event MUST be one UTF-8 JSON value on exactly one `data:` line followed by one blank line; CR/LF in JSON is escaped, multi-line data is rejected, and every event is flushed.
- **FR-018**: Invocation metadata contracts MUST exclude input, output, chunks, credentials, endpoints, raw dependency errors, and runtime telemetry.
- **FR-019**: Breaking auth, shape, error, and framing changes MUST receive new versions and migration notes; historical artifacts MUST remain unchanged and MUST NOT be runtime fallbacks.

### Key Entities

- **Agent caller binding**: Required secret-bearing deployment association from one opaque credential to one exact Agent identity; secret value never enters contract facts.
- **Nested invocation request**: Untrusted parent reference, target, capability, input, and mode only.
- **Trusted child context**: Router-generated child identity plus parent-derived Workspace/root/Trace/caller lineage.
- **Invocation acceptance**: Successful durable `created` event commit.
- **Interrupted audit history**: Last committed non-terminal facts after an external side effect and a later Ledger failure.
- **Runtime limits**: Required operator-supplied positive values; absence is configuration failure.

### Runtime/Platform Boundary

- **Platform-owned behavior**: Agent caller authentication, parent validation, trusted lineage, exact routing, cancellation propagation, byte/deadline enforcement, result framing, and Ledger facts.
- **Runtime-owned behavior**: Agent model/tool/workflow/session behavior and whether the Agent task can honor cancellation.
- **Cross-runtime proof**: Later T009/T010 uses the same Agent-facing contract from a different Runtime while all nested traffic still traverses Router.

## Success Criteria

### Measurable Outcomes

- **SC-001**: Contract tests reject 100% of request attempts that supply trusted nested fields or mismatch credential and parent Agent identity.
- **SC-002**: All five Agent Card 0.2 authentication declarations have one deterministic Phase 1 outcome, with four unsupported types producing the distinct error and zero Agent requests.
- **SC-003**: The contract state matrix assigns one unambiguous result to every pre-acceptance, normal terminal, and post-side-effect persistence-failure case, including zero-fact and non-terminal-history requirements.
- **SC-004**: The contract state matrix permits cancellation/timeout from all three non-terminal stages, declares one committed terminal winner, and caps A2A cancellation propagation at one request.
- **SC-005**: Every public/internal body, Agent response, A2A event, and SSE event boundary declares a required no-default byte-limit source and one-line framing rule that can be executed by later runtime tests.
- **SC-006**: Historical contract files remain byte-identical, every new contract validates, and all focused/full contract/static checks pass.

## Assumptions

- Workspace exact authorization and Control Plane Internal v2 are already implemented and remain the authoritative resolution boundary.
- Agent caller credentials are explicit Router deployment secrets mapped to an Agent identity; a production secret manager is outside T001.
- No Invocation/Router runtime consumer is deployed, so breaking target versions require migration instructions but no dual-version compatibility window.

## Non-Goals

- Router, Dispatch, Ledger, SDK, sample Agent, persistence, or deployment runtime implementation.
- Secret storage/provider selection or support for `api_key`, bearer, OAuth2 client credentials, or mutual TLS Agent transport.
- Public cancellation endpoint, caller-selected timeout, result persistence/replay, retry, reconciliation, caches, alternate routes, or historical runtime fallback.

## Fallback Report

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
