package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPurchaseStore_GetDHStatusByCertNumber(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	require.NoError(t, repo.CreateCampaign(ctx, &inventory.Campaign{
		ID: "camp-1", Name: "Camp 1", Phase: inventory.PhasePending, CreatedAt: now, UpdatedAt: now,
	}))

	// Seed a purchase with cert=CERT1 and dh_status=listed.
	p := newTestPurchase("camp-1", "CERT1")
	p.DHStatus = inventory.DHStatusListed
	require.NoError(t, repo.CreatePurchase(ctx, p))

	tests := []struct {
		name       string
		certNumber string
		wantID     string
		wantStatus string
	}{
		{name: "existing cert returns id and dh_status", certNumber: "CERT1", wantID: p.ID, wantStatus: inventory.DHStatusListed},
		{name: "missing cert returns empty id, empty status, nil error", certNumber: "NOPE", wantID: "", wantStatus: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotStatus, err := repo.GetDHStatusByCertNumber(ctx, tt.certNumber)
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, gotID)
			assert.Equal(t, tt.wantStatus, gotStatus)
		})
	}
}
