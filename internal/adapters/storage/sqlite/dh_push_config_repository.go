package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// GetDHPushConfig returns the DH push safety config, or defaults if none has been saved.
func (r *CampaignsRepository) GetDHPushConfig(ctx context.Context) (*campaigns.DHPushConfig, error) {
	query := `SELECT swing_pct_threshold, swing_min_cents, disagreement_pct_threshold,
		unreviewed_change_pct_threshold, unreviewed_change_min_cents, updated_at
		FROM dh_push_config WHERE id = 1`
	var cfg campaigns.DHPushConfig
	err := r.db.QueryRowContext(ctx, query).Scan(
		&cfg.SwingPctThreshold, &cfg.SwingMinCents, &cfg.DisagreementPctThreshold,
		&cfg.UnreviewedChangePctThreshold, &cfg.UnreviewedChangeMinCents, &cfg.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		def := campaigns.DefaultDHPushConfig()
		return &def, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveDHPushConfig upserts the DH push safety config.
func (r *CampaignsRepository) SaveDHPushConfig(ctx context.Context, cfg *campaigns.DHPushConfig) error {
	cfg.UpdatedAt = time.Now()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO dh_push_config (id, swing_pct_threshold, swing_min_cents, disagreement_pct_threshold,
			unreviewed_change_pct_threshold, unreviewed_change_min_cents, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			swing_pct_threshold = ?, swing_min_cents = ?, disagreement_pct_threshold = ?,
			unreviewed_change_pct_threshold = ?, unreviewed_change_min_cents = ?, updated_at = ?`,
		cfg.SwingPctThreshold, cfg.SwingMinCents, cfg.DisagreementPctThreshold,
		cfg.UnreviewedChangePctThreshold, cfg.UnreviewedChangeMinCents, cfg.UpdatedAt,
		cfg.SwingPctThreshold, cfg.SwingMinCents, cfg.DisagreementPctThreshold,
		cfg.UnreviewedChangePctThreshold, cfg.UnreviewedChangeMinCents, cfg.UpdatedAt,
	)
	return err
}
