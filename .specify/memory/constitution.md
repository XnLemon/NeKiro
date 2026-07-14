<!--
Sync Impact Report
- Version change: 1.0.0 -> 1.1.0
- Added principle: Runtime-Agnostic Platform, Not an Agent Framework
- Modified principles: Phase 1 Loop First (added explicit user-value test);
  Agent Card as Common Language; Control/Data Plane Ownership; Contract and
  Compatibility First; Router-Mediated Invocation; Explicit Failure and Secret
  Safety; Spec-Driven Delivery and Independent Review
- Added section: Product Boundary
- Removed sections: none
- Templates: plan-template.md updated; spec-template.md updated;
  tasks-template.md updated; checklist-template.md no change required;
  constitution-template.md no change required; commands directory not present
- Runtime guidance: AGENTS.md, README.md, and
  docs/architecture/phase-1-spec.md updated; docs/handoffs/CURRENT.md updated
- Active plan: specs/003-workspace-installation-contracts/plan.md re-checked and
  updated
- Deferred items: none
-->

# NeKiro Agent Operating Platform Constitution

## Core Principles

### I. Phase 1 Loop First

Every Phase 1 feature MUST directly support the demonstrable loop
`Register -> Discover -> Install -> Invoke -> Record`. Work outside that loop
MUST be deferred unless an ADR documents why it is a blocking prerequisite.
The platform is an Agent Operating Platform; Marketplace presentation is not
the first-stage product. Every feature MUST identify the Agent developer,
platform operator, or Workspace user outcome it improves; completing internal
infrastructure alone is not evidence of product value.

### II. Runtime-Agnostic Platform, Not an Agent Framework

NeKiro MUST manage independently implemented Agents through versioned platform
contracts and supported interoperability protocols. It MUST NOT become a
general LLM, tool, planner, workflow graph, memory, RAG, session, or Agent
execution framework. Frameworks such as `trpc-agent-go` MAY be supported as
Agent Runtimes or protocol adapters, but core Control Plane and Data Plane
services MUST NOT depend on a full Agent Runtime framework. Platform value MUST
remain intact when an Agent changes its internal language, model, or framework.

### III. Agent Card Is the Common Language

Agent Card is the versioned, declarative contract used to exchange Agent
identity, capabilities, schemas, protocol endpoint, authentication type,
permissions, and limits. Registry is its sole source of truth. Dynamic health,
latency, success rate, secrets, source code, and invocation statistics MUST NOT
be stored in the Card.

### IV. Control Plane and Data Plane Own Their Boundaries

Frontend MUST call only the Gateway. Gateway MUST call Agents only through the
A2A Router. Registry, Discovery, Workspace, Dispatch, Router, and Ledger MUST
write only data they own. Cross-process communication MUST use versioned
contracts, and the Router MUST NOT import Control Plane internals or query its
tables directly. Logical boundaries do not require premature microservices.

### V. Contracts and Compatibility Precede Implementation

JSON Schema, OpenAPI, and the versioned A2A Profile are the language-neutral
facts for cross-boundary data. Agent versions and Agent Card Schema versions
MUST remain distinct. Breaking field or semantic changes MUST increment the
contract version and include migration impact. Go and TypeScript types are
contract consumers, never competing facts.

### VI. Every Managed Invocation Traverses the Router

User-to-Agent and Agent-to-Agent calls managed by the platform MUST pass
through the A2A Router. Each call MUST preserve `invocation_id`, `root_task_id`,
`trace_id`, and optional `parent_invocation_id`. Ledger facts MUST be append-only
and distinguish success, failure, timeout, cancellation, routing failure, and
protocol failure. Result transport and auditable facts MUST remain separate so
Agent output is not silently persisted as Ledger data.

### VII. Failures Are Explicit and Secrets Stay Out

The fallback addition budget is zero unless a documented product, contract,
ADR, Runbook, SLO, or caller policy proves otherwise. Missing, empty, invalid,
not found, forbidden, disabled, and dependency failure states MUST NOT collapse
into the same response. Required configuration MUST fail at its owning boundary.
Cards, logs, public errors, events, and Ledger records MUST NOT contain API keys,
tokens, credentials, or internal dependency details.

### VIII. Delivery Is Spec-Driven and Independently Reviewed

Behavior changes MUST progress through `observe -> specify -> clarify -> plan ->
tasks -> analyze -> implement -> tests -> review -> converge`. Implementation
MUST not introduce behavior absent from the accepted Spec and Tasks. This
project intentionally implements the approved behavior before adding its mapped
tests; TDD is not a mandatory workflow. A Review Agent that did not implement
the module MUST assess it against the Spec, Plan, Tasks, contracts, and this
constitution. Passing tests do not substitute for Review.

## Product Boundary

- NeKiro owns Agent registration, publication, discovery, Workspace
  installation and permission acceptance, exact-version resolution, managed
  invocation routing, and cross-Agent Ledger lineage.
- Agent Runtimes own model calls, prompts, tools, planning, workflows, memory,
  knowledge retrieval, session execution, and runtime-internal telemetry.
- `sdks/agent-sdk` MUST remain a thin integration layer for Agent Card
  conformance, platform context propagation, and nested calls through the A2A
  Router. Runtime features MUST stay in adapters or external frameworks.
- Phase 1 acceptance MUST include at least two runnable sample Agents backed by
  different runtime implementations. Their managed nested call MUST traverse
  the Router and produce one correlated Ledger lineage.
- A proposed core feature that is useful only to one Agent Runtime MUST be
  rejected, moved to that Runtime's adapter, or justified by an ADR as a
  cross-runtime platform requirement.

## Platform Constraints

- Console: React, Vite, TypeScript, TailwindCSS, and the shared shadcn/ui system.
- Control Plane and A2A Router: Go; Node.js MUST NOT implement backend services.
- Storage: PostgreSQL through `pgx/v5`, with explicit module data ownership.
- A2A interoperability: `github.com/a2aproject/a2a-go`, pinned by the A2A Profile.
- Backend HTTP boundaries use Go standard library facilities unless an ADR
  approves a replacement.
- Full Agent Runtime frameworks MUST remain outside Control Plane and Router
  core dependencies; protocol libraries and isolated adapters are permitted.
- Phase 1 MUST avoid speculative queues, search clusters, schedulers, runtime
  orchestration, billing, rating, federation, and premature service splitting.

## Delivery Workflow

1. Observe the current repository, contracts, ownership, dependencies, and
   active Spec without modifying business code.
2. Create or revise `specs/<feature>/spec.md`; resolve contract and domain
   ambiguity before planning.
3. Produce `plan.md` and required research, data model, contract, and quickstart
   artifacts; pass the Constitution Check.
4. Derive `tasks.md` with dependencies, parallel markers, module ownership, and
   disjoint write scopes; run consistency analysis before implementation.
5. Implement only approved tasks. New ambiguity returns the work to the Spec.
6. Add tests after implementation and trace them to acceptance scenarios,
   failure semantics, and compatibility requirements.
7. Run independent Review. Findings update the Spec or Tasks before fixes;
   fixes require a fresh independent Review.
8. Commit each logical implementation with repository-local identity
   `Nene7ko_ <1604009816@qq.com>`.

## Governance

This constitution is derived from and subordinate only to the repository root
`AGENTS.md`, which remains the full project charter. Amendments MUST state their
reason, impact, compatibility implications, and required migration. Constitution
versions use semantic versioning: MAJOR for incompatible governance changes,
MINOR for new or materially expanded principles, and PATCH for clarifications.

Every Plan MUST perform the Constitution Check before design work and again
after design. Every independent Review MUST verify compliance. When this file,
`AGENTS.md`, a feature Spec, or an ADR conflict, implementation stops until the
conflict is resolved in the higher-order artifact and propagated downward.

**Version**: 1.1.0 | **Ratified**: 2026-07-13 | **Last Amended**: 2026-07-13
