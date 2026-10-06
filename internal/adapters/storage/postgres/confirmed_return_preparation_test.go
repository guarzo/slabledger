package postgres

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/require"
)

func TestConfirmedReturnPreparationCommitFailureNoPhantom(t *testing.T) {
	db, _, svc, fake, id := setupReturnCore(t, "prepare-rollback-cert", 364577, "")
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `CREATE FUNCTION fail_return_prepare_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'prepare commit rejected'; END $$; CREATE CONSTRAINT TRIGGER fail_return_prepare_commit AFTER INSERT ON dh_mutation_attempts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fail_return_prepare_commit()`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DROP TRIGGER IF EXISTS fail_return_prepare_commit ON dh_mutation_attempts; DROP FUNCTION IF EXISTS fail_return_prepare_commit()`)
	})
	fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
		t.Fatal("uncommitted preparation dispatched")
		return nil, nil
	}
	state, err := svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedTarget: observedReturnTarget("prepare-rollback-cert", 364577)})
	require.Error(t, err)
	require.NotNil(t, state)
	require.Nil(t, state.Operation)
	require.Nil(t, state.PrecedingAttempt)
}

func TestConfirmedReturnCampaignDeleteAtomicAndRetention(t *testing.T) {
	for _, complete := range []bool{false, true} {
		t.Run(map[bool]string{false: "unresolved atomic conflict", true: "completed retained receipt"}[complete], func(t *testing.T) {
			db, _, svc, fake, id := setupReturnCore(t, "campaign-return-cert", 364577, "")
			ctx := context.Background()
			_, err := db.ExecContext(ctx, `INSERT INTO campaign_purchases(id,campaign_id,card_name,cert_number,grader,purchase_date,created_at,updated_at) VALUES('a-free-first','return-c','Card','unrelated-cert','PSA','2026-01-01',now(),now())`)
			require.NoError(t, err)
			fake.ReturnInventoryToStockFn = func(context.Context, int, string) (*inventory.DHReturnResult, error) {
				if !complete {
					return nil, context.DeadlineExceeded
				}
				return &inventory.DHReturnResult{DHInventoryID: 364577, ItemStatus: "in_stock", ExternalSaleID: 848}, nil
			}
			state, err := svc.ConfirmReturn(ctx, id, inventory.ConfirmReturnRequest{ReturnConfirmed: true, ExpectedTarget: observedReturnTarget("campaign-return-cert", 364577)})
			if complete {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			err = NewCampaignStore(db.DB, nil).DeleteCampaign(ctx, "return-c")
			var purchases int
			require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM campaign_purchases WHERE campaign_id='return-c'`).Scan(&purchases))
			if complete {
				require.NoError(t, err)
				require.Zero(t, purchases)
				var live, authorized *string
				require.NoError(t, db.QueryRowContext(ctx, `SELECT purchase_id,listing_authorized_at::text FROM confirmed_dh_returns WHERE id=$1`, state.Operation.ID).Scan(&live, &authorized))
				require.Nil(t, live)
				require.Nil(t, authorized)
			} else {
				require.ErrorIs(t, err, inventory.ErrReturnConflict)
				require.Equal(t, 2, purchases)
			}
		})
	}
}
