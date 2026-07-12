package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// PSAPortalSnapshotStore persists the single most-recent PSA portal rows
// snapshot: the raw flattened Lightdash rows the harvester fetched. The id
// column is locked to 1 by a CHECK constraint (migration 000017), mirroring
// psa_portal_token, so the table holds at most one row. Rows are stored
// unmapped so mapper fixes ship with an app deploy, not a harvester rebuild.
type PSAPortalSnapshotStore struct {
	db *sql.DB
}

func NewPSAPortalSnapshotStore(db *sql.DB) *PSAPortalSnapshotStore {
	return &PSAPortalSnapshotStore{db: db}
}

// SaveSnapshot upserts the single snapshot row.
func (s *PSAPortalSnapshotStore) SaveSnapshot(ctx context.Context, rows []map[string]string, fetchedAt time.Time) error {
	b, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("psa_portal_snapshot: marshal rows: %w", err)
	}
	const q = `
		INSERT INTO psa_portal_snapshot (id, rows, fetched_at, updated_at)
		VALUES (1, $1, $2, now())
		ON CONFLICT (id) DO UPDATE
		   SET rows       = EXCLUDED.rows,
		       fetched_at = EXCLUDED.fetched_at,
		       updated_at = now()`
	if _, err := s.db.ExecContext(ctx, q, string(b), fetchedAt); err != nil {
		return fmt.Errorf("psa_portal_snapshot: upsert: %w", err)
	}
	return nil
}

// CurrentSnapshot returns the stored rows and when they were fetched.
// No row yet → (nil, zero time, nil).
func (s *PSAPortalSnapshotStore) CurrentSnapshot(ctx context.Context) ([]map[string]string, time.Time, error) {
	const q = `SELECT rows, fetched_at FROM psa_portal_snapshot WHERE id = 1`
	var b []byte
	var fetchedAt time.Time
	err := s.db.QueryRowContext(ctx, q).Scan(&b, &fetchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, nil
	}
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("psa_portal_snapshot: query: %w", err)
	}
	var rows []map[string]string
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, time.Time{}, fmt.Errorf("psa_portal_snapshot: unmarshal rows: %w", err)
	}
	return rows, fetchedAt, nil
}

// SnapshotFetchedAt returns when the stored snapshot was fetched without
// loading the rows payload. No row yet → (zero time, nil).
func (s *PSAPortalSnapshotStore) SnapshotFetchedAt(ctx context.Context) (time.Time, error) {
	const q = `SELECT fetched_at FROM psa_portal_snapshot WHERE id = 1`
	var fetchedAt time.Time
	err := s.db.QueryRowContext(ctx, q).Scan(&fetchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("psa_portal_snapshot: query fetched_at: %w", err)
	}
	return fetchedAt, nil
}
