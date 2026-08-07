# ADR 0010: Provider-Neutral Instance Registry Directory

- Status: Accepted for issue #89 first delivery slice
- Date: 2026-08-07
- Decision owner: Platform Architecture
- Related issue: [#89](https://github.com/NeKiro-project/NeKiro/issues/89)

## Context

The Catalog Registry owns durable Agent Cards, Releases, and trusted Endpoint
Bindings. A deployment with more than one Agent replica also needs ephemeral
topology: current addresses, Kubernetes readiness/termination state, and
provider failures. Treating this information as another Catalog record would
give an ephemeral source authority over immutable authorization and release
facts.

Kubernetes Service and EndpointSlice changes are separate Kubernetes resources.
They do not share a resourceVersion or a single watch stream. Kubernetes may
temporarily duplicate EndpointSlice endpoints while it reshards slices. A
generic HTTP client can also carry redirect, proxy, cache, retry, or credential
behavior that the topology package cannot audit.

## Decision

1. Add the root Go package `registry/` with package name `registry`. It is the
   **Instance Registry**, not the Catalog Registry. It owns no durable Catalog,
   Workspace, permission, invocation, or Ledger fact.
2. Its v1 provider-neutral boundary is an immutable exact `ReleaseTarget`,
   `InstanceDirectory.Snapshot`, `InstanceDirectory.Observe`, a pull-only
   single-consumer `InstanceWatch`, typed outcomes, and separately reported
   optional capabilities. The target audience must equal the canonical Router
   credential audience derived from its exact endpoint.
3. The first provider is `registry/kubernetes`, read/watch only. It has no
   registration, lease, heartbeat, persistence, selection, load-balancing,
   route policy, failover, or Router/Gateway invocation behavior.
4. A Kubernetes Binding v1 is explicit immutable configuration supplied to the
   provider. It carries the exact target, API origin, namespace, Service UID,
   owner/slice markers, address/port interpretation, and positive bounds. It
   is not a Catalog binding store and cannot resolve a Release.
5. Kubernetes observation uses exactly one Service List plus one EndpointSlice
   List, followed by exactly one watch for each source from that source's list
   resourceVersion. The directory exposes a local serial order and retains the
   two opaque source tokens as a pair; it never compares them.
6. The provider accepts only the documented `KubernetesRequestExecutor v1`
   boundary. It creates no default HTTP client, credentials, TLS policy,
   redirect behavior, proxy discovery, cache, limiter, retry, relist, reopen,
   or resubscription.
7. A target without configured Binding v1 returns a typed `missing` error. A
   configured binding whose Service currently does not exist returns an
   immutable missing snapshot and remains observable. A valid Service with no
   matching EndpointSlices returns an immutable empty snapshot.
8. Endpoint identity is `targetRef.uid`. Duplicate, identical network tuples
   for the same UID fold during normal resharding; conflicts never use
   first-wins precedence and are typed invalid provider data.
9. Nacos, provider federation, registration/lease behavior, selection,
   invocation affinity, Router consumption, and external Gateway consumption
   need their own Specs/ADRs. They are not emulated in v1.
10. A provider must make a missing-to-empty or empty-to-missing snapshot-state
    transition observable with an explicit `state_changed` event carrying the
    prior state and no instance delta. It is limited to empty instance sets;
    populated transitions retain their concrete upserts/deletions through
    `instances_changed`, and no-op source events remain silent.
11. After a Kubernetes watch has opened successfully, only a status event with
    code 410 is `stale`. Every other valid status event is the terminal typed
    `watch_interrupted(watch_status_error)` outcome; pre-open HTTP status
    classification is not reused for a live stream.

## Consequences

- Consumers receive topology only for an already exact authorized target.
  Directory output cannot substitute a Release, endpoint, audience, or Card
  digest.
- Observers must explicitly decide whether and when to create a new
  observation after stale, interruption, authorization, overflow, or close.
  The directory does not recover silently.
- K8s operator/Stack work owns binding publication, RBAC, executor
  authentication/TLS, and concrete positive bound values. The Core package
  validates these inputs but does not create or repair them.
- Core tests use a deterministic executor fixture. Cross-repository Samples
  and Stack tests own traffic selection, stream affinity, drain grace, and
  provider deployment evidence.

## Rejected Alternatives

### Make Instance Registry a Catalog Registry submodule

Rejected because Catalog stores durable authorization facts and Instance
Registry reports ephemeral provider state. Combining them would blur data
ownership and risk topology mutating trusted release semantics.

### Use one Kubernetes list/watch token for Service and EndpointSlices

Rejected because Kubernetes resourceVersions are resource-specific. A
synthetic global ordering would falsely claim guarantees Kubernetes does not
provide.

### Use a default `http.Client` with ordinary retry behavior

Rejected because redirect, proxy, cache, retry, and credential policy would be
implicit and cannot prove the issue's one-attempt/no-fallback requirement.

### Treat duplicate endpoint UIDs as invalid without aggregation

Rejected because EndpointSlice resharding can legitimately duplicate endpoints
across slices. Identical records must fold, but conflicting records still fail
explicitly.

## Fallback Delta

Fallback delta: removed 0, retained 1 documented Kubernetes condition
normalization, added 0, net +0.

The retained normalization is Kubernetes `discovery/v1` semantics: nil ready
and serving mean true; nil terminating means false. It is source interpretation
specified by Kubernetes, not a recovery or default data source.

Added fallback evidence: none.
