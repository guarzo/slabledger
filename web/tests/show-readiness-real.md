# Show preparation: real cached-only fixture

`TestShowReadinessRealBrowser` in `cmd/slabledger/showprep_readiness_e2e_test.go`
starts the real Go router, LocalAPIToken authentication, inventory/show services,
PostgreSQL stores and a blocked counting CardLadder endpoint. It serves the production
frontend directly. No Vite, port 4173, production API or shared browser is involved.
`web/tests/show-readiness-real.cjs` launches its own Chromium and drives real endpoints.

**Qualified snapshots are seeded explicitly as a PRECONDITION. This proves cached
use, not worker population.** The old browser-owned cold warm-up scenario is retired.
Worker-owned population requires a separate Task 4 acceptance test.

## Safety and prerequisites

Run only from a feature worktree. This is an opt-in destructive fixture. The only
accepted URL is:

```
postgres://showprep:showprep_test@127.0.0.1:44620/showprep_readiness_e2e?sslmode=disable
```

The Task 1 parent-owned container is `slabledger-cached-show-01a09dd6`, full ID
`d56b5500410d90a5609c98656c2a33b7c9364135f588d3013bbfc34f0b3d496b`.
Verify its exact ID and published loopback port before resetting the browser DB.
The separate `showprep_cached_test` storage DB is reserved for Task 2: do not reset it.
Never substitute DATABASE_URL, a default/developer database, or production. Never
use `make screenshots` or the default `make test-postgres` target for this fixture.

Requires Go 1.26, installed web dependencies from this worktree's lockfile and
matching Playwright Chromium (`cd web && npm ci && npx playwright install chromium`).
Each run resets only the browser database's public schema, seeds historical ledger
and legacy comps at migration 45, upgrades, proves the verified store initially
empty, then publishes qualified snapshots through the real store. It creates 155
unsold inventory cards plus one historical sold card. Current, missing, old, failed,
unresolved, no-price, zero-sale, thin and below-target cases are deliberate fixtures.
No provider is used for this precondition.

## Repeatable cached-use acceptance

From the worktree root:

```bash
cd web && npm run build && cd ..
TZ=UTC DATABASE_URL= POSTGRES_TEST_URL= \
SHOW_READINESS_E2E_URL='postgres://showprep:showprep_test@127.0.0.1:44620/showprep_readiness_e2e?sslmode=disable' \
SHOW_READINESS_MODE=cached SHOW_READINESS_ARTIFACTS=/tmp/show-cached-browser \
go test -race -v -count=1 -timeout 6m ./cmd/slabledger -run TestShowReadinessRealBrowser
```

Without the e2e URL ordinary Go discovery skips the test. Cached mode is mandatory;
the old acquisition scenario cannot silently count as passing. Broad Go discovery
and browser artifact generation must be sequential. Leave POSTGRES_TEST_URL unset
for broad discovery; this browser proof does not authorize another DB reset.

Assertions cover:

- Real authenticated JSON410 on the retired route, with evidence/list/item/hold
  rows unchanged. Missing/wrong authentication remains rejected.
- Inventory → Supported → existing checkbox → stored evidence → create list →
  add → pack; then reload/restart and add another card to the existing list.
- Provider requests exactly zero, browser acquisition POSTs exactly zero, and
  checkbox/select-all/clear/Support filter request logs separately empty.
- In-browser 155-card filter/select/clear timings under 100 ms, excluding driver latency.
- Genuine server evaluator outcomes, retained dated sales, distinct availability
  labels, and separate DH listing $300 / reviewed price $400 / median $280 context.
- UTC rollover and read-only focus update: captured selection retained, stale
  Add/Pack commands conflict, previous packing history unchanged. Weak/stale evidence
  can still be manually packed with the newly observed version.
- Whole persisted campaigns, purchases (all prices and DH fields), sales and legacy
  comp rows byte-for-byte unchanged. No financial HTTP writes.
- Desktop/mobile/tablet geometry, fine/coarse pointers, repeated evidence expansion,
  stable virtual rows, last-row clearance, keyboard packing and destination focus return.

No application API response is intercepted. All external browser requests are blocked
except the existing public Google font GETs, fetched without credentials or redirects.
The local source counts even malformed/rejected requests. The source stays blocked
throughout use. App/auth/session and full-body evaluation cancellation remain covered
by their focused tests.

Artifacts include full-page/viewport PNGs, `metrics.json`, `wire-snapshots.json`
(precondition/final persisted rows, request paths, checkbox and acquisition logs),
and `fixture.json` (ephemeral loopback addresses and fixture-only auth token).
Servers and the owned browser close after completion or failure. PG stays running
for the parent; the browser DB is left inspectable. No push, deploy or production
readiness claim is implied.

## Fixture safety regressions

No DB is needed for these:

```bash
node --test web/tests/show-readiness-browser-checks.cjs
TZ=UTC DATABASE_URL= POSTGRES_TEST_URL= SHOW_READINESS_E2E_URL= \
go test -race -count=1 ./cmd/slabledger -run 'TestReadiness(Restart|ConcurrentRestart|FixtureClock|Source)'
```

The actual control-state error regression resets only the same pinned browser DB;
run sequentially, not alongside browser acceptance:

```bash
TZ=UTC DATABASE_URL= POSTGRES_TEST_URL= \
SHOW_READINESS_E2E_URL='postgres://showprep:showprep_test@127.0.0.1:44620/showprep_readiness_e2e?sslmode=disable' \
go test -race -count=1 -timeout 1m ./cmd/slabledger -run TestReadinessControlErrors
```

It preserves independent HTTP error reporting, ledger invariants, restart lock
release, continued requests and cleanup. It does not prove evidence acquisition.
