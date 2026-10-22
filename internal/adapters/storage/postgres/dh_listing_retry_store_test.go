package postgres

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/require"
)

func TestPrepareDHListingReplay(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		explicit, changed, returned bool
		want                        bool
	}{
		{"explicit", true, false, false, true}, {"automatic", false, false, false, false},
		{"changed payload", true, true, false, false}, {"return hold", true, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, s, _, _, id := setupReturnCore(t, "retry-cert", 42, "")
			ctx := context.Background()
			req := inventory.DHMutationRequest{Kind: "list", Phase: "intake_patch_sync", PayloadIdentity: "approved"}
			var a *inventory.DHMutationAttempt
			require.NoError(t, s.WithPurchaseMutation(ctx, id, func(c context.Context) error { var e error; a, e = s.PrepareDHMutation(c, id, req); return e }))
			require.False(t, a.Retryable)
			require.NoError(t, s.WithPurchaseMutation(ctx, id, func(c context.Context) error { return s.EnableDHListingRetry(c, id, a) }))
			if tc.returned {
				_, err := db.ExecContext(ctx, `INSERT INTO confirmed_dh_returns(id,idempotency_key,captured_purchase_id,dh_inventory_id,cert_number,grader) VALUES('hold','key',$1,42,'retry-cert','PSA')`, id)
				require.NoError(t, err)
			}
			req.AllowListingReplay = tc.explicit
			if tc.changed {
				req.PayloadIdentity = "different"
			}
			var replay *inventory.DHMutationAttempt
			err := s.WithPurchaseMutation(ctx, id, func(c context.Context) error { var e error; replay, e = s.PrepareDHMutation(c, id, req); return e })
			if tc.want {
				require.NoError(t, err)
				require.Equal(t, a.ID, replay.ID)
				require.True(t, replay.Replaying)
			} else {
				require.ErrorIs(t, err, inventory.ErrReturnConflict)
				if tc.changed {
					var conflict *inventory.ReturnConflict
					require.ErrorAs(t, err, &conflict)
					require.Equal(t, "listing_request_changed", conflict.Code)
				}
			}
			var enabled bool
			require.NoError(t, db.QueryRowContext(ctx, "SELECT listing_retryable FROM dh_mutation_attempts WHERE id=$1", a.ID).Scan(&enabled))
			require.Equal(t, !tc.want, enabled)
		})
	}
}

func TestDHListingRetryPermission(t *testing.T) {
	for _, tc := range []struct {
		name, kind, phase, key string
		target                 int
		settled                bool
		want                   bool
	}{
		{"list", "list", "intake_patch_sync", "", 42, false, true},
		{"price", "price", "patch", "", 42, false, false},
		{"inline", "list", "intake_patch_sync", "", 0, false, false},
		{"other phase", "list", "other", "", 42, false, false},
		{"keyed list", "list", "intake_patch_sync", "key", 42, false, false},
		{"settled", "list", "intake_patch_sync", "", 42, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, s, _, _, id := setupReturnCore(t, "retry-cert", tc.target, "")
			ctx := context.Background()
			var a *inventory.DHMutationAttempt
			require.NoError(t, s.WithPurchaseMutation(ctx, id, func(c context.Context) error {
				var e error
				a, e = s.PrepareDHMutation(c, id, inventory.DHMutationRequest{Kind: tc.kind, Phase: tc.phase, Key: tc.key, PayloadIdentity: "approved"})
				return e
			}))
			if tc.settled {
				_, e := db.ExecContext(ctx, "UPDATE dh_mutation_attempts SET outcome='succeeded',settled_at=clock_timestamp() WHERE id=$1", a.ID)
				require.NoError(t, e)
			}
			err := s.WithPurchaseMutation(ctx, id, func(c context.Context) error { return s.EnableDHListingRetry(c, id, a) })
			if tc.want {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
