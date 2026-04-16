package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDHOrdersIngester struct {
	runOnceFn func(ctx context.Context, since string) (*scheduler.DHOrdersPollSummary, error)
}

func (m *mockDHOrdersIngester) RunOnce(ctx context.Context, since string) (*scheduler.DHOrdersPollSummary, error) {
	if m.runOnceFn != nil {
		return m.runOnceFn(ctx, since)
	}
	return &scheduler.DHOrdersPollSummary{Since: since}, nil
}

func newTestDHHandlerWithIngester(ingester DHOrdersIngester) *DHHandler {
	return NewDHHandler(DHHandlerDeps{
		Logger:         mocks.NewMockLogger(),
		BaseCtx:        context.Background(),
		OrdersIngester: ingester,
	})
}

func TestHandleIngestOrders_ValidSince(t *testing.T) {
	var capturedSince string
	ingester := &mockDHOrdersIngester{
		runOnceFn: func(_ context.Context, since string) (*scheduler.DHOrdersPollSummary, error) {
			capturedSince = since
			return &scheduler.DHOrdersPollSummary{
				Since: since, OrdersFetched: 5, Matched: 3, NotFound: 1, AlreadySold: 1,
			}, nil
		},
	}
	h := newTestDHHandlerWithIngester(ingester)

	req := httptest.NewRequest("POST", "/api/dh/ingest-orders?since=2026-01-01T00:00:00Z", nil)
	rr := httptest.NewRecorder()
	h.HandleIngestOrders(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "2026-01-01T00:00:00Z", capturedSince)

	var summary scheduler.DHOrdersPollSummary
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&summary))
	assert.Equal(t, 3, summary.Matched)
	assert.Equal(t, 1, summary.NotFound)
	assert.Equal(t, 1, summary.AlreadySold)
}

func TestHandleIngestOrders_MissingSince(t *testing.T) {
	h := newTestDHHandlerWithIngester(&mockDHOrdersIngester{})

	req := httptest.NewRequest("POST", "/api/dh/ingest-orders", nil)
	rr := httptest.NewRecorder()
	h.HandleIngestOrders(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleIngestOrders_BadSinceFormat(t *testing.T) {
	h := newTestDHHandlerWithIngester(&mockDHOrdersIngester{})

	req := httptest.NewRequest("POST", "/api/dh/ingest-orders?since=not-a-date", nil)
	rr := httptest.NewRecorder()
	h.HandleIngestOrders(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleIngestOrders_Unwired(t *testing.T) {
	h := NewDHHandler(DHHandlerDeps{
		Logger:  mocks.NewMockLogger(),
		BaseCtx: context.Background(),
	}) // no ingester

	req := httptest.NewRequest("POST", "/api/dh/ingest-orders?since=2026-01-01T00:00:00Z", nil)
	rr := httptest.NewRecorder()
	h.HandleIngestOrders(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}
