package dhprice_test

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/clients/dhprice"
	"github.com/guarzo/slabledger/internal/domain/arbitrage"
)

type stubIDLookup struct {
	ids map[string]string // "cardName|setName|cardNumber" → external_id
}

func (s *stubIDLookup) GetExternalID(_ context.Context, cardName, setName, collectorNumber, _ string) (string, error) {
	key := cardName + "|" + setName + "|" + collectorNumber
	return s.ids[key], nil
}

type stubBatchClient struct {
	response *dh.BatchAnalyticsResponse
}

func (s *stubBatchClient) BatchAnalytics(_ context.Context, _ []int, _ []string) (*dh.BatchAnalyticsResponse, error) {
	return s.response, nil
}

func TestBatchAdapter_ResolveDHCardID(t *testing.T) {
	adapter := dhprice.NewBatchAdapter(
		nil,
		&stubIDLookup{ids: map[string]string{
			"Charizard|Base Set|4": "42",
		}},
	)

	id, err := adapter.ResolveDHCardID(context.Background(), "Charizard", "Base Set", "4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 42 {
		t.Errorf("expected 42, got %d", id)
	}
}

func TestBatchAdapter_ResolveDHCardID_NotFound(t *testing.T) {
	adapter := dhprice.NewBatchAdapter(nil, &stubIDLookup{ids: map[string]string{}})

	id, err := adapter.ResolveDHCardID(context.Background(), "Unknown", "Set", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 0 {
		t.Errorf("expected 0 for unmapped card, got %d", id)
	}
}

func TestBatchAdapter_BatchPriceDistribution(t *testing.T) {
	distMap := dh.PriceDistributionMetrics{
		"PSA 10": {Min: 100.0, Max: 300.0, Median: 200.0, Avg: 190.0, SampleSize: 15},
		"Raw":    {Min: 50.0, Max: 150.0, Median: 80.0, Avg: 85.0, SampleSize: 10},
	}
	adapter := dhprice.NewBatchAdapter(
		&stubBatchClient{response: &dh.BatchAnalyticsResponse{
			Results: []dh.CardAnalytics{
				{
					CardID:            42,
					PriceDistribution: &distMap,
				},
			},
		}},
		nil,
	)

	result, err := adapter.BatchPriceDistribution(context.Background(), []int{42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dist, ok := result[42]
	if !ok {
		t.Fatal("expected card 42 in results")
	}

	psa10 := dist.ByGrade["PSA 10"]
	if psa10.MedianCents != 20000 {
		t.Errorf("expected PSA 10 median 20000 cents, got %d", psa10.MedianCents)
	}
	if psa10.SampleSize != 15 {
		t.Errorf("expected sample size 15, got %d", psa10.SampleSize)
	}

	raw := dist.ByGrade["Raw"]
	if raw.MedianCents != 8000 {
		t.Errorf("expected Raw median 8000 cents, got %d", raw.MedianCents)
	}
}

func TestBatchAdapter_BatchPriceDistribution_SkipsErrors(t *testing.T) {
	distMap := dh.PriceDistributionMetrics{
		"PSA 8": {Median: 50.0, SampleSize: 5},
	}
	adapter := dhprice.NewBatchAdapter(
		&stubBatchClient{response: &dh.BatchAnalyticsResponse{
			Results: []dh.CardAnalytics{
				{CardID: 42, Error: "not computed yet"},
				{CardID: 43, PriceDistribution: &distMap},
			},
		}},
		nil,
	)

	result, err := adapter.BatchPriceDistribution(context.Background(), []int{42, 43})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := result[42]; ok {
		t.Error("expected card 42 to be omitted (has error)")
	}
	if _, ok := result[43]; !ok {
		t.Error("expected card 43 in results")
	}
}

var _ arbitrage.BatchPricer = (*dhprice.BatchAdapter)(nil)
