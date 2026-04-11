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
	"github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestHandleCapitalSummary(t *testing.T) {
	tests := []struct {
		name                 string
		mockFn               func(_ context.Context) (*campaigns.CapitalSummary, error)
		wantStatus           int
		wantOutstandingCents int
		wantWeeksToCover     float64
		wantAlertLevel       campaigns.AlertLevel
	}{
		{
			name: "success",
			mockFn: func(_ context.Context) (*campaigns.CapitalSummary, error) {
				return &campaigns.CapitalSummary{OutstandingCents: 2500000, WeeksToCover: 12.5, AlertLevel: campaigns.AlertOK}, nil
			},
			wantStatus:           http.StatusOK,
			wantOutstandingCents: 2500000,
			wantWeeksToCover:     12.5,
			wantAlertLevel:       campaigns.AlertOK,
		},
		{
			name: "database error",
			mockFn: func(_ context.Context) (*campaigns.CapitalSummary, error) {
				return nil, fmt.Errorf("database error")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{GetCapitalSummaryFn: tt.mockFn}
			h := newTestHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/credit/summary", nil)
			rec := httptest.NewRecorder()
			h.HandleCapitalSummary(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
			if tt.wantStatus == http.StatusOK {
				var result campaigns.CapitalSummary
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if result.OutstandingCents != tt.wantOutstandingCents {
					t.Errorf("expected OutstandingCents=%d, got %d", tt.wantOutstandingCents, result.OutstandingCents)
				}
				if result.WeeksToCover != tt.wantWeeksToCover {
					t.Errorf("expected WeeksToCover=%v, got %v", tt.wantWeeksToCover, result.WeeksToCover)
				}
				if result.AlertLevel != tt.wantAlertLevel {
					t.Errorf("expected AlertLevel=%s, got %s", tt.wantAlertLevel, result.AlertLevel)
				}
			} else {
				decodeErrorResponse(t, rec)
			}
		})
	}
}

func TestHandleGetCashflowConfig(t *testing.T) {
	tests := []struct {
		name       string
		mockFn     func(_ context.Context) (*campaigns.CashflowConfig, error)
		wantStatus int
	}{
		{
			name: "success",
			mockFn: func(_ context.Context) (*campaigns.CashflowConfig, error) {
				return &campaigns.CashflowConfig{CapitalBudgetCents: 5000000, CashBufferCents: 1000000}, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "error",
			mockFn: func(_ context.Context) (*campaigns.CashflowConfig, error) {
				return nil, fmt.Errorf("db error")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{GetCashflowConfigFn: tt.mockFn}
			h := newTestHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/credit/config", nil)
			rec := httptest.NewRecorder()
			h.HandleGetCashflowConfig(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
		})
	}
}

func TestHandleListInvoices(t *testing.T) {
	tests := []struct {
		name             string
		mockFn           func(_ context.Context) ([]campaigns.Invoice, error)
		wantStatus       int
		wantCount        int
		checkNotNil      bool
		wantPendingCents int
	}{
		{
			name: "success - one invoice with pending receipt",
			mockFn: func(_ context.Context) ([]campaigns.Invoice, error) {
				return []campaigns.Invoice{
					{
						ID: "inv-1", InvoiceDate: "2026-01-15",
						TotalCents: 10000, PaidCents: 10000,
						PendingReceiptCents: 3500,
						Status:              "paid",
					},
				}, nil
			},
			wantStatus:       http.StatusOK,
			wantCount:        1,
			wantPendingCents: 3500,
		},
		{
			name: "empty",
			mockFn: func(_ context.Context) ([]campaigns.Invoice, error) {
				return nil, nil
			},
			wantStatus:  http.StatusOK,
			checkNotNil: true,
		},
		{
			name: "error",
			mockFn: func(_ context.Context) ([]campaigns.Invoice, error) {
				return nil, fmt.Errorf("db error")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{ListInvoicesFn: tt.mockFn}
			h := newTestHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/credit/invoices", nil)
			rec := httptest.NewRecorder()
			h.HandleListInvoices(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
			if tt.wantStatus == http.StatusOK {
				var result []campaigns.Invoice
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if tt.wantCount > 0 && len(result) != tt.wantCount {
					t.Errorf("expected %d invoice(s), got %d", tt.wantCount, len(result))
				}
				if tt.checkNotNil && result == nil {
					t.Error("expected empty array, got nil")
				}
				if tt.wantStatus == http.StatusOK && len(result) > 0 && tt.wantPendingCents > 0 {
					if result[0].PendingReceiptCents != tt.wantPendingCents {
						t.Errorf("expected PendingReceiptCents=%d, got %d", tt.wantPendingCents, result[0].PendingReceiptCents)
					}
				}
			}
		})
	}
}

func TestHandleUpdateInvoice(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		mockFn     func(_ context.Context, _ *campaigns.Invoice) error
		wantStatus int
	}{
		{
			name: "success",
			body: `{"id":"inv-1","totalCents":10000}`,
			mockFn: func(_ context.Context, _ *campaigns.Invoice) error {
				return nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "not found",
			body: `{"id":"missing"}`,
			mockFn: func(_ context.Context, _ *campaigns.Invoice) error {
				return campaigns.ErrInvoiceNotFound
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "invalid body",
			body:       "{bad",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{}
			if tt.mockFn != nil {
				svc.UpdateInvoiceFn = tt.mockFn
			}
			h := newTestHandler(svc)

			req := httptest.NewRequest(http.MethodPut, "/api/credit/invoices", bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()
			h.HandleUpdateInvoice(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandlePortfolioHealth(t *testing.T) {
	tests := []struct {
		name       string
		mockFn     func(_ context.Context) (*campaigns.PortfolioHealth, error)
		wantStatus int
	}{
		{
			name: "success",
			mockFn: func(_ context.Context) (*campaigns.PortfolioHealth, error) {
				return &campaigns.PortfolioHealth{}, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "error",
			mockFn: func(_ context.Context) (*campaigns.PortfolioHealth, error) {
				return nil, fmt.Errorf("service error")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{GetPortfolioHealthFn: tt.mockFn}
			h := newTestHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/portfolio/health", nil)
			rec := httptest.NewRecorder()
			h.HandlePortfolioHealth(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
		})
	}
}

func TestHandlePortfolioChannelVelocity(t *testing.T) {
	tests := []struct {
		name       string
		mockFn     func(_ context.Context) ([]campaigns.ChannelVelocity, error)
		wantStatus int
	}{
		{
			name: "success",
			mockFn: func(_ context.Context) ([]campaigns.ChannelVelocity, error) {
				return []campaigns.ChannelVelocity{{Channel: "ebay"}}, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "error",
			mockFn: func(_ context.Context) ([]campaigns.ChannelVelocity, error) {
				return nil, fmt.Errorf("service error")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{GetPortfolioChannelVelocityFn: tt.mockFn}
			h := newTestHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/portfolio/channel-velocity", nil)
			rec := httptest.NewRecorder()
			h.HandlePortfolioChannelVelocity(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
		})
	}
}

func TestHandlePortfolioInsights(t *testing.T) {
	tests := []struct {
		name       string
		mockFn     func(_ context.Context) (*campaigns.PortfolioInsights, error)
		wantStatus int
	}{
		{
			name: "success",
			mockFn: func(_ context.Context) (*campaigns.PortfolioInsights, error) {
				return &campaigns.PortfolioInsights{}, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "error",
			mockFn: func(_ context.Context) (*campaigns.PortfolioInsights, error) {
				return nil, fmt.Errorf("service error")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{GetPortfolioInsightsFn: tt.mockFn}
			h := newTestHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/portfolio/insights", nil)
			rec := httptest.NewRecorder()
			h.HandlePortfolioInsights(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
		})
	}
}

func TestHandleCampaignSuggestions(t *testing.T) {
	tests := []struct {
		name       string
		mockFn     func(_ context.Context) (*campaigns.SuggestionsResponse, error)
		wantStatus int
	}{
		{
			name: "success",
			mockFn: func(_ context.Context) (*campaigns.SuggestionsResponse, error) {
				return &campaigns.SuggestionsResponse{}, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "error",
			mockFn: func(_ context.Context) (*campaigns.SuggestionsResponse, error) {
				return nil, fmt.Errorf("service error")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{GetCampaignSuggestionsFn: tt.mockFn}
			h := newTestHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/portfolio/suggestions", nil)
			rec := httptest.NewRecorder()
			h.HandleCampaignSuggestions(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
		})
	}
}

func TestHandleCapitalTimeline(t *testing.T) {
	tests := []struct {
		name       string
		mockFn     func(_ context.Context) (*campaigns.CapitalTimeline, error)
		wantStatus int
	}{
		{
			name: "success",
			mockFn: func(_ context.Context) (*campaigns.CapitalTimeline, error) {
				return &campaigns.CapitalTimeline{}, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "error",
			mockFn: func(_ context.Context) (*campaigns.CapitalTimeline, error) {
				return nil, fmt.Errorf("service error")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{GetCapitalTimelineFn: tt.mockFn}
			h := newTestHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/portfolio/capital-timeline", nil)
			rec := httptest.NewRecorder()
			h.HandleCapitalTimeline(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
		})
	}
}

func TestHandleWeeklyReview(t *testing.T) {
	tests := []struct {
		name       string
		mockFn     func(_ context.Context) (*campaigns.WeeklyReviewSummary, error)
		wantStatus int
	}{
		{
			name: "success",
			mockFn: func(_ context.Context) (*campaigns.WeeklyReviewSummary, error) {
				return &campaigns.WeeklyReviewSummary{}, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "error",
			mockFn: func(_ context.Context) (*campaigns.WeeklyReviewSummary, error) {
				return nil, fmt.Errorf("service error")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{GetWeeklyReviewSummaryFn: tt.mockFn}
			h := newTestHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/portfolio/weekly-review", nil)
			rec := httptest.NewRecorder()
			h.HandleWeeklyReview(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
		})
	}
}

func TestHandleListRevocationFlags(t *testing.T) {
	tests := []struct {
		name        string
		mockFn      func(_ context.Context) ([]campaigns.RevocationFlag, error)
		wantStatus  int
		wantCount   int
		checkNotNil bool
	}{
		{
			name: "success",
			mockFn: func(_ context.Context) ([]campaigns.RevocationFlag, error) {
				return []campaigns.RevocationFlag{{ID: "rf-1"}}, nil
			},
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name: "empty",
			mockFn: func(_ context.Context) ([]campaigns.RevocationFlag, error) {
				return nil, nil
			},
			wantStatus:  http.StatusOK,
			checkNotNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{ListRevocationFlagsFn: tt.mockFn}
			h := newTestHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/portfolio/revocations", nil)
			rec := httptest.NewRecorder()
			h.HandleListRevocationFlags(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
			if tt.wantStatus == http.StatusOK {
				var result []campaigns.RevocationFlag
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if tt.wantCount > 0 && len(result) != tt.wantCount {
					t.Errorf("expected %d flag(s), got %d", tt.wantCount, len(result))
				}
				if tt.checkNotNil && result == nil {
					t.Error("expected empty array, got nil")
				}
			}
		})
	}
}

func TestHandleCreateRevocationFlag(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		mockFn     func(_ context.Context, label, dim, reason string) (*campaigns.RevocationFlag, error)
		wantStatus int
	}{
		{
			name: "success",
			body: `{"segmentLabel":"low-margin","segmentDimension":"channel","reason":"underperforming"}`,
			mockFn: func(_ context.Context, label, dim, reason string) (*campaigns.RevocationFlag, error) {
				return &campaigns.RevocationFlag{SegmentLabel: label, SegmentDimension: dim, Reason: reason}, nil
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "too soon",
			body: `{"segmentLabel":"x","segmentDimension":"y","reason":"z"}`,
			mockFn: func(_ context.Context, _, _, _ string) (*campaigns.RevocationFlag, error) {
				return nil, errors.NewAppError(campaigns.ErrCodeRevocationTooSoon, "revocation already submitted within the past 7 days")
			},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "invalid body",
			body:       "{bad",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{}
			if tt.mockFn != nil {
				svc.FlagForRevocationFn = tt.mockFn
			}
			h := newTestHandler(svc)

			req := httptest.NewRequest(http.MethodPost, "/api/portfolio/revocations", bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()
			h.HandleCreateRevocationFlag(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d; body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleRevocationEmail(t *testing.T) {
	tests := []struct {
		name       string
		flagID     string
		mockFn     func(_ context.Context, flagID string) (string, error)
		wantStatus int
		wantBody   bool
	}{
		{
			name:   "success",
			flagID: "rf-1",
			mockFn: func(_ context.Context, _ string) (string, error) {
				return "Dear partner, we are revoking...", nil
			},
			wantStatus: http.StatusOK,
			wantBody:   true,
		},
		{
			name:       "missing flag ID",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "error",
			flagID: "rf-1",
			mockFn: func(_ context.Context, _ string) (string, error) {
				return "", fmt.Errorf("generation failed")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{}
			if tt.mockFn != nil {
				svc.GenerateRevocationEmailFn = tt.mockFn
			}
			h := newTestHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/portfolio/revocations/"+tt.flagID+"/email", nil)
			if tt.flagID != "" {
				req.SetPathValue("flagId", tt.flagID)
			}
			rec := httptest.NewRecorder()
			h.HandleRevocationEmail(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
			if tt.wantBody {
				var result map[string]string
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if result["emailText"] == "" {
					t.Error("expected non-empty emailText")
				}
			}
		})
	}
}
