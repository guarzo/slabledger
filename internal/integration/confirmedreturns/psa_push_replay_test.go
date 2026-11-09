//go:build integration

package confirmedreturns_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestPSAPushReplay(t *testing.T) {
	for _, tc := range []struct {
		name, resolution, receipt string
		batch, uncertain, blocked bool
	}{
		{name: "already imported", resolution: dh.PSAImportStatusAlreadyListed},
		{name: "batch rejected", batch: true, receipt: "batch_rejected"},
		{name: "PSA error", resolution: dh.PSAImportStatusPSAError, receipt: "psa_error"},
		{name: "rate limited", resolution: dh.PSAImportStatusPSAError, receipt: "rate_limited"},
		{name: "partner error", resolution: dh.PSAImportStatusPartnerCardError, receipt: "partner_card_error"},
		{name: "unknown resolution", resolution: "private-provider-text", receipt: "unknown_resolution"},
		{name: "transport", uncertain: true},
		{name: "nil response", uncertain: true},
		{name: "empty response", uncertain: true},
		{name: "wrong response cert", resolution: dh.PSAImportStatusPSAError, uncertain: true},
		{name: "malformed payload", blocked: true},
		{name: "wrong payload cert", blocked: true},
		{name: "sale appeared", blocked: true},
		{name: "no longer pending", blocked: true},
		{name: "return guard", blocked: true},
		{name: "wrong cert", blocked: true},
		{name: "wrong grader", blocked: true},
		{name: "wrong target", blocked: true},
		{name: "wrong kind", blocked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, store, _, _, id := setupIntegrationReturn(t, "push-cert")
			ctx := context.Background()
			logger := mocks.NewMockLogger()
			_, err := db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_inventory_id=0,dh_push_status='pending',buy_cost_cents=1000 WHERE id=$1`, id)
			require.NoError(t, err)
			repo := postgres.NewPurchaseStore(db.DB, logger)
			item := dh.PSAImportItem{CertNumber: "push-cert", CostBasisCents: 1000, Status: "in_stock", Overrides: &dh.PSAImportOverrides{Name: "Card"}}
			payload, err := json.Marshal(item)
			require.NoError(t, err)
			coord := inventory.NewDHMutationCoordinator(store, store)
			require.ErrorIs(t, coord.Run(ctx, id, func(context.Context) (inventory.DHMutationRequest, error) {
				return inventory.DHMutationRequest{Kind: "push", Phase: "psa_import", PayloadIdentity: string(payload)}, nil
			}, func(context.Context, *inventory.DHMutationAttempt) (*inventory.DHMutationSettlement, error) {
				return nil, context.DeadlineExceeded
			}), context.DeadlineExceeded)
			// Match a legacy open target-0 attempt, with receipt and metadata changed later.
			_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET received_at=now()+interval '1 second',buy_cost_cents=2222,card_name='Edited card' WHERE id=$1`, id)
			require.NoError(t, err)
			before, err := store.GetReturnState(ctx, id)
			require.NoError(t, err)
			var receivedLater bool
			require.NoError(t, db.QueryRowContext(ctx, `SELECT received_at > $1 FROM campaign_purchases WHERE id=$2`, before.PrecedingAttempt.StartedAt, id).Scan(&receivedLater))
			require.True(t, receivedLater)
			switch tc.name {
			case "return guard":
				_, err = db.ExecContext(ctx, `INSERT INTO confirmed_dh_returns(id,purchase_id,captured_purchase_id,dh_inventory_id,cert_number,grader,idempotency_key,state) VALUES('hold',$1,$1,42,'push-cert','PSA','return-key','pending')`, id)
			case "wrong cert":
				_, err = db.ExecContext(ctx, `UPDATE dh_mutation_attempts SET cert_number='other'`)
			case "wrong grader":
				_, err = db.ExecContext(ctx, `UPDATE dh_mutation_attempts SET grader='CGC'`)
			case "wrong target":
				_, err = db.ExecContext(ctx, `UPDATE dh_mutation_attempts SET dh_inventory_id=42`)
			case "wrong kind":
				_, err = db.ExecContext(ctx, `UPDATE dh_mutation_attempts SET kind='void'`)
			case "malformed payload":
				_, err = db.ExecContext(ctx, `UPDATE dh_mutation_attempts SET payload_identity='{'`)
			case "wrong payload cert":
				_, err = db.ExecContext(ctx, `UPDATE dh_mutation_attempts SET payload_identity='{"cert_number":"other","status":"in_stock"}'`)
			case "sale appeared":
				_, err = db.ExecContext(ctx, `INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_date,created_at,updated_at) VALUES('new-sale',$1,'local','2026-01-03',now(),now())`, id)
			case "no longer pending":
				_, err = db.ExecContext(ctx, `UPDATE campaign_purchases SET dh_push_status='dismissed' WHERE id=$1`, id)
			}
			require.NoError(t, err)
			remote := &mocks.DHPSAImportClientMock{PSAImportFn: func(_ context.Context, items []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
				require.Equal(t, []dh.PSAImportItem{item}, items, "replay must preserve original request")
				switch tc.name {
				case "transport":
					return nil, context.DeadlineExceeded
				case "nil response":
					return nil, nil
				case "empty response":
					return &dh.PSAImportResponse{}, nil
				case "wrong response cert":
					return &dh.PSAImportResponse{Success: true, Results: []dh.PSAImportResult{{CertNumber: "other", Resolution: tc.resolution}}}, nil
				}
				if tc.batch {
					return &dh.PSAImportResponse{Error: "private-provider-text"}, nil
				}
				return &dh.PSAImportResponse{Success: true, Results: []dh.PSAImportResult{{CertNumber: "push-cert", Resolution: tc.resolution, RateLimited: tc.name == "rate limited", DHCardID: 123, DHInventoryID: 99, Status: "in_stock", Error: "private-provider-text"}}}, nil
			}}
			cycle := func() {
				var s *scheduler.DHPushScheduler
				pending := &mocks.PurchaseRepositoryMock{GetPurchasesByDHPushStatusFn: func(c context.Context, _ string, _ int) ([]inventory.Purchase, error) {
					s.Stop()
					p, e := repo.GetPurchase(c, id)
					return []inventory.Purchase{*p}, e
				}}
				s = scheduler.NewDHPushScheduler(pending, repo, remote, repo, postgres.NewCardIDMappingRepository(db.DB), logger, scheduler.DHPushConfig{Enabled: true, Interval: time.Hour}, scheduler.WithDHPushConfigLoader(postgres.NewDHStore(db.DB, logger)), scheduler.WithDHPushMutationCoordinator(coord, store, store, repo))
				s.Start(ctx)
			}
			cycle()
			after, err := store.GetReturnState(ctx, id)
			require.NoError(t, err)
			if tc.blocked || tc.uncertain {
				require.NotNil(t, after.PrecedingAttempt)
				require.Equal(t, before.PrecedingAttempt.ID, after.PrecedingAttempt.ID)
				if tc.name != "no longer pending" {
					require.Equal(t, "pending", after.Purchase.DHPushStatus)
				}
				if tc.blocked {
					require.Zero(t, remote.Calls)
				} else {
					require.Equal(t, 1, remote.Calls)
				}
				return
			}
			require.Equal(t, 1, remote.Calls)
			require.Nil(t, after.PrecedingAttempt)
			if tc.receipt != "" {
				require.Equal(t, "pending", after.Purchase.DHPushStatus)
				var outcome, receipt string
				require.NoError(t, db.QueryRowContext(ctx, `SELECT outcome,receipt FROM dh_mutation_attempts WHERE id=$1`, before.PrecedingAttempt.ID).Scan(&outcome, &receipt))
				require.Equal(t, "rejected", outcome)
				require.Equal(t, "psa-import:"+tc.receipt, receipt)
				// A subsequent ordinary cycle must prepare a new attempt.
				remote.PSAImportFn = func(context.Context, []dh.PSAImportItem) (*dh.PSAImportResponse, error) {
					return nil, context.DeadlineExceeded
				}
				cycle()
				after, err = store.GetReturnState(ctx, id)
				require.NoError(t, err)
				require.NotNil(t, after.PrecedingAttempt)
				require.NotEqual(t, before.PrecedingAttempt.ID, after.PrecedingAttempt.ID)
			} else {
				require.Equal(t, "matched", after.Purchase.DHPushStatus)
				require.Equal(t, 99, after.Purchase.DHInventoryID)
				mapped, e := postgres.NewCardIDMappingRepository(db.DB).GetExternalID(ctx, "Card", "", "", pricing.SourceDH)
				require.NoError(t, e)
				require.Equal(t, "123", mapped, "map the captured metadata, not later edits")
			}
		})
	}
}
