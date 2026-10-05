package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestDHMutationSeparatePreparationDurability(t *testing.T) {
	for _, mode := range []string{"inline sale", "legacy sale"} {
		t.Run(mode, func(t *testing.T) {
			db, store, _, _, id := setupReturnCore(t, "journal-cert", 364577, "")
			ctx := context.Background()
			sales := NewSaleStore(db.DB, mocks.NewMockLogger())
			if mode == "legacy sale" {
				_, err := db.ExecContext(ctx, `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date,created_at,updated_at) VALUES('journal-sale',$1,'local','2026-01-03',now(),now())`, id)
				require.NoError(t, err)
			}
			coordinator := inventory.NewDHMutationCoordinator(store, store)
			prepare := func(c context.Context) (inventory.DHMutationRequest, error) {
				if mode == "inline sale" {
					s, e := sales.GetSaleByPurchaseID(c, id)
					if e != nil {
						e = sales.CreateSale(c, &inventory.Sale{ID: "journal-sale", PurchaseID: id, SaleChannel: inventory.SaleChannelLocal, SaleDate: "2026-01-03", CreatedAt: time.Now(), UpdatedAt: time.Now(), DHIdempotencyKey: "durable-key"})
					}
					if s != nil {
						require.Equal(t, "durable-key", s.DHIdempotencyKey)
					}
					if e != nil {
						return inventory.DHMutationRequest{}, e
					}
				}
				key, err := sales.SetSaleIdempotencyKeyIfAbsent(c, "journal-sale", "durable-key")
				return inventory.DHMutationRequest{Kind: "sale", Phase: "record_sale", Key: key, PayloadIdentity: "sale-journal-sale-v1"}, err
			}
			err := coordinator.Run(ctx, id, prepare, func(c context.Context, a *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
				require.Equal(t, "durable-key", a.Key)
				var key string
				require.NoError(t, db.QueryRowContext(ctx, `SELECT dh_idempotency_key FROM campaign_sales WHERE id='journal-sale'`).Scan(&key))
				require.Equal(t, "durable-key", key)
				require.NoError(t, sales.SetSaleDHSaleID(c, "journal-sale", "remote-success", time.Now()))
				return nil, context.DeadlineExceeded
			})
			require.Error(t, err)
			sale, err := sales.GetSaleByPurchaseID(ctx, id)
			require.NoError(t, err)
			require.Equal(t, "durable-key", sale.DHIdempotencyKey)
			require.Empty(t, sale.DHSaleID)
			state, err := store.GetReturnState(ctx, id)
			require.NoError(t, err)
			require.NotNil(t, state.PrecedingAttempt)
			require.Error(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error { return store.AssertMutationAllowed(c, id) }))
			require.Error(t, coordinator.Run(ctx, id, func(context.Context) (inventory.DHMutationRequest, error) {
				return inventory.DHMutationRequest{Kind: "price", Phase: "patch", PayloadIdentity: "price-2"}, nil
			}, func(context.Context, *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
				t.Fatal("incompatible mutation dispatched")
				return nil, nil
			}))
			require.NoError(t, coordinator.Run(ctx, id, prepare, func(c context.Context, a *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
				require.Equal(t, state.PrecedingAttempt.ID, a.ID)
				require.NoError(t, sales.SetSaleDHSaleID(c, "journal-sale", "remote-success", time.Now()))
				return &inventory.DHMutationSettlement{Outcome: "succeeded", Receipt: "remote-success"}, nil
			}))
		})
	}
}

func TestDHMutationUnkeyedAndDefinitiveRejection(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		blocked bool
	}{
		{"transport uncertain", context.DeadlineExceeded, true},
		{"unknown 422", errors.New("422 unknown provider guard"), true},
		{"unknown 503", errors.New("503 provider outcome unknown"), true},
		{"verified error retains prior uncertainty", &inventory.DHNonMutationError{Code: "inventory_sold", Message: "sold guard", Uncertain: true}, true},
		{"cancellation plus later rejection", errors.Join(context.Canceled, inventory.NewDHNonMutationError("inventory_sold", "sold guard", nil)), true},
		{"source verified sold guard", inventory.NewDHNonMutationError("inventory_sold", "Cannot update item with status 'sold'", nil), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, store, svc, _, id := setupReturnCore(t, "uncertain-cert", 364577, "")
			ctx := context.Background()
			coordinator := inventory.NewDHMutationCoordinator(store, store)
			prepare := func(context.Context) (inventory.DHMutationRequest, error) {
				return inventory.DHMutationRequest{Kind: "listing", Phase: "patch_and_sync", PayloadIdentity: "exact-list-payload"}, nil
			}
			require.Error(t, coordinator.Run(ctx, id, prepare, func(context.Context, *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
				return nil, tt.err
			}))
			state, err := store.GetReturnState(ctx, id)
			require.NoError(t, err)
			if tt.blocked {
				require.NotNil(t, state.PrecedingAttempt)
				require.Error(t, coordinator.Run(ctx, id, prepare, func(context.Context, *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
					t.Fatal("unkeyed replay")
					return nil, nil
				}))
				_, err = svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true})
				require.ErrorIs(t, err, inventory.ErrReturnConflict)
			} else {
				require.Nil(t, state.PrecedingAttempt)
				require.NoError(t, store.WithPurchaseMutation(ctx, id, func(c context.Context) error { return store.AssertMutationAllowed(c, id) }))
			}
		})
	}
}

func TestDHMutationObservationWatermarks(t *testing.T) {
	db, store, _, _, id := setupReturnCore(t, "observe-cert", 364577, "")
	ctx := context.Background()
	fetched, err := store.ObservationTime(ctx)
	require.NoError(t, err)
	coord := inventory.NewDHMutationCoordinator(store, store)
	require.NoError(t, coord.Run(ctx, id, func(context.Context) (inventory.DHMutationRequest, error) {
		return inventory.DHMutationRequest{Kind: "price", Phase: "patch", PayloadIdentity: "exact-price-20000"}, nil
	}, func(c context.Context, _ *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
		_, e := executor(c, db.DB).ExecContext(c, `UPDATE campaign_purchases SET dh_status='listed',dh_listing_price_cents=20000 WHERE id=$1`, id)
		return &inventory.DHMutationSettlement{Outcome: "succeeded", Receipt: "verified-response"}, e
	}))
	applied, err := store.ApplyDHObservation(ctx, id, 364577, fetched, func(context.Context) error { t.Fatal("old observation applied"); return nil })
	require.NoError(t, err)
	require.False(t, applied)
	fresh, err := store.ObservationTime(ctx)
	require.NoError(t, err)
	applied, err = store.ApplyDHObservation(ctx, id, 364577, fresh, func(c context.Context) error {
		return NewPurchaseStore(db.DB, mocks.NewMockLogger()).UpdatePurchaseDHStatus(c, id, "listed")
	})
	require.NoError(t, err)
	require.True(t, applied)
}
