package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestDeletionReturnsBoundedReturnConflict(t *testing.T) {
	conflict := inventory.NewReturnConflict("return_in_progress", "Confirmed return is unresolved")
	for _, route := range []string{"campaign", "purchase", "sale"} {
		t.Run(route, func(t *testing.T) {
			svc := &mocks.MockInventoryService{GetPurchaseFn: func(context.Context, string) (*inventory.Purchase, error) {
				return &inventory.Purchase{ID: "p1", CampaignID: "c1"}, nil
			}, DeleteCampaignFn: func(context.Context, string) error { return conflict }, DeletePurchaseFn: func(context.Context, string) error { return conflict }, DeleteSaleByPurchaseIDFn: func(context.Context, string) error { return conflict }}
			h := NewCampaignsHandler(svc, nil, nil, nil, mocks.NewMockLogger(), nil)
			r := withUser(httptest.NewRequest(http.MethodDelete, "/", nil))
			r.SetPathValue("id", "c1")
			r.SetPathValue("purchaseId", "p1")
			w := httptest.NewRecorder()
			switch route {
			case "campaign":
				h.HandleDelete(w, r)
			case "purchase":
				h.HandleDeletePurchase(w, r)
			case "sale":
				h.HandleDeleteSale(w, r)
			}
			if w.Code != 409 || !strings.Contains(w.Body.String(), "return_in_progress") {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
