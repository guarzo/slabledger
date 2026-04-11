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

func TestHandleInventory(t *testing.T) {
	tests := []struct {
		name           string
		agingFn        func(context.Context, string) (*campaigns.InventoryResult, error)
		expectedStatus int
		checkBody      bool
	}{
		{
			name: "success with items and warnings",
			agingFn: func(_ context.Context, _ string) (*campaigns.InventoryResult, error) {
				return &campaigns.InventoryResult{
					Items:    []campaigns.AgingItem{{DaysHeld: 10}},
					Warnings: []string{"Price flag data unavailable"},
				}, nil
			},
			expectedStatus: http.StatusOK,
			checkBody:      true,
		},
		{
			name: "service error",
			agingFn: func(_ context.Context, _ string) (*campaigns.InventoryResult, error) {
				return nil, fmt.Errorf("internal error")
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{
				GetInventoryAgingFn: tt.agingFn,
			}
			h := newTestHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/campaigns/c1/inventory", nil)
			req.SetPathValue("id", "c1")
			rec := httptest.NewRecorder()
			h.HandleInventory(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.expectedStatus, rec.Code, rec.Body.String())
			}
			if tt.checkBody {
				var result campaigns.InventoryResult
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if len(result.Items) != 1 {
					t.Errorf("expected 1 item, got %d", len(result.Items))
				}
				if len(result.Warnings) == 0 {
					t.Error("expected non-empty warnings")
				}
			} else {
				decodeErrorResponse(t, rec)
			}
		})
	}
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
	decodeErrorResponse(t, rec)
}

func TestHandleSelectedSellSheet_TooManyIDs(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	ids := make([]string, 5001)
	for i := range ids {
		ids[i] = fmt.Sprintf("id-%d", i)
	}
	body, _ := json.Marshal(map[string]any{"purchaseIds": ids})
	req := httptest.NewRequest(http.MethodPost, "/api/portfolio/sell-sheet", bytes.NewBuffer(body))
	rec := httptest.NewRecorder()
	h.HandleSelectedSellSheet(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// --- HandleGlobalInventory ---

func TestHandleGlobalInventory(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		setupSvc func() *mocks.MockCampaignService
		wantCode int
		check    func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name:   "success",
			method: http.MethodGet,
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					GetGlobalInventoryAgingFn: func(_ context.Context) (*campaigns.InventoryResult, error) {
						return &campaigns.InventoryResult{
							Items:    []campaigns.AgingItem{{DaysHeld: 5}},
							Warnings: []string{},
						}, nil
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var result campaigns.InventoryResult
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if len(result.Items) != 1 {
					t.Errorf("expected 1 item, got %d", len(result.Items))
				}
			},
		},
		{
			name:     "method not allowed",
			method:   http.MethodPost,
			setupSvc: func() *mocks.MockCampaignService { return &mocks.MockCampaignService{} },
			wantCode: http.StatusMethodNotAllowed,
		},
		{
			name:   "service error",
			method: http.MethodGet,
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					GetGlobalInventoryAgingFn: func(_ context.Context) (*campaigns.InventoryResult, error) {
						return nil, fmt.Errorf("database error")
					},
				}
			},
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(tc.setupSvc())
			req := httptest.NewRequest(tc.method, "/api/inventory", nil)
			rec := httptest.NewRecorder()
			h.HandleGlobalInventory(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d; body: %s", tc.wantCode, rec.Code, rec.Body.String())
			}
			if tc.check != nil {
				tc.check(t, rec)
			}
		})
	}
}

// --- HandleGlobalSellSheet ---

func TestHandleGlobalSellSheet(t *testing.T) {
	tests := []struct {
		name     string
		setupSvc func() *mocks.MockCampaignService
		wantCode int
		check    func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name: "success",
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					GenerateGlobalSellSheetFn: func(_ context.Context) (*campaigns.SellSheet, error) {
						return &campaigns.SellSheet{CampaignName: "Global", Items: []campaigns.SellSheetItem{}}, nil
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var result campaigns.SellSheet
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if result.CampaignName != "Global" {
					t.Errorf("expected CampaignName=Global, got %q", result.CampaignName)
				}
			},
		},
		{
			name: "service error",
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					GenerateGlobalSellSheetFn: func(_ context.Context) (*campaigns.SellSheet, error) {
						return nil, fmt.Errorf("sheet generation failed")
					},
				}
			},
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(tc.setupSvc())
			req := httptest.NewRequest(http.MethodPost, "/api/sell-sheet", nil)
			rec := httptest.NewRecorder()
			h.HandleGlobalSellSheet(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d; body: %s", tc.wantCode, rec.Code, rec.Body.String())
			}
			if tc.check != nil {
				tc.check(t, rec)
			}
		})
	}
}

// --- HandleCrackCandidates ---

func TestHandleCrackCandidates(t *testing.T) {
	tests := []struct {
		name     string
		pathID   string
		setupSvc func() *mocks.MockCampaignService
		wantCode int
		check    func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name:   "success",
			pathID: "c1",
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					GetCrackCandidatesFn: func(_ context.Context, id string) ([]campaigns.CrackAnalysis, error) {
						return []campaigns.CrackAnalysis{{PurchaseID: "p1"}}, nil
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var result []campaigns.CrackAnalysis
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if len(result) != 1 {
					t.Errorf("expected 1 candidate, got %d", len(result))
				}
			},
		},
		{
			name:   "campaign not found",
			pathID: "c1",
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					GetCrackCandidatesFn: func(_ context.Context, _ string) ([]campaigns.CrackAnalysis, error) {
						return nil, campaigns.ErrCampaignNotFound
					},
				}
			},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "missing id",
			pathID:   "", // no SetPathValue
			setupSvc: func() *mocks.MockCampaignService { return &mocks.MockCampaignService{} },
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(tc.setupSvc())
			req := httptest.NewRequest(http.MethodGet, "/api/campaigns/c1/crack-candidates", nil)
			if tc.pathID != "" {
				req.SetPathValue("id", tc.pathID)
			}
			rec := httptest.NewRecorder()
			h.HandleCrackCandidates(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d; body: %s", tc.wantCode, rec.Code, rec.Body.String())
			}
			if tc.check != nil {
				tc.check(t, rec)
			}
		})
	}
}

// --- HandleExpectedValues ---

func TestHandleExpectedValues(t *testing.T) {
	tests := []struct {
		name     string
		setupSvc func() *mocks.MockCampaignService
		wantCode int
	}{
		{
			name: "success",
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					GetExpectedValuesFn: func(_ context.Context, id string) (*campaigns.EVPortfolio, error) {
						return &campaigns.EVPortfolio{}, nil
					},
				}
			},
			wantCode: http.StatusOK,
		},
		{
			name: "service error",
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					GetExpectedValuesFn: func(_ context.Context, _ string) (*campaigns.EVPortfolio, error) {
						return nil, fmt.Errorf("internal error")
					},
				}
			},
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(tc.setupSvc())
			req := httptest.NewRequest(http.MethodGet, "/api/campaigns/c1/expected-values", nil)
			req.SetPathValue("id", "c1")
			rec := httptest.NewRecorder()
			h.HandleExpectedValues(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d; body: %s", tc.wantCode, rec.Code, rec.Body.String())
			}
		})
	}
}

// --- HandleEvaluatePurchase ---

func TestHandleEvaluatePurchase(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		setupSvc func() *mocks.MockCampaignService
		wantCode int
	}{
		{
			name: "success",
			body: `{"cardName":"Charizard","grade":9,"buyCostCents":5000}`,
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					EvaluatePurchaseFn: func(_ context.Context, id, cardName string, grade float64, buyCostCents int) (*campaigns.ExpectedValue, error) {
						return &campaigns.ExpectedValue{CardName: cardName}, nil
					},
				}
			},
			wantCode: http.StatusOK,
		},
		{
			name:     "missing cardName",
			body:     `{"cardName":"","grade":9,"buyCostCents":5000}`,
			setupSvc: func() *mocks.MockCampaignService { return &mocks.MockCampaignService{} },
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "grade below 1",
			body:     `{"cardName":"Charizard","grade":0,"buyCostCents":5000}`,
			setupSvc: func() *mocks.MockCampaignService { return &mocks.MockCampaignService{} },
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "grade above 10",
			body:     `{"cardName":"Charizard","grade":11,"buyCostCents":5000}`,
			setupSvc: func() *mocks.MockCampaignService { return &mocks.MockCampaignService{} },
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "negative buyCostCents",
			body:     `{"cardName":"Charizard","grade":9,"buyCostCents":-1}`,
			setupSvc: func() *mocks.MockCampaignService { return &mocks.MockCampaignService{} },
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "invalid json",
			body:     `{bad`,
			setupSvc: func() *mocks.MockCampaignService { return &mocks.MockCampaignService{} },
			wantCode: http.StatusBadRequest,
		},
		{
			name: "campaign not found",
			body: `{"cardName":"Charizard","grade":9,"buyCostCents":5000}`,
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					EvaluatePurchaseFn: func(_ context.Context, _ string, _ string, _ float64, _ int) (*campaigns.ExpectedValue, error) {
						return nil, campaigns.ErrCampaignNotFound
					},
				}
			},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(tc.setupSvc())
			req := httptest.NewRequest(http.MethodPost, "/api/campaigns/c1/evaluate-purchase", bytes.NewBufferString(tc.body))
			req.SetPathValue("id", "c1")
			rec := httptest.NewRecorder()
			h.HandleEvaluatePurchase(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d; body: %s", tc.wantCode, rec.Code, rec.Body.String())
			}
		})
	}
}

// --- HandleActivationChecklist ---

func TestHandleActivationChecklist(t *testing.T) {
	tests := []struct {
		name     string
		setupSvc func() *mocks.MockCampaignService
		wantCode int
	}{
		{
			name: "success",
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					GetActivationChecklistFn: func(_ context.Context, id string) (*campaigns.ActivationChecklist, error) {
						return &campaigns.ActivationChecklist{}, nil
					},
				}
			},
			wantCode: http.StatusOK,
		},
		{
			name: "campaign not found",
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					GetActivationChecklistFn: func(_ context.Context, _ string) (*campaigns.ActivationChecklist, error) {
						return nil, campaigns.ErrCampaignNotFound
					},
				}
			},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(tc.setupSvc())
			req := httptest.NewRequest(http.MethodGet, "/api/campaigns/c1/activation-checklist", nil)
			req.SetPathValue("id", "c1")
			rec := httptest.NewRecorder()
			h.HandleActivationChecklist(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d; body: %s", tc.wantCode, rec.Code, rec.Body.String())
			}
		})
	}
}

// --- HandleProjections ---

func TestHandleProjections(t *testing.T) {
	tests := []struct {
		name     string
		setupSvc func() *mocks.MockCampaignService
		wantCode int
	}{
		{
			name: "success",
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					RunProjectionFn: func(_ context.Context, id string) (*campaigns.MonteCarloComparison, error) {
						return &campaigns.MonteCarloComparison{}, nil
					},
				}
			},
			wantCode: http.StatusOK,
		},
		{
			name: "campaign not found",
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					RunProjectionFn: func(_ context.Context, _ string) (*campaigns.MonteCarloComparison, error) {
						return nil, campaigns.ErrCampaignNotFound
					},
				}
			},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(tc.setupSvc())
			req := httptest.NewRequest(http.MethodGet, "/api/campaigns/c1/projections", nil)
			req.SetPathValue("id", "c1")
			rec := httptest.NewRecorder()
			h.HandleProjections(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d; body: %s", tc.wantCode, rec.Code, rec.Body.String())
			}
		})
	}
}

// --- HandleSetReviewedPrice ---

func TestHandleSetReviewedPrice(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		setupSvc func() *mocks.MockCampaignService
		wantCode int
		check    func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name: "success",
			body: `{"priceCents":1000,"source":"market"}`,
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					SetReviewedPriceFn: func(_ context.Context, purchaseID string, priceCents int, source string) error {
						return nil
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var result map[string]any
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if result["success"] != true {
					t.Errorf("expected success=true, got %v", result["success"])
				}
				if _, ok := result["reviewedAt"]; !ok {
					t.Error("expected reviewedAt field in response")
				}
			},
		},
		{
			name: "purchase not found",
			body: `{"priceCents":1000,"source":"market"}`,
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					SetReviewedPriceFn: func(_ context.Context, _ string, _ int, _ string) error {
						return campaigns.ErrPurchaseNotFound
					},
				}
			},
			wantCode: http.StatusNotFound,
		},
		{
			name: "validation error",
			body: `{"priceCents":1000,"source":"market"}`,
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					SetReviewedPriceFn: func(_ context.Context, _ string, _ int, _ string) error {
						return campaigns.ErrCampaignNameRequired
					},
				}
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "invalid body",
			body:     `{bad`,
			setupSvc: func() *mocks.MockCampaignService { return &mocks.MockCampaignService{} },
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(tc.setupSvc())
			req := httptest.NewRequest(http.MethodPatch, "/api/purchases/p1/review-price", bytes.NewBufferString(tc.body))
			req.SetPathValue("purchaseId", "p1")
			rec := httptest.NewRecorder()
			h.HandleSetReviewedPrice(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d; body: %s", tc.wantCode, rec.Code, rec.Body.String())
			}
			if tc.check != nil {
				tc.check(t, rec)
			}
		})
	}
}

// --- HandleCreatePriceFlag ---

func TestHandleCreatePriceFlag(t *testing.T) {
	tests := []struct {
		name     string
		withAuth bool
		setupSvc func() *mocks.MockCampaignService
		wantCode int
		check    func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name:     "success",
			withAuth: true,
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					CreatePriceFlagFn: func(_ context.Context, purchaseID string, userID int64, reason string) (int64, error) {
						return 101, nil
					},
				}
			},
			wantCode: http.StatusCreated,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var result map[string]any
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if _, ok := result["id"]; !ok {
					t.Error("expected id field in response")
				}
				if _, ok := result["flaggedAt"]; !ok {
					t.Error("expected flaggedAt field in response")
				}
			},
		},
		{
			name:     "requires user",
			withAuth: false,
			setupSvc: func() *mocks.MockCampaignService { return &mocks.MockCampaignService{} },
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "purchase not found",
			withAuth: true,
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					CreatePriceFlagFn: func(_ context.Context, _ string, _ int64, _ string) (int64, error) {
						return 0, campaigns.ErrPurchaseNotFound
					},
				}
			},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(tc.setupSvc())
			body := `{"reason":"price seems off"}`
			req := httptest.NewRequest(http.MethodPost, "/api/purchases/p1/flag", bytes.NewBufferString(body))
			if tc.withAuth {
				req = withUser(req)
			}
			req.SetPathValue("purchaseId", "p1")
			rec := httptest.NewRecorder()
			h.HandleCreatePriceFlag(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d; body: %s", tc.wantCode, rec.Code, rec.Body.String())
			}
			if tc.check != nil {
				tc.check(t, rec)
			}
		})
	}
}

// --- HandleShopifyPriceSync ---

func TestHandleShopifyPriceSync(t *testing.T) {
	tests := []struct {
		name     string
		body     func() []byte
		setupSvc func() *mocks.MockCampaignService
		wantCode int
		check    func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name: "success",
			body: func() []byte {
				return []byte(`{"items":[{"certNumber":"12345","grader":"PSA","currentPriceCents":1000}]}`)
			},
			setupSvc: func() *mocks.MockCampaignService {
				return &mocks.MockCampaignService{
					MatchShopifyPricesFn: func(_ context.Context, items []campaigns.ShopifyPriceSyncItem) (*campaigns.ShopifyPriceSyncResponse, error) {
						return &campaigns.ShopifyPriceSyncResponse{
							Matched: []campaigns.ShopifyPriceSyncMatch{{CertNumber: "12345"}},
						}, nil
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var result campaigns.ShopifyPriceSyncResponse
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if len(result.Matched) != 1 {
					t.Errorf("expected 1 matched item, got %d", len(result.Matched))
				}
			},
		},
		{
			name:     "empty items",
			body:     func() []byte { return []byte(`{"items":[]}`) },
			setupSvc: func() *mocks.MockCampaignService { return &mocks.MockCampaignService{} },
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "invalid json",
			body:     func() []byte { return []byte(`{bad`) },
			setupSvc: func() *mocks.MockCampaignService { return &mocks.MockCampaignService{} },
			wantCode: http.StatusBadRequest,
		},
		{
			name: "too many items",
			body: func() []byte {
				items := make([]campaigns.ShopifyPriceSyncItem, 5001)
				b, _ := json.Marshal(map[string]any{"items": items})
				return b
			},
			setupSvc: func() *mocks.MockCampaignService { return &mocks.MockCampaignService{} },
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(tc.setupSvc())
			req := httptest.NewRequest(http.MethodPost, "/api/shopify/price-sync", bytes.NewBuffer(tc.body()))
			rec := httptest.NewRecorder()
			h.HandleShopifyPriceSync(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d; body: %s", tc.wantCode, rec.Code, rec.Body.String())
			}
			if tc.check != nil {
				tc.check(t, rec)
			}
		})
	}
}
