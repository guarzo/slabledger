package dhlisting_test

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// --- local adapters (not duplicated fakes — just data-shape shims) ---------

// purchaseLookupByCert returns pre-seeded purchases keyed by cert number.
// Lookups through this shim are the only way to exercise ListPurchases
// without coupling the tests to any particular store implementation.
type purchaseLookupByCert struct {
	byCert map[string]*inventory.Purchase
}

func (p *purchaseLookupByCert) GetPurchasesByCertNumbers(_ context.Context, certs []string) (map[string]*inventory.Purchase, error) {
	out := make(map[string]*inventory.Purchase)
	for _, c := range certs {
		if x, ok := p.byCert[c]; ok {
			out[c] = x
		}
	}
	return out, nil
}

// noopLister satisfies dhlisting.DHInventoryLister without doing anything —
// the inline-push tests only care about the pre-list path.
type noopLister struct{}

func (noopLister) UpdateInventoryStatus(_ context.Context, _ int, _ dhlisting.DHInventoryStatusUpdate) (int, error) {
	return 0, nil
}
func (noopLister) SyncChannels(_ context.Context, _ int, _ []string) error { return nil }

// --- helper ----------------------------------------------------------------

type serviceDeps struct {
	importer *mocks.MockDHPSAImporter
	fields   *mocks.MockDHFieldsUpdater
	status   *mocks.MockDHPushStatusUpdater
	cardSvr  *mocks.DHCardIDSaverMock
	events   *mocks.MockEventRecorder
}

func buildService(t *testing.T, purchase *inventory.Purchase, importer *mocks.MockDHPSAImporter) (dhlisting.Service, serviceDeps) {
	t.Helper()
	lookup := &purchaseLookupByCert{byCert: map[string]*inventory.Purchase{purchase.CertNumber: purchase}}
	deps := serviceDeps{
		importer: importer,
		fields:   &mocks.MockDHFieldsUpdater{},
		status:   &mocks.MockDHPushStatusUpdater{},
		cardSvr:  &mocks.DHCardIDSaverMock{},
		events:   &mocks.MockEventRecorder{},
	}
	svc, err := dhlisting.NewDHListingService(
		lookup,
		observability.NewNoopLogger(),
		dhlisting.WithDHListingPSAImporter(importer),
		dhlisting.WithDHListingLister(noopLister{}),
		dhlisting.WithDHListingFieldsUpdater(deps.fields),
		dhlisting.WithDHListingPushStatusUpdater(deps.status),
		dhlisting.WithDHListingCardIDSaver(deps.cardSvr),
		dhlisting.WithEventRecorder(deps.events),
	)
	if err != nil {
		t.Fatalf("NewDHListingService: %v", err)
	}
	return svc, deps
}

// pending is a minimal purchase in the state the inline push acts on.
func pending(cert string) *inventory.Purchase {
	return &inventory.Purchase{
		ID:           "p1",
		CertNumber:   cert,
		CardName:     "Pikachu",
		SetName:      "Base",
		CardNumber:   "25",
		CardYear:     "1999",
		DHPushStatus: inventory.DHPushStatusPending,
	}
}

// --- tests -----------------------------------------------------------------

func TestListPurchases_InlineMatchedSuccess(t *testing.T) {
	importer := &mocks.MockDHPSAImporter{
		Results: [][]dhlisting.DHPSAImportResult{{
			{Resolution: dhlisting.PSAImportStatusMatched, DHCardID: 999, DHInventoryID: 1234, DHStatus: "in_stock"},
		}},
	}
	svc, deps := buildService(t, pending("111"), importer)

	svc.ListPurchases(context.Background(), []string{"111"})

	if importer.Calls != 1 {
		t.Fatalf("expected 1 psa_import call, got %d", importer.Calls)
	}
	if len(deps.fields.Calls) != 1 || deps.fields.Calls[0].InventoryID != 1234 {
		t.Fatalf("expected fields persisted with inventory ID 1234, got %+v", deps.fields.Calls)
	}
	if n := len(deps.status.Calls); n == 0 || deps.status.Calls[0].Status != inventory.DHPushStatusMatched {
		t.Fatalf("expected first status call to be matched, got %+v", deps.status.Calls)
	}
	foundPushedEvent := false
	for _, e := range deps.events.Events {
		if e.Type == dhevents.TypePushed {
			foundPushedEvent = true
			break
		}
	}
	if !foundPushedEvent {
		t.Fatalf("expected a pushed event, got %+v", deps.events.Events)
	}
}

func TestListPurchases_InlineRateLimitedRotatesAndRetries(t *testing.T) {
	keysLeft := 3
	importer := &mocks.MockDHPSAImporter{
		Results: [][]dhlisting.DHPSAImportResult{
			{{Resolution: dhlisting.PSAImportStatusPSAError, RateLimited: true}},
			{{Resolution: dhlisting.PSAImportStatusMatched, DHCardID: 5, DHInventoryID: 55}},
		},
		RotatePSAKeyFn: func() bool {
			if keysLeft > 0 {
				keysLeft--
				return true
			}
			return false
		},
	}
	svc, deps := buildService(t, pending("111"), importer)

	svc.ListPurchases(context.Background(), []string{"111"})

	if importer.Calls != 2 {
		t.Fatalf("expected 2 psa_import calls (rotate then succeed), got %d", importer.Calls)
	}
	if importer.RotateCalls != 1 {
		t.Fatalf("expected 1 rotation, got %d", importer.RotateCalls)
	}
	if len(deps.fields.Calls) != 1 || deps.fields.Calls[0].InventoryID != 55 {
		t.Fatalf("expected inventory ID 55 persisted, got %+v", deps.fields.Calls)
	}
}

func TestListPurchases_InlinePSAErrorLeavesPending(t *testing.T) {
	importer := &mocks.MockDHPSAImporter{
		Results: [][]dhlisting.DHPSAImportResult{{
			{Resolution: dhlisting.PSAImportStatusPSAError, Error: "Certificate not found in PSA database"},
		}},
	}
	svc, deps := buildService(t, pending("111"), importer)

	svc.ListPurchases(context.Background(), []string{"111"})

	if len(deps.fields.Calls) != 0 {
		t.Fatalf("expected no fields persisted on psa_error, got %+v", deps.fields.Calls)
	}
	for _, c := range deps.status.Calls {
		if c.Status == inventory.DHPushStatusMatched {
			t.Fatalf("psa_error should not flip status to matched, got %+v", deps.status.Calls)
		}
	}
}

func TestListPurchases_InlineMissingIDsLeavesPending(t *testing.T) {
	importer := &mocks.MockDHPSAImporter{
		Results: [][]dhlisting.DHPSAImportResult{{
			{Resolution: dhlisting.PSAImportStatusMatched, DHCardID: 0, DHInventoryID: 0},
		}},
	}
	svc, deps := buildService(t, pending("111"), importer)

	svc.ListPurchases(context.Background(), []string{"111"})

	if len(deps.fields.Calls) != 0 {
		t.Fatalf("expected no persistence when IDs are missing, got %+v", deps.fields.Calls)
	}
}

func TestListPurchases_InlineAPIErrorLeavesPending(t *testing.T) {
	importer := &mocks.MockDHPSAImporter{
		Errs: []error{errors.New("connection refused")},
	}
	svc, deps := buildService(t, pending("111"), importer)

	svc.ListPurchases(context.Background(), []string{"111"})

	if len(deps.fields.Calls) != 0 {
		t.Fatalf("expected no persistence on API error, got %+v", deps.fields.Calls)
	}
}
