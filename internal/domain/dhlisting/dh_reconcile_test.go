package dhlisting_test

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubSnapshotFetcher struct {
	ids map[int]struct{}
	err error
}

func (s stubSnapshotFetcher) FetchAllInventoryIDs(_ context.Context) (map[int]struct{}, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.ids, nil
}

func idSet(ids ...int) map[int]struct{} {
	m := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}

func TestReconcile(t *testing.T) {
	tests := []struct {
		name            string
		snapshot        map[int]struct{}
		snapshotErr     error
		purchases       []inventory.Purchase
		resetErrForIDs  map[string]error
		wantErr         bool
		wantScanned     int
		wantMissingOnDH int
		wantReset       int
		wantResetIDs    []string
		wantErrorCount  int
	}{
		{
			name:     "all local IDs present on DH resets nothing",
			snapshot: idSet(101, 102, 103),
			purchases: []inventory.Purchase{
				{ID: "p1", DHInventoryID: 101},
				{ID: "p2", DHInventoryID: 102},
				{ID: "p3", DHInventoryID: 103},
			},
			wantScanned:     3,
			wantMissingOnDH: 0,
			wantReset:       0,
		},
		{
			name:     "only missing IDs get reset; unlinked purchases skipped from scan",
			snapshot: idSet(101, 103),
			purchases: []inventory.Purchase{
				{ID: "p1", DHInventoryID: 101}, // present on DH — leave alone
				{ID: "p2", DHInventoryID: 200}, // ghost — reset
				{ID: "p3", DHInventoryID: 103}, // present on DH
				{ID: "p4", DHInventoryID: 201}, // ghost — reset
				{ID: "p5", DHInventoryID: 0},   // not linked — skip scan
			},
			wantScanned:     4,
			wantMissingOnDH: 2,
			wantReset:       2,
			wantResetIDs:    []string{"p2", "p4"},
		},
		{
			name:        "snapshot error aborts run with zero resets",
			snapshotErr: errors.New("DH 500"),
			purchases: []inventory.Purchase{
				{ID: "p1", DHInventoryID: 200}, // would be a ghost — but we never get here
			},
			wantErr: true,
		},
		{
			name:     "per-item reset error is reported but does not abort",
			snapshot: idSet(101),
			purchases: []inventory.Purchase{
				{ID: "p1", DHInventoryID: 200},
				{ID: "p2", DHInventoryID: 201},
			},
			resetErrForIDs:  map[string]error{"p1": errors.New("db locked")},
			wantScanned:     2,
			wantMissingOnDH: 2,
			wantReset:       1,
			wantResetIDs:    []string{"p2"},
			wantErrorCount:  1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			purchases := tc.purchases // capture
			var plainResetCalls []string
			repo := &mocks.PurchaseRepositoryMock{
				ListAllUnsoldPurchasesFn: func(_ context.Context) ([]inventory.Purchase, error) {
					return purchases, nil
				},
				ResetDHFieldsForRepushFn: func(_ context.Context, id string) error {
					plainResetCalls = append(plainResetCalls, id)
					return nil
				},
				ResetDHFieldsForRepushDueToDeleteFn: func(_ context.Context, id string) error {
					if err, ok := tc.resetErrForIDs[id]; ok {
						return err
					}
					return nil
				},
			}
			recon, err := dhlisting.NewReconciler(
				stubSnapshotFetcher{ids: tc.snapshot, err: tc.snapshotErr},
				repo,
				repo,
				mocks.NewMockLogger(),
			)
			if err != nil {
				t.Fatalf("NewReconciler: %v", err)
			}

			got, err := recon.Reconcile(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if got.Reset != 0 {
					t.Errorf("expected zero resets on error, got %d", got.Reset)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.Scanned != tc.wantScanned {
				t.Errorf("Scanned = %d, want %d", got.Scanned, tc.wantScanned)
			}
			if got.MissingOnDH != tc.wantMissingOnDH {
				t.Errorf("MissingOnDH = %d, want %d", got.MissingOnDH, tc.wantMissingOnDH)
			}
			if got.Reset != tc.wantReset {
				t.Errorf("Reset = %d, want %d", got.Reset, tc.wantReset)
			}
			if len(got.Errors) != tc.wantErrorCount {
				t.Errorf("Errors = %v, want %d entries", got.Errors, tc.wantErrorCount)
			}
			if len(got.ResetIDs) != len(tc.wantResetIDs) {
				t.Fatalf("ResetIDs = %v, want %v", got.ResetIDs, tc.wantResetIDs)
			}
			for i, id := range tc.wantResetIDs {
				if got.ResetIDs[i] != id {
					t.Errorf("ResetIDs[%d] = %q, want %q", i, got.ResetIDs[i], id)
				}
			}
			// Reconciler must use the delete-aware variant so rows get stamped
			// with dh_unlisted_detected_at, never the plain reset.
			if len(plainResetCalls) != 0 {
				t.Errorf("plain ResetDHFieldsForRepush called %d times (%v); reconciler must use ResetDHFieldsForRepushDueToDelete", len(plainResetCalls), plainResetCalls)
			}
		})
	}
}

// TestReconcile_EmitsUnlistedEvent verifies that each successful reset records
// a dh_state_events row so operators can confirm the reconciler ran and which
// purchases it touched. Without this event, reconciler runs are invisible
// outside the log stream.
func TestReconcile_EmitsUnlistedEvent(t *testing.T) {
	cases := []struct {
		name       string
		snapshot   map[int]struct{}
		purchases  []inventory.Purchase
		recordErr  error // when set, MockEventRecorder.Record returns this
		wantEvents []dhevents.Event
		wantReset  int // reset count should be unaffected by recorder failures
	}{
		{
			name:     "records event per reset, nothing for healthy or unlinked rows",
			snapshot: idSet(101),
			purchases: []inventory.Purchase{
				{ID: "p1", CertNumber: "c1", DHInventoryID: 101, DHCardID: 55, DHStatus: inventory.DHStatusListed},
				{ID: "p2", CertNumber: "c2", DHInventoryID: 200, DHCardID: 77, DHStatus: inventory.DHStatusListed},
				{ID: "p3", CertNumber: "c3", DHInventoryID: 0},
			},
			wantEvents: []dhevents.Event{{
				PurchaseID:    "p2",
				CertNumber:    "c2",
				Type:          dhevents.TypeUnlisted,
				PrevDHStatus:  string(inventory.DHStatusListed),
				DHInventoryID: 200,
				DHCardID:      77,
				Source:        dhevents.SourceDHReconcile,
				Notes:         "inventory ID missing from DH snapshot",
			}},
			wantReset: 1,
		},
		{
			name:      "no events when nothing drifts",
			snapshot:  idSet(101, 102),
			purchases: []inventory.Purchase{{ID: "p1", DHInventoryID: 101}, {ID: "p2", DHInventoryID: 102}},
		},
		{
			name:     "recorder error is swallowed; reset still counted",
			snapshot: idSet(101),
			purchases: []inventory.Purchase{
				{ID: "p2", CertNumber: "c2", DHInventoryID: 200, DHCardID: 77, DHStatus: inventory.DHStatusListed},
			},
			recordErr: errors.New("db locked"),
			wantEvents: []dhevents.Event{{
				PurchaseID:    "p2",
				CertNumber:    "c2",
				Type:          dhevents.TypeUnlisted,
				PrevDHStatus:  string(inventory.DHStatusListed),
				DHInventoryID: 200,
				DHCardID:      77,
				Source:        dhevents.SourceDHReconcile,
				Notes:         "inventory ID missing from DH snapshot",
			}},
			wantReset: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mocks.PurchaseRepositoryMock{
				ListAllUnsoldPurchasesFn: func(_ context.Context) ([]inventory.Purchase, error) {
					return tc.purchases, nil
				},
			}
			rec := &mocks.MockEventRecorder{
				RecordFn: func(_ context.Context, _ dhevents.Event) error { return tc.recordErr },
			}
			recon, err := dhlisting.NewReconciler(
				stubSnapshotFetcher{ids: tc.snapshot},
				repo, repo,
				mocks.NewMockLogger(),
				dhlisting.WithReconcileEventRecorder(rec),
			)
			require.NoError(t, err)

			result, err := recon.Reconcile(context.Background())
			require.NoError(t, err, "recorder errors must be swallowed; Reconcile must not propagate them")
			assert.Equal(t, tc.wantEvents, rec.Events)
			assert.Equal(t, tc.wantReset, result.Reset, "recorder failure must not affect reset count")
		})
	}
}

func TestNewReconciler_rejectsMissingDeps(t *testing.T) {
	repo := &mocks.PurchaseRepositoryMock{}
	fetcher := stubSnapshotFetcher{}
	logger := mocks.NewMockLogger()

	tests := []struct {
		name    string
		fetcher dhlisting.DHInventorySnapshotFetcher
		lister  dhlisting.DHReconcilePurchaseLister
		reset   dhlisting.DHReconcileResetter
	}{
		{"no fetcher", nil, repo, repo},
		{"no lister", fetcher, nil, repo},
		{"no resetter", fetcher, repo, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := dhlisting.NewReconciler(tc.fetcher, tc.lister, tc.reset, logger); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	if _, err := dhlisting.NewReconciler(fetcher, repo, repo, nil); err == nil {
		t.Fatal("expected error when logger is nil")
	}
}
