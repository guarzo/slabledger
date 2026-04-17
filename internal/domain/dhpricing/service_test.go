package dhpricing

import (
	"context"
	"errors"
	"testing"
	"time"

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

func (f *fakeUpdater) UpdateInventoryStatus(_ context.Context, invID int, status string, price int) (int, error) {
	f.calls = append(f.calls, updaterCall{invID, status, price})
	if f.err != nil {
		return 0, f.err
	}
	if f.returnPriceCents == 0 {
		return price, nil
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
