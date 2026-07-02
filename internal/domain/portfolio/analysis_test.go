package portfolio

import (
	"math"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// approx asserts that got is within tol of want.
func approx(t *testing.T, label string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: got %v, want %v (±%v)", label, got, want, tol)
	}
}

// --- TestParseRange ---

func TestParseRange(t *testing.T) {
	cases := []struct {
		input   string
		wantMin float64
		wantMax float64
		wantOK  bool
	}{
		{"9-10", 9, 10, true},
		{"10", 10, 10, true},
		{"", 0, 0, false},
		{"junk", 0, 0, false},
		{"50-500", 50, 500, true},
	}
	for _, tc := range cases {
		name := tc.input
		if name == "" {
			name = "(empty)"
		}
		t.Run(name, func(t *testing.T) {
			gotMin, gotMax, ok := parseRange(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("parseRange(%q) ok=%v, want %v", tc.input, ok, tc.wantOK)
			}
			if ok {
				if gotMin != tc.wantMin {
					t.Errorf("parseRange(%q) min=%v, want %v", tc.input, gotMin, tc.wantMin)
				}
				if gotMax != tc.wantMax {
					t.Errorf("parseRange(%q) max=%v, want %v", tc.input, gotMax, tc.wantMax)
				}
			}
		})
	}
}

// --- TestComputeAnalysisBPCLAtBuy ---

func TestComputeAnalysisBPCLAtBuy(t *testing.T) {
	// Worked example from the 2026-06-22 session (Mega Dragonite):
	//   buyCost=51100, clAtBuy=68133 → dollar-weighted BPCL = 51100/68133 ≈ 0.75
	//   current CL=44400 → drift = (44400-68133)/68133*100 ≈ -34.8 (±0.1)
	// A second purchase with CLValueAtPurchaseCents==0 raises Total but not N.

	c := inventory.Campaign{ID: "c1", Name: "Dragonite", Phase: inventory.PhaseActive, BuyTermsCLPct: 0.75}

	p1 := inventory.Purchase{
		CampaignID:             "c1",
		BuyCostCents:           51100,
		CLValueCents:           44400, // current CL (used for drift)
		CLValueAtPurchaseCents: 68133, // snapshot at buy
		PurchaseDate:           "2026-06-01",
	}
	p2 := inventory.Purchase{
		CampaignID:             "c1",
		BuyCostCents:           30000,
		CLValueCents:           35000,
		CLValueAtPurchaseCents: 0, // no snapshot — excluded from BPCL/drift, counted in Total
		PurchaseDate:           "2026-06-01",
	}

	rows := []inventory.PurchaseWithSale{{Purchase: p1}, {Purchase: p2}}
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	result := ComputeAnalysis([]inventory.Campaign{c}, rows, nil, "", now)

	if len(result.Campaigns) != 1 {
		t.Fatalf("expected 1 campaign, got %d", len(result.Campaigns))
	}
	bp := result.Campaigns[0].BPCLAtBuy

	if bp.Total != 2 {
		t.Errorf("Total=%d, want 2", bp.Total)
	}
	if bp.N != 1 {
		t.Errorf("N=%d, want 1", bp.N)
	}
	approx(t, "CoveragePct", bp.CoveragePct, 50.0, 0.01)
	approx(t, "DollarWeighted", bp.DollarWeighted, 0.75, 0.01)
	approx(t, "MeanDriftPct", bp.MeanDriftPct, -34.8, 0.1)
}

// --- TestComputeAnalysisForcedSplit ---

func TestComputeAnalysisForcedSplit(t *testing.T) {
	// Discretionary sale: revenue=12000, netProfit=2000 → cost=10000 → ROIPct=20%
	// Forced sale: revenue=8000, netProfit=-500 → cost=8500 → ROIPct≈-5.88%

	c := inventory.Campaign{ID: "c1", Name: "Split", Phase: inventory.PhaseActive}
	p1 := inventory.Purchase{CampaignID: "c1", BuyCostCents: 10000, PurchaseDate: "2026-06-01"}
	p2 := inventory.Purchase{CampaignID: "c1", BuyCostCents: 8500, PurchaseDate: "2026-06-02"}

	s1 := &inventory.Sale{
		SalePriceCents:    12000,
		NetProfitCents:    2000,
		SaleDate:          "2026-06-10",
		ForcedLiquidation: false,
	}
	s2 := &inventory.Sale{
		SalePriceCents:    8000,
		NetProfitCents:    -500,
		SaleDate:          "2026-06-11",
		ForcedLiquidation: true,
	}

	rows := []inventory.PurchaseWithSale{{Purchase: p1, Sale: s1}, {Purchase: p2, Sale: s2}}
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	result := ComputeAnalysis([]inventory.Campaign{c}, rows, nil, "", now)

	if len(result.Campaigns) != 1 {
		t.Fatalf("expected 1 campaign, got %d", len(result.Campaigns))
	}
	pnl := result.Campaigns[0].PNL

	disc := pnl.Discretionary
	if disc.SoldCount != 1 {
		t.Errorf("disc.SoldCount=%d, want 1", disc.SoldCount)
	}
	if disc.RevenueCents != 12000 {
		t.Errorf("disc.RevenueCents=%d, want 12000", disc.RevenueCents)
	}
	if disc.NetProfitCents != 2000 {
		t.Errorf("disc.NetProfitCents=%d, want 2000", disc.NetProfitCents)
	}
	approx(t, "disc.ROIPct", disc.ROIPct, 20.0, 0.01)

	forced := pnl.Forced
	if forced.SoldCount != 1 {
		t.Errorf("forced.SoldCount=%d, want 1", forced.SoldCount)
	}
	if forced.RevenueCents != 8000 {
		t.Errorf("forced.RevenueCents=%d, want 8000", forced.RevenueCents)
	}
	if forced.NetProfitCents != -500 {
		t.Errorf("forced.NetProfitCents=%d, want -500", forced.NetProfitCents)
	}
}

// --- TestComputeAnalysisWeeklyFill ---

func TestComputeAnalysisWeeklyFill(t *testing.T) {
	// now = 2026-07-06 (Monday). DailySpendCapCents=100000 → CapCents=700000/wk.
	// Two purchases on 2026-07-06 (same Monday week) → Fills=2, SpendCents=sum.
	// UtilizationPct = spendCents/700000*100.

	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	c := inventory.Campaign{
		ID:                 "c1",
		Name:               "Fill",
		Phase:              inventory.PhaseActive,
		DailySpendCapCents: 100000,
	}
	p1 := inventory.Purchase{CampaignID: "c1", BuyCostCents: 50000, PurchaseDate: "2026-07-06"}
	p2 := inventory.Purchase{CampaignID: "c1", BuyCostCents: 30000, PurchaseDate: "2026-07-06"}
	// old purchase outside trailing 8 weeks — should not appear
	p3 := inventory.Purchase{CampaignID: "c1", BuyCostCents: 20000, PurchaseDate: "2025-01-01"}

	rows := []inventory.PurchaseWithSale{{Purchase: p1}, {Purchase: p2}, {Purchase: p3}}
	result := ComputeAnalysis([]inventory.Campaign{c}, rows, nil, "", now)

	if len(result.Campaigns) != 1 {
		t.Fatalf("expected 1 campaign, got %d", len(result.Campaigns))
	}
	wf := result.Campaigns[0].WeeklyFill

	// Should have exactly 8 week buckets.
	if len(wf) != 8 {
		t.Fatalf("WeeklyFill len=%d, want 8", len(wf))
	}

	// Find the bucket for 2026-07-06.
	var found *WeeklyFill
	for i := range wf {
		if wf[i].WeekStart == "2026-07-06" {
			found = &wf[i]
		}
	}
	if found == nil {
		t.Fatalf("no WeeklyFill bucket for 2026-07-06; buckets: %v", wf)
	}
	if found.Fills != 2 {
		t.Errorf("Fills=%d, want 2", found.Fills)
	}
	if found.SpendCents != 80000 {
		t.Errorf("SpendCents=%d, want 80000", found.SpendCents)
	}
	if found.CapCents != 700000 {
		t.Errorf("CapCents=%d, want 700000", found.CapCents)
	}
	wantUtil := 80000.0 / 700000.0 * 100
	approx(t, "UtilizationPct", found.UtilizationPct, wantUtil, 0.01)
}

// --- TestComputeAnalysisScopeFilter ---

func TestComputeAnalysisScopeFilter(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name        string
		campaign    inventory.Campaign
		purchase    inventory.Purchase
		wantInScope bool
	}{
		{
			name:        "grade below range excluded",
			campaign:    inventory.Campaign{ID: "c1", GradeRange: "9-10"},
			purchase:    inventory.Purchase{CampaignID: "c1", GradeValue: 8, BuyCostCents: 10000, PurchaseDate: "2026-07-06"},
			wantInScope: false,
		},
		{
			name:        "grade at bottom of range included",
			campaign:    inventory.Campaign{ID: "c1", GradeRange: "9-10"},
			purchase:    inventory.Purchase{CampaignID: "c1", GradeValue: 9, BuyCostCents: 10000, PurchaseDate: "2026-07-06"},
			wantInScope: true,
		},
		{
			name:        "price below range excluded ($40 card, range 50-500)",
			campaign:    inventory.Campaign{ID: "c1", PriceRange: "50-500"},
			purchase:    inventory.Purchase{CampaignID: "c1", GradeValue: 9, BuyCostCents: 4000, PurchaseDate: "2026-07-06"},
			wantInScope: false,
		},
		{
			name:        "price in range included ($100 card, range 50-500)",
			campaign:    inventory.Campaign{ID: "c1", PriceRange: "50-500"},
			purchase:    inventory.Purchase{CampaignID: "c1", GradeValue: 9, BuyCostCents: 10000, PurchaseDate: "2026-07-06"},
			wantInScope: true,
		},
		{
			name:        "inclusion list excludes pikachu (ExclusionMode=false)",
			campaign:    inventory.Campaign{ID: "c1", InclusionList: "charizard,blastoise", ExclusionMode: false},
			purchase:    inventory.Purchase{CampaignID: "c1", GradeValue: 9, BuyCostCents: 10000, CardPlayer: "Pikachu", PurchaseDate: "2026-07-06"},
			wantInScope: false,
		},
		{
			name:        "inclusion list includes charizard (ExclusionMode=false, case-insensitive)",
			campaign:    inventory.Campaign{ID: "c1", InclusionList: "charizard,blastoise", ExclusionMode: false},
			purchase:    inventory.Purchase{CampaignID: "c1", GradeValue: 9, BuyCostCents: 10000, CardPlayer: "Charizard", PurchaseDate: "2026-07-06"},
			wantInScope: true,
		},
		{
			name:        "exclusion mode excludes charizard",
			campaign:    inventory.Campaign{ID: "c1", InclusionList: "charizard,blastoise", ExclusionMode: true},
			purchase:    inventory.Purchase{CampaignID: "c1", GradeValue: 9, BuyCostCents: 10000, CardPlayer: "Charizard", PurchaseDate: "2026-07-06"},
			wantInScope: false,
		},
		{
			name:        "exclusion mode includes pikachu (not in exclusion list)",
			campaign:    inventory.Campaign{ID: "c1", InclusionList: "charizard,blastoise", ExclusionMode: true},
			purchase:    inventory.Purchase{CampaignID: "c1", GradeValue: 9, BuyCostCents: 10000, CardPlayer: "Pikachu", PurchaseDate: "2026-07-06"},
			wantInScope: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := ComputeAnalysis(
				[]inventory.Campaign{tc.campaign},
				[]inventory.PurchaseWithSale{{Purchase: tc.purchase}},
				nil, "", now,
			)
			if len(result.Campaigns) != 1 {
				t.Fatalf("expected 1 campaign, got %d", len(result.Campaigns))
			}
			scope := result.Campaigns[0].InScopeByGrade
			inScope := len(scope) > 0 && scope[0].N > 0
			if inScope != tc.wantInScope {
				t.Errorf("inScope=%v, want %v (scope rows: %v)", inScope, tc.wantInScope, scope)
			}
		})
	}
}

// --- TestComputeAnalysisDeltas ---

func TestComputeAnalysisDeltas(t *testing.T) {
	// since="2026-06-25": purchase 6/26 counted, 6/20 not; sale 6/27 counted;
	// campaign UpdatedAt 6/28 listed in CampaignsUpdated; invoice 6/30 summarized.

	since := "2026-06-25"
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)

	campaigns := []inventory.Campaign{
		{
			ID:        "c1",
			Name:      "Alpha",
			Phase:     inventory.PhaseActive,
			UpdatedAt: time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC), // after since → listed
		},
		{
			ID:        "c2",
			Name:      "Beta",
			Phase:     inventory.PhaseActive,
			UpdatedAt: time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC), // before since → not listed
		},
	}

	rows := []inventory.PurchaseWithSale{
		{
			Purchase: inventory.Purchase{CampaignID: "c1", BuyCostCents: 10000, PurchaseDate: "2026-06-26"},
			Sale: &inventory.Sale{SalePriceCents: 12000, NetProfitCents: 2000, SaleDate: "2026-06-27"},
		},
		{
			Purchase: inventory.Purchase{CampaignID: "c1", BuyCostCents: 5000, PurchaseDate: "2026-06-20"}, // before since
		},
	}

	invoices := []inventory.Invoice{
		{InvoiceDate: "2026-06-30", DueDate: "2026-07-01", TotalCents: 50000, Status: "unpaid"},
		{InvoiceDate: "2026-06-10", DueDate: "2026-06-11", TotalCents: 30000, Status: "paid"}, // before since
	}

	result := ComputeAnalysis(campaigns, rows, invoices, since, now)
	d := result.Deltas

	if d.NewPurchases != 1 {
		t.Errorf("NewPurchases=%d, want 1", d.NewPurchases)
	}
	if d.NewPurchaseCents != 10000 {
		t.Errorf("NewPurchaseCents=%d, want 10000", d.NewPurchaseCents)
	}
	if d.NewSales != 1 {
		t.Errorf("NewSales=%d, want 1", d.NewSales)
	}
	if d.NewSaleCents != 12000 {
		t.Errorf("NewSaleCents=%d, want 12000", d.NewSaleCents)
	}

	if len(d.CampaignsUpdated) != 1 || d.CampaignsUpdated[0] != "Alpha" {
		t.Errorf("CampaignsUpdated=%v, want [Alpha]", d.CampaignsUpdated)
	}

	if len(d.Invoices) != 1 {
		t.Errorf("Invoices len=%d, want 1", len(d.Invoices))
	} else if d.Invoices[0].InvoiceDate != "2026-06-30" {
		t.Errorf("Invoices[0].InvoiceDate=%s, want 2026-06-30", d.Invoices[0].InvoiceDate)
	}
}
