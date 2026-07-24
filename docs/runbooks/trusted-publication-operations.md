# Trusted publication operations

This runbook operates the current `http_well_known` trusted-publication flow
and diagnoses its accepted failure categories. It uses only Gateway APIs and
explicit deployment configuration. It does not use SQL to change domain state,
call an Agent directly as a recovery path, or mutate an immutable Release.

## Scope and invariants

- The provider owns Agent Card registration, endpoint proof material, new Card
  versions, Endpoint Bindings, and Releases.
- The Workspace owner owns Installation creation and the explicit
  `enabled`/`disabled` state.
- The platform operator owns Control Plane/Router availability, endpoint
  verification network policy, and Router signing/Agent verifier deployment
  configuration.
- Registry owns Cards, Bindings, and Releases. Workspace owns Installations.
  Router owns accepted Invocation and Ledger facts.
- The successful Router `created` Ledger commit is the accepted-Invocation
  boundary. Earlier Gateway policy failures have a Gateway-created Trace and
  typed error but no Invocation Ledger record. Direct Agent rejections expose
  only the fixed 401/403 response; they have no authoritative platform Trace
  or Ledger fact.
- A challenge is single-use and time-bounded. A failed, expired, or consumed
  challenge is never repaired or reused.
- A published Release preserves its Card digest, endpoint Binding, and trust
  evidence. A suspended or revoked Release is not restored in place.
- Recovery always ends with an explicit read or a fresh caller-initiated
  invocation. There is no automatic retry, alternate endpoint, credential
  fallback, or old-contract compatibility path.

## Safe request setup

Set the Gateway origin and the provider/Workspace-owner credential in the
invoking shell. Do not put the raw bearer value in Cards, request bodies,
logs, issue comments, or committed files.

```powershell
$gateway = 'https://gateway.example'
$token = '<provider-or-workspace-owner-credential>'
$headers = @{ Authorization = "Bearer $token" }
```

Gateway API error responses include `x-nek-trace-id`. Record that safe Trace
value for investigation. A direct Agent 401/403 is outside Gateway correlation
and does not supply an authoritative platform Trace. Never record the
Authorization header, challenge proof, signed Router credential, signature,
private/public key text, or `jti`.

## Publish a trusted Agent version

Register the Agent Card through `POST /v3/agents` before these steps. The Card
declares the exact Agent version and endpoint; it contains no endpoint secret.

1. Create an Endpoint Binding for that exact version.

   ```powershell
   $binding = Invoke-RestMethod -Method Post -Uri "$gateway/v4/providers/$providerId/agents/$agentId/endpoint-bindings" -Headers $headers -ContentType 'application/json' -Body (@{
     endpoint = $endpoint
     method = 'http_well_known'
     version = $version
   } | ConvertTo-Json -Compress)
   ```

2. Request a one-time challenge.

   ```powershell
   $challenge = Invoke-RestMethod -Method Post -Uri "$gateway/v4/providers/$providerId/endpoint-bindings/$($binding.bindingId)/challenges" -Headers $headers
   ```

   This authenticated issuance response is the only public response allowed
   to contain the raw proof. Make the exact proof available from the bound
   Agent origin at
   `/.well-known/nekiro/challenges/{challengeId}` until completion, without
   logging or persisting it in platform data.

3. Complete the challenge once.

   ```powershell
   $binding = Invoke-RestMethod -Method Post -Uri "$gateway/v4/providers/$providerId/endpoint-bindings/$($binding.bindingId)/challenges/$($challenge.challengeId)/complete" -Headers $headers
   ```

   Completion is successful only when `verificationStatus` is `verified` and
   `verificationEvidenceDigest` is present. Remove the raw proof from the
   Agent endpoint after the request finishes.

4. Create and publish the immutable Release.

   ```powershell
   $release = Invoke-RestMethod -Method Post -Uri "$gateway/v4/providers/$providerId/agents/$agentId/releases" -Headers $headers -ContentType 'application/json' -Body (@{
     version = $version
     endpointBindingId = $binding.bindingId
   } | ConvertTo-Json -Compress)

   if ($release.state -eq 'pending_verification') {
     $release = Invoke-RestMethod -Method Post -Uri "$gateway/v4/releases/$($release.releaseId)/verify" -Headers $headers
   }
   $release = Invoke-RestMethod -Method Post -Uri "$gateway/v4/releases/$($release.releaseId)/publish" -Headers $headers
   ```

   Completion requires `state=published`, the expected Agent/Card version,
   Card digest, Binding ID, `verificationMethod=http_well_known`, evidence
   digest, and `publishedAt`.

5. The Workspace owner installs the version through
   `POST /v3/workspaces/{workspaceId}/installations` and verifies that
   `installedReleaseId` equals the published Release ID and `status=enabled`.

## Inspect trust and Invocation provenance

Use the owning public reads; do not join module tables manually.

```powershell
$binding = Invoke-RestMethod -Method Get -Uri "$gateway/v4/providers/$providerId/endpoint-bindings/$bindingId" -Headers $headers
$release = Invoke-RestMethod -Method Get -Uri "$gateway/v4/releases/$releaseId" -Headers $headers
$invocation = Invoke-RestMethod -Method Get -Uri "$gateway/v4/workspaces/$workspaceId/invocations/$invocationId" -Headers $headers
$trace = Invoke-RestMethod -Method Get -Uri "$gateway/v4/workspaces/$workspaceId/traces/$traceId" -Headers $headers
```

For every accepted trusted Invocation, verify this chain:

```text
Invocation/Event agentReleaseId + agentCardDigest
  -> GET /v4/releases/{agentReleaseId}
  -> same Agent ID + Card version + Card digest
  -> published state + Endpoint Binding + http_well_known evidence metadata
```

For a nested call, root and child use one Trace and root Task, the child names
the root Invocation as `parentInvocationId`, and each Invocation links to its
own exact Release.

## Suspend and revoke

Suspension blocks new managed invocations but keeps the historical Release
queryable. Revocation is terminal.

```powershell
$suspended = Invoke-RestMethod -Method Post -Uri "$gateway/v4/releases/$releaseId/suspend" -Headers $headers
$revoked = Invoke-RestMethod -Method Post -Uri "$gateway/v4/releases/$releaseId/revoke" -Headers $headers
```

Confirm `state=suspended` and `suspendedAt`, or `state=revoked` and
`revokedAt`. Do not edit timestamps, clear the state, or republish the same
Release. To resume distribution, register a new Card version and complete a
new Binding/challenge/Release lifecycle.

## Automatic facts versus manual actions

| Boundary | Automatically recorded | Manual owner action |
| --- | --- | --- |
| Challenge completion | Binding status, safe failure code, timestamps, and evidence digest on success | Provider corrects endpoint proof material and creates a fresh challenge |
| Release lifecycle | Immutable Release identity, state, Binding/evidence metadata, and transition timestamps | Provider creates/publishes a new version after suspension or revocation |
| Installation | Exact installed Release/version, accepted permissions, status, and history | Workspace owner explicitly enables or disables the Installation |
| Accepted invocation | Invocation/Task/Trace correlation, exact Release provenance, status, latency, and typed error | Caller decides whether to issue a fresh invocation after the cause is corrected |
| Pre-acceptance rejection | Gateway Trace and typed error only | Provider or Workspace owner corrects the rejected state/request |
| Direct Agent rejection | Fixed 401/403 at the Agent adapter; no Ledger fact | Platform operator corrects managed signing/verifier deployment; callers return to Gateway |

## Failure and recovery matrix

| Observed state or error | Inspect | Primary owner | Manual next action | Completion check |
| --- | --- | --- | --- | --- |
| `WRONG_PROOF`; Binding `failed/wrong_proof` | Gateway Trace and `GET` Binding | Provider | Remove the wrong material, serve the exact newly issued proof, create a fresh challenge, and complete it once | Binding is `verified` with an evidence digest; old proof is absent |
| `CHALLENGE_EXPIRED`; Binding `failed/challenge_expired` | Challenge `expiresAt`, Gateway Trace, and `GET` Binding | Provider | Correct any delay, create a fresh challenge, serve its exact proof, and complete before its returned expiry | Fresh challenge completes and Binding is `verified` |
| `CHALLENGE_REUSED` | Gateway Trace and `GET` Binding | Provider | Do not repeat the consumed challenge; create a fresh challenge if the Binding still needs verification | Fresh challenge ID completes once |
| `DISALLOWED_NETWORK`; Binding `failed/disallowed_network` | Gateway Trace, Card endpoint, Binding endpoint, and operator allow policy | Provider | Publish a reachable endpoint permitted by platform network policy and create a new Card version/Binding when the bound endpoint changes | New Binding verifies; no allow rule was silently broadened |
| `ENDPOINT_UNAVAILABLE`; Binding `failed/endpoint_unavailable` | Gateway Trace, Binding endpoint, Agent readiness, DNS/TLS/network from Control Plane | Provider | Restore the declared verification route or version the endpoint, then create a fresh challenge | Fresh challenge completes; readiness is healthy |
| `AGENT_RELEASE_UNPUBLISHED` during install | Exact Release read and Installation request | Provider | Complete verification and publish the intended Release; do not install a draft/pending Release | Release is `published`; a new install returns its exact Release ID |
| `INSTALLATION_DISABLED` before Invocation acceptance | Installation read/history and Gateway Trace | Workspace owner | Explicitly set that Installation to `enabled` after confirming permissions and Release | Installation reads `enabled`; a fresh invocation is accepted |
| `AGENT_RELEASE_SUSPENDED` before Invocation acceptance | Installation pin, Release read, and Gateway Trace | Provider | Investigate the suspension and publish a new verified Card version/Release; there is no unsuspend operation | Workspace installs the new published Release and a fresh invocation is accepted |
| `AGENT_RELEASE_REVOKED` before Invocation acceptance | Installation pin, Release read, and Gateway Trace | Provider | Replace it with a new Card version, Binding, challenge, and Release; never restore the revoked Release | New Release is published/installed; revoked history remains queryable |
| Agent 401 `UNAUTHENTICATED` for missing, malformed, forged, expired, unknown-key, or replayed credential | Agent readiness and exact Router/Agent issuer, key ID, key material distribution, TTL, and clocks; never log the token | Platform operator | Correct explicit signing/verifier deployment configuration and synchronized clocks, then restart through the normal deployment procedure | A fresh Gateway invocation succeeds with a new credential; direct request still returns 401 |
| Agent 403 `FORBIDDEN` for wrong audience or context | Exact Agent audience and Router-resolved Release/context configuration | Platform operator | Correct the Agent's exact canonical audience or the managed deployment wiring; do not accept a global/alternate audience | Fresh Gateway invocation succeeds; wrong-audience fixture still returns 403 |
| Direct request without a Router credential returns 401 | Agent response only; there is no Invocation to query | Platform operator | No recovery for the direct caller; use Gateway/Client SDK so the Router creates a managed credential | Managed request succeeds and direct request remains rejected |
| Correlated `AGENT_UNAVAILABLE` after acceptance | Error Invocation/root Task/Trace, Ledger terminal event, linked Release, Agent readiness/network | Provider | Restore the exact published endpoint if the outage is transient, or publish/install a new version when the endpoint changes | Existing Invocation remains `failed`; one fresh caller invocation succeeds |
| Caller cancellation during stream chunk/terminal commit | Invocation/Trace read and ordered Ledger events for the accepted Invocation | Workspace owner/application caller | Confirm the cancellation was intentional; issue one fresh explicit invocation only if the work is still needed | Existing Invocation has exactly one final `canceled` event and never remains `running` or becomes `DEPENDENCY_ERROR` |

The first four Release/Installation errors occur before Router acceptance and
must not be padded with empty Invocation IDs. `AGENT_UNAVAILABLE` occurs after
acceptance and must include Invocation/root Task correlation and one terminal
Ledger failure with the exact Release ID and Card digest.

## Deferred decisions: Needs policy

Do not approximate these items in scripts, fallback branches, or local tables:

| Needs policy | Missing decision |
| --- | --- |
| Verification evidence retention | Duration, deletion authority, audit requirement, and legal/operational retention owner |
| Suspension approval | Who may request/approve suspension and the future organization/RBAC model |
| Signing-key rotation | Multi-key overlap, distribution, activation, retirement, rollback, and incident procedure |
| Cross-replica replay protection | Deployment topology, consistency, availability, storage owner, and failure semantics |
| Automated reconciliation | Which failures are retryable, retry authority/budget, idempotency, visibility, and operator override |

Until those policies are approved by a separate Spec/ADR, the current system
uses one configured signing key, Agent-local in-memory replay detection, manual
state-preserving recovery, and no automatic retry or reconciliation.

## Validate the complete acceptance

The authoritative clean Compose/PostgreSQL run is documented in
[`specs/026-trusted-publication-acceptance/quickstart.md`](../../specs/026-trusted-publication-acceptance/quickstart.md).
It proves the positive cross-runtime lineage, every failure row above, exact
Release provenance, secret absence, log capture, and volume cleanup in the same
isolated run.
