package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestCertIntake_ReceiptAndPollingDoNotListWithoutPriceReview(t *testing.T) {
	for _, tc := range []struct {
		name, path, body string
		handle           func(*CampaignsHandler, http.ResponseWriter, *http.Request)
	}{
		{"import", "/api/purchases/import-certs", `{"certNumbers":["123"]}`, (*CampaignsHandler).HandleImportCerts},
		{"scan", "/api/purchases/scan-cert", `{"certNumber":"123"}`, (*CampaignsHandler).HandleScanCert},
		{"batch poll", "/api/purchases/scan-certs", `{"certNumbers":["123"]}`, (*CampaignsHandler).HandleScanCerts},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var listed atomic.Int32
			svc := &mocks.MockInventoryService{
				ImportCertsFn: func(context.Context, []string) (*inventory.CertImportResult, error) {
					return &inventory.CertImportResult{Imported: 1}, nil
				},
				ScanCertFn: func(context.Context, string) (*inventory.ScanCertResult, error) {
					return &inventory.ScanCertResult{Status: "existing", PurchaseID: "p1"}, nil
				},
				ScanCertsFn: func(context.Context, []string) (*inventory.ScanCertsResult, error) {
					return &inventory.ScanCertsResult{Results: map[string]*inventory.ScanCertResult{"123": {Status: "existing", PurchaseID: "p1"}}}, nil
				},
			}
			lister := &mocks.MockDHListingService{ListPurchasesFn: func(context.Context, []string) dhlisting.DHListingResult {
				listed.Add(1)
				return dhlisting.DHListingResult{Listed: 1}
			}}
			h := NewCampaignsHandler(svc, nil, nil, nil, mocks.NewMockLogger(), context.Background(), WithDHListingService(lister))
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			tc.handle(h, rec, req)
			h.WaitBackground()
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if n := listed.Load(); n != 0 {
				t.Errorf("listing calls = %d, want none before manual price review", n)
			}
		})
	}
}
