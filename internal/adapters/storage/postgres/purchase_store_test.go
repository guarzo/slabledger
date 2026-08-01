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

func TestCreatePurchaseProvenanceRoundTrip(t *testing.T) {
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

	intPtr := func(v int) *int { return &v }
	floatPtr := func(v float64) *float64 { return &v }

	tests := []struct {
		name string
		set  func(p *inventory.Purchase)
	}{
		{
			name: "all provenance fields set",
			set: func(p *inventory.Purchase) {
				p.CLConfidenceAtPurchase = intPtr(85)
				p.PopulationAtPurchase = intPtr(120)
				p.DHConfidenceAtPurchase = floatPtr(0.92)
				p.SourceCountAtPurchase = intPtr(4)
				p.ActiveListingsAtPurchase = intPtr(7)
				p.SalesLast30dAtPurchase = intPtr(3)
			},
		},
		{
			name: "all provenance fields nil",
			set:  func(p *inventory.Purchase) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := makeTestPurchase()
			tt.set(p)
			if err := ps.CreatePurchase(ctx, p); err != nil {
				t.Fatalf("create: %v", err)
			}

			got, err := ps.GetPurchase(ctx, p.ID)
			if err != nil {
				t.Fatalf("get: %v", err)
			}

			assertIntPtrEqual(t, "CLConfidenceAtPurchase", p.CLConfidenceAtPurchase, got.CLConfidenceAtPurchase)
			assertIntPtrEqual(t, "PopulationAtPurchase", p.PopulationAtPurchase, got.PopulationAtPurchase)
			assertFloatPtrEqual(t, "DHConfidenceAtPurchase", p.DHConfidenceAtPurchase, got.DHConfidenceAtPurchase)
			assertIntPtrEqual(t, "SourceCountAtPurchase", p.SourceCountAtPurchase, got.SourceCountAtPurchase)
			assertIntPtrEqual(t, "ActiveListingsAtPurchase", p.ActiveListingsAtPurchase, got.ActiveListingsAtPurchase)
			assertIntPtrEqual(t, "SalesLast30dAtPurchase", p.SalesLast30dAtPurchase, got.SalesLast30dAtPurchase)
		})
	}
}

func assertIntPtrEqual(t *testing.T, field string, want, got *int) {
	t.Helper()
	if (want == nil) != (got == nil) {
		t.Errorf("%s: want nil=%v, got nil=%v", field, want == nil, got == nil)
		return
	}
	if want != nil && *want != *got {
		t.Errorf("%s = %d, want %d", field, *got, *want)
	}
}

func assertFloatPtrEqual(t *testing.T, field string, want, got *float64) {
	t.Helper()
	if (want == nil) != (got == nil) {
		t.Errorf("%s: want nil=%v, got nil=%v", field, want == nil, got == nil)
		return
	}
	if want != nil && *want != *got {
		t.Errorf("%s = %v, want %v", field, *got, *want)
	}
}

func TestUpdatePurchaseMarketSnapshotProvenanceSetOnce(t *testing.T) {
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

	t.Run("set-once freeze on first enrichment, unchanged on second", func(t *testing.T) {
		p := makeTestPurchase()
		if err := ps.CreatePurchase(ctx, p); err != nil {
			t.Fatalf("create: %v", err)
		}

		first := inventory.MarketSnapshotData{
			Confidence:         0.9,
			SourceCountRaw:     2,
			MarketDataObserved: true,
			ActiveListings:     3,
			SalesLast30d:       5,
		}
		if err := ps.UpdatePurchaseMarketSnapshot(ctx, p.ID, first); err != nil {
			t.Fatalf("first update: %v", err)
		}

		got, err := ps.GetPurchase(ctx, p.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		assertFloatPtrEqual(t, "DHConfidenceAtPurchase", floatPtr(0.9), got.DHConfidenceAtPurchase)
		assertIntPtrEqual(t, "SourceCountAtPurchase", intPtr(2), got.SourceCountAtPurchase)
		assertIntPtrEqual(t, "ActiveListingsAtPurchase", intPtr(3), got.ActiveListingsAtPurchase)
		assertIntPtrEqual(t, "SalesLast30dAtPurchase", intPtr(5), got.SalesLast30dAtPurchase)

		// Second enrichment with different values must not overwrite (set-once).
		second := inventory.MarketSnapshotData{
			Confidence:         0.1,
			SourceCountRaw:     99,
			MarketDataObserved: true,
			ActiveListings:     100,
			SalesLast30d:       200,
		}
		if err := ps.UpdatePurchaseMarketSnapshot(ctx, p.ID, second); err != nil {
			t.Fatalf("second update: %v", err)
		}

		got2, err := ps.GetPurchase(ctx, p.ID)
		if err != nil {
			t.Fatalf("get2: %v", err)
		}
		assertFloatPtrEqual(t, "DHConfidenceAtPurchase", floatPtr(0.9), got2.DHConfidenceAtPurchase)
		assertIntPtrEqual(t, "SourceCountAtPurchase", intPtr(2), got2.SourceCountAtPurchase)
		assertIntPtrEqual(t, "ActiveListingsAtPurchase", intPtr(3), got2.ActiveListingsAtPurchase)
		assertIntPtrEqual(t, "SalesLast30dAtPurchase", intPtr(5), got2.SalesLast30dAtPurchase)
	})

	t.Run("market data not observed leaves market provenance nil but still freezes confidence/source count", func(t *testing.T) {
		p := makeTestPurchase()
		if err := ps.CreatePurchase(ctx, p); err != nil {
			t.Fatalf("create: %v", err)
		}

		snap := inventory.MarketSnapshotData{
			Confidence:         0.75,
			SourceCountRaw:     6,
			MarketDataObserved: false,
			ActiveListings:     42, // must be ignored since not observed
			SalesLast30d:       43,
		}
		if err := ps.UpdatePurchaseMarketSnapshot(ctx, p.ID, snap); err != nil {
			t.Fatalf("update: %v", err)
		}

		got, err := ps.GetPurchase(ctx, p.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		assertFloatPtrEqual(t, "DHConfidenceAtPurchase", floatPtr(0.75), got.DHConfidenceAtPurchase)
		assertIntPtrEqual(t, "SourceCountAtPurchase", intPtr(6), got.SourceCountAtPurchase)
		if got.ActiveListingsAtPurchase != nil {
			t.Errorf("ActiveListingsAtPurchase = %v, want nil (market data not observed)", *got.ActiveListingsAtPurchase)
		}
		if got.SalesLast30dAtPurchase != nil {
			t.Errorf("SalesLast30dAtPurchase = %v, want nil (market data not observed)", *got.SalesLast30dAtPurchase)
		}
	})
}

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }
