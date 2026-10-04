package postgres

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestConfirmedReturnLocalWithoutExternalCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name              string
		sale, typedRemote bool
	}{
		{"local sale nil remote", true, false}, {"local sale typed nil remote", true, true},
		{"local noop nil remote", false, false}, {"local noop typed nil remote", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, store, _, _, id := setupReturnCore(t, "local-capability", 0, "")
			ctx := context.Background()
			req := inventory.ConfirmReturnRequest{ReturnConfirmed: true}
			if tc.sale {
				sale := "local-sale"
				req.ExpectedSaleID = &sale
				_, err := db.ExecContext(ctx, `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_price_cents,sale_date,created_at,updated_at) VALUES($1,$2,'local',10000,'2026-01-03',now(),now())`, sale, id)
				require.NoError(t, err)
			}
			var remote inventory.DHReturner
			if tc.typedRemote {
				remote = (*mocks.DHReturnerMock)(nil)
			}
			svc := inventory.NewConfirmedReturnService(store, store, remote, nil, nil)
			before, err := svc.GetReturnState(ctx, id)
			require.NoError(t, err)
			require.Equal(t, tc.sale, before.Sale != nil)
			state, err := svc.ConfirmReturn(ctx, id, req)
			require.NoError(t, err)
			require.Nil(t, state.Sale)
			want := "no_return_required"
			if tc.sale {
				want = "local"
			}
			require.Equal(t, want, state.Outcome)
			require.Equal(t, before.Purchase, state.Purchase, "local unsell must not reset DH fields or prices")
			require.Nil(t, state.Operation)
			require.Nil(t, state.PrecedingAttempt)
			var n int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM confirmed_dh_returns)+(SELECT count(*) FROM dh_mutation_attempts)`).Scan(&n))
			require.Zero(t, n)
		})
	}
}

func TestConfirmedReturnExternalMissingCapabilitiesNoGhost(t *testing.T) {
	for _, tc := range []struct {
		name                                         string
		sale, typedRemote, missingGenerator, pending bool
	}{
		{name: "preflight nil remote"}, {name: "preflight typed nil remote", typedRemote: true},
		{name: "imported nil remote", sale: true}, {name: "imported typed nil remote", sale: true, typedRemote: true},
		{name: "new episode missing generator", sale: true, missingGenerator: true},
		{name: "preflight sold missing generator", missingGenerator: true},
		{name: "pending nil remote", sale: true, pending: true}, {name: "pending typed nil remote", sale: true, pending: true, typedRemote: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			order := ""
			if tc.sale {
				order = "ext-848"
			}
			db, store, _, fake, id := setupReturnCore(t, "missing-capability", 364577, order)
			ctx := context.Background()
			req := inventory.ConfirmReturnRequest{ReturnConfirmed: true}
			if tc.sale {
				sale := "old-sale"
				req.ExpectedSaleID = &sale
			}
			if tc.pending {
				prepareCapabilityReplay(t, store, id, req)
			}
			var remote inventory.DHReturner
			if tc.typedRemote {
				remote = (*mocks.DHReturnerMock)(nil)
			}
			generated, dispatched := 0, 0
			gen := func() string { generated++; return "unexpected" }
			if tc.missingGenerator {
				gen = nil
				remote = fake
				fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) { dispatched++; return nil, nil }
			}
			svc := inventory.NewConfirmedReturnService(store, store, remote, nil, gen)
			var err error
			require.NotPanics(t, func() { _, err = svc.ConfirmReturn(ctx, id, req) })
			require.ErrorIs(t, err, inventory.ErrReturnConflict)
			var conflict *inventory.ReturnConflict
			require.ErrorAs(t, err, &conflict)
			require.Equal(t, "coordination_unavailable", conflict.Code)
			require.Zero(t, generated)
			require.Zero(t, dispatched)
			fresh, err := store.GetReturnState(ctx, id)
			require.NoError(t, err)
			require.Equal(t, tc.sale, fresh.Sale != nil)
			require.Equal(t, "pending", fresh.Purchase.DHPushStatus)
			require.Equal(t, 999, fresh.Purchase.DHListingPriceCents)
			require.False(t, fresh.AwaitingListing)
			if tc.pending {
				require.NotNil(t, fresh.PrecedingAttempt)
				require.Equal(t, "persisted-key", fresh.PrecedingAttempt.Key)
				require.Equal(t, "open", fresh.PrecedingAttempt.Outcome)
			} else {
				require.Nil(t, fresh.PrecedingAttempt)
			}
			var operations, attempts int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM confirmed_dh_returns),(SELECT count(*) FROM dh_mutation_attempts)`).Scan(&operations, &attempts))
			wantOperations := 0
			if tc.pending {
				wantOperations = 1
				require.Equal(t, "persisted-key", fresh.Operation.Key)
				require.Equal(t, "pending", fresh.Operation.State)
			}
			require.Equal(t, wantOperations, operations)
			require.Equal(t, wantOperations, attempts)
		})
	}
}

func TestConfirmedReturnNoGeneratorForExistingEpisodeOrNoop(t *testing.T) {
	for _, tc := range []struct {
		name, status                    string
		pending, completed, typedRemote bool
	}{
		{name: "in stock noop", status: "in_stock"}, {name: "listed noop", status: "listed"},
		{name: "pending replay", pending: true},
		{name: "completed nil remote", completed: true}, {name: "completed typed nil remote", completed: true, typedRemote: true},
		{name: "completed replay after listing nil remote", completed: true, status: "listed"},
		{name: "completed replay after subsequent sale nil remote", completed: true, status: "sold"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, store, initial, fake, id := setupReturnCore(t, "existing-capability", 364577, "")
			ctx := context.Background()
			req := inventory.ConfirmReturnRequest{ReturnConfirmed: true}
			calls := 0
			fake.ReturnInventoryToStockFn = func(_ context.Context, target int, key string) (*inventory.DHReturnResult, error) {
				calls++
				if tc.pending {
					require.Equal(t, "persisted-key", key)
				}
				return &inventory.DHReturnResult{DHInventoryID: target, ExternalSaleID: 848, ItemStatus: "in_stock"}, nil
			}
			if tc.pending {
				prepareCapabilityReplay(t, store, id, req)
			}
			var remote inventory.DHReturner = fake
			if tc.completed {
				state, err := initial.ConfirmReturn(ctx, id, req)
				require.NoError(t, err)
				req.OperationID = state.Operation.ID
				if tc.status != "" {
					require.NoError(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error { return store.AuthorizeReturnedListing(c, id, state.Operation.ID) }))
					_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_status=$1 WHERE id=$2`, tc.status, id)
					require.NoError(t, err)
					if tc.status == "sold" {
						_, err = db.ExecContext(ctx, `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_price_cents,sale_date,order_id,created_at,updated_at) VALUES('new-sale',$1,'ebay',25000,'2026-01-04','ext-999',now(),now())`, id)
						require.NoError(t, err)
					}
				}
				remote = nil
				if tc.typedRemote {
					remote = (*mocks.DHReturnerMock)(nil)
				}
			} else if !tc.pending {
				fake.GetReturnInventoryStatusFn = func(context.Context, int, string) (string, error) { return tc.status, nil }
			} else {
				fake.GetReturnInventoryStatusFn = func(context.Context, int, string) (string, error) {
					t.Fatal("pending replay must not preflight")
					return "", nil
				}
			}
			svc := inventory.NewConfirmedReturnService(store, store, remote, nil, nil, inventory.WithConfirmedReturnLogger((*mocks.CapturingLogger)(nil)))
			before, err := svc.GetReturnState(ctx, id)
			require.NoError(t, err)
			var state *inventory.ConfirmedReturnState
			require.NotPanics(t, func() { state, err = svc.ConfirmReturn(ctx, id, req) })
			require.NoError(t, err)
			if tc.completed {
				require.Equal(t, "completed_replay", state.Outcome)
				require.Equal(t, tc.status == "", state.AwaitingListing)
				require.Equal(t, before.Operation, state.Operation)
				require.Equal(t, before.Purchase, state.Purchase)
				require.Equal(t, before.Sale, state.Sale)
				require.Equal(t, 1, calls)
			} else if tc.pending {
				require.Equal(t, "completed", state.Operation.State)
				require.Equal(t, "persisted-op", state.Operation.ID)
				require.Equal(t, 1, calls)
			} else {
				require.Equal(t, "no_return_required", state.Outcome)
				require.Nil(t, state.Operation)
				require.Equal(t, before.Purchase, state.Purchase)
				require.Zero(t, calls)
			}
			var n, attempts int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM confirmed_dh_returns),(SELECT count(*) FROM dh_mutation_attempts)`).Scan(&n, &attempts))
			wantOperations := 0
			if tc.pending || tc.completed {
				wantOperations = 1
			}
			require.Equal(t, wantOperations, n)
			require.Equal(t, wantOperations, attempts)
		})
	}
}

func prepareCapabilityReplay(t *testing.T, store *ConfirmedReturnStore, id string, req inventory.ConfirmReturnRequest) {
	t.Helper()
	require.NoError(t, store.WithPurchaseMutation(context.Background(), id, func(c context.Context) error {
		if _, err := store.PrepareReturn(c, id, req, "persisted-op", "persisted-key"); err != nil {
			return err
		}
		_, err := store.PrepareDHMutation(c, id, inventory.DHMutationRequest{Kind: "return", Phase: "return_to_stock", Key: "persisted-key", PayloadIdentity: "return:persisted-op:364577:true", ReturnOperationID: "persisted-op"})
		return err
	}))
}
