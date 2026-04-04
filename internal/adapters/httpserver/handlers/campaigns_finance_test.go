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

func TestHandleCreditSummary_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetCreditSummaryFn: func(_ context.Context) (*campaigns.CreditSummary, error) {
			return &campaigns.CreditSummary{CreditLimitCents: 5000000}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/credit/summary", nil)
	rec := httptest.NewRecorder()
	h.HandleCreditSummary(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result campaigns.CreditSummary
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.CreditLimitCents != 5000000 {
		t.Errorf("expected CreditLimitCents=5000000, got %d", result.CreditLimitCents)
	}
}

func TestHandleCreditSummary_Error(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetCreditSummaryFn: func(_ context.Context) (*campaigns.CreditSummary, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/credit/summary", nil)
	rec := httptest.NewRecorder()
	h.HandleCreditSummary(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}

func TestHandleGetCashflowConfig_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetCashflowConfigFn: func(_ context.Context) (*campaigns.CashflowConfig, error) {
			return &campaigns.CashflowConfig{CreditLimitCents: 5000000, CashBufferCents: 1000000}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/credit/config", nil)
	rec := httptest.NewRecorder()
	h.HandleGetCashflowConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleGetCashflowConfig_Error(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetCashflowConfigFn: func(_ context.Context) (*campaigns.CashflowConfig, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/credit/config", nil)
	rec := httptest.NewRecorder()
	h.HandleGetCashflowConfig(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleUpdateCashflowConfig_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		UpdateCashflowConfigFn: func(_ context.Context, cfg *campaigns.CashflowConfig) error {
			return nil
		},
	}
	h := newTestHandler(svc)

	body := `{"creditLimitCents":6000000,"cashBufferCents":1500000}`
	req := httptest.NewRequest(http.MethodPut, "/api/credit/config", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleUpdateCashflowConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateCashflowConfig_InvalidBody(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	req := httptest.NewRequest(http.MethodPut, "/api/credit/config", bytes.NewBufferString("{bad"))
	rec := httptest.NewRecorder()
	h.HandleUpdateCashflowConfig(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleUpdateCashflowConfig_Error(t *testing.T) {
	svc := &mocks.MockCampaignService{
		UpdateCashflowConfigFn: func(_ context.Context, _ *campaigns.CashflowConfig) error {
			return fmt.Errorf("write error")
		},
	}
	h := newTestHandler(svc)

	body := `{"creditLimitCents":6000000}`
	req := httptest.NewRequest(http.MethodPut, "/api/credit/config", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.HandleUpdateCashflowConfig(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleListInvoices_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ListInvoicesFn: func(_ context.Context) ([]campaigns.Invoice, error) {
			return []campaigns.Invoice{{ID: "inv-1"}}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/credit/invoices", nil)
	rec := httptest.NewRecorder()
	h.HandleListInvoices(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result []campaigns.Invoice
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 invoice, got %d", len(result))
	}
}

func TestHandleListInvoices_Empty(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ListInvoicesFn: func(_ context.Context) ([]campaigns.Invoice, error) {
			return nil, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/credit/invoices", nil)
	rec := httptest.NewRecorder()
	h.HandleListInvoices(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result []campaigns.Invoice
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result == nil {
		t.Error("expected empty array, got nil")
	}
}

func TestHandleListInvoices_Error(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ListInvoicesFn: func(_ context.Context) ([]campaigns.Invoice, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/credit/invoices", nil)
	rec := httptest.NewRecorder()
	h.HandleListInvoices(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleUpdateInvoice_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		UpdateInvoiceFn: func(_ context.Context, inv *campaigns.Invoice) error {
			return nil
		},
	}
	h := newTestHandler(svc)

	body := `{"id":"inv-1","totalCents":10000}`
	req := httptest.NewRequest(http.MethodPut, "/api/credit/invoices", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.HandleUpdateInvoice(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateInvoice_NotFound(t *testing.T) {
	svc := &mocks.MockCampaignService{
		UpdateInvoiceFn: func(_ context.Context, _ *campaigns.Invoice) error {
			return campaigns.ErrInvoiceNotFound
		},
	}
	h := newTestHandler(svc)

	body := `{"id":"missing"}`
	req := httptest.NewRequest(http.MethodPut, "/api/credit/invoices", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.HandleUpdateInvoice(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleUpdateInvoice_InvalidBody(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	req := httptest.NewRequest(http.MethodPut, "/api/credit/invoices", bytes.NewBufferString("{bad"))
	rec := httptest.NewRecorder()
	h.HandleUpdateInvoice(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandlePortfolioHealth_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetPortfolioHealthFn: func(_ context.Context) (*campaigns.PortfolioHealth, error) {
			return &campaigns.PortfolioHealth{}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolio/health", nil)
	rec := httptest.NewRecorder()
	h.HandlePortfolioHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandlePortfolioHealth_Error(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetPortfolioHealthFn: func(_ context.Context) (*campaigns.PortfolioHealth, error) {
			return nil, fmt.Errorf("service error")
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolio/health", nil)
	rec := httptest.NewRecorder()
	h.HandlePortfolioHealth(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandlePortfolioChannelVelocity_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetPortfolioChannelVelocityFn: func(_ context.Context) ([]campaigns.ChannelVelocity, error) {
			return []campaigns.ChannelVelocity{{Channel: "ebay"}}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolio/channel-velocity", nil)
	rec := httptest.NewRecorder()
	h.HandlePortfolioChannelVelocity(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandlePortfolioInsights_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetPortfolioInsightsFn: func(_ context.Context) (*campaigns.PortfolioInsights, error) {
			return &campaigns.PortfolioInsights{}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolio/insights", nil)
	rec := httptest.NewRecorder()
	h.HandlePortfolioInsights(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleCampaignSuggestions_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetCampaignSuggestionsFn: func(_ context.Context) (*campaigns.SuggestionsResponse, error) {
			return &campaigns.SuggestionsResponse{}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolio/suggestions", nil)
	rec := httptest.NewRecorder()
	h.HandleCampaignSuggestions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleCapitalTimeline_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetCapitalTimelineFn: func(_ context.Context) (*campaigns.CapitalTimeline, error) {
			return &campaigns.CapitalTimeline{}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolio/capital-timeline", nil)
	rec := httptest.NewRecorder()
	h.HandleCapitalTimeline(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleCapitalTimeline_Error(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetCapitalTimelineFn: func(_ context.Context) (*campaigns.CapitalTimeline, error) {
			return nil, fmt.Errorf("service error")
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolio/capital-timeline", nil)
	rec := httptest.NewRecorder()
	h.HandleCapitalTimeline(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleWeeklyReview_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetWeeklyReviewSummaryFn: func(_ context.Context) (*campaigns.WeeklyReviewSummary, error) {
			return &campaigns.WeeklyReviewSummary{}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolio/weekly-review", nil)
	rec := httptest.NewRecorder()
	h.HandleWeeklyReview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleWeeklyReview_Error(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetWeeklyReviewSummaryFn: func(_ context.Context) (*campaigns.WeeklyReviewSummary, error) {
			return nil, fmt.Errorf("service error")
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolio/weekly-review", nil)
	rec := httptest.NewRecorder()
	h.HandleWeeklyReview(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestHandleListRevocationFlags_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ListRevocationFlagsFn: func(_ context.Context) ([]campaigns.RevocationFlag, error) {
			return []campaigns.RevocationFlag{{ID: "rf-1"}}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolio/revocations", nil)
	rec := httptest.NewRecorder()
	h.HandleListRevocationFlags(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleListRevocationFlags_Empty(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ListRevocationFlagsFn: func(_ context.Context) ([]campaigns.RevocationFlag, error) {
			return nil, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolio/revocations", nil)
	rec := httptest.NewRecorder()
	h.HandleListRevocationFlags(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result []campaigns.RevocationFlag
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result == nil {
		t.Error("expected empty array, got nil")
	}
}

func TestHandleCreateRevocationFlag_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		FlagForRevocationFn: func(_ context.Context, label, dim, reason string) (*campaigns.RevocationFlag, error) {
			return &campaigns.RevocationFlag{SegmentLabel: label, SegmentDimension: dim, Reason: reason}, nil
		},
	}
	h := newTestHandler(svc)

	body := `{"segmentLabel":"low-margin","segmentDimension":"channel","reason":"underperforming"}`
	req := httptest.NewRequest(http.MethodPost, "/api/portfolio/revocations", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.HandleCreateRevocationFlag(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCreateRevocationFlag_TooSoon(t *testing.T) {
	svc := &mocks.MockCampaignService{
		FlagForRevocationFn: func(_ context.Context, _, _, _ string) (*campaigns.RevocationFlag, error) {
			return nil, errors.NewAppError(campaigns.ErrCodeRevocationTooSoon, "revocation already submitted within the past 7 days")
		},
	}
	h := newTestHandler(svc)

	body := `{"segmentLabel":"x","segmentDimension":"y","reason":"z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/portfolio/revocations", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.HandleCreateRevocationFlag(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestHandleCreateRevocationFlag_InvalidBody(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	req := httptest.NewRequest(http.MethodPost, "/api/portfolio/revocations", bytes.NewBufferString("{bad"))
	rec := httptest.NewRecorder()
	h.HandleCreateRevocationFlag(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleRevocationEmail_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GenerateRevocationEmailFn: func(_ context.Context, flagID string) (string, error) {
			return "Dear partner, we are revoking...", nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolio/revocations/rf-1/email", nil)
	req.SetPathValue("flagId", "rf-1")
	rec := httptest.NewRecorder()
	h.HandleRevocationEmail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["emailText"] == "" {
		t.Error("expected non-empty emailText")
	}
}

func TestHandleRevocationEmail_MissingFlagID(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	req := httptest.NewRequest(http.MethodGet, "/api/portfolio/revocations//email", nil)
	rec := httptest.NewRecorder()
	h.HandleRevocationEmail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleRevocationEmail_Error(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GenerateRevocationEmailFn: func(_ context.Context, _ string) (string, error) {
			return "", fmt.Errorf("generation failed")
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/portfolio/revocations/rf-1/email", nil)
	req.SetPathValue("flagId", "rf-1")
	rec := httptest.NewRecorder()
	h.HandleRevocationEmail(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
