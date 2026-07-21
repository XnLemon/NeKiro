# Spec 022: Console runtime integration

## Goal

Connect the existing Console to the active Northbound Invocation API v4 and
metadata read APIs, while allowing the local Console origin to call the
Control Plane through an explicit CORS policy.

## User scenarios

1. An Owner opens the Console with one active Workspace, invokes an enabled
   Agent with JSON input, and sees the real result or a correlated Platform
   Error.
2. An Owner requests streaming output and sees ordered v2 result events until
   a terminal event; an interrupted stream is surfaced as an error.
3. An Owner reads Invocation detail or Trace metadata for the active Workspace
   and can inspect the recorded lineage without result payload persistence.
4. A browser served from a configured local origin can call public Gateway
   routes; internal Router routes remain unavailable to browser origins.

## Requirements

- Use only the active Northbound v4 paths and runtime contract versions.
- Preserve exact JSON/SSE ordering, correlation, and terminal semantics.
- Map pre-correlation and correlated Platform Error v4 payloads without
  inventing messages or identifiers.
- Keep one active Workspace and Owner authorization as the only supported
  Console policy in this slice.
- Agent authentication UI declares only the authentication type; it never
  collects or persists secrets.
- The development bearer token is required at the Console configuration
  boundary and must be sent exactly as configured; surrounding whitespace is
  invalid.
- Installation list requests must send an explicit validated limit.
- Missing Agent Card facts must fail validation instead of receiving generated
  defaults.
- CORS must require an explicit, exact origin allowlist. Wildcard and implicit
  localhost fallback are not allowed.
- CORS applies to public northbound routes only; preflight does not require a
  bearer token.
- Remove unused AI Studio/Gemini/Express scaffolding from the Console.

## Non-goals

- Multiple Workspace selection, delegated roles, or enterprise RBAC.
- Browser access to internal Router routes.
- Client-side persistence or replay of Invocation/Ledger data.
- Agent credentials, deployment, or runtime orchestration.

## Acceptance criteria

- `GET/POST/PATCH/DELETE/OPTIONS` public routes respond with the configured
  exact CORS origin and required headers; an unlisted origin receives no CORS
  grant.
- Console invokes through `/v4/workspaces/{workspaceId}/invocations` and
  reads `/v4/workspaces/{workspaceId}/invocations/{invocationId}` and
  `/v4/workspaces/{workspaceId}/traces/{traceId}`.
- JSON and SSE client tests cover request construction, v4 errors,
  correlation, order, and terminal/interrupted behavior.
- Console typecheck, tests, and production build pass.
- Backend Go tests and vet pass.
