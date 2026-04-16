package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// ---------------------------------------------------------------------------
// MockDHOrdersClient implements scheduler.DHOrdersClient for testing.
// ---------------------------------------------------------------------------

// MockDHOrdersClient is a test double for scheduler.DHOrdersClient.
type MockDHOrdersClient struct {
	GetOrdersFn func(ctx context.Context, filters dh.OrderFilters) (*dh.OrdersResponse, error)
	CalledWith  dh.OrderFilters
	CallCount   int
}

func (m *MockDHOrdersClient) GetOrders(ctx context.Context, filters dh.OrderFilters) (*dh.OrdersResponse, error) {
	m.CallCount++
	m.CalledWith = filters
	if m.GetOrdersFn != nil {
		return m.GetOrdersFn(ctx, filters)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// MockDHInventoryListClient implements scheduler.DHInventoryListClient for testing.
// ---------------------------------------------------------------------------

// MockDHInventoryListClient is a test double for scheduler.DHInventoryListClient.
type MockDHInventoryListClient struct {
	ListInventoryFn func(ctx context.Context, filters dh.InventoryFilters) (*dh.InventoryListResponse, error)
}

func (m *MockDHInventoryListClient) ListInventory(ctx context.Context, filters dh.InventoryFilters) (*dh.InventoryListResponse, error) {
	if m.ListInventoryFn != nil {
		return m.ListInventoryFn(ctx, filters)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// MockDHFieldsUpdater implements scheduler.DHFieldsUpdater for testing.
// ---------------------------------------------------------------------------

// MockDHFieldsUpdater is a test double for scheduler.DHFieldsUpdater.
type MockDHFieldsUpdater struct {
	UpdatePurchaseDHFieldsFn func(ctx context.Context, id string, update inventory.DHFieldsUpdate) error
	Calls                    []inventory.DHFieldsUpdate
	IDs                      []string
}

func (m *MockDHFieldsUpdater) UpdatePurchaseDHFields(ctx context.Context, id string, update inventory.DHFieldsUpdate) error {
	if m.UpdatePurchaseDHFieldsFn != nil {
		return m.UpdatePurchaseDHFieldsFn(ctx, id, update)
	}
	m.IDs = append(m.IDs, id)
	m.Calls = append(m.Calls, update)
	return nil
}

// ---------------------------------------------------------------------------
// MockPurchaseByCertLookup implements scheduler.PurchaseByCertLookup for testing.
// ---------------------------------------------------------------------------

// MockPurchaseByCertLookup is a test double for scheduler.PurchaseByCertLookup.
type MockPurchaseByCertLookup struct {
	GetPurchaseIDByCertNumberFn   func(ctx context.Context, certNumber string) (string, error)
	GetPurchaseIDsByCertNumbersFn func(ctx context.Context, certNumbers []string) (map[string]string, error)
	GetDHStatusByCertNumberFn     func(ctx context.Context, certNumber string) (string, string, error)
	Mapping                       map[string]string // fallback: certNumber -> purchaseID
	DHStatusByCert                map[string]string // fallback: certNumber -> dhStatus (used alongside Mapping)
}

func (m *MockPurchaseByCertLookup) GetPurchaseIDByCertNumber(ctx context.Context, certNumber string) (string, error) {
	if m.GetPurchaseIDByCertNumberFn != nil {
		return m.GetPurchaseIDByCertNumberFn(ctx, certNumber)
	}
	if m.Mapping != nil {
		return m.Mapping[certNumber], nil
	}
	return "", nil
}

func (m *MockPurchaseByCertLookup) GetPurchaseIDsByCertNumbers(ctx context.Context, certNumbers []string) (map[string]string, error) {
	if m.GetPurchaseIDsByCertNumbersFn != nil {
		return m.GetPurchaseIDsByCertNumbersFn(ctx, certNumbers)
	}
	if m.Mapping == nil {
		return make(map[string]string), nil
	}
	out := make(map[string]string, len(certNumbers))
	for _, c := range certNumbers {
		if id, ok := m.Mapping[c]; ok {
			out[c] = id
		}
	}
	return out, nil
}

func (m *MockPurchaseByCertLookup) GetDHStatusByCertNumber(ctx context.Context, certNumber string) (string, string, error) {
	if m.GetDHStatusByCertNumberFn != nil {
		return m.GetDHStatusByCertNumberFn(ctx, certNumber)
	}
	id := ""
	if m.Mapping != nil {
		id = m.Mapping[certNumber]
	}
	status := ""
	if m.DHStatusByCert != nil {
		status = m.DHStatusByCert[certNumber]
	}
	return id, status, nil
}
