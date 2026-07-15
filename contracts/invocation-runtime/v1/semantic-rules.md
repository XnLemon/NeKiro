# Invocation Runtime Semantic Rules v1

| Rule | Requirement |
| --- | --- |
| `IRT-ERR-001` | Pre-correlation error has exactly code/message/Trace; correlated error additionally requires exact Invocation/root Task. |
| `IRT-ERR-002` | Every Platform Error v4 code has one fixed public message. |
| `IRT-NEST-001` | Child parent/root/Trace/Workspace and caller Agent match the trusted running parent; child ID differs. |
| `IRT-LIFE-001` | Lifecycle begins `created/pending` at sequence zero and preserves immutable context. |
| `IRT-LIFE-002` | Only declared pending/routing/running transitions are legal; success is running-only. |
| `IRT-LIFE-003` | Event and chunk indexes are gapless and no event follows the first terminal. |
| `IRT-STREAM-001` | Result Stream v2 begins with accepted, preserves outer/nested error correlation, has gapless event/chunk indexes, and rejects every event after first terminal. |
| `IRT-STREAM-002` | Result Stream v2 EOF/Finish succeeds only after one valid terminal; accepted-only and accepted-plus-chunk exhaustion are interrupted failures. |
| `IRT-READ-001` | Invocation detail projection and events share exact Workspace/Invocation/Trace/context and last status; Trace lineage is non-empty, Workspace/Trace/root-Task-stable, unique, parent-before-child, self-parent-free, and cycle-free. |
| `IRT-MEDIA-001` | Non-stream accepts exactly `application/json`, `application/*`, or `*/*`; stream accepts exactly `text/event-stream`. |

The conformance corpus is authoritative executable evidence for these rules.
