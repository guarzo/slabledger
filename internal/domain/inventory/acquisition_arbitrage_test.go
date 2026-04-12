package inventory

import (
	"math"
	"testing"
)

func TestComputeAcquisitionOpportunity(t *testing.T) {
	tests := []struct {
		name            string
		cardName        string
		setName         string
		cardNumber      string
		rawNM           int
		gradedEstimates map[string]int
		wantNil         bool
		wantGrade       string
		wantGradedCents int
		wantProfit      int
		wantROI         float64
	}{
		{
			name:       "profitable with multiple grades",
			cardName:   "Charizard",
			setName:    "Base Set",
			cardNumber: "4/102",
			rawNM:      5000, // $50
			gradedEstimates: map[string]int{
				"PSA 9":  20000, // $200
				"PSA 10": 50000, // $500
			},
			wantNil:         false,
			wantGrade:       "PSA 10",
			wantGradedCents: 50000,
			// net = 50000 - round(50000*0.1235) = 50000 - 6175 = 43825
			// profit = 43825 - 5000 = 38825
			wantProfit: 38825,
		},
		{
			name:            "below threshold",
			cardName:        "Common Card",
			setName:         "Set A",
			cardNumber:      "1/100",
			rawNM:           500,                            // $5
			gradedEstimates: map[string]int{"PSA 10": 1500}, // $15
			// net = 1500 - round(1500*0.1235) = 1500 - 185 = 1315
			// profit = 1315 - 500 = 815 < 10000 threshold
			wantNil: true,
		},
		{
			name:            "negative profit",
			cardName:        "Underwater Card",
			setName:         "Set B",
			cardNumber:      "2/100",
			rawNM:           10000,                          // $100
			gradedEstimates: map[string]int{"PSA 10": 8000}, // $80
			// net = 8000 - round(8000*0.1235) = 8000 - 988 = 7012
			// profit = 7012 - 10000 = -2988 < 10000 threshold
			wantNil: true,
		},
		{
			name:            "empty graded estimates",
			cardName:        "Some Card",
			setName:         "Set C",
			cardNumber:      "3/100",
			rawNM:           5000,
			gradedEstimates: map[string]int{},
			wantNil:         true,
		},
		{
			name:            "ROI calculation",
			cardName:        "Pikachu",
			setName:         "Promo",
			cardNumber:      "001",
			rawNM:           100000,                           // $1000
			gradedEstimates: map[string]int{"PSA 10": 250000}, // $2500
			wantNil:         false,
			wantGrade:       "PSA 10",
			wantGradedCents: 250000,
			// net = 250000 - round(250000*0.1235) = 250000 - 30875 = 219125
			// profit = 219125 - 100000 = 119125
			// ROI = 119125 / 100000 = 1.19125
			wantProfit: 119125,
			wantROI:    1.19125,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opp := computeAcquisitionOpportunity(
				tt.cardName, tt.setName, tt.cardNumber, "",
				tt.rawNM, tt.gradedEstimates, DefaultMarketplaceFeePct, "test",
			)

			if tt.wantNil {
				if opp != nil {
					t.Errorf("expected nil, got profit=%d", opp.ProfitCents)
				}
				return
			}

			if opp == nil {
				t.Fatal("expected non-nil opportunity")
			}
			if opp.BestGrade != tt.wantGrade {
				t.Errorf("BestGrade = %q, want %q", opp.BestGrade, tt.wantGrade)
			}
			if opp.BestGradedCents != tt.wantGradedCents {
				t.Errorf("BestGradedCents = %d, want %d", opp.BestGradedCents, tt.wantGradedCents)
			}
			if opp.ProfitCents != tt.wantProfit {
				t.Errorf("ProfitCents = %d, want %d", opp.ProfitCents, tt.wantProfit)
			}
			if opp.RawNMCents != tt.rawNM {
				t.Errorf("RawNMCents = %d, want %d", opp.RawNMCents, tt.rawNM)
			}
			if opp.Source != "test" {
				t.Errorf("Source = %q, want test", opp.Source)
			}
			if tt.wantROI > 0 && math.Abs(opp.ProfitROI-tt.wantROI) > 0.001 {
				t.Errorf("ProfitROI = %.4f, want %.4f", opp.ProfitROI, tt.wantROI)
			}
		})
	}
}
