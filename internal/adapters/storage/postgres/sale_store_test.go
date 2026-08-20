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

func TestSetSaleIdempotencyKeyIfAbsent_MintsOnce(t *testing.T) {
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

	first, err := ss.SetSaleIdempotencyKeyIfAbsent(ctx, s.ID, "key-one")
	if err != nil {
		t.Fatalf("SetSaleIdempotencyKeyIfAbsent (first call): %v", err)
	}
	if first != "key-one" {
		t.Fatalf("first call = %q, want %q", first, "key-one")
	}

	second, err := ss.SetSaleIdempotencyKeyIfAbsent(ctx, s.ID, "key-two")
	if err != nil {
		t.Fatalf("SetSaleIdempotencyKeyIfAbsent (second call): %v", err)
	}
	if second != "key-one" {
		t.Fatalf("second call = %q, want %q (the effective, first-written key)", second, "key-one")
	}
}

func TestSetSaleIdempotencyKeyIfAbsent_SaleNotFound(t *testing.T) {
	db := setupTestDB(t)
	logger := mocks.NewMockLogger()
	ss := NewSaleStore(db.DB, logger)
	ctx := context.Background()

	_, err := ss.SetSaleIdempotencyKeyIfAbsent(ctx, "sale-does-not-exist", "some-key")
	if !errors.Is(err, inventory.ErrSaleNotFound) {
		t.Fatalf("SetSaleIdempotencyKeyIfAbsent for missing sale: got %v, want ErrSaleNotFound", err)
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

func TestListSalesNeedingDHRecord_Scoping(t *testing.T) {
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

	// (a) eligible: blank dh_sale_id, dh_inventory_id > 0, blank conflict.
	eligiblePurchase := makeTestPurchase()
	eligiblePurchase.DHInventoryID = 111
	eligiblePurchase.PurchaseDate = "2026-02-02"
	if err := ps.CreatePurchase(ctx, eligiblePurchase); err != nil {
		t.Fatalf("create eligible purchase: %v", err)
	}
	eligibleSale := makeTestSale(eligiblePurchase.ID)
	if err := ss.CreateSale(ctx, eligibleSale); err != nil {
		t.Fatalf("create eligible sale: %v", err)
	}

	// (b) excluded: conflict flagged on the purchase.
	conflictPurchase := makeTestPurchase()
	conflictPurchase.DHInventoryID = 222
	if err := ps.CreatePurchase(ctx, conflictPurchase); err != nil {
		t.Fatalf("create conflict purchase: %v", err)
	}
	conflictSale := makeTestSale(conflictPurchase.ID)
	if err := ss.CreateSale(ctx, conflictSale); err != nil {
		t.Fatalf("create conflict sale: %v", err)
	}
	if err := ps.SetDHSaleConflict(ctx, conflictPurchase.ID, "permanent error"); err != nil {
		t.Fatalf("SetDHSaleConflict: %v", err)
	}

	// (c) excluded: dh_sale_id already set.
	recordedPurchase := makeTestPurchase()
	recordedPurchase.DHInventoryID = 333
	if err := ps.CreatePurchase(ctx, recordedPurchase); err != nil {
		t.Fatalf("create recorded purchase: %v", err)
	}
	recordedSale := makeTestSale(recordedPurchase.ID)
	if err := ss.CreateSale(ctx, recordedSale); err != nil {
		t.Fatalf("create recorded sale: %v", err)
	}
	if err := ss.SetSaleDHSaleID(ctx, recordedSale.ID, "dh-sale-already", time.Now().UTC()); err != nil {
		t.Fatalf("SetSaleDHSaleID: %v", err)
	}

	got, err := ss.ListSalesNeedingDHRecord(ctx, 10)
	if err != nil {
		t.Fatalf("ListSalesNeedingDHRecord: %v", err)
	}

	var found *inventory.SaleNeedingDHRecord
	for i := range got {
		if got[i].ID == eligibleSale.ID {
			found = &got[i]
		}
		if got[i].ID == conflictSale.ID {
			t.Errorf("conflict-flagged sale %q must not appear", conflictSale.ID)
		}
		if got[i].ID == recordedSale.ID {
			t.Errorf("already-recorded sale %q must not appear", recordedSale.ID)
		}
	}
	if found == nil {
		t.Fatalf("eligible sale %q not found in result", eligibleSale.ID)
	}
	if found.DHInventoryID != 111 {
		t.Errorf("DHInventoryID = %d, want 111", found.DHInventoryID)
	}
	if found.PurchaseDate != "2026-02-02" {
		t.Errorf("PurchaseDate = %q, want %q", found.PurchaseDate, "2026-02-02")
	}
}

func TestListSalesNeedingDHRecord_IncludesLegacyKeylessSales(t *testing.T) {
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
	p.DHInventoryID = 444
	if err := ps.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("create purchase: %v", err)
	}
	// A legacy sale: no idempotency key set (the zero value), matching the
	// 25 sales from the 2026-08-15 incident.
	s := makeTestSale(p.ID)
	if err := ss.CreateSale(ctx, s); err != nil {
		t.Fatalf("create sale: %v", err)
	}
	if s.DHIdempotencyKey != "" {
		t.Fatalf("test fixture already has a key: %q", s.DHIdempotencyKey)
	}

	got, err := ss.ListSalesNeedingDHRecord(ctx, 10)
	if err != nil {
		t.Fatalf("ListSalesNeedingDHRecord: %v", err)
	}
	var found bool
	for _, r := range got {
		if r.ID == s.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("keyless legacy sale %q must appear in ListSalesNeedingDHRecord", s.ID)
	}
}
