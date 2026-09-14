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

func TestServiceFailsClosedWithoutLosingPhysicalEligibility(t *testing.T) {
	for _, tt := range []struct {
		name                 string
		holdErr, evidenceErr bool
	}{{"no credentials", false, false}, {"hold storage fails", true, false}, {"evidence storage fails", false, true}} {
		t.Run(tt.name, func(t *testing.T) {
			store := &mocks.ShowPrepStoreMock{ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
				return map[string]sp.Purchase{"p": {ID: "p", Known: true, Exists: true, CampaignExists: true, Phase: "pending", Received: true, ListedPriceCents: 30000, ProfileID: "psa-1", Grader: "PSA", Grade: 10}}, nil
			}}
			if tt.holdErr {
				store.ObservePriceAssociationsFn = func(context.Context, []string) (map[string]bool, error) {
					return nil, errors.New("cannot durably record hold")
				}
			}
			if tt.evidenceErr {
				store.ReadSnapshotsFn = func(context.Context, []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
					return nil, errors.New("db read failed")
				}
			}
			svc := sp.NewService(store, nil, time.Now)
			got, err := svc.Evaluate(context.Background(), []string{"p"})
			require.NoError(t, err)
			require.Len(t, got, 1)
			require.Equal(t, sp.NeedsReview, got[0].Status)
			require.True(t, got[0].CanPack)
			require.Equal(t, tt.holdErr, got[0].PriceAssociationUnclear)
		})
	}
}

func TestShowPrepRefreshStorageFailuresAreRequestErrors(t *testing.T) {
	for _, tt := range []struct {
		name      string
		configure func(*mocks.ShowPrepStoreMock)
	}{
		{"result save", func(m *mocks.ShowPrepStoreMock) {
			m.FinishAttemptFn = func(context.Context, sp.Identity, int64, sp.Snapshot) error { return errors.New("storage failed") }
		}},
		{"evidence readback", func(m *mocks.ShowPrepStoreMock) {
			m.ReadSnapshotsFn = func(context.Context, []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
				return nil, errors.New("storage failed")
			}
		}},
		{"association readback", func(m *mocks.ShowPrepStoreMock) {
			m.ObservePriceAssociationsFn = func(context.Context, []string) (map[string]bool, error) { return nil, errors.New("storage failed") }
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &mocks.ShowPrepStoreMock{ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
				return map[string]sp.Purchase{"p": {ID: "p", ProfileID: "psa-1", Grader: "PSA", Grade: 10, Known: true, ListedPriceCents: 30000}}, nil
			}}
			tt.configure(store)
			_, err := sp.NewService(store, nil, time.Now).Refresh(context.Background(), []string{"p"}, time.Now().Add(time.Second), time.Now().Add(2*time.Second))
			require.Error(t, err)
		})
	}
}

func TestServiceRefreshReservesPersistenceContext(t *testing.T) {
	for _, tt := range []struct {
		name         string
		cancelParent bool
	}{{"source timeout", false}, {"client cancelled", true}} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			now := time.Now()
			attempt := false
			finished := false
			store := &mocks.ShowPrepStoreMock{ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
				return map[string]sp.Purchase{"p": {ID: "p", Known: true, ProfileID: "psa-1", Grader: "PSA", Grade: 10, ListedPriceCents: 30000}}, nil
			},
				BeginAttemptFn: func(context.Context, sp.Identity, time.Time) (int64, error) { attempt = true; return 1, nil },
				FinishAttemptFn: func(c context.Context, _ sp.Identity, _ int64, s sp.Snapshot) error {
					require.NoError(t, c.Err())
					require.False(t, s.Complete)
					require.NotEmpty(t, s.AttemptError)
					finished = true
					return nil
				}}
			source := &mocks.ShowPrepSourceMock{FetchFn: func(c context.Context, _ sp.Identity, _ time.Time) (sp.Snapshot, error) {
				require.True(t, attempt)
				if tt.cancelParent {
					cancel()
				}
				<-c.Done()
				return sp.Snapshot{}, c.Err()
			}}
			svc := sp.NewService(store, source, time.Now)
			_, err := svc.Refresh(ctx, []string{"p"}, now.Add(20*time.Millisecond), now.Add(time.Second))
			require.Equal(t, !tt.cancelParent, finished)
			if tt.cancelParent {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
