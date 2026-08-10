package portfolio

import (
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func intPtr(i int) *int { return &i }

func TestComputeConfidenceBuyCohorts(t *testing.T) {
	// Row A: conf=2, buyCost 7000, clAtPurchase 10000 -> "2","70-75".
	pA := inventory.Purchase{
		CLPolicyConfidenceMinAtPurchase: intPtr(2),
		BuyCostCents:                    7000,
		CLValueAtPurchaseCents:          10000,
	}
	// Row B: confidence nil -> "unknown".
	pB := inventory.Purchase{
		CLPolicyConfidenceMinAtPurchase: nil,
		BuyCostCents:                    5000,
		CLValueAtPurchaseCents:          10000,
	}
	// Row C: cl 0 -> buyTerms "unknown".
	pC := inventory.Purchase{
		CLPolicyConfidenceMinAtPurchase: intPtr(3),
		BuyCostCents:                    5000,
		CLValueAtPurchaseCents:          0,
	}
	// Boundary D: buyCost 7500, cl 10000 -> ratio exactly 0.75 -> "75-80" (lower-inclusive).
	pD := inventory.Purchase{
		CLPolicyConfidenceMinAtPurchase: intPtr(2),
		BuyCostCents:                    7500,
		CLValueAtPurchaseCents:          10000,
	}
	// Boundary E: buyCost 5000, cl 10000 -> ratio 0.50 -> "50-55" (not "<50").
	pE := inventory.Purchase{
		CLPolicyConfidenceMinAtPurchase: intPtr(5),
		BuyCostCents:                    5000,
		CLValueAtPurchaseCents:          10000,
	}
	// Boundary F1: ratio 1.00 -> ">=100".
	pF1 := inventory.Purchase{
		CLPolicyConfidenceMinAtPurchase: intPtr(4),
		BuyCostCents:                    10000,
		CLValueAtPurchaseCents:          10000,
	}
	// Boundary F2: ratio 0.4999 -> "<50".
	pF2 := inventory.Purchase{
		CLPolicyConfidenceMinAtPurchase: intPtr(4),
		BuyCostCents:                    4999,
		CLValueAtPurchaseCents:          10000,
	}
	// avgActiveListings: skips nil pointers but includes a real 0.
	zero := 0
	pG1 := inventory.Purchase{
		CLPolicyConfidenceMinAtPurchase: intPtr(9),
		BuyCostCents:                    6000,
		CLValueAtPurchaseCents:          10000,
		ActiveListingsAtPurchase:        &zero,
		SalesLast30dAtPurchase:          intPtr(1),
	}
	pG2 := inventory.Purchase{
		CLPolicyConfidenceMinAtPurchase: intPtr(9),
		BuyCostCents:                    6100,
		CLValueAtPurchaseCents:          10000,
		ActiveListingsAtPurchase:        nil,
		SalesLast30dAtPurchase:          nil,
	}

	rows := []inventory.PurchaseWithSale{
		{Purchase: pA}, {Purchase: pB}, {Purchase: pC}, {Purchase: pD}, {Purchase: pE},
		{Purchase: pF1}, {Purchase: pF2}, {Purchase: pG1}, {Purchase: pG2},
	}

	result := computeConfidenceBuyCohorts(rows)

	find := func(conf, buy string) *ConfBuyCohortRow {
		for i := range result {
			if result[i].ConfidenceBucket == conf && result[i].BuyTermsBucket == buy {
				return &result[i]
			}
		}
		return nil
	}

	if r := find("2", "70-75"); r == nil {
		t.Errorf("expected cohort (2, 70-75) for row A, not found")
	}
	if r := find("unknown", "50-55"); r == nil {
		t.Errorf("expected cohort (unknown, 50-55) for row B, not found")
	}
	if r := find("3", "unknown"); r == nil {
		t.Errorf("expected cohort (3, unknown) for row C, not found")
	}
	if r := find("2", "75-80"); r == nil {
		t.Errorf("expected cohort (2, 75-80) for boundary D (ratio 0.75), not found")
	}
	if r := find("5", "50-55"); r == nil {
		t.Errorf("expected cohort (5, 50-55) for boundary E (ratio 0.50), not found")
	} else if r := find("5", "<50"); r != nil {
		t.Errorf("boundary E (ratio 0.50) incorrectly landed in <50")
	}
	if r := find("4", ">=100"); r == nil {
		t.Errorf("expected cohort (4, >=100) for boundary F1 (ratio 1.00), not found")
	}
	if r := find("4", "<50"); r == nil {
		t.Errorf("expected cohort (4, <50) for boundary F2 (ratio 0.4999), not found")
	}

	g := find("9", "60-65")
	if g == nil {
		t.Fatalf("expected cohort (9, 60-65) for rows G1/G2, not found")
	}
	if g.CoverageMarket != 1 {
		t.Errorf("CoverageMarket=%d, want 1 (only G1 has non-nil market fields)", g.CoverageMarket)
	}
	if g.AvgActiveListings != 0 {
		t.Errorf("AvgActiveListings=%v, want 0 (real zero from G1, nil from G2 skipped)", g.AvgActiveListings)
	}
	if g.AvgSalesLast30d != 1 {
		t.Errorf("AvgSalesLast30d=%v, want 1", g.AvgSalesLast30d)
	}

	// Confirm ordering is stable/deterministic by running twice.
	result2 := computeConfidenceBuyCohorts(rows)
	if len(result) != len(result2) {
		t.Fatalf("non-deterministic length: %d vs %d", len(result), len(result2))
	}
	for i := range result {
		if result[i].ConfidenceBucket != result2[i].ConfidenceBucket || result[i].BuyTermsBucket != result2[i].BuyTermsBucket {
			t.Fatalf("non-deterministic order at index %d: (%s,%s) vs (%s,%s)",
				i, result[i].ConfidenceBucket, result[i].BuyTermsBucket,
				result2[i].ConfidenceBucket, result2[i].BuyTermsBucket)
		}
	}
	// Explicitly assert sort order for confidence "unknown" group's position.
	seenNumeric := false
	for _, r := range result {
		if r.ConfidenceBucket == "unknown" {
			seenNumeric = true // once we hit unknown, no more numeric buckets should follow
			continue
		}
		if seenNumeric {
			t.Fatalf("numeric confidence bucket %q found after unknown bucket", r.ConfidenceBucket)
		}
	}
}

func TestByReasonSplit(t *testing.T) {
	c := inventory.Campaign{ID: "c1", Name: "Reasons", Phase: inventory.PhaseActive}

	mk := func(reason string, forced bool, revenue, profit int) inventory.PurchaseWithSale {
		return inventory.PurchaseWithSale{
			Purchase: inventory.Purchase{CampaignID: "c1", BuyCostCents: revenue - profit, PurchaseDate: "2026-06-01"},
			Sale: &inventory.Sale{
				SalePriceCents:    revenue,
				NetProfitCents:    profit,
				SaleDate:          "2026-06-10",
				ForcedLiquidation: forced,
				SaleReason:        reason,
			},
		}
	}

	rows := []inventory.PurchaseWithSale{
		mk(inventory.SaleReasonDiscretionary, false, 10000, 1000),
		mk(inventory.SaleReasonAgingPolicy, false, 5000, 500),
		mk("", false, 3000, 300), // legacy/unknown: excluded from ByReason
	}

	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	result := ComputeAnalysis([]inventory.Campaign{c}, rows, nil, "", now)
	if len(result.Campaigns) != 1 {
		t.Fatalf("expected 1 campaign, got %d", len(result.Campaigns))
	}
	byReason := result.Campaigns[0].PNL.ByReason

	wantKeys := []string{
		inventory.SaleReasonDiscretionary,
		inventory.SaleReasonInvoicePressure,
		inventory.SaleReasonAgingPolicy,
		inventory.SaleReasonBulkLot,
		inventory.SaleReasonShowClearout,
	}
	if len(byReason) != len(wantKeys) {
		t.Fatalf("ByReason has %d keys, want %d", len(byReason), len(wantKeys))
	}
	for _, k := range wantKeys {
		if _, ok := byReason[k]; !ok {
			t.Errorf("ByReason missing key %q", k)
		}
	}

	disc := byReason[inventory.SaleReasonDiscretionary]
	if disc.SoldCount != 1 || disc.RevenueCents != 10000 || disc.NetProfitCents != 1000 {
		t.Errorf("Discretionary bucket = %+v, want SoldCount=1 Revenue=10000 NetProfit=1000", disc)
	}

	aging := byReason[inventory.SaleReasonAgingPolicy]
	if aging.SoldCount != 1 || aging.RevenueCents != 5000 {
		t.Errorf("AgingPolicy bucket = %+v, want SoldCount=1 Revenue=5000", aging)
	}

	// Zero-sale key should have zero PNLBlock.
	invoicePressure := byReason[inventory.SaleReasonInvoicePressure]
	if invoicePressure.SoldCount != 0 || invoicePressure.RevenueCents != 0 {
		t.Errorf("InvoicePressure bucket should be zero-valued, got %+v", invoicePressure)
	}

	// The "" reason row still counts toward the top-level Discretionary/Forced split.
	if result.Campaigns[0].PNL.Discretionary.SoldCount != 3 {
		t.Errorf("Discretionary.SoldCount=%d, want 3 (all 3 sales are non-forced)", result.Campaigns[0].PNL.Discretionary.SoldCount)
	}
}

func TestBuyTermsBucketOrdering(t *testing.T) {
	// purchaseFor builds a Purchase whose buy-cost/CL-value ratio produces
	// the given percentage (as buyTermsBucket computes it: BuyCostCents*100/
	// CLValueAtPurchaseCents), or an "unknown" bucket when clCents is 0.
	purchaseFor := func(pct, clCents int) inventory.Purchase {
		return inventory.Purchase{BuyCostCents: pct, CLValueAtPurchaseCents: clCents}
	}

	tests := []struct {
		name       string
		bucket     inventory.Purchase
		wantLabel  string
		before     inventory.Purchase
		beforeWant string
	}{
		{
			name:       "below-50 sorts before 50-55",
			bucket:     purchaseFor(40, 100),
			wantLabel:  "<50",
			before:     purchaseFor(52, 100),
			beforeWant: "50-55",
		},
		{
			name:       "50-55 sorts before 60-65",
			bucket:     purchaseFor(52, 100),
			wantLabel:  "50-55",
			before:     purchaseFor(62, 100),
			beforeWant: "60-65",
		},
		{
			name:       "90-95 sorts before >=100",
			bucket:     purchaseFor(92, 100),
			wantLabel:  "90-95",
			before:     purchaseFor(100, 100),
			beforeWant: ">=100",
		},
		{
			name:       ">=100 sorts before unknown",
			bucket:     purchaseFor(100, 100),
			wantLabel:  ">=100",
			before:     purchaseFor(0, 0),
			beforeWant: "unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buyTermsBucket(tt.bucket)
			if got.label != tt.wantLabel {
				t.Fatalf("buyTermsBucket(%+v).label = %q, want %q", tt.bucket, got.label, tt.wantLabel)
			}
			before := buyTermsBucket(tt.before)
			if before.label != tt.beforeWant {
				t.Fatalf("buyTermsBucket(%+v).label = %q, want %q", tt.before, before.label, tt.beforeWant)
			}
			if got.rank >= before.rank {
				t.Errorf("rank(%q)=%d, want < rank(%q)=%d", got.label, got.rank, before.label, before.rank)
			}
		})
	}
}
