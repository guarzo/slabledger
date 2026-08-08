package demand_test

import (
	"encoding/json"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/demand"
)

// TestCharacterDemand_DecodesLegacyByEraBlob pins the backward-compatibility
// behaviors the persisted rows depend on: an obsolete by_era.data_quality key
// (no longer a field, per SLA-61) is ignored rather than rejected, an omitted
// total_search_clicks defaults to zero rather than failing to decode, and an
// absent by_era key leaves a nil map rather than erroring.
func TestCharacterDemand_DecodesLegacyByEraBlob(t *testing.T) {
	tests := []struct {
		name              string
		blob              string
		wantSearchClicks  int
		wantEra           string // "" = expect no by_era entry
		wantEraCards      int
		wantEraScore      float64
		wantEraSearchClks int
	}{
		{
			name: "obsolete by_era.data_quality is ignored and omitted search clicks default to zero",
			blob: `{
				"character_name": "Umbreon",
				"card_count": 10,
				"avg_demand_score": 0.9,
				"total_views": 400,
				"total_wishlist_adds": 20,
				"data_quality": "full",
				"by_era": {
					"sword_shield": {
						"card_count": 6,
						"avg_demand_score": 0.95,
						"total_views": 240,
						"total_wishlist_adds": 12,
						"data_quality": "full"
					}
				}
			}`,
			wantSearchClicks: 0,
			wantEra:          "sword_shield",
			wantEraCards:     6,
			wantEraScore:     0.95,
		},
		{
			name: "present search clicks are preserved at both levels",
			blob: `{
				"character_name": "Umbreon",
				"total_search_clicks": 7,
				"by_era": {
					"sword_shield": {
						"card_count": 6,
						"avg_demand_score": 0.95,
						"total_search_clicks": 3
					}
				}
			}`,
			wantSearchClicks:  7,
			wantEra:           "sword_shield",
			wantEraCards:      6,
			wantEraScore:      0.95,
			wantEraSearchClks: 3,
		},
		{
			name:             "absent by_era leaves an empty map",
			blob:             `{"character_name":"Umbreon","card_count":10}`,
			wantSearchClicks: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got demand.CharacterDemand
			if err := json.Unmarshal([]byte(tc.blob), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if got.TotalSearchClicks != tc.wantSearchClicks {
				t.Errorf("TotalSearchClicks = %d, want %d", got.TotalSearchClicks, tc.wantSearchClicks)
			}
			if tc.wantEra == "" {
				if len(got.ByEra) != 0 {
					t.Errorf("ByEra = %+v, want empty", got.ByEra)
				}
				return
			}
			era, ok := got.ByEra[tc.wantEra]
			if !ok {
				t.Fatalf("by_era[%s] missing", tc.wantEra)
			}
			if era.TotalSearchClicks != tc.wantEraSearchClks {
				t.Errorf("era TotalSearchClicks = %d, want %d", era.TotalSearchClicks, tc.wantEraSearchClks)
			}
			if era.CardCount != tc.wantEraCards || era.AvgDemandScore != tc.wantEraScore {
				t.Errorf("era decoded wrong: %+v", era)
			}
		})
	}
}

// TestCharacterVelocity_DecodesLegacyFlatBlob pins that the two fields the
// scheduler used to marshal from the raw DH struct — sell_through and
// avg_days_to_sell — are silently dropped, and that an absent by_grade key
// decodes to a nil map rather than an empty one.
func TestCharacterVelocity_DecodesLegacyFlatBlob(t *testing.T) {
	tests := []struct {
		name           string
		blob           string
		wantSampleSize int
		wantMedian     *float64 // nil = expect a nil pointer
		wantByGrade    map[string]demand.VelocityTierStat
	}{
		{
			name: "obsolete sell_through and avg_days_to_sell keys are dropped",
			blob: `{
				"median_days_to_sell": 9.5,
				"sample_size": 120,
				"sell_through": {},
				"avg_days_to_sell": 8.1
			}`,
			wantSampleSize: 120,
			wantMedian:     f64Ptr(9.5),
		},
		{
			name:           "absent median_days_to_sell stays nil",
			blob:           `{"sample_size": 4}`,
			wantSampleSize: 4,
		},
		{
			name:           "present by_grade decodes into the tier map",
			blob:           `{"sample_size":4,"by_grade":{"10":{"median_days":6.5,"sample_size":30}}}`,
			wantSampleSize: 4,
			wantByGrade:    map[string]demand.VelocityTierStat{"10": {MedianDays: 6.5, SampleSize: 30}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got demand.CharacterVelocity
			if err := json.Unmarshal([]byte(tc.blob), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if got.SampleSize != tc.wantSampleSize {
				t.Errorf("SampleSize = %d, want %d", got.SampleSize, tc.wantSampleSize)
			}
			switch {
			case tc.wantMedian == nil && got.MedianDaysToSell != nil:
				t.Errorf("MedianDaysToSell = %v, want nil", *got.MedianDaysToSell)
			case tc.wantMedian != nil && (got.MedianDaysToSell == nil || *got.MedianDaysToSell != *tc.wantMedian):
				t.Errorf("MedianDaysToSell = %v, want %v", got.MedianDaysToSell, *tc.wantMedian)
			}
			if tc.wantByGrade == nil {
				if got.ByGrade != nil {
					t.Errorf("ByGrade = %v, want nil (absent key)", got.ByGrade)
				}
				return
			}
			if len(got.ByGrade) != len(tc.wantByGrade) {
				t.Fatalf("ByGrade = %+v, want %+v", got.ByGrade, tc.wantByGrade)
			}
			for grade, want := range tc.wantByGrade {
				if got.ByGrade[grade] != want {
					t.Errorf("ByGrade[%q] = %+v, want %+v", grade, got.ByGrade[grade], want)
				}
			}
		})
	}
}

// TestCharacterSaturation_FlatVsNestedLegacyShapes pins the invariant that
// active_listing_count is decoded from the flat shape, not the nested one: a
// flat blob (today's correct shape, post-fix) decodes fully, but the
// pre-existing nested shape the scheduler used to write for saturation
// decodes ActiveListingCount to zero. This is not new lossy behavior — it
// pins what already-persisted rows contain until the next scheduler refresh
// overwrites them with the flat shape.
func TestCharacterSaturation_FlatVsNestedLegacyShapes(t *testing.T) {
	tests := []struct {
		name string
		blob string
		want int
	}{
		{
			name: "flat shape decodes the count",
			blob: `{"active_listing_count":42,"computed_at":"2026-04-15T03:00:00Z"}`,
			want: 42,
		},
		{
			name: "legacy nested shape loses the count (pre-existing rows)",
			blob: `{"character_name":"Pikachu","saturation":{"active_listing_count":42}}`,
			want: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got demand.CharacterSaturation
			if err := json.Unmarshal([]byte(tc.blob), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.ActiveListingCount != tc.want {
				t.Errorf("ActiveListingCount = %d, want %d", got.ActiveListingCount, tc.want)
			}
		})
	}
}

// TestCharacterVelocity_RoundTrip covers the pointer fields and both tier
// maps surviving a marshal -> unmarshal cycle unchanged.
func TestCharacterVelocity_RoundTrip(t *testing.T) {
	medianDays := 9.5
	changePct := 14.2
	avgDaily := 3.2
	sellThrough := 0.6
	vol7 := 21
	vol30 := 90
	supply := 12

	want := demand.CharacterVelocity{
		MedianDaysToSell:   &medianDays,
		SampleSize:         120,
		VelocityChangePct:  &changePct,
		AvgDailySales:      &avgDaily,
		SellThroughRate30d: &sellThrough,
		SalesVolume7d:      &vol7,
		SalesVolume30d:     &vol30,
		SupplyCount:        &supply,
		ByGrade: map[string]demand.VelocityTierStat{
			"9":  {MedianDays: 8.0, SampleSize: 40},
			"10": {MedianDays: 6.5, SampleSize: 30},
		},
		ByPriceTier: map[string]demand.VelocityTierStat{
			"low":  {MedianDays: 12.0, SampleSize: 20},
			"high": {MedianDays: 4.0, SampleSize: 15},
		},
	}

	blob, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got demand.CharacterVelocity
	if err := json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if *got.MedianDaysToSell != *want.MedianDaysToSell {
		t.Errorf("MedianDaysToSell = %v, want %v", *got.MedianDaysToSell, *want.MedianDaysToSell)
	}
	if *got.VelocityChangePct != *want.VelocityChangePct {
		t.Errorf("VelocityChangePct = %v, want %v", *got.VelocityChangePct, *want.VelocityChangePct)
	}
	if *got.AvgDailySales != *want.AvgDailySales {
		t.Errorf("AvgDailySales = %v, want %v", *got.AvgDailySales, *want.AvgDailySales)
	}
	if *got.SellThroughRate30d != *want.SellThroughRate30d {
		t.Errorf("SellThroughRate30d = %v, want %v", *got.SellThroughRate30d, *want.SellThroughRate30d)
	}
	if *got.SalesVolume7d != *want.SalesVolume7d {
		t.Errorf("SalesVolume7d = %v, want %v", *got.SalesVolume7d, *want.SalesVolume7d)
	}
	if *got.SalesVolume30d != *want.SalesVolume30d {
		t.Errorf("SalesVolume30d = %v, want %v", *got.SalesVolume30d, *want.SalesVolume30d)
	}
	if *got.SupplyCount != *want.SupplyCount {
		t.Errorf("SupplyCount = %v, want %v", *got.SupplyCount, *want.SupplyCount)
	}
	if len(got.ByGrade) != 2 || got.ByGrade["9"] != want.ByGrade["9"] || got.ByGrade["10"] != want.ByGrade["10"] {
		t.Errorf("ByGrade = %+v, want %+v", got.ByGrade, want.ByGrade)
	}
	if len(got.ByPriceTier) != 2 || got.ByPriceTier["low"] != want.ByPriceTier["low"] || got.ByPriceTier["high"] != want.ByPriceTier["high"] {
		t.Errorf("ByPriceTier = %+v, want %+v", got.ByPriceTier, want.ByPriceTier)
	}
}
