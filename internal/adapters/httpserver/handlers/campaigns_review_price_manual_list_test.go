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

func TestHandleSetReviewedPrice_ExplicitIntakeListingDoesNotRaceBackgroundDH(t *testing.T) {
	for _, tc := range []struct {
		name, body  string
		wantDHCalls int32
	}{
		{"ordinary review keeps background updates", `{"priceCents":12000,"source":"market"}`, 1},
		{"intake explicit list defers background updates", `{"priceCents":12000,"source":"market","manualList":true}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var priceSyncs, listings atomic.Int32
			svc := &mocks.MockInventoryService{
				SetReviewedPriceFn: func(context.Context, string, int, string) error { return nil },
				GetPurchaseFn: func(context.Context, string) (*inventory.Purchase, error) {
					return &inventory.Purchase{ID: "p1", CertNumber: "123", DHInventoryID: 77}, nil
				},
			}
			price := &mocks.MockDHPriceSyncer{SyncPurchasePriceFn: func(context.Context, string) { priceSyncs.Add(1) }}
			listing := &mocks.MockDHListingService{ListPurchasesFn: func(context.Context, []string) dhlisting.DHListingResult {
				listings.Add(1)
				return dhlisting.DHListingResult{Listed: 1}
			}}
			h := NewCampaignsHandler(svc, nil, nil, nil, mocks.NewMockLogger(), context.Background(), WithDHPriceSyncer(price), WithDHListingService(listing))
			req := httptest.NewRequest(http.MethodPatch, "/api/purchases/p1/review-price", strings.NewReader(tc.body))
			req.SetPathValue("purchaseId", "p1")
			rec := httptest.NewRecorder()
			h.HandleSetReviewedPrice(rec, req)
			h.WaitBackground()
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if got := priceSyncs.Load(); got != tc.wantDHCalls {
				t.Errorf("background price syncs = %d, want %d", got, tc.wantDHCalls)
			}
			if got := listings.Load(); got != tc.wantDHCalls {
				t.Errorf("background listings = %d, want %d", got, tc.wantDHCalls)
			}
		})
	}
}
