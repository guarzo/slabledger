package scheduler

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// --- test doubles -----------------------------------------------------------

// stubPSAImporter is a scheduler.DHPushPSAImporter double that returns canned
// responses from a FIFO queue. Also optionally implements dh.PSAKeyRotator for
// rotation-path tests (RotateFn + RotateCalls).
type stubPSAImporter struct {
	responses []*dh.PSAImportResponse
	errs      []error
	requests  [][]dh.PSAImportItem

	// Optional rotator hooks — nil means the importer doesn't satisfy
	// dh.PSAKeyRotator (which the scheduler tolerates).
	RotateFn          func() bool
	RotateCalls       int
	ResetRotationFn   func()
	ResetRotationCall int
}

func (s *stubPSAImporter) PSAImport(_ context.Context, items []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
	s.requests = append(s.requests, items)
	if len(s.errs) > 0 {
		e := s.errs[0]
		s.errs = s.errs[1:]
		if e != nil {
			return nil, e
		}
	}
	if len(s.responses) == 0 {
		return &dh.PSAImportResponse{}, nil
	}
	r := s.responses[0]
	s.responses = s.responses[1:]
	return r, nil
}

// rotatingPSAImporter wraps stubPSAImporter and implements dh.PSAKeyRotator.
// Separate type (rather than methods on stubPSAImporter) so tests that don't
// want the rotator-path don't accidentally satisfy the type assertion.
type rotatingPSAImporter struct {
	*stubPSAImporter
	keysLeft int // how many times RotatePSAKey() returns true before returning false
	reset    int
}

func (r *rotatingPSAImporter) RotatePSAKey() bool {
	r.RotateCalls++
	if r.keysLeft > 0 {
		r.keysLeft--
		return true
	}
	return false
}

func (r *rotatingPSAImporter) ResetPSAKeyRotation() { r.reset++ }

type statusCall struct {
	ID     string
	Status string
}

type stubStatusUpdater struct {
	Calls []statusCall
}

func (s *stubStatusUpdater) UpdatePurchaseDHPushStatus(_ context.Context, id, status string) error {
	s.Calls = append(s.Calls, statusCall{id, status})
	return nil
}

type stubCardIDSaver struct {
	Calls []string
}

func (s *stubCardIDSaver) SaveExternalID(_ context.Context, _, _, _, provider, externalID string) error {
	s.Calls = append(s.Calls, provider+":"+externalID)
	return nil
}

type stubEventRecorder struct {
	Events []dhevents.Event
}

func (s *stubEventRecorder) Record(_ context.Context, e dhevents.Event) error {
	s.Events = append(s.Events, e)
	return nil
}

// --- helper -----------------------------------------------------------------

func newTestDHPushScheduler(importer DHPushPSAImporter, fields *mocks.MockDHFieldsUpdater, status *stubStatusUpdater, mapper *stubCardIDSaver, events *stubEventRecorder) *DHPushScheduler {
	var opts []DHPushOption
	if events != nil {
		opts = append(opts, WithDHPushEventRecorder(events))
	}
	return NewDHPushScheduler(
		nil, status, importer, fields, mapper,
		mocks.NewMockLogger(),
		DHPushConfig{Enabled: false},
		opts...,
	)
}

// --- tests ------------------------------------------------------------------

func TestProcessPurchase_AlreadyPushedFixesStatus(t *testing.T) {
	status := &stubStatusUpdater{}
	s := newTestDHPushScheduler(&stubPSAImporter{}, &mocks.MockDHFieldsUpdater{}, status, &stubCardIDSaver{}, nil)

	p := inventory.Purchase{ID: "p1", CertNumber: "111", DHInventoryID: 42}
	got := s.processPurchase(context.Background(), p, inventory.DefaultDHPushConfig())

	if got != processMatched {
		t.Fatalf("got=%v want=processMatched", got)
	}
	if len(status.Calls) != 1 || status.Calls[0].Status != inventory.DHPushStatusMatched {
		t.Fatalf("expected one status update to matched, got %+v", status.Calls)
	}
}

func TestProcessPurchase_NoCertNumberMarksUnmatched(t *testing.T) {
	status := &stubStatusUpdater{}
	events := &stubEventRecorder{}
	s := newTestDHPushScheduler(&stubPSAImporter{}, &mocks.MockDHFieldsUpdater{}, status, &stubCardIDSaver{}, events)

	p := inventory.Purchase{ID: "p1", CertNumber: ""}
	got := s.processPurchase(context.Background(), p, inventory.DefaultDHPushConfig())

	if got != processUnmatched {
		t.Fatalf("got=%v want=processUnmatched", got)
	}
	if len(status.Calls) != 1 || status.Calls[0].Status != inventory.DHPushStatusUnmatched {
		t.Fatalf("expected one status update to unmatched, got %+v", status.Calls)
	}
	if len(events.Events) != 1 || events.Events[0].Type != dhevents.TypeUnmatched {
		t.Fatalf("expected one unmatched event, got %+v", events.Events)
	}
}

func TestPushViaPSAImport_SuccessPathsPersist(t *testing.T) {
	successes := []string{
		dh.PSAImportStatusMatched,
		dh.PSAImportStatusUnmatchedCreated,
		dh.PSAImportStatusOverrideCorrected,
		dh.PSAImportStatusAlreadyListed,
	}
	for _, resolution := range successes {
		t.Run(resolution, func(t *testing.T) {
			importer := &stubPSAImporter{
				responses: []*dh.PSAImportResponse{{
					Success: true,
					Results: []dh.PSAImportResult{{
						CertNumber:    "111",
						Resolution:    resolution,
						DHCardID:      999,
						DHInventoryID: 1234,
						Status:        dh.InventoryStatusInStock,
					}},
				}},
			}
			fields := &mocks.MockDHFieldsUpdater{}
			status := &stubStatusUpdater{}
			mapper := &stubCardIDSaver{}
			s := newTestDHPushScheduler(importer, fields, status, mapper, nil)

			p := inventory.Purchase{ID: "p1", CertNumber: "111"}
			got := s.pushViaPSAImport(context.Background(), p)

			if got != processMatchedComplete {
				t.Fatalf("got=%v want=processMatchedComplete", got)
			}
			if len(fields.Calls) != 1 || fields.Calls[0].InventoryID != 1234 {
				t.Fatalf("expected fields persisted with inventory ID 1234, got %+v", fields.Calls)
			}
			if len(status.Calls) != 1 || status.Calls[0].Status != inventory.DHPushStatusMatched {
				t.Fatalf("expected status=matched, got %+v", status.Calls)
			}
			if len(mapper.Calls) != 1 || mapper.Calls[0] != "doubleholo:999" {
				t.Fatalf("expected card-ID mapping saved, got %+v", mapper.Calls)
			}
		})
	}
}

func TestPushViaPSAImport_RateLimitedRotatesAndRetries(t *testing.T) {
	importer := &rotatingPSAImporter{
		stubPSAImporter: &stubPSAImporter{
			responses: []*dh.PSAImportResponse{
				{Success: true, Results: []dh.PSAImportResult{{Resolution: dh.PSAImportStatusPSAError, RateLimited: true, Error: "rate limited"}}},
				{Success: true, Results: []dh.PSAImportResult{{Resolution: dh.PSAImportStatusMatched, DHCardID: 5, DHInventoryID: 55}}},
			},
		},
		keysLeft: 3,
	}
	fields := &mocks.MockDHFieldsUpdater{}
	status := &stubStatusUpdater{}
	s := newTestDHPushScheduler(importer, fields, status, &stubCardIDSaver{}, nil)

	got := s.pushViaPSAImport(context.Background(), inventory.Purchase{ID: "p1", CertNumber: "111"})

	if got != processMatchedComplete {
		t.Fatalf("got=%v want=processMatchedComplete", got)
	}
	if importer.RotateCalls != 1 {
		t.Fatalf("expected one key rotation, got %d", importer.RotateCalls)
	}
	if len(importer.requests) != 2 {
		t.Fatalf("expected 2 psa_import calls, got %d", len(importer.requests))
	}
}

func TestPushViaPSAImport_RateLimitedExhaustedLeavesPending(t *testing.T) {
	importer := &rotatingPSAImporter{
		stubPSAImporter: &stubPSAImporter{
			responses: []*dh.PSAImportResponse{
				{Success: true, Results: []dh.PSAImportResult{{Resolution: dh.PSAImportStatusPSAError, RateLimited: true, Error: "rate limited"}}},
			},
		},
		keysLeft: 0, // no keys to rotate to
	}
	fields := &mocks.MockDHFieldsUpdater{}
	status := &stubStatusUpdater{}
	s := newTestDHPushScheduler(importer, fields, status, &stubCardIDSaver{}, nil)

	got := s.pushViaPSAImport(context.Background(), inventory.Purchase{ID: "p1", CertNumber: "111"})

	if got != processSkipped {
		t.Fatalf("got=%v want=processSkipped (should stay pending when keys exhausted)", got)
	}
	if len(status.Calls) != 0 {
		t.Fatalf("expected no status update on rate-limit exhaustion, got %+v", status.Calls)
	}
	if len(fields.Calls) != 0 {
		t.Fatalf("expected no fields update on failure, got %+v", fields.Calls)
	}
}

func TestPushViaPSAImport_PSAErrorLeavesPending(t *testing.T) {
	importer := &stubPSAImporter{
		responses: []*dh.PSAImportResponse{{
			Success: true,
			Results: []dh.PSAImportResult{{Resolution: dh.PSAImportStatusPSAError, Error: "Certificate not found in PSA database"}},
		}},
	}
	fields := &mocks.MockDHFieldsUpdater{}
	status := &stubStatusUpdater{}
	s := newTestDHPushScheduler(importer, fields, status, &stubCardIDSaver{}, nil)

	got := s.pushViaPSAImport(context.Background(), inventory.Purchase{ID: "p1", CertNumber: "111"})

	if got != processSkipped {
		t.Fatalf("got=%v want=processSkipped (psa_error without rate-limit stays pending for later review)", got)
	}
	if len(status.Calls) != 0 {
		t.Fatalf("expected no status update on psa_error, got %+v", status.Calls)
	}
}

func TestPushViaPSAImport_PartnerCardErrorLeavesPending(t *testing.T) {
	importer := &stubPSAImporter{
		responses: []*dh.PSAImportResponse{{
			Success: true,
			Results: []dh.PSAImportResult{{Resolution: dh.PSAImportStatusPartnerCardError, Error: "Unknown language override 'klingon'"}},
		}},
	}
	fields := &mocks.MockDHFieldsUpdater{}
	status := &stubStatusUpdater{}
	s := newTestDHPushScheduler(importer, fields, status, &stubCardIDSaver{}, nil)

	got := s.pushViaPSAImport(context.Background(), inventory.Purchase{ID: "p1", CertNumber: "111"})

	if got != processSkipped {
		t.Fatalf("got=%v want=processSkipped", got)
	}
	if len(status.Calls) != 0 {
		t.Fatalf("expected no status update on partner_card_error, got %+v", status.Calls)
	}
}

func TestPushViaPSAImport_BatchLevelFailureLeavesPending(t *testing.T) {
	importer := &stubPSAImporter{
		responses: []*dh.PSAImportResponse{{Success: false, Error: "vendor profile missing"}},
	}
	fields := &mocks.MockDHFieldsUpdater{}
	status := &stubStatusUpdater{}
	s := newTestDHPushScheduler(importer, fields, status, &stubCardIDSaver{}, nil)

	got := s.pushViaPSAImport(context.Background(), inventory.Purchase{ID: "p1", CertNumber: "111"})

	if got != processSkipped {
		t.Fatalf("got=%v want=processSkipped", got)
	}
}

func TestPushViaPSAImport_SuccessMissingIDsSkips(t *testing.T) {
	importer := &stubPSAImporter{
		responses: []*dh.PSAImportResponse{{
			Success: true,
			Results: []dh.PSAImportResult{{Resolution: dh.PSAImportStatusMatched, DHCardID: 0, DHInventoryID: 0}},
		}},
	}
	fields := &mocks.MockDHFieldsUpdater{}
	status := &stubStatusUpdater{}
	s := newTestDHPushScheduler(importer, fields, status, &stubCardIDSaver{}, nil)

	got := s.pushViaPSAImport(context.Background(), inventory.Purchase{ID: "p1", CertNumber: "111"})

	if got != processSkipped {
		t.Fatalf("got=%v want=processSkipped", got)
	}
	if len(fields.Calls) != 0 {
		t.Fatalf("expected no persistence when IDs are missing, got %+v", fields.Calls)
	}
}
