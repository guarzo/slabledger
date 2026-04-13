package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/cardutil"
)

// PurchaseStore implements inventory.PurchaseRepository operations.
type PurchaseStore struct {
	base
}

// NewPurchaseStore creates a new purchase store.
func NewPurchaseStore(db *sql.DB, logger observability.Logger) *PurchaseStore {
	return &PurchaseStore{base{db: db, logger: logger}}
}

var _ inventory.PurchaseRepository = (*PurchaseStore)(nil)

// UnsoldCardInfo represents a distinct unsold card (deduped by name, set, number).
type UnsoldCardInfo struct {
	CardName   string
	SetName    string
	CardNumber string
}

func (ps *PurchaseStore) CreatePurchase(ctx context.Context, p *inventory.Purchase) error {
	if p.Grader == "" {
		p.Grader = "PSA"
	}
	query := `
		INSERT INTO campaign_purchases (
			-- identity
			id, campaign_id, card_name, cert_number, card_number, set_name, grader, grade_value,
			-- costs
			cl_value_cents, buy_cost_cents, psa_sourcing_fee_cents,
			-- dates
			population, purchase_date, created_at, updated_at,
			-- market snapshot
			last_sold_cents, lowest_list_cents, conservative_cents, median_cents,
			active_listings, sales_last_30d, trend_30d, snapshot_date, snapshot_json,
			-- provenance
			received_at, psa_ship_date, invoice_date, was_refunded, front_image_url, back_image_url, purchase_source,
			-- PSA
			psa_listing_title, snapshot_status, snapshot_retry_count,
			-- price overrides
			override_price_cents, override_source, override_set_at,
			-- AI suggestions
			ai_suggested_price_cents, ai_suggested_at,
			-- misc
			card_year, ebay_export_flagged_at,
			-- review
			reviewed_price_cents, reviewed_at, review_source,
			-- DH
			dh_card_id, dh_inventory_id, dh_cert_status, dh_listing_price_cents, dh_channels_json, dh_status, dh_push_status, dh_candidates,
			-- gem/spec
			gem_rate_id, psa_spec_id
		) VALUES (
			-- identity
			?, ?, ?, ?, ?, ?, ?, ?,
			-- costs
			?, ?, ?,
			-- dates
			?, ?, ?, ?,
			-- market snapshot
			?, ?, ?, ?, ?, ?, ?, ?, ?,
			-- provenance
			?, ?, ?, ?, ?, ?, ?,
			-- PSA
			?, ?, ?,
			-- price overrides
			?, ?, ?,
			-- AI suggestions
			?, ?,
			-- misc
			?, ?,
			-- review
			?, ?, ?,
			-- DH
			?, ?, ?, ?, ?, ?, ?, ?,
			-- gem/spec
			?, ?
		)
	`
	_, err := ps.db.ExecContext(ctx, query,
		p.ID, p.CampaignID, p.CardName, p.CertNumber,
		p.CardNumber, p.SetName,
		p.Grader, p.GradeValue,
		p.CLValueCents, p.BuyCostCents, p.PSASourcingFeeCents,
		p.Population, p.PurchaseDate, p.CreatedAt, p.UpdatedAt,
		p.LastSoldCents, p.LowestListCents, p.ConservativeCents, p.MedianCents,
		p.ActiveListings, p.SalesLast30d, p.Trend30d, p.SnapshotDate, p.SnapshotJSON,
		p.ReceivedAt, p.PSAShipDate, p.InvoiceDate, p.WasRefunded, p.FrontImageURL, p.BackImageURL, p.PurchaseSource,
		p.PSAListingTitle, p.SnapshotStatus, p.SnapshotRetryCount,
		p.OverridePriceCents, p.OverrideSource, p.OverrideSetAt,
		p.AISuggestedPriceCents, p.AISuggestedAt,
		p.CardYear, p.EbayExportFlaggedAt,
		p.ReviewedPriceCents, p.ReviewedAt, string(p.ReviewSource),
		p.DHCardID, p.DHInventoryID, p.DHCertStatus, p.DHListingPriceCents, p.DHChannelsJSON, p.DHStatus, p.DHPushStatus, p.DHCandidatesJSON,
		p.GemRateID, p.PSASpecID,
	)
	if err != nil && isUniqueConstraintError(err) {
		return inventory.ErrDuplicateCertNumber
	}
	if err != nil {
		return fmt.Errorf("create purchase: %w", err)
	}
	return nil
}

func (ps *PurchaseStore) GetPurchase(ctx context.Context, id string) (*inventory.Purchase, error) {
	query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases WHERE id = ?`
	var p inventory.Purchase
	err := scanPurchase(ps.db.QueryRowContext(ctx, query, id), &p)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, inventory.ErrPurchaseNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (ps *PurchaseStore) ListPurchasesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]inventory.Purchase, error) {
	query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases WHERE campaign_id = ?
		ORDER BY purchase_date DESC
		LIMIT ? OFFSET ?`
	rows, err := ps.db.QueryContext(ctx, query, campaignID, limit, offset)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.Purchase, error) {
		var p inventory.Purchase
		err := scanPurchase(rs, &p)
		return p, err
	})
}

func (ps *PurchaseStore) ListUnsoldPurchases(ctx context.Context, campaignID string) ([]inventory.Purchase, error) {
	query := `SELECT ` + purchaseColumnsAliased + `
		FROM campaign_purchases p
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE p.campaign_id = ? AND s.id IS NULL
		ORDER BY p.purchase_date DESC`
	rows, err := ps.db.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.Purchase, error) {
		var p inventory.Purchase
		err := scanPurchase(rs, &p)
		return p, err
	})
}

func (ps *PurchaseStore) ListAllUnsoldPurchases(ctx context.Context) ([]inventory.Purchase, error) {
	query := `SELECT ` + purchaseColumnsAliased + `
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL AND c.phase != 'closed'
		ORDER BY c.created_at DESC, p.purchase_date DESC`
	rows, err := ps.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.Purchase, error) {
		var p inventory.Purchase
		err := scanPurchase(rs, &p)
		return p, err
	})
}

// ListUnsoldCards returns distinct card name + set name + card number triples from unsold purchases
// across all active (non-archived) inventory. Used for background batch processing.
// Names and sets are normalized before deduplication so formatting variants collapse into one entry.
func (ps *PurchaseStore) ListUnsoldCards(ctx context.Context) ([]UnsoldCardInfo, error) {
	query := `
		SELECT DISTINCT p.card_name, p.set_name, p.card_number
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL AND c.phase != 'closed'
	`
	rows, err := ps.db.QueryContext(ctx, query)
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

func (ps *PurchaseStore) CountPurchasesByCampaign(ctx context.Context, campaignID string) (int, error) {
	var count int
	err := ps.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM campaign_purchases WHERE campaign_id = ?`, campaignID,
	).Scan(&count)
	return count, err
}

// execAndExpectRow runs a write query and returns ErrPurchaseNotFound if no row was affected.
func (ps *PurchaseStore) execAndExpectRow(ctx context.Context, op, query string, args ...any) error {
	result, err := ps.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: rows affected: %w", op, err)
	}
	if n == 0 {
		return inventory.ErrPurchaseNotFound
	}
	return nil
}

func (ps *PurchaseStore) UpdatePurchaseCLValue(ctx context.Context, id string, clValueCents int, population int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	return ps.execAndExpectRow(ctx, "update cl value",
		`UPDATE campaign_purchases SET cl_value_cents = ?, population = ?, cl_value_updated_at = ?, updated_at = ? WHERE id = ?`,
		clValueCents, population, now, now, id,
	)
}

// UpdatePurchaseMMError records or clears the last mapping/pricing failure reason
// for a purchase. Pass reason="" to clear on success. reasonAt is normalized
// to "" whenever reason is "" so the timestamp never lags behind the tag.
func (ps *PurchaseStore) UpdatePurchaseMMError(ctx context.Context, id, reason, reasonAt string) error {
	if reason == "" {
		reasonAt = ""
	}
	return ps.execAndExpectRow(ctx, "update mm last error",
		`UPDATE campaign_purchases SET mm_last_error = ?, mm_last_error_at = ?, updated_at = ? WHERE id = ?`,
		reason, reasonAt, time.Now().UTC().Format(time.RFC3339), id,
	)
}

// UpdatePurchaseCLError records or clears the last mapping/pricing failure reason
// for a purchase. Pass reason="" to clear on success. reasonAt is normalized
// to "" whenever reason is "" so the timestamp never lags behind the tag.
func (ps *PurchaseStore) UpdatePurchaseCLError(ctx context.Context, id, reason, reasonAt string) error {
	if reason == "" {
		reasonAt = ""
	}
	return ps.execAndExpectRow(ctx, "update cl last error",
		`UPDATE campaign_purchases SET cl_last_error = ?, cl_last_error_at = ?, updated_at = ? WHERE id = ?`,
		reason, reasonAt, time.Now().UTC().Format(time.RFC3339), id,
	)
}

// UpdatePurchaseCLSyncedAt sets the cl_synced_at timestamp for a purchase,
// indicating when the card was last pushed/synced to the CL Firestore collection.
func (ps *PurchaseStore) UpdatePurchaseCLSyncedAt(ctx context.Context, id string, syncedAt string) error {
	return ps.execAndExpectRow(ctx, "update cl_synced_at",
		`UPDATE campaign_purchases SET cl_synced_at = ?, updated_at = ? WHERE id = ?`,
		syncedAt, time.Now().UTC().Format(time.RFC3339), id,
	)
}

func (ps *PurchaseStore) UpdatePurchaseMMValue(ctx context.Context, id string, mmValueCents int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	return ps.execAndExpectRow(ctx, "update mm value",
		`UPDATE campaign_purchases SET mm_value_cents = ?, mm_value_updated_at = ?, updated_at = ? WHERE id = ?`,
		mmValueCents, now, now, id,
	)
}

// UpdatePurchaseMMSignals writes all Market Movers signals in a single statement.
// Used by the daily MM refresh scheduler. mmValueCents is the 30-day count-weighted
// average, mmTrendPct is the 30-day price-change fraction (e.g. 0.15 = +15%),
// mmSales30d is the total sales count over 30 days, and mmActiveLowCents is the
// lowest active BIN listing price (0 if no active listings found).
func (ps *PurchaseStore) UpdatePurchaseMMSignals(
	ctx context.Context,
	id string,
	mmValueCents int,
	mmTrendPct float64,
	mmSales30d int,
	mmActiveLowCents int,
) error {
	now := time.Now().UTC().Format(time.RFC3339)
	return ps.execAndExpectRow(ctx, "update mm signals",
		`UPDATE campaign_purchases
		 SET mm_value_cents = ?, mm_trend_pct = ?, mm_sales_30d = ?, mm_active_low_cents = ?, mm_value_updated_at = ?, updated_at = ?
		 WHERE id = ?`,
		mmValueCents, mmTrendPct, mmSales30d, mmActiveLowCents, now, now, id,
	)
}

func (ps *PurchaseStore) UpdatePurchaseCardMetadata(ctx context.Context, id string, cardName, cardNumber, setName string) error {
	return ps.execAndExpectRow(ctx, "update card metadata",
		`UPDATE campaign_purchases SET card_name = ?, card_number = ?, set_name = ?, updated_at = ? WHERE id = ?`,
		cardName, cardNumber, setName, time.Now(), id,
	)
}

func (ps *PurchaseStore) UpdatePurchaseGrade(ctx context.Context, id string, gradeValue float64) error {
	return ps.execAndExpectRow(ctx, "update grade",
		`UPDATE campaign_purchases SET grade_value = ?, updated_at = ? WHERE id = ?`,
		gradeValue, time.Now(), id,
	)
}

func (ps *PurchaseStore) UpdatePurchaseBuyCost(ctx context.Context, id string, buyCostCents int) error {
	return ps.execAndExpectRow(ctx, "update buy cost",
		`UPDATE campaign_purchases SET buy_cost_cents = ?, updated_at = ? WHERE id = ?`,
		buyCostCents, time.Now(), id,
	)
}

func (ps *PurchaseStore) UpdatePurchaseCampaign(ctx context.Context, purchaseID string, campaignID string, sourcingFeeCents int) error {
	// Conditional update: only reassign if no linked sale exists.
	// This prevents a TOCTOU race between checking for sales and updating the campaign.
	result, err := ps.db.ExecContext(ctx,
		`UPDATE campaign_purchases
		 SET campaign_id = ?, psa_sourcing_fee_cents = ?, updated_at = ?
		 WHERE id = ?
		   AND NOT EXISTS (SELECT 1 FROM campaign_sales WHERE purchase_id = ?)`,
		campaignID, sourcingFeeCents, time.Now(), purchaseID, purchaseID,
	)
	if err != nil {
		return fmt.Errorf("update campaign: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update campaign: rows affected: %w", err)
	}
	if n == 0 {
		// Distinguish "not found" from "has a linked sale".
		var exists int
		if qErr := ps.db.QueryRowContext(ctx,
			`SELECT 1 FROM campaign_purchases WHERE id = ?`, purchaseID,
		).Scan(&exists); qErr != nil {
			return inventory.ErrPurchaseNotFound
		}
		return inventory.ErrPurchaseHasSale
	}
	return nil
}

func (ps *PurchaseStore) UpdatePurchaseCardYear(ctx context.Context, id string, year string) error {
	return ps.execAndExpectRow(ctx, "update card year",
		`UPDATE campaign_purchases SET card_year = ?, updated_at = ? WHERE id = ?`,
		year, time.Now(), id,
	)
}
func (ps *PurchaseStore) GetPurchaseByCertNumber(ctx context.Context, grader string, certNumber string) (*inventory.Purchase, error) {
	query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases WHERE grader = ? AND cert_number = ?`
	var p inventory.Purchase
	err := scanPurchase(ps.db.QueryRowContext(ctx, query, grader, certNumber), &p)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, inventory.ErrPurchaseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan purchase by cert %s/%s: %w", grader, certNumber, err)
	}
	return &p, nil
}
