//go:build integration

package confirmedreturns_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/testutil/mocks"

	"github.com/stretchr/testify/require"
)

func TestListingReplayBeforeDispatchFailure(t *testing.T) {
	for _, mode := range []string{"receipt removed", "identity changed", "transient scope failure", "sale appeared"} {
		t.Run(mode, func(t *testing.T) {
			f := newListingRetryFixture(t, "server failure")
			require.Equal(t, 502, f.listHTTP(t))
			initial := f.state(t).PrecedingAttempt.ID
			hook := func() {
				var err error
				switch mode {
				case "receipt removed":
					_, err = f.db.Exec("UPDATE campaign_purchases SET received_at=NULL WHERE id='return-p'")
				case "identity changed":
					_, err = f.db.Exec("UPDATE campaign_purchases SET dh_inventory_id=43 WHERE id='return-p'")
				case "sale appeared":
					_, err = f.db.Exec(`INSERT INTO campaign_sales(id,purchase_id,sale_channel,sale_price_cents,sale_date,created_at,updated_at) VALUES('late-sale','return-p','local',25000,'2026-10-21',now(),now())`)
				default:
					f.failExecution.Store(true)
				}
				require.NoError(t, err)
			}
			f.afterPreparation.Store(&hook)
			f.fail.Store(false)
			require.NotEqual(t, 200, f.listHTTP(t))
			held := f.state(t).PrecedingAttempt
			require.Equal(t, initial, held.ID)
			require.Equal(t, "open", held.Outcome)
			require.True(t, held.Retryable)
			require.Equal(t, int32(1), f.patches.Load())
			_, err := f.db.Exec("DELETE FROM campaign_sales WHERE id='late-sale'; UPDATE campaign_purchases SET received_at=now(),dh_inventory_id=42 WHERE id='return-p'")
			require.NoError(t, err)
			require.Equal(t, 200, f.listHTTP(t))
		})
	}
}

func TestListingPatchTimeoutCanResumeIdenticalRequest(t *testing.T) {
	f := newListingRetryFixture(t, "timeout")
	gate := make(chan struct{})
	f.blockPatch.Store(&gate)
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(gate) }) })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { f.listing.ListPurchaseExplicit(ctx, "return-p", "retry-cert", ""); close(done) }()
	select {
	case <-f.patchStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("original PATCH not dispatched")
	}
	<-done
	require.True(t, f.state(t).PrecedingAttempt.Retryable)
	f.fail.Store(false)
	f.blockPatch.Store(nil)
	require.Equal(t, 200, f.listHTTP(t))
	release.Do(func() { close(gate) })
	select {
	case <-f.latePatchDone:
	case <-time.After(3 * time.Second):
		t.Fatal("late PATCH not released")
	}
	require.Nil(t, f.state(t).PrecedingAttempt)
	f.bodiesMu.Lock()
	defer f.bodiesMu.Unlock()
	require.Len(t, f.bodies, 2)
	require.Equal(t, f.bodies[0], f.bodies[1])
	require.Equal(t, int32(1), f.syncs.Load())
}

func TestPausedListingRetryPreservesPermission(t *testing.T) {
	f := newListingRetryFixture(t, "server failure")
	require.Equal(t, 502, f.listHTTP(t))
	cfgStore := postgres.NewDHStore(f.db.DB, mocks.NewMockLogger())
	ctx := context.Background()
	cfg, err := cfgStore.GetDHPushConfig(ctx)
	require.NoError(t, err)
	cfg.ListingsPaused = true
	require.NoError(t, cfgStore.SaveDHPushConfig(ctx, cfg))
	t.Cleanup(func() { cfg.ListingsPaused = false; require.NoError(t, cfgStore.SaveDHPushConfig(ctx, cfg)) })
	f.fail.Store(false)
	require.Equal(t, 409, f.listHTTP(t))
	require.True(t, f.state(t).PrecedingAttempt.Retryable)
	require.Equal(t, int32(1), f.patches.Load())
	cfg.ListingsPaused = false
	require.NoError(t, cfgStore.SaveDHPushConfig(ctx, cfg))
	require.Equal(t, 200, f.listHTTP(t))
}

func TestConcurrentListingRetryDoesNotReplaySyncFailure(t *testing.T) {
	f := newListingRetryFixture(t, "sync failure after retry")
	require.Equal(t, 502, f.listHTTP(t))
	f.fail.Store(false)
	gate := make(chan struct{})
	f.blockPatch.Store(&gate)
	results := make(chan int, 2)
	go func() { results <- f.listHTTP(t) }()
	select {
	case <-f.patchStarted:
	case <-time.After(5 * time.Second):
		close(gate)
		t.Fatal("replay PATCH never dispatched")
	}
	go func() { results <- f.listHTTP(t) }()
	close(gate)
	require.ElementsMatch(t, []int{502, 409}, []int{<-results, <-results})
	require.Equal(t, int32(2), f.patches.Load())
	require.Equal(t, int32(1), f.syncs.Load())
	require.False(t, f.state(t).PrecedingAttempt.Retryable)
}
