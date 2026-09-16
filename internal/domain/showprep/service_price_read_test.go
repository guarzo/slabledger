package showprep_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestPurchaseReadFailureIsNotMissingAsking(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	for _, partial := range []bool{false, true} {
		name := "empty failed read"
		if partial {
			name = "partial failed read"
		}
		t.Run(name, func(t *testing.T) {
			item := sp.Item{ID: "item", PurchaseID: id, CardName: "Saved card", CertNumber: "123", Grader: "PSA", Grade: 10,
				AcknowledgedPriceCents: 240000, AcknowledgedStatus: sp.Supported, PackedAt: "2026-09-15T12:00:00Z", Version: 7, LastCommand: "saved-intent"}
			store := &mocks.ShowPrepStoreMock{
				ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
					var purchases map[string]sp.Purchase
					if partial {
						purchases = map[string]sp.Purchase{id: {ID: id, Known: true, LocalPriceCents: 240000}}
					}
					return purchases, errors.New("controlled purchase storage failure")
				},
				GetItemsFn: func(context.Context, string) ([]sp.Item, error) { return []sp.Item{item}, nil },
			}
			svc := sp.NewService(store, nil, func() time.Time { return now })
			t.Run("aggregate", func(t *testing.T) {
				evaluations, err := svc.Evaluate(context.Background(), []string{id})
				require.NoError(t, err)
				e := evaluations[0]
				require.Equal(t, sp.Unknown, e.Availability)
				require.Equal(t, sp.NeedsReview, e.Status, "a failed read cannot establish a missing asking")
				require.Contains(t, e.Reason, "Purchase")
				require.False(t, e.CanAdd)
				require.False(t, e.CanPack)
				require.Nil(t, e.Recent.GapPct)
				projection := e
				projection.Version, projection.Readiness = "", nil
				require.Equal(t, sp.Fingerprint(struct {
					Purchase   sp.Purchase
					Evaluation sp.Evaluation
				}{sp.Purchase{ID: id}, projection}), e.Version, "hash the final business status, not the old unpriced result")
				// Full-page frontend tests consume this actual service wire shape, including zero fields.
				wire, err := json.Marshal(e)
				require.NoError(t, err)
				fixture, err := os.ReadFile("../../../web/src/react/pages/price-review/unknown-purchase.test-support.json")
				require.NoError(t, err)
				require.JSONEq(t, string(fixture), string(wire))
			})
			t.Run("list", func(t *testing.T) {
				detail, err := svc.ListDetail(context.Background(), "list")
				require.NoError(t, err)
				require.Zero(t, detail.Summary.MissingPriceCount, "unknown numeric zero is not a missing asking")
				require.Zero(t, detail.Summary.KnownValueCents)
				require.Equal(t, 1, detail.Summary.UnavailableCount)
				require.Equal(t, 1, detail.Summary.PackedCount)
				got := detail.Items[0]
				require.False(t, got.PriceChanged, "do not compare an unreadable price against history")
				require.False(t, got.SupportChanged, "do not compare an unreadable assessment against history")
				require.Equal(t, "Saved card", got.Evaluation.CardName)
				got.Evaluation = sp.Evaluation{}
				require.Equal(t, item, got, "reads preserve all historical acknowledgments, locks and exact-retry fields")
			})
		})
	}
}

func TestKnownAskingAssessmentIndependentOfPhysicalAndEvidenceDiagnostics(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name         string
		price        int
		received     bool
		sold         bool
		unhealthy    string
		want         sp.Status
		missing      int
		availability sp.Availability
	}{
		{"unpriced healthy", 0, true, false, "", sp.NoListedPrice, 1, sp.Ready},
		{"unpriced failed evidence", 0, true, false, "failed", sp.NoListedPrice, 1, sp.Ready},
		{"unpriced stale evidence", 0, true, false, "stale", sp.NoListedPrice, 1, sp.Ready},
		{"unpriced unreadable evidence", 0, true, false, "read", sp.NoListedPrice, 1, sp.Ready},
		{"not received and DH hold", 240000, false, false, "", sp.Supported, 0, sp.NotReceived},
		{"sold and DH hold", 240000, true, true, "", sp.Supported, 0, sp.Sold},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := sp.Purchase{ID: "p", Known: true, Exists: true, CampaignExists: true, Phase: "active", Received: tt.received,
				Sold: tt.sold, LocalPriceCents: tt.price, ListedPriceCents: 900000, ProfileID: "profile", Grader: "PSA", Grade: 10}
			start, end := sp.Window(now)
			snapshot := &sp.Snapshot{Identity: p.Identity(), Source: "cardladder", WindowStart: start, WindowEnd: end, Complete: true,
				AttemptState: "complete", RefreshedAt: now, Sales: []sp.Sale{{ID: "a", Date: end, PriceCents: 240000}, {ID: "b", Date: end, PriceCents: 240000}}}
			if tt.unhealthy == "failed" {
				snapshot.AttemptState, snapshot.AttemptError = "failed", "source failed"
			}
			if tt.unhealthy == "stale" {
				snapshot.RefreshedAt = now.Add(-25 * time.Hour)
			}
			store := &mocks.ShowPrepStoreMock{
				ReadPurchasesFn: func(context.Context, []string) (map[string]sp.Purchase, error) {
					return map[string]sp.Purchase{"p": p}, nil
				},
				ReadSnapshotsFn: func(context.Context, []sp.Identity) (map[sp.Identity]*sp.Snapshot, error) {
					if tt.unhealthy == "read" {
						return nil, errors.New("controlled evidence storage failure")
					}
					return map[sp.Identity]*sp.Snapshot{p.Identity(): snapshot}, nil
				},
				ObservePriceAssociationsFn: func(context.Context, []string) (map[string]bool, error) { return map[string]bool{"p": true}, nil },
				GetItemsFn: func(context.Context, string) ([]sp.Item, error) {
					return []sp.Item{{PurchaseID: "p", AcknowledgedPriceCents: 100000, AcknowledgedStatus: sp.BelowTarget}}, nil
				},
			}
			d, err := sp.NewService(store, nil, func() time.Time { return now }).ListDetail(context.Background(), "list")
			require.NoError(t, err)
			e := d.Items[0].Evaluation
			require.Equal(t, tt.availability, e.Availability)
			require.Equal(t, tt.want, e.Status)
			require.Equal(t, tt.missing, d.Summary.MissingPriceCount)
			require.Equal(t, tt.unhealthy != "", e.EvidenceNeedsReview)
			require.True(t, d.Items[0].PriceChanged, "known changes still compare against history")
			require.True(t, d.Items[0].SupportChanged)
			require.Equal(t, 1, d.Summary.AmbiguousPriceCount, "DH holds remain diagnostic")
		})
	}
}
