package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// makeTestSale returns a minimal valid Sale for the given purchase.
func makeTestSale(purchaseID string) *inventory.Sale {
	id := "sale-" + time.Now().Format("150405.000000000")
	return &inventory.Sale{
		ID:             id,
		PurchaseID:     purchaseID,
		SaleChannel:    inventory.SaleChannelLocal,
		SalePriceCents: 6000,
		SaleFeeCents:   0,
		SaleDate:       "2026-07-01",
		DaysToSell:     180,
		NetProfitCents: 1000,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
}

func TestSaleForcedLiquidationRoundtrip(t *testing.T) {
	db := setupTestDB(t)
	logger := mocks.NewMockLogger()
	ps := NewPurchaseStore(db.DB, logger)
	ss := NewSaleStore(db.DB, logger)
	ctx := context.Background()

	// Seed the campaign required by the foreign-key constraint.
	_, err := db.ExecContext(ctx,
		`INSERT INTO campaigns (id, name, phase, created_at, updated_at)
		 VALUES ('camp-sale-fl', 'Sale FL Campaign', 'pending', NOW(), NOW())
		 ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		t.Fatalf("seed campaign: %v", err)
	}

	tests := []struct {
		name              string
		forcedLiquidation bool
	}{
		{"forced liquidation true", true},
		{"forced liquidation false (default)", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a purchase to satisfy the FK.
			p := makeTestPurchase()
			p.CampaignID = "camp-sale-fl"
			if err := ps.CreatePurchase(ctx, p); err != nil {
				t.Fatalf("create purchase: %v", err)
			}

			// Create a sale with the desired ForcedLiquidation value.
			sale := makeTestSale(p.ID)
			sale.ForcedLiquidation = tt.forcedLiquidation
			if err := ss.CreateSale(ctx, sale); err != nil {
				t.Fatalf("create sale: %v", err)
			}

			// Read back via GetSaleByPurchaseID and verify.
			got, err := ss.GetSaleByPurchaseID(ctx, p.ID)
			if err != nil {
				t.Fatalf("get sale: %v", err)
			}
			if got.ForcedLiquidation != tt.forcedLiquidation {
				t.Errorf("ForcedLiquidation = %v, want %v", got.ForcedLiquidation, tt.forcedLiquidation)
			}
		})
	}
}
