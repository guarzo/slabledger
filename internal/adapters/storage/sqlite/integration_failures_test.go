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
	// purchaseSpec captures how a test row should be created and (optionally)
	// tagged with a CL error reason. `clValueCents` lets a case mark a row
	// as already priced.
	type purchaseSpec struct {
		id             string
		cert           string
		clValueCents   int
		taggedReason   string
		taggedReasonAt string
	}

	tests := []struct {
		name               string
		purchases          []purchaseSpec
		wantByReason       map[string]int
		wantSampleByReason map[string]int
	}{
		{
			name: "mixed: tagged, priced, and two silent rows",
			purchases: []purchaseSpec{
				{id: "p-tagged", cert: "cert-tagged", taggedReason: "no_value", taggedReasonAt: "2026-04-18T12:00:00Z"},
				{id: "p-priced", cert: "cert-priced", clValueCents: 1500},
				{id: "p-silent-1", cert: "cert-silent-1"},
				{id: "p-silent-2", cert: "cert-silent-2"},
			},
			wantByReason: map[string]int{
				"no_value":        1,
				ReasonUnprocessed: 2,
			},
			wantSampleByReason: map[string]int{
				"no_value":        1,
				ReasonUnprocessed: 2,
			},
		},
		{
			name: "no silent rows — unprocessed bucket absent",
			purchases: []purchaseSpec{
				{id: "p-tagged", cert: "cert-tagged", taggedReason: "api_error", taggedReasonAt: "2026-04-18T12:00:00Z"},
				{id: "p-priced", cert: "cert-priced", clValueCents: 2000},
			},
			wantByReason: map[string]int{
				"api_error": 1,
			},
			wantSampleByReason: map[string]int{
				"api_error": 1,
			},
		},
		{
			name: "all silent — only unprocessed bucket populated",
			purchases: []purchaseSpec{
				{id: "p-silent-1", cert: "cert-silent-1"},
				{id: "p-silent-2", cert: "cert-silent-2"},
				{id: "p-silent-3", cert: "cert-silent-3"},
			},
			wantByReason: map[string]int{
				ReasonUnprocessed: 3,
			},
			wantSampleByReason: map[string]int{
				ReasonUnprocessed: 3,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupCampaignsRepo(t)
			ctx := context.Background()
			const campaignID = "camp-unproc"
			require.NoError(t, repo.CreateCampaign(ctx, &inventory.Campaign{
				ID: campaignID, Name: "T", Phase: inventory.PhaseActive,
			}))

			for _, spec := range tc.purchases {
				p := newTestPurchase(campaignID, spec.cert)
				p.ID = spec.id
				p.CLValueCents = spec.clValueCents
				require.NoError(t, repo.CreatePurchase(ctx, p))
				if spec.taggedReason != "" {
					require.NoError(t, repo.UpdatePurchaseCLError(ctx, p.ID, spec.taggedReason, spec.taggedReasonAt))
				}
			}

			// Call the shared helper directly — no need to stand up a
			// CardLadderStore when the store wrapper is a one-line forwarder.
			report, err := queryIntegrationFailures(ctx, repo.PurchaseStore.db, "cl_last_error", "cl_last_error_at", "cl_value_cents", 50)
			require.NoError(t, err)

			require.Equal(t, tc.wantByReason, report.ByReason)

			gotSampleByReason := map[string]int{}
			for _, s := range report.Samples {
				gotSampleByReason[s.Reason]++
			}
			require.Equal(t, tc.wantSampleByReason, gotSampleByReason)
		})
	}
}
