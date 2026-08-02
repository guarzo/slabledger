package postgres

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

var testSaleIDCounter atomic.Int64

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
		name              string
		saleReason        string
		clValueAtPurchase int
		forcedLiquidation bool
	}{
		{"forced by invoice pressure, cl-at-buy=5000", inventory.SaleReasonInvoicePressure, 5000, true},
		{"discretionary, cl-at-buy=0", inventory.SaleReasonDiscretionary, 0, false},
		{"aging policy, cl-at-buy=12000", inventory.SaleReasonAgingPolicy, 12000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// One counter value per case drives every generated identifier, so
			// IDs stay unique across cases without depending on wall-clock time.
			seq := testSaleIDCounter.Add(1)
			p := &inventory.Purchase{
				ID:                     fmt.Sprintf("analytics-rt-%d", seq),
				CampaignID:             "camp-analytics-rt",
				CardName:               "Charizard",
				CertNumber:             fmt.Sprintf("CERT-ART-%d", seq),
				Grader:                 "PSA",
				GradeValue:             10,
				BuyCostCents:           4000,
				PurchaseDate:           "2026-06-01",
				CLValueAtPurchaseCents: tt.clValueAtPurchase,
				CreatedAt:              time.Now().UTC(),
				UpdatedAt:              time.Now().UTC(),
			}
			if err := ps.CreatePurchase(ctx, p); err != nil {
				t.Fatalf("create purchase: %v", err)
			}

			sale := &inventory.Sale{
				ID:                 fmt.Sprintf("sale-art-%d", seq),
				PurchaseID:         p.ID,
				SaleChannel:        inventory.SaleChannelLocal,
				SalePriceCents:     5000,
				SaleFeeCents:       0,
				SaleDate:           "2026-06-15",
				DaysToSell:         14,
				NetProfitCents:     1000,
				ForcedLiquidation:  tt.forcedLiquidation,
				SaleReason:         tt.saleReason,
				CLValueAtSaleCents: tt.clValueAtPurchase,
				CreatedAt:          time.Now().UTC(),
				UpdatedAt:          time.Now().UTC(),
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
			if found.Sale.SaleReason != tt.saleReason {
				t.Errorf("SaleReason = %q, want %q", found.Sale.SaleReason, tt.saleReason)
			}
			if found.Sale.CLValueAtSaleCents != tt.clValueAtPurchase {
				t.Errorf("CLValueAtSaleCents = %d, want %d",
					found.Sale.CLValueAtSaleCents, tt.clValueAtPurchase)
			}
			if found.Sale.ChannelFeePctAtSale != nil {
				t.Errorf("ChannelFeePctAtSale = %v, want nil for a sale with no stored fee", *found.Sale.ChannelFeePctAtSale)
			}
		})
	}
}
