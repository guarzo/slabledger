# Orders Sales Import Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Import sales from an orders export CSV (Shopify/eBay) by matching PSA cert numbers against existing inventory, with a review step before committing.

**Architecture:** Two new backend endpoints (upload-and-preview, confirm) plus a new frontend tab on the Tools page. Parser filters to PSA-only rows, deduplicates by cert, matches against `campaign_purchases` via `GetPurchasesByCertNumbers`. Confirmation creates sales through existing `CreateSale` logic. Website channel fee changes from 0% to 3%.

**Tech Stack:** Go 1.26 backend, React + TypeScript frontend, TanStack Query, Tailwind CSS, existing `campaigns` domain package patterns.

---

### Task 1: Website Channel Fee (0% → 3%)

**Files:**
- Modify: `internal/domain/campaigns/channel_fees.go:17-31`
- Test: `internal/domain/campaigns/channel_fees_test.go` (create if absent)

- [ ] **Step 1: Write the failing test**

Create `internal/domain/campaigns/channel_fees_test.go`:

```go
package campaigns

import "testing"

func TestCalculateSaleFee_WebsiteChannel(t *testing.T) {
	campaign := &Campaign{EbayFeePct: 0.1235}

	// Website channel should charge 3% fee
	fee := CalculateSaleFee(SaleChannelWebsite, 10000, campaign)
	if fee != 300 {
		t.Errorf("website fee: got %d, want 300 (3%% of 10000)", fee)
	}

	// eBay should still charge 12.35%
	ebayFee := CalculateSaleFee(SaleChannelEbay, 10000, campaign)
	if ebayFee != 1235 {
		t.Errorf("ebay fee: got %d, want 1235", ebayFee)
	}

	// Local should still be 0%
	localFee := CalculateSaleFee(SaleChannelLocal, 10000, campaign)
	if localFee != 0 {
		t.Errorf("local fee: got %d, want 0", localFee)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspace && go test ./internal/domain/campaigns/ -run TestCalculateSaleFee_WebsiteChannel -v`
Expected: FAIL — website fee returns 0, not 300.

- [ ] **Step 3: Update channel_fees.go**

In `internal/domain/campaigns/channel_fees.go`, add a constant and update `CalculateSaleFee`:

```go
// DefaultWebsiteFeePct is the fee percentage for website/online store sales (3% credit card processing).
const DefaultWebsiteFeePct = 0.03
```

Change the switch in `CalculateSaleFee` (line 19-31): move `SaleChannelWebsite` out of the zero-fee case and give it its own case:

```go
func CalculateSaleFee(channel SaleChannel, salePriceCents int, campaign *Campaign) int {
	switch channel {
	case SaleChannelEbay, SaleChannelTCGPlayer:
		feePct := campaign.EbayFeePct
		if feePct == 0 {
			feePct = DefaultMarketplaceFeePct
		}
		return int(math.Round(float64(salePriceCents) * feePct))
	case SaleChannelWebsite:
		return int(math.Round(float64(salePriceCents) * DefaultWebsiteFeePct))
	case SaleChannelLocal, SaleChannelOther, SaleChannelGameStop, SaleChannelCardShow:
		return 0
	default:
		return 0
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspace && go test ./internal/domain/campaigns/ -run TestCalculateSaleFee -v`
Expected: PASS

- [ ] **Step 5: Run all existing tests to check for regressions**

Run: `cd /workspace && go test ./internal/domain/campaigns/... -count=1`
Expected: PASS — no existing tests should break (the fee change only affects future sales; stored fees are immutable).

- [ ] **Step 6: Commit**

```bash
git add internal/domain/campaigns/channel_fees.go internal/domain/campaigns/channel_fees_test.go
git commit -m "feat: add 3% fee for website sales channel

Website/online store sales now incur a 3% credit card processing fee.
Previously grouped with zero-fee channels (local, cardshow, etc.)."
```

---

### Task 2: Data Types

**Files:**
- Modify: `internal/domain/campaigns/import_types.go` (append new types)
- Modify: `web/src/types/campaigns/core.ts` (append new interfaces)

- [ ] **Step 1: Add Go types**

Append to `internal/domain/campaigns/import_types.go`:

```go
// OrdersExportRow represents a single row parsed from an orders export CSV.
type OrdersExportRow struct {
	OrderNumber  string
	Date         string      // YYYY-MM-DD
	SalesChannel SaleChannel // Mapped from CSV value
	ProductTitle string
	Grader       string
	CertNumber   string
	Grade        float64
	UnitPrice    float64 // Dollars
}

// OrdersImportResult categorizes parsed order rows by match status.
type OrdersImportResult struct {
	Matched     []OrdersImportMatch `json:"matched"`
	AlreadySold []OrdersImportSkip  `json:"alreadySold"`
	NotFound    []OrdersImportSkip  `json:"notFound"`
	Skipped     []OrdersImportSkip  `json:"skipped"`
}

// OrdersImportMatch represents a CSV row matched to an unsold inventory purchase.
type OrdersImportMatch struct {
	CertNumber     string      `json:"certNumber"`
	ProductTitle   string      `json:"productTitle"`
	SaleChannel    SaleChannel `json:"saleChannel"`
	SaleDate       string      `json:"saleDate"`
	SalePriceCents int         `json:"salePriceCents"`
	SaleFeeCents   int         `json:"saleFeeCents"`
	PurchaseID     string      `json:"purchaseId"`
	CampaignID     string      `json:"campaignId"`
	CardName       string      `json:"cardName"`
	BuyCostCents   int         `json:"buyCostCents"`
	NetProfitCents int         `json:"netProfitCents"`
}

// OrdersImportSkip represents a CSV row that was skipped or couldn't be matched.
type OrdersImportSkip struct {
	CertNumber   string `json:"certNumber"`
	ProductTitle string `json:"productTitle"`
	Reason       string `json:"reason"` // "already_sold", "not_found", "duplicate", "not_psa", "unknown_channel"
}

// OrdersConfirmItem carries the data needed to create a sale from a confirmed import match.
type OrdersConfirmItem struct {
	PurchaseID     string      `json:"purchaseId"`
	SaleChannel    SaleChannel `json:"saleChannel"`
	SaleDate       string      `json:"saleDate"`
	SalePriceCents int         `json:"salePriceCents"`
}
```

- [ ] **Step 2: Add TypeScript types**

Append to `web/src/types/campaigns/core.ts`:

```typescript
// Orders sales import types

export interface OrdersImportMatch {
  certNumber: string;
  productTitle: string;
  saleChannel: string;
  saleDate: string;
  salePriceCents: number;
  saleFeeCents: number;
  purchaseId: string;
  campaignId: string;
  cardName: string;
  buyCostCents: number;
  netProfitCents: number;
}

export interface OrdersImportSkip {
  certNumber: string;
  productTitle: string;
  reason: string;
}

export interface OrdersImportResult {
  matched: OrdersImportMatch[];
  alreadySold: OrdersImportSkip[];
  notFound: OrdersImportSkip[];
  skipped: OrdersImportSkip[];
}

export interface OrdersConfirmItem {
  purchaseId: string;
  saleChannel: string;
  saleDate: string;
  salePriceCents: number;
}
```

- [ ] **Step 3: Verify Go compiles**

Run: `cd /workspace && go build ./internal/domain/campaigns/...`
Expected: builds successfully.

- [ ] **Step 4: Verify TypeScript compiles**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: no type errors.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/campaigns/import_types.go web/src/types/campaigns/core.ts
git commit -m "feat: add orders sales import data types

Go and TypeScript types for the orders CSV import flow:
OrdersExportRow, OrdersImportResult, OrdersImportMatch,
OrdersImportSkip, OrdersConfirmItem."
```

---

### Task 3: CSV Parser

**Files:**
- Create: `internal/domain/campaigns/parse_orders.go`
- Create: `internal/domain/campaigns/parse_orders_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/domain/campaigns/parse_orders_test.go`:

```go
package campaigns

import (
	"testing"
)

func TestParseOrdersExportRows(t *testing.T) {
	records := [][]string{
		{"Order", "Date", "Sales Channel", "Product Title", "Grading Company", "Cert Number", "Grade", "Qty", "Unit Price", "Line Subtotal"},
		{"#1002", "2026-03-10", "eBay", "Dark Gengar Holo - Neo Destiny - #6 PSA 5", "PSA", "194544353", "5", "1", "259.35", "259.35"},
		{"#1001", "2026-03-09", "Online Store", "Ditto - Old Maid CGC 10", "CGC", "", "10", "1", "22.80", "22.80"},
		{"#1004", "2026-03-14", "eBay", "Dragonite Holo PSA 3", "PSA", "191055511", "3", "1", "54.86", "54.86"},
		// Duplicate cert — should be skipped
		{"#1005", "2026-03-15", "eBay", "Dragonite Holo PSA 3", "PSA", "191055511", "3", "1", "54.86", "54.86"},
		// No grading company — should be skipped
		{"#1008", "2026-03-15", "eBay", "Umbreon & Darkrai GX", "", "", "", "1", "37.90", "37.90"},
		// PSA but empty cert — should be skipped
		{"#1012", "2026-03-20", "eBay", "Mewtwo PSA 9", "PSA", "", "9", "1", "442.89", "442.89"},
		// Unknown channel — should be skipped
		{"#1099", "2026-03-20", "Amazon", "Pikachu PSA 10", "PSA", "999999", "10", "1", "100.00", "100.00"},
	}

	rows, skipped, err := ParseOrdersExportRows(records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 2 valid PSA rows
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}

	// First row: eBay PSA card
	if rows[0].CertNumber != "194544353" {
		t.Errorf("row 0 cert: got %q, want 194544353", rows[0].CertNumber)
	}
	if rows[0].SalesChannel != SaleChannelEbay {
		t.Errorf("row 0 channel: got %q, want ebay", rows[0].SalesChannel)
	}
	if rows[0].UnitPrice != 259.35 {
		t.Errorf("row 0 price: got %f, want 259.35", rows[0].UnitPrice)
	}
	if rows[0].Date != "2026-03-10" {
		t.Errorf("row 0 date: got %q, want 2026-03-10", rows[0].Date)
	}

	// Second row: Website PSA card
	if rows[1].CertNumber != "191055511" {
		t.Errorf("row 1 cert: got %q, want 191055511", rows[1].CertNumber)
	}
	if rows[1].SalesChannel != SaleChannelEbay {
		t.Errorf("row 1 channel: got %q, want ebay", rows[1].SalesChannel)
	}

	// Skipped: CGC (1) + duplicate cert (1) + no grader (1) + PSA empty cert (1) + unknown channel (1) = 5
	if len(skipped) != 5 {
		t.Fatalf("got %d skipped, want 5", len(skipped))
	}

	// Check skip reasons
	reasons := map[string]int{}
	for _, s := range skipped {
		reasons[s.Reason]++
	}
	if reasons["not_psa"] != 2 {
		t.Errorf("not_psa skips: got %d, want 2", reasons["not_psa"])
	}
	if reasons["duplicate"] != 1 {
		t.Errorf("duplicate skips: got %d, want 1", reasons["duplicate"])
	}
	if reasons["no_cert"] != 1 {
		t.Errorf("no_cert skips: got %d, want 1", reasons["no_cert"])
	}
	if reasons["unknown_channel"] != 1 {
		t.Errorf("unknown_channel skips: got %d, want 1", reasons["unknown_channel"])
	}
}

func TestParseOrdersExportRows_MissingHeader(t *testing.T) {
	records := [][]string{
		{"Order", "Date", "Product Title"}, // missing required columns
	}
	_, _, err := ParseOrdersExportRows(records)
	if err == nil {
		t.Fatal("expected error for missing columns")
	}
}

func TestParseOrdersExportRows_OnlineStoreChannel(t *testing.T) {
	records := [][]string{
		{"Order", "Date", "Sales Channel", "Product Title", "Grading Company", "Cert Number", "Grade", "Qty", "Unit Price", "Line Subtotal"},
		{"#1055", "2026-03-26", "Online Store", "Karen's Flareon PSA 8", "PSA", "139288937", "8", "1", "71.25", "71.25"},
	}
	rows, _, err := ParseOrdersExportRows(records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].SalesChannel != SaleChannelWebsite {
		t.Errorf("channel: got %q, want website", rows[0].SalesChannel)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspace && go test ./internal/domain/campaigns/ -run TestParseOrdersExport -v`
Expected: FAIL — `ParseOrdersExportRows` undefined.

- [ ] **Step 3: Implement the parser**

Create `internal/domain/campaigns/parse_orders.go`:

```go
package campaigns

import (
	"fmt"
	"math"
	"strings"
)

// mapOrdersChannel maps CSV "Sales Channel" values to SaleChannel constants.
// Returns empty string for unknown channels.
func mapOrdersChannel(raw string) SaleChannel {
	switch strings.TrimSpace(raw) {
	case "eBay":
		return SaleChannelEbay
	case "Online Store":
		return SaleChannelWebsite
	default:
		return ""
	}
}

// ParseOrdersExportRows parses CSV records from an orders export.
// The first row must be the header row.
// Returns valid PSA rows, skipped rows (with reasons), and a fatal error
// if the CSV structure is invalid.
func ParseOrdersExportRows(records [][]string) ([]OrdersExportRow, []OrdersImportSkip, error) {
	if len(records) < 2 {
		return nil, nil, fmt.Errorf("CSV must have a header row and at least one data row")
	}

	headerMap := BuildHeaderMap(records[0])
	colIdx := func(name string) int {
		if idx, ok := headerMap[name]; ok {
			return idx
		}
		return -1
	}

	// Validate required columns
	required := []string{"date", "sales channel", "product title", "grading company", "cert number", "unit price"}
	for _, col := range required {
		if _, ok := headerMap[col]; !ok {
			return nil, nil, fmt.Errorf("CSV is missing required column: %s", col)
		}
	}

	seen := make(map[string]bool)
	var rows []OrdersExportRow
	var skipped []OrdersImportSkip

	for _, rec := range records[1:] {
		getField := func(idx int) string {
			if idx >= 0 && idx < len(rec) {
				return strings.TrimSpace(rec[idx])
			}
			return ""
		}

		orderNumber := getField(colIdx("order"))
		date := getField(colIdx("date"))
		channelRaw := getField(colIdx("sales channel"))
		productTitle := getField(colIdx("product title"))
		grader := getField(colIdx("grading company"))
		certRaw := getField(colIdx("cert number"))
		gradeRaw := getField(colIdx("grade"))
		priceRaw := getField(colIdx("unit price"))

		// Filter: only PSA
		if !strings.EqualFold(grader, "PSA") {
			skipped = append(skipped, OrdersImportSkip{
				CertNumber:   certRaw,
				ProductTitle: productTitle,
				Reason:       "not_psa",
			})
			continue
		}

		// Filter: must have cert number
		cert := NormalizePSACert(certRaw)
		if cert == "" {
			skipped = append(skipped, OrdersImportSkip{
				CertNumber:   certRaw,
				ProductTitle: productTitle,
				Reason:       "no_cert",
			})
			continue
		}

		// Map channel
		channel := mapOrdersChannel(channelRaw)
		if channel == "" {
			skipped = append(skipped, OrdersImportSkip{
				CertNumber:   cert,
				ProductTitle: productTitle,
				Reason:       "unknown_channel",
			})
			continue
		}

		// Deduplicate by cert — first occurrence wins
		if seen[cert] {
			skipped = append(skipped, OrdersImportSkip{
				CertNumber:   cert,
				ProductTitle: productTitle,
				Reason:       "duplicate",
			})
			continue
		}
		seen[cert] = true

		// Parse price
		price, err := ParseCurrencyString(priceRaw)
		if err != nil {
			skipped = append(skipped, OrdersImportSkip{
				CertNumber:   cert,
				ProductTitle: productTitle,
				Reason:       fmt.Sprintf("invalid_price: %s", priceRaw),
			})
			continue
		}

		// Parse grade (best-effort, 0 if unparseable)
		var grade float64
		if gradeRaw != "" {
			if v, err := ParseCurrencyString(gradeRaw); err == nil {
				grade = v
			}
		}

		rows = append(rows, OrdersExportRow{
			OrderNumber:  orderNumber,
			Date:         date,
			SalesChannel: channel,
			ProductTitle: productTitle,
			Grader:       "PSA",
			CertNumber:   cert,
			Grade:        grade,
			UnitPrice:    price,
		})
	}

	return rows, skipped, nil
}

// DollarsToCents converts a dollar amount to cents with rounding.
func DollarsToCents(dollars float64) int {
	return int(math.Round(dollars * 100))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspace && go test ./internal/domain/campaigns/ -run TestParseOrdersExport -v`
Expected: PASS (all 3 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/domain/campaigns/parse_orders.go internal/domain/campaigns/parse_orders_test.go
git commit -m "feat: add orders export CSV parser

Parses 10-column orders export CSV, filters to PSA-only rows with
cert numbers, deduplicates by cert (first wins), maps eBay/Online Store
to sale channels, and skips unknown channels."
```

---

### Task 4: Service Methods

**Files:**
- Create: `internal/domain/campaigns/service_import_orders.go`
- Create: `internal/domain/campaigns/service_import_orders_test.go`
- Modify: `internal/domain/campaigns/service.go:121` (add methods to Service interface)

- [ ] **Step 1: Add methods to Service interface**

In `internal/domain/campaigns/service.go`, add these two methods inside the `Service` interface (after `ImportExternalCSV`, around line 146):

```go
	// Orders sales import
	ImportOrdersSales(ctx context.Context, rows []OrdersExportRow) (*OrdersImportResult, error)
	ConfirmOrdersSales(ctx context.Context, items []OrdersConfirmItem) (*BulkSaleResult, error)
```

- [ ] **Step 2: Verify Go fails to compile (interface not satisfied)**

Run: `cd /workspace && go build ./internal/domain/campaigns/...`
Expected: FAIL — `*service` does not implement `Service` (missing `ImportOrdersSales` and `ConfirmOrdersSales`).

- [ ] **Step 3: Write tests**

Create `internal/domain/campaigns/service_import_orders_test.go`:

```go
package campaigns

import (
	"context"
	"testing"
)

func TestImportOrdersSales(t *testing.T) {
	repo := newMockRepo()

	// Set up a campaign and two purchases
	campaign := &Campaign{ID: "camp-1", Name: "Test Campaign", EbayFeePct: 0.1235}
	repo.campaigns["camp-1"] = campaign

	repo.purchases["purch-1"] = &Purchase{
		ID:                  "purch-1",
		CampaignID:          "camp-1",
		CertNumber:          "111111",
		CardName:            "Charizard",
		BuyCostCents:        10000,
		PSASourcingFeeCents: 300,
		GradeValue:          9,
		PurchaseDate:        "2026-01-01",
	}
	repo.purchases["purch-2"] = &Purchase{
		ID:                  "purch-2",
		CampaignID:          "camp-1",
		CertNumber:          "222222",
		CardName:            "Pikachu",
		BuyCostCents:        5000,
		PSASourcingFeeCents: 300,
		GradeValue:          10,
		PurchaseDate:        "2026-01-01",
	}
	// purch-3 already sold
	repo.purchases["purch-3"] = &Purchase{
		ID:                  "purch-3",
		CampaignID:          "camp-1",
		CertNumber:          "333333",
		CardName:            "Blastoise",
		BuyCostCents:        8000,
		PSASourcingFeeCents: 300,
		GradeValue:          8,
		PurchaseDate:        "2026-01-01",
	}
	repo.sales["sale-3"] = &Sale{ID: "sale-3", PurchaseID: "purch-3"}
	repo.purchaseSales["purch-3"] = true

	svc := NewService(repo, WithIDGenerator(func() string { return "gen-id" }))
	defer svc.Close()

	rows := []OrdersExportRow{
		{CertNumber: "111111", Date: "2026-03-10", SalesChannel: SaleChannelEbay, ProductTitle: "Charizard PSA 9", UnitPrice: 200.00},
		{CertNumber: "333333", Date: "2026-03-11", SalesChannel: SaleChannelEbay, ProductTitle: "Blastoise PSA 8", UnitPrice: 150.00},
		{CertNumber: "999999", Date: "2026-03-12", SalesChannel: SaleChannelWebsite, ProductTitle: "Unknown PSA 10", UnitPrice: 50.00},
	}

	result, err := svc.ImportOrdersSales(context.Background(), rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1 matched (cert 111111)
	if len(result.Matched) != 1 {
		t.Fatalf("matched: got %d, want 1", len(result.Matched))
	}
	m := result.Matched[0]
	if m.CertNumber != "111111" {
		t.Errorf("matched cert: got %q, want 111111", m.CertNumber)
	}
	if m.PurchaseID != "purch-1" {
		t.Errorf("matched purchaseID: got %q, want purch-1", m.PurchaseID)
	}
	if m.SalePriceCents != 20000 {
		t.Errorf("matched salePriceCents: got %d, want 20000", m.SalePriceCents)
	}
	// eBay fee: 12.35% of 20000 = 2470
	if m.SaleFeeCents != 2470 {
		t.Errorf("matched saleFeeCents: got %d, want 2470", m.SaleFeeCents)
	}
	// Net: 20000 - 10000 - 300 - 2470 = 7230
	if m.NetProfitCents != 7230 {
		t.Errorf("matched netProfit: got %d, want 7230", m.NetProfitCents)
	}

	// 1 already sold (cert 333333)
	if len(result.AlreadySold) != 1 {
		t.Fatalf("alreadySold: got %d, want 1", len(result.AlreadySold))
	}
	if result.AlreadySold[0].CertNumber != "333333" {
		t.Errorf("alreadySold cert: got %q, want 333333", result.AlreadySold[0].CertNumber)
	}

	// 1 not found (cert 999999)
	if len(result.NotFound) != 1 {
		t.Fatalf("notFound: got %d, want 1", len(result.NotFound))
	}
	if result.NotFound[0].CertNumber != "999999" {
		t.Errorf("notFound cert: got %q, want 999999", result.NotFound[0].CertNumber)
	}
}

func TestConfirmOrdersSales(t *testing.T) {
	repo := newMockRepo()

	campaign := &Campaign{ID: "camp-1", Name: "Test Campaign", EbayFeePct: 0.1235}
	repo.campaigns["camp-1"] = campaign

	repo.purchases["purch-1"] = &Purchase{
		ID:                  "purch-1",
		CampaignID:          "camp-1",
		CertNumber:          "111111",
		CardName:            "Charizard",
		BuyCostCents:        10000,
		PSASourcingFeeCents: 300,
		GradeValue:          9,
		PurchaseDate:        "2026-01-01",
	}

	idCounter := 0
	svc := NewService(repo, WithIDGenerator(func() string {
		idCounter++
		return fmt.Sprintf("sale-%d", idCounter)
	}))
	defer svc.Close()

	items := []OrdersConfirmItem{
		{PurchaseID: "purch-1", SaleChannel: SaleChannelEbay, SaleDate: "2026-03-10", SalePriceCents: 20000},
		{PurchaseID: "purch-bad", SaleChannel: SaleChannelEbay, SaleDate: "2026-03-10", SalePriceCents: 5000},
	}

	result, err := svc.ConfirmOrdersSales(context.Background(), items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Created != 1 {
		t.Errorf("created: got %d, want 1", result.Created)
	}
	if result.Failed != 1 {
		t.Errorf("failed: got %d, want 1", result.Failed)
	}

	// Verify the sale was actually created
	if _, exists := repo.sales["sale-1"]; !exists {
		t.Error("expected sale-1 to exist in repo")
	}
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `cd /workspace && go test ./internal/domain/campaigns/ -run "TestImportOrdersSales|TestConfirmOrdersSales" -v`
Expected: FAIL — methods not defined.

- [ ] **Step 5: Implement service methods**

Create `internal/domain/campaigns/service_import_orders.go`:

```go
package campaigns

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

func (s *service) ImportOrdersSales(ctx context.Context, rows []OrdersExportRow) (*OrdersImportResult, error) {
	// Collect all cert numbers for batch lookup
	certs := make([]string, 0, len(rows))
	for _, r := range rows {
		certs = append(certs, r.CertNumber)
	}

	purchaseMap, err := s.repo.GetPurchasesByCertNumbers(ctx, certs)
	if err != nil {
		return nil, fmt.Errorf("batch cert lookup failed: %w", err)
	}

	result := &OrdersImportResult{}

	for _, r := range rows {
		purchase, found := purchaseMap[r.CertNumber]
		if !found {
			result.NotFound = append(result.NotFound, OrdersImportSkip{
				CertNumber:   r.CertNumber,
				ProductTitle: r.ProductTitle,
				Reason:       "not_found",
			})
			continue
		}

		// Check if already sold
		existingSale, _ := s.repo.GetSaleByPurchaseID(ctx, purchase.ID)
		if existingSale != nil {
			result.AlreadySold = append(result.AlreadySold, OrdersImportSkip{
				CertNumber:   r.CertNumber,
				ProductTitle: r.ProductTitle,
				Reason:       "already_sold",
			})
			continue
		}

		// Compute fee and net profit preview
		salePriceCents := DollarsToCents(r.UnitPrice)

		campaign, err := s.repo.GetCampaign(ctx, purchase.CampaignID)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "campaign lookup failed for import preview",
					observability.String("campaignID", purchase.CampaignID),
					observability.Err(err))
			}
			// Use a zero-fee campaign as fallback
			campaign = &Campaign{}
		}

		saleFeeCents := CalculateSaleFee(r.SalesChannel, salePriceCents, campaign)
		netProfit := CalculateNetProfit(salePriceCents, purchase.BuyCostCents, purchase.PSASourcingFeeCents, saleFeeCents)

		result.Matched = append(result.Matched, OrdersImportMatch{
			CertNumber:     r.CertNumber,
			ProductTitle:   r.ProductTitle,
			SaleChannel:    r.SalesChannel,
			SaleDate:       r.Date,
			SalePriceCents: salePriceCents,
			SaleFeeCents:   saleFeeCents,
			PurchaseID:     purchase.ID,
			CampaignID:     purchase.CampaignID,
			CardName:       purchase.CardName,
			BuyCostCents:   purchase.BuyCostCents,
			NetProfitCents: netProfit,
		})
	}

	return result, nil
}

func (s *service) ConfirmOrdersSales(ctx context.Context, items []OrdersConfirmItem) (*BulkSaleResult, error) {
	result := &BulkSaleResult{}

	for _, item := range items {
		purchase, err := s.repo.GetPurchase(ctx, item.PurchaseID)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: "purchase not found"})
			continue
		}

		campaign, err := s.repo.GetCampaign(ctx, purchase.CampaignID)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: "campaign not found"})
			continue
		}

		sa := &Sale{
			PurchaseID:     item.PurchaseID,
			SaleChannel:    item.SaleChannel,
			SalePriceCents: item.SalePriceCents,
			SaleDate:       item.SaleDate,
		}

		if err := s.CreateSale(ctx, sa, campaign, purchase); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: err.Error()})
			continue
		}
		result.Created++
	}

	return result, nil
}
```

- [ ] **Step 6: Add missing import to test file**

Add `"fmt"` import to `service_import_orders_test.go` (used by `Sprintf` in `TestConfirmOrdersSales`).

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd /workspace && go test ./internal/domain/campaigns/ -run "TestImportOrdersSales|TestConfirmOrdersSales" -v`
Expected: PASS

- [ ] **Step 8: Run all domain tests**

Run: `cd /workspace && go test ./internal/domain/campaigns/... -count=1`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add internal/domain/campaigns/service.go internal/domain/campaigns/service_import_orders.go internal/domain/campaigns/service_import_orders_test.go
git commit -m "feat: add ImportOrdersSales and ConfirmOrdersSales service methods

ImportOrdersSales batch-matches cert numbers against inventory, categorizes
as matched/already_sold/not_found, and previews fees and net profit.
ConfirmOrdersSales creates sales through existing CreateSale logic."
```

---

### Task 5: HTTP Handlers

**Files:**
- Modify: `internal/adapters/httpserver/handlers/campaigns_imports.go` (add two handler methods)
- Modify: `internal/adapters/httpserver/router.go:370-374` (register routes)

- [ ] **Step 1: Add handler methods**

Append to `internal/adapters/httpserver/handlers/campaigns_imports.go` (before the `parseGlobalCSVUpload` method):

```go
// HandleImportOrders handles POST /api/purchases/import-orders.
// Accepts an orders export CSV, matches PSA certs against inventory, and returns
// categorized results for review before confirmation.
func (h *CampaignsHandler) HandleImportOrders(w http.ResponseWriter, r *http.Request) {
	rows, ok := h.parseGlobalCSVUpload(w, r)
	if !ok {
		return
	}

	orderRows, skipped, err := campaigns.ParseOrdersExportRows(rows)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if len(orderRows) == 0 {
		// No valid PSA rows — return result with only skipped items
		writeJSON(w, http.StatusOK, &campaigns.OrdersImportResult{
			Skipped: skipped,
		})
		return
	}

	result, svcErr := h.service.ImportOrdersSales(r.Context(), orderRows)
	if svcErr != nil {
		h.logger.Error(r.Context(), "orders import failed", observability.Err(svcErr))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Merge parser-level skips into the result
	result.Skipped = append(result.Skipped, skipped...)

	writeJSON(w, http.StatusOK, result)
}

// HandleConfirmOrdersSales handles POST /api/purchases/import-orders/confirm.
// Accepts confirmed matches and creates sale records.
func (h *CampaignsHandler) HandleConfirmOrdersSales(w http.ResponseWriter, r *http.Request) {
	const maxBytes = 1 << 20 // 1MB
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	var items []campaigns.OrdersConfirmItem
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if len(items) == 0 {
		writeError(w, http.StatusBadRequest, "No items provided")
		return
	}

	result, err := h.service.ConfirmOrdersSales(r.Context(), items)
	if err != nil {
		h.logger.Error(r.Context(), "confirm orders sales failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
```

- [ ] **Step 2: Register routes in router.go**

In `internal/adapters/httpserver/router.go`, add two lines after the `import-external` route (around line 373):

```go
		mux.Handle("POST /api/purchases/import-orders", authRoute(rt.campaignsHandler.HandleImportOrders))
		mux.Handle("POST /api/purchases/import-orders/confirm", authRoute(rt.campaignsHandler.HandleConfirmOrdersSales))
```

- [ ] **Step 3: Verify Go compiles**

Run: `cd /workspace && go build ./...`
Expected: builds successfully.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/httpserver/handlers/campaigns_imports.go internal/adapters/httpserver/router.go
git commit -m "feat: add import-orders and confirm HTTP endpoints

POST /api/purchases/import-orders — upload CSV, return categorized matches
POST /api/purchases/import-orders/confirm — create sales for confirmed items"
```

---

### Task 6: Update Mock Repositories

**Files:**
- Modify: `internal/domain/campaigns/mock_repo_test.go` (add any missing method stubs)
- Modify: `internal/testutil/mocks/campaign_repository.go` (add any missing method stubs)

- [ ] **Step 1: Check if mock_repo_test.go needs updates**

The two new service methods (`ImportOrdersSales`, `ConfirmOrdersSales`) are on the service, not the repository — so no new repo methods were added. However, verify by running:

Run: `cd /workspace && go build ./...`

If it compiles, no mock changes are needed. If there are "missing method" errors, add the required stubs.

- [ ] **Step 2: Run full test suite**

Run: `cd /workspace && go test ./... -count=1 2>&1 | tail -30`
Expected: PASS. If any failures, fix missing mock methods.

- [ ] **Step 3: Commit (only if changes were needed)**

```bash
git add internal/domain/campaigns/mock_repo_test.go internal/testutil/mocks/campaign_repository.go
git commit -m "fix: update mock repositories for new service interface methods"
```

---

### Task 7: Frontend API Client

**Files:**
- Modify: `web/src/js/api/campaigns.ts` (add two methods)
- Modify: `web/src/types/campaigns/core.ts` (types already added in Task 2)

- [ ] **Step 1: Add API methods**

In `web/src/js/api/campaigns.ts`, add after the `globalImportExternal` method (around line 347):

```typescript
// Orders sales import (upload CSV, get categorized matches)
proto.importOrdersSales = async function (this: APIClient, file: File): Promise<OrdersImportResult> {
  return this.uploadFile<OrdersImportResult>('/purchases/import-orders', file);
};

// Orders sales confirm (create sales for confirmed matches)
proto.confirmOrdersSales = async function (this: APIClient, items: OrdersConfirmItem[]): Promise<BulkSaleResult> {
  return this.post<BulkSaleResult>('/purchases/import-orders/confirm', items);
};
```

- [ ] **Step 2: Add type imports**

In `web/src/js/api/campaigns.ts`, add `OrdersImportResult`, `OrdersConfirmItem` to the import from `../../types/campaigns`:

Find the existing import line for types and add the two new types.

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add web/src/js/api/campaigns.ts
git commit -m "feat: add importOrdersSales and confirmOrdersSales API client methods"
```

---

### Task 8: Frontend — Import Sales Tab

**Files:**
- Create: `web/src/react/pages/tools/ImportSalesTab.tsx`
- Modify: `web/src/react/pages/ToolsPage.tsx` (add tab)

- [ ] **Step 1: Create the ImportSalesTab component**

Create `web/src/react/pages/tools/ImportSalesTab.tsx`:

```tsx
import { useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { api } from '../../../js/api';
import type { OrdersImportResult, OrdersImportMatch, OrdersImportSkip, BulkSaleResult } from '../../../types/campaigns';
import { queryKeys } from '../../queries/queryKeys';
import { useToast } from '../../contexts/ToastContext';
import { Button, CardShell } from '../../ui';
import { formatCents, getErrorMessage } from '../../utils/formatters';

type Phase = 'upload' | 'review' | 'confirming';

export default function ImportSalesTab() {
  const toast = useToast();
  const queryClient = useQueryClient();
  const fileRef = useRef<HTMLInputElement>(null);

  const [phase, setPhase] = useState<Phase>('upload');
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<OrdersImportResult | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [confirmResult, setConfirmResult] = useState<BulkSaleResult | null>(null);

  async function handleUpload(file: File) {
    try {
      setLoading(true);
      setResult(null);
      setConfirmResult(null);
      const res = await api.importOrdersSales(file);
      setResult(res);
      // Select all matched by default
      setSelected(new Set(res.matched.map(m => m.purchaseId)));
      setPhase('review');
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to parse orders CSV'));
    } finally {
      setLoading(false);
    }
  }

  async function handleConfirm() {
    if (!result) return;
    const items = result.matched
      .filter(m => selected.has(m.purchaseId))
      .map(m => ({
        purchaseId: m.purchaseId,
        saleChannel: m.saleChannel,
        saleDate: m.saleDate,
        salePriceCents: m.salePriceCents,
      }));

    if (items.length === 0) {
      toast.error('No items selected');
      return;
    }

    try {
      setPhase('confirming');
      const res = await api.confirmOrdersSales(items);
      setConfirmResult(res);
      toast.success(`${res.created} sales created${res.failed > 0 ? `, ${res.failed} failed` : ''}`);

      // Invalidate relevant queries
      queryClient.invalidateQueries({ queryKey: queryKeys.campaigns.all });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.health });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.globalInventory });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.sellSheet });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.insights });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.capitalTimeline });
      queryClient.invalidateQueries({ queryKey: queryKeys.portfolio.weeklyReview });

      // Reset to upload phase
      setPhase('upload');
      setResult(null);
      setSelected(new Set());
    } catch (err) {
      toast.error(getErrorMessage(err, 'Failed to confirm sales'));
      setPhase('review');
    }
  }

  function handleReset() {
    setPhase('upload');
    setResult(null);
    setSelected(new Set());
    setConfirmResult(null);
  }

  function toggleSelect(purchaseId: string) {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(purchaseId)) next.delete(purchaseId);
      else next.add(purchaseId);
      return next;
    });
  }

  function toggleAll() {
    if (!result) return;
    if (selected.size === result.matched.length) {
      setSelected(new Set());
    } else {
      setSelected(new Set(result.matched.map(m => m.purchaseId)));
    }
  }

  const channelLabel = (ch: string) => {
    switch (ch) {
      case 'ebay': return 'eBay';
      case 'website': return 'Website';
      default: return ch;
    }
  };

  // Upload phase
  if (phase === 'upload') {
    return (
      <div className="space-y-4">
        <div className="mb-4">
          <h2 className="text-base font-semibold text-[var(--text)]">Import Sales from Orders</h2>
          <p className="text-xs text-[var(--text-muted)] mt-0.5">
            Upload an orders export CSV to match sales against your inventory by PSA cert number.
            Only PSA-graded cards with cert numbers will be processed.
          </p>
        </div>

        <CardShell variant="default" padding="lg">
          <div className="flex flex-col items-center text-center gap-4 py-4">
            <div className="w-12 h-12 rounded-full bg-[var(--brand-500)]/15 flex items-center justify-center">
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-[var(--brand-500)]" aria-hidden="true">
                <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
                <polyline points="17 8 12 3 7 8" />
                <line x1="12" y1="3" x2="12" y2="15" />
              </svg>
            </div>
            <div>
              <div className="text-sm font-semibold text-[var(--text)]">Upload Orders CSV</div>
              <div className="text-xs text-[var(--text-muted)] mt-1">
                Expects columns: Order, Date, Sales Channel, Product Title, Grading Company, Cert Number, Grade, Qty, Unit Price, Line Subtotal
              </div>
            </div>
            <Button
              size="md"
              variant="primary"
              loading={loading}
              onClick={() => fileRef.current?.click()}
            >
              Choose File
            </Button>
            <input
              ref={fileRef}
              type="file"
              accept=".csv"
              className="hidden"
              onChange={(e) => {
                const file = e.target.files?.[0];
                if (file) handleUpload(file);
                e.target.value = '';
              }}
            />
          </div>
        </CardShell>

        {confirmResult && (
          <div className="p-3 rounded-lg bg-[var(--success-bg)]/30 text-sm">
            <span className="text-[var(--success)] font-medium">{confirmResult.created} sales created</span>
            {confirmResult.failed > 0 && (
              <span className="text-[var(--danger)] ml-2">{confirmResult.failed} failed</span>
            )}
          </div>
        )}
      </div>
    );
  }

  // Review phase
  if (!result) return null;

  const matchedCount = result.matched.length;
  const alreadySoldCount = result.alreadySold.length;
  const notFoundCount = result.notFound.length;
  const skippedCount = result.skipped.length;
  const selectedCount = selected.size;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-base font-semibold text-[var(--text)]">Review Import</h2>
          <div className="flex flex-wrap gap-3 text-xs mt-1">
            {matchedCount > 0 && <span className="text-[var(--success)]">{matchedCount} matched</span>}
            {alreadySoldCount > 0 && <span className="text-[var(--warning)]">{alreadySoldCount} already sold</span>}
            {notFoundCount > 0 && <span className="text-orange-400">{notFoundCount} not found</span>}
            {skippedCount > 0 && <span className="text-[var(--text-muted)]">{skippedCount} skipped</span>}
          </div>
        </div>
        <Button size="sm" variant="ghost" onClick={handleReset}>
          Start Over
        </Button>
      </div>

      {/* Matched table */}
      {matchedCount > 0 && (
        <CardShell variant="default" padding="none">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[var(--surface-2)]">
                  <th className="py-2 px-3 text-left">
                    <input
                      type="checkbox"
                      checked={selectedCount === matchedCount}
                      onChange={toggleAll}
                      className="accent-[var(--brand-500)]"
                    />
                  </th>
                  <th className="py-2 px-3 text-left text-xs text-[var(--text-muted)] font-medium">Card</th>
                  <th className="py-2 px-3 text-left text-xs text-[var(--text-muted)] font-medium">Cert #</th>
                  <th className="py-2 px-3 text-left text-xs text-[var(--text-muted)] font-medium">Channel</th>
                  <th className="py-2 px-3 text-left text-xs text-[var(--text-muted)] font-medium">Date</th>
                  <th className="py-2 px-3 text-right text-xs text-[var(--text-muted)] font-medium">Sale Price</th>
                  <th className="py-2 px-3 text-right text-xs text-[var(--text-muted)] font-medium">Fee</th>
                  <th className="py-2 px-3 text-right text-xs text-[var(--text-muted)] font-medium">Cost</th>
                  <th className="py-2 px-3 text-right text-xs text-[var(--text-muted)] font-medium">Net Profit</th>
                </tr>
              </thead>
              <tbody>
                {result.matched.map((m) => (
                  <tr key={m.purchaseId} className="border-b border-[var(--surface-2)]/50 hover:bg-[var(--surface-1)]/50">
                    <td className="py-2 px-3">
                      <input
                        type="checkbox"
                        checked={selected.has(m.purchaseId)}
                        onChange={() => toggleSelect(m.purchaseId)}
                        className="accent-[var(--brand-500)]"
                      />
                    </td>
                    <td className="py-2 px-3 text-xs text-[var(--text)]">
                      <div className="font-medium">{m.cardName}</div>
                      <div className="text-[var(--text-muted)] text-[10px]">{m.productTitle}</div>
                    </td>
                    <td className="py-2 px-3 text-xs text-[var(--text-muted)] font-mono">{m.certNumber}</td>
                    <td className="py-2 px-3 text-xs text-[var(--text)]">{channelLabel(m.saleChannel)}</td>
                    <td className="py-2 px-3 text-xs text-[var(--text-muted)]">{m.saleDate}</td>
                    <td className="py-2 px-3 text-xs text-right text-[var(--text)]">{formatCents(m.salePriceCents)}</td>
                    <td className="py-2 px-3 text-xs text-right text-[var(--text-muted)]">{formatCents(m.saleFeeCents)}</td>
                    <td className="py-2 px-3 text-xs text-right text-[var(--text-muted)]">{formatCents(m.buyCostCents)}</td>
                    <td className={`py-2 px-3 text-xs text-right font-medium ${m.netProfitCents >= 0 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
                      {m.netProfitCents >= 0 ? '+' : ''}{formatCents(m.netProfitCents)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="flex items-center justify-between p-3 border-t border-[var(--surface-2)]">
            <span className="text-xs text-[var(--text-muted)]">{selectedCount} of {matchedCount} selected</span>
            <Button
              size="sm"
              variant="primary"
              loading={phase === 'confirming'}
              disabled={selectedCount === 0}
              onClick={handleConfirm}
            >
              Confirm {selectedCount} Sale{selectedCount !== 1 ? 's' : ''}
            </Button>
          </div>
        </CardShell>
      )}

      {/* Collapsible sections for non-matched rows */}
      <SkipSection title="Already Sold" items={result.alreadySold} />
      <SkipSection title="Not Found in Inventory" items={result.notFound} />
      <SkipSection title="Skipped (CGC/Ungraded/Duplicate)" items={result.skipped} />
    </div>
  );
}

function SkipSection({ title, items }: { title: string; items: OrdersImportSkip[] }) {
  const [open, setOpen] = useState(false);

  if (items.length === 0) return null;

  return (
    <div className="border border-[var(--surface-2)] rounded-lg">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="w-full flex items-center justify-between p-3 text-xs text-[var(--text-muted)] hover:text-[var(--text)]"
      >
        <span>{title} ({items.length})</span>
        <svg
          width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
          className={`transform transition-transform ${open ? 'rotate-180' : ''}`}
          aria-hidden="true"
        >
          <polyline points="6 9 12 15 18 9" />
        </svg>
      </button>
      {open && (
        <div className="px-3 pb-3">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-[var(--surface-2)]">
                <th className="py-1 text-left text-[var(--text-muted)] font-medium">Cert #</th>
                <th className="py-1 text-left text-[var(--text-muted)] font-medium">Product Title</th>
                <th className="py-1 text-left text-[var(--text-muted)] font-medium">Reason</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item, idx) => (
                <tr key={`${item.certNumber}-${idx}`} className="border-b border-[var(--surface-2)]/30">
                  <td className="py-1 text-[var(--text-muted)] font-mono">{item.certNumber || '—'}</td>
                  <td className="py-1 text-[var(--text)]">{item.productTitle}</td>
                  <td className="py-1 text-[var(--text-muted)]">{item.reason}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Add tab to ToolsPage**

In `web/src/react/pages/ToolsPage.tsx`:

Add the import at the top:

```typescript
import ImportSalesTab from './tools/ImportSalesTab';
```

Add to the TABS array (after `'ebay-export'`):

```typescript
  { id: 'import-sales', label: 'Import Sales' },
```

Add the tab content inside `<Tabs.Root>` (after the `ebay-export` Tabs.Content block):

```tsx
        <Tabs.Content value="import-sales">
          <SectionErrorBoundary sectionName="Import Sales">
            <ImportSalesTab />
          </SectionErrorBoundary>
        </Tabs.Content>
```

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Verify frontend builds**

Run: `cd /workspace/web && npm run build`
Expected: builds successfully.

- [ ] **Step 5: Commit**

```bash
git add web/src/react/pages/tools/ImportSalesTab.tsx web/src/react/pages/ToolsPage.tsx
git commit -m "feat: add Import Sales tab to Tools page

Two-phase UI: upload orders CSV, review matched/skipped/not-found rows
with fee and profit preview, then confirm to create sales. Accessible
from the Tools page as a new tab."
```

---

### Task 9: Lint & Type Checks

**Files:** (no new files — validation pass)

- [ ] **Step 1: Run Go linter**

Run: `cd /workspace && golangci-lint run ./...`
Expected: PASS (or fix any issues).

- [ ] **Step 2: Run frontend lint**

Run: `cd /workspace/web && npm run lint`
Expected: PASS (or fix any issues).

- [ ] **Step 3: Run frontend typecheck**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 4: Run full Go test suite**

Run: `cd /workspace && go test -race -timeout 5m ./...`
Expected: PASS.

- [ ] **Step 5: Fix any issues and commit**

```bash
git add -A
git commit -m "fix: lint and type-check cleanup for orders sales import"
```

---

### Task 10: Update Documentation

**Files:**
- Modify: `CLAUDE.md` (update API routes table, migration count is unchanged)

- [ ] **Step 1: Update CLAUDE.md API Routes table**

Add a row to the "Global Purchases" group (the table row with 9 Auth routes on `/api/purchases/`):

Change `9` to `11` in the count.

- [ ] **Step 2: Add env var note if needed**

No new env vars are needed for this feature — skip.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update API routes table for import-orders endpoints"
```
