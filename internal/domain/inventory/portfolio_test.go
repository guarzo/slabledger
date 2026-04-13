package inventory

import (
	"testing"

	"github.com/guarzo/slabledger/internal/domain/mathutil"
)

func TestExtractCharacter(t *testing.T) {
	campaigns := []Campaign{
		{InclusionList: "Charizard, Pikachu, Blastoise"},
	}

	tests := []struct {
		cardName string
		want     string
	}{
		{"2021 Pokemon Celebrations Charizard PSA 9", "Charizard"},
		{"Pikachu VMAX Rainbow", "Pikachu"},
		{"Some Unknown Card", "Other"},
		{"Mewtwo GX Full Art", "Mewtwo"},
		{"blastoise base set", "Blastoise"},
	}

	for _, tc := range tests {
		t.Run(tc.cardName, func(t *testing.T) {
			got := ExtractCharacter(tc.cardName, campaigns)
			if got != tc.want {
				t.Errorf("ExtractCharacter(%q) = %q, want %q", tc.cardName, got, tc.want)
			}
		})
	}
}

func TestExtractCharacter_MewtwoVsMew(t *testing.T) {
	campaigns := []Campaign{}
	// Mewtwo should match before Mew (longest match first)
	got := ExtractCharacter("Mewtwo EX Full Art", campaigns)
	if got != "Mewtwo" {
		t.Errorf("ExtractCharacter(Mewtwo EX) = %q, want Mewtwo", got)
	}

	got = ExtractCharacter("Mew Base Set Holo", campaigns)
	if got != "Mew" {
		t.Errorf("ExtractCharacter(Mew Base) = %q, want Mew", got)
	}
}

func TestClassifyEra(t *testing.T) {
	tests := []struct {
		date string
		want string
	}{
		{"2000-05-01", "vintage"},
		{"2003-12-31", "vintage"},
		{"2004-01-01", "ex_era"},
		{"2007-06-15", "ex_era"},
		{"2008-01-01", "mid_era"},
		{"2015-12-31", "mid_era"},
		{"2016-01-01", "modern"},
		{"2026-03-01", "modern"},
		{"abc", "unknown"},
		{"", "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.date, func(t *testing.T) {
			got := ClassifyEra(tc.date)
			if got != tc.want {
				t.Errorf("ClassifyEra(%q) = %q, want %q", tc.date, got, tc.want)
			}
		})
	}
}

func TestClassifyPriceTier(t *testing.T) {
	tests := []struct {
		cents int
		want  string
	}{
		{2500, "$0-50"},
		{5000, "$50-100"},
		{15000, "$100-200"},
		{30000, "$200-500"},
		{60000, "$500+"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := ClassifyPriceTier(tc.cents)
			if got != tc.want {
				t.Errorf("ClassifyPriceTier(%d) = %q, want %q", tc.cents, got, tc.want)
			}
		})
	}
}

func Test_ComputeSegmentPerformance(t *testing.T) {
	data := []PurchaseWithSale{
		{
			Purchase: Purchase{CardName: "Charizard Base", GradeValue: 9, BuyCostCents: 8000, PSASourcingFeeCents: 300, CLValueCents: 10000, CampaignID: "c1", PurchaseDate: "2026-01-01"},
			Sale:     &Sale{SalePriceCents: 12000, SaleFeeCents: 1200, NetProfitCents: 2500, DaysToSell: 10, SaleChannel: SaleChannelEbay},
		},
		{
			Purchase: Purchase{CardName: "Charizard Celebrations", GradeValue: 10, BuyCostCents: 5000, PSASourcingFeeCents: 300, CLValueCents: 7000, CampaignID: "c1", PurchaseDate: "2026-01-05"},
			Sale:     &Sale{SalePriceCents: 8000, SaleFeeCents: 800, NetProfitCents: 1900, DaysToSell: 7, SaleChannel: SaleChannelEbay},
		},
		{
			Purchase: Purchase{CardName: "Pikachu VMAX", GradeValue: 10, BuyCostCents: 3000, PSASourcingFeeCents: 300, CLValueCents: 4000, CampaignID: "c2", PurchaseDate: "2026-02-01"},
			Sale:     nil,
		},
	}

	// By character
	result := ComputeSegmentPerformance(data, "character", func(d PurchaseWithSale) string {
		return ExtractCharacter(d.Purchase.CardName, nil)
	})

	if len(result) < 2 {
		t.Fatalf("expected at least 2 segments, got %d", len(result))
	}

	// Charizard should be first (highest profit)
	if result[0].Label != "Charizard" {
		t.Errorf("expected first segment to be Charizard, got %s", result[0].Label)
	}
	if result[0].PurchaseCount != 2 {
		t.Errorf("Charizard PurchaseCount = %d, want 2", result[0].PurchaseCount)
	}
	if result[0].SoldCount != 2 {
		t.Errorf("Charizard SoldCount = %d, want 2", result[0].SoldCount)
	}
	if result[0].NetProfitCents != 4400 {
		t.Errorf("Charizard NetProfitCents = %d, want 4400", result[0].NetProfitCents)
	}
}

func Test_ComputePortfolioInsights(t *testing.T) {
	data := []PurchaseWithSale{
		{
			Purchase: Purchase{CardName: "Charizard Base", GradeValue: 9, BuyCostCents: 8000, PSASourcingFeeCents: 300, CLValueCents: 10000, CampaignID: "c1", PurchaseDate: "2026-01-01"},
			Sale:     &Sale{SalePriceCents: 12000, SaleFeeCents: 1200, NetProfitCents: 2500, DaysToSell: 10, SaleChannel: SaleChannelEbay},
		},
		{
			Purchase: Purchase{CardName: "Pikachu VMAX", GradeValue: 10, BuyCostCents: 3000, PSASourcingFeeCents: 300, CLValueCents: 4000, CampaignID: "c2", PurchaseDate: "2026-02-01"},
		},
	}

	campaigns := []Campaign{
		{ID: "c1", Phase: PhaseActive, InclusionList: "Charizard"},
		{ID: "c2", Phase: PhaseActive, InclusionList: "Pikachu"},
	}

	channelPNL := []ChannelPNL{
		{Channel: SaleChannelEbay, SaleCount: 1, RevenueCents: 12000, NetProfitCents: 2500},
	}

	insights := ComputePortfolioInsights(data, channelPNL, campaigns)

	if insights.DataSummary.TotalPurchases != 2 {
		t.Errorf("TotalPurchases = %d, want 2", insights.DataSummary.TotalPurchases)
	}
	if insights.DataSummary.TotalSales != 1 {
		t.Errorf("TotalSales = %d, want 1", insights.DataSummary.TotalSales)
	}
	if insights.DataSummary.CampaignsAnalyzed != 2 {
		t.Errorf("CampaignsAnalyzed = %d, want 2", insights.DataSummary.CampaignsAnalyzed)
	}
	if len(insights.ByCharacter) < 2 {
		t.Errorf("expected at least 2 characters, got %d", len(insights.ByCharacter))
	}
	if len(insights.ByGrade) < 1 {
		t.Errorf("expected at least 1 grade, got %d", len(insights.ByGrade))
	}
	if len(insights.ByEra) < 1 {
		t.Errorf("expected at least 1 era, got %d", len(insights.ByEra))
	}
	if len(insights.ByPriceTier) < 1 {
		t.Errorf("expected at least 1 price tier, got %d", len(insights.ByPriceTier))
	}
	if len(insights.ByChannel) != 1 {
		t.Errorf("expected 1 channel, got %d", len(insights.ByChannel))
	}
}

func Test_ComputePortfolioInsights_Empty(t *testing.T) {
	insights := ComputePortfolioInsights(nil, nil, nil)
	if insights.DataSummary.TotalPurchases != 0 {
		t.Errorf("expected 0 purchases, got %d", insights.DataSummary.TotalPurchases)
	}
}

func TestDetectCoverageGaps(t *testing.T) {
	byCharacter := []SegmentPerformance{
		{Label: "Charizard", ROI: 0.20, SoldCount: 5, CampaignCount: 1, Dimension: "character"},
		{Label: "Gengar", ROI: 0.25, SoldCount: 8, CampaignCount: 1, Dimension: "character"},
	}
	byGrade := []SegmentPerformance{
		{Label: "PSA 9", ROI: 0.18, SoldCount: 10, CampaignCount: 1, Dimension: "grade"},
	}

	campaigns := []Campaign{
		{Phase: PhaseActive, InclusionList: "Charizard, Pikachu"},
	}

	gaps := DetectCoverageGaps(byCharacter, byGrade, campaigns)

	// Gengar is profitable but not in any campaign's inclusion list
	found := false
	for _, g := range gaps {
		if g.Segment.Label == "Gengar" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Gengar to be identified as a coverage gap")
	}
}

func TestConfidenceLabel(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{3, "low"},
		{5, "medium"},
		{19, "medium"},
		{20, "high"},
		{100, "high"},
	}
	for _, tc := range tests {
		got := mathutil.ConfidenceLabel(tc.n)
		if got != tc.want {
			t.Errorf("confidenceLabel(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestConfidenceLabelWithAge(t *testing.T) {
	tests := []struct {
		name           string
		n              int
		latestSaleDate string
		now            string
		want           string
	}{
		{"high stays high with recent data", 25, "2026-02-01", "2026-03-01", "high"},
		{"high decays to medium with old data", 25, "2025-06-01", "2026-03-01", "medium"},
		{"medium decays to low with old data", 10, "2025-06-01", "2026-03-01", "low"},
		{"low stays low with old data", 3, "2025-06-01", "2026-03-01", "low"},
		{"empty date uses base confidence", 25, "", "2026-03-01", "high"},
		{"exactly 6 months stays same", 25, "2025-09-01", "2026-03-01", "high"},
		{"7 months decays", 25, "2025-08-01", "2026-03-01", "medium"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := confidenceLabelWithAge(tc.n, tc.latestSaleDate, tc.now)
			if got != tc.want {
				t.Errorf("confidenceLabelWithAge(%d, %q, %q) = %q, want %q",
					tc.n, tc.latestSaleDate, tc.now, got, tc.want)
			}
		})
	}
}

func TestComputeCampaignMetrics(t *testing.T) {
	data := []PurchaseWithSale{
		{
			Purchase: Purchase{CampaignID: "c1", BuyCostCents: 8000, PSASourcingFeeCents: 300},
			Sale:     &Sale{NetProfitCents: 2500},
		},
		{
			Purchase: Purchase{CampaignID: "c1", BuyCostCents: 5000, PSASourcingFeeCents: 300},
			Sale:     &Sale{NetProfitCents: 1000},
		},
		{
			Purchase: Purchase{CampaignID: "c2", BuyCostCents: 3000, PSASourcingFeeCents: 300},
		},
	}

	metrics := computeCampaignMetrics(data)
	if len(metrics) != 2 {
		t.Fatalf("expected 2 campaign metrics, got %d", len(metrics))
	}

	metricsMap := make(map[string]CampaignPNLBrief)
	for _, m := range metrics {
		metricsMap[m.CampaignID] = m
	}

	c1 := metricsMap["c1"]
	if c1.PurchaseCount != 2 {
		t.Errorf("c1 PurchaseCount = %d, want 2", c1.PurchaseCount)
	}
	if c1.SoldCount != 2 {
		t.Errorf("c1 SoldCount = %d, want 2", c1.SoldCount)
	}
	if c1.SpendCents != 13600 {
		t.Errorf("c1 SpendCents = %d, want 13600", c1.SpendCents)
	}
	if c1.ProfitCents != 3500 {
		t.Errorf("c1 ProfitCents = %d, want 3500", c1.ProfitCents)
	}

	c2 := metricsMap["c2"]
	if c2.PurchaseCount != 1 {
		t.Errorf("c2 PurchaseCount = %d, want 1", c2.PurchaseCount)
	}
	if c2.SoldCount != 0 {
		t.Errorf("c2 SoldCount = %d, want 0", c2.SoldCount)
	}
}

func TestSegmentPerformanceLatestSaleDate(t *testing.T) {
	data := []PurchaseWithSale{
		{
			Purchase: Purchase{CardName: "Charizard Base", CampaignID: "c1", BuyCostCents: 8000, PSASourcingFeeCents: 300, CLValueCents: 10000},
			Sale:     &Sale{SalePriceCents: 12000, SaleFeeCents: 1200, NetProfitCents: 2500, DaysToSell: 10, SaleChannel: SaleChannelEbay, SaleDate: "2026-01-15"},
		},
		{
			Purchase: Purchase{CardName: "Charizard Celebrations", CampaignID: "c1", BuyCostCents: 5000, PSASourcingFeeCents: 300, CLValueCents: 7000},
			Sale:     &Sale{SalePriceCents: 8000, SaleFeeCents: 800, NetProfitCents: 1900, DaysToSell: 7, SaleChannel: SaleChannelEbay, SaleDate: "2026-02-20"},
		},
	}

	result := ComputeSegmentPerformance(data, "character", func(d PurchaseWithSale) string {
		return ExtractCharacter(d.Purchase.CardName, nil)
	})

	for _, seg := range result {
		if seg.Label == "Charizard" {
			if seg.LatestSaleDate != "2026-02-20" {
				t.Errorf("expected LatestSaleDate 2026-02-20, got %s", seg.LatestSaleDate)
			}
			return
		}
	}
	t.Error("Charizard segment not found")
}
