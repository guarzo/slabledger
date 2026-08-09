package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/csvimport"
	"github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// --- HandleImportCertSales ---

func TestHandleImportCertSales(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		setupSvc func() *mocks.MockImportService
		notWired bool
		wantCode int
		check    func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name: "success",
			body: `{"saleDate":"2026-08-09","saleChannel":"local","negotiatedPct":0.85,"items":[{"certNumber":"12345678","theirCompCents":10000}]}`,
			setupSvc: func() *mocks.MockImportService {
				return &mocks.MockImportService{
					ImportCertSalesFn: func(_ context.Context, req csvimport.CertSaleImportRequest) (*csvimport.CertSaleImportResult, error) {
						if len(req.Items) != 1 {
							t.Errorf("items = %d, want 1", len(req.Items))
						}
						return &csvimport.CertSaleImportResult{ComputedTotalCents: 8500}, nil
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var result csvimport.CertSaleImportResult
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if result.ComputedTotalCents != 8500 {
					t.Errorf("computedTotalCents = %d, want 8500", result.ComputedTotalCents)
				}
			},
		},
		{
			name:     "empty items rejected",
			body:     `{"saleDate":"2026-08-09","saleChannel":"local","negotiatedPct":0.85,"items":[]}`,
			setupSvc: func() *mocks.MockImportService { return &mocks.MockImportService{} },
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "malformed JSON",
			body:     `not json`,
			setupSvc: func() *mocks.MockImportService { return &mocks.MockImportService{} },
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "import service not wired",
			body:     `{"saleDate":"2026-08-09","saleChannel":"local","negotiatedPct":0.85,"items":[{"certNumber":"12345678","theirCompCents":10000}]}`,
			setupSvc: func() *mocks.MockImportService { return &mocks.MockImportService{} },
			notWired: true,
			wantCode: http.StatusServiceUnavailable,
		},
		{
			name: "service error",
			body: `{"saleDate":"2026-08-09","saleChannel":"local","negotiatedPct":0.85,"items":[{"certNumber":"12345678","theirCompCents":10000}]}`,
			setupSvc: func() *mocks.MockImportService {
				return &mocks.MockImportService{
					ImportCertSalesFn: func(_ context.Context, _ csvimport.CertSaleImportRequest) (*csvimport.CertSaleImportResult, error) {
						return nil, fmt.Errorf("database failure")
					},
				}
			},
			wantCode: http.StatusInternalServerError,
		},
		{
			// negotiatedPct is out of (0, 1] here (72 instead of 0.72) — a caller
			// mistake the service already classifies as ErrCodeValidation
			// (service_import_certs.go:19-21). It must surface as 400 with the
			// service's own message, not the generic 500 the pre-fix code returned.
			name: "whole-batch validation error maps to 400",
			body: `{"saleDate":"2026-08-09","saleChannel":"local","negotiatedPct":72,"items":[{"certNumber":"12345678","theirCompCents":10000}]}`,
			setupSvc: func() *mocks.MockImportService {
				return &mocks.MockImportService{
					ImportCertSalesFn: func(_ context.Context, _ csvimport.CertSaleImportRequest) (*csvimport.CertSaleImportResult, error) {
						return nil, errors.NewAppError(errors.ErrCodeValidation, "negotiatedPct must be in (0, 1], got 72")
					},
				}
			},
			wantCode: http.StatusBadRequest,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var body map[string]string
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if body["error"] != "negotiatedPct must be in (0, 1], got 72" {
					t.Errorf("error = %q, want service's validation message", body["error"])
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var h *CampaignsHandler
			if tc.notWired {
				h = NewCampaignsHandler(&mocks.MockInventoryService{}, nil, nil, nil,
					mocks.NewMockLogger(), nil,
					WithFinanceService(&mocks.MockFinanceService{}),
					WithExportService(&mocks.MockExportService{}))
			} else {
				h = newTestHandlerWithImport(tc.setupSvc())
			}

			req := httptest.NewRequest(http.MethodPost, "/api/sales/import-certs", bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.HandleImportCertSales(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tc.wantCode, rec.Body.String())
			}
			if tc.check != nil {
				tc.check(t, rec)
			}
		})
	}
}

// --- HandleConfirmCertSales ---

func TestHandleConfirmCertSales(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		setupSvc func() *mocks.MockImportService
		notWired bool
		wantCode int
		check    func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name: "success",
			body: `[{"purchaseId":"p1","saleChannel":"local","saleDate":"2026-08-09","salePriceCents":8500,"theirCompCents":10000,"priceSource":"itemized"}]`,
			setupSvc: func() *mocks.MockImportService {
				return &mocks.MockImportService{
					ConfirmOrdersSalesFn: func(_ context.Context, items []csvimport.OrdersConfirmItem) (*inventory.BulkSaleResult, error) {
						if len(items) != 1 {
							t.Errorf("items = %d, want 1", len(items))
						}
						return &inventory.BulkSaleResult{Created: 1}, nil
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var result inventory.BulkSaleResult
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if result.Created != 1 {
					t.Errorf("created = %d, want 1", result.Created)
				}
			},
		},
		{
			name:     "empty items rejected",
			body:     `[]`,
			setupSvc: func() *mocks.MockImportService { return &mocks.MockImportService{} },
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "malformed JSON",
			body:     `not json`,
			setupSvc: func() *mocks.MockImportService { return &mocks.MockImportService{} },
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "import service not wired",
			body:     `[{"purchaseId":"p1","saleChannel":"local","saleDate":"2026-08-09","salePriceCents":8500}]`,
			setupSvc: func() *mocks.MockImportService { return &mocks.MockImportService{} },
			notWired: true,
			wantCode: http.StatusServiceUnavailable,
		},
		{
			name: "service error",
			body: `[{"purchaseId":"p1","saleChannel":"local","saleDate":"2026-08-09","salePriceCents":8500}]`,
			setupSvc: func() *mocks.MockImportService {
				return &mocks.MockImportService{
					ConfirmOrdersSalesFn: func(_ context.Context, _ []csvimport.OrdersConfirmItem) (*inventory.BulkSaleResult, error) {
						return nil, fmt.Errorf("database failure")
					},
				}
			},
			wantCode: http.StatusInternalServerError,
		},
		{
			// Same mapping applied here as HandleImportCertSales, for consistency
			// even though ConfirmOrdersSales does not currently return
			// ErrCodeValidation for a whole batch — a future validation added
			// there should not silently regress to a 500.
			name: "whole-batch validation error maps to 400",
			body: `[{"purchaseId":"p1","saleChannel":"local","saleDate":"2026-08-09","salePriceCents":8500}]`,
			setupSvc: func() *mocks.MockImportService {
				return &mocks.MockImportService{
					ConfirmOrdersSalesFn: func(_ context.Context, _ []csvimport.OrdersConfirmItem) (*inventory.BulkSaleResult, error) {
						return nil, errors.NewAppError(errors.ErrCodeValidation, "saleChannel is invalid")
					},
				}
			},
			wantCode: http.StatusBadRequest,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var body map[string]string
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if body["error"] != "saleChannel is invalid" {
					t.Errorf("error = %q, want service's validation message", body["error"])
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var h *CampaignsHandler
			if tc.notWired {
				h = NewCampaignsHandler(&mocks.MockInventoryService{}, nil, nil, nil,
					mocks.NewMockLogger(), nil,
					WithFinanceService(&mocks.MockFinanceService{}),
					WithExportService(&mocks.MockExportService{}))
			} else {
				h = newTestHandlerWithImport(tc.setupSvc())
			}

			req := httptest.NewRequest(http.MethodPost, "/api/sales/import-certs/confirm", bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.HandleConfirmCertSales(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tc.wantCode, rec.Body.String())
			}
			if tc.check != nil {
				tc.check(t, rec)
			}
		})
	}
}
