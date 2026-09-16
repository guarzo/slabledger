package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	cl "github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/platform/crypto"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
	"github.com/stretchr/testify/require"
)

func TestShowPrepIdentityHandoffNewCachedAndUnresolved(t *testing.T) {
	for _, tc := range []struct {
		name               string
		cached, unresolved bool
		initial, want      string
	}{
		{"cached new purchase", true, false, "", "canonical-profile"},
		{"new verified result", false, false, "", "canonical-profile"},
		{"unresolved", false, true, "", ""},
		{"preserve existing identity", true, false, "existing-profile", "existing-profile"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := showRuntimeDB(t)
			ctx := context.Background()
			logger := mocks.NewMockLogger()
			id := "11111111-1111-4111-8111-111111111111"
			showRuntimeSeed(t, db, id, tc.initial)
			_, err := db.Exec(`UPDATE campaign_purchases SET grader='BGS',grade_value=9.5 WHERE id=$1`, id)
			require.NoError(t, err)
			encryptor, err := crypto.NewAESEncryptor(strings.Repeat("fixture-", 5))
			require.NoError(t, err)
			store := postgres.NewCardLadderStore(db.DB, encryptor)
			require.NoError(t, store.SaveConfig(ctx, "fixture", "token", "collection", "key", "uid"))
			if tc.cached {
				require.NoError(t, store.SaveMappingPricing(ctx, id, " \tcanonical-profile\u2003", "BGS 9.5"))
			}
			var resolutions atomic.Int32
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/httpbuildcollectioncard":
					resolutions.Add(1)
					profile := " \tcanonical-profile\u2003"
					if tc.unresolved {
						profile = ""
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]string{"profileId": profile, "grade": "g9.5"}})
				case "/httpcardestimate":
					_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]int{"estimatedValue": 0}})
				default:
					t.Errorf("unexpected acquisition %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer fixture.Close()
			client := cl.NewClient(cl.WithFunctionsURL(fixture.URL), cl.WithBaseURL(fixture.URL), cl.WithStaticToken("fixture"))
			purchases := postgres.NewPurchaseStore(db.DB, logger)
			s := scheduler.NewCardLadderRefreshScheduler(client, store, purchases, purchases, purchases, nil, logger, config.CardLadderConfig{})
			p := inventory.Purchase{ID: id, CertNumber: id, SetName: "Base Set", Grader: "BGS", GradeValue: 9.5, GemRateID: tc.initial}
			require.NoError(t, s.PriceSinglePurchase(ctx, &p))
			var profile, grader string
			var grade float64
			require.NoError(t, db.QueryRow(`SELECT gem_rate_id,grader,grade_value FROM campaign_purchases WHERE id=$1`, id).Scan(&profile, &grader, &grade))
			require.Equal(t, tc.want, profile)
			require.Equal(t, tc.want, p.GemRateID)
			require.Equal(t, "BGS", grader)
			require.Equal(t, 9.5, grade)
			if tc.cached {
				require.Zero(t, resolutions.Load())
			} else {
				require.Equal(t, int32(1), resolutions.Load())
			}
			candidates, err := postgres.NewShowPrepWorkerStore(db.DB).Candidates(ctx)
			require.NoError(t, err)
			require.Len(t, candidates, 1)
			require.Equal(t, tc.want, candidates[0].Identity.ProfileID)
			var collectionID string
			if !tc.unresolved {
				require.NoError(t, db.QueryRow(`SELECT cl_collection_card_id FROM cl_card_mappings WHERE slab_serial=$1`, id).Scan(&collectionID))
				require.Empty(t, collectionID, "identity handoff must not require collection membership")
			}
		})
	}
}
