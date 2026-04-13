package arbitrage

import "testing"

func TestComputeCrackAnalysis(t *testing.T) {
	tests := []struct {
		name              string
		purchaseID        string
		campaignID        string
		cardName          string
		certNumber        string
		grade             float64
		buyCents          int
		feeCents          int
		rawMarketCents    int
		gradedMarketCents int
		convRate          float64
		// expected
		wantCostBasis         int
		wantRawMarket         int
		wantIsCandidate       bool
		wantPositiveBreakeven bool
		wantPositiveAdvantage bool
		wantNegativeGradedNet bool
		wantPositiveCrackNet  bool
		wantCrackROIGtGraded  bool
	}{
		{
			name:                  "base",
			purchaseID:            "p1",
			campaignID:            "camp1",
			cardName:              "Charizard",
			certNumber:            "12345",
			grade:                 8,
			buyCents:              8500,
			feeCents:              300,
			rawMarketCents:        12000,
			gradedMarketCents:     10000,
			convRate:              0.1235,
			wantCostBasis:         8800,
			wantRawMarket:         12000,
			wantIsCandidate:       true,
			wantPositiveBreakeven: true,
			wantPositiveAdvantage: true,
		},
		{
			name:              "not_candidate",
			purchaseID:        "p2",
			campaignID:        "camp1",
			cardName:          "Pikachu",
			certNumber:        "67890",
			grade:             8,
			buyCents:          5000,
			feeCents:          300,
			rawMarketCents:    3000,
			gradedMarketCents: 8000,
			convRate:          0.1235,
			wantIsCandidate:   false,
		},
		{
			name:                  "psa7",
			purchaseID:            "p3",
			campaignID:            "camp1",
			cardName:              "Umbreon VMAX",
			certNumber:            "99999",
			grade:                 7,
			buyCents:              14700,
			feeCents:              300,
			rawMarketCents:        25000,
			gradedMarketCents:     20000,
			convRate:              0.1235,
			wantCostBasis:         15000,
			wantIsCandidate:       true,
			wantPositiveAdvantage: true,
			wantCrackROIGtGraded:  true,
		},
		{
			name:                  "psa6",
			purchaseID:            "p4",
			campaignID:            "camp1",
			cardName:              "Charizard ex",
			certNumber:            "88888",
			grade:                 6,
			buyCents:              7500,
			feeCents:              300,
			rawMarketCents:        13000,
			gradedMarketCents:     8000,
			convRate:              0.1235,
			wantIsCandidate:       true,
			wantNegativeGradedNet: true,
			wantPositiveCrackNet:  true,
		},
		{
			// Zero fee must be treated as unset and substituted with the default
			// marketplace fee; a real 0% fee would overstate crack proceeds.
			name:                  "zero_fee_normalised",
			purchaseID:            "p5",
			campaignID:            "camp1",
			cardName:              "Blastoise",
			certNumber:            "11111",
			grade:                 8,
			buyCents:              8500,
			feeCents:              300,
			rawMarketCents:        12000,
			gradedMarketCents:     10000,
			convRate:              0, // unset — should fall back to DefaultMarketplaceFeePct
			wantCostBasis:         8800,
			wantRawMarket:         12000,
			wantIsCandidate:       true,
			wantPositiveBreakeven: true,
			wantPositiveAdvantage: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputeCrackAnalysis(
				tt.purchaseID, tt.campaignID, tt.cardName, tt.certNumber,
				tt.grade,
				tt.buyCents, tt.feeCents, tt.rawMarketCents, tt.gradedMarketCents,
				tt.convRate,
			)

			if tt.wantCostBasis != 0 && result.CostBasisCents != tt.wantCostBasis {
				t.Errorf("CostBasisCents: want %d, got %d", tt.wantCostBasis, result.CostBasisCents)
			}
			if tt.wantRawMarket != 0 && result.RawMarketCents != tt.wantRawMarket {
				t.Errorf("RawMarketCents: want %d, got %d", tt.wantRawMarket, result.RawMarketCents)
			}
			if result.IsCrackCandidate != tt.wantIsCandidate {
				t.Errorf("IsCrackCandidate: want %v, got %v", tt.wantIsCandidate, result.IsCrackCandidate)
			}
			if tt.wantPositiveBreakeven && result.BreakevenRawCents <= 0 {
				t.Error("BreakevenRawCents: want positive, got <= 0")
			}
			if tt.wantPositiveAdvantage && result.CrackAdvantage <= 0 {
				t.Error("CrackAdvantage: want positive, got <= 0")
			}
			if tt.wantNegativeGradedNet && result.GradedNetCents >= 0 {
				t.Error("GradedNetCents: want negative (graded unprofitable), got >= 0")
			}
			if tt.wantPositiveCrackNet && result.CrackNetCents <= 0 {
				t.Error("CrackNetCents: want positive, got <= 0")
			}
			if tt.wantCrackROIGtGraded && result.CrackROI <= result.GradedROI {
				t.Errorf("CrackROI (%f) should exceed GradedROI (%f)", result.CrackROI, result.GradedROI)
			}
		})
	}
}
