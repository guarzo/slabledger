package cardladder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	sp "github.com/guarzo/slabledger/internal/domain/showprep"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestShowPrepVerifiedSource(t *testing.T) {
	for _, tt := range []struct {
		name     string
		total    int
		modify   func(int, []map[string]any) []map[string]any
		complete bool
		count    int
	}{
		{"empty", 0, nil, true, 0}, {"two", 2, nil, true, 2}, {"all pages", 111, nil, true, 111}, {"five pages", 500, nil, true, 500}, {"over budget", 600, nil, false, 500},
		{"deduplicated", 2, func(_ int, h []map[string]any) []map[string]any { h[1]["itemId"] = h[0]["itemId"]; return h }, true, 1},
		{"contradictory duplicate", 2, func(_ int, h []map[string]any) []map[string]any {
			h[1]["itemId"] = h[0]["itemId"]
			h[1]["price"] = 271.0
			return h
		}, false, 1},
		{"disjoint full pages 1 to 200", 200, func(p int, h []map[string]any) []map[string]any {
			for i := range h {
				h[i]["itemId"] = fmt.Sprintf("ebay-%d", p*100+i+1)
			}
			return h
		}, true, 200},
		{"overlapping full pages 1 to 100 and 100 to 199", 200, func(p int, h []map[string]any) []map[string]any {
			for i := range h {
				h[i]["itemId"] = fmt.Sprintf("ebay-%d", p*99+i+1)
			}
			return h
		}, false, 199},
		{"wrong profile", 2, func(_ int, h []map[string]any) []map[string]any { h[0]["profileId"] = "wrong"; return h }, false, 0},
		{"wrong grader", 2, func(_ int, h []map[string]any) []map[string]any { h[0]["gradingCompany"] = "BGS"; return h }, false, 0},
		{"missing grader", 2, func(_ int, h []map[string]any) []map[string]any { delete(h[0], "gradingCompany"); return h }, false, 0},
		{"wrong grade", 2, func(_ int, h []map[string]any) []map[string]any { h[0]["condition"] = "g9"; return h }, false, 0},
		{"zero price", 2, func(_ int, h []map[string]any) []map[string]any { h[0]["price"] = 0; return h }, false, 0},
		{"foreign currency", 2, func(_ int, h []map[string]any) []map[string]any { h[0]["currency"] = "EUR"; return h }, false, 0},
		{"bad date", 2, func(_ int, h []map[string]any) []map[string]any { h[0]["date"] = "yesterday"; return h }, false, 0},
		{"future", 2, func(_ int, h []map[string]any) []map[string]any { h[0]["date"] = "2026-09-15"; return h }, false, 0},
		{"UTC date conversion", 1, func(_ int, h []map[string]any) []map[string]any { h[0]["date"] = "2026-09-13T23:30:00-02:00"; return h }, true, 1},
		{"verified cutoff", 600, func(_ int, h []map[string]any) []map[string]any { h[len(h)-1]["date"] = "2026-08-15"; return h }, true, 99},
		{"ordering contradiction", 2, func(_ int, h []map[string]any) []map[string]any { h[0]["date"] = "2026-09-13"; return h }, false, 1},
		{"missing page", 111, func(p int, h []map[string]any) []map[string]any {
			if p == 1 {
				return nil
			}
			return h
		}, false, 100},
		{"repeated page", 201, func(p int, h []map[string]any) []map[string]any {
			if p == 1 {
				for i := range h {
					h[i]["itemId"] = fmt.Sprintf("ebay-%d", i)
				}
			}
			return h
		}, false, 100},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, "Bearer fixture", r.Header.Get("Authorization"))
				require.Contains(t, r.URL.Query().Get("filters"), "condition:g10")
				require.Equal(t, "desc", r.URL.Query().Get("direction"))
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				hits := []map[string]any{}
				for i := page * 100; i < min(tt.total, (page+1)*100); i++ {
					hits = append(hits, map[string]any{"itemId": fmt.Sprintf("ebay-%d", i), "profileId": "psa-1", "condition": "g10", "gradingCompany": "psa", "date": "2026-09-14", "price": 270.0, "platform": "ebay", "listingType": "BestOffer", "url": "https://example.test/sale"})
				}
				if tt.modify != nil {
					hits = tt.modify(page, hits)
				}
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"hits": hits, "totalHits": tt.total}))
			}))
			defer server.Close()
			client := NewClient(WithBaseURL(server.URL), WithStaticToken("fixture"))
			client.rateLimiter = rate.NewLimiter(rate.Inf, 1)
			source := NewShowPrepSource(client, nil)
			snap, err := source.Fetch(context.Background(), sp.Identity{ProfileID: "psa-1", Grader: "PSA", Grade: 10}, time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC))
			require.Equal(t, tt.complete, snap.Complete)
			if tt.complete {
				require.NoError(t, err)
			} else {
				require.NotEmpty(t, snap.AttemptError)
			}
			require.Len(t, snap.Sales, tt.count)
			require.LessOrEqual(t, calls, 5)
			if tt.count > 0 {
				require.Equal(t, 27000, snap.Sales[0].PriceCents)
			}
		})
	}
}
