package scheduler

import (
	"context"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// refreshSalesComps fetches recent sales comps for every unique
// (gemRateID, condition) pair in the provided mappings and upserts them into
// the CL sales comps store. Rate-limited by the client's built-in limiter.
func (s *CardLadderRefreshScheduler) refreshSalesComps(ctx context.Context, client *cardladder.Client, mappings []postgres.CLCardMapping) {
	seen := make(map[compKey]bool, len(mappings))
	fetched := 0

	for _, m := range mappings {
		if m.CLGemRateID == "" || m.CLCondition == "" {
			continue
		}

		key := compKey{m.CLGemRateID, m.CLCondition}
		if seen[key] {
			continue
		}
		seen[key] = true

		select {
		case <-ctx.Done():
			return
		default:
		}

		// Extract grader from condition (e.g. "PSA 10" → "psa").
		grader := "psa"
		if parts := strings.SplitN(m.CLCondition, " ", 2); len(parts) > 0 && parts[0] != "" {
			grader = strings.ToLower(parts[0])
		}

		resp, err := client.FetchSalesComps(ctx, m.CLGemRateID, m.CLCondition, grader, 0, 100)
		if err != nil {
			s.logger.Warn(ctx, "CL sales: fetch failed",
				observability.String("gemRateId", m.CLGemRateID),
				observability.Err(err))
			continue
		}

		for _, comp := range resp.Hits {
			priceCents := mathutil.ToCentsInt(comp.Price)
			saleDate := comp.Date
			if len(saleDate) > 10 {
				saleDate = saleDate[:10]
			}
			if err := s.salesStore.UpsertSaleComp(ctx, postgres.CLSaleCompRecord{
				GemRateID:   comp.GemRateID,
				Condition:   m.CLCondition,
				ItemID:      comp.ItemID,
				SaleDate:    saleDate,
				PriceCents:  priceCents,
				Platform:    comp.Platform,
				ListingType: comp.ListingType,
				Seller:      comp.Seller,
				ItemURL:     comp.URL,
				SlabSerial:  comp.SlabSerial,
			}); err != nil {
				s.logger.Debug(ctx, "CL sales: upsert failed",
					observability.String("itemId", comp.ItemID),
					observability.Err(err))
			}
		}
		fetched++
	}

	s.logger.Info(ctx, "CL sales: refresh complete",
		observability.Int("cardsProcessed", fetched))
}
