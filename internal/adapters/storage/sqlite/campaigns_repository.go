package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// CampaignsRepository implements campaigns.Repository using SQLite.
type CampaignsRepository struct {
	db *sql.DB
}

// NewCampaignsRepository creates a new SQLite campaigns repository.
func NewCampaignsRepository(db *sql.DB) *CampaignsRepository {
	return &CampaignsRepository{db: db}
}

var _ campaigns.Repository = (*CampaignsRepository)(nil)

// --- Campaign CRUD ---

func (r *CampaignsRepository) CreateCampaign(ctx context.Context, c *campaigns.Campaign) error {
	query := `
		INSERT INTO campaigns (id, name, sport, year_range, grade_range, price_range,
			cl_confidence, buy_terms_cl_pct, daily_spend_cap_cents, inclusion_list,
			exclusion_mode, phase, psa_sourcing_fee_cents, ebay_fee_pct, expected_fill_rate, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query,
		c.ID, c.Name, c.Sport, c.YearRange, c.GradeRange, c.PriceRange,
		c.CLConfidence, c.BuyTermsCLPct, c.DailySpendCapCents, c.InclusionList,
		c.ExclusionMode, c.Phase, c.PSASourcingFeeCents, c.EbayFeePct, c.ExpectedFillRate, c.CreatedAt, c.UpdatedAt,
	)
	return err
}

func (r *CampaignsRepository) GetCampaign(ctx context.Context, id string) (*campaigns.Campaign, error) {
	query := `
		SELECT id, name, sport, year_range, grade_range, price_range,
			cl_confidence, buy_terms_cl_pct, daily_spend_cap_cents, inclusion_list,
			exclusion_mode, phase, psa_sourcing_fee_cents, ebay_fee_pct, expected_fill_rate, created_at, updated_at
		FROM campaigns WHERE id = ?
	`
	var c campaigns.Campaign
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&c.ID, &c.Name, &c.Sport, &c.YearRange, &c.GradeRange, &c.PriceRange,
		&c.CLConfidence, &c.BuyTermsCLPct, &c.DailySpendCapCents, &c.InclusionList,
		&c.ExclusionMode, &c.Phase, &c.PSASourcingFeeCents, &c.EbayFeePct, &c.ExpectedFillRate, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, campaigns.ErrCampaignNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *CampaignsRepository) ListCampaigns(ctx context.Context, activeOnly bool) (result []campaigns.Campaign, err error) {
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

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	const campaignsInitialCapacity = 64
	result = make([]campaigns.Campaign, 0, campaignsInitialCapacity)
	for rows.Next() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		var c campaigns.Campaign
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Sport, &c.YearRange, &c.GradeRange, &c.PriceRange,
			&c.CLConfidence, &c.BuyTermsCLPct, &c.DailySpendCapCents, &c.InclusionList,
			&c.ExclusionMode, &c.Phase, &c.PSASourcingFeeCents, &c.EbayFeePct, &c.ExpectedFillRate, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (r *CampaignsRepository) DeleteCampaign(ctx context.Context, id string) (retErr error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			_ = tx.Rollback() //nolint:errcheck // best-effort; error logged via retErr
		}
	}()

	// Delete sales associated with this campaign's purchases
	if _, retErr = tx.ExecContext(ctx,
		`DELETE FROM campaign_sales WHERE purchase_id IN (SELECT id FROM campaign_purchases WHERE campaign_id = ?)`, id,
	); retErr != nil {
		return retErr
	}

	// Delete purchases
	if _, retErr = tx.ExecContext(ctx,
		`DELETE FROM campaign_purchases WHERE campaign_id = ?`, id,
	); retErr != nil {
		return retErr
	}

	// Delete campaign
	result, retErr := tx.ExecContext(ctx, `DELETE FROM campaigns WHERE id = ?`, id)
	if retErr != nil {
		return retErr
	}
	n, retErr := result.RowsAffected()
	if retErr != nil {
		return retErr
	}
	if n == 0 {
		return campaigns.ErrCampaignNotFound
	}

	return tx.Commit()
}

func (r *CampaignsRepository) DeletePurchase(ctx context.Context, id string) (retErr error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if retErr != nil {
			_ = tx.Rollback() //nolint:errcheck // best-effort; error logged via retErr
		}
	}()

	// Delete any sales associated with this purchase
	if _, retErr = tx.ExecContext(ctx,
		`DELETE FROM campaign_sales WHERE purchase_id = ?`, id,
	); retErr != nil {
		return retErr
	}

	// Delete the purchase
	result, err := tx.ExecContext(ctx, `DELETE FROM campaign_purchases WHERE id = ?`, id)
	if err != nil {
		retErr = err
		return retErr
	}
	n, err := result.RowsAffected()
	if err != nil {
		retErr = err
		return retErr
	}
	if n == 0 {
		retErr = campaigns.ErrPurchaseNotFound
		return retErr
	}

	return tx.Commit()
}

func (r *CampaignsRepository) UpdateCampaign(ctx context.Context, c *campaigns.Campaign) error {
	query := `
		UPDATE campaigns SET name = ?, sport = ?, year_range = ?, grade_range = ?,
			price_range = ?, cl_confidence = ?, buy_terms_cl_pct = ?,
			daily_spend_cap_cents = ?, inclusion_list = ?, exclusion_mode = ?, phase = ?,
			psa_sourcing_fee_cents = ?, ebay_fee_pct = ?, expected_fill_rate = ?, updated_at = ?
		WHERE id = ?
	`
	result, err := r.db.ExecContext(ctx, query,
		c.Name, c.Sport, c.YearRange, c.GradeRange, c.PriceRange,
		c.CLConfidence, c.BuyTermsCLPct, c.DailySpendCapCents, c.InclusionList,
		c.ExclusionMode, c.Phase, c.PSASourcingFeeCents, c.EbayFeePct, c.ExpectedFillRate, c.UpdatedAt, c.ID,
	)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return campaigns.ErrCampaignNotFound
	}
	return nil
}
