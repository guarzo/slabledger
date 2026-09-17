package showprep_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func serviceReadiness(t *testing.T, e sp.Evaluation) map[string]any {
	t.Helper()
	b, err := json.Marshal(e)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(b, &wire))
	r, ok := wire["readiness"].(map[string]any)
	require.True(t, ok, "service evaluation must carry readiness")
	return r
}

func TestServiceReadinessReadOutcomes(t *testing.T) {
	for _, tt := range []struct {
		name, state, eligibility                   string
		purchaseErr, evidenceErr, holdErr, invalid bool
	}{
		{"cold", "not_checked", "needed", false, false, false, false},
		{"purchase read fails", "unavailable", "unavailable", true, false, false, false},
		{"snapshot read fails", "unavailable", "unavailable", false, true, false, false},
		{"both reads fail", "unavailable", "unavailable", true, true, false, false},
		{"invalid identity", "unavailable", "unavailable", false, false, false, true},
		{"hold failure is not evidence failure", "not_checked", "needed", false, false, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
			p := sp.Purchase{ID: "p", Known: true, Exists: true, CampaignExists: true, Received: true, Phase: "active", Grader: "PSA", Grade: 10, ProfileID: "psa-1", ListedPriceCents: 30000, LocalPriceCents: 30000}
			if tt.invalid {
				p.ProfileID = ""
			}
			store := &mocks.ShowPrepStoreMock{
				ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
					if tt.purchaseErr {
						return map[string]sp.Purchase{"p": p}, errors.New("any purchase error text")
					}
					return map[string]sp.Purchase{"p": p}, nil
				},
				ReadSnapshotsFn: func(context.Context, []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
					if tt.evidenceErr {
						return map[sp.Identity]*sp.Snapshot{p.Identity(): {Identity: p.Identity(), AttemptState: "complete", Complete: true}}, errors.New("arbitrary evidence failure")
					}
					return nil, nil
				},
				ObservePriceAssociationsFn: func(context.Context, []string) (map[string]bool, error) {
					if tt.holdErr {
						return nil, errors.New("hold read failed")
					}
					return nil, nil
				},
				GetItemsFn: func(context.Context, string) ([]sp.Item, error) { return []sp.Item{{ID: "item", PurchaseID: "p"}}, nil },
				BeginAttemptFn: func(context.Context, sp.Identity, time.Time) (int64, error) {
					t.Fatal("reads must not begin acquisition")
					return 0, nil
				},
			}
			svc := sp.NewService(store, &mocks.ShowPrepSourceMock{FetchFn: func(context.Context, sp.Identity, time.Time) (sp.Snapshot, error) {
				t.Fatal("read must not call source")
				return sp.Snapshot{}, nil
			}}, func() time.Time { return now })
			evaluations, err := svc.Evaluate(context.Background(), []string{"p"})
			require.NoError(t, err)
			r := serviceReadiness(t, evaluations[0])
			require.Equal(t, tt.state, r["state"])
			require.Equal(t, tt.eligibility, r["refreshEligibility"])
			if tt.purchaseErr || tt.invalid {
				require.Empty(t, r["identityKey"])
			} else {
				require.Equal(t, p.Identity().Key(), r["identityKey"])
			}
			if tt.evidenceErr && !tt.purchaseErr && !tt.invalid {
				require.Equal(t, "Evidence storage unavailable", evaluations[0].EvidenceReason)
				require.Equal(t, "5ad0c4fb129ea6e003f98aeee7f45016169dc318c871cb2906d4c54c0902de58", evaluations[0].Version)
				require.Equal(t, "fcdc1fb5b4cadc3df2da7670b8391b0930eac8aa99c9d6479eceffaf6e68e9d2", evaluations[0].EvidenceVersion)
			}
			if tt.purchaseErr && !tt.evidenceErr {
				require.Equal(t, "e87eee8204c142dc7c9b0d7eacf1ae3c602e3cc7d7da7de95d2c7704a53dfcd6", evaluations[0].Version)
			}
			now = now.Add(time.Minute)
			reread, err := svc.Evaluate(context.Background(), []string{"p"})
			require.NoError(t, err)
			require.Equal(t, evaluations, reread, "read instants cannot change unavailable versions either")
			detail, err := svc.ListDetail(context.Background(), "list")
			require.NoError(t, err)
			require.Equal(t, r, serviceReadiness(t, detail.Items[0].Evaluation))
			evidence, err := svc.Evidence(context.Background(), "p")
			if tt.purchaseErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, evaluations[0], evidence.Evaluation)
			require.Empty(t, evidence.Sales)
		})
	}
}

func TestServiceReadinessRefreshFailureRemainsManualOnly(t *testing.T) {
	now := time.Now().UTC()
	p := sp.Purchase{ID: "p", Known: true, Exists: true, CampaignExists: true, Phase: "active", ProfileID: "psa-1", Grader: "PSA", Grade: 10}
	var saved *sp.Snapshot
	store := &mocks.ShowPrepStoreMock{
		ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
			return map[string]sp.Purchase{"p": p}, nil
		},
		FinishAttemptFn: func(_ context.Context, _ sp.Identity, _ int64, s sp.Snapshot) error {
			s.AttemptState = "failed"
			saved = &s
			return nil
		},
		ReadSnapshotsFn: func(context.Context, []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
			return map[sp.Identity]*sp.Snapshot{p.Identity(): saved}, nil
		},
	}
	svc := sp.NewService(store, nil, func() time.Time { return now })
	evaluations, err := svc.Refresh(context.Background(), []string{"p"}, now.Add(time.Minute), now.Add(75*time.Second))
	require.NoError(t, err)
	require.Equal(t, "CardLadder credentials unavailable", evaluations[0].EvidenceReason)
	require.Equal(t, sp.NoListedPrice, evaluations[0].Status)
	r := serviceReadiness(t, evaluations[0])
	require.Equal(t, "failed", r["state"])
	require.Equal(t, "retry_only", r["refreshEligibility"])
}
