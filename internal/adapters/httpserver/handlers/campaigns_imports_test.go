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

	"github.com/guarzo/slabledger/internal/domain/campaigns"
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

// --- HandleGlobalExportCL ---

func TestHandleGlobalExportCL_Success(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ExportCLFormatGlobalFn: func(_ context.Context, _ bool) ([]campaigns.CLExportEntry, error) {
			return []campaigns.CLExportEntry{
				{DatePurchased: "3/9/2026", CertNumber: "12345678", Grader: "PSA", Investment: 150.00, EstimatedValue: 200.00},
			}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/purchases/export-cl", nil)
	rec := httptest.NewRecorder()
	h.HandleGlobalExportCL(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/csv" {
		t.Errorf("Content-Type = %q, want text/csv", ct)
	}

	// Parse the CSV to verify structure
	reader := csv.NewReader(rec.Body)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v", err)
	}
	// header + 1 data row
	if len(records) != 2 {
		t.Fatalf("expected 2 rows (header + data), got %d", len(records))
	}
	if records[0][0] != "Date Purchased" {
		t.Errorf("header[0] = %q, want 'Date Purchased'", records[0][0])
	}
	if records[1][1] != "12345678" {
		t.Errorf("data cert = %q, want 12345678", records[1][1])
	}
}

func TestHandleGlobalExportCL_Empty(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ExportCLFormatGlobalFn: func(_ context.Context, _ bool) ([]campaigns.CLExportEntry, error) {
			return []campaigns.CLExportEntry{}, nil
		},
	}
	h := newTestHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/purchases/export-cl", nil)
	rec := httptest.NewRecorder()
	h.HandleGlobalExportCL(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	reader := csv.NewReader(rec.Body)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v", err)
	}
	// Only header row
	if len(records) != 1 {
		t.Fatalf("expected 1 row (header only), got %d", len(records))
	}
}

// --- HandleGlobalRefreshCL ---

func TestHandleGlobalRefreshCL_Success(t *testing.T) {
	var capturedRows []campaigns.CLExportRow
	svc := &mocks.MockCampaignService{
		RefreshCLValuesGlobalFn: func(_ context.Context, rows []campaigns.CLExportRow) (*campaigns.GlobalCLRefreshResult, error) {
			capturedRows = rows
			return &campaigns.GlobalCLRefreshResult{Updated: len(rows)}, nil
		},
	}
	h := newTestHandler(svc)

	body, contentType := createCSVMultipart(t, [][]string{
		{"Slab Serial #", "Current Value", "Population"},
		{"CERT001", "250.00", "150"},
		{"CERT002", "120.00", "200"},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/purchases/refresh-cl", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.HandleGlobalRefreshCL(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if len(capturedRows) != 2 {
		t.Errorf("expected 2 rows passed to service, got %d", len(capturedRows))
	}

	var result campaigns.GlobalCLRefreshResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Updated != 2 {
		t.Errorf("Updated = %d, want 2", result.Updated)
	}
}

func TestHandleGlobalRefreshCL_MissingFile(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/purchases/refresh-cl", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	h.HandleGlobalRefreshCL(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGlobalRefreshCL_BadCSVHeader(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	body, contentType := createCSVMultipart(t, [][]string{
		{"Wrong Column", "Current Value"},
		{"CERT001", "100.00"},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/purchases/refresh-cl", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.HandleGlobalRefreshCL(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	errMsg := decodeErrorResponse(t, rec)
	if !strings.Contains(strings.ToLower(errMsg), "slab serial") {
		t.Errorf("expected error about missing slab serial column, got: %s", errMsg)
	}
}

func TestHandleGlobalRefreshCL_ServiceError(t *testing.T) {
	svc := &mocks.MockCampaignService{
		RefreshCLValuesGlobalFn: func(_ context.Context, _ []campaigns.CLExportRow) (*campaigns.GlobalCLRefreshResult, error) {
			return nil, fmt.Errorf("database failure")
		},
	}
	h := newTestHandler(svc)

	body, contentType := createCSVMultipart(t, [][]string{
		{"Slab Serial #", "Current Value"},
		{"CERT001", "250.00"},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/purchases/refresh-cl", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.HandleGlobalRefreshCL(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// --- HandleGlobalImportCL ---

func TestHandleGlobalImportCL_Success(t *testing.T) {
	var capturedRows []campaigns.CLExportRow
	svc := &mocks.MockCampaignService{
		ImportCLExportGlobalFn: func(_ context.Context, rows []campaigns.CLExportRow) (*campaigns.GlobalImportResult, error) {
			capturedRows = rows
			return &campaigns.GlobalImportResult{Allocated: len(rows)}, nil
		},
	}
	h := newTestHandler(svc)

	body, contentType := createCSVMultipart(t, [][]string{
		{"Slab Serial #", "Investment", "Current Value", "Card", "Condition", "Date Purchased"},
		{"NEW001", "150.00", "300.00", "Charizard PSA 9", "PSA 9", "3/9/2026"},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/purchases/import-cl", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.HandleGlobalImportCL(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if len(capturedRows) != 1 {
		t.Errorf("expected 1 row passed to service, got %d", len(capturedRows))
	}
	if capturedRows[0].SlabSerial != "NEW001" {
		t.Errorf("SlabSerial = %q, want NEW001", capturedRows[0].SlabSerial)
	}
	// Date should be converted from M/D/YYYY to YYYY-MM-DD
	if capturedRows[0].DatePurchased != "2026-03-09" {
		t.Errorf("DatePurchased = %q, want 2026-03-09", capturedRows[0].DatePurchased)
	}
}

func TestHandleGlobalImportCL_MissingFile(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/purchases/import-cl", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	h.HandleGlobalImportCL(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGlobalImportCL_BadCSVHeader(t *testing.T) {
	h := newTestHandler(&mocks.MockCampaignService{})

	body, contentType := createCSVMultipart(t, [][]string{
		{"Wrong Column", "Another Bad Column", "Nope"},
		{"val1", "val2", "val3"},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/purchases/import-cl", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.HandleGlobalImportCL(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	errMsg := decodeErrorResponse(t, rec)
	if !strings.Contains(strings.ToLower(errMsg), "missing required column") {
		t.Errorf("expected error about missing required column, got: %s", errMsg)
	}
}

func TestHandleGlobalImportCL_BadDateFormat(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ImportCLExportGlobalFn: func(_ context.Context, rows []campaigns.CLExportRow) (*campaigns.GlobalImportResult, error) {
			return &campaigns.GlobalImportResult{Allocated: len(rows)}, nil
		},
	}
	h := newTestHandler(svc)

	body, contentType := createCSVMultipart(t, [][]string{
		{"Slab Serial #", "Investment", "Current Value", "Card", "Condition", "Date Purchased"},
		{"NEW001", "150.00", "300.00", "Charizard PSA 9", "PSA 9", "2026-03-09"},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/purchases/import-cl", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.HandleGlobalImportCL(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad date format, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid Date Purchased") {
		t.Errorf("expected error about invalid date, got: %s", rec.Body.String())
	}
}

func TestHandleGlobalImportCL_ServiceError(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ImportCLExportGlobalFn: func(_ context.Context, _ []campaigns.CLExportRow) (*campaigns.GlobalImportResult, error) {
			return nil, fmt.Errorf("database failure")
		},
	}
	h := newTestHandler(svc)

	body, contentType := createCSVMultipart(t, [][]string{
		{"Slab Serial #", "Investment", "Current Value", "Card", "Condition", "Date Purchased"},
		{"NEW001", "150.00", "300.00", "Charizard PSA 9", "PSA 9", "3/9/2026"},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/purchases/import-cl", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.HandleGlobalImportCL(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body: %s", rec.Code, rec.Body.String())
	}
}
