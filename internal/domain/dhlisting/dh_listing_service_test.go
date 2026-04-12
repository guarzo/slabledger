package dhlisting

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// --- mock implementations ---

type mockPurchaseLookup struct {
	purchases map[string]*inventory.Purchase
	err       error
}

func (m *mockPurchaseLookup) GetPurchasesByCertNumbers(_ context.Context, certNumbers []string) (map[string]*inventory.Purchase, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make(map[string]*inventory.Purchase)
	for _, cn := range certNumbers {
		if p, ok := m.purchases[cn]; ok {
			result[cn] = p
		}
	}
	return result, nil
}

type mockInventoryLister struct {
	updateStatusErr error
	syncChannelsErr error
}

func (m *mockInventoryLister) UpdateInventoryStatus(_ context.Context, _ int, _ string) error {
	return m.updateStatusErr
}

func (m *mockInventoryLister) SyncChannels(_ context.Context, _ int, _ []string) error {
	return m.syncChannelsErr
}

type mockFieldsUpdater struct {
	updateErr error
	calls     []inventory.DHFieldsUpdate
}

func (m *mockFieldsUpdater) UpdatePurchaseDHFields(_ context.Context, _ string, update inventory.DHFieldsUpdate) error {
	m.calls = append(m.calls, update)
	return m.updateErr
}

func newTestService(t *testing.T, lookup DHListingPurchaseLookup, opts ...DHListingServiceOption) DHListingService {
	t.Helper()
	svc, err := NewDHListingService(lookup, observability.NewNoopLogger(), opts...)
	if err != nil {
		t.Fatalf("NewDHListingService: %v", err)
	}
	return svc
}

// TestListPurchases_PersistFailure_DecrementsListedCount verifies that when
// UpdatePurchaseDHFields fails after a successful UpdateInventoryStatus +
// SyncChannels, the listed count is NOT incremented (decremented back to 0).
func TestListPurchases_PersistFailure_DecrementsListedCount(t *testing.T) {
	certNum := "12345678"
	purchase := &inventory.Purchase{
		ID:            "purchase-1",
		CertNumber:    certNum,
		DHInventoryID: 42,
	}

	lookup := &mockPurchaseLookup{
		purchases: map[string]*inventory.Purchase{certNum: purchase},
	}
	lister := &mockInventoryLister{} // success
	fieldsUpdater := &mockFieldsUpdater{updateErr: errors.New("db error")}

	svc := newTestService(t, lookup,
		WithDHListingLister(lister),
		WithDHListingFieldsUpdater(fieldsUpdater),
	)

	result := svc.ListPurchases(context.Background(), []string{certNum})

	if result.Listed != 0 {
		t.Errorf("expected Listed=0 when persist fails, got %d", result.Listed)
	}
	if result.Total != 1 {
		t.Errorf("expected Total=1, got %d", result.Total)
	}
}

// TestListPurchases_LookupFailure_ReturnsZeroResult verifies that when
// GetPurchasesByCertNumbers fails, an empty result is returned.
func TestListPurchases_LookupFailure_ReturnsZeroResult(t *testing.T) {
	lookup := &mockPurchaseLookup{err: errors.New("db connection lost")}
	lister := &mockInventoryLister{}

	svc := newTestService(t, lookup, WithDHListingLister(lister))

	result := svc.ListPurchases(context.Background(), []string{"99999999"})

	if result.Listed != 0 || result.Synced != 0 || result.Total != 0 {
		t.Errorf("expected zero result on lookup failure, got %+v", result)
	}
}

// TestListPurchases_Success verifies normal path increments listed and synced.
func TestListPurchases_Success(t *testing.T) {
	certNum := "55555555"
	purchase := &inventory.Purchase{
		ID:            "purchase-2",
		CertNumber:    certNum,
		DHInventoryID: 99,
	}

	lookup := &mockPurchaseLookup{
		purchases: map[string]*inventory.Purchase{certNum: purchase},
	}
	lister := &mockInventoryLister{}
	fieldsUpdater := &mockFieldsUpdater{}

	svc := newTestService(t, lookup,
		WithDHListingLister(lister),
		WithDHListingFieldsUpdater(fieldsUpdater),
	)

	result := svc.ListPurchases(context.Background(), []string{certNum})

	if result.Listed != 1 {
		t.Errorf("expected Listed=1, got %d", result.Listed)
	}
	if result.Synced != 1 {
		t.Errorf("expected Synced=1, got %d", result.Synced)
	}
	if result.Total != 1 {
		t.Errorf("expected Total=1, got %d", result.Total)
	}
}
