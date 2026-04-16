package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func newPriceFlagsHandler(svc *mocks.MockPricingService) *PriceFlagsHandler {
	return NewPriceFlagsHandler(svc, mocks.NewMockLogger())
}

// --- HandleListPriceFlags ---

func TestHandleListPriceFlags_StatusFiltering(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus string
		wantCode   int
	}{
		{"explicit open", "?status=open", "open", http.StatusOK},
		{"explicit resolved", "?status=resolved", "resolved", http.StatusOK},
		{"explicit all", "?status=all", "all", http.StatusOK},
		{"empty defaults to open", "", "open", http.StatusOK},
		{"invalid status rejected", "?status=bogus", "", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedStatus string
			svc := &mocks.MockPricingService{
				ListPriceFlagsFn: func(_ context.Context, status string) ([]inventory.PriceFlagWithContext, error) {
					receivedStatus = status
					return []inventory.PriceFlagWithContext{}, nil
				},
			}
			h := newPriceFlagsHandler(svc)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/admin/price-flags"+tt.query, nil)
			h.HandleListPriceFlags(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status: got %d, want %d (body=%s)", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantCode == http.StatusOK && receivedStatus != tt.wantStatus {
				t.Errorf("service status: got %q, want %q", receivedStatus, tt.wantStatus)
			}
		})
	}
}

func TestHandleListPriceFlags_NilResultBecomesEmptyArray(t *testing.T) {
	svc := &mocks.MockPricingService{
		ListPriceFlagsFn: func(_ context.Context, _ string) ([]inventory.PriceFlagWithContext, error) {
			return nil, nil
		},
	}
	h := newPriceFlagsHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/price-flags", nil)
	h.HandleListPriceFlags(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body struct {
		Flags []inventory.PriceFlagWithContext `json:"flags"`
		Total int                              `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Flags == nil {
		t.Errorf("flags should be empty array, not null")
	}
	if body.Total != 0 {
		t.Errorf("total: got %d, want 0", body.Total)
	}
}

func TestHandleListPriceFlags_ReturnsFlagsAndTotal(t *testing.T) {
	flags := []inventory.PriceFlagWithContext{
		{PriceFlag: inventory.PriceFlag{ID: 1, PurchaseID: "p1", FlaggedBy: 7, Reason: inventory.PriceFlagWrongMatch}},
		{PriceFlag: inventory.PriceFlag{ID: 2, PurchaseID: "p2", FlaggedBy: 7, Reason: inventory.PriceFlagStaleData}},
	}
	svc := &mocks.MockPricingService{
		ListPriceFlagsFn: func(_ context.Context, _ string) ([]inventory.PriceFlagWithContext, error) {
			return flags, nil
		},
	}
	h := newPriceFlagsHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/price-flags", nil)
	h.HandleListPriceFlags(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body struct {
		Flags []inventory.PriceFlagWithContext `json:"flags"`
		Total int                              `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Total != len(flags) {
		t.Errorf("total: got %d, want %d", body.Total, len(flags))
	}
	if len(body.Flags) != len(flags) {
		t.Fatalf("flags: got %d, want %d", len(body.Flags), len(flags))
	}
	if body.Flags[0].ID != flags[0].ID || body.Flags[1].ID != flags[1].ID {
		t.Errorf("flag IDs not preserved: got %v %v", body.Flags[0].ID, body.Flags[1].ID)
	}
}

func TestHandleListPriceFlags_ServiceError(t *testing.T) {
	svc := &mocks.MockPricingService{
		ListPriceFlagsFn: func(_ context.Context, _ string) ([]inventory.PriceFlagWithContext, error) {
			return nil, inventory.ErrCampaignNotFound
		},
	}
	h := newPriceFlagsHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/price-flags", nil)
	h.HandleListPriceFlags(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// --- HandleResolvePriceFlag ---

func TestHandleResolvePriceFlag_Success(t *testing.T) {
	var capturedID, capturedUserID int64
	svc := &mocks.MockPricingService{
		ResolvePriceFlagFn: func(_ context.Context, flagID int64, resolvedBy int64) error {
			capturedID = flagID
			capturedUserID = resolvedBy
			return nil
		},
	}
	h := newPriceFlagsHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/price-flags/123/resolve", nil)
	req.SetPathValue("flagId", "123")
	req = withUser(req)
	h.HandleResolvePriceFlag(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if capturedID != 123 {
		t.Errorf("flagID: got %d, want 123", capturedID)
	}
	if capturedUserID != 42 {
		t.Errorf("resolvedBy: got %d, want 42 (from withUser)", capturedUserID)
	}
}

func TestHandleResolvePriceFlag_InvalidFlagID(t *testing.T) {
	svc := &mocks.MockPricingService{
		ResolvePriceFlagFn: func(_ context.Context, _ int64, _ int64) error {
			t.Fatalf("service should not be called when flagID is invalid")
			return nil
		},
	}
	h := newPriceFlagsHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/price-flags/not-a-number/resolve", nil)
	req.SetPathValue("flagId", "not-a-number")
	req = withUser(req)
	h.HandleResolvePriceFlag(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleResolvePriceFlag_Unauthorized(t *testing.T) {
	svc := &mocks.MockPricingService{
		ResolvePriceFlagFn: func(_ context.Context, _ int64, _ int64) error {
			t.Fatalf("service should not be called when user is missing")
			return nil
		},
	}
	h := newPriceFlagsHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/price-flags/1/resolve", nil)
	req.SetPathValue("flagId", "1")
	// No user context.
	h.HandleResolvePriceFlag(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleResolvePriceFlag_NotFound(t *testing.T) {
	svc := &mocks.MockPricingService{
		ResolvePriceFlagFn: func(_ context.Context, _ int64, _ int64) error {
			return inventory.ErrPriceFlagNotFound
		},
	}
	h := newPriceFlagsHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/price-flags/999/resolve", nil)
	req.SetPathValue("flagId", strconv.Itoa(999))
	req = withUser(req)
	h.HandleResolvePriceFlag(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestHandleResolvePriceFlag_ServiceError(t *testing.T) {
	svc := &mocks.MockPricingService{
		ResolvePriceFlagFn: func(_ context.Context, _ int64, _ int64) error {
			return inventory.ErrCampaignNotFound // any non-NotFound error
		},
	}
	h := newPriceFlagsHandler(svc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/price-flags/1/resolve", nil)
	req.SetPathValue("flagId", "1")
	req = withUser(req)
	h.HandleResolvePriceFlag(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
