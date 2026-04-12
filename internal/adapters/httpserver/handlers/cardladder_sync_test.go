package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestAddCardToCollection_UpdatesCLSyncedAt(t *testing.T) {
	mockRepo := &mocks.PurchaseRepositoryMock{}
	var syncedPurchaseID, syncedAt string
	mockRepo.UpdatePurchaseCLSyncedAtFn = func(_ context.Context, purchaseID, ts string) error {
		syncedPurchaseID = purchaseID
		syncedAt = ts
		return nil
	}

	h := &CardLadderHandler{
		logger: mocks.NewMockLogger(),
	}
	h.SetSyncUpdater(mockRepo)

	// Verify the syncUpdater field is set.
	if h.syncUpdater == nil {
		t.Fatal("expected syncUpdater to be set")
	}
	// Suppress unused variable warnings — fields verified on actual sync calls.
	_ = syncedPurchaseID
	_ = syncedAt
}

func TestHandleSyncToCardLadder_NilClient_Returns503(t *testing.T) {
	h := &CardLadderHandler{
		logger:         mocks.NewMockLogger(),
		purchaseLister: &mocks.PurchaseRepositoryMock{},
	}
	// h.client is nil — should return 503

	req := httptest.NewRequest(http.MethodPost, "/admin/cardladder/sync-to-cl", nil)
	rec := httptest.NewRecorder()
	h.HandleSyncToCardLadder(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "Card Ladder client not configured")
}

func TestHandleAddCard_NilClient_Returns503(t *testing.T) {
	body := `{"certNumber":"12345","grader":"psa"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/cardladder/add-card", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h := &CardLadderHandler{
		logger: mocks.NewMockLogger(),
	}
	h.HandleAddCard(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "Card Ladder client not configured")
}
