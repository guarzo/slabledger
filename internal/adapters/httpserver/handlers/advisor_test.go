package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/advisor"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// --- Inline mocks for advisor.Service and advisor.CacheStore ---

type mockAdvisorService struct {
	GenerateDigestFn     func(ctx context.Context, stream func(advisor.StreamEvent)) error
	AnalyzeCampaignFn    func(ctx context.Context, campaignID string, stream func(advisor.StreamEvent)) error
	AnalyzeLiquidationFn func(ctx context.Context, stream func(advisor.StreamEvent)) error
	AssessPurchaseFn     func(ctx context.Context, req advisor.PurchaseAssessmentRequest, stream func(advisor.StreamEvent)) error
	CollectDigestFn      func(ctx context.Context) (string, error)
	CollectLiquidationFn func(ctx context.Context) (string, error)
}

func (m *mockAdvisorService) GenerateDigest(ctx context.Context, stream func(advisor.StreamEvent)) error {
	if m.GenerateDigestFn != nil {
		return m.GenerateDigestFn(ctx, stream)
	}
	return nil
}

func (m *mockAdvisorService) AnalyzeCampaign(ctx context.Context, campaignID string, stream func(advisor.StreamEvent)) error {
	if m.AnalyzeCampaignFn != nil {
		return m.AnalyzeCampaignFn(ctx, campaignID, stream)
	}
	return nil
}

func (m *mockAdvisorService) AnalyzeLiquidation(ctx context.Context, stream func(advisor.StreamEvent)) error {
	if m.AnalyzeLiquidationFn != nil {
		return m.AnalyzeLiquidationFn(ctx, stream)
	}
	return nil
}

func (m *mockAdvisorService) AssessPurchase(ctx context.Context, req advisor.PurchaseAssessmentRequest, stream func(advisor.StreamEvent)) error {
	if m.AssessPurchaseFn != nil {
		return m.AssessPurchaseFn(ctx, req, stream)
	}
	return nil
}

func (m *mockAdvisorService) CollectDigest(ctx context.Context) (string, error) {
	if m.CollectDigestFn != nil {
		return m.CollectDigestFn(ctx)
	}
	return "", nil
}

func (m *mockAdvisorService) CollectLiquidation(ctx context.Context) (string, error) {
	if m.CollectLiquidationFn != nil {
		return m.CollectLiquidationFn(ctx)
	}
	return "", nil
}

type mockCacheStore struct {
	GetFn               func(ctx context.Context, analysisType advisor.AnalysisType) (*advisor.CachedAnalysis, error)
	MarkRunningFn       func(ctx context.Context, analysisType advisor.AnalysisType) (string, error)
	AcquireRefreshFn    func(ctx context.Context, analysisType advisor.AnalysisType) (string, bool, error)
	ForceAcquireStaleFn func(ctx context.Context, analysisType advisor.AnalysisType, staleThreshold time.Duration) (string, bool, error)
	SaveResultFn        func(ctx context.Context, analysisType advisor.AnalysisType, lease, content, errMsg string) error
}

func (m *mockCacheStore) Get(ctx context.Context, analysisType advisor.AnalysisType) (*advisor.CachedAnalysis, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, analysisType)
	}
	return nil, nil
}

func (m *mockCacheStore) MarkRunning(ctx context.Context, analysisType advisor.AnalysisType) (string, error) {
	if m.MarkRunningFn != nil {
		return m.MarkRunningFn(ctx, analysisType)
	}
	return "lease-1", nil
}

func (m *mockCacheStore) AcquireRefresh(ctx context.Context, analysisType advisor.AnalysisType) (string, bool, error) {
	if m.AcquireRefreshFn != nil {
		return m.AcquireRefreshFn(ctx, analysisType)
	}
	return "lease-1", true, nil
}

func (m *mockCacheStore) ForceAcquireStale(ctx context.Context, analysisType advisor.AnalysisType, staleThreshold time.Duration) (string, bool, error) {
	if m.ForceAcquireStaleFn != nil {
		return m.ForceAcquireStaleFn(ctx, analysisType, staleThreshold)
	}
	return "", false, nil
}

func (m *mockCacheStore) SaveResult(ctx context.Context, analysisType advisor.AnalysisType, lease, content, errMsg string) error {
	if m.SaveResultFn != nil {
		return m.SaveResultFn(ctx, analysisType, lease, content, errMsg)
	}
	return nil
}

// newAdvisorHandler creates an AdvisorHandler for testing.
func newAdvisorHandler(svc advisor.Service, cache advisor.CacheStore) *AdvisorHandler {
	return NewAdvisorHandler(svc, nil, cache, mocks.NewMockLogger())
}

// --- HandleGetCached ---

func TestHandleGetCached_NoCacheStore(t *testing.T) {
	h := newAdvisorHandler(&mockAdvisorService{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/advisor/cached/digest", nil)
	req = withUser(req)
	req.SetPathValue("type", "digest")
	rec := httptest.NewRecorder()
	h.HandleGetCached(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetCached_RequiresUser(t *testing.T) {
	h := newAdvisorHandler(&mockAdvisorService{}, &mockCacheStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/advisor/cached/digest", nil)
	// No withUser — no auth
	req.SetPathValue("type", "digest")
	rec := httptest.NewRecorder()
	h.HandleGetCached(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleGetCached_InvalidType(t *testing.T) {
	h := newAdvisorHandler(&mockAdvisorService{}, &mockCacheStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/advisor/cached/badtype", nil)
	req = withUser(req)
	req.SetPathValue("type", "badtype")
	rec := httptest.NewRecorder()
	h.HandleGetCached(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleGetCached_EmptyCache(t *testing.T) {
	cache := &mockCacheStore{
		GetFn: func(_ context.Context, _ advisor.AnalysisType) (*advisor.CachedAnalysis, error) {
			return nil, nil // not cached
		},
	}
	h := newAdvisorHandler(&mockAdvisorService{}, cache)

	req := httptest.NewRequest(http.MethodGet, "/api/advisor/cached/digest", nil)
	req = withUser(req)
	req.SetPathValue("type", "digest")
	rec := httptest.NewRecorder()
	h.HandleGetCached(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var result map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["status"] != string(advisor.StatusEmpty) {
		t.Errorf("expected status=empty, got %q", result["status"])
	}
}

func TestHandleGetCached_WithCachedResult(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	cache := &mockCacheStore{
		GetFn: func(_ context.Context, _ advisor.AnalysisType) (*advisor.CachedAnalysis, error) {
			return &advisor.CachedAnalysis{
				AnalysisType: advisor.AnalysisDigest,
				Status:       advisor.StatusComplete,
				Content:      "analysis content",
				UpdatedAt:    now,
			}, nil
		},
	}
	h := newAdvisorHandler(&mockAdvisorService{}, cache)

	req := httptest.NewRequest(http.MethodGet, "/api/advisor/cached/digest", nil)
	req = withUser(req)
	req.SetPathValue("type", "digest")
	rec := httptest.NewRecorder()
	h.HandleGetCached(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var result map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["status"] != string(advisor.StatusComplete) {
		t.Errorf("expected status=complete, got %v", result["status"])
	}
	if result["content"] != "analysis content" {
		t.Errorf("expected content='analysis content', got %v", result["content"])
	}
	if _, ok := result["updatedAt"]; !ok {
		t.Error("expected updatedAt in response")
	}
}

func TestHandleGetCached_ServiceError(t *testing.T) {
	cache := &mockCacheStore{
		GetFn: func(_ context.Context, _ advisor.AnalysisType) (*advisor.CachedAnalysis, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := newAdvisorHandler(&mockAdvisorService{}, cache)

	req := httptest.NewRequest(http.MethodGet, "/api/advisor/cached/digest", nil)
	req = withUser(req)
	req.SetPathValue("type", "digest")
	rec := httptest.NewRecorder()
	h.HandleGetCached(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// --- HandleRefreshTrigger ---

func TestHandleRefreshTrigger_NoCacheStore(t *testing.T) {
	h := newAdvisorHandler(&mockAdvisorService{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/advisor/refresh/digest", nil)
	req = withUser(req)
	req.SetPathValue("type", "digest")
	rec := httptest.NewRecorder()
	h.HandleRefreshTrigger(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHandleRefreshTrigger_Acquired(t *testing.T) {
	svc := &mockAdvisorService{
		CollectDigestFn: func(_ context.Context) (string, error) {
			return "digest content", nil
		},
	}
	cache := &mockCacheStore{
		AcquireRefreshFn: func(_ context.Context, _ advisor.AnalysisType) (string, bool, error) {
			return "lease-1", true, nil
		},
		SaveResultFn: func(_ context.Context, _ advisor.AnalysisType, _, _, _ string) error {
			return nil
		},
	}
	h := newAdvisorHandler(svc, cache)

	req := httptest.NewRequest(http.MethodPost, "/api/advisor/refresh/digest", nil)
	req = withUser(req)
	req.SetPathValue("type", "digest")
	rec := httptest.NewRecorder()
	h.HandleRefreshTrigger(rec, req)

	// Wait for background goroutine to finish
	h.Wait()

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var result map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["status"] != string(advisor.StatusRunning) {
		t.Errorf("expected status=running, got %q", result["status"])
	}
}

func TestHandleRefreshTrigger_AlreadyRunning(t *testing.T) {
	cache := &mockCacheStore{
		AcquireRefreshFn: func(_ context.Context, _ advisor.AnalysisType) (string, bool, error) {
			return "", false, nil // not acquired - already running
		},
		ForceAcquireStaleFn: func(_ context.Context, _ advisor.AnalysisType, _ time.Duration) (string, bool, error) {
			return "", false, nil // also not stale
		},
	}
	h := newAdvisorHandler(&mockAdvisorService{}, cache)

	req := httptest.NewRequest(http.MethodPost, "/api/advisor/refresh/digest", nil)
	req = withUser(req)
	req.SetPathValue("type", "digest")
	rec := httptest.NewRecorder()
	h.HandleRefreshTrigger(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var result map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["status"] != string(advisor.StatusRunning) {
		t.Errorf("expected status=running, got %q", result["status"])
	}
}

func TestHandleRefreshTrigger_InvalidType(t *testing.T) {
	h := newAdvisorHandler(&mockAdvisorService{}, &mockCacheStore{})

	req := httptest.NewRequest(http.MethodPost, "/api/advisor/refresh/badtype", nil)
	req = withUser(req)
	req.SetPathValue("type", "badtype")
	rec := httptest.NewRecorder()
	h.HandleRefreshTrigger(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// --- HandleDigest ---

func TestHandleDigest_MethodNotAllowed(t *testing.T) {
	h := newAdvisorHandler(&mockAdvisorService{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/advisor/digest", nil)
	req = withUser(req)
	rec := httptest.NewRecorder()
	h.HandleDigest(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleDigest_RequiresUser(t *testing.T) {
	h := newAdvisorHandler(&mockAdvisorService{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/advisor/digest", nil)
	// No withUser
	rec := httptest.NewRecorder()
	h.HandleDigest(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleDigest_Success(t *testing.T) {
	svc := &mockAdvisorService{
		GenerateDigestFn: func(_ context.Context, stream func(advisor.StreamEvent)) error {
			stream(advisor.StreamEvent{Type: advisor.EventDelta, Content: "digest text"})
			return nil
		},
	}
	h := newAdvisorHandler(svc, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/advisor/digest", nil)
	req = withUser(req)
	rec := httptest.NewRecorder()
	h.HandleDigest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "data: [DONE]") {
		t.Errorf("expected 'data: [DONE]' in SSE body, got: %s", body)
	}
}

// --- HandleCampaignAnalysis ---

func TestHandleCampaignAnalysis_MissingCampaignID(t *testing.T) {
	h := newAdvisorHandler(&mockAdvisorService{}, nil)

	body := `{"campaignId":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/advisor/campaign", bytes.NewBufferString(body))
	req = withUser(req)
	rec := httptest.NewRecorder()
	h.HandleCampaignAnalysis(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCampaignAnalysis_Success(t *testing.T) {
	svc := &mockAdvisorService{
		AnalyzeCampaignFn: func(_ context.Context, campaignID string, stream func(advisor.StreamEvent)) error {
			stream(advisor.StreamEvent{Type: advisor.EventDelta, Content: "campaign analysis for " + campaignID})
			return nil
		},
	}
	h := newAdvisorHandler(svc, nil)

	body := `{"campaignId":"c1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/advisor/campaign", bytes.NewBufferString(body))
	req = withUser(req)
	rec := httptest.NewRecorder()
	h.HandleCampaignAnalysis(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body2 := rec.Body.String()
	if !strings.Contains(body2, "data: [DONE]") {
		t.Errorf("expected DONE sentinel in SSE body")
	}
}

func TestHandleCampaignAnalysis_MethodNotAllowed(t *testing.T) {
	h := newAdvisorHandler(&mockAdvisorService{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/advisor/campaign", nil)
	req = withUser(req)
	rec := httptest.NewRecorder()
	h.HandleCampaignAnalysis(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

// --- HandleLiquidationAnalysis ---

func TestHandleLiquidationAnalysis_Success(t *testing.T) {
	svc := &mockAdvisorService{
		AnalyzeLiquidationFn: func(_ context.Context, stream func(advisor.StreamEvent)) error {
			stream(advisor.StreamEvent{Type: advisor.EventDelta, Content: "liquidation candidates"})
			return nil
		},
	}
	h := newAdvisorHandler(svc, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/advisor/liquidation", nil)
	req = withUser(req)
	rec := httptest.NewRecorder()
	h.HandleLiquidationAnalysis(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleLiquidationAnalysis_RequiresUser(t *testing.T) {
	h := newAdvisorHandler(&mockAdvisorService{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/advisor/liquidation", nil)
	rec := httptest.NewRecorder()
	h.HandleLiquidationAnalysis(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// --- HandlePurchaseAssessment ---

func TestHandlePurchaseAssessment_ValidationErrors(t *testing.T) {
	gradeStr := "PSA 9"
	costCents := 5000

	tests := []struct {
		name string
		body string
	}{
		{"missing campaignId", `{"campaignId":"","cardName":"Charizard","grade":"9","buyCostCents":5000}`},
		{"missing cardName", `{"campaignId":"c1","cardName":"","grade":"9","buyCostCents":5000}`},
		{"missing grade", `{"campaignId":"c1","cardName":"Charizard","buyCostCents":5000}`},
		{"missing buyCostCents", `{"campaignId":"c1","cardName":"Charizard","grade":"9"}`},
		{"invalid json", `{bad`},
	}
	_ = gradeStr
	_ = costCents

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newAdvisorHandler(&mockAdvisorService{}, nil)
			req := httptest.NewRequest(http.MethodPost, "/api/advisor/purchase", bytes.NewBufferString(tt.body))
			req = withUser(req)
			rec := httptest.NewRecorder()
			h.HandlePurchaseAssessment(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandlePurchaseAssessment_Success(t *testing.T) {
	svc := &mockAdvisorService{
		AssessPurchaseFn: func(_ context.Context, req advisor.PurchaseAssessmentRequest, stream func(advisor.StreamEvent)) error {
			stream(advisor.StreamEvent{Type: advisor.EventDelta, Content: "assessment for " + req.CardName})
			return nil
		},
	}
	campaignsSvc := &mocks.MockCampaignService{
		GetCampaignFn: func(_ context.Context, id string) (*campaigns.Campaign, error) {
			return &campaigns.Campaign{ID: id, Name: "Test Campaign"}, nil
		},
	}
	h := NewAdvisorHandler(svc, campaignsSvc, nil, mocks.NewMockLogger())

	grade := "9"
	buyCost := 5000
	bodyData := map[string]any{
		"campaignId":   "c1",
		"cardName":     "Charizard",
		"grade":        grade,
		"buyCostCents": buyCost,
	}
	bodyBytes, _ := json.Marshal(bodyData)
	req := httptest.NewRequest(http.MethodPost, "/api/advisor/purchase", bytes.NewBuffer(bodyBytes))
	req = withUser(req)
	rec := httptest.NewRecorder()
	h.HandlePurchaseAssessment(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "data: [DONE]") {
		t.Error("expected DONE sentinel in SSE response")
	}
}

func TestHandlePurchaseAssessment_RequiresUser(t *testing.T) {
	h := newAdvisorHandler(&mockAdvisorService{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/advisor/purchase", bytes.NewBufferString(`{}`))
	// No withUser
	rec := httptest.NewRecorder()
	h.HandlePurchaseAssessment(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandlePurchaseAssessment_MethodNotAllowed(t *testing.T) {
	h := newAdvisorHandler(&mockAdvisorService{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/advisor/purchase", nil)
	req = withUser(req)
	rec := httptest.NewRecorder()
	h.HandlePurchaseAssessment(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
