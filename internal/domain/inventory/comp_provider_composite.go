package inventory

import "context"

// CompositeCompProvider chains multiple CompSummaryProviders, returning results
// from the first provider that has data for each key. Typical order: CL → MM → DH.
type CompositeCompProvider struct {
	providers []CompSummaryProvider
}

// NewCompositeCompProvider creates a provider that tries each delegate in order.
// Nil providers are filtered out.
func NewCompositeCompProvider(providers ...CompSummaryProvider) *CompositeCompProvider {
	filtered := make([]CompSummaryProvider, 0, len(providers))
	for _, p := range providers {
		if p != nil {
			filtered = append(filtered, p)
		}
	}
	return &CompositeCompProvider{providers: filtered}
}

func (c *CompositeCompProvider) GetCompSummary(ctx context.Context, gemRateID, certNumber string) (*CompSummary, error) {
	for _, p := range c.providers {
		sum, err := p.GetCompSummary(ctx, gemRateID, certNumber)
		if err != nil {
			return nil, err
		}
		if sum != nil {
			return sum, nil
		}
	}
	return nil, nil
}

func (c *CompositeCompProvider) GetCompSummariesByKeys(ctx context.Context, keys []CompKey) (map[CompKey]*CompSummary, error) {
	result := make(map[CompKey]*CompSummary)
	remaining := keys

	for _, p := range c.providers {
		if len(remaining) == 0 {
			break
		}
		got, err := p.GetCompSummariesByKeys(ctx, remaining)
		if err != nil {
			return nil, err
		}
		for k, v := range got {
			result[k] = v
		}
		if len(got) > 0 {
			next := make([]CompKey, 0, len(remaining)-len(got))
			for _, k := range remaining {
				if _, ok := got[k]; !ok {
					next = append(next, k)
				}
			}
			remaining = next
		}
	}
	return result, nil
}

var _ CompSummaryProvider = (*CompositeCompProvider)(nil)
