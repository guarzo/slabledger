package campaigns

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// enrichCompSummaries attaches CompSummary to aging items that have a gemRateID.
// Computes once per unique gemRateID + grade combination (since gemRateID is grade-agnostic)
// and derives per-purchase CompsAboveCL and CompsAboveCost from the cached PriceCentsList.
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

	// Collect unique gemRateID + grade pairs with a representative cert for condition lookup
	seen := make(map[compCacheKey]string) // key → certNumber
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

	// Compute summaries once per gemRateID + grade (threshold-agnostic)
	cache := make(map[compCacheKey]*CompSummary, len(seen))
	for key, certNumber := range seen {
		summary, err := s.compProv.GetCompSummary(ctx, key.gemRateID, certNumber)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "comp summary lookup failed",
					observability.String("gemRateId", key.gemRateID), observability.Err(err))
			}
			continue
		}
		if summary != nil {
			cache[key] = summary
		}
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
