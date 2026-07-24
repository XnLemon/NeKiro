# Research: Trusted Publication Acceptance and Operations

## Decision 1: Extend the existing clean acceptance run

**Decision**: Extend `tests/e2e/invoke-record` and its `backend-acceptance` CI
job.

**Rationale**: It already owns the fresh Compose/PostgreSQL lifecycle, positive
trusted publication, nested Runtime A -> Runtime B call, Ledger reads,
concurrency, cancellation, restart, and secret scans. A second suite would
duplicate the fact source and make cleanup/order evidence weaker.

**Alternatives considered**: A separate trusted-publication package or a
unit-only matrix. Both were rejected because Issue #52 requires one real fresh
environment and cross-service boundaries.

## Decision 2: Use real challenge time and service APIs

**Decision**: Configure an explicit short acceptance challenge TTL, wait until
the returned `expiresAt`, and exercise every state transition through Gateway.

**Rationale**: Direct Catalog table writes would violate ownership and could
prove a state the product cannot actually reach. A real clock boundary proves
the deployed process configuration and store behavior together.

**Alternatives considered**: SQL update of `expires_at`, a test-only clock
endpoint, or skipping expiration. All either cross ownership, add product
surface, or leave FR-006 unproved.

## Decision 3: Reach Agent endpoints only inside the Compose network

**Decision**: Use `docker compose exec` in a sample container for direct and
invalid-credential Agent requests.

**Rationale**: Sample Agents intentionally have no host port. Keeping them on
the internal network proves that ordinary consumers cannot use a newly exposed
port and preserves the managed Gateway/Router path.

**Alternatives considered**: Publishing Agent ports or adding a proxy service.
Both would widen deployment surface solely for a test.

## Decision 4: Generate credential fixtures from explicit test configuration

**Decision**: Build strict compact Ed25519 credentials in the E2E process using
the existing test-only issuer, key ID, private key, and actual published Runtime
B Release provenance.

**Rationale**: This reaches the real Runtime verifier with valid context while
changing exactly the signature, expiration, or audience under test. No
credential minting endpoint or production behavior is added.

**Alternatives considered**: Reuse one captured Router credential or add a
Router debug endpoint. Credentials are single-use/short-lived, and a debug
endpoint would violate the product boundary.

## Decision 5: Query trust method through the Release owner

**Decision**: Validate Release ID and Card digest in Ledger, then read the exact
Release through Gateway to obtain `verificationMethod`.

**Rationale**: Ledger owns immutable invocation provenance, while Catalog owns
the verification method. Copying the method into Ledger would create a new
cross-boundary contract and duplicate a Catalog fact.

**Alternatives considered**: Add `verificationMethod` to Invocation Event 0.3
or query Catalog tables from Router. Both violate current ownership and are not
required for queryability.

## Decision 6: Preserve the Router acceptance boundary

**Decision**: Require Ledger facts only after the Router-owned `created`
commit. Pre-authorization, direct-Agent, and invalid-credential failures expose
typed errors/Trace but no accepted Invocation.

**Rationale**: ADR 0006 explicitly defines this boundary. Recording a false
Invocation before acceptance would change Ledger semantics and could imply an
Agent side effect.

**Alternatives considered**: Generate IDs before Workspace authorization or
append rejected-request audit events to Ledger. Both are new product behavior
outside Issue #52 and require a separate contract/ADR.

## Decision 7: Document manual, state-preserving recovery

**Decision**: The runbook maps existing states to owner actions and requires a
fresh explicit request after correction. Suspended/revoked immutable Releases
are replaced by a new version lifecycle.

**Rationale**: Current contracts provide no unsuspend, retry, rollback,
alternate endpoint, or automatic republish policy.

**Alternatives considered**: Automatic retries, in-place restoration, or
runbook SQL. They are unsupported fallbacks and bypass ownership.

## Decision 8: Preserve one terminal commit across caller cancellation races

**Decision**: Keep non-terminal stream writes on the caller context, but give
every terminal Ledger transaction one unconditional bounded context that does
not inherit caller cancellation. Advance SSE sequence only after its matching
stream metadata commit.

**Rationale**: The clean acceptance exposed two ADR 0006 violations: a caller
could cancel between terminal-context selection and append, or during a chunk
append after delivery sequence had already advanced. Both paths left an
accepted Invocation `running` or misreported `DEPENDENCY_ERROR`. A terminal
commit is a distinct fact, not a retry of the interrupted chunk.

**Alternatives considered**: Increase E2E polling time, retry the chunk, treat
caller cancellation as Ledger dependency, or detach all Ledger writes. These
respectively hide the defect, violate the no-retry rule, record the wrong
failure, or let non-terminal work outlive the caller.

## Fallback conclusion

No fallback is added. The existing bounded cancellation Ledger observation is
retained as evidenced acceptance behavior; it neither retries nor changes the
Invocation.
