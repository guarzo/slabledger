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
