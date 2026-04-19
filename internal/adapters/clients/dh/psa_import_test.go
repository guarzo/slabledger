package dh

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
)

func TestPSAImport_SendsRequest(t *testing.T) {
	var capturedBody PSAImportRequest
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &capturedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PSAImportResponse{
			Results: []PSAImportResult{{
				CertNumber:    "12345678",
				Resolution:    PSAImportStatusUnmatchedCreated,
				DHCardID:      9001,
				DHInventoryID: 555,
				Status:        "in_stock",
			}},
			Summary: PSAImportSummary{UnmatchedCreated: 1},
		})
	}))
	defer server.Close()

	c := newTestClient(server.URL)
	c.psaKeys = []string{"psa-key-abc"}

	resp, err := c.PSAImport(context.Background(), []PSAImportItem{{
		CertNumber:     "12345678",
		CostBasisCents: 5000,
		Overrides:      &PSAImportOverrides{Name: "Charizard", Language: PSAImportLanguageGerman},
	}})
	if err != nil {
		t.Fatalf("PSAImport error: %v", err)
	}

	if capturedPath != "/api/v1/enterprise/inventory/psa_import" {
		t.Errorf("path = %q, want /api/v1/enterprise/inventory/psa_import", capturedPath)
	}
	if capturedBody.PSAAPIKey != "psa-key-abc" {
		t.Errorf("psa_api_key = %q, want psa-key-abc", capturedBody.PSAAPIKey)
	}
	if len(capturedBody.Items) != 1 || capturedBody.Items[0].CertNumber != "12345678" {
		t.Errorf("unexpected items in request: %+v", capturedBody.Items)
	}
	if len(resp.Results) != 1 || resp.Results[0].DHCardID != 9001 {
		t.Errorf("unexpected response results: %+v", resp.Results)
	}
}

func TestPSAImport_RejectsEmptyItems(t *testing.T) {
	c := newTestClient("http://unused")
	c.psaKeys = []string{"k"}
	_, err := c.PSAImport(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected empty-items error, got: %v", err)
	}
}

func TestPSAImport_RejectsOverMax(t *testing.T) {
	c := newTestClient("http://unused")
	c.psaKeys = []string{"k"}
	items := make([]PSAImportItem, PSAImportMaxItems+1)
	for i := range items {
		items[i] = PSAImportItem{CertNumber: "x"}
	}
	_, err := c.PSAImport(context.Background(), items)
	if err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Errorf("expected max-items error, got: %v", err)
	}
}

func TestPSAImport_RequiresPSAKey(t *testing.T) {
	c := newTestClient("http://unused")
	// no psaKeys configured
	_, err := c.PSAImport(context.Background(), []PSAImportItem{{CertNumber: "1"}})
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperrors.ErrCodeConfigMissing {
		t.Errorf("expected ConfigMissing error, got: %v", err)
	}
}
