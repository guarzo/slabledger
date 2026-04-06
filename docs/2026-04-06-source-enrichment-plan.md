# Source Enrichment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enrich the pricing pipeline so Sources/SourceCount represent independent data signals — break out DH per-platform sales, add Card Ladder as a source.

**Architecture:** The `dhprice` provider groups DH sales by (grade, platform) to populate per-platform detail and distinct platform sources. Card Ladder is appended as a source in `applyCLCorrection` after evaluating multi-source trust. A new `Sources []string` field on `MarketSnapshot` provides full traceability.

**Tech Stack:** Go 1.26, existing `pricing.EbayGradeDetail` / `GradeDetail` types, SQLite (no migration needed — sources stored in snapshot_json blob).

---

### Task 1: Add `Sources` field to `MarketSnapshot`

**Files:**
- Modify: `internal/domain/campaigns/service.go:60-62`

- [ ] **Step 1: Add the Sources field**

In `internal/domain/campaigns/service.go`, add `Sources []string` between `SourceCount` and `Confidence`:

```go
// Pricing metadata
SourceCount int      `json:"sourceCount,omitempty"`
Sources     []string `json:"sources,omitempty"`
Confidence  float64  `json:"confidence,omitempty"`
```

- [ ] **Step 2: Run tests to verify no breakage**

Run: `go test ./internal/domain/campaigns/... -count=1`
Expected: All tests PASS (new field is zero-value, existing JSON without it deserializes fine)

- [ ] **Step 3: Commit**

```bash
git add internal/domain/campaigns/service.go
git commit -m "feat: add Sources []string to MarketSnapshot for source traceability"
```

---

### Task 2: Platform-aware `buildPrice` in `dhprice` provider

**Files:**
- Modify: `internal/adapters/clients/dhprice/provider.go:139-204`
- Test: `internal/adapters/clients/dhprice/provider_test.go`

- [ ] **Step 1: Write failing test for eBay platform breakdown**

Add this test to `internal/adapters/clients/dhprice/provider_test.go`:

```go
func TestBuildPrice_PlatformBreakdown(t *testing.T) {
	sales := []dh.RecentSale{
		{GradingCompany: "PSA", Grade: "10", Price: 100.00, Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "10", Price: 120.00, Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "10", Price: 110.00, Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "9", Price: 50.00, Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "9", Price: 60.00, Platform: "ebay"},
	}

	got := buildPrice("Charizard", sales)
	if got == nil {
		t.Fatal("expected non-nil price")
	}

	// Sources should list distinct platforms, not "doubleholo"
	if len(got.Sources) != 1 || got.Sources[0] != "ebay" {
		t.Errorf("Sources = %v, want [ebay]", got.Sources)
	}

	// PSA10 Ebay detail should be populated
	psa10 := got.GradeDetails[pricing.GradePSA10.String()]
	if psa10 == nil {
		t.Fatal("missing PSA10 grade detail")
	}
	if psa10.Ebay == nil {
		t.Fatal("PSA10 Ebay detail is nil")
	}
	if psa10.Ebay.PriceCents != 11000 {
		t.Errorf("PSA10 Ebay.PriceCents = %d, want 11000", psa10.Ebay.PriceCents)
	}
	if psa10.Ebay.MinCents != 10000 {
		t.Errorf("PSA10 Ebay.MinCents = %d, want 10000", psa10.Ebay.MinCents)
	}
	if psa10.Ebay.MaxCents != 12000 {
		t.Errorf("PSA10 Ebay.MaxCents = %d, want 12000", psa10.Ebay.MaxCents)
	}
	if psa10.Ebay.SalesCount != 3 {
		t.Errorf("PSA10 Ebay.SalesCount = %d, want 3", psa10.Ebay.SalesCount)
	}
	if psa10.Ebay.Confidence != "medium" {
		t.Errorf("PSA10 Ebay.Confidence = %q, want medium", psa10.Ebay.Confidence)
	}

	// Estimate should still contain cross-platform aggregate (same values here since all eBay)
	if psa10.Estimate == nil {
		t.Fatal("PSA10 Estimate is nil")
	}
	if psa10.Estimate.PriceCents != 11000 {
		t.Errorf("PSA10 Estimate.PriceCents = %d, want 11000", psa10.Estimate.PriceCents)
	}
}

func TestBuildPrice_MixedPlatforms(t *testing.T) {
	sales := []dh.RecentSale{
		{GradingCompany: "PSA", Grade: "10", Price: 100.00, Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "10", Price: 120.00, Platform: "ebay"},
		{GradingCompany: "PSA", Grade: "10", Price: 90.00, Platform: "tcgplayer"},
		{GradingCompany: "PSA", Grade: "10", Price: 95.00, Platform: "tcgplayer"},
	}

	got := buildPrice("Pikachu", sales)
	if got == nil {
		t.Fatal("expected non-nil price")
	}

	// Sources should contain both platforms (sorted)
	if len(got.Sources) != 2 {
		t.Fatalf("Sources = %v, want 2 platforms", got.Sources)
	}
	wantSources := map[string]bool{"ebay": true, "tcgplayer": true}
	for _, s := range got.Sources {
		if !wantSources[s] {
			t.Errorf("unexpected source %q", s)
		}
	}

	// Estimate is cross-platform aggregate: median of [90, 95, 100, 120] = 97.50
	psa10 := got.GradeDetails[pricing.GradePSA10.String()]
	if psa10 == nil || psa10.Estimate == nil {
		t.Fatal("missing PSA10 Estimate detail")
	}
	if psa10.Estimate.PriceCents != 9750 {
		t.Errorf("Estimate.PriceCents = %d, want 9750 (cross-platform median)", psa10.Estimate.PriceCents)
	}

	// Ebay detail: median of [100, 120] = 110
	if psa10.Ebay == nil {
		t.Fatal("PSA10 Ebay detail is nil")
	}
	if psa10.Ebay.PriceCents != 11000 {
		t.Errorf("Ebay.PriceCents = %d, want 11000", psa10.Ebay.PriceCents)
	}
	if psa10.Ebay.SalesCount != 2 {
		t.Errorf("Ebay.SalesCount = %d, want 2", psa10.Ebay.SalesCount)
	}
}

func TestBuildPrice_NoPlatform(t *testing.T) {
	// Sales with empty platform string should still work (treated as unknown platform)
	sales := []dh.RecentSale{
		{GradingCompany: "PSA", Grade: "10", Price: 100.00, Platform: ""},
	}

	got := buildPrice("Mewtwo", sales)
	if got == nil {
		t.Fatal("expected non-nil price")
	}

	// Estimate should still be populated
	psa10 := got.GradeDetails[pricing.GradePSA10.String()]
	if psa10 == nil || psa10.Estimate == nil {
		t.Fatal("missing PSA10 Estimate")
	}

	// Ebay should NOT be populated (platform is not "ebay")
	if psa10.Ebay != nil {
		t.Error("Ebay detail should be nil for empty platform")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapters/clients/dhprice/... -run TestBuildPrice_Platform -v`
Expected: FAIL — `Sources` still returns `["doubleholo"]`, `Ebay` is nil

- [ ] **Step 3: Implement platform-aware `buildPrice`**

Replace the `buildPrice` function in `internal/adapters/clients/dhprice/provider.go`:

```go
// ebayConfidence returns a confidence label based on sale count.
func ebayConfidence(saleCount int) string {
	switch {
	case saleCount >= 10:
		return "high"
	case saleCount >= 3:
		return "medium"
	default:
		return "low"
	}
}

// buildPrice groups sales by grade, computes per-platform and aggregate medians, and assembles a Price.
func buildPrice(productName string, sales []dh.RecentSale) *pricing.Price {
	type gradeplatform struct {
		grade    pricing.Grade
		platform string
	}

	// Group sale prices by (grade, platform) and by grade (aggregate).
	byGradePlatform := make(map[gradeplatform][]float64)
	byGrade := make(map[pricing.Grade][]float64)
	platforms := make(map[string]bool)

	for _, s := range sales {
		key := s.GradingCompany + " " + s.Grade
		g, ok := gradeKey[key]
		if !ok {
			continue
		}
		platform := strings.ToLower(s.Platform)
		byGrade[g] = append(byGrade[g], s.Price)
		if platform != "" {
			byGradePlatform[gradeplatform{g, platform}] = append(byGradePlatform[gradeplatform{g, platform}], s.Price)
			platforms[platform] = true
		}
	}

	if len(byGrade) == 0 {
		return nil
	}

	var grades pricing.GradedPrices
	details := make(map[string]*pricing.GradeDetail, len(byGrade))

	for g, prices := range byGrade {
		med := median(prices)
		cents := mathutil.ToCents(med)
		pricing.SetGradePrice(&grades, g, cents)

		lo, hi := priceRange(prices)
		detail := &pricing.GradeDetail{
			Estimate: &pricing.EstimateGradeDetail{
				PriceCents: cents,
				LowCents:   mathutil.ToCents(lo),
				HighCents:  mathutil.ToCents(hi),
				Confidence: dhConfidence,
			},
		}

		// Populate eBay-specific detail from eBay sales for this grade.
		if ebayPrices, ok := byGradePlatform[gradeplatform{g, "ebay"}]; ok && len(ebayPrices) > 0 {
			eMed := median(ebayPrices)
			eLo, eHi := priceRange(ebayPrices)
			detail.Ebay = &pricing.EbayGradeDetail{
				PriceCents:  mathutil.ToCents(eMed),
				MedianCents: mathutil.ToCents(eMed),
				MinCents:    mathutil.ToCents(eLo),
				MaxCents:    mathutil.ToCents(eHi),
				SalesCount:  len(ebayPrices),
				Confidence:  ebayConfidence(len(ebayPrices)),
			}
		}

		details[g.String()] = detail
	}

	// Pick the best available grade price for Amount.
	amount := grades.PSA10Cents
	if amount == 0 {
		for _, fallback := range []int64{
			grades.BGS10Cents,
			grades.Grade95Cents,
			grades.PSA9Cents,
			grades.PSA8Cents,
			grades.PSA7Cents,
			grades.PSA6Cents,
		} {
			if fallback != 0 {
				amount = fallback
				break
			}
		}
	}

	// Sources = distinct platforms seen in sales data (sorted for deterministic output).
	sources := make([]string, 0, len(platforms))
	for p := range platforms {
		sources = append(sources, p)
	}
	sort.Strings(sources)

	return &pricing.Price{
		ProductName:  productName,
		Amount:       amount,
		Currency:     "USD",
		Source:       pricing.SourceDH,
		Grades:       grades,
		Confidence:   dhConfidence,
		GradeDetails: details,
		Sources:      sources,
	}
}
```

Also add `"sort"` and `"strings"` to the imports (the file already imports `"sort"`, so just add `"strings"`).

- [ ] **Step 4: Update existing test expectations**

In `TestGetPrice_WithSales`, the test at line 166-168 expects `Sources = ["doubleholo"]`. The existing test sales have no `Platform` field set (empty string), so `Sources` will now be empty. Add `Platform: "ebay"` to the existing test sales and update the assertion:

```go
// In TestGetPrice_WithSales, update sales to include Platform:
sales := []dh.RecentSale{
	{GradingCompany: "PSA", Grade: "10", Price: 100.00, Platform: "ebay"},
	{GradingCompany: "PSA", Grade: "10", Price: 120.00, Platform: "ebay"},
	{GradingCompany: "PSA", Grade: "10", Price: 110.00, Platform: "ebay"},
	{GradingCompany: "PSA", Grade: "9", Price: 50.00, Platform: "ebay"},
	{GradingCompany: "PSA", Grade: "9", Price: 60.00, Platform: "ebay"},
	{GradingCompany: "PSA", Grade: "8", Price: 30.00, Platform: "ebay"},
	{GradingCompany: "BGS", Grade: "10", Price: 200.00, Platform: "ebay"},
	{GradingCompany: "BGS", Grade: "9.5", Price: 80.00, Platform: "ebay"},
	{GradingCompany: "CGC", Grade: "9.5", Price: 70.00, Platform: "ebay"},
	{GradingCompany: "XYZ", Grade: "99", Price: 999.00, Platform: "ebay"}, // unknown grade, still skipped
}

// Update source assertion (line 166-168):
if len(got.Sources) != 1 || got.Sources[0] != "ebay" {
	t.Errorf("Sources = %v, want [ebay]", got.Sources)
}
```

Also in `TestGetPrice_AmountFallback`, add Platform to sales:

```go
sales := []dh.RecentSale{
	{GradingCompany: "PSA", Grade: "9", Price: 50.00, Platform: "ebay"},
	{GradingCompany: "PSA", Grade: "9", Price: 60.00, Platform: "ebay"},
}
```

And in `TestLookupCard`:

```go
sales := []dh.RecentSale{
	{GradingCompany: "PSA", Grade: "10", Price: 50.00, Platform: "ebay"},
}
```

- [ ] **Step 5: Run all dhprice tests**

Run: `go test ./internal/adapters/clients/dhprice/... -v -count=1`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/clients/dhprice/provider.go internal/adapters/clients/dhprice/provider_test.go
git commit -m "feat: break out DH sales by platform, populate EbayGradeDetail per grade"
```

---

### Task 3: Wire Sources through `pricelookup` adapter

**Files:**
- Modify: `internal/adapters/clients/pricelookup/adapter.go:156-160`
- Test: `internal/adapters/clients/pricelookup/adapter_test.go`

- [ ] **Step 1: Write failing test for Sources passthrough**

Add this test to `internal/adapters/clients/pricelookup/adapter_test.go`:

```go
func TestGetMarketSnapshot_SourcesPassthrough(t *testing.T) {
	mock := &mockPriceProvider{
		lookupFn: func(_ context.Context, _ string, _ domainCards.Card) (*pricing.Price, error) {
			return &pricing.Price{
				Grades:  pricing.GradedPrices{PSA10Cents: 10000},
				Sources: []string{"ebay", "tcgplayer"},
				GradeDetails: map[string]*pricing.GradeDetail{
					"psa10": {
						Ebay: &pricing.EbayGradeDetail{
							PriceCents: 10500,
							SalesCount: 5,
							Confidence: "medium",
						},
						Estimate: &pricing.EstimateGradeDetail{
							PriceCents: 10000,
							Confidence: 0.9,
						},
					},
				},
			}, nil
		},
	}
	adapter := NewAdapter(mock)
	snap, err := adapter.GetMarketSnapshot(context.Background(), testCard, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}

	// Sources should be passed through from Price
	if len(snap.Sources) != 2 {
		t.Fatalf("Sources = %v, want [ebay tcgplayer]", snap.Sources)
	}
	if snap.Sources[0] != "ebay" || snap.Sources[1] != "tcgplayer" {
		t.Errorf("Sources = %v, want [ebay tcgplayer]", snap.Sources)
	}
	if snap.SourceCount != 2 {
		t.Errorf("SourceCount = %d, want 2", snap.SourceCount)
	}

	// Ebay data should flow through to LastSoldCents fallback
	if snap.LastSoldCents != 10500 {
		t.Errorf("LastSoldCents = %d, want 10500 (from Ebay fallback)", snap.LastSoldCents)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/clients/pricelookup/... -run TestGetMarketSnapshot_SourcesPassthrough -v`
Expected: FAIL — `snap.Sources` is nil

- [ ] **Step 3: Add Sources passthrough in adapter**

In `internal/adapters/clients/pricelookup/adapter.go`, replace the source count block (lines 156-160):

```go
	// Confidence and source count
	snap.Confidence = price.Confidence
	if len(price.Sources) > 0 {
		snap.Sources = price.Sources
		snap.SourceCount = len(price.Sources)
	}
```

- [ ] **Step 4: Update existing test fixture**

In `TestGetMarketSnapshot_Full` (around line 487), the fixture has `Sources: []string{"doubleholo", "ebay"}`. Update to just `["ebay"]` to match the new platform-based semantics, and update the `SourceCount` assertion:

```go
Sources: []string{"ebay"},
```

And update the assertion at line 530-532:

```go
if snap.SourceCount != 1 {
	t.Errorf("SourceCount = %d, want 1", snap.SourceCount)
}
```

Also add a Sources assertion after the SourceCount check:

```go
if len(snap.Sources) != 1 || snap.Sources[0] != "ebay" {
	t.Errorf("Sources = %v, want [ebay]", snap.Sources)
}
```

- [ ] **Step 5: Run all pricelookup tests**

Run: `go test ./internal/adapters/clients/pricelookup/... -v -count=1`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/clients/pricelookup/adapter.go internal/adapters/clients/pricelookup/adapter_test.go
git commit -m "feat: pass Sources through pricelookup adapter to MarketSnapshot"
```

---

### Task 4: Add Card Ladder as source in `applyCLCorrection`

**Files:**
- Modify: `internal/domain/campaigns/service_snapshots.go:55-91`
- Test: `internal/domain/campaigns/service_snapshots_test.go`

- [ ] **Step 1: Write failing tests for CL source tracking**

Add these test cases to the `TestApplyCLCorrection` table in `internal/domain/campaigns/service_snapshots_test.go`. Add `wantSources []string` to the test struct:

First, update the test struct to add the new field:

```go
tests := []struct {
	name         string
	snapshot     *MarketSnapshot
	clValueCents int
	// Expected fields after correction
	wantMedian          int
	wantGradePriceCents int
	wantConservative    int
	wantOptimistic      int
	wantP10             int
	wantP90             int
	wantCLValueCents    int
	wantCLDeviationPct  float64
	wantCLAnchorApplied bool
	wantPricingGap      bool
	wantSources         []string // NEW
}{
```

Then update all existing test cases to include `wantSources`. For cases where `clValueCents > 0`, expect `"cardladder"` in sources. For `clValueCents <= 0`, expect whatever sources the snapshot started with.

Update the `nil snapshot` case — no change needed (returns early).

Update the `clValueCents zero` case:
```go
wantSources: nil, // no CL, no sources added
```

Update the `clValueCents negative` case:
```go
wantSources: nil,
```

Update the `pricing gap fills from CL` case:
```go
wantSources: []string{"cardladder"},
```

Update the `low deviation single source no correction` case. This snapshot starts with `SourceCount: 1` but no `Sources` set — add initial sources:
```go
snapshot: &MarketSnapshot{MedianCents: 9000, SourceCount: 1, Sources: []string{"ebay"}},
wantSources: []string{"ebay", "cardladder"},
```

Update the `high deviation single source corrects` case:
```go
snapshot: &MarketSnapshot{MedianCents: 5000, SourceCount: 1, Sources: []string{"ebay"}},
wantSources: []string{"ebay", "cardladder"},
```

Update the `high deviation single source market above CL trusts market` case:
```go
snapshot: &MarketSnapshot{MedianCents: 5700, SourceCount: 1, Sources: []string{"ebay"}},
wantSources: []string{"ebay", "cardladder"},
```

Update the `high deviation multi-source trusts market` case (rename from "trusts fusion"):
```go
name:     "high deviation multi-source trusts market",
snapshot: &MarketSnapshot{MedianCents: 5000, SourceCount: 2, Sources: []string{"ebay", "tcgplayer"}},
wantSources: []string{"ebay", "tcgplayer", "cardladder"},
```

Update the `exactly at threshold no correction` case:
```go
snapshot: &MarketSnapshot{MedianCents: 6000, SourceCount: 1, Sources: []string{"ebay"}},
wantSources: []string{"ebay", "cardladder"},
```

Update the `CLValueCents always set on snapshot` case:
```go
snapshot: &MarketSnapshot{MedianCents: 9500, SourceCount: 3, Sources: []string{"ebay", "tcgplayer", "other"}},
wantSources: []string{"ebay", "tcgplayer", "other", "cardladder"},
```

Update the `no median but GradePriceCents present still anchors` case:
```go
snapshot: &MarketSnapshot{MedianCents: 0, GradePriceCents: 8000, SourceCount: 1, Sources: []string{"ebay"}},
wantSources: []string{"ebay", "cardladder"},
```

Update the `CL anchor clears IsEstimated` case:
```go
snapshot: &MarketSnapshot{MedianCents: 5000, SourceCount: 1, Sources: []string{"ebay"}, IsEstimated: true},
wantSources: []string{"ebay", "cardladder"},
```

Add the Sources assertion to the test verification block (after the PricingGap check):

```go
if tc.wantSources != nil {
	if len(tc.snapshot.Sources) != len(tc.wantSources) {
		t.Errorf("Sources = %v, want %v", tc.snapshot.Sources, tc.wantSources)
	} else {
		for i, s := range tc.wantSources {
			if tc.snapshot.Sources[i] != s {
				t.Errorf("Sources[%d] = %q, want %q", i, tc.snapshot.Sources[i], s)
			}
		}
	}
	if tc.snapshot.SourceCount != len(tc.wantSources) {
		t.Errorf("SourceCount = %d, want %d", tc.snapshot.SourceCount, len(tc.wantSources))
	}
}
```

Also update `TestApplyCLCorrection_EstimateFallback` snapshots to include Sources:
```go
snapshot := &MarketSnapshot{
	MedianCents:         10000,
	SourceCount:         2,
	Sources:             []string{"ebay", "tcgplayer"},
	EstimatedValueCents: tc.estimatedValueCents,
	EstimateSource:      tc.estimateSource,
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/domain/campaigns/... -run TestApplyCLCorrection -v`
Expected: FAIL — Sources assertions fail (Sources is never populated by `applyCLCorrection`)

- [ ] **Step 3: Implement CL source tracking in `applyCLCorrection`**

In `internal/domain/campaigns/service_snapshots.go`, update `applyCLCorrection`:

Update the doc comment (lines 55-65):

```go
// applyCLCorrection adjusts snapshot values when market data produces unreliable results.
// It compares the snapshot median against the purchase's CL value and corrects when:
//   - The snapshot has a pricing gap (no median) and CL is available
//   - The market price is BELOW CL with high deviation and only 1 market source (likely a variant mismatch)
//
// CL acts as a price floor: when a single source reports a price ABOVE CL, the market data
// is trusted (the card may genuinely be worth more than CL's estimate). Anchoring only
// applies downward to prevent underpricing.
//
// When multiple market sources agree (len(Sources) >= 2), the market result is trusted even if
// it diverges from CL — multi-source agreement is a stronger signal than CL alone.
//
// After evaluation, Card Ladder is appended to Sources as an independent price signal.
```

At the end of the function (before the closing `}`), after the estimate fallback block, add:

```go
	// Append Card Ladder as a source — CL is an independent price signal
	// regardless of whether anchoring was applied.
	snapshot.Sources = append(snapshot.Sources, "cardladder")
	snapshot.SourceCount = len(snapshot.Sources)
```

Update the multi-source trust guard at line 88 to use `len(snapshot.Sources)` instead of `snapshot.SourceCount`:

```go
		// Only correct single-source results when market price is BELOW CL with high deviation.
		// When market is above CL, trust the market — the card may be worth more than CL thinks.
		// Multi-source market data that diverges from CL is more likely correct (CL may be stale).
		if deviation > clDeviationThreshold && len(snapshot.Sources) <= 1 && snapshot.MedianCents < clValueCents {
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/campaigns/... -run TestApplyCLCorrection -v -count=1`
Expected: All PASS

- [ ] **Step 5: Run full campaigns test suite**

Run: `go test ./internal/domain/campaigns/... -count=1`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/domain/campaigns/service_snapshots.go internal/domain/campaigns/service_snapshots_test.go
git commit -m "feat: add Card Ladder as source in applyCLCorrection, update multi-source trust logic"
```

---

### Task 5: Fix stale fusion comments

**Files:**
- Modify: `cmd/slabledger/main.go:235`
- Modify: `internal/adapters/scheduler/inventory_refresh.go:109`
- Modify: `internal/domain/errors/errors_test.go:181`

- [ ] **Step 1: Fix main.go comment**

In `cmd/slabledger/main.go`, line 235, change:

```go
// Initialize DH client (optional — market intelligence + fusion source)
```

to:

```go
// Initialize DH client (optional — market intelligence + pricing source)
```

- [ ] **Step 2: Fix inventory_refresh.go comment**

In `internal/adapters/scheduler/inventory_refresh.go`, line 109, change:

```go
// are already coalesced at the fusion provider layer via singleflight
```

to:

```go
// are already coalesced at the DH provider layer via singleflight
```

- [ ] **Step 3: Fix errors_test.go stale assertion message**

In `internal/domain/errors/errors_test.go`, line 181, change:

```go
t.Errorf("Context[provider] = %v, want PriceCharting", err2.Context["provider"])
```

to:

```go
t.Errorf("Context[provider] = %v, want doubleholo", err2.Context["provider"])
```

- [ ] **Step 4: Run affected tests**

Run: `go test ./internal/domain/errors/... ./internal/adapters/scheduler/... -count=1`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/slabledger/main.go internal/adapters/scheduler/inventory_refresh.go internal/domain/errors/errors_test.go
git commit -m "fix: update stale fusion/PriceCharting comments to reflect DH-only pricing"
```

---

### Task 6: Full test suite and lint verification

**Files:** None (verification only)

- [ ] **Step 1: Run full test suite with race detection**

Run: `go test -race -timeout 10m ./...`
Expected: All PASS

- [ ] **Step 2: Run quality checks**

Run: `make check`
Expected: 0 lint issues, architecture check passed, file size check passed

- [ ] **Step 3: Verify no regressions in pricelookup adapter**

The `buildSourcePrices` function in `pricelookup/adapter.go` should now produce "eBay" source prices from the populated `detail.Ebay`. Run:

Run: `go test ./internal/adapters/clients/pricelookup/... -v -count=1`
Expected: All PASS — the eBay fallback path at line 100-105 and the eBay source price path at line 287-307 now activate with real data
