package psaexchange

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"
)

type fakeCatalogClient struct {
	catalog    Catalog
	catalogErr error
	cardLadder map[string]CardLadder
	cardErr    map[string]error
	calls      map[string]int // cert -> times FetchCardLadder was called (used for cache assertions)
}

func (f *fakeCatalogClient) FetchCatalog(_ context.Context) (Catalog, error) {
	return f.catalog, f.catalogErr
}
func (f *fakeCatalogClient) FetchCardLadder(_ context.Context, cert string) (CardLadder, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[cert]++
	if err := f.cardErr[cert]; err != nil {
		return CardLadder{}, err
	}
	cl, ok := f.cardLadder[cert]
	if !ok {
		return CardLadder{}, errors.New("no fixture for cert " + cert)
	}
	return cl, nil
}
func (f *fakeCatalogClient) CategoryURL(_ string) string { return "https://example/catalog" }

func TestService_Opportunities_FiltersAndRanks(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	client := &fakeCatalogClient{
		catalog: Catalog{
			Cards: []CatalogCard{
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
		cardLadder: map[string]CardLadder{
			"A": {EstimatedValue: 20000, Confidence: 5, OneMonthData: CardLadderBucket{Velocity: 10}, OneQuarterData: CardLadderBucket{Velocity: 30}, Description: "Card A"},
			"C": {EstimatedValue: 20000, Confidence: 5, OneMonthData: CardLadderBucket{Velocity: 5}, OneQuarterData: CardLadderBucket{Velocity: 0}},
			"D": {EstimatedValue: 20000, Confidence: 2, OneMonthData: CardLadderBucket{Velocity: 5}, OneQuarterData: CardLadderBucket{Velocity: 5}},
			"E": {EstimatedValue: 20000, Confidence: 5, OneMonthData: CardLadderBucket{Velocity: 1}, OneQuarterData: CardLadderBucket{Velocity: 3}, Description: "Card E"},
		},
	}
	svc := NewService(client, withClock(func() time.Time { return now }))
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
	client := &fakeCatalogClient{
		catalog: Catalog{Cards: []CatalogCard{
			{Cert: "A", Category: "POKEMON CARDS", Grade: "10", Price: 10000.00},
			{Cert: "B", Category: "POKEMON CARDS", Grade: "10", Price: 10000.00},
		}},
		cardLadder: map[string]CardLadder{
			"A": {EstimatedValue: 20000, Confidence: 5, OneMonthData: CardLadderBucket{Velocity: 5}, OneQuarterData: CardLadderBucket{Velocity: 5}},
		},
		cardErr: map[string]error{"B": errors.New("boom")},
	}
	svc := NewService(client)
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
	client := &fakeCatalogClient{catalogErr: want}
	svc := NewService(client)
	_, err := svc.Opportunities(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want wraps %v", err, want)
	}
}

func TestService_Opportunities_CachesCardLadder(t *testing.T) {
	client := &fakeCatalogClient{
		catalog: Catalog{Cards: []CatalogCard{
			{Cert: "A", Category: "POKEMON CARDS", Grade: "10", Price: 10000},
		}},
		cardLadder: map[string]CardLadder{
			"A": {EstimatedValue: 20000, Confidence: 5, OneMonthData: CardLadderBucket{Velocity: 5}, OneQuarterData: CardLadderBucket{Velocity: 5}},
		},
	}
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	tick := now
	svc := NewService(client,
		withClock(func() time.Time { return tick }),
		WithCardLadderCacheTTL(24*time.Hour),
	)
	// First call: cache miss, adapter hit.
	if _, err := svc.Opportunities(context.Background()); err != nil {
		t.Fatalf("call 1: %v", err)
	}
	if client.calls["A"] != 1 {
		t.Fatalf("after call 1: calls[A] = %d, want 1", client.calls["A"])
	}
	// Second call within TTL: cache hit, no new adapter call.
	tick = now.Add(23 * time.Hour)
	if _, err := svc.Opportunities(context.Background()); err != nil {
		t.Fatalf("call 2: %v", err)
	}
	if client.calls["A"] != 1 {
		t.Fatalf("after call 2 (23h): calls[A] = %d, want 1 (cache hit)", client.calls["A"])
	}
	// Third call past TTL: refetch.
	tick = now.Add(25 * time.Hour)
	if _, err := svc.Opportunities(context.Background()); err != nil {
		t.Fatalf("call 3: %v", err)
	}
	if client.calls["A"] != 2 {
		t.Fatalf("after call 3 (25h): calls[A] = %d, want 2 (refetch)", client.calls["A"])
	}
}

func TestService_Opportunities_CacheDoesNotStoreErrors(t *testing.T) {
	client := &fakeCatalogClient{
		catalog: Catalog{Cards: []CatalogCard{
			{Cert: "A", Category: "POKEMON CARDS", Grade: "10", Price: 10000},
		}},
		cardLadder: map[string]CardLadder{},
		cardErr:    map[string]error{"A": errors.New("transient")},
	}
	svc := NewService(client, WithCardLadderCacheTTL(24*time.Hour))
	if _, err := svc.Opportunities(context.Background()); err != nil {
		t.Fatalf("call 1: %v", err)
	}
	// Recover by giving the fake the data and clearing the error
	client.cardErr = nil
	client.cardLadder["A"] = CardLadder{EstimatedValue: 20000, Confidence: 5, OneMonthData: CardLadderBucket{Velocity: 5}, OneQuarterData: CardLadderBucket{Velocity: 5}}
	res, err := svc.Opportunities(context.Background())
	if err != nil {
		t.Fatalf("call 2: %v", err)
	}
	if res.AfterFilter != 1 {
		t.Fatalf("afterFilter = %d, want 1 (errors must not be cached)", res.AfterFilter)
	}
	if client.calls["A"] != 2 {
		t.Fatalf("calls[A] = %d, want 2 (errors must not poison the cache)", client.calls["A"])
	}
}
