package inventory

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// enrichCompSummaries attaches CompSummary to aging items that have a gemRateID.
// Uses the batch CompSummaryProvider method so the whole set costs three SQL
// queries regardless of how many unique variants are in the inventory page.
func (s *service) enrichCompSummaries(ctx context.Context, items []AgingItem) {
	if s.compProv == nil {
		return
	}

	// compCacheKey groups purchases by card variant + grade so that different grades
	// of the same card get separate comp summaries.
	type compCacheKey struct {
		gemRateID  string
		gradeValue float64
	}

	// Collect unique (gemRateID, grade) pairs, picking one representative cert each.
	// The batch method resolves condition from that representative cert.
	seen := make(map[compCacheKey]string)
	for i := range items {
		p := &items[i].Purchase
		if p.GemRateID == "" {
			continue
		}
		key := compCacheKey{gemRateID: p.GemRateID, gradeValue: p.GradeValue}
		if _, ok := seen[key]; !ok {
			seen[key] = p.CertNumber
		}
	}
	if len(seen) == 0 {
		return
	}

	// Build the batch key list in a stable order to make logs reproducible.
	batchKeys := make([]CompKey, 0, len(seen))
	cacheKeyFor := make(map[CompKey]compCacheKey, len(seen))
	for k, cert := range seen {
		bk := CompKey{GemRateID: k.gemRateID, CertNumber: cert}
		batchKeys = append(batchKeys, bk)
		cacheKeyFor[bk] = k
	}

	results, err := s.compProv.GetCompSummariesByKeys(ctx, batchKeys)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "batch comp summary lookup failed", observability.Err(err))
		}
		return
	}

	// Re-index by cache key for O(1) attachment below.
	cache := make(map[compCacheKey]*CompSummary, len(results))
	for bk, summary := range results {
		if summary == nil {
			continue
		}
		cache[cacheKeyFor[bk]] = summary
	}

	// Attach to items — derive CompsAboveCL and CompsAboveCost per-purchase
	// since different purchases of the same card may have different CL values and costs.
	for i := range items {
		p := &items[i].Purchase
		key := compCacheKey{gemRateID: p.GemRateID, gradeValue: p.GradeValue}
		summary, ok := cache[key]
		if !ok {
			continue
		}
		cs := *summary
		cs.CompsAboveCL = CountAboveCost(summary.PriceCentsList, p.CLValueCents)
		cs.CompsAboveCost = CountAboveCost(summary.PriceCentsList, p.BuyCostCents)
		cs.PriceCentsList = nil
		items[i].CompSummary = &cs
	}
}
