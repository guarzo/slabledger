package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCampaignsRepo(t *testing.T) *CampaignsRepository {
	t.Helper()
	db := setupTestDB(t)
	return NewCampaignsRepository(db.DB)
}

func TestCampaignsRepository_CampaignCRUD(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	c := &campaigns.Campaign{
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
		Phase:               campaigns.PhasePending,
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
	c.Phase = campaigns.PhaseActive
	c.UpdatedAt = time.Now()
	err = repo.UpdateCampaign(ctx, c)
	require.NoError(t, err)

	got, _ = repo.GetCampaign(ctx, "camp-1")
	assert.Equal(t, campaigns.PhaseActive, got.Phase)

	// Not found
	_, err = repo.GetCampaign(ctx, "nonexistent")
	assert.ErrorIs(t, err, campaigns.ErrCampaignNotFound)
}

func TestCampaignsRepository_PurchaseCRUD(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)

	// Create campaign first
	c := &campaigns.Campaign{ID: "camp-1", Name: "Test", Phase: campaigns.PhasePending, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, c))

	p := &campaigns.Purchase{
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
	assert.ErrorIs(t, err, campaigns.ErrDuplicateCertNumber)

	// Count
	count, err := repo.CountPurchasesByCampaign(ctx, "camp-1")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// List unsold
	unsold, err := repo.ListUnsoldPurchases(ctx, "camp-1")
	require.NoError(t, err)
	assert.Len(t, unsold, 1)
}

func TestCampaignsRepository_SaleCRUD(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)

	// Setup campaign + purchase
	c := &campaigns.Campaign{ID: "camp-1", Name: "Test", Phase: campaigns.PhasePending, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, c))

	p := &campaigns.Purchase{ID: "purch-1", CampaignID: "camp-1", CardName: "Charizard", CertNumber: "11111111", GradeValue: 9, BuyCostCents: 80000, PurchaseDate: "2026-01-10", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreatePurchase(ctx, p))

	s := &campaigns.Sale{
		ID:             "sale-1",
		PurchaseID:     "purch-1",
		SaleChannel:    campaigns.SaleChannelEbay,
		SalePriceCents: 95000,
		SaleFeeCents:   11733,
		SaleDate:       "2026-01-25",
		DaysToSell:     15,
		NetProfitCents: 2967,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Create
	err := repo.CreateSale(ctx, s)
	require.NoError(t, err)

	// Get by purchase ID
	got, err := repo.GetSaleByPurchaseID(ctx, "purch-1")
	require.NoError(t, err)
	assert.Equal(t, campaigns.SaleChannelEbay, got.SaleChannel)
	assert.Equal(t, 95000, got.SalePriceCents)

	// Duplicate sale for same purchase
	s2 := *s
	s2.ID = "sale-2"
	err = repo.CreateSale(ctx, &s2)
	assert.ErrorIs(t, err, campaigns.ErrDuplicateSale)

	// List by campaign
	sales, err := repo.ListSalesByCampaign(ctx, "camp-1", 100, 0)
	require.NoError(t, err)
	assert.Len(t, sales, 1)

	// After sale, unsold list should be empty
	unsold, err := repo.ListUnsoldPurchases(ctx, "camp-1")
	require.NoError(t, err)
	assert.Len(t, unsold, 0)
}

func TestCampaignsRepository_UpdatePurchaseCampaign(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)

	// Create two campaigns with different sourcing fees
	src := &campaigns.Campaign{ID: "camp-src", Name: "Source", Phase: campaigns.PhasePending, PSASourcingFeeCents: 300, CreatedAt: now, UpdatedAt: now}
	dst := &campaigns.Campaign{ID: "camp-dst", Name: "Destination", Phase: campaigns.PhaseActive, PSASourcingFeeCents: 500, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, src))
	require.NoError(t, repo.CreateCampaign(ctx, dst))

	// Create a purchase in the source campaign
	p := &campaigns.Purchase{
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
	assert.ErrorIs(t, err, campaigns.ErrPurchaseNotFound)
}

func TestCampaignsRepository_Analytics(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)

	// Setup campaign + purchase + sale
	c := &campaigns.Campaign{ID: "camp-1", Name: "Test", Phase: campaigns.PhasePending, DailySpendCapCents: 100000, EbayFeePct: 0.1235, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, c))

	p := &campaigns.Purchase{ID: "purch-1", CampaignID: "camp-1", CardName: "Charizard", CertNumber: "11111111", GradeValue: 9, CLValueCents: 100000, BuyCostCents: 80000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-10", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreatePurchase(ctx, p))

	s := &campaigns.Sale{ID: "sale-1", PurchaseID: "purch-1", SaleChannel: campaigns.SaleChannelEbay, SalePriceCents: 95000, SaleFeeCents: 11733, SaleDate: "2026-01-25", DaysToSell: 15, NetProfitCents: 2967, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateSale(ctx, s))

	// GetCampaignPNL
	pnl, err := repo.GetCampaignPNL(ctx, "camp-1")
	require.NoError(t, err)
	assert.Equal(t, "camp-1", pnl.CampaignID)
	assert.Equal(t, 1, pnl.TotalPurchases)
	assert.Equal(t, 1, pnl.TotalSold)
	assert.Equal(t, 0, pnl.TotalUnsold)
	assert.Equal(t, 95000, pnl.TotalRevenueCents)

	// GetPNLByChannel
	channels, err := repo.GetPNLByChannel(ctx, "camp-1")
	require.NoError(t, err)
	assert.Len(t, channels, 1)
	assert.Equal(t, campaigns.SaleChannelEbay, channels[0].Channel)

	// GetDaysToSellDistribution
	buckets, err := repo.GetDaysToSellDistribution(ctx, "camp-1")
	require.NoError(t, err)
	assert.Len(t, buckets, 1)
	assert.Equal(t, "15-30", buckets[0].Label)

}

func TestGetPortfolioChannelVelocity(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	// No sales → empty result
	velocity, err := repo.GetPortfolioChannelVelocity(ctx)
	require.NoError(t, err)
	assert.Len(t, velocity, 0)

	// Create campaign and purchases
	c := &campaigns.Campaign{ID: "camp-vel", Name: "Velocity Test", Phase: campaigns.PhaseActive, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, c))

	// Create 3 purchases with different cards
	purchases := []campaigns.Purchase{
		{ID: "pv-1", CampaignID: "camp-vel", CardName: "Charizard", CertNumber: "VEL001", GradeValue: 9, BuyCostCents: 50000, PurchaseDate: "2026-01-01", CreatedAt: now, UpdatedAt: now},
		{ID: "pv-2", CampaignID: "camp-vel", CardName: "Pikachu", CertNumber: "VEL002", GradeValue: 10, BuyCostCents: 30000, PurchaseDate: "2026-01-05", CreatedAt: now, UpdatedAt: now},
		{ID: "pv-3", CampaignID: "camp-vel", CardName: "Blastoise", CertNumber: "VEL003", GradeValue: 9, BuyCostCents: 40000, PurchaseDate: "2026-01-10", CreatedAt: now, UpdatedAt: now},
		{ID: "pv-4", CampaignID: "camp-vel", CardName: "Venusaur", CertNumber: "VEL004", GradeValue: 8, BuyCostCents: 25000, PurchaseDate: "2026-01-12", CreatedAt: now, UpdatedAt: now},
		{ID: "pv-5", CampaignID: "camp-vel", CardName: "Mewtwo", CertNumber: "VEL005", GradeValue: 9, BuyCostCents: 60000, PurchaseDate: "2026-01-15", CreatedAt: now, UpdatedAt: now},
	}
	for i := range purchases {
		require.NoError(t, repo.CreatePurchase(ctx, &purchases[i]))
	}

	// Create sales across different channels:
	// eBay: 2 sales (pv-1, pv-2)
	// inperson: 3 sales (pv-3, pv-4, pv-5) — legacy gamestop and local both normalize to inperson
	sales := []campaigns.Sale{
		{ID: "sv-1", PurchaseID: "pv-1", SaleChannel: campaigns.SaleChannelEbay, SalePriceCents: 65000, SaleFeeCents: 8000, SaleDate: "2026-01-15", DaysToSell: 14, NetProfitCents: 7000, CreatedAt: now, UpdatedAt: now},
		{ID: "sv-2", PurchaseID: "pv-2", SaleChannel: campaigns.SaleChannelEbay, SalePriceCents: 45000, SaleFeeCents: 5500, SaleDate: "2026-01-25", DaysToSell: 20, NetProfitCents: 9500, CreatedAt: now, UpdatedAt: now},
		{ID: "sv-3", PurchaseID: "pv-3", SaleChannel: campaigns.SaleChannelInPerson, SalePriceCents: 50000, SaleFeeCents: 0, SaleDate: "2026-01-12", DaysToSell: 2, NetProfitCents: 10000, CreatedAt: now, UpdatedAt: now},
		{ID: "sv-4", PurchaseID: "pv-4", SaleChannel: campaigns.SaleChannelInPerson, SalePriceCents: 30000, SaleFeeCents: 0, SaleDate: "2026-01-18", DaysToSell: 6, NetProfitCents: 5000, CreatedAt: now, UpdatedAt: now},
		{ID: "sv-5", PurchaseID: "pv-5", SaleChannel: campaigns.SaleChannelInPerson, SalePriceCents: 70000, SaleFeeCents: 0, SaleDate: "2026-01-20", DaysToSell: 5, NetProfitCents: 10000, CreatedAt: now, UpdatedAt: now},
	}
	for i := range sales {
		require.NoError(t, repo.CreateSale(ctx, &sales[i]))
	}

	velocity, err = repo.GetPortfolioChannelVelocity(ctx)
	require.NoError(t, err)
	assert.Len(t, velocity, 2, "expected 2 channels: ebay and inperson")

	// Results should be ordered DESC by count: inperson (3), eBay (2)
	assert.Equal(t, campaigns.SaleChannelInPerson, velocity[0].Channel, "inperson should be first with most sales")
	assert.Equal(t, 3, velocity[0].SaleCount, "inperson should have 3 sales")

	assert.Equal(t, campaigns.SaleChannelEbay, velocity[1].Channel, "ebay should be second")
	assert.Equal(t, 2, velocity[1].SaleCount, "ebay should have 2 sales")

	// Build a map for easier assertions on each channel
	chanMap := make(map[campaigns.SaleChannel]campaigns.ChannelVelocity)
	for _, cv := range velocity {
		chanMap[cv.Channel] = cv
	}

	// eBay: 2 sales, avg days = (14+20)/2 = 17, revenue = 65000+45000 = 110000
	ebay := chanMap[campaigns.SaleChannelEbay]
	assert.Equal(t, 2, ebay.SaleCount)
	assert.InDelta(t, 17.0, ebay.AvgDaysToSell, 0.1)
	assert.Equal(t, 110000, ebay.RevenueCents)

	// InPerson: 3 sales, avg days = (2+6+5)/3 = 4.33, revenue = 50000+30000+70000 = 150000
	inperson := chanMap[campaigns.SaleChannelInPerson]
	assert.Equal(t, 3, inperson.SaleCount)
	assert.InDelta(t, 4.33, inperson.AvgDaysToSell, 0.1)
	assert.Equal(t, 150000, inperson.RevenueCents)
}

func TestGetCapitalSummary_OutstandingAndPayments(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	// Test with no purchases → zero outstanding
	summary, err := repo.GetCapitalSummary(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, summary.OutstandingCents, "no purchases → zero outstanding")

	// Create campaign
	c := &campaigns.Campaign{ID: "camp-credit", Name: "Credit Test", Phase: campaigns.PhaseActive, PSASourcingFeeCents: 300, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateCampaign(ctx, c))

	purchaseDate1 := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	purchaseDate2 := time.Now().AddDate(0, 0, -20).Format("2006-01-02")

	p1 := &campaigns.Purchase{
		ID: "pc-1", CampaignID: "camp-credit", CardName: "Charizard",
		CertNumber: "CR001", GradeValue: 9, BuyCostCents: 50000,
		PSASourcingFeeCents: 300, PurchaseDate: purchaseDate1,
		InvoiceDate: purchaseDate1,
		CreatedAt:   now, UpdatedAt: now,
	}
	p2 := &campaigns.Purchase{
		ID: "pc-2", CampaignID: "camp-credit", CardName: "Pikachu",
		CertNumber: "CR002", GradeValue: 10, BuyCostCents: 30000,
		PSASourcingFeeCents: 300, PurchaseDate: purchaseDate2,
		InvoiceDate: purchaseDate2,
		CreatedAt:   now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreatePurchase(ctx, p1))
	require.NoError(t, repo.CreatePurchase(ctx, p2))

	// With no invoices paid, outstanding = total invoiced spend = (50000+300) + (30000+300) = 80600
	summary, err = repo.GetCapitalSummary(ctx)
	require.NoError(t, err)
	assert.Equal(t, 80600, summary.OutstandingCents, "outstanding = total invoiced spend with no payments")

	// Create an unpaid invoice and partially pay it
	dueDate := time.Now().AddDate(0, 0, 15).Format("2006-01-02")
	inv := &campaigns.Invoice{
		ID:          "inv-1",
		InvoiceDate: purchaseDate1,
		TotalCents:  50300,
		PaidCents:   0,
		DueDate:     dueDate,
		Status:      "unpaid",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	require.NoError(t, repo.CreateInvoice(ctx, inv))

	inv.PaidCents = 20000
	inv.Status = "partial"
	inv.UpdatedAt = now
	require.NoError(t, repo.UpdateInvoice(ctx, inv))

	summary, err = repo.GetCapitalSummary(ctx)
	require.NoError(t, err)
	assert.Equal(t, 80600-20000, summary.OutstandingCents,
		"outstanding should be reduced by paid amount")
	assert.Equal(t, 20000, summary.PaidCents)

	// Test with refunded purchase: add a refunded purchase and verify it doesn't count in outstanding
	p3 := &campaigns.Purchase{
		ID: "pc-3", CampaignID: "camp-credit", CardName: "Blastoise",
		CertNumber: "CR003", GradeValue: 9, BuyCostCents: 40000,
		PSASourcingFeeCents: 300, PurchaseDate: purchaseDate2,
		InvoiceDate: purchaseDate2, WasRefunded: true,
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreatePurchase(ctx, p3))

	summary, err = repo.GetCapitalSummary(ctx)
	require.NoError(t, err)
	assert.Equal(t, 80600-20000, summary.OutstandingCents,
		"refunded purchase should not increase outstanding")
	assert.Equal(t, 40300, summary.RefundedCents)

	// Test with all invoices fully paid → outstanding = 0
	inv2 := &campaigns.Invoice{
		ID:          "inv-2",
		InvoiceDate: purchaseDate2,
		TotalCents:  30300,
		PaidCents:   30300,
		DueDate:     dueDate,
		Status:      "paid",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	require.NoError(t, repo.CreateInvoice(ctx, inv2))

	inv.PaidCents = inv.TotalCents
	inv.Status = "paid"
	inv.UpdatedAt = now
	require.NoError(t, repo.UpdateInvoice(ctx, inv))

	summary, err = repo.GetCapitalSummary(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, summary.OutstandingCents,
		"fully paid should result in zero outstanding")
}
