# Go Client SDK Contract

## Purpose

This document freezes the public application library surface for Issue #51.
The language-neutral wire facts remain Northbound Invocation v4, Invocation
Result v1, Result Stream Event v2, and Platform Error v4. This API does not
replace or revise those contracts.

## Package

```go
import clientsdk "github.com/Nene7ko/NeKiro/sdks/client-sdk"
```

The package may import `github.com/Nene7ko/NeKiro/contracts`. It must not import
the Agent SDK or any `apps/**/internal` package.

## Configuration

```go
type Config struct {
    HTTPClient            *http.Client
    GatewayOrigin         string
    WorkspaceID           string
    ApplicationCredential string
    RequestLimitBytes     int64
    ResponseLimitBytes    int64
    StreamEventLimitBytes int64
}

func NewClient(config Config) (*Client, error)
```

Every field is required and exact. `NewClient` does not trim, infer, normalize,
or default a field. It clones `HTTPClient` and rejects redirects without
mutating the caller's instance. A nil HTTP client does not select
`http.DefaultClient`.

## Invocation request

```go
type InvokeRequest struct {
    AgentID    string
    Capability string
    Input      json.RawMessage
}
```

This exact three-field export surface is intentional. Result mode is selected
by the method. No public request field may be added for Workspace, credential,
endpoint, Router, version, Release, Card digest, correlation, or Agent secret.

## Non-streaming call

```go
type Result struct {
    InvocationID string
    RootTaskID   string
    TraceID      contracts.TraceID
    Output       json.RawMessage
}

func (client *Client) Invoke(
    ctx context.Context,
    request InvokeRequest,
) (*Result, error)
```

`Invoke` sends exactly one request with `stream=false` and `Accept:
application/json`. A successful return has passed strict Result v1, media,
Trace-header, and body/header correlation validation. The returned Output is a
copy of the validated raw result value.

## Streaming call

```go
type StreamEvent = contracts.InvocationResultStreamEventV2

type Stream struct { /* private state */ }

func (client *Client) InvokeStream(
    ctx context.Context,
    request InvokeRequest,
) (*Stream, error)

func (stream *Stream) Recv() (StreamEvent, error)
func (stream *Stream) Close() error
```

`InvokeStream` sends exactly one request with `stream=true` and `Accept:
text/event-stream`. It returns only after status, media, and exactly one Trace
header are valid.

`Recv` returns every validated accepted/chunk/terminal event to the caller. It
returns clean `io.EOF` only after a valid terminal event is followed by actual
transport EOF. Early EOF, post-terminal data, and correlation change are errors.

`Close` always closes the body. Before clean EOF it records and returns an error
wrapping `contracts.ErrRuntimeStreamInterrupted`, including when terminal was
read but EOF was not yet confirmed. After clean EOF it returns nil. Repeated
Close is idempotent and returns the same recorded outcome. A Stream has one
consumer and is not safe for concurrent `Recv`/`Close` calls.

## Typed Gateway error

```go
type PlatformError struct {
    StatusCode   int
    Code         contracts.PlatformErrorCode
    TraceID      contracts.TraceID
    InvocationID string
    RootTaskID   string
}

func (platformError *PlatformError) Error() string
func (platformError *PlatformError) Correlated() bool
```

Applications use `errors.As(err, *PlatformError)` to branch on a validated
Gateway failure. `Error()` contains only the status and stable code. The type
does not retain raw response bytes, fixed message text, credential material, or
dependency detail.

`Correlated()` is true only when Invocation ID and Root Task ID are both
present. The SDK never fabricates this pair.

## HTTP response matrix

| HTTP | Allowed Platform Error v4 codes | Allowed shape |
| --- | --- | --- |
| 400 | `VALIDATION_ERROR` | pre-correlation |
| 401 | `UNAUTHENTICATED` | pre-correlation |
| 403 | `FORBIDDEN`, `CAPABILITY_NOT_ALLOWED` | pre-correlation |
| 404 | `NOT_FOUND`, `AGENT_NOT_INSTALLED` | pre-correlation |
| 406 | `NOT_ACCEPTABLE` | pre-correlation |
| 409 | `CONFLICT`, `INSTALLATION_DISABLED`, `AGENT_DISABLED`, `AGENT_RELEASE_UNPUBLISHED`, `AGENT_RELEASE_SUSPENDED`, `AGENT_RELEASE_REVOKED`, `CANCELED` | pre or correlated |
| 413 | `PAYLOAD_TOO_LARGE` | pre-correlation |
| 500 | `INTERNAL_ERROR` | pre or correlated |
| 502 | `AGENT_AUTH_UNSUPPORTED`, `AGENT_RESPONSE_TOO_LARGE`, `AGENT_EXECUTION_FAILED`, `A2A_PROTOCOL_ERROR` | correlated |
| 503 | `ROUTE_NOT_FOUND`, `AGENT_UNAVAILABLE`, `DEPENDENCY_ERROR` | pre or correlated |
| 504 | `TIMEOUT` | pre or correlated |

Any other 3xx/4xx/5xx status, code, shape, fixed message, media type, member,
Trace header, or body/header correlation is a local response-contract error,
not a `PlatformError`.

## Credential safety

- The configured credential is written to exactly one `Authorization: Bearer`
  header per request.
- It never appears in the request body, public result, `PlatformError`, local
  error text, Stream event, example output, or accessor.
- The SDK performs no credential issuance, persistence, hashing, rotation,
  revocation, retry, refresh, or alternate lookup.
- Redirects are not followed, preventing credential forwarding to another
  destination.

## Compatibility

- The SDK consumes Northbound Invocation v4 only.
- It does not probe, retry, or fall back to historical v3 invocation routes.
- Adding a public request field that permits routing/version/credential input,
  changing an existing field type, or relaxing the status/phase matrix is a
  breaking SDK contract change and requires a future Spec.
