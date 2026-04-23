package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/liquidation"
)

// LiquidationStore implements liquidation.PurchaseLister and liquidation.CompReader.
type LiquidationStore struct {
	db *sql.DB
}

// NewLiquidationStore creates a new LiquidationStore.
func NewLiquidationStore(db *sql.DB) *LiquidationStore {
	return &LiquidationStore{db: db}
}

// ListUnsoldForLiquidation returns all unsold purchases from non-closed campaigns.
func (s *LiquidationStore) ListUnsoldForLiquidation(ctx context.Context) ([]liquidation.UnsoldPurchase, error) {
	const q = `
		SELECT p.id, p.cert_number, p.card_name, p.grade_value,
		       c.name, p.buy_cost_cents, p.cl_value_cents,
		       COALESCE(p.gem_rate_id, ''), p.reviewed_price_cents
		FROM campaign_purchases p
		JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE c.phase != 'closed'
		  AND s.id IS NULL
		ORDER BY p.cl_value_cents DESC`

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("liquidation store: list unsold: %w", err)
	}
	defer rows.Close()

	var result []liquidation.UnsoldPurchase
	for rows.Next() {
		var p liquidation.UnsoldPurchase
		if err := rows.Scan(
			&p.ID, &p.CertNumber, &p.CardName, &p.GradeValue,
			&p.CampaignName, &p.BuyCostCents, &p.CLValueCents,
			&p.GemRateID, &p.ReviewedPriceCents,
		); err != nil {
			return nil, fmt.Errorf("liquidation store: scan unsold purchase: %w", err)
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("liquidation store: rows error: %w", err)
	}
	return result, nil
}

// GetSaleCompsForCard returns historical sale comps for a card, most recent first.
func (s *LiquidationStore) GetSaleCompsForCard(ctx context.Context, gemRateID, condition string) ([]liquidation.SaleComp, error) {
	const q = `
		SELECT sale_date, price_cents
		FROM cl_sales_comps
		WHERE gem_rate_id = $1
		  AND condition = $2
		ORDER BY sale_date DESC`

	rows, err := s.db.QueryContext(ctx, q, gemRateID, condition)
	if err != nil {
		return nil, fmt.Errorf("liquidation store: get sale comps: %w", err)
	}
	defer rows.Close()

	var result []liquidation.SaleComp
	for rows.Next() {
		var c liquidation.SaleComp
		if err := rows.Scan(&c.SaleDate, &c.PriceCents); err != nil {
			return nil, fmt.Errorf("liquidation store: scan sale comp: %w", err)
		}
		result = append(result, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("liquidation store: rows error: %w", err)
	}
	return result, nil
}
