package scheduler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

// Missing or swallowed valuation failures must never renew the clean clock.
func TestCLFreshnessCleanSweep(t *testing.T) {
	for _, tc := range []struct {
		name, failure string
		clean         bool
	}{
		{"value", "", true}, {"no value", "no-value", true}, {"empty resolution", "empty", true},
		{"no cert", "no-cert", true}, {"API", "api", false}, {"quota", "quota", false},
		{"mapping read", "read", false}, {"mapping write", "mapping", false},
		{"profile write", "profile", false}, {"metadata write", "metadata", false},
		{"set write", "set", false}, {"value write", "value", false}, {"diagnostic write", "diagnostic", false},
		{"marker write", "marker", false}, {"on demand", "on-demand", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			fail := errors.New("fixture failure")
			failAt := func(stage string) error {
				if tc.failure == stage {
					return fail
				}
				return nil
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch tc.failure {
				case "api":
					http.Error(w, "fixture", http.StatusInternalServerError)
					return
				case "quota":
					http.Error(w, `{"error":{"status":"RESOURCE_EXHAUSTED"}}`, http.StatusTooManyRequests)
					return
				}
				body := `{"result":{"profileId":"profile","grade":"g10","player":"Player","set":"Real Set"}}`
				if r.URL.Path == "/httpcardestimate" {
					body = `{"result":{"estimatedValue":10}}`
				}
				if tc.failure == "empty" || (tc.failure == "no-value" && r.URL.Path == "/httpcardestimate") {
					body = `{"result":{}}`
				}
				_, err := w.Write([]byte(body))
				require.NoError(t, err)
			}))
			defer server.Close()
			clock := newMockSyncStateStore()
			clock.values["feed_cardladder_clean"] = "previous-clean"
			mappingReads := 0
			store := &mocks.CardLadderStoreMock{
				GetConfigFn: func(context.Context) (*postgres.CardLadderConfig, error) {
					return &postgres.CardLadderConfig{}, nil
				},
				ListMappingsFn: func(context.Context) ([]postgres.CLCardMapping, error) {
					mappingReads++
					if tc.clean && mappingReads == 2 {
						require.NotEqual(t, "previous-clean", clock.values["feed_cardladder_clean"], "valuation clock must precede collection hygiene")
					}
					return nil, failAt("read")
				},
				SaveMappingPricingFn: func(context.Context, string, string, string) error {
					return failAt("mapping")
				},
			}
			p := inventory.Purchase{ID: "p", CertNumber: "123", SetName: "TCG Cards"}
			if tc.failure == "no-cert" {
				p.CertNumber = ""
			}
			repo := &mocks.PurchaseRepositoryMock{ListAllUnsoldPurchasesFn: func(context.Context) ([]inventory.Purchase, error) {
				return []inventory.Purchase{p}, nil
			}}
			repo.UpdatePurchaseCLValueFn = func(context.Context, string, int, int, *int) error {
				return failAt("value")
			}
			repo.UpdatePurchaseCLErrorFn = func(context.Context, string, string, string) error {
				return failAt("diagnostic")
			}
			repo.UpdatePurchaseGemRateIDFn = func(context.Context, string, string) error {
				return failAt("profile")
			}
			repo.UpdatePurchaseCLCardMetadataFn = func(context.Context, string, string, string, string) error {
				return failAt("metadata")
			}
			repo.UpdatePurchaseSetNameFn = func(context.Context, string, string) error {
				return failAt("set")
			}
			if tc.failure == "marker" {
				clock.SetFn = func(context.Context, string, string) error { return fail }
			}
			client := cardladder.NewClient(cardladder.WithFunctionsURL(server.URL), cardladder.WithStaticToken("fixture"))
			s := NewCardLadderRefreshScheduler(client, store, repo, repo, repo, nil, mocks.NewMockLogger(), config.CardLadderConfig{Enabled: true}, WithCLFreshnessStore(clock))
			before := time.Now().UTC().Truncate(time.Second)
			var err error
			if tc.failure == "on-demand" {
				err = s.PriceSinglePurchase(ctx, &p)
			} else {
				err = s.RunOnce(ctx)
			}
			require.NoError(t, err)
			if tc.failure == "marker" {
				require.Equal(t, 2, mappingReads, "monitoring failure must not skip collection hygiene")
			}
			got := clock.values["feed_cardladder_clean"]
			if !tc.clean {
				require.Equal(t, "previous-clean", got)
				return
			}
			completion, err := time.Parse(time.RFC3339, got)
			require.NoError(t, err)
			require.Equal(t, time.UTC, completion.Location())
			require.False(t, completion.Before(before))
			require.False(t, completion.After(time.Now().UTC()))
		})
	}
}
