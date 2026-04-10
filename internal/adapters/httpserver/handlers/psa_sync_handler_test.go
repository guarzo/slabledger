package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// mockPSASyncRefresher is a test mock for handlers.PSASyncRefresher.
type mockPSASyncRefresher struct {
	RunOnceFn         func(ctx context.Context) error
	GetLastRunStatsFn func() *scheduler.PSASyncRunStats
}

func (m *mockPSASyncRefresher) RunOnce(ctx context.Context) error {
	if m.RunOnceFn != nil {
		return m.RunOnceFn(ctx)
	}
	return nil
}

func (m *mockPSASyncRefresher) GetLastRunStats() *scheduler.PSASyncRunStats {
	if m.GetLastRunStatsFn != nil {
		return m.GetLastRunStatsFn()
	}
	return nil
}

func TestPSASyncHandler_HandleStatus(t *testing.T) {
	pendingRepo := &mocks.MockPendingItemRepository{
		CountPendingItemsFn: func(ctx context.Context) (int, error) { return 5, nil },
	}
	h := handlers.NewPSASyncHandler(handlers.PSASyncHandlerConfig{
		PendingRepo:   pendingRepo,
		SpreadsheetID: "sheet-123",
		Interval:      "24h0m0s",
		Logger:        mocks.NewMockLogger(),
	})
	req := httptest.NewRequest("GET", "/api/admin/psa-sync/status", nil)
	rr := httptest.NewRecorder()
	h.HandleStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["configured"] != true {
		t.Error("expected configured=true")
	}
	if resp["pendingCount"] != float64(5) {
		t.Errorf("expected pendingCount=5, got %v", resp["pendingCount"])
	}
}

func TestPSASyncHandler_HandleListPendingItems(t *testing.T) {
	pendingRepo := &mocks.MockPendingItemRepository{
		ListPendingItemsFn: func(ctx context.Context) ([]campaigns.PendingItem, error) {
			return []campaigns.PendingItem{
				{ID: "pi-1", CertNumber: "CERT001", Status: "ambiguous", Candidates: []string{"c1", "c2"}},
			}, nil
		},
	}
	h := handlers.NewPSASyncHandler(handlers.PSASyncHandlerConfig{
		PendingRepo: pendingRepo,
		Logger:      mocks.NewMockLogger(),
	})
	req := httptest.NewRequest("GET", "/api/purchases/psa-pending", nil)
	rr := httptest.NewRecorder()
	h.HandleListPendingItems(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp struct {
		Items []campaigns.PendingItem `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
}

func TestPSASyncHandler_HandleAssignPendingItem(t *testing.T) {
	resolved := false
	pendingRepo := &mocks.MockPendingItemRepository{
		GetPendingItemByIDFn: func(ctx context.Context, id string) (*campaigns.PendingItem, error) {
			if id == "pi-1" {
				return &campaigns.PendingItem{
					ID: "pi-1", CertNumber: "CERT001", CardName: "Charizard",
					Grade: 10, BuyCostCents: 1500, PurchaseDate: "2026-03-15",
					Status: "ambiguous", Candidates: []string{"c1", "c2"},
				}, nil
			}
			return nil, campaigns.ErrPendingItemNotFound
		},
		ResolvePendingItemFn: func(ctx context.Context, id, campaignID string) error {
			resolved = true
			return nil
		},
	}
	svc := &mocks.MockCampaignService{
		CreatePurchaseFn: func(ctx context.Context, p *campaigns.Purchase) error {
			p.ID = "new-purchase"
			return nil
		},
	}
	h := handlers.NewPSASyncHandler(handlers.PSASyncHandlerConfig{
		PendingRepo: pendingRepo,
		Service:     svc,
		Logger:      mocks.NewMockLogger(),
	})
	body := `{"campaignId": "c1"}`
	req := httptest.NewRequest("POST", "/api/purchases/psa-pending/pi-1/assign", strings.NewReader(body))
	req.SetPathValue("id", "pi-1")
	rr := httptest.NewRecorder()
	h.HandleAssignPendingItem(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !resolved {
		t.Error("expected pending item to be resolved")
	}
}

func TestPSASyncHandler_HandleDismissPendingItem(t *testing.T) {
	dismissed := false
	pendingRepo := &mocks.MockPendingItemRepository{
		DismissPendingItemFn: func(ctx context.Context, id string) error {
			dismissed = true
			return nil
		},
	}
	h := handlers.NewPSASyncHandler(handlers.PSASyncHandlerConfig{
		PendingRepo: pendingRepo,
		Logger:      mocks.NewMockLogger(),
	})
	req := httptest.NewRequest("DELETE", "/api/purchases/psa-pending/pi-1", nil)
	req.SetPathValue("id", "pi-1")
	rr := httptest.NewRecorder()
	h.HandleDismissPendingItem(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
	if !dismissed {
		t.Error("expected pending item to be dismissed")
	}
}

func TestPSASyncHandler_HandleRefresh_Success(t *testing.T) {
	refresher := &mockPSASyncRefresher{
		RunOnceFn: func(ctx context.Context) error { return nil },
	}
	h := handlers.NewPSASyncHandler(handlers.PSASyncHandlerConfig{
		PendingRepo: &mocks.MockPendingItemRepository{},
		Refresher:   refresher,
		Logger:      mocks.NewMockLogger(),
	})
	req := httptest.NewRequest("POST", "/api/admin/psa-sync/refresh", nil)
	rr := httptest.NewRecorder()
	h.HandleRefresh(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPSASyncHandler_HandleRefresh_NoRefresher(t *testing.T) {
	h := handlers.NewPSASyncHandler(handlers.PSASyncHandlerConfig{
		PendingRepo: &mocks.MockPendingItemRepository{},
		Logger:      mocks.NewMockLogger(),
	})
	req := httptest.NewRequest("POST", "/api/admin/psa-sync/refresh", nil)
	rr := httptest.NewRecorder()
	h.HandleRefresh(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPSASyncHandler_HandleRefresh_Error(t *testing.T) {
	refresher := &mockPSASyncRefresher{
		RunOnceFn: func(ctx context.Context) error { return errors.New("sync failed") },
	}
	h := handlers.NewPSASyncHandler(handlers.PSASyncHandlerConfig{
		PendingRepo: &mocks.MockPendingItemRepository{},
		Refresher:   refresher,
		Logger:      mocks.NewMockLogger(),
	})
	req := httptest.NewRequest("POST", "/api/admin/psa-sync/refresh", nil)
	rr := httptest.NewRecorder()
	h.HandleRefresh(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPSASyncHandler_HandleAssignPendingItem_NilService(t *testing.T) {
	pendingRepo := &mocks.MockPendingItemRepository{
		GetPendingItemByIDFn: func(ctx context.Context, id string) (*campaigns.PendingItem, error) {
			return &campaigns.PendingItem{
				ID: "pi-1", CertNumber: "CERT001", CardName: "Charizard",
				Grade: 10, BuyCostCents: 1500, PurchaseDate: "2026-03-15",
			}, nil
		},
	}
	// Service intentionally nil
	h := handlers.NewPSASyncHandler(handlers.PSASyncHandlerConfig{
		PendingRepo: pendingRepo,
		Logger:      mocks.NewMockLogger(),
	})
	body := `{"campaignId": "c1"}`
	req := httptest.NewRequest("POST", "/api/purchases/psa-pending/pi-1/assign", strings.NewReader(body))
	req.SetPathValue("id", "pi-1")
	rr := httptest.NewRecorder()
	h.HandleAssignPendingItem(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}
