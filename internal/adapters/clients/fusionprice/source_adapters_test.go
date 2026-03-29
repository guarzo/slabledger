package fusionprice

import (
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardhedger"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// requireEstimate is a test helper that fetches an estimate by grade key and fails the test if nil.
func requireEstimate(t *testing.T, estimates map[string]*pricing.EstimateGradeDetail, key string) *pricing.EstimateGradeDetail {
	t.Helper()
	d, ok := estimates[key]
	if !ok || d == nil {
		t.Fatalf("expected estimates[%q] to be non-nil", key)
	}
	return d
}

// Test: Verify convertCardHedgerWithDetails produces correct fusion data and estimate details.
func TestConvertCardHedgerWithDetails(t *testing.T) {
	resp := &cardhedger.AllPricesByCardResponse{
		Prices: []cardhedger.GradePrice{
			{Grade: "PSA 10", Price: "16999.99"},
			{Grade: "PSA 9", Price: "8500.00"},
			{Grade: "PSA 8", Price: "3200.50"},
			{Grade: "Raw", Price: "525.00"},
			{Grade: "BGS 10", Price: "999.99"}, // should be skipped
		},
	}

	result, estimates := convertCardHedgerWithDetails(resp)

	// Verify PSA 10 fusion data
	psa10 := result["psa10"]
	if len(psa10) != 1 {
		t.Fatalf("expected 1 psa10 entry, got %d", len(psa10))
	}
	if psa10[0].Value != 16999.99 {
		t.Errorf("psa10 Value = %v, want 16999.99", psa10[0].Value)
	}
	if psa10[0].Source.Name != "cardhedger" {
		t.Errorf("psa10 Source.Name = %q, want \"cardhedger\"", psa10[0].Source.Name)
	}
	if psa10[0].Source.Confidence != 0.85 {
		t.Errorf("psa10 Source.Confidence = %v, want 0.85", psa10[0].Source.Confidence)
	}

	// Verify raw fusion data
	raw := result["raw"]
	if len(raw) != 1 {
		t.Fatalf("expected 1 raw entry, got %d", len(raw))
	}
	if raw[0].Value != 525.00 {
		t.Errorf("raw Value = %v, want 525.00", raw[0].Value)
	}

	// BGS 10 maps to GradeBGS10 via displayToGrade
	bgs10 := result["bgs10"]
	if len(bgs10) != 1 {
		t.Fatalf("expected 1 bgs10 entry, got %d", len(bgs10))
	}
	if bgs10[0].Value != 999.99 {
		t.Errorf("bgs10 Value = %v, want 999.99", bgs10[0].Value)
	}

	// Verify PSA 9 and PSA 8 exist
	if len(result["psa9"]) != 1 {
		t.Errorf("expected 1 psa9 entry, got %d", len(result["psa9"]))
	}
	if len(result["psa8"]) != 1 {
		t.Errorf("expected 1 psa8 entry, got %d", len(result["psa8"]))
	}

	// Verify estimate details
	estPsa10 := requireEstimate(t, estimates, "psa10")
	if estPsa10.PriceCents != 1699999 {
		t.Errorf("estimates psa10 PriceCents = %v, want 1699999", estPsa10.PriceCents)
	}
	if estPsa10.Confidence != 0.85 {
		t.Errorf("estimates psa10 Confidence = %v, want 0.85", estPsa10.Confidence)
	}
	if estPsa10.LowCents != 0 {
		t.Errorf("estimates psa10 LowCents = %v, want 0 (not enriched by batch)", estPsa10.LowCents)
	}
	if estPsa10.HighCents != 0 {
		t.Errorf("estimates psa10 HighCents = %v, want 0 (not enriched by batch)", estPsa10.HighCents)
	}

	estRaw := requireEstimate(t, estimates, "raw")
	if estRaw.PriceCents != 52500 {
		t.Errorf("estimates raw PriceCents = %v, want 52500", estRaw.PriceCents)
	}

	// BGS 10 should be in estimates (maps to GradeBGS10)
	estBgs10 := requireEstimate(t, estimates, "bgs10")
	if estBgs10.PriceCents != 99999 {
		t.Errorf("estimates bgs10 PriceCents = %v, want 99999", estBgs10.PriceCents)
	}
}

func TestTruncateAtVariant(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"single name + holo", "PIKACHU Holo", "PIKACHU"},
		{"multi-word + holo", "DARK GYARADOS Holo 1ST EDITION", "DARK GYARADOS"},
		{"reverse keyword", "MEWTWO Reverse Foil", "MEWTWO"},
		{"foil keyword", "CHARIZARD Foil Special", "CHARIZARD"},
		{"type suffix preserved", "SYLVEON ex", "SYLVEON ex"},
		{"no variant", "PIKACHU VMAX", "PIKACHU VMAX"},
		{"variant at start ignored", "Holo PIKACHU", "Holo PIKACHU"},
		{"empty", "", ""},
		{"single word", "PIKACHU", "PIKACHU"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateAtVariant(tc.input)
			if got != tc.want {
				t.Errorf("truncateAtVariant(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestShouldRejectSetMismatch(t *testing.T) {
	tests := []struct {
		name         string
		matchedSet   string
		requestedSet string
		wantReject   bool
	}{
		{
			name:         "matching sets accepted",
			matchedSet:   "Base Set",
			requestedSet: "Base Set",
			wantReject:   false,
		},
		{
			name:         "normalized matching accepted",
			matchedSet:   "2024 Pokemon Scarlet & Violet Paldean Fates",
			requestedSet: "PAF EN-PALDEAN FATES",
			wantReject:   false,
		},
		{
			name:         "completely different set rejected",
			matchedSet:   "CELEBRATIONS CLASSIC COLLECTION",
			requestedSet: "PROMO BLACK STAR",
			wantReject:   true,
		},
		{
			name:         "same card number different set rejected",
			matchedSet:   "EX EMERALD",
			requestedSet: "SWSH BLACK STAR PROMO",
			wantReject:   true,
		},
		{
			name:         "both promo sets accepted",
			matchedSet:   "2024 Pokemon Scarlet & Violet Black Star Promos",
			requestedSet: "SVP EN-SV BLACK STAR PROMO",
			wantReject:   false,
		},
		{
			name:         "empty matched set accepted",
			matchedSet:   "",
			requestedSet: "Base Set",
			wantReject:   false,
		},
		{
			name:         "empty requested set accepted",
			matchedSet:   "Base Set",
			requestedSet: "",
			wantReject:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldRejectSetMismatch(tc.matchedSet, tc.requestedSet)
			if got != tc.wantReject {
				t.Errorf("shouldRejectSetMismatch(%q, %q) = %v, want %v",
					tc.matchedSet, tc.requestedSet, got, tc.wantReject)
			}
		})
	}
}

// Test: Verify grades with unparseable or zero/negative prices are skipped.
func TestConvertCardHedgerWithDetails_InvalidPrice(t *testing.T) {
	resp := &cardhedger.AllPricesByCardResponse{
		Prices: []cardhedger.GradePrice{
			{Grade: "PSA 10", Price: "not-a-number"},
			{Grade: "PSA 9", Price: "0"},
			{Grade: "PSA 8", Price: "-50"},
			{Grade: "Raw", Price: "100.00"},
		},
	}

	result, estimates := convertCardHedgerWithDetails(resp)

	// Only raw should exist
	if len(result) != 1 {
		t.Errorf("expected 1 grade in result, got %d", len(result))
	}
	if _, ok := result["raw"]; !ok {
		t.Error("expected result[\"raw\"] to exist")
	}
	if _, ok := result["psa10"]; ok {
		t.Error("psa10 with unparseable price should not be in result")
	}
	if _, ok := result["psa9"]; ok {
		t.Error("psa9 with zero price should not be in result")
	}
	if _, ok := result["psa8"]; ok {
		t.Error("psa8 with negative price should not be in result")
	}

	// Only raw should be in estimates
	if len(estimates) != 1 {
		t.Errorf("expected 1 entry in estimates, got %d", len(estimates))
	}
	if _, ok := estimates["raw"]; !ok {
		t.Error("expected estimates[\"raw\"] to exist")
	}
}
