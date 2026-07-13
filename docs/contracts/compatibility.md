# Contract Compatibility Policy

## Versions

Agent Card Schema version, Agent version, HTTP API version, event version, and A2A protocol version are independent values. They must not be inferred from one another.

Phase 1 starts with:

| Contract | Version |
| --- | --- |
| Agent Card Schema | `0.1` |
| Northbound API | `v1` |
| Internal API | `v1` |
| Invocation Event Schema | `0.1` |
| A2A protocol profile | `0.3.0` |

The versioned JSON Schema, OpenAPI, and A2A Profile files under `contracts/` are the contract facts. Go structs and future TypeScript types must be validated against or generated from those files.

## Compatible Changes

- Adding an optional field is additive.
- Adding a new endpoint or event type is additive when existing consumers remain valid.
- Adding an enum member requires consumer impact review; exhaustive consumers may treat it as breaking.

## Breaking Changes

- Removing or renaming a field
- Changing a field type or requiredness
- Changing an existing field's semantics
- Reusing an error code for a different state
- Reinterpreting historical Ledger events

Breaking changes require a new contract version, migration guidance, and a stated compatibility window.

## Failure Semantics

Missing input, invalid input, not found, forbidden, disabled, dependency failure, timeout, cancellation, and protocol failure are distinct states. Contracts must not collapse them into `null`, an empty collection, a boolean, or a normal success response.
