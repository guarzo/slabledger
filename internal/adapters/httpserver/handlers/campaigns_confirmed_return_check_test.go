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

func TestDHSaleCheckHandlerUnavailableCapability(t *testing.T) {
	svc := &mocks.ConfirmedReturnServiceMock{CheckDHSaleFn: func(context.Context, string) (*inventory.DHSaleCheck, error) {
		return nil, inventory.NewReturnConflict("coordination_unavailable", "DH capability unavailable")
	}}
	h := NewCampaignsHandler(nil, nil, nil, nil, mocks.NewMockLogger(), nil, WithConfirmedReturnService(svc))
	r := withUser(httptest.NewRequest(http.MethodGet, "/", nil))
	r.SetPathValue("purchaseId", "p1")
	w := httptest.NewRecorder()
	h.HandleDHSaleCheck(w, r)
	require.Equal(t, 503, w.Code)
	require.Contains(t, w.Body.String(), "coordination_unavailable")
}

func TestDHSaleCheckHandlerRequiresUser(t *testing.T) {
	calls := 0
	svc := &mocks.ConfirmedReturnServiceMock{CheckDHSaleFn: func(context.Context, string) (*inventory.DHSaleCheck, error) {
		calls++
		return nil, nil
	}}
	h := NewCampaignsHandler(nil, nil, nil, nil, mocks.NewMockLogger(), nil, WithConfirmedReturnService(svc))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.SetPathValue("purchaseId", "p1")
	w := httptest.NewRecorder()
	h.HandleDHSaleCheck(w, r)
	require.Equal(t, 401, w.Code)
	require.Zero(t, calls)
}
