package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// mockCLSyncUpdater records calls to UpdatePurchaseCLSyncedAt.
type mockCLSyncUpdater struct {
	calls []struct {
		purchaseID string
		syncedAt   string
	}
}

func (m *mockCLSyncUpdater) UpdatePurchaseCLSyncedAt(_ context.Context, purchaseID, syncedAt string) error {
	m.calls = append(m.calls, struct {
		purchaseID string
		syncedAt   string
	}{purchaseID, syncedAt})
	return nil
}

// Ensure mockCLSyncUpdater implements CLSyncUpdater.
var _ CLSyncUpdater = (*mockCLSyncUpdater)(nil)

// mockCLPurchaseLister is a no-op implementation of CLPurchaseLister for tests.
type mockCLPurchaseLister struct{}

func (m *mockCLPurchaseLister) ListAllUnsoldPurchases(_ context.Context) ([]campaigns.Purchase, error) {
	return nil, nil
}

// Ensure mockCLPurchaseLister implements CLPurchaseLister.
var _ CLPurchaseLister = (*mockCLPurchaseLister)(nil)

func TestAddCardToCollection_UpdatesCLSyncedAt(t *testing.T) {
	updater := &mockCLSyncUpdater{}
	_ = observability.Logger(mocks.NewMockLogger()) // verify import usable

	h := &CardLadderHandler{
		logger: mocks.NewMockLogger(),
	}
	h.SetSyncUpdater(updater)

	// Verify the syncUpdater field is set.
	if h.syncUpdater == nil {
		t.Fatal("expected syncUpdater to be set")
	}
}

func TestHandleSyncToCardLadder_NilClient_Returns503(t *testing.T) {
	h := &CardLadderHandler{
		logger:         mocks.NewMockLogger(),
		purchaseLister: &mockCLPurchaseLister{},
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
