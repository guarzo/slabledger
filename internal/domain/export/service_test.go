package export_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/export"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// --- minimal ExportReader stub ---

// stubExportReader implements export.ExportReader for testing.
// Each method delegates to a function field; nil fields return safe defaults.
type stubExportReader struct {
	getPurchasesByIDsFn      func(ctx context.Context, purchaseIDs []string) (map[string]*inventory.Purchase, error)
	listAllUnsoldPurchasesFn func(ctx context.Context) ([]inventory.Purchase, error)
	getCampaignFn            func(ctx context.Context, id string) (*inventory.Campaign, error)
	listCampaignsFn          func(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error)
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

