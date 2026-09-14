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
