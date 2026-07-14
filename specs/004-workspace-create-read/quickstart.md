# Quickstart: Workspace Create and Read

This guide validates issue #4 against the dependent #3 branch. It does not
claim Installation, Router, or end-to-end invocation completion.

## Prerequisites

- Go version declared by `go.mod`.
- Docker Compose and a dedicated PostgreSQL database ending in `_test`.
- Required server configuration supplied explicitly; do not print bearer
  tokens or database credentials.

## Contract and Unit Checks

```powershell
go test -count=1 ./contracts
go test -count=1 ./apps/control-plane/internal/workspace/...
go test -count=1 ./apps/control-plane/internal/gateway
```

## PostgreSQL Evidence

Set `NEKIRO_TEST_DATABASE_URL` to an explicit `_test` database, apply the
forward migration, and run:

```powershell
go test -tags=integration -count=1 ./apps/control-plane/internal/workspace/postgres
go test -tags=integration -count=1 ./apps/control-plane/internal/workspace/integration
```

The integration evidence must cover durable create/read, duplicate conflict,
owner rejection, unknown reads, service reconstruction, and explicit
dependency/readiness failures.

## HTTP Evidence

Run the Control Plane with explicit database, listen address, and development
principal configuration. With an owner bearer token:

```powershell
$headers = @{ Authorization = "Bearer $ownerToken" }
$created = Invoke-RestMethod -Method Post -Uri "$base/v3/workspaces" `
  -Headers $headers -ContentType 'application/json' `
  -Body '{"workspaceId":"workspace-quickstart"}'
$read = Invoke-RestMethod -Method Get -Uri "$base/v3/workspaces/workspace-quickstart" `
  -Headers $headers
if ($read.workspaceId -ne $created.workspaceId -or $read.ownerId -ne $created.ownerId) {
  throw 'Workspace create/read mismatch'
}
```

Verify that an unauthenticated request returns `401`, a non-owner returns
`403`, an unknown identifier returns `404`, and a repeated create returns
`409`. Stop and reconstruct the process against the same database, then read
the Workspace again and compare all four fields.

## Static Verification and Fallback Audit

```powershell
go test -race -count=1 ./...
go vet ./...
go build ./...
go mod tidy -diff
git diff --check
```

Fallback delta must remain:

```text
Fallback delta: removed 0, retained 0, added 0, net 0
Added fallback evidence: none
```
