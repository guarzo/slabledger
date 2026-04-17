package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
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
			ID:                 purchaseID,
			CertNumber:         "CERT123",
			ReceivedAt:         receivedAt,
			DHInventoryID:      42,
			DHStatus:           inventory.DHStatusInStock,
			ReviewedPriceCents: 50000, // required: DH now honors listing_price_cents as-is
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
			name: "purchase not found → 404",
			getPurchase: func(ctx context.Context, id string) (*inventory.Purchase, error) {
				return nil, inventory.ErrPurchaseNotFound
			},
			wantStatus:    http.StatusNotFound,
			wantErrSubstr: "not found",
		},
		{
			name:          "inventory service error → 500",
			getPurchase:   func(ctx context.Context, id string) (*inventory.Purchase, error) { return nil, errors.New("db error") },
			wantStatus:    http.StatusInternalServerError,
			wantErrSubstr: "Internal",
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
			name: "not yet pushed to DH and not pending → 409",
			getPurchase: func(ctx context.Context, id string) (*inventory.Purchase, error) {
				p := readyPurchase()
				p.DHInventoryID = 0
				return p, nil
			},
			wantStatus:    http.StatusConflict,
			wantErrSubstr: "not yet pushed",
		},
		{
			name: "pending DH push is forwarded to service for inline push + list",
			getPurchase: func(ctx context.Context, id string) (*inventory.Purchase, error) {
				p := readyPurchase()
				p.DHInventoryID = 0
				p.DHPushStatus = inventory.DHPushStatusPending
				return p, nil
			},
			listFn: func(ctx context.Context, certs []string) dhlisting.DHListingResult {
				return dhlisting.DHListingResult{Listed: 1, Synced: 1, Total: 1}
			},
			wantStatus:     http.StatusOK,
			wantListedFrom: []string{"CERT123"},
		},
		{
			name: "held DH push → 409 with held-specific message",
			getPurchase: func(ctx context.Context, id string) (*inventory.Purchase, error) {
				p := readyPurchase()
				p.DHInventoryID = 0
				p.DHPushStatus = inventory.DHPushStatusHeld
				return p, nil
			},
			wantStatus:    http.StatusConflict,
			wantErrSubstr: "held for review",
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
			name: "no reviewed price → 409",
			getPurchase: func(ctx context.Context, id string) (*inventory.Purchase, error) {
				p := readyPurchase()
				p.ReviewedPriceCents = 0
				return p, nil
			},
			wantStatus:    http.StatusConflict,
			wantErrSubstr: "Review the price",
		},
		{
			name:          "dh listing service not configured → 503",
			omitDHSvc:     true,
			wantStatus:    http.StatusServiceUnavailable,
			wantErrSubstr: "not configured",
		},
		{
			name:        "listing returns zero listed → 502",
			getPurchase: func(ctx context.Context, id string) (*inventory.Purchase, error) { return readyPurchase(), nil },
			listFn: func(ctx context.Context, certs []string) dhlisting.DHListingResult {
				return dhlisting.DHListingResult{Listed: 0, Total: 1}
			},
			wantStatus:    http.StatusBadGateway,
			wantErrSubstr: "check server logs",
		},
		{
			name: "listing returns zero listed with stale inventory ID → 502 reconcile hint",
			getPurchase: func() func(ctx context.Context, id string) (*inventory.Purchase, error) {
				call := 0
				return func(ctx context.Context, id string) (*inventory.Purchase, error) {
					call++
					if call == 1 {
						return readyPurchase(), nil // initial validation passes
					}
					p := readyPurchase()
					p.DHInventoryID = 0 // re-read shows ID was cleared
					return p, nil
				}
			}(),
			listFn: func(ctx context.Context, certs []string) dhlisting.DHListingResult {
				return dhlisting.DHListingResult{Listed: 0, Total: 1}
			},
			wantStatus:    http.StatusBadGateway,
			wantErrSubstr: "will retry automatically",
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
				if !strings.Contains(msg, tt.wantErrSubstr) {
					t.Errorf("error body = %q, want substring %q", msg, tt.wantErrSubstr)
				}
			}
			if tt.wantListedFrom != nil {
				if !slices.Equal(capturedCerts, tt.wantListedFrom) {
					t.Errorf("ListPurchases called with %v, want %v", capturedCerts, tt.wantListedFrom)
				}
			}
		})
	}
}
