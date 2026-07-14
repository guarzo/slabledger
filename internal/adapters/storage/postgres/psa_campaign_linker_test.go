package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestPSACampaignLinker(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	logger := mocks.NewMockLogger()
	repo := NewCampaignStore(db.DB, logger)

	now := time.Now().UTC().Truncate(time.Second)
	const campaignID = "link-camp-1"
	require.NoError(t, repo.CreateCampaign(ctx, &inventory.Campaign{
		ID:                  campaignID,
		Name:                "Modern 10s",
		Sport:               "Pokemon",
		YearRange:           "2024-2026",
		GradeRange:          "10",
		PriceRange:          "500-3000",
		CLConfidence:        "3-4",
		BuyTermsCLPct:       0.72,
		DailySpendCapCents:  300000,
		Phase:               inventory.PhasePending,
		PSASourcingFeeCents: 300,
		CreatedAt:           now,
		UpdatedAt:           now,
	}))

	l := NewPSACampaignLinker(db.DB)

	// No link yet.
	before, err := l.LinkedPSACampaignID(ctx, campaignID)
	require.NoError(t, err)
	require.Empty(t, before)

	require.NoError(t, l.LinkPSACampaign(ctx, campaignID, "uuid-new-1"))

	var got string
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT psa_campaign_request_id FROM campaigns WHERE id = $1`, campaignID).Scan(&got))
	require.Equal(t, "uuid-new-1", got)

	// After link, the lookup returns the portal id (idempotency guard input).
	after, err := l.LinkedPSACampaignID(ctx, campaignID)
	require.NoError(t, err)
	require.Equal(t, "uuid-new-1", after)

	// Unknown campaign returns "" without error.
	missing, err := l.LinkedPSACampaignID(ctx, "nonexistent-id")
	require.NoError(t, err)
	require.Empty(t, missing)

	err = l.LinkPSACampaign(ctx, "nonexistent-id", "x")
	require.ErrorContains(t, err, "no campaign with id")
}
