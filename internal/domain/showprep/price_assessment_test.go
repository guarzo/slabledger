package showprep

import (
	"encoding/json"
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecentPriceCounterexample(t *testing.T) {
	for _, tc := range []struct {
		ask  int
		want Status
	}{
		{320000, BelowTarget}, {254000, MixedEvidence}, {240000, Supported},
	} {
		t.Run(strconv.Itoa(tc.ask), func(t *testing.T) {
			status, _, recent := assessRecentPrice(tc.ask, []Sale{
				{ID: "a", Date: "2026-09-16", PriceCents: 231500},
				{ID: "b", Date: "2026-09-16", PriceCents: 202500},
				{ID: "c", Date: "2026-09-15", PriceCents: 309937},
				{ID: "d", Date: "2026-09-15", PriceCents: 242500},
				{ID: "e", Date: "2026-09-15", PriceCents: 220000},
				{ID: "f", Date: "2026-09-15", PriceCents: 233012},
				{ID: "g", Date: "2026-09-15", PriceCents: 222500},
				{ID: "h", Date: "2026-09-15", PriceCents: 232500},
			}, time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC))
			require.Equal(t, tc.want, status)
			require.Equal(t, 8, recent.Count)
			require.Equal(t, 232000, recent.MedianCents)
			require.Equal(t, []string{"a", "b", "c", "d", "e", "f", "g", "h"}, recent.SaleIDs)
			require.Equal(t, "2026-09-16", recent.LatestSaleDate)
			require.Equal(t, 2, recent.LatestSaleCount)
			require.Equal(t, 202500, recent.LatestSaleMinCents)
			require.Equal(t, 231500, recent.LatestSaleMaxCents)
		})
	}
}

func TestRecentPriceClassification(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		ask    int
		prices []int
		want   Status
		reason string
		median int
		gap    float64
	}{
		{"exact 90 percent", 10000, []int{9000, 9000}, Supported, "Recent matching sales support the asking price", 9000, 10},
		{"half cent below 90 percent", 10000, []int{8999, 9000}, BelowTarget, "Recent median below 90% of asking price", 9000, 10.005},
		{"exact 20 percent newest day drop", 10000, []int{8000, 10000}, Supported, "Recent matching sales support the asking price", 9000, 10},
		{"more than 20 percent newest day drop", 10000, []int{7999, 10001}, MixedEvidence, "A newest-day sale is more than 20% below asking", 9000, 10},
		{"median failure precedes newest day drop", 10000, []int{7000, 10000}, BelowTarget, "Recent median below 90% of asking price", 8500, 15},
		{"one supporting sale", 10000, []int{10000}, ThinEvidence, "Only one recent matching sale; review its amount", 10000, 0},
		{"one sharply low sale stays thin", 10000, []int{1000}, ThinEvidence, "Only one recent matching sale; review its amount", 1000, 90},
		{"odd median", 10000, []int{8000, 9500, 11000}, Supported, "Recent matching sales support the asking price", 9500, 5},
		{"sales above asking have negative gap", 10000, []int{11000, 11000}, Supported, "Recent matching sales support the asking price", 11000, -10},
		{"zero asking retains evidence", 0, []int{7000, 10000}, NoListedPrice, "No positive SlabLedger asking price", 8500, 0},
		{"negative asking retains evidence", -1, []int{1000}, NoListedPrice, "No positive SlabLedger asking price", 1000, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sales := make([]Sale, len(tc.prices))
			for i, price := range tc.prices {
				sales[i] = Sale{ID: string(rune('a' + i)), Date: "2026-09-16", PriceCents: price}
			}
			status, reason, recent := assessRecentPrice(tc.ask, sales, now)
			require.Equal(t, tc.want, status)
			require.Equal(t, tc.reason, reason)
			require.Equal(t, len(tc.prices), recent.Count)
			require.Equal(t, tc.median, recent.MedianCents)
			require.Equal(t, "2026-09-10", recent.WindowStart)
			require.Equal(t, "2026-09-16", recent.WindowEnd)
			if tc.ask <= 0 {
				require.Nil(t, recent.GapPct)
			} else {
				require.NotNil(t, recent.GapPct)
				require.InDelta(t, tc.gap, *recent.GapPct, 1e-10)
			}
		})
	}
}

func TestRecentPriceEmptyEvidence(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		ask    int
		sales  []Sale
		want   Status
		reason string
	}{
		{"no sales", 10000, nil, NoRecentComps, "No matching sales in the past seven UTC dates"},
		{"old sales only", 10000, []Sale{{ID: "old", Date: "2026-09-09", PriceCents: 10000}}, NoRecentComps, "No matching sales in the past seven UTC dates"},
		{"zero asking precedes no sales", 0, nil, NoListedPrice, "No positive SlabLedger asking price"},
		{"negative asking precedes no sales", -1, nil, NoListedPrice, "No positive SlabLedger asking price"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, reason, recent := assessRecentPrice(tc.ask, tc.sales, now)
			require.Equal(t, tc.want, status)
			require.Equal(t, tc.reason, reason)
			require.Equal(t, RecentPriceEvidence{
				WindowStart: "2026-09-10", WindowEnd: "2026-09-16", SaleIDs: []string{},
			}, recent)
			encoded, err := json.Marshal(recent)
			require.NoError(t, err)
			var wire map[string]any
			require.NoError(t, json.Unmarshal(encoded, &wire))
			require.Equal(t, []any{}, wire["saleIds"], "empty evidence must not serialize sale IDs as null")
			require.Contains(t, wire, "gapPct")
			require.Nil(t, wire["gapPct"])
		})
	}
}

func TestRecentPriceSevenUTCDates(t *testing.T) {
	sales := []Sale{
		{ID: "old", Date: "2026-09-09", PriceCents: 1},
		{ID: "start", Date: "2026-09-10", PriceCents: 10000},
		{ID: "end", Date: "2026-09-16", PriceCents: 10000},
	}
	for _, tc := range []struct {
		name  string
		now   time.Time
		start string
		end   string
		ids   []string
		want  Status
	}{
		{"start of date", time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC), "2026-09-10", "2026-09-16", []string{"end", "start"}, Supported},
		{"end of date", time.Date(2026, 9, 16, 23, 59, 59, 999999999, time.UTC), "2026-09-10", "2026-09-16", []string{"end", "start"}, Supported},
		{"UTC date not caller offset", time.Date(2026, 9, 17, 1, 0, 0, 0, time.FixedZone("UTC+2", 2*60*60)), "2026-09-10", "2026-09-16", []string{"end", "start"}, Supported},
		{"next date expires oldest", time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC), "2026-09-11", "2026-09-17", []string{"end"}, ThinEvidence},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, recent := assessRecentPrice(10000, sales, tc.now)
			require.Equal(t, tc.want, status)
			require.Equal(t, tc.start, recent.WindowStart)
			require.Equal(t, tc.end, recent.WindowEnd)
			require.Equal(t, tc.ids, recent.SaleIDs)
			require.Equal(t, len(tc.ids), recent.Count)
			require.Equal(t, 10000, recent.MedianCents)
		})
	}
}

func TestRecentPriceNewestFiveAndAllCutoffTies(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		sales []Sale
		ids   []string
	}{
		{
			name: "exactly five newest dates",
			sales: []Sale{
				{ID: "older", Date: "2026-09-11", PriceCents: 1},
				{ID: "e", Date: "2026-09-12", PriceCents: 10000},
				{ID: "d", Date: "2026-09-13", PriceCents: 10000},
				{ID: "c", Date: "2026-09-14", PriceCents: 10000},
				{ID: "b", Date: "2026-09-15", PriceCents: 10000},
				{ID: "a", Date: "2026-09-16", PriceCents: 10000},
			},
			ids: []string{"a", "b", "c", "d", "e"},
		},
		{
			name: "every fifth date tie but no older dates",
			sales: []Sale{
				{ID: "older", Date: "2026-09-11", PriceCents: 1},
				{ID: "g", Date: "2026-09-12", PriceCents: 10000},
				{ID: "e", Date: "2026-09-12", PriceCents: 10000},
				{ID: "f", Date: "2026-09-12", PriceCents: 10000},
				{ID: "d", Date: "2026-09-13", PriceCents: 10000},
				{ID: "c", Date: "2026-09-14", PriceCents: 10000},
				{ID: "b", Date: "2026-09-15", PriceCents: 10000},
				{ID: "a", Date: "2026-09-16", PriceCents: 10000},
			},
			ids: []string{"a", "b", "c", "d", "e", "f", "g"},
		},
		{
			name: "five sales not five distinct dates",
			sales: []Sale{
				{ID: "older", Date: "2026-09-14", PriceCents: 1},
				{ID: "f", Date: "2026-09-15", PriceCents: 10000},
				{ID: "e", Date: "2026-09-15", PriceCents: 10000},
				{ID: "d", Date: "2026-09-16", PriceCents: 10000},
				{ID: "c", Date: "2026-09-16", PriceCents: 10000},
				{ID: "b", Date: "2026-09-16", PriceCents: 10000},
				{ID: "a", Date: "2026-09-16", PriceCents: 10000},
			},
			ids: []string{"a", "b", "c", "d", "e", "f"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rng := rand.New(rand.NewPCG(1, 2))
			for range 64 {
				sales := slices.Clone(tc.sales)
				rng.Shuffle(len(sales), func(i, j int) { sales[i], sales[j] = sales[j], sales[i] })
				before := slices.Clone(sales)
				status, _, recent := assessRecentPrice(10000, sales, now)
				require.Equal(t, Supported, status)
				require.Equal(t, tc.ids, recent.SaleIDs)
				require.Equal(t, len(tc.ids), recent.Count)
				require.Equal(t, 10000, recent.MedianCents)
				require.Equal(t, before, sales, "assessment must not reorder or mutate caller-owned sales")
			}
		})
	}
}

func TestRecentPriceNewestDayRecovery(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		sales  []Sale
		want   Status
		count  int
		min    int
		max    int
		median int
	}{
		{"70 then 100 then 100 recovers", []Sale{
			{ID: "a", Date: "2026-09-14", PriceCents: 7000},
			{ID: "b", Date: "2026-09-15", PriceCents: 10000},
			{ID: "c", Date: "2026-09-16", PriceCents: 10000},
		}, Supported, 1, 10000, 10000, 10000},
		{"any newest day low sale contradicts", []Sale{
			{ID: "a", Date: "2026-09-16", PriceCents: 10000},
			{ID: "b", Date: "2026-09-16", PriceCents: 7000},
			{ID: "c", Date: "2026-09-15", PriceCents: 12000},
		}, MixedEvidence, 2, 7000, 10000, 10000},
		{"older history cannot rescue recent contradiction", []Sale{
			{ID: "a", Date: "2026-09-16", PriceCents: 7000},
			{ID: "b", Date: "2026-09-15", PriceCents: 7000},
			{ID: "c", Date: "2026-09-09", PriceCents: 12000},
			{ID: "d", Date: "2026-09-08", PriceCents: 12000},
			{ID: "e", Date: "2026-09-07", PriceCents: 12000},
		}, BelowTarget, 1, 7000, 7000, 7000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, recent := assessRecentPrice(10000, tc.sales, now)
			require.Equal(t, tc.want, status)
			require.Equal(t, "2026-09-16", recent.LatestSaleDate)
			require.Equal(t, tc.count, recent.LatestSaleCount)
			require.Equal(t, tc.min, recent.LatestSaleMinCents)
			require.Equal(t, tc.max, recent.LatestSaleMaxCents)
			require.Equal(t, tc.median, recent.MedianCents)
		})
	}
}

func TestRecentPriceLargeIntegerBoundaries(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	maxInt := int(^uint(0) >> 1)
	ask := maxInt - maxInt%10
	ninetyPercent := 9 * (ask / 10)
	eightyPercent := 4 * (ask / 5)
	for _, tc := range []struct {
		name   string
		ask    int
		prices []int
		want   Status
		median int
	}{
		{"maximum even median", maxInt, []int{maxInt, maxInt}, Supported, maxInt},
		{"maximum odd median", maxInt, []int{maxInt, maxInt, maxInt}, Supported, maxInt},
		{"maximum half cent rounds up", maxInt, []int{maxInt - 1, maxInt}, Supported, maxInt},
		{"maximum asking low evidence", maxInt, []int{1, 1}, BelowTarget, 1},
		{"maximum evidence tiny asking", 1, []int{maxInt, maxInt}, Supported, maxInt},
		{"large exact 90 percent", ask, []int{ninetyPercent, ninetyPercent}, Supported, ninetyPercent},
		{"large half cent below 90 percent", ask, []int{ninetyPercent - 1, ninetyPercent}, BelowTarget, ninetyPercent},
		{"large exact 20 percent drop", ask, []int{eightyPercent, ask}, Supported, ninetyPercent},
		{"large more than 20 percent drop", ask, []int{eightyPercent - 1, ask + 1}, MixedEvidence, ninetyPercent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sales := make([]Sale, len(tc.prices))
			for i, price := range tc.prices {
				sales[i] = Sale{ID: string(rune('a' + i)), Date: "2026-09-16", PriceCents: price}
			}
			status, _, recent := assessRecentPrice(tc.ask, sales, now)
			require.Equal(t, tc.want, status)
			require.Equal(t, tc.median, recent.MedianCents)
			require.Equal(t, len(tc.prices), recent.Count)
			require.NotNil(t, recent.GapPct)
		})
	}
}
