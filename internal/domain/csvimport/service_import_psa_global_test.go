package csvimport_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/csvimport"
	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// These tests moved here from internal/domain/inventory when CSV intake was
// split out (SLA-35). They still drive setup through the inventory service —
// campaigns and their phase are inventory's to create — and exercise the import
// through csvimport, which is the seam under test.

// captureEventRecorder is a thread-safe recorder for black-box tests.
type captureEventRecorder struct {
	mu     sync.Mutex
	events []dhevents.Event
}

func (r *captureEventRecorder) Record(_ context.Context, e dhevents.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	return nil
}

func (r *captureEventRecorder) snapshot() []dhevents.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]dhevents.Event, len(r.events))
	copy(out, r.events)
	return out
}

// newServices builds the inventory service used for setup alongside the import
// service under test. Both share one repo and one ID generator, so IDs stay
// unique across campaigns and purchases the way they are in production.
func newServices(repo *mocks.InMemoryCampaignStore, pending inventory.PendingItemRepository, invOpts ...inventory.ServiceOption) (inventory.Service, csvimport.Service) {
	return newServicesWithResolver(repo, pending, nil, invOpts...)
}

// newServicesWithResolver is newServices for the tests that drive PSA campaign
// attribution: the resolver is an inventory option on one side and a Deps field
// on the other, so it cannot ride along in invOpts.
func newServicesWithResolver(repo *mocks.InMemoryCampaignStore, pending inventory.PendingItemRepository, resolver inventory.PSACampaignResolver, invOpts ...inventory.ServiceOption) (inventory.Service, csvimport.Service) {
	var counter atomic.Int64
	idGen := func() string { return fmt.Sprintf("test-id-%d", counter.Add(1)) }

	opts := append([]inventory.ServiceOption{inventory.WithIDGenerator(idGen)}, invOpts...)
	if pending != nil {
		opts = append(opts, inventory.WithPendingItemRepository(pending))
	}
	if resolver != nil {
		opts = append(opts, inventory.WithPSACampaignResolver(resolver))
	}
	inv := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, opts...)

	imp := csvimport.NewService(csvimport.Deps{
		Campaigns:    repo,
		Purchases:    repo,
		Sales:        repo,
		Finance:      repo,
		PendingItems: pending,
		Inventory:    inv,
		PSAResolver:  resolver,
		IDGen:        idGen,
	})
	return inv, imp
}

// setupActiveVintageCampaign creates and activates the same "Vintage"
// grade-range campaign TestService_ImportPSAExportGlobal_Allocate uses, so the
// pricing-enqueue tests below allocate their rows through the identical match
// path instead of duplicating a second fixture shape.
func setupActiveVintageCampaign(t *testing.T, repo *mocks.InMemoryCampaignStore) (inventory.Service, error) {
	t.Helper()
	ctx := context.Background()
	svc, _ := newServices(repo, nil)
	c := &inventory.Campaign{Name: "Vintage", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "8-10", PSASourcingFeeCents: 300}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		return nil, err
	}
	c.Phase = inventory.PhaseActive
	if err := svc.UpdateCampaign(ctx, c); err != nil {
		return nil, err
	}
	return svc, nil
}

func TestService_ImportPSAExportGlobal_EnqueuesPricing(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc, err := setupActiveVintageCampaign(t, repo)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	pricingQueue := &mocks.PricingEnqueuerMock{}
	imp := csvimport.NewService(csvimport.Deps{
		Campaigns:    repo,
		Purchases:    repo,
		Sales:        repo,
		Finance:      repo,
		Inventory:    svc,
		PricingQueue: pricingQueue,
		IDGen:        func() string { return "test-id-pricing" },
	})

	rows := []csvimport.PSAExportRow{
		{CertNumber: "PRICE01", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 9, PricePaid: 500, Date: "2026-01-15", Category: "Pokemon"},
	}
	result, err := imp.ImportPSAExportGlobal(context.Background(), rows)
	if err != nil {
		t.Fatalf("ImportPSAExportGlobal: %v", err)
	}
	if result.Allocated != 1 {
		t.Fatalf("Allocated = %d, want 1", result.Allocated)
	}
	got := pricingQueue.Certs()
	if len(got) != 1 || got[0] != "PRICE01" {
		t.Errorf("pricingQueue certs = %v, want [PRICE01]", got)
	}
}

func TestService_ImportPSAExportGlobal_NilPricingQueueDoesNotPanic(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc, err := setupActiveVintageCampaign(t, repo)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	imp := csvimport.NewService(csvimport.Deps{
		Campaigns: repo,
		Purchases: repo,
		Sales:     repo,
		Finance:   repo,
		Inventory: svc,
		// PricingQueue intentionally left nil.
		IDGen: func() string { return "test-id-nilpricing" },
	})

	rows := []csvimport.PSAExportRow{
		{CertNumber: "PRICE02", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 9, PricePaid: 500, Date: "2026-01-15", Category: "Pokemon"},
	}
	if _, err := imp.ImportPSAExportGlobal(context.Background(), rows); err != nil {
		t.Fatalf("ImportPSAExportGlobal with nil PricingQueue: %v", err)
	}
}

func TestService_ImportPSAExportGlobal_Allocate(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc, imp := newServices(repo, nil)
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

	rows := []csvimport.PSAExportRow{
		{CertNumber: "PSA001", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 9, PricePaid: 500, Date: "2026-01-15", Category: "Pokemon"},
	}

	result, err := imp.ImportPSAExportGlobal(ctx, rows)
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

// TestService_ImportPSAExportGlobal_RecordsEnrollmentEvent verifies that a
// newly-allocated PSA import emits a TypeEnrolled event with SourcePSAImport.
func TestService_ImportPSAExportGlobal_RecordsEnrollmentEvent(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	rec := &captureEventRecorder{}
	svc, imp := newServices(repo, nil, inventory.WithEventRecorder(rec))
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Vintage", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "8-10", PSASourcingFeeCents: 300}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}
	c.Phase = inventory.PhaseActive
	if err := svc.UpdateCampaign(ctx, c); err != nil {
		t.Fatalf("setup activate: %v", err)
	}

	rows := []csvimport.PSAExportRow{
		{CertNumber: "PSA001", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 9, PricePaid: 500, Date: "2026-01-15", Category: "Pokemon"},
	}
	if _, err := imp.ImportPSAExportGlobal(ctx, rows); err != nil {
		t.Fatalf("ImportPSAExportGlobal: %v", err)
	}

	// Filter to TypeEnrolled events for the allocated cert (batchResolveCardIDs
	// may also fire events if a cardIDResolver were set — here we didn't set one).
	var enrolled []dhevents.Event
	for _, e := range rec.snapshot() {
		if e.Type == dhevents.TypeEnrolled {
			enrolled = append(enrolled, e)
		}
	}
	if len(enrolled) != 1 {
		t.Fatalf("enrolled events = %d, want 1", len(enrolled))
	}
	got := enrolled[0]
	if got.CertNumber != "PSA001" {
		t.Errorf("certNumber = %q, want PSA001", got.CertNumber)
	}
	if got.NewPushStatus != inventory.DHPushStatusPending {
		t.Errorf("newPushStatus = %q, want %q", got.NewPushStatus, inventory.DHPushStatusPending)
	}
	if got.Source != dhevents.SourcePSAImport {
		t.Errorf("source = %q, want %q", got.Source, dhevents.SourcePSAImport)
	}
}

func TestService_ImportPSAExportGlobal_SkipExisting(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc, imp := newServices(repo, nil)
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

	rows := []csvimport.PSAExportRow{
		{CertNumber: "PSA002", ListingTitle: "Charizard", Grade: 9, PricePaid: 500,
			ShipDate: "2026-02-01", InvoiceDate: "2026-02-01", PurchaseSource: "PSA"},
	}

	result, err := imp.ImportPSAExportGlobal(ctx, rows)
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
	svc, imp := newServices(repo, nil)
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
	rows := []csvimport.PSAExportRow{
		{CertNumber: "PSA004", ListingTitle: "Umbreon PSA 7", Grade: 7, PricePaid: 200, Date: "2026-01-15", Category: "Pokemon"},
	}

	result, err := imp.ImportPSAExportGlobal(ctx, rows)
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
	svc, imp := newServices(repo, pendingRepo)
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
	rows := []csvimport.PSAExportRow{
		{CertNumber: "PSA-PEND-001", ListingTitle: "Pikachu PSA 5", Grade: 5, PricePaid: 100, Date: "2026-03-01", Category: "Pokemon"},
	}

	result, err := imp.ImportPSAExportGlobal(ctx, rows)
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
	svc, imp := newServices(repo, nil)
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
	rows := []csvimport.PSAExportRow{
		{CertNumber: "PSA-NIL-001", ListingTitle: "Mewtwo PSA 5", Grade: 5, PricePaid: 150, Date: "2026-03-01", Category: "Pokemon"},
	}

	// Should not panic
	result, err := imp.ImportPSAExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportPSAExportGlobal: %v", err)
	}
	if result.Unmatched != 1 {
		t.Errorf("Unmatched = %d, want 1", result.Unmatched)
	}
}

func TestService_ImportPSAExportGlobal_SkipEmpty(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	_, imp := newServices(repo, nil)
	ctx := context.Background()

	rows := []csvimport.PSAExportRow{
		{CertNumber: "", ListingTitle: "Empty cert"},                   // Skipped: no cert
		{CertNumber: "PSA005", ListingTitle: "No grade", PricePaid: 0}, // Skipped: no price
	}

	result, err := imp.ImportPSAExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportPSAExportGlobal: %v", err)
	}

	if result.Skipped != 2 {
		t.Errorf("Skipped = %d, want 2", result.Skipped)
	}
}

func TestService_ImportPSAExportGlobal_DuplicateSkip(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc, imp := newServices(repo, nil)
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", Sport: "Pokemon", BuyTermsCLPct: 0.78, GradeRange: "8-10"}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup: %v", err)
	}
	c.Phase = inventory.PhaseActive
	if err := svc.UpdateCampaign(ctx, c); err != nil {
		t.Fatalf("setup activate: %v", err)
	}

	rows := []csvimport.PSAExportRow{
		{CertNumber: "PSA006", ListingTitle: "Charizard PSA 9", Grade: 9, PricePaid: 500, Date: "2026-01-15", Category: "Pokemon"},
		{CertNumber: "PSA006", ListingTitle: "Charizard PSA 9", Grade: 9, PricePaid: 500, Date: "2026-01-15", Category: "Pokemon"},
	}

	result, err := imp.ImportPSAExportGlobal(ctx, rows)
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
	svc, imp := newServices(repo, nil)
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
	rows := []csvimport.PSAExportRow{
		{CertNumber: "PSA007", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 0, PricePaid: 500, Date: "2026-01-15", Category: "Pokemon"},
	}

	result, err := imp.ImportPSAExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportPSAExportGlobal: %v", err)
	}

	if result.Allocated != 1 {
		t.Errorf("Allocated = %d, want 1 (grade should be extracted from title)", result.Allocated)
	}
}

func TestService_ImportPSAExportGlobal_InvoiceUpdatesOnReimport(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc, imp := newServices(repo, nil)
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
	rows1 := []csvimport.PSAExportRow{
		{CertNumber: "INV001", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 9, PricePaid: 200, Date: "2026-03-01", InvoiceDate: "2026-03-15", Category: "Pokemon"},
	}
	result1, err := imp.ImportPSAExportGlobal(ctx, rows1)
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
	rows2 := []csvimport.PSAExportRow{
		{CertNumber: "INV001", ListingTitle: "2022 POKEMON CHARIZARD PSA 9", Grade: 9, PricePaid: 200, Date: "2026-03-01", InvoiceDate: "2026-03-15", Category: "Pokemon"},
		{CertNumber: "INV002", ListingTitle: "2022 POKEMON PIKACHU PSA 10", Grade: 10, PricePaid: 150, Date: "2026-03-02", InvoiceDate: "2026-03-15", Category: "Pokemon"},
	}
	result2, err := imp.ImportPSAExportGlobal(ctx, rows2)
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
