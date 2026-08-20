package inventory

import (
	"testing"
	"time"
)

func TestDeriveDHSoldAt(t *testing.T) {
	createdAt := time.Date(2026, 1, 20, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		saleDate     string
		purchaseDate string
		createdAt    time.Time
		want         time.Time
	}{
		{
			name:         "normal case: saleDate within [purchaseDate, createdAt]",
			saleDate:     "2026-01-15",
			purchaseDate: "2026-01-01",
			createdAt:    createdAt,
			want:         time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "saleDate before purchaseDate clamps up to purchaseDate",
			saleDate:     "2025-12-01",
			purchaseDate: "2026-01-01",
			createdAt:    createdAt,
			want:         time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "saleDate after createdAt clamps down to createdAt",
			saleDate:     "2026-02-01",
			purchaseDate: "2026-01-01",
			createdAt:    createdAt,
			want:         createdAt,
		},
		{
			name:         "malformed saleDate falls back to createdAt",
			saleDate:     "not-a-date",
			purchaseDate: "2026-01-01",
			createdAt:    createdAt,
			want:         createdAt,
		},
		{
			name:         "malformed purchaseDate omits the lower clamp",
			saleDate:     "2025-06-01",
			purchaseDate: "garbage",
			createdAt:    createdAt,
			want:         time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveDHSoldAt(tt.saleDate, tt.purchaseDate, tt.createdAt)
			if !got.Equal(tt.want) {
				t.Errorf("DeriveDHSoldAt(%q, %q, %v) = %v, want %v",
					tt.saleDate, tt.purchaseDate, tt.createdAt, got, tt.want)
			}
			if got.Location() != time.UTC {
				t.Errorf("DeriveDHSoldAt() location = %v, want UTC", got.Location())
			}
		})
	}
}

// TestDeriveDHSoldAt_TimezoneIndependence proves the result is identical
// regardless of the zone createdAt arrives in (design doc §2, finding A from
// the second review): CreatedAt is written by an unnormalised time.Now() into
// a timezone-less column, so if DeriveDHSoldAt read wall-clock fields without
// normalising first, the same stored instant could produce two different
// sold_at values depending on which zone the process ran in -- and under a
// fixed idempotency key, DH would 422 the second as idempotency_key_reused.
func TestDeriveDHSoldAt_TimezoneIndependence(t *testing.T) {
	saleDate := "2026-01-15"
	purchaseDate := "2026-01-01"

	// The same instant, expressed once in UTC and once in a fixed non-UTC
	// zone. FixedZone keeps this hermetic without touching time.Local.
	utcCreatedAt := time.Date(2026, 1, 20, 10, 0, 0, 0, time.UTC)
	nonUTCZone := time.FixedZone("UTC-5", -5*60*60)
	nonUTCCreatedAt := utcCreatedAt.In(nonUTCZone)

	if !utcCreatedAt.Equal(nonUTCCreatedAt) {
		t.Fatal("test setup bug: the two createdAt values must denote the same instant")
	}

	gotUTC := DeriveDHSoldAt(saleDate, purchaseDate, utcCreatedAt)
	gotNonUTC := DeriveDHSoldAt(saleDate, purchaseDate, nonUTCCreatedAt)

	if !gotUTC.Equal(gotNonUTC) {
		t.Fatalf("different instants for UTC vs non-UTC createdAt: %v vs %v", gotUTC, gotNonUTC)
	}
	if gotUTC.Location() != time.UTC || gotNonUTC.Location() != time.UTC {
		t.Fatal("DeriveDHSoldAt must always return a UTC-located time.Time")
	}

	// Also exercise the upper-clamp path under a non-UTC createdAt, since that
	// is where a naive implementation using createdAt's wall-clock fields
	// (rather than its normalised instant) would diverge.
	lateSaleDate := "2026-02-01" // after createdAt in both cases
	gotUTCClamped := DeriveDHSoldAt(lateSaleDate, purchaseDate, utcCreatedAt)
	gotNonUTCClamped := DeriveDHSoldAt(lateSaleDate, purchaseDate, nonUTCCreatedAt)
	if !gotUTCClamped.Equal(gotNonUTCClamped) {
		t.Fatalf("clamped result differs by input zone: %v vs %v", gotUTCClamped, gotNonUTCClamped)
	}
}
