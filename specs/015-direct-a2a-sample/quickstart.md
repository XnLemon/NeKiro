# Quickstart: Deterministic Direct A2A Sample

## Prerequisites

- Go 1.26.0
- Repository root at the T005 branch

## Validate

```powershell
go test ./agents/runtime-b/...
go test -race ./agents/runtime-b/...
go test ./...
```

Expected result: all four active A2A operations pass through the official
server/client path; exact JSON, five-event success stream, explicit failure,
held-task cancellation, context headers, and concurrent identity isolation are
verified.

## Run

The listener address is required and has no default:

```powershell
$env:RUNTIME_B_LISTEN_ADDR = '127.0.0.1:4102'
go run ./agents/runtime-b/cmd/runtime-b
```

The process exits during configuration validation when the variable is absent,
blank, or not a valid TCP listener address. It does not connect to a platform
database or another platform service.

## Fixture Input

Message operations require one data part:

```json
{
  "kind": "data",
  "data": {
    "fixture": "success",
    "value": {"request": "sample"}
  }
}
```

Change `fixture` explicitly to `stream-success`, `failure`, or `hold` for the
other mapped scenarios. No omitted or unknown value is accepted.
