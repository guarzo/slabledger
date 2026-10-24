package inventory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestDHListingRetryPersistence(t *testing.T) {
	for _, tc := range []struct {
		name                                                string
		replaying, cancel, rejected, ownFails, persistFails bool
		wantEnable, wantSettle                              int
	}{
		{"patch failure", false, false, false, false, false, 1, 0},
		{"caller cancelled", false, true, false, false, false, 1, 0},
		{"definitive rejection", false, false, true, false, false, 0, 1},
		{"replayed rejection", true, false, true, false, false, 1, 0},
		{"before execution", true, false, false, true, false, 1, 0},
		{"persistence failure", false, false, false, false, true, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			original := errors.New("provider failed")
			persistence := errors.New("persistence failed")
			a := &inventory.DHMutationAttempt{ID: "a", CapturedPurchaseID: "p", DHInventoryID: 42, Kind: "list", Phase: "intake_patch_sync", Replaying: tc.replaying}
			enabled, settled, owns := 0, 0, 0
			repo := &mocks.ConfirmedReturnRepositoryMock{
				PrepareDHMutationFn: func(context.Context, string, inventory.DHMutationRequest) (*inventory.DHMutationAttempt, error) {
					return a, nil
				},
				OwnDHMutationFn: func(context.Context, string, *inventory.DHMutationAttempt) error {
					owns++
					if tc.ownFails && owns == 1 {
						return original
					}
					return nil
				},
				EnableDHListingRetryFn: func(c context.Context, _ string, _ *inventory.DHMutationAttempt) error {
					require.NoError(t, c.Err())
					enabled++
					if tc.persistFails {
						return persistence
					}
					return nil
				},
				SettleDHMutationFn: func(context.Context, string, *inventory.DHMutationAttempt, inventory.DHMutationSettlement) error {
					settled++
					return nil
				},
			}
			scope := &mocks.PurchaseMutationScopeMock{WithPurchaseMutationFn: func(c context.Context, _ string, fn func(context.Context) error) error { return fn(c) }}
			err := inventory.NewDHMutationCoordinator(scope, repo).Run(ctx, "p", func(context.Context) (inventory.DHMutationRequest, error) {
				return inventory.DHMutationRequest{Kind: "list", Phase: "intake_patch_sync", PayloadIdentity: "approved"}, nil
			}, func(context.Context, *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
				if tc.cancel {
					cancel()
				}
				cause := error(original)
				if tc.rejected {
					cause = inventory.NewDHNonMutationError("sold", "sold", original)
				}
				return nil, &inventory.DHListingPatchRetryError{Cause: cause}
			})
			require.ErrorIs(t, err, original)
			require.Equal(t, tc.wantEnable, enabled)
			require.Equal(t, tc.wantSettle, settled)
			if tc.persistFails {
				require.ErrorIs(t, err, persistence)
			}
		})
	}
}
