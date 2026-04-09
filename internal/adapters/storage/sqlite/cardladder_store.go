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
		return nil, err
	}

	token, err := s.encryptor.Decrypt(encToken)
	if err != nil {
		return nil, err
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
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
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
	)
	return err
}

// UpdateRefreshToken updates just the refresh token (after token refresh).
// Returns an error if the singleton config row does not exist.
func (s *CardLadderStore) UpdateRefreshToken(ctx context.Context, refreshToken string) error {
	encToken, err := s.encryptor.Encrypt(refreshToken)
	if err != nil {
		return err
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE cardladder_config SET encrypted_refresh_token = ?, updated_at = ? WHERE id = 1`,
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
		return ErrConfigNotFound
	}
	return nil
}

// DeleteConfig removes the CL configuration.
func (s *CardLadderStore) DeleteConfig(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM cardladder_config WHERE id = 1`)
	return err
}

// SaveMapping upserts a cert→CL card mapping.
func (s *CardLadderStore) SaveMapping(ctx context.Context, slabSerial, clCardID, gemRateID, condition string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cl_card_mappings (slab_serial, cl_collection_card_id, cl_gem_rate_id, cl_condition, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(slab_serial) DO UPDATE SET
		   cl_collection_card_id = excluded.cl_collection_card_id,
		   cl_gem_rate_id = excluded.cl_gem_rate_id,
		   cl_condition = excluded.cl_condition,
		   updated_at = excluded.updated_at`,
		slabSerial, clCardID, gemRateID, condition, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// GetMapping returns a mapping for a cert, or nil if not found.
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
		return nil, err
	}
	return &m, nil
}

// DeleteMapping removes a cert→CL card mapping by slab serial.
func (s *CardLadderStore) DeleteMapping(ctx context.Context, slabSerial string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM cl_card_mappings WHERE slab_serial = ?`, slabSerial)
	return err
}

// ListMappings returns all stored mappings.
func (s *CardLadderStore) ListMappings(ctx context.Context) ([]CLCardMapping, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT slab_serial, cl_collection_card_id, cl_gem_rate_id, cl_condition FROM cl_card_mappings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var mappings []CLCardMapping
	for rows.Next() {
		var m CLCardMapping
		if err := rows.Scan(&m.SlabSerial, &m.CLCollectionCardID, &m.CLGemRateID, &m.CLCondition); err != nil {
			return nil, err
		}
		mappings = append(mappings, m)
	}
	return mappings, rows.Err()
}
