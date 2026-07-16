# Data Model: Deterministic Direct A2A Sample

## Fixture Request

| Field | Type | Rules |
| --- | --- | --- |
| `fixture` | string | Required; one of `success`, `stream-success`, `failure`, `hold`. |
| `value` | JSON value | Required; passed through in deterministic structured output. |

The A2A request must contain one user-role Message with a non-empty message ID
and exactly one structured data part. No missing field receives a default.

## Runtime Task

| Field | Type | Rules |
| --- | --- | --- |
| `id` | A2A Task ID | Deterministically derived from input message ID. |
| `context_id` | string | Derived from the same input identity with a separate domain. |
| `state` | A2A Task state | `working -> completed` or `working -> canceled`; terminal is immutable. |
| `history` | Message list | Contains the accepted user Message; reads return an explicit suffix length. |
| `cancel` | signal | Closed at most once when working cancellation commits. |

### State Transitions

```text
message/stream(stream-success): working -> completed
message/stream(hold):           working -> canceled
tasks/cancel(working):          working -> canceled
tasks/cancel(terminal):         task-not-cancelable
```

## Fixture Result

One successful JSON result is an Agent-role Message containing one DataPart:

```json
{
  "agent": "runtime-b",
  "fixture": "success",
  "value": "<exact request value>"
}
```

The successful stream emits exactly:

```text
Task(working)
-> Message(agent)
-> ArtifactUpdate(base)
-> ArtifactUpdate(append,lastChunk)
-> StatusUpdate(completed,final)
```

The held stream emits `Task(working)` and, after explicit cancellation,
`StatusUpdate(canceled,final)`.
