# Data Model: Workspace Client SDK

This feature adds application-local library values only. It creates no database
schema, Registry fact, Workspace record, credential record, or Ledger field.

## Client Configuration

| Field | Meaning | Validation |
| --- | --- | --- |
| HTTP client | Application-owned transport and timeout policy | Required non-nil `*http.Client`; cloned by the SDK; redirect hook replaced with rejection |
| Gateway origin | Only northbound platform destination | Required exact canonical HTTP(S) origin; no userinfo, path, query, fragment, surrounding whitespace, trailing slash, or implicit normalization |
| Workspace ID | Fixed Workspace authorization scope | Required platform-safe identifier |
| Application credential | Opaque Gateway Bearer mapped to the Owner principal | Required exact printable non-whitespace value; never trimmed, serialized, logged, returned, or exposed after construction |
| Request limit bytes | Maximum complete encoded invocation request | Required integer in the active runtime byte-limit range |
| Response limit bytes | Maximum JSON result or HTTP error body | Required integer in the active runtime byte-limit range |
| Stream event limit bytes | Maximum complete SSE frame | Required integer in the active runtime byte-limit range |

The constructed Client is immutable and safe for concurrent invocations. The
application still owns the original HTTP client; the SDK does not close or
mutate it.

## Application Invocation Request

| Field | Meaning | Validation |
| --- | --- | --- |
| Agent ID | Installed logical Agent target | Required platform-safe identifier |
| Capability | Requested installed capability | Required platform-safe identifier |
| Input | Agent business input | Required duplicate-free JSON object encoded as `json.RawMessage`; null, scalar, array, and malformed JSON are invalid |

The request contains exactly these three public fields. Result mode is selected
by the method. Workspace, application credential, endpoint, version, Release,
Card digest, Router, correlation, and Agent credential are configuration or
platform-owned facts and cannot be supplied here.

## Gateway Invocation Wire Request

This value is private to the SDK and maps the public request to Northbound
Invocation v4.

| Field | Source |
| --- | --- |
| `agentId` | Application Invocation Request |
| `capability` | Application Invocation Request |
| `input` | Application Invocation Request |
| `stream` | `false` for `Invoke`; `true` for `InvokeStream` |

The full encoded object must fit the configured request limit before transport.

## Application Invocation Result

| Field | Meaning |
| --- | --- |
| Invocation ID | Gateway-created root Invocation identity |
| Root Task ID | Gateway-created root Task identity |
| Trace ID | Gateway-created Trace, validated against the response header |
| Output | Raw Agent result JSON value |

The wire-only Schema version and constant `succeeded` status are validated but
not duplicated in the public application result.

## Application Result Stream

The Stream exclusively owns one successful SSE response body. It exposes
validated Result Stream Event v2 values and tracks the following private state:

| State | Meaning | Allowed next action |
| --- | --- | --- |
| Open | HTTP 200 and Trace header validated; no event accepted | `Recv` must obtain accepted sequence 0; `Close` records interruption |
| Active | Accepted read; zero or more chunks processed | `Recv` accepts next chunk or one terminal; `Close` records interruption |
| Terminal observed | Exactly one terminal event returned | Next `Recv` must observe EOF; another frame is invalid; `Close` records interruption |
| Finished | Terminal followed by EOF and body closed | `Recv` returns `io.EOF`; `Close` returns the recorded clean result |
| Failed | Framing, decoding, validation, context, transport, or early EOF failure; body closed | `Recv`/`Close` return the recorded failure according to method contract |
| Closed early | Caller closed before Finished; body closed | Repeated `Close` returns the same interruption; `Recv` reports closed |

All events in one stream have identical invocation, root Task, and Trace values.
The first event Trace must equal the already validated response Trace header.
Only one goroutine may consume or close a given Stream at a time; the Client
itself remains safe for concurrent independent calls.

## Client Platform Error

| Field | Meaning | Constraint |
| --- | --- | --- |
| HTTP status | Gateway response status | Must match the active v4 code/phase matrix |
| Code | Stable Platform Error v4 code | Fixed message validated before mapping |
| Trace ID | Gateway request Trace | Required and equal to the one response header |
| Invocation ID | Accepted Invocation identity | Absent with Root Task ID before correlation; otherwise valid |
| Root Task ID | Accepted root Task identity | Absent with Invocation ID before correlation; otherwise valid |

Raw body, fixed message, unknown members, endpoints, credentials, dependency
detail, Agent input, and Agent output are not retained. `Correlated()` is true
only when the invocation/root Task pair is present.

## Correlation relationship

```text
Client Configuration.Workspace ID
  -> Gateway path authorization scope

Gateway-created Trace
  -> Router dispatch Trace
  -> Router response Trace header
  -> Gateway northbound Trace header
  -> JSON result/error Trace or every SSE event Trace
```

Any missing, duplicate, or unequal link is an explicit contract failure. No
layer substitutes a newly generated or downstream-selected Trace.
