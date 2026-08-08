package postgres

import (
	"database/sql"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/demand"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestScanCardCacheRow(t *testing.T) {
	fetchedAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	demandComputedAt := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	analyticsComputedAt := time.Date(2026, 8, 1, 7, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		values []any
		assert func(t *testing.T, row *demand.CardCache)
	}{
		{
			name: "all columns populated",
			values: []any{
				"sv1-25", "30d",
				sql.NullString{String: "Charizard", Valid: true},
				sql.NullFloat64{Float64: 0.82, Valid: true},
				sql.NullString{String: "full", Valid: true},
				sql.NullTime{Time: analyticsComputedAt, Valid: true},
				sql.NullTime{Time: demandComputedAt, Valid: true},
				fetchedAt,
			},
			assert: func(t *testing.T, row *demand.CardCache) {
				if row.CardID != "sv1-25" {
					t.Errorf("CardID = %q, want %q", row.CardID, "sv1-25")
				}
				if row.Window != "30d" {
					t.Errorf("Window = %q, want %q", row.Window, "30d")
				}
				if row.CharacterName == nil || *row.CharacterName != "Charizard" {
					t.Errorf("CharacterName = %v, want \"Charizard\"", row.CharacterName)
				}
				if row.DemandScore == nil || *row.DemandScore != 0.82 {
					t.Errorf("DemandScore = %v, want 0.82", row.DemandScore)
				}
				if row.DemandDataQuality == nil || *row.DemandDataQuality != "full" {
					t.Errorf("DemandDataQuality = %v, want \"full\"", row.DemandDataQuality)
				}
				if row.AnalyticsComputedAt == nil || !row.AnalyticsComputedAt.Equal(analyticsComputedAt) {
					t.Errorf("AnalyticsComputedAt = %v, want %v", row.AnalyticsComputedAt, analyticsComputedAt)
				}
				if row.DemandComputedAt == nil || !row.DemandComputedAt.Equal(demandComputedAt) {
					t.Errorf("DemandComputedAt = %v, want %v", row.DemandComputedAt, demandComputedAt)
				}
				if !row.FetchedAt.Equal(fetchedAt) {
					t.Errorf("FetchedAt = %v, want %v", row.FetchedAt, fetchedAt)
				}
			},
		},
		{
			name: "every nullable column NULL",
			values: []any{
				"sv1-26", "7d",
				sql.NullString{},
				sql.NullFloat64{},
				sql.NullString{},
				sql.NullTime{},
				sql.NullTime{},
				fetchedAt,
			},
			assert: func(t *testing.T, row *demand.CardCache) {
				if row.CharacterName != nil {
					t.Errorf("CharacterName = %v, want nil", row.CharacterName)
				}
				if row.DemandScore != nil {
					t.Errorf("DemandScore = %v, want nil", row.DemandScore)
				}
				if row.DemandDataQuality != nil {
					t.Errorf("DemandDataQuality = %v, want nil", row.DemandDataQuality)
				}
				if row.AnalyticsComputedAt != nil {
					t.Errorf("AnalyticsComputedAt = %v, want nil", row.AnalyticsComputedAt)
				}
				if row.DemandComputedAt != nil {
					t.Errorf("DemandComputedAt = %v, want nil", row.DemandComputedAt)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row, err := scanCardCacheRow(&mocks.RowScanner{Values: tt.values})
			if err != nil {
				t.Fatalf("scanCardCacheRow: %v", err)
			}
			tt.assert(t, row)
		})
	}
}

func TestScanCharacterCacheRow(t *testing.T) {
	fetchedAt := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	demandComputedAt := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	analyticsComputedAt := time.Date(2026, 8, 1, 7, 0, 0, 0, time.UTC)

	validDemand := `{"character_name":"Pikachu","card_count":5,"avg_demand_score":0.8,` +
		`"total_views":100,"total_search_clicks":10,"total_wishlist_adds":3,` +
		`"data_quality":"full","computed_at":"2026-08-01T06:00:00Z"}`
	validVelocity := `{"sample_size":12}`
	validSaturation := `{"active_listing_count":42,"computed_at":"2026-08-01T07:00:00Z"}`

	tests := []struct {
		name              string
		values            []any
		wantDemandNil     bool
		wantVelocityNil   bool
		wantSaturationNil bool
		wantMalformed     []demand.MalformedPayload
		// wantSampleSize and wantActiveListingCount, when non-nil, assert the
		// decoded values on the valid-payload case — otherwise this test only
		// checks nil-ness and a regression like reverting
		// mapCharacterSaturation to marshal the whole nested entry would pass
		// silently (SLA-41 item 6).
		wantSampleSize         *int
		wantActiveListingCount *int
	}{
		{
			name: "all three payload columns valid",
			values: []any{
				"Pikachu", "7d",
				sql.NullString{String: validDemand, Valid: true},
				sql.NullString{String: validVelocity, Valid: true},
				sql.NullString{String: validSaturation, Valid: true},
				sql.NullTime{Time: demandComputedAt, Valid: true},
				sql.NullTime{Time: analyticsComputedAt, Valid: true},
				fetchedAt,
			},
			wantSampleSize:         intPtr(12),
			wantActiveListingCount: intPtr(42),
		},
		{
			name: "all three NULL",
			values: []any{
				"Pikachu", "7d",
				sql.NullString{},
				sql.NullString{},
				sql.NullString{},
				sql.NullTime{},
				sql.NullTime{},
				fetchedAt,
			},
			wantDemandNil:     true,
			wantVelocityNil:   true,
			wantSaturationNil: true,
		},
		{
			name: "velocity column present but garbage",
			values: []any{
				"Pikachu", "7d",
				sql.NullString{String: validDemand, Valid: true},
				sql.NullString{String: "{", Valid: true},
				sql.NullString{String: validSaturation, Valid: true},
				sql.NullTime{Time: demandComputedAt, Valid: true},
				sql.NullTime{Time: analyticsComputedAt, Valid: true},
				fetchedAt,
			},
			wantVelocityNil: true,
			wantMalformed: []demand.MalformedPayload{
				{Column: demand.MalformedColumnVelocity},
			},
		},
		{
			name: "demand and saturation both garbage",
			values: []any{
				"Pikachu", "7d",
				sql.NullString{String: "{", Valid: true},
				sql.NullString{String: validVelocity, Valid: true},
				sql.NullString{String: "{", Valid: true},
				sql.NullTime{Time: demandComputedAt, Valid: true},
				sql.NullTime{Time: analyticsComputedAt, Valid: true},
				fetchedAt,
			},
			wantDemandNil:     true,
			wantSaturationNil: true,
			wantMalformed: []demand.MalformedPayload{
				{Column: demand.MalformedColumnDemand},
				{Column: demand.MalformedColumnSaturation},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			row, err := scanCharacterCacheRow(&mocks.RowScanner{Values: tc.values})
			if err != nil {
				t.Fatalf("scanCharacterCacheRow: unexpected error: %v", err)
			}

			if (row.Demand == nil) != tc.wantDemandNil {
				t.Errorf("Demand nil = %v, want %v", row.Demand == nil, tc.wantDemandNil)
			}
			if (row.Velocity == nil) != tc.wantVelocityNil {
				t.Errorf("Velocity nil = %v, want %v", row.Velocity == nil, tc.wantVelocityNil)
			}
			if (row.Saturation == nil) != tc.wantSaturationNil {
				t.Errorf("Saturation nil = %v, want %v", row.Saturation == nil, tc.wantSaturationNil)
			}
			if tc.wantSampleSize != nil {
				if row.Velocity == nil || row.Velocity.SampleSize != *tc.wantSampleSize {
					t.Errorf("Velocity.SampleSize = %v, want %d", row.Velocity, *tc.wantSampleSize)
				}
			}
			if tc.wantActiveListingCount != nil {
				if row.Saturation == nil || row.Saturation.ActiveListingCount != *tc.wantActiveListingCount {
					t.Errorf("Saturation.ActiveListingCount = %v, want %d", row.Saturation, *tc.wantActiveListingCount)
				}
			}

			if len(row.MalformedPayloads) != len(tc.wantMalformed) {
				t.Fatalf("MalformedPayloads = %d entries, want %d: %+v", len(row.MalformedPayloads), len(tc.wantMalformed), row.MalformedPayloads)
			}
			for i, want := range tc.wantMalformed {
				got := row.MalformedPayloads[i]
				if got.Column != want.Column {
					t.Errorf("MalformedPayloads[%d].Column = %q, want %q", i, got.Column, want.Column)
				}
				if got.Err == nil {
					t.Errorf("MalformedPayloads[%d].Err = nil, want non-nil", i)
				}
			}
		})
	}
}
