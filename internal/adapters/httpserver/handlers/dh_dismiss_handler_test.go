package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEventRecorder is a test-local recorder for asserting DH state-event emission.
type mockEventRecorder struct {
	events []dhevents.Event
}

func (r *mockEventRecorder) Record(_ context.Context, e dhevents.Event) error {
	r.events = append(r.events, e)
	return nil
}

func TestHandleDismissMatch_RecordsDismissedEvent(t *testing.T) {
	purchaseID := "pur-dismiss-1"
	purchase := &inventory.Purchase{
		ID:           purchaseID,
		CertNumber:   "12345678",
		DHPushStatus: inventory.DHPushStatusUnmatched,
	}
	repo := &mocks.PurchaseRepositoryMock{
		GetPurchaseFn: func(_ context.Context, id string) (*inventory.Purchase, error) {
			assert.Equal(t, purchaseID, id)
			return purchase, nil
		},
	}
	rec := &mockEventRecorder{}

	h := NewDHHandler(DHHandlerDeps{
		PurchaseLister:    repo,
		PushStatusUpdater: repo,
		Logger:            mocks.NewMockLogger(),
		BaseCtx:           context.Background(),
		EventRecorder:     rec,
	})

	body, _ := json.Marshal(dhDismissRequest{PurchaseID: purchaseID})
	req := httptest.NewRequest(http.MethodPost, "/api/dh/dismiss", bytes.NewReader(body))
	req = authenticatedRequest(req)
	rr := httptest.NewRecorder()
	h.HandleDismissMatch(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Len(t, rec.events, 1)
	evt := rec.events[0]
	assert.Equal(t, dhevents.TypeDismissed, evt.Type)
	assert.Equal(t, purchaseID, evt.PurchaseID)
	assert.Equal(t, "12345678", evt.CertNumber)
	assert.Equal(t, inventory.DHPushStatusDismissed, evt.NewPushStatus)
	assert.Equal(t, dhevents.SourceManualUI, evt.Source)
}

func TestHandleDismissMatch_NilRecorderIsSafe(t *testing.T) {
	purchase := &inventory.Purchase{
		ID:           "pur-x",
		CertNumber:   "9",
		DHPushStatus: inventory.DHPushStatusUnmatched,
	}
	repo := &mocks.PurchaseRepositoryMock{
		GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
			return purchase, nil
		},
	}

	h := NewDHHandler(DHHandlerDeps{
		PurchaseLister:    repo,
		PushStatusUpdater: repo,
		Logger:            mocks.NewMockLogger(),
		BaseCtx:           context.Background(),
		// no EventRecorder
	})

	body, _ := json.Marshal(dhDismissRequest{PurchaseID: "pur-x"})
	req := httptest.NewRequest(http.MethodPost, "/api/dh/dismiss", bytes.NewReader(body))
	req = authenticatedRequest(req)
	rr := httptest.NewRecorder()
	h.HandleDismissMatch(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}
