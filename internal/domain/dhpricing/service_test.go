package dhpricing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

type fakeLookup struct {
	purchases map[string]*inventory.Purchase
	drift     []inventory.Purchase
	getErr    error
	listErr   error
}

func (f *fakeLookup) GetPurchase(_ context.Context, id string) (*inventory.Purchase, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	p, ok := f.purchases[id]
	if !ok {
		return nil, inventory.ErrPurchaseNotFound
	}
	return p, nil
}

func (f *fakeLookup) ListDHPriceDrift(_ context.Context) ([]inventory.Purchase, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.drift, nil
}

type updaterCall struct {
	inventoryID       int
	status            string
	listingPriceCents int
}

type fakeUpdater struct {
	calls            []updaterCall
	returnPriceCents int
	err              error
}

func (f *fakeUpdater) UpdateInventoryStatus(_ context.Context, invID int, update dhlisting.DHInventoryStatusUpdate) (int, error) {
	f.calls = append(f.calls, updaterCall{invID, update.Status, update.ListingPriceCents})
	if f.err != nil {
		return 0, f.err
	}
	if f.returnPriceCents == 0 {
		return update.ListingPriceCents, nil
	}
	return f.returnPriceCents, nil
}

type writerCall struct {
	id       string
	price    int
	syncedAt time.Time
}

type fakeWriter struct {
	calls []writerCall
	err   error
}

func (f *fakeWriter) UpdatePurchaseDHPriceSync(_ context.Context, id string, price int, syncedAt time.Time) error {
	f.calls = append(f.calls, writerCall{id, price, syncedAt})
	return f.err
}

type fakeResetter struct{ calls []string }

func (f *fakeResetter) ResetDHFieldsForRepush(_ context.Context, id string) error {
	f.calls = append(f.calls, id)
	return nil
}

func newTestService(lookup PurchaseLookup, updater DHPriceUpdater, writer DHPriceWriter, resetter DHReconcileResetter) Service {
	return NewService(lookup, updater, writer, resetter, observability.NewNoopLogger())
}

func TestSyncPurchasePrice(t *testing.T) {
	baseP := func(mods func(*inventory.Purchase)) *inventory.Purchase {
		p := &inventory.Purchase{
			ID:                  "pur-1",
			DHInventoryID:       42,
			ReviewedPriceCents:  12000,
			DHListingPriceCents: 10000,
			DHStatus:            inventory.DHStatusListed,
		}
		if mods != nil {
			mods(p)
		}
		return p
	}

	tests := []struct {
		name            string
		purchase        *inventory.Purchase
		updaterErr      error
		updaterReturn   int
		writerErr       error
		wantOutcome     Outcome
		wantUpdaterCall bool
		wantWriterCall  bool
		wantResetCall   bool
	}{
		{
			name:        "no dh inventory id → skip",
			purchase:    baseP(func(p *inventory.Purchase) { p.DHInventoryID = 0 }),
			wantOutcome: OutcomeSkippedNoInventory,
		},
		{
			name:        "zero reviewed → skip (gate B)",
			purchase:    baseP(func(p *inventory.Purchase) { p.ReviewedPriceCents = 0 }),
			wantOutcome: OutcomeSkippedZeroReviewed,
		},
		{
			name:        "reviewed matches DH → no drift, skip",
			purchase:    baseP(func(p *inventory.Purchase) { p.DHListingPriceCents = 12000 }),
			wantOutcome: OutcomeSkippedNoDrift,
		},
		{
			name:            "drift present → patch DH + persist",
			purchase:        baseP(nil),
			wantOutcome:     OutcomeSynced,
			wantUpdaterCall: true,
			wantWriterCall:  true,
		},
		{
			name:            "patch succeeds but DH returns a different price → persist DH's value",
			purchase:        baseP(nil),
			updaterReturn:   11900,
			wantOutcome:     OutcomeSynced,
			wantUpdaterCall: true,
			wantWriterCall:  true,
		},
		{
			name:            "DH returns ERR_PROV_NOT_FOUND → reset for repush",
			purchase:        baseP(nil),
			updaterErr:      apperrors.ProviderNotFound("DH", "VendorInventoryItem id=42"),
			wantOutcome:     OutcomeStaleInventoryID,
			wantUpdaterCall: true,
			wantResetCall:   true,
		},
		{
			name:            "DH network error → error outcome, no persist",
			purchase:        baseP(nil),
			updaterErr:      errors.New("500 upstream"),
			wantOutcome:     OutcomeError,
			wantUpdaterCall: true,
		},
		{
			name:            "writer error → error outcome (DH already patched)",
			purchase:        baseP(nil),
			writerErr:       errors.New("disk full"),
			wantOutcome:     OutcomeError,
			wantUpdaterCall: true,
			wantWriterCall:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lookup := &fakeLookup{purchases: map[string]*inventory.Purchase{"pur-1": tc.purchase}}
			updater := &fakeUpdater{err: tc.updaterErr, returnPriceCents: tc.updaterReturn}
			writer := &fakeWriter{err: tc.writerErr}
			resetter := &fakeResetter{}

			svc := newTestService(lookup, updater, writer, resetter)
			got := svc.SyncPurchasePrice(context.Background(), "pur-1")

			if got.Outcome != tc.wantOutcome {
				t.Errorf("Outcome = %q, want %q (err=%v)", got.Outcome, tc.wantOutcome, got.Err)
			}
			if tc.wantUpdaterCall && len(updater.calls) != 1 {
				t.Errorf("expected 1 updater call, got %d", len(updater.calls))
			}
			if !tc.wantUpdaterCall && len(updater.calls) != 0 {
				t.Errorf("expected no updater calls, got %d", len(updater.calls))
			}
			if tc.wantWriterCall && len(writer.calls) != 1 {
				t.Errorf("expected 1 writer call, got %d", len(writer.calls))
			}
			if !tc.wantWriterCall && len(writer.calls) != 0 {
				t.Errorf("expected no writer calls, got %d", len(writer.calls))
			}
			if tc.wantResetCall && len(resetter.calls) != 1 {
				t.Errorf("expected 1 resetter call, got %d", len(resetter.calls))
			}
			if !tc.wantResetCall && len(resetter.calls) != 0 {
				t.Errorf("expected no resetter calls, got %d", len(resetter.calls))
			}
		})
	}
}

func TestSyncDriftedPurchases(t *testing.T) {
	drift := []inventory.Purchase{
		{ID: "a", DHInventoryID: 1, ReviewedPriceCents: 11000, DHListingPriceCents: 10000, DHStatus: inventory.DHStatusListed},
		{ID: "b", DHInventoryID: 2, ReviewedPriceCents: 22000, DHListingPriceCents: 20000, DHStatus: inventory.DHStatusListed},
		{ID: "c", DHInventoryID: 3, ReviewedPriceCents: 33000, DHListingPriceCents: 30000, DHStatus: inventory.DHStatusInStock},
	}
	lookup := &fakeLookup{
		purchases: map[string]*inventory.Purchase{
			"a": &drift[0], "b": &drift[1], "c": &drift[2],
		},
		drift: drift,
	}
	updater := &fakeUpdater{}
	writer := &fakeWriter{}
	resetter := &fakeResetter{}

	svc := newTestService(lookup, updater, writer, resetter)
	got := svc.SyncDriftedPurchases(context.Background())

	if got.Total != 3 {
		t.Errorf("Total = %d, want 3", got.Total)
	}
	if got.ByOutcome[OutcomeSynced] != 3 {
		t.Errorf("synced = %d, want 3 (map: %v)", got.ByOutcome[OutcomeSynced], got.ByOutcome)
	}
	if len(updater.calls) != 3 {
		t.Errorf("updater calls = %d, want 3", len(updater.calls))
	}
}

func TestSyncDriftedPurchases_EmptyDriftList(t *testing.T) {
	lookup := &fakeLookup{}
	updater := &fakeUpdater{}
	writer := &fakeWriter{}
	resetter := &fakeResetter{}
	svc := newTestService(lookup, updater, writer, resetter)

	got := svc.SyncDriftedPurchases(context.Background())
	if got.Total != 0 || len(updater.calls) != 0 {
		t.Errorf("expected no work, got total=%d calls=%d", got.Total, len(updater.calls))
	}
	if got.ListErr != nil {
		t.Errorf("ListErr = %v, want nil", got.ListErr)
	}
}

func TestSyncDriftedPurchases_ListErrorSurfaces(t *testing.T) {
	listErr := errors.New("db unavailable")
	lookup := &fakeLookup{listErr: listErr}
	updater := &fakeUpdater{}
	writer := &fakeWriter{}
	resetter := &fakeResetter{}
	svc := newTestService(lookup, updater, writer, resetter)

	got := svc.SyncDriftedPurchases(context.Background())
	if got.ListErr == nil || !errors.Is(got.ListErr, listErr) {
		t.Errorf("ListErr = %v, want %v", got.ListErr, listErr)
	}
	if got.Total != 0 {
		t.Errorf("Total = %d, want 0 on list failure", got.Total)
	}
	if len(updater.calls) != 0 {
		t.Errorf("updater should not be called on list failure, got %d calls", len(updater.calls))
	}
}

// fakeUpdaterFn lets a test supply per-call behavior.
type fakeUpdaterFn struct {
	fn func(ctx context.Context, inventoryID int, status string, price int) (int, error)
}

func (f *fakeUpdaterFn) UpdateInventoryStatus(ctx context.Context, inventoryID int, update dhlisting.DHInventoryStatusUpdate) (int, error) {
	return f.fn(ctx, inventoryID, update.Status, update.ListingPriceCents)
}

func TestSyncDriftedPurchases_ContinuesOnError(t *testing.T) {
	drift := []inventory.Purchase{
		{ID: "err", DHInventoryID: 1, ReviewedPriceCents: 11000, DHListingPriceCents: 10000, DHStatus: inventory.DHStatusListed},
		{ID: "ok", DHInventoryID: 2, ReviewedPriceCents: 22000, DHListingPriceCents: 20000, DHStatus: inventory.DHStatusListed},
	}
	lookup := &fakeLookup{
		purchases: map[string]*inventory.Purchase{"err": &drift[0], "ok": &drift[1]},
		drift:     drift,
	}
	var callCount int
	updater := &fakeUpdaterFn{
		fn: func(_ context.Context, _ int, _ string, price int) (int, error) {
			callCount++
			if callCount == 1 {
				return 0, errors.New("transient")
			}
			return price, nil
		},
	}
	writer := &fakeWriter{}
	resetter := &fakeResetter{}

	svc := newTestService(lookup, updater, writer, resetter)
	got := svc.SyncDriftedPurchases(context.Background())

	if got.Total != 2 {
		t.Errorf("Total = %d, want 2", got.Total)
	}
	if got.ByOutcome[OutcomeError] != 1 || got.ByOutcome[OutcomeSynced] != 1 {
		t.Errorf("outcomes = %v, want error=1 synced=1", got.ByOutcome)
	}
}
