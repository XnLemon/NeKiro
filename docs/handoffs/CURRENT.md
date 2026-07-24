# Current Handoff: Workspace Client SDK

**Updated**: 2026-07-24 (Asia/Shanghai)

**State**: Spec 025 / Issue #51 is implemented, fully verified, independently
reviewed, and converged on `codex/workspace-client-sdk`. Commit, PR, CI, and
merge remain in progress.

## Repository State

- Upstream: `https://github.com/NeKiro-project/NeKiro.git`
- Fork: `https://github.com/XnLemon/NeKiro.git`
- Current branch: `codex/workspace-client-sdk`
- Active artifacts: `specs/025-workspace-client-sdk/`
- Parent trusted-publication artifacts: `specs/023-trusted-agent-publication/`
- Required Git identity: `Nene7ko_ <1604009816@qq.com>`

## Delivered Scope

- Standalone `sdks/client-sdk` package, separate from the Agent SDK and with no
  service-internal dependency.
- One immutable Client binds an explicit HTTP client, canonical Gateway origin,
  Workspace ID, opaque Owner-mapped application credential, and explicit
  request/response/SSE limits.
- Per-call input is exactly Agent ID, capability, and duplicate-free JSON
  object; endpoint, Router, version, Release, digest, Workspace, correlation,
  and Agent credentials are not accepted.
- One-request non-streaming Invocation Result v1 delivery with exact media,
  body, Trace, correlation, and size validation.
- Incremental Result Stream Event v2 delivery with strict compact SSE framing,
  accepted/chunk/terminal ordering, contiguous indices, context cancellation,
  and terminal-followed-by-real-EOF completion.
- Complete typed Platform Error v4 status/code/phase matrix. Public errors retain
  only status, code, Trace, and optional Invocation/root Task correlation.
- Northbound Invocation v4 and Router Internal v4 now declare required Trace
  headers and HTTP 500 `INTERNAL_ERROR`; Router no longer defaults that code to
  503.
- Control Plane requires one Router response Trace equal to its dispatch Trace,
  and Gateway keeps the Trace it created instead of selecting a downstream
  replacement.
- SDK README, compiled application example, project entry point, compatibility
  guidance, and focused configuration/request/stream/error/secrecy tests.

## Verification Completed

- `go test ./sdks/client-sdk/... -run '^Example'`
- `go test ./contracts ./sdks/client-sdk/...`
- `go test ./apps/control-plane/internal/invocation ./apps/control-plane/internal/gateway`
- `go test ./apps/a2a-router/internal/api`
- `go build ./...`, `go test ./...`, and `go vet ./...`
- WSL/Linux `go test -race ./sdks/client-sdk/...`
- Exact CI `golangci-lint v2.12.2 run` and `git diff --check`
- Independent Standards/Spec final Reviews: PASS with zero High/Medium/Low
  findings; `speckit-converge`: zero gaps and no appended tasks.

## Remaining Delivery Gates

- Commit implementation, push, open a ready PR referencing #51 and #47, wait
  for green CI, merge, close #51, and sync `main`.
- Keep parent Issue #47 open for dependent Issue #52, which owns the complete
  clean Compose trusted-publication negative-path acceptance and operator
  recovery presentation.

Frontend Console work remains paused and `apps/console` is not present. Do not
add credential lifecycle, delegated roles, retries, redirects, alternate
destinations, Ledger result polling, v3 invocation compatibility, or direct
Agent access without a new approved Spec/ADR.
