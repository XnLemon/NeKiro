# NeKiro Workspace Client SDK for Go

`clientsdk` lets application code invoke Agents already installed in one
NeKiro Workspace. It talks only to the Control Plane Gateway. It never accepts
an Agent endpoint, Router address, version, Release, Card digest, or Agent
credential.

Installation remains a separate, explicit Gateway workflow:

```text
Discover -> accept permissions -> install into Workspace
                                      |
                                      v
                         clientsdk Invoke / InvokeStream
```

The SDK does not discover, install, enable, upgrade, deploy, or replace an
Agent during invocation.

## Configure one Workspace client

```go
client, err := clientsdk.NewClient(clientsdk.Config{
    HTTPClient:            &http.Client{Timeout: 30 * time.Second},
    GatewayOrigin:         "https://api.nekiro.dev",
    WorkspaceID:           "workspace-production",
    ApplicationCredential: os.Getenv("NEKIRO_APPLICATION_CREDENTIAL"),
    RequestLimitBytes:     1 << 20,
    ResponseLimitBytes:    4 << 20,
    StreamEventLimitBytes: 256 << 10,
})
```

Every field is required. The Gateway origin must be one exact canonical
HTTP(S) origin, and the caller supplies its own HTTP timeout/transport policy
and all byte limits. `NewClient` clones the HTTP client and rejects redirects;
it does not select `http.DefaultClient`, retry, normalize the origin, or invent
limits.

The application credential is an opaque Bearer supplied out of band and
mapped by Gateway to the existing Workspace Owner in Phase 1. Keep the raw
value in process-local secret configuration. Do not commit, print, serialize,
or pass it in an invocation body. The SDK neither issues nor persists it and
has no credential accessor.

## Invoke an installed Agent

```go
result, err := client.Invoke(ctx, clientsdk.InvokeRequest{
    AgentID:    "summarizer",
    Capability: "document.summarize",
    Input:      json.RawMessage(`{"document":"..."}`),
})
```

The per-call request has exactly Agent ID, capability, and a duplicate-free
JSON object. Gateway owns Workspace authorization, installed-version and
verified-Release resolution, endpoint selection, Router dispatch, and
Invocation/Task/Trace assignment. A successful `Result` exposes those three
correlation identifiers and the raw Agent output.

## Handle typed platform failures

```go
var platformError *clientsdk.PlatformError
if errors.As(err, &platformError) {
    switch platformError.Code {
    case contracts.ErrorCodeAgentNotInstalled:
        // Installation is required before another invocation.
    case contracts.ErrorCodeInstallationDisabled:
        // The Workspace owner must explicitly enable the Installation.
    case contracts.ErrorCodeAgentReleaseRevoked:
        // The exact installed Release is no longer invocable.
    }
}
```

`PlatformError` contains only validated HTTP status, stable code, Trace, and an
optional complete Invocation/root Task pair. `Correlated()` reports whether
that pair exists. Raw error bodies, fixed messages, credentials, provider
details, and unknown members are never retained or exposed. Transport and
caller-context errors remain local errors and preserve `errors.Is`.

## Consume a live stream

```go
stream, err := client.InvokeStream(ctx, request)
if err != nil {
    return err
}
defer stream.Close()

for {
    event, err := stream.Recv()
    if errors.Is(err, io.EOF) {
        break
    }
    if err != nil {
        return err
    }
    consume(event)
}
```

One Stream has one consumer. `Recv` validates the first accepted event,
contiguous sequence and chunk indices, correlation, terminal event, and every
bounded compact SSE frame. Completion is clean only after a terminal event is
followed by actual EOF. Calling `Close` before that EOF—including immediately
after receiving terminal—returns an error wrapping
`contracts.ErrRuntimeStreamInterrupted`. Cancellation comes from the caller's
context; the SDK does not reconnect, replay, retry, or poll Ledger content.

See the compiled package example in `example_test.go` and the active wire/API
contract in `specs/025-workspace-client-sdk/contracts/client-sdk-api.md`.
