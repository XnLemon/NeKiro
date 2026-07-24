# Clarification Record: Workspace Client SDK

**Date**: 2026-07-24

No critical ambiguity requires additional user input. Issue #51, parent Issue
#47, the existing Owner-only decision, Spec 023, the repository constitution,
and the active Northbound Invocation contract already determine the feature
boundary.

## Resolved decisions

- The Phase 1 application credential is an opaque out-of-band Bearer mapped by
  Gateway to an existing Workspace Owner principal. This feature neither
  creates a credential lifecycle nor introduces a second authorization model.
- One Client instance is bound to one Gateway origin and one Workspace. The
  public per-call request contains only Agent identity, capability, and input.
- Gateway remains the only northbound destination and owns invocation, root
  Task, and Trace generation. The SDK returns and validates platform correlation
  but does not accept caller-generated platform identifiers.
- Non-streaming and streaming calls use the existing active invocation
  contracts. There is no alternate endpoint, version, Router, polling, retry,
  redirect, or compatibility fallback.
- The Client SDK is application-facing and remains separate from the Agent SDK;
  it accepts neither trusted nested-invocation context nor Agent-to-Router
  bindings.
- Required byte limits are explicit application policy. Missing or invalid
  limits fail configuration rather than selecting SDK defaults.
- Gateway-created Trace is the sole northbound Trace fact. A missing,
  duplicate, or different Router response Trace is an internal contract failure,
  not a value Gateway may forward or use as a replacement.
- `INTERNAL_ERROR` is HTTP 500 at Router Internal v4 and Northbound Invocation
  v4. It is not an unknown-code path that may fall through to dependency
  unavailable.
- Credential management, delegated roles, service accounts, OAuth/OIDC,
  language variants, and aggregate load policy remain explicit non-goals.

## Coverage status

| Category | Status | Evidence |
| --- | --- | --- |
| Functional scope and user roles | Clear | Three prioritized user stories and Owner-only FR-001 through FR-006 |
| Domain and identity | Clear | Client configuration, opaque credential, request/result/stream/error entities |
| Interaction flow | Clear | JSON, streaming, cancellation, terminal, and typed-error scenarios |
| Security and privacy | Clear | Gateway authorization, credential secrecy, forbidden routing inputs, no raw errors |
| Reliability and limits | Clear | Explicit limits, interruption semantics, no retry/redirect/fallback |
| Integration and compatibility | Clear | Active Gateway invocation contracts and language-neutral mappings only |
| Observability | Clear | Platform Trace and optional invocation/root Task correlation are exposed safely |
| Completion signals | Clear | SC-001 through SC-007 and FR-019 through FR-021 acceptance evidence |
| Deferred policy | Clear | Credential lifecycle, roles, other languages, scale policy, and Runtime behavior are non-goals |

## Checklist revalidation

The specification quality checklist remains 16/16 passing. No checklist item
changed state and no clarification marker remains.
