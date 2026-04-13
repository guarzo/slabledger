package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordSnapshot_RoundTrip(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		entry inventory.SnapshotHistoryEntry
	}{
		{
			name: "basic snapshot persists",
			entry: inventory.SnapshotHistoryEntry{
				CardName:            "Charizard",
				SetName:             "Base Set",
				CardNumber:          "4",
				GradeValue:          9.0,
				MedianCents:         120000,
				ConservativeCents:   100000,
				OptimisticCents:     140000,
				LastSoldCents:       118000,
				LowestListCents:     115000,
				EstimatedValueCents: 122000,
				ActiveListings:      5,
				SalesLast30d:        3,
				SalesLast90d:        8,
				DailyVelocity:       0.1,
				WeeklyVelocity:      0.7,
				Trend30d:            0.05,
				Trend90d:            0.03,
				Volatility:          0.12,
				SourceCount:         2,
				Confidence:          0.85,
				SnapshotJSON:        `{"median":1200}`,
				SnapshotDate:        "2026-01-15",
			},
		},
		{
			name: "zero-value fields accepted",
			entry: inventory.SnapshotHistoryEntry{
				CardName:     "Pikachu",
				SetName:      "Jungle",
				CardNumber:   "60",
				GradeValue:   10.0,
				SnapshotDate: "2026-02-01",
			},
		},
		{
			name: "empty set and card number",
			entry: inventory.SnapshotHistoryEntry{
				CardName:     "Mew",
				SnapshotDate: "2026-03-01",
				MedianCents:  50000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close()
			store := NewSnapshotStore(db.DB, nil)

			err := store.RecordSnapshot(ctx, tt.entry)
			require.NoError(t, err)

			// Verify via direct DB query
			var cardName, setName, cardNumber, snapshotDate string
			var gradeValue float64
			var medianCents int
			err = db.QueryRowContext(ctx,
				`SELECT card_name, set_name, card_number, grade_value, median_cents,
				 strftime('%Y-%m-%d', snapshot_date) AS snapshot_date
				 FROM market_snapshot_history
				 WHERE card_name = ? AND strftime('%Y-%m-%d', snapshot_date) = ?`,
				tt.entry.CardName, tt.entry.SnapshotDate,
			).Scan(&cardName, &setName, &cardNumber, &gradeValue, &medianCents, &snapshotDate)
			require.NoError(t, err)

			assert.Equal(t, tt.entry.CardName, cardName)
			assert.Equal(t, tt.entry.SetName, setName)
			assert.Equal(t, tt.entry.CardNumber, cardNumber)
			assert.Equal(t, tt.entry.GradeValue, gradeValue)
			assert.Equal(t, tt.entry.MedianCents, medianCents)
			assert.Equal(t, tt.entry.SnapshotDate, snapshotDate)
		})
	}
}

func TestRecordSnapshot_UpsertOnConflict(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()
	store := NewSnapshotStore(db.DB, nil)

	base := inventory.SnapshotHistoryEntry{
		CardName:     "Charizard",
		SetName:      "Base Set",
		CardNumber:   "4",
		GradeValue:   9.0,
		MedianCents:  100000,
		SnapshotDate: "2026-01-15",
	}
	updated := base
	updated.MedianCents = 150000
	updated.SnapshotJSON = `{"median":1500}`

	require.NoError(t, store.RecordSnapshot(ctx, base))
	require.NoError(t, store.RecordSnapshot(ctx, updated))

	var count int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM market_snapshot_history WHERE card_name='Charizard' AND strftime('%Y-%m-%d', snapshot_date)='2026-01-15'`,
	).Scan(&count))
	assert.Equal(t, 1, count, "upsert should produce exactly one row")

	var medianCents int
	var snapshotJSON string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT median_cents, snapshot_json FROM market_snapshot_history WHERE card_name='Charizard' AND strftime('%Y-%m-%d', snapshot_date)='2026-01-15'`,
	).Scan(&medianCents, &snapshotJSON))
	assert.Equal(t, 150000, medianCents, "upsert should update median_cents")
	assert.Equal(t, `{"median":1500}`, snapshotJSON)
}

func TestRecordPopulation_RoundTrip(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		entry inventory.PopulationEntry
	}{
		{
			name: "basic population persists",
			entry: inventory.PopulationEntry{
				CardName:        "Charizard",
				SetName:         "Base Set",
				CardNumber:      "4",
				GradeValue:      10.0,
				Grader:          "PSA",
				Population:      45,
				PopHigher:       0,
				ObservationDate: "2026-01-15",
				Source:          "psa_api",
			},
		},
		{
			name: "pop_higher populated",
			entry: inventory.PopulationEntry{
				CardName:        "Lugia",
				SetName:         "Neo Genesis",
				CardNumber:      "9",
				GradeValue:      9.0,
				Grader:          "PSA",
				Population:      120,
				PopHigher:       45,
				ObservationDate: "2026-02-01",
				Source:          "manual",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close()
			store := NewSnapshotStore(db.DB, nil)

			err := store.RecordPopulation(ctx, tt.entry)
			require.NoError(t, err)

			var cardName, grader, source string
			var population, popHigher int
			err = db.QueryRowContext(ctx,
				`SELECT card_name, grader, population, pop_higher, source
				 FROM population_history
				 WHERE card_name = ? AND strftime('%Y-%m-%d', observation_date) = ? AND grader = ?`,
				tt.entry.CardName, tt.entry.ObservationDate, tt.entry.Grader,
			).Scan(&cardName, &grader, &population, &popHigher, &source)
			require.NoError(t, err)

			assert.Equal(t, tt.entry.CardName, cardName)
			assert.Equal(t, tt.entry.Grader, grader)
			assert.Equal(t, tt.entry.Population, population)
			assert.Equal(t, tt.entry.PopHigher, popHigher)
			assert.Equal(t, tt.entry.Source, source)
		})
	}
}

func TestRecordPopulation_UpsertOnConflict(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()
	store := NewSnapshotStore(db.DB, nil)

	base := inventory.PopulationEntry{
		CardName:        "Mewtwo",
		SetName:         "Base Set",
		CardNumber:      "10",
		GradeValue:      10.0,
		Grader:          "PSA",
		Population:      30,
		PopHigher:       0,
		ObservationDate: "2026-01-20",
		Source:          "psa_api",
	}
	updated := base
	updated.Population = 31
	updated.PopHigher = 1

	require.NoError(t, store.RecordPopulation(ctx, base))
	require.NoError(t, store.RecordPopulation(ctx, updated))

	var count int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM population_history WHERE card_name='Mewtwo' AND strftime('%Y-%m-%d', observation_date)='2026-01-20' AND grader='PSA'`,
	).Scan(&count))
	assert.Equal(t, 1, count, "upsert should produce exactly one row")

	var pop, popHigher int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT population, pop_higher FROM population_history WHERE card_name='Mewtwo' AND strftime('%Y-%m-%d', observation_date)='2026-01-20'`,
	).Scan(&pop, &popHigher))
	assert.Equal(t, 31, pop)
	assert.Equal(t, 1, popHigher)
}

func TestRecordCLValue_RoundTrip(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		entry inventory.CLValueEntry
	}{
		{
			name: "basic cl value persists",
			entry: inventory.CLValueEntry{
				CertNumber:      "12345678",
				CardName:        "Charizard",
				SetName:         "Base Set",
				CardNumber:      "4",
				GradeValue:      9.0,
				CLValueCents:    95000,
				ObservationDate: "2026-01-15",
				Source:          "cl_api",
			},
		},
		{
			name: "different cert same card",
			entry: inventory.CLValueEntry{
				CertNumber:      "87654321",
				CardName:        "Charizard",
				SetName:         "Base Set",
				CardNumber:      "4",
				GradeValue:      9.0,
				CLValueCents:    98000,
				ObservationDate: "2026-01-15",
				Source:          "cl_api",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close()
			store := NewSnapshotStore(db.DB, nil)

			err := store.RecordCLValue(ctx, tt.entry)
			require.NoError(t, err)

			var certNumber, cardName, source string
			var clValueCents int
			err = db.QueryRowContext(ctx,
				`SELECT cert_number, card_name, cl_value_cents, source
				 FROM cl_value_history
				 WHERE cert_number = ? AND strftime('%Y-%m-%d', observation_date) = ?`,
				tt.entry.CertNumber, tt.entry.ObservationDate,
			).Scan(&certNumber, &cardName, &clValueCents, &source)
			require.NoError(t, err)

			assert.Equal(t, tt.entry.CertNumber, certNumber)
			assert.Equal(t, tt.entry.CardName, cardName)
			assert.Equal(t, tt.entry.CLValueCents, clValueCents)
			assert.Equal(t, tt.entry.Source, source)
		})
	}
}

func TestRecordCLValue_UpsertOnConflict(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()
	store := NewSnapshotStore(db.DB, nil)

	base := inventory.CLValueEntry{
		CertNumber:      "11223344",
		CardName:        "Pikachu",
		SetName:         "Jungle",
		CardNumber:      "60",
		GradeValue:      10.0,
		CLValueCents:    12000,
		ObservationDate: "2026-02-10",
		Source:          "cl_api",
	}
	updated := base
	updated.CLValueCents = 13500

	require.NoError(t, store.RecordCLValue(ctx, base))
	require.NoError(t, store.RecordCLValue(ctx, updated))

	var count int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM cl_value_history WHERE cert_number='11223344' AND strftime('%Y-%m-%d', observation_date)='2026-02-10'`,
	).Scan(&count))
	assert.Equal(t, 1, count, "upsert should produce exactly one row")

	var clValueCents int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT cl_value_cents FROM cl_value_history WHERE cert_number='11223344' AND strftime('%Y-%m-%d', observation_date)='2026-02-10'`,
	).Scan(&clValueCents))
	assert.Equal(t, 13500, clValueCents)
}

func TestRecordCLValue_MultipleObservationDates(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()
	store := NewSnapshotStore(db.DB, nil)

	certNumber := "55667788"
	dates := []string{"2026-01-01", "2026-02-01", "2026-03-01"}
	values := []int{10000, 11000, 12000}

	for i, date := range dates {
		err := store.RecordCLValue(ctx, inventory.CLValueEntry{
			CertNumber:      certNumber,
			CardName:        "Blastoise",
			SetName:         "Base Set",
			CardNumber:      "2",
			GradeValue:      9.0,
			CLValueCents:    values[i],
			ObservationDate: date,
			Source:          "cl_api",
		})
		require.NoError(t, err)
	}

	var count int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM cl_value_history WHERE cert_number = ?`, certNumber,
	).Scan(&count))
	assert.Equal(t, 3, count, "each distinct observation_date should be a separate row")
}

func TestSnapshotStore_MultipleEntriesDifferentCards(t *testing.T) {
	ctx := context.Background()
	db := setupTestDB(t)
	defer db.Close()
	store := NewSnapshotStore(db.DB, nil)

	cards := []struct {
		name  string
		grade float64
	}{
		{"Charizard", 10},
		{"Pikachu", 9},
		{"Mewtwo", 8},
	}

	date := time.Now().Format("2006-01-02")
	for _, c := range cards {
		err := store.RecordSnapshot(ctx, inventory.SnapshotHistoryEntry{
			CardName:     c.name,
			GradeValue:   c.grade,
			MedianCents:  50000,
			SnapshotDate: date,
		})
		require.NoError(t, err)
	}

	var count int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM market_snapshot_history WHERE snapshot_date = ?`, date,
	).Scan(&count))
	assert.Equal(t, 3, count)
}
