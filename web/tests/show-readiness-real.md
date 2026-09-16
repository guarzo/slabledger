# Show preparation: real worker and cached-use proof

`TestShowReadinessRealBrowser` uses the real Go router, LocalAPIToken auth service
and PostgreSQL auth repository, inventory/show services, worker scheduler and
production frontend. Its owned Chromium is launched by
`web/tests/show-readiness-real.cjs`. No application response is intercepted.
Only external CardLadder/Firebase HTTP and the test business clock are controlled.
No Vite, port 4173, production API, static-token source bypass or shared CDP.

## Safety and prerequisites

Feature worktree only. The fixture **resets public schema** on this exact URL:

```
postgres://showprep:showprep_test@127.0.0.1:44620/showprep_readiness_e2e?sslmode=disable
```

Verify the owner's resource before running:

```bash
docker inspect slabledger-cached-show-01a09dd6 --format '{{.Id}} {{.State.Running}} {{json .NetworkSettings.Ports}}'
docker exec slabledger-cached-show-01a09dd6 psql -U showprep -d showprep_readiness_e2e \
  -c 'SELECT current_database(),current_user,pg_get_userbyid(datdba) FROM pg_database WHERE datname=current_database();'
```

Expected container ID:
`d56b5500410d90a5609c98656c2a33b7c9364135f588d3013bbfc34f0b3d496b`, running,
5432 published only at `127.0.0.1:44620`; database/user/owner are
`showprep_readiness_e2e`/`showprep`/`showprep`. The harness also verifies the exact
URL and database/user/owner before reset. Retain this parent-owned container.

Use installed Go 1.26 and worktree-lockfile web dependencies with matching
Playwright Chromium. Build first: `cd web && npm run build && cd ..`.
Do not substitute a default/developer/production DB or run `make screenshots`.
Use three separate databases: `showprep_cached_test` for storage tests,
`showprep_runtime_test` for cmd production-runtime tests, and
`showprep_readiness_e2e` for browser fixtures. Storage resets schema while Go runs
packages in parallel. Runtime tests require **`SHOW_PREP_RUNTIME_TEST_URL`** and
skip when it is unset, even if `POSTGRES_TEST_URL` is set. No application/default
DB fallback is permitted. The runtime guard accepts only an explicit loopback
PostgreSQL URI/port, database `showprep_runtime_test`, user `showprep` or
`slabledger`, and `sslmode=disable` without connection overrides; the connected
user must own the database before migrations/truncation.

After verifying the exact container above, create the dedicated runtime DB once
(if absent) and verify ownership:

```bash
docker exec slabledger-cached-show-01a09dd6 createdb -U showprep -O showprep showprep_runtime_test
docker exec slabledger-cached-show-01a09dd6 psql -U showprep -d showprep_runtime_test \
  -c 'SELECT current_database(),current_user,pg_get_userbyid(datdba) FROM pg_database WHERE datname=current_database();'
```

CI creates the same dedicated runtime database owned by its `slabledger` role
before supplying both independent opt-ins to `go test ./...`. This local command
exercises the same **parallel-package isolation**, not a claim that remote CI ran:

```bash
unset DATABASE_URL POSTGRES_TEST_DSN POSTGRES_ADMIN_URL LOCAL_DB_URL SUPABASE_URL SHOW_READINESS_E2E_URL
POSTGRES_TEST_URL='postgres://showprep:showprep_test@127.0.0.1:44620/showprep_cached_test?sslmode=disable' \
SHOW_PREP_RUNTIME_TEST_URL='postgres://showprep:showprep_test@127.0.0.1:44620/showprep_runtime_test?sslmode=disable' \
TZ=UTC go test -race -count=1 -timeout 10m ./...
```

Keep this broad discovery separate from browser artifact generation. Archive old
artifact directories instead of overwriting them.

## Three explicitly separated phases

| Mode/phase | Evidence precondition | Provider counts |
|---|---|---|
| `cached` | Explicit store-seeded qualified/old/failed snapshots | Absolute zero SDK and source requests throughout |
| `worker`, A | Legacy ledger/comps at migration45, upgrade47 with empty verified store; actual `initializeSchedulers` → configured source provider → Group → EvidenceWorker → PG → source adapter | 142 sales requests + one Firebase token exchange, **before browser launch** |
| Both modes, B | Worker cancelled/stopped/joined; provider blocked; new disabled production composition for cached reads | Zero **additional** requests; full population history retained; restart stays disabled |
| `worker`, C | After B is archived, separately enable the actual runtime and hold one failed-identity repair | Exactly one additional sales request + one token exchange; never attributed to zero-source B |

There are 155 unsold cards and one historical sold purchase. Twelve pairs share
exact identities; one card remains unresolved. Worker A asserts the entire exact
142-identity source cohort and coverage: **141 current / 1 deliberate incomplete /
0 missing / 0 stale identities; 153 current / 155 eligible cards; 1 unresolved**.
The partial response for card29 is deliberately inspectable but never current.
Positive $270/$290 sales produce a $280 median against a saved $300 DH price;
legacy $999 comps are not certified. Complete zero, one-sale Thin evidence,
Below target and no-DH-price cards are separately checked. The seeded-cache mode
additionally supplies old successful evidence and retained sales after failure;
these seeded results are not a claim of worker ingestion.

B drives Admin status, inventory → Supported → checkbox → stored evidence →
create/list/add/pack → reload → rebuilt SQL pool/auth/router/services → add to an
existing list → UTC stale selection and explicit Add/Pack409. It preserves whole
campaign/purchase/sale/legacy-comp rows and packing history. Checkbox/filter/select
all/clear request logs are independently empty; browser refresh POSTs are zero.
Unauthenticated/authorized explicit401/410 retirement probes are logged separately.

C repairs only card29 through the real admin retry endpoint and runtime, while it
is selected under Needs review. A real financial form opens and cancels while the
source is held. Publication changes its status to Supported but retains selected
ID/version and row position. Explicit stale Add/Pack return409; no silent rebinding,
financial write, or packing-history change. This phase returns the test clock from
B's synthetic next-day stale observation to real wall time before starting runtime;
there is no production clock or reset API.

## Repeatable commands

Run each separately from the worktree root with different artifact directories:

```bash
unset DATABASE_URL POSTGRES_TEST_URL SHOW_PREP_RUNTIME_TEST_URL POSTGRES_TEST_DSN POSTGRES_ADMIN_URL LOCAL_DB_URL SUPABASE_URL
SHOW_READINESS_E2E_URL='postgres://showprep:showprep_test@127.0.0.1:44620/showprep_readiness_e2e?sslmode=disable' \
SHOW_READINESS_MODE=cached SHOW_READINESS_ARTIFACTS=/tmp/showprep-cached \
TZ=UTC go test -race -v -count=1 -timeout 10m ./cmd/slabledger -run TestShowReadinessRealBrowser

SHOW_READINESS_E2E_URL='postgres://showprep:showprep_test@127.0.0.1:44620/showprep_readiness_e2e?sslmode=disable' \
SHOW_READINESS_MODE=worker SHOW_READINESS_ARTIFACTS=/tmp/showprep-worker \
TZ=UTC go test -race -v -count=1 -timeout 10m ./cmd/slabledger -run TestShowReadinessRealBrowser
```

Worker population intentionally retains the production one-request/second pace,
so allow roughly three minutes plus browser work. A rare launch within four minutes
of UTC midnight waits for the next day to keep cohort-count assertions deterministic.
Ordinary discovery skips this test without the opt-in e2e URL. Mode is mandatory.

For the cold negative control, add `SHOW_READINESS_COLD_PROBE=1` to worker mode.
It disables only runtime execution and **must fail** the whole-cohort assertion
before any browser process is started. No old production source is mutated.

The small actual-source renewal and active financial-write proof uses the other DB:

```bash
unset SHOW_READINESS_E2E_URL DATABASE_URL POSTGRES_TEST_URL
SHOW_PREP_RUNTIME_TEST_URL='postgres://showprep:showprep_test@127.0.0.1:44620/showprep_runtime_test?sslmode=disable' \
SHOW_READINESS_RUNTIME_ARTIFACTS=/tmp/showprep-runtime TZ=UTC \
go test -race -v -count=1 -timeout 2m ./cmd/slabledger -run TestShowPrepRuntimePositiveMidnightAndFinancialWrite
```

It starts actual production composition, proves positive and complete-zero results,
restarts without acquisition, then uses the existing domain clock seam through a
real scheduler Group with the configured production source provider. It advances to
**next UTC midnight, less than24h**, not blindly now+24h: the source adapter stamps
real wall-time `RefreshedAt`. A real authenticated price-override PATCH completes
while source HTTP is held. That intentional 32500-cent write has its own before/
after financial baselines; renewal then makes zero further financial changes.
Do not confuse this explicit mutation with B's whole-flow immutability.

Existing freshly runnable runtime/PG seams cover new and resolved inventory without
a browser, first-save/cross-instance activation, durable token400/source401 auth
holds, explicit recovery, retry exhaustion/restart, two connections, lease loss,
late publication, scope/availability safeguards and financial locks. They are part
of the full dedicated cmd and storage race suites, not duplicated here.

## Artifacts and browser checks

- `upgrade-legacy-45.json`, `upgrade-empty-47.json`: actual before/after ledger and
  initially empty verified store. Upgrade asserts every financial row unchanged.
- Worker mode `worker-before.json`, `worker-populated.json`: exact source query
  history, evidence, durable worker state, coverage and full ledger.
- `cached-operator-complete.json`: B's immutable ledger and unchanged acquisition
  history, before any C acquisition is allowed.
- `wire-snapshots.json`: persisted snapshots/history, browser and explicit request
  paths/bodies/statuses, empty checkbox/filter and browser-acquisition logs; C is
  a separate named snapshot.
- `operator-final.json`: final persisted ledger/evidence/coverage; in worker mode
  it includes the explicitly separate repair phase.
- `metrics.json`: desktop/mobile/tablet geometry, repeated expansion/collapse,
  stable virtual rows, last-row clearance, keyboard packing/destination focus,
  fine/coarse pointer geometry and rendered webfont checks.
- Actual full-page/viewport PNGs, including Admin, selected evidence, mobile
  destinations and concurrent publication. Public Google font GETs are the only
  external browser allowlist, fetched without credentials or redirects. No overlay.
- Event-to-rendered-frame 155-card filter/select/clear timings are local samples
  against a <100ms goal, not a flaky CI wall-clock gate or production percentile.
- `fixture.json` contains ephemeral loopback URLs and fixture-only local token.

Cleanup is owned: servers, Groups, SQL pools and the launched Chromium close after
success/failure; the PG container stays running. `SHOW_READINESS_SERVE=1` remains
an optional separate interactive session; stop with SIGINT/SIGTERM. Never attach
to shared Chrome. Test-only controls are compiled only in `_test.go`.

## Focused held-selection DOM geometry

The small fully resolved/priced fixture catches a coverage notice disappearing
at 100% current and the no-search Needs Attention banner disappearing after
publication removes the last support match. The actual React/query/virtual-row
code runs in owned Chromium at desktop/mobile widths. API responses are controlled
**only in this focused layout test**; it is not worker/source/financial proof.
Existing minute polling delivers the coverage update. Initial checkbox geometry,
held publication geometry, live zero counts, unchanged selected versions/disabled
Add and clear/explicit-view reset are asserted. A quiet initial view must not gain
a header row on selection. Checkbox/reset request logs stay empty.

```bash
cd web
SHOW_LAYOUT_ARTIFACTS=/tmp/showprep-layout-fresh \
  npx playwright test --config tests/show-readiness-layout.config.ts
```

The config owns loopback45173 (no server reuse/4173/shared CDP); geometry JSON is
saved per case. Run the two actual backend modes after production layout changes,
with fresh artifact paths, and then the four cleanup checks below.

## Fixture cleanup/error regressions

```bash
node --test web/tests/show-readiness-browser-checks.cjs
unset DATABASE_URL POSTGRES_TEST_URL SHOW_PREP_RUNTIME_TEST_URL SHOW_READINESS_E2E_URL
TZ=UTC go test -race -count=1 ./cmd/slabledger -run 'TestReadiness(Restart|ConcurrentRestart|FixtureClock|Source)'

SHOW_READINESS_E2E_URL='postgres://showprep:showprep_test@127.0.0.1:44620/showprep_readiness_e2e?sslmode=disable' \
TZ=UTC go test -race -count=1 -timeout 1m ./cmd/slabledger -run 'TestReadinessControlErrors|TestReadinessCachedSeedAtUTCMidnight'
```

The last command resets only the pinned browser DB; run separately from acceptance.
Failures retain contextual HTTP500 and independent test reporting; cleanup/lock
release and continued valid requests are exercised. Historical proof artifacts
are not overwritten. No push/deploy/backfill/production-readiness claim is implied.
Worker rollout remains incomplete until separately authorized deployment, initial
server-owned catch-up and actual production verification.
