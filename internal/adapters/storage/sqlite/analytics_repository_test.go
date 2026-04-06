package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupAnalyticsData creates a campaign with multiple purchases and sales across channels/grades.
// Returns the campaign ID for use in analytics queries.
func setupAnalyticsData(t *testing.T, repo *CampaignsRepository) string {
	t.Helper()
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	c := &campaigns.Campaign{
		ID: "camp-analytics", Name: "Analytics Test", Phase: campaigns.PhaseActive,
		EbayFeePct: 0.1235, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreateCampaign(ctx, c))

	purchases := []campaigns.Purchase{
		{ID: "ap-1", CampaignID: "camp-analytics", CardName: "Charizard", CertNumber: "A0001", GradeValue: 10, CLValueCents: 200000, BuyCostCents: 150000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-01", CreatedAt: now, UpdatedAt: now},
		{ID: "ap-2", CampaignID: "camp-analytics", CardName: "Pikachu", CertNumber: "A0002", GradeValue: 9, CLValueCents: 50000, BuyCostCents: 40000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-05", CreatedAt: now, UpdatedAt: now},
		{ID: "ap-3", CampaignID: "camp-analytics", CardName: "Blastoise", CertNumber: "A0003", GradeValue: 9, CLValueCents: 80000, BuyCostCents: 60000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-10", CreatedAt: now, UpdatedAt: now},
		{ID: "ap-4", CampaignID: "camp-analytics", CardName: "Venusaur", CertNumber: "A0004", GradeValue: 10, CLValueCents: 100000, BuyCostCents: 80000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-12", CreatedAt: now, UpdatedAt: now},
		{ID: "ap-5", CampaignID: "camp-analytics", CardName: "Mewtwo", CertNumber: "A0005", GradeValue: 8, CLValueCents: 30000, BuyCostCents: 20000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-15", CreatedAt: now, UpdatedAt: now},
	}
	for i := range purchases {
		require.NoError(t, repo.CreatePurchase(ctx, &purchases[i]))
	}

	// Sales: ap-1 eBay, ap-2 inperson, ap-3 eBay, ap-5 inperson (ap-4 unsold)
	sales := []campaigns.Sale{
		{ID: "as-1", PurchaseID: "ap-1", SaleChannel: campaigns.SaleChannelEbay, SalePriceCents: 180000, SaleFeeCents: 22230, SaleDate: "2026-01-20", DaysToSell: 19, NetProfitCents: 7470, CreatedAt: now, UpdatedAt: now},
		{ID: "as-2", PurchaseID: "ap-2", SaleChannel: campaigns.SaleChannelInPerson, SalePriceCents: 55000, SaleFeeCents: 0, SaleDate: "2026-01-08", DaysToSell: 3, NetProfitCents: 14700, CreatedAt: now, UpdatedAt: now},
		{ID: "as-3", PurchaseID: "ap-3", SaleChannel: campaigns.SaleChannelEbay, SalePriceCents: 75000, SaleFeeCents: 9263, SaleDate: "2026-02-05", DaysToSell: 26, NetProfitCents: 5437, CreatedAt: now, UpdatedAt: now},
		{ID: "as-5", PurchaseID: "ap-5", SaleChannel: campaigns.SaleChannelInPerson, SalePriceCents: 25000, SaleFeeCents: 0, SaleDate: "2026-01-15", DaysToSell: 0, NetProfitCents: 4700, CreatedAt: now, UpdatedAt: now},
	}
	for i := range sales {
		require.NoError(t, repo.CreateSale(ctx, &sales[i]))
	}

	return "camp-analytics"
}

func TestGetPNLByChannel(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	t.Run("empty campaign", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)
		c := &campaigns.Campaign{ID: "camp-empty-ch", Name: "Empty", Phase: campaigns.PhaseActive, CreatedAt: now, UpdatedAt: now}
		require.NoError(t, repo.CreateCampaign(ctx, c))

		channels, err := repo.GetPNLByChannel(ctx, "camp-empty-ch")
		require.NoError(t, err)
		assert.Empty(t, channels)
	})

	t.Run("multiple channels", func(t *testing.T) {
		campID := setupAnalyticsData(t, repo)

		channels, err := repo.GetPNLByChannel(ctx, campID)
		require.NoError(t, err)
		assert.Len(t, channels, 2, "expected eBay and inperson channels")

		chanMap := make(map[campaigns.SaleChannel]campaigns.ChannelPNL)
		for _, ch := range channels {
			chanMap[ch.Channel] = ch
		}

		// eBay: 2 sales (as-1: 180000 rev, 22230 fees, 7470 profit; as-3: 75000 rev, 9263 fees, 5437 profit)
		ebay := chanMap[campaigns.SaleChannelEbay]
		assert.Equal(t, 2, ebay.SaleCount)
		assert.Equal(t, 255000, ebay.RevenueCents)
		assert.Equal(t, 31493, ebay.FeesCents)
		assert.Equal(t, 12907, ebay.NetProfitCents)
		assert.InDelta(t, 22.5, ebay.AvgDaysToSell, 0.1) // (19+26)/2

		// InPerson: 2 sales (as-2: 55000 rev, 0 fees, 14700 profit; as-5: 25000 rev, 0 fees, 4700 profit)
		inperson := chanMap[campaigns.SaleChannelInPerson]
		assert.Equal(t, 2, inperson.SaleCount)
		assert.Equal(t, 80000, inperson.RevenueCents)
		assert.Equal(t, 0, inperson.FeesCents)
		assert.Equal(t, 19400, inperson.NetProfitCents)
		assert.InDelta(t, 1.5, inperson.AvgDaysToSell, 0.1) // (3+0)/2

		// Ordered by net_profit DESC: inperson (19400) > ebay (12907)
		assert.Equal(t, campaigns.SaleChannelInPerson, channels[0].Channel)
		assert.Equal(t, campaigns.SaleChannelEbay, channels[1].Channel)
	})

	t.Run("nonexistent campaign", func(t *testing.T) {
		channels, err := repo.GetPNLByChannel(ctx, "nonexistent")
		require.NoError(t, err)
		assert.Empty(t, channels)
	})
}

func TestGetDaysToSellDistribution(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	t.Run("empty campaign", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)
		c := &campaigns.Campaign{ID: "camp-empty-dts", Name: "Empty", Phase: campaigns.PhaseActive, CreatedAt: now, UpdatedAt: now}
		require.NoError(t, repo.CreateCampaign(ctx, c))

		buckets, err := repo.GetDaysToSellDistribution(ctx, "camp-empty-dts")
		require.NoError(t, err)
		assert.Empty(t, buckets)
	})

	t.Run("multiple buckets", func(t *testing.T) {
		campID := setupAnalyticsData(t, repo)

		buckets, err := repo.GetDaysToSellDistribution(ctx, campID)
		require.NoError(t, err)

		// Sales: DaysToSell = 19 (15-30), 3 (0-7), 26 (15-30), 0 (0-7)
		// Expected buckets: "0-7" (count 2), "15-30" (count 2)
		assert.Len(t, buckets, 2)

		bucketMap := make(map[string]campaigns.DaysToSellBucket)
		for _, b := range buckets {
			bucketMap[b.Label] = b
		}

		b07 := bucketMap["0-7"]
		assert.Equal(t, 2, b07.Count)
		assert.Equal(t, 0, b07.Min)
		assert.Equal(t, 7, b07.Max)

		b1530 := bucketMap["15-30"]
		assert.Equal(t, 2, b1530.Count)
		assert.Equal(t, 15, b1530.Min)
		assert.Equal(t, 30, b1530.Max)

		// Ordered by min_val ASC
		assert.Equal(t, "0-7", buckets[0].Label)
		assert.Equal(t, "15-30", buckets[1].Label)
	})

	t.Run("same day sale lands in 0-7 bucket", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)
		c := &campaigns.Campaign{ID: "camp-sameday", Name: "Same Day", Phase: campaigns.PhaseActive, CreatedAt: now, UpdatedAt: now}
		require.NoError(t, repo.CreateCampaign(ctx, c))

		p := &campaigns.Purchase{ID: "sd-p1", CampaignID: "camp-sameday", CardName: "Mew", CertNumber: "SD001", GradeValue: 10, BuyCostCents: 50000, PurchaseDate: "2026-03-01", CreatedAt: now, UpdatedAt: now}
		require.NoError(t, repo.CreatePurchase(ctx, p))

		s := &campaigns.Sale{ID: "sd-s1", PurchaseID: "sd-p1", SaleChannel: campaigns.SaleChannelInPerson, SalePriceCents: 60000, SaleFeeCents: 0, SaleDate: "2026-03-01", DaysToSell: 0, NetProfitCents: 10000, CreatedAt: now, UpdatedAt: now}
		require.NoError(t, repo.CreateSale(ctx, s))

		buckets, err := repo.GetDaysToSellDistribution(ctx, "camp-sameday")
		require.NoError(t, err)
		require.Len(t, buckets, 1)
		assert.Equal(t, "0-7", buckets[0].Label)
		assert.Equal(t, 1, buckets[0].Count)
	})
}

func TestGetPerformanceByGrade_Analytics(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	repo := NewCampaignsRepository(db.DB)
	ctx := context.Background()

	t.Run("empty campaign", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)
		c := &campaigns.Campaign{ID: "camp-empty-gr", Name: "Empty", Phase: campaigns.PhaseActive, CreatedAt: now, UpdatedAt: now}
		require.NoError(t, repo.CreateCampaign(ctx, c))

		grades, err := repo.GetPerformanceByGrade(ctx, "camp-empty-gr")
		require.NoError(t, err)
		assert.Empty(t, grades)
	})

	t.Run("multiple grades with sold and unsold", func(t *testing.T) {
		campID := setupAnalyticsData(t, repo)

		grades, err := repo.GetPerformanceByGrade(ctx, campID)
		require.NoError(t, err)

		// Purchases by grade:
		//   8:  ap-5 (sold, cost=20300)
		//   9:  ap-2 (sold, cost=40300), ap-3 (sold, cost=60300)
		//   10: ap-1 (sold, cost=150300), ap-4 (unsold, cost=80300)
		assert.Len(t, grades, 3)

		gradeMap := make(map[float64]campaigns.GradePerformance)
		for _, g := range grades {
			gradeMap[g.Grade] = g
		}

		// Grade 8: 1 purchase, 1 sold
		g8 := gradeMap[8]
		assert.Equal(t, 1, g8.PurchaseCount)
		assert.Equal(t, 1, g8.SoldCount)
		assert.InDelta(t, 1.0, g8.SellThroughPct, 0.01)
		assert.Equal(t, 20300, g8.TotalSpendCents) // 20000 + 300
		assert.Equal(t, 25000, g8.TotalRevenueCents)
		assert.Equal(t, 4700, g8.NetProfitCents)
		assert.InDelta(t, 0.0, g8.AvgDaysToSell, 0.1) // same day

		// Grade 9: 2 purchases, 2 sold
		g9 := gradeMap[9]
		assert.Equal(t, 2, g9.PurchaseCount)
		assert.Equal(t, 2, g9.SoldCount)
		assert.InDelta(t, 1.0, g9.SellThroughPct, 0.01)
		assert.Equal(t, 100600, g9.TotalSpendCents) // (40000+300) + (60000+300)
		assert.Equal(t, 130000, g9.TotalRevenueCents)
		assert.Equal(t, 20137, g9.NetProfitCents)      // 14700 + 5437
		assert.InDelta(t, 14.5, g9.AvgDaysToSell, 0.1) // (3+26)/2

		// Grade 10: 2 purchases, 1 sold (ap-4 unsold)
		g10 := gradeMap[10]
		assert.Equal(t, 2, g10.PurchaseCount)
		assert.Equal(t, 1, g10.SoldCount)
		assert.InDelta(t, 0.5, g10.SellThroughPct, 0.01)
		assert.Equal(t, 230600, g10.TotalSpendCents) // (150000+300) + (80000+300)
		assert.Equal(t, 180000, g10.TotalRevenueCents)
		assert.Equal(t, 7470, g10.NetProfitCents)

		// AvgBuyPctOfCL: grade 9 = avg(40000/50000, 60000/80000) = 0.775; grade 8 = 20000/30000 = 0.667
		assert.InDelta(t, 0.775, g9.AvgBuyPctOfCL, 0.01)
		assert.InDelta(t, 0.667, g8.AvgBuyPctOfCL, 0.01)

		// ROI check for grade 9: net/spend
		assert.InDelta(t, float64(20137)/float64(100600), g9.ROI, 0.001)

		// Ordered by grade ASC
		assert.Equal(t, float64(8), grades[0].Grade)
		assert.Equal(t, float64(9), grades[1].Grade)
		assert.Equal(t, float64(10), grades[2].Grade)
	})

}
