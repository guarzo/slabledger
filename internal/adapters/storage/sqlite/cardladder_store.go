package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/platform/crypto"
)

// ErrConfigNotFound is returned when the singleton cardladder_config row doesn't exist.
var ErrConfigNotFound = errors.New("cardladder_config row not found")

// CardLadderConfig holds the stored CL connection configuration.
type CardLadderConfig struct {
	Email          string
	RefreshToken   string // decrypted
	CollectionID   string
	FirebaseAPIKey string
	FirebaseUID    string
}

// CLCardMapping maps a purchase cert to a CL collection card.
type CLCardMapping struct {
	SlabSerial         string
	CLCollectionCardID string
	CLGemRateID        string
	CLCondition        string
}

// CardLadderStore manages Card Ladder config and mapping persistence.
type CardLadderStore struct {
	db        *sql.DB
	encryptor crypto.Encryptor
}

// NewCardLadderStore creates a new Card Ladder store.
func NewCardLadderStore(db *sql.DB, encryptor crypto.Encryptor) *CardLadderStore {
	return &CardLadderStore{db: db, encryptor: encryptor}
}

// GetConfig returns the current CL config, or nil if not configured.
// The nil-on-missing contract is intentional: callers treat "not configured" as a
// valid initial state, not an error.
func (s *CardLadderStore) GetConfig(ctx context.Context) (*CardLadderConfig, error) {
	var (
		email, encToken, collectionID, apiKey, uid string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT email, encrypted_refresh_token, collection_id, firebase_api_key, firebase_uid
		 FROM cardladder_config WHERE id = 1`,
	).Scan(&email, &encToken, &collectionID, &apiKey, &uid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query cardladder config: %w", err)
	}

	token, err := s.encryptor.Decrypt(encToken)
	if err != nil {
		return nil, fmt.Errorf("decrypt cardladder refresh token: %w", err)
	}

	return &CardLadderConfig{
		Email:          email,
		RefreshToken:   token,
		CollectionID:   collectionID,
		FirebaseAPIKey: apiKey,
		FirebaseUID:    uid,
	}, nil
}

// SaveConfig stores CL connection info. Upserts the singleton row.
func (s *CardLadderStore) SaveConfig(ctx context.Context, email, refreshToken, collectionID, firebaseAPIKey, firebaseUID string) error {
	encToken, err := s.encryptor.Encrypt(refreshToken)
	if err != nil {
		return fmt.Errorf("encrypt cardladder refresh token: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO cardladder_config (id, email, encrypted_refresh_token, collection_id, firebase_api_key, firebase_uid, updated_at)
		 VALUES (1, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   email = excluded.email,
		   encrypted_refresh_token = excluded.encrypted_refresh_token,
		   collection_id = excluded.collection_id,
		   firebase_api_key = excluded.firebase_api_key,
		   firebase_uid = excluded.firebase_uid,
		   updated_at = excluded.updated_at`,
		email, encToken, collectionID, firebaseAPIKey, firebaseUID, now,
	); err != nil {
		return fmt.Errorf("upsert cardladder config: %w", err)
	}
	return nil
}

// UpdateRefreshToken updates just the refresh token (after token refresh).
// Returns an error if the singleton config row does not exist.
func (s *CardLadderStore) UpdateRefreshToken(ctx context.Context, refreshToken string) error {
	encToken, err := s.encryptor.Encrypt(refreshToken)
	if err != nil {
		return fmt.Errorf("encrypt cardladder refresh token: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE cardladder_config SET encrypted_refresh_token = ?, updated_at = ? WHERE id = 1`,
		encToken, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("update cardladder refresh token: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if n == 0 {
		return ErrConfigNotFound
	}
	return nil
}

// DeleteConfig removes the CL configuration.
func (s *CardLadderStore) DeleteConfig(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM cardladder_config WHERE id = 1`); err != nil {
		return fmt.Errorf("delete cardladder config: %w", err)
	}
	return nil
}

// SaveMapping upserts a cert→CL card mapping.
func (s *CardLadderStore) SaveMapping(ctx context.Context, slabSerial, clCardID, gemRateID, condition string) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO cl_card_mappings (slab_serial, cl_collection_card_id, cl_gem_rate_id, cl_condition, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(slab_serial) DO UPDATE SET
		   cl_collection_card_id = excluded.cl_collection_card_id,
		   cl_gem_rate_id = excluded.cl_gem_rate_id,
		   cl_condition = excluded.cl_condition,
		   updated_at = excluded.updated_at`,
		slabSerial, clCardID, gemRateID, condition, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("upsert cl card mapping %s: %w", slabSerial, err)
	}
	return nil
}

// SaveMappingPricing upserts a cert→gemRateID+condition mapping without
// touching cl_collection_card_id. Used by the cert-first refresh flow that
// resolves pricing via BuildCollectionCard (cert → gemRateID) without a push
// to the CL remote collection, so an existing pushed doc ID is preserved.
func (s *CardLadderStore) SaveMappingPricing(ctx context.Context, slabSerial, gemRateID, condition string) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO cl_card_mappings (slab_serial, cl_collection_card_id, cl_gem_rate_id, cl_condition, updated_at)
		 VALUES (?, '', ?, ?, ?)
		 ON CONFLICT(slab_serial) DO UPDATE SET
		   cl_gem_rate_id = excluded.cl_gem_rate_id,
		   cl_condition = excluded.cl_condition,
		   updated_at = excluded.updated_at`,
		slabSerial, gemRateID, condition, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("upsert cl pricing mapping %s: %w", slabSerial, err)
	}
	return nil
}

// GetMapping returns a mapping for a cert, or nil if not found.
// The nil-on-missing contract is intentional: callers treat "no mapping yet" as
// a valid state, not an error.
func (s *CardLadderStore) GetMapping(ctx context.Context, slabSerial string) (*CLCardMapping, error) {
	var m CLCardMapping
	err := s.db.QueryRowContext(ctx,
		`SELECT slab_serial, cl_collection_card_id, cl_gem_rate_id, cl_condition
		 FROM cl_card_mappings WHERE slab_serial = ?`, slabSerial,
	).Scan(&m.SlabSerial, &m.CLCollectionCardID, &m.CLGemRateID, &m.CLCondition)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query cl card mapping %s: %w", slabSerial, err)
	}
	return &m, nil
}

// DeleteMapping removes a cert→CL card mapping by slab serial.
func (s *CardLadderStore) DeleteMapping(ctx context.Context, slabSerial string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM cl_card_mappings WHERE slab_serial = ?`, slabSerial); err != nil {
		return fmt.Errorf("delete cl card mapping %s: %w", slabSerial, err)
	}
	return nil
}

// ListMappings returns all stored mappings.
func (s *CardLadderStore) ListMappings(ctx context.Context) ([]CLCardMapping, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT slab_serial, cl_collection_card_id, cl_gem_rate_id, cl_condition FROM cl_card_mappings`)
	if err != nil {
		return nil, fmt.Errorf("query cl card mappings: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var mappings []CLCardMapping
	for rows.Next() {
		var m CLCardMapping
		if err := rows.Scan(&m.SlabSerial, &m.CLCollectionCardID, &m.CLGemRateID, &m.CLCondition); err != nil {
			return nil, fmt.Errorf("scan cl card mapping row: %w", err)
		}
		mappings = append(mappings, m)
	}
	return mappings, rows.Err()
}

// CLPriceStats summarizes CL value freshness across unsold inventory.
// Mirrors MMPriceStats so the frontend can render a symmetric panel.
type CLPriceStats struct {
	UnsoldTotal  int    `json:"unsoldTotal"`  // Total unsold purchases
	WithCLValue  int    `json:"withCLValue"`  // Unsold purchases with a CL value
	SyncedCount  int    `json:"syncedCount"`  // Unsold purchases pushed to the CL remote collection
	OldestUpdate string `json:"oldestUpdate"` // Oldest cl_value_updated_at among priced cards
	NewestUpdate string `json:"newestUpdate"` // Newest cl_value_updated_at
	StaleCount   int    `json:"staleCount"`   // Priced cards whose value is older than 7 days
}

// GetCLFailures returns unsold purchases whose last CL refresh recorded a
// failure reason, grouped by reason with a bounded sample list for the UI.
// sampleLimit is clamped inside queryIntegrationFailures.
func (s *CardLadderStore) GetCLFailures(ctx context.Context, sampleLimit int) (*IntegrationFailuresReport, error) {
	return queryIntegrationFailures(ctx, s.db, "cl_last_error", "cl_last_error_at", sampleLimit)
}

// GetCLPriceStats computes summary statistics about CL value freshness across unsold inventory.
// SyncedCount counts unsold purchases whose cert currently has a row in
// cl_card_mappings (i.e. the card is still in the CL remote collection) — more
// accurate than cl_synced_at, which is a historical push timestamp that never
// gets cleared when removeSoldCards prunes the mapping.
func (s *CardLadderStore) GetCLPriceStats(ctx context.Context) (*CLPriceStats, error) {
	var stats CLPriceStats

	staleCutoff := time.Now().UTC().AddDate(0, 0, -7).Format(time.RFC3339)
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) AS unsold_total,
			COALESCE(SUM(CASE WHEN p.cl_value_cents > 0 THEN 1 ELSE 0 END), 0) AS with_cl_value,
			COALESCE(SUM(CASE WHEN m.slab_serial IS NOT NULL THEN 1 ELSE 0 END), 0) AS synced_count,
			COALESCE(MIN(CASE WHEN p.cl_value_cents > 0 AND p.cl_value_updated_at != '' THEN p.cl_value_updated_at END), '') AS oldest_update,
			COALESCE(MAX(CASE WHEN p.cl_value_cents > 0 AND p.cl_value_updated_at != '' THEN p.cl_value_updated_at END), '') AS newest_update,
			COALESCE(SUM(CASE WHEN p.cl_value_cents > 0 AND (p.cl_value_updated_at = '' OR p.cl_value_updated_at < ?) THEN 1 ELSE 0 END), 0) AS stale_count
		FROM campaign_purchases p
		INNER JOIN campaigns c ON c.id = p.campaign_id
		LEFT JOIN campaign_sales s ON s.purchase_id = p.id
		LEFT JOIN cl_card_mappings m ON m.slab_serial = p.cert_number
		WHERE s.id IS NULL AND c.phase != 'closed'
	`, staleCutoff,
	).Scan(&stats.UnsoldTotal, &stats.WithCLValue, &stats.SyncedCount, &stats.OldestUpdate, &stats.NewestUpdate, &stats.StaleCount)
	if err != nil {
		return nil, fmt.Errorf("cl price stats: %w", err)
	}

	return &stats, nil
}
