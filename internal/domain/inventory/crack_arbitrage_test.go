package inventory

import "testing"

func Test_computeCrackAnalysis(t *testing.T) {
	result := computeCrackAnalysis(
		"p1", "camp1", "Charizard", "12345", 8,
		8500, 300, 12000, 10000,
		0.1235,
	)

	if result.CostBasisCents != 8800 {
		t.Errorf("expected cost basis 8800, got %d", result.CostBasisCents)
	}
	if result.BreakevenRawCents <= 0 {
		t.Error("expected positive breakeven raw price")
	}
	if result.RawMarketCents != 12000 {
		t.Errorf("expected raw market 12000, got %d", result.RawMarketCents)
	}
	// With raw at 12000 and graded at 10000, cracking should be more profitable
	if !result.IsCrackCandidate {
		t.Error("expected card to be a crack candidate")
	}
	if result.CrackAdvantage <= 0 {
		t.Error("expected positive crack advantage")
	}
}

func TestCrackAnalysis_NotCandidate(t *testing.T) {
	result := computeCrackAnalysis(
		"p2", "camp1", "Pikachu", "67890", 8,
		5000, 300, 3000, 8000,
		0.1235,
	)

	// Raw price is low, graded is much higher - should NOT be crack candidate
	if result.IsCrackCandidate {
		t.Error("expected card NOT to be a crack candidate")
	}
}

func TestCrackAnalysis_PSA7(t *testing.T) {
	// PSA 7 modern card: raw NM should exceed graded PSA 7 value
	result := computeCrackAnalysis(
		"p3", "camp1", "Umbreon VMAX", "99999", 7,
		14700, 300, 25000, 20000, // buy $147 + $3 fee, raw $250, graded $200
		0.1235,
	)

	if result.CostBasisCents != 15000 {
		t.Errorf("expected cost basis 15000, got %d", result.CostBasisCents)
	}
	if !result.IsCrackCandidate {
		t.Error("expected PSA 7 card to be a crack candidate (raw > graded)")
	}
	if result.CrackAdvantage <= 0 {
		t.Error("expected positive crack advantage")
	}
	// Verify crack ROI is higher than graded ROI
	if result.CrackROI <= result.GradedROI {
		t.Errorf("crack ROI (%f) should exceed graded ROI (%f)", result.CrackROI, result.GradedROI)
	}
}

func TestCrackAnalysis_PSA6(t *testing.T) {
	// PSA 6 card: lower grade, even bigger spread expected
	result := computeCrackAnalysis(
		"p4", "camp1", "Charizard ex", "88888", 6,
		7500, 300, 13000, 8000, // buy $75 + $3 fee, raw $130, graded $80
		0.1235,
	)

	if !result.IsCrackCandidate {
		t.Error("expected PSA 6 card to be a crack candidate")
	}
	// Verify the math: crackNet = 13000 - 1606 - 7800 = 3594
	// gradedNet = 8000 - 988 - 7800 = -788
	if result.GradedNetCents >= 0 {
		t.Error("expected negative graded net (graded is unprofitable)")
	}
	if result.CrackNetCents <= 0 {
		t.Error("expected positive crack net")
	}
}
