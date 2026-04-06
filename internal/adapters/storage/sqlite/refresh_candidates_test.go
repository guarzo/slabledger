package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRefreshCandidateRepository_GetRefreshCandidates(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	// Create a campaign
	_, err := db.ExecContext(ctx, `INSERT INTO campaigns (id, name, phase) VALUES ('c1', 'Test', 'active')`)
	require.NoError(t, err)

	// Create an unsold purchase
	_, err = db.ExecContext(ctx, `INSERT INTO campaign_purchases (id, campaign_id, card_name, set_name, card_number, psa_listing_title, buy_cost_cents, cert_number, purchase_date, created_at)
		VALUES ('p1', 'c1', 'Charizard', 'Base Set', '4/102', 'PSA 10 Charizard', 50000, '12345678', '2024-01-01', CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	// Create a sold purchase (should be excluded)
	_, err = db.ExecContext(ctx, `INSERT INTO campaign_purchases (id, campaign_id, card_name, set_name, card_number, buy_cost_cents, cert_number, purchase_date, created_at)
		VALUES ('p2', 'c1', 'Pikachu', 'Base Set', '58/102', 1000, '87654321', '2024-01-02', CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO campaign_sales (id, purchase_id, sale_channel, sale_price_cents, sale_date, created_at)
		VALUES ('s1', 'p2', 'ebay', 2000, '2024-02-01', CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	repo := NewRefreshCandidateRepository(db.DB)
	candidates, err := repo.GetRefreshCandidates(ctx, 10)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, "Charizard", candidates[0].CardName)
	require.Equal(t, "Base Set", candidates[0].SetName)
	require.Equal(t, "4/102", candidates[0].CardNumber)
	require.Equal(t, "PSA 10 Charizard", candidates[0].PSAListingTitle)
}

func TestRefreshCandidateRepository_ExcludesArchivedCampaigns(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `INSERT INTO campaigns (id, name, phase) VALUES ('c1', 'Archived', 'closed')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO campaign_purchases (id, campaign_id, card_name, set_name, buy_cost_cents, cert_number, purchase_date, created_at)
		VALUES ('p1', 'c1', 'Charizard', 'Base Set', 50000, '11111111', '2024-01-01', CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	repo := NewRefreshCandidateRepository(db.DB)
	candidates, err := repo.GetRefreshCandidates(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, candidates)
}
