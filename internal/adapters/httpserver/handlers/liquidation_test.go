package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/liquidation"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

type mockLiquidationService struct {
	PreviewFn func(ctx context.Context, req liquidation.PreviewRequest) (liquidation.PreviewResponse, error)
	ApplyFn   func(ctx context.Context, req liquidation.ApplyRequest) (liquidation.ApplyResult, error)
}

func (m *mockLiquidationService) Preview(ctx context.Context, req liquidation.PreviewRequest) (liquidation.PreviewResponse, error) {
	return m.PreviewFn(ctx, req)
}

func (m *mockLiquidationService) Apply(ctx context.Context, req liquidation.ApplyRequest) (liquidation.ApplyResult, error) {
	return m.ApplyFn(ctx, req)
}

func TestHandleLiquidationPreview(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		previewFn  func(ctx context.Context, req liquidation.PreviewRequest) (liquidation.PreviewResponse, error)
		wantStatus int
	}{
		{
			name: "valid request returns 200",
			body: `{"baseDiscountPct":10,"noCompDiscountPct":20}`,
			previewFn: func(_ context.Context, _ liquidation.PreviewRequest) (liquidation.PreviewResponse, error) {
				return liquidation.PreviewResponse{
					Items:   []liquidation.PreviewItem{},
					Summary: liquidation.PreviewSummary{TotalCards: 0},
				}, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid JSON returns 400",
			body:       `not-json`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockLiquidationService{PreviewFn: tc.previewFn}
			h := NewLiquidationHandler(svc, mocks.NewMockLogger())

			req := httptest.NewRequest(http.MethodPost, "/api/liquidation/preview", bytes.NewBufferString(tc.body))
			rec := httptest.NewRecorder()
			h.HandlePreview(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestHandleLiquidationApply(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		applyFn    func(ctx context.Context, req liquidation.ApplyRequest) (liquidation.ApplyResult, error)
		wantStatus int
		wantApplied int
	}{
		{
			name: "valid request returns 200 with applied count",
			body: `{"items":[{"purchaseId":"abc","newPriceCents":1000}]}`,
			applyFn: func(_ context.Context, _ liquidation.ApplyRequest) (liquidation.ApplyResult, error) {
				return liquidation.ApplyResult{Applied: 1}, nil
			},
			wantStatus:  http.StatusOK,
			wantApplied: 1,
		},
		{
			name:       "empty items returns 400",
			body:       `{"items":[]}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid JSON returns 400",
			body:       `not-json`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockLiquidationService{ApplyFn: tc.applyFn}
			h := NewLiquidationHandler(svc, mocks.NewMockLogger())

			req := httptest.NewRequest(http.MethodPost, "/api/liquidation/apply", bytes.NewBufferString(tc.body))
			rec := httptest.NewRecorder()
			h.HandleApply(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tc.wantStatus)
			}

			if tc.wantApplied > 0 {
				var result liquidation.ApplyResult
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if result.Applied != tc.wantApplied {
					t.Errorf("got Applied=%d, want %d", result.Applied, tc.wantApplied)
				}
			}
		})
	}
}
