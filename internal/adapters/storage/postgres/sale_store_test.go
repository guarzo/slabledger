package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// makeTestSale returns a minimal valid Sale for the given purchase.
func makeTestSale(purchaseID string) *inventory.Sale {
	id := fmt.Sprintf("sale-%d", testSaleIDCounter.Add(1))
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

func TestSetSaleIdempotencyKeyIfAbsent(t *testing.T) {
	db := setupTestDB(t)
	logger := mocks.NewMockLogger()
	ps := NewPurchaseStore(db.DB, logger)
	ss := NewSaleStore(db.DB, logger)
	ctx := context.Background()

	_, err := db.ExecContext(ctx,
		`INSERT INTO campaigns (id, name, phase, created_at, updated_at)
		 VALUES ('camp-1', 'Test Campaign', 'pending', NOW(), NOW())
		 ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		t.Fatalf("seed campaign: %v", err)
	}

	tests := []struct {
		name       string
		seedSale   bool // whether a sale row exists before the calls
		saleID     string
		calls      []string // successive keys passed to SetSaleIdempotencyKeyIfAbsent
		wantValues []string // expected return value per call
		wantErr    error
	}{
		{
			name:       "mints on first call, subsequent calls return the effective first key",
			seedSale:   true,
			calls:      []string{"key-one", "key-two"},
			wantValues: []string{"key-one", "key-one"},
		},
		{
			name:    "missing sale returns ErrSaleNotFound",
			saleID:  "sale-does-not-exist",
			calls:   []string{"some-key"},
			wantErr: inventory.ErrSaleNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saleID := tt.saleID
			if tt.seedSale {
				p := makeTestPurchase()
				if err := ps.CreatePurchase(ctx, p); err != nil {
					t.Fatalf("create purchase: %v", err)
				}
				s := makeTestSale(p.ID)
				if err := ss.CreateSale(ctx, s); err != nil {
					t.Fatalf("create sale: %v", err)
				}
				saleID = s.ID
			}

			for i, key := range tt.calls {
				got, err := ss.SetSaleIdempotencyKeyIfAbsent(ctx, saleID, key)
				if tt.wantErr != nil {
					if !errors.Is(err, tt.wantErr) {
						t.Fatalf("call %d: error = %v, want %v", i, err, tt.wantErr)
					}
					continue
				}
				if err != nil {
					t.Fatalf("call %d: SetSaleIdempotencyKeyIfAbsent: %v", i, err)
				}
				if got != tt.wantValues[i] {
					t.Fatalf("call %d = %q, want %q", i, got, tt.wantValues[i])
				}
			}
		})
	}
}

func TestSetSaleDHSaleID_Roundtrip(t *testing.T) {
	db := setupTestDB(t)
	logger := mocks.NewMockLogger()
	ps := NewPurchaseStore(db.DB, logger)
	ss := NewSaleStore(db.DB, logger)
	ctx := context.Background()

	_, err := db.ExecContext(ctx,
		`INSERT INTO campaigns (id, name, phase, created_at, updated_at)
		 VALUES ('camp-1', 'Test Campaign', 'pending', NOW(), NOW())
		 ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		t.Fatalf("seed campaign: %v", err)
	}

	p := makeTestPurchase()
	if err := ps.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("create purchase: %v", err)
	}
	s := makeTestSale(p.ID)
	if err := ss.CreateSale(ctx, s); err != nil {
		t.Fatalf("create sale: %v", err)
	}

	recordedAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := ss.SetSaleDHSaleID(ctx, s.ID, "dh-sale-123", recordedAt); err != nil {
		t.Fatalf("SetSaleDHSaleID: %v", err)
	}

	got, err := ss.GetSaleByPurchaseID(ctx, p.ID)
	if err != nil {
		t.Fatalf("get sale: %v", err)
	}
	if got.DHSaleID != "dh-sale-123" {
		t.Errorf("DHSaleID = %q, want %q", got.DHSaleID, "dh-sale-123")
	}
	if got.DHSaleRecordedAt == nil || !got.DHSaleRecordedAt.Equal(recordedAt) {
		t.Errorf("DHSaleRecordedAt = %v, want %v", got.DHSaleRecordedAt, recordedAt)
	}
}

func TestListSalesNeedingDHRecord(t *testing.T) {
	db := setupTestDB(t)
	logger := mocks.NewMockLogger()
	ps := NewPurchaseStore(db.DB, logger)
	ss := NewSaleStore(db.DB, logger)
	ctx := context.Background()

	_, err := db.ExecContext(ctx,
		`INSERT INTO campaigns (id, name, phase, created_at, updated_at)
		 VALUES ('camp-1', 'Test Campaign', 'pending', NOW(), NOW())
		 ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		t.Fatalf("seed campaign: %v", err)
	}

	tests := []struct {
		name string
		// setup seeds a purchase/sale pair and returns the sale ID plus
		// whether ListSalesNeedingDHRecord must include it.
		setup     func(t *testing.T) (saleID string, wantIncluded bool)
		wantCheck func(t *testing.T, found inventory.SaleNeedingDHRecord)
	}{
		{
			name: "eligible: blank dh_sale_id, dh_inventory_id set, blank conflict",
			setup: func(t *testing.T) (string, bool) {
				purchase := makeTestPurchase()
				purchase.DHInventoryID = 111
				purchase.PurchaseDate = "2026-02-02"
				if err := ps.CreatePurchase(ctx, purchase); err != nil {
					t.Fatalf("create eligible purchase: %v", err)
				}
				sale := makeTestSale(purchase.ID)
				if err := ss.CreateSale(ctx, sale); err != nil {
					t.Fatalf("create eligible sale: %v", err)
				}
				return sale.ID, true
			},
			wantCheck: func(t *testing.T, found inventory.SaleNeedingDHRecord) {
				if found.DHInventoryID != 111 {
					t.Errorf("DHInventoryID = %d, want 111", found.DHInventoryID)
				}
				if found.PurchaseDate != "2026-02-02" {
					t.Errorf("PurchaseDate = %q, want %q", found.PurchaseDate, "2026-02-02")
				}
			},
		},
		{
			name: "excluded: conflict flagged on the purchase",
			setup: func(t *testing.T) (string, bool) {
				purchase := makeTestPurchase()
				purchase.DHInventoryID = 222
				if err := ps.CreatePurchase(ctx, purchase); err != nil {
					t.Fatalf("create conflict purchase: %v", err)
				}
				sale := makeTestSale(purchase.ID)
				if err := ss.CreateSale(ctx, sale); err != nil {
					t.Fatalf("create conflict sale: %v", err)
				}
				if err := ps.SetDHSaleConflict(ctx, purchase.ID, "permanent error"); err != nil {
					t.Fatalf("SetDHSaleConflict: %v", err)
				}
				return sale.ID, false
			},
		},
		{
			name: "excluded: dh_sale_id already set",
			setup: func(t *testing.T) (string, bool) {
				purchase := makeTestPurchase()
				purchase.DHInventoryID = 333
				if err := ps.CreatePurchase(ctx, purchase); err != nil {
					t.Fatalf("create recorded purchase: %v", err)
				}
				sale := makeTestSale(purchase.ID)
				if err := ss.CreateSale(ctx, sale); err != nil {
					t.Fatalf("create recorded sale: %v", err)
				}
				if err := ss.SetSaleDHSaleID(ctx, sale.ID, "dh-sale-already", time.Now().UTC()); err != nil {
					t.Fatalf("SetSaleDHSaleID: %v", err)
				}
				return sale.ID, false
			},
		},
		{
			// A legacy sale: no idempotency key set (the zero value), matching the
			// 25 sales from the 2026-08-15 incident. Must still appear.
			name: "included: legacy sale with no idempotency key",
			setup: func(t *testing.T) (string, bool) {
				purchase := makeTestPurchase()
				purchase.DHInventoryID = 444
				if err := ps.CreatePurchase(ctx, purchase); err != nil {
					t.Fatalf("create purchase: %v", err)
				}
				sale := makeTestSale(purchase.ID)
				if err := ss.CreateSale(ctx, sale); err != nil {
					t.Fatalf("create sale: %v", err)
				}
				if sale.DHIdempotencyKey != "" {
					t.Fatalf("test fixture already has a key: %q", sale.DHIdempotencyKey)
				}
				return sale.ID, true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saleID, wantIncluded := tt.setup(t)

			got, err := ss.ListSalesNeedingDHRecord(ctx, 10)
			if err != nil {
				t.Fatalf("ListSalesNeedingDHRecord: %v", err)
			}

			var found *inventory.SaleNeedingDHRecord
			for i := range got {
				if got[i].ID == saleID {
					found = &got[i]
				}
			}
			switch {
			case wantIncluded && found == nil:
				t.Fatalf("sale %q not found in result, want included", saleID)
			case !wantIncluded && found != nil:
				t.Fatalf("sale %q found in result, want excluded", saleID)
			}
			if found != nil && tt.wantCheck != nil {
				tt.wantCheck(t, *found)
			}
		})
	}
}
