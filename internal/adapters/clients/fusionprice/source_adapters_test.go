package fusionprice

import (
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardhedger"
	"github.com/guarzo/slabledger/internal/adapters/clients/pokemonprice"
	"github.com/guarzo/slabledger/internal/domain/fusion"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// requireDetail is a test helper that fetches a detail by grade key and fails the test if nil.
func requireDetail(t *testing.T, details map[string]*pricing.EbayGradeDetail, key string) *pricing.EbayGradeDetail {
	t.Helper()
	d, ok := details[key]
	if !ok || d == nil {
		t.Fatalf("expected ebayDetails[%q] to be non-nil", key)
	}
	return d
}

// requireEstimate is a test helper that fetches an estimate by grade key and fails the test if nil.
func requireEstimate(t *testing.T, estimates map[string]*pricing.EstimateGradeDetail, key string) *pricing.EstimateGradeDetail {
	t.Helper()
	d, ok := estimates[key]
	if !ok || d == nil {
		t.Fatalf("expected estimates[%q] to be non-nil", key)
	}
	return d
}

// convertPokemonPriceToFusionData is a test helper that wraps convertPokemonPriceWithDetails.
func convertPokemonPriceToFusionData(ppData *pokemonprice.CardPriceData) map[string][]fusion.PriceData {
	fusionData, _, _ := convertPokemonPriceWithDetails(ppData)
	return fusionData
}

func TestConvertPokemonPriceToFusionData_WithEbay(t *testing.T) {
	now := time.Now()
	high := "high"
	medium := "medium"
	low := "low"
	trendUp := "up"
	trendDown := "down"

	ppData := &pokemonprice.CardPriceData{
		Name:       "Charizard",
		SetName:    "Base Set",
		CardNumber: "004/102",
		Prices: pokemonprice.PriceInfo{
			Market:  525.82,
			Low:     200,
			Sellers: 53,
		},
		UpdatedAt: now,
		Ebay: &pokemonprice.EbayData{
			UpdatedAt:  now,
			TotalSales: 177,
			SalesByGrade: map[string]pokemonprice.GradeSalesData{
				"ungraded": {
					Count:       63,
					MedianPrice: 485,
					MinPrice:    175,
					MaxPrice:    25000,
					MarketTrend: &trendDown,
					SmartMarketPrice: pokemonprice.SmartMarketPrice{
						Price:      487,
						Confidence: low,
						Method:     "7day_filtered_weighted",
						DaysUsed:   7,
					},
				},
				"psa8": {
					Count:       26,
					MedianPrice: 926.73,
					MinPrice:    650,
					MaxPrice:    2900,
					MarketTrend: &trendUp,
					SmartMarketPrice: pokemonprice.SmartMarketPrice{
						Price:      1174.99,
						Confidence: high,
						Method:     "30day_filtered_weighted",
						DaysUsed:   30,
					},
				},
				"psa9": {
					Count:       14,
					MedianPrice: 2487.50,
					MinPrice:    1725,
					MaxPrice:    21020.04,
					MarketTrend: &trendDown,
					SmartMarketPrice: pokemonprice.SmartMarketPrice{
						Price:      2500,
						Confidence: low,
						Method:     "30day_filtered_weighted",
						DaysUsed:   30,
					},
				},
				"psa10": {
					Count:       1,
					MedianPrice: 17500,
					MinPrice:    17500,
					MaxPrice:    17500,
					SmartMarketPrice: pokemonprice.SmartMarketPrice{
						Price:      14875,
						Confidence: low,
						Method:     "all_filtered_weighted",
						DaysUsed:   129,
					},
				},
				"psa5": {
					Count:       17,
					MarketTrend: &trendDown,
					SmartMarketPrice: pokemonprice.SmartMarketPrice{
						Price:      318.75,
						Confidence: medium,
						Method:     "14day_filtered_weighted",
						DaysUsed:   14,
					},
				},
				// Non-PSA grades should be skipped
				"bgs4": {
					Count: 1,
					SmartMarketPrice: pokemonprice.SmartMarketPrice{
						Price:      299,
						Confidence: low,
					},
				},
				"cgc8": {
					Count: 3,
					SmartMarketPrice: pokemonprice.SmartMarketPrice{
						Price:      702.5,
						Confidence: medium,
					},
				},
			},
			SalesVelocity: pokemonprice.SalesVelocity{
				DailyAverage:  1.07,
				WeeklyAverage: 7.44,
				MonthlyTotal:  32,
			},
		},
	}

	result := convertPokemonPriceToFusionData(ppData)

	// Should have raw, psa5, psa8, psa9, psa10
	expectedGrades := []string{"raw", "psa5", "psa8", "psa9", "psa10"}
	for _, grade := range expectedGrades {
		if _, ok := result[grade]; !ok {
			t.Errorf("expected grade %q in result, but not found", grade)
		}
	}

	// Non-PSA grades should NOT be present
	for _, skipGrade := range []string{"bgs4", "cgc8"} {
		if _, ok := result[skipGrade]; ok {
			t.Errorf("non-PSA grade %q should not be in result", skipGrade)
		}
	}

	// Verify raw uses eBay smartMarketPrice (not TCGPlayer market)
	rawData := result["raw"]
	if len(rawData) != 1 {
		t.Fatalf("expected 1 raw price data point, got %d", len(rawData))
	}
	if rawData[0].Value != 487 {
		t.Errorf("raw price = %v, want 487 (eBay smartMarketPrice)", rawData[0].Value)
	}
	if rawData[0].Source.Volume != 63 {
		t.Errorf("raw volume = %d, want 63", rawData[0].Source.Volume)
	}

	// Verify PSA 8
	psa8Data := result["psa8"]
	if len(psa8Data) != 1 {
		t.Fatalf("expected 1 psa8 price data point, got %d", len(psa8Data))
	}
	if psa8Data[0].Value != 1174.99 {
		t.Errorf("psa8 price = %v, want 1174.99", psa8Data[0].Value)
	}
	if psa8Data[0].Source.Confidence != 0.95 {
		t.Errorf("psa8 confidence = %v, want 0.95 (high)", psa8Data[0].Source.Confidence)
	}
	if psa8Data[0].Source.Volume != 26 {
		t.Errorf("psa8 volume = %d, want 26", psa8Data[0].Source.Volume)
	}

	// Verify PSA 10
	psa10Data := result["psa10"]
	if len(psa10Data) != 1 {
		t.Fatalf("expected 1 psa10 price data point, got %d", len(psa10Data))
	}
	if psa10Data[0].Value != 14875 {
		t.Errorf("psa10 price = %v, want 14875", psa10Data[0].Value)
	}
	if psa10Data[0].Source.Confidence != 0.60 {
		t.Errorf("psa10 confidence = %v, want 0.60 (low)", psa10Data[0].Source.Confidence)
	}
}

func TestConvertPokemonPriceToFusionData_WithoutEbay(t *testing.T) {
	ppData := &pokemonprice.CardPriceData{
		Name:    "Pikachu",
		SetName: "Base Set",
		Prices: pokemonprice.PriceInfo{
			Market:  15.50,
			Low:     10.00,
			Sellers: 100,
		},
		UpdatedAt: time.Now(),
		Ebay:      nil, // No eBay data (includeEbay not used)
	}

	result := convertPokemonPriceToFusionData(ppData)

	// Should only have raw price from TCGPlayer
	if len(result) != 1 {
		t.Errorf("expected 1 grade (raw), got %d", len(result))
	}

	rawData := result["raw"]
	if len(rawData) != 1 {
		t.Fatalf("expected 1 raw price data point, got %d", len(rawData))
	}
	if rawData[0].Value != 15.50 {
		t.Errorf("raw price = %v, want 15.50 (TCGPlayer market)", rawData[0].Value)
	}
	if rawData[0].Source.Volume != 100 {
		t.Errorf("raw volume = %d, want 100 (sellers count)", rawData[0].Source.Volume)
	}
}

func TestConvertPokemonPriceToFusionData_EmptyEbay(t *testing.T) {
	ppData := &pokemonprice.CardPriceData{
		Name:    "Pikachu",
		SetName: "Base Set",
		Prices: pokemonprice.PriceInfo{
			Market: 15.50,
		},
		UpdatedAt: time.Now(),
		Ebay: &pokemonprice.EbayData{
			SalesByGrade: map[string]pokemonprice.GradeSalesData{},
		},
	}

	result := convertPokemonPriceToFusionData(ppData)

	// Should only have raw price from TCGPlayer (no eBay grades to override)
	if len(result) != 1 {
		t.Errorf("expected 1 grade (raw), got %d", len(result))
	}
	if result["raw"][0].Value != 15.50 {
		t.Errorf("raw price = %v, want 15.50", result["raw"][0].Value)
	}
}

func TestConfidenceToScore(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"high", 0.95},
		{"medium", 0.80},
		{"low", 0.60},
		{"", 0.50},
		{"unknown", 0.50},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := confidenceToScore(tt.input)
			if got != tt.want {
				t.Errorf("confidenceToScore(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// Test 1: Verify convertPokemonPriceWithDetails populates the ebayDetails map correctly.
func TestConvertPokemonPriceWithDetails_EbayGradeDetail(t *testing.T) {
	now := time.Now()
	high := "high"
	medium := "medium"
	low := "low"
	trendUp := "up"
	trendDown := "down"
	sevenDay := 450.0
	dailyVol := 2.5

	ppData := &pokemonprice.CardPriceData{
		Name:       "Charizard",
		SetName:    "Base Set",
		CardNumber: "004/102",
		Prices: pokemonprice.PriceInfo{
			Market:  525.82,
			Low:     200,
			Sellers: 53,
		},
		UpdatedAt: now,
		Ebay: &pokemonprice.EbayData{
			UpdatedAt:  now,
			TotalSales: 177,
			SalesByGrade: map[string]pokemonprice.GradeSalesData{
				"ungraded": {
					Count:           63,
					MedianPrice:     485,
					MinPrice:        175,
					MaxPrice:        25000,
					MarketTrend:     &trendDown,
					MarketPrice7Day: &sevenDay,
					DailyVolume7Day: &dailyVol,
					SmartMarketPrice: pokemonprice.SmartMarketPrice{
						Price:      487,
						Confidence: low,
						Method:     "7day_filtered_weighted",
						DaysUsed:   7,
					},
				},
				"psa8": {
					Count:       26,
					MedianPrice: 926.73,
					MinPrice:    650,
					MaxPrice:    2900,
					MarketTrend: &trendUp,
					SmartMarketPrice: pokemonprice.SmartMarketPrice{
						Price:      1174.99,
						Confidence: high,
						Method:     "30day_filtered_weighted",
						DaysUsed:   30,
					},
				},
				"psa9": {
					Count:       14,
					MedianPrice: 2487.50,
					MinPrice:    1725,
					MaxPrice:    21020.04,
					MarketTrend: &trendDown,
					SmartMarketPrice: pokemonprice.SmartMarketPrice{
						Price:      2500,
						Confidence: low,
						Method:     "30day_filtered_weighted",
						DaysUsed:   30,
					},
				},
				"psa10": {
					Count:       1,
					MedianPrice: 17500,
					MinPrice:    17500,
					MaxPrice:    17500,
					SmartMarketPrice: pokemonprice.SmartMarketPrice{
						Price:      14875,
						Confidence: low,
						Method:     "all_filtered_weighted",
						DaysUsed:   129,
					},
				},
				"psa5": {
					Count:       17,
					MarketTrend: &trendDown,
					SmartMarketPrice: pokemonprice.SmartMarketPrice{
						Price:      318.75,
						Confidence: medium,
						Method:     "14day_filtered_weighted",
						DaysUsed:   14,
					},
				},
				"bgs4": {
					Count: 1,
					SmartMarketPrice: pokemonprice.SmartMarketPrice{
						Price:      299,
						Confidence: low,
					},
				},
				"cgc8": {
					Count: 3,
					SmartMarketPrice: pokemonprice.SmartMarketPrice{
						Price:      702.5,
						Confidence: medium,
					},
				},
			},
			SalesVelocity: pokemonprice.SalesVelocity{
				DailyAverage:  1.07,
				WeeklyAverage: 7.44,
				MonthlyTotal:  32,
			},
		},
	}

	_, ebayDetails, _ := convertPokemonPriceWithDetails(ppData)

	// Verify raw detail
	rawDetail := requireDetail(t, ebayDetails, "raw")
	if rawDetail.PriceCents != 48700 {
		t.Errorf("raw PriceCents = %v, want 48700", rawDetail.PriceCents)
	}
	if rawDetail.Confidence != "low" {
		t.Errorf("raw Confidence = %q, want \"low\"", rawDetail.Confidence)
	}
	if rawDetail.SalesCount != 63 {
		t.Errorf("raw SalesCount = %d, want 63", rawDetail.SalesCount)
	}
	if rawDetail.MedianCents != 48500 {
		t.Errorf("raw MedianCents = %v, want 48500", rawDetail.MedianCents)
	}
	if rawDetail.MinCents != 17500 {
		t.Errorf("raw MinCents = %v, want 17500", rawDetail.MinCents)
	}
	if rawDetail.MaxCents != 2500000 {
		t.Errorf("raw MaxCents = %v, want 2500000", rawDetail.MaxCents)
	}
	if rawDetail.Trend != "down" {
		t.Errorf("raw Trend = %q, want \"down\"", rawDetail.Trend)
	}
	if rawDetail.Avg7DayCents != 45000 {
		t.Errorf("raw Avg7DayCents = %v, want 45000", rawDetail.Avg7DayCents)
	}
	if rawDetail.Volume7Day != 2.5 {
		t.Errorf("raw Volume7Day = %v, want 2.5", rawDetail.Volume7Day)
	}

	// Verify PSA 8 detail
	psa8Detail := requireDetail(t, ebayDetails, "psa8")
	if psa8Detail.PriceCents != 117499 {
		t.Errorf("psa8 PriceCents = %v, want 117499", psa8Detail.PriceCents)
	}
	if psa8Detail.Confidence != "high" {
		t.Errorf("psa8 Confidence = %q, want \"high\"", psa8Detail.Confidence)
	}
	if psa8Detail.SalesCount != 26 {
		t.Errorf("psa8 SalesCount = %d, want 26", psa8Detail.SalesCount)
	}
	if psa8Detail.Trend != "up" {
		t.Errorf("psa8 Trend = %q, want \"up\"", psa8Detail.Trend)
	}

	// Verify PSA 9 detail
	psa9Detail := requireDetail(t, ebayDetails, "psa9")
	if psa9Detail.PriceCents != 250000 {
		t.Errorf("psa9 PriceCents = %v, want 250000", psa9Detail.PriceCents)
	}
	if psa9Detail.Confidence != "low" {
		t.Errorf("psa9 Confidence = %q, want \"low\"", psa9Detail.Confidence)
	}
	if psa9Detail.Trend != "down" {
		t.Errorf("psa9 Trend = %q, want \"down\"", psa9Detail.Trend)
	}

	// Verify PSA 10 detail
	psa10Detail := requireDetail(t, ebayDetails, "psa10")
	if psa10Detail.PriceCents != 1487500 {
		t.Errorf("psa10 PriceCents = %v, want 1487500", psa10Detail.PriceCents)
	}
	if psa10Detail.SalesCount != 1 {
		t.Errorf("psa10 SalesCount = %d, want 1", psa10Detail.SalesCount)
	}

	// Verify PSA 5 detail
	psa5Detail := requireDetail(t, ebayDetails, "psa5")
	if psa5Detail.PriceCents != 31875 {
		t.Errorf("psa5 PriceCents = %v, want 31875", psa5Detail.PriceCents)
	}

	// Non-PSA grades should NOT exist
	if _, ok := ebayDetails["bgs4"]; ok {
		t.Error("non-PSA grade \"bgs4\" should not be in ebayDetails")
	}
	if _, ok := ebayDetails["cgc8"]; ok {
		t.Error("non-PSA grade \"cgc8\" should not be in ebayDetails")
	}
}

// Test 2: Verify sales velocity (3rd return value) extraction.
func TestConvertPokemonPriceWithDetails_Velocity(t *testing.T) {
	t.Run("normal velocity", func(t *testing.T) {
		ppData := &pokemonprice.CardPriceData{
			Prices:    pokemonprice.PriceInfo{Market: 10},
			UpdatedAt: time.Now(),
			Ebay: &pokemonprice.EbayData{
				SalesByGrade: map[string]pokemonprice.GradeSalesData{},
				SalesVelocity: pokemonprice.SalesVelocity{
					DailyAverage:  1.07,
					WeeklyAverage: 7.44,
					MonthlyTotal:  32,
				},
			},
		}
		_, _, velocity := convertPokemonPriceWithDetails(ppData)
		if velocity == nil {
			t.Fatal("expected non-nil velocity")
			return // unreachable but satisfies staticcheck SA5011
		}
		if velocity.DailyAverage != 1.07 {
			t.Errorf("DailyAverage = %v, want 1.07", velocity.DailyAverage)
		}
		if velocity.WeeklyAverage != 7.44 {
			t.Errorf("WeeklyAverage = %v, want 7.44", velocity.WeeklyAverage)
		}
		if velocity.MonthlyTotal != 32 {
			t.Errorf("MonthlyTotal = %d, want 32", velocity.MonthlyTotal)
		}
	})

	t.Run("zero velocity", func(t *testing.T) {
		ppData := &pokemonprice.CardPriceData{
			Prices:    pokemonprice.PriceInfo{Market: 10},
			UpdatedAt: time.Now(),
			Ebay: &pokemonprice.EbayData{
				SalesByGrade: map[string]pokemonprice.GradeSalesData{},
				SalesVelocity: pokemonprice.SalesVelocity{
					DailyAverage:  0,
					WeeklyAverage: 0,
					MonthlyTotal:  0,
				},
			},
		}
		_, _, velocity := convertPokemonPriceWithDetails(ppData)
		if velocity != nil {
			t.Errorf("expected nil velocity for zero values, got %+v", velocity)
		}
	})

	t.Run("nil ebay", func(t *testing.T) {
		ppData := &pokemonprice.CardPriceData{
			Prices:    pokemonprice.PriceInfo{Market: 10},
			UpdatedAt: time.Now(),
			Ebay:      nil,
		}
		_, _, velocity := convertPokemonPriceWithDetails(ppData)
		if velocity != nil {
			t.Errorf("expected nil velocity when Ebay is nil, got %+v", velocity)
		}
	})
}

// Test 3: Verify nil pointer fields produce zero-value fields (no panic).
func TestConvertPokemonPriceWithDetails_NilPointerFields(t *testing.T) {
	low := "low"
	ppData := &pokemonprice.CardPriceData{
		Prices:    pokemonprice.PriceInfo{Market: 10},
		UpdatedAt: time.Now(),
		Ebay: &pokemonprice.EbayData{
			SalesByGrade: map[string]pokemonprice.GradeSalesData{
				"psa9": {
					Count:           5,
					MarketTrend:     nil,
					MarketPrice7Day: nil,
					DailyVolume7Day: nil,
					SmartMarketPrice: pokemonprice.SmartMarketPrice{
						Price:      100,
						Confidence: low,
					},
				},
			},
		},
	}

	_, ebayDetails, _ := convertPokemonPriceWithDetails(ppData)

	detail := requireDetail(t, ebayDetails, "psa9")
	if detail.Trend != "" {
		t.Errorf("Trend = %q, want \"\" for nil MarketTrend pointer", detail.Trend)
	}
	if detail.Avg7DayCents != 0 {
		t.Errorf("Avg7DayCents = %v, want 0 for nil MarketPrice7Day pointer", detail.Avg7DayCents)
	}
	if detail.Volume7Day != 0 {
		t.Errorf("Volume7Day = %v, want 0 for nil DailyVolume7Day pointer", detail.Volume7Day)
	}
}

// Test 4: Verify convertCardHedgerWithDetails produces correct fusion data and estimate details.
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

// Test 7: Verify grades with unparseable or zero/negative prices are skipped.
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
