# Orders Sales Import — Design Spec

Import sales from an orders export CSV (Shopify/eBay) by matching PSA cert numbers against existing inventory purchases. Replaces manual sale entry for high-volume order periods.

## Scope

- PSA cards with cert numbers only — CGC, ungraded, and missing-cert rows are skipped
- Sales import only — no purchase creation
- Two-phase flow: upload + review, then confirm

## CSV Format

| Column | Example | Usage |
|--------|---------|-------|
| Order | #1001 | Metadata only (not stored) |
| Date | 2026-03-09 | Sale date |
| Sales Channel | eBay, Online Store | Mapped to SaleChannel |
| Product Title | Dark Gengar Holo - Neo Destiny - #6 PSA 5 | Display in review UI |
| Grading Company | PSA, CGC, (empty) | Filter: only PSA processed |
| Cert Number | 194544353 | Match key against campaign_purchases |
| Grade | 5 | Display/validation |
| Qty | 1 | Ignored (always 1 per graded card) |
| Unit Price | 259.35 | Sale price (dollars, converted to cents) |
| Line Subtotal | 259.35 | Ignored (same as Unit Price for qty 1) |

### Channel Mapping

| CSV Value | SaleChannel | Fee |
|-----------|-------------|-----|
| eBay | `ebay` | 12.35% (campaign-configurable) |
| Online Store | `website` | 3% |
| (unknown) | — | Row skipped |

## Data Model

### New Types (`internal/domain/campaigns/import_types.go`)

```go
type OrdersExportRow struct {
    OrderNumber  string
    Date         string      // YYYY-MM-DD
    SalesChannel SaleChannel // Mapped from CSV
    ProductTitle string
    Grader       string
    CertNumber   string
    Grade        float64
    UnitPrice    float64     // Dollars
}

type OrdersImportResult struct {
    Matched     []OrdersImportMatch
    AlreadySold []OrdersImportSkip
    NotFound    []OrdersImportSkip
    Skipped     []OrdersImportSkip
}

type OrdersImportMatch struct {
    CertNumber     string
    ProductTitle   string
    SaleChannel    SaleChannel
    SaleDate       string
    SalePriceCents int
    SaleFeeCents   int
    PurchaseID     string
    CampaignID     string
    CardName       string
    BuyCostCents   int
    NetProfitCents int // Preview: salePrice - buyCost - sourcingFee - saleFee
}

type OrdersImportSkip struct {
    CertNumber   string
    ProductTitle string
    Reason       string // "already_sold", "not_found", "duplicate", "not_psa", "unknown_channel"
}

type OrdersConfirmItem struct {
    PurchaseID     string
    SaleChannel    SaleChannel
    SaleDate       string
    SalePriceCents int
}
```

## Backend

### Parser (`internal/domain/campaigns/parse_orders.go`)

- Reads 10-column CSV with header row
- Maps "Sales Channel": `"eBay"` → `ebay`, `"Online Store"` → `website`, unknown → skip
- Filters: skip rows where `Grader != "PSA"` or `CertNumber` is empty
- Deduplicates by cert number — first occurrence wins, subsequent marked as `"duplicate"`
- Converts `UnitPrice` from dollars to cents

### Service Methods

**`ImportOrdersSales(ctx, rows []OrdersExportRow) (*OrdersImportResult, error)`**

1. Batch-fetch all purchases by cert numbers (single DB query via existing `GetPurchasesByCertNumbers` on Repository)
2. For each row:
   - No purchase found → `NotFound`
   - Purchase already has a sale (checked via existing `GetSaleByPurchaseID`) → `AlreadySold`
   - Purchase found and unsold → `Matched` with computed fee and net profit preview
3. Return categorized result for frontend review

**`ConfirmOrdersSales(ctx, items []OrdersConfirmItem) (*BulkSaleResult, error)`**

1. For each item: fetch purchase, fetch campaign, call `CreateSale`
2. Return `BulkSaleResult` with created/failed counts and per-item errors
3. Uses existing sale creation logic — fees, profit, days-to-sell all computed normally

### API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/purchases/import-orders` | Auth | Upload CSV, return categorized matches |
| POST | `/api/purchases/import-orders/confirm` | Auth | Create sales for confirmed matches (JSON body) |

### Channel Fee Change

`website` channel fee changes from 0% to 3% in `channel_fees.go`. This affects all future website sales, not just imports. Existing sales are unaffected (fees stored at creation time).

## Frontend

### Page: `web/src/react/pages/sales/ImportOrdersPage.tsx`

Route: `/sales/import`

#### Phase 1: Upload

- File upload area (drag-and-drop or click)
- "Import Orders" button
- Matches existing import page styling

#### Phase 2: Review & Confirm

**Summary bar:** `42 matched · 3 already sold · 5 not found · 12 skipped`

**Matched table:**

| Checkbox | Card Name | Cert # | Channel | Sale Date | Sale Price | Fee | Cost | Net Profit |
|----------|-----------|--------|---------|-----------|------------|-----|------|------------|

- All rows checked by default
- Net profit column: green for positive, red for negative
- "Confirm Selected" button creates sales
- After confirmation: success toast with count, page resets

**Collapsible sections** for Already Sold, Not Found, Skipped — informational, no actions.

### Navigation

Add "Import Sales" link to the main nav, alongside existing import options.

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| Duplicate cert in CSV | First wins, rest skipped with reason "duplicate" |
| Cert already sold | Categorized as "already_sold", shown in collapsible section |
| Cert not in inventory | Categorized as "not_found", no purchase creation |
| Unknown channel value | Row skipped with reason "unknown_channel" |
| Race: sold between preview and confirm | CreateSale fails gracefully, reported in BulkSaleResult errors |
| Partial confirm failures | Per-item errors returned; successful sales are not rolled back |

## Files to Create/Modify

### New Files
- `internal/domain/campaigns/parse_orders.go` — CSV parser
- `internal/domain/campaigns/service_import_orders.go` — ImportOrdersSales + ConfirmOrdersSales
- `web/src/react/pages/sales/ImportOrdersPage.tsx` — Frontend page

### Modified Files
- `internal/domain/campaigns/import_types.go` — New types
- `internal/domain/campaigns/service.go` — Add methods to Service interface
- `internal/domain/campaigns/channel_fees.go` — Website fee 0% → 3%
- `internal/adapters/httpserver/handlers/campaigns_imports.go` — New handler methods
- `internal/adapters/httpserver/router.go` — Register new routes
- `web/src/js/api/campaigns.ts` — API client methods
- `web/src/react/App.tsx` (or router config) — Add route
- Navigation component — Add "Import Sales" link
