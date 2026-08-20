package postgres

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestResetDHFieldsForRelistAfterVoid(t *testing.T) {
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
	p.DHPushAttempts = 3
	p.DHListingPriceCents = 1999
	p.DHHoldReason = "stale hold"
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

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"DHInventoryID preserved", got.DHInventoryID, 555},
		{"DHPushStatus reset to pending", got.DHPushStatus, inventory.DHPushStatusPending},
		{"DHStatus set to in_stock", got.DHStatus, inventory.DHStatusInStock},
		{"DHChannelsJSON cleared", got.DHChannelsJSON, "[]"},
		{"DHPushAttempts reset", got.DHPushAttempts, 0},
		{"DHListingPriceCents reset", got.DHListingPriceCents, 0},
		{"DHHoldReason cleared", got.DHHoldReason, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
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

	steps := []struct {
		name         string
		action       func() error
		wantConflict string
		wantAtIsNil  bool
	}{
		{
			name:         "after SetDHSaleConflict",
			action:       func() error { return ps.SetDHSaleConflict(ctx, p.ID, "idempotency key reused") },
			wantConflict: "idempotency key reused",
			wantAtIsNil:  false,
		},
		{
			name:         "after ClearDHSaleConflict",
			action:       func() error { return ps.ClearDHSaleConflict(ctx, p.ID) },
			wantConflict: "",
			wantAtIsNil:  true,
		},
	}

	for _, tt := range steps {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.action(); err != nil {
				t.Fatalf("action: %v", err)
			}
			got, err := ps.GetPurchase(ctx, p.ID)
			if err != nil {
				t.Fatalf("get purchase: %v", err)
			}
			if got.DHSaleConflict != tt.wantConflict {
				t.Errorf("DHSaleConflict = %q, want %q", got.DHSaleConflict, tt.wantConflict)
			}
			if tt.wantAtIsNil && got.DHSaleConflictAt != nil {
				t.Errorf("DHSaleConflictAt = %v, want nil", got.DHSaleConflictAt)
			}
			if !tt.wantAtIsNil && got.DHSaleConflictAt == nil {
				t.Errorf("DHSaleConflictAt = nil, want non-nil")
			}
		})
	}
}
