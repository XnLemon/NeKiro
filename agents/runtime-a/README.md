# Runtime A

Runtime A is the isolated second sample Runtime for Issue #29. It uses
`trpc-agent-go` only inside this nested Go module for Agent/Runner/Event
execution. It uses the repository's active `a2a-go` JSON-RPC adapter at the
wire boundary and the thin `sdks/agent-sdk` for exactly one nested Router call.

The sample has no platform database access, no Runtime B package imports, no
direct target URL, no retry/cache/alternate route, and no configuration
defaults. Every setting below is required:

```text
RUNTIME_A_LISTEN_ADDR
RUNTIME_A_AGENT_ID
RUNTIME_A_ROUTER_URL
RUNTIME_A_ROUTER_TOKEN
RUNTIME_A_TARGET_AGENT_ID
RUNTIME_A_TARGET_CAPABILITY
RUNTIME_A_RESPONSE_LIMIT_BYTES
RUNTIME_A_EVENT_LIMIT_BYTES
NEKIRO_AGENT_CHALLENGE_DIRECTORY
```

`NEKIRO_AGENT_CHALLENGE_DIRECTORY` is an absolute, explicitly configured
directory used only to serve provider-owned one-time HTTP ownership proofs at
`/.well-known/nekiro/challenges/{challengeId}`. It has no default and is not a
platform secret store.

The child slice accepts JSON `message/send` requests with exactly one data part
containing `fixture: "success"` and a JSON `value`. Streaming and task storage
remain outside this slice and are covered by Spec 021. The result is a
deterministic Runtime A data message containing the validated child result.

Run the isolated tests with:

```powershell
go test ./...
go test -race ./...
go vet ./...
```

`RUNTIME_A_ROUTER_TOKEN` is an exact credential. It must not be logged,
trimmed, copied into A2A data, or placed in platform facts.
