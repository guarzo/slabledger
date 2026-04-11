package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// CardIDMappingRepository provides access to the card_id_mappings table.
// This caches the mapping from (card_name, set_name, provider) → external_id
// so external API search calls are not repeated.
type CardIDMappingRepository struct {
	db *sql.DB
}

// NewCardIDMappingRepository creates a new repository backed by the given database.
func NewCardIDMappingRepository(db *sql.DB) *CardIDMappingRepository {
	return &CardIDMappingRepository{db: db}
}

// GetExternalID returns the cached external ID for the given card+provider, or "" if not found.
func (r *CardIDMappingRepository) GetExternalID(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error) {
	var externalID string
	err := r.db.QueryRowContext(ctx,
		`SELECT external_id FROM card_id_mappings WHERE card_name = ? AND set_name = ? AND collector_number = ? AND provider = ?`,
		cardName, setName, collectorNumber, provider,
	).Scan(&externalID)

	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return externalID, err
}

// GetLocalCard returns the (card_name, set_name) for a given provider and external_id.
// Returns ("", "") if no mapping exists.
func (r *CardIDMappingRepository) GetLocalCard(ctx context.Context, provider, externalID string) (string, string, error) {
	var cardName, setName string
	err := r.db.QueryRowContext(ctx,
		`SELECT card_name, set_name FROM card_id_mappings WHERE provider = ? AND external_id = ?`,
		provider, externalID,
	).Scan(&cardName, &setName)

	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	return cardName, setName, err
}

// CardIDMapping represents a single cached external ID mapping.
type CardIDMapping struct {
	CardName        string
	SetName         string
	CollectorNumber string
	ExternalID      string
}

// ListByProvider returns all mapped cards for the given provider.
func (r *CardIDMappingRepository) ListByProvider(ctx context.Context, provider string) (_ []CardIDMapping, err error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT card_name, set_name, collector_number, external_id FROM card_id_mappings WHERE provider = ?`,
		provider,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	mappings := make([]CardIDMapping, 0, 64)
	for rows.Next() {
		var m CardIDMapping
		if err := rows.Scan(&m.CardName, &m.SetName, &m.CollectorNumber, &m.ExternalID); err != nil {
			return nil, fmt.Errorf("scan card id mapping row: %w", err)
		}
		mappings = append(mappings, m)
	}
	return mappings, rows.Err()
}

// GetMappedSet returns the set of card keys that have an external ID mapping for
// the given provider. The returned map is keyed by "cardName|setName|collectorNumber".
func (r *CardIDMappingRepository) GetMappedSet(ctx context.Context, provider string) (_ map[string]string, err error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT card_name, set_name, collector_number, external_id FROM card_id_mappings WHERE provider = ?`,
		provider,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	result := make(map[string]string, 64)
	for rows.Next() {
		var cardName, setName, collectorNumber, externalID string
		if err := rows.Scan(&cardName, &setName, &collectorNumber, &externalID); err != nil {
			return nil, fmt.Errorf("scan card id mapping to map row: %w", err)
		}
		result[cardName+"|"+setName+"|"+collectorNumber] = externalID
	}
	return result, rows.Err()
}

// GetExternalIDFresh returns the cached external ID only if it was updated within maxAge.
// Returns "" if no mapping exists or the mapping is stale.
func (r *CardIDMappingRepository) GetExternalIDFresh(ctx context.Context, cardName, setName, collectorNumber, provider string, maxAge time.Duration) (string, error) {
	cutoff := time.Now().Add(-maxAge)
	var externalID string
	err := r.db.QueryRowContext(ctx,
		`SELECT external_id FROM card_id_mappings
		 WHERE card_name = ? AND set_name = ? AND collector_number = ? AND provider = ? AND updated_at > ?`,
		cardName, setName, collectorNumber, provider, cutoff,
	).Scan(&externalID)

	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return externalID, err
}

// DeleteByCard removes external ID mappings for a given card name and set,
// across all providers. When collectorNumber is non-empty, only the specific
// variant is deleted; otherwise all variants for the card are removed.
func (r *CardIDMappingRepository) DeleteByCard(ctx context.Context, cardName, setName, collectorNumber string) (int64, error) {
	var result sql.Result
	var err error
	if collectorNumber != "" {
		result, err = r.db.ExecContext(ctx,
			`DELETE FROM card_id_mappings WHERE card_name = ? AND set_name = ? AND collector_number = ?`,
			cardName, setName, collectorNumber,
		)
	} else {
		result, err = r.db.ExecContext(ctx,
			`DELETE FROM card_id_mappings WHERE card_name = ? AND set_name = ?`,
			cardName, setName,
		)
	}
	if err != nil {
		return 0, fmt.Errorf("delete card id mappings: %w", err)
	}
	return result.RowsAffected()
}

// SaveExternalID stores (or updates) the external ID mapping.
// Manual hints (hint_source='manual') are never overwritten by auto-discovery.
func (r *CardIDMappingRepository) SaveExternalID(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO card_id_mappings (card_name, set_name, collector_number, provider, external_id, hint_source, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'auto', ?, ?)
		 ON CONFLICT(card_name, set_name, collector_number, provider)
		 DO UPDATE SET external_id = excluded.external_id, updated_at = excluded.updated_at
		 WHERE hint_source = 'auto'`,
		cardName, setName, collectorNumber, provider, externalID, now, now,
	)
	if err != nil {
		return fmt.Errorf("save external id: %w", err)
	}
	return nil
}

// SaveHint stores a user-provided price hint, overwriting any existing mapping.
func (r *CardIDMappingRepository) SaveHint(ctx context.Context, cardName, setName, collectorNumber, provider, externalID string) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO card_id_mappings (card_name, set_name, collector_number, provider, external_id, hint_source, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'manual', ?, ?)
		 ON CONFLICT(card_name, set_name, collector_number, provider)
		 DO UPDATE SET external_id = excluded.external_id, hint_source = 'manual', updated_at = excluded.updated_at`,
		cardName, setName, collectorNumber, provider, externalID, now, now,
	)
	if err != nil {
		return fmt.Errorf("upsert card hint: %w", err)
	}
	return nil
}

// GetHint returns the external ID only when a manual hint exists for the given card+provider.
// Returns "" if no manual hint is set.
func (r *CardIDMappingRepository) GetHint(ctx context.Context, cardName, setName, collectorNumber, provider string) (string, error) {
	var externalID string
	err := r.db.QueryRowContext(ctx,
		`SELECT external_id FROM card_id_mappings
		 WHERE card_name = ? AND set_name = ? AND collector_number = ? AND provider = ? AND hint_source = 'manual'`,
		cardName, setName, collectorNumber, provider,
	).Scan(&externalID)

	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return externalID, err
}

// DeleteHint removes a manual hint, allowing auto-discovery to take over again.
func (r *CardIDMappingRepository) DeleteHint(ctx context.Context, cardName, setName, collectorNumber, provider string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM card_id_mappings WHERE card_name = ? AND set_name = ? AND collector_number = ? AND provider = ? AND hint_source = 'manual'`,
		cardName, setName, collectorNumber, provider,
	)
	if err != nil {
		return fmt.Errorf("delete hint: %w", err)
	}
	return nil
}

// ListHints returns all manual hint mappings.
func (r *CardIDMappingRepository) ListHints(ctx context.Context) (_ []pricing.HintMapping, err error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT card_name, set_name, collector_number, provider, external_id
		 FROM card_id_mappings WHERE hint_source = 'manual' ORDER BY card_name, set_name`,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	hints := make([]pricing.HintMapping, 0, 64)
	for rows.Next() {
		var h pricing.HintMapping
		if err := rows.Scan(&h.CardName, &h.SetName, &h.CollectorNumber, &h.Provider, &h.ExternalID); err != nil {
			return nil, fmt.Errorf("scan hint row: %w", err)
		}
		hints = append(hints, h)
	}
	return hints, rows.Err()
}
