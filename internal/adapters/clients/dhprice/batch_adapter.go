package dhprice

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/domain/arbitrage"
)

// BatchAnalyticsClient is the subset of dh.Client needed for batch pricing.
type BatchAnalyticsClient interface {
	BatchAnalytics(ctx context.Context, cardIDs []int, fields []string) (*dh.BatchAnalyticsResponse, error)
}

// BatchAdapter implements arbitrage.BatchPricer by wrapping the DH
// BatchAnalytics endpoint and the card_id_mappings DB table.
type BatchAdapter struct {
	client     BatchAnalyticsClient
	idResolver CardIDLookup
}

// NewBatchAdapter constructs a BatchAdapter. Either dependency may be nil
// if only one method will be called (e.g. tests that only exercise
// ResolveDHCardID don't need a BatchAnalyticsClient).
func NewBatchAdapter(client BatchAnalyticsClient, idResolver CardIDLookup) *BatchAdapter {
	return &BatchAdapter{client: client, idResolver: idResolver}
}

const dhBatchProviderKey = "doubleholo"

// ResolveDHCardID looks up the DH card ID from the card_id_mappings table.
// Returns 0 if the card has no mapping.
func (a *BatchAdapter) ResolveDHCardID(ctx context.Context, cardName, setName, cardNumber string) (int, error) {
	if a.idResolver == nil {
		return 0, nil
	}
	extID, err := a.idResolver.GetExternalID(ctx, cardName, setName, cardNumber, dhBatchProviderKey)
	if err != nil {
		return 0, fmt.Errorf("resolve DH card ID for %q: %w", cardName, err)
	}
	if extID == "" {
		return 0, nil
	}
	id, err := strconv.Atoi(extID)
	if err != nil {
		return 0, fmt.Errorf("invalid DH card ID %q: %w", extID, err)
	}
	return id, nil
}

// BatchPriceDistribution calls the DH BatchAnalytics endpoint with
// ["price_distribution"] and converts the response to domain types.
// Cards with errors or no price_distribution data are omitted.
func (a *BatchAdapter) BatchPriceDistribution(ctx context.Context, cardIDs []int) (map[int]arbitrage.GradedDistribution, error) {
	if a.client == nil || len(cardIDs) == 0 {
		return map[int]arbitrage.GradedDistribution{}, nil
	}

	resp, err := a.client.BatchAnalytics(ctx, cardIDs, []string{"price_distribution"})
	if err != nil {
		return nil, fmt.Errorf("batch analytics: %w", err)
	}

	out := make(map[int]arbitrage.GradedDistribution, len(resp.Results))
	for _, r := range resp.Results {
		if r.Error != "" || r.PriceDistribution == nil {
			continue
		}
		dist := arbitrage.GradedDistribution{
			ByGrade: make(map[string]arbitrage.PriceBucket, len(*r.PriceDistribution)),
		}
		for gradeKey, bucket := range *r.PriceDistribution {
			dist.ByGrade[gradeKey] = arbitrage.PriceBucket{
				MinCents:    dollarsToCents(bucket.Min),
				MedianCents: dollarsToCents(bucket.Median),
				MaxCents:    dollarsToCents(bucket.Max),
				AvgCents:    dollarsToCents(bucket.Avg),
				SampleSize:  bucket.SampleSize,
			}
		}
		out[r.CardID] = dist
	}
	return out, nil
}

func dollarsToCents(dollars float64) int {
	return int(math.Round(dollars * 100))
}

var _ arbitrage.BatchPricer = (*BatchAdapter)(nil)
