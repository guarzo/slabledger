package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

type mockEventHistoryStore struct {
	byPurchase  []dhevents.StoredEvent
	byCert      []dhevents.StoredEvent
	err         error
	lastSubject string
	lastLimit   int
	purchaseHit int
	certHit     int
}

func (m *mockEventHistoryStore) ByPurchase(_ context.Context, purchaseID string, limit int) ([]dhevents.StoredEvent, error) {
	m.purchaseHit++
	m.lastSubject, m.lastLimit = purchaseID, limit
	return m.byPurchase, m.err
}

func (m *mockEventHistoryStore) ByCert(_ context.Context, certNumber string, limit int) ([]dhevents.StoredEvent, error) {
	m.certHit++
	m.lastSubject, m.lastLimit = certNumber, limit
	return m.byCert, m.err
}

func TestHandleGetEvents_NotConfigured(t *testing.T) {
	h := NewDHHandler(DHHandlerDeps{Logger: mocks.NewMockLogger()})

	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/events?cert=123", nil))
	rr := httptest.NewRecorder()
	h.HandleGetEvents(rr, req)

	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

func TestHandleGetEvents_ParamValidation(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		wantCode int
	}{
		{name: "no subject", query: "", wantCode: http.StatusBadRequest},
		{name: "both subjects", query: "?purchaseId=p1&cert=123", wantCode: http.StatusBadRequest},
		{name: "non-numeric limit", query: "?cert=123&limit=abc", wantCode: http.StatusBadRequest},
		{name: "zero limit", query: "?cert=123&limit=0", wantCode: http.StatusBadRequest},
		{name: "negative limit", query: "?cert=123&limit=-1", wantCode: http.StatusBadRequest},
		{name: "purchase only", query: "?purchaseId=p1", wantCode: http.StatusOK},
		{name: "cert only", query: "?cert=123", wantCode: http.StatusOK},
		{name: "cert with limit", query: "?cert=123&limit=5", wantCode: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewDHHandler(DHHandlerDeps{
				Logger:            mocks.NewMockLogger(),
				EventHistoryStore: &mockEventHistoryStore{},
			})

			req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/events"+tt.query, nil))
			rr := httptest.NewRecorder()
			h.HandleGetEvents(rr, req)

			require.Equal(t, tt.wantCode, rr.Code, rr.Body.String())
		})
	}
}

func TestHandleGetEvents_RoutesToCorrectSubject(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		wantPurchase int
		wantCert     int
		wantSubject  string
		wantLimit    int
	}{
		{
			name:         "purchaseId routes to ByPurchase with default limit",
			query:        "?purchaseId=p-42",
			wantPurchase: 1,
			wantSubject:  "p-42",
			wantLimit:    0, // passed through unclamped; the store clamps
		},
		{
			name:        "cert routes to ByCert with explicit limit",
			query:       "?cert=98765&limit=7",
			wantCert:    1,
			wantSubject: "98765",
			wantLimit:   7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockEventHistoryStore{}
			h := NewDHHandler(DHHandlerDeps{
				Logger:            mocks.NewMockLogger(),
				EventHistoryStore: store,
			})

			req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/events"+tt.query, nil))
			rr := httptest.NewRecorder()
			h.HandleGetEvents(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, tt.wantPurchase, store.purchaseHit)
			assert.Equal(t, tt.wantCert, store.certHit)
			assert.Equal(t, tt.wantSubject, store.lastSubject)
			assert.Equal(t, tt.wantLimit, store.lastLimit)
		})
	}
}

func TestHandleGetEvents_Response(t *testing.T) {
	at := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	store := &mockEventHistoryStore{
		byCert: []dhevents.StoredEvent{{
			ID:      9,
			EventAt: at,
			Event: dhevents.Event{
				PurchaseID:     "p-1",
				CertNumber:     "98765",
				Type:           dhevents.TypeSold,
				Source:         dhevents.SourceDHOrdersPoll,
				NewPushStatus:  "sold",
				DHInventoryID:  1234,
				SalePriceCents: 4500,
			},
		}},
	}
	h := NewDHHandler(DHHandlerDeps{
		Logger:            mocks.NewMockLogger(),
		EventHistoryStore: store,
	})

	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/events?cert=98765", nil))
	rr := httptest.NewRecorder()
	h.HandleGetEvents(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var body struct {
		Events []dhEventResponse `json:"events"`
		Count  int               `json:"count"`
		Limit  int               `json:"limit"`
	}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))

	require.Len(t, body.Events, 1)
	assert.Equal(t, 1, body.Count)
	assert.Equal(t, dhevents.DefaultHistoryLimit, body.Limit)

	got := body.Events[0]
	assert.Equal(t, int64(9), got.ID)
	assert.Equal(t, "2026-04-15T12:00:00Z", got.EventAt)
	assert.Equal(t, "p-1", got.PurchaseID)
	assert.Equal(t, "98765", got.CertNumber)
	assert.Equal(t, string(dhevents.TypeSold), got.Type)
	assert.Equal(t, string(dhevents.SourceDHOrdersPoll), got.Source)
	assert.Equal(t, "sold", got.NewPushStatus)
	assert.Equal(t, 1234, got.DHInventoryID)
	assert.Equal(t, 4500, got.SalePriceCents)
}

func TestHandleGetEvents_EmptyResultIsAnArray(t *testing.T) {
	h := NewDHHandler(DHHandlerDeps{
		Logger:            mocks.NewMockLogger(),
		EventHistoryStore: &mockEventHistoryStore{},
	})

	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/events?cert=nope", nil))
	rr := httptest.NewRecorder()
	h.HandleGetEvents(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	// A JSON null here would break any client doing `.map` on the field.
	assert.Contains(t, rr.Body.String(), `"events":[]`)
}

func TestHandleGetEvents_StoreError(t *testing.T) {
	h := NewDHHandler(DHHandlerDeps{
		Logger:            mocks.NewMockLogger(),
		EventHistoryStore: &mockEventHistoryStore{err: errors.New("boom")},
	})

	req := authenticatedRequest(httptest.NewRequest(http.MethodGet, "/api/dh/events?purchaseId=p-1", nil))
	rr := httptest.NewRecorder()
	h.HandleGetEvents(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
}
