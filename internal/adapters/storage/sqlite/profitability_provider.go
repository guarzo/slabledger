package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/guarzo/slabledger/internal/domain/picks"
)

// ProfitabilityProvider queries campaign_purchases + campaign_sales
// to surface historical profitability patterns for the AI picks engine.
type ProfitabilityProvider struct {
	db *sql.DB
}

// NewProfitabilityProvider creates a new ProfitabilityProvider backed by db.
func NewProfitabilityProvider(db *sql.DB) *ProfitabilityProvider {
	return &ProfitabilityProvider{db: db}
}

var _ picks.ProfitabilityProvider = (*ProfitabilityProvider)(nil)

// GetProfitablePatterns returns a ProfitabilityProfile built from sold card data.
// Each sub-query runs independently; failures are tolerated so the caller receives
// whatever partial data is available.
func (p *ProfitabilityProvider) GetProfitablePatterns(ctx context.Context) (picks.ProfitabilityProfile, error) {
	var profile picks.ProfitabilityProfile
	var errs []error

	eras, err := p.queryTopEras(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("top eras: %w", err))
	}
	profile.TopEras = eras

	grades, err := p.queryProfitableGrades(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("profitable grades: %w", err))
	}
	profile.ProfitableGrades = grades

	days, err := p.queryAvgDaysToSell(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("avg days to sell: %w", err))
	}
	profile.AvgDaysToSell = days

	channels, err := p.queryTopChannels(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("top channels: %w", err))
	}
	profile.TopChannels = channels

	// ProfitablePriceTiers intentionally left empty for MVP.

	if len(errs) > 0 {
		return profile, errors.Join(errs...)
	}
	return profile, nil
}

// queryTopEras returns the top 5 set names by average ROI (requiring >= 3 sold cards).
func (p *ProfitabilityProvider) queryTopEras(ctx context.Context) ([]string, error) {
	const q = `
		SELECT p.set_name
		FROM campaign_purchases p
		INNER JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE p.buy_cost_cents > 0
		GROUP BY p.set_name
		HAVING COUNT(*) >= 3
		   AND AVG(CAST(s.net_profit_cents AS REAL) / CAST(p.buy_cost_cents AS REAL) * 100) > 0
		ORDER BY AVG(CAST(s.net_profit_cents AS REAL) / CAST(p.buy_cost_cents AS REAL) * 100) DESC
		LIMIT 5
	`
	rows, err := p.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var eras []string
	for rows.Next() {
		var era string
		if err := rows.Scan(&era); err != nil {
			return eras, err
		}
		eras = append(eras, era)
	}
	return eras, rows.Err()
}

// queryProfitableGrades returns avg ROI, avg margin, and count per grade (requiring >= 2 sold cards).
func (p *ProfitabilityProvider) queryProfitableGrades(ctx context.Context) ([]picks.GradeProfile, error) {
	const q = `
		SELECT
			CAST(p.grade_value AS TEXT) AS grade,
			AVG(CAST(s.net_profit_cents AS REAL) / CAST(p.buy_cost_cents AS REAL) * 100) AS avg_roi,
			AVG(s.net_profit_cents) AS avg_margin,
			COUNT(*) AS cnt
		FROM campaign_purchases p
		INNER JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE p.buy_cost_cents > 0
		GROUP BY p.grade_value
		HAVING COUNT(*) >= 2
		   AND AVG(CAST(s.net_profit_cents AS REAL) / CAST(p.buy_cost_cents AS REAL) * 100) > 0
		ORDER BY avg_roi DESC
	`
	rows, err := p.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var profiles []picks.GradeProfile
	for rows.Next() {
		var gp picks.GradeProfile
		var avgMarginFloat float64
		if err := rows.Scan(&gp.Grade, &gp.AvgROI, &avgMarginFloat, &gp.Count); err != nil {
			return profiles, err
		}
		gp.AvgMargin = int(math.Round(avgMarginFloat))
		profiles = append(profiles, gp)
	}
	return profiles, rows.Err()
}

// queryAvgDaysToSell returns the overall average days from purchase creation to sale.
func (p *ProfitabilityProvider) queryAvgDaysToSell(ctx context.Context) (int, error) {
	const q = `
		SELECT COALESCE(AVG(JULIANDAY(s.sale_date) - JULIANDAY(p.created_at)), 0)
		FROM campaign_purchases p
		INNER JOIN campaign_sales s ON s.purchase_id = p.id
	`
	var avgDays float64
	err := p.db.QueryRowContext(ctx, q).Scan(&avgDays)
	if err != nil {
		return 0, err
	}
	return int(math.Round(avgDays)), nil
}

// queryTopChannels returns the top 3 sale channels by number of sales.
func (p *ProfitabilityProvider) queryTopChannels(ctx context.Context) ([]string, error) {
	const q = `
		SELECT sale_channel
		FROM campaign_sales
		GROUP BY sale_channel
		ORDER BY COUNT(*) DESC
		LIMIT 3
	`
	rows, err := p.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var channels []string
	for rows.Next() {
		var ch string
		if err := rows.Scan(&ch); err != nil {
			return channels, err
		}
		channels = append(channels, ch)
	}
	return channels, rows.Err()
}

// InventoryProvider returns the set of currently held (unsold) card identifiers
// for exclusion from AI picks.
type InventoryProvider struct {
	db *sql.DB
}

// NewInventoryProvider creates a new InventoryProvider backed by db.
func NewInventoryProvider(db *sql.DB) *InventoryProvider {
	return &InventoryProvider{db: db}
}

var _ picks.InventoryProvider = (*InventoryProvider)(nil)

// GetHeldCardNames returns "CardName | SetName | Grade" strings for every
// campaign_purchase that has no corresponding sale record.
func (iv *InventoryProvider) GetHeldCardNames(ctx context.Context) ([]string, error) {
	const q = `
		SELECT DISTINCT
			p.card_name,
			p.set_name,
			CAST(p.grade_value AS TEXT) AS grade
		FROM campaign_purchases p
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL
		ORDER BY p.card_name
	`
	rows, err := iv.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query held cards: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var names []string
	for rows.Next() {
		var cardName, setName, grade string
		if err := rows.Scan(&cardName, &setName, &grade); err != nil {
			return names, fmt.Errorf("scan held card row: %w", err)
		}
		names = append(names, cardName+" | "+setName+" | "+grade)
	}
	return names, rows.Err()
}
