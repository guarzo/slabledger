package dhpricing_test

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/dhpricing"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestCoordinatedPriceRequiresCurrentSaleState(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		missing, typedNil        bool
		saleRead, errorRead      int
		invalidState             string
		wantCode, wantSettlement string
		wantPrepared             int
	}{
		{name: "missing_reader", missing: true, wantCode: "coordination_unavailable"},
		{name: "typed_nil_reader", typedNil: true, wantCode: "coordination_unavailable"},
		{name: "sale_in_preparation", saleRead: 1, wantCode: "current_sale_present"},
		{name: "sale_in_execution", saleRead: 2, wantCode: "current_sale_present", wantPrepared: 1, wantSettlement: "rejected"},
		{name: "reader_failure_preparation", errorRead: 1},
		{name: "reader_failure_execution", errorRead: 2, wantPrepared: 1},
		{name: "missing_state", invalidState: "state", wantCode: "identity_conflict"},
		{name: "missing_purchase", invalidState: "purchase", wantCode: "identity_conflict"},
		{name: "wrong_purchase", invalidState: "identity", wantCode: "identity_conflict"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			p := &inventory.Purchase{ID: "p", DHInventoryID: 42, DHStatus: "listed", ReviewedPriceCents: 25000}
			lookup := &mocks.PurchaseRepositoryMock{GetPurchaseFn: func(context.Context, string) (*inventory.Purchase, error) { copy := *p; return &copy, nil }}
			reads, mutations, prepared, scopes := 0, 0, 0, 0
			readErr := errors.New("read failed")
			var settlement *inventory.DHMutationSettlement
			journal := &mocks.ConfirmedReturnRepositoryMock{
				PrepareDHMutationFn: func(_ context.Context, _ string, r inventory.DHMutationRequest) (*inventory.DHMutationAttempt, error) {
					prepared++
					return &inventory.DHMutationAttempt{PayloadIdentity: r.PayloadIdentity}, nil
				},
				SettleDHMutationFn: func(_ context.Context, _ string, _ *inventory.DHMutationAttempt, r inventory.DHMutationSettlement) error {
					settlement = &r
					return nil
				},
				GetReturnStateFn: func(context.Context, string) (*inventory.ConfirmedReturnState, error) {
					reads++
					if tc.errorRead == reads {
						return nil, readErr
					}
					state := &inventory.ConfirmedReturnState{Purchase: p}
					if tc.saleRead == reads {
						state.Sale = &inventory.Sale{ID: "sale"}
					}
					switch tc.invalidState {
					case "state":
						return nil, nil
					case "purchase":
						state.Purchase = nil
					case "identity":
						state.Purchase = &inventory.Purchase{ID: "another"}
					}
					return state, nil
				},
			}
			scope := &mocks.PurchaseMutationScopeMock{WithPurchaseMutationFn: func(c context.Context, _ string, fn func(context.Context) error) error { scopes++; return fn(c) }}
			remote := &mocks.DHInventoryListerMock{UpdateInventoryStatusFn: func(context.Context, int, inventory.DHInventoryStatusUpdate) (int, error) {
				mutations++
				return 25000, nil
			}}
			var reader dhpricing.ReturnStateReader = journal
			if tc.missing {
				reader = nil
			}
			if tc.typedNil {
				var nilReader *mocks.ConfirmedReturnRepositoryMock
				reader = nilReader
			}
			price := dhpricing.NewService(lookup, remote, lookup, lookup, mocks.NewMockLogger(), dhpricing.WithMutationCoordinator(inventory.NewDHMutationCoordinator(scope, journal), &mocks.DHMutationGuardsMock{}, reader))
			result := price.SyncPurchasePrice(ctx, "p")
			require.Equal(t, dhpricing.OutcomeError, result.Outcome)
			require.Error(t, result.Err)
			if tc.wantCode != "" {
				var conflict *inventory.ReturnConflict
				require.ErrorAs(t, result.Err, &conflict)
				require.Equal(t, tc.wantCode, conflict.Code)
			} else {
				require.ErrorIs(t, result.Err, readErr)
			}
			require.Zero(t, mutations)
			require.Equal(t, tc.wantPrepared, prepared)
			if tc.wantSettlement == "" {
				require.Nil(t, settlement)
			} else {
				require.NotNil(t, settlement)
				require.Equal(t, tc.wantSettlement, settlement.Outcome)
				require.Equal(t, "not-dispatched:current-sale", settlement.Receipt)
			}
			if tc.missing || tc.typedNil {
				require.Zero(t, scopes)
			}
		})
	}
}
