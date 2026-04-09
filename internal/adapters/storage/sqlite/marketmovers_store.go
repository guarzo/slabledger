package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/platform/crypto"
)

// ErrMMConfigNotFound is returned when the singleton marketmovers_config row doesn't exist.
var ErrMMConfigNotFound = errors.New("marketmovers_config row not found")

// MarketMoversConfig holds the stored Market Movers connection configuration.
type MarketMoversConfig struct {
	Username     string
	RefreshToken string // decrypted
}

// MMCardMapping maps a purchase cert to a Market Movers collectible ID.
type MMCardMapping struct {
	SlabSerial       string
	MMCollectibleID  int64
	MasterID         int64  // Grade-agnostic variant ID (shared across all grades of the same card)
	SearchTitle      string // MM canonical search title (e.g. "Charizard 1999 Base Set Holo #4/102 PSA 10")
	CollectionItemID int64  // MM collection item ID returned after sync (0 = not synced)
}

// MarketMoversStore manages Market Movers config and mapping persistence.
type MarketMoversStore struct {
	db        *sql.DB
	encryptor crypto.Encryptor
}

// NewMarketMoversStore creates a new Market Movers store.
func NewMarketMoversStore(db *sql.DB, encryptor crypto.Encryptor) *MarketMoversStore {
	return &MarketMoversStore{db: db, encryptor: encryptor}
}

// GetConfig returns the current MM config, or nil if not configured.
func (s *MarketMoversStore) GetConfig(ctx context.Context) (*MarketMoversConfig, error) {
	var username, encToken string
	err := s.db.QueryRowContext(ctx,
		`SELECT username, encrypted_refresh_token FROM marketmovers_config WHERE id = 1`,
	).Scan(&username, &encToken)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	token, err := s.encryptor.Decrypt(encToken)
	if err != nil {
		return nil, err
	}

	return &MarketMoversConfig{
		Username:     username,
		RefreshToken: token,
	}, nil
}

// SaveConfig stores Market Movers credentials. Upserts the singleton row.
func (s *MarketMoversStore) SaveConfig(ctx context.Context, username, refreshToken string) error {
	encToken, err := s.encryptor.Encrypt(refreshToken)
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO marketmovers_config (id, username, encrypted_refresh_token, updated_at)
		 VALUES (1, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   username = excluded.username,
		   encrypted_refresh_token = excluded.encrypted_refresh_token,
		   updated_at = excluded.updated_at`,
		username, encToken, now,
	)
	return err
}

// UpdateRefreshToken updates just the refresh token (after token refresh).
// Returns ErrMMConfigNotFound if the singleton config row does not exist.
func (s *MarketMoversStore) UpdateRefreshToken(ctx context.Context, refreshToken string) error {
	encToken, err := s.encryptor.Encrypt(refreshToken)
	if err != nil {
		return err
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE marketmovers_config SET encrypted_refresh_token = ?, updated_at = ? WHERE id = 1`,
		encToken, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return ErrMMConfigNotFound
	}
	return nil
}

// DeleteConfig removes the Market Movers configuration.
func (s *MarketMoversStore) DeleteConfig(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM marketmovers_config WHERE id = 1`)
	return err
}

// SaveMapping upserts a cert → MM collectible ID + master ID + search title mapping.
// masterID is the grade-agnostic variant identifier (0 if unknown).
func (s *MarketMoversStore) SaveMapping(ctx context.Context, slabSerial string, mmCollectibleID, masterID int64, searchTitle string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO mm_card_mappings (slab_serial, mm_collectible_id, mm_master_id, mm_search_title, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(slab_serial) DO UPDATE SET
		   mm_collectible_id = excluded.mm_collectible_id,
		   mm_master_id = excluded.mm_master_id,
		   mm_search_title = excluded.mm_search_title,
		   mm_collection_item_id = CASE
		     WHEN mm_card_mappings.mm_collectible_id != excluded.mm_collectible_id
		       OR mm_card_mappings.mm_master_id != excluded.mm_master_id
		       OR mm_card_mappings.mm_search_title != excluded.mm_search_title
		     THEN 0
		     ELSE mm_card_mappings.mm_collection_item_id
		   END,
		   updated_at = excluded.updated_at`,
		slabSerial, mmCollectibleID, masterID, searchTitle, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// GetMapping returns the MM collectible ID for a cert, or nil if not found.
func (s *MarketMoversStore) GetMapping(ctx context.Context, slabSerial string) (*MMCardMapping, error) {
	var m MMCardMapping
	err := s.db.QueryRowContext(ctx,
		`SELECT slab_serial, mm_collectible_id, mm_master_id, mm_search_title, mm_collection_item_id FROM mm_card_mappings WHERE slab_serial = ?`,
		slabSerial,
	).Scan(&m.SlabSerial, &m.MMCollectibleID, &m.MasterID, &m.SearchTitle, &m.CollectionItemID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListMappings returns all stored MM card mappings.
func (s *MarketMoversStore) ListMappings(ctx context.Context) ([]MMCardMapping, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT slab_serial, mm_collectible_id, mm_master_id, mm_search_title, mm_collection_item_id FROM mm_card_mappings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var mappings []MMCardMapping
	for rows.Next() {
		select {
		case <-ctx.Done():
			rows.Close() //nolint:errcheck // best-effort close on ctx cancel
			return nil, ctx.Err()
		default:
		}
		var m MMCardMapping
		if err := rows.Scan(&m.SlabSerial, &m.MMCollectibleID, &m.MasterID, &m.SearchTitle, &m.CollectionItemID); err != nil {
			return nil, err
		}
		mappings = append(mappings, m)
	}
	return mappings, rows.Err()
}

// SaveCollectionItemID records the MM collection item ID for a synced cert.
func (s *MarketMoversStore) SaveCollectionItemID(ctx context.Context, slabSerial string, collectionItemID int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE mm_card_mappings SET mm_collection_item_id = ?, updated_at = ? WHERE slab_serial = ?`,
		collectionItemID, time.Now().UTC().Format(time.RFC3339), slabSerial,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("no mapping found for cert %s", slabSerial)
	}
	return nil
}

// ListUnsyncedMappings returns mappings that have a collectible ID but no collection item ID.
func (s *MarketMoversStore) ListUnsyncedMappings(ctx context.Context) ([]MMCardMapping, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT slab_serial, mm_collectible_id, mm_master_id, mm_search_title, mm_collection_item_id
		 FROM mm_card_mappings
		 WHERE mm_collectible_id > 0 AND mm_collection_item_id = 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var mappings []MMCardMapping
	for rows.Next() {
		select {
		case <-ctx.Done():
			rows.Close() //nolint:errcheck
			return nil, ctx.Err()
		default:
		}
		var m MMCardMapping
		if err := rows.Scan(&m.SlabSerial, &m.MMCollectibleID, &m.MasterID, &m.SearchTitle, &m.CollectionItemID); err != nil {
			return nil, err
		}
		mappings = append(mappings, m)
	}
	return mappings, rows.Err()
}

// MMPriceStats holds summary statistics about Market Movers price freshness.
type MMPriceStats struct {
	UnsoldTotal  int    `json:"unsoldTotal"`  // Total unsold purchases
	WithMMPrice  int    `json:"withMMPrice"`  // Unsold purchases that have an MM value
	SyncedCount  int    `json:"syncedCount"`  // Mappings with a collection_item_id
	OldestUpdate string `json:"oldestUpdate"` // Oldest mm_value_updated_at among cards with MM data
	NewestUpdate string `json:"newestUpdate"` // Newest mm_value_updated_at
	StaleCount   int    `json:"staleCount"`   // Unsold cards with MM data older than 7 days
}

// GetMMPriceStats computes summary statistics about MM price freshness across unsold inventory.
func (s *MarketMoversStore) GetMMPriceStats(ctx context.Context) (*MMPriceStats, error) {
	var stats MMPriceStats

	// Unsold total + cards with MM prices + oldest/newest update
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) AS unsold_total,
			COALESCE(SUM(CASE WHEN p.mm_value_cents > 0 THEN 1 ELSE 0 END), 0) AS with_mm_price,
			COALESCE(MIN(CASE WHEN p.mm_value_cents > 0 AND p.mm_value_updated_at != '' THEN p.mm_value_updated_at END), '') AS oldest_update,
			COALESCE(MAX(CASE WHEN p.mm_value_cents > 0 AND p.mm_value_updated_at != '' THEN p.mm_value_updated_at END), '') AS newest_update,
			COALESCE(SUM(CASE WHEN p.mm_value_cents > 0 AND (p.mm_value_updated_at = '' OR p.mm_value_updated_at < ?) THEN 1 ELSE 0 END), 0) AS stale_count
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		WHERE s.id IS NULL AND c.phase != 'closed'
	`, time.Now().UTC().AddDate(0, 0, -7).Format(time.RFC3339),
	).Scan(&stats.UnsoldTotal, &stats.WithMMPrice, &stats.OldestUpdate, &stats.NewestUpdate, &stats.StaleCount)
	if err != nil {
		return nil, fmt.Errorf("mm price stats: %w", err)
	}

	// Synced count from mappings
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mm_card_mappings WHERE mm_collection_item_id > 0`,
	).Scan(&stats.SyncedCount)
	if err != nil {
		return nil, fmt.Errorf("mm synced count: %w", err)
	}

	return &stats, nil
}
