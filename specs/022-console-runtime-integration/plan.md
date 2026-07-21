# Implementation plan

1. Add strict Control Plane CORS configuration and middleware tests.
2. Finish the three visual demos without importing demo data into production
   runtime paths.
3. Replace stale Console v3 Invocation/Ledger placeholders with handwritten
   v4/v0.3/v2 mappings and live Owner-only controls.
4. Remove unsupported client fallbacks and unused AI Studio scaffolding.
5. Add focused API and UI tests, update Console documentation, and run full
   backend/Console verification.

## Boundaries

- Gateway owns browser CORS and public API responses.
- Router and Ledger remain backend-owned; Console only consumes northbound
  metadata.
- `contracts/` remains the language-neutral source of truth; TypeScript types
  are strict handwritten mappings for this integration slice.
