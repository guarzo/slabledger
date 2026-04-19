package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CampaignStore implements campaign CRUD operations.
type CampaignStore struct {
	base
}

// NewCampaignStore creates a new campaign store.
func NewCampaignStore(db *sql.DB, logger observability.Logger) *CampaignStore {
	return &CampaignStore{base{db: db, logger: logger}}
}

var _ inventory.CampaignRepository = (*CampaignStore)(nil)

func (cs *CampaignStore) CreateCampaign(ctx context.Context, c *inventory.Campaign) error {
	query := `
		INSERT INTO campaigns (id, name, sport, year_range, grade_range, price_range,
			cl_confidence, buy_terms_cl_pct, daily_spend_cap_cents, inclusion_list,
			exclusion_mode, phase, psa_sourcing_fee_cents, ebay_fee_pct, expected_fill_rate, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`
	_, err := cs.db.ExecContext(ctx, query,
		c.ID, c.Name, c.Sport, c.YearRange, c.GradeRange, c.PriceRange,
		c.CLConfidence, c.BuyTermsCLPct, c.DailySpendCapCents, c.InclusionList,
		c.ExclusionMode, c.Phase, c.PSASourcingFeeCents, c.EbayFeePct, c.ExpectedFillRate, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create campaign: %w", err)
	}
	return nil
}

func (cs *CampaignStore) GetCampaign(ctx context.Context, id string) (*inventory.Campaign, error) {
	query := `
		SELECT id, name, sport, year_range, grade_range, price_range,
			cl_confidence, buy_terms_cl_pct, daily_spend_cap_cents, inclusion_list,
			exclusion_mode, phase, psa_sourcing_fee_cents, ebay_fee_pct, expected_fill_rate, created_at, updated_at
		FROM campaigns WHERE id = $1
	`
	var c inventory.Campaign
	err := cs.db.QueryRowContext(ctx, query, id).Scan(
		&c.ID, &c.Name, &c.Sport, &c.YearRange, &c.GradeRange, &c.PriceRange,
		&c.CLConfidence, &c.BuyTermsCLPct, &c.DailySpendCapCents, &c.InclusionList,
		&c.ExclusionMode, &c.Phase, &c.PSASourcingFeeCents, &c.EbayFeePct, &c.ExpectedFillRate, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, inventory.ErrCampaignNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (cs *CampaignStore) ListCampaigns(ctx context.Context, activeOnly bool) (result []inventory.Campaign, err error) {
	query := `
		SELECT id, name, sport, year_range, grade_range, price_range,
			cl_confidence, buy_terms_cl_pct, daily_spend_cap_cents, inclusion_list,
			exclusion_mode, phase, psa_sourcing_fee_cents, ebay_fee_pct, expected_fill_rate, created_at, updated_at
		FROM campaigns
	`
	if activeOnly {
		query += ` WHERE phase = 'active'`
	}
	query += ` ORDER BY created_at DESC`

	rows, err := cs.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query campaigns: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	const campaignsInitialCapacity = 64
	result = make([]inventory.Campaign, 0, campaignsInitialCapacity)
	for rows.Next() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		var c inventory.Campaign
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Sport, &c.YearRange, &c.GradeRange, &c.PriceRange,
			&c.CLConfidence, &c.BuyTermsCLPct, &c.DailySpendCapCents, &c.InclusionList,
			&c.ExclusionMode, &c.Phase, &c.PSASourcingFeeCents, &c.EbayFeePct, &c.ExpectedFillRate, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan campaign row: %w", err)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (cs *CampaignStore) DeleteCampaign(ctx context.Context, id string) (retErr error) {
	tx, err := cs.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if retErr != nil {
			_ = tx.Rollback() //nolint:errcheck // best-effort; error logged via retErr
		}
	}()

	if _, retErr = tx.ExecContext(ctx,
		`DELETE FROM campaign_sales WHERE purchase_id IN (SELECT id FROM campaign_purchases WHERE campaign_id = $1)`, id,
	); retErr != nil {
		return retErr
	}

	if _, retErr = tx.ExecContext(ctx,
		`DELETE FROM campaign_purchases WHERE campaign_id = $1`, id,
	); retErr != nil {
		return retErr
	}

	result, retErr := tx.ExecContext(ctx, `DELETE FROM campaigns WHERE id = $1`, id)
	if retErr != nil {
		return retErr
	}
	n, retErr := result.RowsAffected()
	if retErr != nil {
		return retErr
	}
	if n == 0 {
		return inventory.ErrCampaignNotFound
	}

	return tx.Commit()
}

func (cs *CampaignStore) UpdateCampaign(ctx context.Context, c *inventory.Campaign) error {
	query := `
		UPDATE campaigns SET name = $1, sport = $2, year_range = $3, grade_range = $4,
			price_range = $5, cl_confidence = $6, buy_terms_cl_pct = $7,
			daily_spend_cap_cents = $8, inclusion_list = $9, exclusion_mode = $10, phase = $11,
			psa_sourcing_fee_cents = $12, ebay_fee_pct = $13, expected_fill_rate = $14, updated_at = $15
		WHERE id = $16
	`
	result, err := cs.db.ExecContext(ctx, query,
		c.Name, c.Sport, c.YearRange, c.GradeRange, c.PriceRange,
		c.CLConfidence, c.BuyTermsCLPct, c.DailySpendCapCents, c.InclusionList,
		c.ExclusionMode, c.Phase, c.PSASourcingFeeCents, c.EbayFeePct, c.ExpectedFillRate, c.UpdatedAt, c.ID,
	)
	if err != nil {
		return fmt.Errorf("update campaign: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return inventory.ErrCampaignNotFound
	}
	return nil
}
