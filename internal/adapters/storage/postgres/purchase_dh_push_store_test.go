package postgres

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestResetDHFieldsForRelistAfterVoid_PreservesInventoryID(t *testing.T) {
	db := setupTestDB(t)
	logger := mocks.NewMockLogger()
	ps := NewPurchaseStore(db.DB, logger)
	ctx := context.Background()

	_, err := db.ExecContext(ctx,
		`INSERT INTO campaigns (id, name, phase, created_at, updated_at)
		 VALUES ('camp-1', 'Test Campaign', 'pending', NOW(), NOW())
		 ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		t.Fatalf("seed campaign: %v", err)
	}

	p := makeTestPurchase()
	p.DHInventoryID = 555
	p.DHPushStatus = inventory.DHPushStatusMatched
	p.DHStatus = "sold"
	p.DHChannelsJSON = `["ebay"]`
	if err := ps.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("create purchase: %v", err)
	}

	if err := ps.ResetDHFieldsForRelistAfterVoid(ctx, p.ID); err != nil {
		t.Fatalf("ResetDHFieldsForRelistAfterVoid: %v", err)
	}

	got, err := ps.GetPurchase(ctx, p.ID)
	if err != nil {
		t.Fatalf("get purchase: %v", err)
	}
	if got.DHInventoryID != 555 {
		t.Errorf("DHInventoryID = %d, want 555 (preserved)", got.DHInventoryID)
	}
	if got.DHPushStatus != inventory.DHPushStatusPending {
		t.Errorf("DHPushStatus = %q, want %q", got.DHPushStatus, inventory.DHPushStatusPending)
	}
	if got.DHStatus != inventory.DHStatusInStock {
		t.Errorf("DHStatus = %q, want %q", got.DHStatus, inventory.DHStatusInStock)
	}
	if got.DHChannelsJSON != "[]" {
		t.Errorf("DHChannelsJSON = %q, want %q", got.DHChannelsJSON, "[]")
	}
	if got.DHUnlistedDetectedAt == nil {
		t.Errorf("DHUnlistedDetectedAt = nil, want non-nil")
	}
}

func TestSetDHSaleConflict_ClearDHSaleConflict_Roundtrip(t *testing.T) {
	db := setupTestDB(t)
	logger := mocks.NewMockLogger()
	ps := NewPurchaseStore(db.DB, logger)
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

	if err := ps.SetDHSaleConflict(ctx, p.ID, "idempotency key reused"); err != nil {
		t.Fatalf("SetDHSaleConflict: %v", err)
	}
	got, err := ps.GetPurchase(ctx, p.ID)
	if err != nil {
		t.Fatalf("get purchase after set: %v", err)
	}
	if got.DHSaleConflict != "idempotency key reused" {
		t.Errorf("DHSaleConflict = %q, want %q", got.DHSaleConflict, "idempotency key reused")
	}
	if got.DHSaleConflictAt == nil {
		t.Errorf("DHSaleConflictAt = nil, want non-nil after SetDHSaleConflict")
	}

	if err := ps.ClearDHSaleConflict(ctx, p.ID); err != nil {
		t.Fatalf("ClearDHSaleConflict: %v", err)
	}
	got, err = ps.GetPurchase(ctx, p.ID)
	if err != nil {
		t.Fatalf("get purchase after clear: %v", err)
	}
	if got.DHSaleConflict != "" {
		t.Errorf("DHSaleConflict = %q, want \"\" after clear", got.DHSaleConflict)
	}
	if got.DHSaleConflictAt != nil {
		t.Errorf("DHSaleConflictAt = %v, want nil after clear", got.DHSaleConflictAt)
	}
}
