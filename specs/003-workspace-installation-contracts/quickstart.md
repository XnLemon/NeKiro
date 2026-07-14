# Quickstart: Validate Minimal Workspace and Installation

This guide has two gates. Section 1 validates the contract and documentation
changes delivered by issue #3. The remaining sections are runnable after the
later runtime implementation tasks in `tasks.md` are complete. They validate
`Discover -> Install` only; no A2A call, Router transport, Ledger, Agent
deployment, or Frontend is required.

## Prerequisites

- Go version declared by `go.mod`
- Docker Engine and Docker Compose 2.20 or newer for runtime acceptance
- A dedicated local PostgreSQL database whose name ends in `_test`
- Repository dependencies downloaded with checksum verification enabled
- Current working directory at repository root

Never point integration tests or migration validation at a shared, staging, or
production database. The suite may clear Workspace-owned rows.

## 1. Validate the Contract Gate

```powershell
go test -count=1 ./contracts
go vet ./contracts
git diff --check
```

Expected outcomes:

- Workspace v1 accepts exactly the four approved fields.
- Installation v2 requires `uninstalledAt` only for terminal history and rejects
  non-canonical permission order or timestamp relationships.
- Northbound v3 exposes create/read Workspace and create/read/list/lifecycle
  Installation operations with Bearer security, Trace headers, and exact errors.
- Control Plane Internal v2 requires a trusted service identity, distinguishes
  pre-correlation errors, and preserves request correlation after validation.
- Platform Error v3 accepts `INSTALLATION_DISABLED` only with its fixed public
  message; Platform Error v2 remains unchanged for Catalog/Invocation.
- Historical contract files remain unchanged.
- Spec, plan, research, data model, contract guide, tasks, ADR, and compatibility
  documentation contain no unresolved placeholder or fallback policy.

## 2. Configure a Dedicated Test Database

After runtime implementation, create the ignored local environment file and
choose explicit local-only values. The application provides no database,
listen, caller, token, or internal-service defaults.

```powershell
Copy-Item .env.example .env
docker compose --env-file .env --file deploy/compose.yaml config --quiet
docker compose --env-file .env --file deploy/compose.yaml up --detach --wait postgres
$env:NEKIRO_TEST_DATABASE_URL = 'postgresql://<user>:<password>@127.0.0.1:<port>/<database>_test?sslmode=disable'
$env:NEKIRO_DATABASE_URL = $env:NEKIRO_TEST_DATABASE_URL
```

Generate separate local bearer tokens for one owner, one non-owner, and one
trusted internal service. Store only SHA-256 digests in configuration. Do not
print, commit, or place raw tokens in `.env`.

The later implementation requires a separately configured internal
authenticator. A Northbound principal is not implicitly trusted to call
`/internal/v2/resolve-agent`, and an internal principal is not a Workspace owner
unless independently configured as one.

## 3. Apply Explicit Migrations

```powershell
go run ./apps/control-plane/cmd/control-plane migrate up
```

Expected outcomes:

- Pending Catalog and Workspace migrations apply in order.
- Re-running `migrate up` leaves committed data unchanged.
- Unsupported directions fail before changing either module's schema.
- Serving still does not migrate automatically.
- Readiness fails if either required module schema is missing or invalid; it
  never reports degraded success.

## 4. Run Unit and Contract Verification

```powershell
go test -count=1 ./contracts
go test -count=1 ./apps/control-plane/internal/catalog/...
go test -count=1 ./apps/control-plane/internal/workspace/...
go test -count=1 ./apps/control-plane/internal/gateway/...
```

Expected outcomes:

- Strict SemVer range, pre-release participation, and build-metadata tie-break
  cases select one deterministic exact version.
- Owner-only policy, permission subset canonicalization, capability set
  containment, and every lifecycle transition pass their table tests.
- Invalid, unauthenticated, forbidden, not-found, conflict,
  installation-disabled, Agent-disabled, capability-denied, and dependency
  outcomes map to the exact contract statuses and codes.
- No unit test adds a retry, stale Card, default owner, wildcard constraint, or
  same-state success policy.

## 5. Run PostgreSQL and HTTP Acceptance

```powershell
go test -tags=integration -count=1 ./apps/control-plane/internal/workspace/postgres ./tests/integration/workspace
```

Expected outcomes:

- Owner Workspace and Installation facts survive process reconstruction and a
  PostgreSQL restart.
- Stable and explicitly eligible pre-release versions resolve by SemVer; equal
  precedence uses the exact-string tie-break.
- Unknown accepted permissions fail without a partial Installation.
- Empty accepted permissions persist and authorize only permission-free
  capabilities.
- One hundred simultaneous same-Agent install requests commit exactly one
  current Installation.
- Lifecycle races cannot skip disabled, create two current rows, or resurrect
  uninstalled history.
- List returns bounded pages in stable order with opaque continuation and
  returns an empty array only for a real empty Workspace.
- Exact resolution returns only an enabled, exact, currently published,
  capability-authorized Card and Installation.
- Catalog disable preserves the pin and changes the next resolution to
  `AGENT_DISABLED`.
- Injected Workspace and Catalog failures return `DEPENDENCY_ERROR` and never
  use process memory, empty results, or historical Cards.

## 6. Exercise the Owner Workflow

Start the implemented Control Plane with explicit database, listen,
Northbound-auth, and internal-auth configuration. Use one owner bearer without
printing it.

```powershell
$ownerHeaders = @{Authorization = "Bearer $ownerToken"}
$base = 'http://127.0.0.1:18080'

$workspace = Invoke-RestMethod -Method Post -Uri "$base/v3/workspaces" `
  -Headers $ownerHeaders -ContentType 'application/json' `
  -Body '{"workspaceId":"workspace-quickstart"}'

$readWorkspace = Invoke-RestMethod -Method Get `
  -Uri "$base/v3/workspaces/workspace-quickstart" -Headers $ownerHeaders

$installBody = @{
  agentId = 'runtime-a'
  versionConstraint = '^1.0.0'
  acceptedPermissions = @()
} | ConvertTo-Json -Compress

$installation = Invoke-RestMethod -Method Post `
  -Uri "$base/v3/workspaces/workspace-quickstart/installations" `
  -Headers $ownerHeaders -ContentType 'application/json' -Body $installBody

$listed = Invoke-RestMethod -Method Get `
  -Uri "$base/v3/workspaces/workspace-quickstart/installations?limit=25" `
  -Headers $ownerHeaders

if ($null -ne $listed.nextCursor) {
  $cursor = [uri]::EscapeDataString($listed.nextCursor)
  $nextPage = Invoke-RestMethod -Method Get `
    -Uri "$base/v3/workspaces/workspace-quickstart/installations?limit=25&cursor=$cursor" `
    -Headers $ownerHeaders
}

$disabled = Invoke-RestMethod -Method Patch `
  -Uri "$base/v3/workspaces/workspace-quickstart/installations/$($installation.installationId)" `
  -Headers $ownerHeaders -ContentType 'application/json' `
  -Body '{"status":"disabled"}'

$enabled = Invoke-RestMethod -Method Patch `
  -Uri "$base/v3/workspaces/workspace-quickstart/installations/$($installation.installationId)" `
  -Headers $ownerHeaders -ContentType 'application/json' `
  -Body '{"status":"enabled"}'

$disabledAgain = Invoke-RestMethod -Method Patch `
  -Uri "$base/v3/workspaces/workspace-quickstart/installations/$($installation.installationId)" `
  -Headers $ownerHeaders -ContentType 'application/json' `
  -Body '{"status":"disabled"}'

$uninstalled = Invoke-RestMethod -Method Delete `
  -Uri "$base/v3/workspaces/workspace-quickstart/installations/$($installation.installationId)" `
  -Headers $ownerHeaders
```

Verify:

```powershell
if ($workspace.ownerId -ne $readWorkspace.ownerId) { throw 'Workspace owner changed' }
if ($installation.installedVersion -eq '') { throw 'Exact version was not pinned' }
if ($listed.items.Count -ne 1) { throw 'Installation list is incorrect' }
if ($disabled.status -ne 'disabled' -or $enabled.status -ne 'enabled') { throw 'Lifecycle failed' }
if ($uninstalled.status -ne 'uninstalled' -or $null -eq $uninstalled.uninstalledAt) { throw 'Uninstall history failed' }
```

Use the non-owner token to read the Workspace and confirm `403 FORBIDDEN`. Use
the owner token with a repeated create, repeated state update, direct enabled
uninstall, and repeated uninstall; confirm each documented conflict rather than
success.

## 7. Exercise Internal Exact Resolution

Create a fresh enabled Installation whose accepted permissions authorize the
fixture capability. Use only the separately configured internal token.

```powershell
$internalHeaders = @{Authorization = "Bearer $internalToken"}
$resolveBody = @{
  invocationId = 'invocation-quickstart'
  rootTaskId = 'task-quickstart'
  traceId = 'trace-quickstart'
  workspaceId = 'workspace-quickstart'
  agentId = 'runtime-a'
  version = $activeInstallation.installedVersion
  capability = 'runtime.echo'
} | ConvertTo-Json -Compress

$resolved = Invoke-RestMethod -Method Post `
  -Uri "$base/internal/v2/resolve-agent" -Headers $internalHeaders `
  -ContentType 'application/json' -Body $resolveBody
```

Confirm the Card version equals both requested `version` and
`installation.installedVersion`, status is `enabled`, and the response Trace
header equals `trace-quickstart`.

Then independently verify:

- owner token on the internal route -> `401 UNAUTHENTICATED`;
- mismatched exact version or uninstalled record -> `404 AGENT_NOT_INSTALLED`;
- disabled Installation -> `403 INSTALLATION_DISABLED`;
- disabled Catalog version -> `403 AGENT_DISABLED`;
- missing or permission-denied capability -> `403 CAPABILITY_NOT_ALLOWED`;
- injected Catalog/Workspace failure -> `503 DEPENDENCY_ERROR`.

Every internal error after valid correlation must repeat the exact three request
correlation IDs and contain no Card fragment. Missing or malformed correlation
must instead return only fixed error fields plus a generated trace ID.

## 8. Run Repository Verification

```powershell
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./apps/control-plane/cmd/control-plane
go mod tidy -diff
git diff --check
```

CI additionally provisions PostgreSQL and runs integration-tagged Catalog and
Workspace suites. Do not change `pnpm-lock.yaml` or relax package cooling policy
for this backend feature.

## 9. Scope and Fallback Audit

Confirm that:

- no Workspace or Installation field contains Kubernetes, deployment,
  membership, generic policy, credentials, invocation content, or Agent output;
- Workspace SQL never reads or writes Catalog-owned tables;
- exact version pins and accepted permissions never auto-upgrade;
- Catalog and PostgreSQL failures never become empty, stale, not-found, or
  success responses;
- the only retained fallback-classified behavior is the approved genuine empty
  Installation list;
- fallback delta remains removed `0`, retained `1`, added `0`, net `0`, with
  added fallback evidence `none`.

## 10. Stop Local Dependencies

```powershell
docker compose --env-file .env --file deploy/compose.yaml down
```

Do not add `--volumes` unless intentionally deleting all local test data.
