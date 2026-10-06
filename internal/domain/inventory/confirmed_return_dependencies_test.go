package inventory_test

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func requireCoordinationUnavailable(t *testing.T, err error) {
	t.Helper()
	require.ErrorIs(t, err, inventory.ErrReturnConflict)
	var conflict *inventory.ReturnConflict
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, "coordination_unavailable", conflict.Code)
}

func TestConfirmedReturnDurableDependencies(t *testing.T) {
	for _, tc := range []struct {
		name                                                 string
		get                                                  bool
		nilService, nilScope, typedScope, nilRepo, typedRepo bool
	}{
		{name: "nil receiver GET", get: true, nilService: true},
		{name: "nil receiver confirm", nilService: true},
		{name: "nil repository GET", get: true, nilRepo: true},
		{name: "typed nil repository GET", get: true, typedRepo: true},
		{name: "nil repository confirm", nilRepo: true},
		{name: "typed nil repository confirm", typedRepo: true},
		{name: "nil scope confirm", nilScope: true},
		{name: "typed nil scope confirm", typedScope: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scopeCalls, repoCalls, remoteCalls, generated := 0, 0, 0, 0
			var scope inventory.PurchaseMutationScope = &mocks.PurchaseMutationScopeMock{WithPurchaseMutationFn: func(ctx context.Context, _ string, fn func(context.Context) error) error {
				scopeCalls++
				return fn(ctx)
			}}
			var repo inventory.ConfirmedReturnRepository = &mocks.ConfirmedReturnRepositoryMock{
				AssertDHPreparationAllowedFn: func(context.Context) error { repoCalls++; return nil },
				GetReturnStateFn: func(context.Context, string) (*inventory.ConfirmedReturnState, error) {
					repoCalls++
					return nil, inventory.ErrPurchaseNotFound
				},
			}
			if tc.nilScope {
				scope = nil
			}
			if tc.typedScope {
				scope = (*mocks.PurchaseMutationScopeMock)(nil)
			}
			if tc.nilRepo {
				repo = nil
			}
			if tc.typedRepo {
				repo = (*mocks.ConfirmedReturnRepositoryMock)(nil)
			}
			remote := &mocks.DHReturnerMock{ReturnInventoryToStockFn: func(context.Context, int, string) (*inventory.DHReturnResult, error) { remoteCalls++; return nil, nil }}
			svc := inventory.NewConfirmedReturnService(scope, repo, remote, nil, func() string { generated++; return "unexpected" })
			if tc.nilService {
				svc = nil
			}
			var err error
			require.NotPanics(t, func() {
				if tc.get {
					_, err = svc.GetReturnState(context.Background(), "p")
				} else {
					_, err = svc.ConfirmReturn(context.Background(), "p", inventory.ConfirmReturnRequest{ReturnConfirmed: true})
				}
			})
			requireCoordinationUnavailable(t, err)
			require.Zero(t, scopeCalls)
			require.Zero(t, repoCalls)
			require.Zero(t, remoteCalls)
			require.Zero(t, generated)
		})
	}
}

// A mismatched observed identity must stop preparation before any DH write.
func TestConfirmedReturnTargetPreconditionBeforePreparation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		observed inventory.ReturnTargetIdentity
		wantCode string
	}{
		{"changed inventory ID", inventory.ReturnTargetIdentity{DHInventoryID: 43, CertNumber: "cert", Grader: "PSA"}, "identity_conflict"},
		{"changed cert", inventory.ReturnTargetIdentity{DHInventoryID: 42, CertNumber: "new-cert", Grader: "PSA"}, "identity_conflict"},
		{"changed grader", inventory.ReturnTargetIdentity{DHInventoryID: 42, CertNumber: "cert", Grader: "BGS"}, "identity_conflict"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			readTarget := &inventory.ReturnTargetIdentity{DHInventoryID: 42, CertNumber: "cert", Grader: "PSA"}
			prepared, journaled, dispatched := 0, 0, 0
			state := &inventory.ConfirmedReturnState{Purchase: &inventory.Purchase{ID: "p", DHInventoryID: tc.observed.DHInventoryID, CertNumber: tc.observed.CertNumber, Grader: tc.observed.Grader}, Sale: &inventory.Sale{ID: "sale", OrderID: "ext-848", SaleChannel: inventory.SaleChannelEbay}}
			repo := &mocks.ConfirmedReturnRepositoryMock{
				ResolveReturnFn: func(context.Context, string, inventory.ConfirmReturnRequest) (*inventory.ConfirmedReturnState, error) {
					return state, nil
				},
				GetReturnStateFn: func(context.Context, string) (*inventory.ConfirmedReturnState, error) { return state, nil },
				PrepareReturnFn: func(_ context.Context, _ string, _ inventory.ConfirmReturnRequest, op, key string) (*inventory.ConfirmedReturnState, error) {
					prepared++
					state.Operation = &inventory.ConfirmedReturnEpisode{ID: op, Key: key, DHInventoryID: tc.observed.DHInventoryID, CapturedOrderID: "ext-848"}
					return state, nil
				},
				PrepareDHMutationFn: func(context.Context, string, inventory.DHMutationRequest) (*inventory.DHMutationAttempt, error) {
					journaled++
					return &inventory.DHMutationAttempt{ID: "attempt"}, nil
				},
			}
			remote := &mocks.DHReturnerMock{ReturnInventoryToStockFn: func(_ context.Context, target int, _ string) (*inventory.DHReturnResult, error) {
				dispatched++
				return &inventory.DHReturnResult{DHInventoryID: target, ExternalSaleID: 848, ItemStatus: "in_stock"}, nil
			}}
			scope := &mocks.PurchaseMutationScopeMock{WithPurchaseMutationFn: func(ctx context.Context, _ string, fn func(context.Context) error) error { return fn(ctx) }}
			svc := inventory.NewConfirmedReturnService(scope, repo, remote, nil, func() string { return "generated" })
			sale := "sale"
			_, err := svc.ConfirmReturn(context.Background(), "p", inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedSaleID: &sale, ExpectedTarget: readTarget})
			var conflict *inventory.ReturnConflict
			require.ErrorAs(t, err, &conflict)
			require.Equal(t, tc.wantCode, conflict.Code)
			require.Zero(t, prepared)
			require.Zero(t, journaled)
			require.Zero(t, dispatched)
		})
	}
}

func TestConfirmedReturnStateOnlyRequiresRepository(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scope inventory.PurchaseMutationScope
	}{
		{"nil scope", nil}, {"typed nil scope", (*mocks.PurchaseMutationScopeMock)(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mocks.ConfirmedReturnRepositoryMock{GetReturnStateFn: func(context.Context, string) (*inventory.ConfirmedReturnState, error) {
				return &inventory.ConfirmedReturnState{Purchase: &inventory.Purchase{ID: "p"}}, nil
			}}
			svc := inventory.NewConfirmedReturnService(tc.scope, repo, nil, nil, nil)
			state, err := svc.GetReturnState(context.Background(), "p")
			require.NoError(t, err)
			require.Equal(t, "p", state.Purchase.ID)
		})
	}
}

func TestDHMutationCoordinatorDependencies(t *testing.T) {
	for _, tc := range []struct {
		name                                                                             string
		nilCoordinator, nilScope, typedScope, nilRepo, typedRepo, nilPrepare, nilExecute bool
	}{
		{name: "nil receiver", nilCoordinator: true},
		{name: "nil scope", nilScope: true},
		{name: "typed nil scope", typedScope: true},
		{name: "nil repository", nilRepo: true},
		{name: "typed nil repository", typedRepo: true},
		{name: "nil preparation callback", nilPrepare: true},
		{name: "nil execution callback", nilExecute: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scopeCalls, repoCalls, prepared, executed := 0, 0, 0, 0
			var scope inventory.PurchaseMutationScope = &mocks.PurchaseMutationScopeMock{WithPurchaseMutationFn: func(ctx context.Context, _ string, fn func(context.Context) error) error {
				scopeCalls++
				return fn(ctx)
			}}
			var repo inventory.DHMutationRepository = &mocks.ConfirmedReturnRepositoryMock{AssertDHPreparationAllowedFn: func(context.Context) error { repoCalls++; return nil }}
			if tc.nilScope {
				scope = nil
			}
			if tc.typedScope {
				scope = (*mocks.PurchaseMutationScopeMock)(nil)
			}
			if tc.nilRepo {
				repo = nil
			}
			if tc.typedRepo {
				repo = (*mocks.ConfirmedReturnRepositoryMock)(nil)
			}
			prepare := func(context.Context) (inventory.DHMutationRequest, error) {
				prepared++
				return inventory.DHMutationRequest{}, nil
			}
			execute := func(context.Context, *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
				executed++
				return nil, nil
			}
			if tc.nilPrepare {
				prepare = nil
			}
			if tc.nilExecute {
				execute = nil
			}
			coord := inventory.NewDHMutationCoordinator(scope, repo)
			if tc.nilCoordinator {
				coord = nil
			}
			var err error
			require.NotPanics(t, func() { err = coord.Run(context.Background(), "p", prepare, execute) })
			requireCoordinationUnavailable(t, err)
			require.Zero(t, scopeCalls)
			require.Zero(t, repoCalls)
			require.Zero(t, prepared)
			require.Zero(t, executed)
		})
	}
}
