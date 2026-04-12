package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPurchaseStore_PurchaseCRUD(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)

	// Create campaign first
	c := &inventory.Campaign{ID: "camp-1", Name: "Test", Phase: inventory.PhasePending, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, c))

	p := &inventory.Purchase{
		ID:                  "purch-1",
		CampaignID:          "camp-1",
		CardName:            "Charizard",
		CertNumber:          "11111111",
		GradeValue:          9.5,
		CLValueCents:        100000,
		BuyCostCents:        80000,
		PSASourcingFeeCents: 300,
		PurchaseDate:        "2026-01-10",
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	// Create
	err := repo.CreatePurchase(ctx, p)
	require.NoError(t, err)

	// Get
	got, err := repo.GetPurchase(ctx, "purch-1")
	require.NoError(t, err)
	assert.Equal(t, "Charizard", got.CardName)
	assert.Equal(t, 9.5, got.GradeValue)

	// Duplicate cert number
	p2 := *p
	p2.ID = "purch-2"
	err = repo.CreatePurchase(ctx, &p2)
	assert.ErrorIs(t, err, inventory.ErrDuplicateCertNumber)

	// Count
	count, err := repo.CountPurchasesByCampaign(ctx, "camp-1")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// List unsold
	unsold, err := repo.ListUnsoldPurchases(ctx, "camp-1")
	require.NoError(t, err)
	assert.Len(t, unsold, 1)
}

func TestPurchaseStore_UpdatePurchaseCampaign(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)

	// Create two campaigns with different sourcing fees
	src := &inventory.Campaign{ID: "camp-src", Name: "Source", Phase: inventory.PhasePending, PSASourcingFeeCents: 300, CreatedAt: now, UpdatedAt: now}
	dst := &inventory.Campaign{ID: "camp-dst", Name: "Destination", Phase: inventory.PhaseActive, PSASourcingFeeCents: 500, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, src))
	require.NoError(t, repo.CreateCampaign(ctx, dst))

	// Create a purchase in the source campaign
	p := &inventory.Purchase{
		ID: "purch-1", CampaignID: "camp-src", CardName: "Charizard",
		CertNumber: "99999999", GradeValue: 9, BuyCostCents: 80000,
		PSASourcingFeeCents: 300, PurchaseDate: "2026-01-10",
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreatePurchase(ctx, p))

	// Successful reassignment
	err := repo.UpdatePurchaseCampaign(ctx, "purch-1", "camp-dst", 500)
	require.NoError(t, err)

	got, err := repo.GetPurchase(ctx, "purch-1")
	require.NoError(t, err)
	assert.Equal(t, "camp-dst", got.CampaignID)
	assert.Equal(t, 500, got.PSASourcingFeeCents)

	// Nonexistent purchase
	err = repo.UpdatePurchaseCampaign(ctx, "nonexistent", "camp-dst", 500)
	assert.ErrorIs(t, err, inventory.ErrPurchaseNotFound)
}
