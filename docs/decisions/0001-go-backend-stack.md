# ADR 0001: Go Backend And Router

- Status: Accepted
- Date: 2026-07-13

## Context

The first contract prototype used TypeScript and TypeBox for both runtime validation and shared types. The platform architecture later fixed a different long-term boundary: React/Vite/TypeScript for the Console, with Go for the Control Plane, A2A Router, and initial Agent SDK.

Using TypeScript runtime types as the contract source would make Go services depend on Node tooling and would create two possible sources of truth once Go structs exist.

## Decision

- The repository uses one root Go module for backend services and Go libraries.
- The Control Plane and A2A Router use Go standard library HTTP boundaries unless a later ADR proves another Router is necessary.
- PostgreSQL access will use `pgx/v5`.
- A2A interoperability uses `github.com/a2aproject/a2a-go` pinned by the versioned A2A Profile.
- JSON Schema and OpenAPI are the cross-language contract sources.
- Go structs map the contract artifacts and are tested against them.
- Node.js remains frontend and engineering tooling only.

## Consequences

- The TypeBox prototype is removed before business modules begin.
- Backend modules can share Go domain types without importing frontend code.
- Future TypeScript client types must be generated from or validated against the same artifacts.
- Contract changes require compatibility review independent of either language implementation.
