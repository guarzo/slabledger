package postgres

import (
	"context"
	"errors"
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
			// All three subtests get a fresh purchase_date, not just "snapshot at
			// first enrichment": that one strictly needs it (createCL is 0, so the
			// create-time freeze never fires there and it depends entirely on
			// UpdatePurchaseCLValue's own write-time lateness guard, D4, which
			// requires purchase_date within clFreezeMaxAgeDays of today) --
			// makeTestPurchase()'s fixed "2026-01-01" default is stale by
			// construction and would fail it. "snapshot at creation" doesn't need
			// this to pass, but a fresh date is still correct there: it keeps that
			// subtest from accidentally passing "for the wrong reason" if the
			// create-time freeze is ever changed to also consult recency.
			p.PurchaseDate = time.Now().UTC().Format("2006-01-02")
			if err := ps.CreatePurchase(ctx, p); err != nil {
				t.Fatalf("create: %v", err)
			}
			for _, cl := range tt.updates {
				if err := ps.UpdatePurchaseCLValue(ctx, p.ID, cl, 10, nil); err != nil {
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

	tests := []struct {
		name string
		set  func(p *inventory.Purchase)
	}{
		{
			name: "all provenance fields set",
			set: func(p *inventory.Purchase) {
				// 42 and 120 are deliberately distinct: these two columns are
				// adjacent in the INSERT's parameter list, so equal values
				// would let a swapped binding pass unnoticed.
				p.CLPolicyConfidenceMinAtPurchase = intPtr(42)
				p.PopulationAtPurchase = intPtr(120)
				p.DHConfidenceAtPurchase = floatPtr(0.92)
				p.SourceCountAtPurchase = intPtr(4)
				p.ActiveListingsAtPurchase = intPtr(7)
				p.SalesLast30dAtPurchase = intPtr(3)
				// 0.78 is deliberately distinct from DHConfidenceAtPurchase's
				// 0.92 above: both are DOUBLE PRECISION and adjacent-ish in the
				// INSERT list, so equal values would hide a swapped binding.
				p.BuyTermsCLPctAtPurchase = floatPtr(0.78)
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

			assertIntPtrEqual(t, "CLPolicyConfidenceMinAtPurchase", p.CLPolicyConfidenceMinAtPurchase, got.CLPolicyConfidenceMinAtPurchase)
			assertIntPtrEqual(t, "PopulationAtPurchase", p.PopulationAtPurchase, got.PopulationAtPurchase)
			assertFloatPtrEqual(t, "DHConfidenceAtPurchase", p.DHConfidenceAtPurchase, got.DHConfidenceAtPurchase)
			assertIntPtrEqual(t, "SourceCountAtPurchase", p.SourceCountAtPurchase, got.SourceCountAtPurchase)
			assertIntPtrEqual(t, "ActiveListingsAtPurchase", p.ActiveListingsAtPurchase, got.ActiveListingsAtPurchase)
			assertIntPtrEqual(t, "SalesLast30dAtPurchase", p.SalesLast30dAtPurchase, got.SalesLast30dAtPurchase)
			assertFloatPtrEqual(t, "BuyTermsCLPctAtPurchase", p.BuyTermsCLPctAtPurchase, got.BuyTermsCLPctAtPurchase)
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

// newStoreWithUnsoldPurchase seeds a campaign and a single unsold purchase,
// returning the store and the purchase's ID.
func newStoreWithUnsoldPurchase(t *testing.T) (*PurchaseStore, string) {
	t.Helper()
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
	_, err = db.ExecContext(ctx,
		`INSERT INTO campaigns (id, name, phase, created_at, updated_at)
		 VALUES ('campaign-b', 'Campaign B', 'pending', NOW(), NOW())
		 ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		t.Fatalf("seed campaign-b: %v", err)
	}

	p := makeTestPurchase()
	p.CLPolicyConfidenceMinAtPurchase = intPtr(37)
	if err := ps.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}
	return ps, p.ID
}

// newStoreWithSoldPurchase seeds a campaign, a purchase, and a linked sale,
// returning the store and the purchase's ID.
func newStoreWithSoldPurchase(t *testing.T) (*PurchaseStore, string) {
	t.Helper()
	ps, purchaseID := newStoreWithUnsoldPurchase(t)
	ctx := context.Background()

	_, err := ps.db.ExecContext(ctx,
		`INSERT INTO campaign_sales (id, purchase_id, sale_channel, sale_price_cents, sale_date, forced_liquidation)
		 VALUES ($1, $2, 'ebay', 9000, '2026-01-15', FALSE)`,
		"sale-"+purchaseID, purchaseID,
	)
	if err != nil {
		t.Fatalf("seed sale: %v", err)
	}
	return ps, purchaseID
}

// mustGetPurchase fetches a purchase by ID, failing the test on error.
func mustGetPurchase(t *testing.T, ps *PurchaseStore, purchaseID string) *inventory.Purchase {
	t.Helper()
	got, err := ps.GetPurchase(context.Background(), purchaseID)
	if err != nil {
		t.Fatalf("GetPurchase: %v", err)
	}
	return got
}

func TestReattributePurchase_RefusesWhenSaleExists(t *testing.T) {
	ps, purchaseID := newStoreWithSoldPurchase(t)
	err := ps.ReattributePurchase(context.Background(), purchaseID, inventory.Reattribution{
		CampaignID:          "campaign-b",
		PSACampaignName:     "Modern High Band",
		PSASourcingFeeCents: 300,
	})
	if !errors.Is(err, inventory.ErrPurchaseHasSale) {
		t.Fatalf("err = %v, want ErrPurchaseHasSale", err)
	}
}

func TestReattributePurchase_NullsCLConfidenceWhenNil(t *testing.T) {
	ps, purchaseID := newStoreWithUnsoldPurchase(t)
	err := ps.ReattributePurchase(context.Background(), purchaseID, inventory.Reattribution{
		CampaignID:                      "campaign-b",
		PSACampaignName:                 "Modern",
		PSASourcingFeeCents:             300,
		CLPolicyConfidenceMinAtPurchase: nil,
	})
	if err != nil {
		t.Fatalf("ReattributePurchase: %v", err)
	}
	got := mustGetPurchase(t, ps, purchaseID)
	// The seed row set this to 37, so nil here proves the UPDATE actually
	// wrote the column rather than leaving it untouched.
	if got.CLPolicyConfidenceMinAtPurchase != nil {
		t.Errorf("CLPolicyConfidenceMinAtPurchase = %v, want nil", *got.CLPolicyConfidenceMinAtPurchase)
	}
	if got.AttributionSource != inventory.AttributionSourcePSA {
		t.Errorf("AttributionSource = %q, want %q", got.AttributionSource, inventory.AttributionSourcePSA)
	}
}

func TestReattributePurchase_WritesConfidenceColumn(t *testing.T) {
	ps, purchaseID := newStoreWithUnsoldPurchase(t)
	err := ps.ReattributePurchase(context.Background(), purchaseID, inventory.Reattribution{
		CampaignID:                      "campaign-b",
		PSACampaignName:                 "Modern",
		PSASourcingFeeCents:             300,
		CLPolicyConfidenceMinAtPurchase: intPtr(24),
	})
	if err != nil {
		t.Fatalf("ReattributePurchase: %v", err)
	}
	got := mustGetPurchase(t, ps, purchaseID)
	assertIntPtrEqual(t, "CLPolicyConfidenceMinAtPurchase", intPtr(24), got.CLPolicyConfidenceMinAtPurchase)
}

func TestUpdatePurchaseCampaign_SetsManualAttribution(t *testing.T) {
	ps, purchaseID := newStoreWithUnsoldPurchase(t)
	ctx := context.Background()

	before := mustGetPurchase(t, ps, purchaseID)
	if before.AttributionSource != inventory.AttributionSourceInferred {
		t.Fatalf("precondition: AttributionSource = %q, want %q", before.AttributionSource, inventory.AttributionSourceInferred)
	}

	if err := ps.UpdatePurchaseCampaign(ctx, purchaseID, "campaign-b", 300); err != nil {
		t.Fatalf("UpdatePurchaseCampaign: %v", err)
	}

	got := mustGetPurchase(t, ps, purchaseID)
	if got.CampaignID != "campaign-b" {
		t.Errorf("CampaignID = %q, want %q", got.CampaignID, "campaign-b")
	}
	if got.PSASourcingFeeCents != 300 {
		t.Errorf("PSASourcingFeeCents = %d, want 300", got.PSASourcingFeeCents)
	}
	if got.AttributionSource != inventory.AttributionSourceManual {
		t.Errorf("AttributionSource = %q, want %q", got.AttributionSource, inventory.AttributionSourceManual)
	}
}

func TestUpdatePurchaseCampaign_RefusesWhenSaleExists(t *testing.T) {
	ps, purchaseID := newStoreWithSoldPurchase(t)
	ctx := context.Background()

	err := ps.UpdatePurchaseCampaign(ctx, purchaseID, "campaign-b", 300)
	if !errors.Is(err, inventory.ErrPurchaseHasSale) {
		t.Fatalf("err = %v, want ErrPurchaseHasSale", err)
	}

	got := mustGetPurchase(t, ps, purchaseID)
	if got.CampaignID != "camp-1" {
		t.Errorf("CampaignID = %q, want unchanged %q", got.CampaignID, "camp-1")
	}
	if got.AttributionSource == inventory.AttributionSourceManual {
		t.Errorf("AttributionSource = %q, want unchanged (not manual) since the guard should have blocked the update", got.AttributionSource)
	}
}

func TestUpdatePurchaseAttributionName(t *testing.T) {
	ps, purchaseID := newStoreWithUnsoldPurchase(t)
	err := ps.UpdatePurchaseAttributionName(context.Background(), purchaseID, "Modern High Band", inventory.AttributionSourcePSA)
	if err != nil {
		t.Fatalf("UpdatePurchaseAttributionName: %v", err)
	}
	got := mustGetPurchase(t, ps, purchaseID)
	if got.PSACampaignName != "Modern High Band" {
		t.Errorf("PSACampaignName = %q, want %q", got.PSACampaignName, "Modern High Band")
	}
	if got.AttributionSource != inventory.AttributionSourcePSA {
		t.Errorf("AttributionSource = %q, want %q", got.AttributionSource, inventory.AttributionSourcePSA)
	}
}

func TestPurchaseCLProvenanceColumnsRoundTrip(t *testing.T) {
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
	p.CLValueAtPurchaseCents = 5000
	p.CLValueAtPurchaseObservedAt = "2026-01-01T00:00:00Z"
	p.CLValueAtPurchaseSource = inventory.CLProvenanceSourceCardLadder
	p.CLCardConfidenceAtPurchase = intPtr(72)
	if err := ps.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := ps.GetPurchase(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CLValueAtPurchaseObservedAt != p.CLValueAtPurchaseObservedAt {
		t.Errorf("CLValueAtPurchaseObservedAt = %q, want %q", got.CLValueAtPurchaseObservedAt, p.CLValueAtPurchaseObservedAt)
	}
	if got.CLValueAtPurchaseSource != p.CLValueAtPurchaseSource {
		t.Errorf("CLValueAtPurchaseSource = %q, want %q", got.CLValueAtPurchaseSource, p.CLValueAtPurchaseSource)
	}
	if got.CLCardConfidenceAtPurchase == nil || *got.CLCardConfidenceAtPurchase != *p.CLCardConfidenceAtPurchase {
		t.Errorf("CLCardConfidenceAtPurchase = %v, want %v", got.CLCardConfidenceAtPurchase, p.CLCardConfidenceAtPurchase)
	}
}

func TestCreatePurchaseCLProvenanceSource(t *testing.T) {
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

	tests := []struct {
		name             string
		clValueCents     int
		clValueUpdatedAt string
		wantSource       string
		wantAtCents      int
		wantObservedSet  bool // true: must be non-empty and RFC3339-parseable
	}{
		{
			name:             "CardLadder already answered before create",
			clValueCents:     4200,
			clValueUpdatedAt: "2026-03-01T12:00:00Z",
			wantSource:       inventory.CLProvenanceSourceCardLadder,
			wantAtCents:      4200,
			wantObservedSet:  true,
		},
		{
			name:             "value carried from intake, CardLadder never answered",
			clValueCents:     3100,
			clValueUpdatedAt: "",
			wantSource:       inventory.CLProvenanceSourceIntake,
			wantAtCents:      3100,
			wantObservedSet:  true,
		},
		{
			name:             "no CL value at all",
			clValueCents:     0,
			clValueUpdatedAt: "",
			wantSource:       "",
			wantAtCents:      0,
			wantObservedSet:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := makeTestPurchase()
			p.CLValueCents = tt.clValueCents
			p.CLValueUpdatedAt = tt.clValueUpdatedAt
			if err := ps.CreatePurchase(ctx, p); err != nil {
				t.Fatalf("create: %v", err)
			}
			got, err := ps.GetPurchase(ctx, p.ID)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if got.CLValueAtPurchaseCents != tt.wantAtCents {
				t.Errorf("CLValueAtPurchaseCents = %d, want %d", got.CLValueAtPurchaseCents, tt.wantAtCents)
			}
			if got.CLValueAtPurchaseSource != tt.wantSource {
				t.Errorf("CLValueAtPurchaseSource = %q, want %q", got.CLValueAtPurchaseSource, tt.wantSource)
			}
			if tt.wantObservedSet {
				if got.CLValueAtPurchaseObservedAt == "" {
					t.Fatalf("CLValueAtPurchaseObservedAt must be set, got empty")
				}
				if _, err := time.Parse(time.RFC3339, got.CLValueAtPurchaseObservedAt); err != nil {
					t.Errorf("CLValueAtPurchaseObservedAt = %q is not RFC3339: %v", got.CLValueAtPurchaseObservedAt, err)
				}
				if tt.clValueUpdatedAt != "" && got.CLValueAtPurchaseObservedAt != tt.clValueUpdatedAt {
					t.Errorf("CLValueAtPurchaseObservedAt = %q, want %q (must equal CLValueUpdatedAt for a CardLadder-sourced freeze)", got.CLValueAtPurchaseObservedAt, tt.clValueUpdatedAt)
				}
			} else if got.CLValueAtPurchaseObservedAt != "" {
				t.Errorf("CLValueAtPurchaseObservedAt = %q, want empty", got.CLValueAtPurchaseObservedAt)
			}
		})
	}
}

func TestUpdatePurchaseCLValue_FreezeGuardAndConfidence(t *testing.T) {
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

	today := time.Now().UTC().Format("2006-01-02")
	stale := time.Now().UTC().AddDate(0, 0, -30).Format("2006-01-02")
	conf72 := 72
	conf0 := 0
	preConf := 1

	tests := []struct {
		name            string
		purchaseDate    string
		confidence      *int
		preFreeze       bool // apply an in-window freezing update before the one under test
		wantCentsFrozen bool
		wantPopFrozen   bool
		wantConfFrozen  bool
		wantConfValue   *int
	}{
		{
			name:            "recent purchase freezes everything",
			purchaseDate:    today,
			confidence:      &conf72,
			wantCentsFrozen: true,
			wantPopFrozen:   true,
			wantConfFrozen:  true,
			wantConfValue:   &conf72,
		},
		{
			name:            "stale purchase freezes nothing",
			purchaseDate:    stale,
			confidence:      &conf72,
			wantCentsFrozen: false,
			wantPopFrozen:   false,
			wantConfFrozen:  false,
		},
		{
			name:            "malformed purchase_date fails closed",
			purchaseDate:    "not-a-date",
			confidence:      &conf72,
			wantCentsFrozen: false,
			wantPopFrozen:   false,
			wantConfFrozen:  false,
		},
		{
			// The dangerous class: date-SHAPED but calendar-invalid. A ::date or
			// to_date() guard RAISES on these, aborting the whole UPDATE -- and
			// purchase_date is validated for non-emptiness only, so a client can
			// plant one and poison every later call. This case must return an
			// error-free "froze nothing", not an error.
			name:            "date-shaped but out-of-range month/day fails closed without erroring",
			purchaseDate:    "2026-99-99",
			confidence:      &conf72,
			wantCentsFrozen: false,
			wantPopFrozen:   false,
			wantConfFrozen:  false,
		},
		{
			// Calendar-invalid but shape-valid, and also far outside the window.
			// The guard does NOT reject this for being an impossible day -- the
			// regex accepts day 30 under month 02 and the comparison is textual,
			// exactly as cl_freeze.go's documented residual says. What this case
			// pins is that such input is handled as ordinary out-of-window text:
			// no freeze, and critically no error, where a ::date cast would raise
			// and abort the whole UPDATE.
			name:            "calendar-invalid date is compared as text, not cast",
			purchaseDate:    "2026-02-30",
			confidence:      &conf72,
			wantCentsFrozen: false,
			wantPopFrozen:   false,
			wantConfFrozen:  false,
		},
		{
			// Forged recency: a future purchase_date is within "7 days before
			// today" under a lower-bound-only guard. The upper bound rejects it.
			name:            "future purchase_date fails closed",
			purchaseDate:    "2099-01-01",
			confidence:      &conf72,
			wantCentsFrozen: false,
			wantPopFrozen:   false,
			wantConfFrozen:  false,
		},
		{
			name:            "nil confidence stays null",
			purchaseDate:    today,
			confidence:      nil,
			wantCentsFrozen: true,
			wantPopFrozen:   true,
			wantConfFrozen:  false,
		},
		{
			name:            "zero confidence freezes as zero, not null",
			purchaseDate:    today,
			confidence:      &conf0,
			wantCentsFrozen: true,
			wantPopFrozen:   true,
			wantConfFrozen:  true,
			wantConfValue:   &conf0,
		},
		{
			name:            "already-frozen recent purchase holds set-once",
			purchaseDate:    today,
			confidence:      &conf72,
			preFreeze:       true,
			wantCentsFrozen: true,
			wantPopFrozen:   true,
			wantConfFrozen:  true,
			wantConfValue:   &conf72,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := makeTestPurchase()
			p.PurchaseDate = tt.purchaseDate
			if err := ps.CreatePurchase(ctx, p); err != nil {
				t.Fatalf("create: %v", err)
			}

			if tt.preFreeze {
				if err := ps.UpdatePurchaseCLValue(ctx, p.ID, 900, 5, &preConf); err != nil {
					t.Fatalf("pre-freeze update: %v", err)
				}
			}

			// This Fatalf is load-bearing for the malformed-date cases: the whole
			// point of the shape-check-plus-text-comparison guard is that a
			// garbage purchase_date makes the freeze not fire, WITHOUT raising.
			// A cast-based guard fails here with a Postgres error, not with a
			// wrong value.
			if err := ps.UpdatePurchaseCLValue(ctx, p.ID, 500, 10, tt.confidence); err != nil {
				t.Fatalf("update: %v", err)
			}

			got, err := ps.GetPurchase(ctx, p.ID)
			if err != nil {
				t.Fatalf("get: %v", err)
			}

			// cl_value_cents and population always update, freeze guard notwithstanding.
			if got.CLValueCents != 500 {
				t.Errorf("CLValueCents = %d, want 500", got.CLValueCents)
			}
			if got.Population != 10 {
				t.Errorf("Population = %d, want 10", got.Population)
			}

			if tt.wantCentsFrozen {
				wantCents := 500
				if tt.preFreeze {
					wantCents = 900
				}
				if got.CLValueAtPurchaseCents != wantCents {
					t.Errorf("CLValueAtPurchaseCents = %d, want %d", got.CLValueAtPurchaseCents, wantCents)
				}
				if got.CLValueAtPurchaseObservedAt == "" {
					t.Error("CLValueAtPurchaseObservedAt should be set when frozen")
				}
				if got.CLValueAtPurchaseSource != inventory.CLProvenanceSourceCardLadder {
					t.Errorf("CLValueAtPurchaseSource = %q, want %q",
						got.CLValueAtPurchaseSource, inventory.CLProvenanceSourceCardLadder)
				}
			} else {
				if got.CLValueAtPurchaseCents != 0 {
					t.Errorf("CLValueAtPurchaseCents = %d, want 0 (not frozen)", got.CLValueAtPurchaseCents)
				}
				if got.CLValueAtPurchaseObservedAt != "" {
					t.Errorf("CLValueAtPurchaseObservedAt = %q, want empty", got.CLValueAtPurchaseObservedAt)
				}
				if got.CLValueAtPurchaseSource != "" {
					t.Errorf("CLValueAtPurchaseSource = %q, want empty", got.CLValueAtPurchaseSource)
				}
			}

			if tt.wantPopFrozen {
				wantPop := 10
				if tt.preFreeze {
					wantPop = 5
				}
				if got.PopulationAtPurchase == nil || *got.PopulationAtPurchase != wantPop {
					t.Errorf("PopulationAtPurchase = %v, want %d", got.PopulationAtPurchase, wantPop)
				}
			} else if got.PopulationAtPurchase != nil {
				t.Errorf("PopulationAtPurchase = %v, want nil", got.PopulationAtPurchase)
			}

			if tt.wantConfFrozen {
				if got.CLCardConfidenceAtPurchase == nil {
					t.Fatal("CLCardConfidenceAtPurchase = nil, want frozen value")
				}
				want := tt.wantConfValue
				if tt.preFreeze {
					want = &preConf
				}
				if *got.CLCardConfidenceAtPurchase != *want {
					t.Errorf("CLCardConfidenceAtPurchase = %d, want %d", *got.CLCardConfidenceAtPurchase, *want)
				}
			} else if got.CLCardConfidenceAtPurchase != nil {
				t.Errorf("CLCardConfidenceAtPurchase = %v, want nil", got.CLCardConfidenceAtPurchase)
			}
		})
	}
}
