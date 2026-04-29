package psaexchange_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/psaexchange"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// catalogFixture wires a mocks.MockCatalogClient with the given fixture data
// and a per-test call counter that lets tests assert cache hit/miss behavior.
type catalogFixture struct {
	catalog    psaexchange.Catalog
	catalogErr error
	cardLadder map[string]psaexchange.CardLadder
	cardErr    map[string]error
	calls      map[string]int // cert -> times FetchCardLadder was called
}

func (f *catalogFixture) client() *mocks.MockCatalogClient {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	return &mocks.MockCatalogClient{
		FetchCatalogFn: func(_ context.Context) (psaexchange.Catalog, error) {
			return f.catalog, f.catalogErr
		},
		FetchCardLadderFn: func(_ context.Context, cert string) (psaexchange.CardLadder, error) {
			f.calls[cert]++
			if err := f.cardErr[cert]; err != nil {
				return psaexchange.CardLadder{}, err
			}
			cl, ok := f.cardLadder[cert]
			if !ok {
				return psaexchange.CardLadder{}, errors.New("no fixture for cert " + cert)
			}
			return cl, nil
		},
		CategoryURLFn: func(string) string { return "https://example/catalog" },
	}
}

func TestService_Opportunities_FiltersAndRanks(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	fx := &catalogFixture{
		catalog: psaexchange.Catalog{
			Cards: []psaexchange.CatalogCard{
				// In-scope: Pokemon, confidence=5, quarterVel=6 — eligible
				{Cert: "A", Category: "POKEMON CARDS", Grade: "10", Price: 10000.00, Front: "fa", Back: "ba", Name: "A name"},
				// Wrong category — excluded
				{Cert: "B", Category: "BASEBALL CARDS", Grade: "10", Price: 10000.00},
				// Pokemon but quarterVelocity=0 — excluded
				{Cert: "C", Category: "POKEMON CARDS", Grade: "10", Price: 10000.00},
				// Pokemon but confidence=2 — excluded
				{Cert: "D", Category: "POKEMON CARDS", Grade: "10", Price: 10000.00},
				// Pokemon, eligible, lower score
				{Cert: "E", Category: "POKEMON CARDS", Grade: "10", Price: 10000.00},
			},
			Count: 5,
		},
		cardLadder: map[string]psaexchange.CardLadder{
			"A": {EstimatedValue: 20000, Confidence: 5, OneMonthData: psaexchange.CardLadderBucket{Velocity: 10}, OneQuarterData: psaexchange.CardLadderBucket{Velocity: 30}, Description: "Card A"},
			"C": {EstimatedValue: 20000, Confidence: 5, OneMonthData: psaexchange.CardLadderBucket{Velocity: 5}, OneQuarterData: psaexchange.CardLadderBucket{Velocity: 0}},
			"D": {EstimatedValue: 20000, Confidence: 2, OneMonthData: psaexchange.CardLadderBucket{Velocity: 5}, OneQuarterData: psaexchange.CardLadderBucket{Velocity: 5}},
			"E": {EstimatedValue: 20000, Confidence: 5, OneMonthData: psaexchange.CardLadderBucket{Velocity: 1}, OneQuarterData: psaexchange.CardLadderBucket{Velocity: 3}, Description: "Card E"},
		},
	}
	svc := psaexchange.NewService(fx.client(), psaexchange.WithClock(func() time.Time { return now }))
	res, err := svc.Opportunities(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TotalCatalog != 4 {
		t.Fatalf("totalCatalog = %d, want 4 (Pokemon-only)", res.TotalCatalog)
	}
	if res.AfterFilter != 2 {
		t.Fatalf("afterFilter = %d, want 2", res.AfterFilter)
	}
	if len(res.Opportunities) != 2 {
		t.Fatalf("opportunities len = %d, want 2", len(res.Opportunities))
	}
	if res.Opportunities[0].Cert != "A" {
		t.Fatalf("top cert = %s, want A (highest score)", res.Opportunities[0].Cert)
	}
	if !sort.SliceIsSorted(res.Opportunities, func(i, j int) bool {
		return res.Opportunities[i].Score > res.Opportunities[j].Score
	}) {
		t.Fatal("results not sorted by score desc")
	}
	if res.CategoryURL != "https://example/catalog" {
		t.Fatalf("categoryURL = %q", res.CategoryURL)
	}
	if !res.FetchedAt.Equal(now) {
		t.Fatalf("fetchedAt = %v, want %v", res.FetchedAt, now)
	}
}

func TestService_Opportunities_SkipsCardLadderErrors(t *testing.T) {
	fx := &catalogFixture{
		catalog: psaexchange.Catalog{Cards: []psaexchange.CatalogCard{
			{Cert: "A", Category: "POKEMON CARDS", Grade: "10", Price: 10000.00},
			{Cert: "B", Category: "POKEMON CARDS", Grade: "10", Price: 10000.00},
		}},
		cardLadder: map[string]psaexchange.CardLadder{
			"A": {EstimatedValue: 20000, Confidence: 5, OneMonthData: psaexchange.CardLadderBucket{Velocity: 5}, OneQuarterData: psaexchange.CardLadderBucket{Velocity: 5}},
		},
		cardErr: map[string]error{"B": errors.New("boom")},
	}
	svc := psaexchange.NewService(fx.client())
	res, err := svc.Opportunities(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.AfterFilter != 1 {
		t.Fatalf("afterFilter = %d, want 1", res.AfterFilter)
	}
	if res.EnrichmentErrors != 1 {
		t.Fatalf("enrichmentErrors = %d, want 1", res.EnrichmentErrors)
	}
}

func TestService_Opportunities_CatalogErrorPropagates(t *testing.T) {
	want := errors.New("catalog down")
	fx := &catalogFixture{catalogErr: want}
	svc := psaexchange.NewService(fx.client())
	_, err := svc.Opportunities(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want wraps %v", err, want)
	}
}

func TestService_Opportunities_CachesCardLadder(t *testing.T) {
	fx := &catalogFixture{
		catalog: psaexchange.Catalog{Cards: []psaexchange.CatalogCard{
			{Cert: "A", Category: "POKEMON CARDS", Grade: "10", Price: 10000},
		}},
		cardLadder: map[string]psaexchange.CardLadder{
			"A": {EstimatedValue: 20000, Confidence: 5, OneMonthData: psaexchange.CardLadderBucket{Velocity: 5}, OneQuarterData: psaexchange.CardLadderBucket{Velocity: 5}},
		},
	}
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	tick := now
	svc := psaexchange.NewService(fx.client(),
		psaexchange.WithClock(func() time.Time { return tick }),
		psaexchange.WithCardLadderCacheTTL(24*time.Hour),
	)
	// First call: cache miss, adapter hit.
	if _, err := svc.Opportunities(context.Background()); err != nil {
		t.Fatalf("call 1: %v", err)
	}
	if fx.calls["A"] != 1 {
		t.Fatalf("after call 1: calls[A] = %d, want 1", fx.calls["A"])
	}
	// Second call within TTL: cache hit, no new adapter call.
	tick = now.Add(23 * time.Hour)
	if _, err := svc.Opportunities(context.Background()); err != nil {
		t.Fatalf("call 2: %v", err)
	}
	if fx.calls["A"] != 1 {
		t.Fatalf("after call 2 (23h): calls[A] = %d, want 1 (cache hit)", fx.calls["A"])
	}
	// Third call past TTL: refetch.
	tick = now.Add(25 * time.Hour)
	if _, err := svc.Opportunities(context.Background()); err != nil {
		t.Fatalf("call 3: %v", err)
	}
	if fx.calls["A"] != 2 {
		t.Fatalf("after call 3 (25h): calls[A] = %d, want 2 (refetch)", fx.calls["A"])
	}
}

func TestService_Opportunities_CacheDoesNotStoreErrors(t *testing.T) {
	fx := &catalogFixture{
		catalog: psaexchange.Catalog{Cards: []psaexchange.CatalogCard{
			{Cert: "A", Category: "POKEMON CARDS", Grade: "10", Price: 10000},
		}},
		cardLadder: map[string]psaexchange.CardLadder{},
		cardErr:    map[string]error{"A": errors.New("transient")},
	}
	svc := psaexchange.NewService(fx.client(), psaexchange.WithCardLadderCacheTTL(24*time.Hour))
	if _, err := svc.Opportunities(context.Background()); err != nil {
		t.Fatalf("call 1: %v", err)
	}
	// Recover by giving the fixture the data and clearing the error.
	fx.cardErr = nil
	fx.cardLadder["A"] = psaexchange.CardLadder{EstimatedValue: 20000, Confidence: 5, OneMonthData: psaexchange.CardLadderBucket{Velocity: 5}, OneQuarterData: psaexchange.CardLadderBucket{Velocity: 5}}
	res, err := svc.Opportunities(context.Background())
	if err != nil {
		t.Fatalf("call 2: %v", err)
	}
	if res.AfterFilter != 1 {
		t.Fatalf("afterFilter = %d, want 1 (errors must not be cached)", res.AfterFilter)
	}
	if fx.calls["A"] != 2 {
		t.Fatalf("calls[A] = %d, want 2 (errors must not poison the cache)", fx.calls["A"])
	}
}
