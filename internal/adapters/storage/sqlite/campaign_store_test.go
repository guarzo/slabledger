package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCampaignStore_CampaignCRUD(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	c := &inventory.Campaign{
		ID:                  "camp-1",
		Name:                "Vintage Core PSA 8-9",
		Sport:               "Pokemon",
		YearRange:           "1999-2003",
		GradeRange:          "8-9",
		PriceRange:          "250-1500",
		CLConfidence:        "3-4",
		BuyTermsCLPct:       0.80,
		DailySpendCapCents:  150000,
		InclusionList:       "charizard pikachu blastoise",
		Phase:               inventory.PhasePending,
		ExclusionMode:       true,
		PSASourcingFeeCents: 300,
		EbayFeePct:          0.1235,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	// Create
	err := repo.CreateCampaign(ctx, c)
	require.NoError(t, err)

	// Get
	got, err := repo.GetCampaign(ctx, "camp-1")
	require.NoError(t, err)
	assert.Equal(t, c.Name, got.Name)
	assert.Equal(t, c.Sport, got.Sport)
	assert.Equal(t, c.BuyTermsCLPct, got.BuyTermsCLPct)
	assert.Equal(t, c.Phase, got.Phase)
	assert.Equal(t, true, got.ExclusionMode)

	// List
	list, err := repo.ListCampaigns(ctx, false)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// Update
	c.Phase = inventory.PhaseActive
	c.UpdatedAt = time.Now()
	err = repo.UpdateCampaign(ctx, c)
	require.NoError(t, err)

	got, _ = repo.GetCampaign(ctx, "camp-1")
	assert.Equal(t, inventory.PhaseActive, got.Phase)

	// Not found
	_, err = repo.GetCampaign(ctx, "nonexistent")
	assert.ErrorIs(t, err, inventory.ErrCampaignNotFound)
}
