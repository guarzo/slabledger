package sqlite

import (
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
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

func newTestPurchase(campaignID, certNumber string) *campaigns.Purchase {
	now := time.Now().Truncate(time.Second)
	return &campaigns.Purchase{
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

func createTestCampaign(t *testing.T, db *DB, id, name string) {
	t.Helper()
	now := time.Now().Truncate(time.Second)
	_, err := db.Exec(
		`INSERT INTO campaigns (id, name, phase, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		id, name, "pending", now, now,
	)
	require.NoError(t, err)
}
