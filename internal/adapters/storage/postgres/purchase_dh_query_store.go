package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

const receivedOrShippedPredicate = `(p.received_at IS NOT NULL OR p.psa_ship_date != '')`

// GetPurchasesByDHPushStatus returns received or shipped, unsold purchases with the given DH push status.
func (ps *PurchaseStore) GetPurchasesByDHPushStatus(ctx context.Context, status string, limit int) ([]inventory.Purchase, error) {
	query := fmt.Sprintf(
		`SELECT %s FROM campaign_purchases p
		 LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		 WHERE p.dh_push_status = $1
		   AND %s
		   AND s.id IS NULL
		 ORDER BY p.updated_at ASC LIMIT $2`,
		purchaseColumnsAliased, receivedOrShippedPredicate,
	)
	rows, err := ps.db.QueryContext(ctx, query, status, limit)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.Purchase, error) {
		var p inventory.Purchase
		err := scanPurchase(rs, &p)
		return p, err
	})
}

// GetPurchasesByDHCertStatus returns purchases with the given DH cert resolution status.
func (ps *PurchaseStore) GetPurchasesByDHCertStatus(ctx context.Context, status string, limit int) ([]inventory.Purchase, error) {
	query := fmt.Sprintf(
		`SELECT %s FROM campaign_purchases WHERE dh_cert_status = $1 ORDER BY updated_at ASC LIMIT $2`,
		purchaseColumns,
	)
	rows, err := ps.db.QueryContext(ctx, query, status, limit)
	if err != nil {
		return nil, err
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.Purchase, error) {
		var p inventory.Purchase
		err := scanPurchase(rs, &p)
		return p, err
	})
}

// CountUnsoldByDHPushStatus returns counts of unsold purchases grouped by dh_push_status.
// Purchases with no status are reported under an empty string key; purchases with a
// DHCardID but no push status are counted as "matched" for legacy compatibility.
func (ps *PurchaseStore) CountUnsoldByDHPushStatus(ctx context.Context) (map[string]int, error) {
	query := `
		SELECT
			CASE
				WHEN p.dh_push_status != '' THEN p.dh_push_status
				WHEN p.dh_card_id != 0 THEN 'matched'
				ELSE ''
			END AS status_bucket,
			COUNT(*) AS cnt
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL AND c.phase != 'closed'
		GROUP BY status_bucket`
	rows, err := ps.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // best-effort close

	counts := make(map[string]int)
	for rows.Next() {
		var bucket string
		var cnt int
		if err := rows.Scan(&bucket, &cnt); err != nil {
			return nil, err
		}
		counts[bucket] = cnt
	}
	return counts, rows.Err()
}

// CountDHPipelineHealth returns the two counts the DH status dashboard needs to
// reconcile its aggregate with the actual queue: how many received-or-shipped
// pending rows there are (matches ListDHPendingItems' draining query) and how
// many received-or-shipped rows have never been enrolled in the push pipeline.
// Both counts exclude sold rows and rows in closed campaigns.
func (ps *PurchaseStore) CountDHPipelineHealth(ctx context.Context) (inventory.DHPipelineHealth, error) {
	var health inventory.DHPipelineHealth
	err := ps.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT
			COALESCE(SUM(CASE WHEN p.dh_push_status = 'pending' THEN 1 ELSE 0 END), 0) AS pending_received,
			COALESCE(SUM(CASE
				WHEN (p.dh_push_status IS NULL OR p.dh_push_status = '')
				     AND p.dh_inventory_id = 0
				THEN 1 ELSE 0 END), 0) AS unenrolled_received
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL
		  AND %s
		  AND c.phase != 'closed'
	`, receivedOrShippedPredicate)).Scan(&health.PendingReceived, &health.UnenrolledReceived)
	if err != nil {
		return inventory.DHPipelineHealth{}, fmt.Errorf("count dh pipeline health: %w", err)
	}
	return health, nil
}

// ListDHPendingItems returns received or shipped, unsold purchases that are queued for the
// DH push pipeline (dh_push_status = 'pending') with an active parent campaign.
func (ps *PurchaseStore) ListDHPendingItems(ctx context.Context) ([]inventory.DHPendingItem, error) {
	query := fmt.Sprintf(`
		SELECT p.id, p.card_name, p.set_name, p.grade_value,
		       p.mid_price_cents, p.dh_last_synced_at, p.created_at
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE p.dh_push_status = 'pending'
		  AND %s
		  AND s.id IS NULL
		  AND c.phase != 'closed'
		ORDER BY p.updated_at DESC`, receivedOrShippedPredicate)
	rows, err := ps.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query dh pending items: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort close

	items := make([]inventory.DHPendingItem, 0)
	now := time.Now()
	for rows.Next() {
		var (
			purchaseID     string
			cardName       string
			setName        string
			grade          float64
			midPriceCents  int
			dhLastSyncedAt string
			createdAt      time.Time
		)
		if err := rows.Scan(&purchaseID, &cardName, &setName, &grade,
			&midPriceCents, &dhLastSyncedAt, &createdAt); err != nil {
			return nil, fmt.Errorf("scan dh pending item: %w", err)
		}

		confidence := "low"
		var daysQueued int
		if dhLastSyncedAt != "" {
			if syncedAt, parseErr := time.Parse(time.RFC3339, dhLastSyncedAt); parseErr == nil {
				elapsed := now.Sub(syncedAt)
				daysQueued = int(elapsed.Hours() / 24)
				switch {
				case elapsed < 24*time.Hour:
					confidence = "high"
				case elapsed < 7*24*time.Hour:
					confidence = "medium"
				default:
					confidence = "low"
				}
			} else {
				// Unparseable timestamp — fall back to created_at age.
				daysQueued = int(now.Sub(createdAt).Hours() / 24)
			}
		} else {
			daysQueued = int(now.Sub(createdAt).Hours() / 24)
		}

		items = append(items, inventory.DHPendingItem{
			PurchaseID:            purchaseID,
			CardName:              cardName,
			SetName:               setName,
			Grade:                 grade,
			RecommendedPriceCents: midPriceCents,
			DaysQueued:            daysQueued,
			DHConfidence:          confidence,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dh pending items: %w", err)
	}
	return items, nil
}

// ListUnsoldDHCardIDs returns the distinct dh_card_id values for unsold purchases
// in non-closed campaigns. Used by the DH demand analytics refresh scheduler to
// seed per-card analytics calls with our own inventory. Rows where dh_card_id is
// zero or NULL are skipped.
func (ps *PurchaseStore) ListUnsoldDHCardIDs(ctx context.Context) ([]int, error) {
	query := `
		SELECT DISTINCT p.dh_card_id
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL
		  AND c.phase != 'closed'
		  AND p.dh_card_id IS NOT NULL
		  AND p.dh_card_id != 0`
	rows, err := ps.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list unsold dh card ids: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort close

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan dh card id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dh card ids: %w", err)
	}
	return ids, nil
}

// ListUnsoldDHCardSeeds returns distinct (dh_card_id, card_name, set_name, card_number)
// tuples for unsold purchases in non-closed campaigns where dh_card_id is set.
// Used by the DH intelligence refresh scheduler to seed the market_intelligence
// table with cards we own but for which intelligence has not yet been fetched.
func (ps *PurchaseStore) ListUnsoldDHCardSeeds(ctx context.Context) ([]intelligence.SeedCandidate, error) {
	query := `
		SELECT p.dh_card_id, p.card_name, p.set_name, p.card_number
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL
		  AND c.phase != 'closed'
		  AND p.dh_card_id IS NOT NULL
		  AND p.dh_card_id != 0
		  AND NOT EXISTS (
		    SELECT 1 FROM market_intelligence m WHERE m.dh_card_id = CAST(p.dh_card_id AS TEXT)
		  )
		GROUP BY p.dh_card_id, p.card_name, p.set_name, p.card_number`
	rows, err := ps.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list unsold dh card seeds: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort close

	var seeds []intelligence.SeedCandidate
	for rows.Next() {
		var (
			id     int
			name   string
			set    string
			number string
		)
		if err := rows.Scan(&id, &name, &set, &number); err != nil {
			return nil, fmt.Errorf("scan dh card seed: %w", err)
		}
		seeds = append(seeds, intelligence.SeedCandidate{
			DHCardID:   strconv.Itoa(id),
			CardName:   name,
			SetName:    set,
			CardNumber: number,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dh card seeds: %w", err)
	}
	return seeds, nil
}

// UpdatePurchaseGemRateID sets the CL gemRateID on a purchase.
func (ps *PurchaseStore) UpdatePurchaseGemRateID(ctx context.Context, id, gemRateID string) error {
	return ps.execAndExpectRow(ctx, "update gem rate ID",
		`UPDATE campaign_purchases SET gem_rate_id = $1, updated_at = $2 WHERE id = $3`,
		gemRateID, time.Now(), id,
	)
}

// UpdatePurchasePSASpecID sets the PSA spec ID on a purchase.
func (ps *PurchaseStore) UpdatePurchasePSASpecID(ctx context.Context, id string, psaSpecID int) error {
	return ps.execAndExpectRow(ctx, "update PSA spec ID",
		`UPDATE campaign_purchases SET psa_spec_id = $1, updated_at = $2 WHERE id = $3`,
		psaSpecID, time.Now(), id,
	)
}

// UpdatePurchaseCLCardMetadata persists Card Ladder catalog metadata (player, variation, category) on a purchase.
func (ps *PurchaseStore) UpdatePurchaseCLCardMetadata(ctx context.Context, id, player, variation, category string) error {
	return ps.execAndExpectRow(ctx, "update CL card metadata",
		`UPDATE campaign_purchases SET card_player = $1, card_variation = $2, card_category = $3, updated_at = $4 WHERE id = $5`,
		player, variation, category, time.Now(), id,
	)
}

// UpdatePurchaseDHPriceSync updates only dh_listing_price_cents and
// dh_last_synced_at. Used by the DH price re-sync flow after a successful
// DH PATCH; avoids the full-field overwrite of UpdatePurchaseDHFields.
func (ps *PurchaseStore) UpdatePurchaseDHPriceSync(ctx context.Context, id string, listingPriceCents int, syncedAt time.Time) error {
	return ps.execAndExpectRow(ctx, "update dh price sync",
		`UPDATE campaign_purchases
		 SET dh_listing_price_cents = $1,
		     dh_last_synced_at      = $2,
		     updated_at             = $3
		 WHERE id = $4`,
		listingPriceCents, syncedAt.UTC().Format(time.RFC3339), time.Now(), id,
	)
}

// ListDHPriceDrift returns unsold purchases whose reviewed price has diverged
// from dh_listing_price_cents. Excludes items not on DH yet, items with no
// reviewed price, items whose push was dismissed or held, and sold items.
// Ordered oldest-synced first so the most-stale items sync first across
// scheduler ticks; never-synced items (default empty-string timestamp) sort
// before any RFC3339 string in ASC order.
func (ps *PurchaseStore) ListDHPriceDrift(ctx context.Context) ([]inventory.Purchase, error) {
	query := fmt.Sprintf(
		`SELECT %s FROM campaign_purchases p
		 LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		 WHERE p.dh_inventory_id > 0
		   AND p.reviewed_price_cents > 0
		   AND p.reviewed_price_cents != COALESCE(p.dh_listing_price_cents, 0)
		   AND COALESCE(p.dh_push_status, '') NOT IN ($1, $2)
		   AND s.id IS NULL
		 ORDER BY p.dh_last_synced_at ASC, p.id`,
		purchaseColumnsAliased,
	)
	rows, err := ps.db.QueryContext(ctx, query, inventory.DHPushStatusDismissed, inventory.DHPushStatusHeld)
	if err != nil {
		return nil, fmt.Errorf("list dh price drift: %w", err)
	}
	return scanRows(ctx, rows, func(rs *sql.Rows) (inventory.Purchase, error) {
		var p inventory.Purchase
		err := scanPurchase(rs, &p)
		return p, err
	})
}
