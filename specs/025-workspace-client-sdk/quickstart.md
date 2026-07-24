# Quickstart: Validate the Workspace Client SDK

## Prerequisites

- Go 1.26.0
- Repository-local Git identity configured as required by `AGENTS.md`
- An optional running NeKiro stack with one published, installed, enabled Agent
  for manual application testing
- An out-of-band opaque Gateway Bearer mapped to the target Workspace Owner

Do not commit or print the raw application credential.

## 1. Review the frozen boundaries

Read:

- [spec.md](spec.md)
- [client-sdk-api.md](contracts/client-sdk-api.md)
- [data-model.md](data-model.md)

Confirm that `InvokeRequest` has only Agent ID, capability, and input, and that
the Client is fixed to one Gateway/Workspace/credential context.

## 2. Run focused contract and SDK tests

```powershell
go test ./contracts ./sdks/client-sdk/...
go test -race ./sdks/client-sdk/...
```

Expected outcome:

- v4 OpenAPI declares one Trace header and HTTP 500 `INTERNAL_ERROR` on both
  Northbound and Router Internal invocation responses.
- SDK configuration, request, JSON, SSE, error, cancellation, limit, redirect,
  credential-secrecy, and concurrent-client cases pass.
- Truncated or post-terminal streams never finish successfully.

## 3. Run platform adapter regressions

```powershell
go test ./apps/control-plane/internal/invocation ./apps/control-plane/internal/gateway
go test ./apps/a2a-router/internal/api
```

Expected outcome:

- Missing, duplicate, or changed Router response Trace is rejected before
  Gateway exposes a response.
- Gateway keeps the Trace it created for the northbound request.
- Router `INTERNAL_ERROR` maps to HTTP 500 rather than the 503 dependency path.

## 4. Compile the application example

```powershell
go test ./sdks/client-sdk/... -run '^Example'
```

The example documents installation as a separate Gateway operation and shows
one explicitly configured Client invoking an installed Agent. It contains no
raw committed credential and no endpoint/version/Router request field.

## 5. Run full repository gates

```powershell
go vet ./...
go test ./...
golangci-lint run
git diff --check
```

All commands must pass before independent Review.

## 6. Optional manual application call

Start the existing authenticated Compose stack using the local-development
runbook, register/verify/publish/install the sample Release, and inject the raw
Owner credential into the application process only. Run the documented Client
example against the explicit Gateway origin and Workspace.

Verify:

1. the application supplies only Agent ID, capability, and input per call;
2. managed JSON and streaming calls succeed through Gateway and Router;
3. a wrong credential returns typed `UNAUTHENTICATED`;
4. disabled Installation and revoked Release return distinct typed codes;
5. direct Agent access is neither configured nor attempted by the Client.

The complete clean Compose/PostgreSQL trusted-publication acceptance matrix is
owned by Issue #52 and is not duplicated in this slice.

## Fallback report

```text
Fallback delta: removed 2, retained 1, added 0, net -2
Added fallback evidence: none
```

The retained behavior is only Go's documented default transport when the
explicitly supplied non-nil HTTP client has a nil `Transport`. The SDK provides
no default client, timeout, URL, Workspace, credential, limit, retry, redirect,
route, result, or compatibility path.

The two removals are the conditional Router/Gateway Trace substitution and the
Router `INTERNAL_ERROR` default-to-503 status branch.
