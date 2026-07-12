package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
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

// TestProcessPurchase_UnlistedDetected covers the auto-relist branch added to
// fix the stuck-row bug: after the reconciler stamps dh_unlisted_detected_at
// and the inventory poll re-discovers the cert with a new inventory ID, the
// scheduler used to short-circuit to matched and the row would sit in_stock
// forever. The relist branch only fires when a committed price is present;
// without one it falls through to the legacy "flip to matched" behavior so
// the operator can commit a price.
func TestProcessPurchase_UnlistedDetected(t *testing.T) {
	cases := []struct {
		name               string
		overridePriceCents int
		listResult         dhlisting.DHListingResult
		wantRelistCalls    int
		wantStatusCalls    int
		wantResult         processResult
	}{
		{
			name:               "committed price → invokes relister",
			overridePriceCents: 12500,
			listResult:         dhlisting.DHListingResult{Listed: 1, Total: 1},
			wantRelistCalls:    1,
			wantStatusCalls:    0, // listing service owns the transition
			wantResult:         processMatched,
		},
		{
			name:               "no committed price → falls through to matched",
			overridePriceCents: 0,
			listResult:         dhlisting.DHListingResult{}, // unused: relister not called
			wantRelistCalls:    0,
			wantStatusCalls:    1,
			wantResult:         processMatched,
		},
		{
			name:               "already fully synced → terminal matched, no retry",
			overridePriceCents: 12500,
			listResult:         dhlisting.DHListingResult{Listed: 0, Synced: 1, Total: 1},
			wantRelistCalls:    1,
			wantStatusCalls:    0,
			wantResult:         processMatched,
		},
		{
			name:               "relister no-op (skipped) → skipped, retry next cycle",
			overridePriceCents: 12500,
			listResult:         dhlisting.DHListingResult{Listed: 0, Skipped: 1, Total: 1},
			wantRelistCalls:    1,
			wantStatusCalls:    0,
			wantResult:         processSkipped,
		},
	}

	now := time.Now()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			relistCalls := 0
			relister := &mocks.MockDHListingService{
				ListPurchasesFn: func(_ context.Context, certs []string) dhlisting.DHListingResult {
					relistCalls++
					if len(certs) != 1 || certs[0] != "9d81" {
						t.Fatalf("expected single-cert relist call for cert=9d81, got %+v", certs)
					}
					return tc.listResult
				},
			}
			status := &stubStatusUpdater{}
			s := NewDHPushScheduler(
				nil, status, &stubPSAImporter{}, &mocks.MockDHFieldsUpdater{}, &stubCardIDSaver{},
				mocks.NewMockLogger(),
				DHPushConfig{Enabled: false},
				WithDHPushRelister(relister),
			)

			p := inventory.Purchase{
				ID:                   "p1",
				CertNumber:           "9d81",
				DHInventoryID:        2294,
				DHPushStatus:         inventory.DHPushStatusPending,
				DHUnlistedDetectedAt: &now,
				OverridePriceCents:   tc.overridePriceCents,
			}
			got := s.processPurchase(context.Background(), p, inventory.DefaultDHPushConfig())

			if got != tc.wantResult {
				t.Fatalf("got=%v want=%v", got, tc.wantResult)
			}
			if relistCalls != tc.wantRelistCalls {
				t.Fatalf("relist calls: got=%d want=%d", relistCalls, tc.wantRelistCalls)
			}
			if len(status.Calls) != tc.wantStatusCalls {
				t.Fatalf("status calls: got=%d want=%d (calls=%+v)", len(status.Calls), tc.wantStatusCalls, status.Calls)
			}
		})
	}
}

// failingStatusUpdater always returns an error — used to verify that setHeld
// returns processSkipped (not processHeld) when the DB write fails.
type failingStatusUpdater struct{ err error }

func (f *failingStatusUpdater) UpdatePurchaseDHPushStatus(_ context.Context, _, _ string) error {
	return f.err
}

func TestSetHeld_DBFailureReturnsSkipped(t *testing.T) {
	// No holdSetter → setHeld falls back to statusUpdater, which here always fails.
	events := &stubEventRecorder{}
	s := NewDHPushScheduler(
		nil,
		&failingStatusUpdater{err: errors.New("db down")},
		&stubPSAImporter{},
		&mocks.MockDHFieldsUpdater{},
		&stubCardIDSaver{},
		mocks.NewMockLogger(),
		DHPushConfig{Enabled: false},
		WithDHPushEventRecorder(events),
	)

	got := s.setHeld(context.Background(), inventory.Purchase{ID: "p1", CertNumber: "111"}, "over-cap")

	if got != processSkipped {
		t.Fatalf("got=%v want=processSkipped — caller must know the hold did not land", got)
	}
	if len(events.Events) != 0 {
		t.Fatalf("expected no held event when DB write fails, got %+v", events.Events)
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

// TestProcessPurchase_ListingsPausedStillMatches asserts that pausing listings
// only gates the listing transition (relister path), not matching via
// psa_import. psa_import always creates DH inventory as in_stock (never
// listed), so gating it behind ListingsPaused wrongly blocked matching when
// the operator only intended to pause listing.
func TestProcessPurchase_ListingsPausedStillMatches(t *testing.T) {
	importer := &stubPSAImporter{
		responses: []*dh.PSAImportResponse{{
			Success: true,
			Results: []dh.PSAImportResult{{
				CertNumber:    "111",
				Resolution:    dh.PSAImportStatusMatched,
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

	pushCfg := inventory.DefaultDHPushConfig()
	pushCfg.ListingsPaused = true

	p := inventory.Purchase{ID: "p1", CertNumber: "111"}
	got := s.processPurchase(context.Background(), p, pushCfg)

	if got != processMatchedComplete {
		t.Fatalf("got=%v want=processMatchedComplete — paused listings must not block psa_import matching", got)
	}
	if len(importer.requests) != 1 {
		t.Fatalf("expected psa_import to be called once, got %d calls", len(importer.requests))
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
	// DH signals "PSA key exhausted, rotate" two ways: the explicit
	// RateLimited bool (per-key burst) and a free-form Error string with a
	// daily-limit phrase (per-key daily quota, observed in prod with
	// RateLimited=false). Both must trigger rotation and a retry.
	tests := []struct {
		name        string
		firstResult dh.PSAImportResult
	}{
		{
			name:        "RateLimited flag set",
			firstResult: dh.PSAImportResult{Resolution: dh.PSAImportStatusPSAError, RateLimited: true, Error: "rate limited"},
		},
		{
			name:        "daily-limit error message without flag",
			firstResult: dh.PSAImportResult{Resolution: dh.PSAImportStatusPSAError, RateLimited: false, Error: "Daily PSA API limit reached"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			importer := &rotatingPSAImporter{
				stubPSAImporter: &stubPSAImporter{
					responses: []*dh.PSAImportResponse{
						{Success: true, Results: []dh.PSAImportResult{tc.firstResult}},
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
		})
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

// TestPushViaPSAImport_SkipEventsEmittedOnEveryPath verifies that each skip
// path in pushViaPSAImport (and applyPSAImportSuccess missing-IDs) emits a
// TypeSkipped event with the expected reason prefix.
func TestPushViaPSAImport_SkipEventsEmittedOnEveryPath(t *testing.T) {
	p := inventory.Purchase{ID: "p1", CertNumber: "111"}

	cases := []struct {
		name           string
		importer       DHPushPSAImporter
		wantNotePrefix string
	}{
		{
			name:           "api_error",
			importer:       &stubPSAImporter{errs: []error{errors.New("network timeout")}},
			wantNotePrefix: "api_error: ",
		},
		{
			name:           "batch_reject",
			importer:       &stubPSAImporter{responses: []*dh.PSAImportResponse{{Success: false, Error: "vendor profile missing"}}},
			wantNotePrefix: "batch_reject: ",
		},
		{
			name:           "empty_results",
			importer:       &stubPSAImporter{responses: []*dh.PSAImportResponse{{Success: true}}},
			wantNotePrefix: "empty_results",
		},
		{
			name: "rate_limit_exhausted",
			importer: &rotatingPSAImporter{
				stubPSAImporter: &stubPSAImporter{
					responses: []*dh.PSAImportResponse{{Success: true, Results: []dh.PSAImportResult{{RateLimited: true, Error: "rate limit"}}}},
				},
				keysLeft: 0,
			},
			wantNotePrefix: "rate_limit_exhausted: ",
		},
		{
			name: "psa_error",
			importer: &stubPSAImporter{responses: []*dh.PSAImportResponse{{
				Success: true,
				Results: []dh.PSAImportResult{{Resolution: dh.PSAImportStatusPSAError, Error: "cert not found"}},
			}}},
			wantNotePrefix: "psa_error: ",
		},
		{
			name: "partner_card_error",
			importer: &stubPSAImporter{responses: []*dh.PSAImportResponse{{
				Success: true,
				Results: []dh.PSAImportResult{{Resolution: dh.PSAImportStatusPartnerCardError, Error: "unknown language"}},
			}}},
			wantNotePrefix: "partner_card_error: ",
		},
		{
			name: "unknown_resolution",
			importer: &stubPSAImporter{responses: []*dh.PSAImportResponse{{
				Success: true,
				Results: []dh.PSAImportResult{{Resolution: "weird_new_status"}},
			}}},
			wantNotePrefix: "unknown_resolution: ",
		},
		{
			name: "success_missing_ids",
			importer: &stubPSAImporter{responses: []*dh.PSAImportResponse{{
				Success: true,
				Results: []dh.PSAImportResult{{Resolution: dh.PSAImportStatusMatched, DHCardID: 0, DHInventoryID: 0}},
			}}},
			wantNotePrefix: "success_missing_ids: ",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			events := &stubEventRecorder{}
			s := newTestDHPushScheduler(tc.importer, &mocks.MockDHFieldsUpdater{}, &stubStatusUpdater{}, &stubCardIDSaver{}, events)

			got := s.pushViaPSAImport(context.Background(), p)

			if got != processSkipped {
				t.Fatalf("got=%v want=processSkipped", got)
			}
			if len(events.Events) != 1 {
				t.Fatalf("expected 1 skip event, got %d: %+v", len(events.Events), events.Events)
			}
			evt := events.Events[0]
			if evt.Type != dhevents.TypeSkipped {
				t.Fatalf("event type: got=%q want=%q", evt.Type, dhevents.TypeSkipped)
			}
			if evt.Source != dhevents.SourceDHPush {
				t.Fatalf("event source: got=%q want=%q", evt.Source, dhevents.SourceDHPush)
			}
			if evt.PurchaseID != p.ID {
				t.Fatalf("event purchaseID: got=%q want=%q", evt.PurchaseID, p.ID)
			}
			if len(evt.Notes) < len(tc.wantNotePrefix) || evt.Notes[:len(tc.wantNotePrefix)] != tc.wantNotePrefix {
				t.Fatalf("event notes: got=%q, want prefix %q", evt.Notes, tc.wantNotePrefix)
			}
		})
	}
}

// TestPushViaPSAImport_KeyRotationCapEmitsSkipEvent verifies the
// key_rotation_cap path (loop exhausted by repeated rate-limited responses).
func TestPushViaPSAImport_KeyRotationCapEmitsSkipEvent(t *testing.T) {
	// Each call returns rate-limited; rotator has 8 keys so the loop hits the cap.
	rateLimitedResp := &dh.PSAImportResponse{
		Success: true,
		Results: []dh.PSAImportResult{{RateLimited: true, Error: "rate limit"}},
	}
	responses := make([]*dh.PSAImportResponse, psaImportMaxAttempts)
	for i := range responses {
		responses[i] = rateLimitedResp
	}
	importer := &rotatingPSAImporter{
		stubPSAImporter: &stubPSAImporter{responses: responses},
		keysLeft:        psaImportMaxAttempts,
	}
	events := &stubEventRecorder{}
	s := newTestDHPushScheduler(importer, &mocks.MockDHFieldsUpdater{}, &stubStatusUpdater{}, &stubCardIDSaver{}, events)

	got := s.pushViaPSAImport(context.Background(), inventory.Purchase{ID: "p1", CertNumber: "111"})

	if got != processSkipped {
		t.Fatalf("got=%v want=processSkipped", got)
	}
	if len(events.Events) != 1 {
		t.Fatalf("expected 1 skip event, got %d", len(events.Events))
	}
	if events.Events[0].Notes != "key_rotation_cap" {
		t.Fatalf("expected notes=key_rotation_cap, got %q", events.Events[0].Notes)
	}
}
