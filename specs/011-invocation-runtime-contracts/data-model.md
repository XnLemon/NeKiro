# Data Model: Invocation Runtime Contracts

## Trust Sources

| Field | Root source | Child source |
| --- | --- | --- |
| `invocation_id` | Gateway-generated, trusted service request | Router-generated after parent/auth validation |
| `root_task_id` | Gateway-generated | Exact committed parent value |
| `parent_invocation_id` | absent | Only request reference; validated against committed parent |
| `trace_id` | Gateway-generated | Exact committed parent value |
| `caller` | Gateway-authenticated user/service | Router-authenticated Agent binding |
| `workspace_id` | Authenticated path + Workspace policy | Exact committed parent value |
| target/capability/input/mode | caller request after strict validation | Agent request after strict validation |
| `agent_card_version` | Dispatch authorization/exact pin | Router exact re-resolution; never SDK-supplied |

## Agent Caller Binding

One required deployment secret binds one opaque Bearer credential to one exact Agent ID. The secret is input to authentication only. Contract DTOs, Card, Ledger, error, and logs contain no token, fingerprint, locator, or credential metadata.

## Lifecycle

```text
unaccepted --created commit--> pending --> routing --> running --> succeeded
                                  |          |          |--> failed
                                  |          |          |--> canceled
                                  |          |          `--> timed_out
                                  |          |--> failed/canceled/timed_out
                                  `--> canceled/timed_out
```

- Unsupported authentication is `routing -> failed` with `AGENT_AUTH_UNSUPPORTED`.
- Oversize detected after acceptance terminalizes from the current state when the terminal append commits.
- Post-side-effect Ledger loss does not add a state. The durable projection stays at the last committed non-terminal status and the live response is non-success.
- A terminal event is accepted only when its append/projection transaction commits first.

## Required Runtime Limit Inputs

| Setting | Rule |
| --- | --- |
| invocation deadline ms | Required base-10 integer, `1..600000`, no whitespace normalization/default |
| public request bytes | Required base-10 integer, `1..2147483647` |
| internal request bytes | Required base-10 integer, `1..2147483647` |
| Agent response bytes | Required base-10 integer, `1..2147483647` |
| A2A event bytes | Required base-10 integer, `1..2147483647` |
| SSE event bytes | Required base-10 integer, `1..2147483647` |

An empty, signed, spaced, fractional, exponent, overflow, or out-of-range value is invalid. No value is inferred. Effective Card-bound values use `min(configured, card)`.

## SSE Frame

```text
data:<compact JSON value>\n
\n
```

The encoded event is one UTF-8 line; JSON escaping prevents literal CR/LF. The byte limit applies to the JSON data value before the framing prefix/delimiters. Every complete event flushes.

## Persistent Content Exclusion

Ledger facts may contain only stable correlation/routing metadata, sequence/timestamp, status, byte counts, latency, and fixed safe error code/message. They never contain input, output, chunk values, endpoint, credentials, raw dependency data, or Runtime telemetry.
