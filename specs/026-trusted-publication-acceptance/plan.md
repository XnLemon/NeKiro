# Implementation Plan: Trusted Publication Acceptance and Operations

**Branch**: `codex/trusted-publication-acceptance` | **Date**: 2026-07-24 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/026-trusted-publication-acceptance/spec.md`

## Summary

Extend the existing clean Compose/PostgreSQL Invoke-to-Record acceptance into
the final trusted-publication acceptance owned by Issue #52. The same isolated
run will prove the positive cross-runtime lifecycle, real endpoint challenge
failure and recovery categories, Release/Installation gates, direct
Router-credential rejection at the Agent boundary, accepted endpoint failure
recording, Release provenance linkage, and secret absence. Add one
provider/operator runbook that describes the existing automatic facts and the
manual recovery owned by each actor. Correct the existing Router cancellation
race exposed by that acceptance so ADR 0006's terminal semantics hold during
chunk/terminal commit. No public contract, database schema, or runtime
dependency changes.

## Technical Context

**Language/Version**: Go 1.26.0 and Markdown

**Primary Dependencies**: Go standard `net/http`, `os/exec`, `crypto/ed25519`,
`encoding/json`, and existing NeKiro contract validators; Docker Compose;
existing PostgreSQL/pgx acceptance inspection

**Storage**: Existing Catalog, Workspace, and Router Ledger PostgreSQL schemas;
no new table, column, or owner

**Testing**: Focused Router cancellation regression tests; build-tagged Go E2E
against one fresh Compose stack; existing Go unit/contract/integration/race/
vet/lint gates; and Markdown/runbook review

**Target Platform**: Linux CI and developer machines with Docker Compose; Go
test remains portable to Windows while container-internal checks execute in the
Linux sample images

**Project Type**: Cross-service acceptance suite plus operator/provider runbook

**Performance Goals**: One isolated acceptance job within the existing
35-minute CI budget; one real bounded challenge-expiry wait; no retry or
background polling beyond existing bounded Ledger finalization observation

**Constraints**: Reuse active contracts and routes; no Agent port exposure; no
direct database state mutation to manufacture domain transitions; no automatic
retry, fallback endpoint, in-place Release recovery, secret logging, or legacy
runtime compatibility path; terminal persistence is one bounded attempt and
never retries the interrupted stream-chunk append

**Scale/Scope**: One fresh stack, two cross-runtime positive Agents, focused
publication/lifecycle fixtures, three signed-credential rejection cases, one
runbook, and one CI job

## Constitution Check

### Pre-design gate

- **Phase 1 loop**: PASS. This is the final executable proof of the trusted
  Register -> Discover -> Install -> Invoke -> Record loop.
- **Ownership**: PASS. The test calls Gateway/Router/Agent interfaces and reads
  owned persistence only for secrecy evidence. It does not write module tables.
- **Runtime independence**: PASS. Runtime A and Runtime B remain independently
  implemented and communicate only through A2A/Router contracts.
- **Contracts**: PASS. Trusted Publication v1, Installation v2, Northbound and
  Router v4, Agent credential v1, Event 0.3, Result v1/v2, and Platform Error v4
  remain the facts. No version change is required.
- **Invocation lineage**: PASS. Accepted root/child calls and accepted endpoint
  failure are checked in Ledger; pre-acceptance and direct-Agent failures are
  asserted absent from accepted semantics.
- **Failure safety**: PASS. Each negative case asserts a distinct existing code
  and scans every post-issuance public/persisted/log surface for credentials
  and proofs. The one-time authenticated challenge response remains the
  contract-defined delivery boundary for its proof.
- **SDD traceability**: PASS. The acceptance matrix maps every FR and story to
  an executable case or runbook section; tests follow approved harness changes.
- **Cross-runtime proof**: PASS. Runtime A -> Router -> Runtime B remains the
  nested acceptance path with one Trace and exact Release provenance.

### Post-design gate

PASS with no exception or complexity waiver. Research resolves the harness,
real-time expiry, internal Agent access, provenance query, recovery ownership,
and fallback decisions. No implementation-affecting clarification remains.

## Design

### One authoritative acceptance run

Extend `tests/e2e/invoke-record/invoke_record_test.go` rather than create a
second stack or competing positive flow. The run requires an explicit isolated
Compose project name, begins with empty named volumes, registers all fixture
Cards through Gateway, creates and completes
real well-known challenges through sample containers, publishes exact Releases,
and installs them through Workspace APIs. Existing JSON, SSE, nested,
cancellation, timeout, concurrency, restart, and metadata-only evidence remains
part of the same regression.

The CI job keeps `up --build --detach --wait` and unconditional logs/volume
cleanup. Its challenge TTL and verification timeout become explicit short
acceptance values that remain valid under the existing configuration contract.
The expiry case waits until the server-issued `expiresAt` boundary; it does not
rewrite Catalog data or mock the service clock.

### Cancellation commit race

The acceptance keeps timeout and caller cancellation on separate streaming
fixtures so a 50 ms timeout does not masquerade as a cancellation result. A
focused Router regression covers cancellation arriving (a) after terminal
context selection but during terminal append and (b) during a stream-chunk
append.

Terminal Ledger transactions always receive the existing one-second bounded
context derived with `context.WithoutCancel`; this is one commit attempt, not a
retry. Non-terminal stream chunks retain the caller context. A stream event is
schema-validated without advancing delivery sequence, then its metadata is
committed. Only a successful chunk commit advances the SSE sequence. When that
commit loses to cancellation/deadline, the uncommitted chunk is not retried and
the Router commits the local terminal winner at the same next sequence. A real
Ledger dependency failure remains the explicit delivery-only
`DEPENDENCY_ERROR` with non-terminal history unchanged.

### Publication and lifecycle matrix

Small E2E helpers expose existing user operations: register Card, create
Binding, create/complete Challenge, create/read/transition Release, install,
and change Installation status. They retain full response validation and secret
scanning.

The matrix proves:

- wrong proof -> safe failed Binding -> reused challenge rejection -> fresh
  challenge succeeds;
- expired challenge via the real configured TTL;
- disallowed private destination and reachable-name/unavailable-port outcomes;
- pending/unpublished Release installation denial;
- published -> suspended -> revoked exact invocation errors;
- enabled -> disabled -> enabled Installation exact invocation behavior;
- an accepted `/unavailable` Agent route creates a correlated terminal Ledger
  failure with the published Release provenance.

### Router credential boundary matrix

The E2E process creates compact Ed25519 credentials using the explicit
test-only key configuration already supplied to the Router. It sends requests
from inside the `runtime-b` container network without exposing an Agent port.
All signed context headers repeat the credential claims exactly.

- a credential signed by another key returns `401 UNAUTHENTICATED`;
- an expired credential signed by the configured key returns
  `401 UNAUTHENTICATED`;
- a credential with the Runtime A audience presented to Runtime B returns
  `403 FORBIDDEN`;
- no credential returns `401 UNAUTHENTICATED`.

No invalid credential is logged or stored, and no negative helper retries.

### Provenance and trust query

`registerAndPublish` returns the immutable published Release. Invocation detail
validation requires the projection and every event to repeat its Release ID and
Card digest. Trace validation does the same for root and child. The test then
uses each exact Release ID through the existing authorized Release read and
checks provider, Agent, Card version, digest, published state, Binding ID,
verification evidence digest, and `http_well_known` method.

Verification method stays Catalog-owned and is not copied into Ledger. The
query chain is Ledger Invocation -> Release ID -> Catalog Release.

### Runbook and recovery boundary

`docs/runbooks/trusted-publication-operations.md` documents setup and query
surfaces, automatic state recording, provider/Workspace owner/platform operator
ownership, state/error decision tables, suspension/revocation commands, and
recovery completion checks.

Recovery preserves immutable history:

- failed/expired/used challenge: correct endpoint material and create a fresh
  challenge on the eligible Binding;
- disabled Installation: Workspace owner explicitly re-enables it;
- suspended/revoked Release: provider registers a new Card version and completes
  a new Binding/Release lifecycle;
- endpoint unavailable: provider restores or versions the endpoint, then a
  caller issues a fresh invocation;
- signing/verifier mismatch: platform operator corrects explicit deployment
  configuration and validates with a new request.

There is no retry, alternate endpoint, implicit unsuspension, or automatic
republish. Undecided retention, approval, rotation, and cross-replica replay
rules are labelled `Needs policy`.

## Fallback Inventory

| Existing or proposed behavior | Classification | Decision | Evidence |
| --- | --- | --- | --- |
| Bounded Ledger finalization observation after caller cancellation | Existing acceptance observation, not business recovery | Keep unchanged | ADR 0006 cancellation is asynchronous relative to client disconnect; existing E2E requires a bounded query |
| Retry verification/invocation, alternate endpoint, automatic Release recovery, old contract read, or database-written fixture state | Unsupported fallback | Do not add | FR-011, FR-016, constitution VII, ADR 0004/0006 |
| Treat disabled Workspace wording as a new Workspace status | Unsupported domain fallback | Do not add | Spec 023 R-007 and active Installation v2 define the disabled Installation state |
| Omit unavailable, expired, or credential cases when local timing/network differs | Error-swallowing test fallback | Do not add | FR-006 through FR-010 require exact deterministic outcomes |

Fallback target: removed 0, retained 1, added 0, net 0.
Added fallback evidence: none.

### Retained fallback evidence

- **Evidence**: ADR 0006 defines cancellation propagation after a client
  disconnect and first committed terminal semantics; the existing E2E already
  observes the durable result with a five-second bound.
- **Trigger**: Only the explicit cancellation acceptance case after the caller
  closes the response body.
- **Semantics**: It reads the authoritative Ledger until the required terminal
  fact is visible; it never changes a result or repeats an invocation.
- **Boundary**: The acceptance harness owns observation; Router owns the state
  transition.
- **Visibility**: Timeout fails the test with the Invocation ID and desired
  state.
- **Tests**: Existing canceled SSE acceptance and Ledger validation.

## Project Structure

### Documentation (this feature)

```text
specs/026-trusted-publication-acceptance/
|-- spec.md
|-- clarify.md
|-- plan.md
|-- research.md
|-- data-model.md
|-- quickstart.md
|-- contracts/
|   `-- acceptance-matrix.md
|-- checklists/
|   `-- requirements.md
`-- tasks.md
```

### Source Code (repository root)

```text
tests/e2e/invoke-record/
`-- invoke_record_test.go

apps/a2a-router/internal/api/
|-- dispatch_handler.go
`-- dispatch_handler_test.go

docs/runbooks/
|-- local-development.md
`-- trusted-publication-operations.md

.github/workflows/
`-- ci.yml

specs/023-trusted-agent-publication/
`-- tasks.md

AGENTS.md
docs/handoffs/CURRENT.md
README.md
```

**Structure Decision**: Extend the existing authoritative backend acceptance
harness and CI stack. Add only the missing operations runbook and Spec 026
artifacts; do not add a second service, test stack, contract package, or Agent
fixture binary.

## Complexity Tracking

No constitution violation requires justification.

## Verification Commands

```text
go test ./contracts/... ./sdks/agent-sdk/... ./agents/runtime-b/...
go test -tags=e2e -count=1 ./tests/e2e/invoke-record
go test -race ./...
go vet ./...
go test ./...
golangci-lint run
docker compose --file deploy/compose.yaml config --quiet
git diff --check
```
