package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// SyncStateRepository provides access to the sync_state table.
// Stores key-value pairs for tracking synchronization state (e.g., last poll timestamps).
type SyncStateRepository struct {
	db *sql.DB
}

// NewSyncStateRepository creates a new repository backed by the given database.
func NewSyncStateRepository(db *sql.DB) *SyncStateRepository {
	return &SyncStateRepository{db: db}
}

// Get returns the value for the given key, or "" if not found.
func (r *SyncStateRepository) Get(ctx context.Context, key string) (string, error) {
	var value string
	err := r.db.QueryRowContext(ctx,
		`SELECT value FROM sync_state WHERE key = ?`, key,
	).Scan(&value)

	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

// Set stores (or updates) the value for the given key.
func (r *SyncStateRepository) Set(ctx context.Context, key, value string) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO sync_state (key, value, updated_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(key)
		 DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, now,
	)
	return err
}
