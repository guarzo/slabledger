package dhlisting

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// --- doubles ---------------------------------------------------------------

type fakePSAImporter struct {
	responses [][]DHPSAImportResult
	errs      []error
	calls     int
}

func (f *fakePSAImporter) PSAImport(_ context.Context, _ []DHPSAImportItem) ([]DHPSAImportResult, error) {
	f.calls++
	if len(f.errs) > 0 {
		e := f.errs[0]
		f.errs = f.errs[1:]
		if e != nil {
			return nil, e
		}
	}
	if len(f.responses) == 0 {
		return nil, nil
	}
	r := f.responses[0]
	f.responses = f.responses[1:]
	return r, nil
}

// rotatingFakeImporter adds rotator behaviour for the rate-limit retry test.
type rotatingFakeImporter struct {
	*fakePSAImporter
	keysLeft    int
	rotateCalls int
}

func (r *rotatingFakeImporter) RotatePSAKey() bool {
	r.rotateCalls++
	if r.keysLeft > 0 {
		r.keysLeft--
		return true
	}
	return false
}

func (r *rotatingFakeImporter) ResetPSAKeyRotation() {}

type fakeFieldsUpdater struct {
	calls []inventory.DHFieldsUpdate
}

func (f *fakeFieldsUpdater) UpdatePurchaseDHFields(_ context.Context, _ string, u inventory.DHFieldsUpdate) error {
	f.calls = append(f.calls, u)
	return nil
}

type fakePushStatusUpdater struct {
	calls []string
}

func (f *fakePushStatusUpdater) UpdatePurchaseDHPushStatus(_ context.Context, _ string, status string) error {
	f.calls = append(f.calls, status)
	return nil
}

type fakeCardIDSaver struct{ calls []string }

func (f *fakeCardIDSaver) SaveExternalID(_ context.Context, _, _, _, _, externalID string) error {
	f.calls = append(f.calls, externalID)
	return nil
}

type fakeEventRec struct{ events []dhevents.Event }

func (f *fakeEventRec) Record(_ context.Context, e dhevents.Event) error {
	f.events = append(f.events, e)
	return nil
}

// --- helper ---------------------------------------------------------------

func newInlineTestService(imp DHPSAImporter) (*dhListingService, *fakeFieldsUpdater, *fakePushStatusUpdater, *fakeCardIDSaver, *fakeEventRec) {
	fields := &fakeFieldsUpdater{}
	status := &fakePushStatusUpdater{}
	cardSaver := &fakeCardIDSaver{}
	events := &fakeEventRec{}
	svc := &dhListingService{
		psaImporter:       imp,
		fieldsUpdater:     fields,
		pushStatusUpdater: status,
		cardIDSaver:       cardSaver,
		logger:            observability.NewNoopLogger(),
		eventRec:          events,
	}
	return svc, fields, status, cardSaver, events
}

// --- tests ----------------------------------------------------------------

func TestInlineMatchAndPush_MatchedSuccess(t *testing.T) {
	imp := &fakePSAImporter{responses: [][]DHPSAImportResult{{
		{Resolution: PSAImportStatusMatched, DHCardID: 999, DHInventoryID: 1234, DHStatus: "in_stock"},
	}}}
	svc, fields, status, cardSaver, events := newInlineTestService(imp)

	p := &inventory.Purchase{ID: "p1", CertNumber: "111", CardName: "Pikachu", SetName: "Base", CardNumber: "25"}
	got := svc.inlineMatchAndPush(context.Background(), p)

	if got != 1234 {
		t.Fatalf("got=%d want=1234", got)
	}
	if p.DHInventoryID != 1234 || p.DHCardID != 999 {
		t.Fatalf("expected purchase updated with IDs, got card=%d inv=%d", p.DHCardID, p.DHInventoryID)
	}
	if len(fields.calls) != 1 || fields.calls[0].InventoryID != 1234 {
		t.Fatalf("expected fields persisted, got %+v", fields.calls)
	}
	if len(status.calls) != 1 || status.calls[0] != inventory.DHPushStatusMatched {
		t.Fatalf("expected status=matched, got %+v", status.calls)
	}
	if len(cardSaver.calls) != 1 || cardSaver.calls[0] != "999" {
		t.Fatalf("expected card-ID mapping saved, got %+v", cardSaver.calls)
	}
	if len(events.events) != 1 || events.events[0].Type != dhevents.TypePushed {
		t.Fatalf("expected one pushed event, got %+v", events.events)
	}
}

func TestInlineMatchAndPush_EmptyCertReturnsZero(t *testing.T) {
	imp := &fakePSAImporter{}
	svc, _, _, _, _ := newInlineTestService(imp)

	got := svc.inlineMatchAndPush(context.Background(), &inventory.Purchase{ID: "p1"})

	if got != 0 {
		t.Fatalf("expected 0 for empty cert, got %d", got)
	}
	if imp.calls != 0 {
		t.Fatalf("expected no psa_import call, got %d", imp.calls)
	}
}

func TestInlineMatchAndPush_RateLimitedRotatesAndRetries(t *testing.T) {
	imp := &rotatingFakeImporter{
		fakePSAImporter: &fakePSAImporter{responses: [][]DHPSAImportResult{
			{{Resolution: PSAImportStatusPSAError, RateLimited: true}},
			{{Resolution: PSAImportStatusMatched, DHCardID: 5, DHInventoryID: 55}},
		}},
		keysLeft: 3,
	}
	svc, _, _, _, _ := newInlineTestService(imp)

	got := svc.inlineMatchAndPush(context.Background(), &inventory.Purchase{ID: "p1", CertNumber: "111"})

	if got != 55 {
		t.Fatalf("expected 55 after rotation, got %d", got)
	}
	if imp.rotateCalls != 1 {
		t.Fatalf("expected one rotation, got %d", imp.rotateCalls)
	}
	if imp.calls != 2 {
		t.Fatalf("expected 2 psa_import calls, got %d", imp.calls)
	}
}

func TestInlineMatchAndPush_PSAErrorReturnsZero(t *testing.T) {
	imp := &fakePSAImporter{responses: [][]DHPSAImportResult{{
		{Resolution: PSAImportStatusPSAError, Error: "Certificate not found in PSA database"},
	}}}
	svc, fields, status, _, _ := newInlineTestService(imp)

	got := svc.inlineMatchAndPush(context.Background(), &inventory.Purchase{ID: "p1", CertNumber: "111"})

	if got != 0 {
		t.Fatalf("expected 0 on psa_error, got %d", got)
	}
	if len(fields.calls) != 0 {
		t.Fatalf("expected no fields persisted, got %+v", fields.calls)
	}
	if len(status.calls) != 0 {
		t.Fatalf("expected no status flip on psa_error, got %+v", status.calls)
	}
}

func TestInlineMatchAndPush_MissingIDsReturnsZero(t *testing.T) {
	imp := &fakePSAImporter{responses: [][]DHPSAImportResult{{
		{Resolution: PSAImportStatusMatched, DHCardID: 0, DHInventoryID: 0},
	}}}
	svc, fields, _, _, _ := newInlineTestService(imp)

	got := svc.inlineMatchAndPush(context.Background(), &inventory.Purchase{ID: "p1", CertNumber: "111"})

	if got != 0 {
		t.Fatalf("expected 0 when IDs missing, got %d", got)
	}
	if len(fields.calls) != 0 {
		t.Fatalf("expected no persistence, got %+v", fields.calls)
	}
}
