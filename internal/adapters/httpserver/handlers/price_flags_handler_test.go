package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func newPriceFlagsHandler(svc *mocks.MockPricingService) *PriceFlagsHandler {
	return NewPriceFlagsHandler(svc, mocks.NewMockLogger())
}

// --- HandleListPriceFlags ---

func TestHandleListPriceFlags(t *testing.T) {
	twoFlags := []inventory.PriceFlagWithContext{
		{PriceFlag: inventory.PriceFlag{ID: 1, PurchaseID: "p1", FlaggedBy: 7, Reason: inventory.PriceFlagWrongMatch}},
		{PriceFlag: inventory.PriceFlag{ID: 2, PurchaseID: "p2", FlaggedBy: 7, Reason: inventory.PriceFlagStaleData}},
	}

	tests := []struct {
		name string
		// query is appended to the URL.
		query string
		// listResult is what the mock service returns when invoked.
		// If listError is non-nil, listResult is ignored.
		listResult []inventory.PriceFlagWithContext
		listError  error

		wantCode int
		// wantStatus is the value the service should receive (only checked
		// when wantCode is OK and the service is expected to be called).
		wantStatus string
		// wantServiceCalled asserts whether the mock should be invoked.
		// Invalid-status requests must short-circuit before the service.
		wantServiceCalled bool
		// wantBodyTotal verifies the JSON body's "total" field on OK
		// responses. Use -1 to skip.
		wantBodyTotal int
		// wantBodyFlagsLen verifies len(body.flags). Use -1 to skip.
		wantBodyFlagsLen int
		// wantNonNullFlags asserts the JSON "flags" key is `[]` not `null`.
		wantNonNullFlags bool
	}{
		{
			name:              "explicit open",
			query:             "?status=open",
			listResult:        []inventory.PriceFlagWithContext{},
			wantCode:          http.StatusOK,
			wantStatus:        "open",
			wantServiceCalled: true,
			wantBodyTotal:     0,
			wantBodyFlagsLen:  0,
		},
		{
			name:              "explicit resolved",
			query:             "?status=resolved",
			listResult:        []inventory.PriceFlagWithContext{},
			wantCode:          http.StatusOK,
			wantStatus:        "resolved",
			wantServiceCalled: true,
			wantBodyTotal:     0,
			wantBodyFlagsLen:  0,
		},
		{
			name:              "explicit all",
			query:             "?status=all",
			listResult:        []inventory.PriceFlagWithContext{},
			wantCode:          http.StatusOK,
			wantStatus:        "all",
			wantServiceCalled: true,
			wantBodyTotal:     0,
			wantBodyFlagsLen:  0,
		},
		{
			name:              "empty defaults to open",
			query:             "",
			listResult:        []inventory.PriceFlagWithContext{},
			wantCode:          http.StatusOK,
			wantStatus:        "open",
			wantServiceCalled: true,
			wantBodyTotal:     0,
			wantBodyFlagsLen:  0,
		},
		{
			name:              "invalid status rejected without calling service",
			query:             "?status=bogus",
			wantCode:          http.StatusBadRequest,
			wantServiceCalled: false,
			wantBodyTotal:     -1,
			wantBodyFlagsLen:  -1,
		},
		{
			name:              "nil result becomes empty array",
			query:             "",
			listResult:        nil,
			wantCode:          http.StatusOK,
			wantStatus:        "open",
			wantServiceCalled: true,
			wantBodyTotal:     0,
			wantBodyFlagsLen:  0,
			wantNonNullFlags:  true,
		},
		{
			name:              "returns flags and total preserved",
			query:             "",
			listResult:        twoFlags,
			wantCode:          http.StatusOK,
			wantStatus:        "open",
			wantServiceCalled: true,
			wantBodyTotal:     len(twoFlags),
			wantBodyFlagsLen:  len(twoFlags),
		},
		{
			name:              "service error → 500",
			query:             "",
			listError:         inventory.ErrCampaignNotFound,
			wantCode:          http.StatusInternalServerError,
			wantServiceCalled: true,
			wantBodyTotal:     -1,
			wantBodyFlagsLen:  -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedStatus string
			called := false
			svc := &mocks.MockPricingService{
				ListPriceFlagsFn: func(_ context.Context, status string) ([]inventory.PriceFlagWithContext, error) {
					called = true
					receivedStatus = status
					return tt.listResult, tt.listError
				},
			}
			h := newPriceFlagsHandler(svc)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/admin/price-flags"+tt.query, nil)
			h.HandleListPriceFlags(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status: got %d, want %d (body=%s)", rec.Code, tt.wantCode, rec.Body.String())
			}
			if called != tt.wantServiceCalled {
				t.Errorf("service called: got %t, want %t", called, tt.wantServiceCalled)
			}
			if tt.wantServiceCalled && tt.wantCode == http.StatusOK && receivedStatus != tt.wantStatus {
				t.Errorf("service status: got %q, want %q", receivedStatus, tt.wantStatus)
			}

			if tt.wantBodyTotal == -1 && tt.wantBodyFlagsLen == -1 {
				return
			}
			var body struct {
				Flags []inventory.PriceFlagWithContext `json:"flags"`
				Total int                              `json:"total"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if tt.wantBodyTotal >= 0 && body.Total != tt.wantBodyTotal {
				t.Errorf("total: got %d, want %d", body.Total, tt.wantBodyTotal)
			}
			if tt.wantBodyFlagsLen >= 0 && len(body.Flags) != tt.wantBodyFlagsLen {
				t.Errorf("flags length: got %d, want %d", len(body.Flags), tt.wantBodyFlagsLen)
			}
			if tt.wantNonNullFlags && body.Flags == nil {
				t.Errorf("flags should be empty array, not null")
			}
			// Sanity check on the populated case. Re-check actual length so a
			// length-mismatch error above (t.Errorf, not Fatalf) doesn't lead
			// to a panic indexing a too-short slice.
			if tt.wantBodyFlagsLen == len(twoFlags) && len(body.Flags) >= len(twoFlags) {
				if body.Flags[0].ID != twoFlags[0].ID || body.Flags[1].ID != twoFlags[1].ID {
					t.Errorf("flag IDs not preserved: got %v %v", body.Flags[0].ID, body.Flags[1].ID)
				}
			}
		})
	}
}

// --- HandleResolvePriceFlag ---

func TestHandleResolvePriceFlag(t *testing.T) {
	tests := []struct {
		name string
		// flagIDPath is the path-value passed via SetPathValue. Empty string
		// for cases where the test exercises a path that won't reach the
		// service.
		flagIDPath string
		// withAuth controls whether withUser() wraps the request.
		withAuth bool
		// resolveError is what the mock returns. nil means success.
		resolveError error

		wantCode             int
		wantServiceCalled    bool
		wantCapturedFlagID   int64
		wantCapturedResolver int64 // 42 from withUser
	}{
		{
			name:                 "success → 204 with captured IDs",
			flagIDPath:           "123",
			withAuth:             true,
			wantCode:             http.StatusNoContent,
			wantServiceCalled:    true,
			wantCapturedFlagID:   123,
			wantCapturedResolver: 42,
		},
		{
			name:              "invalid flagID → 400, service not called",
			flagIDPath:        "not-a-number",
			withAuth:          true,
			wantCode:          http.StatusBadRequest,
			wantServiceCalled: false,
		},
		{
			name:              "no user → 401, service not called",
			flagIDPath:        "1",
			withAuth:          false,
			wantCode:          http.StatusUnauthorized,
			wantServiceCalled: false,
		},
		{
			name:              "not found → 404",
			flagIDPath:        "999",
			withAuth:          true,
			resolveError:      inventory.ErrPriceFlagNotFound,
			wantCode:          http.StatusNotFound,
			wantServiceCalled: true,
		},
		{
			name:              "other service error → 500",
			flagIDPath:        "1",
			withAuth:          true,
			resolveError:      inventory.ErrCampaignNotFound,
			wantCode:          http.StatusInternalServerError,
			wantServiceCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedID, capturedUserID int64
			called := false
			svc := &mocks.MockPricingService{
				ResolvePriceFlagFn: func(_ context.Context, flagID int64, resolvedBy int64) error {
					called = true
					capturedID = flagID
					capturedUserID = resolvedBy
					return tt.resolveError
				},
			}
			h := newPriceFlagsHandler(svc)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, "/api/admin/price-flags/"+tt.flagIDPath+"/resolve", nil)
			req.SetPathValue("flagId", tt.flagIDPath)
			if tt.withAuth {
				req = withUser(req)
			}
			h.HandleResolvePriceFlag(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status: got %d, want %d (body=%s)", rec.Code, tt.wantCode, rec.Body.String())
			}
			if called != tt.wantServiceCalled {
				t.Errorf("service called: got %t, want %t", called, tt.wantServiceCalled)
			}
			if tt.wantServiceCalled && tt.resolveError == nil {
				if capturedID != tt.wantCapturedFlagID {
					t.Errorf("flagID captured: got %d, want %d", capturedID, tt.wantCapturedFlagID)
				}
				if capturedUserID != tt.wantCapturedResolver {
					t.Errorf("resolvedBy captured: got %d, want %d", capturedUserID, tt.wantCapturedResolver)
				}
			}
		})
	}
}
