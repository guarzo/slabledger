package showprep_test

import (
	"context"
	"errors"
	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestShowPrepReadFailureRetainsUnknownMembers(t *testing.T) {
	store := &mocks.ShowPrepStoreMock{
		ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
			return nil, errors.New("availability read failed")
		},
		GetItemsFn: func(context.Context, string) ([]sp.Item, error) {
			return []sp.Item{{ID: "item", PurchaseID: "p", CardName: "Saved card", CertNumber: "123", Grader: "PSA", Grade: 10, PackedAt: "2026-09-14T12:00:00Z", Version: 1}}, nil
		},
	}
	svc := sp.NewService(store, nil, time.Now)
	d, err := svc.ListDetail(context.Background(), "list")
	require.NoError(t, err)
	require.Len(t, d.Items, 1)
	require.Equal(t, sp.Unknown, d.Items[0].Evaluation.Availability)
	require.False(t, d.Items[0].Evaluation.CanPack)
	require.Equal(t, "Saved card", d.Items[0].Evaluation.CardName)
	require.Zero(t, d.Summary.KnownValueCents)
	require.Equal(t, 1, d.Summary.PackedCount)
}
func TestShowPrepBatchEvidenceRead(t *testing.T) {
	ids := make([]string, 200)
	purchases := map[string]sp.Purchase{}
	for i := range ids {
		ids[i] = string(rune('a' + i))
		purchases[ids[i]] = sp.Purchase{ID: ids[i], Known: true, Exists: true, CampaignExists: true, Phase: "active", Received: true, Grader: "PSA", Grade: 10, ProfileID: ids[i], ListedPriceCents: 30000}
	}
	calls := 0
	store := &mocks.ShowPrepStoreMock{ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) { return purchases, nil }, ReadSnapshotsFn: func(_ context.Context, identities []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
		calls++
		require.Len(t, identities, 200)
		return nil, nil
	}}
	got, err := sp.NewService(store, nil, time.Now).Evaluate(context.Background(), ids)
	require.NoError(t, err)
	require.Len(t, got, 200)
	require.Equal(t, 1, calls)
}
func TestShowPrepCanonicalKnownValueIndependentOfDHWarnings(t *testing.T) {
	ctx := context.Background()
	purchases := map[string]sp.Purchase{}
	items := []sp.Item{}
	for _, tt := range []struct {
		id       string
		price    int
		received bool
	}{
		{"ready", 30000, true}, {"missing", 0, true}, {"not-received", 40000, false},
	} {
		purchases[tt.id] = sp.Purchase{ID: tt.id, Known: true, Exists: true, CampaignExists: true, Phase: "active", Received: tt.received, LocalPriceCents: tt.price, ListedPriceCents: 90000}
		items = append(items, sp.Item{PurchaseID: tt.id, AcknowledgedPriceCents: 90000})
	}
	store := &mocks.ShowPrepStoreMock{
		ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) { return purchases, nil },
		GetItemsFn:      func(context.Context, string) ([]sp.Item, error) { return items, nil },
		ObservePriceAssociationsFn: func(context.Context, []string) (map[string]bool, error) {
			return map[string]bool{"ready": true, "missing": true, "not-received": true}, nil
		},
	}
	d, err := sp.NewService(store, nil, time.Now).ListDetail(ctx, "list")
	require.NoError(t, err)
	require.Equal(t, 30000, d.Summary.KnownValueCents)
	require.Equal(t, 3, d.Summary.AmbiguousPriceCount)
	require.Equal(t, 1, d.Summary.MissingPriceCount)
	require.Equal(t, 1, d.Summary.NotReceivedCount)
	for _, item := range d.Items {
		require.True(t, item.PriceChanged)
		require.Equal(t, 90000, item.AcknowledgedPriceCents)
	}
}

func TestShowPrepMutationAndNameValidation(t *testing.T) {
	for _, tt := range []struct {
		name string
		cmd  sp.UpdateItem
		want error
	}{
		{"missing intent", sp.UpdateItem{Version: 1}, sp.ErrInvalid},
		{"invalid version", sp.UpdateItem{Acknowledge: true}, sp.ErrInvalid},
		{"stale ack", sp.UpdateItem{Version: 1, Acknowledge: true, EvaluationVersion: "old"}, sp.ErrConflict},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &mocks.ShowPrepStoreMock{GetItemsFn: func(context.Context, string) ([]sp.Item, error) {
				return []sp.Item{{ID: "item", PurchaseID: "p", Version: 1}}, nil
			}}
			_, err := sp.NewService(store, nil, time.Now).UpdateItem(context.Background(), "list", "item", tt.cmd)
			require.ErrorIs(t, err, tt.want)
		})
	}
}
