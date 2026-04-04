package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSale(purchaseID string) *campaigns.Sale {
	now := time.Now().Truncate(time.Second)
	return &campaigns.Sale{
		ID:             "sale-" + purchaseID,
		PurchaseID:     purchaseID,
		SaleChannel:    campaigns.SaleChannelEbay,
		SalePriceCents: 95000,
		SaleFeeCents:   11733,
		SaleDate:       "2026-02-01",
		DaysToSell:     17,
		NetProfitCents: 2967,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func setupSaleTestData(t *testing.T, db *DB, repo *CampaignsRepository, campaignID, certNumber string) *campaigns.Purchase {
	t.Helper()
	createTestCampaign(t, db, campaignID, "Sale Test "+campaignID)
	p := newTestPurchase(campaignID, certNumber)
	require.NoError(t, repo.CreatePurchase(context.Background(), p))
	return p
}

func TestCreateSale(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	p := setupSaleTestData(t, db, repo, "camp-s1", "90000001")

	t.Run("success", func(t *testing.T) {
		s := newTestSale(p.ID)
		err := repo.CreateSale(ctx, s)
		require.NoError(t, err)
	})

	t.Run("duplicate sale for same purchase", func(t *testing.T) {
		dup := newTestSale(p.ID)
		dup.ID = "sale-dup"
		err := repo.CreateSale(ctx, dup)
		assert.ErrorIs(t, err, campaigns.ErrDuplicateSale)
		assert.True(t, campaigns.IsDuplicateSale(err))
	})
}

func TestGetSaleByPurchaseID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	p := setupSaleTestData(t, db, repo, "camp-s2", "91000001")

	t.Run("found", func(t *testing.T) {
		s := newTestSale(p.ID)
		require.NoError(t, repo.CreateSale(ctx, s))

		got, err := repo.GetSaleByPurchaseID(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, campaigns.SaleChannelEbay, got.SaleChannel)
		assert.Equal(t, 95000, got.SalePriceCents)
		assert.Equal(t, 17, got.DaysToSell)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := repo.GetSaleByPurchaseID(ctx, "nonexistent")
		assert.ErrorIs(t, err, campaigns.ErrSaleNotFound)
		assert.True(t, campaigns.IsSaleNotFound(err))
	})
}

func TestListSalesByCampaign(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-s3", "Sales Campaign")

	certs := []string{"92000001", "92000002", "92000003"}
	for i, cert := range certs {
		p := newTestPurchase("camp-s3", cert)
		p.PurchaseDate = time.Date(2026, 1, 10+i, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		require.NoError(t, repo.CreatePurchase(ctx, p))

		s := newTestSale(p.ID)
		s.SaleDate = time.Date(2026, 2, 1+i, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		require.NoError(t, repo.CreateSale(ctx, s))
	}

	t.Run("filters by campaign", func(t *testing.T) {
		sales, err := repo.ListSalesByCampaign(ctx, "camp-s3", 100, 0)
		require.NoError(t, err)
		assert.Len(t, sales, 3)
	})

	t.Run("pagination", func(t *testing.T) {
		sales, err := repo.ListSalesByCampaign(ctx, "camp-s3", 2, 0)
		require.NoError(t, err)
		assert.Len(t, sales, 2)

		sales2, err := repo.ListSalesByCampaign(ctx, "camp-s3", 2, 2)
		require.NoError(t, err)
		assert.Len(t, sales2, 1)
	})

	t.Run("empty campaign", func(t *testing.T) {
		sales, err := repo.ListSalesByCampaign(ctx, "camp-nonexistent", 100, 0)
		require.NoError(t, err)
		assert.Empty(t, sales)
	})
}

func TestListUnsoldPurchases(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	createTestCampaign(t, db, "camp-s4", "Unsold Test")

	p1 := newTestPurchase("camp-s4", "93000001")
	p2 := newTestPurchase("camp-s4", "93000002")
	p3 := newTestPurchase("camp-s4", "93000003")
	require.NoError(t, repo.CreatePurchase(ctx, p1))
	require.NoError(t, repo.CreatePurchase(ctx, p2))
	require.NoError(t, repo.CreatePurchase(ctx, p3))

	unsold, err := repo.ListUnsoldPurchases(ctx, "camp-s4")
	require.NoError(t, err)
	assert.Len(t, unsold, 3)

	s := newTestSale(p2.ID)
	require.NoError(t, repo.CreateSale(ctx, s))

	unsold, err = repo.ListUnsoldPurchases(ctx, "camp-s4")
	require.NoError(t, err)
	assert.Len(t, unsold, 2)

	unsoldIDs := make(map[string]bool)
	for _, u := range unsold {
		unsoldIDs[u.ID] = true
	}
	assert.True(t, unsoldIDs[p1.ID], "p1 should be unsold")
	assert.False(t, unsoldIDs[p2.ID], "p2 should be sold")
	assert.True(t, unsoldIDs[p3.ID], "p3 should be unsold")
}
