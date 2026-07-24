# Acceptance Matrix Contract

This is an acceptance mapping over existing versioned contracts, not a new wire
contract.

| Boundary | Case | Expected outcome | Ledger expectation |
| --- | --- | --- | --- |
| Trusted Publication v1 | correct challenge | verified Binding with evidence digest | none |
| Trusted Publication v1 | wrong proof | HTTP 400 `WRONG_PROOF`; Binding failure `wrong_proof` | none |
| Trusted Publication v1 | expired challenge | HTTP 409 `CHALLENGE_EXPIRED` | none |
| Trusted Publication v1 | reused challenge | HTTP 409 `CHALLENGE_REUSED` | none |
| Trusted Publication v1 | disallowed destination | HTTP 403 `DISALLOWED_NETWORK` | none |
| Trusted Publication v1 | verification endpoint unavailable | HTTP 503 `ENDPOINT_UNAVAILABLE` | none |
| Installation v2 | unpublished Release install | `AGENT_RELEASE_UNPUBLISHED` | none |
| Northbound Invocation v4 | disabled Installation | HTTP 409 `INSTALLATION_DISABLED`, pre-correlation shape | none |
| Northbound Invocation v4 | suspended Release | HTTP 409 `AGENT_RELEASE_SUSPENDED`, pre-correlation shape | none |
| Northbound Invocation v4 | revoked Release | HTTP 409 `AGENT_RELEASE_REVOKED`, pre-correlation shape | none |
| Router Agent Credential v1 | forged signature | HTTP 401 `UNAUTHENTICATED` | none; direct Agent boundary |
| Router Agent Credential v1 | expired credential | HTTP 401 `UNAUTHENTICATED` | none; direct Agent boundary |
| Router Agent Credential v1 | wrong audience | HTTP 403 `FORBIDDEN` | none; direct Agent boundary |
| Router Agent Credential v1 | no credential | HTTP 401 `UNAUTHENTICATED` | none; direct Agent boundary |
| Router/Ledger v4/v0.3 | accepted endpoint unavailable | correlated `AGENT_UNAVAILABLE` | terminal `failed` with Release ID/digest |
| Router/Ledger v4/v0.3 | caller cancellation during chunk/terminal commit | local `CANCELED`; contiguous writable SSE | exactly one terminal `canceled`; no interrupted chunk or dependency remap |
| Router/Ledger v4/v0.3 | nested success | succeeded root and child | one Trace, one root Task, exact parent and per-Agent Release provenance |

All public/persisted/log surfaces are subject to the secret prohibition in
FR-012. A pre-correlation shape must not be padded with empty Invocation IDs;
an accepted correlated failure must not omit them.
