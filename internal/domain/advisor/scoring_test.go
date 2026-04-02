package advisor

import (
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/scoring"
)

func TestBuildPurchaseScoreCard(t *testing.T) {
	data := &PurchaseFactorData{
		PriceChangePct:    ptrFloat(15.0),
		SalesPerMonth:     ptrFloat(8.0),
		ROIPct:            ptrFloat(20.0),
		ConcentrationRisk: "low",
		PriceConfidence:   0.9,
		MarketSource:      "fusion",
	}
	sc, err := BuildScoreCard("cert-123", "purchase", data, scoring.PurchaseAssessmentProfile)
	if err != nil {
		t.Fatalf("BuildScoreCard: %v", err)
	}
	if sc.EntityID != "cert-123" {
		t.Errorf("EntityID = %s, want cert-123", sc.EntityID)
	}
	if len(sc.Factors) < 3 {
		t.Errorf("Factors count = %d, want >= 3", len(sc.Factors))
	}
	if len(sc.DataGaps) > 4 {
		t.Errorf("too many gaps: %d", len(sc.DataGaps))
	}
}

func TestBuildPurchaseScoreCard_InsufficientData(t *testing.T) {
	// Only portfolio_fit is unconditional, giving 1 factor with 6 gaps.
	// Score() requires MinFactors (2) when gaps exist.
	data := &PurchaseFactorData{
		PriceConfidence: 0.5,
		MarketSource:    "test",
	}
	_, err := BuildScoreCard("cert-456", "purchase", data, scoring.PurchaseAssessmentProfile)
	if err == nil {
		t.Fatal("expected error for insufficient data")
	}
	var insuffErr *scoring.ErrInsufficientData
	if !errors.As(err, &insuffErr) {
		t.Fatalf("expected *ErrInsufficientData, got %T: %v", err, err)
	}
}

func TestBuildCampaignScoreCard(t *testing.T) {
	data := &CampaignFactorData{
		ROIPct:          ptrFloat(15.0),
		SellThroughPct:  ptrFloat(60.0),
		Trend30dPct:     ptrFloat(5.0),
		PriceConfidence: 0.8,
		MarketSource:    "test",
	}
	sc, err := BuildScoreCard("camp-1", "campaign", data, scoring.CampaignAnalysisProfile)
	if err != nil {
		t.Fatalf("BuildScoreCard: %v", err)
	}
	if sc.EntityID != "camp-1" {
		t.Errorf("EntityID = %s, want camp-1", sc.EntityID)
	}
	if len(sc.Factors) < 3 {
		t.Errorf("Factors count = %d, want >= 3", len(sc.Factors))
	}
}

func TestBuildLiquidationScoreCard(t *testing.T) {
	data := &LiquidationFactorData{
		DaysHeld:        90,
		CreditUtilPct:   ptrFloat(75.0),
		PriceChangePct:  ptrFloat(-5.0),
		SalesPerMonth:   ptrFloat(3.0),
		PriceConfidence: 0.7,
		MarketSource:    "test",
	}
	sc, err := BuildScoreCard("purch-1", "inventory_item", data, scoring.LiquidationProfile)
	if err != nil {
		t.Fatalf("BuildScoreCard: %v", err)
	}
	if len(sc.Factors) < 4 {
		t.Errorf("Factors count = %d, want >= 4", len(sc.Factors))
	}
}

func TestBuildSuggestionScoreCard(t *testing.T) {
	data := &SuggestionFactorData{
		ProjectedROIPct: ptrFloat(25.0),
		FillsGap:        true,
		OverlapCount:    0,
		Trend30dPct:     ptrFloat(8.0),
		SalesPerMonth:   ptrFloat(12.0),
		PriceChangePct:  ptrFloat(10.0),
		PriceConfidence: 0.85,
		MarketSource:    "fusion",
	}
	sc, err := BuildScoreCard("suggestion-1", "suggestion", data, scoring.CampaignSuggestionsProfile)
	if err != nil {
		t.Fatalf("BuildScoreCard: %v", err)
	}
	if sc.EntityID != "suggestion-1" {
		t.Errorf("EntityID = %s, want suggestion-1", sc.EntityID)
	}
	if len(sc.Factors) != 5 {
		t.Errorf("Factors count = %d, want 5", len(sc.Factors))
	}
	if len(sc.DataGaps) != 0 {
		t.Errorf("DataGaps count = %d, want 0", len(sc.DataGaps))
	}
}

func ptrFloat(f float64) *float64 { return &f }
