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

// TestListPurchases covers the three key scenarios for ListPurchases in a
// single table-driven test.
func TestListPurchases(t *testing.T) {
	certNum := "55555555"
	purchase := &inventory.Purchase{
		ID:            "purchase-1",
		CertNumber:    certNum,
		DHInventoryID: 99,
	}

	tests := []struct {
		name          string
		lookup        *mockPurchaseLookup
		lister        *mockInventoryLister
		fieldsUpdater *mockFieldsUpdater
		certs         []string
		wantListed    int
		wantSynced    int
		wantTotal     int
		wantErrSet    bool
	}{
		{
			name: "Success",
			lookup: &mockPurchaseLookup{
				purchases: map[string]*inventory.Purchase{certNum: purchase},
			},
			lister:        &mockInventoryLister{},
			fieldsUpdater: &mockFieldsUpdater{},
			certs:         []string{certNum},
			wantListed:    1,
			wantSynced:    1,
			wantTotal:     1,
		},
		{
			name: "PersistFailure_DecrementsListedCount",
			lookup: &mockPurchaseLookup{
				purchases: map[string]*inventory.Purchase{
					"12345678": {ID: "purchase-2", CertNumber: "12345678", DHInventoryID: 42},
				},
			},
			lister:        &mockInventoryLister{},
			fieldsUpdater: &mockFieldsUpdater{updateErr: errors.New("db error")},
			certs:         []string{"12345678"},
			wantListed:    0,
			wantSynced:    1, // synced is incremented before persist; persist failure only decrements listed
			wantTotal:     1,
		},
		{
			name:       "LookupFailure_ReturnsZeroResult",
			lookup:     &mockPurchaseLookup{err: errors.New("db connection lost")},
			lister:     &mockInventoryLister{},
			certs:      []string{"99999999"},
			wantListed: 0,
			wantSynced: 0,
			wantTotal:  0,
			wantErrSet: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := []DHListingServiceOption{WithDHListingLister(tc.lister)}
			if tc.fieldsUpdater != nil {
				opts = append(opts, WithDHListingFieldsUpdater(tc.fieldsUpdater))
			}
			svc := newTestService(t, tc.lookup, opts...)

			result := svc.ListPurchases(context.Background(), tc.certs)

			if result.Listed != tc.wantListed {
				t.Errorf("Listed: got %d, want %d", result.Listed, tc.wantListed)
			}
			if result.Synced != tc.wantSynced {
				t.Errorf("Synced: got %d, want %d", result.Synced, tc.wantSynced)
			}
			if result.Total != tc.wantTotal {
				t.Errorf("Total: got %d, want %d", result.Total, tc.wantTotal)
			}
			if tc.wantErrSet && result.Error == nil {
				t.Error("expected Error to be set, got nil")
			}
			if !tc.wantErrSet && result.Error != nil {
				t.Errorf("expected no Error, got %v", result.Error)
			}
		})
	}
}
