package sqlite

import (
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	logger := mocks.NewMockLogger()

	db, err := Open(":memory:", logger)
	require.NoError(t, err)

	err = RunMigrations(db, "migrations")
	require.NoError(t, err)

	return db
}

func setupCampaignsRepo(t *testing.T) *testCampaignsRepository {
	t.Helper()
	db := setupTestDB(t)
	logger := mocks.NewMockLogger()
	return &testCampaignsRepository{
		CampaignStore:  NewCampaignStore(db.DB, logger),
		PurchaseStore:  NewPurchaseStore(db.DB, logger),
		SaleStore:      NewSaleStore(db.DB, logger),
		AnalyticsStore: NewAnalyticsStore(db.DB, logger),
		FinanceStore:   NewFinanceStore(db.DB, logger),
		PricingStore:   NewPricingStore(db.DB, logger),
		DHStore:        NewDHStore(db.DB, logger),
		SellSheetStore: NewSellSheetStore(db.DB, logger),
	}
}

// testCampaignsRepository is a test-only composite that wraps individual stores
// to provide backward compatibility for existing test code.
type testCampaignsRepository struct {
	*CampaignStore
	*PurchaseStore
	*SaleStore
	*AnalyticsStore
	*FinanceStore
	*PricingStore
	*DHStore
	*SellSheetStore
}

func newTestPurchase(campaignID, certNumber string) *inventory.Purchase {
	now := time.Now().Truncate(time.Second)
	return &inventory.Purchase{
		ID:           "purch-" + certNumber,
		CampaignID:   campaignID,
		CardName:     "Charizard",
		CertNumber:   certNumber,
		Grader:       "PSA",
		GradeValue:   9.0,
		BuyCostCents: 80000,
		PurchaseDate: "2026-01-15",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func newTestSale(purchaseID string) *inventory.Sale {
	now := time.Now().Truncate(time.Second)
	return &inventory.Sale{
		ID:             "sale-" + purchaseID,
		PurchaseID:     purchaseID,
		SaleChannel:    inventory.SaleChannelEbay,
		SalePriceCents: 95000,
		SaleFeeCents:   11733,
		SaleDate:       "2026-02-01",
		DaysToSell:     17,
		NetProfitCents: 2967,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func createTestCampaign(t *testing.T, db *DB, id, name string) {
	t.Helper()
	now := time.Now().Truncate(time.Second)
	_, err := db.Exec(
		`INSERT INTO campaigns (id, name, phase, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		id, name, "pending", now, now,
	)
	require.NoError(t, err)
}
