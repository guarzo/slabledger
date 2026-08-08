package inventory_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// testIDGen returns a deterministic ID generator for tests.
func testIDGen() func() string {
	var counter atomic.Int64
	return func() string { return fmt.Sprintf("test-id-%d", counter.Add(1)) }
}

// withTestIDGen is a convenience option for tests.
func withTestIDGen() inventory.ServiceOption {
	return inventory.WithIDGenerator(testIDGen())
}

// withDisabledBackgroundWorkers disables background workers to prevent races with non-thread-safe mocks.
func withDisabledBackgroundWorkers() inventory.ServiceOption {
	return inventory.WithDisableBackgroundWorkers()
}

func TestService_CreateCampaign(t *testing.T) {
	svc := inventory.NewService(mocks.NewInMemoryCampaignStore(), mocks.NewInMemoryCampaignStore(), mocks.NewInMemoryCampaignStore(), mocks.NewInMemoryCampaignStore(), mocks.NewInMemoryCampaignStore(), mocks.NewInMemoryCampaignStore(), mocks.NewInMemoryCampaignStore(), withTestIDGen())
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Vintage Core PSA 8-9", Sport: "Pokemon", BuyTermsCLPct: 0.80}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}
	if c.ID == "" {
		t.Error("expected ID to be set")
	}
	if c.Phase != inventory.PhasePending {
		t.Errorf("expected phase pending, got %s", c.Phase)
	}
}

func TestService_CreateCampaign_ValidationError(t *testing.T) {
	svc := inventory.NewService(mocks.NewInMemoryCampaignStore(), mocks.NewInMemoryCampaignStore(), mocks.NewInMemoryCampaignStore(), mocks.NewInMemoryCampaignStore(), mocks.NewInMemoryCampaignStore(), mocks.NewInMemoryCampaignStore(), mocks.NewInMemoryCampaignStore(), withTestIDGen())
	ctx := context.Background()

	c := &inventory.Campaign{Name: ""}
	if err := svc.CreateCampaign(ctx, c); !errors.Is(err, inventory.ErrCampaignNameRequired) {
		t.Errorf("expected ErrCampaignNameRequired, got %v", err)
	}
}

func TestService_CreatePurchase(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	// Create campaign first
	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p := &inventory.Purchase{
		CampaignID:   c.ID,
		CardName:     "Charizard",
		CertNumber:   "11111111",
		GradeValue:   9,
		BuyCostCents: 50000,
		PurchaseDate: "2026-01-15",
	}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("CreatePurchase: %v", err)
	}

	if p.ID == "" {
		t.Error("expected ID to be set")
	}
}

func TestService_CreatePurchase_DuplicateCert(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p1 := &inventory.Purchase{CampaignID: c.ID, CardName: "Charizard", CertNumber: "11111111", GradeValue: 9, BuyCostCents: 50000, PurchaseDate: "2026-01-15"}
	if err := svc.CreatePurchase(ctx, p1); err != nil {
		t.Fatalf("setup CreatePurchase: %v", err)
	}

	p2 := &inventory.Purchase{CampaignID: c.ID, CardName: "Pikachu", CertNumber: "11111111", GradeValue: 10, BuyCostCents: 30000, PurchaseDate: "2026-01-16"}
	if err := svc.CreatePurchase(ctx, p2); !errors.Is(err, inventory.ErrDuplicateCertNumber) {
		t.Errorf("expected ErrDuplicateCertNumber, got %v", err)
	}
}

func TestService_CreateSale_ComputesFieldsEbay(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78, EbayFeePct: 0.1235}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p := &inventory.Purchase{CampaignID: c.ID, CardName: "Charizard", CertNumber: "22222222", GradeValue: 9, BuyCostCents: 50000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-15"}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup CreatePurchase: %v", err)
	}

	s := &inventory.Sale{PurchaseID: p.ID, SaleChannel: inventory.SaleChannelEbay, SalePriceCents: 75000, SaleDate: "2026-02-01"}
	if err := svc.CreateSale(ctx, s, c, p); err != nil {
		t.Fatalf("CreateSale: %v", err)
	}

	// Check days to sell: Jan 15 -> Feb 1 = 17 days
	if s.DaysToSell != 17 {
		t.Errorf("DaysToSell = %d, want 17", s.DaysToSell)
	}

	// Check sale fee: 75000 * 0.1235 = 9262.5 -> 9263 (rounded)
	if s.SaleFeeCents != 9263 {
		t.Errorf("SaleFeeCents = %d, want 9263", s.SaleFeeCents)
	}

	// Check net profit: 75000 - 50000 - 300 - 9263 = 15437
	if s.NetProfitCents != 15437 {
		t.Errorf("NetProfitCents = %d, want 15437", s.NetProfitCents)
	}
}

func TestService_CreateSale_SaleDateBeforePurchase(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78, EbayFeePct: 0.12}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Charizard", CertNumber: "66666666",
		GradeValue: 9, BuyCostCents: 50000, PurchaseDate: "2026-02-15",
	}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup CreatePurchase: %v", err)
	}

	// Sale date is before purchase date
	s := &inventory.Sale{
		PurchaseID:     p.ID,
		SaleChannel:    inventory.SaleChannelEbay,
		SalePriceCents: 75000,
		SaleDate:       "2026-01-01",
	}
	err := svc.CreateSale(ctx, s, c, p)
	if !errors.Is(err, inventory.ErrSaleDateBeforePurchase) {
		t.Errorf("expected ErrSaleDateBeforePurchase, got %v", err)
	}
}

func TestService_CreateSale_SameDateAllowed(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78, EbayFeePct: 0.12}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Pikachu", CertNumber: "77777777",
		GradeValue: 10, BuyCostCents: 30000, PurchaseDate: "2026-03-01",
	}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup CreatePurchase: %v", err)
	}

	// Sale on the same day as purchase should succeed
	s := &inventory.Sale{
		PurchaseID:     p.ID,
		SaleChannel:    inventory.SaleChannelInPerson,
		SalePriceCents: 40000,
		SaleDate:       "2026-03-01",
	}
	if err := svc.CreateSale(ctx, s, c, p); err != nil {
		t.Fatalf("expected no error for same-day sale, got %v", err)
	}
	if s.DaysToSell != 0 {
		t.Errorf("DaysToSell = %d, want 0 for same-day sale", s.DaysToSell)
	}
}

// --- Cert lookup and QuickAdd tests ---

// mockCertLookup is a local test double for CertLookup using the Fn-field pattern.
// The shared version lives in testutil/mocks.MockCertLookup for use by other packages.
type mockCertLookup struct {
	LookupCertFn   func(ctx context.Context, certNumber string) (*inventory.CertInfo, error)
	LookupImagesFn func(ctx context.Context, certNumber string) (string, string, error)
}

func (m *mockCertLookup) LookupCert(ctx context.Context, certNumber string) (*inventory.CertInfo, error) {
	if m.LookupCertFn != nil {
		return m.LookupCertFn(ctx, certNumber)
	}
	return nil, nil
}

func (m *mockCertLookup) LookupImages(ctx context.Context, certNumber string) (string, string, error) {
	if m.LookupImagesFn != nil {
		return m.LookupImagesFn(ctx, certNumber)
	}
	return "", "", nil
}

// mockPriceLookup is a local test double for PriceLookup using the Fn-field pattern.
// The shared version lives in testutil/mocks.MockPriceLookup for use by other packages.
type mockPriceLookup struct {
	GetLastSoldCentsFn  func(ctx context.Context, card inventory.CardIdentity, grade float64) (int, error)
	GetMarketSnapshotFn func(ctx context.Context, card inventory.CardIdentity, grade float64) (*inventory.MarketSnapshot, error)
}

func (m *mockPriceLookup) GetLastSoldCents(ctx context.Context, card inventory.CardIdentity, grade float64) (int, error) {
	if m.GetLastSoldCentsFn != nil {
		return m.GetLastSoldCentsFn(ctx, card, grade)
	}
	return 0, nil
}

func (m *mockPriceLookup) GetMarketSnapshot(ctx context.Context, card inventory.CardIdentity, grade float64) (*inventory.MarketSnapshot, error) {
	if m.GetMarketSnapshotFn != nil {
		return m.GetMarketSnapshotFn(ctx, card, grade)
	}
	return nil, nil
}

// newDefaultCertLookup returns a mockCertLookup that always returns a fixed CertInfo.
func newDefaultCertLookup() *mockCertLookup {
	return &mockCertLookup{
		LookupCertFn: func(_ context.Context, certNumber string) (*inventory.CertInfo, error) {
			return &inventory.CertInfo{
				CertNumber: certNumber,
				CardName:   "2022 POKEMON CHARIZARD",
				Grade:      9,
				Year:       "2022",
				Brand:      "POKEMON",
				Subject:    "CHARIZARD",
				Population: 100,
			}, nil
		},
	}
}

// newDefaultPriceLookup returns a mockPriceLookup that returns fixed market data.
// If expectSetName is non-empty, GetMarketSnapshot will verify the identity's SetName.
func newDefaultPriceLookup(t *testing.T, expectSetName string) *mockPriceLookup {
	return &mockPriceLookup{
		GetLastSoldCentsFn: func(_ context.Context, _ inventory.CardIdentity, _ float64) (int, error) {
			return 55000, nil
		},
		GetMarketSnapshotFn: func(_ context.Context, identity inventory.CardIdentity, _ float64) (*inventory.MarketSnapshot, error) {
			if t != nil && expectSetName != "" {
				if identity.SetName != expectSetName {
					t.Errorf("GetMarketSnapshot: SetName = %q, want %q", identity.SetName, expectSetName)
				}
			}
			return &inventory.MarketSnapshot{
				LastSoldCents:     55000,
				GradePriceCents:   60000,
				MedianCents:       57000,
				ConservativeCents: 50000,
				OptimisticCents:   65000,
				SalesLast30d:      12,
			}, nil
		},
	}
}

func TestService_LookupCert(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen(), withDisabledBackgroundWorkers(), inventory.WithCertLookup(newDefaultCertLookup()), inventory.WithPriceLookup(newDefaultPriceLookup(nil, "")))
	ctx := context.Background()

	info, snapshot, err := svc.LookupCert(ctx, "12345678")
	if err != nil {
		t.Fatalf("LookupCert: %v", err)
	}
	if info.CardName != "2022 POKEMON CHARIZARD" {
		t.Errorf("CardName = %q", info.CardName)
	}
	if info.Grade != 9 {
		t.Errorf("Grade = %g", info.Grade)
	}
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}
	if snapshot.MedianCents != 57000 {
		t.Errorf("MedianCents = %d", snapshot.MedianCents)
	}
}

func TestService_LookupCert_NotConfigured(t *testing.T) {
	svc := inventory.NewService(mocks.NewInMemoryCampaignStore(), mocks.NewInMemoryCampaignStore(), mocks.NewInMemoryCampaignStore(), mocks.NewInMemoryCampaignStore(), mocks.NewInMemoryCampaignStore(), mocks.NewInMemoryCampaignStore(), mocks.NewInMemoryCampaignStore(), withTestIDGen())
	_, _, err := svc.LookupCert(context.Background(), "12345678")
	if err == nil {
		t.Error("expected error when cert lookup not configured")
	}
}

func TestService_QuickAddPurchase(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen(), inventory.WithCertLookup(newDefaultCertLookup()))
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78, PSASourcingFeeCents: 300}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	purchase, err := svc.QuickAddPurchase(ctx, c.ID, inventory.QuickAddRequest{
		CertNumber:   "87654321",
		BuyCostCents: 45000,
		CLValueCents: 50000,
	})
	if err != nil {
		t.Fatalf("QuickAddPurchase: %v", err)
	}
	if purchase.CardName != "2022 POKEMON CHARIZARD" {
		t.Errorf("CardName = %q", purchase.CardName)
	}
	if purchase.GradeValue != 9 {
		t.Errorf("GradeValue = %v", purchase.GradeValue)
	}
	if purchase.PSASourcingFeeCents != 300 {
		t.Errorf("PSASourcingFeeCents = %d", purchase.PSASourcingFeeCents)
	}
	// The campaign is a parameter chosen by the operator in the UI, not a
	// heuristic match, so quick-add is a manual attribution. The in-memory store
	// applies no attribution default, so this only passes if QuickAddPurchase
	// sets it.
	if purchase.AttributionSource != inventory.AttributionSourceManual {
		t.Errorf("AttributionSource = %q, want %q",
			purchase.AttributionSource, inventory.AttributionSourceManual)
	}
}

func TestService_CreateSale_ComputesFieldsLocal(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p := &inventory.Purchase{CampaignID: c.ID, CardName: "Umbreon", CertNumber: "33333333", GradeValue: 9, BuyCostCents: 80000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-10"}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup CreatePurchase: %v", err)
	}

	// InPerson sale (no fee)
	s := &inventory.Sale{PurchaseID: p.ID, SaleChannel: inventory.SaleChannelInPerson, SalePriceCents: 90000, SaleDate: "2026-01-20"}
	if err := svc.CreateSale(ctx, s, c, p); err != nil {
		t.Fatalf("CreateSale: %v", err)
	}

	if s.SaleFeeCents != 0 {
		t.Errorf("SaleFeeCents = %d, want 0 for local", s.SaleFeeCents)
	}

	// Net: 90000 - 80000 - 300 - 0 = 9700
	if s.NetProfitCents != 9700 {
		t.Errorf("NetProfitCents = %d, want 9700", s.NetProfitCents)
	}

	if s.DaysToSell != 10 {
		t.Errorf("DaysToSell = %d, want 10", s.DaysToSell)
	}
}

func TestService_CreatePurchase_CapturesSnapshot(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen(), withDisabledBackgroundWorkers(), inventory.WithPriceLookup(newDefaultPriceLookup(t, "Base Set")))
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Charizard", CertNumber: "44444444",
		SetName: "Base Set", GradeValue: 9, BuyCostCents: 50000, PurchaseDate: "2026-01-15",
	}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("CreatePurchase: %v", err)
	}

	if p.MedianCents != 57000 {
		t.Errorf("MedianCents = %d, want 57000", p.MedianCents)
	}
	if p.LastSoldCents != 55000 {
		t.Errorf("LastSoldCents = %d, want 55000", p.LastSoldCents)
	}
	if p.ConservativeCents != 50000 {
		t.Errorf("ConservativeCents = %d, want 50000", p.ConservativeCents)
	}
	if p.SalesLast30d != 12 {
		t.Errorf("SalesLast30d = %d, want 12", p.SalesLast30d)
	}
	if p.SnapshotDate == "" {
		t.Error("expected SnapshotDate to be set")
	}
}

func TestService_CreateSale_CapturesSnapshot(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen(), withDisabledBackgroundWorkers(), inventory.WithPriceLookup(newDefaultPriceLookup(t, "Base Set")))
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78, EbayFeePct: 0.1235}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Charizard", CertNumber: "55555555",
		SetName: "Base Set", GradeValue: 9, BuyCostCents: 50000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-15",
	}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup CreatePurchase: %v", err)
	}

	s := &inventory.Sale{PurchaseID: p.ID, SaleChannel: inventory.SaleChannelEbay, SalePriceCents: 60000, SaleDate: "2026-02-01"}
	if err := svc.CreateSale(ctx, s, c, p); err != nil {
		t.Fatalf("CreateSale: %v", err)
	}

	if s.MedianCents != 57000 {
		t.Errorf("Sale MedianCents = %d, want 57000", s.MedianCents)
	}
	if s.LastSoldCents != 55000 {
		t.Errorf("Sale LastSoldCents = %d, want 55000", s.LastSoldCents)
	}
	if s.SnapshotDate == "" {
		t.Error("expected sale SnapshotDate to be set")
	}
}

func TestCreatePurchase_FreezesCreationFacts(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen(), withDisabledBackgroundWorkers(), inventory.WithPriceLookup(newDefaultPriceLookup(t, "")))
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78, CLConfidence: "2.5-4"}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	tests := []struct {
		name           string
		certNumber     string
		population     int
		wantPopulation *int
	}{
		{name: "positive population is frozen", certNumber: "88888881", population: 50, wantPopulation: func() *int { v := 50; return &v }()},
		{name: "zero population freezes to nil", certNumber: "88888882", population: 0, wantPopulation: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &inventory.Purchase{
				CampaignID: c.ID, CardName: "Charizard", CertNumber: tt.certNumber,
				GradeValue: 9, BuyCostCents: 50000, PurchaseDate: "2026-01-15",
				Population: tt.population,
			}
			if err := svc.CreatePurchase(ctx, p); err != nil {
				t.Fatalf("CreatePurchase: %v", err)
			}

			if p.CLConfidenceAtPurchase == nil || *p.CLConfidenceAtPurchase != 2 {
				t.Errorf("CLConfidenceAtPurchase = %v, want 2", p.CLConfidenceAtPurchase)
			}
			if tt.wantPopulation == nil {
				if p.PopulationAtPurchase != nil {
					t.Errorf("PopulationAtPurchase = %v, want nil for Population:0", p.PopulationAtPurchase)
				}
				return
			}
			if p.PopulationAtPurchase == nil || *p.PopulationAtPurchase != *tt.wantPopulation {
				t.Errorf("PopulationAtPurchase = %v, want %v", p.PopulationAtPurchase, *tt.wantPopulation)
			}
		})
	}
}

func TestCreatePurchase_IgnoresClientForgedProvenance(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	failingLookup := &mockPriceLookup{
		GetMarketSnapshotFn: func(_ context.Context, _ inventory.CardIdentity, _ float64) (*inventory.MarketSnapshot, error) {
			return nil, fmt.Errorf("boom")
		},
	}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen(), withDisabledBackgroundWorkers(), inventory.WithPriceLookup(failingLookup))
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78, CLConfidence: "2.5-4"}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	junkInt := 999999
	junkFloat := 0.99
	p := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Charizard", CertNumber: "88888883",
		GradeValue: 9, BuyCostCents: 50000, PurchaseDate: "2026-01-15",
		Population:               50,
		CLConfidenceAtPurchase:   &junkInt,
		PopulationAtPurchase:     &junkInt,
		DHConfidenceAtPurchase:   &junkFloat,
		SourceCountAtPurchase:    &junkInt,
		ActiveListingsAtPurchase: &junkInt,
		SalesLast30dAtPurchase:   &junkInt,
	}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("CreatePurchase: %v", err)
	}

	if p.DHConfidenceAtPurchase != nil {
		t.Errorf("DHConfidenceAtPurchase = %v, want nil (capture failed)", p.DHConfidenceAtPurchase)
	}
	if p.SourceCountAtPurchase != nil {
		t.Errorf("SourceCountAtPurchase = %v, want nil (capture failed)", p.SourceCountAtPurchase)
	}
	if p.ActiveListingsAtPurchase != nil {
		t.Errorf("ActiveListingsAtPurchase = %v, want nil (capture failed)", p.ActiveListingsAtPurchase)
	}
	if p.SalesLast30dAtPurchase != nil {
		t.Errorf("SalesLast30dAtPurchase = %v, want nil (capture failed)", p.SalesLast30dAtPurchase)
	}
	if p.PopulationAtPurchase == nil || *p.PopulationAtPurchase != 50 {
		t.Errorf("PopulationAtPurchase = %v, want 50 (real value)", p.PopulationAtPurchase)
	}
	if p.CLConfidenceAtPurchase == nil || *p.CLConfidenceAtPurchase != 2 {
		t.Errorf("CLConfidenceAtPurchase = %v, want 2 (derived from campaign)", p.CLConfidenceAtPurchase)
	}
}

// --- ImportPSAExportGlobal tests ---

func TestService_CreateSale_WasCracked(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.80, EbayFeePct: 0.1235}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Umbreon VMAX", CertNumber: "77777777",
		GradeValue: 7, BuyCostCents: 14700, PSASourcingFeeCents: 300,
		PurchaseDate: "2026-03-20",
	}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup CreatePurchase: %v", err)
	}

	s := &inventory.Sale{
		PurchaseID:     p.ID,
		SaleChannel:    inventory.SaleChannelEbay,
		SalePriceCents: 25000,
		SaleDate:       "2026-03-25",
		WasCracked:     true,
	}
	if err := svc.CreateSale(ctx, s, c, p); err != nil {
		t.Fatalf("CreateSale: %v", err)
	}

	if !s.WasCracked {
		t.Error("expected WasCracked to be true after CreateSale")
	}
}

func TestService_CreateSale_SetsForcedLiquidation(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	// Seed an invoice due 2026-06-20
	repo.Invoices["inv1"] = &inventory.Invoice{ID: "inv1", DueDate: "2026-06-20"}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}
	p := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Charizard", CertNumber: "FL-CERT-1",
		GradeValue: 9, BuyCostCents: 50000, PurchaseDate: "2026-05-01",
	}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup CreatePurchase: %v", err)
	}

	tests := []struct {
		name     string
		channel  inventory.SaleChannel
		saleDate string
		want     bool
	}{
		{"inperson within 6d of due", inventory.SaleChannelInPerson, "2026-06-16", true},
		{"inperson outside window", inventory.SaleChannelInPerson, "2026-06-13", false},
		{"ebay inside window not forced", inventory.SaleChannelEbay, "2026-06-16", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a fresh cert number for each sub-test so CreatePurchase doesn't hit duplicate
			localP := &inventory.Purchase{
				CampaignID: c.ID, CardName: "Charizard", CertNumber: "FL-" + tt.name,
				GradeValue: 9, BuyCostCents: 50000, PurchaseDate: "2026-05-01",
			}
			if err := svc.CreatePurchase(ctx, localP); err != nil {
				t.Fatalf("setup CreatePurchase: %v", err)
			}
			s := &inventory.Sale{
				PurchaseID:     localP.ID,
				SaleChannel:    tt.channel,
				SalePriceCents: 70000,
				SaleDate:       tt.saleDate,
			}
			if err := svc.CreateSale(ctx, s, c, localP); err != nil {
				t.Fatalf("CreateSale: %v", err)
			}
			if s.ForcedLiquidation != tt.want {
				t.Errorf("ForcedLiquidation = %v, want %v", s.ForcedLiquidation, tt.want)
			}
		})
	}
}
