package dhlisting

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
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

type mockEventRecorder struct {
	events []dhevents.Event
	err    error
}

func (m *mockEventRecorder) Record(_ context.Context, e dhevents.Event) error {
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, e)
	return nil
}

func newTestService(t *testing.T, lookup DHListingPurchaseLookup, opts ...DHListingServiceOption) Service {
	t.Helper()
	svc, err := NewDHListingService(lookup, observability.NewNoopLogger(), opts...)
	if err != nil {
		t.Fatalf("NewDHListingService: %v", err)
	}
	return svc
}

// TestListPurchases_RecordsListedAndChannelSyncedEvents asserts that TypeListed
// and TypeChannelSynced events are emitted on successful listing.
func TestListPurchases_RecordsListedAndChannelSyncedEvents(t *testing.T) {
	certNum := "55555555"
	purchase := &inventory.Purchase{
		ID:            "purchase-1",
		CertNumber:    certNum,
		DHInventoryID: 99,
		DHCardID:      42,
	}

	lookup := &mockPurchaseLookup{
		purchases: map[string]*inventory.Purchase{certNum: purchase},
	}
	lister := &mockInventoryLister{}
	fieldsUpdater := &mockFieldsUpdater{}
	eventRec := &mockEventRecorder{}

	svc := newTestService(t, lookup,
		WithDHListingLister(lister),
		WithDHListingFieldsUpdater(fieldsUpdater),
		WithEventRecorder(eventRec),
	)

	result := svc.ListPurchases(context.Background(), []string{certNum})

	if result.Listed != 1 {
		t.Errorf("Listed: got %d, want 1", result.Listed)
	}
	if result.Synced != 1 {
		t.Errorf("Synced: got %d, want 1", result.Synced)
	}

	// Check that listed and channel_synced events were recorded
	if len(eventRec.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(eventRec.events))
	}

	listedEvent := eventRec.events[0]
	if listedEvent.Type != dhevents.TypeListed {
		t.Errorf("first event: expected type %s, got %s", dhevents.TypeListed, listedEvent.Type)
	}
	if listedEvent.PurchaseID != "purchase-1" {
		t.Errorf("first event: expected purchaseID purchase-1, got %s", listedEvent.PurchaseID)
	}
	if listedEvent.CertNumber != certNum {
		t.Errorf("first event: expected cert %s, got %s", certNum, listedEvent.CertNumber)
	}
	if listedEvent.DHInventoryID != 99 {
		t.Errorf("first event: expected inventoryID 99, got %d", listedEvent.DHInventoryID)
	}
	if listedEvent.DHCardID != 42 {
		t.Errorf("first event: expected cardID 42, got %d", listedEvent.DHCardID)
	}
	if listedEvent.Source != dhevents.SourceDHListing {
		t.Errorf("first event: expected source %s, got %s", dhevents.SourceDHListing, listedEvent.Source)
	}

	syncedEvent := eventRec.events[1]
	if syncedEvent.Type != dhevents.TypeChannelSynced {
		t.Errorf("second event: expected type %s, got %s", dhevents.TypeChannelSynced, syncedEvent.Type)
	}
	if syncedEvent.PurchaseID != "purchase-1" {
		t.Errorf("second event: expected purchaseID purchase-1, got %s", syncedEvent.PurchaseID)
	}
	if syncedEvent.Source != dhevents.SourceDHListing {
		t.Errorf("second event: expected source %s, got %s", dhevents.SourceDHListing, syncedEvent.Source)
	}
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

// TestListPurchases_EmptyInput asserts that an empty cert-number slice
// returns a zero result and does not panic.
func TestListPurchases_EmptyInput(t *testing.T) {
	lookup := &mockPurchaseLookup{
		purchases: map[string]*inventory.Purchase{},
	}
	svc := newTestService(t, lookup, WithDHListingLister(&mockInventoryLister{}))

	result := svc.ListPurchases(context.Background(), []string{})

	if result.Listed != 0 || result.Synced != 0 || result.Total != 0 {
		t.Errorf("expected zero result for empty input, got %+v", result)
	}
	if result.Error != nil {
		t.Errorf("expected no error for empty input, got %v", result.Error)
	}
}

// TestListPurchases_UnenrolledPurchaseIsSkippedNotProcessed asserts that a
// purchase with no DH inventory ID and an empty dh_push_status is skipped
// rather than silently treated as "listed". Regression guard for the bug that
// stranded cert intake → DH sync.
func TestListPurchases_UnenrolledPurchaseIsSkippedNotProcessed(t *testing.T) {
	certNum := "77777777"
	unenrolled := &inventory.Purchase{
		ID:            "p-unenrolled",
		CertNumber:    certNum,
		DHInventoryID: 0,
		DHPushStatus:  "",
	}
	lookup := &mockPurchaseLookup{
		purchases: map[string]*inventory.Purchase{certNum: unenrolled},
	}
	lister := &mockInventoryLister{}
	updater := &mockFieldsUpdater{}

	svc := newTestService(t, lookup,
		WithDHListingLister(lister),
		WithDHListingFieldsUpdater(updater),
	)
	result := svc.ListPurchases(context.Background(), []string{certNum})

	if result.Listed != 0 {
		t.Errorf("Listed: got %d, want 0 (unenrolled must not be listed)", result.Listed)
	}
	if result.Synced != 0 {
		t.Errorf("Synced: got %d, want 0", result.Synced)
	}
	if result.Total != 1 {
		t.Errorf("Total: got %d, want 1", result.Total)
	}
	if len(updater.calls) != 0 {
		t.Errorf("UpdatePurchaseDHFields called %d times, want 0 (unenrolled row should not be touched)", len(updater.calls))
	}
}

// TestListPurchases_SkippedCounterIncrements asserts that purchases hitting
// skip paths increment the Skipped counter in the result.
func TestListPurchases_SkippedCounterIncrements(t *testing.T) {
	certNum := "88888888"
	// Unenrolled purchase: DHInventoryID == 0 && DHPushStatus != pending
	unenrolled := &inventory.Purchase{
		ID:            "p-unenrolled",
		CertNumber:    certNum,
		DHInventoryID: 0,
		DHPushStatus:  "", // not pending
	}
	lookup := &mockPurchaseLookup{
		purchases: map[string]*inventory.Purchase{certNum: unenrolled},
	}
	lister := &mockInventoryLister{}

	svc := newTestService(t, lookup, WithDHListingLister(lister))
	result := svc.ListPurchases(context.Background(), []string{certNum})

	if result.Skipped != 1 {
		t.Errorf("Skipped: got %d, want 1", result.Skipped)
	}
	if result.Total != 1 {
		t.Errorf("Total: got %d, want 1", result.Total)
	}
}

// TestDisambiguateCandidates covers the package-level disambiguateCandidates
// function (white-box test; requires package dhlisting).
func TestDisambiguateCandidates(t *testing.T) {
	tests := []struct {
		name           string
		candidates     []DHCertCandidate
		cardNumber     string
		saveFnErr      error
		wantID         int
		wantSaveCalled bool
		wantErr        bool
	}{
		{
			name:           "empty candidates — saveFn called with empty JSON",
			candidates:     []DHCertCandidate{},
			cardNumber:     "001",
			wantID:         0,
			wantSaveCalled: true,
		},
		{
			name: "single candidate matching card number",
			candidates: []DHCertCandidate{
				{DHCardID: 42, CardNumber: "001"},
			},
			cardNumber:     "001",
			wantID:         42,
			wantSaveCalled: false,
		},
		{
			name: "single candidate non-matching card number — saveFn called",
			candidates: []DHCertCandidate{
				{DHCardID: 99, CardNumber: "002"},
			},
			cardNumber:     "001",
			wantID:         0,
			wantSaveCalled: true,
		},
		{
			name: "multiple candidates — exact card number match returns ID",
			candidates: []DHCertCandidate{
				{DHCardID: 10, CardNumber: "001"},
				{DHCardID: 20, CardNumber: "002"},
			},
			cardNumber:     "001",
			wantID:         10,
			wantSaveCalled: false,
		},
		{
			name: "multiple candidates — no card number match calls saveFn",
			candidates: []DHCertCandidate{
				{DHCardID: 10, CardNumber: "003"},
				{DHCardID: 20, CardNumber: "004"},
			},
			cardNumber:     "001",
			wantID:         0,
			wantSaveCalled: true,
		},
		{
			name: "multiple candidates — ambiguous (two match same number) calls saveFn",
			candidates: []DHCertCandidate{
				{DHCardID: 10, CardNumber: "001"},
				{DHCardID: 20, CardNumber: "001"},
			},
			cardNumber:     "001",
			wantID:         0,
			wantSaveCalled: true,
		},
		{
			name: "saveFn error is propagated",
			candidates: []DHCertCandidate{
				{DHCardID: 5, CardNumber: "999"},
			},
			cardNumber:     "001",
			saveFnErr:      errors.New("save failed"),
			wantID:         0,
			wantSaveCalled: true,
			wantErr:        true,
		},
		{
			name:           "nil saveFn — no panic on unresolvable candidates",
			candidates:     []DHCertCandidate{{DHCardID: 7, CardNumber: "002"}},
			cardNumber:     "001",
			wantID:         0,
			wantSaveCalled: false, // saveFn is nil — no call
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			saveCalled := false
			var saveFn func(string) error
			if tc.name != "nil saveFn — no panic on unresolvable candidates" {
				err := tc.saveFnErr
				saveFn = func(_ string) error {
					saveCalled = true
					return err
				}
			}

			id, err := disambiguateCandidates(tc.candidates, tc.cardNumber, saveFn)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if id != tc.wantID {
				t.Errorf("expected ID %d, got %d", tc.wantID, id)
			}
			if tc.wantSaveCalled && !saveCalled {
				t.Error("expected saveFn to be called, but it was not")
			}
			if !tc.wantSaveCalled && saveCalled {
				t.Error("expected saveFn not to be called, but it was")
			}
		})
	}
}
