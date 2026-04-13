package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"time"
)

// DHStore implements inventory.DHRepository operations.
type DHStore struct {
	base
}

// NewDHStore creates a new DH store.
func NewDHStore(db *sql.DB, logger observability.Logger) *DHStore {
	return &DHStore{base{db: db, logger: logger}}
}

var _ inventory.DHRepository = (*DHStore)(nil)

// GetDHPushConfig returns the DH push safety config, or defaults if none has been saved.
func (dhs *DHStore) GetDHPushConfig(ctx context.Context) (*inventory.DHPushConfig, error) {
	query := `SELECT swing_pct_threshold, swing_min_cents, disagreement_pct_threshold,
		unreviewed_change_pct_threshold, unreviewed_change_min_cents, updated_at
		FROM dh_push_config WHERE id = 1`
	var cfg inventory.DHPushConfig
	err := dhs.db.QueryRowContext(ctx, query).Scan(
		&cfg.SwingPctThreshold, &cfg.SwingMinCents, &cfg.DisagreementPctThreshold,
		&cfg.UnreviewedChangePctThreshold, &cfg.UnreviewedChangeMinCents, &cfg.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		def := inventory.DefaultDHPushConfig()
		return &def, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveDHPushConfig upserts the DH push safety config.
func (dhs *DHStore) SaveDHPushConfig(ctx context.Context, cfg *inventory.DHPushConfig) error {
	if cfg == nil {
		return fmt.Errorf("dh push config cannot be nil")
	}
	cfg.UpdatedAt = time.Now()
	_, err := dhs.db.ExecContext(ctx,
		`INSERT INTO dh_push_config (id, swing_pct_threshold, swing_min_cents, disagreement_pct_threshold,
			unreviewed_change_pct_threshold, unreviewed_change_min_cents, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			swing_pct_threshold = excluded.swing_pct_threshold,
			swing_min_cents = excluded.swing_min_cents,
			disagreement_pct_threshold = excluded.disagreement_pct_threshold,
			unreviewed_change_pct_threshold = excluded.unreviewed_change_pct_threshold,
			unreviewed_change_min_cents = excluded.unreviewed_change_min_cents,
			updated_at = excluded.updated_at`,
		cfg.SwingPctThreshold, cfg.SwingMinCents, cfg.DisagreementPctThreshold,
		cfg.UnreviewedChangePctThreshold, cfg.UnreviewedChangeMinCents, cfg.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("save dh push config: %w", err)
	}
	return nil
}
