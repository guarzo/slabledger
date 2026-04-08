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
	SlabSerial      string
	MMCollectibleID int64
	MasterID        int64 // Grade-agnostic variant ID (shared across all grades of the same card)
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
	if err == sql.ErrNoRows {
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

// SaveMapping upserts a cert → MM collectible ID + master ID mapping.
// masterID is the grade-agnostic variant identifier (0 if unknown).
func (s *MarketMoversStore) SaveMapping(ctx context.Context, slabSerial string, mmCollectibleID, masterID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO mm_card_mappings (slab_serial, mm_collectible_id, mm_master_id, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(slab_serial) DO UPDATE SET
		   mm_collectible_id = excluded.mm_collectible_id,
		   mm_master_id = excluded.mm_master_id,
		   updated_at = excluded.updated_at`,
		slabSerial, mmCollectibleID, masterID, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// GetMapping returns the MM collectible ID for a cert, or nil if not found.
func (s *MarketMoversStore) GetMapping(ctx context.Context, slabSerial string) (*MMCardMapping, error) {
	var m MMCardMapping
	err := s.db.QueryRowContext(ctx,
		`SELECT slab_serial, mm_collectible_id, mm_master_id FROM mm_card_mappings WHERE slab_serial = ?`,
		slabSerial,
	).Scan(&m.SlabSerial, &m.MMCollectibleID, &m.MasterID)
	if err == sql.ErrNoRows {
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
		`SELECT slab_serial, mm_collectible_id, mm_master_id FROM mm_card_mappings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var mappings []MMCardMapping
	for rows.Next() {
		var m MMCardMapping
		if err := rows.Scan(&m.SlabSerial, &m.MMCollectibleID, &m.MasterID); err != nil {
			return nil, err
		}
		mappings = append(mappings, m)
	}
	return mappings, rows.Err()
}
