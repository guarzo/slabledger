package campaigns_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// testIDGen returns a deterministic ID generator for tests.
func testIDGen() func() string {
	var counter atomic.Int64
	return func() string { return fmt.Sprintf("test-id-%d", counter.Add(1)) }
}

// withTestIDGen is a convenience option for tests.
func withTestIDGen() campaigns.ServiceOption {
	return campaigns.WithIDGenerator(testIDGen())
}

func TestService_CreateCampaign(t *testing.T) {
	svc := campaigns.NewService(mocks.NewMockCampaignRepository(), withTestIDGen())
	ctx := context.Background()

	c := &campaigns.Campaign{Name: "Vintage Core PSA 8-9", Sport: "Pokemon", BuyTermsCLPct: 0.80}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}
	if c.ID == "" {
		t.Error("expected ID to be set")
	}
	if c.Phase != campaigns.PhasePending {
		t.Errorf("expected phase pending, got %s", c.Phase)
	}
}

func TestService_CreateCampaign_ValidationError(t *testing.T) {
	svc := campaigns.NewService(mocks.NewMockCampaignRepository(), withTestIDGen())
	ctx := context.Background()

	c := &campaigns.Campaign{Name: ""}
	if err := svc.CreateCampaign(ctx, c); !errors.Is(err, campaigns.ErrCampaignNameRequired) {
		t.Errorf("expected ErrCampaignNameRequired, got %v", err)
	}
}

func TestService_CreatePurchase(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	// Create campaign first
	c := &campaigns.Campaign{Name: "Test", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p := &campaigns.Purchase{
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
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	c := &campaigns.Campaign{Name: "Test", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p1 := &campaigns.Purchase{CampaignID: c.ID, CardName: "Charizard", CertNumber: "11111111", GradeValue: 9, BuyCostCents: 50000, PurchaseDate: "2026-01-15"}
	if err := svc.CreatePurchase(ctx, p1); err != nil {
		t.Fatalf("setup CreatePurchase: %v", err)
	}

	p2 := &campaigns.Purchase{CampaignID: c.ID, CardName: "Pikachu", CertNumber: "11111111", GradeValue: 10, BuyCostCents: 30000, PurchaseDate: "2026-01-16"}
	if err := svc.CreatePurchase(ctx, p2); !errors.Is(err, campaigns.ErrDuplicateCertNumber) {
		t.Errorf("expected ErrDuplicateCertNumber, got %v", err)
	}
}

func TestService_CreateSale_ComputesFieldsEbay(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	c := &campaigns.Campaign{Name: "Test", BuyTermsCLPct: 0.78, EbayFeePct: 0.1235}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p := &campaigns.Purchase{CampaignID: c.ID, CardName: "Charizard", CertNumber: "22222222", GradeValue: 9, BuyCostCents: 50000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-15"}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup CreatePurchase: %v", err)
	}

	s := &campaigns.Sale{PurchaseID: p.ID, SaleChannel: campaigns.SaleChannelEbay, SalePriceCents: 75000, SaleDate: "2026-02-01"}
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
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	c := &campaigns.Campaign{Name: "Test", BuyTermsCLPct: 0.78, EbayFeePct: 0.12}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p := &campaigns.Purchase{
		CampaignID: c.ID, CardName: "Charizard", CertNumber: "66666666",
		GradeValue: 9, BuyCostCents: 50000, PurchaseDate: "2026-02-15",
	}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup CreatePurchase: %v", err)
	}

	// Sale date is before purchase date
	s := &campaigns.Sale{
		PurchaseID:     p.ID,
		SaleChannel:    campaigns.SaleChannelEbay,
		SalePriceCents: 75000,
		SaleDate:       "2026-01-01",
	}
	err := svc.CreateSale(ctx, s, c, p)
	if !errors.Is(err, campaigns.ErrSaleDateBeforePurchase) {
		t.Errorf("expected ErrSaleDateBeforePurchase, got %v", err)
	}
}

func TestService_CreateSale_SameDateAllowed(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	c := &campaigns.Campaign{Name: "Test", BuyTermsCLPct: 0.78, EbayFeePct: 0.12}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p := &campaigns.Purchase{
		CampaignID: c.ID, CardName: "Pikachu", CertNumber: "77777777",
		GradeValue: 10, BuyCostCents: 30000, PurchaseDate: "2026-03-01",
	}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup CreatePurchase: %v", err)
	}

	// Sale on the same day as purchase should succeed
	s := &campaigns.Sale{
		PurchaseID:     p.ID,
		SaleChannel:    campaigns.SaleChannelLocal,
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
	LookupCertFn func(ctx context.Context, certNumber string) (*campaigns.CertInfo, error)
}

func (m *mockCertLookup) LookupCert(ctx context.Context, certNumber string) (*campaigns.CertInfo, error) {
	if m.LookupCertFn != nil {
		return m.LookupCertFn(ctx, certNumber)
	}
	return nil, nil
}

// mockPriceLookup is a local test double for PriceLookup using the Fn-field pattern.
// The shared version lives in testutil/mocks.MockPriceLookup for use by other packages.
type mockPriceLookup struct {
	GetLastSoldCentsFn  func(ctx context.Context, card campaigns.CardIdentity, grade float64) (int, error)
	GetMarketSnapshotFn func(ctx context.Context, card campaigns.CardIdentity, grade float64) (*campaigns.MarketSnapshot, error)
}

func (m *mockPriceLookup) GetLastSoldCents(ctx context.Context, card campaigns.CardIdentity, grade float64) (int, error) {
	if m.GetLastSoldCentsFn != nil {
		return m.GetLastSoldCentsFn(ctx, card, grade)
	}
	return 0, nil
}

func (m *mockPriceLookup) GetMarketSnapshot(ctx context.Context, card campaigns.CardIdentity, grade float64) (*campaigns.MarketSnapshot, error) {
	if m.GetMarketSnapshotFn != nil {
		return m.GetMarketSnapshotFn(ctx, card, grade)
	}
	return nil, nil
}

// newDefaultCertLookup returns a mockCertLookup that always returns a fixed CertInfo.
func newDefaultCertLookup() *mockCertLookup {
	return &mockCertLookup{
		LookupCertFn: func(_ context.Context, certNumber string) (*campaigns.CertInfo, error) {
			return &campaigns.CertInfo{
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
		GetLastSoldCentsFn: func(_ context.Context, _ campaigns.CardIdentity, _ float64) (int, error) {
			return 55000, nil
		},
		GetMarketSnapshotFn: func(_ context.Context, identity campaigns.CardIdentity, _ float64) (*campaigns.MarketSnapshot, error) {
			if t != nil && expectSetName != "" {
				if identity.SetName != expectSetName {
					t.Errorf("GetMarketSnapshot: SetName = %q, want %q", identity.SetName, expectSetName)
				}
			}
			return &campaigns.MarketSnapshot{
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
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen(), campaigns.WithCertLookup(newDefaultCertLookup()), campaigns.WithPriceLookup(newDefaultPriceLookup(nil, "")))
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
	svc := campaigns.NewService(mocks.NewMockCampaignRepository(), withTestIDGen())
	_, _, err := svc.LookupCert(context.Background(), "12345678")
	if err == nil {
		t.Error("expected error when cert lookup not configured")
	}
}

func TestService_QuickAddPurchase(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen(), campaigns.WithCertLookup(newDefaultCertLookup()))
	ctx := context.Background()

	c := &campaigns.Campaign{Name: "Test", BuyTermsCLPct: 0.78, PSASourcingFeeCents: 300}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	purchase, err := svc.QuickAddPurchase(ctx, c.ID, campaigns.QuickAddRequest{
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

func TestService_GenerateSellSheet(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen(), campaigns.WithPriceLookup(newDefaultPriceLookup(nil, "")))
	ctx := context.Background()

	c := &campaigns.Campaign{Name: "Test", BuyTermsCLPct: 0.78, EbayFeePct: 0.1235}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p := &campaigns.Purchase{CampaignID: c.ID, CardName: "Charizard", CertNumber: "99999999", SetName: "Base Set", GradeValue: 9, BuyCostCents: 50000, CLValueCents: 55000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-15"}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup CreatePurchase: %v", err)
	}

	sheet, err := svc.GenerateSellSheet(ctx, c.ID, []string{p.ID})
	if err != nil {
		t.Fatalf("GenerateSellSheet: %v", err)
	}
	if sheet.CampaignName != "Test" {
		t.Errorf("CampaignName = %q", sheet.CampaignName)
	}
	if len(sheet.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(sheet.Items))
	}
	item := sheet.Items[0]
	if item.CostBasisCents != 50300 {
		t.Errorf("CostBasisCents = %d, want 50300", item.CostBasisCents)
	}
	if item.CurrentMarket == nil {
		t.Error("expected currentMarket")
	}
}

func TestService_CreateSale_ComputesFieldsLocal(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	c := &campaigns.Campaign{Name: "Test", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p := &campaigns.Purchase{CampaignID: c.ID, CardName: "Umbreon", CertNumber: "33333333", GradeValue: 9, BuyCostCents: 80000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-10"}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup CreatePurchase: %v", err)
	}

	// Local sale (e.g., GameStop at 90% CL)
	s := &campaigns.Sale{PurchaseID: p.ID, SaleChannel: campaigns.SaleChannelLocal, SalePriceCents: 90000, SaleDate: "2026-01-20"}
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
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen(), campaigns.WithPriceLookup(newDefaultPriceLookup(t, "Base Set")))
	ctx := context.Background()

	c := &campaigns.Campaign{Name: "Test", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p := &campaigns.Purchase{
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
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen(), campaigns.WithPriceLookup(newDefaultPriceLookup(t, "Base Set")))
	ctx := context.Background()

	c := &campaigns.Campaign{Name: "Test", BuyTermsCLPct: 0.78, EbayFeePct: 0.1235}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p := &campaigns.Purchase{
		CampaignID: c.ID, CardName: "Charizard", CertNumber: "55555555",
		SetName: "Base Set", GradeValue: 9, BuyCostCents: 50000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-15",
	}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup CreatePurchase: %v", err)
	}

	s := &campaigns.Sale{PurchaseID: p.ID, SaleChannel: campaigns.SaleChannelEbay, SalePriceCents: 60000, SaleDate: "2026-02-01"}
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
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	// Create an active campaign with grade range 8-10
	c := &campaigns.Campaign{Name: "Vintage", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "8-10", PSASourcingFeeCents: 300}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Activate campaign
	c.Phase = campaigns.PhaseActive
	if err := svc.UpdateCampaign(ctx, c); err != nil {
		t.Fatalf("setup activate: %v", err)
	}

	rows := []campaigns.PSAExportRow{
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
}

func TestService_ImportPSAExportGlobal_SkipExisting(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	c := &campaigns.Campaign{Name: "Test", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Pre-create a purchase with this cert number
	p := &campaigns.Purchase{
		CampaignID: c.ID, CardName: "Charizard", CertNumber: "PSA002",
		GradeValue: 9, BuyCostCents: 50000, PurchaseDate: "2026-01-15",
	}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup CreatePurchase: %v", err)
	}

	rows := []campaigns.PSAExportRow{
		{CertNumber: "PSA002", ListingTitle: "Charizard", Grade: 9, PricePaid: 500,
			VaultStatus: "IN_VAULT", InvoiceDate: "2026-02-01", PurchaseSource: "PSA"},
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
	if updated.VaultStatus != "IN_VAULT" {
		t.Errorf("VaultStatus = %q, want %q", updated.VaultStatus, "IN_VAULT")
	}
	if updated.PurchaseSource != "PSA" {
		t.Errorf("PurchaseSource = %q, want %q", updated.PurchaseSource, "PSA")
	}
}

func TestService_ImportPSAExportGlobal_Unmatched(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	// Create a campaign with strict grade range
	c := &campaigns.Campaign{Name: "High Grade Only", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "9-10"}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}
	c.Phase = campaigns.PhaseActive
	if err := svc.UpdateCampaign(ctx, c); err != nil {
		t.Fatalf("setup activate: %v", err)
	}

	// Import a grade 7 card — won't match 9-10 campaign
	rows := []campaigns.PSAExportRow{
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

func TestService_ImportPSAExportGlobal_SkipEmpty(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	rows := []campaigns.PSAExportRow{
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
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	c := &campaigns.Campaign{Name: "Test", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "8-10"}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}
	c.Phase = campaigns.PhaseActive
	if err := svc.UpdateCampaign(ctx, c); err != nil {
		t.Fatalf("setup activate: %v", err)
	}

	rows := []campaigns.PSAExportRow{
		{CertNumber: "PSA006", ListingTitle: "Charizard PSA 9", Grade: 9, PricePaid: 500, Date: "2026-01-15", Category: "Pokemon"},
		{CertNumber: "PSA006", ListingTitle: "Charizard PSA 9", Grade: 9, PricePaid: 500, Date: "2026-01-15", Category: "Pokemon"},
	}

	result, err := svc.ImportPSAExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportPSAExportGlobal: %v", err)
	}

	// First row allocated, second row updates the same purchase with PSA fields
	if result.Allocated != 1 {
		t.Errorf("Allocated = %d, want 1", result.Allocated)
	}
	if result.Updated != 1 {
		t.Errorf("Updated = %d, want 1", result.Updated)
	}
}

func TestService_ImportPSAExportGlobal_ExtractGrade(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	c := &campaigns.Campaign{Name: "Test", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "8-10"}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}
	c.Phase = campaigns.PhaseActive
	if err := svc.UpdateCampaign(ctx, c); err != nil {
		t.Fatalf("setup activate: %v", err)
	}

	// Grade=0, but title contains "PSA 9" — should extract grade 9
	rows := []campaigns.PSAExportRow{
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

// --- GetPortfolioHealth tests ---

func TestService_GetPortfolioHealth_Healthy(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	c := &campaigns.Campaign{Name: "Profitable", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}
	repo.PNLData[c.ID] = &campaigns.CampaignPNL{
		CampaignID:        c.ID,
		TotalSpendCents:   100000,
		TotalRevenueCents: 120000,
		TotalFeesCents:    15000,
		ROI:               0.05,
		TotalPurchases:    10,
		TotalSold:         8,
		TotalUnsold:       2,
		SellThroughPct:    0.80,
		AvgDaysToSell:     14,
	}

	health, err := svc.GetPortfolioHealth(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioHealth: %v", err)
	}

	if len(health.Campaigns) != 1 {
		t.Fatalf("expected 1 campaign, got %d", len(health.Campaigns))
	}
	if health.Campaigns[0].HealthStatus != "healthy" {
		t.Errorf("HealthStatus = %q, want healthy", health.Campaigns[0].HealthStatus)
	}
	if health.TotalDeployed != 100000 {
		t.Errorf("TotalDeployed = %d, want 100000", health.TotalDeployed)
	}
}

func TestService_GetPortfolioHealth_Warning(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	c := &campaigns.Campaign{Name: "Losing", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}
	repo.PNLData[c.ID] = &campaigns.CampaignPNL{
		CampaignID:        c.ID,
		TotalSpendCents:   100000,
		TotalRevenueCents: 90000,
		TotalFeesCents:    10000,
		ROI:               -0.05,
		TotalPurchases:    10,
		TotalSold:         5,
		TotalUnsold:       5,
		AvgDaysToSell:     20,
	}

	health, err := svc.GetPortfolioHealth(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioHealth: %v", err)
	}

	if health.Campaigns[0].HealthStatus != "warning" {
		t.Errorf("HealthStatus = %q, want warning (ROI -5%%)", health.Campaigns[0].HealthStatus)
	}
}

func TestService_GetPortfolioHealth_Critical(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	c := &campaigns.Campaign{Name: "Bleeding", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}
	repo.PNLData[c.ID] = &campaigns.CampaignPNL{
		CampaignID:        c.ID,
		TotalSpendCents:   200000,
		TotalRevenueCents: 100000,
		TotalFeesCents:    15000,
		ROI:               -0.50,
		TotalPurchases:    20,
		TotalSold:         5,
		TotalUnsold:       15,
		AvgDaysToSell:     60,
	}

	health, err := svc.GetPortfolioHealth(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioHealth: %v", err)
	}

	if health.Campaigns[0].HealthStatus != "critical" {
		t.Errorf("HealthStatus = %q, want critical (ROI -50%%, 15 unsold)", health.Campaigns[0].HealthStatus)
	}
}

func TestService_GetPortfolioHealth_SlowSellThrough(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	c := &campaigns.Campaign{Name: "Slow", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}
	repo.PNLData[c.ID] = &campaigns.CampaignPNL{
		CampaignID:        c.ID,
		TotalSpendCents:   100000,
		TotalRevenueCents: 110000,
		TotalFeesCents:    5000,
		ROI:               0.05,
		TotalPurchases:    10,
		TotalSold:         3,
		TotalUnsold:       7,
		AvgDaysToSell:     50, // > 45 threshold
	}

	health, err := svc.GetPortfolioHealth(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioHealth: %v", err)
	}

	// Positive ROI but slow sell-through → warning
	if health.Campaigns[0].HealthStatus != "warning" {
		t.Errorf("HealthStatus = %q, want warning (slow sell-through)", health.Campaigns[0].HealthStatus)
	}
}

func TestService_GetPortfolioHealth_OverallROI(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	c1 := &campaigns.Campaign{Name: "Campaign A", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c1); err != nil {
		t.Fatalf("setup: %v", err)
	}
	c2 := &campaigns.Campaign{Name: "Campaign B", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c2); err != nil {
		t.Fatalf("setup: %v", err)
	}

	repo.PNLData[c1.ID] = &campaigns.CampaignPNL{
		CampaignID: c1.ID, TotalSpendCents: 100000, TotalRevenueCents: 120000, TotalFeesCents: 10000,
		ROI: 0.10, TotalSold: 5, TotalUnsold: 0,
	}
	repo.PNLData[c2.ID] = &campaigns.CampaignPNL{
		CampaignID: c2.ID, TotalSpendCents: 50000, TotalRevenueCents: 40000, TotalFeesCents: 5000,
		ROI: -0.20, TotalSold: 3, TotalUnsold: 2,
	}

	health, err := svc.GetPortfolioHealth(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioHealth: %v", err)
	}

	if len(health.Campaigns) != 2 {
		t.Fatalf("expected 2 campaigns, got %d", len(health.Campaigns))
	}
	// Total deployed = 150000, total recovered = (120000-10000) + (40000-5000) = 145000
	// OverallROI = (145000 - 150000) / 150000 ≈ -0.0333
	if health.TotalDeployed != 150000 {
		t.Errorf("TotalDeployed = %d, want 150000", health.TotalDeployed)
	}
	expectedRecovered := 110000 + 35000
	if health.TotalRecovered != expectedRecovered {
		t.Errorf("TotalRecovered = %d, want %d", health.TotalRecovered, expectedRecovered)
	}
}

func TestService_GetPortfolioHealth_RealizedROI(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	c1 := &campaigns.Campaign{Name: "Fully Sold", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c1); err != nil {
		t.Fatalf("setup: %v", err)
	}
	c2 := &campaigns.Campaign{Name: "Mixed", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c2); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Campaign 1: all sold (5/5), no unsold
	// soldCostBasis = 100000 * 5/5 = 100000
	// soldProfit = 120000 - 10000 - 100000 = 10000
	repo.PNLData[c1.ID] = &campaigns.CampaignPNL{
		CampaignID: c1.ID, TotalSpendCents: 100000, TotalRevenueCents: 120000, TotalFeesCents: 10000,
		ROI: 0.10, TotalSold: 5, TotalUnsold: 0, TotalPurchases: 5,
	}

	// Campaign 2: partially sold (2/5)
	// soldCostBasis = 50000 * 2/5 = 20000
	// soldProfit = 30000 - 3000 - 20000 = 7000
	repo.PNLData[c2.ID] = &campaigns.CampaignPNL{
		CampaignID: c2.ID, TotalSpendCents: 50000, TotalRevenueCents: 30000, TotalFeesCents: 3000,
		ROI: -0.20, TotalSold: 2, TotalUnsold: 3, TotalPurchases: 5,
	}

	health, err := svc.GetPortfolioHealth(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioHealth: %v", err)
	}

	// totalSoldCostBasis = 100000 + 20000 = 120000
	// totalSoldNetProfit = 10000 + 7000 = 17000
	// RealizedROI = 17000 / 120000 ≈ 0.14167
	wantROI := float64(17000) / float64(120000)
	if diff := health.RealizedROI - wantROI; diff > 0.0001 || diff < -0.0001 {
		t.Errorf("RealizedROI = %f, want ~%f", health.RealizedROI, wantROI)
	}
}

func TestService_GetPortfolioHealth_RealizedROI_Rounding(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	c1 := &campaigns.Campaign{Name: "Odd Split", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c1); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// 1000 cents spent, 1 of 3 sold → soldCostBasis = round(1000*1/3) = round(333.33) = 333
	// soldProfit = 500 - 50 - 333 = 117
	repo.PNLData[c1.ID] = &campaigns.CampaignPNL{
		CampaignID: c1.ID, TotalSpendCents: 1000, TotalRevenueCents: 500, TotalFeesCents: 50,
		ROI: 0, TotalSold: 1, TotalUnsold: 2, TotalPurchases: 3,
	}

	health, err := svc.GetPortfolioHealth(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioHealth: %v", err)
	}

	// With float64 rounding: soldCostBasis = 333, profit = 117
	// RealizedROI = 117 / 333 ≈ 0.35135
	wantROI := float64(117) / float64(333)
	if diff := health.RealizedROI - wantROI; diff > 0.001 || diff < -0.001 {
		t.Errorf("RealizedROI = %f, want ~%f", health.RealizedROI, wantROI)
	}
}

func TestService_GetPortfolioHealth_RealizedROI_NoSales(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	c1 := &campaigns.Campaign{Name: "No Sales", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c1); err != nil {
		t.Fatalf("setup: %v", err)
	}

	repo.PNLData[c1.ID] = &campaigns.CampaignPNL{
		CampaignID: c1.ID, TotalSpendCents: 50000, TotalRevenueCents: 0, TotalFeesCents: 0,
		ROI: 0, TotalSold: 0, TotalUnsold: 5, TotalPurchases: 5,
	}

	health, err := svc.GetPortfolioHealth(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioHealth: %v", err)
	}

	// No sales → totalSoldCostBasis stays 0 → RealizedROI stays 0
	if health.RealizedROI != 0 {
		t.Errorf("RealizedROI = %f, want 0 (no sales)", health.RealizedROI)
	}
}

// --- GetPortfolioChannelVelocity tests ---

func TestService_GetPortfolioChannelVelocity_Empty(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	velocity, err := svc.GetPortfolioChannelVelocity(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioChannelVelocity: %v", err)
	}
	if len(velocity) != 0 {
		t.Errorf("expected empty result, got %d entries", len(velocity))
	}
}

func TestService_GetPortfolioChannelVelocity_WithData(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	repo.ChannelVelocity = []campaigns.ChannelVelocity{
		{Channel: campaigns.SaleChannelEbay, SaleCount: 10, AvgDaysToSell: 14.5, RevenueCents: 500000},
		{Channel: campaigns.SaleChannelGameStop, SaleCount: 5, AvgDaysToSell: 0, RevenueCents: 250000},
		{Channel: campaigns.SaleChannelLocal, SaleCount: 3, AvgDaysToSell: 7.0, RevenueCents: 120000},
	}
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	velocity, err := svc.GetPortfolioChannelVelocity(ctx)
	if err != nil {
		t.Fatalf("GetPortfolioChannelVelocity: %v", err)
	}
	if len(velocity) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(velocity))
	}

	// Verify passthrough returns data as-is from the repository
	if velocity[0].Channel != campaigns.SaleChannelEbay {
		t.Errorf("velocity[0].Channel = %q, want ebay", velocity[0].Channel)
	}
	if velocity[0].SaleCount != 10 {
		t.Errorf("velocity[0].SaleCount = %d, want 10", velocity[0].SaleCount)
	}
	if velocity[0].AvgDaysToSell != 14.5 {
		t.Errorf("velocity[0].AvgDaysToSell = %f, want 14.5", velocity[0].AvgDaysToSell)
	}
	if velocity[0].RevenueCents != 500000 {
		t.Errorf("velocity[0].RevenueCents = %d, want 500000", velocity[0].RevenueCents)
	}

	if velocity[1].Channel != campaigns.SaleChannelGameStop {
		t.Errorf("velocity[1].Channel = %q, want gamestop", velocity[1].Channel)
	}
	if velocity[1].SaleCount != 5 {
		t.Errorf("velocity[1].SaleCount = %d, want 5", velocity[1].SaleCount)
	}

	if velocity[2].Channel != campaigns.SaleChannelLocal {
		t.Errorf("velocity[2].Channel = %q, want local", velocity[2].Channel)
	}
	if velocity[2].RevenueCents != 120000 {
		t.Errorf("velocity[2].RevenueCents = %d, want 120000", velocity[2].RevenueCents)
	}
}

func TestService_CreateSale_WasCracked(t *testing.T) {
	repo := mocks.NewMockCampaignRepository()
	svc := campaigns.NewService(repo, withTestIDGen())
	ctx := context.Background()

	c := &campaigns.Campaign{Name: "Test", BuyTermsCLPct: 0.80, EbayFeePct: 0.1235}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup CreateCampaign: %v", err)
	}

	p := &campaigns.Purchase{
		CampaignID: c.ID, CardName: "Umbreon VMAX", CertNumber: "77777777",
		GradeValue: 7, BuyCostCents: 14700, PSASourcingFeeCents: 300,
		PurchaseDate: "2026-03-20",
	}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup CreatePurchase: %v", err)
	}

	s := &campaigns.Sale{
		PurchaseID:     p.ID,
		SaleChannel:    campaigns.SaleChannelEbay,
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
