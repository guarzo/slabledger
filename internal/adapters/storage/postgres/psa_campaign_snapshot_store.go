package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

// PSACampaignSnapshotStore persists the most recent full snapshot of PSA
// portal campaigns as a single JSONB blob (migration 000017). The id column
// is locked to 1, so the table holds at most one row.
type PSACampaignSnapshotStore struct {
	db *sql.DB
}

var _ psacampaign.SnapshotStore = (*PSACampaignSnapshotStore)(nil)

func NewPSACampaignSnapshotStore(db *sql.DB) *PSACampaignSnapshotStore {
	return &PSACampaignSnapshotStore{db: db}
}

// SaveSnapshot upserts the singleton snapshot row.
func (s *PSACampaignSnapshotStore) SaveSnapshot(ctx context.Context, campaigns []psacampaign.PortalCampaign) error {
	if len(campaigns) == 0 {
		return fmt.Errorf("psa_campaign_snapshot: refusing to save empty snapshot")
	}
	raw, err := json.Marshal(campaigns)
	if err != nil {
		return fmt.Errorf("psa_campaign_snapshot: marshal: %w", err)
	}
	const q = `
		INSERT INTO psa_campaign_snapshot (id, raw_json, fetched_at, updated_at)
		VALUES (1, $1::jsonb, now(), now())
		ON CONFLICT (id) DO UPDATE
		   SET raw_json   = EXCLUDED.raw_json,
		       fetched_at = now(),
		       updated_at = now()`
	if _, err := s.db.ExecContext(ctx, q, string(raw)); err != nil {
		return fmt.Errorf("psa_campaign_snapshot: upsert: %w", err)
	}
	return nil
}

// GetSnapshot returns the stored campaigns and when they were fetched.
// No row yet → (empty slice, zero time, nil).
func (s *PSACampaignSnapshotStore) GetSnapshot(ctx context.Context) ([]psacampaign.PortalCampaign, time.Time, error) {
	const q = `SELECT raw_json, fetched_at FROM psa_campaign_snapshot WHERE id = 1`
	var raw []byte
	var fetchedAt time.Time
	err := s.db.QueryRowContext(ctx, q).Scan(&raw, &fetchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return []psacampaign.PortalCampaign{}, time.Time{}, nil
	}
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("psa_campaign_snapshot: query: %w", err)
	}
	var campaigns []psacampaign.PortalCampaign
	if err := json.Unmarshal(raw, &campaigns); err != nil {
		return nil, time.Time{}, fmt.Errorf("psa_campaign_snapshot: unmarshal: %w", err)
	}
	return campaigns, fetchedAt, nil
}
