# Quickstart: Verify Invocation Runtime Contracts

1. Set `.specify/feature.json` to this feature and run the prerequisite checker.
2. Run focused tests: `go test ./contracts -run RuntimeContract`.
3. Run all contract tests: `go test ./contracts`.
4. Run repository tests and static checks: `go test ./...` and `go vet ./...`.
5. Confirm historical files are unchanged with `git diff -- contracts/openapi/control-plane.v3.yaml contracts/openapi/router-internal.v2.yaml contracts/schemas/platform-error.v2.schema.json contracts/schemas/invocation-event.v0.2.schema.json contracts/schemas/invocation-result-stream-event.v1.schema.json`.

Expected evidence:

- Agent-facing DTO cannot carry caller, Workspace, root/Trace, child ID, Card version, endpoint, or credential.
- Wrong caller class/auth is rejected before owned behavior by the declared security schemes.
- `AGENT_AUTH_UNSUPPORTED` and `PAYLOAD_TOO_LARGE` are distinct fixed errors.
- New event schemas preserve exact correlation and metadata-only content.
- Every runtime limit is required and has no default.
- SSE one-line framing, flushing, acceptance, and interruption rules are explicit.
