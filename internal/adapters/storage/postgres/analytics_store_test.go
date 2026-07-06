package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// TestGetAllPurchasesWithSalesFieldRoundtrip verifies that forced_liquidation and
// cl_value_at_purchase_cents flow correctly through GetAllPurchasesWithSales.
// These fields are critical for the portfolio analysis split-P&L computation.
func TestGetAllPurchasesWithSalesFieldRoundtrip(t *testing.T) {
	db := setupTestDB(t)
	logger := mocks.NewMockLogger()
	ps := NewPurchaseStore(db.DB, logger)
	ss := NewSaleStore(db.DB, logger)
	as := NewAnalyticsStore(db.DB, logger)
	ctx := context.Background()

	// Seed campaign required by FK constraint.
	_, err := db.ExecContext(ctx,
		`INSERT INTO campaigns (id, name, phase, created_at, updated_at)
		 VALUES ('camp-analytics-rt', 'Analytics RT Campaign', 'pending', NOW(), NOW())
		 ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		t.Fatalf("seed campaign: %v", err)
	}

	tests := []struct {
		name               string
		clValueAtPurchase  int
		forcedLiquidation  bool
	}{
		{"forced=true, cl-at-buy=5000", 5000, true},
		{"forced=false, cl-at-buy=0", 0, false},
		{"forced=false, cl-at-buy=12000", 12000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &inventory.Purchase{
				ID:                    "analytics-rt-" + time.Now().Format("150405.000000000"),
				CampaignID:            "camp-analytics-rt",
				CardName:              "Charizard",
				CertNumber:            "CERT-ART-" + time.Now().Format("150405.000000000"),
				Grader:                "PSA",
				GradeValue:            10,
				BuyCostCents:          4000,
				PurchaseDate:          "2026-06-01",
				CLValueAtPurchaseCents: tt.clValueAtPurchase,
				CreatedAt:             time.Now().UTC(),
				UpdatedAt:             time.Now().UTC(),
			}
			if err := ps.CreatePurchase(ctx, p); err != nil {
				t.Fatalf("create purchase: %v", err)
			}

			sale := &inventory.Sale{
				ID:                "sale-art-" + time.Now().Format("150405.000000000"),
				PurchaseID:        p.ID,
				SaleChannel:       inventory.SaleChannelLocal,
				SalePriceCents:    5000,
				SaleFeeCents:      0,
				SaleDate:          "2026-06-15",
				DaysToSell:        14,
				NetProfitCents:    1000,
				ForcedLiquidation: tt.forcedLiquidation,
				CreatedAt:         time.Now().UTC(),
				UpdatedAt:         time.Now().UTC(),
			}
			if err := ss.CreateSale(ctx, sale); err != nil {
				t.Fatalf("create sale: %v", err)
			}

			rows, err := as.GetAllPurchasesWithSales(ctx, inventory.WithExcludeExternal())
			if err != nil {
				t.Fatalf("GetAllPurchasesWithSales: %v", err)
			}

			var found *inventory.PurchaseWithSale
			for i := range rows {
				if rows[i].Purchase.ID == p.ID {
					found = &rows[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("purchase %s not found in GetAllPurchasesWithSales result", p.ID)
			}

			if found.Purchase.CLValueAtPurchaseCents != tt.clValueAtPurchase {
				t.Errorf("CLValueAtPurchaseCents = %d, want %d",
					found.Purchase.CLValueAtPurchaseCents, tt.clValueAtPurchase)
			}

			if found.Sale == nil {
				t.Fatal("expected sale to be present, got nil")
			}
			if found.Sale.ForcedLiquidation != tt.forcedLiquidation {
				t.Errorf("ForcedLiquidation = %v, want %v",
					found.Sale.ForcedLiquidation, tt.forcedLiquidation)
			}
		})
	}
}
