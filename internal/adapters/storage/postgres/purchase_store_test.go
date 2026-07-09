package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// makeTestPurchase returns a minimal valid Purchase for use in store tests.
// Each call generates a unique cert/id based on the current nanosecond to avoid
// unique-constraint collisions when multiple sub-tests run in the same Postgres session.
func makeTestPurchase() *inventory.Purchase {
	id := "test-" + time.Now().Format("150405.000000000")
	return &inventory.Purchase{
		ID:           id,
		CampaignID:   "camp-1",
		CardName:     "Charizard",
		CertNumber:   "CERT-" + id,
		Grader:       "PSA",
		GradeValue:   10,
		BuyCostCents: 5000,
		PurchaseDate: "2026-01-01",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
}

func TestCLValueAtPurchaseSetOnce(t *testing.T) {
	db := setupTestDB(t)
	logger := mocks.NewMockLogger()
	ps := NewPurchaseStore(db.DB, logger)
	ctx := context.Background()

	// Seed the campaign required by the foreign-key constraint.
	_, err := db.ExecContext(ctx,
		`INSERT INTO campaigns (id, name, phase, created_at, updated_at)
		 VALUES ('camp-1', 'Test Campaign', 'pending', NOW(), NOW())
		 ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		t.Fatalf("seed campaign: %v", err)
	}

	tests := []struct {
		name      string
		createCL  int // CLValueCents at creation
		updates   []int
		wantAtBuy int
	}{
		{"snapshot at creation", 1000, []int{800, 600}, 1000},
		{"snapshot at first enrichment", 0, []int{500, 300}, 500},
		{"never enriched", 0, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := makeTestPurchase()
			p.CLValueCents = tt.createCL
			if err := ps.CreatePurchase(ctx, p); err != nil {
				t.Fatalf("create: %v", err)
			}
			for _, cl := range tt.updates {
				if err := ps.UpdatePurchaseCLValue(ctx, p.ID, cl, 10); err != nil {
					t.Fatalf("update: %v", err)
				}
			}
			got, err := ps.GetPurchase(ctx, p.ID)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if got.CLValueAtPurchaseCents != tt.wantAtBuy {
				t.Errorf("CLValueAtPurchaseCents = %d, want %d", got.CLValueAtPurchaseCents, tt.wantAtBuy)
			}
		})
	}
}
