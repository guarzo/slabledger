package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// listClientByStatus serves inventory items keyed by the requested status, so a
// test can assert the sweep queries every listable status.
func listClientByStatus(
	byStatus map[string][]dh.InventoryListItem,
	err error,
	calls *[]string,
) *mocks.MockDHInventoryListClient {
	return &mocks.MockDHInventoryListClient{
		ListInventoryFn: func(_ context.Context, f dh.InventoryFilters) (*dh.InventoryListResponse, error) {
			*calls = append(*calls, f.Status)
			if err != nil {
				return nil, err
			}
			items := byStatus[f.Status]
			resp := &dh.InventoryListResponse{Items: items}
			resp.Meta.TotalCount = len(items)
			return resp, nil
		},
	}
}

func invItem(cert string, invID int) dh.InventoryListItem {
	return dh.InventoryListItem{CertNumber: cert, DHInventoryID: invID}
}

// purchaseOwning returns a repo mock whose purchases carry the given DH
// inventory IDs, so the sweep's ownership check resolves as the test intends.
func purchaseOwning(byPurchaseID map[string]int) *mocks.PurchaseRepositoryMock {
	return &mocks.PurchaseRepositoryMock{
		GetPurchaseFn: func(_ context.Context, id string) (*inventory.Purchase, error) {
			invID, ok := byPurchaseID[id]
			if !ok {
				return nil, nil
			}
			return &inventory.Purchase{ID: id, DHInventoryID: invID}, nil
		},
	}
}

// The SLA-109 shape: cards sold in person, marked sold locally, but still
// offered on DH. The sweep must retire exactly those and leave the rest alone.
func TestDHSoldReconciler_SweepDH(t *testing.T) {
	tests := []struct {
		name       string
		listed     []dh.InventoryListItem
		mapping    map[string]string // cert -> purchaseID
		statuses   map[string]string // cert -> local dh_status
		owns       map[string]int    // purchaseID -> dh_inventory_id it owns
		notifyErr  error
		lookupErr  error
		clientErr  error
		wantMarked []int
	}{
		{
			name:       "sold locally but still listed on DH is retired",
			listed:     []dh.InventoryListItem{invItem("111", 5001)},
			mapping:    map[string]string{"111": "p1"},
			statuses:   map[string]string{"111": "sold"},
			owns:       map[string]int{"p1": 5001},
			wantMarked: []int{5001},
		},
		{
			name:       "genuinely listed item is left alone",
			listed:     []dh.InventoryListItem{invItem("222", 5002)},
			mapping:    map[string]string{"222": "p2"},
			statuses:   map[string]string{"222": "listed"},
			owns:       map[string]int{"p2": 5002},
			wantMarked: nil,
		},
		{
			name:       "cert unknown locally is left alone",
			listed:     []dh.InventoryListItem{invItem("333", 5003)},
			mapping:    map[string]string{},
			statuses:   map[string]string{},
			wantMarked: nil,
		},
		{
			name:       "item without a DH inventory id is skipped",
			listed:     []dh.InventoryListItem{invItem("444", 0)},
			mapping:    map[string]string{"444": "p4"},
			statuses:   map[string]string{"444": "sold"},
			owns:       map[string]int{"p4": 0},
			wantMarked: nil,
		},
		{
			name:       "lookup failure does not abort the sweep",
			listed:     []dh.InventoryListItem{invItem("555", 5005)},
			lookupErr:  errors.New("db down"),
			wantMarked: nil,
		},
		{
			name:       "DH failure is recorded but does not panic",
			listed:     []dh.InventoryListItem{invItem("666", 5006)},
			mapping:    map[string]string{"666": "p6"},
			statuses:   map[string]string{"666": "sold"},
			owns:       map[string]int{"p6": 5006},
			notifyErr:  errors.New("dh 500"),
			wantMarked: []int{5006},
		},
		{
			name:     "sold row for a re-acquired cert does not pull the new listing",
			listed:   []dh.InventoryListItem{invItem("777", 5007)},
			mapping:  map[string]string{"777": "pOld"},
			statuses: map[string]string{"777": "sold"},
			// The stale sold purchase owns a different DH item than the one
			// currently listed under this cert, so it must be left alone.
			owns:       map[string]int{"pOld": 4004},
			wantMarked: nil,
		},
		{
			name:       "list failure leaves nothing retired",
			clientErr:  errors.New("dh unavailable"),
			wantMarked: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			client := listClientByStatus(
				map[string][]dh.InventoryListItem{dh.InventoryStatusListed: tt.listed},
				tt.clientErr, &calls)

			lookup := &mocks.MockPurchaseByCertLookup{Mapping: tt.mapping, DHStatusByCert: tt.statuses}
			if tt.lookupErr != nil {
				lookup.GetDHStatusByCertNumberFn = func(context.Context, string) (string, string, error) {
					return "", "", tt.lookupErr
				}
			}

			notifier := &mocks.DHSoldNotifierMock{
				MarkInventorySoldFn: func(context.Context, int) error { return tt.notifyErr },
			}

			s := NewDHSoldReconcilerScheduler(
				&mocks.PurchaseRepositoryMock{}, &mocks.PurchaseRepositoryMock{},
				observability.NewNoopLogger(), DHSoldReconcilerConfig{Enabled: true},
				WithDHSoldSweep(client, lookup, purchaseOwning(tt.owns), notifier),
			)

			s.sweepDH(context.Background())

			assertMarked(t, notifier.MarkedSold(), tt.wantMarked)
		})
	}
}

func assertMarked(t *testing.T, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("MarkedSold() = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("MarkedSold() = %v, want %v", got, want)
		}
	}
}

// Both listable statuses are swept: an in_stock item for a sold purchase is
// still wrong state, even though `listed` is the one carrying sale risk.
func TestDHSoldReconciler_SweepsBothListableStatuses(t *testing.T) {
	var calls []string
	client := listClientByStatus(map[string][]dh.InventoryListItem{
		dh.InventoryStatusListed:  {invItem("111", 1)},
		dh.InventoryStatusInStock: {invItem("222", 2)},
	}, nil, &calls)

	notifier := &mocks.DHSoldNotifierMock{}
	s := NewDHSoldReconcilerScheduler(
		&mocks.PurchaseRepositoryMock{}, &mocks.PurchaseRepositoryMock{},
		observability.NewNoopLogger(), DHSoldReconcilerConfig{Enabled: true},
		WithDHSoldSweep(client, &mocks.MockPurchaseByCertLookup{
			Mapping:        map[string]string{"111": "p1", "222": "p2"},
			DHStatusByCert: map[string]string{"111": "sold", "222": "sold"},
		}, purchaseOwning(map[string]int{"p1": 1, "p2": 2}), notifier),
	)

	s.sweepDH(context.Background())

	if len(notifier.MarkedSold()) != 2 {
		t.Fatalf("MarkedSold() = %v, want both items retired", notifier.MarkedSold())
	}
	if len(calls) != 2 {
		t.Fatalf("ListInventory called for %v, want both listable statuses", calls)
	}
}

// Without the option the reconciler must keep its previous behavior exactly:
// repair the local column, touch no external service.
func TestDHSoldReconciler_SweepDisabledWhenUnwired(t *testing.T) {
	var updated []string
	repo := &mocks.PurchaseRepositoryMock{
		ListStaleDHStatusSoldPurchasesFn: func(context.Context) ([]string, error) {
			return []string{"p1"}, nil
		},
		UpdatePurchaseDHStatusFn: func(_ context.Context, id, _ string) error {
			updated = append(updated, id)
			return nil
		},
	}

	s := NewDHSoldReconcilerScheduler(repo, repo,
		observability.NewNoopLogger(), DHSoldReconcilerConfig{Enabled: true})

	s.reconcile(context.Background())

	if len(updated) != 1 || updated[0] != "p1" {
		t.Fatalf("updated = %v, want [p1]", updated)
	}
}

// The local pass must be visible to the sweep in the same cycle, not the next.
func TestDHSoldReconciler_LocalPassRunsBeforeSweep(t *testing.T) {
	var calls []string
	client := listClientByStatus(map[string][]dh.InventoryListItem{
		dh.InventoryStatusListed: {invItem("111", 9001)},
	}, nil, &calls)

	// The cert starts stale ('listed' locally despite having a sale). Only the
	// local pass flips it to 'sold', and the sweep retires nothing unless it
	// observes that flip — so a retire proves the ordering rather than merely
	// showing both passes ran.
	var updated []string
	localStatus := string(inventory.DHStatusListed)
	repo := &mocks.PurchaseRepositoryMock{
		ListStaleDHStatusSoldPurchasesFn: func(context.Context) ([]string, error) {
			return []string{"p1"}, nil
		},
		UpdatePurchaseDHStatusFn: func(_ context.Context, id, status string) error {
			updated = append(updated, id)
			localStatus = status
			return nil
		},
	}
	notifier := &mocks.DHSoldNotifierMock{}

	s := NewDHSoldReconcilerScheduler(repo, repo,
		observability.NewNoopLogger(), DHSoldReconcilerConfig{Enabled: true},
		WithDHSoldSweep(client, &mocks.MockPurchaseByCertLookup{
			GetDHStatusByCertNumberFn: func(context.Context, string) (string, string, error) {
				return "p1", localStatus, nil
			},
		}, purchaseOwning(map[string]int{"p1": 9001}), notifier),
	)

	s.reconcile(context.Background())

	if len(updated) != 1 {
		t.Fatalf("local pass did not run: updated = %v", updated)
	}
	if len(notifier.MarkedSold()) != 1 || notifier.MarkedSold()[0] != 9001 {
		t.Fatalf("sweep did not observe the local pass's flip to sold: marked = %v", notifier.MarkedSold())
	}
}
