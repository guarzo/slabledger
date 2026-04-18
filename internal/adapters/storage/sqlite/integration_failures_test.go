package sqlite

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/require"
)

// TestQueryIntegrationFailures_UnprocessedBucket covers the observability fix
// for silent misses: purchases with no value and no error tag previously were
// invisible to the admin UI. They must now appear under the synthetic
// "unprocessed" reason.
func TestQueryIntegrationFailures_UnprocessedBucket(t *testing.T) {
	repo := setupCampaignsRepo(t)
	ctx := context.Background()
	campaignID := "camp-unproc"
	require.NoError(t, repo.CreateCampaign(ctx, &inventory.Campaign{
		ID: campaignID, Name: "T", Phase: inventory.PhaseActive,
	}))

	// Four purchases covering the quadrants of (has value?) x (has error?):
	tagged := newTestPurchase(campaignID, "cert-tagged")        // error tag + no value → counts under tag
	priced := newTestPurchase(campaignID, "cert-priced")        // has value + no error → skipped
	silent := newTestPurchase(campaignID, "cert-silent")        // no value + no error → unprocessed
	silentOld := newTestPurchase(campaignID, "cert-silent-old") // second unprocessed row
	priced.CLValueCents = 1500
	for _, p := range []*inventory.Purchase{tagged, priced, silent, silentOld} {
		require.NoError(t, repo.CreatePurchase(ctx, p))
	}

	// Tag one row with a real error reason. UpdatePurchaseCLError is on
	// PurchaseStore, which is embedded in testCampaignsRepository.
	require.NoError(t, repo.UpdatePurchaseCLError(ctx, tagged.ID, "no_value", "2026-04-18T12:00:00Z"))

	// Call the shared helper directly — no need to stand up a CardLadderStore
	// when the store wrapper is a one-line forwarder.
	report, err := queryIntegrationFailures(ctx, repo.PurchaseStore.db, "cl_last_error", "cl_last_error_at", "cl_value_cents", 50)
	require.NoError(t, err)

	require.Equal(t, 1, report.ByReason["no_value"], "tagged row counted under its reason")
	require.Equal(t, 2, report.ByReason[ReasonUnprocessed], "silent rows counted as unprocessed")

	// Both silent rows should appear in the samples, alongside the tagged one.
	reasons := map[string]int{}
	for _, s := range report.Samples {
		reasons[s.Reason]++
	}
	require.Equal(t, 2, reasons[ReasonUnprocessed])
	require.Equal(t, 1, reasons["no_value"])
}
