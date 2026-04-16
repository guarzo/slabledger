package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- Local test doubles for select-match dependencies ----

type mockDHInventoryPusher struct {
	PushFn func(ctx context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error)
}

func (m *mockDHInventoryPusher) PushInventory(ctx context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error) {
	if m.PushFn != nil {
		return m.PushFn(ctx, items)
	}
	return &dh.InventoryPushResponse{
		Results: []dh.InventoryResult{
			{DHInventoryID: 777, Status: "in_stock", AssignedPriceCents: 5000},
		},
	}, nil
}

type mockDHCardIDSaver struct{}

func (m *mockDHCardIDSaver) SaveExternalID(_ context.Context, _, _, _, _, _ string) error {
	return nil
}

func (m *mockDHCardIDSaver) GetExternalID(_ context.Context, _, _, _, _ string) (string, error) {
	return "", nil
}

func (m *mockDHCardIDSaver) GetMappedSet(_ context.Context, _ string) (map[string]string, error) {
	return map[string]string{}, nil
}

func TestHandleSelectMatch_RecordsPushedEvent(t *testing.T) {
	purchaseID := "pur-select-1"
	purchase := &inventory.Purchase{
		ID:           purchaseID,
		CertNumber:   "87654321",
		CardName:     "Pikachu",
		SetName:      "Base Set",
		CardNumber:   "58",
		GradeValue:   10,
		BuyCostCents: 5000,
		CLValueCents: 9000,
		DHPushStatus: inventory.DHPushStatusUnmatched,
	}

	repo := &mocks.PurchaseRepositoryMock{
		GetPurchaseFn: func(_ context.Context, id string) (*inventory.Purchase, error) {
			assert.Equal(t, purchaseID, id)
			return purchase, nil
		},
	}

	pusher := &mockDHInventoryPusher{
		PushFn: func(_ context.Context, items []dh.InventoryItem) (*dh.InventoryPushResponse, error) {
			require.Len(t, items, 1)
			assert.Equal(t, 4242, items[0].DHCardID)
			return &dh.InventoryPushResponse{
				Results: []dh.InventoryResult{
					{DHInventoryID: 9999, Status: "in_stock", AssignedPriceCents: 9000},
				},
			}, nil
		},
	}

	rec := &mockEventRecorder{}

	h := NewDHHandler(DHHandlerDeps{
		PurchaseLister:    repo,
		PushStatusUpdater: repo,
		DHFieldsUpdater:   repo,
		CandidatesSaver:   repo,
		InventoryPusher:   pusher,
		CardIDSaver:       &mockDHCardIDSaver{},
		Logger:            mocks.NewMockLogger(),
		BaseCtx:           context.Background(),
		EventRecorder:     rec,
	})

	body, _ := json.Marshal(selectMatchRequest{PurchaseID: purchaseID, DHCardID: 4242})
	req := httptest.NewRequest(http.MethodPost, "/api/dh/select-match", bytes.NewReader(body))
	req = authenticatedRequest(req)
	rr := httptest.NewRecorder()
	h.HandleSelectMatch(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
	require.Len(t, rec.events, 1)
	evt := rec.events[0]
	assert.Equal(t, dhevents.TypePushed, evt.Type)
	assert.Equal(t, purchaseID, evt.PurchaseID)
	assert.Equal(t, "87654321", evt.CertNumber)
	assert.Equal(t, inventory.DHPushStatusManual, evt.NewPushStatus)
	assert.Equal(t, 4242, evt.DHCardID)
	assert.Equal(t, 9999, evt.DHInventoryID)
	assert.Equal(t, dhevents.SourceManualUI, evt.Source)
}
