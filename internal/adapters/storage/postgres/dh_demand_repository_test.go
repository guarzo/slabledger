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
				sql.NullFloat64{},
				sql.NullString{},
				sql.NullTime{},
				sql.NullTime{},
				fetchedAt,
			},
			assert: func(t *testing.T, row *demand.CardCache) {
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
