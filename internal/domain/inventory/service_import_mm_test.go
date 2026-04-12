package inventory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// ---------------------------------------------------------------------------
// ExportMMFormatGlobal
// ---------------------------------------------------------------------------

func TestExportMMFormatGlobal_EmptyInventory(t *testing.T) {
	r := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(r, r, r, r, r, r, r, r, withTestIDGen())
	entries, err := svc.ExportMMFormatGlobal(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestExportMMFormatGlobal_RepoError(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	wantErr := errors.New("db down")
	repo.ListAllUnsoldPurchasesFn = func(_ context.Context) ([]inventory.Purchase, error) {
		return nil, wantErr
	}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	_, err := svc.ExportMMFormatGlobal(context.Background(), false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want to wrap %v", err, wantErr)
	}
}

func TestExportMMFormatGlobal_FullFields(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	purchase := &inventory.Purchase{
		ID:            "p1",
		CardName:      "Charizard",
		CardPlayer:    "Charizard",
		CertNumber:    "12345678",
		CardNumber:    "4",
		SetName:       "Base Set",
		Grader:        "PSA",
		GradeValue:    9,
		BuyCostCents:  10000, // $100.00
		PurchaseDate:  "2024-01-15",
		MMValueCents:  15000, // $150.00
		CLValueCents:  12000, // $120.00
		CardYear:      "1999",
		CardVariation: "Holo Rare",
		CardCategory:  "Pokemon",
	}
	purchase.SnapshotDate = "2024-03-01"
	repo.Purchases["p1"] = purchase

	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	entries, err := svc.ExportMMFormatGlobal(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Sport != "Pokemon" {
		t.Errorf("Sport = %q, want %q", e.Sport, "Pokemon")
	}
	if e.Grade != "PSA 9" {
		t.Errorf("Grade = %q, want %q", e.Grade, "PSA 9")
	}
	if e.PlayerName != "Charizard" {
		t.Errorf("PlayerName = %q, want %q", e.PlayerName, "Charizard")
	}
	if e.Year != "1999" {
		t.Errorf("Year = %q, want %q", e.Year, "1999")
	}
	if e.Set != "Base Set" {
		t.Errorf("Set = %q, want %q", e.Set, "Base Set")
	}
	if e.CardNumber != "#4" {
		t.Errorf("CardNumber = %q, want %q", e.CardNumber, "#4")
	}
	if e.Quantity != "1" {
		t.Errorf("Quantity = %q, want %q", e.Quantity, "1")
	}
	if e.Notes != "12345678" {
		t.Errorf("Notes (cert) = %q, want %q", e.Notes, "12345678")
	}
	if e.PurchasePricePerCard != 100.0 {
		t.Errorf("PurchasePricePerCard = %v, want 100.0", e.PurchasePricePerCard)
	}
}

func TestExportMMFormatGlobal_GraderDefaults(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	repo.Purchases["p1"] = &inventory.Purchase{
		ID: "p1", CardName: "Pikachu", GradeValue: 10, BuyCostCents: 5000,
	}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	entries, err := svc.ExportMMFormatGlobal(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Grade != "PSA 10" {
		t.Errorf("Grade = %q, want %q (missing grader should default to PSA)", entries[0].Grade, "PSA 10")
	}
}

func TestExportMMFormatGlobal_HalfGrade(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	repo.Purchases["p1"] = &inventory.Purchase{
		ID: "p1", CardName: "Mewtwo", Grader: "BGS", GradeValue: 9.5, BuyCostCents: 8000,
	}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	entries, err := svc.ExportMMFormatGlobal(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Grade != "BGS 9.5" {
		t.Errorf("Grade = %q, want %q", entries[0].Grade, "BGS 9.5")
	}
}

func TestExportMMFormatGlobal_PlayerNameFallback(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	repo.Purchases["p1"] = &inventory.Purchase{
		ID: "p1", CardName: "Gengar Holo", CardPlayer: "", BuyCostCents: 3000,
	}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	entries, err := svc.ExportMMFormatGlobal(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries[0].PlayerName != "Gengar Holo" {
		t.Errorf("PlayerName = %q, want %q (should fall back to CardName)", entries[0].PlayerName, "Gengar Holo")
	}
}

func TestExportMMFormatGlobal_MissingMMOnly(t *testing.T) {
	tests := []struct {
		name          string
		missingMMOnly bool
		wantCount     int
	}{
		{"all items", false, 3},
		{"missing MM only", true, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := mocks.NewInMemoryCampaignStore()
			repo.Purchases["p1"] = &inventory.Purchase{
				ID: "p1", CardName: "Card A", MMValueCents: 20000, BuyCostCents: 5000,
			}
			repo.Purchases["p2"] = &inventory.Purchase{
				ID: "p2", CardName: "Card B", MMValueCents: 0, BuyCostCents: 5000,
			}
			repo.Purchases["p3"] = &inventory.Purchase{
				ID: "p3", CardName: "Card C", MMValueCents: 0, CLValueCents: 15000, BuyCostCents: 5000,
			}
			svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
			entries, err := svc.ExportMMFormatGlobal(context.Background(), tc.missingMMOnly)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(entries) != tc.wantCount {
				t.Errorf("got %d entries, want %d", len(entries), tc.wantCount)
			}
		})
	}
}

func TestExportMMFormatGlobal_EmptyCardNumber(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	repo.Purchases["p1"] = &inventory.Purchase{
		ID: "p1", CardName: "Pikachu", CardNumber: "", BuyCostCents: 5000,
	}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, repo, withTestIDGen())
	entries, err := svc.ExportMMFormatGlobal(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries[0].CardNumber != "" {
		t.Errorf("CardNumber = %q, want empty string (no # prefix for empty card number)", entries[0].CardNumber)
	}
}

// ---------------------------------------------------------------------------
// RefreshMMValuesGlobal
// ---------------------------------------------------------------------------

func TestRefreshMMValuesGlobal_HappyPath(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	repo.Purchases["p1"] = &inventory.Purchase{
		ID: "p1", CardName: "Charizard", CertNumber: "11111111", MMValueCents: 10000,
	}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, repo, withTestIDGen())

	result, err := svc.RefreshMMValuesGlobal(context.Background(), []inventory.MMRefreshRow{
		{CertNumber: "11111111", LastSalePrice: 200.0},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Updated != 1 {
		t.Errorf("Updated = %d, want 1", result.Updated)
	}
	if result.Failed != 0 || result.NotFound != 0 || result.Skipped != 0 {
		t.Errorf("unexpected counts: failed=%d notFound=%d skipped=%d", result.Failed, result.NotFound, result.Skipped)
	}
	if len(result.Results) != 1 {
		t.Fatalf("Results length = %d, want 1", len(result.Results))
	}
	r := result.Results[0]
	if r.Status != "updated" {
		t.Errorf("Status = %q, want %q", r.Status, "updated")
	}
	if r.OldValueCents != 10000 {
		t.Errorf("OldValueCents = %d, want 10000", r.OldValueCents)
	}
	if r.NewValueCents != 20000 {
		t.Errorf("NewValueCents = %d, want 20000 ($200.00 → 20000 cents)", r.NewValueCents)
	}
	// Verify the purchase was actually updated
	if repo.Purchases["p1"].MMValueCents != 20000 {
		t.Errorf("Purchase MMValueCents = %d, want 20000 after update", repo.Purchases["p1"].MMValueCents)
	}
}

func TestRefreshMMValuesGlobal_CertNotFound(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, repo, withTestIDGen())

	result, err := svc.RefreshMMValuesGlobal(context.Background(), []inventory.MMRefreshRow{
		{CertNumber: "99999999", LastSalePrice: 50.0},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NotFound != 1 {
		t.Errorf("NotFound = %d, want 1", result.NotFound)
	}
	if result.Results[0].Status != "skipped" {
		t.Errorf("Status = %q, want skipped", result.Results[0].Status)
	}
}

func TestRefreshMMValuesGlobal_ZeroPrice(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	repo.Purchases["p1"] = &inventory.Purchase{
		ID: "p1", CardName: "Pikachu", CertNumber: "22222222", MMValueCents: 5000,
	}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, repo, withTestIDGen())

	result, err := svc.RefreshMMValuesGlobal(context.Background(), []inventory.MMRefreshRow{
		{CertNumber: "22222222", LastSalePrice: 0},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}
	if result.Results[0].Status != "skipped" {
		t.Errorf("Status = %q, want skipped", result.Results[0].Status)
	}
	// MMValueCents should remain unchanged
	if repo.Purchases["p1"].MMValueCents != 5000 {
		t.Errorf("MMValueCents = %d, want 5000 (unchanged)", repo.Purchases["p1"].MMValueCents)
	}
}

func TestRefreshMMValuesGlobal_EmptyCert(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, repo, withTestIDGen())

	result, err := svc.RefreshMMValuesGlobal(context.Background(), []inventory.MMRefreshRow{
		{CertNumber: "", LastSalePrice: 100.0},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
	if result.Results[0].Status != "failed" {
		t.Errorf("Status = %q, want failed", result.Results[0].Status)
	}
}

func TestRefreshMMValuesGlobal_MixedRows(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	repo.Purchases["p1"] = &inventory.Purchase{
		ID: "p1", CardName: "Charizard", CertNumber: "11111111", MMValueCents: 10000,
	}
	repo.Purchases["p2"] = &inventory.Purchase{
		ID: "p2", CardName: "Pikachu", CertNumber: "22222222", MMValueCents: 5000,
	}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, repo, withTestIDGen())

	rows := []inventory.MMRefreshRow{
		{CertNumber: "11111111", LastSalePrice: 200.0}, // updated
		{CertNumber: "99999999", LastSalePrice: 50.0},  // not found → skipped
		{CertNumber: "22222222", LastSalePrice: 0},     // zero price → skipped
		{CertNumber: "", LastSalePrice: 75.0},          // empty cert → failed
	}
	result, err := svc.RefreshMMValuesGlobal(context.Background(), rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Updated != 1 {
		t.Errorf("Updated = %d, want 1", result.Updated)
	}
	if result.NotFound != 1 {
		t.Errorf("NotFound = %d, want 1", result.NotFound)
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
	if len(result.Results) != 4 {
		t.Errorf("Results length = %d, want 4", len(result.Results))
	}
}

func TestRefreshMMValuesGlobal_RepoError(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	wantErr := errors.New("db error")
	repo.GetPurchasesByCertNumbersFn = func(_ context.Context, _ []string) (map[string]*inventory.Purchase, error) {
		return nil, wantErr
	}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo, repo, withTestIDGen())

	_, err := svc.RefreshMMValuesGlobal(context.Background(), []inventory.MMRefreshRow{
		{CertNumber: "11111111", LastSalePrice: 100.0},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want to wrap %v", err, wantErr)
	}
}
