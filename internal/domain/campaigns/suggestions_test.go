package campaigns

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestGenerateSuggestions_Empty(t *testing.T) {
	insights := &PortfolioInsights{
		DataSummary: InsightsDataSummary{TotalPurchases: 0},
	}
	resp := GenerateSuggestions(context.Background(),insights, nil)
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if len(resp.NewCampaigns) != 0 {
		t.Errorf("expected 0 new campaigns, got %d", len(resp.NewCampaigns))
	}
	if len(resp.Adjustments) != 0 {
		t.Errorf("expected 0 adjustments, got %d", len(resp.Adjustments))
	}
}

func TestGenerateSuggestions_TopCharacterExpansion(t *testing.T) {
	insights := &PortfolioInsights{
		ByCharacter: []SegmentPerformance{
			{Label: "Charizard", ROI: 0.25, SoldCount: 10, CampaignCount: 1, PurchaseCount: 15, BestChannel: SaleChannelEbay, AvgMarginPct: 0.20, AvgDaysToSell: 14},
		},
		DataSummary: InsightsDataSummary{TotalPurchases: 50},
	}
	campaigns := []Campaign{
		{Name: "Campaign A", Phase: PhaseActive, InclusionList: "Pikachu, Blastoise"},
	}

	resp := GenerateSuggestions(context.Background(),insights, campaigns)

	found := false
	for _, s := range resp.NewCampaigns {
		if s.SuggestedParams.InclusionList == "Charizard" {
			found = true
			if s.Confidence != "medium" {
				t.Errorf("expected medium confidence for 10 sales, got %s", s.Confidence)
			}
			if math.Abs(s.ExpectedMetrics.ExpectedROI-0.25) > 1e-6 {
				t.Errorf("expected ROI ~0.25, got %f", s.ExpectedMetrics.ExpectedROI)
			}
		}
	}
	if !found {
		t.Error("expected Charizard expansion suggestion")
	}
}

func TestGenerateSuggestions_CharacterAdjustments(t *testing.T) {
	insights := &PortfolioInsights{
		ByCharacter: []SegmentPerformance{
			{Label: "Charizard", ROI: 0.30, SoldCount: 10, PurchaseCount: 12, CampaignCount: 2},
			{Label: "Pikachu", ROI: -0.10, SoldCount: 8, PurchaseCount: 15, CampaignCount: 1},
			{Label: "Blastoise", ROI: 0.20, SoldCount: 6, PurchaseCount: 8, CampaignCount: 1},
		},
		DataSummary: InsightsDataSummary{TotalPurchases: 35},
	}
	campaigns := []Campaign{
		{Name: "Test Campaign", Phase: PhaseActive, InclusionList: "Pikachu, Blastoise"},
	}

	resp := GenerateSuggestions(context.Background(),insights, campaigns)

	// Should suggest removing Pikachu (negative ROI)
	foundRemove := false
	foundAdd := false
	for _, s := range resp.Adjustments {
		if s.Type == "adjust" {
			if s.Title == "Remove underperformers from Test Campaign" {
				foundRemove = true
			}
			if s.Title == "Add top performers to Test Campaign" {
				foundAdd = true
			}
		}
	}
	if !foundRemove {
		t.Error("expected adjustment to remove underperforming Pikachu")
	}
	if !foundAdd {
		t.Error("expected adjustment to add top performer Charizard")
	}
}

func TestGenerateSuggestions_CoverageGap(t *testing.T) {
	insights := &PortfolioInsights{
		ByCharacter: []SegmentPerformance{
			{Label: "Gengar", ROI: 0.30, SoldCount: 6, PurchaseCount: 10, CampaignCount: 1, Dimension: "character", BestChannel: SaleChannelLocal},
		},
		CoverageGaps: []CoverageGap{
			{
				Segment: SegmentPerformance{Label: "Gengar", ROI: 0.30, SoldCount: 6, PurchaseCount: 10, Dimension: "character", BestChannel: SaleChannelLocal},
				Reason:  "Gengar has 30% ROI but not in any active campaign",
			},
		},
		DataSummary: InsightsDataSummary{TotalPurchases: 50},
	}

	resp := GenerateSuggestions(context.Background(),insights, nil)

	found := false
	for _, s := range resp.NewCampaigns {
		if s.Type == "gap" && s.SuggestedParams.InclusionList == "Gengar" {
			found = true
		}
	}
	if !found {
		t.Error("expected coverage gap suggestion for Gengar")
	}
}

func TestGenerateSuggestions_ChannelInformedBuyTerms(t *testing.T) {
	insights := &PortfolioInsights{
		ByChannel: []ChannelPNL{
			{Channel: SaleChannelEbay, SaleCount: 20, RevenueCents: 100000, FeesCents: 12350, NetProfitCents: 30000},
			{Channel: SaleChannelLocal, SaleCount: 5, RevenueCents: 30000, FeesCents: 0, NetProfitCents: 15000},
		},
		DataSummary: InsightsDataSummary{TotalPurchases: 50, TotalSales: 25},
	}
	campaigns := []Campaign{
		{Name: "Aggressive Campaign", Phase: PhaseActive, BuyTermsCLPct: 0.85, EbayFeePct: 0.1235},
	}

	resp := GenerateSuggestions(context.Background(),insights, campaigns)

	// Local channel has 50% margin (15000/30000), eBay has 30%
	// Best channel margin = 50%, maxBuy = 50% - 10% - 12.35% = 27.65%
	// Campaign buyTerms at 85% is way above, should suggest lowering
	found := false
	for _, s := range resp.Adjustments {
		if s.Type == "adjust" && s.SuggestedParams.Name == "Aggressive Campaign" && s.SuggestedParams.BuyTermsCLPct > 0 {
			found = true
		}
	}
	if !found {
		t.Error("expected channel-informed buy terms suggestion for aggressive campaign")
	}
}

func TestGenerateSuggestions_SpendCapRebalancing(t *testing.T) {
	insights := &PortfolioInsights{
		DataSummary: InsightsDataSummary{TotalPurchases: 50, OverallROI: 0.20},
	}
	campaigns := []Campaign{
		{Name: "Low Cap", Phase: PhaseActive, DailySpendCapCents: 5000},   // $50/day
		{Name: "High Cap", Phase: PhaseActive, DailySpendCapCents: 50000}, // $500/day
	}

	resp := GenerateSuggestions(context.Background(),insights, campaigns)

	// Caps differ by 10x, should suggest rebalancing
	found := false
	for _, s := range resp.Adjustments {
		if s.Title == "Rebalance spend caps" {
			found = true
			if s.SuggestedParams.DailySpendCapCents != 27500 {
				t.Errorf("expected avg cap 27500, got %d", s.SuggestedParams.DailySpendCapCents)
			}
		}
	}
	if !found {
		t.Error("expected spend cap rebalancing suggestion")
	}
}

func TestGenerateSuggestions_GradeSweetSpot(t *testing.T) {
	insights := &PortfolioInsights{
		ByGrade: []SegmentPerformance{
			{Label: "PSA 9", ROI: 0.30, SoldCount: 15, PurchaseCount: 20, CampaignCount: 1, AvgMarginPct: 0.25, AvgDaysToSell: 10},
			{Label: "PSA 10", ROI: 0.05, SoldCount: 10, PurchaseCount: 15, CampaignCount: 2},
		},
		DataSummary: InsightsDataSummary{TotalPurchases: 35},
	}
	campaigns := []Campaign{
		{Phase: PhaseActive, GradeRange: "9-10"},
		{Phase: PhaseActive, GradeRange: "8-10"},
	}

	resp := GenerateSuggestions(context.Background(),insights, campaigns)

	found := false
	for _, s := range resp.NewCampaigns {
		if s.Title == "PSA 9 Sweet Spot Campaign" {
			found = true
			if math.Abs(s.ExpectedMetrics.ExpectedROI-0.30) > 1e-6 {
				t.Errorf("expected ROI ~0.30, got %f", s.ExpectedMetrics.ExpectedROI)
			}
		}
	}
	if !found {
		t.Error("expected PSA 9 sweet spot suggestion")
	}
}

func TestDeduplicateSuggestions(t *testing.T) {
	suggestions := []CampaignSuggestion{
		{
			Type:       "adjust",
			Title:      "Lower buy terms on Campaign A",
			Confidence: "high",
			DataPoints: 30,
			SuggestedParams: CampaignSuggestionParams{
				Name:          "Campaign A",
				BuyTermsCLPct: 0.60,
			},
		},
		{
			Type:       "adjust",
			Title:      "Lower buy terms on Campaign A (dup)",
			Confidence: "medium",
			DataPoints: 10,
			SuggestedParams: CampaignSuggestionParams{
				Name:          "Campaign A",
				BuyTermsCLPct: 0.55,
			},
		},
		{
			Type:       "adjust",
			Title:      "Add top performers to Campaign A",
			Confidence: "medium",
			DataPoints: 20,
			SuggestedParams: CampaignSuggestionParams{
				Name:          "Campaign A",
				InclusionList: "add: Charizard",
			},
		},
	}

	result := deduplicateSuggestions(suggestions)

	if len(result) != 2 {
		t.Fatalf("expected 2 suggestions after dedup, got %d", len(result))
	}

	// The high-confidence buy terms suggestion should survive
	for _, s := range result {
		if s.SuggestedParams.BuyTermsCLPct > 0 && s.Confidence != "high" {
			t.Error("expected high-confidence buy terms suggestion to survive dedup")
		}
	}
}

func TestGameStopPayoutRange(t *testing.T) {
	insights := &PortfolioInsights{
		ByChannel: []ChannelPNL{
			{Channel: SaleChannelGameStop, SaleCount: 15, RevenueCents: 80000, FeesCents: 0, NetProfitCents: 40000},
			{Channel: SaleChannelEbay, SaleCount: 10, RevenueCents: 50000, FeesCents: 6175, NetProfitCents: 10000},
		},
		DataSummary: InsightsDataSummary{TotalPurchases: 50, TotalSales: 25},
	}
	campaigns := []Campaign{
		{Name: "GS Campaign", Phase: PhaseActive, BuyTermsCLPct: 0.80, EbayFeePct: 0.1235},
	}

	resp := GenerateSuggestions(context.Background(),insights, campaigns)

	found := false
	for _, s := range resp.Adjustments {
		if s.SuggestedParams.Name == "GS Campaign" && s.SuggestedParams.BuyTermsCLPct > 0 {
			found = true
			const eps = 1e-9
			// Conservative: 70% - 10% = 60%
			if math.Abs(s.SuggestedParams.BuyTermsCLPct-0.60) > eps {
				t.Errorf("expected conservative maxBuy 0.60, got %f", s.SuggestedParams.BuyTermsCLPct)
			}
			// Optimistic: 90% - 10% = 80%
			if math.Abs(s.SuggestedParams.BuyTermsCLPctOptimistic-0.80) > eps {
				t.Errorf("expected optimistic maxBuy 0.80, got %f", s.SuggestedParams.BuyTermsCLPctOptimistic)
			}
		}
	}
	if !found {
		t.Error("expected GameStop payout range suggestion")
	}
}

func TestROIWeightedSpendCaps(t *testing.T) {
	insights := &PortfolioInsights{
		DataSummary: InsightsDataSummary{TotalPurchases: 50, OverallROI: 0.20},
		CampaignMetrics: []CampaignPNLBrief{
			{CampaignID: "c1", ROI: 0.40, SpendCents: 100000, ProfitCents: 40000, SoldCount: 15, PurchaseCount: 20},
			{CampaignID: "c2", ROI: 0.05, SpendCents: 200000, ProfitCents: 10000, SoldCount: 10, PurchaseCount: 30},
		},
	}
	campaigns := []Campaign{
		{ID: "c1", Name: "High ROI", Phase: PhaseActive, DailySpendCapCents: 5000},
		{ID: "c2", Name: "Low ROI", Phase: PhaseActive, DailySpendCapCents: 50000},
	}

	resp := GenerateSuggestions(context.Background(),insights, campaigns)

	found := false
	for _, s := range resp.Adjustments {
		if s.Title == "Rebalance spend caps by ROI" {
			found = true
			// Should mention raising High ROI and lowering Low ROI
			if !containsAll(s.Rationale, "raise", "High ROI", "lower", "Low ROI") {
				t.Errorf("expected ROI-weighted rationale, got: %s", s.Rationale)
			}
		}
	}
	if !found {
		t.Error("expected ROI-weighted spend cap suggestion")
	}
}

func TestPhaseTransition_ArchiveUnderperformer(t *testing.T) {
	insights := &PortfolioInsights{
		DataSummary: InsightsDataSummary{TotalPurchases: 50, OverallROI: 0.10},
		CampaignMetrics: []CampaignPNLBrief{
			{CampaignID: "c1", ROI: -0.20, SpendCents: 100000, ProfitCents: -20000, SoldCount: 5, PurchaseCount: 25},
		},
	}
	campaigns := []Campaign{
		{ID: "c1", Name: "Losing Campaign", Phase: PhaseActive},
	}

	resp := GenerateSuggestions(context.Background(),insights, campaigns)

	found := false
	for _, s := range resp.Adjustments {
		if s.Title == "Consider closing Losing Campaign" {
			found = true
		}
	}
	if !found {
		t.Error("expected phase transition suggestion to close underperformer")
	}
}

func TestPhaseTransition_ActivatePending(t *testing.T) {
	insights := &PortfolioInsights{
		ByCharacter: []SegmentPerformance{
			{Label: "Charizard", ROI: 0.25, SoldCount: 10, PurchaseCount: 15, Dimension: "character"},
		},
		DataSummary: InsightsDataSummary{TotalPurchases: 50},
		CampaignMetrics: []CampaignPNLBrief{
			{CampaignID: "c1", PurchaseCount: 0},
		},
	}
	campaigns := []Campaign{
		{ID: "c1", Name: "Pending Charizard", Phase: PhasePending, InclusionList: "Charizard"},
	}

	resp := GenerateSuggestions(context.Background(),insights, campaigns)

	found := false
	for _, s := range resp.Adjustments {
		if s.Title == "Activate Pending Charizard" {
			found = true
		}
	}
	if !found {
		t.Error("expected phase transition suggestion to activate pending campaign")
	}
}

func containsAll(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
