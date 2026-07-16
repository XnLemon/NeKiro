# Quickstart: Control Plane Invocation Dispatch

Run focused and complete verification after implementation:

```powershell
go test ./apps/control-plane/internal/invocation ./apps/control-plane/internal/gateway ./apps/control-plane/internal/workspace ./apps/control-plane/internal/config ./apps/control-plane/cmd/control-plane
go test ./...
go vet ./...
```

The handler requires an authenticated `POST /v4/workspaces/{workspaceId}/invocations`, `Content-Type: application/json`, and an exact supported `Accept` value matching `stream`.

Expected outcomes:

- Focused and full tests pass with no Router/Agent process required.
- `go vet` reports no issue.
- Invalid public requests invoke neither Workspace authorization nor Router where their ordering excludes it.
- Valid requests contain one exact installed version and only the configured Router destination.
- SSE tests observe one flush for every bounded complete event.
