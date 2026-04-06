package sqlite

import (
	"context"
	"database/sql"

	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// RefreshCandidateRepository queries campaign_purchases for cards needing price refresh.
type RefreshCandidateRepository struct {
	db *sql.DB
}

var _ pricing.RefreshCandidateProvider = (*RefreshCandidateRepository)(nil)

func NewRefreshCandidateRepository(db *sql.DB) *RefreshCandidateRepository {
	return &RefreshCandidateRepository{db: db}
}

// GetRefreshCandidates returns distinct unsold cards from active campaigns,
// ordered by most recently accessed first.
func (r *RefreshCandidateRepository) GetRefreshCandidates(ctx context.Context, limit int) ([]pricing.RefreshCandidate, error) {
	query := `
		SELECT DISTINCT
			cp.card_name,
			COALESCE(cp.card_number, '') AS card_number,
			cp.set_name,
			COALESCE(cp.psa_listing_title, '') AS psa_listing_title
		FROM campaign_purchases cp
		JOIN campaigns c ON cp.campaign_id = c.id
		LEFT JOIN campaign_sales cs ON cp.id = cs.purchase_id
		LEFT JOIN card_access_log cal
			ON cal.card_name = cp.card_name
			AND cal.set_name = cp.set_name
		WHERE cs.id IS NULL
			AND c.phase != 'closed'
		GROUP BY cp.card_name, cp.card_number, cp.set_name
		ORDER BY
			MAX(cal.accessed_at) DESC NULLS LAST,
			cp.created_at DESC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // best-effort close

	candidates := make([]pricing.RefreshCandidate, 0, limit)
	for rows.Next() {
		var c pricing.RefreshCandidate
		if err := rows.Scan(&c.CardName, &c.CardNumber, &c.SetName, &c.PSAListingTitle); err != nil {
			return nil, err
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}
