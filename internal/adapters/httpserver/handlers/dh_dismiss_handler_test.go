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
	rec := &mocks.MockEventRecorder{}

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
	require.Len(t, rec.Events, 1)
	evt := rec.Events[0]
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

func TestHandleDismissMatch_StateMatrix(t *testing.T) {
	cases := []struct {
		name      string
		status    inventory.DHPushStatus
		wantCode  int
		wantEvent bool
	}{
		{"pending allowed", inventory.DHPushStatusPending, http.StatusOK, true},
		{"unmatched allowed", inventory.DHPushStatusUnmatched, http.StatusOK, true},
		{"matched allowed", inventory.DHPushStatusMatched, http.StatusOK, true},
		{"manual allowed", inventory.DHPushStatusManual, http.StatusOK, true},
		{"held allowed", inventory.DHPushStatusHeld, http.StatusOK, true},
		{"already dismissed rejected", inventory.DHPushStatusDismissed, http.StatusConflict, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			purchase := &inventory.Purchase{
				ID:           "pur-1",
				CertNumber:   "c-1",
				DHPushStatus: tc.status,
			}
			repo := &mocks.PurchaseRepositoryMock{
				GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
					return purchase, nil
				},
			}
			rec := &mocks.MockEventRecorder{}
			h := NewDHHandler(DHHandlerDeps{
				PurchaseLister:    repo,
				PushStatusUpdater: repo,
				Logger:            mocks.NewMockLogger(),
				BaseCtx:           context.Background(),
				EventRecorder:     rec,
			})

			body, _ := json.Marshal(dhDismissRequest{PurchaseID: "pur-1"})
			req := authenticatedRequest(httptest.NewRequest(http.MethodPost, "/api/dh/dismiss", bytes.NewReader(body)))
			rr := httptest.NewRecorder()
			h.HandleDismissMatch(rr, req)

			require.Equal(t, tc.wantCode, rr.Code)
			if tc.wantEvent {
				require.Len(t, rec.Events, 1)
				assert.Equal(t, tc.status, rec.Events[0].PrevPushStatus)
				assert.Equal(t, inventory.DHPushStatusDismissed, rec.Events[0].NewPushStatus)
			} else {
				assert.Len(t, rec.Events, 0)
			}
		})
	}
}
