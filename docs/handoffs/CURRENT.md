# Current Handoff: Spec 002 Review Remediation

**Updated**: 2026-07-14 (Asia/Hong_Kong)
**State**: Spec 002 implementation is present, but delivery is paused before a
fresh Review of the latest unbounded-integer validation fix. It is not complete.

## Repository State

- Repository: <https://github.com/XnLemon/NeKiro.git>
- Branch: `codex/002-catalog-registry-discovery`
- Base last verified: `origin/main` at `3bcb844`
- Latest implementation commit: `7899239`
  (`fix(contracts): validate unbounded card integers lexically`)
- Local Git identity: `Nene7ko_ <1604009816@qq.com>`
- Expected worktree after the handoff commit: clean
- Frontend remains paused.
- No PR has been created. Do not create one until final Review PASS and
  convergence are complete.

The handoff commit hash is intentionally not recorded because this file is part
of that commit. Resolve the repository root on the new machine; do not assume
the Windows path above or any previous machine path exists.

## Cross-Machine Recovery

```powershell
git clone https://github.com/XnLemon/NeKiro.git
Set-Location NeKiro
git fetch origin --prune
git switch --track origin/codex/002-catalog-registry-discovery
git config --local user.name Nene7ko_
git config --local user.email 1604009816@qq.com
git status --short --branch
git log -8 --oneline
```

If the branch already exists locally, use:

```powershell
git switch codex/002-catalog-registry-discovery
git pull --ff-only origin codex/002-catalog-registry-discovery
```

Before modifying anything, read in full:

- `AGENTS.md`
- `.specify/memory/constitution.md`
- this file
- every artifact under `specs/002-catalog-registry-discovery/`
- `docs/decisions/0004-catalog-persistence-and-consistency.md`
- `docs/runbooks/local-development.md`

Use the branch and worktree as authoritative. Do not reconstruct state from an
old chat, a stale clone, or a machine-specific temporary directory.

## Implemented Scope

The runnable Control Plane Catalog currently closes:

```text
Register -> Publish -> Discover -> Disable
```

It includes immutable Agent Card `0.2` registration and exact reads, stable
owner enforcement, publication/disablement, PostgreSQL persistence, explicit
forward-only migrations, transactional publication ordering, repeatable-read
cursor snapshots, published-only Discovery, strict development Bearer auth,
fixed Platform Error v2 responses, readiness, Docker/Compose/CI wiring, and two
Runtime-neutral metadata fixtures. It does not invoke an Agent Runtime.

Out of scope remains Workspace Installation, Invocation Dispatch, A2A Router,
Ledger, SDK/runtime behavior, live sample Agents, Frontend, deployment, hot/cold
storage, Marketplace, and the remainder of the Phase 1 loop.

## Review History

The overall T044 Review gate remains **FAIL** until a new Reviewer evaluates
commit `7899239` and returns `High 0`, `Medium 0`.

1. Round 1 found four Medium issues: body bounds, machine-range Card limits,
   Publication Clock readiness, and concurrency coverage. Fixed.
2. Round 2 found PostgreSQL `jsonb` rejects legal `1e131072`. Fixed by schema v2
   Card text storage with transactionally derived query columns.
3. Round 3 found destructive public `migrate down` and a connection-wide timeout
   that consumed header time. Fixed with forward-only public migration and a
   per-body ResponseController deadline.
4. Round 4 found cross-owner exact duplicates returned 403 before FR-005's 409.
   Fixed with exact-version precedence under the stable identity row lock.
5. Round 5 found the pinned JSON Schema engine rejects legal `1e1000001` through
   its internal `big.Rat` exponent limit. The fix is now committed as `7899239`
   but has not received a fresh independent Review.

Future Review uses a fresh child Agent and a primary-authored boundary prompt.
Do not use OCR/open-code-review. Test success does not replace Review PASS.

## Latest Fix

Tasks T064-T066 are implemented:

- `contracts/validate.go` validates the two active unbounded limit fields using
  JSON-number syntax, mathematical integrality, and minimum `1` without
  materializing the value or exponent.
- Validation substitutes `1` only in an Agent Card copy passed to the pinned
  Schema engine. The decoded and persisted Card remains unchanged.
- Contract tests cover integral decimal/exponent forms, arbitrary exponent
  digits, zero, negative, fractional, huge-negative, and malformed values.
- Real PostgreSQL/HTTP acceptance now proves `1e1000001` registration, text
  persistence, publication, Discovery, restart, and exact read.

The implementation Agent failed after writing the patch because its external
provider returned HTTP 403 for exhausted quota. The primary Agent reviewed the
work, ran verification, and committed it. There is no known missing patch from
that Agent.

## Verification Evidence

After `7899239`, the following passed:

```powershell
go test -count=1 ./contracts
go test -count=1 ./...
go test -tags=integration -count=1 ./tests/integration/catalog
go test -race -count=1 ./contracts ./apps/control-plane/internal/catalog/... ./apps/control-plane/internal/gateway
go vet ./...
go mod tidy -diff
git diff --check
```

The integration run used a disposable PostgreSQL 17 database named
`review_test`; its container was stopped afterward. Full race verification,
final Docker build, and final Compose rendering must be rerun under T067 before
the next Review because the latest fix touched shared contract validation:

```powershell
go test -race -count=1 ./...
```

Go verification may use:

```powershell
$env:GOPROXY = 'https://goproxy.cn,direct'
$env:GOSUMDB = 'sum.golang.google.cn'
```

Use only an explicit disposable database whose name ends in `_test`. Never infer
a DSN or run the integration suite against shared/staging/production data.

Fallback delta remains: removed `0`, retained `3`, added `0`, net `0`.
Added fallback evidence: none. Retained Spec policies are omitted limit `25`,
genuine empty Discovery, and idempotent disablement.

## Exact Next Steps

1. Confirm the remote branch contains `7899239` and this handoff commit.
2. Fetch `origin`; if `origin/HEAD` advanced, rebase the clean branch without
   force and rerun the complete matrix.
3. Complete T067 with default, full race, real PostgreSQL integration, vet,
   temporary-output binary build, pinned Docker build, Compose rendering,
   `go mod tidy -diff`, and `git diff --check`.
4. Create a fresh read-only child Reviewer without OCR. Focus on arbitrary JSON
   exponent syntax/integrality/minimum equivalence, validation-copy isolation,
   preservation of every other Schema rule, `1e1000001` persistence, and all
   prior migration/deadline/duplicate/concurrency boundaries.
5. If Review fails, update Spec/Plan/Data Model/Tasks/contracts before behavior,
   use an implementation Agent with the `implement` skill, then create another
   fresh Reviewer.
6. Only after explicit PASS: mark T044/T067, run `speckit-converge`, complete
   FR-001 through FR-025 and all 19 acceptance mappings, set Spec status to
   complete, and update `AGENTS.md`, `README.md`, and this handoff.
7. Fetch/rebase/retest/review the integrated final state as required, then push
   and create the PR for user review. Stop at the PR; do not begin Workspace
   Installation until the user reviews it.

Do not modify `pnpm-lock.yaml` or relax `minimumReleaseAge`. A cooling-period
failure such as `postcss@8.5.18` is a supply-chain policy result, not a reason to
change project policy.
