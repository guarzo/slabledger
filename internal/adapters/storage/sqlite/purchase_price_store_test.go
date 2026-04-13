package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedPurchaseWithAISuggestion creates a campaign + purchase and sets an AI suggestion,
// returning the purchase ID and the suggested price.
func seedPurchaseWithAISuggestion(t *testing.T, repo *testCampaignsRepository, suggestedCents int) (purchaseID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	c := &inventory.Campaign{ID: "camp-ai", Name: "AI Test", Phase: inventory.PhasePending, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, c))

	p := &inventory.Purchase{
		ID:           "purch-ai-1",
		CampaignID:   "camp-ai",
		CardName:     "Pikachu",
		CertNumber:   "99990001",
		GradeValue:   10,
		CLValueCents: 50000,
		BuyCostCents: 40000,
		PurchaseDate: "2026-01-01",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, repo.CreatePurchase(ctx, p))
	require.NoError(t, repo.UpdatePurchaseAISuggestion(ctx, p.ID, suggestedCents))
	return p.ID
}

func TestAcceptAISuggestion(t *testing.T) {
	tests := []struct {
		name       string
		setupPrice int // AI suggested price stored in DB
		acceptWith int // price passed to AcceptAISuggestion
		wantErr    error
	}{
		{
			name:       "happy path — matching price",
			setupPrice: 48000,
			acceptWith: 48000,
			wantErr:    nil,
		},
		{
			name:       "stale suggestion — price changed between fetch and accept",
			setupPrice: 48000,
			acceptWith: 47000, // caller thinks price is 47000, but DB has 48000
			wantErr:    inventory.ErrNoAISuggestion,
		},
		{
			name:       "zero accept price rejected before hitting DB",
			setupPrice: 48000,
			acceptWith: 0,
			wantErr:    inventory.ErrNoAISuggestion,
		},
		{
			name:       "negative accept price rejected before hitting DB",
			setupPrice: 48000,
			acceptWith: -1,
			wantErr:    inventory.ErrNoAISuggestion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupCampaignsRepo(t)
			ctx := context.Background()
			purchaseID := seedPurchaseWithAISuggestion(t, repo, tt.setupPrice)

			err := repo.AcceptAISuggestion(ctx, purchaseID, tt.acceptWith)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr),
					"expected %v, got %v", tt.wantErr, err)
				return
			}
			require.NoError(t, err)

			// Verify that the override was persisted and the suggestion was cleared.
			purchases, err := repo.ListPurchasesByCampaign(ctx, "camp-ai", 100, 0)
			require.NoError(t, err)
			require.Len(t, purchases, 1)
			p := purchases[0]
			assert.Equal(t, tt.acceptWith, p.OverridePriceCents, "override_price_cents should be set")
			assert.Equal(t, inventory.OverrideSourceAIAccepted, p.OverrideSource, "override_source should be ai_accepted")
			assert.Equal(t, 0, p.AISuggestedPriceCents, "ai_suggested_price_cents should be cleared")
		})
	}
}

func TestAcceptAISuggestion_PurchaseNotFound(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()

	err := repo.AcceptAISuggestion(ctx, "non-existent-id", 48000)

	require.Error(t, err)
	assert.True(t, errors.Is(err, inventory.ErrNoAISuggestion),
		"should return ErrNoAISuggestion for missing purchase")
}
