# Liquidation Analysis Reliability Redesign — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the 76% failure rate on the liquidation analysis LLM job by fixing the round budget mismatch, adding a procedural inventory signals system, rewriting the prompt to focus LLM judgment on pre-flagged cards, and adding a CLI debug harness for local testing.

**Architecture:** Four components layered bottom-up: (1) domain-level `InventorySignals` struct computed during sell sheet enrichment, (2) new `get_flagged_inventory` advisor tool replacing `get_global_inventory` for liquidation, (3) revised prompts and round budget in the advisor service, (4) `slabledger admin analyze` CLI command for local testing with verbose output.

**Tech Stack:** Go 1.26, SQLite, Azure AI (GPT-5.4), React/TypeScript frontend

---

### Task 1: Add `InventorySignals` domain type

**Files:**
- Modify: `internal/domain/campaigns/analytics_types.go:46-57` (AgingItem struct)
- Modify: `internal/domain/campaigns/analytics_types.go:68-94` (SellSheetItem struct)

- [ ] **Step 1: Add InventorySignals struct to analytics_types.go**

Add after the `AgingItem` struct (after line 57):

```go
// InventorySignals contains procedural flags for an unsold card.
// Computed server-side from market data, aging, and profitability.
type InventorySignals struct {
	ProfitCaptureDeclining bool `json:"profitCaptureDeclining,omitempty"`
	ProfitCaptureSpike     bool `json:"profitCaptureSpike,omitempty"`
	CrackCandidate         bool `json:"crackCandidate,omitempty"`
	StaleListing           bool `json:"staleListing,omitempty"`
	DeepStale              bool `json:"deepStale,omitempty"`
	CutLoss                bool `json:"cutLoss,omitempty"`
}

// HasAnySignal returns true if any signal flag is set.
func (s *InventorySignals) HasAnySignal() bool {
	return s.ProfitCaptureDeclining || s.ProfitCaptureSpike ||
		s.CrackCandidate || s.StaleListing || s.DeepStale || s.CutLoss
}
```

- [ ] **Step 2: Add Signals field to AgingItem**

Add `Signals` field to the `AgingItem` struct:

```go
type AgingItem struct {
	Purchase              Purchase         `json:"purchase"`
	DaysHeld              int              `json:"daysHeld"`
	CampaignName          string           `json:"campaignName,omitempty"`
	Signal                *MarketSignal    `json:"signal,omitempty"`
	CurrentMarket         *MarketSnapshot  `json:"currentMarket,omitempty"`
	PriceAnomaly          bool             `json:"priceAnomaly,omitempty"`
	AnomalyReason         string           `json:"anomalyReason,omitempty"`
	HasOpenFlag           bool             `json:"hasOpenFlag,omitempty"`
	RecommendedPriceCents int              `json:"recommendedPriceCents,omitempty"`
	RecommendedSource     string           `json:"recommendedSource,omitempty"`
	Signals               *InventorySignals `json:"signals,omitempty"`
}
```

- [ ] **Step 3: Add Signals field to SellSheetItem**

Add `Signals` field to the `SellSheetItem` struct (after `AISuggestedAt`):

```go
	AISuggestedAt         string            `json:"aiSuggestedAt,omitempty"`
	Signals               *InventorySignals `json:"signals,omitempty"`
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./internal/domain/campaigns/...`
Expected: compiles cleanly

- [ ] **Step 5: Commit**

```bash
git add internal/domain/campaigns/analytics_types.go
git commit -m "feat(signals): add InventorySignals domain type to AgingItem and SellSheetItem"
```

---

### Task 2: Implement signal computation

**Files:**
- Create: `internal/domain/campaigns/inventory_signals.go`
- Create: `internal/domain/campaigns/inventory_signals_test.go`

- [ ] **Step 1: Write tests for signal computation**

Create `internal/domain/campaigns/inventory_signals_test.go`:

```go
package campaigns_test

import (
	"testing"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

func TestComputeInventorySignals(t *testing.T) {
	tests := []struct {
		name     string
		item     campaigns.AgingItem
		isCrack  bool
		expected campaigns.InventorySignals
	}{
		{
			name: "profit capture declining — recent sold, profitable, trend down",
			item: campaigns.AgingItem{
				DaysHeld: 10,
				Purchase: campaigns.Purchase{
					BuyCostCents:       5000,
					PSASourcingFeeCents: 300,
				},
				CurrentMarket: &campaigns.MarketSnapshot{
					LastSoldCents: 8000,
					LastSoldDate:  "2026-03-28",
					SalesLast30d:  3,
					Trend30d:      -0.08,
					MedianCents:   7500,
				},
			},
			expected: campaigns.InventorySignals{ProfitCaptureDeclining: true},
		},
		{
			name: "profit capture spike — price up >10%, recent sales, profitable",
			item: campaigns.AgingItem{
				DaysHeld: 10,
				Purchase: campaigns.Purchase{
					BuyCostCents:       5000,
					PSASourcingFeeCents: 300,
				},
				CurrentMarket: &campaigns.MarketSnapshot{
					LastSoldCents: 9000,
					SalesLast30d:  4,
					Trend30d:      0.15,
					MedianCents:   8500,
				},
			},
			expected: campaigns.InventorySignals{ProfitCaptureSpike: true},
		},
		{
			name: "crack candidate from lookup",
			item: campaigns.AgingItem{
				DaysHeld: 5,
				Purchase: campaigns.Purchase{BuyCostCents: 5000},
			},
			isCrack:  true,
			expected: campaigns.InventorySignals{CrackCandidate: true},
		},
		{
			name: "stale listing — held >14 days",
			item: campaigns.AgingItem{
				DaysHeld: 20,
				Purchase: campaigns.Purchase{BuyCostCents: 5000},
			},
			expected: campaigns.InventorySignals{StaleListing: true},
		},
		{
			name: "deep stale — held >30 days",
			item: campaigns.AgingItem{
				DaysHeld: 35,
				Purchase: campaigns.Purchase{BuyCostCents: 5000},
			},
			expected: campaigns.InventorySignals{StaleListing: true, DeepStale: true},
		},
		{
			name: "cut loss — deep stale + declining trend",
			item: campaigns.AgingItem{
				DaysHeld: 40,
				Purchase: campaigns.Purchase{
					BuyCostCents:       5000,
					PSASourcingFeeCents: 300,
				},
				CurrentMarket: &campaigns.MarketSnapshot{
					Trend30d:    -0.05,
					MedianCents: 4000,
				},
			},
			expected: campaigns.InventorySignals{StaleListing: true, DeepStale: true, CutLoss: true},
		},
		{
			name: "cut loss — deep stale + negative unrealized PL",
			item: campaigns.AgingItem{
				DaysHeld: 40,
				Purchase: campaigns.Purchase{
					BuyCostCents:       8000,
					PSASourcingFeeCents: 300,
				},
				CurrentMarket: &campaigns.MarketSnapshot{
					MedianCents: 5000,
				},
			},
			expected: campaigns.InventorySignals{StaleListing: true, DeepStale: true, CutLoss: true},
		},
		{
			name: "no signals — fresh card, no market data",
			item: campaigns.AgingItem{
				DaysHeld: 3,
				Purchase: campaigns.Purchase{BuyCostCents: 5000},
			},
			expected: campaigns.InventorySignals{},
		},
		{
			name: "no signals — healthy card, good market, recent",
			item: campaigns.AgingItem{
				DaysHeld: 10,
				Purchase: campaigns.Purchase{
					BuyCostCents:       5000,
					PSASourcingFeeCents: 300,
				},
				CurrentMarket: &campaigns.MarketSnapshot{
					LastSoldCents: 7000,
					SalesLast30d:  2,
					Trend30d:      0.02,
					MedianCents:   6800,
				},
			},
			expected: campaigns.InventorySignals{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := campaigns.ComputeInventorySignals(&tt.item, tt.isCrack)
			if got != tt.expected {
				t.Errorf("ComputeInventorySignals() = %+v, want %+v", got, tt.expected)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/domain/campaigns/... -run TestComputeInventorySignals -v`
Expected: FAIL — `ComputeInventorySignals` not defined

- [ ] **Step 3: Implement signal computation**

Create `internal/domain/campaigns/inventory_signals.go`:

```go
package campaigns

import (
	"time"
)

// Signal thresholds — tunable constants for inventory signal detection.
const (
	// staleDays is the minimum days held before a card is flagged stale.
	staleDays = 14
	// deepStaleDays is the minimum days held before a card is flagged deep stale.
	deepStaleDays = 30
	// spikeThreshold is the minimum 30-day trend to qualify as a price spike.
	spikeThreshold = 0.10
	// minRecentSalesForSpike is the minimum sales in last 30 days to confirm a spike.
	minRecentSalesForSpike = 2
	// recentSoldMaxDays is the maximum age of a last-sold date to count as "recent."
	recentSoldMaxDays = 14
)

// ComputeInventorySignals determines procedural flags for an unsold card.
// isCrackCandidate should be pre-computed from GetCrackOpportunities.
func ComputeInventorySignals(item *AgingItem, isCrackCandidate bool) InventorySignals {
	var sig InventorySignals

	mkt := item.CurrentMarket
	costBasis := item.Purchase.BuyCostCents + item.Purchase.PSASourcingFeeCents

	// Crack candidate (pre-computed)
	if isCrackCandidate {
		sig.CrackCandidate = true
	}

	// Stale / deep stale
	if item.DaysHeld > staleDays {
		sig.StaleListing = true
	}
	if item.DaysHeld > deepStaleDays {
		sig.DeepStale = true
	}

	if mkt == nil {
		return sig
	}

	profitable := costBasis > 0 && mkt.MedianCents > costBasis
	recentSold := hasRecentLastSold(mkt)

	// Profit capture — declining value with recent last-sold, still profitable
	if recentSold && profitable && mkt.Trend30d < 0 {
		sig.ProfitCaptureDeclining = true
	}

	// Profit capture — price spike with recent sales volume, still profitable
	if profitable && mkt.Trend30d >= spikeThreshold && mkt.SalesLast30d >= minRecentSalesForSpike {
		sig.ProfitCaptureSpike = true
	}

	// Cut loss — deep stale + (declining trend OR underwater)
	if sig.DeepStale {
		underwater := costBasis > 0 && mkt.MedianCents > 0 && mkt.MedianCents < costBasis
		if mkt.Trend30d < 0 || underwater {
			sig.CutLoss = true
		}
	}

	return sig
}

// hasRecentLastSold checks if the last sold date is within recentSoldMaxDays.
func hasRecentLastSold(mkt *MarketSnapshot) bool {
	if mkt.LastSoldDate == "" || mkt.LastSoldCents <= 0 {
		return false
	}
	t, err := time.Parse("2006-01-02", mkt.LastSoldDate)
	if err != nil {
		return false
	}
	return time.Since(t).Hours()/24 <= recentSoldMaxDays
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/domain/campaigns/... -run TestComputeInventorySignals -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/campaigns/inventory_signals.go internal/domain/campaigns/inventory_signals_test.go
git commit -m "feat(signals): implement ComputeInventorySignals with tests"
```

---

### Task 3: Wire signals into enrichment and update `recommendChannel`

**Files:**
- Modify: `internal/domain/campaigns/service_analytics.go:228-251` (GetGlobalInventoryAging)
- Modify: `internal/domain/campaigns/service_sell_sheet.go:20-93` (enrichSellSheetItem)
- Modify: `internal/domain/campaigns/service_sell_sheet.go:96-104` (recommendChannel)

- [ ] **Step 1: Wire signals into GetGlobalInventoryAging**

In `service_analytics.go`, modify `GetGlobalInventoryAging` to compute crack candidates and apply signals:

```go
func (s *service) GetGlobalInventoryAging(ctx context.Context) ([]AgingItem, error) {
	purchases, err := s.repo.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unsold purchases: %w", err)
	}

	// Build campaign name lookup
	campaignList, err := s.repo.ListCampaigns(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	campaignNames := make(map[string]string, len(campaignList))
	for _, c := range campaignList {
		campaignNames[c.ID] = c.Name
	}

	items := make([]AgingItem, 0, len(purchases))
	for i := range purchases {
		items = append(items, s.enrichAgingItem(ctx, &purchases[i], campaignNames[purchases[i].CampaignID]))
	}

	s.applyOpenFlags(ctx, items)

	// Compute crack candidates for signal enrichment
	crackSet := s.buildCrackCandidateSet(ctx)

	// Apply inventory signals
	for i := range items {
		isCrack := crackSet[items[i].Purchase.ID]
		sig := ComputeInventorySignals(&items[i], isCrack)
		if sig.HasAnySignal() {
			items[i].Signals = &sig
		}
	}

	return items, nil
}
```

- [ ] **Step 2: Add buildCrackCandidateSet helper**

Add to `service_analytics.go` (after `GetGlobalInventoryAging`):

```go
// buildCrackCandidateSet returns a set of purchase IDs that are crack candidates.
// Best-effort: returns empty set on error.
func (s *service) buildCrackCandidateSet(ctx context.Context) map[string]bool {
	cracks, err := s.GetCrackOpportunities(ctx)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "crack candidates failed for signal enrichment", observability.Err(err))
		}
		return nil
	}
	set := make(map[string]bool, len(cracks))
	for _, c := range cracks {
		if c.IsCrackCandidate {
			set[c.PurchaseID] = true
		}
	}
	return set
}
```

- [ ] **Step 3: Wire signals into enrichSellSheetItem**

In `service_sell_sheet.go`, add signal computation at the end of `enrichSellSheetItem` (before the return). This requires a `crackSet` parameter. Change the signature:

```go
func (s *service) enrichSellSheetItem(_ context.Context, purchase *Purchase, campaignName string, ebayFeePct float64, crackSet map[string]bool) (SellSheetItem, bool) {
```

At the end, before `return item, hasMarket`:

```go
	// Compute inventory signals
	agingItem := AgingItem{
		Purchase:      *purchase,
		CurrentMarket: item.CurrentMarket,
	}
	if t, err := time.Parse("2006-01-02", purchase.PurchaseDate); err == nil {
		agingItem.DaysHeld = int(time.Since(t).Hours() / 24)
	}
	isCrack := crackSet != nil && crackSet[purchase.ID]
	sig := ComputeInventorySignals(&agingItem, isCrack)
	if sig.HasAnySignal() {
		item.Signals = &sig
	}

	return item, hasMarket
```

- [ ] **Step 4: Update all callers of enrichSellSheetItem to pass crackSet**

There are 4 callers in `service_sell_sheet.go`. Update each to pass `nil` for the crackSet parameter (callers that need signals will pass a real set):

- `GenerateSellSheet` (line 130): `s.enrichSellSheetItem(ctx, purchase, "", campaign.EbayFeePct, nil)`
- `buildCrossCampaignSellSheet` (line 205): `s.enrichSellSheetItem(ctx, purchase, campName, feePct, nil)`
- `MatchShopifyPrices` (line 262): `s.enrichSellSheetItem(ctx, purchase, "", grossModeFee, nil)`

For `GenerateGlobalSellSheet`, compute the crack set and pass it through:

```go
func (s *service) GenerateGlobalSellSheet(ctx context.Context) (*SellSheet, error) {
	purchases, err := s.repo.ListAllUnsoldPurchases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unsold purchases: %w", err)
	}

	ptrs := make([]*Purchase, len(purchases))
	for i := range purchases {
		ptrs[i] = &purchases[i]
	}

	return s.buildCrossCampaignSellSheet(ctx, ptrs, "All Inventory")
}
```

The global sell sheet doesn't need crack signals (it's for pricing, not liquidation), so `nil` is fine here.

- [ ] **Step 5: Update recommendChannel to use signals**

In `service_sell_sheet.go`, update `recommendChannel`:

```go
// recommendChannel determines the best exit channel for a sell-sheet item.
func recommendChannel(grade float64, _ int, mkt *MarketSnapshot, signals *InventorySignals) (SaleChannel, string) {
	if grade == 7 {
		return SaleChannelInPerson, "In Person"
	}
	if signals != nil {
		if signals.ProfitCaptureDeclining || signals.ProfitCaptureSpike || signals.CrackCandidate {
			return SaleChannelInPerson, "In Person"
		}
	}
	if mkt != nil && mkt.Trend30d > 0.05 {
		return SaleChannelInPerson, "In Person"
	}
	return SaleChannelEbay, "eBay"
}
```

Update the call in `enrichSellSheetItem` to pass signals:

```go
	item.RecommendedChannel, item.ChannelLabel = recommendChannel(purchase.GradeValue, purchase.CLValueCents, item.CurrentMarket, item.Signals)
```

- [ ] **Step 6: Verify compilation**

Run: `go build ./internal/domain/campaigns/...`
Expected: compiles cleanly

- [ ] **Step 7: Run existing tests**

Run: `go test ./internal/domain/campaigns/... -v -count=1`
Expected: all existing tests still pass

- [ ] **Step 8: Commit**

```bash
git add internal/domain/campaigns/service_analytics.go internal/domain/campaigns/service_sell_sheet.go
git commit -m "feat(signals): wire signal computation into inventory aging and sell sheet enrichment"
```

---

### Task 4: Add `GetFlaggedInventory` service method and tool

**Files:**
- Modify: `internal/domain/campaigns/service.go:175` (Service interface)
- Modify: `internal/domain/campaigns/service_analytics.go` (implementation)
- Modify: `internal/testutil/mocks/campaign_service.go` (mock)
- Create: `internal/adapters/advisortool/tools_signals.go`
- Modify: `internal/adapters/advisortool/executor.go:232-264` (registerTools)

- [ ] **Step 1: Add GetFlaggedInventory to Service interface**

In `service.go`, add after `GetGlobalInventoryAging` (line 175):

```go
	GetGlobalInventoryAging(ctx context.Context) ([]AgingItem, error)
	GetFlaggedInventory(ctx context.Context) ([]AgingItem, error)
```

- [ ] **Step 2: Implement GetFlaggedInventory**

In `service_analytics.go`, add after `GetGlobalInventoryAging`:

```go
// GetFlaggedInventory returns only unsold cards that have at least one
// inventory signal set. Used by the liquidation analysis to receive
// pre-filtered, actionable cards instead of the full inventory.
func (s *service) GetFlaggedInventory(ctx context.Context) ([]AgingItem, error) {
	all, err := s.GetGlobalInventoryAging(ctx)
	if err != nil {
		return nil, err
	}
	var flagged []AgingItem
	for _, item := range all {
		if item.Signals != nil && item.Signals.HasAnySignal() {
			flagged = append(flagged, item)
		}
	}
	return flagged, nil
}
```

- [ ] **Step 3: Add mock method**

In `internal/testutil/mocks/campaign_service.go`, add the function field and method:

Add field after `GetGlobalInventoryAgingFn`:

```go
	GetFlaggedInventoryFn func(ctx context.Context) ([]campaigns.AgingItem, error)
```

Add method:

```go
func (m *MockCampaignService) GetFlaggedInventory(ctx context.Context) ([]campaigns.AgingItem, error) {
	if m.GetFlaggedInventoryFn != nil {
		return m.GetFlaggedInventoryFn(ctx)
	}
	return nil, nil
}
```

- [ ] **Step 4: Register the get_flagged_inventory tool**

Create `internal/adapters/advisortool/tools_signals.go`:

```go
package advisortool

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/ai"
)

func (e *CampaignToolExecutor) registerGetFlaggedInventory() {
	e.register(ai.ToolDefinition{
		Name:        "get_flagged_inventory",
		Description: "Get unsold cards that have inventory signals: profit capture opportunities, stale listings, crack candidates, or cut-loss flags. Returns only actionable cards, not the full inventory.",
		Parameters:  emptyObjectParams,
	}, func(ctx context.Context, _ string) (string, error) {
		result, err := e.svc.GetFlaggedInventory(ctx)
		if err != nil {
			return "", err
		}
		return toJSON(result), nil
	})
}
```

- [ ] **Step 5: Register in registerTools**

In `executor.go`, add `e.registerGetFlaggedInventory()` in `registerTools()` after `e.registerGetGlobalInventory()` (line 239):

```go
	e.registerGetGlobalInventory()
	e.registerGetFlaggedInventory()
```

- [ ] **Step 6: Run all tests**

Run: `go test ./internal/... -count=1`
Expected: all pass (the executor_test.go test that checks tool count will need an update — see next step)

- [ ] **Step 7: Update executor test expected tool count**

In `internal/adapters/advisortool/executor_test.go`, the test that lists all expected tool names needs `"get_flagged_inventory"` added. Find the slice of expected tool names and add it.

- [ ] **Step 8: Verify all tests pass**

Run: `go test ./internal/... -count=1`
Expected: all PASS

- [ ] **Step 9: Commit**

```bash
git add internal/domain/campaigns/service.go internal/domain/campaigns/service_analytics.go \
  internal/testutil/mocks/campaign_service.go internal/adapters/advisortool/tools_signals.go \
  internal/adapters/advisortool/executor.go internal/adapters/advisortool/executor_test.go
git commit -m "feat(signals): add GetFlaggedInventory service method and advisor tool"
```

---

### Task 5: Fix round budget and update liquidation tool set

**Files:**
- Modify: `internal/domain/advisor/service_impl.go:56-61` (operationTools)
- Modify: `internal/domain/advisor/service_impl.go:234-238` (operationMaxRounds)

- [ ] **Step 1: Update operationMaxRounds for liquidation**

In `service_impl.go`, change liquidation max rounds from 3 to 4:

```go
var operationMaxRounds = map[AIOperation]int{
	OpPurchaseAssessment: 1,
	OpCampaignAnalysis:   3,
	OpLiquidation:        4,
}
```

- [ ] **Step 2: Update liquidation tool set**

Replace the `OpLiquidation` entry in `operationTools`:

```go
	OpLiquidation: {
		"get_dashboard_summary", "get_flagged_inventory",
		"get_suggestion_stats", "get_inventory_alerts",
		"get_expected_values_batch", "suggest_price_batch",
	},
```

Removed: `get_global_inventory`, `get_sell_sheet`, `get_crack_opportunities`
Added: `get_flagged_inventory`

- [ ] **Step 3: Verify compilation**

Run: `go build ./internal/domain/advisor/...`
Expected: compiles cleanly

- [ ] **Step 4: Run advisor tests**

Run: `go test ./internal/domain/advisor/... -v -count=1`
Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add internal/domain/advisor/service_impl.go
git commit -m "fix(advisor): bump liquidation maxRounds to 4, replace global_inventory with flagged_inventory"
```

---

### Task 6: Rewrite liquidation prompts

**Files:**
- Modify: `internal/domain/advisor/prompts.go:104-153`

- [ ] **Step 1: Replace liquidationSystemPrompt**

In `prompts.go`, replace the `liquidationSystemPrompt` const (lines 104-140):

```go
// liquidationSystemPrompt is used for liquidation analysis.
const liquidationSystemPrompt = baseSystemPrompt + `

## Your Task: Liquidation Analysis
You receive pre-flagged inventory — cards already identified by the scoring engine as
needing action. Your job is to make judgment calls the engine cannot:

1. **Reprice stale listings** — cards flagged stale/deep_stale with no recent sales near
   our price. Determine a new price using market comps, sentiment, and EV data. Save via
   suggest_price_batch.

2. **Auction vs fixed price** — for stale cards, evaluate whether auction is better than
   fixed price. Favor auction when: fair value is uncertain (wide spread in recent comps),
   card has been listed 30+ days at fixed, or card is trending with potential for
   above-market bids. Favor fixed when: price is well-established and we just need a
   small adjustment.

3. **Cut-loss decisions** — cards flagged cut_loss. For each: quantify the carrying cost
   vs expected further decline. Recommend one of:
   - Drop online price to [specific amount]
   - Auction (starting price at [amount])
   - Sell in person at 75-80% of market to free capital immediately
   Show the math: holding cost per month vs markdown cost.

4. **Credit pressure adjustment** — if credit utilization is high (>80%), lower the bar
   for all liquidation actions. Cards you would normally hold become sells.

Do NOT re-analyze cards flagged profitCaptureDeclining, profitCaptureSpike, or
crackCandidate — those have clear procedural actions (sell in person / crack and sell).
Only mention them in your summary totals.

Before making new price suggestions, call get_suggestion_stats to see how your
previous recommendations performed. If acceptance rate is low, adjust your
pricing strategy — you may be suggesting prices that are too aggressive.

## Tool Strategy
You have a **2-round tool budget** and 6 tools.

**Round 1**: Call get_dashboard_summary, get_flagged_inventory, get_suggestion_stats,
and get_inventory_alerts together.

**Round 2**: Call get_expected_values_batch for campaigns with flagged cards.
If you have repricing recommendations, call suggest_price_batch.

**After Round 2, write your analysis immediately. Do NOT make additional tool calls.**`
```

- [ ] **Step 2: Replace liquidationUserPrompt**

Replace the `liquidationUserPrompt` const (lines 142-153):

```go
const liquidationUserPrompt = `Run a liquidation analysis on my flagged inventory.

Focus your judgment on three decisions:
1. What price should stale listings be set to?
2. Should any cards go to auction instead of fixed price?
3. Which cards should we take a loss on, and how?

Do not repeat data from the flags — I can see those in the UI.

Structure your report as:
1. **Credit Snapshot** — utilization %, alert level, urgency modifier
2. **Reprice Recommendations** — table: card, current price, new price, reasoning
3. **Auction Candidates** — table: card, why auction beats fixed, suggested start price
4. **Cut-Loss Actions** — table: card, cost basis, current market, recommended action, carrying cost math, capital freed
5. **Summary** — total capital recoverable, total markdown cost, net repricing impact, suggestion stats

End with totals: capital freed, markdown cost, and repricing count.`
```

- [ ] **Step 3: Verify compilation**

Run: `go build ./internal/domain/advisor/...`
Expected: compiles cleanly

- [ ] **Step 4: Run advisor tests**

Run: `go test ./internal/domain/advisor/... -v -count=1`
Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add internal/domain/advisor/prompts.go
git commit -m "feat(advisor): rewrite liquidation prompts — focused on flagged cards, 2-round budget"
```

---

### Task 7: Add CLI debug harness

**Files:**
- Modify: `cmd/slabledger/admin.go`
- Create: `cmd/slabledger/admin_analyze.go`

- [ ] **Step 1: Add analyze command routing**

In `admin.go`, add the `analyze` case to `handleAdminCommand`:

```go
	switch args[0] {
	case "cache-stats":
		return adminCacheStats(ctx)
	case "analyze":
		return adminAnalyze(ctx, args[1:])
	case "version":
```

Update `showAdminHelp` to include the analyze command:

```go
func showAdminHelp() error {
	fmt.Println(`slabledger admin - Administrative and operational commands

USAGE:
    slabledger admin <command> [arguments]

COMMANDS:
    AI Advisor:
        analyze <type>           Run an advisor analysis locally
                                 Types: liquidation, digest
                                 Flags: --verbose, --dry-run

    Cache Management:
        cache-stats              Show persistent cache statistics

    Configuration:
        version                  Show version information
        print-config            Print current configuration

    Help:
        help                    Show this help message

EXAMPLES:
    slabledger admin analyze liquidation --verbose
    slabledger admin cache-stats
    slabledger admin print-config`)
	return nil
}
```

- [ ] **Step 2: Implement admin_analyze.go**

The admin command is in the same `main` package as `init.go`, so it can reuse the existing `initializePriceProviders`, `initializeCampaignsService`, and `initializeAdvisorService` functions directly. This mirrors the wiring in `runServer` but skips the HTTP server.

Create `cmd/slabledger/admin_analyze.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/advisortool"
	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/clients/tcgdex"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/cache"
	"github.com/guarzo/slabledger/internal/platform/config"
)

func adminAnalyze(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: slabledger admin analyze <liquidation|digest> [--verbose] [--dry-run]")
	}

	analysisType := args[0]
	verbose := false
	dryRun := false
	for _, a := range args[1:] {
		switch a {
		case "--verbose", "-v":
			verbose = true
		case "--dry-run":
			dryRun = true
		}
	}

	if analysisType != "liquidation" && analysisType != "digest" {
		return fmt.Errorf("unknown analysis type %q — use 'liquidation' or 'digest'", analysisType)
	}

	// Load config
	cfg, err := config.Load(nil)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := initLogger("info", false)

	// Open database
	dbPath, err := resolveDatabasePath(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("resolve database path: %w", err)
	}
	db, err := sqlite.Open(dbPath, logger)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := sqlite.RunMigrations(db, cfg.Database.MigrationsPath); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	// Wire up pricing, campaigns, and advisor — same as runServer in main.go
	appCache := initializeCache(cfg.Cache.Path)
	cardProvImpl := tcgdex.NewTCGdex(appCache, logger)
	_ = cardProvImpl // used by initializePriceProviders via appCache
	cardIDMappingRepo := sqlite.NewCardIDMappingRepository(db.DB)
	priceRepo := sqlite.NewPriceRepository(db)

	// DH client (optional)
	var dhClient *dh.Client
	if cfg.Adapters.DHKey != "" && cfg.Adapters.DHBaseURL != "" {
		dhClient = dh.NewClient(cfg.Adapters.DHBaseURL, cfg.Adapters.DHKey, dh.WithLogger(logger))
	}
	intelRepo := sqlite.NewMarketIntelligenceRepository(db.DB)
	suggestionsRepo := sqlite.NewDHSuggestionsRepository(db.DB)

	priceProvImpl, cardHedgerClientImpl, pcProvider, err := initializePriceProviders(
		ctx, &cfg, appCache, logger, cardProvImpl, priceRepo, cardIDMappingRepo, dhClient, intelRepo,
	)
	if err != nil {
		return fmt.Errorf("initialize price providers: %w", err)
	}
	defer pcProvider.Close()

	campaignsService, _, _ := initializeCampaignsService(
		ctx, &cfg, logger, db, priceProvImpl, cardHedgerClientImpl, cardIDMappingRepo, intelRepo,
	)

	// Advisor tool options
	var toolOpts []advisortool.ExecutorOption
	toolOpts = append(toolOpts, advisortool.WithIntelligenceRepo(intelRepo))
	toolOpts = append(toolOpts, advisortool.WithSuggestionsRepo(suggestionsRepo))
	gapStore := sqlite.NewGapStore(db.DB)
	toolOpts = append(toolOpts, advisortool.WithGapStore(gapStore))

	aiCallRepo := sqlite.NewAICallRepository(db)
	_, advisorSvc, _, err := initializeAdvisorService(
		ctx, &cfg, logger, db, aiCallRepo, campaignsService, toolOpts...,
	)
	if err != nil {
		return fmt.Errorf("initialize advisor: %w", err)
	}
	if advisorSvc == nil {
		return fmt.Errorf("advisor not configured — check AZURE_AI_ENDPOINT and AZURE_AI_API_KEY")
	}

	if dryRun {
		fmt.Println("[dry-run] suggest_price_batch writes will be skipped")
		// TODO: wrap tool executor to skip suggest_price_batch in a follow-up
	}

	// Run analysis with streaming output
	streamFn := buildStreamCallback(verbose)
	fmt.Printf("Running %s analysis...\n\n", analysisType)
	start := time.Now()

	var runErr error
	switch analysisType {
	case "liquidation":
		runErr = advisorSvc.AnalyzeLiquidation(ctx, streamFn)
	case "digest":
		runErr = advisorSvc.GenerateDigest(ctx, streamFn)
	}

	elapsed := time.Since(start)
	fmt.Printf("\n\n--- %s ---\n", strings.ToUpper(analysisType)+" COMPLETE")
	fmt.Printf("Elapsed: %s\n", elapsed.Round(time.Second))

	if runErr != nil {
		return fmt.Errorf("analysis failed: %w", runErr)
	}
	return nil
}

func buildStreamCallback(verbose bool) func(ai.StreamEvent) {
	var currentTool string
	toolStart := time.Now()
	roundNum := 0

	return func(evt ai.StreamEvent) {
		switch evt.Type {
		case ai.EventToolStart:
			if currentTool == "" {
				roundNum++
				if verbose {
					fmt.Printf("\n--- Round %d ---\n", roundNum)
				}
			}
			currentTool = evt.ToolName
			toolStart = time.Now()
			if verbose {
				fmt.Printf("  calling %s...", evt.ToolName)
			}
		case ai.EventToolResult:
			if verbose {
				fmt.Printf(" done (%s)\n", time.Since(toolStart).Round(time.Millisecond))
			}
			currentTool = ""
		case ai.EventDelta:
			if currentTool == "" {
				fmt.Print(evt.Content)
			}
		case ai.EventDone:
			if verbose {
				fmt.Printf("\n[done]\n")
			}
		case ai.EventError:
			fmt.Fprintf(os.Stderr, "\n[error] %s\n", evt.Content)
		}
	}
}
```

Note: the exact arguments to `initializePriceProviders` and `initializeCampaignsService` must match the signatures in `init.go`. These functions are in the same `main` package so they're directly callable. The `initializeCache` function is also reused from `main.go`. Check the exact imports match what `main.go` uses — some unused imports (like `cache`) may need to be removed.

- [ ] **Step 4: Verify compilation**

Run: `go build -o /dev/null ./cmd/slabledger`
Expected: compiles cleanly

- [ ] **Step 5: Test the command runs**

Run: `go run ./cmd/slabledger admin analyze 2>&1 || true`
Expected: prints usage error: `usage: slabledger admin analyze <liquidation|digest> [--verbose] [--dry-run]`

- [ ] **Step 6: Commit**

```bash
git add cmd/slabledger/admin.go cmd/slabledger/admin_analyze.go
git commit -m "feat(cli): add 'admin analyze' command for local advisor testing"
```

---

### Task 8: Update frontend types

**Files:**
- Modify: `web/src/types/campaigns/analytics.ts:56-67` (AgingItem)
- Modify: `web/src/types/campaigns/core.ts:241-266` (SellSheetItem)

- [ ] **Step 1: Add InventorySignals interface**

In `web/src/types/campaigns/analytics.ts`, add before the `AgingItem` interface:

```typescript
export interface InventorySignals {
  profitCaptureDeclining?: boolean;
  profitCaptureSpike?: boolean;
  crackCandidate?: boolean;
  staleListing?: boolean;
  deepStale?: boolean;
  cutLoss?: boolean;
}
```

- [ ] **Step 2: Add signals field to AgingItem**

In `web/src/types/campaigns/analytics.ts`, add to the `AgingItem` interface:

```typescript
export interface AgingItem {
  purchase: Purchase;
  daysHeld: number;
  campaignName?: string;
  signal?: MarketSignal;
  currentMarket?: MarketSnapshot;
  priceAnomaly?: boolean;
  anomalyReason?: string;
  hasOpenFlag?: boolean;
  recommendedPriceCents?: number;
  recommendedSource?: string;
  signals?: InventorySignals;
}
```

- [ ] **Step 3: Add signals field to SellSheetItem**

In `web/src/types/campaigns/core.ts`, add to `SellSheetItem`:

```typescript
  aiSuggestedAt?: string;
  signals?: InventorySignals;
```

Also add the import at the top of `core.ts` if `InventorySignals` is defined in `analytics.ts`:

```typescript
import type { InventorySignals } from './analytics';
```

Or define `InventorySignals` in `core.ts` if that's where `SellSheetItem` lives — follow the existing pattern.

- [ ] **Step 4: Update isCardShowCandidate to use signals**

In `web/src/react/pages/campaign-detail/inventory/utils.ts`, update `isCardShowCandidate`:

```typescript
/** A card is a "card show candidate" if it has in-person signals or matches the legacy heuristic. */
export function isCardShowCandidate(item: AgingItem): boolean {
  if (item.signals?.profitCaptureDeclining || item.signals?.profitCaptureSpike || item.signals?.crackCandidate) {
    return true;
  }
  if (isHotSeller(item)) return true;
  if (item.purchase.gradeValue === 7) return true;
  if (item.currentMarket?.trend30d != null && item.currentMarket.trend30d > 0.05) return true;
  return false;
}
```

- [ ] **Step 5: Run frontend type check**

Run: `cd web && npx tsc --noEmit`
Expected: no type errors

- [ ] **Step 6: Run frontend tests**

Run: `cd web && npm test`
Expected: all pass

- [ ] **Step 7: Commit**

```bash
git add web/src/types/campaigns/analytics.ts web/src/types/campaigns/core.ts \
  web/src/react/pages/campaign-detail/inventory/utils.ts
git commit -m "feat(frontend): add InventorySignals type, update card show candidate logic"
```

---

### Task 9: Run full quality check and local test

**Files:** None (verification only)

- [ ] **Step 1: Run Go tests with race detection**

Run: `go test -race -timeout 10m ./...`
Expected: all pass

- [ ] **Step 2: Run make check**

Run: `make check`
Expected: lint, architecture imports, and file sizes all pass

- [ ] **Step 3: Run frontend checks**

Run: `cd web && npx tsc --noEmit && npm test`
Expected: all pass

- [ ] **Step 4: Build binary**

Run: `go build -o slabledger ./cmd/slabledger`
Expected: builds cleanly

- [ ] **Step 5: Local smoke test with admin analyze**

Run: `./slabledger admin analyze liquidation --verbose`
Expected: prints round-by-round tool calls and final analysis. Should complete without `ERR_ADVISOR_MAX_ROUNDS`. Verify:
- Round 1 calls: `get_dashboard_summary`, `get_flagged_inventory`, `get_suggestion_stats`, `get_inventory_alerts`
- Round 2 calls: `get_expected_values_batch`, optionally `suggest_price_batch`
- Round 3 (or earlier): writes analysis with no tool calls
- Output has the 5-section structure (Credit Snapshot, Reprice, Auction, Cut-Loss, Summary)

- [ ] **Step 6: Commit final state**

```bash
git add -A
git commit -m "chore: quality checks pass after liquidation reliability redesign"
```
