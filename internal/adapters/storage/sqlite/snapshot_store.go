package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// SnapshotStore implements snapshot and history recording operations.
type SnapshotStore struct {
	base
}

// NewSnapshotStore creates a new Snapshot store.
func NewSnapshotStore(db *sql.DB, logger observability.Logger) *SnapshotStore {
	return &SnapshotStore{base{db: db, logger: logger}}
}

var (
	_ inventory.SnapshotHistoryRecorder   = (*SnapshotStore)(nil)
	_ inventory.PopulationHistoryRecorder = (*SnapshotStore)(nil)
	_ inventory.CLValueHistoryRecorder    = (*SnapshotStore)(nil)
)

func (snaps *SnapshotStore) RecordSnapshot(ctx context.Context, e inventory.SnapshotHistoryEntry) error {
	query := `
		INSERT INTO market_snapshot_history (
			card_name, set_name, card_number, grade_value,
			median_cents, conservative_cents, optimistic_cents,
			last_sold_cents, lowest_list_cents, estimated_value_cents,
			active_listings, sales_last_30d, sales_last_90d,
			daily_velocity, weekly_velocity, trend_30d, trend_90d, volatility,
			source_count, fusion_confidence, snapshot_json, snapshot_date
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(card_name, set_name, card_number, grade_value, snapshot_date)
		DO UPDATE SET
			median_cents = excluded.median_cents,
			conservative_cents = excluded.conservative_cents,
			optimistic_cents = excluded.optimistic_cents,
			last_sold_cents = excluded.last_sold_cents,
			lowest_list_cents = excluded.lowest_list_cents,
			estimated_value_cents = excluded.estimated_value_cents,
			active_listings = excluded.active_listings,
			sales_last_30d = excluded.sales_last_30d,
			sales_last_90d = excluded.sales_last_90d,
			daily_velocity = excluded.daily_velocity,
			weekly_velocity = excluded.weekly_velocity,
			trend_30d = excluded.trend_30d,
			trend_90d = excluded.trend_90d,
			volatility = excluded.volatility,
			source_count = excluded.source_count,
			fusion_confidence = excluded.fusion_confidence,
			snapshot_json = excluded.snapshot_json
	`
	_, err := snaps.db.ExecContext(ctx, query,
		e.CardName, e.SetName, e.CardNumber, e.GradeValue,
		e.MedianCents, e.ConservativeCents, e.OptimisticCents,
		e.LastSoldCents, e.LowestListCents, e.EstimatedValueCents,
		e.ActiveListings, e.SalesLast30d, e.SalesLast90d,
		e.DailyVelocity, e.WeeklyVelocity, e.Trend30d, e.Trend90d, e.Volatility,
		e.SourceCount, e.Confidence, e.SnapshotJSON, e.SnapshotDate,
	)
	if err != nil {
		return fmt.Errorf("record snapshot: %w", err)
	}
	return nil
}

// RecordPopulation inserts or updates a population history observation.
func (snaps *SnapshotStore) RecordPopulation(ctx context.Context, e inventory.PopulationEntry) error {
	query := `
		INSERT INTO population_history (
			card_name, set_name, card_number, grade_value, grader,
			population, pop_higher, observation_date, source
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(card_name, set_name, card_number, grade_value, grader, observation_date)
		DO UPDATE SET population = excluded.population, pop_higher = excluded.pop_higher
	`
	_, err := snaps.db.ExecContext(ctx, query,
		e.CardName, e.SetName, e.CardNumber, e.GradeValue, e.Grader,
		e.Population, e.PopHigher, e.ObservationDate, e.Source,
	)
	if err != nil {
		return fmt.Errorf("record population: %w", err)
	}
	return nil
}

// RecordCLValue inserts or updates a Card Ladder value history observation.
func (snaps *SnapshotStore) RecordCLValue(ctx context.Context, e inventory.CLValueEntry) error {
	query := `
		INSERT INTO cl_value_history (
			cert_number, card_name, set_name, card_number, grade_value,
			cl_value_cents, observation_date, source
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(cert_number, observation_date)
		DO UPDATE SET cl_value_cents = excluded.cl_value_cents
	`
	_, err := snaps.db.ExecContext(ctx, query,
		e.CertNumber, e.CardName, e.SetName, e.CardNumber, e.GradeValue,
		e.CLValueCents, e.ObservationDate, e.Source,
	)
	if err != nil {
		return fmt.Errorf("record cl value: %w", err)
	}
	return nil
}
