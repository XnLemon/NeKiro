# Quickstart: Validate Catalog Registration and Discovery

This guide is runnable after all implementation and test tasks in `tasks.md`
are complete. It validates the first Control Plane Catalog slice only. No A2A
Router, Agent process, Workspace Installation, or Frontend is required.

## Prerequisites

- Go version declared by `go.mod`
- Docker Engine and Docker Compose 2.20 or newer
- A dedicated local PostgreSQL database whose name ends in `_test`
- Repository dependencies downloaded with checksum verification enabled
- Current working directory at repository root

Do not point the integration suite at a shared, staging, or production database.
It applies the feature migrations and clears Catalog-owned test rows.

## 1. Configure and Start PostgreSQL

Create the ignored local environment file from `.env.example` and choose
non-empty local-only values. Use a database name ending in `_test`, an explicit
Compose PostgreSQL URL whose host is `postgres`, one strict development
principal digest array, and an available Control Plane port. Do not put the raw
bearer token in `.env`.

```powershell
Copy-Item .env.example .env
docker compose --env-file .env --file deploy/compose.yaml config --quiet
docker compose --env-file .env --file deploy/compose.yaml up --detach --wait postgres
```

Set an explicit test DSN from those local values without printing it. No DSN,
host, port, database, credential, or SSL-mode default is provided by the
application.

```powershell
$env:NEKIRO_TEST_DATABASE_URL = 'postgresql://<user>:<password>@127.0.0.1:<port>/<database>_test?sslmode=disable'
$env:NEKIRO_DATABASE_URL = $env:NEKIRO_TEST_DATABASE_URL
```

## 2. Apply Explicit Migrations

```powershell
go run ./apps/control-plane/cmd/control-plane migrate up
```

Expected outcomes:

- The command applies every pending embedded migration in order.
- Re-running it reports no pending migration and does not rewrite Catalog data.
- `migrate down` and every non-`up` direction fail before changing schema or
  deleting data; destructive rollback is not a public Catalog operation.
- Missing, blank, malformed, or unreachable database configuration fails
  explicitly; the command does not create a localhost/default connection.

## 3. Run Contract and Unit Verification

```powershell
go test -count=1 ./contracts
go test -count=1 ./apps/control-plane/...
```

Expected outcomes:

- Northbound API v2 declares the completed Catalog authentication, pagination,
  visibility, and exact error mappings.
- Active Agent Card structural and semantic validation remains unchanged.
- Registration, ownership, lifecycle, cursor, configuration, and fixed-error
  unit cases pass without a database dependency.

## 4. Run PostgreSQL and HTTP Acceptance

```powershell
go test -tags=integration -count=1 ./apps/control-plane/internal/catalog/postgres ./tests/integration/catalog
```

Expected outcomes:

- A valid Runtime-A Card registers as immutable draft, publishes, and appears in
  capability discovery.
- A conforming Card with `1e1000001`, beyond PostgreSQL `jsonb` and the pinned
  Schema library's numeric-materialization range, registers, survives process
  reconstruction, and round-trips through exact read and Discovery without
  coercion.
- A valid Runtime-B Card follows the same path without loading either Runtime.
- Invalid, duplicate, cross-owner, unauthenticated, forbidden, not-found, and
  illegal-state cases produce their distinct fixed outcomes.
- Draft and disabled versions never appear in discovery.
- Exact reads, idempotent disablement, process reconstruction, and committed
  timestamps remain durable.
- The integration suite proves unsupported public migration directions leave a
  populated Catalog unchanged.
- Cursor traversal returns a fixed 1,000-version dataset exactly once, rejects
  malformed/filter-mismatched cursors, excludes later publications, and removes
  versions disabled before a later page.
- Injected database failure is explicit and never returns empty/stale success.
- The 10,000-version acceptance dataset meets the first-page latency criterion.

## 5. Validate the Runnable Process

Generate local-only bearer credentials and set every required server variable
explicitly. The development authentication mode has no built-in caller or token.

```powershell
$rng = [Security.Cryptography.RandomNumberGenerator]::Create()
$tokenBytes = New-Object byte[] 32
$rng.GetBytes($tokenBytes)
$rng.Dispose()
$token = ([BitConverter]::ToString($tokenBytes)).Replace('-', '').ToLowerInvariant()
$sha = [Security.Cryptography.SHA256]::Create()
$tokenHash = ([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($token)))).Replace('-', '').ToLowerInvariant()
$sha.Dispose()
$env:NEKIRO_LISTEN_ADDRESS = '127.0.0.1:18080'
$env:NEKIRO_AUTH_MODE = 'development-static'
$principal = [ordered]@{id='catalog-dev'; tokenSha256=$tokenHash}
$env:NEKIRO_DEV_AUTH_PRINCIPALS_JSON = ConvertTo-Json -InputObject (, $principal) -Compress
$binary = Join-Path $env:TEMP "nekiro-control-plane-$PID.exe"
go build -o $binary ./apps/control-plane/cmd/control-plane
$server = Start-Process -FilePath $binary -ArgumentList 'serve' -PassThru -WindowStyle Hidden
$binary healthcheck 'http://127.0.0.1:18080/readyz'
```

Use the same shell to exercise the five Catalog operations without printing the
token. The fixture capability is `runtime.echo`.

```powershell
$headers = @{Authorization = "Bearer $token"}
$card = Get-Content tests/fixtures/catalog/runtime-a-card.json -Raw | ConvertFrom-Json
$body = @{card = $card} | ConvertTo-Json -Depth 100 -Compress
$draft = Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18080/v2/agents' -Headers $headers -ContentType 'application/json' -Body $body
$published = Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18080/v2/agents/runtime-a/versions/1.0.0/publish' -Headers $headers
$found = Invoke-RestMethod -Method Get -Uri 'http://127.0.0.1:18080/v2/agents?capability=runtime.echo' -Headers $headers
$disabled = Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:18080/v2/agents/runtime-a/versions/1.0.0/disable' -Headers $headers
$afterDisable = Invoke-RestMethod -Method Get -Uri 'http://127.0.0.1:18080/v2/agents?capability=runtime.echo' -Headers $headers
if ($draft.publicationStatus -ne 'draft' -or $published.publicationStatus -ne 'published' -or $found.items.Count -ne 1 -or $disabled.publicationStatus -ne 'disabled' -or $afterDisable.items.Count -ne 0) { throw 'Catalog live workflow failed' }
Stop-Process -Id $server.Id
Remove-Item -LiteralPath $binary
```

Never commit or emit the generated token or digest. The process must expose
liveness/readiness separately, reject a missing bearer credential, and return
one trace ID consistently in the response header and Platform Error body.

Stopping and restarting the process must preserve every committed Catalog row.
Changing or omitting required configuration must fail startup rather than
falling back to localhost, anonymous access, or an in-memory store.

## 6. Run Repository Static Verification

```powershell
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./apps/control-plane/cmd/control-plane
git diff --check
```

CI additionally provisions PostgreSQL and runs the integration-tagged Catalog
suite. Frontend dependency policy is unchanged; do not modify pnpm
`minimumReleaseAge` or regenerate the lockfile to work around a package cooling
period.

## 7. Stop Local Dependencies

```powershell
docker compose --env-file .env --file deploy/compose.yaml down
```

Do not add `--volumes` unless intentionally deleting all local database data.

## 8. Review Migration and Scope Evidence

Confirm that:

- historical Northbound v1 and Agent Card 0.1 artifacts remain unchanged;
- no runtime route accepts those historical versions;
- Discovery has no independent Card store, stale-cache fallback, or search
  cluster;
- Catalog contains no Workspace, Invocation, Router, Ledger, deployment, health,
  Runtime, or Agent credential data;
- every implementation module reports fallback delta/evidence and total added
  fallback count remains zero.
