package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const tombstoneThreshold = 3

type DHCardTombstoneStore struct {
	db *sql.DB
}

func NewDHCardTombstoneStore(db *sql.DB) *DHCardTombstoneStore {
	return &DHCardTombstoneStore{db: db}
}

func (s *DHCardTombstoneStore) IsTombstoned(ctx context.Context, cardID int) (bool, error) {
	var attempts int
	err := s.db.QueryRowContext(ctx,
		`SELECT attempts FROM dh_card_tombstones WHERE dh_card_id = $1`,
		cardID,
	).Scan(&attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("is tombstoned: %w", err)
	}
	return attempts >= tombstoneThreshold, nil
}

func (s *DHCardTombstoneStore) RecordFailure(ctx context.Context, cardID int, errMsg string) (int, error) {
	var attempts int
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO dh_card_tombstones (dh_card_id, attempts, last_error)
		 VALUES ($1, 1, $2)
		 ON CONFLICT (dh_card_id) DO UPDATE
		   SET attempts = dh_card_tombstones.attempts + 1,
		       last_seen_at = NOW(),
		       last_error = EXCLUDED.last_error
		 RETURNING attempts`,
		cardID, errMsg,
	).Scan(&attempts)
	if err != nil {
		return 0, fmt.Errorf("record failure: %w", err)
	}
	return attempts, nil
}

func (s *DHCardTombstoneStore) Clear(ctx context.Context, cardID int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM dh_card_tombstones WHERE dh_card_id = $1`, cardID)
	if err != nil {
		return fmt.Errorf("clear tombstone: %w", err)
	}
	return nil
}

func (s *DHCardTombstoneStore) ClearAll(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM dh_card_tombstones`)
	if err != nil {
		return 0, fmt.Errorf("clear all tombstones: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("clear all tombstones rows: %w", err)
	}
	return int(n), nil
}

func (s *DHCardTombstoneStore) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM dh_card_tombstones WHERE attempts >= $1`,
		tombstoneThreshold,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count tombstones: %w", err)
	}
	return n, nil
}
