package handlers

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// --- createCSVMultipart helper ---

// createCSVMultipart creates a multipart form body with a CSV file attached under field "file".
func createCSVMultipart(t *testing.T, rows [][]string) (*bytes.Buffer, string) {
	t.Helper()
	var csvBuf bytes.Buffer
	w := csv.NewWriter(&csvBuf)
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			t.Fatalf("csv write: %v", err)
		}
	}
	w.Flush()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "test.csv")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(csvBuf.Bytes()); err != nil {
		t.Fatalf("write csv to part: %v", err)
	}
	writer.Close()
	return &body, writer.FormDataContentType()
}

// --- HandleImportCerts ---

func TestHandleImportCerts_Success(t *testing.T) {
	svc := &mocks.MockInventoryService{
		ImportCertsFn: func(_ context.Context, certs []string) (*inventory.CertImportResult, error) {
			return &inventory.CertImportResult{
				Imported:       len(certs),
				AlreadyExisted: 0,
				Failed:         0,
			}, nil
		},
	}
	h := newTestHandler(svc)

	body := strings.NewReader(`{"certNumbers":["111","222"]}`)
	req := httptest.NewRequest("POST", "/api/purchases/import-certs", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleImportCerts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var result inventory.CertImportResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Imported != 2 {
		t.Errorf("imported = %d, want 2", result.Imported)
	}
}

func TestHandleImportCerts_EmptyCerts(t *testing.T) {
	h := newTestHandler(&mocks.MockInventoryService{})

	body := strings.NewReader(`{"certNumbers":[]}`)
	req := httptest.NewRequest("POST", "/api/purchases/import-certs", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleImportCerts(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleImportCerts_InvalidJSON(t *testing.T) {
	h := newTestHandler(&mocks.MockInventoryService{})

	body := strings.NewReader(`not json`)
	req := httptest.NewRequest("POST", "/api/purchases/import-certs", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleImportCerts(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleImportCerts_ServiceError(t *testing.T) {
	svc := &mocks.MockInventoryService{
		ImportCertsFn: func(_ context.Context, _ []string) (*inventory.CertImportResult, error) {
			return nil, fmt.Errorf("database failure")
		},
	}
	h := newTestHandler(svc)

	body := strings.NewReader(`{"certNumbers":["111"]}`)
	req := httptest.NewRequest("POST", "/api/purchases/import-certs", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleImportCerts(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", rec.Code, rec.Body.String())
	}
}

// --- HandleScanCert ---

func TestHandleScanCert_Existing(t *testing.T) {
	svc := &mocks.MockInventoryService{
		ScanCertFn: func(_ context.Context, cert string) (*inventory.ScanCertResult, error) {
			return &inventory.ScanCertResult{
				Status:     "existing",
				CardName:   "Charizard PSA 10",
				PurchaseID: "p1",
				CampaignID: "c1",
			}, nil
		},
	}
	h := newTestHandler(svc)

	body := strings.NewReader(`{"certNumber":"12345678"}`)
	req := httptest.NewRequest("POST", "/api/purchases/scan-cert", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleScanCert(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var result inventory.ScanCertResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "existing" {
		t.Errorf("status = %q, want existing", result.Status)
	}
	if result.CardName != "Charizard PSA 10" {
		t.Errorf("cardName = %q, want Charizard PSA 10", result.CardName)
	}
}

func TestHandleScanCert_EmptyCert(t *testing.T) {
	h := newTestHandler(&mocks.MockInventoryService{})

	body := strings.NewReader(`{"certNumber":""}`)
	req := httptest.NewRequest("POST", "/api/purchases/scan-cert", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleScanCert(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleScanCert_InvalidJSON(t *testing.T) {
	h := newTestHandler(&mocks.MockInventoryService{})

	body := strings.NewReader(`not json`)
	req := httptest.NewRequest("POST", "/api/purchases/scan-cert", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleScanCert(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleScanCert_ServiceError(t *testing.T) {
	svc := &mocks.MockInventoryService{
		ScanCertFn: func(_ context.Context, _ string) (*inventory.ScanCertResult, error) {
			return nil, fmt.Errorf("database failure")
		},
	}
	h := newTestHandler(svc)

	body := strings.NewReader(`{"certNumber":"111"}`)
	req := httptest.NewRequest("POST", "/api/purchases/scan-cert", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleScanCert(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", rec.Code, rec.Body.String())
	}
}

// --- HandleResolveCert ---

func TestHandleResolveCert_Success(t *testing.T) {
	svc := &mocks.MockInventoryService{
		ResolveCertFn: func(_ context.Context, cert string) (*inventory.CertInfo, error) {
			return &inventory.CertInfo{
				CertNumber: cert, CardName: "Umbreon VMAX", Grade: 10,
				Year: "2022", Category: "EVOLVING SKIES",
				Subject: "2022 Pokemon Evolving Skies Umbreon VMAX",
			}, nil
		},
	}
	h := newTestHandler(svc)

	body := strings.NewReader(`{"certNumber":"91234567"}`)
	req := httptest.NewRequest("POST", "/api/purchases/resolve-cert", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleResolveCert(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var result inventory.ResolveCertResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.CardName != "Umbreon VMAX" {
		t.Errorf("cardName = %q, want Umbreon VMAX", result.CardName)
	}
	if result.Grade != 10 {
		t.Errorf("grade = %v, want 10", result.Grade)
	}
}

func TestHandleResolveCert_NotFound(t *testing.T) {
	svc := &mocks.MockInventoryService{
		ResolveCertFn: func(_ context.Context, _ string) (*inventory.CertInfo, error) {
			return nil, inventory.ErrCertNotFound
		},
	}
	h := newTestHandler(svc)

	body := strings.NewReader(`{"certNumber":"00000000"}`)
	req := httptest.NewRequest("POST", "/api/purchases/resolve-cert", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleResolveCert(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleResolveCert_ServiceError(t *testing.T) {
	svc := &mocks.MockInventoryService{
		ResolveCertFn: func(_ context.Context, _ string) (*inventory.CertInfo, error) {
			return nil, fmt.Errorf("PSA API timeout")
		},
	}
	h := newTestHandler(svc)

	body := strings.NewReader(`{"certNumber":"11111111"}`)
	req := httptest.NewRequest("POST", "/api/purchases/resolve-cert", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleResolveCert(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleResolveCert_EmptyCert(t *testing.T) {
	h := newTestHandler(&mocks.MockInventoryService{})

	body := strings.NewReader(`{"certNumber":""}`)
	req := httptest.NewRequest("POST", "/api/purchases/resolve-cert", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.HandleResolveCert(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

// --- HandleGlobalImportPSA ---

func TestHandleGlobalImportPSA(t *testing.T) {
	tests := []struct {
		name     string
		setupReq func(t *testing.T) (*bytes.Buffer, string)
		setupSvc func() *mocks.MockInventoryService
		wantCode int
		check    func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name: "success",
			setupReq: func(t *testing.T) (*bytes.Buffer, string) {
				t.Helper()
				return createCSVMultipart(t, [][]string{
					{"cert number", "listing title", "grade"},
					{"12345678", "2020 Pokémon Charizard PSA 9", "9"},
				})
			},
			setupSvc: func() *mocks.MockInventoryService {
				return &mocks.MockInventoryService{
					ImportPSAExportGlobalFn: func(_ context.Context, rows []inventory.PSAExportRow) (*inventory.PSAImportResult, error) {
						return &inventory.PSAImportResult{Allocated: len(rows)}, nil
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var result inventory.PSAImportResult
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if result.Allocated != 1 {
					t.Errorf("expected Allocated=1, got %d", result.Allocated)
				}
			},
		},
		{
			name: "missing file",
			setupReq: func(t *testing.T) (*bytes.Buffer, string) {
				t.Helper()
				var buf bytes.Buffer
				writer := multipart.NewWriter(&buf)
				writer.Close()
				return &buf, writer.FormDataContentType()
			},
			setupSvc: func() *mocks.MockInventoryService { return &mocks.MockInventoryService{} },
			wantCode: http.StatusBadRequest,
		},
		{
			name: "invalid header",
			setupReq: func(t *testing.T) (*bytes.Buffer, string) {
				t.Helper()
				return createCSVMultipart(t, [][]string{
					{"wrong column", "bad column", "nope"},
					{"12345678", "Charizard", "9"},
				})
			},
			setupSvc: func() *mocks.MockInventoryService { return &mocks.MockInventoryService{} },
			wantCode: http.StatusBadRequest,
		},
		{
			name: "service error",
			setupReq: func(t *testing.T) (*bytes.Buffer, string) {
				t.Helper()
				return createCSVMultipart(t, [][]string{
					{"cert number", "listing title", "grade"},
					{"12345678", "Charizard PSA 9", "9"},
				})
			},
			setupSvc: func() *mocks.MockInventoryService {
				return &mocks.MockInventoryService{
					ImportPSAExportGlobalFn: func(_ context.Context, _ []inventory.PSAExportRow) (*inventory.PSAImportResult, error) {
						return nil, fmt.Errorf("database failure")
					},
				}
			},
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(tc.setupSvc())
			body, contentType := tc.setupReq(t)
			req := httptest.NewRequest(http.MethodPost, "/api/purchases/import-psa", body)
			req.Header.Set("Content-Type", contentType)
			rec := httptest.NewRecorder()
			h.HandleGlobalImportPSA(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d; body: %s", tc.wantCode, rec.Code, rec.Body.String())
			}
			if tc.check != nil {
				tc.check(t, rec)
			}
		})
	}
}

// --- HandleSyncPSASheets ---

func TestHandleSyncPSASheets(t *testing.T) {
	tests := []struct {
		name         string
		setupHandler func(svc *mocks.MockInventoryService) *CampaignsHandler
		setupSvc     func() *mocks.MockInventoryService
		wantCode     int
		check        func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name: "not configured",
			setupHandler: func(svc *mocks.MockInventoryService) *CampaignsHandler {
				return newTestHandler(svc) // no WithSheetFetcher
			},
			setupSvc: func() *mocks.MockInventoryService { return &mocks.MockInventoryService{} },
			wantCode: http.StatusServiceUnavailable,
		},
		{
			name: "success",
			setupHandler: func(svc *mocks.MockInventoryService) *CampaignsHandler {
				fetcher := &mocks.MockSheetFetcher{
					ReadSheetFn: func(_ context.Context, _, _ string) ([][]string, error) {
						return [][]string{
							{"cert number", "listing title", "grade"},
							{"12345678", "Charizard PSA 9", "9"},
						}, nil
					},
				}
				return NewCampaignsHandler(svc, nil, nil, nil, mocks.NewMockLogger(), nil, WithSheetFetcher(fetcher, "sheet-id", "Sheet1"))
			},
			setupSvc: func() *mocks.MockInventoryService {
				return &mocks.MockInventoryService{
					ImportPSAExportGlobalFn: func(_ context.Context, rows []inventory.PSAExportRow) (*inventory.PSAImportResult, error) {
						return &inventory.PSAImportResult{Allocated: len(rows)}, nil
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var result inventory.PSAImportResult
				if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if result.Allocated != 1 {
					t.Errorf("expected Allocated=1, got %d", result.Allocated)
				}
			},
		},
		{
			name: "sheet fetch error",
			setupHandler: func(svc *mocks.MockInventoryService) *CampaignsHandler {
				fetcher := &mocks.MockSheetFetcher{
					ReadSheetFn: func(_ context.Context, _, _ string) ([][]string, error) {
						return nil, fmt.Errorf("google sheets API error")
					},
				}
				return NewCampaignsHandler(svc, nil, nil, nil, mocks.NewMockLogger(), nil, WithSheetFetcher(fetcher, "sheet-id", "Sheet1"))
			},
			setupSvc: func() *mocks.MockInventoryService { return &mocks.MockInventoryService{} },
			wantCode: http.StatusBadGateway,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := tc.setupSvc()
			h := tc.setupHandler(svc)
			req := httptest.NewRequest(http.MethodPost, "/api/purchases/sync-psa-sheets", nil)
			rec := httptest.NewRecorder()
			h.HandleSyncPSASheets(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d; body: %s", tc.wantCode, rec.Code, rec.Body.String())
			}
			if tc.check != nil {
				tc.check(t, rec)
			}
		})
	}
}
