package psaexchange

import (
	"math"
	"testing"
)

func TestSelectTier(t *testing.T) {
	tests := []struct {
		name          string
		velocityMonth int
		confidence    int
		wantName      string
		wantOfferPct  float64
	}{
		{"high liquidity meets both", 5, 5, "high_liquidity", 0.75},
		{"velocity below threshold", 4, 5, "default", 0.65},
		{"confidence below threshold", 5, 4, "default", 0.65},
		{"both below", 0, 0, "default", 0.65},
		{"velocity well above, max confidence", 50, 5, "high_liquidity", 0.75},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SelectTier(tt.velocityMonth, tt.confidence)
			if got.Name != tt.wantName || got.MaxOfferPct != tt.wantOfferPct {
				t.Fatalf("got tier %+v, want name=%s pct=%v", got, tt.wantName, tt.wantOfferPct)
			}
		})
	}
}

func TestScoreListing_BasicMath(t *testing.T) {
	// Given a $10,000 list, $20,000 comp, velocityMonth=10, confidence=5:
	// tier => high_liquidity, max_offer_pct=0.75
	// target_offer = $15,000
	// edge_at_offer = (20000 - 15000) / 15000 = 1/3
	// velocity_score = log(1+10) ≈ 2.3979
	// score = (1/3) × 2.3979 ≈ 0.7993
	// list_runway = (10000 - 15000) / 10000 = -0.5 → mayTakeAtList = true
	got := ScoreListing(ScoreInputs{
		ListPriceCents: 1000000,
		CompCents:      2000000,
		VelocityMonth:  10,
		Confidence:     5,
	})
	if got.Tier.Name != "high_liquidity" {
		t.Fatalf("tier name = %s, want high_liquidity", got.Tier.Name)
	}
	if got.TargetOfferCents != 1500000 {
		t.Fatalf("targetOfferCents = %d, want 1500000", got.TargetOfferCents)
	}
	if math.Abs(got.EdgeAtOffer-1.0/3.0) > 1e-9 {
		t.Fatalf("edgeAtOffer = %v, want ~0.3333", got.EdgeAtOffer)
	}
	wantScore := (1.0 / 3.0) * math.Log(1+10)
	if math.Abs(got.Score-wantScore) > 1e-9 {
		t.Fatalf("score = %v, want %v", got.Score, wantScore)
	}
	if !got.MayTakeAtList {
		t.Fatal("mayTakeAtList should be true when list ≤ targetOffer")
	}
	if math.Abs(got.ListRunwayPct-(-0.5)) > 1e-9 {
		t.Fatalf("listRunwayPct = %v, want -0.5", got.ListRunwayPct)
	}
}

func TestScoreListing_DefaultTier(t *testing.T) {
	// velocityMonth=1, confidence=3 → default tier, 0.65 max_offer_pct
	// $14,000 list, $20,000 comp → target_offer = $13,000; list > target so mayTakeAtList = false
	got := ScoreListing(ScoreInputs{
		ListPriceCents: 1400000,
		CompCents:      2000000,
		VelocityMonth:  1,
		Confidence:     3,
	})
	if got.Tier.Name != "default" {
		t.Fatalf("tier name = %s, want default", got.Tier.Name)
	}
	if got.TargetOfferCents != 1300000 {
		t.Fatalf("targetOfferCents = %d, want 1300000", got.TargetOfferCents)
	}
	if got.MayTakeAtList {
		t.Fatal("mayTakeAtList should be false when list > targetOffer")
	}
}
