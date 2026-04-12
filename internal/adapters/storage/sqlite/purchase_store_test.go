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

func TestPurchaseStore_GetPurchase_NotFound(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		id      string
		wantErr error
	}{
		{
			name:    "not found returns ErrPurchaseNotFound",
			id:      "nonexistent",
			wantErr: inventory.ErrPurchaseNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := repo.GetPurchase(ctx, tt.id)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestPurchaseStore_ListPurchasesByCampaign(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	require.NoError(t, repo.CreateCampaign(ctx, &inventory.Campaign{ID: "lc-camp-1", Name: "Camp 1", Phase: inventory.PhasePending, CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, repo.CreateCampaign(ctx, &inventory.Campaign{ID: "lc-camp-2", Name: "Camp 2", Phase: inventory.PhasePending, CreatedAt: now, UpdatedAt: now}))

	p1 := newTestPurchase("lc-camp-1", "lc-11111111")
	p2 := newTestPurchase("lc-camp-1", "lc-22222222")
	pOther := newTestPurchase("lc-camp-2", "lc-33333333")
	require.NoError(t, repo.CreatePurchase(ctx, p1))
	require.NoError(t, repo.CreatePurchase(ctx, p2))
	require.NoError(t, repo.CreatePurchase(ctx, pOther))

	tests := []struct {
		name       string
		campaignID string
		limit      int
		offset     int
		wantCount  int
	}{
		{name: "two purchases for camp-1", campaignID: "lc-camp-1", limit: 100, offset: 0, wantCount: 2},
		{name: "one purchase for camp-2", campaignID: "lc-camp-2", limit: 100, offset: 0, wantCount: 1},
		{name: "empty for unknown campaign", campaignID: "unknown", limit: 100, offset: 0, wantCount: 0},
		{name: "limit 1 returns only one", campaignID: "lc-camp-1", limit: 1, offset: 0, wantCount: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.ListPurchasesByCampaign(ctx, tt.campaignID, tt.limit, tt.offset)
			require.NoError(t, err)
			require.Len(t, got, tt.wantCount)
		})
	}
}

func TestPurchaseStore_GetPurchaseByCertNumber(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	require.NoError(t, repo.CreateCampaign(ctx, &inventory.Campaign{ID: "gc-camp-1", Name: "Camp 1", Phase: inventory.PhasePending, CreatedAt: now, UpdatedAt: now}))
	p := newTestPurchase("gc-camp-1", "gc-99887766")
	require.NoError(t, repo.CreatePurchase(ctx, p))

	tests := []struct {
		name       string
		grader     string
		certNumber string
		wantErr    error
	}{
		{name: "existing cert returns purchase", grader: "PSA", certNumber: "gc-99887766", wantErr: nil},
		{name: "wrong grader returns ErrPurchaseNotFound", grader: "CGC", certNumber: "gc-99887766", wantErr: inventory.ErrPurchaseNotFound},
		{name: "missing cert returns ErrPurchaseNotFound", grader: "PSA", certNumber: "gc-00000000", wantErr: inventory.ErrPurchaseNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.GetPurchaseByCertNumber(ctx, tt.grader, tt.certNumber)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			require.Equal(t, tt.certNumber, got.CertNumber)
		})
	}
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
