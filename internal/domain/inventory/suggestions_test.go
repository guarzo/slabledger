package inventory

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
	resp := GenerateSuggestions(context.Background(), insights, nil, nil)
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

	resp := GenerateSuggestions(context.Background(), insights, campaigns, nil)

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
			{Label: "Charizard", ROI: 0.30, SoldCount: 10, PurchaseCount: 12, CampaignCount: 2, AvgMarginPct: 0.22, AvgDaysToSell: 18},
			{Label: "Pikachu", ROI: -0.10, SoldCount: 8, PurchaseCount: 15, CampaignCount: 1},
			{Label: "Blastoise", ROI: 0.20, SoldCount: 6, PurchaseCount: 8, CampaignCount: 1},
		},
		DataSummary: InsightsDataSummary{TotalPurchases: 35},
	}
	campaigns := []Campaign{
		{Name: "Test Campaign", Phase: PhaseActive, InclusionList: "Pikachu, Blastoise"},
	}

	resp := GenerateSuggestions(context.Background(), insights, campaigns, nil)

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
				// ExpectedMetrics should be propagated from Charizard (top-ROI add).
				if math.Abs(s.ExpectedMetrics.ExpectedROI-0.30) > 1e-6 {
					t.Errorf("expected ExpectedROI ~0.30 from Charizard, got %f", s.ExpectedMetrics.ExpectedROI)
				}
				if math.Abs(s.ExpectedMetrics.ExpectedMarginPct-0.22) > 1e-6 {
					t.Errorf("expected ExpectedMarginPct ~0.22 from Charizard, got %f", s.ExpectedMetrics.ExpectedMarginPct)
				}
				if math.Abs(s.ExpectedMetrics.AvgDaysToSell-18) > 1e-6 {
					t.Errorf("expected AvgDaysToSell ~18 from Charizard, got %f", s.ExpectedMetrics.AvgDaysToSell)
				}
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
			{Label: "Gengar", ROI: 0.30, SoldCount: 6, PurchaseCount: 10, CampaignCount: 1, Dimension: "character", BestChannel: SaleChannelInPerson},
		},
		CoverageGaps: []CoverageGap{
			{
				Segment: SegmentPerformance{Label: "Gengar", ROI: 0.30, SoldCount: 6, PurchaseCount: 10, Dimension: "character", BestChannel: SaleChannelInPerson},
				Reason:  "Gengar has 30% ROI but not in any active campaign",
			},
		},
		DataSummary: InsightsDataSummary{TotalPurchases: 50},
	}

	resp := GenerateSuggestions(context.Background(), insights, nil, nil)

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
	cases := []struct {
		name            string
		insights        *PortfolioInsights
		campaigns       []Campaign
		wantCampaign    string // empty means no suggestion expected
		wantNewTerms    float64
		wantExact       bool
		wantExpectedROI float64 // expected ExpectedMetrics.ExpectedROI = suggTargetMargin / newTerms
	}{
		{
			name: "weighted margin already meets target",
			// totalRev 100000, totalNet 12000 => weighted 12% (above 10% target)
			insights: &PortfolioInsights{
				ByChannel: []ChannelPNL{
					{Channel: SaleChannelEbay, SaleCount: 20, RevenueCents: 70000, NetProfitCents: 7000},
					{Channel: SaleChannelInPerson, SaleCount: 10, RevenueCents: 30000, NetProfitCents: 5000},
				},
				DataSummary: InsightsDataSummary{TotalPurchases: 50, TotalSales: 30},
			},
			campaigns: []Campaign{{Name: "Healthy", Phase: PhaseActive, BuyTermsCLPct: 0.85}},
		},
		{
			name: "gap below reduction buffer",
			// weighted 8%, target 10%, gap 2% < 5% buffer
			insights: &PortfolioInsights{
				ByChannel: []ChannelPNL{
					{Channel: SaleChannelEbay, SaleCount: 20, RevenueCents: 100000, NetProfitCents: 8000},
				},
				DataSummary: InsightsDataSummary{TotalPurchases: 50, TotalSales: 20},
			},
			campaigns: []Campaign{{Name: "Near Target", Phase: PhaseActive, BuyTermsCLPct: 0.85}},
		},
		{
			name: "gap exceeds buffer — suggest reduction",
			// weighted 3%, target 10%, gap 7% → lower 0.85 to 0.78
			insights: &PortfolioInsights{
				ByChannel: []ChannelPNL{
					{Channel: SaleChannelEbay, SaleCount: 20, RevenueCents: 100000, NetProfitCents: 3000},
				},
				DataSummary: InsightsDataSummary{TotalPurchases: 50, TotalSales: 20},
			},
			campaigns:       []Campaign{{Name: "Underperformer", Phase: PhaseActive, BuyTermsCLPct: 0.85}},
			wantCampaign:    "Underperformer",
			wantNewTerms:    0.78,
			wantExact:       true,
			wantExpectedROI: 0.10 / 0.78,
		},
		{
			name: "floor respected at 70%",
			// weighted -5%, target 10%, gap 15% → 0.74 - 0.15 = 0.59, floored to 0.70
			insights: &PortfolioInsights{
				ByChannel: []ChannelPNL{
					{Channel: SaleChannelEbay, SaleCount: 20, RevenueCents: 100000, NetProfitCents: -5000},
				},
				DataSummary: InsightsDataSummary{TotalPurchases: 50, TotalSales: 20},
			},
			campaigns:       []Campaign{{Name: "Bleeding", Phase: PhaseActive, BuyTermsCLPct: 0.74}},
			wantCampaign:    "Bleeding",
			wantNewTerms:    0.70,
			wantExact:       true,
			wantExpectedROI: 0.10 / 0.70,
		},
		{
			name: "no channel data",
			insights: &PortfolioInsights{
				ByChannel:   nil,
				DataSummary: InsightsDataSummary{TotalPurchases: 50, TotalSales: 20},
			},
			campaigns: []Campaign{{Name: "X", Phase: PhaseActive, BuyTermsCLPct: 0.85}},
		},
		{
			name: "single channel — weighted == channel margin",
			// single eBay channel at 2% → gap 8% → 0.85 - 0.08 = 0.77
			insights: &PortfolioInsights{
				ByChannel: []ChannelPNL{
					{Channel: SaleChannelEbay, SaleCount: 25, RevenueCents: 50000, NetProfitCents: 1000},
				},
				DataSummary: InsightsDataSummary{TotalPurchases: 40, TotalSales: 25},
			},
			campaigns:       []Campaign{{Name: "Solo eBay", Phase: PhaseActive, BuyTermsCLPct: 0.85}},
			wantCampaign:    "Solo eBay",
			wantNewTerms:    0.77,
			wantExact:       true,
			wantExpectedROI: 0.10 / 0.77,
		},
		{
			name: "mixed channels below buffer",
			// eBay +20% on 60% of rev, InPerson -15% on 40% → weighted = 0.12*0.6 + -0.15*0.4 = 0.072 - 0.06 = 0.012
			// Actually: 0.20*0.6 + -0.15*0.4 = 0.12 - 0.06 = 0.06 → gap 4% (below buffer)
			insights: &PortfolioInsights{
				ByChannel: []ChannelPNL{
					{Channel: SaleChannelEbay, SaleCount: 18, RevenueCents: 60000, NetProfitCents: 12000},
					{Channel: SaleChannelInPerson, SaleCount: 8, RevenueCents: 40000, NetProfitCents: -6000},
				},
				DataSummary: InsightsDataSummary{TotalPurchases: 40, TotalSales: 26},
			},
			campaigns: []Campaign{{Name: "Mixed", Phase: PhaseActive, BuyTermsCLPct: 0.85}},
		},
		{
			name: "archived campaign skipped",
			insights: &PortfolioInsights{
				ByChannel: []ChannelPNL{
					{Channel: SaleChannelEbay, SaleCount: 20, RevenueCents: 100000, NetProfitCents: 3000},
				},
				DataSummary: InsightsDataSummary{TotalPurchases: 50, TotalSales: 20},
			},
			campaigns: []Campaign{{Name: "Closed", Phase: PhaseClosed, BuyTermsCLPct: 0.85}},
		},
		{
			name: "zero revenue edge case",
			insights: &PortfolioInsights{
				ByChannel: []ChannelPNL{
					{Channel: SaleChannelEbay, SaleCount: 0, RevenueCents: 0, NetProfitCents: 0},
				},
				DataSummary: InsightsDataSummary{TotalPurchases: 30, TotalSales: 20},
			},
			campaigns: []Campaign{{Name: "Empty", Phase: PhaseActive, BuyTermsCLPct: 0.85}},
		},
		{
			name: "production smoke: Modern eBay +26.6% no longer fires",
			// eBay margin +26.6% on full revenue → weighted 26.6% ≫ 10% target → no suggestion.
			// Regression guard for the old "lower Modern to 28.57%" bad output.
			insights: &PortfolioInsights{
				ByChannel: []ChannelPNL{
					{Channel: SaleChannelEbay, SaleCount: 30, RevenueCents: 500000, NetProfitCents: 133000},
				},
				DataSummary: InsightsDataSummary{TotalPurchases: 60, TotalSales: 30},
			},
			campaigns: []Campaign{{Name: "Modern", Phase: PhaseActive, BuyTermsCLPct: 0.85, EbayFeePct: 0.1235}},
		},
		{
			name: "below minimum total sales — skipped",
			insights: &PortfolioInsights{
				ByChannel: []ChannelPNL{
					{Channel: SaleChannelEbay, SaleCount: 5, RevenueCents: 20000, NetProfitCents: -2000},
				},
				DataSummary: InsightsDataSummary{TotalPurchases: 20, TotalSales: 5},
			},
			campaigns: []Campaign{{Name: "Sparse", Phase: PhaseActive, BuyTermsCLPct: 0.85}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := GenerateSuggestions(context.Background(), tc.insights, tc.campaigns, nil)

			var match *CampaignSuggestion
			for i := range resp.Adjustments {
				s := &resp.Adjustments[i]
				if s.Type != "adjust" || s.SuggestedParams.BuyTermsCLPct <= 0 {
					continue
				}
				// Only consider suggestions from the channel-informed rule (Title starts with "Lower buy terms on").
				if !strings.HasPrefix(s.Title, "Lower buy terms on ") {
					continue
				}
				match = s
				break
			}

			if tc.wantCampaign == "" {
				if match != nil {
					t.Errorf("expected no channel-informed buy terms suggestion, got %+v", match.SuggestedParams)
				}
				return
			}

			if match == nil {
				t.Fatalf("expected channel-informed buy terms suggestion for %s, got none", tc.wantCampaign)
			}
			if match.SuggestedParams.Name != tc.wantCampaign {
				t.Errorf("expected campaign %s, got %s", tc.wantCampaign, match.SuggestedParams.Name)
			}
			if match.SuggestedParams.BuyTermsCLPct >= tc.campaigns[0].BuyTermsCLPct {
				t.Errorf("suggested terms %.4f must be < current %.4f",
					match.SuggestedParams.BuyTermsCLPct, tc.campaigns[0].BuyTermsCLPct)
			}
			if tc.wantExact {
				const eps = 1e-9
				if math.Abs(match.SuggestedParams.BuyTermsCLPct-tc.wantNewTerms) > eps {
					t.Errorf("expected newTerms %.4f, got %.4f", tc.wantNewTerms, match.SuggestedParams.BuyTermsCLPct)
				}
			}
			// ExpectedROI is derived from the projected margin (suggTargetMargin)
			// and the suggested buy terms so it describes the post-adjustment
			// state consistently with ExpectedMarginPct.
			if match.ExpectedMetrics.ExpectedROI == 0 {
				t.Errorf("expected non-zero ExpectedROI, got 0")
			}
			if math.Abs(match.ExpectedMetrics.ExpectedROI-tc.wantExpectedROI) > 1e-9 {
				t.Errorf("expected ExpectedROI %.4f (suggTargetMargin/newTerms), got %.4f",
					tc.wantExpectedROI, match.ExpectedMetrics.ExpectedROI)
			}
		})
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

	resp := GenerateSuggestions(context.Background(), insights, campaigns, nil)

	// Caps differ by 10x, should suggest rebalancing
	found := false
	for _, s := range resp.Adjustments {
		if s.Title == "Rebalance spend caps" {
			found = true
			if s.SuggestedParams.DailySpendCapCents != 27500 {
				t.Errorf("expected avg cap 27500, got %d", s.SuggestedParams.DailySpendCapCents)
			}
			// Non-ROI branch sets ExpectedROI = OverallROI (0.20).
			if s.ExpectedMetrics.ExpectedROI == 0 {
				t.Errorf("expected non-zero ExpectedROI, got 0")
			}
			if math.Abs(s.ExpectedMetrics.ExpectedROI-0.20) > 1e-9 {
				t.Errorf("expected ExpectedROI ~0.20 (OverallROI), got %f", s.ExpectedMetrics.ExpectedROI)
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

	resp := GenerateSuggestions(context.Background(), insights, campaigns, nil)

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

	resp := GenerateSuggestions(context.Background(), insights, campaigns, nil)

	found := false
	for _, s := range resp.Adjustments {
		if s.Title == "Rebalance spend caps by ROI" {
			found = true
			// Should mention raising High ROI and lowering Low ROI
			if !containsAll(s.Rationale, "raise", "High ROI", "lower", "Low ROI") {
				t.Errorf("expected ROI-weighted rationale, got: %s", s.Rationale)
			}
			// ROI-weighted branch sets ExpectedROI = avgROI (OverallROI = 0.20).
			if s.ExpectedMetrics.ExpectedROI == 0 {
				t.Errorf("expected non-zero ExpectedROI, got 0")
			}
			if math.Abs(s.ExpectedMetrics.ExpectedROI-0.20) > 1e-9 {
				t.Errorf("expected ExpectedROI ~0.20 (avgROI), got %f", s.ExpectedMetrics.ExpectedROI)
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

	resp := GenerateSuggestions(context.Background(), insights, campaigns, nil)

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

	resp := GenerateSuggestions(context.Background(), insights, campaigns, nil)

	found := false
	for _, s := range resp.Adjustments {
		if s.Title == "Activate Pending Charizard" {
			found = true
			// bestROI of matching segment (Charizard = 0.25) should propagate.
			if math.Abs(s.ExpectedMetrics.ExpectedROI-0.25) > 1e-6 {
				t.Errorf("expected ExpectedROI ~0.25 (Charizard bestROI), got %f", s.ExpectedMetrics.ExpectedROI)
			}
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

func TestGenerateSuggestions_BuyTermsFromLiquidation(t *testing.T) {
	// Minimal insights that won't trigger any unrelated rule. The
	// liquidation-aware rule reads CampaignHealth directly, not insights.
	baseInsights := func() *PortfolioInsights {
		return &PortfolioInsights{
			DataSummary: InsightsDataSummary{TotalPurchases: 0, TotalSales: 0},
		}
	}

	cases := []struct {
		name           string
		campaigns      []Campaign
		health         map[string]CampaignHealth
		wantCampaign   string // empty means no suggestion expected
		wantNewTerms   float64
		wantConfidence string
		wantDataPoints int
		wantMarginPct  float64 // expected ExpectedMarginPct == h.EbayChannelMarginPct
	}{
		{
			name: "below loss threshold",
			// $400 loss over 10 sales — below $500 threshold, skip.
			campaigns: []Campaign{{ID: "c1", Name: "LowLoss", Phase: PhaseActive, BuyTermsCLPct: 0.85}},
			health: map[string]CampaignHealth{
				"c1": {CampaignID: "c1", LiquidationLossCents: -40000, LiquidationSaleCount: 10},
			},
		},
		{
			name: "below sample size",
			// $600 loss but only 4 sales — below 5-sample guard, skip.
			campaigns: []Campaign{{ID: "c2", Name: "TooFewSales", Phase: PhaseActive, BuyTermsCLPct: 0.85}},
			health: map[string]CampaignHealth{
				"c2": {CampaignID: "c2", LiquidationLossCents: -60000, LiquidationSaleCount: 4},
			},
		},
		{
			name: "bucket $15-30/sale → reduction 3%, medium confidence",
			// $25/sale × 22 sales = $550 total, avg $25. Bucket is 3%.
			// Marketplace margin positive so the gate allows the rule.
			campaigns: []Campaign{{ID: "c3", Name: "Mid-Era", Phase: PhaseActive, BuyTermsCLPct: 0.80}},
			health: map[string]CampaignHealth{
				"c3": {CampaignID: "c3", LiquidationLossCents: -55000, LiquidationSaleCount: 22, EbayChannelMarginPct: 0.18},
			},
			wantCampaign:   "Mid-Era",
			wantNewTerms:   0.77,
			wantConfidence: "high",
			wantDataPoints: 22,
			wantMarginPct:  0.18,
		},
		{
			name: "bucket $30-50/sale → reduction 5%",
			// $40/sale avg × 13 sales = $520. Bucket is 5%. 13 sales → high confidence.
			campaigns: []Campaign{{ID: "c4", Name: "Vintage Core", Phase: PhaseActive, BuyTermsCLPct: 0.80}},
			health: map[string]CampaignHealth{
				"c4": {CampaignID: "c4", LiquidationLossCents: -52000, LiquidationSaleCount: 13, EbayChannelMarginPct: 0.20},
			},
			wantCampaign:   "Vintage Core",
			wantNewTerms:   0.75,
			wantConfidence: "high",
			wantDataPoints: 13,
			wantMarginPct:  0.20,
		},
		{
			name: "bucket >$50/sale → reduction 8%",
			// $80/sale avg × 8 sales = $640. Bucket is 8%. 8 sales → medium confidence (below 10).
			campaigns: []Campaign{{ID: "c5", Name: "Vintage Low Grade", Phase: PhaseActive, BuyTermsCLPct: 0.82}},
			health: map[string]CampaignHealth{
				"c5": {CampaignID: "c5", LiquidationLossCents: -64000, LiquidationSaleCount: 8, EbayChannelMarginPct: 0.22},
			},
			wantCampaign:   "Vintage Low Grade",
			wantNewTerms:   0.74,
			wantConfidence: "medium",
			wantDataPoints: 8,
			wantMarginPct:  0.22,
		},
		{
			name: "floor clamps reduction",
			// Campaign at 74% with 8% bucket ($80/sale × 10) → 74-8=66, clamped to 70.
			campaigns: []Campaign{{ID: "c6", Name: "NearFloor", Phase: PhaseActive, BuyTermsCLPct: 0.74}},
			health: map[string]CampaignHealth{
				"c6": {CampaignID: "c6", LiquidationLossCents: -80000, LiquidationSaleCount: 10, EbayChannelMarginPct: 0.15},
			},
			wantCampaign:   "NearFloor",
			wantNewTerms:   0.70,
			wantConfidence: "high",
			wantDataPoints: 10,
			wantMarginPct:  0.15,
		},
		{
			name: "already at floor — skip",
			// 70% + 8% bucket = would be 62, clamped to 70, which equals current → skip.
			campaigns: []Campaign{{ID: "c7", Name: "AtFloor", Phase: PhaseActive, BuyTermsCLPct: 0.70}},
			health: map[string]CampaignHealth{
				"c7": {CampaignID: "c7", LiquidationLossCents: -80000, LiquidationSaleCount: 10, EbayChannelMarginPct: 0.15},
			},
		},
		{
			name: "below floor — skip",
			// 0.68 < 0.70 floor → clamped to 0.70 which is > current → skip (rule never raises terms).
			campaigns: []Campaign{{ID: "c8", Name: "BelowFloor", Phase: PhaseActive, BuyTermsCLPct: 0.68}},
			health: map[string]CampaignHealth{
				"c8": {CampaignID: "c8", LiquidationLossCents: -80000, LiquidationSaleCount: 10, EbayChannelMarginPct: 0.15},
			},
		},
		{
			name: "sample size tier — 10 sales → high",
			// avg $60/sale × 10 = $600, bucket 8%. Tier boundary: 10 sales → high.
			campaigns: []Campaign{{ID: "c9", Name: "Boundary10", Phase: PhaseActive, BuyTermsCLPct: 0.85}},
			health: map[string]CampaignHealth{
				"c9": {CampaignID: "c9", LiquidationLossCents: -60000, LiquidationSaleCount: 10, EbayChannelMarginPct: 0.15},
			},
			wantCampaign:   "Boundary10",
			wantNewTerms:   0.77,
			wantConfidence: "high",
			wantDataPoints: 10,
			wantMarginPct:  0.15,
		},
		{
			name: "sample size tier — 9 sales → medium",
			// avg ~$67/sale × 9 = $600, bucket 8%. 9 sales → medium.
			campaigns: []Campaign{{ID: "c10", Name: "Boundary9", Phase: PhaseActive, BuyTermsCLPct: 0.85}},
			health: map[string]CampaignHealth{
				"c10": {CampaignID: "c10", LiquidationLossCents: -60000, LiquidationSaleCount: 9, EbayChannelMarginPct: 0.15},
			},
			wantCampaign:   "Boundary9",
			wantNewTerms:   0.77,
			wantConfidence: "medium",
			wantDataPoints: 9,
			wantMarginPct:  0.15,
		},
		{
			name: "archived campaign skipped",
			campaigns: []Campaign{
				{ID: "c11", Name: "Archived", Phase: PhaseClosed, BuyTermsCLPct: 0.85},
			},
			health: map[string]CampaignHealth{
				"c11": {CampaignID: "c11", LiquidationLossCents: -80000, LiquidationSaleCount: 10, EbayChannelMarginPct: 0.15},
			},
		},
		{
			name:      "missing health row — no panic, no suggestion",
			campaigns: []Campaign{{ID: "c12", Name: "NoHealth", Phase: PhaseActive, BuyTermsCLPct: 0.85}},
			health: map[string]CampaignHealth{
				"other": {CampaignID: "other", LiquidationLossCents: -80000, LiquidationSaleCount: 10, EbayChannelMarginPct: 0.15},
			},
		},
		{
			name:         "nil health map — rule skips cleanly",
			campaigns:    []Campaign{{ID: "c13", Name: "NilHealth", Phase: PhaseActive, BuyTermsCLPct: 0.85}},
			health:       nil,
			wantCampaign: "",
		},
		{
			name: "zero marketplace margin — rule skips (no marketplace sales to baseline)",
			// Plenty of liquidation damage, but no marketplace sales means we
			// don't have a trustworthy margin baseline to recommend from.
			campaigns: []Campaign{{ID: "c14", Name: "NoMarketplaceSales", Phase: PhaseActive, BuyTermsCLPct: 0.85}},
			health: map[string]CampaignHealth{
				"c14": {CampaignID: "c14", LiquidationLossCents: -80000, LiquidationSaleCount: 10, EbayChannelMarginPct: 0.0},
			},
		},
		{
			name: "negative marketplace margin — rule skips (channel itself is broken)",
			// Lowering buy terms won't rescue a campaign whose marketplace
			// channel is already underwater. Fix the channel first.
			campaigns: []Campaign{{ID: "c15", Name: "BrokenMarketplace", Phase: PhaseActive, BuyTermsCLPct: 0.85}},
			health: map[string]CampaignHealth{
				"c15": {CampaignID: "c15", LiquidationLossCents: -80000, LiquidationSaleCount: 10, EbayChannelMarginPct: -0.05},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := GenerateSuggestions(context.Background(), baseInsights(), tc.campaigns, tc.health)

			var match *CampaignSuggestion
			for i := range resp.Adjustments {
				s := &resp.Adjustments[i]
				if s.Type != "adjust" || s.SuggestedParams.BuyTermsCLPct <= 0 {
					continue
				}
				if !strings.Contains(s.Title, "(liquidation buffer)") {
					continue
				}
				match = s
				break
			}

			if tc.wantCampaign == "" {
				if match != nil {
					t.Errorf("expected no liquidation-buffer suggestion, got %+v", match.SuggestedParams)
				}
				return
			}
			if match == nil {
				t.Fatalf("expected liquidation-buffer suggestion for %s, got none", tc.wantCampaign)
			}
			if match.SuggestedParams.Name != tc.wantCampaign {
				t.Errorf("expected campaign %s, got %s", tc.wantCampaign, match.SuggestedParams.Name)
			}
			const eps = 1e-9
			if math.Abs(match.SuggestedParams.BuyTermsCLPct-tc.wantNewTerms) > eps {
				t.Errorf("expected newTerms %.4f, got %.4f", tc.wantNewTerms, match.SuggestedParams.BuyTermsCLPct)
			}
			if match.SuggestedParams.BuyTermsCLPct >= tc.campaigns[0].BuyTermsCLPct {
				t.Errorf("suggested terms %.4f must be < current %.4f",
					match.SuggestedParams.BuyTermsCLPct, tc.campaigns[0].BuyTermsCLPct)
			}
			if match.Confidence != tc.wantConfidence {
				t.Errorf("expected confidence %q, got %q", tc.wantConfidence, match.Confidence)
			}
			if match.DataPoints != tc.wantDataPoints {
				t.Errorf("expected DataPoints %d, got %d", tc.wantDataPoints, match.DataPoints)
			}
			// ExpectedMarginPct == h.EbayChannelMarginPct.
			// ExpectedROI is derived as margin/newTerms so both fields describe
			// the projected post-adjustment state consistently.
			if math.Abs(match.ExpectedMetrics.ExpectedMarginPct-tc.wantMarginPct) > 1e-9 {
				t.Errorf("expected ExpectedMarginPct %.4f (EbayChannelMarginPct), got %.4f",
					tc.wantMarginPct, match.ExpectedMetrics.ExpectedMarginPct)
			}
			wantROI := tc.wantMarginPct / tc.wantNewTerms
			if math.Abs(match.ExpectedMetrics.ExpectedROI-wantROI) > 1e-9 {
				t.Errorf("expected ExpectedROI %.4f (margin/newTerms), got %.4f",
					wantROI, match.ExpectedMetrics.ExpectedROI)
			}
		})
	}
}

func TestComputeBuyTermsReduction(t *testing.T) {
	cases := []struct {
		name       string
		lossCents  int
		saleCount  int
		wantResult float64
	}{
		{"zero sales → zero reduction", -50000, 0, 0},
		{"default bucket ($10/sale)", -50000, 50, 0.02},
		{"$15–30 bucket ($20/sale)", -40000, 20, 0.03},
		{"$30–50 bucket ($40/sale)", -40000, 10, 0.05},
		{">$50 bucket ($100/sale)", -100000, 10, 0.08},
		{"boundary $15 → default bucket", -15000, 10, 0.02},
		{"boundary $30 → $15–30 bucket", -30000, 10, 0.03},
		{"boundary $50 → $30–50 bucket", -50000, 10, 0.05},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := CampaignHealth{
				LiquidationLossCents: tc.lossCents,
				LiquidationSaleCount: tc.saleCount,
			}
			got := computeBuyTermsReduction(h)
			if math.Abs(got-tc.wantResult) > 1e-9 {
				t.Errorf("want %.2f, got %.2f", tc.wantResult, got)
			}
		})
	}
}
