package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// newSellSheetHandler creates a SellSheetItemsHandler with the given mock repo.
func newSellSheetHandler(repo *mocks.MockSellSheetRepository) *SellSheetItemsHandler {
	return NewSellSheetItemsHandler(repo, mocks.NewMockLogger())
}

// --- HandleGetItems ---

func TestHandleGetItems_Success(t *testing.T) {
	repo := &mocks.MockSellSheetRepository{
		GetSellSheetItemsFn: func(_ context.Context) ([]string, error) {
			return []string{"p1", "p2", "p3"}, nil
		},
	}
	h := newSellSheetHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/sell-sheet/items", nil)
	rec := httptest.NewRecorder()
	h.HandleGetItems(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp map[string][]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, []string{"p1", "p2", "p3"}, resp["purchaseIds"])
}

func TestHandleGetItems_EmptyList(t *testing.T) {
	repo := &mocks.MockSellSheetRepository{
		GetSellSheetItemsFn: func(_ context.Context) ([]string, error) {
			return []string{}, nil
		},
	}
	h := newSellSheetHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/sell-sheet/items", nil)
	rec := httptest.NewRecorder()
	h.HandleGetItems(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string][]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.NotNil(t, resp["purchaseIds"], "expected non-null array")
	assert.Empty(t, resp["purchaseIds"])
}

func TestHandleGetItems_NilFromRepo(t *testing.T) {
	// nil returned from repo should be coerced to empty slice, not null JSON.
	repo := &mocks.MockSellSheetRepository{
		GetSellSheetItemsFn: func(_ context.Context) ([]string, error) {
			return nil, nil
		},
	}
	h := newSellSheetHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/sell-sheet/items", nil)
	rec := httptest.NewRecorder()
	h.HandleGetItems(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	// Ensure purchaseIds is present and is an array (not null)
	assert.Equal(t, json.RawMessage("[]"), resp["purchaseIds"])
}

func TestHandleGetItems_RepoError(t *testing.T) {
	repo := &mocks.MockSellSheetRepository{
		GetSellSheetItemsFn: func(_ context.Context) ([]string, error) {
			return nil, fmt.Errorf("database unavailable")
		},
	}
	h := newSellSheetHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/sell-sheet/items", nil)
	rec := httptest.NewRecorder()
	h.HandleGetItems(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	decodeErrorResponse(t, rec)
}

// --- HandleAddItems ---

func TestHandleAddItems_Success(t *testing.T) {
	var capturedIDs []string
	repo := &mocks.MockSellSheetRepository{
		AddSellSheetItemsFn: func(_ context.Context, ids []string) error {
			capturedIDs = ids
			return nil
		},
	}
	h := newSellSheetHandler(repo)

	body := `{"purchaseIds":["p1","p2"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/sell-sheet/items", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleAddItems(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, []string{"p1", "p2"}, capturedIDs)
}

func TestHandleAddItems_EmptyPurchaseIDs(t *testing.T) {
	h := newSellSheetHandler(mocks.NewMockSellSheetRepository())

	body := `{"purchaseIds":[]}`
	req := httptest.NewRequest(http.MethodPut, "/api/sell-sheet/items", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleAddItems(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	decodeErrorResponse(t, rec)
}

func TestHandleAddItems_MissingPurchaseIDs(t *testing.T) {
	// omitting purchaseIds entirely is treated as empty
	h := newSellSheetHandler(mocks.NewMockSellSheetRepository())

	body := `{}`
	req := httptest.NewRequest(http.MethodPut, "/api/sell-sheet/items", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleAddItems(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	decodeErrorResponse(t, rec)
}

func TestHandleAddItems_InvalidBody(t *testing.T) {
	h := newSellSheetHandler(mocks.NewMockSellSheetRepository())

	req := httptest.NewRequest(http.MethodPut, "/api/sell-sheet/items", bytes.NewBufferString("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleAddItems(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleAddItems_RepoError(t *testing.T) {
	repo := &mocks.MockSellSheetRepository{
		AddSellSheetItemsFn: func(_ context.Context, _ []string) error {
			return fmt.Errorf("storage failure")
		},
	}
	h := newSellSheetHandler(repo)

	body := `{"purchaseIds":["p1"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/sell-sheet/items", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleAddItems(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	decodeErrorResponse(t, rec)
}

// --- HandleRemoveItems ---

func TestHandleRemoveItems_Success(t *testing.T) {
	var capturedIDs []string
	repo := &mocks.MockSellSheetRepository{
		RemoveSellSheetItemsFn: func(_ context.Context, ids []string) error {
			capturedIDs = ids
			return nil
		},
	}
	h := newSellSheetHandler(repo)

	body := `{"purchaseIds":["p1","p2"]}`
	req := httptest.NewRequest(http.MethodDelete, "/api/sell-sheet/items", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleRemoveItems(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, []string{"p1", "p2"}, capturedIDs)
}

func TestHandleRemoveItems_EmptyPurchaseIDs(t *testing.T) {
	h := newSellSheetHandler(mocks.NewMockSellSheetRepository())

	body := `{"purchaseIds":[]}`
	req := httptest.NewRequest(http.MethodDelete, "/api/sell-sheet/items", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleRemoveItems(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	decodeErrorResponse(t, rec)
}

func TestHandleRemoveItems_InvalidBody(t *testing.T) {
	h := newSellSheetHandler(mocks.NewMockSellSheetRepository())

	req := httptest.NewRequest(http.MethodDelete, "/api/sell-sheet/items", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleRemoveItems(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleRemoveItems_RepoError(t *testing.T) {
	repo := &mocks.MockSellSheetRepository{
		RemoveSellSheetItemsFn: func(_ context.Context, _ []string) error {
			return fmt.Errorf("remove failed")
		},
	}
	h := newSellSheetHandler(repo)

	body := `{"purchaseIds":["p99"]}`
	req := httptest.NewRequest(http.MethodDelete, "/api/sell-sheet/items", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleRemoveItems(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	decodeErrorResponse(t, rec)
}

// --- HandleClearItems ---

func TestHandleClearItems_Success(t *testing.T) {
	cleared := false
	repo := &mocks.MockSellSheetRepository{
		ClearSellSheetFn: func(_ context.Context) error {
			cleared = true
			return nil
		},
	}
	h := newSellSheetHandler(repo)

	req := httptest.NewRequest(http.MethodDelete, "/api/sell-sheet/items/all", nil)
	rec := httptest.NewRecorder()
	h.HandleClearItems(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.True(t, cleared)
}

func TestHandleClearItems_RepoError(t *testing.T) {
	repo := &mocks.MockSellSheetRepository{
		ClearSellSheetFn: func(_ context.Context) error {
			return fmt.Errorf("clear failed")
		},
	}
	h := newSellSheetHandler(repo)

	req := httptest.NewRequest(http.MethodDelete, "/api/sell-sheet/items/all", nil)
	rec := httptest.NewRecorder()
	h.HandleClearItems(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	decodeErrorResponse(t, rec)
}
