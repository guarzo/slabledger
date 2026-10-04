package dhlisting_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	adapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/stretchr/testify/require"
)

// Receipt mode must not hide an unknown first PATCH behind PSA rotation.
func TestInventoryAdapterCoordinatedNoRotation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		strict    bool
		status    int
		wantCalls int32
		wantError bool
	}{
		{"strict_unknown_503", true, 503, 1, true},
		{"strict_auth_401", true, 401, 1, true},
		{"strict_rate_limit_422", true, 422, 1, true},
		{"legacy_rotation", false, 503, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "PATCH", r.Method)
				w.Header().Set("Content-Type", "application/json")
				if calls.Add(1) == 1 {
					w.WriteHeader(tc.status)
					_, _ = fmt.Fprint(w, `{"error":"PSA API rate limit exceeded"}`)
					return
				}
				_, _ = fmt.Fprint(w, `{"dh_inventory_id":42,"status":"listed","listing_price_cents":25000}`)
			}))
			defer provider.Close()
			client := dh.NewClient(provider.URL, dh.WithEnterpriseKey("fixture"), dh.WithPSAKeys("key1,key2"), dh.WithRateLimitRPS(1000))
			remote := adapter.NewInventoryAdapter(client)
			if tc.strict {
				remote.WithMutationReceipts()
			}
			_, err := remote.UpdateInventoryStatus(context.Background(), 42, inventory.DHInventoryStatusUpdate{Status: "listed", ListingPriceCents: 25000})
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantCalls, calls.Load())
		})
	}
}
