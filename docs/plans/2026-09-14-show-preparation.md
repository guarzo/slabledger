# Card-show Preparation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Filter inventory by defensible listed-price evidence and save persistent show packing lists.

**Architecture:** A `showprep` domain sibling owns evaluation, availability, and list lifecycle behind narrow ports. PostgreSQL supplies separate verified evidence snapshots and list persistence; the existing CardLadder client supplies bounded detailed sales. Authenticated API operations serve both inventory and shortlist UI without changing inventory's existing 90-day analytics or financial behavior.

**Tech Stack:** Go 1.26, PostgreSQL/pgx, React 19, TypeScript, TanStack Query, Vitest, Playwright; no new dependencies.

**Spec:** `docs/specs/2026-09-14-show-preparation-design.md` (including the accepted independent-review revisions).

## Global Constraints

- Evaluate the last-synced DH listing price, not a new show-specific price.
- Recent means 30 days. Support means a median at least 90% of listed price.
- At least two matching sales are required for `Supported`; one qualifying sale is `Thin evidence`.
- Selection and packing never reprice, delist, reserve, or sell inventory.
- Domain code never imports adapters or another inventory sibling. Stored money is integer cents. Keep source files below 500 lines; 600 is the hard limit.
- Exact source/profile/grader/grade identity, entire 30 UTC calendar date window including today, <=24h complete evidence only. No aggregate fallback and no partial results classified as supported.
- Price-association ambiguity considers every purchase sharing the cert, not only current unsold rows. Persist detected ambiguity independently of purchases/campaigns; removing the competing record must not clear it. Do not repair the DH synchronization pipeline in this feature.
- Availability policy precedence: missing purchase/campaign, sold, refunded, closed campaign, not received, ready. Pending campaigns are eligible. Unknown availability fails closed for new adds/packing. Retain existing list rows and packing history.
- No files outside the linked worktree; no production writes, source refresh mutations against production, pushes, merges, or new dependencies. No secrets in output or artifacts.
- Follow Go table-driven tests and central mocks in `internal/testutil/mocks/`. Write failing behavior tests before implementation. Record actual red/green commands in task reports.
- Fresh race checks before commits; PostgreSQL checks always use `-count=1` and explicitly reset the new independent tables per test. Controller owns commits while file-disjoint backend/frontend work runs concurrently, so neither worker commits the other's incomplete files.

## Pre-implementation source gate

**2026-09-14: blocked; no application code written.** Production inventory and CL
status endpoints returned 200, and authorized credential retrieval/token exchange
succeeded. The first authenticated CardLadder sales query returned 403. A separate
header-only request confirmed `cf-mitigated: challenge`: Cloudflare is requesting
a browser challenge, not providing a sales response. No live sales, pagination,
USD/accepted-offer, or shipping semantics were verified. One of the eight allowed
sample requests was used; no production records or credentials were changed.

Execution is paused for the operator's choice: restore source access first, or
proceed with fail-closed feature implementation while retaining live source
verification as a release blocker. Do not certify existing 90-day aggregates or
assume a successful future HTTP response establishes unverified price semantics.

## Execution and file ownership

User's standing agent-team workflow selects coordinated subagents without another execution-choice prompt. Native team tools are unavailable; use named background subagents with explicit ownership. Backend owns `internal/` and `cmd/`; frontend owns `web/`. Controller owns this plan, implementation notes, API/user documentation, integration checks, and final review. Neither implementer may dispatch additional agents. Backend/frontend may run concurrently against the fixed wire contract below; API-contract changes must be coordinated first. Each slice receives spec/quality review, then the integrated branch receives independent review and polish.

Repository ignores `docs/superpowers/`; use established `docs/plans/` and `docs/specs/` instead.

## Wire contract (shared by Tasks 1 and 2)

All monetary numbers below are cents, including API fields, matching existing campaign endpoints. Times are RFC3339 UTC; dates are `YYYY-MM-DD`. Required array fields return `[]`, never `null`. Error responses follow existing `{error: string}` handling; invalid input 400, unauthenticated 401, missing resource 404, stale state/unavailable mutation 409, internal storage failure 500. Evidence problems are per-card evaluation reasons, not whole-list 500s.

```ts
type SupportStatus = 'supported' | 'thin_evidence' | 'below_target' | 'no_recent_comps' | 'needs_review' | 'no_listed_price';
type Availability = 'ready' | 'not_received' | 'sold' | 'refunded' | 'campaign_closed' | 'removed' | 'unknown';
interface ShowEvaluation {
  purchaseId: string;
  cardName: string;
  certNumber: string;
  grader: string;
  grade: number;
  status: SupportStatus;
  reason: string;
  availability: Availability;
  canAdd: boolean;
  canPack: boolean;
  listedPriceCents: number;
  localPriceCents: number;
  priceMismatch: boolean;
  priceAssociationUnclear: boolean;
  listingSyncedAt: string;
  medianCents: number; // display-rounded only; server compares exact even medians
  compCount: number;
  latestSaleDate: string;
  windowStart: string;
  windowEnd: string;
  refreshedAt: string;
  evidenceVersion: string;
  version: string; // fingerprint of evaluated inputs, not the instant of reading
}
interface ShowSale {
  id: string;
  date: string;
  priceCents: number;
  platform: string;
  url: string;
  listingType: string;
}
interface ShowEvidence { evaluation: ShowEvaluation; sales: ShowSale[] }
interface ShowList { id: string; name: string; createdAt: string; updatedAt: string }
interface ShowListItem {
  id: string;
  purchaseId: string;
  cardName: string;
  certNumber: string;
  grader: string;
  grade: number;
  addedAt: string;
  packedAt: string; // empty when unpacked
  version: number;
  acknowledgedPriceCents: number;
  acknowledgedStatus: SupportStatus;
  evaluation: ShowEvaluation;
  priceChanged: boolean;
  supportChanged: boolean;
}
interface ShowListDetail {
  list: ShowList;
  items: ShowListItem[];
  summary: {
    totalCount: number;
    packedCount: number;
    notReceivedCount: number;
    unavailableCount: number;
    knownValueCents: number;
    missingPriceCount: number;
    ambiguousPriceCount: number;
  };
}
```

| Operation | Request | Success body |
|---|---|---|
| `POST /api/show-prep/evaluate` | `{purchaseIds: string[]}` (1–200 UUIDs) | `{evaluations: ShowEvaluation[]}` |
| `GET /api/show-prep/evidence/{purchaseID}` | none | `ShowEvidence` |
| `POST /api/show-prep/refresh` | `{purchaseIds: string[]}` (1–10 UUIDs) | `{evaluations: ShowEvaluation[]}`; individual failures carry reasons |
| `GET /api/show-prep/lists` | none | `{lists: ShowList[]}` |
| `POST /api/show-prep/lists` | `{id: string, name: string}` | `ShowList`; caller UUID makes transport retries idempotent |
| `GET /api/show-prep/lists/{listID}` | none | `ShowListDetail` |
| `PUT /api/show-prep/lists/{listID}` | `{name: string}` | `ShowList` |
| `POST /api/show-prep/lists/{listID}/items` | `{items: {purchaseId: string, evaluationVersion: string}[]}` (1–200) | `ShowListDetail` |
| `PUT /api/show-prep/lists/{listID}/items/{itemID}` | `{version: number, evaluationVersion: string, packed?: boolean, acknowledge?: boolean}` | `ShowListDetail` |
| `DELETE /api/show-prep/lists/{listID}/items/{itemID}` | none | `{removed: true}` |

Names are trimmed, required, and at most 120 Unicode characters. UUIDs and body sizes are bounded at the handler. Batch add is atomic and unique by list/purchase; an existing member is not silently re-acknowledged. Pack/ack require the displayed evaluation version and item version. Unpack is allowed for unavailable members without evidence refresh. Explicit repeated operations do not toggle or double-add; stale conflicting intent yields 409, and the UI refetches without discarding selection. Acknowledge only operates on data the operator saw. List creation retries with the same ID/name return the existing list; ID reuse with a different name conflicts.

Refresh processes <=10 unique identities and <=5 pages of 100 sales per identity, using the client's existing rate limiter and request coalescing per identity. Exhaustion of those bounds means incomplete, not success. Do not automatically re-refresh on inventory render. Existing evidence remains readable on a failed refresh, but the new attempt's failure makes classification need review. Persist complete empty windows. Publish generations atomically; stale/out-of-order refresh completions cannot overwrite newer attempts.

### Refresh deadline hierarchy

The existing server defaults to a 90-second write timeout, but that value is configurable. Pass the actual `cfg.Server.WriteTimeout` through application wiring to the refresh handler; do not hardcode the default or increase the global server timeout. Capture the request start at route entry, before body decoding or other refresh work, and anchor all phase deadlines to that instant so work already spent is not granted a fresh budget.

```go
func refreshDeadlines(start time.Time, writeTimeout time.Duration, parent context.Context) (source, persistence, response time.Time) {
    window := 80 * time.Second
    if writeTimeout > 0 { // zero is an unlimited server write timeout, not an unlimited refresh
        window = min(window, writeTimeout)
    }
    if deadline, ok := parent.Deadline(); ok {
        window = min(window, max(time.Duration(0), deadline.Sub(start)))
    }
    reserve := min(5*time.Second, window/4)
    source = start.Add(min(60*time.Second, window-2*reserve))
    persistence = start.Add(window-reserve)
    response = start.Add(window)
    return
}
```

Under the default configuration, upstream work stops by 60 seconds, persistence stops by 75 seconds, and response delivery has a budget ending at 80 seconds, ahead of the server's 90-second deadline. Shorter configured write timeouts or parent deadlines proportionally shorten those phases. All phase contexts remain children of the original request context; cancellation must not start detached background work. If decoding or previous work has exhausted a phase, do not start another source call.

Commit an attempt-start marker before upstream work. It makes incomplete attempts visible even if client cancellation later prevents a final failure write. Persist timeout/failure results using the longer-lived persistence context, never the expired source context; leave time to encode and deliver per-card reasons. Storage failure is a request error, never a successful complete refresh. Recheck remaining time between identities/pages and do not clear an attempt-start marker without committing its result.

The UI keeps `timeoutMs: 120000` as a transport ceiling, not a server budget. Explicit refresh uses a feature-local instance of the existing `APIClient` with `maxRetries = 1`; do not change the global client or automatically replay a timed-out/failed refresh. The operator can retry explicitly. Ordinary reads and the already-idempotent list operations retain their existing retry behavior.

## Task 1: Tested backend slice (domain, persistence, evidence, API)

**Owner:** backend subagent. **Consumes:** approved spec and fixed wire contract. **Produces:** all `/api/show-prep/*` operations and backend unit/integration tests; no frontend edits.

**Files (new unless marked Modify):**
- `internal/domain/showprep/types.go`, `evaluation.go`, `availability.go`, `repository.go`, `service.go`, `service_lists.go`, `service_refresh.go`, and corresponding tests.
- `internal/testutil/mocks/showprep.go` for any required central Fn-field mocks.
- `internal/adapters/storage/postgres/showprep_store.go`, `showprep_evidence.go`, `showprep_lists.go`, `showprep_items.go`, `showprep_price_holds.go`, and corresponding tests. Create `showprep_testhelper_test.go` for explicit fixture isolation. Split further by responsibility if a file approaches 500 lines.
- `internal/adapters/storage/postgres/migrations/000046_show_preparation.up.sql` and `.down.sql`; migration test `migration_000046_test.go`.
- `internal/adapters/clients/cardladder/showprep.go` and `showprep_test.go`, adapting the existing client without altering the old sales pipeline.
- `internal/adapters/httpserver/handlers/showprep.go`, `showprep_lists.go`, `showprep_deadlines.go`, and tests.
- `internal/adapters/httpserver/routes_showprep.go` and `showprep_deadline_test.go` for real-server timeout coverage; Modify `internal/adapters/httpserver/router.go` to add optional handler wiring and register routes.
- Modify `cmd/slabledger/handlers.go` and `cmd/slabledger/server.go` to inject the DB-backed service and optional existing CL client. Extract helper wiring to `cmd/slabledger/showprep.go` rather than enlarging already-large functions.

- [ ] **Step 1: Read source preflight result and pin domain behavior with failing tests.** Record unresolved upstream facts in implementation notes; do not certify unsupported semantics. Use the approved availability decision table and wire contract as assertions. Test even-median arithmetic without floats:

```go
// For sorted integer-cent prices:
medianTwice := int64(prices[len(prices)/2]) * 2
if len(prices)%2 == 0 {
    medianTwice = int64(prices[len(prices)/2-1]) + int64(prices[len(prices)/2])
}
supportedPrice := 5*medianTwice >= 9*int64(listedPriceCents)
```

Define a pure `Evaluate` function with injected time and a pure availability function. A representative table includes listing 30000 with sale sets `{27000,27000}` supported, `{26999,27000}` below target, `{40000}` thin, `{}` no recent comps only when a current complete snapshot exists, and missing/partial snapshots needing review. A matching CL snapshot plus cross-grader cert collision still needs review. Every explicit availability reason overrides `ready`; pending campaign remains ready. Record the initial failing `go test ./internal/domain/showprep/...` command before adding production logic.

- [ ] **Step 2: Implement the domain and narrow ports.** Wire DTO field names verbatim. Version fingerprints include purchase identity/availability, listed/local prices, durable association-hold state, and evidence generation/window; do not include `time.Now()` itself. Keep actual costs/listing operations outside the service's ports. Add a narrow price-association observation port; its adapter records collisions before returning the effective hold flags, while `Evaluate` remains pure:

```go
type PriceAssociationStore interface {
    ObservePriceAssociations(ctx context.Context, purchaseIDs []string) (map[string]bool, error)
}
```

`true` means this purchase has a durable ambiguity hold, whether or not the competing row still exists. A failure to read or durably record association state cannot produce a trusted price: surface `Needs review` and exclude that price from known-value totals. Update central mocks and run `go test -race -count=1 ./internal/domain/showprep/...`.

- [ ] **Step 3: Add failing PostgreSQL tests, then additive persistence.** Create `showprep_lists`, `showprep_items`, `showprep_evidence`, and `showprep_price_holds`. Store lists/items with immutable identity snapshots and original purchase UUID that survives purchase/campaign deletion (do not cascade-delete members with purchases). Use list-owned member FKs, unique list/purchase membership, positive item versions, and packed/acknowledged fields. Verified evidence retains full profile/grader/grade identity and atomic JSONB generation plus source/coverage/attempt metadata. Separate latest failed attempt state from last readable successful payload. RLS follows role-guarded service-role policies and revokes from anon/authenticated for all four tables. Queries include all cert collisions and complete availability facts, not merely global unsold readers.

**Durable ambiguity rule:** `showprep_price_holds` is keyed by original purchase UUID and retains cert, grader, and first-detected timestamp without a purchase/campaign foreign key or automatic expiry. In a transaction/consistent SQL snapshot, observe cert collisions across the whole ledger and insert holds for every affected purchase, including a competing purchase outside the requested batch. Commit those holds before returning evaluation flags. `ON CONFLICT` preserves the first observation. Effective ambiguity is a persisted hold OR a newly observed collision, not merely the result of a fresh collision-count query.

**Clearing rule:** v1 has no automatic or public hold-clear operation. Deleting a competing purchase/campaign, editing its cert, changing price, advancing sync time, refreshing CL evidence, or acknowledging a shortlist warning never clears the hold. A future clearing operation is permissible only after an identity-safe DH read verifies the surviving purchase's immutable DH inventory identity, cert, grader, and current price together, with durable verification provenance. That verification capability is not present in this plan, so holds remain sticky in v1 rather than exposing a reset that pretends to verify identity. This adds only showprep-owned safety persistence; it does not modify DH synchronization or purchase-deletion behavior.

**Regression:** create two graders sharing a cert with different DH prices, evaluate one, and assert both durable holds were written. Delete the competitor via the existing purchase deletion path, then repeat via campaign deletion in a separate case. Reevaluation must remain `Needs review`, `priceAssociationUnclear` must stay true, and known-value totals must continue excluding the price across service reconstruction/reload. Acknowledge and refreshed timestamps must not clear it. Also verify no unrelated unique-cert purchase acquires a hold.

**Test isolation:** existing `setupTestDB` truncates campaigns only; the new independent tables deliberately survive that. Every showprep storage test uses the following dedicated helper, and tests sharing that database must not use `t.Parallel`:

```go
func setupShowPrepTestDB(t *testing.T) *DB {
    t.Helper()
    db := setupTestDB(t)
    reset := func() {
        _, err := db.ExecContext(context.Background(), `TRUNCATE TABLE
            showprep_items, showprep_lists, showprep_evidence, showprep_price_holds
            RESTART IDENTITY`)
        require.NoError(t, err)
    }
    reset()
    t.Cleanup(reset)
    return db
}
```

The helper's explicit reset is test-only; do not introduce production cascade-delete links to obtain isolation. Existing migration tests retain their schema reset/restore pattern rather than invoking this helper while tables are intentionally absent. Add a fixture test that seeds every new table, invokes the reset, and verifies all four are empty. Add retention tests that delete purchases/campaigns and assert the independent records remain before fixture cleanup.

For atomic pack eligibility, lock the campaign row and purchase row in a consistent order, re-read sale/refund/receipt/phase after locking, and update the list item only if both expected versions match. PostgreSQL's sale foreign key must participate in the purchase lock so an insertion cannot interleave between validation and commit; prove with a concurrency test rather than changing sale writers speculatively. Batch add validates every new candidate within its transaction and retains existing members unchanged. Unpack/remove remain possible after source records disappear. Execute only against the controller-provided disposable `POSTGRES_TEST_URL`, never `DATABASE_URL`.

- [ ] **Step 4: Add failing httptest-backed CL adapter tests, then implement bounded refresh.** Check profile, normalized grader and condition on every record; reject missing/contradictory identity, nonfinite/nonpositive/unverified-currency prices and malformed/future dates. Parse source timestamps into UTC dates. Deduplicate IDs, verify date ordering across pages and stable pagination bounds, detect repeated pages, and establish exhaustion or verified cutoff coverage. More than five pages produces partial evidence. No existing unqualified CL rows are automatically trusted. Implement the shared deadline hierarchy: source work receives the shorter context, while attempt/result persistence receives the reserved context. Test unavailable credentials, zero hits, source timeout with successful failure-state persistence, client cancellation leaving an incomplete attempt, malformed records, failed subsequent refresh retaining older evidence, and two concurrent refresh attempts. Log bounded per-identity failures without credentials.

- [ ] **Step 5: Add handler/router tests, then implement and wire API.** Use existing JSON/error helpers, `RequireAuth`, a safe disabled response when auth/client is unavailable, and method-specific mux routes. Add a SPA `/shows` route for the new frontend while keeping all data endpoints authenticated. Service construction is independent of CL availability so saved lists and missing-evidence browsing still work. Wire the actual server write timeout into refresh deadlines without changing the server's global timeout. Handler tests must cover transport contract, 400/401/404/409, stale acknowledgement and state, duplicate requests, and source failures returning explicit per-card evaluations.

Add a real HTTP-server regression using `httptest.NewUnstartedServer`, set `server.Config.WriteTimeout = time.Second` before starting it, and give the refresh handler that same configured timeout. Use a fake source that waits for cancellation beyond its 500ms source budget, then a persistence fake that verifies its context is still live. Read the full response through `server.Client()`, asserting successful JSON decoding, per-card `Needs review` reasons, saved incomplete/failure metadata, and no additional source call. A response recorder alone does not exercise socket write deadlines. Table-test the deadline helper for default 90s, short 1s, unlimited 0, shorter parent deadline, expired parent, and elapsed body-decoding time. Run focused handler, router, and `cmd/slabledger` tests.

- [ ] **Step 6: Backend self-review and task review handoff.** Run `go test -race -count=1 -timeout 10m ./...`, `scripts/check-imports.sh`, and file-size checks. Run the PostgreSQL command below only after the controller has provisioned and verified an isolated throwaway database and set `SHOWPREP_TEST_URL` to that database; never fall back to `DATABASE_URL`. `-count=1` is mandatory, not an optional rerun flag:

```bash
POSTGRES_TEST_URL="${SHOWPREP_TEST_URL:?controller must supply the verified disposable database URL}" \
  go test -race -count=1 -timeout 10m ./internal/adapters/storage/postgres/...
```

Record the explicit per-test cleanup and actual uncached DB execution in the task report alongside red/green evidence, paths, migration behavior, and unresolved source issues. Do not commit while the frontend agent is running; controller reviews and commits the finished slice.

## Task 2: Tested inventory and saved-show UI

**Owner:** frontend subagent. **Consumes:** fixed wire contract above. **Produces:** inventory support filters, evidence disclosure, shortlist selection and saved packing page. Backend may be under construction; mock these exact endpoints in tests, not guessed alternatives.

**Files:**
- Create `web/src/types/showprep.ts` (the wire types), `web/src/js/api/showprep.ts`, `web/src/react/queries/useShowPrepQueries.ts`.
- Create `web/src/react/pages/show-preparation/` components/hooks/tests for support filters/badges, evidence details, add-to-list controls, list management and packing rows; keep each component below 500 lines.
- Create `web/src/react/pages/ShowPreparationPage.tsx` and tests.
- Modify `web/src/react/App.tsx`, `web/src/react/pages/GlobalInventoryPage.tsx`, `web/src/react/pages/campaign-detail/InventoryTab.tsx`, and bounded integration points in `inventory/{DesktopRow,MobileCard,ExpandedDetail,InventorySelectionBar,useInventoryState,inventoryCalcs}.tsx|ts` as needed. Do not restructure unrelated inventory flows.
- Create `web/tests/show-preparation.spec.ts` for browser workflow and responsive coverage.

**UI brief:** approved spec is the confirmed product brief. Existing dark product theme, system typography, dense tabular data and inline disclosure; no new palette, decorative imagery, or general redesign. Native image generation is unavailable, so probes/north-star mock are skipped. Read PRODUCT.md/DESIGN.md and the impeccable spatial/typography/interaction/responsive references before implementation. Reuse existing components where they fit; new state has visible text, not color-only cues.

- [ ] **Step 1: Add wire types and write failing behavior tests.** Test a representative complete evaluation and list response, then introduce the real API/query layer. Example transport and visible assertions:

```ts
await showPrepAPI.evaluate(['11111111-1111-4111-8111-111111111111']);
expect(fetch).toHaveBeenCalledWith('/api/show-prep/evaluate', expect.objectContaining({
  method: 'POST', body: JSON.stringify({purchaseIds: ['11111111-1111-4111-8111-111111111111']}),
}));
// Component tests use actual query hooks + mocked HTTP responses:
expect(screen.getByText('Supported')).toBeVisible();
expect(screen.getByRole('checkbox', {name: /Packed.*12345678/})).not.toBeChecked();
```

Test behavior rather than just mocks in component tests: applying support plus search/tab filters narrows the actual rows and count; a failed evaluation is not displayed as no comps; an unreceived/refunded row is not packable; ambiguous price is marked rather than included in totals. Record the failing test command.

- [ ] **Step 2: Implement API/query layer.** Batch all relevant inventory IDs in groups <=200 with bounded concurrency and stable query keys; no upstream refresh on read. Refresh explicit user-selected cards in groups <=10 with `timeoutMs: 120000`, visible progress/cancellation/failure state, and query invalidation after writes. Use a feature-local `APIClient` with `maxRetries = 1` for refresh only; prove that timeout/network error and 5xx each make one request, while the UI offers an explicit retry. Do not change the global client. Handle arrays and missing per-card results defensively as loading/error rather than supported. Use caller-created UUID for list creation retries. Preserve selection when adding fails and refetch on 409 without falsely claiming success.

- [ ] **Step 3: Integrate support into existing inventory.** Display DH list, median/count/latest sale and status compactly; inline lazy evidence detail shows all returned eligible sales, freshness/window, sources and safe links. Do not reuse `List / Rec` as the actual DH quote. Add support and show-selection controls without forking a second inventory browser. Show selection defaults to `ready`, with opt-in `not_received`; enforce intersections after existing search/tab filtering and ensure counts/select-all operate on the same displayed set. Selection should preserve previously selected IDs across filter changes; only explicitly selected IDs are added. Provide named-list picker/create and an inventory link to `/shows`.

- [ ] **Step 4: Implement saved show page.** Choose/create/rename saved lists, inspect evidence, remove items, explicitly set packing state, and acknowledge price/status changes against observed versions. Existing sold/refunded/removed/closed members stay visible, history stays checked, and unpack/remove remain available. Show all contract totals with clear availability and price-ambiguity labels. Persist across reload via server reads, not local-only state. Render useful empty/loading/partial/error states with accessible retry buttons and long-card-name handling. No repricing/sale/delist actions on the new page.

- [ ] **Step 5: Browser tests and self-review.** Playwright intercepts new and inventory/auth endpoints to exercise filter → inspect → add → navigate → pack → reload → change price/refund → warn without removing. Cover desktop, tablet, mobile; include at least one fresh screenshot per viewport for controller inspection. Run focused Vitest tests, `npm run typecheck`, `npm run lint`, `npm run build`, and focused Playwright. Record exact results in the frontend report. Do not modify backend/docs or commit other workers' changes.

## Task 3: Integration, review, polish, and release-ready verification

**Owner:** controller and independently dispatched reviewers. **Files:** `docs/API.md`, `docs/USER_GUIDE.md`, `implementation-notes.md`, plus explicitly assigned fixes in reviewed files.

- [ ] **Step 1: Review backend and frontend slices against spec and quality.** Hand each reviewer the fixed wire contract, task report, and scoped diff. Resolve material findings with original implementers and rerun focused tests. Check public DTO parity explicitly rather than relying solely on each side's isolated tests. Record findings/resolutions in the ledger.
- [ ] **Step 2: Document the real shipped API and workflow.** Add endpoint request/response/limits to API docs, and inventory/show steps plus needs-review/no-comp distinction to the user guide. Update implementation notes with actual source verification, unavailable capabilities and deviations. Do not label the feature fully supported if the upstream evidence gate is unresolved.
- [ ] **Step 3: Exercise local real backend + disposable PostgreSQL.** Never connect the test runner to developer or production databases. Seed only disposable fixtures, start backend with local-only auth token, and call evaluate/list/add/pack/read/unpack/remove endpoints. Verify auth denial, surviving sale/refund/deletion states, snapshot/price mismatch behavior and no financial mutations. Inspect screenshots from frontend viewport tests; fix material defects and rerun.
- [ ] **Step 4: Run `polish-core --fix` against the implementation base `ae5b9e10`, inspect its edits, and request one independent whole-branch review.** Prioritize evidence correctness, API/DB concurrency, source assumptions, and UI contract. Apply approved high-confidence fixes and rerun affected verification.
- [ ] **Step 5: Fresh final gates and commit.** Run `go test -race -count=1 -timeout 10m ./...`, the explicit `POSTGRES_TEST_URL="${SHOWPREP_TEST_URL:?controller must supply the verified disposable database URL}" go test -race -count=1 -timeout 10m ./internal/adapters/storage/postgres/...` command from Task 1, frontend tests/typecheck/build, focused Playwright, `make check`, and `git diff --check`. Confirm the DB run is uncached and exercises independent-table cleanup and deletion-retention cases; include the real-server deadline regression and durable price-hold regression in the results. Review final diff and status for unrelated files/placeholders. Commit only reviewed work; do not push/merge without user authorization. Final explanation states what works, exact checks, local branch/worktree, and any remaining source/runtime limits.
