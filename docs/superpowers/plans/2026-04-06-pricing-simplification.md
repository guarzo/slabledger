# Pricing Pipeline Simplification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove CardHedger, PriceCharting, JustTCG, and the fusion engine; replace with a DH-backed PriceProvider.

**Architecture:** The current fusion pipeline (5 sources, weighted-median engine) is replaced by a single DHPriceProvider that fetches per-grade prices from DH's market data API. The `pricing.PriceProvider` interface is preserved so all downstream consumers (campaigns, advisor tools, API handlers) work unchanged. CardLadder and TCGdex remain untouched.

**Tech Stack:** Go 1.26, SQLite (WAL mode), hexagonal architecture. Frontend: React + TypeScript + Vite.

**Spec:** `docs/superpowers/specs/2026-04-06-pricing-simplification-design.md`

---

## Phase 0: Quick Fix

### Task 0: Make `db-pull` / `db-push` resilient to large DBs

**Files:**
- Modify: `Makefile:110-162`

- [ ] **Step 1: Add SSH_OPTS variable and apply to db targets**

In `Makefile`, add a variable near the top of the db section (around line 110) and update all `ssh` and `scp` commands in both `db-push` and `db-pull`:

```makefile
# Near line 110, after LOCAL_DB definition:
SSH_OPTS ?= -o ServerAliveInterval=60 -o ServerAliveCountMax=10
```

Then replace every bare `ssh` with `ssh $(SSH_OPTS)` and every `scp` with `scp $(SSH_OPTS)` in both targets. There are 5 `ssh` calls and 1 `scp` in `db-push`, and 3 `ssh` calls and 1 `scp` in `db-pull`.

- [ ] **Step 2: Test the change**

Run: `make help` (should still display correctly)
Run: `make db-pull` (confirm it prompts and the SSH options are visible)

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "fix: add SSH keepalive to db-pull/db-push for large DB transfers"
```

---

## Phase 1: Remove CardHedger

### Task 1: Delete CardHedger client package

**Files:**
- Delete: `internal/adapters/clients/cardhedger/` (5 files, ~1,221 lines)

- [ ] **Step 1: Delete the package**

```bash
rm -rf internal/adapters/clients/cardhedger/
```

- [ ] **Step 2: Verify no surprise dependents**

```bash
grep -r '"github.com/guarzo/slabledger/internal/adapters/clients/cardhedger"' --include='*.go' .
```

Expected: hits in `cmd/slabledger/init.go`, `cmd/slabledger/main.go`, `internal/adapters/scheduler/builder.go`, `internal/adapters/scheduler/cert_sweep_adapter.go`, `internal/adapters/clients/fusionprice/cardhedger_adapter.go`. These will be fixed in subsequent tasks.

- [ ] **Step 3: Commit**

```bash
git add -A internal/adapters/clients/cardhedger/
git commit -m "refactor: delete CardHedger client package"
```

---

### Task 2: Delete CardHedger schedulers and cert sweep

**Files:**
- Delete: `internal/adapters/scheduler/cardhedger_batch.go` (444 lines)
- Delete: `internal/adapters/scheduler/cardhedger_batch_discovery.go` (426 lines)
- Delete: `internal/adapters/scheduler/cardhedger_batch_test.go` (593 lines)
- Delete: `internal/adapters/scheduler/cardhedger_refresh.go` (287 lines)
- Delete: `internal/adapters/scheduler/cardhedger_refresh_test.go` (278 lines)
- Delete: `internal/adapters/scheduler/cardhedger_test_helpers_test.go` (23 lines)
- Delete: `internal/adapters/scheduler/cert_sweep_adapter.go` (57 lines)

- [ ] **Step 1: Delete all CardHedger scheduler files**

```bash
rm internal/adapters/scheduler/cardhedger_batch.go
rm internal/adapters/scheduler/cardhedger_batch_discovery.go
rm internal/adapters/scheduler/cardhedger_batch_test.go
rm internal/adapters/scheduler/cardhedger_refresh.go
rm internal/adapters/scheduler/cardhedger_refresh_test.go
rm internal/adapters/scheduler/cardhedger_test_helpers_test.go
rm internal/adapters/scheduler/cert_sweep_adapter.go
```

- [ ] **Step 2: Commit**

```bash
git add -A internal/adapters/scheduler/
git commit -m "refactor: delete CardHedger schedulers and cert sweep adapter"
```

---

### Task 3: Delete CardHedger fusion adapter

**Files:**
- Delete: `internal/adapters/clients/fusionprice/cardhedger_adapter.go` (440 lines)

- [ ] **Step 1: Delete the adapter**

```bash
rm internal/adapters/clients/fusionprice/cardhedger_adapter.go
```

- [ ] **Step 2: Commit**

```bash
git add internal/adapters/clients/fusionprice/cardhedger_adapter.go
git commit -m "refactor: delete CardHedger fusion adapter"
```

---

### Task 4: Remove CardHedger from scheduler builder

**Files:**
- Modify: `internal/adapters/scheduler/builder.go`

- [ ] **Step 1: Remove the CardHedgerClient interface and deps**

In `builder.go`, remove:
- Lines 22-27: The `CardHedgerClient` interface definition and its composite types
- Lines 40-51: The CardHedger dependency fields from `BuildDeps` (`CardHedgerClient`, `SyncStateStore`, `CardIDMappingLookup`, `CardIDMappingLister`, `CardIDMappingSaver`, `DiscoveryFailureTracker`, `FavoritesLister`, `CampaignCardLister`, `CertSweeper`)
- Line 119: `CardDiscoverer` from `BuildResult`
- The `CardHedgerBatchClient` and `CardHedgerRefreshClient` interface types (find them elsewhere in the scheduler package and delete those files/definitions)
- The `CardDiscoverer` interface type
- All CardHedger scheduler construction code in `BuildGroup()` (search for `CardHedger` within the function — remove the entire conditional blocks that create CardHedger refresh and batch schedulers)
- The variable `cardDiscoverer` from `BuildGroup()` and its assignment to `BuildResult`

Note: `SyncStateStore`, `CardIDMappingLookup/Lister/Saver`, `FavoritesLister`, `CampaignCardLister` may also be used by the DH push scheduler or other schedulers. Only remove fields that are EXCLUSIVELY used by CardHedger schedulers. Check each field's usage with grep before removing.

- [ ] **Step 2: Verify compilation**

```bash
go build ./internal/adapters/scheduler/...
```

Expected: May fail due to references in `cmd/slabledger/`. That's OK — we fix those next.

- [ ] **Step 3: Commit**

```bash
git add internal/adapters/scheduler/builder.go
git commit -m "refactor: remove CardHedger deps from scheduler builder"
```

---

### Task 5: Remove CardHedger from wiring code

**Files:**
- Modify: `cmd/slabledger/init.go`
- Modify: `cmd/slabledger/main.go`

- [ ] **Step 1: Update `initializePriceProviders()` in init.go**

Remove the `cardhedger` import (line 13). Remove `cardHedgerClient` from the function's return signature and body. The function should no longer create a `cardhedger.Client` or pass a `CardHedgerAdapter` to `secondarySources`.

The return signature changes from:
```go
) (priceProvider *fusionprice.FusionPriceProvider, cardHedgerClient *cardhedger.Client, pcProvider *pricecharting.PriceCharting, err error) {
```
to:
```go
) (priceProvider *fusionprice.FusionPriceProvider, pcProvider *pricecharting.PriceCharting, err error) {
```

Remove lines 59-65 (CardHedger client creation and adapter wrapping). The `secondarySources` slice should only contain the DH adapter.

- [ ] **Step 2: Update `initializeCampaignsService()` in init.go**

Remove the `cardHedgerClientImpl *cardhedger.Client` parameter. Remove lines 120-130 (CardHedger cert resolver wiring — the `if cardHedgerClientImpl.Available()` block). Remove the `cardRequestRepo` creation if it's only used for CardHedger.

- [ ] **Step 3: Update `schedulerDeps` struct in init.go**

Remove `CardHedgerClientImpl`, `DiscoveryFailureRepo`, `CertSweeper` fields. Remove the corresponding assignments in `initializeSchedulers()`.

- [ ] **Step 4: Update main.go**

Remove the `cardhedger` import (line 20). Update the call to `initializePriceProviders()` to handle the new return signature (no `cardHedgerClient`). Update the call to `initializeCampaignsService()` to remove the `cardHedgerClientImpl` argument. Remove CardHedger cert sweeper setup (lines 364-372). Remove `CardHedgerStats` from server dependencies.

- [ ] **Step 5: Verify compilation**

```bash
go build ./cmd/slabledger/...
```

Expected: PASS (or failures in test files — fix those next)

- [ ] **Step 6: Commit**

```bash
git add cmd/slabledger/init.go cmd/slabledger/main.go
git commit -m "refactor: remove CardHedger from application wiring"
```

---

### Task 6: Remove CardHedger config and env vars

**Files:**
- Modify: `internal/platform/config/types.go`
- Modify: `internal/platform/config/loader.go`
- Modify: `internal/platform/config/defaults.go`
- Modify: `internal/platform/config/config_test.go`
- Modify: `cmd/slabledger/server.go`
- Modify: `.env.example`

- [ ] **Step 1: Remove config structs**

In `types.go`, remove `CardHedgerSchedulerConfig` struct and the `CardHedgerKey`/`CardHedgerClientID` fields from `AdapterConfig`. Remove the `CardHedger` field from the main `Config` struct.

In `defaults.go`, remove any CardHedger default values.

In `loader.go`, remove all `CARD_HEDGER_*` env var loading.

- [ ] **Step 2: Update server.go env validation**

In `server.go`, remove the `CARD_HEDGER_API_KEY` entry from the env check list (line ~102).

- [ ] **Step 3: Update .env.example**

Remove `CARD_HEDGER_API_KEY`, `CARD_HEDGER_CLIENT_ID`, `CARD_HEDGER_ENABLED`, `CARD_HEDGER_POLL_INTERVAL`, `CARD_HEDGER_BATCH_INTERVAL`, `CARD_HEDGER_MAX_CARDS_PER_RUN`.

- [ ] **Step 4: Update config tests**

Remove or update any tests in `config_test.go` that reference CardHedger config fields.

- [ ] **Step 5: Verify**

```bash
go test ./internal/platform/config/...
go build ./cmd/slabledger/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/platform/config/ cmd/slabledger/server.go .env.example
git commit -m "refactor: remove CardHedger config and env vars"
```

---

### Task 7: Remove CardHedger from API handlers and frontend

**Files:**
- Modify: `internal/adapters/httpserver/handlers/api_status_handler.go`
- Modify: `internal/adapters/httpserver/handlers/price_hints.go`
- Modify: `web/src/react/PriceHintDialog.tsx`
- Modify: `web/src/react/pages/admin/MissingCardsTab.tsx`
- Modify: `web/src/react/pages/admin/ApiStatusTab.tsx`

- [ ] **Step 1: Update api_status_handler.go**

Remove `pricing.SourceCardHedger` from `providerDailyLimits` map and `knownProviders` slice. Remove any CardHedger-specific stats fields (like `MinuteCallsUsed`, `Last429Time` if they're CH-specific).

- [ ] **Step 2: Update price_hints.go**

Update the provider validation (line ~84) to accept `"doubleholo"` instead of `"pricecharting"` and `"cardhedger"`. Or if hints are no longer needed for any remaining provider, consider simplifying.

- [ ] **Step 3: Update frontend components**

In `PriceHintDialog.tsx`: Remove CardHedger from provider dropdown. Update to use DH if needed, or remove if hints aren't used.

In `MissingCardsTab.tsx`: This tab is CardHedger-dependent (shows cards that couldn't be matched in CardHedger). Either remove the tab entirely or repurpose for DH matching failures.

In `ApiStatusTab.tsx`: Remove CardHedger from provider display mapping.

- [ ] **Step 4: Verify**

```bash
go test ./internal/adapters/httpserver/...
cd web && npm run typecheck && cd ..
```

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/httpserver/ web/src/
git commit -m "refactor: remove CardHedger from API handlers and frontend"
```

---

### Task 8: Update tests and mocks for CardHedger removal

**Files:**
- Modify: `internal/testutil/mocks/` (any CardHedger-specific mocks)
- Modify: `internal/adapters/storage/sqlite/prices_test.go` (pokemonprice test fixtures)
- Modify: Any other test files that reference `SourceCardHedger`

- [ ] **Step 1: Find and fix all remaining CardHedger test references**

```bash
grep -r 'CardHedger\|cardhedger\|SourceCardHedger' --include='*.go' . | grep -v '_test.go' | grep -v vendor
grep -r 'CardHedger\|cardhedger\|SourceCardHedger' --include='*.go' . | grep '_test.go'
```

Fix each reference: remove CardHedger-specific mocks, update test data that uses `source = "cardhedger"`.

- [ ] **Step 2: Run full test suite**

```bash
go test ./...
```

Fix any remaining failures.

- [ ] **Step 3: Run quality checks**

```bash
make check
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: clean up CardHedger references from tests and mocks"
```

---

## Phase 2: Replace Fusion with DH Provider

### Task 9: Create DHPriceProvider

**Files:**
- Create: `internal/adapters/clients/dhprice/provider.go`
- Create: `internal/adapters/clients/dhprice/provider_test.go`

- [ ] **Step 1: Write the test file**

```go
package dhprice

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// mockMarketDataClient implements the MarketDataClient interface for testing.
type mockMarketDataClient struct {
	recentSales []dh.RecentSale
	err         error
}

func (m *mockMarketDataClient) RecentSales(_ context.Context, _ int) ([]dh.RecentSale, error) {
	return m.recentSales, m.err
}

// mockCardIDLookup implements the CardIDLookup interface for testing.
type mockCardIDLookup struct {
	id  string
	err error
}

func (m *mockCardIDLookup) GetExternalID(_ context.Context, _, _, _, _ string) (string, error) {
	return m.id, m.err
}

func TestGetPrice_NoCardID(t *testing.T) {
	p := NewProvider(&mockMarketDataClient{}, &mockCardIDLookup{id: ""}, nil)
	price, err := p.GetPrice(context.Background(), pricing.Card{Name: "test", Set: "set"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if price != nil {
		t.Fatal("expected nil price for unmapped card")
	}
}

func TestGetPrice_WithSales(t *testing.T) {
	sales := []dh.RecentSale{
		{GradingCompany: "PSA", Grade: "10", Price: 150.00},
		{GradingCompany: "PSA", Grade: "10", Price: 170.00},
		{GradingCompany: "PSA", Grade: "9", Price: 80.00},
	}
	p := NewProvider(
		&mockMarketDataClient{recentSales: sales},
		&mockCardIDLookup{id: "123"},
		nil,
	)
	price, err := p.GetPrice(context.Background(), pricing.Card{Name: "Charizard", Set: "Base Set"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if price == nil {
		t.Fatal("expected non-nil price")
	}
	if price.Source != pricing.Source(pricing.SourceDH) {
		t.Errorf("expected source %q, got %q", pricing.SourceDH, price.Source)
	}
	// PSA 10 should have a price derived from sales
	if price.Grades.PSA10Cents <= 0 {
		t.Errorf("expected PSA10 price > 0, got %d", price.Grades.PSA10Cents)
	}
	if price.Grades.PSA9Cents <= 0 {
		t.Errorf("expected PSA9 price > 0, got %d", price.Grades.PSA9Cents)
	}
}

func TestAvailable(t *testing.T) {
	p := NewProvider(&mockMarketDataClient{}, &mockCardIDLookup{}, nil)
	if !p.Available() {
		t.Error("expected Available() = true")
	}
	p2 := NewProvider(nil, nil, nil)
	if p2.Available() {
		t.Error("expected Available() = false with nil clients")
	}
}

func TestName(t *testing.T) {
	p := NewProvider(nil, nil, nil)
	if p.Name() != pricing.SourceDH {
		t.Errorf("expected %q, got %q", pricing.SourceDH, p.Name())
	}
}

func TestLookupCard(t *testing.T) {
	sales := []dh.RecentSale{
		{GradingCompany: "PSA", Grade: "10", Price: 100.00},
	}
	p := NewProvider(
		&mockMarketDataClient{recentSales: sales},
		&mockCardIDLookup{id: "456"},
		nil,
	)
	price, err := p.LookupCard(context.Background(), "Base Set", domainCards.Card{Name: "Pikachu"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if price == nil {
		t.Fatal("expected non-nil price from LookupCard")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/adapters/clients/dhprice/...
```

Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Write the provider implementation**

Create `internal/adapters/clients/dhprice/provider.go`. This adapts the existing logic from `fusionprice/dh_adapter.go` into a `pricing.PriceProvider` implementation:

```go
package dhprice

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

const dhConfidence = 0.90

// MarketDataClient is the subset of dh.Client used by the provider.
type MarketDataClient interface {
	RecentSales(ctx context.Context, cardID int) ([]dh.RecentSale, error)
}

// CardIDLookup resolves card names to DH card IDs.
type CardIDLookup interface {
	GetExternalID(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error)
}

// Provider implements pricing.PriceProvider using DH market data.
type Provider struct {
	client     MarketDataClient
	idResolver CardIDLookup
	logger     observability.Logger
}

// NewProvider creates a new DH-backed PriceProvider.
func NewProvider(client MarketDataClient, idResolver CardIDLookup, logger observability.Logger) *Provider {
	return &Provider{client: client, idResolver: idResolver, logger: logger}
}

func (p *Provider) GetPrice(ctx context.Context, card pricing.Card) (*pricing.Price, error) {
	if p.client == nil || p.idResolver == nil {
		return nil, nil
	}

	dhCardID, err := p.idResolver.GetExternalID(ctx, card.Name, card.Set, card.Number, pricing.SourceDH)
	if err != nil {
		return nil, fmt.Errorf("dh: card ID lookup: %w", err)
	}
	if dhCardID == "" {
		return nil, nil
	}

	cardIDInt, err := strconv.Atoi(dhCardID)
	if err != nil {
		return nil, fmt.Errorf("dh: invalid card ID %q: %w", dhCardID, err)
	}

	sales, err := p.client.RecentSales(ctx, cardIDInt)
	if err != nil {
		return nil, fmt.Errorf("dh: recent sales: %w", err)
	}
	if len(sales) == 0 {
		return nil, nil
	}

	return salesToPrice(sales, card), nil
}

func (p *Provider) LookupCard(ctx context.Context, setName string, card domainCards.Card) (*pricing.Price, error) {
	return p.GetPrice(ctx, pricing.Card{
		Name: card.Name,
		Set:  setName,
	})
}

func (p *Provider) Available() bool { return p.client != nil && p.idResolver != nil }
func (p *Provider) Name() string    { return pricing.SourceDH }
func (p *Provider) Close() error    { return nil }
func (p *Provider) GetStats(_ context.Context) *pricing.ProviderStats { return nil }

// salesToPrice converts DH recent sales into a pricing.Price.
func salesToPrice(sales []dh.RecentSale, card pricing.Card) *pricing.Price {
	byGrade := groupSalesByGrade(sales)

	var grades pricing.GradedPrices
	gradeDetails := make(map[string]*pricing.GradeDetail)

	for fusionKey, prices := range byGrade {
		median := medianPrice(prices)
		cents := int64(median * 100)
		setGradePrice(&grades, fusionKey, cents)

		gradeDetails[fusionKey] = &pricing.GradeDetail{
			Estimate: &pricing.EstimateGradeDetail{
				PriceCents: cents,
				Confidence: dhConfidence,
			},
		}
	}

	return &pricing.Price{
		ProductName: card.Name,
		Amount:      grades.PSA10Cents,
		Currency:    "USD",
		Source:      pricing.Source(pricing.SourceDH),
		Grades:      grades,
		Confidence:  dhConfidence,
		GradeDetails: gradeDetails,
		Sources:     []string{pricing.SourceDH},
	}
}

// groupSalesByGrade buckets sale prices by fusion grade key.
func groupSalesByGrade(sales []dh.RecentSale) map[string][]float64 {
	result := make(map[string][]float64)
	for _, sale := range sales {
		key := dhGradeToFusionKey(sale.GradingCompany, sale.Grade)
		if key == "" || sale.Price <= 0 {
			continue
		}
		result[key] = append(result[key], sale.Price)
	}
	return result
}

// medianPrice returns the median of a slice of prices.
func medianPrice(prices []float64) float64 {
	sort.Float64s(prices)
	n := len(prices)
	if n == 0 {
		return 0
	}
	if n%2 == 0 {
		return (prices[n/2-1] + prices[n/2]) / 2
	}
	return prices[n/2]
}

// setGradePrice sets the appropriate grade field on GradedPrices.
func setGradePrice(g *pricing.GradedPrices, key string, cents int64) {
	switch key {
	case pricing.GradePSA10.String():
		g.PSA10Cents = cents
	case pricing.GradePSA9.String():
		g.PSA9Cents = cents
	case pricing.GradePSA95.String():
		g.Grade95Cents = cents
	case pricing.GradePSA8.String():
		g.PSA8Cents = cents
	case pricing.GradePSA7.String():
		g.PSA7Cents = cents
	case pricing.GradePSA6.String():
		g.PSA6Cents = cents
	case pricing.GradeBGS10.String():
		g.BGS10Cents = cents
	case pricing.GradeRaw.String():
		g.RawCents = cents
	}
}

// dhGradeToFusionKey converts a DH grading company + grade to a fusion key.
func dhGradeToFusionKey(company, grade string) string {
	key := strings.ToUpper(strings.TrimSpace(company)) + " " + strings.TrimSpace(grade)
	switch key {
	case "PSA 10":
		return pricing.GradePSA10.String()
	case "PSA 9":
		return pricing.GradePSA9.String()
	case "PSA 9.5":
		return pricing.GradePSA95.String()
	case "PSA 8":
		return pricing.GradePSA8.String()
	case "PSA 7":
		return pricing.GradePSA7.String()
	case "PSA 6":
		return pricing.GradePSA6.String()
	case "BGS 10":
		return pricing.GradeBGS10.String()
	case "BGS 9.5", "CGC 9.5":
		return pricing.GradePSA95.String()
	default:
		return ""
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/adapters/clients/dhprice/...
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/clients/dhprice/
git commit -m "feat: add DHPriceProvider implementing pricing.PriceProvider"
```

---

### Task 10: Wire DHPriceProvider into init.go, replace FusionProvider

**Files:**
- Modify: `cmd/slabledger/init.go`

- [ ] **Step 1: Replace `initializePriceProviders()`**

Replace the entire function. The new version creates a `dhprice.Provider` instead of a `FusionPriceProvider`:

```go
// initializePriceProviders creates the DH-backed price provider.
func initializePriceProviders(
	ctx context.Context,
	cfg *config.Config,
	logger observability.Logger,
	cardIDMappingRepo *sqlite.CardIDMappingRepository,
	dhClient *dh.Client,
) (pricing.PriceProvider, error) {
	if dhClient == nil || !dhClient.EnterpriseAvailable() {
		logger.Warn(ctx, "DH client not available; price provider will be inactive")
		return dhprice.NewProvider(nil, nil, logger), nil
	}

	provider := dhprice.NewProvider(dhClient, cardIDMappingRepo, logger)
	logger.Info(ctx, "DH price provider initialized")
	return provider, nil
}
```

Remove imports for `fusionprice`, `pricecharting`, `cardhedger`, `fusion`. Add import for `dhprice`.

- [ ] **Step 2: Update callers in main.go**

The call to `initializePriceProviders()` in `main.go` now returns `(pricing.PriceProvider, error)` instead of the old 4-return signature. Update accordingly. Remove the `pcProvider.Close()` deferred call.

Update `schedulerDeps.PriceProvImpl` to use the new provider (change its type from `*fusionprice.FusionPriceProvider` to `pricing.PriceProvider`). Update the `schedulerDeps` struct definition in `init.go` to match.

- [ ] **Step 3: Update `initializeCampaignsService()`**

Change the `priceProvImpl` parameter type from `*fusionprice.FusionPriceProvider` to `pricing.PriceProvider`. The `pricelookup.NewAdapter()` call should still work since it takes the `PriceProvider` interface.

- [ ] **Step 4: Verify compilation**

```bash
go build ./cmd/slabledger/...
```

- [ ] **Step 5: Commit**

```bash
git add cmd/slabledger/
git commit -m "refactor: wire DHPriceProvider replacing FusionProvider"
```

---

### Task 11: Delete fusion engine and fusionprice package

**Files:**
- Delete: `internal/domain/fusion/` (5 files, ~1,013 lines)
- Delete: `internal/adapters/clients/fusionprice/` (15 files, ~4,483 lines)

- [ ] **Step 1: Delete fusion domain**

```bash
rm -rf internal/domain/fusion/
```

- [ ] **Step 2: Delete fusionprice adapters**

```bash
rm -rf internal/adapters/clients/fusionprice/
```

- [ ] **Step 3: Fix any remaining compile errors**

```bash
go build ./...
```

The scheduler `builder.go` may still reference fusion types in `BuildDeps.PriceProvider`. Update the type from `*fusionprice.FusionPriceProvider` to `pricing.PriceProvider` if not done in Task 10.

- [ ] **Step 4: Commit**

```bash
git add -A internal/domain/fusion/ internal/adapters/clients/fusionprice/
git commit -m "refactor: delete fusion engine and fusionprice package"
```

---

### Task 12: Remove fusion config

**Files:**
- Modify: `internal/platform/config/types.go`
- Modify: `internal/platform/config/loader.go`
- Modify: `internal/platform/config/defaults.go`
- Modify: `.env.example`

- [ ] **Step 1: Remove FusionConfig**

In `types.go`: Remove `FusionConfig` struct and the `Fusion` field from the main `Config` struct.

In `defaults.go`: Remove FusionConfig defaults.

In `loader.go`: Remove `FUSION_*` env var loading.

In `.env.example`: Remove `FUSION_CACHE_TTL`, `FUSION_PRICECHARTING_TIMEOUT`, `FUSION_SECONDARY_TIMEOUT`.

- [ ] **Step 2: Verify**

```bash
go test ./internal/platform/config/...
go build ./cmd/slabledger/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/platform/config/ .env.example
git commit -m "refactor: remove fusion config structs and env vars"
```

---

### Task 13: Clean up pricing domain — remove dead fields and constants

**Files:**
- Modify: `internal/domain/pricing/provider.go`
- Modify: `internal/domain/pricing/repository.go`

- [ ] **Step 1: Remove dead source constants**

In `provider.go`, remove `SourcePriceCharting`, `SourceCardHedger`, `SourceJustTCG`. Keep `SourceDH`.

- [ ] **Step 2: Remove dead struct fields**

In `provider.go`:
- Remove `RawNMCents` from `GradedPrices` (line 50)
- Remove `PCGrades *GradedPrices` from `Price` (line 120)
- Remove `FusionMetadata *FusionMetadata` from `Price` (line 113) and the entire `FusionMetadata` struct (lines 132-148) and `SourceResult` struct (lines 144-148)
- Update the comment on `EstimateGradeDetail` (line 180) — it's now DH-sourced, not CardHedger
- Update the comment on `GradeDetail.Estimate` (line 192) — now DH data, not CardHedger
- Update the `PSAListingTitle` comment on `Card` (lines 42-44) — remove CardHedger reference

- [ ] **Step 3: Remove DiscoveryFailureTracker interface**

In `repository.go`, remove the `DiscoveryFailureTracker` interface and `DiscoveryFailure` type if they exist.

- [ ] **Step 4: Fix all compile errors from removed types**

```bash
go build ./...
```

This will likely surface references in:
- `internal/adapters/storage/sqlite/prices.go` (FusionMetadata fields on PriceEntry)
- `internal/adapters/storage/sqlite/pricing_diagnostics.go` (fusion classification)
- `internal/adapters/clients/pricelookup/adapter.go` (PCGrades, RawNMCents)
- Various test files

Fix each one by removing the dead field references.

- [ ] **Step 5: Run tests**

```bash
go test ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/domain/pricing/ internal/adapters/
git commit -m "refactor: remove dead pricing constants, fields, and interfaces"
```

---

## Phase 3: Remove PriceCharting + JustTCG

### Task 14: Delete PriceCharting package

**Files:**
- Delete: `internal/adapters/clients/pricecharting/` (30 files, ~5,532 lines)

- [ ] **Step 1: Delete the package**

```bash
rm -rf internal/adapters/clients/pricecharting/
```

- [ ] **Step 2: Commit**

```bash
git add -A internal/adapters/clients/pricecharting/
git commit -m "refactor: delete PriceCharting client package"
```

---

### Task 15: Delete JustTCG package and scheduler

**Files:**
- Delete: `internal/adapters/clients/justtcg/` (3 files, ~649 lines)
- Delete: `internal/adapters/scheduler/justtcg_refresh.go` (434 lines)

- [ ] **Step 1: Delete JustTCG client**

```bash
rm -rf internal/adapters/clients/justtcg/
rm internal/adapters/scheduler/justtcg_refresh.go
```

- [ ] **Step 2: Remove JustTCG from scheduler builder**

In `builder.go`, remove the `JustTCGClient` field from `BuildDeps` and the JustTCG scheduler construction code in `BuildGroup()`.

- [ ] **Step 3: Commit**

```bash
git add -A internal/adapters/clients/justtcg/ internal/adapters/scheduler/
git commit -m "refactor: delete JustTCG client and scheduler"
```

---

### Task 16: Remove PriceCharting + JustTCG from wiring and config

**Files:**
- Modify: `cmd/slabledger/init.go`
- Modify: `cmd/slabledger/main.go`
- Modify: `cmd/slabledger/server.go`
- Modify: `internal/platform/config/types.go`
- Modify: `internal/platform/config/loader.go`
- Modify: `internal/platform/config/defaults.go`
- Modify: `.env.example`

- [ ] **Step 1: Remove from init.go and main.go**

Remove `pricecharting` and `justtcg` imports. Remove any remaining references to PriceCharting or JustTCG client initialization.

In `schedulerDeps` struct, remove `JustTCGClient` field. Remove the nil-safe JustTCG wiring block in `initializeSchedulers()`.

- [ ] **Step 2: Remove from server.go**

Remove the `PRICECHARTING_TOKEN` required env check.

- [ ] **Step 3: Remove config**

In `types.go`: Remove `JustTCGConfig` struct, `PriceChartingToken` and `JustTCGKey` from `AdapterConfig`, `JustTCG` field from `Config`.

In `loader.go`: Remove `PRICECHARTING_TOKEN` and `JUSTTCG_*` loading.

In `defaults.go`: Remove JustTCG defaults.

In `.env.example`: Remove `PRICECHARTING_TOKEN` and all `JUSTTCG_*` vars.

- [ ] **Step 4: Verify**

```bash
go build ./cmd/slabledger/...
go test ./internal/platform/config/...
```

- [ ] **Step 5: Commit**

```bash
git add cmd/slabledger/ internal/platform/config/ .env.example
git commit -m "refactor: remove PriceCharting and JustTCG from wiring and config"
```

---

### Task 17: Clean up pricelookup adapter

**Files:**
- Modify: `internal/adapters/clients/pricelookup/adapter.go`

- [ ] **Step 1: Remove dead source logic**

Remove the JustTCG NM preference (lines 64-67):
```go
// DELETE these lines:
if grade == 0 && price.Grades.RawNMCents > 0 {
    return int(price.Grades.RawNMCents), nil
}
```

Remove PriceCharting source price block in `buildSourcePrices()` (lines 305-315):
```go
// DELETE this block:
if price.PCGrades != nil { ... }
```

Update the CardHedger estimate block in `buildSourcePrices()` (lines 345-367) — change the source name from `"CardHedger"` to `"DH"` since estimates now come from DH:
```go
Source: "DH",
```

- [ ] **Step 2: Verify**

```bash
go test ./internal/adapters/clients/pricelookup/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/adapters/clients/pricelookup/
git commit -m "refactor: remove PriceCharting and JustTCG logic from pricelookup adapter"
```

---

### Task 18: Clean up pricing diagnostics

**Files:**
- Modify: `internal/adapters/storage/sqlite/pricing_diagnostics.go`

- [ ] **Step 1: Simplify queryCardQuality()**

The function currently classifies cards as "full_fusion", "partial", "pc_only". Replace with a simpler classification based on whether DH data is available.

- [ ] **Step 2: Remove discovery failure queries**

If `queryDiscoveryFailureCount()` or similar functions exist, remove them (discovery failures were CardHedger-only).

- [ ] **Step 3: Verify**

```bash
go test ./internal/adapters/storage/sqlite/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/storage/sqlite/pricing_diagnostics.go
git commit -m "refactor: simplify pricing diagnostics — remove fusion classification"
```

---

### Task 19: Database migration — update CHECK constraints

**Files:**
- Create: `internal/adapters/storage/sqlite/migrations/000032_remove_legacy_sources.up.sql`
- Create: `internal/adapters/storage/sqlite/migrations/000032_remove_legacy_sources.down.sql`

- [ ] **Step 1: Write the up migration**

The migration needs to recreate tables with updated CHECK constraints (SQLite doesn't support `ALTER TABLE ... DROP CONSTRAINT`). For each affected table, the pattern is: create new table → copy data → drop old → rename.

However, since historical data with old source values still exists, a simpler approach is to REMOVE the CHECK constraint entirely (the application layer enforces valid values):

```sql
-- price_history: remove source CHECK constraint
-- (SQLite requires table recreation to modify constraints)
-- Since we keep historical data with old source values, removing the
-- constraint is safer than trying to enumerate all valid sources.
CREATE TABLE price_history_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    card_name TEXT NOT NULL,
    set_name TEXT NOT NULL,
    card_number TEXT NOT NULL DEFAULT '',
    grade TEXT NOT NULL DEFAULT '',
    price_cents INTEGER NOT NULL,
    source TEXT NOT NULL DEFAULT '',
    price_date TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    confidence REAL NOT NULL DEFAULT 0,
    fusion_source_count INTEGER NOT NULL DEFAULT 0,
    fusion_outliers_removed INTEGER NOT NULL DEFAULT 0,
    fusion_method TEXT NOT NULL DEFAULT ''
);

INSERT INTO price_history_new SELECT * FROM price_history;
DROP TABLE price_history;
ALTER TABLE price_history_new RENAME TO price_history;

-- Recreate indexes
CREATE INDEX idx_price_history_card ON price_history(card_name, set_name, grade);
CREATE INDEX idx_price_history_staleness ON price_history(source, updated_at DESC);
CREATE INDEX idx_price_history_date ON price_history(price_date DESC);
CREATE INDEX idx_price_history_lookup ON price_history(card_name, set_name, card_number, grade, source, price_date DESC);
CREATE UNIQUE INDEX idx_price_history_unique ON price_history(card_name, set_name, card_number, grade, source, price_date);
```

Apply the same pattern for `api_calls`, `api_rate_limits`, and `price_refresh_queue` tables.

Note: Check the exact current schema of each table by reading the latest migration that touched it (migrations 000026 and 000028 recreated several of these tables). Reproduce the EXACT current schema minus the CHECK constraint.

- [ ] **Step 2: Write the down migration**

The down migration should add the CHECK constraints back. Use the same table-recreation pattern.

- [ ] **Step 3: Test the migration**

```bash
go test ./internal/adapters/storage/sqlite/... -run TestMigration
```

Or manually: back up your DB, run the app, verify it starts.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/storage/sqlite/migrations/
git commit -m "migration: remove legacy source CHECK constraints from price tables"
```

---

### Task 20: Clean up remaining test references and PokemonPrice remnants

**Files:**
- Modify: Various test files
- Modify: `internal/adapters/storage/sqlite/prices_test.go`

- [ ] **Step 1: Find all remaining references**

```bash
grep -rn 'pricecharting\|cardhedger\|justtcg\|pokemonprice\|SourcePriceCharting\|SourceCardHedger\|SourceJustTCG\|FusionMetadata\|PCGrades\|RawNMCents' --include='*.go' .
```

Fix every hit: update test data sources to `"doubleholo"`, remove references to deleted types.

- [ ] **Step 2: Find frontend references**

```bash
grep -rn 'cardhedger\|pricecharting\|justtcg\|pokemonprice\|fusionConfidence\|fusion_confidence' web/src/
```

Fix every hit: remove dead provider references, update type definitions.

- [ ] **Step 3: Run full verification**

```bash
go test -race -timeout 10m ./...
make check
cd web && npm run typecheck && npm test && cd ..
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore: clean up all remaining references to removed pricing sources"
```

---

### Task 21: Update documentation

**Files:**
- Modify: `CLAUDE.md`
- Modify: `.env.example` (final check)
- Modify: `docs/PRICING_DATA.md`

- [ ] **Step 1: Update CLAUDE.md**

Remove references to PriceCharting, CardHedger, JustTCG, and fusion engine. Update the "Pricing Pipeline" and "Environment Variables" sections. Update the architecture tree to remove deleted packages. Remove `PRICECHARTING_TOKEN` from the Required env vars section.

- [ ] **Step 2: Update docs/PRICING_DATA.md**

This document describes the full pricing pipeline, normalization, fusion engine, and caching. It needs a major rewrite to reflect the simplified DH-only architecture. Or mark it as outdated with a note pointing to the design spec.

- [ ] **Step 3: Final .env.example check**

Verify no dead env vars remain.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md .env.example docs/
git commit -m "docs: update for pricing simplification — DH-only architecture"
```

---

### Task 22: Update memory files

**Files:**
- Remove or update: `.claude/projects/-workspace/memory/feedback_cardhedger_freely.md`
- Update: `.claude/projects/-workspace/memory/MEMORY.md`

- [ ] **Step 1: Remove obsolete memory**

Delete `feedback_cardhedger_freely.md` — the guidance "use CardHedger freely" is now irrelevant.

- [ ] **Step 2: Update MEMORY.md**

Remove the entry for `feedback_cardhedger_freely.md`. Update the "Key Architecture Notes" section to reflect the new DH-only pricing architecture.

- [ ] **Step 3: Commit**

```bash
git add .claude/
git commit -m "chore: update memory files for pricing simplification"
```
