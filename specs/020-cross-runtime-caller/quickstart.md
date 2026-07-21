# Quickstart: Cross-Runtime Caller Sample

## Prerequisites

- Go 1.26 and a clean checkout with Specs 015 and 019 merged.
- A running A2A Router and Runtime B, plus a Workspace-bound Runtime A Agent credential issued by deployment configuration.

## Isolated build and tests

```powershell
Set-Location agents/runtime-a
go test ./...
go test -race ./...
go vet ./...
```

The root platform regression remains separate:

```powershell
Set-Location ../..
go test ./...
```

On the current Windows development host, `go test -race ./...` is present and
the 100-call test is implemented, but execution requires a cgo C compiler that
is not installed. Linux CI must run that command before this child is closed.

## Required Runtime A configuration

Set every value explicitly before starting; there are no defaults:

```text
RUNTIME_A_LISTEN_ADDR=127.0.0.1:4103
RUNTIME_A_AGENT_ID=agent-runtime-a
RUNTIME_A_ROUTER_URL=http://127.0.0.1:4101
RUNTIME_A_ROUTER_TOKEN=<workspace-bound-opaque-token>
RUNTIME_A_TARGET_AGENT_ID=agent-runtime-b
RUNTIME_A_TARGET_CAPABILITY=fixture
RUNTIME_A_RESPONSE_LIMIT_BYTES=1048576
RUNTIME_A_EVENT_LIMIT_BYTES=1048576
```

Run:

```powershell
go run ./cmd/runtime-a
```

## Focused proof

Invoke Runtime A through the Gateway/Router path with a JSON input such as `{"fixture":"success","value":"cross-runtime"}`. The expected result is one valid A2A agent message whose data contains `agent=runtime-a`, the child Invocation ID, and Runtime B's deterministic result. Query the invocation trace and verify that the child has the same Workspace/root Task/Trace identifiers and the root Invocation as parent.

The test suite also proves the root A2A -> trpc Runner -> real Agent SDK HTTP -> Router fixture -> deterministic callee result path, malformed configuration/input, SDK rejection, correlation mismatch, direct URL absence, content exclusion, and 100 concurrent calls. The final parent acceptance feature replaces the Router fixture with real Router/Ledger/Runtime B processes; SSE is intentionally deferred to Spec 021.
