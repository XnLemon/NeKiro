# Implementation Plan: Complete Invocation Contracts

**Branch**: `main` | **Date**: 2026-07-13 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from
`/specs/001-complete-invocation-contracts/spec.md`

## Summary

Complete the Phase 1 contract foundation before backend business modules begin.
The plan introduces direct same-request Invocation Result delivery through the
Gateway and Router, keeps result content separate from Ledger events, splits
directional internal APIs by service owner, makes terminal event combinations
coherent, publishes portable Agent Card semantic conformance rules, and expands
the pinned A2A profile into executable conformance cases.

Breaking contracts receive new versions; historical artifacts remain available
for migration evidence but are not implemented by the first backend runtime.

## Technical Context

**Language/Version**: Go 1.26 for contract mappings and validators; JSON Schema
2020-12 and OpenAPI 3.1 for language-neutral contract facts

**Primary Dependencies**: `github.com/a2aproject/a2a-go v0.3.15`,
`github.com/getkin/kin-openapi v0.142.0`,
`github.com/santhosh-tekuri/jsonschema/v6 v6.0.2`

**Storage**: N/A for result delivery; Invocation Results are transient and
Ledger facts remain append-only. This feature does not add database code.

**Testing**: Go contract/conformance tests added after contract implementation,
then `go test -count=1 ./...`, `go vet ./...`, and `git diff --check`

**Target Platform**: Cross-platform contract artifacts; Go validation on the
repository-supported Windows and Linux CI environments

**Project Type**: Contract library and API specification slice in a Go
monorepo

**Performance Goals**: No new runtime throughput target. Streaming contracts
MUST permit forwarding ordered chunks without requiring whole-result buffering.

**Constraints**: Result content cannot enter Ledger; Frontend cannot access the
Router; public errors remain fixed and secret-safe; fallback addition budget is
zero; historical contract identity cannot be silently rewritten

**Scale/Scope**: Four contract families: result delivery and directional APIs,
Invocation Event terminal semantics, Agent Card semantic conformance, and A2A
profile conformance

## Constitution Check

*GATE: Passed before research; re-checked and passed after design.*

- **Phase 1 loop — PASS**: The feature repairs the `Invoke -> Record` boundary
  and unblocks all later backend modules.
- **Ownership — PASS**: Gateway owns northbound normalization, Control Plane
  owns resolution, Router owns dispatch/result transport, and Ledger receives
  metadata-only lifecycle facts.
- **Contracts — PASS**: New breaking versions are explicit; historical files
  remain unchanged and migration impact is documented.
- **Invocation lineage — PASS**: Result and stream envelopes carry invocation,
  root task, and trace identifiers; Ledger and result transport are separate.
- **Failure safety — PASS**: Terminal combinations are discriminated, public
  messages stay fixed, and neither result nor secret content enters errors or
  Ledger facts.
- **SDD traceability — PASS**: Design artifacts map every requirement to exact
  target contract files; implementation tasks precede mapped tests.

## Design Decisions

### Direct Result Delivery

- `POST /v2/workspaces/{workspaceId}/invocations` is the result channel.
- `stream=false` returns one completed JSON Invocation Result.
- `stream=true` returns Server-Sent Events on the same response. Events are
  discriminated as accepted, chunk, completed, failed, canceled, or timed out.
- The request `stream` field and `Accept` header must agree; incompatible media
  negotiation returns `406` and is covered by contract tests.
- Pre-dispatch failures return a fixed Platform Error. Post-dispatch failures
  include invocation context without Agent output or dependency details.
- Results and chunks are transient. Disconnect does not create a result replay
  or polling store; Ledger remains available for lifecycle diagnosis.
- Exactly one terminal event ends a clean stream. EOF without one is interrupted
  delivery, and success/timeout/cancellation use first-terminal-wins semantics.

### Directional Internal APIs

- `control-plane-internal.v1.yaml` contains Control Plane-owned resolution only.
- `router-internal.v2.yaml` contains Router-owned dispatch, result streaming,
  Ledger event reads, and trace reads.
- The Control Plane and Router use different server destinations. The Router
  cannot resolve through its own API or query Registry storage directly.

### Version Strategy

- Preserve `control-plane.v1.yaml`, `router-internal.v1.yaml`, Agent Card `0.1`,
  and Invocation Event `0.1` as historical artifacts.
- Add Northbound API `v2`, Router Internal API `v2`, Agent Card Schema `0.2`,
  Invocation Event Schema `0.2`, Platform Error `v2`, and Invocation Result `v1`.
- The first backend runtime implements only the new active versions. No runtime
  currently publishes or consumes the historical versions, so there is no
  speculative dual-read compatibility branch.
- The A2A protocol remains `0.3.0`; its platform profile metadata advances to
  schema `0.2` because the wire protocol version itself is unchanged.

### Portable Semantic Rules

- JSON Schema remains the structural Agent Card contract.
- A versioned RFC 2119 semantic-rules document defines uniqueness and
  declared-permission invariants that JSON Schema cannot portably express.
- Positive and negative raw JSON fixtures plus a manifest are normative
  conformance cases. Every language validator implements named rule IDs and
  MUST produce the expected fixture decision.
- The project does not introduce a custom expression language, CEL, CUE, or
  JSON Logic evaluator for three stable invariants.

### A2A Profile Conformance

- The profile describes required methods, accepted event kinds, transient,
  terminal, and unsupported task states, plus required correlation headers.
- Language-neutral JSON-RPC fixtures verify wire shapes.
- The conformance manifest is strict and executable: it rejects duplicate or
  unknown members and unsafe paths, validates every metadata combination, and
  requires each listed rule and expected type to be asserted by the harness.
- Go tests compile and execute against the pinned SDK client methods
  `SendMessage`, `SendStreamingMessage`, `GetTask`, and `CancelTask` and their
  corresponding server handlers. Direct transport checks are supplemental.
- Decoding a result is not sufficient conformance. Agent-authored Message
  results and Task results must satisfy the profile's semantic invariants.

## Project Structure

### Documentation (this feature)

```text
specs/001-complete-invocation-contracts/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── result-delivery.md
│   ├── directional-internal-api.md
│   ├── agent-card-semantics.md
│   └── a2a-conformance.md
├── checklists/
│   └── requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
contracts/
├── agent-card/
│   └── v0.2/
│       ├── semantic-rules.md
│       └── conformance/
├── a2a-profile/
│   ├── v0.3.0.json
│   └── v0.3.0/
│       ├── profile.v0.2.json
│       └── conformance/
├── openapi/
│   ├── control-plane.v1.yaml
│   ├── control-plane.v2.yaml
│   ├── control-plane-internal.v1.yaml
│   ├── router-internal.v1.yaml
│   └── router-internal.v2.yaml
├── schemas/
│   ├── agent-card.v0.1.schema.json
│   ├── agent-card.v0.2.schema.json
│   ├── invocation-event.v0.1.schema.json
│   ├── invocation-event.v0.2.schema.json
│   ├── invocation-result.v1.schema.json
│   ├── invocation-result-stream-event.v1.schema.json
│   ├── platform-error.v1.schema.json
│   ├── a2a-profile.v0.2.schema.json
│   └── platform-error.v2.schema.json
├── contracts.go
├── validate.go
└── contracts_test.go

docs/contracts/
└── compatibility.md

docs/decisions/
└── 0002-invocation-result-transport-and-internal-api-direction.md

docs/architecture/
└── phase-1-spec.md

AGENTS.md
README.md
```

**Structure Decision**: This feature changes only contract facts, Go mappings,
conformance fixtures, and compatibility documentation. It does not create
Control Plane or Router runtime packages; those are separate feature Specs.

## Implementation Order

1. Add new versioned schema and semantic/profile artifacts while preserving
   historical files.
2. Add directional OpenAPI documents and direct result response contracts.
3. Update Go mappings and validators to consume active versions.
4. Update compatibility documentation and migration guidance.
5. After implementation, add contract and A2A conformance tests mapped to the
   Spec acceptance scenarios.
6. Run verification and an independent Review Agent; findings return to Tasks
   before fixes and require a fresh Review Agent.

## Complexity Tracking

No constitution violations require justification. New contract files are
versioned replacements, not additional runtime services or fallback paths.
