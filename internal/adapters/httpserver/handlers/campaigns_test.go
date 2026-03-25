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

// --- helpers ---

func newTestHandler(svc *mocks.MockCampaignService) *CampaignsHandler {
	return NewCampaignsHandler(svc, mocks.NewMockLogger(), nil, nil)
}

// decodeErrorResponse decodes a JSON response and returns the "error" field value.
func decodeErrorResponse(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	msg, ok := body["error"]
	if !ok {
		t.Fatalf("expected JSON response to contain \"error\" key, got: %v", body)
	}
	return msg
}

// --- HandleListCampaigns (GET list) ---

func TestHandleListCampaigns_GET_List(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		activeOnly bool
		campaigns  []campaigns.Campaign
	}{
		{
			name:       "list all campaigns",
			query:      "",
			activeOnly: false,
			campaigns: []campaigns.Campaign{
				{ID: "c1", Name: "Camp A"},
				{ID: "c2", Name: "Camp B"},
			},
		},
		{
			name:       "list active only",
			query:      "?activeOnly=true",
			activeOnly: true,
			campaigns:  []campaigns.Campaign{{ID: "c1", Name: "Active"}},
		},
		{
			name:       "empty list returns JSON array",
			query:      "",
			activeOnly: false,
			campaigns:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{
				ListCampaignsFn: func(_ context.Context, activeOnly bool) ([]campaigns.Campaign, error) {
					if activeOnly != tt.activeOnly {
						t.Errorf("expected activeOnly=%v, got %v", tt.activeOnly, activeOnly)
					}
					return tt.campaigns, nil
				},
			}
			h := newTestHandler(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/campaigns"+tt.query, nil)
			rec := httptest.NewRecorder()
			h.HandleListCampaigns(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("expected Content-Type application/json, got %q", ct)
			}
			// Verify response is a JSON array (even when nil from service)
			var result []campaigns.Campaign
			if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
		})
	}
}

func TestHandleListCampaigns_GET_ServiceError(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ListCampaignsFn: func(_ context.Context, _ bool) ([]campaigns.Campaign, error) {
			return nil, fmt.Errorf("db connection lost")
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns", nil)
	rec := httptest.NewRecorder()
	h.HandleListCampaigns(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	msg := decodeErrorResponse(t, rec)
	if msg == "" {
		t.Error("expected non-empty error message")
	}
}

// --- HandleCreateCampaign (POST) ---

func TestHandleCreateCampaign_POST_CreateSuccess(t *testing.T) {
	svc := &mocks.MockCampaignService{
		CreateCampaignFn: func(_ context.Context, c *campaigns.Campaign) error {
			c.ID = "new-id"
			return nil
		},
	}
	h := newTestHandler(svc)

	body := `{"name":"My Campaign","sport":"pokemon","buyTermsCLPct":0.55}`
	req := httptest.NewRequest(http.MethodPost, "/api/campaigns", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleCreateCampaign(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var result campaigns.Campaign
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if result.ID != "new-id" {
		t.Errorf("expected ID=new-id, got %q", result.ID)
	}
}

func TestHandleCreateCampaign_POST_InvalidBody(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	req := httptest.NewRequest(http.MethodPost, "/api/campaigns", bytes.NewBufferString("{invalid"))
	rec := httptest.NewRecorder()
	h.HandleCreateCampaign(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}

func TestHandleCreateCampaign_POST_ValidationError(t *testing.T) {
	svc := &mocks.MockCampaignService{
		CreateCampaignFn: func(_ context.Context, _ *campaigns.Campaign) error {
			return campaigns.ErrCampaignNameRequired
		},
	}
	h := newTestHandler(svc)

	body := `{"name":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/campaigns", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.HandleCreateCampaign(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	decodeErrorResponse(t, rec)
}

// --- HandleGetCampaign (GET by ID) ---

func TestHandleGetCampaign_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetCampaignFn: func(_ context.Context, id string) (*campaigns.Campaign, error) {
			return &campaigns.Campaign{ID: id, Name: "Found"}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/abc-123", nil)
	req.SetPathValue("id", "abc-123")
	rec := httptest.NewRecorder()
	h.HandleGetCampaign(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var result campaigns.Campaign
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.ID != "abc-123" {
		t.Errorf("expected ID=abc-123, got %q", result.ID)
	}
}

func TestHandleGetCampaign_NotFound(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetCampaignFn: func(_ context.Context, _ string) (*campaigns.Campaign, error) {
			return nil, campaigns.ErrCampaignNotFound
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/missing", nil)
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()
	h.HandleGetCampaign(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}

func TestHandleGetCampaign_InternalError(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetCampaignFn: func(_ context.Context, _ string) (*campaigns.Campaign, error) {
			return nil, fmt.Errorf("database down")
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/abc-123", nil)
	req.SetPathValue("id", "abc-123")
	rec := httptest.NewRecorder()
	h.HandleGetCampaign(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}

// --- HandleUpdateCampaign (PUT) ---

func TestHandleUpdateCampaign_PUT_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		UpdateCampaignFn: func(_ context.Context, c *campaigns.Campaign) error {
			if c.ID != "abc-123" {
				t.Errorf("expected ID=abc-123, got %q", c.ID)
			}
			return nil
		},
	}
	h := newTestHandler(svc)

	body := `{"name":"Updated","sport":"pokemon","buyTermsCLPct":0.6}`
	req := httptest.NewRequest(http.MethodPut, "/api/campaigns/abc-123", bytes.NewBufferString(body))
	req.SetPathValue("id", "abc-123")
	rec := httptest.NewRecorder()
	h.HandleUpdateCampaign(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateCampaign_PUT_NotFound(t *testing.T) {
	svc := &mocks.MockCampaignService{
		UpdateCampaignFn: func(_ context.Context, _ *campaigns.Campaign) error {
			return campaigns.ErrCampaignNotFound
		},
	}
	h := newTestHandler(svc)

	body := `{"name":"X"}`
	req := httptest.NewRequest(http.MethodPut, "/api/campaigns/missing", bytes.NewBufferString(body))
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()
	h.HandleUpdateCampaign(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}

func TestHandleUpdateCampaign_PUT_ValidationError(t *testing.T) {
	svc := &mocks.MockCampaignService{
		UpdateCampaignFn: func(_ context.Context, _ *campaigns.Campaign) error {
			return campaigns.ErrInvalidBuyTermsPct
		},
	}
	h := newTestHandler(svc)

	body := `{"name":"Test","buyTermsCLPct":5.0}`
	req := httptest.NewRequest(http.MethodPut, "/api/campaigns/abc-123", bytes.NewBufferString(body))
	req.SetPathValue("id", "abc-123")
	rec := httptest.NewRecorder()
	h.HandleUpdateCampaign(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	decodeErrorResponse(t, rec)
}

func TestHandleUpdateCampaign_PUT_InvalidBody(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	req := httptest.NewRequest(http.MethodPut, "/api/campaigns/abc-123", bytes.NewBufferString("{bad"))
	req.SetPathValue("id", "abc-123")
	rec := httptest.NewRecorder()
	h.HandleUpdateCampaign(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}

// --- HandleDelete ---

func TestHandleDelete_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		DeleteCampaignFn: func(_ context.Context, id string) error {
			if id != "abc-123" {
				t.Errorf("expected id=abc-123, got %q", id)
			}
			return nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/campaigns/abc-123", nil)
	req.SetPathValue("id", "abc-123")
	rec := httptest.NewRecorder()
	h.HandleDelete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDelete_NotFound(t *testing.T) {
	svc := &mocks.MockCampaignService{
		DeleteCampaignFn: func(_ context.Context, _ string) error {
			return campaigns.ErrCampaignNotFound
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/campaigns/missing", nil)
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()
	h.HandleDelete(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}

func TestHandleDelete_MissingID(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	req := httptest.NewRequest(http.MethodDelete, "/api/campaigns/", nil)
	// No SetPathValue — simulates missing ID
	rec := httptest.NewRecorder()
	h.HandleDelete(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	decodeErrorResponse(t, rec)
}
