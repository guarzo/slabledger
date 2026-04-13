package scoring

import (
	"errors"
	"math"
	"testing"
)

func TestScore_CompositeCalculation(t *testing.T) {
	tests := []struct {
		name             string
		factors          []Factor
		weights          []FactorWeight
		wantComposite    float64
		compositeEpsilon float64
	}{
		{
			name: "two equal factors same direction",
			factors: []Factor{
				{Name: "market_trend", Value: 0.5, Confidence: 1.0, Source: "test"},
				{Name: "liquidity", Value: 0.5, Confidence: 1.0, Source: "test"},
			},
			weights: []FactorWeight{
				{Name: "market_trend", Weight: 0.5},
				{Name: "liquidity", Weight: 0.5},
			},
			wantComposite:    0.5,
			compositeEpsilon: 0.001,
		},
		{
			name: "two factors weighted",
			factors: []Factor{
				{Name: "market_trend", Value: 0.8, Confidence: 1.0, Source: "test"},
				{Name: "liquidity", Value: -0.4, Confidence: 1.0, Source: "test"},
			},
			weights: []FactorWeight{
				{Name: "market_trend", Weight: 0.6},
				{Name: "liquidity", Weight: 0.4},
			},
			wantComposite:    0.8*0.6 + (-0.4)*0.4, // 0.48 - 0.16 = 0.32
			compositeEpsilon: 0.001,
		},
		{
			name: "missing third factor renormalizes weights",
			factors: []Factor{
				{Name: "market_trend", Value: 0.5, Confidence: 1.0, Source: "test"},
				{Name: "liquidity", Value: 0.5, Confidence: 1.0, Source: "test"},
			},
			weights: []FactorWeight{
				{Name: "market_trend", Weight: 0.5},
				{Name: "liquidity", Weight: 0.3},
				{Name: "scarcity", Weight: 0.2}, // missing from factors
			},
			// market_trend(0.5) + liquidity(0.5), weights renormalized to 0.5/0.8 and 0.3/0.8
			wantComposite:    0.5*(0.5/0.8) + 0.5*(0.3/0.8),
			compositeEpsilon: 0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := ScoreRequest{
				EntityID:   "test-1",
				EntityType: "purchase",
				Factors:    tt.factors,
			}
			profile := WeightProfile{Name: "test", Weights: tt.weights}
			sc, err := Score(req, profile)
			if err != nil {
				t.Fatalf("Score() error: %v", err)
			}
			if math.Abs(sc.Composite-tt.wantComposite) > tt.compositeEpsilon {
				t.Errorf("Composite = %f, want %f (±%f)", sc.Composite, tt.wantComposite, tt.compositeEpsilon)
			}
		})
	}
}

func TestScore_VerdictDerivation(t *testing.T) {
	tests := []struct {
		name        string
		composite   float64
		wantVerdict Verdict
	}{
		{"strong buy", 0.7, VerdictStrongBuy},
		{"buy", 0.4, VerdictBuy},
		{"lean buy", 0.15, VerdictLeanBuy},
		{"hold positive edge", 0.05, VerdictHold},
		{"hold negative edge", -0.05, VerdictHold},
		{"lean sell", -0.15, VerdictLeanSell},
		{"sell", -0.4, VerdictSell},
		{"strong sell", -0.7, VerdictStrongSell},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VerdictFromComposite(tt.composite)
			if got != tt.wantVerdict {
				t.Errorf("VerdictFromComposite(%f) = %s, want %s", tt.composite, got, tt.wantVerdict)
			}
		})
	}
}

func TestScore_ConfidenceCalculation(t *testing.T) {
	// All factors present, high values, same direction → high confidence
	req := ScoreRequest{
		EntityID:   "test-1",
		EntityType: "purchase",
		Factors: []Factor{
			{Name: "a", Value: 0.8, Confidence: 1.0, Source: "test"},
			{Name: "b", Value: 0.6, Confidence: 0.9, Source: "test"},
			{Name: "c", Value: 0.7, Confidence: 0.8, Source: "test"},
		},
	}
	profile := WeightProfile{
		Name: "test",
		Weights: []FactorWeight{
			{Name: "a", Weight: 0.4},
			{Name: "b", Weight: 0.3},
			{Name: "c", Weight: 0.3},
		},
	}
	sc, err := Score(req, profile)
	if err != nil {
		t.Fatalf("Score() error: %v", err)
	}
	if sc.Confidence < 0.7 {
		t.Errorf("expected high confidence, got %f", sc.Confidence)
	}
	if sc.Confidence > 0.95 {
		t.Errorf("confidence should be clamped to 0.95, got %f", sc.Confidence)
	}
}

func TestScore_InsufficientData(t *testing.T) {
	req := ScoreRequest{
		EntityID:   "test-1",
		EntityType: "purchase",
		Factors: []Factor{
			{Name: "a", Value: 0.5, Confidence: 1.0, Source: "test"},
		},
		DataGaps: []DataGap{
			{FactorName: "b", Reason: "no_market_data"},
			{FactorName: "c", Reason: "no_price_source"},
		},
	}
	profile := WeightProfile{
		Name: "test",
		Weights: []FactorWeight{
			{Name: "a", Weight: 0.4},
			{Name: "b", Weight: 0.3},
			{Name: "c", Weight: 0.3},
		},
	}
	_, err := Score(req, profile)
	if err == nil {
		t.Fatal("expected ErrInsufficientData, got nil")
	}
	var insuffErr *ErrInsufficientData
	if !errors.As(err, &insuffErr) {
		t.Fatalf("expected *ErrInsufficientData, got %T", err)
	}
	if insuffErr.Available != 1 {
		t.Errorf("Available = %d, want 1", insuffErr.Available)
	}
}

func TestScore_InsufficientFactors(t *testing.T) {
	tests := []struct {
		name    string
		factors []Factor
		gaps    []DataGap
	}{
		{
			name:    "zero factors",
			factors: nil, // zero factors, no gaps
			gaps:    nil,
		},
		{
			name:    "too few factors no gaps",
			factors: []Factor{{Name: "roi_potential", Value: 0.5, Confidence: 0.8}},
			gaps:    nil, // only 1 factor below MinFactors, DataGaps empty
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := ScoreRequest{
				EntityID:   "test",
				EntityType: "campaign",
				Factors:    tt.factors,
				DataGaps:   tt.gaps,
			}
			_, err := Score(req, PurchaseAssessmentProfile)
			if err == nil {
				t.Fatal("expected ErrInsufficientData, got nil")
			}
			var insuffErr *ErrInsufficientData
			if !errors.As(err, &insuffErr) {
				t.Fatalf("expected *ErrInsufficientData, got %T: %v", err, err)
			}
		})
	}
}

func TestScore_EntityFieldsPreserved(t *testing.T) {
	req := ScoreRequest{
		EntityID:   "card-123",
		EntityType: "purchase",
		Factors: []Factor{
			{Name: "a", Value: 0.5, Confidence: 1.0, Source: "s1"},
			{Name: "b", Value: 0.3, Confidence: 0.8, Source: "s2"},
		},
		DataGaps: []DataGap{
			{FactorName: "c", Reason: "missing"},
		},
	}
	profile := WeightProfile{Name: "test", Weights: []FactorWeight{
		{Name: "a", Weight: 0.5},
		{Name: "b", Weight: 0.3},
		{Name: "c", Weight: 0.2},
	}}
	sc, err := Score(req, profile)
	if err != nil {
		t.Fatalf("Score() error: %v", err)
	}
	if sc.EntityID != "card-123" {
		t.Errorf("EntityID = %s, want card-123", sc.EntityID)
	}
	if sc.EntityType != "purchase" {
		t.Errorf("EntityType = %s, want purchase", sc.EntityType)
	}
	if len(sc.DataGaps) != 1 {
		t.Errorf("DataGaps len = %d, want 1", len(sc.DataGaps))
	}
	if sc.ScoredAt.IsZero() {
		t.Error("ScoredAt should not be zero")
	}
}
