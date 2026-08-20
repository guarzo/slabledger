package dhlisting_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	adapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// fakeDHSaleServer enforces the DH contract rather than accepting whatever we
// send: it requires the Idempotency-Key header, replays on same-key+same-body,
// and 422s on same-key+different-body — including the identical body against a
// different inventory id, the trap the design doc calls out explicitly.
type fakeDHSaleServer struct {
	*httptest.Server

	mu     sync.Mutex
	sales  map[string]recordedSale // keyed by Idempotency-Key
	voided map[string]bool         // keyed by dh_sale_id
	nextID int
}

type recordedSale struct {
	inventoryID int
	bodyHash    string
	saleID      string
}

func newFakeDHSaleServer(t *testing.T) *fakeDHSaleServer {
	t.Helper()
	f := &fakeDHSaleServer{sales: map[string]recordedSale{}, voided: map[string]bool{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeDHSaleServer) handle(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/sale"):
		f.handleSale(w, r)
	case strings.HasSuffix(r.URL.Path, "/void"):
		f.handleVoid(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeDHSaleServer) handleSale(w http.ResponseWriter, r *http.Request) {
	key := r.Header.Get("Idempotency-Key")
	if key == "" || len(key) > 255 {
		writeJSONError(w, http.StatusUnprocessableEntity, "validation", "Idempotency-Key must be 1-255 characters")
		return
	}

	segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	inventoryID, _ := strconv.Atoi(segments[len(segments)-2])

	var req dh.InventorySaleRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	bodyBytes, _ := json.Marshal(req)
	hash := fmt.Sprintf("%x", sha256.Sum256(bodyBytes))

	f.mu.Lock()
	defer f.mu.Unlock()

	if existing, ok := f.sales[key]; ok {
		if existing.bodyHash == hash && existing.inventoryID == inventoryID {
			writeJSON(w, http.StatusOK, dh.InventorySaleResponse{
				SaleID: existing.saleID, DHInventoryID: inventoryID,
				ItemStatus: "sold", Delisted: true, Replayed: true,
			})
			return
		}
		writeJSONError(w, http.StatusUnprocessableEntity, "idempotency_key_reused", "idempotency key reused with a different request")
		return
	}

	f.nextID++
	saleID := fmt.Sprintf("sale_%d", f.nextID)
	f.sales[key] = recordedSale{inventoryID: inventoryID, bodyHash: hash, saleID: saleID}

	writeJSON(w, http.StatusOK, dh.InventorySaleResponse{
		SaleID: saleID, DHInventoryID: inventoryID,
		ItemStatus: "sold", Delisted: true, Replayed: false,
	})
}

func (f *fakeDHSaleServer) handleVoid(w http.ResponseWriter, r *http.Request) {
	segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	saleID := segments[len(segments)-2]

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.voided[saleID] {
		writeJSON(w, http.StatusOK, dh.VoidSaleResponse{Reversed: false})
		return
	}
	for _, s := range f.sales {
		if s.saleID == saleID {
			f.voided[saleID] = true
			writeJSON(w, http.StatusOK, dh.VoidSaleResponse{Reversed: true})
			return
		}
	}
	http.NotFound(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}

func TestInventoryAdapter_RecordInventorySale_ContractFake(t *testing.T) {
	fake := newFakeDHSaleServer(t)
	a := adapter.NewInventoryAdapter(dh.NewClient(fake.URL, dh.WithEnterpriseKey("test_key")))

	req := inventory.DHSaleRequest{
		DHInventoryID:  100,
		IdempotencyKey: "slabledger-sale-key-1",
		SalePriceCents: 45000,
	}

	result, err := a.RecordInventorySale(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.Replayed, "first call records")

	result, err = a.RecordInventorySale(context.Background(), req)
	require.NoError(t, err)
	require.True(t, result.Replayed, "identical retry must replay, not double-dispose")

	t.Run("same key, different price: idempotency_key_reused", func(t *testing.T) {
		mutated := req
		mutated.SalePriceCents = 50000
		_, err := a.RecordInventorySale(context.Background(), mutated)
		require.ErrorIs(t, err, inventory.ErrDHIdempotencyKeyReused)
	})

	t.Run("same key, same body, different inventory id: idempotency_key_reused", func(t *testing.T) {
		mutated := req
		mutated.DHInventoryID = 200
		_, err := a.RecordInventorySale(context.Background(), mutated)
		require.ErrorIs(t, err, inventory.ErrDHIdempotencyKeyReused)
	})

	t.Run("missing idempotency key is rejected", func(t *testing.T) {
		noKey := req
		noKey.IdempotencyKey = ""
		noKey.DHInventoryID = 999
		_, err := a.RecordInventorySale(context.Background(), noKey)
		require.ErrorIs(t, err, inventory.ErrDHValidation)
	})
}

func TestInventoryAdapter_VoidInventorySale_ContractFake(t *testing.T) {
	fake := newFakeDHSaleServer(t)
	a := adapter.NewInventoryAdapter(dh.NewClient(fake.URL, dh.WithEnterpriseKey("test_key")))

	result, err := a.RecordInventorySale(context.Background(), inventory.DHSaleRequest{
		DHInventoryID:  1,
		IdempotencyKey: "slabledger-sale-void-1",
		SalePriceCents: 1000,
	})
	require.NoError(t, err)

	require.NoError(t, a.VoidInventorySale(context.Background(), result.DHSaleID, "returned"))
	require.NoError(t, a.VoidInventorySale(context.Background(), result.DHSaleID, "returned"),
		"re-voiding an already-voided sale is success (reversed:false), not an error")
	require.NoError(t, a.VoidInventorySale(context.Background(), "sale_never_existed", ""),
		"void of an unknown sale id is success-with-a-log")
}
