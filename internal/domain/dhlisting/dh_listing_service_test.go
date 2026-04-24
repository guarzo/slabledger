package dhlisting

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

type mockResetter struct {
	resetErr         error
	resetCalls       []string // purchaseIDs passed to ResetDHFieldsForRepush
	resetDeleteCalls []string // purchaseIDs passed to ResetDHFieldsForRepushDueToDelete
}

func (m *mockResetter) ResetDHFieldsForRepush(_ context.Context, purchaseID string) error {
	m.resetCalls = append(m.resetCalls, purchaseID)
	return m.resetErr
}

func (m *mockResetter) ResetDHFieldsForRepushDueToDelete(_ context.Context, purchaseID string) error {
	m.resetDeleteCalls = append(m.resetDeleteCalls, purchaseID)
	return m.resetErr
}

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
	updateStatusErr      error
	syncChannelsErr      error
	returnedListingPrice int
	lastListingPriceSent int
	updateCalls          int
	syncCalls            int
}

func (m *mockInventoryLister) UpdateInventoryStatus(_ context.Context, _ int, update DHInventoryStatusUpdate) (int, error) {
	m.updateCalls++
	m.lastListingPriceSent = update.ListingPriceCents
	return m.returnedListingPrice, m.updateStatusErr
}

func (m *mockInventoryLister) SyncChannels(_ context.Context, _ int, _ []string) error {
	m.syncCalls++
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

type mockUnlistedClearer struct {
	calls []string
	err   error
}

func (m *mockUnlistedClearer) ClearDHUnlistedDetectedAt(_ context.Context, purchaseID string) error {
	m.calls = append(m.calls, purchaseID)
	return m.err
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
		ID:                 "purchase-1",
		CertNumber:         certNum,
		DHInventoryID:      99,
		DHCardID:           42,
		ReviewedPriceCents: 50000, // required by listing gate
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

// TestListPurchases_ClearsUnlistedDetectedAtOnSuccess asserts that after a
// successful list (UpdateInventoryStatus + SyncChannels + persist all succeed),
// the unlisted clearer is invoked with the purchase ID to remove the
// "unlisted-on-DH" badge. A clearer error must NOT fail the list operation —
// this is a best-effort UI cleanup.
func TestListPurchases_ClearsUnlistedDetectedAtOnSuccess(t *testing.T) {
	certNum := "33334444"
	purchase := &inventory.Purchase{
		ID:                 "purchase-clear-1",
		CertNumber:         certNum,
		DHInventoryID:      101,
		DHCardID:           7,
		ReviewedPriceCents: 75000,
	}

	tests := []struct {
		name        string
		clearerErr  error
		wantListed  int
		wantSynced  int
		wantCallIDs []string
	}{
		{
			name:        "clearer invoked with purchase ID after successful list",
			clearerErr:  nil,
			wantListed:  1,
			wantSynced:  1,
			wantCallIDs: []string{"purchase-clear-1"},
		},
		{
			name:        "clearer error does not fail the listing",
			clearerErr:  errors.New("db write failed"),
			wantListed:  1,
			wantSynced:  1,
			wantCallIDs: []string{"purchase-clear-1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lookup := &mockPurchaseLookup{
				purchases: map[string]*inventory.Purchase{certNum: purchase},
			}
			lister := &mockInventoryLister{}
			fieldsUpdater := &mockFieldsUpdater{}
			clearer := &mockUnlistedClearer{err: tc.clearerErr}

			svc := newTestService(t, lookup,
				WithDHListingLister(lister),
				WithDHListingFieldsUpdater(fieldsUpdater),
				WithDHListingUnlistedClearer(clearer),
			)

			result := svc.ListPurchases(context.Background(), []string{certNum})

			if result.Listed != tc.wantListed {
				t.Errorf("Listed: got %d, want %d", result.Listed, tc.wantListed)
			}
			if result.Synced != tc.wantSynced {
				t.Errorf("Synced: got %d, want %d", result.Synced, tc.wantSynced)
			}
			if len(clearer.calls) != len(tc.wantCallIDs) {
				t.Fatalf("clearer.calls: got %d, want %d (%v)", len(clearer.calls), len(tc.wantCallIDs), clearer.calls)
			}
			for i, want := range tc.wantCallIDs {
				if clearer.calls[i] != want {
					t.Errorf("clearer.calls[%d]: got %q, want %q", i, clearer.calls[i], want)
				}
			}
		})
	}
}

// TestListPurchases covers the three key scenarios for ListPurchases in a
// single table-driven test.
func TestListPurchases(t *testing.T) {
	certNum := "55555555"
	purchase := &inventory.Purchase{
		ID:                 "purchase-1",
		CertNumber:         certNum,
		DHInventoryID:      99,
		ReviewedPriceCents: 50000, // required by listing gate
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
					"12345678": {ID: "purchase-2", CertNumber: "12345678", DHInventoryID: 42, ReviewedPriceCents: 50000},
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

// mockInventoryListerWithRotator is a lister mock that also implements
// PSAKeyRotator, so ListPurchases can type-assert and exercise the rotation
// reset and exhaustion paths.
type mockInventoryListerWithRotator struct {
	updateFn func(ctx context.Context, id int, upd DHInventoryStatusUpdate) (int, error)
	syncFn   func(ctx context.Context, id int, channels []string) error
	rotateFn func() bool
	resetFn  func()
}

func (m *mockInventoryListerWithRotator) UpdateInventoryStatus(ctx context.Context, id int, upd DHInventoryStatusUpdate) (int, error) {
	return m.updateFn(ctx, id, upd)
}

func (m *mockInventoryListerWithRotator) SyncChannels(ctx context.Context, id int, channels []string) error {
	return m.syncFn(ctx, id, channels)
}

func (m *mockInventoryListerWithRotator) RotatePSAKey() bool {
	if m.rotateFn != nil {
		return m.rotateFn()
	}
	return false
}

func (m *mockInventoryListerWithRotator) ResetPSAKeyRotation() {
	if m.resetFn != nil {
		m.resetFn()
	}
}

// TestListPurchases_ResetsPSARotationAtStart asserts that the service calls
// ResetPSAKeyRotation at the start of each ListPurchases invocation so an
// exhausted rotation index from a prior push cycle doesn't poison this call.
func TestListPurchases_ResetsPSARotationAtStart(t *testing.T) {
	resetCalls := 0
	lister := &mockInventoryListerWithRotator{
		updateFn: func(_ context.Context, _ int, upd DHInventoryStatusUpdate) (int, error) {
			return upd.ListingPriceCents, nil
		},
		syncFn:  func(context.Context, int, []string) error { return nil },
		resetFn: func() { resetCalls++ },
	}

	purchase := &inventory.Purchase{
		ID:                 "p1",
		CertNumber:         "c1",
		DHInventoryID:      5,
		DHPushStatus:       inventory.DHPushStatusMatched,
		DHStatus:           inventory.DHStatusInStock,
		ReviewedPriceCents: 1000,
		FrontImageURL:      "https://x/a.jpg",
	}
	lookup := &mockPurchaseLookup{
		purchases: map[string]*inventory.Purchase{"c1": purchase},
	}

	svc := newTestService(t, lookup, WithDHListingLister(lister))

	_ = svc.ListPurchases(context.Background(), []string{"c1"})

	if resetCalls != 1 {
		t.Errorf("ResetPSAKeyRotation calls: got %d, want 1", resetCalls)
	}
}

// TestListPurchases_PassesImagesOnStatusUpdate asserts that the primary
// listing call threads FrontImageURL/BackImageURL through to
// DHInventoryStatusUpdate so DH can skip its own PSA lookup.
func TestListPurchases_PassesImagesOnStatusUpdate(t *testing.T) {
	var capturedUpdates []DHInventoryStatusUpdate
	lister := &mockInventoryListerWithRotator{
		updateFn: func(_ context.Context, _ int, upd DHInventoryStatusUpdate) (int, error) {
			capturedUpdates = append(capturedUpdates, upd)
			return upd.ListingPriceCents, nil
		},
		syncFn: func(context.Context, int, []string) error { return nil },
	}

	purchase := &inventory.Purchase{
		ID:                 "p1",
		CertNumber:         "c1",
		DHInventoryID:      5,
		DHPushStatus:       inventory.DHPushStatusMatched,
		DHStatus:           inventory.DHStatusInStock,
		ReviewedPriceCents: 1000,
		FrontImageURL:      "https://x/front.jpg",
		BackImageURL:       "https://x/back.jpg",
	}
	lookup := &mockPurchaseLookup{
		purchases: map[string]*inventory.Purchase{"c1": purchase},
	}

	svc := newTestService(t, lookup, WithDHListingLister(lister))

	_ = svc.ListPurchases(context.Background(), []string{"c1"})

	if len(capturedUpdates) != 1 {
		t.Fatalf("expected 1 captured update, got %d", len(capturedUpdates))
	}
	if capturedUpdates[0].Status != inventory.DHStatusListed {
		t.Errorf("Status: got %q, want %q", capturedUpdates[0].Status, inventory.DHStatusListed)
	}
	if capturedUpdates[0].ListingPriceCents != 1000 {
		t.Errorf("ListingPriceCents: got %d, want 1000", capturedUpdates[0].ListingPriceCents)
	}
	if capturedUpdates[0].CertImageURLFront != "https://x/front.jpg" {
		t.Errorf("CertImageURLFront: got %q, want https://x/front.jpg", capturedUpdates[0].CertImageURLFront)
	}
	if capturedUpdates[0].CertImageURLBack != "https://x/back.jpg" {
		t.Errorf("CertImageURLBack: got %q, want https://x/back.jpg", capturedUpdates[0].CertImageURLBack)
	}
}

// TestListPurchases_PSAExhaustionRecordsEventAndSurfacesError asserts that
// when UpdateInventoryStatus returns an error wrapping ErrPSAKeysExhausted,
// the service (1) records a TypeListDeferred event with notes
// "psa_auth_exhausted", (2) short-circuits the batch, and (3) surfaces the
// wrapped sentinel on DHListingResult.Error.
func TestListPurchases_PSAExhaustionRecordsEventAndSurfacesError(t *testing.T) {
	lister := &mockInventoryListerWithRotator{
		updateFn: func(_ context.Context, _ int, _ DHInventoryStatusUpdate) (int, error) {
			return 0, fmt.Errorf("%w: underlying 401", ErrPSAKeysExhausted)
		},
		syncFn: func(context.Context, int, []string) error { return nil },
	}
	eventRec := &mockEventRecorder{}

	purchase := &inventory.Purchase{
		ID:                 "p1",
		CertNumber:         "c1",
		DHInventoryID:      5,
		DHCardID:           42,
		DHPushStatus:       inventory.DHPushStatusMatched,
		DHStatus:           inventory.DHStatusInStock,
		ReviewedPriceCents: 1000,
		// no image URLs — forces PSA fallback that then fails
	}
	lookup := &mockPurchaseLookup{
		purchases: map[string]*inventory.Purchase{"c1": purchase},
	}

	svc := newTestService(t, lookup,
		WithDHListingLister(lister),
		WithEventRecorder(eventRec),
	)

	result := svc.ListPurchases(context.Background(), []string{"c1"})

	if !errors.Is(result.Error, ErrPSAKeysExhausted) {
		t.Errorf("result.Error: got %v, want wrap of ErrPSAKeysExhausted", result.Error)
	}
	if result.Listed != 0 {
		t.Errorf("Listed: got %d, want 0", result.Listed)
	}

	deferredCount := 0
	for _, e := range eventRec.events {
		if e.Type != dhevents.TypeListDeferred {
			continue
		}
		deferredCount++
		if e.PurchaseID != "p1" {
			t.Errorf("deferred event PurchaseID: got %q, want p1", e.PurchaseID)
		}
		if e.CertNumber != "c1" {
			t.Errorf("deferred event CertNumber: got %q, want c1", e.CertNumber)
		}
		if e.Source != dhevents.SourceDHListing {
			t.Errorf("deferred event Source: got %q, want %q", e.Source, dhevents.SourceDHListing)
		}
		if !strings.Contains(e.Notes, "psa_auth_exhausted") {
			t.Errorf("deferred event Notes: got %q, want to contain psa_auth_exhausted", e.Notes)
		}
	}
	if deferredCount != 1 {
		t.Errorf("TypeListDeferred events: got %d, want 1", deferredCount)
	}
}

// TestListPurchases_StaleInventoryID covers inline reset behavior when
// UpdateInventoryStatus returns ERR_PROV_NOT_FOUND (DH item removed remotely).
func TestListPurchases_StaleInventoryID(t *testing.T) {
	notFoundErr := apperrors.ProviderNotFound("DH", "VendorInventoryItem id=522")

	tests := []struct {
		name                string
		purchase            *inventory.Purchase
		listerErr           error
		resetErr            error
		wantListed          int
		wantSkipped         int
		wantResetCalls      int
		wantResetPurchaseID string
	}{
		{
			name: "resets and skips when item missing on DH",
			purchase: &inventory.Purchase{
				ID:                 "p-stale",
				CertNumber:         "12341234",
				DHInventoryID:      522,
				DHCardID:           7,
				ReviewedPriceCents: 60000,
			},
			listerErr:           notFoundErr,
			wantListed:          0,
			wantSkipped:         1,
			wantResetCalls:      1,
			wantResetPurchaseID: "p-stale",
		},
		{
			name: "still skips when reset itself errors",
			purchase: &inventory.Purchase{
				ID:                 "p-stale-2",
				CertNumber:         "43214321",
				DHInventoryID:      522,
				ReviewedPriceCents: 60000,
			},
			listerErr:      notFoundErr,
			resetErr:       errors.New("db error"),
			wantListed:     0,
			wantSkipped:    1,
			wantResetCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lookup := &mockPurchaseLookup{
				purchases: map[string]*inventory.Purchase{tc.purchase.CertNumber: tc.purchase},
			}
			lister := &mockInventoryLister{updateStatusErr: tc.listerErr}
			resetter := &mockResetter{resetErr: tc.resetErr}

			svc := newTestService(t, lookup,
				WithDHListingLister(lister),
				WithDHListingResetter(resetter),
			)

			result := svc.ListPurchases(context.Background(), []string{tc.purchase.CertNumber})

			if result.Listed != tc.wantListed {
				t.Errorf("Listed: got %d, want %d", result.Listed, tc.wantListed)
			}
			if result.Skipped != tc.wantSkipped {
				t.Errorf("Skipped: got %d, want %d", result.Skipped, tc.wantSkipped)
			}
			if len(resetter.resetCalls) != tc.wantResetCalls {
				t.Errorf("ResetDHFieldsForRepush calls: got %d, want %d", len(resetter.resetCalls), tc.wantResetCalls)
			}
			if tc.wantResetPurchaseID != "" {
				if len(resetter.resetCalls) == 0 || resetter.resetCalls[0] != tc.wantResetPurchaseID {
					t.Errorf("ResetDHFieldsForRepush purchaseID: got %v, want %s", resetter.resetCalls, tc.wantResetPurchaseID)
				}
			}
		})
	}
}

// TestListPurchases_NoOpGuard asserts the exact-match no-op short-circuit:
// when a purchase is already DHStatusListed with DHListingPriceCents equal to
// the resolved listing price AND has a non-empty DHChannelsJSON (evidence of
// a prior successful channel sync), ListPurchases skips the PATCH, the
// SyncChannels call, the persist, and event emission. DH's
// inventory_upsert_service cancels and recreates MarketOrders on every
// request regardless of whether values changed, so unguarded re-sends churn
// eBay/Shopify state for nothing. Drift in any of status / price / channels
// falls through to the full path.
func TestListPurchases_NoOpGuard(t *testing.T) {
	const listingPrice = 34360

	tests := []struct {
		name         string
		purchase     inventory.Purchase
		wantUpdate   int // expected UpdateInventoryStatus calls
		wantSync     int // expected SyncChannels calls
		wantPersists int // expected UpdatePurchaseDHFields calls
		wantEvents   int // expected events recorded
		wantListed   int
		wantSynced   int
		wantSkipped  int
	}{
		{
			name: "already listed at target price with channels synced — skip everything",
			purchase: inventory.Purchase{
				ID:                  "purchase-noop",
				CertNumber:          "77778888",
				DHInventoryID:       938,
				DHCardID:            52340,
				DHStatus:            inventory.DHStatusListed,
				DHListingPriceCents: listingPrice,
				DHChannelsJSON:      `["ebay","shopify"]`,
				ReviewedPriceCents:  listingPrice,
			},
			wantUpdate:   0,
			wantSync:     0,
			wantPersists: 0,
			wantEvents:   0,
			wantListed:   0,
			wantSynced:   1,
			wantSkipped:  0,
		},
		{
			name: "listed but price drifted — run the full path",
			purchase: inventory.Purchase{
				ID:                  "purchase-drift",
				CertNumber:          "99990000",
				DHInventoryID:       940,
				DHCardID:            12345,
				DHStatus:            inventory.DHStatusListed,
				DHListingPriceCents: 30000, // DH is stale
				DHChannelsJSON:      `["ebay","shopify"]`,
				ReviewedPriceCents:  listingPrice,
			},
			wantUpdate:   1,
			wantSync:     1,
			wantPersists: 1,
			wantEvents:   2, // listed + channel_synced
			wantListed:   1,
			wantSynced:   1,
			wantSkipped:  0,
		},
		{
			name: "listed with matching price but empty channels — run the full path",
			purchase: inventory.Purchase{
				ID:                  "purchase-no-channels",
				CertNumber:          "11112222",
				DHInventoryID:       941,
				DHCardID:            54321,
				DHStatus:            inventory.DHStatusListed,
				DHListingPriceCents: listingPrice,
				DHChannelsJSON:      "", // no record of prior sync
				ReviewedPriceCents:  listingPrice,
			},
			wantUpdate:   1,
			wantSync:     1,
			wantPersists: 1,
			wantEvents:   2,
			wantListed:   1,
			wantSynced:   1,
			wantSkipped:  0,
		},
		{
			name: "in_stock with matching price — run the full path (transition to listed)",
			purchase: inventory.Purchase{
				ID:                  "purchase-in-stock",
				CertNumber:          "33334444",
				DHInventoryID:       942,
				DHCardID:            11111,
				DHStatus:            inventory.DHStatusInStock,
				DHListingPriceCents: listingPrice,
				DHChannelsJSON:      `["ebay","shopify"]`,
				ReviewedPriceCents:  listingPrice,
			},
			wantUpdate:   1,
			wantSync:     1,
			wantPersists: 1,
			wantEvents:   2,
			wantListed:   1,
			wantSynced:   1,
			wantSkipped:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.purchase
			lookup := &mockPurchaseLookup{
				purchases: map[string]*inventory.Purchase{p.CertNumber: &p},
			}
			lister := &mockInventoryLister{returnedListingPrice: listingPrice}
			fieldsUpdater := &mockFieldsUpdater{}
			eventRec := &mockEventRecorder{}

			svc := newTestService(t, lookup,
				WithDHListingLister(lister),
				WithDHListingFieldsUpdater(fieldsUpdater),
				WithEventRecorder(eventRec),
			)

			result := svc.ListPurchases(context.Background(), []string{p.CertNumber})

			if lister.updateCalls != tc.wantUpdate {
				t.Errorf("UpdateInventoryStatus calls: got %d, want %d", lister.updateCalls, tc.wantUpdate)
			}
			if lister.syncCalls != tc.wantSync {
				t.Errorf("SyncChannels calls: got %d, want %d", lister.syncCalls, tc.wantSync)
			}
			if len(fieldsUpdater.calls) != tc.wantPersists {
				t.Errorf("UpdatePurchaseDHFields calls: got %d, want %d", len(fieldsUpdater.calls), tc.wantPersists)
			}
			if len(eventRec.events) != tc.wantEvents {
				t.Errorf("events emitted: got %d, want %d", len(eventRec.events), tc.wantEvents)
			}
			if result.Listed != tc.wantListed {
				t.Errorf("Listed: got %d, want %d", result.Listed, tc.wantListed)
			}
			if result.Synced != tc.wantSynced {
				t.Errorf("Synced: got %d, want %d", result.Synced, tc.wantSynced)
			}
			if result.Skipped != tc.wantSkipped {
				t.Errorf("Skipped: got %d, want %d", result.Skipped, tc.wantSkipped)
			}
			if result.Total != 1 {
				t.Errorf("Total: got %d, want 1", result.Total)
			}
			// When we do patch, verify the reviewed price is what we sent.
			if tc.wantUpdate == 1 && lister.lastListingPriceSent != listingPrice {
				t.Errorf("UpdateInventoryStatus should send reviewed price %d; got %d",
					listingPrice, lister.lastListingPriceSent)
			}
		})
	}
}
