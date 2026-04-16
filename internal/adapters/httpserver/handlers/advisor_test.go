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
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// newAdvisorHandler creates an AdvisorHandler for testing.
func newAdvisorHandler(svc advisor.Service, cache advisor.CacheStore) *AdvisorHandler {
	return NewAdvisorHandler(svc, nil, cache, mocks.NewMockLogger())
}

// --- HandleGetCached ---

func TestHandleGetCached(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	tests := []struct {
		name       string
		setupCache func() advisor.CacheStore
		withAuth   bool
		pathType   string
		wantCode   int
		checkBody  func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name:       "no cache store returns 503",
			setupCache: func() advisor.CacheStore { return nil },
			withAuth:   true,
			pathType:   "digest",
			wantCode:   http.StatusServiceUnavailable,
		},
		{
			name:       "requires auth",
			setupCache: func() advisor.CacheStore { return &mocks.MockCacheStore{} },
			withAuth:   false,
			pathType:   "digest",
			wantCode:   http.StatusUnauthorized,
		},
		{
			name:       "invalid type returns 400",
			setupCache: func() advisor.CacheStore { return &mocks.MockCacheStore{} },
			withAuth:   true,
			pathType:   "badtype",
			wantCode:   http.StatusBadRequest,
		},
		{
			name: "empty cache returns status=empty",
			setupCache: func() advisor.CacheStore {
				return &mocks.MockCacheStore{
					GetFn: func(_ context.Context, _ advisor.AnalysisType) (*advisor.CachedAnalysis, error) {
						return nil, nil
					},
				}
			},
			withAuth: true,
			pathType: "digest",
			wantCode: http.StatusOK,
			checkBody: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var result map[string]string
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if result["status"] != string(advisor.StatusEmpty) {
					t.Errorf("expected status=empty, got %q", result["status"])
				}
			},
		},
		{
			name: "cached result returns status and content",
			setupCache: func() advisor.CacheStore {
				return &mocks.MockCacheStore{
					GetFn: func(_ context.Context, _ advisor.AnalysisType) (*advisor.CachedAnalysis, error) {
						return &advisor.CachedAnalysis{
							AnalysisType: advisor.AnalysisDigest,
							Status:       advisor.StatusComplete,
							Content:      "analysis content",
							UpdatedAt:    now,
						}, nil
					},
				}
			},
			withAuth: true,
			pathType: "digest",
			wantCode: http.StatusOK,
			checkBody: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
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
			},
		},
		{
			name: "cache get error returns 500",
			setupCache: func() advisor.CacheStore {
				return &mocks.MockCacheStore{
					GetFn: func(_ context.Context, _ advisor.AnalysisType) (*advisor.CachedAnalysis, error) {
						return nil, fmt.Errorf("database error")
					},
				}
			},
			withAuth: true,
			pathType: "digest",
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newAdvisorHandler(&mocks.MockAdvisorService{}, tc.setupCache())
			req := httptest.NewRequest(http.MethodGet, "/api/advisor/cached/"+tc.pathType, nil)
			if tc.withAuth {
				req = withUser(req)
			}
			req.SetPathValue("type", tc.pathType)
			rec := httptest.NewRecorder()
			h.HandleGetCached(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d; body: %s", tc.wantCode, rec.Code, rec.Body.String())
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rec)
			}
		})
	}
}

// --- HandleRefreshTrigger ---

func TestHandleRefreshTrigger(t *testing.T) {
	tests := []struct {
		name       string
		pathType   string
		setupSvc   func() *mocks.MockAdvisorService
		setupCache func() advisor.CacheStore
		waitBg     bool
		wantCode   int
		checkBody  func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name:       "no cache store returns 503",
			pathType:   "digest",
			setupSvc:   func() *mocks.MockAdvisorService { return &mocks.MockAdvisorService{} },
			setupCache: func() advisor.CacheStore { return nil },
			wantCode:   http.StatusServiceUnavailable,
		},
		{
			name:     "acquired returns 202 with status=running",
			pathType: "digest",
			setupSvc: func() *mocks.MockAdvisorService {
				return &mocks.MockAdvisorService{
					CollectDigestFn: func(_ context.Context) (string, error) {
						return "digest content", nil
					},
				}
			},
			setupCache: func() advisor.CacheStore {
				return &mocks.MockCacheStore{
					AcquireRefreshFn: func(_ context.Context, _ advisor.AnalysisType) (string, bool, error) {
						return "lease-1", true, nil
					},
					SaveResultFn: func(_ context.Context, _ advisor.AnalysisType, _, _, _ string) error {
						return nil
					},
				}
			},
			waitBg:   true,
			wantCode: http.StatusAccepted,
			checkBody: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var result map[string]string
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if result["status"] != string(advisor.StatusRunning) {
					t.Errorf("expected status=running, got %q", result["status"])
				}
			},
		},
		{
			name:     "already running returns 200 with status=running",
			pathType: "digest",
			setupSvc: func() *mocks.MockAdvisorService { return &mocks.MockAdvisorService{} },
			setupCache: func() advisor.CacheStore {
				return &mocks.MockCacheStore{
					AcquireRefreshFn: func(_ context.Context, _ advisor.AnalysisType) (string, bool, error) {
						return "", false, nil
					},
					ForceAcquireStaleFn: func(_ context.Context, _ advisor.AnalysisType, _ time.Duration) (string, bool, error) {
						return "", false, nil
					},
				}
			},
			wantCode: http.StatusOK,
			checkBody: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var result map[string]string
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if result["status"] != string(advisor.StatusRunning) {
					t.Errorf("expected status=running, got %q", result["status"])
				}
			},
		},
		{
			name:       "invalid type returns 400",
			pathType:   "badtype",
			setupSvc:   func() *mocks.MockAdvisorService { return &mocks.MockAdvisorService{} },
			setupCache: func() advisor.CacheStore { return &mocks.MockCacheStore{} },
			wantCode:   http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newAdvisorHandler(tc.setupSvc(), tc.setupCache())
			req := httptest.NewRequest(http.MethodPost, "/api/advisor/refresh/"+tc.pathType, nil)
			req = withUser(req)
			req.SetPathValue("type", tc.pathType)
			rec := httptest.NewRecorder()
			h.HandleRefreshTrigger(rec, req)
			if tc.waitBg {
				h.Wait()
			}

			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d; body: %s", tc.wantCode, rec.Code, rec.Body.String())
			}
			if tc.checkBody != nil {
				tc.checkBody(t, rec)
			}
		})
	}
}

// --- HandleDigest ---

func TestHandleDigest_RequiresUser(t *testing.T) {
	h := newAdvisorHandler(&mocks.MockAdvisorService{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/advisor/digest", nil)
	// No withUser
	rec := httptest.NewRecorder()
	h.HandleDigest(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleDigest_Success(t *testing.T) {
	svc := &mocks.MockAdvisorService{
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
	h := newAdvisorHandler(&mocks.MockAdvisorService{}, nil)

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
	svc := &mocks.MockAdvisorService{
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
	if !strings.Contains(rec.Body.String(), "data: [DONE]") {
		t.Errorf("expected DONE sentinel in SSE body")
	}
}

// --- HandleLiquidationAnalysis ---

func TestHandleLiquidationAnalysis_Success(t *testing.T) {
	svc := &mocks.MockAdvisorService{
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
	h := newAdvisorHandler(&mocks.MockAdvisorService{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/advisor/liquidation", nil)
	rec := httptest.NewRecorder()
	h.HandleLiquidationAnalysis(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

