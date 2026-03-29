package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// --- HandleFillRate (with days param bounds validation) ---

func TestHandleFillRate(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		expectedStatus int
		expectedDays   int
	}{
		{
			name:           "default days (no param)",
			query:          "",
			expectedStatus: http.StatusOK,
			expectedDays:   30,
		},
		{
			name:           "explicit days=7",
			query:          "?days=7",
			expectedStatus: http.StatusOK,
			expectedDays:   7,
		},
		{
			name:           "days=365 (upper bound)",
			query:          "?days=365",
			expectedStatus: http.StatusOK,
			expectedDays:   365,
		},
		{
			name:           "days=1 (lower bound)",
			query:          "?days=1",
			expectedStatus: http.StatusOK,
			expectedDays:   1,
		},
		{
			name:           "days=0 falls back to default 30",
			query:          "?days=0",
			expectedStatus: http.StatusOK,
			expectedDays:   30,
		},
		{
			name:           "days=-1 falls back to default 30",
			query:          "?days=-1",
			expectedStatus: http.StatusOK,
			expectedDays:   30,
		},
		{
			name:           "days=366 exceeds max falls back to default 30",
			query:          "?days=366",
			expectedStatus: http.StatusOK,
			expectedDays:   30,
		},
		{
			name:           "days=abc non-numeric falls back to default 30",
			query:          "?days=abc",
			expectedStatus: http.StatusOK,
			expectedDays:   30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedDays int
			svc := &mocks.MockCampaignService{
				GetDailySpendFn: func(_ context.Context, _ string, days int) ([]campaigns.DailySpend, error) {
					capturedDays = days
					return []campaigns.DailySpend{{Date: "2025-01-01", SpendCents: 500}}, nil
				},
				GetCampaignFn: func(_ context.Context, _ string) (*campaigns.Campaign, error) {
					return &campaigns.Campaign{ID: "c1", DailySpendCapCents: 1000}, nil
				},
			}
			h := newTestHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/campaigns/c1/fill-rate"+tt.query, nil)
			req.SetPathValue("id", "c1")
			rec := httptest.NewRecorder()
			h.HandleFillRate(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.expectedStatus, rec.Code, rec.Body.String())
			}
			if tt.expectedStatus == http.StatusOK && capturedDays != tt.expectedDays {
				t.Errorf("expected days=%d, got %d", tt.expectedDays, capturedDays)
			}
		})
	}
}

func TestHandleFillRate_ServiceError(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetDailySpendFn: func(_ context.Context, _ string, _ int) ([]campaigns.DailySpend, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/c1/fill-rate", nil)
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleFillRate(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}

func TestHandleFillRate_MissingID(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns//fill-rate", nil)
	// No SetPathValue — simulates missing ID
	rec := httptest.NewRecorder()
	h.HandleFillRate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}

func TestHandleFillRate_EnrichesWithCap(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetDailySpendFn: func(_ context.Context, _ string, _ int) ([]campaigns.DailySpend, error) {
			return []campaigns.DailySpend{{Date: "2025-01-01", SpendCents: 500}}, nil
		},
		GetCampaignFn: func(_ context.Context, _ string) (*campaigns.Campaign, error) {
			return &campaigns.Campaign{ID: "c1", DailySpendCapCents: 1000}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/c1/fill-rate", nil)
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleFillRate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result []campaigns.DailySpend
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].CapCents != 1000 {
		t.Errorf("expected CapCents=1000, got %d", result[0].CapCents)
	}
	if result[0].FillRatePct != 0.5 {
		t.Errorf("expected FillRatePct=0.5, got %f", result[0].FillRatePct)
	}
}

// --- HandleCampaignPNL ---

func TestHandleCampaignPNL_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetCampaignPNLFn: func(_ context.Context, cid string) (*campaigns.CampaignPNL, error) {
			return &campaigns.CampaignPNL{CampaignID: cid, NetProfitCents: 1000}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/c1/pnl", nil)
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleCampaignPNL(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleCampaignPNL_ServiceError(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetCampaignPNLFn: func(_ context.Context, _ string) (*campaigns.CampaignPNL, error) {
			return nil, fmt.Errorf("internal error")
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/c1/pnl", nil)
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleCampaignPNL(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}

// --- HandleSellSheet ---

func TestHandleSellSheet_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GenerateSellSheetFn: func(_ context.Context, _ string, pids []string) (*campaigns.SellSheet, error) {
			return &campaigns.SellSheet{CampaignName: "Test", Items: make([]campaigns.SellSheetItem, len(pids))}, nil
		},
	}
	h := newTestHandler(svc)

	body := `{"purchaseIds":["p1","p2"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/sell-sheet", bytes.NewBufferString(body))
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleSellSheet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleSellSheet_EmptyPurchaseIDs(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	body := `{"purchaseIds":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/sell-sheet", bytes.NewBufferString(body))
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleSellSheet(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}

func TestHandleSellSheet_CampaignNotFound(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GenerateSellSheetFn: func(_ context.Context, _ string, _ []string) (*campaigns.SellSheet, error) {
			return nil, fmt.Errorf("campaign lookup: %w", campaigns.ErrCampaignNotFound)
		},
	}
	h := newTestHandler(svc)

	body := `{"purchaseIds":["p1"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/sell-sheet", bytes.NewBufferString(body))
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleSellSheet(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
	decodeErrorResponse(t, rec)
}

func TestHandleSellSheet_InvalidBody(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/sell-sheet", bytes.NewBufferString("{bad"))
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleSellSheet(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}

// --- HandleTuning ---

func TestHandleTuning_Success(t *testing.T) {
	resp := &campaigns.TuningResponse{
		CampaignID:   "camp-1",
		CampaignName: "Test Campaign",
		ByGrade: []campaigns.GradePerformance{
			{Grade: 9, PurchaseCount: 10, SoldCount: 7, ROI: 0.15},
		},
		Recommendations: []campaigns.TuningRecommendation{},
	}
	svc := &mocks.MockCampaignService{
		GetCampaignTuningFn: func(_ context.Context, _ string) (*campaigns.TuningResponse, error) {
			return resp, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/camp-1/tuning", nil)
	req.SetPathValue("id", "camp-1")
	rec := httptest.NewRecorder()
	h.HandleTuning(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got campaigns.TuningResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.CampaignID != "camp-1" {
		t.Errorf("campaignId = %q, want camp-1", got.CampaignID)
	}
	if len(got.ByGrade) != 1 || got.ByGrade[0].Grade != 9 {
		t.Errorf("unexpected byGrade: %+v", got.ByGrade)
	}
}

func TestHandleTuning_ServiceError(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetCampaignTuningFn: func(_ context.Context, _ string) (*campaigns.TuningResponse, error) {
			return nil, campaigns.ErrCampaignNotFound
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/camp-1/tuning", nil)
	req.SetPathValue("id", "camp-1")
	rec := httptest.NewRecorder()
	h.HandleTuning(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}

// --- HandleInventory ---

func TestHandleInventory_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetInventoryAgingFn: func(_ context.Context, _ string) ([]campaigns.AgingItem, error) {
			return []campaigns.AgingItem{{DaysHeld: 10}}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/c1/inventory", nil)
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleInventory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleInventory_ServiceError(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetInventoryAgingFn: func(_ context.Context, _ string) ([]campaigns.AgingItem, error) {
			return nil, fmt.Errorf("internal error")
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/c1/inventory", nil)
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleInventory(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}

// --- HandleDaysToSell ---

func TestHandleDaysToSell_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetDaysToSellDistFn: func(_ context.Context, _ string) ([]campaigns.DaysToSellBucket, error) {
			return []campaigns.DaysToSellBucket{{Label: "0-7", Count: 5}}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/c1/days-to-sell", nil)
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandleDaysToSell(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// --- HandlePNLByChannel ---

func TestHandlePNLByChannel_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetPNLByChannelFn: func(_ context.Context, _ string) ([]campaigns.ChannelPNL, error) {
			return []campaigns.ChannelPNL{{Channel: "ebay", SaleCount: 3}}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/c1/pnl-by-channel", nil)
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandlePNLByChannel(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandlePNLByChannel_NilReturnsEmptyArray(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetPNLByChannelFn: func(_ context.Context, _ string) ([]campaigns.ChannelPNL, error) {
			return nil, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/c1/pnl-by-channel", nil)
	req.SetPathValue("id", "c1")
	rec := httptest.NewRecorder()
	h.HandlePNLByChannel(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result []campaigns.ChannelPNL
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result == nil {
		t.Error("expected non-nil array, got nil")
	}
}

// --- HandleSelectedSellSheet ---

func TestHandleSelectedSellSheet_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GenerateSelectedSellSheetFn: func(_ context.Context, pids []string) (*campaigns.SellSheet, error) {
			return &campaigns.SellSheet{CampaignName: "All Inventory", Items: make([]campaigns.SellSheetItem, len(pids))}, nil
		},
	}
	h := newTestHandler(svc)

	body := `{"purchaseIds":["p1","p2","p3"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/portfolio/sell-sheet", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.HandleSelectedSellSheet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleSelectedSellSheet_EmptyIDs(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	body := `{"purchaseIds":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/portfolio/sell-sheet", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.HandleSelectedSellSheet(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}

func TestHandleSelectedSellSheet_InvalidBody(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	req := httptest.NewRequest(http.MethodPost, "/api/portfolio/sell-sheet", bytes.NewBufferString("{bad"))
	rec := httptest.NewRecorder()
	h.HandleSelectedSellSheet(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
