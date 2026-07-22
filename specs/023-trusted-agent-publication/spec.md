# Spec 023: Trusted Agent Publication

## Goal

Make a published Agent independently trustworthy at the platform boundary. A
provider must be identifiable, prove control of the declared endpoint, and
publish a release whose identity cannot be silently changed before a
Workspace installs and invokes it.

This feature supports the platform loop by making `Register -> Discover ->
Install -> Invoke -> Record` safe to rely on across provider and consumer
boundaries. It does not deploy or execute an Agent.

## User scenarios & testing

### Scenario 1: Provider claims an Agent identity

Given an authenticated provider submits an Agent Card, when the platform
accepts it, then the Card is associated with an explicit provider identity and
the owner identity cannot be changed by another provider.

### Scenario 2: Provider proves endpoint ownership

Given a registered Card declares an HTTP(S) endpoint, when the provider starts
the documented ownership challenge and the endpoint returns the exact
challenge response, then the endpoint binding becomes verified. If the
endpoint is unreachable, returns the wrong response, redirects, or resolves
to a disallowed network, verification fails with a typed result and the
release cannot be published as verified.

### Scenario 3: Provider publishes an immutable release

Given a verified endpoint binding and a valid Card version, when the provider
publishes the release, then the platform exposes the exact Card digest,
provider identity, endpoint binding, and publication state. Changing any
bound fact requires a new release; historical releases remain queryable.

### Scenario 4: Workspace installs only a trusted release

Given a Workspace installs an Agent, when version resolution runs, then it
selects an exact release that is published and verified. Pending, suspended,
revoked, or unverified releases are rejected with a distinguishable error.
Resolution evaluates the highest SemVer version matching the requested
constraint and never silently downgrades to a lower version after that release
fails the trust gate.

### Scenario 5: Operator can recover a failed publication

Given a challenge failure or a provider suspension, when an operator inspects
the release, then the state, failure reason category, timestamps, and next
action are visible without exposing challenge secrets or other credentials.

## Requirements

- **R-001 Provider identity**: The platform MUST represent provider identity
  separately from the Agent Card display owner and bind one Agent identity to
  one provider according to an explicit conflict rule.
- **R-002 Ownership challenge**: The platform MUST issue a single-use,
  time-bounded challenge for the declared endpoint and MUST accept verification
  only when the endpoint returns the exact challenge proof over the declared
  origin.
- **R-003 Network safety**: Verification MUST reject loopback, link-local,
  multicast, unspecified, and private destinations by default. Any development
  exception MUST be an explicit, validated allowlist configuration; there is no
  implicit localhost or alternate endpoint fallback.
- **R-004 Explicit failures**: Missing, malformed, expired, reused, wrong,
  unreachable, redirected, and dependency-failed verification states MUST be
  distinguishable. A dependency failure MUST NOT be reported as an unverified
  or empty result.
- **R-005 Release binding**: A release MUST bind provider identity, exact
  Agent Card version, canonical Card digest, endpoint origin/path, verification
  evidence, and publication timestamps.
- **R-006 Immutable history**: Once a release is published or revoked, its
  bound facts MUST NOT be updated in place. A changed Card or endpoint MUST
  create a new release identity. Because the endpoint is a versioned Agent
  Card fact, an endpoint change MUST first register a new Card version; one
  Card version maps to at most one release identity.
- **R-007 Installation gate**: Workspace installation and invocation MUST
  resolve an exact published, verified release and MUST reject other release
  states. Unpublished/unverified, suspended, and revoked releases MUST produce
  the distinct stable codes `AGENT_RELEASE_UNPUBLISHED`,
  `AGENT_RELEASE_SUSPENDED`, and `AGENT_RELEASE_REVOKED`; disabled
  Installations remain `INSTALLATION_DISABLED`.
- **R-008 Secret safety**: Challenge values, signing material, API keys, and
  tokens MUST NOT appear in Agent Cards, public responses, logs, errors, or
  Ledger records.
- **R-009 Ownership boundaries**: Registry owns provider/release facts;
  Workspace owns installations; Router owns transport and invocation facts.
  Cross-boundary data MUST use versioned contracts.
- **R-010 Compatibility**: Existing Agent Card v0.2 registration, discovery,
  and managed invocation remain compatible for already published sample
  Agents. Legacy unverified sample fixtures MUST NOT be treated as evidence
  that production publication is trusted.
- **R-011 Ledger release provenance**: Every invocation resolved from a trusted
  release MUST carry the exact `releaseId` and Card digest through the Router
  into the immutable Invocation Ledger projection and events. Historical
  pre-v4 published rows MUST be explicitly marked unverified in Catalog, and the
  absence of both release provenance fields is their wire-level Ledger
  encoding. Before invoking an Agent or appending Ledger events, Router MUST
  compare the dispatch provenance pair with the exact Catalog-owned pair
  returned by Control Plane resolution and fail explicitly on omission or
  mismatch. A newly registered version MUST NOT become legacy merely because it
  has no Release row.

## Non-goals

- Automatic Agent deployment, health-based routing, autoscaling, or rollback.
- Billing, rating, certification, marketplace review, federation, or full
  enterprise RBAC/OIDC.
- Provider-managed API-key forwarding, mTLS, or every external identity
  provider in the first vertical slice.
- Direct Console, Client SDK, or Agent access to Registry storage.

## Success criteria

- A provider can complete registration and endpoint verification in one
  documented flow; a wrong or unreachable endpoint never becomes verified.
- 100% of release records returned to a Workspace include an exact immutable
  identity and verification state; no record contains a secret.
- A fresh acceptance environment demonstrates
  `Register -> Verify -> Publish -> Install -> Invoke -> Record`.
- The acceptance suite distinguishes at least these failures: wrong proof,
  expired/reused challenge, disallowed destination, endpoint unavailable,
  unpublished/unverified release, disabled installation, and revoked release.
- Existing sample Agent end-to-end tests continue to pass without changing
  their runtime-internal types or bypassing Gateway/Router boundaries.

## Key entities

- **Provider**: platform identity that owns one or more Agent identities.
- **Endpoint Binding**: immutable association between a provider, endpoint
  origin/path, verification method, and verification evidence metadata.
- **Agent Release**: immutable versioned publication of an Agent Card and its
  endpoint binding.
- **Verification Challenge**: single-use, time-bounded proof request whose
  secret is never part of a persistent or public Agent record.
- **Publication State**: draft, pending verification, verified, published,
  suspended, or revoked.

## Assumptions

- The first ownership method is an HTTP well-known challenge over the exact
  endpoint origin; DNS and organization attestation are follow-up methods.
- Provider authentication uses the existing Gateway identity boundary in the
  first slice; introducing a new identity provider is out of scope.
- Challenge expiry and network allowlist values are explicit service
  configuration, not silently defaulted security settings.
- A verification attempt follows redirects no further than the declared
  endpoint; redirect targets are rejected rather than trusted.

## Open policy intentionally deferred

The retention period for verification evidence and the operator workflow for
manual suspension are deferred to the operator runbook sub-issue. This Spec
requires their state and audit category to be visible, but does not invent a
retention or approval policy.

## Slice B lifecycle decisions (#49)

The Registry stores an `Agent Release` as a separate immutable identity from
the Agent Card version. A release records the exact Card digest, provider,
endpoint binding, canonical endpoint origin/path, and verification evidence
digest. The Card version and release binding are never updated in place; a
changed Card or endpoint is represented by a new release identity.

Release creation may reference a pending or already verified binding. It
starts in `pending_verification` for a pending binding and in `verified` for a
verified binding, copying the exact existing evidence in the latter case. A
pending release becomes `verified` only after the referenced binding is
verified, and any release becomes `published` only through an explicit state
transition. A release can move from `verified` to `published`, and a verified
or published release can move to `suspended` or `revoked`; terminal `revoked`
state and all bound facts are immutable. Invalid transitions return a typed
conflict and never look like not-found or success.

Workspace installations persist the exact `releaseId` when they select a
trusted release. The field is additive and optional only for historical
installations that resolve an explicitly marked pre-v4 version; those legacy
rows remain explicitly unverified and are not eligible for the new release
state machine. New release rows must have a non-empty release identity. The
Catalog migration marks every pre-v4 published row explicitly because Trusted
Publication did not exist when any of them were published;
post-migration registrations and old publication attempts without a Release
are not eligible for installation. Trusted invocation projections and events
persist the pinned Release ID and Card digest so historical Ledger reads cannot
be reinterpreted after a later publication change.
