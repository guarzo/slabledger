package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestDeleteFailedListingConsent(t *testing.T) {
	for _, tt := range []struct {
		name, query, campaign string
		status                int
		called, confirmed     bool
	}{
		{"ordinary", "", "c", 204, true, false},
		{"confirmed", "?confirmFailedListing=true", "c", 204, true, true},
		{"false is not consent", "?confirmFailedListing=false", "c", 204, true, false},
		{"wrong campaign", "?confirmFailedListing=true", "other", 403, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			svc := &mocks.MockInventoryService{GetPurchaseFn: func(context.Context, string) (*inventory.Purchase, error) {
				return &inventory.Purchase{ID: "p", CampaignID: tt.campaign}, nil
			}, DeletePurchaseFn: func(ctx context.Context, id string) error {
				called = true
				require.Equal(t, "p", id)
				require.Equal(t, tt.confirmed, inventory.FailedListingDeleteConfirmed(ctx))
				return nil
			}}
			req := httptest.NewRequest(http.MethodDelete, "/api/campaigns/c/purchases/p"+tt.query, nil)
			req.SetPathValue("id", "c")
			req.SetPathValue("purchaseId", "p")
			w := httptest.NewRecorder()
			newTestHandler(svc).HandleDeletePurchase(w, req)
			require.Equal(t, tt.status, w.Code)
			require.Equal(t, tt.called, called)
		})
	}
}

func TestDeleteFailedListingConflictCode(t *testing.T) {
	svc := &mocks.MockInventoryService{GetPurchaseFn: func(context.Context, string) (*inventory.Purchase, error) {
		return &inventory.Purchase{ID: "p", CampaignID: "c"}, nil
	}, DeletePurchaseFn: func(context.Context, string) error {
		return inventory.NewReturnConflict("failed_listing_delete_confirmation_required", "Confirm Delete with warning")
	}}
	req := httptest.NewRequest(http.MethodDelete, "/api/campaigns/c/purchases/p", nil)
	req.SetPathValue("id", "c")
	req.SetPathValue("purchaseId", "p")
	w := httptest.NewRecorder()
	newTestHandler(svc).HandleDeletePurchase(w, req)
	require.Equal(t, 409, w.Code)
	require.Contains(t, w.Body.String(), `"code":"failed_listing_delete_confirmation_required"`)
}
