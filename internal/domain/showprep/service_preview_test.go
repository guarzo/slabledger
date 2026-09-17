package showprep_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

const previewID = "11111111-1111-4111-8111-111111111111"

func previewReadOnlyStore(t *testing.T) (*mocks.ShowPrepStoreMock, *mocks.ShowPrepSourceMock) {
	t.Helper()
	forbidden := func() { t.Helper(); t.Fatal("preview attempted acquisition, hold observation, or a list write") }
	return &mocks.ShowPrepStoreMock{
			ObservePriceAssociationsFn: func(context.Context, []string) (map[string]bool, error) { forbidden(); return nil, nil },
			BeginAttemptFn:             func(context.Context, sp.Identity, time.Time) (int64, error) { forbidden(); return 0, nil },
			FinishAttemptFn:            func(context.Context, sp.Identity, int64, sp.Snapshot) error { forbidden(); return nil },
			WithinFn:                   func(context.Context, func(sp.Session) error) error { forbidden(); return nil },
			ListsFn:                    func(context.Context) ([]sp.List, error) { forbidden(); return nil, nil },
			CreateListFn:               func(context.Context, string, string) (sp.List, error) { forbidden(); return sp.List{}, nil },
			RenameListFn:               func(context.Context, string, string) (sp.List, error) { forbidden(); return sp.List{}, nil },
			GetListFn:                  func(context.Context, string) (sp.List, error) { forbidden(); return sp.List{}, nil },
			GetItemsFn:                 func(context.Context, string) ([]sp.Item, error) { forbidden(); return nil, nil },
			RemoveItemFn:               func(context.Context, string, string) error { forbidden(); return nil },
			LockPurchasesFn:            func(context.Context, []string) error { forbidden(); return nil },
			InsertItemFn:               func(context.Context, string, sp.Item) error { forbidden(); return nil },
			SaveItemFn:                 func(context.Context, string, sp.Item) error { forbidden(); return nil },
		}, &mocks.ShowPrepSourceMock{FetchFn: func(context.Context, sp.Identity, time.Time) (sp.Snapshot, error) {
			forbidden()
			return sp.Snapshot{}, nil
		}}
}

func TestPreviewUsesOneCachedReadAndNeverMutates(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		price  int
		status sp.Status
	}{
		{240000, sp.Supported}, {254000, sp.MixedEvidence}, {300000, sp.BelowTarget},
	} {
		t.Run(string(tc.status), func(t *testing.T) {
			p := sp.Purchase{ID: previewID, ProfileID: "card", Grader: "PSA", Grade: 10, Known: true, Exists: true,
				CampaignExists: true, Received: true, Phase: "active", LocalPriceCents: 280000, ListedPriceCents: 350000}
			snapshot := &sp.Snapshot{Identity: p.Identity(), Source: "cardladder", Complete: true, AttemptState: "complete",
				WindowStart: "2026-08-18", WindowEnd: "2026-09-16", RefreshedAt: now,
				Sales: []sp.Sale{{ID: "old", Date: "2026-09-14", PriceCents: 240000}, {ID: "latest", Date: "2026-09-16", PriceCents: 200000}, {ID: "middle", Date: "2026-09-15", PriceCents: 230000}}}
			purchases := map[string]sp.Purchase{previewID: p}
			before, err := json.Marshal(snapshot)
			require.NoError(t, err)
			store, source := previewReadOnlyStore(t)
			purchaseReads, snapshotReads := 0, 0
			store.ReadPurchasesFn = func(ctx context.Context, ids []string) (map[string]sp.Purchase, error) {
				require.NoError(t, ctx.Err())
				require.Equal(t, []string{previewID}, ids)
				purchaseReads++
				return purchases, nil
			}
			store.ReadSnapshotsFn = func(ctx context.Context, ids []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
				require.NoError(t, ctx.Err())
				require.Equal(t, []sp.Identity{p.Identity()}, ids)
				snapshotReads++
				return map[sp.Identity]*sp.Snapshot{p.Identity(): snapshot}, nil
			}
			got, err := sp.NewService(store, source, func() time.Time { return now }).Preview(context.Background(), previewID, tc.price)
			require.NoError(t, err)
			require.Equal(t, 1, purchaseReads)
			require.Equal(t, 1, snapshotReads)
			require.Equal(t, previewID, got.PurchaseID)
			require.Equal(t, 280000, got.CurrentPriceCents)
			require.Equal(t, tc.price, got.TrialPriceCents)
			require.Equal(t, tc.status, got.Status)
			require.NotEmpty(t, got.Reason)
			require.False(t, got.EvidenceNeedsReview)
			require.Empty(t, got.EvidenceReason)
			require.Equal(t, "recent-sales-v1", got.PolicyVersion)
			require.NotEmpty(t, got.EvidenceVersion)
			require.Equal(t, []string{"latest", "middle", "old"}, got.Recent.SaleIDs)
			require.Equal(t, 230000, got.Recent.MedianCents)
			require.Equal(t, p, purchases[previewID])
			after, err := json.Marshal(snapshot)
			require.NoError(t, err)
			require.Equal(t, before, after)
			encoded, err := json.Marshal(got)
			require.NoError(t, err)
			var body map[string]any
			require.NoError(t, json.Unmarshal(encoded, &body))
			for _, forbidden := range []string{"version", "canAdd", "canPack", "readiness", "evaluation"} {
				require.NotContains(t, body, forbidden)
			}
		})
	}
}

func TestPreviewDirectCallerValidation(t *testing.T) {
	for _, tc := range []struct {
		name, id string
		price    int
	}{
		{"missing id", "", 1}, {"invalid id", "bad", 1}, {"noncanonical", "11111111111141118111111111111111", 1},
		{"zero", previewID, 0}, {"negative", previewID, -1}, {"unsafe", previewID, 9007199254740992},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, source := previewReadOnlyStore(t)
			store.ReadPurchasesFn = func(context.Context, []string) (map[string]sp.Purchase, error) {
				t.Fatal("invalid request read storage")
				return nil, nil
			}
			_, err := sp.NewService(store, source, time.Now).Preview(context.Background(), tc.id, tc.price)
			require.ErrorIs(t, err, sp.ErrInvalid)
		})
	}
}

func TestPreviewReadFailuresAndCancellation(t *testing.T) {
	storageErr := errors.New("private storage detail")
	for _, tc := range []struct {
		name, stage string
		cause       error
		cancel      bool
		wantReads   int
	}{
		{"missing", "missing", sp.ErrNotFound, false, 0},
		{"purchase error", "purchase", storageErr, false, 0},
		{"snapshot error", "snapshot", storageErr, false, 1},
		{"already canceled", "before", context.Canceled, true, 0},
		{"cancel purchase", "purchase", context.Canceled, true, 0},
		{"cancel snapshot", "snapshot", context.Canceled, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, source := previewReadOnlyStore(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.stage == "before" {
				cancel()
			}
			reads := 0
			store.ReadPurchasesFn = func(c context.Context, _ []string) (map[string]sp.Purchase, error) {
				require.Same(t, ctx, c)
				if tc.stage == "before" {
					t.Fatal("canceled preview read storage")
				}
				if tc.stage == "purchase" {
					if tc.cancel {
						cancel()
						return nil, nil
					}
					return nil, tc.cause
				}
				if tc.stage == "missing" {
					return nil, nil
				}
				return map[string]sp.Purchase{previewID: {ID: previewID}}, nil
			}
			store.ReadSnapshotsFn = func(c context.Context, _ []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
				require.Same(t, ctx, c)
				reads++
				if tc.cancel {
					cancel()
					return nil, nil
				}
				return nil, tc.cause
			}
			got, err := sp.NewService(store, source, time.Now).Preview(ctx, previewID, 240000)
			require.ErrorIs(t, err, tc.cause)
			require.Equal(t, sp.PricePreview{}, got)
			require.Equal(t, tc.wantReads, reads)
			if !tc.cancel && tc.cause == storageErr {
				require.ErrorContains(t, err, tc.stage)
				require.ErrorContains(t, err, previewID)
			}
		})
	}
}

func TestPreviewRetainsUnhealthyFactsAndAcceptsPriceBounds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		price int
	}{
		{"minimum positive cent", 1},
		{"maximum safe integer cents", 9007199254740991},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, source := previewReadOnlyStore(t)
			store.ReadPurchasesFn = func(context.Context, []string) (map[string]sp.Purchase, error) {
				return map[string]sp.Purchase{previewID: {ID: previewID, ProfileID: "card", Grader: "PSA", Grade: 10}}, nil
			}
			store.ReadSnapshotsFn = func(_ context.Context, ids []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
				return map[sp.Identity]*sp.Snapshot{ids[0]: {Identity: ids[0], AttemptState: "failed", AttemptError: "Retained evidence after failed refresh",
					Sales: []sp.Sale{{ID: "sale", Date: "2026-09-16", PriceCents: 250000}}}}, nil
			}
			got, err := sp.NewService(store, source, func() time.Time { return time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC) }).Preview(context.Background(), previewID, tc.price)
			require.NoError(t, err)
			require.Equal(t, sp.NeedsReview, got.Status)
			require.True(t, got.EvidenceNeedsReview)
			require.Equal(t, "Retained evidence after failed refresh", got.EvidenceReason)
			require.Nil(t, got.Recent.GapPct)
			require.Equal(t, []string{"sale"}, got.Recent.SaleIDs)
			require.Zero(t, got.CurrentPriceCents)
			require.Equal(t, tc.price, got.TrialPriceCents)
		})
	}
}
