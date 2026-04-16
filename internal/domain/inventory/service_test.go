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
	LookupCertFn func(ctx context.Context, certNumber string) (*inventory.CertInfo, error)
}

func (m *mockCertLookup) LookupCert(ctx context.Context, certNumber string) (*inventory.CertInfo, error) {
	if m.LookupCertFn != nil {
		return m.LookupCertFn(ctx, certNumber)
	}
	return nil, nil
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

// --- ImportPSAExportGlobal tests ---

func TestService_ImportPSAExportGlobal_Allocate(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	// Create an active campaign with grade range 8-10
	c := &inventory.Campaign{Name: "Vintage", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "8-10", PSASourcingFeeCents: 300}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Activate campaign
	c.Phase = inventory.PhaseActive
	if err := svc.UpdateCampaign(ctx, c); err != nil {
		t.Fatalf("setup activate: %v", err)
	}

	rows := []inventory.PSAExportRow{
		{CertNumber: "PSA001", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 9, PricePaid: 500, Date: "2026-01-15", Category: "Pokemon"},
	}

	result, err := svc.ImportPSAExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportPSAExportGlobal: %v", err)
	}

	if result.Allocated != 1 {
		t.Errorf("Allocated = %d, want 1", result.Allocated)
	}
	if result.Unmatched != 0 {
		t.Errorf("Unmatched = %d, want 0", result.Unmatched)
	}
	if len(result.Results) != 1 {
		t.Fatalf("Results count = %d, want 1", len(result.Results))
	}
	if result.Results[0].Status != "allocated" {
		t.Errorf("Status = %q, want allocated", result.Results[0].Status)
	}
	if result.Results[0].CampaignID != c.ID {
		t.Errorf("CampaignID = %q, want %q", result.Results[0].CampaignID, c.ID)
	}

	// Verify the newly-allocated purchase is enrolled in the DH push pipeline.
	// Without this, the DH push scheduler silently skips the row and it never
	// gets matched to a DH card_id or pushed to DH inventory.
	p, err := repo.GetPurchaseByCertNumber(ctx, "PSA", "PSA001")
	if err != nil {
		t.Fatalf("lookup new purchase by cert: %v", err)
	}
	if p == nil {
		t.Fatal("new purchase not found after import")
	}
	if p.DHPushStatus != inventory.DHPushStatusPending {
		t.Errorf("DHPushStatus = %q, want %q (new PSA imports must enroll in DH push pipeline)", p.DHPushStatus, inventory.DHPushStatusPending)
	}
}

func TestService_ImportPSAExportGlobal_SkipExisting(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Pre-create a purchase with this cert number
	p := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Charizard", CertNumber: "PSA002",
		GradeValue: 9, BuyCostCents: 50000, PurchaseDate: "2026-01-15",
	}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup CreatePurchase: %v", err)
	}

	rows := []inventory.PSAExportRow{
		{CertNumber: "PSA002", ListingTitle: "Charizard", Grade: 9, PricePaid: 500,
			ShipDate: "2026-02-01", InvoiceDate: "2026-02-01", PurchaseSource: "PSA"},
	}

	result, err := svc.ImportPSAExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportPSAExportGlobal: %v", err)
	}

	if result.Updated != 1 {
		t.Errorf("Updated = %d, want 1", result.Updated)
	}
	if result.Allocated != 0 {
		t.Errorf("Allocated = %d, want 0", result.Allocated)
	}

	// Verify PSA fields were backfilled on the existing purchase
	updated, _ := repo.GetPurchaseByCertNumber(ctx, "PSA", "PSA002")
	if updated.InvoiceDate != "2026-02-01" {
		t.Errorf("InvoiceDate = %q, want %q", updated.InvoiceDate, "2026-02-01")
	}
	if updated.PSAShipDate != "2026-02-01" {
		t.Errorf("PSAShipDate = %q, want %q", updated.PSAShipDate, "2026-02-01")
	}
	if updated.PurchaseSource != "PSA" {
		t.Errorf("PurchaseSource = %q, want %q", updated.PurchaseSource, "PSA")
	}
}

func TestService_ImportPSAExportGlobal_Unmatched(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	// Create a campaign with strict grade range
	c := &inventory.Campaign{Name: "High Grade Only", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "9-10"}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}
	c.Phase = inventory.PhaseActive
	if err := svc.UpdateCampaign(ctx, c); err != nil {
		t.Fatalf("setup activate: %v", err)
	}

	// Import a grade 7 card — won't match 9-10 campaign
	rows := []inventory.PSAExportRow{
		{CertNumber: "PSA004", ListingTitle: "Umbreon PSA 7", Grade: 7, PricePaid: 200, Date: "2026-01-15", Category: "Pokemon"},
	}

	result, err := svc.ImportPSAExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportPSAExportGlobal: %v", err)
	}

	if result.Unmatched != 1 {
		t.Errorf("Unmatched = %d, want 1", result.Unmatched)
	}
}

func TestService_ImportPSAExportGlobal_SavesPendingItems(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	pendingRepo := &mocks.MockPendingItemRepository{}
	var savedItems []inventory.PendingItem
	pendingRepo.SavePendingItemsFn = func(_ context.Context, items []inventory.PendingItem) error {
		savedItems = items
		return nil
	}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo,
		withTestIDGen(),
		inventory.WithPendingItemRepository(pendingRepo),
	)
	ctx := context.Background()

	// Create an active campaign with strict grade range 9-10
	c := &inventory.Campaign{Name: "High Grade", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "9-10"}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}
	c.Phase = inventory.PhaseActive
	if err := svc.UpdateCampaign(ctx, c); err != nil {
		t.Fatalf("setup activate: %v", err)
	}

	// Import a grade 5 card — won't match the 9-10 campaign, becomes unmatched
	rows := []inventory.PSAExportRow{
		{CertNumber: "PSA-PEND-001", ListingTitle: "Pikachu PSA 5", Grade: 5, PricePaid: 100, Date: "2026-03-01", Category: "Pokemon"},
	}

	result, err := svc.ImportPSAExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportPSAExportGlobal: %v", err)
	}

	if result.Unmatched != 1 {
		t.Errorf("Unmatched = %d, want 1", result.Unmatched)
	}
	if len(savedItems) != 1 {
		t.Fatalf("savedItems count = %d, want 1", len(savedItems))
	}
	if savedItems[0].CertNumber != "PSA-PEND-001" {
		t.Errorf("CertNumber = %q, want %q", savedItems[0].CertNumber, "PSA-PEND-001")
	}
	if savedItems[0].Status != "unmatched" {
		t.Errorf("Status = %q, want %q", savedItems[0].Status, "unmatched")
	}
	if savedItems[0].Source != "manual" {
		t.Errorf("Source = %q, want %q", savedItems[0].Source, "manual")
	}
	if savedItems[0].Grade != 5 {
		t.Errorf("Grade = %v, want 5", savedItems[0].Grade)
	}
	if savedItems[0].ID == "" {
		t.Error("ID should not be empty")
	}
}

func TestService_ImportPSAExportGlobal_NilPendingRepoDoesNotPanic(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	// Create service WITHOUT WithPendingItemRepository
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	// Create an active campaign with strict grade range 9-10
	c := &inventory.Campaign{Name: "Strict", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "9-10"}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}
	c.Phase = inventory.PhaseActive
	if err := svc.UpdateCampaign(ctx, c); err != nil {
		t.Fatalf("setup activate: %v", err)
	}

	// Import a grade 5 card — unmatched, but no pending repo configured
	rows := []inventory.PSAExportRow{
		{CertNumber: "PSA-NIL-001", ListingTitle: "Mewtwo PSA 5", Grade: 5, PricePaid: 150, Date: "2026-03-01", Category: "Pokemon"},
	}

	// Should not panic
	result, err := svc.ImportPSAExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportPSAExportGlobal: %v", err)
	}
	if result.Unmatched != 1 {
		t.Errorf("Unmatched = %d, want 1", result.Unmatched)
	}
}

func TestService_ImportPSAExportGlobal_SkipEmpty(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	rows := []inventory.PSAExportRow{
		{CertNumber: "", ListingTitle: "Empty cert"},                   // Skipped: no cert
		{CertNumber: "PSA005", ListingTitle: "No grade", PricePaid: 0}, // Skipped: no price
	}

	result, err := svc.ImportPSAExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportPSAExportGlobal: %v", err)
	}

	if result.Skipped != 2 {
		t.Errorf("Skipped = %d, want 2", result.Skipped)
	}
}

func TestService_ImportPSAExportGlobal_DuplicateSkip(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "8-10"}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}
	c.Phase = inventory.PhaseActive
	if err := svc.UpdateCampaign(ctx, c); err != nil {
		t.Fatalf("setup activate: %v", err)
	}

	rows := []inventory.PSAExportRow{
		{CertNumber: "PSA006", ListingTitle: "Charizard PSA 9", Grade: 9, PricePaid: 500, Date: "2026-01-15", Category: "Pokemon"},
		{CertNumber: "PSA006", ListingTitle: "Charizard PSA 9", Grade: 9, PricePaid: 500, Date: "2026-01-15", Category: "Pokemon"},
	}

	result, err := svc.ImportPSAExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportPSAExportGlobal: %v", err)
	}

	// First row allocated, second row detects no changes and skips the update
	if result.Allocated != 1 {
		t.Errorf("Allocated = %d, want 1", result.Allocated)
	}
	if result.Updated != 0 {
		t.Errorf("Updated = %d, want 0 (duplicate row with identical fields)", result.Updated)
	}
	if len(result.Results) != 2 {
		t.Fatalf("Results len = %d, want 2", len(result.Results))
	}
	if result.Results[1].Status != "unchanged" {
		t.Errorf("Results[1].Status = %q, want \"unchanged\"", result.Results[1].Status)
	}
}

func TestService_ImportPSAExportGlobal_ExtractGrade(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "8-10"}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}
	c.Phase = inventory.PhaseActive
	if err := svc.UpdateCampaign(ctx, c); err != nil {
		t.Fatalf("setup activate: %v", err)
	}

	// Grade=0, but title contains "PSA 9" — should extract grade 9
	rows := []inventory.PSAExportRow{
		{CertNumber: "PSA007", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 0, PricePaid: 500, Date: "2026-01-15", Category: "Pokemon"},
	}

	result, err := svc.ImportPSAExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportPSAExportGlobal: %v", err)
	}

	if result.Allocated != 1 {
		t.Errorf("Allocated = %d, want 1 (grade should be extracted from title)", result.Allocated)
	}
}

func TestService_ImportPSAExportGlobal_InvoiceUpdatesOnReimport(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "8-10", PSASourcingFeeCents: 300}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}
	c.Phase = inventory.PhaseActive
	if err := svc.UpdateCampaign(ctx, c); err != nil {
		t.Fatalf("setup activate: %v", err)
	}

	// First import: one purchase with invoice date
	rows1 := []inventory.PSAExportRow{
		{CertNumber: "INV001", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 9, PricePaid: 200, Date: "2026-03-01", InvoiceDate: "2026-03-15", Category: "Pokemon"},
	}
	result1, err := svc.ImportPSAExportGlobal(ctx, rows1)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if result1.InvoicesCreated != 1 {
		t.Errorf("first import InvoicesCreated = %d, want 1", result1.InvoicesCreated)
	}

	// Find the created invoice and verify its total
	var firstInvoice *inventory.Invoice
	for _, inv := range repo.Invoices {
		if inv.InvoiceDate == "2026-03-15" {
			firstInvoice = inv
			break
		}
	}
	if firstInvoice == nil {
		t.Fatal("expected invoice for 2026-03-15 to exist after first import")
	}
	// 200 * 100 (cents) + 300 (sourcing fee) = 20300
	if firstInvoice.TotalCents != 20300 {
		t.Errorf("first invoice TotalCents = %d, want 20300", firstInvoice.TotalCents)
	}

	// Second import: new purchase for the same invoice date
	rows2 := []inventory.PSAExportRow{
		{CertNumber: "INV001", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 9, PricePaid: 200, Date: "2026-03-01", InvoiceDate: "2026-03-15", Category: "Pokemon"},
		{CertNumber: "INV002", ListingTitle: "2022 POKEMON PIKACHU PSA 10", Grade: 10, PricePaid: 150, Date: "2026-03-02", InvoiceDate: "2026-03-15", Category: "Pokemon"},
	}
	result2, err := svc.ImportPSAExportGlobal(ctx, rows2)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}

	// Should update the existing invoice, not create a new one
	if result2.InvoicesCreated != 0 {
		t.Errorf("second import InvoicesCreated = %d, want 0", result2.InvoicesCreated)
	}
	if result2.InvoicesUpdated != 1 {
		t.Errorf("second import InvoicesUpdated = %d, want 1", result2.InvoicesUpdated)
	}

	// Verify the invoice total now includes both purchases
	// Purchase 1: 20000 cents + 300 fee = 20300
	// Purchase 2: 15000 cents + 300 fee = 15300
	// Total: 35600
	if firstInvoice.TotalCents != 35600 {
		t.Errorf("updated invoice TotalCents = %d, want 35600", firstInvoice.TotalCents)
	}
}

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
