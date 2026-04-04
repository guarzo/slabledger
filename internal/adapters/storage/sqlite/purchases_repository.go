package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// --- Purchase CRUD ---

func (r *CampaignsRepository) CreatePurchase(ctx context.Context, p *campaigns.Purchase) error {
	if p.Grader == "" {
		p.Grader = "PSA"
	}
	query := `
		INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number,
			card_number, set_name,
			grader, grade_value,
			cl_value_cents, buy_cost_cents, psa_sourcing_fee_cents,
			population, purchase_date, created_at, updated_at,
			last_sold_cents, lowest_list_cents, conservative_cents, median_cents,
			active_listings, sales_last_30d, trend_30d, snapshot_date, snapshot_json,
			vault_status, invoice_date, was_refunded, front_image_url, back_image_url, purchase_source,
			psa_listing_title, snapshot_status, snapshot_retry_count,
			override_price_cents, override_source, override_set_at,
			ai_suggested_price_cents, ai_suggested_at,
			card_year, ebay_export_flagged_at,
			reviewed_price_cents, reviewed_at, review_source,
			dh_card_id, dh_inventory_id, dh_cert_status, dh_listing_price_cents, dh_channels_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query,
		p.ID, p.CampaignID, p.CardName, p.CertNumber,
		p.CardNumber, p.SetName,
		p.Grader, p.GradeValue,
		p.CLValueCents, p.BuyCostCents, p.PSASourcingFeeCents,
		p.Population, p.PurchaseDate, p.CreatedAt, p.UpdatedAt,
		p.LastSoldCents, p.LowestListCents, p.ConservativeCents, p.MedianCents,
		p.ActiveListings, p.SalesLast30d, p.Trend30d, p.SnapshotDate, p.SnapshotJSON,
		p.VaultStatus, p.InvoiceDate, p.WasRefunded, p.FrontImageURL, p.BackImageURL, p.PurchaseSource,
		p.PSAListingTitle, p.SnapshotStatus, p.SnapshotRetryCount,
		p.OverridePriceCents, p.OverrideSource, p.OverrideSetAt,
		p.AISuggestedPriceCents, p.AISuggestedAt,
		p.CardYear, p.EbayExportFlaggedAt,
		p.ReviewedPriceCents, p.ReviewedAt, string(p.ReviewSource),
		p.DHCardID, p.DHInventoryID, p.DHCertStatus, p.DHListingPriceCents, p.DHChannelsJSON,
	)
	if err != nil && isUniqueConstraintError(err) {
		return campaigns.ErrDuplicateCertNumber
	}
	return err
}

func (r *CampaignsRepository) GetPurchase(ctx context.Context, id string) (*campaigns.Purchase, error) {
	query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases WHERE id = ?`
	var p campaigns.Purchase
	err := scanPurchase(r.db.QueryRowContext(ctx, query, id), &p)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, campaigns.ErrPurchaseNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *CampaignsRepository) ListPurchasesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]campaigns.Purchase, error) {
	query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases WHERE campaign_id = ?
		ORDER BY purchase_date DESC
		LIMIT ? OFFSET ?`
	rows, err := r.db.QueryContext(ctx, query, campaignID, limit, offset)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (campaigns.Purchase, error) {
		var p campaigns.Purchase
		err := scanPurchase(rs, &p)
		return p, err
	})
}

func (r *CampaignsRepository) ListUnsoldPurchases(ctx context.Context, campaignID string) ([]campaigns.Purchase, error) {
	query := `SELECT ` + purchaseColumnsAliased + `
		FROM campaign_purchases p
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE p.campaign_id = ? AND s.id IS NULL
		ORDER BY p.purchase_date DESC`
	rows, err := r.db.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (campaigns.Purchase, error) {
		var p campaigns.Purchase
		err := scanPurchase(rs, &p)
		return p, err
	})
}

func (r *CampaignsRepository) ListAllUnsoldPurchases(ctx context.Context) ([]campaigns.Purchase, error) {
	query := `SELECT ` + purchaseColumnsAliased + `
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL AND c.phase != 'closed'
		ORDER BY c.created_at DESC, p.purchase_date DESC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (campaigns.Purchase, error) {
		var p campaigns.Purchase
		err := scanPurchase(rs, &p)
		return p, err
	})
}

// UnsoldCardInfo holds card name, set name, and card number for unsold purchases.
type UnsoldCardInfo struct {
	CardName   string
	SetName    string
	CardNumber string
}

// ListUnsoldCards returns distinct card name + set name + card number triples from unsold purchases
// across all active (non-archived) campaigns. Used for background batch processing.
// Names and sets are normalized before deduplication so formatting variants collapse into one entry.
func (r *CampaignsRepository) ListUnsoldCards(ctx context.Context) ([]UnsoldCardInfo, error) {
	query := `
		SELECT DISTINCT p.card_name, p.set_name, p.card_number
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL AND c.phase != 'closed'
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	raw, err := scanRows(ctx, rows, func(rs *sql.Rows) (UnsoldCardInfo, error) {
		var info UnsoldCardInfo
		err := rs.Scan(&info.CardName, &info.SetName, &info.CardNumber)
		return info, err
	})
	if err != nil {
		return nil, err
	}

	// Normalize and dedupe so formatting variants (e.g. "REV.FOIL" vs "Reverse Foil") collapse.
	seen := make(map[string]struct{}, len(raw))
	result := make([]UnsoldCardInfo, 0, len(raw))
	for _, info := range raw {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		normName := cardutil.SimplifyForSearch(cardutil.NormalizePurchaseName(info.CardName))
		normSet := cardutil.NormalizeSetNameForSearch(info.SetName)
		key := normName + "|" + normSet + "|" + info.CardNumber
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, UnsoldCardInfo{
			CardName:   normName,
			SetName:    normSet,
			CardNumber: info.CardNumber,
		})
	}
	return result, nil
}

// ListUnmappedPurchaseCerts returns cert numbers of unsold purchases that don't have
// a card_id_mapping for the given provider and aren't tracked as missing in card_request_submissions.
// When grader is non-empty, only purchases with that grader are returned.
func (r *CampaignsRepository) ListUnmappedPurchaseCerts(ctx context.Context, provider string, grader string) ([]string, error) {
	query := `
		SELECT DISTINCT cp.cert_number
		FROM campaign_purchases cp
		LEFT JOIN campaign_sales cs ON cs.purchase_id = cp.id
		WHERE cp.was_refunded = 0
		  AND cs.id IS NULL
		  AND cp.cert_number != ''
		  AND (? = '' OR cp.grader = ?)
		  AND NOT EXISTS (
		    SELECT 1 FROM card_id_mappings m
		    WHERE m.provider = ?
		      AND m.collector_number = cp.card_number
		      AND m.set_name = cp.set_name
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM card_request_submissions crs
		    WHERE crs.cert_number = cp.cert_number
		  )
	`
	rows, err := r.db.QueryContext(ctx, query, grader, grader, provider)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // best-effort close

	var certs []string
	for rows.Next() {
		var cert string
		if err := rows.Scan(&cert); err != nil {
			return certs, err
		}
		certs = append(certs, cert)
	}
	return certs, rows.Err()
}

func (r *CampaignsRepository) CountPurchasesByCampaign(ctx context.Context, campaignID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM campaign_purchases WHERE campaign_id = ?`, campaignID,
	).Scan(&count)
	return count, err
}

func (r *CampaignsRepository) UpdatePurchaseCLValue(ctx context.Context, id string, clValueCents int, population int) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET cl_value_cents = ?, population = ?, updated_at = ? WHERE id = ?`,
		clValueCents, population, time.Now(), id,
	)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return campaigns.ErrPurchaseNotFound
	}
	return nil
}

func (r *CampaignsRepository) UpdatePurchaseCardMetadata(ctx context.Context, id string, cardName, cardNumber, setName string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET card_name = ?, card_number = ?, set_name = ?, updated_at = ? WHERE id = ?`,
		cardName, cardNumber, setName, time.Now(), id,
	)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return campaigns.ErrPurchaseNotFound
	}
	return nil
}

func (r *CampaignsRepository) UpdatePurchaseGrade(ctx context.Context, id string, gradeValue float64) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET grade_value = ?, updated_at = ? WHERE id = ?`,
		gradeValue, time.Now(), id,
	)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return campaigns.ErrPurchaseNotFound
	}
	return nil
}

func (r *CampaignsRepository) UpdatePurchaseBuyCost(ctx context.Context, id string, buyCostCents int) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET buy_cost_cents = ?, updated_at = ? WHERE id = ?`,
		buyCostCents, time.Now(), id,
	)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return campaigns.ErrPurchaseNotFound
	}
	return nil
}

func (r *CampaignsRepository) UpdatePurchaseCampaign(ctx context.Context, purchaseID string, campaignID string, sourcingFeeCents int) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET campaign_id = ?, psa_sourcing_fee_cents = ?, updated_at = ? WHERE id = ?`,
		campaignID, sourcingFeeCents, time.Now(), purchaseID,
	)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return campaigns.ErrPurchaseNotFound
	}
	return nil
}

func (r *CampaignsRepository) UpdatePurchaseCardYear(ctx context.Context, id string, year string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases SET card_year = ? WHERE id = ?`,
		year, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return campaigns.ErrPurchaseNotFound
	}
	return nil
}

// UpdatePurchaseDHFields updates DH v2 tracking fields on a purchase.
func (r *CampaignsRepository) UpdatePurchaseDHFields(ctx context.Context, id string, update campaigns.DHFieldsUpdate) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE campaign_purchases
		 SET dh_card_id = ?, dh_inventory_id = ?, dh_cert_status = ?,
		     dh_listing_price_cents = ?, dh_channels_json = ?, updated_at = ?
		 WHERE id = ?`,
		update.CardID, update.InventoryID, update.CertStatus, update.ListingPriceCents, update.ChannelsJSON, time.Now(), id,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return campaigns.ErrPurchaseNotFound
	}
	return nil
}

// GetPurchasesByDHCertStatus returns purchases with the given DH cert resolution status.
func (r *CampaignsRepository) GetPurchasesByDHCertStatus(ctx context.Context, status string, limit int) ([]campaigns.Purchase, error) {
	query := fmt.Sprintf(
		`SELECT %s FROM campaign_purchases WHERE dh_cert_status = ? ORDER BY updated_at ASC LIMIT ?`,
		purchaseColumns,
	)
	rows, err := r.db.QueryContext(ctx, query, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // best-effort close

	var purchases []campaigns.Purchase
	for rows.Next() {
		var p campaigns.Purchase
		if err := scanPurchase(rows, &p); err != nil {
			return nil, err
		}
		purchases = append(purchases, p)
	}
	return purchases, rows.Err()
}

// GetPurchaseIDByCertNumber returns the purchase ID for a given cert number.
func (r *CampaignsRepository) GetPurchaseIDByCertNumber(ctx context.Context, certNumber string) (string, error) {
	var id string
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM campaign_purchases WHERE cert_number = ?`, certNumber,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}
