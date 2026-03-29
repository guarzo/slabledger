package campaigns

import (
	"math"
	"testing"
)

func TestComputeAcquisitionOpportunity_Profitable(t *testing.T) {
	rawNM := 5000  // $50
	psa9 := 20000  // $200
	psa10 := 50000 // $500

	gradedEstimates := map[string]int{
		"PSA 9":  psa9,
		"PSA 10": psa10,
	}

	opp := computeAcquisitionOpportunity(
		"Charizard", "Base Set", "4/102", "",
		rawNM, gradedEstimates, DefaultMarketplaceFeePct, "test",
	)

	if opp == nil {
		t.Fatal("expected non-nil opportunity")
	}
	if opp.BestGrade != "PSA 10" {
		t.Errorf("BestGrade = %q, want PSA 10", opp.BestGrade)
	}
	if opp.BestGradedCents != psa10 {
		t.Errorf("BestGradedCents = %d, want %d", opp.BestGradedCents, psa10)
	}

	// net = 50000 - round(50000*0.1235) = 50000 - 6175 = 43825
	// profit = 43825 - 5000 = 38825
	wantProfit := 38825
	if opp.ProfitCents != wantProfit {
		t.Errorf("ProfitCents = %d, want %d", opp.ProfitCents, wantProfit)
	}
	if opp.RawNMCents != rawNM {
		t.Errorf("RawNMCents = %d, want %d", opp.RawNMCents, rawNM)
	}
	if opp.Source != "test" {
		t.Errorf("Source = %q, want test", opp.Source)
	}
}

func TestComputeAcquisitionOpportunity_BelowThreshold(t *testing.T) {
	rawNM := 500  // $5
	psa10 := 1500 // $15

	gradedEstimates := map[string]int{"PSA 10": psa10}

	opp := computeAcquisitionOpportunity(
		"Common Card", "Set A", "1/100", "",
		rawNM, gradedEstimates, DefaultMarketplaceFeePct, "test",
	)

	// net = 1500 - round(1500*0.1235) = 1500 - 185 = 1315
	// profit = 1315 - 500 = 815 < 10000 threshold
	if opp != nil {
		t.Errorf("expected nil for below-threshold profit, got profit=%d", opp.ProfitCents)
	}
}

func TestComputeAcquisitionOpportunity_Negative(t *testing.T) {
	rawNM := 10000 // $100
	psa10 := 8000  // $80

	gradedEstimates := map[string]int{"PSA 10": psa10}

	opp := computeAcquisitionOpportunity(
		"Underwater Card", "Set B", "2/100", "",
		rawNM, gradedEstimates, DefaultMarketplaceFeePct, "test",
	)

	// net = 8000 - round(8000*0.1235) = 8000 - 988 = 7012
	// profit = 7012 - 10000 = -2988 < 10000 threshold
	if opp != nil {
		t.Errorf("expected nil for negative profit, got profit=%d", opp.ProfitCents)
	}
}

func TestComputeAcquisitionOpportunity_NoGradedEstimates(t *testing.T) {
	opp := computeAcquisitionOpportunity(
		"Some Card", "Set C", "3/100", "",
		5000, map[string]int{}, DefaultMarketplaceFeePct, "test",
	)

	if opp != nil {
		t.Error("expected nil for empty gradedEstimates")
	}
}

func TestComputeAcquisitionOpportunity_ROI(t *testing.T) {
	rawNM := 100000  // $1000
	psa10 := 250000  // $2500

	gradedEstimates := map[string]int{"PSA 10": psa10}

	opp := computeAcquisitionOpportunity(
		"Pikachu", "Promo", "001", "",
		rawNM, gradedEstimates, DefaultMarketplaceFeePct, "test",
	)

	if opp == nil {
		t.Fatal("expected non-nil opportunity")
	}

	// net = 250000 - round(250000*0.1235) = 250000 - 30875 = 219125
	// profit = 219125 - 100000 = 119125
	// ROI = 119125 / 100000 = 1.19125
	wantProfit := 119125
	if opp.ProfitCents != wantProfit {
		t.Errorf("ProfitCents = %d, want %d", opp.ProfitCents, wantProfit)
	}

	wantROI := float64(wantProfit) / float64(rawNM)
	if math.Abs(opp.ProfitROI-wantROI) > 0.001 {
		t.Errorf("ProfitROI = %.4f, want %.4f", opp.ProfitROI, wantROI)
	}
}
