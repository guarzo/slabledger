package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// SaleStore implements inventory.SaleRepository operations.
type SaleStore struct {
	base
}

// NewSaleStore creates a new Sale store.
func NewSaleStore(db *sql.DB, logger observability.Logger) *SaleStore {
	return &SaleStore{base{db: db, logger: logger}}
}

var _ inventory.SaleRepository = (*SaleStore)(nil)

func (ss *SaleStore) CreateSale(ctx context.Context, s *inventory.Sale) error {
	query := `
		INSERT INTO campaign_sales (` + saleColumns + `)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36)
	`
	_, err := ss.db.ExecContext(ctx, query,
		s.ID, s.PurchaseID, string(s.SaleChannel), s.SalePriceCents,
		s.SaleFeeCents, s.SaleDate, s.DaysToSell, s.NetProfitCents,
		s.CreatedAt, s.UpdatedAt,
		s.LastSoldCents, s.LowestListCents, s.ConservativeCents, s.MedianCents,
		s.ActiveListings, s.SalesLast30d, s.Trend30d, s.SnapshotDate, s.SnapshotJSON,
		s.OriginalListPriceCents, s.PriceReductions, s.DaysListed, s.SoldAtAskingPrice,
		s.WasCracked, s.OrderID, s.ForcedLiquidation,
		s.SaleReason, s.CLValueAtSaleCents, s.ChannelFeePctAtSale,
		s.TheirCompCents, s.PriceSource, s.CLValueAtSaleObservedAt, s.CLValueAtSaleSource,
		s.DHIdempotencyKey, s.DHSaleID, s.DHSaleRecordedAt,
	)
	if err != nil && isUniqueConstraintError(err) {
		return inventory.ErrDuplicateSale
	}
	if err != nil {
		return fmt.Errorf("create sale: %w", err)
	}
	return nil
}

func (ss *SaleStore) GetSaleByPurchaseID(ctx context.Context, purchaseID string) (*inventory.Sale, error) {
	query := `SELECT ` + saleColumns + ` FROM campaign_sales WHERE purchase_id = $1`
	s, err := scanSale(ss.db.QueryRowContext(ctx, query, purchaseID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, inventory.ErrSaleNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (ss *SaleStore) GetSalesByPurchaseIDs(ctx context.Context, purchaseIDs []string) (map[string]*inventory.Sale, error) {
	if len(purchaseIDs) == 0 {
		return map[string]*inventory.Sale{}, nil
	}

	// Chunk to stay under Postgres's parameter limit.
	// Matches the chunkSize used by GetPurchasesByCertNumbers / GetPurchasesByIDs.
	const chunkSize = 500
	result := make(map[string]*inventory.Sale, len(purchaseIDs))

	for start := 0; start < len(purchaseIDs); start += chunkSize {
		end := min(start+chunkSize, len(purchaseIDs))
		chunk := purchaseIDs[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, id := range chunk {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = id
		}

		query := `SELECT ` + saleColumns + ` FROM campaign_sales WHERE purchase_id IN (` + strings.Join(placeholders, ",") + `)`
		if err := ss.scanSalesChunk(ctx, query, args, result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (ss *SaleStore) scanSalesChunk(ctx context.Context, query string, args []any, into map[string]*inventory.Sale) (err error) {
	rows, err := ss.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query sales by purchase ids chunk: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = fmt.Errorf("close sales rows in purchase ids chunk: %w", cerr)
		}
	}()
	for rows.Next() {
		s, scanErr := scanSale(rows)
		if scanErr != nil {
			return fmt.Errorf("scan sale row in purchase ids chunk: %w", scanErr)
		}
		into[s.PurchaseID] = &s
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate sales rows in purchase ids chunk: %w", err)
	}
	return nil
}

func (ss *SaleStore) ListSalesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]inventory.Sale, error) {
	query := `
		SELECT ` + saleColumns + `
		FROM campaign_sales
		WHERE purchase_id IN (SELECT id FROM campaign_purchases WHERE campaign_id = $1)
		ORDER BY sale_date DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := ss.db.QueryContext(ctx, query, campaignID, limit, offset)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.Sale, error) {
		return scanSale(rs)
	})
}

func (ss *SaleStore) DeleteSale(ctx context.Context, saleID string) error {
	result, err := ss.db.ExecContext(ctx, `DELETE FROM campaign_sales WHERE id = $1`, saleID)
	if err != nil {
		return fmt.Errorf("delete sale: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return inventory.ErrSaleNotFound
	}
	return nil
}

func (ss *SaleStore) UpdateSaleReason(ctx context.Context, campaignID, saleID, reason string, forcedLiquidation bool) error {
	result, err := ss.db.ExecContext(ctx,
		`UPDATE campaign_sales SET sale_reason = $1, forced_liquidation = $2, updated_at = $3
		 WHERE id = $4 AND purchase_id IN (SELECT id FROM campaign_purchases WHERE campaign_id = $5)`,
		reason, forcedLiquidation, time.Now(), saleID, campaignID)
	if err != nil {
		return fmt.Errorf("update sale reason: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update sale reason rows affected: %w", err)
	}
	if n == 0 {
		return inventory.ErrSaleNotFound
	}
	return nil
}

func (ss *SaleStore) DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error {
	result, err := ss.db.ExecContext(ctx, `DELETE FROM campaign_sales WHERE purchase_id = $1`, purchaseID)
	if err != nil {
		return fmt.Errorf("delete sale by purchase id: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return inventory.ErrSaleNotFound
	}
	return nil
}

// SetSaleIdempotencyKeyIfAbsent mints an idempotency key for a sale that has
// none, via compare-and-set (spec §5a). It always returns the EFFECTIVE key:
// the one it just wrote, or — if a concurrent caller won the race — the one
// that caller wrote. Two callers can therefore never send two different keys
// for the same sale to DH.
func (ss *SaleStore) SetSaleIdempotencyKeyIfAbsent(ctx context.Context, saleID, key string) (string, error) {
	var effective string
	err := ss.db.QueryRowContext(ctx, `
		UPDATE campaign_sales
		   SET dh_idempotency_key = $1
		 WHERE id = $2 AND dh_idempotency_key = ''
		RETURNING dh_idempotency_key`,
		key, saleID,
	).Scan(&effective)
	if err == nil {
		return effective, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("set sale idempotency key if absent: %w", err)
	}

	// Lost the race, or the sale already had a key from an earlier call:
	// re-read whatever is there now. A genuinely missing sale surfaces as
	// ErrSaleNotFound rather than handing an empty string to DH.
	var existing sql.NullString
	err = ss.db.QueryRowContext(ctx,
		`SELECT dh_idempotency_key FROM campaign_sales WHERE id = $1`, saleID,
	).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return "", inventory.ErrSaleNotFound
	}
	if err != nil {
		return "", fmt.Errorf("re-read sale idempotency key after lost race: %w", err)
	}
	return existing.String, nil
}

// SetSaleDHSaleID persists the DH-issued sale handle after a successful
// RecordInventorySale call (or a replay). Without this handle a later void
// can never reach DH (spec §5b, §7).
func (ss *SaleStore) SetSaleDHSaleID(ctx context.Context, saleID, dhSaleID string, recordedAt time.Time) error {
	result, err := ss.db.ExecContext(ctx,
		`UPDATE campaign_sales SET dh_sale_id = $1, dh_sale_recorded_at = $2, updated_at = $3 WHERE id = $4`,
		dhSaleID, recordedAt.UTC(), time.Now(), saleID,
	)
	if err != nil {
		return fmt.Errorf("set sale dh sale id: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set sale dh sale id rows affected: %w", err)
	}
	if rows == 0 {
		return inventory.ErrSaleNotFound
	}
	return nil
}

// ListSalesNeedingDHRecord returns sales that need a DH-side sale handle
// (spec §5b). It is scoped by OUR state, not DH's inventory listing, so it
// catches sales DH already accepted whose handle we failed to persist — a
// window the DH-inventory-scoped sweep can never see, because a successfully
// recorded sale delists the item and drops it out of that sweep's view.
//
// An empty dh_sale_conflict is the terminal-state clause, and it is
// load-bearing: only two DH error codes are retryable (spec §3); every other failure is
// permanent. Without this clause, a sale that failed with a permanent error
// (e.g. 422 idempotency_key_reused) would keep its key, never gain a handle,
// and be re-attempted every cycle forever — reproducing the hourly-422 noise
// this design exists to end, just against a new endpoint. A human clearing
// dh_sale_conflict on the purchase is what re-enrolls the row.
//
// Rows with no idempotency key are intentionally included (there is no
// non-empty-key clause) — those are the pre-migration legacy sales, including
// the 25 from the 2026-08-15 incident, which mint a key on first visit via
// SetSaleIdempotencyKeyIfAbsent (spec §5a) rather than being skipped.
//
// An empty order_id excludes sales that ORIGINATED at DH. The DH orders poller
// imports those via ConfirmOrdersSales, which writes the row through the
// repository and so never mints a key — leaving them indistinguishable, by the
// other three clauses alone, from a sale of ours that DH has not been told
// about. Recording one would report DH's own sale back to them: either a 409
// item_sold_on_channel that conflict-flags a perfectly healthy row, or a
// duplicate disposal in their ledger.
//
// This is not a migration-hygiene edge case. Measured 2026-08-20: 434 of 1584
// existing sales carry an order_id, and DH-native sales are now the MAJORITY of
// new ones (69 of 101 in August). Without this clause the recovery pass would
// misclassify the primary sales channel every cycle, and the conflict flag —
// which exists to surface real drift — would drown in false positives.
//
// Only the two DH order paths set order_id (dh_orders_poll.go,
// service_import_orders.go); nothing in CreateSale/CreateBulkSales does, so it
// is a sound origin discriminator.
//
// Consequence, accepted deliberately: a DH-native sale never gains a
// dh_sale_id, so un-selling one cannot void it on DH. That matches the
// pre-branch status quo (nothing voided at all) and cannot be closed until DH
// exposes the sale id at order-import time.
func (ss *SaleStore) ListSalesNeedingDHRecord(ctx context.Context, limit int) ([]inventory.SaleNeedingDHRecord, error) {
	query := `
		SELECT ` + saleColumnsAliased + `, p.dh_inventory_id, p.purchase_date
		FROM campaign_sales s
		JOIN campaign_purchases p ON p.id = s.purchase_id
		WHERE s.dh_sale_id = ''
		  AND s.order_id = ''
		  AND p.dh_inventory_id <> 0
		  AND p.dh_sale_conflict = ''
		ORDER BY s.created_at ASC
		LIMIT $1
	`
	rows, err := ss.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list sales needing dh record: %w", err)
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.SaleNeedingDHRecord, error) {
		var n saleNulls
		var dhInventoryID int
		var purchaseDate string
		dests := append(saleScanDests(&n), &dhInventoryID, &purchaseDate)
		if err := rs.Scan(dests...); err != nil {
			return inventory.SaleNeedingDHRecord{}, err
		}
		return inventory.SaleNeedingDHRecord{
			Sale:          n.sale(),
			DHInventoryID: dhInventoryID,
			PurchaseDate:  purchaseDate,
		}, nil
	})
}
