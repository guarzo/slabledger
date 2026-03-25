package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPerformanceByGrade(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	// Setup campaign
	c := &campaigns.Campaign{ID: "camp-1", Name: "Test", Phase: campaigns.PhasePending, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, c))

	// Create purchases at different grades
	purchases := []*campaigns.Purchase{
		{ID: "p1", CampaignID: "camp-1", CardName: "Charizard", CertNumber: "111", GradeValue: 9, CLValueCents: 100000, BuyCostCents: 80000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-01", CreatedAt: now, UpdatedAt: now},
		{ID: "p2", CampaignID: "camp-1", CardName: "Pikachu", CertNumber: "222", GradeValue: 9.5, CLValueCents: 50000, BuyCostCents: 40000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-02", CreatedAt: now, UpdatedAt: now},
		{ID: "p3", CampaignID: "camp-1", CardName: "Blastoise", CertNumber: "333", GradeValue: 10, CLValueCents: 200000, BuyCostCents: 170000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-03", CreatedAt: now, UpdatedAt: now},
	}
	for _, p := range purchases {
		require.NoError(t, repo.CreatePurchase(ctx, p))
	}

	// Create a sale for the first purchase
	sale := &campaigns.Sale{
		ID: "s1", PurchaseID: "p1", SaleChannel: campaigns.SaleChannelEbay,
		SalePriceCents: 95000, SaleFeeCents: 11733, SaleDate: "2026-01-15",
		DaysToSell: 14, NetProfitCents: 2967, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreateSale(ctx, sale))

	// Test
	result, err := repo.GetPerformanceByGrade(ctx, "camp-1")
	require.NoError(t, err)
	require.Len(t, result, 3, "expected 3 grades (9, 9.5 and 10)")

	// Grade 9: 1 purchase, 1 sold; Grade 9.5: 1 purchase, 0 sold; Grade 10: 1 purchase, 0 sold
	var grade9, grade95, grade10 *campaigns.GradePerformance
	for i := range result {
		switch result[i].Grade {
		case 9:
			grade9 = &result[i]
		case 9.5:
			grade95 = &result[i]
		case 10:
			grade10 = &result[i]
		}
	}

	require.NotNil(t, grade9)
	assert.Equal(t, 1, grade9.PurchaseCount)
	assert.Equal(t, 1, grade9.SoldCount)
	assert.InDelta(t, 1.0, grade9.SellThroughPct, 0.01)
	assert.Equal(t, 14.0, grade9.AvgDaysToSell)

	require.NotNil(t, grade95)
	assert.Equal(t, 1, grade95.PurchaseCount)
	assert.Equal(t, 0, grade95.SoldCount)
	assert.InDelta(t, 9.5, grade95.Grade, 0.01)

	require.NotNil(t, grade10)
	assert.Equal(t, 1, grade10.PurchaseCount)
	assert.Equal(t, 0, grade10.SoldCount)
	assert.Equal(t, 0.0, grade10.SellThroughPct)
}

func TestGetPerformanceByGrade_Empty(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	c := &campaigns.Campaign{ID: "camp-1", Name: "Test", Phase: campaigns.PhasePending, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, c))

	result, err := repo.GetPerformanceByGrade(ctx, "camp-1")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestGetPurchasesWithSales(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	// Setup campaign
	c := &campaigns.Campaign{ID: "camp-1", Name: "Test", Phase: campaigns.PhasePending, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, c))

	// Create purchases
	p1 := &campaigns.Purchase{ID: "p1", CampaignID: "camp-1", CardName: "Charizard", CertNumber: "111", GradeValue: 9, CLValueCents: 100000, BuyCostCents: 80000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-01", CreatedAt: now, UpdatedAt: now}
	p2 := &campaigns.Purchase{ID: "p2", CampaignID: "camp-1", CardName: "Pikachu", CertNumber: "222", GradeValue: 9.5, CLValueCents: 50000, BuyCostCents: 40000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-02", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreatePurchase(ctx, p1))
	require.NoError(t, repo.CreatePurchase(ctx, p2))

	// Create a sale for p1 only
	sale := &campaigns.Sale{
		ID: "s1", PurchaseID: "p1", SaleChannel: campaigns.SaleChannelEbay,
		SalePriceCents: 95000, SaleFeeCents: 11733, SaleDate: "2026-01-15",
		DaysToSell: 14, NetProfitCents: 2967, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreateSale(ctx, sale))

	// Test
	result, err := repo.GetPurchasesWithSales(ctx, "camp-1")
	require.NoError(t, err)
	require.Len(t, result, 2)

	// Results ordered by purchase_date DESC, so p2 first
	assert.Equal(t, "p2", result[0].Purchase.ID)
	assert.Equal(t, 9.5, result[0].Purchase.GradeValue)
	assert.Nil(t, result[0].Sale, "p2 should have no sale")

	assert.Equal(t, "p1", result[1].Purchase.ID)
	require.NotNil(t, result[1].Sale, "p1 should have a sale")
	assert.Equal(t, "s1", result[1].Sale.ID)
	assert.Equal(t, 95000, result[1].Sale.SalePriceCents)
	assert.Equal(t, 14, result[1].Sale.DaysToSell)
}

func TestGetPurchasesWithSales_Empty(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	c := &campaigns.Campaign{ID: "camp-1", Name: "Test", Phase: campaigns.PhasePending, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, c))

	result, err := repo.GetPurchasesWithSales(ctx, "camp-1")
	require.NoError(t, err)
	assert.Empty(t, result)
}
