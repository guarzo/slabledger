package scoring

import (
	"math"
	"testing"
)

func TestWeightProfiles_SumToOne(t *testing.T) {
	profiles := []WeightProfile{
		PurchaseAssessmentProfile,
		CampaignAnalysisProfile,
		LiquidationProfile,
		CampaignSuggestionsProfile,
	}
	for _, p := range profiles {
		t.Run(p.Name, func(t *testing.T) {
			var sum float64
			for _, w := range p.Weights {
				if w.Weight <= 0 {
					t.Errorf("weight for %s is non-positive: %f", w.Name, w.Weight)
				}
				sum += w.Weight
			}
			if math.Abs(sum-1.0) > 0.001 {
				t.Errorf("weights sum to %f, want 1.0", sum)
			}
		})
	}
}

func TestWeightProfiles_NoDuplicateFactors(t *testing.T) {
	profiles := []WeightProfile{
		PurchaseAssessmentProfile,
		CampaignAnalysisProfile,
		LiquidationProfile,
		CampaignSuggestionsProfile,
	}
	for _, p := range profiles {
		t.Run(p.Name, func(t *testing.T) {
			seen := make(map[string]bool)
			for _, w := range p.Weights {
				if seen[w.Name] {
					t.Errorf("duplicate factor: %s", w.Name)
				}
				seen[w.Name] = true
			}
		})
	}
}
