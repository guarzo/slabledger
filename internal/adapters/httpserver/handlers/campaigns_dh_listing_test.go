package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func newTestHandlerWithDHListing(svc *mocks.MockInventoryService, dhSvc dhlisting.Service) *CampaignsHandler {
	return NewCampaignsHandler(svc, nil, nil, nil, mocks.NewMockLogger(), nil,
		WithFinanceService(&mocks.MockFinanceService{}),
		WithExportService(&mocks.MockExportService{}),
		WithDHListingService(dhSvc))
}

func TestHandleListPurchaseOnDH(t *testing.T) {
	const purchaseID = "p1"
	receivedAtStr := "2026-04-16T00:00:00Z"
	receivedAt := &receivedAtStr

	readyPurchase := func() *inventory.Purchase {
		return &inventory.Purchase{
			ID:            purchaseID,
			CertNumber:    "CERT123",
			ReceivedAt:    receivedAt,
			DHInventoryID: 42,
			DHStatus:      inventory.DHStatusInStock,
		}
	}

	tests := []struct {
		name           string
		getPurchase    func(ctx context.Context, id string) (*inventory.Purchase, error)
		listFn         func(ctx context.Context, certs []string) dhlisting.DHListingResult
		dhSvc          dhlisting.Service // if nil, a default mock with listFn is used; if explicitly nil-interface, handler runs without dhListingSvc
		omitDHSvc      bool
		wantStatus     int
		wantErrSubstr  string
		wantListedFrom []string // if set, verifies the cert numbers passed to ListPurchases
	}{
		{
			name:        "success lists purchase",
			getPurchase: func(ctx context.Context, id string) (*inventory.Purchase, error) { return readyPurchase(), nil },
			listFn: func(ctx context.Context, certs []string) dhlisting.DHListingResult {
				return dhlisting.DHListingResult{Listed: 1, Synced: 1, Total: 1}
			},
			wantStatus:     http.StatusOK,
			wantListedFrom: []string{"CERT123"},
		},
		{
			name:          "purchase not found → 404",
			getPurchase:   func(ctx context.Context, id string) (*inventory.Purchase, error) { return nil, inventory.ErrPurchaseNotFound },
			wantStatus:    http.StatusNotFound,
			wantErrSubstr: "not found",
		},
		{
			name: "not received → 409",
			getPurchase: func(ctx context.Context, id string) (*inventory.Purchase, error) {
				p := readyPurchase()
				p.ReceivedAt = nil
				return p, nil
			},
			wantStatus:    http.StatusConflict,
			wantErrSubstr: "received",
		},
		{
			name: "not yet pushed to DH → 409",
			getPurchase: func(ctx context.Context, id string) (*inventory.Purchase, error) {
				p := readyPurchase()
				p.DHInventoryID = 0
				return p, nil
			},
			wantStatus:    http.StatusConflict,
			wantErrSubstr: "not yet pushed",
		},
		{
			name: "already listed → 409",
			getPurchase: func(ctx context.Context, id string) (*inventory.Purchase, error) {
				p := readyPurchase()
				p.DHStatus = inventory.DHStatusListed
				return p, nil
			},
			wantStatus:    http.StatusConflict,
			wantErrSubstr: "already listed",
		},
		{
			name:          "dh listing service not configured → 503",
			omitDHSvc:     true,
			wantStatus:    http.StatusServiceUnavailable,
			wantErrSubstr: "not configured",
		},
		{
			name:        "listing returns zero listed → 500",
			getPurchase: func(ctx context.Context, id string) (*inventory.Purchase, error) { return readyPurchase(), nil },
			listFn: func(ctx context.Context, certs []string) dhlisting.DHListingResult {
				return dhlisting.DHListingResult{Listed: 0, Total: 1}
			},
			wantStatus:    http.StatusInternalServerError,
			wantErrSubstr: "did not list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockInventoryService{GetPurchaseFn: tt.getPurchase}

			var capturedCerts []string
			var dhSvc dhlisting.Service
			if !tt.omitDHSvc {
				dhSvc = &mocks.MockDHListingService{
					ListPurchasesFn: func(ctx context.Context, certs []string) dhlisting.DHListingResult {
						capturedCerts = certs
						if tt.listFn != nil {
							return tt.listFn(ctx, certs)
						}
						return dhlisting.DHListingResult{}
					},
				}
			}

			h := newTestHandlerWithDHListing(svc, dhSvc)

			req := httptest.NewRequest(http.MethodPost, "/api/purchases/"+purchaseID+"/list-on-dh", nil)
			req.SetPathValue("purchaseId", purchaseID)
			rec := httptest.NewRecorder()

			h.HandleListPurchaseOnDH(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantErrSubstr != "" {
				msg := decodeErrorResponse(t, rec)
				if !containsSubstring(msg, tt.wantErrSubstr) {
					t.Errorf("error body = %q, want substring %q", msg, tt.wantErrSubstr)
				}
			}
			if tt.wantListedFrom != nil {
				if len(capturedCerts) != len(tt.wantListedFrom) || (len(capturedCerts) > 0 && capturedCerts[0] != tt.wantListedFrom[0]) {
					t.Errorf("ListPurchases called with %v, want %v", capturedCerts, tt.wantListedFrom)
				}
			}
		})
	}
}

func containsSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
