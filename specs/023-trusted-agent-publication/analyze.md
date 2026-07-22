# Analyze: Trusted Agent Publication

## Consistency result

**PASS for Slice B planning; implementation is scoped to T009–T011.**

- Constitution: provider/release verification is a Control Plane prerequisite
  for the Phase 1 loop and does not introduce runtime-framework behavior.
- Spec → Plan: release binding, state transitions, typed failures, secret
  safety, compatibility, and exact Workspace pins are represented in both
  artifacts.
- Plan → Tasks: T009–T011 cover the release contract, Registry state machine,
  Workspace gate, migrations, and lifecycle tests; Router credentials and the
  Client SDK remain explicitly deferred.
- Contracts → ownership: Registry owns provider/binding/release facts;
  Workspace owns installation pins; Gateway exposes only versioned operations.
- Fallback policy: no default release, implicit verification, legacy upgrade,
  retry, or dependency-to-business-state conversion is introduced.

## Slice B implementation gate

- `AgentRelease` is a separate Registry fact with copied immutable Card,
  endpoint, provider, binding, and evidence fields; no existing Card payload is
  rewritten.
- Release transitions are row-locked and typed. A transition never changes a
  bound fact, and a revoked release is terminal.
- Workspace copies `installedReleaseId` for trusted rows and rejects every
  non-published/non-verified release state. The optional field is limited to
  explicitly marked pre-v4 rows and is not treated as trust evidence.
- Catalog and Workspace migration readiness checks are updated with exact
  columns, FK, state/timestamp, digest, and index assertions before serving.

## Risks carried into implementation review

1. A binding can be revoked or a provider suspended after release creation;
   every verification and publish transition must re-read current state.
2. Release state and the legacy publication projection must commit together so
   discovery cannot expose a pending or revoked trusted release.
3. Installation creation and exact resolution must copy the same release ID;
   a Card version alone is not sufficient trust evidence.
4. Existing pre-v4 published rows must remain readable without being
   silently upgraded to verified.

## Fallback audit

Fallback delta: removed 0, retained 0, added 0, net 0.
Added fallback evidence: none.
