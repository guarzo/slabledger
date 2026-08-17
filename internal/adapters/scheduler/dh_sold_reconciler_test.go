package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

type stubStaleLister struct {
	ids []string
	err error
}

func (s stubStaleLister) ListStaleDHStatusSoldPurchases(context.Context) ([]string, error) {
	return s.ids, s.err
}

type stubDHStatusUpdater struct{ updated []string }

func (s *stubDHStatusUpdater) UpdatePurchaseDHStatus(_ context.Context, id, _ string) error {
	s.updated = append(s.updated, id)
	return nil
}

// stubSweepClient serves one page of inventory per status.
type stubSweepClient struct {
	byStatus map[string][]dh.InventoryListItem
	err      error
	calls    []string
}

func (c *stubSweepClient) ListInventory(_ context.Context, f dh.InventoryFilters) (*dh.InventoryListResponse, error) {
	c.calls = append(c.calls, f.Status)
	if c.err != nil {
		return nil, c.err
	}
	items := c.byStatus[f.Status]
	resp := &dh.InventoryListResponse{Items: items}
	resp.Meta.TotalCount = len(items)
	return resp, nil
}

// stubCertLookup maps cert number -> (purchaseID, local dh_status).
type stubCertLookup struct {
	byCert map[string][2]string
	err    error
}

func (l stubCertLookup) GetDHStatusByCertNumber(_ context.Context, cert string) (string, string, error) {
	if l.err != nil {
		return "", "", l.err
	}
	v, ok := l.byCert[cert]
	if !ok {
		return "", "", nil
	}
	return v[0], v[1], nil
}

type stubSoldNotifier struct {
	marked []int
	err    error
}

func (n *stubSoldNotifier) MarkInventorySold(_ context.Context, id int) error {
	n.marked = append(n.marked, id)
	return n.err
}

func item(cert string, invID int) dh.InventoryListItem {
	return dh.InventoryListItem{CertNumber: cert, DHInventoryID: invID}
}

// The SLA-109 shape: cards sold in person, marked sold locally, but still
// offered on DH. The sweep must retire exactly those and leave the rest alone.
func TestDHSoldReconciler_SweepDH(t *testing.T) {
	tests := []struct {
		name       string
		listed     []dh.InventoryListItem
		byCert     map[string][2]string
		notifyErr  error
		lookupErr  error
		clientErr  error
		wantMarked []int
	}{
		{
			name:       "sold locally but still listed on DH is retired",
			listed:     []dh.InventoryListItem{item("111", 5001)},
			byCert:     map[string][2]string{"111": {"p1", "sold"}},
			wantMarked: []int{5001},
		},
		{
			name:       "genuinely listed item is left alone",
			listed:     []dh.InventoryListItem{item("222", 5002)},
			byCert:     map[string][2]string{"222": {"p2", "listed"}},
			wantMarked: nil,
		},
		{
			name:       "cert unknown locally is left alone",
			listed:     []dh.InventoryListItem{item("333", 5003)},
			byCert:     map[string][2]string{},
			wantMarked: nil,
		},
		{
			name:       "item without a DH inventory id is skipped",
			listed:     []dh.InventoryListItem{item("444", 0)},
			byCert:     map[string][2]string{"444": {"p4", "sold"}},
			wantMarked: nil,
		},
		{
			name:       "lookup failure does not abort the sweep",
			listed:     []dh.InventoryListItem{item("555", 5005)},
			byCert:     map[string][2]string{"555": {"p5", "sold"}},
			lookupErr:  errors.New("db down"),
			wantMarked: nil,
		},
		{
			name:       "DH failure is recorded but does not panic",
			listed:     []dh.InventoryListItem{item("666", 5006)},
			byCert:     map[string][2]string{"666": {"p6", "sold"}},
			notifyErr:  errors.New("dh 500"),
			wantMarked: []int{5006},
		},
		{
			name:       "list failure leaves nothing retired",
			clientErr:  errors.New("dh unavailable"),
			wantMarked: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &stubSweepClient{
				byStatus: map[string][]dh.InventoryListItem{dh.InventoryStatusListed: tt.listed},
				err:      tt.clientErr,
			}
			notifier := &stubSoldNotifier{err: tt.notifyErr}
			s := NewDHSoldReconcilerScheduler(
				stubStaleLister{}, &stubDHStatusUpdater{},
				observability.NewNoopLogger(), DHSoldReconcilerConfig{Enabled: true},
				WithDHSoldSweep(client, stubCertLookup{byCert: tt.byCert, err: tt.lookupErr}, notifier),
			)

			s.sweepDH(context.Background())

			if len(notifier.marked) != len(tt.wantMarked) {
				t.Fatalf("marked = %v, want %v", notifier.marked, tt.wantMarked)
			}
			for i := range notifier.marked {
				if notifier.marked[i] != tt.wantMarked[i] {
					t.Fatalf("marked = %v, want %v", notifier.marked, tt.wantMarked)
				}
			}
		})
	}
}

// Both listable statuses are swept: an in_stock item for a sold purchase is
// still wrong state, even though `listed` is the one carrying sale risk.
func TestDHSoldReconciler_SweepsBothListableStatuses(t *testing.T) {
	client := &stubSweepClient{byStatus: map[string][]dh.InventoryListItem{
		dh.InventoryStatusListed:  {item("111", 1)},
		dh.InventoryStatusInStock: {item("222", 2)},
	}}
	notifier := &stubSoldNotifier{}
	s := NewDHSoldReconcilerScheduler(
		stubStaleLister{}, &stubDHStatusUpdater{},
		observability.NewNoopLogger(), DHSoldReconcilerConfig{Enabled: true},
		WithDHSoldSweep(client, stubCertLookup{byCert: map[string][2]string{
			"111": {"p1", "sold"}, "222": {"p2", "sold"},
		}}, notifier),
	)

	s.sweepDH(context.Background())

	if len(notifier.marked) != 2 {
		t.Fatalf("marked = %v, want both items retired", notifier.marked)
	}
	if len(client.calls) != 2 {
		t.Fatalf("ListInventory called for %v, want both listable statuses", client.calls)
	}
}

// Without the option the reconciler must keep its previous behavior exactly:
// repair the local column, touch no external service.
func TestDHSoldReconciler_SweepDisabledWhenUnwired(t *testing.T) {
	updater := &stubDHStatusUpdater{}
	s := NewDHSoldReconcilerScheduler(
		stubStaleLister{ids: []string{"p1"}}, updater,
		observability.NewNoopLogger(), DHSoldReconcilerConfig{Enabled: true},
	)

	s.reconcile(context.Background())

	if len(updater.updated) != 1 || updater.updated[0] != "p1" {
		t.Fatalf("updated = %v, want [p1]", updater.updated)
	}
}

// The local pass must be visible to the sweep in the same cycle, not the next.
func TestDHSoldReconciler_LocalPassRunsBeforeSweep(t *testing.T) {
	client := &stubSweepClient{byStatus: map[string][]dh.InventoryListItem{
		dh.InventoryStatusListed: {item("111", 9001)},
	}}
	notifier := &stubSoldNotifier{}
	updater := &stubDHStatusUpdater{}
	s := NewDHSoldReconcilerScheduler(
		stubStaleLister{ids: []string{"p1"}}, updater,
		observability.NewNoopLogger(), DHSoldReconcilerConfig{Enabled: true},
		WithDHSoldSweep(client, stubCertLookup{byCert: map[string][2]string{
			"111": {"p1", string(inventory.DHStatusSold)},
		}}, notifier),
	)

	s.reconcile(context.Background())

	if len(updater.updated) != 1 {
		t.Fatalf("local pass did not run: updated = %v", updater.updated)
	}
	if len(notifier.marked) != 1 || notifier.marked[0] != 9001 {
		t.Fatalf("sweep did not run after local pass: marked = %v", notifier.marked)
	}
}
