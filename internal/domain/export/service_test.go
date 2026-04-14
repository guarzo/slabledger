package export_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/export"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// --- minimal ExportReader stub ---

// stubExportReader implements export.ExportReader for testing.
// Each method delegates to a function field; nil fields return safe defaults.
type stubExportReader struct {
	getSellSheetItemsFn         func(ctx context.Context) ([]string, error)
	addSellSheetItemsFn         func(ctx context.Context, purchaseIDs []string) error
	removeSellSheetItemsFn      func(ctx context.Context, purchaseIDs []string) error
	clearSellSheetFn            func(ctx context.Context) error
	getPurchasesByIDsFn         func(ctx context.Context, purchaseIDs []string) (map[string]*inventory.Purchase, error)
	listAllUnsoldPurchasesFn    func(ctx context.Context) ([]inventory.Purchase, error)
	listEbayFlaggedPurchasesFn  func(ctx context.Context) ([]inventory.Purchase, error)
	clearEbayExportFlagsFn      func(ctx context.Context, purchaseIDs []string) error
	getPurchasesByCertNumbersFn func(ctx context.Context, certNumbers []string) (map[string]*inventory.Purchase, error)
	getCampaignFn               func(ctx context.Context, id string) (*inventory.Campaign, error)
	listCampaignsFn             func(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error)
}

func (s *stubExportReader) GetSellSheetItems(ctx context.Context) ([]string, error) {
	if s.getSellSheetItemsFn != nil {
		return s.getSellSheetItemsFn(ctx)
	}
	return []string{}, nil
}

func (s *stubExportReader) AddSellSheetItems(ctx context.Context, purchaseIDs []string) error {
	if s.addSellSheetItemsFn != nil {
		return s.addSellSheetItemsFn(ctx, purchaseIDs)
	}
	return nil
}

func (s *stubExportReader) RemoveSellSheetItems(ctx context.Context, purchaseIDs []string) error {
	if s.removeSellSheetItemsFn != nil {
		return s.removeSellSheetItemsFn(ctx, purchaseIDs)
	}
	return nil
}

func (s *stubExportReader) ClearSellSheet(ctx context.Context) error {
	if s.clearSellSheetFn != nil {
		return s.clearSellSheetFn(ctx)
	}
	return nil
}

func (s *stubExportReader) GetPurchasesByIDs(ctx context.Context, purchaseIDs []string) (map[string]*inventory.Purchase, error) {
	if s.getPurchasesByIDsFn != nil {
		return s.getPurchasesByIDsFn(ctx, purchaseIDs)
	}
	return map[string]*inventory.Purchase{}, nil
}

func (s *stubExportReader) ListAllUnsoldPurchases(ctx context.Context) ([]inventory.Purchase, error) {
	if s.listAllUnsoldPurchasesFn != nil {
		return s.listAllUnsoldPurchasesFn(ctx)
	}
	return []inventory.Purchase{}, nil
}

func (s *stubExportReader) ListEbayFlaggedPurchases(ctx context.Context) ([]inventory.Purchase, error) {
	if s.listEbayFlaggedPurchasesFn != nil {
		return s.listEbayFlaggedPurchasesFn(ctx)
	}
	return []inventory.Purchase{}, nil
}

func (s *stubExportReader) ClearEbayExportFlags(ctx context.Context, purchaseIDs []string) error {
	if s.clearEbayExportFlagsFn != nil {
		return s.clearEbayExportFlagsFn(ctx, purchaseIDs)
	}
	return nil
}

func (s *stubExportReader) GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*inventory.Purchase, error) {
	if s.getPurchasesByCertNumbersFn != nil {
		return s.getPurchasesByCertNumbersFn(ctx, certNumbers)
	}
	return map[string]*inventory.Purchase{}, nil
}

func (s *stubExportReader) GetCampaign(ctx context.Context, id string) (*inventory.Campaign, error) {
	if s.getCampaignFn != nil {
		return s.getCampaignFn(ctx, id)
	}
	return &inventory.Campaign{ID: id, Name: "Test Campaign"}, nil
}

func (s *stubExportReader) ListCampaigns(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error) {
	if s.listCampaignsFn != nil {
		return s.listCampaignsFn(ctx, activeOnly)
	}
	return []inventory.Campaign{}, nil
}

// Compile-time interface check.
var _ export.ExportReader = (*stubExportReader)(nil)

// receivedAt returns a non-nil string pointer for Purchase.ReceivedAt.
func receivedAt() *string {
	s := time.Now().Format(time.RFC3339)
	return &s
}

// --- GenerateEbayCSV ---

func TestExportService_GenerateEbayCSV(t *testing.T) {
	psaPurchase := &inventory.Purchase{
		ID:         "psa-1",
		CertNumber: "12345678",
		CardName:   "Charizard",
		SetName:    "Base Set",
		CardNumber: "4",
		CardYear:   "1999",
		GradeValue: 10,
		Grader:     "PSA",
	}

	tests := []struct {
		name            string
		items           []inventory.EbayExportGenerateItem
		purchasesByIDs  map[string]*inventory.Purchase
		wantErr         bool
		wantErrContains string
		wantCSVContains string
	}{
		{
			name:            "error — empty items slice",
			items:           []inventory.EbayExportGenerateItem{},
			wantErr:         true,
			wantErrContains: "no items",
		},
		{
			name: "error — item with zero price",
			items: []inventory.EbayExportGenerateItem{
				{PurchaseID: "psa-1", PriceCents: 0},
			},
			wantErr:         true,
			wantErrContains: "invalid price",
		},
		{
			name: "error — duplicate purchase IDs",
			items: []inventory.EbayExportGenerateItem{
				{PurchaseID: "psa-1", PriceCents: 5000},
				{PurchaseID: "psa-1", PriceCents: 5000},
			},
			wantErr:         true,
			wantErrContains: "duplicate",
		},
		{
			name: "error — purchase not found in repo",
			items: []inventory.EbayExportGenerateItem{
				{PurchaseID: "missing", PriceCents: 5000},
			},
			purchasesByIDs:  map[string]*inventory.Purchase{},
			wantErr:         true,
			wantErrContains: "not found",
		},
		{
			name: "success — single PSA item",
			items: []inventory.EbayExportGenerateItem{
				{PurchaseID: "psa-1", PriceCents: 10000},
			},
			purchasesByIDs: map[string]*inventory.Purchase{
				"psa-1": psaPurchase,
			},
			wantCSVContains: "Charizard",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubExportReader{}
			if tc.purchasesByIDs != nil {
				pids := tc.purchasesByIDs
				repo.getPurchasesByIDsFn = func(_ context.Context, _ []string) (map[string]*inventory.Purchase, error) {
					return pids, nil
				}
			}
			svc := export.New(repo)

			csvBytes, err := svc.GenerateEbayCSV(context.Background(), tc.items)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.wantErrContains != "" && !strings.Contains(err.Error(), tc.wantErrContains) {
					t.Errorf("expected error to contain %q, got: %v", tc.wantErrContains, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(csvBytes) == 0 {
				t.Fatal("expected non-empty CSV output")
			}
			if tc.wantCSVContains != "" && !strings.Contains(string(csvBytes), tc.wantCSVContains) {
				t.Errorf("expected CSV to contain %q", tc.wantCSVContains)
			}
		})
	}
}

// --- ListEbayExportItems ---

func TestExportService_ListEbayExportItems(t *testing.T) {
	psaPurchases := []inventory.Purchase{
		{
			ID:           "p-1",
			CertNumber:   "11111111",
			CardName:     "Pikachu",
			Grader:       "PSA",
			GradeValue:   9,
			CLValueCents: 8000,
		},
		{
			ID:                 "p-2",
			CertNumber:         "22222222",
			CardName:           "Blastoise",
			Grader:             "PSA",
			GradeValue:         8,
			MarketSnapshotData: inventory.MarketSnapshotData{MedianCents: 5000},
		},
	}

	tests := []struct {
		name        string
		flaggedOnly bool
		flaggedFn   func(context.Context) ([]inventory.Purchase, error)
		allFn       func(context.Context) ([]inventory.Purchase, error)
		wantLen     int
		wantErr     bool
	}{
		{
			name:        "flagged-only returns PSA items",
			flaggedOnly: true,
			flaggedFn: func(_ context.Context) ([]inventory.Purchase, error) {
				return psaPurchases, nil
			},
			wantLen: 2,
		},
		{
			name:        "all unsold returns PSA items",
			flaggedOnly: false,
			allFn: func(_ context.Context) ([]inventory.Purchase, error) {
				return psaPurchases, nil
			},
			wantLen: 2,
		},
		{
			name:        "non-PSA purchases are filtered out",
			flaggedOnly: true,
			flaggedFn: func(_ context.Context) ([]inventory.Purchase, error) {
				return []inventory.Purchase{
					{ID: "p-3", Grader: "BGS", CardName: "Mewtwo"},
				}, nil
			},
			wantLen: 0,
		},
		{
			name:        "repo error propagated",
			flaggedOnly: true,
			flaggedFn: func(_ context.Context) ([]inventory.Purchase, error) {
				return nil, errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubExportReader{}
			if tc.flaggedFn != nil {
				repo.listEbayFlaggedPurchasesFn = tc.flaggedFn
			}
			if tc.allFn != nil {
				repo.listAllUnsoldPurchasesFn = tc.allFn
			}
			svc := export.New(repo)

			resp, err := svc.ListEbayExportItems(context.Background(), tc.flaggedOnly)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(resp.Items) != tc.wantLen {
				t.Errorf("expected %d items, got %d", tc.wantLen, len(resp.Items))
			}
		})
	}
}

// --- GenerateSellSheet ---

func TestExportService_GenerateSellSheet_NoPurchases(t *testing.T) {
	repo := &stubExportReader{
		getCampaignFn: func(_ context.Context, id string) (*inventory.Campaign, error) {
			return &inventory.Campaign{ID: id, Name: "Empty Campaign"}, nil
		},
		getPurchasesByIDsFn: func(_ context.Context, _ []string) (map[string]*inventory.Purchase, error) {
			return map[string]*inventory.Purchase{}, nil
		},
	}
	svc := export.New(repo)

	sheet, err := svc.GenerateSellSheet(context.Background(), "camp-1", []string{"p-1", "p-2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sheet == nil {
		t.Fatal("expected non-nil sell sheet")
	}
	if len(sheet.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(sheet.Items))
	}
	// Both purchase IDs were not found — they should be counted as skipped.
	if sheet.Totals.SkippedItems != 2 {
		t.Errorf("expected 2 skipped items, got %d", sheet.Totals.SkippedItems)
	}
}

func TestExportService_GenerateSellSheet_CampaignError(t *testing.T) {
	repo := &stubExportReader{
		getCampaignFn: func(_ context.Context, _ string) (*inventory.Campaign, error) {
			return nil, errors.New("campaign not found")
		},
	}
	svc := export.New(repo)

	_, err := svc.GenerateSellSheet(context.Background(), "camp-missing", []string{"p-1"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- GenerateGlobalSellSheet ---

func TestExportService_GenerateGlobalSellSheet(t *testing.T) {
	tests := []struct {
		name    string
		listFn  func(context.Context) ([]inventory.Purchase, error)
		wantErr bool
	}{
		{
			name: "success — no unsold purchases",
			listFn: func(_ context.Context) ([]inventory.Purchase, error) {
				return []inventory.Purchase{}, nil
			},
		},
		{
			name: "success — purchases without ReceivedAt are skipped",
			listFn: func(_ context.Context) ([]inventory.Purchase, error) {
				return []inventory.Purchase{
					{ID: "p-pending", CardName: "Eevee", ReceivedAt: nil},
					{ID: "p-received", CardName: "Vaporeon", ReceivedAt: receivedAt()},
				}, nil
			},
		},
		{
			name: "error — repo error propagated",
			listFn: func(_ context.Context) ([]inventory.Purchase, error) {
				return nil, errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubExportReader{
				listAllUnsoldPurchasesFn: tc.listFn,
			}
			svc := export.New(repo)

			sheet, err := svc.GenerateGlobalSellSheet(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sheet == nil {
				t.Fatal("expected non-nil sell sheet")
			}
		})
	}
}

// --- GenerateSelectedSellSheet ---

// makePurchaseWithMarketData creates a purchase with market snapshot data populated.
func makePurchaseWithMarketData(id, cert, card string) *inventory.Purchase {
	return &inventory.Purchase{
		ID:           id,
		CertNumber:   cert,
		CardName:     card,
		SetName:      "Base Set",
		CardNumber:   "4",
		GradeValue:   9,
		Grader:       "PSA",
		ReceivedAt:   receivedAt(),
		PurchaseDate: time.Now().Add(-30 * 24 * time.Hour).Format(time.DateOnly), // 30 days ago
		// Market snapshot data (so HasAnyPriceData returns true)
		MarketSnapshotData: inventory.MarketSnapshotData{
			MedianCents:       10000,
			ConservativeCents: 8000,
			LastSoldCents:     9500,
			SnapshotDate:      time.Now().Format(time.RFC3339), // Required for SnapshotFromPurchase to work
		},
	}
}

func TestExportService_GenerateSelectedSellSheet(t *testing.T) {
	tests := []struct {
		name        string
		purchaseIDs []string
		purchaseMap map[string]*inventory.Purchase
		listFn      func(context.Context, bool) ([]inventory.Campaign, error) // for ListCampaigns
		wantCount   int
		wantErr     bool
	}{
		{
			name:        "returns only selected purchases with market data",
			purchaseIDs: []string{"p1", "p2"},
			purchaseMap: map[string]*inventory.Purchase{
				"p1": makePurchaseWithMarketData("p1", "11111111", "Charizard"),
				"p2": makePurchaseWithMarketData("p2", "22222222", "Blastoise"),
				"p3": makePurchaseWithMarketData("p3", "33333333", "Venusaur"),
			},
			listFn: func(_ context.Context, _ bool) ([]inventory.Campaign, error) {
				return []inventory.Campaign{}, nil
			},
			wantCount: 2,
		},
		{
			name:        "empty selection returns empty sheet",
			purchaseIDs: []string{},
			purchaseMap: map[string]*inventory.Purchase{
				"p1": makePurchaseWithMarketData("p1", "11111111", "Charizard"),
			},
			listFn: func(_ context.Context, _ bool) ([]inventory.Campaign, error) {
				return []inventory.Campaign{}, nil
			},
			wantCount: 0,
		},
		{
			name:        "skips purchases without ReceivedAt",
			purchaseIDs: []string{"p1", "p2"},
			purchaseMap: map[string]*inventory.Purchase{
				"p1": makePurchaseWithMarketData("p1", "11111111", "Charizard"),
				"p2": {
					ID:         "p2",
					CertNumber: "22222222",
					CardName:   "Pikachu",
					GradeValue: 10,
					Grader:     "PSA",
					ReceivedAt: nil, // not received yet
					MarketSnapshotData: inventory.MarketSnapshotData{
						MedianCents:       5000,
						ConservativeCents: 4000,
					},
				},
			},
			listFn: func(_ context.Context, _ bool) ([]inventory.Campaign, error) {
				return []inventory.Campaign{}, nil
			},
			wantCount: 1,
		},
		{
			name:        "skips missing purchases",
			purchaseIDs: []string{"p1", "p-missing"},
			purchaseMap: map[string]*inventory.Purchase{
				"p1": makePurchaseWithMarketData("p1", "11111111", "Charizard"),
			},
			listFn: func(_ context.Context, _ bool) ([]inventory.Campaign, error) {
				return []inventory.Campaign{}, nil
			},
			wantCount: 1,
		},
		{
			name:        "repo error on GetPurchasesByIDs propagated",
			purchaseIDs: []string{"p1"},
			purchaseMap: nil, // signal error
			wantErr:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubExportReader{}
			if tc.purchaseMap != nil {
				purchaseMapCopy := tc.purchaseMap
				repo.getPurchasesByIDsFn = func(_ context.Context, _ []string) (map[string]*inventory.Purchase, error) {
					return purchaseMapCopy, nil
				}
			} else if tc.wantErr {
				repo.getPurchasesByIDsFn = func(_ context.Context, _ []string) (map[string]*inventory.Purchase, error) {
					return nil, errors.New("repo error")
				}
			}

			if tc.listFn != nil {
				repo.listCampaignsFn = tc.listFn
			}

			svc := export.New(repo)
			sheet, err := svc.GenerateSelectedSellSheet(context.Background(), tc.purchaseIDs)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sheet == nil {
				t.Fatal("expected non-nil sell sheet")
			}
			if len(sheet.Items) != tc.wantCount {
				t.Errorf("item count = %d, want %d", len(sheet.Items), tc.wantCount)
			}
		})
	}
}

// --- MatchShopifyPrices ---

func TestExportService_MatchShopifyPrices(t *testing.T) {
	tests := []struct {
		name           string
		items          []inventory.ShopifyPriceSyncItem
		purchaseMap    map[string]*inventory.Purchase
		wantMatchCount int
		wantUnmatched  []string
		wantErr        bool
	}{
		{
			name: "matches single purchase by cert number",
			items: []inventory.ShopifyPriceSyncItem{
				{CertNumber: "12345678", CurrentPriceCents: 5000},
			},
			purchaseMap: map[string]*inventory.Purchase{
				"12345678": {
					ID:         "p1",
					CertNumber: "12345678",
					CardName:   "Charizard",
					SetName:    "Base Set",
					GradeValue: 10,
					Grader:     "PSA",
					ReceivedAt: receivedAt(),
				},
			},
			wantMatchCount: 1,
			wantUnmatched:  []string{},
		},
		{
			name:           "empty items list returns empty results",
			items:          []inventory.ShopifyPriceSyncItem{},
			purchaseMap:    map[string]*inventory.Purchase{},
			wantMatchCount: 0,
			wantUnmatched:  []string{},
		},
		{
			name: "unmatched cert numbers are tracked",
			items: []inventory.ShopifyPriceSyncItem{
				{CertNumber: "11111111", CurrentPriceCents: 3000},
				{CertNumber: "22222222", CurrentPriceCents: 4000},
			},
			purchaseMap: map[string]*inventory.Purchase{
				"11111111": {
					ID:         "p1",
					CertNumber: "11111111",
					CardName:   "Pikachu",
					GradeValue: 9,
					Grader:     "PSA",
					ReceivedAt: receivedAt(),
				},
			},
			wantMatchCount: 1,
			wantUnmatched:  []string{"22222222"},
		},
		{
			name: "multiple matches",
			items: []inventory.ShopifyPriceSyncItem{
				{CertNumber: "11111111", CurrentPriceCents: 3000},
				{CertNumber: "22222222", CurrentPriceCents: 4000},
				{CertNumber: "33333333", CurrentPriceCents: 5000},
			},
			purchaseMap: map[string]*inventory.Purchase{
				"11111111": {
					ID:         "p1",
					CertNumber: "11111111",
					CardName:   "Pikachu",
					GradeValue: 9,
					Grader:     "PSA",
					ReceivedAt: receivedAt(),
				},
				"22222222": {
					ID:         "p2",
					CertNumber: "22222222",
					CardName:   "Meowth",
					GradeValue: 8,
					Grader:     "PSA",
					ReceivedAt: receivedAt(),
				},
				"33333333": {
					ID:         "p3",
					CertNumber: "33333333",
					CardName:   "Psyduck",
					GradeValue: 7,
					Grader:     "PSA",
					ReceivedAt: receivedAt(),
				},
			},
			wantMatchCount: 3,
			wantUnmatched:  []string{},
		},
		{
			name: "repo error propagated",
			items: []inventory.ShopifyPriceSyncItem{
				{CertNumber: "11111111", CurrentPriceCents: 3000},
			},
			purchaseMap: nil, // signal error
			wantErr:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubExportReader{}
			if tc.purchaseMap != nil {
				purchaseMapCopy := tc.purchaseMap
				repo.getPurchasesByCertNumbersFn = func(_ context.Context, _ []string) (map[string]*inventory.Purchase, error) {
					return purchaseMapCopy, nil
				}
			} else if tc.wantErr {
				repo.getPurchasesByCertNumbersFn = func(_ context.Context, _ []string) (map[string]*inventory.Purchase, error) {
					return nil, errors.New("repo error")
				}
			}

			svc := export.New(repo)
			resp, err := svc.MatchShopifyPrices(context.Background(), tc.items)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp == nil {
				t.Fatal("expected non-nil response")
			}
			if len(resp.Matched) != tc.wantMatchCount {
				t.Errorf("matched count = %d, want %d", len(resp.Matched), tc.wantMatchCount)
			}
			if len(resp.Unmatched) != len(tc.wantUnmatched) {
				t.Errorf("unmatched count = %d, want %d", len(resp.Unmatched), len(tc.wantUnmatched))
			}
			for _, wantCert := range tc.wantUnmatched {
				found := false
				for _, gotCert := range resp.Unmatched {
					if gotCert == wantCert {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected unmatched cert %q not found", wantCert)
				}
			}
		})
	}
}
