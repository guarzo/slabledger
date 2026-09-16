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

// A real PSA purchase anchors the serial cache, but not a BGS purchase with
// the same cert number. Neither a matching valuation condition nor another
// purchase's verified identity is provenance for the unverified BGS slab.
func TestShowPrepIdentitySharedCertDoesNotPromoteAcrossGraders(t *testing.T) {
	for _, tc := range []struct {
		name, condition, want string
		fail                  bool
	}{
		{"fresh grader-specific profile", "PSA 10", "bgs-card", false},
		{"failure stays unresolved", "PSA 10", "", true},
		{"condition is not provenance", "BGS 9.5", "bgs-card", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := showRuntimeDB(t)
			ctx := context.Background()
			logger := mocks.NewMockLogger()
			const psaID = "11111111-1111-4111-8111-111111111111"
			const bgsID = "22222222-2222-4222-8222-222222222222"
			showRuntimeSeed(t, db, psaID, "psa-card")
			showRuntimeSeed(t, db, bgsID, "")
			_, err := db.Exec(`UPDATE campaign_purchases SET cert_number=$1,grader='BGS',grade_value=9.5 WHERE id=$2`, psaID, bgsID)
			require.NoError(t, err)
			encryptor, err := crypto.NewAESEncryptor(strings.Repeat("fixture-", 5))
			require.NoError(t, err)
			store := postgres.NewCardLadderStore(db.DB, encryptor)
			require.NoError(t, store.SaveConfig(ctx, "fixture", "token", "collection", "key", "uid"))
			require.NoError(t, store.SaveMappingPricing(ctx, psaID, "psa-card", tc.condition))
			var resolutions atomic.Int32
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/httpbuildcollectioncard":
					resolutions.Add(1)
					var body struct {
						Data cl.BuildCardRequest `json:"data"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body.Data.Cert != psaID || body.Data.Grader != "bgs" {
						t.Errorf("wrong cert/grader resolution: %+v", body.Data)
					}
					if tc.fail {
						http.Error(w, "cannot resolve this grader", http.StatusBadRequest)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]string{"profileId": "bgs-card", "grade": "g9.5"}})
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
			p := inventory.Purchase{ID: bgsID, CertNumber: psaID, SetName: "Base Set", Grader: "BGS", GradeValue: 9.5}
			require.NoError(t, s.PriceSinglePurchase(ctx, &p))
			var profile, grader string
			var grade float64
			require.NoError(t, db.QueryRow(`SELECT gem_rate_id,grader,grade_value FROM campaign_purchases WHERE id=$1`, bgsID).Scan(&profile, &grader, &grade))
			require.Equal(t, tc.want, profile, "a serial cache must not promote the other grader's card")
			require.Equal(t, tc.want, p.GemRateID)
			require.Equal(t, "BGS", grader)
			require.Equal(t, 9.5, grade)
			require.Equal(t, int32(1), resolutions.Load())
			require.NoError(t, db.QueryRow(`SELECT gem_rate_id FROM campaign_purchases WHERE id=$1`, psaID).Scan(&profile))
			require.Equal(t, "psa-card", profile)
			candidates, err := postgres.NewShowPrepWorkerStore(db.DB).Candidates(ctx)
			require.NoError(t, err)
			require.Len(t, candidates, 2)
			var foundBGS bool
			for _, candidate := range candidates {
				if candidate.Identity.Grader == "BGS" {
					foundBGS = true
					require.Equal(t, tc.want, candidate.Identity.ProfileID)
				}
			}
			require.True(t, foundBGS, "worker candidates must retain the BGS slab, even unresolved")
		})
	}
}
