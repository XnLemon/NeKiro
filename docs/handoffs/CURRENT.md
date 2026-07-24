# Current Handoff: Router-to-Agent Authentication

**Updated**: 2026-07-24 (Asia/Shanghai)

**State**: Spec 024 / Issue #50 is implemented and independently reviewed on
`codex/router-agent-auth`. PR #55 CI run `30060752722` is green; merge remains
before this handoff is considered merged upstream.

## Repository State

- Upstream: `https://github.com/NeKiro-project/NeKiro.git`
- Fork: `https://github.com/XnLemon/NeKiro.git`
- Current branch: `codex/router-agent-auth`
- Active authentication artifacts: `specs/024-router-agent-authentication/`
- Backend acceptance artifacts: `specs/021-invoke-record-acceptance/`
- Parent invocation artifacts: `specs/010-invocation-routing-ledger/`
- Required Git identity: `Nene7ko_ <1604009816@qq.com>`

## Delivered Scope

- Catalog, Discovery, Workspace, Installation, and exact-version authorization.
- Gateway v4 Invocation Dispatch through Router Internal dispatch v4; Router
  Internal v3 remains the metadata-read contract.
- Independent A2A Router with JSON/SSE delivery and strict endpoint resolution.
- Router-owned append-only metadata-only Invocation Ledger and scoped reads.
- Runtime B direct A2A sample.
- Thin Agent SDK and authenticated Router-mediated nested invocation.
- Isolated Runtime A caller using `trpc-agent-go` only inside its own module.
- Runtime A -> Router -> Runtime B parent-child lineage.
- Compose/PostgreSQL deployment and real Invoke-to-Record E2E acceptance.
- Router Invocation Credential v1 with Ed25519 signing, exact claim/header
  binding, strict 401/403 responses, and Agent-local one-time `jti` replay
  rejection.
- Authenticated JSON, SSE, nested, failure, and cancel transport paths; direct
  Agent execution is rejected before Runtime logic.

## Verification

Local unit, contract, Runtime sample, vet, Compose-config, focused secrecy, and
E2E compile gates passed. PR #55 CI run `30060752722` also passed root
build/test/race/vet/lint, Runtime A test/vet/race, PostgreSQL integration,
Compose configuration, Frontend, Codecov, and the real authenticated
Invoke-to-Record acceptance.

## Remaining Scope

- `apps/console` is not present; frontend work remains intentionally paused.
- `sdks/client-sdk` and production identity/governance are not implemented.
- Spec 010 T020/T021 remain `Needs policy` for task retention/capacity,
  timeout ownership, graceful shutdown, and in-flight SSE/Ledger semantics.
- Do not add retry, cache, stale-card compatibility, silent task eviction, or
  degraded success without a new approved Spec/ADR.

Before changing public behavior, read `AGENTS.md`, the active child Spec
artifacts, the language-neutral contracts, and the relevant ADRs. Contract,
data-ownership, trace, or failure-policy changes must return to SDD before code
changes.
