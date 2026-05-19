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

// TestHandleUndismissMatch_ReceivedAtRoutesCorrectly verifies the B3 fix:
// - received_at set → pending + TypeEnrolled event
// - received_at nil → empty status + TypeUnmatched event
// - always emits accurate NewPushStatus in the event
func TestHandleUndismissMatch_ReceivedAtRoutesCorrectly(t *testing.T) {
	now := "2026-05-19T12:00:00Z"
	cases := []struct {
		name           string
		receivedAt     *string
		wantStatus     inventory.DHPushStatus
		wantEventType  dhevents.Type
	}{
		{
			name:          "received → pending + enrolled event",
			receivedAt:    &now,
			wantStatus:    inventory.DHPushStatusPending,
			wantEventType: dhevents.TypeEnrolled,
		},
		{
			name:          "not received → empty status + unmatched event",
			receivedAt:    nil,
			wantStatus:    "",
			wantEventType: dhevents.TypeUnmatched,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			purchase := &inventory.Purchase{
				ID:           "pur-u1",
				CertNumber:   "99887766",
				DHPushStatus: inventory.DHPushStatusDismissed,
				ReceivedAt:   tc.receivedAt,
			}
			var capturedStatus inventory.DHPushStatus
			repo := &mocks.PurchaseRepositoryMock{
				GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
					return purchase, nil
				},
				UpdatePurchaseDHPushStatusFn: func(_ context.Context, _ string, status string) error {
					capturedStatus = status
					return nil
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

			body, _ := json.Marshal(dhDismissRequest{PurchaseID: "pur-u1"})
			req := httptest.NewRequest(http.MethodPost, "/api/dh/undismiss", bytes.NewReader(body))
			req = authenticatedRequest(req)
			rr := httptest.NewRecorder()
			h.HandleUndismissMatch(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, tc.wantStatus, capturedStatus)

			require.Len(t, rec.Events, 1)
			evt := rec.Events[0]
			assert.Equal(t, tc.wantEventType, evt.Type)
			assert.Equal(t, dhevents.SourceManualUI, evt.Source)
			assert.Equal(t, inventory.DHPushStatusDismissed, evt.PrevPushStatus)
			assert.Equal(t, tc.wantStatus, evt.NewPushStatus)
			assert.Equal(t, "pur-u1", evt.PurchaseID)
		})
	}
}

// TestHandleUndismissMatch_PreviousAlwaysUnmatchedGone verifies that a
// received purchase no longer returns "unmatched" — the old (wrong) behavior.
func TestHandleUndismissMatch_PreviousAlwaysUnmatchedGone(t *testing.T) {
	now := "2026-05-19T12:00:00Z"
	purchase := &inventory.Purchase{
		ID:           "pur-old",
		CertNumber:   "55555555",
		DHPushStatus: inventory.DHPushStatusDismissed,
		ReceivedAt:   &now,
	}
	var capturedStatus inventory.DHPushStatus
	repo := &mocks.PurchaseRepositoryMock{
		GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
			return purchase, nil
		},
		UpdatePurchaseDHPushStatusFn: func(_ context.Context, _ string, status string) error {
			capturedStatus = status
			return nil
		},
	}
	h := NewDHHandler(DHHandlerDeps{
		PurchaseLister:    repo,
		PushStatusUpdater: repo,
		Logger:            mocks.NewMockLogger(),
		BaseCtx:           context.Background(),
	})

	body, _ := json.Marshal(dhDismissRequest{PurchaseID: "pur-old"})
	req := authenticatedRequest(httptest.NewRequest(http.MethodPost, "/api/dh/undismiss", bytes.NewReader(body)))
	rr := httptest.NewRecorder()
	h.HandleUndismissMatch(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	if capturedStatus == inventory.DHPushStatusUnmatched {
		t.Fatal("received purchase must not be routed to unmatched anymore — regression of B3 fix")
	}
}
