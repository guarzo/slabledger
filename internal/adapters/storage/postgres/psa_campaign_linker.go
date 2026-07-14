package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

// PSACampaignLinker writes a freshly created portal campaign id onto the
// internal campaign row (campaigns.psa_campaign_request_id).
type PSACampaignLinker struct {
	db *sql.DB
}

var _ psacampaign.CampaignLinker = (*PSACampaignLinker)(nil)

func NewPSACampaignLinker(db *sql.DB) *PSACampaignLinker {
	return &PSACampaignLinker{db: db}
}

func (l *PSACampaignLinker) LinkPSACampaign(ctx context.Context, internalCampaignID, psaCampaignRequestID string) error {
	const q = `UPDATE campaigns SET psa_campaign_request_id = $2 WHERE id = $1`
	res, err := l.db.ExecContext(ctx, q, internalCampaignID, psaCampaignRequestID)
	if err != nil {
		return fmt.Errorf("psa_campaign_link: update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("psa_campaign_link: rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("psa_campaign_link: no campaign with id %q", internalCampaignID)
	}
	return nil
}
