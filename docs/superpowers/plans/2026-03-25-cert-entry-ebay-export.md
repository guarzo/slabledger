# Cert Entry & eBay Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable users to import PSA-graded cards by pasting certificate numbers (instead of building a CSV), and export inventory as eBay File Exchange CSV with a price review screen.

**Architecture:** Two new features added to the existing hexagonal architecture. Domain service methods for cert import and eBay export, HTTP handlers following the existing `CampaignsHandler` pattern, and frontend tabs on the Tools page following the `ShopifySyncPage` 3-phase pattern.

**Tech Stack:** Go 1.26 (backend), SQLite with golang-migrate (migrations), React + TypeScript (frontend), Radix Tabs (UI).

**Spec:** `docs/superpowers/specs/2026-03-25-cert-entry-ebay-export-design.md`

---

## File Map

### New Files
| File | Responsibility |
|------|---------------|
| `internal/adapters/storage/sqlite/migrations/000018_cert_entry_ebay_export.up.sql` | Add `card_year` and `ebay_export_flagged_at` columns |
| `internal/adapters/storage/sqlite/migrations/000018_cert_entry_ebay_export.down.sql` | Rollback migration |
| `internal/domain/campaigns/service_cert_entry.go` | `ImportCerts` service method |
| `internal/domain/campaigns/service_cert_entry_test.go` | Tests for cert entry |
| `internal/domain/campaigns/service_export_ebay.go` | `ListEbayExportItems`, `GenerateEbayCSV`, `ClearEbayExportFlags` |
| `internal/domain/campaigns/service_export_ebay_test.go` | Tests for eBay export |
| `internal/domain/campaigns/ebay_types.go` | Types for cert entry + eBay export |
| `web/src/react/pages/tools/CertEntryTab.tsx` | Cert entry UI component |
| `web/src/react/pages/tools/EbayExportTab.tsx` | eBay export UI with price review |

### Modified Files
| File | Changes |
|------|---------|
| `internal/domain/campaigns/types.go` | Add `CardYear`, `EbayExportFlaggedAt` fields to `Purchase` |
| `internal/domain/campaigns/repository.go` | Add repo methods: `SetEbayExportFlag`, `ClearEbayExportFlags`, `ListEbayFlaggedPurchases`, `UpdatePurchaseCardYear` |
| `internal/domain/campaigns/service.go` | Add service interface methods |
| `internal/adapters/storage/sqlite/purchases.go` | Implement new repo methods, update scans to include new columns |
| `internal/adapters/httpserver/handlers/campaigns_imports.go` | Add `HandleImportCerts`, `HandleListEbayExport`, `HandleGenerateEbayCSV` handlers |
| `internal/adapters/httpserver/handlers/campaigns_imports_test.go` | Tests for new handlers |
| `internal/adapters/httpserver/router.go` | Register 3 new routes |
| `internal/testutil/mocks/campaign_service.go` | Add mock function fields + methods |
| `web/src/types/campaigns/core.ts` | Add TS types for cert entry + eBay export |
| `web/src/js/api/campaigns.ts` | Add API client methods |
| `web/src/react/pages/ToolsPage.tsx` | Add Cert Entry and eBay Export tabs |

---

## Task 1: Database Migration

Add `card_year` and `ebay_export_flagged_at` columns to the purchases table.

**Files:**
- Create: `internal/adapters/storage/sqlite/migrations/000018_cert_entry_ebay_export.up.sql`
- Create: `internal/adapters/storage/sqlite/migrations/000018_cert_entry_ebay_export.down.sql`

- [ ] **Step 1: Create the up migration**

```sql
-- 000018_cert_entry_ebay_export.up.sql
ALTER TABLE campaign_purchases ADD COLUMN card_year TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_purchases ADD COLUMN ebay_export_flagged_at TIMESTAMP NULL;
```

- [ ] **Step 2: Create the down migration**

```sql
-- 000018_cert_entry_ebay_export.down.sql
ALTER TABLE campaign_purchases DROP COLUMN card_year;
ALTER TABLE campaign_purchases DROP COLUMN ebay_export_flagged_at;
```

- [ ] **Step 3: Verify migration files exist**

Run: `ls -la internal/adapters/storage/sqlite/migrations/000018*`
Expected: Two files listed

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/storage/sqlite/migrations/000018_cert_entry_ebay_export.up.sql internal/adapters/storage/sqlite/migrations/000018_cert_entry_ebay_export.down.sql
git commit -m "feat: add migration for card_year and ebay_export_flagged_at"
```

---

## Task 2: Domain Types

Add new fields to `Purchase` and create types for cert entry and eBay export.

**Files:**
- Modify: `internal/domain/campaigns/types.go` (Purchase struct, ~line 111)
- Create: `internal/domain/campaigns/ebay_types.go`

- [ ] **Step 1: Add fields to Purchase struct**

In `internal/domain/campaigns/types.go`, add these two fields to the `Purchase` struct (after the `AISuggestedAt` field, before `CreatedAt`):

```go
CardYear              string     `json:"cardYear,omitempty"`
EbayExportFlaggedAt   *time.Time `json:"ebayExportFlaggedAt,omitempty"`
```

- [ ] **Step 2: Create ebay_types.go with all new types**

Create `internal/domain/campaigns/ebay_types.go`:

```go
package campaigns

// CertImportRequest holds the input for cert-based import.
type CertImportRequest struct {
	CertNumbers []string `json:"certNumbers"`
}

// CertImportResult holds the outcome of a cert-based import.
type CertImportResult struct {
	Imported       int              `json:"imported"`
	AlreadyExisted int              `json:"alreadyExisted"`
	Failed         int              `json:"failed"`
	Errors         []CertImportError `json:"errors"`
}

// CertImportError describes a single cert that failed to import.
type CertImportError struct {
	CertNumber string `json:"certNumber"`
	Error      string `json:"error"`
}

// EbayExportItem holds one purchase's data for the eBay export review screen.
type EbayExportItem struct {
	PurchaseID        string  `json:"purchaseId"`
	CertNumber        string  `json:"certNumber"`
	CardName          string  `json:"cardName"`
	SetName           string  `json:"setName"`
	CardNumber        string  `json:"cardNumber"`
	CardYear          string  `json:"cardYear"`
	GradeValue        float64 `json:"gradeValue"`
	Grader            string  `json:"grader"`
	CLValueCents      int     `json:"clValueCents"`
	MarketMedianCents int     `json:"marketMedianCents"`
	SuggestedPriceCents int   `json:"suggestedPriceCents"`
	HasCLValue        bool    `json:"hasCLValue"`
	HasMarketData     bool    `json:"hasMarketData"`
	FrontImageURL     string  `json:"frontImageUrl,omitempty"`
	BackImageURL      string  `json:"backImageUrl,omitempty"`
}

// EbayExportListResponse is the API response for listing items to export.
type EbayExportListResponse struct {
	Items []EbayExportItem `json:"items"`
}

// EbayExportGenerateItem is one item in the generate request with the user's chosen price.
type EbayExportGenerateItem struct {
	PurchaseID string `json:"purchaseId"`
	PriceCents int    `json:"priceCents"`
}

// EbayExportGenerateRequest is the request body for generating the eBay CSV.
type EbayExportGenerateRequest struct {
	Items []EbayExportGenerateItem `json:"items"`
}
```

- [ ] **Step 3: Verify build**

Run: `cd /workspace && go build ./internal/domain/campaigns/...`
Expected: Build succeeds (new types are standalone, no broken references yet)

- [ ] **Step 4: Commit**

```bash
git add internal/domain/campaigns/types.go internal/domain/campaigns/ebay_types.go
git commit -m "feat: add Purchase.CardYear, EbayExportFlaggedAt fields and eBay export types"
```

---

## Task 3: Repository Interface & SQLite Implementation

Add repository methods for the new columns and update SQL scan lists.

**Files:**
- Modify: `internal/domain/campaigns/repository.go`
- Modify: `internal/adapters/storage/sqlite/purchases.go`

- [ ] **Step 1: Add repository interface methods**

In `internal/domain/campaigns/repository.go`, add these methods to `PurchaseRepository`:

```go
// eBay export flag management
SetEbayExportFlag(ctx context.Context, purchaseID string, flaggedAt time.Time) error
ClearEbayExportFlags(ctx context.Context, purchaseIDs []string) error
ListEbayFlaggedPurchases(ctx context.Context) ([]Purchase, error)
UpdatePurchaseCardYear(ctx context.Context, id string, year string) error
```

- [ ] **Step 2: Update SQLite column lists**

In `internal/adapters/storage/sqlite/purchases.go`, find the column lists used in SELECT and INSERT queries. Add `card_year` and `ebay_export_flagged_at` to:
1. The SELECT column list used by `scanPurchase` / purchase scanning
2. The INSERT statement in `CreatePurchase`
3. The `scanPurchase` function's Scan call

The exact locations depend on how the file is structured — look for existing column references like `ai_suggested_at` (the most recently added column from migration 000017) and add the new columns after it.

For scanning, `card_year` scans to `p.CardYear` (string), and `ebay_export_flagged_at` scans to `p.EbayExportFlaggedAt` (*time.Time, nullable).

- [ ] **Step 3: Implement SetEbayExportFlag**

```go
func (r *CampaignsRepository) SetEbayExportFlag(ctx context.Context, purchaseID string, flaggedAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE purchases SET ebay_export_flagged_at = ? WHERE id = ?`,
		flaggedAt, purchaseID)
	return err
}
```

- [ ] **Step 4: Implement ClearEbayExportFlags**

```go
func (r *CampaignsRepository) ClearEbayExportFlags(ctx context.Context, purchaseIDs []string) error {
	if len(purchaseIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(purchaseIDs))
	args := make([]any, len(purchaseIDs))
	for i, id := range purchaseIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `UPDATE purchases SET ebay_export_flagged_at = NULL WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}
```

- [ ] **Step 5: Implement ListEbayFlaggedPurchases**

```go
func (r *CampaignsRepository) ListEbayFlaggedPurchases(ctx context.Context) ([]Purchase, error) {
	// Reuse the same SELECT columns as ListAllUnsoldPurchases but filter on ebay_export_flagged_at IS NOT NULL
	// and sold_id IS NULL (unsold only)
}
```

Follow the pattern of `ListAllUnsoldPurchases` but add `AND p.ebay_export_flagged_at IS NOT NULL` to the WHERE clause.

- [ ] **Step 6: Implement UpdatePurchaseCardYear**

```go
func (r *CampaignsRepository) UpdatePurchaseCardYear(ctx context.Context, id string, year string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE purchases SET card_year = ? WHERE id = ?`,
		year, id)
	return err
}
```

- [ ] **Step 7: Verify build**

Run: `cd /workspace && go build ./...`
Expected: Build succeeds

- [ ] **Step 8: Commit**

```bash
git add internal/domain/campaigns/repository.go internal/adapters/storage/sqlite/purchases.go
git commit -m "feat: add repository methods for card_year and ebay_export_flag"
```

---

## Task 4: Service Interface Updates

Add service methods to the interface and update the mock.

**Files:**
- Modify: `internal/domain/campaigns/service.go`
- Modify: `internal/testutil/mocks/campaign_service.go`

- [ ] **Step 1: Add methods to Service interface**

In `internal/domain/campaigns/service.go`, add these methods to the `Service` interface (near the MatchShopifyPrices method):

```go
// Cert entry
ImportCerts(ctx context.Context, certNumbers []string) (*CertImportResult, error)

// eBay export
ListEbayExportItems(ctx context.Context, flaggedOnly bool) (*EbayExportListResponse, error)
GenerateEbayCSV(ctx context.Context, items []EbayExportGenerateItem) ([]byte, error)
```

- [ ] **Step 2: Add mock function fields**

In `internal/testutil/mocks/campaign_service.go`, add to the `MockCampaignService` struct:

```go
ImportCertsFn         func(ctx context.Context, certNumbers []string) (*campaigns.CertImportResult, error)
ListEbayExportItemsFn func(ctx context.Context, flaggedOnly bool) (*campaigns.EbayExportListResponse, error)
GenerateEbayCSVFn     func(ctx context.Context, items []campaigns.EbayExportGenerateItem) ([]byte, error)
```

- [ ] **Step 3: Add mock method implementations**

```go
func (m *MockCampaignService) ImportCerts(ctx context.Context, certNumbers []string) (*campaigns.CertImportResult, error) {
	if m.ImportCertsFn != nil {
		return m.ImportCertsFn(ctx, certNumbers)
	}
	return nil, nil
}

func (m *MockCampaignService) ListEbayExportItems(ctx context.Context, flaggedOnly bool) (*campaigns.EbayExportListResponse, error) {
	if m.ListEbayExportItemsFn != nil {
		return m.ListEbayExportItemsFn(ctx, flaggedOnly)
	}
	return nil, nil
}

func (m *MockCampaignService) GenerateEbayCSV(ctx context.Context, items []campaigns.EbayExportGenerateItem) ([]byte, error) {
	if m.GenerateEbayCSVFn != nil {
		return m.GenerateEbayCSVFn(ctx, items)
	}
	return nil, nil
}
```

- [ ] **Step 4: Verify build + mock satisfies interface**

Run: `cd /workspace && go build ./...`
Expected: Build succeeds (the `var _ campaigns.Service = (*MockCampaignService)(nil)` line at `campaign_service.go:108` will catch missing methods)

- [ ] **Step 5: Commit**

```bash
git add internal/domain/campaigns/service.go internal/testutil/mocks/campaign_service.go
git commit -m "feat: add ImportCerts, ListEbayExportItems, GenerateEbayCSV to service interface"
```

---

## Task 5: Cert Entry Service Implementation

Implement `ImportCerts` — the core business logic for cert-based import.

**Files:**
- Create: `internal/domain/campaigns/service_cert_entry.go`
- Create: `internal/domain/campaigns/service_cert_entry_test.go`

- [ ] **Step 1: Update mock_repo_test.go with new methods**

The `mockRepo` in `internal/domain/campaigns/mock_repo_test.go` uses map-based storage (not function fields). Add these methods to satisfy the updated `Repository` interface:

```go
func (m *mockRepo) SetEbayExportFlag(_ context.Context, purchaseID string, flaggedAt time.Time) error {
	p, ok := m.purchases[purchaseID]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.EbayExportFlaggedAt = &flaggedAt
	return nil
}

func (m *mockRepo) ClearEbayExportFlags(_ context.Context, purchaseIDs []string) error {
	for _, id := range purchaseIDs {
		if p, ok := m.purchases[id]; ok {
			p.EbayExportFlaggedAt = nil
		}
	}
	return nil
}

func (m *mockRepo) ListEbayFlaggedPurchases(_ context.Context) ([]Purchase, error) {
	var result []Purchase
	for _, p := range m.purchases {
		if p.EbayExportFlaggedAt != nil && !m.purchaseSales[p.ID] {
			result = append(result, *p)
		}
	}
	return result, nil
}

func (m *mockRepo) UpdatePurchaseCardYear(_ context.Context, id string, year string) error {
	p, ok := m.purchases[id]
	if !ok {
		return ErrPurchaseNotFound
	}
	p.CardYear = year
	return nil
}
```

- [ ] **Step 2: Write test for ImportCerts — new cert happy path**

Create `internal/domain/campaigns/service_cert_entry_test.go`:

```go
package campaigns

import (
	"context"
	"testing"
)

// mockCertLookup implements CertLookup for testing.
type mockCertLookup struct {
	lookupFn func(ctx context.Context, certNumber string) (*CertInfo, error)
}

func (m *mockCertLookup) LookupCert(ctx context.Context, certNumber string) (*CertInfo, error) {
	return m.lookupFn(ctx, certNumber)
}

func TestImportCerts_NewCert(t *testing.T) {
	repo := newMockRepo()
	// Pre-populate external campaign so EnsureExternalCampaign succeeds
	repo.campaigns[ExternalCampaignID] = &Campaign{ID: ExternalCampaignID, Name: ExternalCampaignName}

	certLookup := &mockCertLookup{
		lookupFn: func(_ context.Context, cert string) (*CertInfo, error) {
			return &CertInfo{
				CertNumber: cert,
				CardName:   "Charizard",
				Grade:      8.0,
				Year:       "1999",
				Category:   "BASE SET",
				CardNumber: "4",
				Population: 500,
			}, nil
		},
	}

	svc := &service{
		repo:       repo,
		certLookup: certLookup,
		idGen:      func() string { return "test-id" },
	}

	result, err := svc.ImportCerts(context.Background(), []string{"12345678"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Imported != 1 {
		t.Errorf("imported = %d, want 1", result.Imported)
	}
	created := repo.purchases["test-id"]
	if created == nil {
		t.Fatal("purchase was not created")
	}
	if created.CertNumber != "12345678" {
		t.Errorf("certNumber = %q, want 12345678", created.CertNumber)
	}
	if created.CardYear != "1999" {
		t.Errorf("cardYear = %q, want 1999", created.CardYear)
	}
	if created.CampaignID != ExternalCampaignID {
		t.Errorf("campaignID = %q, want %q", created.CampaignID, ExternalCampaignID)
	}
	if created.EbayExportFlaggedAt == nil {
		t.Error("expected ebay export flag to be set")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /workspace && go test ./internal/domain/campaigns/ -run TestImportCerts_NewCert -v`
Expected: FAIL — `ImportCerts` method does not exist yet

- [ ] **Step 4: Implement ImportCerts**

Create `internal/domain/campaigns/service_cert_entry.go`:

```go
package campaigns

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ImportCerts imports purchases by PSA certificate numbers.
// New certs are looked up via the PSA API and created as purchases.
// Existing certs are flagged for eBay export without re-importing.
func (s *service) ImportCerts(ctx context.Context, certNumbers []string) (*CertImportResult, error) {
	// Deduplicate and clean input
	seen := make(map[string]bool, len(certNumbers))
	cleaned := make([]string, 0, len(certNumbers))
	for _, cn := range certNumbers {
		cn = strings.TrimSpace(cn)
		if cn == "" || seen[cn] {
			continue
		}
		seen[cn] = true
		cleaned = append(cleaned, cn)
	}

	// Ensure external campaign exists
	_, err := s.EnsureExternalCampaign(ctx)
	if err != nil {
		return nil, fmt.Errorf("ensure external campaign: %w", err)
	}

	result := &CertImportResult{}
	now := time.Now()

	for _, certNum := range cleaned {
		// Check if cert already exists
		existing, _ := s.repo.GetPurchaseByCertNumber(ctx, "PSA", certNum)
		if existing != nil {
			// Flag for eBay export
			_ = s.repo.SetEbayExportFlag(ctx, existing.ID, now)
			result.AlreadyExisted++
			continue
		}

		// Look up cert via PSA API
		if s.certLookup == nil {
			result.Failed++
			result.Errors = append(result.Errors, CertImportError{
				CertNumber: certNum,
				Error:      "cert lookup not configured",
			})
			continue
		}

		info, lookupErr := s.certLookup.LookupCert(ctx, certNum)
		if lookupErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, CertImportError{
				CertNumber: certNum,
				Error:      lookupErr.Error(),
			})
			continue
		}
		if info == nil {
			result.Failed++
			result.Errors = append(result.Errors, CertImportError{
				CertNumber: certNum,
				Error:      "cert not found",
			})
			continue
		}

		// Resolve set name from PSA category
		setName := info.Category
		if setName != "" {
			resolved := resolvePSACategory(setName)
			if !isGenericSetName(resolved) {
				setName = resolved
			}
		}

		// Build card name with variety
		cardName := info.CardName
		if info.Variety != "" && !strings.Contains(strings.ToUpper(cardName), strings.ToUpper(info.Variety)) {
			cardName = cardName + " " + info.Variety
		}

		purchase := &Purchase{
			ID:                  s.idGen(),
			CampaignID:          ExternalCampaignID,
			CardName:            cardName,
			CertNumber:          certNum,
			CardNumber:          info.CardNumber,
			SetName:             setName,
			Grader:              "PSA",
			GradeValue:          info.Grade,
			Population:          info.Population,
			CardYear:            info.Year,
			BuyCostCents:        0,
			CLValueCents:        0,
			PSASourcingFeeCents: 0,
			PurchaseDate:        now.Format("2006-01-02"),
			PSAListingTitle:     info.Subject,
			EbayExportFlaggedAt: &now,
			CreatedAt:           now,
			UpdatedAt:           now,
		}

		if createErr := s.repo.CreatePurchase(ctx, purchase); createErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, CertImportError{
				CertNumber: certNum,
				Error:      createErr.Error(),
			})
			continue
		}

		// Queue for background cert enrichment (images via GetImages(), market snapshots).
		// Image fetching is deferred to the enrichment worker since each GetImages call
		// counts against the same 100/day PSA API rate limit as LookupCert.
		if s.certEnrichCh != nil {
			select {
			case s.certEnrichCh <- certNum:
			default:
				// Channel full — will be enriched later
			}
		}

		result.Imported++
	}

	return result, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /workspace && go test ./internal/domain/campaigns/ -run TestImportCerts_NewCert -v`
Expected: PASS

- [ ] **Step 6: Write test for existing cert**

Add to `service_cert_entry_test.go`:

```go
func TestImportCerts_ExistingCert(t *testing.T) {
	repo := newMockRepo()
	// Pre-populate existing purchase
	repo.purchases["existing-id"] = &Purchase{
		ID: "existing-id", CertNumber: "12345678", Grader: "PSA",
	}
	repo.certNumbers["12345678"] = true

	svc := &service{repo: repo, idGen: func() string { return "test-id" }}
	result, err := svc.ImportCerts(context.Background(), []string{"12345678"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AlreadyExisted != 1 {
		t.Errorf("alreadyExisted = %d, want 1", result.AlreadyExisted)
	}
	if repo.purchases["existing-id"].EbayExportFlaggedAt == nil {
		t.Error("expected ebay export flag to be set")
	}
}
```

- [ ] **Step 7: Write test for deduplication**

```go
func TestImportCerts_Deduplication(t *testing.T) {
	repo := newMockRepo()
	repo.campaigns[ExternalCampaignID] = &Campaign{ID: ExternalCampaignID}

	lookupCount := 0
	idCounter := 0
	certLookup := &mockCertLookup{
		lookupFn: func(_ context.Context, _ string) (*CertInfo, error) {
			lookupCount++
			return &CertInfo{CertNumber: "111", CardName: "Test", Grade: 9}, nil
		},
	}

	svc := &service{
		repo: repo, certLookup: certLookup,
		idGen: func() string { idCounter++; return fmt.Sprintf("id-%d", idCounter) },
	}
	result, _ := svc.ImportCerts(context.Background(), []string{"111", "111", " 111 ", ""})
	if result.Imported != 1 {
		t.Errorf("imported = %d, want 1 (duplicates removed)", result.Imported)
	}
	if lookupCount != 1 {
		t.Errorf("lookup called %d times, want 1", lookupCount)
	}
}
```

- [ ] **Step 8: Run all cert entry tests**

Run: `cd /workspace && go test ./internal/domain/campaigns/ -run TestImportCerts -v`
Expected: All PASS

- [ ] **Step 9: Commit**

```bash
git add internal/domain/campaigns/service_cert_entry.go internal/domain/campaigns/service_cert_entry_test.go internal/domain/campaigns/mock_repo_test.go
git commit -m "feat: implement ImportCerts service method with tests"
```

---

## Task 6: eBay Export Service Implementation

Implement `ListEbayExportItems` and `GenerateEbayCSV`.

**Files:**
- Create: `internal/domain/campaigns/service_export_ebay.go`
- Create: `internal/domain/campaigns/service_export_ebay_test.go`

- [ ] **Step 1: Write test for ListEbayExportItems**

Create `internal/domain/campaigns/service_export_ebay_test.go`:

```go
package campaigns

import (
	"context"
	"testing"
	"time"
)

func TestListEbayExportItems_FlaggedOnly(t *testing.T) {
	now := time.Now()
	repo := newMockRepo()
	repo.purchases["p1"] = &Purchase{
		ID: "p1", CertNumber: "111", CardName: "Charizard", SetName: "Base Set",
		CardNumber: "4", CardYear: "1999", GradeValue: 8, Grader: "PSA",
		CLValueCents: 25000, EbayExportFlaggedAt: &now,
		MarketSnapshotData: MarketSnapshotData{MedianCents: 27500},
	}

	svc := &service{repo: repo, idGen: func() string { return "id" }}
	resp, err := svc.ListEbayExportItems(context.Background(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(resp.Items))
	}
	item := resp.Items[0]
	if item.SuggestedPriceCents != 25000 {
		t.Errorf("suggestedPrice = %d, want 25000 (CL value)", item.SuggestedPriceCents)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspace && go test ./internal/domain/campaigns/ -run TestListEbayExportItems -v`
Expected: FAIL — method not yet implemented

- [ ] **Step 3: Implement ListEbayExportItems**

Create `internal/domain/campaigns/service_export_ebay.go`:

```go
package campaigns

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/guarzo/slabledger/internal/domain/mathutil"
)

// ListEbayExportItems returns purchases for the eBay export review screen.
func (s *service) ListEbayExportItems(ctx context.Context, flaggedOnly bool) (*EbayExportListResponse, error) {
	var purchases []Purchase
	var err error

	if flaggedOnly {
		purchases, err = s.repo.ListEbayFlaggedPurchases(ctx)
	} else {
		purchases, err = s.repo.ListAllUnsoldPurchases(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("list purchases for ebay export: %w", err)
	}

	items := make([]EbayExportItem, 0, len(purchases))
	for _, p := range purchases {
		if p.Grader != "PSA" {
			continue
		}

		hasCL := p.CLValueCents > 0
		hasMarket := p.MedianCents > 0

		// Suggested price: prefer CL value, fall back to market median
		suggested := p.CLValueCents
		if suggested <= 0 {
			suggested = p.MedianCents
		}

		items = append(items, EbayExportItem{
			PurchaseID:          p.ID,
			CertNumber:          p.CertNumber,
			CardName:            p.CardName,
			SetName:             p.SetName,
			CardNumber:          p.CardNumber,
			CardYear:            p.CardYear,
			GradeValue:          p.GradeValue,
			Grader:              p.Grader,
			CLValueCents:        p.CLValueCents,
			MarketMedianCents:   p.MedianCents,
			SuggestedPriceCents: suggested,
			HasCLValue:          hasCL,
			HasMarketData:       hasMarket,
			FrontImageURL:       p.FrontImageURL,
			BackImageURL:        p.BackImageURL,
		})
	}

	return &EbayExportListResponse{Items: items}, nil
}

// GenerateEbayCSV builds an eBay File Exchange CSV from user-confirmed prices.
// Returns the CSV content as bytes.
func (s *service) GenerateEbayCSV(ctx context.Context, items []EbayExportGenerateItem) ([]byte, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no items to export")
	}

	// Validate: no zero prices
	for _, item := range items {
		if item.PriceCents <= 0 {
			return nil, fmt.Errorf("item %s has invalid price %d: must be > 0", item.PurchaseID, item.PriceCents)
		}
	}

	// Look up all purchases
	purchaseIDs := make([]string, len(items))
	priceMap := make(map[string]int, len(items))
	for i, item := range items {
		purchaseIDs[i] = item.PurchaseID
		priceMap[item.PurchaseID] = item.PriceCents
	}

	purchases := make(map[string]*Purchase, len(items))
	for _, id := range purchaseIDs {
		p, err := s.repo.GetPurchase(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("get purchase %s: %w", id, err)
		}
		purchases[id] = p
	}

	// Build CSV
	var buf bytes.Buffer

	// Line 1: Metadata
	buf.WriteString("Info,Version=1.0.0,Template=fx_category_template_EBAY_US\n")

	// Line 2: Headers
	w := csv.NewWriter(&buf)
	headers := []string{
		"*Action(SiteID=US|Country=US|Currency=USD|Version=1193|CC=UTF-8)",
		"CustomLabel",
		"*Title",
		"*C:Card Name",
		"*C:Set",
		"*C:Card Number",
		"CD:Grade - (ID: 27502)",
		"CD:Professional Grader - (ID: 27501)",
		"CDA:Certification Number - (ID: 27503)",
		"CD:Card Condition - (ID: 40001)",
		"*C:Rarity",
		"C:Year Manufactured",
		"C:Language",
		"*StartPrice",
		"PicURL",
		"*Description",
	}
	if err := w.Write(headers); err != nil {
		return nil, fmt.Errorf("write headers: %w", err)
	}

	// Data rows
	for _, item := range items {
		p := purchases[item.PurchaseID]
		priceDollars := mathutil.ToDollars(int64(item.PriceCents))

		setPrefix := "Pokemon "
		if isJapaneseSet(p.SetName) {
			setPrefix = "Pokemon Japanese "
		}

		gradeStr := formatGrade(p.GradeValue)
		title := buildEbayTitle(p.CardName, p.SetName, p.CardNumber, gradeStr)

		var picURL string
		var pics []string
		if p.FrontImageURL != "" {
			pics = append(pics, p.FrontImageURL)
		}
		if p.BackImageURL != "" {
			pics = append(pics, p.BackImageURL)
		}
		if len(pics) > 0 {
			picURL = strings.Join(pics, " | ")
		}

		description := fmt.Sprintf("<p>%s from %s, card number %s. PSA %s.</p>",
			p.CardName, p.SetName, p.CardNumber, gradeStr)

		row := []string{
			"Add",                                                // Action
			fmt.Sprintf("PSA-%s", p.CertNumber),                 // CustomLabel
			title,                                                // Title
			p.CardName,                                           // Card Name
			setPrefix + p.SetName,                                // Set
			p.CardNumber,                                         // Card Number
			gradeStr,                                             // Grade
			"Professional Sports Authenticator (PSA)",            // Professional Grader
			p.CertNumber,                                         // Certification Number
			"",                                                   // Card Condition (graded = empty)
			"",                                                   // Rarity
			p.CardYear,                                           // Year Manufactured
			"",                                                   // Language (auto-detected by upload script)
			fmt.Sprintf("%.2f", priceDollars),                    // StartPrice
			picURL,                                               // PicURL
			description,                                          // Description
		}
		if err := w.Write(row); err != nil {
			return nil, fmt.Errorf("write row: %w", err)
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("csv flush: %w", err)
	}

	// Clear export flags on all exported items
	if err := s.repo.ClearEbayExportFlags(ctx, purchaseIDs); err != nil {
		// Non-fatal: CSV was generated successfully
		if s.logger != nil {
			s.logger.Warn(ctx, "failed to clear ebay export flags", observability.Err(err))
		}
	}

	return buf.Bytes(), nil
}

// isJapaneseSet returns true if the set name indicates a Japanese set.
func isJapaneseSet(setName string) bool {
	upper := strings.ToUpper(setName)
	return strings.Contains(upper, "JAPANESE") || strings.HasPrefix(upper, "JA ")
}

// formatGrade converts a float64 grade to a clean string (e.g. 8 → "8", 9.5 → "9.5").
func formatGrade(grade float64) string {
	if grade == float64(int(grade)) {
		return fmt.Sprintf("%d", int(grade))
	}
	return fmt.Sprintf("%.1f", grade)
}

// buildEbayTitle builds the eBay listing title.
func buildEbayTitle(cardName, setName, cardNumber, grade string) string {
	return fmt.Sprintf("%s Pokemon %s %s PSA %s", cardName, setName, cardNumber, grade)
}
```

Note: add `"github.com/guarzo/slabledger/internal/domain/observability"` to imports for the logger call in GenerateEbayCSV.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspace && go test ./internal/domain/campaigns/ -run TestListEbayExportItems -v`
Expected: PASS

- [ ] **Step 5: Write test for GenerateEbayCSV**

Add to `service_export_ebay_test.go`:

```go
func TestGenerateEbayCSV_Success(t *testing.T) {
	repo := newMockRepo()
	repo.purchases["p1"] = &Purchase{
		ID: "p1", CertNumber: "12345678", CardName: "Charizard",
		SetName: "Base Set", CardNumber: "4", CardYear: "1999",
		GradeValue: 8, Grader: "PSA",
		FrontImageURL: "https://example.com/front.jpg",
		BackImageURL:  "https://example.com/back.jpg",
	}

	svc := &service{repo: repo, idGen: func() string { return "id" }}
	csvBytes, err := svc.GenerateEbayCSV(context.Background(), []EbayExportGenerateItem{
		{PurchaseID: "p1", PriceCents: 25000},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := string(csvBytes)
	if !strings.Contains(content, "Info,Version=1.0.0") {
		t.Error("missing metadata line")
	}
	if !strings.Contains(content, "PSA-12345678") {
		t.Error("missing CustomLabel")
	}
	if !strings.Contains(content, "250.00") {
		t.Error("missing StartPrice")
	}
	if !strings.Contains(content, "front.jpg") {
		t.Error("missing front image")
	}
}

func TestGenerateEbayCSV_RejectsZeroPrice(t *testing.T) {
	svc := &service{idGen: func() string { return "id" }}
	_, err := svc.GenerateEbayCSV(context.Background(), []EbayExportGenerateItem{
		{PurchaseID: "p1", PriceCents: 0},
	})
	if err == nil {
		t.Fatal("expected error for zero price")
	}
}
```

- [ ] **Step 6: Run all eBay export tests**

Run: `cd /workspace && go test ./internal/domain/campaigns/ -run TestGenerateEbayCSV -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/domain/campaigns/service_export_ebay.go internal/domain/campaigns/service_export_ebay_test.go
git commit -m "feat: implement ListEbayExportItems and GenerateEbayCSV with tests"
```

---

## Task 7: Update enrichSingleCert to Persist CardYear

The existing cert enrichment worker should save the year from PSA cert lookup.

**Files:**
- Modify: `internal/domain/campaigns/service_import_psa.go` (~line 366, `enrichSingleCert`)

- [ ] **Step 1: Add year persistence to enrichSingleCert**

In `internal/domain/campaigns/service_import_psa.go`, inside `enrichSingleCert()`, after the `UpdatePurchaseCardMetadata` call (~line 412), add:

```go
if info.Year != "" && purchase.CardYear == "" {
	_ = s.repo.UpdatePurchaseCardYear(ctx, purchase.ID, info.Year)
}
```

- [ ] **Step 2: Verify build**

Run: `cd /workspace && go build ./internal/domain/campaigns/...`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add internal/domain/campaigns/service_import_psa.go
git commit -m "feat: persist card year during cert enrichment"
```

---

## Task 8: HTTP Handlers

Add three new handlers for cert import, eBay export listing, and eBay CSV generation.

**Files:**
- Modify: `internal/adapters/httpserver/handlers/campaigns_imports.go`
- Modify: `internal/adapters/httpserver/handlers/campaigns_imports_test.go`
- Modify: `internal/adapters/httpserver/router.go`

- [ ] **Step 1: Write test for HandleImportCerts**

Add to `internal/adapters/httpserver/handlers/campaigns_imports_test.go`:

Note: Handler tests are in the `handlers` package (same package), so use `NewCampaignsHandler` not `handlers.NewCampaignsHandler`. Use `mocks.NewMockLogger()` for the logger argument.

```go
func TestHandleImportCerts_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ImportCertsFn: func(_ context.Context, certs []string) (*campaigns.CertImportResult, error) {
			return &campaigns.CertImportResult{
				Imported:       len(certs),
				AlreadyExisted: 0,
				Failed:         0,
			}, nil
		},
	}
	h := NewCampaignsHandler(svc, mocks.NewMockLogger(), nil, nil)

	body := strings.NewReader(`{"certNumbers":["111","222"]}`)
	req := httptest.NewRequest("POST", "/api/purchases/import-certs", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleImportCerts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var result campaigns.CertImportResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Imported != 2 {
		t.Errorf("imported = %d, want 2", result.Imported)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /workspace && go test ./internal/adapters/httpserver/handlers/ -run TestHandleImportCerts -v`
Expected: FAIL — method doesn't exist

- [ ] **Step 3: Implement HandleImportCerts**

Add to `internal/adapters/httpserver/handlers/campaigns_imports.go`:

```go
// HandleImportCerts handles POST /api/purchases/import-certs.
// Accepts a JSON body with certificate numbers for direct PSA import.
func (h *CampaignsHandler) HandleImportCerts(w http.ResponseWriter, r *http.Request) {
	var req campaigns.CertImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if len(req.CertNumbers) == 0 {
		writeError(w, http.StatusBadRequest, "No certificate numbers provided")
		return
	}

	result, err := h.service.ImportCerts(r.Context(), req.CertNumbers)
	if err != nil {
		h.logger.Error(r.Context(), "cert import failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /workspace && go test ./internal/adapters/httpserver/handlers/ -run TestHandleImportCerts -v`
Expected: PASS

- [ ] **Step 5: Implement HandleListEbayExport**

```go
// HandleListEbayExport handles GET /api/purchases/export-ebay.
// Returns items for the eBay export review screen.
func (h *CampaignsHandler) HandleListEbayExport(w http.ResponseWriter, r *http.Request) {
	flaggedOnly := r.URL.Query().Get("flagged_only") == "true"
	resp, err := h.service.ListEbayExportItems(r.Context(), flaggedOnly)
	if err != nil {
		h.logger.Error(r.Context(), "list ebay export items failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
```

- [ ] **Step 6: Implement HandleGenerateEbayCSV**

```go
// HandleGenerateEbayCSV handles POST /api/purchases/export-ebay/generate.
// Generates and returns an eBay File Exchange CSV file.
func (h *CampaignsHandler) HandleGenerateEbayCSV(w http.ResponseWriter, r *http.Request) {
	var req campaigns.EbayExportGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "No items provided")
		return
	}

	csvBytes, err := h.service.GenerateEbayCSV(r.Context(), req.Items)
	if err != nil {
		h.logger.Error(r.Context(), "generate ebay CSV failed", observability.Err(err))
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=ebay_import.csv")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(csvBytes)
}
```

- [ ] **Step 7: Register routes**

In `internal/adapters/httpserver/router.go`, add near the other purchase import/export routes:

```go
mux.Handle("POST /api/purchases/import-certs", authRoute(rt.campaignsHandler.HandleImportCerts))
mux.Handle("GET /api/purchases/export-ebay", authRoute(rt.campaignsHandler.HandleListEbayExport))
mux.Handle("POST /api/purchases/export-ebay/generate", authRoute(rt.campaignsHandler.HandleGenerateEbayCSV))
```

- [ ] **Step 8: Verify full build**

Run: `cd /workspace && go build ./...`
Expected: Build succeeds

- [ ] **Step 9: Run all handler tests**

Run: `cd /workspace && go test ./internal/adapters/httpserver/handlers/ -v -count=1`
Expected: All PASS

- [ ] **Step 10: Commit**

```bash
git add internal/adapters/httpserver/handlers/campaigns_imports.go internal/adapters/httpserver/handlers/campaigns_imports_test.go internal/adapters/httpserver/router.go
git commit -m "feat: add HTTP handlers and routes for cert import and eBay export"
```

---

## Task 9: Frontend Types & API Client

Add TypeScript types and API methods for the new endpoints.

**Files:**
- Modify: `web/src/types/campaigns/core.ts`
- Modify: `web/src/js/api/campaigns.ts`

- [ ] **Step 1: Add TypeScript types**

Add to `web/src/types/campaigns/core.ts`:

```typescript
// Cert entry
export interface CertImportRequest {
  certNumbers: string[];
}

export interface CertImportError {
  certNumber: string;
  error: string;
}

export interface CertImportResult {
  imported: number;
  alreadyExisted: number;
  failed: number;
  errors: CertImportError[];
}

// eBay export
export interface EbayExportItem {
  purchaseId: string;
  certNumber: string;
  cardName: string;
  setName: string;
  cardNumber: string;
  cardYear: string;
  gradeValue: number;
  grader: string;
  clValueCents: number;
  marketMedianCents: number;
  suggestedPriceCents: number;
  hasCLValue: boolean;
  hasMarketData: boolean;
  frontImageUrl?: string;
  backImageUrl?: string;
}

export interface EbayExportListResponse {
  items: EbayExportItem[];
}

export interface EbayExportGenerateItem {
  purchaseId: string;
  priceCents: number;
}
```

- [ ] **Step 2: Add API client methods**

In `web/src/js/api/campaigns.ts`, add to the `declare module` block:

```typescript
importCerts(certNumbers: string[]): Promise<CertImportResult>;
listEbayExportItems(flaggedOnly: boolean): Promise<EbayExportListResponse>;
generateEbayCSV(items: EbayExportGenerateItem[]): Promise<Blob>;
```

And add the implementations:

```typescript
proto.importCerts = async function(
  this: APIClient, certNumbers: string[],
): Promise<CertImportResult> {
  return this.post<CertImportResult>('/purchases/import-certs', { certNumbers });
};

proto.listEbayExportItems = async function(
  this: APIClient, flaggedOnly: boolean,
): Promise<EbayExportListResponse> {
  const params = flaggedOnly ? '?flagged_only=true' : '';
  return this.get<EbayExportListResponse>(`/purchases/export-ebay${params}`);
};

proto.generateEbayCSV = async function(
  this: APIClient, items: EbayExportGenerateItem[],
): Promise<Blob> {
  const response = await this.fetchWithRetry(
    `${this.baseURL}/purchases/export-ebay/generate`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ items }),
    },
  );
  return response.blob();
};
```

Note: The `generateEbayCSV` method follows the same pattern as `globalExportCL` — uses `fetchWithRetry` directly and calls `.blob()` on the response, since `post<T>` always calls `.json()` and can't return a Blob.

- [ ] **Step 3: Add imports**

Make sure the new types (`CertImportResult`, `EbayExportListResponse`, `EbayExportGenerateItem`, `EbayExportItem`) are imported in the campaigns.ts API file from `'@/types/campaigns/core'`.

- [ ] **Step 4: Verify frontend build**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 5: Commit**

```bash
git add web/src/types/campaigns/core.ts web/src/js/api/campaigns.ts
git commit -m "feat: add frontend types and API methods for cert import and eBay export"
```

---

## Task 10: Cert Entry Tab Component

Build the cert entry UI as a new tab on the Tools page.

**Files:**
- Create: `web/src/react/pages/tools/CertEntryTab.tsx`
- Modify: `web/src/react/pages/ToolsPage.tsx`

- [ ] **Step 1: Create CertEntryTab component**

Create `web/src/react/pages/tools/CertEntryTab.tsx`:

```tsx
import { useState } from 'react';
import { api } from '@/js/api';
import type { CertImportResult } from '@/types/campaigns/core';

export default function CertEntryTab() {
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<CertImportResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  const handleImport = async () => {
    const certNumbers = input
      .split('\n')
      .map(s => s.trim())
      .filter(s => s.length > 0);

    if (certNumbers.length === 0) {
      setError('Please enter at least one certificate number');
      return;
    }

    setLoading(true);
    setError(null);
    setResult(null);

    try {
      const res = await api.importCerts(certNumbers);
      setResult(res);
      if (res.imported > 0 || res.alreadyExisted > 0) {
        setInput('');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-4">
      <p className="text-sm text-gray-400">
        Paste PSA certificate numbers (one per line) to import cards directly.
        Existing certs will be flagged for eBay export.
      </p>

      <textarea
        value={input}
        onChange={e => setInput(e.target.value)}
        placeholder={"12345678\n87654321\n11223344"}
        rows={10}
        className="w-full rounded border border-gray-700 bg-gray-900 p-3 font-mono text-sm text-gray-100 placeholder-gray-600 focus:border-blue-500 focus:outline-none"
        disabled={loading}
      />

      <button
        onClick={handleImport}
        disabled={loading || input.trim().length === 0}
        className="rounded bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50"
      >
        {loading ? 'Importing...' : 'Import Certificates'}
      </button>

      {error && (
        <div className="rounded border border-red-700 bg-red-900/30 p-3 text-sm text-red-300">
          {error}
        </div>
      )}

      {result && (
        <div className="space-y-2 rounded border border-gray-700 bg-gray-800 p-4">
          <h3 className="text-sm font-medium text-gray-200">Import Results</h3>
          <div className="grid grid-cols-3 gap-4 text-sm">
            <div>
              <span className="text-green-400">{result.imported}</span>{' '}
              <span className="text-gray-400">imported</span>
            </div>
            <div>
              <span className="text-blue-400">{result.alreadyExisted}</span>{' '}
              <span className="text-gray-400">already existed</span>
            </div>
            <div>
              <span className="text-red-400">{result.failed}</span>{' '}
              <span className="text-gray-400">failed</span>
            </div>
          </div>

          {result.errors.length > 0 && (
            <div className="mt-2 space-y-1">
              <h4 className="text-xs font-medium text-gray-400">Errors:</h4>
              {result.errors.map((e, i) => (
                <div key={i} className="text-xs text-red-400">
                  Cert {e.certNumber}: {e.error}
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Add Cert Entry tab to ToolsPage**

In `web/src/react/pages/ToolsPage.tsx`:

1. Import: `import CertEntryTab from './tools/CertEntryTab';`
2. Add to `TABS` array: `{ id: 'cert-entry', label: 'Cert Entry' }`
3. Add Tabs.Content:
```tsx
<Tabs.Content value="cert-entry">
  <CertEntryTab />
</Tabs.Content>
```

- [ ] **Step 3: Verify frontend build**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 4: Commit**

```bash
git add web/src/react/pages/tools/CertEntryTab.tsx web/src/react/pages/ToolsPage.tsx
git commit -m "feat: add Cert Entry tab to Tools page"
```

---

## Task 11: eBay Export Tab Component

Build the eBay export UI with price review, following the ShopifySyncPage pattern.

**Files:**
- Create: `web/src/react/pages/tools/EbayExportTab.tsx`
- Modify: `web/src/react/pages/ToolsPage.tsx`

- [ ] **Step 1: Create EbayExportTab component**

Create `web/src/react/pages/tools/EbayExportTab.tsx`. This is a two-phase component:

**Phase 1 — Review**: Fetch items, show in a table with Accept/Edit/Skip per row.
**Phase 2 — Export**: Download the CSV.

```tsx
import { useState, useCallback } from 'react';
import { api } from '@/js/api';
import type { EbayExportItem, EbayExportGenerateItem } from '@/types/campaigns/core';

type Decision = { action: 'accept' | 'edit'; priceCents: number } | { action: 'skip' };
type Phase = 'review' | 'export';

function centsToDollars(cents: number): string {
  return (cents / 100).toFixed(2);
}

function dollarsToCents(dollars: string): number {
  return Math.round(parseFloat(dollars) * 100);
}

export default function EbayExportTab() {
  const [phase, setPhase] = useState<Phase>('review');
  const [items, setItems] = useState<EbayExportItem[]>([]);
  const [decisions, setDecisions] = useState<Map<string, Decision>>(new Map());
  const [flaggedOnly, setFlaggedOnly] = useState(true);
  const [loading, setLoading] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editPrice, setEditPrice] = useState('');
  const [exportCount, setExportCount] = useState(0);

  const fetchItems = useCallback(async () => {
    setLoading(true);
    try {
      const resp = await api.listEbayExportItems(flaggedOnly);
      setItems(resp.items);
      setDecisions(new Map());
    } catch (err) {
      console.error('Failed to fetch export items:', err);
    } finally {
      setLoading(false);
    }
  }, [flaggedOnly]);

  const setDecision = (purchaseId: string, decision: Decision) => {
    setDecisions(prev => new Map(prev).set(purchaseId, decision));
  };

  const acceptAll = () => {
    const next = new Map(decisions);
    for (const item of items) {
      if (item.suggestedPriceCents > 0) {
        next.set(item.purchaseId, { action: 'accept', priceCents: item.suggestedPriceCents });
      }
    }
    setDecisions(next);
  };

  const skipAll = () => {
    const next = new Map(decisions);
    for (const item of items) {
      next.set(item.purchaseId, { action: 'skip' });
    }
    setDecisions(next);
  };

  const handleEdit = (id: string, currentCents: number) => {
    setEditingId(id);
    setEditPrice(centsToDollars(currentCents));
  };

  const confirmEdit = (id: string) => {
    const cents = dollarsToCents(editPrice);
    if (cents > 0) {
      setDecision(id, { action: 'edit', priceCents: cents });
    }
    setEditingId(null);
  };

  const handleExport = async () => {
    const exportItems: EbayExportGenerateItem[] = [];
    for (const [purchaseId, decision] of decisions) {
      if (decision.action === 'accept' || decision.action === 'edit') {
        exportItems.push({ purchaseId, priceCents: decision.priceCents });
      }
    }

    if (exportItems.length === 0) return;

    setLoading(true);
    try {
      const blob = await api.generateEbayCSV(exportItems);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'ebay_import.csv';
      a.click();
      URL.revokeObjectURL(url);
      setExportCount(exportItems.length);
      setPhase('export');
    } catch (err) {
      console.error('Failed to generate CSV:', err);
    } finally {
      setLoading(false);
    }
  };

  const acceptedCount = Array.from(decisions.values()).filter(
    d => d.action === 'accept' || d.action === 'edit'
  ).length;

  if (phase === 'export') {
    return (
      <div className="rounded border border-green-700 bg-green-900/20 p-6 text-center">
        <h3 className="text-lg font-medium text-green-300">Export Complete</h3>
        <p className="mt-2 text-sm text-gray-400">
          {exportCount} items exported to ebay_import.csv
        </p>
        <button
          onClick={() => { setPhase('review'); setItems([]); setDecisions(new Map()); }}
          className="mt-4 rounded bg-gray-700 px-4 py-2 text-sm text-gray-200 hover:bg-gray-600"
        >
          Start Over
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Controls */}
      <div className="flex items-center gap-4">
        <label className="flex items-center gap-2 text-sm text-gray-300">
          <input
            type="checkbox"
            checked={flaggedOnly}
            onChange={e => setFlaggedOnly(e.target.checked)}
            className="rounded border-gray-600"
          />
          Flagged for export only
        </label>
        <button
          onClick={fetchItems}
          disabled={loading}
          className="rounded bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50"
        >
          {loading ? 'Loading...' : items.length > 0 ? 'Refresh' : 'Load Items'}
        </button>
      </div>

      {items.length > 0 && (
        <>
          {/* Bulk actions + summary */}
          <div className="flex items-center justify-between">
            <div className="flex gap-2">
              <button onClick={acceptAll} className="rounded bg-green-700 px-3 py-1 text-xs text-white hover:bg-green-600">
                Accept All
              </button>
              <button onClick={skipAll} className="rounded bg-gray-700 px-3 py-1 text-xs text-gray-200 hover:bg-gray-600">
                Skip All
              </button>
            </div>
            <div className="text-sm text-gray-400">
              {items.length} items · {acceptedCount} accepted
            </div>
          </div>

          {/* Review table */}
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-gray-700 text-xs text-gray-400">
                <tr>
                  <th className="pb-2 pr-4">Card</th>
                  <th className="pb-2 pr-4">Set</th>
                  <th className="pb-2 pr-4">#</th>
                  <th className="pb-2 pr-4">Grade</th>
                  <th className="pb-2 pr-4">Cert</th>
                  <th className="pb-2 pr-4 text-right">CL Value</th>
                  <th className="pb-2 pr-4 text-right">Market</th>
                  <th className="pb-2 pr-4 text-right">Price</th>
                  <th className="pb-2">Action</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-800">
                {items.map(item => {
                  const decision = decisions.get(item.purchaseId);
                  const priceCents = decision && decision.action !== 'skip'
                    ? decision.priceCents
                    : item.suggestedPriceCents;
                  const isEditing = editingId === item.purchaseId;

                  return (
                    <tr key={item.purchaseId} className="text-gray-300">
                      <td className="py-2 pr-4 font-medium">{item.cardName}</td>
                      <td className="py-2 pr-4">{item.setName}</td>
                      <td className="py-2 pr-4">{item.cardNumber}</td>
                      <td className="py-2 pr-4">PSA {item.gradeValue}</td>
                      <td className="py-2 pr-4 font-mono text-xs">{item.certNumber}</td>
                      <td className="py-2 pr-4 text-right">
                        {item.hasCLValue ? `$${centsToDollars(item.clValueCents)}` : (
                          <span className="text-yellow-500">No CL</span>
                        )}
                      </td>
                      <td className="py-2 pr-4 text-right">
                        {item.hasMarketData ? `$${centsToDollars(item.marketMedianCents)}` : (
                          <span className="text-yellow-500">No Data</span>
                        )}
                      </td>
                      <td className="py-2 pr-4 text-right">
                        {isEditing ? (
                          <div className="flex items-center justify-end gap-1">
                            <span className="text-gray-400">$</span>
                            <input
                              type="number"
                              value={editPrice}
                              onChange={e => setEditPrice(e.target.value)}
                              onKeyDown={e => {
                                if (e.key === 'Enter') confirmEdit(item.purchaseId);
                                if (e.key === 'Escape') setEditingId(null);
                              }}
                              className="w-20 rounded border border-gray-600 bg-gray-800 px-2 py-1 text-right text-sm"
                              autoFocus
                            />
                          </div>
                        ) : (
                          priceCents > 0 ? `$${centsToDollars(priceCents)}` : (
                            <span className="text-red-400">$0</span>
                          )
                        )}
                      </td>
                      <td className="py-2">
                        <div className="flex gap-1">
                          <button
                            onClick={() => setDecision(item.purchaseId, { action: 'accept', priceCents: item.suggestedPriceCents })}
                            disabled={item.suggestedPriceCents <= 0}
                            className={`rounded px-2 py-1 text-xs ${
                              decision?.action === 'accept'
                                ? 'bg-green-600 text-white'
                                : 'bg-gray-700 text-gray-300 hover:bg-green-700 disabled:opacity-30'
                            }`}
                          >
                            Accept
                          </button>
                          <button
                            onClick={() => handleEdit(item.purchaseId, priceCents || item.suggestedPriceCents)}
                            className={`rounded px-2 py-1 text-xs ${
                              decision?.action === 'edit'
                                ? 'bg-blue-600 text-white'
                                : 'bg-gray-700 text-gray-300 hover:bg-blue-700'
                            }`}
                          >
                            Edit
                          </button>
                          <button
                            onClick={() => setDecision(item.purchaseId, { action: 'skip' })}
                            className={`rounded px-2 py-1 text-xs ${
                              decision?.action === 'skip'
                                ? 'bg-red-600 text-white'
                                : 'bg-gray-700 text-gray-300 hover:bg-red-700'
                            }`}
                          >
                            Skip
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          {/* Export button */}
          <div className="flex justify-end">
            <button
              onClick={handleExport}
              disabled={loading || acceptedCount === 0}
              className="rounded bg-green-600 px-6 py-2 text-sm font-medium text-white hover:bg-green-500 disabled:opacity-50"
            >
              {loading ? 'Generating...' : `Export eBay CSV (${acceptedCount} items)`}
            </button>
          </div>
        </>
      )}

      {!loading && items.length === 0 && (
        <p className="text-sm text-gray-500">
          Click "Load Items" to see inventory available for export.
        </p>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Add eBay Export tab to ToolsPage**

In `web/src/react/pages/ToolsPage.tsx`:

1. Import: `import EbayExportTab from './tools/EbayExportTab';`
2. Add to `TABS` array: `{ id: 'ebay-export', label: 'eBay Export' }`
3. Add Tabs.Content:
```tsx
<Tabs.Content value="ebay-export">
  <EbayExportTab />
</Tabs.Content>
```

- [ ] **Step 3: Verify frontend build**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 4: Commit**

```bash
git add web/src/react/pages/tools/EbayExportTab.tsx web/src/react/pages/ToolsPage.tsx
git commit -m "feat: add eBay Export tab with price review to Tools page"
```

---

## Task 12: Full Integration Test

Verify everything compiles and existing tests still pass.

**Files:** None (verification only)

- [ ] **Step 1: Run Go build**

Run: `cd /workspace && go build ./...`
Expected: Build succeeds

- [ ] **Step 2: Run all Go tests**

Run: `cd /workspace && go test ./... -count=1`
Expected: All PASS (no regressions)

- [ ] **Step 3: Run frontend type check**

Run: `cd /workspace/web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 4: Run frontend lint**

Run: `cd /workspace/web && npm run lint`
Expected: No lint errors (or only pre-existing ones)

- [ ] **Step 5: Run golangci-lint**

Run: `cd /workspace && golangci-lint run ./...`
Expected: No new lint issues

- [ ] **Step 6: Fix any issues found and commit**

If any tests or linting issues were found, fix them and commit the fixes.
