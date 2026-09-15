# Show readiness: real-wire upgrade fixture

`TestShowReadinessRealBrowser` in `cmd/slabledger/showprep_readiness_e2e_test.go`
starts the real Go router, LocalAPIToken authentication, inventory/show services,
PostgreSQL stores and CardLadder HTTP adapter. It serves the production frontend
build directly; no Vite process or port 4173 is involved. The companion
`web/tests/show-readiness-real.cjs` drives Chromium through those real endpoints.

## Safety and prerequisites

Run only from a feature worktree. This is an opt-in destructive fixture, **not** a
production verification command. It accepts exactly this URL and rejects all
others before connecting:

```
postgres://showprep:showprep_test@127.0.0.1:44620/showprep_readiness_e2e?sslmode=disable
```

The owned disposable PostgreSQL 17 container is
`slabledger-show-readiness-01a09dd6`. Provision its `showprep_readiness_e2e` database
before running. The separately owned `showprep_readiness_test` database is for the
destructive storage-adapter suite only. Never substitute DATABASE_URL, a developer
ledger, or production. Never use `make screenshots` (production DB pull) or the
default `make test-postgres` target for this fixture.

Requires the repository's Go 1.26, Node, installed web dependencies and matching
Playwright Chromium. No new application dependencies are needed. The test applies
migration 45, seeds purchases/a historical sale/legacy CL comps, then applies all
remaining migrations and proves `showprep_evidence` is empty. Every run resets the
e2e database's public schema. Do not run another test or preview against it at the
same time. The fixture leaves its ledger for inspection; the owner cleans up PG.

## Repeatable regression

From the worktree root:

```bash
cd web && npm run build && cd ..
TZ=UTC DATABASE_URL= POSTGRES_TEST_URL= \
SHOW_READINESS_E2E_URL='postgres://showprep:showprep_test@127.0.0.1:44620/showprep_readiness_e2e?sslmode=disable' \
SHOW_READINESS_ARTIFACTS=/tmp/show-readiness-browser \
go test -race -v -count=1 -timeout 4m ./cmd/slabledger -run TestShowReadinessRealBrowser
```

Without `SHOW_READINESS_E2E_URL`, ordinary Go discovery skips this test. Run broad
Go discovery and browser artifact generation sequentially. Explicitly unset the
e2e variable for the full Go suite and for the separate PostgreSQL adapter suite.

Harness failure-path checks need no PostgreSQL or running fixture:

```bash
node --test web/tests/show-readiness-browser-checks.cjs
TZ=UTC DATABASE_URL= POSTGRES_TEST_URL= SHOW_READINESS_E2E_URL= \
go test -race -count=1 ./cmd/slabledger -run 'TestReadinessRestartFailureReleasesRequests|TestReadinessFixtureClockAdvances'
```

These use real Chromium/failed artifact writes and a separate loopback HTTP server
to prove original-error preservation, independent diagnostics, browser closure,
failed-restart lock release, continued requests/shutdown, and whole-row clearance.
They do not reset or access the preview database. The Node checks are deliberately
outside Vitest discovery and run explicitly with Node's test runner.

The test asserts:

- Missing/wrong auth is rejected by actual middleware. Ordinary inventory,
  evidence and empty-list reads make no source calls.
- Cold Supported-first, before selection or list creation, reaches the local
  source over HTTP. 24 scoped purchases coalesce to 12 identities in 10+2 refresh
  batches, then render genuine persisted Supported results. Out-of-scope and
  missing-price identities are not acquired automatically.
- Reload and a new router/auth/inventory/show service, client, store and SQL pool
  preserve current evidence without reacquisition.
- Next UTC date makes old evidence stale/non-Supported on real reads. Selection
  remains checked and non-addable until explicit review; clearing it permits
  renewal of the correct window. Packed membership is unchanged.
- A controlled source HTTP 401 and an incomplete page produce failed/partial
  attempts, retain the verified sales and never autonomously retry on focus,
  reload or clearing selection. Explicit Retry recovers.
- Full rows in campaigns, purchases (all prices and DH fields), sales and legacy
  comps remain byte-for-byte equal. Only the three explicit create/add/pack
  requests change lists/items. The packed row remains identical through renewal
  and failed checks; no financial HTTP writes occur.
- Actual SPA links navigate inventory → Shows → inventory without reloading the
  QueryClient. The App/provider regression suite separately exercises budget,
  Cancel, pending dialog writes, identity changes and abandoned auth responses.
- Reviewed price $400, evaluated DH listing $300, median $280: compact DH price
  context remains visible on desktop/mobile without replacing CL/Market valuation.
- Adjacent virtual-row bounds after three repeated evidence open/close cycles,
  middle/end/start scrolls, desktop → mobile → desktop resizing, and an actual
  fine → coarse pointer transition. CDP pointer:none is not called fine; both
  media-query values and coarse-trigger 44px heights are asserted.
- Desktop/tablet/mobile rendered states, keyboard pack/Escape/focus return,
  progressive destination, inline evidence, virtualized final-row clearance and
  horizontal overflow. Existing unit/stream tests retain budget, late-body,
  retryAt, hidden-selection and 805-card ordering coverage beyond this flow.

Only the external CardLadder HTTP response and service/browser clock are controlled;
logger output uses the existing test logger. OAuth token exchange is never called:
LocalAPIToken uses the real auth service/repository. No evaluate, refresh, inventory,
list or packing response is intercepted. External requests are blocked except the
app's existing public Google Font GETs, fetched without credentials or redirects.

The source retains its real completion wall clock and existing 1/sec limiter.
The test service/source-fixture clock advances with real elapsed time, then shifts
to the next UTC midnight on explicit rollover, keeping genuine refresh timestamps
within 24 hours. The browser receives each fixture clock value. To avoid an
uncontrolled real midnight during cold/reload assertions, a regression launched
in the final two UTC minutes waits for the next day before seeding (still within
the four-minute timeout). Interactive mode does not wait and its clock keeps
advancing, so it remains usable beyond a brief screenshot session.
The definitive failure case uses HTTP 401; transient 503s have existing internal
CardLadder/httpx retries. Do not confuse those with browser refresh-POST replay.

Artifacts: full-page and viewport PNGs (geometry/pointer states use viewport-only
CDP captures so clipped/full-page capture cannot reset live pointer emulation),
`metrics.json`, `wire-snapshots.json`
(persisted rows, request paths, refresh bodies), and `fixture.json` (ephemeral
server addresses and fixture-only auth token). Diagnostic failures cannot replace
a primary exercise failure or prevent browser closure; later diagnostics are still
attempted. A failed restart returns HTTP 500 without retaining the application lock,
so another request/restart and shutdown can complete. If reopening failed after the
old pool closed, data reads may return errors until a successful restart.
All test servers and browsers close on normal completion. This is local integration evidence, not production rollout.

## Separate fixture server for two-tab critique

Run **after** the count-sensitive regression finishes. This mode resets the same
e2e database but runs no browser assertions. Build a test binary so Ctrl-C reaches
its cleanup handler directly:

```bash
# From the worktree root; build the web frontend first as above.
go test -c -o /tmp/slabledger-show-readiness.test ./cmd/slabledger
cd cmd/slabledger
TZ=UTC DATABASE_URL= POSTGRES_TEST_URL= \
SHOW_READINESS_E2E_URL='postgres://showprep:showprep_test@127.0.0.1:44620/showprep_readiness_e2e?sslmode=disable' \
SHOW_READINESS_SERVE=1 SHOW_READINESS_ARTIFACTS=/tmp/show-readiness-preview \
/tmp/slabledger-show-readiness.test -test.v -test.run TestShowReadinessRealBrowser -test.timeout 30m
```

Read `/tmp/show-readiness-preview/fixture.json`; every launch chooses free loopback
API/source/control ports. Open its `app` + `/inventory` or `/shows` in **two new
independent tabs**, each sending:

```
Authorization: Bearer show-readiness-local-fixture
```

Use Playwright `browser.newContext({ extraHTTPHeaders: { Authorization: ... } })`
(or CDP Network.setExtraHTTPHeaders on each new tab). Do not use production
browser credentials or fetch the user's existing tabs. Block external requests;
if fonts are needed, forward only public font GETs without the fixture header as
the committed browser script does. Set each tab's fixed clock to `fixture.now`
when inspecting rollover. Label the tabs `[LLM]` and `[Human]` for critique.

Only the separate **test-only control server** accepts these authenticated calls:

| Method/path | Effect |
|---|---|
| `GET /state` | Source/request log, clock and actual persisted evidence/list rows; checks ledger equality. |
| `POST /source?mode=hold` | Hold the next source response so Checking can be inspected. |
| `POST /source?mode=complete` | Release a held response; subsequent pages are complete. |
| `POST /source?mode=failed` | Subsequent source requests return a definitive HTTP 401. |
| `POST /source?mode=partial` | Return two records but totalHits=3; the real adapter rejects coverage. |
| `POST /rollover` | Advance service/source-fixture date to next UTC midnight; elapsed time continues. |
| `POST /restart` | Drain requests and rebuild all app instances without reseeding. |

Release held work before restarting. After rollover update browser time and focus
or reactivate show mode to observe readback. These controls cannot be compiled into
the production executable. Ctrl-C/SIGTERM stops API/source/control servers; PG stays
running for its owner. No deploy, push, merge or production verification is implied.
