# Data model: Trusted Agent Publication

## Provider

| Field | Meaning |
| --- | --- |
| provider_id | Stable platform identifier |
| owner_identity | Authenticated owner binding |
| verification_status | `unverified`, `verified`, `suspended` |
| verification_method | `http_well_known` for Slice A |
| verified_at | Timestamp of successful provider verification |
| created_at / updated_at | Registry lifecycle timestamps |

An Agent identity may claim at most one provider identity. The claim is a
Registry ownership fact and is not inferred from the Card's display owner.
The first provider binding establishes the claim; another provider receives a
conflict and cannot replace it.

## Endpoint Binding

| Field | Meaning |
| --- | --- |
| binding_id | Stable binding identity |
| provider_id | Owning Provider |
| agent_id | Bound Agent identity |
| agent_card_version | Exact Card version resolved by the binding |
| endpoint_origin | Canonical scheme/host/port |
| endpoint_path | Exact A2A path |
| verification_status | `pending`, `verified`, `failed`, `revoked` |
| verification_method | Proof method identifier |
| verification_evidence_digest | Non-secret evidence digest |
| verification_failure_code | Typed category, never raw dependency detail |
| verified_at / revoked_at | State timestamps |

## Verification Challenge

| Field | Meaning |
| --- | --- |
| challenge_id | Public lookup identity |
| binding_id | Target endpoint binding |
| proof_digest | Hash of the one-time proof |
| expires_at | Explicit expiry boundary |
| used_at | Single-use marker |
| created_at | Creation timestamp |

The raw proof is transient and is never a persistent field. All identifiers
are validated at the Gateway/Registry boundary. No model has an endpoint
credential, API key, or JWT signing material.

## Agent Release

| Field | Meaning |
| --- | --- |
| release_id | Stable immutable publication identity |
| provider_id | Provider that owns the publication |
| agent_id / agent_card_version | Exact Card version bound by the release |
| card_digest | SHA-256 digest of the canonical Card bytes |
| endpoint_binding_id | Exact verified/pending binding referenced by the release |
| endpoint_origin / endpoint_path | Copied canonical A2A destination facts |
| verification_method | Ownership proof method copied from the binding |
| verification_evidence_digest | Non-secret proof evidence copied from the binding |
| state | `draft`, `pending_verification`, `verified`, `published`, `suspended`, or `revoked` |
| created_at / verified_at / published_at / suspended_at / revoked_at | State timestamps |

The release identity and every binding/digest field are immutable. Only the
state and its corresponding timestamp may change through the transition table.
Each `(agent_id, agent_card_version)` has at most one Release. The endpoint is
part of the immutable Agent Card version, so endpoint rotation first requires
a newly registered Card version and then a new Release identity; the unique
version binding does not permit endpoint mutation in place.
`installed_release_id` is copied into a Workspace Installation for trusted
rows; a null value is retained only when resolution selects an explicitly
marked pre-v4 legacy row.

## Legacy publication marker

The Catalog keeps an internal `legacy_unverified` marker on Agent versions.
Migration sets it for every row already published before schema v4 because
none of those rows could have passed the Trusted Publication release flow;
the Phase 1 samples are included in that bounded pre-v4 set. New registrations
are false, and the marker is never inferred from a missing Release row. This
preserves pre-v4 compatibility without allowing a newly published but
unverified version to enter a Workspace installation.

## Invocation release provenance

Trusted Invocation projections and immutable lifecycle events carry the
optional additive pair `agent_release_id` and `agent_card_digest`. Both fields
are absent for explicitly pre-v4 legacy invocations or both are present for a
trusted release. Pair absence is the explicit legacy/unverified wire encoding
retained for Invocation Event 0.3 compatibility; it does not authorize
Catalog to infer legacy status for a new version. The digest is the canonical
SHA-256 Card digest encoded as lower-case hexadecimal. Control Plane exact Card
resolution returns the same pair from Catalog-owned facts, and Router requires
it to match the incoming dispatch pair before transport or Ledger persistence;
Router does not recompute a digest from response JSON whose numeric spelling
may have been normalized by historical storage migration.
