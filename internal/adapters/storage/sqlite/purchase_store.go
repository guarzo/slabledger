package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/cardutil"
	"strings"
	"time"
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
		INSERT INTO campaign_purchases (id, campaign_id, card_name, cert_number,
			card_number, set_name,
			grader, grade_value,
			cl_value_cents, buy_cost_cents, psa_sourcing_fee_cents,
			population, purchase_date, created_at, updated_at,
			last_sold_cents, lowest_list_cents, conservative_cents, median_cents,
			active_listings, sales_last_30d, trend_30d, snapshot_date, snapshot_json,
			received_at, psa_ship_date, invoice_date, was_refunded, front_image_url, back_image_url, purchase_source,
			psa_listing_title, snapshot_status, snapshot_retry_count,
			override_price_cents, override_source, override_set_at,
			ai_suggested_price_cents, ai_suggested_at,
			card_year, ebay_export_flagged_at,
			reviewed_price_cents, reviewed_at, review_source,
			dh_card_id, dh_inventory_id, dh_cert_status, dh_listing_price_cents, dh_channels_json, dh_status, dh_push_status, dh_candidates,
			gem_rate_id, psa_spec_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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

// ListUnmappedPurchaseCerts returns cert numbers of unsold purchases that don't have
// a card_id_mapping for the given provider and aren't tracked as missing in card_request_submissions.
// When grader is non-empty, only purchases with that grader are returned.
func (ps *PurchaseStore) ListUnmappedPurchaseCerts(ctx context.Context, provider string, grader string) ([]string, error) {
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
	rows, err := ps.db.QueryContext(ctx, query, grader, grader, provider)
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
	return ps.execAndExpectRow(ctx, "update campaign",
		`UPDATE campaign_purchases SET campaign_id = ?, psa_sourcing_fee_cents = ?, updated_at = ? WHERE id = ?`,
		campaignID, sourcingFeeCents, time.Now(), purchaseID,
	)
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

// GetPurchasesByGraderAndCertNumbers retrieves multiple purchases by grader and cert numbers.
// Large inputs are chunked to stay within SQLite's parameter limit.
// Returns a map keyed by cert number.
func (ps *PurchaseStore) GetPurchasesByGraderAndCertNumbers(ctx context.Context, grader string, certNumbers []string) (map[string]*inventory.Purchase, error) {
	if len(certNumbers) == 0 {
		return make(map[string]*inventory.Purchase), nil
	}

	const chunkSize = 500
	result := make(map[string]*inventory.Purchase, len(certNumbers))

	for start := 0; start < len(certNumbers); start += chunkSize {
		end := min(start+chunkSize, len(certNumbers))
		chunk := certNumbers[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, 0, len(chunk)+1)
		args = append(args, grader)
		for i, cn := range chunk {
			placeholders[i] = "?"
			args = append(args, cn)
		}

		query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases
			WHERE grader = ? AND cert_number IN (` + strings.Join(placeholders, ",") + `)`

		rows, err := ps.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("query purchases by grader/cert chunk: %w", err)
		}

		for rows.Next() {
			select {
			case <-ctx.Done():
				rows.Close() //nolint:errcheck // best-effort close on ctx cancel
				return nil, ctx.Err()
			default:
			}

			var p inventory.Purchase
			if err := scanPurchase(rows, &p); err != nil {
				rows.Close() //nolint:errcheck // best-effort close on scan error
				return nil, fmt.Errorf("scan purchase in grader/cert chunk: %w", err)
			}
			result[p.CertNumber] = &p
		}
		if err := rows.Err(); err != nil {
			rows.Close() //nolint:errcheck // best-effort close on iteration error
			return nil, fmt.Errorf("iterate purchases by grader/cert: %w", err)
		}
		if cerr := rows.Close(); cerr != nil {
			return nil, fmt.Errorf("close purchases by grader/cert rows: %w", cerr)
		}
	}
	return result, nil
}

// GetPurchasesByCertNumbers retrieves purchases by cert numbers across all graders.
// Large inputs are chunked to stay within SQLite's parameter limit.
func (ps *PurchaseStore) GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*inventory.Purchase, error) {
	if len(certNumbers) == 0 {
		return make(map[string]*inventory.Purchase), nil
	}

	const chunkSize = 500
	result := make(map[string]*inventory.Purchase, len(certNumbers))

	for start := 0; start < len(certNumbers); start += chunkSize {
		end := min(start+chunkSize, len(certNumbers))
		chunk := certNumbers[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, cn := range chunk {
			placeholders[i] = "?"
			args[i] = cn
		}

		query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases
			WHERE cert_number IN (` + strings.Join(placeholders, ",") + `)`

		rows, err := ps.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("query purchases by cert chunk: %w", err)
		}

		for rows.Next() {
			select {
			case <-ctx.Done():
				rows.Close() //nolint:errcheck // best-effort close on ctx cancel
				return nil, ctx.Err()
			default:
			}

			var p inventory.Purchase
			if err := scanPurchase(rows, &p); err != nil {
				rows.Close() //nolint:errcheck // best-effort close on scan error
				return nil, fmt.Errorf("scan purchase in cert chunk: %w", err)
			}
			result[p.CertNumber] = &p
		}
		if err := rows.Err(); err != nil {
			rows.Close() //nolint:errcheck // best-effort close on iteration error
			return nil, fmt.Errorf("iterate purchases by cert: %w", err)
		}
		if cerr := rows.Close(); cerr != nil {
			return nil, fmt.Errorf("close purchases by cert rows: %w", cerr)
		}
	}

	return result, nil
}

// GetPurchasesByIDs retrieves multiple purchases by their IDs in a single query.
// Large inputs are chunked to stay within SQLite's parameter limit.
func (ps *PurchaseStore) GetPurchasesByIDs(ctx context.Context, ids []string) (map[string]*inventory.Purchase, error) {
	if len(ids) == 0 {
		return make(map[string]*inventory.Purchase), nil
	}

	const chunkSize = 500
	result := make(map[string]*inventory.Purchase, len(ids))

	for start := 0; start < len(ids); start += chunkSize {
		end := min(start+chunkSize, len(ids))
		chunk := ids[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, id := range chunk {
			placeholders[i] = "?"
			args[i] = id
		}

		query := `SELECT ` + purchaseColumns + ` FROM campaign_purchases WHERE id IN (` + strings.Join(placeholders, ",") + `)`
		rows, err := ps.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("query purchases by IDs chunk: %w", err)
		}

		purchases, err := scanRows(ctx, rows, func(rs *sql.Rows) (inventory.Purchase, error) {
			var p inventory.Purchase
			err := scanPurchase(rs, &p)
			return p, err
		})
		if err != nil {
			return nil, fmt.Errorf("scan purchases by IDs chunk: %w", err)
		}
		for i := range purchases {
			result[purchases[i].ID] = &purchases[i]
		}
	}

	return result, nil
}

func (ps *PurchaseStore) SetReceivedAt(ctx context.Context, purchaseID string, receivedAt time.Time) error {
	return ps.execAndExpectRow(ctx, "set received_at",
		`UPDATE campaign_purchases SET received_at = ?, updated_at = ? WHERE id = ?`,
		receivedAt, time.Now().UTC(), purchaseID,
	)
}

// GetPurchaseIDByCertNumber returns the purchase ID for a given cert number.
func (ps *PurchaseStore) GetPurchaseIDByCertNumber(ctx context.Context, certNumber string) (string, error) {
	var id string
	err := ps.db.QueryRowContext(ctx,
		`SELECT id FROM campaign_purchases WHERE cert_number = ?`, certNumber,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (ps *PurchaseStore) DeletePurchase(ctx context.Context, id string) (retErr error) {
	tx, err := ps.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
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
		retErr = fmt.Errorf("delete purchase: %w", err)
		return retErr
	}
	n, err := result.RowsAffected()
	if err != nil {
		retErr = fmt.Errorf("check rows affected: %w", err)
		return retErr
	}
	if n == 0 {
		retErr = inventory.ErrPurchaseNotFound
		return retErr
	}

	return tx.Commit()
}
