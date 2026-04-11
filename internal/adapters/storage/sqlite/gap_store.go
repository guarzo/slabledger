package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/scoring"
)

type GapStore struct {
	db *sql.DB
}

var _ scoring.GapStore = (*GapStore)(nil)

func NewGapStore(db *sql.DB) *GapStore {
	return &GapStore{db: db}
}

func (s *GapStore) RecordGaps(ctx context.Context, gaps []scoring.GapRecord) error {
	if len(gaps) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO scoring_data_gaps (factor_name, reason, entity_type, entity_id, card_name, set_name, recorded_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert gaps statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, g := range gaps {
		recordedAt := g.RecordedAt
		if recordedAt.IsZero() {
			recordedAt = time.Now()
		}
		if _, err := stmt.ExecContext(ctx, g.FactorName, g.Reason, g.EntityType, g.EntityID, g.CardName, g.SetName, recordedAt); err != nil {
			return fmt.Errorf("insert gap record: %w", err)
		}
	}
	return tx.Commit()
}

func (s *GapStore) GetGapReport(ctx context.Context, since time.Time) (*scoring.GapReport, error) {
	var totalEntities, totalGaps int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT entity_type || '|' || entity_id), COUNT(*) FROM scoring_data_gaps WHERE recorded_at >= ?`, since).
		Scan(&totalEntities, &totalGaps)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT g.factor_name, COUNT(DISTINCT g.entity_type || '|' || g.entity_id) as cnt,
		        (SELECT reason FROM scoring_data_gaps r
		         WHERE r.factor_name = g.factor_name AND r.recorded_at >= ?
		         GROUP BY reason ORDER BY COUNT(*) DESC LIMIT 1) as top_reason
		 FROM scoring_data_gaps g
		 WHERE g.recorded_at >= ?
		 GROUP BY g.factor_name
		 ORDER BY cnt DESC`, since, since)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var byFactor []scoring.GapFactorSummary
	for rows.Next() {
		var gs scoring.GapFactorSummary
		if err := rows.Scan(&gs.Factor, &gs.Count, &gs.TopReason); err != nil {
			return nil, err
		}
		if totalEntities > 0 {
			gs.Pct = float64(gs.Count) / float64(totalEntities)
		}
		byFactor = append(byFactor, gs)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	setRows, err := s.db.QueryContext(ctx,
		`SELECT set_name, COUNT(*) as cnt, GROUP_CONCAT(DISTINCT factor_name) as factors
		 FROM scoring_data_gaps
		 WHERE recorded_at >= ? AND set_name != ''
		 GROUP BY set_name
		 ORDER BY cnt DESC
		 LIMIT 10`, since)
	if err != nil {
		return nil, err
	}
	defer func() { _ = setRows.Close() }()

	var mostAffected []scoring.GapSetSummary
	for setRows.Next() {
		var ss scoring.GapSetSummary
		var factors string
		if err := setRows.Scan(&ss.SetName, &ss.GapCount, &factors); err != nil {
			return nil, err
		}
		if factors != "" {
			ss.MissingFactors = strings.Split(factors, ",")
		}
		mostAffected = append(mostAffected, ss)
	}
	if err := setRows.Err(); err != nil {
		return nil, err
	}

	var suggestions []string
	for _, gf := range byFactor {
		suggestions = append(suggestions, gapSuggestion(gf))
	}

	gapRate := 0.0
	if totalEntities > 0 {
		gapRate = float64(totalGaps) / float64(totalEntities)
	}

	return &scoring.GapReport{
		Period:        fmt.Sprintf("%dd", int(time.Since(since).Hours()/24)),
		TotalScorings: totalEntities,
		TotalGaps:     totalGaps,
		GapRate:       gapRate,
		ByFactor:      byFactor,
		MostAffected:  mostAffected,
		Suggestions:   suggestions,
	}, nil
}

func (s *GapStore) PruneOldGaps(ctx context.Context, olderThan time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM scoring_data_gaps WHERE recorded_at < ?`, olderThan)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func gapSuggestion(gf scoring.GapFactorSummary) string {
	switch gf.TopReason {
	case "no_population_data":
		return fmt.Sprintf("%s: %d cards missing population data - consider enabling PSA population lookups", gf.Factor, gf.Count)
	case "no_market_data":
		return fmt.Sprintf("%s: %d cards missing market data - check price source coverage", gf.Factor, gf.Count)
	case "insufficient_sales":
		return fmt.Sprintf("%s: %d cards have insufficient sales history - data will improve with time", gf.Factor, gf.Count)
	default:
		return fmt.Sprintf("%s: %d gaps (%s)", gf.Factor, gf.Count, gf.TopReason)
	}
}
