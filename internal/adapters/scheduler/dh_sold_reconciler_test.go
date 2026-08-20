package scheduler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// stubSalesLister serves a fixed purchaseID -> sale map, satisfying
// DHSalesByPurchaseLister.
type stubSalesLister struct {
	byPurchaseID map[string]*inventory.Sale
	err          error
}

func (l *stubSalesLister) GetSalesByPurchaseIDs(_ context.Context, ids []string) (map[string]*inventory.Sale, error) {
	if l.err != nil {
		return nil, l.err
	}
	out := make(map[string]*inventory.Sale, len(ids))
	for _, id := range ids {
		if s, ok := l.byPurchaseID[id]; ok {
			out[id] = s
		}
	}
	return out, nil
}

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

// resolverFor returns a repo mock that maps DH inventory IDs to purchases with
// the given local dh_status. Inventory IDs absent from the map are items DH has
// but we do not, and must be left alone.
func resolverFor(byInventoryID map[int]inventory.DHStatus, err error) *mocks.PurchaseRepositoryMock {
	return &mocks.PurchaseRepositoryMock{
		GetPurchasesByDHInventoryIDsFn: func(_ context.Context, ids []int) (map[int]*inventory.Purchase, error) {
			if err != nil {
				return nil, err
			}
			out := make(map[int]*inventory.Purchase, len(ids))
			for _, id := range ids {
				if status, ok := byInventoryID[id]; ok {
					out[id] = &inventory.Purchase{
						ID:            fmt.Sprintf("p-%d", id),
						DHInventoryID: id,
						DHStatus:      status,
					}
				}
			}
			return out, nil
		},
	}
}

// saleFor builds a minimal sale for a purchase, so a sweep test can populate
// the stubSalesLister for purchases it expects the sweep to record.
func saleFor(purchaseID string) *inventory.Sale {
	return &inventory.Sale{ID: "s-" + purchaseID, PurchaseID: purchaseID, SalePriceCents: 12345, SaleDate: "2026-07-01"}
}

// assertRecorded checks the set of DH inventory IDs the sweep recorded a sale
// for. RecordInventorySale's own request carries the inventory id, so this
// asserts on recorder.RecordedSales() rather than a retired-notifier call.
func assertRecorded(t *testing.T, got []inventory.DHSaleRequest, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("RecordedSales() = %+v, want inventory ids %v", got, want)
	}
	for i := range got {
		if got[i].DHInventoryID != want[i] {
			t.Fatalf("RecordedSales() = %+v, want inventory ids %v", got, want)
		}
	}
}

// The SLA-109 shape: cards sold in person, marked sold locally, but still
// offered on DH. The sweep must record a real sale for exactly those and leave
// the rest alone.
func TestDHSoldReconciler_SweepDH(t *testing.T) {
	tests := []struct {
		name       string
		listed     []dh.InventoryListItem
		purchases  map[int]inventory.DHStatus // dh_inventory_id -> local dh_status
		resolveErr error
		recordErr  error
		clientErr  error
		wantMarked []int
	}{
		{
			name:       "sold locally but still listed on DH is recorded",
			listed:     []dh.InventoryListItem{invItem("111", 5001)},
			purchases:  map[int]inventory.DHStatus{5001: inventory.DHStatusSold},
			wantMarked: []int{5001},
		},
		{
			name:       "genuinely listed item is left alone",
			listed:     []dh.InventoryListItem{invItem("222", 5002)},
			purchases:  map[int]inventory.DHStatus{5002: inventory.DHStatusListed},
			wantMarked: nil,
		},
		{
			name:       "DH item with no local purchase is left alone",
			listed:     []dh.InventoryListItem{invItem("333", 5003)},
			purchases:  map[int]inventory.DHStatus{},
			wantMarked: nil,
		},
		{
			name:       "item without a DH inventory id is skipped",
			listed:     []dh.InventoryListItem{invItem("444", 0)},
			purchases:  map[int]inventory.DHStatus{},
			wantMarked: nil,
		},
		{
			// The whole point of keying on inventory ID: the cert is shared with
			// an older sold purchase, but DH is offering the new one, so the
			// resolver returns the new purchase and nothing is recorded.
			name:       "re-acquired cert keeps its new listing",
			listed:     []dh.InventoryListItem{invItem("777", 5007)},
			purchases:  map[int]inventory.DHStatus{5007: inventory.DHStatusListed},
			wantMarked: nil,
		},
		{
			name:       "resolve failure does not record anything",
			listed:     []dh.InventoryListItem{invItem("555", 5005)},
			resolveErr: errors.New("db down"),
			wantMarked: nil,
		},
		{
			name:       "DH failure is recorded but does not panic",
			listed:     []dh.InventoryListItem{invItem("666", 5006)},
			purchases:  map[int]inventory.DHStatus{5006: inventory.DHStatusSold},
			recordErr:  errors.New("dh 500"),
			wantMarked: []int{5006}, // RecordInventorySale records the attempt even though it errors
		},
		{
			name:       "list failure leaves nothing recorded",
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

			sales := map[string]*inventory.Sale{}
			store := mocks.NewInMemoryCampaignStore()
			for id, status := range tt.purchases {
				if status == inventory.DHStatusSold {
					pid := fmt.Sprintf("p-%d", id)
					sale := saleFor(pid)
					sales[pid] = sale
					// recordSale mints the idempotency key via the writer, which
					// (like the real store) requires the sale row to already exist.
					store.Sales[sale.ID] = sale
				}
			}

			recorder := &mocks.DHSaleRecorderMock{
				RecordInventorySaleFn: func(_ context.Context, req inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
					if tt.recordErr != nil {
						return nil, tt.recordErr
					}
					return &inventory.DHSaleResult{DHSaleID: fmt.Sprintf("dh-%d", req.DHInventoryID), Delisted: true}, nil
				},
			}

			s := NewDHSoldReconcilerScheduler(
				&mocks.PurchaseRepositoryMock{}, &mocks.PurchaseRepositoryMock{},
				observability.NewNoopLogger(), DHSoldReconcilerConfig{Enabled: true},
				WithDHSoldSweep(client, resolverFor(tt.purchases, tt.resolveErr),
					&stubSalesLister{byPurchaseID: sales}, recorder, store, &mocks.PurchaseRepositoryMock{}),
			)

			s.sweepDH(context.Background())

			gotIDs := make([]int, 0, len(recorder.RecordedSales()))
			for _, req := range recorder.RecordedSales() {
				gotIDs = append(gotIDs, req.DHInventoryID)
			}
			if len(gotIDs) != len(tt.wantMarked) {
				t.Fatalf("recorded inventory ids = %v, want %v", gotIDs, tt.wantMarked)
			}
			for i := range gotIDs {
				if gotIDs[i] != tt.wantMarked[i] {
					t.Fatalf("recorded inventory ids = %v, want %v", gotIDs, tt.wantMarked)
				}
			}
		})
	}
}

// Zero inventory IDs must never reach the resolver — they are DH's "not linked"
// sentinel and would match every unpushed purchase.
func TestDHSoldReconciler_SweepSkipsZeroInventoryIDs(t *testing.T) {
	var calls []string
	client := listClientByStatus(map[string][]dh.InventoryListItem{
		dh.InventoryStatusListed: {invItem("111", 0), invItem("222", 42)},
	}, nil, &calls)

	var gotIDs []int
	repo := &mocks.PurchaseRepositoryMock{
		GetPurchasesByDHInventoryIDsFn: func(_ context.Context, ids []int) (map[int]*inventory.Purchase, error) {
			gotIDs = append(gotIDs, ids...)
			return map[int]*inventory.Purchase{}, nil
		},
	}
	store := mocks.NewInMemoryCampaignStore()

	s := NewDHSoldReconcilerScheduler(
		&mocks.PurchaseRepositoryMock{}, &mocks.PurchaseRepositoryMock{},
		observability.NewNoopLogger(), DHSoldReconcilerConfig{Enabled: true},
		WithDHSoldSweep(client, repo, &stubSalesLister{}, &mocks.DHSaleRecorderMock{}, store, &mocks.PurchaseRepositoryMock{}),
	)

	s.sweepDH(context.Background())

	if len(gotIDs) != 1 || gotIDs[0] != 42 {
		t.Fatalf("resolver received %v, want only [42]", gotIDs)
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

	recorder := &mocks.DHSaleRecorderMock{
		RecordInventorySaleFn: func(_ context.Context, req inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
			return &inventory.DHSaleResult{DHSaleID: fmt.Sprintf("dh-%d", req.DHInventoryID), Delisted: true}, nil
		},
	}
	store := mocks.NewInMemoryCampaignStore()
	sale1, sale2 := saleFor("p-1"), saleFor("p-2")
	store.Sales[sale1.ID] = sale1
	store.Sales[sale2.ID] = sale2
	salesLister := &stubSalesLister{byPurchaseID: map[string]*inventory.Sale{
		"p-1": sale1, "p-2": sale2,
	}}

	s := NewDHSoldReconcilerScheduler(
		&mocks.PurchaseRepositoryMock{}, &mocks.PurchaseRepositoryMock{},
		observability.NewNoopLogger(), DHSoldReconcilerConfig{Enabled: true},
		WithDHSoldSweep(client, resolverFor(map[int]inventory.DHStatus{
			1: inventory.DHStatusSold, 2: inventory.DHStatusSold,
		}, nil), salesLister, recorder, store, &mocks.PurchaseRepositoryMock{}),
	)

	s.sweepDH(context.Background())

	if len(recorder.RecordedSales()) != 2 {
		t.Fatalf("RecordedSales() = %+v, want both items recorded", recorder.RecordedSales())
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
	// local pass flips it to 'sold', and the sweep records nothing unless it
	// observes that flip — so a record proves the ordering rather than merely
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
	recorder := &mocks.DHSaleRecorderMock{
		RecordInventorySaleFn: func(_ context.Context, req inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
			return &inventory.DHSaleResult{DHSaleID: fmt.Sprintf("dh-%d", req.DHInventoryID), Delisted: true}, nil
		},
	}
	store := mocks.NewInMemoryCampaignStore()
	sale := saleFor("p1")
	store.Sales[sale.ID] = sale
	salesLister := &stubSalesLister{byPurchaseID: map[string]*inventory.Sale{"p1": sale}}

	s := NewDHSoldReconcilerScheduler(repo, repo,
		observability.NewNoopLogger(), DHSoldReconcilerConfig{Enabled: true},
		WithDHSoldSweep(client, &mocks.PurchaseRepositoryMock{
			GetPurchasesByDHInventoryIDsFn: func(_ context.Context, ids []int) (map[int]*inventory.Purchase, error) {
				return map[int]*inventory.Purchase{
					9001: {ID: "p1", DHInventoryID: 9001, DHStatus: inventory.DHStatus(localStatus)},
				}, nil
			},
		}, salesLister, recorder, store, &mocks.PurchaseRepositoryMock{}),
	)

	s.reconcile(context.Background())

	if len(updated) != 1 {
		t.Fatalf("local pass did not run: updated = %v", updated)
	}
	assertRecorded(t, recorder.RecordedSales(), []int{9001})
}

// TestDHSoldReconciler_RecoveryPass_MintsKeyForLegacySale asserts a legacy sale
// (predating this feature, with no idempotency key) has one minted before its
// first DH call, and that same key is what DH received.
func TestDHSoldReconciler_RecoveryPass_MintsKeyForLegacySale(t *testing.T) {
	store := mocks.NewInMemoryCampaignStore()
	sale := inventory.Sale{ID: "s1", PurchaseID: "p1", SalePriceCents: 5000, SaleDate: "2026-07-01"} // no key: legacy
	store.ListSalesNeedingDHRecordFn = func(context.Context, int) ([]inventory.SaleNeedingDHRecord, error) {
		return []inventory.SaleNeedingDHRecord{{Sale: sale, DHInventoryID: 9001, PurchaseDate: "2026-06-01"}}, nil
	}
	var mintedFor, mintedKey string
	store.SetSaleIdempotencyKeyIfAbsentFn = func(_ context.Context, saleID, key string) (string, error) {
		mintedFor, mintedKey = saleID, key
		return key, nil
	}
	var setDHSaleID string
	store.SetSaleDHSaleIDFn = func(_ context.Context, _ string, dhSaleID string, _ time.Time) error {
		setDHSaleID = dhSaleID
		return nil
	}

	var gotKey string
	recorder := &mocks.DHSaleRecorderMock{
		RecordInventorySaleFn: func(_ context.Context, req inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
			gotKey = req.IdempotencyKey
			return &inventory.DHSaleResult{DHSaleID: "dh-legacy-1", Delisted: true}, nil
		},
	}

	s := NewDHSoldReconcilerScheduler(
		&mocks.PurchaseRepositoryMock{}, &mocks.PurchaseRepositoryMock{},
		observability.NewNoopLogger(), DHSoldReconcilerConfig{Enabled: true},
		WithDHSaleHandleRecovery(store, recorder, store, &mocks.PurchaseRepositoryMock{}),
	)

	s.recoverDHSaleHandles(context.Background())

	if mintedFor != "s1" {
		t.Fatalf("SetSaleIdempotencyKeyIfAbsent called for %q, want s1", mintedFor)
	}
	if mintedKey == "" || gotKey != mintedKey {
		t.Fatalf("minted key %q was not the key sent to DH (%q)", mintedKey, gotKey)
	}
	if setDHSaleID != "dh-legacy-1" {
		t.Fatalf("SetSaleDHSaleID got %q, want dh-legacy-1", setDHSaleID)
	}
}

// TestDHSoldReconciler_RecoveryPass_ReplayDoesNotDoubleRecord asserts a sale
// that already has a key is never re-minted, and DH's idempotent replay
// response still completes the local record.
func TestDHSoldReconciler_RecoveryPass_ReplayDoesNotDoubleRecord(t *testing.T) {
	store := mocks.NewInMemoryCampaignStore()
	sale := inventory.Sale{
		ID: "s1", PurchaseID: "p1", SalePriceCents: 5000, SaleDate: "2026-07-01",
		DHIdempotencyKey: "slabledger-sale-existing",
	}
	store.ListSalesNeedingDHRecordFn = func(context.Context, int) ([]inventory.SaleNeedingDHRecord, error) {
		return []inventory.SaleNeedingDHRecord{{Sale: sale, DHInventoryID: 9001, PurchaseDate: "2026-06-01"}}, nil
	}
	var mintCalls int
	store.SetSaleIdempotencyKeyIfAbsentFn = func(context.Context, string, string) (string, error) {
		mintCalls++
		return "", nil
	}
	var persistedID string
	store.SetSaleDHSaleIDFn = func(_ context.Context, _ string, dhSaleID string, _ time.Time) error {
		persistedID = dhSaleID
		return nil
	}

	recorder := &mocks.DHSaleRecorderMock{
		RecordInventorySaleFn: func(_ context.Context, req inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
			if req.IdempotencyKey != "slabledger-sale-existing" {
				t.Fatalf("replay used key %q, want the existing persisted key", req.IdempotencyKey)
			}
			return &inventory.DHSaleResult{DHSaleID: "dh-existing", Replayed: true, Delisted: true}, nil
		},
	}

	s := NewDHSoldReconcilerScheduler(
		&mocks.PurchaseRepositoryMock{}, &mocks.PurchaseRepositoryMock{},
		observability.NewNoopLogger(), DHSoldReconcilerConfig{Enabled: true},
		WithDHSaleHandleRecovery(store, recorder, store, &mocks.PurchaseRepositoryMock{}),
	)

	s.recoverDHSaleHandles(context.Background())

	if mintCalls != 0 {
		t.Fatalf("SetSaleIdempotencyKeyIfAbsent called %d times for a sale that already had a key, want 0", mintCalls)
	}
	if persistedID != "dh-existing" {
		t.Fatalf("SetSaleDHSaleID got %q, want dh-existing", persistedID)
	}
}

// TestDHSoldReconciler_RecoveryPass_SkipsConflictFlaggedSale asserts the
// scheduler records nothing for a row the lister withheld — i.e. the
// terminal-state gate (dh_sale_conflict = ”) holds end-to-end, since
// ListSalesNeedingDHRecord's own predicate excludes conflict-flagged rows.
func TestDHSoldReconciler_RecoveryPass_SkipsConflictFlaggedSale(t *testing.T) {
	store := mocks.NewInMemoryCampaignStore()
	store.ListSalesNeedingDHRecordFn = func(context.Context, int) ([]inventory.SaleNeedingDHRecord, error) {
		return nil, nil
	}
	var recordCalls int
	recorder := &mocks.DHSaleRecorderMock{
		RecordInventorySaleFn: func(context.Context, inventory.DHSaleRequest) (*inventory.DHSaleResult, error) {
			recordCalls++
			return &inventory.DHSaleResult{DHSaleID: "dh-x", Delisted: true}, nil
		},
	}

	s := NewDHSoldReconcilerScheduler(
		&mocks.PurchaseRepositoryMock{}, &mocks.PurchaseRepositoryMock{},
		observability.NewNoopLogger(), DHSoldReconcilerConfig{Enabled: true},
		WithDHSaleHandleRecovery(store, recorder, store, &mocks.PurchaseRepositoryMock{}),
	)

	s.recoverDHSaleHandles(context.Background())

	if recordCalls != 0 {
		t.Fatalf("RecordInventorySale called %d times, want 0", recordCalls)
	}
}
