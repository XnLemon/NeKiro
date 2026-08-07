# Instance Registry Directory v1

- Status: Active for issue #89 first delivery slice
- Go package: `github.com/NeKiro-project/NeKiro/registry`
- Kubernetes provider: `github.com/NeKiro-project/NeKiro/registry/kubernetes`
- Architecture decision: [ADR 0010](../decisions/0010-instance-registry-directory.md)

## Ownership

This contract describes ephemeral Agent-instance topology for an already exact
Catalog-authorized target. It does not create, resolve, alter, persist, or
authorize Agent Cards, Releases, Endpoint Bindings, Workspace facts,
permissions, invocation facts, or Ledger facts.

`registry` is intentionally distinct from the Control Plane Catalog Registry.

## Root API

```go
type InstanceDirectory interface {
    Snapshot(context.Context, ReleaseTarget) (InstanceSnapshot, error)
    Observe(context.Context, ReleaseTarget) (InstanceObservation, error)
    Capabilities() Capabilities
    Close() error
}

type InstanceWatch interface {
    Next(context.Context) (InstanceChange, error)
    Close() error
}
```

All returned values are immutable. Accessors for slices, maps, and nested
values return copies. A watch has exactly one active `Next` caller; a
concurrent call is typed `invalid`.

`Next` has no callback and does not select an instance. Cancelling its context
returns typed `canceled` for that call only and consumes no queued change.
`Close` is idempotent and unblocks pending calls with the latched terminal
cause, or `closed` when no source terminal cause has won.

## Exact Target

ReleaseTarget v1 consists of the following byte-exact strings:

| Field | Meaning |
| --- | --- |
| Agent ID | Catalog Agent identity |
| Agent Card version | exact Card version |
| Release ID | exact immutable Release identity |
| Card digest | lower-case 64-hex Agent Card digest |
| Canonical endpoint | canonical exact A2A endpoint |
| Audience | canonical Router credential audience origin derived from the exact endpoint |

The directory validates this value at construction/use and must preserve every
field byte-for-byte in snapshots and changes. It must not resolve another
Release, endpoint, or audience.

`Audience` must equal the canonical HTTP(S) origin derived from the exact
`Canonical endpoint`, using the same Router credential audience rule. A target
with an independently valid but different audience is invalid; the directory
does not repair or substitute it.

## States And Outcomes

| Situation | Public result |
| --- | --- |
| No configured target binding | typed `missing` error, no snapshot/watch |
| Configured binding; Service absent | immutable `missing` snapshot state; Observe remains live |
| Valid Service; no selected EndpointSlices | immutable `empty` snapshot state |
| Valid Service and endpoint topology | immutable `populated` snapshot state |
| Bad target/binding/provider data | typed `invalid` error |
| Kubernetes 401/403 | typed `unauthorized` error |
| Pre-open network failure or 429/5xx | typed `unavailable` error |
| Expired resourceVersion/410 | typed `stale` error |
| Stream EOF/non-410 watch `ERROR`/overflow | typed `watch_interrupted` error |
| Caller context cancellation | typed `canceled` error for that call |
| Explicit directory/watch closure | typed `closed` error |

`draining` is an instance lifecycle state. `target_deleted` is one terminal
change carrying a missing snapshot, followed by `closed`; neither is an error
alias.

## Change Kinds

`instances_changed` carries a non-empty upsert and/or deletion delta together
with the resulting complete snapshot. `state_changed` makes a missing-to-empty
or empty-to-missing transition observable when the instance set remains empty.
It carries an explicit `PreviousState`, has no upserts or deletions, and may
never use `populated` as either state. A populated-to-empty transition carries
deletions and an empty-to-populated transition carries upserts through
`instances_changed`.

`target_deleted` remains the terminal Service-owner deletion event. It carries
the final missing snapshot and is delivered once before `closed`. A source
event that leaves both snapshot state and complete instance topology unchanged
produces no public change.

## Kubernetes Binding v1

A Binding v1 includes one exact ReleaseTarget, canonical HTTP(S) API origin
without a path/query/fragment, namespace, Service name/UID, expected
EndpointSlice managed-by label, required Service owner labels, required
EndpointSlice labels, one `IPv4` or `IPv6` address type, one non-empty TCP port
name, and explicit positive byte/count/queue bounds.

The required reserved labels are present on the Service and every selected
EndpointSlice:

```text
registry.nekiro.dev/binding-version = v1
registry.nekiro.dev/release-target  = <target-key>
```

`<target-key>` is lower-case unpadded base32 of SHA-256 over this ordered record:

```text
u32be(len(agent_id))              || agent_id UTF-8 bytes
u32be(len(agent_card_version))    || agent_card_version UTF-8 bytes
u32be(len(release_id))            || release_id UTF-8 bytes
u32be(len(card_digest))           || card_digest UTF-8 bytes
u32be(len(canonical_endpoint))    || canonical_endpoint UTF-8 bytes
u32be(len(audience))              || audience UTF-8 bytes
```

For `agent-a`, `1.0.0`, `release-a`, 64 lower-case `a` digest characters,
`https://agent.example/a2a`, and `https://agent.example`, the target-key is:

```text
f7nt6cxpnngsq3jmjii4h5fwo4gixmr6bk5jk2ptoh7knjxc7nmq
```

Raw Release IDs, legacy marker formats, aliases, default namespaces/ports, and
alternate target serializations are invalid.

## Kubernetes Request Executor v1

The provider accepts an injected executor contract, not `*http.Client` or
`http.RoundTripper`. Its declared v1 guarantees are exactly one network attempt
per request, no redirect, environment proxy discovery, response cache,
implicit limiter, retry, authority switch, or hidden re-authentication.

The directory calls it exactly once for each Service List, EndpointSlice List,
Service watch-open, and EndpointSlice watch-open. Authentication and TLS remain
outside the directory and are not included in Binding v1 or logs.

## Kubernetes Observation

`Snapshot` performs one Service List and one EndpointSlice List. `Observe`
performs those lists, then one watch-open per resource using each List's opaque
resourceVersion. It returns only after both watch-open responses have HTTP 200.
The source resourceVersions are stored as a pair; all public event ordering is
a local serialized order and never claims cross-resource timestamp ordering.

No List continuation, `resourceVersion=0`, bookmarks, `sendInitialEvents`,
relist, retry, reopen, resubscribe, provider switch, endpoint switch, cache,
or stale topology result is permitted.

## Endpoint Normalization

Endpoint identity is `targetRef.uid`. Identical duplicate network tuples for
the same UID fold; distinct tuples union. A state/zone/metadata conflict for
one UID, or a tuple claimed by two UIDs, is invalid.

Only one configured address type and TCP port name are supported. All matching
addresses are canonical IPs and remain in sorted output. FQDN, mixed/malformed
addresses, missing/duplicate/invalid matching ports, and non-TCP protocol are
invalid. The derived state is:

| terminating | serving | ready | state |
| --- | --- | --- | --- |
| true | true | any | draining |
| true | false | any | unavailable |
| false | true | true | ready |
| false | false | any | unavailable |
| false | true | false | unavailable |

Kubernetes nil semantics are source normalization: ready/serving nil are true;
terminating nil is false. The raw resolved booleans remain observable.

## Non-Goals

Nacos, registration/leases, federation, instance selection, load balancing,
retry/failover, invocation pinning, Router/Gateway consumption, traffic policy,
and Kubernetes binding publication/reconciliation are outside v1.

## Compatibility

This is a Go package v1 contract. Additive optional capabilities may be added
without changing existing behavior. Renaming a field, changing a typed outcome,
changing Binding v1 serialization, or altering watch ordering/terminal behavior
is a breaking change and requires a new contract version plus migration policy.
