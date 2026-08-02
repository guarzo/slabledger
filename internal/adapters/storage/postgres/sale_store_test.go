package postgres

import (
	"context"
	"errors"
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

func TestSaleProvenanceRoundtrip(t *testing.T) {
	db := setupTestDB(t)
	logger := mocks.NewMockLogger()
	ps := NewPurchaseStore(db.DB, logger)
	ss := NewSaleStore(db.DB, logger)
	ctx := context.Background()

	_, err := db.ExecContext(ctx,
		`INSERT INTO campaigns (id, name, phase, created_at, updated_at)
		 VALUES ('camp-sale-prov', 'Sale Provenance Campaign', 'pending', NOW(), NOW())
		 ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		t.Fatalf("seed campaign: %v", err)
	}

	p := makeTestPurchase()
	p.CampaignID = "camp-sale-prov"
	if err := ps.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("create purchase: %v", err)
	}

	feePct := 0.10
	sale := makeTestSale(p.ID)
	sale.SaleReason = "invoice_pressure"
	sale.CLValueAtSaleCents = 9000
	sale.ChannelFeePctAtSale = &feePct
	sale.ForcedLiquidation = true
	if err := ss.CreateSale(ctx, sale); err != nil {
		t.Fatalf("create sale: %v", err)
	}

	got, err := ss.GetSaleByPurchaseID(ctx, p.ID)
	if err != nil {
		t.Fatalf("get sale: %v", err)
	}
	if got.SaleReason != "invoice_pressure" {
		t.Errorf("SaleReason = %q, want %q", got.SaleReason, "invoice_pressure")
	}
	if got.CLValueAtSaleCents != 9000 {
		t.Errorf("CLValueAtSaleCents = %d, want 9000", got.CLValueAtSaleCents)
	}
	if got.ChannelFeePctAtSale == nil || *got.ChannelFeePctAtSale != 0.10 {
		t.Errorf("ChannelFeePctAtSale = %v, want 0.10", got.ChannelFeePctAtSale)
	}
	if !got.ForcedLiquidation {
		t.Errorf("ForcedLiquidation = %v, want true", got.ForcedLiquidation)
	}
}

func TestSaleStoreUpdateSaleReason(t *testing.T) {
	db := setupTestDB(t)
	logger := mocks.NewMockLogger()
	ps := NewPurchaseStore(db.DB, logger)
	ss := NewSaleStore(db.DB, logger)
	ctx := context.Background()

	_, err := db.ExecContext(ctx,
		`INSERT INTO campaigns (id, name, phase, created_at, updated_at)
		 VALUES ('camp-sale-reason-a', 'Sale Reason Campaign A', 'pending', NOW(), NOW()),
			('camp-sale-reason-b', 'Sale Reason Campaign B', 'pending', NOW(), NOW())
		 ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		t.Fatalf("seed campaigns: %v", err)
	}

	tests := []struct {
		name           string
		updateCampaign string // campaign ID passed to UpdateSaleReason
		wantErr        error
	}{
		{
			name:           "wrong campaign scoping returns ErrSaleNotFound",
			updateCampaign: "camp-sale-reason-b",
			wantErr:        inventory.ErrSaleNotFound,
		},
		{
			name:           "correct campaign updates the reason and flips forced_liquidation to false",
			updateCampaign: "camp-sale-reason-a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := makeTestPurchase()
			p.CampaignID = "camp-sale-reason-a"
			if err := ps.CreatePurchase(ctx, p); err != nil {
				t.Fatalf("create purchase: %v", err)
			}

			sale := makeTestSale(p.ID)
			sale.SaleReason = "invoice_pressure"
			sale.ForcedLiquidation = true
			if err := ss.CreateSale(ctx, sale); err != nil {
				t.Fatalf("create sale: %v", err)
			}

			err := ss.UpdateSaleReason(ctx, tt.updateCampaign, sale.ID, "aging_policy", false)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("UpdateSaleReason error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateSaleReason: %v", err)
			}

			got, err := ss.GetSaleByPurchaseID(ctx, p.ID)
			if err != nil {
				t.Fatalf("get sale: %v", err)
			}
			if got.SaleReason != "aging_policy" {
				t.Errorf("SaleReason = %q, want %q", got.SaleReason, "aging_policy")
			}
			if got.ForcedLiquidation {
				t.Errorf("ForcedLiquidation = true, want false after non-invoice_pressure reason")
			}
			if got.ChannelFeePctAtSale != nil {
				t.Errorf("ChannelFeePctAtSale = %v, want nil for a sale with no stored fee", *got.ChannelFeePctAtSale)
			}
		})
	}
}
