# Quickstart: Trusted Publication Acceptance and Operations

## Prerequisites

- Go version from `go.mod`
- Docker Engine with Compose v2
- All required Compose environment values explicitly set as documented in
  `docs/runbooks/local-development.md`
- Acceptance challenge TTL and verification timeout set explicitly with the
  timeout shorter than the TTL

## Run the clean acceptance

Use the same environment values as the `backend-acceptance` CI job, including
fresh database credentials, development principals, internal service tokens,
test-only Ed25519 material, explicit byte/deadline limits, and:

```text
NEKIRO_ENDPOINT_CHALLENGE_TTL_SECONDS=10
NEKIRO_ENDPOINT_VERIFICATION_TIMEOUT_MS=3000
NEKIRO_E2E_CONTROL_PLANE_URL=http://127.0.0.1:18080
NEKIRO_E2E_ROUTER_URL=http://127.0.0.1:18081
NEKIRO_E2E_COMPOSE_FILE=<absolute-path>/deploy/compose.yaml
NEKIRO_E2E_COMPOSE_PROJECT=nekiro-acceptance-local
```

Then run:

```text
docker compose --project-name nekiro-acceptance-local --file deploy/compose.yaml down --volumes --remove-orphans
docker compose --project-name nekiro-acceptance-local --file deploy/compose.yaml up --build --detach --wait --wait-timeout 120
go test -tags=e2e -count=1 ./tests/e2e/invoke-record
docker compose --project-name nekiro-acceptance-local --file deploy/compose.yaml logs --no-color
docker compose --project-name nekiro-acceptance-local --file deploy/compose.yaml down --volumes --remove-orphans
```

Expected result:

- positive Register -> Verify -> Publish -> Discover -> Install -> Invoke ->
  Record and Runtime A -> Runtime B nested flow pass;
- every row in [acceptance-matrix.md](contracts/acceptance-matrix.md) returns
  its exact outcome;
- accepted Invocations expose matching Release provenance and link to
  `http_well_known` Releases;
- metadata, persistence, errors, and logs contain no scanned proof, token,
  signed credential, key material, or fixture content.
- caller cancellation reaches one durable `canceled` terminal even when it
  races with stream metadata or terminal persistence.

## Operational walkthrough

Follow `docs/runbooks/trusted-publication-operations.md` for publication,
suspension, revocation, investigation, and recovery. Do not use SQL to change
domain state and do not rerun a failed invocation automatically.

## Verification record

Local verification on 2026-07-24 used the isolated Compose project
`nekiro-acceptance-spec026` and completed:

- clean `go test -tags=e2e -count=1 ./tests/e2e/invoke-record` against newly
  created volumes: PASS in 28.351s, including exact no-Ledger reads for all
  direct/credential rejection cases and five real cancellation probes;
- `go test ./...`, `go vet ./...`, and `golangci-lint run`: PASS;
- Linux/WSL `go test -race ./...`: PASS;
- Runtime A `go test ./...`, `go vet ./...`, and Linux/WSL
  `go test -race ./...`: PASS;
- `docker compose ... config --quiet`, GitHub Actions `actionlint`, and
  `git diff --check`: PASS;
- independent Spec and standards re-review: zero High/Medium findings;
- `speckit-converge`: 18/18 FR, 8/8 SC, 12/12 acceptance scenarios,
  7/7 plan decisions, and 8/8 constitution principles satisfied with no
  appended task.

The Windows shell had `CGO_ENABLED=0` and no C compiler, so the authoritative
race gate was run in the same Linux environment class used by CI; no compiler
or race fallback was added.
