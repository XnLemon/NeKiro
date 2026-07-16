# Research: Deterministic Direct A2A Sample

## Decision 1: Runtime approach

- **Decision**: Implement Runtime B directly against
  `github.com/a2aproject/a2a-go/a2asrv.RequestHandler` and serve it with
  `a2asrv.NewJSONRPCHandler`.
- **Rationale**: The library and version are pinned by the active A2A Profile,
  its public server surface implements JSON-RPC and SSE framing, and a direct
  adapter is observably distinct from the later SDK-backed caller Runtime.
- **Alternatives considered**: A full Agent framework was rejected because it
  adds behavior outside this sample and risks framework leakage. A hand-written
  JSON-RPC/SSE server was rejected because it would compete with the pinned
  protocol library and weaken conformance evidence.

## Decision 2: Deterministic identity

- **Decision**: Derive output Message, Task, context, and artifact IDs from a
  domain-separated SHA-256 digest of the caller-supplied message identity.
- **Rationale**: The same request produces stable fixtures while distinct
  messages remain isolated under concurrency. No clock, random source, or
  fallback identity is required.
- **Alternatives considered**: Fixed global IDs collide under concurrent
  acceptance. Random UUIDs make fixture assertions unstable. Missing message
  IDs are rejected rather than replaced.

## Decision 3: Fixture instruction and result form

- **Decision**: Require exactly one A2A `DataPart` with `fixture` and `value`.
  Supported fixture values are `success`, `stream-success`, `failure`, and
  `hold`. Success returns a structured `DataPart`.
- **Rationale**: Structured input/output supports exact JSON assertions and
  prevents text parsing from becoming a second implicit contract. Explicit
  fixture selection enforces the zero-fallback policy.
- **Alternatives considered**: Text commands were rejected as ambiguous.
  Treating absent fixture as success was rejected as an inferred default.

## Decision 4: Task lifecycle

- **Decision**: Keep Runtime Task snapshots in a mutex-protected in-memory map.
  Successful streams become completed; hold streams remain working until one
  explicit cancel changes them to canceled. `tasks/get` returns a clone with
  the requested bounded history.
- **Rationale**: A2A task operations need coherent state, while persistence and
  durable audit are explicitly Router Ledger responsibilities. A one-process
  store proves protocol behavior without platform database access.
- **Alternatives considered**: PostgreSQL or filesystem persistence violates
  sample ownership and adds no T005 value. Pre-seeded tasks do not prove that
  message and task operations share identity.

## Decision 5: Failure and unsupported operations

- **Decision**: Invalid fixture requests return `a2a.ErrInvalidParams`;
  deterministic business failure returns a stable explicit error; missing and
  terminal task operations use `a2a.ErrTaskNotFound` and
  `a2a.ErrTaskNotCancelable`. Non-profile methods return
  `a2a.ErrUnsupportedOperation`.
- **Rationale**: These errors remain distinct through the pinned server's
  official JSON-RPC mapping and do not fabricate successful values.
- **Alternatives considered**: Empty Messages, zero Tasks, catch-and-success,
  retries, and compatibility branches were rejected.

## Fallback Inventory

| Candidate | Classification | Evidence and behavior |
| --- | --- | --- |
| Missing fixture selects success | Remove | No policy evidence; invalid params is explicit. |
| Missing message ID generates UUID | Remove | No policy evidence; invalid params is explicit. |
| Unknown task returns zero Task | Remove | Active Profile requires task-not-found. |
| Terminal cancel returns existing Task | Remove | Active Profile requires task-not-cancelable. |
| Request context ends held stream | Keep (not fallback) | Standard cancellation semantics terminate work without alternate result. |

No fallback is added or retained.
