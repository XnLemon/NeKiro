# Current Handoff: Spec 002 Review Remediation

**Updated**: 2026-07-14 (Asia/Hong_Kong)
**State**: Spec 002 Catalog implementation, verification, independent Review,
and convergence are complete. The next Phase 1 feature is Workspace
Installation.

## Repository State

- Repository: <https://github.com/XnLemon/NeKiro.git>
- Branch: `codex/002-catalog-registry-discovery`
- Base last verified: `origin/main` at `0d24f2b`
- Latest implementation/closure line: `f5e6bbb`
  (`fix(catalog): preserve historical migration digests`)
- Local Git identity: `Nene7ko_ <1604009816@qq.com>`
- Expected worktree after the closure commit: clean
- Frontend remains paused.
- No PR has been created and no push was performed.

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

The overall T044 Review gate is **PASS**: the post-fix closure Reviewer
`019f5ef0-8189-7ef1-b677-3f1ef2bd29bf` returned `High 0`, `Medium 0`, `Low 0`.

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
   its internal `big.Rat` exponent limit. The fix uses bounded-memory lexical
   validation and a validation-only sentinel copy.
6. The final review found no High or Medium issues. Migration tests now cover
   `1e3` and `1.0`, proving PostgreSQL normalization does not rewrite the
   historical digest identity.
7. The post-fix closure review independently rechecked `f5e6bbb` and returned
   `PASS` with `High 0`, `Medium 0`, `Low 0`.

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

The final remediation preserves migrated historical `card_digest` values as
identity evidence rather than re-hashing PostgreSQL-normalized text. The
primary Agent reviewed the patch, ran the verification matrix, and obtained a
fresh independent Review PASS. There is no known missing patch.

## Verification Evidence

After the final remediation, the following passed:

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
`catalog_test`. Pinned Linux race, final Docker build, and Compose rendering
also passed:

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

## Closure Evidence

1. Default Go tests, vet, binary build, module tidy-diff, Compose rendering,
   frontend pnpm scripts, and pinned Docker build passed.
2. Dedicated PostgreSQL 17 migration and HTTP acceptance passed with a
   database named `catalog_test`.
3. Pinned Linux Go 1.26.4 `go test -race -count=1 ./...` passed. Windows race
   was not runnable because this host has no cgo compiler.
4. Fresh post-fix Reviewer `019f5ef0-8189-7ef1-b677-3f1ef2bd29bf` returned
   `PASS`, `High 0`, `Medium 0`, `Low 0`; fallback delta is removed `0`,
   retained `3`, added `0`, net `0`.
5. Spec Kit convergence found no remaining implementation gaps and appended no
   tasks. Do not begin Workspace Installation in this branch until the user
   reviews the closure; do not push unless explicitly requested.

Do not modify `pnpm-lock.yaml` or relax `minimumReleaseAge`. A cooling-period
failure such as `postcss@8.5.18` is a supply-chain policy result, not a reason to
change project policy.
