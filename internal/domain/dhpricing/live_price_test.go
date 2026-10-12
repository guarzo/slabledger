package dhpricing_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/dhpricing"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestLivePriceReplaysFrozenOperationBeforeNewPrice(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		sold         bool
	}{
		{"new reviewed price", "listed", false},
		{"no longer listed", "in_stock", false},
		{"local sale", "sold", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &inventory.Purchase{ID: "p", CertNumber: "cert", Grader: "PSA", DHInventoryID: 42, DHStatus: tc.status, ReviewedPriceCents: 30000, DHListingPriceCents: 10000}
			a := &inventory.DHMutationAttempt{ID: "attempt-A", CapturedPurchaseID: "p", CertNumber: "cert", Grader: "PSA", DHInventoryID: 42, Kind: "price", Phase: "live_price", Key: "key-A", PayloadIdentity: `{"ID":42,"Price":25000}`}
			state := &inventory.ConfirmedReturnState{Purchase: p, PrecedingAttempt: a}
			if tc.sold {
				state.Sale = &inventory.Sale{ID: "sale"}
			}
			repo := &mocks.ConfirmedReturnRepositoryMock{
				GetReturnStateFn: func(context.Context, string) (*inventory.ConfirmedReturnState, error) { return state, nil },
				PrepareDHMutationFn: func(_ context.Context, _ string, r inventory.DHMutationRequest) (*inventory.DHMutationAttempt, error) {
					require.Equal(t, "key-A", r.Key)
					require.Equal(t, `{"ID":42,"Price":25000}`, r.PayloadIdentity)
					a.Replaying = true
					return a, nil
				},
			}
			scope := &mocks.PurchaseMutationScopeMock{WithPurchaseMutationFn: func(c context.Context, _ string, fn func(context.Context) error) error { return fn(c) }}
			fields := &mocks.PurchaseRepositoryMock{UpdatePurchaseDHPriceSyncFn: func(_ context.Context, _ string, price int, _ time.Time) error {
				p.DHListingPriceCents = price
				return nil
			}}
			remote := &mocks.DHLivePriceUpdaterMock{UpdateLivePriceFn: func(_ context.Context, target int, key string, cents int) (*inventory.DHLivePriceResult, error) {
				require.Equal(t, 42, target)
				require.Equal(t, "key-A", key)
				require.Equal(t, 25000, cents)
				return &inventory.DHLivePriceResult{DHInventoryID: 42, ListingPriceCents: 25000, Replayed: true}, nil
			}}
			svc := dhpricing.NewLivePriceService(fields, remote, fields, inventory.NewDHMutationCoordinator(scope, repo), &mocks.DHMutationGuardsMock{}, repo, mocks.NewMockLogger())
			r := svc.SyncPurchasePrice(context.Background(), "p")
			require.NoError(t, r.Err)
			require.Equal(t, 30000, p.ReviewedPriceCents)
			if tc.status == "listed" {
				require.Equal(t, 25000, p.DHListingPriceCents)
			} else {
				require.Equal(t, 10000, p.DHListingPriceCents, "historical receipt must not rewrite current cache")
			}
		})
	}
}

func TestLivePriceExecutionUsesFreshCooldown(t *testing.T) {
	p := &inventory.Purchase{ID: "p", DHInventoryID: 42, DHStatus: "listed"}
	a := &inventory.DHMutationAttempt{ID: "A", Kind: "price", Phase: "live_price", Key: "key-A", DHInventoryID: 42, PayloadIdentity: `{"ID":42,"Price":25000}`}
	deadline := time.Now().Add(time.Hour)
	fresh := *a
	fresh.RetryNotBefore = &deadline
	reads := 0
	repo := &mocks.ConfirmedReturnRepositoryMock{
		GetReturnStateFn: func(context.Context, string) (*inventory.ConfirmedReturnState, error) {
			reads++
			pending := a
			if reads > 1 {
				pending = &fresh
			}
			return &inventory.ConfirmedReturnState{Purchase: p, PrecedingAttempt: pending}, nil
		},
		PrepareDHMutationFn: func(context.Context, string, inventory.DHMutationRequest) (*inventory.DHMutationAttempt, error) {
			a.Replaying = true
			return a, nil
		},
	}
	scope := &mocks.PurchaseMutationScopeMock{WithPurchaseMutationFn: func(c context.Context, _ string, fn func(context.Context) error) error { return fn(c) }}
	fields := &mocks.PurchaseRepositoryMock{}
	remote := &mocks.DHLivePriceUpdaterMock{UpdateLivePriceFn: func(context.Context, int, string, int) (*inventory.DHLivePriceResult, error) {
		t.Error("fresh durable cooldown bypassed")
		return &inventory.DHLivePriceResult{DHInventoryID: 42, ListingPriceCents: 25000}, nil
	}}
	svc := dhpricing.NewLivePriceService(fields, remote, fields, inventory.NewDHMutationCoordinator(scope, repo), &mocks.DHMutationGuardsMock{}, repo, mocks.NewMockLogger())
	var retry *inventory.DHMutationRetryError
	require.ErrorAs(t, svc.SyncPurchasePrice(context.Background(), "p").Err, &retry)
	require.True(t, retry.NotBefore.Equal(deadline))
}

func TestLivePriceDiscoveryFailuresAreDiagnosed(t *testing.T) {
	for _, phase := range []string{"pending", "drift"} {
		t.Run(phase, func(t *testing.T) {
			cause := errors.New("discovery unavailable")
			lookup := &mocks.PurchaseRepositoryMock{
				ListPendingDHLivePricesFn: func(context.Context) ([]string, error) {
					if phase == "pending" {
						return nil, cause
					}
					return nil, nil
				},
				ListDHPriceDriftFn: func(context.Context) ([]inventory.Purchase, error) { return nil, cause },
			}
			logger := mocks.NewCapturingLogger()
			svc := dhpricing.NewLivePriceService(lookup, nil, nil, nil, nil, nil, logger)
			require.ErrorIs(t, svc.SyncDriftedPurchases(context.Background()).ListErr, cause)
			entries := logger.Entries()
			require.Len(t, entries, 1)
			require.Equal(t, "warn", entries[0].Level)
			value, ok := entries[0].FindField("discoveryPhase")
			require.True(t, ok)
			require.Equal(t, phase, value)
			value, ok = entries[0].FindField("error")
			require.True(t, ok)
			require.Equal(t, cause, value)
		})
	}
}

func TestLivePriceRefusesUnkeyedHistory(t *testing.T) {
	for _, tc := range []struct{ name, phase, key string }{
		{"historical price patch", "patch", ""}, {"unkeyed live price", "live_price", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mocks.ConfirmedReturnRepositoryMock{GetReturnStateFn: func(context.Context, string) (*inventory.ConfirmedReturnState, error) {
				return &inventory.ConfirmedReturnState{Purchase: &inventory.Purchase{ID: "p", DHInventoryID: 42, DHStatus: "listed"}, PrecedingAttempt: &inventory.DHMutationAttempt{ID: "old-attempt", CapturedPurchaseID: "p", DHInventoryID: 42, Kind: "price", Phase: tc.phase, Key: tc.key, PayloadIdentity: `{"ID":42,"Price":25000}`}}, nil
			}}
			scope := &mocks.PurchaseMutationScopeMock{WithPurchaseMutationFn: func(c context.Context, _ string, fn func(context.Context) error) error { return fn(c) }}
			fields := &mocks.PurchaseRepositoryMock{}
			remote := &mocks.DHLivePriceUpdaterMock{UpdateLivePriceFn: func(context.Context, int, string, int) (*inventory.DHLivePriceResult, error) {
				t.Fatal("unkeyed history dispatched")
				return nil, errors.New("unexpected")
			}}
			svc := dhpricing.NewLivePriceService(fields, remote, fields, inventory.NewDHMutationCoordinator(scope, repo), &mocks.DHMutationGuardsMock{}, repo, mocks.NewMockLogger())
			var conflict *inventory.ReturnConflict
			require.ErrorAs(t, svc.SyncPurchasePrice(context.Background(), "p").Err, &conflict)
			require.Equal(t, "preceding_dh_mutation_uncertain", conflict.Code)
		})
	}
}
