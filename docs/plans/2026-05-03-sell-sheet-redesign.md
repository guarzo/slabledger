# Sell Sheet Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the "select items → add to sell sheet → print one consolidated sheet" flow with a global page that always shows all received-but-unsold inventory and lets the user print one of seven pre-defined slices (PSA 10s, Modern 2020+, Vintage pre-2020, High-Value $1k+, Under $1k, By Grade, Full List).

**Architecture:** Backend keeps only `GenerateGlobalSellSheet`. Removes the per-campaign and "selected" sell-sheet endpoints, removes `sell_sheet_items` persistence (table + repo + handlers), and removes the "add to sell sheet" UX from the inventory views. Frontend gets a new `/sell-sheet` route that fetches the global sheet once and computes seven overlapping slices client-side via pure helper functions. The existing `SellSheetPrintRow` component (cert + barcode + CL value + blank Agreed Price column) is reused unchanged.

**Tech Stack:** Go 1.26 (hexagonal, pgx/v5, golang-migrate), React + TypeScript, Vite, React Router, TanStack Query, Vitest.

**Spec:** `docs/specs/2026-05-03-sell-sheet-redesign-design.md`

---

## File Structure

**Backend — created:**
- `internal/adapters/storage/postgres/migrations/000005_drop_sell_sheet_items.up.sql`
- `internal/adapters/storage/postgres/migrations/000005_drop_sell_sheet_items.down.sql`

**Backend — modified:**
- `internal/domain/export/service_sell_sheet.go` — remove `GenerateSellSheet` and `GenerateSelectedSellSheet`
- `internal/domain/export/service.go` — remove the matching interface methods
- `internal/domain/export/service_test.go` — remove tests for the deleted methods (keep `GenerateGlobalSellSheet` tests)
- `internal/adapters/httpserver/handlers/campaigns_analytics.go` — delete `HandleSellSheet` and `HandleSelectedSellSheet`
- `internal/adapters/httpserver/handlers/campaigns_analytics_test.go` — delete tests for the two removed handlers
- `internal/adapters/httpserver/routes.go` — drop `POST /api/portfolio/sell-sheet`, `POST /api/campaigns/{id}/sell-sheet`, and the four `/api/sell-sheet/items*` routes; keep `GET /api/sell-sheet`
- `internal/adapters/httpserver/router.go` — remove `sellSheetItemsHandler` field and wiring
- `internal/testutil/mocks/export_service.go` — drop the two removed methods from the mock
- `internal/testutil/mocks/export_sell_sheet_repo.go` — delete (no longer used)

**Backend — deleted:**
- `internal/adapters/httpserver/handlers/sell_sheet_items.go`
- `internal/adapters/httpserver/handlers/sell_sheet_items_test.go`
- `internal/adapters/storage/postgres/sellsheet_store.go`
- `internal/domain/inventory/repository_sellsheet.go`

**Frontend — created:**
- `web/src/react/utils/sellSheetSlices.ts` — pure slice helpers (filter + sort + totals)
- `web/src/react/utils/sellSheetSlices.test.ts` — unit tests
- `web/src/react/pages/SellSheetPage.tsx` — new global `/sell-sheet` page (slice menu + print view)
- `web/src/react/pages/SellSheetPage.test.tsx` — smoke test

**Frontend — modified:**
- `web/src/react/App.tsx` — add `/sell-sheet` route
- `web/src/react/components/Navigation.tsx` — add "Sell Sheet" nav link
- `web/src/js/api/campaignPurchases.ts` — drop `generateSellSheet`, `generateSelectedSellSheet`, and the four `*SellSheetItems` methods; switch `generateGlobalSellSheet` from POST to GET
- `web/src/react/queries/useCampaignQueries.ts` — `useGlobalSellSheet` already exists; remove invalidation queueing for the removed endpoints
- `web/src/react/queries/queryKeys.ts` — remove `sellSheetItems` key
- `web/src/react/pages/campaign-detail/InventoryTab.tsx` — remove `onAddToSellSheet`, `onRemoveFromSellSheet`, `pageSellSheetCount`, `sellSheetActive`, `MobileSellSheetView` rendering, `SellSheetModals` import, the `sell-sheet-print` block (campaign-scoped print)
- `web/src/react/pages/campaign-detail/inventory/InventoryHeader.tsx` — remove `SellSheetActions` usage and props
- `web/src/react/hooks/useSellSheet.ts` — delete (campaign-scoped persisted-selection hook)
- `web/src/react/hooks/useSellSheet.test.ts` — delete
- `web/src/react/hooks/index.ts` — drop `useSellSheet` re-export

**Frontend — deleted:**
- `web/src/react/pages/campaign-detail/SellSheetView.tsx`
- `web/src/react/pages/campaign-detail/inventory/MobileSellSheetView.tsx`
- `web/src/react/pages/campaign-detail/inventory/MobileSellSheetRow.tsx`

**Frontend — kept (no changes):**
- `web/src/react/pages/campaign-detail/inventory/SellSheetPrintRow.tsx`
- `web/src/react/pages/campaign-detail/inventory/SellSheetPrintRow.test.tsx`
- `web/src/styles/print-sell-sheet.css`
- `web/src/react/utils/sellSheetHelpers.tsx` (still used by `SellSheetPrintRow`)

**Verification at end of plan:**
- `go build ./...`
- `go test -race ./...`
- `cd web && npm run lint && npm run typecheck && npm test`

---

## Task 1: Backend — strip campaign-scoped and selected sell-sheet service methods

**Files:**
- Modify: `internal/domain/export/service.go` (interface)
- Modify: `internal/domain/export/service_sell_sheet.go` (remove two methods)
- Modify: `internal/domain/export/service_test.go` (drop their tests)
- Modify: `internal/testutil/mocks/export_service.go` (drop the two methods)

- [ ] **Step 1: Identify the exact interface declarations**

Run: `grep -n "GenerateSellSheet\|GenerateSelectedSellSheet\|GenerateGlobalSellSheet" internal/domain/export/service.go`

Expected: three method declarations on the `Service` interface.

- [ ] **Step 2: Remove `GenerateSellSheet` and `GenerateSelectedSellSheet` from the interface**

In `internal/domain/export/service.go`, delete the lines for those two methods. Keep `GenerateGlobalSellSheet`.

- [ ] **Step 3: Remove their implementations**

In `internal/domain/export/service_sell_sheet.go`, delete:
- The function `func (s *service) GenerateSellSheet(ctx context.Context, campaignID string, purchaseIDs []string) (*inventory.SellSheet, error)` (lines ~117–160 in current file)
- The function `func (s *service) GenerateSelectedSellSheet(ctx context.Context, purchaseIDs []string) (*inventory.SellSheet, error)` (lines ~188–215)

Keep `GenerateGlobalSellSheet`, `enrichSellSheetItem`, `recommendChannel`, `buildCrossCampaignSellSheet`, `computeRecommendation`, `computeTargetPrice`. `buildCrossCampaignSellSheet` is still used by `GenerateGlobalSellSheet` — keep it.

- [ ] **Step 4: Remove tests for the two deleted methods**

Run: `grep -n "TestGenerateSellSheet\|TestGenerateSelectedSellSheet\|GenerateSellSheet(\|GenerateSelectedSellSheet(" internal/domain/export/service_test.go`

Delete every test function that references `GenerateSellSheet(` (campaign-scoped) or `GenerateSelectedSellSheet(`. Keep `TestGenerateGlobalSellSheet*`.

- [ ] **Step 5: Update the export-service mock**

In `internal/testutil/mocks/export_service.go`, remove the `GenerateSellSheetFn`/`GenerateSelectedSellSheetFn` fields and the matching methods. Keep `GenerateGlobalSellSheetFn` and its method. Match the field+method idiom already in that file.

- [ ] **Step 6: Build and verify**

Run: `go build ./...`
Expected: PASS. Any failures are call sites in handlers — they get fixed in Task 2.

If only handler call sites fail, that's expected — proceed.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/export/service.go internal/domain/export/service_sell_sheet.go internal/domain/export/service_test.go internal/testutil/mocks/export_service.go
git commit -m "refactor(export): remove campaign-scoped and selected sell-sheet methods

Per the redesign, only GenerateGlobalSellSheet is retained.
GenerateSellSheet (campaign-scoped, selection-driven) and
GenerateSelectedSellSheet (cross-campaign, selection-driven) are
removed along with their tests and mock fields.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 2: Backend — remove the two HTTP handlers and routes

**Files:**
- Modify: `internal/adapters/httpserver/handlers/campaigns_analytics.go`
- Modify: `internal/adapters/httpserver/handlers/campaigns_analytics_test.go`
- Modify: `internal/adapters/httpserver/routes.go`

- [ ] **Step 1: Delete `HandleSellSheet`**

In `internal/adapters/httpserver/handlers/campaigns_analytics.go`, delete the entire `HandleSellSheet` function (the one at line ~134, comment `// HandleSellSheet handles POST /api/campaigns/{id}/sell-sheet.`).

- [ ] **Step 2: Delete `HandleSelectedSellSheet`**

Same file: delete `HandleSelectedSellSheet` (line ~197, comment `// HandleSelectedSellSheet handles POST /api/portfolio/sell-sheet.`).

Keep `HandleGlobalSellSheet`.

- [ ] **Step 3: Remove the matching test cases**

Run: `grep -n "HandleSellSheet\|HandleSelectedSellSheet" internal/adapters/httpserver/handlers/campaigns_analytics_test.go`

Delete each test that calls `HandleSellSheet(` or `HandleSelectedSellSheet(`. Keep tests for `HandleGlobalSellSheet`.

- [ ] **Step 4: Remove the routes**

In `internal/adapters/httpserver/routes.go`:
- Delete the line `mux.Handle("POST /api/portfolio/sell-sheet", authRoute(rt.campaignsHandler.HandleSelectedSellSheet))` (line ~111)
- Delete the line `mux.Handle("POST /api/campaigns/{id}/sell-sheet", authRoute(rt.campaignsHandler.HandleSellSheet))` (line ~149)
- Remove `"/api/campaigns/{id}/sell-sheet"` from any route lists later in the file (line ~354 area — there is a string list of route paths; delete that entry).

Keep `mux.Handle("GET /api/sell-sheet", authRoute(rt.campaignsHandler.HandleGlobalSellSheet))`. The current code at line 110 says `GET` — verify with `grep -n "/api/sell-sheet" internal/adapters/httpserver/routes.go`. If `HandleGlobalSellSheet` registers as `POST`, change it to `GET` here as part of this step (the new frontend uses GET).

- [ ] **Step 5: Verify `HandleGlobalSellSheet` is GET-compatible**

Open `internal/adapters/httpserver/handlers/campaigns_analytics.go` and look at `HandleGlobalSellSheet` (line ~181). It calls the service with no body input. If it currently reads/decodes a request body, remove that decode — there is no input. Method check: it should not insist on POST.

- [ ] **Step 6: Build and run server tests**

Run: `go build ./...`
Expected: PASS.

Run: `go test ./internal/adapters/httpserver/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/httpserver/handlers/campaigns_analytics.go internal/adapters/httpserver/handlers/campaigns_analytics_test.go internal/adapters/httpserver/routes.go
git commit -m "refactor(httpserver): remove campaign-scoped + selected sell-sheet routes

Drops POST /api/campaigns/{id}/sell-sheet and POST /api/portfolio/sell-sheet
along with their handlers and tests. Switches GET /api/sell-sheet to
omit body decoding.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 3: Backend — remove `sell_sheet_items` persistence layer

**Files:**
- Delete: `internal/adapters/httpserver/handlers/sell_sheet_items.go`
- Delete: `internal/adapters/httpserver/handlers/sell_sheet_items_test.go`
- Delete: `internal/adapters/storage/postgres/sellsheet_store.go`
- Delete: `internal/domain/inventory/repository_sellsheet.go`
- Delete: `internal/testutil/mocks/export_sell_sheet_repo.go`
- Modify: `internal/adapters/httpserver/router.go`
- Modify: `internal/adapters/httpserver/routes.go`

- [ ] **Step 1: Confirm no other consumers**

Run:
```bash
grep -rn "SellSheetItem\|sellSheetItem\|sell_sheet_items\|SellSheetStore\|SellSheetItemsHandler" internal/ | grep -v "SellSheetItem\b" | grep -v "_test.go"
```

If any non-trivial consumers exist outside the four files about to be deleted, stop and re-evaluate. The expected hits are: `router.go`, `routes.go`, the four deletion targets.

Note: `SellSheetItem` (the singular type, used in `SellSheet.Items`) is unrelated and stays. Filter it out as above.

- [ ] **Step 2: Delete the files**

```bash
git rm internal/adapters/httpserver/handlers/sell_sheet_items.go \
       internal/adapters/httpserver/handlers/sell_sheet_items_test.go \
       internal/adapters/storage/postgres/sellsheet_store.go \
       internal/domain/inventory/repository_sellsheet.go \
       internal/testutil/mocks/export_sell_sheet_repo.go
```

- [ ] **Step 3: Remove router wiring**

In `internal/adapters/httpserver/router.go`:
- Delete the field `sellSheetItemsHandler *handlers.SellSheetItemsHandler` (line ~45).
- Delete the assignment `rt.sellSheetItemsHandler = cfg.SellSheetItemsHandler` (line ~193).
- Find and remove `SellSheetItemsHandler` from the router config struct (look for `SellSheetItemsHandler` declarations in the same file — there is one in `cfg`).

Run: `grep -n "SellSheetItemsHandler\|sellSheetItemsHandler" internal/adapters/httpserver/router.go`
Expected after edits: no matches.

- [ ] **Step 4: Remove the routes**

In `internal/adapters/httpserver/routes.go`, delete the entire `if rt.sellSheetItemsHandler != nil ...` block (lines ~117–122) — the four `/api/sell-sheet/items*` routes.

- [ ] **Step 5: Find and remove main.go wiring**

Run: `grep -rn "SellSheetItemsHandler\|NewSellSheetItemsHandler\|SellSheetStore\|NewSellSheetStore" cmd/ internal/`

For each match outside the deletion targets (likely in `cmd/slabledger/main.go`):
- Delete the construction line (e.g. `sellSheetStore := postgres.NewSellSheetStore(db)`)
- Delete the handler construction (e.g. `sellSheetItemsHandler := handlers.NewSellSheetItemsHandler(sellSheetStore)`)
- Delete the field assignment in the router config (e.g. `SellSheetItemsHandler: sellSheetItemsHandler,`)

- [ ] **Step 6: Build**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 7: Run tests**

Run: `go test -race ./...`
Expected: PASS. If a domain `inventory` test referenced the old repo type, delete the offending test or update it to not depend on the removed type.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "refactor: remove sell_sheet_items persistence layer

Drops SellSheetItemsHandler, SellSheetStore, the inventory
SellSheetRepository interface, and their wiring + mocks. The new
sell-sheet flow operates entirely on inventory state (received +
unsold) — no per-user 'sheet' is persisted.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 4: Backend — drop the `sell_sheet_items` table via migration

**Files:**
- Create: `internal/adapters/storage/postgres/migrations/000005_drop_sell_sheet_items.up.sql`
- Create: `internal/adapters/storage/postgres/migrations/000005_drop_sell_sheet_items.down.sql`

- [ ] **Step 1: Verify next migration number**

Run: `ls internal/adapters/storage/postgres/migrations/ | sort | tail -8`

Expected newest pair: `000004_add_resolved_at_indexes.up.sql` / `.down.sql`. Next number: `000005`.

- [ ] **Step 2: Read the original CREATE TABLE for reference**

Run: `grep -A 20 "CREATE TABLE sell_sheet_items" internal/adapters/storage/postgres/migrations/000001_initial_schema.up.sql`

Capture the full `CREATE TABLE` for the `down` migration. It is referenced at line 617 of `000001_initial_schema.up.sql`.

- [ ] **Step 3: Write the up migration**

Create `internal/adapters/storage/postgres/migrations/000005_drop_sell_sheet_items.up.sql`:

```sql
-- Drop sell_sheet_items: the new sell-sheet flow operates on inventory
-- state (received + unsold) and does not persist a per-user sheet.

DROP INDEX IF EXISTS public.idx_sell_sheet_items_added_at;
DROP TABLE IF EXISTS public.sell_sheet_items CASCADE;
```

- [ ] **Step 4: Write the down migration**

Create `internal/adapters/storage/postgres/migrations/000005_drop_sell_sheet_items.down.sql`. Paste the original CREATE TABLE captured in Step 2, plus the index from `000003`. The down migration must restore the table exactly as `000003` left it. Example (verify against actual DDL):

```sql
-- Restore sell_sheet_items table.

CREATE TABLE IF NOT EXISTS public.sell_sheet_items (
    purchase_id TEXT PRIMARY KEY,
    added_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE public.sell_sheet_items ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.sell_sheet_items
    USING (true) WITH CHECK (true);

CREATE INDEX IF NOT EXISTS idx_sell_sheet_items_added_at
    ON public.sell_sheet_items USING btree (added_at);
```

(Adjust column types/order to match the original captured DDL.)

- [ ] **Step 5: Boot the server locally to run the migration**

```bash
go build -o slabledger ./cmd/slabledger
./slabledger &
sleep 3
kill %1
```

Expected: server logs show `migrated to version 5` (or similar — see existing log format with `grep -rn "migrated\|migration" internal/adapters/storage/postgres/`). No errors.

If the local DB does not have a `DATABASE_URL` configured, skip server boot and rely on CI to verify.

- [ ] **Step 6: Verify down migration is reversible (optional, local only)**

```bash
psql "$DATABASE_URL" -c "\d sell_sheet_items" 2>&1 | head
```

Expected after up: `relation "sell_sheet_items" does not exist`.

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/storage/postgres/migrations/000005_drop_sell_sheet_items.up.sql internal/adapters/storage/postgres/migrations/000005_drop_sell_sheet_items.down.sql
git commit -m "feat(db): drop sell_sheet_items table

Migration 000005. The redesigned sell sheet operates on inventory
state and persists nothing.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 5: Frontend — clean up the API client

**Files:**
- Modify: `web/src/js/api/campaignPurchases.ts`

- [ ] **Step 1: Remove unused interface methods**

In `web/src/js/api/campaignPurchases.ts`, delete from the `interface APIClient` block:
- `generateSellSheet(campaignId: string, purchaseIds: string[]): Promise<SellSheet>;`
- `generateSelectedSellSheet(purchaseIds: string[]): Promise<SellSheet>;`
- `getSellSheetItems(): Promise<{ purchaseIds: string[] }>;`
- `addSellSheetItems(purchaseIds: string[]): Promise<void>;`
- `removeSellSheetItems(purchaseIds: string[]): Promise<void>;`
- `clearSellSheetItems(): Promise<void>;`

Keep: `generateGlobalSellSheet(): Promise<SellSheet>;`.

- [ ] **Step 2: Remove the method implementations**

Delete from the file:
- `proto.generateSellSheet = ...`
- `proto.generateSelectedSellSheet = ...`
- `proto.getSellSheetItems = ...`
- `proto.addSellSheetItems = ...`
- `proto.removeSellSheetItems = ...`
- `proto.clearSellSheetItems = ...`
- The `// --- Sell sheet item persistence ---` comment block

- [ ] **Step 3: Switch `generateGlobalSellSheet` to GET**

Change:
```ts
proto.generateGlobalSellSheet = async function (this: APIClient): Promise<SellSheet> {
  return this.post<SellSheet>('/sell-sheet', {});
};
```
To:
```ts
proto.generateGlobalSellSheet = async function (this: APIClient): Promise<SellSheet> {
  return this.get<SellSheet>('/sell-sheet');
};
```

- [ ] **Step 4: Typecheck**

Run: `cd web && npm run typecheck`

Expected: errors at every consumer of the removed methods. They get fixed in Tasks 6–9. If errors are restricted to those files, proceed.

- [ ] **Step 5: Commit**

```bash
git add web/src/js/api/campaignPurchases.ts
git commit -m "refactor(api): drop campaign-scoped + persisted sell-sheet methods

Removes generateSellSheet, generateSelectedSellSheet, and the four
*SellSheetItems methods. Switches generateGlobalSellSheet to GET.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 6: Frontend — write the slice helpers (TDD)

**Files:**
- Create: `web/src/react/utils/sellSheetSlices.ts`
- Create: `web/src/react/utils/sellSheetSlices.test.ts`

- [ ] **Step 1: Write the failing tests**

Create `web/src/react/utils/sellSheetSlices.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { computeSlices, parseCardYear, type SliceID } from './sellSheetSlices';
import type { SellSheetItem } from '../../types/campaigns';

function item(overrides: Partial<SellSheetItem> = {}): SellSheetItem {
  return {
    purchaseId: 'p1',
    certNumber: '12345',
    cardName: 'Charizard',
    setName: 'Base Set',
    cardNumber: '4',
    grade: 9,
    grader: 'PSA',
    buyCostCents: 10000,
    costBasisCents: 11000,
    clValueCents: 20000,
    recommendation: 'stable',
    targetSellPrice: 50000,
    minimumAcceptPrice: 45000,
    ...overrides,
  };
}

describe('parseCardYear', () => {
  it('extracts a leading 4-digit year', () => {
    expect(parseCardYear('1999')).toBe(1999);
    expect(parseCardYear('1999-2000')).toBe(1999);
    expect(parseCardYear('2022 Pokemon')).toBe(2022);
  });

  it('returns null when no leading 4-digit run', () => {
    expect(parseCardYear('')).toBeNull();
    expect(parseCardYear(undefined)).toBeNull();
    expect(parseCardYear('Pokemon 1999')).toBeNull(); // not at start
    expect(parseCardYear('99-00')).toBeNull();
  });
});

describe('computeSlices', () => {
  const all: SellSheetItem[] = [
    item({ purchaseId: 'a', grader: 'PSA', grade: 10, targetSellPrice: 150000 }), // PSA10, high
    item({ purchaseId: 'b', grader: 'PSA', grade: 10, targetSellPrice: 50000 }),  // PSA10, low
    item({ purchaseId: 'c', grader: 'BGS', grade: 10, targetSellPrice: 80000 }),  // BGS 10 — NOT a PSA10
    item({ purchaseId: 'd', grader: 'PSA', grade: 9,  targetSellPrice: 200000 }), // high-value, no PSA10
  ];

  it('PSA10s slice: only PSA grader at grade 10, sorted by price desc', () => {
    const slices = computeSlices(all);
    const ids = slices.psa10.items.map((i) => i.purchaseId);
    expect(ids).toEqual(['a', 'b']);
  });

  it('PSA10s totals reflect the filtered subset', () => {
    const slices = computeSlices(all);
    expect(slices.psa10.itemCount).toBe(2);
    expect(slices.psa10.totalAskCents).toBe(150000 + 50000);
  });

  it('high-value slice: targetSellPrice >= $1000 (100000c), sorted desc', () => {
    const slices = computeSlices(all);
    const ids = slices.highValue.items.map((i) => i.purchaseId);
    expect(ids).toEqual(['d', 'a']);
  });

  it('under-1k slice: targetSellPrice < 100000c', () => {
    const slices = computeSlices(all);
    const ids = slices.underOneK.items.map((i) => i.purchaseId);
    expect(ids).toEqual(['c', 'b']); // 80000 then 50000, desc
  });

  it('era split uses cardYear regex; missing year excluded from both eras', () => {
    const items: SellSheetItem[] = [
      item({ purchaseId: 'm1', cardYear: '2022', setName: 'B', cardNumber: '2' }),
      item({ purchaseId: 'm2', cardYear: '2020-2021', setName: 'A', cardNumber: '1' }),
      item({ purchaseId: 'v1', cardYear: '1999', setName: 'C', cardNumber: '4' }),
      item({ purchaseId: 'v2', cardYear: '2019', setName: 'D', cardNumber: '3' }),
      item({ purchaseId: 'x', cardYear: '' }), // unparseable
    ];
    const slices = computeSlices(items);
    expect(slices.modern.items.map((i) => i.purchaseId)).toEqual(['m2', 'm1']); // set A then B
    expect(slices.vintage.items.map((i) => i.purchaseId)).toEqual(['v1', 'v2']); // set C then D
    expect(slices.unparseableYearCount).toBe(1);
  });

  it('byGrade slice: all items, grade desc then price desc', () => {
    const slices = computeSlices(all);
    const ids = slices.byGrade.items.map((i) => i.purchaseId);
    expect(ids).toEqual(['a', 'c', 'b', 'd']); // grade 10s (a=150k, c=80k, b=50k), then grade 9 (d=200k)
  });

  it('full slice: all items, set asc then card number asc', () => {
    const items: SellSheetItem[] = [
      item({ purchaseId: '1', setName: 'B', cardNumber: '1' }),
      item({ purchaseId: '2', setName: 'A', cardNumber: '10' }),
      item({ purchaseId: '3', setName: 'A', cardNumber: '2' }),
    ];
    const slices = computeSlices(items);
    expect(slices.full.items.map((i) => i.purchaseId)).toEqual(['3', '2', '1']);
  });

  it('overall total reflects the full input', () => {
    const slices = computeSlices(all);
    expect(slices.totalItemCount).toBe(4);
    expect(slices.totalAskCents).toBe(150000 + 50000 + 80000 + 200000);
  });
});
```

- [ ] **Step 2: Run the failing test**

Run: `cd web && npx vitest run src/react/utils/sellSheetSlices.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `sellSheetSlices.ts`**

Create `web/src/react/utils/sellSheetSlices.ts`:

```ts
import type { SellSheetItem } from '../../types/campaigns';

export type SliceID =
  | 'psa10'
  | 'modern'
  | 'vintage'
  | 'highValue'
  | 'underOneK'
  | 'byGrade'
  | 'full';

export interface SliceResult {
  id: SliceID;
  label: string;
  description: string;
  items: SellSheetItem[];
  itemCount: number;
  totalAskCents: number;
}

export interface SliceSet {
  psa10: SliceResult;
  modern: SliceResult;
  vintage: SliceResult;
  highValue: SliceResult;
  underOneK: SliceResult;
  byGrade: SliceResult;
  full: SliceResult;
  totalItemCount: number;
  totalAskCents: number;
  unparseableYearCount: number;
}

const HIGH_VALUE_CENTS = 100000;
const ERA_CUTOFF_YEAR = 2020;
const LEADING_YEAR_RE = /^(\d{4})/;

export function parseCardYear(input: string | undefined | null): number | null {
  if (!input) return null;
  const m = LEADING_YEAR_RE.exec(input);
  return m ? parseInt(m[1], 10) : null;
}

function totals(items: SellSheetItem[]): { itemCount: number; totalAskCents: number } {
  return {
    itemCount: items.length,
    totalAskCents: items.reduce((sum, i) => sum + (i.targetSellPrice ?? 0), 0),
  };
}

function byPriceDesc(a: SellSheetItem, b: SellSheetItem): number {
  return (b.targetSellPrice ?? 0) - (a.targetSellPrice ?? 0);
}

function bySetThenNumber(a: SellSheetItem, b: SellSheetItem): number {
  const setCmp = (a.setName ?? '').localeCompare(b.setName ?? '');
  if (setCmp !== 0) return setCmp;
  // Numeric-aware compare for card numbers like "4", "4a", "150"
  return (a.cardNumber ?? '').localeCompare(b.cardNumber ?? '', undefined, { numeric: true });
}

function byGradeThenPriceDesc(a: SellSheetItem, b: SellSheetItem): number {
  const g = (b.grade ?? 0) - (a.grade ?? 0);
  if (g !== 0) return g;
  return byPriceDesc(a, b);
}

function makeSlice(
  id: SliceID,
  label: string,
  description: string,
  items: SellSheetItem[],
): SliceResult {
  const t = totals(items);
  return { id, label, description, items, ...t };
}

export function computeSlices(input: SellSheetItem[]): SliceSet {
  const psa10Items = input
    .filter((i) => i.grader === 'PSA' && i.grade === 10)
    .slice()
    .sort(byPriceDesc);

  const modernItems: SellSheetItem[] = [];
  const vintageItems: SellSheetItem[] = [];
  let unparseableYearCount = 0;
  for (const it of input) {
    const yr = parseCardYear(it.cardYear);
    if (yr === null) {
      unparseableYearCount++;
      continue;
    }
    if (yr >= ERA_CUTOFF_YEAR) modernItems.push(it);
    else vintageItems.push(it);
  }
  modernItems.sort(bySetThenNumber);
  vintageItems.sort(bySetThenNumber);

  const highValueItems = input
    .filter((i) => (i.targetSellPrice ?? 0) >= HIGH_VALUE_CENTS)
    .slice()
    .sort(byPriceDesc);

  const underOneKItems = input
    .filter((i) => (i.targetSellPrice ?? 0) < HIGH_VALUE_CENTS)
    .slice()
    .sort(byPriceDesc);

  const byGradeItems = input.slice().sort(byGradeThenPriceDesc);

  const fullItems = input.slice().sort(bySetThenNumber);

  const overall = totals(input);

  return {
    psa10: makeSlice('psa10', 'PSA 10s', 'Every PSA 10, priced high to low', psa10Items),
    modern: makeSlice('modern', `Modern (${ERA_CUTOFF_YEAR}+)`, `Cards from ${ERA_CUTOFF_YEAR} or later, by set`, modernItems),
    vintage: makeSlice('vintage', `Vintage (pre-${ERA_CUTOFF_YEAR})`, `Cards before ${ERA_CUTOFF_YEAR}, by set`, vintageItems),
    highValue: makeSlice('highValue', 'High-Value ($1,000+)', 'Cards asking $1,000 or more', highValueItems),
    underOneK: makeSlice('underOneK', 'Under $1,000', 'Cards asking under $1,000', underOneKItems),
    byGrade: makeSlice('byGrade', 'By Grade (local card store)', 'Sorted grade desc, then price', byGradeItems),
    full: makeSlice('full', 'Full List', 'Every item, by set', fullItems),
    totalItemCount: overall.itemCount,
    totalAskCents: overall.totalAskCents,
    unparseableYearCount,
  };
}
```

Note: `cardYear` is not declared on the existing `SellSheetItem` interface. The Go struct has `CardYear` — confirm it is included in `enrichSellSheetItem` output (currently it's not — see Step 4).

- [ ] **Step 4: Add `cardYear` to the backend `SellSheetItem` and the frontend type**

Run: `grep -n "cardYear" web/src/types/campaigns/market.ts`
Expected: no match.

Add `cardYear?: string;` to `SellSheetItem` in `web/src/types/campaigns/market.ts` near `grade`/`grader`.

Locate the Go `SellSheetItem` struct: `grep -rn "type SellSheetItem struct" internal/domain/inventory/`. Add `CardYear string \`json:"cardYear,omitempty"\`` field.

In `internal/domain/export/service_sell_sheet.go`, in `enrichSellSheetItem`, set `item.CardYear = purchase.CardYear` near the other field assignments at the top.

- [ ] **Step 5: Run the test, expect pass**

Run: `cd web && npx vitest run src/react/utils/sellSheetSlices.test.ts`
Expected: PASS, all 8 tests.

- [ ] **Step 6: Backend build + test**

Run: `go build ./... && go test ./internal/domain/export/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src/react/utils/sellSheetSlices.ts web/src/react/utils/sellSheetSlices.test.ts web/src/types/campaigns/market.ts internal/domain/inventory/types_core.go internal/domain/export/service_sell_sheet.go
git commit -m "feat(sell-sheet): add slice helpers + cardYear plumbing

Pure helper computeSlices() returns the seven sell-sheet slices
(PSA 10s, Modern, Vintage, High-Value, Under \$1k, By Grade, Full)
with totals. Backend SellSheetItem now exposes cardYear (already on
Purchase) so the frontend can split modern vs vintage.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 7: Frontend — build the new global Sell Sheet page

**Files:**
- Create: `web/src/react/pages/SellSheetPage.tsx`
- Create: `web/src/react/pages/SellSheetPage.test.tsx`

- [ ] **Step 1: Write the smoke test first**

Create `web/src/react/pages/SellSheetPage.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import SellSheetPage from './SellSheetPage';
import type { SellSheet } from '../../types/campaigns';

vi.mock('../../js/api', () => ({
  api: {
    generateGlobalSellSheet: vi.fn(async (): Promise<SellSheet> => ({
      generatedAt: '2026-05-03T00:00:00Z',
      campaignName: 'All Inventory',
      items: [
        {
          purchaseId: 'p1', certNumber: '1', cardName: 'Charizard', setName: 'Base',
          cardNumber: '4', grade: 10, grader: 'PSA', buyCostCents: 0, costBasisCents: 0,
          clValueCents: 100000, recommendation: 'stable', targetSellPrice: 150000,
          minimumAcceptPrice: 100000, cardYear: '1999',
        },
      ],
      totals: {
        totalCostBasis: 0, totalExpectedRevenue: 150000, totalProjectedProfit: 150000,
        itemCount: 1, skippedItems: 0,
      },
    })),
  },
}));

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <SellSheetPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('SellSheetPage', () => {
  it('renders all seven slice options with counts after data loads', async () => {
    renderPage();
    expect(await screen.findByText(/PSA 10s/)).toBeInTheDocument();
    expect(screen.getByText(/Modern \(2020\+\)/)).toBeInTheDocument();
    expect(screen.getByText(/Vintage \(pre-2020\)/)).toBeInTheDocument();
    expect(screen.getByText(/High-Value/)).toBeInTheDocument();
    expect(screen.getByText(/Under \$1,000/)).toBeInTheDocument();
    expect(screen.getByText(/By Grade/)).toBeInTheDocument();
    expect(screen.getByText(/Full List/)).toBeInTheDocument();
  });

  it('opens the print view when a slice is selected', async () => {
    const user = userEvent.setup();
    renderPage();
    const printBtn = await screen.findAllByRole('button', { name: /^Print$/i });
    await user.click(printBtn[0]); // first slice
    expect(screen.getByTestId('sell-sheet-print-view')).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run, expect fail**

Run: `cd web && npx vitest run src/react/pages/SellSheetPage.test.tsx`
Expected: FAIL (module not found).

- [ ] **Step 3: Implement `SellSheetPage.tsx`**

Create `web/src/react/pages/SellSheetPage.tsx`:

```tsx
import { useEffect, useMemo, useState } from 'react';
import { useGlobalSellSheet } from '../queries/useCampaignQueries';
import { computeSlices, type SliceID, type SliceResult } from '../utils/sellSheetSlices';
import SellSheetPrintRow from './campaign-detail/inventory/SellSheetPrintRow';
import type { AgingItem } from '../../types/campaigns';
import type { SellSheetItem } from '../../types/campaigns';
import '../../styles/print-sell-sheet.css';

function dollars(cents: number): string {
  return `$${(cents / 100).toLocaleString('en-US', { minimumFractionDigits: 0, maximumFractionDigits: 0 })}`;
}

// Adapt a SellSheetItem into the shape SellSheetPrintRow expects (AgingItem with purchase + recommendedPriceCents).
function asAgingItem(item: SellSheetItem): AgingItem {
  return {
    purchase: {
      id: item.purchaseId ?? '',
      certNumber: item.certNumber,
      cardName: item.cardName,
      setName: item.setName,
      cardNumber: item.cardNumber,
      gradeValue: item.grade,
      grader: item.grader ?? '',
      clValueCents: item.clValueCents,
    } as AgingItem['purchase'],
    recommendedPriceCents: item.targetSellPrice,
  } as AgingItem;
}

interface PrintViewProps {
  slice: SliceResult;
  onBack: () => void;
}

function PrintView({ slice, onBack }: PrintViewProps) {
  useEffect(() => {
    // Trigger the print dialog after the next paint so the print view renders.
    const t = setTimeout(() => window.print(), 100);
    return () => clearTimeout(t);
  }, []);

  return (
    <div data-testid="sell-sheet-print-view">
      <div className="no-print mb-3 flex items-center gap-3">
        <button onClick={onBack} className="px-3 py-1 border rounded">Back</button>
        <button onClick={() => window.print()} className="px-3 py-1 border rounded">Print</button>
      </div>
      <div className="sell-sheet-print">
        <div className="sell-sheet-print-header">
          <h1>Sell Sheet — {slice.label}</h1>
          <div className="sell-sheet-print-meta">
            {slice.itemCount} cards · {dollars(slice.totalAskCents)} total ask
          </div>
        </div>
        <div className="sell-sheet-print-thead">
          <div className="sell-sheet-print-cell" data-cell="num">#</div>
          <div className="sell-sheet-print-cell" data-cell="card">Card</div>
          <div className="sell-sheet-print-cell" data-cell="grade">Grade</div>
          <div className="sell-sheet-print-cell" data-cell="cert">Cert</div>
          <div className="sell-sheet-print-cell" data-cell="cl">CL</div>
          <div className="sell-sheet-print-cell" data-cell="agreed">Agreed Price</div>
        </div>
        {slice.items.map((it, idx) => (
          <SellSheetPrintRow key={it.purchaseId ?? idx} item={asAgingItem(it)} rowNumber={idx + 1} />
        ))}
      </div>
    </div>
  );
}

export default function SellSheetPage() {
  const { data, isLoading, error } = useGlobalSellSheet();
  const [activeSliceId, setActiveSliceId] = useState<SliceID | null>(null);

  const slices = useMemo(
    () => (data ? computeSlices(data.items) : null),
    [data],
  );

  if (isLoading) return <div className="p-6">Loading inventory…</div>;
  if (error) return <div className="p-6 text-red-600">Failed to load sell sheet.</div>;
  if (!slices) return null;

  if (activeSliceId) {
    const slice = slices[activeSliceId];
    return <PrintView slice={slice} onBack={() => setActiveSliceId(null)} />;
  }

  const order: SliceID[] = ['psa10', 'modern', 'vintage', 'highValue', 'underOneK', 'byGrade', 'full'];

  return (
    <div className="p-6 max-w-3xl mx-auto">
      <header className="mb-6">
        <h1 className="text-2xl font-semibold">Sell Sheet</h1>
        <div className="text-sm text-[var(--text-muted)] mt-1">
          All Inventory · {slices.totalItemCount} cards in hand · {dollars(slices.totalAskCents)} total ask
        </div>
        {slices.unparseableYearCount > 0 && (
          <div className="text-xs text-[var(--text-muted)] mt-1">
            {slices.unparseableYearCount} cards have no parseable year and were excluded from the era slices.
          </div>
        )}
      </header>

      <ul className="divide-y border rounded">
        {order.map((id) => {
          const s = slices[id];
          return (
            <li key={id} className="flex items-center justify-between p-4">
              <div>
                <div className="font-medium">{s.label}</div>
                <div className="text-sm text-[var(--text-muted)]">
                  {s.itemCount} · {dollars(s.totalAskCents)}
                </div>
                <div className="text-xs text-[var(--text-muted)] mt-0.5">{s.description}</div>
              </div>
              <button
                onClick={() => setActiveSliceId(id)}
                className="px-4 py-1.5 rounded bg-[var(--brand-500)] text-white disabled:opacity-50"
                disabled={s.itemCount === 0}
              >
                Print
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
```

- [ ] **Step 4: Verify the api singleton path**

Run: `grep -rn "from.*'\.\./js/api'\|from.*'\.\./\.\./js/api'" web/src/react/ | head -5`

Confirm the import in `SellSheetPage.tsx` (`'../../js/api'`) matches conventions. If the project uses `'../../js/api/client'` instead, update the test mock and component import accordingly.

- [ ] **Step 5: Verify `SellSheetPrintRow` props compatibility**

Run: `grep -n "interface Props\|item: AgingItem" web/src/react/pages/campaign-detail/inventory/SellSheetPrintRow.tsx`
Expected: `item: AgingItem;` and `rowNumber: number;`. If different, adjust `asAgingItem` and the print row call to match.

- [ ] **Step 6: Run the test**

Run: `cd web && npx vitest run src/react/pages/SellSheetPage.test.tsx`
Expected: PASS.

- [ ] **Step 7: Typecheck**

Run: `cd web && npm run typecheck`
Expected: PASS for `SellSheetPage.tsx` and helpers. Pre-existing errors elsewhere are addressed in later tasks.

- [ ] **Step 8: Commit**

```bash
git add web/src/react/pages/SellSheetPage.tsx web/src/react/pages/SellSheetPage.test.tsx
git commit -m "feat(sell-sheet): global SellSheetPage with slice menu + print view

New /sell-sheet page (route added in next task). Lists the seven
slices with counts/totals, opens a print-ready view per slice that
reuses SellSheetPrintRow.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 8: Frontend — wire the route + nav link

**Files:**
- Modify: `web/src/react/App.tsx`
- Modify: `web/src/react/components/Navigation.tsx`

- [ ] **Step 1: Add the route**

In `web/src/react/App.tsx`, just below the `/inventory` route (`<Route path="/inventory" element={...}>`), add:

```tsx
<Route path="/sell-sheet" element={
  <ProtectedRoute>
    <SellSheetPage />
  </ProtectedRoute>
} />
```

Add the import at the top:

```tsx
import SellSheetPage from './pages/SellSheetPage';
```

Mirror the surrounding route style — copy the wrapping (`ProtectedRoute`, layout, etc.) of an adjacent route exactly.

- [ ] **Step 2: Add the nav link**

In `web/src/react/components/Navigation.tsx`, append to the `navItems` array:

```ts
{ path: '/sell-sheet', label: 'Sell Sheet', shortLabel: 'Sheet' },
```

Position it after `Inventory` for logical grouping.

- [ ] **Step 3: Build + typecheck**

Run: `cd web && npm run typecheck && npm run build`
Expected: PASS.

- [ ] **Step 4: Manual smoke (optional)**

```bash
cd web && npm run dev &
# In another shell: curl http://localhost:5173/sell-sheet
```

Visit `/sell-sheet`, confirm the menu renders. Kill the dev server when done.

- [ ] **Step 5: Commit**

```bash
git add web/src/react/App.tsx web/src/react/components/Navigation.tsx
git commit -m "feat(sell-sheet): add /sell-sheet route and nav link

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 9: Frontend — strip the old in-campaign sell-sheet UX

**Files:**
- Modify: `web/src/react/pages/campaign-detail/InventoryTab.tsx`
- Modify: `web/src/react/pages/campaign-detail/inventory/InventoryHeader.tsx`
- Modify: `web/src/react/queries/useCampaignQueries.ts`
- Modify: `web/src/react/queries/queryKeys.ts`
- Modify: `web/src/react/hooks/index.ts`
- Delete: `web/src/react/hooks/useSellSheet.ts`
- Delete: `web/src/react/hooks/useSellSheet.test.ts`
- Delete: `web/src/react/pages/campaign-detail/SellSheetView.tsx`
- Delete: `web/src/react/pages/campaign-detail/inventory/MobileSellSheetView.tsx`
- Delete: `web/src/react/pages/campaign-detail/inventory/MobileSellSheetRow.tsx`

- [ ] **Step 1: Remove `useSellSheet` from `hooks/index.ts`**

Delete the line `export { useSellSheet } from './useSellSheet';`.

- [ ] **Step 2: Remove the persisted-items query key**

In `web/src/react/queries/queryKeys.ts`, delete the line:
```ts
sellSheetItems: ['portfolio', 'sellSheetItems'] as const,
```

- [ ] **Step 3: Drop dead invalidations in `useCampaignQueries.ts`**

Run: `grep -n "sellSheetItems" web/src/react/queries/useCampaignQueries.ts`. Delete each match. Keep `queryKeys.portfolio.sellSheet` (used by `useGlobalSellSheet`).

- [ ] **Step 4: Delete the old hook and its test**

```bash
git rm web/src/react/hooks/useSellSheet.ts web/src/react/hooks/useSellSheet.test.ts
```

- [ ] **Step 5: Delete the campaign-scoped views**

```bash
git rm web/src/react/pages/campaign-detail/SellSheetView.tsx \
       web/src/react/pages/campaign-detail/inventory/MobileSellSheetView.tsx \
       web/src/react/pages/campaign-detail/inventory/MobileSellSheetRow.tsx
```

- [ ] **Step 6: Strip the consumers in `InventoryTab.tsx`**

In `web/src/react/pages/campaign-detail/InventoryTab.tsx`:
- Remove imports: `MobileSellSheetView`, `SellSheetModals`, anything from the deleted hook.
- Remove the destructured names `pageSellSheetCount`, `sellSheetActive`, `sellSheet` from whatever `useInventoryTab()` (or similar) returns. Track those down to their source hook and delete them there too.
- Remove the `<MobileSellSheetView ... />` rendering block.
- Remove the `<div className="sell-sheet-print">` block (campaign-scoped print).
- Remove the `onAddToSellSheet`, `onRemoveFromSellSheet` props passed to inventory tables.

This task is the messy one. After each removal, run `npm run typecheck` and chase the next compile error.

- [ ] **Step 7: Strip `InventoryHeader.tsx`**

Remove the `import { SellSheetActions } from '../SellSheetView';` and any usage of `<SellSheetActions ... />`. Remove related props from `InventoryHeaderProps`.

Cascade: any caller of `InventoryHeader` that passed sell-sheet props (e.g. `onClearSellSheet`) — remove those passes.

- [ ] **Step 8: Find and remove orphaned add-to-sell-sheet callers**

Run: `grep -rn "onAddToSellSheet\|onRemoveFromSellSheet\|sellSheetActive\|pageSellSheetCount\|SellSheetActions\|SellSheetModals\|useSellSheet\b" web/src/`

Each match must be either deleted (the consumer) or fixed (a downstream prop). Continue until grep is empty.

- [ ] **Step 9: Typecheck + lint + tests**

Run: `cd web && npm run typecheck && npm run lint && npm test`
Expected: PASS.

If a test deep-imports `SellSheetView` or `useSellSheet`, delete the test (the surface is gone) or rewrite it against `SellSheetPage`/`computeSlices`.

- [ ] **Step 10: Commit**

```bash
git add -A
git commit -m "refactor(sell-sheet): remove campaign-scoped sell-sheet UX

Drops SellSheetView, SellSheetActions, SellSheetModals,
MobileSellSheetView, MobileSellSheetRow, useSellSheet, and the
add/remove plumbing in InventoryTab + InventoryHeader. The new
global /sell-sheet page is the only sell-sheet entry point.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 10: Final verification

- [ ] **Step 1: Backend — full test + race**

Run: `go test -race -timeout 10m ./...`
Expected: PASS.

- [ ] **Step 2: Backend — quality checks**

Run: `make check`
Expected: PASS (lint, architecture imports, file size).

- [ ] **Step 3: Frontend — full test + lint + typecheck + build**

Run: `cd web && npm run lint && npm run typecheck && npm test && npm run build`
Expected: PASS.

- [ ] **Step 4: Boot the app and click through**

```bash
go build -o slabledger ./cmd/slabledger
./slabledger &
sleep 3
cd web && npm run dev &
```

In a browser:
- Visit `/sell-sheet` — confirm the seven-slice menu loads with counts.
- Click `Print` on `PSA 10s` — confirm the print view renders and the browser print dialog opens.
- Visit `/campaigns/<id>` (any campaign) — confirm there is no longer an "Add to sell sheet" or "Print sell sheet" button anywhere on the inventory tab.

Kill both processes when satisfied.

- [ ] **Step 5: Push the branch**

```bash
git push -u origin sell-sheet-redesign
```

- [ ] **Step 6: Open a PR**

```bash
gh pr create --title "Redesign sell sheet: global slice menu" --body "$(cat <<'EOF'
## Summary
- Replace the select-and-add sell-sheet flow with a global /sell-sheet page that always shows all received-but-unsold inventory.
- Print one of seven pre-defined slices: PSA 10s, Modern (2020+), Vintage (pre-2020), High-Value (\$1,000+), Under \$1,000, By Grade (local card store), Full List.
- Drop the campaign-scoped and selected sell-sheet endpoints, the sell_sheet_items table, and all "add to sell sheet" UX.
- Spec: docs/specs/2026-05-03-sell-sheet-redesign-design.md

## Test plan
- [ ] go test -race ./... passes
- [ ] make check passes
- [ ] web npm test + typecheck + lint pass
- [ ] /sell-sheet renders all seven slices with correct counts on real data
- [ ] Each slice's Print button opens a print-ready view that reuses SellSheetPrintRow (cert + barcode + CL value + Agreed Price column)
- [ ] No "Add to sell sheet" UI remains on campaign detail inventory tab
- [ ] Migration 000005 applies cleanly on app boot

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-Review (post-write)

**Spec coverage:**
- Global page at `/sell-sheet` → Tasks 7–8.
- Per-campaign sell-sheet removal → Tasks 1, 2, 9.
- Slice menu with counts/totals + print views → Tasks 6, 7.
- Seven slice definitions (PSA 10s, Modern 2020+, Vintage, High-Value, Under $1k, By Grade, Full) → Task 6.
- Era split via `cardYear` regex with unparseable footer → Task 6 (Steps 3–4 add `cardYear`).
- Backend keeps `GenerateGlobalSellSheet` only → Tasks 1–3.
- `sell_sheet_items` removal → Tasks 3 (code) + 4 (migration).
- Reuse `SellSheetPrintRow` unchanged → Task 7 (uses it via `asAgingItem` adapter).
- Slicing client-side → Task 6 helpers, Task 7 page.
- Tests for slice helpers + page → Tasks 6, 7.

All spec sections covered.

**Placeholder scan:** No "TBD"/"TODO"/"add appropriate handling"/"similar to" markers. Code samples are complete. Commands are exact.

**Type consistency:**
- `SliceID`, `SliceResult`, `SliceSet` defined in Task 6 are referenced verbatim in Task 7.
- `computeSlices` signature matches between definition and usage.
- `cardYear` is added to both Go `SellSheetItem` and TS `SellSheetItem` in Task 6 Step 4 before any consumer reads it.
- `useGlobalSellSheet` is referenced in Task 7 — already exists in `useCampaignQueries.ts` per pre-plan exploration.
- `SellSheetPrintRow` props (`item: AgingItem`, `rowNumber: number`) are validated in Task 7 Step 5.

**Known thin spot:** Task 9 Step 6 ("messy one") relies on the implementer reading actual call sites and chasing typecheck errors. This is unavoidable given how `useInventoryTab` may transitively expose sell-sheet state — listing every line up-front would risk going stale. The task gives the search command (`grep -rn "onAddToSellSheet|..."`) and a clear stopping condition (grep returns empty).

Plan is ready.
