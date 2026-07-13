# Quickstart: Validate Complete Invocation Contracts

This guide is runnable after all implementation and test tasks in `tasks.md`
are complete. It validates contract artifacts only; no Control Plane, Router, or
PostgreSQL process is required for this feature.

## Prerequisites

- Go version declared by `go.mod`
- Repository dependencies downloaded with checksum verification enabled
- Current working directory at repository root

## 1. Validate All Contract Mappings

```powershell
go test -count=1 ./contracts
```

Expected outcomes:

- Active JSON Schemas compile and accept their valid examples.
- Historical schemas remain parseable and unchanged.
- Northbound v2, Control Plane Internal v1, and Router Internal v2 OpenAPI
  documents load and map to the intended Go DTOs.
- The A2A Profile Schema version and pinned Go module version agree.

## 2. Validate Result Delivery and Ledger Separation

```powershell
go test -count=1 ./contracts -run 'TestInvocationResult|TestInvocationResultStream|TestInvocationEvent'
```

Expected outcomes:

- Non-streaming result fixtures preserve arbitrary valid JSON output.
- Non-streaming results and streaming chunks preserve legal large JSON number
  tokens such as top-level and nested `1e400` without weakening duplicate-member
  rejection or typed envelope constraints.
- Streaming accepted/chunk/terminal sequences validate in order.
- Event after terminal, duplicate terminal, EOF-without-terminal fixture, and
  contradictory terminal error codes are rejected.
- Raw `INV-CORR-001` fixtures reject any nested error whose invocation,
  root-task, or trace identifier differs from the enclosing event.
- Resolution requests and errors preserve existing correlation, while Router
  Ledger/trace reads expose dependency-only unavailable semantics.
- Post-dispatch HTTP errors require all correlation identifiers, non-streaming
  results are bound to their request context, and duplicate-member public DTO
  payloads are rejected before typed decoding.
- Invocation Event fixtures cannot contain result or chunk content.

## 3. Validate Agent Card Portability

```powershell
go test -count=1 ./contracts -run 'TestAgentCardConformance'
```

Expected outcomes:

- Every raw fixture matches the manifest decision.
- Duplicate skill ID maps to `AC-SEM-001`.
- Duplicate permission ID maps to `AC-SEM-002`.
- Undeclared or cross-version permission reference maps to `AC-SEM-003`.
- A structurally valid but semantically invalid Card is rejected.
- Endpoint URIs containing userinfo are structurally rejected.
- Manifests with missing/null required fields, duplicate members, or unsafe
  fixture paths are rejected before filesystem access.

## 4. Validate A2A Profile Compatibility

```powershell
go test -count=1 ./contracts -run 'TestA2AProfileConformance'
```

Expected outcomes:

- Fixed wire fixtures exercise message send/stream and task get/cancel through
  all four pinned SDK client methods and matching server handlers.
- Message, Task, status update, and artifact update variants are recognized.
- Bad JSON-RPC envelopes, semantically empty Messages, zero-valued Tasks,
  unsupported states, mismatched identities, and incomplete streams are
  rejected.
- Manifests with duplicate members, unsafe paths, invalid metadata combinations,
  unknown rules, or type/rule claims that the harness does not execute are
  rejected before they can claim coverage.
- JSON-RPC response cases reject both/neither result and error, unsupported ID
  types, and cross-wired `protocolError` classifications.
- Profile operations reject result, event, or error fields owned by another
  method variant.
- All required NeKiro context headers are emitted.

## 5. Run Repository Static Verification

```powershell
go test -count=1 ./...
go vet ./...
git diff --check
```

`go test -race` is an additional CI/platform check when a working CGO toolchain
is available; it is not claimed on Windows environments where CGO is disabled.

## 6. Review Migration Evidence

Confirm that:

- historical v1/0.1 artifacts still exist;
- active versions and migration notes are listed in
  `docs/contracts/compatibility.md`;
- ADR 0002 records direct result delivery and directional internal APIs;
- `docs/architecture/phase-1-spec.md` points to the active contract versions;
- no runtime compatibility fallback was added for versions that never had a
  deployed consumer.
