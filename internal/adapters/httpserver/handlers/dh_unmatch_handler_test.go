package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func postUnmatchDH(h *DHHandler, purchaseID string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"purchaseId": purchaseID})
	req := httptest.NewRequest(http.MethodPost, "/api/dh/unmatch", bytes.NewReader(body))
	req = authenticatedRequest(req)
	rr := httptest.NewRecorder()
	h.HandleUnmatchDH(rr, req)
	return rr
}

func TestHandleUnmatchDH_MissingAuth(t *testing.T) {
	h := NewDHHandler(DHHandlerDeps{Logger: mocks.NewMockLogger(), BaseCtx: context.Background()})
	body, _ := json.Marshal(map[string]string{"purchaseId": "p1"})
	req := httptest.NewRequest(http.MethodPost, "/api/dh/unmatch", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.HandleUnmatchDH(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleUnmatchDH_MissingPurchaseID(t *testing.T) {
	h := NewDHHandler(DHHandlerDeps{Logger: mocks.NewMockLogger(), BaseCtx: context.Background()})
	body, _ := json.Marshal(map[string]string{"purchaseId": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/dh/unmatch", bytes.NewReader(body))
	req = authenticatedRequest(req)
	rr := httptest.NewRecorder()
	h.HandleUnmatchDH(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "purchaseId is required")
}

func TestHandleUnmatchDH_PurchaseNotFound(t *testing.T) {
	repo := &mocks.PurchaseRepositoryMock{
		GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
			return nil, inventory.ErrPurchaseNotFound
		},
	}
	h := NewDHHandler(DHHandlerDeps{
		PurchaseLister: repo,
		Logger:         mocks.NewMockLogger(),
		BaseCtx:        context.Background(),
	})
	rr := postUnmatchDH(h, "missing-id")
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandleUnmatchDH_Success(t *testing.T) {
	purchase := &inventory.Purchase{
		ID:            "p1",
		DHPushStatus:  inventory.DHPushStatusMatched,
		DHCardID:      555,
		DHInventoryID: 666,
	}
	var updatedFields inventory.DHFieldsUpdate
	var updatedStatus string
	var clearedCandidates string
	repo := &mocks.PurchaseRepositoryMock{
		GetPurchaseFn: func(_ context.Context, _ string) (*inventory.Purchase, error) {
			return purchase, nil
		},
		UpdatePurchaseDHFieldsFn: func(_ context.Context, _ string, u inventory.DHFieldsUpdate) error {
			updatedFields = u
			return nil
		},
		UpdatePurchaseDHPushStatusFn: func(_ context.Context, _ string, status string) error {
			updatedStatus = status
			return nil
		},
		UpdatePurchaseDHCandidatesFn: func(_ context.Context, _ string, c string) error {
			clearedCandidates = c
			return nil
		},
	}
	h := NewDHHandler(DHHandlerDeps{
		PurchaseLister:    repo,
		DHFieldsUpdater:   repo,
		PushStatusUpdater: repo,
		CandidatesSaver:   repo,
		Logger:            mocks.NewMockLogger(),
		BaseCtx:           context.Background(),
	})
	rr := postUnmatchDH(h, "p1")
	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
	assert.Equal(t, 0, updatedFields.CardID)
	assert.Equal(t, 0, updatedFields.InventoryID)
	assert.Equal(t, inventory.DHPushStatusUnmatched, updatedStatus)
	assert.Equal(t, "", clearedCandidates)
}
