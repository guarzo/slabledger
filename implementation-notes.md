# Card-show preparation implementation notes

## Scope and references

- Spec: `docs/specs/2026-09-14-show-preparation-design.md`.
- Plan: `docs/plans/2026-09-14-show-preparation.md`.
- Implementation base: `3a32378f`; branch `docs/show-preparation-design` in the linked `show-preparation-design` worktree.
- Backend owns domain, PostgreSQL, CardLadder adapter, HTTP API and application wiring. Frontend owns existing inventory integration and saved `/shows` UI. A fixed JSON contract coordinates the independent slices.

## Source integration

Production CardLadder is working, as confirmed by the operator. The earlier direct
request from the development environment received a Cloudflare browser challenge;
that was not evidence of production failure and is not a release gate.

Authorized read-only production database inspection on 2026-09-14 confirmed that
individual stored comps contain profile ID, grade condition, source sale ID,
sale date, integer-cent amount, platform, listing type and source URL. Recent
records include BestOffer and FixedPrice sales. This feature reuses those
source-reported conventions and the existing configured client/authentication.
No production records, credentials, or refresh state were modified.

Existing 90-day aggregates remain unchanged. New per-card evaluation must retain
honest window/identity/freshness/completeness checks; lack of coverage metadata
must not be confused with an absence of recent sales.

## Review-driven decisions

- Persist known DH price-association ambiguity independently of purchase/campaign
  deletion. No unverified reset in v1; a timestamp or disappearing competitor is
  not proof that a price belongs to the selected slab.
- Define one show-specific availability policy for candidates, totals and packing:
  removed, sold, refunded, closed campaign, not received, then ready. Pending
  campaigns are eligible. Keep saved rows/history after availability changes.
- Reserve time for refresh persistence and response delivery before server write
  deadlines; do not automatically replay failed refresh requests in the browser.
- Test new independent tables with explicit cleanup and uncached PostgreSQL runs.

## Verification environment

- Dedicated disposable PostgreSQL container `slabledger-showprep-test` on local
  port 55439; destructive fixtures target only its `showprep_test` database.
- Baseline `POSTGRES_TEST_URL= go test -race -count=1 -timeout 10m ./...` passed.
  PostgreSQL-backed tests were skipped for that baseline, not claimed as exercised.
- Frontend dependencies installed with `npm ci --no-audit --no-fund` in the worktree.

## Implemented behavior and task review

The backend supplies the shared evaluator, bounded detailed-source refresh,
versioned evidence snapshots, saved lists and atomic packing/acknowledgment API.
The frontend integrates into the existing inventory browser and adds `/shows`.
All source files remain below the repository hard limit, and no dependencies were
added to the application.

Task review found and fixed:

- Selected-but-hidden cards could silently acquire a newly fetched evaluation
  version. Selection now retains the version actually observed; changes require
  explicit review/reselection.
- An older in-flight inventory batch could replace newer detail evidence. Cache
  publication now preserves newer observations, including initially incomplete
  batches; interrupted partial loads refetch on remount instead of appearing fresh.
- A collision discovered during locked validation could disappear with a rejected
  mutation. A transaction savepoint rolls back rejected list writes while restoring
  the captured identity/time records, even if the competitor has already vanished.
- Initial lock ordering inverted existing campaign deletion. New mutations lock
  purchases before their current campaigns and use `NOWAIT`; contention returns
  the existing 409 rather than deadlocking with legacy deletion, including bulk adds.

Both task slices received spec and quality approval after scoped re-review.

## Verification performed by the controller

- `POSTGRES_TEST_URL= go test -race -count=1 -timeout 10m ./...`: PASS.
- Dedicated `POSTGRES_TEST_URL` on the disposable `showprep_test` database,
  `go test -race -count=1 -timeout 10m ./internal/adapters/storage/postgres/...`:
  PASS, uncached (120.611 seconds in the controller run).
- `npm test`: PASS, 71 files / 621 tests.
- `npm run typecheck`, `npm run lint`, `npm run build`: PASS.
- `npx playwright test tests/show-preparation.spec.ts --project=chromium --workers=1`:
  PASS, three mobile/tablet/desktop workflows.
- `PATH="/tmp/slabledger-showprep-tools:$PATH" make check`: PASS, zero lint issues,
  architecture self-tests/import rules, file limits, documentation paths and
  Playwright version alignment. The temporary tool binary is golangci-lint2.11.3,
  matching the repository devcontainer pin, built with Go1.26.5; no global tooling
  configuration was changed.
- Real Go API + separate disposable `showprep_smoke` DB exercise: PASS for auth
  rejection, source-fixture support calculation/details, idempotent create/add,
  packing persistence, stale acknowledgment, changed price/support, refunds,
  deletion retention, sticky ambiguity, unpack/remove and no financial mutations.
- Real Chromium UI -> real Go API -> disposable PostgreSQL exercise: PASS for
  supported filtering, individual sale detail, slab addition, packing, and reload
  persistence. Only OAuth identity was stubbed; inventory/preparation requests
  were forwarded to the actual local API. Test snapshots were explicitly seeded
  locally, not presented as newly fetched marketplace evidence.
- Screenshots inspected at mobile, tablet and desktop widths, plus the real-backend
  packing page. Browser test artifacts remain under ignored `web/test-results/`.

## Final coordinated fix wave after `76d3a2a2`

The three deduplicated final-review findings were reproduced with failing tests
and fixed without reopening the feature design:

- A cancelled evidence-detail request now calls `signal.throwIfAborted()` after
  awaiting the parsed response and before any cache write/invalidation. The
  regression uses the actual APIClient and native fetch against a loopback HTTP
  server that sends headers before releasing the JSON body. It preserves the
  newer below-target/price observation, creates no stale evidence alias, and
  does not invalidate a newer list. The global client remains unchanged.
- Required `evidenceNeedsReview` / `evidenceReason` fields expose evidence health
  independently of DH price. The evaluator runs the existing checks once, then
  applies the unchanged price-support precedence. No price remains
  `no_listed_price` with its original reason; failed/stale/partial/missing evidence
  still drives refresh review counts and detail warnings. Healthy complete
  evidence, including empty windows or ambiguous DH price, is not described as
  failed evidence. Go/TS contracts, strict boundary checks and fixtures agree.
  Health enters the existing evaluation fingerprint; no stored schema changes or
  duplicate refresh-outcomes model were needed. A real PostgreSQL regression
  proves failed rechecks retain readable sales and health after service reload.
- The CardLadder traversal tracks each sale ID's first-seen page. Cross-page
  overlap retains inspectable records but prevents completion, even with stable
  totals and identical dates. The exact 1–100 / 100–199 fixture retains 199 partial
  sales; disjoint 1–100 / 101–200 remains complete. Within-page identical dedup and
  contradictory duplicate rejection remain covered.

Final-wave verification (separate from the earlier controller checks):

- `POSTGRES_TEST_URL= go test -race -count=1 -timeout 10m ./...`: PASS; DB fixtures
  skipped in this command and exercised separately below.
- Explicit authorized disposable `showprep_test` URL on 127.0.0.1:55439,
  `go test -race -count=1 -timeout 10m ./internal/adapters/storage/postgres/...`:
  PASS, uncached, 134.038 seconds. No fixture use of the separate smoke database.
- `npm test`: PASS, 72 files / 633 tests. `npm run typecheck`, `npm run lint`,
  `npm run build`: PASS.
- `CI= npx playwright test tests/show-preparation.spec.ts --project=chromium --workers=1`:
  PASS, five workflows including failed/healthy no-price evidence and the prior
  mobile/tablet/desktop selection/packing/retention coverage. Requests intercepted;
  test-owned Vite used port 5173, not the controller's preview or API.
- `make check` with `/tmp/slabledger-showprep-tools` and the installed Go/Node
  directories prefixed in PATH: PASS, zero lint issues. `git diff --check`: PASS.
- Scoped local `polish-core --fix` review: no unresolved requested findings;
  fixture-health consistency and existing loading waits updated. No subagents.
- Intermediate verification failures were resolved: two prior loading tests were
  waiting on the old healthy-empty text, and the first concurrent `make check`
  encountered Playwright deleting `web/test-results` during Go package discovery.
  The unchanged checks passed after correcting the waits and avoiding that race.

Full paths, red/green commands, browser artifact paths and handoff details are in
`.superpowers/sdd/2026-09-14-show-preparation/final-fix-report.md` (local report).
The fixing agent made no staging/commits, production work, credential changes,
dependency changes, or controller smoke-server/preview interactions.

## Final controller verification and review

After the final fix, the controller reran the root uncached Go race suite and the
explicit disposable PostgreSQL race suite (117.279 seconds), all 633 frontend tests,
typecheck, lint, build, five Chromium workflows, and `make check` (zero lint issues).
These commands ran sequentially to avoid generated-browser-artifact deletion racing
Go package discovery. Both working-tree and staged diff checks passed.

The rebuilt real local Go API passed the lifecycle/identity/price smoke again.
The real browser/API/PostgreSQL workflow also verified a failed refresh after the
DH price disappears: `No listed price` remained, progress counted one review issue,
and the actual failure reason plus retained sale rows stayed visible. Only local
OAuth identity was stubbed; the local client intentionally had no source credentials
for that failure test. No production state was changed.

Final independent whole-range review found the three issues above; one scoped
re-review verified each fix and returned **SHIP**, with no new Important/Critical
breakage. The final implementation is ready for integration, not deployed.

## Recorded implementation decisions

- Separate backend/frontend ownership allowed parallel implementation; the fixed
  wire contract and real integration exercise controlled interface-drift risk.
- Reuse the working production CardLadder integration without a successful local
  direct call as a gate; actual per-card source failures remain visible/reviewable.
- Align purchase-before-campaign locks with existing deletion and use `NOWAIT`;
  the tradeoff is an explicit 409/retry under contention rather than a deadlock.
- Document one restart after first-time runtime source configuration instead of
  adding a separate hot-reload mechanism; already-configured deployments are unaffected.
- Add independent evidence-health fields rather than duplicate refresh outcomes;
  every Go/TS producer, boundary check, fixture and consumer was updated together.

## User-approved CodeRabbit follow-up

CodeRabbit reviewed the committed feature through `d0fccf47` against `fe8ad267`:
17 raw findings reduced to 12 distinct items. Read-only validation established
four fixes, which the user approved before push/PR:

- **Later-batch cache ordering:** removed aggregate-wide reference ordering. A
  changed detail interrupts overlapping reads, awaits cancellation settlement,
  checks cancellation again, publishes its observation, and revalidates those
  reads. The 805-card regression proves a later batch's server C replaces detail B;
  unchanged detail does not create refetch loops. This may reread local evaluation
  batches; it does not initiate additional marketplace refreshes.
- **Resolved retryer edge:** independent review caught that synchronous abort is
  insufficient once a retryer resolves but before its cache write. Awaiting
  settlement prevents old data from overwriting the detail or clearing a pending
  replacement's fetching state. Tests cover 22 microtask alignments and cancellation
  during settlement, alongside the existing streamed-body and remount regressions.
- **Applied intent replay:** the persisted exact-command check now precedes
  purchase locks and reevaluation, without bypassing preliminary ambiguity
  observation. Ten real PostgreSQL pack/ack cases prove replay after price changes,
  refund, deletion, purchase locking, and new collisions returns current warnings
  without changing packing, acknowledgment or version. Different or superseded
  stale intents still conflict. All ten cases failed before the fix.
- **Success destination and UUID errors:** changing the actual target list clears
  stale add-success feedback. Secure UUID creation is inside the existing catch;
  failures remain visible and same-name creation retries retain their UUID.

Scoped polish and independent re-review approved the corrected changes. CodeRabbit's
final fix-slice review reported only two overlapping trivial suggestions about a
scheduling-specific test assertion; that assertion was removed while keeping the
matrix-wide ordering/fetch-status checks and independent 805-card revalidation test.
The earlier source-leader cancellation tradeoff and optional cleanups were not part
of the four approved fixes and remain unchanged; no detached source context was added.

Fresh verification for this follow-up:

- `POSTGRES_TEST_URL= go test -race -count=1 -timeout 10m ./...`: PASS.
- Explicit disposable `showprep_test` URL, full PostgreSQL package with
  `-race -count=1 -timeout 10m`: PASS, 141.946 seconds. Focused new replay cases:
  PASS, 8.692 seconds, after the ten-case failing run.
- `npm test`: PASS, 73 files / 661 tests after the final test-only cleanup.
- `npm run typecheck`, `npm run lint`, `npm run build`: PASS.
- Five existing Chromium show-preparation workflows: PASS on the final production
  code, including mobile/tablet/desktop and independent no-price evidence health.
- Compatible-toolchain `make check`: PASS, zero lint issues; diff checks PASS.

The real browser/API/database smoke documented above was not repeated for this
follow-up. Replay behavior was exercised against real PostgreSQL; cache/feedback
changes used real hooks/components and controlled HTTP boundaries. Existing global
header, jsdom, Vite and source-size warnings remain unchanged.

## PR feedback: API-prefix containment

The user approved the authenticated JSON-404 fallback after triaging CodeRabbit's
PR comments. Configured show-preparation routes now claim their whole API prefix;
unknown paths cannot fall through to the public SPA shell. Known endpoint handlers,
missing-auth behavior, and missing-service 503 responses remain unchanged. This
corrects an API-contract defect, not demonstrated leakage of protected ledger data.

`TestShowPrepAPIPrefixIsolation` exercises eleven actual-router cases: missing/wrong
credentials, authenticated unknown GET/POST/root/nested paths, known routes, absent
auth configuration, and absent service. Six unknown-path cases returned 200 before
the fix; all eleven pass afterward. Focused handler/router race tests, the full
`POSTGRES_TEST_URL= go test -race -count=1 -timeout 10m ./...` suite, compatible-toolchain
`make check` (zero lint issues), and diff checks passed. Database-backed and frontend
suites were not repeated for this routing-only follow-up; their prior results are
recorded above. Remaining optional PR comments were triaged, not silently applied.

## Remaining operational notes

- First-time CardLadder configuration in a running process requires one restart
  for showprep to acquire its client. Already-configured production installations
  need no extra setup. This is documented rather than adding unrelated hot reload.
- Existing global-header axe warnings, jsdom scrollTo notices, a Vite loader notice,
  and existing oversized-file guideline warnings remain; none was introduced by
  this feature or used to bypass a failing check.
- No production data, credentials, refresh state, or deployment were modified.
  Source snapshots remain subject to actual per-card completeness/freshness checks;
  a development-only direct-access challenge is not a release gate.

## Reviewer focus

1. Why must a detected DH association hold survive a rejected mutation and deletion?
2. How do exact even medians preserve the 90% threshold without rounding errors?
3. Which observed versions protect hidden selections and packing acknowledgment?
4. How do the new locks avoid the existing campaign-deletion inversion?
5. Why does a failed refresh retain readable evidence but not a Supported status?

## Readiness repair: local upgrade verification (2026-09-15)

The approved amendment is `docs/plans/2026-09-14-show-preparation-readiness.md`.
Tasks 1–4 add derived readiness outside business fingerprints, one bounded tab
coordinator, full-body refresh cancellation/deadlines, read-only expiry/retryAt
observation, selection-safe renewal, compact inventory evidence and progressive
show destinations. The original qualification/money/availability/history rules
remain unchanged. Ordinary inventory/evidence/list reads do not acquire comps.

Task 5 adds no production code, migration or application dependency. Its committed
Go `_test.go` composition and browser driver live alongside existing app-wiring
and browser tests. Reproduction, safety, exact URLs and a separate two-tab preview
mode are documented in `web/tests/show-readiness-real.md`.

Unlike the earlier warmed/snapshot-seeded smoke above, this regression migrates to
45, seeds a real ledger and legacy comps, then upgrades through migration 46 with
**empty** verified evidence. Chromium uses the real Go router, auth service and
LocalAPIToken middleware, inventory/show services, PostgreSQL stores and CardLadder
source adapter. Only CardLadder's remote HTTP response, fixture clock and logging
are controlled. No inventory/evaluate/refresh/list/packing response is stubbed.
No scheduler, live business API, external credential or production ledger is used.

Verified real-wire behaviors:

- Cold Supported-first makes real source calls before selection/list creation;
  24 scoped purchases share 12 normalized identities and acquire in 10+2 batches.
- Actual source-reported $270/$290 sales persist and render Supported at a $300
  DH listed price. The $999 legacy comp never masquerades as verified evidence.
- Ordinary reads, fresh reload, and reconstruction of router/auth/inventory/show
  services, source client, stores and SQL pool do not reacquire current evidence.
- UTC rollover returns stale/Needs review on real reads, retains selected intent
  and disables Add. Clearing selection renews the new 30-date window. Packed
  membership/acknowledgment remains unchanged without an explicit list operation.
- Definitive source HTTP 401 and incomplete-page responses persist failed/partial
  attempts and retain readable verified sales. Focus, reload and selection clear
  do not retry them; explicit Retry recovers.
- Full campaigns/purchases/sales/legacy-comp rows remain byte-for-byte identical,
  covering every stored price and DH listing field. Only explicit create/add/pack
  changes show membership. Final counts: 27 source GETs, seven refresh POSTs,
  three list writes. No financial HTTP writes or ambiguity holds occur.

The new regression was first demonstrated RED by temporarily disabling only the
existing automatic coordinator invocation (the original explicit-only cold-init
path): expected one real source request, received zero after 12 seconds. Restoring
that line yielded GREEN against real persisted data; no production fix was needed.
This was a targeted mutation check, not a claim to have run the entire historical
application at an old SHA.

Local resources: owned PostgreSQL `slabledger-show-readiness-01a09dd6`, loopback
44620; e2e `showprep_readiness_e2e`, destructive adapter suite separately
`showprep_readiness_test`. The test rejects any other e2e URL before connecting.
API/source/control servers use ephemeral loopback ports and stop after testing.
The interactive preview mode runs separately, with test-only controls and an
emitted `fixture.json` for the parent's independent tabs. PG remains owner-managed.

Fresh local gates: full `TZ=UTC DATABASE_URL= POSTGRES_TEST_URL=
SHOW_READINESS_E2E_URL= go test -race -count=1 -timeout 10m ./...` passed; explicit
isolated PostgreSQL race suite passed (86.113s); frontend 82 files/838 tests,
typecheck/lint/build passed. Real-browser race runs passed, including desktop,
390×844 mobile and 768×1024 tablet captures (final real-wire race run 53.651s).
Compatible-toolchain `make check` passed with zero lint issues, including import,
file-size, documentation-path and Playwright-version gates. A separate two-tab
preview launch and SIGTERM cleanup passed. Local Task 5 report records exact
addresses, artifacts and cleanup. Existing late-body,
five-minute, retryAt, observed-version and 805-card regression tests remain intact.

Limits: the fixture service/source clock follows real elapsed time and shifts to
the next UTC midnight on command; source completion retains real wall time. Browser
clock values are set from that fixture. The regression waits up to two minutes if
launched immediately before real midnight; interactive mode does not wait. It does
not simulate weeks of aging. A test-only regression also prevents a frozen preview
clock from invalidating later genuine source completion timestamps. Fixture clock
advancement uses the same wall clock as the real source adapter: an observed host
clock correction made a monotonic-anchored prototype lag by about one second and
conservatively invalidate its newest result. That test-only clock was corrected,
not the business freshness guard. The source
client's pre-existing transient-HTTP retry policy is unchanged (an initial 503
fixture exercised those retries); the deterministic failure case uses a definitive
401. The browser never replays refresh POSTs. Real provider latency/credential
configuration and cross-tab global quotas are not verified here. The fixture omits
the unrelated API-status handler, so its header status warning is not a production
source-health result. The existing tablet global-header overlap remains visible.

**Production rollout remains pending:** separately authorized controlled
production verification with a missing/stale identity and a fresh identity, with
no financial mutations. Local final review and verification are recorded below. No deployment, push,
merge, production warming or production operation was authorized or performed.

## Final readiness fix wave after `631c4854`

All ten final findings (F1–F7 and N1–N3) were reproduced and repaired in one
combined wave, without reopening the approved design or changing backend
qualification, fingerprints, holds, history, or authorization. A configured turn
limit interrupted the first verification pass; the same agent resumed and
completed the remaining gates. No additional review agents were used.

- **F1, actual tab/session lifetime:** QueryProvider is above the pathname-keyed
  error boundary. Route authentication still remounts/revalidates; the QueryClient
  survives only while verified identity is unchanged. Identity changes cancel and
  clear the old cache; abandoned auth responses cannot reset a newer session.
  Non-show queries are invalidated on navigation to preserve fresh-on-entry reads.
  Budgets, stops, and pending writes survive real SPA navigation; logout still
  hard-navigates. There is no process-global shared user cache.
- **F2, bounded evaluation reads:** evaluate shares the feature-local full-body
  stream consumer, retaining cancellation and its 30-second per-attempt timeout
  through success/error bodies. Network/429/5xx reads retain three attempts and
  1s/2s backoff; cancel, timeout and invalid success JSON are not replayed.
  Aggregate cancellation checks prevent late publication. Observation failure
  releases its gate and exposes explicit read retry. Inactive old cohort errors
  do not poison current reads. The global APIClient is unchanged.
- **F3, retry origin:** manual packing-list retry retains the originating list
  and failed/undispatched IDs. Moving A→B cannot offer A's retry; returning to A
  does. Changed selection cannot retarget that old command. Busy/terminal guards
  remain shared across the tab.
- **F4–F7, compact presentation:** retaining stable-key measured virtual-row
  heights fixes collapse overlap without inflating estimates. Show-context badges
  explicitly identify the actual DH price on desktop/mobile, separately from
  local reviewed and CL/Market values. Missing/unavailable/unverified prices stay
  labeled. Accessible button names include visible status, cert and disclosure
  action; readiness summaries use singular card where appropriate.
- **N1–N2, editor versus request lifetime:** the mounted sale form owns its
  presentation blocker; closing/filtering/mobile layout clears abandoned inline
  intent. Async sale, override save/clear, AI accept/dismiss, price hint and DH
  match requests hold coordinator leases through full promise settlement, even
  after form/page unmount. Existing payloads and callbacks are retained.
- **N3, readiness-only observation:** an open disclosure uses incoming aggregate
  readiness for the same business version, retaining detailed sales and existing
  different-version/cache-ordering precedence. No fingerprint, selection version,
  acknowledgment or detail-query key changes were introduced.

Correct-seam RED/GREEN tests cover actual App/BrowserRouter/provider lifetimes,
streamed evaluate success/error bodies, pending dialog operations across real
route navigation/unmount, A→B list retry, visible inline forms, and same-version
open summaries. The existing cancellation-settlement test now supplies a genuine
streamed Response rather than a JSON-only fake; its ordering assertions remain.
The real-wire fixture now seeds reviewed $400, DH $300 and source sales $270/$290
(median $280), while full-ledger immutability assertions remain intact.

Fresh final verification:

- `cd web && npm test && npm run typecheck && npm run lint && npm run build`:
  **86 files / 876 tests passed**, TypeScript/ESLint clean, Vite 373 modules.
  Lint was repeated after the final browser-harness changes and remained clean.
- `TZ=UTC DATABASE_URL= POSTGRES_TEST_URL= SHOW_READINESS_E2E_URL=
  go test -race -count=1 -timeout 10m ./...`: **PASS**, uncached.
- Explicit `POSTGRES_TEST_URL` targeting only local
  `showprep_readiness_test` on 127.0.0.1:44620, with DATABASE_URL and the e2e URL
  empty, `go test -race -count=1 -timeout 10m ./internal/adapters/storage/postgres/...`:
  **PASS, 56.986s**. Preflight confirmed zero other sessions.
- `make check` with `/tmp/slabledger-showprep-tools` plus installed Go1.26.5/Node
  paths: **PASS, zero lint issues**, including architecture, file-size,
  documentation-path and Playwright-version checks. No tool installation or
  hook bypass. Diff/file-size/doc-path checks also ran explicitly.
- `node --test web/tests/show-readiness-browser-checks.cjs`: **4 passed**.
  Focused Go restart/clock race regressions: **PASS, 1.068s**.
- Final real-wire test against only the pinned `showprep_readiness_e2e` database:
  **PASS, 64.02s** (package 65.052s), with the rebuilt production frontend and
  real router/auth/service/PG/source adapter. Counts remain 27 source GETs,
  7 refresh POSTs and 3 explicit list writes; financial ledger unchanged.
  Actual SPA links, repeated disclosure collapse, scrolling, breakpoints and
  fine→coarse pointer changes are exercised. **37 geometry observations / 476
  adjacent pairs, minimum gap 0px**, no overlap or horizontal overflow; trigger
  heights 22px fine and 44px coarse. Final fine/coarse/mobile/desktop-return PNGs
  were opened and inspected, not inferred solely from DOM tests.

Authoritative final browser artifacts are in
`/tmp/show-readiness-final-completion-verified/`; exact per-finding commands,
RED/GREEN logs, exports and screenshots are recorded in the local SDD
`final-fix-report.md`. Geometry screenshots use unclipped CDP viewport capture:
Chromium's full-page/clipped capture was proven to reset pointer emulation and
produce a misleading coarse-labeled image. New assertions verify pointer mode and
button height survive capture. This was a harness correction, not a UI workaround.

Earlier runs encountered two unchanged flaky tests: DHPushConfigCard's cleared
input test and PSA's TestDoRequest_TransientUnauthorizedKeyKept. Both passed
focused reruns without unrelated edits, and the final full suites passed. Existing
jsdom scrollTo, Vite configuration-loader, six unrelated file-size guideline
warnings and the global tablet-header overlap remain disclosed. The temporary
auth-cleanup lint warning was corrected; final ESLint output is clean.

All owned fixture servers and test browsers closed; known fixture ports were
checked closed and both fixture DBs had no lingering test sessions. After final
verification, the parent removed only the identity-checked disposable PostgreSQL
container `slabledger-show-readiness-01a09dd6` and confirmed port 44620 closed.
No production, credentials, dependencies, permissions, push, merge or deployment
changes. Shared-browser assessment tabs and the feature worktree were preserved.

Final independent scoped review of `631c4854..190b9aea` returned **SHIP**: all ten
findings addressed, no new Critical/Important regression found. The reviewer
inspected the complete fix diff, actual App/transport/dialog/virtualizer callers,
four final screenshots, and independently recalculated the 476 adjacent pairs.
The parent also inspected fine/coarse/mobile images and freshly reran all
876 frontend tests, TypeScript, ESLint, compatible `make check`, and the full-range
whitespace check. All passed. The official cached Impeccable CLI again returned
zero findings for the show-preparation directory and InventoryTab; this is a
regex-based source scan, not a visual guarantee. The official browser detector
remained unavailable (`config_missing`); no detector overlay or browser pass is
claimed. Separately authorized production verification and integration remain
pending.

## PR706: three approved pre-ready follow-ups

Follow-up base: `285ab864098592bdd7d51200f540126eaf9f0573`; same feature branch and
linked worktree, without rebase/merge. Only the three approved findings changed:

- HTTP fixture handlers now return contextual errors through a shared test-only
  HTTP500/nonfatal-reporting boundary. The real browser fixture supplies `t.Errorf`,
  so an unexpected source/control error fails the owning Go test even when Chromium
  tolerates or retries the response. Ledger and row queries use request contexts;
  baseline/final assertions remain fatal only in the test goroutine. The shared
  state response checks all four whole-row ledgers before reading evidence, lists,
  items and holds. Controlled source 401/partial responses, gates and cleanup stay
  intact. No speculative final-pool locking/lifecycle change was made.
- The inventory coordinating wrapper distinguishes refusal-before-start from an
  operation that ran. Its inline-price action still owns the operation-error toast
  and rethrows the original API rejection; the wrapper reports its own refusal
  once. Other coordinated actions and pending-write lease behavior are unchanged.
- API/user documentation scopes focus/visibility, UTC, expiry and retryAt observation
  to active inventory show workflows (selection or non-All Support filter). Saved
  lists retain explicit **Update list status** and action invalidation, not timers.

Behavioral REDs were observed before fixes: malformed source parameters and three
real-PostgreSQL control failures (ledger mismatch, ledger read, persisted-row read)
ended with EOF rather than HTTP500. Shared-path HTTP regressions now assert useful
responses, independent error recording, restored valid reads, lock release and
cleanup. The real inventory hook/ToastProvider/APIClient regression observed two
notifications before the fix; afterward it verifies exactly one, the identical
APIError rejection, cents payload, success invalidation, refusal without dispatch,
lease retention through a streamed error body and a non-rethrowing coordinated action.

Fresh verification for this follow-up:

- Full uncached `TZ=UTC go test -race -count=1 -timeout 10m ./...`, with
  `DATABASE_URL`, `POSTGRES_TEST_URL` and `SHOW_READINESS_E2E_URL` unset: PASS.
- Focused source/control/restart/clock race tests against only the pinned disposable
  e2e URI: PASS (3.066s). No separate storage-adapter suite was needed or run.
- Frontend `npm test`: PASS, **87 files / 881 tests**; `npm run typecheck`,
  `npm run lint`, `npm run build`: PASS (373 modules).
- `PATH=/tmp/slabledger-showprep-tools:$PATH make check`: PASS, zero lint issues.
- `node --test web/tests/show-readiness-browser-checks.cjs`: **4 passed**.
- Rebuilt real-wire browser regression with the explicit pinned e2e URI: PASS,
  **64.31s** (package 65.348s), **27 source GETs / 7 refresh POSTs / 3 explicit list
  writes**, ledger unchanged. Artifacts: `/tmp/pr706-fixes-real-wire/`.
- Scoped local `polish-core --fix` against the follow-up base, including new and
  uncommitted files: no unresolved correctness findings; only safe prose wrapping/
  whitespace cleanup. No subagents. Final diff checks passed.

The parent-provisioned container is `slabledger-readiness-pr706-fixes`, full ID
`586dc9f18f746c844fe33d99c81bbce16a0e484bbbe157c978e13167dc45a53e`,
`postgres:17-alpine`, loopback44620, database `showprep_readiness_e2e` only. The real
app/source/control used ports45495/46279/44943; all were confirmed closed afterward,
and the e2e database had zero other sessions. Test browsers and servers closed;
PG remains running for the parent's cleanup. No external source calls, production
operations, credentials, dependencies, push or ready-state changes were made.
Existing jsdom scrollTo, Vite-loader and six file-size guideline warnings remain.
No unrelated test flake occurred in the final gates. The broader stalled non-show
mutation transport limitation and optional findings remain deliberately unchanged.

## Cached show-preparation recovery: Task4 local product proof

Approved recovery: `docs/specs/2026-09-15-cached-show-preparation-design.md` and
`docs/plans/2026-09-15-cached-show-preparation.md`. Task4 base is
`0a13314bb9f9efbecf284cc9cd51194816833e29` on `fix/cached-show-preparation` in the
linked `show-preparation-cached-design` worktree. Tasks1–3 were already implemented
and reviewed. Earlier browser-owned warming, retry/control and first-configuration
restart statements above are historical; the recovery supersedes that ownership.
Final review was pending at this checkpoint; see the completion record below.
No production deployment, source access, backfill, push or PR was authorized/performed.

### What the recovery owns now

The application scheduler owns evidence acquisition, not inventory/checkboxes.
Actual `initializeSchedulers` composes the configured shared CL client/source
provider, PostgreSQL worker store, domain EvidenceWorker and scheduler Group.
The operator service has no source capability. Authenticated legacy `/refresh`
returns410; Admin run/retry only record durable intent. Migration47 adds bounded
lease/control and retry metadata while preserving original evidence, versions,
lists/items and safety holds. No qualification, cents, five-page, 30-UTC-date,
24-hour, lease, retry, exact-identity or financial rules were relaxed.

Task4 adds no backend production behavior, migration or dependency. It extends the
existing opt-in test harness and repeatable guide. Its production UI changes are:

- Correct the selected CL-value sum's label from “list” to **CL value**, without
  changing the sum or inventing a show price. Focused render RED/GREEN: one failed
  of eight before the copy fix; all eight passed afterward.
- Fix a real-browser discovery: a background support change removed the final
  matching price-band pill row above a held selection. The existing view-presentation
  snapshot now retains only prior band keys alongside retained row IDs. Header
  controls retain live zero counts until selection clears or the explicit view
  changes. Prices/evaluations/versions are never frozen or advanced. Focused real
  InventoryTab regression failed after publication before the fix and passed after;
  the final real-worker browser measured identical checkbox coordinates across
  publication (x21/y594.234375), with selection retained and Add blocked for review.
  No new visual design, source controls, pricing behavior or reservation machinery.

### Two decisive modes, plus a separately counted concurrency phase

**Actual worker, A:** seed historical ledger and legacy comps at migration45,
upgrade to47 with empty verified evidence and whole financial rows unchanged.
Start the real production runtime before spawning Node/Chromium. Local Firebase
and CardLadder HTTP fixtures are the only provider replacements; no static-token
bypass, fake worker, Service.Refresh call or successful snapshot seeding. The real
one-request/second shared-client pace and actual source adapter stay in place.

The exact expected 142-identity cohort is asserted, not just a timestamp or nil
RunOnce: 141 current, one controlled incomplete identity, no missing/stale resolved
identities; 153/155 current cards and one unresolved card. Twelve duplicate pairs
acquire once each. $270/$290 source sales yield median$280/Supported against DH$300;
CL$310 and reviewed$400 remain distinct. Zero sales is a successful current result;
one-sale Thin evidence, Below target and missing-DH-price cases are explicit.
The partial identity's two inspectable sales are not certified as complete or
misreported as “No recent sales.” Source history: **142 searches + one Firebase
refresh =143 requests before any browser**. The cold negative control disables
only runtime execution and fails the full-cohort assertion before browser launch.

**Cached use, B:** stop/cancel/join the worker and block every provider endpoint.
Create a disabled production composition for actual coverage/Admin and cached
router/auth/inventory/show/PG responses. Rebuild SQL pool/router/auth/services on
restart with the worker still disabled. Both modes exercise inventory, Supported,
checkbox/select-all/clear, stored evidence, named list creation, add existing, pack,
reload/restart, stale Add/Pack409 and retained history. The source-history arrays
are compared without resetting them: **zero additional source/SDK requests**, zero
browser refresh POSTs, and five empty checkbox/filter request phases.

The explicitly seeded-cache mode is separately labelled a PRECONDITION, not
worker ingestion. It supplies old/failed retained successful evidence and proves
**absolute zero** provider requests. Both B runs end with one list/two members,
whole campaigns/purchases/sales/legacy-comp rows identical, and previous packed
row history unchanged. All156 purchases (155 unsold plus the historical sold row)
and every persisted price/DH field are covered by whole-row equality.

**Concurrent publication, C:** only after B's completed zero-source artifact is
written, worker mode separately permits one actual runtime repair of card29.
The test clock returns from B's synthetic next-day stale observation to real time;
source HTTP is held, and an actual admin retry202 initiates background work. The
selected Needs-review row and its observed version remain held; a real financial
form opens/cancels while the worker is active. Release publishes a genuine complete
source result. The card becomes Supported without disappearing, moving, or silently
acknowledging the new version. New-destination Add and Pack with stale observations
both return409. Existing-membership Add is intentionally idempotent, so it is not
used as the stale-new-add probe. Lists/items stay unchanged by publication/conflicts;
two lists/three memberships reflect only explicit C setup writes. Financial rows
remain unchanged. This separate phase adds **one search + one token**: final history
143 searches/145 total provider requests, never described as zero-source B.

### Actual-source midnight, recovery and financial seams

`TestShowPrepRuntimePositiveMidnightAndFinancialWrite` uses the separate cmd-test
DB. Actual production startup acquires positive and complete-zero evidence, and a
fresh runtime restart preserves both without provider requests. A real scheduler
Group then reuses the configured production SourceProvider through the existing
domain clock seam, advanced to **next UTC midnight (<24h)**. This matters because
the actual source adapter stamps wall-time RefreshedAt. No production clock/reset
API, fabricated success timestamp or weakened freshness guard was added.

During held real source HTTP, an actual authenticated router/service/PG price-
override PATCH204 writes32500 cents promptly. That intentional financial action
has a separate after-write baseline; subsequent renewal changes no further financial
rows. Positive and zero evidence renew current, with four total searches/one token
across population+restart+renewal. Runtime artifact preserves all phase baselines.

Fresh full cmd/storage race suites reuse the existing correct seams for new/resolved
identities without a browser, shared-cert/grader safety, first-save/cross-instance
activation, source401/Firebase400 durable auth hold and explicit recovery, retry
exhaustion/restart, two connections, heartbeat/lease loss, late publication/generation
fencing, failure fairness and sold/refunded/closed/not-received safeguards. Actual
financial write tests and existing form-owned pending/transport tests remain intact.

### Fresh verification and artifacts

Exact runner commands and resource checks: `web/tests/show-readiness-real.md`.
The local SDD `task-4-report.md` records complete commands/results and review focus.
Final code gates performed after local polish:

- Full `TZ=UTC go test -race -count=1 -timeout 10m ./...`, with all database and
  e2e URL variables unset: PASS. DB-dependent tests skip here, not claimed exercised.
- Explicit owned `POSTGRES_TEST_URL`, sequential full `./cmd/slabledger` and
  `./internal/adapters/storage/postgres` races: PASS, **15.297s /75.902s**, including
  migration47 up/down/retention and service-role/anon/authenticated RLS tests.
- `npm test`: **87 files/859 tests**, PASS (14.68s). Typecheck, ESLint and production
  build PASS (373 modules/869ms). Compatible-toolchain `make check` PASS, zero lint
  issues, architecture self-tests/import rules, file sizes, doc paths and Playwright
  version alignment. Existing six unrelated source-size warnings remain.
- Actual worker browser: PASS **179.65s** (package180.686s), artifact directory
  `/tmp/showprep-task4-worker-verified/`. Actual seeded-cache browser: PASS **36.82s**
  (package37.857s), `/tmp/showprep-task4-cached-verified/`.
- Four owned-browser cleanup/whole-row-clearance tests PASS. Focused Go real-PG
  control failure, midnight seed, source error/counting, restart and clock races
  PASS, **6.129s**. No unrelated failing gate was bypassed.
- Geometry: worker36 observations/475 adjacent row pairs, cached36/473 pairs;
  minimum gap0px, no overlap/horizontal overflow. Fine/coarse trigger sizing,
  repeated expand/collapse/scroll, mobile/tablet bottom-bar clearance, keyboard
  packing, Escape and destination focus-return passed. Rendered fonts loaded.
- Local 155-card event-to-next-frame timings (filter/select/clear): worker
  **15.1/19.0/18.3ms**; cached **13.7/19.3/20.0ms**. These are local samples under
  the100ms target, not production percentile or provider-latency claims.

Final actual desktop/mobile/Admin/selected-evidence/destination/last-row and
publication screenshots were opened and inspected. Artifacts contain migration
pre/post, full financial baselines, evidence and worker rows, exact source queries,
coverage, browser/explicit request logs with stale versions/statuses, item history,
geometry and PNGs. Historical artifacts and earlier notes remain preserved.

### Recovery/rollout limits

Only parent-owned PostgreSQL `slabledger-cached-show-01a09dd6`, exact ID
`d56b5500410d90a5609c98656c2a33b7c9364135f588d3013bbfc34f0b3d496b`, loopback44620,
user/owner showprep was used. Browser schema resets are restricted to
showprep_readiness_e2e; sequential cmd/storage suites use showprep_cached_test.
All owned servers, Groups, SQL pools and browsers are closed; PG is retained for
the parent. No shared CDP, default DB, live Firebase/CardLadder credentials, port4173,
production operation, dependency change, amend, hook bypass, push or deployment.

The Admin screenshot's unrelated CL/DH/PSA panels are unconfigured/omitted fixture
integrations, not a production-health result. Inventory fleet coverage can lag a
new per-card read until its existing minute poll; it never initiates acquisition.
The legacy value/comp pipeline remains separate and can make its own provider calls.
The test clock covers next-midnight renewal, not arbitrary weeks of clock changes.

Rolling schema47 down preserves snapshots/list/history/holds but loses worker
ownership/auth-hold/retry bookkeeping; stop workers and use a compatible app before
an authorized rollback. Local tests do not authorize resetting production retries.
Worker rollout is **not complete**: separately authorize deployment, initial server-
owned catch-up and real-inventory coverage plus zero-source browser verification.
Final review was outstanding at this checkpoint; the completion record below supersedes it.

## Final bounded recovery fix wave after `6fa439d0`

This appendix supersedes Task4's shared cmd/storage test-DB recipe, not its
historical results. Only validated findings F1–F3 were addressed. No further
review agents, push, PR, deployment, production credentials or live provider
requests were used. The scoped recheck and its final residual correction are recorded below.

### Changes and RED/GREEN

- **F1, independent runtime database:** cmd fixtures now require
  `SHOW_PREP_RUNTIME_TEST_URL`, never `POSTGRES_TEST_URL` or an app DB fallback.
  The guard admits only an explicit loopback PostgreSQL URI/port, known test user,
  exact `showprep_runtime_test` database and no query overrides; connected user
  must own it before migrations/truncation. CI provisions that separate DB and
  wires both independent opt-ins for parallel packages. The storage-only CI-URI
  regression failed against the old helper, then correctly skipped runtime;
  dedicated runtime cases passed. Created only `showprep_runtime_test` in the
  already verified parent-owned container. `showprep_cached_test` remains storage
  only; `showprep_readiness_e2e` remains browser only. No remote CI was run.
- **F2, held layout:** the existing presentation snapshot retains an already
  visible Needs Attention banner with live zero counts. The coverage component
  retains its measured existing height and live counts while selection is held,
  keyed by the existing view so explicit changes/clear restore normal quiet
  behavior. No initial checkbox-induced header row, new controls, CSS framework,
  frozen evaluations or acknowledged versions. Actual desktop/mobile DOM RED
  caught coverage collapse (desktop26px/mobile70px) and attention collapse59px.
  All12 cases now pass, including initially quiet views and both clear and
  explicit price-band view reset. Existing price-band/version regressions pass.
- **F3, safe diagnostics:** existing AppError constructors/WithContext preserve
  internal operation and original causes, including Is/As and lease-loss identity.
  Runtime logs only allowlisted operation/category, deadline/canceled booleans,
  and recognized database SQLSTATEs. No raw error, provider text, SQL parameter,
  URL, credential or arbitrary context is serialized. Real worker-injected
  Acquire/Candidates/Begin/Finish/End failures and actual ConfiguredClient config
  read failures cover SQL/cancel/deadline/malicious cases. Source resolve/fetch
  causes remain inspectable internally but safely redacted in logs. Public DTOs,
  persisted safe messages, qualification, retry, lease/pacing and ownership stay
  unchanged. RED failed missing stage/log fields and dropped source causes;
  GREEN passes all29 diagnostic cases plus existing worker/runtime races.

### Fresh final verification

- Full UTC uncached Go-race without DBs: PASS; DB fixtures skipped as intended.
- Actual CI-shaped parallel `go test -race -v -count=1 -timeout 10m
  -coverprofile=… -covermode=atomic ./...`, with storage and runtime in their
  separate databases: PASS. cmd16.142s, storage84.353s; migration47 local and
  service-role/anon/authenticated RLS retention subtests ran. This is a local
  isolation result, not remote CI/deployment verification.
- Full frontend:87 files/859 tests PASS (21.78s); typecheck/lint/build PASS.
  Focused Chromium geometry12/12 PASS (24.9s). Compatible-toolchain `make check`
  PASS, zero lint issues; existing source-size/Vite/jsdom warnings remain.
- Actual production-composition worker browser PASS179.85s, cached browser
  PASS36.61s, both after final layout and diagnostics changes. A remains142
  searches+1token; B adds0, with five empty checkbox/filter logs per mode and0
  browser refresh POSTs. Separate C adds1search+1token, final143/145 totals.
- Financial/migration baseline equality checked again from phase artifacts:
 156 purchases, one campaign/sale/legacy comp per browser mode unchanged;
  original packed history retained. Runtime positive/zero next-midnight renewal
  reran on the dedicated runtime DB:32500-cent deliberate PATCH has its own
  baseline, renewal adds no further financial changes;4searches/1token total.
- Worker C checkbox remains x21/y594.234375,13×13 before/after publication.
  New focused desktop coverage/attention rows stay at y593.234375/y598.234375;
  mobile at y786.421875/y747.421875. Reset hides notices normally; geometry JSON
  records the released table footprint. Existing36 virtual-row observations
  per real mode have475/473 adjacent pairs, minimum gap0 and no overflow.
- Four actual owned-browser cleanup/whole-row tests PASS (1.948s); Go control,
  source, restart and clock cleanup races PASS (6.055s).

Fresh artifacts (old Task4/RED directories retained): `/tmp/showprep-final-worker/`,
`/tmp/showprep-final-cached/`, `/tmp/showprep-final-runtime/runtime-renewal.json`,
`/tmp/showprep-final-layout-verified/`. Exact commands, logs, source/financial
attribution and changes are in local ignored
`.superpowers/sdd/2026-09-15-cached-show-preparation/final/fix-report.md`.
The durable runner is `web/tests/show-readiness-real.md`.

Final resource checks: same owned container ID and loopback44620; all three DBs
owned by showprep; zero other sessions. Worker ports45089/46353/45989, cached
ports46091/45485/44921 and focused layout45173 closed. Groups, pools, source/app/
control servers and owned browsers joined/closed; container retained. The final
control tests reset only the browser schema; phase data/history lives in artifacts.
Production rollout remains explicitly incomplete.

## Local completion — 2026-09-16

Final reviewed implementation: `5ce621c51ca236aff4a0993300e4e0ffbe313196`.
The independent whole-branch review and scoped corrections conclude **SHIP**;
Task4 spec and quality pass. This is a local code verdict, not a deployment or
real-inventory readiness claim.

The scoped recheck found one additional presentation regression: an already-complete
fleet could show only a missing-price notice, but selecting a card used that
notice's height to enable previously absent fleet-summary text. The final two-file
correction tracks prior summary visibility separately from overall notice height.
Actual mobile RED measured a66px shift; all16 desktop/mobile Chromium cases now
pass, including price-only content, live updates, stale versions, clear/view reset
and zero interaction requests. Full859 frontend tests, typecheck, lint, build and
make check passed after that correction. Backend/CI/diagnostics were unchanged.

The controller also freshly ran full Go-race with both isolated PostgreSQL opt-ins
(cmd16.574s/storage97.914s),859 frontend tests, typecheck/lint/build and make check
on `43892d49`, before the final price-only conditional correction. Actual worker
and cached browser modes passed179.85s/36.61s on that same backend/layout baseline;
they were not rerun after the two-file residual. Fresh16-case Chromium geometry
covers the residual and all12 original focused cases. Prior source/financial
phase artifacts remain valid historical evidence, not relabeled as new runs.

Important implementation decisions remain: identity/card coverage units are
explicit; abandoned starts consume the bounded retry budget; serial-only CL
mappings require an existing purchase identity anchor or grader-aware enrichment;
auth rejection is held durably; control acknowledgements distinguish uncertainty;
and runtime tests have an independent CI database. No monetary rules, business
fingerprints, list/packing history or price-association protections were weakened.
Safe worker diagnostics preserve internal causes but expose only allowed fields.

The final pass required one extra bounded price-only follow-up beyond the planned
fix wave, rather than parking a known checkbox regression. No further broad audit,
new feature, dependency or production operation was added. Remote CI remains unrun.
The linked branch and review/test artifacts are retained for the next integration
choice; the owned disposable PostgreSQL container is removed after verification.

Reviewer knowledge check:
1. Which provider requests belong to server preparation, cached use and explicit background repair?
2. Why are lease ownership, attempt generation and credential generation separate fences?
3. Why is a cert-only cached mapping insufficient to establish another purchase's identity?
4. What distinguishes retained layout footprint from frozen data or silent version acknowledgement?
5. Which verification remains necessary after separately authorized production deployment?
